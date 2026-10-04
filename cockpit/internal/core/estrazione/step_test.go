// L1 — l'adattatore STEP (A1b-14; piano A, 5.4.5, «Mappatura STEP»): oltre ai golden di F-STEP-1…10
// (golden_test.go), le proprietà della mappatura che un golden da solo non dice: l'ordine dei fatti non cambia
// il documento; un nodo e i suoi attributi stanno insieme solo per EntitaID, e le relazioni solo per i legami
// del worker; le formazioni non passano da un nodo all'altro; uno STEP senza fatti non è uno STEP senza codici;
// la completezza viene solo dal motivo del caricatore; i conteggi in code point Python.
//
// Tutti i dati sono sintetici (ACME, codici di fantasia, UUID 00000000-0000-4000-8000-0000000000nn): il
// repository è pubblico.
package estrazione

import (
	"encoding/json"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// strutturaACME: un payload con la struttura v3 data per nodi e relazioni già scritti in JSON.
func strutturaACME(radici, nodi, relazioni, scarti string) string {
	return `{"esito":{"tipo_proposto":"cad_3d","codice":"ACME0000000","rev":"9"},"struttura":{"versione":3,"schema":"AP214",` +
		`"radici":` + radici + `,"nodi":` + nodi + `,"relazioni":` + relazioni + `,"avvisi":[],"scarti":` + scarti + `,` +
		`"limiti":{"nodi_max":2000,"occorrenze_max":50000,"tempo_max_s":120.0,"byte_letti":100,"tempo_s":0.01,"troncato":false,"motivo":""}}}`
}

const scartiZero = `{"prodotti_senza_definizione":0,"occorrenze_non_risolte":0,"occorrenze_su_se_stesse":0,"testi_troncati":0}`

func nodoACME(chiave, id, rev string) string {
	return `{"chiave":"` + chiave + `","id_grezzo":"` + id + `","nome_grezzo":"` + id + `","descrizione_grezza":"","rev_grezza":"` + rev +
		`","evidenza":{"entita":"PRODUCT","riga":"` + chiave + `","formation":"#9` + chiave[1:] + `","definition":"#8` + chiave[1:] + `"}}`
}

func relazioneACME(padre, figlio string, qta int, righe ...string) string {
	r, _ := json.Marshal(righe)
	return `{"padre":"` + padre + `","figlio":"` + figlio + `","qta":` + itoa(qta) +
		`,"evidenza":{"entita":"NEXT_ASSEMBLY_USAGE_OCCURRENCE","righe":` + string(r) + `}}`
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// stepACME: il documento di uno STEP sintetico, con il motivo di completezza dato ("" = completo).
func stepACME(t *testing.T, payload string, motivo *string) evidenze.DocumentoEvidenze {
	t.Helper()
	a := allegatoACME("ACME7000100.stp", ptr("stp"))
	a.Sha256 = ptr(shaACME)
	f := fattiACME(t, shaACME, payload)
	f.MotivoParziale = motivo
	d, err := DaAllegato(a, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestLaMappaturaSTEPNonDipendeDallOrdineDeiFatti: nodi, relazioni e radici in un altro ordine danno lo stesso
// documento, byte per byte (gli ID nascono dalle chiavi del file, l'ordine canonico è per ID). Il digest del
// payload cambia, perché il payload è un altro testo: lo si toglie dal confronto.
func TestLaMappaturaSTEPNonDipendeDallOrdineDeiFatti(t *testing.T) {
	n1, n2, n3 := nodoACME("#10", "ACME7000100", "1"), nodoACME("#20", "ACME7000111", "1"), nodoACME("#30", "ACME7000112", "2")
	r1, r2 := relazioneACME("#10", "#20", 1, "#40"), relazioneACME("#10", "#30", 3, "#41", "#42", "#43")
	a := stepACME(t, strutturaACME(`["#10"]`, "["+n1+","+n2+","+n3+"]", "["+r1+","+r2+"]", scartiZero), ptr(""))
	b := stepACME(t, strutturaACME(`["#10"]`, "["+n3+","+n1+","+n2+"]", "["+r2+","+r1+"]", scartiZero), ptr(""))
	a.Fonte.RiferimentoFatti.DigestPayload, b.Fonte.RiferimentoFatti.DigestPayload = "", ""
	a.BundleID, b.BundleID = "", ""
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("l'ordine dei fatti cambia il documento:\n%s\n%s", ja, jb)
	}
}

// TestNodiEAttributiStannoInsiemeSoloPerEntita (A-C02 nella parte dell'adattatore): ogni unità STEP ha
// l'EntitaID del suo nodo e la chiave del suo nodo nel localizzatore; la formazione di un nodo non compare in un
// altro; le relazioni sono soltanto legami padre_figlio_step, con la quantità e i riferimenti del worker. Il
// codice dell'esito del worker non entra.
func TestNodiEAttributiStannoInsiemeSoloPerEntita(t *testing.T) {
	d := stepACME(t, strutturaACME(`["#10"]`,
		"["+nodoACME("#10", "ACME7000100", "1")+","+nodoACME("#20", "ACME7000111", "2")+"]",
		"["+relazioneACME("#10", "#20", 2, "#40", "#41")+"]", scartiZero), ptr(""))
	for _, u := range d.Unita {
		if u.ID == idUnitaNome {
			continue
		}
		chiave := strings.Split(u.ID, ":")[2]
		if u.EntitaID != "e:step:"+chiave || u.Posizione.STEP == nil || u.Posizione.STEP.Chiave != chiave {
			t.Errorf("unità %s legata a %q con il localizzatore %+v", u.ID, u.EntitaID, u.Posizione.STEP)
		}
		if strings.Contains(u.Testo, "ACME0000000") {
			t.Errorf("l'unità %s porta il codice dell'esito del worker", u.ID)
		}
		radice := chiave == "#10"
		if strings.HasPrefix(u.Selettore.String(), "radice_step.") != radice {
			t.Errorf("unità %s con il selettore %s: le radici vengono da struttura.radici", u.ID, u.Selettore)
		}
	}
	if u, _ := unitaDi(d, "u:step:#10:revisione"); u.Testo != "1" || u.Posizione.STEP.Attributo != "formazione" || u.Selettore.String() != "radice_step.revisione" {
		t.Errorf("formazione della radice %+v", u)
	}
	if u, _ := unitaDi(d, "u:step:#20:revisione"); u.Testo != "2" || u.Selettore.String() != "nodo_step.revisione" {
		t.Errorf("formazione del figlio %+v", u)
	}
	if len(d.Legami) != 1 {
		t.Fatalf("legami %+v", d.Legami)
	}
	l := d.Legami[0]
	if l.Tipo != "padre_figlio_step" || l.Da != "e:step:#10" || l.A != "e:step:#20" || l.Quantita == nil || *l.Quantita != 2 ||
		strings.Join(l.Evidenze, " ") != "#40 #41" || l.Altre != 0 {
		t.Errorf("legame %+v", l)
	}
}

// TestIRiferimentiNAUOSonoAlPiuVenti: il worker elenca al più 20 occorrenze per coppia e conta le altre; se un
// fatto ne elencasse di più, il legame ne tiene 20 e le altre si sommano ad Altre, senza perderne il numero.
func TestIRiferimentiNAUOSonoAlPiuVenti(t *testing.T) {
	var righe []string
	for i := 0; i < 22; i++ {
		righe = append(righe, "#"+itoa(100+i))
	}
	rel := strings.TrimSuffix(relazioneACME("#10", "#20", 25, righe...), "}}") + `,"altre":3}}`
	d := stepACME(t, strutturaACME(`["#10"]`, "["+nodoACME("#10", "ACME7000100", "1")+","+nodoACME("#20", "ACME7000111", "1")+"]",
		"["+rel+"]", scartiZero), ptr(""))
	l := d.Legami[0]
	if len(l.Evidenze) != maxRiferimentiNAUO || l.Evidenze[0] != "#100" || l.Altre != 5 || *l.Quantita != 25 {
		t.Errorf("legame %+v: 20 riferimenti e 5 altre attesi", l)
	}
}

// TestUnoSTEPSenzaFattiNonEUnoSTEPSenzaCodici: uno STEP senza fatti ha step.non_analizzato e la struttura non
// disponibile; la fonte è parziale.
func TestUnoSTEPSenzaFattiNonEUnoSTEPSenzaCodici(t *testing.T) {
	for _, nome := range []string{"ACME7000100.stp", "ACME7000100.STEP"} {
		d, err := DaAllegato(allegatoACME(nome, ptr(strings.ToLower(nome[strings.LastIndexByte(nome, '.')+1:]))), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := capacitaDi(d, capStruttura)
		if d.Qualita.Stato != statoParziale || c.Stato != statoNonDisponibile || strings.Join(codiciDi(d), " ") != CodiceSTEPNonAnalizzato {
			t.Errorf("%s: stato %q, struttura %+v, diagnostiche %v", nome, d.Qualita.Stato, c, codiciDi(d))
		}
		if _, ok := capacitaDi(d, capContenuto); ok {
			t.Errorf("%s: per uno STEP parla la capacità della struttura, non quella del contenuto", nome)
		}
	}
	// Un file che non ha l'estensione dello STEP ma ha la struttura nei fatti si legge come STEP.
	a := allegatoACME("ACME7000100.dat", ptr("dat"))
	f := fattiACME(t, shaACME, strutturaACME(`["#10"]`, "["+nodoACME("#10", "ACME7000100", "1")+"]", `[]`, scartiZero))
	d, err := DaAllegato(a, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := unitaDi(d, "u:step:#10:id"); !ok {
		t.Error("la struttura dei fatti non si è letta")
	}
}

// TestLaCompletezzaVieneSoloDalCaricatore (E-20; R32 b = A): grafo_completo dipende solo da Fatti.MotivoParziale,
// non dallo stato di lettura.
func TestLaCompletezzaVieneSoloDalCaricatore(t *testing.T) {
	payload := strutturaACME(`["#10"]`, "["+nodoACME("#10", "ACME7000100", "1")+"]", `[]`, scartiZero)
	for _, c := range []struct {
		motivo *string
		stato  string
	}{
		{nil, statoNonDisponibile},
		{ptr(""), statoDisponibile},
		{ptr("occorrenze non risolte"), statoParziale},
	} {
		d := stepACME(t, payload, c.motivo)
		g, ok := capacitaDi(d, capGrafoCompleto)
		if !ok || g.Stato != c.stato || (c.motivo != nil && *c.motivo != "" && g.Motivo != *c.motivo) {
			t.Errorf("motivo %v: grafo_completo %+v, atteso %q", c.motivo, g, c.stato)
		}
		if d.Qualita.Stato != statoDisponibile {
			t.Errorf("motivo %v: la completezza ha cambiato lo stato di lettura in %q", c.motivo, d.Qualita.Stato)
		}
	}
}

// TestLaLunghezzaPythonContaLeCoppieSurrogate (5.4.8): due per una runa sopra U+FFFF, uno per le altre; e
// un'unità è candidata troncata solo se il worker ha dichiarato testi troncati.
func TestLaLunghezzaPythonContaLeCoppieSurrogate(t *testing.T) {
	for _, c := range []struct {
		s string
		n int
	}{{"", 0}, {"ACME", 4}, {"è", 1}, {"\U0001F527", 2}, {"A\U0001F527B", 4}} {
		if got := lunghezzaPython(c.s); got != c.n {
			t.Errorf("lunghezzaPython(%q) = %d, attesa %d", c.s, got, c.n)
		}
	}
	lungo := strings.Repeat("X", 199) + "\U0001F527" // 200 rune, 201 code point Python: arriva al tetto
	nodo := strings.Replace(nodoACME("#10", "ACME7000100", "1"), `"nome_grezzo":"ACME7000100"`, `"nome_grezzo":"`+lungo+`"`, 1)
	conTroncati := strings.Replace(scartiZero, `"testi_troncati":0`, `"testi_troncati":1`, 1)
	for _, c := range []struct {
		scarti   string
		troncata bool
	}{{scartiZero, false}, {conTroncati, true}} {
		d := stepACME(t, strutturaACME(`["#10"]`, "["+nodo+"]", `[]`, c.scarti), ptr(""))
		u, _ := unitaDi(d, "u:step:#10:nome")
		if u.Qualita.Troncata != c.troncata {
			t.Errorf("scarti %s: troncata %v, attesa %v", c.scarti, u.Qualita.Troncata, c.troncata)
		}
	}
}

// TestDueChiamateDannoLoStessoDocumento: niente orologio né caso, quindi lo stesso ingresso dà gli stessi byte.
func TestDueChiamateDannoLoStessoDocumento(t *testing.T) {
	payload := strutturaACME(`["#10"]`, "["+nodoACME("#10", "ACME7000100", "1")+","+nodoACME("#20", "ACME7000111", "1")+"]",
		"["+relazioneACME("#10", "#20", 1, "#40")+"]", scartiZero)
	a, b := stepACME(t, payload, ptr("")), stepACME(t, payload, ptr(""))
	if a.BundleID != b.BundleID {
		t.Errorf("BundleID %s e %s", a.BundleID, b.BundleID)
	}
}
