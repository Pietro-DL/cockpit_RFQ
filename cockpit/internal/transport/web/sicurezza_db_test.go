//go:build integrazione

// L4 — i valori dell'indirizzo che la pagina rimette in un hx-get o in un hx-vals, e i byte di un file
// che non devono entrare nella pagina (revisione di sicurezza del 25/09, H1).
//
// Il difetto: `sel` dell'Inbox finiva com'era in hx-get="/messaggio/{{sel}}" con hx-trigger="load", e
// «?sel=../allegato/<id>/anteprima» faceva chiedere alla schermata i byte di un allegato e li innestava
// nel pannello. Un «PDF» caricato da fuori puo' cominciare per %PDF- e contenere HTML con uno script.
// Tre difese, e qui si provano le due dal lato del server: nella pagina entra solo un uuid, e
// l'anteprima rifiuta le richieste htmx. La terza (htmx:beforeSwap nel layout) e' in statici_test.go
// (c'è) e nella prova I di e2e/inbox_quadranti.py (funziona nel browser).
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNellInboxEntraSoloUnSelCheEUnMessaggio(t *testing.T) {
	b := preparaBancoWeb(t)
	w := operatore(b)
	allegato := uuid.NewString()
	for _, cattivo := range []string{
		"../allegato/" + allegato + "/anteprima",
		"..%2fallegato%2f" + allegato + "%2fanteprima",
		`x","filtro":"tutti`,
	} {
		resp, pagina := w.fai(http.MethodGet, "/inbox?q=tutti&sel="+url.QueryEscape(cattivo), nil, false)
		if resp.StatusCode != 200 {
			t.Fatalf("sel=%q: %d", cattivo, resp.StatusCode)
		}
		if strings.Contains(pagina, allegato) || strings.Contains(pagina, "allegato/") {
			t.Errorf("sel=%q: il valore e' finito nella pagina:\n%s", cattivo, estratto(pagina, allegato))
		}
		if strings.Contains(pagina, `hx-trigger="load"`) {
			t.Errorf("sel=%q: il pannello chiede da solo un indirizzo all'apertura", cattivo)
		}
		if strings.Contains(leggibile(pagina), `"filtro":"tutti"`) {
			t.Errorf("sel=%q: il valore ha aggiunto parametri all'hx-vals della casella", cattivo)
		}
		if !strings.Contains(pagina, `"sel":""`) {
			t.Errorf("sel=%q: l'hx-vals della casella non porta una selezione vuota", cattivo)
		}
	}

	// un messaggio vero resta scelto: il pannello si apre su di lui
	msg := b.messaggioIn("SEL-VALIDO", b.francesco)
	_, pagina := w.fai(http.MethodGet, "/inbox?q=tutti&filtro=tutti&sel="+msg.String(), nil, false)
	if !strings.Contains(pagina, `hx-get="/messaggio/`+msg.String()+`" hx-trigger="load"`) {
		t.Errorf("con un uuid il pannello non si apre sul messaggio:\n%s", estratto(pagina, `id="pannello"`))
	}
	// e lo stesso uuid scritto in maiuscolo torna nella forma canonica
	_, pagina = w.fai(http.MethodGet, "/inbox?q=tutti&filtro=tutti&sel="+strings.ToUpper(msg.String()), nil, false)
	if !strings.Contains(pagina, `hx-get="/messaggio/`+msg.String()+`"`) {
		t.Errorf("un uuid in maiuscolo non torna nella forma canonica")
	}
}

// Il poll della coda job e degli scarti rimette nell'indirizzo lo stato e l'origine: ci torna solo un
// valore riconosciuto, mai il testo dell'indirizzo.
func TestIlPollDellAdminNonRiportaIlTestoDellIndirizzo(t *testing.T) {
	b := preparaBancoWeb(t)
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	casi := []struct{ percorso, cattivo, buono, atteso string }{
		{"/admin/job?stato=", `pronto&stato=fatto`, "pronto", `hx-get="/admin/job?stato=pronto"`},
		{"/admin/scarti?origine=", `ingest"x`, "ingest", `hx-get="/admin/scarti?origine=ingest"`},
	}
	for _, c := range casi {
		_, pagina := ad.fai(http.MethodGet, c.percorso+url.QueryEscape(c.cattivo), nil, false)
		vuoto := `hx-get="` + c.percorso + `"`
		if !strings.Contains(pagina, vuoto) {
			t.Errorf("%s%q: il poll non riparte da un valore vuoto:\n%s", c.percorso, c.cattivo, estratto(pagina, "hx-get"))
		}
		_, pagina = ad.fai(http.MethodGet, c.percorso+c.buono, nil, false)
		if !strings.Contains(pagina, c.atteso) {
			t.Errorf("%s%s: il poll non tiene il valore riconosciuto:\n%s", c.percorso, c.buono, estratto(pagina, "hx-get"))
		}
	}
}

// L'anteprima e' un documento: l'iframe e pdf.js la aprono, una richiesta htmx no. Con HX-Request la
// rotta risponde 400 senza servire un byte; la stessa richiesta senza e' servita come sempre.
func TestLAnteprimaRifiutaLeRichiesteHtmx(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(64*1024), true, false)
	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), map[string]string{"HX-Request": "true"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("anteprima chiesta da htmx: %d, attesa 400", resp.StatusCode)
	}
	if strings.HasPrefix(string(corpo), "%PDF-") || strings.HasPrefix(resp.Header.Get("Content-Type"), "application/pdf") {
		t.Fatal("anteprima chiesta da htmx: la risposta porta il PDF")
	}
	resp, corpo = s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 || !strings.HasPrefix(string(corpo), "%PDF-") {
		t.Fatalf("anteprima dall'iframe: %d", resp.StatusCode)
	}
}
