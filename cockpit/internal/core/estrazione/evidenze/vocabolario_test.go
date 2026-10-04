package evidenze

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

// L1 — il vocabolario della foglia (A1a-VOC; parte 1 §4.1-§4.2, R19 a, R20 c, D-12): undici contesti, la
// tabella chiusa delle coppie contesto/campo, la forma testuale dei selettori andata e ritorno, e gli
// errori con codice e percorso, senza ripieghi.
//
// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un
// dato dell'azienda e questo repository è pubblico; qui, del resto, non c'è nessun cliente: il vocabolario
// dice solo dove si è letto qualcosa.

// TestA1aVOCUndiciContestiChiusi: l'elenco della parte 1 §4.1, né uno di più né uno di meno.
func TestA1aVOCUndiciContestiChiusi(t *testing.T) {
	attesi := []Contesto{"oggetto", "corpo", "storia", "nome_file", "voce_archivio", "cartiglio",
		"elenco_pdf", "testo_pdf", "metadati_pdf", "radice_step", "nodo_step"}
	if !reflect.DeepEqual(contesti, attesi) {
		t.Fatalf("contesti = %v, attesi %v", contesti, attesi)
	}
	if len(variantePerContesto) != len(attesi) {
		t.Fatalf("la tabella delle varianti ha %d contesti, attesi %d", len(variantePerContesto), len(attesi))
	}
	if v, campi := CampiAmmessi("figlio_step"); v != "" || campi != nil {
		t.Fatalf("figlio_step non è un contesto (R19 a): %q %v", v, campi)
	}
}

// TestA1aVOCTabellaDelleCoppie: la parte 1 §4.2, contesto per contesto. Lo STEP ha anche «revisione».
func TestA1aVOCTabellaDelleCoppie(t *testing.T) {
	cartiglio := []string{"codice", "numero_disegno", "revisione", "titolo", "scala", "materiale", "particolare_simile", "sconosciuto"}
	step := []string{"id", "nome", "descrizione", "revisione"}
	elenco := []string{"codice", "descrizione", "revisione", "quantita", "posizione", "sconosciuto"}
	casi := []struct {
		c     Contesto
		v     VarianteCampo
		campi []string
	}{
		{ContestoOggetto, VarianteNessuno, nil},
		{ContestoCorpo, VarianteNessuno, nil},
		{ContestoStoria, VarianteNessuno, nil},
		{ContestoNomeFile, VarianteNessuno, nil},
		{ContestoVoceArchivio, VarianteNessuno, nil},
		{ContestoCartiglio, VarianteCartiglio, cartiglio},
		{ContestoElencoPDF, VarianteElenco, elenco},
		{ContestoTestoPDF, VarianteNessuno, nil},
		{ContestoMetadatiPDF, VarianteNessuno, nil},
		{ContestoRadiceSTEP, VarianteStep, step},
		{ContestoNodoSTEP, VarianteStep, step},
	}
	for _, c := range casi {
		v, campi := CampiAmmessi(c.c)
		if v != c.v || !reflect.DeepEqual(campi, c.campi) {
			t.Errorf("%s: %q %v, attesi %q %v", c.c, v, campi, c.v, c.campi)
		}
	}
	// L'elenco restituito è una copia: cambiarlo non tocca la tabella.
	_, campi := CampiAmmessi(ContestoCartiglio)
	campi[0] = "manomesso"
	if _, diNuovo := CampiAmmessi(ContestoCartiglio); diNuovo[0] != "codice" {
		t.Fatal("CampiAmmessi espone la tabella interna")
	}
}

// TestA1aVOCAndataERitorno: ogni coppia ammessa si legge e si riscrive uguale, anche i contesti senza campo
// («storia», «nome_file»: variante «nessuno», valore vuoto). Anche attraverso il JSON.
func TestA1aVOCAndataERitorno(t *testing.T) {
	n := 0
	for _, c := range contesti {
		v, campi := CampiAmmessi(c)
		testi := []string{string(c)}
		if v != VarianteNessuno {
			testi = nil
			for _, x := range campi {
				testi = append(testi, string(c)+"."+x)
			}
		}
		for _, s := range testi {
			sel, err := LeggiSelettore(s)
			if err != nil {
				t.Errorf("%q: %v", s, err)
				continue
			}
			if sel.String() != s {
				t.Errorf("%q → %q", s, sel.String())
			}
			if err := sel.Valida(); err != nil {
				t.Errorf("%q: Valida: %v", s, err)
			}
			b, err := json.Marshal(sel)
			if err != nil || string(b) != `"`+s+`"` {
				t.Errorf("%q: JSON %s %v", s, b, err)
			}
			var diNuovo Selettore
			if err := json.Unmarshal(b, &diNuovo); err != nil || diNuovo != sel {
				t.Errorf("%q: JSON di ritorno %+v %v", s, diNuovo, err)
			}
			n++
		}
	}
	// 7 contesti senza campo, il cartiglio con 8 campi, l'elenco con 6, i due STEP con 4.
	if n != 7+8+6+4+4 {
		t.Fatalf("coppie provate: %d", n)
	}
	sel, _ := LeggiSelettore("storia")
	if sel != (Selettore{Contesto: ContestoStoria, Campo: CampoFonte{Variante: VarianteNessuno}}) {
		t.Fatalf("storia: %+v", sel)
	}
	sel, _ = LeggiSelettore("radice_step.id")
	if sel != (Selettore{Contesto: ContestoRadiceSTEP, Campo: CampoFonte{Variante: VarianteStep, Valore: "id"}}) {
		t.Fatalf("radice_step.id: %+v", sel)
	}
}

// TestA1aVOCSelettoriNonAmmessi: un contesto fuori elenco (anche «figlio_step», che solo il runner degli
// attesi traduce), una coppia fuori tabella, «nessuno» usato come jolly, spazi e maiuscole: errore di
// contratto con il codice e il percorso, nessun ripiego.
func TestA1aVOCSelettoriNonAmmessi(t *testing.T) {
	casi := []struct {
		testo, percorso string
	}{
		{"figlio_step.id", "contesto"},
		{"cartiglio.id", "campo"},
		{"nome_file.codice", "campo"},
		{"nessuno", "contesto"},
		{"corpo.nessuno", "campo"},
		{"cartiglio.nessuno", "campo"},
		{"cartiglio", "campo"},
		{"radice_step", "campo"},
		{"radice_step.codice", "campo"},
		{"elenco_pdf.numero_disegno", "campo"},
		{"storia.", "campo"},
		{"", "contesto"},
		{"Cartiglio.codice", "contesto"},
		{"cartiglio.Codice", "campo"},
		{" cartiglio.codice", "contesto"},
		{"cartiglio.codice ", "campo"},
		{"cartiglio.codice.x", "campo"},
		{"figlio_step.*", "contesto"},
		{"*.codice", "contesto"},
	}
	for _, c := range casi {
		sel, err := LeggiSelettore(c.testo)
		if sel != (Selettore{}) {
			t.Errorf("%q: restituito %+v insieme all'errore", c.testo, sel)
		}
		d := unicaDiagnostica(t, c.testo, err)
		if d == nil {
			continue
		}
		if d.Codice != CodiceSelettoreNonAmmesso || d.Gravita != GravitaErrore || d.Natura != NaturaContratto {
			t.Errorf("%q: %+v", c.testo, *d)
		}
		if d.Percorso != c.percorso {
			t.Errorf("%q: percorso %q, atteso %q", c.testo, d.Percorso, c.percorso)
		}
	}
}

// TestA1aVOCSelettoreGenerico: «cartiglio.*» non è una coppia: contratto.selettore_generico (D-12). Se in
// A1 il generico sia riservato lo dice la grammatica, non la foglia.
func TestA1aVOCSelettoreGenerico(t *testing.T) {
	for _, s := range []string{"cartiglio.*", "radice_step.*", "elenco_pdf.*", "nome_file.*"} {
		_, err := LeggiSelettore(s)
		d := unicaDiagnostica(t, s, err)
		if d == nil {
			continue
		}
		if d.Codice != CodiceSelettoreGenerico || d.Gravita != GravitaErrore || d.Natura != NaturaContratto || d.Percorso != "campo" {
			t.Errorf("%q: %+v", s, *d)
		}
	}
}

// TestA1aVOCValidaSenzaRipieghi: un Selettore costruito a mano con un campo STEP sul cartiglio, una variante
// sbagliata o un valore sul contesto senza campi non passa Valida (P1 §4.2: «errore, non fallback»).
func TestA1aVOCValidaSenzaRipieghi(t *testing.T) {
	casi := []Selettore{
		{Contesto: ContestoCartiglio, Campo: CampoFonte{Variante: VarianteStep, Valore: "id"}},
		{Contesto: ContestoCartiglio, Campo: CampoFonte{Variante: VarianteCartiglio, Valore: "id"}},
		{Contesto: ContestoNomeFile, Campo: CampoFonte{Variante: VarianteCartiglio, Valore: "codice"}},
		{Contesto: ContestoNomeFile, Campo: CampoFonte{Variante: VarianteNessuno, Valore: "codice"}},
		{Contesto: ContestoCorpo, Campo: CampoFonte{}},
		{Contesto: "figlio_step", Campo: CampoFonte{Variante: VarianteStep, Valore: "id"}},
		{},
	}
	for _, s := range casi {
		d := unicaDiagnostica(t, s.String(), s.Valida())
		if d != nil && d.Codice != CodiceSelettoreNonAmmesso {
			t.Errorf("%+v: %+v", s, *d)
		}
	}
	var s Selettore
	if err := json.Unmarshal([]byte(`"cartiglio.id"`), &s); err == nil {
		t.Error("il JSON accetta una coppia non ammessa")
	}
}

// TestA1aVOCIntervalloInJSON: l'intervallo si scrive con i nomi italiani dei campi.
func TestA1aVOCIntervalloInJSON(t *testing.T) {
	b, err := json.Marshal(Intervallo{Inizio: 3, Fine: 7})
	if err != nil || string(b) != `{"inizio":3,"fine":7}` {
		t.Fatalf("%s %v", b, err)
	}
}

// unicaDiagnostica: l'errore deve essere un *ErroreContratto con una diagnostica sola.
func unicaDiagnostica(t *testing.T, cosa string, err error) *Diagnostica {
	t.Helper()
	var ec *ErroreContratto
	if !errors.As(err, &ec) {
		t.Errorf("%q: atteso un *ErroreContratto, ottenuto %v", cosa, err)
		return nil
	}
	if len(ec.Diagnostiche) != 1 {
		t.Errorf("%q: attesa una diagnostica, ottenute %d", cosa, len(ec.Diagnostiche))
		return nil
	}
	if ec.Diagnostiche[0].Messaggio == "" {
		t.Errorf("%q: diagnostica senza messaggio", cosa)
	}
	return &ec.Diagnostiche[0]
}
