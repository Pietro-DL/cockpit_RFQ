//go:build integrazione

// L4 — Smistamento F6 (A5.4.8, U2): la barra «Sul NAS» con il database vero. I numeri vengono da
// StatoMaterializzazione, si vedono nella testata della RFQ e nel Fascicolo, e «Riprova copie» dal
// Fascicolo risponde come il Fascicolo. Il gate non li conta.
package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// Prova 198, parte schermata, contro il database: tre documenti decisi (scritto, in coda, in errore).
func TestLaBarraSulNasLeggeLaMaterializzazione(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "NAS198")
	comp := r.componente("7120001")
	r.documento("7120001_1.pdf", "7120001", comp, db.StatoNasScritto)
	r.documento("7120001_2.pdf", "7120001", comp, db.StatoNasInCoda)
	r.documento("7120001_3.pdf", "7120001", comp, db.StatoNasErrore)
	w := operatore(b)

	_, fasc := w.fai(http.MethodGet, r.base(), nil, false)
	haTesto(t, "Fascicolo", fasc, `<span class="chip urg" id="sul-nas"`, ">Sul NAS: 1 di 3</span>", `id="sul-nas-barra"`,
		"<b>Sul NAS: 1 di 3</b> scritti", "1 documento in coda per la copia sul NAS", "1 documento in errore sul NAS",
		`hx-post="/thread/`+r.thread.String()+`/riprova-copie"`, `id="gate-materializzazione"`)
	// il NAS non e' fra i motivi del gate (il primo <ul class="gate">, se c'e')
	if m := regexp.MustCompile(`(?s)<ul class="gate">(.*?)</ul>`).FindStringSubmatch(fasc); m != nil && strings.Contains(m[1], "NAS") {
		t.Errorf("il NAS e' fra i motivi del gate: %s", m[1])
	}

	_, rfq := w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false)
	haTesto(t, "pagina della RFQ", rfq, `<b id="sul-nas"`, ">Sul NAS: 1 di 3</b>", "1 documento in errore sul NAS")

	// «Riprova copie» dalla barra: la stessa rotta della pagina della RFQ, che risponde come il Fascicolo
	_, html := w.daFascicolo(http.MethodPost, "/thread/"+r.thread.String()+"/riprova-copie", url.Values{}, r.thread, "")
	if a := avvisoF(html); !strings.Contains(a, "1 copia rimessa in coda") {
		t.Errorf("avviso di «Riprova copie» dal Fascicolo: %q", a)
	}
	haTesto(t, "risposta di «Riprova copie»", html, `id="fasc-testata" hx-swap-oob="innerHTML"`, `id="sul-nas-barra"`)

	// le copie riescono: la voce diventa verde, la barra non ha piu' frasi ne' gesto
	if _, err := b.pool.Exec(b.ctx, `UPDATE documento SET stato_nas = 'scritto', errore_nas = NULL, scritto_il = now() WHERE thread_id = $1`, r.thread); err != nil {
		t.Fatal(err)
	}
	_, fasc = w.fai(http.MethodGet, r.base(), nil, false)
	haTesto(t, "dopo le copie", fasc, `<span class="chip ok" id="sul-nas"`, "<b>Sul NAS: 3 di 3</b> scritti")
	senzaTesto(t, "dopo le copie", fasc, "riprova-copie", "in coda per la copia", `id="gate-materializzazione"`)
}
