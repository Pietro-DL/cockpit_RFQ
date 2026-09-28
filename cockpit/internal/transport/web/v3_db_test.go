//go:build integrazione

// L4 — Fascicolo v3: le rotte nuove sul database vero. Le note sui disegni (0021): dove stanno, chi le
// cambia, i rifiuti con le loro parole e i CHECK della tabella; la struttura confermata dall'editor in un
// gesto (bom/applica): prendere la proposta dello STEP, spostare, condividere, togliere, cambiare quantita',
// i componenti scritti dall'operatore (la carta «n:», Smistamento U4), gli scarti, le rimozioni tenute, e i
// rifiuti che non lasciano niente scritto (fra cui le carte «k:» e «E' il prodotto», tolte dallo Smistamento
// F2); un file che entra senza componente; la sezione pigra del pannello e i dati dell'editor.
//
// Smistamento F5 (A5.4, A5.3.11): l'editor lavora sull'autorita'. Gli STEP delle scene sono autorizzati per il
// loro componente (autorizza, qui sotto, con testutil.AutorizzaStep), e l'editor prende solo i figli diretti di
// un file autorizzato; il resto e' guida, si vede e non entra.

package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/testutil"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
)

// fattiV3: uno STEP con una radice senza codice (il nome del CAD, che e' il prodotto), l'assieme 77720517 ×2
// sotto di lei e un pezzo nuovo, 77811111 ×2, sotto l'assieme.
const fattiV3 = `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [],
	"nodi": [{"chiave": "#1", "id_grezzo": "TOP-ASM", "nome_grezzo": "TOP-ASM", "evidenza": {}},
	         {"chiave": "#2", "id_grezzo": "77720517", "nome_grezzo": "77720517", "evidenza": {}},
	         {"chiave": "#3", "id_grezzo": "77811111", "nome_grezzo": "77811111", "evidenza": {}}],
	"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}, {"padre": "#2", "figlio": "#3", "qta": 2, "evidenza": {}}],
	"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	"occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`

// fattiV3A: lo STEP dell'assieme 77720517, con il pezzo nuovo 77811111 ×2 suo figlio diretto.
var fattiV3A = fattiDi("#1", []string{"#1=77720517", "#3=77811111"}, []string{"#1>#3*2"})

// scenaV3 e' la scena di B8.7 con lo STEP dell'assieme gia' letto: le sue tre proposte di nodo.
//
// Riscritta per lo Smistamento (R8, fase F1): prima le proposte nascevano aprendo il Fascicolo (la GET
// rileggeva lo STEP). Adesso nascono con il gesto esplicito «Rianalizza», e la scena controlla che la GET
// da sola non ne scriva.
//
// Riscritta ancora per F5 (A5.4, Domanda 1 = B): lo STEP «assieme.stp» (radice senza codice, l'assieme, il pezzo nuovo sotto
// l'assieme) e' autorizzato per il prodotto: la sua radice e' il prodotto, l'assieme e' un figlio diretto, e il
// pezzo nuovo sotto l'assieme e' guida. Il pezzo nuovo come proposta dell'autorita' arriva dallo STEP
// dell'assieme, «77720517.stp», autorizzato per l'assieme: s.nuovo e' il suo nodo 77811111. L'autorita' si
// compone livello per livello.
type scenaV3 struct {
	*scenaB87
	w                 *browser
	stp, stpA         uuid.UUID // gli allegati STEP: del prodotto e dell'assieme
	top, nodo2, nuovo uuid.UUID // le proposte #1 (senza codice) e #2 (77720517) di stp, #3 (77811111) di stpA
	nuovoGuida        uuid.UUID // #3 (77811111) di stp: sotto l'assieme nel file del prodotto, guida
}

func (b *bancoWeb) scenaV3(t *testing.T, chiave string) *scenaV3 {
	t.Helper()
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	s := &scenaV3{scenaB87: b.scenaB87(chiave), w: operatore(b)}
	s.stp = s.stepNellaRfq("assieme.stp", fattiV3, an)
	s.stpA = s.stepNellaRfq("77720517.stp", fattiV3A, an)
	s.rilegge(t, s.w, s.stp)
	testutil.AutorizzaStep(t, b.pool, s.thread, s.prodotto, s.stp, s.utente)
	s.autorizza(t, s.w, s.stpA, s.assieme)
	prop := func(a uuid.UUID, k string) uuid.UUID {
		return uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = $2`, a, k)
	}
	s.top, s.nodo2, s.nuovoGuida, s.nuovo = prop(s.stp, "#1"), prop(s.stp, "#2"), prop(s.stp, "#3"), prop(s.stpA, "#3")
	return s
}

// autorizza: il file e' autorizzato a proporre i figli diretti del componente (Smistamento F5; l'aiuto comune
// testutil.AutorizzaStep scrive la marcatura, e per un finito lo STEP strutturale), e si rilegge con
// «Rianalizza» come fara' il gesto. Le righe del file devono esserci.
func (s *scenaB87) autorizza(t *testing.T, w *browser, allegato, comp uuid.UUID) {
	t.Helper()
	testutil.AutorizzaStep(t, s.b.pool, s.thread, comp, allegato, s.utente)
	if a := s.gesto(w, s.base()+"/rianalizza", nil); !strings.Contains(a, "STEP riletto") && !strings.Contains(a, "STEP riletti") {
		t.Fatalf("rianalizza dopo l'autorizzazione: %q", a)
	}
}

func refC(id uuid.UUID) string { return "c:" + id.String() }
func refP(id uuid.UUID) string { return "p:" + id.String() }

// strutturaDi e' la struttura di adesso sotto la radice, come l'editor la manda senza toccare niente: gli
// archi attivi della RFQ sono voluti e visti.
func (r *rfqFascicolo) strutturaDi(radice uuid.UUID) fascicolo.StrutturaVoluta {
	r.b.t.Helper()
	v := fascicolo.StrutturaVoluta{Radice: radice}
	righe, err := r.b.pool.Query(r.b.ctx, `SELECT padre_id, figlio_id, qta FROM componente_relazione WHERE thread_id = $1 ORDER BY padre_id, figlio_id`, r.thread)
	if err != nil {
		r.b.t.Fatal(err)
	}
	defer righe.Close()
	for righe.Next() {
		var p, f uuid.UUID
		var q int32
		if err := righe.Scan(&p, &f, &q); err != nil {
			r.b.t.Fatal(err)
		}
		a := fascicolo.ArcoVoluto{Padre: refC(p), Figlio: refC(f), Qta: q}
		v.Archi = append(v.Archi, a)
		v.Visti = append(v.Visti, a)
	}
	return v
}

// senza toglie dalla struttura voluta l'arco padre → figlio.
func senza(v fascicolo.StrutturaVoluta, padre, figlio uuid.UUID) fascicolo.StrutturaVoluta {
	var out []fascicolo.ArcoVoluto
	for _, a := range v.Archi {
		if a.Padre != refC(padre) || a.Figlio != refC(figlio) {
			out = append(out, a)
		}
	}
	v.Archi = out
	return v
}

// applica manda la struttura voluta dalla Struttura BOM; restituisce l'avviso, l'evento per l'editor e lo
// stato HTTP.
func (r *rfqFascicolo) applica(w *browser, v fascicolo.StrutturaVoluta) (string, string, int) {
	r.b.t.Helper()
	j, err := json.Marshal(v)
	if err != nil {
		r.b.t.Fatal(err)
	}
	resp, html := w.daFascicolo(http.MethodPost, r.base()+"/bom/applica", url.Values{"struttura": {string(j)}}, r.thread, "?vista=bom")
	return avvisoF(html), resp.Header.Get("HX-Trigger"), resp.StatusCode
}

// applicaConRadici manda la struttura voluta con il campo di prima «radici_proposte» («E' il prodotto»):
// dal F5 il campo non e' piu' nel core (StrutturaVoluta), e la rotta rifiuta chi lo manda ancora.
func (r *rfqFascicolo) applicaConRadici(w *browser, v fascicolo.StrutturaVoluta, radici ...uuid.UUID) (string, string, int) {
	r.b.t.Helper()
	j, err := json.Marshal(v)
	if err != nil {
		r.b.t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(j, &m); err != nil {
		r.b.t.Fatal(err)
	}
	m["radici_proposte"] = radici
	if j, err = json.Marshal(m); err != nil {
		r.b.t.Fatal(err)
	}
	resp, html := w.daFascicolo(http.MethodPost, r.base()+"/bom/applica", url.Values{"struttura": {string(j)}}, r.thread, "?vista=bom")
	return avvisoF(html), resp.Header.Get("HX-Trigger"), resp.StatusCode
}

// esitoEditor legge l'evento bom-esito dell'intestazione HX-Trigger.
func esitoEditor(t *testing.T, h string) (bool, string) {
	t.Helper()
	for _, r := range h {
		if r > 127 {
			t.Errorf("HX-Trigger non e' ASCII: %q", h)
			break
		}
	}
	var ev struct {
		Esito struct {
			Ok    bool   `json:"ok"`
			Testo string `json:"testo"`
		} `json:"bom-esito"`
	}
	if err := json.Unmarshal([]byte(h), &ev); err != nil {
		t.Fatalf("HX-Trigger non si legge (%v): %q", err, h)
	}
	return ev.Esito.Ok, ev.Esito.Testo
}

// congelaAMano congela la working com'e' adesso (la V1, senza istantanee: basta al muro BOM01).
func (r *rfqFascicolo) congelaAMano() {
	r.b.t.Helper()
	r.esegui(`INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		SELECT $1, 1, 'preventivo', nome_fase, fase_log_id, 'prova', $2 FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, r.thread, r.utente)
	r.esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE thread_id = $1`, r.thread, r.utente)
}

// ---------------------------------------------------------------- l'editor: la proposta dello STEP

// La proposta dello STEP presa nell'editor: 77811111 nasce sotto l'assieme con la sua quantita', la
// proposta dell'arco e' presa, l'evento per l'editor dice che e' fatto. La radice senza codice resta una
// proposta aperta, con il suo arco: non e' il prodotto per un gesto dell'editor.
//
// Riscritta per lo Smistamento (R5, fase F2): prima la radice senza codice era detta «il prodotto» (E' il
// prodotto, RadiciProposte): diventava duplicato del prodotto, il suo arco verso l'assieme un duplicato
// dell'arco della BOM, e il banner dello STEP spariva. Quel gesto e' tolto (la radice la fissa lo STEP
// strutturale): la radice e il suo arco restano aperti, e lo STEP resta da guardare.
//
// Riscritta ancora per F5 (A5.4, Domanda 1 = B): la radice di «assieme.stp» e' il prodotto perche' il file e' autorizzato
// per lui (la marcatura, con chi l'ha decisa), non per un gesto dell'editor; l'assieme, suo figlio diretto,
// resta aperto (il codice uguale non lo aggancia) e 77811111, sotto l'assieme in quel file, e' guida. La
// proposta presa nell'editor e' 77811111 figlio diretto dello STEP dell'assieme, autorizzato per l'assieme; l'arco
// della guida resta com'era.
func TestLEditorPrendeLaPropostaDelloStep(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaV3(t, "ED1")
	if got := s.valore(`SELECT string_agg(chiave || ':' || coalesce(codice, '-') || ':' || stato::text, ' ' ORDER BY chiave) FROM componente_proposta WHERE allegato_id = $1`, s.stp); got != "#1:-:duplicato #2:77720517:aperta #3:77811111:aperta" {
		t.Fatalf("le proposte dello STEP: %s", got)
	}
	_, pagina := s.w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if !strings.Contains(pagina, `id="step-`+s.stpA.String()+`"`) || !strings.Contains(pagina, "Apri proposta BOM") {
		t.Fatal("la Struttura BOM non offre la proposta dello STEP dell'assieme")
	}

	v := s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refP(s.nuovo), Qta: 2})
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: s.stpA, Padre: "#1", Figlio: "#3"}}
	avviso, evento, stato := s.applica(s.w, v)
	if stato != 200 || avviso != "Struttura di 77722757 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Fatalf("editor: %d %q", stato, avviso)
	}
	if ok, testo := esitoEditor(t, evento); !ok || testo != avviso {
		t.Errorf("l'evento per l'editor: %v %q", ok, testo)
	}
	if got := s.archi(); got != "77720517>77811111x2 77720517>77817189x4 77722757>77720517x2 77722757>77817189x1" {
		t.Errorf("archi: %s", got)
	}
	if got := s.valore(`SELECT tipo::text || '/' || origine::text FROM componente WHERE thread_id = $1 AND codice = '77811111'`, s.thread); !strings.HasPrefix(got, "sciolto/") {
		t.Errorf("77811111 nasce particolare: %s", got)
	}
	if got := s.valore(`SELECT string_agg(chiave || ':' || stato::text || ':' || (componente_id = $2)::text || ':' || (deciso_da IS NOT NULL)::text, ' ' ORDER BY chiave)
		FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#1'`, s.stp, s.prodotto); got != "#1:duplicato:true:true" {
		t.Errorf("la radice dello STEP e' il prodotto per l'autorizzazione di una persona: %s", got)
	}
	if got := s.valore(`SELECT string_agg(padre_chiave || '>' || figlio_chiave || ':' || stato::text, ' ' ORDER BY padre_chiave) FROM relazione_proposta WHERE allegato_id = $1`, s.stp); got != "#1>#2:aperta #2>#3:aperta" {
		t.Errorf("le proposte di arco del file del prodotto: %s", got)
	}
	if got := s.valore(`SELECT string_agg(padre_chiave || '>' || figlio_chiave || ':' || stato::text, ' ' ORDER BY padre_chiave) FROM relazione_proposta WHERE allegato_id = $1`, s.stpA); got != "#1>#3:confermata" {
		t.Errorf("la proposta di arco presa: %s", got)
	}
	_, pagina = s.w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if !strings.Contains(pagina, `id="step-`+s.stp.String()+`"`) {
		t.Error("con l'assieme ancora da decidere lo STEP del prodotto resta da guardare: il banner c'e'")
	}
}

// ---------------------------------------------------------------- l'editor: spostare, condividere, togliere

// Senza STEP l'operatore sistema la struttura a mano: spostare toglie l'arco vecchio, un secondo padre e' un
// arco voluto in piu', la quantita' cambia sull'arco, togliere tutto rimanda il pezzo fra quelli da
// sistemare, un particolare che riceve un figlio diventa assieme. Ogni conferma e' una transazione e dice che
// cosa e' cambiato.
func TestLEditorSpostaCondivideTogli(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ED2")
	w := operatore(b)
	passo := func(nome string, v fascicolo.StrutturaVoluta, avvisoAtteso, archiAttesi string) {
		t.Helper()
		a, ev, _ := s.applica(w, v)
		if a != avvisoAtteso {
			t.Fatalf("%s: %q", nome, a)
		}
		if ok, _ := esitoEditor(t, ev); !ok {
			t.Errorf("%s: l'evento dice rifiutato", nome)
		}
		if got := s.archi(); got != archiAttesi {
			t.Fatalf("%s: archi %s", nome, got)
		}
	}
	passo("sposta", senza(s.strutturaDi(s.prodotto), s.assieme, s.particolare),
		"Struttura di 77722757 confermata: 1 legame tolto.", "77722757>77720517x2 77722757>77817189x1")

	v := s.strutturaDi(s.prodotto)
	for i := range v.Archi {
		if v.Archi[i].Figlio == refC(s.particolare) {
			v.Archi[i].Qta = 5
		}
	}
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refC(s.particolare), Qta: 3})
	passo("condividi e quantita'", v, "Struttura di 77722757 confermata: 1 legame aggiunto, 1 quantità cambiata.",
		"77720517>77817189x3 77722757>77720517x2 77722757>77817189x5")

	passo("niente", s.strutturaDi(s.prodotto), "Nessun cambiamento: la struttura di 77722757 è già così.",
		"77720517>77817189x3 77722757>77720517x2 77722757>77817189x5")

	v = senza(senza(s.strutturaDi(s.prodotto), s.assieme, s.particolare), s.prodotto, s.particolare)
	passo("togli", v, "Struttura di 77722757 confermata: 2 legami tolti. Fuori dalla struttura e senza padre, da sistemare: 77817189.",
		"77722757>77720517x2")

	vite := s.componenteTipo("54000000", "sciolto")
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refC(s.particolare), Qta: 1},
		fascicolo.ArcoVoluto{Padre: refC(s.particolare), Figlio: refC(vite), Qta: 2})
	passo("un particolare con un figlio", v, "Struttura di 77722757 confermata: 2 legami aggiunti, 1 particolare diventa assieme.",
		"77720517>77817189x1 77722757>77720517x2 77817189>54000000x2")
	if got := s.valore(`SELECT tipo::text FROM componente WHERE componente_id = $1`, s.particolare); got != "sottoassieme" {
		t.Errorf("77817189 con un figlio: %s", got)
	}
}

// ---------------------------------------------------------------- l'editor: i rifiuti

// Una struttura disegnata su dati vecchi, un ciclo, la BOM congelata, un JSON rotto, una radice che non e'
// un prodotto, chi consulta: niente scritto, e l'editor lo sa (l'evento dice rifiutato, in ASCII).
func TestLEditorRifiutaSenzaScrivereNiente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaV3(t, "ED3")
	foto := func() string {
		return s.archi() + " | " + s.valore(`SELECT string_agg(chiave || ':' || stato::text, ' ' ORDER BY chiave) FROM componente_proposta WHERE allegato_id = $1`, s.stp) +
			" | " + s.valore(`SELECT string_agg(codice, ' ' ORDER BY codice) FROM componente WHERE thread_id = $1`, s.thread) +
			" | " + s.valore(`SELECT string_agg(stato::text, ' ' ORDER BY padre_chiave) FROM relazione_proposta WHERE allegato_id = $1`, s.stp)
	}
	prima := foto()
	rifiuto := func(nome string, v fascicolo.StrutturaVoluta, pezzo string, radici ...uuid.UUID) {
		t.Helper()
		a, ev, _ := s.applica(s.w, v)
		if len(radici) > 0 {
			a, ev, _ = s.applicaConRadici(s.w, v, radici...)
		}
		if !strings.HasPrefix(a, "Niente è cambiato: ") || !strings.Contains(a, pezzo) {
			t.Errorf("%s: %q", nome, a)
		}
		if ok, testo := esitoEditor(t, ev); ok || testo != a {
			t.Errorf("%s: l'evento per l'editor %v %q", nome, ok, testo)
		}
		if got := foto(); got != prima {
			t.Fatalf("%s ha scritto qualcosa:\n  prima %s\n  dopo  %s", nome, prima, got)
		}
	}

	// l'editor non mostrava l'arco assieme → particolare (un collega l'ha messo nel frattempo)
	v := s.strutturaDi(s.prodotto)
	v = senza(v, s.assieme, s.particolare)
	var visti []fascicolo.ArcoVoluto
	for _, a := range v.Visti {
		if a.Padre != refC(s.assieme) {
			visti = append(visti, a)
		}
	}
	v.Visti = visti
	rifiuto("dati vecchi", v, "77720517 ha sotto 77817189, che l'editor non mostrava (un altro gesto nel frattempo): riapri l'editor")

	// tanti cambiamenti buoni e un ciclo in fondo: niente. Riscritta per lo Smistamento (F2): prima fra i
	// cambiamenti buoni c'era «E' il prodotto» (RadiciProposte), che adesso si rifiuta da se' (la prova qui sotto)
	v = s.strutturaDi(s.prodotto)
	v.Codici = map[string]fascicolo.CodiceScritto{}
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refP(s.nuovo), Qta: 2},
		fascicolo.ArcoVoluto{Padre: refP(s.nuovo), Figlio: refC(s.assieme), Qta: 1})
	rifiuto("ciclo", v, "la struttura chiuderebbe un ciclo")

	// «E' il prodotto»: il campo di prima, che la rotta rifiuta (dal F5 non e' piu' nel core)
	rifiuto("E' il prodotto", s.strutturaDi(s.prodotto), "«È il prodotto» non c'è più", s.top)
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: "k:77811111", Qta: 1})
	rifiuto("carta k:", v, "77811111: un codice trovato nella RFQ non diventa un componente dall'editor")

	v = s.strutturaDi(s.assieme)
	rifiuto("radice non prodotto", v, "77720517 non è un prodotto finito")

	altra := b.rfqFascicolo(b.clienteDiProva("BETA", "Beta S.r.l.", "beta.example"), "ED3-ALTRA")
	prodottoAltro := altra.componenteTipo("77799999", "finito")
	rifiuto("radice di un'altra RFQ", s.strutturaDi(prodottoAltro), "la radice della struttura non è un componente di questa RFQ")
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refC(prodottoAltro), Qta: 1})
	rifiuto("componente di un'altra RFQ", v, "un componente della struttura non è di questa RFQ")

	_, html := s.w.daFascicolo(http.MethodPost, s.base()+"/bom/applica", url.Values{"struttura": {`{"radice": `}}, s.thread, "?vista=bom")
	if a := avvisoF(html); a != "Niente è cambiato: la struttura mandata non si legge: riapri l'editor" {
		t.Errorf("JSON rotto: %q", a)
	}

	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	j, _ := json.Marshal(s.strutturaDi(s.prodotto))
	if resp, _ := co.daFascicolo(http.MethodPost, s.base()+"/bom/applica", url.Values{"struttura": {string(j)}}, s.thread, "?vista=bom"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("consultazione: %d", resp.StatusCode)
	}

	s.congelaAMano()
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refP(s.nuovo), Qta: 2})
	rifiuto("BOM congelata", v, "congelata")
	if got := foto(); got != prima {
		t.Fatalf("dopo i rifiuti: %s", got)
	}
}

// ---------------------------------------------------------------- l'editor: componenti scritti, scarti, rimozioni

// La struttura a mano con un codice trovato nella RFQ (la carta «k:» si rifiuta; il componente nasce solo
// scritto, con la carta «n:»), un codice quasi uguale al prodotto (rifiuto senza la conferma), un nodo dello
// STEP scartato con l'arco che lo tocca, «E' il prodotto» rifiutato, e una rimozione proposta dallo STEP
// strutturale su un arco che l'operatore tiene: si chiude.
//
// Riscritta per lo Smistamento (R1, R5, U4, fase F2): prima era TestLEditorCodiciTrovatiScartiERimozioni e
// fissava che il codice trovato 77899999 nascesse dalla carta «k:» (sciolto, codice_rilevato, con la rev C
// delle evidenze), che un «k:» non trovato si rifiutasse, e che la radice dello STEP detta «il prodotto»
// facesse del suo arco un duplicato. Adesso il «k:» si rifiuta anche per il codice trovato; lo stesso codice
// scritto nasce con origine manuale e la revisione scritta; «E' il prodotto» si rifiuta e l'arco resta aperto.
//
// Riscritta ancora per F5 (correzione: contano, e si tengono, solo le rimozioni di un'autorizzazione valida):
// prima la rimozione tenuta veniva da un altro STEP scelto come strutturale con il gesto, senza nessuna riga
// che ne facesse un'autorizzazione valida (e scegliendolo, l'autorizzazione di «assieme.stp» diventava
// incoerente). Adesso la rimozione viene dal file autorizzato per il prodotto, che e' anche il suo STEP
// strutturale (I10), e il resto e' com'era.
func TestLEditorComponentiScrittiScartiERimozioni(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaV3(t, "ED4")
	s.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
		VALUES ($1, '77899999', 'prodotto', 'C', 'generico', '', 30, 'corpo')`, s.msg)

	v := s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.prodotto), Figlio: "k:77899999", Qta: 1})
	if a, _, _ := s.applica(s.w, v); a != "Niente è cambiato: 77899999: un codice trovato nella RFQ non diventa un componente dall'editor. Se è un pezzo della distinta, scrivine il codice con «+ Componente con codice»" {
		t.Fatalf("codice trovato: %q", a)
	}
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.prodotto), Figlio: "n:77899999", Qta: 1})
	v.Nuovi = []fascicolo.ComponenteNuovo{{Codice: "77899999", Tipo: "sciolto", Rev: "d"}}
	if a, _, _ := s.applica(s.w, v); a != "Struttura di 77722757 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Fatalf("componente scritto: %q", a)
	}
	if got := s.valore(`SELECT tipo::text || '/' || origine::text || '/' || coalesce(rev, '-') FROM componente WHERE thread_id = $1 AND codice = '77899999'`, s.thread); got != "sciolto/manuale/D" {
		t.Errorf("il componente scritto: %s", got)
	}
	v = s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.prodotto), Figlio: "n:77722757A", Qta: 1})
	v.Nuovi = []fascicolo.ComponenteNuovo{{Codice: "77722757A", Tipo: "sciolto"}}
	if a, _, _ := s.applica(s.w, v); a != "Niente è cambiato: 77722757A è quasi uguale a 77722757 (lettera finale): se è lo stesso pezzo si usa quello; se è un pezzo diverso togli la carta di 77722757A e riscrivila con «+ Componente con codice», confermando che è un pezzo diverso" {
		t.Errorf("codice quasi uguale: %q", a)
	}

	// (Smistamento F5: il nodo 77811111 e il suo arco sono quelli dello STEP dell'assieme, autorizzato)
	v = s.strutturaDi(s.prodotto)
	v.Scarta = []uuid.UUID{s.nuovo}
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: s.stpA, Padre: "#1", Figlio: "#3"}}
	if a, _, _ := s.applica(s.w, v); a != "Struttura di 77722757 confermata: 1 nodo proposto scartato, 1 proposta di legame chiusa." {
		t.Fatalf("scarto: %q", a)
	}
	if got := s.valore(`SELECT stato::text || ' ' || coalesce(nota, '') FROM relazione_proposta WHERE allegato_id = $1 AND figlio_chiave = '#3'`, s.stpA); got != "scartata nodo scartato nella struttura: 77811111" {
		t.Errorf("l'arco verso il nodo scartato: %q", got)
	}

	v = s.strutturaDi(s.prodotto)
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: s.stp, Padre: "#1", Figlio: "#2"}}
	if a, _, _ := s.applicaConRadici(s.w, v, s.top); !strings.HasPrefix(a, "Niente è cambiato: «È il prodotto» non c'è più") {
		t.Fatalf("radice: %q", a)
	}
	if got := s.valore(`SELECT stato::text FROM relazione_proposta WHERE allegato_id = $1 AND padre_chiave = '#1'`, s.stp); got != "aperta" {
		t.Errorf("l'arco della radice resta aperto: %s", got)
	}

	// lo STEP autorizzato per il prodotto (e suo STEP strutturale) propone di togliere prodotto → assieme;
	// l'operatore lo tiene
	docStp := uuidSQL(t, b, `SELECT d.documento_id FROM documento d JOIN allegato a ON a.sha256 = d.sha256 WHERE d.thread_id = $1 AND a.allegato_id = $2`, s.thread, s.stp)
	if got := s.valore(`SELECT (step_strutturale_id = $2)::text FROM componente WHERE componente_id = $1`, s.prodotto, docStp); got != "true" {
		t.Fatalf("assieme.stp e' lo STEP strutturale del prodotto: %s", got)
	}
	s.esegui(`INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 2)`,
		s.thread, docStp, s.prodotto, s.assieme)
	if a, _, _ := s.applica(s.w, s.strutturaDi(s.prodotto)); a != "Struttura di 77722757 confermata: 1 rimozione proposta dallo STEP chiusa: il legame resta." {
		t.Fatalf("rimozione tenuta: %q", a)
	}
	if got := s.valore(`SELECT stato::text || ' ' || coalesce(nota, '') FROM rimozione_proposta WHERE thread_id = $1`, s.thread); got != "scartata tenuto nella struttura confermata" {
		t.Errorf("la rimozione: %q", got)
	}
	if got := s.archi(); !strings.Contains(got, "77722757>77720517x2") {
		t.Errorf("l'arco tenuto: %s", got)
	}
}

// ---------------------------------------------------------------- le note sui disegni

// Una nota sta su un punto di una pagina di un PDF della RFQ, con o senza componente; si scrive anche sulla
// BOM congelata; la cambia e la toglie solo chi l'ha scritta; chi consulta non scrive. I rifiuti dicono
// perche', e i CHECK della 0021 tengono anche senza il server.
func TestLeNoteSuiDisegni(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("NOTE")
	w := operatore(b)
	nota := func(w *browser, f url.Values) string {
		t.Helper()
		return s.gesto(w, s.base()+"/nota", f)
	}
	buona := func() url.Values {
		return url.Values{"componente": {s.particolare.String()}, "allegato": {s.allLibero.String()}, "pagina": {"1"}, "x": {"0.3"}, "y": {"0.4"},
			"testo": {"Tolleranza da verificare sul foro"}}
	}
	if a := nota(w, buona()); a != "Nota aggiunta su 77817189.pdf, pagina 1 (77817189)." {
		t.Fatalf("nota: %q", a)
	}
	if got := s.valore(`SELECT pagina || ' ' || x_norm || ' ' || y_norm || ' ' || testo || ' ' || (componente_id = $2)::text || ' ' || (allegato_id = $3)::text
		FROM annotazione_pdf WHERE thread_id = $1`, s.thread, s.particolare, s.allLibero); got != "1 0.300000 0.400000 Tolleranza da verificare sul foro true true" {
		t.Errorf("la nota nel database: %s", got)
	}
	f := buona()
	f.Del("componente")
	f.Set("x", "1")
	f.Set("y", "0")
	if a := nota(w, f); a != "Nota aggiunta su 77817189.pdf, pagina 1 (senza componente)." {
		t.Errorf("nota senza componente, sul bordo del foglio: %q", a)
	}

	// i rifiuti
	altra := b.rfqFascicolo(b.clienteDiProva("BETA", "Beta S.r.l.", "beta.example"), "NOTE-ALTRA")
	fileAltro, _ := altra.allegatoExt("altro.pdf", "pdf")
	compAltro := altra.componente("77799999")
	senzaImpronta, _ := s.allegatoExt("senza impronta.pdf", "pdf")
	s.esegui(`UPDATE allegato SET sha256 = NULL WHERE allegato_id = $1`, senzaImpronta)
	casi := []struct {
		nome, campo, valore, pezzo string
	}{
		{"x NaN", "x", "NaN", "non è sul foglio (x)"},
		{"x infinito", "x", "Inf", "non è sul foglio (x)"},
		{"x negativo", "x", "-0.1", "non è sul foglio (x)"},
		{"x oltre il foglio", "x", "1.0001", "non è sul foglio (x)"},
		{"x non numero", "x", "trenta", "non è sul foglio (x)"},
		{"y mancante", "y", "", "non è sul foglio (y)"},
		{"pagina zero", "pagina", "0", "la pagina della nota non è valida"},
		{"pagina troppo alta", "pagina", "10001", "la pagina della nota non è valida"},
		{"testo vuoto", "testo", "   ", "la nota è vuota"},
		{"testo troppo lungo", "testo", strings.Repeat("è", MaxTestoNota+1), "più di 2000 caratteri"},
		{"file di un'altra RFQ", "allegato", fileAltro.String(), "il file non è di questa RFQ"},
		{"componente di un'altra RFQ", "componente", compAltro.String(), "il componente non è di questa RFQ"},
		{"uno STEP", "allegato", s.allStep.String(), "le note si mettono su un PDF già scaricato"},
		{"un PDF senza impronta", "allegato", senzaImpronta.String(), "le note si mettono su un PDF già scaricato"},
		{"file non valido", "allegato", "boh", "Niente è cambiato"},
	}
	for _, c := range casi {
		f := buona()
		f.Set(c.campo, c.valore)
		if a := nota(w, f); !strings.HasPrefix(a, "Niente è cambiato: ") || !strings.Contains(a, c.pezzo) {
			t.Errorf("%s: %q", c.nome, a)
		}
	}
	if n := s.conta(`SELECT count(*) FROM annotazione_pdf`); n != 2 {
		t.Fatalf("i rifiuti hanno scritto delle note: %d", n)
	}
	// duemila caratteri si', anche se in byte sono di piu'
	f = buona()
	f.Set("testo", strings.Repeat("è", MaxTestoNota))
	if a := nota(w, f); !strings.HasPrefix(a, "Nota aggiunta") {
		t.Errorf("duemila caratteri accentati: %q", a)
	}

	// chi l'ha scritta la cambia e la toglie; un altro no
	id := s.valore(`SELECT annotazione_id::text FROM annotazione_pdf WHERE testo = 'Tolleranza da verificare sul foro' AND componente_id IS NOT NULL`)
	lu := b.browser("10.0.0.2")
	lu.login("LU", "prova-lu")
	if a := s.gesto(lu, s.base()+"/nota/"+id+"/modifica", url.Values{"testo": {"scritta da un altro"}}); !strings.Contains(a, "la cambia solo chi l'ha scritta") {
		t.Errorf("un altro la cambia: %q", a)
	}
	if a := s.gesto(lu, s.base()+"/nota/"+id+"/elimina", nil); !strings.Contains(a, "la toglie solo chi l'ha scritta") {
		t.Errorf("un altro la toglie: %q", a)
	}
	if a := s.gesto(w, s.base()+"/nota/"+id+"/modifica", url.Values{"testo": {"  "}}); !strings.Contains(a, "la nota è vuota") {
		t.Errorf("cambiata in vuota: %q", a)
	}
	if a := s.gesto(w, s.base()+"/nota/"+id+"/modifica", url.Values{"testo": {"</script><script>alert(1)</script> & <b>foro</b>"}}); a != "Nota cambiata." {
		t.Errorf("cambia: %q", a)
	}
	if got := s.valore(`SELECT (modificata_da = creata_da AND modificata_il IS NOT NULL)::text FROM annotazione_pdf WHERE annotazione_id = $1`, id); got != "true" {
		t.Errorf("chi l'ha cambiata: %s", got)
	}
	// il testo si mostra com'e', non si esegue: nel pannello e nell'indice del viewer
	_, pagina := w.fai(http.MethodGet, s.base()+"?nodo="+s.particolare.String()+"&file="+s.allLibero.String(), nil, false)
	if strings.Contains(pagina, "<script>alert(1)</script>") || strings.Contains(pagina, "<b>foro</b>") {
		t.Error("il testo di una nota entra nella pagina come HTML")
	}
	if !strings.Contains(pagina, "&lt;script&gt;alert(1)&lt;/script&gt; &amp; &lt;b&gt;foro&lt;/b&gt;") {
		t.Error("la nota non si vede nel pannello del file")
	}
	// chi consulta la legge e non scrive
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if resp, _ := co.daFascicolo(http.MethodPost, s.base()+"/nota", buona(), s.thread, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("consultazione, nuova nota: %d", resp.StatusCode)
	}
	if resp, _ := co.daFascicolo(http.MethodPost, s.base()+"/nota/"+id+"/elimina", url.Values{}, s.thread, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("consultazione, togli: %d", resp.StatusCode)
	}
	if a := s.gesto(w, s.base()+"/nota/"+id+"/elimina", nil); a != "Nota tolta." {
		t.Errorf("togli: %q", a)
	}
	if a := s.gesto(w, s.base()+"/nota/"+id+"/elimina", nil); !strings.Contains(a, "la nota non c'è più") {
		t.Errorf("togli due volte: %q", a)
	}
	// un componente con delle note non si cancella: si archivia
	vite := s.componenteTipo("54000000", "sciolto")
	f = buona()
	f.Set("componente", vite.String())
	nota(w, f)
	if a := s.gesto(w, s.comp(vite, "rimuovi"), nil); !strings.Contains(a, "ha delle note sui disegni") {
		t.Errorf("togliere un componente con delle note: %q", a)
	}
	// la BOM congelata non ferma le note: non sono struttura
	s.congelaAMano()
	if a := nota(w, buona()); !strings.HasPrefix(a, "Nota aggiunta") {
		t.Errorf("nota sulla BOM congelata: %q", a)
	}

	// i CHECK della tabella, senza il server
	for _, c := range []struct{ nome, sql string }{
		{"punto fuori dal foglio", `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da) VALUES ($1, $2, 1, 1.5, 0, 'x', $3)`},
		{"pagina zero", `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da) VALUES ($1, $2, 0, 0, 0, 'x', $3)`},
		{"testo vuoto", `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da) VALUES ($1, $2, 1, 0, 0, '   ', $3)`},
		{"testo troppo lungo", `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da) VALUES ($1, $2, 1, 0, 0, repeat('a', 2001), $3)`},
		{"modifica a meta'", `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da, modificata_da) VALUES ($1, $2, 1, 0, 0, 'x', $3, $3)`},
	} {
		if _, err := b.pool.Exec(b.ctx, c.sql, s.thread, s.allLibero, s.utente); err == nil {
			t.Errorf("CHECK %s: accettato", c.nome)
		}
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO annotazione_pdf (thread_id, componente_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da)
		VALUES ($1, $2, $3, 1, 0, 0, 'x', $4)`, s.thread, compAltro, s.allLibero, s.utente); err == nil {
		t.Error("FK: una nota con il componente di un'altra RFQ")
	}
}

// ---------------------------------------------------------------- un file senza componente

// «Documentazione generale» e «Capitolato» dal pannello: il file diventa un documento della RFQ senza
// componente, anche se la proposta era agganciata a uno. Un altro tipo, un file gia' deciso, un file di
// un'altra RFQ si rifiutano; chi consulta non lo fa.
func TestUnFileEntraSenzaComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaB87("GEN")
	w := operatore(b)
	generale := func(w *browser, pid uuid.UUID, tipo string) string {
		t.Helper()
		return s.gesto(w, s.base()+"/file/"+pid.String()+"/generale", url.Values{"tipo": {tipo}})
	}
	s.esegui(`UPDATE documento_proposta SET componente_id = $2 WHERE proposta_id = $1`, s.pdfLibero2, s.assieme)
	if a := generale(w, s.pdfLibero2, "disegno_2d"); !strings.Contains(a, "come documentazione generale o come capitolato") {
		t.Errorf("un altro tipo: %q", a)
	}
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if resp, _ := co.daFascicolo(http.MethodPost, s.base()+"/file/"+s.pdfLibero2.String()+"/generale", url.Values{"tipo": {"altro"}}, s.thread, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("consultazione: %d", resp.StatusCode)
	}
	if a := generale(w, s.pdfLibero2, "altro"); !strings.HasPrefix(a, "Confermato") {
		t.Fatalf("documentazione generale: %q", a)
	}
	if got := s.valore(`SELECT d.tipo::text || ' ' || coalesce(d.componente_id::text, 'senza') || ' ' || p.stato::text || ' ' || coalesce(p.componente_id::text, 'senza')
		FROM documento d JOIN documento_provenienza v USING (documento_id) JOIN documento_proposta p ON p.allegato_id = v.allegato_id
		WHERE p.proposta_id = $1`, s.pdfLibero2); got != "altro senza confermata senza" {
		t.Errorf("il documento e la proposta: %s", got)
	}
	if a := generale(w, s.pdfLibero2, "altro"); !strings.Contains(a, "il file è già stato deciso") {
		t.Errorf("due volte: %q", a)
	}
	altra := b.rfqFascicolo(b.clienteDiProva("BETA", "Beta S.r.l.", "beta.example"), "GEN-ALTRA")
	pAltra, _ := altra.propostaDa("altro.pdf", "")
	if a := generale(w, pAltra, "altro"); !strings.Contains(a, "il file non è di questa RFQ") {
		t.Errorf("un file di un'altra RFQ: %q", a)
	}
	if a := generale(w, s.pdfLibero, "capitolato"); !strings.HasPrefix(a, "Confermato") {
		t.Fatalf("capitolato: %q", a)
	}
	if got := s.valore(`SELECT d.tipo::text || ' ' || coalesce(d.componente_id::text, 'senza') FROM documento d JOIN documento_provenienza v USING (documento_id)
		WHERE v.allegato_id = $1`, s.allLibero); got != "capitolato senza" {
		t.Errorf("il capitolato: %s", got)
	}
}

// ---------------------------------------------------------------- la sezione e i dati dell'editor

// La sezione di destra della vista Documenti si chiede da sola quando si sceglie un altro componente; i dati
// dell'editor sono la working intera, le proposte con i riferimenti che bom/applica accetta, e chi puo'
// scrivere.
func TestLaSezioneEIDatiDellEditor(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaV3(t, "SEZ")
	resp, html := s.w.daFascicolo(http.MethodGet, s.base()+"/sezione?nodo="+s.assieme.String(), nil, s.thread, "?nodo="+s.assieme.String())
	if resp.StatusCode != 200 || !strings.Contains(html, `class="docv-sez"`) || !strings.Contains(html, `data-k="c:`+s.assieme.String()+`"`) ||
		!strings.Contains(html, "77720517.pdf") || strings.Contains(html, "<html") {
		t.Errorf("la sezione dell'assieme: %d\n%.400s", resp.StatusCode, html)
	}

	dati := func(w *browser) datiEditor {
		t.Helper()
		resp, corpo := w.daFascicolo(http.MethodGet, s.base()+"/bom/dati?prodotto="+s.prodotto.String(), nil, s.thread, "?vista=bom")
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("bom/dati: %d %s %s", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"))
		}
		var d datiEditor
		if err := json.Unmarshal([]byte(corpo), &d); err != nil {
			t.Fatalf("bom/dati non e' JSON: %v", err)
		}
		return d
	}
	d := dati(s.w)
	if d.Prodotto != refC(s.prodotto) || !d.Scrive || d.Bloccata != 0 {
		t.Errorf("prodotto %s, scrive %v, bloccata %d", d.Prodotto, d.Scrive, d.Bloccata)
	}
	if n := d.Nodi[refC(s.assieme)]; n.Codice != "77720517" {
		t.Errorf("l'assieme fra i nodi: %+v", n)
	}
	if n := d.Nodi[refP(s.nuovo)]; n.Codice != "77811111" || !n.Proposto {
		t.Errorf("il nodo proposto: %+v", n)
	}
	// Smistamento F5: la radice senza codice e' la sorgente del file autorizzato per il prodotto, quindi e' il
	// prodotto (nessuna carta sua); l'assieme, figlio diretto con il codice di un componente, e' un ritrovato da
	// vedere; il pezzo nuovo sotto l'assieme nel file del prodotto e' guida
	if _, ok := d.Nodi[refP(s.top)]; ok {
		t.Errorf("la radice di un file autorizzato e' il suo componente, non una carta: %+v", d.Nodi[refP(s.top)])
	}
	if len(d.Ritrovati) != 1 || d.Ritrovati[0].Proposta != s.nodo2 || d.Ritrovati[0].Ref != refC(s.assieme) {
		t.Errorf("il ritrovato per codice: %+v", d.Ritrovati)
	}
	guida := false
	for _, g := range d.Guida {
		guida = guida || (g.File == "assieme.stp" && g.Padre == "77720517" && g.Figlio == "77811111" && g.Qta == 2)
	}
	if !guida {
		t.Errorf("la guida del file del prodotto: %+v", d.Guida)
	}
	archi := map[string]int32{}
	for _, a := range d.Archi {
		archi[a.Padre+">"+a.Figlio] = a.Qta
	}
	if archi[refC(s.prodotto)+">"+refC(s.assieme)] != 2 || archi[refC(s.assieme)+">"+refC(s.particolare)] != 4 || len(d.Archi) != 3 {
		t.Errorf("gli archi della working: %+v", d.Archi)
	}
	trovato := false
	for _, p := range d.Proposti {
		if p.Padre == refC(s.assieme) && p.Figlio == refP(s.nuovo) && p.Qta == 2 && p.PK == "#1" && p.FK == "#3" && p.Allegato == s.stpA {
			trovato = true
		}
		if p.Allegato == s.stp && p.FK == "#3" {
			t.Errorf("un arco della guida fra le proposte: %+v", p)
		}
	}
	if !trovato {
		t.Errorf("l'arco proposto sotto l'assieme (la radice dello STEP dell'assieme e' l'assieme): %+v", d.Proposti)
	}
	// i dati dell'editor si mandano indietro com'erano: bom/applica li accetta
	v := fascicolo.StrutturaVoluta{Radice: s.prodotto}
	for _, a := range d.Archi {
		av := fascicolo.ArcoVoluto{Padre: a.Padre, Figlio: a.Figlio, Qta: a.Qta}
		v.Archi, v.Visti = append(v.Archi, av), append(v.Visti, av)
	}
	if a, _, _ := s.applica(s.w, v); a != "Nessun cambiamento: la struttura di 77722757 è già così." {
		t.Errorf("i dati dell'editor rimandati: %q", a)
	}

	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if d := dati(co); d.Scrive {
		t.Error("chi consulta non scrive nell'editor")
	}
	s.congelaAMano()
	if d := dati(s.w); d.Bloccata != 1 {
		t.Errorf("la BOM congelata: %d", d.Bloccata)
	}
	if resp, _ := s.w.daFascicolo(http.MethodGet, "/thread/"+uuid.NewString()+"/fascicolo/bom/dati", nil, s.thread, ""); resp.StatusCode != http.StatusNotFound {
		t.Errorf("una RFQ che non c'e': %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- l'editor: i casi della revisione

// fattiDi scrive i fatti di uno STEP: nodi "#k=testo" (il testo e' id e nome), archi "#p>#f*q".
func fattiDi(radice string, nodi []string, archi []string) string {
	var n, r []string
	for _, x := range nodi {
		k, testo, _ := strings.Cut(x, "=")
		n = append(n, `{"chiave": "`+k+`", "id_grezzo": "`+testo+`", "nome_grezzo": "`+testo+`", "evidenza": {}}`)
	}
	for _, x := range archi {
		pf, q, _ := strings.Cut(x, "*")
		p, f, _ := strings.Cut(pf, ">")
		r = append(r, `{"padre": "`+p+`", "figlio": "`+f+`", "qta": `+q+`, "evidenza": {}}`)
	}
	return `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["` + radice + `"], "avvisi": [], "nodi": [` + strings.Join(n, ", ") +
		`], "relazioni": [` + strings.Join(r, ", ") + `], "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0,
		"occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
}

// stepLetto mette nella RFQ uno STEP con i suoi fatti e lo fa rileggere con «Rianalizza».
//
// Riscritta per lo Smistamento (R8, fase F1): prima apriva il Fascicolo, e la GET rileggeva lo STEP.
func (s *scenaB87) stepLetto(t *testing.T, w *browser, nome, fatti string) uuid.UUID {
	t.Helper()
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	s.b.ws.Analizzatore = an
	t.Cleanup(func() { s.b.ws.Analizzatore = coda.Analizzatore{} })
	a := s.stepNellaRfq(nome, fatti, an)
	s.rilegge(t, w, a)
	return a
}

// rilegge fa nascere le proposte dello STEP allegato con il gesto «Rianalizza», dopo aver visto che aprire
// il Fascicolo non ne scrive (Smistamento F1: nessuna GET scrive).
func (s *scenaB87) rilegge(t *testing.T, w *browser, allegato uuid.UUID) {
	t.Helper()
	if resp, _ := w.fai(http.MethodGet, s.base(), nil, false); resp.StatusCode != 200 {
		t.Fatalf("apertura: %d", resp.StatusCode)
	}
	if n := s.conta(`SELECT count(*) FROM componente_proposta WHERE allegato_id = $1`, allegato); n != 0 {
		t.Fatalf("aprire il Fascicolo ha scritto %d proposte dello STEP", n)
	}
	if a := s.gesto(w, s.base()+"/rianalizza", nil); !strings.Contains(a, "STEP riletto") && !strings.Contains(a, "STEP riletti") {
		t.Fatalf("rianalizza: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM componente_proposta WHERE allegato_id = $1`, allegato); n == 0 {
		t.Fatal("«Rianalizza» non ha scritto le proposte dello STEP")
	}
}

func (s *scenaB87) propostaNodo(t *testing.T, allegato uuid.UUID, chiave string) uuid.UUID {
	t.Helper()
	return uuidSQL(t, s.b, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = $2`, allegato, chiave)
}

// Due nodi con lo stesso codice uno dentro l'altro (STEP veri): confermata la struttura, l'arco fra i due e'
// un pezzo dentro se stesso, e si chiude con il motivo; la struttura dello STEP non resta da confermare.
//
// Riscritta per lo Smistamento (F5, Domanda 1 = B): prima i due nodi 77811111 erano uno sotto l'altro in fondo al file, e
// l'editor chiudeva l'arco fra loro. Adesso il nipote e' guida (non si decide dall'editor): il pezzo dentro se
// stesso e' l'assieme stesso fra i figli diretti del suo STEP, autorizzato per lui. L'editor lo mostra come
// ritrovato, la conferma lo accetta come l'assieme, e l'arco dall'assieme a se stesso si chiude con il motivo.
func TestLEditorChiudeLArcoDiUnPezzoDentroSeStesso(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ED5")
	w := operatore(b)
	stp := s.stepLetto(t, w, "assieme.stp", fattiDi("#1", []string{"#1=77720517", "#2=77811111", "#3=77720517"}, []string{"#1>#2*2", "#1>#3*1"}))
	s.autorizza(t, w, stp, s.assieme)
	v := s.strutturaDi(s.prodotto)
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.assieme), Figlio: refP(s.propostaNodo(t, stp, "#2")), Qta: 2})
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: stp, Padre: "#1", Figlio: "#2"}, {Allegato: stp, Padre: "#1", Figlio: "#3"}}
	v.RitrovatiVisti = []uuid.UUID{s.propostaNodo(t, stp, "#3")}
	if a, _, _ := s.applica(w, v); a != "Struttura di 77722757 confermata: 1 componente nuovo, 1 nodo proposto ritrovato nella BOM, 1 legame aggiunto, 1 proposta di legame chiusa." {
		t.Fatalf("editor: %q", a)
	}
	if got := s.valore(`SELECT stato::text || ' ' || coalesce(nota, '') FROM relazione_proposta WHERE allegato_id = $1 AND figlio_chiave = '#3'`, stp); got != "scartata padre e figlio sono lo stesso componente: un pezzo non contiene se stesso" {
		t.Errorf("l'arco fra i due nodi uguali: %q", got)
	}
	if n := s.conta(`SELECT count(*) FROM relazione_proposta WHERE allegato_id = $1 AND stato = 'aperta'`, stp); n != 0 {
		t.Errorf("proposte di arco aperte: %d", n)
	}
	_, pagina := w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if strings.Contains(pagina, `id="step-`+stp.String()+`"`) {
		t.Error("la struttura dello STEP resta da confermare")
	}
}

// Un nodo senza codice a cui l'operatore scrive il codice di un assieme che sta sotto un altro prodotto: il nodo
// ritrova l'assieme, e i figli dell'assieme (che l'editor non mostrava sotto quella carta) restano dove sono.
func TestLEditorNonToccaIFigliDiUnPezzoRitrovatoPerCodice(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ED6")
	w := operatore(b)
	altro := s.componenteTipo("77799999", "finito")
	s.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, s.prodotto, s.assieme)
	s.arco(altro, s.assieme, 1)
	stp := s.stepLetto(t, w, "prodotto.stp", fattiDi("#1", []string{"#1=77722757", "#5=Part", "#6=77866666"}, []string{"#1>#5*1", "#5>#6*3"}))
	s.autorizza(t, w, stp, s.prodotto) // Smistamento F5: il file e' autorizzato per il prodotto; #6, sotto Part, e' guida
	part := s.propostaNodo(t, stp, "#5")
	v := s.strutturaDi(s.prodotto)
	v.Archi = nil
	for _, a := range v.Visti {
		if a.Padre == refC(s.prodotto) {
			v.Archi = append(v.Archi, a)
		}
	}
	v.Archi = append(v.Archi, fascicolo.ArcoVoluto{Padre: refC(s.prodotto), Figlio: refP(part), Qta: 1})
	v.Codici = map[string]fascicolo.CodiceScritto{part.String(): {Codice: "77720517"}}
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: stp, Padre: "#1", Figlio: "#5"}}
	// riscritta per lo Smistamento (F5): prima la struttura prendeva anche il nipote 77866666 sotto Part; e' guida,
	// e si rifiuta
	guida := v
	guida.Archi = append(append([]fascicolo.ArcoVoluto{}, v.Archi...), fascicolo.ArcoVoluto{Padre: refP(part), Figlio: refP(s.propostaNodo(t, stp, "#6")), Qta: 3})
	if a, _, _ := s.applica(w, guida); !strings.Contains(a, "77866666 è guida") {
		t.Fatalf("il nipote dal file del prodotto: %q", a)
	}
	a, _, _ := s.applica(w, v)
	if !strings.HasPrefix(a, "Struttura di 77722757 confermata") || strings.Contains(a, "tolt") {
		t.Fatalf("editor: %q", a)
	}
	if got := s.archi(); got != "77720517>77817189x4 77722757>77720517x1 77722757>77817189x1 77799999>77720517x1" {
		t.Errorf("archi: %s", got)
	}
	if got := s.valore(`SELECT stato::text || ' ' || (componente_id = $2)::text FROM componente_proposta WHERE proposta_id = $1`, part, s.assieme); got != "duplicato true" {
		t.Errorf("il nodo con il codice scritto: %s", got)
	}
}

// La struttura disegnata su dati vecchi: una quantita' cambiata nel frattempo, un arco tolto nel frattempo. La
// conferma lo dice e non scrive niente.
func TestLEditorSiAccorgeDeiCambiamentiNelFrattempo(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ED7")
	w := operatore(b)
	vecchia := s.strutturaDi(s.prodotto)
	s.esegui(`UPDATE componente_relazione SET qta = 5 WHERE padre_id = $1 AND figlio_id = $2`, s.assieme, s.particolare)
	a, ev, _ := s.applica(w, senza(vecchia, s.prodotto, s.particolare))
	if a != "Niente è cambiato: 77817189 sotto 77720517 nel frattempo è diventato ×5 (l'editor mostrava ×4): riapri l'editor" {
		t.Errorf("quantita' cambiata: %q", a)
	}
	if ok, _ := esitoEditor(t, ev); ok {
		t.Error("l'evento dice fatto")
	}
	if got := s.archi(); got != "77720517>77817189x5 77722757>77720517x2 77722757>77817189x1" {
		t.Fatalf("archi dopo il rifiuto: %s", got)
	}
	vecchia = s.strutturaDi(s.prodotto)
	s.esegui(`DELETE FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, s.prodotto, s.particolare)
	if a, _, _ := s.applica(w, vecchia); a != "Niente è cambiato: 77817189 sotto 77722757 nel frattempo è stato tolto (un altro gesto): riapri l'editor" {
		t.Errorf("arco tolto: %q", a)
	}
	if got := s.archi(); got != "77720517>77817189x5 77722757>77720517x2" {
		t.Errorf("archi dopo il secondo rifiuto: %s", got)
	}
}

// Lo stesso nodo in due STEP sono due carte nell'editor, una per file: «scarta» su una vale per il suo file, e
// dopo lo scarto di tutte e due non resta niente di aperto.
//
// Riscritta per lo Smistamento (R5, fase F2): prima la carta comune era la radice PRT-00017 detta «il
// prodotto» (RadiciProposte, due radici riconosciute, duplicato duplicato). «E' il prodotto» e' tolto e si
// rifiuta senza scrivere niente; la carta comune si prova con lo scarto, che chiude le due radici e i loro
// archi.
//
// Riscritta ancora per F5 (A5.4): le due radici sono le sorgenti di due file autorizzati (uno per il prodotto,
// uno per l'assieme: un componente ha un solo file autorizzato), quindi sono i loro componenti e non si
// scartano; la carta comune e' 77811111, figlio diretto in tutti e due, e lo scarto chiude i due nodi e i loro
// archi.
//
// Riscritta ancora per F5 (correzione, A5.4.7: le riconciliazioni no): la carta comune a due file decideva
// per uguaglianza di codice anche la proposta che la carta non mostrava. Adesso lo scarto della carta del
// primo file chiude il suo nodo e il suo arco, e la proposta del secondo file resta aperta; lo scarto della
// seconda carta chiude il resto, e alla fine vale quello che valeva prima.
func TestLEditorUnaCartaValePerTuttiGliStep(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ED8")
	w := operatore(b)
	fatti := fattiDi("#1", []string{"#1=PRT-00017", "#2=77811111"}, []string{"#1>#2*2"})
	primo := s.stepLetto(t, w, "a.stp", fatti)
	secondo := s.stepLetto(t, w, "b.stp", fatti)
	testutil.AutorizzaStep(t, s.b.pool, s.thread, s.prodotto, primo, s.utente)
	s.autorizza(t, w, secondo, s.assieme)
	v := s.strutturaDi(s.prodotto)
	if a, _, _ := s.applicaConRadici(w, v, s.propostaNodo(t, primo, "#1")); !strings.HasPrefix(a, "Niente è cambiato: «È il prodotto» non c'è più") {
		t.Fatalf("E' il prodotto: %q", a)
	}
	v.Scarta = []uuid.UUID{s.propostaNodo(t, primo, "#2")}
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: primo, Padre: "#1", Figlio: "#2"}}
	a, _, _ := s.applica(w, v)
	if a != "Struttura di 77722757 confermata: 1 nodo proposto scartato, 1 proposta di legame chiusa." {
		t.Fatalf("editor, la carta del primo file: %q", a)
	}
	if got := s.valore(`SELECT stato::text FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, secondo); got != "aperta" {
		t.Errorf("la proposta del secondo file non e' nella carta del primo: %s", got)
	}
	v = s.strutturaDi(s.prodotto)
	v.Scarta = []uuid.UUID{s.propostaNodo(t, secondo, "#2")}
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: secondo, Padre: "#1", Figlio: "#2"}}
	if a, _, _ = s.applica(w, v); a != "Struttura di 77722757 confermata: 1 nodo proposto scartato, 1 proposta di legame chiusa." {
		t.Fatalf("editor, la carta del secondo file: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND stato = 'aperta'`, s.thread); n != 0 {
		t.Errorf("nodi proposti ancora aperti: %d", n)
	}
	if n := s.conta(`SELECT count(*) FROM relazione_proposta WHERE thread_id = $1 AND stato = 'aperta'`, s.thread); n != 0 {
		t.Errorf("archi proposti ancora aperti: %d", n)
	}
	if got := s.valore(`SELECT string_agg(stato::text, ' ' ORDER BY stato) FROM componente_proposta WHERE thread_id = $1 AND chiave = '#2'`, s.thread); got != "scartata scartata" {
		t.Errorf("i due nodi 77811111: %s", got)
	}
	if got := s.valore(`SELECT string_agg(stato::text, ' ' ORDER BY stato) FROM componente_proposta WHERE thread_id = $1 AND chiave = '#1'`, s.thread); got != "duplicato duplicato" {
		t.Errorf("le due radici restano le sorgenti: %s", got)
	}
}

// Aperto da uno STEP, l'editor si apre sul prodotto da cui si raggiunge la sua struttura, non sul primo.
func TestIDatiDellEditorSiApronoSulProdottoDelloStep(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ED9")
	w := operatore(b)
	secondo := s.componenteTipo("77799999", "finito")
	stp := s.stepLetto(t, w, "secondo.stp", fattiDi("#1", []string{"#1=77799999", "#2=77811111"}, []string{"#1>#2*2"}))
	s.autorizza(t, w, stp, secondo) // Smistamento F5: le proposte dell'editor sono quelle di un file autorizzato
	prodottoDi := func(q string) string {
		t.Helper()
		_, corpo := w.daFascicolo(http.MethodGet, s.base()+"/bom/dati"+q, nil, s.thread, "?vista=bom")
		var d datiEditor
		if err := json.Unmarshal([]byte(corpo), &d); err != nil {
			t.Fatal(err)
		}
		return d.Prodotto
	}
	if got := prodottoDi("?step=" + stp.String()); got != refC(secondo) {
		t.Errorf("con lo STEP: %s, atteso il secondo prodotto", got)
	}
	if got := prodottoDi(""); got != refC(secondo) {
		t.Errorf("senza niente: %s, atteso il prodotto con una proposta", got)
	}
	if got := prodottoDi("?prodotto=" + s.prodotto.String() + "&step=" + stp.String()); got != refC(s.prodotto) {
		t.Errorf("il prodotto scelto vince: %s", got)
	}
}

// Un nodo dello STEP ancora aperto con il codice di un componente che c'e' (il codice del componente e' stato
// corretto dopo): l'editor lo disegna come quel componente, e la conferma lo ritrova, prendendo l'arco proposto
// per quello che e' (non lo chiude come «tolto»).
//
// Riscritta per lo Smistamento (F5, P13): il nodo e' l'assieme 77720517, figlio diretto dello STEP autorizzato per
// il prodotto, e l'arco mostrato e' quello dal prodotto (prima era quello sotto l'assieme, che adesso e' guida). La
// conferma lo ritrova solo se l'editor l'ha mostrato come ritrovato (RitrovatiVisti): senza, si rifiuta; con,
// il nodo e' l'assieme e l'arco, uguale a quello della BOM, e' un duplicato deciso dalla persona.
func TestLEditorRitrovaUnNodoApertoConIlCodiceDiUnComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaV3(t, "ED10")
	v := s.strutturaDi(s.prodotto)
	v.RelazioniViste = []fascicolo.RelazioneVista{{Allegato: s.stp, Padre: "#1", Figlio: "#2"}}
	if a, _, _ := s.applica(s.w, v); !strings.Contains(a, "l'editor non l'ha mostrato come ritrovato") {
		t.Fatalf("senza il ritrovato mostrato: %q", a)
	}
	v.RitrovatiVisti = []uuid.UUID{s.nodo2}
	a, _, _ := s.applica(s.w, v)
	if !strings.HasPrefix(a, "Struttura di 77722757 confermata") {
		t.Fatalf("editor: %q", a)
	}
	if got := s.valore(`SELECT stato::text || ' ' || (componente_id = $2)::text || ' ' || (deciso_da IS NOT NULL)::text FROM componente_proposta WHERE proposta_id = $1`, s.nodo2, s.assieme); got != "duplicato true true" {
		t.Errorf("il nodo 77720517: %s", got)
	}
	if got := s.valore(`SELECT stato::text FROM relazione_proposta WHERE allegato_id = $1 AND padre_chiave = '#1'`, s.stp); got != "duplicato" {
		t.Errorf("l'arco proposto verso l'assieme: %s", got)
	}
}

// Il file «77730000» in una RFQ il cui pezzo e' nato «77730000_PRT» prima della regola del cliente: «Da verificare»
// offre di assegnarlo a quello, e il gesto gli da' il componente e il suo codice (un documento ha il codice del
// suo componente: la FK composita lo tiene).
func TestAssegnareIlFileAlPezzoConIlSuffisso(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ALIAS")
	w := operatore(b)
	s.esegui(`UPDATE cliente SET regole = '{"suffissi_decorativi": ["_PRT"]}' WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $1)`, s.thread)
	vecchio := s.componenteTipo("77730000_PRT", "sciolto")
	p, _ := s.propostaDa("77730000.pdf", "77730000")
	_, html := w.fai(http.MethodGet, s.base()+"?cassetto=verifica", nil, false)
	haTesto(t, "da verificare", html, "lo stesso pezzo con il suffisso del cliente", "Assegna a 77730000_PRT",
		`"componente":"`+vecchio.String()+`","correggi_codice":"1"`)
	if strings.Contains(html, `hx-vals='{"codice":"77730000"`) {
		t.Error("il file offre di far nascere 77730000 accanto a 77730000_PRT")
	}
	a := s.gesto(w, s.base()+"/assegna", url.Values{"proposta": {p.String()}, "componente": {vecchio.String()}, "correggi_codice": {"1"}})
	if strings.HasPrefix(a, "Niente è cambiato") {
		t.Fatalf("assegna: %q", a)
	}
	if got := s.valore(`SELECT codice || ' ' || (componente_id = $2)::text FROM documento_proposta WHERE proposta_id = $1`, p, vecchio); got != "77730000_PRT true" {
		t.Errorf("la proposta dopo il gesto: %s", got)
	}
	_, html = w.fai(http.MethodGet, s.base()+"?cassetto=verifica", nil, false)
	if strings.Contains(html, "Assegna a 77730000_PRT") {
		t.Error("dopo il gesto il file chiede ancora")
	}
}
