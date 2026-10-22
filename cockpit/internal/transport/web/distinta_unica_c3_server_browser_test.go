//go:build integrazione && browser

// Browser contro il server vero — messaggio 46 [U] (21/10), i rilievi del collaudo visivo UI46, giro 3 (finale): la pagina
// della Distinta unica servita dal banco web VERO (preparaBancoWeb: sessioni, CSRF, rotte vere, calcolo vero del Motore A
// con le regole ACME), in Edge:
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaC3ServerVeroNelBrowser ./internal/transport/web/
//
// La scena è quella del giro 2 (duServerVeroUI46) con un finito in più: 9170001A con lo STEP 9170001A_1.stp, la cui radice
// è 9170099A, diversa dal prodotto (il caso di S8 dell'utente con la radice diversa: il motore non dà una struttura per il
// prodotto, e il nome del file dice 9170001A). Lo script e2e/distinta_unica_c3.py controlla i rilievi del collaudo 3 (ogni
// controllo dice PASSATO o FALLITO):
//   - F3-B1: il pulsante principale della card di uno STEP proposto senza struttura importabile (9130001A, 9170001A) apre
//     il dialogo «solo prodotto», senza motivo, e la conferma riesce;
//   - F3-I1: «Aggiorna proposta…» spento con il perché, finché il server non dà il confronto, e il dialogo «Riparti
//     dalla proposta» non lo suggerisce;
//   - F3-I2: le diagnostiche del motore nel pannello del nodo sotto «Dettagli tecnici», chiuse;
//   - F3-I3: la BOM confermata (working) senza il «?» e il tratteggio del tipo proposto;
//   - F3-I4: la card e l'anteprima dello STEP con la radice diversa dicono la radice letta (9170099A);
//   - F3-I5: l'anteprima e l'organigramma si aprono già adattati al riquadro;
//   - F4-I1 (collaudo finale 4): dalla Revisione senza bozza «Guarda l'anteprima» porta l'anteprima in cima alla finestra,
//     con l'albero in vista o adattato alla parte che si vede, a 1600x1000 e a 1366x768.
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

func TestDistintaUnicaC3ServerVeroNelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s, p3 := b.duServerVeroUI46(t, "DUC3")
	r := s.rfqFascicolo
	p7 := r.componenteTipo("9170001A", "finito")
	allStep7, sha := r.allegatoExt("9170001A_1.stp", "stp")
	r.analisiCorrente(sha, fattiDi("#1", []string{"#1=9170099A", "#2=9170010A", "#3=9170020A"}, []string{"#1>#2*2", "#1>#3*1"}))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '9170001A', '1', 80, 'nome_file')`,
		allStep7, r.thread)

	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "distinta_unica_c3.py")
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
	dati, _ := json.Marshal(map[string]any{"thread": s.thread, "prodotto": s.prodotto, "prodotto2": s.prodotto2, "prodotto3": p3, "prodotto7": p7, "allegati": nomi})
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
	t.Logf("giro C3 nel browser:\n%s", testo)
	for _, riga := range strings.Split(testo, "\n") {
		if strings.Contains(riga, "FALLITO") {
			t.Errorf("%s", strings.TrimSpace(riga))
		}
	}
	if err != nil {
		t.Fatalf("il giro nel browser non è passato: %v", err)
	}
	if !strings.Contains(testo, "giro C3 PASSATO") {
		t.Fatalf("il giro nel browser non dice di essere passato")
	}
}
