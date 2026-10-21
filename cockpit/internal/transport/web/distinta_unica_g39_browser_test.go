//go:build integrazione && browser

// Browser, giro G39 (G39-03 [U], decisioni dell'utente del 20/10 sera), corsia interfaccia: DU-B69 di e2e/distinta_unica.py
// su un banco finto suo (nuovoBancoDU), perché lo spostamento della sorgente cambia le sorgenti dei due prodotti e la
// sequenza lunga di TestDistintaUnicaNelBrowser non se lo aspetta (lo script non la fa girare senza --prove 69):
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaG39NelBrowser ./internal/transport/web/
//
// Dopo lo script il test rilegge il banco: lo spostamento ha revocato la sorgente del secondo prodotto con la causa
// «sorgente errata» e annullato la sua bozza; la correzione della sorgente è dichiarata; la revoca ha annullato la bozza
// nata dalla sorgente revocata; nessuna rotta legacy.

package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaG39NelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica.py"), "--url", b.srv.URL, "--prove", "69"}
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
		t.Fatalf("DU-B69 nel browser è fallita (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	if !strings.Contains(testo, "DU-B69") || !strings.Contains(testo, "ESEGUITA PASSATA") {
		t.Fatalf("DU-B69 non è stata eseguita:\n%s", testo)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
	if s := b.s8[bdP2]; s == nil || s["valida"] != false || s["causa"] != "sorgente_errata" || s["spostato_a"] != bdP1 {
		t.Errorf("la sorgente del secondo prodotto non è revocata dallo spostamento con la causa: %v", s)
	}
	if bz := b.bozze[bdP2]; bz == nil || bz.Stato != "abbandonata" {
		t.Errorf("la bozza del secondo prodotto nata dallo STEP spostato non è annullata: %+v", bz)
	}
	if s := b.s8[bdP1]; s == nil || s["valida"] != false || s["causa"] != "sorgente_errata" || s["correzione"] != true {
		t.Errorf("la sorgente del primo prodotto: correzione dichiarata e poi revocata con la causa, invece %v", s)
	}
	if bz := b.bozze[bdP1]; bz == nil || bz.Stato != "abbandonata" {
		t.Errorf("la bozza del primo prodotto nata dalla sorgente revocata non è annullata: %+v", bz)
	}
}

// DU-B70 (giro G39, parte 2 della corsia interfaccia): G39-01 [U] «motore» e «corretta» dalla pagina, il 422
// proposta_non_corrispondente, l'avviso della bozza manuale con gli STEP candidati di R1, su un banco suo:
//
//	COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaG39ProvenienzaNelBrowser ./internal/transport/web/
//
// Dopo lo script il test rilegge il banco: le provenienze le ha decise il banco come il server, la pagina non le ha scritte.
func TestDistintaUnicaG39ProvenienzaNelBrowser(t *testing.T) {
	b := nuovoBancoDU(t)
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{"-I", filepath.Join("e2e", "distinta_unica.py"), "--url", b.srv.URL, "--prove", "70"}
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
		t.Fatalf("DU-B70 nel browser è fallita (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
	if !strings.Contains(testo, "DU-B70") || !strings.Contains(testo, "ESEGUITA PASSATA") {
		t.Fatalf("DU-B70 non è stata eseguita:\n%s", testo)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.legacy) > 0 {
		t.Errorf("la pagina ha chiamato rotte legacy (MAT §7): %v", b.legacy)
	}
	prov := map[string]string{}
	for _, x := range bdElenco(b.bozze[bdP1].Snapshot["documenti_preassociati"]) {
		d := x.(m)
		prov[bdStr(d["allegato_id"])] = bdStr(d["chiave_bozza"]) + "|" + bdStr(d["provenienza"])
	}
	if prov[bdA2] != "k-7|motore" || prov[bdA3] != "k-11|manuale" {
		t.Errorf("le PB della bozza dopo DU-B70: %v", prov)
	}
}
