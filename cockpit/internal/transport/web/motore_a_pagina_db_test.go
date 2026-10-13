//go:build integrazione

// L4 — giro 5, A1d-P4, passo B5.1 (R-05 della revisione di B5.1): la pagina della Distinta nuova, GET
// /thread/{id}/motore-a, sul database di prova (COCKPIT_TEST_DSN, cockpit_test, con il semaforo di preparaBancoWeb), con le
// sessioni vere e la scena B87 di mwScena (motore_a_db_test.go), a flag acceso con le regole ACME del suo cliente.
//
// Sono le righe della pagina che la PR #18 di Stefano Valo (153117d) metteva in TestNessunaGetScrive
// (nessunaget_db_test.go): la pagina nelle tre viste, con un prodotto che non c'è, il frammento. Quel file è comune e
// fuori dal perimetro della corsia E: qui stanno in un file della pagina, con la stessa foto del database prima e dopo
// (fotoDelDatabase, differenze). In più la rilettura come la fa il ponte (HX-Target "ma" e HX-Current-URL, R-01) e HEAD.
//   - amministratore: 200, text/html, no-store, con la pagina o il frammento della RFQ;
//   - operatore e consultazione: 403 (la pagina è solo dell'amministratore);
//   - per nessuno una riga cambiata, nessun job accodato.
// Dopo la direttiva 12 (R-01, R-02, D-18): la radice ha data-ma-calcolo (con la scena calcolata, mai «errore»); la
// rilettura del ponte prende la scelta dall'indirizzo (data-ma-sorgente="indirizzo"); un prodotto che non c'è si dice
// (#ma-prodotto-non-trovato).
//
// NON ESEGUITA in B5.1 (nessun DB nella corsia). Prima di lanciarla: il DSN di COCKPIT_TEST_DSN verificato (deve essere
// cockpit_test), come per ogni L4. Solo dati della scena B87 e ACME.

package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestMotoreAPaginaL4NessunaGetScrive(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MA410")
	mwAccendi(t, b, s.regole)
	pagina := "/thread/" + s.thread.String() + "/motore-a"
	get := []struct {
		percorso     string
		intestazioni map[string]string
		frammento    bool
	}{
		{pagina, nil, false},
		{pagina + "?vista=documenti", nil, false},
		{pagina + "?vista=rfq", nil, false},
		{pagina + "?prodotto=boh", nil, false},
		{pagina, map[string]string{"HX-Request": "true"}, true},
		// la rilettura del ponte: senza query, la vista dall'indirizzo che il browser mostra (R-01)
		{pagina, map[string]string{"HX-Request": "true", "HX-Target": "ma", "HX-Current-URL": b.srv.URL + pagina + "?vista=documenti&prodotto=boh"}, true},
	}
	prima := fotoDelDatabase(t, b)
	for _, chi := range []struct {
		nome  string
		w     *browser
		admin bool
	}{{"operatore", s.op, false}, {"consultazione", s.co, false}, {"amministratore", s.ad, true}} {
		for _, g := range get {
			for _, metodo := range []string{http.MethodGet, http.MethodHead} {
				resp, corpo := chi.w.chiedi(metodo, g.percorso, g.intestazioni)
				if !chi.admin {
					if resp.StatusCode != http.StatusForbidden {
						t.Errorf("%s, %s %s: %d, atteso 403", chi.nome, metodo, g.percorso, resp.StatusCode)
					}
					continue
				}
				if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" ||
					resp.Header.Get("Cache-Control") != "no-store" {
					t.Errorf("%s, %s %s: %d %q %q", chi.nome, metodo, g.percorso, resp.StatusCode, resp.Header.Get("Content-Type"),
						resp.Header.Get("Cache-Control"))
					continue
				}
				if metodo == http.MethodHead {
					continue
				}
				html := string(corpo)
				if !strings.Contains(html, `data-ma-pagina="`+s.thread.String()+`"`) {
					t.Errorf("%s, GET %s: senza la pagina della RFQ: %.300s", chi.nome, g.percorso, html)
				}
				if g.frammento == strings.Contains(html, "<html") {
					t.Errorf("%s, GET %s: frammento atteso %v", chi.nome, g.percorso, g.frammento)
				}
				if g.intestazioni["HX-Target"] == "ma" && (!strings.Contains(html, `data-ma-vista="documenti"`) || !strings.Contains(html, `data-ma-sorgente="indirizzo"`)) {
					t.Errorf("%s, GET %s: la rilettura del ponte non tiene la vista dell'indirizzo mostrato", chi.nome, g.percorso)
				}
				if strings.Contains(html, `data-ma-calcolo="errore"`) || !strings.Contains(html, `data-ma-calcolo="`) {
					t.Errorf("%s, GET %s: la scena calcolata senza data-ma-calcolo, o con l'errore: %.300s", chi.nome, g.percorso, html)
				}
				chiesto := strings.Contains(g.percorso, "prodotto=boh") || strings.Contains(g.intestazioni["HX-Current-URL"], "prodotto=boh")
				if chiesto != strings.Contains(html, `id="ma-prodotto-non-trovato"`) {
					t.Errorf("%s, GET %s: la striscia del prodotto che non c'è", chi.nome, g.percorso)
				}
			}
		}
		if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
			t.Errorf("le GET della pagina di %s hanno scritto in: %s", chi.nome, d)
		}
	}
}
