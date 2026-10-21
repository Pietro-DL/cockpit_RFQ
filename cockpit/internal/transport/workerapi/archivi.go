package workerapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/archivio"
	"promatec/cockpit/internal/platform/storage/staging"
)

// L'estrazione di un archivio è un JOB, non un pezzo di una richiesta HTTP (blocco 4A).
//
// Prima uno zip veniva scompattato dentro il gestore del result: non dentro la transazione — quello
// era già stato corretto alla voce 1.4 — ma comunque dentro la richiesta. Il server teneva aperta una
// connessione HTTP mentre scriveva su disco fino a mezzo gigabyte di file, e il worker Outlook, che è
// uno per PC ed è seriale, restava fermo ad aspettare la risposta di un lavoro che non era il suo.
//
// Ora il result fa una cosa sola: promuove il contenuto verificato e accoda `estrai_archivio`. Il job
// è di `worker_tipo = 'server'`, quindi lo prende l'esecutore interno — la stessa goroutine che
// scrive sul NAS — che sta accanto al disco e non occupa il web server.
//
// Perché non farlo fare al worker Outlook, che il file ce l'ha già in mano: perché sulla rete
// passerebbero i file scompattati invece dell'archivio. Uno zip da 8 MB con dentro 120 MB di disegni
// viaggia oggi come 8 MB; scompattarlo sulla postazione ne farebbe passare 120. La compressione è il
// motivo per cui l'archivio esiste, e va disfatta il più tardi possibile.

// EstraiArchivio scompatta un archivio già in staging, mette ogni voce fra i contenuti e la registra
// come allegato figlio con la sua proposta e la sua analisi. Restituisce quante voci ha trovato.
//
// È ripetibile, e deve esserlo: se il server muore a metà, il job torna in coda e riparte da capo.
// Le voci finiscono fra i contenuti con il proprio sha256 per nome — quindi la seconda volta ci sono
// già — e le righe si riscrivono per upsert. Per lo stesso motivo non c'è bisogno che il tentativo
// sia verificato qui dentro: due tentativi che si sovrappongono scrivono le stesse cose, e a chiudere
// il job sarà comunque uno solo (Completa verifica il tentativo e risponde 409 a quello scaduto).
//
// Un archivio illeggibile NON fa fallire il job: riprovarlo darebbe all'infinito lo stesso esito.
// L'allegato va in errore con il motivo visibile, e il job si chiude.
func (s *Server) EstraiArchivio(ctx context.Context, allegatoID uuid.UUID, token uuid.UUID) (int, error) {
	// le voci toccano la RFQ dell'archivio: il flusso ancorato al prodotto la rifa' dopo il commit (IN2)
	ctx, raccolta := fascicolo.ConRismistamenti(ctx)
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, allegatoID)
	if err != nil {
		return 0, fmt.Errorf("allegato %s: %w", allegatoID, err)
	}
	if (!a.PathStaging.Valid || !(staging.FileStaging{}).Presente(a.PathStaging.String)) && s.ArchiviInAltriFile != nil {
		// V1 (U2-P45): un archivio in Altri file si riestrae leggendolo dalla sua copia nella cartella ALTRO, in sola
		// lettura; il servizio del ciclo dei file lo rimette nello staging (per un archivio attivo o scartato non fa niente)
		if p, err := s.ArchiviInAltriFile.ArchivioNelloStaging(ctx, allegatoID); err == nil {
			a.PathStaging = txt(p)
		} else if !errors.Is(err, filerfq.ErrArchivioNonInAltriFile) {
			s.Log.Warn("archivio in Altri file: la copia in ALTRO non torna nello staging", "allegato", allegatoID, "err", err)
		}
	}
	if !a.PathStaging.Valid || !(staging.FileStaging{}).Presente(a.PathStaging.String) {
		// Il file non c'è più: qualcuno ha svuotato lo staging fra il download e adesso. Non è un
		// guasto da ritentare, è un allegato da riscaricare.
		return 0, q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: allegatoID,
			Stato: db.StatoAllegatoErrore, Errore: txt("archivio non presente in staging: usa Riscarica")})
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		return 0, err
	}
	// V1 (EM §10.7, G-7; U2-P45): un archivio scartato nella Sorgente non scrive voci; uno in Altri file rimette solo i
	// byte delle voci attive che ci sono gia', senza voci nuove (si ricontrolla sotto il lucchetto della RFQ, qui sotto:
	// questo evita solo l'estrazione inutile)
	if stato, err := statoOccorrenza(ctx, q, a.AllegatoID); err != nil {
		return 0, err
	} else if stato != "attivo" && stato != "altri_file" {
		s.Log.Info("estrazione di un archivio non attivo nella Sorgente: nessuna voce scritta", "allegato", allegatoID, "stato", stato)
		return 0, nil
	}

	// le voci restano nella cartella temporanea fino al controllo sotto il lucchetto per contenuto (V1, EM §10.6): si
	// promuovono dentro la transazione, se il contenuto non c'e'
	voci, tmp, errEstrazione := s.estraiInTemporanea(a.PathStaging.String, token)
	defer os.RemoveAll(tmp)
	troncato := errors.Is(errEstrazione, archivio.ErrLimite)
	if errEstrazione != nil && !troncato {
		s.Log.Warn("archivio non leggibile", "allegato", allegatoID, "file", a.NomeFile, "err", errEstrazione)
		return 0, q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: allegatoID,
			Stato: db.StatoAllegatoErrore, Errore: txt("archivio non leggibile: " + errEstrazione.Error())})
	}

	// Le righe in una transazione sola: o l'archivio ha le sue voci con le loro proposte e le loro
	// analisi, oppure non ne ha nessuna. Un archivio registrato a metà si presenterebbe all'operatore
	// come completo, e mancherebbero proprio i disegni che il job non ha fatto in tempo a scrivere.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	qt := db.New(tx)
	// V1 (EM §10.6, §10.7 G-7): la RFQ, poi le righe, poi i contenuti (in ordine crescente). Sotto il lucchetto della RFQ
	// lo stato dell'archivio e delle voci gia' scritte non cambia: un archivio non attivo non scrive voci, una voce
	// scartata non si riscrive, una voce in Altri file non riceve la lettura, e una voce con un nome o uno sha diversi
	// dalla sua fotografia e' un'anomalia della riestrazione (l'evento lo scrive il servizio del ciclo dei file).
	if m.ThreadID.Valid {
		if _, err := qt.BloccaThread(ctx, m.ThreadID.UUID); err != nil {
			return 0, err
		}
	}
	soloEsistenti := false
	if stato, err := statoOccorrenza(ctx, qt, a.AllegatoID); err != nil {
		return 0, err
	} else if stato == "altri_file" {
		soloEsistenti = true
	} else if stato != "attivo" {
		s.Log.Info("estrazione di un archivio non attivo nella Sorgente: nessuna voce scritta", "allegato", allegatoID, "stato", stato)
		return 0, nil
	}
	esistenti, err := vociEsistenti(ctx, qt, a)
	if err != nil {
		return 0, err
	}
	shaDaBloccare := map[string]bool{}
	for i, v := range voci {
		if ex, c := esistenti[int16(i+1)]; (c && ex.stato != "attivo") || (soloEsistenti && !c) {
			continue
		}
		shaDaBloccare[strings.ToLower(v.Sha256)] = true
	}
	ordinati := make([]string, 0, len(shaDaBloccare))
	for h := range shaDaBloccare {
		ordinati = append(ordinati, h)
	}
	sort.Strings(ordinati)
	for _, h := range ordinati {
		if err := qt.BloccaContenuto(ctx, h); err != nil {
			return 0, err
		}
	}
	// Prima gli STEP e i PDF il cui nome e' compatibile con un prodotto della RFQ (Smistamento F8, A5.13.6):
	// la loro analisi passa prima delle altre voci, perche' e' da loro che l'ancora puo' arrivare.
	var compatibili fascicolo.Compatibili
	if m.ThreadID.Valid {
		if compatibili, err = fascicolo.CompatibiliConIProdotti(ctx, qt, m.ThreadID.UUID); err != nil {
			return 0, err
		}
		fascicolo.SegnaRismistamento(ctx, m.ThreadID.UUID)
	}
	for i, v := range voci {
		ex, c := esistenti[int16(i+1)]
		if soloEsistenti && !c {
			// un archivio in Altri file non scrive voci nuove (U2-P45)
			continue
		}
		if soloEsistenti && (ex.nomeFoto != v.NomeFile || !strings.EqualFold(ex.shaFoto, v.Sha256)) {
			// la voce attiva gia' scritta non e' quella che la riestrazione trova a quell'indice: anomalia, niente cambia
			if err := filerfq.SegnaAnomaliaRiestrazione(ctx, qt, m.ThreadID.UUID, ex.allegatoID, map[string]any{
				"indice": i + 1, "nome_trovato": v.NomeFile, "sha256_trovato": strings.ToLower(v.Sha256),
				"nome_fotografia": ex.nomeFoto, "sha256_fotografia": ex.shaFoto}); err != nil {
				return 0, err
			}
			continue
		}
		if c && ex.stato != "attivo" {
			// una voce scartata non si riscrive; una in Altri file non riceve la lettura e non cambia (la sua copia di
			// conservazione e' di quei byte). Se la riestrazione mette un altro file allo stesso indice, lo si dice.
			if ex.nomeFoto != v.NomeFile || !strings.EqualFold(ex.shaFoto, v.Sha256) {
				if err := filerfq.SegnaAnomaliaRiestrazione(ctx, qt, m.ThreadID.UUID, ex.allegatoID, map[string]any{
					"indice": i + 1, "nome_trovato": v.NomeFile, "sha256_trovato": strings.ToLower(v.Sha256),
					"nome_fotografia": ex.nomeFoto, "sha256_fotografia": ex.shaFoto}); err != nil {
					return 0, err
				}
			}
			continue
		}
		// il contenuto sotto il lucchetto: se c'e' si usa, altrimenti si promuove la voce estratta
		if err := adottaVoce(&voci[i]); err != nil {
			return 0, err
		}
		v = voci[i]
		figlio, err := qt.UpsertAllegato(ctx, db.UpsertAllegatoParams{
			MessaggioID: a.MessaggioID, ContenitoreID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true}, Indice: int16(i + 1),
			NomeFile: v.NomeFile, PathInterno: txt(v.PathInterno), Estensione: txt(strings.ToLower(strings.TrimPrefix(filepath.Ext(v.NomeFile), "."))),
			Natura: db.NaturaAllegatoFile, Origine: a.Origine, Bytes: pgtype.Int8{Int64: v.Bytes, Valid: true}, Sha256: txt(v.Sha256), RicevutoIl: a.RicevutoIl,
		})
		if err != nil {
			return 0, fmt.Errorf("voce zip %s: %w", v.NomeFile, err)
		}
		if err := qt.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: figlio.AllegatoID, PathStaging: txt(v.Path), Sha256: txt(v.Sha256), Bytes: pgtype.Int8{Int64: v.Bytes, Valid: true}}); err != nil {
			return 0, err
		}
		figlio.PathStaging, figlio.Sha256 = txt(v.Path), txt(v.Sha256)
		// ogni voce si legge dal SUO nome (K9, P15): lo stesso disegno in due archivi con due nomi ha due letture
		rp, err := s.scriviProposta(ctx, qt, figlio, m.ThreadID, classificazione.IngressoFile{Da: classificazione.DaArchivio,
			Bytes: v.Bytes, Direzione: string(m.Direzione)}, map[string]any{"path_interno": v.PathInterno, "bytes": v.Bytes, "zip": a.NomeFile})
		if err != nil {
			return 0, err
		}
		if rp.Tipo == "rumore" {
			_ = qt.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: figlio.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
			continue
		}
		priorita := coda.PrioritaAnalisi
		if compatibili.Compatibile(figlio.NomeFile, v.PathInterno, a.NomeFile) {
			priorita = coda.PrioritaAnalisiCompatibile
		}
		jf, err := coda.AccodaAnalisiCon(ctx, qt, figlio, m.ThreadID, s.Analizzatore, priorita)
		if err != nil {
			return 0, err
		}
		if jf == nil {
			// lo stesso disegno dentro due zip di due mail: i fatti ci sono già, e la voce nuova li
			// riceve subito invece di restare con la sola proposta dal nome
			if err := s.applicaFattiEsistenti(ctx, qt, figlio); err != nil {
				return 0, err
			}
		}
	}
	// Lo zip stesso resta un contenitore: non va sul NAS a meno di una conferma esplicita.
	dettagli := map[string]any{"voci": len(voci), "estensione": estensioneArchivio(a.PathStaging.String, a.Estensione.String), "bytes": a.Bytes.Int64}
	if troncato {
		dettagli["troncato"] = true
	}
	// l'archivio non dice il tipo (ext_archivio, stato `nessuna`): la valutazione lo dice, e le voci si
	// propongono una per una
	if _, err := s.scriviProposta(ctx, qt, a, m.ThreadID, classificazione.IngressoFile{Da: classificazione.DaArchivio,
		Bytes: a.Bytes.Int64, Direzione: string(m.Direzione)}, dettagli); err != nil {
		return 0, err
	}
	if err := qt.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	s.Log.Info("archivio estratto", "allegato", allegatoID, "file", a.NomeFile, "voci", len(voci), "troncato", troncato)
	raccolta.Esegui(ctx, s.smistatore(), s.Log)
	return len(voci), nil
}

// voceEstratta e' una voce scompattata nella cartella temporanea del tentativo (Path), con il posto che avra' fra i
// contenuti (dest).
type voceEstratta struct {
	archivio.Voce
	dest string
}

// estraiInTemporanea estrae un archivio nella cartella temporanea del tentativo e calcola per ogni voce il suo posto
// fra i contenuti, con il suo hash per nome. Non promuove niente: la promozione sta nella transazione che scrive
// path_staging, sotto il lucchetto per contenuto (V1, EM §10.6; adottaVoce). La cartella la toglie chi chiama, dopo.
//
// L'estrazione passa per una cartella temporanea perché non c'è modo di fare altrimenti: il nome
// definitivo di una voce è il suo sha256, e lo sha256 si conosce dopo averla scritta. La temporanea
// sta sotto _parti, con il token del tentativo nel nome, così anche lei è di QUESTO tentativo e la
// pulizia sa di chi era se il tentativo non arriva in fondo.
func (s *Server) estraiInTemporanea(zip string, token uuid.UUID) ([]voceEstratta, string, error) {
	tmp := staging.PercorsoEstrazione(s.Staging, token)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, tmp, err
	}
	estratte, errEstrazione := archivio.Estrai(zip, tmp)
	voci := make([]voceEstratta, 0, len(estratte))
	for _, v := range estratte {
		dest, err := staging.PercorsoContenuto(s.Staging, v.Sha256, v.NomeFile)
		if err != nil {
			return voci, tmp, err
		}
		voci = append(voci, voceEstratta{Voce: v, dest: dest})
	}
	return voci, tmp, errEstrazione
}

// adottaVoce: sotto il lucchetto per contenuto, una voce gia' presente fra i contenuti (lo stesso disegno dentro due
// archivi diversi, che e' la norma quando un cliente rimanda la stessa commessa con una revisione in piu') non si
// riscrive; altrimenti la voce estratta si promuove. Il Path della voce diventa quello del contenuto.
func adottaVoce(v *voceEstratta) error {
	if staging.ContenutoGiaPresente(v.dest, v.Sha256) {
		staging.ToccaContenuto(v.dest)
		v.Path = v.dest
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(v.dest), 0o755); err != nil {
		return err
	}
	if err := os.Rename(v.Path, v.dest); err != nil {
		return fmt.Errorf("voce %s dello zip: %w", v.NomeFile, err)
	}
	v.Path = v.dest
	return nil
}

// voceEsistente: una voce gia' scritta dell'archivio (per indice), con lo stato della sua occorrenza e, se ha una riga
// di stato, la fotografia (nome e sha) presa al primo gesto.
type voceEsistente struct {
	allegatoID uuid.UUID
	stato      string
	nomeFoto   string
	shaFoto    string
}

// vociEsistenti: le voci gia' scritte dell'archivio, per indice.
func vociEsistenti(ctx context.Context, q *db.Queries, a db.Allegato) (map[int16]voceEsistente, error) {
	tutte, err := q.ListAllegatiMessaggio(ctx, a.MessaggioID)
	if err != nil {
		return nil, err
	}
	out := map[int16]voceEsistente{}
	for _, x := range tutte {
		if !x.ContenitoreID.Valid || x.ContenitoreID.UUID != a.AllegatoID {
			continue
		}
		st, err := q.StatoOccorrenzaAllegato(ctx, x.AllegatoID)
		if err != nil {
			return nil, err
		}
		ve := voceEsistente{allegatoID: x.AllegatoID, stato: st.Stato, nomeFoto: x.NomeFile, shaFoto: x.Sha256.String}
		if st.Stato != "attivo" && st.ThreadID.Valid {
			if f, err := q.GetFileRfq(ctx, db.GetFileRfqParams{ThreadID: st.ThreadID.UUID, AllegatoID: x.AllegatoID}); err == nil {
				ve.nomeFoto, ve.shaFoto = f.NomeFile, f.Sha256.String
			}
		}
		out[x.Indice] = ve
	}
	return out, nil
}

// estensioneArchivio: il formato vero dell'archivio (zip o 7z, dai primi byte) per i dettagli della
// proposta; se i byte non si leggono, l'estensione del nome.
func estensioneArchivio(percorso, ext string) string {
	if f, err := archivio.Formato(percorso); err == nil {
		return f
	}
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}
