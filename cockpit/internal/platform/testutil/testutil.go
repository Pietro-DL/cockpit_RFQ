// Package testutil fornisce ai test d'integrazione (L4) un pool sul DB di test e uno schema pulito.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./...
//
// `-p 1` è obbligatorio: i pacchetti condividono un solo database e alcuni test ricreano lo schema.
// Senza, due pacchetti si distruggono lo schema a vicenda e gli errori che ne escono ("cache lookup
// failed for type …") non hanno nulla a che vedere con il codice in prova.
//
// Senza COCKPIT_TEST_DSN i test che lo richiedono vengono SALTATI (visibili come SKIP, mai come PASS).
// Per sicurezza il nome del database deve contenere "test": lo schema viene distrutto e ricreato
// (vedi DatabaseDiTest per come si legge il nome).
package testutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
)

// DSN restituisce COCKPIT_TEST_DSN o salta il test.
func DSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("COCKPIT_TEST_DSN")
	if dsn == "" {
		t.Skip("COCKPIT_TEST_DSN non impostata: test d'integrazione saltato")
	}
	if err := DatabaseDiTest(dsn); err != nil {
		t.Fatalf("COCKPIT_TEST_DSN: %v", err)
	}
	return dsn
}

// DatabaseDiTest dice se un DSN porta davvero a un database il cui nome contiene "test": i test che
// lo usano distruggono e ricreano lo schema, e su un database di sviluppo lo farebbero lo stesso.
//
// Il nome si legge come lo leggerà pgx al momento di connettersi, non dal testo del DSN. Prima si
// guardava il percorso dell'URL, e passavano tre forme che portano altrove: un DSN chiave=valore
// («host=x user=tester dbname=cockpit_dev» non ha un percorso, e «tester» bastava a sembrare un
// test), un URL con `?dbname=` che vince sul percorso, e un DSN senza database, dove decide
// PGDATABASE o il nome dell'utente. Per tutte e tre fa fede il nome risolto.
func DatabaseDiTest(dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("DSN non leggibile: %w", err)
	}
	if !strings.Contains(strings.ToLower(cfg.Database), "test") {
		return fmt.Errorf("deve portare a un database il cui nome contiene \"test\": porta a %q", cfg.Database)
	}
	return nil
}

// Pool apre un pool sul DB di test e lo chiude alla fine del test.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(context.Background(), DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// SchemaVuoto distrugge e ricrea lo schema public senza applicare migrazioni.
func SchemaVuoto(t testing.TB, p *pgxpool.Pool) {
	t.Helper()
	if _, err := p.Exec(context.Background(), "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}

// SchemaPulito ricrea lo schema e applica tutte le migrazioni incorporate.
func SchemaPulito(t testing.TB, p *pgxpool.Pool) {
	t.Helper()
	SchemaVuoto(t, p)
	if _, err := migrazioni.Applica(context.Background(), p, risorse.FS, LogSilenzioso()); err != nil {
		t.Fatalf("migrazioni: %v", err)
	}
}

// SchemaPresente si limita ad applicare le migrazioni mancanti: non distrugge nulla. Serve ai test
// che puliscono da sé le righe che creano e che non devono dipendere dall'ordine di esecuzione.
func SchemaPresente(t testing.TB, p *pgxpool.Pool) {
	t.Helper()
	if _, err := migrazioni.Applica(context.Background(), p, risorse.FS, LogSilenzioso()); err != nil {
		t.Fatalf("migrazioni: %v", err)
	}
}

// SchemaFinoA ricrea lo schema e applica le migrazioni fino alla versione indicata (test S2).
func SchemaFinoA(t testing.TB, p *pgxpool.Pool, v int) {
	t.Helper()
	SchemaVuoto(t, p)
	if _, err := migrazioni.ApplicaFinoA(context.Background(), p, risorse.FS, v, LogSilenzioso()); err != nil {
		t.Fatalf("migrazioni fino a %d: %v", v, err)
	}
}

// Conta restituisce il numero di righe di una tabella.
func Conta(t testing.TB, p *pgxpool.Pool, tabella string) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(), "SELECT count(*) FROM "+tabella).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", tabella, err)
	}
	return n
}

// LogSilenzioso è un logger che scrive solo warning ed errori: i test restano leggibili.
func LogSilenzioso() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}
