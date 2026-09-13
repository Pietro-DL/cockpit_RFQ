// Package jobs gestisce la coda in PostgreSQL: accodamento idempotente, claim con lease,
// scheduler (lease scaduti, sync_outlook periodico) ed esecuzione dei job di tipo 'server'.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// WorkerPer restituisce il worker che esegue un tipo di job.
func WorkerPer(t db.TipoJob) db.WorkerTipo {
	switch t {
	case db.TipoJobSyncOutlook, db.TipoJobStageAllegato, db.TipoJobCreaBozzaOutlook,
		db.TipoJobApriElementoOutlook, db.TipoJobSpostaInCartella, db.TipoJobSegnaLetto:
		return db.WorkerTipoOutlook
	case db.TipoJobAnalizzaAllegato:
		return db.WorkerTipoAnalisi
	default:
		return db.WorkerTipoServer
	}
}

// LeaseSecondi per tipo: i job brevi hanno lease corto, sync e copie NAS più lungo.
func LeaseSecondi(t db.TipoJob) int {
	switch t {
	case db.TipoJobSyncOutlook, db.TipoJobCopiaNas, db.TipoJobBackupDb:
		return 300
	default:
		return 120
	}
}

// Accoda inserisce un job nella transazione corrente. chiave vuota = nessuna idempotenza.
// Restituisce (nil, nil) se un job con la stessa chiave esiste già.
func Accoda(ctx context.Context, q *db.Queries, tipo db.TipoJob, payload any, chiave string, priorita int16) (*db.Job, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload %s: %w", tipo, err)
	}
	var ch pgtype.Text
	if chiave != "" {
		ch = pgtype.Text{String: chiave, Valid: true}
	}
	j, err := q.InsertJob(ctx, db.InsertJobParams{
		Tipo: tipo, WorkerTipo: WorkerPer(tipo), Payload: raw, ChiaveIdempotenza: ch, Priorita: priorita,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // già in coda
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// Claim prova a prendere un job per il worker indicato, con long-poll fino a attesa.
func Claim(ctx context.Context, q *db.Queries, worker db.WorkerTipo, workerID string, attesa time.Duration) (*db.Job, error) {
	scadenza := time.Now().Add(attesa)
	for {
		j, err := q.ClaimJob(ctx, db.ClaimJobParams{LeaseSecondi: 120, WorkerID: pgtype.Text{String: workerID, Valid: true}, WorkerTipo: worker})
		if err == nil {
			// il lease reale dipende dal tipo
			if ls := LeaseSecondi(j.Tipo); ls != 120 {
				_, _ = q.HeartbeatJob(ctx, db.HeartbeatJobParams{LeaseSecondi: int32(ls), JobID: j.JobID, WorkerID: pgtype.Text{String: workerID, Valid: true}})
			}
			return &j, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if time.Now().After(scadenza) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(1 * time.Second):
		}
	}
}

// InJob converte una riga db.Job nel contratto verso il worker.
func InJob(j *db.Job) api.Job {
	return api.Job{JobID: j.JobID, Tipo: string(j.Tipo), Payload: j.Payload, Tentativi: int(j.Tentativi), LeaseS: LeaseSecondi(j.Tipo)}
}

// Scheduler gira nel server: libera i lease scaduti, accoda il sync_outlook periodico, pulisce le sessioni.
type Scheduler struct {
	Q              *db.Queries
	Log            *slog.Logger
	Cartelle       []string
	IntervalloSync time.Duration
	Dal            time.Time
	Lotto          int
}

func (s *Scheduler) Avvia(ctx context.Context) {
	go s.loop(ctx, 30*time.Second, "lease", func(ctx context.Context) error {
		n, err := s.Q.RilasciaLeaseScaduti(ctx)
		if n > 0 {
			s.Log.Warn("lease scaduti rilasciati", "n", n)
		}
		return err
	})
	if s.IntervalloSync > 0 {
		go s.loop(ctx, s.IntervalloSync, "sync_outlook", s.accodaSync)
	}
	go s.loop(ctx, time.Hour, "sessioni", func(ctx context.Context) error {
		_, err := s.Q.EliminaSessioniScadute(ctx)
		return err
	})
}

func (s *Scheduler) loop(ctx context.Context, ogni time.Duration, nome string, f func(context.Context) error) {
	t := time.NewTicker(ogni)
	defer t.Stop()
	if err := f(ctx); err != nil {
		s.Log.Error("scheduler", "task", nome, "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := f(ctx); err != nil {
				s.Log.Error("scheduler", "task", nome, "err", err)
			}
		}
	}
}

// accodaSync crea il job sync_outlook con i cursori correnti. La chiave contiene la finestra temporale:
// se il worker è fermo, in coda resta al più un job per finestra e il claim li consuma in sequenza.
func (s *Scheduler) accodaSync(ctx context.Context) error {
	// se un sync è già in coda o in corso (worker fermo o lento) non se ne accumulano altri
	if pendente, err := s.Q.EsisteJobPronto(ctx, db.TipoJobSyncOutlook); err != nil || pendente {
		return err
	}
	cursori, err := s.Q.ListSyncCursori(ctx)
	if err != nil {
		return err
	}
	perCartella := map[string]*time.Time{}
	var piuVecchio *time.Time
	for _, c := range cursori {
		perCartella[c.Cartella] = c.UltimoReceived
		if c.UltimoReceived != nil {
			if piuVecchio == nil || c.UltimoReceived.Before(*piuVecchio) {
				piuVecchio = c.UltimoReceived
			}
		}
	}
	dal := s.Dal
	if dal.IsZero() {
		if piuVecchio != nil {
			dal = *piuVecchio
		} else {
			dal = time.Now().AddDate(0, -1, 0)
		}
	}
	p := api.PayloadSyncOutlook{Dal: dal, SovrapposizioneS: 600, Lotto: s.Lotto}
	for _, c := range s.Cartelle {
		p.Cartelle = append(p.Cartelle, api.CartellaCursore{Cartella: c, UltimoReceived: perCartella[c]})
	}
	finestra := time.Now().Unix() / int64(s.IntervalloSync.Seconds())
	_, err = Accoda(ctx, s.Q, db.TipoJobSyncOutlook, p, fmt.Sprintf("sync_outlook:%d", finestra), 5)
	return err
}
