package fascicolo

// La rianalisi degli STEP di una RFQ (Blocco 8, B8.5): due meta' con inneschi diversi.
//
// Gli STEP che hanno gia' i fatti con la chiave corrente si RILEGGONO (RileggiStepDellaRfq): la
// classificazione e il confronto con la working si rifanno adesso, con le regole del cliente di adesso
// (una regola nuova riclassifica le proposte ancora aperte, A1.2), e le rimozioni si ricalcolano. Quelli
// che non li hanno (fatti v1 o v2, o mai analizzati) si ACCODANO al worker (AccodaAnalisiMancanti), pochi
// alla volta: la coda e' condivisa, e una RFQ con cento STEP non deve riempirla. La chiave idempotente
// dell'analisi e' per contenuto, versione e configurazione: accodare due volte non raddoppia niente.
//
// Smistamento, fase F1 (addendum A5.4.5, D52): nessuna GET scrive. Accodare e' lavoro grezzo, e lo fa
// anche la preparazione (POST …/fascicolo/prepara); rileggere scrive proposte di struttura, e succede solo
// su un evento esplicito: la creazione o l'aggancio della RFQ (gli STEP analizzati prima, critica C6) e il
// gesto «Rianalizza», che fa le due meta' insieme (RianalizzaRfq).

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

// Rianalisi dice che cosa hanno fatto RileggiStepDellaRfq, AccodaAnalisiMancanti o RianalizzaRfq.
type Rianalisi struct {
	Riletti      int // STEP con i fatti correnti, riapplicati alla RFQ
	Accodati     int // analisi accodate adesso
	GiaInCoda    int // analisi gia' accodate da prima (stessa chiave idempotente)
	Rimandati    int // oltre il limite: alla prossima volta
	SenzaStaging int // il contenuto non e' piu' in staging: si riprende con «Riscarica», poi si rianalizza
	Proposte     int // righe di proposta scritte o riscritte
}

// piu somma due esiti: RianalizzaRfq e' le due meta' insieme, e il gesto dice il totale.
func (r Rianalisi) piu(o Rianalisi) Rianalisi {
	return Rianalisi{Riletti: r.Riletti + o.Riletti, Accodati: r.Accodati + o.Accodati, GiaInCoda: r.GiaInCoda + o.GiaInCoda,
		Rimandati: r.Rimandati + o.Rimandati, SenzaStaging: r.SenzaStaging + o.SenzaStaging, Proposte: r.Proposte + o.Proposte}
}

// RianalizzaRfq e' il gesto «Rianalizza»: rilegge gli STEP della RFQ con i fatti correnti e accoda
// l'analisi di quelli che non li hanno, al massimo maxAccodati. an e' l'analizzatore corrente del server
// (cfg.Analisi).
func RianalizzaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int) (Rianalisi, error) {
	riletti, err := RileggiStepDellaRfq(ctx, q, thread, an)
	if err != nil {
		return riletti, err
	}
	accodati, err := AccodaAnalisiMancanti(ctx, q, thread, an, maxAccodati)
	return riletti.piu(accodati), err
}

// RileggiStepDellaRfq riapplica alla RFQ i fatti correnti dei suoi STEP: le proposte di struttura si
// rifanno con le regole del cliente di adesso. Scrive proposte, quindi si chiama solo da un gesto o da un
// evento esplicito (la creazione o l'aggancio della RFQ, «Rianalizza»), mai dall'apertura di una pagina.
// Gli STEP senza fatti correnti non si toccano: li accoda AccodaAnalisiMancanti.
func RileggiStepDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore) (Rianalisi, error) {
	var r Rianalisi
	step, err := q.ListStepDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil || len(step) == 0 {
		return r, err
	}
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return r, err
	}
	hash := an.Hash()
	for _, a := range step {
		f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String,
			VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: hash})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return r, err
		}
		es, err := ApplicaStruttura(ctx, q, thread, a, f.Fatti, m)
		if err != nil {
			return r, err
		}
		r.Riletti++
		r.Proposte += es.Nodi + es.Relazioni
	}
	return r, nil
}

// AccodaAnalisiMancanti accoda l'analisi degli STEP della RFQ che non hanno i fatti con la chiave corrente
// (mai analizzati, o analizzati da una versione vecchia dell'analizzatore), al massimo maxAccodati. E'
// solo lavoro per il worker: non scrive proposte. I risultati, quando arrivano, passano per la strada
// normale dell'analisi. E' la meta' del gesto «Rianalizza»: una persona l'ha chiesto, quindi riprova anche
// un'analisi il cui ultimo tentativo e' fallito.
func AccodaAnalisiMancanti(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int) (Rianalisi, error) {
	return accodaAnalisiMancanti(ctx, q, thread, an, maxAccodati, false)
}

// AccodaAnalisiMancantiDaSola e' AccodaAnalisiMancanti per la preparazione, che nessuno chiede con un gesto
// (la pagina la manda aprendosi, Smistamento F1). Come PreparaFile non insiste da sola: un'analisi il cui
// ultimo tentativo e' fallito non si riaccoda a ogni apertura, la guarda una persona e la riprova con
// «Rianalizza». E lascia a PreparaFile i file ancora fermi nello staging, che ha appena guardato con le sue
// regole: contati due volte, lo stesso file sarebbe «accodato» e «gia' in coda» nello stesso avviso.
func AccodaAnalisiMancantiDaSola(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int) (Rianalisi, error) {
	return accodaAnalisiMancanti(ctx, q, thread, an, maxAccodati, true)
}

func accodaAnalisiMancanti(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int, daSola bool) (Rianalisi, error) {
	var r Rianalisi
	step, err := q.ListStepDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return r, err
	}
	hash := an.Hash()
	accodato := map[string]bool{}
	for _, a := range step {
		if daSola && a.Stato == db.StatoAllegatoInStaging {
			continue // e' un file fermo: lo prepara PreparaFile
		}
		_, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String,
			VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: hash})
		switch {
		case err == nil:
			continue // i fatti ci sono: e' materia di RileggiStepDellaRfq
		case !errors.Is(err, pgx.ErrNoRows):
			return r, err
		}
		if daSola {
			j, err := q.UltimoJobPerChiave(ctx, testo(coda.ChiaveAnalisi(a.Sha256.String, an)))
			switch {
			case err == nil && j.Stato == db.StatoJobFallito:
				continue
			case err != nil && !errors.Is(err, pgx.ErrNoRows):
				return r, err
			}
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
