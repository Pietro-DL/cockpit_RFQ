package web

// L1 — giro 5, A1d-P4, passo B5.1: la pagina della Distinta nuova (motore_a_pagina.go, web/templates/motore_a.html), con
// httptest e senza database. Si aggiungono alle prove riallineate della PR #18 (motore_a_casi_test.go,
// motore_a_render_test.go, motore_a_statici_test.go) e alle prove della porta di A1d-P2 (motore_a_test.go, dove mwRotte
// ha anche la pagina: flag spento = 404 come un indirizzo che non c'è, acceso senza sessione = login, altri metodi = 405):
//   - registraPaginaMotoreA monta proprio conMotoreA(conNoStore(soloAdmin(s.motoreAPagina))) e la riga gemella, e
//     registraMotoreA la chiama;
//   - chi non è admin riceve il 403 di nega, con no-store; l'admin arriva al gestore;
//   - i codici della pagina: 400 per un id non valido, 404 per una RFQ che non c'è (anche con le regole non pronte, e con
//     HEAD), la pagina con Errore per ogni altro problema (D-18 = A, decisione dell'utente nella direttiva 12 del 13/10;
//     in B5.1 era la raccomandata seguita come scelta tecnica provvisoria; le prove della direttiva 12 sono in
//     motore_a_pagina_d12_test.go);
//   - all'apertura nessuna scrittura: ogni transazione è REPEATABLE READ READ ONLY, nessun Exec, nessun Commit;
//   - guardie statiche: nel template nessuna chiamata al metodo Nodo dell'indice, nessun attributo htmx che chiami il
//     server, ogni data-ma-gesto nella lista chiusa del ponte, nessun indirizzo delle rotte nuove di P4 (R1-R14) né nel
//     template né nel modulo; nel gestore nessuna parola di scrittura;
//   - i posti dei dati di P4 (tipo_visualizzato, STEP secondario, CODICE_DA_ASSEGNARE e «(senza codice)»): senza dati
//     niente, con dati la resa attesa e i gesti spenti con «in arrivo con P4»;
//   - «Accetta il legame» non c'è (Q-A1d-06 = A [U]); «Scarta il legame» è spento.
// Dopo la revisione di B5.1 (07-frontend/2026-10-13-B5-1-revisione.md):
//   - R-01: vista e prodotto nella rilettura del ponte (la query, poi HX-Current-URL della rilettura htmx di #ma);
//   - R-02: le diagnostiche dell'esito del thread su ogni combinazione (il caso e20 ne ha tre);
//   - R-03: nessun posto di P4 e nessun «mono_pezzo» su ogni combinazione finché postiP4Da è vuota; la dichiarazione in
//     bozza resa come etichetta separata; R-04: motivo e numero della conferma, la grafia "finito mono_pezzo" non
//     riconosciuta; R-07, R-14: blocchi e avvisi del riepilogo R4 nel gruppo della distinta in bozza;
//   - R-05: la L4 della pagina (motore_a_pagina_db_test.go) c'è e ha le righe della PR; R-06: Init con funzioniMotoreA;
//   - R-09: «(senza codice)» nella casella, nell'anteprima e nell'aria-label; R-11 (d): il titolo della pagina.
// Le L4 (motore_a_pagina_db_test.go, la pagina che non scrive con le sessioni vere) restano NON ESEGUITE in B5.1.
//
// Solo dati ACME inventati: UUID 00000000-0000-4000-8000-0000000000nn, codici 712xxxx.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/net/html"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// ---------------------------------------------------------------- il montaggio

// La pagina è montata da registraPaginaMotoreA con la catena delle rotte JSON, e registraMotoreA la chiama.
func TestMotoreAPaginaSiMonta(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "motore_a_pagina.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	montate := map[string]string{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "registraPaginaMotoreA" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "HandleFunc" || sel.Sel.Name == "Handle") {
					p, _ := strconv.Unquote(types.ExprString(c.Args[0]))
					montate[p] = types.ExprString(c.Args[1])
				}
			}
			return true
		})
	}
	attese := map[string]string{
		"GET /thread/{id}/motore-a": "s.conMotoreA(conNoStore(s.soloAdmin(s.motoreAPagina)))",
		"/thread/{id}/motore-a":     "s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA))",
	}
	if len(montate) != len(attese) {
		t.Errorf("registraPaginaMotoreA monta %d schemi (%v), attesi %d", len(montate), montate, len(attese))
	}
	for p, g := range attese {
		if montate[p] != g {
			t.Errorf("registraPaginaMotoreA: %q montata con %q, attesa %q", p, montate[p], g)
		}
	}
	src, err := readFileMW("motore_a.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(src, "s.registraPaginaMotoreA(mux)") != 1 {
		t.Error("registraMotoreA non chiama s.registraPaginaMotoreA(mux) una volta sola")
	}
}

// Chi non è admin riceve il 403 di nega con no-store e il gestore non parte; l'admin arriva al gestore: qui le regole non
// sono pronte e il database non c'è, quindi la pagina con l'errore delle regole (D-18 A), in HTML, mai un 403. Per chi
// non è admin solo con htmx: senza, nega rende la pagina intera del divieto con la testata, che legge il database (come
// in TestMotoreASoloLAdmin; la catena con le sessioni vere è delle L4, NON ESEGUITE in B5.1).
func TestMotoreAPaginaSoloLAdmin(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	id := mwUUID(1).String()
	porta := s.conMotoreA(conNoStore(s.soloRuolo(db.RuoloUtenteAdmin, s.motoreAPagina)))
	for _, ruolo := range db.AllRuoloUtenteValues() {
		for _, hx := range []bool{false, true} {
			if !hx && ruolo != db.RuoloUtenteAdmin {
				continue
			}
			req := httptest.NewRequest(http.MethodGet, "/thread/"+id+"/motore-a", nil)
			req.SetPathValue("id", id)
			if hx {
				req.Header.Set("HX-Request", "true")
			}
			req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "acme", Ruolo: ruolo}))
			rec, panico := mwServi(porta, req)
			if panico != nil {
				t.Errorf("%s htmx=%v: panic: %v", ruolo, hx, panico)
				continue
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s htmx=%v: Cache-Control %q", ruolo, hx, rec.Header().Get("Cache-Control"))
			}
			corpo := rec.Body.String()
			if ruolo != db.RuoloUtenteAdmin {
				if rec.Code != http.StatusForbidden || strings.Contains(corpo, "data-ma-pagina") {
					t.Errorf("%s htmx=%v: %d, atteso il 403 di nega senza la pagina", ruolo, hx, rec.Code)
				}
				continue
			}
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
				!strings.Contains(corpo, `data-ma-pagina="`+id+`"`) {
				t.Errorf("admin htmx=%v: %d %q, attesa la pagina: %.300s", hx, rec.Code, rec.Header().Get("Content-Type"), corpo)
			}
			if !strings.Contains(corpo, htmlEsc(s.MotoreA.Errore())) && !strings.Contains(corpo, htmlEsc(testoMotoreARegole)) {
				t.Errorf("admin htmx=%v: la pagina non dice l'errore delle regole", hx)
			}
			// R-11 (d): il titolo non è quello della Distinta legacy («Distinta · Cockpit RFQ»)
			if !hx && (!strings.Contains(corpo, "<title>Distinta nuova · Cockpit RFQ</title>") || strings.Contains(corpo, "<title>Distinta · ")) {
				t.Error("admin: la pagina non ha il titolo «Distinta nuova · Cockpit RFQ»")
			}
		}
	}
}

func htmlEsc(s string) string { return strings.NewReplacer("'", "&#39;", `"`, "&#34;").Replace(s) }

// ---------------------------------------------------------------- i codici e nessuna scrittura

// mpTx: una transazione finta che conta le letture, gli Exec, i Commit e i savepoint. GetThread risponde getThread,
// ogni altra lettura altro; un savepoint (il caricatore) non si apre.
type mpTx struct {
	pgx.Tx
	getThread, altro                    error
	letture, exec, commit, rollback, sp int
}

func (x *mpTx) risposta(sql string) error {
	x.letture++
	if strings.Contains(sql, "name: GetThread ") {
		return x.getThread
	}
	return x.altro
}
func (x *mpTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	return mwRiga{x.risposta(sql)}
}
func (x *mpTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	return nil, x.risposta(sql)
}
func (x *mpTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	x.exec++
	return pgconn.CommandTag{}, errors.New("scrittura nella prova della pagina")
}
func (x *mpTx) Begin(context.Context) (pgx.Tx, error) {
	x.sp++
	return nil, errors.New("savepoint non aperto nella prova della pagina")
}
func (x *mpTx) Commit(context.Context) error   { x.commit++; return nil }
func (x *mpTx) Rollback(context.Context) error { x.rollback++; return nil }

// mpIniziatore: registra le opzioni di ogni transazione aperta.
type mpIniziatore struct {
	tx      *mpTx
	opzioni []pgx.TxOptions
}

func (i *mpIniziatore) BeginTx(_ context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	i.opzioni = append(i.opzioni, o)
	return i.tx, nil
}

// I codici della pagina con un database finto, e nessuna scrittura: ogni transazione in sola lettura e REPEATABLE READ,
// chiusa con ROLLBACK; nessun Exec, nessun Commit.
func TestMotoreAPaginaICodiciENessunaScrittura(t *testing.T) {
	s := mwServer(t, mwPronte(time.Second))
	id := mwUUID(1).String()
	nonPronte := motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente}
	for _, c := range []struct {
		nome          string
		regole        motorea_web.Regole
		getThread     error
		metodo, resto string
		stato         int
		contiene      string
	}{
		{"RFQ che non c'è", mwPronte(time.Second), pgx.ErrNoRows, http.MethodGet, "/motore-a", http.StatusNotFound, testoMotoreARFQPagina},
		{"RFQ che non c'è, HEAD", mwPronte(time.Second), pgx.ErrNoRows, http.MethodHead, "/motore-a", http.StatusNotFound, ""},
		{"RFQ che non c'è, regole non pronte", nonPronte, pgx.ErrNoRows, http.MethodGet, "/motore-a", http.StatusNotFound, testoMotoreARFQPagina},
		{"lettura che non riesce", mwPronte(time.Second), nil, http.MethodGet, "/motore-a?vista=documenti", http.StatusOK, testoMotoreAErrore},
		{"regole non pronte", nonPronte, nil, http.MethodGet, "/motore-a?vista=rfq", http.StatusOK, "data-ma-pagina"},
		{"HEAD di una RFQ che c'è", mwPronte(time.Second), nil, http.MethodHead, "/motore-a", http.StatusOK, ""},
	} {
		ini := &mpIniziatore{tx: &mpTx{getThread: c.getThread, altro: errors.New("lettura ACME non riuscita")}}
		svc := motorea_web.Servizio{Regole: c.regole, Pool: ini, Log: s.Log}
		rec := mwChiama(t, s, s.paginaMotoreA, svc, c.metodo, id, c.resto)
		if rec.Code != c.stato {
			t.Errorf("%s: %d, atteso %d: %.300s", c.nome, rec.Code, c.stato, rec.Body.String())
		}
		if c.contiene != "" && !strings.Contains(rec.Body.String(), c.contiene) {
			t.Errorf("%s: il corpo non contiene %q", c.nome, c.contiene)
		}
		if c.metodo == http.MethodHead && rec.Body.Len() != 0 {
			t.Errorf("%s: HEAD con un corpo", c.nome)
		}
		if c.stato == http.StatusOK && (rec.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
			rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Errorf("%s: intestazioni %s", c.nome, mwIntestazioni(rec.Header()))
		}
		if len(ini.opzioni) == 0 {
			t.Errorf("%s: nessuna transazione aperta (la prova non prova niente)", c.nome)
		}
		for _, o := range ini.opzioni {
			if o.AccessMode != pgx.ReadOnly || o.IsoLevel != pgx.RepeatableRead {
				t.Errorf("%s: transazione %q %q, attesa REPEATABLE READ READ ONLY", c.nome, o.IsoLevel, o.AccessMode)
			}
		}
		if x := ini.tx; x.exec != 0 || x.commit != 0 || x.rollback != len(ini.opzioni) {
			t.Errorf("%s: %d Exec, %d Commit, %d Rollback su %d transazioni: all'apertura la pagina non scrive", c.nome, x.exec,
				x.commit, x.rollback, len(ini.opzioni))
		}
	}
	// id non valido: 400, senza leggere
	ini := &mpIniziatore{tx: &mpTx{}}
	rec := mwChiama(t, s, s.paginaMotoreA, motorea_web.Servizio{Regole: mwPronte(time.Second), Pool: ini, Log: s.Log}, http.MethodGet, "xyz", "/motore-a")
	if rec.Code != http.StatusBadRequest || len(ini.opzioni) != 0 {
		t.Errorf("id non valido: %d, %d transazioni; attesi 400 e nessuna lettura", rec.Code, len(ini.opzioni))
	}
}

// Con l'errore la pagina e il frammento hanno ThreadID, vista e Scrive, l'errore scritto e nessun esito; il frammento
// è un solo elemento con data-ma-pagina, senza #ma, #ma-avviso, script o attributi htmx che chiamano il server: la forma
// che il ponte accetta nella rilettura (P3-PONTE-02).
func TestMotoreAPaginaErroreNelFrammento(t *testing.T) {
	s := mwServer(t, mwPronte(time.Second))
	id := mwUUID(1).String()
	ini := &mpIniziatore{tx: &mpTx{altro: errors.New("lettura ACME non riuscita")}}
	svc := motorea_web.Servizio{Regole: mwPronte(time.Second), Pool: ini, Log: s.Log}
	req := httptest.NewRequest(http.MethodGet, "/thread/"+id+"/motore-a?vista=documenti", nil)
	req.SetPathValue("id", id)
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "AD", Ruolo: db.RuoloUtenteAdmin}))
	rec, panico := mwServi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.paginaMotoreA(w, r, svc) }), req)
	if panico != nil {
		t.Fatal(panico)
	}
	fr := rec.Body.Bytes()
	radici := maRRadici(t, fr)
	if len(radici) != 1 || maRA(radici[0], "data-ma-pagina") != id || maRA(radici[0], "data-ma-vista") != "documenti" {
		t.Fatalf("frammento d'errore: %d radici: %.300s", len(radici), fr)
	}
	mpFrammentoAmmesso(t, "frammento d'errore", fr, id)
	if !bytes.Contains(fr, []byte(testoMotoreAErrore)) {
		t.Error("il frammento d'errore non dice l'errore")
	}
}

// mpFrammentoAmmesso: le condizioni di frammentoNonAmmesso del ponte (motore_a_gesti.mjs), lette sul frammento.
func mpFrammentoAmmesso(t *testing.T, nome string, fr []byte, id string) {
	t.Helper()
	doc := maRDoc(t, fr)
	if n := len(maRTutti(doc, maRCon("", "data-ma-pagina"))); n != 1 {
		t.Errorf("%s: %d elementi data-ma-pagina, atteso uno", nome, n)
	}
	if len(maRID(doc, "ma")) != 0 || len(maRID(doc, "ma-avviso")) != 0 {
		t.Errorf("%s: #ma o #ma-avviso nel frammento", nome)
	}
	vietato := regexp.MustCompile(`^(data-)?hx-(get|post|put|patch|delete|swap-oob|on)`)
	for _, n := range maRTutti(doc, func(*html.Node) bool { return true }) {
		if n.Data == "script" {
			t.Errorf("%s: uno script nel frammento", nome)
		}
		for _, a := range n.Attr {
			if vietato.MatchString(a.Key) {
				t.Errorf("%s: l'attributo %s nel frammento", nome, a.Key)
			}
		}
	}
}

// Ogni frammento dei casi riallineati ha la forma che il ponte accetta nella rilettura.
func TestMotoreAPaginaFrammentiAmmessiDalPonte(t *testing.T) {
	s := maRServer(t)
	maROgniResa(t, s, func(r *maRResa, _ int) {
		mpFrammentoAmmesso(t, r.String(), r.Frammento.Body.Bytes(), r.P.ThreadID.String())
	})
}

// ---------------------------------------------------------------- le guardie statiche

// mpSenzaCommentiTemplate: il template senza i commenti {{/* … */}}.
func mpSenzaCommentiTemplate(s string) string {
	return regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`).ReplaceAllString(s, "")
}

// Le guardie statiche del template, del modulo e del gestore (piano B5.1; P4-0b; IX-05; P3-GO-04).
func TestMotoreAPaginaGuardieStatiche(t *testing.T) {
	leggi := func(nome string) string {
		b, err := fs.ReadFile(risorse.FS, nome)
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	tmpl := mpSenzaCommentiTemplate(leggi("web/templates/motore_a.html"))
	js := leggi("web/static/motore_a.mjs")

	// IX-05: mai il metodo Nodo dell'indice (NodoIn sì)
	if m := regexp.MustCompile(`Indice\.Nodo([^A-Za-z]|$)`).FindString(tmpl); m != "" {
		t.Errorf("il template chiama il metodo Nodo dell'indice (%q): si legge NodoIn con il gruppo", m)
	}
	if !strings.Contains(tmpl, "Indice.NodoIn") || !strings.Contains(tmpl, "Indice.ComponenteDelNodo") {
		t.Error("il template non usa NodoIn o ComponenteDelNodo: la guardia non prova niente")
	}
	// P3-GO-04 e P3-PONTE-02: nessun attributo htmx che chiami il server, mai una form che scriva da sola
	if m := regexp.MustCompile(`(data-)?hx-(get|post|put|patch|delete|swap-oob|on)\b`).FindString(tmpl); m != "" {
		t.Errorf("il template ha %q: dalla pagina nuova al server si va solo con il ponte", m)
	}
	if m := regexp.MustCompile(`(?i)<form[^>]*\b(action|method)=`).FindString(tmpl); m != "" {
		t.Errorf("una form con action o method (%q): i gesti li manda il ponte", m)
	}
	// ogni data-ma-gesto e data-ma-sposta nella lista chiusa del ponte (ROTTE), con i segnaposto normalizzati
	lista := map[string]bool{}
	for _, r := range ponteElenco(t, "ROTTE") {
		lista[normalizza(r)] = true
	}
	gesti := regexp.MustCompile(`data-ma-(gesto|sposta)="([^"]*)"`).FindAllStringSubmatch(tmpl, -1)
	if len(gesti) < 20 {
		t.Errorf("solo %d data-ma-gesto nel template: la guardia non prova niente", len(gesti))
	}
	for _, m := range gesti {
		forma := normalizza(regexp.MustCompile(`\{\{[^}]*\}\}`).ReplaceAllString(m[2], "{}"))
		if !lista[forma] {
			t.Errorf("data-ma-%s=%q: la rotta non è nella lista chiusa del ponte", m[1], m[2])
		}
	}
	for _, v := range []string{"relazione/accetta", "relazione/scarta", "Accetta il legame"} {
		if strings.Contains(tmpl, v) {
			t.Errorf("il template contiene %q (Q-A1d-06 [U]; Q-P3-01)", v)
		}
	}
	// nessun indirizzo delle rotte nuove di P4 (contratto rev. 3 §1.2: R1-R14), né nel template né nel modulo: i loro
	// gesti sono spenti e senza indirizzo finché le rotte non ci sono
	p4 := regexp.MustCompile(`motore-a/(bozza|bom/|intento|conferme|fascicolo/|documento/|riallinea|fonte/|allegato/)`)
	for nome, src := range map[string]string{"motore_a.html": tmpl, "motore_a.mjs": js} {
		if m := p4.FindString(src); m != "" {
			t.Errorf("%s nomina una rotta di P4 che non c'è (%q)", nome, m)
		}
	}
	if n := strings.Count(tmpl, "In arrivo con P4"); n < 15 {
		t.Errorf("%d gesti «In arrivo con P4» nel template: attesi quelli di R1-R5, R7-R14 ed E1", n)
	}
	// il gestore della pagina non scrive: nessuna parola di scrittura nel sorgente
	g, err := readFileMW("motore_a_pagina.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"Exec(", "Commit(", "Insert", "Update", "Delete", "Accoda", "coda.", "Begin(", "Rismista", "Prepara"} {
		if strings.Contains(g, v) {
			t.Errorf("motore_a_pagina.go contiene %q", v)
		}
	}
	// il modulo legge la RFQ da data-ma-pagina e chiama confermaAlbero come il ponte di P3 (firma, base)
	if !strings.Contains(js, "dataset.maPagina") || strings.Contains(js, "maThread") {
		t.Error("motore_a.mjs non legge la RFQ da data-ma-pagina")
	}
	if !strings.Contains(js, "Gesti.confermaAlbero(r.firma, r.albero_firma)") {
		t.Error("motore_a.mjs non chiama confermaAlbero(firma, base) come lo espone il ponte di P3")
	}
	// R-08 della revisione di B5.1: nell'anteprima dello STEP il «già» non ha un paragrafo suo (sta già fra le frasi del
	// server) e il motore si dice a parole, con il codice nel title. Solo come testo: jsdom non c'è (Node assente)
	if regexp.MustCompile(`a\.gia\s*\?`).MatchString(js) || !strings.Contains(js, `title: "motore = " + a.motore`) ||
		strings.Contains(js, "(motore «${a.motore}»)") {
		t.Error("motore_a.mjs: il «già» dell'anteprima dello STEP in un paragrafo suo, o il motore scritto col codice grezzo")
	}
}

// ---------------------------------------------------------------- i posti di P4

// I posti dei dati di P4: senza dati niente; con dati il badge del tipo visualizzato (mai dedotto), l'avviso dello STEP
// secondario e il blocco CODICE_DA_ASSEGNARE con i nodi «(senza codice)», con i gesti spenti. Il caso e3 (BOM di lavoro).
func TestMotoreAPaginaPostiP4(t *testing.T) {
	s := maRServer(t)
	c := leggiCasoMotoreA(t, "e3")
	rif := maRProdotti(c)[0]
	rendi := func(posti postiP4) string {
		p := maRPagina(c, motorea_web.VistaDistinta, rif, true)
		req := httptest.NewRequest(http.MethodGet, "/thread/"+p.ThreadID.String()+"/motore-a", nil)
		req.Header.Set("HX-Request", "true")
		rec := httptest.NewRecorder()
		s.rendiVistaMotoreA(rec, req, paginaMotoreAVista{Pagina: p, P4: posti})
		if rec.Code != http.StatusOK {
			t.Fatalf("resa: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	vuota := rendi(postiP4{})
	for _, v := range []string{`data-ma-posto="tipo_visualizzato"`, `id="ma-step-secondario"`, `id="ma-blocchi-p4"`,
		"finito mono_pezzo", "proposto in bozza", "STEP secondario disponibile"} {
		if strings.Contains(vuota, v) {
			t.Errorf("senza dati di P4 la pagina contiene %q", v)
		}
	}
	if fmt := postiP4Da(nil); fmt.TipoVisualizzato != nil || fmt.AvvisiStepSecondario != nil || fmt.Blocchi != nil {
		t.Error("postiP4Da dà dati: oggi nessuna rotta di P4 li porta")
	}

	all := maU("30")
	piena := rendi(postiP4{
		TipoVisualizzato:     map[string]tipoVisualizzatoP4{rif: {Valore: tipoVisualizzatoMonoPezzo}},
		AvvisiStepSecondario: map[string][]avvisoP4{rif: {{Codice: avvisoStepSecondario, Testo: "Uno STEP del sottoassieme 7120200 è nella RFQ.", Allegato: &all, NomeFile: "7120200_acme.stp"}}},
		Blocchi: map[string][]bloccoP4{rif: {{Codice: bloccoCodiceDaAssegnare, Testo: "Due nodi creati nella bozza non hanno un codice.",
			Nodi: []nodoP4{{Chiave: "n1"}, {Chiave: "n2", Codice: "7120210"}}}}},
		AvvisiR4: map[string][]avvisoP4{rif: {{Codice: avvisoStepSecondario, Testo: "Lo STEP del sottoassieme 7120200 si può esaminare.",
			NomeFile: "7120200_acme.stp"}}},
	})
	doc := maRDoc(t, []byte(piena))
	badge := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "data-ma-posto") == "tipo_visualizzato" })
	if len(badge) != 2 || maRTesto(badge[0]) != "finito mono_pezzo" {
		t.Errorf("badge del tipo visualizzato: %d, %q; attesi due (scheda e albero) «finito mono_pezzo»", len(badge), maRTesto(badge[0]))
	}
	sec := maRID(doc, "ma-step-secondario")
	if len(sec) != 1 || !strings.Contains(maRTesto(sec[0]), "Uno STEP del sottoassieme 7120200 è nella RFQ.") ||
		!strings.Contains(maRTesto(sec[0]), "7120200_acme.stp") {
		t.Fatalf("avviso dello STEP secondario: %d", len(sec))
	}
	mpTuttiSpentiP4(t, "STEP secondario", sec[0], 3)
	bl := maRID(doc, "ma-blocchi-p4")
	if len(bl) != 1 {
		t.Fatal("il blocco CODICE_DA_ASSEGNARE non c'è")
	}
	senza := maRTutti(bl[0], func(n *html.Node) bool { return maRClasse(n, "ma-senza-codice") })
	if len(senza) != 1 || maRTesto(senza[0]) != "(senza codice)" || !strings.Contains(maRTesto(bl[0]), "7120210") {
		t.Errorf("nodi del blocco: %d «(senza codice)», attesi uno e il codice 7120210", len(senza))
	}
	mpTuttiSpentiP4(t, "blocco CODICE_DA_ASSEGNARE", bl[0], 1)
	// R-07, R-14: blocchi e avvisi del riepilogo R4 stanno nel gruppo della distinta in bozza (formato 2), dopo i suoi
	// gesti; la conferma formato 1 («Conferma l'albero», data-ma-azione="albero") resta fuori dal gruppo
	inBozza := func(n *html.Node) bool { return maRClasse(n, "ma-bozza-p4") }
	av := maRID(doc, "ma-avvisi-r4")
	if len(av) != 1 || !strings.Contains(maRTesto(av[0]), "Lo STEP del sottoassieme 7120200 si può esaminare.") ||
		!strings.Contains(maRTesto(av[0]), "7120200_acme.stp") || len(maRTutti(av[0], maRCon("button", ""))) != 0 {
		t.Errorf("avvisi del riepilogo R4: %d, attesi uno con testo e file, senza gesti", len(av))
	}
	for nome, n := range map[string][]*html.Node{"blocchi": bl, "avvisi": av} {
		if len(n) == 1 && maRAntenato(n[0], inBozza) == nil {
			t.Errorf("%s del riepilogo R4 fuori dal gruppo della distinta in bozza (R-07)", nome)
		}
	}
	for _, b := range maRTutti(doc, func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "albero" }) {
		if maRAntenato(b, inBozza) != nil {
			t.Error("la conferma formato 1 sta nel gruppo della distinta in bozza")
		}
	}
	if g := maRTutti(doc, inBozza); len(g) != 1 || !strings.HasPrefix(maRTesto(g[0]), "Distinta in bozza (formato 2)") {
		t.Errorf("gruppo della distinta in bozza: %d", len(g))
	}

	// con figli: il numero dal server, mai contato qui; un valore sconosciuto si dice tale
	for _, c := range []struct {
		tv    tipoVisualizzatoP4
		testo string
	}{
		{tipoVisualizzatoP4{Valore: tipoVisualizzatoConFigli, Figli: 3}, "finito con 3 figli"},
		{tipoVisualizzatoP4{Valore: tipoVisualizzatoConFigli, Figli: 1}, "finito con 1 figlio"},
		{tipoVisualizzatoP4{PropostoInBozza: true}, "mono_pezzo proposto in bozza"},
		{tipoVisualizzatoP4{Valore: "altro"}, "tipo visualizzato «altro» non riconosciuto dalla pagina"},
	} {
		b := (postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: c.tv}}).Badge(rif)
		if b == nil || b.Testo != c.testo {
			t.Errorf("%+v: badge %+v, atteso %q", c.tv, b, c.testo)
		}
	}
	if b := (postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: {}}}).Badge(rif); b != nil {
		t.Errorf("valore vuoto: badge %+v, atteso nessuno", b)
	}
	if !(postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: {Valore: tipoVisualizzatoMonoPezzo, PropostoInBozza: true}}}).InBozza(rif) {
		t.Error("mono_pezzo ufficiale con una dichiarazione in bozza: manca l'etichetta separata (D-T-12)")
	}

	// R-03 (M6): la dichiarazione in bozza accanto al badge ufficiale è un'etichetta separata, nella resa; senza la
	// dichiarazione solo il badge. Ogni etichetta due volte: scheda del prodotto e testa dell'albero.
	for _, c := range []struct {
		bozza           bool
		etichette, prop int
	}{{true, 4, 2}, {false, 2, 0}} {
		posti := postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: {Valore: tipoVisualizzatoMonoPezzo, PropostoInBozza: c.bozza}}}
		doc := maRDoc(t, []byte(rendi(posti)))
		tv := maRTutti(doc, func(n *html.Node) bool { return maRClasse(n, "ma-tipovis") })
		prop := 0
		for _, n := range tv {
			if maRTesto(n) == "mono_pezzo proposto in bozza" {
				prop++
				if maRHa(n, "data-ma-posto") {
					t.Error("l'etichetta «proposto in bozza» porta il posto del badge ufficiale")
				}
			}
		}
		if len(tv) != c.etichette || prop != c.prop {
			t.Errorf("mono_pezzo, in bozza = %v: %d etichette (%d «proposto in bozza»), attese %d (%d)", c.bozza, len(tv), prop,
				c.etichette, c.prop)
		}
	}

	// R-04: il motivo della dichiarazione accanto al badge e il numero della conferma nel titolo, solo se il server li
	// dà; la grafia del piano B3.6 ("finito mono_pezzo") non è riconosciuta: la pagina non indovina
	motivata := rendi(postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: {Valore: tipoVisualizzatoMonoPezzo,
		Motivo: "Pezzo unico ACME: nessun figlio nello STEP.", Conferma: 3}}})
	if n := strings.Count(motivata, "motivo: Pezzo unico ACME: nessun figlio nello STEP."); n != 2 {
		t.Errorf("il motivo della dichiarazione compare %d volte, atteso accanto ai due badge", n)
	}
	if !strings.Contains(motivata, "dalla conferma n. 3 della BOM") {
		t.Error("il numero della conferma non è nel titolo del badge")
	}
	if strings.Contains(rendi(postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: {Valore: tipoVisualizzatoMonoPezzo}}}), "motivo:") {
		t.Error("senza motivo dal server la pagina scrive un motivo")
	}
	if b := (postiP4{TipoVisualizzato: map[string]tipoVisualizzatoP4{rif: {Valore: "finito mono_pezzo"}}}).Badge(rif); b == nil ||
		b.Testo != "tipo visualizzato «finito mono_pezzo» non riconosciuto dalla pagina" {
		t.Errorf("la grafia del piano B3.6: badge %+v, atteso «non riconosciuto» finché il contratto non la fissa (R-04)", b)
	}
}

// mpTuttiSpentiP4: dentro n ci sono almeno quanti pulsanti, tutti spenti, senza indirizzo, con «In arrivo con P4».
func mpTuttiSpentiP4(t *testing.T, nome string, n *html.Node, quanti int) {
	t.Helper()
	bottoni := maRTutti(n, maRCon("button", ""))
	if len(bottoni) < quanti {
		t.Errorf("%s: %d pulsanti, attesi almeno %d", nome, len(bottoni), quanti)
	}
	for _, b := range bottoni {
		if !maRSpento(b) || maRHa(b, "data-ma-gesto") || maRHa(b, "data-ma-azione") || !strings.HasPrefix(maRA(b, "title"), "In arrivo con P4") {
			t.Errorf("%s: il pulsante %q non è spento con «In arrivo con P4»", nome, maRTesto(b))
		}
	}
}

// Su ogni combinazione dei casi: nessun «Accetta il legame»; «Scarta il legame» solo spento; i gesti che vogliono le
// rotte di P4 spenti, senza indirizzo; Archivia e Rimuovi del prodotto della RFQ spenti (Q-P3-04, base A [T]).
func TestMotoreAPaginaGestiSpentiERimossi(t *testing.T) {
	s := maRServer(t)
	visti := map[string]int{}
	maROgniResa(t, s, func(r *maRResa, _ int) {
		doc := maRDoc(t, r.Frammento.Body.Bytes())
		for _, b := range maRTutti(doc, maRCon("button", "")) {
			testo := maRTesto(b)
			switch {
			case strings.Contains(testo, "Accetta il legame"):
				t.Errorf("%s: «Accetta il legame» c'è", r)
			case testo == "Scarta il legame":
				visti["scarta"]++
				if !maRSpento(b) || maRHa(b, "data-ma-gesto") {
					t.Errorf("%s: «Scarta il legame» non è spento", r)
				}
			case strings.HasPrefix(maRA(b, "title"), "In arrivo con P4"):
				visti["p4"]++
				if !maRSpento(b) || maRHa(b, "data-ma-gesto") {
					t.Errorf("%s: %q non è spento", r, testo)
				}
			case strings.Contains(maRA(b, "title"), "Q-P3-04"):
				visti["prodotto"]++
				if !maRSpento(b) {
					t.Errorf("%s: %q sul prodotto della RFQ non è spento", r, testo)
				}
			}
		}
		// R-03 (M5): finché postiP4Da è vuota, nessun posto di P4 e nessun «mono_pezzo» in nessuna combinazione, né nella
		// pagina né nel frammento. Un badge dedotto dal server (per esempio per i prodotti senza nodi: «nessun automatismo
		// dall'assenza di figli», E2.12 [U]) qui si vede
		for nome, corpo := range map[string]string{"pagina": r.Pagina.Body.String(), "frammento": r.Frammento.Body.String()} {
			for _, v := range []string{"data-ma-posto=", "mono_pezzo", "ma-tipovis"} {
				if strings.Contains(corpo, v) {
					t.Errorf("%s, %s: %q senza dati di P4", r, nome, v)
				}
			}
		}
		// Archivia e Rimuovi con il componente del prodotto non partono mai
		if p := maRProdottoScelto(r); p != nil && p.ComponenteID != nil {
			for _, suffisso := range []string{"/archivia", "/rimuovi"} {
				g := "/thread/" + r.P.ThreadID.String() + "/fascicolo/componente/" + p.ComponenteID.String() + suffisso
				if strings.Contains(r.Frammento.Body.String(), `data-ma-gesto="`+g+`"`) {
					t.Errorf("%s: %s sul prodotto della RFQ", r, suffisso)
				}
			}
		}
	})
	if visti["p4"] == 0 || visti["prodotto"] == 0 {
		t.Errorf("gesti visti: %v; la prova non prova niente", visti)
	}
	t.Logf("gesti spenti visti: %v", visti)
}

// maRProdottoScelto: il prodotto valutato scelto nella combinazione; nil senza esito.
func maRProdottoScelto(r *maRResa) *valutazioneProdotto {
	if r.P.Esito == nil {
		return nil
	}
	for i := range r.P.Esito.ProdottiValutati {
		if r.P.Esito.ProdottiValutati[i].Rif == r.Prodotto {
			return &valutazioneProdotto{ComponenteID: r.P.Esito.ProdottiValutati[i].ComponenteID}
		}
	}
	return nil
}

// valutazioneProdotto: del prodotto valutato serve qui solo il componente.
type valutazioneProdotto struct{ ComponenteID *uuid.UUID }

// Il modulo e il foglio di stile della pagina si servono con il tipo giusto, e la pagina li carica con l'impronta del
// contenuto (statico), il ponte con l'import map (nella PR in statici_test.go; qui per non toccare un file comune).
func TestMotoreAPaginaStatici(t *testing.T) {
	s := serverTest(t)
	h := http.StripPrefix("/static/", s.statici())
	for _, c := range []struct{ nome, tipo string }{
		{"motore_a.mjs", "text/javascript; charset=utf-8"},
		{"motore_a.css", "text/css; charset=utf-8"},
		{"motore_a_gesti.mjs", "text/javascript; charset=utf-8"},
	} {
		for _, percorso := range []string{"/static/" + c.nome, s.statico(c.nome)} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, percorso, nil))
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != c.tipo {
				t.Errorf("%s: %d %q, atteso 200 %q", percorso, rec.Code, rec.Header().Get("Content-Type"), c.tipo)
			}
		}
		if !strings.Contains(s.statico(c.nome), "?v=") {
			t.Errorf("%s senza impronta: %s", c.nome, s.statico(c.nome))
		}
	}
	tt, err := s.templateMotoreA()
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := tt.ExecuteTemplate(&b, "contenuto", vista{Titolo: "Distinta", Dati: paginaMotoreAVista{Pagina: &motorea_web.Pagina{ThreadID: mwUUID(1)}}}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{`src="` + s.statico("motore_a.mjs") + `"`, `href="` + s.statico("motore_a.css") + `"`,
		`"/static/motore_a_gesti.mjs":"` + s.statico("motore_a_gesti.mjs") + `"`} {
		if !strings.Contains(b.String(), v) {
			t.Errorf("la pagina non contiene %s", v)
		}
	}
}

// ---------------------------------------------------------------- dopo la revisione di B5.1

// R-01: vista e prodotto della rilettura del ponte. La query vince; senza query, solo la rilettura htmx di #ma
// (HX-Request "true", HX-Target "ma") li prende da HX-Current-URL, e solo se è la stessa pagina (stesso percorso, a meno
// della / finale; lo stesso host quando c'è). httptest.NewRequest dà l'host example.com.
func TestMotoreAPaginaVistaEProdottoNellaRilettura(t *testing.T) {
	id := mwUUID(1).String()
	pagina := "/thread/" + id + "/motore-a"
	rilettura := func(corrente string) map[string]string {
		return map[string]string{"HX-Request": "true", "HX-Target": "ma", "HX-Current-URL": corrente}
	}
	for _, c := range []struct {
		nome, indirizzo string
		intestazioni    map[string]string
		vista, prodotto string
	}{
		{"query", pagina + "?vista=documenti&prodotto=P2", nil, "documenti", "P2"},
		{"query e rilettura: vince la query", pagina + "?vista=rfq", rilettura("http://example.com" + pagina + "?vista=documenti&prodotto=P2"), "rfq", ""},
		{"rilettura del ponte", pagina, rilettura("http://example.com" + pagina + "?prodotto=P2&vista=documenti"), "documenti", "P2"},
		{"rilettura, host con un'altra grafia", pagina, rilettura("http://EXAMPLE.com" + pagina + "/?vista=rfq"), "rfq", ""},
		{"rilettura, indirizzo senza host", pagina, rilettura(pagina + "?prodotto=P3"), "", "P3"},
		{"rilettura della pagina senza query", pagina, rilettura("http://example.com" + pagina), "", ""},
		{"senza HX-Target", pagina, map[string]string{"HX-Request": "true", "HX-Current-URL": pagina + "?vista=rfq"}, "", ""},
		{"un altro bersaglio", pagina, map[string]string{"HX-Request": "true", "HX-Target": "ma-avviso", "HX-Current-URL": pagina + "?vista=rfq"}, "", ""},
		{"senza htmx", pagina, map[string]string{"HX-Target": "ma", "HX-Current-URL": pagina + "?vista=rfq"}, "", ""},
		{"un'altra pagina", pagina, rilettura("/thread/" + id + "/distinta?vista=documenti&prodotto=P2"), "", ""},
		{"un'altra RFQ", pagina, rilettura("/thread/" + mwUUID(2).String() + "/motore-a?vista=documenti"), "", ""},
		{"un altro host", pagina, rilettura("http://altro.example" + pagina + "?vista=documenti"), "", ""},
		{"un indirizzo che non si legge", pagina, rilettura("http://example.com/%zz?vista=documenti"), "", ""},
	} {
		req := httptest.NewRequest(http.MethodGet, c.indirizzo, nil)
		for k, v := range c.intestazioni {
			req.Header.Set(k, v)
		}
		if v, p := vistaEProdottoMotoreA(req); v != c.vista || p != c.prodotto {
			t.Errorf("%s: vista %q, prodotto %q; attesi %q, %q", c.nome, v, p, c.vista, c.prodotto)
		}
	}

	// dal gestore: la rilettura senza query di una pagina che non si calcola tiene la vista dell'indirizzo mostrato (il
	// frammento d'errore ha la vista; il prodotto lo sceglie il servizio, che qui non legge)
	s := mwServer(t, mwPronte(time.Second))
	ini := &mpIniziatore{tx: &mpTx{altro: errors.New("lettura ACME non riuscita")}}
	svc := motorea_web.Servizio{Regole: mwPronte(time.Second), Pool: ini, Log: s.Log}
	req := httptest.NewRequest(http.MethodGet, pagina, nil)
	req.SetPathValue("id", id)
	for k, v := range rilettura("http://example.com" + pagina + "?prodotto=P2&vista=documenti") {
		req.Header.Set(k, v)
	}
	req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "AD", Ruolo: db.RuoloUtenteAdmin}))
	rec, panico := mwServi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.paginaMotoreA(w, r, svc) }), req)
	if panico != nil {
		t.Fatal(panico)
	}
	radici := maRRadici(t, rec.Body.Bytes())
	if len(radici) != 1 || maRA(radici[0], "data-ma-vista") != "documenti" {
		t.Errorf("rilettura senza query: %d radici, vista %q; attesa documenti", len(radici), maRA(radici[0], "data-ma-vista"))
	}
}

// R-02 (P4-0c, F-07): le diagnostiche dell'esito del thread, su ogni combinazione, nella pagina e nel frammento: c'è il
// riquadro solo se il server ne dà; una riga per diagnostica, nell'ordine del server; codice, gravità e natura dalle
// etichette, il codice grezzo nel title, il messaggio così com'è, il tono della gravità. e20 ne ha tre.
func TestMotoreAPaginaDiagnostiche(t *testing.T) {
	s := maRServer(t)
	tono := map[evidenze.Gravita]string{evidenze.GravitaErrore: "ma-st--ko", evidenze.GravitaAvviso: "ma-st--ver", evidenze.GravitaNota: "ma-st--neu"}
	viste := 0
	maROgniResa(t, s, func(r *maRResa, _ int) {
		var attese []evidenze.Diagnostica
		if r.P.Esito != nil {
			attese = r.P.Esito.Diagnostiche
		}
		for nome, rec := range map[string]*httptest.ResponseRecorder{"pagina": r.Pagina, "frammento": r.Frammento} {
			doc := maRDoc(t, rec.Body.Bytes())
			riquadro := maRID(doc, "ma-diagnostiche")
			if len(attese) == 0 {
				if len(riquadro) != 0 {
					t.Errorf("%s, %s: il riquadro delle diagnostiche senza diagnostiche", r, nome)
				}
				continue
			}
			viste++
			if len(riquadro) != 1 {
				t.Errorf("%s, %s: %d riquadri delle diagnostiche, atteso uno", r, nome, len(riquadro))
				continue
			}
			if titolo := fmt.Sprintf("Diagnostiche della valutazione del thread (%d)", len(attese)); !strings.Contains(maRTesto(riquadro[0]), titolo) {
				t.Errorf("%s, %s: manca %q", r, nome, titolo)
			}
			righe := maRTutti(riquadro[0], func(n *html.Node) bool { return maRHa(n, "data-ma-diagnostica") })
			if len(righe) != len(attese) {
				t.Errorf("%s, %s: %d diagnostiche, attese %d", r, nome, len(righe), len(attese))
				continue
			}
			for i, d := range attese {
				riga := righe[i]
				if maRHa(riga, "hidden") || maRAntenato(riga, func(n *html.Node) bool { return maRHa(n, "hidden") }) != nil {
					t.Errorf("%s, %s: la diagnostica %s è nascosta (nessun filtro nel template)", r, nome, d.Codice)
				}
				if maRA(riga, "data-ma-diagnostica") != d.Codice {
					t.Errorf("%s, %s: diagnostica %d %q, attesa %q (l'ordine è del server)", r, nome, i, maRA(riga, "data-ma-diagnostica"), d.Codice)
				}
				testo := maRTesto(riga)
				for _, v := range []string{motorea_web.Etichetta("codice_diagnostica", d.Codice), motorea_web.Etichetta("gravita_diagnostica", d.Gravita),
					motorea_web.Etichetta("natura_diagnostica", d.Natura), d.Messaggio} {
					if !strings.Contains(testo, v) {
						t.Errorf("%s, %s: la diagnostica %s non dice %q (%q)", r, nome, d.Codice, v, testo)
					}
				}
				if len(maRTutti(riga, func(n *html.Node) bool { return strings.HasPrefix(maRA(n, "title"), "codice = "+d.Codice) })) != 1 {
					t.Errorf("%s, %s: il codice grezzo %s non è nel title", r, nome, d.Codice)
				}
				if d.Percorso != "" && len(maRTutti(riga, func(n *html.Node) bool { return strings.Contains(maRA(n, "title"), "percorso = "+d.Percorso) })) != 1 {
					t.Errorf("%s, %s: il percorso %s non è nel title", r, nome, d.Percorso)
				}
				if len(maRTutti(riga, func(n *html.Node) bool { return maRClasse(n, tono[d.Gravita]) })) != 1 {
					t.Errorf("%s, %s: la diagnostica %s senza il tono %s", r, nome, d.Codice, tono[d.Gravita])
				}
			}
		}
	})
	if viste == 0 {
		t.Error("nessuna combinazione con diagnostiche: la prova non prova niente (e20)")
	}
}

// R-05: la L4 della pagina c'è, compila con il tag delle L4 (go vet -tags integrazione) e ha le righe della PR #18 per
// TestNessunaGetScrive: la pagina nelle tre viste, un prodotto che non c'è, il frammento, la rilettura del ponte, HEAD,
// la foto del database prima e dopo per ogni ruolo. È NON ESEGUITA in B5.1 (nessun DB): qui si prova solo che non sparisca.
func TestMotoreAPaginaNellaL4(t *testing.T) {
	src, err := readFileMW("motore_a_pagina_db_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(src, "//go:build integrazione") {
		t.Error("motore_a_pagina_db_test.go senza il tag integrazione")
	}
	for _, v := range []string{"func TestMotoreAPaginaL4NessunaGetScrive(", `"?vista=documenti"`, `"?vista=rfq"`, `"?prodotto=`,
		`"HX-Request": "true"`, `"HX-Target": "ma"`, "HX-Current-URL", "http.MethodHead", "fotoDelDatabase(t, b)", "differenze(prima,",
		"s.op", "s.co", "s.ad"} {
		if !strings.Contains(src, v) {
			t.Errorf("la L4 della pagina non ha %s", v)
		}
	}
}

// R-06: la pagina entra in Init (server.go, dalla direttiva 12). Senza funzioniMotoreA il clone non si compone (e Init
// fermerebbe il server, anche a flag spento); con funzioniMotoreA si compone, e templateMotoreA usa quello. Se server.go
// nomina motore_a.html, deve nominare anche funzioniMotoreA: la prova diventa rossa prima dell'avvio.
func TestMotoreAPaginaInitConLeFunzioni(t *testing.T) {
	s := serverTest(t)
	base, err := template.New("layout").Funcs(funzioni).Funcs(template.FuncMap{"statico": s.statico}).
		ParseFS(s.Templ, "layout.html", "frammenti.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := template.Must(base.Clone()).ParseFS(s.Templ, "motore_a.html"); err == nil || !strings.Contains(err.Error(), `"etichetta" not defined`) {
		t.Errorf("senza funzioniMotoreA il template della pagina si compone (err = %v): la prova non prova niente", err)
	}
	tt, err := template.Must(base.Clone()).Funcs(funzioniMotoreA).ParseFS(s.Templ, "motore_a.html")
	if err != nil {
		t.Fatalf("con funzioniMotoreA il template della pagina non si compone: %v", err)
	}
	s.pagine["motore_a.html"] = tt
	if got, err := s.templateMotoreA(); err != nil || got != tt {
		t.Errorf("con la pagina in Init templateMotoreA non usa quella (err = %v)", err)
	}
	c := leggiCasoMotoreA(t, "e3")
	p := maRPagina(c, motorea_web.VistaDistinta, maRProdotti(c)[0], true)
	rec := httptest.NewRecorder()
	s.rendiMotoreA(rec, httptest.NewRequest(http.MethodGet, "/thread/"+p.ThreadID.String()+"/motore-a", nil), p)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-ma-pagina="`+p.ThreadID.String()+`"`) {
		t.Errorf("la pagina con il template di Init: %d", rec.Code)
	}
	src, err := readFileMW("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(src, `"motore_a.html"`) && !strings.Contains(src, "funzioniMotoreA") {
		t.Error(`server.go nomina "motore_a.html" senza funzioniMotoreA: Init fallirebbe e il server non partirebbe`)
	}
}

// R-09: un nodo senza codice (né confermato, né a mano, né proposto, né letto nel file, senza decisione) si scrive
// «(senza codice)» nella casella, nell'anteprima (titolo e catena) e nell'aria-label della casella. Il caso e3 con il
// codice di un nodo della BOM di lavoro svuotato in memoria (nessun caso dei diciotto ha un nodo senza codice).
func TestMotoreAPaginaSenzaCodiceUnico(t *testing.T) {
	s := maRServer(t)
	c := leggiCasoMotoreA(t, "e3")
	p := &c.Pagina.Esito.ProdottiValutati[0]
	rif := ""
	for i := range p.Nodi {
		n := &p.Nodi[i]
		if len(n.Parentela) == 0 {
			continue
		}
		n.Nodo.Codice, n.Nodo.Decisione = ancoraggio.CatenaCodice{}, nil
		rif = n.Nodo.Rif
		break
	}
	if rif == "" {
		t.Fatal("e3 senza un nodo figlio nella BOM di lavoro")
	}
	r := maRRendi(s, "e3 con un nodo senza codice", c, motorea_web.VistaDistinta, p.Rif, true)
	if r.Frammento.Code != http.StatusOK {
		t.Fatalf("resa: %d", r.Frammento.Code)
	}
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	senza := func(n *html.Node) []*html.Node {
		return maRTutti(n, func(x *html.Node) bool { return maRClasse(x, "ma-senza-codice") && maRTesto(x) == "(senza codice)" })
	}
	casella := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "data-ma-chiave") == "bom|"+rif })
	if len(casella) != 1 {
		t.Fatalf("%d caselle bom|%s, attesa una", len(casella), rif)
	}
	if !strings.HasPrefix(maRA(casella[0], "aria-label"), "(senza codice)") {
		t.Errorf("aria-label della casella: %q", maRA(casella[0], "aria-label"))
	}
	if cod := maRTutti(casella[0], func(n *html.Node) bool { return maRClasse(n, "ma-nodo-cod") }); len(cod) == 0 || len(senza(cod[0])) != 1 ||
		strings.Contains(maRTesto(cod[0]), "—") {
		t.Error("la catena breve della casella non scrive «(senza codice)», o scrive «—»")
	}
	pannello := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "data-ma-pannello") == "bom|"+rif })
	if len(pannello) != 1 {
		t.Fatalf("%d anteprime bom|%s, attesa una", len(pannello), rif)
	}
	if h := maRTutti(pannello[0], maRCon("h2", "")); len(h) == 0 || maRTesto(h[0]) != "(senza codice)" {
		t.Error("il titolo dell'anteprima non è «(senza codice)»")
	}
	if len(senza(pannello[0])) == 0 {
		t.Error("la catena dell'anteprima non scrive «(senza codice)»")
	}
}
