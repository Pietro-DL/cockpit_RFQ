package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/db"
)

// Riprova rimette in gioco un elemento finito in scarto. I due tipi di scarto si riprovano in modi
// diversi, e la differenza non è un dettaglio:
//
//   - origine 'ingest': il payload dell'elemento è in database. Si riprova da lì, senza Outlook. È
//     l'unico modo che funzioni anche quando l'elemento nel frattempo è stato spostato o eliminato
//     dalla casella, cioè proprio nei casi in cui si riprova a distanza di giorni.
//   - origine 'lettura': il worker non era riuscito a leggere l'elemento, quindi un payload non c'è.
//     Si accoda un job che lo rilegge da quella casella e lo rimanda in un lotto da uno.
//
// Restituisce una frase da mostrare a chi ha premuto il pulsante: chi riprova vuole sapere che cosa è
// successo, non solo che «l'operazione è riuscita».
func (s *Servizio) Riprova(ctx context.Context, scartoID int64) (string, error) {
	q := db.New(s.Pool)
	sc, err := q.GetIngestScarto(ctx, scartoID)
	if err != nil {
		return "", err
	}
	casella, err := q.GetCasella(ctx, sc.CasellaID)
	if err != nil {
		return "", fmt.Errorf("casella dello scarto: %w", err)
	}

	switch sc.Origine {
	case "ingest":
		var m api.MessaggioIn
		if err := json.Unmarshal(sc.Payload, &m); err != nil {
			return "", fmt.Errorf("payload dello scarto illeggibile: %w", err)
		}
		res, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: []api.MessaggioIn{m}})
		if err != nil {
			return "", err
		}
		if res.Falliti > 0 {
			motivo := "motivo non riportato"
			if len(res.Esiti) > 0 && res.Esiti[0].Errore != "" {
				motivo = res.Esiti[0].Errore
			}
			return "ancora in errore: " + motivo, nil
		}
		// Ingerisci ha già tolto lo scarto per quell'elemento
		if res.Inseriti > 0 {
			return "acquisito", nil
		}
		return "già presente: aggiornato", nil

	case "lettura":
		j, err := jobs.AccodaCon(ctx, q, db.TipoJobRileggiElemento, api.PayloadRileggiElemento{
			CasellaID: sc.CasellaID, EntryID: sc.EntryID, Cartella: sc.Cartella.String, MessageID: sc.MessageID.String,
		}, fmt.Sprintf("rileggi:%s:%s", sc.CasellaID, sc.EntryID), 3,
			jobs.Opzioni{Casella: uuidValido(sc.CasellaID)})
		if err != nil {
			return "", err
		}
		if j == nil {
			return "già in coda: la rilettura è in attesa di un worker", nil
		}
		return fmt.Sprintf("rilettura accodata (job %d)", j.JobID), nil
	}
	return "", fmt.Errorf("origine dello scarto sconosciuta: %q", sc.Origine)
}

// uuidValido incapsula un uuid in una NullUUID valorizzata.
func uuidValido(u uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: u, Valid: true} }
