//go:build integrazione

// L4 — A1d P4, Scenario G della direttiva 12 (§6 e §10; D-K6-01 = A [U] con la precisazione): la conferma contestuale
// del cambio di tipo nel Fascicolo legacy, dal server vero. Il particolare 7120012 ha il suo STEP autorizzato, che
// propone i figli diretti 7120021 e 7120022 ×4.
//   - «Accetta l'arco» dalla schermata del Fascicolo: la risposta e' la domanda per 7120012, con il form che rimanda
//     lo stesso gesto con il suo id in `assieme`; la working e le proposte non cambiano (anche il «No» della persona,
//     che non manda niente, lascia tutto cosi').
//   - Il form rimandato: arco e conversione insieme, l'avviso lo dice.
//   - La stessa richiesta senza la schermata del Fascicolo (la pagina della RFQ): il rifiuto di sempre, che nomina la
//     scheda, e nessun form.
//   - La stessa richiesta dalla vista Documenti (la predefinita): nessuna domanda, perche' li' il «Sì» non partirebbe
//     (revisione D-E del 13/10, rilievo 1); il rifiuto nomina anche la vista Albero, e niente e' scritto.
//   - `assieme` non valido: «Niente è cambiato», nessuna domanda.
//   - «accetta il sottoalbero» (file/{aid}/accetta con `chiave`): la domanda rimanda anche `chiave`.
//
// Stato: eseguita e passata nell'integrazione della corsia D (13/10/2026, L4).

package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/testutil"
)

var reDomandaTipo = regexp.MustCompile(`(?s)<div class="avviso-f no conferma-tipo" role="alertdialog"[^>]*>(.*?)</div>`)

// domandaTipo e' la domanda del cambio di tipo nella risposta, come la legge l'operatore ("" se non c'e').
func domandaTipo(html string) string {
	if m := reDomandaTipo.FindStringSubmatch(html); m != nil {
		return leggibile(m[1])
	}
	return ""
}

// campiNascosti sono i campi del form della domanda, «nome=valore» nell'ordine della pagina. Si leggono solo dentro
// la domanda: il resto del frammento ha altri form con campi nascosti (per esempio «Prepara i file», da=fascicolo).
func campiNascosti(html string) []string {
	d := reDomandaTipo.FindStringSubmatch(html)
	if d == nil {
		return nil
	}
	var out []string
	for _, m := range regexp.MustCompile(`<input type="hidden" name="([^"]+)" value="([^"]*)">`).FindAllStringSubmatch(d[1], -1) {
		out = append(out, m[1]+"="+leggibile(m[2]))
	}
	return out
}

func TestScenarioGConfermaContestualeDalFascicolo(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "P4-SCENARIO-G")
	r.fase("FATTIBILITA")
	r.esegui(`UPDATE cliente SET regole = $2 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $1)`, r.thread,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`)
	p := r.componenteTipo("7120012", "sciolto")
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	stp := r.stepNellaRfq("7120012.stp", fattiDi("#1", []string{"#1=7120012", "#2=7120021", "#3=7120022"}, []string{"#1>#2*1", "#1>#3*4"}), an)
	w := operatore(b)
	rianalizza := func() {
		t.Helper()
		_, html := w.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, "")
		if a := avvisoF(html); !strings.Contains(a, "1 STEP riletto") {
			t.Fatalf("rianalizza: %q", a)
		}
	}
	rianalizza()
	testutil.AutorizzaStep(t, b.pool, r.thread, p, stp, r.utente)
	rianalizza()
	for _, k := range []string{"#2", "#3"} {
		pid := valoreR(r, `SELECT proposta_id::text FROM componente_proposta WHERE allegato_id = $1 AND chiave = $2`, stp, k)
		if _, html := w.daFascicolo(http.MethodPost, r.base()+"/nodo/"+pid+"/accetta", url.Values{}, r.thread, "?vista=albero"); strings.HasPrefix(avvisoF(html), "Niente") {
			t.Fatalf("accetta il nodo %s: %q", k, avvisoF(html))
		}
	}
	stato := func() string {
		return valoreR(r, `SELECT concat_ws(' # ',
			(SELECT string_agg(codice || ':' || tipo, '|' ORDER BY codice) FROM componente WHERE thread_id = $1),
			(SELECT coalesce(string_agg(pc.codice || '>' || fc.codice || '*' || x.qta, '|' ORDER BY pc.codice, fc.codice), '-')
			   FROM componente_relazione x JOIN componente pc ON pc.componente_id = x.padre_id JOIN componente fc ON fc.componente_id = x.figlio_id
			  WHERE x.thread_id = $1),
			(SELECT string_agg(padre_chiave || '>' || figlio_chiave || ':' || stato, '|' ORDER BY padre_chiave, figlio_chiave)
			   FROM relazione_proposta WHERE thread_id = $1))`, r.thread)
	}
	arco := url.Values{"allegato": {stp.String()}, "padre": {"#1"}, "figlio": {"#2"}, "corpo_chiave": {"vuoto"}}
	prima := stato()

	// 1. dalla schermata del Fascicolo: la domanda, niente scritto
	resp, html := w.daFascicolo(http.MethodPost, r.base()+"/relazione/accetta", arco, r.thread, "?vista=albero")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stato HTTP della domanda: %d", resp.StatusCode)
	}
	if d := domandaTipo(html); !strings.Contains(d, "Il componente 7120012 è attualmente un particolare. Aggiungendo 7120021 come figlio, "+
		"7120012 diventerà un sottoassieme. Vuoi confermare questa modifica?") || !strings.Contains(d, "Finché non confermi, niente è cambiato.") {
		t.Fatalf("la domanda: %q\n%.600s", d, html)
	}
	if !strings.Contains(html, `hx-post="`+r.base()+`/relazione/accetta"`) {
		t.Errorf("il form rimanda lo stesso gesto")
	}
	campi := strings.Join(campiNascosti(html), " ")
	if campi != "allegato="+stp.String()+" padre=#1 figlio=#2 assieme="+p.String() {
		t.Errorf("i campi del form: %s", campi)
	}
	if avvisoF(html) != "" {
		t.Errorf("la domanda non e' un esito: %q", avvisoF(html))
	}
	if dopo := stato(); dopo != prima {
		t.Errorf("la domanda ha scritto (o il «No» lascia uno stato diverso):\n prima %s\n dopo  %s", prima, dopo)
	}

	// 2. la stessa richiesta dalla pagina della RFQ: il rifiuto con la scheda, nessun form
	_, html = w.fai(http.MethodPost, r.base()+"/relazione/accetta", arco, true)
	if a := avvisoDi(html); a != "Niente è cambiato: 7120012 → 7120021: 7120012 è un particolare, e il tipo non cambia da solo: se contiene dei pezzi, cambialo prima in assieme dalla sua scheda" {
		t.Errorf("dalla pagina della RFQ: %q", a)
	}
	if strings.Contains(html, "conferma-tipo") {
		t.Error("fuori dal Fascicolo nessuna domanda")
	}

	// 2b. dalla vista Documenti del Fascicolo: nessuna domanda, il rifiuto con la vista Albero, niente scritto
	_, html = w.daFascicolo(http.MethodPost, r.base()+"/relazione/accetta", arco, r.thread, "")
	if a := avvisoF(html); a != "Niente è cambiato: 7120012 → 7120021: 7120012 è un particolare, e il tipo non cambia da solo: "+
		"se contiene dei pezzi, cambialo prima in assieme dalla sua scheda; oppure rifai il gesto dalla vista Albero del Fascicolo, "+
		"che chiede di confermare il cambio" || domandaTipo(html) != "" {
		t.Errorf("dalla vista Documenti: %q (domanda %q)", a, domandaTipo(html))
	}
	if dopo := stato(); dopo != prima {
		t.Errorf("il rifiuto dalla vista Documenti ha scritto: %s → %s", prima, dopo)
	}

	// 3. un `assieme` non valido: rifiuto, nessuna domanda, niente scritto
	cattivo := url.Values{"allegato": {stp.String()}, "padre": {"#1"}, "figlio": {"#2"}, "assieme": {"7120012"}}
	_, html = w.daFascicolo(http.MethodPost, r.base()+"/relazione/accetta", cattivo, r.thread, "?vista=albero")
	if a := avvisoF(html); a != "Niente è cambiato: componente da cambiare in assieme non valido" || domandaTipo(html) != "" {
		t.Errorf("assieme non valido: %q", a)
	}
	if dopo := stato(); dopo != prima {
		t.Errorf("un rifiuto ha scritto: %s → %s", prima, dopo)
	}

	// 4. il sottoalbero di #3: la domanda rimanda anche `chiave`
	_, html = w.daFascicolo(http.MethodPost, r.base()+"/file/"+stp.String()+"/accetta", url.Values{"chiave": {"#3"}}, r.thread, "?vista=albero")
	if !strings.Contains(domandaTipo(html), "Aggiungendo 7120022 come figlio") ||
		strings.Join(campiNascosti(html), " ") != "chiave=#3 assieme="+p.String() ||
		!strings.Contains(html, `hx-post="`+r.base()+`/file/`+stp.String()+`/accetta"`) {
		t.Errorf("la domanda del sottoalbero: %q %v", domandaTipo(html), campiNascosti(html))
	}

	// 5. «Sì»: il form rimandato, arco e conversione insieme
	conferma := url.Values{"allegato": {stp.String()}, "padre": {"#1"}, "figlio": {"#2"}, "assieme": {p.String()}, "corpo_chiave": {"vuoto"}}
	_, html = w.daFascicolo(http.MethodPost, r.base()+"/relazione/accetta", conferma, r.thread, "?vista=albero")
	if a := avvisoF(html); a != "7120012 → 7120021 ×1 entra nella BOM; 7120012 diventa un assieme." {
		t.Errorf("la conferma: %q", a)
	}
	if got := valoreR(r, `SELECT tipo::text FROM componente WHERE componente_id = $1`, p); got != "sottoassieme" {
		t.Errorf("confermato, 7120012 e' un assieme: %s", got)
	}
	if got := stato(); !strings.Contains(got, "7120012>7120021*1") || !strings.Contains(got, "#1>#2:confermata") {
		t.Errorf("arco e proposta: %s", got)
	}
}
