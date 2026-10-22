//go:build integrazione && browser

// Browser, richiesta del frontend del 22/10 (alcuni 2D TIFF non si vedono in anteprima), corsia TIF:
// DU-T01…T05 di e2e/distinta_unica_tif.py sul banco finto (nuovoBancoDU) con le TIF sintetiche di aggiungiTif, Playwright
// su Edge. Le miniature e il visore dei 2D TIFF (G4, LZW), il motivo per una TIF rotta e per una Deflate, la pagina
// ferma, i byte dal ponte:
//
// COCKPIT_DU_FINESTRA=1366x768 cambia la finestra (1440x900 se manca).
//
//	COCKPIT_BROWSER_CANALE=msedge COCKPIT_FOTO_DIR=… go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaTifNelBrowser ./internal/transport/web/
//
// Codici e nomi dei file inventati (ACM-xxxx).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaTifNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	b.aggiungiTif()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica_tif.py"), "--url", b.srv.URL}
	if prove := os.Getenv("COCKPIT_DU_PROVE"); prove != "" {
		arg = append(arg, "--prove", prove)
	}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if finestra := os.Getenv("COCKPIT_DU_FINESTRA"); finestra != "" {
		arg = append(arg, "--finestra", finestra)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	cmd := exec.Command(python, arg...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	uscita, err := cmd.CombinedOutput()
	testo := strings.TrimRight(string(uscita), "\r\n")
	if err != nil {
		if strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("le prove delle TIF nel browser sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	for _, n := range []string{"T01", "T02", "T03", "T04", "T05"} {
		if os.Getenv("COCKPIT_DU_PROVE") == "" && !strings.Contains(testo, "DU-"+n+" ") {
			t.Fatalf("DU-%s non è stata eseguita:\n%s", n, testo)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
}
