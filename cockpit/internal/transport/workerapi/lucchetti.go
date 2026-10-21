package workerapi

// lucchetti.go: l'ordine dei lucchetti del risultato di un job (giro G39-bis, corsia file).
//
// L'ordine unico di chi scrive file o letture di file in una RFQ e' la RFQ per prima, come R5 (core/rfq/distintabozza),
// i gesti legacy (transport/web/lucchetti_legacy.go), i caricamenti (fascicolo.notaDellaRFQ, E1′ compreso) e
// l'estrazione di un archivio (archivi.go):
//
//  1. le RFQ che il risultato tocca, per thread_id crescente: FOR KEY SHARE (BloccaThreadCondiviso) nel risultato di un
//     download o di un'analisi, che non alza mai quel lucchetto; FOR UPDATE (BloccaThread) nell'estrazione e in chi
//     decide;
//  2. la riga del job (coda.Blocca);
//  3. le proposte dell'allegato (BloccaPropostaDiAllegato), poi il contenuto (BloccaContenuto, per sha crescente quando
//     sono piu' d'uno), poi le righe che il lavoro scrive.
//
// Prima il risultato bloccava il job, le proposte e il contenuto, e la RFQ arrivava per ultima, con le chiavi esterne
// delle righe nuove (documento_proposta, componente_proposta e le altre) o con il trigger della working: chi teneva la
// RFQ FOR UPDATE e poi chiedeva una di quelle righe (R5, un gesto, un caricamento che aspetta il contenuto) chiudeva un
// ciclo, e PostgreSQL ne uccideva uno (40P01). Un risultato del worker ucciso cosi' diventava un 422 con il fallimento
// definitivo del job, cioe' un allegato in errore per una gara.
//
// Le RFQ si leggono prima della transazione (rfqDelRisultato): una RFQ che compare dopo la lettura (il messaggio
// agganciato in quell'istante, una copia nuova dello stesso contenuto) si prende con la chiave esterna, fuori ordine.
// Per quel caso, e per un ciclo che nasce fuori da quest'ordine, un 40P01 o un 40001 non e' un risultato non
// applicabile: la transazione si rifa' da capo, al piu' maxTentativiRisultato volte (rifarla e' idempotente: il rollback
// toglie le righe, e il contenuto gia' promosso dal tentativo precedente si ritrova sotto BloccaContenuto). Finiti i
// tentativi, 503: il job resta in corso con lo stesso tentativo. Il worker Python non ripete un 5xx (riporta_risultato in
// workers/cockpit_client.py ripete solo quando cade la rete): il lavoro lo rifa' da capo la scadenza del lease, come
// per ogni result perso. [T-39, da ratificare: prima un 40P01 era un 422 con il fallimento definitivo del job.]

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// maxTentativiRisultato: quante volte il risultato di un job prova la sua transazione prima di rispondere 503.
const maxTentativiRisultato = 3

// rfqDelRisultato: le RFQ che il risultato del job tocchera', per thread_id crescente e senza ripetizioni. Un download:
// la RFQ del messaggio dell'allegato. Un'analisi: quella dell'allegato del job, quelle delle proposte aperte con lo
// stesso contenuto (a cui va la lettura) e quelle aperte con lo stesso contenuto (a cui va la struttura). Gli altri
// lavori non toccano RFQ. Si legge fuori dalla transazione (commento di testa).
func (s *Server) rfqDelRisultato(ctx context.Context, q *db.Queries, j *db.Job, prep *stagePronto) ([]uuid.UUID, error) {
	viste := map[uuid.UUID]bool{}
	aggiungi := func(id uuid.NullUUID) {
		if id.Valid {
			viste[id.UUID] = true
		}
	}
	rfqDelMessaggio := func(messaggio uuid.UUID) error {
		m, err := q.GetMessaggio(ctx, messaggio)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		aggiungi(m.ThreadID)
		return nil
	}
	switch j.Tipo {
	case db.TipoJobStageAllegato:
		if prep == nil {
			return nil, nil
		}
		if err := rfqDelMessaggio(prep.a.MessaggioID); err != nil {
			return nil, err
		}
	case db.TipoJobAnalizzaAllegato:
		var p worker.PayloadAnalizzaAllegato
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, nil // il risultato lo dira' non applicabile, con il motivo
		}
		a, err := q.GetAllegato(ctx, p.AllegatoID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if err := rfqDelMessaggio(a.MessaggioID); err != nil {
			return nil, err
		}
		if a.Sha256.Valid && a.Sha256.String != "" {
			aperte, err := q.ListProposteAperteStessoFile(ctx, a.Sha256)
			if err != nil {
				return nil, err
			}
			for _, pr := range aperte {
				if err := rfqDelMessaggio(pr.MessaggioID); err != nil {
					return nil, err
				}
			}
			righe, err := q.ListRfqAperteConContenuto(ctx, a.Sha256)
			if err != nil {
				return nil, err
			}
			for _, r := range righe {
				aggiungi(r.RfqID)
			}
		}
	}
	out := make([]uuid.UUID, 0, len(viste))
	for id := range viste {
		out = append(out, id)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].String() < out[k].String() })
	return out, nil
}

// bloccaRfq prende le RFQ FOR KEY SHARE nell'ordine dato (crescente: rfqDelRisultato). Una RFQ che non c'e' piu' non e'
// un errore: non c'e' niente da mettere in fila.
//
// Senza lock_timeout, a differenza di R6 (il commento di BloccaThreadCondiviso): R6 risponde a una pagina che aspetta, il
// result a un worker che ha gia' fatto il lavoro. Il result aspetta chi tiene la RFQ FOR UPDATE quanto dura la sua
// transazione (R5, che ha il suo lock_timeout sulle righe, un gesto, un caricamento, un'estrazione), e l'attesa ha
// comunque un limite: il timeout HTTP del worker (60 s in workers/cockpit_client.py) chiude la connessione, il contesto
// della richiesta si annulla e la query con lui (rollback, il job resta in corso; il client ripete il result perche' e'
// un errore di rete, e la ripetizione o fa il lavoro o riceve 409). Con un lock_timeout una R5 lunga diventerebbe un
// errore e, per un client che non ripete i 5xx, un lavoro rifatto da capo alla scadenza del lease: aspettare costa meno.
// [T-39, da ratificare]
func bloccaRfq(ctx context.Context, q *db.Queries, rfq []uuid.UUID) error {
	for _, id := range rfq {
		if _, err := q.BloccaThreadCondiviso(ctx, id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return nil
}

// temporaneo: un errore del database che si risolve rifacendo la transazione (40P01, 40001).
func temporaneo(err error) bool { return filerfq.ErroreTemporaneo(err) }

// attendiRitento aspetta un poco prima del tentativo dopo il numero n (25 ms, 50 ms, ...); falso se il contesto finisce.
func attendiRitento(ctx context.Context, n int) bool {
	t := time.NewTimer(time.Duration(25*n) * time.Millisecond)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
