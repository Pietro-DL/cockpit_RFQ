package distintabozza

// R6 (EM §9.6): l'esito di un intento con chiave, senza duplicare niente e senza scrivere. Prima di leggere prende la riga
// della RFQ FOR KEY SHARE con un tempo massimo di 3 secondi, in una transazione che non scrive: cosi' aspetta una R5 (o un
// gesto dei file) ancora in corso, che tiene la riga FOR UPDATE fino al COMMIT; se il lucchetto non arriva, l'esito e'
// in_corso e la pagina richiede R6 dopo qualche secondo.
//
// Gli stati: registrato (l'esito scritto nella transazione del gesto), registrato_senza_esito (l'intento e' oltre i 90
// giorni, ma l'atto c'e': la conferma con quella chiave, l'evento del file o dell'associazione con quella chiave e quella
// RFQ), non_registrato (si ritenta con la stessa chiave e lo stesso corpo), in_corso. Una chiave usata in un'altra RFQ e'
// 422 FUORI_RFQ, mai l'esito di quella RFQ.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// Gli stati di R6.
const (
	IntentoRegistrato           = "registrato"
	IntentoNonRegistrato        = "non_registrato"
	IntentoRegistratoSenzaEsito = "registrato_senza_esito"
	IntentoInCorso              = "in_corso"
)

// I gesti di esito_intento (EM §8.4): i tredici della 0023, con conferma_sorgente (S8) e rifiuta_proposta (R15) del delta
// dell'ondata C (U2 §3.14 riga 6) e revoca_sorgente (direttiva 39, dalla variante 20261019c; vigente la 20261019d).
var GestiIntento = []string{"conferma_bom_v2", "conferma_bom_v3", "lotto_fascicolo", "correggi_documento", "fonte_step",
	"riprova_copia", "scarta_file", "altri_file", "riattiva_file", "associa_step", "conferma_sorgente", "rifiuta_proposta",
	"revoca_sorgente"}

var gestiDeiFile = map[string]bool{"scarta_file": true, "altri_file": true, "riattiva_file": true}

// AttesaLucchettoR6: quanto R6 aspetta la riga della RFQ prima di dire in_corso (EM §9.6).
const AttesaLucchettoR6 = 3 * time.Second

// StatiCorrenti: gli stati di un'occorrenza letti adesso, per i gesti dei file (EM §9.6, A2 §6.0).
type StatiCorrenti struct {
	AllegatoID          uuid.UUID `json:"allegato_id"`
	Stato               string    `json:"stato"`
	Versione            int       `json:"versione"`
	Rimozione           string    `json:"rimozione,omitempty"`
	MotivoRimozione     string    `json:"motivo_rimozione,omitempty"`
	RimozioneRipetibile bool      `json:"rimozione_ritentabile,omitempty"`
	Conservazione       string    `json:"conservazione,omitempty"`
}

// EsitoR6 e' la risposta di R6.
type EsitoR6 struct {
	Stato          string          `json:"stato"`
	Gesto          string          `json:"gesto,omitempty"`
	Formato        int             `json:"formato,omitempty"`
	CreatoIl       *time.Time      `json:"creato_il,omitempty"`
	Esito          json.RawMessage `json:"esito,omitempty"`
	F8Stato        string          `json:"f8_stato,omitempty"`
	StatiCorrenti  *StatiCorrenti  `json:"stati_correnti,omitempty"`
	ConfermaID     *uuid.UUID      `json:"conferma_id,omitempty"`
	NumeroConferma int             `json:"numero_conferma,omitempty"`
}

// LeggiIntento e' R6 (commento di testa). La transazione la apre qui, e la chiude con ROLLBACK: non scrive niente.
func LeggiIntento(ctx context.Context, pool Avviatore, thread, chiave uuid.UUID) (EsitoR6, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return EsitoR6{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sola lettura: si chiude sempre con ROLLBACK
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '3s'"); err != nil {
		return EsitoR6{}, err
	}
	q := db.New(tx)
	if _, err := q.BloccaThreadCondiviso(ctx, thread); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return EsitoR6{}, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		case sqlstate(err) == "55P03":
			return EsitoR6{Stato: IntentoInCorso}, nil
		}
		return EsitoR6{}, err
	}
	rfq, err := q.ListRfqDellaChiave(ctx, chiave)
	if err != nil {
		return EsitoR6{}, err
	}
	diQuesta := false
	for _, t := range rfq {
		if t == thread {
			diQuesta = true
		}
	}
	if len(rfq) > 0 && !diQuesta {
		return EsitoR6{}, fuori("chiave", "la chiave %s e' di un'altra RFQ", chiave)
	}
	ei, err := q.GetEsitoIntento(ctx, db.GetEsitoIntentoParams{Chiave: chiave, ThreadID: thread})
	switch {
	case err == nil:
		creato := ei.CreatoIl
		out := EsitoR6{Stato: IntentoRegistrato, Gesto: ei.Gesto, CreatoIl: &creato, Esito: canonico(ei.Esito)}
		switch ei.Gesto {
		case "conferma_bom_v2":
			out.Formato = 2
		case "conferma_bom_v3":
			out.Formato = 3
		}
		if ei.ConfermaID.Valid {
			id := ei.ConfermaID.UUID
			out.ConfermaID = &id
		}
		if f, err := q.GetF8StatoPerChiave(ctx, db.GetF8StatoPerChiaveParams{ThreadID: thread, ChiaveIntento: uuid.NullUUID{UUID: chiave, Valid: true}}); err == nil {
			out.F8Stato = f.Stato
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return EsitoR6{}, err
		}
		if gestiDeiFile[ei.Gesto] {
			if out.StatiCorrenti, err = statiCorrentiDellaChiave(ctx, q, thread, chiave); err != nil {
				return EsitoR6{}, err
			}
		}
		return out, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return EsitoR6{}, err
	}
	// oltre la conservazione degli esiti: l'atto si trova da se'
	if c, err := q.GetConfermaPerChiave(ctx, db.GetConfermaPerChiaveParams{ChiaveIdempotenza: uuid.NullUUID{UUID: chiave, Valid: true}, ThreadID: thread}); err == nil {
		id := c.ConfermaID
		out := EsitoR6{Stato: IntentoRegistratoSenzaEsito, ConfermaID: &id, NumeroConferma: int(c.Numero), Formato: int(c.Formato)}
		switch c.Formato {
		case 2:
			out.Gesto = "conferma_bom_v2"
		case 3:
			out.Gesto = "conferma_bom_v3"
		}
		if f, err := q.GetF8StatoPerConferma(ctx, db.GetF8StatoPerConfermaParams{ThreadID: thread, ConfermaID: uuid.NullUUID{UUID: id, Valid: true}}); err == nil {
			out.F8Stato = f.Stato
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return EsitoR6{}, err
		}
		return out, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return EsitoR6{}, err
	}
	sc, err := statiCorrentiDellaChiave(ctx, q, thread, chiave)
	if err != nil {
		return EsitoR6{}, err
	}
	if sc != nil {
		return EsitoR6{Stato: IntentoRegistratoSenzaEsito, StatiCorrenti: sc}, nil
	}
	ae, err := q.GetAssociazioneEventoPerChiave(ctx, db.GetAssociazioneEventoPerChiaveParams{ChiaveIdempotenza: uuid.NullUUID{UUID: chiave, Valid: true}, ThreadID: thread})
	if err != nil {
		return EsitoR6{}, err
	}
	if len(ae) > 0 {
		return EsitoR6{Stato: IntentoRegistratoSenzaEsito, Gesto: "associa_step"}, nil
	}
	return EsitoR6{Stato: IntentoNonRegistrato}, nil
}

// statiCorrentiDellaChiave: l'occorrenza dell'evento dei file scritto con quella chiave nella RFQ, con gli stati letti
// adesso; nil se nessun evento dei file ha quella chiave.
func statiCorrentiDellaChiave(ctx context.Context, q *db.Queries, thread, chiave uuid.UUID) (*StatiCorrenti, error) {
	ee, err := q.GetFileEventoPerChiave(ctx, db.GetFileEventoPerChiaveParams{ChiaveIdempotenza: uuid.NullUUID{UUID: chiave, Valid: true}, ThreadID: thread})
	if err != nil {
		return nil, err
	}
	if len(ee) == 0 {
		return nil, nil
	}
	a := ee[0].AllegatoID
	s, err := q.GetStatiCorrentiFile(ctx, db.GetStatiCorrentiFileParams{ThreadID: thread, AllegatoID: a})
	if err != nil {
		return nil, err
	}
	return &StatiCorrenti{AllegatoID: a, Stato: s.Stato, Versione: int(s.Versione), Rimozione: s.RimozioneStato,
		MotivoRimozione: s.RimozioneMotivo, RimozioneRipetibile: s.RimozioneRitentabile, Conservazione: s.ConservazioneStato}, nil
}
