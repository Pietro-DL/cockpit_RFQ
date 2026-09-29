//go:build integrazione

// L4 — l'albero proposto della Distinta e il riepilogo della bozza nel web (giro 4, fase 4.4a.1a), contro PostgreSQL
// vero: le due GET e la POST del riepilogo (una LETTURA con la bozza nel corpo, dichiarata cosi' in distinta_albero.go)
// rispondono in JSON, senza cache, per chi lavora; chi consulta legge le GET e riceve 403 sulla POST; e niente scrive:
// ogni tabella dello schema ha le stesse righe con lo stesso contenuto prima e dopo (la prova 98, estesa). Poi la
// controprova: un gesto vero («scarta il nodo») cambia il database, e l'albero lo mostra.

package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
)

// scenaAlberoWeb e' una RFQ ACME del prodotto 7120001 (il codice della richiesta confermato e il suo finito) con lo STEP
// del prodotto (7120010, 7120011 ×2; 7120011 → 7121003) e quello di 7120010 (7121003 ×2 e una vite normata ×4), le
// righe fatte nascere da «Rianalizza», il gesto di una persona.
func (b *bancoWeb) scenaAlberoWeb(t *testing.T, chiave string) *rfqFascicolo {
	t.Helper()
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	r.esegui(`UPDATE cliente SET regole = $1 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $2)`, famiglieDistintaDB, r.thread)
	r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '7120001', 'proposta_famiglia', $2)`,
		r.thread, r.utente)
	r.componenteTipo("7120001", "finito")
	r.stepNellaRfq("7120001.stp", fattiDi("#1", []string{"#1=7120001", "#2=7120010", "#3=7120011", "#4=7121003"},
		[]string{"#1>#2*1", "#1>#3*2", "#3>#4*1"}), an)
	r.stepNellaRfq("7120010.stp", fattiDi("#1", []string{"#1=7120010", "#2=7121003", "#3=7121007_VITE TCEI ISO 4762 M6X16"},
		[]string{"#1>#2*2", "#1>#3*4"}), an)
	w := operatore(b)
	if resp, html := w.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, ""); resp.StatusCode != 200 {
		t.Fatalf("rianalizza: %d %s", resp.StatusCode, avvisoF(html))
	}
	if n := r.conta(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1`, r.thread); n != 7 {
		t.Fatalf("le righe degli STEP: %d, attese 7", n)
	}
	return r
}

// chiediJSON fa una richiesta con un corpo JSON (se c'e') e restituisce la risposta e il corpo.
func (w *browser) chiediJSON(metodo, percorso, corpo string) (*http.Response, string) {
	w.b.t.Helper()
	var in io.Reader
	if corpo != "" {
		in = strings.NewReader(corpo)
	}
	req, _ := http.NewRequest(metodo, w.b.srv.URL+percorso, in)
	if corpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(req)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return resp, string(testo)
}

// Prova (fase 4.4a.1a, prova 98 estesa): l'albero proposto e il riepilogo si leggono e non scrivono. L'operatore legge
// l'albero (i due STEP, 7121003 con due padri, la proposta commerciale della vite normata), il riepilogo della bozza
// vuota (non confermabile: la proposta commerciale e' aperta), quello della sua bozza con il ✓ (confermabile), e una
// bozza che non si legge (422, con il rifiuto). Chi consulta legge l'albero e il riepilogo della bozza vuota, e riceve
// 403 sulla POST. Ogni tabella dello schema e' uguale prima e dopo. La controprova: «scarta il nodo» scrive, e l'albero
// mostra il nodo scartato.
func TestAlberoPropostoNelWebNonScrive(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.scenaAlberoWeb(t, "ALB44")
	base := "/thread/" + r.thread.String() + "/distinta/albero"
	op := operatore(b)
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	prima := fotoDelDatabase(t, b)

	resp, corpo := op.chiediJSON(http.MethodGet, base, "")
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("l'albero: %d, %q, %q", resp.StatusCode, resp.Header.Get("Cache-Control"), resp.Header.Get("Content-Type"))
	}
	var a fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &a); err != nil {
		t.Fatal(err)
	}
	if len(a.Prodotti) != 1 || a.Prodotti[0].Codice != "7120001" || a.Prodotti[0].Ancora != fascicolo.LivelloPiena || a.Firma == "" {
		t.Errorf("il prodotto: %+v", a.Prodotti)
	}
	n, ok := a.Nodo("cod:7121003")
	if !ok || strings.Join(n.Padri, " ") != "cod:7120010 cod:7120011" {
		t.Errorf("7121003: %+v", n)
	}
	vite, ok := a.Nodo("cod:7121007")
	if !ok || vite.Commerciale == nil || vite.Commerciale.Motivo != "normato: ISO 4762 · M6X16 · TCEI" || vite.Tipo != "sciolto" {
		t.Errorf("la vite: %+v", vite)
	}

	resp, corpo = op.chiediJSON(http.MethodGet, base+"/riepilogo", "")
	var vuota fascicolo.RiepilogoAlbero
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || json.Unmarshal([]byte(corpo), &vuota) != nil {
		t.Fatalf("il riepilogo della bozza vuota: %d %s", resp.StatusCode, corpo)
	}
	if vuota.Confermabile || len(vuota.Commerciali) != 1 || vuota.Commerciali[0].Stato != fascicolo.CommercialeAperta ||
		vuota.AlberoFirma != a.Firma || len(vuota.Nuovi) != 4 {
		t.Errorf("la bozza vuota: confermabile %v, commerciali %+v, nuovi %d", vuota.Confermabile, vuota.Commerciali, len(vuota.Nuovi))
	}

	bozza := `{"formato": 1, "base": "` + a.Firma + `", "commerciali": [{"nodo": "cod:7121007", "risposta": "si"}]}`
	resp, corpo = op.chiediJSON(http.MethodPost, base+"/riepilogo", bozza)
	var conBozza fascicolo.RiepilogoAlbero
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || json.Unmarshal([]byte(corpo), &conBozza) != nil {
		t.Fatalf("il riepilogo della bozza: %d %s", resp.StatusCode, corpo)
	}
	if !conBozza.Confermabile || conBozza.Commerciali[0].Stato != fascicolo.CommercialeSi || conBozza.Firma == vuota.Firma {
		t.Errorf("con il ✓: confermabile %v, blocchi %v, %+v", conBozza.Confermabile, conBozza.Blocchi, conBozza.Commerciali)
	}
	for _, c := range []struct{ corpo, frase string }{
		{`{"formato": 1, "tolti": [{"nodo": "cod:7129999"}]}`, "non è nell'albero di adesso"},
		{`{"formato": 1, "tolti": [{"nodo": "cod:7120001"}]}`, "non si toglie dall'albero"},
		{`{"formato": 7}`, "formato 7"},
		{`non è json`, "non si legge"},
	} {
		resp, corpo = op.chiediJSON(http.MethodPost, base+"/riepilogo", c.corpo)
		var e struct{ Errore string }
		if resp.StatusCode != http.StatusUnprocessableEntity || json.Unmarshal([]byte(corpo), &e) != nil || !strings.Contains(e.Errore, c.frase) {
			t.Errorf("la bozza %s: %d %s", c.corpo, resp.StatusCode, corpo)
		}
	}

	for _, p := range []string{base, base + "/riepilogo"} {
		if resp, _ := co.chiediJSON(http.MethodGet, p, ""); resp.StatusCode != 200 {
			t.Errorf("chi consulta, GET %s: %d", p, resp.StatusCode)
		}
	}
	if resp, _ := co.chiediJSON(http.MethodPost, base+"/riepilogo", bozza); resp.StatusCode != http.StatusForbidden {
		t.Errorf("chi consulta, POST del riepilogo: %d, atteso 403", resp.StatusCode)
	}
	if resp, _ := op.chiediJSON(http.MethodGet, "/thread/"+uuid.NewString()+"/distinta/albero", ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("una RFQ che non c'e': %d", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("l'albero e il riepilogo hanno scritto in: %s", d)
	}

	// la controprova: un gesto che scrive cambia il database, e l'albero lo mostra
	pid := uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND codice = '7121007'`, r.thread)
	if resp, html := op.daFascicolo(http.MethodPost, r.base()+"/nodo/"+pid.String()+"/scarta", url.Values{}, r.thread, ""); resp.StatusCode != 200 {
		t.Fatalf("scarta: %d %s", resp.StatusCode, avvisoF(html))
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); !strings.Contains(d, "componente_proposta") {
		t.Fatalf("lo scarto doveva scrivere (tabelle cambiate: %q): la prova non proverebbe niente", d)
	}
	_, corpo = op.chiediJSON(http.MethodGet, base, "")
	var dopo fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &dopo); err != nil {
		t.Fatal(err)
	}
	if v, ok := dopo.Nodo("cod:7121007"); !ok || v.Stato != fascicolo.StatoAlberoScartato || v.Commerciale != nil || dopo.Firma == a.Firma {
		t.Errorf("dopo lo scarto: %+v, firma cambiata %v", v, dopo.Firma != a.Firma)
	}
}
