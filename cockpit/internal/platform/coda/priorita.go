package coda

import (
	"context"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Le priorita' delle analisi (Smistamento F8, addendum A5.13.6). La coda serve per (priorita, job_id): un
// numero piu' basso passa prima. Le analisi sono a 6, le estrazioni degli archivi a 4, i download a 2. Gli
// STEP e i PDF il cui nome e' compatibile con un prodotto della RFQ passano a 5: sono i file da cui il flusso
// ancorato al prodotto aspetta l'ancora, e il resto dei file si puo' collocare solo dopo di loro. Le analisi
// continuano sempre tutte: la priorita' decide soltanto l'ordine.
const (
	PrioritaAnalisi            int16 = 6
	PrioritaAnalisiCompatibile int16 = 5
)

// AccodaAnalisiCon e' AccodaAnalisi con la priorita' data (A5.13.6, IN2): lo stesso job, con la stessa chiave
// e lo stesso controllo dei fatti gia' calcolati, e poi, se e' nato adesso e la priorita' chiesta passa prima
// del 6, il numero abbassato nella stessa transazione. (nil, nil) come AccodaAnalisi: i fatti ci sono gia', o
// un'analisi con la stessa chiave e' gia' pendente (e allora la si alza con AlzaPriorita).
func AccodaAnalisiCon(ctx context.Context, q *db.Queries, a db.Allegato, threadID uuid.NullUUID, an Analizzatore, priorita int16) (*db.Job, error) {
	j, err := AccodaAnalisi(ctx, q, a, threadID, an)
	if err != nil || j == nil || priorita >= j.Priorita {
		return j, err
	}
	if _, err := AlzaPriorita(ctx, q, []string{ChiaveAnalisi(a.Sha256.String, an)}, priorita); err != nil {
		return nil, err
	}
	j.Priorita = priorita
	return j, nil
}

// RiaccodaAnalisiCon e' RiaccodaAnalisi con la priorita' data (giro 4, fase 4.2: «Rianalizza» passa prima il
// disegno del prodotto): lo stesso job, con la stessa chiave, e il numero abbassato nella stessa transazione. A
// differenza di AccodaAnalisiCon passa avanti anche un'analisi con la stessa chiave gia' pronta in coda ((nil,
// nil) come RiaccodaAnalisi): una persona ha chiesto di rileggere, e quel file e' quello da cui il flusso
// aspetta l'ancora.
func RiaccodaAnalisiCon(ctx context.Context, q *db.Queries, a db.Allegato, threadID uuid.NullUUID, an Analizzatore, priorita int16) (*db.Job, error) {
	j, err := RiaccodaAnalisi(ctx, q, a, threadID, an)
	if err != nil || priorita >= PrioritaAnalisi {
		return j, err
	}
	if _, err := AlzaPriorita(ctx, q, []string{ChiaveAnalisi(a.Sha256.String, an)}, priorita); err != nil {
		return nil, err
	}
	if j != nil && priorita < j.Priorita {
		j.Priorita = priorita
	}
	return j, nil
}

// AlzaPriorita porta alla priorita' data i job PRONTI con quelle chiavi, se ne hanno una piu' lenta: mai il
// contrario, e mai un job gia' preso da un worker. Restituisce quanti ne ha cambiati. Idempotente.
func AlzaPriorita(ctx context.Context, q *db.Queries, chiavi []string, priorita int16) (int64, error) {
	if len(chiavi) == 0 {
		return 0, nil
	}
	return q.AlzaPrioritaJob(ctx, db.AlzaPrioritaJobParams{Chiavi: chiavi, Priorita: priorita})
}
