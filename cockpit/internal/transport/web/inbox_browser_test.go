//go:build integrazione && browser

// L7 — l'Inbox a quadranti in un BROWSER VERO (checkpoint 7B.5).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7 ./internal/transport/web/
//
// Serve Playwright per Python (`python -m pip install playwright`) e un browser installato: si usa
// Microsoft Edge (`--canale msedge`), che su questo PC c'è già, così non si scarica niente.
//
// Perché non basta L4. I test `-tags integrazione` chiedono al server un frammento e leggono l'HTML
// che torna: tutti verdi anche il 18/09/2026, quando nel browser le linguette dei quadranti
// restavano quelle di prima mentre le righe cambiavano. Il difetto non stava in una risposta
// sbagliata, stava nel fatto che la pagina ne sostituiva soltanto UN PEZZO: `hx-target="#lista"`
// mentre linguette, filtri e contatori sono fuori da `#lista`. Un difetto che vive fra due risposte
// giuste lo vede solo chi guarda la pagina intera, cioè un browser.
//
// Il banco è lo stesso di tutti gli altri test web: PostgreSQL vero, server vero su una porta vera.
// L'unica finzione è l'IP della postazione (header X-Prova-IP), come negli altri L4.
package web

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

func TestL7InboxQuadrantiNelBrowser(t *testing.T) {
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	b := preparaBancoWeb(t)
	cliente := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	b.unFornitore("Fresature Esempio", db.TipoFornitoreVerniciatore, "fresature-esempio.example", "cataforesi")

	// I tre quadranti, con dentro le direzioni e i tre stati del filtro. Gli oggetti sono in
	// maiuscolo e inventati: il test del browser cerca proprio quelle parole nelle righe.
	b.posta("entrata", "acquisti@acme.example", "CLIENTE ENTRATA UNO richiesta d'offerta", true)
	b.posta("entrata", "acquisti@acme.example", "CLIENTE ENTRATA DUE richiesta d'offerta", false)
	b.posta("uscita", "commerciale@azienda.example", "CLIENTE USCITA nostra offerta", false, "acquisti@acme.example")
	agganciato := b.posta("entrata", "acquisti@acme.example", "CLIENTE AGGANCIATO in una RFQ", false)
	ignorato := b.posta("entrata", "acquisti@acme.example", "CLIENTE IGNORATO messo da parte", false)
	b.posta("entrata", "info@fresature-esempio.example", "FORNITORE ENTRATA offerta con prezzo", true)
	b.posta("uscita", "commerciale@azienda.example", "FORNITORE USCITA nostra richiesta", false, "info@fresature-esempio.example")
	b.posta("entrata", "qualcuno@altrove.example", "SCONOSCIUTO UNO chi sarà mai", false)
	b.posta("entrata", "altro@ignoto.example", "SCONOSCIUTO DUE nemmeno lui", false)
	// Prova H: una mail come arriva davvero, con l'avviso di posta esterna, una tabella incollata da
	// Excel (TAB nel testo, <table> nell'HTML) e la firma di Outlook per iOS. Si guarda come si legge.
	tabella := b.posta("entrata", "acquisti@acme.example", "CLIENTE TABELLA quotazione", false)
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET corpo_testo = $2, corpo_html = $3 WHERE messaggio_id = $1`,
		tabella, corpoDiProva, htmlDiProva); err != nil {
		t.Fatal(err)
	}

	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto)
		VALUES ($1, 'outlook', now(), 'RFQ di prova') RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2, aggancio = 'operatore' WHERE messaggio_id = $1`, agganciato, thread); err != nil {
		t.Fatal(err)
	}
	// «ignorato» non e' una colonna: e' una proposta rifiutata (v_inbox). Si mette da parte per la
	// strada vera, il pulsante «Ignora», non con una UPDATE che imiterebbe il risultato.
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	if _, esito := fp.fai(http.MethodPost, "/messaggio/"+ignorato.String()+"/ignora", nil, true); !strings.Contains(esito, "ignorato") {
		t.Fatalf("il messaggio non risulta messo da parte: %s", primi400(esito))
	}

	script := filepath.Join("e2e", "inbox_quadranti.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	arg := []string{script, "--url", b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "FP", "--password", "prova-fp"}
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
}
