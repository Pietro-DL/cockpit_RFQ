package fascicolo

// L1 — le regole pure del Fascicolo: il gate del congelamento (A4.6, passo 3; A4.5), il ciclo nella
// struttura, la differenza fra una versione e la working (A4.7).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

func step(codice, esito string, deroga, derogaCad bool) db.ListGateStepRow {
	r := db.ListGateStepRow{VStepProdotto: db.VStepProdotto{ComponenteID: uuid.New(), Codice: codice, Esito: esito,
		MotivoParziale: pgtype.Text{String: "lettura troncata: troppi nodi", Valid: esito == StepParziale}}, DerogaCad: derogaCad}
	if deroga {
		r.VStepProdotto.DerogaStrutturaID = uuid.NullUUID{UUID: uuid.New(), Valid: true}
	}
	return r
}

// Prova 24, la regola: una condizione per volta, e il gate e' rosso con la sua frase.
func TestIlGateSiFermaSuOgniCondizione(t *testing.T) {
	pulito := db.GateCongelamentoRow{}
	casi := []struct {
		nome   string
		c      db.GateCongelamentoRow
		step   []db.ListGateStepRow
		archi  []Arco
		frase  string // "" = passa
		avviso string
	}{
		{nome: "tutto a posto", step: []db.ListGateStepRow{step("P1", StepAnalizzato, false, false)}},
		{nome: "requisiti bloccanti", c: db.GateCongelamentoRow{NBloccanti: 2}, frase: "2 requisiti bloccanti"},
		{nome: "proposte di componente", c: db.GateCongelamentoRow{NProposteComponente: 1}, frase: "1 proposte strutturali aperte (1 componenti, 0 relazioni, 0 rimozioni)"},
		{nome: "proposte di relazione", c: db.GateCongelamentoRow{NProposteRelazione: 3}, frase: "3 proposte strutturali aperte"},
		{nome: "proposte di rimozione", c: db.GateCongelamentoRow{NProposteRimozione: 1}, frase: "0 relazioni, 1 rimozioni"},
		{nome: "documenti in errore", c: db.GateCongelamentoRow{NDocumentiErrore: 1}, frase: "1 documenti della BOM in errore sul NAS"},
		{nome: "anomalie NAS", c: db.GateCongelamentoRow{NAnomalieNas: 1}, frase: "1 anomalie NAS aperte"},
		{nome: "STEP parziale senza deroga", step: []db.ListGateStepRow{step("P1", StepParziale, false, true)}, frase: "serve una deroga strutturale"},
		{nome: "STEP parziale con la deroga strutturale", step: []db.ListGateStepRow{step("P1", StepParziale, true, false)}},
		{nome: "STEP non analizzato senza deroga", step: []db.ListGateStepRow{step("P1", StepNonAnalizzato, false, false)}, frase: "PRESENTE, NON ANALIZZATO"},
		{nome: "STEP da scegliere, anche con le deroghe", step: []db.ListGateStepRow{step("P1", StepDaScegliere, true, true)}, frase: "si sceglie lo STEP strutturale"},
		{nome: "riferimento superato, anche con le deroghe", step: []db.ListGateStepRow{step("P1", StepRiferimentoSuperato, true, true)}, frase: "RIFERIMENTO SUPERATO"},
		{nome: "STEP mancante", step: []db.ListGateStepRow{step("P1", StepMancante, false, false)}, frase: "P1: STEP prodotto finito: MANCANTE — da sollecitare"},
		{nome: "STEP mancante con la deroga del fabbisogno", step: []db.ListGateStepRow{step("P1", StepMancante, false, true)}},
		{nome: "solo un altro 3D con la deroga del fabbisogno", step: []db.ListGateStepRow{step("P1", StepSoloAltro3d, false, true)}},
		{nome: "sul portale senza deroga", step: []db.ListGateStepRow{step("P1", StepSulPortale, false, false)}, frase: "SUL PORTALE"},
		{nome: "da confermare senza deroga", step: []db.ListGateStepRow{step("P1", StepDaConfermare, false, false)}, frase: "DA CONFERMARE"},
		{nome: "radice senza qualifica: avviso", step: []db.ListGateStepRow{step("S1", StepRadiceSenzaQualifica, false, false)}, avviso: "radice senza qualifica"},
	}
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	casi = append(casi, struct {
		nome   string
		c      db.GateCongelamentoRow
		step   []db.ListGateStepRow
		archi  []Arco
		frase  string
		avviso string
	}{nome: "ciclo", c: pulito, archi: []Arco{{a, b}, {b, c}, {c, a}}, frase: "la struttura ha un ciclo: A → B → C → A"})
	codici := map[uuid.UUID]string{a: "A", b: "B", c: "C"}
	for _, cs := range casi {
		t.Run(cs.nome, func(t *testing.T) {
			g := Valuta(cs.c, cs.step, cs.archi, codici)
			if cs.frase == "" {
				if !g.Passa() {
					t.Fatalf("doveva passare: %v", g.Problemi)
				}
			} else {
				if g.Passa() {
					t.Fatalf("doveva fermarsi con «%s»", cs.frase)
				}
				if !strings.Contains(g.Motivo(), cs.frase) {
					t.Fatalf("motivo %q, atteso che contenga %q", g.Motivo(), cs.frase)
				}
			}
			if cs.avviso != "" && (len(g.Avvisi) != 1 || !strings.Contains(g.Avvisi[0], cs.avviso)) {
				t.Errorf("avvisi %v, atteso %q", g.Avvisi, cs.avviso)
			}
		})
	}
}

func TestCiclo(t *testing.T) {
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if got := Ciclo([]Arco{{a, b}, {a, c}, {b, d}, {c, d}}); got != nil {
		t.Errorf("un diamante non e' un ciclo: %v", got)
	}
	if got := Ciclo([]Arco{{a, b}, {b, a}}); len(got) != 3 || got[0] != got[2] {
		t.Errorf("A → B → A: %v", got)
	}
	if got := Ciclo([]Arco{{a, a}}); len(got) != 2 {
		t.Errorf("un pezzo dentro se stesso: %v", got)
	}
	if got := Ciclo(nil); got != nil {
		t.Errorf("nessun arco: %v", got)
	}
}

func TestEtichettaStepEDeterministica(t *testing.T) {
	for _, e := range []string{StepRadiceSenzaQualifica, StepRiferimentoSuperato, StepNonAnalizzato, StepAnalizzato, StepParziale,
		StepDaScegliere, StepSoloAltro3d, StepDaConfermare, StepSulPortale, StepMancante} {
		if EtichettaStep(e) != EtichettaStep(e) || strings.Contains(EtichettaStep(e), "sconosciuto") {
			t.Errorf("%s: %q", e, EtichettaStep(e))
		}
	}
	if got := EtichettaStep(StepMancante); got != "STEP prodotto finito: MANCANTE — da sollecitare" {
		t.Errorf("mancante: %q", got)
	}
}

func riga(oggetto, chiave, prima, dopo string) db.DiffBomWorkingRow {
	r := db.DiffBomWorkingRow{Oggetto: oggetto, Chiave: chiave}
	if prima != "" {
		r.Prima = json.RawMessage(prima)
	}
	if dopo != "" {
		r.Dopo = json.RawMessage(dopo)
	}
	return r
}

// La differenza V(n) ↔ working (A4.7): aggiunto, tolto, cambiato nei campi; uguale = niente.
func TestLeDifferenzeVedonoAggiunteTolteECambi(t *testing.T) {
	righe := []db.DiffBomWorkingRow{
		riga("componente", "k1", `{"codice":"P1","qta":1,"rev":null}`, `{"codice":"P1","qta":1,"rev":null}`),
		riga("componente", "k2", `{"codice":"F1","qta":1,"rev":"A"}`, `{"codice":"F1","qta":2,"rev":"B"}`),
		riga("componente", "k3", "", `{"codice":"N1"}`),
		riga("arco", "k1>k2", `{"qta":2}`, ""),
		riga("deroga", "k2:sviluppo_dxf", `{"motivo":"lo facciamo noi"}`, `{"motivo":"lo manda il cliente"}`),
		riga("deroga_struttura", "g1", "null", `{"motivo":"va bene"}`),
		riga("documento", "d1", "null", "null"),
	}
	diff, err := Differenze(righe)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range diff {
		got = append(got, d.String())
	}
	atteso := []string{
		"componente k2 cambiato (qta, rev)",
		"componente k3 aggiunto",
		"arco k1>k2 tolto",
		"deroga k2:sviluppo_dxf cambiato (motivo)",
		"deroga_struttura g1 aggiunto",
	}
	if strings.Join(got, "\n") != strings.Join(atteso, "\n") {
		t.Errorf("differenze:\n%s\natteso:\n%s", strings.Join(got, "\n"), strings.Join(atteso, "\n"))
	}
	if _, err := Differenze([]db.DiffBomWorkingRow{riga("componente", "x", "{rotto", "")}); err == nil {
		t.Error("un JSON rotto deve essere un errore, non una differenza vuota")
	}
}
