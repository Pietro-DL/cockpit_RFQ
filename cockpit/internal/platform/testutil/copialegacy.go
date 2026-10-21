package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// iniziaTransazione: un *pgxpool.Pool, una *pgx.Conn o una pgx.Tx (che apre un savepoint).
type iniziaTransazione interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// CopiaLegacy scrive nel database di prova un dato legacy della copia documento_proposta.componente_id: una
// pre-assegnazione (P1) del vecchio «Assegna», cioe' la copia cambiata senza AR e senza evento del giornale, come la
// scrivevano le pagine legacy prima della 0023.
//
// Dalla variante 20261019d della 0023 trg_proposta_copia_derivata e' acceso (direttiva 39, dopo C-6b): un cambio della
// copia senza l'evento del servizio unico nella stessa transazione e' rifiutato al COMMIT (P4X10). I dati che le prove
// preparano cosi' sono precedenti a quella regola: la scrittura si fa in una transazione propria con il trigger spento
// solo per lei (ALTER TABLE … DISABLE TRIGGER, la scrittura, ENABLE TRIGGER, COMMIT; un evento differito non si accoda
// mentre il trigger e' spento, e alla fine il trigger torna com'era). Senza la 0023 la scrittura e' diretta. Solo su
// un database il cui nome contiene «test» (come SchemaVuoto). [T-39, corsia persistenza]
func CopiaLegacy(t testing.TB, d iniziaTransazione, sql string, args ...any) {
	t.Helper()
	ctx := context.Background()
	tx, err := d.Begin(ctx)
	if err != nil {
		t.Fatalf("copia legacy: inizio: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var nome string
	if err := tx.QueryRow(ctx, `SELECT current_database()`).Scan(&nome); err != nil {
		t.Fatalf("copia legacy: nome del database: %v", err)
	}
	if !strings.Contains(strings.ToLower(nome), "test") {
		t.Fatalf("copia legacy: il database %q non e' un database di prova", nome)
	}
	var stato string
	err = tx.QueryRow(ctx, `SELECT t.tgenabled::text FROM pg_trigger t
		 WHERE t.tgname = 'trg_proposta_copia_derivata' AND t.tgrelid = 'documento_proposta'::regclass`).Scan(&stato)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("copia legacy: stato del trigger: %v", err)
	}
	acceso := err == nil && stato == "O"
	if acceso {
		if _, err := tx.Exec(ctx, `ALTER TABLE documento_proposta DISABLE TRIGGER trg_proposta_copia_derivata`); err != nil {
			t.Fatalf("copia legacy: trigger spento: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("copia legacy: %v\n%s", err, sql)
	}
	if acceso {
		if _, err := tx.Exec(ctx, `ALTER TABLE documento_proposta ENABLE TRIGGER trg_proposta_copia_derivata`); err != nil {
			t.Fatalf("copia legacy: trigger riacceso: %v", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("copia legacy: COMMIT: %v", err)
	}
}
