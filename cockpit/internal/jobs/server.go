package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/nas"
)

// ScrivePerNas dice se il job ha bisogno che il NAS ci sia. Sono i due che scrivono sotto la radice:
// senza NAS non possono nemmeno cominciare, e provarci non insegna niente a nessuno.
func ScrivePerNas(t db.TipoJob) bool {
	return t == db.TipoJobCopiaNas || t == db.TipoJobCreaCartellaThread
}

// EsecutoreServer prende i job con worker_tipo='server' (scrittura NAS) e li esegue in una goroutine.
type EsecutoreServer struct {
	Pool *pgxpool.Pool
	NAS  *nas.Scrittore
	Log  *slog.Logger
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
			j, err := Claim(ctx, q, db.WorkerTipoServer, id, Destinazione{}, 20*time.Second)
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
			t := Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: id}
			if e.rinviaSeNasAssente(ctx, q, t, j) {
				continue
			}
			res, err := e.esegui(ctx, q, j)
			if err != nil {
				e.Log.Error("job server fallito", "job", j.JobID, "tipo", j.Tipo, "err", err)
				if _, err := Fallisci(ctx, q, t, err.Error(), errors.Is(err, nas.ErrConflitto)); err != nil {
					e.Log.Warn("fallimento non registrato", "job", j.JobID, "err", err)
				}
				continue
			}
			raw, _ := json.Marshal(res)
			if _, err := Completa(ctx, q, t, json.RawMessage(raw)); err != nil {
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
func (e *EsecutoreServer) rinviaSeNasAssente(ctx context.Context, q *db.Queries, t Tentativo, j *db.Job) bool {
	if e.NAS.DryRun || !ScrivePerNas(j.Tipo) || e.NAS.Raggiungibile() {
		return false
	}
	fra := e.RitardoNasAssente
	if fra <= 0 {
		fra = time.Minute
	}
	if _, err := Rinvia(ctx, q, t, fra, "NAS non raggiungibile: tentativo non consumato"); err != nil {
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

func (e *EsecutoreServer) esegui(ctx context.Context, q *db.Queries, j *db.Job) (any, error) {
	switch j.Tipo {
	case db.TipoJobCreaCartellaThread:
		var p api.PayloadCreaCartellaThread
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		t, err := q.GetThread(ctx, p.ThreadID)
		if err != nil {
			return nil, err
		}
		if !t.CartellaRelativa.Valid {
			return nil, fmt.Errorf("thread %s senza cartella_relativa", p.ThreadID)
		}
		layout, err := q.ListCartellaDocumento(ctx)
		if err != nil {
			return nil, err
		}
		// solo le sottocartelle della convenzione (ELENCO DISEGNI, OFFERTE FORNITORI); le altre nascono alla prima copia
		var sotto []string
		for _, l := range layout {
			if l.CreaSempre {
				sotto = append(sotto, l.Sottocartella)
			}
		}
		base, err := e.NAS.CreaCartella(t.CartellaRelativa.String, sotto)
		if err != nil {
			return nil, err
		}
		if !e.NAS.DryRun {
			if err := q.SetCartellaCreata(ctx, p.ThreadID); err != nil {
				return nil, err
			}
		}
		return map[string]any{"cartella": base, "dry_run": e.NAS.DryRun}, nil

	case db.TipoJobCopiaNas:
		var p api.PayloadCopiaNAS
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		d, err := q.GetDocumento(ctx, p.DocumentoID)
		if err != nil {
			return nil, err
		}
		t, err := q.GetThread(ctx, d.ThreadID)
		if err != nil {
			return nil, err
		}
		if !t.CartellaRelativa.Valid {
			return nil, fmt.Errorf("thread %s senza cartella_relativa", d.ThreadID)
		}
		src, err := e.sorgenteStaging(ctx, q, d)
		if err != nil {
			return nil, err
		}
		dst, err := e.NAS.Copia(src, t.CartellaRelativa.String, d.PathRelativo, d.Sha256)
		if err != nil {
			_ = q.SetDocumentoErrore(ctx, db.SetDocumentoErroreParams{DocumentoID: d.DocumentoID, ErroreNas: pgtype.Text{String: err.Error(), Valid: true}})
			return nil, err
		}
		if !e.NAS.DryRun {
			if err := q.SetDocumentoScritto(ctx, d.DocumentoID); err != nil {
				return nil, err
			}
		}
		return map[string]any{"destinazione": dst, "dry_run": e.NAS.DryRun}, nil
	}
	return nil, fmt.Errorf("tipo job non gestito dal server: %s", j.Tipo)
}

// sorgenteStaging trova un allegato del thread con lo stesso hash del documento e un path_staging valido.
func (e *EsecutoreServer) sorgenteStaging(ctx context.Context, q *db.Queries, d db.Documento) (string, error) {
	all, err := q.ListAllegatiThread(ctx, uuid.NullUUID{UUID: d.ThreadID, Valid: true})
	if err != nil {
		return "", err
	}
	for _, a := range all {
		if a.Sha256.Valid && a.Sha256.String == d.Sha256 && a.PathStaging.Valid {
			return a.PathStaging.String, nil
		}
	}
	return "", fmt.Errorf("%w: nessun allegato in staging con sha256 %s", pgx.ErrNoRows, d.Sha256)
}
