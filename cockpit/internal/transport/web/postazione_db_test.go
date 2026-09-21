//go:build integrazione

// L4 — voce 2.7 (W14, P1) e 2.2 lato UI (M3, M4, M10): la sessione ha una postazione, abbinata per
// IP o scelta in testata, e le azioni interattive partono SOLO da lì.
//
// Gli IP sono simulati: entrambi i server leggono l'indirizzo da un header di prova (IndirizzoClient),
// che in produzione non esiste — lì conta RemoteAddr. È l'unico modo di avere «due PC» su una
// macchina sola; ciò che NON prova (il browser vero su un PC vero) è L7, in esiti_reali.md.
package web

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/testutil"
	"promatec/cockpit/internal/transport/workerapi"
)

const tokenWorkerProva = "token-worker-di-prova"

// tokenDelWorker: dalla voce 2.4 ogni worker ha il suo segreto, e il server lo usa per sapere CHI
// sta chiamando. Un token solo per tutti non esiste più, nemmeno nei banchi di prova.
func tokenDelWorker(nome string) string {
	switch nome {
	case "outlook@PC-FRANCESCO":
		return tokenWorkerProva + "-francesco"
	case "outlook@PC-LUIGI":
		return tokenWorkerProva + "-luigi"
	}
	return tokenWorkerProva
}

type bancoWeb struct {
	t                             *testing.T
	ctx                           context.Context
	pool                          *pgxpool.Pool
	q                             *db.Queries
	srv                           *httptest.Server
	ws                            *Server
	commerciale, francesco, luigi uuid.UUID
	pcFrancesco, pcLuigi          uuid.UUID
	// cfg: la configurazione da cui sono nate le fondazioni. La tiene il banco perché «riavviare il
	// server» significa esattamente riapplicarla (b.riavvia): è così che si prova che cosa succede ai
	// segreti già in database al riavvio successivo.
	cfg *config.Config
}

// riavvia rifà il seed con la configurazione corrente: è ciò che fa cockpit.exe a ogni avvio.
func (b *bancoWeb) riavvia() {
	b.t.Helper()
	if _, err := fondazioni.Semina(b.ctx, b.q, b.cfg, testutil.LogSilenzioso()); err != nil {
		b.t.Fatal(err)
	}
}

func preparaBancoWeb(t *testing.T) *bancoWeb {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	cfg := &config.Config{}
	cfg.Outlook.CasellaDefault = "francesco@azienda.example"
	cfg.Utenti = []config.Utente{
		{Sigla: "FP", Nome: "Francesco", Ufficio: "Commerciale", Ruolo: "operatore", Password: "prova-fp"},
		{Sigla: "LU", Nome: "Luigi", Ufficio: "Tecnico", Ruolo: "tecnico", Password: "prova-lu"},
		{Sigla: "AD", Nome: "Admin", Ufficio: "IT", Ruolo: "admin", Password: "prova-ad"},
		{Sigla: "CO", Nome: "Consultazione", Ufficio: "Direzione", Ruolo: "consultazione", Password: "prova-co"},
	}
	cfg.Caselle = []config.Casella{
		{Indirizzo: "commerciale@azienda.example", Nome: "Commerciale", Canale: "outlook", Condivisa: true},
		{Indirizzo: "francesco@azienda.example", Nome: "Francesco", Canale: "outlook", Utente: "FP"},
		{Indirizzo: "luigi@azienda.example", Nome: "Luigi", Canale: "outlook", Utente: "LU"},
	}
	cfg.Postazioni = []config.Postazione{{NomeHost: "PC-FRANCESCO", Utente: "FP"}, {NomeHost: "PC-LUIGI", Utente: "LU"}}
	cfg.Worker = []config.Worker{
		{Nome: "outlook@PC-FRANCESCO", Tipo: "outlook", Token: tokenWorkerProva + "-francesco", Postazione: "PC-FRANCESCO",
			Caselle: []string{"francesco@azienda.example", "commerciale@azienda.example"}},
		{Nome: "outlook@PC-LUIGI", Tipo: "outlook", Token: tokenWorkerProva + "-luigi", Postazione: "PC-LUIGI", Caselle: []string{"luigi@azienda.example"}},
	}
	utenti := make([]struct{ Sigla, Nome, Ufficio, Ruolo, Password string }, 0)
	for _, u := range cfg.Utenti {
		utenti = append(utenti, struct{ Sigla, Nome, Ufficio, Ruolo, Password string }{u.Sigla, u.Nome, u.Ufficio, u.Ruolo, u.Password})
	}
	if err := SeedUtenti(ctx, q, utenti, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	if _, err := fondazioni.Semina(ctx, q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	ip := func(r *http.Request) string { return r.Header.Get("X-Prova-IP") }
	templ, _ := fs.Sub(risorse.FS, "web/templates")
	static, _ := fs.Sub(risorse.FS, "web/static")
	// Workers: il banco monta lo stesso filesystem incorporato del server vero, perché il pacchetto
	// della postazione (D22) deve contenere i file del worker e non solo la configurazione.
	// SyncAperturaInbox acceso come in produzione (assente in cockpit.toml = true): se accodare un
	// sync alla prima apertura dell'Inbox disturbasse qualcosa, e' qui che si deve vedere, non sul
	// banco reale.
	ws := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Templ: templ, Static: static, IndirizzoClient: ip,
		Workers: risorse.FS, SyncAperturaInbox: true,
		Sync: jobs.SyncOpzioni{Cartelle: []string{"Inbox", "Sent Items"}, Lotto: 50}}
	if err := ws.Init(); err != nil {
		t.Fatal(err)
	}
	wa := &workerapi.Server{Pool: pool, Log: testutil.LogSilenzioso(), Staging: t.TempDir(), IndirizzoClient: ip}
	mux := http.NewServeMux()
	ws.Registra(mux)
	wa.Registra(mux)
	// Il banco monta la stessa protezione CSRF di produzione (voce 2.5): tutto ciò che questi test
	// provano passa di lì, quindi nessuno può aggiungere una rotta che funziona solo senza.
	srv := httptest.NewServer(ProtezioneCSRF(mux))
	t.Cleanup(srv.Close)
	// L'indirizzo lo si sa solo dopo l'avvio: è quello che finisce nel worker.toml del pacchetto.
	ws.Indirizzo = strings.TrimPrefix(srv.URL, "http://")
	b := &bancoWeb{t: t, ctx: ctx, pool: pool, q: q, srv: srv, ws: ws, cfg: cfg}
	casella := func(ind string) uuid.UUID {
		c, err := q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: ind})
		if err != nil {
			t.Fatal(err)
		}
		return c.CasellaID
	}
	b.commerciale, b.francesco, b.luigi = casella("commerciale@azienda.example"), casella("francesco@azienda.example"), casella("luigi@azienda.example")
	post := func(host string) uuid.UUID {
		p, err := q.GetPostazionePerHost(ctx, host)
		if err != nil {
			t.Fatal(err)
		}
		return p.PostazioneID
	}
	b.pcFrancesco, b.pcLuigi = post("PC-FRANCESCO"), post("PC-LUIGI")
	return b
}

// browser è un client con i cookie, che non segue i redirect e che «sta» a un IP.
type browser struct {
	b  *bancoWeb
	c  *http.Client
	ip string
}

func (b *bancoWeb) browser(ip string) *browser {
	jar, _ := cookiejar.New(nil)
	return &browser{b: b, ip: ip, c: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (w *browser) fai(metodo, percorso string, form url.Values, hx bool) (*http.Response, string) {
	w.b.t.Helper()
	var corpo io.Reader
	if form != nil {
		corpo = strings.NewReader(form.Encode())
	}
	r, _ := http.NewRequest(metodo, w.b.srv.URL+percorso, corpo)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hx {
		r.Header.Set("HX-Request", "true")
	}
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return resp, string(testo)
}

func (w *browser) login(sigla, password string) {
	w.b.t.Helper()
	resp, _ := w.fai(http.MethodPost, "/login", url.Values{"sigla": {sigla}, "password": {password}}, false)
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "/inbox" {
		w.b.t.Fatalf("login di %s: %d → %s", sigla, resp.StatusCode, resp.Header.Get("Location"))
	}
}

// sessione legge la postazione della sessione corrente dal database (il cookie è nel jar).
func (w *browser) sessione() db.GetSessioneRow {
	w.b.t.Helper()
	u, _ := url.Parse(w.b.srv.URL)
	for _, c := range w.c.Jar.Cookies(u) {
		if c.Name == cookieSessione {
			s, err := w.b.q.GetSessione(w.b.ctx, c.Value)
			if err != nil {
				w.b.t.Fatal(err)
			}
			return s
		}
	}
	w.b.t.Fatal("nessun cookie di sessione")
	return db.GetSessioneRow{}
}

// workerClaim fa fare un claim al worker di una postazione da un IP: è ciò che registra l'IP.
func (b *bancoWeb) workerClaim(nome, ip string, caselle ...uuid.UUID) {
	b.t.Helper()
	aperte := make([]api.CasellaAperta, 0, len(caselle))
	for _, c := range caselle {
		aperte = append(aperte, api.CasellaAperta{CasellaID: c, StoreID: "STORE-" + c.String()[:8]})
	}
	corpo, _ := json.Marshal(api.ClaimRichiesta{Worker: "outlook", WorkerID: nome, AttesaS: 1, OutlookOk: true, CaselleAperte: aperte})
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", strings.NewReader(string(corpo)))
	r.Header.Set("X-Cockpit-Token", tokenDelWorker(nome))
	r.Header.Set("X-Prova-IP", ip)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		b.t.Fatalf("claim di %s: %d", nome, resp.StatusCode)
	}
}

func (b *bancoWeb) messaggioIn(chiave string, caselle ...uuid.UUID) uuid.UUID {
	b.t.Helper()
	var convID, msgID uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now())
		RETURNING conversazione_id`, "CONV-"+chiave).Scan(&convID); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, mittente_indirizzo)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ W14', 'mario.rossi@acme.example') RETURNING messaggio_id`, chiave, convID).Scan(&msgID); err != nil {
		b.t.Fatal(err)
	}
	for _, c := range caselle {
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, cartella, ricevuto_il)
			VALUES ($1, $2, $3, 'Posta in arrivo', now())`, msgID, c, "ENTRY-"+c.String()[:8]); err != nil {
			b.t.Fatal(err)
		}
	}
	return msgID
}

func (b *bancoWeb) jobInterattivi() []db.Job {
	b.t.Helper()
	righe, err := b.q.ListJob(b.ctx, db.ListJobParams{Limit: 100})
	if err != nil {
		b.t.Fatal(err)
	}
	return righe
}

// jobDiTipo filtra la coda per tipo. Serve da quando la prima apertura dell'Inbox accoda da sola un
// aggiornamento per casella (blocco 3): la coda non contiene piu' solo cio' che il test ha chiesto.
func (b *bancoWeb) jobDiTipo(tipo db.TipoJob) []db.Job {
	b.t.Helper()
	var out []db.Job
	for _, j := range b.jobInterattivi() {
		if j.Tipo == tipo {
			out = append(out, j)
		}
	}
	return out
}

// W14 — (a) login dall'IP registrato dal worker di PC-FRANCESCO → sessione con postazione PC-FRANCESCO,
// origine ip; (b) login da un IP sconosciuto → «Sei su: —», «Apri» non parte con il motivo; scelta
// esplicita → parte, origine scelta; (c) cambio scelta → i job successivi portano la nuova postazione.
func TestW14PostazioneDellaSessione(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	msg := b.messaggioIn("<w14@acme.example>", b.francesco, b.commerciale)

	// (a) stesso IP del worker → abbinata per IP
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	if s := fp.sessione(); !s.PostazioneID.Valid || s.PostazioneID.UUID != b.pcFrancesco || s.PostazioneOrigine.String != "ip" {
		t.Fatalf("(a) sessione: postazione=%v origine=%q, attese PC-FRANCESCO/ip", s.PostazioneID, s.PostazioneOrigine.String)
	}
	_, pagina := fp.fai(http.MethodGet, "/inbox", nil, false)
	if !strings.Contains(pagina, "Sei su:") || !strings.Contains(pagina, `selected>PC-FRANCESCO`) {
		t.Errorf("(a) la testata non mostra la postazione: %s", estratto(pagina, "stato-worker"))
	}
	// M3 via HTTP: «Apri» crea il job con casella, postazione, richiedente, scadenza
	resp, corpo := fp.fai(http.MethodPost, "/messaggio/"+msg.String()+"/apri", nil, true)
	if resp.StatusCode != 200 || !strings.Contains(corpo, "PC-FRANCESCO") || strings.Contains(corpo, "Non aperto") {
		t.Fatalf("(a) apri: %d %s", resp.StatusCode, corpo)
	}
	jobs := b.jobDiTipo(db.TipoJobApriElementoOutlook)
	if len(jobs) != 1 || !jobs[0].PostazioneID.Valid || jobs[0].PostazioneID.UUID != b.pcFrancesco ||
		!jobs[0].CasellaID.Valid || jobs[0].CasellaID.UUID != b.francesco || !jobs[0].RichiestoDa.Valid || jobs[0].ScadeIl == nil {
		t.Fatalf("(a) job di apri: %+v", jobs)
	}
	if strings.Contains(string(jobs[0].Payload), "store_id") {
		t.Errorf("(a) il payload porta uno store_id: %s", jobs[0].Payload)
	}

	// (b) IP sconosciuto → senza postazione; «Apri» NON crea un job e dice perché
	lu := b.browser("10.0.0.99:51000")
	lu.login("LU", "prova-lu")
	if s := lu.sessione(); s.PostazioneID.Valid {
		t.Fatalf("(b) sessione da IP sconosciuto abbinata a %v", s.PostazioneID)
	}
	_, pagina = lu.fai(http.MethodGet, "/inbox", nil, false)
	if !strings.Contains(pagina, `<option value="" selected>—</option>`) || !strings.Contains(pagina, "senza postazione") {
		t.Errorf("(b) la testata non mostra «Sei su: —»: %s", estratto(pagina, "stato-worker"))
	}
	resp, corpo = lu.fai(http.MethodPost, "/messaggio/"+msg.String()+"/apri", nil, true)
	if resp.StatusCode != 200 || !strings.Contains(corpo, "Non aperto") || !strings.Contains(corpo, "nessuna postazione") {
		t.Fatalf("(b) apri senza postazione: %d %s", resp.StatusCode, corpo)
	}
	_, pannello := lu.fai(http.MethodGet, "/messaggio/"+msg.String(), nil, true)
	if !strings.Contains(pannello, "azioni Outlook non disponibili") || !strings.Contains(pannello, `<button disabled`) {
		t.Errorf("(b) il pannello non disabilita le azioni con il motivo: %s", estratto(pannello, "azioni"))
	}
	if n := len(b.jobDiTipo(db.TipoJobApriElementoOutlook)); n != 1 {
		t.Fatalf("(b) creati job senza postazione: %d", n)
	}
	// scelta esplicita di PC-LUIGI (la sua) → origine scelta; ma il messaggio non è in Luigi: M10,
	// nessun job, motivo esplicito, nessuna finestra altrove
	resp, _ = lu.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {b.pcLuigi.String()}}, true)
	if resp.StatusCode != 204 || resp.Header.Get("HX-Refresh") != "true" {
		t.Fatalf("(b) scelta: %d", resp.StatusCode)
	}
	if s := lu.sessione(); !s.PostazioneID.Valid || s.PostazioneID.UUID != b.pcLuigi || s.PostazioneOrigine.String != "scelta" {
		t.Fatalf("(b) sessione dopo la scelta: %v %q", s.PostazioneID, s.PostazioneOrigine.String)
	}
	resp, corpo = lu.fai(http.MethodPost, "/messaggio/"+msg.String()+"/apri", nil, true)
	if resp.StatusCode != 200 || !strings.Contains(corpo, "Non aperto") || !strings.Contains(corpo, "PC-LUIGI") || !strings.Contains(corpo, "non viene dirottata") {
		t.Fatalf("(b/M10) apri da PC-LUIGI su un messaggio che PC-LUIGI non serve: %d %s", resp.StatusCode, corpo)
	}
	if n := len(b.jobDiTipo(db.TipoJobApriElementoOutlook)); n != 1 {
		t.Fatalf("(b/M10) è stato creato un job di ripiego: %d job", n)
	}

	// (c) cambio scelta: l'admin può scegliere qualunque postazione; i job successivi portano la nuova
	ad := b.browser("10.0.0.77:51000")
	ad.login("AD", "prova-ad")
	if resp, _ := ad.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {b.pcLuigi.String()}}, true); resp.StatusCode != 204 {
		t.Fatalf("(c) admin su PC-LUIGI: %d", resp.StatusCode)
	}
	msgLuigi := b.messaggioIn("<w14-luigi@acme.example>", b.luigi)
	if resp, corpo := ad.fai(http.MethodPost, "/messaggio/"+msgLuigi.String()+"/letto", url.Values{"letto": {"1"}}, true); resp.StatusCode != 200 || strings.Contains(corpo, "Non aggiornato") {
		t.Fatalf("(c) letto da PC-LUIGI: %d %s", resp.StatusCode, corpo)
	}
	if resp, _ := ad.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {b.pcFrancesco.String()}}, true); resp.StatusCode != 204 {
		t.Fatalf("(c) admin su PC-FRANCESCO: %d", resp.StatusCode)
	}
	if resp, corpo := ad.fai(http.MethodPost, "/messaggio/"+msg.String()+"/apri", nil, true); resp.StatusCode != 200 || strings.Contains(corpo, "Non aperto") {
		t.Fatalf("(c) apri da PC-FRANCESCO: %d %s", resp.StatusCode, corpo)
	}
	// solo i job interattivi: i sync di apertura non hanno postazione e non c'entrano con W14
	jobs = append(b.jobDiTipo(db.TipoJobApriElementoOutlook), b.jobDiTipo(db.TipoJobSegnaLetto)...)
	perPostazione := map[uuid.UUID]int{}
	for _, j := range jobs {
		if j.PostazioneID.Valid {
			perPostazione[j.PostazioneID.UUID]++
		}
	}
	if len(jobs) != 3 || perPostazione[b.pcLuigi] != 1 || perPostazione[b.pcFrancesco] != 2 {
		t.Fatalf("(c) job per postazione: %v su %d job", perPostazione, len(jobs))
	}
}

// W14 (d) — il browser aperto PRIMA che il worker si accenda.
//
// È l'ordine normale di una mattina: si apre il Cockpit, poi parte il worker. Al login l'IP non
// corrispondeva a nessuna postazione, quindi la sessione è nata senza; senza riabbinamento
// l'operatore resterebbe «sei su: —» per dodici ore, con le azioni su Outlook spente, e l'unico
// rimedio (uscire e rientrare) non lo sa nessuno.
func TestW14LaSessioneSiRiabbinaQuandoIlWorkerSiAccende(t *testing.T) {
	b := preparaBancoWeb(t)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp") // nessun worker ha ancora fatto claim da questo IP
	if s := fp.sessione(); s.PostazioneID.Valid {
		t.Fatalf("sessione abbinata senza nessun worker acceso: %v", s.PostazioneID)
	}

	// si accende il worker di quel PC, dallo stesso indirizzo
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)

	// la richiesta successiva del browser già aperto trova la postazione
	_, pagina := fp.fai(http.MethodGet, "/inbox", nil, false)
	if !strings.Contains(pagina, "selected>PC-FRANCESCO") {
		t.Errorf("la testata non si è riabbinata: %s", estratto(pagina, "stato-worker"))
	}
	s := fp.sessione()
	if !s.PostazioneID.Valid || s.PostazioneID.UUID != b.pcFrancesco || s.PostazioneOrigine.String != "ip" {
		t.Fatalf("sessione dopo il claim: %v %q", s.PostazioneID, s.PostazioneOrigine.String)
	}
	// e adesso «Apri» parte davvero: è questo che l'operatore stava aspettando
	msg := b.messaggioIn("<w14d@acme.example>", b.francesco)
	if resp, corpo := fp.fai(http.MethodPost, "/messaggio/"+msg.String()+"/apri", nil, true); resp.StatusCode != 200 || strings.Contains(corpo, "Non aperto") {
		t.Fatalf("apri dopo il riabbinamento: %d %s", resp.StatusCode, corpo)
	}

	// Una postazione SCELTA non si tocca: l'admin che sta all'IP di PC-FRANCESCO ma ha scelto
	// PC-LUIGI continua a lavorare su PC-LUIGI. È la metà che conta: un riabbinamento che
	// sovrascrivesse una scelta esplicita sposterebbe le finestre di Outlook su un altro PC mentre
	// l'operatore guarda quello che ha scelto lui.
	ad := b.browser("10.0.0.5:51000") // stesso IP del worker di PC-FRANCESCO
	ad.login("AD", "prova-ad")
	if resp, _ := ad.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {b.pcLuigi.String()}}, true); resp.StatusCode != 204 {
		t.Fatal("scelta di PC-LUIGI")
	}
	for i := 0; i < 3; i++ { // tre poll della testata: nessuno deve cambiare idea
		ad.fai(http.MethodGet, "/stato/worker", nil, true)
	}
	if s := ad.sessione(); !s.PostazioneID.Valid || s.PostazioneID.UUID != b.pcLuigi || s.PostazioneOrigine.String != "scelta" {
		t.Errorf("la scelta esplicita è stata sovrascritta dall'IP: %v %q", s.PostazioneID, s.PostazioneOrigine.String)
	}

	// Il «—» invece vuol dire «non lo so», non «non voglio»: riporta la sessione allo stato di
	// partenza, e da un PC che ha il suo worker acceso l'IP torna a dire quale è con certezza. Chi
	// vuole lavorare altrove sceglie l'altra postazione, che è la riga qui sopra. (Distinguere le
	// due cose in database vorrebbe dire un terzo valore in `postazione_origine`, che il CHECK della
	// 0005 non ammette: è un limite dichiarato, non un difetto nascosto.)
	if resp, _ := fp.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {""}}, true); resp.StatusCode != 204 {
		t.Fatal("rimozione della postazione")
	}
	if s := fp.sessione(); s.PostazioneID.Valid {
		t.Fatalf("la rimozione non ha tolto niente: %v", s.PostazioneID)
	}
	fp.fai(http.MethodGet, "/inbox", nil, false)
	if s := fp.sessione(); !s.PostazioneID.Valid || s.PostazioneID.UUID != b.pcFrancesco {
		t.Errorf("dopo il «—» l'IP non ha più ridetto dov'è la sessione: %v", s.PostazioneID)
	}
}

// P1 — l'abbinamento per IP è una comodità, non un'autorizzazione: Luigi che entra dall'IP del
// worker di PC-FRANCESCO NON prende quella postazione; e un utente abilitato a X che chiede Y
// riceve 403.
func TestP1PostazioneSoloSeAutorizzata(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco)
	lu := b.browser("10.0.0.5:51000") // stesso IP del worker di Francesco
	lu.login("LU", "prova-lu")
	if s := lu.sessione(); s.PostazioneID.Valid {
		t.Fatalf("Luigi ha preso la postazione di Francesco per IP: %v", s.PostazioneID)
	}
	resp, corpo := lu.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {b.pcFrancesco.String()}}, true)
	if resp.StatusCode != 403 || !strings.Contains(corpo, "non sei abilitato") {
		t.Fatalf("scelta di una postazione altrui: %d %s", resp.StatusCode, corpo)
	}
	if s := lu.sessione(); s.PostazioneID.Valid {
		t.Fatalf("la scelta rifiutata ha comunque scritto la postazione: %v", s.PostazioneID)
	}
	// la testata gli offre solo la sua
	_, pagina := lu.fai(http.MethodGet, "/inbox", nil, false)
	testata := estratto(pagina, "stato-worker")
	if strings.Contains(testata, ">PC-FRANCESCO<") || !strings.Contains(testata, ">PC-LUIGI<") {
		t.Errorf("la testata offre postazioni non autorizzate: %s", testata)
	}
	// una postazione disattivata non è scegliibile nemmeno dal proprietario
	if _, err := b.pool.Exec(b.ctx, `UPDATE postazione SET attiva = false WHERE postazione_id = $1`, b.pcLuigi); err != nil {
		t.Fatal(err)
	}
	if resp, _ := lu.fai(http.MethodPost, "/sessione/postazione", url.Values{"postazione_id": {b.pcLuigi.String()}}, true); resp.StatusCode != 403 {
		t.Errorf("postazione disattivata scelta: %d", resp.StatusCode)
	}
}

// M2 (testata) — tre stati distinti per casella, letti dalla presenza per worker: Francesco attiva
// (su PC-FRANCESCO), Commerciale non risolta (il worker c'è ma non l'ha trovata), Luigi OFFLINE (il
// suo worker non ha mai fatto claim). Più il caso «non configurata».
func TestM2TestataPerCasella(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco) // Commerciale NON dichiarata
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO casella (canale, indirizzo, nome, condivisa) VALUES ('outlook','archivio@azienda.example','Archivio',true)`); err != nil {
		t.Fatal(err)
	}
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	_, frammento := fp.fai(http.MethodGet, "/stato/worker", nil, true)
	attesi := []string{"Francesco attiva", "Commerciale non risolta", "Luigi OFFLINE", "Archivio non configurata", "analisi non configurata",
		"attiva su PC-FRANCESCO", "non trova questa casella nel proprio profilo", "non è mai stato avviato"}
	for _, a := range attesi {
		if !strings.Contains(frammento, a) {
			t.Errorf("la testata non dice %q:\n%s", a, frammento)
		}
	}
}

// estratto restituisce il pezzo di HTML attorno a un marcatore, per messaggi d'errore leggibili.
func estratto(html, marcatore string) string {
	i := strings.Index(html, marcatore)
	if i < 0 {
		return "(marcatore " + marcatore + " assente)"
	}
	fine := i + 1200
	if fine > len(html) {
		fine = len(html)
	}
	return html[i:fine]
}
