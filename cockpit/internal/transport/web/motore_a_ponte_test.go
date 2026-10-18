package web

// L1 — giro 5, A1d-Q3: il ponte dei gesti della Distinta nuova, web/static/motore_a_gesti.mjs (direttiva A1d §4.3;
// mappa dei gesti A1d-P3 §2). Prove statiche sul sorgente, letto da risorse.FS. Derivate dalle prove della PR #18 di
// Stefano Valo (67dcc02: motore_a_ponte_test.go), adattate alla lista chiusa di P3 e strette dopo la revisione di P3
// (Q3-01, Q3-02, Q3-06):
//   - la lista ROTTE ha 27 POST (il numero della mappa §2.1, dichiarato una volta: ponteNumeroRotte), senza doppioni,
//     e il mondo è chiuso: ogni rotta POST /thread/… o /proposta/… che il server registra (lette dai sorgenti del
//     package) è in ROTTE, è ALBERO o è fra le escluse di ponteEscluse con il suo motivo; relazione/accetta
//     (Q-A1d-06 [U]) e relazione/scarta (Q-P3-01) sono registrate nel server e assenti dal ponte. La mappa è un documento
//     privato fuori dal repository: la prova non la legge, e una rotta nuova (nel server o nel ponte) costringe a
//     scegliere di proposito in quale delle tre parti va;
//   - ogni rotta della lista esiste nel mux del server con il suo metodo; LETTURE ha le quattro GET;
//   - nessuna API che inserisca HTML o nodi del server (salvo la rilettura di htmx della pagina, alle sue condizioni),
//     nessun altro canale d'invio (XHR, sendBeacon, submit, WebSocket), nessuna rotta aggiunta fuori dal blocco ROTTE,
//     nessuna stringa di percorso fuori dalle liste, nessun resto dimostrativo, nessun import;
//   - ogni fetch porta HX-Request e HX-Current-URL = la pagina /thread/{id}/motore-a, credenziali della stessa origine;
//   - le righe che decidono sono fissate per testo esatto (l'esito ok/no, il controllo dell'altra RFQ, la lista e la
//     guardia in esegui, il corpo di ogni guardia, le condizioni della rilettura, lo stato occupato): una mutazione
//     plausibile di una di queste righe fa cadere la prova (esiti di Q3, «Dopo la revisione»).
//
// LIMITE DICHIARATO: sono prove sul testo, non sul comportamento. Una riscrittura equivalente fa cadere la prova (che va
// aggiornata di proposito), e un difetto fuori dalle righe fissate può passare. Il comportamento nel DOM (esiti,
// rilettura, doppio clic, login, guardie eseguite) lo proverà il kit jsdom (testdata/motore_a/kit/prove/ponte.cjs della
// PR, ramo d'archivio, da riallineare a P3): serve Node, che su questa postazione non c'è. NON ESEGUITO in Q3; il kit e
// la prova da capo a fondo della pagina sono di A1d-P4.

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

const ponteFile = "web/static/motore_a_gesti.mjs"

// ponteNumeroRotte: le POST della lista chiusa, dalla mappa dei gesti A1d-P3, §2.1 (bozza dell'11/10: «ROTTE di P3: 27
// POST»). Il solo punto delle prove in cui il numero è scritto.
const ponteNumeroRotte = 27

// ponteNumeroLetture: le GET JSON di sola lettura (mappa §2.1, LETTURE).
const ponteNumeroLetture = 4

func ponteSorgente(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(risorse.FS, ponteFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// ponteCodice: il sorgente senza le righe di solo commento (le parole dei commenti non sono codice).
func ponteCodice(t *testing.T) string {
	t.Helper()
	return ponteSenzaCommenti(ponteSorgente(t))
}

func ponteSenzaCommenti(src string) string {
	var out []string
	for _, riga := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(riga), "//") {
			continue
		}
		out = append(out, riga)
	}
	return strings.Join(out, "\n")
}

// ponteBlocco: il testo fra «// <nome>:inizio» e «// <nome>:fine», marcatori presenti una volta sola.
func ponteBlocco(t *testing.T, nome string) string {
	t.Helper()
	src := ponteSorgente(t)
	if strings.Count(src, "// "+nome+":inizio") != 1 || strings.Count(src, "// "+nome+":fine") != 1 {
		t.Fatalf("i marcatori di %s mancano o compaiono più di una volta", nome)
	}
	_, dopo, _ := strings.Cut(src, "// "+nome+":inizio")
	dentro, _, _ := strings.Cut(dopo, "// "+nome+":fine")
	return dentro
}

var reStringa = regexp.MustCompile(`"([^"\n]*)"`)

// ponteElenco: le stringhe di un blocco (fuori dalle righe di commento), una per riga, nell'ordine.
func ponteElenco(t *testing.T, nome string) []string {
	t.Helper()
	var out []string
	for _, riga := range strings.Split(ponteBlocco(t, nome), "\n") {
		if strings.HasPrefix(strings.TrimSpace(riga), "//") {
			continue
		}
		for _, m := range reStringa.FindAllStringSubmatch(riga, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// ponteFunzione: il corpo di una funzione del ponte (dalla firma alla prima «}» a inizio riga).
func ponteFunzione(t *testing.T, codice, firma string) string {
	t.Helper()
	_, corpo, ok := strings.Cut(codice, firma)
	if !ok {
		t.Fatalf("nel ponte manca %q", firma)
	}
	corpo, _, _ = strings.Cut(corpo, "\n}\n")
	return corpo
}

var reSegnaposto = regexp.MustCompile(`\{[^}]*\}`)

// normalizza: i segnaposto ({id}, {u}, {cid}, {pid}…) tutti uguali, per confrontare il ponte con le rotte del server.
func normalizza(r string) string { return reSegnaposto.ReplaceAllString(r, "{}") }

// ponteAlbero: la chiamata separata di «Conferma l'albero».
const ponteAlbero = "/thread/{id}/distinta/albero/conferma"

// ponteEscluse: le rotte POST /thread/… e /proposta/… che il server registra e il ponte NON chiama, con il motivo
// (mappa §1.F e §2.2, o fuori dalla Distinta). Insieme a ROTTE e ad ALBERO sono tutte le POST del server sotto quei
// prefissi: una rotta nuova va messa di proposito in una delle tre parti.
var ponteEscluse = map[string]string{
	"/thread/{id}/fascicolo/relazione/accetta":                          "Q-A1d-06 = A [U]: la BOM si conferma con «Conferma l'albero»",
	"/thread/{id}/fascicolo/relazione/scarta":                           "Q-P3-01, base di lavoro A: fuori finché non è decisa",
	"/thread/{id}/fascicolo/bom/applica":                                "mappa §1.F: editor in blocco (E37, righe senza segno)",
	"/thread/{id}/fascicolo/file/{aid}/accetta":                         "mappa §1.F: catena cumulativa (D2, Q-06)",
	"/thread/{id}/fascicolo/proposta/{pid}/decidi":                      "mappa §1.F: R-B7-05",
	"/proposta/{id}/conferma":                                           "mappa §1.F: conferma dal pannello Inbox",
	"/thread/{id}/fascicolo/componente/{cid}/step-strutturale/riattiva": "mappa §1.F: TEC-15",
	"/thread/{id}/fascicolo/nota":                                       "mappa §1.F: note sui PDF fuori dalla v1",
	"/thread/{id}/fascicolo/nota/{nid}/modifica":                        "mappa §1.F: note sui PDF fuori dalla v1",
	"/thread/{id}/fascicolo/nota/{nid}/elimina":                         "mappa §1.F: note sui PDF fuori dalla v1",
	"/thread/{id}/fascicolo/componente/{cid}/deroga-struttura":          "mappa §1.F: fuori dal contratto §3.4",
	"/thread/{id}/fascicolo/deroga-struttura/{did}/revoca":              "mappa §1.F: fuori dal contratto §3.4",
	"/thread/{id}/fascicolo/documento/{did}/annulla-sostituzione":       "mappa §1.F: fuori dal contratto §3.4",
	"/thread/{id}/fascicolo/revisione/apri":                             "mappa §1.F: fuori dal contratto §3.4",
	"/thread/{id}/fascicolo/revisione/abbandona":                        "mappa §1.F: fuori dal contratto §3.4",
	"/thread/{id}/fascicolo/congela":                                    "mappa §1.F: fuori dal contratto §3.4",
	"/thread/{id}/distinta/albero/riepilogo":                            "la POST del riepilogo con una bozza non vuota: fuori dalla v1 (la GET è in LETTURE)",
	"/thread/{id}/fascicolo/nas/importa":                                "non è un gesto della Distinta: caricamenti del Fascicolo",
	"/thread/{id}/fascicolo/carica":                                     "non è un gesto della Distinta: caricamenti del Fascicolo",
	"/thread/{id}/fascicolo/file/{pid}/generale":                        "non è un gesto della Distinta: documenti generali del Fascicolo",
	"/thread/{id}/richiesta":                                            "non è un gesto della Distinta: richieste ai fornitori",
	"/thread/{id}/richiesta/{rid}/annulla":                              "non è un gesto della Distinta: richieste ai fornitori",
	// V1, ondata B (corsia persistenza): le POST della bozza della Distinta unica le chiama il ponte nuovo
	// (distinta_unica_ponte.mjs, EM §9), non questo; restano qui fino alla riscrittura di T-06 (R-7 dell'interfaccia)
	"/thread/{id}/motore-a/bozza":                   "Distinta unica di V1: R2 del ponte nuovo (EM §9.2), fino a T-06",
	"/thread/{id}/motore-a/bozza/{bozza}/abbandona": "Distinta unica di V1: R3 del ponte nuovo (EM §9.3), fino a T-06",
	"/thread/{id}/motore-a/riallinea":               "Distinta unica di V1: R11 (EM §9.8), fino a T-06",
}

// ponteDedicate (V1, PM23 PP-09 e T-06): le rotte POST che la pagina nuova chiama direttamente, con il loro contratto
// JSON (EM §10.9), fuori dalla lista ROTTE del ponte: «Scarta», «Metti in Altri file» e «Riattiva» della Sorgente, e
// «carica», che lascia le escluse per il suo ramo JSON (E1′; il ramo HTML resta della pagina della RFQ, 14/10). E' la
// quarta parte del mondo chiuso: registrate = ROTTE ∪ {ALBERO} ∪ escluse ∪ dedicate, e le parti non si toccano.
var ponteDedicate = map[string]string{
	"/thread/{id}/sorgente/file/{allegato}/scarta":     "S2, EM §10.9",
	"/thread/{id}/sorgente/file/{allegato}/altri-file": "S3, EM §10.9",
	"/thread/{id}/sorgente/file/{allegato}/riattiva":   "S4, EM §10.9",
	"/thread/{id}/carica":                              "E1′, il ramo JSON (EM §10.9); il ramo HTML resta della pagina della RFQ (14/10)",
}

// ponteRotteRegistrate: le rotte «METODO /thread/…» e «METODO /proposta/…» che il server registra, lette dai sorgenti
// non di prova del package (mux.HandleFunc con il pattern scritto per esteso), normalizzate.
func ponteRotteRegistrate(t *testing.T, metodo string) map[string]bool {
	t.Helper()
	file, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("` + metodo + ` (/(?:thread|proposta)/[^"]*)"`)
	out := map[string]bool{}
	for _, f := range file {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			out[normalizza(m[1])] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("nessuna rotta %s letta dai sorgenti", metodo)
	}
	return out
}

func chiaviOrdinate(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestPonteLaListaChiusa (Q3-01): il numero, i doppioni, il mondo chiuso rispetto alle rotte registrate, le due escluse
// della relazione, ALBERO e LETTURE.
func TestPonteLaListaChiusa(t *testing.T) {
	rotte := ponteElenco(t, "ROTTE")
	if len(rotte) != ponteNumeroRotte {
		t.Errorf("ROTTE ha %d voci, la mappa §2.1 %d", len(rotte), ponteNumeroRotte)
	}
	letture := ponteElenco(t, "LETTURE")
	if len(letture) != ponteNumeroLetture {
		t.Errorf("LETTURE ha %d voci, attese %d", len(letture), ponteNumeroLetture)
	}
	nelPonte := map[string]bool{}
	for _, r := range rotte {
		if nelPonte[normalizza(r)] {
			t.Errorf("rotta ripetuta in ROTTE: %q", r)
		}
		nelPonte[normalizza(r)] = true
	}
	// il mondo chiuso: registrate = ROTTE ∪ {ALBERO} ∪ escluse, e le tre parti non si toccano
	registrate := ponteRotteRegistrate(t, "POST")
	escluse := map[string]bool{}
	for r := range ponteEscluse {
		escluse[normalizza(r)] = true
		if nelPonte[normalizza(r)] {
			t.Errorf("la rotta esclusa %q è nella lista del ponte (%s)", r, ponteEscluse[r])
		}
	}
	dedicate := map[string]bool{}
	for r := range ponteDedicate {
		dedicate[normalizza(r)] = true
		if nelPonte[normalizza(r)] || escluse[normalizza(r)] {
			t.Errorf("la rotta dedicata %q è anche nella lista del ponte o fra le escluse (%s)", r, ponteDedicate[r])
		}
	}
	if nelPonte[normalizza(ponteAlbero)] || escluse[normalizza(ponteAlbero)] || dedicate[normalizza(ponteAlbero)] {
		t.Error("ALBERO è a parte: né in ROTTE né fra le escluse né fra le dedicate")
	}
	attese := map[string]bool{normalizza(ponteAlbero): true}
	for r := range nelPonte {
		attese[r] = true
	}
	for r := range escluse {
		attese[r] = true
	}
	for r := range dedicate {
		attese[r] = true
	}
	for _, r := range chiaviOrdinate(registrate) {
		if !attese[r] {
			t.Errorf("il server registra POST %s, che non è in ROTTE, non è ALBERO, non è fra le escluse né fra le dedicate: va deciso di proposito", r)
		}
	}
	for _, r := range chiaviOrdinate(attese) {
		if !registrate[r] {
			t.Errorf("POST %s (ROTTE, ALBERO o escluse) non è registrata nel server", r)
		}
	}
	// la coppia della relazione: registrata nel server, esclusa, assente da ogni stringa del codice del ponte
	codice := ponteCodice(t)
	for _, r := range []string{"/thread/{id}/fascicolo/relazione/accetta", "/thread/{id}/fascicolo/relazione/scarta"} {
		if !registrate[normalizza(r)] || ponteEscluse[r] == "" {
			t.Errorf("%s deve essere registrata nel server ed esclusa dal ponte", r)
		}
	}
	// (le escluse sono POST: le GET di LETTURE non si confrontano con loro)
	for _, m := range reStringa.FindAllStringSubmatch(ponteSenzaCommenti(strings.Replace(ponteSorgente(t), ponteBlocco(t, "LETTURE"), "", 1)), -1) {
		if strings.Contains(m[1], "relazione/") || escluse[normalizza(m[1])] {
			t.Errorf("una stringa del codice del ponte è una rotta esclusa: %q", m[1])
		}
	}
	for _, r := range append(append([]string(nil), rotte...), letture...) {
		for _, v := range []string{"auto", "?", "/allegato/", "nodo:", "componente:"} {
			if strings.Contains(r, v) {
				t.Errorf("la rotta %q contiene %q", r, v)
			}
		}
	}
	if !strings.Contains(codice, `const ALBERO = "`+ponteAlbero+`";`) {
		t.Error("la conferma dell'albero non è la costante ALBERO, fuori da ROTTE")
	}
	if strings.Count(codice, "ALBERO") != 2 {
		t.Error("ALBERO si usa solo in confermaAlbero")
	}
	if !strings.Contains(codice, `JSON.stringify({ firma, bozza: { formato: 1, base } })`) {
		t.Error("«Conferma l'albero» non manda {firma, bozza: {formato: 1, base}}, la bozza vuota della v1")
	}
}

// TestPonteLeRotteEsistonoNelServer: ogni POST della lista, la conferma dell'albero e le quattro letture sono rotte che il
// server registra con quel metodo (UUID inventati al posto dei segnaposto). Le letture motore-a vanno al loro
// registraMotoreA anche a flag spento: il mux le monta sempre, la porta è dentro.
func TestPonteLeRotteEsistonoNelServer(t *testing.T) {
	s := serverTest(t)
	mux := http.NewServeMux()
	s.Registra(mux)
	id := "00000000-0000-4000-8000-000000000001"
	u := "00000000-0000-4000-8000-000000000010"
	prova := func(metodo, r string) {
		t.Helper()
		percorso := strings.NewReplacer("{id}", id, "{u}", u).Replace(r)
		_, pat := mux.Handler(httptest.NewRequest(metodo, percorso, nil))
		if normalizza(pat) != metodo+" "+normalizza(r) {
			t.Errorf("%s %s: il server lo manda a %q", metodo, r, pat)
		}
	}
	for _, r := range ponteElenco(t, "ROTTE") {
		prova(http.MethodPost, r)
	}
	prova(http.MethodPost, ponteAlbero)
	for _, r := range ponteElenco(t, "LETTURE") {
		prova(http.MethodGet, r)
	}
}

// TestPonteNonInserisceHTMLENonHaResti (Q3-06): nessun inserimento di HTML o di nodi del server, nessun altro canale
// d'invio, nessuna rotta aggiunta fuori dal blocco, nessuna stringa di percorso fuori dalle liste.
func TestPonteNonInserisceHTMLENonHaResti(t *testing.T) {
	src := ponteSorgente(t)
	// la sola eccezione dichiarata: la rilettura di htmx, che sostituisce i figli di #ma con la pagina del server
	// riletta (GET /thread/{id}/motore-a, alle condizioni di alPrimaDelloSwap), mai con la risposta di un gesto
	const swap = `hx.ajax("GET", pagina(), { target: "#ma", swap: "innerHTML" })`
	if strings.Count(src, swap) != 1 {
		t.Fatalf("la rilettura con htmx deve comparire una volta, così: %s", swap)
	}
	resto := strings.Replace(src, swap, "", 1)
	for _, v := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "insertAdjacentElement", "createContextualFragment",
		"document.write", "eval(", "new Function", "Function(", "srcdoc", "__esitiDemo", "rifiutiMotoreA", "esitoMotoreA", "__gesti",
		"__login", "copione", "dalCopione", "location.reload", "http://", "https://", "cdn", "unpkg", "jsdelivr",
		"hx.swap", "htmx.swap", "importNode", "adoptNode", "HTMLUnsafe", "XMLHttpRequest", "sendBeacon", ".submit(",
		"requestSubmit", "WebSocket", "EventSource", "htmx.config", "hx.config"} {
		if strings.Contains(strings.ToLower(resto), strings.ToLower(v)) {
			t.Errorf("nel ponte c'è %q", v)
		}
	}
	codice := ponteCodice(t)
	for _, v := range []string{"demo", "simul", "finto", "setTimeout", "setInterval", "localStorage", "sessionStorage"} {
		if strings.Contains(strings.ToLower(codice), strings.ToLower(v)) {
			t.Errorf("nel codice del ponte c'è %q (un modo dimostrativo, una risposta simulata, un timer, uno stato tenuto in proprio)", v)
		}
	}
	// nella pagina il ponte scrive solo testo, in #ma-avviso; l'unico nodo che inserisce è la regione dell'avviso che
	// crea da sé quando il template non l'ha (append di URLSearchParams è dei campi, non dei nodi)
	for _, v := range []string{"appendChild", "insertBefore", "replaceChild", "replaceWith", "replaceChildren", ".prepend(", ".before(", ".after("} {
		if strings.Contains(codice, v) {
			t.Errorf("il ponte inserisce nodi nella pagina: %q", v)
		}
	}
	if n, p, b := strings.Count(codice, ".append("), strings.Count(codice, "p.append("), strings.Count(codice, "document.body.append(el);"); n != p+b || b != 1 {
		t.Errorf("append: %d in tutto, %d di p (i campi), %d di document.body (la regione dell'avviso, attesa una): un append su un altro elemento", n, p, b)
	}
	if !strings.Contains(codice, `el = document.createElement("div");`) {
		t.Error("la regione dell'avviso che il ponte inserisce deve essere un elemento creato da sé")
	}
	if !strings.Contains(codice, `el.textContent = testo || "";`) {
		t.Error("mostra non scrive l'avviso con textContent")
	}
	// un solo hx.ajax (la rilettura), le liste usate solo dove servono, nessuna rotta aggiunta fuori dal blocco
	if n := strings.Count(codice, "hx.ajax("); n != 1 {
		t.Errorf("%d hx.ajax(, atteso solo quello della rilettura", n)
	}
	for nome, n := range map[string]int{`\bROTTE\b`: 2, `\bRE_ROTTE\b`: 2, `\bLETTURE\b`: 2, `\bRE_LETTURE\b`: 2, `\bGUARDIE\b`: 2} {
		if g := len(regexp.MustCompile(nome).FindAllString(codice, -1)); g != n {
			t.Errorf("%s compare %d volte nel codice, attese %d (la dichiarazione e il suo solo uso)", nome, g, n)
		}
	}
	if strings.Count(codice, "GUARDIE[a.rotta]") != 1 {
		t.Error("GUARDIE si legge solo con la rotta trovata in esegui")
	}
	for _, v := range []string{".push(", ".concat(", ".splice(", ".unshift(", "defineProperty", "`", "'/"} {
		if strings.Contains(codice, v) {
			t.Errorf("nel codice del ponte c'è %q (una lista che cresce, una proprietà ridefinita, una stringa fuori dai doppi apici)", v)
		}
	}
	// ogni stringa di percorso del codice, fuori dalle due liste, è un pezzo noto della pagina, /login o ALBERO
	fuori := ponteSenzaCommenti(strings.Replace(strings.Replace(src, ponteBlocco(t, "ROTTE"), "", 1), ponteBlocco(t, "LETTURE"), "", 1))
	ammesse := map[string]bool{"/thread/": true, "/motore-a": true, "/proposta/": true, "/login": true, ponteAlbero: true}
	for _, r := range ponteElenco(t, "ROTTE") {
		ammesse[r] = true // le chiavi di GUARDIE (TestPonteLeGuardieDiInvio: ognuna è una rotta della lista)
	}
	for _, m := range reStringa.FindAllStringSubmatch(fuori, -1) {
		if strings.HasPrefix(m[1], "/") && !ammesse[m[1]] {
			t.Errorf("una stringa di percorso fuori dalle liste: %q", m[1])
		}
	}
	// nessun import (né statico né dinamico): niente CDN, niente moduli
	if regexp.MustCompile(`(?m)^\s*import[\s{*"']`).MatchString(src) || strings.Contains(codice, "import(") {
		t.Error("il ponte importa qualcosa")
	}
}

// TestPonteLaRiletturaHaLeSueCondizioni (P3-PONTE-02, -03): la rilettura entra in #ma solo alle condizioni dichiarate, con
// un limite di tempo, e marca #ma occupata.
func TestPonteLaRiletturaHaLeSueCondizioni(t *testing.T) {
	codice := ponteCodice(t)
	for _, v := range []string{
		`document.addEventListener("htmx:configRequest", alConfigRequest);`,
		`document.addEventListener("htmx:beforeSwap", alPrimaDelloSwap);`,
		`if (d && laRilettura(d, d.target)) d.timeout = RILETTURA_MS;`,
		`const RILETTURA_MS = 60000;`,
		`String(cfg.verb || "").toLowerCase() === "get" &&`,
		`target.id === "ma" &&`,
		`percorsoDi(String(cfg.path || "")) === percorsoPagina()`,
		`if (!d || !d.shouldSwap || !laRilettura(d.requestConfig, d.target)) return;`,
		`const altrove = !x || percorsoDi(String(x.responseURL || "")) !== percorsoPagina();`,
		`if (altrove || frammentoNonAmmesso(String(d.serverResponse || "")) !== "") {`,
		`d.shouldSwap = false;`,
		`const ATTRIBUTO_VIETATO = /^(data-)?hx-(get|post|put|patch|delete|swap-oob|on)/i;`,
		`if (marcati.length !== 1 || (marcati[0].getAttribute("data-ma-pagina") || "").toLowerCase() !== threadID().toLowerCase()) {`,
		`if (doc.getElementById("ma") || doc.getElementById("ma-avviso")) return "con #ma o #ma-avviso dentro";`,
		`if (el.tagName === "SCRIPT") return "con uno script";`,
		`for (const at of el.attributes) if (ATTRIBUTO_VIETATO.test(at.name)) return "con l'attributo " + at.name;`,
		`ma.setAttribute("aria-busy", "true");`,
		`ma.removeAttribute("aria-busy");`,
	} {
		if !strings.Contains(codice, v) {
			t.Errorf("rilettura: nel ponte manca %q", v)
		}
	}
	// il percorso della pagina: lo stesso pezzo per HX-Current-URL e per il confronto della rilettura
	if !strings.Contains(codice, `return "/thread/" + threadID() + "/motore-a";`) || !strings.Contains(codice, "return location.origin + percorsoPagina();") {
		t.Error("pagina() e percorsoPagina() non sono la pagina /thread/{id}/motore-a")
	}
	r := ponteFunzione(t, codice, "async function rileggi() {")
	if i, j := strings.Index(r, `ma.setAttribute("aria-busy", "true");`), strings.Index(r, "await hx.ajax("); i < 0 || j < 0 || i > j {
		t.Error("rileggi: aria-busy su #ma prima della rilettura")
	}
	if !strings.Contains(r, "} finally {\n    ma.removeAttribute(\"aria-busy\");") {
		t.Error("rileggi: aria-busy tolto in un finally")
	}
}

func TestPonteIntestazioniEAPI(t *testing.T) {
	codice := ponteCodice(t)
	// ogni fetch porta headers: intestazioni(…) e credentials same-origin, nella stessa chiamata
	pezzi := strings.Split(codice, "fetch(")
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
	for _, v := range []string{
		`{ "HX-Request": "true", "HX-Current-URL": pagina() }`,              // ogni richiesta, anche le GET JSON
		`"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8"`, // il corpo dei gesti
		`if (Array.isArray(v)) v.forEach((x) => p.append(k, valoreDi(x)));`, // campi ripetuti, senza []
		`else p.append(k, valoreDi(v));`,
		`p.append(c[0], valoreDi(c[1]));`,
		`if (a.url.search !== "") return finisci("errore"`, // niente query nei gesti
		`if (u.origin !== location.origin) return { motivo: "rotta di un'altra origine" };`,
		// il controllo dell'altra RFQ, per intero (Q3-02, mutazione b)
		`if (m[1] !== undefined && m[1].toLowerCase() !== t.toLowerCase()) return { motivo: "rotta di un'altra RFQ" };`,
		`if (!t) return { motivo: "il ponte vale solo nella pagina /thread/{id}/motore-a" };`,
		// la lista e il suo rifiuto in esegui
		`const a = ammessa(url, RE_ROTTE);`,
		`if (!a.url) return finisci("errore", "Il gesto non è partito: " + a.motivo + ".");`,
		`const a = ammessa(url, RE_LETTURE);`,
		// gli esiti: 4xx/5xx errore; solo un 200 text/html si legge (P3-PONTE-05); 2xx senza esito errore
		`if (!risp.ok) return { esito: "errore"`,
		`if (risp.status !== 200 || !tipoHTML(risp)) return { esito: "errore", testo: SENZA_ESITO };`,
		`return (risp.headers.get("Content-Type") || "").trim().toLowerCase().startsWith("text/html");`,
		`return leggiEsito(corpo) || { esito: "errore", testo: SENZA_ESITO };`,
		// l'esito come testo, ok e no per testo esatto (Q3-02, mutazione a)
		`const v = el.getAttribute("data-ma-esito");`,
		`const testo = (el.textContent || "")`,
		`if (v === "ok") return { esito: "ok", testo: testo || FATTO };`,
		`if (v === "no") return { esito: "no", testo: testo || NIENTE };`,
		`return { esito: "errore", testo: "Esito sconosciuto «" + v + "»`,
		`if (!el || doc.body.childElementCount !== 1 || el.tagName !== "DIV" || !el.hasAttribute("data-ma-esito")) return null;`,
		// la sessione scaduta: detta e poi il login (P3-PONTE-06)
		`if (risp.status === 401)`,
		`if (red && percorsoDi(red) === "/login") return red;`,
		"mostra(\"errore\", SESSIONE);\n  vaiAlLogin(dove);",
		`p.set("ritorna_thread", threadID());`,
		`if (risp.status === 200 && j) r = { esito: "ok"`,
		`else if (risp.status === 422 && j) r = { esito: "no"`,
		// solo stringhe e numeri finiti nei campi (P3-PONTE-04)
		"if (typeof v === \"string\") return v;\n  if (typeof v === \"number\" && Number.isFinite(v)) return String(v);\n  throw new TypeError(",
		// la regione viva dell'avviso (P3-PONTE-06)
		`if (!el.hasAttribute("role")) el.setAttribute("role", "status");`,
		`if (!el.hasAttribute("aria-live")) el.setAttribute("aria-live", "polite");`,
	} {
		if !strings.Contains(codice, v) {
			t.Errorf("nel ponte manca %q", v)
		}
	}
	// «ok» e «no» nascono solo nei due punti che leggono l'esito (leggiEsito e confermaAlbero)
	for v, n := range map[string]int{`esito: "ok"`: 2, `esito: "no"`: 2, `if (r.login) return sessioneScaduta(r.login);`: 2} {
		if g := strings.Count(codice, v); g != n {
			t.Errorf("%q compare %d volte, attese %d", v, g, n)
		}
	}
	// la rilettura solo dopo un «ok» verificato, nei due punti che la chiamano
	if n := strings.Count(codice, "await aggiorna()"); n != 2 || strings.Count(codice, `if (r.esito === "ok") await aggiorna();`) != 2 {
		t.Errorf("la rilettura: %d chiamate, attese 2, ognuna dopo if (r.esito === \"ok\")", n)
	}
	// un solo stato occupato (gesto in volo o rilettura): esegui e confermaAlbero lo controllano prima di partire e lo
	// liberano in un finally DOPO la rilettura; lancia lo controlla prima di spegnere i pulsanti (P3-PONTE-03)
	for _, v := range []string{"let inVolo = false;", "return inVolo || rilettura !== null || prossima !== null;",
		`el.getAttribute("data-ma-in-corso") === "1"`, "b.disabled = true;", "if (occupato()) {\n    mostra(\"errore\", IN_VOLO);"} {
		if !strings.Contains(codice, v) {
			t.Errorf("doppi invii: nel ponte manca %q", v)
		}
	}
	if strings.Count(codice, `if (occupato()) return finisci("errore", IN_VOLO);`) != 2 || strings.Count(codice, "inVolo = true;") != 2 ||
		strings.Count(codice, "    inVolo = false;") != 2 || strings.Contains(codice, "if (inVolo)") {
		t.Error("esegui e confermaAlbero devono tutti e due controllare occupato() e liberare inVolo una volta")
	}
	for _, firma := range []string{"export async function esegui(", "export async function confermaAlbero("} {
		f := ponteFunzione(t, codice, firma)
		o, v, a, l := strings.Index(f, "if (occupato())"), strings.Index(f, "inVolo = true;"), strings.Index(f, `if (r.esito === "ok") await aggiorna();`), strings.Index(f, "} finally {\n    inVolo = false;")
		if o < 0 || v < o || a < v || l < a {
			t.Errorf("%s: occupato(), poi inVolo = true, poi la rilettura, poi inVolo = false nel finally", firma)
		}
	}
	var esportate []string
	for _, m := range regexp.MustCompile(`(?m)^export\s+(?:async\s+)?function\s+(\w+)\(([^)]*)\)`).FindAllStringSubmatch(codice, -1) {
		esportate = append(esportate, m[1]+"("+m[2]+")")
	}
	if strings.Join(esportate, ",") != "esegui(url, campi),leggi(url),confermaAlbero(firma, base),aggiorna()" {
		t.Errorf("l'API esportata è %v", esportate)
	}
	if n := len(regexp.MustCompile(`(?m)^export\b`).FindAllString(codice, -1)); n != 4 {
		t.Errorf("%d export, attesi 4", n)
	}
}

// TestPonteLeGuardieDiInvio: le guardie della mappa §2.1 (Q-P3-02, base di lavoro A) sono nel blocco GUARDIE, una per
// rotta della lista, con i nomi dei campi della mappa e le righe che decidono per testo esatto (Q3-02, mutazione c);
// esegui le applica alla rotta trovata prima dell'invio. I campi che il gestore legge con FormValue passano da unico:
// una volta sola, il primo valore non vuoto dove serve (P3-PONTE-01). In «Sposta» la quantità si chiede solo con un
// padre di arrivo (SEM-03).
func TestPonteLeGuardieDiInvio(t *testing.T) {
	blocco := ponteBlocco(t, "GUARDIE")
	attese := map[string][]string{
		"/thread/{id}/fascicolo/nodo/{u}/accetta": {`p.getAll("tipo").some((v) => v.trim().toLowerCase() === "commerciale")`,
			`? "«Accetta il nodo» non offre il tipo commerciale`},
		"/thread/{id}/fascicolo/componente/{u}/modifica": {`if (p.has("tipo")) return "«Rev e descrizione» non cambia il tipo`,
			`const m = unico(p, "rev", true) || unico(p, "descrizione", true);`, `return m ? "«Rev e descrizione» manda sempre rev e descrizione insieme`},
		"/thread/{id}/fascicolo/componente/{u}/collega": {`const m = unico(p, "qta", false);`, `return m ? "la quantità va scritta, una volta sola`},
		"/thread/{id}/fascicolo/componente/{u}/sposta": {`const m = unico(p, "da", true) || unico(p, "a", true);`,
			`if (m) return "«Sposta» manda sempre da e a`, `if (p.get("a").trim() === "") return "";`, `const q = unico(p, "qta", false);`,
			`return q ? "con un padre di arrivo la quantità va scritta`},
		"/thread/{id}/fascicolo/conferma": {`if (p.getAll("voce").length !== 1) return "«Conferma il documento» conferma esattamente una voce";`,
			`const v = presente(p, ["struttura", "strutturale", "firma", "conferma", "selezione"]);`, `return v ? "«Conferma il documento» non manda`},
		"/proposta/{u}/scarta": {`(p.has("rumore") ? "lo scarto del file dalla Distinta nuova non segna il rumore" : "")`},
		// WP-U, notte 14/10: correggi_codice solo con correzione_confermata (Usa come STEP del prodotto, Q-P3-03 [T])
		"/thread/{id}/fascicolo/assegna": {
			`const confermata = p.has("correggi_codice") && !unico(p, "correggi_codice", false) && !unico(p, "correzione_confermata", false);`,
			`const v = (confermata ? "" : presente(p, ["correggi_codice", "correzione_confermata"])) || conPrefisso(p, "nuovo_riferimento");`,
			`return v ? "l'associazione dalla Distinta nuova non manda`},
		"/thread/{id}/fascicolo/documento/{u}/sostituisci": {`const v = conPrefisso(p, "nuovo_riferimento");`, `return v ? "«Sostituisci» dalla Distinta nuova non manda`},
		"/thread/{id}/fascicolo/prepara":                   {`(p.has("auto") ? "“Prepara i file” è solo il pulsante (DB7-6)" : "")`},
	}
	chiavi := regexp.MustCompile(`(?m)^  "([^"]+)": \(p\) =>`).FindAllStringSubmatch(blocco, -1)
	if len(chiavi) != len(attese) {
		t.Errorf("GUARDIE ha %d voci, attese %d", len(chiavi), len(attese))
	}
	nella := map[string]bool{}
	for _, r := range ponteElenco(t, "ROTTE") {
		nella[r] = true
	}
	for i, k := range chiavi {
		rotta := k[1]
		if !nella[rotta] {
			t.Errorf("la guardia di %q non è di una rotta della lista", rotta)
		}
		voce := blocco[strings.Index(blocco, k[0]):]
		if i+1 < len(chiavi) {
			voce = voce[:strings.Index(voce, chiavi[i+1][0])]
		}
		pezzi, ok := attese[rotta]
		if !ok {
			t.Errorf("guardia non attesa: %q", rotta)
			continue
		}
		for _, p := range pezzi {
			if !strings.Contains(voce, p) {
				t.Errorf("la guardia di %q non contiene %q", rotta, p)
			}
		}
	}
	codice := ponteCodice(t)
	// unico: il primo valore come FormValue, i ripetuti rifiutati
	u := ponteFunzione(t, codice, "function unico(p, n, ancheVuoto) {")
	for _, v := range []string{`const v = p.getAll(n);`, `if (v.length === 0) return "manca «" + n + "»";`, `if (v.length > 1) return "«" + n + "» è ripetuto";`,
		`if (!ancheVuoto && v[0].trim() === "") return "«" + n + "» è vuoto";`, `return "";`} {
		if !strings.Contains(u, v) {
			t.Errorf("unico: manca %q", v)
		}
	}
	if strings.Contains(codice, "function manca(") || strings.Contains(codice, ".every(") {
		t.Error("la vecchia guardia «tutti i valori vuoti» (manca, every) non deve tornare: si guarda il primo valore")
	}
	esegui := ponteFunzione(t, codice, "export async function esegui(")
	g, inv := strings.Index(esegui, "const guardia = GUARDIE[a.rotta];"), strings.Index(esegui, "postHTTP(a.url.pathname, p)")
	if g < 0 || inv < 0 || g > inv || !strings.Contains(esegui, `if (motivo) return finisci("errore", "Il gesto non è partito: " + motivo + ".");`) ||
		!strings.Contains(esegui, `const motivo = guardia ? guardia(p) : "";`) {
		t.Error("esegui non applica la guardia della rotta prima dell'invio")
	}
}

// TestPonteNessunaQuantitaImplicita: «qta» compare solo nelle guardie, che la chiedono scritta; il ponte non mette mai
// un valore di quantità (né 1 né altro) e non trasforma tipi.
func TestPonteNessunaQuantitaImplicita(t *testing.T) {
	codice := ponteCodice(t)
	fuori := ponteSenzaCommenti(strings.Replace(ponteSorgente(t), ponteBlocco(t, "GUARDIE"), "", 1))
	if strings.Contains(fuori, "qta") {
		t.Error("«qta» compare fuori dalle guardie: il ponte non mette quantità da sé")
	}
	for _, v := range []string{`set("qta"`, `append("qta"`, `set("tipo"`, `append("tipo"`, `set("firma"`, `set("voce"`,
		`set("padre"`, `set("da"`, `set("a"`} {
		if strings.Contains(codice, v) {
			t.Errorf("il ponte scrive un campo del gesto da sé: %q", v)
		}
	}
	// l'unico campo che il ponte aggiunge è ritorna_thread delle rotte /proposta/
	if n := len(regexp.MustCompile(`p\.(set|append)\("`).FindAllString(codice, -1)); n != 1 {
		t.Errorf("il ponte aggiunge %d campi da sé, atteso solo ritorna_thread", n)
	}
}
