//go:build integrazione

// L4 — i comandi che leggono soltanto (-conta-anagrafiche, -anteprima-fornitori) non migrano, non seminano e
// non toccano la coda: con uno schema diverso da quello del binario si fermano e dicono di fare -migra dopo
// un backup. E un avvio che si ferma lo scrive anche nel log del server, non solo sullo stderr.

package runtime

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func ultimaDelBinario(t *testing.T) int {
	t.Helper()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	return migs[len(migs)-1].Versione
}

// tomlDiProva e' un cockpit.toml minimo sul database di prova, con lo staging (e quindi il log) in una
// cartella temporanea.
func tomlDiProva(t *testing.T) (percorso, staging string) {
	t.Helper()
	d := t.TempDir()
	staging = filepath.Join(d, "staging")
	percorso = filepath.Join(d, "cockpit.toml")
	corpo := "[db]\ndsn = '" + testutil.DSN(t) + "'\n[nas]\nstaging = '" + staging + "'\n"
	if err := os.WriteFile(percorso, []byte(corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	return percorso, staging
}

func TestIComandiInLetturaNonMigranoIlDatabase(t *testing.T) {
	p := testutil.Pool(t)
	ctx := context.Background()
	ultima := ultimaDelBinario(t)
	testutil.SchemaFinoA(t, p, ultima-1)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	cfg, staging := tomlDiProva(t)
	prima := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prima) })

	for _, o := range []Opzioni{{ContaAnagrafiche: true}, {SemeFornitori: filepath.Join(t.TempDir(), "seme_fornitori.json")}} {
		err := Esegui(cfg, config.Rete{}, o)
		if err == nil || !strings.Contains(err.Error(), "-migra") || !strings.Contains(err.Error(), "backup") {
			t.Errorf("%+v su uno schema vecchio: %v, atteso che si fermi e dica backup e -migra", o, err)
		}
		applicate, err := migrazioni.Applicate(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if v := ultimaApplicata(applicate); v != ultima-1 {
			t.Errorf("%+v ha portato lo schema alla %d: un comando che legge non migra", o, v)
		}
		// Semina scrive l'analizzatore corrente a ogni avvio: qui non deve essere partita
		if n := testutil.Conta(t, p, "analizzatore_corrente"); n != 0 {
			t.Errorf("%+v ha seminato (analizzatore_corrente: %d righe)", o, n)
		}
	}
	// il motivo dell'avvio fallito sta anche nel log del server: un'attivita' pianificata una finestra
	// non ce l'ha
	b, err := os.ReadFile(filepath.Join(staging, "log", "cockpit.log"))
	if err != nil {
		t.Fatalf("il log del server non c'e': %v", err)
	}
	if !strings.Contains(string(b), "cockpit si ferma") || !strings.Contains(string(b), "-migra") {
		t.Errorf("il log non dice perche' l'avvio si e' fermato:\n%s", b)
	}
}

func TestIlDatabaseInLetturaNonScrive(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.DB.DSN = testutil.DSN(t)
	pool, err := ApriDatabaseInLettura(ctx, cfg, risorse.FS)
	if err != nil {
		t.Fatalf("con lo schema del binario il database si apre: %v", err)
	}
	defer pool.Close()
	if err := ContaAnagrafiche(ctx, pool); err != nil {
		t.Fatalf("il conto in sola lettura: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('RO', 'Prova', 'IT')`)
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != "25006" {
		t.Errorf("una scrittura dal pool in sola lettura: %v, atteso read_only_sql_transaction (25006)", err)
	}
}
