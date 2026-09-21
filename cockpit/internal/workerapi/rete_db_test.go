//go:build integrazione

// L4 (+L2) — blocco 2: credenziali individuali (voce 2.4, W4) e TLS con impronta (W11).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/workerapi/
//
// Fino al blocco 1 l'API dei worker aveva un token solo, uguale per tutti e scritto in chiaro in ogni
// worker.toml. Autenticava «un worker», non «questo worker»: chi lo aveva letto poteva presentarsi
// come il worker di un altro PC e farsi assegnare i suoi job interattivi e la sua posta. E viaggiava
// su HTTP, cioè leggibile da chiunque fosse sullo stesso switch.
//
// W11 fa girare il CLIENT VERO (Python) contro un server TLS vero, perché l'unica cosa che conta
// davvero è se quel client rifiuta un certificato che non è quello dichiarato. Un test che lo
// verificasse in Go proverebbe la libreria di Go, non il worker.
package workerapi

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/rete"
)

// ---------------------------------------------------------------------------------------------
// W4 — senza credenziale non si entra, e una credenziale è di UN worker.
// ---------------------------------------------------------------------------------------------

func TestW4SenzaCredenzialeNonSiEntra(t *testing.T) {
	b := preparaBanco(t, 0)
	b.censisci("outlook@PC-A", db.WorkerTipoOutlook)

	casi := []struct {
		nome   string
		header string
		valore string
		stato  int
		dice   string
	}{
		{"nessun header", "", "", 401, "credenziale mancante"},
		{"header vuoto", "X-Cockpit-Token", "   ", 401, "credenziale mancante"},
		{"token sconosciuto", "X-Cockpit-Token", "un-token-che-nessuno-ha-mai-censito", 401, "non riconosciuta"},
		{"token giusto", "X-Cockpit-Token", tokenDi("outlook@PC-A"), 200, ""},
	}
	for _, c := range casi {
		r, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/api/v1/sync/cursori", nil)
		if c.header != "" {
			r.Header.Set(c.header, c.valore)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		var corpo bytes.Buffer
		_, _ = corpo.ReadFrom(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.stato {
			t.Errorf("%s: %d, atteso %d (%s)", c.nome, resp.StatusCode, c.stato, corpo.String())
			continue
		}
		if c.dice != "" && !strings.Contains(corpo.String(), c.dice) {
			t.Errorf("%s: il motivo non dice %q: %s", c.nome, c.dice, corpo.String())
		}
	}
}

func TestW4UnTokenDiDueWorkerNonIdentificaNessuno(t *testing.T) {
	// È la situazione da cui si arriva: lo stesso token condiviso copiato nella riga di ogni worker.
	// Assegnare l'identità al primo in ordine alfabetico funzionerebbe quasi sempre, e sbaglierebbe
	// senza dirlo: il worker di analisi si prenderebbe i job interattivi di Outlook.
	b := preparaBanco(t, 0)
	condiviso := "lo-stesso-token-per-tutti"
	for _, w := range []struct {
		nome string
		tipo db.WorkerTipo
	}{{"outlook@PC-A", db.WorkerTipoOutlook}, {"analisi@PC-A", db.WorkerTipoAnalisi}} {
		if _, err := b.q.UpsertWorkerCredenziale(b.ctx, db.UpsertWorkerCredenzialeParams{
			WorkerNome: w.nome, WorkerTipo: w.tipo, TokenHash: rete.ImprontaToken(condiviso), Caselle: []uuid.UUID{},
		}); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/api/v1/sync/cursori", nil)
	r.Header.Set("X-Cockpit-Token", condiviso)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	var corpo bytes.Buffer
	_, _ = corpo.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("un token di due worker è stato accettato: %d", resp.StatusCode)
	}
	for _, nome := range []string{"outlook@PC-A", "analisi@PC-A"} {
		if !strings.Contains(corpo.String(), nome) {
			t.Errorf("il motivo non nomina %s: %s", nome, corpo.String())
		}
	}
}

// ---------------------------------------------------------------------------------------------
// W11 — TLS e impronta, con il client vero.
// ---------------------------------------------------------------------------------------------

// scriptImpronta chiede a cockpit_client di fare una chiamata e stampa una sola parola: com'è andata.
const scriptImpronta = `
import sys
sys.path.insert(0, sys.argv[1])
from cockpit_client import Cockpit, ErroreHTTP, ImprontaSbagliata
try:
    api = Cockpit(sys.argv[2], "token-inutile", impronta=sys.argv[3])
    api.chiama("GET", "/api/v1/sync/cursori", timeout=10)
    print("PASSATA")
except ErroreHTTP as e:
    # il TLS ha retto: il server ha risposto (401, perché il token non è censito)
    print("TLS-OK-HTTP-%d" % e.stato)
except ImprontaSbagliata as e:
    print("IMPRONTA-RIFIUTATA")
except Exception as e:
    print("ALTRO: %s: %s" % (type(e).__name__, e))
`

func python(t *testing.T) string {
	t.Helper()
	py, err := exec.LookPath("python")
	if err != nil {
		// Non si salta: il worker È Python. Su una macchina senza interprete questa riga dice che
		// cosa manca, invece di far passare per verde una prova che non è stata fatta.
		t.Fatalf("python non è nel PATH: W11 fa girare il client vero: %v", err)
	}
	return py
}

func eseguiScript(t *testing.T, py, url, impronta string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "workers"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(py, "-c", scriptImpronta, dir, url, impronta)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	return strings.TrimSpace(out.String())
}

func TestW11IlWorkerParlaSoloConIlCertificatoDichiarato(t *testing.T) {
	b := preparaBanco(t, 0)
	py := python(t)

	// Un certificato vero, generato come lo genera il server al primo avvio.
	dir := t.TempDir()
	m, err := rete.Prepara(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"),
		[]string{"localhost", "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Generato || len(m.Impronta) != 64 {
		t.Fatalf("materiale inatteso: generato=%v impronta=%q", m.Generato, m.Impronta)
	}

	mux := http.NewServeMux()
	b.s.Registra(mux)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{m.Certificato}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	url := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	// 1. con l'impronta giusta il collegamento si stabilisce e il server risponde (401: il token di
	//    quello script non è censito, ed è esattamente ciò che dimostra che il TLS ha retto).
	if esito := eseguiScript(t, py, url, m.Impronta); esito != "TLS-OK-HTTP-401" {
		t.Errorf("impronta giusta: %q (atteso TLS-OK-HTTP-401)", esito)
	}

	// 2. con un'impronta diversa il worker si ferma PRIMA di mandare il token. È il caso in cui
	//    dall'altra parte c'è qualcun altro, e mandare il segreto sarebbe l'unico errore irreparabile.
	falsa := strings.Repeat("ab", 32)
	if esito := eseguiScript(t, py, url, falsa); esito != "IMPRONTA-RIFIUTATA" {
		t.Errorf("impronta sbagliata: %q (atteso IMPRONTA-RIFIUTATA)", esito)
	}

	// 3. la stessa impronta scritta com'è comoda copiarla — maiuscole e due punti — deve valere.
	//    Un controllo che si rompe sul formato è un controllo che qualcuno toglie.
	var conDuePunti strings.Builder
	for i := 0; i < len(m.Impronta); i += 2 {
		if i > 0 {
			conDuePunti.WriteString(":")
		}
		conDuePunti.WriteString(strings.ToUpper(m.Impronta[i : i+2]))
	}
	if esito := eseguiScript(t, py, url, conDuePunti.String()); esito != "TLS-OK-HTTP-401" {
		t.Errorf("impronta in maiuscolo con i due punti: %q", esito)
	}

	// 4. su un listener TLS una richiesta in chiaro non arriva a nessun gestore. Non è che «la porta
	//    non risponde»: Go chiude la conversazione con un 400 che dice «hai parlato HTTP a un server
	//    HTTPS». Ciò che conta è che l'API non abbia risposto — nessun 401, nessun 200, nessun corpo
	//    JSON — perché un worker mal configurato manderebbe il proprio token in chiaro alla prima
	//    chiamata, e quel token resterebbe leggibile sulla rete per sempre.
	inChiaro := &http.Client{Timeout: 5 * time.Second}
	resp, err := inChiaro.Get("http://" + strings.TrimPrefix(url, "https://") + "/api/v1/sync/cursori")
	if err == nil {
		var corpo bytes.Buffer
		_, _ = corpo.ReadFrom(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(corpo.String(), "HTTPS") {
			t.Errorf("il listener TLS ha servito una richiesta in chiaro: %d %s", resp.StatusCode, corpo.String())
		}
	}
}

func TestUnImprontaSenzaHttpsNonSiAccetta(t *testing.T) {
	// L1 sul client: dichiarare un'impronta e parlare in chiaro è la configurazione peggiore di
	// tutte, perché sembra sicura. Il worker non parte.
	py := python(t)
	esito := eseguiScript(t, py, "http://127.0.0.1:1", strings.Repeat("ab", 32))
	if !strings.HasPrefix(esito, "ALTRO: ValueError") {
		t.Errorf("impronta su http: %q (atteso un ValueError all'avvio)", esito)
	}
}
