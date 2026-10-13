package motore_a

// L1 — giro 5, A1d, notte 14/10, WP-U (U3, U4): gli STEP della RFQ per un prodotto senza strutture candidate
// (Indice.StepDelThread: le radici lette dal documento delle evidenze e il gesto esistente che porta all'autorizzazione)
// e gli elementi della mail (ElementoDellaMail, ElementiDellaMail). Pure: nessun database, nessun orologio.
//
// Solo dati ACME inventati: codici ACM-nnnn, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// maDocSTEP: il documento delle evidenze di uno STEP: le entità nodo_step con le chiavi date, le radici con le unità
// del contesto radice_step (testi per campo: "id", "nome"; nessuno = solo la chiave), gli altri nodi nel contesto
// nodo_step.
func maDocSTEP(nodi []string, radici map[string]map[string]string) evidenze.DocumentoEvidenze {
	var d evidenze.DocumentoEvidenze
	for _, k := range nodi {
		id := "e:step:" + k
		d.Entita = append(d.Entita, evidenze.EntitaLocale{ID: id, Tipo: "nodo_step", ChiaveOriginale: k})
		contesto := evidenze.ContestoNodoSTEP
		campi, radice := radici[k]
		if radice {
			contesto = evidenze.ContestoRadiceSTEP
		}
		// il nome prima dell'id: la lettura sceglie l'id per campo, non per ordine
		for _, campo := range []string{"nome", "id"} {
			testo, ok := campi[campo]
			if !ok && radice {
				continue
			}
			if !ok {
				testo = "nodo " + k
			}
			d.Unita = append(d.Unita, evidenze.UnitaEvidenza{ID: "u:step:" + k + ":" + campo, EntitaID: id, Testo: testo,
				Selettore: evidenze.Selettore{Contesto: contesto, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteStep, Valore: campo}}})
		}
		if radice && len(campi) == 0 {
			// una radice senza testi: un'unità senza campo letto, per segnarla come radice
			d.Unita = append(d.Unita, evidenze.UnitaEvidenza{ID: "u:step:" + k + ":descrizione", EntitaID: id, Testo: "",
				Selettore: evidenze.Selettore{Contesto: contesto, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteStep, Valore: "descrizione"}}})
		}
	}
	return d
}

// maScenaStep: il prodotto ACM-1000 (componente 10, senza strutture) e sei allegati:
//
//	31 STEP, documento corrente del componente 10 (→ autorizza), due radici: una con id e nome, una con il solo nome;
//	32 STEP con una proposta aperta di 3D con un altro codice, associata al componente 20 (→ nessun gesto: il
//	   componente 10 ha già il 3D corrente 51; senza, usa con la correzione: TestStepDelThreadUsaSenza3DCorrente);
//	33 STEP con una proposta aperta di 3D senza codice (→ nessun gesto per lo stesso motivo; senza il 3D, usa senza
//	   correzione);
//	34 STEP con una proposta aperta di 2D (→ nessun gesto);
//	35 STEP, documento corrente del componente 20 (→ nessun gesto, si sposta dal Fascicolo);
//	36 STEP senza proposta (→ nessun gesto);
//	37 PDF senza nodi (non è nell'elenco); 38 elemento della mail (natura_non_file, non è nell'elenco).
func maScenaStep() (*fotorfq.Thread, *valutazione.EsitoThread) {
	th := &fotorfq.Thread{ID: maUUID(1), ClienteID: maUUID(2),
		Componenti: []fotorfq.Componente{{ID: maUUID(10), Codice: "ACM-1000"}, {ID: maUUID(20), Codice: "ACM-2000"}}}
	nomi := map[int]string{31: "ACM-1000_A.stp", 32: "ACM-9001.stp", 33: "senza_codice.stp", 34: "ACM-9002.stp", 35: "ACM-2000.stp",
		36: "ACM-9003.stp", 37: "ACM-1000.pdf", 38: "image001.png"}
	for nn := 31; nn <= 38; nn++ {
		th.Allegati = append(th.Allegati, fotorfq.Allegato{ID: maUUID(nn), NomeFile: nomi[nn]})
	}
	th.Documenti = []fotorfq.DocumentoConfermato{
		{ID: maUUID(51), ComponenteID: maUUIDp(10), Tipo: "cad_3d", Allegati: []uuid.UUID{maUUID(31)}},
		{ID: maUUID(52), ComponenteID: maUUIDp(20), Tipo: "cad_3d", Allegati: []uuid.UUID{maUUID(35)}},
		// un documento sostituito non conta
		{ID: maUUID(53), ComponenteID: maUUIDp(20), Tipo: "cad_3d", Allegati: []uuid.UUID{maUUID(36)}, SostituitoDa: maUUIDp(52)},
	}
	aperta, decisa := string(db.StatoPropostaAperta), string(db.StatoPropostaConfermata)
	th.Proposte = []fotorfq.PropostaAttuale{
		{ID: maUUID(71), AllegatoID: maUUID(32), Tipo: "cad_3d", Codice: maStr(" acm-9001 "), ComponenteID: maUUIDp(20), Stato: aperta},
		{ID: maUUID(72), AllegatoID: maUUID(33), Tipo: "cad_3d", Stato: aperta},
		{ID: maUUID(73), AllegatoID: maUUID(34), Tipo: "disegno_2d", Codice: maStr("ACM-9002"), Stato: aperta},
		{ID: maUUID(74), AllegatoID: maUUID(36), Tipo: "cad_3d", Codice: maStr("ACM-9003"), Stato: decisa},
	}
	e := &valutazione.EsitoThread{ThreadID: maUUID(1), Valutato: true,
		ProdottiValutati: []valutazione.ProdottoValutato{{Rif: maP1, ComponenteID: maUUIDp(10), CodiceRichiesto: "ACM-1000", Struttura: "nessuna"},
			{Rif: "identificativo:ACM-3000", CodiceRichiesto: "ACM-3000", Struttura: "nessuna"}}}
	due := map[string]map[string]string{"#1": {"id": "ACM-5001_01  -  staffa", "nome": "staffa"}, "#4": {"nome": "solo nome"}}
	e.File = []valutazione.FileInterpretato{
		{AllegatoID: maUUID(31), Documento: maDocSTEP([]string{"#1", "#2", "#3", "#4"}, due)},
		{AllegatoID: maUUID(32), Documento: maDocSTEP([]string{"#1", "#2"}, map[string]map[string]string{"#1": {"id": "ACM-5002"}})},
		{AllegatoID: maUUID(33), Documento: maDocSTEP([]string{"#7"}, map[string]map[string]string{"#7": {}})},
		{AllegatoID: maUUID(34), Documento: maDocSTEP([]string{"#1"}, map[string]map[string]string{"#1": {"id": "ACM-5003"}})},
		{AllegatoID: maUUID(35), Documento: maDocSTEP([]string{"#1"}, map[string]map[string]string{"#1": {"id": "ACM-5004"}})},
		{AllegatoID: maUUID(36), Documento: maDocSTEP([]string{"#1", "#2"}, nil)},
		{AllegatoID: maUUID(37)},
		{AllegatoID: maUUID(38), Motivo: valutazione.MotivoFileNaturaNonFile},
	}
	return th, e
}

func TestStepDelThreadGestiERadici(t *testing.T) {
	th, e := maScenaStep()
	x := NuovoIndice(th, e, nil, nil)
	got := x.StepDelThread(maP1)
	if len(got) != 6 {
		t.Fatalf("%d STEP, attesi 6 (31-36: né il PDF senza nodi né l'elemento della mail)", len(got))
	}
	type riga struct {
		All        int
		Gesto      string
		Correzione bool
		Nodi       int
		Radici     string
		ConMotivo  bool
	}
	var righe []riga
	for _, s := range got {
		var rr []string
		for _, r := range s.Radici {
			rr = append(rr, r.Chiave+"="+r.Testo)
		}
		n := 0
		for nn := 31; nn <= 38; nn++ {
			if s.AllegatoID == maUUID(nn) {
				n = nn
			}
		}
		righe = append(righe, riga{n, s.Gesto, s.Correzione, s.Nodi, strings.Join(rr, ";"), s.Motivo != ""})
		if s.CodiceComponente != "ACM-1000" {
			t.Errorf("%s: codice del componente %q", s.NomeFile, s.CodiceComponente)
		}
	}
	want := []riga{
		{31, GestoAutorizza, false, 4, "#1=ACM-5001_01  -  staffa;#4=solo nome", false},
		{32, "", false, 2, "#1=ACM-5002", true},
		{33, "", false, 1, "#7=#7", true},
		{34, "", false, 1, "#1=ACM-5003", true},
		{35, "", false, 1, "#1=ACM-5004", true},
		{36, "", false, 2, "", true},
	}
	if !reflect.DeepEqual(righe, want) {
		t.Errorf("STEP:\n got %+v\nwant %+v", righe, want)
	}
	if d := got[0].Documento; d == nil || *d != maUUID(51) {
		t.Errorf("31: il documento per l'autorizzazione %v, atteso 51", d)
	}
	if p := got[1].Proposta; p == nil || p.ID != maUUID(71) || p.Codice != "acm-9001" || p.ComponenteID == nil || *p.ComponenteID != maUUID(20) {
		t.Errorf("32: la proposta %+v", p)
	}
	// revisione della notte 14/10: con il 3D corrente del componente la catena «Usa come STEP» non si offre
	for _, i := range []int{1, 2} {
		if got[i].Motivo != "ACM-1000 ha già un 3D corrente: si aggiunge o si sostituisce dalla pagina del Fascicolo" || got[i].Proposta == nil {
			t.Errorf("%s: il motivo %q, la proposta %+v", got[i].NomeFile, got[i].Motivo, got[i].Proposta)
		}
	}
	if !strings.Contains(got[3].Motivo, "non è un 3D") || !strings.Contains(got[4].Motivo, "ACM-2000") ||
		!strings.Contains(got[5].Motivo, "non ha una proposta aperta") {
		t.Errorf("motivi: %q, %q, %q", got[3].Motivo, got[4].Motivo, got[5].Motivo)
	}
	// una copia: cambiarla non cambia la ricerca successiva
	got[1].Proposta.Codice = "cambiato"
	*got[1].Proposta.ComponenteID = maUUID(99)
	if again := x.StepDelThread(maP1); again[1].Proposta.Codice != "acm-9001" || *again[1].Proposta.ComponenteID != maUUID(20) {
		t.Error("StepDelThread non dà copie della proposta")
	}

	// il prodotto senza componente: gli STEP ci sono, nessun gesto, il motivo lo dice
	for _, s := range x.StepDelThread("identificativo:ACM-3000") {
		if s.Gesto != "" || !strings.Contains(s.Motivo, "non ha ancora un componente") || s.CodiceComponente != "" {
			t.Errorf("senza componente: %+v", s)
		}
	}
	if x.StepDelThread(maComp(99)) != nil {
		t.Error("un prodotto che non c'è ha degli STEP")
	}
	var nilx *Indice
	if nilx.StepDelThread(maP1) != nil || nilx.ElementiDellaMail() != 0 || nilx.ElementoDellaMail(maUUID(38)) || nilx.IndirizzoFascicolo(maP1) != "" {
		t.Error("l'indice nil risponde con qualcosa")
	}
	// senza la fotografia: niente gesti, il motivo
	if s := NuovoIndice(nil, e, nil, nil).StepDelThread(maP1); len(s) != 6 || s[0].Gesto != "" || s[0].Motivo == "" {
		t.Errorf("senza fotografia: %+v", s)
	}
}

// TestStepDelThreadUsaSenza3DCorrente (revisione della notte 14/10): «Usa come STEP» si offre solo se il componente
// del prodotto non ha un 3D corrente. Nella scena il 3D 51 del componente 10 è sostituito (un documento sostituito non
// conta): 32 e 33 hanno la catena, 32 con la correzione del codice; 31 non è più un documento corrente, quindi non ha
// l'autorizzazione (e non ha una proposta). Un 3D corrente di un altro componente (52, di 20) non conta.
func TestStepDelThreadUsaSenza3DCorrente(t *testing.T) {
	th, e := maScenaStep()
	th.Documenti[0].SostituitoDa = maUUIDp(59)
	got := NuovoIndice(th, e, nil, nil).StepDelThread(maP1)
	if len(got) != 6 {
		t.Fatalf("%d STEP, attesi 6", len(got))
	}
	for _, c := range []struct {
		i          int
		gesto      string
		correzione bool
	}{{0, "", false}, {1, GestoUsa, true}, {2, GestoUsa, false}} {
		s := got[c.i]
		if s.Gesto != c.gesto || s.Correzione != c.correzione || (c.gesto == GestoUsa) != (s.Motivo == "") {
			t.Errorf("%s: gesto %q, correzione %v, motivo %q", s.NomeFile, s.Gesto, s.Correzione, s.Motivo)
		}
	}
	if got[0].Documento != nil || !strings.Contains(got[0].Motivo, "non ha una proposta aperta") {
		t.Errorf("31 con il documento sostituito: %+v", got[0])
	}
}

// TestStepDelThreadDeterministico: due calcoli sulla stessa scena danno lo stesso elenco (nessuna mappa scorsa).
func TestStepDelThreadDeterministico(t *testing.T) {
	th, e := maScenaStep()
	primo := NuovoIndice(th, e, nil, nil).StepDelThread(maP1)
	for i := 0; i < 20; i++ {
		if !reflect.DeepEqual(primo, NuovoIndice(th, e, nil, nil).StepDelThread(maP1)) {
			t.Fatal("due calcoli danno elenchi diversi")
		}
	}
}

func TestElementiDellaMailEFascicolo(t *testing.T) {
	th, e := maScenaStep()
	x := NuovoIndice(th, e, nil, nil)
	if !x.ElementoDellaMail(maUUID(38)) || x.ElementoDellaMail(maUUID(37)) || x.ElementoDellaMail(maUUID(99)) {
		t.Error("ElementoDellaMail: solo 38 è un elemento della mail")
	}
	if n := x.ElementiDellaMail(); n != 1 {
		t.Errorf("ElementiDellaMail = %d, atteso 1", n)
	}
	if u := x.IndirizzoFascicolo(maP1); u != "/thread/"+maUUID(1).String()+"/fascicolo?nodo="+maUUID(10).String() {
		t.Errorf("IndirizzoFascicolo del prodotto con componente: %q", u)
	}
	if u := x.IndirizzoFascicolo("identificativo:ACM-3000"); u != "/thread/"+maUUID(1).String()+"/fascicolo" {
		t.Errorf("IndirizzoFascicolo del prodotto senza componente: %q", u)
	}
}

func TestStessoCodiceVista(t *testing.T) {
	for _, c := range []struct {
		a, b string
		ok   bool
	}{{"ACM-1000", "acm-1000", true}, {" ACM-1000 ", "ACM-1000", true}, {"ACM-1000", "ACM-1001", false}, {"", "ACM-1000", false}} {
		if stessoCodiceVista(c.a, c.b) != c.ok {
			t.Errorf("stessoCodiceVista(%q, %q) != %v", c.a, c.b, c.ok)
		}
	}
}

// TestVocabolarioDelleRadiciComeEvidenze: i valori del vocabolario delle evidenze ripetuti come testo in
// step_del_thread.go (il sottopackage non importa evidenze) sono quelli delle costanti.
func TestVocabolarioDelleRadiciComeEvidenze(t *testing.T) {
	if contestoRadiceSTEP != string(evidenze.ContestoRadiceSTEP) || campoIDSTEP != "id" || campoNomeSTEP != "nome" ||
		tipoEntitaNodoSTEP != "nodo_step" || MotivoFileNaturaNonFile != valutazione.MotivoFileNaturaNonFile {
		t.Error("il vocabolario ripetuto in step_del_thread.go non è più quello di evidenze e valutazione")
	}
}
