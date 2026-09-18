//go:build integrazione

// L4 — checkpoint 7B.5: dopo un seed, i messaggi già arrivati cambiano quadrante SUBITO.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/web/
//
// Il difetto che questi test tengono chiuso: l'import dei fornitori scriveva l'anagrafica e basta.
// Dalla 0014 la controparte è un fatto scritto sul messaggio all'ingest, non una domanda che
// l'Inbox rifà a ogni lettura; senza un ricalcolo, censire un fornitore e guardare l'Inbox dava la
// stessa schermata di prima, e per vedere il cambiamento bisognava riavviare il server e
// risincronizzare la posta — cioè aspettare che arrivasse una mail nuova per vedere sistemata una
// vecchia. Nomi e domini sono inventati (.example).
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
)

// L'import dei fornitori dalla schermata: tre messaggi dello stesso dominio sconosciuto, uno già
// deciso. Dopo l'import i due non decisi stanno in «Fornitori», il deciso non si muove, e non è
// arrivata nessuna mail nuova.
func TestB75ImportFornitoriRicalcolaIMessaggiGiaArrivati(t *testing.T) {
	b := preparaBancoWeb(t)
	altro := b.unCliente("Altro S.p.A.", "ALTRO", "altro.example")
	uno := b.posta("entrata", "acquisti@polver.example", "OFFERTA polver uno", true)
	due := b.posta("entrata", "info@polver.example", "OFFERTA polver due", false)
	deciso := b.posta("entrata", "x@polver.example", "OFFERTA polver tre, già decisa", false)

	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto)
		VALUES ($1, 'outlook', now(), 'decisa') RETURNING thread_id`, altro).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2, aggancio = 'operatore' WHERE messaggio_id = $1`, deciso, thread); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{uno, due, deciso} {
		if tipo, _ := b.controparteDi(id); tipo != "sconosciuto" {
			t.Fatalf("prima dell'import la controparte deve essere sconosciuta, non %s", tipo)
		}
	}

	ad := b.browser("10.0.0.5:51000")
	ad.login("AD", "prova-ad")
	seme := `{"fornitori":[{"ragione_sociale":"Polver","tipo":"verniciatore","domini":["polver.example"],
		"contatti":[{"nome":"Ufficio","email":"acquisti@polver.example"}]}]}`

	// prima l'anteprima: non deve scrivere niente, nemmeno il ricalcolo
	resp, corpo := ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"anteprima"}, "testo": {seme}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("anteprima: stato %d", resp.StatusCode)
	}
	if strings.Contains(corpo, "messaggi ricalcolati") {
		t.Errorf("l'anteprima ha ricalcolato i messaggi: deve solo guardare\n%s", primi400(corpo))
	}
	if tipo, _ := b.controparteDi(uno); tipo != "sconosciuto" {
		t.Fatalf("dopo la sola anteprima la controparte è già %s", tipo)
	}

	// poi l'import vero
	resp, corpo = ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"applica"}, "testo": {seme}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("import: stato %d", resp.StatusCode)
	}
	for _, atteso := range []string{"Seme applicato", "2 messaggi ricalcolati", "2 controparti cambiate"} {
		if !strings.Contains(corpo, atteso) {
			t.Errorf("l'import non dice %q:\n%s", atteso, primi400(corpo))
		}
	}

	for _, id := range []uuid.UUID{uno, due} {
		if tipo, _ := b.controparteDi(id); tipo != "fornitore" {
			t.Errorf("messaggio non deciso %s: controparte %s, atteso fornitore", id, tipo)
		}
	}
	if tipo, _ := b.controparteDi(deciso); tipo != "sconosciuto" {
		t.Errorf("il messaggio già deciso è stato toccato: controparte %s", tipo)
	}

	// e nell'Inbox, senza nessun sync in mezzo
	_, fornitori := ad.fai(http.MethodGet, "/inbox?q=fornitori&filtro=tutti", nil, true)
	for _, atteso := range []string{"OFFERTA polver uno", "OFFERTA polver due"} {
		if !strings.Contains(fornitori, atteso) {
			t.Errorf("il quadrante Fornitori non ha %q:\n%s", atteso, primi400(fornitori))
		}
	}
	_, validare := ad.fai(http.MethodGet, "/inbox?q=validare&filtro=tutti", nil, true)
	if strings.Contains(validare, "OFFERTA polver uno") {
		t.Errorf("il messaggio è rimasto anche in Da validare:\n%s", primi400(validare))
	}

	// un secondo import dello stesso file non scrive e non ricalcola niente di nuovo
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"applica"}, "testo": {seme}}, true)
	if !strings.Contains(corpo, "0 messaggi ricalcolati") {
		t.Errorf("il secondo import doveva non avere niente da riguardare:\n%s", primi400(corpo))
	}
}

// Il frammento dell'Inbox porta i COMANDI E LA LISTA insieme. È la prova a L4 di ciò che il browser
// verifica a L7: rispondere con la sola lista lascia sullo schermo le linguette di prima.
func TestB75IlFrammentoInboxPortaComandiELista(t *testing.T) {
	b := preparaBancoWeb(t)
	b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	b.unFornitore("Euroforesi", db.TipoFornitoreVerniciatore, "euroforesi.example", "cataforesi")
	b.posta("entrata", "acquisti@acme.example", "DEL CLIENTE", false)
	b.posta("entrata", "info@euroforesi.example", "DEL FORNITORE", false)
	b.posta("entrata", "chi@ignoto.example", "DI NESSUNO", false)

	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")

	for _, c := range []struct{ q, riga, acceso string }{
		{"fornitori", "DEL FORNITORE", `class="quadrante fornitori attivo"`},
		{"validare", "DI NESSUNO", `class="quadrante validare attivo"`},
		{"buyer", "DEL CLIENTE", `class="quadrante buyer attivo"`},
	} {
		_, frammento := fp.fai(http.MethodGet, "/inbox?q="+c.q+"&filtro=tutti", nil, true)
		if !strings.Contains(frammento, c.acceso) {
			t.Errorf("q=%s: il frammento non accende la linguetta giusta (%s):\n%s", c.q, c.acceso, primi400(frammento))
		}
		if !strings.Contains(frammento, c.riga) {
			t.Errorf("q=%s: il frammento non porta la lista (manca %q)", c.q, c.riga)
		}
		for _, indispensabile := range []string{`id="inbox-stato"`, `class="quadranti"`, `class="filtri"`, `id="lista"`} {
			if !strings.Contains(frammento, indispensabile) {
				t.Errorf("q=%s: il frammento non contiene %s: i comandi e la lista devono tornare insieme", c.q, indispensabile)
			}
		}
	}

	// e il frammento NON porta il pannello di destra: cambiare quadrante non chiude il messaggio
	_, frammento := fp.fai(http.MethodGet, "/inbox?q=buyer", nil, true)
	if strings.Contains(frammento, `id="pannello"`) {
		t.Errorf("il frammento dell'Inbox si porta dietro il pannello del messaggio:\n%s", primi400(frammento))
	}

	// il ritorno dalla cronologia del browser vuole la PAGINA, non il pezzo
	r, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/inbox?q=validare", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-History-Restore-Request", "true")
	r.Header.Set("X-Prova-IP", fp.ip)
	resp, err := fp.c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	corpo := make([]byte, 4096)
	n, _ := resp.Body.Read(corpo)
	if !strings.Contains(string(corpo[:n]), "<!doctype html>") {
		t.Errorf("il ritorno dalla cronologia ha avuto un frammento invece della pagina:\n%s", string(corpo[:n]))
	}
}
