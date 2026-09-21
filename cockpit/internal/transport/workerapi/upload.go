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
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// caricaFile è PUT /api/v1/allegati/{id}/file?job_id=&lease_token=&worker_id= (voce 2.3).
//
// Il corpo è il file, così com'è. Il server lo scrive in <staging>\_parti, legato al tentativo, e risponde
// 204: NON è ancora l'allegato. Lo diventa solo con il result valido dello stesso tentativo, che ne
// verifica lo sha256 e lo rinomina. Perciò questo gestore non tocca né path_staging né lo stato
// dell'allegato: un tentativo può caricare anche tre volte, e finché non chiude con un result
// valido non ha consegnato niente.
//
// Il tentativo si verifica due volte: prima di aprire il file — un tentativo che non vale già più
// non deve nemmeno cominciare a scrivere — e dopo averlo chiuso. La seconda è quella che conta per
// M13: un lease scaduto a metà trasferimento riceve 409 e il suo .parte viene rimosso, mentre il
// tentativo che gli è subentrato ha già il suo file con il suo token e non se ne accorge.
//
// Fra le due verifiche non c'è nessuna transazione aperta: un trasferimento dura quanto dura, e una
// transazione lunga quanto un upload è esattamente il difetto che la voce 1.4 ha tolto dagli zip.
func (s *Server) caricaFile(w http.ResponseWriter, r *http.Request) {
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
		s.Log.Warn("upload senza tentativo dichiarato: il file non viene accettato",
			"job", jobID, "allegato", allegatoID, "err", err)
		errore(w, 400, err)
		return
	}
	q := db.New(s.Pool)

	// 1. il tentativo deve essere valido PRIMA di scrivere un solo byte
	j, err := jobs.Verifica(ctx, q, t)
	if errors.Is(err, jobs.ErrTentativoNonValido) {
		// il file di questo token, se c'è, non sarà promosso da nessuno: via subito
		if parte := s.parteDi(ctx, q, jobID, allegatoID, t.LeaseToken); parte != "" {
			jobs.RimuoviParte(parte)
		}
		s.nonValido(w, ctx, jobID)
		return
	}
	if err != nil {
		errore(w, 500, err)
		return
	}
	// 2. il tentativo deve essere QUELLO di questo allegato: un tentativo valido di un altro job
	// non può depositare file a nome di un download che non è il suo
	_, a, err := s.stageDelJob(ctx, q, &j, allegatoID)
	if err != nil {
		errore(w, 422, err)
		return
	}
	// Dove finira' il file non si sa ancora: lo dira' il suo sha256, e lo sha256 si conosce quando il
	// trasferimento e' finito. Qui si sa soltanto CHI sta caricando e DENTRO QUALE TENTATIVO, e tanto
	// basta per dargli un posto suo in _parti.
	parte := jobs.PercorsoParte(s.Staging, allegatoID, t.LeaseToken)

	// 3. il limite si applica prima di leggere: con Content-Length dichiarato non si trasferisce
	// niente, senza si legge fino al limite e poi si scarta (M8)
	if r.ContentLength > s.maxUpload() {
		s.oltreIlLimite(ctx, q, a, r.ContentLength)
		errore(w, http.StatusRequestEntityTooLarge, s.erroreLimite(r.ContentLength))
		return
	}
	if err := os.MkdirAll(filepath.Dir(parte), 0o755); err != nil {
		errore(w, 500, err)
		return
	}
	f, err := os.Create(parte)
	if err != nil {
		errore(w, 500, err)
		return
	}
	n, err := io.Copy(f, http.MaxBytesReader(w, r.Body, s.maxUpload()))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		jobs.RimuoviParte(parte)
		var troppo *http.MaxBytesError
		if errors.As(err, &troppo) {
			s.oltreIlLimite(ctx, q, a, -1)
			errore(w, http.StatusRequestEntityTooLarge, s.erroreLimite(-1))
			return
		}
		// il trasferimento si è interrotto: 5xx dice al worker di riprovare l'upload tale e quale
		s.Log.Warn("upload interrotto", "job", jobID, "allegato", allegatoID, "byte", n, "err", err)
		errore(w, 500, fmt.Errorf("trasferimento interrotto dopo %d byte: %w", n, err))
		return
	}

	// 4. il tentativo deve essere ancora valido DOPO: è la verifica che chiude M13
	if _, err := jobs.Verifica(ctx, q, t); err != nil {
		jobs.RimuoviParte(parte)
		if errors.Is(err, jobs.ErrTentativoNonValido) {
			s.Log.Warn("upload di un tentativo scaduto durante il trasferimento: file rimosso",
				"job", jobID, "allegato", allegatoID, "byte", n)
			s.nonValido(w, ctx, jobID)
			return
		}
		errore(w, 500, err)
		return
	}
	s.Log.Info("file caricato", "job", jobID, "allegato", allegatoID, "byte", n, "file", filepath.Base(parte))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) maxUpload() int64 {
	if s.MaxUpload > 0 {
		return s.MaxUpload
	}
	return 64 << 20
}

func (s *Server) erroreLimite(dichiarati int64) error {
	if dichiarati > 0 {
		return fmt.Errorf("file di %d MB oltre il limite di upload di %d MB (max_upload_mb)", dichiarati>>20, s.maxUpload()>>20)
	}
	return fmt.Errorf("file oltre il limite di upload di %d MB (max_upload_mb)", s.maxUpload()>>20)
}

// oltreIlLimite rende visibile il motivo sull'allegato (M8: «stato errore visibile»). Non chiude il
// job: lo farà il worker con un result di errore definitivo, oppure la scadenza del lease. Scrivere
// lo stato qui è lecito perché il tentativo è stato verificato un momento fa.
func (s *Server) oltreIlLimite(ctx context.Context, q *db.Queries, a db.Allegato, dichiarati int64) {
	msg := s.erroreLimite(dichiarati).Error()
	if err := q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoErrore, Errore: pgtype.Text{String: msg, Valid: true}}); err != nil {
		s.Log.Warn("stato allegato oltre il limite non scritto", "allegato", a.AllegatoID, "err", err)
	}
	s.Log.Warn("upload rifiutato", "allegato", a.AllegatoID, "err", msg)
}

// stageDelJob legge il payload del job e verifica che sia il download di QUESTO allegato.
func (s *Server) stageDelJob(ctx context.Context, q *db.Queries, j *db.Job, allegatoID uuid.UUID) (worker.PayloadStageAllegato, db.Allegato, error) {
	var p worker.PayloadStageAllegato
	if j.Tipo != db.TipoJobStageAllegato {
		return p, db.Allegato{}, fmt.Errorf("il job %d è %s, non un download di allegato", j.JobID, j.Tipo)
	}
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return p, db.Allegato{}, fmt.Errorf("payload del job %d: %w", j.JobID, err)
	}
	if p.AllegatoID != allegatoID {
		return p, db.Allegato{}, fmt.Errorf("il job %d scarica l'allegato %s, non %s", j.JobID, p.AllegatoID, allegatoID)
	}
	a, err := q.GetAllegato(ctx, allegatoID)
	if err != nil {
		return p, db.Allegato{}, fmt.Errorf("allegato %s: %w", allegatoID, err)
	}
	return p, a, nil
}

// parteDi è il percorso del .parte di un token per un job che non vale più, così da poterlo rimuovere.
// Vuoto se il job non è quello di questo allegato: allora quel file non l'ha scritto questo
// tentativo, e non tocca a noi toglierlo.
func (s *Server) parteDi(ctx context.Context, q *db.Queries, jobID int64, allegatoID, token uuid.UUID) string {
	j, err := q.GetJob(ctx, jobID)
	if err != nil {
		return ""
	}
	if _, _, err := s.stageDelJob(ctx, q, &j, allegatoID); err != nil {
		return ""
	}
	return jobs.PercorsoParte(s.Staging, allegatoID, token)
}
