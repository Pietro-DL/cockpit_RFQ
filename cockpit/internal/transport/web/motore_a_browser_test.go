//go:build integrazione && browser

// Browser — giro 5, A1d-P4, passo B5.1 dopo la direttiva 12 del 13/10: lo Scenario H (§10: «RFQ con almeno due
// prodotti. Dopo una modifica sul secondo prodotto nella vista Documenti, la rilettura deve mantenere prodotto e vista.
// Gli errori e le diagnostiche devono essere visibili»), con i rilievi R-01 e R-02 (§7) e D-18 = A (§9), in un browser
// vero (Playwright su Edge, come le altre prove *_browser_test.go):
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestMotoreAScenarioHNelBrowser ./internal/transport/web/
//
// NON ESEGUITA nella corsia E (nessun database, nessun browser): va eseguita con il DSN verificato prima (deve essere
// cockpit_test) e il semaforo delle L4. Finché non gira, il JavaScript della pagina (motore_a.mjs: l'indirizzo riscritto
// dopo la rilettura, l'avviso con il frammento d'errore) NON è verificato.
//
// La scena è la B87 di mwScena (motore_a_db_test.go: il prodotto finito 77722757 con lo STEP e il 2D, l'assieme, il
// particolare, due PDF in proposta aperta, le regole ACME del cliente, l'amministratore) con un secondo prodotto finito,
// 7120801, inventato. L'ordine dei prodotti lo dà il server: la prova lo legge da /motore-a/dati e passa allo script il
// primo e il secondo, con il numero delle diagnostiche del thread (i due codici non si leggono con la grammatica ACME:
// ancoraggio.target_non_leggibile, avviso). La «sonda» è un indirizzo di questa prova: GET = lo stato delle proposte
// della RFQ; POST /regole-non-pronte e /regole-pronte cambiano le regole del server (il calcolo che non riesce, D-18).
//
// Lo script e2e/motore_a.py entra come amministratore e prova, nell'ordine (H1…H6, sulla stessa RFQ):
//   - H1 l'apertura senza query: nessun errore in console, la radice con data-ma-calcolo, l'indirizzo riscritto con il
//     primo prodotto e la vista distinta (R-01, la sorgente di verità nell'indirizzo), nessuna scrittura;
//   - H2 il secondo prodotto e la vista Documenti dai collegamenti: radice, schede correnti e indirizzo concordi;
//   - H3 «Scarta il file» sul secondo prodotto nella vista Documenti: avviso «ok», #ma riletto senza ricaricare la
//     pagina, prodotto e vista uguali, la sorgente «indirizzo», nessuna striscia di scelta predefinita, la proposta
//     scartata nella sonda. Il gesto vale per tutta la RFQ (il file «Da smistare»): non e' una modifica di un dato del
//     secondo prodotto, perche' i gesti per prodotto sono spenti (R1-R14); quando B5.3/B5.4 li accendono, vanno qui
//     (revisione D-E del 13/10, rilievo 11);
//   - H4 «Aggiorna»: lo stesso;
//   - H5 le diagnostiche del thread visibili, tante quante dice /dati; con l'esito incompleto nessun badge di verifica
//     positivo e la striscia #ma-incompleto visibile, con l'esito completo nessuna striscia (R-02);
//   - H6 il calcolo che non riesce (regole non pronte) e «Aggiorna»: frammento 200 con data-ma-calcolo="errore" e
//     data-ma-errore="regole_non_pronte", la striscia dell'errore visibile, #ma-avviso con data-esito="errore" e la frase
//     della pagina senza esito, l'indirizzo intatto; regole di nuovo pronte e «Aggiorna»: di nuovo il secondo prodotto
//     nella vista Documenti, e l'avviso senza la frase della pagina senza esito e senza data-esito="errore" (D-18;
//     revisione D-E del 13/10, rilievo 4).
//
// Clienti, codici, nomi dei file e ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// maScenaH: la scena B87 con il secondo prodotto, la sonda e i dati che lo script vuole.
type maScenaH struct {
	*mwScena
	b               *bancoWeb
	secondoProdotto uuid.UUID
	primo, secondo  string // i Rif dei due prodotti, nell'ordine del server
	diagnostiche    int    // le diagnostiche del thread nell'esito
	sonda           *httptest.Server
}

// maSondaH: GET = lo stato di ogni proposta della RFQ (nome del file → stato) e il numero dei job; POST /regole-non-pronte
// = regole accese ma non pronte (il calcolo della pagina non parte: 200 con l'errore, D-18); POST /regole-pronte = le
// regole ACME della scena.
func (s *maScenaH) maSondaH(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/regole-non-pronte":
		s.b.ws.MotoreA = motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente}
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/regole-pronte":
		s.b.ws.MotoreA = s.regole
		w.WriteHeader(http.StatusNoContent)
		return
	}
	righe, err := s.b.pool.Query(r.Context(), `SELECT a.nome_file, p.stato::text FROM documento_proposta p
		JOIN allegato a ON a.allegato_id = p.allegato_id WHERE p.thread_id = $1`, s.thread)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer righe.Close()
	proposte := map[string]string{}
	for righe.Next() {
		var k, v string
		if err := righe.Scan(&k, &v); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		proposte[k] = v
	}
	var job int
	if err := s.b.pool.QueryRow(r.Context(), `SELECT count(*) FROM job`).Scan(&job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"proposte": proposte, "job": job})
}

// pdfVeriNelloStaging: le miniature della pagina chiedono /allegato/{id}/anteprima?v=<sha> per ogni PDF della RFQ, e lo
// script conta come errore ogni risposta >= 400. Nella scena B87 i PDF sono contenuti finti e la radice dello staging
// non è dichiarata: il server risponde 404 (la pagina lo mostra bene, «anteprima non disponibile», ma lo Scenario H vuole
// zero errori). Come le altre prove *_browser_test.go (richieste, fascicolo): la radice dello staging è la cartella
// della scena e al posto di ogni PDF della RFQ c'è una tavola vera (pdfTavola), con impronta e dimensione aggiornate
// nell'allegato e nei documenti che ne vengono. Si fa prima di leggere /dati, così l'esito che lo script confronta è
// quello della scena con i PDF veri. (Integrazione E del 13/10, prima corsa nel browser.)
func (s *maScenaH) pdfVeriNelloStaging(t *testing.T) {
	t.Helper()
	b := s.b
	righe, err := b.pool.Query(b.ctx, `SELECT a.allegato_id, a.path_staging FROM allegato a
		JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.thread_id = $1 AND lower(a.nome_file) LIKE '%.pdf' AND a.path_staging IS NOT NULL
		ORDER BY a.nome_file`, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	type pdfScena struct {
		id       uuid.UUID
		percorso string
	}
	var pdf []pdfScena
	for righe.Next() {
		var p pdfScena
		if err := righe.Scan(&p.id, &p.percorso); err != nil {
			righe.Close()
			t.Fatal(err)
		}
		pdf = append(pdf, p)
	}
	righe.Close()
	if err := righe.Err(); err != nil {
		t.Fatal(err)
	}
	if len(pdf) == 0 {
		t.Fatal("la scena B87 non ha PDF nello staging: le miniature non si provano")
	}
	radice := filepath.Dir(pdf[0].percorso)
	for i, p := range pdf {
		if filepath.Dir(p.percorso) != radice {
			t.Fatalf("i PDF della scena non stanno in una sola cartella: %s, %s", radice, p.percorso)
		}
		contenuto := pdfTavola(fmt.Sprintf("712090%d", i+1))
		if err := os.WriteFile(p.percorso, contenuto, 0o644); err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`,
			`UPDATE documento SET sha256 = $2, bytes = $3 WHERE documento_id IN
				(SELECT documento_id FROM documento_provenienza WHERE allegato_id = $1)`,
		} {
			if _, err := b.pool.Exec(b.ctx, q, p.id, shaDi(contenuto), len(contenuto)); err != nil {
				t.Fatal(err)
			}
		}
	}
	b.ws.Staging = radice
}

// maScenaHNelBrowser: la scena, il secondo prodotto, il flag acceso, l'ordine dei prodotti e le diagnostiche da /dati.
func (b *bancoWeb) maScenaHNelBrowser(t *testing.T, chiave string) *maScenaH {
	t.Helper()
	s := &maScenaH{mwScena: b.mwScena(t, chiave), b: b}
	s.secondoProdotto = s.componenteTipo("7120801", "finito")
	s.pdfVeriNelloStaging(t)
	mwAccendi(t, b, s.regole)
	resp, corpo := s.ad.fai(http.MethodGet, s.dati(), nil, false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/motore-a/dati: %d %.300s", resp.StatusCode, corpo)
	}
	var d struct {
		Esito *struct {
			ProdottiValutati []struct {
				Rif string `json:"rif"`
			} `json:"prodotti_valutati"`
			Diagnostiche []json.RawMessage `json:"diagnostiche"`
		} `json:"esito"`
	}
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatal(err)
	}
	if d.Esito == nil || len(d.Esito.ProdottiValutati) < 2 {
		t.Fatalf("la scena non ha due prodotti valutati: %.400s", corpo)
	}
	s.primo, s.secondo = d.Esito.ProdottiValutati[0].Rif, d.Esito.ProdottiValutati[1].Rif
	s.diagnostiche = len(d.Esito.Diagnostiche)
	if s.diagnostiche == 0 {
		t.Fatalf("la scena non ha diagnostiche del thread: lo Scenario H vuole che si vedano (%.400s)", corpo)
	}
	s.sonda = httptest.NewServer(http.HandlerFunc(s.maSondaH))
	t.Cleanup(s.sonda.Close)
	return s
}

// lanciaScenarioH: lo script del browser, come le altre prove *_browser_test.go: python da exec.LookPath, lo script
// e2e/motore_a.py, l'amministratore del banco web (AD, il solo che vede la pagina), COCKPIT_FOTO_DIR e
// COCKPIT_BROWSER_CANALE; salta se Python, Playwright o il browser mancano.
func lanciaScenarioH(t *testing.T, s *maScenaH, prove string) {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "motore_a.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	arg := []string{script, "--url", s.b.srv.URL, "--sonda", s.sonda.URL, "--ip", "10.0.0.5:51000", "--sigla", "AD", "--password", "prova-ad",
		"--thread", s.thread.String(), "--primo", s.primo, "--secondo", s.secondo, "--diagnostiche", strconv.Itoa(s.diagnostiche),
		"--prove", prove}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	uscita, err := exec.Command(python, arg...).CombinedOutput()
	testo := strings.TrimRight(string(uscita), "\r\n")
	if err != nil {
		if strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("le prove nel browser sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
}

// TestMotoreAScenarioHNelBrowser: H1…H6 in sequenza sulla stessa RFQ. Dopo lo script il database dice che cosa è
// successo davvero: una sola proposta scartata (H3), l'altra ancora aperta, il secondo prodotto intatto; le regole sono
// di nuovo quelle della scena (H6 le rimette).
func TestMotoreAScenarioHNelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaHNelBrowser(t, "MAH01")
	lanciaScenarioH(t, s, "123456")
	scartate := contaSQL(t, b, `SELECT count(*) FROM documento_proposta WHERE thread_id = $1 AND stato = 'scartata'`, s.thread)
	if scartate != 1 {
		t.Errorf("proposte scartate dopo lo Scenario H: %d, attesa una (H3)", scartate)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE componente_id = $1 AND archiviato_il IS NULL`, s.secondoProdotto); n != 1 {
		t.Errorf("il secondo prodotto non è più attivo: %d", n)
	}
	if b.ws.MotoreA.Stato != s.regole.Stato {
		t.Errorf("dopo H6 le regole non sono quelle della scena: %q", b.ws.MotoreA.Stato)
	}
}
