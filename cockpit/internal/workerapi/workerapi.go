// Package workerapi espone su loopback le rotte usate dai worker Python: coda job e ingest.
// Autenticazione: header X-Cockpit-Token uguale a [server].token_worker.
package workerapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/jobs"
)

type Server struct {
	Pool    *pgxpool.Pool
	Log     *slog.Logger
	Token   string
	Ingest  *ingest.Servizio
	Staging string
}

func (s *Server) Registra(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/claim", s.auth(s.claim))
	mux.HandleFunc("POST /api/v1/jobs/{id}/heartbeat", s.auth(s.heartbeat))
	mux.HandleFunc("POST /api/v1/jobs/{id}/result", s.auth(s.result))
	mux.HandleFunc("POST /api/v1/ingest/messaggi", s.auth(s.ingest))
	mux.HandleFunc("GET /api/v1/sync/cursori", s.auth(s.cursori))
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.Header.Get("X-Cockpit-Token")
		if t == "" || subtle.ConstantTimeCompare([]byte(t), []byte(s.Token)) != 1 {
			http.Error(w, `{"errore":"token worker non valido"}`, http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func scriviJSON(w http.ResponseWriter, stato int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(stato)
	_ = json.NewEncoder(w).Encode(v)
}

func errore(w http.ResponseWriter, stato int, err error) {
	scriviJSON(w, stato, map[string]string{"errore": err.Error()})
}

func leggi(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<20))
	return dec.Decode(v)
}

// ---------------------------------------------------------------- coda

func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var req api.ClaimRichiesta
	if err := leggi(r, &req); err != nil {
		errore(w, 400, err)
		return
	}
	wt := db.WorkerTipo(req.Worker)
	if !wt.Valid() || wt == db.WorkerTipoServer {
		errore(w, 400, fmt.Errorf("worker non valido: %q", req.Worker))
		return
	}
	attesa := time.Duration(req.AttesaS) * time.Second
	if attesa <= 0 || attesa > 25*time.Second {
		attesa = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), attesa+5*time.Second)
	defer cancel()
	j, err := jobs.Claim(ctx, db.New(s.Pool), wt, req.WorkerID, attesa)
	if err != nil {
		errore(w, 500, err)
		return
	}
	if j == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	scriviJSON(w, 200, jobs.InJob(j))
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errore(w, 400, err)
		return
	}
	var req api.HeartbeatRichiesta
	_ = leggi(r, &req)
	q := db.New(s.Pool)
	j, err := q.GetJob(r.Context(), id)
	if err != nil {
		errore(w, 404, err)
		return
	}
	n, err := q.HeartbeatJob(r.Context(), db.HeartbeatJobParams{LeaseSecondi: int32(jobs.LeaseSecondi(j.Tipo)), JobID: id, WorkerID: pgtype.Text{String: req.WorkerID, Valid: true}})
	if err != nil {
		errore(w, 500, err)
		return
	}
	if n == 0 {
		errore(w, 409, errors.New("job non in corso per questo worker (lease perso?)"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) result(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errore(w, 400, err)
		return
	}
	var req api.RisultatoRichiesta
	if err := leggi(r, &req); err != nil {
		errore(w, 400, err)
		return
	}
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		errore(w, 500, err)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	j, err := q.GetJob(ctx, id)
	if err != nil {
		errore(w, 404, err)
		return
	}
	if j.Stato != db.StatoJobInCorso {
		errore(w, 409, fmt.Errorf("job %d in stato %s", id, j.Stato))
		return
	}
	if req.Esito != "ok" {
		msg := req.Errore
		if msg == "" {
			msg = "errore non specificato"
		}
		if req.Definitivo {
			s.fallimentoDefinitivo(ctx, q, &j, msg)
		}
		if _, err := q.FallisciJob(ctx, db.FallisciJobParams{Definitivo: req.Definitivo, Errore: pgtype.Text{String: msg, Valid: true}, JobID: id}); err != nil {
			errore(w, 500, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			errore(w, 500, err)
			return
		}
		s.Log.Warn("job fallito", "job", id, "tipo", j.Tipo, "tentativi", j.Tentativi, "definitivo", req.Definitivo, "err", msg)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.applicaRisultato(ctx, q, &j, req.Dati); err != nil {
		errore(w, 422, fmt.Errorf("risultato %s non applicabile: %w", j.Tipo, err))
		return
	}
	dati := req.Dati
	if len(dati) == 0 {
		dati = json.RawMessage("{}")
	}
	if _, err := q.CompletaJob(ctx, db.CompletaJobParams{JobID: id, Risultato: &dati}); err != nil {
		errore(w, 500, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errore(w, 500, err)
		return
	}
	s.Log.Info("job fatto", "job", id, "tipo", j.Tipo)
	w.WriteHeader(http.StatusNoContent)
}

// fallimentoDefinitivo scrive gli effetti di un errore non recuperabile sull'entità del job.
func (s *Server) fallimentoDefinitivo(ctx context.Context, q *db.Queries, j *db.Job, msg string) {
	switch j.Tipo {
	case db.TipoJobStageAllegato:
		var p api.PayloadStageAllegato
		if json.Unmarshal(j.Payload, &p) == nil {
			_ = q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: p.AllegatoID, Stato: db.StatoAllegatoErrore, Errore: pgtype.Text{String: msg, Valid: true}})
		}
	case db.TipoJobAnalizzaAllegato:
		var p api.PayloadAnalizzaAllegato
		if json.Unmarshal(j.Payload, &p) == nil {
			_ = q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: p.AllegatoID, Stato: db.StatoAllegatoErrore, Errore: pgtype.Text{String: msg, Valid: true}})
		}
	case db.TipoJobCreaBozzaOutlook:
		var p api.PayloadCreaBozza
		if json.Unmarshal(j.Payload, &p) == nil {
			_ = q.SetBozzaErrore(ctx, db.SetBozzaErroreParams{BozzaID: p.BozzaID, Errore: pgtype.Text{String: msg, Valid: true}})
		}
	}
}

// applicaRisultato scrive nel DB gli effetti di un job riuscito.
func (s *Server) applicaRisultato(ctx context.Context, q *db.Queries, j *db.Job, dati json.RawMessage) error {
	switch j.Tipo {
	case db.TipoJobSyncOutlook:
		var r api.RisultatoSync
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		for _, c := range r.Cartelle {
			if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{Cartella: c.Cartella, UltimoReceived: c.UltimoReceived, NMessaggi: int32(c.NMessaggi), Errore: txt(c.Errore)}); err != nil {
				return err
			}
		}
		return nil

	case db.TipoJobStageAllegato:
		var r api.RisultatoStage
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		if r.Sha256 == "" || r.PathStaging == "" {
			return errors.New("sha256 e path_staging obbligatori")
		}
		if !filepath.IsAbs(r.PathStaging) || !strings.HasPrefix(strings.ToLower(filepath.Clean(r.PathStaging)), strings.ToLower(filepath.Clean(s.Staging))) {
			return fmt.Errorf("path_staging fuori dalla cartella di staging: %s", r.PathStaging)
		}
		if err := q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: r.AllegatoID, PathStaging: txt(r.PathStaging), Sha256: txt(r.Sha256), Bytes: pgtype.Int8{Int64: r.Bytes, Valid: true}}); err != nil {
			return err
		}
		return s.dopoStaging(ctx, q, r)

	case db.TipoJobAnalizzaAllegato:
		var r api.RisultatoAnalisi
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		var threadID uuid.NullUUID
		a, err := q.GetAllegato(ctx, r.AllegatoID)
		if err == nil {
			m, err := q.GetMessaggio(ctx, a.MessaggioID)
			if err == nil && m.ThreadID.Valid {
				threadID = m.ThreadID
			}
		}
		codice := r.Codice
		rev := r.Rev
		tipo := db.TipoDocumento(r.TipoProposto)
		if tipo == db.TipoDocumentoOffertaPromatec {
			codice = ""
			rev = ""
		}
		dett := r.Dettagli
		if len(dett) == 0 {
			dett = json.RawMessage("{}")
		}
		if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
			AllegatoID:   r.AllegatoID,
			ThreadID:     threadID,
			TipoProposto: tipo,
			Codice:       txt(codice),
			Rev:          txt(rev),
			Confidenza:   int16(r.Confidenza),
			Fonte:        db.FonteProposta(r.Fonte),
			Dettagli:     dett,
		}); err != nil {
			return fmt.Errorf("upsert proposta da analisi: %w", err)
		}
		return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{
			AllegatoID: r.AllegatoID,
			Stato:      db.StatoAllegatoAnalizzato,
		})

	case db.TipoJobCreaBozzaOutlook:
		var p api.PayloadCreaBozza
		var r api.RisultatoBozza
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return err
		}
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		return q.SetBozzaAperta(ctx, db.SetBozzaApertaParams{BozzaID: p.BozzaID, EntryID: txt(r.EntryID)})
	}
	return nil // apri_elemento, sposta, segna_letto: nessun effetto persistente (il prossimo sync riallinea)
}

// dopoStaging: prima INTERPRETAZIONE economica lato server (estensione + nome file + rumore),
// poi accoda l'analisi Python che la raffina (STEP, cartiglio, regole cliente) finché la proposta è aperta.
func (s *Server) dopoStaging(ctx context.Context, q *db.Queries, r api.RisultatoStage) error {
	a, err := q.GetAllegato(ctx, r.AllegatoID)
	if err != nil {
		return err
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		return err
	}
	dominio := ""
	if m.MittenteIndirizzo.Valid {
		if i := strings.LastIndex(m.MittenteIndirizzo.String, "@"); i >= 0 {
			dominio = m.MittenteIndirizzo.String[i+1:]
		}
	}
	ext := strings.ToLower(a.Estensione.String)
	tipo, fonte, conf := propostaDaEstensione(ext)
	codice, rev := "", ""
	if c, rv := domain.CodiceRev(strings.TrimSuffix(a.NomeFile, filepath.Ext(a.NomeFile))); len(domain.EstraiCodici(c)) > 0 {
		codice, rev = c, rv
		if tipo == db.TipoDocumentoDisegno2d || tipo == db.TipoDocumentoCad3d || tipo == db.TipoDocumentoSviluppoDxf {
			conf += 20
			fonte = db.FontePropostaNomeFile
		}
	} else if tipo == db.TipoDocumentoDisegno2d && ext == "pdf" {
		tipo, conf = db.TipoDocumentoAltro, 30
	}
	// rumore: immagini piccole, o hash già scartato / visto ≥ 3 volte dallo stesso dominio
	if dominio != "" {
		if seen, _ := q.IsHashRumore(ctx, db.IsHashRumoreParams{Sha256: r.Sha256, Lower: dominio}); seen {
			tipo, fonte, conf = db.TipoDocumentoRumore, db.FontePropostaRumore, 95
		} else if n, _ := q.ContaHashVisto(ctx, db.ContaHashVistoParams{Sha256: txt(r.Sha256), Lower: dominio}); n >= 3 && estImmagine(ext) {
			tipo, fonte, conf = db.TipoDocumentoRumore, db.FontePropostaRumore, 85
		}
	}
	if estImmagine(ext) && r.Bytes < 100*1024 && tipo != db.TipoDocumentoRumore {
		tipo, fonte, conf = db.TipoDocumentoRumore, db.FontePropostaRumore, 70
	}
	// offerta Promatec in uscita: "SO 5467.pdf"
	if m.Direzione == db.DirezioneUscita && ext == "pdf" && strings.HasPrefix(strings.ToUpper(a.NomeFile), "SO ") {
		tipo, fonte, conf, codice, rev = db.TipoDocumentoOffertaPromatec, db.FontePropostaDirezione, 90, "", ""
	}
	if conf > 100 {
		conf = 100
	}
	dett, _ := json.Marshal(map[string]any{"estensione": ext, "bytes": r.Bytes})
	if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
		AllegatoID: a.AllegatoID, ThreadID: m.ThreadID, TipoProposto: tipo, Codice: txt(codice), Rev: txt(rev),
		Confidenza: int16(conf), Fonte: fonte, Dettagli: dett,
	}); err != nil {
		return fmt.Errorf("proposta: %w", err)
	}
	if tipo == db.TipoDocumentoRumore {
		return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
	}
	_, err = jobs.Accoda(ctx, q, db.TipoJobAnalizzaAllegato, map[string]any{
		"allegato_id": a.AllegatoID, "path_staging": r.PathStaging, "sha256": r.Sha256, "nome_file": a.NomeFile,
		"thread_id": nullUUID(m.ThreadID), "messaggio_id": m.MessaggioID,
	}, "analizza:"+a.AllegatoID.String(), 6)
	return err
}

func propostaDaEstensione(ext string) (db.TipoDocumento, db.FonteProposta, int) {
	switch ext {
	case "stp", "step", "sldprt", "sldasm", "igs", "iges", "x_t", "x_b", "prt", "par", "asm":
		return db.TipoDocumentoCad3d, db.FontePropostaEstensione, 70
	case "dxf":
		return db.TipoDocumentoSviluppoDxf, db.FontePropostaEstensione, 70
	case "dwg", "tif", "tiff", "pdf":
		return db.TipoDocumentoDisegno2d, db.FontePropostaEstensione, 50
	case "xls", "xlsx", "csv":
		return db.TipoDocumentoCommerciale, db.FontePropostaEstensione, 50
	case "zip", "7z", "rar":
		return db.TipoDocumentoAltro, db.FontePropostaEstensione, 20 // il worker-analisi lo appiattisce
	case "msg", "eml":
		return db.TipoDocumentoCorrispondenza, db.FontePropostaEstensione, 60
	}
	return db.TipoDocumentoAltro, db.FontePropostaEstensione, 20
}

func estImmagine(ext string) bool {
	switch ext {
	case "png", "jpg", "jpeg", "gif", "bmp", "svg", "webp", "ico":
		return true
	}
	return false
}

func nullUUID(u uuid.NullUUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	return &u.UUID
}

func txt(s string) pgtype.Text {
	if strings.TrimSpace(s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ---------------------------------------------------------------- ingest e cursori

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var req api.IngestRichiesta
	if err := leggi(r, &req); err != nil {
		errore(w, 400, err)
		return
	}
	if len(req.Messaggi) == 0 {
		scriviJSON(w, 200, api.IngestRisposta{Esiti: []api.EsitoMessaggio{}})
		return
	}
	res, err := s.Ingest.Ingerisci(r.Context(), req.Messaggi)
	if err != nil {
		errore(w, 422, err)
		return
	}
	if res.Esiti == nil {
		res.Esiti = []api.EsitoMessaggio{}
	}
	scriviJSON(w, 200, res)
}

func (s *Server) cursori(w http.ResponseWriter, r *http.Request) {
	c, err := db.New(s.Pool).ListSyncCursori(r.Context())
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		errore(w, 500, err)
		return
	}
	out := make([]api.CartellaCursore, 0, len(c))
	for _, x := range c {
		out = append(out, api.CartellaCursore{Cartella: x.Cartella, UltimoReceived: x.UltimoReceived})
	}
	scriviJSON(w, 200, out)
}
