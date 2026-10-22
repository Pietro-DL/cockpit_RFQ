//go:build integrazione && browser

// Browser, messaggio 46 dell'utente (21/10: «Questa UI non è presentabile»), corsia SORGENTE: le prove DU-B71…DU-B74 di
// e2e/distinta_unica.py, ognuna su un banco finto suo (nuovoBancoDU), perché confermano e revocano la sorgente del primo
// prodotto e la sequenza lunga di TestDistintaUnicaNelBrowser non se lo aspetta (lo script non le fa girare senza --prove):
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaUI46Sorgente ./internal/transport/web/
//
// Una prova per gruppo di difetti visti a schermo (l'elenco D01…D47 dei fotogrammi del video dell'utente):
//   - 71: l'anteprima dello STEP con la candidata «incerta» in vista da sola, il dialogo di conferma che dice che cosa si
//     conferma, l'attesa nel dialogo, l'esito una volta sola (D08, D09, D17, D23, D24, D26, D31, D32, D33, D34);
//   - 72: uno stato solo per lo STEP sorgente dopo la conferma e dopo la revoca (D10, D11, D28, D46, D47);
//   - 73: la barra dei gesti sulle selezioni (D18, D19, D20);
//   - 74: il prodotto scelto dalle linguette, l'identificativo del cliente, il colore della bozza (D12, D21, D22, D35);
//   - 75: i rilievi della revisione della corsia: «null» nell'intestazione di un prodotto senza sorgente (R1), la versione
//     del progetto nel dettaglio di uno STEP candidato (R2), la sorgente superata (R5), lo stato della sorgente due volte
//     nella riga e il separatore in «Destinazioni» (R6).
// COCKPIT_FOTO_DIR salva le foto UI46_B7n_*.png (prima dei controlli: su 342010f mostrano il difetto).
//
// Codici e nomi dei file sono quelli inventati del banco (ACME, ACM-xxxx).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaUI46Sorgente(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	for _, n := range []string{"71", "72", "73", "74", "75"} {
		t.Run("DU-B"+n, func(t *testing.T) {
			b := nuovoBancoDU(t)
			arg := []string{"-I", filepath.Join("e2e", "distinta_unica.py"), "--url", b.srv.URL, "--prove", n}
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
				t.Fatalf("DU-B%s nel browser è fallita (%v):\n%s", n, err, testo)
			}
			t.Logf("prove nel browser:\n%s", testo)
			if !strings.Contains(testo, "DU-B"+n) || !strings.Contains(testo, "ESEGUITA PASSATA") {
				t.Fatalf("DU-B%s non è stata eseguita:\n%s", n, testo)
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			if len(b.legacy) > 0 {
				t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
			}
		})
	}
}
