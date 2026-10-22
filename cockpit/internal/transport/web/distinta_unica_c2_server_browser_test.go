//go:build integrazione && browser

// Browser contro il server vero — messaggio 46 [U] (21/10), i rilievi del collaudo visivo UI46, giro 2: la pagina della
// Distinta unica servita dal banco web VERO (preparaBancoWeb: sessioni, CSRF, rotte vere, calcolo vero del Motore A con le
// regole ACME), in Edge:
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaC2ServerVeroNelBrowser ./internal/transport/web/
//
// La scena è quella del collaudo, giro 2, con quattro prodotti finiti: 9120001A con lo STEP 9120001.stp (la radice
// 9120001A, 9120010A ×2 → 9120020A ×4, 9120040A ×2: candidatura univoca; la tavola 9120010A_1.pdf), 9130001A con lo STEP
// 9130001.stp (la radice 9130001 senza marcatore: proposto senza un gruppo importabile), 9140001A senza STEP e 9150001A
// con lo STEP 9150001A_1.stp, la cui radice 9150001 non porta il marcatore del prodotto: il codice 9150001A lo dice solo il
// nome del file, e la candidatura è «incerta». Lo script e2e/distinta_unica_c2.py controlla i rilievi rimasti dopo
// l'unione del ramo finale (ogni controllo dice PASSATO o FALLITO): l'anteprima dalla Revisione senza bozza (C2-B1), la
// tela dell'anteprima nello spazio visibile (C2-I4), la card dello STEP proposto in un posto solo (C2-I5), il tab che
// porta con sé il corpo (C2-I6), il perché della «incerta» (C2-I7) e il dialogo «Sposta» (minori).
//
// Clienti, codici e nomi dei file sono inventati (ACME, 91xxxxx, acme.example): il repository è pubblico.

package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistintaUnicaC2ServerVeroNelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s, p3 := b.duServerVeroUI46(t, "DUC2")
	r := s.rfqFascicolo
	p4 := r.componenteTipo("9150001A", "finito")
	allStep4, sha := r.allegatoExt("9150001A_1.stp", "stp")
	r.analisiCorrente(sha, fattiDi("#1", []string{"#1=9150001", "#2=9150010A", "#3=9150020A"}, []string{"#1>#2*2", "#1>#3*1"}))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '9150001A', '1', 80, 'nome_file')`,
		allStep4, r.thread)

	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "distinta_unica_c2.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	nomi := map[string]string{}
	righe, err := b.pool.Query(b.ctx, `SELECT a.nome_file, a.allegato_id::text FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.thread_id = $1`, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	for righe.Next() {
		var k, v string
		if err := righe.Scan(&k, &v); err != nil {
			righe.Close()
			t.Fatal(err)
		}
		nomi[k] = v
	}
	righe.Close()
	dati, _ := json.Marshal(map[string]any{"thread": s.thread, "prodotto": s.prodotto, "prodotto2": s.prodotto2, "prodotto3": p3, "prodotto4": p4, "allegati": nomi})
	arg := []string{"-I", script, "--url", b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "AD", "--password", "prova-ad", "--dati", string(dati)}
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
	if err != nil && (strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist")) {
		t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
	}
	t.Logf("giro C2 nel browser:\n%s", testo)
	for _, riga := range strings.Split(testo, "\n") {
		if strings.Contains(riga, "FALLITO") {
			t.Errorf("%s", strings.TrimSpace(riga))
		}
	}
	if err != nil {
		t.Fatalf("il giro nel browser non è passato: %v", err)
	}
	if !strings.Contains(testo, "giro C2 PASSATO") {
		t.Fatalf("il giro nel browser non dice di essere passato")
	}
}
