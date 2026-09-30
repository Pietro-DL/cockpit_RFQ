//go:build integrazione

// L4 — «Conferma l'albero» nel web (giro 4, fase 4.4a.1b), contro PostgreSQL vero: la POST della conferma con la firma
// del riepilogo e la bozza; i rifiuti (firma vecchia, proposta commerciale aperta, un corpo che non si legge) rispondono
// 422 e non scrivono niente; chi consulta riceve 403; la conferma riuscita risponde con la frase, senza cache, e scrive
// i pezzi, il commerciale con il ✓ (confermato da chi conferma) e il segno sulle righe; le risposte d'errore delle
// rotte dell'albero hanno anche loro Cache-Control: no-store. «Riapri il nodo» ha la sua rotta POST. Dalla verifica
// della fase: una conferma che fallisce a meta' (un legame che chiude un ciclo) risponde 422 e non lascia niente.

package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// corpoConferma e' il corpo della POST della conferma.
func corpoConferma(t *testing.T, firma, bozza string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"firma": firma, "bozza": json.RawMessage(bozza)})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Prova (fase 4.4a.1b): la conferma dell'albero nel web. L'operatore legge l'albero e il riepilogo della sua bozza con il
// ✓ sulla vite; con la firma sbagliata, senza la risposta alla proposta commerciale o con un corpo che non si legge la
// conferma risponde 422 con il rifiuto, e nessuna tabella cambia; chi consulta riceve 403. Con la firma del riepilogo la
// conferma riesce: 200, no-store, la frase; nascono i pezzi, la vite particolare commerciale confermata da chi ha
// confermato, e le righe degli STEP hanno il segno. Dopo, l'albero e' tutto nella distinta.
func TestLaConfermaDellAlberoNelWeb(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.scenaAlberoWeb(t, "ALB45")
	base := "/thread/" + r.thread.String() + "/distinta/albero"
	op := operatore(b)
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")

	_, corpo := op.chiediJSON(http.MethodGet, base, "")
	var a fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &a); err != nil {
		t.Fatal(err)
	}
	bozza := `{"formato": 1, "base": "` + a.Firma + `", "commerciali": [{"nodo": "cod:7121007", "risposta": "si"}]}`
	_, corpo = op.chiediJSON(http.MethodPost, base+"/riepilogo", bozza)
	var rp fascicolo.RiepilogoAlbero
	if err := json.Unmarshal([]byte(corpo), &rp); err != nil || !rp.Confermabile {
		t.Fatalf("il riepilogo: %v %s", err, corpo)
	}

	prima := fotoDelDatabase(t, b)
	vuota := `{"formato": 1, "base": "` + a.Firma + `"}`
	_, corpoVuota := op.chiediJSON(http.MethodPost, base+"/riepilogo", vuota)
	var rv fascicolo.RiepilogoAlbero
	if err := json.Unmarshal([]byte(corpoVuota), &rv); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ nome, corpo, frase string }{
		{"firma sbagliata", corpoConferma(t, "abc", bozza), "rileggi il riepilogo"},
		{"proposta commerciale aperta", corpoConferma(t, rv.Firma, vuota), "1 proposta commerciale senza ✓ o ✗"},
		{"senza firma", `{"bozza": ` + bozza + `}`, "senza la firma del riepilogo"},
		{"non e' json", `non è json`, "non si legge"},
	} {
		resp, corpo := op.chiediJSON(http.MethodPost, base+"/conferma", c.corpo)
		var e struct{ Errore string }
		if resp.StatusCode != http.StatusUnprocessableEntity || resp.Header.Get("Cache-Control") != "no-store" ||
			json.Unmarshal([]byte(corpo), &e) != nil || !strings.Contains(e.Errore, c.frase) {
			t.Errorf("%s: %d %q %s", c.nome, resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
		}
	}
	if resp, _ := co.chiediJSON(http.MethodPost, base+"/conferma", corpoConferma(t, rp.Firma, bozza)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("chi consulta: %d, atteso 403", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("le conferme rifiutate hanno scritto in: %s", d)
	}

	resp, corpo := op.chiediJSON(http.MethodPost, base+"/conferma", corpoConferma(t, rp.Firma, bozza))
	var esito struct{ Testo string }
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" || json.Unmarshal([]byte(corpo), &esito) != nil ||
		!strings.Contains(esito.Testo, "Albero confermato: 4 pezzi nuovi (1 particolare commerciale confermato)") {
		t.Fatalf("la conferma: %d %q %s", resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
	}
	var working string
	if err := b.pool.QueryRow(b.ctx, `SELECT string_agg(c.codice || ':' || c.tipo || ':' || u.sigla, ' ' ORDER BY c.codice) FROM componente c
		JOIN utente u ON u.utente_id = c.confermato_da WHERE c.thread_id = $1 AND c.codice <> '7120001'`, r.thread).Scan(&working); err != nil {
		t.Fatal(err)
	}
	if working != "7120010:sottoassieme:FP 7120011:sottoassieme:FP 7121003:sciolto:FP 7121007:commerciale:FP" {
		t.Errorf("i pezzi nati: %s", working)
	}
	if n := r.conta(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND NOT (evidenza ? 'albero') AND chiave <> '#1'`, r.thread); n != 0 {
		t.Errorf("righe degli STEP (tranne le radici) senza il segno: %d", n)
	}
	_, corpo = op.chiediJSON(http.MethodGet, base, "")
	var dopo fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &dopo); err != nil {
		t.Fatal(err)
	}
	for _, n := range dopo.Nodi {
		if n.Stato != fascicolo.StatoAlberoNellaDistinta {
			t.Errorf("dopo la conferma %s e' %s", n.Chiave, n.Stato)
		}
	}
}

// Prova (studio § 2.8, «il ventesimo arco che chiude un ciclo annulla tutto»; fase 4.4a.1b, dalla verifica): la
// conferma che fallisce a meta', nel web. 7120010 e 7121003 ci sono gia', fuori dall'albero del prodotto, con l'arco
// 7121003 → 7120010. L'albero (che vede solo quello che si raggiunge dal prodotto) propone 7120010 → 7121003, e il
// riepilogo e' confermabile; la conferma fa nascere 7120011 e la vite, e al legame 7120010 → 7121003 collega trova il
// ciclo con la working intera. La POST risponde 422 con il rifiuto, senza cache, e nessuna tabella cambia: la rotta
// annulla la transazione, e il flusso di F8 dopo il gesto non parte.
func TestLaConfermaCheFallisceAMetaNonLasciaNienteNelWeb(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.scenaAlberoWeb(t, "ALB47")
	base := "/thread/" + r.thread.String() + "/distinta/albero"
	op := operatore(b)
	a10 := r.componenteTipo("7120010", "sottoassieme")
	p03 := r.componenteTipo("7121003", "sottoassieme")
	r.arco(p03, a10, 1)

	_, corpo := op.chiediJSON(http.MethodGet, base, "")
	var a fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &a); err != nil {
		t.Fatal(err)
	}
	bozza := `{"formato": 1, "base": "` + a.Firma + `", "commerciali": [{"nodo": "cod:7121007", "risposta": "si"}]}`
	_, corpo = op.chiediJSON(http.MethodPost, base+"/riepilogo", bozza)
	var rp fascicolo.RiepilogoAlbero
	if err := json.Unmarshal([]byte(corpo), &rp); err != nil || !rp.Confermabile || len(rp.Ritrovati) != 2 || len(rp.Nuovi) != 2 {
		t.Fatalf("il riepilogo: %v %s", err, corpo)
	}
	prima := fotoDelDatabase(t, b)
	resp, corpo := op.chiediJSON(http.MethodPost, base+"/conferma", corpoConferma(t, rp.Firma, bozza))
	var e struct{ Errore string }
	if resp.StatusCode != http.StatusUnprocessableEntity || resp.Header.Get("Cache-Control") != "no-store" ||
		json.Unmarshal([]byte(corpo), &e) != nil || !strings.Contains(e.Errore, "chiuderebbe un ciclo") {
		t.Errorf("la conferma che chiude un ciclo: %d %q %s", resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("la conferma fallita a meta' ha lasciato scritture in: %s", d)
	}
}

// Prova (fase 4.4a.1b, dalla verifica della 4.4a.1a): le risposte d'errore delle rotte dell'albero non vanno in nessuna
// cache, come quelle riuscite: un indirizzo con un id che non e' un uuid (400) e una RFQ che non c'e' (404 per le GET,
// il rifiuto 422 per la conferma), per la GET dell'albero, del riepilogo e per la conferma.
func TestLeRisposteDErroreDellAlberoSonoNoStore(t *testing.T) {
	b := preparaBancoWeb(t)
	op := operatore(b)
	for _, c := range []struct {
		metodo, percorso, corpo string
		stato                   int
	}{
		{http.MethodGet, "/thread/non-un-id/distinta/albero", "", http.StatusBadRequest},
		{http.MethodGet, "/thread/non-un-id/distinta/albero/riepilogo", "", http.StatusBadRequest},
		{http.MethodPost, "/thread/non-un-id/distinta/albero/conferma", `{}`, http.StatusBadRequest},
		{http.MethodGet, "/thread/" + uuid.NewString() + "/distinta/albero", "", http.StatusNotFound},
		{http.MethodGet, "/thread/" + uuid.NewString() + "/distinta/albero/riepilogo", "", http.StatusNotFound},
		// la conferma blocca la RFQ per prima cosa: una RFQ che non c'e' e' il suo rifiuto («RFQ non trovata»), un 422
		{http.MethodPost, "/thread/" + uuid.NewString() + "/distinta/albero/conferma", `{"firma": "f", "bozza": {"formato": 1}}`, http.StatusUnprocessableEntity},
	} {
		resp, corpo := op.chiediJSON(c.metodo, c.percorso, c.corpo)
		if resp.StatusCode != c.stato || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s %s: %d %q (%s)", c.metodo, c.percorso, resp.StatusCode, resp.Header.Get("Cache-Control"), strings.TrimSpace(corpo))
		}
	}
}

// Prova (F5, A5.4.5; la rotta dalla fase 4.4a.1b): «Riapri il nodo» ha la sua POST. Il nodo scartato torna aperto, con
// la storia di chi l'aveva scartato e di chi lo riapre, e l'albero lo ripropone; un nodo che non e' scartato non si
// riapre (niente cambia); chi consulta riceve 403.
func TestRiapriIlNodoDallaSuaRotta(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.scenaAlberoWeb(t, "ALB46")
	op := operatore(b)
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	pid := uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND codice = '7121007'`, r.thread)
	stato := func() string {
		var s string
		if err := b.pool.QueryRow(b.ctx, `SELECT stato || ':' || coalesce(evidenza -> 'storia' -> -1 ->> 'evento', '-') FROM componente_proposta
			WHERE proposta_id = $1`, pid).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if _, html := op.daFascicolo(http.MethodPost, r.base()+"/nodo/"+pid.String()+"/riapri", url.Values{}, r.thread, ""); !strings.Contains(avvisoF(html), "si riapre solo un nodo scartato") ||
		stato() != "aperta:-" {
		t.Errorf("riaprire un nodo aperto: %q, %s", avvisoF(html), stato())
	}
	if _, html := op.daFascicolo(http.MethodPost, r.base()+"/nodo/"+pid.String()+"/scarta", url.Values{}, r.thread, ""); stato() != "scartata:-" {
		t.Fatalf("scarta: %q, %s", avvisoF(html), stato())
	}
	if resp, _ := co.chiediJSON(http.MethodPost, r.base()+"/nodo/"+pid.String()+"/riapri", ""); resp.StatusCode != http.StatusForbidden || stato() != "scartata:-" {
		t.Errorf("chi consulta: %d, %s", resp.StatusCode, stato())
	}
	if _, html := op.daFascicolo(http.MethodPost, r.base()+"/nodo/"+pid.String()+"/riapri", url.Values{}, r.thread, ""); !strings.Contains(avvisoF(html), "riaperto: torna fra le proposte") ||
		stato() != "aperta:nodo_riaperto" {
		t.Errorf("riapri: %q, %s", avvisoF(html), stato())
	}
	_, corpo := op.chiediJSON(http.MethodGet, "/thread/"+r.thread.String()+"/distinta/albero", "")
	var a fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &a); err != nil {
		t.Fatal(err)
	}
	if n, ok := a.Nodo("cod:7121007"); !ok || n.Stato != fascicolo.StatoAlberoProposto || n.Commerciale == nil {
		t.Errorf("dopo la riapertura l'albero: %+v", n)
	}
}
