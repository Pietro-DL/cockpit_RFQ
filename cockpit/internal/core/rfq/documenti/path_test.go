package documenti

import (
	"errors"
	"go/scanner"
	"go/token"
	"io/fs"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestCartellaThread(t *testing.T) {
	d := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	casi := []struct{ cliente, cognome, oggetto, atteso string }{
		{"ACME", "Rossi", "Supporto cofano", `ACME\WIP\2026 09 08 Rossi Supporto cofano`},
		{"ACME ITALIA", "Bianchi", "RE: R: RICHIESTA D'OFFERTA 123456789", `ACME ITALIA\WIP\2026 09 08 Bianchi RICHIESTA D'OFFERTA 123456789`},
		{"ACME", "", "Staffa: nuova/rev. 2?", `ACME\WIP\2026 09 08 Staffa nuova rev. 2`},
		{"ACME", "Rossi", "", `ACME\WIP\2026 09 08 Rossi senza nome`},
		{"ACME", "Rossi", strings.Repeat("x", 100), `ACME\WIP\2026 09 08 Rossi ` + strings.Repeat("x", 60)},
		{"ACME", "Rossi", "Fine con punto.", `ACME\WIP\2026 09 08 Rossi Fine con punto`},
		// il taglio a 60 caratteri cade subito dopo il punto di «r.»: il punto non resta in coda
		{"ACME", "Rossi", strings.Repeat("x", 57) + " r. seguito", `ACME\WIP\2026 09 08 Rossi ` + strings.Repeat("x", 57) + " r"},
		// e lo stesso per la cartella del cliente, tagliata a 80
		{strings.Repeat("C", 79) + ". SRL", "Rossi", "Staffa", strings.Repeat("C", 79) + `\WIP\2026 09 08 Rossi Staffa`},
	}
	for _, c := range casi {
		if got := CartellaThread(c.cliente, d, c.cognome, c.oggetto); got != c.atteso {
			t.Errorf("CartellaThread(%q,%q,%q) = %q, atteso %q", c.cliente, c.cognome, c.oggetto, got, c.atteso)
		}
	}
}

// Un nome tagliato non finisce con un punto o uno spazio: Windows li toglie da solo (la cartella avrebbe
// un nome diverso da quello nel database) e con il prefisso \\?\ ne nasce una che Esplora risorse non
// apre. Il nome che esce, ripassato, resta uguale.
func TestNomeSicuroDopoIlTaglioNonFinisceConUnPunto(t *testing.T) {
	casi := []struct {
		s      string
		max    int
		atteso string
	}{
		{"Offerta rev. 2", 12, "Offerta rev"},
		{"abc. . . def", 7, "abc"},
		{"abc....def", 5, "abc"},
		{"...abc", 2, "senza nome"},
		{"àèìòù. ùòìèà", 6, "àèìòù"},
		{"corto.", 60, "corto"},
	}
	for _, c := range casi {
		got := NomeSicuro(c.s, c.max)
		if got != c.atteso {
			t.Errorf("NomeSicuro(%q, %d) = %q, atteso %q", c.s, c.max, got, c.atteso)
		}
		if strings.HasSuffix(got, ".") || strings.HasSuffix(got, " ") {
			t.Errorf("NomeSicuro(%q, %d) = %q finisce con un punto o uno spazio", c.s, c.max, got)
		}
		if due := NomeSicuro(got, c.max); got != "senza nome" && due != got {
			t.Errorf("NomeSicuro ripassato su %q = %q: non e' stabile", got, due)
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
		{disegni, true, "1234567A", "1234567A_4.pdf", `ELENCO DISEGNI\1234567A\1234567A_4.pdf`},
		{disegni, false, "1234567A", "1234567A_4.pdf", `ELENCO DISEGNI\1234567A_4.pdf`},
		// il cliente senza cartella per codice: il layout non la chiede, e il codice non serve al percorso
		{disegni, false, "", "assieme.STEP", `ELENCO DISEGNI\assieme.step`},
		{radice, true, "1234567A", "SO 5467.pdf", `SO 5467.pdf`},
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

// B8.3: la cartella e il nome si separano. CartellaDocumento e' la cartella di PathDocumento, e
// NomeNelPercorso ne ritrova il nome: una correzione di codice ricompone il percorso con la cartella
// nuova e il nome di prima, e il file non cambia nome.
func TestCartellaENomeRicompongonoIlPercorso(t *testing.T) {
	disegni := LayoutDocumento{Sottocartella: "ELENCO DISEGNI", PerCodice: true}
	casi := []struct {
		l         LayoutDocumento
		perCodice bool
		codice    string
		cartella  string
	}{
		{disegni, true, "ab12", `ELENCO DISEGNI\AB12`},
		{disegni, false, "ab12", `ELENCO DISEGNI`},
		{LayoutDocumento{"", false}, true, "ab12", ``},
	}
	for _, c := range casi {
		cartella, err := CartellaDocumento(c.l, c.perCodice, c.codice)
		if err != nil || cartella != c.cartella {
			t.Errorf("CartellaDocumento(%v,%v,%q) = %q, %v; atteso %q", c.l, c.perCodice, c.codice, cartella, err, c.cartella)
		}
		p, _ := PathDocumento(c.l, c.perCodice, c.codice, "Disegno 1.PDF")
		if got := NellaCartella(cartella, NomeNelPercorso(p)); got != p {
			t.Errorf("cartella %q + nome di %q = %q", cartella, p, got)
		}
	}
	if _, err := CartellaDocumento(disegni, true, " "); !errors.Is(err, ErrCodiceMancante) {
		t.Errorf("CartellaDocumento senza codice: %v, atteso ErrCodiceMancante", err)
	}
	if got := NomeNelPercorso(`ELENCO DISEGNI\AB12\LEGGIMI`); got != "LEGGIMI" {
		t.Errorf("NomeNelPercorso = %q, atteso LEGGIMI", got)
	}
	if got := NomeNelPercorso(`SO 5467.pdf`); got != `SO 5467.pdf` {
		t.Errorf("NomeNelPercorso senza cartella = %q", got)
	}
}

// Prova 52 (addendum A4.1, R1.7): NomeFileSicuro, ripassata sul nome che ha prodotto, lo restituisce
// uguale. Prima «LEGGIMI» diventava «LEGGIMIsenza nome» e poi «LEGGIMIsenza nomesenza nome»; e
// fuori da quel caso cambiavano alla seconda passata anche «a.B.», «a.B/c» e un nome lungo il cui
// taglio finiva su un punto.
func TestNomeFileSicuroEIdempotente(t *testing.T) {
	idempotente := func(t *testing.T, nome string) string {
		t.Helper()
		una := NomeFileSicuro(nome)
		if due := NomeFileSicuro(una); due != una {
			t.Errorf("NomeFileSicuro(%q) = %q, e ripassato %q", nome, una, due)
		}
		return una
	}
	a := func(n int) string { return strings.Repeat("a", n) }

	casi := []struct{ nome, atteso string }{
		{"LEGGIMI", "LEGGIMI"},
		{"nome.", "nome"},
		{".", "senza nome"},
		{"", "senza nome"},
		{".pdf", "senza nome.pdf"},
		{"Disegno 1.PDF", "Disegno 1.pdf"},
		// doppi punti
		{"..", "senza nome"},
		{"a..pdf", "a.pdf"},
		{"a..B", "a.b"},
		{"file.tar.GZ", "file.tar.gz"},
		// spazi e punti finali
		{"nome . . ", "nome"},
		{"nome.pdf . ", "nome.pdf"},
		{"  Tavola  2 .PDF  ", "Tavola 2.pdf"},
		{"a.B.", "a.b"},
		// dopo la pulizia la barra e' uno spazio, e «.B c» con lo spazio non e' un'estensione
		{"a.B/c", "a.B c"},
		{"cartella/Disegno.PDF", "cartella Disegno.pdf"},
		// un punto che non apre un'estensione: troppo lunga, o con uno spazio
		{"Offerta 12.03.2026 rev finale", "Offerta 12.03.2026 rev finale"},
		{"Offerta 12.03.2026 REV", "Offerta 12.03.2026 REV"},
		{"a." + strings.Repeat("X", 9), "a." + strings.Repeat("x", 9)},
		{"a." + strings.Repeat("X", 10), "a." + strings.Repeat("X", 10)},
		// al limite di lunghezza
		{a(maxNomeFile) + ".PDF", a(maxNomeFile) + ".pdf"},
		{a(maxNomeFile) + "b.PDF", a(maxNomeFile) + ".pdf"},
		{a(maxNomeFile-1) + ".b.PDF", a(maxNomeFile-1) + ".pdf"},
		{a(maxNomeFile + 10), a(maxNomeFile)},
		{a(maxNomeFile-2) + ".B" + strings.Repeat("c", 20), a(maxNomeFile-2) + ".b"},
		{a(maxNomeFile-1) + ". c", a(maxNomeFile - 1)},
		// non ASCII
		{"Staffa – rev À.PDF", "Staffa – rev À.pdf"},
		{"日本語.PDF", "日本語.pdf"},
		{"ÉTUDE.ÉPS", "ÉTUDE.éps"},
		{"nome\u00a0.pdf", "nome.pdf"},
		{"Rev\u00a0B.\u00a0", "Rev\u00a0B"},
		{"é" + strings.Repeat("è", maxNomeFile) + ".PDF", "é" + strings.Repeat("è", maxNomeFile-1) + ".pdf"},
	}
	for _, c := range casi {
		if got := idempotente(t, c.nome); got != c.atteso {
			t.Errorf("NomeFileSicuro(%q) = %q, atteso %q", c.nome, got, c.atteso)
		}
	}

	// Ogni carattere, come estensione e subito prima: e' qui che la minuscola e il taglio potrebbero
	// dare un nome che alla passata dopo si legge in un altro modo.
	for r := rune(0); r <= unicode.MaxRune; r++ {
		idempotente(t, "a."+string(r))
		idempotente(t, "a"+string(r)+".B")
	}

	// Nomi a caso, fatti dei pezzi che danno fastidio: punti, spazi (anche non ASCII), barre, caratteri
	// vietati o di controllo, maiuscole che cambiano in minuscolo, un byte che non e' UTF-8.
	pezzi := []string{"a", "B", "7", ".", ".", " ", "  ", "\t", "\u00a0", "/", `\`, ":", "*", "?", `"`,
		"À", "ß", "İ", "K", "Σ", "日", "\x00", "\x1f", "\xff", "-", "_", "PDF", ".STEP"}
	g := rand.New(rand.NewSource(52))
	for i := 0; i < 50000; i++ {
		var b strings.Builder
		for n := g.Intn(200); n > 0; n-- {
			b.WriteString(pezzi[g.Intn(len(pezzi))])
		}
		idempotente(t, b.String())
	}

	// ...e un giro su tutti i nomi di file che le prove del modulo danno agli allegati.
	nomi := nomiDiFileNellaSuite(t)
	for _, attesi := range []string{"1234567A_4.pdf", "Tavola 1.PDF"} {
		if !nomi[attesi] {
			t.Fatalf("il giro sulle prove non ha trovato %q: %d nomi", attesi, len(nomi))
		}
	}
	for n := range nomi {
		idempotente(t, n)
	}
}

// nomiDiFileNellaSuite raccoglie le stringhe con la forma di un nome di file («qualcosa.ext») da tutti i
// *_test.go del modulo.
func nomiDiFileNellaSuite(t *testing.T) map[string]bool {
	t.Helper()
	radice, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(radice, "go.mod")); err == nil {
			break
		}
		su := filepath.Dir(radice)
		if su == radice {
			t.Fatal("go.mod non trovato")
		}
		radice = su
	}
	reExt := regexp.MustCompile(`^\.[0-9A-Za-z]{1,5}$`)
	nomi := map[string]bool{}
	err = filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var s scanner.Scanner
		fset := token.NewFileSet()
		s.Init(fset.AddFile(p, -1, len(src)), src, nil, 0)
		for {
			_, tok, lit := s.Scan()
			if tok == token.EOF {
				return nil
			}
			if tok != token.STRING {
				continue
			}
			if v, err := strconv.Unquote(lit); err == nil && len(v) > len(path.Ext(v)) && reExt.MatchString(path.Ext(v)) {
				nomi[v] = true
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return nomi
}
