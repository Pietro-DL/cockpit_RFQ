package runtime

// L1 — V1, corsia file (U2 §3.13 punti 3 e 5; direttiva 35, C-6a): l'avvio non prepara nessun deposito locale degli
// «Altri file» e non scrive niente sul disco; il posto delle copie e' nas.Conservatore sulla radice del NAS, con la
// capacita' nas_scrittura: spenta, «Metti in Altri file» risponde 503 (nas_scrittura), senza radice 503
// (conservazione). Mai un falso successo.

import (
	"context"
	"os"
	"testing"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/testutil"
)

// TestCicloFileCapacitaRilettaAlGesto (U2 §3.5): la capacita' accesa all'avvio e poi spenta (ImpostaCapacita) spegne il
// posto al gesto dopo; senza [nas].radice nessun posto (503 conservazione).
func TestCicloFileCapacitaRilettaAlGesto(t *testing.T) {
	defer coda.ImpostaCapacita(coda.CapacitaAttuali())
	cfg := &config.Config{}
	cfg.NAS.Radice = t.TempDir()
	coda.ImpostaCapacita(coda.Capacita{NasScrittura: true})
	svc, err := preparaCicloFile(context.Background(), cfg, nil, t.TempDir(), coda.Capacita{NasScrittura: true}, testutil.LogSilenzioso())
	if err != nil {
		t.Fatal(err)
	}
	if ok, motivo := svc.Conservatore.Disponibile(); !ok {
		t.Fatalf("con la scrittura accesa il posto non e' disponibile: %q", motivo)
	}
	coda.ImpostaCapacita(coda.Capacita{})
	if ok, motivo := svc.Conservatore.Disponibile(); ok || motivo != "nas_scrittura" {
		t.Errorf("spenta la capacita' il posto resta disponibile: %v %q", ok, motivo)
	}
	cfg.NAS.Radice = ""
	senza, err := preparaCicloFile(context.Background(), cfg, nil, t.TempDir(), coda.Capacita{NasScrittura: true}, testutil.LogSilenzioso())
	if err != nil || senza.ConservazioneConfigurata() {
		t.Errorf("senza [nas].radice: posto %v, %v", senza != nil && senza.ConservazioneConfigurata(), err)
	}
}

func TestCicloFileSenzaDepositoLocale(t *testing.T) {
	for _, acceso := range []bool{false, true} {
		cartella := t.TempDir()
		cfg := &config.Config{}
		cfg.MotoreA.Anteprima = acceso
		cfg.NAS.Radice = cartella
		svc, err := preparaCicloFile(context.Background(), cfg, nil, cartella, coda.Capacita{}, testutil.LogSilenzioso())
		if err != nil {
			t.Fatalf("flag %v: l'avvio si ferma: %v", acceso, err)
		}
		if !svc.ConservazioneConfigurata() {
			t.Errorf("flag %v: il servizio non ha il posto degli Altri file sulla radice del NAS", acceso)
		}
		if ok, motivo := svc.Conservatore.Disponibile(); ok || motivo != "nas_scrittura" {
			t.Errorf("flag %v: con la scrittura spenta il posto e' disponibile (%v, %q)", acceso, ok, motivo)
		}
		voci, err := os.ReadDir(cartella)
		if err != nil {
			t.Fatal(err)
		}
		if len(voci) != 0 {
			t.Errorf("flag %v: l'avvio ha scritto %d voci sul disco", acceso, len(voci))
		}
	}
}
