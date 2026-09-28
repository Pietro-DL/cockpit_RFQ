//go:build integrazione

// L4 — Smistamento F5 (A5.4.10): a valle del worker la struttura di uno STEP diventa solo GUIDA dove il file non
// e' autorizzato, anche quando i codici dei nodi sono quelli dei componenti della RFQ. Prove 120 (Scarica e
// Riscarica: i fatti riusati dopo uno stage) e 121 (il fan-out in un'altra RFQ, anche con la BOM congelata;
// le RFQ chiuse o unite restano fuori).

package workerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// strutturaGuida: la radice 77722757 e il figlio 77720517 ×2, i codici dei componenti delle RFQ di prova.
const strutturaGuida = `{"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [],
	"nodi": [{"chiave": "#1", "id_grezzo": "77722757", "nome_grezzo": "77722757", "evidenza": {}},
	         {"chiave": "#2", "id_grezzo": "77720517", "nome_grezzo": "77720517", "evidenza": {}}],
	"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}],
	"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	"occorrenze_su_se_stesse": 0, "testi_troncati": 0}}`

// bancoGuida: RFQ di prova con il prodotto 77722757, l'assieme 77720517 e l'arco fra loro ×2 (quello che lo
// STEP dice: prima di F5 la lettura ne faceva due duplicati agganciati e un arco duplicato).
type bancoGuida struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	utente uuid.UUID
	n      int
}

func (b *bancoGuida) riga(sql string, arg ...any) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&id); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	return id
}

func (b *bancoGuida) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (b *bancoGuida) valore(sql string, arg ...any) string {
	b.t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&s); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	return s
}

// rfq e' una RFQ con i due componenti e l'arco; senza componenti se vuota.
func (b *bancoGuida) rfq(conComponenti bool) uuid.UUID {
	b.t.Helper()
	b.n++
	cliente := b.riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1, $1) RETURNING cliente_id`, fmt.Sprintf("ACME%d", b.n))
	th := b.riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente)
	if conComponenti {
		p := b.riga(`INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, '77722757', 'finito', $2) RETURNING componente_id`, th, b.utente)
		a := b.riga(`INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, '77720517', 'sottoassieme', $2) RETURNING componente_id`, th, b.utente)
		b.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 2, 'manuale', $4)`, th, p, a, b.utente)
	}
	return th
}

// allegato e' uno STEP con il contenuto sha in un messaggio del cliente, agganciato alla RFQ (uuid.Nil = a
// nessuna), gia' in staging.
func (b *bancoGuida) allegato(thread uuid.UUID, sha string) uuid.UUID {
	b.t.Helper()
	b.n++
	conv := b.riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("C-F5-%d", b.n))
	msg := b.riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3) RETURNING messaggio_id`, fmt.Sprintf("<f5-%d@acme.example>", b.n), conv,
		uuid.NullUUID{UUID: thread, Valid: thread != uuid.Nil})
	return b.riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, ricevuto_il, stato)
		VALUES ($1, 1, 'assieme.stp', 'stp', 'file', 'outlook', 100, $2, 'C:\staging\f5.stp', now(), 'in_staging') RETURNING allegato_id`, msg, sha)
}

// righe: "chiave:stato:componente" dei nodi e "padre>figlio:stato" degli archi della RFQ; "" se non ce ne sono.
func (b *bancoGuida) righe(thread uuid.UUID) string {
	return b.valore(`SELECT concat_ws(' | ',
		(SELECT string_agg(chiave || ':' || stato || ':' || coalesce(componente_id::text, '-'), ' ' ORDER BY chiave) FROM componente_proposta WHERE thread_id = $1),
		(SELECT string_agg(padre_chiave || '>' || figlio_chiave || ':' || stato, ' ' ORDER BY padre_chiave) FROM relazione_proposta WHERE thread_id = $1))`, thread)
}

// Prova 121 (critica L5, A5.4.10): un'analisi fatta per una RFQ scrive nelle altre che hanno lo stesso
// contenuto solo righe di GUIDA: aperte, senza componente, anche dove i codici dei nodi sono quelli dei
// componenti (prima diventavano duplicati agganciati, e l'arco un duplicato), e anche con la BOM congelata,
// dove il riesame (contato sull'autorita') non si accende. Le RFQ chiuse e quelle unite a un'altra non
// ricevono niente.
func TestUnAnalisiNonScriveProposteUtilizzabiliInUnAltraRfq(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	b := &bancoGuida{t: t, ctx: context.Background(), pool: pool}
	b.utente = b.riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('F5', 'Prova', 'Tecnico') RETURNING utente_id`)
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}
	sha := "f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5f5"

	origine, altra, congelata, chiusa, unita := b.rfq(false), b.rfq(true), b.rfq(true), b.rfq(true), b.rfq(true)
	b.esegui(`UPDATE thread_offerta SET stato = 'CHIUSA' WHERE thread_id = $1`, chiusa)
	b.esegui(`UPDATE thread_offerta SET unito_in = $2 WHERE thread_id = $1`, unita, altra)
	fase := b.riga(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now() - interval '1 hour') RETURNING fase_log_id`, congelata)
	v1 := b.riga(`INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prima', $3) RETURNING bom_versione_id`, congelata, fase, b.utente)
	b.esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() - interval '1 minute' WHERE bom_versione_id = $1`, v1, b.utente)
	primo := b.allegato(origine, sha)
	for _, th := range []uuid.UUID{altra, congelata, chiusa, unita} {
		b.allegato(th, sha)
	}

	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{AllegatoID: primo, Sha256: sha, NomeFile: "assieme.stp",
		VersioneAnalizzatore: 3, HashConfigurazione: an.Hash()})
	var jobID int64
	if err := pool.QueryRow(b.ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
		VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := q.GetJob(b.ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: primo, TipoProposto: "cad_3d", Confidenza: 60, Fonte: "step",
		Dettagli: json.RawMessage(`{"struttura": ` + strutturaGuida + `}`), VersioneAnalizzatore: 3, HashConfigurazione: an.Hash()})
	tx, err := pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.applicaRisultato(b.ctx, db.New(tx), &j, dati, nil); err != nil {
		_ = tx.Rollback(b.ctx)
		t.Fatalf("il risultato non si applica: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}

	const guida = "#1:aperta:- #2:aperta:- | #1>#2:aperta"
	for nome, th := range map[string]uuid.UUID{"l'altra RFQ": altra, "la RFQ congelata": congelata, "la RFQ dell'analisi": origine} {
		if got := b.righe(th); got != guida {
			t.Errorf("%s: %q, attese le sole righe di guida %q", nome, got, guida)
		}
	}
	for nome, th := range map[string]uuid.UUID{"la RFQ chiusa": chiusa, "la RFQ unita": unita} {
		if got := b.righe(th); got != "" {
			t.Errorf("%s ha ricevuto proposte: %q", nome, got)
		}
	}
	if n := testutil.Conta(t, pool, "componente_proposta WHERE componente_id IS NOT NULL"); n != 0 {
		t.Errorf("%d nodi con un componente scritto dalla lettura", n)
	}
	// la BOM congelata: la vista conta le righe nuove, il riesame sull'autorita' no
	var vista, riesame int
	righe, err := q.ListThreadDaRiesaminare(b.ctx, congelata)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range righe {
		if r.TipoMotivo == "evidenze_nuove" {
			vista++
		}
	}
	filtrate, err := fascicolo.RiesameNellAutorita(b.ctx, q, congelata)
	if err != nil {
		t.Fatal(err)
	}
	riesame = len(filtrate)
	if vista != 1 || riesame != 0 {
		t.Errorf("riesame della RFQ congelata: vista %d (attesa 1: conta la guida), riesame %d (atteso 0)", vista, riesame)
	}
}

// Prova 120 (critica L4, E30): Scarica e Riscarica non scrivono struttura utilizzabile. Il file scende in una
// RFQ che ha gia' i componenti con i codici dei nodi; i fatti del suo contenuto ci sono gia' (un'altra copia li
// ha fatti fare), e dopo lo stage il server li applica (applicaFattiEsistenti, la stessa strada del «Rileggi»
// della preparazione per un file fermo o riusato): nascono solo righe di guida, nessun duplicato e nessun
// componente sulle righe.
func TestScaricaERiscaricaNonScrivonoStruttura(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	b := &bancoGuida{t: t, ctx: context.Background(), pool: pool}
	b.utente = b.riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('F5', 'Prova', 'Tecnico') RETURNING utente_id`)
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}
	sha := "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5"
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)`,
		sha, an.Hash(), `{"esito": {"tipo_proposto": "cad_3d", "confidenza": 60, "fonte": "step"}, "struttura": `+strutturaGuida+`}`)
	th := b.rfq(true)
	for i := 0; i < 2; i++ { // Scarica, poi Riscarica: la seconda copia dello stesso contenuto
		a := b.allegato(th, sha)
		if err := s.dopoStaging(b.ctx, q, worker.RisultatoStage{AllegatoID: a, Sha256: sha, Bytes: 100}); err != nil {
			t.Fatalf("dopo lo staging %d: %v", i, err)
		}
		if got := b.righe(th); got != "#1:aperta:- #2:aperta:- | #1>#2:aperta" {
			t.Errorf("dopo lo staging %d: %q, attese le sole righe di guida", i, got)
		}
	}
	if n := testutil.Conta(t, pool, "job"); n != 0 {
		t.Errorf("%d job: i fatti c'erano gia'", n)
	}
}
