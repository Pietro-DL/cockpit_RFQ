//go:build integrazione && browser

// Browser — messaggio 46 dell'utente (21/10) [U], corsia MOTIVO: DU-M46 di e2e/distinta_unica_motivo46.py su un banco finto
// suo (nuovoBancoDU, dati inventati), Playwright su Edge:
//
//	COCKPIT_BROWSER_CANALE=msedge COCKPIT_FOTO_DIR=… go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaMotivo46NelBrowser ./internal/transport/web/
//
// I dialoghi della Distinta unica che chiedevano un motivo all'utente (la conferma della sorgente, anche con il codice della
// radice diverso; «Correggi codice, rev e descrizione»; «Non includere nella BOM»; «Escludi»; «Fissa l'emittente»;
// «Abbandona la bozza»; la revoca della sorgente; «Scarta»; «Metti in Altri file») non hanno più il campo del motivo né i
// testi che lo chiedono, il gesto parte senza, e i corpi non lo portano. Le foto di ogni dialogo vanno in COCKPIT_FOTO_DIR.
// Sul codice di 342010f la prova FALLISCE (i difetti D01-D04 dell'elenco del messaggio 46). Dopo lo script il test rilegge
// il banco: la revoca e lo scarto sono registrati, nessuna rotta legacy.
//
// Revisione della corsia (rilievi 2 e 3): dopo i gesti il motivo registrato non si mostra (pannello del nodo non incluso,
// wizard, file scartato) e il dialogo della conferma della sorgente dice il codice letto dal file diverso da quello del
// prodotto: per questo il banco fa leggere ad ACM-1000_rev2.stp il codice ACM-1000-R2 (un codice inventato).

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaMotivo46NelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	// il codice letto dal file dello STEP rev 2 (S1 proposta.codice) non è quello del prodotto ACM-1000
	b.mu.Lock()
	b.fileDi(bdA10).Codice = "ACM-1000-R2"
	b.mu.Unlock()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica_motivo46.py"), "--url", b.srv.URL}
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
		t.Fatalf("DU-M46 nel browser è fallita (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	if !strings.Contains(testo, "DU-M46") || !strings.Contains(testo, "ESEGUITA PASSATA") {
		t.Fatalf("DU-M46 non è stata eseguita:\n%s", testo)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
	if s := b.s8[bdP1]; s == nil || s["valida"] != false || s["causa"] != "sorgente_errata" {
		t.Errorf("la revoca senza motivo non è registrata nel banco: %v", s)
	}
	if f := b.fileDi(bdA4); f == nil || f.Stato != "scartato" || !strings.HasPrefix(bdStr(f.Scarto["motivo"]), "gesto di AD da «Scarta»: ") {
		t.Errorf("lo scarto senza motivo non è registrato con la traccia del gesto: %+v", f)
	}
}
