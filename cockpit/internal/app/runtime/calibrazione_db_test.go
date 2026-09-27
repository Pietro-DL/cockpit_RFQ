//go:build integrazione

// L4 — il comando `cockpit -calibrazione` (Smistamento M3, A5.14.6): è un comando che legge soltanto. Non
// migra uno schema vecchio (si ferma e dice backup e -migra), e sullo schema del binario apre il database
// in sola lettura e stampa per prima cosa quale database sta leggendo.

package runtime

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// 236 (parte posta, il comando) — le misure stesse sono provate in core/calibrazione con una scena dai
// risultati noti; qui la strada del comando: sola lettura, nessuna migrazione, prima riga il database.
func TestIlComandoCalibrazioneLeggeSoltanto(t *testing.T) {
	p := testutil.Pool(t)
	ctx := context.Background()
	ultima := ultimaDelBinario(t)
	testutil.SchemaFinoA(t, p, ultima-1)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	cfgFile, _ := tomlDiProva(t)
	prima := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prima) })

	dal := time.Now()
	err := Esegui(cfgFile, config.Rete{}, Opzioni{Calibrazione: true, CalibrazioneDal: &dal})
	if err == nil || !strings.Contains(err.Error(), "-migra") || !strings.Contains(err.Error(), "backup") {
		t.Errorf("-calibrazione su uno schema vecchio: %v, atteso che si fermi e dica backup e -migra", err)
	}
	applicate, err := migrazioni.Applicate(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if v := ultimaApplicata(applicate); v != ultima-1 {
		t.Errorf("-calibrazione ha portato lo schema alla %d: un comando che legge non migra", v)
	}
	if n := testutil.Conta(t, p, "analizzatore_corrente"); n != 0 {
		t.Errorf("-calibrazione ha seminato (analizzatore_corrente: %d righe)", n)
	}

	// sullo schema del binario: il pool in sola lettura, la stampa che comincia dal database
	testutil.SchemaPulito(t, p)
	cfg := &config.Config{}
	cfg.DB.DSN = testutil.DSN(t)
	pool, err := ApriDatabaseInLettura(ctx, cfg, risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var b bytes.Buffer
	if err := Calibrazione(ctx, pool, &b, nil); err != nil {
		t.Fatalf("-calibrazione: %v", err)
	}
	cc := pool.Config().ConnConfig
	righe := strings.Split(b.String(), "\n")
	if !strings.Contains(righe[0], cc.Database) || !strings.Contains(righe[0], cc.Host) || strings.Contains(righe[0], "@") {
		t.Errorf("la prima riga deve dire database e host, senza credenziali: %q", righe[0])
	}
	if !strings.Contains(righe[1], "sola lettura: sì") {
		t.Errorf("la transazione delle misure non è in sola lettura: %q", righe[1])
	}
	if !strings.Contains(b.String(), "nessuna decisione con la fotografia") {
		t.Errorf("su un database senza decisioni:\n%s", b.String())
	}
}
