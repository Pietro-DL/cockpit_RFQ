//go:build integrazione && browser

// L7 — la richiesta con lo ZIP, dall'arrivo della mail al Fascicolo confermato (B8.7b).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7 ./cmd/cockpit/
//
// Tutto vero tranne Outlook. cockpit.exe si compila da qui e parte con un cockpit.toml scritto in una
// cartella temporanea: il database di prova (lo schema lo ricrea il server stesso, con le migrazioni
// dell'avvio), un NAS e uno staging temporanei, la postazione con il nome di QUESTO PC (il worker la
// dichiara al claim). Il worker Outlook e' quello vero con la posta finta (prova_e2e.py sostituisce solo la
// classe che parla con COM: la mail di ACME con lo ZIP); il worker di analisi e' quello vero. Il browser
// (Playwright su Edge, e2e/zip_fascicolo.py) fa quello che fa l'operatore. Nel database non entra niente
// se non dalle strade del prodotto: nessuna riga scritta dal test.
//
// Il worker di analisi parte quando il browser lo chiede, a Fascicolo aperto: l'estrazione e le analisi
// arrivano mentre la pagina e' davanti, e l'aggiornamento senza F5 si vede davvero (B8.7b, il punto rotto
// del checkpoint: la pipeline era giusta, la pagina non si aggiornava).
//
// Dopo il browser, il database, un passo per volta: la RFQ e il prodotto finito nato dalla decisione (prima
// di ogni analisi), lo ZIP nello staging, l'estrazione, i figli nella cache per contenuto, le analisi v3, le
// proposte dello STEP, la conferma unica (componenti, archi, documenti, STEP strutturale), le copie sul NAS
// con il contenuto giusto, il DXF importato dal NAS e rimasto dov'era.
//
// Serve Python con Playwright (canale msedge), PyMuPDF e le dipendenze dei worker. Senza Playwright o senza
// browser la prova si salta (SKIP, mai PASS); con COCKPIT_TEST_SENZA_PYTHON anche.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/testutil"
)

// processoL7 e' un processo del banco (server, worker): l'output va in un file, che si legge solo se
// qualcosa va storto — ed e' allora l'unico posto dove si vede che cosa si sono detti.
type processoL7 struct {
	nome   string
	cmd    *exec.Cmd
	log    string
	uscito chan struct{} // chiuso quando il processo finisce
}

func avviaL7(t *testing.T, nome, dir, log string, env []string, prog string, arg ...string) *processoL7 {
	t.Helper()
	f, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(prog, arg...)
	cmd.Dir = dir
	// le variabili in coda vincono su quelle ereditate: niente worker.toml di questo PC, niente server vero
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		t.Fatalf("avvio di %s: %v", nome, err)
	}
	p := &processoL7{nome: nome, cmd: cmd, log: log, uscito: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.uscito)
	}()
	// prima della cartella temporanea (le Cleanup vanno all'indietro): su Windows un file aperto non si cancella
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-p.uscito
		_ = f.Close()
	})
	return p
}

// vivo dice se il processo non e' ancora uscito.
func (p *processoL7) vivo() bool {
	select {
	case <-p.uscito:
		return false
	default:
		return true
	}
}

// coda sono le ultime righe dell'output.
func (p *processoL7) coda(n int) string {
	b, err := os.ReadFile(p.log)
	if err != nil {
		return err.Error()
	}
	righe := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	if len(righe) > n {
		righe = righe[len(righe)-n:]
	}
	return fmt.Sprintf("--- %s (ultime %d righe)\n%s", p.nome, len(righe), strings.Join(righe, "\n"))
}

func gettone(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func portaLibera(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// cockpitTomlL7 e' la configurazione del banco. Le password e i gettoni valgono per la durata della prova.
func cockpitTomlL7(porta int, dsn, radice, staging, host, gOutlook, gAnalisi string) string {
	return fmt.Sprintf(`# banco della prova L7 dello ZIP (zip_browser_test.go): tutto temporaneo
[server]
indirizzo = "127.0.0.1:%d"
log_livello = "info"
modalita = "produzione"

[db]
dsn = %q

[nas]
radice = '%s'
staging = '%s'
radici_produzione = ['\\nas-finto\PRODUZIONE']
intervallo_integrita_s = 0

[outlook]
cartelle = ["Inbox"]
intervallo_sync_s = 3
sync_apertura_inbox = false
giorni_sync_iniziale = 7
lotto = 50
casella_default = "commerciale@azienda.example"

[sicurezza]
outlook_scrittura = false
bozze             = false
nas_scrittura     = true

[analisi]
versione = 3
[analisi.parametri]
termini_cartiglio = ["scala", "materiale", "tolleranze", "trattamento"]

[staging]
automatico = false

[[utenti]]
sigla = "FP"
nome = "Operatore Prova"
ufficio = "Commerciale"
ruolo = "operatore"
password = "prova-fp"

[[utenti]]
sigla = "PS"
nome = "Admin Prova"
ufficio = "IT"
ruolo = "admin"
password = "prova-ps"

[[casella]]
indirizzo = "commerciale@azienda.example"
nome      = "Commerciale"
condivisa = true

[[postazione]]
nome_host   = %q
descrizione = "banco della prova L7 dello ZIP"
utente      = "FP"

[[worker]]
nome       = "outlook@%s"
tipo       = "outlook"
token      = %q
postazione = %q
caselle    = ["commerciale@azienda.example"]

[[worker]]
nome       = "analisi@%s"
tipo       = "analisi"
token      = %q
postazione = %q
`, porta, dsn, radice, staging, host, host, gOutlook, host, host, gAnalisi, host)
}

// postaL7 e' la mail di ACME, arrivata un quarto d'ora fa, con lo ZIP: il formato di COCKPIT_E2E_POSTA.
func postaL7(zip string, n int64) map[string]any {
	quando := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	return map[string]any{
		"messaggi": []map[string]any{{
			"message_id": "<rfq-acme-52922757@acme.example>", "entry_id": "ENTRY-ACME-1", "conversation_id": "CONV-ACME-1",
			"cartella": "Inbox", "direzione": "entrata", "data_evento": quando, "ricevuto_il": quando,
			"mittente_nome": "Mario Rossi", "mittente_indirizzo": "mario.rossi@acme.example",
			"destinatari": []map[string]any{{"nome": "Commerciale", "indirizzo": "commerciale@azienda.example", "tipo": "a"}},
			"oggetto":     "Richiesta di offerta 52922757 supporto cofano",
			"corpo_testo": "Buongiorno,\nvi chiediamo un'offerta per il supporto cofano 52922757 (200 pezzi/anno).\nIn allegato lo zip con STEP, disegni e capitolato.\nCordiali saluti\nMario Rossi",
			"non_letto":   true,
			"allegati":    []map[string]any{{"indice": 1, "nome_file": "RFQ ACME 52922757.zip", "estensione": "zip", "natura": "file", "bytes": n}},
		}},
		"file": map[string]string{"ENTRY-ACME-1|1": zip},
	}
}

// DXF minimo, lo sviluppo che sta gia' nell'archivio del cliente sul NAS.
const dxfL7 = "0\nSECTION\n2\nENTITIES\n0\nLINE\n8\n0\n10\n0\n20\n0\n11\n100\n21\n50\n0\nENDSEC\n0\nEOF\n"

func shaFile(t *testing.T, p string) string {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("il file %s: %v", p, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestL7LoZipDallaPostaAlFascicoloConfermato(t *testing.T) {
	if os.Getenv("COCKPIT_TEST_SENZA_PYTHON") != "" {
		t.Skip("COCKPIT_TEST_SENZA_PYTHON: i worker veri non vengono avviati, prova non verificata")
	}
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	goexe, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go non trovato: serve per compilare cockpit.exe: %v", err)
	}
	dsn := testutil.DSN(t) // un database il cui nome contiene «test»: lo schema si butta
	pool := testutil.Pool(t)
	testutil.SchemaVuoto(t, pool) // le migrazioni le applica il server all'avvio, come sempre
	ctx := context.Background()

	pkg, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workers, err := filepath.Abs(filepath.Join(pkg, "..", "..", "workers"))
	if err != nil {
		t.Fatal(err)
	}
	e2e := filepath.Join(pkg, "e2e")
	dir := t.TempDir()
	radice := filepath.Join(dir, "nas", "PREVENTIVI DA FARE")
	staging := filepath.Join(dir, "staging")
	wOutlook, wAnalisi := filepath.Join(dir, "w_outlook"), filepath.Join(dir, "w_analisi")
	archivio := filepath.Join(radice, "ACME", "ARCHIVIO 2025", "52922757")
	for _, d := range []string{staging, wOutlook, wAnalisi, archivio} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dxfNas := filepath.Join(archivio, "52922757.dxf")
	if err := os.WriteFile(dxfNas, []byte(dxfL7), 0o644); err != nil {
		t.Fatal(err)
	}

	// lo ZIP della richiesta e la posta finta
	uscita, err := exec.Command(python, filepath.Join(e2e, "zip_richiesta.py"), filepath.Join(dir, "fixture")).CombinedOutput()
	if err != nil {
		if strings.Contains(string(uscita), "ModuleNotFoundError") {
			t.Skipf("manca un modulo Python (PyMuPDF?):\n%s", uscita)
		}
		t.Fatalf("lo ZIP di prova: %v\n%s", err, uscita)
	}
	zip := strings.TrimSpace(string(uscita))
	st, err := os.Stat(zip)
	if err != nil {
		t.Fatal(err)
	}
	posta, _ := json.Marshal(postaL7(zip, st.Size()))
	fposta := filepath.Join(dir, "posta.json")
	if err := os.WriteFile(fposta, posta, 0o644); err != nil {
		t.Fatal(err)
	}

	// cockpit.exe, compilato da qui
	exe := filepath.Join(dir, "cockpit_l7.exe")
	if out, err := exec.Command(goexe, "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	host = strings.ToUpper(host)
	gOutlook, gAnalisi := gettone(t), gettone(t)
	porta := portaLibera(t)
	url := fmt.Sprintf("http://127.0.0.1:%d", porta)
	cfg := filepath.Join(dir, "cockpit.toml")
	if err := os.WriteFile(cfg, []byte(cockpitTomlL7(porta, dsn, radice, staging, host, gOutlook, gAnalisi)), 0o600); err != nil {
		t.Fatal(err)
	}
	server := avviaL7(t, "server", dir, filepath.Join(dir, "server.out"), nil, exe, "-config", cfg)
	pronto := time.Now().Add(90 * time.Second)
	for {
		r, err := http.Get(url + "/login")
		if err == nil {
			r.Body.Close()
			if r.StatusCode == http.StatusOK {
				break
			}
		}
		if !server.vivo() {
			t.Fatalf("il server e' uscito invece di partire:\n%s", server.coda(60))
		}
		if time.Now().After(pronto) {
			t.Fatalf("il server non risponde su %s dopo 90 s\n%s", url, server.coda(60))
		}
		time.Sleep(300 * time.Millisecond)
	}
	var processi = []*processoL7{server}
	logDi := func() string {
		var b strings.Builder
		for _, p := range processi {
			b.WriteString(p.coda(40) + "\n")
		}
		return b.String()
	}

	py := func(staging, gettone, id string, extra ...string) []string {
		return append([]string{"COCKPIT_URL=" + url, "COCKPIT_TOKEN=" + gettone, "COCKPIT_WORKER_ID=" + id,
			"COCKPIT_STAGING=" + staging, "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1"}, extra...)
	}
	processi = append(processi, avviaL7(t, "worker outlook", workers, filepath.Join(dir, "outlook.out"),
		py(wOutlook, gOutlook, "outlook@"+host, "COCKPIT_E2E_POSTA="+fposta, "COCKPIT_E2E_CONTINUO=1"), python, "prova_e2e.py"))

	// il browser; l'analisi parte quando il browser ha il Fascicolo davanti
	segnale := filepath.Join(dir, "fascicolo-aperto")
	arg := []string{filepath.Join(e2e, "zip_fascicolo.py"), "--url", url, "--segnale", segnale}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	browser := exec.Command(python, arg...)
	var out strings.Builder
	browser.Stdout, browser.Stderr = &out, &out
	browser.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	if err := browser.Start(); err != nil {
		t.Fatal(err)
	}
	fatto := make(chan error, 1)
	go func() { fatto <- browser.Wait() }()
	analisi := false
	limite := time.After(8 * time.Minute)
	var errBrowser error
attesa:
	for {
		select {
		case errBrowser = <-fatto:
			break attesa
		case <-limite:
			_ = browser.Process.Kill()
			errBrowser = fmt.Errorf("il browser non ha finito in 8 minuti")
			<-fatto
			break attesa
		case <-time.After(200 * time.Millisecond):
			if _, err := os.Stat(segnale); err == nil && !analisi {
				analisi = true
				processi = append(processi, avviaL7(t, "worker analisi", workers, filepath.Join(dir, "analisi.out"),
					py(wAnalisi, gAnalisi, "analisi@"+host), python, "worker_analisi.py", "--config", filepath.Join(dir, "nessun-worker.toml")))
			}
		}
	}
	testo := strings.TrimRight(out.String(), "\r\n")
	if errBrowser != nil {
		if strings.Contains(testo, "ModuleNotFoundError: No module named 'playwright'") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("i passi nel browser sono falliti (%v):\n%s\n%s", errBrowser, testo, logDi())
	}
	t.Logf("passi nel browser:\n%s", testo)
	var esito struct {
		Thread string `json:"thread"`
	}
	for _, riga := range strings.Split(testo, "\n") {
		if strings.HasPrefix(riga, "ESITO ") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(riga), "ESITO ")), &esito); err != nil {
				t.Fatalf("la riga ESITO: %v", err)
			}
		}
	}
	if esito.Thread == "" {
		t.Fatalf("il browser non ha detto il thread:\n%s", testo)
	}
	d := &dbL7{t: t, ctx: ctx, pool: pool, thread: esito.Thread}
	d.controlla(zip, radice, staging, dxfNas)
	if t.Failed() {
		t.Logf("log del banco:\n%s", logDi())
	}
}

// dbL7 legge il database dopo il browser.
type dbL7 struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	thread string
}

func (d *dbL7) valore(sql string, arg ...any) string {
	d.t.Helper()
	var s string
	if err := d.pool.QueryRow(d.ctx, sql, arg...).Scan(&s); err != nil {
		d.t.Fatalf("%s: %v", sql, err)
	}
	return s
}

func (d *dbL7) righe(sql string, arg ...any) []string {
	d.t.Helper()
	rr, err := d.pool.Query(d.ctx, sql, arg...)
	if err != nil {
		d.t.Fatalf("%s: %v", sql, err)
	}
	defer rr.Close()
	var out []string
	for rr.Next() {
		var s string
		if err := rr.Scan(&s); err != nil {
			d.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func (d *dbL7) uguale(passo, sql, atteso string, arg ...any) {
	d.t.Helper()
	if got := d.valore(sql, arg...); got != atteso {
		d.t.Errorf("%s: %q, atteso %q", passo, got, atteso)
	}
}

func (d *dbL7) insieme(passo, sql string, attesi []string, arg ...any) {
	d.t.Helper()
	got := strings.Join(d.righe(sql, arg...), "; ")
	if w := strings.Join(attesi, "; "); got != w {
		d.t.Errorf("%s:\n  trovati %s\n  attesi  %s", passo, got, w)
	}
}

func (d *dbL7) controlla(zip, radice, staging, dxfNas string) {
	t, th := d.t, d.thread
	shaZip := shaFile(t, zip)

	// 1. la RFQ: il codice della richiesta confermato, il prodotto finito nato con la decisione, prima di ogni analisi
	d.uguale("il codice della richiesta", `SELECT (confermato_da IS NOT NULL)::text FROM identificativo_thread WHERE thread_id = $1 AND codice = '52922757'`, "true", th)
	d.uguale("il prodotto finito della richiesta", `SELECT tipo::text || ' ' || origine::text FROM componente WHERE thread_id = $1 AND codice = '52922757'`, "finito codice_rilevato", th)
	d.uguale("il prodotto nasce prima delle analisi", `SELECT (c.creato_il < (SELECT min(f.calcolato_il) FROM analisi_fatti f))::text
		FROM componente c WHERE c.thread_id = $1 AND c.codice = '52922757'`, "true", th)

	// 2. lo ZIP nello staging del server, lo stesso contenuto della mail
	var zipID, pathZip, statoZip string
	if err := d.pool.QueryRow(d.ctx, `SELECT a.allegato_id::text, coalesce(a.path_staging, ''), a.stato::text FROM allegato a
		JOIN messaggio m USING (messaggio_id) WHERE m.thread_id = $1 AND a.nome_file = 'RFQ ACME 52922757.zip'`, th).Scan(&zipID, &pathZip, &statoZip); err != nil {
		t.Fatalf("lo ZIP nel database: %v", err)
	}
	if !strings.HasPrefix(strings.ToLower(pathZip), strings.ToLower(staging)) || shaFile(t, pathZip) != shaZip {
		t.Errorf("lo ZIP nello staging: %q (%s)", pathZip, statoZip)
	}
	// 3. l'estrazione, fatta dal server
	d.uguale("il job di estrazione", `SELECT stato::text FROM job WHERE chiave_idempotenza = $1 ORDER BY job_id DESC LIMIT 1`, "fatto", "estrai:"+zipID)
	// 4. i figli, nella cache per contenuto: il file sta dove dice il suo hash
	voci := []string{"52920517.pdf", "52922757.pdf", "52922757.stp", "53017189 foglio 2.pdf", "Capitolato fornitura.pdf"}
	d.insieme("le voci dello ZIP", `SELECT nome_file FROM allegato WHERE contenitore_id = $1 ORDER BY nome_file`, voci, zipID)
	rr, err := d.pool.Query(d.ctx, `SELECT nome_file, coalesce(path_staging, ''), coalesce(sha256, '') FROM allegato WHERE contenitore_id = $1`, zipID)
	if err != nil {
		t.Fatal(err)
	}
	var shaFigli []string
	for rr.Next() {
		var nome, p, sha string
		if err := rr.Scan(&nome, &p, &sha); err != nil {
			t.Fatal(err)
		}
		cache := strings.ToLower(filepath.Join(staging, "_contenuti"))
		if !strings.HasPrefix(strings.ToLower(p), cache) || !strings.Contains(strings.ToLower(p), sha[:8]) || shaFile(t, p) != sha {
			t.Errorf("%s non sta nella cache per contenuto: %q (sha %s)", nome, p, sha)
		}
		shaFigli = append(shaFigli, sha)
	}
	rr.Close()
	// 5. le analisi: una per contenuto, con l'analizzatore v3
	for _, sha := range shaFigli {
		d.uguale("l'analisi v3 di "+sha[:12], `SELECT count(*)::text FROM analisi_fatti WHERE sha256 = $1 AND versione_analizzatore = 3`, "1", sha)
	}
	// 6. le proposte dello STEP: quattro nodi, quattro archi (le quantita' contano le occorrenze). La radice e'
	// il prodotto nato dal triage: lo STEP lo ritrova (duplicato, agganciato a quello), non ne propone un altro
	d.insieme("i nodi proposti dallo STEP", `SELECT p.codice || ' ' || p.stato::text FROM componente_proposta p JOIN allegato a USING (allegato_id)
		WHERE a.contenitore_id = $1 AND a.nome_file = '52922757.stp' ORDER BY p.codice`,
		[]string{"52920517 confermata", "52922757 duplicato", "53011111 confermata", "53017189 confermata"}, zipID)
	d.uguale("la radice dello STEP e' il prodotto del triage", `SELECT (p.componente_id = c.componente_id)::text FROM componente_proposta p
		JOIN allegato a USING (allegato_id) JOIN componente c ON c.thread_id = $2 AND c.codice = '52922757'
		WHERE a.contenitore_id = $1 AND a.nome_file = '52922757.stp' AND p.codice = '52922757'`, "true", zipID, th)
	d.insieme("gli archi proposti dallo STEP", `SELECT pp.codice || '>' || pf.codice || ' x' || r.qta || ' ' || r.stato::text FROM relazione_proposta r
		JOIN componente_proposta pp ON pp.allegato_id = r.allegato_id AND pp.chiave = r.padre_chiave
		JOIN componente_proposta pf ON pf.allegato_id = r.allegato_id AND pf.chiave = r.figlio_chiave
		JOIN allegato a ON a.allegato_id = r.allegato_id WHERE a.contenitore_id = $1 ORDER BY 1`,
		[]string{"52920517>53011111 x2 confermata", "52920517>53017189 x1 confermata", "52922757>52920517 x2 confermata", "52922757>53017189 x4 confermata"}, zipID)

	// 7. la conferma: la BOM, i documenti, lo STEP strutturale
	d.insieme("i componenti", `SELECT codice || ' ' || tipo::text FROM componente WHERE thread_id = $1 ORDER BY codice`,
		[]string{"52920517 sottoassieme", "52922757 finito", "53011111 sciolto", "53017189 sciolto"}, th)
	d.insieme("gli archi della BOM", `SELECT p.codice || '>' || f.codice || ' x' || r.qta FROM componente_relazione r
		JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id WHERE r.thread_id = $1 ORDER BY 1`,
		[]string{"52920517>53011111 x2", "52920517>53017189 x1", "52922757>52920517 x2", "52922757>53017189 x4"}, th)
	d.insieme("i documenti", `SELECT d.nome_file || ' ' || d.tipo::text || ' ' || coalesce(c.codice, '-') FROM documento d
		LEFT JOIN componente c ON c.componente_id = d.componente_id WHERE d.thread_id = $1 ORDER BY d.nome_file`,
		[]string{"52920517.pdf disegno_2d 52920517", "52922757.dxf sviluppo_dxf 52922757", "52922757.pdf disegno_2d 52922757",
			"52922757.stp cad_3d 52922757", "53017189 foglio 2.pdf disegno_2d 53017189", "Capitolato fornitura.pdf capitolato -"}, th)
	d.uguale("lo STEP strutturale del prodotto", `SELECT coalesce(s.nome_file, '') FROM componente c LEFT JOIN documento s ON s.documento_id = c.step_strutturale_id
		WHERE c.thread_id = $1 AND c.codice = '52922757'`, "52922757.stp", th)
	d.uguale("lo ZIP non e' un documento", `SELECT count(*)::text FROM documento WHERE thread_id = $1 AND nome_file LIKE '%.zip'`, "0", th)

	// 8. le copie sul NAS: nella cartella della RFQ (sotto quella del cliente), con il contenuto giusto
	fine := time.Now().Add(60 * time.Second)
	for d.valore(`SELECT count(*)::text FROM documento WHERE thread_id = $1 AND stato_nas <> 'scritto'`, th) != "0" && time.Now().Before(fine) {
		time.Sleep(time.Second)
	}
	cartella := d.valore(`SELECT coalesce(cartella_relativa, '') FROM thread_offerta WHERE thread_id = $1`, th)
	rr, err = d.pool.Query(d.ctx, `SELECT nome_file, stato_nas::text, coalesce(path_relativo, ''), coalesce(sha256, '') FROM documento WHERE thread_id = $1`, th)
	if err != nil {
		t.Fatal(err)
	}
	defer rr.Close()
	for rr.Next() {
		var nome, stato, rel, sha string
		if err := rr.Scan(&nome, &stato, &rel, &sha); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(radice, filepath.FromSlash(strings.ReplaceAll(cartella, `\`, "/")), filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
		switch {
		case stato != "scritto":
			t.Errorf("%s: sul NAS %s", nome, stato)
		case !strings.HasPrefix(strings.ToUpper(cartella), `ACME\`) || rel == "":
			t.Errorf("%s: la copia %q non sta nella cartella della RFQ %q, sotto quella del cliente", nome, rel, cartella)
		case shaFile(t, p) != sha:
			t.Errorf("%s: la copia sul NAS non ha il contenuto del documento", nome)
		}
	}
	// 9. il DXF importato dal NAS: copiato, non spostato
	if shaFile(t, dxfNas) != hex.EncodeToString(func() []byte { h := sha256.Sum256([]byte(dxfL7)); return h[:] }()) {
		t.Errorf("il DXF nell'archivio del cliente e' cambiato")
	}
	d.uguale("il DXF viene dal NAS", `SELECT coalesce(a.origine::text, '') || ' ' || (coalesce(a.path_interno, '') LIKE 'NAS: %')::text FROM allegato a
		WHERE a.nome_file = '52922757.dxf' AND a.messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE thread_id = $1)`, "manuale true", th)
	// nessun job fallito lungo la strada
	if f := d.righe(`SELECT tipo::text || ' ' || coalesce(chiave_idempotenza, '') || ': ' || coalesce(errore, '') FROM job WHERE stato = 'fallito'`); len(f) > 0 {
		t.Errorf("job falliti: %s", strings.Join(f, "; "))
	}
}
