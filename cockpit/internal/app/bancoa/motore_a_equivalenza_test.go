package bancoa

// L1 — giro 5, A1d-Q2, dopo la revisione di A1d-P2 (Q2-01): la guardia di deriva della prova di equivalenza fra la
// pagina della Distinta nuova e il banco (motore_a_equivalenza_db_test.go, L4). Quella prova esegue le funzioni del banco
// (messaggiFuori, clientiFuori, caricaSenzaIMancanti, inFiles, inProdotti), ma la sequenza che le lega è ricopiata da
// fotografiaDSN e da valuta. Qui si leggono banco.go e il file della prova con go/parser e si confrontano:
//   - in fotografiaDSN e in eqPercorsoDelBanco, la chiamata caricaSenzaIMancanti(…, b.messaggiFuori()) con dentro
//     caricatore.Carica(ctx, pool, caricatore.Richiesta{…}): le stesse chiavi del letterale, con gli stessi valori
//     (Sorgente a parte, che nel banco è la destinazione del rapporto);
//   - in valuta e in eqPercorsoDelBanco, la stessa chiamata valut.Calcola(f, b.ins, b.ingressi), lo stesso
//     righe := inFiles(et.Confrontabili) e lo stesso confronto.Confronta(righe, inProdotti(et.ProdottiConfrontabili),
//     atteso);
//   - in valuta, atteso è un *confronto.Atteso dichiarato nil, assegnato solo dentro if tr != nil, e tr solo dentro
//     if b.o.Attesi: senza -attesi l'atteso è nil, come nella prova (che non lo assegna mai).
//
// Se il banco cambia la Richiesta (un campo nuovo, un filtro) o la sequenza (un atteso anche senza -attesi), la guardia
// cade e la prova di equivalenza va riallineata. Gira senza database e senza il tag integrazione: il file della prova si
// legge, non si compila.
//
// Qui non ci sono clienti: si leggono solo i sorgenti del package.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// eqFunzione: la dichiarazione di funzione (o di metodo) con quel nome nel file.
func eqFunzione(t *testing.T, fset *token.FileSet, percorso, nome string) *ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(fset, percorso, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == nome {
			return fd
		}
	}
	t.Fatalf("%s: la funzione %s non c'è più: la guardia va riallineata", percorso, nome)
	return nil
}

// eqTesto: un nodo stampato, su una riga, senza spazi in più.
func eqTesto(t *testing.T, fset *token.FileSet, n ast.Node) string {
	t.Helper()
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, n); err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// eqChiamate: le chiamate nel corpo della funzione il cui Fun stampato è nome (per esempio «valut.Calcola»).
func eqChiamate(t *testing.T, fset *token.FileSet, fd *ast.FuncDecl, nome string) []*ast.CallExpr {
	t.Helper()
	var out []*ast.CallExpr
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && eqTesto(t, fset, c.Fun) == nome {
			out = append(out, c)
		}
		return true
	})
	return out
}

// eqRichiesta: in fd, la chiamata caricaSenzaIMancanti (una sola), il secondo argomento stampato, la chiamata
// caricatore.Carica dentro il primo e il suo letterale caricatore.Richiesta, come chiave → valore stampato.
func eqRichiesta(t *testing.T, fset *token.FileSet, fd *ast.FuncDecl) (messaggi, carica string, campi map[string]string) {
	t.Helper()
	cc := eqChiamate(t, fset, fd, "caricaSenzaIMancanti")
	if len(cc) != 1 || len(cc[0].Args) != 2 {
		t.Fatalf("%s: %d chiamate a caricaSenzaIMancanti, attesa una con due argomenti", fd.Name.Name, len(cc))
	}
	messaggi = eqTesto(t, fset, cc[0].Args[1])
	var lit *ast.CompositeLit
	ast.Inspect(cc[0].Args[0], func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || eqTesto(t, fset, c.Fun) != "caricatore.Carica" || len(c.Args) != 3 {
			return true
		}
		carica = eqTesto(t, fset, c.Fun) + "(" + eqTesto(t, fset, c.Args[0]) + ", " + eqTesto(t, fset, c.Args[1]) + ", …)"
		if l, ok := c.Args[2].(*ast.CompositeLit); ok && eqTesto(t, fset, l.Type) == "caricatore.Richiesta" {
			lit = l
		}
		return false
	})
	if lit == nil {
		t.Fatalf("%s: caricatore.Carica con un letterale caricatore.Richiesta non c'è più", fd.Name.Name)
	}
	campi = map[string]string{}
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			t.Fatalf("%s: il letterale caricatore.Richiesta ha un elemento senza chiave", fd.Name.Name)
		}
		campi[eqTesto(t, fset, kv.Key)] = eqTesto(t, fset, kv.Value)
	}
	return messaggi, carica, campi
}

// eqSoloDentro: ogni assegnazione a nome nel corpo di fd sta dentro il ramo di un if la cui condizione stampata è cond.
// Restituisce quante sono.
func eqSoloDentro(t *testing.T, fset *token.FileSet, fd *ast.FuncDecl, nome, cond string) int {
	t.Helper()
	var pila []ast.Node
	n := 0
	ast.Inspect(fd.Body, func(x ast.Node) bool {
		if x == nil {
			pila = pila[:len(pila)-1]
			return true
		}
		pila = append(pila, x)
		as, ok := x.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, l := range as.Lhs {
			if id, ok := l.(*ast.Ident); !ok || id.Name != nome {
				continue
			}
			n++
			dentro := false
			for i := len(pila) - 2; i >= 0; i-- {
				if ifs, ok := pila[i].(*ast.IfStmt); ok && eqTesto(t, fset, ifs.Cond) == cond && as.Pos() >= ifs.Body.Pos() && as.End() <= ifs.Body.End() {
					dentro = true
					break
				}
			}
			if !dentro {
				t.Errorf("%s: %s si assegna fuori da «if %s» (%s): senza -attesi l'atteso non sarebbe più nil", fd.Name.Name, nome, cond, fset.Position(as.Pos()))
			}
		}
		return true
	})
	return n
}

// eqDichiarazione: la dichiarazione «var nome tipo» c'è nel corpo di fd.
func eqDichiarazione(t *testing.T, fset *token.FileSet, fd *ast.FuncDecl, testo string) bool {
	t.Helper()
	trovata := false
	ast.Inspect(fd.Body, func(x ast.Node) bool {
		if d, ok := x.(*ast.DeclStmt); ok && eqTesto(t, fset, d) == testo {
			trovata = true
		}
		return true
	})
	return trovata
}

func TestEquivalenzaMotoreAIlPercorsoDelBancoNonDeriva(t *testing.T) {
	fset := token.NewFileSet()
	foto := eqFunzione(t, fset, "banco.go", "fotografiaDSN")
	valuta := eqFunzione(t, fset, "banco.go", "valuta")
	prova := eqFunzione(t, fset, "motore_a_equivalenza_db_test.go", "eqPercorsoDelBanco")

	// la Richiesta di fotografiaDSN
	mb, cb, rb := eqRichiesta(t, fset, foto)
	mp, cp, rp := eqRichiesta(t, fset, prova)
	if mb != mp || cb != cp {
		t.Errorf("la fotografia: banco caricaSenzaIMancanti(…, %s) con %s; prova (…, %s) con %s", mb, cb, mp, cp)
	}
	chiavi := func(m map[string]string) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	if a, b := strings.Join(chiavi(rb), " "), strings.Join(chiavi(rp), " "); a != b {
		t.Errorf("le chiavi di caricatore.Richiesta: banco %q, prova %q", a, b)
	}
	for _, k := range chiavi(rb) {
		if k == "Sorgente" {
			continue // nel banco è la destinazione del rapporto
		}
		if rb[k] != rp[k] {
			t.Errorf("caricatore.Richiesta.%s: banco %q, prova %q", k, rb[k], rp[k])
		}
	}
	for _, k := range []string{"Thread", "Tutti", "Messaggi", "Clienti", "Sorgente"} {
		if _, ok := rb[k]; !ok {
			t.Errorf("la Richiesta del banco non ha più %s: la scena -thread / -tutti della prova va riletta", k)
		}
	}

	// la sequenza di valuta, senza -attesi
	for _, c := range []struct{ nome, atteso string }{
		{"valut.Calcola", "valut.Calcola(f, b.ins, b.ingressi)"},
		{"confronto.Confronta", "confronto.Confronta(righe, inProdotti(et.ProdottiConfrontabili), atteso)"},
		{"inFiles", "inFiles(et.Confrontabili)"},
	} {
		for chi, fd := range map[string]*ast.FuncDecl{"banco.go, valuta": valuta, "la prova": prova} {
			cc := eqChiamate(t, fset, fd, c.nome)
			if len(cc) != 1 || eqTesto(t, fset, cc[0]) != c.atteso {
				var testi []string
				for _, x := range cc {
					testi = append(testi, eqTesto(t, fset, x))
				}
				t.Errorf("%s: %s chiamato %q, atteso una volta %q", chi, c.nome, testi, c.atteso)
			}
		}
	}
	for chi, fd := range map[string]*ast.FuncDecl{"banco.go, valuta": valuta, "la prova": prova} {
		if !eqDichiarazione(t, fset, fd, "var atteso *confronto.Atteso") {
			t.Errorf("%s: «var atteso *confronto.Atteso» non c'è più", chi)
		}
	}
	if n := eqSoloDentro(t, fset, valuta, "atteso", "tr != nil"); n == 0 {
		t.Error("banco.go, valuta: atteso non si assegna più dentro «if tr != nil»: la guardia va riletta")
	}
	if n := eqSoloDentro(t, fset, valuta, "tr", "b.o.Attesi"); n == 0 {
		t.Error("banco.go, valuta: tr non si assegna più dentro «if b.o.Attesi»: la guardia va riletta")
	}
	if n := eqSoloDentro(t, fset, prova, "atteso", "false"); n != 0 {
		t.Errorf("la prova assegna atteso %d volte: senza -attesi resta nil", n)
	}
}
