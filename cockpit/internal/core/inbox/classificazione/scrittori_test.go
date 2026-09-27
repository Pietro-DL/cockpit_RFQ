package classificazione

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestLaConfidenzaSiScriveSoloDalRiepilogo (Smistamento, prova 229, A5.14.7, P24 esteso): la colonna
// `confidenza` di documento_proposta e' il riepilogo della valutazione, e nessuno scrittore ci mette un numero
// suo. Si leggono i sorgenti (non le prove): ogni UpsertPropostaParams, InsertPropostaSeAssenteParams e
// AggiornaValutazionePropostaParams costruito in Go ha `Confidenza: int16(x.Confidenza)` con x preso, nella
// stessa funzione, da ConValutazione o da Riepilogo(). E le sole query che scrivono la colonna sono quelle
// dichiarate: le tre di sopra, e la decisione dell'operatore (100, che non e' uno score).
func TestLaConfidenzaSiScriveSoloDalRiepilogo(t *testing.T) {
	radice := filepath.Join("..", "..", "..")
	parametri := map[string]bool{"UpsertPropostaParams": true, "InsertPropostaSeAssenteParams": true, "AggiornaValutazionePropostaParams": true}
	trovati := map[string]int{}
	err := filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "db" && strings.HasSuffix(filepath.Dir(p), "platform") {
				return filepath.SkipDir // il codice generato definisce i parametri, non li riempie
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			dalRiepilogo := variabiliDalRiepilogo(fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				sel, ok := cl.Type.(*ast.SelectorExpr)
				if !ok || !parametri[sel.Sel.Name] {
					return true
				}
				dove := fset.Position(cl.Pos()).String()
				trovati[sel.Sel.Name]++
				conf := false
				for _, el := range cl.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						t.Errorf("%s: %s senza i nomi dei campi", dove, sel.Sel.Name)
						continue
					}
					if k, _ := kv.Key.(*ast.Ident); k == nil || k.Name != "Confidenza" {
						continue
					}
					conf = true
					if x := variabileDellaConfidenza(kv.Value); x == "" || !dalRiepilogo[x] {
						t.Errorf("%s: %s.Confidenza non viene dal riepilogo della valutazione", dove, sel.Sel.Name)
					}
				}
				if !conf {
					t.Errorf("%s: %s senza la confidenza (resterebbe 0, non il riepilogo)", dove, sel.Sel.Name)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// gli scrittori ci sono tutti: ingest (Insert), la lettura del server (Upsert), struttura e risposta (Aggiorna)
	if trovati["InsertPropostaSeAssenteParams"] != 1 || trovati["UpsertPropostaParams"] != 1 || trovati["AggiornaValutazionePropostaParams"] != 2 {
		t.Errorf("scrittori di documento_proposta: %v", trovati)
	}

	// le query: chi scrive la colonna confidenza di documento_proposta
	query := filepath.Join(radice, "platform", "db", "queries")
	voci, err := os.ReadDir(query)
	if err != nil {
		t.Fatal(err)
	}
	reNome := regexp.MustCompile(`(?m)^-- name: (\w+)`)
	var scrivono []string
	for _, v := range voci {
		b, err := os.ReadFile(filepath.Join(query, v.Name()))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		idx := reNome.FindAllStringSubmatchIndex(s, -1)
		for i, m := range idx {
			fine := len(s)
			if i+1 < len(idx) {
				fine = idx[i+1][0]
			}
			corpo := senzaCommentiSQL(s[m[1]:fine])
			tocca := strings.Contains(corpo, "UPDATE documento_proposta") || strings.Contains(corpo, "INSERT INTO documento_proposta")
			if tocca && strings.Contains(corpo, "confidenza") {
				scrivono = append(scrivono, s[m[2]:m[3]])
			}
		}
	}
	sort.Strings(scrivono)
	if got := strings.Join(scrivono, " "); got != "AggiornaValutazioneProposta DecidiPropostaDocumento InsertPropostaSeAssente UpsertProposta" {
		t.Errorf("query che scrivono la confidenza di documento_proposta: %s", got)
	}
}

// variabiliDalRiepilogo sono i nomi a cui la funzione assegna il risultato di ConValutazione (secondo valore) o
// di un Riepilogo().
func variabiliDalRiepilogo(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		nome := ""
		switch f := call.Fun.(type) {
		case *ast.SelectorExpr:
			nome = f.Sel.Name
		case *ast.Ident:
			nome = f.Name
		}
		switch {
		case nome == "ConValutazione" && len(as.Lhs) == 2:
			if id, ok := as.Lhs[1].(*ast.Ident); ok {
				out[id.Name] = true
			}
		case nome == "Riepilogo" && len(as.Lhs) == 1:
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				out[id.Name] = true
			}
		}
		return true
	})
	return out
}

// variabileDellaConfidenza: da `int16(x.Confidenza)` restituisce x.
func variabileDellaConfidenza(e ast.Expr) string {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return ""
	}
	if f, ok := call.Fun.(*ast.Ident); !ok || f.Name != "int16" {
		return ""
	}
	sel, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Confidenza" {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

// senzaCommentiSQL toglie le righe di commento, che parlano della colonna senza scriverla.
func senzaCommentiSQL(s string) string {
	var b strings.Builder
	for _, r := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(r), "--") {
			b.WriteString(r + "\n")
		}
	}
	return b.String()
}
