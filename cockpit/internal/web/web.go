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
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/nas"
)

type Server struct {
	Pool   *pgxpool.Pool
	Log    *slog.Logger
	NAS    *nas.Scrittore
	Ingest *ingest.Servizio // riprova degli scarti dell'ingest
	Templ  fs.FS            // web/templates
	Static fs.FS            // web/static
	pagine map[string]*template.Template
	// IntervalloSync: ogni quanto lo scheduler accoda il sync (0 = mai). Serve solo a dirlo
	// all'operatore nella schermata, con parole che corrispondono alla configurazione.
	IntervalloSync time.Duration
	// Sync sono cartelle, data minima e lotto del sync ordinario: le stesse dello scheduler, perché
	// «Aggiorna ora» accoda esattamente il job che accoderebbe lui (voce 2.16).
	Sync jobs.SyncOpzioni
	// Modalita è "shadow" o "produzione" (§2.7): la testata lo dice sempre, perché in shadow metà
	// dei pulsanti non fa quello che c'è scritto sopra, e va saputo prima di premerli.
	Modalita string
	// IndirizzoClient: da quale IP arriva il browser (voce 2.7, abbinamento sessione → postazione).
	// Nil = RemoteAddr. X-Forwarded-For non si legge in produzione: lo scriverebbe chiunque.
	IndirizzoClient func(*http.Request) string
}

type chiaveCtx int

const ctxUtente chiaveCtx = 1

const cookieSessione = "cockpit_sess"

var funzioni = template.FuncMap{
	"list":      func(a ...string) []string { return a },
	"data":      func(t time.Time) string { return t.Local().Format("02/01 15:04") },
	"dataLunga": func(t time.Time) string { return t.Local().Format("02/01/2006 15:04") },
	// oraBreve/oraLunga accettano un istante che può non esserci (l'ultimo sync di una casella che
	// non ha mai sincronizzato): il template deve poterle chiamare senza sapere se il valore c'è.
	"oraBreve": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Local().Format("15:04")
	},
	"oraLunga": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.Local().Format("02/01/2006 15:04")
	},
	"txt": func(t pgtype.Text) string { return t.String },
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
	"tipiDocumento": func() []string {
		out := make([]string, 0, len(db.AllTipoDocumentoValues()))
		for _, t := range db.AllTipoDocumentoValues() {
			out = append(out, string(t))
		}
		return out
	},
	"vistaAllegati": func(a []AllegatoUI, messaggioID uuid.UUID, agganciato bool, ritornaThread string) allegatiVista {
		v := allegatiVista{Allegati: a, MessaggioID: messaggioID, Agganciato: agganciato, RitornaThread: ritornaThread}
		for _, x := range a {
			if x.Scaricabile() {
				v.NScaricabili++
			}
		}
		return v
	},
	"rigaAllegato": func(a AllegatoUI, agganciato bool, ritornaThread string, figlio bool) rigaAllegato {
		return rigaAllegato{A: a, Agganciato: agganciato, RitornaThread: ritornaThread, Figlio: figlio}
	},
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
	for _, p := range []string{"inbox.html", "login.html", "job.html", "scarti.html", "cruscotto.html", "thread.html"} {
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
	mux.HandleFunc("POST /inbox/aggiorna", s.autenticato(s.aggiornaOra))
	mux.HandleFunc("POST /inbox/sync-storico", s.autenticato(s.syncStorico))
	mux.HandleFunc("GET /inbox/sync-storico/stato", s.autenticato(s.syncStoricoStato))
	mux.HandleFunc("GET /stato/worker", s.autenticato(s.statoWorker))
	mux.HandleFunc("POST /sessione/postazione", s.autenticato(s.scegliPostazione))
	mux.HandleFunc("GET /messaggio/{id}", s.autenticato(s.messaggio))
	mux.HandleFunc("POST /messaggio/{id}/apri", s.autenticato(s.apriInOutlook))
	mux.HandleFunc("POST /messaggio/{id}/bozza", s.autenticato(s.bozza))
	mux.HandleFunc("POST /messaggio/{id}/letto", s.autenticato(s.segnaLetto))
	// blocco 4: triage
	mux.HandleFunc("GET /messaggio/{id}/triage", s.autenticato(s.triageForm))
	mux.HandleFunc("POST /messaggio/{id}/rfq", s.autenticato(s.nuovaRFQ))
	mux.HandleFunc("POST /messaggio/{id}/aggancia", s.autenticato(s.agganciaEsistente))
	mux.HandleFunc("POST /messaggio/{id}/ignora", s.autenticato(s.ignora))
	mux.HandleFunc("GET /anagrafica/buyer", s.autenticato(s.buyerSelect))
	mux.HandleFunc("GET /thread/cerca", s.autenticato(s.cercaThread))
	mux.HandleFunc("GET /thread/{id}", s.autenticato(s.thread))
	// download su richiesta e smistamento (blocco 5)
	mux.HandleFunc("POST /messaggio/{id}/scarica", s.autenticato(s.scarica))
	mux.HandleFunc("POST /allegato/{id}/riscarica", s.autenticato(s.riscarica))
	mux.HandleFunc("POST /proposta/{id}/conferma", s.autenticato(s.conferma))
	mux.HandleFunc("POST /proposta/{id}/scarta", s.autenticato(s.scarta))
	mux.HandleFunc("GET /cruscotto", s.autenticato(s.cruscotto))
	mux.HandleFunc("GET /admin/job", s.autenticato(s.adminJob))
	mux.HandleFunc("POST /admin/job/{id}/riaccoda", s.autenticato(s.riaccodaJob))
	mux.HandleFunc("POST /admin/job/{id}/annulla", s.autenticato(s.annullaJob))
	mux.HandleFunc("GET /admin/scarti", s.autenticato(s.adminScarti))
	mux.HandleFunc("POST /admin/scarti/{id}/riprova", s.autenticato(s.riprovaScarto))
}

// ---------------------------------------------------------------- rendering

type vista struct {
	Utente    *db.Utente
	Titolo    string
	Dati      any
	Frammento bool
	// Stato è la testata: postazione della sessione, stato per casella, worker di analisi (M2).
	Stato *statoUI
}

func (s *Server) statoWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["inbox.html"].ExecuteTemplate(w, "stato_worker", vista{Utente: utenteDa(r.Context()), Stato: s.stato(r.Context(), sessioneDa(r.Context())), Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "stato_worker", "err", err)
	}
}

func (s *Server) rendi(w http.ResponseWriter, r *http.Request, pagina, frammento string, titolo string, dati any) {
	t := s.pagine[pagina]
	v := vista{Utente: utenteDa(r.Context()), Titolo: titolo, Dati: dati, Frammento: r.Header.Get("HX-Request") == "true"}
	if !v.Frammento && v.Utente != nil {
		v.Stato = s.stato(r.Context(), sessioneDa(r.Context()))
	}
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
		riga, err := q.GetSessione(r.Context(), c.Value)
		if err != nil {
			s.aLogin(w, r)
			return
		}
		_ = q.ToccaSessione(r.Context(), c.Value)
		u := riga.Utente
		sess := sessioneUI{Token: c.Value, Utente: &u, Postazione: riga.PostazioneID, NomeHost: riga.NomeHost.String, Origine: riga.PostazioneOrigine.String}
		ctx := context.WithValue(r.Context(), ctxUtente, &u)
		ctx = context.WithValue(ctx, ctxSessione, sess)
		h(w, r.WithContext(ctx))
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
	// Abbinamento per IP (voce 2.7): se un worker attivo di una postazione autorizzata ha fatto
	// claim da questo stesso indirizzo, la sessione nasce su quella postazione ('ip'). Altrimenti
	// nasce senza, e l'operatore sceglie dalla testata.
	var post uuid.NullUUID
	var origine pgtype.Text
	if ip, ok := s.indirizzoDi(r); ok {
		id, motivo := s.abbinaPostazione(r.Context(), q, &u, ip)
		if id.Valid {
			post, origine = id, pgtype.Text{String: "ip", Valid: true}
			s.Log.Info("sessione abbinata alla postazione per IP", "utente", u.Sigla, "postazione", motivo, "ip", ip.String())
		} else {
			s.Log.Info("sessione senza postazione", "utente", u.Sigla, "ip", ip.String(), "motivo", motivo)
		}
	}
	if _, err := q.CreaSessioneConPostazione(r.Context(), db.CreaSessioneConPostazioneParams{
		Token: tok, UtenteID: u.UtenteID, ScadeIl: time.Now().Add(12 * time.Hour), PostazioneID: post, PostazioneOrigine: origine,
	}); err != nil {
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
	// Sync dice all'operatore se e ogni quanto il server accoda il sync: prima la schermata diceva
	// «ogni minuto» qualunque fosse la configurazione, e con intervallo_sync_s = 0 era falso.
	Sync     string
	Filtro   string
	Righe    []db.VInbox
	Conta    db.ContaInboxRow
	Selezion string
	// Caselle sono quelle attive; Casella è quella scelta nel selettore (vuoto = tutte). È un FILTRO
	// della schermata, non un'autorizzazione: che cosa un utente possa vedere è la voce 2.2, e fino
	// ad allora resta la regola restrittiva di D12.
	Caselle []db.Casella
	Casella string
	// Nuovi sono i messaggi arrivati dopo l'ultima visita: la lista li segna con un pallino, e il
	// numero sta in testata (voce 2.16, SV3). Vale per la pagina appena caricata: chi resta fermo
	// sulla schermata vede crescere il contatore e comparire i pallini ai poll successivi.
	Nuovi  map[uuid.UUID]bool
	NNuove int
}

// ENuovo dice se una riga della lista è arrivata dopo l'ultima visita dell'operatore.
func (d inboxDati) ENuovo(id uuid.UUID) bool { return d.Nuovi[id] }

// descrizioneSync è la frase della schermata sullo stato della sincronizzazione automatica.
func (s *Server) descrizioneSync() string {
	if s.IntervalloSync <= 0 {
		return "Sincronizzazione automatica disattivata (intervallo_sync_s = 0): nessun sync viene accodato; «Carica precedenti» resta disponibile."
	}
	return fmt.Sprintf("Il worker Outlook sincronizza ogni %d s.", int(s.IntervalloSync/time.Second))
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	filtro := r.URL.Query().Get("filtro")
	if filtro != "agganciati" && filtro != "tutti" && filtro != "ignorati" {
		filtro = "orfani"
	}
	caselle, _ := q.ListCaselleAttive(r.Context())
	var scelta uuid.NullUUID
	grezzo := r.URL.Query().Get("casella")
	if id, err := uuid.Parse(grezzo); err == nil {
		for _, c := range caselle {
			if c.CasellaID == id {
				scelta = uuid.NullUUID{UUID: id, Valid: true}
			}
		}
	}
	if !scelta.Valid {
		grezzo = ""
	}
	righe, err := q.ListInbox(r.Context(), db.ListInboxParams{Filtro: filtro, Casella: scelta, Limite: 200, Salta: 0})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaInbox(r.Context(), scelta)
	u := utenteDa(r.Context())
	nov := s.novitaPer(r.Context(), q, u)
	d := inboxDati{Filtro: filtro, Righe: righe, Conta: conta, Selezion: r.URL.Query().Get("sel"),
		Caselle: caselle, Casella: grezzo, Sync: s.descrizioneSync(), Nuovi: nov.Id, NNuove: nov.Totale}
	s.rendi(w, r, "inbox.html", "inbox_lista", "Inbox", d)
	// La visita si segna DOPO aver reso la pagina, e solo se è una pagina: il poll HTMX ogni 15 s
	// chiede lo stesso indirizzo, e se azzerasse anche lui il contatore direbbe sempre zero (SV3).
	if r.Header.Get("HX-Request") != "true" {
		s.segnaVista(r.Context(), q, u)
	}
}

// syncStorico accoda una finestra di 30 giorni PRIMA di quanto già coperto: il limite superiore è il cursore
// storico_fino_a (se un "Carica precedenti" è già riuscito) oppure la mail più vecchia in archivio.
//
// Dalla 0004 è PER CASELLA: ogni casella ha i suoi cursori e la sua storia, e un unico job storico
// avrebbe letto l'archivio di una casella sola facendo credere di averle coperte tutte. Un job per
// casella attiva, chiave `sync_storico:<casella_id>`: al più uno pendente per ciascuna.
func (s *Server) syncStorico(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	caselle, err := q.ListCaselleAttive(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var ultimo *db.Job
	for _, c := range caselle {
		if c.Canale != db.CanaleOutlook {
			continue
		}
		chiave := "sync_storico:" + c.CasellaID.String()
		if j, err := q.JobPendentePerChiave(ctx, pgtype.Text{String: chiave, Valid: true}); err == nil {
			ultimo = &j
			continue
		}
		al, dal, cartelle, err := s.finestraStorico(ctx, q, c.CasellaID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		cid := c.CasellaID
		payload := api.PayloadSyncOutlook{CasellaID: &cid, Cartelle: cartelle, Dal: dal, Al: &al, SovrapposizioneS: 600, Lotto: 50}
		job, err := jobs.AccodaCon(ctx, q, db.TipoJobSyncOutlook, payload, chiave, 2,
			jobs.Opzioni{Casella: uuid.NullUUID{UUID: cid, Valid: true}})
		if err != nil {
			s.Log.Error("accoda sync storico", "casella", c.Indirizzo, "err", err)
			http.Error(w, err.Error(), 500)
			return
		}
		if job != nil {
			ultimo = job
		}
	}
	if ultimo == nil {
		s.avviso(w, "Nessuna casella attiva da cui caricare l'archivio.")
		return
	}
	s.badgeStorico(w, ultimo)
}

// finestraStorico calcola [al-30gg, al] e le cartelle da leggere per UNA casella (senza cursore:
// finestra esatta). «Quanto indietro siamo già andati» è una proprietà della casella: mescolare le
// storie di due caselle farebbe ripartire l'una da dove è arrivata l'altra.
func (s *Server) finestraStorico(ctx context.Context, q *db.Queries, casella uuid.UUID) (al, dal time.Time, cartelle []api.CartellaCursore, err error) {
	cursori, _ := q.ListSyncCursoriCasella(ctx, casella)
	for _, c := range cursori {
		cartelle = append(cartelle, api.CartellaCursore{Cartella: c.Cartella})
		if c.StoricoFinoA != nil && (al.IsZero() || c.StoricoFinoA.Before(al)) {
			al = *c.StoricoFinoA
		}
	}
	if len(cartelle) == 0 {
		cartelle = []api.CartellaCursore{{Cartella: "Inbox"}, {Cartella: "Sent Items"}}
	}
	if al.IsZero() {
		var minData *time.Time
		if err = s.Pool.QueryRow(ctx, `SELECT min(mc.ricevuto_il) FROM messaggio_casella mc WHERE mc.casella_id = $1`, casella).Scan(&minData); err != nil {
			return
		}
		if minData != nil {
			al = *minData
		} else {
			al = time.Now()
		}
	}
	dal = al.AddDate(0, 0, -30)
	return
}

// syncStoricoStato è il frammento che il badge ricarica finché il job non è chiuso.
func (s *Server) syncStoricoStato(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	if j, err := q.JobPendenteConPrefisso(r.Context(), "sync_storico"); err == nil {
		s.badgeStorico(w, &j)
		return
	}
	j, err := q.UltimoJobPerChiavePrefisso(r.Context(), "sync_storico")
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.badgeStorico(w, &j)
}

func (s *Server) badgeStorico(w http.ResponseWriter, j *db.Job) {
	var p api.PayloadSyncOutlook
	_ = json.Unmarshal(j.Payload, &p)
	finestra := p.Dal.Local().Format("02/01/2006")
	if p.Al != nil {
		finestra += " – " + p.Al.Local().Format("02/01/2006")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	switch j.Stato {
	case db.StatoJobPronto, db.StatoJobInCorso:
		fmt.Fprintf(w, `<span class="badge avviso" hx-get="/inbox/sync-storico/stato" hx-trigger="every 3s" hx-swap="outerHTML">Sync storico #%d %s [%s]…</span>`, j.JobID, j.Stato, finestra)
	case db.StatoJobFatto:
		fmt.Fprintf(w, `<span class="badge verde">Sync storico #%d completato [%s]. Premi di nuovo per il mese precedente.</span>`, j.JobID, finestra)
	default:
		fmt.Fprintf(w, `<span class="badge errore">Sync storico #%d fallito: %s</span>`, j.JobID, template.HTMLEscapeString(j.Errore.String))
	}
}

type messaggioDati struct {
	M    db.Messaggio
	Riga db.VInbox
	// Copia è la copia su cui agiscono i pulsanti («Apri in Outlook», «Segna letto», «Bozza»): quella
	// che il worker della POSTAZIONE DELLA SESSIONE serve (voci 2.2 e 2.7). Nil = le azioni non sono
	// disponibili, e MotivoAzioni dice perché (nessuna postazione, o nessun worker idoneo su quella).
	// Presenze è l'elenco completo, che è ciò che l'operatore deve vedere per capire da dove arriva.
	Copia        *jobs.Copia
	MotivoAzioni string
	Presenze     []db.ListPresenzeRow
	Allegati     []AllegatoUI
	Portale      []db.RiferimentoPortale
	Bozze        []db.Bozza
	Thread       *db.ThreadOfferta
	Avviso       string
}

// Agganciato: il messaggio appartiene a una RFQ (i download sono consentiti).
func (d *messaggioDati) Agganciato() bool { return d.Thread != nil }

// allegatiVista e rigaAllegato sono i dati passati ai frammenti "allegati_tabella" e "allegato_riga",
// condivisi fra il pannello del messaggio e la schermata B.
type allegatiVista struct {
	Allegati      []AllegatoUI
	MessaggioID   uuid.UUID
	Agganciato    bool
	RitornaThread string
	NScaricabili  int
}

type rigaAllegato struct {
	A             AllegatoUI
	Agganciato    bool
	RitornaThread string
	Figlio        bool
}

func (s *Server) messaggio(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaMessaggio(r.Context(), id, sessioneDa(r.Context()))
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

func (s *Server) caricaMessaggio(ctx context.Context, id uuid.UUID, sess sessioneUI) (*messaggioDati, error) {
	q := db.New(s.Pool)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &messaggioDati{M: m}
	d.Riga, _ = q.GetInboxRiga(ctx, id)
	d.Presenze, _ = q.ListPresenze(ctx, id)
	if len(d.Presenze) > 0 {
		d.Copia, d.MotivoAzioni = s.copiaInterattiva(ctx, q, id, sess)
	}
	if m.ThreadID.Valid {
		if t, err := q.GetThread(ctx, m.ThreadID.UUID); err == nil {
			d.Thread = &t
		}
	}
	d.Allegati, _ = s.allegatiUI(ctx, q, id)
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

// copiaInterattiva decide su quale copia agisce un job interattivo chiesto in QUESTA sessione, o
// spiega perché non può: senza postazione, o senza un worker idoneo su quella postazione. Il motivo
// è per l'operatore, quindi è una frase e non un codice.
func (s *Server) copiaInterattiva(ctx context.Context, q *db.Queries, id uuid.UUID, sess sessioneUI) (*jobs.Copia, string) {
	var richiedente uuid.UUID
	if sess.Utente != nil {
		richiedente = sess.Utente.UtenteID
	}
	c, err := jobs.CopiaPerPostazione(ctx, q, id, sess.Postazione, richiedente)
	switch {
	case err == nil:
		return &c, ""
	case errors.Is(err, jobs.ErrNessunaPostazione):
		return nil, "nessuna postazione associata a questa sessione: scegli dalla testata il PC su cui stai lavorando"
	}
	var nessuno *jobs.ErrNessunWorkerIdoneo
	if errors.As(err, &nessuno) {
		return nil, nessuno.Motivo()
	}
	return nil, err.Error()
}

// accodaInterattivo è il percorso comune di «Apri in Outlook» e «Segna letto» (voci 2.2, 2.7, M3):
// la copia servita dalla postazione della sessione, il job con casella + postazione + richiedente +
// scadenza. Se non c'è una copia servibile NON accoda niente e restituisce il motivo (M4, M10): un
// job «alla prima copia disponibile» aprirebbe la finestra su un altro PC.
func (s *Server) accodaInterattivo(ctx context.Context, q *db.Queries, id uuid.UUID, sess sessioneUI, tipo db.TipoJob, payload func(jobs.Copia, db.Messaggio) any, priorita int16) (*jobs.Copia, string, error) {
	c, motivo := s.copiaInterattiva(ctx, q, id, sess)
	if c == nil {
		return nil, motivo, nil
	}
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if _, err := jobs.AccodaCon(ctx, q, tipo, payload(*c, m), "", priorita, jobs.OpzioniInterattive(tipo, *c, sess.Postazione, sess.Utente.UtenteID)); err != nil {
		if errors.Is(err, jobs.ErrShadow) {
			return nil, motivoShadow(tipo), nil
		}
		return nil, "", err
	}
	return c, "", nil
}

// motivoShadow è la frase che l'operatore legge quando preme un pulsante che in shadow non parte.
// Dice tre cose: che cosa non è successo, perché, e dove si cambia — perché un rifiuto senza la
// terza è indistinguibile da un guasto.
func motivoShadow(tipo db.TipoJob) string {
	return fmt.Sprintf("il server è in modalità shadow (sola lettura verso Outlook e NAS): %s non viene eseguito. "+
		"Si cambia con [server].modalita = \"produzione\" in cockpit.toml", tipo)
}

// apriInOutlook accoda apri_elemento_outlook con priorità massima: il worker della postazione della
// sessione fa Display() sull'elemento nella casella che serve.
func (s *Server) apriInOutlook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	sess := sessioneDa(r.Context())
	c, motivo, err := s.accodaInterattivo(r.Context(), db.New(s.Pool), id, sess, db.TipoJobApriElementoOutlook, func(c jobs.Copia, m db.Messaggio) any {
		return api.PayloadApriElemento{EntryID: c.EntryID, RiferimentoElemento: rifIn(m, c.CasellaID)}
	}, 1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if c == nil {
		s.avvisoErrore(w, "Non aperto: "+motivo)
		return
	}
	s.avviso(w, fmt.Sprintf("Richiesta inviata a Outlook: la copia in %s si apre su %s.", c.CasellaNome, sess.NomeHost))
}

func (s *Server) segnaLetto(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	letto := r.FormValue("letto") != "0"
	sess := sessioneDa(r.Context())
	c, motivo, err := s.accodaInterattivo(r.Context(), q, id, sess, db.TipoJobSegnaLetto, func(c jobs.Copia, m db.Messaggio) any {
		return api.PayloadSegnaLetto{EntryID: c.EntryID, Letto: letto, RiferimentoElemento: rifIn(m, c.CasellaID)}
	}, 2)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if c == nil {
		s.avvisoErrore(w, "Non aggiornato: "+motivo)
		return
	}
	// Si segna letta la COPIA su cui si è agito, non «il messaggio»: le altre caselle hanno il loro
	// stato di lettura e nessuno ha chiesto di toccarlo.
	if err := q.SetNonLettoPresenza(r.Context(), db.SetNonLettoPresenzaParams{MessaggioID: id, CasellaID: c.CasellaID, NonLetto: !letto}); err != nil {
		s.Log.Warn("stato di lettura non aggiornato", "messaggio", id, "err", err)
	}
	s.avviso(w, "Aggiornato in Outlook ("+c.CasellaNome+").")
}

// bozza prepara la risposta nel Cockpit e la apre come bozza in Outlook (l'invio resta manuale, SPEC §6.1).
func (s *Server) bozza(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	sess := sessioneDa(r.Context())
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
	// La bozza si apre in Outlook sul PC del richiedente: stessa regola di «Apri», nessun ripiego.
	pr, motivo := s.copiaInterattiva(ctx, q, id, sess)
	if pr == nil {
		s.avvisoErrore(w, "Bozza non preparata: "+motivo)
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
	if _, err := jobs.AccodaCon(ctx, q, db.TipoJobCreaBozzaOutlook, api.PayloadCreaBozza{
		BozzaID: b.BozzaID, Tipo: string(tipo), EntryID: pr.EntryID, Destinatari: []api.Destinatario{},
		CorpoHTML: html, CorpoTesto: corpo, Allegati: []string{}, Mostra: true, Invia: false, RiferimentoElemento: rifIn(m, pr.CasellaID),
	}, "bozza:"+b.BozzaID.String(), 1, jobs.OpzioniInterattive(db.TipoJobCreaBozzaOutlook, *pr, sess.Postazione, u.UtenteID)); err != nil {
		if errors.Is(err, jobs.ErrShadow) {
			// niente Commit: la bozza non è stata preparata, e una riga `bozza` senza la finestra in
			// Outlook sarebbe una risposta che l'operatore crede di avere e non ha
			s.avvisoErrore(w, "Bozza non preparata: "+motivoShadow(db.TipoJobCreaBozzaOutlook))
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.avviso(w, fmt.Sprintf("Bozza in preparazione: si apre in Outlook su %s tra pochi secondi. Rileggi e premi Invia lì.", sess.NomeHost))
}

// rif costruisce il riferimento stabile all'elemento (Message-ID) per i job che lo devono ritrovare in Outlook.
func rif(m db.Messaggio) api.RiferimentoElemento {
	id := m.MessaggioID
	return api.RiferimentoElemento{MessaggioID: &id, MessageID: m.ChiaveEsterna}
}

// rifIn è rif per un'azione su UNA copia: porta anche la casella, così quando il worker ritrova
// l'elemento a un EntryID diverso il server riallinea quella presenza e non un'altra.
func rifIn(m db.Messaggio, casella uuid.UUID) api.RiferimentoElemento {
	r := rif(m)
	c := casella
	r.CasellaID = &c
	return r
}

// avviso risponde con un frammento al posto del pulsante che è stato premuto: chi ha premuto legge
// che cosa è successo, invece di vedere una pagina che non cambia.
func (s *Server) avviso(w http.ResponseWriter, testo string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="avviso">%s</div>`, template.HTMLEscapeString(testo))
}

// avvisoErrore è avviso per un'azione che NON è partita: stesso posto, colore diverso. Risponde 200
// perché è un frammento HTMX da mostrare, non un errore del server.
func (s *Server) avvisoErrore(w http.ResponseWriter, testo string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="avviso errore-box">%s</div>`, template.HTMLEscapeString(testo))
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
	_, err = db.New(s.Pool).RiaccodaJob(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		// C'è già un job pendente con la stessa chiave di idempotenza (o il job non è chiuso).
		// Riaccodarlo violerebbe l'indice unico parziale: prima era un 500, adesso è una frase.
		s.avviso(w, "non riaccodato: ce n'è già uno in coda con la stessa chiave")
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// annullaJob è «Annulla» (Q17): un job pronto o in corso che l'operatore non vuole più. Se è in
// corso il tentativo lo scopre al prossimo heartbeat (409) e si ferma.
func (s *Server) annullaJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id", 400)
		return
	}
	_, err = db.New(s.Pool).AnnullaJob(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.avviso(w, "non annullato: il job è già chiuso")
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Log.Info("job annullato dall'operatore", "job", id, "utente", utenteDa(r.Context()).Sigla)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

type scartiDati struct {
	Conta   []db.ContaIngestScartiPerOrigineRow
	Scarti  []db.IngestScarto
	Origine string
}

// adminScarti mostra gli elementi che l'ingest ha rifiutato. Le due origini sono due sezioni diverse
// perché si riprovano in due modi diversi: dal payload in database, o rileggendo da Outlook.
func (s *Server) adminScarti(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	var origine pgtype.Text
	if o := r.URL.Query().Get("origine"); o == "ingest" || o == "lettura" {
		origine = pgtype.Text{String: o, Valid: true}
	}
	lista, err := q.ListIngestScarti(r.Context(), db.ListIngestScartiParams{Origine: origine, Limit: 200})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaIngestScartiPerOrigine(r.Context())
	s.rendi(w, r, "scarti.html", "scarti_tabella", "Scarti", scartiDati{Conta: conta, Scarti: lista, Origine: r.URL.Query().Get("origine")})
}

func (s *Server) riprovaScarto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id", 400)
		return
	}
	if s.Ingest == nil {
		http.Error(w, "servizio di ingest non disponibile", 500)
		return
	}
	esito, err := s.Ingest.Riprova(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.avviso(w, "scarto non trovato: forse è già stato ripreso")
		return
	}
	if err != nil {
		s.Log.Error("riprova scarto", "scarto", id, "err", err)
		s.avviso(w, "non riuscita: "+err.Error())
		return
	}
	s.Log.Info("scarto ripreso", "scarto", id, "esito", esito)
	s.avviso(w, esito)
}
