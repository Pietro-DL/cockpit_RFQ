package distintabozza

// L1 — la revisione con un secondo STEP (riconciliazione3.go, preparaRevisione; direttiva 39 §2-§3 [U + T-39], scelta
// [T-39] della corsia persistenza): la vista preparata per il cambio della fonte toglie la casella della fonte di prima ai
// soli nodi registrati che il registro nuovo non ha per rif, con la base confermata; i nodi proposti, il prodotto, gli
// archi che non li toccano restano com'erano; la versione di partenza non cambia. Senza base confermata, o fra due
// occorrenze dello stesso contenuto (stessi rif), la riconciliazione non vale. Dati sintetici.

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

func TestV1D39PreparaRevisioneConDueStep(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	shaA, shaB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	prodotto, comp := uuid.New(), uuid.New()
	rifA := func(k string) string { return "nodo:" + shaA + ":" + k }
	conferma := uuid.New()
	prima := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: prodotto, VersioneBomBase: &VersioneBomBase{ConfermaID: &conferma, Numero: 1, Contatore: 3},
		Sorgente: &Sorgente3{AllegatoID: &a, Sha256: shaA, Gruppo: "bom"},
		Nodi: []Nodo3{
			{ChiaveBozza: "p", ComponenteID: &prodotto, Op: OpRiusa, Tipo: "finito", Origine: OrigineRegistrato, Caselle: []Casella3{{AllegatoID: a, Gruppo: "bom", Rif: rifA("#1")}},
				Identita: Identita{Codice: "ACM-1000", Modo: ModoRegistrato}},
			{ChiaveBozza: "x", ComponenteID: &comp, Op: OpRiusa, Tipo: "sciolto", Origine: OrigineStep, Caselle: []Casella3{{AllegatoID: a, Gruppo: "bom", Rif: rifA("#2")}},
				Identita: Identita{Codice: "ACM-1100", Modo: ModoRegistrato}},
			{ChiaveBozza: "y", Op: OpCrea, Tipo: "sciolto", Origine: OrigineStep, Caselle: []Casella3{{AllegatoID: a, Gruppo: "bom", Rif: rifA("#3")}},
				Identita: Identita{Codice: "ACM-1150", Modo: ModoProposta}},
		},
		Archi: []Arco3{
			{Padre: "p", Figlio: "x", Qta: 1, Origine: OrigineStep, Step: &StepArco3{AllegatoID: a, Gruppo: "bom"}},
			{Padre: "p", Figlio: "y", Qta: 4, Origine: OrigineStep, Step: &StepArco3{AllegatoID: a, Gruppo: "bom"}},
		},
	}
	voce := func(sha, k string) importazione.Voce {
		return importazione.Voce{Origine: importazione.OrigineVoce{Rif: "nodo:" + sha + ":" + k}}
	}
	nuovoB := importazione.Contenuto{Proposta: importazione.Proposta{Nodi: []importazione.Voce{voce(shaB, "#1"), voce(shaB, "#5"), voce(shaB, "#6")}}}

	r, ok := preparaRevisione(prima, nuovoB)
	if !ok || len(r.origini) != 1 || r.origini["x"] != OrigineStep || r.allegatoPrima != a {
		t.Fatalf("revisione: %v %+v", ok, r.origini)
	}
	x, y, p := r.snapshot.Nodi[1], r.snapshot.Nodi[2], r.snapshot.Nodi[0]
	if len(x.Caselle) != 0 || x.Origine != OrigineConferma || len(y.Caselle) != 1 || y.Origine != OrigineStep || len(p.Caselle) != 1 {
		t.Fatalf("nodi preparati: %+v %+v %+v", x, y, p)
	}
	if a0, a1 := r.snapshot.Archi[0], r.snapshot.Archi[1]; a0.Origine != OrigineConferma || a0.Step != nil || a1.Origine != OrigineStep || a1.Step == nil ||
		!r.archi[[2]string{"p", "x"}] || r.archi[[2]string{"p", "y"}] {
		t.Fatalf("archi preparati: %+v %+v %v", a0, a1, r.archi)
	}
	// la versione di partenza non cambia
	if len(prima.Nodi[1].Caselle) != 1 || prima.Nodi[1].Origine != OrigineStep || prima.Archi[0].Step == nil || prima.Archi[0].Origine != OrigineStep {
		t.Fatalf("la versione di partenza e' cambiata: %+v %+v", prima.Nodi[1], prima.Archi[0])
	}
	if s, ok := PreparaRevisione(prima, nuovoB); !ok || len(s.Nodi[1].Caselle) != 0 {
		t.Fatalf("PreparaRevisione: %v %+v", ok, s.Nodi[1])
	}

	// senza base confermata: cambio-fonte-1 com'era
	senza := prima.copia()
	senza.VersioneBomBase = &VersioneBomBase{}
	if _, ok := preparaRevisione(senza, nuovoB); ok {
		t.Fatal("senza base confermata la riconciliazione della revisione non vale")
	}
	// lo stesso contenuto su un'altra occorrenza (stessi rif): niente da riconciliare
	stesso := importazione.Contenuto{Proposta: importazione.Proposta{Nodi: []importazione.Voce{voce(shaA, "#1"), voce(shaA, "#2"), voce(shaA, "#3")}}}
	if _, ok := preparaRevisione(prima, stesso); ok {
		t.Fatal("fra due occorrenze dello stesso contenuto la revisione non vale")
	}
	// la voce del registrato fuori per decisione nel registro nuovo: il nodo resta con cambio-fonte-1
	fuori := nuovoB
	fuori.Proposta.FuoriPerDecisione = []importazione.FuoriPerDecisione{{Voce: "n:" + b.String() + ":" + rifA("#2")}}
	if _, ok := preparaRevisione(prima, fuori); ok {
		t.Fatal("un rif del registrato fra i fuori per decisione del registro nuovo: il nodo non e' da riconciliare")
	}
	// una bozza senza struttura segue cambio-fonte-2, non la revisione
	manuale := prima.copia()
	manuale.Sorgente = nil
	if _, ok := preparaRevisione(manuale, nuovoB); ok {
		t.Fatal("la bozza senza sorgente non e' una revisione")
	}
}
