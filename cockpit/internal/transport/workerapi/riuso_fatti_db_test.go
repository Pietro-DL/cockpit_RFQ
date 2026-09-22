//go:build integrazione

// L4 — B8.0: il secondo allegato con lo stesso contenuto riceve la lettura dell'analisi.
//
// `AccodaAnalisi` restituisce (nil, nil) quando i fatti per (contenuto, versione, configurazione) ci
// sono già, e il suo commento diceva «il chiamante li riuserà»: i due chiamanti scartavano il valore.
// Il risultato era che in una RFQ dove lo stesso disegno arriva in due mail il secondo file restava
// con la proposta dal NOME — senza cartiglio, senza codice, senza revisione — e nessuno poteva dire
// perché, visto che il primo li aveva.
package workerapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

const shaRiuso = "bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff001122334455"

// allegatoSceso scrive una mail con un allegato già in staging: è lo stato in cui `dopoStaging` lo trova.
func allegatoSceso(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffisso, nomeFile string) uuid.UUID {
	t.Helper()
	var convID, msgID, allegatoID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', $1, now()) RETURNING conversazione_id`, "CONV-RIUSO-"+suffisso).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ riuso') RETURNING messaggio_id`,
		"<riuso-"+suffisso+"@acme.example>", convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura,
		origine, bytes, sha256, path_staging, ricevuto_il, stato)
		VALUES ($1, 1, $2, 'pdf', 'file', 'outlook', 1000, $3, $4, now(), 'in_staging')
		RETURNING allegato_id`, msgID, nomeFile, shaRiuso,
		`C:\staging\_contenuti\bb\`+shaRiuso+`.pdf`).Scan(&allegatoID); err != nil {
		t.Fatal(err)
	}
	return allegatoID
}

func TestIlSecondoAllegatoConLoStessoContenutoRiceveLaProposta(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 1, Parametri: map[string]any{"termini": []any{"scala"}}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}

	// due mail diverse, lo stesso disegno allegato a tutte e due
	primo := allegatoSceso(t, ctx, pool, "a", "6674611A_4.pdf")
	if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
		AllegatoID: primo, TipoProposto: db.TipoDocumentoAltro, Confidenza: 20,
		Fonte: db.FontePropostaEstensione, Dettagli: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	// l'analisi del PRIMO allegato: i fatti restano per (contenuto, versione, configurazione)
	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{
		AllegatoID: primo, Bytes: 1000, Sha256: shaRiuso, NomeFile: "6674611A_4.pdf",
		VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})
	var jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato, chiave_idempotenza)
		VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'fatto', $2) RETURNING job_id`,
		payload, "analizza:"+shaRiuso+":1:"+an.Hash()).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	dati, _ := json.Marshal(worker.RisultatoAnalisi{
		AllegatoID: primo, TipoProposto: "disegno_2d", Codice: "6674611A", Rev: "4",
		Confidenza: 92, Fonte: "cartiglio", Dettagli: json.RawMessage(`{"cartiglio":true,"termini":["scala"]}`),
		VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})
	if err := s.applicaRisultato(ctx, q, &j, dati, nil); err != nil {
		t.Fatalf("applica risultato della prima analisi: %v", err)
	}

	// il SECONDO allegato scende adesso: stesso contenuto, altra mail, nessuna analisi da accodare
	secondo := allegatoSceso(t, ctx, pool, "b", "6674611A_4.pdf")
	if err := s.dopoStaging(ctx, q, worker.RisultatoStage{
		AllegatoID: secondo, Sha256: shaRiuso, Bytes: 1000,
	}); err != nil {
		t.Fatalf("dopo lo staging del secondo allegato: %v", err)
	}

	// nessun secondo job: la chiave è per contenuto, e quel contenuto è già stato letto
	if n := testutil.Conta(t, pool, "job"); n != 1 {
		t.Errorf("job in coda = %d, atteso 1: il contenuto era già stato analizzato", n)
	}

	var (
		tipo, fonte, codice, rev, stato string
		conf                            int
		dettagli                        []byte
	)
	if err := pool.QueryRow(ctx, `SELECT p.tipo_proposto, p.fonte, coalesce(p.codice,''), coalesce(p.rev,''),
		p.confidenza, p.dettagli, a.stato
		FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
		WHERE p.allegato_id = $1`, secondo).Scan(&tipo, &fonte, &codice, &rev, &conf, &dettagli, &stato); err != nil {
		t.Fatalf("proposta del secondo allegato: %v", err)
	}
	if tipo != "disegno_2d" || codice != "6674611A" || rev != "4" || conf != 92 || fonte != "cartiglio" {
		t.Errorf("il secondo allegato non ha ricevuto la lettura dell'analisi: tipo=%s codice=%q rev=%q conf=%d fonte=%s",
			tipo, codice, rev, conf, fonte)
	}
	if stato != "analizzato" {
		t.Errorf("stato dell'allegato = %s, atteso analizzato", stato)
	}
	// i dettagli sono quelli del worker; l'esito sta nei FATTI, non nella proposta
	if !strings.Contains(string(dettagli), "cartiglio") {
		t.Errorf("dettagli senza i fatti dell'analisi: %s", dettagli)
	}
	if strings.Contains(string(dettagli), "esito") {
		t.Errorf("l'esito dei fatti è finito nei dettagli della proposta: %s", dettagli)
	}
}

// Fatti scritti prima che l'esito esistesse: non si indovina un tipo proposto. La proposta resta
// quella dal nome finché una rianalisi non riscrive i fatti con l'esito dentro.
func TestIFattiSenzaEsitoNonRiscrivonoLaProposta(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 1, Parametri: map[string]any{"termini": []any{"scala"}}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}

	allegatoID := allegatoSceso(t, ctx, pool, "v1", "listino.pdf")
	// fatti in forma v1: solo i dettagli del worker, nessun `esito`
	if _, err := q.UpsertAnalisiFatti(ctx, db.UpsertAnalisiFattiParams{
		Sha256: shaRiuso, VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
		Fatti: json.RawMessage(`{"termini":["scala"]}`),
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.dopoStaging(ctx, q, worker.RisultatoStage{AllegatoID: allegatoID, Sha256: shaRiuso, Bytes: 1000}); err != nil {
		t.Fatalf("dopo lo staging: %v", err)
	}
	var tipo, stato string
	if err := pool.QueryRow(ctx, `SELECT p.tipo_proposto, a.stato FROM documento_proposta p
		JOIN allegato a ON a.allegato_id = p.allegato_id WHERE p.allegato_id = $1`, allegatoID).Scan(&tipo, &stato); err != nil {
		t.Fatal(err)
	}
	if tipo == "disegno_2d" {
		t.Error("i fatti senza esito hanno riscritto la proposta: il tipo proposto è stato inventato")
	}
	if stato == "analizzato" {
		t.Errorf("l'allegato è stato dichiarato analizzato senza un esito: stato=%s", stato)
	}
}
