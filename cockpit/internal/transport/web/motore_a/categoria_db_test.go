//go:build integrazione

// L4 — V1, ondata C: la conferma della categoria letta dall'ultima conferma valida (emendamento U2 §6.4.2; CAT-08 la
// parte del motore) sulla scena ACME di servizio_db_test.go: senza conferme il figlio sciolto chiede il 2D; con una
// conferma formato 3 del prodotto il cui registro delle decisioni ha conferma_categoria minuteria sul figlio (soggetto
// per chiave della bozza, risolta con la mappa), il calcolo di /dati non chiede più il 2D al figlio; con la stessa voce
// ma esito «rifiuto» il 2D resta. La sola classe della grammatica non esenta (nessuna voce = nessuna esenzione).
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx), come nella scena.
package motore_a

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/valutazione"
)

func TestL4LaConfermaDellaCategoria(t *testing.T) {
	for _, c := range []struct {
		nome   string
		esito  string
		chiede bool
	}{{"conferma", "conferma", false}, {"rifiuto", "rifiuto", true}} {
		t.Run(c.nome, func(t *testing.T) {
			p, s, r := preparaMA(t)
			ctx := context.Background()
			svc := Servizio{Regole: r, Pool: p}
			chiede2D := func(t *testing.T) bool {
				t.Helper()
				ris, err := svc.Calcola(ctx, s.thread)
				if err != nil {
					t.Fatal(err)
				}
				for _, pv := range ris.EsitoThread.ProdottiValutati {
					for _, v := range pv.Documenti.Voci {
						if v.ComponenteID == s.figlio && v.TipoDocumento == "disegno_2d" {
							return true
						}
					}
					for _, v := range pv.Documenti.NonBloccanti {
						if v.ComponenteID == s.figlio && v.TipoDocumento == "disegno_2d" {
							return true
						}
					}
				}
				return false
			}
			inTransazione(t, p,
				passoSQL{`UPDATE componente_proposta SET stato = 'confermata', componente_id = $2, deciso_da = $3, deciso_il = now()
					WHERE thread_id = $1 AND chiave = '#2'`, []any{s.thread, s.figlio, s.utente}},
				passoSQL{`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 2, 'step', $4)`,
					[]any{s.thread, s.prodotto, s.figlio, s.utente}})
			if !chiede2D(t) {
				t.Fatal("senza conferme il figlio sciolto non chiede il 2D: la prova non dice niente")
			}
			cliente := "cliente:" + s.cliente.String()
			decisioni, _ := json.Marshal([]any{map[string]any{"id": "cc-1", "tipo": "conferma_categoria", "soggetto": map[string]any{"chiave_bozza": "k-f"},
				"categoria": valutazione.CategoriaMinuteria, "esito": c.esito, "origine_categoria": "manuale", "ingresso": "editor",
				"motivo": "vite di serie (prova)", "autore": s.utente.String(), "quando": "2026-10-18T08:00:00Z"}})
			snap, _ := json.Marshal(map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
				"nodi": []any{
					map[string]any{"chiave_bozza": "k-p", "componente_id": s.prodotto.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
						"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "9120100", "modo": "registrato"}},
					map[string]any{"chiave_bozza": "k-f", "componente_id": s.figlio.String(), "op": "riusa", "tipo": "sciolto", "origine": "registrato",
						"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "9120101", "rev": "2", "modo": "registrato"}},
				},
				"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-f", "qta": 2, "origine": "conferma"}}, "decisioni": json.RawMessage(decisioni)})
			mappa, _ := json.Marshal(map[string]string{"k-p": s.prodotto.String(), "k-f": s.figlio.String()})
			b1 := uuid.New()
			inTransazione(t, p,
				passoSQL{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
					VALUES ($1, $2, $3, 3, 0, 0, repeat('1', 64), $4, $4)`, []any{b1, s.thread, s.prodotto, s.utente}},
				passoSQL{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
					chiave_salvataggio, impronta_richiesta) VALUES ($1, 1, $2::jsonb, repeat('5', 64), 0, 0, repeat('1', 64), $3, gen_random_uuid(), repeat('4', 64))`,
					[]any{b1, string(snap), s.utente}},
				passoSQL{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
					[]any{b1, s.utente}})
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var c1 uuid.UUID
			if err := tx.QueryRow(ctx, `INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot,
				delta, mappa, decisioni, criterio_verifica, chiave_idempotenza, confermato_da, importazioni_citate, verifica)
				SELECT $1, $2, 1, NULL, 3, false, repeat('3', 64), repeat('2', 64), NULL, 0, repeat('1', 64),
				       COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = $1), 0),
				       $3, 1, '{"rotta": "motore-a/bom/conferma"}', $4::jsonb, repeat('5', 64), '{"formato": 3}', $5::jsonb, $6::jsonb, 'nessuno', gen_random_uuid(), $7,
				       '{}'::uuid[], '{"regola": "prima-istanza-1"}'
				RETURNING conferma_id`, s.thread, s.prodotto, b1, string(snap), string(mappa), string(decisioni), s.utente).Scan(&c1); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatalf("la conferma formato 3: %v", err)
			}
			for _, x := range []passoSQL{
				{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo,
					step_strutturale_id, step_sha256, emittente, emittente_presunto, namespace)
				  SELECT $1, $2, $3, true, 'k-p', '9120100', NULL, NULL, 'finito', $4, d.sha256, $5, true, NULL FROM documento d WHERE d.documento_id = $4`,
					[]any{c1, s.thread, s.prodotto, s.docStep, cliente}},
				{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo,
					emittente, emittente_presunto, namespace) VALUES ($1, $2, $3, false, 'k-f', '9120101', '2', NULL, 'sciolto', $4, true, NULL)`,
					[]any{c1, s.thread, s.figlio, cliente}},
				{`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ($1, $2, $3, 2, 'conferma')`, []any{c1, s.prodotto, s.figlio}},
				{`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = $2 WHERE bozza_id = $1`, []any{b1, s.utente}},
			} {
				if _, err := tx.Exec(ctx, x.sql, x.arg...); err != nil {
					_ = tx.Rollback(ctx)
					t.Fatalf("la conferma formato 3: %v\n%s", err, x.sql)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("COMMIT della conferma formato 3: %v", err)
			}
			ris, err := svc.Calcola(ctx, s.thread)
			if err != nil {
				t.Fatal(err)
			}
			if u, ok := ris.V1.Conferme[s.prodotto]; !ok || !u.Valida {
				t.Fatalf("l'ultima conferma del prodotto non è valida: la prova non dice niente (%+v)", u)
			}
			if got := chiede2D(t); got != c.chiede {
				t.Errorf("esito %s: il figlio chiede il 2D %v, atteso %v", c.esito, got, c.chiede)
			}
		})
	}
}
