//go:build integrazione

// Impalcatura condivisa dei test L4 di questo package (la stessa di `app/runtime`,
// `platform/coda` e `platform/storage/staging`: ripetuta, non condivisa).
package documenti

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

// tutto sono le capacità accese: è lo stato di default del processo (platform/coda/capacita.go).
var tutto = coda.Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}

// conCapacita fissa le capacità per la durata del test e le rimette com'erano.
func conCapacita(t *testing.T, c coda.Capacita) {
	t.Helper()
	coda.ImpostaCapacita(c)
	t.Cleanup(func() { coda.ImpostaCapacita(tutto) })
}
