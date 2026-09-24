//go:build integrazione

// L4 — l'analizzatore corrente (addendum A4.5, B8.A4a): l'avvio scrive in analizzatore_corrente la
// versione e l'hash della configurazione [analisi], cosi' v_step_prodotto sa quale analisi di uno STEP
// e' quella corrente. La data cambia solo se cambia la chiave.

package runtime

import (
	"testing"
	"time"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/testutil"
)

func TestLAvvioScriveLAnalizzatoreCorrente(t *testing.T) {
	_, q, ctx := preparaDB(t)
	cfg := &config.Config{}
	cfg.Analisi.Versione = 3
	cfg.Analisi.Parametri = map[string]any{"termini": []any{"scala"}}
	if err := Semina(ctx, q, cfg, 20, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	ac, err := q.GetAnalizzatoreCorrente(ctx)
	if err != nil {
		t.Fatalf("l'avvio non ha scritto l'analizzatore corrente: %v", err)
	}
	atteso := coda.Analizzatore{Versione: 3, Parametri: cfg.Analisi.Parametri}.Hash()
	if ac.VersioneAnalizzatore != 3 || ac.HashConfigurazione != atteso {
		t.Fatalf("analizzatore corrente v%d %s, atteso v3 %s", ac.VersioneAnalizzatore, ac.HashConfigurazione, atteso)
	}
	time.Sleep(10 * time.Millisecond)
	if err := Semina(ctx, q, cfg, 20, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	stesso, _ := q.GetAnalizzatoreCorrente(ctx)
	if !stesso.ImpostatoIl.Equal(ac.ImpostatoIl) {
		t.Error("lo stesso avvio ha riscritto la data: la chiave non era cambiata")
	}
	cfg.Analisi.Parametri = map[string]any{"termini": []any{"scala", "materiale"}}
	if err := Semina(ctx, q, cfg, 20, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	nuovo, _ := q.GetAnalizzatoreCorrente(ctx)
	if nuovo.HashConfigurazione == ac.HashConfigurazione || !nuovo.ImpostatoIl.After(ac.ImpostatoIl) {
		t.Errorf("una configurazione nuova non ha cambiato la chiave: %s (%v)", nuovo.HashConfigurazione, nuovo.ImpostatoIl)
	}
}
