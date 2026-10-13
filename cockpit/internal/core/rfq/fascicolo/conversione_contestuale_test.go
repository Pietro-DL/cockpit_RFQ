package fascicolo

// L1 — A1d P4, D-K6-01 = A [U] con la precisazione della direttiva 12 §6: la risposta del gesto che cambierebbe un
// particolare in assieme (RichiestaConversione). La domanda nomina il componente e il cambiamento; per chi non sa
// chiederla e' un Rifiuto con la strada che c'e' gia' (la scheda); DecisioneTipo vale solo per i componenti nominati.
// Il percorso con il database (la domanda non scrive, la conferma e' atomica, un ciclo vince) e' nelle L4 dello
// Scenario G (conversioni_tacite_db_test.go). Codici della famiglia di prova ACME 712.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func TestRichiestaConversioneDomandaERifiuto(t *testing.T) {
	b := uuid.MustParse("a1000000-0000-4000-8000-000000000012")
	rc := RichiestaConversione{Componente: b, Codice: "7120012", Figlio: "7120021"}
	if want := "Il componente 7120012 è attualmente un particolare. Aggiungendo 7120021 come figlio, 7120012 diventerà un " +
		"sottoassieme. Vuoi confermare questa modifica?"; rc.Domanda() != want {
		t.Errorf("domanda:\n  %s\natteso:\n  %s", rc.Domanda(), want)
	}
	const rifiuto = "7120012 → 7120021: 7120012 è un particolare, e il tipo non cambia da solo: se contiene dei pezzi, " +
		"cambialo prima in assieme dalla sua scheda"
	if rc.Error() != rifiuto {
		t.Errorf("rifiuto: %q", rc.Error())
	}
	// avvolta da chi chiama, resta riconoscibile in tutti e due i modi
	err := fmt.Errorf("la struttura di 7120012.stp: %w", rc)
	var r Rifiuto
	if !errors.As(err, &r) || string(r) != rifiuto {
		t.Errorf("per chi non sa chiedere la conferma e' un Rifiuto: %v", err)
	}
	var di RichiestaConversione
	if !errors.As(err, &di) || di != rc {
		t.Errorf("chi sa chiedere la conferma ritrova il componente: %+v", di)
	}
	if errors.As(Rifiuto("un altro no"), &di) {
		t.Error("un Rifiuto qualunque non e' una richiesta di conversione")
	}
}

func TestDecisioneTipoNominaIComponenti(t *testing.T) {
	b, c := uuid.New(), uuid.New()
	if (DecisioneTipo{}).dichiaraAssieme(b) {
		t.Error("senza dichiarazione nessun componente e' dichiarato")
	}
	d := DecisioneTipo{Assiemi: []uuid.UUID{b}}
	if !d.dichiaraAssieme(b) || d.dichiaraAssieme(c) {
		t.Error("la dichiarazione vale per il componente nominato, e solo per lui")
	}
}
