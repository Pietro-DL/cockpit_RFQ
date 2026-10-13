package web

// L1 — giro 5, A1d: la pagina della Distinta nuova resa dal template vero (QFE-02; piano A1d frontend §9.1; piano A1d
// backend §9.1; contratto A1d §2, §3, §5; RAC-04, RAC-07, RAC-09, RAC-17, RAC-18, RAC-22, RAC-23, RAC-24; FE-01,
// FE-04…FE-06, FE-08…FE-13; T-A1d-FE-01…06).
//
// Ogni caso ACME di testdata/motore_a (letto con leggiCasoMotoreA, RAC-20), per ogni vista (distinta, documenti, rfq),
// ogni prodotto valutato (o nessuno) e Scrive vero e falso, si rende con Init vero (serverTest) e rendiMotoreA: la
// pagina intera e, con HX-Request, il frammento motore_a_corpo; l'indice viene dalla fotografia del caso, come in
// componiPaginaMotoreA. Le uscite si leggono con golang.org/x/net/html. Si controlla che la pagina mostri e basta:
// nessuna scrittura che parta da sola, le rotte solo dalla lista chiusa del ponte (RAC-13), nessuna scelta fatta al
// posto di chi guarda (nessuna struttura scelta, nessun radio spuntato, nessuna qta scritta), i tre difetti corretti,
// un solo workspace per distinta e documenti, tutto spento in sola lettura.
//
// A1d-P4, passo B5.1: riallineate dalla PR #18 di Stefano Valo (153117d). I tipi sono quelli del sottopackage
// (motorea_web.Pagina, NuovoIndice con le dichiarazioni nil, le costanti delle viste); il template lo compone Init (dalla
// direttiva 12; in B5.1 lo componeva templateMotoreA alla prima richiesta) e .Dati è paginaMotoreAVista; la radice del
// frammento ha data-ma-pagina; i collegamenti delle viste non hanno più hx-get (data-ma-vai); le rotte del ponte sono 27
// (ponteNumeroRotte). Le prove di B5.1 sulla pagina (montaggio, codici, scritture, guardie, posti di P4) sono in
// motore_a_pagina_test.go.
//
// Con MA_HTML=<cartella fuori dal repository> la prova scrive le pagine e casi.json per il kit jsdom (QFE-03, nella PR
// testdata/motore_a/kit/prove/prove.cjs, non portato in B5.1: Node non c'è). Senza la variabile non scrive niente.
//
// Sta a L1: nessun database (la richiesta non ha un utente nel contesto: rendiMotoreA non legge la testata), nessun
// server. Solo dati ACME inventati: codici 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn, acme.example.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/valutazione"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// ---------------------------------------------------------------- la resa dei casi

// maRServer: il server delle prove di resa, con Init vero (serverTest: tutte le pagine, motore_a.html compreso, con
// funzioniMotoreA), il log scartato e la porta accesa.
func maRServer(t *testing.T) *Server {
	t.Helper()
	s := serverTest(t)
	s.Log = slog.New(slog.DiscardHandler)
	s.MotoreA = motorea_web.Regole{Accesa: true}
	return s
}

// maRResa: una combinazione resa (caso, vista, prodotto, Scrive): la pagina (rendiMotoreA senza HX-Request), il
// frammento (con HX-Request) e il define «contenuto» da solo (RAC-22: i controlli valgono sull'uscita della pagina, non
// sul layout).
type maRResa struct {
	Caso, Vista, Prodotto string
	Scrive                bool
	C                     *casoMotoreA
	P                     *motorea_web.Pagina
	Pagina, Frammento     *httptest.ResponseRecorder
	Contenuto             []byte
	ErrContenuto          error
}

func (r *maRResa) String() string {
	return fmt.Sprintf("%s vista=%s prodotto=%q scrive=%v", r.Caso, r.Vista, r.Prodotto, r.Scrive)
}

// maRProdotti: i Rif dei prodotti valutati del caso, nell'ordine; [""] senza esito o senza prodotti.
func maRProdotti(c *casoMotoreA) []string {
	if c.Pagina.Esito == nil || len(c.Pagina.Esito.ProdottiValutati) == 0 {
		return []string{""}
	}
	var out []string
	for _, p := range c.Pagina.Esito.ProdottiValutati {
		out = append(out, p.Rif)
	}
	return out
}

// maRPagina: la pagina del caso per una combinazione. L'indice viene dalla fotografia e dall'esito del caso, come in
// componiPaginaMotoreA (piano A1d backend §5.4.1); senza esito non c'è (RAC-05).
func maRPagina(c *casoMotoreA, vis, prodotto string, scrive bool) *motorea_web.Pagina {
	p := c.Pagina
	p.Vista, p.Prodotto, p.Scrive = vis, prodotto, scrive
	if p.Esito != nil {
		foto := c.Foto
		p.Indice = motorea_web.NuovoIndice(&foto, p.Esito, p.Confronto, nil)
	}
	return &p
}

// maRRendi rende una combinazione.
func maRRendi(s *Server, nome string, c *casoMotoreA, vis, prodotto string, scrive bool) *maRResa {
	p := maRPagina(c, vis, prodotto, scrive)
	q := url.Values{"vista": {vis}}
	if prodotto != "" {
		q.Set("prodotto", prodotto)
	}
	indirizzo := "/thread/" + p.ThreadID.String() + "/motore-a?" + q.Encode()
	r := &maRResa{Caso: nome, Vista: vis, Prodotto: prodotto, Scrive: scrive, C: c, P: p}
	r.Pagina = httptest.NewRecorder()
	s.rendiMotoreA(r.Pagina, httptest.NewRequest(http.MethodGet, indirizzo, nil), p)
	req := httptest.NewRequest(http.MethodGet, indirizzo, nil)
	req.Header.Set("HX-Request", "true")
	r.Frammento = httptest.NewRecorder()
	s.rendiMotoreA(r.Frammento, req, p)
	var b bytes.Buffer
	if tt, err := s.templateMotoreA(); err != nil {
		r.ErrContenuto = err
	} else {
		r.ErrContenuto = tt.ExecuteTemplate(&b, "contenuto", vista{Titolo: "Distinta", Dati: paginaMotoreAVista{Pagina: p}})
	}
	r.Contenuto = b.Bytes()
	return r
}

// maRViste: le tre viste della pagina (contratto §2.1).
var maRViste = []string{motorea_web.VistaDistinta, motorea_web.VistaDocumenti, motorea_web.VistaRFQ}

// maROgniResa rende ogni caso × vista × prodotto × Scrive e chiama f; k è la posizione della combinazione dentro la
// coppia (caso, vista), per i nomi dei file di MA_HTML.
func maROgniResa(t *testing.T, s *Server, f func(r *maRResa, k int)) {
	t.Helper()
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, vis := range maRViste {
			k := 0
			for _, prodotto := range maRProdotti(c) {
				for _, scrive := range []bool{true, false} {
					f(maRRendi(s, nome, c, vis, prodotto, scrive), k)
					k++
				}
			}
		}
	}
}

// maRCaso: una sola combinazione di un caso (le prove mirate).
func maRCaso(t *testing.T, s *Server, nome, vis, prodotto string, scrive bool) *maRResa {
	t.Helper()
	c := leggiCasoMotoreA(t, nome)
	if prodotto == "" {
		prodotto = maRProdotti(c)[0]
	}
	r := maRRendi(s, nome, c, vis, prodotto, scrive)
	if r.Pagina.Code != http.StatusOK || r.Frammento.Code != http.StatusOK {
		t.Fatalf("%s: pagina %d, frammento %d: %.200s", r, r.Pagina.Code, r.Frammento.Code, r.Pagina.Body.String())
	}
	return r
}

// ---------------------------------------------------------------- lettura dell'HTML

func maRDoc(t testing.TB, b []byte) *html.Node {
	t.Helper()
	n, err := html.Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("l'HTML non si legge: %v", err)
	}
	return n
}

// maRRadici: gli elementi di primo livello di un frammento (spazi e commenti esclusi).
func maRRadici(t testing.TB, b []byte) []*html.Node {
	t.Helper()
	nn, err := html.ParseFragment(bytes.NewReader(b), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		t.Fatalf("il frammento non si legge: %v", err)
	}
	var out []*html.Node
	for _, n := range nn {
		switch {
		case n.Type == html.ElementNode:
			out = append(out, n)
		case n.Type == html.TextNode && strings.TrimSpace(n.Data) == "", n.Type == html.CommentNode:
		default:
			out = append(out, n)
		}
	}
	return out
}

// maRTutti: gli elementi sotto n (n compreso) per cui ok dice sì, nell'ordine del documento.
func maRTutti(n *html.Node, ok func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	var giro func(*html.Node)
	giro = func(x *html.Node) {
		if x.Type == html.ElementNode && ok(x) {
			out = append(out, x)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			giro(c)
		}
	}
	giro(n)
	return out
}

func maRHa(n *html.Node, k string) bool {
	for _, a := range n.Attr {
		if a.Key == k {
			return true
		}
	}
	return false
}

func maRA(n *html.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

func maRClasse(n *html.Node, c string) bool {
	for _, x := range strings.Fields(maRA(n, "class")) {
		if x == c {
			return true
		}
	}
	return false
}

// maRCon: un predicato «elemento tag con l'attributo k» ("" = qualunque tag).
func maRCon(tag, k string) func(*html.Node) bool {
	return func(n *html.Node) bool { return (tag == "" || n.Data == tag) && (k == "" || maRHa(n, k)) }
}

func maRID(root *html.Node, id string) []*html.Node {
	return maRTutti(root, func(n *html.Node) bool { return maRA(n, "id") == id })
}

// maRTesto: il testo di n, con gli spazi ridotti a uno.
func maRTesto(n *html.Node) string {
	var b strings.Builder
	var giro func(*html.Node)
	giro = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			b.WriteByte(' ')
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			giro(c)
		}
	}
	giro(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// maRAntenato: il primo antenato di n per cui ok dice sì (n escluso); nil se non c'è.
func maRAntenato(n *html.Node, ok func(*html.Node) bool) *html.Node {
	for x := n.Parent; x != nil; x = x.Parent {
		if x.Type == html.ElementNode && ok(x) {
			return x
		}
	}
	return nil
}

func maRDentro(n, contenitore *html.Node) bool {
	for x := n; x != nil; x = x.Parent {
		if x == contenitore {
			return true
		}
	}
	return false
}

// maRSpento: un controllo è spento se ha disabled o sta in un fieldset disabled.
func maRSpento(n *html.Node) bool {
	return maRHa(n, "disabled") || maRAntenato(n, func(x *html.Node) bool { return x.Data == "fieldset" && maRHa(x, "disabled") }) != nil
}

// maRControlli: i controlli di un form (button, input, select, textarea), o il nodo stesso se è uno di loro.
func maRControlli(n *html.Node) []*html.Node {
	return maRTutti(n, func(x *html.Node) bool {
		return x.Data == "button" || (x.Data == "input" && maRA(x, "type") != "hidden") || x.Data == "select" || x.Data == "textarea"
	})
}

// ---------------------------------------------------------------- le rotte del ponte (RAC-13)

var maRUUID = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`

// maRRotte: le rotte della lista chiusa del ponte (fra // ROTTE:inizio e // ROTTE:fine, la sola fonte, RAC-13) come
// espressioni ancorate, con {id} = la RFQ della pagina e {u} = un UUID scritto da uuid.UUID.String().
func maRRotte(t *testing.T, thread uuid.UUID) []*regexp.Regexp {
	t.Helper()
	var out []*regexp.Regexp
	for _, r := range ponteElenco(t, "ROTTE") {
		var b strings.Builder
		b.WriteString("^")
		for i, pezzo := range strings.Split(r, "{u}") {
			if i > 0 {
				b.WriteString(maRUUID)
			}
			b.WriteString(strings.ReplaceAll(regexp.QuoteMeta(pezzo), regexp.QuoteMeta("{id}"), regexp.QuoteMeta(thread.String())))
		}
		b.WriteString("$")
		out = append(out, regexp.MustCompile(b.String()))
	}
	if len(out) != ponteNumeroRotte {
		t.Fatalf("il ponte ha %d rotte, attese %d (mappa dei gesti A1d-P3, §2.1)", len(out), ponteNumeroRotte)
	}
	return out
}

func maRNellaLista(rotte []*regexp.Regexp, r string) bool {
	for _, re := range rotte {
		if re.MatchString(r) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- i controlli generici (QFE-02)

var (
	// RAC-22: nell'uscita della pagina (contenuto e frammento), mai nel layout
	maRVietati = []string{"hx-post", "hx-trigger", "hx-get", "hx-put", "hx-patch", "hx-delete", "hx-swap-oob", "hx-on", "http://", "https://", "<iframe", "Prossimo gesto", "ma-steps",
		"ma-navsteps", "ma-dviste", "ma-prossimo", "ma-conta-tipi", "analisi" + "_messaggio"}
	maRImportmap = regexp.MustCompile(`(?s)<script type="importmap">.*?</script>`)
	maRStatico   = regexp.MustCompile(`/static/[^"'\s<>)]*`)
	// un valore che etichetta non conosce torna fra quadre (T-A1d-29): mai nel testo della pagina
	maREtichettaIgnota = regexp.MustCompile(`\[[a-z0-9_.:-]*\]`)
	// i campi che il ponte manda o che il JS legge come ID (contratto §1.9): mai un Rif
	maRAttrID = []string{"data-ma-campi", "data-ma-gesto", "data-ma-sposta", "data-ma-drop", "data-ma-da",
		"data-ma-componente", "data-ma-documento", "data-ma-nodo"}
)

// maRControlliGenerici: i controlli di §9.1 del piano A1d frontend su una combinazione.
func maRControlliGenerici(t *testing.T, r *maRResa, rotte []*regexp.Regexp) {
	t.Helper()
	for nome, rec := range map[string]*httptest.ResponseRecorder{"pagina": r.Pagina, "frammento": r.Frammento} {
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
			rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s, %s: %d %q %q: %.300s", r, nome, rec.Code, rec.Header().Get("Content-Type"),
				rec.Header().Get("Cache-Control"), rec.Body.String())
			return
		}
	}
	if r.ErrContenuto != nil {
		t.Errorf("%s: contenuto: %v", r, r.ErrContenuto)
		return
	}
	id := r.P.ThreadID.String()

	// il frammento: un solo elemento di primo livello, la .ma con i suoi tre attributi (RAC-23); niente layout
	fr := r.Frammento.Body.Bytes()
	if bytes.Contains(bytes.ToLower(fr), []byte("<html")) || bytes.Contains(fr, []byte(`id="ma-avviso"`)) {
		t.Errorf("%s: il frammento ha il layout o #ma-avviso", r)
	}
	radici := maRRadici(t, fr)
	if len(radici) != 1 || radici[0].Data != "div" || !maRClasse(radici[0], "ma") {
		t.Errorf("%s: il frammento ha %d elementi di primo livello, atteso uno solo, la div.ma (RAC-23)", r, len(radici))
	} else if g := radici[0]; maRA(g, "data-ma-pagina") != id || maRA(g, "data-ma-prodotto") != r.Prodotto ||
		maRA(g, "data-ma-vista") != r.Vista || !maRHa(g, "data-ma-prodotto") {
		t.Errorf("%s: .ma con data-ma-pagina %q, prodotto %q, vista %q", r, maRA(g, "data-ma-pagina"), maRA(g, "data-ma-prodotto"),
			maRA(g, "data-ma-vista"))
	}

	// la pagina: un solo #ma-avviso, fuori da #ma; #ma con una sola .ma; pdf.js dai dati; id unici
	doc := maRDoc(t, r.Pagina.Body.Bytes())
	avvisi, mm := maRID(doc, "ma-avviso"), maRID(doc, "ma")
	if len(avvisi) != 1 || len(mm) != 1 {
		t.Errorf("%s: %d #ma-avviso e %d #ma, attesi uno e uno", r, len(avvisi), len(mm))
		return
	}
	ma := mm[0]
	if maRDentro(avvisi[0], ma) {
		t.Errorf("%s: #ma-avviso dentro #ma: la rilettura lo cancellerebbe", r)
	}
	if maRA(ma, "data-ma-pdfjs") == "" || maRA(ma, "data-ma-pdfjs-worker") == "" {
		t.Errorf("%s: #ma senza data-ma-pdfjs o data-ma-pdfjs-worker", r)
	}
	figli := maRTutti(ma, func(n *html.Node) bool { return n.Parent == ma })
	if len(figli) != 1 || !maRClasse(figli[0], "ma") {
		t.Errorf("%s: #ma ha %d figli, atteso uno solo, la .ma (RAC-23)", r, len(figli))
	}
	visti := map[string]bool{}
	for _, n := range maRTutti(doc, maRCon("", "id")) {
		if visti[maRA(n, "id")] {
			t.Errorf("%s: id %q ripetuto", r, maRA(n, "id"))
		}
		visti[maRA(n, "id")] = true
	}

	// le rotte: solo la lista chiusa del ponte, con la RFQ della pagina; nessun Rif dove va un ID (contratto §1.9)
	for _, n := range maRTutti(ma, func(n *html.Node) bool { return maRHa(n, "data-ma-gesto") || maRHa(n, "data-ma-sposta") }) {
		for _, k := range []string{"data-ma-gesto", "data-ma-sposta"} {
			if v := maRA(n, k); maRHa(n, k) && !maRNellaLista(rotte, v) {
				t.Errorf("%s: %s=%q fuori dalla lista chiusa del ponte (o con un'altra RFQ)", r, k, v)
			}
		}
	}
	for _, n := range maRTutti(ma, func(*html.Node) bool { return true }) {
		var valori []string
		if n.Data == "input" || n.Data == "option" || n.Data == "button" {
			valori = append(valori, maRA(n, "value"))
		}
		for _, k := range maRAttrID {
			valori = append(valori, maRA(n, k))
		}
		for _, v := range valori {
			if strings.Contains(v, "nodo:") || strings.Contains(v, "componente:") {
				t.Errorf("%s: un Rif dove va un ID (%s %q)", r, n.Data, v)
			}
		}
	}

	// il testo: nessun valore di etichetta fra quadre
	var giro func(*html.Node)
	giro = func(x *html.Node) {
		if x.Type == html.ElementNode && (x.Data == "script" || x.Data == "style") {
			return
		}
		if x.Type == html.TextNode {
			if m := maREtichettaIgnota.FindString(x.Data); m != "" {
				t.Errorf("%s: %q nel testo: un valore senza etichetta (T-A1d-29)", r, m)
			}
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			giro(c)
		}
	}
	giro(ma)

	// RAC-22 e RAC-18 sull'uscita della pagina: nessuna scrittura che parta da sola, nessun indirizzo esterno, ogni
	// statico con l'impronta (salvo pdf.js e la sola chiave dell'importmap)
	for nome, b := range map[string][]byte{"contenuto": r.Contenuto, "frammento": fr} {
		for _, v := range maRVietati {
			if bytes.Contains(b, []byte(v)) {
				t.Errorf("%s, %s: contiene %q", r, nome, v)
			}
		}
		senzaMappa := maRImportmap.ReplaceAll(b, nil)
		for _, m := range maRStatico.FindAll(senzaMappa, -1) {
			s := string(m)
			if !strings.Contains(s, "?v=") && !strings.HasPrefix(s, "/static/pdfjs-6.3.289/") {
				t.Errorf("%s, %s: %q scritto senza {{statico}} (RAC-18)", r, nome, s)
			}
		}
	}
}

// TestMotoreAPaginaEFrammentoPerOgniCaso: i controlli generici su ogni combinazione; con MA_HTML scrive le pagine, i
// frammenti e casi.json per jsdom (QFE-02, FE-01; piano A1d frontend §9.1).
func TestMotoreAPaginaEFrammentoPerOgniCaso(t *testing.T) {
	s := maRServer(t)
	cartella := os.Getenv("MA_HTML")
	if cartella != "" {
		if err := os.MkdirAll(cartella, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	type voce struct {
		Pagina    string `json:"pagina"`
		Frammento string `json:"frammento"`
		Caso      string `json:"caso"`
		Vista     string `json:"vista"`
		Prodotto  string `json:"prodotto"`
		Scrive    bool   `json:"scrive"`
	}
	var (
		elenco    []voce
		rotte     = map[uuid.UUID][]*regexp.Regexp{}
		quante    int
		piuGrande int
		chi       string
	)
	maROgniResa(t, s, func(r *maRResa, k int) {
		quante++
		if rotte[r.P.ThreadID] == nil {
			rotte[r.P.ThreadID] = maRRotte(t, r.P.ThreadID)
		}
		maRControlliGenerici(t, r, rotte[r.P.ThreadID])
		if n := r.Pagina.Body.Len(); n > piuGrande {
			piuGrande, chi = n, r.String()
		}
		if cartella == "" {
			return
		}
		base := fmt.Sprintf("%s__%s__%d", r.Caso, r.Vista, k)
		for nome, b := range map[string][]byte{base + ".html": r.Pagina.Body.Bytes(), base + ".frammento.html": r.Frammento.Body.Bytes()} {
			if err := os.WriteFile(filepath.Join(cartella, nome), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		elenco = append(elenco, voce{base + ".html", base + ".frammento.html", r.Caso, r.Vista, r.Prodotto, r.Scrive})
	})
	if quante < len(maNomiCasi)*len(maRViste)*2 {
		t.Errorf("%d combinazioni rese, troppo poche", quante)
	}
	// RK-10: la misura della pagina più grande, nel log della prova
	t.Logf("%d combinazioni; la pagina più grande: %d byte (%s)", quante, piuGrande, chi)
	if cartella != "" {
		b, err := json.MarshalIndent(elenco, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cartella, "casi.json"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("MA_HTML: %d pagine e casi.json in %s", len(elenco), cartella)
	}
}

// ---------------------------------------------------------------- le prove mirate

// maRNormalizzaVista: l'uscita di documenti resa come quella di distinta (data-ma-vista, vista= negli indirizzi, la
// voce corrente delle viste, la classe dell'evidenza): il resto deve coincidere byte per byte (FE-13).
var (
	maRNormalizza = strings.NewReplacer(`data-ma-vista="documenti"`, `data-ma-vista="distinta"`,
		"vista=documenti", "vista=distinta", " ma-evidenza", "")
	// B5.1: i collegamenti delle viste finiscono con data-ma-vai (nella PR con hx-push-url="true", fra virgolette)
	maRCorrente = regexp.MustCompile(`(data-ma-vai|")\s*(aria-current="page")?\s*>`)
)

// maRNormalizzaVista: la normalizzazione di sopra, più la voce corrente delle viste con gli spazi che la circondano.
func maRNormalizzaVista(s string) string {
	return maRCorrente.ReplaceAllString(maRNormalizza.Replace(s), `$1>`)
}

// TestMotoreADistintaEDocumentiStessoWorkspace: distinta e documenti sono lo stesso workspace (ELENCO §4.1, FE-13): dopo
// la normalizzazione le due uscite coincidono; #ma-documenti.ma-evidenza solo con documenti; con rfq nessun .ma-ws.
func TestMotoreADistintaEDocumentiStessoWorkspace(t *testing.T) {
	s := maRServer(t)
	confrontati := 0
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			for _, scrive := range []bool{true, false} {
				d := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, scrive)
				o := maRRendi(s, nome, c, motorea_web.VistaDocumenti, prodotto, scrive)
				q := maRRendi(s, nome, c, motorea_web.VistaRFQ, prodotto, scrive)
				a, b := maRNormalizzaVista(d.Frammento.Body.String()), maRNormalizzaVista(o.Frammento.Body.String())
				if a != b {
					i := 0
					for i < len(a) && i < len(b) && a[i] == b[i] {
						i++
					}
					t.Errorf("%s: documenti diverso da distinta dal byte %d: %q contro %q", d, i, a[max(0, i-60):min(len(a), i+60)],
						b[max(0, i-60):min(len(b), i+60)])
				}
				confrontati++
				for _, r := range []*maRResa{d, o, q} {
					doc := maRDoc(t, r.Frammento.Body.Bytes())
					ev := maRTutti(doc, func(n *html.Node) bool { return maRClasse(n, "ma-evidenza") })
					ws := maRTutti(doc, func(n *html.Node) bool { return maRClasse(n, "ma-ws") })
					switch {
					case r.Vista == motorea_web.VistaDocumenti && c.Pagina.Esito != nil && prodotto != "":
						if len(ev) != 1 || maRA(ev[0], "id") != "ma-documenti" {
							t.Errorf("%s: %d elementi .ma-evidenza, atteso solo #ma-documenti", r, len(ev))
						}
					default:
						if len(ev) != 0 {
							t.Errorf("%s: .ma-evidenza fuori dalla vista documenti (MF09)", r)
						}
					}
					if r.Vista == motorea_web.VistaRFQ && len(ws) != 0 {
						t.Errorf("%s: con rfq un .ma-ws", r)
					}
					if r.Vista != motorea_web.VistaRFQ && c.Pagina.Esito != nil && prodotto != "" && len(ws) != 1 {
						t.Errorf("%s: %d .ma-ws, atteso uno (un solo workspace, FE-01)", r, len(ws))
					}
				}
			}
		}
	}
	if confrontati == 0 {
		t.Fatal("nessun confronto")
	}
}

// TestMotoreASolaLetturaTuttoSpento: con Scrive falso ogni gesto è spento, sul pulsante o con il fieldset (contratto
// §2.2); spenti anche «Autorizza», «Vedi l'effetto del cambio…» e «Conferma l'albero»; niente trascinamento. Con Scrive
// vero almeno un gesto è acceso (la prova distingue).
func TestMotoreASolaLetturaTuttoSpento(t *testing.T) {
	s := maRServer(t)
	accesi := 0
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			for _, scrive := range []bool{false, true} {
				r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, scrive)
				doc := maRDoc(t, r.Frammento.Body.Bytes())
				gesti := maRTutti(doc, func(n *html.Node) bool {
					a := maRA(n, "data-ma-azione")
					return maRHa(n, "data-ma-gesto") || a == "step" || a == "tipo" || a == "albero"
				})
				for _, g := range gesti {
					controlli := maRControlli(g)
					if len(controlli) == 0 {
						t.Errorf("%s: %s %q senza un controllo", r, g.Data, maRA(g, "data-ma-gesto"))
					}
					for _, x := range controlli {
						if !scrive && !maRSpento(x) {
							t.Errorf("%s: %s di %q acceso in sola lettura", r, x.Data, maRA(g, "data-ma-gesto")+maRA(g, "data-ma-azione"))
						}
						if scrive && !maRSpento(x) {
							accesi++
						}
					}
				}
				if !scrive {
					for _, k := range []string{"draggable", "data-ma-drop", "data-ma-sposta"} {
						if n := maRTutti(doc, maRCon("", k)); len(n) != 0 {
							t.Errorf("%s: %d elementi con %s in sola lettura", r, len(n), k)
						}
					}
				}
			}
		}
	}
	if accesi == 0 {
		t.Error("con Scrive vero nessun gesto acceso: la prova non distingue")
	}
}

// TestMotoreAQtaObbligatoria: la qta di Collega e Sposta è obbligatoria e mai scritta (difetto 1, FE-08): type=number,
// required, min=1, step=1, nessun value, nessun placeholder.
func TestMotoreAQtaObbligatoria(t *testing.T) {
	s := maRServer(t)
	viste := 0
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, true)
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			for _, f := range maRTutti(doc, maRCon("form", "data-ma-gesto")) {
				g := maRA(f, "data-ma-gesto")
				if !strings.HasSuffix(g, "/collega") && !strings.HasSuffix(g, "/sposta") {
					continue
				}
				q := maRTutti(f, func(n *html.Node) bool { return n.Data == "input" && maRA(n, "name") == "qta" })
				if len(q) != 1 {
					t.Errorf("%s: %s con %d campi qta", r, g, len(q))
					continue
				}
				viste++
				x := q[0]
				if maRA(x, "type") != "number" || !maRHa(x, "required") || maRA(x, "min") != "1" || maRA(x, "step") != "1" ||
					maRHa(x, "value") || maRHa(x, "placeholder") {
					t.Errorf("%s: qta di %s: %v (attesi type=number required min=1 step=1, senza value né placeholder)", r, g, x.Attr)
				}
			}
		}
	}
	if viste == 0 {
		t.Fatal("nessun Collega o Sposta nei casi: la prova non prova niente")
	}
}

// TestMotoreAChiaviDiGruppo: chiavi «<gruppo>|<rif>» (difetto 2, FE-04): data-ma-pannello unici; ogni casella e ogni
// pannello cominciano con il data-ma-gruppo del loro gruppo; data-ma-cammino unici; in e18 il #30 delle due candidate
// ha due chiavi diverse.
func TestMotoreAChiaviDiGruppo(t *testing.T) {
	s := maRServer(t)
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, true)
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			unici := func(k string) {
				visti := map[string]bool{}
				for _, n := range maRTutti(doc, maRCon("", k)) {
					if visti[maRA(n, k)] {
						t.Errorf("%s: %s %q ripetuto", r, k, maRA(n, k))
					}
					visti[maRA(n, k)] = true
				}
			}
			unici("data-ma-pannello")
			unici("data-ma-cammino")
			for _, li := range maRTutti(doc, maRCon("li", "data-ma-chiave")) {
				tp := maRAntenato(li, func(x *html.Node) bool { return maRA(x, "role") == "tabpanel" })
				if tp == nil || !strings.HasPrefix(maRA(li, "data-ma-chiave"), maRA(tp, "data-ma-gruppo")+"|") ||
					maRA(li, "data-ma-chiave") != maRA(tp, "data-ma-gruppo")+"|"+maRA(li, "data-ma-rif") {
					t.Errorf("%s: la casella %q non ha la chiave del suo gruppo", r, maRA(li, "data-ma-chiave"))
				}
			}
			for _, p := range maRTutti(doc, maRCon("", "data-ma-pannello")) {
				if !strings.HasPrefix(maRA(p, "data-ma-pannello"), maRA(p, "data-ma-gruppo")+"|") {
					t.Errorf("%s: il pannello %q senza il suo gruppo", r, maRA(p, "data-ma-pannello"))
				}
			}
			if nome != "e18_stesso_rif_due_candidate" {
				continue
			}
			rif30 := maRN("30", "#30")
			chiavi := map[string]bool{}
			for _, li := range maRTutti(doc, func(n *html.Node) bool { return n.Data == "li" && maRA(n, "data-ma-rif") == rif30 }) {
				if strings.HasPrefix(maRA(li, "data-ma-chiave"), "cand:") {
					chiavi[maRA(li, "data-ma-chiave")] = true
				}
			}
			pannelli := 0
			for k := range chiavi {
				pannelli += len(maRTutti(doc, func(n *html.Node) bool { return maRA(n, "data-ma-pannello") == k }))
			}
			if len(chiavi) != 2 || pannelli != 2 {
				t.Errorf("%s: #30 con %d chiavi diverse nelle candidate e %d pannelli, attesi 2 e 2 (D6)", r, len(chiavi), pannelli)
			}
		}
	}
}

// TestMotoreASchedeSenzaScelta: una tablist per prodotto, una scheda per gruppo di Indice.Gruppi con aria-controls
// validi, una sola scheda aria-selected e un solo pannello visibile; nessun input spuntato né aria-checked in R3 e R5,
// nessuna casella scelta; «proposta dal file, non è la BOM» in ogni pannello cand; «nessuna è scelta» in R3 per e1, e2
// ed e18 (FE-04; D-PF-05, D-PF-06).
func TestMotoreASchedeSenzaScelta(t *testing.T) {
	s := maRServer(t)
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		if c.Pagina.Esito == nil {
			continue
		}
		for _, prodotto := range maRProdotti(c) {
			if prodotto == "" {
				continue
			}
			r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, true)
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			gruppi := r.P.Indice.Gruppi(prodotto)
			liste := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "role") == "tablist" })
			if len(gruppi) == 0 {
				if len(liste) != 0 {
					t.Errorf("%s: una tablist senza gruppi", r)
				}
				continue
			}
			if len(liste) != 1 {
				t.Errorf("%s: %d tablist, attesa una", r, len(liste))
				continue
			}
			schede := maRTutti(liste[0], func(n *html.Node) bool { return maRA(n, "role") == "tab" })
			pannelli := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "role") == "tabpanel" })
			if len(schede) != len(gruppi) || len(pannelli) != len(gruppi) {
				t.Errorf("%s: %d schede e %d pannelli per %d gruppi", r, len(schede), len(pannelli), len(gruppi))
				continue
			}
			scelte, visibili := 0, 0
			for i, sc := range schede {
				if maRA(sc, "data-ma-gruppo") != gruppi[i].Chiave {
					t.Errorf("%s: scheda %d del gruppo %q, attesa %q (l'ordine del server)", r, i, maRA(sc, "data-ma-gruppo"), gruppi[i].Chiave)
				}
				if p := maRID(doc, maRA(sc, "aria-controls")); len(p) != 1 || maRA(p[0], "role") != "tabpanel" {
					t.Errorf("%s: aria-controls %q non porta a un tabpanel", r, maRA(sc, "aria-controls"))
				}
				if maRA(sc, "aria-selected") == "true" {
					scelte++
				}
			}
			for _, p := range pannelli {
				if !maRHa(p, "hidden") {
					visibili++
				}
				if strings.Contains(maRA(p, "class"), "ma-pannello-albero--cand") &&
					!strings.Contains(strings.ToLower(maRTesto(p)), "proposta dal file, non è la bom") {
					t.Errorf("%s: un pannello cand senza «proposta dal file, non è la BOM»", r)
				}
			}
			if scelte != 1 || visibili != 1 {
				t.Errorf("%s: %d schede scelte e %d pannelli visibili, attesi 1 e 1", r, scelte, visibili)
			}
			for _, id := range []string{"ma-fonte", "ma-schede-albero"} {
				for _, reg := range maRID(doc, id) {
					if n := maRTutti(reg, func(n *html.Node) bool { return maRHa(n, "checked") || maRHa(n, "aria-checked") }); len(n) != 0 {
						t.Errorf("%s: %d elementi spuntati in #%s", r, len(n), id)
					}
				}
			}
			if n := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "role") == "treeitem" && maRA(n, "aria-selected") == "true" }); len(n) != 0 {
				t.Errorf("%s: %d caselle scelte all'apertura (D-PF-06)", r, len(n))
			}
			switch nome {
			case "e1_albero_candidato", "e2_due_strutture", "e18_stesso_rif_due_candidate":
				if f := maRID(doc, "ma-fonte"); len(f) != 1 || !strings.Contains(maRTesto(f[0]), "nessuna è scelta") {
					t.Errorf("%s: R3 senza «nessuna è scelta»", r)
				}
			}
		}
	}
}

// TestMotoreALegamiDelFileEConfermati: «nel file» e «confermato» accanto, mai fusi (FE-05, ELENCO §4.5, D5 di P7e). In
// e14 il #30 sotto #20 ha il tratteggio e il badge «confermato sotto 7120100 ✓×3»; la Posizione ha i due elenchi; il «Da»
// di Sposta ha la sola opzione u10; «Scollega» solo da u10. In e3 #10 → #40 ha la linea piena. RAC-09 su e3 ed e14:
// Figli[].Decisa = Legami.Confermati[].NelFile per ogni arco (D-PF-02).
func TestMotoreALegamiDelFileEConfermati(t *testing.T) {
	s := maRServer(t)
	u10 := maU("10").String()

	r := maRCaso(t, s, "e14_nodo_spostato", motorea_web.VistaDistinta, "", true)
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	rif20, rif30 := maRN("30", "#20"), maRN("30", "#30")
	var li30 *html.Node
	for _, li := range maRTutti(doc, func(n *html.Node) bool { return n.Data == "li" && maRA(n, "data-ma-rif") == rif30 }) {
		if p := maRAntenato(li, func(x *html.Node) bool { return x.Data == "li" }); p != nil && maRA(p, "data-ma-rif") == rif20 {
			li30 = li
		}
	}
	switch {
	case li30 == nil:
		t.Error("e14: #30 non sta sotto #20")
	case !maRClasse(li30, "ma-li--file"):
		t.Errorf("e14: #30 sotto #20 con la classe %q, atteso il tratteggio ma-li--file", maRA(li30, "class"))
	case !strings.Contains(maRTesto(li30), "confermato sotto 7120100 ✓×3"):
		t.Errorf("e14: #30 senza il badge «confermato sotto 7120100 ✓×3»: %q", maRTesto(li30))
	}
	pann := maRTutti(doc, func(n *html.Node) bool { return strings.HasSuffix(maRA(n, "data-ma-pannello"), "|"+rif30) })
	if len(pann) != 1 {
		t.Fatalf("e14: %d pannelli di #30", len(pann))
	}
	pos := maRTesto(pann[0])
	for _, atteso := range []string{"Nel file", "×2 dal file", "da confermare", "Relazioni confermate correnti", "✓ 7120100", "✓×3", "solo confermata"} {
		if !strings.Contains(pos, atteso) {
			t.Errorf("e14: la Posizione di #30 non ha %q", atteso)
		}
	}
	var da []string
	scollega := 0
	for _, f := range maRTutti(pann[0], maRCon("form", "data-ma-gesto")) {
		switch g := maRA(f, "data-ma-gesto"); {
		case strings.HasSuffix(g, "/sposta"):
			for _, o := range maRTutti(f, func(n *html.Node) bool {
				sel := maRAntenato(n, maRCon("select", ""))
				return n.Data == "option" && sel != nil && maRA(sel, "name") == "da"
			}) {
				if !maRHa(o, "disabled") {
					da = append(da, maRA(o, "value"))
				}
				if maRHa(o, "selected") && !maRHa(o, "disabled") {
					t.Errorf("e14: un «Da» preselezionato: %q", maRA(o, "value"))
				}
			}
		case strings.HasSuffix(g, "/scollega"):
			scollega++
			if p := maRTutti(f, func(n *html.Node) bool { return maRA(n, "name") == "padre" }); len(p) != 1 || maRA(p[0], "value") != u10 {
				t.Errorf("e14: Scollega da un padre che non è u10")
			}
		}
	}
	if !reflect.DeepEqual(da, []string{u10}) || scollega != 1 {
		t.Errorf("e14: «Da» di Sposta %v e %d Scollega; attesi solo %s e uno (FE-08)", da, scollega, u10)
	}
	if dd := maRTutti(doc, maRCon("", "data-ma-da")); len(dd) == 0 {
		t.Error("e14: nessuna casella trascinabile")
	} else {
		for _, n := range dd {
			if maRA(n, "data-ma-sposta") != "" && strings.Contains(maRA(n, "data-ma-sposta"), maU("12").String()) && maRA(n, "data-ma-da") != u10 {
				t.Errorf("e14: data-ma-da di 7120120 = %q, atteso %s", maRA(n, "data-ma-da"), u10)
			}
		}
	}

	r = maRCaso(t, s, "e3_bom_di_lavoro", motorea_web.VistaDistinta, "", true)
	doc = maRDoc(t, r.Frammento.Body.Bytes())
	rif10, rif40 := maRN("30", "#10"), maRN("30", "#40")
	conf := false
	for _, li := range maRTutti(doc, func(n *html.Node) bool { return n.Data == "li" && maRA(n, "data-ma-rif") == rif40 }) {
		if p := maRAntenato(li, func(x *html.Node) bool { return x.Data == "li" }); p != nil && maRA(p, "data-ma-rif") == rif10 &&
			strings.HasPrefix(maRA(li, "data-ma-chiave"), "bom|") {
			conf = maRClasse(li, "ma-li--conf")
		}
	}
	if !conf {
		t.Error("e3: #10 → #40 senza la linea piena ma-li--conf")
	}

	// RAC-09 (D-PF-02): sulle fixture, ogni arco «deciso» dell'albero è «nel file» fra i confermati del figlio
	for _, nome := range []string{"e3_bom_di_lavoro", "e14_nodo_spostato"} {
		c := leggiCasoMotoreA(t, nome)
		p := maRPagina(c, motorea_web.VistaDistinta, "", true)
		archi := 0
		for _, prodotto := range maRProdotti(c) {
			for _, g := range p.Indice.Gruppi(prodotto) {
				if g.Tipo == "cand" {
					continue
				}
				for _, padre := range g.Nodi {
					for _, f := range p.Indice.Figli(prodotto, g.Chiave, padre.Nodo.Rif) {
						nelFile := false
						for _, lc := range p.Indice.Legami(prodotto, g.Chiave, f.Nodo.Nodo.Rif).Confermati {
							if lc.PadreRif == padre.Nodo.Rif && lc.NelFile {
								nelFile = true
							}
						}
						archi++
						if f.Decisa && !nelFile && !f.Nodo.Nodo.SenzaFile {
							t.Errorf("%s: %s → %s deciso, ma fra i confermati del figlio non è «nel file» (RAC-09)", nome, padre.Nodo.Rif, f.Nodo.Nodo.Rif)
						}
						if !f.Decisa && nelFile {
							t.Errorf("%s: %s → %s non deciso, ma «nel file» fra i confermati (RAC-09)", nome, padre.Nodo.Rif, f.Nodo.Nodo.Rif)
						}
					}
				}
			}
		}
		if archi == 0 {
			t.Errorf("%s: nessun arco", nome)
		}
	}
}

// maRComponentiDecisi: i componenti decisi di un prodotto (il suo componente, poi le Decisioni dei nodi e dei
// confermati senza STEP), come insieme di UUID scritti: l'oracolo di «Associa a…».
func maRComponentiDecisi(p *valutazione.ProdottoValutato) map[string]bool {
	out := map[string]bool{}
	if p.ComponenteID != nil {
		out[p.ComponenteID.String()] = true
	}
	for _, nn := range [][]valutazione.NodoBOM{p.Nodi, p.ConfermatiSenzaSTEP} {
		for _, n := range nn {
			if n.Nodo.Decisione != nil {
				out[n.Nodo.Decisione.ComponenteID.String()] = true
			}
		}
	}
	return out
}

// TestMotoreAAssociaConIProdottiDelFile: «Associa a…» di R8 offre i componenti decisi dei prodotti DEL FILE, poi quelli
// del messaggio; senza prodotti quelli del prodotto scelto, con l'avviso (difetto 3, FE-10, T-A1d-FE-06; e17). Nessuna
// opzione selezionata salvo il segnaposto spento.
func TestMotoreAAssociaConIProdottiDelFile(t *testing.T) {
	s := maRServer(t)
	r := maRCaso(t, s, "e17_da_smistare_contesto", motorea_web.VistaDistinta, "", true)
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	e := r.P.Esito
	prodotti := map[string]*valutazione.ProdottoValutato{}
	for i := range e.ProdottiValutati {
		prodotti[e.ProdottiValutati[i].Rif] = &e.ProdottiValutati[i]
	}
	visti, senza := 0, 0
	for i, f := range e.DaSmistare {
		if r.P.Indice.PropostaAperta(f.AllegatoID) == nil {
			continue
		}
		dlg := maRID(doc, fmt.Sprintf("ma-ds-%d", i))
		if len(dlg) != 1 {
			t.Errorf("%s: nessun cassetto «Associa a…»", f.NomeFile)
			continue
		}
		sel := maRTutti(dlg[0], func(n *html.Node) bool { return n.Data == "select" && maRA(n, "name") == "componente" })
		if len(sel) != 1 {
			t.Errorf("%s: %d select del componente", f.NomeFile, len(sel))
			continue
		}
		visti++
		type gruppo struct {
			etichetta string
			opzioni   map[string]bool
		}
		var atteso []gruppo
		for _, rif := range f.Prodotti {
			atteso = append(atteso, gruppo{"del file · " + prodotti[rif].CodiceRichiesto, maRComponentiDecisi(prodotti[rif])})
		}
		for _, rif := range f.ProdottiContesto {
			atteso = append(atteso, gruppo{"dal messaggio · " + prodotti[rif].CodiceRichiesto, maRComponentiDecisi(prodotti[rif])})
		}
		if len(atteso) == 0 {
			senza++
			atteso = append(atteso, gruppo{"prodotto scelto (il file non ha prodotti) · " + prodotti[r.Prodotto].CodiceRichiesto,
				maRComponentiDecisi(prodotti[r.Prodotto])})
			if !strings.Contains(maRTesto(dlg[0]), "Il file non ha prodotti") {
				t.Errorf("%s: senza prodotti e senza l'avviso", f.NomeFile)
			}
		}
		var reso []gruppo
		for _, og := range maRTutti(sel[0], maRCon("optgroup", "")) {
			g := gruppo{maRA(og, "label"), map[string]bool{}}
			for _, o := range maRTutti(og, maRCon("option", "")) {
				g.opzioni[maRA(o, "value")] = true
			}
			reso = append(reso, g)
		}
		if !reflect.DeepEqual(reso, atteso) {
			t.Errorf("%s: optgroup %+v, attesi %+v", f.NomeFile, reso, atteso)
		}
		for _, o := range maRTutti(sel[0], maRCon("option", "")) {
			if maRHa(o, "selected") && !(maRHa(o, "disabled") && maRA(o, "value") == "") {
				t.Errorf("%s: opzione %q selezionata", f.NomeFile, maRA(o, "value"))
			}
		}
	}
	if visti != 3 || senza != 1 {
		t.Errorf("e17: %d cassetti di R8 provati (attesi 3), %d senza prodotti (atteso 1)", visti, senza)
	}
}

// TestMotoreASceltaSoloConDocumento: «scelta» solo nel form con documento (FE-11, U-08; e15): valori aggiungi o
// sostituisci:<uuid> fra i correnti del componente del fieldset (Indice.Correnti), mai spuntati; fieldset spenti e
// nascosti (li accende motore_a.mjs).
func TestMotoreASceltaSoloConDocumento(t *testing.T) {
	s := maRServer(t)
	visti := 0
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			r := maRRendi(s, nome, c, motorea_web.VistaDocumenti, prodotto, true)
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			for _, x := range maRTutti(doc, func(n *html.Node) bool { return n.Data == "input" && maRA(n, "name") == "scelta" }) {
				form := maRAntenato(x, maRCon("form", ""))
				fs := maRAntenato(x, maRCon("fieldset", "data-ma-scelta-per"))
				dlg := maRAntenato(x, maRCon("dialog", ""))
				if form == nil || len(maRTutti(form, func(n *html.Node) bool { return maRA(n, "name") == "documento" })) != 1 ||
					len(maRTutti(form, func(n *html.Node) bool { return maRA(n, "name") == "proposta" })) != 0 {
					t.Errorf("%s: «scelta» fuori da un form con documento", r)
					continue
				}
				if fs == nil || !maRHa(fs, "disabled") || !maRHa(fs, "hidden") {
					t.Errorf("%s: «scelta» fuori da un fieldset data-ma-scelta-per spento e nascosto", r)
					continue
				}
				if maRHa(x, "checked") {
					t.Errorf("%s: «scelta» spuntata", r)
				}
				var i int
				if _, err := fmt.Sscanf(maRA(dlg, "id"), "ma-isp-%d", &i); err != nil {
					t.Errorf("%s: «scelta» fuori da un cassetto di una voce", r)
					continue
				}
				p := r.P.Indice.Prodotto(prodotto)
				vo := p.Documenti.Voci[i]
				comp, err := uuid.Parse(maRA(fs, "data-ma-scelta-per"))
				if err != nil {
					t.Errorf("%s: data-ma-scelta-per %q", r, maRA(fs, "data-ma-scelta-per"))
					continue
				}
				ammessi := map[string]bool{"aggiungi": true}
				for _, cc := range r.P.Indice.Correnti(comp, string(vo.TipoDocumento)) {
					ammessi["sostituisci:"+cc.DocumentoID.String()] = true
				}
				if !ammessi[maRA(x, "value")] {
					t.Errorf("%s: «scelta» = %q fuori dai correnti di %s", r, maRA(x, "value"), comp)
				}
				visti++
			}
		}
	}
	if visti == 0 {
		t.Fatal("nessuna «scelta» nei casi: la prova non prova niente (e15)")
	}
	r := maRCaso(t, s, "e15_corrente_stesso_tipo", motorea_web.VistaDocumenti, "", true)
	if !strings.Contains(r.Frammento.Body.String(), `value="sostituisci:`+maU("55").String()+`"`) {
		t.Error("e15: nessun «sostituisci» del corrente 55")
	}
}

// TestMotoreARevEDescrizioneEDRipristina: «Salva rev e descrizione» solo con la descrizione registrata (ELENCO §4.6,
// FE-08; e16): acceso e precompilato per u11; per u12 spento con il motivo; «Ripristina» solo per u15, senza «Archivia».
func TestMotoreARevEDescrizioneEDRipristina(t *testing.T) {
	s := maRServer(t)
	r := maRCaso(t, s, "e16_descrizione_registrata", motorea_web.VistaDistinta, "", true)
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	modifica := maRTutti(doc, func(n *html.Node) bool { return strings.HasSuffix(maRA(n, "data-ma-gesto"), "/modifica") })
	if len(modifica) == 0 {
		t.Fatal("e16: nessun «Salva rev e descrizione»")
	}
	for _, f := range modifica {
		if !strings.Contains(maRA(f, "data-ma-gesto"), "/componente/"+maU("11").String()+"/") {
			t.Errorf("e16: «Salva rev e descrizione» per %q, atteso solo per u11", maRA(f, "data-ma-gesto"))
			continue
		}
		val := map[string]string{}
		for _, x := range maRTutti(f, maRCon("input", "name")) {
			val[maRA(x, "name")] = maRA(x, "value")
			if maRSpento(x) {
				t.Error("e16: «Salva rev e descrizione» spento per u11")
			}
		}
		if val["rev"] != "B" || val["descrizione"] != "Staffa di prova ACME" {
			t.Errorf("e16: valori %v, attesi rev B e «Staffa di prova ACME»", val)
		}
	}
	// il pannello di u12: lo trova il suo Collega
	trovato := false
	for _, p := range maRTutti(doc, maRCon("section", "data-ma-pannello")) {
		if len(maRTutti(p, func(n *html.Node) bool {
			return maRA(n, "data-ma-gesto") == "/thread/"+r.P.ThreadID.String()+"/fascicolo/componente/"+maU("12").String()+"/collega"
		})) == 0 {
			continue
		}
		trovato = true
		na := false
		for _, x := range maRTutti(p, func(n *html.Node) bool { return maRClasse(n, "ma-na") }) {
			if strings.Contains(maRTesto(x), "Salva rev e descrizione") {
				na = true
			}
		}
		if !na {
			t.Error("e16: per u12 «Salva rev e descrizione» non è spento con il motivo")
		}
	}
	if !trovato {
		t.Error("e16: nessun pannello di u12")
	}
	rip := maRTutti(doc, func(n *html.Node) bool { return strings.HasSuffix(maRA(n, "data-ma-gesto"), "/ripristina") })
	if len(rip) == 0 {
		t.Error("e16: nessun «Ripristina»")
	}
	for _, x := range rip {
		if !strings.Contains(maRA(x, "data-ma-gesto"), "/componente/"+maU("15").String()+"/") {
			t.Errorf("e16: «Ripristina» per %q, atteso solo per u15", maRA(x, "data-ma-gesto"))
		}
	}
	if n := maRTutti(doc, func(n *html.Node) bool {
		return strings.HasSuffix(maRA(n, "data-ma-gesto"), "/componente/"+maU("15").String()+"/archivia")
	}); len(n) != 0 {
		t.Error("e16: «Archivia» per u15, che è archiviato")
	}
}

// maRFraseDB72: la frase DB7-2 della conferma con la struttura non verificata (piano A1d frontend §5.1 C).
const maRFraseDB72 = "La copia sul NAS parte subito; la struttura non è ancora verificata."

// TestMotoreAConfermaUnaSolaVoce: ogni «Conferma il documento» manda UNA voce (FE-12): data-ma-campi = {"voce": <uuid>} e
// niente altro; la frase DB7-2 c'è se e solo se la BOM del prodotto scelto non è verificata (D-PF-03).
func TestMotoreAConfermaUnaSolaVoce(t *testing.T) {
	s := maRServer(t)
	visti := map[bool]int{}
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, true)
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			for _, x := range maRTutti(doc, func(n *html.Node) bool { return strings.HasSuffix(maRA(n, "data-ma-gesto"), "/fascicolo/conferma") }) {
				var campi map[string]string
				if err := json.Unmarshal([]byte(maRA(x, "data-ma-campi")), &campi); err != nil || len(campi) != 1 {
					t.Errorf("%s: data-ma-campi %q, atteso {\"voce\": <uuid>}", r, maRA(x, "data-ma-campi"))
					continue
				}
				if _, err := uuid.Parse(campi["voce"]); err != nil {
					t.Errorf("%s: voce %q non è un UUID", r, campi["voce"])
				}
				verificata := r.P.Indice.Prodotto(prodotto).BOM.Verificata
				if strings.Contains(maRA(x, "data-ma-domanda"), maRFraseDB72) == verificata {
					t.Errorf("%s: frase DB7-2 %v con BOM verificata %v", r, !verificata, verificata)
				}
				visti[verificata]++
			}
		}
	}
	if visti[false] == 0 || visti[true] == 0 {
		t.Errorf("conferme viste con la BOM non verificata %d e verificata %d: servono tutte e due", visti[false], visti[true])
	}
}

// maRTestoU07: la nota U-07 alla lettera (ELENCO §6 U-07, R-B7-04).
const maRTestoU07 = `La strada principale è "Conferma l'albero", dopo il riepilogo. Le decisioni nodo per nodo non rendono la BOM verificata e, prese prima del riepilogo, possono lasciarla da verificare.`

// TestMotoreANotaR_B7_04: il testo U-07 alla lettera in #ma-albero; «vale per tutta la RFQ (NumeroTarget prodotti)»;
// #ma-albero prima di #ma-schede nel documento (FE-09, U-07).
func TestMotoreANotaR_B7_04(t *testing.T) {
	s := maRServer(t)
	visti := 0
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		if c.Pagina.Esito == nil {
			continue
		}
		for _, prodotto := range maRProdotti(c) {
			if prodotto == "" {
				continue
			}
			r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, true)
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			al := maRID(doc, "ma-albero")
			if len(al) != 1 {
				t.Errorf("%s: %d #ma-albero", r, len(al))
				continue
			}
			visti++
			nota := maRTutti(al[0], func(n *html.Node) bool { return maRClasse(n, "ma-u07") })
			if len(nota) != 1 || maRTesto(nota[0]) != maRTestoU07 {
				t.Errorf("%s: la nota U-07 non è alla lettera", r)
			}
			// revisione P3, B4: «1 prodotto», «n prodotti»
			parola := "prodotti"
			if r.P.Esito.Fascicolo.NumeroTarget == 1 {
				parola = "prodotto"
			}
			if atteso := fmt.Sprintf("vale per tutta la RFQ (%d %s)", r.P.Esito.Fascicolo.NumeroTarget, parola); !strings.Contains(maRTesto(al[0]), atteso) {
				t.Errorf("%s: #ma-albero senza «%s»", r, atteso)
			}
			if sc := maRID(doc, "ma-schede"); len(sc) == 1 {
				ordine := maRTutti(doc, func(n *html.Node) bool { id := maRA(n, "id"); return id == "ma-albero" || id == "ma-schede" })
				if maRA(ordine[0], "id") != "ma-albero" {
					t.Errorf("%s: #ma-schede prima di #ma-albero", r)
				}
			}
		}
	}
	if visti == 0 {
		t.Fatal("nessuna barra dell'albero")
	}
}

// maRAnteprime: ogni valutazione.RiferimentoAnteprima dell'esito (anche dentro i puntatori), per allegato: l'oracolo
// delle miniature.
func maRAnteprime(e *valutazione.EsitoThread) map[uuid.UUID][]valutazione.RiferimentoAnteprima {
	out := map[uuid.UUID][]valutazione.RiferimentoAnteprima{}
	tipo := reflect.TypeOf(valutazione.RiferimentoAnteprima{})
	var giro func(v reflect.Value)
	giro = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				giro(v.Elem())
			}
		case reflect.Struct:
			if v.Type() == tipo {
				a := v.Interface().(valutazione.RiferimentoAnteprima)
				if a.AllegatoID != nil {
					out[*a.AllegatoID] = append(out[*a.AllegatoID], a)
				}
				return
			}
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					giro(v.Field(i))
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				giro(v.Index(i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				giro(v.MapIndex(k))
			}
		}
	}
	giro(reflect.ValueOf(e))
	return out
}

// TestMotoreAMiniatureSoloDisponibili: ogni [data-ma-mini] ha data-ma-allegato (un UUID) e data-ma-sha non vuoto, e
// corrisponde a un'anteprima Disponibile con lo stesso sha (dell'esito o di Indice.Anteprima); ogni altra miniatura
// dice «anteprima non disponibile» (FE-06, X-09; mai un disegno inventato).
func TestMotoreAMiniatureSoloDisponibili(t *testing.T) {
	s := maRServer(t)
	mini := 0
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		if c.Pagina.Esito == nil {
			continue
		}
		ant := maRAnteprime(c.Pagina.Esito)
		for _, prodotto := range maRProdotti(c) {
			for _, vis := range []string{motorea_web.VistaDistinta, motorea_web.VistaRFQ} {
				r := maRRendi(s, nome, c, vis, prodotto, true)
				doc := maRDoc(t, r.Frammento.Body.Bytes())
				for _, n := range maRTutti(doc, maRCon("", "data-ma-mini")) {
					mini++
					a, err := uuid.Parse(maRA(n, "data-ma-allegato"))
					sha := maRA(n, "data-ma-sha")
					if err != nil || a == uuid.Nil || sha == "" {
						t.Errorf("%s: miniatura con allegato %q e sha %q", r, maRA(n, "data-ma-allegato"), sha)
						continue
					}
					// l'anteprima viene dall'esito (i disegni) o dalla ricerca dell'indice (R8, R9, fuori perimetro)
					ok := false
					for _, x := range ant[a] {
						if x.Disponibile && x.Sha256 == sha {
							ok = true
						}
					}
					if x := r.P.Indice.Anteprima(a); x != nil && x.Disponibile && x.Sha256 == sha {
						ok = true
					}
					if !ok {
						t.Errorf("%s: miniatura di %s senza un'anteprima disponibile con lo sha %q", r, a, sha)
					}
				}
				for _, n := range maRTutti(doc, func(n *html.Node) bool { return maRClasse(n, "ma-mini--na") }) {
					if tt := maRTesto(n); tt != "anteprima non disponibile" && tt != "nessun 2D associato" {
						t.Errorf("%s: miniatura spenta con %q", r, tt)
					}
				}
			}
		}
	}
	if mini == 0 {
		t.Fatal("nessuna miniatura nei casi: la prova non prova niente")
	}
}

// ---------------------------------------------------------------- le correzioni della revisione di P3

// TestMotoreARamiSenzaCasoSiRendono: i rami del template che nessun caso ACME tocca (revisione P3 dati, M-1 e B-1) si
// rendono con i tipi veri e dicono quello che il server manda. Sul caso e15, in memoria (i JSON non cambiano): un nodo
// con un disegno principale riceve AncheIn, un codice documentale con la Correzione e una DiscordanzaDecisione, una
// DiscordanzaRevisione del disegno e una RelazioneDisegni da confermare; il fascicolo non è calcolato. Un campo
// rinominato in valutazione o ancoraggio rompe la compilazione di questa prova o la resa, non la pagina vera.
func TestMotoreARamiSenzaCasoSiRendono(t *testing.T) {
	s := maRServer(t)
	c := leggiCasoMotoreA(t, "e15")
	e := c.Pagina.Esito
	p := &e.ProdottiValutati[0]
	var n *valutazione.NodoBOM
	for i := range p.Nodi {
		if p.Nodi[i].Disegni.Primario != nil {
			n = &p.Nodi[i]
			break
		}
	}
	if n == nil {
		t.Fatal("e15 senza un nodo con il disegno principale: la prova non prova niente")
	}
	altro := ancoraggio.RifComponente(maU("99"))
	rev := "B"
	n.AncheIn = []string{altro}
	n.Nodo.Codice.Documentale = append(n.Nodo.Codice.Documentale, ancoraggio.CodiceDocumentale{
		AllegatoID: maU("3f"), Lettura: "7120990", Esito: ancoraggio.RiconciliazioneCorrezioneProposta,
		Correzione: &ancoraggio.CorrezioneProposta{Codice: "7120991", Revisione: &rev, AllegatoID: maU("3f")},
		Discordanze: []ancoraggio.DiscordanzaDecisione{{Parte: ancoraggio.ParteDiscordanzaCodice,
			Effetto: ancoraggio.EffettoDiscordanzaIndicatore, Motivo: ancoraggio.MotivoDiscordanzaEvidenzaNuova,
			Decisione: ancoraggio.CodiceDeciso{Codice: "7120992"}}},
	})
	n.Disegni.Primario.Discordanze = append(n.Disegni.Primario.Discordanze, valutazione.DiscordanzaRevisione{
		Tra: valutazione.TraDocumentoComponente, A: "A", B: "C", FonteA: "cartiglio", FonteB: "decisione", Effetto: "indicatore"})
	n.Disegni.RelazioniDaConfermare = append(n.Disegni.RelazioniDaConfermare, valutazione.RelazioneDisegni{
		A: maShaA("3e"), B: maShaA("3f"), Tipo: valutazione.RelazioneRevisioneDiversa, Stato: valutazione.StatoRelazioneDaConfermare})
	e.Fascicolo.Calcolato = false

	for _, vis := range []string{motorea_web.VistaDistinta, motorea_web.VistaDocumenti} {
		r := maRRendi(s, "e15", c, vis, p.Rif, true)
		if r.Pagina.Code != http.StatusOK || r.Frammento.Code != http.StatusOK {
			t.Fatalf("%s: pagina %d, frammento %d: %.300s", r, r.Pagina.Code, r.Frammento.Code, r.Pagina.Body.String())
		}
		testo := maRTesto(maRDoc(t, r.Frammento.Body.Bytes()))
		for _, atteso := range []string{
			"anche in " + altro,                  // ma_legami_box (M-1: $n.AncheIn)
			"Anche in: " + altro,                 // ma_posizione
			"correzione proposta: 7120991 rev B", // Documentale[].Correzione
			"discorda sul codice con 7120992",    // Documentale[].Discordanze
			motorea_web.Etichetta("discordanza", valutazione.TraDocumentoComponente) + ": A ≠ C",                                            // Disegno2D.Discordanze
			"relazione da confermare fra due disegni: " + motorea_web.Etichetta("relazione_disegni", valutazione.RelazioneRevisioneDiversa), // RelazioniDaConfermare
			"fascicolo non calcolato", // B-1
		} {
			if !strings.Contains(testo, atteso) {
				t.Errorf("%s: manca «%s»", r, atteso)
			}
		}
		if strings.Contains(testo, "prodotti verificati") {
			t.Errorf("%s: con fascicolo.calcolato falso la pagina scrive ancora i contatori (B-1)", r)
		}
	}

	// S-04: un disegno disponibile senza sha non apre il visore (sempre /allegato/{id}/anteprima?v=<sha>)
	n.Disegni.Primario.Anteprima.Sha256 = ""
	n.Disegni.Primario.Anteprima.Disponibile = true
	r := maRRendi(s, "e15", c, motorea_web.VistaDistinta, p.Rif, true)
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	for _, b := range maRTutti(doc, func(x *html.Node) bool { return maRA(x, "data-ma-azione") == "pdf" }) {
		if maRA(b, "data-ma-sha") == "" {
			t.Errorf("%s: «Apri il visore» di %s senza sha (S-04)", r, maRA(b, "data-ma-file"))
		}
	}
}

// TestMotoreACaselleDetteAVoce: la casella si sente con quello che si vede (revisione P3, M5 e B2): ogni tree ha la
// spiegazione dei tasti; un legame del file non confermato si dice «legame da confermare» (o «confermata» con la
// quantità); la radice della BOM di lavoro senza Decisione dice il componente del prodotto, a vista e a voce.
func TestMotoreACaselleDetteAVoce(t *testing.T) {
	s := maRServer(t)
	visteRadici, visteFile := 0, 0
	for _, nome := range []string{"e3", "e14"} {
		r := maRCaso(t, s, nome, motorea_web.VistaDistinta, "", true)
		doc := maRDoc(t, r.Frammento.Body.Bytes())
		if len(maRID(doc, "ma-tasti-albero")) != 1 {
			t.Errorf("%s: %d #ma-tasti-albero, atteso uno", r, len(maRID(doc, "ma-tasti-albero")))
		}
		for _, tr := range maRTutti(doc, func(x *html.Node) bool { return maRA(x, "role") == "tree" }) {
			if maRA(tr, "aria-describedby") != "ma-tasti-albero" {
				t.Errorf("%s: un tree senza aria-describedby=ma-tasti-albero", r)
			}
		}
		for _, li := range maRTutti(doc, func(x *html.Node) bool { return maRA(x, "role") == "treeitem" }) {
			et := maRA(li, "aria-label")
			if maRClasse(li, "ma-li--file") {
				visteFile++
				if !strings.Contains(et, "legame da confermare") && !strings.Contains(et, "confermata") {
					t.Errorf("%s: casella %s con un legame del file e l'etichetta %q", r, maRA(li, "data-ma-rif"), et)
				}
			}
			if strings.Contains(et, "componente del prodotto") {
				visteRadici++
				box := maRTutti(li, func(x *html.Node) bool {
					return maRClasse(x, "ma-cat--ok") && strings.HasPrefix(maRTesto(x), "componente del prodotto ✓ ")
				})
				if len(box) == 0 {
					t.Errorf("%s: la radice %s dice a voce il componente del prodotto ma non a vista", r, maRA(li, "data-ma-rif"))
				}
			}
		}
	}
	if visteRadici == 0 || visteFile == 0 {
		t.Fatalf("radici col componente del prodotto %d, caselle con un legame del file %d: la prova non prova niente", visteRadici, visteFile)
	}
}

// TestMotoreATestiOstiliRestanoTesto: in ogni caso ACME «ACME» diventa un testo ostile (virgolette, <img onerror>,
// <script>, un'entità) e all'Errore si aggiunge un <img onerror>; ogni vista, prodotto e Scrive, pagina e frammento:
// nessun <img> o <script> iniettato, nessun attributo on…, ogni data-ma-campi è JSON (revisione P3, S-05). Il testo
// ostile deve arrivare nella pagina, come testo: se no la prova non prova niente.
func TestMotoreATestiOstiliRestanoTesto(t *testing.T) {
	s := maRServer(t)
	const ostile = `ACME\"><img src=x onerror=alert(1)>'<script>z()</script>&amp;`
	file, err := filepath.Glob(filepath.Join(maCartellaCasi, "e*_*.json"))
	if err != nil || len(file) == 0 {
		t.Fatalf("nessun caso in %s: %v", maCartellaCasi, err)
	}
	rese, conTesto, campi := 0, 0, 0
	for _, f := range file {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		c, err := maLeggiCasoStretto(bytes.ReplaceAll(b, []byte("ACME"), []byte(ostile)))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if c.Pagina.Errore != "" {
			c.Pagina.Errore += `<img src=x onerror=alert(2)>"'`
		}
		nome := filepath.Base(f)
		for _, vis := range maRViste {
			for _, pr := range maRProdotti(c) {
				for _, sc := range []bool{true, false} {
					r := maRRendi(s, nome, c, vis, pr, sc)
					if r.Pagina.Code != http.StatusOK || r.Frammento.Code != http.StatusOK {
						t.Errorf("%s: pagina %d, frammento %d", r, r.Pagina.Code, r.Frammento.Code)
						continue
					}
					for _, uscita := range [][]byte{r.Pagina.Body.Bytes(), r.Frammento.Body.Bytes()} {
						rese++
						doc := maRDoc(t, uscita)
						if strings.Contains(maRTesto(doc), "<img src=x onerror=alert(1)>") {
							conTesto++
						}
						for _, x := range maRTutti(doc, func(*html.Node) bool { return true }) {
							if x.DataAtom == atom.Img {
								t.Errorf("%s: <img> iniettato", r)
							}
							if x.DataAtom == atom.Script && strings.Contains(maRTesto(x), "z()") {
								t.Errorf("%s: <script> iniettato", r)
							}
							for _, a := range x.Attr {
								if strings.HasPrefix(strings.ToLower(a.Key), "on") {
									t.Errorf("%s: attributo %s=%q", r, a.Key, a.Val)
								}
								if a.Key == "data-ma-campi" {
									campi++
									var m map[string]any
									if err := json.Unmarshal([]byte(a.Val), &m); err != nil {
										t.Errorf("%s: data-ma-campi non è JSON: %q", r, a.Val)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if conTesto == 0 || campi == 0 {
		t.Fatalf("%d rese, %d con il testo ostile, %d data-ma-campi: la prova non prova niente", rese, conTesto, campi)
	}
	t.Logf("%d rese, %d con il testo ostile (rimasto testo), %d data-ma-campi", rese, conTesto, campi)
}
