package fascicolo

// L1 — Smistamento F8 (addendum A5.13.9, FP3, FP4, P24): che cosa il flusso ancorato al prodotto scrive, letto
// nei sorgenti. Il flusso legge lo stato e scrive una cosa sola, dettagli.destinazione con ScriviDestinazione;
// abbassa il numero di priorita' delle analisi pronte con AlzaPrioritaJob. Nessun'altra query del flusso
// scrive: ne' componente, ne' componente_relazione, ne' documento, ne' le colonne della proposta. E i soli
// chiamanti delle due query sono questi. Una scorciatoia aggiunta domani la ferma questa prova.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// chiamateSuQ sono i metodi chiamati su q (le query) in un file.
func chiamateSuQ(t *testing.T, file string) map[string]bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if s, ok := c.Fun.(*ast.SelectorExpr); ok {
				if id, ok := s.X.(*ast.Ident); ok && id.Name == "q" {
					out[s.Sel.Name] = true
				}
			}
		}
		return true
	})
	return out
}

func TestIlFlussoScriveSoloLeDestinazioni(t *testing.T) {
	// le letture del flusso, e la sola scrittura
	ammesse := map[string]bool{
		"GetThread": true, "GetCliente": true, "ListIdentificativi": true, "ListComponentiThread": true, "ListRelazioniAttive": true,
		"ListComponenteProposteThread": true, "ListRelazioneProposteThread": true, "ListFileDelFlusso": true,
		"ListProposteDocumentoThread": true, "ListShaConFattiCorrenti": true, "ListLavoroPendenteRfq": true, "ListUltimiJobPerChiavi": true,
		"ScriviDestinazione": true,
	}
	file, err := filepath.Glob("smistamento_*.go")
	if err != nil {
		t.Fatal(err)
	}
	var visti []string
	for _, f := range file {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for m := range chiamateSuQ(t, f) {
			visti = append(visti, m)
			if !ammesse[m] {
				t.Errorf("%s chiama q.%s: il flusso legge lo stato e scrive solo la destinazione (FP3, FP4)", f, m)
			}
		}
	}
	sort.Strings(visti)
	if !strings.Contains(strings.Join(visti, " "), "ScriviDestinazione") {
		t.Fatalf("il censimento non vede la scrittura del flusso: %v", visti)
	}

	// i chiamanti delle due scritture, in tutto il codice di produzione
	radice := filepath.Join("..", "..", "..")
	chiamanti := map[string][]string{}
	err = filepath.WalkDir(radice, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") ||
			strings.Contains(filepath.ToSlash(p), "platform/db/") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, q := range []string{".ScriviDestinazione(", ".AlzaPrioritaJob("} {
			if strings.Contains(string(b), q) {
				chiamanti[q] = append(chiamanti[q], filepath.ToSlash(p))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(chiamanti[".ScriviDestinazione("], " "); !strings.HasSuffix(got, "core/rfq/fascicolo/smistamento_flusso.go") || strings.Contains(got, " ") {
		t.Errorf("chi scrive la destinazione: %s (solo il flusso)", got)
	}
	if got := strings.Join(chiamanti[".AlzaPrioritaJob("], " "); !strings.HasSuffix(got, "platform/coda/priorita.go") || strings.Contains(got, " ") {
		t.Errorf("chi alza la priorita' dei job: %s (solo coda.AlzaPriorita)", got)
	}
}
