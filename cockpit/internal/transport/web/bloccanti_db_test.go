//go:build integrazione

// L4 — Smistamento G (A5.4.8, U2): i contatori dei bloccanti sulle schermate sono il conteggio logico del gate
// (ContaBloccantiLogici), non v_cruscotto.n_bloccanti. F6 aveva diviso il gate, ma tre posti contavano ancora le
// copie sul NAS come bloccanti: la testata e la Completezza del Fascicolo, la pagina della RFQ, la card e il
// filtro «solo bloccanti» di Richieste. Una copia in errore sul NAS resta visibile come materializzazione (la
// barra «Sul NAS», e il cassetto Avvisi del Fascicolo, che la elencava prima di F6 e la elenca di nuovo).
package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// Una RFQ con un solo documento, il 2D (requisito bloccante) di 7120001, in errore sul NAS: 0 bloccanti in
// testata del Fascicolo, nella Completezza, nella pagina della RFQ e nella card di Richieste, che non passa il
// filtro «solo bloccanti»; la copia in errore e' nella barra «Sul NAS» e nel cassetto Avvisi. La vista di prima
// la contava (1). Poi la stessa RFQ con un requisito bloccante che manca davvero (7120010, senza il 2D): 1
// bloccante ovunque, e la card passa il filtro.
func TestIBloccantiDelleSchermateSonoQuelliDelGate(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "BLOCG")
	comp := r.componente("7120001")
	r.documento("7120001_2D.pdf", "7120001", comp, db.StatoNasErrore)
	w := operatore(b)
	rfq := "/thread/" + r.thread.String()
	vista := func() int64 {
		var n int64
		if err := b.pool.QueryRow(b.ctx, `SELECT n_bloccanti FROM v_thread_bloccanti WHERE thread_id = $1`, r.thread).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := vista(); n != 1 {
		t.Fatalf("v_thread_bloccanti.n_bloccanti = %d: la prova vuole il 2D in errore contato dalla vista di prima", n)
	}
	testata := regexp.MustCompile(`<span class="k">\d+ component[ei] · \d+ file(.*?)</span>`)
	linguetta := regexp.MustCompile(`(?s)data-vista="completezza">Completezza(.*?)</a>`)
	perCongelare := regexp.MustCompile(`class="avviso-riga urg">Per congelare: ([^<]*)<`)

	guarda := func(passo string, n int) {
		t.Helper()
		frase := map[int]string{0: "", 1: "1 bloccante"}[n]
		_, fasc := w.fai(http.MethodGet, r.base(), nil, false)
		if m := testata.FindStringSubmatch(fasc); m == nil {
			t.Fatalf("%s: la testata del Fascicolo non si trova", passo)
		} else if got := m[1]; (n == 0 && strings.Contains(got, "bloccant")) || (n > 0 && !strings.Contains(got, `<b class="urg-t">`+frase+`</b>`)) {
			t.Errorf("%s: la testata del Fascicolo dice %q, attesi %d bloccanti", passo, got, n)
		}
		if m := linguetta.FindStringSubmatch(fasc); m == nil {
			t.Fatalf("%s: la linguetta Completezza non si trova", passo)
		} else if got := m[1]; (n == 0 && got != "") || (n > 0 && got != ` <small class="urg-t">1</small>`) {
			t.Errorf("%s: la linguetta Completezza dice %q, attesi %d bloccanti", passo, got, n)
		}
		_, compl := w.fai(http.MethodGet, r.base()+"?vista=completezza", nil, false)
		atteso := `Bloccanti: <b>0</b>`
		if n > 0 {
			atteso = `Bloccanti: <b class="urg-t">1</b>`
		}
		haTesto(t, passo+": Completezza", compl, atteso)

		_, pagina := w.fai(http.MethodGet, rfq, nil, false)
		haTesto(t, passo+": pagina della RFQ", pagina, `id="rfq-bloccanti"`, map[int]string{0: `>0</span>`, 1: `>1</span>`}[n],
			map[int]string{0: `<b id="rfq-fascicolo-bloccanti">0</b> bloccanti`, 1: `<b id="rfq-fascicolo-bloccanti">1</b> bloccante`}[n])

		_, richieste := w.fai(http.MethodGet, "/richieste", nil, false)
		inizio := strings.Index(richieste, `href="`+rfq+`"`)
		if inizio < 0 {
			t.Fatalf("%s: la card della RFQ non c'e' in Richieste", passo)
		}
		fine := strings.Index(richieste[inizio:], "</article>")
		segnali := richieste[inizio : inizio+fine]
		if (n == 0 && strings.Contains(segnali, "rq-segnale bloc")) || (n > 0 && !strings.Contains(segnali, `<span class="rq-segnale bloc">⚠ `+frase+`</span>`)) {
			t.Errorf("%s: la card di Richieste: %q, attesi %d bloccanti", passo, segnali, n)
		}
		_, filtrate := w.fai(http.MethodGet, "/richieste?bloccanti=1", nil, false)
		if dentro := strings.Contains(filtrate, `href="`+rfq+`"`); dentro != (n > 0) {
			t.Errorf("%s: con il filtro «solo bloccanti» la RFQ c'e': %v, attesi %d bloccanti", passo, dentro, n)
		}

		// la copia in errore resta visibile, come materializzazione: la barra e il cassetto Avvisi
		haTesto(t, passo+": barra «Sul NAS»", fasc, `id="sul-nas-barra"`, "1 documento in errore sul NAS")
		haTesto(t, passo+": pagina della RFQ, «Sul NAS»", pagina, `<b id="sul-nas"`, "1 documento in errore sul NAS")
		_, avvisi := w.fai(http.MethodGet, r.base()+"?cassetto=avvisi", nil, false)
		haTesto(t, passo+": cassetto Avvisi", avvisi, `class="avviso-riga warn">Sul NAS: 1 documento in errore. Non ferma il congelamento`)
		for _, m := range perCongelare.FindAllStringSubmatch(avvisi, -1) {
			if strings.Contains(m[1], "NAS") {
				t.Errorf("%s: il NAS e' fra i motivi che fermano il congelamento: %q", passo, m[1])
			}
		}
		if got := len(perCongelare.FindAllString(avvisi, -1)); got != n {
			t.Errorf("%s: %d motivi «Per congelare» nel cassetto, attesi %d", passo, got, n)
		}
	}

	guarda("il solo 2D in errore sul NAS", 0)

	// un requisito bloccante che manca davvero: 7120010 senza il 2D
	r.componente("7120010")
	if n := vista(); n != 2 {
		t.Fatalf("v_thread_bloccanti.n_bloccanti = %d, attesi 2 (la copia in errore e il 2D che manca)", n)
	}
	guarda("un requisito bloccante che manca", 1)
}
