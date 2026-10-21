package confermabom

import (
	"testing"

	"github.com/google/uuid"
)

// L1: il legame storico univoco di D-AR04 (iii) A [U] con la lettura di R39-06 (commento di legame_storico.go).
func TestLegameStoricoUnivoco(t *testing.T) {
	a := DocumentoDelProdotto{DocumentoID: uuid.New(), Sha256: "a", NomeFile: "ACM-6000.stp", Estensione: "stp"}
	b := DocumentoDelProdotto{DocumentoID: uuid.New(), Sha256: "b", NomeFile: "ACM-6000_bis.STEP", Estensione: ""}
	vecchio := DocumentoDelProdotto{DocumentoID: uuid.New(), Sha256: "c", NomeFile: "ACM-6000_old.stp", Estensione: "stp", Sostituito: true}
	igs := DocumentoDelProdotto{DocumentoID: uuid.New(), Sha256: "d", NomeFile: "ACM-6000.igs", Estensione: "igs"}
	casi := []struct {
		nome   string
		docs   []DocumentoDelProdotto
		conS8  bool
		atteso *uuid.UUID
	}{
		{"nessuno", nil, false, nil},
		{"uno solo", []DocumentoDelProdotto{a}, false, &a.DocumentoID},
		{"uno solo dal nome del file", []DocumentoDelProdotto{b}, false, &b.DocumentoID},
		{"uno corrente e uno sostituito", []DocumentoDelProdotto{vecchio, a}, false, &a.DocumentoID},
		{"uno STEP e un altro formato", []DocumentoDelProdotto{igs, a}, false, &a.DocumentoID},
		{"due correnti: non univoco", []DocumentoDelProdotto{a, b}, false, nil},
		{"con una conferma di S8", []DocumentoDelProdotto{a}, true, nil},
		{"solo il sostituito", []DocumentoDelProdotto{vecchio}, false, nil},
	}
	for _, c := range casi {
		d, ok := LegameStoricoUnivoco(c.docs, c.conS8)
		if (c.atteso == nil) == ok || (ok && d.DocumentoID != *c.atteso) {
			t.Errorf("%s: avuto %v %+v", c.nome, ok, d)
		}
	}
	for _, x := range []struct {
		est, nome string
		step      bool
	}{{"stp", "", true}, {" .STEP ", "", true}, {"", "a.Stp ", true}, {"igs", "a.igs", false}, {"", "stp", false}} {
		if EstensioneOFileStep(x.est, x.nome) != x.step {
			t.Errorf("EstensioneOFileStep(%q, %q) != %v", x.est, x.nome, x.step)
		}
	}
}
