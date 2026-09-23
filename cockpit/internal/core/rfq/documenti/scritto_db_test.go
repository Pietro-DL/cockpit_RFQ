//go:build integrazione

package documenti

import (
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// B8.3, addendum A1.4 regola 2, l'ultima difesa: «scritto» vale per il percorso su cui il file e'
// finito. La copia legge il percorso quando parte; se nel frattempo una correzione del codice l'ha
// cambiato, segnare scritto il documento vorrebbe dire dichiarare un file in una cartella dove non
// c'e'. SetDocumentoScritto con il percorso vecchio non scrive niente, e il documento resta da copiare.
func TestScrittoValeSoloPerIlPercorsoCopiato(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, _ := bancoIntegrita(t, ctx, p, "S")
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	copiato := d.PathRelativo
	nuovo := `ELENCO DISEGNI\ALTRO\disegno.pdf`
	if _, err := q.SetPathDocumentoInCoda(ctx, db.SetPathDocumentoInCodaParams{DocumentoID: doc, PathRelativo: nuovo}); err != nil {
		t.Fatal(err)
	}

	n, err := q.SetDocumentoScritto(ctx, db.SetDocumentoScrittoParams{DocumentoID: doc, PathRelativo: copiato})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("segnato scritto con il percorso vecchio (%d righe)", n)
	}
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasInCoda {
		t.Errorf("stato %s, atteso in_coda: il file non e' al percorso del documento", s)
	}

	if n, err = q.SetDocumentoScritto(ctx, db.SetDocumentoScrittoParams{DocumentoID: doc, PathRelativo: nuovo}); err != nil || n != 1 {
		t.Fatalf("con il percorso giusto: %d righe, %v", n, err)
	}
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasScritto {
		t.Errorf("stato %s, atteso scritto", s)
	}
	// e un documento scritto non cambia piu' percorso da SetPathDocumentoInCoda (lo spostamento e' B8.8)
	if n, err := q.SetPathDocumentoInCoda(ctx, db.SetPathDocumentoInCodaParams{DocumentoID: doc, PathRelativo: copiato}); err != nil || n != 0 {
		t.Errorf("SetPathDocumentoInCoda su uno scritto: %d righe, %v", n, err)
	}
}
