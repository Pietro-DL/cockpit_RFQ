package web

// L1 — giro 5, A1d, V1: il guscio della Distinta unica (distinta_unica_pagina.go, web/templates/distinta_unica.html), con
// httptest e senza database (direttiva 23 §6; EM §9 regole comuni, §12; MAT TS-01, §1.2):
//   - registraPaginaDistintaUnica monta proprio conMotoreA(conNoStore(soloAdmin(p.servi))) e la riga gemella;
//   - a flag spento il 404 del router a tutti, prima della sessione; a flag acceso, senza sessione, il login; gli altri
//     metodi 405 con Allow;
//   - chi non è admin riceve il 403 di nega; l'admin arriva al gestore;
//   - i codici: 400 per un id non valido, 404 per una RFQ che non c'è (anche con HEAD, senza corpo), la pagina 200 con
//     l'errore scritto per una lettura che non riesce (D-18 = A [U]);
//   - all'apertura nessuna scrittura: una sola lettura (la testata), nessuna parola di scrittura nel gestore;
//   - il guscio: la RFQ e l'utente negli attributi, foglio e modulo con l'impronta, nessun attributo htmx che chiami il
//     server, nessun gesto legacy, il testo per chi non ha JavaScript.
//
// Solo dati inventati: UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// duUUID: un UUID inventato riconoscibile.
func duUUID(nn int) uuid.UUID {
	return uuid.MustParse("00000000-0000-4000-8000-" + strings.Repeat("0", 12-len(strconv.Itoa(nn))) + strconv.Itoa(nn))
}

// duServer: il server delle prove del guscio, con i template e gli statici dell'embed, il log scartato e il flag dato.
func duServer(t *testing.T, accesa bool) *Server {
	t.Helper()
	s := serverTest(t)
	s.Log = slog.New(slog.DiscardHandler)
	s.MotoreA = motorea_web.Regole{Accesa: accesa}
	return s
}

// duLettore: una lettura finta della testata che conta le chiamate.
type duLettore struct {
	chiamate int
	testata  testataDistintaUnica
	err      error
}

func (l *duLettore) leggi(_ context.Context, _ uuid.UUID) (testataDistintaUnica, error) {
	l.chiamate++
	return l.testata, l.err
}

// duConUtente: la richiesta con l'utente nel contesto, come lo mette autenticato.
func duConUtente(r *http.Request, ruolo db.RuoloUtente) *http.Request {
	u := &db.Utente{UtenteID: duUUID(77), Sigla: "AD", Nome: "Amministratore di prova", Ruolo: ruolo}
	return r.WithContext(context.WithValue(r.Context(), ctxUtente, u))
}

// Il guscio è montato da registraPaginaDistintaUnica con la catena delle rotte di motore-a.
func TestDistintaUnicaSiMonta(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "distinta_unica_pagina.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	montate := map[string]string{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "registraPaginaDistintaUnica" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "HandleFunc" || sel.Sel.Name == "Handle") {
					montate[types.ExprString(c.Args[0])] = types.ExprString(c.Args[1])
				}
			}
			return true
		})
	}
	attese := map[string]string{
		`"GET " + percorsoDistintaUnica`: "s.conMotoreA(conNoStore(s.soloAdmin(p.servi)))",
		"percorsoDistintaUnica":          "s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA))",
	}
	if len(montate) != len(attese) {
		t.Errorf("registraPaginaDistintaUnica monta %d schemi (%v), attesi %d", len(montate), montate, len(attese))
	}
	for p, g := range attese {
		if montate[p] != g {
			t.Errorf("registraPaginaDistintaUnica: %s montata con %q, attesa %q", p, montate[p], g)
		}
	}
	if percorsoDistintaUnica != "/thread/{id}/distinta-unica" {
		t.Errorf("percorso del guscio cambiato: %q (il resoconto della corsia dice dove sta e perché)", percorsoDistintaUnica)
	}
}

// A flag spento il guscio risponde come un indirizzo che non esiste, a tutti e con ogni metodo, prima della sessione
// (Pool nil: una lettura della sessione andrebbe in panic). A flag acceso, senza sessione, il login; gli altri metodi 405.
func TestDistintaUnicaPortaEMetodi(t *testing.T) {
	id := duUUID(1).String()
	riferimento := httptest.NewRecorder()
	http.NotFound(riferimento, httptest.NewRequest(http.MethodGet, "/non-esiste", nil))
	for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete} {
		s := duServer(t, false)
		mux := http.NewServeMux()
		s.registraPaginaDistintaUnica(mux)
		rec, panico := duServi(mux, httptest.NewRequest(metodo, "/thread/"+id+"/distinta-unica", nil))
		if panico != nil {
			t.Fatalf("flag spento, %s: panic %v (la sessione non va letta)", metodo, panico)
		}
		if rec.Code != http.StatusNotFound || rec.Header().Get("Cache-Control") != "" || rec.Header().Get("Allow") != "" {
			t.Errorf("flag spento, %s: %d %v, atteso il 404 del router senza intestazioni in più", metodo, rec.Code, rec.Header())
		}
		if metodo != http.MethodHead && rec.Body.String() != riferimento.Body.String() {
			t.Errorf("flag spento, %s: corpo %q, atteso quello di un indirizzo che non c'è", metodo, rec.Body.String())
		}
	}
	s := duServer(t, true)
	mux := http.NewServeMux()
	s.registraPaginaDistintaUnica(mux)
	rec, panico := duServi(mux, httptest.NewRequest(http.MethodGet, "/thread/"+id+"/distinta-unica", nil))
	if panico != nil || rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("senza sessione: %d %v %v, atteso 302 verso /login con no-store", rec.Code, rec.Header(), panico)
	}
	req := httptest.NewRequest(http.MethodGet, "/thread/"+id+"/distinta-unica", nil)
	req.Header.Set("HX-Request", "true")
	rec, _ = duServi(mux, req)
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("HX-Redirect") != "/login" {
		t.Errorf("senza sessione con htmx: %d %v", rec.Code, rec.Header())
	}
	for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec, panico := duServi(mux, httptest.NewRequest(metodo, "/thread/"+id+"/distinta-unica", nil))
		if panico != nil || rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s a flag acceso: %d %v %v, atteso 405 con Allow: GET, HEAD", metodo, rec.Code, rec.Header(), panico)
		}
	}
}

func duServi(h http.Handler, req *http.Request) (rec *httptest.ResponseRecorder, panico any) {
	rec = httptest.NewRecorder()
	func() {
		defer func() { panico = recover() }()
		h.ServeHTTP(rec, req)
	}()
	return rec, panico
}

// Solo l'admin: chi non lo è riceve il 403 di nega (con htmx, il frammento: senza, la pagina intera del divieto legge la
// testata dal database), l'admin arriva al gestore.
func TestDistintaUnicaSoloLAdmin(t *testing.T) {
	s := duServer(t, true)
	l := &duLettore{testata: testataDistintaUnica{Oggetto: "RFQ di prova", Riferimento: "ACME-0418", Stato: "APERTA"}}
	p := s.nuovaPaginaDistintaUnica(l.leggi)
	porta := s.conMotoreA(conNoStore(s.soloRuolo(db.RuoloUtenteAdmin, p.servi)))
	id := duUUID(2).String()
	for _, ruolo := range db.AllRuoloUtenteValues() {
		req := duConUtente(httptest.NewRequest(http.MethodGet, "/thread/"+id+"/distinta-unica", nil), ruolo)
		req.SetPathValue("id", id)
		if ruolo != db.RuoloUtenteAdmin {
			req.Header.Set("HX-Request", "true")
		}
		rec, panico := duServi(porta, req)
		if panico != nil {
			t.Fatalf("%s: panic %v", ruolo, panico)
		}
		if ruolo == db.RuoloUtenteAdmin {
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="du"`) {
				t.Errorf("admin: %d, attesa la pagina", rec.Code)
			}
			continue
		}
		if rec.Code != http.StatusForbidden || rec.Header().Get("Cache-Control") != "no-store" || strings.Contains(rec.Body.String(), `id="du"`) {
			t.Errorf("%s: %d %q, atteso il 403 di nega con no-store", ruolo, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}

// I codici del guscio e la lettura unica: 400, 404 (anche con HEAD), la pagina con l'errore per una lettura che non
// riesce, HEAD senza corpo. Una GET legge la testata una volta sola e nient'altro.
func TestDistintaUnicaCodiciELettura(t *testing.T) {
	s := duServer(t, true)
	id := duUUID(3).String()
	chiedi := func(l *duLettore, metodo, idp string) *httptest.ResponseRecorder {
		p := s.nuovaPaginaDistintaUnica(l.leggi)
		req := duConUtente(httptest.NewRequest(metodo, "/thread/"+idp+"/distinta-unica", nil), db.RuoloUtenteAdmin)
		req.SetPathValue("id", idp)
		rec := httptest.NewRecorder()
		p.servi(rec, req)
		return rec
	}
	l := &duLettore{}
	if rec := chiedi(l, http.MethodGet, "non-un-uuid"); rec.Code != http.StatusBadRequest || l.chiamate != 0 {
		t.Errorf("id non valido: %d, %d letture", rec.Code, l.chiamate)
	}
	l = &duLettore{err: errRFQDistintaUnica}
	if rec := chiedi(l, http.MethodGet, id); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), testoDistintaUnicaRFQ) {
		t.Errorf("RFQ inesistente: %d %q", rec.Code, rec.Body.String())
	}
	if rec := chiedi(l, http.MethodHead, id); rec.Code != http.StatusNotFound || rec.Body.Len() != 0 {
		t.Errorf("RFQ inesistente con HEAD: %d, corpo di %d byte", rec.Code, rec.Body.Len())
	}
	l = &duLettore{err: errors.New("connessione rifiutata")}
	rec := chiedi(l, http.MethodGet, id)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "La testata della RFQ non si legge") || strings.Contains(rec.Body.String(), "connessione rifiutata") {
		t.Errorf("lettura non riuscita: %d; attesa la pagina 200 con il testo generico e il dettaglio solo nel log", rec.Code)
	}
	l = &duLettore{testata: testataDistintaUnica{Oggetto: "RFQ di prova"}}
	rec = chiedi(l, http.MethodHead, id)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("HEAD: %d, %d byte, %v", rec.Code, rec.Body.Len(), rec.Header())
	}
	l = &duLettore{testata: testataDistintaUnica{Oggetto: "RFQ di prova"}}
	rec = chiedi(l, http.MethodGet, id)
	if rec.Code != http.StatusOK || l.chiamate != 1 {
		t.Errorf("GET: %d con %d letture, attesa una sola lettura della testata", rec.Code, l.chiamate)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("GET: intestazioni %v", rec.Header())
	}
}

// Il guscio: RFQ e utente negli attributi, foglio e modulo con l'impronta, nessun attributo htmx che chiami il server,
// nessun gesto legacy, il testo per chi non ha JavaScript, nessuno stato inventato.
func TestDistintaUnicaGuscio(t *testing.T) {
	s := duServer(t, true)
	id := duUUID(4).String()
	l := &duLettore{testata: testataDistintaUnica{Oggetto: "RFQ di prova ACME", Riferimento: "ACME-0418", Stato: "APERTA"}}
	p := s.nuovaPaginaDistintaUnica(l.leggi)
	req := duConUtente(httptest.NewRequest(http.MethodGet, "/thread/"+id+"/distinta-unica", nil), db.RuoloUtenteAdmin)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	p.servi(rec, req)
	pagina := rec.Body.String()
	for _, atteso := range []string{
		`<title>Distinta unica · Cockpit RFQ</title>`,
		`data-du-thread="` + id + `"`,
		`data-du-utente="` + duUUID(77).String() + `"`,
		`href="` + s.statico("distinta_unica.css") + `"`,
		`src="` + s.statico("distinta_unica.mjs") + `"`,
		`data-du-pdfjs="` + s.statico("pdfjs-6.3.289/pdf.min.mjs") + `"`,
		`data-du-pdfjs-worker="` + s.statico("pdfjs-6.3.289/pdf.worker.min.mjs") + `"`,
		`type="module"`,
		`id="du-avviso"`, `role="status"`, `id="du-corpo"`, `id="du-barra"`, `id="du-finestra"`, `id="du-wizard"`,
		"<noscript>", "RFQ di prova ACME", "ACME-0418",
	} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("il guscio non ha %q", atteso)
		}
	}
	if !strings.Contains(s.statico("distinta_unica.mjs"), "?v=") || !strings.Contains(s.statico("distinta_unica.css"), "?v=") {
		t.Error("foglio e modulo senza l'impronta del contenuto: Init non li ha visti in web/static")
	}
	inizio := strings.Index(pagina, `<section class="du"`)
	fine := strings.Index(pagina, `<script type="module"`)
	if inizio < 0 || fine < inizio {
		t.Fatal("il guscio non ha la sezione della pagina")
	}
	corpo := pagina[inizio:fine]
	if m := regexp.MustCompile(`(?i)\bhx-(get|post|put|patch|delete|trigger|swap|on)`).FindString(corpo); m != "" {
		t.Errorf("il guscio ha l'attributo htmx %q: nessuna chiamata al server fuori dal ponte", m)
	}
	for _, vietato := range []string{"Conferma l'albero", "/distinta/albero", "/fascicolo/nodo/", "/proposta/", "fascicolo/assegna", "Scarta il file", "data-ma-gesto"} {
		if strings.Contains(corpo, vietato) {
			t.Errorf("il guscio contiene il gesto legacy %q (MAT §7)", vietato)
		}
	}
}

// Il template si compone con le funzioni di tutte le pagine; distinta_unica.html definisce solo «contenuto» (gli altri
// nomi si scontrerebbero con layout e frammenti); ogni {{template}} porta a un define che c'è.
func TestDistintaUnicaTemplateSiCompone(t *testing.T) {
	s := duServer(t, true)
	tt, err := s.templateDistintaUnica()
	if err != nil {
		t.Fatal(err)
	}
	src := duFile(t, "web/templates/distinta_unica.html")
	definiti := regexp.MustCompile(`\{\{define "([^"]+)"\}\}`).FindAllStringSubmatch(src, -1)
	if len(definiti) != 1 || definiti[0][1] != "contenuto" {
		t.Errorf("define di distinta_unica.html: %v, atteso solo «contenuto»", definiti)
	}
	for _, m := range regexp.MustCompile(`\{\{-?\s*template "([^"]+)"`).FindAllStringSubmatch(src, -1) {
		if tt.Lookup(m[1]) == nil {
			t.Errorf("{{template %q}} senza il suo define", m[1])
		}
	}
	// senza i template (un Server costruito a mano) è un errore, mai un panic
	if _, err := (&Server{}).templateDistintaUnica(); err == nil {
		t.Error("templateDistintaUnica senza Templ: atteso un errore")
	}
	p := (&Server{Log: slog.New(slog.DiscardHandler)}).nuovaPaginaDistintaUnica((&duLettore{}).leggi)
	req := httptest.NewRequest(http.MethodGet, "/thread/"+duUUID(5).String()+"/distinta-unica", nil)
	req.SetPathValue("id", duUUID(5).String())
	rec := httptest.NewRecorder()
	p.servi(rec, req)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), testoDistintaUnicaRotta) {
		t.Errorf("template che non si compone: %d %q, atteso il 500 «pagina non disponibile»", rec.Code, rec.Body.String())
	}
}

// All'apertura nessuna scrittura (D-18 = A [U]; D23:337): nel gestore nessuna parola di scrittura né di coda, e la sola
// query è GetThread.
func TestDistintaUnicaNessunaScritturaNelGestore(t *testing.T) {
	src, err := os.ReadFile("distinta_unica_pagina.go")
	if err != nil {
		t.Fatal(err)
	}
	codice := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(src), "")
	for _, v := range []string{".Exec(", "Insert", "Update", "Delete", "Commit", "BeginTx", "Begin(", "BloccaThread", "Accoda", "Prepara", "Tocca", "SetComponente", "SetAllegato"} {
		if strings.Contains(codice, v) {
			t.Errorf("distinta_unica_pagina.go contiene %q: il guscio non scrive e non accoda", v)
		}
	}
	query := regexp.MustCompile(`db\.New\([^)]*\)\.([A-Za-z]+)\(`).FindAllStringSubmatch(codice, -1)
	if len(query) != 1 || query[0][1] != "GetThread" {
		t.Errorf("query del guscio: %v, attesa solo GetThread", query)
	}
}

// duFile: un file dell'embed com'è servito, con i soli LF (i fine riga del checkout dipendono da core.autocrlf).
func duFile(t *testing.T, nome string) string {
	t.Helper()
	b, err := fs.ReadFile(risorse.FS, nome)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}
