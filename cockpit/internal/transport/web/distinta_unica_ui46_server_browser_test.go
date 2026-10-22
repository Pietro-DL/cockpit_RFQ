//go:build integrazione && browser

// Browser contro il server vero — messaggio 46 [U] (21/10), i rilievi del collaudo visivo UI46, giro 1: la pagina della
// Distinta unica servita dal banco web VERO (preparaBancoWeb: sessioni, CSRF, rotte vere, calcolo vero del Motore A con le
// regole ACME), in Edge:
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaUI46ServerVeroNelBrowser ./internal/transport/web/
//
// Il banco finto inventava lo stato di partenza dell'anteprima e non conosceva la proposta aggiornata dopo la conferma:
// per questo le prove nel browser passavano mentre il collaudo sul server vero vedeva i difetti. La scena è quella del
// collaudo, con tre prodotti finiti: 9120001A con lo STEP 9120001.stp (la radice 9120001A, 9120010A ×2 → 9120020A ×4,
// 9120040A ×2: candidatura univoca; la tavola 9120010A_1.pdf), 9130001A con lo STEP 9130001.stp (la radice 9130001 senza marcatore, nessun figlio:
// proposto come sorgente senza un gruppo importabile), 9140001A senza STEP. Lo script e2e/distinta_unica_ui46.py fa il
// giro della riunione con i soli gesti della pagina e controlla, uno per uno (ogni controllo dice PASSATO o FALLITO):
//   - B1: l'anteprima dello STEP (con la miniatura del 2D sulla scheda di 9120010A) e il dialogo di S8 mostrano l'albero
//     proposto;
//   - I2: il pannello del nodo senza «[object Object]»;
//   - I1: dopo «Cambia tipo» la pillola della Revisione e la barra dicono la stessa versione salvata;
//   - I4: il riepilogo di R4 senza codici, hash né «non e'»;
//   - B3: dopo «Conferma BOM» il wizard dice solo l'esito; la Sorgente non dice «bozza non creata», «Riprendi», «Diventa
//     lo STEP della BOM»; l'ultima mossa è la conferma;
//   - B2: nessuna «Proposta aggiornata» subito dopo la conferma;
//   - I5: la striscia di un prodotto non resta sull'altro;
//   - I3: lo STEP proposto senza gruppo non è «nessuno STEP», «0 STEP», ed è fra gli STEP della RFQ;
//   - I6: «Parti dalla BOM registrata» non c'è senza una BOM registrata;
//   - I7: niente versioni del progetto né «UUID» e codici del server nei testi in vista.
// Poi il database dice che la conferma n. 1 c'è.
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
	"time"

	"github.com/google/uuid"
)

func (b *bancoWeb) duServerVeroUI46(t *testing.T, chiave string) (*duServerVero, uuid.UUID) {
	t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, chiave)
	s := &duServerVero{rfqFascicolo: r, b: b}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("9120001A", "finito")
	s.prodotto2 = r.componenteTipo("9130001A", "finito")
	p3 := r.componenteTipo("9140001A", "finito")
	var sha string
	s.allStep, sha = r.allegatoExt("9120001.stp", "stp")
	r.analisiCorrente(sha, fattiDi("#1", []string{"#1=9120001A", "#2=9120010A", "#3=9120020A", "#4=9120040A"}, []string{"#1>#2*2", "#2>#3*4", "#1>#4*2"}))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '9120001', 80, 'nome_file')`,
		s.allStep, r.thread)
	s.allStep2, s.shaStep2 = r.allegatoExt("9130001.stp", "stp")
	r.analisiCorrente(s.shaStep2, fattiDi("#1", []string{"#1=9130001"}, nil))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '9130001', 80, 'nome_file')`,
		s.allStep2, r.thread)
	var percorso string
	if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, s.allStep).Scan(&percorso); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err == nil {
		b.ws.Staging = filepath.Dir(percorso)
	}
	// la tavola del sottoassieme 9120010A, vera nello staging: le miniature dell'anteprima la leggono
	s.allPDF, _ = r.allegatoExt("9120010A_1.pdf", "pdf")
	contenuto := pdfTavola("9120010")
	var pp string
	if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, s.allPDF).Scan(&pp); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pp, contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	r.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, s.allPDF, shaDi(contenuto), len(contenuto))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', '9120010A', 85, 'nome_file')`,
		s.allPDF, r.thread)
	mwAccendi(t, b, mwRegoleACME(t, cliente, "Acme S.p.A.", 20*time.Second))
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	return s, p3
}

// lanciaUI46: lo script del browser del giro (e2e/distinta_unica_ui46.py), con i dati della scena.
func (s *duServerVero) lanciaUI46(t *testing.T, p3 uuid.UUID) (string, error) {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "distinta_unica_ui46.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	nomi := map[string]string{}
	righe, err := s.b.pool.Query(s.b.ctx, `SELECT a.nome_file, a.allegato_id::text FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
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
	dati, _ := json.Marshal(map[string]any{"thread": s.thread, "prodotto": s.prodotto, "prodotto2": s.prodotto2, "prodotto3": p3, "allegati": nomi})
	arg := []string{"-I", script, "--url", s.b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "AD", "--password", "prova-ad", "--dati", string(dati), "--fase", "ui46"}
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
	return testo, err
}

func TestDistintaUnicaUI46ServerVeroNelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s, p3 := b.duServerVeroUI46(t, "DUUI46")
	testo, err := s.lanciaUI46(t, p3)
	t.Logf("giro UI46 nel browser:\n%s", testo)
	for _, riga := range strings.Split(testo, "\n") {
		if strings.Contains(riga, "FALLITO") {
			t.Errorf("%s", strings.TrimSpace(riga))
		}
	}
	if err != nil {
		t.Fatalf("il giro nel browser non è passato: %v", err)
	}
	if !strings.Contains(testo, "giro UI46 PASSATO") {
		t.Fatalf("il giro nel browser non dice di essere passato")
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
		t.Errorf("conferme di 9120001A dopo il giro: %d, attesa una", n)
	}
}
