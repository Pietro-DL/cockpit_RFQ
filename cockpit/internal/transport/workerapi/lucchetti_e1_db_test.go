//go:build integrazione

package workerapi

// L4 — giro G39-bis, corsia file: l'ordine dei lucchetti fra E1′ («+ Aggiungi file», filerfq.RegistraFile) e i lavori del
// worker sulla stessa RFQ (lucchetti.go). Il difetto: la prova d1 nel browser (TestV1DistintaUnicaPercorsoPersistito)
// falliva una corsa su cinque con un 500 «da verificare», e nel log di PostgreSQL c'era un 40P01 fra due UpsertAllegato.
// E1′ teneva il messaggio della nota (UpsertNotaInterna, BloccaMessaggio) e, con l'allegato nuovo, il trigger
// aggiorna_n_allegati riscriveva quel messaggio: la chiave esterna verso thread_offerta chiedeva la RFQ FOR KEY SHARE.
// L'estrazione dell'archivio caricato poco prima (EstraiArchivio) teneva la RFQ FOR UPDATE e, con la sua voce, lo stesso
// trigger chiedeva il messaggio della nota: un ciclo.
//
// Le prove mettono le due transazioni nel punto del ciclo senza contare sul caso: una transazione del banco tiene il
// lucchetto per contenuto del file di E1′ (lo stesso pg_advisory_xact_lock di BloccaContenuto), E1′ si ferma li', il
// secondo lavoro parte e si ferma sul suo lucchetto, e solo allora il banco rilascia. Ogni giro ha file nuovi; il
// contatore deadlocks di pg_stat_database non deve crescere. Solo dati sintetici.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// giriLucchetti: i giri di ogni prova (con -count=N le ripetizioni si moltiplicano). giriMassimiRitento: fin dove vanno
// le due prove del ritento con il 40P01 quando il servizio non e' ancora stato la vittima (commento piu' sotto).
const (
	giriLucchetti      = 3
	giriMassimiRitento = 12
)

// spiaLucchetti: una connessione fuori dal pool del banco, per guardare chi aspetta e per leggere il contatore dei 40P01.
type spiaLucchetti struct {
	t    *testing.T
	ctx  context.Context
	conn *pgx.Conn
}

func nuovaSpia(t *testing.T) *spiaLucchetti {
	t.Helper()
	ctx := context.Background()
	c, err := pgx.Connect(ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return &spiaLucchetti{t: t, ctx: ctx, conn: c}
}

// inAttesa: quante connessioni del database aspettano un lucchetto; advisory = il lucchetto per contenuto, altrimenti le
// righe (transactionid, tuple).
func (s *spiaLucchetti) inAttesa(advisory bool) int {
	s.t.Helper()
	cond := "wait_event = 'advisory'"
	if !advisory {
		cond = "wait_event <> 'advisory'"
	}
	var n int
	if err := s.conn.QueryRow(s.ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
		AND pid <> pg_backend_pid() AND wait_event_type = 'Lock' AND `+cond).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// aspetta finche' almeno n connessioni sono ferme sul lucchetto indicato (al piu' 10 secondi); falso se non succede.
func (s *spiaLucchetti) aspetta(advisory bool, n int) bool {
	s.t.Helper()
	fine := time.Now().Add(10 * time.Second)
	for time.Now().Before(fine) {
		if s.inAttesa(advisory) >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// stalli: il contatore deadlocks del database, dopo aver chiuso le connessioni del pool (i contatori delle altre
// connessioni arrivano in pg_stat_database quando escono), come in transport/web (c6b_r5_concorrenza_db_test.go).
func (s *spiaLucchetti) stalli(b *bancoCiclo) int64 {
	s.t.Helper()
	b.pool.Reset()
	fine := time.Now().Add(5 * time.Second)
	for time.Now().Before(fine) {
		var n int
		if err := s.conn.QueryRow(s.ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
			AND pid <> pg_backend_pid() AND backend_type = 'client backend'`).Scan(&n); err != nil {
			s.t.Fatal(err)
		}
		if n == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := s.conn.Exec(s.ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		s.t.Fatal(err)
	}
	var d int64
	if err := s.conn.QueryRow(s.ctx, `SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()`).Scan(&d); err != nil {
		s.t.Fatal(err)
	}
	return d
}

// custode: la transazione del banco che tiene il lucchetto per contenuto di uno sha (la stessa chiave di BloccaContenuto).
type custode struct {
	tx  pgx.Tx
	ctx context.Context
}

func (b *bancoCiclo) custodisci(sha string) *custode {
	b.t.Helper()
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	if _, err := tx.Exec(b.ctx, `SELECT pg_advisory_xact_lock(hashtextextended('contenuto:' || lower($1::text), 0))`, sha); err != nil {
		_ = tx.Rollback(b.ctx)
		b.t.Fatal(err)
	}
	return &custode{tx: tx, ctx: b.ctx}
}

func (c *custode) rilascia() { _ = c.tx.Rollback(c.ctx) }

// parziale: un file arrivato con E1′ (nello staging come parziale, non ancora registrato).
func (b *bancoCiclo) parziale(nome string, c []byte) filerfq.FileArrivato {
	b.t.Helper()
	f, err := filerfq.ScriviParziale(b.staging, bytes.NewReader(c), nome, "application/octet-stream", 1<<30)
	if err != nil {
		b.t.Fatal(err)
	}
	return f
}

type esitoE1 struct {
	es  filerfq.EsitoFile
	err error
}

// TestG39bisE1ControEstrazioneNellaNota: E1′ registra un file nella nota del cliente mentre l'estrazione dell'archivio
// caricato poco prima nella stessa nota scrive le sue voci (il caso di d1, F2 «+ Aggiungi file»: lo zip e due file).
// Atteso: tutte e due finiscono, nessun 40P01, le voci e il file ci sono. Sul codice di prima: 40P01 a ogni giro.
func TestG39bisE1ControEstrazioneNellaNota(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	th := b.rfq("RFQ-G39BIS-A")
	stalliIn := spia.stalli(b)
	for giro := 1; giro <= giriLucchetti; giro++ {
		// lo zip, caricato con E1′ come «file del cliente»: la strada d'ingresso accoda la sua estrazione
		z := zipOrdinato(t, [2]string{fmt.Sprintf("V%d-1.pdf", giro), fmt.Sprintf("%%PDF-1.4 voce uno del giro %d", giro)},
			[2]string{fmt.Sprintf("V%d-2.stp", giro), fmt.Sprintf("ISO-10303-21; voce due del giro %d", giro)})
		es, err := b.svc.RegistraFile(b.ctx, th, b.utente, false, b.parziale(fmt.Sprintf("PACCHETTO-%d.zip", giro), z))
		if err != nil || es.Esito != "caricato" || es.AllegatoID == nil {
			t.Fatalf("giro %d: lo zip non entra: %+v %v", giro, es, err)
		}
		zipID := *es.AllegatoID
		// il secondo file, con un contenuto diverso dalle voci
		c := []byte(fmt.Sprintf("%%PDF-1.4 SCALA secondo file del giro %d", giro))
		f := b.parziale(fmt.Sprintf("SECONDO-%d.pdf", giro), c)
		cu := b.custodisci(f.Sha256)
		e1 := make(chan esitoE1, 1)
		go func() {
			es, err := b.svc.RegistraFile(b.ctx, th, b.utente, false, f)
			e1 <- esitoE1{es, err}
		}()
		if !spia.aspetta(true, 1) {
			cu.rilascia()
			t.Fatalf("giro %d: E1′ non si e' fermato sul lucchetto per contenuto", giro)
		}
		estr := make(chan error, 1)
		go func() {
			_, err := b.s.EstraiArchivio(b.ctx, zipID, uuid.New())
			estr <- err
		}()
		fermo := spia.aspetta(false, 1)
		cu.rilascia()
		if !fermo {
			t.Errorf("giro %d: l'estrazione non ha aspettato nessun lucchetto (la prova non ha messo le transazioni nel punto del ciclo)", giro)
		}
		var r esitoE1
		var errEstr error
		select {
		case r = <-e1:
		case <-time.After(30 * time.Second):
			t.Fatalf("giro %d: E1′ non finisce", giro)
		}
		select {
		case errEstr = <-estr:
		case <-time.After(30 * time.Second):
			t.Fatalf("giro %d: l'estrazione non finisce", giro)
		}
		if r.err != nil || r.es.Esito != "caricato" {
			t.Errorf("giro %d: E1′ durante l'estrazione: %+v %v", giro, r.es, r.err)
		}
		if errEstr != nil {
			t.Errorf("giro %d: l'estrazione durante E1′: %v", giro, errEstr)
		}
		var voci int
		b.riga(`SELECT count(*) FROM allegato WHERE contenitore_id = $1`, &voci, zipID)
		if voci != 2 && errEstr == nil {
			t.Errorf("giro %d: voci dell'archivio: %d, attese 2", giro, voci)
		}
	}
	if d := spia.stalli(b); d != stalliIn {
		t.Errorf("40P01: pg_stat_database.deadlocks da %d a %d", stalliIn, d)
	}
	if n := b.disallineate(); n != 0 {
		t.Errorf("proposte disallineate dalla RFQ del loro messaggio: %d", n)
	}
	var file int
	b.riga(`SELECT count(*) FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.canale = 'nota' AND a.contenitore_id IS NULL AND a.nome_file LIKE 'SECONDO-%'`, &file)
	if file != giriLucchetti && !t.Failed() {
		t.Errorf("file registrati da E1′: %d, attesi %d", file, giriLucchetti)
	}
}

// inTentativo mette il job in corso con un tentativo nuovo, come lo lascia un claim, e lo restituisce.
func (b *bancoCiclo) inTentativo(jobID int64, workerID string) (db.Job, coda.Tentativo) {
	b.t.Helper()
	tok := uuid.New()
	b.esegui(`UPDATE job SET stato = 'in_corso', lease_token = $2, worker_id = $3, lease_fino_a = now() + interval '1 hour',
		avviato_il = now(), durata_max_s = 3600 WHERE job_id = $1`, jobID, tok, workerID)
	j, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		b.t.Fatal(err)
	}
	return j, coda.Tentativo{JobID: jobID, LeaseToken: tok, WorkerID: workerID}
}

type esitoRisultato struct {
	es  esitoScrittura
	err error
}

// attendi2 riceve l'esito del risultato e quello di E1′ (o del gesto), al piu' 30 secondi ciascuno.
func attendi2[T any](t *testing.T, c chan T, cosa string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(30 * time.Second):
		t.Fatalf("%s non finisce", cosa)
	}
	var zero T
	return zero
}

// TestG39bisRisultatoDelDownloadControE1: il risultato del download di un allegato della mail della RFQ (lo stage) e
// E1′ con lo stesso contenuto nella stessa RFQ. Il risultato aspetta il lucchetto per contenuto; E1′ arriva dopo. Con la
// RFQ presa per prima da tutti e due (lucchetti.go, fascicolo.notaDellaRFQ) si mettono in fila; con la RFQ per prima solo
// in E1′, il risultato teneva il contenuto e chiedeva la RFQ con la chiave esterna della proposta nuova, mentre E1′
// teneva la RFQ e chiedeva il contenuto: 40P01. Una transazione per tentativo, senza ritenti: si prova l'ordine.
func TestG39bisRisultatoDelDownloadControE1(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	th := b.rfq("RFQ-G39BIS-B")
	stalliIn := spia.stalli(b)
	for giro := 1; giro <= giriLucchetti; giro++ {
		c := []byte(fmt.Sprintf("%%PDF-1.4 SCALA stesso contenuto del giro %d", giro))
		sha := shaDi(c)
		nome := fmt.Sprintf("DISEGNO-%d.pdf", giro)
		msg := b.mail(th)
		var all uuid.UUID
		b.riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, ricevuto_il)
			VALUES ($1, 1, $2, 'pdf', 'file', 'outlook', $3, now()) RETURNING allegato_id`, &all, msg, nome, len(c))
		job, err := coda.Accoda(b.ctx, b.q, db.TipoJobStageAllegato, worker.PayloadStageAllegato{AllegatoID: all, NomeFile: nome},
			"stage:"+all.String(), 1)
		if err != nil || job == nil {
			t.Fatalf("giro %d: stage non accodato: %v %v", giro, job, err)
		}
		j, tent := b.inTentativo(job.JobID, "outlook@PC-G39BIS")
		parte := staging.PercorsoParte(b.staging, all, tent.LeaseToken)
		if err := os.MkdirAll(filepath.Dir(parte), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(parte, c, 0o644); err != nil {
			t.Fatal(err)
		}
		dati, _ := json.Marshal(worker.RisultatoStage{AllegatoID: all, Sha256: sha, Bytes: int64(len(c))})
		prep, err := b.s.preparaStage(b.ctx, &j, tent, dati)
		if err != nil {
			t.Fatalf("giro %d: preparaStage: %v", giro, err)
		}
		rfq, err := b.s.rfqDelRisultato(b.ctx, b.q, &j, prep)
		if err != nil || len(rfq) != 1 || rfq[0] != th {
			t.Fatalf("giro %d: le RFQ del risultato: %v %v", giro, rfq, err)
		}
		cu := b.custodisci(sha)
		ris := make(chan esitoRisultato, 1)
		go func() {
			es, _, err := b.s.scriviRisultato(b.ctx, &j, tent, worker.RisultatoRichiesta{Esito: "ok", Dati: dati}, prep, rfq)
			ris <- esitoRisultato{es, err}
		}()
		if !spia.aspetta(true, 1) {
			cu.rilascia()
			t.Fatalf("giro %d: il risultato non si e' fermato sul lucchetto per contenuto", giro)
		}
		f := b.parziale(fmt.Sprintf("STESSO-%d.pdf", giro), c)
		e1 := make(chan esitoE1, 1)
		go func() {
			es, err := b.svc.RegistraFile(b.ctx, th, b.utente, false, f)
			e1 <- esitoE1{es, err}
		}()
		// E1′ aspetta: la RFQ (con l'ordine unico) o il contenuto (con la RFQ per prima solo in E1′)
		fermo := false
		for fine := time.Now().Add(10 * time.Second); time.Now().Before(fine); time.Sleep(10 * time.Millisecond) {
			if spia.inAttesa(true)+spia.inAttesa(false) >= 2 {
				fermo = true
				break
			}
		}
		cu.rilascia()
		if !fermo {
			t.Errorf("giro %d: E1′ non ha aspettato nessun lucchetto", giro)
		}
		r := attendi2(t, ris, fmt.Sprintf("giro %d: il risultato del download", giro))
		e := attendi2(t, e1, fmt.Sprintf("giro %d: E1′", giro))
		if r.es != scritturaFatta || r.err != nil {
			t.Errorf("giro %d: il risultato del download durante E1′: esito %d, %v", giro, r.es, r.err)
		}
		if e.err != nil || e.es.Esito != "caricato" {
			t.Errorf("giro %d: E1′ durante il risultato del download: %+v %v", giro, e.es, e.err)
		}
	}
	if d := spia.stalli(b); d != stalliIn {
		t.Errorf("40P01: pg_stat_database.deadlocks da %d a %d", stalliIn, d)
	}
	if n := b.disallineate(); n != 0 {
		t.Errorf("proposte disallineate dalla RFQ del loro messaggio: %d", n)
	}
}

// TestG39bisRisultatoDellAnalisiControGestoSullaRFQ: il risultato dell'analisi di uno STEP della RFQ (con la struttura,
// che diventa proposte di nodi nella RFQ) e un gesto che tiene la RFQ FOR UPDATE e poi blocca la proposta di documento
// di quel file, come R5 e i gesti legacy. Prima il risultato bloccava la proposta e chiedeva la RFQ con la chiave
// esterna delle proposte di nodo nuove: con il gesto in mezzo, 40P01. Dopo, il risultato prende la RFQ per prima e
// aspetta il gesto senza tenere niente.
func TestG39bisRisultatoDellAnalisiControGestoSullaRFQ(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	th := b.rfq("RFQ-G39BIS-C")
	stalliIn := spia.stalli(b)
	for giro := 1; giro <= giriLucchetti; giro++ {
		codici := [3]string{fmt.Sprintf("910%04dA", giro), fmt.Sprintf("910%04d-10", giro), fmt.Sprintf("910%04d-20", giro)}
		a := b.sceso(th, codici[0]+".stp", []byte(fmt.Sprintf("ISO-10303-21; STEP sintetico del giro %d", giro)))
		var jobID int64
		b.riga(`SELECT job_id FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'allegato_id' = $1::text
			AND stato = 'pronto' ORDER BY job_id DESC LIMIT 1`, &jobID, a.AllegatoID.String())
		j, tent := b.inTentativo(jobID, "analisi@PC-G39BIS")
		r := worker.RisultatoAnalisi{AllegatoID: a.AllegatoID, VersioneAnalizzatore: b.s.Analizzatore.Versione,
			HashConfigurazione: b.s.Analizzatore.Hash(), TipoProposto: "cad_3d", Codice: codici[0], Confidenza: 80, Fonte: "step",
			Dettagli: strutturaACME(t, codici, "oggi")}
		dati, _ := json.Marshal(r)
		rfq, err := b.s.rfqDelRisultato(b.ctx, b.q, &j, nil)
		if err != nil || len(rfq) != 1 || rfq[0] != th {
			t.Fatalf("giro %d: le RFQ del risultato: %v %v", giro, rfq, err)
		}
		// il gesto: la RFQ FOR UPDATE
		gesto, err := b.pool.Begin(b.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.New(gesto).BloccaThread(b.ctx, th); err != nil {
			_ = gesto.Rollback(b.ctx)
			t.Fatal(err)
		}
		ris := make(chan esitoRisultato, 1)
		go func() {
			es, _, err := b.s.scriviRisultato(b.ctx, &j, tent, worker.RisultatoRichiesta{Esito: "ok", Dati: dati}, nil, rfq)
			ris <- esitoRisultato{es, err}
		}()
		if !spia.aspetta(false, 1) {
			_ = gesto.Rollback(b.ctx)
			t.Fatalf("giro %d: il risultato dell'analisi non ha aspettato la RFQ", giro)
		}
		// il gesto blocca la proposta di documento del file, dopo la RFQ (l'ordine di R5)
		_, errGesto := gesto.Exec(b.ctx, `SELECT 1 FROM documento_proposta WHERE allegato_id = $1 FOR UPDATE`, a.AllegatoID)
		if errGesto == nil {
			errGesto = gesto.Commit(b.ctx)
		} else {
			_ = gesto.Rollback(b.ctx)
		}
		res := attendi2(t, ris, fmt.Sprintf("giro %d: il risultato dell'analisi", giro))
		if errGesto != nil {
			t.Errorf("giro %d: il gesto durante il risultato dell'analisi: %v", giro, errGesto)
		}
		if res.es != scritturaFatta || res.err != nil {
			t.Errorf("giro %d: il risultato dell'analisi durante il gesto: esito %d, %v", giro, res.es, res.err)
		}
		var nodi int
		b.riga(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1`, &nodi, th)
		if nodi == 0 && res.err == nil {
			t.Errorf("giro %d: la struttura dello STEP non ha scritto proposte di nodo (la prova non arriva alla chiave esterna)", giro)
		}
	}
	if d := spia.stalli(b); d != stalliIn {
		t.Errorf("40P01: pg_stat_database.deadlocks da %d a %d", stalliIn, d)
	}
}

// ---------------------------------------------------------------- il ritento, dove un 40P01 resta possibile

// Le due prove che seguono chiudono apposta un ciclo che l'ordine unico non puo' evitare, perche' una delle due
// transazioni e' del banco e non lo segue (come una RFQ che il risultato non ha potuto leggere prima, o un lavoro
// fuori dall'ordine). PostgreSQL ne uccide una: se e' quella del servizio, il servizio la rifa' da capo e finisce, senza
// un 500 o un fallimento definitivo del job. Il contatore deadlocks qui cresce per costruzione, quindi non si guarda.
// La vittima la sceglie PostgreSQL, piu' o meno una volta su due (osservato il 21/10: da 1 a 3 giri su 3): i giri vanno
// avanti oltre giriLucchetti finche' il servizio e' stato la vittima almeno una volta, al piu' giriMassimiRitento, e
// un servizio mai vittima e' un FAIL (revisione G39-bis: «rifatto 0 su 3» passava). Senza ritento le due prove
// falliscono quindi salvo una volta su 2^giriMassimiRitento; le varianti deterministiche (40001) sono piu' sotto.

// TestG39bisRisultatoRifattoDopoUn40P01: il risultato dell'analisi senza le RFQ lette prima (rfq vuoto: la RFQ arriva
// con la chiave esterna, fuori ordine) contro il gesto sulla RFQ. Ogni giro: o il risultato e' la vittima e si rifa' (la
// riga del log), o la vittima e' il gesto; il risultato finisce sempre con il job chiuso.
func TestG39bisRisultatoRifattoDopoUn40P01(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	var registro bytes.Buffer
	b.s.Log = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelWarn}))
	th := b.rfq("RFQ-G39BIS-D")
	rifatti, giri := 0, 0
	for giro := 1; giro <= giriLucchetti || (rifatti == 0 && giro <= giriMassimiRitento); giro++ {
		giri = giro
		codici := [3]string{fmt.Sprintf("920%04dA", giro), fmt.Sprintf("920%04d-10", giro), fmt.Sprintf("920%04d-20", giro)}
		a := b.sceso(th, codici[0]+".stp", []byte(fmt.Sprintf("ISO-10303-21; STEP sintetico D del giro %d", giro)))
		var jobID int64
		b.riga(`SELECT job_id FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'allegato_id' = $1::text
			AND stato = 'pronto' ORDER BY job_id DESC LIMIT 1`, &jobID, a.AllegatoID.String())
		j, tent := b.inTentativo(jobID, "analisi@PC-G39BIS")
		r := worker.RisultatoAnalisi{AllegatoID: a.AllegatoID, VersioneAnalizzatore: b.s.Analizzatore.Versione,
			HashConfigurazione: b.s.Analizzatore.Hash(), TipoProposto: "cad_3d", Codice: codici[0], Confidenza: 80, Fonte: "step",
			Dettagli: strutturaACME(t, codici, "oggi")}
		dati, _ := json.Marshal(r)
		prima := strings.Count(registro.String(), "si rifa' da capo")
		gesto, err := b.pool.Begin(b.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.New(gesto).BloccaThread(b.ctx, th); err != nil {
			_ = gesto.Rollback(b.ctx)
			t.Fatal(err)
		}
		ris := make(chan esitoRisultato, 1)
		go func() {
			es, _, err := b.s.scriviRisultatoConRitento(b.ctx, &j, tent, worker.RisultatoRichiesta{Esito: "ok", Dati: dati}, nil, nil)
			ris <- esitoRisultato{es, err}
		}()
		if !spia.aspetta(false, 1) {
			_ = gesto.Rollback(b.ctx)
			t.Fatalf("giro %d: il risultato dell'analisi non ha aspettato la RFQ", giro)
		}
		_, errGesto := gesto.Exec(b.ctx, `SELECT 1 FROM documento_proposta WHERE allegato_id = $1 FOR UPDATE`, a.AllegatoID)
		if errGesto == nil {
			errGesto = gesto.Commit(b.ctx)
		} else {
			_ = gesto.Rollback(b.ctx)
		}
		res := attendi2(t, ris, fmt.Sprintf("giro %d: il risultato dell'analisi", giro))
		if res.es != scritturaFatta || res.err != nil {
			t.Errorf("giro %d: il risultato dell'analisi: esito %d, %v", giro, res.es, res.err)
		}
		rifatto := strings.Count(registro.String(), "si rifa' da capo") > prima
		switch {
		case rifatto && errGesto == nil:
			rifatti++
		case !rifatto && filerfq.ErroreTemporaneo(errGesto):
		default:
			t.Errorf("giro %d: atteso un 40P01 o nel risultato (rifatto) o nel gesto: rifatto=%v, gesto=%v", giro, rifatto, errGesto)
		}
		if jj, err := b.q.GetJob(b.ctx, jobID); err != nil || jj.Stato != db.StatoJobFatto {
			t.Errorf("giro %d: il job dell'analisi: %s %v", giro, jj.Stato, err)
		}
	}
	t.Logf("giri con il risultato rifatto dopo un 40P01: %d su %d", rifatti, giri)
	if rifatti == 0 {
		t.Errorf("in %d giri la vittima del 40P01 e' sempre stata il gesto: il ritento del risultato non e' stato provato", giri)
	}
}

// TestG39bisE1RifattoDopoUn40P01: E1′ contro una transazione che tiene il lucchetto per contenuto del suo file e poi
// chiede la RFQ FOR KEY SHARE (l'ordine dei risultati del worker di prima). Ogni giro: o E1′ e' la vittima e si rifa'
// (la riga del log), o la vittima e' l'altra; E1′ risponde sempre «caricato», mai il 500 «da verificare».
func TestG39bisE1RifattoDopoUn40P01(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	var registro bytes.Buffer
	b.svc.Log = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelWarn}))
	th := b.rfq("RFQ-G39BIS-E")
	rifatti, giri := 0, 0
	for giro := 1; giro <= giriLucchetti || (rifatti == 0 && giro <= giriMassimiRitento); giro++ {
		giri = giro
		c := []byte(fmt.Sprintf("%%PDF-1.4 SCALA file E del giro %d", giro))
		f := b.parziale(fmt.Sprintf("FILE-E-%d.pdf", giro), c)
		prima := strings.Count(registro.String(), "si rifa' da capo")
		cu := b.custodisci(f.Sha256)
		e1 := make(chan esitoE1, 1)
		go func() {
			es, err := b.svc.RegistraFile(b.ctx, th, b.utente, false, f)
			e1 <- esitoE1{es, err}
		}()
		if !spia.aspetta(true, 1) {
			cu.rilascia()
			t.Fatalf("giro %d: E1′ non si e' fermato sul lucchetto per contenuto", giro)
		}
		_, errAltra := cu.tx.Exec(b.ctx, `SELECT 1 FROM thread_offerta WHERE thread_id = $1 FOR KEY SHARE`, th)
		if errAltra == nil {
			errAltra = cu.tx.Commit(b.ctx)
		} else {
			cu.rilascia()
		}
		e := attendi2(t, e1, fmt.Sprintf("giro %d: E1′", giro))
		if e.err != nil || e.es.Esito != "caricato" {
			t.Errorf("giro %d: E1′: %+v %v", giro, e.es, e.err)
		}
		rifatto := strings.Count(registro.String(), "si rifa' da capo") > prima
		switch {
		case rifatto && errAltra == nil:
			rifatti++
		case !rifatto && filerfq.ErroreTemporaneo(errAltra):
		default:
			t.Errorf("giro %d: atteso un 40P01 o in E1′ (rifatto) o nell'altra transazione: rifatto=%v, altra=%v", giro, rifatto, errAltra)
		}
	}
	t.Logf("giri con E1′ rifatto dopo un 40P01: %d su %d", rifatti, giri)
	if rifatti == 0 {
		t.Errorf("in %d giri la vittima del 40P01 e' sempre stata l'altra transazione: il ritento di E1′ non e' stato provato", giri)
	}
}

// ---------------------------------------------------------------- il ritento, in modo deterministico (40001)

// Le prove che seguono non contano sulla vittima scelta da PostgreSQL. Il servizio lavora su un pool con le transazioni
// REPEATABLE READ (poolRipetibile, solo nelle prove: il servizio vero usa READ COMMITTED): una transazione del banco
// tiene FOR UPDATE una riga che il servizio deve bloccare, la riscrive, aspetta che il servizio sia fermo su quella riga
// e fa COMMIT; il servizio riceve allora un 40001 («could not serialize access due to concurrent update»), sempre. Con
// k transazioni in fila (serieDiConflitti) il servizio riceve k errori temporanei di fila: k = maxTentativi da' l'esito
// dopo l'ultimo tentativo (il 503 del result, l'errore di E1′ che diventa il 5xx «da verificare»), k = 1 un solo giro
// rifatto. 40P01 e 40001 sono la stessa classe (filerfq.ErroreTemporaneo), quindi la strada provata e' quella del 40P01.

// poolRipetibile: un pool sullo stesso database di prova, con default_transaction_isolation = repeatable read.
func poolRipetibile(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_isolation"] = "repeatable read"
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// registroCondiviso: il log del servizio, scritto dalle sue goroutine e letto dalla prova.
type registroCondiviso struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (r *registroCondiviso) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.Write(p)
}

func (r *registroCondiviso) conta(s string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Count(r.b.String(), s)
}

// serieDiConflitti: k transazioni del banco in fila sulla riga di blocca (con l'argomento arg). La prima tiene la riga e
// la riscrive, poi avvia() fa partire il servizio; per ogni tentativo del servizio: si aspetta che sia fermo sulla riga,
// la transazione dopo si mette in coda dietro di lui, quella di adesso fa COMMIT (il tentativo riceve il 40001) e quella
// dopo prende la riga e la riscrive prima che il servizio, dopo la sua pausa, ci torni.
func (b *bancoCiclo) serieDiConflitti(spia *spiaLucchetti, blocca, riscrivi string, arg any, k int, avvia func()) {
	b.t.Helper()
	prendi := func() (pgx.Tx, error) {
		tx, err := b.pool.Begin(b.ctx)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(b.ctx, blocca, arg); err != nil {
			_ = tx.Rollback(b.ctx)
			return nil, err
		}
		if _, err := tx.Exec(b.ctx, riscrivi, arg); err != nil {
			_ = tx.Rollback(b.ctx)
			return nil, err
		}
		return tx, nil
	}
	ora, err := prendi()
	if err != nil {
		b.t.Fatal(err)
	}
	avvia()
	type presa struct {
		tx  pgx.Tx
		err error
	}
	for n := 1; n <= k; n++ {
		if !spia.aspetta(false, 1) {
			_ = ora.Rollback(b.ctx)
			b.t.Fatalf("tentativo %d: il servizio non si e' fermato sulla riga", n)
		}
		dopo := make(chan presa, 1)
		if n < k {
			go func() {
				tx, err := prendi()
				dopo <- presa{tx, err}
			}()
			if !spia.aspetta(false, 2) {
				_ = ora.Rollback(b.ctx)
				b.t.Fatalf("tentativo %d: la transazione dopo non si e' messa in coda", n)
			}
		}
		if err := ora.Commit(b.ctx); err != nil {
			b.t.Fatalf("tentativo %d: COMMIT della transazione del banco: %v", n, err)
		}
		if n < k {
			p := attendi2(b.t, dopo, fmt.Sprintf("tentativo %d: la transazione dopo", n))
			if p.err != nil {
				b.t.Fatalf("tentativo %d: la transazione dopo: %v", n, p.err)
			}
			ora = p.tx
		}
	}
}

const workerG39bis = "analisi-g39bis"

// TestG39bisResult503DopoLUltimoTentativo: POST /api/v1/jobs/{id}/result dell'analisi di uno STEP, attraverso l'handler
// vero, con un errore temporaneo a ogni tentativo (40001 sulla riga del job, maxTentativiRisultato volte): 503, il job
// resta in corso con lo stesso tentativo e niente e' scritto; due righe «si rifa' da capo» e una «anche all'ultimo
// tentativo» nel log. Poi lo stesso result con un solo errore temporaneo: 204 dopo un giro rifatto, il job e' fatto e la
// struttura dello STEP c'e'. Prima del giro G39-bis la prima parte era un 422 con il fallimento definitivo del job.
func TestG39bisResult503DopoLUltimoTentativo(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	var registro registroCondiviso
	s := &Server{Pool: poolRipetibile(t), Log: slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelWarn})),
		Staging: b.staging, Analizzatore: b.s.Analizzatore}
	mux := http.NewServeMux()
	s.Registra(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if _, err := b.q.UpsertWorkerCredenziale(b.ctx, db.UpsertWorkerCredenzialeParams{WorkerNome: workerG39bis,
		WorkerTipo: db.WorkerTipoAnalisi, TokenHash: ImprontaToken(tokenDi(workerG39bis)), Caselle: []uuid.UUID{}}); err != nil {
		t.Fatal(err)
	}
	th := b.rfq("RFQ-G39BIS-F")
	codici := [3]string{"9300001A", "9300001-10", "9300001-20"}
	a := b.sceso(th, codici[0]+".stp", []byte("ISO-10303-21; STEP sintetico F"))
	var jobID int64
	b.riga(`SELECT job_id FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'allegato_id' = $1::text
		AND stato = 'pronto' ORDER BY job_id DESC LIMIT 1`, &jobID, a.AllegatoID.String())
	_, tent := b.inTentativo(jobID, workerG39bis)
	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: a.AllegatoID, VersioneAnalizzatore: b.s.Analizzatore.Versione,
		HashConfigurazione: b.s.Analizzatore.Hash(), TipoProposto: "cad_3d", Codice: codici[0], Confidenza: 80, Fonte: "step",
		Dettagli: strutturaACME(t, codici, "oggi")})
	corpo, _ := json.Marshal(worker.RisultatoRichiesta{Esito: "ok", Dati: dati, WorkerID: workerG39bis, LeaseToken: tent.LeaseToken.String()})
	invia := func(risposta chan int) func() {
		return func() {
			go func() {
				req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/jobs/%d/result", srv.URL, jobID), bytes.NewReader(corpo))
				req.Header.Set("X-Cockpit-Token", tokenDi(workerG39bis))
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					risposta <- -1
					return
				}
				resp.Body.Close()
				risposta <- resp.StatusCode
			}()
		}
	}
	const bloccaJob = `SELECT 1 FROM job WHERE job_id = $1 FOR UPDATE`
	const riscriviJob = `UPDATE job SET lease_fino_a = lease_fino_a WHERE job_id = $1`

	// 1. un errore temporaneo a ogni tentativo
	r1 := make(chan int, 1)
	b.serieDiConflitti(spia, bloccaJob, riscriviJob, jobID, maxTentativiRisultato, invia(r1))
	if code := attendi2(t, r1, "il result con un errore temporaneo a ogni tentativo"); code != http.StatusServiceUnavailable {
		t.Errorf("result con un errore temporaneo a ogni tentativo: %d, atteso 503", code)
	}
	jj, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if jj.Stato != db.StatoJobInCorso || !jj.LeaseToken.Valid || jj.LeaseToken.UUID != tent.LeaseToken {
		t.Errorf("dopo il 503 il job deve restare in corso con lo stesso tentativo: %s %v", jj.Stato, jj.LeaseToken)
	}
	var nodi int
	b.riga(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1`, &nodi, th)
	if nodi != 0 {
		t.Errorf("dopo il 503 la struttura dello STEP non deve esserci: %d proposte di nodo", nodi)
	}
	if n := registro.conta("si rifa' da capo"); n != maxTentativiRisultato-1 {
		t.Errorf("giri rifatti nel log: %d, attesi %d", n, maxTentativiRisultato-1)
	}
	if n := registro.conta("anche all'ultimo tentativo"); n != 1 {
		t.Errorf("righe «anche all'ultimo tentativo» nel log: %d, attesa 1", n)
	}

	// 2. lo stesso result, con un solo errore temporaneo
	r2 := make(chan int, 1)
	b.serieDiConflitti(spia, bloccaJob, riscriviJob, jobID, 1, invia(r2))
	if code := attendi2(t, r2, "il result con un errore temporaneo"); code != http.StatusNoContent {
		t.Errorf("result ripetuto con un solo errore temporaneo: %d, atteso 204", code)
	}
	if jj, err := b.q.GetJob(b.ctx, jobID); err != nil || jj.Stato != db.StatoJobFatto {
		t.Errorf("il job dopo il result ripetuto: %s %v", jj.Stato, err)
	}
	b.riga(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1`, &nodi, th)
	if nodi == 0 {
		t.Errorf("dopo il result ripetuto la struttura dello STEP non c'e'")
	}
	if n := registro.conta("si rifa' da capo"); n != maxTentativiRisultato {
		t.Errorf("giri rifatti nel log dopo il result ripetuto: %d, attesi %d (uno in piu')", n, maxTentativiRisultato)
	}
}

// TestG39bisE1ErroreTemporaneoDopoLUltimoTentativo: E1′ (filerfq.RegistraFile) con un errore temporaneo a ogni tentativo
// (40001 sulla riga della RFQ, che E1′ prende per prima: 3 volte, maxTentativiRegistrazione) restituisce l'errore
// temporaneo, che l'handler (transport/web/sorgente_carica.go) rende come il 5xx «da verificare» di sempre: il file non
// e' registrato e il parziale non resta. Poi lo stesso file con un solo errore temporaneo: «caricato» dopo un giro rifatto.
func TestG39bisE1ErroreTemporaneoDopoLUltimoTentativo(t *testing.T) {
	b := preparaBancoCiclo(t)
	spia := nuovaSpia(t)
	var registro registroCondiviso
	svc := *b.svc
	svc.Pool = poolRipetibile(t)
	svc.Log = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelWarn}))
	th := b.rfq("RFQ-G39BIS-G")
	c := []byte("%PDF-1.4 SCALA file G")
	const bloccaRFQ = `SELECT 1 FROM thread_offerta WHERE thread_id = $1 FOR UPDATE`
	const riscriviRFQ = `UPDATE thread_offerta SET oggetto = oggetto WHERE thread_id = $1`
	carica := func(f filerfq.FileArrivato, e1 chan esitoE1) func() {
		return func() {
			go func() {
				es, err := svc.RegistraFile(b.ctx, th, b.utente, false, f)
				e1 <- esitoE1{es, err}
			}()
		}
	}

	// 1. un errore temporaneo a ogni tentativo
	f1 := b.parziale("FILE-G.pdf", c)
	e1 := make(chan esitoE1, 1)
	b.serieDiConflitti(spia, bloccaRFQ, riscriviRFQ, th, 3, carica(f1, e1))
	r := attendi2(t, e1, "E1′ con un errore temporaneo a ogni tentativo")
	if !filerfq.ErroreTemporaneo(r.err) {
		t.Errorf("E1′ con un errore temporaneo a ogni tentativo: atteso l'errore temporaneo, ricevuto %+v %v", r.es, r.err)
	}
	var file int
	b.riga(`SELECT count(*) FROM allegato WHERE nome_file = 'FILE-G.pdf'`, &file)
	if file != 0 {
		t.Errorf("dopo l'ultimo tentativo il file non deve essere registrato: %d", file)
	}
	if _, err := os.Stat(f1.Parte); !os.IsNotExist(err) {
		t.Errorf("dopo l'ultimo tentativo il parziale non deve restare: %v", err)
	}
	if n := registro.conta("si rifa' da capo"); n != 2 {
		t.Errorf("giri rifatti nel log: %d, attesi 2 (3 tentativi)", n)
	}

	// 2. lo stesso file, con un solo errore temporaneo
	f2 := b.parziale("FILE-G.pdf", c)
	e2 := make(chan esitoE1, 1)
	b.serieDiConflitti(spia, bloccaRFQ, riscriviRFQ, th, 1, carica(f2, e2))
	r = attendi2(t, e2, "E1′ con un errore temporaneo")
	if r.err != nil || r.es.Esito != "caricato" {
		t.Errorf("E1′ con un solo errore temporaneo: %+v %v", r.es, r.err)
	}
	b.riga(`SELECT count(*) FROM allegato WHERE nome_file = 'FILE-G.pdf'`, &file)
	if file != 1 {
		t.Errorf("file registrati dopo il giro rifatto: %d, atteso 1", file)
	}
	if n := registro.conta("si rifa' da capo"); n != 3 {
		t.Errorf("giri rifatti nel log dopo il secondo invio: %d, attesi 3 (uno in piu')", n)
	}
}
