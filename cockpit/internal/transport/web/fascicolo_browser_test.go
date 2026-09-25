//go:build integrazione && browser

// L7 — la schermata del Fascicolo in un BROWSER VERO (B8.7, piano §13 e §14.5; rifatta in B8.7b).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7 ./internal/transport/web/
//
// Serve Playwright per Python e Microsoft Edge (`--canale msedge`), come le altre prove L7. Con
// COCKPIT_FOTO_DIR lo script salva le fotografie della pagina in quella cartella.
//
// Perche' non basta L4. Le prove L4 leggono l'HTML che torna e verificano che la risposta di un gesto
// porti i pannelli fuori banda e non il corpo dell'anteprima. Che il browser li metta al posto giusto,
// che il PDF aperto resti lo stesso elemento dopo un gesto, che la pagina non si ricarichi, che la
// tabella con cento file non scorra in orizzontale: questo lo vede solo un browser.
package web

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
)

// famiglieL7: la famiglia dei codici 52/53 del cliente di prova, perche' i nodi degli STEP si
// classifichino come codici.
const famiglieL7 = `{"famiglie_codice": [{"regex": "(?P<codice>5[23]\\d{6})", "descrizione": "disegni 52/53", "esempio": "52922757"}]}`

// fattiL7 e' uno STEP del prodotto con un nodo nuovo: 52922757 → 52920517 ×2 (c'e' gia') → 53011111 ×2.
const fattiL7 = `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [],
	"nodi": [{"chiave": "#1", "id_grezzo": "52922757", "nome_grezzo": "52922757", "evidenza": {}},
	         {"chiave": "#2", "id_grezzo": "52920517", "nome_grezzo": "52920517", "evidenza": {}},
	         {"chiave": "#3", "id_grezzo": "53011111", "nome_grezzo": "53011111", "evidenza": {}}],
	"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}, {"padre": "#2", "figlio": "#3", "qta": 2, "evidenza": {}}],
	"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	"occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`

type scenaL7 struct {
	*scenaB87
	pdf      uuid.UUID   // l'allegato PDF vero, per il viewer
	stp      uuid.UUID   // l'allegato STEP con i fatti
	proposte []uuid.UUID // tre file da assegnare con un gesto
}

func (b *bancoWeb) scenaL7(t *testing.T, chiave string, extra int) *scenaL7 {
	t.Helper()
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	s := &scenaL7{scenaB87: b.scenaB87(chiave)}
	s.esegui(`UPDATE cliente SET regole = $1 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $2)`, famiglieL7, s.thread)
	// un codice visto nella mail e non ancora nella BOM: nel cassetto «Codici» offre «+ Prodotto / + Assieme / + Particolare»
	s.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
		VALUES ($1, '53099999', 'prodotto', '', 'famiglia', 'disegni 52/53', 80, 'corpo')`, s.msg)
	b.caricamentoAcceso(t)
	// l'anteprima serve dallo staging solo cio' che sta sotto la radice dichiarata
	b.ws.Staging = s.staging
	// v3: la vista Documenti disegna con pdf.js ogni PDF del prodotto, anche il 2D gia' confermato: deve essere
	// un PDF vero (un file rotto sarebbe un errore nella pagina)
	s.pdfVero(t, s.allDisegno, 1)
	s.pdf = s.allLibero
	s.pdfVero(t, s.pdf, 256*1024)
	var libero2 uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT allegato_id FROM documento_proposta WHERE proposta_id = $1`, s.pdfLibero2).Scan(&libero2); err != nil {
		t.Fatal(err)
	}
	s.pdfVero(t, libero2, 64*1024)
	s.stp = s.stepNellaRfq("assieme.stp", fattiL7, an)
	for i, nome := range []string{"53017189 foglio 2.pdf", "53017189 foglio 3.pdf", "53017189 foglio 4.pdf"} {
		p, a := s.propostaDa(nome, "53017189")
		s.pdfVero(t, a, 64*1024+i+1)
		s.proposte = append(s.proposte, p)
	}
	for i := 0; i < extra; i++ {
		s.propostaDa(fmt.Sprintf("disegno %03d con un nome lungo come quelli dei clienti veri, per vedere se va a capo.pdf", i), fmt.Sprintf("5300%04d", i))
	}
	return s
}

// pdfVero mette nello staging, al posto del contenuto finto, un PDF vero (una tavola che pdf.js e il viewer
// del browser sanno disegnare; n diversi, contenuti diversi): il dettaglio di un componente apre da solo il
// primo 2D in arrivo (B8.7b), e il browser deve ricevere un PDF, non un 415. Il documento nato da quel file
// prende la stessa impronta.
func (s *scenaL7) pdfVero(t *testing.T, allegato uuid.UUID, n int) {
	t.Helper()
	pdf := pdfTavola(fmt.Sprintf("TAV-%d", n))
	var percorso string
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, allegato).Scan(&percorso); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(percorso, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	s.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, allegato, shaDi(pdf), len(pdf))
	s.esegui(`UPDATE documento SET sha256 = $2, bytes = $3 WHERE documento_id IN
		(SELECT documento_id FROM documento_provenienza WHERE allegato_id = $1)`, allegato, shaDi(pdf), len(pdf))
}

// lanciaL7 esegue lo script del browser sulle prove indicate.
func lanciaL7(t *testing.T, s *scenaL7, prove string) {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "fascicolo.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	ids := make([]string, len(s.proposte))
	for i, p := range s.proposte {
		ids[i] = p.String()
	}
	arg := []string{script, "--url", s.b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "FP", "--password", "prova-fp",
		"--thread", s.thread.String(), "--prodotto", s.prodotto.String(), "--assieme", s.assieme.String(), "--particolare", s.particolare.String(),
		"--pdf", s.pdf.String(), "--stp", s.stp.String(), "--proposte", strings.Join(ids, ","), "--prove", prove}
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

// Accettare il nodo e poi la struttura proposta (nell'editor, Fascicolo v3) cambia la BOM senza ricaricare
// la pagina e senza chiudere il PDF aperto; un clic sulla card apre il dettaglio del componente; un clic sul
// file apre il PDF; la struttura si corregge (tipo) e la completezza sulla card cambia; le viste di servizio
// restano.
func TestL7IlFascicoloSiCostruisceSenzaF5(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "L7A", 0)
	lanciaL7(t, s, "ABCDFH")
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '53011111'`, s.thread); n != 1 {
		t.Errorf("il nodo accettato nel browser non e' nella BOM: %d", n)
	}
	if got := s.valore(`SELECT tipo::text FROM componente WHERE componente_id = $1`, s.assieme); got != "sciolto" {
		t.Errorf("il tipo cambiato nel browser: %s", got)
	}
	if got := s.valore(`SELECT tipo::text || '/' || origine::text FROM componente WHERE thread_id = $1 AND codice = '53099999'`, s.thread); got != "sciolto/codice_rilevato" {
		t.Errorf("il codice aggiunto dal cassetto nel browser: %s", got)
	}
}

// Tre file con un gesto: scelti con le caselle, assegnati al nodo scelto.
func TestL7AssegnareTreFileConUnGesto(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "L7E", 0)
	lanciaL7(t, s, "E")
	if n := s.conta(`SELECT count(*) FROM documento_proposta WHERE componente_id = $1`, s.particolare); n != 3 {
		t.Errorf("proposte assegnate al particolare: %d, attese 3", n)
	}
}

// Cento file: la pagina si disegna in meno di un secondo, il filtro funziona, niente scorrimento
// orizzontale.
func TestL7CentoAllegati(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "L7G", 100)
	lanciaL7(t, s, "G")
}

// «Conferma Fascicolo» (B8.7b): il riepilogo spunta tutto il pronto, un gesto porta nel fascicolo i documenti
// e lo STEP strutturale del prodotto, senza ricaricare. Fascicolo v3: la struttura dello STEP non entra con
// quel gesto; entra dopo, dall'editor della Struttura BOM, e lo script lo verifica fra i due.
func TestL7ConfermaFascicolo(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "L7I", 0)
	lanciaL7(t, s, "I")
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '53011111'`, s.thread); n != 1 {
		t.Errorf("il nodo dello STEP non e' nato con la conferma nell'editor: %d", n)
	}
	// i due documenti della scena, piu' i cinque PDF del piano: 53017189.pdf e i tre fogli al particolare,
	// 52920517.pdf all'assieme
	if n := s.conta(`SELECT count(*) FROM documento WHERE thread_id = $1`, s.thread); n != 7 {
		t.Errorf("documenti dopo la conferma: %d, attesi 7", n)
	}
	if n := s.conta(`SELECT count(*) FROM documento WHERE componente_id = $1 AND tipo = 'disegno_2d'`, s.particolare); n != 4 {
		t.Errorf("disegni del particolare dopo la conferma: %d, attesi 4", n)
	}
	if got := s.valore(`SELECT coalesce(step_strutturale_id::text, '') FROM componente WHERE componente_id = $1`, s.prodotto); got != s.step.String() {
		t.Errorf("STEP strutturale del prodotto: %q, atteso %s", got, s.step)
	}
}
