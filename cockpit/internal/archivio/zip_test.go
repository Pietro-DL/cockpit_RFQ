package archivio

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func creaZip(t *testing.T, voci map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for nome, contenuto := range voci {
		fw, err := w.Create(nome)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(contenuto))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return p
}

func TestEstrai(t *testing.T) {
	z := creaZip(t, map[string]string{
		"disegni/6674611A_4.pdf":  "pdf",
		"disegni/6674611A.stp":    "step",
		"__MACOSX/._6674611A.stp": "junk",
		"cartella/":               "",
		"../evil.txt":             "slip",
		"sub/dir/Thumbs.db":       "junk",
		"altro/6674611A_4.pdf":    "pdf duplicato in altra cartella",
	})
	dest := filepath.Join(t.TempDir(), "out")
	voci, err := Estrai(z, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(voci) != 3 {
		t.Fatalf("attese 3 voci, trovate %d: %+v", len(voci), voci)
	}
	nomi := map[string]int{}
	for _, v := range voci {
		nomi[v.NomeFile]++
		if v.Sha256 == "" || v.Bytes == 0 {
			t.Errorf("voce senza hash/bytes: %+v", v)
		}
		if _, err := os.Stat(v.Path); err != nil {
			t.Errorf("file non scritto: %s", v.Path)
		}
		if filepath.Dir(v.Path) != dest {
			t.Errorf("voce fuori da destDir: %s", v.Path)
		}
	}
	if nomi["6674611A_4.pdf"] != 2 || nomi["6674611A.stp"] != 1 {
		t.Errorf("nomi inattesi: %v", nomi)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); err == nil {
		t.Error("zip-slip: evil.txt scritto fuori dalla destinazione")
	}
}
