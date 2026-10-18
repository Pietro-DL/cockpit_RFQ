// L1 — il segnale LS-02 sui nodi degli STEP (V1: emendamento di V1 §11.1; addendum ad A1c §2.2 del 18/10; piano della
// 0023 e delle prove, §10 riga 6): la regola elemento_cad_nome_foglia_v1 dai fatti del worker all'adattatore STEP, a
// StrutturaDa e a ProponiStrutture, cioè dove nasce il NodoProposto.
//   - Q-01: gli esempi positivi e negativi sintetici di A3 §3.6, con i limiti dichiarati fissati come limiti (i corpi
//     «WELD_<codice>», i plurali e il tedesco non presi; «BEAD … RING» preso), più la scena ACME di sempre; l'equivalenza
//     fra l'espressione Go e quella Python della misura del 17/10 sugli stessi testi.
//   - Q-02: l'indipendenza dalla geometria e da LS-01 (fatti v3 senza diagnostica, v3 con la diagnostica 1, la v4 della
//     regressione): lo stesso esito; i contro-indizi lettura_identita e grafo_non_completo compaiono quando devono, senza
//     cambiare l'esito.
//   - Q-02b: il JSON di un nodo senza segnali è byte per byte quello di prima (omitempty, in fondo al tipo); quello di un
//     nodo segnalato è quello di prima con il campo segnali in più, in fondo.
//
// I nomi sono sintetici, con le stesse forme dei casi veri (ACME, codici di fantasia): i casi veri si misurano solo sul
// banco privato, fuori dal repository.
package ancoraggio_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// ---- i fatti di uno STEP, anche con la diagnostica ----

// nodoCAD: un PRODUCT dei fatti con la sua geometria (solo per i fatti con la diagnostica: la regola non la legge).
type nodoCAD struct {
	chiave, id, nome string
	vuota            bool
	solidi           int
}

// Le forme dei fatti di uno STEP: la v3 di sempre, senza diagnostica; la v3 con la diagnostica 1 (LS-01, il worker di
// oggi); la v4 della regressione di GP-3, con la geometria e senza il numero della diagnostica.
const (
	formaV3             = "v3"
	formaV3Diagnostica1 = "v3+diagnostica1"
	formaV4Regressione  = "v4"
)

// fattiCAD: i fatti di uno STEP nella forma data. motivo è quello che darebbe la funzione SQL della completezza
// (struttura_motivo_parziale): "" completo.
func fattiCAD(t *testing.T, forma, sha string, radici []string, nodi []nodoCAD, archi []arcoS, motivo string) *fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		m := map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": x.nome, "descrizione_grezza": "", "rev_grezza": "",
			"evidenza": map[string]any{}}
		if forma != formaV3 {
			m["geometria"] = map[string]any{"solidi": x.solidi, "superfici": 0, "curve": 0, "mappati": 0, "rappresentazioni": 1, "vuota": x.vuota}
		}
		n = append(n, m)
	}
	r := []map[string]any{}
	for _, a := range archi {
		m := map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": map[string]any{}}
		if forma != formaV3 {
			m["istanze"] = []map[string]any{{"rif": "#9" + a.figlio[1:], "id": "", "nome": "istanza ACME"}}
		}
		r = append(r, m)
	}
	s := map[string]any{"versione": 3, "schema": "AP214", "radici": radici, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}
	switch forma {
	case formaV3Diagnostica1:
		s["versione_diagnostica"], s["stato"], s["motivo_stato"] = 1, "valida", ""
	case formaV4Regressione:
		s["versione"], s["stato"], s["motivo_stato"] = 4, "valida", ""
	}
	raw, err := json.Marshal(map[string]any{"struttura": s})
	if err != nil {
		t.Fatal(err)
	}
	return fattiGrezzi(t, sha, raw, &motivo)
}

// nodiCAD: i nodi di una struttura proposta per ogni radice del file, con la fonte confermata su quella radice (la BOM di
// lavoro: la struttura si costruisce anche quando nessuna famiglia legge la radice), per Rif; poi la struttura del file e
// le strutture proposte. Le righe aperte della tabella legacy (proposto) entrano nel contesto. Un nodo raggiunto da due
// radici ha gli stessi segnali nelle due strutture: il segnale guarda il file, non la struttura.
func nodiCAD(t *testing.T, m *motorea.Motore, id uuid.UUID, nome, sha string, f *fotorfq.Fatti, radici []string,
	proposto ...ancoraggio.RigaPropostaLegacy) (map[string]ancoraggio.NodoProposto, ancoraggio.StrutturaFile, []ancoraggio.StrutturaProdotto) {
	t.Helper()
	sf := strutturaS(t, fileS(t, m, id, nome, "stp", sha, f))
	var target []ancoraggio.ProdottoRichiesto
	for _, r := range radici {
		target = append(target, ancoraggio.ProdottoRichiesto{Rif: "scenario:ls02:" + nome + ":" + r, Autorita: ancoraggio.AutoritaScenario,
			ClienteID: clienteACME, FonteConfermata: fonteR59(sha, r, &id)})
	}
	strutture, _ := proponiS(t, target, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{sf}, Proposto: proposto})
	out := map[string]ancoraggio.NodoProposto{}
	for _, s := range strutture {
		for _, n := range s.Nodi {
			if prima, ok := out[n.Rif]; ok && !reflect.DeepEqual(prima.Segnali, n.Segnali) {
				t.Errorf("%s: il nodo %s ha segnali diversi in due strutture: %+v e %+v", nome, n.Rif, prima.Segnali, n.Segnali)
			}
			out[n.Rif] = n
		}
	}
	return out, sf, strutture
}

// righeCAD: due righe aperte della tabella legacy dell'assieme degli esempi, del primo cordone e del primo pezzo vero.
func righeCAD() []ancoraggio.RigaPropostaLegacy {
	var out []ancoraggio.RigaPropostaLegacy
	for i, chiave := range []string{"#100", "#211"} {
		out = append(out, ancoraggio.RigaPropostaLegacy{ID: uidS(0x7e1 + i), AllegatoID: idAssiemeCAD, Sha256: shaAssiemeCAD, Chiave: chiave,
			Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto})
	}
	return out
}

// ---- la scena degli esempi di A3 §3.6 ----

var (
	idAssiemeCAD  = uidS(0x7c1)
	shaAssiemeCAD = strings.Repeat("7c", 32)
)

// esempioCAD: un figlio della radice dell'assieme degli esempi, con l'esito atteso.
type esempioCAD struct {
	nodo      nodoCAD
	segnalato bool
	perche    string // il caso vero da cui viene la forma, o il limite che la prova fissa
}

// esempiCAD: i figli della radice #1 («7120900 TELAIO ACME»), tutti foglie salvo il contenitore #300, nell'ordine della
// tabella di A3 §3.6. La geometria vale solo per i fatti con la diagnostica: i cordoni senza solido salvo cinque, i pezzi
// veri con un solido, le foglie con il codice e senza geometria (che sta sui nodi padre nel file CAD).
func esempiCAD() []esempioCAD {
	var out []esempioCAD
	for i := 0; i <= 50; i++ {
		nome := fmt.Sprintf("ACME_111_2.000_Saldatura d'angolo 1_oa_%d", i)
		out = append(out, esempioCAD{nodoCAD{fmt.Sprintf("#%d", 100+i), nome, nome, i%10 != 4, map[bool]int{true: 1}[i%10 == 4]}, true,
			"i 51 cordoni del caso vero, anche i cinque con un solido"})
	}
	add := func(chiave, id, nome string, vuota bool, solidi int, segnalato bool, perche string) {
		out = append(out, esempioCAD{nodoCAD{chiave, id, nome, vuota, solidi}, segnalato, perche})
	}
	add("#200", "SALDATURA_D_ANGOLO_ID_7", "SALDATURA_D_ANGOLO_ID_7", false, 1, true, "corpo di saldatura SALDATURA_…, con un solido")
	add("#201", "SALDATURA_TESTA_ID_8", "SALDATURA_TESTA_ID_8", false, 1, true, "corpo di saldatura SALDATURA_…, con un solido")
	add("#202", "FILLET_WELD_ID_3", "FILLET_WELD_ID_3", false, 1, true, "corpo di saldatura FILLET_WELD_ID_…")
	add("#203", "ACME-9001.206_1-WELD BOLT M6x1x20", "ACME-9001.206_1-WELD BOLT M6x1x20", false, 1, false, "minuteria da saldare: un articolo")
	add("#204", "ACME-9001.003_1-WELD NUT M6x1", "ACME-9001.003_1-WELD NUT M6x1", false, 1, false, "minuteria da saldare: un articolo")
	add("#205", "ACME - Weld nut M16", "ACME - Weld nut M16", false, 1, false, "articolo: «weld» non seguito da bead, seam o id")
	add("#206", "ACME - weld-on link spring relief", "ACME - weld-on link spring relief", false, 1, false, "articolo: «weld» non seguito da bead, seam o id")
	add("#207", "ACME - welding bush", "ACME - welding bush", false, 1, false, "articolo: «weld» non seguito da bead, seam o id")
	add("#208", "12345_oa_0", "12345_oa_0", false, 1, false, "pezzo vero con il suffisso _oa_, che da solo non vale")
	for i := 1; i <= 26; i++ {
		c := fmt.Sprintf("ACME-P-%03d", i)
		add(fmt.Sprintf("#%d", 210+i), c, c, false, 1, false, "i 26 pezzi veri dell'assieme dei cordoni")
	}
	for i := 1; i <= 19; i++ {
		c := fmt.Sprintf("ACME-0900.%d_0-PEZZO", i)
		add(fmt.Sprintf("#%d", 240+i), c, c, true, 0, false, "foglia con il codice e senza geometria: la geometria non è un criterio")
	}
	for i := 1; i <= 6; i++ {
		c := fmt.Sprintf("9900000%d", i)
		add(fmt.Sprintf("#%d", 260+i), c, c, false, 1, false, "figlio senza 2D: l'assenza del 2D non è un criterio")
	}
	add("#270", "WELD_9Z00001", "WELD_9Z00001", false, 1, false, "limite dichiarato: i corpi «WELD_<codice>» non sono presi")
	add("#271", "WELDS_9Z00002", "WELDS_9Z00002", false, 1, false, "limite dichiarato: i corpi «WELD_<codice>» non sono presi")
	add("#272", "9Z00003_WELD", "9Z00003_WELD", false, 1, false, "limite dichiarato: i corpi «WELD_<codice>» non sono presi")
	add("#273", "ACME BEAD RING 15", "ACME BEAD RING 15", false, 1, true, "falso positivo dichiarato: un dominio nuovo (ruote)")
	add("#274", "saldature ACME", "saldature ACME", true, 0, false, "limite dichiarato: i plurali non sono presi")
	add("#275", "Schweißnaht ACME", "Schweißnaht ACME", true, 0, false, "limite dichiarato: il tedesco non è preso")
	add("#276", "7120155", "Saldatura d'angolo 3", true, 0, true, "un cordone con un codice letto: il contro-indizio, l'esito non cambia")
	add("#277", "WELD_SEAM_4", "CORDONE 4", true, 0, true, "parole diverse nell'id e nel nome")
	add("#278", "bead_bead", "", true, 0, true, "due parole nello stesso campo")
	add("#300", "Saldatura gruppo ACME", "Saldatura gruppo ACME", true, 0, false, "un contenitore CAD con figli: non è una foglia, resta un nodo")
	add("#301", "ACME-P-099", "ACME-P-099", false, 1, false, "il figlio del contenitore")
	return out
}

// assiemeCAD: i nodi e gli archi dell'assieme degli esempi: la radice #1 e i figli; #301 sta sotto il contenitore #300.
func assiemeCAD() ([]nodoCAD, []arcoS) {
	nodi := []nodoCAD{{"#1", "7120900", "TELAIO ACME", false, 0}}
	var archi []arcoS
	for _, e := range esempiCAD() {
		nodi = append(nodi, e.nodo)
		padre := "#1"
		if e.nodo.chiave == "#301" {
			padre = "#300"
		}
		archi = append(archi, arcoS{padre, e.nodo.chiave, 1, nil})
	}
	return nodi, archi
}

// TestQ01GliEsempiDiA3 (Q-01): sugli esempi di A3 §3.6 il segnale c'è se e solo se l'id o il nome hanno la parola, il
// nodo è una foglia nel file e non è una radice del file. Per ogni nodo segnalato: un segnale solo, con il codice, la
// regola, un riscontro per campo nell'ordine id, nome, le parole com'erano nel testo con i loro intervalli in byte, e la
// spiegazione. Il segnale non toglie niente: il nodo resta con il suo arco, e Leggibile resta quello delle letture.
func TestQ01GliEsempiDiA3(t *testing.T) {
	m := motoreStrutture(t)
	nodi, archi := assiemeCAD()
	got, sf, strutture := nodiCAD(t, m, idAssiemeCAD, "ACME-030P7120900 00 IN_WORK.stp", shaAssiemeCAD,
		fattiCAD(t, formaV3, shaAssiemeCAD, []string{"#1"}, nodi, archi, ""), []string{"#1"}, righeCAD()...)
	if len(got) != len(nodi) {
		t.Fatalf("%d nodi nelle strutture, attesi %d: il segnale non toglie nessun nodo", len(got), len(nodi))
	}
	// Il segnale non cambia la struttura: il cordone e il pezzo con la riga aperta la tengono, e le due righe restano da
	// decidere; ogni figlio ha il suo arco dalla radice (o dal contenitore).
	if len(strutture) != 1 || len(strutture[0].Archi) != len(archi) {
		t.Fatalf("strutture %d, archi %d: attesi 1 e %d", len(strutture), len(strutture[0].Archi), len(archi))
	}
	if want := []string{ancoraggio.RifRigaProposta(uidS(0x7e1)), ancoraggio.RifRigaProposta(uidS(0x7e2))}; !reflect.DeepEqual(strutture[0].RigheDaDecidere, want) {
		t.Errorf("righe da decidere %v, attese %v: il segnale non decide le righe", strutture[0].RigheDaDecidere, want)
	}
	if c := got[ancoraggio.RifNodo(shaAssiemeCAD, "#100")]; c.RigaLegacy == nil || len(c.Segnali) != 1 {
		t.Errorf("il cordone con la riga aperta: riga %+v, segnali %d", c.RigaLegacy, len(c.Segnali))
	}
	if r := got[ancoraggio.RifNodo(shaAssiemeCAD, "#1")]; len(r.Segnali) != 0 {
		t.Errorf("la radice del file ha un segnale: %+v", r.Segnali)
	}
	segnalati := 0
	for _, e := range esempiCAD() {
		rif := ancoraggio.RifNodo(shaAssiemeCAD, e.nodo.chiave)
		n, ok := got[rif]
		if !ok {
			t.Errorf("%s (%s): il nodo non c'è", e.nodo.nome, e.perche)
			continue
		}
		if (len(n.Segnali) > 0) != e.segnalato {
			t.Errorf("%q / %q (%s): segnalato %v, atteso %v", e.nodo.id, e.nodo.nome, e.perche, len(n.Segnali) > 0, e.segnalato)
			continue
		}
		if !e.segnalato {
			continue
		}
		segnalati++
		if len(n.Segnali) != 1 {
			t.Errorf("%s: %d segnali, uno solo per regola", e.nodo.nome, len(n.Segnali))
			continue
		}
		s := n.Segnali[0]
		if s.Codice != ancoraggio.CodiceSegnaleElementoCAD || s.Regola != ancoraggio.RegolaElementoCADNomeFogliaV1 || s.Spiegazione == "" {
			t.Errorf("%s: segnale %+v", e.nodo.nome, s)
		}
		controllaRiscontri(t, sf, rif, e.nodo, s)
		if (n.Leggibile) != slices.Contains(s.ControIndizi, ancoraggio.ControIndizioLetturaIdentita) {
			t.Errorf("%s: Leggibile %v ma contro-indizi %v", e.nodo.nome, n.Leggibile, s.ControIndizi)
		}
	}
	if segnalati != 58 {
		t.Errorf("%d nodi segnalati, attesi 58 (51 cordoni, 2 SALDATURA_…, 1 FILLET_WELD_ID_…, 1 falso positivo dichiarato, 3 casi della spiegazione)", segnalati)
	}

	t.Run("le spiegazioni", func(t *testing.T) {
		cordone := got[ancoraggio.RifNodo(shaAssiemeCAD, "#100")].Segnali[0]
		if want := "L'id e il nome del pezzo nel file contengono «Saldatura»; il nodo non ha figli e non è una radice del file: " +
			"possibile elemento CAD (un cordone di saldatura o simile), non un articolo. È solo un segnale: da solo non toglie il nodo dalla proposta né il " +
			"suo 2D, e la decisione è tua."; cordone.Spiegazione != want {
			t.Errorf("spiegazione del cordone:\n%s\nattesa:\n%s", cordone.Spiegazione, want)
		}
		letto := got[ancoraggio.RifNodo(shaAssiemeCAD, "#276")].Segnali[0]
		if !reflect.DeepEqual(letto.ControIndizi, []string{ancoraggio.ControIndizioLetturaIdentita}) ||
			!strings.HasPrefix(letto.Spiegazione, "Il nome del pezzo nel file contiene «Saldatura»;") ||
			!strings.HasSuffix(letto.Spiegazione, " Ha anche un codice letto (7120155): controlla che non sia un pezzo vero.") {
			t.Errorf("il cordone con un codice letto: %+v", letto)
		}
		diversi := got[ancoraggio.RifNodo(shaAssiemeCAD, "#277")].Segnali[0]
		if !strings.HasPrefix(diversi.Spiegazione, "L'id del pezzo nel file contiene «WELD_SEAM» e il nome «CORDONE»;") || len(diversi.ControIndizi) != 0 {
			t.Errorf("parole diverse nell'id e nel nome: %+v", diversi)
		}
		doppio := got[ancoraggio.RifNodo(shaAssiemeCAD, "#278")].Segnali[0]
		if len(doppio.Riscontri) != 1 || !reflect.DeepEqual(doppio.Riscontri[0].Parole, []string{"bead", "bead"}) ||
			!strings.HasPrefix(doppio.Spiegazione, "L'id del pezzo nel file contiene «bead»;") {
			t.Errorf("due parole nello stesso campo: %+v", doppio)
		}
		fillet := got[ancoraggio.RifNodo(shaAssiemeCAD, "#202")].Segnali[0]
		if want := []string{"FILLET", "WELD_ID"}; !reflect.DeepEqual(fillet.Riscontri[0].Parole, want) || !strings.HasPrefix(fillet.Spiegazione, "L'id e il nome del pezzo nel file contengono «FILLET» e «WELD_ID»;") {
			t.Errorf("il corpo FILLET_WELD_ID_3: %+v", fillet)
		}
	})

	// La forma del segnale nel JSON, scritta a mano (non con json.Marshal del tipo): è quella che il registro
	// d'importazione copia in nodi[].segnali[] e che entra nelle sue impronte (EM §3.8.1). Senza contro-indizi la chiave
	// contro_indizi non c'è (omitempty, AD-LS02-2); con un codice letto c'è, prima della spiegazione.
	t.Run("il JSON del segnale", func(t *testing.T) {
		spiegazione := "possibile elemento CAD (un cordone di saldatura o simile), non un articolo. È solo un segnale: da solo non toglie il nodo dalla proposta né il " +
			"suo 2D, e la decisione è tua."
		attesi := map[string]string{
			"#100": `[{"codice":"ancoraggio.possibile_elemento_cad","regola":"elemento_cad_nome_foglia_v1","riscontri":[` +
				`{"campo":"id","unita_id":"u:step:#100:id","parole":["Saldatura"],"intervalli":[{"inizio":15,"fine":24}]},` +
				`{"campo":"nome","unita_id":"u:step:#100:nome","parole":["Saldatura"],"intervalli":[{"inizio":15,"fine":24}]}],` +
				`"spiegazione":"L'id e il nome del pezzo nel file contengono «Saldatura»; il nodo non ha figli e non è una radice del file: ` +
				spiegazione + `"}]`,
			"#276": `[{"codice":"ancoraggio.possibile_elemento_cad","regola":"elemento_cad_nome_foglia_v1","riscontri":[` +
				`{"campo":"nome","unita_id":"u:step:#276:nome","parole":["Saldatura"],"intervalli":[{"inizio":0,"fine":9}]}],` +
				`"contro_indizi":["lettura_identita"],` +
				`"spiegazione":"Il nome del pezzo nel file contiene «Saldatura»; il nodo non ha figli e non è una radice del file: ` +
				spiegazione + ` Ha anche un codice letto (7120155): controlla che non sia un pezzo vero."}]`,
		}
		for chiave, want := range attesi {
			b, err := json.Marshal(got[ancoraggio.RifNodo(shaAssiemeCAD, chiave)].Segnali)
			if err != nil || string(b) != want {
				t.Errorf("%s: il JSON del segnale\n%s\natteso\n%s (%v)", chiave, b, want, err)
			}
		}
	})

	// I motivi sono quelli che la funzione SQL della completezza (0020, struttura_motivo_parziale) dà davvero a questi
	// file: un file a un solo nodo non ha occorrenze di assieme, un file con due radici non è una distinta. Con il grafo non
	// completo ogni segnale ha il contro-indizio grafo_non_completo, anche quando il motivo non può nascondere figli (le
	// due radici): è il limite dichiarato dell'addendum (§5; la proposta P-5 del resoconto della corsia), fissato qui.
	t.Run("le radici e i file a un nodo", func(t *testing.T) {
		casi := []struct {
			nome   string
			radici []string
			nodi   []nodoCAD
			archi  []arcoS
			motivo string   // struttura_motivo_parziale del file: "" completo
			attesi []string // le chiavi segnalate
		}{
			{"uno STEP a un solo nodo che si chiama Saldatura", []string{"#1"}, []nodoCAD{{"#1", "Saldatura ACME", "Saldatura ACME", true, 0}}, nil,
				"nessuna occorrenza di assieme: solo parti", nil},
			{"una radice con «re. weld.» e un articolo", []string{"#1"},
				[]nodoCAD{{"#1", "ACME-000 - supporting frame re. weld.", "ACME-000 - supporting frame re. weld.", false, 0}, {"#2", "ACME - Weld nut M16", "ACME - Weld nut M16", false, 1}},
				[]arcoS{{"#1", "#2", 2, nil}}, "", nil},
			{"una radice con WELDED e la minuteria da saldare", []string{"#1"},
				[]nodoCAD{{"#1", "ACME-9000.002_0-LH STABILIZER WELDED SUPPORT", "ACME-9000.002_0-LH STABILIZER WELDED SUPPORT", false, 0}, {"#2", "ACME-9001.003_1-WELD NUT M6x1", "ACME-9001.003_1-WELD NUT M6x1", false, 1}},
				[]arcoS{{"#1", "#2", 4, nil}}, "", nil},
			{"una radice che si chiama Saldatura con un cordone sotto", []string{"#1"},
				[]nodoCAD{{"#1", "Saldatura radice ACME", "Saldatura radice ACME", false, 0}, {"#2", "ACME_222_Saldatura d'angolo 2_oa_0", "ACME_222_Saldatura d'angolo 2_oa_0", true, 0}},
				[]arcoS{{"#1", "#2", 1, nil}}, "", []string{"#2"}},
			{"due radici, una è una foglia che si chiama Saldatura", []string{"#1", "#5"},
				[]nodoCAD{{"#1", "7120901", "TELAIO ACME", false, 0}, {"#2", "SALDATURA_D_ANGOLO_ID_9", "SALDATURA_D_ANGOLO_ID_9", false, 1}, {"#5", "Saldatura isolata ACME", "Saldatura isolata ACME", true, 0}},
				[]arcoS{{"#1", "#2", 1, nil}}, "2 radici nel file: una distinta ne ha una", []string{"#2"}},
		}
		for i, c := range casi {
			t.Run(c.nome, func(t *testing.T) {
				id, sha := uidS(0x7d0+i), strings.Repeat(fmt.Sprintf("%02x", 0x7d+i), 32)
				got, sf, _ := nodiCAD(t, m, id, fmt.Sprintf("ACME caso %d.stp", i), sha, fattiCAD(t, formaV3, sha, c.radici, c.nodi, c.archi, c.motivo), c.radici)
				if sf.GrafoCompleto != (c.motivo == "") || sf.MotivoGrafo != c.motivo {
					t.Fatalf("GrafoCompleto %v, MotivoGrafo %q: atteso il motivo %q", sf.GrafoCompleto, sf.MotivoGrafo, c.motivo)
				}
				var segnalati []string
				for _, n := range c.nodi {
					x, ok := got[ancoraggio.RifNodo(sha, n.chiave)]
					switch {
					case !ok:
						t.Errorf("il nodo %s non c'è", n.chiave)
					case len(x.Segnali) > 0:
						segnalati = append(segnalati, n.chiave)
						if slices.Contains(x.Segnali[0].ControIndizi, ancoraggio.ControIndizioGrafoNonCompleto) != (c.motivo != "") ||
							strings.HasSuffix(x.Segnali[0].Spiegazione, " Il grafo del file non è completo ("+c.motivo+"): che il nodo non abbia figli potrebbe non essere un'informazione.") != (c.motivo != "") {
							t.Errorf("%s con il motivo %q: contro-indizi %v, spiegazione %q", n.chiave, c.motivo, x.Segnali[0].ControIndizi, x.Segnali[0].Spiegazione)
						}
					}
				}
				if !reflect.DeepEqual(segnalati, c.attesi) {
					t.Errorf("segnalati %v, attesi %v", segnalati, c.attesi)
				}
			})
		}
	})

	// La radice di una struttura che nel file non è una radice (il prodotto come nodo interno: una BOM di lavoro o una
	// candidata con RadiceDelFile falso) e che ha la parola: il segnale guarda il file, quindi c'è anche su di lei, uguale
	// a quello dello stesso nodo nella struttura della radice del file. È il limite dichiarato dell'addendum (§5): la
	// radice di una struttura è il prodotto, che non si può non includere (PRODOTTO_PROTETTO, EM §7.3), e la pagina non
	// lo propone per «Non includere»; la proposta P-6 del resoconto della corsia dice le alternative.
	t.Run("la radice di una struttura che nel file è una foglia", func(t *testing.T) {
		id, sha := uidS(0x7df), strings.Repeat("e7", 32)
		nodi := []nodoCAD{{"#1", "7120902", "RUOTA ACME", false, 0}, {"#2", "ACME BEAD RING 15", "ACME BEAD RING 15", false, 1}}
		got, _, strutture := nodiCAD(t, m, id, "ACME ruota.stp", sha, fattiCAD(t, formaV3, sha, []string{"#1"}, nodi, []arcoS{{"#1", "#2", 1, nil}}, ""),
			[]string{"#1", "#2"})
		anello := ancoraggio.RifNodo(sha, "#2")
		trovata := false
		for _, s := range strutture {
			if s.Radice != anello {
				continue
			}
			trovata = true
			if s.RadiceDelFile || len(s.Nodi) != 1 || s.Nodi[0].Rif != anello || len(s.Nodi[0].Segnali) != 1 ||
				!reflect.DeepEqual(s.Nodi[0].Segnali, got[anello].Segnali) {
				t.Errorf("la struttura con la radice #2 (radice del file %v, nodi %d): segnali %+v", s.RadiceDelFile, len(s.Nodi), s.Nodi)
			}
		}
		if !trovata {
			t.Fatal("nessuna struttura con la radice #2")
		}
	})

	t.Run("la scena ACME di sempre: la saldatura che nessuna famiglia legge", func(t *testing.T) {
		strutture, _ := proponiS(t, targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: scenaACME(t, m, "")})
		visti := 0
		for _, s := range strutture {
			for _, n := range s.Nodi {
				saldatura := n.Rif == ancoraggio.RifNodo(shaAssieme100, "#5")
				if saldatura != (len(n.Segnali) > 0) {
					t.Errorf("%s/%s: segnali %+v", s.Target, n.Rif, n.Segnali)
				}
				if saldatura {
					visti++
					if r := n.Segnali[0].Riscontri; len(r) != 1 || r[0].Campo != ancoraggio.CampoGrezzoID || !reflect.DeepEqual(r[0].Parole, []string{"SALDATURA"}) {
						t.Errorf("la saldatura SALDATURA_1: %+v", n.Segnali)
					}
				}
			}
		}
		if visti == 0 {
			t.Error("la saldatura della scena ACME non c'è")
		}
	})
}

// controllaRiscontri: i riscontri di un segnale sono quelli dei grezzi del nodo nel file: un riscontro per ogni campo con
// la parola, nell'ordine id, nome, con l'unità del grezzo; ogni parola è il testo del grezzo nel suo intervallo, ed è una
// parola della regola con i suoi confini.
func controllaRiscontri(t *testing.T, sf ancoraggio.StrutturaFile, rif string, nodo nodoCAD, s ancoraggio.SegnaleNodo) {
	t.Helper()
	parola := regexp.MustCompile(`(?i)^(?:saldatura|cordone|fillet|bead|weld[ _-]*(?:bead|seam|id))$`)
	espressione := regexp.MustCompile(ancoraggio.EspressioneElementoCADV1)
	var grezzi []ancoraggio.ValoreGrezzo
	for _, n := range sf.Nodi {
		if n.Rif == rif {
			grezzi = n.Grezzi
		}
	}
	var attesi []string
	for _, g := range grezzi {
		if espressione.MatchString(g.Testo) {
			attesi = append(attesi, g.Campo+"|"+g.UnitaID)
		}
	}
	var visti []string
	for _, r := range s.Riscontri {
		visti = append(visti, r.Campo+"|"+r.UnitaID)
		testo := nodo.id
		if r.Campo == ancoraggio.CampoGrezzoNome {
			testo = nodo.nome
		}
		if len(r.Parole) == 0 || len(r.Parole) != len(r.Intervalli) {
			t.Errorf("%s: riscontro %+v", nodo.nome, r)
			continue
		}
		for i, p := range r.Parole {
			iv := r.Intervalli[i]
			if iv.Inizio < 0 || iv.Fine > len(testo) || testo[iv.Inizio:iv.Fine] != p || !parola.MatchString(p) ||
				(iv.Inizio > 0 && lettera(testo[iv.Inizio-1])) || (iv.Fine < len(testo) && lettera(testo[iv.Fine])) {
				t.Errorf("%s: la parola %q in %+v non è nel testo %q con i suoi confini", nodo.nome, p, iv, testo)
			}
		}
	}
	if !reflect.DeepEqual(visti, attesi) {
		t.Errorf("%s: riscontri %v, attesi %v (un riscontro per campo con la parola, nell'ordine id, nome)", nodo.nome, visti, attesi)
	}
}

func lettera(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// TestQ01EquivalenzaConLEspressionePython (Q-01): l'espressione Go della regola (EspressioneElementoCADV1) dà lo stesso
// esito dell'espressione Python della misura del 17/10 sugli stessi testi. L'esito Python è quello dato da Python 3 con
// l'espressione di EM §11.1 e re.I (lo script, fuori dal repository, sta nel resoconto della corsia motore di V1): qui è
// scritto per esteso. Le tre differenze note fuori dall'ASCII (le i turche) sono fissate a parte, come limite dichiarato:
// vale l'espressione Go.
func TestQ01EquivalenzaConLEspressionePython(t *testing.T) {
	espressione := regexp.MustCompile(ancoraggio.EspressioneElementoCADV1)
	python := map[string]bool{
		"": false, "12345_oa_0": false, "2bead": true, "7120155": false, "ACME-9000.002_0-LH STABILIZER WELDED SUPPORT": false,
		"ACME-9001.003_1-WELD NUT M6x1": false, "ACME-9001.206_1-WELD BOLT M6x1x20": false, "99000001": false, "9Z00003_WELD": false,
		"ACME - Weld nut M16": false, "ACME - weld-on link spring relief": false, "ACME - welding bush": false, "ACME BEAD RING 15": true,
		"ACME-000 - supporting frame re. weld.": false, "ACME-P-001": false, "ACME_111_2.000_Saldatura d'angolo 1_oa_0": true,
		"ACME_111_2.000_Saldatura d'angolo 1_oa_50": true, "BEAD": true, "CORDONE_3": true, "FILLET-3": true, "FILLET_WELD_ID": true,
		"FILLET_WELD_ID_3": true, "SALDATURA_1": true, "SALDATURA_D_ANGOLO_ID_7": true, "SALDATURA_TAPPO_ID_9_PRT": true,
		"SALDATURA_TESTA_ID_8": true, "Saldatura ACME": true, "Saldatura d'angolo 3": true, "Saldatura gruppo ACME": true,
		"Schweißnaht ACME": false, "TELAIO ACME": false, "WELD-ID": true, "WELDS_9Z00002": false, "WELD_9Z00001": false, "abead": false,
		"bEaD": true, "bead": true, "bead2": true, "bead_bead": true, "beads": false, "beadß": true, "beadé": true, "bead\u017f": false,
		"bead\u212a": false, "cordone": true, "cordoni": false, "fillet": true, "fillets": false, "saldaturA": true, "saldatura_x": true,
		"saldature ACME": false, "weld": false, "weld   seam": true, "weld _-_ id": true, "weld bead": true, "weld seam id": true,
		"weld_identifier": false, "weld_seam": true, "weldbead": true, "weldid": true, "wel\u212a": false, "xsaldatura": false, "ébead": true,
		"\u017faldatura": true,
	}
	for testo, atteso := range python {
		if got := espressione.MatchString(testo); got != atteso {
			t.Errorf("%q: Go %v, Python %v", testo, got, atteso)
		}
	}
	// Il limite dichiarato: Python con re.I tratta la I con il punto (U+0130) e la i senza punto (U+0131) come «i», anche
	// come confine; RE2 no. La s lunga (U+017F) e il segno del kelvin (U+212A) li piegano tutti e due (sopra).
	limite := map[string]bool{"\u0131bead": true, "\u0130bead": true, "f\u0131llet": false}
	for testo, goAtteso := range limite {
		if got := espressione.MatchString(testo); got != goAtteso {
			t.Errorf("%q: Go %v, il limite dichiarato dice %v (Python dà il contrario)", testo, got, goAtteso)
		}
	}

	// Le stesse attese passano dalla regola vera, non solo dalla costante: dai fatti a ProponiStrutture, uno STEP con un
	// figlio foglia per testo (il testo nell'id, il nome vuoto). Così conta l'espressione che segnaliElementoCAD usa
	// davvero, compilata nel pacchetto.
	attese := map[string]bool{}
	for testo, atteso := range python {
		attese[testo] = atteso
	}
	for testo, goAtteso := range limite {
		attese[testo] = goAtteso
	}
	testi := make([]string, 0, len(attese))
	for testo := range attese {
		testi = append(testi, testo)
	}
	slices.Sort(testi)
	nodi := []nodoCAD{{"#1", "7120999", "TELAIO ACME", false, 0}}
	var archi []arcoS
	for i, testo := range testi {
		chiave := fmt.Sprintf("#%d", 10+i)
		nodi = append(nodi, nodoCAD{chiave, testo, "", false, 1})
		archi = append(archi, arcoS{"#1", chiave, 1, nil})
	}
	id, sha := uidS(0x7de), strings.Repeat("e6", 32)
	got, _, _ := nodiCAD(t, motoreStrutture(t), id, "ACME testi.stp", sha, fattiCAD(t, formaV3, sha, []string{"#1"}, nodi, archi, ""), []string{"#1"})
	for i, testo := range testi {
		n, ok := got[ancoraggio.RifNodo(sha, fmt.Sprintf("#%d", 10+i))]
		if !ok {
			t.Errorf("%q: il nodo non c'è", testo)
			continue
		}
		if (len(n.Segnali) > 0) != attese[testo] {
			t.Errorf("%q: dalla regola segnalato %v, atteso %v", testo, len(n.Segnali) > 0, attese[testo])
		}
	}
}

// TestQ02IndipendenzaDallaGeometriaEDaLS01 (Q-02): lo stesso assieme letto con i fatti v3 senza diagnostica, con la v3 e
// la diagnostica 1 (la geometria per nodo: cordoni senza solido e con un solido, foglie con il codice senza geometria) e
// con la v4 della regressione di GP-3 dà gli stessi nodi segnalati e gli stessi riscontri. La geometria non entra mai. Con
// il grafo non completo (la v4, per la funzione SQL della completezza) ogni segnale ha il contro-indizio
// grafo_non_completo e la frase in più nella spiegazione, senza cambiare l'esito; con la v3 completa no. Il nodo con un
// codice letto ha lettura_identita in tutte e tre.
func TestQ02IndipendenzaDallaGeometriaEDaLS01(t *testing.T) {
	m := motoreStrutture(t)
	nodi, archi := assiemeCAD()
	motivi := map[string]string{formaV3: "", formaV3Diagnostica1: "", formaV4Regressione: "struttura v4: gli scarti non sono numerati, serve la rianalisi v3"}
	esiti := map[string]map[string]ancoraggio.NodoProposto{}
	for _, forma := range []string{formaV3, formaV3Diagnostica1, formaV4Regressione} {
		got, sf, _ := nodiCAD(t, m, idAssiemeCAD, "ACME-030P7120900 00 IN_WORK.stp", shaAssiemeCAD,
			fattiCAD(t, forma, shaAssiemeCAD, []string{"#1"}, nodi, archi, motivi[forma]), []string{"#1"})
		if sf.GrafoCompleto != (motivi[forma] == "") {
			t.Fatalf("%s: GrafoCompleto %v con il motivo %q", forma, sf.GrafoCompleto, motivi[forma])
		}
		esiti[forma] = got
	}
	for rif, n3 := range esiti[formaV3] {
		nd, n4 := esiti[formaV3Diagnostica1][rif], esiti[formaV4Regressione][rif]
		if !reflect.DeepEqual(n3.Segnali, nd.Segnali) {
			t.Errorf("%s: v3 %+v, v3 con la diagnostica %+v: la geometria non deve contare", rif, n3.Segnali, nd.Segnali)
		}
		if len(n3.Segnali) != len(n4.Segnali) {
			t.Errorf("%s: v3 %d segnali, v4 %d: l'esito non dipende da LS-01", rif, len(n3.Segnali), len(n4.Segnali))
			continue
		}
		if len(n3.Segnali) == 0 {
			continue
		}
		s3, s4 := n3.Segnali[0], n4.Segnali[0]
		if slices.Contains(s3.ControIndizi, ancoraggio.ControIndizioGrafoNonCompleto) || !reflect.DeepEqual(s3.Riscontri, s4.Riscontri) {
			t.Errorf("%s: v3 %+v, v4 %+v", rif, s3, s4)
		}
		attesi := append(append([]string(nil), s3.ControIndizi...), ancoraggio.ControIndizioGrafoNonCompleto)
		if !reflect.DeepEqual(s4.ControIndizi, attesi) ||
			s4.Spiegazione != s3.Spiegazione+" Il grafo del file non è completo ("+motivi[formaV4Regressione]+"): che il nodo non abbia figli potrebbe non essere un'informazione." {
			t.Errorf("%s: con il grafo non completo contro-indizi %v e spiegazione %q", rif, s4.ControIndizi, s4.Spiegazione)
		}
	}
	for _, forma := range []string{formaV3, formaV3Diagnostica1, formaV4Regressione} {
		if s := esiti[forma][ancoraggio.RifNodo(shaAssiemeCAD, "#276")].Segnali; len(s) != 1 || !slices.Contains(s[0].ControIndizi, ancoraggio.ControIndizioLetturaIdentita) {
			t.Errorf("%s: il cordone con un codice letto %+v", forma, s)
		}
	}
}

// nodoPropostoPrima: il NodoProposto di ancoraggio-2, campo per campo, con gli stessi tag: il JSON «di prima».
type nodoPropostoPrima struct {
	Rif                string                         `json:"rif"`
	AllegatoID         uuid.UUID                      `json:"allegato_id"`
	EntitaID           string                         `json:"entita_id"`
	Codice             ancoraggio.CatenaCodice        `json:"codice"`
	Padri              []string                       `json:"padri,omitempty"`
	Leggibile          bool                           `json:"leggibile"`
	SenzaFile          bool                           `json:"senza_file"`
	Decisione          *ancoraggio.ComponenteDeciso   `json:"decisione,omitempty"`
	RigaLegacy         *ancoraggio.RigaPropostaLegacy `json:"riga_legacy,omitempty"`
	RigaDecisa         *ancoraggio.RigaDecisaLegacy   `json:"riga_decisa,omitempty"`
	DecisoDaPersona    bool                           `json:"deciso_da_persona"`
	AbbinamentoPerBase *ancoraggio.AbbinamentoPerBase `json:"abbinamento_per_base,omitempty"`
}

func primaDi(n ancoraggio.NodoProposto) nodoPropostoPrima {
	return nodoPropostoPrima{Rif: n.Rif, AllegatoID: n.AllegatoID, EntitaID: n.EntitaID, Codice: n.Codice, Padri: n.Padri, Leggibile: n.Leggibile,
		SenzaFile: n.SenzaFile, Decisione: n.Decisione, RigaLegacy: n.RigaLegacy, RigaDecisa: n.RigaDecisa, DecisoDaPersona: n.DecisoDaPersona,
		AbbinamentoPerBase: n.AbbinamentoPerBase}
}

// TestQ02bIlJSONDeiNodiSenzaSegnaliNonCambia (Q-02b): con i campi di prima riportati uno per uno nel tipo di ancoraggio-2,
// il JSON di ogni nodo senza segnali è byte per byte quello di prima (omitempty), sulle strutture della scena ACME (con
// le catene del codice) e dell'assieme degli esempi (con due righe legacy aperte); quello di un nodo segnalato è quello
// di prima con «"segnali":[…]» in fondo. Il resto dell'esito cambia solo per la versione del servizio e le impronte: lo
// dice la non regressione di P7e in valutazione, che normalizza solo quelle.
func TestQ02bIlJSONDeiNodiSenzaSegnaliNonCambia(t *testing.T) {
	m := motoreStrutture(t)
	strutture, _ := proponiS(t, targetScenaACME(t, m), ancoraggio.ContestoStrutturale{Strutture: scenaACME(t, m, "")})
	nodi, archi := assiemeCAD()
	cad, _, _ := nodiCAD(t, m, idAssiemeCAD, "ACME-030P7120900 00 IN_WORK.stp", shaAssiemeCAD,
		fattiCAD(t, formaV3, shaAssiemeCAD, []string{"#1"}, nodi, archi, ""), []string{"#1"}, righeCAD()...)
	var tutti []ancoraggio.NodoProposto
	for _, s := range strutture {
		tutti = append(tutti, s.Nodi...)
	}
	for _, n := range cad {
		tutti = append(tutti, n)
	}
	senza, con := 0, 0
	for _, n := range tutti {
		dopo, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		prima, err := json.Marshal(primaDi(n))
		if err != nil {
			t.Fatal(err)
		}
		if len(n.Segnali) == 0 {
			senza++
			if string(dopo) != string(prima) {
				t.Errorf("%s: senza segnali il JSON è cambiato:\n%s\n%s", n.Rif, dopo, prima)
			}
			continue
		}
		con++
		segnali, err := json.Marshal(n.Segnali)
		if err != nil {
			t.Fatal(err)
		}
		if want := string(prima[:len(prima)-1]) + `,"segnali":` + string(segnali) + "}"; string(dopo) != want {
			t.Errorf("%s: con i segnali il JSON non è quello di prima con il campo in fondo:\n%s\n%s", n.Rif, dopo, want)
		}
	}
	if senza == 0 || con == 0 {
		t.Errorf("%d nodi senza segnali e %d con: la prova vuole tutti e due", senza, con)
	}
}
