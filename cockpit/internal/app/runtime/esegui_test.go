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
		{Opzioni{Calibrazione: true}, true}, // Smistamento M3: -calibrazione legge soltanto
		{Opzioni{SemeFornitori: "seme.json"}, true},
		{Opzioni{SemeFornitori: "seme.json", ApplicaFornitori: true}, false},
		{Opzioni{SemeAnagrafica: "seme.json"}, false},
		{Opzioni{SoloMigrazioni: true}, false},
		{Opzioni{}, false},
		// Smistamento F7: il comando U5 legge soltanto in anteprima, scrive con il nome del database
		{Opzioni{RiapriAgganci: true}, true},
		{Opzioni{RiapriAgganci: true, DatabaseRiapertura: "cockpit_prova_test"}, false},
	} {
		if got := c.o.SoloLettura(); got != c.lettura {
			t.Errorf("%+v: SoloLettura = %v, atteso %v", c.o, got, c.lettura)
		}
	}
}

// Prova 201 (A5.15.2, P38): il comando U5 in applicazione vuole il nome del database, e il nome deve essere
// quello del DSN del file (prima di collegarsi) e quello che il server dice di essere (dopo). La prima riga
// dice il database come lo leggera' pgx, senza la password.
func TestIlComandoVuoleIlNomeDelDatabase(t *testing.T) {
	for _, c := range []struct {
		nome                     string
		argomento, dsn, corrente string
		frase                    string // contenuta nell'errore; "" = passa
	}{
		{"il nome giusto, prima di collegarsi", "cockpit_prova_test", "cockpit_prova_test", "", ""},
		{"il nome giusto, collegati", "cockpit_prova_test", "cockpit_prova_test", "cockpit_prova_test", ""},
		{"nessun nome", "", "cockpit_prova_test", "", "vuole il nome del database"},
		{"un altro nome", "cockpit_dev", "cockpit_prova_test", "", `il file punta a "cockpit_prova_test", hai scritto "cockpit_dev": controlla -config`},
		{"maiuscole diverse", "Cockpit_Prova_Test", "cockpit_prova_test", "", "hai scritto"},
		{"il server e' un altro", "cockpit_prova_test", "cockpit_prova_test", "cockpit_altro_test", `il server collegato dice di essere "cockpit_altro_test"`},
	} {
		err := ControllaNomeDatabase(c.argomento, c.dsn, c.corrente)
		switch {
		case c.frase == "" && err != nil:
			t.Errorf("%s: %v", c.nome, err)
		case c.frase != "" && (err == nil || !strings.Contains(err.Error(), c.frase)):
			t.Errorf("%s: %v, atteso che dica %q", c.nome, err, c.frase)
		case c.frase != "" && !strings.Contains(err.Error(), "controlla -config") && c.argomento != "":
			t.Errorf("%s: %v, atteso che dica di controllare -config", c.nome, err)
		}
	}

	for _, c := range []struct {
		nome, dsn, testo, database string
	}{
		{"URL", "postgres://utente_prova:segreta@127.0.0.1:5433/cockpit_prova_test", "127.0.0.1:5433/cockpit_prova_test come utente_prova", "cockpit_prova_test"},
		// il nome e' quello risolto: `?dbname=` vince sul percorso
		{"URL con dbname", "postgres://utente_prova:segreta@127.0.0.1:5433/cockpit_dev?dbname=cockpit_prova_test", "127.0.0.1:5433/cockpit_prova_test come utente_prova", "cockpit_prova_test"},
		{"chiave=valore", "host=127.0.0.1 port=5433 user=utente_prova password=segreta dbname=cockpit_prova_test", "127.0.0.1:5433/cockpit_prova_test come utente_prova", "cockpit_prova_test"},
		{"IPv6", "postgres://utente_prova:segreta@[::1]:5433/cockpit_prova_test", "[::1]:5433/cockpit_prova_test come utente_prova", "cockpit_prova_test"},
	} {
		testo, nome, err := DestinazioneDelDSN(c.dsn)
		if err != nil || testo != c.testo || nome != c.database {
			t.Errorf("%s: %q %q %v; atteso %q %q", c.nome, testo, nome, err, c.testo, c.database)
		}
		if strings.Contains(testo, "segreta") {
			t.Errorf("%s: la destinazione dice la password: %q", c.nome, testo)
		}
	}
	if _, _, err := DestinazioneDelDSN("postgres://utente_prova:segreta@127.0.0.1:porta/x"); err == nil || strings.Contains(err.Error(), "segreta") {
		t.Errorf("un DSN che non si legge: %v (senza ripetere il DSN)", err)
	}
}

// Il comando U5 dalla riga di comando (A5.15.2): le due forme insieme sono un errore, come per i fornitori;
// -riapri-agganci vuole il nome del database; -rfq e -uscita valgono solo con il comando; -rfq e' un uuid.
func TestLeOpzioniDelComandoU5(t *testing.T) {
	id := "6f1c2d3e-4b5a-4c6d-8e7f-001122334455"
	for _, c := range []struct {
		nome                         string
		anteprima, applica           bool
		database, rfq, uscita, frase string
		atteso                       Opzioni
	}{
		{nome: "niente", atteso: Opzioni{}},
		{nome: "anteprima", anteprima: true, atteso: Opzioni{RiapriAgganci: true}},
		{nome: "anteprima di una RFQ", anteprima: true, rfq: id, uscita: "r.json", atteso: Opzioni{RiapriAgganci: true, UscitaRiapertura: "r.json"}},
		{nome: "applicazione", applica: true, database: " cockpit_prova_test ", atteso: Opzioni{RiapriAgganci: true, DatabaseRiapertura: "cockpit_prova_test"}},
		{nome: "le due forme insieme", anteprima: true, applica: true, database: "cockpit_prova_test", frase: "prima si guarda, poi si scrive"},
		{nome: "applicazione senza nome", applica: true, frase: "vuole il nome del database"},
		{nome: "-rfq da solo", rfq: id, frase: "valgono solo con"},
		{nome: "-uscita da sola", uscita: "r.json", frase: "valgono solo con"},
		{nome: "-rfq che non e' un uuid", anteprima: true, rfq: "7120001", frase: "un uuid"},
	} {
		var o Opzioni
		err := o.PreparaRiapertura(c.anteprima, c.applica, c.database, c.rfq, c.uscita)
		if c.frase != "" {
			if err == nil || !strings.Contains(err.Error(), c.frase) {
				t.Errorf("%s: %v, atteso che dica %q", c.nome, err, c.frase)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.nome, err)
			continue
		}
		if c.rfq != "" && (o.RfqRiapertura == nil || o.RfqRiapertura.String() != c.rfq) {
			t.Errorf("%s: -rfq = %v", c.nome, o.RfqRiapertura)
		}
		o.RfqRiapertura = nil
		if o != c.atteso {
			t.Errorf("%s: %+v, atteso %+v", c.nome, o, c.atteso)
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
	// Smistamento F7: il comando U5 in applicazione scrive ma non migra, e lo dice
	if err := schemaDelBinario(15, 21, "non migra"); err == nil || strings.Contains(err.Error(), "legge soltanto") ||
		!strings.Contains(err.Error(), "non migra") || !strings.Contains(err.Error(), "backup") {
		t.Errorf("schema piu' vecchio per un comando che scrive: %v", err)
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
