package motorea

import (
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — marcatori e categorie (A1a-MRK; piano A, par.4.4.5, 4.7.2; D1, R7, E-02): la A e la X stanno fra la
// base e la revisione, fanno parte del testo e non della base; la categoria minuteria viene dalla famiglia
// dichiarata, non da un riconoscitore sul nome: lo stesso codice con la X in una famiglia senza categorie non
// ne ha. Che il motore non chiami il riconoscitore legacy della minuteria lo controlla la guardia sugli
// import e sulle chiamate (evidenze/dipendenze_test.go, G1).
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono acme-marcatore e acme-marcatore-x di sintetiche_test.go.

func TestMarcatoreAFraBaseERevisione(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
	casi := []struct {
		sel, testo, forma, rev string
	}{
		{"nome_file", "9123456A_2.pdf", "nome-sottolineato", "2"},
		{"nome_file", "9123456A2.pdf", "nome", "2"},
		{"cartiglio.codice", "9123456A2", "cartiglio", "2"},
		{"radice_step.id", "9123456A", "step", ""},
		{"nodo_step.id", "9123456A", "step", ""},
		{"radice_step.id", "9123456A_PRT", "step-prt", ""},
	}
	for _, c := range casi {
		letture, _ := riconosci(t, m, c.sel, c.testo)
		l := unaLettura(t, letture, "acme-marcatore", c.forma)
		if l.Marcatore == nil || l.Marcatore.Valore != "A" || l.Marcatore.Intervallo.Inizio != 7 || l.Marcatore.Intervallo.Fine != 8 {
			t.Errorf("%q: marcatore %+v, atteso «A» in [7,8)", c.testo, l.Marcatore)
		}
		if l.Base.Normalizzata != "9123456" || l.CodiceRichiesto != "9123456" {
			t.Errorf("%q: base %q, codice richiesto %q: il marcatore non è nella base", c.testo, l.Base.Normalizzata, l.CodiceRichiesto)
		}
		switch {
		case c.rev == "" && l.Revisione != nil:
			t.Errorf("%q: revisione %+v, attesa nessuna", c.testo, l.Revisione)
		case c.rev != "" && (l.Revisione == nil || l.Revisione.Normalizzata != c.rev || l.Revisione.Stato != StatoRevisioneLetta):
			t.Errorf("%q: revisione %+v, attesa %q", c.testo, l.Revisione, c.rev)
		}
		if len(l.Categorie) != 0 {
			t.Errorf("%q: categorie %v, la famiglia non ne dichiara", c.testo, l.Categorie)
		}
	}
}

// TestMarcatoreMinuscoloEToken: nel nome DXF il marcatore minuscolo si conserva com'è e «_drw_<n>» è un token
// conservato, senza revisione (D1, D10).
func TestMarcatoreMinuscoloEToken(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
	letture, _ := riconosci(t, m, "nome_file", "dxf_9123456a_drw_1.dxf")
	l := unaLettura(t, letture, "acme-marcatore", "dxf")
	if l.Marcatore == nil || l.Marcatore.Valore != "a" {
		t.Errorf("marcatore %+v, atteso «a»", l.Marcatore)
	}
	if l.Token == nil || l.Token.Valore != "1" || l.Revisione != nil {
		t.Errorf("token %+v, revisione %+v: atteso il token «1» e nessuna revisione", l.Token, l.Revisione)
	}
	if l.Base.Normalizzata != "9123456" || l.CodiceRichiesto != "9123456" {
		t.Errorf("base %q, codice richiesto %q", l.Base.Normalizzata, l.CodiceRichiesto)
	}
}

// TestMarcatoreXConCategoriaDallaFamiglia: la X come la A (R7), con la revisione facoltativa sul nodo, e la
// categoria minuteria copiata dalla famiglia.
func TestMarcatoreXConCategoriaDallaFamiglia(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
	casi := []struct {
		sel, testo, forma, base, rev string
	}{
		{"nodo_step.id", "912345X1", "x-nodo", "912345", "1"},
		{"nodo_step.id", "9123456X", "x-nodo", "9123456", ""},
		{"nome_file", "912345X_1.stp", "x-nome", "912345", "1"},
		{"radice_step.id", "912345X1_PRT", "x-radice", "912345", "1"},
		{"radice_step.id", "9123456X_PRT", "x-radice", "9123456", ""},
	}
	for _, c := range casi {
		letture, _ := riconosci(t, m, c.sel, c.testo)
		l := unaLettura(t, letture, "acme-marcatore-x", c.forma)
		if l.Marcatore == nil || l.Marcatore.Valore != "X" {
			t.Errorf("%q: marcatore %+v", c.testo, l.Marcatore)
		}
		if l.Base.Normalizzata != c.base {
			t.Errorf("%q: base %q, attesa %q", c.testo, l.Base.Normalizzata, c.base)
		}
		if (c.rev == "") != (l.Revisione == nil) || (l.Revisione != nil && l.Revisione.Normalizzata != c.rev) {
			t.Errorf("%q: revisione %+v, attesa %q", c.testo, l.Revisione, c.rev)
		}
		if len(l.Categorie) != 1 || l.Categorie[0] != grammatica.CategoriaMinuteria {
			t.Errorf("%q: categorie %v, attesa minuteria dalla famiglia", c.testo, l.Categorie)
		}
		if l.Namespace != "acme-marcatore" {
			t.Errorf("%q: namespace %q: la X sta nello spazio della A", c.testo, l.Namespace)
		}
	}
}

// TestMarcatoreXSenzaCategorieNellaFamiglia: lo stesso codice con la X, in una famiglia che non dichiara
// categorie, non ne ha: nessun riconoscitore sul nome decide la minuteria (R7).
func TestMarcatoreXSenzaCategorieNellaFamiglia(t *testing.T) {
	f := famMarcatoreX()
	f.Categorie = nil
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), f))
	letture, _ := riconosci(t, m, "nodo_step.id", "912345X1")
	l := unaLettura(t, letture, "acme-marcatore-x", "x-nodo")
	if len(l.Categorie) != 0 {
		t.Errorf("categorie %v, attese nessuna", l.Categorie)
	}
}

// TestMarcatoreLeCategorieNonSonoCondivise: cambiare le categorie di una lettura non cambia quelle delle
// letture successive: il motore è immutabile.
func TestMarcatoreLeCategorieNonSonoCondivise(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
	letture, _ := riconosci(t, m, "nodo_step.id", "912345X1")
	letture[0].Categorie[0] = "alterata"
	letture, _ = riconosci(t, m, "nodo_step.id", "912345X1")
	if letture[0].Categorie[0] != grammatica.CategoriaMinuteria {
		t.Errorf("categorie %v dopo aver alterato una lettura precedente", letture[0].Categorie)
	}
}
