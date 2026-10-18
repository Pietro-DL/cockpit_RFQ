package filerfq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// EsitoPasso dice che cosa ha fatto un passo del tempo 2 o del rilascio (per il log del custode e per le prove).
type EsitoPasso struct {
	Saltato bool   // la riga era in mano a un altro (SKIP LOCKED) o gia' in uno stato finale
	Stato   string // lo stato dopo il passo
	Motivo  string
}

// EseguiRimozione e' un passo del tempo 2 dello scarto per una riga di file_rfq_rimozione (EM §10.4, «Tempo 2, la
// rimozione»): sempre lo stesso codice, dopo il gesto, dal custode e al reinvio. I passi 1-10 stanno in una sola
// transazione, che tiene la riga e BloccaContenuto fino al COMMIT, e os.Remove avviene prima del COMMIT: nessun
// controllo invecchia fra la verifica e la cancellazione (giro 1, SIC-04). custode: la riga si prende SKIP LOCKED.
func (s *Servizio) EseguiRimozione(ctx context.Context, rimozioneID int64, custode bool) (EsitoPasso, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return EsitoPasso{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	// passo 1: la riga; uno stato finale o un errore non ritentabile si fermano qui (il secondo esce solo con la bonifica)
	var r db.FileRfqRimozione
	if custode {
		r, err = q.GetRimozioneSeLibera(ctx, rimozioneID)
	} else {
		r, err = q.GetRimozionePerAggiornare(ctx, rimozioneID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return EsitoPasso{Saltato: true}, nil
	}
	if err != nil {
		return EsitoPasso{}, err
	}
	if r.Stato == "eseguita" || r.Stato == "non_necessaria_condiviso" || (r.Stato == "errore" && !r.Ritentabile) {
		return EsitoPasso{Saltato: true, Stato: r.Stato, Motivo: r.Motivo}, nil
	}
	sha := ""
	if r.Sha256.Valid {
		sha = strings.ToLower(strings.TrimSpace(r.Sha256.String))
	}
	// passo 2: il lucchetto per contenuto (senza sha, niente lucchetto)
	if sha != "" {
		if err := q.BloccaContenuto(ctx, sha); err != nil {
			return EsitoPasso{}, err
		}
	}
	chiudi := func(stato, motivo string, esito map[string]any) (EsitoPasso, error) {
		if err := s.passoRimozione(ctx, q, r, stato, motivo, pgtype.Text{}, pgtype.Text{}); err != nil {
			return EsitoPasso{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return EsitoPasso{}, err
		}
		return EsitoPasso{Stato: stato, Motivo: motivo}, nil
	}
	// passo 3: l'allegato e la sua RFQ, riletti
	a, err := q.GetAllegatoConRfq(ctx, r.AllegatoID)
	if err != nil {
		return EsitoPasso{}, err
	}
	if !a.ThreadID.Valid || a.ThreadID.UUID != r.ThreadID {
		return chiudi("non_necessaria_condiviso", "rfq_cambiata", nil)
	}
	if !a.PathStaging.Valid || strings.TrimSpace(a.PathStaging.String) == "" {
		return chiudi("eseguita", "gia_assente", nil)
	}
	percorso := filepath.Clean(a.PathStaging.String)
	attendi := func(motivo string) (EsitoPasso, error) { return chiudi("in_attesa", motivo, nil) }
	if strings.TrimSpace(s.Staging) == "" {
		// senza la radice dello staging nessun file si riconosce: il tempo 2 aspetta, non sbaglia
		return attendi("da_eseguire")
	}
	// passo 4: il file dell'occorrenza e' il suo path_staging solo se e' il posto del contenuto con quello sha
	if !staging.EContenuto(s.Staging, percorso, sha) {
		condiviso, err := s.indicatoDaAltri(ctx, q, r, sha, percorso)
		if err != nil {
			return EsitoPasso{}, err
		}
		if _, err := q.AzzeraPathStagingAllegato(ctx, r.AllegatoID); err != nil {
			return EsitoPasso{}, err
		}
		trovato := pgtype.Text{String: troncaPercorso(a.PathStaging.String), Valid: true}
		if condiviso {
			if err := s.passoRimozione(ctx, q, r, "non_necessaria_condiviso", "condiviso", pgtype.Text{}, trovato); err != nil {
				return EsitoPasso{}, err
			}
			if err := tx.Commit(ctx); err != nil {
				return EsitoPasso{}, err
			}
			return EsitoPasso{Stato: "non_necessaria_condiviso", Motivo: "condiviso"}, nil
		}
		motivo := "percorso_non_valido"
		if sottoRadice(s.Staging, percorso) {
			motivo = "fuori_cache"
		}
		if err := s.passoRimozione(ctx, q, r, "errore", motivo, pgtype.Text{String: "il percorso non è nella cache dello staging", Valid: true}, trovato); err != nil {
			return EsitoPasso{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return EsitoPasso{}, err
		}
		return EsitoPasso{Stato: "errore", Motivo: motivo}, nil
	}
	// passo 5: lavori dell'occorrenza ancora pendenti
	n, err := q.ContaLavoriPendentiOccorrenza(ctx, r.AllegatoID.String())
	if err != nil {
		return EsitoPasso{}, err
	}
	if n > 0 {
		return attendi("lavoro_in_corso")
	}
	// passo 6: copia sul NAS in attesa o anomalia NAS aperta per lo stesso sha nella RFQ
	nas, err := q.CopiaNasOAnomaliaPerSha(ctx, db.CopiaNasOAnomaliaPerShaParams{ThreadID: r.ThreadID, Sha256: sha})
	if err != nil {
		return EsitoPasso{}, err
	}
	if nas.CopiaInAttesa {
		return attendi("copia_nas_in_attesa")
	}
	if nas.AnomaliaAperta {
		return attendi("anomalia_nas")
	}
	// passo 7: altri allegati che indicano lo stesso file, con un'occorrenza non scartata → si toglie solo il riferimento
	condiviso, err := s.indicatoDaAltri(ctx, q, r, sha, percorso)
	if err != nil {
		return EsitoPasso{}, err
	}
	if condiviso {
		if _, err := q.AzzeraPathStagingAllegato(ctx, r.AllegatoID); err != nil {
			return EsitoPasso{}, err
		}
		return chiudi("non_necessaria_condiviso", "condiviso", nil)
	}
	// passo 8: altri pin del file (non e' uno stato finale: si ritenta)
	pin, err := q.PinDelFile(ctx, db.PinDelFileParams{Sha256: sha, Escluso: r.AllegatoID, Percorso: percorso})
	if err != nil {
		return EsitoPasso{}, err
	}
	if pin != "" {
		return attendi("pin_transitorio")
	}
	// passo 9: un file piu' giovane della EtaMinima della cache non si cancella
	st, errStat := os.Stat(percorso)
	if errStat == nil && s.adesso().Sub(st.ModTime()) < s.etaMinima() {
		return attendi("file_giovane")
	}
	if s.Agganci != nil && s.Agganci.PrimaDiCancellare != nil {
		s.Agganci.PrimaDiCancellare(rimozioneID)
	}
	// passo 10: path_staging = NULL per chi indica QUEL file, l'esito, os.Remove, COMMIT
	if _, err := q.AzzeraPathStagingPerFile(ctx, db.AzzeraPathStagingPerFileParams{Sha256: sha, Percorso: percorso}); err != nil {
		return EsitoPasso{}, err
	}
	if errors.Is(errStat, os.ErrNotExist) {
		return chiudi("eseguita", "gia_assente", nil)
	}
	if err := s.passoRimozione(ctx, q, r, "eseguita", "rimosso", pgtype.Text{}, pgtype.Text{}); err != nil {
		return EsitoPasso{}, err
	}
	if err := os.Remove(percorso); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = tx.Rollback(ctx)
		return s.erroreRimozione(ctx, r.RimozioneID, err)
	}
	if s.Agganci != nil && s.Agganci.DopoLaCancellazione != nil {
		s.Agganci.DopoLaCancellazione(rimozioneID)
	}
	if err := tx.Commit(ctx); err != nil {
		// il file e' gia' tolto: il tentativo dopo lo trova assente e chiude eseguita / gia_assente
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "eseguita", Motivo: "rimosso"}, nil
}

// passoRimozione scrive un passo della rimozione dallo stato atteso (confronta e scambia) e, per gli stati che hanno un
// evento (eseguita, non_necessaria_condiviso, errore), l'evento del server con l'esito: motivo, tentativo, errore in
// parole (mai un percorso).
func (s *Servizio) passoRimozione(ctx context.Context, q *db.Queries, r db.FileRfqRimozione, stato, motivo string,
	errore pgtype.Text, trovato pgtype.Text) error {
	tentativi := r.Tentativi + 1
	p := db.UpdateRimozioneParams{Stato: stato, Motivo: motivo, Ritentabile: motivo != "fuori_cache" && motivo != "percorso_non_valido",
		Errore: errore, Tentativi: tentativi, PercorsoTrovato: trovato, RimozioneID: r.RimozioneID, StatoAtteso: r.Stato}
	if stato == "in_attesa" || (stato == "errore" && p.Ritentabile) {
		p.ProssimoTentativoIl = s.traDopo(attesaDopo(tentativi))
	}
	if stato == "errore" && !p.Errore.Valid {
		p.Errore = pgtype.Text{String: motivo, Valid: true}
	}
	n, err := q.UpdateRimozione(ctx, p)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("rimozione %d: lo stato e' cambiato durante il passo", r.RimozioneID)
	}
	azione := ""
	switch stato {
	case "eseguita":
		azione = "rimozione_eseguita"
	case "non_necessaria_condiviso":
		azione = "rimozione_non_necessaria"
	case "errore":
		azione = "rimozione_errore"
	default:
		return nil
	}
	esito := map[string]any{"stato": stato, "motivo": motivo, "tentativo": tentativi}
	if stato == "errore" {
		esito["errore"] = TestoRimozione(stato, motivo)
	}
	_, err = scriviEvento(ctx, q, r.ThreadID, evento{o: Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: r.AllegatoID}},
		azione: azione, esito: esito, origine: OrigineServer})
	return err
}

// erroreRimozione: os.Remove non e' riuscito (file aperto da un altro programma, errore di I/O). La transazione del
// passo e' annullata: in una nuova la riga passa a errore (file_occupato o errore_io), tentativi + 1, prossimo
// tentativo, evento. Il dettaglio dell'errore va nel log, non nella risposta.
func (s *Servizio) erroreRimozione(ctx context.Context, rimozioneID int64, causa error) (EsitoPasso, error) {
	motivo := "errore_io"
	if fileOccupato(causa) {
		motivo = "file_occupato"
	}
	s.log().Warn("ciclo dei file: la rimozione dallo staging non e' riuscita", "rimozione", rimozioneID, "motivo", motivo, "err", causa)
	err := s.inTx(ctx, func(q *db.Queries) error {
		r, err := q.GetRimozionePerAggiornare(ctx, rimozioneID)
		if err != nil {
			return err
		}
		if r.Stato != "in_attesa" && !(r.Stato == "errore" && r.Ritentabile) {
			return nil
		}
		return s.passoRimozione(ctx, q, r, "errore", motivo, pgtype.Text{String: troncaErrore(causa.Error()), Valid: true}, pgtype.Text{})
	})
	if err != nil {
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "errore", Motivo: motivo}, nil
}

// indicatoDaAltri: un altro allegato, con un'occorrenza non scartata (uno scarto col gesto precedente conta come
// scartato), indica lo stesso file. I percorsi si confrontano puliti e senza maiuscole, come EContenuto (EM §10.4 passo
// 7). Lo stesso contenuto con un'altra estensione e' un altro file.
func (s *Servizio) indicatoDaAltri(ctx context.Context, q *db.Queries, r db.FileRfqRimozione, sha, percorso string) (bool, error) {
	altri, err := q.ListAltriAllegatiStessoSha(ctx, db.ListAltriAllegatiStessoShaParams{Escluso: r.AllegatoID,
		Sha256: pgtype.Text{String: sha, Valid: sha != ""}, Percorso: percorso})
	if err != nil {
		return false, err
	}
	for _, x := range altri {
		if !x.Scartata && stessoPercorso(x.PathStaging, percorso) {
			return true, nil
		}
	}
	return false, nil
}

// stessoPercorso: due percorsi puliti uguali senza badare alle maiuscole (NTFS).
func stessoPercorso(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// sottoRadice: il percorso sta sotto la radice (pulito, senza maiuscole).
func sottoRadice(radice, percorso string) bool {
	base := filepath.Clean(strings.TrimRight(strings.TrimSpace(radice), `\/`)) + string(filepath.Separator)
	p := filepath.Clean(percorso)
	return len(p) > len(base) && strings.EqualFold(p[:len(base)], base)
}

// troncaPercorso: percorso_trovato e' varchar(500).
func troncaPercorso(p string) string {
	if len(p) <= 500 {
		return p
	}
	return p[:500]
}

func troncaErrore(e string) string {
	if len(e) <= 2000 {
		return e
	}
	return e[:2000]
}
