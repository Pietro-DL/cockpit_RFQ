package web

// L1 — giro 5, A1d: il ponte dei gesti della Distinta nuova, web/static/motore_a_gesti.mjs (BE-13; contratto A1d
// §3.1-§3.3; v1.1 §4.9; piano A1d backend §5.7 e §7.2; T-A1d-08, T-A1d-30, T-A1d-31, T-A1d-32; RAC-13). Prove
// statiche sul sorgente, letto da risorse.FS (così -overlay vale anche qui): nessuna API che inserisca HTML, nessun
// resto del demo; la lista chiusa fra i marcatori ROTTE:inizio e ROTTE:fine è esattamente quella del contratto, e
// ogni sua rotta è una POST che il server registra; ogni fetch porta le intestazioni di htmx; l'API è quella fissa.
// Il comportamento (esiti, rilettura, doppio clic, login) lo prova il kit jsdom testdata/motore_a/kit/prove/ponte.cjs,
// che si lancia a mano con Node (QFE-04).
//
// I clienti di questi test sono inventati: UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

const maPonte = "web/static/motore_a_gesti.mjs"

func ponteSorgente(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(risorse.FS, maPonte)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ponteElenco: le stringhe fra «// <nome>:inizio» e «// <nome>:fine» del ponte, una per riga, nell'ordine.
func ponteElenco(t *testing.T, nome string) []string {
	t.Helper()
	src := ponteSorgente(t)
	_, dopo, ok := strings.Cut(src, "// "+nome+":inizio")
	if !ok {
		t.Fatalf("manca il marcatore // %s:inizio nel ponte", nome)
	}
	dentro, _, ok := strings.Cut(dopo, "// "+nome+":fine")
	if !ok {
		t.Fatalf("manca il marcatore // %s:fine nel ponte", nome)
	}
	if strings.Count(src, "// "+nome+":inizio") != 1 || strings.Count(src, "// "+nome+":fine") != 1 {
		t.Fatalf("i marcatori di %s compaiono più di una volta", nome)
	}
	var out []string
	for _, m := range regexp.MustCompile(`"([^"\n]*)"`).FindAllStringSubmatch(dentro, -1) {
		out = append(out, m[1])
	}
	return out
}

// maRotteContratto: le 29 POST del ponte (contratto A1d §3.4 più ripristina, v1.1 §4.6; piano A1d backend §5.7.2).
var maRotteContratto = []string{
	"/thread/{id}/fascicolo/componente/{u}/step-strutturale",
	"/thread/{id}/fascicolo/componente/{u}/step-strutturale/revoca",
	"/thread/{id}/fascicolo/nodo/{u}/accetta",
	"/thread/{id}/fascicolo/nodo/{u}/scarta",
	"/thread/{id}/fascicolo/nodo/{u}/codice",
	"/thread/{id}/fascicolo/nodo/{u}/riapri",
	"/thread/{id}/fascicolo/componente/{u}/codice",
	"/thread/{id}/fascicolo/componente/{u}/modifica",
	"/thread/{id}/fascicolo/componente/{u}/tipo",
	"/thread/{id}/fascicolo/componente/{u}/collega",
	"/thread/{id}/fascicolo/componente/{u}/sposta",
	"/thread/{id}/fascicolo/componente/{u}/scollega",
	"/thread/{id}/fascicolo/componente/{u}/archivia",
	"/thread/{id}/fascicolo/componente/{u}/rimuovi",
	"/thread/{id}/fascicolo/componente/{u}/ripristina",
	"/thread/{id}/fascicolo/relazione/accetta",
	"/thread/{id}/fascicolo/relazione/scarta",
	"/thread/{id}/fascicolo/rimozione/accetta",
	"/thread/{id}/fascicolo/rimozione/scarta",
	"/thread/{id}/fascicolo/conferma",
	"/proposta/{u}/scarta",
	"/thread/{id}/fascicolo/assegna",
	"/thread/{id}/fascicolo/documento/{u}/sostituisci",
	"/thread/{id}/fascicolo/componente/{u}/deroga",
	"/thread/{id}/fascicolo/deroga/{u}/revoca",
	"/thread/{id}/riprova-copie",
	"/thread/{id}/fascicolo/prepara",
	"/thread/{id}/fascicolo/rianalizza",
	"/thread/{id}/smistamento/aggiorna",
}

// maLettureContratto: le 4 GET JSON di sola lettura della pagina.
var maLettureContratto = []string{
	"/thread/{id}/motore-a/dati",
	"/thread/{id}/motore-a/anteprima/step-strutturale",
	"/thread/{id}/distinta/tipo",
	"/thread/{id}/distinta/albero/riepilogo",
}

func TestPonteNonInserisceHTML(t *testing.T) {
	src := ponteSorgente(t)
	// la sola eccezione dichiarata: lo stile della rilettura di htmx (contratto §3.3), che sostituisce i figli di #ma
	// con i nodi della GET della pagina; il ponte non scrive mai HTML da sé
	const swap = `hx.ajax("GET", location.href, { target: "#ma", swap: "innerHTML" })`
	if strings.Count(src, swap) != 1 {
		t.Fatalf("la rilettura con htmx deve comparire una volta, così: %s", swap)
	}
	resto := strings.Replace(src, swap, "", 1)
	vietate := []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "insertAdjacentElement", "createContextualFragment",
		"document.write", "eval(", "new Function", "srcdoc", "__esitiDemo", "__gesti", "__login", "copione", "registra(",
		"dalCopione", "location.reload", "http://", "https://", "analisi" + "_messaggio"}
	for _, v := range vietate {
		if strings.Contains(strings.ToLower(resto), strings.ToLower(v)) {
			t.Errorf("nel ponte c'è %q", v)
		}
	}
	// nella pagina il ponte scrive solo testo, in #ma-avviso (append di URLSearchParams è dei campi, non dei nodi)
	for _, v := range []string{"appendChild", "insertBefore", "replaceChild", "replaceWith", ".prepend(", ".before(", ".after("} {
		if strings.Contains(resto, v) {
			t.Errorf("il ponte inserisce nodi nella pagina: %q", v)
		}
	}
	if !strings.Contains(src, `el.textContent = testo || "";`) {
		t.Error("mostra non scrive l'avviso con textContent")
	}
}

func TestPonteLaListaDelleRotteEQuellaDelContratto(t *testing.T) {
	uguali := func(nome string, got, want []string) {
		t.Helper()
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s del ponte:\n%s\nattese (%d):\n%s", nome, strings.Join(got, "\n"), len(want), strings.Join(want, "\n"))
		}
	}
	rotte := ponteElenco(t, "ROTTE")
	uguali("ROTTE", rotte, maRotteContratto)
	if len(rotte) != 29 {
		t.Errorf("ROTTE ha %d voci, il contratto 29", len(rotte))
	}
	uguali("LETTURE", ponteElenco(t, "LETTURE"), maLettureContratto)
	for _, r := range append(rotte, ponteElenco(t, "LETTURE")...) {
		for _, v := range []string{"bom/applica", "congela", "revisione", "auto", "?", "/allegato/", "nodo:", "componente:"} {
			if strings.Contains(r, v) {
				t.Errorf("la rotta %q contiene %q, fuori dal contratto (DB7-5, DB7-6)", r, v)
			}
		}
	}
	src := ponteSorgente(t)
	if !strings.Contains(src, `const ALBERO = "/thread/{id}/distinta/albero/conferma";`) {
		t.Error("la conferma dell'albero non è la costante ALBERO, fuori da ROTTE")
	}
	if strings.Count(src, "ALBERO") != 2 {
		t.Error("ALBERO si usa solo in confermaAlbero")
	}
}

// TestPonteLeRotteEsistonoNelServer: ogni POST della lista è una rotta che il server registra (con UUID ACME al posto
// dei segnaposto), e così le quattro letture: le due della Distinta e le due motore-a di registraMotoreA (che
// TestMotoreALeRotteSiMontano prova anche con HEAD).
func TestPonteLeRotteEsistonoNelServer(t *testing.T) {
	s := serverTest(t)
	mux := http.NewServeMux()
	s.Registra(mux)
	id := "00000000-0000-4000-8000-000000000001"
	u := "00000000-0000-4000-8000-000000000010"
	prova := func(metodo, r string) {
		t.Helper()
		percorso := strings.NewReplacer("{id}", id, "{u}", u).Replace(r)
		req := httptest.NewRequest(metodo, percorso, nil)
		_, pat := mux.Handler(req)
		if !strings.HasPrefix(pat, metodo+" ") || maNormalizzaPattern(pat) != metodo+" "+maNormalizzaRotta(r) {
			t.Errorf("%s %s: il server lo manda a %q", metodo, r, pat)
		}
	}
	for _, r := range ponteElenco(t, "ROTTE") {
		prova("POST", r)
	}
	prova("POST", "/thread/{id}/distinta/albero/conferma")
	for _, r := range ponteElenco(t, "LETTURE") {
		prova("GET", r)
	}
}

func TestPonteIntestazioniEAPI(t *testing.T) {
	src := ponteSorgente(t)
	// ogni fetch porta HX-Request e HX-Current-URL (intestazioni), nella stessa chiamata
	pezzi := strings.Split(src, "fetch(")
	if len(pezzi)-1 != 3 {
		t.Errorf("il ponte ha %d fetch(, attese 3 (postHTTP, leggi, confermaAlbero)", len(pezzi)-1)
	}
	for i, p := range pezzi[1:] {
		profondita, fine := 1, -1
		for j, c := range p {
			if c == '(' {
				profondita++
			}
			if c == ')' {
				profondita--
				if profondita == 0 {
					fine = j
					break
				}
			}
		}
		if fine < 0 || !strings.Contains(p[:fine], "headers: intestazioni(") || !strings.Contains(p[:fine], `credentials: "same-origin"`) {
			t.Errorf("la fetch %d non porta headers: intestazioni(…) e credentials same-origin", i+1)
		}
	}
	if !strings.Contains(src, `{ "HX-Request": "true", "HX-Current-URL": location.href }`) {
		t.Error("intestazioni non mette HX-Request: true e HX-Current-URL = location.href")
	}
	var esportate []string
	for _, m := range regexp.MustCompile(`(?m)^export\s+(?:async\s+)?function\s+(\w+)`).FindAllStringSubmatch(src, -1) {
		esportate = append(esportate, m[1])
	}
	if strings.Join(esportate, ",") != "esegui,leggi,confermaAlbero,aggiorna" {
		t.Errorf("l'API esportata è %v, il contratto dice esegui, leggi, confermaAlbero, aggiorna", esportate)
	}
	if n := len(regexp.MustCompile(`(?m)^export\b`).FindAllString(src, -1)); n != 4 {
		t.Errorf("%d export, attesi 4", n)
	}
	if regexp.MustCompile(`(?m)^\s*import[\s{*"']`).MatchString(src) || strings.Contains(src, "import(") {
		t.Error("il ponte importa qualcosa: il kit non lo carica più togliendo gli export")
	}
	for _, v := range []string{"data-ma-esito", "ritorna_thread", "HX-Current-URL", `const INSTALLATO = "__maGesti";`,
		`const FATTO = "Fatto: il server non ha aggiunto altro.";`, "La pagina non si è riletta", "Sessione scaduta",
		"rotta non prevista dal contratto", "location.assign("} {
		if !strings.Contains(src, v) {
			t.Errorf("nel ponte manca %q", v)
		}
	}
}
