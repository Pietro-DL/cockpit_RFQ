//go:build integrazione

// L4 — scelta 6 dello Smistamento (confermata dall'utente il 27/09): un prodotto tolto dalla working BOM non
// rinasce all'aggancio di una mail successiva. Il gesto del triage (la creazione della RFQ, l'aggancio) crea
// soltanto i componenti dei codici confermati in quello stesso gesto, sulla porta HTTP vera.

package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestUnProdottoToltoNonRinasceAllAggancio (scelta 6): la RFQ nasce con 7120001 confermato, e il prodotto
// nasce come prima. 7120001 si toglie dalla BOM (cancellato: non aveva storia). L'aggancio di una seconda mail
// che cita 7120001 ma non conferma codici non lo fa rinascere, e nel form nessun codice nasce spuntato;
// l'aggancio di una terza mail che conferma 7120002 fa nascere soltanto 7120002. Chi conferma di nuovo 7120001
// all'aggancio lo rimette: e' una decisione su quel codice.
func TestUnProdottoToltoNonRinasceAllAggancio(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	w := operatore(b)
	prodotti := func(thread uuid.UUID) string {
		t.Helper()
		var s string
		if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(string_agg(codice, ',' ORDER BY codice), '') FROM componente WHERE thread_id = $1`, thread).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// la creazione della RFQ con il codice confermato fa il prodotto, come prima
	m1 := b.mailConAllegati("Richiesta di offerta 7120001", "Offerta per il supporto 7120001, 200 pezzi.")
	_, html := w.fai(http.MethodPost, "/messaggio/"+m1.String()+"/rfq", url.Values{"cliente_id": {acme.String()}, "oggetto": {"Supporto 7120001"},
		"codice": {"7120001"}, "priorita": {"1"}}, true)
	if a := leggibile(html); !strings.Contains(a, "RFQ creata") || !strings.Contains(a, "7120001 nella BOM come prodotto finito") {
		t.Fatalf("creazione: %q", estrai(html, "avviso"))
	}
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, m1).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if got := prodotti(thread); got != "7120001" {
		t.Fatalf("prodotti dopo la creazione: %q", got)
	}
	origine := ""
	if err := b.pool.QueryRow(b.ctx, `SELECT origine::text FROM identificativo_thread WHERE thread_id = $1 AND codice = '7120001'`, thread).Scan(&origine); err != nil ||
		!strings.HasPrefix(origine, "proposta_") {
		t.Fatalf("7120001 confermato dalla casella del candidato: %q %v", origine, err)
	}

	// il prodotto si toglie dalla BOM: non ha storia, si cancella
	cid := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '7120001'`, thread)
	base := "/thread/" + thread.String() + "/fascicolo"
	if _, h := w.daFascicolo(http.MethodPost, base+"/componente/"+cid.String()+"/rimuovi", url.Values{}, thread, ""); !strings.Contains(avvisoF(h), "7120001 tolto") {
		t.Fatalf("togliere 7120001: %q", avvisoF(h))
	}

	// una seconda mail che cita 7120001: nel form dell'aggancio i codici ci sono, nessuno spuntato
	m2 := b.mailConAllegati("RE: Richiesta di offerta 7120001", "Confermiamo la quantita' per 7120001 e aggiungiamo 7120002.")
	_, form := w.fai(http.MethodGet, "/messaggio/"+m2.String()+"/triage?azione=aggancia", nil, true)
	for _, codice := range []string{"7120001", "7120002"} {
		if !strings.Contains(form, `name="codice" value="`+codice+`"`) {
			t.Fatalf("il form dell'aggancio offre %s fra i codici", codice)
		}
	}
	caselle := regexp.MustCompile(`<input type="checkbox" name="codice"[^>]*>`).FindAllString(form, -1)
	if len(caselle) < 2 {
		t.Fatalf("le caselle dei codici nel form dell'aggancio: %v", caselle)
	}
	for _, c := range caselle {
		if strings.Contains(c, "checked") {
			t.Errorf("nel form dell'aggancio un codice nasce spuntato: %s", c)
		}
	}
	// l'aggancio senza codici confermati: 7120001 non rinasce
	_, html = w.fai(http.MethodPost, "/messaggio/"+m2.String()+"/aggancia", url.Values{"thread_id": {thread.String()}}, true)
	if a := leggibile(html); !strings.Contains(a, "Agganciato") || strings.Contains(a, "nella BOM come prodott") {
		t.Fatalf("aggancio senza codici: %q", estrai(html, "avviso"))
	}
	if got := prodotti(thread); got != "" {
		t.Errorf("l'aggancio di una mail che non conferma codici ha fatto nascere %q", got)
	}

	// una terza mail: l'aggancio conferma 7120002, e nasce soltanto 7120002
	m3 := b.mailConAllegati("RE: Richiesta di offerta 7120001", "Aggiungiamo 7120002; 7120001 resta come da mail precedente.")
	_, html = w.fai(http.MethodPost, "/messaggio/"+m3.String()+"/aggancia", url.Values{"thread_id": {thread.String()}, "codice": {"7120002"}}, true)
	if a := leggibile(html); !strings.Contains(a, "7120002 nella BOM come prodotto finito") || strings.Contains(a, "7120001 nella BOM") {
		t.Fatalf("aggancio che conferma 7120002: %q", estrai(html, "avviso"))
	}
	if got := prodotti(thread); got != "7120002" {
		t.Errorf("prodotti dopo l'aggancio che conferma 7120002: %q, atteso solo 7120002", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM identificativo_thread WHERE thread_id = $1 AND codice = '7120002' AND confermato_da IS NOT NULL`, thread); n != 1 {
		t.Errorf("7120002 entra fra i codici della richiesta, confermato: %d", n)
	}

	// chi scrive di nuovo 7120001 all'aggancio lo rimette
	m4 := b.mailConAllegati("RE: Richiesta di offerta 7120001", "Ci ripensiamo: anche 7120001.")
	_, html = w.fai(http.MethodPost, "/messaggio/"+m4.String()+"/aggancia", url.Values{"thread_id": {thread.String()}, "identificativi": {"7120001"}}, true)
	if a := leggibile(html); !strings.Contains(a, "7120001 nella BOM come prodotto finito") {
		t.Errorf("confermato di nuovo, 7120001 rientra: %q", estrai(html, "avviso"))
	}
	if got := prodotti(thread); got != "7120001,7120002" {
		t.Errorf("prodotti alla fine: %q", got)
	}
}
