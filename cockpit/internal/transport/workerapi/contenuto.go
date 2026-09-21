package workerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"

	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// scaricaContenuto e' GET /api/v1/allegati/{id}/contenuto?job_id=&lease_token=&worker_id= (7C.1, P0).
//
// E' l'upload al contrario. Il file di un allegato sta nello staging del SERVER, e il worker che
// deve analizzarlo puo' stare su un altro PC: un percorso nel payload non gli dice niente, come non
// diceva niente al server il percorso dichiarato dal worker prima della voce 2.3. Il job dice che
// cosa analizzare; i byte se li prende da qui.
//
// Chi puo' scaricare: SOLO il tentativo valido di un job `analizza_allegato` per QUESTO allegato.
// Non basta essere un worker autenticato: un token di worker vale per la coda, non per leggere
// qualunque allegato di qualunque messaggio dell'azienda. Il lease scaduto riceve 409 come per
// l'upload; un job di un altro allegato 422; un contenuto che non c'e' piu' nella cache 410, con
// la stessa frase — «usa Riscarica» — che l'estrazione di un archivio da' nello stesso caso.
//
// Il server non tocca lo stato dell'allegato: se il worker non riesce a prendere il file lo dice
// lui, con un result di errore, e il job fallisce con il motivo visibile. Qui si legge e basta.
func (s *Server) scaricaContenuto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	allegatoID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		errore(w, 400, fmt.Errorf("allegato: %w", err))
		return
	}
	par := r.URL.Query()
	jobID, err := strconv.ParseInt(par.Get("job_id"), 10, 64)
	if err != nil {
		errore(w, 400, fmt.Errorf("job_id mancante o non valido"))
		return
	}
	if _, ok := stessoWorker(w, r, par.Get("worker_id")); !ok {
		return
	}
	t, err := tentativo(jobID, par.Get("worker_id"), par.Get("lease_token"))
	if err != nil {
		s.Log.Warn("download di un contenuto senza tentativo dichiarato: rifiutato",
			"job", jobID, "allegato", allegatoID, "err", err)
		errore(w, 400, err)
		return
	}
	q := db.New(s.Pool)
	j, err := jobs.Verifica(ctx, q, t)
	if errors.Is(err, jobs.ErrTentativoNonValido) {
		s.nonValido(w, ctx, jobID)
		return
	}
	if err != nil {
		errore(w, 500, err)
		return
	}
	a, err := s.analisiDelJob(ctx, q, &j, allegatoID)
	if err != nil {
		errore(w, 422, err)
		return
	}
	if !a.PathStaging.Valid || !(jobs.FileStaging{}).Presente(a.PathStaging.String) {
		// Non e' un guasto da ritentare: il file e' sparito dalla cache fra il download e adesso.
		// Il worker lo riporta come errore definitivo e l'allegato va in errore con questo motivo.
		errore(w, http.StatusGone, fmt.Errorf("contenuto di %q non presente in staging: usa Riscarica", a.NomeFile))
		return
	}
	f, err := os.Open(a.PathStaging.String)
	if err != nil {
		errore(w, 500, fmt.Errorf("apertura del contenuto: %w", err))
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		errore(w, 500, fmt.Errorf("contenuto non leggibile"))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if a.Sha256.Valid {
		w.Header().Set("X-Cockpit-Sha256", a.Sha256.String)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(a.NomeFile)))
	w.WriteHeader(http.StatusOK)
	n, err := io.Copy(w, f)
	if err != nil {
		// il worker se ne accorge da solo: Content-Length non torna, oppure lo sha256 non torna
		s.Log.Warn("download del contenuto interrotto", "job", jobID, "allegato", allegatoID, "byte", n, "err", err)
		return
	}
	s.Log.Info("contenuto consegnato al worker", "job", jobID, "allegato", allegatoID, "byte", n, "worker", t.WorkerID)
}

// analisiDelJob verifica che il job sia l'analisi di QUESTO allegato e restituisce l'allegato.
// Un tentativo valido di un altro job non legge contenuti che non sono i suoi.
func (s *Server) analisiDelJob(ctx context.Context, q *db.Queries, j *db.Job, allegatoID uuid.UUID) (db.Allegato, error) {
	if j.Tipo != db.TipoJobAnalizzaAllegato {
		return db.Allegato{}, fmt.Errorf("il job %d è %s, non un'analisi di allegato", j.JobID, j.Tipo)
	}
	p, err := payloadAnalisi(j)
	if err != nil {
		return db.Allegato{}, err
	}
	if p.AllegatoID != allegatoID {
		return db.Allegato{}, fmt.Errorf("il job %d analizza l'allegato %s, non %s", j.JobID, p.AllegatoID, allegatoID)
	}
	a, err := q.GetAllegato(ctx, allegatoID)
	if err != nil {
		return db.Allegato{}, fmt.Errorf("allegato %s: %w", allegatoID, err)
	}
	return a, nil
}

func payloadAnalisi(j *db.Job) (worker.PayloadAnalizzaAllegato, error) {
	var p worker.PayloadAnalizzaAllegato
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return p, fmt.Errorf("payload del job %d: %w", j.JobID, err)
	}
	return p, nil
}
