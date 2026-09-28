//go:build integrazione

// L4 — Smistamento F5 nel web: il principio dell'utente del 27/09 («nessun nodo acquisisce identita' di
// componente perche' nodo.codice == componente.codice») e i dati dell'editor sull'autorita', con la guida da
// vedere e il codice uguale come suggerimento.

package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/testutil"
)

// La prova del principio chiesta dall'utente. In una RFQ con i componenti 7120010 e 7120011 gia' esistenti:
//   - un nodo 7120010 in un file NON autorizzato (7120010.stp),
//   - un nodo 7120011 sotto il primo livello di un file autorizzato per 7120001 (7120001A_1.stp),
//
// restano aperti, senza componente_id e senza duplicato, e il codice uguale compare solo come suggerimento: i
// dati dell'editor disegnano il figlio diretto 7120010 come «ritrovato per codice» (da vedere, la conferma lo
// accetta solo cosi') e la guida dice «stesso codice di 7120011» senza collegarlo. La BOM non cambia, e la
// Struttura BOM dice in una riga che cosa resta guida.
func TestIlCodiceUgualeEUnSuggerimentoNonUnaIdentita(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "F5-PRINCIPIO")
	r.fase("FATTIBILITA")
	r.esegui(`UPDATE cliente SET regole = $2 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $1)`, r.thread,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`)
	prodotto := r.componenteTipo("7120001", "finito")
	c10 := r.componenteTipo("7120010", "sottoassieme")
	r.componenteTipo("7120011", "sciolto")
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	guida := r.stepNellaRfq("7120010.stp", fattiDi("#1", []string{"#1=7120010", "#2=7120012"}, []string{"#1>#2*1"}), an)
	stp := r.stepNellaRfq("7120001A_1.stp", fattiDi("#1", []string{"#1=7120001", "#2=7120010", "#3=7120011"}, []string{"#1>#2*1", "#2>#3*2"}), an)
	w := operatore(b)
	rianalizza := func() {
		t.Helper()
		_, html := w.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, "")
		if a := avvisoF(html); !strings.Contains(a, "STEP riletti") {
			t.Fatalf("rianalizza: %q", a)
		}
	}
	bom := func() string {
		return valoreR(r, `SELECT concat_ws(' # ', (SELECT string_agg(codice || ':' || tipo, '|' ORDER BY codice) FROM componente WHERE thread_id = $1),
			(SELECT string_agg(padre_id::text || '>' || figlio_id::text, '|' ORDER BY padre_id, figlio_id) FROM componente_relazione WHERE thread_id = $1))`, r.thread)
	}
	prima := bom()
	rianalizza()
	testutil.AutorizzaStep(t, b.pool, r.thread, prodotto, stp, r.utente)
	rianalizza()

	riga := func(allegato uuid.UUID, chiave string) string {
		return valoreR(r, `SELECT stato::text || ':' || coalesce(componente_id::text, 'senza componente') FROM componente_proposta
			WHERE allegato_id = $1 AND chiave = $2`, allegato, chiave)
	}
	for _, c := range []struct {
		nome     string
		allegato uuid.UUID
		chiave   string
	}{{"7120010 nel file non autorizzato", guida, "#1"}, {"7120011 sotto il primo livello del file autorizzato", stp, "#3"},
		{"7120010 figlio diretto, non ancora deciso", stp, "#2"}} {
		if got := riga(c.allegato, c.chiave); got != "aperta:senza componente" {
			t.Errorf("%s: %s", c.nome, got)
		}
	}
	if n := r.conta(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND stato = 'duplicato' AND deciso_da IS NULL`, r.thread); n != 0 {
		t.Errorf("%d duplicati senza chi li ha decisi: il codice uguale ha agganciato", n)
	}
	if got := riga(stp, "#1"); !strings.HasPrefix(got, "duplicato:"+prodotto.String()) {
		t.Errorf("la radice del file autorizzato e' il prodotto per l'autorizzazione di una persona: %s", got)
	}
	if got := bom(); got != prima {
		t.Errorf("la BOM e' cambiata:\n prima %s\n dopo  %s", prima, got)
	}

	// i dati dell'editor: il codice uguale e' un suggerimento
	_, corpo := w.daFascicolo(http.MethodGet, r.base()+"/bom/dati?prodotto="+prodotto.String(), nil, r.thread, "?vista=bom")
	var d datiEditor
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatalf("bom/dati: %v", err)
	}
	ritrovato := false
	for _, x := range d.Ritrovati {
		if x.Ref == refC(c10) && x.Codice == "7120010" && x.File == "7120001A_1.stp" {
			ritrovato = true
		}
	}
	if !ritrovato || len(d.Ritrovati) != 1 {
		t.Errorf("il figlio diretto 7120010 e' un ritrovato da vedere: %+v", d.Ritrovati)
	}
	proposto := false
	for _, p := range d.Proposti {
		if p.Padre == refC(prodotto) && p.Figlio == refC(c10) && p.Allegato == stp {
			proposto = true
		}
		if p.FK == "#3" || p.Allegato == guida {
			t.Errorf("un arco della guida fra le proposte dell'editor: %+v", p)
		}
	}
	if !proposto {
		t.Errorf("l'arco dell'autorita' verso il ritrovato: %+v", d.Proposti)
	}
	suggerito := map[string]string{}
	for _, g := range d.Guida {
		suggerito[g.File+":"+g.Padre+">"+g.Figlio] = g.Suggerito
	}
	if s, ok := suggerito["7120001A_1.stp:7120010>7120011"]; !ok || s != "7120011" {
		t.Errorf("la guida sotto il primo livello, con il suggerimento del codice uguale: %+v", d.Guida)
	}
	if _, ok := suggerito["7120010.stp:7120010>7120012"]; !ok {
		t.Errorf("la guida del file non autorizzato: %+v", d.Guida)
	}
	for ref := range d.Nodi {
		if strings.HasPrefix(ref, "p:") {
			t.Errorf("un nodo della guida, o un ritrovato, e' una carta dell'editor: %s", ref)
		}
	}

	// la Struttura BOM dice la guida in una riga, e non la conta fra le proposte
	_, pagina := w.fai(http.MethodGet, r.base()+"?vista=bom", nil, false)
	if !strings.Contains(pagina, `class="k guida-riga"`) || !strings.Contains(pagina, "1 STEP analizzato non è autorizzato per nessun componente") {
		t.Errorf("la riga della guida nella Struttura BOM:\n%.600s", estraiGuida(pagina))
	}
}

// Smistamento F5 (A5.4.7, guardie collegate): il documento di un file autorizzato a proporre i figli diretti di
// un componente non lascia il suo componente, ne' spostato su un altro ne' sganciato (la revoca
// dell'associazione): prima si revoca l'autorizzazione. Il rifiuto non scrive niente.
func TestIlDocumentoDiUnFileAutorizzatoRestaAlSuoComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaV3(t, "F5-DOC")
	sha := valoreR(s.rfqFascicolo, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.stpA)
	doc := uuidSQL(t, b, `SELECT documento_id FROM documento WHERE thread_id = $1 AND sha256 = $2`, s.thread, sha)
	for nome, form := range map[string]url.Values{
		"spostato":  {"componente": {s.prodotto.String()}, "documento": {doc.String()}, "scelta": {"aggiungi"}},
		"sganciato": {"componente": {""}, "documento": {doc.String()}},
	} {
		if a := s.assegna(s.w, form); !strings.Contains(a, "è lo STEP autorizzato a proporre i figli diretti di 77720517: prima si revoca l'autorizzazione") {
			t.Errorf("%s: %q", nome, a)
		}
	}
	if got := valoreR(s.rfqFascicolo, `SELECT (componente_id = $2)::text FROM documento WHERE documento_id = $1`, doc, s.assieme); got != "true" {
		t.Errorf("il documento ha lasciato il suo componente")
	}
}

// valoreR legge un valore dal database della RFQ di prova, come testo.
func valoreR(r *rfqFascicolo, sql string, arg ...any) string {
	r.b.t.Helper()
	var s string
	if err := r.b.pool.QueryRow(r.b.ctx, sql, arg...).Scan(&s); err != nil {
		r.b.t.Fatalf("%v\n%s", err, sql)
	}
	return s
}

// estraiGuida e' il paragrafo della guida nella pagina, per i messaggi delle prove.
func estraiGuida(pagina string) string {
	i := strings.Index(pagina, "guida-riga")
	if i < 0 {
		return "(nessuna riga della guida)"
	}
	return pagina[i:]
}

// datiEditorDi legge i dati dell'editor (GET .../bom/dati) per il prodotto, con il corpo JSON com'e'.
func datiEditorDi(t *testing.T, w *browser, r *rfqFascicolo, prodotto uuid.UUID) (datiEditor, string) {
	t.Helper()
	_, corpo := w.daFascicolo(http.MethodGet, r.base()+"/bom/dati?prodotto="+prodotto.String(), nil, r.thread, "?vista=bom")
	var d datiEditor
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatalf("bom/dati: %v", err)
	}
	return d, corpo
}

// L'editor mostra la guida profonda (decisioni del 27/09 ter, Domanda 1 = B: F5 non tronca e non nasconde il
// grafo profondo). Lo STEP autorizzato per 7120001 ha quattro livelli, 7120001 > 7120010 > 7120011 > 7120012:
// l'autorita' e' il primo (7120001 → 7120010, una carta dell'editor); gli archi di secondo e di terzo livello
// (7120010 → 7120011, 7120011 → 7120012) sono nella guida dell'editor, distinti, e nessun gesto li porta nella
// working: nessuna carta «p:», nessun arco proposto, nessun ritrovato, nessun id delle loro proposte nella
// risposta. Nessun «accetta guida»: per usarli si autorizza una sorgente per 7120010 (F5b).
func TestLEditorMostraLaGuidaProfonda(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "F5-PROFONDA")
	r.fase("FATTIBILITA")
	r.esegui(`UPDATE cliente SET regole = $2 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $1)`, r.thread,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`)
	prodotto := r.componenteTipo("7120001", "finito")
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	stp := r.stepNellaRfq("7120001A_1.stp", fattiDi("#1", []string{"#1=7120001", "#2=7120010", "#3=7120011", "#4=7120012"},
		[]string{"#1>#2*1", "#2>#3*2", "#3>#4*3"}), an)
	w := operatore(b)
	rianalizza := func() {
		t.Helper()
		_, html := w.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, "")
		if a := avvisoF(html); !strings.Contains(a, "1 STEP riletto") {
			t.Fatalf("rianalizza: %q", a)
		}
	}
	rianalizza()
	testutil.AutorizzaStep(t, b.pool, r.thread, prodotto, stp, r.utente)
	rianalizza()

	d, corpo := datiEditorDi(t, w, r, prodotto)
	guida := map[string]int32{}
	for _, g := range d.Guida {
		guida[g.File+":"+g.Padre+">"+g.Figlio] = g.Qta
	}
	if q, ok := guida["7120001A_1.stp:7120011>7120012"]; !ok || q != 3 {
		t.Errorf("l'arco di terzo livello 7120011 → 7120012 e' nella guida dell'editor: %+v", d.Guida)
	}
	if q, ok := guida["7120001A_1.stp:7120010>7120011"]; !ok || q != 2 {
		t.Errorf("l'arco di secondo livello 7120010 → 7120011 e' nella guida dell'editor: %+v", d.Guida)
	}
	if _, ok := guida["7120001A_1.stp:7120001>7120010"]; ok || len(d.Guida) != 2 {
		t.Errorf("l'arco dell'autorita' non e' guida: %+v", d.Guida)
	}
	// l'autorita': una carta per 7120010 e il suo arco dal prodotto
	carta := ""
	for ref, n := range d.Nodi {
		if strings.HasPrefix(ref, "p:") {
			if n.Codice != "7120010" || carta != "" {
				t.Errorf("solo il figlio diretto 7120010 e' una carta dell'editor: %s %+v", ref, n)
			}
			carta = ref
		}
	}
	if carta == "" || len(d.Proposti) != 1 || d.Proposti[0].Padre != refC(prodotto) || d.Proposti[0].Figlio != carta {
		t.Errorf("l'arco dell'autorita' 7120001 → 7120010: carta %q, proposti %+v", carta, d.Proposti)
	}
	if len(d.Ritrovati) != 0 {
		t.Errorf("nessun ritrovato: %+v", d.Ritrovati)
	}
	for _, k := range []string{"#3", "#4"} {
		id := valoreR(r, `SELECT proposta_id::text FROM componente_proposta WHERE allegato_id = $1 AND chiave = $2`, stp, k)
		if strings.Contains(corpo, id) {
			t.Errorf("la risposta porta l'id della proposta del nodo di guida %s: un gesto la potrebbe accettare", k)
		}
		for _, p := range d.Proposti {
			if p.FK == k || p.PK == k {
				t.Errorf("un arco della guida fra le proposte dell'editor: %+v", p)
			}
		}
	}
	// e la guida resta tutta nelle proposte: nessuna riga si e' chiusa o decisa
	if got := valoreR(r, `SELECT string_agg(chiave || ':' || stato, ' ' ORDER BY chiave) FROM componente_proposta WHERE allegato_id = $1`, stp); got != "#1:duplicato #2:aperta #3:aperta #4:aperta" {
		t.Errorf("le proposte del file: %s", got)
	}
}

// Una carta dell'editor e' di un file solo (A5.4.7: le riconciliazioni no). Due STEP autorizzati, per 7120010 e
// per 7120012, hanno tutti e due il figlio diretto 7120099, che non e' ancora un componente: l'editor ne fa due
// carte, una per file, e ciascuna dice il suo file. Prima era una carta sola, con il file della prima
// proposta, e accettarla decideva anche la proposta dell'altro file per uguaglianza di codice.
func TestLoStessoCodiceInDueFileSonoDueCarte(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "F5-CARTE")
	r.fase("FATTIBILITA")
	r.esegui(`UPDATE cliente SET regole = $2 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $1)`, r.thread,
		`{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`)
	prodotto := r.componenteTipo("7120001", "finito")
	c10 := r.componenteTipo("7120010", "sottoassieme")
	c12 := r.componenteTipo("7120012", "sottoassieme")
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	s10 := r.stepNellaRfq("7120010.stp", fattiDi("#1", []string{"#1=7120010", "#2=7120099"}, []string{"#1>#2*1"}), an)
	s12 := r.stepNellaRfq("7120012.stp", fattiDi("#1", []string{"#1=7120012", "#2=7120099"}, []string{"#1>#2*4"}), an)
	w := operatore(b)
	rianalizza := func() {
		t.Helper()
		_, html := w.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, "")
		if a := avvisoF(html); !strings.Contains(a, "STEP riletti") {
			t.Fatalf("rianalizza: %q", a)
		}
	}
	rianalizza()
	testutil.AutorizzaStep(t, b.pool, r.thread, c10, s10, r.utente)
	testutil.AutorizzaStep(t, b.pool, r.thread, c12, s12, r.utente)
	rianalizza()

	d, _ := datiEditorDi(t, w, r, prodotto)
	carte := map[string]string{} // file → carta
	for ref, n := range d.Nodi {
		if strings.HasPrefix(ref, "p:") && n.Codice == "7120099" {
			carte[n.File] = ref
		}
	}
	if len(carte) != 2 || carte["7120010.stp"] == "" || carte["7120012.stp"] == "" || carte["7120010.stp"] == carte["7120012.stp"] {
		t.Fatalf("due carte 7120099, una per file: %+v", carte)
	}
	archi := map[string]string{}
	for _, p := range d.Proposti {
		archi[p.Padre] = p.Figlio
	}
	if archi[refC(c10)] != carte["7120010.stp"] || archi[refC(c12)] != carte["7120012.stp"] {
		t.Errorf("ogni arco dell'autorita' va alla carta del suo file: %+v, carte %+v", d.Proposti, carte)
	}
}
