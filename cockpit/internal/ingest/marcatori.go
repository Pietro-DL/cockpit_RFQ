package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/platform/db"
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
//
// Vale SOLO dove il Cockpit l'ha messo (7C.0, invariante 5). Cioe':
//   - sulla nostra posta in USCITA: il Cockpit scrive le UserProperties sulla bozza che prepara, mai
//     su una mail che arriva. Un marcatore su una mail in entrata non l'abbiamo messo noi — un
//     inoltro dentro Exchange che si porta dietro le proprieta', o chiunque altro — e non vale;
//   - su un messaggio che nessuno ha gia' messo in un'ALTRA RFQ: il risync di una mail decisa a mano
//     non la sposta, e non lega nemmeno la richiesta, perche' una richiesta della RFQ A su una mail
//     che sta nella RFQ B e' una contraddizione, non un legame.
//
// La direzione arriva da fuori perche' la decide il server dalle caselle censite (D8), non il
// campo del worker.
func (s *Servizio) applicaMarcatori(ctx context.Context, q *db.Queries, row *db.UpsertMessaggioRow, m *api.MessaggioIn, dir db.Direzione) (bool, error) {
	if len(m.Marcatori) == 0 {
		return false, nil
	}
	agganciato := false
	if v := m.Marcatori[MarcatoreRichiesta]; v != "" {
		rid, err := uuid.Parse(v)
		if err != nil {
			s.avvisa("marcatore richiesta non valido", "messaggio", m.MessageID, "valore", v)
		} else if dir != db.DirezioneUscita {
			s.avvisa("marcatore richiesta su una mail in entrata: ignorato, il Cockpit non lo ha messo lui", "messaggio", m.MessageID, "richiesta", rid)
		} else {
			r, err := q.GetRichiesta(ctx, rid)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				s.avvisa("marcatore richiesta senza richiesta", "messaggio", m.MessageID, "richiesta", rid)
			case err != nil:
				return false, fmt.Errorf("marcatore richiesta: %w", err)
			case row.ThreadID.Valid && row.ThreadID.UUID != r.ThreadID:
				s.avvisa("marcatore richiesta su un messaggio gia' deciso in un'altra RFQ: ignorato", "messaggio", m.MessageID,
					"richiesta", rid, "rfq_del_messaggio", row.ThreadID.UUID, "rfq_della_richiesta", r.ThreadID)
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
