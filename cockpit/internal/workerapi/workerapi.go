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
	"promatec/cockpit/internal/archivio"
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
	// CasellaDefault: indirizzo attribuito ai lotti che non dichiarano una casella. Serve finché il
	// worker non manda sempre casella_id (fase 2); una casella sbagliata qui è meglio di una assente,
	// perché almeno è dichiarata e verificabile.
	CasellaDefault string
	// Analizzatore: versione e configurazione con cui si chiedono le analisi (voce 1.12).
	Analizzatore jobs.Analizzatore
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
	// presenza: la UI mostra "OFFLINE" se un worker non fa claim da più di un minuto
	if err := db.New(s.Pool).UpsertWorkerPresenza(r.Context(), db.UpsertWorkerPresenzaParams{WorkerTipo: wt, WorkerID: req.WorkerID, ConJob: j != nil}); err != nil {
		s.Log.Warn("worker_presenza", "err", err)
	}
	if j == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	scriviJSON(w, 200, jobs.InJob(j))
}

// tentativo estrae dalla richiesta l'identità del tentativo. Senza token non si prosegue: accettare
// una scrittura «di qualcuno che dice di essere il worker» vanificherebbe tutto il resto.
func tentativo(jobID int64, workerID, token string) (jobs.Tentativo, error) {
	if workerID == "" {
		return jobs.Tentativo{}, errors.New("worker_id mancante")
	}
	t, err := uuid.Parse(strings.TrimSpace(token))
	if err != nil {
		return jobs.Tentativo{}, fmt.Errorf("lease_token mancante o non valido: %w", err)
	}
	return jobs.Tentativo{JobID: jobID, LeaseToken: t, WorkerID: workerID}, nil
}

// nonValido risponde 409 e, se il tentativo è caduto per durata massima, riporta il job a 'pronto'
// con il motivo scritto: altrimenti resterebbe «in corso» senza che nessuno lo stia eseguendo.
func (s *Server) nonValido(w http.ResponseWriter, ctx context.Context, jobID int64) {
	if jobs.ChiudiSeDurataSuperata(ctx, db.New(s.Pool), jobID) {
		s.Log.Warn("tentativo oltre la durata massima: job riaccodato", "job", jobID)
		errore(w, 409, errors.New("durata massima superata: il tentativo non vale più"))
		return
	}
	errore(w, 409, errors.New("tentativo non più valido (lease perso o job ripreso da un altro tentativo)"))
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errore(w, 400, err)
		return
	}
	var req api.HeartbeatRichiesta
	_ = leggi(r, &req)
	t, err := tentativo(id, req.WorkerID, req.LeaseToken)
	if err != nil {
		errore(w, 400, err)
		return
	}
	switch err := jobs.Batte(r.Context(), db.New(s.Pool), t); {
	case errors.Is(err, jobs.ErrTentativoNonValido):
		s.nonValido(w, r.Context(), id)
	case err != nil:
		errore(w, 500, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
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
	t, err := tentativo(id, req.WorkerID, req.LeaseToken)
	if err != nil {
		errore(w, 400, err)
		return
	}
	ctx := r.Context()
	// Il job si legge PRIMA di aprire la transazione, perché l'eventuale estrazione di uno zip va
	// fatta fuori (voce 1.4). La lettura non ha bisogno di essere nella stessa transazione delle
	// scritture: ciò che protegge il job è il predicato del tentativo, verificato al momento di
	// scrivere, non l'istante in cui si è letta la riga.
	j, err := db.New(s.Pool).GetJob(ctx, id)
	if err != nil {
		errore(w, 404, err)
		return
	}
	est, err := s.estraiFuoriTransazione(ctx, &j, req)
	if err != nil {
		errore(w, 500, err)
		return
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		errore(w, 500, err)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	// --------------------------------------------------- il worker riporta un errore
	if req.Esito != "ok" {
		msg := req.Errore
		if msg == "" {
			msg = "errore non specificato"
		}
		// Prima si verifica il tentativo, poi si tocca l'entità del job: un fallimento riportato da un
		// tentativo scaduto non deve marcare in errore un allegato che il tentativo nuovo sta
		// scaricando bene (Q19).
		if _, err := jobs.Fallisci(ctx, q, t, msg, req.Definitivo); err != nil {
			if errors.Is(err, jobs.ErrTentativoNonValido) {
				s.nonValido(w, ctx, id)
				return
			}
			errore(w, 500, err)
			return
		}
		if req.Definitivo {
			s.fallimentoDefinitivo(ctx, q, &j, msg)
		}
		if err := tx.Commit(ctx); err != nil {
			errore(w, 500, err)
			return
		}
		s.Log.Warn("job fallito", "job", id, "tipo", j.Tipo, "tentativi", j.Tentativi, "definitivo", req.Definitivo, "err", msg)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// --------------------------------------------------- il worker riporta un successo
	if err := s.applicaRisultato(ctx, q, &j, req.Dati, &est); err != nil {
		// Il risultato non è applicabile: il contenuto è sbagliato, non la rete. Ritentarlo darebbe lo
		// stesso esito all'infinito. Si annulla la transazione e si chiude il job in una nuova (N6):
		// senza questo il job restava «in corso» fino alla scadenza del lease, e l'operatore non
		// vedeva nessun motivo.
		_ = tx.Rollback(ctx)
		motivo := fmt.Sprintf("risultato %s non applicabile: %v", j.Tipo, err)
		fuori := db.New(s.Pool)
		if _, e := jobs.Fallisci(ctx, fuori, t, motivo, true); e != nil && !errors.Is(e, jobs.ErrTentativoNonValido) {
			s.Log.Error("fallimento dopo 422 non registrato", "job", id, "err", e)
		} else if e == nil {
			s.fallimentoDefinitivo(ctx, fuori, &j, motivo)
		}
		s.Log.Warn("risultato non applicabile", "job", id, "tipo", j.Tipo, "err", err)
		errore(w, 422, errors.New(motivo))
		return
	}
	dati := req.Dati
	if len(dati) == 0 {
		dati = json.RawMessage("{}")
	}
	if _, err := jobs.Completa(ctx, q, t, dati); err != nil {
		if errors.Is(err, jobs.ErrTentativoNonValido) {
			// tutto ciò che applicaRisultato ha scritto sparisce con il rollback: «nulla applicato»
			// non è una promessa, è la transazione (Q15, Q20)
			s.nonValido(w, ctx, id)
			return
		}
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

// estrazione è il risultato del lavoro su disco fatto PRIMA di aprire la transazione (voce 1.4, G1).
//
// Estrarre uno zip dentro la transazione del risultato era il difetto: un archivio da qualche centinaio
// di megabyte tiene aperta una transazione per tutto il tempo della scrittura su disco, con la riga del
// job bloccata e lo snapshot fermo. In una coda che ha già i suoi vincoli di lease, una transazione
// lunga quanto un'operazione di I/O è il modo più semplice per far scadere il tentativo che la sta
// eseguendo. Ora l'estrazione avviene fuori: la transazione contiene solo scritture in database e dura
// quanto quelle.
//
// Un'estrazione ripetuta non fa danno: le voci finiscono nella stessa cartella con gli stessi nomi e
// gli stessi hash, e le righe si riscrivono per upsert. È la condizione che rende sicuro farla fuori
// dalla transazione che poi potrebbe non essere confermata.
type estrazione struct {
	fatta    bool            // false = questo risultato non richiedeva nessuna estrazione
	voci     []archivio.Voce //
	troncato bool            // l'archivio ha superato i limiti: le voci sono quelle entrate
	errore   error           // zip illeggibile: l'allegato va in errore, il job no
}

// estraiFuoriTransazione riconosce il solo caso che richiede lavoro su disco — un allegato .zip appena
// messo in staging — e lo svolge. Per tutti gli altri risultati non fa niente.
func (s *Server) estraiFuoriTransazione(ctx context.Context, j *db.Job, req api.RisultatoRichiesta) (estrazione, error) {
	var est estrazione
	if j.Tipo != db.TipoJobStageAllegato || req.Esito != "ok" {
		return est, nil
	}
	var r api.RisultatoStage
	if err := json.Unmarshal(req.Dati, &r); err != nil || r.PathStaging == "" {
		return est, nil // il risultato è malformato: lo dirà applicaRisultato, con il suo 422
	}
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, r.AllegatoID)
	if err != nil {
		return est, nil // idem: l'errore va riportato dove si applica il risultato, non qui
	}
	if strings.ToLower(a.Estensione.String) != "zip" {
		return est, nil
	}
	dest := filepath.Join(filepath.Dir(r.PathStaging), fmt.Sprintf("%02d_zip", a.Indice))
	voci, err := archivio.Estrai(r.PathStaging, dest)
	est.fatta = true
	est.voci = voci
	switch {
	case errors.Is(err, archivio.ErrLimite):
		est.troncato = true
	case err != nil:
		est.errore = err
	}
	return est, nil
}

// enumValido converte una stringa del contratto in un valore dell'enum del database, rifiutando ciò
// che l'enum non prevede (N7, voce 1.3).
//
// Prima questa conversione era un cast e basta. Un `tipo_proposto: "boh"` arrivava così com'era fino
// a PostgreSQL, che lo rifiutava con «invalid input value for enum»: il comportamento finale era
// giusto per caso, ma il motivo era illeggibile e — cosa peggiore — dipendeva dal fatto che la
// colonna fosse davvero un enum. Il giorno in cui diventasse `text`, il valore inventato entrerebbe
// in database senza che nessuno se ne accorga.
//
// Non esiste un valore «più vicino» a cui ricondurre un termine fuori enum: sarebbe un dato inventato
// dal server. Il risultato non è applicabile, quindi il job fallisce in modo definitivo (Q9):
// ritentarlo darebbe all'infinito lo stesso esito.
func enumValido[T interface {
	~string
	Valid() bool
}](campo, v string) (T, error) {
	e := T(v)
	if !e.Valid() {
		return "", fmt.Errorf("%s fuori enum: %q", campo, v)
	}
	return e, nil
}

// applicaRisultato scrive nel DB gli effetti di un job riuscito.
func (s *Server) applicaRisultato(ctx context.Context, q *db.Queries, j *db.Job, dati json.RawMessage, est *estrazione) error {
	switch j.Tipo {
	case db.TipoJobSyncOutlook:
		var r api.RisultatoSync
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		var p api.PayloadSyncOutlook
		_ = json.Unmarshal(j.Payload, &p)
		// Il cursore è per (casella, cartella): senza sapere di quale casella sia questo sync, il
		// risultato non è applicabile. Non si sceglie una casella per difetto — sarebbe il cursore di
		// una casella fatto avanzare dal sync di un'altra, cioè il difetto che la 0004 elimina.
		cas := j.CasellaID
		if !cas.Valid && p.CasellaID != nil {
			cas = uuid.NullUUID{UUID: *p.CasellaID, Valid: true}
		}
		if !cas.Valid {
			return fmt.Errorf("risultato di sync senza casella: il cursore è per (casella, cartella) e non si può attribuire")
		}
		for _, c := range r.Cartelle {
			if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{
				CasellaID: cas.UUID, Cartella: c.Cartella, UltimoReceived: c.UltimoReceived,
				NMessaggi: int32(c.NMessaggi), Errore: txt(c.Errore),
			}); err != nil {
				return err
			}
			// sync storico riuscito per la cartella: la finestra [dal, al] è coperta, il prossimo "Carica precedenti" parte da dal
			if p.Al != nil && c.Errore == "" {
				if err := q.SetStoricoFinoA(ctx, db.SetStoricoFinoAParams{CasellaID: cas.UUID, Cartella: c.Cartella, StoricoFinoA: &p.Dal}); err != nil {
					return err
				}
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
		var p api.PayloadStageAllegato
		if json.Unmarshal(j.Payload, &p) == nil {
			s.riallineaEntryID(ctx, q, p.RiferimentoElemento, p.EntryID, r.RisultatoElemento)
		}
		return s.dopoStaging(ctx, q, r, est)

	case db.TipoJobAnalizzaAllegato:
		var r api.RisultatoAnalisi
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		var p api.PayloadAnalizzaAllegato
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return err
		}
		// Il worker deve rimandare indietro la combinazione che gli era stata chiesta. Se dichiara una
		// versione o una configurazione diverse, i suoi fatti finirebbero archiviati sotto una chiave
		// che non descrive come sono stati ottenuti, e verrebbero riusati per file che non c’entrano.
		// È un errore di contenuto: il risultato non è applicabile e il job fallisce in modo definitivo.
		if r.VersioneAnalizzatore != p.VersioneAnalizzatore || r.HashConfigurazione != p.HashConfigurazione {
			return fmt.Errorf("il risultato dichiara analizzatore v%d/%.8s ma era stato chiesto v%d/%.8s",
				r.VersioneAnalizzatore, r.HashConfigurazione, p.VersioneAnalizzatore, p.HashConfigurazione)
		}
		tipo, err := enumValido[db.TipoDocumento]("tipo_proposto", r.TipoProposto)
		if err != nil {
			return err
		}
		fonte, err := enumValido[db.FonteProposta]("fonte", r.Fonte)
		if err != nil {
			return err
		}
		dett := r.Dettagli
		if len(dett) == 0 {
			dett = json.RawMessage("{}")
		}

		// I FATTI si conservano per (contenuto, versione, configurazione): non appartengono
		// all’allegato che ha fatto partire l’analisi, ma al file. È ciò che rende possibile non
		// rianalizzare lo stesso disegno per ogni RFQ in cui compare.
		a, err := q.GetAllegato(ctx, r.AllegatoID)
		if err != nil {
			return fmt.Errorf("allegato dell'analisi: %w", err)
		}
		if a.Sha256.Valid && a.Sha256.String != "" {
			if _, err := q.UpsertAnalisiFatti(ctx, db.UpsertAnalisiFattiParams{
				Sha256: a.Sha256.String, VersioneAnalizzatore: int16(r.VersioneAnalizzatore),
				HashConfigurazione: r.HashConfigurazione, Fatti: dett,
			}); err != nil {
				return fmt.Errorf("analisi_fatti: %w", err)
			}
		}

		// L’INTERPRETAZIONE, invece, è di ogni singola proposta: dipende dalla direzione del messaggio
		// e dalle regole del cliente di quella RFQ. Gli stessi fatti danno proposte diverse in RFQ
		// diverse, e ognuna va scritta con le sue regole (A15).
		destinatari := []db.Allegato{a}
		if a.Sha256.Valid && a.Sha256.String != "" {
			altre, err := q.ListProposteAperteStessoFile(ctx, a.Sha256)
			if err != nil {
				return fmt.Errorf("proposte aperte con lo stesso contenuto: %w", err)
			}
			for _, pr := range altre {
				if pr.AllegatoID == a.AllegatoID {
					continue
				}
				al, err := q.GetAllegato(ctx, pr.AllegatoID)
				if err != nil {
					return err
				}
				destinatari = append(destinatari, al)
			}
		}
		for _, al := range destinatari {
			if err := s.propostaDaAnalisi(ctx, q, al, tipo, fonte, r, dett); err != nil {
				return err
			}
		}
		return nil

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

	case db.TipoJobApriElementoOutlook, db.TipoJobSegnaLetto, db.TipoJobSpostaInCartella:
		// nessun effetto sul dominio; se il worker ha ritrovato l'elemento altrove, si riallinea l'EntryID
		var p struct {
			EntryID string `json:"entry_id"`
			api.RiferimentoElemento
		}
		var r api.RisultatoElemento
		if json.Unmarshal(j.Payload, &p) == nil && json.Unmarshal(dati, &r) == nil {
			s.riallineaEntryID(ctx, q, p.RiferimentoElemento, p.EntryID, r)
		}
		return nil
	}
	return nil
}

// propostaDaAnalisi scrive la proposta di UN allegato a partire dai fatti dell’analisi. È separata
// perché gli stessi fatti vengono applicati a più allegati con lo stesso contenuto, ciascuno nel suo
// messaggio: la direzione del messaggio può cambiare la lettura, e va riletta per ognuno (A15).
func (s *Server) propostaDaAnalisi(ctx context.Context, q *db.Queries, a db.Allegato,
	tipo db.TipoDocumento, fonte db.FonteProposta, r api.RisultatoAnalisi, dett json.RawMessage) error {
	var threadID uuid.NullUUID
	entrata := false
	if m, err := q.GetMessaggio(ctx, a.MessaggioID); err == nil {
		threadID = m.ThreadID
		entrata = m.Direzione == db.DirezioneEntrata
	}
	codice, rev, conf := r.Codice, r.Rev, r.Confidenza
	if tipo == db.TipoDocumentoOffertaPromatec {
		codice, rev = "", ""
		// un'offerta Promatec la mandiamo noi: in entrata (senza "SO " nel nome) è un documento commerciale del cliente
		if entrata && !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(a.NomeFile)), "SO ") {
			tipo, conf = db.TipoDocumentoCommerciale, conf-30
		}
	}
	if conf < 0 {
		conf = 0
	}
	if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
		AllegatoID: a.AllegatoID, ThreadID: threadID, TipoProposto: tipo, Codice: txt(codice), Rev: txt(rev),
		Confidenza: int16(conf), Fonte: fonte, Dettagli: dett,
	}); err != nil {
		return fmt.Errorf("upsert proposta da analisi: %w", err)
	}
	return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
}

// riallineaEntryID aggiorna la PRESENZA quando il worker ha trovato l'elemento con un EntryID diverso
// da quello del payload (elemento spostato di cartella dopo l'ultimo sync).
//
// Dalla 0004 l'EntryID è per copia, quindi serve sapere in QUALE casella il worker ha cercato: senza
// casella_id si scriverebbe l'EntryID trovato in una casella sopra a quello di un'altra, e da quel
// momento l'elemento nell'altra casella non si aprirebbe più. Un payload senza casella_id — un worker
// più vecchio del server — non viene indovinato: si lascia la presenza com'è e lo si scrive nel log,
// perché un EntryID stantio fa fallire un'operazione e si vede, mentre uno sbagliato ne fa fallire
// un'altra, in un altro momento, senza che nessuno colleghi le due cose.
func (s *Server) riallineaEntryID(ctx context.Context, q *db.Queries, rif api.RiferimentoElemento, entryPayload string, r api.RisultatoElemento) {
	if rif.MessaggioID == nil || r.EntryID == "" || r.EntryID == entryPayload {
		return
	}
	if rif.CasellaID == nil {
		s.Log.Warn("entry_id nuovo senza casella_id nel payload: presenza non riallineata",
			"messaggio", rif.MessaggioID, "entry_id", r.EntryID)
		return
	}
	if err := q.SetEntryIDPresenza(ctx, db.SetEntryIDPresenzaParams{
		MessaggioID: *rif.MessaggioID, CasellaID: *rif.CasellaID, EntryID: r.EntryID,
		Cartella: txt(r.Cartella), StoreIDLocale: r.StoreID,
	}); err != nil {
		s.Log.Warn("riallinea entry_id", "messaggio", rif.MessaggioID, "err", err)
		return
	}
	s.Log.Info("entry_id riallineato", "messaggio", rif.MessaggioID, "casella", rif.CasellaID, "cartella", r.Cartella)
}

// dopoStaging: il file richiesto dall'operatore è in staging. Si raffina la proposta con ciò che ora si sa
// (hash → rumore già scartato), si estraggono gli zip in allegati figli e si accoda l'analisi Python
// (cartiglio, STEP) che raffina ancora finché la proposta resta aperta.
func (s *Server) dopoStaging(ctx context.Context, q *db.Queries, r api.RisultatoStage, est *estrazione) error {
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
	pr := domain.PropostaDaNome(a.NomeFile, r.Bytes, string(m.Direzione))
	ext := strings.ToLower(a.Estensione.String)
	// rumore: hash già scartato da un operatore, o immagine vista ≥ 3 volte dallo stesso dominio (firme, loghi)
	if dominio != "" && pr.Tipo != "rumore" {
		if seen, _ := q.IsHashRumore(ctx, db.IsHashRumoreParams{Sha256: r.Sha256, Lower: dominio}); seen {
			pr = domain.Proposta{Tipo: "rumore", Fonte: "rumore", Confidenza: 95}
		} else if n, _ := q.ContaHashVisto(ctx, db.ContaHashVistoParams{Sha256: txt(r.Sha256), Lower: dominio}); n >= 3 && domain.EstImmagine(ext) {
			pr = domain.Proposta{Tipo: "rumore", Fonte: "rumore", Confidenza: 85}
		}
	}
	if err := s.scriviProposta(ctx, q, a, m.ThreadID, pr, map[string]any{"estensione": ext, "bytes": r.Bytes}); err != nil {
		return err
	}
	if pr.Tipo == "rumore" {
		return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
	}
	if ext == "zip" {
		return s.registraVociZip(ctx, q, a, m, r, est)
	}
	_, err = jobs.AccodaAnalisi(ctx, q, a, m.ThreadID, s.Analizzatore)
	return err
}

func (s *Server) scriviProposta(ctx context.Context, q *db.Queries, a db.Allegato, threadID uuid.NullUUID, pr domain.Proposta, dettagli map[string]any) error {
	// pr arriva dal nostro dominio, non da un worker: qui un valore fuori enum sarebbe un errore di
	// programmazione. Si controlla lo stesso, perché è il punto in cui un tipo nuovo aggiunto al
	// dominio e dimenticato nella migrazione si vedrebbe subito e con il nome giusto.
	tipo, err := enumValido[db.TipoDocumento]("tipo_proposto", pr.Tipo)
	if err != nil {
		return err
	}
	fonte, err := enumValido[db.FonteProposta]("fonte", pr.Fonte)
	if err != nil {
		return err
	}
	dett, _ := json.Marshal(dettagli)
	if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
		AllegatoID: a.AllegatoID, ThreadID: threadID, TipoProposto: tipo, Codice: txt(pr.Codice), Rev: txt(pr.Rev),
		Confidenza: int16(pr.Confidenza), Fonte: fonte, Dettagli: dett,
	}); err != nil {
		return fmt.Errorf("proposta: %w", err)
	}
	return nil
}

// estraiZip appiattisce lo zip in allegati figli (contenitore_id = zip), ognuno con hash, proposta e analisi.
// Lo zip stesso resta come contenitore: non va sul NAS a meno di conferma esplicita.
func (s *Server) registraVociZip(ctx context.Context, q *db.Queries, a db.Allegato, m db.Messaggio, r api.RisultatoStage, est *estrazione) error {
	if est == nil || !est.fatta {
		return fmt.Errorf("estrazione dello zip %s non eseguita prima della transazione", a.NomeFile)
	}
	if est.errore != nil {
		return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoErrore, Errore: txt("zip non leggibile: " + est.errore.Error())})
	}
	voci := est.voci
	for i, v := range voci {
		figlio, err := q.UpsertAllegato(ctx, db.UpsertAllegatoParams{
			MessaggioID: a.MessaggioID, ContenitoreID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true}, Indice: int16(i + 1),
			NomeFile: v.NomeFile, PathInterno: txt(v.PathInterno), Estensione: txt(strings.ToLower(strings.TrimPrefix(filepath.Ext(v.NomeFile), "."))),
			Natura: db.NaturaAllegatoFile, Origine: a.Origine, Bytes: pgtype.Int8{Int64: v.Bytes, Valid: true}, Sha256: txt(v.Sha256), RicevutoIl: a.RicevutoIl,
		})
		if err != nil {
			return fmt.Errorf("voce zip %s: %w", v.NomeFile, err)
		}
		if err := q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: figlio.AllegatoID, PathStaging: txt(v.Path), Sha256: txt(v.Sha256), Bytes: pgtype.Int8{Int64: v.Bytes, Valid: true}}); err != nil {
			return err
		}
		figlio.PathStaging, figlio.Sha256 = txt(v.Path), txt(v.Sha256)
		pr := domain.PropostaDaNome(v.NomeFile, v.Bytes, string(m.Direzione))
		if err := s.scriviProposta(ctx, q, figlio, m.ThreadID, pr, map[string]any{"path_interno": v.PathInterno, "bytes": v.Bytes, "zip": a.NomeFile}); err != nil {
			return err
		}
		if pr.Tipo == "rumore" {
			_ = q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: figlio.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
			continue
		}
		if _, err := jobs.AccodaAnalisi(ctx, q, figlio, m.ThreadID, s.Analizzatore); err != nil {
			return err
		}
	}
	dettagli := map[string]any{"voci": len(voci), "estensione": "zip", "bytes": r.Bytes}
	if est.troncato {
		dettagli["troncato"] = true
	}
	if err := s.scriviProposta(ctx, q, a, m.ThreadID, domain.Proposta{Tipo: "altro", Fonte: "estensione", Confidenza: 20}, dettagli); err != nil {
		return err
	}
	return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
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
	// Un lotto arriva sempre dentro un tentativo di sync: senza, il server non potrebbe distinguere
	// un worker vivo da uno scaduto che sta ancora scrivendo (precisazione P5).
	t, err := tentativo(req.JobID, req.WorkerID, req.LeaseToken)
	if err != nil {
		errore(w, 400, fmt.Errorf("ingest senza tentativo valido: %w", err))
		return
	}
	ctx := r.Context()

	// La casella si verifica PRIMA della transazione: se non è censita è un errore di configurazione
	// che riguarda tutto il lotto, non un dato da scartare elemento per elemento (I23).
	casella, err := ingest.RisolviCasella(ctx, db.New(s.Pool), req.CasellaID, s.CasellaDefault)
	if err != nil {
		if errors.Is(err, ingest.ErrCasellaNonCensita) {
			// il sync di questa casella non può funzionare finché qualcuno non corregge la
			// configurazione: farlo ritentare cinque volte non serve a nulla
			if _, e := jobs.Fallisci(ctx, db.New(s.Pool), t, err.Error(), true); e != nil && !errors.Is(e, jobs.ErrTentativoNonValido) {
				s.Log.Error("fallimento per casella non censita non registrato", "job", req.JobID, "err", e)
			}
			s.Log.Error("lotto rifiutato", "job", req.JobID, "err", err)
			errore(w, 422, err)
			return
		}
		errore(w, 500, err)
		return
	}

	res, err := s.Ingest.Ingerisci(ctx, ingest.Lotto{
		Casella:   casella,
		Tentativo: &ingest.Tentativo{JobID: t.JobID, LeaseToken: t.LeaseToken, WorkerID: t.WorkerID},
		Messaggi:  req.Messaggi,
		Saltati:   req.Saltati,
		Cursore:   req.Cursore,
	})
	switch {
	case errors.Is(err, ingest.ErrTentativoNonValido):
		s.nonValido(w, ctx, req.JobID)
		return
	case err != nil:
		// Un errore qui è un guasto (database irraggiungibile, commit fallito), non un dato sbagliato:
		// un dato sbagliato finisce in scarto e il lotto continua. 5xx dice al worker di ripetere il
		// lotto identico, ed è sicuro farlo perché non è stato scritto niente (I16).
		s.Log.Error("ingest lotto", "job", req.JobID, "casella", casella.Indirizzo, "err", err)
		errore(w, 500, err)
		return
	}
	if res.Esiti == nil {
		res.Esiti = []api.EsitoMessaggio{}
	}
	if res.Falliti > 0 {
		s.Log.Warn("lotto con scarti", "job", req.JobID, "inseriti", res.Inseriti, "aggiornati", res.Aggiornati, "falliti", res.Falliti)
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
