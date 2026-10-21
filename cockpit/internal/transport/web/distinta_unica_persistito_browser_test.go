//go:build integrazione && browser

// La prova d1 del piano di migrazione (PM23 §12.2): TestV1DistintaUnicaPercorsoPersistito, la Distinta unica con i dati
// persistiti e il RIAVVIO VERO del server.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test_c COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestV1DistintaUnicaPercorsoPersistito ./internal/transport/web/
//
// Diversamente dalle altre prove della pagina, qui il server non è il banco web del pacchetto: è cockpit.exe, compilato
// in una cartella temporanea da cmd/cockpit e avviato come processo con un cockpit.toml nuovo (§12.2 «Server vero»):
// [motore_a] anteprima = true con un indice di grammatiche sintetico (la grammatica ACME di app/runtime/testdata, con
// l'UUID del cliente della prova), [nas].radice e staging temporanei, modalita = "produzione", nas_scrittura accesa e le
// altre due capacità spente, radici_produzione finta e diversa dalla radice; lo schema lo migra il server stesso
// (testutil.SchemaVuoto prima). Il worker di analisi è quello vero, con --config verso un file che non esiste. Il
// riavvio è vero: il processo si ferma con Kill (come un guasto), e un processo nuovo parte con la stessa configurazione
// e lo stesso database; ogni fase nel browser è un browser nuovo, con un accesso nuovo.
//
// Il percorso (e2e/v1_distinta.py, Playwright su Edge, 1440×900) e i controlli sul database fra una fase e l'altra:
//   - F0: avvio, schema 23 con la variante della 0023, nessuna bozza;
//   - F1: la pagina, la Sorgente e la Revisione, «Guarda» di due STEP (S6), «Apri» di un PDF (S5), lo smistamento (S9):
//     ogni tabella uguale prima e dopo, nessun lavoro accodato;
//   - F3: «Conferma questo STEP come sorgente di 9120001…» (S8): la bozza formato 3 v1 dal server, la conferma del
//     documento, la copia dello STEP sul NAS fatta dal lavoro vero; working invariata;
//   - F2: «+ Aggiungi file» (lo zip, il DXF, il collaudo), le analisi del worker vero; F4: le modifiche della bozza; F5: i
//     file (Altri file nella cartella ALTRO del NAS temporaneo, Scarta, i rifiuti con le dipendenze, Riattiva);
//   - F6: il riavvio vero; R1 dà la stessa bozza (lo snapshot salvato, byte per byte), S9 la stessa tabella con le PB e la
//     loro provenienza, la pagina ridisegna la stessa bozza, Altri file e Scartati si rileggono; poi «Usa nuovo STEP…»
//     con lo STEP rev 2 (S6 ?bozza=, la sorgente cambiata nella stessa bozza) e, sul secondo prodotto, S8, «Riparti
//     dalla proposta» e «Abbandona la bozza» (le AR e associazione_evento invariate);
//   - F7: il wizard (R4) con le preassociazioni in conflitto tolte dal rimedio del wizard, «Conferma BOM» (R5) con la prima
//     risposta persa: «da verificare», R6, «Confermata»; una conferma formato 3 e un esito_intento; le AR scritte da R5;
//   - F8: R5 di nuovo con la stessa chiave e lo stesso corpo (riproposto, niente scritto), poi con un corpo diverso (409);
//   - F9: il secondo riavvio vero: lo storico (R7), la conferma, le AR lette da S9 e la working uguali a prima;
//   - F10: un giro di sole letture, nessuna scrittura;
//   - F12: due sessioni (l'admin con il wizard della revisione aperto, un secondo admin in un altro browser): N1, N4, N2 o N5.
// Le fasi che non si possono fare su questo server, o parti di fase, la prova le dice NON ESEGUITE con il motivo (riga
// «NON ESEGUITA» nel log), mai finte: l'elenco è nel resoconto della corsia interfaccia.
//
// Clienti, codici e nomi dei file sono inventati (ACME, 912xxxx, acme.example): il repository è pubblico.

package web

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// ---------------------------------------------------------------- il processo del server

// processoD1: un processo del banco (cockpit.exe, il worker di analisi), con l'uscita in un file.
type processoD1 struct {
	nome   string
	cmd    *exec.Cmd
	log    string
	file   *os.File
	uscito chan struct{}
}

func avviaProcessoD1(t *testing.T, nome, dir, log string, env []string, prog string, arg ...string) *processoD1 {
	t.Helper()
	f, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(prog, arg...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		t.Fatalf("avvio di %s: %v", nome, err)
	}
	p := &processoD1{nome: nome, cmd: cmd, log: log, file: f, uscito: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.uscito)
	}()
	// prima della cartella temporanea (le Cleanup vanno all'indietro): su Windows un file aperto non si cancella
	t.Cleanup(func() { p.ferma() })
	return p
}

// ferma: Kill (TerminateProcess su Windows: come un guasto, niente chiusura ordinata), poi aspetta l'uscita.
func (p *processoD1) ferma() {
	select {
	case <-p.uscito:
	default:
		_ = p.cmd.Process.Kill()
		<-p.uscito
	}
	_ = p.file.Close()
}

func (p *processoD1) vivo() bool {
	select {
	case <-p.uscito:
		return false
	default:
		return true
	}
}

func (p *processoD1) coda(n int) string {
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

// serverD1: cockpit.exe con la sua configurazione; avvia e riavvia sempre lo stesso binario sullo stesso database.
type serverD1 struct {
	t        *testing.T
	exe, cfg string
	dir      string
	url      string
	porta    int
	proc     *processoD1
	avvii    int
	pid      []int
}

func (s *serverD1) avvia() {
	s.t.Helper()
	s.avvii++
	s.proc = avviaProcessoD1(s.t, fmt.Sprintf("server (avvio %d)", s.avvii), s.dir, filepath.Join(s.dir, fmt.Sprintf("server_%d.out", s.avvii)), nil, s.exe, "-config", s.cfg)
	s.pid = append(s.pid, s.proc.cmd.Process.Pid)
	pronto := time.Now().Add(120 * time.Second)
	for {
		r, err := http.Get(s.url + "/login")
		if err == nil {
			r.Body.Close()
			if r.StatusCode == http.StatusOK {
				return
			}
		}
		if !s.proc.vivo() {
			s.t.Fatalf("il server è uscito invece di partire:\n%s", s.proc.coda(80))
		}
		if time.Now().After(pronto) {
			s.t.Fatalf("il server non risponde su %s dopo 120 s\n%s", s.url, s.proc.coda(80))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// riavvia: il riavvio vero (PM23 §12.2, F6 e F9): Kill del processo, la porta di nuovo libera, un processo nuovo con la
// stessa configurazione e lo stesso database.
func (s *serverD1) riavvia() {
	s.t.Helper()
	vecchio := s.proc
	vecchio.ferma()
	if vecchio.vivo() {
		s.t.Fatal("il server non si è fermato")
	}
	libera := time.Now().Add(30 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", s.porta), 300*time.Millisecond)
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(libera) {
			s.t.Fatalf("la porta %d risponde ancora dopo il Kill del server", s.porta)
		}
		time.Sleep(200 * time.Millisecond)
	}
	s.avvia()
	if s.pid[len(s.pid)-1] == s.pid[len(s.pid)-2] {
		s.t.Fatalf("il riavvio ha lo stesso processo: %v", s.pid)
	}
}

func gettoneD1(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func portaLiberaD1(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// cockpitTomlD1: la configurazione del banco (PM23 §12.2 «Server vero»; U2 §3, IMP §5.4). Password e gettoni valgono
// per la durata della prova.
func cockpitTomlD1(porta int, dsn, radice, staging, regole, host, gAnalisi string) string {
	return fmt.Sprintf(`# banco della prova d1 della Distinta unica (distinta_unica_persistito_browser_test.go): tutto temporaneo
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
intervallo_sync_s = 3600
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
termini_cartiglio = ["scala"]

[staging]
automatico = false

[motore_a]
anteprima = true
regole = '%s'

[[utenti]]
sigla = "FP"
nome = "Operatore Prova"
ufficio = "Commerciale"
ruolo = "operatore"
password = "prova-fp"

[[utenti]]
sigla = "AD"
nome = "Admin Prova"
ufficio = "IT"
ruolo = "admin"
password = "prova-ad"

# la seconda sessione di F12 (la pagina della Distinta unica è dell'amministratore)
[[utenti]]
sigla = "AB"
nome = "Admin Secondo"
ufficio = "IT"
ruolo = "admin"
password = "prova-ab"

[[casella]]
indirizzo = "commerciale@azienda.example"
nome      = "Commerciale"
condivisa = true

[[postazione]]
nome_host   = %q
descrizione = "banco della prova d1"
utente      = "FP"

[[worker]]
nome       = "analisi@%s"
tipo       = "analisi"
token      = %q
postazione = %q
`, porta, dsn, radice, staging, regole, host, host, gAnalisi, host)
}

// regoleD1 scrive l'indice delle grammatiche del banco: la grammatica ACME di app/runtime/testdata/motore_a con l'UUID e
// la ragione sociale del cliente della prova, senza file dei casi (come mwRegoleACME, ma su disco per il server vero).
func regoleD1(t *testing.T, dir string, cliente uuid.UUID, ragione string) string {
	t.Helper()
	leggi := func(nome string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "app", "runtime", "testdata", "motore_a", nome))
		if err != nil {
			t.Fatalf("regole ACME: %v", err)
		}
		return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	}
	ind, err := grammatica.LeggiIndice(leggi("indice_acme.v1.json"))
	if err != nil {
		t.Fatalf("regole ACME: l'indice non si legge: %v", err)
	}
	var g map[string]any
	dec := json.NewDecoder(bytes.NewReader(leggi("acme.v1.json")))
	dec.UseNumber()
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	cl := g["cliente"].(map[string]any)
	cl["id"], cl["ragione_sociale"] = cliente.String(), ragione
	gram, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ind.Grammatiche = []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: jsoncanonico.Impronta(gram)}}
	ind.Casi = nil
	raw, err := json.MarshalIndent(ind, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acme.v1.json"), gram, 0o644); err != nil {
		t.Fatal(err)
	}
	indice := filepath.Join(dir, "indice_banco_d1.v1.json")
	if err := os.WriteFile(indice, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return indice
}

// ---------------------------------------------------------------- la scena

// scenaD1: la RFQ sintetica della prova (PM23 §12.2 «Dati sintetici», nella misura detta nel resoconto).
type scenaD1 struct {
	*rfqFascicolo
	b                   *bancoWeb
	srv                 *serverD1
	cliente             uuid.UUID
	prodotto, prodotto2 uuid.UUID
	al                  map[string]uuid.UUID
	sha                 map[string]string
	caricare            map[string]string // i file che il browser carica con «+ Aggiungi file» (F2): nome -> percorso
	ad                  *browser
	dir                 string
	radice              string
	staging             string
	foto                string
}

// fattiD1: lo STEP v3 del prodotto 9120001 (PM23 §12.2): la radice, il sottoassieme 9120010 (×2), quattro pezzi
// (9120020 ×4 e 9120030 sotto l'assieme, 9120040 ×2 sotto la radice, 9120060 sotto WELDED SUPPORT), tre foglie della
// forma «…_Saldatura d'angolo …_oa_…» (positivi LS-02) e il nodo non foglia «WELDED SUPPORT» (negativo).
func fattiD1() string {
	return fattiDi("#1", []string{"#1=9120001A", "#2=9120010A", "#3=9120020A", "#4=9120030A", "#5=9120040A", "#6=WELDED SUPPORT",
		"#7=9120060A", "#8=ACME_111_2.000_Saldatura d'angolo 1_oa_0", "#9=ACME_111_2.000_Saldatura d'angolo 2_oa_0",
		"#10=ACME_111_2.000_Saldatura d'angolo 3_oa_0"},
		[]string{"#1>#2*2", "#2>#3*4", "#2>#4*1", "#1>#5*2", "#1>#6*1", "#6>#7*1", "#6>#8*1", "#6>#9*1", "#2>#10*1"})
}

// fattiD1r2: il secondo STEP del prodotto (il cambio della fonte in F6): 9120030 ×2 e senza 9120040.
func fattiD1r2() string {
	return fattiDi("#1", []string{"#1=9120001A", "#2=9120010A", "#3=9120020A", "#4=9120030A"}, []string{"#1>#2*2", "#2>#3*4", "#2>#4*2"})
}

// fattiD1p2: lo STEP del secondo prodotto finito 9130001 (F6: «Riparti dalla proposta», «Abbandona la bozza»).
func fattiD1p2() string {
	return fattiDi("#1", []string{"#1=9130001A", "#2=9130010A"}, []string{"#1>#2*3"})
}

// analisiD1 scrive i fatti correnti di un contenuto con l'analizzatore che il server ha registrato all'avvio
// (analizzatore_corrente, da [analisi] del cockpit.toml), non con quello delle scene del pacchetto.
func (s *scenaD1) analisiD1(sha, fatti string) {
	s.b.t.Helper()
	s.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti)
		SELECT $1, versione_analizzatore, hash_configurazione, $2 FROM analizzatore_corrente`, sha, fatti)
}

// fileVero mette un contenuto dato nella cache per contenuto dello staging del server (staging.PercorsoContenuto: è lì
// che S5 e il ciclo dei file lo cercano) e ne fa un allegato del messaggio della RFQ.
func (s *scenaD1) fileVero(nome, ext string, contenuto []byte, origine string) uuid.UUID {
	s.b.t.Helper()
	s.n++
	sha, n := shaDi(contenuto), len(contenuto)
	percorso, err := staging.PercorsoContenuto(s.staging, sha, nome)
	if err != nil {
		s.b.t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err != nil {
		s.b.t.Fatal(err)
	}
	if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
		s.b.t.Fatal(err)
	}
	var id uuid.UUID
	if err := s.b.pool.QueryRow(s.b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,$2,$3,$4,'file',$5,$6,$7,$8,'analizzato',now()) RETURNING allegato_id`,
		s.msg, s.n, nome, ext, origine, n, sha, percorso).Scan(&id); err != nil {
		s.b.t.Fatal(err)
	}
	s.al[nome], s.sha[nome] = id, sha
	return id
}

// stepD1: uno STEP della RFQ, letto dall'analizzatore corrente, proposto come 3D del prodotto (mai confermato).
func (s *scenaD1) stepD1(nome, codice, fatti string) {
	s.b.t.Helper()
	a := s.fileVero(nome, "stp", []byte("ISO-10303-21;\n/* STEP sintetico della prova d1: "+nome+" */\nEND-ISO-10303-21;\n"), "outlook")
	s.analisiD1(s.sha[nome], fatti)
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', $3, 80, 'nome_file')`,
		a, s.thread, codice)
}

// pdfD1: un PDF vero (una tavola) con la sua proposta di 2D; codice vuoto = una proposta senza codice.
func (s *scenaD1) pdfD1(nome, cartiglio, codice string) {
	s.b.t.Helper()
	a := s.fileVero(nome, "pdf", pdfTavola(cartiglio), "outlook")
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', NULLIF($3, ''), 85, 'nome_file')`,
		a, s.thread, codice)
}

func (b *bancoWeb) scenaD1(t *testing.T, srv *serverD1, cliente uuid.UUID, dir, radice, staging string) *scenaD1 {
	t.Helper()
	b.eseguiD1(`INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ($1, 'ACME', 'Acme S.p.A.')`, cliente)
	b.eseguiD1(`INSERT INTO dominio_cliente (dominio, cliente_id) VALUES ('acme.example', $1)`, cliente)
	r := b.rfqFascicolo(cliente, "D1V1")
	r.staging = staging
	s := &scenaD1{rfqFascicolo: r, b: b, srv: srv, cliente: cliente, al: map[string]uuid.UUID{}, sha: map[string]string{},
		caricare: map[string]string{}, dir: dir, radice: radice, staging: staging, foto: os.Getenv("COCKPIT_FOTO_DIR")}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("9120001", "finito")
	s.prodotto2 = r.componenteTipo("9130001", "finito")
	s.stepD1("9120001.stp", "9120001", fattiD1())
	s.stepD1("9120001_r2.stp", "9120001", fattiD1r2())
	s.stepD1("9130001.stp", "9130001", fattiD1p2())
	// tre PDF: uno con il codice di un pezzo, uno con un codice che lo STEP non ha (in conflitto dove lo si preassocia),
	// uno senza codice
	s.pdfD1("9120020A_1.pdf", "9120020", "9120020A")
	s.pdfD1("9120099A_1.pdf", "9120099", "9120099A")
	s.pdfD1("NOTA_TECNICA.pdf", "NOTA", "")
	// il disegno del pezzo del secondo prodotto: la PB iniziale della sua bozza (F6, «Riparti» e «Abbandona»)
	s.pdfD1("9130010A_1.pdf", "9130010", "9130010A")
	// i file che il browser carica in F2: lo zip con due voci, un DXF, un file di collaudo del cliente (origine portale)
	car := filepath.Join(dir, "da_caricare")
	if err := os.MkdirAll(car, 0o755); err != nil {
		t.Fatal(err)
	}
	var z bytes.Buffer
	zw := zip.NewWriter(&z)
	for _, v := range []struct{ nome, codice string }{{"9120030A_1.pdf", "9120030"}, {"9120040A_1.pdf", "9120040"}} {
		w, err := zw.Create(v.nome)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(pdfTavola(v.codice)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for nome, c := range map[string][]byte{
		"PACCHETTO_9120001.zip": z.Bytes(),
		"9120001A_SVILUPPO.dxf": []byte("0\nSECTION\n2\nENTITIES\n0\nLINE\n8\n0\n10\n0\n20\n0\n11\n120\n21\n60\n0\nENDSEC\n0\nEOF\n"),
		"COLLAUDO_PROVA.pdf":    pdfTavola("COLLAUDO"),
		// il documento che la seconda sessione carica con il wizard aperto (F12): un contenuto nuovo (l'analisi è per
		// contenuto: uno sha già analizzato non va in analisi)
		"9120010A_1.pdf": pdfTavola("9120010"),
	} {
		p := filepath.Join(car, nome)
		if err := os.WriteFile(p, c, 0o644); err != nil {
			t.Fatal(err)
		}
		s.caricare[nome] = p
	}
	return s
}

func (b *bancoWeb) eseguiD1(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

// rinomina: dopo il caricamento (F2) gli allegati nuovi entrano nella mappa per nome.
func (s *scenaD1) rileggiAllegati() {
	s.b.t.Helper()
	righe, err := s.b.pool.Query(s.b.ctx, `SELECT a.nome_file, a.allegato_id, coalesce(a.sha256, '') FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.thread_id = $1 AND a.messaggio_id IS NOT NULL`, s.thread)
	if err != nil {
		s.b.t.Fatal(err)
	}
	defer righe.Close()
	for righe.Next() {
		var nome, sha string
		var id uuid.UUID
		if err := righe.Scan(&nome, &id, &sha); err != nil {
			s.b.t.Fatal(err)
		}
		s.al[nome], s.sha[nome] = id, sha
	}
}

// lancia: lo script del browser per una fase. Le righe «DATI {…}» dell'uscita tornano al test.
func (s *scenaD1) lancia(t *testing.T, fase string, extra map[string]any) (string, map[string]json.RawMessage) {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "v1_distinta.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	s.rileggiAllegati()
	nomi := map[string]string{}
	for k, v := range s.al {
		nomi[k] = v.String()
	}
	d := map[string]any{"thread": s.thread, "prodotto": s.prodotto, "prodotto2": s.prodotto2, "allegati": nomi, "caricare": s.caricare}
	for k, v := range extra {
		d[k] = v
	}
	dati, _ := json.Marshal(d)
	arg := []string{"-I", script, "--url", s.srv.url, "--sigla", "AD", "--password", "prova-ad", "--dati", string(dati), "--fase", fase}
	if s.foto != "" {
		arg = append(arg, "--foto", s.foto)
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
		t.Fatalf("la fase %s nel browser è fallita (%v):\n%s\n%s", fase, err, testo, s.srv.proc.coda(40))
	}
	t.Logf("fase %s nel browser:\n%s", fase, testo)
	out := map[string]json.RawMessage{}
	for _, riga := range strings.Split(testo, "\n") {
		riga = strings.TrimSpace(riga)
		if strings.HasPrefix(riga, "DATI ") {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(strings.TrimPrefix(riga, "DATI ")), &m); err != nil {
				t.Fatalf("la riga DATI della fase %s: %v", fase, err)
			}
			for k, v := range m {
				out[k] = v
			}
		}
	}
	return testo, out
}

// ---------------------------------------------------------------- le letture e le impronte

// fotoD1: ogni tabella dello schema, come fotoDelDatabase, meno quello che cambia con la sola presenza di chi guarda o
// del worker, non con la pagina: sessione (ogni accesso del browser ne apre una), worker_presenza (il worker di analisi
// vero la aggiorna a ogni claim) e, di utente, ultima_vista_inbox (l'accesso porta all'Inbox, che la aggiorna).
func fotoD1(t *testing.T, b *bancoWeb) map[string]string {
	t.Helper()
	f := fotoDelDatabase(t, b)
	delete(f, "sessione")
	delete(f, "worker_presenza")
	var u string
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-')
		FROM (SELECT utente_id, sigla, nome, ufficio, ruolo, password_hash, attivo, creato_il FROM utente) x`).Scan(&u); err != nil {
		t.Fatal(err)
	}
	f["utente"] = u
	return f
}

// jobD1: i lavori nella coda, per stato (nessun lavoro accodato = la stessa riga prima e dopo).
func jobD1(t *testing.T, b *bancoWeb) string {
	t.Helper()
	righe, err := b.pool.Query(b.ctx, `SELECT tipo::text || ':' || stato::text || ':' || count(*)::text FROM job GROUP BY tipo, stato ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer righe.Close()
	var out []string
	for righe.Next() {
		var s string
		if err := righe.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return strings.Join(out, " ")
}

// aspettaCodaFerma: nessun lavoro pronto o in corso (le copie sul NAS, le estrazioni, le analisi), entro il limite. Il
// sync della casella che il server accoda all'avvio resta «pronto»: nel banco non c'è il worker di Outlook (§12.2 vuole
// solo quello di analisi), e la capacità di scrittura su Outlook è spenta.
func aspettaCodaFerma(t *testing.T, b *bancoWeb, limite time.Duration) {
	t.Helper()
	fine := time.Now().Add(limite)
	for {
		if contaSQL(t, b, `SELECT count(*) FROM job WHERE stato IN ('pronto', 'in_corso') AND tipo <> 'sync_outlook'`) == 0 {
			return
		}
		if time.Now().After(fine) {
			t.Fatalf("la coda non si ferma in %v: %s", limite, jobD1(t, b))
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// leggiD1: una GET autenticata del browser della prova sul server vero; il corpo, con lo stato.
func (s *scenaD1) leggiD1(t *testing.T, percorso string) string {
	t.Helper()
	resp, testo := s.ad.chiediJSON(http.MethodGet, percorso, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %.400s", percorso, resp.StatusCode, testo)
	}
	return testo
}

// letturePersistite: quello che il server rilegge dal database, per il confronto prima e dopo un riavvio. R1 (la bozza
// del prodotto, con gli eventi), S9 (lo smistamento di tutti i prodotti: le righe con le PB, la loro provenienza e le AR
// registrate), S1 (i file della RFQ), R7 (lo storico delle conferme). Dalle risposte si tolgono solo i campi che dicono
// «adesso» (l'ora della lettura), elencati in campiAdessoD1.
func (s *scenaD1) letturePersistite(t *testing.T) map[string]string {
	t.Helper()
	th := "/thread/" + s.thread.String()
	out := map[string]string{
		"R1":      s.leggiD1(t, th+"/motore-a/bozza?prodotto="+s.prodotto.String()+"&eventi=1"),
		"S9":      s.leggiD1(t, th+"/motore-a/dati?sorgente=1"),
		"S1":      s.leggiD1(t, th+"/sorgente"),
		"R7":      s.leggiD1(t, th+"/motore-a/conferme?prodotto="+s.prodotto.String()),
		"dati":    s.leggiD1(t, th+"/motore-a/dati?prodotto="+s.prodotto.String()),
		"R1 (P2)": s.leggiD1(t, th+"/motore-a/bozza?prodotto="+s.prodotto2.String()+"&eventi=1"),
	}
	for k, v := range out {
		out[k] = senzaAdessoD1(t, k, v)
	}
	return out
}

// campiAdessoD1: i campi delle letture che dicono l'ora della lettura o del processo, non lo stato persistito.
var campiAdessoD1 = map[string]bool{"calcolato": true}

func senzaAdessoD1(t *testing.T, nome, testo string) string {
	t.Helper()
	var v any
	dec := json.NewDecoder(strings.NewReader(testo))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s non è JSON: %v\n%.300s", nome, err, testo)
	}
	var togli func(x any) any
	togli = func(x any) any {
		switch y := x.(type) {
		case map[string]any:
			for k, w := range y {
				if campiAdessoD1[k] {
					delete(y, k)
					continue
				}
				y[k] = togli(w)
			}
			return y
		case []any:
			for i := range y {
				y[i] = togli(y[i])
			}
			return y
		}
		return x
	}
	b, err := json.Marshal(togli(v))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// controparteNotaD1: il tipo di controparte del messaggio «nota» della RFQ (il contenitore dei caricamenti interni di «+
// Aggiungi file», UpsertNotaInterna), con la data della risoluzione: per il difetto noto della corsia file detto in
// normalizzaNotaD1.
func (s *scenaD1) controparteNotaD1(t *testing.T) (uuid.UUID, string) {
	t.Helper()
	var id uuid.UUID
	var tipo string
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT messaggio_id, controparte_tipo::text || ' (controparte_il ' || coalesce(controparte_il::text, 'NULL') || ')'
		FROM messaggio WHERE thread_id = $1 AND canale = 'nota'`, s.thread).Scan(&id, &tipo); err != nil {
		t.Fatalf("il messaggio dei caricamenti interni: %v", err)
	}
	return id, tipo
}

// normalizzaNotaD1: il DIFETTO NOTO di un'altra corsia che la prova d1 ha trovato al riavvio vero (F6), non della pagina.
// UpsertNotaInterna (platform/db/queries/schermata.sql, «-- name: UpsertNotaInterna») scrive il messaggio «nota» dei
// caricamenti con controparte_tipo = 'interno' ma senza controparte_il; all'avvio RicalcolaControparti
// (core/inbox/ingest/controparte.go) prende i messaggi con controparte_il NULL (ListMessaggiSenzaControparte,
// fornitori.sql) e lo riclassifica dal mittente, che non c'è: 'sconosciuto'. Cambia quindi, con il solo riavvio, il motivo
// del gesto di quel messaggio nella valutazione della richiesta di /dati («controparte «interno»» → «controparte
// «sconosciuto»») e con lui l'impronta degli ingressi dei prodotti. Lo stato e la bozza no. La prova lo dice nel log e,
// SOLO quando il database mostra quel passaggio, confronta /dati senza quei tre campi: ogni altra differenza resta un
// errore.
func normalizzaNotaD1(t *testing.T, testo string, msg uuid.UUID) string {
	t.Helper()
	var v map[string]any
	dec := json.NewDecoder(strings.NewReader(testo))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("/dati non è JSON: %v", err)
	}
	if e, ok := v["esito"].(map[string]any); ok {
		if r, ok := e["richiesta"].(map[string]any); ok {
			if gg, ok := r["gesti"].([]any); ok {
				for _, x := range gg {
					if g, ok := x.(map[string]any); ok && g["messaggio_id"] == msg.String() {
						if m, _ := g["motivo"].(string); strings.HasPrefix(m, "controparte «") {
							g["motivo"] = "controparte della nota dei caricamenti (difetto noto: riclassificata all'avvio)"
						}
					}
				}
			}
		}
		if p, ok := e["prodotti"].(map[string]any); ok {
			delete(p, "hash_ingresso")
			delete(p, "impronta")
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// confrontaLetture: le letture di prima e di dopo un riavvio, uguali; per ognuna diversa, la prima differenza.
func confrontaLetture(t *testing.T, quando string, prima, dopo map[string]string) {
	t.Helper()
	var nomi []string
	for k := range prima {
		nomi = append(nomi, k)
	}
	sort.Strings(nomi)
	for _, k := range nomi {
		if prima[k] == dopo[k] {
			continue
		}
		a, b := prima[k], dopo[k]
		i := 0
		for i < len(a) && i < len(b) && a[i] == b[i] {
			i++
		}
		da := i - 150
		if da < 0 {
			da = 0
		}
		fa, fb := i+250, i+250
		if fa > len(a) {
			fa = len(a)
		}
		if fb > len(b) {
			fb = len(b)
		}
		t.Errorf("%s: %s diversa dopo il riavvio (byte %d)\n  prima: …%s…\n  dopo:  …%s…", quando, k, i, a[da:fa], b[da:fb])
		// le due letture intere, per guardarle (COCKPIT_FOTO_DIR, la cartella delle schermate della prova)
		if dir := os.Getenv("COCKPIT_FOTO_DIR"); dir != "" {
			nome := strings.NewReplacer(" ", "_", "(", "", ")", "").Replace(quando + "_" + k)
			_ = os.WriteFile(filepath.Join(dir, nome+"_prima.json"), []byte(a), 0o644)
			_ = os.WriteFile(filepath.Join(dir, nome+"_dopo.json"), []byte(b), 0o644)
		}
	}
}

// snapshotD1: l'ultima versione salvata della bozza aperta (o confermata) di un prodotto, com'è nel database.
func (s *scenaD1) snapshotD1(t *testing.T, prodotto uuid.UUID) (uuid.UUID, int, string, string) {
	t.Helper()
	var bozza uuid.UUID
	var versione int
	var stato, snap string
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT b.bozza_id, v.versione, b.stato::text, v.snapshot::text FROM distinta_bozza b
		JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id WHERE b.thread_id = $1 AND b.prodotto_id = $2
		ORDER BY b.creata_il DESC, v.versione DESC LIMIT 1`, s.thread, prodotto).Scan(&bozza, &versione, &stato, &snap); err != nil {
		t.Fatalf("la bozza di %v: %v", prodotto, err)
	}
	return bozza, versione, stato, snap
}

// controllaFileF5: dopo i gesti dei file (F5), che cosa dice il database e che cosa c'è sul disco.
func (s *scenaD1) controllaFileF5(t *testing.T, r2 *rfqFascicolo, fonte0 int) {
	t.Helper()
	b := s.b
	conta := func(sql string, arg ...any) int { t.Helper(); return contaSQL(t, b, sql, arg...) }
	statoFile := func(nome string) string {
		t.Helper()
		var st string
		if err := b.pool.QueryRow(b.ctx, `SELECT coalesce((SELECT stato FROM file_rfq WHERE thread_id = $1 AND allegato_id = $2), 'attivo (nessuna riga)')`,
			s.thread, s.al[nome]).Scan(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	for nome, atteso := range map[string]string{"9120001A_SVILUPPO.dxf": "attivo", "COLLAUDO_PROVA.pdf": "scartato", "9120040A_1.pdf": "scartato",
		"9120020A_1.pdf": "attivo (nessuna riga)", "PACCHETTO_9120001.zip": "attivo (nessuna riga)"} {
		if st := statoFile(nome); st != atteso {
			t.Errorf("F5: %s è %q nel registro dei file, atteso %q", nome, st, atteso)
		}
	}
	dxf := s.al["9120001A_SVILUPPO.dxf"]
	if ev := righeD1(t, b, `SELECT azione || ' ' || coalesce(stato_prima, '-') || '>' || coalesce(stato_dopo, '-') FROM file_rfq_evento
		WHERE thread_id = $1 AND allegato_id = $2 AND azione IN ('altri_file', 'riattiva') ORDER BY evento_id`, s.thread, dxf); ev != "altri_file attivo>altri_file\nriattiva altri_file>attivo" {
		t.Errorf("F5: gli eventi del DXF: %q", ev)
	}
	// la copia nella cartella ALTRO della RFQ sul NAS temporaneo: verificata con lo sha, e dopo «Riattiva» trattenuta (U2
	// §3.11, base B di V1-Q25)
	var rel, cart, statoCons string
	if err := b.pool.QueryRow(b.ctx, `SELECT c.percorso_relativo, coalesce(c.cartella_thread, t.cartella_relativa), c.stato FROM file_rfq_conservazione c
		JOIN thread_offerta t ON t.thread_id = c.thread_id WHERE c.thread_id = $1 AND c.allegato_id = $2 ORDER BY c.conservazione_id DESC LIMIT 1`,
		s.thread, dxf).Scan(&rel, &cart, &statoCons); err != nil {
		t.Errorf("F5: la copia di conservazione del DXF: %v", err)
	} else {
		p := filepath.Join(s.radice, filepath.FromSlash(strings.ReplaceAll(cart, `\`, "/")), filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
		if sha, _, err := nas.Sha256File(p); err != nil || sha != s.sha["9120001A_SVILUPPO.dxf"] {
			t.Errorf("F5: la copia del DXF in ALTRO (%s): %v, sha %s", p, err, sha)
		}
		if statoCons != "trattenuta" {
			t.Errorf("F5: la copia in ALTRO del DXF riattivato è %q, attesa «trattenuta»", statoCons)
		}
	}
	// dopo «Riattiva»: i byte nello staging con lo sha giusto; nessun documento, nessuno STEP strutturale, nessun evento
	// della sorgente
	var ps string
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(path_staging, '') FROM allegato WHERE allegato_id = $1`, dxf).Scan(&ps); err != nil {
		t.Fatal(err)
	}
	if sha, _, err := nas.Sha256File(ps); err != nil || sha != s.sha["9120001A_SVILUPPO.dxf"] {
		t.Errorf("F5: il DXF riattivato nello staging (%s): %v, sha %s", ps, err, sha)
	}
	if n := conta(`SELECT count(*) FROM documento WHERE thread_id = $1 AND sha256 = $2`, s.thread, s.sha["9120001A_SVILUPPO.dxf"]); n != 0 {
		t.Errorf("F5: documenti del DXF dopo «Riattiva»: %d", n)
	}
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1`, s.thread); n != fonte0 {
		t.Errorf("F5: eventi della sorgente scritti dai gesti dei file: %d -> %d", fonte0, n)
	}
	if n := conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND step_strutturale_id IS NOT NULL`, s.thread); n != 0 {
		t.Errorf("F5: STEP strutturali scritti dai gesti dei file: %d", n)
	}
	// lo scarto del collaudo: la rimozione dallo staging; l'altra RFQ con lo stesso contenuto non perde i byte
	t.Logf("F5: rimozioni dallo staging: %s", strings.ReplaceAll(righeD1(t, b, `SELECT a.nome_file || ' ' || r.stato || '/' || r.motivo FROM file_rfq_rimozione r
		JOIN allegato a ON a.allegato_id = r.allegato_id WHERE r.thread_id = $1 ORDER BY 1`, s.thread), "\n", "; "))
	var ps2, sha2 string
	if err := b.pool.QueryRow(b.ctx, `SELECT a.path_staging, a.sha256 FROM allegato a WHERE a.messaggio_id = $1`, r2.msg).Scan(&ps2, &sha2); err != nil {
		t.Fatal(err)
	}
	if sha, _, err := nas.Sha256File(ps2); err != nil || sha != sha2 {
		t.Errorf("F5: l'altra RFQ con lo stesso contenuto ha perso i byte del collaudo (%s): %v", ps2, err)
	}
}

// idD1: gli allegato_id dei file dati per nome.
func idD1(s *scenaD1, nomi []string) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(nomi))
	for _, n := range nomi {
		out = append(out, s.al[n])
	}
	return out
}

// righeD1: le righe di una query, una stringa per riga (per confrontare tabelle intere prima e dopo).
func righeD1(t *testing.T, b *bancoWeb, sql string, arg ...any) string {
	t.Helper()
	rr, err := b.pool.Query(b.ctx, sql, arg...)
	if err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	defer rr.Close()
	var out []string
	for rr.Next() {
		var s string
		if err := rr.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

// statoD1: le tabelle dello stato persistito della V1 (bozza formato 3, sorgente S8, PB e provenienza, conferma R5, AR),
// riga per riga, per il confronto prima e dopo un riavvio.
func (s *scenaD1) statoD1(t *testing.T) map[string]string {
	t.Helper()
	b := s.b
	return map[string]string{
		"distinta_bozza": righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM distinta_bozza WHERE thread_id = $1 ORDER BY bozza_id) x`, s.thread),
		"distinta_bozza_versione": righeD1(t, b, `SELECT v.bozza_id::text || ' v' || v.versione || ' ' || md5(v.snapshot::text) FROM distinta_bozza_versione v
			JOIN distinta_bozza d ON d.bozza_id = v.bozza_id WHERE d.thread_id = $1 ORDER BY 1`, s.thread),
		"fonte_step_evento":     righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM fonte_step_evento WHERE thread_id = $1 ORDER BY 1) x`, s.thread),
		"file_rfq_associazione": righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM file_rfq_associazione ORDER BY associazione_id) x`),
		"associazione_evento":   righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM associazione_evento ORDER BY 1) x`),
		"conferma_strutturale":  righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM conferma_strutturale WHERE thread_id = $1 ORDER BY 1) x`, s.thread),
		"componente":            righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM componente WHERE thread_id = $1 ORDER BY componente_id) x`, s.thread),
		"componente_relazione":  righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM componente_relazione WHERE thread_id = $1 ORDER BY 1) x`, s.thread),
		"documento":             righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM documento WHERE thread_id = $1 ORDER BY documento_id) x`, s.thread),
	}
}

// ---------------------------------------------------------------- la prova

func TestV1DistintaUnicaPercorsoPersistito(t *testing.T) {
	if os.Getenv("COCKPIT_TEST_SENZA_PYTHON") != "" {
		t.Skip("COCKPIT_TEST_SENZA_PYTHON: il worker vero e il browser non vengono avviati, prova non verificata")
	}
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	goexe, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go non trovato: serve per compilare cockpit.exe: %v", err)
	}
	dsn := testutil.DSN(t)
	pool := testutil.Pool(t)
	testutil.SchemaVuoto(t, pool) // le migrazioni le applica il server all'avvio (§12.2: testutil.SchemaVuoto)
	ctx := context.Background()

	pkg, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workers, err := filepath.Abs(filepath.Join(pkg, "..", "..", "..", "workers"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	radice := filepath.Join(dir, "nas", "PREVENTIVI DA FARE")
	staging := filepath.Join(dir, "staging")
	wAnalisi := filepath.Join(dir, "w_analisi")
	for _, d := range []string{radice, staging, wAnalisi} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cliente := uuid.New()
	indice := regoleD1(t, filepath.Join(dir, "regole"), cliente, "Acme S.p.A.")

	exe := filepath.Join(dir, "cockpit_d1.exe")
	if out, err := exec.Command(goexe, "build", "-o", exe, filepath.Join(pkg, "..", "..", "..", "cmd", "cockpit")).CombinedOutput(); err != nil {
		t.Fatalf("go build di cockpit.exe: %v\n%s", err, out)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	host = strings.ToUpper(host)
	gAnalisi := gettoneD1(t)
	porta := portaLiberaD1(t)
	srv := &serverD1{t: t, exe: exe, cfg: filepath.Join(dir, "cockpit.toml"), dir: dir, porta: porta, url: fmt.Sprintf("http://127.0.0.1:%d", porta)}
	if err := os.WriteFile(srv.cfg, []byte(cockpitTomlD1(porta, dsn, radice, staging, indice, host, gAnalisi)), 0o600); err != nil {
		t.Fatal(err)
	}

	// ---- F0: avvio; il server migra da solo; nessuna bozza
	srv.avvia()
	b := &bancoWeb{t: t, ctx: ctx, pool: pool, q: db.New(pool), srv: &httptest.Server{URL: srv.url}}
	conta := func(sql string, arg ...any) int { t.Helper(); return contaSQL(t, b, sql, arg...) }
	var versione int
	var variante string
	if err := pool.QueryRow(ctx, `SELECT (SELECT max(versione) FROM schema_versione), coalesce((SELECT variante FROM schema_variante WHERE versione = 23), '')`).Scan(&versione, &variante); err != nil {
		t.Fatalf("F0: lo schema dopo l'avvio: %v", err)
	}
	if versione != 23 || variante != "0023-v1-formato3-file-20261019d" {
		t.Fatalf("F0: lo schema migrato dal server è alla %d con la variante %q", versione, variante)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza`); n != 0 {
		t.Fatalf("F0: bozze dopo l'avvio: %d", n)
	}
	s := b.scenaD1(t, srv, cliente, dir, radice, staging)
	s.ad = b.browser("127.0.0.1")
	s.ad.login("AD", "prova-ad")
	worker := avviaProcessoD1(t, "worker analisi", workers, filepath.Join(dir, "analisi.out"),
		[]string{"COCKPIT_URL=" + srv.url, "COCKPIT_TOKEN=" + gAnalisi, "COCKPIT_WORKER_ID=analisi@" + host, "COCKPIT_STAGING=" + wAnalisi,
			"PYTHONIOENCODING=utf-8", "PYTHONUTF8=1"},
		python, "worker_analisi.py", "--config", filepath.Join(dir, "nessun-worker.toml"))
	logDi := func() string { return srv.proc.coda(60) + "\n" + worker.coda(30) }
	defer func() {
		if t.Failed() {
			t.Logf("log del banco:\n%s", logDi())
		}
	}()
	t.Logf("F0 ESEGUITA: cockpit.exe (pid %d) ha migrato lo schema vuoto alla 23 (%s); nessuna bozza", srv.pid[0], variante)
	t.Log("F0 «posta finta → crea RFQ» NON ESEGUITA: la RFQ sintetica la scrive la prova (come TestDistintaUnicaServerVeroNelBrowser); il percorso dalla posta alla RFQ è di TestL7LoZipDallaPostaAlFascicoloConfermato (cmd/cockpit)")
	aspettaCodaFerma(t, b, 60*time.Second)

	// ---- F1: apertura senza scritture
	prima, jobPrima := fotoD1(t, b), jobD1(t, b)
	s.lancia(t, "F1", nil)
	aspettaCodaFerma(t, b, 30*time.Second)
	if d := differenze(prima, fotoD1(t, b)); d != "" {
		t.Fatalf("F1: aprire la pagina, guardare due STEP, aprire un PDF e leggere S9 ha scritto in: %s", d)
	}
	if j := jobD1(t, b); j != jobPrima {
		t.Fatalf("F1: lavori accodati dall'apertura: %s -> %s", jobPrima, j)
	}

	working := func() string {
		return righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM componente WHERE thread_id = $1 ORDER BY componente_id) x`, s.thread) + "\n" +
			righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM componente_relazione WHERE thread_id = $1 ORDER BY 1) x`, s.thread)
	}
	w0 := working()

	// ---- F2: «+ Aggiungi file»: lo zip (estratto dal server), il DXF e il collaudo; le analisi del worker vero
	s.lancia(t, "F2", nil)
	aspettaCodaFerma(t, b, 90*time.Second)
	s.rileggiAllegati()
	caricati := []string{"PACCHETTO_9120001.zip", "9120001A_SVILUPPO.dxf", "COLLAUDO_PROVA.pdf", "9120030A_1.pdf", "9120040A_1.pdf"}
	for _, n := range caricati {
		if s.al[n] == uuid.Nil {
			t.Fatalf("F2: l'allegato %s non c'è dopo il caricamento", n)
		}
	}
	if n := conta(`SELECT count(*) FROM allegato WHERE contenitore_id = $1`, s.al["PACCHETTO_9120001.zip"]); n != 2 {
		t.Errorf("F2: voci dello zip estratte dal server: %d", n)
	}
	if n := conta(`SELECT count(*) FROM documento_proposta WHERE allegato_id = ANY($1) AND componente_id IS NOT NULL`, idD1(s, caricati)); n != 0 {
		t.Errorf("F2: file caricati già su un componente: %d proposte", n)
	}
	if n := conta(`SELECT count(*) FROM documento WHERE sha256 IN (SELECT sha256 FROM allegato WHERE allegato_id = ANY($1))`, idD1(s, caricati)); n != 0 {
		t.Errorf("F2: documenti dai file caricati: %d", n)
	}
	if n := conta(`SELECT count(*) FROM documento_proposta WHERE allegato_id = ANY($1)`, idD1(s, []string{"9120030A_1.pdf", "9120040A_1.pdf"})); n != 2 {
		t.Errorf("F2: proposte di documento delle voci dello zip dopo l'analisi: %d", n)
	}
	if w := working(); w != w0 {
		t.Error("F2: il caricamento ha cambiato la working")
	}
	// l'altra RFQ sintetica, con lo stesso contenuto (lo stesso sha e lo stesso posto nella cache) del collaudo
	r2 := b.rfqFascicolo(cliente, "D1V2")
	b.eseguiD1(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		SELECT $1, 1, 'COLLAUDO_PROVA.pdf', 'pdf', 'file', 'outlook', bytes, sha256, path_staging, 'analizzato', now() FROM allegato WHERE allegato_id = $2`,
		r2.msg, s.al["COLLAUDO_PROVA.pdf"])

	// ---- F3: S8 crea la bozza formato 3 dal server; working invariata; la copia dello STEP sul NAS
	s.lancia(t, "F3", nil)
	aspettaCodaFerma(t, b, 60*time.Second)
	bozza, ver, stato, _ := s.snapshotD1(t, s.prodotto)
	if stato != "aperta" {
		t.Errorf("F3: la bozza di 9120001 dopo S8 è %s", stato)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND formato = 3`, bozza); n != 1 || ver != 1 {
		t.Errorf("F3: la bozza dopo S8 non è formato 3 versione 1: formato 3 = %d, versione %d", n, ver)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND fonte_allegato_id = $2`, bozza, s.al["9120001.stp"]); n == 0 {
		t.Error("F3: la bozza non ha lo STEP 9120001.stp come sorgente")
	}
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'conferma_documento'`, s.thread, s.prodotto); n != 1 {
		t.Errorf("F3: conferme S8 della sorgente: %d", n)
	}
	if w := working(); w != w0 {
		t.Errorf("F3: S8 ha cambiato la working (componente, componente_relazione)")
	}
	if n := conta(`SELECT count(*) FROM componente WHERE componente_id = $1 AND step_strutturale_id IS NOT NULL`, s.prodotto); n != 0 {
		t.Error("F3: S8 ha scritto lo STEP strutturale del prodotto (lo fa solo R5)")
	}
	var relNas, cartella string
	if err := pool.QueryRow(ctx, `SELECT coalesce(d.path_relativo, ''), coalesce(t.cartella_relativa, '') FROM documento d JOIN thread_offerta t ON t.thread_id = d.thread_id
		WHERE d.thread_id = $1 AND d.sha256 = $2 AND d.stato_nas = 'scritto'`, s.thread, s.sha["9120001.stp"]).Scan(&relNas, &cartella); err != nil {
		t.Errorf("F3: lo STEP sorgente sul NAS (documento scritto): %v\n%s", err, jobD1(t, b))
	} else {
		p := filepath.Join(radice, filepath.FromSlash(strings.ReplaceAll(cartella, `\`, "/")), filepath.FromSlash(strings.ReplaceAll(relNas, `\`, "/")))
		if sha, _, err := nas.Sha256File(p); err != nil || sha != s.sha["9120001.stp"] {
			t.Errorf("F3: la copia sul NAS di 9120001.stp (%s): %v, sha %s", p, err, sha)
		}
	}
	t.Log("F3 «Autorizza» NON ESEGUITA: superata per la pagina nuova dalla direttiva 39 (PM23 §10, «Le prove che cambiano l'atteso»)")

	// ---- F4: le modifiche della bozza; working e proposte invariate
	proposte := func() string {
		return righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM documento_proposta WHERE thread_id = $1 ORDER BY proposta_id) x`, s.thread) + "\n" +
			righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT p.* FROM componente_proposta p JOIN allegato a USING (allegato_id) JOIN messaggio m USING (messaggio_id)
				WHERE m.thread_id = $1 ORDER BY 1) x`, s.thread)
	}
	p0 := proposte()
	_, d4 := s.lancia(t, "F4", nil)
	pagina := d4["pagina"]
	if w := working(); w != w0 {
		t.Error("F4: le modifiche della bozza hanno cambiato la working")
	}
	if p := proposte(); p != p0 {
		t.Error("F4: le modifiche della bozza hanno cambiato le proposte")
	}
	if n := conta(`SELECT count(DISTINCT dopo->>'azione_id') FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'decisione_aggiunta' AND dopo->>'tipo' = 'non_includere'`, bozza); n != 1 {
		t.Errorf("F4: i «Non includere» del gesto hanno %d azione_id (atteso uno solo)", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'annulla'`, bozza); n == 0 {
		t.Error("F4: nessun evento «annulla» dopo «Annulla»")
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'file_lasciato' AND dopo->>'allegato_id' = $2`, bozza, s.al["NOTA_TECNICA.pdf"].String()); n != 1 {
		t.Errorf("F4: eventi file_lasciato di NOTA_TECNICA.pdf: %d", n)
	}
	t.Logf("F4: chiavi della bozza %s; «Accetta le proposte (N)» %s", d4["chiavi"], d4["accetta_le_proposte"])

	// ---- F5: i file; il NAS temporaneo e ALTRO; la rimozione dallo staging; l'altra RFQ con lo stesso contenuto
	fonte0 := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1`, s.thread)
	_, d5 := s.lancia(t, "F5", nil)
	pagina = d5["pagina"]
	aspettaCodaFerma(t, b, 60*time.Second)
	s.controllaFileF5(t, r2, fonte0)
	t.Logf("F5: unica copia chiesta per il collaudo: %s; scarto del PDF preassociato: %s; Altri file dello zip con la voce preassociata: %s",
		d5["unica_copia"], d5["rifiuto_pdf"], d5["rifiuto_zip"])
	t.Log("F5 «il custode della cache con l'orologio avanti» NON ESEGUITA: Cache.Adesso si sposta solo nel processo del banco web (AF-02); con cockpit.exe come processo l'orologio del custode non si tocca")

	// ---- F6: il riavvio vero; tutto si rilegge uguale
	aspettaCodaFerma(t, b, 30*time.Second)
	_, _, _, snapPrima := s.snapshotD1(t, s.prodotto)
	letture1, stato1 := s.letturePersistite(t), s.statoD1(t)
	nota, cpPrima := s.controparteNotaD1(t)
	srv.riavvia()
	t.Logf("F6: riavvio vero: pid %d fermato con Kill, pid %d avviato con la stessa configurazione e lo stesso database", srv.pid[0], srv.pid[1])
	confrontaLetture(t, "F6", stato1, s.statoD1(t))
	letture2 := s.letturePersistite(t)
	if _, cpDopo := s.controparteNotaD1(t); cpDopo != cpPrima {
		t.Logf("F6: DIFETTO NOTO di un'altra corsia (non della pagina): il riavvio riclassifica la controparte del messaggio «nota» dei caricamenti: %s -> %s (UpsertNotaInterna senza controparte_il, RicalcolaControparti all'avvio); /dati si confronta senza il motivo di quel gesto e l'impronta degli ingressi dei prodotti", cpPrima, cpDopo)
		letture1["dati"], letture2["dati"] = normalizzaNotaD1(t, letture1["dati"], nota), normalizzaNotaD1(t, letture2["dati"], nota)
	}
	confrontaLetture(t, "F6", letture1, letture2)
	if _, _, _, snap := s.snapshotD1(t, s.prodotto); snap != snapPrima {
		t.Error("F6: lo snapshot della bozza nel database è cambiato con il riavvio")
	}
	_, d6 := s.lancia(t, "F6", nil)
	if string(pagina) == "" || string(pagina) != string(d6["pagina"]) {
		t.Errorf("F6: la pagina riaperta dopo il riavvio non ridisegna la stessa bozza:\n  prima: %s\n  dopo:  %s", pagina, d6["pagina"])
	}
	rimozioni := righeD1(t, b, `SELECT a.nome_file || ' ' || r.stato || '/' || r.motivo FROM file_rfq_rimozione r JOIN allegato a ON a.allegato_id = r.allegato_id
		WHERE r.thread_id = $1 ORDER BY 1`, s.thread)
	t.Logf("F6: rimozioni dallo staging dopo il riavvio: %s", strings.ReplaceAll(rimozioni, "\n", "; "))
	if strings.Contains(rimozioni, "in_attesa") {
		t.Log("F6 «una rimozione rimasta in_attesa si completa dopo l'avvio» NON ESEGUITA: la rimozione resta in_attesa con il motivo file_giovane (la guardia dell'età del file nello staging); il processo cockpit.exe non ha un orologio da spostare (AF-02 è del banco web)")
	}
	if t.Failed() {
		return
	}

	// ---- F6 (seguito): «Usa nuovo STEP per 9120001…» con lo STEP rev 2 prima di R5 (giro G39-ter; G39-03 [U] e messaggio
	// 43 caso 1 [U]): la bozza e' nata dalla sorgente S8 di F3 e la BOM non e' confermata, quindi S8 la sostituirebbe
	// (non piu' il cambio della fonte sulla stessa bozza): la bozza di F4 abbandonata, la revoca della sorgente di F3, la
	// bozza nuova dalla proposta dello STEP rev 2 con l'evento riassegnazione_sorgente; F7 conferma la bozza nuova
	_, verPrima, _, _ := s.snapshotD1(t, s.prodotto)
	_, d6b := s.lancia(t, "F6b", nil)
	aspettaCodaFerma(t, b, 60*time.Second)
	bozzaR2, verR2, statoR2, _ := s.snapshotD1(t, s.prodotto)
	if bozzaR2 == bozza || statoR2 != "aperta" || verR2 != 1 {
		t.Errorf("F6: la bozza nuova dopo la sostituzione: %v v%d %s (prima %v v%d)", bozzaR2, verR2, statoR2, bozza, verPrima)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'abbandonata' AND versione = $2 AND motivo_chiusura LIKE 'sorgente_errata:%'`, bozza, verPrima); n != 1 {
		t.Errorf("F6: la bozza di F4 non e' abbandonata alla sua versione con la causa (v%d)", verPrima)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1 AND fonte_allegato_id = $2`, bozzaR2, s.al["9120001_r2.stp"]); n != 1 {
		t.Error("F6: la bozza nuova non nasce dallo STEP rev 2")
	}
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'revoca_sorgente' AND allegato_id = $3 AND bozza_id = $4`,
		s.thread, s.prodotto, s.al["9120001.stp"], bozza); n != 1 {
		t.Errorf("F6: revoche della sorgente di F3: %d", n)
	}
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'conferma_documento' AND allegato_id = $3 AND motivo IS NOT NULL`,
		s.thread, s.prodotto, s.al["9120001_r2.stp"]); n != 1 {
		t.Errorf("F6: conferme S8 dello STEP rev 2 con il motivo: %d", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND versione = 1 AND azione = 'riassegnazione_sorgente' AND prima->>'bozza_id' = $2::text`,
		bozzaR2, bozza); n != 1 {
		t.Error("F6: nessun evento riassegnazione_sorgente sulla bozza nuova")
	}
	if w := working(); w != w0 {
		t.Error("F6: la sostituzione della bozza ha cambiato la working")
	}
	t.Logf("F6: «Usa nuovo STEP» prima di R5: bozza %v v%d -> %v v1; detto dal dialogo %s; avviso %s", bozza, verPrima, bozzaR2, d6b["sostituzione"], d6b["avviso"])
	bozza = bozzaR2

	// ---- F6 (seguito): il secondo prodotto, S8, «Riparti dalla proposta», «Abbandona la bozza»
	arPrima := righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM file_rfq_associazione WHERE thread_id = $1 ORDER BY associazione_id) x`, s.thread)
	aePrima := conta(`SELECT count(*) FROM associazione_evento`)
	s.lancia(t, "F6c", nil)
	aspettaCodaFerma(t, b, 60*time.Second)
	bozzeP2 := righeD1(t, b, `SELECT b.stato::text || ' v' || max(v.versione) || ' PB ' || coalesce((SELECT string_agg(d->>'allegato_id' || ':' || coalesce(d->>'provenienza', '-'), ',')
		FROM distinta_bozza_versione v2, jsonb_array_elements(v2.snapshot->'documenti_preassociati') d WHERE v2.bozza_id = b.bozza_id AND v2.versione = 1), '-')
		FROM distinta_bozza b JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id WHERE b.thread_id = $1 AND b.prodotto_id = $2
		GROUP BY b.bozza_id, b.stato, b.creata_il ORDER BY b.creata_il`, s.thread, s.prodotto2)
	t.Logf("F6: le bozze del secondo prodotto (stato, ultima versione, PB della versione 1): %s", strings.ReplaceAll(bozzeP2, "\n", "; "))
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'abbandonata'`, s.thread, s.prodotto2); n != 2 {
		t.Errorf("F6: bozze abbandonate del secondo prodotto dopo «Riparti» e «Abbandona»: %d, attese 2", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto2); n != 0 {
		t.Errorf("F6: bozze aperte del secondo prodotto dopo «Abbandona»: %d", n)
	}
	// «Riparti»: la bozza nuova non eredita le PB della vecchia: le sue vengono solo dalla proposta (provenienza «motore»
	// con il suo registro), mai copiate
	if n := conta(`SELECT count(*) FROM distinta_bozza b JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = 1,
		jsonb_array_elements(v.snapshot->'documenti_preassociati') d
		WHERE b.thread_id = $1 AND b.prodotto_id = $2 AND coalesce(d->>'provenienza', '') <> 'motore'`, s.thread, s.prodotto2); n != 0 {
		t.Errorf("F6: PB non del motore nella versione 1 delle bozze del secondo prodotto: %d", n)
	}
	if ar := righeD1(t, b, `SELECT row_to_json(x)::text FROM (SELECT * FROM file_rfq_associazione WHERE thread_id = $1 ORDER BY associazione_id) x`, s.thread); ar != arPrima {
		t.Error("F6: «Riparti» o «Abbandona» hanno cambiato le associazioni registrate")
	}
	if n := conta(`SELECT count(*) FROM associazione_evento`); n != aePrima {
		t.Errorf("F6: righe di associazione_evento scritte da «Riparti» o «Abbandona»: %d -> %d", aePrima, n)
	}
	if w := working(); w != w0 {
		t.Error("F6: il secondo prodotto ha cambiato la working")
	}
	t.Log("F6 «Aggiorna alla proposta di oggi» con «togli» e «Tieni come nodo a mano» (righe 11 e 13 della matrice) NON ESEGUITA: servono caselle sparite dalla proposta di oggi della sorgente, cioè fatti dello STEP cambiati sotto il server; il banco d1 non riscrive l'analisi. Le provano DU-B (banco finto) e le L4 della corsia motore")
	if t.Failed() {
		return
	}

	// ---- F7: il riepilogo (R4) e «Conferma BOM» (R5) con la prima risposta persa, poi R6
	_, _, _, snapF7 := s.snapshotD1(t, s.prodotto)
	_, d7 := s.lancia(t, "F7", nil)
	aspettaCodaFerma(t, b, 60*time.Second)
	var chiaveR5 string
	if err := json.Unmarshal(d7["chiave"], &chiaveR5); err != nil || chiaveR5 == "" {
		t.Fatalf("F7: la chiave di R5 dallo script: %v %s", err, d7["chiave"])
	}
	t.Logf("F7: blocchi di R4 %s; preassociazioni in conflitto tolte con il rimedio del wizard %s; non inclusi %s", d7["blocchi"], d7["conflitti_risolti"], d7["non_inclusi"])
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
		t.Errorf("F7: conferme del prodotto dopo R5 con la risposta persa: %d, attesa una", n)
	}
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2 AND numero = 1 AND formato = 3 AND chiave_idempotenza = $3 AND bozza_id = $4`,
		s.thread, s.prodotto, chiaveR5, bozza); n != 1 {
		t.Errorf("F7: la conferma non è la n. 1 formato 3 con la chiave dello script e la bozza")
	}
	if n := conta(`SELECT count(*) FROM esito_intento WHERE chiave = $1 AND gesto = 'conferma_bom_v3'`, chiaveR5); n != 1 {
		t.Errorf("F7: esiti d'intento conferma_bom_v3 con la chiave: %d", n)
	}
	if _, _, st, _ := s.snapshotD1(t, s.prodotto); st != "confermata" {
		t.Errorf("F7: la bozza dopo R5 è %s", st)
	}
	if n := conta(`SELECT count(*) FROM componente WHERE componente_id = $1 AND step_strutturale_id IS NOT NULL`, s.prodotto); n != 1 {
		t.Error("F7: dopo R5 lo STEP della working del prodotto non c'è")
	}
	// i codici della working sono quelli della bozza confermata
	codiciBozza := righeD1(t, b, `SELECT DISTINCT n->'identita'->>'codice' FROM jsonb_array_elements($1::jsonb->'nodi') n WHERE n->>'op' <> 'non_includere'
		AND n->'identita'->>'codice' IS NOT NULL ORDER BY 1`, snapF7)
	codiciWorking := righeD1(t, b, `SELECT DISTINCT codice FROM componente WHERE thread_id = $1 AND archiviato_il IS NULL AND codice = ANY(string_to_array($2, E'\n')) ORDER BY 1`, s.thread, codiciBozza)
	if codiciBozza == "" || codiciWorking != codiciBozza {
		t.Errorf("F7: i codici della bozza confermata non sono nella working:\n  bozza:   %q\n  working: %q", codiciBozza, codiciWorking)
	}
	t.Logf("F7: AR scritte da R5: %s", strings.ReplaceAll(righeD1(t, b, `SELECT a.nome_file || ' → ' || c.codice || ' (' || r.origine || ', ' || coalesce(r.provenienza_pb, '-') || ')'
		FROM file_rfq_associazione r JOIN allegato a USING (allegato_id) JOIN componente c ON c.componente_id = r.componente_id
		WHERE r.thread_id = $1 AND r.stato = 'attiva' ORDER BY 1`, s.thread), "\n", "; "))
	if n := conta(`SELECT count(*) FROM file_rfq_associazione WHERE thread_id = $1 AND stato = 'attiva' AND origine = 'conferma_bom' AND provenienza_pb IS NOT NULL`, s.thread); n == 0 {
		t.Error("F7: R5 non ha scritto nessuna AR con la provenienza della PB")
	}
	if n := conta(`SELECT count(*) FROM file_rfq_associazione r JOIN allegato a USING (allegato_id) WHERE r.thread_id = $1 AND a.estensione IN ('stp', 'step')`, s.thread); n != 0 {
		t.Errorf("F7: AR su uno STEP del prodotto finito: %d", n)
	}

	// ---- F8: R5 di nuovo con la stessa chiave: riproposto, niente scritto; con un corpo diverso: 409
	stato8, job8 := s.statoD1(t), jobD1(t, b)
	s.lancia(t, "F8", map[string]any{"r5": d7["r5"]})
	confrontaLetture(t, "F8 (R5 ripetuta)", stato8, s.statoD1(t))
	if j := jobD1(t, b); j != job8 {
		t.Errorf("F8: lavori accodati dalla R5 ripetuta: %s -> %s", job8, j)
	}
	if n := conta(`SELECT count(*) FROM esito_intento WHERE chiave = $1`, chiaveR5); n != 1 {
		t.Errorf("F8: esiti d'intento con la chiave dopo le R5 ripetute: %d", n)
	}

	// ---- F9: il secondo riavvio vero; storico, conferma, AR e working uguali
	aspettaCodaFerma(t, b, 30*time.Second)
	letture9, stato9 := s.letturePersistite(t), s.statoD1(t)
	srv.riavvia()
	t.Logf("F9: secondo riavvio vero: pid %d fermato con Kill, pid %d avviato", srv.pid[1], srv.pid[2])
	confrontaLetture(t, "F9", stato9, s.statoD1(t))
	confrontaLetture(t, "F9", letture9, s.letturePersistite(t))
	_, d9 := s.lancia(t, "F9", nil)
	t.Logf("F9: segni nella Revisione %s; verifica corrente del server %s (stato %s, motivi %s)", d9["segni"], d9["verifica"], d9["stato_verifica"], d9["motivi_verifica"])

	// ---- F10: il giro di sole letture: il DB identico, nessun lavoro accodato
	aspettaCodaFerma(t, b, 30*time.Second)
	prima10, job10 := fotoD1(t, b), jobD1(t, b)
	_, d10 := s.lancia(t, "F10", map[string]any{"chiave": chiaveR5})
	aspettaCodaFerma(t, b, 30*time.Second)
	if d := differenze(prima10, fotoD1(t, b)); d != "" {
		t.Errorf("F10: il giro di sole letture ha scritto in: %s", d)
	}
	if j := jobD1(t, b); j != job10 {
		t.Errorf("F10: lavori accodati dal giro di sole letture: %s -> %s", job10, j)
	}
	t.Logf("F10: R4 dal wizard %s", d10["r4"])
	// ---- F12: due sessioni con il wizard aperto; il worker lo ferma il test e lo riavvia lo script dopo N1 e N4
	worker.ferma()
	_, d12 := s.lancia(t, "F12", map[string]any{"worker": map[string]any{"dir": workers, "arg": []string{"worker_analisi.py", "--config", filepath.Join(dir, "nessun-worker.toml")},
		"env": map[string]string{"COCKPIT_URL": srv.url, "COCKPIT_TOKEN": gAnalisi, "COCKPIT_WORKER_ID": "analisi@" + host, "COCKPIT_STAGING": wAnalisi,
			"PYTHONIOENCODING": "utf-8", "PYTHONUTF8": "1"}}})
	t.Logf("F12: dopo l'analisi del documento caricato da B, su A: %s; «Conferma BOM» acceso prima delle notifiche: %s", d12["dopo_analisi"], d12["conferma_accesa_prima"])
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
		t.Errorf("F12: conferme dopo le notifiche: %d (nessuna conferma di un riepilogo superato)", n)
	}
	if bz, v, st, _ := s.snapshotD1(t, s.prodotto); st != "aperta" || v < 3 || bz == bozza {
		t.Errorf("F12: la revisione dopo i due salvataggi di A e quello di B: %v v%d %s", bz, v, st)
	}
	t.Log("F12 N3 NON ESEGUITA: vuole la conferma di un altro prodotto che tocca il perimetro di A (un componente condiviso); nella RFQ sintetica i due prodotti non condividono componenti")
	t.Log("F12 N6 NON ESEGUITA: con la lettura degli ingressi accesa la pagina spegne «Conferma BOM» al primo segno (N1, N4, N2), quindi il 409 del controllo finale non si raggiunge dalla pagina; il caso è CN-07 (L4 della persistenza)")
	t.Log("F11 NON ESEGUITA: il rispetto del lavoro delle pagine di prima (esclusioni legacy con chi e quando, il codice corretto a mano nella pagina vecchia, la striscia del contesto) vuole una RFQ con il lavoro della pagina vecchia; la RFQ sintetica nasce formato 3 (rfq_formato3_segno), dove i gesti legacy sono rifiutati, e la grammatica ACME del banco non ha un contesto partner. La copertura resta delle L4 della corsia motore (A-11, R-43) e del banco finto")
}
