//go:build integrazione && browser

// Browser, richiesta del 22/10 (frontend): il browser ad albero della Revisione, come quello degli assiemi di SolidWorks
// o di Fusion 360, prima dell'organigramma (distinta_unica.mjs, blocco «browser ad albero»). Lo script e2e/distinta_unica_browser.py
// (python -I) fa le prove DU-BR0…BR6 in sequenza sul banco finto di distinta_unica_banco_test.go (nuovoBancoDU):
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaBrowserAlberoNelBrowser ./internal/transport/web/
//
// COCKPIT_FOTO_DIR salva le schermate. Codici e nomi dei file sono inventati (ACME, ACM-xxxx).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaBrowserAlberoNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica_browser.py"), "--url", b.srv.URL}
	if prove := os.Getenv("COCKPIT_DU_PROVE"); prove != "" {
		arg = append(arg, "--prove", prove)
	}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
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
		t.Fatalf("le prove del browser ad albero sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	if os.Getenv("COCKPIT_DU_PROVE") == "" && !strings.Contains(testo, "DU-BR1 ") {
		t.Fatalf("DU-BR1 non è stata eseguita:\n%s", testo)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
}
