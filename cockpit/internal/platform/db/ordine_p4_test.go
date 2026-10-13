package db

// L1 — una regola d'ordine dei blocchi di P4 (giro 5, A1d; revisione «guardie e scrittori» della notte 12→13/10,
// RSC-04). Con la 0022 v_fascicolo, il gate del congelamento e il cruscotto contano come presente un disegno_2d
// coperto da una copertura valida; il Motore A (contratto A1c congelato) ancora no: la provenienza «copertura» arriva
// con B3. Finche' B3 non c'e', nessuno scrive copertura_documento (ne' una query sqlc ne' SQL nel Go), cosi' la
// divergenza resta latente e non diventa una BOM congelata che il Motore A direbbe incompleta. Quando B3 porta la
// provenienza nel Motore A, questa prova si toglie insieme al primo scrittore (B4, lotto R9).

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var reScriveCoperture = regexp.MustCompile(`(?is)\b(insert\s+into|update|delete\s+from|merge\s+into)\s+copertura_documento\b`)

func TestNessunoScriveLeCopertureFinoAB3(t *testing.T) {
	radice := filepath.Join("..", "..") // internal/
	var trovati []string
	err := filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		nome := d.Name()
		if strings.HasSuffix(nome, "_test.go") || !(strings.HasSuffix(nome, ".go") || strings.HasSuffix(nome, ".sql")) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if reScriveCoperture.Match(b) {
			trovati = append(trovati, filepath.ToSlash(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(trovati) > 0 {
		t.Errorf("copertura_documento ha uno scrittore prima di B3 (provenienza «copertura» nel Motore A): %s", strings.Join(trovati, ", "))
	}
}
