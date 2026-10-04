// L1 — i campi del cartiglio del worker e quelli della foglia (A1b-04; piano A, par.3.1, riga «evidenze»): ogni
// costante worker.Campo* (tipi.go:637-647) ha la sua voce fra i campi che la foglia ammette per il cartiglio.
// Le costanti si leggono anche dai sorgenti del contratto: una voce nuova del worker senza la sua voce nella
// foglia fa fallire la prova, prima che un adattatore la porti in cartiglio.sconosciuto senza dirlo.
//
// Nessun dato di cliente: si confrontano solo i nomi dei campi.
package estrazione

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// TestOgniCampoDelCartiglioDelWorkerHaLaSuaVoce (A1b-04).
func TestOgniCampoDelCartiglioDelWorkerHaLaSuaVoce(t *testing.T) {
	variante, ammessi := evidenze.CampiAmmessi(evidenze.ContestoCartiglio)
	if variante != evidenze.VarianteCartiglio {
		t.Fatalf("il cartiglio ha la variante %q", variante)
	}
	ammesso := map[string]bool{}
	for _, v := range ammessi {
		ammesso[v] = true
	}

	noti := []string{worker.CampoNumeroDisegno, worker.CampoCodice, worker.CampoRevisione, worker.CampoTitolo,
		worker.CampoScala, worker.CampoMateriale, worker.CampoParticolareSimile}
	for _, c := range noti {
		if !ammesso[c] {
			t.Errorf("worker.Campo %q non ha la sua voce fra i campi del cartiglio della foglia", c)
		}
	}

	// Tutte le costanti Campo* del contratto, lette dai sorgenti.
	dir := filepath.Join("..", "..", "platform", "contratti", "worker")
	file, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	trovate := 0
	fset := token.NewFileSet()
	for _, p := range file {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, nome := range vs.Names {
					if !strings.HasPrefix(nome.Name, "Campo") || i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Errorf("%s: %s non è una costante stringa scritta per intero", fset.Position(nome.Pos()), nome.Name)
						continue
					}
					v, _ := strconv.Unquote(lit.Value)
					trovate++
					if !ammesso[v] {
						t.Errorf("%s: %s = %q non ha la sua voce fra i campi del cartiglio della foglia", fset.Position(nome.Pos()), nome.Name, v)
					}
				}
			}
		}
	}
	if trovate < len(noti) {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("i sorgenti del contratto non si leggono: %v", err)
		}
		t.Fatalf("trovate %d costanti Campo*, almeno %d attese: la prova non legge i file giusti", trovate, len(noti))
	}
}
