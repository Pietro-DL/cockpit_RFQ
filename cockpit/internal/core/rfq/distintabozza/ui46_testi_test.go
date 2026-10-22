package distintabozza

// L1 — giro UI46, messaggio 48 [U]: i testi del riepilogo che la Distinta unica mostra nel wizard «Rivedi e conferma BOM»
// hanno gli accenti veri, non gli apostrofi («finche' il conflitto non e' risolto» → «finché il conflitto non è
// risolto»). Prova gli effetti e i testi delle preassociazioni (effettoPB, testiPB, testoPB, rimediPB): sono le righe che
// lo screenshot dell'utente mostrava.

import (
	"regexp"
	"testing"

	"promatec/cockpit/internal/core/rfq/associazioni"
)

// apostrofoPerAccento: una parola che finisce con vocale e apostrofo davanti a uno spazio, a una punteggiatura o alla fine
// («e'», «gia'», «piu'», «finche'»); l'elisione («dell'arco», «un'altra») non conta.
var apostrofoPerAccento = regexp.MustCompile(`(^|[^A-Za-z'])[A-Za-z]*[aeiouAEIOU]'([^A-Za-z]|$)`)

func TestUI46TestiPreassociazioniConGliAccenti(t *testing.T) {
	esiti := []string{associazioni.EsitoApplicabile, associazioni.EsitoGiaApplicata, associazioni.EsitoSostituzione, associazioni.EsitoNonRichiesta,
		associazioni.EsitoInAttesaCodice, associazioni.EsitoInConflitto}
	for _, es := range esiti {
		if s := effettoPB(associazioni.EsitoPB{Esito: es}); apostrofoPerAccento.MatchString(s) {
			t.Errorf("effetto della PB %s con l'apostrofo al posto dell'accento: %q", es, s)
		}
	}
	if s := effettoPB(associazioni.EsitoPB{Esito: associazioni.EsitoInConflitto}); s != "non si applica finché il conflitto non è risolto" {
		t.Errorf("effetto della PB in conflitto: %q", s)
	}
	for motivo, s := range testiPB {
		if apostrofoPerAccento.MatchString(s) {
			t.Errorf("testo del motivo %s con l'apostrofo al posto dell'accento: %q", motivo, s)
		}
		e := associazioni.EsitoPB{Esito: associazioni.EsitoInConflitto, Motivo: motivo}
		if s := testoPB(e, "ACME_9120001A.pdf"); apostrofoPerAccento.MatchString(s) {
			t.Errorf("testo della PB (%s) con l'apostrofo al posto dell'accento: %q", motivo, s)
		}
		for _, r := range rimediPB(e) {
			if apostrofoPerAccento.MatchString(r) {
				t.Errorf("rimedio della PB (%s) con l'apostrofo al posto dell'accento: %q", motivo, r)
			}
		}
	}
	// la regola non scambia l'elisione per un accento mancante
	for _, s := range []string{"dell'arco", "un'altra identità", "l'associazione"} {
		if apostrofoPerAccento.MatchString(s) {
			t.Errorf("la regola della prova scambia l'elisione per un accento: %q", s)
		}
	}
}
