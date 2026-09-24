//go:build integrazione

// L4 — A4.4 (B8.A4a): il worker non modifica mai la BOM. I fatti di uno STEP si conservano e
// aggiornano la proposta del file; componenti e relazioni della working restano quelli, anche quando
// la BOM e' congelata (le proposte sono interpretazione, e il trigger della working non c'entra).
//
// Prova 8 dell'addendum: TestUnNuovoStepNonModificaLaBom.

package workerapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func TestUnNuovoStepNonModificaLaBom(t *testing.T) {
	for _, congelata := range []bool{false, true} {
		nome := "BOM working"
		if congelata {
			nome = "BOM congelata"
		}
		t.Run(nome, func(t *testing.T) {
			pool := testutil.Pool(t)
			testutil.SchemaPulito(t, pool)
			ctx := context.Background()
			q := db.New(pool)
			an := coda.Analizzatore{Versione: 2, Parametri: map[string]any{}}
			s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}
			esegui := func(sql string, arg ...any) {
				t.Helper()
				if _, err := pool.Exec(ctx, sql, arg...); err != nil {
					t.Fatalf("%v\n%s", err, sql)
				}
			}
			var utente, cliente, thread, conv, msg, allegato, p1, f1 uuid.UUID
			riga := func(dst *uuid.UUID, sql string, arg ...any) {
				t.Helper()
				if err := pool.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
					t.Fatalf("%v\n%s", err, sql)
				}
			}
			riga(&utente, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('A8', 'Prova', 'Tecnico') RETURNING utente_id`)
			riga(&cliente, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme') RETURNING cliente_id`)
			riga(&thread, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente)
			riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-A8', now()) RETURNING conversazione_id`)
			riga(&msg, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
				VALUES ('outlook', '<a8@acme>', $1, 'entrata', now(), $2) RETURNING messaggio_id`, conv, thread)
			riga(&allegato, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, ricevuto_il)
				VALUES ($1, 1, '52922757.step', 'step', 'file', 'outlook', 100, repeat('e', 64), 'C:\staging\a8.step', now()) RETURNING allegato_id`, msg)
			esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'altro', 20, 'estensione')`, allegato, thread)
			riga(&p1, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, '52922757', 'finito', $2) RETURNING componente_id`, thread, utente)
			riga(&f1, `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ($1, 'VITE', $2) RETURNING componente_id`, thread, utente)
			esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 4, 'manuale', $4)`, thread, p1, f1, utente)
			if congelata {
				var fase, v1 uuid.UUID
				riga(&fase, `INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now()) RETURNING fase_log_id`, thread)
				riga(&v1, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
					VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prima', $3) RETURNING bom_versione_id`, thread, fase, utente)
				esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, v1, utente)
			}
			bom := func() string {
				var s string
				if err := pool.QueryRow(ctx, `SELECT concat_ws(' # ',
					(SELECT string_agg(to_jsonb(c)::text, '|' ORDER BY componente_id) FROM componente c),
					(SELECT string_agg(to_jsonb(r)::text, '|' ORDER BY padre_id, figlio_id) FROM componente_relazione r))`).Scan(&s); err != nil {
					t.Fatal(err)
				}
				return s
			}
			prima := bom()

			// il risultato di uno STEP con una struttura che la working non ha: un nodo nuovo, un arco
			// nuovo, una qta diversa, e l'arco della working che nel file non c'e'
			struttura := `{"struttura": {"versione": 2, "schema": "AP214", "radici": ["#12"],
				"nodi": [{"chiave": "#12", "nome_grezzo": "52922757"}, {"chiave": "#32", "nome_grezzo": "STAFFA NUOVA"}],
				"relazioni": [{"padre": "#12", "figlio": "#32", "qta": 2}], "avvisi": [], "limiti": {"troncato": false}}}`
			payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{AllegatoID: allegato, Bytes: 100, Sha256: "eeee",
				NomeFile: "52922757.step", VersioneAnalizzatore: 2, HashConfigurazione: an.Hash()})
			var jobID int64
			if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
				VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
				t.Fatal(err)
			}
			j, err := q.GetJob(ctx, jobID)
			if err != nil {
				t.Fatal(err)
			}
			dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: allegato, TipoProposto: "cad_3d", Codice: "52922757",
				Confidenza: 90, Fonte: "step", Dettagli: json.RawMessage(struttura), VersioneAnalizzatore: 2, HashConfigurazione: an.Hash()})
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.applicaRisultato(ctx, db.New(tx), &j, dati, nil); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatalf("il risultato dell'analisi non si applica: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if dopo := bom(); dopo != prima {
				t.Errorf("l'analisi ha cambiato la BOM:\nprima %s\ndopo  %s", prima, dopo)
			}
			if n := testutil.Conta(t, pool, "analisi_fatti"); n != 1 {
				t.Errorf("fatti conservati: %d", n)
			}
			var tipo string
			if err := pool.QueryRow(ctx, `SELECT tipo_proposto::text FROM documento_proposta WHERE allegato_id = $1`, allegato).Scan(&tipo); err != nil || tipo != "cad_3d" {
				t.Errorf("la proposta del file non e' stata aggiornata: %q %v", tipo, err)
			}
		})
	}
}
