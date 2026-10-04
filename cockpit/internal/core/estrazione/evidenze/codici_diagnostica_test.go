package evidenze

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// L1 — i codici delle diagnostiche del motore A (A1a-CAT; parte 1 §7.3 «codice stabile», E-23, R41 b).
// Ogni pacchetto dichiara i codici che produce nel suo codici_diagnostica.go; questa prova li raccoglie
// tutti, dai sorgenti, e controlla che:
//  1. abbiano il formato «area.caso», in minuscolo;
//  2. nessun valore sia dichiarato due volte, nello stesso pacchetto o in due pacchetti;
//  3. ogni codice dell'elenco d'oro (testdata/codici_diagnostica.txt) sia ancora dichiarato: un codice
//     pubblicato non sparisce e non cambia nome; uno ritirato resta, con il commento «ritirato»;
//  4. ogni codice dichiarato sia nell'elenco d'oro: un codice nuovo vi entra nello stesso commit «le prove»;
//  5. nei file non di prova del motore, il campo Codice di un letterale Diagnostica sia sempre una delle
//     costanti dichiarate, mai una stringa scritta sul posto.
//
// I clienti di questi test sono inventati: qui non ce n'è nessuno, l'elenco d'oro contiene solo codici.

// formatoCodice: «<area>.<caso>», lettere minuscole ASCII, cifre e trattino basso.
var formatoCodice = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)

// codiceDichiarato: una costante di un file codici_diagnostica.go.
type codiceDichiarato struct {
	nome, valore, dove string
}

// codiciDichiarati raccoglie le costanti stringa di tutti i file codici_diagnostica.go del modulo.
func codiciDichiarati(t *testing.T, radice string) []codiceDichiarato {
	t.Helper()
	var out []codiceDichiarato
	err := filepath.WalkDir(filepath.Join(radice, "internal"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "codici_diagnostica.go" {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, nome := range vs.Names {
					if i >= len(vs.Values) {
						t.Errorf("%s: %s senza valore", fset.Position(nome.Pos()), nome.Name)
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Errorf("%s: %s non è una costante stringa scritta per intero", fset.Position(nome.Pos()), nome.Name)
						continue
					}
					v, _ := strconv.Unquote(lit.Value)
					out = append(out, codiceDichiarato{nome: nome.Name, valore: v, dove: fset.Position(nome.Pos()).String()})
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// elencoDoro legge testdata/codici_diagnostica.txt: un codice per riga, nessun altro dato.
func elencoDoro(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "codici_diagnostica.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		if r := strings.TrimSpace(s.Text()); r != "" {
			out = append(out, r)
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCodiciDiagnosticaUniciEStabili(t *testing.T) {
	radice := radiceDelModulo(t)
	dichiarati := codiciDichiarati(t, radice)
	if len(dichiarati) == 0 {
		t.Fatal("nessun codice dichiarato: la prova non legge i file giusti")
	}

	// 1 e 2: formato e unicità.
	perValore := map[string]codiceDichiarato{}
	nomi := map[string]bool{}
	for _, c := range dichiarati {
		if !formatoCodice.MatchString(c.valore) {
			t.Errorf("%s: %s = %q non ha il formato «area.caso» in minuscolo", c.dove, c.nome, c.valore)
		}
		if prima, ok := perValore[c.valore]; ok {
			t.Errorf("%q dichiarato due volte: %s (%s) e %s (%s)", c.valore, prima.nome, prima.dove, c.nome, c.dove)
		}
		perValore[c.valore] = c
		nomi[c.nome] = true
	}

	// 3 e 4: stabilità rispetto all'elenco d'oro.
	doro := elencoDoro(t)
	nellOro := map[string]bool{}
	for _, c := range doro {
		if nellOro[c] {
			t.Errorf("elenco d'oro: %q ripetuto", c)
		}
		nellOro[c] = true
		if _, ok := perValore[c]; !ok {
			t.Errorf("il codice pubblicato %q non è più dichiarato: un codice non sparisce, al più si ritira", c)
		}
	}
	valori := make([]string, 0, len(perValore))
	for v := range perValore {
		valori = append(valori, v)
	}
	sort.Strings(valori)
	for _, v := range valori {
		if !nellOro[v] {
			t.Errorf("%s: il codice %q non è nell'elenco d'oro: va aggiunto nello stesso commit delle prove", perValore[v].dove, v)
		}
	}

	// 5: nei sorgenti del motore, il codice di una Diagnostica è sempre una costante dichiarata.
	for _, r := range pacchettiMotoreA {
		fset, files := fileAnalizzati(t, radice, r.percorso)
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if eDiagnostica(cl.Type) {
					controllaCodice(t, fset, cl, nomi)
				}
				// []Diagnostica{{...}}: i letterali interni hanno il tipo sottinteso.
				if at, ok := cl.Type.(*ast.ArrayType); ok && eDiagnostica(at.Elt) {
					for _, el := range cl.Elts {
						if interno, ok := el.(*ast.CompositeLit); ok && interno.Type == nil {
							controllaCodice(t, fset, interno, nomi)
						}
					}
				}
				return true
			})
		}
	}
}

// eDiagnostica: il tipo è Diagnostica (dentro la foglia) o <pacchetto>.Diagnostica (fuori).
func eDiagnostica(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "Diagnostica"
	case *ast.SelectorExpr:
		return x.Sel.Name == "Diagnostica"
	}
	return false
}

func controllaCodice(t *testing.T, fset *token.FileSet, cl *ast.CompositeLit, nomi map[string]bool) {
	t.Helper()
	dove := fset.Position(cl.Pos())
	var valore ast.Expr
	for i, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Codice" {
				valore = kv.Value
			}
		} else if i == 0 {
			valore = el // letterale senza nomi: il primo campo è Codice
		}
	}
	if valore == nil {
		if len(cl.Elts) > 0 {
			t.Errorf("%s: Diagnostica senza codice", dove)
		}
		return
	}
	nome := ""
	switch x := valore.(type) {
	case *ast.Ident:
		nome = x.Name
	case *ast.SelectorExpr:
		nome = x.Sel.Name
	}
	if !nomi[nome] {
		t.Errorf("%s: il codice della Diagnostica non è una costante di un codici_diagnostica.go", dove)
	}
}
