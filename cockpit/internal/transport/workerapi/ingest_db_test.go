//go:build integrazione

// L4 — revisione del 25/09: POST /api/v1/ingest/messaggi scrive solo dove il lease E la credenziale
// lo permettono.
//
// Prima bastava un lease valido di qualunque job, e una casella_id qualunque nel corpo: un worker
// scriveva messaggi, presenze e cursori nella casella di un collega, o nella propria con il lease di un
// download. Qui si prova il gestore vero via HTTP, con le credenziali seminate come in produzione.
package workerapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// bancoIngest è il banco del claim (Francesco autorizzato su Francesco e Commerciale, Luigi solo su
// Luigi) con il servizio di ingest montato.
func preparaBancoIngest(t *testing.T) *bancoClaim {
	t.Helper()
	b := preparaBancoClaim(t)
	b.s.Ingest = &ingest.Servizio{Pool: b.pool, Log: testutil.LogSilenzioso()}
	return b
}

// leaseDi mette in corso un job per quel worker, come dopo un claim, e restituisce il tentativo.
func (b *bancoClaim) leaseDi(workerID string, tipo db.TipoJob, casella uuid.UUID) coda.Tentativo {
	b.t.Helper()
	cid := casella
	payload, _ := json.Marshal(worker.PayloadSyncOutlook{CasellaID: &cid, Modo: worker.ModoAggiornamento})
	var id int64
	var token uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `
		INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, casella_id, stato, worker_id,
		                 lease_token, avviato_il, lease_fino_a)
		VALUES ($1, $2, $3, 300, 1800, $4, 'in_corso', $5, gen_random_uuid(), now(), now() + interval '5 minutes')
		RETURNING job_id, lease_token`, string(tipo), string(coda.WorkerPer(tipo)), payload, casella, workerID).Scan(&id, &token); err != nil {
		b.t.Fatal(err)
	}
	return coda.Tentativo{JobID: id, LeaseToken: token, WorkerID: workerID}
}

// lottoDiProva è un lotto di un messaggio, con il cursore: tutto ciò che un lotto può scrivere.
func lottoDiProva(t coda.Tentativo, casella *uuid.UUID) worker.IngestRichiesta {
	quando := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	return worker.IngestRichiesta{
		CasellaID: casella, JobID: t.JobID, LeaseToken: t.LeaseToken.String(), WorkerID: t.WorkerID,
		Messaggi: []worker.MessaggioIn{{
			MessageID: "<ingest-http@acme.example>", EntryID: "ENTRY-INGEST-HTTP", StoreID: "S", ConversationID: "CONV-INGEST-HTTP",
			Cartella: "Inbox", Direzione: "entrata", DataEvento: quando, RicevutoIl: &quando,
			MittenteIndirizzo: "mario.rossi@acme.example", Oggetto: "RFQ PZ-001", CorpoTesto: "Richiesta d'offerta.",
			Riferimenti: []string{}, Categorie: []string{},
		}},
		Cursore: &worker.CursoreLotto{Cartella: "Inbox", UltimoReceived: quando},
	}
}

func (b *bancoClaim) postIngest(token string, req worker.IngestRichiesta) (int, string) {
	b.t.Helper()
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/ingest/messaggi", bytes.NewReader(mustJSON(req)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Cockpit-Token", token)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	var corpo bytes.Buffer
	_, _ = corpo.ReadFrom(resp.Body)
	return resp.StatusCode, corpo.String()
}

// nienteDelLotto: un lotto rifiutato lascia il database com'era. Il banco ha già un suo messaggio
// (quello del download), quindi si contano le righe di QUESTO lotto.
func (b *bancoClaim) nienteDelLotto(perche string) {
	b.t.Helper()
	for cosa, sql := range map[string]string{
		"messaggi": `SELECT count(*) FROM messaggio WHERE chiave_esterna = '<ingest-http@acme.example>'`,
		"presenze": `SELECT count(*) FROM messaggio_casella WHERE entry_id = 'ENTRY-INGEST-HTTP'`,
		"scarti":   `SELECT count(*) FROM ingest_scarto`,
		"cursori":  `SELECT count(*) FROM sync_cursore`,
	} {
		if n := conta(b.t, b.banco, sql); n != 0 {
			b.t.Errorf("%s: %d %s — il lotto rifiutato ha scritto", perche, n, cosa)
		}
	}
}

func TestIngestRifiutaUnaCasellaCheLaCredenzialeNonAutorizza(t *testing.T) {
	b := preparaBancoIngest(t)
	// il worker di Francesco ha un lease valido di un sync di Commerciale, e consegna un lotto per Luigi
	tent := b.leaseDi("outlook@PC-FRANCESCO", db.TipoJobSyncOutlook, b.commerciale)
	stato, corpo := b.postIngest(tokenDi("outlook@PC-FRANCESCO"), lottoDiProva(tent, &b.luigi))
	if stato != http.StatusForbidden || !strings.Contains(corpo, "non è autorizzata") {
		t.Fatalf("lotto per la casella di un altro: %d %s, atteso 403", stato, corpo)
	}
	b.nienteDelLotto("casella non autorizzata")
	// il job non ha niente di sbagliato: resta in corso, non va in errore
	var st string
	if err := b.pool.QueryRow(b.ctx, `SELECT stato::text FROM job WHERE job_id = $1`, tent.JobID).Scan(&st); err != nil || st != "in_corso" {
		t.Errorf("stato del job dopo il rifiuto: %s %v", st, err)
	}

	// una credenziale con l'elenco VUOTO non autorizza nessuna casella, come per il claim
	vuota := b.leaseDi("analisi@PC-FRANCESCO", db.TipoJobSyncOutlook, b.commerciale)
	if stato, corpo := b.postIngest(tokenDi("analisi@PC-FRANCESCO"), lottoDiProva(vuota, &b.commerciale)); stato != http.StatusForbidden {
		t.Fatalf("credenziale senza caselle: %d %s, atteso 403", stato, corpo)
	}
	b.nienteDelLotto("credenziale senza caselle")
}

func TestIngestRifiutaUnLeaseCheNonEDiQuelLotto(t *testing.T) {
	b := preparaBancoIngest(t)
	casi := []struct {
		nome    string
		tent    coda.Tentativo
		casella uuid.UUID
	}{
		// la credenziale autorizza Francesco, ma il job è il sync di Commerciale
		{"sync di un'altra casella", b.leaseDi("outlook@PC-FRANCESCO", db.TipoJobSyncOutlook, b.commerciale), b.francesco},
		// il lease di un download, sulla casella giusta e autorizzata
		{"lease di un download", b.leaseDi("outlook@PC-FRANCESCO", db.TipoJobStageAllegato, b.commerciale), b.commerciale},
	}
	for _, c := range casi {
		casella := c.casella
		stato, corpo := b.postIngest(tokenDi("outlook@PC-FRANCESCO"), lottoDiProva(c.tent, &casella))
		if stato != http.StatusForbidden || !strings.Contains(corpo, "non autorizza questo lotto") {
			t.Fatalf("%s: %d %s, atteso 403", c.nome, stato, corpo)
		}
		b.nienteDelLotto(c.nome)
	}
}

// Un lotto che non dichiara la casella prende quella del suo job, non [outlook].casella_default: il
// banco ha Francesco come casella predefinita e il job è di Commerciale. È anche il controllo che le
// prove sopra rifiutano per il motivo giusto: lo stesso lotto, con un lease e una casella buoni, entra.
func TestUnLottoSenzaCasellaPrendeQuellaDelSuoJob(t *testing.T) {
	b := preparaBancoIngest(t)
	b.s.CasellaDefault = "francesco@azienda.example"
	tent := b.leaseDi("outlook@PC-FRANCESCO", db.TipoJobSyncOutlook, b.commerciale)
	stato, corpo := b.postIngest(tokenDi("outlook@PC-FRANCESCO"), lottoDiProva(tent, nil))
	if stato != http.StatusOK {
		t.Fatalf("lotto senza casella_id: %d %s", stato, corpo)
	}
	var casella uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT casella_id FROM messaggio_casella WHERE entry_id = 'ENTRY-INGEST-HTTP'`).Scan(&casella); err != nil {
		t.Fatal(err)
	}
	if casella != b.commerciale {
		t.Errorf("la presenza è finita nella casella %s, attesa quella del job (Commerciale %s): il ripiego sulla casella predefinita vale solo per un job che non ne nomina una", casella, b.commerciale)
	}
	if n := testutil.Conta(t, b.pool, "sync_cursore"); n != 1 {
		t.Errorf("cursori = %d, atteso 1", n)
	}
}

// Il risultato di un'analisi riguarda l'allegato del JOB. Un risultato che dichiara un altro allegato
// non si applica — i fatti finirebbero sotto lo sha256 di un altro file, e la proposta di un altro
// messaggio verrebbe riscritta —: 422 e job fallito in modo definitivo, come ogni risultato sbagliato.
func TestIlRisultatoDiUnAnalisiPerUnAltroAllegatoNonSiApplica(t *testing.T) {
	b := preparaBancoAnalisi(t, "1234567A_4.dxf")
	tent := b.claimAnalisi("analisi@PC-A")
	altro, _ := b.messaggioConAllegato("PZ-001.pdf", "pdf")
	if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET sha256 = repeat('d', 64) WHERE allegato_id = $1`, altro.AllegatoID); err != nil {
		t.Fatal(err)
	}
	var propostePrima int
	_ = b.pool.QueryRow(b.ctx, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1`, altro.AllegatoID).Scan(&propostePrima)

	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: altro.AllegatoID, TipoProposto: "disegno_2d", Codice: "PZ-001",
		Confidenza: 90, Fonte: "cartiglio", VersioneAnalizzatore: b.s.Analizzatore.Versione, HashConfigurazione: b.s.Analizzatore.Hash()})
	corpo, _ := json.Marshal(worker.RisultatoRichiesta{Esito: "ok", Dati: dati, WorkerID: tent.WorkerID, LeaseToken: tent.LeaseToken.String()})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/jobs/%d/result", b.srv.URL, tent.JobID), bytes.NewReader(corpo))
	req.Header.Set("X-Cockpit-Token", tokenDi(tent.WorkerID))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	testo := stato(t, resp, http.StatusUnprocessableEntity)
	if !strings.Contains(testo, altro.AllegatoID.String()) {
		t.Errorf("il motivo non nomina l'allegato dichiarato: %s", testo)
	}
	j, err := b.q.GetJob(b.ctx, tent.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Stato != db.StatoJobFallito {
		t.Errorf("job in stato %s, atteso fallito (definitivo: ritentarlo darebbe lo stesso esito)", j.Stato)
	}
	if n := testutil.Conta(t, b.pool, "analisi_fatti"); n != 0 {
		t.Errorf("%d righe in analisi_fatti: i fatti di un file sono finiti sotto un altro", n)
	}
	var proposteDopo int
	_ = b.pool.QueryRow(b.ctx, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND codice = 'PZ-001'`, altro.AllegatoID).Scan(&proposteDopo)
	if proposteDopo != 0 {
		t.Errorf("la proposta dell'altro allegato è stata riscritta dal risultato (prima %d proposte)", propostePrima)
	}
}

// Un errore del database nel leggere il job è un guasto (500, si ripete), non un job che non esiste
// (404, il worker smette e il lavoro è perso). L'errore qui è vero: la colonna che GetJob legge non c'è.
func TestIlResultDistingueUnJobInesistenteDaUnGuasto(t *testing.T) {
	b := preparaBanco(t, 0)
	tent := b.claim("outlook@PC-A")
	invia := func(jobID int64) *http.Response {
		corpo, _ := json.Marshal(worker.RisultatoRichiesta{Esito: "errore", Errore: "prova", WorkerID: tent.WorkerID, LeaseToken: tent.LeaseToken.String()})
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/jobs/%d/result", b.srv.URL, jobID), bytes.NewReader(corpo))
		req.Header.Set("X-Cockpit-Token", tokenDi(tent.WorkerID))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	stato(t, invia(tent.JobID+1000), http.StatusNotFound)

	if _, err := b.pool.Exec(b.ctx, `ALTER TABLE job RENAME COLUMN payload TO payload_nascosto`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// lo schema resta quello delle migrazioni per chi viene dopo, anche se questo test cade
		_, _ = b.pool.Exec(b.ctx, `ALTER TABLE job RENAME COLUMN payload_nascosto TO payload`)
	})
	stato(t, invia(tent.JobID), http.StatusInternalServerError)
}

// Il contenuto si serve solo da dentro lo staging del server. `path_staging` è un dato del database:
// un valore che punta fuori — un file qualunque del server — non si apre, qualunque cosa ci sia.
func TestIlContenutoFuoriDalloStagingNonSiServe(t *testing.T) {
	b := preparaBancoAnalisi(t, "1234567A_4.dxf")
	tent := b.claimAnalisi("analisi@PC-A")
	fuori := filepath.Join(t.TempDir(), "segreto.dxf")
	if err := os.WriteFile(fuori, b.contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET path_staging = $2 WHERE allegato_id = $1`, b.allegato.AllegatoID, fuori); err != nil {
		t.Fatal(err)
	}
	resp := b.get(tent, b.allegato.AllegatoID, tokenDi(tent.WorkerID))
	testo := stato(t, resp, http.StatusGone)
	if strings.Contains(testo, string(b.contenuto[:64])) || !strings.Contains(testo, "Riscarica") {
		t.Errorf("risposta per un percorso fuori dallo staging: %.200s", testo)
	}
	// il controllo: lo stesso file dentro lo staging si serve (preparaBancoAnalisi lo mette lì)
	if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET path_staging = $2 WHERE allegato_id = $1`, b.allegato.AllegatoID, b.allegato.PathStaging.String); err != nil {
		t.Fatal(err)
	}
	stato(t, b.get(tent, b.allegato.AllegatoID, tokenDi(tent.WorkerID)), http.StatusOK)
}
