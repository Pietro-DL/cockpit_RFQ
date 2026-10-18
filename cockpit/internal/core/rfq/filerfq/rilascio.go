package filerfq

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// RilasciaCopia e' un passo del rilascio di una copia di conservazione da_rilasciare (EM §10.5, «Il rilascio di una
// copia»), in una transazione: la riga FOR UPDATE (il custode SKIP LOCKED), BloccaContenuto, poi — se l'occorrenza non
// e' scartata — il controllo che i byte restino raggiungibili altrove, verificato qui; il file della riga e i suoi
// parziali tolti dal deposito; la riga rilasciata con il suo evento. Una copia non si toglie mai supponendo che l'altra
// ci sia: se i byte non sono raggiungibili altrove la riga resta da_rilasciare, con l'errore, e si ritenta.
func (s *Servizio) RilasciaCopia(ctx context.Context, conservazioneID int64, custode bool) (EsitoPasso, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return EsitoPasso{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	var c db.FileRfqConservazione
	if custode {
		c, err = q.GetConservazioneSeLibera(ctx, conservazioneID)
	} else {
		c, err = q.GetConservazionePerAggiornare(ctx, conservazioneID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return EsitoPasso{Saltato: true}, nil
	}
	if err != nil {
		return EsitoPasso{}, err
	}
	if c.Stato != "da_rilasciare" {
		return EsitoPasso{Saltato: true, Stato: c.Stato}, nil
	}
	sha := strings.ToLower(strings.TrimSpace(c.Sha256))
	if err := q.BloccaContenuto(ctx, sha); err != nil {
		return EsitoPasso{}, err
	}
	// la copia di un altro deposito (un tentativo in corso quando la radice e' cambiata): niente si tocca sul disco di
	// questa radice, e la riga si chiude (non era una copia verificata: nessun file in Altri file ne dipende)
	estraneo := s.Deposito == nil || c.DepositoID != s.Deposito.ID
	if s.Deposito == nil {
		return s.rimandaRilascio(ctx, tx, q, c, "deposito non configurato: la copia resta finché il deposito non torna")
	}
	occ, err := q.StatoOccorrenzaAllegato(ctx, c.AllegatoID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return EsitoPasso{}, err
	}
	scartata := err == nil && occ.ThreadID.Valid && occ.ThreadID.UUID == c.ThreadID && (occ.Stato == StatoScartato || occ.ScartatoLegacy)
	if !scartata && !estraneo {
		ok, err := s.byteRaggiungibiliAltrove(ctx, q, c, sha)
		if err != nil {
			return EsitoPasso{}, err
		}
		if !ok {
			return s.rimandaRilascio(ctx, tx, q, c, "byte non raggiungibili altrove")
		}
	}
	if !estraneo {
		p, err := s.Deposito.PercorsoCopia(c.PercorsoRelativo)
		if err != nil {
			// un percorso che esce dal deposito non si cancella: la riga resta da rilasciare, con l'errore (un difetto)
			s.log().Error("ciclo dei file: percorso di conservazione non valido", "conservazione", c.ConservazioneID, "err", err)
			return s.rimandaRilascio(ctx, tx, q, c, "percorso della copia non valido")
		}
		if err := s.passoConservazione(ctx, q, c, "rilasciata", c.Causa, pgtype.Text{}); err != nil {
			return EsitoPasso{}, err
		}
		if err := togliFile(p); err != nil {
			_ = tx.Rollback(ctx)
			return s.erroreRilascio(ctx, c.ConservazioneID, err)
		}
		for _, parte := range s.Deposito.partiDellaRiga(c.ConservazioneID) {
			if err := togliFile(parte); err != nil {
				_ = tx.Rollback(ctx)
				return s.erroreRilascio(ctx, c.ConservazioneID, err)
			}
		}
	} else if err := s.passoConservazione(ctx, q, c, "rilasciata", c.Causa, pgtype.Text{}); err != nil {
		return EsitoPasso{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "rilasciata"}, nil
}

// byteRaggiungibiliAltrove: per un'occorrenza non scartata (Riattiva, fase C rifiutata, ripresa) la copia si rilascia
// solo se i byte restano raggiungibili, verificati in questa transazione (EM §10.5 passo 3): l'occorrenza ha un
// path_staging che EContenuto riconosce, il file c'e' con lo sha giusto (ContenutoGiaPresente) e il contenuto ha il pin
// «proposta aperta» di un'occorrenza attiva; oppure c'e' un documento scritto sul NAS con quello sha; oppure un'altra
// copia copiata dello stesso allegato.
func (s *Servizio) byteRaggiungibiliAltrove(ctx context.Context, q *db.Queries, c db.FileRfqConservazione, sha string) (bool, error) {
	a, err := q.GetAllegatoConRfq(ctx, c.AllegatoID)
	if err != nil {
		return false, err
	}
	if a.PathStaging.Valid && s.Staging != "" && staging.EContenuto(s.Staging, a.PathStaging.String, sha) &&
		staging.ContenutoGiaPresente(a.PathStaging.String, sha) {
		pin, err := q.ListContenutiPinnati(ctx, []string{sha})
		if err != nil {
			return false, err
		}
		for _, p := range pin {
			if p.Motivo == "proposta ancora aperta" {
				return true, nil
			}
		}
	}
	scritto, err := q.EsisteDocumentoScrittoSha(ctx, db.EsisteDocumentoScrittoShaParams{ThreadID: c.ThreadID, Sha256: sha})
	if err != nil {
		return false, err
	}
	if scritto {
		return true, nil
	}
	copie, err := q.ListCopieConservazioneAllegato(ctx, c.AllegatoID)
	if err != nil {
		return false, err
	}
	for _, x := range copie {
		if x.ConservazioneID != c.ConservazioneID && x.Stato == "copiata" {
			return true, nil
		}
	}
	return false, nil
}

// rimandaRilascio: la riga resta da_rilasciare con l'errore (in parole, senza percorsi), tentativi + 1 e il prossimo
// tentativo; COMMIT.
func (s *Servizio) rimandaRilascio(ctx context.Context, tx pgx.Tx, q *db.Queries, c db.FileRfqConservazione, perche string) (EsitoPasso, error) {
	if err := s.passoConservazione(ctx, q, c, "da_rilasciare", c.Causa, pgtype.Text{String: perche, Valid: true}); err != nil {
		return EsitoPasso{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "da_rilasciare", Motivo: perche}, nil
}

// passoConservazione scrive un passo di una copia dallo stato atteso e, per gli stati che hanno un evento (rilasciata,
// errore_copia, copiata), l'evento del server.
func (s *Servizio) passoConservazione(ctx context.Context, q *db.Queries, c db.FileRfqConservazione, stato string, causa,
	errore pgtype.Text) error {
	p := db.UpdateConservazioneParams{Stato: stato, Causa: causa, Errore: errore, Tentativi: c.Tentativi,
		ConservazioneID: c.ConservazioneID, StatoAtteso: c.Stato}
	if stato == "da_rilasciare" && c.Stato == "da_rilasciare" {
		p.Tentativi = c.Tentativi + 1
		p.ProssimoTentativoIl = s.traDopo(attesaDopo(p.Tentativi))
	}
	if stato == "in_copia" || stato == "copiata" {
		p.Causa = pgtype.Text{}
	}
	n, err := q.UpdateConservazione(ctx, p)
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("copia di conservazione: lo stato e' cambiato durante il passo")
	}
	azione := ""
	switch stato {
	case "rilasciata":
		azione = "conservazione_rilasciata"
	case "errore_copia":
		azione = "conservazione_errore"
	default:
		return nil
	}
	esito := map[string]any{"stato": stato, "conservazione_id": c.ConservazioneID}
	if causa.Valid {
		esito["causa"] = causa.String
	}
	_, err = scriviEvento(ctx, q, c.ThreadID, evento{o: Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: c.AllegatoID}},
		azione: azione, esito: esito, origine: OrigineServer})
	return err
}

// erroreRilascio: os.Remove non e' riuscito (il file aperto dall'antivirus o dal visore): in una transazione nuova la
// riga resta da_rilasciare, tentativi + 1, prossimo tentativo, errore in parole.
func (s *Servizio) erroreRilascio(ctx context.Context, conservazioneID int64, causa error) (EsitoPasso, error) {
	s.log().Warn("ciclo dei file: il rilascio di una copia di conservazione non e' riuscito", "conservazione", conservazioneID, "err", causa)
	perche := "errore di scrittura nel deposito"
	if fileOccupato(causa) {
		perche = "il file è aperto da un altro programma"
	}
	err := s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetConservazionePerAggiornare(ctx, conservazioneID)
		if err != nil {
			return err
		}
		if c.Stato != "da_rilasciare" {
			return nil
		}
		return s.passoConservazione(ctx, q, c, "da_rilasciare", c.Causa, pgtype.Text{String: perche, Valid: true})
	})
	if err != nil {
		return EsitoPasso{}, err
	}
	return EsitoPasso{Stato: "da_rilasciare", Motivo: perche}, nil
}

// togliFile: os.Remove, con un file assente che vale come fatto.
func togliFile(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// fileOccupato: l'errore di Windows di un file aperto da un altro programma (ERROR_SHARING_VIOLATION,
// ERROR_LOCK_VIOLATION).
func fileOccupato(err error) bool {
	var e syscall.Errno
	if errors.As(err, &e) {
		return e == 32 || e == 33
	}
	return false
}
