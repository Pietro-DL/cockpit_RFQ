//go:build integrazione

// L4 — B8.5 dal risultato del worker: la struttura di uno STEP diventa proposte in ogni RFQ che ha quel
// file, ciascuna con le regole del suo cliente, e la BOM di nessuna cambia. Piu' il giro intero con il
// worker vero, dentro una RFQ.

package workerapi

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

const regoleFamiglia777 = `{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})(?:_(?P<rev>[A-Z]))?", "descrizione": "disegni 777",
	"rev_nel_codice": true, "esempio": "77722757_B"}]}`

func TestIFattiDiUnoStepDiventanoProposteInOgniRfqCheLoHa(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}
	riga := func(sql string, arg ...any) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := pool.QueryRow(ctx, sql, arg...).Scan(&id); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return id
	}
	sha := "e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5"
	utente := riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('B5', 'Prova', 'Tecnico') RETURNING utente_id`)
	// due RFQ di due clienti: uno con la famiglia 777, uno senza regole
	var rfq, allegati []uuid.UUID
	for i, regole := range []string{regoleFamiglia777, `{}`} {
		cliente := riga(`INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ($1, $1, $2) RETURNING cliente_id`,
			[]string{"CONFAMIGLIA", "SENZAREGOLE"}[i], regole)
		th := riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente)
		conv := riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
			[]string{"C-B5-1", "C-B5-2"}[i])
		msg := riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
			VALUES ('outlook', $1, $2, 'entrata', now(), $3) RETURNING messaggio_id`, []string{"<b5-1@x>", "<b5-2@x>"}[i], conv, th)
		al := riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, ricevuto_il)
			VALUES ($1, 1, 'assieme.stp', 'stp', 'file', 'outlook', 100, $2, 'C:\staging\b5.stp', now()) RETURNING allegato_id`, msg, sha)
		rfq, allegati = append(rfq, th), append(allegati, al)
	}
	// una proposta del documento gia' confermata nella seconda RFQ: la struttura arriva lo stesso
	if _, err := pool.Exec(ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte, stato, deciso_da)
		VALUES ($1, $2, 'cad_3d', 90, 'estensione', 'confermata', $3)`, allegati[1], rfq[1], utente); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte)
		VALUES ($1, $2, 'altro', 20, 'estensione')`, allegati[0], rfq[0]); err != nil {
		t.Fatal(err)
	}

	struttura := `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [],
		"nodi": [{"chiave": "#1", "id_grezzo": "", "nome_grezzo": "77722757_B", "evidenza": {}},
		         {"chiave": "#2", "id_grezzo": "", "nome_grezzo": "77720517_C", "evidenza": {}}],
		"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}],
		"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
		"occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{AllegatoID: allegati[0], Sha256: sha, NomeFile: "assieme.stp",
		VersioneAnalizzatore: 3, HashConfigurazione: an.Hash()})
	var jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
		VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	// il codice che il worker ha letto dal nome non e' di famiglia: D16 lo corregge con la radice
	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: allegati[0], TipoProposto: "cad_3d", Codice: "ASS-7788", Confidenza: 60,
		Fonte: "step", Dettagli: json.RawMessage(struttura), VersioneAnalizzatore: 3, HashConfigurazione: an.Hash()})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.applicaRisultato(ctx, db.New(tx), &j, dati, nil); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("il risultato non si applica: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	lettura := func(th uuid.UUID) string {
		var s string
		if err := pool.QueryRow(ctx, `SELECT string_agg(chiave || '=' || coalesce(codice, '-') || '/' || coalesce(rev, '-') || '/' || coalesce(origine_codice::text, '-'), ' ' ORDER BY chiave)
			FROM componente_proposta WHERE thread_id = $1 AND stato = 'aperta'`, th).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got, want := lettura(rfq[0]), "#1=77722757/B/famiglia #2=77720517/C/famiglia"; got != want {
		t.Errorf("RFQ con la famiglia: %q, attesa %q", got, want)
	}
	if got := lettura(rfq[1]); got == "" || got == lettura(rfq[0]) {
		t.Errorf("RFQ senza regole: %q — le proposte devono esserci, lette con le regole di QUEL cliente", got)
	}
	if n := testutil.Conta(t, pool, "relazione_proposta WHERE stato = 'aperta'"); n != 2 {
		t.Errorf("relazioni proposte aperte: %d, attese 2 (una per RFQ)", n)
	}
	if n := testutil.Conta(t, pool, "componente") + testutil.Conta(t, pool, "componente_relazione"); n != 0 {
		t.Errorf("il worker ha modificato la BOM: %d righe", n)
	}
	// D16 nella prima RFQ: la radice di famiglia corregge la proposta del documento
	var cod, fonte string
	if err := pool.QueryRow(ctx, `SELECT coalesce(codice, ''), fonte::text FROM documento_proposta WHERE allegato_id = $1`, allegati[0]).Scan(&cod, &fonte); err != nil {
		t.Fatal(err)
	}
	if cod != "77722757" || fonte != "regola_cliente" {
		t.Errorf("proposta del documento: %s da %s, attesa 77722757 da regola_cliente", cod, fonte)
	}
}

// Il giro intero con il worker vero dentro una RFQ: il file scende, il worker legge la struttura v3, il
// server la conserva e ne fa proposte; la BOM resta vuota.
func TestE2EIlWorkerVeroFaNascereLeProposteNellaRfq(t *testing.T) {
	if os.Getenv("COCKPIT_TEST_SENZA_PYTHON") != "" {
		t.Skip("COCKPIT_TEST_SENZA_PYTHON: il worker vero non viene avviato, prova non verificata")
	}
	b := preparaBancoAnalisiCon(t, "77722757.step", []byte(stepDuePadri))
	var cliente, thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('E2E85', 'E2E 85') RETURNING cliente_id`).Scan(&cliente); err != nil {
		t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, b.allegato.MessaggioID, thread); err != nil {
		t.Fatal(err)
	}
	uscita, _, _ := eseguiWorkerAnalisiVero(t, b)
	j, err := b.q.GetJob(b.ctx, b.analisi.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Stato != db.StatoJobFatto {
		t.Fatalf("job di analisi in stato %s (errore %q).\nOutput del worker:\n%s", j.Stato, j.Errore.String, uscita)
	}
	var nodi, relazioni, bom int
	if err := b.pool.QueryRow(b.ctx, `SELECT (SELECT count(*) FROM componente_proposta WHERE thread_id = $1),
		(SELECT count(*) FROM relazione_proposta WHERE thread_id = $1),
		(SELECT count(*) FROM componente) + (SELECT count(*) FROM componente_relazione)`, thread).Scan(&nodi, &relazioni, &bom); err != nil {
		t.Fatal(err)
	}
	if nodi != 3 || relazioni != 2 {
		t.Errorf("proposte nella RFQ: %d nodi e %d relazioni, attesi 3 e 2 (il sottoassieme con due padri)", nodi, relazioni)
	}
	if bom != 0 {
		t.Errorf("la BOM ha %d righe: il worker non la modifica mai", bom)
	}
}
