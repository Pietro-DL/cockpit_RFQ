package classificazione

import "testing"

// L1 — Smistamento, giro 4, fase 4.17a (il censimento delle forme): la forma di una stringa. Cifra → 9,
// lettera → A, i separatori restano dove sono; gli spazi in testa e in coda spariscono e quelli in mezzo
// diventano uno solo. FormaCifre tiene le lettere, in maiuscolo: la X della minuteria accanto alla A dei
// pezzi, con le stesse cifre intorno, e Forma le farebbe uguali.
func TestLaFormaDiUnaStringa(t *testing.T) {
	casi := []struct{ in, forma, cifre string }{
		{"7120001A_1", "9999999A_9", "9999999A_9"},
		{"7120001A_1.stp", "9999999A_9.AAA", "9999999A_9.STP"},
		{"7120010x1", "9999999A9", "9999999X9"},
		{"0.712.0011.3/10", "9.999.9999.9/99", "9.999.9999.9/99"},
		{"  RDO   7120012\t del 12/09 ", "AAA 9999999 AAA 99/99", "RDO 9999999 DEL 99/99"},
		{"PC-PROVA", "AA-AAAAA", "PC-PROVA"},
		{"7120001A_1 (2)", "9999999A_9 (9)", "9999999A_9 (9)"},
		{"Ø12 – vite", "A99 – AAAA", "Ø99 – VITE"},
		{"", "", ""},
		{"   ", "", ""},
	}
	for _, c := range casi {
		if got := Forma(c.in); got != c.forma {
			t.Errorf("Forma(%q) = %q, attesa %q", c.in, got, c.forma)
		}
		if got := FormaCifre(c.in); got != c.cifre {
			t.Errorf("FormaCifre(%q) = %q, attesa %q", c.in, got, c.cifre)
		}
	}
	// la stessa forma per due codici della stessa famiglia, e due forme per la X e la A solo con le lettere
	if Forma("7120001A_1") != Forma("7120011A_2") {
		t.Error("due codici della stessa famiglia devono avere la stessa forma")
	}
	if FormaCifre("7120010X1") == FormaCifre("7120010A1") || Forma("7120010X1") != Forma("7120010A1") {
		t.Error("la X e la A: uguali per Forma, diverse per FormaCifre")
	}
}
