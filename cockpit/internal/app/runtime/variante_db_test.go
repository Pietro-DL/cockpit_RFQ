//go:build integrazione

// L4 — la protezione dalle due varianti della 0022 (direttiva 11 par.2) vista dalle tre aperture del database
// del runtime: l'avvio che migra (ApriDatabase), i comandi in sola lettura (ApriDatabaseInLettura) e il comando
// U5 che scrive senza migrare (ApriDatabaseSenzaMigrare). Con la versione 22 registrata ma un'altra variante,
// tutte e tre si fermano con VarianteDiversa e non toccano lo schema.

package runtime

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func TestUnaVarianteDiversaFermaLeTreAperture(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.DB.DSN = testutil.DSN(t)

	// con la variante giusta le tre aperture vanno
	if pool, _, err := ApriDatabase(ctx, cfg, risorse.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("ApriDatabase con la variante giusta: %v", err)
	} else {
		pool.Close()
	}
	if pool, err := ApriDatabaseInLettura(ctx, cfg, risorse.FS); err != nil {
		t.Fatalf("ApriDatabaseInLettura con la variante giusta: %v", err)
	} else {
		pool.Close()
	}
	if pool, err := ApriDatabaseSenzaMigrare(ctx, cfg, risorse.FS); err != nil {
		t.Fatalf("ApriDatabaseSenzaMigrare con la variante giusta: %v", err)
	} else {
		pool.Close()
	}

	// un'altra variante della 22 (come un database con un testo precedente della 0022)
	if _, err := p.Exec(ctx, `UPDATE schema_variante SET variante = '0022-un-altro-testo' WHERE versione = 22`); err != nil {
		t.Fatal(err)
	}
	ferma := func(nome string, err error) {
		t.Helper()
		var vd *migrazioni.VarianteDiversa
		if !errors.As(err, &vd) || vd.Versione != 22 || vd.DelDatabase != "0022-un-altro-testo" {
			t.Errorf("%s: atteso VarianteDiversa sulla 22, ottenuto %v", nome, err)
		}
	}
	pool, _, err := ApriDatabase(ctx, cfg, risorse.FS, slog.New(slog.DiscardHandler))
	if pool != nil {
		pool.Close()
	}
	ferma("ApriDatabase", err)
	poolL, err := ApriDatabaseInLettura(ctx, cfg, risorse.FS)
	if poolL != nil {
		poolL.Close()
	}
	ferma("ApriDatabaseInLettura", err)
	poolU, err := ApriDatabaseSenzaMigrare(ctx, cfg, risorse.FS)
	if poolU != nil {
		poolU.Close()
	}
	ferma("ApriDatabaseSenzaMigrare", err)

	// nessuna delle tre ha toccato la riga
	var v string
	if err := p.QueryRow(ctx, `SELECT variante FROM schema_variante WHERE versione = 22`).Scan(&v); err != nil || v != "0022-un-altro-testo" {
		t.Errorf("la variante nel database dopo i tre rifiuti: %q %v", v, err)
	}
}
