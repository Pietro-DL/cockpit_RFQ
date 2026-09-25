package runtime

// L1 — i pezzi puri dell'avvio e dell'esecutore: il livello del log, i comandi che leggono soltanto, il
// rifiuto di uno schema diverso, quali fallimenti non si riprovano, il conto degli scarti.

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"promatec/cockpit/internal/ai/agente"
	"promatec/cockpit/internal/platform/storage/nas"
)

// Prima tutto quello che non era «debug» valeva info: «warn» nel file non abbassava niente.
func TestIlLivelloDelLogAccettaWarnEError(t *testing.T) {
	for _, c := range []struct {
		scritto string
		livello slog.Level
		ok      bool
	}{
		{"debug", slog.LevelDebug, true},
		{"", slog.LevelInfo, true},
		{"info", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{" WARNING ", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"verboso", slog.LevelInfo, false},
	} {
		l, ok := LivelloLog(c.scritto)
		if l != c.livello || ok != c.ok {
			t.Errorf("LivelloLog(%q) = %v, %v; atteso %v, %v", c.scritto, l, ok, c.livello, c.ok)
		}
	}
}

func TestIComandiCheLeggonoSoltanto(t *testing.T) {
	for _, c := range []struct {
		o       Opzioni
		lettura bool
	}{
		{Opzioni{ContaAnagrafiche: true}, true},
		{Opzioni{SemeFornitori: "seme.json"}, true},
		{Opzioni{SemeFornitori: "seme.json", ApplicaFornitori: true}, false},
		{Opzioni{SemeAnagrafica: "seme.json"}, false},
		{Opzioni{SoloMigrazioni: true}, false},
		{Opzioni{}, false},
	} {
		if got := c.o.SoloLettura(); got != c.lettura {
			t.Errorf("%+v: SoloLettura = %v, atteso %v", c.o, got, c.lettura)
		}
	}
}

func TestUnoSchemaDiversoFermaIComandiInLettura(t *testing.T) {
	if err := stessoSchema(21, 21); err != nil {
		t.Errorf("stesso schema: %v", err)
	}
	if err := stessoSchema(15, 21); err == nil || !strings.Contains(err.Error(), "-migra") || !strings.Contains(err.Error(), "backup") {
		t.Errorf("schema piu' vecchio: %v, atteso che dica backup e -migra", err)
	}
	if err := stessoSchema(22, 21); err == nil || !strings.Contains(err.Error(), "aggiornato") {
		t.Errorf("schema piu' nuovo: %v, atteso che chieda il cockpit.exe aggiornato", err)
	}
}

// Un tipo che il server non sa eseguire, l'analisi semantica spenta, l'estrazione non configurata: si
// ritentavano cinque volte senza che niente potesse cambiare.
func TestIFallimentiCheNonSiRisolvonoRiprovandoSonoDefinitivi(t *testing.T) {
	for _, c := range []struct {
		err        error
		definitivo bool
	}{
		{fmt.Errorf("%w: %s", errTipoNonGestito, "sposta_nas"), true},
		{fmt.Errorf("analisi: %w", agente.ErrSpento), true},
		{errArchiviNonConfigurati, true},
		{fmt.Errorf("%w: X", nas.ErrConflitto), true},
		{errors.New("il NAS ha risposto tardi"), false},
	} {
		if got := definitivo(c.err); got != c.definitivo {
			t.Errorf("definitivo(%v) = %v, atteso %v", c.err, got, c.definitivo)
		}
	}
}

// «scartati» contava i byte del JSON: un elenco vuoto «[]» ne diceva 2.
func TestGliScartiSiContanoPerElemento(t *testing.T) {
	for _, c := range []struct {
		grezzo string
		n      int
	}{
		{`[]`, 0},
		{`[{"campo": "codice", "motivo": "non nel testo"}, {"campo": "rev", "motivo": "vuota"}]`, 2},
		{``, 0},
		{`null`, 0},
	} {
		if got := contaScartati(json.RawMessage(c.grezzo)); got != c.n {
			t.Errorf("contaScartati(%s) = %d, attesi %d", c.grezzo, got, c.n)
		}
	}
}
