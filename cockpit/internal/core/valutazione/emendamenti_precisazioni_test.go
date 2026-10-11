// L1 — gli emendamenti EB7 con le precisazioni D1–D6 dell'utente del 10/10 (P7e/Q7e; direttiva del 10/10, Blocco 4; E2
// §7.3): la divergenza dichiarata fra il tipo dei nodi e i previsti (D1, T-P7e-05), le fonti del tipo con le proposte
// scartate, decise e fuori dall'enum (D3, T-P7e-12, T-P7e-13), la radice candidata che non è il prodotto (D4), la
// parentela dei nodi che porta i legami del file e non la gerarchia confermata (D5, T-P7e-14); e i casi che il lavoro in
// corso non provava: il tipo nelle candidate, il componente senza tipo, il target archiviato o senza componente, lo stato
// del NAS di un documento che la fotografia non ha, la forma JSON dei campi nuovi, il determinismo, la parentela con più
// padri. Esporre più dati non cambia stato, assi e autorità [U]: lo prova la non regressione (p7e_nonregressione_test.go).
package valutazione_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_emendamenti_test.go.

// Gli ID delle scene di queste prove: il componente del nodo #3, il componente sotto due padri, il nipote non rappresentato.
var (
	cNodo3D5       = uid(0xe7e1)
	cDuePadri      = uid(0xe7e2)
	cNipoteSenza   = uid(0xe7e3)
	rifNodo3, rif4 = nodoB("#3"), nodoB("#4")
)

// previstiDelNodo: i tipi di documento dei previsti di un nodo, in ordine.
func previstiDelNodo(d valutazione.CompletezzaDocumentale, nodo string) []string {
	var out []string
	for _, p := range d.Previsti {
		if p.Nodo == nodo {
			out = append(out, p.TipoDocumento)
		}
	}
	sort.Strings(out)
	return out
}

// TestD1LaDivergenzaDichiarata (D1 dell'utente del 10/10, «Pater Familias», applicazione a P7e; T-P7e-05): con l'arco
// #3 → #4 tolto da una persona, il nodo #3 non ha più figli attivi, e il suo tipo proposto dalla regola è sciolto (EB7-2:
// gli archi non tolti). I previsti della completezza guardano invece tutti gli archi delle strutture che contano, com'era
// prima di EB7-2: #3 è previsto come un sottoassieme, con i fabbisogni del sottoassieme. La divergenza è dichiarata e non
// risolta: nessun cambiamento silenzioso dei fabbisogni documentali (la non regressione lo prova sulla stessa scena).
func TestD1LaDivergenzaDichiarata(t *testing.T) {
	th := scenaTreNodiArcoTolto(t)
	th.Fabbisogni = fabbisogniD3()
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, rifNodo3); tipoDi(n) != "sciolto/proposto_dalla_regola" {
		t.Errorf("il nodo #3 senza figli attivi: %s", tipoDi(n))
	}
	if got := previstiDelNodo(p.Documenti, rifNodo3); !reflect.DeepEqual(got, []string{"cad_3d", "disegno_2d"}) {
		t.Errorf("i previsti di #3 hanno i fabbisogni del sottoassieme (tutti gli archi, dubbio T-P7e-05): %v", got)
	}
	if got := previstiDelNodo(p.Documenti, rif4); len(got) != 0 {
		t.Errorf("#4, raggiunto solo dall'arco tolto, non è previsto: %v", got)
	}
}

// TestD3LaRigaScartataNonDaIlTipo (D3 dell'utente del 10/10: «una proposta scartata non ha autorità»; T-P7e-12, T-P7e-13):
// la riga scartata di #4 con tipo_proposto sottoassieme non dà il tipo. Scartata senza una persona, il nodo resta previsto,
// con i fabbisogni dello sciolto della regola: prima di D3 erano quelli del sottoassieme (il cad_3d del cliente), ed è la
// differenza autorizzata della non regressione. Scartata da una persona, il nodo è uscito dalla BOM: non è previsto, e il
// suo tipo resta quello della regola. Lo stato e il motivo della completezza non dipendono dai previsti.
func TestD3LaRigaScartataNonDaIlTipo(t *testing.T) {
	p := prodottoDi(t, esitoSmistamento(t, scenaD3RigaScartata(t)), rifProdB)
	n := nodoBOMDi(t, p, rif4)
	if tipoDi(n) != "sciolto/proposto_dalla_regola" || n.Nodo.RigaDecisa == nil || n.Nodo.RigaDecisa.Stato != "scartata" || n.Nodo.DecisoDaPersona {
		t.Errorf("il nodo con la riga scartata: %s, riga %+v", tipoDi(n), n.Nodo.RigaDecisa)
	}
	if got := previstiDelNodo(p.Documenti, rif4); !reflect.DeepEqual(got, []string{"disegno_2d"}) {
		t.Errorf("i previsti di #4 con la regola (sciolto): %v", got)
	}
	th := scenaD3RigaScartata(t)
	op := operatore
	rigaDi(t, &th, "#4").DecisoDa = &op
	p = prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, rif4); tipoDi(n) != "sciolto/proposto_dalla_regola" || !n.Nodo.DecisoDaPersona {
		t.Errorf("il nodo scartato da una persona: %s", tipoDi(n))
	}
	if got := previstiDelNodo(p.Documenti, rif4); len(got) != 0 {
		t.Errorf("il nodo scartato da una persona non è previsto: %v", got)
	}
}

// TestD3IlValoreFuoriEnum (D3: «i valori proposti devono appartenere all'enum tipo_componente»; T-P7e-12): una riga
// aperta con un valore fuori dall'enum non è una proposta, e il nodo prende la regola (#3 ha il figlio #4: sottoassieme),
// anche nei previsti; un valore dell'enum con gli spazi intorno vale, come prima; la maiuscola no (l'enum del DB la
// distingue). «finito» è dell'enum, ma #3 non è il target: da D-P7e-1 (11/10, A) la riga non ha autorità e #3 prende la
// regola (prima di D-P7e-1 questa prova fissava finito/proposto_dalla_riga; le prove di D-P7e-1 in finito_target_test.go).
func TestD3IlValoreFuoriEnum(t *testing.T) {
	p := prodottoDi(t, esitoSmistamento(t, scenaD3FuoriEnum(t)), rifProdB)
	if n := nodoBOMDi(t, p, rifNodo3); tipoDi(n) != "sottoassieme/proposto_dalla_regola" || n.Nodo.RigaLegacy == nil {
		t.Errorf("il valore fuori dall'enum: %s", tipoDi(n))
	}
	if got := previstiDelNodo(p.Documenti, rifNodo3); !reflect.DeepEqual(got, []string{"cad_3d", "disegno_2d"}) {
		t.Errorf("i previsti di #3 con la regola (sottoassieme): %v", got)
	}
	for _, c := range []struct{ valore, atteso string }{
		{"  sciolto ", "sciolto/proposto_dalla_riga"},
		{"Sciolto", "sottoassieme/proposto_dalla_regola"},
		{"finito", "sottoassieme/proposto_dalla_regola"},
	} {
		th := scenaTreNodi(t, true)
		rigaDi(t, &th, "#3").TipoProposto = testo(c.valore)
		if n := nodoBOMDi(t, prodottoDi(t, esitoSmistamento(t, th), rifProdB), rifNodo3); tipoDi(n) != c.atteso {
			t.Errorf("%q: %s, atteso %s", c.valore, tipoDi(n), c.atteso)
		}
	}
}

// TestD3LaRigaDecisaVaPerLaDecisione (D3: «una proposta già decisa va interpretata attraverso la decisione effettivamente
// registrata, non come se fosse ancora aperta»; T-P7e-12): la riga confermata di #3 con un componente attivo dà al nodo la
// decisione, e il tipo è quello del componente, confermato, anche se la riga proponeva altro; la riga confermata con un
// componente archiviato non arriva al nodo come decisione, e non vale come proposta aperta: il nodo prende la regola,
// anche nei previsti (lettura [T], T-P7e-17: non il tipo del componente archiviato, non «nessun tipo»).
func TestD3LaRigaDecisaVaPerLaDecisione(t *testing.T) {
	th := scenaTreNodi(t, true)
	confermaComponente(&th, cNodo3D5, "7120300A", cProdotto, 1)
	op := operatore
	decidi(t, &th, "#3", "confermata", cNodo3D5, &op)
	rigaDi(t, &th, "#3").TipoProposto = testo("commerciale")
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, rifNodo3); tipoDi(n) != "sciolto/confermato" || n.Nodo.Decisione == nil || n.Nodo.Decisione.ComponenteID != cNodo3D5 {
		t.Errorf("la riga confermata con il componente attivo: %s, decisione %+v", tipoDi(n), n.Nodo.Decisione)
	}
	p = prodottoDi(t, esitoSmistamento(t, scenaD3RigaDecisaArchiviato(t)), rifProdB)
	if n := nodoBOMDi(t, p, rif4); tipoDi(n) != "sciolto/proposto_dalla_regola" || n.Nodo.Decisione != nil || n.Nodo.RigaDecisa == nil {
		t.Errorf("la riga confermata con il componente archiviato: %s, decisione %+v", tipoDi(n), n.Nodo.Decisione)
	}
	if got := previstiDelNodo(p.Documenti, rif4); !reflect.DeepEqual(got, []string{"disegno_2d"}) {
		t.Errorf("i previsti di #4 con la regola (sciolto): %v", got)
	}
}

// TestEB72IlTipoNelleCandidate (EB7-2 A, D4; T-P7e-06, T-P7e-07): nei nodi di una struttura candidata la riga aperta con il
// tipo proposto vince sulla regola; la radice della candidata, che ha lo stesso codice del target, è un sottoassieme della
// regola, mentre il prodotto, fra i confermati senza STEP, è finito e confermato (D4: la radice di uno STEP candidato non
// è il prodotto, e le due cose stanno insieme senza errore). Un nodo deciso con un componente senza tipo non ha un tipo:
// "" e "", senza ricadere sulla proposta.
func TestEB72IlTipoNelleCandidate(t *testing.T) {
	th := scenaCandidata(t)
	rigaDi(t, &th, "#2").TipoProposto = testo("commerciale")
	p := prodottoDi(t, esitoSmistamentoCon(t, th, insiemeConMinuteria(t)), rifProdB)
	if len(p.StruttureCandidate) != 1 {
		t.Fatalf("strutture candidate %v", chiaviCandidate(p))
	}
	nodi := p.StruttureCandidate[0].Nodi
	if r, f, m := nodoFra(t, nodi, nodoB("#1")), nodoFra(t, nodi, nodoB("#2")), nodoFra(t, nodi, nodoB("#3")); tipoDi(r) != "sottoassieme/proposto_dalla_regola" ||
		tipoDi(f) != "commerciale/proposto_dalla_riga" || tipoDi(m) != "sciolto/proposto_dalla_regola" {
		t.Errorf("la radice %s, il figlio con la riga %s, la minuteria %s", tipoDi(r), tipoDi(f), tipoDi(m))
	}
	if len(p.ConfermatiSenzaSTEP) == 0 || p.ConfermatiSenzaSTEP[0].Nodo.Rif != rifProdB || tipoDi(p.ConfermatiSenzaSTEP[0]) != "finito/confermato" {
		t.Errorf("il prodotto fra i confermati senza STEP: %v", rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	th = scenaCompletezza(t, true)
	th.Componenti[1].Tipo = ""
	p = prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, nodoB("#2")); n.Tipo != "" || n.OrigineTipo != "" || n.Nodo.Decisione == nil {
		t.Errorf("il nodo deciso con il componente senza tipo: %q/%q", n.Tipo, n.OrigineTipo)
	}
}

// TestEB71IlTargetSenzaComponente (EB7-1 B; bozza §1.4: «vuoto per un target senza componente»): un identificativo
// confermato senza componente non ha confermati senza STEP.
func TestEB71IlTargetSenzaComponente(t *testing.T) {
	v := valuta(t, scenaSenzaComponente(), motoreACME(t), nil)
	for _, p := range v.ProdottiValutati {
		if p.Rif == rifSenzaComponente {
			if p.ComponenteID != nil || p.ConfermatiSenzaSTEP != nil {
				t.Errorf("il target senza componente: %+v", p.ConfermatiSenzaSTEP)
			}
			return
		}
	}
	t.Fatalf("il prodotto %s non c'è", rifSenzaComponente)
}

// TestEB71IlTargetArchiviato (EB7-1 B; T-P7e-04): il componente del target archiviato non fa un target (targetDelThread
// salta gli archiviati), e i componenti archiviati non entrano mai nelle relazioni attive: nessun prodotto lo ha, e nessun
// confermato senza STEP lo porta. Il ramo di confermatiSenzaSTEP per un componente che il contesto non ha (senza
// Decisione: il valore non si costruisce) resta così una difesa che Calcola non raggiunge.
func TestEB71IlTargetArchiviato(t *testing.T) {
	th := scenaSenzaSTEP(t)
	il := dataACME
	th.Componenti[0].ArchiviatoIl = &il
	for _, p := range esitoSmistamento(t, th).ProdottiValutati {
		if p.ComponenteID != nil && *p.ComponenteID == cProdotto {
			t.Errorf("il componente archiviato è il componente di %s", p.Rif)
		}
		for _, c := range p.ConfermatiSenzaSTEP {
			if c.Nodo.Rif == rifProdB || c.Nodo.Decisione == nil {
				t.Errorf("%s: un confermato senza STEP archiviato o senza decisione: %s", p.Rif, c.Nodo.Rif)
			}
		}
	}
}

// TestEB71LaParentelaDeiConfermati (EB7-1 B; bozza §1.4; T-P7e-02): un componente confermato sotto due padri ha due
// elementi di Parentela, in ordine di Padre, tutti e due con la quantità confermata; un padre che nessun nodo rappresenta
// compare come «componente:<uuid>», anche a più livelli (il nipote sotto lo sciolto, senza STEP).
func TestEB71LaParentelaDeiConfermati(t *testing.T) {
	th := scenaSenzaSTEP(t)
	confermaComponente(&th, cDuePadri, "7120400A", cProdotto, 6)
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cSciolto, FiglioID: cDuePadri, Qta: 7, Origine: "manuale", ConfermatoDa: operatore, CreatoIl: dataACME})
	confermaComponente(&th, cNipoteSenza, "7120410A", cSciolto, 8)
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	var due, nipote *valutazione.NodoBOM
	for i := range p.ConfermatiSenzaSTEP {
		switch p.ConfermatiSenzaSTEP[i].Nodo.Rif {
		case ancoraggio.RifComponente(cDuePadri):
			due = &p.ConfermatiSenzaSTEP[i]
		case ancoraggio.RifComponente(cNipoteSenza):
			nipote = &p.ConfermatiSenzaSTEP[i]
		}
	}
	if due == nil || nipote == nil {
		t.Fatalf("confermati senza STEP %v", rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	sei, sette, otto := 6, 7, 8
	attese := []valutazione.Parentela{{Padre: ancoraggio.RifComponente(cProdotto), Quantita: &sei, Decisa: true},
		{Padre: ancoraggio.RifComponente(cSciolto), Quantita: &sette, Decisa: true}}
	sort.Slice(attese, func(i, j int) bool { return attese[i].Padre < attese[j].Padre })
	if !reflect.DeepEqual(due.Parentela, attese) {
		t.Errorf("due padri: %+v", due.Parentela)
	}
	if !reflect.DeepEqual(nipote.Parentela, []valutazione.Parentela{{Padre: ancoraggio.RifComponente(cSciolto), Quantita: &otto, Decisa: true}}) {
		t.Errorf("il nipote sotto lo sciolto non rappresentato: %+v", nipote.Parentela)
	}
}

// TestD5LaParentelaPortaILegamiDelFile (D5 dell'utente del 10/10; T-P7e-14): NodoBOM.Parentela di un nodo dello STEP porta
// i legami del file (gli archi), con il segno dell'arco confermato accanto; non è la gerarchia confermata. Una relazione
// confermata fra due componenti rappresentati da nodi, che nessun arco dello STEP porta (lo sciolto #2 → il componente di
// #3), non compare in nessuna Parentela, e i due componenti non sono fra i confermati senza STEP: è il limite dichiarato,
// che l'adattatore di A1d risolve leggendo le relazioni confermate dalla fotografia e dagli ancoraggi, senza inventare
// provenienze.
func TestD5LaParentelaPortaILegamiDelFile(t *testing.T) {
	th := scenaTreNodi(t, true)
	confermaComponente(&th, cNodo3D5, "7120300A", cProdotto, 1)
	op := operatore
	decidi(t, &th, "#3", "confermata", cNodo3D5, &op)
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cSciolto, FiglioID: cNodo3D5, Qta: 9, Origine: "manuale", ConfermatoDa: operatore, CreatoIl: dataACME})
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	n := nodoBOMDi(t, p, rifNodo3)
	if len(n.Parentela) != 1 || n.Parentela[0].Padre != nodoB("#1") {
		t.Errorf("la parentela di #3 dagli archi del file: %+v", n.Parentela)
	}
	for _, x := range p.Nodi {
		for _, pa := range x.Parentela {
			if pa.Padre == nodoB("#2") {
				t.Errorf("la relazione confermata senza arco compare nella parentela di %s", x.Nodo.Rif)
			}
		}
	}
	for _, c := range p.ConfermatiSenzaSTEP {
		if c.Nodo.Rif == ancoraggio.RifComponente(cNodo3D5) || c.Nodo.Rif == ancoraggio.RifComponente(cSciolto) {
			t.Errorf("un componente rappresentato fra i confermati senza STEP: %s", c.Nodo.Rif)
		}
	}
}

// TestEB74IlDocumentoNonInFotografia (EB7-4 A; T-P7e-09): lo stato del NAS di un documento che la fotografia non ha è
// "": il valore non si inventa. Lo stato si copia com'è, senza toglierne gli spazi né validarlo.
func TestEB74IlDocumentoNonInFotografia(t *testing.T) {
	th := scenaCompletezza(t, true)
	if s := valutazione.StatoNasDelPerProva(th, uid(0xe7f1)); s != "" {
		t.Errorf("il documento che non c'è: %q", s)
	}
	for i := range th.Documenti {
		if th.Documenti[i].ID == dPDF2 {
			th.Documenti[i].StatoNas = " in_coda "
		}
	}
	if s := valutazione.StatoNasDelPerProva(th, dPDF2); s != " in_coda " {
		t.Errorf("lo stato com'è: %q", s)
	}
}

// TestEB7LaFormaJSON (bozza §1.3, §1.5, §1.6; T-P7e-01): strutture_candidate e confermati_senza_step mancano quando sono
// vuoti; una struttura candidata senza nodi non ha nodi né motivo_grafo; tipo e origine_tipo ci sono sempre, anche vuoti;
// la voce del 2D senza un documento che la decide non ha documento_id né stato_nas.
func TestEB7LaFormaJSON(t *testing.T) {
	testoJSON := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if s := testoJSON(valutazione.ProdottoValutato{}); strings.Contains(s, "strutture_candidate") || strings.Contains(s, "confermati_senza_step") {
		t.Errorf("il prodotto vuoto: %s", s)
	}
	if s := testoJSON(valutazione.StrutturaCandidata{}); strings.Contains(s, `"nodi"`) || strings.Contains(s, "motivo_grafo") ||
		!strings.Contains(s, `"allegato_id"`) || !strings.Contains(s, `"radice_del_file"`) {
		t.Errorf("la struttura candidata vuota: %s", s)
	}
	if s := testoJSON(valutazione.NodoBOM{}); !strings.Contains(s, `"tipo":""`) || !strings.Contains(s, `"origine_tipo":""`) {
		t.Errorf("il nodo senza tipo: %s", s)
	}
	v := voceDi(t, documentiDi(t, valutaConVista(t, scenaDWGEAssegna(t), motoreCatena(t)), rifProdB), cSciolto, "disegno_2d")
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal([]byte(testoJSON(v)), &chiavi); err != nil {
		t.Fatal(err)
	}
	if _, ok := chiavi["documento_id"]; ok {
		t.Errorf("la voce del 2D decisa dall'associazione ha documento_id")
	}
	if _, ok := chiavi["stato_nas"]; ok {
		t.Errorf("la voce del 2D decisa dall'associazione ha stato_nas")
	}
}

// TestEB7IlDeterminismo (EB7-1 B, EB7-2 A; regole comuni, «Determinismo»): con i componenti, le relazioni e le righe di
// componente_proposta in un altro ordine, le strutture candidate, i confermati senza STEP e il tipo dei nodi sono gli
// stessi, byte per byte.
func TestEB7IlDeterminismo(t *testing.T) {
	campi := func(t *testing.T, th fotorfq.Thread) string {
		t.Helper()
		var out []string
		for _, p := range esitoSmistamentoCon(t, th, insiemeConMinuteria(t)).ProdottiValutati {
			b, err := json.Marshal(struct {
				Nodi       []valutazione.NodoBOM            `json:"nodi"`
				Candidate  []valutazione.StrutturaCandidata `json:"candidate"`
				Confermati []valutazione.NodoBOM            `json:"confermati"`
			}{p.Nodi, p.StruttureCandidate, p.ConfermatiSenzaSTEP})
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, p.Rif+" "+string(b))
		}
		return strings.Join(out, "\n")
	}
	for _, s := range []struct {
		nome string
		th   func(t *testing.T) fotorfq.Thread
	}{
		{"accanto", scenaAccanto}, {"due candidate", func(t *testing.T) fotorfq.Thread { th := scenaCandidata(t); conSecondoSTEP(t, &th); return th }},
		{"due prodotti", scenaDueProdotti}, {"riga di #4", scenaTreNodiRiga4Sottoassieme},
	} {
		th := s.th(t)
		prima := campi(t, th)
		th.Componenti, th.Relazioni, th.RigheComponenteProposta = rovescia(th.Componenti), rovescia(th.Relazioni), rovescia(th.RigheComponenteProposta)
		if dopo := campi(t, th); dopo != prima {
			t.Errorf("%s: con gli ingressi rovesciati i campi nuovi cambiano", s.nome)
		}
	}
}
