//go:build integrazione

// L4 — Smistamento, fase T nel web: il tipo del componente lo decide una persona dalla scheda del componente
// (e dall'editor della Struttura BOM, che usa le stesse rotte). La tendina con il motivo di ogni tipo spento;
// l'anteprima in GET (non scrive, porta la firma); la POST htmx che ricontrolla la firma; la POST senza htmx che
// risponde con la pagina intera e l'esito; la riattivazione esplicita di un'autorizzazione sospesa dalla scheda;
// chi consulta vede e non cambia. Le regole del gesto, con le tabelle, stanno in
// core/rfq/fascicolo/tipo_db_test.go.

package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var reFirmaTipo = regexp.MustCompile(`name="firma" value="([0-9a-f]+)"`)

// anteprimaTipo e' la GET dell'anteprima del cambio di tipo dalla scheda.
func (r *rfqFascicolo) anteprimaTipo(w *browser, comp uuid.UUID, tipo string) string {
	r.b.t.Helper()
	resp, html := w.fai(http.MethodGet, r.base()+"/componente/"+comp.String()+"/tipo?tipo="+url.QueryEscape(tipo), nil, true)
	if resp.StatusCode != 200 {
		r.b.t.Fatalf("anteprima del tipo: HTTP %d", resp.StatusCode)
	}
	return html
}

// firmaVista e' la firma che un'anteprima porta nel suo modulo ("" se il modulo non c'e').
func firmaVista(html string) string {
	if m := reFirmaTipo.FindStringSubmatch(html); m != nil {
		return m[1]
	}
	return ""
}

// cambiaTipoDallaScheda: l'anteprima, poi la POST htmx con la firma vista. Restituisce l'avviso.
func (r *rfqFascicolo) cambiaTipoDallaScheda(w *browser, comp uuid.UUID, tipo string) string {
	r.b.t.Helper()
	firma := firmaVista(r.anteprimaTipo(w, comp, tipo))
	_, out := w.daFascicolo(http.MethodPost, r.base()+"/componente/"+comp.String()+"/tipo", url.Values{"tipo": {tipo}, "firma": {firma}}, r.thread, "")
	return avvisoF(out)
}

// Fase T dalla scheda: la sezione «Tipo» con la tendina (nessun tipo nuovo scelto, i tipi spenti con il motivo) e
// senza il tipo nel modulo «Modifica»; l'anteprima non scrive, dice che cosa resta e porta la firma; con la firma
// vista il tipo cambia, con una firma vecchia si rifiuta; un tipo spento non ha il modulo e la POST si rifiuta; chi
// consulta vede l'anteprima senza il modulo e non puo' scrivere.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che l'assieme 77720517, che ha il figlio 77817189,
// diventasse commerciale dalla scheda (l'opzione accesa, l'anteprima con «I figli già nella BOM restano: 77817189»
// e il bottone, il cambio fatto con i figli che restano). Con la regola della PR #7, confermata dall'utente il
// 29/09 sera (domanda 6a: il commerciale e' sempre una foglia), per l'assieme con un figlio particolare e
// particolare commerciale sono spenti, con il motivo e il consiglio, l'anteprima non ha il modulo e la POST si
// rifiuta; il cambio che si fa, con l'anteprima e la firma, e' quello del particolare 77817189, che non ha figli e
// diventa particolare commerciale, restando nella BOM sotto i suoi due padri. Asserzioni (righe con t.Error,
// t.Fatal): prima 15, dopo 19.
func TestIlTipoSiSceglieDallaScheda(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("TIPO1")
	w := operatore(b)
	const (
		spentoParticolare = "77720517 ha 1 figlio: è un assieme; per farlo diventare un particolare si spostano prima i suoi pezzi"
		spentoCommerciale = "77720517 ha 1 figlio: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi"
	)

	_, pagina := w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.assieme.String(), nil, s.thread, "?vista=bom")
	p := leggibile(pagina)
	for _, c := range []string{"<summary>Tipo</summary>", "77720517 è <b>assieme</b>", `<option value="">— scegli il tipo —</option>`,
		`<option value="sciolto" disabled title="` + spentoParticolare + `">particolare</option>`,
		`<option value="commerciale" disabled title="` + spentoCommerciale + `">particolare commerciale</option>`,
		"particolare: " + spentoParticolare + ".", "particolare commerciale: " + spentoCommerciale + ".",
		"prodotto: 77720517 non è un codice della richiesta", "Anteprima del cambio"} {
		if !strings.Contains(p, c) {
			t.Errorf("la scheda dell'assieme: manca %q", c)
		}
	}
	if strings.Contains(p, `selected>commerciale`) || strings.Contains(p, `value="commerciale" selected`) {
		t.Error("nessun tipo nuovo e' gia' scelto")
	}
	if m, _, _ := strings.Cut(estratto(pagina, "/modifica\""), "</form>"); strings.Contains(m, `name="tipo"`) || !strings.Contains(m, `name="rev"`) {
		t.Errorf("il modulo «Modifica» non cambia piu' il tipo:\n%s", m)
	}
	// la scheda del particolare, che non ha figli: il particolare commerciale si puo' scegliere
	_, pagina = w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.particolare.String(), nil, s.thread, "?vista=bom")
	if p := leggibile(pagina); !strings.Contains(p, `<option value="commerciale">particolare commerciale</option>`) || !strings.Contains(p, "77817189 è <b>particolare</b>") {
		t.Error("la scheda del particolare: il particolare commerciale e' acceso")
	}

	prima := fotoDelDatabase(t, b)
	html := s.anteprimaTipo(w, s.particolare, "commerciale")
	spento := leggibile(s.anteprimaTipo(w, s.assieme, "commerciale"))
	if dopo := fotoDelDatabase(t, b); !stessaFoto(prima, dopo) {
		t.Errorf("l'anteprima del tipo ha scritto: %v", differenzeFoto(prima, dopo))
	}
	a := leggibile(html)
	for _, c := range []string{"Tipo di <b class=\"mono\">77817189</b>: particolare → <b>particolare commerciale</b>",
		"77817189 resta nella BOM Promatec come pezzo comprato", ">77817189 diventa un particolare commerciale</button>"} {
		if !strings.Contains(a, c) {
			t.Errorf("l'anteprima: manca %q", c)
		}
	}
	if strings.Contains(a, "I figli già nella BOM restano") {
		t.Error("un particolare commerciale non ha figli che restano")
	}
	firma := firmaVista(html)
	if firma == "" {
		t.Fatal("l'anteprima non porta la firma")
	}
	// l'assieme con un figlio: particolare commerciale spento, niente modulo, e la POST si rifiuta
	if !strings.Contains(spento, `<p class="nota-blocco">`+spentoCommerciale+`</p>`) || strings.Contains(spento, `name="firma"`) {
		t.Errorf("l'anteprima del commerciale con un figlio: %s", spento)
	}
	if av := s.gesto(w, s.comp(s.assieme, "tipo"), url.Values{"tipo": {"commerciale"}, "firma": {firma}}); av != "Niente è cambiato: "+spentoCommerciale {
		t.Errorf("la POST del commerciale con un figlio: %q", av)
	}
	// un tipo spento: niente modulo, e la POST si rifiuta
	spento = leggibile(s.anteprimaTipo(w, s.assieme, "sciolto"))
	if !strings.Contains(spento, `<p class="nota-blocco">`+spentoParticolare+`</p>`) || strings.Contains(spento, `name="firma"`) {
		t.Errorf("l'anteprima di un tipo spento: %s", spento)
	}
	if av := s.gesto(w, s.comp(s.assieme, "tipo"), url.Values{"tipo": {"sciolto"}, "firma": {firma}}); av != "Niente è cambiato: "+spentoParticolare {
		t.Errorf("la POST di un tipo spento: %q", av)
	}
	// una firma vecchia si rifiuta
	if av := s.gesto(w, s.comp(s.particolare, "tipo"), url.Values{"tipo": {"commerciale"}, "firma": {"deadbeef"}}); !strings.Contains(av, "riapri l'anteprima") {
		t.Errorf("la firma vecchia: %q", av)
	}
	// chi consulta: l'anteprima senza il modulo, la POST rifiutata
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if h := s.anteprimaTipo(co, s.particolare, "commerciale"); strings.Contains(h, `name="firma"`) {
		t.Error("chi consulta vede l'anteprima senza il modulo")
	}
	if resp, _ := co.daFascicolo(http.MethodPost, s.comp(s.particolare, "tipo"), url.Values{"tipo": {"commerciale"}, "firma": {firma}}, s.thread, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("chi consulta non cambia il tipo: HTTP %d", resp.StatusCode)
	}
	if got := s.valore(`SELECT string_agg(codice || ':' || tipo::text, ' ' ORDER BY codice) FROM componente WHERE thread_id = $1`, s.thread); got != "77720517:sottoassieme 77722757:finito 77817189:sciolto" {
		t.Fatalf("niente e' cambiato finora: %s", got)
	}

	// con la firma vista, si'
	if av := s.gesto(w, s.comp(s.particolare, "tipo"), url.Values{"tipo": {"commerciale"}, "firma": {firma}}); av != "77817189: tipo particolare → particolare commerciale." {
		t.Errorf("il cambio di tipo: %q", av)
	}
	if got := s.valore(`SELECT tipo::text FROM componente WHERE componente_id = $1`, s.particolare); got != "commerciale" {
		t.Errorf("il tipo: %s", got)
	}
	if s.archi() != "77720517>77817189x4 77722757>77720517x2 77722757>77817189x1" {
		t.Errorf("il particolare commerciale resta nella BOM sotto i suoi padri: %s", s.archi())
	}
}

// La POST senza htmx (il modulo dell'anteprima mandato da un browser senza JavaScript) dice l'esito: la pagina
// intera del Fascicolo, aperta sulla scheda del componente, con l'avviso; fatto 200, rifiutato 422 con «Niente è
// cambiato». Lo stesso per la riattivazione.
//
// Riscritta per lo Smistamento (Distinta): prima fissava i testi «tipo particolare → commerciale», «77817189 è
// <b>commerciale</b>» e il rifiuto «77720517 ha 1 figlio: è un assieme». La PR #7 chiama il tipo «particolare
// commerciale» e aggiunge al rifiuto il consiglio («; per farlo diventare un particolare si spostano prima i suoi
// pezzi»). Asserzioni: prima 5, dopo 5.
func TestIlTipoSenzaHtmxDiceLEsito(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("TIPO2")
	w := operatore(b)
	firma := firmaVista(s.anteprimaTipo(w, s.particolare, "commerciale"))
	resp, html := w.fai(http.MethodPost, s.comp(s.particolare, "tipo"), url.Values{"tipo": {"commerciale"}, "firma": {firma}}, false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("senza htmx: HTTP %d", resp.StatusCode)
	}
	if !strings.Contains(html, "<html") || avvisoF(html) != "77817189: tipo particolare → particolare commerciale." {
		t.Errorf("senza htmx la pagina intera con l'esito: avviso %q, pagina intera %v", avvisoF(html), strings.Contains(html, "<html"))
	}
	if !strings.Contains(html, "77817189 è <b>particolare commerciale</b>") {
		t.Error("la pagina e' aperta sulla scheda del componente")
	}
	resp, html = w.fai(http.MethodPost, s.comp(s.assieme, "tipo"), url.Values{"tipo": {"sciolto"}, "firma": {"x"}}, false)
	if resp.StatusCode != http.StatusUnprocessableEntity || avvisoF(html) != "Niente è cambiato: 77720517 ha 1 figlio: è un assieme; per farlo diventare un particolare si spostano prima i suoi pezzi" ||
		!strings.Contains(html, "<html") {
		t.Errorf("senza htmx, rifiutato: HTTP %d, avviso %q", resp.StatusCode, avvisoF(html))
	}
	resp, html = w.fai(http.MethodPost, s.comp(s.assieme, "step-strutturale/riattiva"), url.Values{"sha": {"x"}, "firma": {"x"}}, false)
	if resp.StatusCode != http.StatusUnprocessableEntity || avvisoF(html) != "Niente è cambiato: 77720517 non ha un'autorizzazione sospesa da riattivare" {
		t.Errorf("la riattivazione senza htmx: HTTP %d, avviso %q", resp.StatusCode, avvisoF(html))
	}
}

// Le prove F5b del cambio sottoassieme ↔ commerciale nella schermata: l'assieme 77720517 ha il suo STEP
// autorizzato; portato a commerciale dalla scheda, l'autorizzazione si sospende (l'anteprima lo annuncia) e la
// scheda dice che si riattiva dopo il cambio di tipo, senza il bottone; la riattivazione di un commerciale e'
// spenta. Tornato assieme resta sospesa, e la scheda offre «Riattiva l'autorizzazione dello STEP …» (mai fatta da
// sola) accanto a «Revoca»; l'anteprima non scrive; la POST con la firma vista la riattiva.
//
// Riscritta per lo Smistamento (Distinta): prima fissava che l'assieme, con il figlio 77817189 nella working,
// diventasse commerciale dalla scheda. Con la regola della PR #7, confermata dall'utente il 29/09 sera (domanda
// 6a: il commerciale e' sempre una foglia), con il figlio l'anteprima e' spenta e la POST si rifiuta; l'operatore
// scollega prima il particolare (che resta sotto il prodotto), e poi il passaggio si fa, con la sospensione di
// prima. Il resto e' com'era; il bottone dice «un particolare commerciale» (NomeTipoFrase, PR #7). Asserzioni
// (righe con t.Error, t.Fatal): prima 12, dopo 15.
func TestLaRiattivazioneDallaSchedaDelComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("TIPO3")
	w := operatore(b)
	stp, _ := s.documentoDa("77720517.stp", "stp", "cad_3d", "77720517", s.assieme, "scritto")
	s.analisiCorrente(s.shaDoc(stp), fattiDi("#1", []string{"#1=77720517", "#2=77817189"}, []string{"#1>#2*4"}))
	if a := s.autorizzaDallaScheda(w, s.assieme, url.Values{"documento": {stp.String()}}, nil); !strings.Contains(a, "77720517.stp è lo STEP autorizzato di 77720517") {
		t.Fatalf("autorizzazione dell'assieme: %q", a)
	}
	// con il figlio 77817189 nella working: spento, e la POST si rifiuta
	const conFiglio = "77720517 ha 1 figlio: è un assieme; per farlo diventare un particolare commerciale si spostano prima i suoi pezzi"
	if a := leggibile(s.anteprimaTipo(w, s.assieme, "commerciale")); !strings.Contains(a, `<p class="nota-blocco">`+conFiglio+`</p>`) || strings.Contains(a, `name="firma"`) {
		t.Errorf("l'anteprima del commerciale con un figlio:\n%s", a)
	}
	if av := s.gesto(w, s.comp(s.assieme, "tipo"), url.Values{"tipo": {"commerciale"}, "firma": {"x"}}); av != "Niente è cambiato: "+conFiglio {
		t.Errorf("la POST del commerciale con un figlio: %q", av)
	}
	// l'operatore sposta prima il pezzo: 77817189 resta sotto il prodotto
	if av := s.gesto(w, s.comp(s.particolare, "scollega"), url.Values{"padre": {s.assieme.String()}}); av != "77817189 non è più sotto 77720517." {
		t.Fatalf("scollega il particolare dall'assieme: %q", av)
	}
	a := leggibile(s.anteprimaTipo(w, s.assieme, "commerciale"))
	if !strings.Contains(a, "Si sospende l'autorizzazione di 77720517.stp per 77720517") || !strings.Contains(a, "77720517 diventa un particolare commerciale: 1 autorizzazione si sospende") {
		t.Errorf("l'anteprima annuncia la sospensione:\n%s", a)
	}
	if av := s.cambiaTipoDallaScheda(w, s.assieme, "commerciale"); !strings.Contains(av, "Sospesa l'autorizzazione di 77720517.stp per 77720517") {
		t.Fatalf("a commerciale: %q", av)
	}
	_, pagina := w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.assieme.String(), nil, s.thread, "?vista=bom")
	p := leggibile(pagina)
	if !strings.Contains(p, "Sospesa: si riattiva dopo il cambio di tipo, oppure si revoca.") || strings.Contains(p, "/step-strutturale/riattiva") {
		t.Error("la scheda del commerciale: la frase, e nessun bottone di riattivazione")
	}
	sha := s.shaDoc(stp)
	rf := leggibile(riattivazione(t, w, s, sha))
	if !strings.Contains(rf, "77720517 è un commerciale: il suo STEP resta guida; per riattivare l'autorizzazione cambia prima il tipo") || strings.Contains(rf, `name="firma"`) {
		t.Errorf("la riattivazione di un commerciale e' spenta:\n%s", rf)
	}

	// tornato assieme: resta sospesa, e la scheda offre la scelta
	if av := s.cambiaTipoDallaScheda(w, s.assieme, "sottoassieme"); !strings.Contains(av, "Resta sospesa l'autorizzazione di 77720517.stp per 77720517") {
		t.Fatalf("di nuovo assieme: %q", av)
	}
	_, pagina = w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.assieme.String(), nil, s.thread, "?vista=bom")
	p = leggibile(pagina)
	for _, c := range []string{"sospesa: 77720517: è diventato commerciale (si riattiva solo con una scelta esplicita)",
		"Riattiva l'autorizzazione dello STEP 77720517.stp…", "/step-strutturale/revoca"} {
		if !strings.Contains(p, c) {
			t.Errorf("la scheda dell'assieme tornato: manca %q", c)
		}
	}
	if got := s.valore(`SELECT count(*)::text FROM componente_proposta WHERE thread_id = $1 AND (evidenza -> 'strutturale') ? 'sospesa'`, s.thread); got != "1" {
		t.Errorf("tornato assieme, la sospensione resta registrata: %s", got)
	}
	prima := fotoDelDatabase(t, b)
	html := riattivazione(t, w, s, sha)
	if dopo := fotoDelDatabase(t, b); !stessaFoto(prima, dopo) {
		t.Errorf("l'anteprima della riattivazione ha scritto: %v", differenzeFoto(prima, dopo))
	}
	rf = leggibile(html)
	for _, c := range []string{"Riattivazione: <b class=\"mono\">77720517.stp</b> per <b class=\"mono\">77720517</b>", "Sospesa: è diventato commerciale",
		"1 figlio diretto torna nell'autorità di 77720517", ">Riattiva l'autorizzazione dello STEP 77720517.stp per 77720517</button>"} {
		if !strings.Contains(rf, c) {
			t.Errorf("l'anteprima della riattivazione: manca %q", c)
		}
	}
	_, out := w.daFascicolo(http.MethodPost, s.comp(s.assieme, "step-strutturale/riattiva"), url.Values{"sha": {sha}, "firma": {firmaVista(html)}}, s.thread, "")
	if av := avvisoF(out); av != "Riattivata l'autorizzazione di 77720517.stp per 77720517: 1 figlio diretto torna nell'autorità." {
		t.Errorf("la riattivazione: %q", av)
	}
	if got := s.valore(`SELECT count(*)::text FROM componente_proposta WHERE thread_id = $1 AND (evidenza -> 'strutturale') ? 'sospesa'`, s.thread); got != "0" {
		t.Errorf("riattivata, nessuna sospensione: %s", got)
	}
}

// riattivazione e' la GET dell'anteprima della riattivazione dalla scheda dell'assieme.
func riattivazione(t *testing.T, w *browser, s *scenaB87, sha string) string {
	t.Helper()
	resp, html := w.fai(http.MethodGet, s.comp(s.assieme, "step-strutturale/riattiva")+"?sha="+sha, nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("anteprima della riattivazione: HTTP %d", resp.StatusCode)
	}
	return html
}
