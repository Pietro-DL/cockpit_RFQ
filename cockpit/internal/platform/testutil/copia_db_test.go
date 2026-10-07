//go:build integrazione && privato

// L4 — la copia usa e getta del dump (A1c-L4T-02; piano A, 6.4.7 e 6.7.5; R33 d, R10, R44): PoolCopiaDelDump passa
// sulla copia _run del manifest (nome, marcatore, utente, schema, sentinelle), e la transazione è davvero
// scrivibile; con la copia intatta al suo posto i controlli si fermano. Senza COCKPIT_DATASET_A,
// COCKPIT_DUMP_COPIA_DSN o, per la seconda parte, COCKPIT_DUMP_DSN, la prova è NON ESEGUITA, mai saltata; la
// seconda parte apre la copia intatta con PoolDump, e senza le impronte della copia nel manifest ha la parte delle
// impronte NON ESEGUITA (R117 b).
//
// Nessun valore reale nel codice (E-18): nomi, ruoli, schema e sentinelle vengono dal manifest privato. La prova
// non scrive righe: la scrittura è una UPDATE che non tocca niente, dentro una transazione chiusa con ROLLBACK.

package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestL4CopiaPoolCopiaDelDump(t *testing.T) {
	m := DatasetA(t)
	p := PoolCopiaDelDump(t, m.CopiaRun)
	ctx := context.Background()

	tx, err := p.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE documento_proposta SET stato = stato WHERE false`); err != nil {
		t.Errorf("la copia usa e getta non è scrivibile dal suo ruolo: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	t.Run("con la copia intatta al suo posto", func(t *testing.T) {
		intatta := PoolDump(t, m.Copia)
		if err := controllaCopiaScrivibile(ctx, leggiDalPool(intatta), m.CopiaRun); err == nil {
			t.Error("i controlli della copia usa e getta passano sulla copia intatta")
		}
	})
}
