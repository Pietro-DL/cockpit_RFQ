package filerfq

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// L1 statiche — PM23 T-08 (CO-12, CO-13) e PP-04 (EM §10.6, §10.7, §10.9):
//   - CO-12: nessuno scrive file_rfq, file_rfq_evento, file_rfq_rimozione, file_rfq_conservazione fuori dal servizio
//     del ciclo dei file (questo pacchetto): ne' una query nuova fuori dai due file di query del ciclo, ne' una chiamata
//     alle query che scrivono fuori da core/rfq/filerfq;
//   - CO-13: ogni funzione che scrive path_staging (SetAllegatoStaging, AzzeraPathStagingPerFile,
//     AzzeraPathStagingAllegato) prende il lucchetto per contenuto prima (BloccaContenuto, o AdottaContenuto che lo
//     prende), nella stessa funzione;
//   - PP-04: il corpo dei gesti della Sorgente non ha campi percorso.

// radiceInternal: la cartella internal/ del modulo, dal percorso di questo file.
func radiceInternal(t *testing.T) string {
	t.Helper()
	_, qui, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("percorso del file di prova sconosciuto")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(qui), "..", "..", ".."))
}

// fileGo: i file .go del modulo sotto internal/, senza le prove e senza il codice generato di platform/db.
func fileGo(t *testing.T) []string {
	t.Helper()
	var out []string
	radice := radiceInternal(t)
	err := filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(radice, p)
		if strings.HasPrefix(filepath.ToSlash(rel), "platform/db/") {
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var scrittoriFileRfq = []string{"InsertFileRfq", "UpdateFileRfqStato", "InsertFileRfqEvento", "InsertRimozione", "UpdateRimozione",
	"InsertConservazione", "UpdateConservazione"}

func TestCO12NessunoScriveIlCicloDeiFileFuoriDalServizio(t *testing.T) {
	radice := radiceInternal(t)
	for _, p := range fileGo(t) {
		rel, _ := filepath.Rel(radice, p)
		if strings.HasPrefix(filepath.ToSlash(rel), "core/rfq/filerfq/") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, nome := range scrittoriFileRfq {
			if regexp.MustCompile(`\.` + nome + `\(`).Match(b) {
				t.Errorf("%s chiama %s: lo stato dei file lo scrive solo core/rfq/filerfq", filepath.ToSlash(rel), nome)
			}
		}
	}
	// le query: solo i due file del ciclo dei file scrivono le tabelle file_rfq*
	cartella := filepath.Join(radice, "platform", "db", "queries")
	voci, err := os.ReadDir(cartella)
	if err != nil {
		t.Fatal(err)
	}
	// le tabelle del ciclo dei file sono file_rfq, file_rfq_evento, file_rfq_rimozione e file_rfq_conservazione; non
	// file_rfq_associazione (le AR della direttiva 35), che scrive solo core/rfq/associazioni (v1_associazioni.sql)
	scrive := regexp.MustCompile(`(?i)(INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+file_rfq(_evento|_rimozione|_conservazione)?\b`)
	for _, v := range voci {
		if v.Name() == "v1_formato3.sql" || v.Name() == "v1_file.sql" || !strings.HasSuffix(v.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(cartella, v.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if scrive.Match(b) {
			t.Errorf("la query %s scrive una tabella del ciclo dei file", v.Name())
		}
	}
}

var scrittoriPathStaging = map[string]bool{"SetAllegatoStaging": true, "AzzeraPathStagingPerFile": true, "AzzeraPathStagingAllegato": true}

func TestCO13OgniScrittoreDiPathStagingPrendeIlLucchetto(t *testing.T) {
	radice := radiceInternal(t)
	fset := token.NewFileSet()
	trovati := 0
	for _, p := range fileGo(t) {
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(radice, p)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			primoLucchetto, primoScrittore := token.NoPos, token.NoPos
			var nomeScrittore string
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var nome string
				switch x := call.Fun.(type) {
				case *ast.SelectorExpr:
					nome = x.Sel.Name
				case *ast.Ident:
					nome = x.Name
				}
				switch {
				case nome == "BloccaContenuto" || nome == "AdottaContenuto":
					if primoLucchetto == token.NoPos || call.Pos() < primoLucchetto {
						primoLucchetto = call.Pos()
					}
				case scrittoriPathStaging[nome]:
					if primoScrittore == token.NoPos || call.Pos() < primoScrittore {
						primoScrittore, nomeScrittore = call.Pos(), nome
					}
				}
				return true
			})
			if primoScrittore == token.NoPos {
				continue
			}
			trovati++
			if primoLucchetto == token.NoPos || primoLucchetto > primoScrittore {
				t.Errorf("%s:%s scrive path_staging (%s) senza prendere prima BloccaContenuto",
					filepath.ToSlash(rel), fn.Name.Name, nomeScrittore)
			}
		}
	}
	if trovati < 6 {
		t.Errorf("trovati solo %d scrittori di path_staging: la prova non guarda dove deve", trovati)
	}
}

// TestPP04NessunCampoPercorsoNelCorpo: il corpo dei gesti non ha campi che portino un percorso (D23:234).
func TestPP04NessunCampoPercorsoNelCorpo(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(radiceInternal(t), "core", "rfq", "filerfq", "corpo.go"))
	if err != nil {
		t.Fatal(err)
	}
	inizio := strings.Index(string(b), "type CorpoGesto struct {")
	fine := strings.Index(string(b)[inizio:], "\n}")
	blocco := strings.ToLower(string(b)[inizio : inizio+fine])
	for _, vietato := range []string{"percorso", "path", "radice", "file\"", "nas"} {
		if strings.Contains(blocco, "json:\""+vietato) {
			t.Errorf("il corpo dei gesti ha un campo %q", vietato)
		}
	}
}
