//go:build integrazione

// L4 — Smistamento F5b nel web: lo STEP autorizzato dalla scheda del componente. Il riquadro «STEP
// autorizzato» per qualunque componente non commerciale, nessuno STEP preselezionato; l'anteprima in GET (non
// scrive, porta la firma, la casella mai spuntata, la presa d'atto, le deleghe); la scrittura in POST che
// ricontrolla la firma; la revoca; la delega dalla scheda del componente annidato; il commerciale spento.
//
// Prove dell'addendum: 95 (parte casella), 111, 113; e la GET dell'anteprima fra quelle che non scrivono (98).
// Le regole del gesto, con le tabelle, stanno in core/rfq/fascicolo/autorizzazione_db_test.go.

package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// anScena e' l'analizzatore delle prove: quello dei fatti che le scene scrivono.
var anScena = coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}

// analisiCorrente scrive i fatti correnti di un contenuto, con l'analizzatore corrente: uno STEP si autorizza
// solo con un'analisi corrente (Smistamento F5b, A5.4.7 passo 3).
func (r *rfqFascicolo) analisiCorrente(sha, fatti string) {
	r.b.t.Helper()
	r.esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)
		ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = EXCLUDED.versione_analizzatore, hash_configurazione = EXCLUDED.hash_configurazione`,
		anScena.Versione, anScena.Hash())
	r.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)
		ON CONFLICT (sha256, versione_analizzatore, hash_configurazione) DO UPDATE SET fatti = EXCLUDED.fatti, calcolato_il = now()`,
		sha, anScena.Versione, anScena.Hash(), fatti)
}

// fattiSoloRadice e' lo STEP del prodotto della scena B87 letto in parte, con la sola radice: si autorizza, le
// rimozioni restano sospese, e nessun figlio diretto chiede una decisione (la deroga strutturale lo copre).
func fattiSoloRadice() string {
	return strings.Replace(fattiDi("#1", []string{"#1=77722757"}, nil), `"limiti": {"troncato": false}`, `"limiti": {"troncato": true, "motivo": "nodi"}`, 1)
}

// shaDoc e' lo sha di un documento.
func (r *rfqFascicolo) shaDoc(doc uuid.UUID) string {
	return valoreR(r, `SELECT sha256 FROM documento WHERE documento_id = $1`, doc)
}

// fattiB87 e' lo STEP del prodotto della scena B87 (77722757.stp): la radice 77722757, l'assieme 77720517 ×2,
// e sotto l'assieme il particolare 77817189 ×4. troncato: la lettura e' parziale.
func fattiB87(troncato bool) string {
	f := fattiDi("#1", []string{"#1=77722757", "#2=77720517", "#3=77817189"}, []string{"#1>#2*2", "#2>#3*4"})
	if troncato {
		f = strings.Replace(f, `"limiti": {"troncato": false}`, `"limiti": {"troncato": true, "motivo": "nodi"}`, 1)
	}
	return f
}

var (
	reFirmaAutorizzazione = regexp.MustCompile(`name="firma" value="([0-9a-f]+)"`)
	reCasella             = regexp.MustCompile(`<input type="checkbox" name="(autorizza|delega|presa_d_atto|raggruppamento)"[^>]*>`)
)

// anteprimaAutorizzazione e' la GET dell'anteprima dalla scheda del componente.
func (r *rfqFascicolo) anteprimaAutorizzazione(w *browser, comp uuid.UUID, campi url.Values) string {
	r.b.t.Helper()
	resp, html := w.fai(http.MethodGet, r.base()+"/componente/"+comp.String()+"/step-strutturale?"+campi.Encode(), nil, true)
	if resp.StatusCode != 200 {
		r.b.t.Fatalf("anteprima: HTTP %d", resp.StatusCode)
	}
	return html
}

// autorizzaDallaScheda e' il gesto di F5b dalla scheda del componente, come lo fa una persona: l'anteprima
// (GET), poi la POST con la firma che l'anteprima porta, la casella spuntata e, se l'anteprima la chiede, la
// presa d'atto. extra si aggiunge al modulo (le deleghe, i raggruppamenti). Restituisce l'avviso.
func (r *rfqFascicolo) autorizzaDallaScheda(w *browser, comp uuid.UUID, campi, extra url.Values) string {
	r.b.t.Helper()
	html := r.anteprimaAutorizzazione(w, comp, campi)
	form := url.Values{}
	for k, v := range campi {
		form[k] = v
	}
	for k, v := range extra {
		form[k] = v
	}
	if m := reFirmaAutorizzazione.FindStringSubmatch(html); m != nil {
		form.Set("firma", m[1])
	}
	form.Set("autorizza", "1")
	if strings.Contains(html, `name="presa_d_atto"`) {
		form.Set("presa_d_atto", "1")
	}
	_, out := w.daFascicolo(http.MethodPost, r.base()+"/componente/"+comp.String()+"/step-strutturale", form, r.thread, "")
	return avvisoF(out)
}

// Prova 95 (la parte della casella, P6, P26, U7) e 98 (F1): la scheda di un componente non commerciale ha il
// riquadro «STEP autorizzato» senza nessuno STEP scelto, anche se ce n'e' uno solo; l'anteprima non scrive
// niente, porta la firma di quello che si vede, e nessuna delle sue caselle nasce spuntata (l'autorizzazione,
// le deleghe). Chi consulta vede l'anteprima senza il modulo. La scrittura senza la casella si rifiuta; con la
// casella, la firma vista e la persona che decide, lo STEP e' autorizzato.
func TestNessunaSceltaStrutturaleNasceSpuntataNellAutorizzazione(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("AUT95")
	s.analisiCorrente(s.shaDoc(s.step), fattiB87(false))
	w := operatore(b)

	_, pagina := w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.prodotto.String(), nil, s.thread, "?vista=bom")
	for _, c := range []string{"<summary>STEP autorizzato</summary>", `<option value="">— scegli lo STEP —</option>`, "Anteprima dell'autorizzazione", "Nessuno STEP è già scelto"} {
		if !strings.Contains(pagina, c) {
			t.Errorf("la scheda del prodotto: manca %q", c)
		}
	}
	if strings.Contains(pagina, `value="`+s.step.String()+`" selected`) || strings.Contains(pagina, `value="`+s.step.String()+`"selected`) {
		t.Error("l'unico STEP del prodotto e' gia' scelto")
	}

	prima := fotoDelDatabase(t, b)
	html := s.anteprimaAutorizzazione(w, s.prodotto, url.Values{"documento": {s.step.String()}})
	if dopo := fotoDelDatabase(t, b); !stessaFoto(prima, dopo) {
		t.Errorf("l'anteprima ha scritto: %v", differenzeFoto(prima, dopo))
	}
	a := leggibile(html)
	for _, c := range []string{"Autorizzazione: <b class=\"mono\">77722757.stp</b> per <b class=\"mono\">77722757</b>", "1 figlio diretto entra nell'autorità di 77722757",
		"77720517", "1 nodo del file resta guida · sotto 77720517: 1", "diventa anche il suo STEP strutturale",
		"Autorizza 77722757.stp per 77722757: 1 figlio diretto"} {
		if !strings.Contains(a, c) {
			t.Errorf("l'anteprima: manca %q", c)
		}
	}
	// la delega di un figlio non ancora deciso e' spenta, e non nomina il componente del file (77722757) ma il nodo
	if !strings.Contains(a, "Autorizza questo STEP anche per il componente di 77720517, quando sarà deciso") || strings.Contains(a, "anche per 77722757") {
		t.Errorf("la casella spenta della delega nomina il nodo:\n%s", estratto(a, "Autorizza questo STEP anche per"))
	}
	caselle := reCasella.FindAllString(html, -1)
	if len(caselle) == 0 {
		t.Fatal("l'anteprima non ha la casella dell'autorizzazione")
	}
	for _, c := range caselle {
		if strings.Contains(c, "checked") {
			t.Errorf("una casella nasce spuntata: %s", c)
		}
	}
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if h := s.anteprimaAutorizzazione(co, s.prodotto, url.Values{"documento": {s.step.String()}}); strings.Contains(h, `name="autorizza"`) || !strings.Contains(h, "1 figlio diretto") {
		t.Error("chi consulta vede l'anteprima, senza il modulo")
	}

	firma := reFirmaAutorizzazione.FindStringSubmatch(html)[1]
	if a := s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}, "firma": {firma}}); !strings.Contains(a, "la casella non nasce spuntata") {
		t.Errorf("senza la casella: %q", a)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente_proposta WHERE evidenza ? 'strutturale'`); n != 0 {
		t.Fatalf("il rifiuto ha scritto %d marcature", n)
	}
	if a := s.autorizzaDallaScheda(w, s.prodotto, url.Values{"documento": {s.step.String()}}, nil); !strings.Contains(a, "77722757.stp è lo STEP autorizzato di 77722757") {
		t.Fatalf("autorizzazione: %q", a)
	}
	if got := s.valore(`SELECT (evidenza -> 'strutturale' ->> 'dichiarato_da') || ' ' || stato FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#1'`, s.allStep); got != s.utente.String()+" duplicato" {
		t.Errorf("la marcatura dice chi ha autorizzato: %s", got)
	}
	// la scheda adesso dice l'autorizzazione, con la revoca, e il file aperto il suo componente
	_, pagina = w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.prodotto.String(), nil, s.thread, "?vista=bom")
	if !strings.Contains(pagina, "/componente/"+s.prodotto.String()+"/step-strutturale/revoca") || !strings.Contains(pagina, "chip ok\">STEP autorizzato</span>") {
		t.Error("la scheda dice l'autorizzazione con la revoca")
	}
	_, file := w.daFascicolo(http.MethodGet, s.base()+"/anteprima?vista=bom&file="+s.allStep.String(), nil, s.thread, "?vista=bom")
	if !strings.Contains(file, "STEP autorizzato di 77722757") {
		t.Error("il file aperto dice di chi e' lo STEP autorizzato")
	}
}

// stessaFoto confronta due foto del database; differenzeFoto dice quali tabelle sono cambiate.
func stessaFoto(a, b map[string]string) bool { return len(differenzeFoto(a, b)) == 0 }

func differenzeFoto(a, b map[string]string) []string {
	var out []string
	for k, v := range a {
		if b[k] != v {
			out = append(out, k)
		}
	}
	return out
}

// Prova 111 (A5.4.4, A5.3.11): prima dell'autorizzazione le proposte dello STEP sono guida (non nell'editor come
// proposte, non nel gate); autorizzato dalla scheda, la sorgente e' il prodotto, deciso dalla persona che ha
// autorizzato, e nell'editor entrano i figli diretti (l'assieme, ritrovato per codice da vedere), non i nipoti
// (il particolare sotto l'assieme resta nella guida).
func TestAutorizzareLoStepPortaIFigliDirettiNellEditor(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("AUT111")
	s.analisiCorrente(s.shaDoc(s.step), fattiB87(false))
	b.ws.Analizzatore = anScena
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	w := operatore(b)
	if a := s.gesto(w, s.base()+"/rianalizza", nil); !strings.Contains(a, "STEP riletto") {
		t.Fatalf("rianalizza: %q", a)
	}
	editor := func() datiEditor {
		t.Helper()
		_, body := w.fai(http.MethodGet, s.base()+"/bom/dati?prodotto="+s.prodotto.String(), nil, false)
		var d datiEditor
		if err := json.Unmarshal([]byte(body), &d); err != nil {
			t.Fatalf("dati dell'editor: %v\n%s", err, body)
		}
		return d
	}
	d := editor()
	if len(d.Proposti)+len(d.Ritrovati) != 0 || len(d.Guida) != 2 {
		t.Fatalf("prima: tutto guida (proposti %d, ritrovati %d, guida %d)", len(d.Proposti), len(d.Ritrovati), len(d.Guida))
	}
	_, pagina := w.fai(http.MethodGet, s.base(), nil, false)
	if strings.Contains(pagina, "decisioni strutturali aperte negli STEP autorizzati") {
		t.Error("prima: la guida non entra nel gate")
	}

	if a := s.autorizzaDallaScheda(w, s.prodotto, url.Values{"documento": {s.step.String()}}, nil); !strings.Contains(a, "è lo STEP autorizzato di 77722757") {
		t.Fatalf("autorizzazione: %q", a)
	}
	if got := s.valore(`SELECT stato || ' ' || coalesce(componente_id::text, '-') || ' ' || coalesce(deciso_da::text, '-') FROM componente_proposta
		WHERE allegato_id = $1 AND chiave = '#1'`, s.allStep); got != "duplicato "+s.prodotto.String()+" "+s.utente.String() {
		t.Errorf("la sorgente e' il prodotto, decisa da chi ha autorizzato: %s", got)
	}
	d = editor()
	if len(d.Ritrovati) != 1 || d.Ritrovati[0].Codice != "77720517" {
		t.Errorf("il figlio diretto 77720517, ritrovato per codice, da vedere: %+v", d.Ritrovati)
	}
	if len(d.Guida) != 1 || d.Guida[0].Figlio != "77817189" || d.Guida[0].Padre != "77720517" {
		t.Errorf("il nipote 77817189 resta guida: %+v", d.Guida)
	}
	for _, p := range d.Proposti {
		if p.PK == "#2" {
			t.Errorf("l'arco verso il nipote non e' una proposta: %+v", p)
		}
	}
}

// Prova 113 (P7): la radice gia' decisa da una persona come un altro componente ferma l'autorizzazione:
// l'anteprima e' spenta e dice che cosa fare, la scrittura si rifiuta e niente cambia.
func TestLaRadiceGiaAccettataComeAltroFermaLoStrutturale(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("AUT113")
	s.analisiCorrente(s.shaDoc(s.step), fattiB87(false))
	b.ws.Analizzatore = anScena
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	w := operatore(b)
	if a := s.gesto(w, s.base()+"/rianalizza", nil); !strings.Contains(a, "STEP riletto") {
		t.Fatalf("rianalizza: %q", a)
	}
	s.esegui(`UPDATE componente_proposta SET stato = 'duplicato', componente_id = $2, deciso_da = $3, deciso_il = now()
		WHERE allegato_id = $1 AND chiave = '#1'`, s.allStep, s.assieme, s.utente)
	html := leggibile(s.anteprimaAutorizzazione(w, s.prodotto, url.Values{"documento": {s.step.String()}}))
	if !strings.Contains(html, "la radice è già il componente 77720517: archivialo o scegli un altro file") || strings.Contains(html, `name="autorizza"`) {
		t.Errorf("l'anteprima spenta dice perche': %s", estratto(html, "autorizzazione"))
	}
	if a := s.autorizzaDallaScheda(w, s.prodotto, url.Values{"documento": {s.step.String()}}, nil); !strings.Contains(a, "la radice è già il componente 77720517") {
		t.Errorf("la scrittura: %q", a)
	}
	if got := s.valore(`SELECT coalesce(step_strutturale_id::text, '-') FROM componente WHERE componente_id = $1`, s.prodotto); got != "-" {
		t.Errorf("niente cambia: %s", got)
	}
}

// Domanda 5 = B nella schermata: la scheda di un commerciale non ha il modulo dell'autorizzazione e dice che
// cosa fare; l'anteprima e' spenta; la POST si rifiuta.
//
// Riscritta per lo Smistamento (Distinta): prima fissava la stessa frase, «77817189 è un commerciale: …», nella
// scheda, nell'anteprima e nella scrittura. La PR #7 chiama il tipo «particolare commerciale» nella scheda
// (fascicolo.html); l'anteprima e la scrittura, che vengono dal core (autorizzazione.go), dicono ancora «è un
// commerciale». Qui si fissano tutte e due le frasi, ognuna dove sta. Asserzioni: prima 3, dopo 3.
func TestLaSchedaDiUnCommercialeNonAutorizza(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("AUTCOM")
	w := operatore(b)
	s.esegui(`UPDATE componente SET tipo = 'commerciale' WHERE componente_id = $1`, s.particolare)
	stp, _ := s.documentoDa("77817189.stp", "stp", "cad_3d", "77817189", s.particolare, "scritto")
	s.analisiCorrente(s.shaDoc(stp), fattiDi("#1", []string{"#1=77817189", "#2=77817190"}, []string{"#1>#2*1"}))
	_, pagina := w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.particolare.String(), nil, s.thread, "?vista=bom")
	const frase = "77817189 è un commerciale: il suo STEP resta guida; per autorizzarlo cambia prima il tipo"
	const fraseScheda = "77817189 è un particolare commerciale: il suo STEP resta guida; per autorizzarlo cambia prima il tipo"
	if !strings.Contains(leggibile(pagina), fraseScheda) || strings.Contains(pagina, "/componente/"+s.particolare.String()+"/step-strutturale\" hx-target") {
		t.Error("la scheda del commerciale: la frase, e nessun modulo dell'anteprima")
	}
	if html := leggibile(s.anteprimaAutorizzazione(w, s.particolare, url.Values{"documento": {stp.String()}})); !strings.Contains(html, frase) || strings.Contains(html, `name="autorizza"`) {
		t.Error("l'anteprima di un commerciale e' spenta")
	}
	if a := s.autorizzaDallaScheda(w, s.particolare, url.Values{"documento": {stp.String()}}, nil); !strings.Contains(a, frase) {
		t.Errorf("la scrittura per un commerciale: %q", a)
	}
}

// La delega dalla scheda del componente annidato (Domanda 1 = B, secondo ingresso): autorizzato lo STEP del
// prodotto e accettato l'assieme, la scheda dell'assieme offre il suo nodo nel file del prodotto (mai
// preselezionato); l'anteprima dice «sotto 77722757 nel file» e il figlio che entra; autorizzata, il particolare
// sotto l'assieme e' nell'autorita'. La revoca dalla scheda del prodotto la porta via.
func TestLaDelegaDallaSchedaDelComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("AUTDEL")
	s.analisiCorrente(s.shaDoc(s.step), fattiB87(false))
	w := operatore(b)
	if a := s.autorizzaDallaScheda(w, s.prodotto, url.Values{"documento": {s.step.String()}}, nil); !strings.Contains(a, "è lo STEP autorizzato") {
		t.Fatalf("autorizzazione del prodotto: %q", a)
	}
	nodo := s.valore(`SELECT proposta_id::text FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, s.allStep)
	if a := s.gesto(w, s.base()+"/nodo/"+nodo+"/accetta", nil); !strings.Contains(a, "77720517") {
		t.Fatalf("accetta l'assieme: %q", a)
	}
	valore := s.allStep.String() + ":#2"
	_, pagina := w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.assieme.String(), nil, s.thread, "?vista=bom")
	if !strings.Contains(pagina, `<option value="`+valore+`">77720517 in 77722757.stp, sotto 77722757</option>`) || !strings.Contains(pagina, "Anteprima della delega") {
		t.Fatalf("la scheda dell'assieme offre il suo nodo nel file del prodotto:\n%s", estratto(pagina, "Anteprima della delega"))
	}
	html := leggibile(s.anteprimaAutorizzazione(w, s.assieme, url.Values{"nodo": {valore}}))
	for _, c := range []string{"Delega", "(nel file, sotto 77722757)", "1 figlio diretto entra nell'autorità di 77720517", "77817189"} {
		if !strings.Contains(html, c) {
			t.Errorf("l'anteprima della delega: manca %q", c)
		}
	}
	if a := s.autorizzaDallaScheda(w, s.assieme, url.Values{"nodo": {valore}}, nil); !strings.Contains(a, "77722757.stp è autorizzato anche per 77720517 (delega, sotto 77722757 nel file)") {
		t.Fatalf("delega: %q", a)
	}
	if got := s.valore(`SELECT evidenza -> 'strutturale' ->> 'ruolo' FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, s.allStep); got != "delega" {
		t.Errorf("la marcatura della delega: %s", got)
	}
	sha := s.shaDoc(s.step)
	if a := s.gesto(w, s.comp(s.prodotto, "step-strutturale/revoca"), url.Values{"sha": {sha}, "motivo": {"file sbagliato"}}); !strings.Contains(a, "Revocate con lui le deleghe di 77720517") {
		t.Errorf("la revoca del padre: %q", a)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente_proposta WHERE evidenza ? 'strutturale'`); n != 0 {
		t.Errorf("dopo la revoca restano %d marcature", n)
	}
}

// La domanda «il nuovo file diventa lo STEP autorizzato?» nel cassetto «Da verificare» vale per lo STEP autorizzato
// di QUALUNQUE componente, come la chiede il server (StepAutorizzatoDi), non solo per lo STEP strutturale di un
// finito: con l'assieme 77720517 autorizzato sul suo STEP, un file nuovo con il suo codice chiede aggiungi o
// sostituisce, e il modulo ha le due risposte, nessuna scelta (P26). Senza risposta il server rifiuta; con il
// «si'» (e l'analisi del file nuovo) la sostituzione passa e il file nuovo e' lo STEP autorizzato dell'assieme.
func TestLaSostituzioneDelloStepAutorizzatoDiUnSottoassiemeChiedeLaRisposta(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaB87("AUTSOS")
	w := operatore(b)
	vecchio, _ := s.documentoDa("77720517.stp", "stp", db.TipoDocumentoCad3d, "77720517", s.assieme, db.StatoNasScritto)
	s.analisiCorrente(s.shaDoc(vecchio), fattiDi("#1", []string{"#1=77720517", "#2=77817189"}, []string{"#1>#2*4"}))
	if a := s.autorizzaDallaScheda(w, s.assieme, url.Values{"documento": {vecchio.String()}}, nil); !strings.Contains(a, "77720517.stp è lo STEP autorizzato di 77720517") {
		t.Fatalf("autorizzazione dell'assieme: %q", a)
	}
	if got := s.valore(`SELECT step_strutturale_id::text FROM componente WHERE componente_id = $1`, s.assieme); got != "NULL" {
		t.Fatalf("un sottoassieme non ha lo STEP strutturale, solo la marcatura: %s", got)
	}
	all, sha := s.allegatoExt("77720517_B.stp", "stp")
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1,$2,'cad_3d','77720517',95,'step')`, all, s.thread)
	pid := uuidSQL(t, b, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, all)

	_, verifica := w.fai(http.MethodGet, s.base()+"/parti?cassetto=verifica", nil, true)
	voce := verifica[strings.Index(verifica, `id="verifica-`+pid.String()+`"`)+1:]
	if i := strings.Index(voce, `class="verifica-voce`); i >= 0 {
		voce = voce[:i]
	}
	for _, c := range []string{"sostituisce 77720517.stp", "Se sostituisce lo STEP autorizzato, il nuovo file diventa lo STEP autorizzato?",
		`name="nuovo_riferimento" value="1">`, `name="nuovo_riferimento" value="0">`} {
		if !strings.Contains(voce, c) {
			t.Errorf("la voce del file nuovo nel cassetto: manca %q\n%s", c, voce)
		}
	}
	if regexp.MustCompile(`name="nuovo_riferimento"[^>]*checked`).MatchString(verifica) {
		t.Error("la risposta sul nuovo STEP autorizzato e' gia' scelta")
	}

	conferma := func(form url.Values) string {
		form.Set("ritorna_thread", s.thread.String())
		_, html := w.daFascicolo(http.MethodPost, "/proposta/"+pid.String()+"/conferma", form, s.thread, "?cassetto=verifica")
		return avvisoF(html)
	}
	f := url.Values{"componente_id": {s.assieme.String()}, "tipo": {"cad_3d"}, "codice": {"77720517"}, "scelta": {vecchio.String()}}
	if a := conferma(f); !strings.Contains(a, "sostituisce lo STEP autorizzato di 77720517") {
		t.Errorf("senza la risposta sul riferimento: %q", a)
	}
	s.analisiCorrente(sha, fattiDi("#1", []string{"#1=77720517", "#2=77817189"}, []string{"#1>#2*2"}))
	f.Set("nuovo_riferimento", "1")
	a := conferma(f)
	if !strings.Contains(a, "77720517.stp sostituito da 77720517_B.stp") || !strings.Contains(a, "77720517_B.stp è lo STEP autorizzato di 77720517") {
		t.Fatalf("la sostituzione con il si': %q", a)
	}
	if got := s.valore(`SELECT evidenza -> 'strutturale' ->> 'ruolo' FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#1'`, all); got != "radice" {
		t.Errorf("il file nuovo e' autorizzato per l'assieme: %s", got)
	}
}
