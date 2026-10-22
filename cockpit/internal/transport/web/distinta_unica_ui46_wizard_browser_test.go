//go:build integrazione && browser

// Browser, giro UI46 (messaggio 48 [U]), corsia wizard: DU-BW1 di e2e/distinta_unica.py su un banco finto suo
// (nuovoBancoDU). «Ci sono dei flickering nel wizard R5 che fanno tornare su la pagina mentre la scrollo» [U]: il wizard
// «Rivedi e conferma BOM» aperto e scorso a metà non torna in cima né si ridisegna con la rilettura in sottofondo
// (R1 ?ingressi=1); una notifica nuova e la presa d'atto tengono scorrimento e fuoco; le preassociazioni mostrano il codice
// e la descrizione del pezzo, mai la chiave del nodo; nessun UUID, null o apostrofo al posto di un accento nel wizard.
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaWizardScorrimentoNelBrowser ./internal/transport/web/
//
// Lo script non fa girare DU-BW1 senza --prove W1: la prova crea la sua bozza dallo STEP, che la sequenza lunga di
// TestDistintaUnicaNelBrowser non si aspetta. Codici e nomi dei file inventati (ACM-xxxx).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaWizardScorrimentoNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica.py"), "--url", b.srv.URL, "--prove", "W1"}
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
		t.Fatalf("DU-BW1 nel browser è fallita (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	if !strings.Contains(testo, "DU-BW1") || !strings.Contains(testo, "ESEGUITA PASSATA") {
		t.Fatalf("DU-BW1 non è stata eseguita:\n%s", testo)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
}
