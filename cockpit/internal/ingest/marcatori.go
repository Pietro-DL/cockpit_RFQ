package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// I MARCATORI (blocco 7B, IB2)
//
// Quando il Cockpit crea una bozza scrive sulla mail due UserProperties: `CockpitBozza` (l'id della
// bozza) e, per una richiesta a un fornitore, `CockpitRichiestaFornitore` (l'id della richiesta).
// Restano attaccate alla mail quando parte, e il sync della Posta inviata le rilegge. Qui il server
// le applica: la richiesta prende la sua mail (stato `inviata`), la mail viene agganciata alla RFQ
// cliente senza indovinare dall'oggetto, la bozza risulta partita.
//
// E' un fatto, non un'interpretazione: il legame lo ha scritto il Cockpit stesso quando l'operatore
// ha chiesto la bozza. Per questo puo' scrivere `thread_id`, cosa che l'ingest altrimenti non fa
// mai (checkpoint 3R §2): non e' una coincidenza, e' la nostra firma.

const (
	MarcatoreBozza     = "CockpitBozza"
	MarcatoreRichiesta = "CockpitRichiestaFornitore"
)

// applicaMarcatori legge i marcatori del lotto e li applica alla riga del messaggio. Restituisce
// true se il messaggio e' stato agganciato qui. Un marcatore che punta a niente (richiesta
// cancellata, bozza di un altro banco) si registra nel log e non ferma l'ingest.
func (s *Servizio) applicaMarcatori(ctx context.Context, q *db.Queries, row *db.UpsertMessaggioRow, m *api.MessaggioIn) (bool, error) {
	if len(m.Marcatori) == 0 {
		return false, nil
	}
	agganciato := false
	if v := m.Marcatori[MarcatoreRichiesta]; v != "" {
		rid, err := uuid.Parse(v)
		if err != nil {
			s.avvisa("marcatore richiesta non valido", "messaggio", m.MessageID, "valore", v)
		} else {
			r, err := q.GetRichiesta(ctx, rid)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				s.avvisa("marcatore richiesta senza richiesta", "messaggio", m.MessageID, "richiesta", rid)
			case err != nil:
				return false, fmt.Errorf("marcatore richiesta: %w", err)
			default:
				n, err := q.SetRichiestaInviata(ctx, db.SetRichiestaInviataParams{RichiestaID: rid,
					MessaggioID: uuid.NullUUID{UUID: row.MessaggioID, Valid: true}, InviataIl: &m.DataEvento})
				if err != nil {
					return false, fmt.Errorf("richiesta inviata: %w", err)
				}
				if err := q.AgganciaRispostaFornitore(ctx, db.AgganciaRispostaFornitoreParams{MessaggioID: row.MessaggioID,
					RichiestaFornitoreID: uuid.NullUUID{UUID: rid, Valid: true}}); err != nil {
					return false, fmt.Errorf("messaggio → richiesta: %w", err)
				}
				if !row.ThreadID.Valid {
					tid := uuid.NullUUID{UUID: r.ThreadID, Valid: true}
					if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{MessaggioID: row.MessaggioID, ThreadID: tid,
						Aggancio: db.AggancioOperatore, AgganciatoDa: r.CreataDa}); err != nil {
						return false, fmt.Errorf("aggancio dal marcatore: %w", err)
					}
					if err := q.InsertAgganciaLog(ctx, db.InsertAgganciaLogParams{MessaggioID: row.MessaggioID, ThreadID: tid, Azione: "aggancia",
						UtenteID: r.CreataDa, Motivo: pgtype.Text{String: "marcatore " + MarcatoreRichiesta + " sulla mail inviata: la richiesta creata dal Cockpit", Valid: true}}); err != nil {
						return false, fmt.Errorf("log dell'aggancio: %w", err)
					}
					row.ThreadID, row.Aggancio = tid, db.AggancioOperatore
					agganciato = true
				}
				if s.Log != nil {
					s.Log.Info("richiesta a fornitore legata alla mail inviata", "richiesta", rid, "messaggio", m.MessageID, "aggiornata", n == 1)
				}
			}
		}
	}
	if v := m.Marcatori[MarcatoreBozza]; v != "" {
		if bid, err := uuid.Parse(v); err == nil {
			n, err := q.SetBozzaInviata(ctx, db.SetBozzaInviataParams{BozzaID: bid, InviataMessaggioID: uuid.NullUUID{UUID: row.MessaggioID, Valid: true}})
			if err != nil {
				return agganciato, fmt.Errorf("bozza inviata: %w", err)
			}
			if n == 1 && s.Log != nil {
				s.Log.Info("bozza partita", "bozza", bid, "messaggio", m.MessageID)
			}
		}
	}
	return agganciato, nil
}

func (s *Servizio) avvisa(msg string, kv ...any) {
	if s.Log != nil {
		s.Log.Warn(msg, kv...)
	}
}
