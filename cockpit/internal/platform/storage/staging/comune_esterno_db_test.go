//go:build integrazione

// Impalcatura dei test che stanno FUORI dal package.
//
// `platform/coda` importa `platform/storage/staging` per l'interfaccia Staging. Un test di staging
// che debba accodare un job non può quindi stare dentro il package — sarebbe un ciclo — e sta nel
// package esterno `staging_test`, che può importarli tutti e due. Da lì si vede solo ciò che staging
// esporta, ed è giusto così: prova la cartella di lavoro da fuori, come la usa il resto del sistema.
package staging_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func preparaDB(t *testing.T) (*pgxpool.Pool, *db.Queries, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	return p, db.New(p), context.Background()
}

func accoda(t *testing.T, ctx context.Context, q *db.Queries, tipo db.TipoJob, chiave string, o coda.Opzioni) db.Job {
	t.Helper()
	j, err := coda.AccodaCon(ctx, q, tipo, map[string]any{"prova": true}, chiave, 5, o)
	if err != nil {
		t.Fatalf("accoda %s: %v", tipo, err)
	}
	if j == nil {
		t.Fatalf("accoda %s: nessun job (chiave %q già pendente?)", tipo, chiave)
	}
	return *j
}

func claim(t *testing.T, ctx context.Context, q *db.Queries, worker db.WorkerTipo, id string) *db.Job {
	t.Helper()
	j, err := coda.Claim(ctx, q, worker, id, coda.Destinazione{}, 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return j
}
