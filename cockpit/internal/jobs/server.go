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

// EsecutoreServer prende i job con worker_tipo='server' (scrittura NAS) e li esegue in una goroutine.
type EsecutoreServer struct {
	Pool *pgxpool.Pool
	NAS  *nas.Scrittore
	Log  *slog.Logger
}

func (e *EsecutoreServer) Avvia(ctx context.Context) {
	go func() {
		q := db.New(e.Pool)
		id := "server"
		for ctx.Err() == nil {
			j, err := Claim(ctx, q, db.WorkerTipoServer, id, 20*time.Second)
			if err != nil {
				e.Log.Error("claim server", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if j == nil {
				continue
			}
			res, err := e.esegui(ctx, q, j)
			if err != nil {
				e.Log.Error("job server fallito", "job", j.JobID, "tipo", j.Tipo, "err", err)
				_, _ = q.FallisciJob(ctx, db.FallisciJobParams{Definitivo: errors.Is(err, nas.ErrConflitto), Errore: pgtype.Text{String: err.Error(), Valid: true}, JobID: j.JobID})
				continue
			}
			raw, _ := json.Marshal(res)
			ris := json.RawMessage(raw)
			if _, err := q.CompletaJob(ctx, db.CompletaJobParams{JobID: j.JobID, Risultato: &ris}); err != nil {
				e.Log.Error("completa job", "job", j.JobID, "err", err)
			}
		}
	}()
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
		var sotto []string
		for _, l := range layout {
			sotto = append(sotto, l.Sottocartella)
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
