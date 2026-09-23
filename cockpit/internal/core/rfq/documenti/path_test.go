package documenti

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCartellaThread(t *testing.T) {
	d := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	casi := []struct{ cliente, cognome, oggetto, atteso string }{
		{"ACME", "Rossi", "Supporto cofano", `ACME\WIP\2026 09 08 Rossi Supporto cofano`},
		{"LANDINI ARGO", "Bianchi", "RE: R: RICHIESTA D'OFFERTA 123456789", `LANDINI ARGO\WIP\2026 09 08 Bianchi RICHIESTA D'OFFERTA 123456789`},
		{"ACME", "", "Staffa: nuova/rev. 2?", `ACME\WIP\2026 09 08 Staffa nuova rev. 2`},
		{"ACME", "Rossi", "", `ACME\WIP\2026 09 08 Rossi senza nome`},
		{"ACME", "Rossi", strings.Repeat("x", 100), `ACME\WIP\2026 09 08 Rossi ` + strings.Repeat("x", 60)},
		{"ACME", "Rossi", "Fine con punto.", `ACME\WIP\2026 09 08 Rossi Fine con punto`},
	}
	for _, c := range casi {
		if got := CartellaThread(c.cliente, d, c.cognome, c.oggetto); got != c.atteso {
			t.Errorf("CartellaThread(%q,%q,%q) = %q, atteso %q", c.cliente, c.cognome, c.oggetto, got, c.atteso)
		}
	}
}

func TestPathDocumento(t *testing.T) {
	disegni := LayoutDocumento{Sottocartella: "ELENCO DISEGNI", PerCodice: true}
	radice := LayoutDocumento{Sottocartella: "", PerCodice: false}
	casi := []struct {
		l         LayoutDocumento
		perCodice bool
		codice    string
		nome      string
		atteso    string
	}{
		{disegni, true, "6674611A", "6674611A_4.pdf", `ELENCO DISEGNI\6674611A\6674611A_4.pdf`},
		{disegni, false, "6674611A", "6674611A_4.pdf", `ELENCO DISEGNI\6674611A_4.pdf`},
		// il cliente senza cartella per codice: il layout non la chiede, e il codice non serve al percorso
		{disegni, false, "", "assieme.STEP", `ELENCO DISEGNI\assieme.step`},
		{radice, true, "6674611A", "SO 5467.pdf", `SO 5467.pdf`},
		{LayoutDocumento{"OFFERTE FORNITORI", false}, true, "", "verniciatura?.pdf", `OFFERTE FORNITORI\verniciatura.pdf`},
	}
	for _, c := range casi {
		got, err := PathDocumento(c.l, c.perCodice, c.codice, c.nome)
		if err != nil || got != c.atteso {
			t.Errorf("PathDocumento(%v,%v,%q,%q) = %q, %v; atteso %q", c.l, c.perCodice, c.codice, c.nome, got, err, c.atteso)
		}
	}
}

// A2.2: se il layout vuole la cartella del codice e il codice manca, il file NON va nella
// sottocartella comune. Fino alla 0018 un assieme.STEP senza codice finiva in ELENCO DISEGNI e
// basta, in silenzio: era il terzo caso della prova qui sopra, che ora vale solo per il cliente
// senza cartella per codice.
func TestPathDocumentoSenzaCodiceNonScegliUnaCartella(t *testing.T) {
	disegni := LayoutDocumento{Sottocartella: "ELENCO DISEGNI", PerCodice: true}
	for _, codice := range []string{"", "   ", "\t"} {
		got, err := PathDocumento(disegni, true, codice, "assieme.STEP")
		if !errors.Is(err, ErrCodiceMancante) || got != "" {
			t.Errorf("PathDocumento senza codice (%q) = %q, %v; atteso ErrCodiceMancante e nessun percorso", codice, got, err)
		}
	}
}
