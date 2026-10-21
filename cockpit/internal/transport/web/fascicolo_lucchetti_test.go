package web

// L1 statica — V1, decisioni dell'ondata B §7 [T] (corsia file, autorizzata su questo gestore legacy per questa sola
// riga): «Documentazione generale» (fileGenerale) prende la RFQ FOR KEY SHARE prima di bloccare la proposta, come la
// conferma della voce e come i gesti della Sorgente (prima la RFQ, poi le proposte). Lo stallo con l'ordine rovesciato
// non si riproduce in modo deterministico: la prova guarda l'ordine delle chiamate nel sorgente.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestDocumentazioneGeneralePrendeLaRFQPrimaDellaProposta(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "fascicolo_gesti_v3.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var corpo *ast.BlockStmt
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "fileGenerale" {
			corpo = fd.Body
		}
	}
	if corpo == nil {
		t.Fatal("fileGenerale non trovato in fascicolo_gesti_v3.go")
	}
	prima := map[string]token.Pos{}
	ast.Inspect(corpo, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
				if _, visto := prima[sel.Sel.Name]; !visto {
					prima[sel.Sel.Name] = c.Pos()
				}
			}
		}
		return true
	})
	rfq, okRfq := prima["BloccaThreadCondiviso"]
	prop, okProp := prima["BloccaProposta"]
	if !okRfq || !okProp {
		t.Fatalf("fileGenerale: BloccaThreadCondiviso %v, BloccaProposta %v", okRfq, okProp)
	}
	if rfq >= prop {
		t.Errorf("fileGenerale blocca la proposta (riga %d) prima della RFQ (riga %d)", fset.Position(prop).Line,
			fset.Position(rfq).Line)
	}
}
