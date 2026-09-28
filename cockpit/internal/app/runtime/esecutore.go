// Package runtime e' il processo: chi mette insieme i pezzi e li fa girare.
//
// esecutore.go ESEGUE i job di tipo 'server', quelli che non hanno un worker dall'altra parte perche'
// il lavoro lo fa il server stesso: prende il job dalla coda, ne riconosce il tipo e chiama chi sa
// farlo — `core/rfq/documenti` per il fascicolo, `transport/workerapi` per gli archivi, `ai/agente`
// per l'analisi. Qui c'e' il CHI esegue; il COSA sta in chi viene chiamato.
//
// vigilanza_nas.go tiene la condizione del mondo: un job che tocca il NAS quando il NAS non c'e' si
// rinvia, non fallisce.
//
// La coda su cui lavora (accodamento, claim, lease, capacita', instradamento) sta in
// `platform/coda`; la cartella di lavoro sul disco in `platform/storage/staging`; che cosa
// significhi copiare un documento, in `core/rfq/documenti`.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/ai/agente"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// Estrattore sa scompattare un archivio gia' in staging e registrarne le voci.
//
// E' un'interfaccia dichiarata QUI, e implementata altrove, per un motivo di dipendenze: il resto
// della pipeline dopo lo staging — proposte, rumore, analisi — vive nel pacchetto che riceve i
// risultati dei worker, e quel pacchetto importa gia' questo. Spostare tutto qui per far girare
// l'estrazione nell'esecutore sarebbe un trasloco molto piu' grande della correzione.
//
// `token` e' il lease del tentativo: la cartella temporanea in cui l'archivio si scompatta porta quel
// token nel nome, cosi' e' roba di QUESTO tentativo e la pulizia sa di chi era se il tentativo non
// arriva in fondo.
type Estrattore interface {
	EstraiArchivio(ctx context.Context, allegatoID uuid.UUID, token uuid.UUID) (int, error)
}

// EsecutoreServer prende i job con worker_tipo='server' (scrittura NAS, estrazione degli archivi) e
// li esegue in una goroutine.
type EsecutoreServer struct {
	Pool *pgxpool.Pool
	NAS  *nas.Scrittore
	Log  *slog.Logger
	// Archivi esegue i job `estrai_archivio`. Nil = quei job falliscono dicendolo, invece di restare
	// in coda a tempo indeterminato mentre gli operatori aspettano le voci di uno zip.
	Archivi Estrattore
	// Agente esegue l'analisi semantica (checkpoint 3R §9). Nil o spento = i job di quel tipo
	// falliscono dicendo che l'analisi non e' attiva, invece di restare in coda a tempo indeterminato.
	Agente *agente.Servizio
	// RitardoNasAssente: fra quanto riprovare un job che tocca il NAS trovato irraggiungibile.
	// Zero = un minuto. È un campo e non una costante perché il test non può aspettare un minuto.
	RitardoNasAssente time.Duration
	// ControlloNas: ogni quanto guardare se il NAS è tornato. Zero = un minuto.
	ControlloNas time.Duration
}

func (e *EsecutoreServer) Avvia(ctx context.Context) {
	q := db.New(e.Pool)
	if !e.NAS.DryRun {
		go e.vigilaNas(ctx, q)
	}
	go func() {
		id := "server"
		for ctx.Err() == nil {
			j, err := coda.Claim(ctx, q, db.WorkerTipoServer, id, coda.Destinazione{}, 20*time.Second)
			if err != nil {
				e.Log.Error("claim server", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if j == nil {
				continue
			}
			// anche l'esecutore interno passa dal tentativo: se il suo lease scade mentre scrive sul
			// NAS, il risultato non deve applicarsi al tentativo che nel frattempo ha ripreso il job
			t := coda.Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: id}
			if e.rinviaSeNasAssente(ctx, q, t, j) {
				continue
			}
			res, err := e.esegui(ctx, q, j, t)
			if err != nil {
				e.Log.Error("job server fallito", "job", j.JobID, "tipo", j.Tipo, "err", err)
				if _, err := coda.Fallisci(ctx, q, t, err.Error(), definitivo(err)); err != nil {
					e.Log.Warn("fallimento non registrato", "job", j.JobID, "err", err)
				}
				continue
			}
			raw, _ := json.Marshal(res)
			if _, err := coda.Completa(ctx, q, t, json.RawMessage(raw)); err != nil {
				e.Log.Error("completa job", "job", j.JobID, "err", err)
			}
		}
	}()
}

func (e *EsecutoreServer) esegui(ctx context.Context, q *db.Queries, j *db.Job, t coda.Tentativo) (any, error) {
	switch j.Tipo {
	case db.TipoJobEstraiArchivio:
		var p worker.PayloadEstraiArchivio
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		if e.Archivi == nil {
			return nil, errArchiviNonConfigurati
		}
		voci, err := e.Archivi.EstraiArchivio(ctx, p.AllegatoID, t.LeaseToken)
		if err != nil {
			return nil, err
		}
		return map[string]any{"allegato_id": p.AllegatoID, "voci": voci}, nil

	case db.TipoJobAnalizzaMessaggioAi:
		var p worker.PayloadAnalizzaMessaggioAI
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		a, err := e.Agente.Analizza(ctx, q, p.MessaggioID)
		if err != nil {
			return nil, err
		}
		// l'esito del job dice che cosa e' successo, compreso «rifiutata»: un output non conforme non
		// e' un errore del sistema, ed e' una misura che va conservata
		return map[string]any{"analisi_id": a.AnalisiID, "stato": a.Stato,
			"scartati": contaScartati(a.Scartato), "token_in": a.TokenIn.Int32, "token_out": a.TokenOut.Int32}, nil

	case db.TipoJobCreaCartellaThread:
		var p worker.PayloadCreaCartellaThread
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		return documenti.CreaCartellaThread(ctx, q, e.NAS, p.ThreadID)

	case db.TipoJobCopiaNas:
		var p worker.PayloadCopiaNAS
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		return documenti.CopiaSulNas(ctx, q, e.NAS, j, p.DocumentoID)
	}
	return nil, fmt.Errorf("%w: %s", errTipoNonGestito, j.Tipo)
}

var (
	// errTipoNonGestito: questo server non sa eseguire il tipo del job (sposta_nas prima di B8.8, un tipo
	// di un binario piu' nuovo).
	errTipoNonGestito = errors.New("tipo job non gestito dal server")
	// errArchiviNonConfigurati: l'esecutore e' partito senza chi scompatta gli archivi.
	errArchiviNonConfigurati = errors.New("estrazione degli archivi non configurata su questo server")
)

// definitivo dice se un fallimento non si risolve riprovando: un conflitto sul NAS (il file di un altro
// non si sovrascrive), un tipo che questo server non sa eseguire, l'analisi semantica spenta, l'estrazione
// non configurata. Riprovarli cinque volte non cambiava niente, se non far aspettare cinque rinvii a chi
// guarda la coda prima di vedere il motivo vero.
func definitivo(err error) bool {
	return errors.Is(err, nas.ErrConflitto) || errors.Is(err, errTipoNonGestito) ||
		errors.Is(err, agente.ErrSpento) || errors.Is(err, errArchiviNonConfigurati)
}

// contaScartati e' quanti elementi scartati ha l'analisi (un array JSON). len() sul grezzo contava i
// byte del JSON: «scartati: 2» voleva dire «[]».
func contaScartati(grezzo json.RawMessage) int {
	var voci []json.RawMessage
	if json.Unmarshal(grezzo, &voci) != nil {
		return 0
	}
	return len(voci)
}
