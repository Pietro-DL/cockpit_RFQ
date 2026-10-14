package archivio

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// testdata/voci.7z è un 7z inventato (codici ACME), fatto con il tar.exe di Windows (--format 7zip): una
// cartella con «2D data» e «3D data», come gli archivi dei portali. Il lettore 7z deve dare le stesse voci di
// uno zip: percorso interno con le cartelle, nome, byte e sha256 calcolati sui byte scritti.
func TestEstrai7z(t *testing.T) {
	voci, err := Estrai(filepath.Join("testdata", "voci.7z"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var interni []string
	for _, v := range voci {
		interni = append(interni, v.PathInterno)
		b, err := os.ReadFile(v.Path)
		if err != nil || int64(len(b)) != v.Bytes || v.Sha256 == "" {
			t.Fatalf("voce %s: letta %d byte (dichiarati %d), sha %q, err %v", v.PathInterno, len(b), v.Bytes, v.Sha256, err)
		}
	}
	sort.Strings(interni)
	atteso := []string{"ACME_assieme/2D data/9123456_D42_100000001_000_00.pdf", "ACME_assieme/3D data/9123456_D41_100000002_000_00.stp"}
	if len(interni) != len(atteso) || interni[0] != atteso[0] || interni[1] != atteso[1] {
		t.Fatalf("voci = %v, attese %v", interni, atteso)
	}
}

// Il budget dei byte vale anche per il 7z: si misura su ciò che si scrive, non su ciò che l'archivio dichiara.
func TestEstrai7zBudget(t *testing.T) {
	_, err := EstraiCon(filepath.Join("testdata", "voci.7z"), t.TempDir(), 1000, 10)
	if !errors.Is(err, ErrLimite) {
		t.Fatalf("err = %v, atteso ErrLimite", err)
	}
}

// Il formato si riconosce dai byte: uno zip col nome .7z resta uno zip, un file qualsiasi non è un archivio.
func TestFormato(t *testing.T) {
	if f, err := Formato(filepath.Join("testdata", "voci.7z")); err != nil || f != Formato7z {
		t.Fatalf("7z: %q %v", f, err)
	}
	z := creaZip(t, map[string]string{"a.txt": "a"})
	finto := filepath.Join(t.TempDir(), "in_realta_zip.7z")
	b, _ := os.ReadFile(z)
	if err := os.WriteFile(finto, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if f, err := Formato(finto); err != nil || f != FormatoZip {
		t.Fatalf("zip col nome .7z: %q %v", f, err)
	}
	altro := filepath.Join(t.TempDir(), "x.pdf")
	_ = os.WriteFile(altro, []byte("%PDF-1.7"), 0o644)
	if _, err := Formato(altro); !errors.Is(err, ErrFormato) {
		t.Fatalf("pdf: err = %v, atteso ErrFormato", err)
	}
	for ext, si := range map[string]bool{"zip": true, ".7Z": true, "rar": false, "pdf": false, "": false} {
		if Estraibile(ext) != si {
			t.Fatalf("Estraibile(%q) = %v", ext, !si)
		}
	}
}

// Un dizionario dichiarato oltre il tetto si rifiuta prima di allocare: è il caso dell'archivio di poche
// centinaia di byte che chiede 4 GB al decodificatore.
func TestDizionario7zOltreIlTetto(t *testing.T) {
	flusso := []io.ReadCloser{io.NopCloser(bytes.NewReader(nil))}
	if _, err := lettoreLZMA2([]byte{40}, 0, flusso); !errors.Is(err, ErrDizionario7z) {
		t.Fatalf("lzma2 da 4 GB: err = %v", err)
	}
	if _, err := lettoreLZMA([]byte{0x5d, 0xff, 0xff, 0xff, 0xff}, 0, flusso); !errors.Is(err, ErrDizionario7z) {
		t.Fatalf("lzma da 4 GB: err = %v", err)
	}
	if _, err := lettoreLZMA2([]byte{41}, 0, flusso); err == nil {
		t.Fatal("lzma2 con proprietà fuori scala: atteso un errore")
	}
}
