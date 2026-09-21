// L1 — il log del server non deve crescere senza fine né perdere ciò che è già stato scritto.
package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func leggi(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("lettura di %s: %v", filepath.Base(p), err)
	}
	return string(b)
}

// Un riavvio del server non azzera il log: quasi sempre è proprio il pezzo precedente quello che
// si sta cercando.
func TestRiapriAccodaENonCancella(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log", "cockpit.log")
	s, err := Apri(p, MaxByteDefault, CopieDefault)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("prima del riavvio\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Apri(p, MaxByteDefault, CopieDefault)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := s2.Write([]byte("dopo il riavvio\n")); err != nil {
		t.Fatal(err)
	}
	c := leggi(t, p)
	if !strings.Contains(c, "prima del riavvio") || !strings.Contains(c, "dopo il riavvio") {
		t.Errorf("il riavvio ha perso qualcosa: %q", c)
	}
}

// Oltre la soglia il file scala di uno e se ne apre uno nuovo; le copie in eccesso spariscono.
func TestRuotaETieneSoloLeCopieChieste(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cockpit.log")
	s, err := Apri(p, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	riga := strings.Repeat("x", 60) + "\n"
	for i := 0; i < 6; i++ {
		if _, err := s.Write([]byte(riga)); err != nil {
			t.Fatal(err)
		}
	}
	for _, atteso := range []string{p, p + ".1", p + ".2"} {
		if _, err := os.Stat(atteso); err != nil {
			t.Errorf("manca %s: %v", filepath.Base(atteso), err)
		}
	}
	if _, err := os.Stat(p + ".3"); !os.IsNotExist(err) {
		t.Errorf("la terza copia non doveva essere tenuta (copie = 2)")
	}
	if n := len(leggi(t, p)); n > 100 {
		t.Errorf("il file corrente ha %d byte, oltre la soglia di 100", n)
	}
}

// Una riga più lunga della soglia finisce comunque intera: un log spezzato a metà riga è illeggibile.
func TestUnaRigaPiuLungaDellaSogliaNonSiSpezza(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cockpit.log")
	s, err := Apri(p, 50, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	lunga := strings.Repeat("y", 200) + "\n"
	if _, err := s.Write([]byte("corta\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte(lunga)); err != nil {
		t.Fatal(err)
	}
	if c := leggi(t, p); c != lunga {
		t.Errorf("la riga lunga non è intera nel file corrente: %d byte", len(c))
	}
}

// slog scrive da più goroutine (lo scheduler, l'esecutore, ogni richiesta HTTP): senza lock le righe
// si mescolerebbero fra loro.
func TestScrittureConcorrentiNonSiMescolano(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cockpit.log")
	s, err := Apri(p, MaxByteDefault, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := s.Write([]byte(strings.Repeat("z", 40) + "\n")); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	righe := strings.Split(strings.TrimRight(leggi(t, p), "\n"), "\n")
	if len(righe) != 400 {
		t.Fatalf("attese 400 righe, trovate %d", len(righe))
	}
	for i, r := range righe {
		if len(r) != 40 {
			t.Fatalf("riga %d lunga %d: le scritture si sono mescolate", i, len(r))
		}
	}
}
