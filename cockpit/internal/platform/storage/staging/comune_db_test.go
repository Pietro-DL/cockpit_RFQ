//go:build integrazione

// Impalcatura condivisa dei test L4 di questo package (la stessa di `internal/jobs` e di
// `platform/coda`: ripetuta, non condivisa).
package staging

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func preparaDB(t *testing.T) (*pgxpool.Pool, *db.Queries, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	return p, db.New(p), context.Background()
}
