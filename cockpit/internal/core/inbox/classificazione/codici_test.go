package classificazione

import (
	"reflect"
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

// L1 — le minuterie dei codici (revisione del 25/09).

// CodiceRev rilegge i NOSTRI nomi sul NAS (documenti.NomeTecnico, D21/D22): un file che torna indietro
// dal NAS — un documento ricaricato, un allegato di una nostra mail — deve dare il codice e la
// revisione che l'hanno fatto nascere, non «X_REV» con revisione «B».
func TestCodiceRevRileggeINostriNomiSulNas(t *testing.T) {
	casi := []struct{ nome, codice, rev string }{
		{"77720517_REV_B", "77720517", "B"},
		{"77720517_REV_B_2", "77720517", "B"},
		{"77720517_REV_ND", "77720517", ""},
		{"77720517_REV_ND_3", "77720517", ""},
		{"PZ-001_REV_04", "PZ-001", "04"},
		{"pz-001_rev_c", "PZ-001", "C"},
		// le convenzioni di sempre non cambiano
		{"1234567A_4", "1234567A", "4"},
		{"1234567A-B", "1234567A", "B"},
		{"1234567A_REV4", "1234567A", "4"},
		{"1234567A_R3", "1234567A", "3"},
		{"1234567A", "1234567A", ""},
	}
	for _, c := range casi {
		if k, r := CodiceRev(c.nome); k != c.codice || r != c.rev {
			t.Errorf("CodiceRev(%q) = (%q, %q), atteso (%q, %q)", c.nome, k, r, c.codice, c.rev)
		}
	}
	// e il giro completo: il nome del file diventa la proposta giusta
	if p := PropostaDaNome("77720517_REV_B_2.pdf", 100_000, "entrata"); p.Codice != "77720517" || p.Rev != "B" {
		t.Errorf("proposta dal nome sul NAS: codice %q rev %q", p.Codice, p.Rev)
	}
}

// Un codice non contiene spazi, di nessun tipo. Lo spazio unificatore, quello stretto e quello a
// larghezza zero arrivano dai cartigli, dai nomi dei file e dal corpo HTML, e a occhio non si vedono:
// «77720517», spazio unificatore (U+00A0), «B» passava come un codice solo.
func TestUnCodiceConUnoSpazioUnicodeNonEAmmissibile(t *testing.T) {
	// i caratteri si scrivono per numero: nel sorgente, scritti per esteso, non si vedrebbero
	spazi := []rune{0x00A0, 0x202F, 0x2007, 0x200B, 0x2028, 0x0085, 0xFEFF}
	var codici []string
	for _, r := range spazi {
		codici = append(codici, "77720517"+string(r)+"B")
	}
	for _, s := range codici {
		if CodiceAmmissibile(s) {
			t.Errorf("CodiceAmmissibile(%q) = true", s)
		}
		if sembraCodice(s) {
			t.Errorf("sembraCodice(%q) = true", s)
		}
	}
	if RevAmmissibile("B" + string(rune(0x00A0)) + "1") {
		t.Error("RevAmmissibile con lo spazio unificatore = true")
	}
	// quelli buoni restano buoni, anche con una lettera accentata
	for _, s := range []string{"77720517", "PZ-001_B", "ÀBC12345"} {
		if !CodiceAmmissibile(s) {
			t.Errorf("CodiceAmmissibile(%q) = false", s)
		}
	}
}

// «richiesta d’offerta» con l'apostrofo tipografico, che Outlook e Word mettono da soli, è una
// richiesta d'offerta: gli stessi trentacinque punti dell'apostrofo dritto.
func TestLApostrofoTipograficoELaStessaRichiestaDOfferta(t *testing.T) {
	dritto := Triage(IngressoTriage{Direzione: "entrata", Oggetto: "Richiesta d'offerta", Corpo: "in allegato"})
	curvo := Triage(IngressoTriage{Direzione: "entrata", Oggetto: "Richiesta d’offerta", Corpo: "in allegato"})
	if !contieneMotivo(curvo.Motivi, "richiesta d’offerta") {
		t.Errorf("l'apostrofo tipografico non è stato riconosciuto: %v", curvo.Motivi)
	}
	if curvo.Confidenza != dritto.Confidenza {
		t.Errorf("confidenza %d con l'apostrofo tipografico, %d con quello dritto", curvo.Confidenza, dritto.Confidenza)
	}
}

// spezzaFrasi con l'espressione compilata una volta sola divide come prima: sul punto, sul punto e
// virgola, sul punto esclamativo e interrogativo seguiti da spazio, e a ogni riga.
func TestSpezzaFrasiDivideComePrima(t *testing.T) {
	got := spezzaFrasi("Buongiorno. Vi abbiamo caricato sul portale i CAD; grazie!\r\nCordiali saluti? Sì\n\n1.5 mm")
	atteso := []string{"Buongiorno", "Vi abbiamo caricato sul portale i CAD", "grazie!", "Cordiali saluti", "Sì", "1.5 mm"}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("spezzaFrasi = %q, atteso %q", got, atteso)
	}
}

// Una finestra di aggancio fuori da 1..3650 vale come non dichiarata (0 = il default dell'aggancio),
// come ogni altra regola ✗: le regole lette dal database non sono passate per forza dal convalidatore.
func TestUnaFinestraImpossibileValeComeNonDichiarata(t *testing.T) {
	for g, atteso := range map[int]int{45: 45, 1: 1, 3650: 3650, 0: 0, -5: 0, 3651: 0, 90000: 0} {
		if got := Compila("ACME", regole.Regole{FinestraAggancioGG: g}).Finestra(); got != atteso {
			t.Errorf("finestra_aggancio_gg = %d: Finestra() = %d, atteso %d", g, got, atteso)
		}
	}
	var nessuno *Motore
	if nessuno.Finestra() != 0 {
		t.Error("un motore nil non ha finestra")
	}
}
