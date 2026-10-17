package valutazione_test

// L1 — LS-01 (direttiva 23 §4.1; 17/10/2026): la versione della struttura STEP è una sola, dal produttore ai consumatori.
//
// GP-3 aveva alzato a 4 la versione scritta dal worker e la costante Go del contratto, e la prova di parità di allora
// confrontava lo schema Python con la costante Go: verde. Nessuno guardava la funzione SQL della completezza (0020), che
// accetta solo la 3, né la regola della gerarchia, che vuole la 3: ogni STEP letto diventava «parziale». Qui i quattro
// numeri si confrontano insieme, senza database:
//   - la costante del worker Python (workers/step_struttura.py, VERSIONE_STRUTTURA e VERSIONE_DIAGNOSTICA);
//   - il contratto Go (worker.VersioneStruttura, worker.VersioneDiagnostica);
//   - la regola della gerarchia (valutazione.VersioneStrutturaStep);
//   - il letterale della funzione SQL struttura_motivo_parziale nell'ULTIMA migrazione che la definisce.
//
// Non sostituisce la prova con il worker vero e il database (workerapi, TestLS01UscitaVeraDelWorkerAttraversaLaCatena),
// che passa un'uscita reale per DTO, SQL, vista, fotografia, Motore A e gerarchia: questa la anticipa in L1, e dice quale
// dei quattro si è mosso da solo. valutazione non importa il contratto del worker (dipendenze_test.go); una prova sì.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// radiceCockpit: la cartella del modulo, dalla cartella del pacchetto (internal/core/valutazione).
const radiceCockpit = "../../.."

var (
	reCostantePython = func(nome string) *regexp.Regexp {
		return regexp.MustCompile(`(?m)^` + nome + `\s*=\s*(\d+)\s*$`)
	}
	reDefinizioneMotivo = regexp.MustCompile(`(?is)CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+struttura_motivo_parziale\s*\(.*?\$\$(.*?)\$\$`)
	reVersioneAccettata = regexp.MustCompile(`\(s\s*->\s*'versione'\)\s*IS\s+DISTINCT\s+FROM\s+'(\d+)'::jsonb`)
)

// costantePython legge una costante intera del worker dal sorgente: il worker non ha un'altra porta da cui il Go la veda.
func costantePython(t *testing.T, sorgente, nome string) int {
	t.Helper()
	m := reCostantePython(nome).FindAllStringSubmatch(sorgente, -1)
	if len(m) != 1 {
		t.Fatalf("workers/step_struttura.py: %d definizioni di %s, attesa una", len(m), nome)
	}
	n, err := strconv.Atoi(m[0][1])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// versioneAccettataDallaSQL: il letterale di «(s -> 'versione') IS DISTINCT FROM 'N'::jsonb» nell'ultima migrazione che
// definisce struttura_motivo_parziale (CREATE o CREATE OR REPLACE), con il file che la definisce.
func versioneAccettataDallaSQL(t *testing.T) (int, string) {
	t.Helper()
	file, err := filepath.Glob(filepath.Join(radiceCockpit, "migrations", "*.sql"))
	if err != nil || len(file) == 0 {
		t.Fatalf("migrazioni non trovate: %v", err)
	}
	sort.Strings(file) // 0001_… 0022_…: l'ordine dei nomi è l'ordine di applicazione
	ultima, corpo := "", ""
	for _, f := range file {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reDefinizioneMotivo.FindAllStringSubmatch(string(b), -1) {
			ultima, corpo = filepath.Base(f), m[1]
		}
	}
	if ultima == "" {
		t.Fatal("nessuna migrazione definisce struttura_motivo_parziale")
	}
	v := reVersioneAccettata.FindAllStringSubmatch(corpo, -1)
	if len(v) != 1 {
		t.Fatalf("%s: struttura_motivo_parziale ha %d controlli della versione, atteso uno: la prova va riscritta insieme alla funzione", ultima, len(v))
	}
	n, err := strconv.Atoi(v[0][1])
	if err != nil {
		t.Fatal(err)
	}
	return n, ultima
}

// TestLS01LaVersioneDellaStrutturaEUnaSola: il worker scrive la forma che SQL e gerarchia accettano, e la diagnostica ha
// lo stesso numero nei due linguaggi.
func TestLS01LaVersioneDellaStrutturaEUnaSola(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(radiceCockpit, "workers", "step_struttura.py"))
	if err != nil {
		t.Fatal(err)
	}
	sorgente := strings.ReplaceAll(string(raw), "\r\n", "\n")
	python := costantePython(t, sorgente, "VERSIONE_STRUTTURA")
	diagnosticaPython := costantePython(t, sorgente, "VERSIONE_DIAGNOSTICA")
	sql, migrazione := versioneAccettataDallaSQL(t)

	if python != worker.VersioneStruttura || worker.VersioneStruttura != valutazione.VersioneStrutturaStep || valutazione.VersioneStrutturaStep != sql {
		t.Errorf("la versione della struttura non è una sola: worker Python %d, contratto Go %d, gerarchia %d, SQL (%s) %d",
			python, worker.VersioneStruttura, valutazione.VersioneStrutturaStep, migrazione, sql)
	}
	if diagnosticaPython != worker.VersioneDiagnostica || worker.VersioneDiagnostica < 1 {
		t.Errorf("la versione della diagnostica: worker Python %d, contratto Go %d", diagnosticaPython, worker.VersioneDiagnostica)
	}
	if worker.VersioneStrutturaGP3 == sql {
		t.Errorf("la forma ritirata di GP-3 (%d) è quella che la SQL accetta (%d)", worker.VersioneStrutturaGP3, sql)
	}
}

// TestLS01LaGerarchiaNonLeggeLaFormaRitirata: con gli stessi numeri di una lettura completa, la forma che il worker scrive
// è riconosciuta; la 4 di GP-3 e una versione sconosciuta no (non_analizzabile, come per la 0020 sono parziali). Nessuna
// scorciatoia: la versione diversa non diventa «riconosciuta» perché i numeri sono buoni.
func TestLS01LaGerarchiaNonLeggeLaFormaRitirata(t *testing.T) {
	completa := func(versione int) valutazione.AnalisiStruttura {
		return valutazione.AnalisiStruttura{Letta: true, Versione: versione, Radici: 1, Relazioni: 3}
	}
	casi := []struct {
		nome     string
		versione int
		atteso   valutazione.GerarchiaStep
	}{
		{"la forma del worker di oggi", worker.VersioneStruttura, valutazione.GerarchiaStepRiconosciuta},
		{"la 4 di GP-3", worker.VersioneStrutturaGP3, valutazione.GerarchiaStepNonAnalizzabile},
		{"una versione sconosciuta", worker.VersioneStruttura + 2, valutazione.GerarchiaStepNonAnalizzabile},
		{"la v2", 2, valutazione.GerarchiaStepNonAnalizzabile},
	}
	for _, c := range casi {
		if got := valutazione.GerarchiaStepDi(completa(c.versione)); got != c.atteso {
			t.Errorf("%s (v%d): %s, atteso %s", c.nome, c.versione, got, c.atteso)
		}
	}
}
