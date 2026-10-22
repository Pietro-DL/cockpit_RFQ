//go:build integrazione && browser

// Browser, frontend del 22/10 [U] (Stefano): la guida interattiva della Distinta unica e le icone «(?)» con il fumetto di
// aiuto (web/static/du_guida.mjs). Le prove G1…G6 di e2e/distinta_unica_guida.py, su un banco finto loro (nuovoBancoDU):
// le icone sugli elementi principali di Sorgente e Revisione, il fumetto al passaggio del mouse e al fuoco con il testo
// del catalogo, Esc che lo chiude; la pagina che non si sposta (scorrimento e posizione degli elementi invariati, ±1 px)
// quando compaiono icone e fumetti; la guida che si apre, avanza, evidenzia elementi esistenti e si chiude, senza
// nessuna richiesta al server; l'interruttore «Mostra aiuti (?)» ricordato dalla scheda.
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaGuidaNelBrowser ./internal/transport/web/
//
// Solo frontend: nessuna rotta, nessun formato nuovo. Codici e nomi dei file inventati (ACM-xxxx).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaGuidaNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica_guida.py"), "--url", b.srv.URL}
	if solo := os.Getenv("COCKPIT_PROVE_GUIDA"); solo != "" {
		arg = append(arg, "--prove", solo)
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
		t.Fatalf("le prove della guida nel browser non sono passate tutte (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
}
