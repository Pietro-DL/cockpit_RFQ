//go:build integrazione

// L4 — 7C.1, P0: il worker analisi non legge il disco del server.
//
// Al banco a due macchine del 20/09/2026 il payload di `analizza_allegato` portava `path_staging`,
// cioe' `C:\promatec\_staging\_contenuti\...` sul disco della MSI; il worker sul portatile lo
// cercava sul proprio disco e falliva con «file non trovato in staging». Qui il server ha lo
// staging in una cartella e il worker in un'altra, e la prova pretende due cose: che il payload
// non contenga nessun percorso, e che i byte arrivino al worker solo da
// GET /api/v1/allegati/{id}/contenuto, dentro il suo tentativo. Rimettere `path_staging` nel
// payload fa diventare rossa la prima; il worker vecchio, che apriva il percorso, la seconda.
package workerapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// bancoAnalisi: il banco dell'upload piu' un contenuto gia' in staging e la sua analisi accodata.
type bancoAnalisi struct {
	*banco
	contenuto []byte
	sha       string
	analisi   db.Job
}

func preparaBancoAnalisi(t *testing.T, nome string) *bancoAnalisi {
	t.Helper()
	contenuto, _ := contenutoCasuale(4096, 21)
	return preparaBancoAnalisiCon(t, nome, contenuto)
}

// preparaBancoAnalisiCon: lo stesso banco, con byte scelti dal test. Serve a chi prova che cosa il
// worker LEGGE dentro il file — la struttura di uno STEP — e non solo che i byte gli arrivino.
func preparaBancoAnalisiCon(t *testing.T, nome string, contenuto []byte) *bancoAnalisi {
	t.Helper()
	b := preparaBanco(t, 0)
	ext := strings.TrimPrefix(filepath.Ext(nome), ".")
	a, m := b.messaggioConAllegato(nome, ext)
	sha := shaDi(contenuto)
	definitivo, err := staging.PercorsoContenuto(b.cartella, sha, nome)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(definitivo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(definitivo, contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET sha256 = $2, bytes = $3, path_staging = $4, stato = 'in_staging' WHERE allegato_id = $1`,
		a.AllegatoID, sha, len(contenuto), definitivo); err != nil {
		t.Fatal(err)
	}
	a, err = b.q.GetAllegato(b.ctx, a.AllegatoID)
	if err != nil {
		t.Fatal(err)
	}
	j, err := coda.AccodaAnalisi(b.ctx, b.q, a, m.ThreadID, b.s.Analizzatore)
	if err != nil || j == nil {
		t.Fatalf("accoda analisi: job=%v err=%v", j, err)
	}
	b.allegato = a
	return &bancoAnalisi{banco: b, contenuto: contenuto, sha: sha, analisi: *j}
}

// claimAnalisi prende il job di analisi come farebbe il worker analisi censito.
func (b *bancoAnalisi) claimAnalisi(worker string) coda.Tentativo {
	b.t.Helper()
	b.censisci(worker, db.WorkerTipoAnalisi)
	j, err := coda.Claim(b.ctx, b.q, db.WorkerTipoAnalisi, worker, coda.Destinazione{}, 0)
	if err != nil || j == nil {
		b.t.Fatalf("claim analisi: job=%v err=%v", j, err)
	}
	if j.JobID != b.analisi.JobID {
		b.t.Fatalf("il claim ha preso il job %d, atteso l'analisi %d", j.JobID, b.analisi.JobID)
	}
	return coda.Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: worker}
}

func (b *bancoAnalisi) get(t coda.Tentativo, allegato uuid.UUID, token string) *http.Response {
	b.t.Helper()
	url := fmt.Sprintf("%s/api/v1/allegati/%s/contenuto?job_id=%d&lease_token=%s&worker_id=%s",
		b.srv.URL, allegato, t.JobID, t.LeaseToken, t.WorkerID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		b.t.Fatal(err)
	}
	req.Header.Set("X-Cockpit-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatalf("GET: %v", err)
	}
	return resp
}

func TestIlPayloadDellAnalisiNonPortaPercorsiDelServer(t *testing.T) {
	b := preparaBancoAnalisi(t, "6674611A_4.dxf")
	payload := string(b.analisi.Payload)
	var campi map[string]any
	if err := json.Unmarshal(b.analisi.Payload, &campi); err != nil {
		t.Fatal(err)
	}
	if _, c := campi["path_staging"]; c {
		t.Errorf("il payload porta ancora path_staging: %s", payload)
	}
	if strings.Contains(payload, filepath.ToSlash(b.cartella)) || strings.Contains(payload, b.cartella) ||
		strings.Contains(payload, staging.CartellaContenuti) {
		t.Errorf("il payload contiene un percorso dello staging del server: %s", payload)
	}
	for _, chiave := range []string{"allegato_id", "sha256", "bytes", "nome_file"} {
		if _, c := campi[chiave]; !c {
			t.Errorf("il payload non porta %s: %s", chiave, payload)
		}
	}
	if campi["sha256"] != b.sha || campi["bytes"] != float64(len(b.contenuto)) {
		t.Errorf("sha256/bytes nel payload non sono quelli del contenuto: %s", payload)
	}
}

func TestIlContenutoLoScaricaSoloIlTentativoValidoDellaSuaAnalisi(t *testing.T) {
	b := preparaBancoAnalisi(t, "6674611A_4.dxf")
	tent := b.claimAnalisi("analisi@PC-A")

	// (a) il tentativo valido riceve i byte, con lo sha256 in testata
	resp := b.get(tent, b.allegato.AllegatoID, tokenDi(tent.WorkerID))
	corpo, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET contenuto: %d %s", resp.StatusCode, corpo)
	}
	if !bytes.Equal(corpo, b.contenuto) {
		t.Errorf("byte diversi: ricevuti %d, attesi %d", len(corpo), len(b.contenuto))
	}
	if resp.Header.Get("X-Cockpit-Sha256") != b.sha || resp.ContentLength != int64(len(b.contenuto)) {
		t.Errorf("testate: sha=%q length=%d", resp.Header.Get("X-Cockpit-Sha256"), resp.ContentLength)
	}
	// il file e' rimasto dov'era, nello staging del server
	if !esiste(b.allegato.PathStaging.String) {
		t.Error("il download ha spostato o cancellato il contenuto del server")
	}

	// (b) un altro worker censito, con il tentativo di questo: non e' il suo
	b.censisci("analisi@PC-B", db.WorkerTipoAnalisi)
	altro := coda.Tentativo{JobID: tent.JobID, LeaseToken: tent.LeaseToken, WorkerID: "analisi@PC-B"}
	resp = b.get(altro, b.allegato.AllegatoID, tokenDi("analisi@PC-B"))
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("un worker che non ha il tentativo: %d, atteso 409", resp.StatusCode)
	}

	// (c) il tentativo e' valido ma l'allegato non e' quello del job
	altroAll, _ := b.messaggioConAllegato("altro.pdf", "pdf")
	resp = b.get(tent, altroAll.AllegatoID, tokenDi(tent.WorkerID))
	corpo, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Errorf("l'allegato di un altro messaggio: %d %s, atteso 422", resp.StatusCode, corpo)
	}

	// (d) il contenuto e' sparito dalla cache: 410 con il rimedio
	if err := os.Remove(b.allegato.PathStaging.String); err != nil {
		t.Fatal(err)
	}
	resp = b.get(tent, b.allegato.AllegatoID, tokenDi(tent.WorkerID))
	corpo, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone || !strings.Contains(string(corpo), "Riscarica") {
		t.Errorf("contenuto sparito: %d %s, atteso 410 con «Riscarica»", resp.StatusCode, corpo)
	}

	// (e) il lease e' scaduto: 409, qualunque cosa ci sia sul disco
	if _, err := b.pool.Exec(b.ctx, `UPDATE job SET lease_fino_a = now() - interval '1 minute' WHERE job_id = $1`, tent.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.q.RilasciaLeaseScaduti(b.ctx); err != nil {
		t.Fatal(err)
	}
	resp = b.get(tent, b.allegato.AllegatoID, tokenDi(tent.WorkerID))
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Errorf("lease scaduto: %d, atteso 409", resp.StatusCode)
	}
}

const tokenProvaAnalisi = "token-e2e-analisi-di-prova-che-non-esiste-in-produzione"

// TestE2EIlWorkerAnalisiVeroConLoStagingSuUnAltraCartella: il worker VERO (worker_analisi.py,
// --una-volta) contro il server vero, con la cartella del worker diversa da quella del server.
// Con il contratto vecchio il worker apriva `path_staging` e sul proprio disco non lo trovava: il
// job falliva. Qui deve chiudersi, la proposta deve essere aggiornata dal contenuto, e nella
// cartella del worker non deve restare niente.
func TestE2EIlWorkerAnalisiVeroConLoStagingSuUnAltraCartella(t *testing.T) {
	if os.Getenv("COCKPIT_TEST_SENZA_PYTHON") != "" {
		t.Skip("COCKPIT_TEST_SENZA_PYTHON: il worker vero non viene avviato, prova non verificata")
	}
	b := preparaBancoAnalisi(t, "6674611A_4.dxf")
	uscita, download, stagingWorker := eseguiWorkerAnalisiVero(t, b)

	j, err := b.q.GetJob(b.ctx, b.analisi.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Stato != db.StatoJobFatto {
		t.Fatalf("job di analisi in stato %s (tentativi %d, errore %q): il worker vero non ha avuto i byte.\nOutput del worker:\n%s",
			j.Stato, j.Tentativi, j.Errore.String, uscita)
	}
	p := propostaDi(t, b.pool, b.allegato.AllegatoID)
	if p.TipoProposto != db.TipoDocumentoSviluppoDxf || p.Codice.String != "6674611A" || p.Rev.String != "4" {
		t.Errorf("proposta non aggiornata dall'analisi: tipo=%s codice=%q rev=%q", p.TipoProposto, p.Codice.String, p.Rev.String)
	}
	if !esiste(b.allegato.PathStaging.String) {
		t.Error("il contenuto del server non c'e' piu'")
	}
	if resti := fileSotto(t, filepath.Join(stagingWorker, "tmp")); len(resti) > 0 {
		t.Errorf("il worker ha lasciato file temporanei sul proprio disco: %v", resti)
	}
	if !strings.Contains(uscita, "completato") {
		t.Errorf("output del worker inatteso:\n%s", uscita)
	}
	if download.Load() < 1 {
		t.Errorf("il worker non ha chiesto il contenuto all'endpoint: i byte sono arrivati da un percorso, non dal server.\nOutput del worker:\n%s", uscita)
	}
}

// eseguiWorkerAnalisiVero avvia worker_analisi.py (quello vero, --una-volta) contro questo banco e
// aspetta che finisca. Restituisce ciò che il worker ha scritto, quante volte ha chiesto il contenuto
// al server e la cartella che gli è stata data — diversa da quella del server, perché è la condizione
// che la prova difende.
func eseguiWorkerAnalisiVero(t *testing.T, b *bancoAnalisi) (string, *atomic.Int32, string) {
	t.Helper()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	host = strings.ToUpper(host)
	workerID := "analisi@" + host
	cfg := &config.Config{}
	cfg.Outlook.CasellaDefault = "commerciale@azienda.it"
	cfg.Caselle = []config.Casella{{Indirizzo: "commerciale@azienda.it", Nome: "Commerciale", Canale: "outlook", Condivisa: true}}
	cfg.Postazioni = []config.Postazione{{NomeHost: host}}
	cfg.Worker = []config.Worker{{Nome: workerID, Tipo: "analisi", Token: tokenProvaAnalisi, Postazione: host}}
	if _, err := fondazioni.Semina(b.ctx, b.q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	// il job dello stage accodato dal banco non e' del worker analisi e non lo disturba; lo si toglie
	// per non lasciare un job pendente che confonda la lettura del risultato
	if _, err := b.q.AnnullaJob(b.ctx, b.job.JobID); err != nil {
		t.Fatal(err)
	}

	stagingWorker := t.TempDir()
	if filepath.Clean(stagingWorker) == filepath.Clean(b.cartella) {
		t.Fatal("la prova vale solo con due cartelle diverse")
	}
	// Sulla stessa macchina un worker che aprisse il percorso del server lo TROVEREBBE: le due
	// cartelle sono diverse ma sullo stesso disco. Per questo la prova conta le chiamate all'endpoint
	// del contenuto: i byte devono essere passati di li', non da un percorso.
	download := &atomic.Int32{}
	mux := http.NewServeMux()
	b.s.Registra(mux)
	contato := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/contenuto") {
			download.Add(1)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(contato.Close)
	e := &esecuzione{out: &bytes.Buffer{}}
	py, err := exec.LookPath("python")
	if err != nil {
		// non si salta: il worker E' Python, e una prova saltata non e' una prova passata
		t.Fatalf("python non è nel PATH: questa prova fa girare il worker analisi vero (serve pymupdf): %v", err)
	}
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "workers"))
	if err != nil {
		t.Fatal(err)
	}
	e.cmd = exec.Command(py, "worker_analisi.py", "--una-volta", "--debug", "--config", filepath.Join(stagingWorker, "nessun-worker.toml"))
	e.cmd.Dir = dir
	e.cmd.Env = append(os.Environ(),
		"COCKPIT_URL="+contato.URL,
		"COCKPIT_TOKEN="+tokenProvaAnalisi,
		"COCKPIT_WORKER_ID="+workerID,
		"COCKPIT_STAGING="+stagingWorker,
		"COCKPIT_IMPRONTA=",
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)
	e.cmd.Stdout, e.cmd.Stderr = e.out, e.out
	if err := e.cmd.Start(); err != nil {
		t.Fatalf("avvio del worker analisi: %v", err)
	}
	return e.attendi(t), download, stagingWorker
}

// TestE2EIlWorkerAnalisiVeroLeggeLaStrutturaDiUnoStep: il worker vero, uno STEP Part 21 vero
// (piccolo, ma vero) e il grafo che ne esce, fino dentro `analisi_fatti`.
//
// È la prova che il contratto della struttura regge il giro intero — Python, JSON, schema, tipo Go —
// e non solo i test delle due parti prese da sole. Il caso scelto è quello per cui la proposta si è
// dovuta dividere in nodi e relazioni: lo stesso sottoassieme sotto due prodotti diversi.
func TestE2EIlWorkerAnalisiVeroLeggeLaStrutturaDiUnoStep(t *testing.T) {
	if os.Getenv("COCKPIT_TEST_SENZA_PYTHON") != "" {
		t.Skip("COCKPIT_TEST_SENZA_PYTHON: il worker vero non viene avviato, prova non verificata")
	}
	b := preparaBancoAnalisiCon(t, "52922757.step", []byte(stepDuePadri))
	uscita, _, _ := eseguiWorkerAnalisiVero(t, b)

	j, err := b.q.GetJob(b.ctx, b.analisi.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Stato != db.StatoJobFatto {
		t.Fatalf("job di analisi in stato %s (errore %q).\nOutput del worker:\n%s", j.Stato, j.Errore.String, uscita)
	}
	fatti, err := b.q.GetAnalisiFatti(b.ctx, db.GetAnalisiFattiParams{
		Sha256: b.sha, VersioneAnalizzatore: int16(b.s.Analizzatore.Versione),
		HashConfigurazione: b.s.Analizzatore.Hash(),
	})
	if err != nil {
		t.Fatalf("i fatti dell'analisi non sono stati conservati: %v", err)
	}
	st, ok := worker.DecodificaStruttura(fatti.Fatti)
	if !ok {
		t.Fatalf("struttura non presente nei fatti: %s", fatti.Fatti)
	}
	if st.Versione != 2 || st.Schema != "AP214" {
		t.Errorf("versione=%d schema=%q", st.Versione, st.Schema)
	}
	if len(st.Nodi) != 3 || len(st.Relazioni) != 2 {
		t.Fatalf("nodi=%d relazioni=%d, attesi 3 e 2: %+v", len(st.Nodi), len(st.Relazioni), st)
	}
	for _, n := range st.Nodi {
		if n.NomeGrezzo == "" {
			t.Errorf("nodo senza nome grezzo: %+v", n)
		}
		if n.Evidenza["riga"] == nil {
			t.Errorf("nodo senza evidenza: %+v", n)
		}
	}
	// il sottoassieme sta sotto tutti e due i prodotti: due relazioni, un nodo solo
	figli := map[string]int{}
	for _, r := range st.Relazioni {
		figli[r.Figlio]++
	}
	if len(figli) != 1 {
		t.Fatalf("atteso un solo figlio con due padri, trovati %v", figli)
	}
	for _, n := range figli {
		if n != 2 {
			t.Errorf("il sottoassieme ha %d padri, attesi 2", n)
		}
	}
	if len(st.Radici) != 2 {
		t.Errorf("radici=%v, attese le due teste", st.Radici)
	}
	// la proposta del documento resta quella di sempre: la struttura non la decide
	p := propostaDi(t, b.pool, b.allegato.AllegatoID)
	if p.TipoProposto != db.TipoDocumentoCad3d {
		t.Errorf("tipo proposto = %s, atteso cad_3d", p.TipoProposto)
	}
}

// Uno STEP minimo con due prodotti che condividono un sottoassieme. È una fixture, non un file di
// prodotto: venti righe di Part 21 si leggono, un file binario in una cartella no.
const stepDuePadri = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('52922757.step','2026-09-22T00:00:00',(''),(''),'','','');
FILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 3 1 1 }'));
ENDSEC;
DATA;
#1=APPLICATION_CONTEXT('prova');
#10=PRODUCT('52922757','52922757_B','PRODOTTO A',(#1));
#11=PRODUCT_DEFINITION_FORMATION('B','',#10);
#12=PRODUCT_DEFINITION('design','',#11,#1);
#20=PRODUCT('52922758','52922758_A','PRODOTTO B',(#1));
#21=PRODUCT_DEFINITION_FORMATION('A','',#20);
#22=PRODUCT_DEFINITION('design','',#21,#1);
#30=PRODUCT('52920517','52920517','SOTTOASSIEME X',(#1));
#31=PRODUCT_DEFINITION_FORMATION('1','',#30);
#32=PRODUCT_DEFINITION('design','',#31,#1);
#40=NEXT_ASSEMBLY_USAGE_OCCURRENCE('1','pos','',#12,#32,$);
#41=NEXT_ASSEMBLY_USAGE_OCCURRENCE('2','pos','',#22,#32,$);
ENDSEC;
END-ISO-10303-21;
`
