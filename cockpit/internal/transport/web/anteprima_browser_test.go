//go:build integrazione && browser

// L7 — l'anteprima PDF in un BROWSER VERO (blocco 8, B8.1).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7Anteprima ./internal/transport/web/
//
// Serve Playwright per Python (`python -m pip install playwright`) e un browser installato: si usa
// Microsoft Edge (`--canale msedge`), lo stesso di `TestL7InboxQuadrantiNelBrowser`.
//
// Perche' non basta L4. Le prove L4 di `anteprima_db_test.go` chiedono al server e guardano la
// risposta: passerebbero tutte anche se il pulsante «Anteprima» non comparisse in nessuna pagina, o
// comparisse con l'indirizzo sbagliato, o aprisse la stessa scheda facendo perdere la RFQ aperta. E
// il `Range` passerebbe anche se fosse il client di Go a chiederlo e nessun browser vero. Qui il
// pulsante si cerca nella riga dell'allegato, si clicca, e i pezzi li chiede la rete del browser.
//
// Il banco e' lo stesso di L4 (`preparaAnteprima`): PostgreSQL vero, un NAS finto su disco con dentro
// il file, e il documento confermato che lo nomina. L'unica finzione e' l'IP della postazione
// (header X-Prova-IP), come in tutti gli altri test web.
package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestL7AnteprimaPdfNelBrowser(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	// Due megabyte: abbastanza perche' un Range da 512 KB sia un pezzo e non tutto il file.
	s := preparaAnteprima(t, pdfFinto(2*1024*1024), false, true)

	script := filepath.Join("e2e", "anteprima_pdf.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	arg := []string{script, "--url", s.b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "FP",
		"--password", "prova-fp", "--thread", s.thread.String(), "--allegato", s.allegato.String(),
		"--byte", strconv.Itoa(len(s.pdf))}
	if solo := os.Getenv("COCKPIT_BROWSER_SOLO"); solo != "" {
		arg = append(arg, "--solo", solo)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	cmd := exec.Command(python, arg...)
	uscita, err := cmd.CombinedOutput()
	testo := strings.TrimRight(string(uscita), "\r\n")
	if err != nil {
		if strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("le prove nel browser sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)

	// Quello che il browser ha fatto si vede anche da questa parte: il file e' stato letto a pezzi, e
	// nessuno di quei pezzi ha fatto ricalcolare l'hash. Se lo avesse fatto, ci sarebbe una riga di
	// log con byte_letti oltre i due megabyte.
	if _, letti := s.reg.servita(t); letti > int64(len(s.pdf)) {
		t.Errorf("l'ultima richiesta del browser ha letto %d byte su un file di %d: qualcosa ha riletto tutto", letti, len(s.pdf))
	}
	if a, c := s.anomalia(t); c {
		t.Errorf("guardare un file che corrisponde ha prodotto una segnalazione (%s)", a.Problema)
	}
}
