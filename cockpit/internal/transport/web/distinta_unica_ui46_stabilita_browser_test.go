//go:build integrazione && browser

// Browser, giro UI46, controllo di stabilità del 21/10 [U] («controlla che l'UI non abbia problemi come "rileggeva i dati
// ogni 5-10 secondi" o problemi che quando si clicca un box o bottone la UI fa dei refresh che spostano la pagina»):
// DU-BX1…DU-BX9 di e2e/distinta_unica.py su un banco finto loro (nuovoBancoDU), una prova per ogni difetto trovato dai
// controllori sull'anteprima 2cedb10. X1: con un file in analisi S1 si rilegge ogni 5 s per 60 s e la Sorgente non si
// ridisegna (elenco e fuoco fermi); X2: la ricerca scritta un tasto ogni 1,2 s non perde tasti; X3: scegliere, spuntare,
// «Togli le spunte» e «+ Aggiungi file» non spostano l'elenco; X4: il pannello del nodo resta dov'è con la stessa scheda,
// la casella, «Confronto», l'OK di «Quantità…» e la chiusura del wizard; X5: chiuso il dialogo aperto trascinando una
// scheda o dal menu del nodo la pagina non salta e il fuoco torna sulla scheda; X6: la barra «N nodi selezionati» non
// sposta la tela; X7: nemmeno la striscia dell'esito; X8: «Guarda l'anteprima» non lascia il fuoco sul body; X9: un
// dialogo chiuso dopo un ridisegno della pagina sotto riporta il fuoco al pulsante che l'ha aperto, ritrovato per id.
// Secondo giro del controllo: X10, dopo «Correggi…» e «Annulla» l'id. cliente del pannello segue la scheda; X11, il clic
// su un'altra scheda non lascia il fuoco su un gesto del nodo nuovo; X12, la pagina della Sorgente non scorre fra 620 e
// 660 px di altezza; X13, il fuoco non finisce sul body (cassetto, «Annulla», le linguette del pannello del nodo,
// «Togli le spunte», <summary>). Seconda rimisura: X14, con la Revisione scorsa fino in fondo «Correggi…» e «Annulla», e
// «Ripeti» dopo un ridimensionamento della finestra, non spostano la pagina né la tela in nessun momento del salvataggio,
// fino all'arrivo di R4 (la barra in fondo non si accorcia).
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaStabilitaNelBrowser ./internal/transport/web/
//
// Le prove creano la loro bozza dallo STEP e aggiungono file in analisi, che la sequenza lunga di
// TestDistintaUnicaNelBrowser non si aspetta. Codici e nomi dei file inventati (ACM-xxxx).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaStabilitaNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	prove := []string{"X1", "X2", "X3", "X4", "X5", "X6", "X7", "X8", "X9", "X10", "X11", "X12", "X13", "X14"}
	if solo := os.Getenv("COCKPIT_PROVE_STABILITA"); solo != "" {
		prove = strings.Split(solo, ",")
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica.py"), "--url", b.srv.URL, "--prove", strings.Join(prove, ",")}
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
		t.Fatalf("le prove di stabilità nel browser non sono passate tutte (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	for _, n := range prove {
		if !strings.Contains(testo, "DU-B"+n+" ") {
			t.Fatalf("DU-B%s non è stata eseguita:\n%s", n, testo)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
}
