// Package jobs ESEGUE i job di tipo 'server': quelli che non hanno un worker dall'altra parte
// perche' il lavoro lo fa il server stesso — la copia sul NAS, la cartella del thread, la ripresa di
// un contenuto sparito — e il ricognitore che confronta i documenti con i file veri.
//
// La coda su cui lavora (accodamento, claim, lease, capacita', instradamento) sta in
// `platform/coda`; la cartella di lavoro sul disco in `platform/storage/staging`. Qui c'e' solo chi
// prende un job e lo porta a termine.
package jobs

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
				if _, err := coda.Fallisci(ctx, q, t, err.Error(), errors.Is(err, nas.ErrConflitto)); err != nil {
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

// rinviaSeNasAssente restituisce true se il job è stato rimesso in coda senza essere eseguito.
//
// Il tentativo non viene consumato: il job non è fallito, non lo abbiamo nemmeno provato. Contarlo
// come fallimento significherebbe bruciare il budget dei tentativi mentre il problema è che il NAS
// non c'è — e dichiarare persa la copia proprio per aver provato tante volte (voce 1.7, N8).
func (e *EsecutoreServer) rinviaSeNasAssente(ctx context.Context, q *db.Queries, t coda.Tentativo, j *db.Job) bool {
	if e.NAS.DryRun || !documenti.ScrivePerNas(j.Tipo) || e.NAS.Raggiungibile() {
		return false
	}
	fra := e.RitardoNasAssente
	if fra <= 0 {
		fra = time.Minute
	}
	if _, err := coda.Rinvia(ctx, q, t, fra, "NAS non raggiungibile: tentativo non consumato"); err != nil {
		e.Log.Error("rinvio non riuscito", "job", j.JobID, "tipo", j.Tipo, "err", err)
		return false // meglio provarci: il peggio che può succedere è un fallimento onesto
	}
	e.Log.Warn("NAS non raggiungibile: job rinviato senza consumare il tentativo",
		"job", j.JobID, "tipo", j.Tipo, "tentativi", j.Tentativi, "fra", fra)
	return true
}

// vigilaNas guarda se il NAS è tornato e, quando torna, rimette in coda le scritture che avevano
// finito i tentativi (N3: «NAS torna → scritto senza intervento»).
//
// Il primo controllo è all'avvio e non aspetta la transizione: se il server viene riavviato dopo che
// il NAS è già tornato, una transizione non ci sarà mai più e le copie resterebbero ferme per sempre.
func (e *EsecutoreServer) vigilaNas(ctx context.Context, q *db.Queries) {
	ogni := e.ControlloNas
	if ogni <= 0 {
		ogni = time.Minute
	}
	presente := e.NAS.Raggiungibile()
	if presente {
		e.RiaccodaAlRitornoDelNas(ctx, q)
	}
	t := time.NewTicker(ogni)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ora := e.NAS.Raggiungibile()
		if ora && !presente {
			e.Log.Info("NAS di nuovo raggiungibile", "radice", e.NAS.Radice)
			e.RiaccodaAlRitornoDelNas(ctx, q)
		}
		if !ora && presente {
			e.Log.Warn("NAS non più raggiungibile: le copie restano in coda", "radice", e.NAS.Radice)
		}
		presente = ora
	}
}

// RiaccodaAlRitornoDelNas rimette 'pronto' le scritture NAS che avevano esaurito i tentativi.
// Esportata perché è il punto che il test N3 chiama: quello che gira in produzione e quello che si
// verifica devono essere la stessa funzione.
func (e *EsecutoreServer) RiaccodaAlRitornoDelNas(ctx context.Context, q *db.Queries) int {
	ids, err := q.RiaccodaScrittureNasEsaurite(ctx)
	if err != nil {
		e.Log.Error("riaccodo al ritorno del NAS", "err", err)
		return 0
	}
	if len(ids) > 0 {
		e.Log.Info("scritture NAS rimesse in coda al ritorno del NAS", "n", len(ids), "job", ids)
	}
	return len(ids)
}

func (e *EsecutoreServer) esegui(ctx context.Context, q *db.Queries, j *db.Job, t coda.Tentativo) (any, error) {
	switch j.Tipo {
	case db.TipoJobEstraiArchivio:
		var p worker.PayloadEstraiArchivio
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		if e.Archivi == nil {
			return nil, errors.New("estrazione degli archivi non configurata su questo server")
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
			"scartati": len(a.Scartato), "token_in": a.TokenIn.Int32, "token_out": a.TokenOut.Int32}, nil

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
	return nil, fmt.Errorf("tipo job non gestito dal server: %s", j.Tipo)
}
