//go:build integrazione

// L4 — voce 2.3: l'upload di un allegato è legato al tentativo (M7, M8, M13).
//
// Il file non entra nello staging del server perché il worker lo ha scritto lì — il worker può stare
// su un altro PC — ma perché lo ha CARICATO con PUT dentro un tentativo valido, e perché il result
// valido dello stesso tentativo lo ha promosso dopo aver verificato lo sha256. Ogni test qui prova un
// punto in cui, senza il token nel nome del file, un tentativo scaduto avrebbe consegnato un file.
package workerapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

const tokenProva = "token-di-prova-lungo-abbastanza"

// banco è tutto ciò che serve per far parlare un worker finto con il server vero: DB pulito, staging
// in una cartella temporanea, un allegato mai scaricato e il suo job stage_allegato già in coda.
type banco struct {
	t        *testing.T
	pool     *pgxpool.Pool
	q        *db.Queries
	ctx      context.Context
	s        *Server
	srv      *httptest.Server
	cartella string
	allegato db.Allegato
	msg      db.Messaggio
	casella  uuid.UUID // la casella della presenza: il claim deve dichiararla (voce 2.2)
	job      db.Job    // il job accodato, ancora 'pronto'
}

func preparaBanco(t *testing.T, maxUpload int64) *banco {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	cartella := t.TempDir()
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Staging: cartella, MaxUpload: maxUpload,
		Analizzatore: coda.Analizzatore{Versione: 1}}
	mux := http.NewServeMux()
	s.Registra(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	b := &banco{t: t, pool: pool, q: q, ctx: ctx, s: s, srv: srv, cartella: cartella}
	b.allegato, b.msg = b.messaggioConAllegato("disegno.pdf", "pdf")
	pr, err := q.PresenzaDaAprire(ctx, b.msg.MessaggioID)
	if err != nil {
		t.Fatal(err)
	}
	b.casella = pr.CasellaID
	_, j, err := coda.AccodaStage(ctx, q, staging.FileStaging{}, b.allegato, b.msg, copiaDi(pr), 1)
	if err != nil || j == nil {
		t.Fatalf("accoda stage: job=%v err=%v", j, err)
	}
	b.job = *j
	return b
}

func (b *banco) messaggioConAllegato(nome, ext string) (db.Allegato, db.Messaggio) {
	b.t.Helper()
	var convID, msgID, allID, casellaID uuid.UUID
	chiave := "<" + strings.ReplaceAll(nome, ".", "-") + "@acme.example>"
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', $1, now()) RETURNING conversazione_id`, "CONV-"+nome).Scan(&convID); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, mittente_indirizzo)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ 2.3', 'mario.rossi@acme.example') RETURNING messaggio_id`,
		chiave, convID).Scan(&msgID); err != nil {
		b.t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO messaggio_outlook (messaggio_id) VALUES ($1)`, msgID); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO casella (canale, indirizzo, nome, condivisa) VALUES ('outlook','commerciale@azienda.it','Commerciale',true)
		ON CONFLICT (canale, indirizzo) DO UPDATE SET nome = EXCLUDED.nome RETURNING casella_id`).Scan(&casellaID); err != nil {
		b.t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, cartella, ricevuto_il)
		VALUES ($1, $2, $3, 'Posta in arrivo', now())`, msgID, casellaID, "ENTRY-"+nome); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, ricevuto_il)
		VALUES ($1, 1, $2, $3, 'file', 'outlook', 2048, now()) RETURNING allegato_id`, msgID, nome, ext).Scan(&allID); err != nil {
		b.t.Fatal(err)
	}
	a, err := b.q.GetAllegato(b.ctx, allID)
	if err != nil {
		b.t.Fatal(err)
	}
	m, err := b.q.GetMessaggio(b.ctx, msgID)
	if err != nil {
		b.t.Fatal(err)
	}
	return a, m
}

// claim prende il job come farebbe il worker: nasce un tentativo con il suo token.
func copiaDi(pr db.PresenzaDaAprireRow) coda.Copia {
	return coda.Copia{CasellaID: pr.CasellaID, CasellaNome: pr.CasellaNome, EntryID: pr.EntryID}
}

// tokenDi è il segreto individuale di un worker in questi test (voce 2.4): uno per nome, mai lo
// stesso per due. Il token condiviso non esiste più, e un banco che ne usasse uno solo proverebbe
// una cosa che il server non fa più.
func tokenDi(worker string) string { return "token-individuale-di-" + worker }

// censisci crea (o aggiorna) la credenziale di un worker con il suo token individuale. Senza, il
// worker non si autentica: dalla voce 2.4 il token È l'identità.
func (b *banco) censisci(worker string, tipo db.WorkerTipo) {
	b.t.Helper()
	if _, err := b.q.UpsertWorkerCredenziale(b.ctx, db.UpsertWorkerCredenzialeParams{
		WorkerNome: worker, WorkerTipo: tipo, TokenHash: ImprontaToken(tokenDi(worker)),
		Caselle: []uuid.UUID{b.casella},
	}); err != nil {
		b.t.Fatalf("credenziale di %s: %v", worker, err)
	}
}

// claim prende il job come farebbe un worker che SERVE la casella della presenza: dalla voce 2.2 un
// job di una casella va solo a chi la dichiara. Censisce anche la credenziale, perché un worker che
// non è censito non si autentica nemmeno.
func (b *banco) claim(worker string) coda.Tentativo {
	b.t.Helper()
	b.censisci(worker, db.WorkerTipoOutlook)
	j, err := coda.Claim(b.ctx, b.q, db.WorkerTipoOutlook, worker, coda.Destinazione{Caselle: []uuid.UUID{b.casella}}, 0)
	if err != nil || j == nil {
		b.t.Fatalf("claim: job=%v err=%v", j, err)
	}
	return coda.Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: worker}
}

// put carica il corpo come farebbe il worker. contentLength < 0 = trasferimento a blocchi (chunked).
func (b *banco) put(t coda.Tentativo, allegato uuid.UUID, corpo io.Reader, contentLength int64) *http.Response {
	b.t.Helper()
	url := fmt.Sprintf("%s/api/v1/allegati/%s/file?job_id=%d&lease_token=%s&worker_id=%s",
		b.srv.URL, allegato, t.JobID, t.LeaseToken, t.WorkerID)
	req, err := http.NewRequest(http.MethodPut, url, corpo)
	if err != nil {
		b.t.Fatal(err)
	}
	req.ContentLength = contentLength
	req.Header.Set("X-Cockpit-Token", tokenDi(t.WorkerID))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatalf("PUT: %v", err)
	}
	return resp
}

func (b *banco) result(t coda.Tentativo, r worker.RisultatoStage) *http.Response {
	b.t.Helper()
	dati, _ := json.Marshal(r)
	corpo, _ := json.Marshal(worker.RisultatoRichiesta{Esito: "ok", Dati: dati, WorkerID: t.WorkerID, LeaseToken: t.LeaseToken.String()})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/jobs/%d/result", b.srv.URL, t.JobID), bytes.NewReader(corpo))
	req.Header.Set("X-Cockpit-Token", tokenDi(t.WorkerID))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatalf("result: %v", err)
	}
	return resp
}

// parte è dove QUESTO tentativo carica: si sa subito, perché dipende da chi carica e non da che
// cosa sta caricando.
func (b *banco) parte(t coda.Tentativo) string {
	b.t.Helper()
	return staging.PercorsoParte(b.cartella, b.allegato.AllegatoID, t.LeaseToken)
}

// contenuto è dove finisce un contenuto: lo dice il suo hash, quindi si sa solo quando il
// trasferimento è finito. I test lo conoscono in anticipo perché sono loro a fabbricare i byte.
func (b *banco) contenuto(sha string) string {
	b.t.Helper()
	p, err := staging.PercorsoContenuto(b.cartella, sha, b.allegato.NomeFile)
	if err != nil {
		b.t.Fatal(err)
	}
	return p
}

func (b *banco) fileDiStaging() []string {
	b.t.Helper()
	var out []string
	_ = filepath.WalkDir(b.cartella, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(b.cartella, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func (b *banco) allegatoOra() db.Allegato {
	b.t.Helper()
	a, err := b.q.GetAllegato(b.ctx, b.allegato.AllegatoID)
	if err != nil {
		b.t.Fatal(err)
	}
	return a
}

func (b *banco) jobOra() db.Job {
	b.t.Helper()
	j, err := b.q.GetJob(b.ctx, b.job.JobID)
	if err != nil {
		b.t.Fatal(err)
	}
	return j
}

func (b *banco) scadeIlLease() {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, `UPDATE job SET lease_fino_a = now() - interval '1 minute' WHERE job_id = $1`, b.job.JobID); err != nil {
		b.t.Fatal(err)
	}
	if _, err := b.q.RilasciaLeaseScaduti(b.ctx); err != nil {
		b.t.Fatal(err)
	}
}

func contenutoCasuale(n int, seme int64) ([]byte, string) {
	c := make([]byte, n)
	rand.New(rand.NewSource(seme)).Read(c)
	return c, shaDi(c)
}

func shaDi(c []byte) string {
	h := sha256.Sum256(c)
	return hex.EncodeToString(h[:])
}

func stato(t *testing.T, resp *http.Response, atteso int) string {
	t.Helper()
	corpo, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != atteso {
		t.Fatalf("stato %d, atteso %d: %s", resp.StatusCode, atteso, corpo)
	}
	return string(corpo)
}

func esiste(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func hashDi(t *testing.T, p string) string {
	t.Helper()
	c, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("leggo %s: %v", p, err)
	}
	h := sha256.Sum256(c)
	return hex.EncodeToString(h[:])
}

// M7 — l'upload scrive .parte.<token>; è il result valido a promuoverlo; e si può rifare.
func TestM7UploadLegatoAlTentativoPromossoDalResult(t *testing.T) {
	b := preparaBanco(t, 0)
	contenuto, sha := contenutoCasuale(300_000, 1)
	tA := b.claim("outlook@PC-A")
	parte, def := b.parte(tA), b.contenuto(sha)

	// l'upload da solo NON consegna niente: c'è il .parte del tentativo, non l'allegato
	stato(t, b.put(tA, b.allegato.AllegatoID, bytes.NewReader(contenuto), int64(len(contenuto))), 204)
	if !esiste(parte) {
		t.Fatalf("dopo il PUT manca %s; staging: %v", filepath.Base(parte), b.fileDiStaging())
	}
	if esiste(def) {
		t.Errorf("il file è definitivo prima del result: %s", def)
	}
	if a := b.allegatoOra(); a.Stato != db.StatoAllegatoGrezzo || a.PathStaging.Valid {
		t.Errorf("l'upload ha toccato l'allegato: stato=%s path=%q", a.Stato, a.PathStaging.String)
	}

	// il result valido promuove: definitivo con l'hash atteso, .parte sparito, allegato in staging
	stato(t, b.result(tA, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: sha, Bytes: int64(len(contenuto))}), 204)
	if !esiste(def) || hashDi(t, def) != sha {
		t.Fatalf("file definitivo assente o diverso: %s", def)
	}
	if esiste(parte) {
		t.Errorf(".parte ancora presente dopo la promozione: %s", parte)
	}
	a := b.allegatoOra()
	if a.Stato == db.StatoAllegatoGrezzo || a.PathStaging.String != def || a.Sha256.String != sha || a.Bytes.Int64 != int64(len(contenuto)) {
		t.Errorf("allegato non in staging: stato=%s path=%q sha=%s bytes=%d", a.Stato, a.PathStaging.String, a.Sha256.String, a.Bytes.Int64)
	}
	if j := b.jobOra(); j.Stato != db.StatoJobFatto {
		t.Errorf("job in stato %s, atteso fatto (errore: %q)", j.Stato, j.Errore.String)
	}
	if n := testutil.Conta(t, b.pool, "documento_proposta"); n < 1 {
		t.Errorf("dopo lo staging non c'è nessuna proposta: dopoStaging non è stato eseguito")
	}

	// RIPETIBILE (M7): un secondo download dello stesso allegato — «Riscarica» dopo che il file è
	// sparito — rifà tutto il giro. Dal blocco 4A il contenuto nuovo ha un nome nuovo, perché il nome
	// E' il contenuto: non sostituisce il file di prima, ne prende uno suo.
	if err := os.Remove(def); err != nil {
		t.Fatal(err)
	}
	pr, err := b.q.PresenzaDaAprire(b.ctx, b.msg.MessaggioID)
	if err != nil {
		t.Fatal(err)
	}
	esito, j2, err := coda.AccodaStage(b.ctx, b.q, staging.FileStaging{}, b.allegatoOra(), b.msg, copiaDi(pr), 1)
	if err != nil || esito != coda.StageAccodato || j2 == nil {
		t.Fatalf("riscarica: esito=%s job=%v err=%v", esito, j2, err)
	}
	b.job = *j2
	contenuto2, sha2 := contenutoCasuale(120_000, 2)
	tB := b.claim("outlook@PC-A")
	stato(t, b.put(tB, b.allegato.AllegatoID, bytes.NewReader(contenuto2), int64(len(contenuto2))), 204)
	stato(t, b.result(tB, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: sha2, Bytes: int64(len(contenuto2))}), 204)
	def2 := b.contenuto(sha2)
	if !esiste(def2) || hashDi(t, def2) != sha2 {
		t.Fatalf("il secondo download non ha portato il contenuto nuovo: %v", b.fileDiStaging())
	}
	for _, f := range b.fileDiStaging() {
		if strings.Contains(f, ".parte.") {
			t.Errorf("file parziale rimasto nello staging: %s", f)
		}
	}
	if a := b.allegatoOra(); a.Sha256.String != sha2 || a.PathStaging.String != def2 {
		t.Errorf("allegato dopo la riscarica: sha=%s path=%q, atteso %s / %s", a.Sha256.String, a.PathStaging.String, sha2, def2)
	}
}

// M8 — 80 MB con limite 64: 413, niente sul disco, stato dell'allegato in errore con il motivo.
func TestM8AllegatoOltreIlLimite(t *testing.T) {
	b := preparaBanco(t, 64<<20)
	tA := b.claim("outlook@PC-A")
	parte := b.parte(tA)

	// con Content-Length dichiarato il server rifiuta PRIMA di leggere il corpo
	zeri := io.LimitReader(zeroReader{}, 80<<20)
	corpo := stato(t, b.put(tA, b.allegato.AllegatoID, zeri, 80<<20), http.StatusRequestEntityTooLarge)
	if !strings.Contains(corpo, "64 MB") {
		t.Errorf("il 413 non dice il limite: %s", corpo)
	}
	if esiste(parte) {
		t.Errorf("un upload rifiutato ha lasciato %s", parte)
	}
	a := b.allegatoOra()
	if a.Stato != db.StatoAllegatoErrore || !strings.Contains(a.Errore.String, "64 MB") {
		t.Errorf("lo stato dell'allegato non rende visibile il motivo: stato=%s errore=%q", a.Stato, a.Errore.String)
	}

	// senza Content-Length (trasferimento a blocchi) il server legge fino al limite e poi scarta:
	// un byte oltre i 64 MB basta
	oltre := io.LimitReader(zeroReader{}, 64<<20+1)
	stato(t, b.put(tA, b.allegato.AllegatoID, oltre, -1), http.StatusRequestEntityTooLarge)
	if esiste(parte) {
		t.Errorf("il file parziale di un upload oltre il limite non è stato rimosso: %s", parte)
	}
	for _, f := range b.fileDiStaging() {
		t.Errorf("file inatteso nello staging dopo due 413: %s", f)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// M13 — il tentativo A carica metà file, il lease scade, B carica e chiude; poi arriva la coda di A.
// L'upload di A è rifiutato (409) e il suo .parte.<tA> rimosso; il definitivo è quello di B; il
// result di A è 409.
func TestM13UploadConTokenVecchio(t *testing.T) {
	b := preparaBanco(t, 0)
	contenutoA, _ := contenutoCasuale(200_000, 3)
	contenutoB, shaB := contenutoCasuale(150_000, 4)
	tA := b.claim("outlook@PC-A")
	parteA, def := b.parte(tA), b.contenuto(shaB)

	// A comincia a caricare e si ferma a metà: il corpo resta aperto
	pr, pw := io.Pipe()
	rispostaA := make(chan *http.Response, 1)
	go func() { rispostaA <- b.put(tA, b.allegato.AllegatoID, pr, -1) }()
	if _, err := pw.Write(contenutoA[:100_000]); err != nil {
		t.Fatal(err)
	}
	// il server ha già aperto il .parte di A
	attendi(t, func() bool { return esiste(parteA) }, "il server non ha aperto il file parziale di A")

	// il lease di A scade e il job torna in coda; B lo prende, carica tutto e chiude con il result
	b.scadeIlLease()
	tB := b.claim("outlook@PC-A")
	if tB.LeaseToken == tA.LeaseToken {
		t.Fatal("B ha lo stesso token di A: il tentativo non è stato rinnovato")
	}
	parteB := b.parte(tB)
	stato(t, b.put(tB, b.allegato.AllegatoID, bytes.NewReader(contenutoB), int64(len(contenutoB))), 204)
	stato(t, b.result(tB, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: shaB, Bytes: int64(len(contenutoB))}), 204)
	if !esiste(def) || hashDi(t, def) != shaB {
		t.Fatalf("il file definitivo non è quello di B")
	}
	if esiste(parteB) {
		t.Errorf(".parte di B ancora presente dopo la promozione")
	}

	// adesso arriva la coda dell'upload di A
	if _, err := pw.Write(contenutoA[100_000:]); err != nil {
		t.Fatal(err)
	}
	pw.Close()
	select {
	case resp := <-rispostaA:
		stato(t, resp, 409)
	case <-time.After(10 * time.Second):
		t.Fatal("l'upload di A non ha ricevuto risposta")
	}
	if esiste(parteA) {
		t.Errorf("il .parte del tentativo scaduto non è stato rimosso: %s", parteA)
	}
	if hashDi(t, def) != shaB {
		t.Errorf("il file definitivo è cambiato dopo l'upload tardivo di A")
	}

	// e il result di A: 409, e niente si muove
	stato(t, b.result(tA, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: shaB, Bytes: int64(len(contenutoB))}), 409)
	if a := b.allegatoOra(); a.PathStaging.String != def || a.Sha256.String != shaB {
		t.Errorf("il result di A ha toccato l'allegato: path=%q sha=%s", a.PathStaging.String, a.Sha256.String)
	}
	if j := b.jobOra(); j.Stato != db.StatoJobFatto || j.Tentativi != 2 {
		t.Errorf("job: stato=%s tentativi=%d, atteso fatto con 2 tentativi", j.Stato, j.Tentativi)
	}
	if fs := b.fileDiStaging(); len(fs) != 1 {
		t.Errorf("nello staging deve restare il solo file definitivo, ci sono: %v", fs)
	}
}

// Un file arrivato con un hash diverso da quello dichiarato non è un contenuto sbagliato ma un
// trasferimento andato male: il job torna in coda (non fallisce per sempre) e il .parte sparisce.
func TestHashDiversoRimetteInCodaSenzaConsegnare(t *testing.T) {
	b := preparaBanco(t, 0)
	contenuto, shaVero := contenutoCasuale(50_000, 5)
	tA := b.claim("outlook@PC-A")
	parte, def := b.parte(tA), b.contenuto(shaVero)
	stato(t, b.put(tA, b.allegato.AllegatoID, bytes.NewReader(contenuto), int64(len(contenuto))), 204)
	corpo := stato(t, b.result(tA, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: strings.Repeat("0", 64), Bytes: int64(len(contenuto))}), 422)
	if !strings.Contains(corpo, "sha256") {
		t.Errorf("il 422 non spiega che l'hash non torna: %s", corpo)
	}
	if esiste(def) || esiste(parte) {
		t.Errorf("un file con l'hash sbagliato è rimasto nello staging: %v", b.fileDiStaging())
	}
	if j := b.jobOra(); j.Stato != db.StatoJobPronto {
		t.Errorf("job in stato %s, atteso pronto (ritentabile); errore: %q", j.Stato, j.Errore.String)
	}
	if a := b.allegatoOra(); a.Stato != db.StatoAllegatoGrezzo || a.PathStaging.Valid {
		t.Errorf("l'allegato è stato toccato: stato=%s path=%q", a.Stato, a.PathStaging.String)
	}
}

// Un result senza upload prima non consegna niente: il worker deve caricare il file, non dichiararne
// il percorso. E un tentativo valido non può depositare file a nome di un allegato che non è il suo.
func TestResultSenzaUploadEUploadSuAltroAllegato(t *testing.T) {
	b := preparaBanco(t, 0)
	tA := b.claim("outlook@PC-A")
	corpo := stato(t, b.result(tA, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: strings.Repeat("a", 64), Bytes: 10}), 422)
	if !strings.Contains(corpo, "PUT /api/v1/allegati") {
		t.Errorf("il 422 non dice che cosa manca: %s", corpo)
	}
	if j := b.jobOra(); j.Stato != db.StatoJobFallito {
		t.Errorf("job in stato %s, atteso fallito (definitivo)", j.Stato)
	}

	b2 := preparaBanco(t, 0)
	altro, _ := b2.messaggioConAllegato("altro.pdf", "pdf")
	tB := b2.claim("outlook@PC-A")
	stato(t, b2.put(tB, altro.AllegatoID, bytes.NewReader([]byte("x")), 1), 422)
	if fs := b2.fileDiStaging(); len(fs) != 0 {
		t.Errorf("un upload rifiutato ha scritto: %v", fs)
	}
}

func attendi(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	scadenza := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(scadenza) {
			t.Fatal(msg)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
