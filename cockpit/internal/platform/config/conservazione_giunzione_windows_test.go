//go:build windows

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestConservazioneAttraversoUnaGiunzione (rilievo della revisione; EM §10.5, SIC-16): un deposito che, attraverso una
// giunzione (mount point), sta dentro lo staging si rifiuta come uno scritto per esteso: il confronto usa il percorso
// finale (GetFinalPathNameByHandleW), che segue le giunzioni anche dove filepath.EvalSymlinks non le segue piu'. Un
// deposito accanto, attraverso una giunzione verso una cartella fuori dallo staging, va. Se la giunzione non si puo'
// creare su questa macchina la prova si salta.
func TestConservazioneAttraversoUnaGiunzione(t *testing.T) {
	dir := t.TempDir()
	contenuti := filepath.Join(dir, "staging", "_contenuti")
	fuori := filepath.Join(dir, "altrove")
	for _, d := range []string{contenuti, fuori} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	giunzione := func(link, verso string) {
		t.Helper()
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, verso).CombinedOutput(); err != nil {
			t.Skipf("la giunzione non si crea su questa macchina: %v %s", err, out)
		}
	}
	dentro := filepath.Join(dir, "deposito-dentro")
	giunzione(dentro, contenuti)
	_, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = '"+filepath.Join(dentro, "deposito")+"'\n"))
	if err == nil || !strings.Contains(err.Error(), "[nas].staging") {
		t.Errorf("il deposito che attraverso una giunzione sta nello staging: %v", err)
	}
	accanto := filepath.Join(dir, "deposito-accanto")
	giunzione(accanto, fuori)
	if _, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = '"+filepath.Join(accanto, "deposito")+"'\n")); err != nil {
		t.Errorf("il deposito attraverso una giunzione verso una cartella fuori dallo staging e' rifiutato: %v", err)
	}
	// il percorso finale di una cartella che esiste e' quello vero, senza il prefisso esteso
	if p, err := percorsoFinale(dentro); err != nil || !strings.EqualFold(filepath.Clean(p), filepath.Clean(mustFinale(t, contenuti))) {
		t.Errorf("percorsoFinale(%s) = %q, %v", dentro, p, err)
	}
	for in, atteso := range map[string]string{`\\?\C:\x`: `C:\x`, `\\?\UNC\srv\cond\x`: `\\srv\cond\x`, `C:\y`: `C:\y`} {
		if got := senzaPrefissoEsteso(in); got != atteso {
			t.Errorf("senzaPrefissoEsteso(%q) = %q", in, got)
		}
	}
}

func mustFinale(t *testing.T, p string) string {
	t.Helper()
	f, err := percorsoFinale(p)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
