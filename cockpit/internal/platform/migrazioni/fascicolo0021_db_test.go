//go:build integrazione

// L4 — la 0021 (Fascicolo v3): le note sui disegni, e il ritorno manuale 20 → 21 → 20 con
// scripts/0021_indietro.sql. I vincoli della tabella e chi scrive le note hanno le loro prove in
// transport/web (v3_db_test.go, TestLeNoteSuiDisegni).

package migrazioni_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func applica21(p *pgxpool.Pool) error {
	_, err := migrazioni.ApplicaFinoA(context.Background(), p, risorse.FS, 21, testutil.LogSilenzioso())
	return err
}

// indietro21 legge lo script com'e'; con perdiLeNote fa il gesto che il file chiede all'operatore per
// togliere le note lo stesso: la variabile a true.
func indietro21(t *testing.T, perdiLeNote bool) string {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "0021_indietro.sql"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(sql)
	if perdiLeNote {
		const riga = "SET LOCAL cockpit.perdi_le_note = 'false';"
		if strings.Count(s, riga) != 1 {
			t.Fatalf("0021_indietro.sql non ha piu' la riga %q", riga)
		}
		s = strings.Replace(s, riga, "SET LOCAL cockpit.perdi_le_note = 'true';", 1)
	}
	return s
}

// 20 → 21 → 20: lo schema torna identico a quello della 20, i dati restano, e la 0021 si riapplica
// sopra il ritorno.
func TestIlRitornoManualeDalla21Alla20(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()

	testutil.SchemaFinoA(t, p, 20)
	prima := impronta18(t, p)
	esegui18(t, p, base18)
	if err := applica21(p); err != nil {
		t.Fatal(err)
	}
	if !applicata(t, p, 21) || !uno[bool](t, p, `SELECT to_regclass('public.annotazione_pdf') IS NOT NULL`) {
		t.Fatal("la 0021 non ha creato annotazione_pdf")
	}

	if _, err := p.Exec(ctx, indietro21(t, false)); err != nil {
		t.Fatalf("0021_indietro.sql: %v", err)
	}
	if d := differenze(prima, impronta18(t, p)); d != "" {
		t.Errorf("lo schema dopo 20 → 21 → 20 non e' quello della 20:\n%s", d)
	}
	if v := uno[int32](t, p, `SELECT max(versione) FROM schema_versione`); v != 20 {
		t.Errorf("schema_versione e' a %d dopo il ritorno: doveva tornare a 20", v)
	}
	if n := uno[int](t, p, `SELECT count(*) FROM allegato WHERE allegato_id = '{A1}'`); n != 1 {
		t.Errorf("il ritorno ha toccato gli allegati: %d", n)
	}

	if err := applica21(p); err != nil {
		t.Fatalf("la 0021 non si riapplica dopo il ritorno: %v", err)
	}
	if !applicata(t, p, 21) {
		t.Error("la 0021 riapplicata non risulta registrata")
	}
}

// Con delle note il ritorno si ferma e non cambia niente; con la variabile a true le toglie. Da uno schema
// che non e' alla 21 si rifiuta.
func TestIlRitornoDalla21SiFermaSulleNote(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()

	testutil.SchemaFinoA(t, p, 21)
	esegui18(t, p, base18)
	esegui18(t, p, `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da)
		VALUES ('{T1}', '{A1}', 1, 0.3, 0.4, 'Tolleranza da verificare', '{U}');`)

	_, err := p.Exec(ctx, indietro21(t, false))
	if err == nil || !strings.Contains(err.Error(), "ci sono 1 note sui disegni") {
		t.Fatalf("atteso il rifiuto con una nota, ottenuto %v", err)
	}
	// la connessione che ha eseguito lo script puo' essere rimasta in una transazione fallita: si
	// controlla da una nuova
	if !applicata(t, p, 21) || uno[int](t, p, `SELECT count(*) FROM annotazione_pdf`) != 1 {
		t.Fatal("il ritorno fermato ha cambiato qualcosa")
	}

	if _, err := p.Exec(ctx, indietro21(t, true)); err != nil {
		t.Fatalf("0021_indietro.sql con perdi_le_note: %v", err)
	}
	if applicata(t, p, 21) || uno[bool](t, p, `SELECT to_regclass('public.annotazione_pdf') IS NOT NULL`) {
		t.Error("con perdi_le_note il ritorno non e' avvenuto")
	}

	_, err = p.Exec(ctx, indietro21(t, false))
	if err == nil || !strings.Contains(err.Error(), "non alla 21") {
		t.Errorf("atteso il rifiuto da uno schema alla 20, ottenuto %v", err)
	}
}
