//go:build integrazione

// L4 — blocco 4A: uno stesso contenuto occupa un posto solo nello staging.
//
// Il difetto si vedeva in una prova reale: due messaggi con lo stesso DISEGNI.zip da 6.398.094 byte,
// scaricati praticamente insieme. La deduplica per hash che c'era (AccodaStage) non poteva accorgersi
// di niente, perche' prima di scaricare da Outlook lo sha256 non lo conosce nessuno: la seconda copia
// era gia' in coda quando la prima ha rivelato il proprio. Con lo staging organizzato per MESSAGGIO
// quelle due copie finivano in due cartelle diverse, e lo zip estratto raddoppiava a sua volta.
//
// Ora il nome di un contenuto E' il suo sha256. Quella corsa non ha piu' un perdente: chi arriva
// secondo trova il file gia' al suo posto, con l'hash giusto, e non scrive niente.
package workerapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
)

// claimJob e' claim, ma dice anche QUALE job ha preso: con due download in coda insieme serve.
func (b *banco) claimJob(worker string) (db.Job, jobs.Tentativo) {
	b.t.Helper()
	b.censisci(worker, db.WorkerTipoOutlook)
	j, err := jobs.Claim(b.ctx, b.q, db.WorkerTipoOutlook, worker, jobs.Destinazione{Caselle: []uuid.UUID{b.casella}}, 0)
	if err != nil || j == nil {
		b.t.Fatalf("claim: job=%v err=%v", j, err)
	}
	return *j, jobs.Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: worker}
}

// accodaDownload mette in coda il download di un allegato qualunque del banco.
func (b *banco) accodaDownload(a db.Allegato, m db.Messaggio) {
	b.t.Helper()
	pr, err := b.q.PresenzaDaAprire(b.ctx, m.MessaggioID)
	if err != nil {
		b.t.Fatal(err)
	}
	_, j, err := jobs.AccodaStage(b.ctx, b.q, jobs.FileStaging{}, a, m, copiaDi(pr), 1)
	if err != nil || j == nil {
		b.t.Fatalf("accoda stage di %s: job=%v err=%v", a.NomeFile, j, err)
	}
}

// contenuti elenca i file sotto _contenuti, con il percorso relativo allo staging.
func (b *banco) contenuti() []string {
	b.t.Helper()
	var out []string
	radice := filepath.Join(b.staging, jobs.CartellaContenuti)
	_ = filepath.WalkDir(radice, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(b.staging, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

type voceRegistrata struct{ nome, percorso, sha string }

// figliDi elenca le voci registrate per un allegato contenitore (le voci di uno zip).
func (b *banco) figliDi(contenitore uuid.UUID) []voceRegistrata {
	b.t.Helper()
	righe, err := b.pool.Query(b.ctx,
		`SELECT nome_file, coalesce(path_staging,''), coalesce(sha256,'') FROM allegato
		 WHERE contenitore_id = $1 ORDER BY indice`, contenitore)
	if err != nil {
		b.t.Fatal(err)
	}
	defer righe.Close()
	var out []voceRegistrata
	for righe.Next() {
		var v voceRegistrata
		if err := righe.Scan(&v.nome, &v.percorso, &v.sha); err != nil {
			b.t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

// Due allegati diversi, lo stesso contenuto, scaricati INSIEME: un file solo sul disco.
//
// I due download vengono presi in carico entrambi prima che l'uno sappia dell'altro: e' la corsa
// vera, quella che la deduplica in accodamento non puo' vedere.
func TestStessoContenutoUnSoloFile(t *testing.T) {
	b := preparaBanco(t, 0)
	secondo, msg2 := b.messaggioConAllegato("disegno-inoltrato.pdf", "pdf")
	b.accodaDownload(secondo, msg2)

	contenuto, sha := contenutoCasuale(80_000, 11)

	// tutti e due in carico PRIMA che uno dei due consegni
	jA, tA := b.claimJob("outlook@PC-A")
	jB, tB := b.claimJob("outlook@PC-B")
	allA, allB := allegatoDi(jA.Payload), allegatoDi(jB.Payload)
	if allA == allB {
		t.Fatalf("i due claim hanno preso lo stesso download (%s)", allA)
	}

	for _, c := range []struct {
		t jobs.Tentativo
		a uuid.UUID
	}{{tA, allA}, {tB, allB}} {
		stato(t, b.put(c.t, c.a, bytes.NewReader(contenuto), int64(len(contenuto))), 204)
		stato(t, b.result(c.t, api.RisultatoStage{AllegatoID: c.a, Sha256: sha, Bytes: int64(len(contenuto))}), 204)
	}

	if fs := b.contenuti(); len(fs) != 1 {
		t.Errorf("lo stesso contenuto occupa %d file: %v", len(fs), fs)
	}
	uno, err := b.q.GetAllegato(b.ctx, allA)
	if err != nil {
		t.Fatal(err)
	}
	altro, err := b.q.GetAllegato(b.ctx, allB)
	if err != nil {
		t.Fatal(err)
	}
	if uno.PathStaging.String != altro.PathStaging.String {
		t.Errorf("due percorsi per lo stesso contenuto:\n  %s\n  %s", uno.PathStaging.String, altro.PathStaging.String)
	}
	if !esiste(uno.PathStaging.String) || hashDi(t, uno.PathStaging.String) != sha {
		t.Errorf("il contenuto non c'e' o non e' quello: %s", uno.PathStaging.String)
	}
	if uno.Stato == db.StatoAllegatoGrezzo || altro.Stato == db.StatoAllegatoGrezzo {
		t.Errorf("uno dei due allegati e' rimasto grezzo: %s / %s", uno.Stato, altro.Stato)
	}
	// e nessun file parziale e' sopravvissuto: quello del secondo va buttato, non promosso
	for _, f := range b.fileDiStaging() {
		if strings.Contains(f, ".parte.") {
			t.Errorf("file parziale rimasto: %s", f)
		}
	}
}

// Uno zip che contiene due volte lo stesso disegno con due nomi interni diversi: due allegati figli,
// un file solo. E' il caso normale quando un cliente rimanda la stessa commessa con una revisione in
// piu' e l'archivio ripete i disegni che non sono cambiati.
func TestVociUgualiDentroLoZipUnFileSolo(t *testing.T) {
	b := preparaBanco(t, 0)
	zipAllegato, msgZip := b.messaggioConAllegato("archivio.zip", "zip")
	b.accodaDownload(zipAllegato, msgZip)

	dentro := []byte("PDF finto ma sempre lo stesso, byte per byte")
	archivio, sha := zipCon(t, map[string][]byte{
		"DISEGNI/6674611A.pdf":      dentro,
		"ALLEGATI/6674611A_bis.pdf": dentro,
	})

	// il banco ha due download in coda (disegno.pdf e archivio.zip): serve quello dello zip
	var tZip jobs.Tentativo
	for i := 0; i < 2; i++ {
		j, tt := b.claimJob("outlook@PC-A")
		if allegatoDi(j.Payload) == zipAllegato.AllegatoID {
			tZip = tt
		}
	}
	if tZip.JobID == 0 {
		t.Fatal("il download dello zip non e' stato preso in carico")
	}
	stato(t, b.put(tZip, zipAllegato.AllegatoID, bytes.NewReader(archivio), int64(len(archivio))), 204)
	stato(t, b.result(tZip, api.RisultatoStage{AllegatoID: zipAllegato.AllegatoID, Sha256: sha, Bytes: int64(len(archivio))}), 204)

	// Il result NON ha estratto niente: ha accodato. Dal blocco 4A scompattare un archivio non si fa
	// dentro la richiesta HTTP con cui il worker consegna il download, perche' il worker aspetta.
	if len(b.figliDi(zipAllegato.AllegatoID)) != 0 {
		t.Error("il result ha estratto l'archivio dentro la richiesta HTTP")
	}
	b.eseguiEstrazione(zipAllegato.AllegatoID)

	figli := b.figliDi(zipAllegato.AllegatoID)
	if len(figli) != 2 {
		t.Fatalf("voci dello zip registrate: %d, attese 2", len(figli))
	}
	if figli[0].percorso != figli[1].percorso {
		t.Errorf("due copie fisiche della stessa voce:\n  %s\n  %s", figli[0].percorso, figli[1].percorso)
	}
	// lo zip piu' la sua unica voce: due file, non tre
	if fs := b.contenuti(); len(fs) != 2 {
		t.Errorf("nello staging ci sono %d contenuti, attesi 2 (lo zip e la voce): %v", len(fs), fs)
	}
	// e le voci stanno fra i contenuti, non in una cartella del messaggio
	for _, f := range figli {
		if !strings.Contains(filepath.ToSlash(f.percorso), "/"+jobs.CartellaContenuti+"/") {
			t.Errorf("la voce %s non sta fra i contenuti: %s", f.nome, f.percorso)
		}
	}
	// la cartella temporanea dell'estrazione non deve sopravvivere
	if resti, _ := os.ReadDir(filepath.Join(b.staging, jobs.CartellaParti)); len(resti) > 0 {
		nomi := make([]string, 0, len(resti))
		for _, r := range resti {
			nomi = append(nomi, r.Name())
		}
		t.Errorf("dopo l'estrazione sono rimasti dei temporanei: %v", nomi)
	}
}

// eseguiEstrazione fa quello che fa l'esecutore interno del server: prende il job `estrai_archivio`
// che il result ha lasciato in coda e lo esegue. Il claim e' quello vero — `worker_tipo = 'server'` —
// cosi' il test si accorge anche se il job venisse accodato per un worker che non esiste.
func (b *banco) eseguiEstrazione(atteso uuid.UUID) {
	b.t.Helper()
	j, err := jobs.Claim(b.ctx, b.q, db.WorkerTipoServer, "server", jobs.Destinazione{}, 0)
	if err != nil || j == nil {
		b.t.Fatalf("nessun job in coda per l'esecutore interno: job=%v err=%v", j, err)
	}
	if j.Tipo != db.TipoJobEstraiArchivio {
		b.t.Fatalf("il job in coda e' %s, atteso %s", j.Tipo, db.TipoJobEstraiArchivio)
	}
	var p api.PayloadEstraiArchivio
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		b.t.Fatal(err)
	}
	if p.AllegatoID != atteso {
		b.t.Fatalf("il job estrae l'allegato %s invece di %s", p.AllegatoID, atteso)
	}
	if _, err := b.s.EstraiArchivio(b.ctx, p.AllegatoID, j.LeaseToken.UUID); err != nil {
		b.t.Fatalf("estrazione: %v", err)
	}
}

// zipCon fabbrica uno zip in memoria e ne restituisce i byte con il proprio sha256.
func zipCon(t *testing.T, voci map[string][]byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	// l'ordine con cui si scorre una map e' casuale: qui non conta quale voce venga prima, conta
	// che ci siano tutte
	for nome, contenuto := range voci {
		f, err := w.Create(nome)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(contenuto); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	somma := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(somma[:])
}
