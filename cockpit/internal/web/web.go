// Package web serve HTML (mai JSON) al browser. Ogni rotta esiste in due forme decise dall'header
// HX-Request: pagina intera (layout + contenuto) o solo il frammento.
package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/nas"
)

type Server struct {
	Pool   *pgxpool.Pool
	Log    *slog.Logger
	NAS    *nas.Scrittore
	Templ  fs.FS // web/templates
	Static fs.FS // web/static
	pagine map[string]*template.Template
}

type chiaveCtx int

const ctxUtente chiaveCtx = 1

const cookieSessione = "cockpit_sess"

var funzioni = template.FuncMap{
	"list":      func(a ...string) []string { return a },
	"data":      func(t time.Time) string { return t.Local().Format("02/01 15:04") },
	"dataLunga": func(t time.Time) string { return t.Local().Format("02/01/2006 15:04") },
	"txt":       func(t pgtype.Text) string { return t.String },
	"kb": func(b pgtype.Int8) string {
		if !b.Valid {
			return ""
		}
		if b.Int64 < 1024 {
			return fmt.Sprintf("%d B", b.Int64)
		}
		return fmt.Sprintf("%.0f KB", float64(b.Int64)/1024)
	},
	"motivi": func(m *json.RawMessage) []string {
		if m == nil {
			return nil
		}
		var out []string
		_ = json.Unmarshal(*m, &out)
		return out
	},
	"uuidBreve": func(u uuid.UUID) string { return u.String()[:8] },
	"colore": func(esito pgtype.Text, conf pgtype.Int2) string {
		if !esito.Valid {
			return ""
		}
		switch esito.String {
		case "nuova_rfq":
			if conf.Int16 >= 75 {
				return "verde"
			}
			return "giallo"
		case "aggancia":
			return "verde"
		}
		return "grigio"
	},
}

func (s *Server) Init() error {
	base, err := template.New("layout").Funcs(funzioni).ParseFS(s.Templ, "layout.html", "frammenti.html")
	if err != nil {
		return err
	}
	s.pagine = map[string]*template.Template{}
	for _, p := range []string{"inbox.html", "login.html", "job.html", "cruscotto.html"} {
		t, err := template.Must(base.Clone()).ParseFS(s.Templ, p)
		if err != nil {
			return fmt.Errorf("template %s: %w", p, err)
		}
		s.pagine[p] = t
	}
	return nil
}

func (s *Server) Registra(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(s.Static)))
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)
	mux.HandleFunc("GET /{$}", s.autenticato(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/inbox", http.StatusFound) }))
	mux.HandleFunc("GET /inbox", s.autenticato(s.inbox))
	mux.HandleFunc("POST /inbox/sync-storico", s.autenticato(s.syncStorico))
	mux.HandleFunc("GET /messaggio/{id}", s.autenticato(s.messaggio))
	mux.HandleFunc("POST /messaggio/{id}/apri", s.autenticato(s.apriInOutlook))
	mux.HandleFunc("POST /messaggio/{id}/bozza", s.autenticato(s.bozza))
	mux.HandleFunc("POST /messaggio/{id}/letto", s.autenticato(s.segnaLetto))
	mux.HandleFunc("GET /cruscotto", s.autenticato(s.cruscotto))
	mux.HandleFunc("GET /admin/job", s.autenticato(s.adminJob))
	mux.HandleFunc("POST /admin/job/{id}/riaccoda", s.autenticato(s.riaccodaJob))
}

// ---------------------------------------------------------------- rendering

type vista struct {
	Utente    *db.Utente
	Titolo    string
	Dati      any
	Frammento bool
}

func (s *Server) rendi(w http.ResponseWriter, r *http.Request, pagina, frammento string, titolo string, dati any) {
	t := s.pagine[pagina]
	v := vista{Utente: utenteDa(r.Context()), Titolo: titolo, Dati: dati, Frammento: r.Header.Get("HX-Request") == "true"}
	nome := "layout"
	if v.Frammento && frammento != "" {
		nome = frammento
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, nome, v); err != nil {
		s.Log.Error("template", "pagina", pagina, "err", err)
	}
}

func (s *Server) frammento(w http.ResponseWriter, nome string, dati any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["inbox.html"].ExecuteTemplate(w, nome, vista{Dati: dati, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", nome, "err", err)
	}
}

func utenteDa(ctx context.Context) *db.Utente {
	u, _ := ctx.Value(ctxUtente).(*db.Utente)
	return u
}

// ---------------------------------------------------------------- healthz

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	out := api.Salute{DB: "ok", NAS: "non_raggiungibile"}
	var v int
	if err := s.Pool.QueryRow(r.Context(), "SELECT max(versione) FROM schema_versione").Scan(&v); err != nil {
		out.DB = "errore: " + err.Error()
	}
	out.Versione = v
	if s.NAS.Raggiungibile() {
		out.NAS = "ok"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// ---------------------------------------------------------------- sessioni

func (s *Server) autenticato(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieSessione)
		if err != nil {
			s.aLogin(w, r)
			return
		}
		q := db.New(s.Pool)
		u, err := q.GetSessioneUtente(r.Context(), c.Value)
		if err != nil {
			s.aLogin(w, r)
			return
		}
		_ = q.ToccaSessione(r.Context(), c.Value)
		h(w, r.WithContext(context.WithValue(r.Context(), ctxUtente, &u)))
	}
}

func (s *Server) aLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.rendi(w, r, "login.html", "", "Accesso", map[string]any{"Errore": r.URL.Query().Get("errore")})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	u, err := q.GetUtentePerSigla(r.Context(), strings.ToUpper(strings.TrimSpace(r.FormValue("sigla"))))
	if err != nil || !u.PasswordHash.Valid || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash.String), []byte(r.FormValue("password"))) != nil {
		http.Redirect(w, r, "/login?errore=credenziali", http.StatusFound)
		return
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	if _, err := q.CreaSessione(r.Context(), db.CreaSessioneParams{Token: tok, UtenteID: u.UtenteID, ScadeIl: time.Now().Add(12 * time.Hour)}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSessione, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 12 * 3600})
	http.Redirect(w, r, "/inbox", http.StatusFound)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieSessione); err == nil {
		_ = db.New(s.Pool).EliminaSessione(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSessione, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// SeedUtenti crea/aggiorna gli utenti iniziali da cockpit.toml (password → bcrypt).
func SeedUtenti(ctx context.Context, q *db.Queries, utenti []struct{ Sigla, Nome, Ufficio, Ruolo, Password string }) error {
	for _, u := range utenti {
		var hash pgtype.Text
		if u.Password != "" {
			h, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			hash = pgtype.Text{String: string(h), Valid: true}
		}
		ruolo := db.RuoloUtente(u.Ruolo)
		if !ruolo.Valid() {
			ruolo = db.RuoloUtenteOperatore
		}
		if _, err := q.UpsertUtente(ctx, db.UpsertUtenteParams{Sigla: strings.ToUpper(u.Sigla), Nome: u.Nome, Ufficio: u.Ufficio, Ruolo: ruolo, PasswordHash: hash}); err != nil {
			return fmt.Errorf("utente %s: %w", u.Sigla, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- inbox (schermata A, versione minima)

type inboxDati struct {
	Filtro   string
	Righe    []db.VInbox
	Conta    db.ContaInboxRow
	Selezion string
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	filtro := r.URL.Query().Get("filtro")
	if filtro != "agganciati" && filtro != "tutti" {
		filtro = "orfani"
	}
	righe, err := q.ListInbox(r.Context(), db.ListInboxParams{Filtro: filtro, Limite: 200, Salta: 0})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaInbox(r.Context())
	d := inboxDati{Filtro: filtro, Righe: righe, Conta: conta, Selezion: r.URL.Query().Get("sel")}
	s.rendi(w, r, "inbox.html", "inbox_lista", "Inbox", d)
}

func (s *Server) syncStorico(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var minData time.Time
	err := s.Pool.QueryRow(ctx, "SELECT COALESCE(MIN(data_evento), now()) FROM messaggio").Scan(&minData)
	if err != nil {
		s.Log.Error("query min data_evento", "err", err)
		minData = time.Now()
	}
	al := minData
	dal := minData.AddDate(0, 0, -30)

	q := db.New(s.Pool)
	cursori, _ := q.ListSyncCursori(ctx)
	var cartelle []api.CartellaCursore
	if len(cursori) > 0 {
		for _, c := range cursori {
			cartelle = append(cartelle, api.CartellaCursore{Cartella: c.Cartella, UltimoReceived: nil})
		}
	} else {
		cartelle = []api.CartellaCursore{
			{Cartella: "Inbox", UltimoReceived: nil},
			{Cartella: "Sent Items", UltimoReceived: nil},
		}
	}

	payload := api.PayloadSyncOutlook{
		Cartelle:         cartelle,
		Dal:              dal,
		Al:               &al,
		SovrapposizioneS: 600,
		Lotto:            50,
	}

	chiave := fmt.Sprintf("sync_storico:%d", dal.Unix())
	job, err := jobs.Accoda(ctx, q, db.TipoJobSyncOutlook, payload, chiave, 2)
	if err != nil {
		s.Log.Error("accoda sync storico", "err", err)
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if job == nil {
		fmt.Fprintf(w, `<span class="badge avviso">Sync storico già in coda [%s - %s]</span>`, dal.Format("02/01/2006"), al.Format("02/01/2006"))
		return
	}
	fmt.Fprintf(w, `<span class="badge verde">Accodato sync storico #%d [%s - %s]</span>`, job.JobID, dal.Format("02/01/2006"), al.Format("02/01/2006"))
}

type AllegatoUI struct {
	db.Allegato
	FileMancante bool
}

type messaggioDati struct {
	M        db.Messaggio
	Riga     db.VInbox
	Outlook  *db.MessaggioOutlook
	Allegati []AllegatoUI
	Proposte map[uuid.UUID]db.DocumentoProposta
	Portale  []db.RiferimentoPortale
	Bozze    []db.Bozza
	Avviso   string
}

func (s *Server) messaggio(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaMessaggio(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/inbox?filtro=tutti&sel="+id.String(), http.StatusFound)
		return
	}
	s.frammento(w, "messaggio_pannello", d)
}

func (s *Server) caricaMessaggio(ctx context.Context, id uuid.UUID) (*messaggioDati, error) {
	q := db.New(s.Pool)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &messaggioDati{M: m, Proposte: map[uuid.UUID]db.DocumentoProposta{}}
	d.Riga, _ = q.GetInboxRiga(ctx, id)
	if o, err := q.GetMessaggioOutlook(ctx, id); err == nil {
		d.Outlook = &o
	}
	allegati, _ := q.ListAllegatiMessaggio(ctx, id)
	for _, a := range allegati {
		var mancante bool
		if a.PathStaging.Valid && a.PathStaging.String != "" {
			if _, err := os.Stat(a.PathStaging.String); err != nil {
				mancante = true
			}
		} else if a.Stato == db.StatoAllegatoInStaging || a.Stato == db.StatoAllegatoAnalizzato {
			mancante = true
		}
		d.Allegati = append(d.Allegati, AllegatoUI{
			Allegato:     a,
			FileMancante: mancante,
		})
	}
	rows, _ := s.Pool.Query(ctx, `SELECT p.* FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id WHERE a.messaggio_id = $1`, id)
	if rows != nil {
		props, _ := pgx.CollectRows(rows, pgx.RowToStructByName[db.DocumentoProposta])
		for _, p := range props {
			d.Proposte[p.AllegatoID] = p
		}
	}
	rp, _ := s.Pool.Query(ctx, `SELECT * FROM riferimento_portale WHERE messaggio_id = $1 ORDER BY creato_il`, id)
	if rp != nil {
		d.Portale, _ = pgx.CollectRows(rp, pgx.RowToStructByName[db.RiferimentoPortale])
	}
	bz, _ := s.Pool.Query(ctx, `SELECT * FROM bozza WHERE in_risposta_a = $1 ORDER BY creata_il DESC`, id)
	if bz != nil {
		d.Bozze, _ = pgx.CollectRows(bz, pgx.RowToStructByName[db.Bozza])
	}
	return d, nil
}

// apriInOutlook accoda apri_elemento_outlook con priorità massima: il worker fa Display() sull'item.
func (s *Server) apriInOutlook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	o, err := q.GetMessaggioOutlook(r.Context(), id)
	if err != nil {
		http.Error(w, "messaggio non Outlook", 404)
		return
	}
	if _, err := jobs.Accoda(r.Context(), q, db.TipoJobApriElementoOutlook, api.PayloadApriElemento{EntryID: o.EntryID, StoreID: o.StoreID}, "", 1); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.avviso(w, "Richiesta inviata a Outlook: l'elemento si apre sul PC Cockpit.")
}

func (s *Server) segnaLetto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	o, err := q.GetMessaggioOutlook(r.Context(), id)
	if err != nil {
		http.Error(w, "messaggio non Outlook", 404)
		return
	}
	letto := r.FormValue("letto") != "0"
	if _, err := jobs.Accoda(r.Context(), q, db.TipoJobSegnaLetto, api.PayloadSegnaLetto{EntryID: o.EntryID, StoreID: o.StoreID, Letto: letto}, "", 2); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_, _ = s.Pool.Exec(r.Context(), `UPDATE messaggio_outlook SET non_letto = $2 WHERE messaggio_id = $1`, id, !letto)
	s.avviso(w, "Aggiornato in Outlook.")
}

// bozza prepara la risposta nel Cockpit e la apre come bozza in Outlook (l'invio resta manuale, SPEC §6.1).
func (s *Server) bozza(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	tipo := db.TipoBozza(r.FormValue("tipo"))
	if !tipo.Valid() || tipo == db.TipoBozzaNuovo {
		tipo = db.TipoBozzaRisposta
	}
	corpo := strings.TrimSpace(r.FormValue("corpo"))
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	o, err := q.GetMessaggioOutlook(ctx, id)
	if err != nil {
		http.Error(w, "messaggio non Outlook", 404)
		return
	}
	b, err := q.InsertBozza(ctx, db.InsertBozzaParams{
		ThreadID: m.ThreadID, InRispostaA: uuid.NullUUID{UUID: id, Valid: true}, Tipo: tipo,
		Destinatari: json.RawMessage("[]"), Oggetto: m.Oggetto, Corpo: pgtype.Text{String: corpo, Valid: corpo != ""}, Documenti: []uuid.UUID{}, CreataDa: u.UtenteID,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	html := ""
	if corpo != "" {
		html = "<div style=\"font-family:Calibri,sans-serif;font-size:11pt\">" + strings.ReplaceAll(template.HTMLEscapeString(corpo), "\n", "<br>") + "</div><br>"
	}
	if _, err := jobs.Accoda(ctx, q, db.TipoJobCreaBozzaOutlook, api.PayloadCreaBozza{
		BozzaID: b.BozzaID, Tipo: string(tipo), EntryID: o.EntryID, StoreID: o.StoreID, Destinatari: []api.Destinatario{},
		CorpoHTML: html, CorpoTesto: corpo, Allegati: []string{}, Mostra: true, Invia: false,
	}, "bozza:"+b.BozzaID.String(), 1); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.avviso(w, "Bozza in preparazione: si apre in Outlook tra pochi secondi. Rileggi e premi Invia lì.")
}

func (s *Server) avviso(w http.ResponseWriter, testo string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="avviso">%s</div>`, template.HTMLEscapeString(testo))
}

// ---------------------------------------------------------------- cruscotto e admin

func (s *Server) cruscotto(w http.ResponseWriter, r *http.Request) {
	righe, err := db.New(s.Pool).ListCruscotto(r.Context(), db.ListCruscottoParams{SoloAperti: r.URL.Query().Get("tutti") == "", Limit: 200})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.rendi(w, r, "cruscotto.html", "cruscotto_tabella", "Cruscotto", righe)
}

type jobDati struct {
	Conta []db.ContaJobPerStatoRow
	Job   []db.Job
	Stato string
}

func (s *Server) adminJob(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	var stato db.NullStatoJob
	if st := r.URL.Query().Get("stato"); st != "" && db.StatoJob(st).Valid() {
		stato = db.NullStatoJob{StatoJob: db.StatoJob(st), Valid: true}
	}
	lista, err := q.ListJob(r.Context(), db.ListJobParams{Stato: stato, Limit: 100})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaJobPerStato(r.Context())
	s.rendi(w, r, "job.html", "job_tabella", "Coda job", jobDati{Conta: conta, Job: lista, Stato: r.URL.Query().Get("stato")})
}

func (s *Server) riaccodaJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id", 400)
		return
	}
	if err := db.New(s.Pool).RiaccodaJob(r.Context(), id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}
