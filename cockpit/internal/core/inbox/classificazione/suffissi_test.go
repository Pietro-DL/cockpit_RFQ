package classificazione

import (
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

// L1 — Fascicolo v3: il motore toglie i suffissi decorativi del cliente dal codice letto («X_PRT» → X). Solo
// per chi li dichiara, solo in coda, una volta, il piu' lungo prima, e solo se quello che resta ha ancora la
// forma di un codice. Lo schema che li accetta sta in core/registro/regole (suffissi_test.go). Le prove L4
// sull'ingest, sullo staging, sull'analisi e sugli STEP stanno con quei pacchetti.

func motoreConSuffissi(t *testing.T, suffissi ...string) *Motore {
	t.Helper()
	return Compila("ACME", regole.Regole{SuffissiDecorativi: suffissi})
}

func TestCanonicoTogliIlSuffissoDecorativo(t *testing.T) {
	m := motoreConSuffissi(t, "_PRT", "_ASM", "_PRT_ASM")
	casi := []struct{ codice, rev, atteso, attesaRev string }{
		{"52920000_PRT", "", "52920000", ""},
		{"52920000_prt", "", "52920000", ""},       // senza distinguere maiuscole
		{" 52920000_PRT ", "", "52920000", ""},     // gli spazi intorno non contano
		{"52920000_PRT_ASM", "", "52920000", ""},   // il piu' lungo prima, non solo «_ASM»
		{"52920000_B_PRT", "", "52920000", "B"},    // la revisione in coda si rilegge
		{"52920000_B_PRT", "C", "52920000_B", "C"}, // con la revisione gia' nota non si rilegge
		{"52920000", "", "52920000", ""},           // niente suffisso: com'e'
		{"52920000_PRTX", "", "52920000_PRTX", ""}, // non e' in coda
		{"1234_PRT", "", "1234_PRT", ""},           // quello che resta non ha la forma di un codice
		{"_PRT", "", "_PRT", ""},                   // non resta niente
		{"PRT_PRT", "", "PRT_PRT", ""},             // «PRT» non e' un codice
	}
	for _, c := range casi {
		k, r := m.Canonico(c.codice, c.rev)
		if k != c.atteso || r != c.attesaRev {
			t.Errorf("Canonico(%q, %q) = %q, %q; atteso %q, %q", c.codice, c.rev, k, r, c.atteso, c.attesaRev)
		}
	}
	// nel nome di un file il suffisso puo' stare prima della revisione
	for in, atteso := range map[string]string{"52920000_PRT_B": "52920000", "52920000_PRT": "52920000", "52920000_B": "52920000_B", "Offerta": "Offerta"} {
		if got := m.CanonicoNome(in); got != atteso {
			t.Errorf("CanonicoNome(%q) = %q, atteso %q", in, got, atteso)
		}
	}
	var nessuno *Motore
	if got := nessuno.CanonicoNome("52920000_PRT_B"); got != "52920000_PRT_B" {
		t.Errorf("senza motore: %q", got)
	}
	if !m.HaSuffissi() || len(m.SuffissiDecorativi()) != 3 || m.SuffissiDecorativi()[0] != "_PRT_ASM" {
		t.Errorf("i suffissi del motore, il piu' lungo prima: %q", m.SuffissiDecorativi())
	}
}

// Chi non dichiara suffissi, o non ha un motore, non vede cambiare niente: per un altro cliente «_PRT» puo'
// essere una parte vera del codice.
func TestSenzaSuffissiIlCodiceResta(t *testing.T) {
	var nessuno *Motore
	for _, m := range []*Motore{nessuno, Compila("ACME", regole.Regole{}), Compila("", regole.Regole{})} {
		if k, r := m.Canonico("52920000_PRT", ""); k != "52920000_PRT" || r != "" {
			t.Errorf("senza suffissi: %q %q", k, r)
		}
		if m.HaSuffissi() || m.SuffissiDecorativi() != nil {
			t.Errorf("senza suffissi il motore ne ha: %v", m.SuffissiDecorativi())
		}
	}
}

// Un suffisso che lo schema rifiuta (una coda da revisione, arrivata da un file o da psql) non si usa: il
// motore si comporta come se non ci fosse.
func TestUnSuffissoRottoNonSiUsa(t *testing.T) {
	m := motoreConSuffissi(t, "_R2", "PRT", "_PRT")
	if got := m.SuffissiDecorativi(); len(got) != 1 || got[0] != "_PRT" {
		t.Fatalf("entrano solo i suffissi buoni: %q", got)
	}
	if k, r := m.Canonico("52920000_R2", ""); k != "52920000_R2" || r != "" {
		t.Errorf("una coda da revisione non si toglie come suffisso: %q %q", k, r)
	}
}

// Nel testo: la via generica toglie il suffisso e rilegge la revisione; la via della famiglia toglie solo il
// suffisso (la famiglia dice gia' dov'e' il codice e dov'e' la revisione).
func TestCodiciDaConISuffissi(t *testing.T) {
	m := motoreConSuffissi(t, "_PRT")
	trovati := m.Codici("allegato 52920000_PRT.stp e 52930000_C_PRT.stp")
	visti := map[string]string{}
	for _, c := range trovati {
		visti[c.Codice] = c.Rev
	}
	if rev, ok := visti["52920000"]; !ok || rev != "" {
		t.Errorf("52920000_PRT nel testo: %+v", trovati)
	}
	if rev, ok := visti["52930000"]; !ok || rev != "C" {
		t.Errorf("52930000_C_PRT nel testo: %+v", trovati)
	}
	for k := range visti {
		if k == "52920000_PRT" || k == "52930000_C_PRT" {
			t.Errorf("il codice con il suffisso e' uscito lo stesso: %+v", trovati)
		}
	}

	// una famiglia che prende anche la coda: si toglie solo il suffisso
	r, err := regole.ValidaRegole([]byte(`{"famiglie_codice": [{"regex": "\\b(?P<codice>5[23]\\d{6}(?:_[A-Z]{3})?)", "descrizione": "disegni 52/53",
		"esempio": "52920000_PRT"}], "suffissi_decorativi": ["_PRT"]}`))
	if err != nil {
		t.Fatal(err)
	}
	f := Compila("ACME", r)
	var famiglia []CodiceTrovato
	for _, c := range f.Codici("vedi 52920000_PRT e 52920001_ASM") {
		if c.Origine == "famiglia" {
			famiglia = append(famiglia, c)
		}
	}
	if len(famiglia) != 2 || famiglia[0].Codice != "52920000" || famiglia[1].Codice != "52920001_ASM" {
		t.Errorf("i codici della famiglia: %+v", famiglia)
	}

	// una famiglia con la revisione in coda: il suffisso non e' la revisione («_P» di «_PRT»)
	r, err = regole.ValidaRegole([]byte(`{"famiglie_codice": [{"regex": "(?P<codice>529\\d{5})(?:_(?P<rev>[A-Z]))?", "rev_nel_codice": true,
		"descrizione": "disegni 529", "esempio": "52922757_B"}], "suffissi_decorativi": ["_PRT"]}`))
	if err != nil {
		t.Fatal(err)
	}
	f = Compila("ACME", r)
	famiglia = nil
	for _, c := range f.Codici("vedi 52920000_PRT.stp, 52930000_C_PRT.pdf e 52940000_prt") {
		if c.Origine == "famiglia" {
			famiglia = append(famiglia, c)
		}
	}
	if len(famiglia) != 3 || famiglia[0].Codice+"/"+famiglia[0].Rev != "52920000/" || famiglia[1].Codice+"/"+famiglia[1].Rev != "52930000/C" ||
		famiglia[2].Codice+"/"+famiglia[2].Rev != "52940000/" {
		t.Errorf("la famiglia con la revisione e il suffisso: %+v", famiglia)
	}
	// il suffisso si toglie solo in coda a una parola: dentro una parola resta
	if got := f.senzaSuffissi("52920000_PRTX 52920000_PRT, _PRT e X_prt"); got != "52920000_PRTX 52920000, _PRT e X" {
		t.Errorf("senzaSuffissi: %q", got)
	}

	// senza motore il testo si legge come prima
	var nessuno *Motore
	for _, c := range nessuno.Codici("allegato 52920000_PRT.stp") {
		if c.Codice == "52920000" && c.Rev == "" {
			t.Errorf("senza motore il suffisso non si toglie: %+v", c)
		}
	}
}
