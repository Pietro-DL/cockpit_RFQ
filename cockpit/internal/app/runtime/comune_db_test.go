//go:build integrazione

// Impalcatura condivisa dei test L4 di questo package.
//
// Sono le stesse funzioni che stanno in `platform/coda`, `platform/storage/staging` e
// `core/rfq/documenti`:
// ripetute, non condivise. Un helper di test non si esporta per farselo prestare da un altro
// package — diventerebbe una dipendenza vera fra due package che nel codice vero non si parlano —
// e dodici righe di impalcatura costano meno di quel legame.
package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

func preparaDB(t *testing.T) (*pgxpool.Pool, *db.Queries, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	return p, db.New(p), context.Background()
}

// accoda mette un job pronto e restituisce la riga.
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

func statoDi(t *testing.T, ctx context.Context, q *db.Queries, id int64) db.Job {
	t.Helper()
	j, err := q.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get job %d: %v", id, err)
	}
	return j
}

// tutto sono le capacità accese: è lo stato di default del processo (vedi platform/coda/capacita.go).
var tutto = coda.Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}

// conCapacita fissa le capacità per la durata del test e le rimette com'erano.
func conCapacita(t *testing.T, c coda.Capacita) {
	t.Helper()
	coda.ImpostaCapacita(c)
	t.Cleanup(func() { coda.ImpostaCapacita(tutto) })
}

// contenutoDiProva scrive un contenuto nella cache con l'orario indicato e ne restituisce percorso e
// hash.
func contenutoDiProva(t *testing.T, radice, testo string, quando time.Time) (string, string) {
	t.Helper()
	somma := sha256.Sum256([]byte(testo))
	sha := hex.EncodeToString(somma[:])
	p, err := staging.PercorsoContenuto(radice, sha, "x.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(testo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, quando, quando); err != nil {
		t.Fatal(err)
	}
	return p, sha
}
