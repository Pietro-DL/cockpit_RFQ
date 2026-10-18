package db

// L1 — T-03 del piano della 0023 (BL V1-Q04 A; EM §5.4): nessuna query delle rotte R2-R5 del formato 3 contiene ON
// CONFLICT. Un doppione nella working o in un registro e' un errore da spiegare (409 BASE_SUPERATA, codice_occupato), mai
// una fusione silenziosa. Le query di R2-R5 stanno in p4_conferme.sql e v1_formato3.sql; quando R2-R5 chiameranno query
// di altri file, la prova si estende ai loro nomi. L'ON CONFLICT di working_contatore nel trigger della 0022 non e' una
// query del Go e resta.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reOnConflict = regexp.MustCompile(`(?is)\bON\s+CONFLICT\b`)

func TestV1NessunaFusioneNelleQueryDiR2R5(t *testing.T) {
	for _, nome := range []string{"p4_conferme.sql", "v1_formato3.sql"} {
		b, err := os.ReadFile(filepath.Join("queries", nome))
		if err != nil {
			t.Fatal(err)
		}
		// fuori dai commenti: un commento puo' citare la regola
		var righe []string
		for _, r := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			if i := strings.Index(r, "--"); i >= 0 {
				r = r[:i]
			}
			righe = append(righe, r)
		}
		if reOnConflict.MatchString(strings.Join(righe, "\n")) {
			t.Errorf("%s contiene ON CONFLICT: le query di R2-R5 non fondono", nome)
		}
	}
	// la controprova: la regola vede la forma che InsertComponente usa
	if !reOnConflict.MatchString("INSERT INTO componente (codice) VALUES ($1)\nON  CONFLICT (thread_id, upper(codice)) DO UPDATE") {
		t.Error("la regola non riconosce ON CONFLICT")
	}
}
