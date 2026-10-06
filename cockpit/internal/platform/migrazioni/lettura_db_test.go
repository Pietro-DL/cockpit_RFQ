//go:build integrazione

// L4 — l'apertura in sola lettura sul database di prova (A1c-L4S-01; piano A, 6.4.3; P-02): il pool di ApriInLettura
// rifiuta una scrittura (25006) anche dentro una transazione che non dice niente; con lo schema alla versione
// prima di quella del binario si ferma con SchemaDiverso e non migra; ControllaSolaLettura legge davvero il
// catalogo e ferma il ruolo delle prove, che può scrivere. Il rifiuto di schema di ApriDatabaseSenzaMigrare,
// rimasta in app/runtime, lo provano le prove di app/runtime, invariate.
//
// Le righe di queste prove sono inventate (ACME): nessun dato reale, il repository è pubblico.

package migrazioni_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func ultimaDelBinarioDiProva(t *testing.T) int {
	t.Helper()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	return migs[len(migs)-1].Versione
}

func TestApriInLetturaNonScrive(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()

	pool, err := migrazioni.ApriInLettura(ctx, testutil.DSN(t), risorse.FS)
	if err != nil {
		t.Fatalf("con lo schema del binario il database si apre: %v", err)
	}
	defer pool.Close()
	var ro string
	if err := pool.QueryRow(ctx, "SHOW default_transaction_read_only").Scan(&ro); err != nil || ro != "on" {
		t.Errorf("default_transaction_read_only = %q (%v), atteso «on»", ro, err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('RO', 'Prova ACME', 'IT')`)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "25006" {
		t.Errorf("una scrittura dal pool in sola lettura: %v, atteso read_only_sql_transaction (25006)", err)
	}
	if n := testutil.Conta(t, p, "utente"); n != 0 {
		t.Errorf("la scrittura rifiutata ha lasciato %d righe", n)
	}
}

func TestApriInLetturaConUnoSchemaVecchioSiFerma(t *testing.T) {
	p := testutil.Pool(t)
	ctx := context.Background()
	ultima := ultimaDelBinarioDiProva(t)
	testutil.SchemaFinoA(t, p, ultima-1)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })

	pool, err := migrazioni.ApriInLettura(ctx, testutil.DSN(t), risorse.FS)
	if err == nil {
		pool.Close()
		t.Fatal("uno schema alla versione prima di quella del binario si è aperto")
	}
	var sd *migrazioni.SchemaDiverso
	if !errors.As(err, &sd) || sd.DelDatabase != ultima-1 || sd.DelBinario != ultima {
		t.Fatalf("errore %v, atteso SchemaDiverso{%d, %d}", err, ultima-1, ultima)
	}
	applicate, err := migrazioni.Applicate(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if v := migrazioni.UltimaApplicata(applicate); v != ultima-1 {
		t.Errorf("l'apertura in lettura ha portato lo schema alla %d: non migra", v)
	}
}

// Il ruolo delle prove possiede il database di prova, quindi può scrivere: ControllaSolaLettura deve fermarlo,
// leggendo il catalogo vero (le domande sono quelle che userà il banco sulla copia del dump).
func TestControllaSolaLetturaSulDatabaseDiProva(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	pool, err := migrazioni.ApriInLettura(ctx, testutil.DSN(t), risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	col, err := migrazioni.ControllaSolaLettura(ctx, pool, []string{"utente", "tabella_che_non_esiste"})
	if err == nil {
		t.Fatalf("il ruolo delle prove, che può scrivere, passa per un ruolo in sola lettura: %+v", col)
	}
	if !col.SolaLettura {
		t.Errorf("il pool di ApriInLettura risulta senza default_transaction_read_only: %+v", col)
	}
	if col.Database != p.Config().ConnConfig.Database || col.Utente == "" {
		t.Errorf("utente e database letti male: %+v", col)
	}
	if !col.Scrive && !col.Superutente && len(col.Ruoli) == 0 {
		t.Errorf("l'errore (%v) non viene da un permesso del ruolo: %+v", err, col)
	}
	if strings.Contains(err.Error(), "SET ROLE") {
		t.Errorf("current_user e session_user devono coincidere sul pool delle prove: %v", err)
	}
}
