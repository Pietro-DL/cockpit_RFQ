//go:build integrazione && browser

// Browser — giro 5, A1d, V1 (direttiva 23), passo 8 di PM23 §14, prima parte: la pagina della Distinta unica di V1
// (distinta_unica.html, distinta_unica.mjs e il ponte distinta_unica_ponte.mjs) in un browser vero, Playwright per
// Python su Edge, come le altre prove *_browser_test.go:
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaNelBrowser ./internal/transport/web/
//
// Niente database: il banco è quello di distinta_unica_banco_test.go, un httptest.Server con il guscio VERO della pagina
// (paginaDistintaUnica, gli statici dell'embed) e risposte FINTE, in memoria, delle rotte di V1 (EM §9 e §10.9). Sono dati
// di prova dichiarati tali: la direttiva 23 §7 lo permette e dice che non sono una prova d'integrazione. Provano la
// pagina e il ponte contro il contratto (i flussi della matrice, gli stati visibili, gli errori 409 e 422, la rete
// interrotta con il recupero dell'intento per R6), non il server: le regole vere le provano le L4 delle altre corsie.
//
// Lo script e2e/distinta_unica.py (python -I) fa le prove DU-B01…DU-B47 in sequenza sulla stessa RFQ del banco (la DU-B34,
// che controlla tutte le richieste della sessione, per ultima): l'elenco e il perché di ognuna stanno nel suo commento di
// testa. Gli esiti che il banco non sa provocare (un 409 o un 422 di un catalogo, un 401, un 413, un 5xx di una lettura) li
// danno risposte finte dello script (page.route), dichiarate tali. COCKPIT_DU_PROVE ne sceglie un sottoinsieme (per
// esempio "01,02,03"); COCKPIT_FOTO_DIR salva le schermate degli errori; COCKPIT_BROWSER_CANALE sceglie il browser.
//
// Dopo lo script il banco dice che cosa è arrivato davvero al «server»: nessuna rotta legacy (MAT §7) e, con tutte le
// prove, le tre conferme registrate una sola volta ciascuna (la risposta persa non duplica la conferma).
//
// Clienti, codici, nomi dei file e ID sono inventati (ACME, BETA, ACM-xxxx): il repository è pubblico.

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "distinta_unica.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	arg := []string{"-I", script, "--url", b.srv.URL}
	prove := os.Getenv("COCKPIT_DU_PROVE")
	if prove != "" {
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
		t.Fatalf("le prove nel browser sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
	if prove == "" {
		// B14, B15 e B16 confermano una volta ciascuna; B13 e B17 non registrano niente
		if n := len(b.conferme[bdP1]); n != 3 {
			t.Errorf("conferme registrate dal banco: %d, attese 3", n)
		}
		if b.conti["R5"] != 3 {
			t.Errorf("R5 registrate dal banco: %d, attese 3 (una risposta persa non duplica la conferma)", b.conti["R5"])
		}
	}
}
