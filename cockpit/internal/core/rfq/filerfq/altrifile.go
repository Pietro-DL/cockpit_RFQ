package filerfq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
)

// battitoOgni: ogni quanto una copia in corso rinfresca aggiornato_il della sua riga (EM §10.5).
var battitoOgni = 60 * time.Second

// battitoFermo: dopo quanto senza battito una copia in_copia di un'altra chiave non e' piu' viva (EM §10.5).
const battitoFermo = 15 * time.Minute

// MettiInAltriFile e' S3, «Metti in Altri file» (EM §10.5; base di lavoro della domanda V1-Q06 A): prima la copia
// verificata nel deposito, poi lo stato. Fase A in una transazione breve (controlli, sorgenti dei byte, righe in_copia
// sotto BloccaContenuto); la copia fuori transazione, con il battito; fase C in una transazione (la propria riga ancora
// in_copia e con la propria chiave, i ricontrolli, poi copiata, lo stato altri_file, l'evento, lo scollegamento
// chiesto, l'esito dell'intento). Ogni esito diverso dal successo porta le righe del gesto a uno stato finale o in
// rilascio: niente byte orfani nel deposito.
func (s *Servizio) MettiInAltriFile(ctx context.Context, thread, allegato, utente uuid.UUID, c CorpoGesto) (*EsitoGesto, error) {
	if s.Deposito == nil {
		return nil, ErrConservazioneNonConfigurata
	}
	g, err := nuovoGesto(GestoAltriFile, thread, allegato, utente, c)
	if err != nil {
		return nil, err
	}
	// fase A
	var righe []rigaCopia
	var registrato *EsitoGesto
	err = s.inTx(ctx, func(q *db.Queries) error {
		pr, reg, err := s.prepara(ctx, q, g)
		if err != nil {
			return err
		}
		if reg != nil {
			registrato = reg
			return nil
		}
		righe, err = s.faseA(ctx, q, g, pr)
		return err
	})
	if err != nil {
		return nil, err
	}
	if registrato != nil {
		return s.ricomponi(ctx, thread, registrato)
	}
	// la copia, fuori transazione
	if err := s.copia(ctx, g, righe); err != nil {
		return nil, err
	}
	if s.Agganci != nil && s.Agganci.DopoLaCopia != nil {
		s.Agganci.DopoLaCopia(allegato)
	}
	// fase C
	var esito *EsitoGesto
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		esito, err = s.faseC(ctx, q, g, righe)
		return err
	})
	if err != nil {
		if _, ok := ComeRifiuto(err); ok {
			// la fase C rifiutata (versione, dipendenze, perimetro, o 409 tentativo_chiuso per una riga chiusa da altri):
			// le proprie righe ANCORA in_copia con la propria chiave vanno in rilascio (causa rifiutata), e si tenta il
			// rilascio. La riga chiusa da altri non e' piu' in_copia con questa chiave, e rifiutaRighe non la tocca (EM
			// §10.5: «la fase C non tocca la conservazione» di quella riga); le altre righe del gesto non restano in
			// copia con il battito fermo (TR-05: «per tutte le righe del gesto»; rilievo della revisione). Un errore che
			// non e' un rifiuto lascia le righe in copia: il reinvio con la stessa chiave le riprende.
			s.rifiutaRighe(ctx, g, righe)
		}
		return nil, err
	}
	return s.ricomponi(ctx, thread, esito)
}

// rigaCopia: una copia del gesto, con quello che serve alla copia fuori transazione.
type rigaCopia struct {
	conservazioneID int64
	occ             Occorrenza
	percorso        string // il definitivo nel deposito, calcolato dal database
	ripresa         bool   // la stessa chiave: una riga in_copia ripresa
}

// faseA: le sorgenti dei byte per ogni occorrenza (altrimenti 422 FILE_SENZA_BYTE e nessuna riga), poi sotto
// BloccaContenuto, sha per sha in ordine crescente, la riga viva esistente e la riga nuova (EM §10.5, fase A).
func (s *Servizio) faseA(ctx context.Context, q *db.Queries, g gesto, pr *preparato) ([]rigaCopia, error) {
	var daCopiare []Occorrenza
	for _, o := range pr.tutte {
		if o.Stato == StatoAttivo && !o.ScartatoLegacy {
			daCopiare = append(daCopiare, o)
		}
	}
	for _, o := range daCopiare {
		ok, err := s.haSorgenteDeiByte(ctx, q, o)
		if err != nil {
			return nil, err
		}
		if !ok {
			t := "Niente è cambiato: il file non è più nello staging. Ricaricalo e riprova."
			if o.Origine == db.OrigineAllegatoOutlook {
				t = "Niente è cambiato: il file non è più nello staging. Scarica il file e riprova."
			}
			return nil, rifiuto(http.StatusUnprocessableEntity, "FILE_SENZA_BYTE", t, extra("allegato_id", o.AllegatoID,
				"origine", string(o.Origine)))
		}
	}
	// le righe vive di ogni occorrenza, FOR UPDATE, prima dei lucchetti per contenuto (EM §10.6: la RFQ, le righe, per
	// ultimo il contenuto), in ordine di allegato; la riga nuova (il pin «conservazione in corso») nasce sotto il
	// lucchetto per contenuto, come vuole EM §10.5
	vivePer := map[uuid.UUID][]db.FileRfqConservazione{}
	for _, o := range daCopiare {
		vive, err := q.ListConservazioniVive(ctx, db.ListConservazioniViveParams{ThreadID: g.thread, AllegatoID: o.AllegatoID})
		if err != nil {
			return nil, err
		}
		vivePer[o.AllegatoID] = vive
	}
	sort.SliceStable(daCopiare, func(i, j int) bool {
		if daCopiare[i].Sha() != daCopiare[j].Sha() {
			return daCopiare[i].Sha() < daCopiare[j].Sha()
		}
		return daCopiare[i].AllegatoID.String() < daCopiare[j].AllegatoID.String()
	})
	visto := map[string]bool{}
	for _, o := range daCopiare {
		if !visto[o.Sha()] {
			visto[o.Sha()] = true
			if err := q.BloccaContenuto(ctx, o.Sha()); err != nil {
				return nil, err
			}
		}
	}
	var righe []rigaCopia
	for _, o := range daCopiare {
		vive := vivePer[o.AllegatoID]
		ripresa := false
		for _, c := range vive {
			switch {
			case c.Stato == "in_copia" && c.ChiaveIdempotenza.Valid && c.ChiaveIdempotenza.UUID == g.it.chiave:
				p, err := s.Deposito.PercorsoCopia(c.PercorsoRelativo)
				if err != nil {
					return nil, err
				}
				righe = append(righe, rigaCopia{conservazioneID: c.ConservazioneID, occ: o, percorso: p, ripresa: true})
				ripresa = true
			case c.Stato == "in_copia" && s.adesso().Sub(c.AggiornatoIl) < battitoFermo:
				return nil, rifiuto(http.StatusConflict, "COPIA_IN_CORSO",
					"Un'altra copia di conservazione di questo file è in corso: aspetta che finisca e rileggi la Sorgente.",
					extra("motivo", "copia_viva", "allegato_id", o.AllegatoID))
			case c.Stato == "in_copia":
				// il battito e' fermo da oltre 15 minuti: la riga vecchia va in rilascio (il suo file e i suoi parziali si
				// tolgono con il rilascio) e se ne apre una nuova
				if _, err := q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "da_rilasciare",
					Causa: testo("tempo_scaduto"), Tentativi: c.Tentativi, ConservazioneID: c.ConservazioneID, StatoAtteso: "in_copia"}); err != nil {
					return nil, err
				}
			case c.Stato == "copiata":
				// un file attivo con una copia ancora copiata: non dovrebbe succedere (la «Riattiva» la porta in rilascio);
				// la si porta in rilascio, cosi' la riga nuova ha il suo file
				if _, err := q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "da_rilasciare",
					Causa: testo("riattivato"), Tentativi: c.Tentativi, ConservazioneID: c.ConservazioneID, StatoAtteso: "copiata"}); err != nil {
					return nil, err
				}
			}
		}
		if ripresa {
			continue
		}
		nuova, err := q.InsertConservazione(ctx, db.InsertConservazioneParams{ThreadID: g.thread, AllegatoID: o.AllegatoID,
			Sha256: o.Sha(), Bytes: o.Bytes, DepositoID: s.Deposito.ID, ChiaveIdempotenza: uid(g.it.chiave)})
		if err != nil {
			return nil, err
		}
		p, err := s.Deposito.PercorsoCopia(nuova.PercorsoRelativo)
		if err != nil {
			return nil, err
		}
		righe = append(righe, rigaCopia{conservazioneID: nuova.ConservazioneID, occ: o, percorso: p})
	}
	return righe, nil
}

// haSorgenteDeiByte: una sorgente dei byte disponibile per la copia (senza aprire i file grandi: la copia verifica lo
// sha mentre scrive). Le sorgenti, nell'ordine, sono quelle di sorgentiByte.
func (s *Servizio) haSorgenteDeiByte(ctx context.Context, q *db.Queries, o Occorrenza) (bool, error) {
	if o.Sha() == "" {
		return false, nil
	}
	// la prima sorgente, il proprio file nello staging, non vuole query
	if o.PathStaging.Valid && presenteNelloStaging(s.Staging, o.PathStaging.String, o.Sha()) {
		return true, nil
	}
	src, err := s.sorgentiByte(ctx, q, o)
	if err != nil {
		return false, err
	}
	return len(src) > 0, nil
}

// sorgenteByte: un file da cui leggere i byte di un'occorrenza, con il suo tipo (sorgente_byte della riga).
type sorgenteByte struct {
	percorso string
	tipo     string // staging | conservazione | nas
}

// sorgentiByte: dove leggere i byte di un'occorrenza, nell'ordine di EM §10.5 (copia): il path_staging se e' il
// contenuto (EContenuto); un'altra occorrenza della stessa RFQ con lo stesso contenuto; una copia copiata dello stesso
// allegato, anche sotto un'altra RFQ; il documento scritto sul NAS, in sola lettura. Nessun download.
func (s *Servizio) sorgentiByte(ctx context.Context, q *db.Queries, o Occorrenza) ([]sorgenteByte, error) {
	sha := o.Sha()
	var out []sorgenteByte
	if o.PathStaging.Valid && presenteNelloStaging(s.Staging, o.PathStaging.String, sha) {
		out = append(out, sorgenteByte{percorso: o.PathStaging.String, tipo: "staging"})
	}
	altre, err := q.ListCopieStagingStessoSha(ctx, db.ListCopieStagingStessoShaParams{Sha256: sha, Esclusi: []uuid.UUID{o.AllegatoID}})
	if err != nil {
		return nil, err
	}
	for _, a := range altre {
		if a.ThreadID.Valid && a.ThreadID.UUID == o.Thread() && presenteNelloStaging(s.Staging, a.PathStaging, sha) {
			out = append(out, sorgenteByte{percorso: a.PathStaging, tipo: "staging"})
		}
	}
	if s.Deposito != nil {
		copie, err := q.ListCopieConservazioneAllegato(ctx, o.AllegatoID)
		if err != nil {
			return nil, err
		}
		for _, c := range copie {
			if c.Stato != "copiata" || c.DepositoID != s.Deposito.ID {
				continue
			}
			if p, err := s.Deposito.PercorsoCopia(c.PercorsoRelativo); err == nil && esiste(p) {
				out = append(out, sorgenteByte{percorso: p, tipo: "conservazione"})
			}
		}
	}
	if p, ok, err := s.dalNas(ctx, q, o.Thread(), sha); err != nil {
		return nil, err
	} else if ok {
		out = append(out, sorgenteByte{percorso: p, tipo: "nas"})
	}
	return out, nil
}

// dalNas: il file del documento della RFQ con questo contenuto scritto sul NAS, senza anomalie aperte (sola lettura).
func (s *Servizio) dalNas(ctx context.Context, q *db.Queries, thread uuid.UUID, sha string) (string, bool, error) {
	if strings.TrimSpace(s.NASRadice) == "" || sha == "" {
		return "", false, nil
	}
	d, err := q.GetDocumentoScrittoPerSha(ctx, db.GetDocumentoScrittoPerShaParams{ThreadID: thread, Sha256: sha})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	if !d.CartellaRelativa.Valid {
		return "", false, nil
	}
	p, err := documenti.PercorsoSulNas(s.NASRadice, d.CartellaRelativa.String, d.PathRelativo)
	if err != nil {
		return "", false, nil
	}
	if !esiste(p.Assoluto) {
		return "", false, nil
	}
	return p.Assoluto, true, nil
}

// copia scrive i byte di ogni riga del gesto nel deposito (EM §10.5, «Copia»): un parziale con creazione esclusiva, lo
// sha calcolato scrivendo, Sync, il confronto con allegato.sha256; poi la rinomina nel definitivo della riga con la riga
// presa FOR UPDATE e ancora in_copia. Il battito rinfresca la riga ogni minuto. Una copia che non riesce porta TUTTE le
// righe del gesto a uno stato finale o in rilascio, e risponde 422 CONSERVAZIONE_NON_RIUSCITA. Una riga chiusa da altri
// durante la copia (uno «Scarta», un'altra scheda) risponde 409 COPIA_IN_CORSO / tentativo_chiuso, e porta anch'essa le
// ALTRE righe del gesto a uno stato finale o in rilascio (TR-05: «per tutte le righe del gesto»): la riga chiusa non si
// tocca, ma l'archivio e le altre voci non restano in copia con il battito fermo, a tenere il pin «conservazione in
// corso» e a far rispondere copia_viva a un nuovo tentativo per 15-30 minuti (rilievo della revisione).
func (s *Servizio) copia(ctx context.Context, g gesto, righe []rigaCopia) error {
	fermaBattito := s.battito(ctx, g, righe)
	defer fermaBattito()
	var fallita *Rifiuto
	causaFallita := ""
	var fallitaID uuid.UUID
	conDefinitivo := map[int64]bool{}
	for _, r := range righe {
		if s.Agganci != nil && s.Agganci.PrimaDiCopiare != nil {
			s.Agganci.PrimaDiCopiare(r.occ.AllegatoID)
		}
		causa, haDefinitivo, err := s.copiaUna(ctx, g, r)
		conDefinitivo[r.conservazioneID] = haDefinitivo
		if err == nil {
			continue
		}
		s.log().Warn("ciclo dei file: copia di conservazione non riuscita", "conservazione", r.conservazioneID,
			"allegato", r.occ.AllegatoID, "causa", causa, "err", err)
		if causa == "tentativo_chiuso" {
			// un'altra scheda ha ripreso la copia, o uno «Scarta» l'ha chiusa: quella riga non e' piu' di questo
			// tentativo, e la copia toglie solo i suoi parziali (EM §10.5: 409 COPIA_IN_CORSO, tentativo_chiuso)
			for _, p := range s.Deposito.partiDellaRiga(r.conservazioneID) {
				_ = togliFile(p)
			}
			fallita = rifiuto(http.StatusConflict, "COPIA_IN_CORSO",
				"Questo tentativo di copia è stato chiuso (un'altra scheda l'ha ripreso, o il file è stato scartato): rileggi la Sorgente.",
				extra("motivo", "tentativo_chiuso", "allegato_id", r.occ.AllegatoID))
			causaFallita, fallitaID = "rifiutata", uuid.Nil
			break
		}
		fallita = rifiuto(http.StatusUnprocessableEntity, "CONSERVAZIONE_NON_RIUSCITA",
			"Niente è cambiato: la copia di conservazione non è riuscita ("+testoCausa(causa)+"). Il file resta fra gli attivi.",
			extra("causa", causa, "allegato_id", r.occ.AllegatoID))
		causaFallita, fallitaID = causa, r.occ.AllegatoID
		break
	}
	if fallita == nil {
		return nil
	}
	// ogni riga del gesto ancora in copia con la propria chiave in uno stato finale o in rilascio (EM §10.5; giro 1,
	// TR-05): senza un definitivo la riga toglie i suoi parziali e va in errore_copia con la causa (le righe non ancora
	// tentate, e tutte dopo un tentativo chiuso, con «rifiutata»); con un definitivo va in rilascio, e il rilascio lo
	// toglie. Una riga che non e' piu' in copia con questa chiave (la riga chiusa da altri) non si tocca.
	for _, r := range righe {
		// la richiesta puo' essere gia' chiusa (il browser se n'e' andato): la pulizia va fino in fondo lo stesso
		c2 := context.WithoutCancel(ctx)
		_ = s.inTx(c2, func(q *db.Queries) error {
			c, err := q.GetConservazionePerAggiornare(c2, r.conservazioneID)
			if err != nil || c.Stato != "in_copia" || !c.ChiaveIdempotenza.Valid || c.ChiaveIdempotenza.UUID != g.it.chiave {
				return err
			}
			if !conDefinitivo[r.conservazioneID] {
				for _, p := range s.Deposito.partiDellaRiga(r.conservazioneID) {
					_ = togliFile(p)
				}
				causa := "rifiutata"
				if r.occ.AllegatoID == fallitaID {
					causa = causaFallita
				}
				return s.passoConservazione(c2, q, c, "errore_copia", testo(causa), testo(testoCausa(causa)))
			}
			return s.passoConservazione(c2, q, c, "da_rilasciare", testo("rifiutata"), pgtype.Text{})
		})
	}
	for _, r := range righe {
		if !conDefinitivo[r.conservazioneID] {
			continue
		}
		if _, err := s.RilasciaCopia(context.WithoutCancel(ctx), r.conservazioneID, false); err != nil {
			s.log().Warn("ciclo dei file: rilascio dopo una copia non riuscita", "conservazione", r.conservazioneID, "err", err)
		}
	}
	return fallita
}

// testoCausa: la causa di una copia non riuscita, in parole.
func testoCausa(causa string) string {
	switch causa {
	case "spazio":
		return "spazio esaurito nel deposito"
	case "permessi":
		return "permessi insufficienti sul deposito"
	case "sorgente_illeggibile":
		return "i byte del file non si leggono"
	case "verifica_sha":
		return "i byte letti non hanno lo sha256 del file"
	case "conflitto":
		return "nel deposito c'è già un file diverso con lo stesso nome"
	}
	return causa
}

// copiaUna copia i byte di un'occorrenza nel definitivo della sua riga. Restituisce la causa di un errore (spazio,
// permessi, sorgente_illeggibile, verifica_sha, conflitto) e se il definitivo della riga esiste (scritto da questa
// copia: un file gia' presente con un altro contenuto non e' della riga, e non si sovrascrive ne' si rilascia).
func (s *Servizio) copiaUna(ctx context.Context, g gesto, r rigaCopia) (string, bool, error) {
	sha := r.occ.Sha()
	// una ripresa con il definitivo gia' presente e lo sha giusto si tiene; con un altro sha: conflitto (EM §10.5)
	if r.ripresa {
		if h, err := shaDelFile(r.percorso); err == nil {
			if h == sha {
				return "", true, s.confermaDefinitivo(ctx, r, "")
			}
			return "conflitto", false, fmt.Errorf("definitivo gia' presente con un altro contenuto")
		}
	}
	// prima il proprio file nello staging, senza query; le altre sorgenti si cercano solo se serve
	var src []sorgenteByte
	propria := r.occ.PathStaging.Valid && presenteNelloStaging(s.Staging, r.occ.PathStaging.String, sha)
	if propria {
		src = append(src, sorgenteByte{percorso: r.occ.PathStaging.String, tipo: "staging"})
	}
	altre := func() ([]sorgenteByte, error) {
		tutte, err := s.sorgentiByte(ctx, db.New(s.Pool), r.occ)
		if err != nil || !propria {
			return tutte, err
		}
		var out []sorgenteByte
		for _, sb := range tutte {
			if !stessoPercorso(sb.percorso, r.occ.PathStaging.String) {
				out = append(out, sb)
			}
		}
		return out, nil
	}
	if !propria {
		var err error
		if src, err = altre(); err != nil {
			return "sorgente_illeggibile", false, err
		}
	}
	if len(src) == 0 {
		return "sorgente_illeggibile", false, errors.New("nessuna sorgente dei byte")
	}
	var ultimo error
	ultimaCausa := "sorgente_illeggibile"
	cercate := !propria
	for i := 0; i < len(src); i++ {
		sb := src[i]
		parte := s.Deposito.percorsoParte(r.conservazioneID)
		causa, err := scriviParziale(sb.percorso, parte, sha)
		if err != nil {
			ultimo, ultimaCausa = err, causa
			_ = togliFile(parte)
			if causa == "sorgente_illeggibile" || causa == "verifica_sha" {
				// un'altra sorgente puo' andare meglio: finite quelle note, si cercano le altre (una volta)
				if i == len(src)-1 && !cercate {
					cercate = true
					piu, errAltre := altre()
					if errAltre != nil {
						return "sorgente_illeggibile", false, errAltre
					}
					src = append(src, piu...)
				}
				continue
			}
			return causa, false, err
		}
		if err := os.MkdirAll(filepath.Dir(r.percorso), 0o755); err != nil {
			_ = togliFile(parte)
			return causaDi(err), false, err
		}
		causa, err = s.rinomina(ctx, r, parte, sha, sb.tipo)
		if err != nil {
			_ = togliFile(parte)
			// verifica_sha dopo la rinomina: il definitivo e' della riga (si rilascia); conflitto: non e' suo
			return causa, causa == "verifica_sha" && esiste(r.percorso), err
		}
		return "", true, nil
	}
	return ultimaCausa, false, ultimo
}

// scriviParziale copia src nel parziale con creazione esclusiva, calcolando lo sha mentre scrive, poi Sync e confronto.
func scriviParziale(src, parte, sha string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "sorgente_illeggibile", err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(parte), 0o755); err != nil {
		return causaDi(err), err
	}
	out, err := os.OpenFile(parte, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return causaDi(err), err
	}
	h := sha256.New()
	_, errCopia := io.Copy(io.MultiWriter(out, h), in)
	errSync := out.Sync()
	errChiudi := out.Close()
	switch {
	case errCopia != nil:
		var pe *os.PathError
		if errors.As(errCopia, &pe) && pe.Path == src {
			return "sorgente_illeggibile", errCopia
		}
		return causaDi(errCopia), errCopia
	case errSync != nil:
		return causaDi(errSync), errSync
	case errChiudi != nil:
		return causaDi(errChiudi), errChiudi
	}
	if hex.EncodeToString(h.Sum(nil)) != sha {
		return "verifica_sha", errors.New("lo sha256 dei byte copiati non e' quello del file")
	}
	return "", nil
}

// rinomina: il parziale diventa il definitivo della riga con la riga presa FOR UPDATE e ancora in_copia con la propria
// chiave, e la verifica dello sha del definitivo sta nella stessa transazione (un rilascio della stessa riga non puo'
// cadere fra le due e lasciare un file orfano). Un definitivo gia' presente con lo stesso sha si tiene; con uno sha
// diverso: conflitto, niente sovrascritto.
func (s *Servizio) rinomina(ctx context.Context, r rigaCopia, parte, sha, tipo string) (string, error) {
	causa := ""
	err := s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetConservazionePerAggiornare(ctx, r.conservazioneID)
		if err != nil {
			return err
		}
		if c.Stato != "in_copia" {
			causa = "tentativo_chiuso"
			return errors.New("la copia non e' piu' in corso")
		}
		if h, err := shaDelFile(r.percorso); err == nil {
			if h != sha {
				causa = "conflitto"
				return errors.New("definitivo gia' presente con un altro contenuto")
			}
			_ = togliFile(parte)
		} else if err := os.Rename(parte, r.percorso); err != nil {
			causa = causaDi(err)
			return err
		}
		if h, err := shaDelFile(r.percorso); err != nil || h != sha {
			causa = "verifica_sha"
			return errors.New("il definitivo non ha lo sha256 del file")
		}
		_, err = q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "in_copia", Tentativi: c.Tentativi,
			SorgenteByte: testo(tipo), ConservazioneID: c.ConservazioneID, StatoAtteso: "in_copia"})
		return err
	})
	if err != nil && causa == "" {
		causa = "sorgente_illeggibile"
	}
	return causa, err
}

// confermaDefinitivo: una ripresa che trova il definitivo gia' giusto segna la sorgente (se manca) e va avanti.
func (s *Servizio) confermaDefinitivo(ctx context.Context, r rigaCopia, tipo string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetConservazionePerAggiornare(ctx, r.conservazioneID)
		if err != nil {
			return err
		}
		if c.Stato != "in_copia" {
			return errors.New("la copia non e' piu' in corso")
		}
		if c.SorgenteByte.Valid {
			return nil
		}
		if tipo == "" {
			tipo = "conservazione"
		}
		_, err = q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "in_copia", Tentativi: c.Tentativi,
			SorgenteByte: testo(tipo), ConservazioneID: c.ConservazioneID, StatoAtteso: "in_copia"})
		return err
	})
}

// battito: ogni minuto, finche' la copia del gesto e' in corso, aggiornato_il delle righe si rinfresca (in_copia →
// in_copia) in una transazione breve con la riga FOR UPDATE; se una riga non e' piu' in_copia, il battito la lascia.
func (s *Servizio) battito(ctx context.Context, g gesto, righe []rigaCopia) func() {
	fine := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(battitoOgni)
		defer t.Stop()
		for {
			select {
			case <-fine:
				return
			case <-t.C:
				for _, r := range righe {
					_ = s.inTx(context.WithoutCancel(ctx), func(q *db.Queries) error {
						c, err := q.GetConservazionePerAggiornare(ctx, r.conservazioneID)
						if err != nil || c.Stato != "in_copia" {
							return err
						}
						_, err = q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "in_copia", Tentativi: c.Tentativi,
							ConservazioneID: c.ConservazioneID, StatoAtteso: "in_copia"})
						return err
					})
				}
			}
		}
	}()
	return func() { close(fine); wg.Wait() }
}

// faseC: la propria riga ancora in_copia e con la propria chiave (altrimenti 409 COPIA_IN_CORSO, tentativo_chiuso, e
// la fase C non tocca la conservazione); occorrenza, versione e dipendenze ricontrollate (altrimenti il 409 o il 422);
// poi copiata, lo stato altri_file, l'evento, lo scollegamento, l'intento. Dopo un rifiuto, chi chiama porta le righe
// del gesto ancora in copia con la propria chiave in rilascio, causa rifiutata (rifiutaRighe).
func (s *Servizio) faseC(ctx context.Context, q *db.Queries, g gesto, righe []rigaCopia) (*EsitoGesto, error) {
	// l'ordine della fase C (EM §10.5): sotto BloccaThread prima l'intento (un reinvio arrivato nel frattempo e' gia'
	// registrato), poi la propria riga, poi i controlli della fase A ripetuti da prepara. La riga viene prima
	// dell'occorrenza: un tentativo chiuso da uno «Scarta» o da un'altra scheda risponde 409 tentativo_chiuso, non il
	// rifiuto dello stato nuovo (PM23 SC-20, CO-10), e non tocca la conservazione.
	if _, err := q.BloccaThread(ctx, g.thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, rfqNonTrovata()
		}
		return nil, err
	}
	if reg, err := cercaIntento(ctx, q, g.thread, g.it); err != nil || reg != nil {
		return reg, err
	}
	for _, r := range righe {
		c, err := q.GetConservazionePerAggiornare(ctx, r.conservazioneID)
		if err != nil {
			return nil, err
		}
		if c.Stato != "in_copia" || !c.ChiaveIdempotenza.Valid || c.ChiaveIdempotenza.UUID != g.it.chiave || !c.SorgenteByte.Valid {
			return nil, rifiuto(http.StatusConflict, "COPIA_IN_CORSO",
				"Questo tentativo di copia è stato chiuso (un'altra scheda l'ha ripreso, o il file è stato scartato): rileggi la Sorgente.",
				extra("motivo", "tentativo_chiuso", "allegato_id", r.occ.AllegatoID))
		}
	}
	pr, reg, err := s.prepara(ctx, q, g)
	if err != nil {
		return nil, err
	}
	if reg != nil {
		return reg, nil
	}
	// le occorrenze da mettere in Altri file sono ancora quelle della fase A
	daMettere := map[uuid.UUID]bool{}
	for _, r := range righe {
		daMettere[r.occ.AllegatoID] = true
	}
	for _, o := range pr.tutte {
		if o.Stato == StatoAttivo && !o.ScartatoLegacy && !daMettere[o.AllegatoID] {
			return nil, perimetroCambiato(pr.per)
		}
	}
	for _, r := range righe {
		c, err := q.GetConservazionePerAggiornare(ctx, r.conservazioneID)
		if err != nil {
			return nil, err
		}
		if _, err := q.UpdateConservazione(ctx, db.UpdateConservazioneParams{Stato: "copiata", Tentativi: c.Tentativi,
			Bytes: r.occ.Bytes, ConservazioneID: c.ConservazioneID, StatoAtteso: "in_copia"}); err != nil {
			return nil, err
		}
		if _, err := scriviEvento(ctx, q, g.thread, evento{o: r.occ, azione: "conservazione_copiata",
			esito:   map[string]any{"stato": "copiata", "conservazione_id": c.ConservazioneID, "sorgente_byte": c.SorgenteByte.String},
			origine: OrigineServer}); err != nil {
			return nil, err
		}
	}
	ids := idDi(pr.tutte)
	if _, err := q.BloccaPropostaDiAllegato(ctx, ids); err != nil {
		return nil, err
	}
	for _, o := range pr.tutte {
		if daMettere[o.AllegatoID] && g.corpo.scollega() && haDipendenza(pr.el, o.AllegatoID, "D-7") {
			if err := scollega(ctx, q, g.thread, o, g); err != nil {
				return nil, err
			}
		}
	}
	if _, err := q.BloccaAllegatiPerFotografia(ctx, ids); err != nil {
		return nil, err
	}
	fresche, err := leggiOccorrenze(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	esito := &EsitoGesto{Esito: "in_altri_file", ChiaveIdempotenza: g.it.chiave, AllegatoID: g.allegato, Stato: StatoAltriFile,
		Occorrenze: ids}
	for _, o := range fresche {
		if !daMettere[o.AllegatoID] {
			continue
		}
		per := (*Perimetro)(nil)
		if o.AllegatoID == g.allegato {
			per = &pr.per
		}
		ver, err := scriviStato(ctx, q, o, StatoAltriFile, g.utente)
		if err != nil {
			return nil, err
		}
		if o.AllegatoID == g.allegato {
			esito.Versione = ver
		}
		if _, err := scriviEvento(ctx, q, g.thread, evento{o: o, azione: "altri_file", prima: StatoAttivo, dopo: StatoAltriFile,
			motivo: g.corpo.motivo(), fotografia: fotografia(o), dipendenze: dipendenzeDellOccorrenza(pr.el, o.AllegatoID, per),
			origine: OriginePonte, autore: uid(g.utente), chiave: uid(g.it.chiave)}); err != nil {
			return nil, err
		}
	}
	_, cons, err := statiCorrenti(ctx, q, g.thread, ids)
	if err != nil {
		return nil, err
	}
	esito.Conservazioni = cons
	esito.Testo = "File messo in Altri file: il contenuto è conservato."
	if err := registraIntento(ctx, q, g.thread, g.it, esito); err != nil {
		return nil, err
	}
	return esito, nil
}

// rifiutaRighe: dopo una fase C rifiutata (versione, dipendenze, perimetro, tentativo chiuso), le proprie righe ancora
// in_copia con la propria chiave vanno in rilascio (causa rifiutata) e si tenta il loro rilascio. Una riga che non e'
// piu' in copia con questa chiave (chiusa da uno «Scarta», ripresa da un'altra scheda) non si tocca: la rilascia chi
// l'ha chiusa, o il custode.
func (s *Servizio) rifiutaRighe(ctx context.Context, g gesto, righe []rigaCopia) {
	c2 := context.WithoutCancel(ctx)
	for _, r := range righe {
		mossa := false
		_ = s.inTx(c2, func(q *db.Queries) error {
			c, err := q.GetConservazionePerAggiornare(c2, r.conservazioneID)
			if err != nil || c.Stato != "in_copia" || !c.ChiaveIdempotenza.Valid || c.ChiaveIdempotenza.UUID != g.it.chiave {
				return err
			}
			if err := s.passoConservazione(c2, q, c, "da_rilasciare", testo("rifiutata"), pgtype.Text{}); err != nil {
				return err
			}
			mossa = true
			return nil
		})
		if !mossa {
			continue
		}
		if _, err := s.RilasciaCopia(c2, r.conservazioneID, false); err != nil {
			s.log().Warn("ciclo dei file: rilascio dopo una fase C rifiutata", "conservazione", r.conservazioneID, "err", err)
		}
	}
}

// causaDi: la causa di un errore di scrittura nel deposito.
func causaDi(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "permessi"
	case strings.Contains(strings.ToLower(err.Error()), "space") || strings.Contains(strings.ToLower(err.Error()), "spazio"):
		return "spazio"
	}
	return "permessi"
}

// shaDelFile: lo sha256 di un file (errore se non c'e').
func shaDelFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
