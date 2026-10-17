package fascicolo

// La rilettura degli STEP i cui fatti correnti hanno la struttura in una forma che il server non accetta (LS-01,
// 17/10/2026; direttiva 23 §4.1).
//
// GP-3 (e565cc8) aveva fatto scrivere al worker la struttura `versione: 4`, con la diagnostica della geometria, dei nomi
// d'istanza e dello stato, senza cambiare la chiave dell'analizzatore: i fatti v3 dei worker di prima e quelli v4 dei
// worker di GP-3 stanno sotto la stessa chiave. Per la completezza (struttura_motivo_parziale della 0020) la 4 non c'e':
// ogni STEP letto da un worker di GP-3 e' «parziale». Dopo LS-01 il worker torna a scrivere la 3, con la diagnostica
// numerata a parte (worker.VersioneDiagnostica); ma i fatti v4 gia' scritti restano, e ne' la preparazione ne'
// AccodaAnalisiMancanti li guardano: i fatti con la chiave corrente ci sono. Questo file li riaccoda.
//
// Come si riconoscono: worker.StrutturaDaRileggere, cioe' un numero esplicito (la forma diversa dalla 3), mai la
// presenza di un campo. Le v3 senza diagnostica (worker di prima di GP-3) non si riaccodano: la loro completezza e'
// giusta, e la diagnostica non la legge ancora nessuno. Nessuna struttura diventa «completa» per decreto: chi non e'
// riletto resta «parziale», con il motivo della 0020.
//
// Come si accoda: solo con il gesto «Rianalizza» (RianalizzaRfq), pochi alla volta, con la stessa chiave idempotente di
// ogni analisi (coda.RiaccodaAnalisi, che non si ferma ai fatti presenti come AccodaAnalisi); mai dalla preparazione
// (AccodaAnalisiMancantiDaSola) ne' da un'apertura di pagina: nessuna rianalisi in massa parte da sola (la regola di F9,
// rianalisi_pdf.go). La chiave dell'analizzatore non cambia: cambiarla rileggerebbe ogni file di ogni tipo.
//
// Che cosa succede al risultato: la strada normale (workerapi). UpsertAnalisiFatti riscrive i fatti sotto la stessa
// chiave e cambia calcolato_il, quindi una deroga strutturale data sulla lettura v4 decade (0020, v_step_prodotto: la
// deroga vale per la stessa analisi); strutturaNelleRfq riapplica la struttura, e le proposte decise da una persona non
// si toccano. Con la lettura completa possono comparire proposte di quantita' e di rimozione prima sospese: sono
// proposte, le decide una persona. Se risponde di nuovo un worker non aggiornato (flotta mista), i fatti tornano v4 e
// restano «parziali»: il server lo scrive nel log, con il worker (workerapi.applicaRisultato, dove il job e' noto,
// prima di strutturaNelleRfq), e il gesto li riaccoda solo quando una persona lo ripreme.

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// stepDaRileggere: un contenuto della RFQ da riaccodare, con la copia da mandare al worker (la prima nello staging, se
// c'e') e se il nome di una delle sue copie e' compatibile con un prodotto della RFQ.
type stepDaRileggere struct {
	allegato    db.Allegato
	inStaging   bool
	compatibile bool
}

// AccodaStepDaRileggere accoda l'analisi degli STEP della RFQ i cui fatti con la chiave corrente hanno una struttura da
// rileggere (worker.StrutturaDaRileggere), al massimo maxAccodati. E' una meta' del gesto «Rianalizza» (RianalizzaRfq),
// e solo di quello: una persona l'ha chiesto, quindi riprova anche un'analisi il cui ultimo tentativo e' fallito.
//   - Tutti gli STEP della RFQ, anche esclusi o di un fornitore, come AccodaAnalisiMancanti: l'analisi e' un fatto del
//     contenuto (decisione dell'utente del 27/09).
//   - Gli STEP senza i fatti correnti non sono di questa funzione: li accoda AccodaAnalisiMancanti.
//   - Per contenuto: lo stesso file in piu' allegati si accoda e si conta una volta, con la copia che e' nello staging.
//   - Un contenuto non piu' in staging non si accoda (il worker fallirebbe con «usa Riscarica»): si conta. Si
//     riprende con «Riscarica» e poi di nuovo «Rianalizza»: «Riscarica» da solo non lo rilegge, perche' al ritorno
//     nello staging dopoStaging → coda.AccodaAnalisi si ferma ai fatti presenti, e applicaFattiEsistenti li riapplica.
//   - Prima i contenuti con un nome compatibile con un prodotto della RFQ, con la priorita' che li fa passare prima
//     nella coda (coda.PrioritaAnalisiCompatibile), come i PDF (giro 4, fase 4.2); il limite lo usano per primi.
func AccodaStepDaRileggere(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxAccodati int) (Rianalisi, error) {
	var r Rianalisi
	step, err := q.ListStepDaAnalizzareDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil || len(step) == 0 {
		return r, err
	}
	hash := an.Hash()
	daRileggere := map[string]bool{} // per sha: i fatti correnti sono da rileggere (letti una volta per contenuto)
	visti := map[string]bool{}
	per := map[string]*stepDaRileggere{}
	var ordine []string
	for _, a := range step {
		sha := a.Sha256.String
		if !visti[sha] {
			visti[sha] = true
			f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: sha,
				VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: hash})
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				// nessun fatto corrente: e' un'analisi mancante, di AccodaAnalisiMancanti
			case err != nil:
				return r, err
			default:
				daRileggere[sha] = worker.StrutturaDaRileggere(f.Fatti)
			}
		}
		if !daRileggere[sha] {
			continue
		}
		c := per[sha]
		if c == nil {
			c = &stepDaRileggere{allegato: a}
			per[sha] = c
			ordine = append(ordine, sha)
		}
		if !c.inStaging && a.PathStaging.Valid && inStaging(a.PathStaging.String) {
			c.allegato, c.inStaging = a, true
		}
	}
	if len(ordine) == 0 {
		return r, nil
	}
	compatibili, err := CompatibiliConIProdotti(ctx, q, thread)
	if err != nil {
		return r, err
	}
	for _, a := range step {
		if c := per[a.Sha256.String]; c != nil && compatibili.Compatibile(a.NomeFile, a.PathInterno.String, "") {
			c.compatibile = true
		}
	}
	sort.SliceStable(ordine, func(i, j int) bool { return per[ordine[i]].compatibile && !per[ordine[j]].compatibile })
	for _, sha := range ordine {
		c := per[sha]
		if !c.inStaging {
			r.StepSenzaStaging++
			continue
		}
		if r.StepRiaccodati >= maxAccodati {
			r.StepRimandati++
			continue
		}
		priorita := coda.PrioritaAnalisi
		if c.compatibile {
			priorita = coda.PrioritaAnalisiCompatibile
		}
		// RiaccodaAnalisi e non AccodaAnalisi: quella si ferma ai fatti correnti, che qui ci sono (la v4)
		j, err := coda.RiaccodaAnalisiCon(ctx, q, c.allegato, uuid.NullUUID{UUID: thread, Valid: true}, an, priorita)
		if err != nil {
			return r, err
		}
		if j == nil {
			r.StepGiaInCoda++ // un'analisi con la stessa chiave e' gia' pendente
			continue
		}
		r.StepRiaccodati++
	}
	return r, nil
}
