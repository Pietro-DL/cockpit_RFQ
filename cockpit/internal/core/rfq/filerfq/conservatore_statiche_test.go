package filerfq

// L1 statiche — V1, ondata C, corsia file (U2 §3.2, §3.13 punti 3-5):
//   - il doppio delle prove (conservatoreprova, il ConservatoreLocale) non lo importa nessun file di produzione, e
//     nessun file di produzione fuori da core/rfq/filerfq imposta il Conservatore di un filerfq.Servizio, salvo il
//     cablaggio di app/runtime/ciclo_file.go, che lo imposta solo con conservatoreNAS (nas.Conservatore sulla radice
//     del NAS, con la capacita' nas_scrittura): direttiva 35, C-6a;
//   - nel ciclo dei file e in storage/nas non c'e' os.Rename (sostituisce la destinazione): le copie si promuovono e si
//     tolgono solo con la rinomina senza sostituzione;
//   - il deposito locale non esiste piu': nessun riferimento a [conservazione], al segnaposto o a PreparaDeposito nel
//     codice di produzione.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestConservatoreLocaleSoloNelleProve(t *testing.T) {
	radice := radiceInternal(t)
	fset := token.NewFileSet()
	for _, p := range fileGo(t) {
		rel := filepath.ToSlash(mustRel(t, radice, p))
		if strings.HasPrefix(rel, "core/rfq/filerfq/conservatoreprova/") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			v, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasSuffix(v, "/filerfq/conservatoreprova") {
				t.Errorf("%s importa il doppio delle prove %s", rel, v)
			}
		}
	}
}

// il cablaggio ammesso: app/runtime/ciclo_file.go, solo con il conservatore del NAS
const fileCablaggio = "app/runtime/ciclo_file.go"

func TestNessunServizioConConservatoreInProduzione(t *testing.T) {
	radice := radiceInternal(t)
	fset := token.NewFileSet()
	cablaggi := 0
	for _, p := range fileGo(t) {
		rel := filepath.ToSlash(mustRel(t, radice, p))
		if strings.HasPrefix(rel, "core/rfq/filerfq/") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Conservatore" {
					t.Errorf("%s:%d imposta un Conservatore del ciclo dei file in un letterale (fuori dal cablaggio di %s)", rel,
						fset.Position(kv.Pos()).Line, fileCablaggio)
				}
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Conservatore" {
				if as, ok := parentAssign(f, sel); ok {
					if rel != fileCablaggio || !soloConservatoreNAS(as) {
						t.Errorf("%s:%d assegna un Conservatore del ciclo dei file fuori dal cablaggio di %s con conservatoreNAS", rel,
							fset.Position(as.Pos()).Line, fileCablaggio)
					} else {
						cablaggi++
					}
				}
			}
			return true
		})
	}
	if cablaggi != 1 {
		t.Errorf("il cablaggio del conservatore del NAS in %s: %d assegnazioni, attesa 1", fileCablaggio, cablaggi)
	}
}

// soloConservatoreNAS: l'assegnazione mette nel Conservatore il risultato di conservatoreNAS(...), e nient'altro.
func soloConservatoreNAS(as *ast.AssignStmt) bool {
	if len(as.Rhs) != 1 {
		return false
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "conservatoreNAS"
}

// TestConservatoreNASCostruisceSoloNasConservatore: conservatoreNAS (il cablaggio) costruisce un nas.Conservatore con
// la capacita' nas_scrittura riletta a ogni gesto (Scrittura) e il DryRun dell'avvio; nessun altro file di produzione,
// fuori da storage/nas, costruisce un nas.Conservatore.
func TestConservatoreNASCostruisceSoloNasConservatore(t *testing.T) {
	radice := radiceInternal(t)
	b, err := os.ReadFile(filepath.Join(radice, filepath.FromSlash(fileCablaggio)))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, atteso := range []string{"func conservatoreNAS(radice string, capacita coda.Capacita) *nas.Conservatore {",
		"return &nas.Conservatore{Radice: radice, DryRun: !capacita.NasScrittura,",
		"Scrittura: func() bool { return coda.CapacitaAttuali().NasScrittura }}"} {
		if !strings.Contains(s, atteso) {
			t.Errorf("%s: manca %q", fileCablaggio, atteso)
		}
	}
	for _, p := range fileGo(t) {
		rel := filepath.ToSlash(mustRel(t, radice, p))
		if rel == fileCablaggio || strings.HasPrefix(rel, "platform/storage/nas/") || strings.HasPrefix(rel, "core/rfq/filerfq/conservatoreprova/") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "nas.Conservatore{") {
			t.Errorf("%s costruisce un nas.Conservatore fuori dal cablaggio", rel)
		}
	}
}

// parentAssign: l'assegnazione in cui sel e' a sinistra, se c'e'.
func parentAssign(f *ast.File, sel *ast.SelectorExpr) (*ast.AssignStmt, bool) {
	var trovato *ast.AssignStmt
	ast.Inspect(f, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			for _, l := range as.Lhs {
				if l == sel {
					trovato = as
				}
			}
		}
		return trovato == nil
	})
	return trovato, trovato != nil
}

func TestNessunOsRenameSulleCopie(t *testing.T) {
	radice := radiceInternal(t)
	fset := token.NewFileSet()
	for _, cartella := range []string{"core/rfq/filerfq", "platform/storage/nas"} {
		voci, err := os.ReadDir(filepath.Join(radice, filepath.FromSlash(cartella)))
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range voci {
			if v.IsDir() || !strings.HasSuffix(v.Name(), ".go") || strings.HasSuffix(v.Name(), "_test.go") {
				continue
			}
			p := filepath.Join(radice, filepath.FromSlash(cartella), v.Name())
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Rename" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" {
						t.Errorf("%s/%s:%d usa os.Rename", cartella, v.Name(), fset.Position(sel.Pos()).Line)
					}
				}
				return true
			})
		}
	}
}

func TestDepositoLocaleTolto(t *testing.T) {
	radice := radiceInternal(t)
	for _, p := range fileGo(t) {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		rel := filepath.ToSlash(mustRel(t, radice, p))
		for _, vietato := range []string{"PreparaDeposito", ".cockpit-deposito", "cfg.Conservazione", "NomeSegnaposto"} {
			if strings.Contains(s, vietato) {
				t.Errorf("%s nomina ancora %q del deposito locale", rel, vietato)
			}
		}
	}
}

func mustRel(t *testing.T, base, p string) string {
	t.Helper()
	r, err := filepath.Rel(base, p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
