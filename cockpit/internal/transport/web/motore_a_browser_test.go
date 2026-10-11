//go:build integrazione && browser

// L4 — giro 5, A1d: la Distinta nuova in un browser vero (piano A1d.3; BE-19; V-40; piano A1d backend §9.3; piano A1d
// frontend §4.6, §9; RAC-01, RAC-20). P-18: scritta ora, NON ESEGUITA qui; si esegue sul PC originale, con il database
// di prova, Python, Playwright ed Edge (T-A1d-35: «L4» per la regola delle testate; la prova della Distinta legacy
// usa «L7»):
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestMotoreANelBrowser ./internal/transport/web/
//
// La scena è quella delle L4 di A1d (maScenaL4, motore_a_db_test.go: scenaSenzaGet con le righe in più) con due PDF
// veri, il disegno di 7120099 e la proposta aperta 7120099_1.pdf, perché le miniature e il palco li disegnino; la
// scrittura sul NAS è spenta, perché «Riprova le copie» sia un rifiuto con 200. Il flag è acceso con le regole ACME.
//
// Lo script e2e/motore_a.py (un file del prodotto, come gli altri e2e/*.py: RAC-01) entra come amministratore e prova:
//   - A l'apertura: nessun errore in console, nessun testo del programma a vista, la mappa del ponte prima del modulo,
//     nessuna scrittura (DB7-6);
//   - B il cambio di scheda dell'albero e di prodotto, poi vista=documenti con l'evidenza e il fuoco sul titolo;
//   - C le miniature: solo GET /allegato/<id>/anteprima, senza HX-Request (il registro delle richieste del browser);
//   - D l'anteprima persistente: la casella scelta resta scelta dopo la rilettura di #ma e dopo il ricaricamento;
//   - E un gesto rifiutato con 200 («Riprova le copie» con la scrittura sul NAS spenta) → #ma-avviso[data-esito="no"],
//     #ma non riletto, la sonda invariata;
//   - F un gesto riuscito («Scarta» della proposta aperta) → data-esito="ok", #ma riletto, una riga cambiata nella sonda;
//   - G il flag spento a metà prova → la pagina risponde 404 alla ricarica.
//
// La «sonda» è un indirizzo di questa prova che restituisce la fotografia delle righe della RFQ (come quella della
// Distinta, distinta_browser_test.go) e, con POST /spegni, spegne il flag (la prova G).
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
)

// maScenaBrowser: la scena delle L4 con la sonda.
type maScenaBrowser struct {
	*maScenaL4
	sonda *httptest.Server
}

// maPdfVero mette nello staging dell'allegato un PDF vero (pdfTavola, richieste_browser_test.go) e riscrive lo sha e la
// dimensione dell'allegato e dei documenti che vengono da lui, come pdfVero della Distinta.
func (s *maScenaBrowser) maPdfVero(t *testing.T, allegato uuid.UUID, cartiglio string) {
	t.Helper()
	pdf := pdfTavola(cartiglio)
	percorso := valoreR(s.rfqFascicolo, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, allegato)
	if err := os.WriteFile(percorso, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	s.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, allegato, shaDi(pdf), len(pdf))
	s.esegui(`UPDATE documento SET sha256 = $2, bytes = $3 WHERE documento_id IN
		(SELECT documento_id FROM documento_provenienza WHERE allegato_id = $1)`, allegato, shaDi(pdf), len(pdf))
}

// maSonda risponde allo script: GET = la fotografia delle righe della RFQ che un gesto della pagina può toccare, in forma
// confrontabile; POST /spegni = il flag spento (la prova G), come un riavvio con anteprima = false.
func (s *maScenaBrowser) maSonda(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/spegni" {
		s.b.ws.MotoreA = AnteprimaMotoreA{}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx := r.Context()
	elenco := func(sql string) []string {
		righe, err := s.b.pool.Query(ctx, sql, s.thread)
		if err != nil {
			panic(err)
		}
		defer righe.Close()
		out := []string{}
		for righe.Next() {
			var x string
			if err := righe.Scan(&x); err != nil {
				panic(err)
			}
			out = append(out, x)
		}
		sort.Strings(out)
		return out
	}
	mappa := func(sql string) map[string]string {
		righe, err := s.b.pool.Query(ctx, sql, s.thread)
		if err != nil {
			panic(err)
		}
		defer righe.Close()
		out := map[string]string{}
		for righe.Next() {
			var k, v string
			if err := righe.Scan(&k, &v); err != nil {
				panic(err)
			}
			out[k] = v
		}
		return out
	}
	foto := map[string]any{
		"componenti": elenco(`SELECT codice || '|' || tipo::text || CASE WHEN archiviato_il IS NULL THEN '' ELSE '|archiviato' END FROM componente WHERE thread_id = $1`),
		"relazioni": elenco(`SELECT p.codice || '>' || f.codice || 'x' || r.qta FROM componente_relazione r
			JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id WHERE r.thread_id = $1`),
		"documenti": elenco(`SELECT d.nome_file || '|' || d.tipo::text || '|' || coalesce(c.codice, '-') || '|' || d.stato_nas::text
			|| CASE WHEN d.sostituito_da IS NULL THEN '' ELSE '|sostituito' END
			FROM documento d LEFT JOIN componente c ON c.componente_id = d.componente_id WHERE d.thread_id = $1`),
		"proposte": mappa(`SELECT a.nome_file, p.stato::text || '|' || coalesce(c.codice, '-') || '|' || coalesce(p.codice, '') || '|' || p.tipo_proposto::text
			FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
			LEFT JOIN componente c ON c.componente_id = p.componente_id WHERE p.thread_id = $1`),
		"copie": mappa(`SELECT d.nome_file, count(j.job_id)::text FROM documento d
			JOIN job j ON j.chiave_idempotenza = 'nas:' || d.documento_id::text AND j.tipo = 'copia_nas'
			WHERE d.thread_id = $1 GROUP BY d.nome_file`),
		"nodi":     mappa(`SELECT stato::text, count(*)::text FROM componente_proposta WHERE thread_id = $1 GROUP BY stato`),
		"archi":    mappa(`SELECT stato::text, count(*)::text FROM relazione_proposta WHERE thread_id = $1 GROUP BY stato`),
		"deroghe":  elenco(`SELECT c.codice || '|' || d.tipo::text FROM deroga_fabbisogno d JOIN componente c ON c.componente_id = d.componente_id WHERE d.thread_id = $1`),
		"versioni": elenco(`SELECT numero::text || '|' || stato::text FROM bom_versione WHERE thread_id = $1`),
		"job":      mappa(`SELECT tipo::text, count(*)::text FROM job WHERE $1::uuid IS NOT NULL GROUP BY tipo`),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(foto)
}

// maScenaNelBrowser: la scena delle L4, i due PDF veri, la sonda, la scrittura sul NAS spenta e il flag acceso.
func (b *bancoWeb) maScenaNelBrowser(t *testing.T, chiave string) *maScenaBrowser {
	t.Helper()
	s := &maScenaBrowser{maScenaL4: b.maScenaL4(t, chiave)}
	s.maPdfVero(t, s.disegno, "7120099")
	s.maPdfVero(t, uuidSQL(t, b, `SELECT allegato_id FROM documento_proposta WHERE proposta_id = $1`, s.proposta), "7120099")
	ImpostaCapacitaProva(t, coda.Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: false})
	maAccendi(t, b, s.maRegole(t, 20000))
	s.sonda = httptest.NewServer(http.HandlerFunc(s.maSonda))
	t.Cleanup(s.sonda.Close)
	return s
}

// lanciaMotoreA esegue lo script del browser sulle prove indicate, come lanciaDistinta (distinta_browser_test.go): python
// da exec.LookPath, lo script e2e/motore_a.py, l'amministratore del banco web (AD, il solo che vede la pagina),
// COCKPIT_FOTO_DIR e COCKPIT_BROWSER_CANALE; salta se Python, Playwright o il browser mancano.
func lanciaMotoreA(t *testing.T, s *maScenaBrowser, prove string) {
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
		"--thread", s.thread.String(), "--proposta", s.proposta.String(), "--prove", prove}
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

// TestMotoreANelBrowser (BE-19; V-40): le prove A…G dello script, in sequenza sulla stessa RFQ (G spegne il flag e va per
// ultima). Dopo lo script il database dice che cosa è successo davvero: la proposta scartata da F, nessuna copia accodata
// da E (la scrittura sul NAS era spenta), il flag spento da G.
func TestMotoreANelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaNelBrowser(t, "MAB01")
	lanciaMotoreA(t, s, "ABCDEFG")
	if stato := valoreR(s.rfqFascicolo, `SELECT stato::text FROM documento_proposta WHERE proposta_id = $1`, s.proposta); stato != "scartata" {
		t.Errorf("la proposta di F: %s, attesa scartata", stato)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 0 {
		t.Errorf("con la scrittura sul NAS spenta sono state accodate %d copie", n)
	}
	if b.ws.MotoreA.Accesa {
		t.Error("dopo la prova G il flag è ancora acceso")
	}
}
