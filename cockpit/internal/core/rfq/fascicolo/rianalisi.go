package fascicolo

// La rianalisi degli STEP di una RFQ (Blocco 8, B8.5): su richiesta e all'apertura.
//
// Due cose diverse con lo stesso gesto. Gli STEP che hanno gia' i fatti con la chiave corrente si
// RILEGGONO: la classificazione e il confronto con la working si rifanno adesso, con le regole del
// cliente di adesso (una regola nuova riclassifica le proposte ancora aperte, A1.2), e le rimozioni si
// ricalcolano. Quelli che non li hanno (fatti v1 o v2, o mai analizzati) si ACCODANO al worker, pochi
// alla volta: la coda e' condivisa, e aprire una RFQ con cento STEP non deve riempirla. La chiave
// idempotente dell'analisi e' per contenuto, versione e configurazione: accodare due volte non
// raddoppia niente.

import (
	"context"
	"errors"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

func inStaging(percorso string) bool {
	st, err := os.Stat(percorso)
	return err == nil && st.Mode().IsRegular()
}

// MaxAccodatiPerApertura e' quanti STEP senza fatti correnti si accodano ogni volta che si apre una RFQ.
// Gli altri aspettano la prossima apertura, o il gesto esplicito.
const MaxAccodatiPerApertura = 5

// Rianalisi dice che cosa ha fatto RianalizzaRfq.
type Rianalisi struct {
	Riletti      int // STEP con i fatti correnti, riapplicati alla RFQ
	Accodati     int // analisi accodate adesso
	GiaInCoda    int // analisi gia' accodate da prima (stessa chiave idempotente)
	Rimandati    int // oltre il limite: alla prossima volta
	SenzaStaging int // il contenuto non e' piu' in staging: si riprende con «Riscarica», poi si rianalizza
	Proposte     int // righe di proposta scritte o riscritte
}

// RianalizzaRfq rilegge gli STEP della RFQ con i fatti correnti e accoda l'analisi di quelli che non li
// hanno, al massimo maxAccodati. an e' l'analizzatore corrente del server (cfg.Analisi).
func RianalizzaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int) (Rianalisi, error) {
	var r Rianalisi
	step, err := q.ListStepDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return r, err
	}
	if len(step) == 0 {
		return r, nil
	}
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return r, err
	}
	hash := an.Hash()
	accodato := map[string]bool{}
	for _, a := range step {
		f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String,
			VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: hash})
		switch {
		case err == nil:
			es, err := ApplicaStruttura(ctx, q, thread, a, f.Fatti, m)
			if err != nil {
				return r, err
			}
			r.Riletti++
			r.Proposte += es.Nodi + es.Relazioni
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return r, err
		}
		if accodato[a.Sha256.String] {
			continue // lo stesso contenuto e' gia' stato accodato per un altro allegato
		}
		// Il worker si prende i byte dallo staging del server: se il contenuto non c'e' piu' (la cache lo
		// ha tolto) l'analisi fallirebbe con «usa Riscarica». Non si accoda: si conta, e lo si dice.
		if !a.PathStaging.Valid || !inStaging(a.PathStaging.String) {
			r.SenzaStaging++
			continue
		}
		if r.Accodati >= maxAccodati {
			r.Rimandati++
			continue
		}
		j, err := coda.AccodaAnalisi(ctx, q, a, uuid.NullUUID{UUID: thread, Valid: true}, an)
		if err != nil {
			return r, err
		}
		accodato[a.Sha256.String] = true
		if j == nil {
			r.GiaInCoda++ // un'analisi con la stessa chiave e' gia' pendente
			continue
		}
		r.Accodati++
	}
	return r, nil
}
