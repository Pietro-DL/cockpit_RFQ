package web

// L1 — Giro 5, A1d (BE-08, BE-17; T-A1d-04; RK-03): la copia di passaggio del web (motore_a_passaggio.go) contro quella
// del banco (app/bancoa/passaggio.go, che transport non importa). Due guardie: la copia porta tutti i campi (i byte JSON
// della sorgente e della copia coincidono, con un valore pieno e con il valore zero, come A1c-L1-31 del banco), e i corpi
// delle funzioni sono quelli del banco a meno dei nomi (maIn* per in*, maCopia* per copia*, valutazione per valut): un
// campo aggiunto a una delle due copie la fa cadere.
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
	"path/filepath"
	"reflect"
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
		{"il file", f, maInFile(f)},
		{"il prodotto", p, maInProdotto(p)},
		{"i file", []valutazione.FileConfrontabile{f, f}, maInFiles([]valutazione.FileConfrontabile{f, f})},
		{"i prodotti", []valutazione.ProdottoConfrontabile{p}, maInProdotti([]valutazione.ProdottoConfrontabile{p})},
		{"il file zero", valutazione.FileConfrontabile{}, maInFile(valutazione.FileConfrontabile{})},
		{"il prodotto zero", valutazione.ProdottoConfrontabile{}, maInProdotto(valutazione.ProdottoConfrontabile{})},
		{"nessun file", []valutazione.FileConfrontabile{}, maInFiles(nil)},
	} {
		if a, b := maJSON(t, c.da), maJSON(t, c.in); !bytes.Equal(a, b) {
			t.Errorf("la copia di %s perde o cambia un campo:\n%s\n%s", c.nome, a, b)
		}
	}
	if t.Failed() {
		return // senza un campo, la copia sotto non ha la memoria da confrontare
	}
	// La copia non condivide la memoria della sorgente.
	copia := maInFile(f)
	copia.Vecchio.Destinazione[0] = "cambiata"
	*copia.Vecchio.Componente = uuid.Nil
	*copia.Vecchio.DecisoIl = time.Time{}
	copia.Nuovo.Basi[0] = "cambiata"
	copia.Nuovo.Candidati[0].Radici[0] = "cambiata"
	q := maInProdotto(p)
	*q.Quantita = -1
	if f.Vecchio.Destinazione[0] == "cambiata" || *f.Vecchio.Componente == uuid.Nil || f.Vecchio.DecisoIl.IsZero() ||
		f.Nuovo.Basi[0] == "cambiata" || f.Nuovo.Candidati[0].Radici[0] == "cambiata" || *p.Quantita == -1 {
		t.Error("la copia condivide slice o puntatori con la sorgente")
	}
	// Due copie dello stesso esito danno la stessa impronta di confronto.
	ff := []valutazione.FileConfrontabile{f, f}
	ff[1].AllegatoID = maUUID(4095)
	pp := []valutazione.ProdottoConfrontabile{p}
	e1 := confronto.Confronta(maInFiles(ff), maInProdotti(pp), nil)
	e2 := confronto.Confronta(maInFiles(ff), maInProdotti(pp), nil)
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

// maNomeDelBanco: il nome di una funzione del web nel banco (maInFile → inFile, maCopiaUUID → copiaUUID), e il
// pacchetto di valutazione con il nome che ha nel banco (valut).
func maNomeDelBanco(nome string) string {
	switch {
	case strings.HasPrefix(nome, "maIn"), strings.HasPrefix(nome, "maCopia"):
		r := strings.TrimPrefix(nome, "ma")
		return strings.ToLower(r[:1]) + r[1:]
	case nome == "valutazione":
		return "valut"
	}
	return nome
}

// TestMotoreAPassaggioUgualeAlBanco (RK-03): le funzioni di motore_a_passaggio.go sono quelle di app/bancoa/passaggio.go
// (inFile, inFiles, inVecchio, inNuovo, inCandidato, inProdotti, inProdotto) e di app/bancoa/banco.go (copiaUUID,
// copiaTempo, copiaStringhe), nessuna di più e nessuna di meno, con la firma e il corpo stampati uguali a meno dei nomi.
// I sorgenti si leggono dal disco: una mutazione si prova su una copia dei file, non con -overlay.
func TestMotoreAPassaggioUgualeAlBanco(t *testing.T) {
	fset := token.NewFileSet()
	web := maFunzioni(t, fset, "motore_a_passaggio.go")
	passaggio := maFunzioni(t, fset, filepath.Join("..", "..", "app", "bancoa", "passaggio.go"))
	banco := maFunzioni(t, fset, filepath.Join("..", "..", "app", "bancoa", "banco.go"))

	attese := map[string]*ast.FuncDecl{}
	for nome, fd := range passaggio {
		attese[nome] = fd
	}
	for _, nome := range []string{"copiaUUID", "copiaTempo", "copiaStringhe"} {
		if banco[nome] == nil {
			t.Fatalf("%s non è più in app/bancoa/banco.go", nome)
		}
		attese[nome] = banco[nome]
	}
	var nomi []string
	for nome := range web {
		nomi = append(nomi, nome)
	}
	sort.Strings(nomi)
	for _, nome := range nomi {
		fd := web[nome]
		nb := maNomeDelBanco(nome)
		orig, ok := attese[nb]
		if !ok || nb == nome {
			t.Errorf("motore_a_passaggio.go: %s non ha un gemello nel banco", nome)
			continue
		}
		delete(attese, nb)
		ast.Inspect(fd, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				id.Name = maNomeDelBanco(id.Name)
			}
			return true
		})
		if a, b := maStampa(t, fset, orig), maStampa(t, fset, fd); a != b {
			t.Errorf("%s non è più la copia di %s:\n--- banco\n%s\n--- web\n%s", nome, nb, a, b)
		}
	}
	for nome := range attese {
		t.Errorf("%s del banco non ha la sua copia in motore_a_passaggio.go", nome)
	}
	if len(nomi) != 10 {
		t.Errorf("motore_a_passaggio.go ha %d funzioni, attese 10", len(nomi))
	}
}
