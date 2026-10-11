package motore_a

// L1 — giro 5, A1d-Q2: la prova gemella di A1c-L1-31 (app/bancoa/passaggio_test.go) per la copia di passaggio del
// sottopackage (passaggio.go) contro quella del banco (app/bancoa/passaggio.go, con gli aiuti del banco che chiama), che
// transport non importa. Derivata dalle prove della PR #18 di Stefano Valo (67dcc02: motore_a_passaggio_test.go). Due
// guardie: la copia porta tutti i campi (i byte JSON della sorgente e della copia coincidono, con un valore pieno e con il
// valore zero, come A1c-L1-31), e i corpi delle funzioni sono quelli del banco, byte per byte (nomi compresi: la copia
// del sottopackage usa gli stessi nomi e lo stesso alias valut): un campo aggiunto a una delle due copie la fa cadere. I
// sorgenti del banco si leggono dal disco, senza importarli: nessuna freccia transport → app, nemmeno nei test.
//
// I clienti di questi test sono inventati: i valori sono «valore-n», UUID ACME e istanti del 2026.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/valutazione"
)

// maRiempi: un valore con ogni campo non zero (come riempi del banco): stringhe, booleani, interi, UUID, istanti in UTC
// al millisecondo, puntatori non nil, slice di due elementi.
func maRiempi(v reflect.Value, n *int) {
	*n++
	switch v.Type() {
	case reflect.TypeOf(uuid.UUID{}):
		v.Set(reflect.ValueOf(maUUID(*n)))
		return
	case reflect.TypeOf(time.Time{}):
		v.Set(reflect.ValueOf(time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC).Add(time.Duration(*n) * time.Millisecond)))
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(fmt.Sprintf("valore-%d", *n))
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16:
		v.SetInt(int64(*n))
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		maRiempi(p.Elem(), n)
		v.Set(p)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 2, 2)
		for i := 0; i < 2; i++ {
			maRiempi(s.Index(i), n)
		}
		v.Set(s)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			maRiempi(v.Field(i), n)
		}
	}
}

func maJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestMotoreAPassaggioPortaTuttiICampi (M-A1d-19): un VecchioPiatto, un NuovoPiatto con i candidati e un
// ProdottoConfrontabile con la quantità, tutti i campi pieni, e il valore zero: i byte JSON della sorgente e della copia
// coincidono; la copia non condivide la memoria; due copie dello stesso esito danno la stessa impronta di confronto.
func TestMotoreAPassaggioPortaTuttiICampi(t *testing.T) {
	var f valutazione.FileConfrontabile
	var p valutazione.ProdottoConfrontabile
	n := 0
	maRiempi(reflect.ValueOf(&f).Elem(), &n)
	maRiempi(reflect.ValueOf(&p).Elem(), &n)
	if f.Vecchio.Destinazione == nil || f.Nuovo.Candidati == nil || p.Quantita == nil {
		t.Fatal("il valore pieno non è pieno")
	}
	for _, c := range []struct {
		nome   string
		da, in any
	}{
		{"il file", f, inFile(f)},
		{"il prodotto", p, inProdotto(p)},
		{"i file", []valutazione.FileConfrontabile{f, f}, inFiles([]valutazione.FileConfrontabile{f, f})},
		{"i prodotti", []valutazione.ProdottoConfrontabile{p}, inProdotti([]valutazione.ProdottoConfrontabile{p})},
		{"il file zero", valutazione.FileConfrontabile{}, inFile(valutazione.FileConfrontabile{})},
		{"il prodotto zero", valutazione.ProdottoConfrontabile{}, inProdotto(valutazione.ProdottoConfrontabile{})},
		{"nessun file", []valutazione.FileConfrontabile{}, inFiles(nil)},
	} {
		if a, b := maJSON(t, c.da), maJSON(t, c.in); !bytes.Equal(a, b) {
			t.Errorf("la copia di %s perde o cambia un campo:\n%s\n%s", c.nome, a, b)
		}
	}
	if t.Failed() {
		return // senza un campo, la copia sotto non ha la memoria da confrontare
	}
	// La copia non condivide la memoria della sorgente.
	copia := inFile(f)
	copia.Vecchio.Destinazione[0] = "cambiata"
	*copia.Vecchio.Componente = uuid.Nil
	*copia.Vecchio.DecisoIl = time.Time{}
	copia.Nuovo.Basi[0] = "cambiata"
	copia.Nuovo.Candidati[0].Radici[0] = "cambiata"
	q := inProdotto(p)
	*q.Quantita = -1
	if f.Vecchio.Destinazione[0] == "cambiata" || *f.Vecchio.Componente == uuid.Nil || f.Vecchio.DecisoIl.IsZero() ||
		f.Nuovo.Basi[0] == "cambiata" || f.Nuovo.Candidati[0].Radici[0] == "cambiata" || *p.Quantita == -1 {
		t.Error("la copia condivide slice o puntatori con la sorgente")
	}
	// Due copie dello stesso esito danno la stessa impronta di confronto.
	ff := []valutazione.FileConfrontabile{f, f}
	ff[1].AllegatoID = maUUID(4095)
	pp := []valutazione.ProdottoConfrontabile{p}
	e1 := confronto.Confronta(inFiles(ff), inProdotti(pp), nil)
	e2 := confronto.Confronta(inFiles(ff), inProdotti(pp), nil)
	if e1.Impronta != e2.Impronta || len(e1.Impronta) != 64 {
		t.Fatalf("impronte %q e %q", e1.Impronta, e2.Impronta)
	}
}

// maFunzioni: le dichiarazioni di funzione di un file Go, per nome (go/parser).
func maFunzioni(t *testing.T, fset *token.FileSet, percorso string) map[string]*ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(fset, percorso, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			out[fd.Name.Name] = fd
		}
	}
	return out
}

// maStampa: la funzione stampata senza il commento: firma e corpo.
func maStampa(t *testing.T, fset *token.FileSet, fd *ast.FuncDecl) string {
	t.Helper()
	senza := *fd
	senza.Doc = nil
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, &senza); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// maFunzioniDelPackage: le funzioni senza ricevitore dei file non di prova di una cartella, per nome.
func maFunzioniDelPackage(t *testing.T, fset *token.FileSet, cartella string) map[string]*ast.FuncDecl {
	t.Helper()
	voci, err := os.ReadDir(cartella)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*ast.FuncDecl{}
	for _, v := range voci {
		if v.IsDir() || !strings.HasSuffix(v.Name(), ".go") || strings.HasSuffix(v.Name(), "_test.go") {
			continue
		}
		for nome, fd := range maFunzioni(t, fset, filepath.Join(cartella, v.Name())) {
			if fd.Recv == nil {
				out[nome] = fd
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("nessuna funzione in %s: la prova non legge i file giusti", cartella)
	}
	return out
}

// TestMotoreAPassaggioUgualeAlBanco (RK-03): le funzioni di passaggio.go sono quelle di app/bancoa/passaggio.go
// (inFile, inFiles, inVecchio, inNuovo, inCandidato, inProdotti, inProdotto) e gli aiuti del banco che quei corpi
// chiamano, nessuna di più e nessuna di meno, con la firma e il corpo stampati uguali. Gli aiuti non sono scritti a mano
// (IX-08 della revisione di A1d-P2): sono le funzioni del package bancoa che i corpi di passaggio.go chiamano, e quelle
// che chiamano loro, fino alla chiusura (oggi copiaUUID, copiaTempo e copiaStringhe di banco.go). Così un aiuto nuovo
// del banco (per esempio un copiaIntero per la quantità) fa cadere la prova anche se nel sottopackage c'è già un omonimo
// scritto per conto suo (indice.go): l'aiuto deve stare in passaggio.go ed essere la copia di quello del banco. I
// sorgenti si leggono dal disco: una mutazione si prova su una copia dei file, non con -overlay.
func TestMotoreAPassaggioUgualeAlBanco(t *testing.T) {
	fset := token.NewFileSet()
	cartellaBanco := filepath.Join("..", "..", "..", "app", "bancoa")
	web := maFunzioni(t, fset, "passaggio.go")
	passaggio := maFunzioni(t, fset, filepath.Join(cartellaBanco, "passaggio.go"))
	banco := maFunzioniDelPackage(t, fset, cartellaBanco)

	attese := map[string]*ast.FuncDecl{}
	var daGuardare []*ast.FuncDecl
	for nome, fd := range passaggio {
		attese[nome] = fd
		daGuardare = append(daGuardare, fd)
	}
	// la chiusura degli aiuti del banco chiamati dai corpi
	var aiuti []string
	for len(daGuardare) > 0 {
		fd := daGuardare[0]
		daGuardare = daGuardare[1:]
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if def := banco[id.Name]; def != nil && attese[id.Name] == nil {
				attese[id.Name] = def
				aiuti = append(aiuti, id.Name)
				daGuardare = append(daGuardare, def)
			}
			return true
		})
	}
	sort.Strings(aiuti)
	if !slices.Equal(aiuti, []string{"copiaStringhe", "copiaTempo", "copiaUUID"}) {
		t.Logf("gli aiuti del banco chiamati da passaggio.go sono cambiati: %v (la copia va allineata)", aiuti)
	}
	var nomi []string
	for nome := range web {
		nomi = append(nomi, nome)
	}
	sort.Strings(nomi)
	for _, nome := range nomi {
		orig, ok := attese[nome]
		if !ok {
			t.Errorf("passaggio.go: %s non ha un gemello nel banco", nome)
			continue
		}
		delete(attese, nome)
		if a, b := maStampa(t, fset, orig), maStampa(t, fset, web[nome]); a != b {
			t.Errorf("%s non è più la copia del banco:\n--- banco\n%s\n--- motore_a\n%s", nome, a, b)
		}
	}
	for nome := range attese {
		t.Errorf("%s del banco non ha la sua copia in passaggio.go", nome)
	}
	if len(nomi) != 10 {
		t.Errorf("passaggio.go ha %d funzioni, attese 10", len(nomi))
	}
}
