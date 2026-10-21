package distintabozza

// L1 — giro G39-ter (21/10), corsia motore, G39-06 [U] («caso il nostro motore non riesca a consigliare la scelta
// giusta ... lasciamo la scelta di conferma all'utente sul file che preferisce»; lettura [T] dell'orchestratore): la PB
// su uno STEP candidato del finito (StepCandidati.BloccaPB, NodoVersoUnFinito). La candidatura univoca è sempre esclusa
// (G39-02 [U]), come il ripiego che non distingue le univoche; la candidatura non univoca è esclusa solo verso un prodotto
// finito (il nodo del prodotto della bozza, un nodo di tipo finito, il nodo di un componente finito della RFQ), non
// verso una parte o un sottoassieme. Falliva su 9eb44e3 (R2 e R4 escludevano ogni candidatura). Dati sintetici.

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/confermabom"
)

func TestV1G39terBloccaPBSoloVersoUnFinitoPerLeNonUnivoche(t *testing.T) {
	prodotto, altroFinito, sotto, parte := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	univoco, nonUnivoco, ordinario := uuid.New(), uuid.New(), uuid.New()
	st := confermabom.NuovoStatoRFQ(uuid.New(), 0, []confermabom.Componente{
		{ID: prodotto, CodiceLetterale: "12345678Z", Tipo: "finito"}, {ID: altroFinito, CodiceLetterale: "12345678Y", Tipo: "finito"},
		{ID: sotto, CodiceLetterale: "12345678-100", Tipo: "sottoassieme"}, {ID: parte, CodiceLetterale: "12345678-200", Tipo: "sciolto"}}, nil, nil, nil)
	snap := Snapshot3{ProdottoID: prodotto, Nodi: []Nodo3{
		{ChiaveBozza: "k-root", ComponenteID: &prodotto, Op: OpRiusa, Tipo: "finito"},
		{ChiaveBozza: "k-sotto", ComponenteID: &sotto, Op: OpRiusa, Tipo: "sottoassieme"},
		{ChiaveBozza: "k-parte", ComponenteID: &parte, Op: OpRiusa, Tipo: "sciolto"},
		{ChiaveBozza: "k-nuova", Op: OpCrea, Tipo: "sciolto"},
		{ChiaveBozza: "k-altro", ComponenteID: &altroFinito, Op: OpRiusa, Tipo: "sottoassieme"},
		{ChiaveBozza: "k-fin-nuovo", Op: OpCrea, Tipo: "finito"},
	}}
	verso := map[string]bool{"k-root": true, "k-sotto": false, "k-parte": false, "k-nuova": false, "k-altro": true, "k-fin-nuovo": true, "k-assente": true}
	for k, atteso := range verso {
		if got := NodoVersoUnFinito(snap, k, st); got != atteso {
			t.Errorf("NodoVersoUnFinito(%s) = %v, atteso %v", k, got, atteso)
		}
	}
	motore := &StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{univoco: {prodotto}, nonUnivoco: {prodotto, altroFinito}},
		Univoche: map[uuid.UUID]uuid.UUID{univoco: prodotto}}
	ripiego := &StepCandidati{Prodotti: map[uuid.UUID][]uuid.UUID{nonUnivoco: {prodotto}}}
	for _, c := range []struct {
		nome   string
		s      *StepCandidati
		a      uuid.UUID
		chiave string
		blocca bool
	}{
		{"univoca verso il sottoassieme", motore, univoco, "k-sotto", true},
		{"univoca verso il finito", motore, univoco, "k-root", true},
		{"non univoca verso il sottoassieme", motore, nonUnivoco, "k-sotto", false},
		{"non univoca verso la parte", motore, nonUnivoco, "k-parte", false},
		{"non univoca verso una parte nuova", motore, nonUnivoco, "k-nuova", false},
		{"non univoca verso il prodotto della bozza", motore, nonUnivoco, "k-root", true},
		{"non univoca verso il nodo di un altro finito della RFQ", motore, nonUnivoco, "k-altro", true},
		{"non univoca verso un nodo nuovo di tipo finito", motore, nonUnivoco, "k-fin-nuovo", true},
		{"il ripiego verso la parte", ripiego, nonUnivoco, "k-parte", true},
		{"un file ordinario verso il finito", motore, ordinario, "k-root", false},
		{"nessun riconoscimento", nil, nonUnivoco, "k-root", false},
	} {
		if got := c.s.BloccaPB(c.a, NodoVersoUnFinito(snap, c.chiave, st)); got != c.blocca {
			t.Errorf("%s: BloccaPB = %v, atteso %v", c.nome, got, c.blocca)
		}
	}
}
