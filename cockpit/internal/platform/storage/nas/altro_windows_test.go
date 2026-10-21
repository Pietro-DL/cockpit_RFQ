//go:build windows

package nas

// L1 solo Windows — V1, ondata C, corsia file (U2 §3.7, passi (a) e (b′)): la rimozione di una copia aperta da un
// altro programma. Senza la condivisione in cancellazione la rinomina fallisce; con la condivisione in cancellazione la
// rinomina riesce ma l'apertura esclusiva no, e il file torna al suo nome. In tutti e due i casi «in attesa / file
// occupato», e il file resta. Cartelle temporanee, contenuti sintetici.

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/google/uuid"
)

// apriCon apre il file come farebbe un altro programma, con la condivisione data.
func apriCon(t *testing.T, p string, condivisione uint32) syscall.Handle {
	t.Helper()
	pp, _ := syscall.UTF16PtrFromString(p)
	h, err := syscall.CreateFile(pp, syscall.GENERIC_READ, condivisione, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("apertura di prova: %v", err)
	}
	return h
}

func TestAltroRimuoviFileOccupato(t *testing.T) {
	dati := []byte("ACME aperto da un altro programma")
	casi := []struct {
		nome         string
		condivisione uint32
	}{
		{"senza la condivisione in cancellazione (a)", syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE},
		{"con la condivisione in cancellazione (b′)", syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE},
	}
	for _, k := range casi {
		t.Run(k.nome, func(t *testing.T) {
			c, p, dst := bancoAltro(t, dati)
			if _, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); err != nil {
				t.Fatal(err)
			}
			h := apriCon(t, dst, k.condivisione)
			e := c.Rimuovi(p, uuid.New(), false)
			syscall.CloseHandle(h)
			if e.Esito != RimozioneInAttesa || e.Motivo != MotivoFileOccupato {
				t.Fatalf("esito %+v", e)
			}
			if got, err := os.ReadFile(dst); err != nil || !bytes.Equal(got, dati) {
				t.Fatalf("il file non e' al suo nome: %v", err)
			}
			if v := vociDi(t, filepath.Dir(dst)); len(v) != 1 {
				t.Errorf("nomi di servizio rimasti: %v", v)
			}
			// chiuso il file, il nuovo tentativo lo toglie
			if e := c.Rimuovi(p, uuid.New(), false); e.Motivo != MotivoRimosso {
				t.Errorf("dopo la chiusura: %+v", e)
			}
		})
	}
}

func TestAltroNomeDiServizioNascosto(t *testing.T) {
	f := filepath.Join(t.TempDir(), NomeServizio("x.pdf", uuid.New()))
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nascondi(f)
	pp, _ := syscall.UTF16PtrFromString(f)
	attr, err := syscall.GetFileAttributes(pp)
	if err != nil || attr&syscall.FILE_ATTRIBUTE_HIDDEN == 0 {
		t.Errorf("il nome di servizio non e' nascosto: %x %v", attr, err)
	}
}

func TestAltroRinominaSenzaSostituire(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	_ = os.WriteFile(a, []byte("a"), 0o644)
	_ = os.WriteFile(b, []byte("b"), 0o644)
	if err := rinominaSenzaSostituire(a, b); err == nil || !os.IsExist(err) {
		t.Fatalf("la rinomina su un nome esistente: %v", err)
	}
	if got, _ := os.ReadFile(b); string(got) != "b" {
		t.Errorf("la destinazione e' stata sostituita: %q", got)
	}
	_ = os.Remove(b)
	if err := rinominaSenzaSostituire(a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Error("il nome vecchio resta")
	}
}
