// L1 — il tipo proposto dalla grammatica (decisione dell'utente del 14/10, [U]), provato sulla funzione unica del tipo
// proposto (bomDelThread.tipoProposto, D3): una voce commerciale o minuteria della tabella delle classi, su un nodo senza
// figli, dà commerciale con l'origine proposto_dalla_grammatica, prima della riga e della regola; un nodo con dei figli,
// o senza voce, tiene la catena di prima. I valori sono inventati.
package valutazione

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

func TestClassiIlTipoPropostoDallaGrammatica(t *testing.T) {
	b := &bomDelThread{radici: map[string]radiceDiTarget{}, radiciDelTarget: map[string]map[string]bool{},
		tipi: map[string][]tipoDellaRiga{"nodo:riga": {{tipo: tipoSciolto}}}}
	commerciale := &motorea.ClasseLetta{Segmento: "serie", Valore: "8", Categoria: grammatica.CategoriaCommerciale, Descrizione: "Boccola inventata"}
	minuteria := &motorea.ClasseLetta{Segmento: "serie", Valore: "9", Categoria: grammatica.CategoriaMinuteria, Descrizione: "Vite inventata"}
	altra := &motorea.ClasseLetta{Segmento: "serie", Valore: "7", Categoria: "altra", Descrizione: "fuori dall'enum"}
	for _, c := range []struct {
		nome     string
		rif      string
		conFigli bool
		classe   *motorea.ClasseLetta
		atteso   string
	}{
		{"foglia commerciale", "nodo:x", false, commerciale, "commerciale/proposto_dalla_grammatica"},
		{"foglia minuteria", "nodo:x", false, minuteria, "commerciale/proposto_dalla_grammatica"},
		{"foglia commerciale con la riga: la grammatica prima", "nodo:riga", false, commerciale, "commerciale/proposto_dalla_grammatica"},
		{"con i figli: la regola", "nodo:x", true, commerciale, "sottoassieme/proposto_dalla_regola"},
		{"con i figli e la riga: la riga", "nodo:riga", true, minuteria, "sciolto/proposto_dalla_riga"},
		{"senza voce: la regola", "nodo:x", false, nil, "sciolto/proposto_dalla_regola"},
		{"una categoria fuori dall'enum non propone", "nodo:x", false, altra, "sciolto/proposto_dalla_regola"},
	} {
		tipo, origine := b.tipoProposto(c.rif, "", c.conFigli, c.classe)
		if got := tipo + "/" + string(origine); got != c.atteso {
			t.Errorf("%s: %s, atteso %s", c.nome, got, c.atteso)
		}
	}
}

// TestClassiNessunFinitoSenzaAutoritaSeDecideLaGrammatica: regressione del 14/10. Una foglia senza decisione con una
// voce commerciale della tabella delle classi prende commerciale/proposto_dalla_grammatica anche se una riga aperta
// propone «finito» senza autorità: la diagnostica del «finito» senza autorità (che dice «il nodo prende un'altra
// proposta aperta o la regola strutturale») non si emette per quel nodo. Si emette ancora per una foglia senza voce e
// per un nodo con dei figli e la voce (lì la grammatica non decide). I valori sono inventati.
func TestClassiNessunFinitoSenzaAutoritaSeDecideLaGrammatica(t *testing.T) {
	finito := func(n byte) []tipoDellaRiga { return []tipoDellaRiga{{tipo: tipoFinito, riga: uuid.UUID{15: n}}} }
	b := &bomDelThread{radici: map[string]radiceDiTarget{}, radiciDelTarget: map[string]map[string]bool{}, archiTolti: map[string]bool{},
		tipi: map[string][]tipoDellaRiga{"nodo:foglia-voce": finito(1), "nodo:foglia": finito(2), "nodo:padre-voce": finito(3)}}
	voce := func() ancoraggio.CatenaCodice {
		return ancoraggio.CatenaCodice{Forma: &motorea.LetturaForma{Classe: &motorea.ClasseLetta{Segmento: "serie", Valore: "4019",
			Categoria: grammatica.CategoriaCommerciale, Descrizione: "Molla inventata", Fonte: "norma ACME 94019"}}}
	}
	st := ancoraggio.StrutturaProdotto{Target: "P", Stato: ancoraggio.StatoStrutturaCandidata, Radice: "nodo:radice",
		Nodi: []ancoraggio.NodoProposto{{Rif: "nodo:radice"}, {Rif: "nodo:foglia-voce", Codice: voce()}, {Rif: "nodo:foglia"},
			{Rif: "nodo:padre-voce", Codice: voce()}, {Rif: "nodo:figlio"}},
		Archi: []ancoraggio.ArcoProposto{{Padre: "nodo:radice", Figlio: "nodo:foglia-voce"}, {Padre: "nodo:radice", Figlio: "nodo:foglia"},
			{Padre: "nodo:radice", Figlio: "nodo:padre-voce"}, {Padre: "nodo:padre-voce", Figlio: "nodo:figlio"}}}
	if tipo, origine := b.tipoProposto("nodo:foglia-voce", "", false, st.Nodi[1].Codice.Forma.Classe); tipo != tipoCommerciale || origine != OrigineTipoPropostoDallaGrammatica {
		t.Fatalf("la foglia con la voce: %s/%s", tipo, origine)
	}
	var percorsi []string
	for _, d := range b.diagnosticheDelFinitoSenzaAutorita(ProdottoValutato{Rif: "P"}, []ancoraggio.StrutturaProdotto{st}) {
		percorsi = append(percorsi, d.Codice+" "+d.Percorso)
	}
	attesi := []string{CodiceBOMTipoFinitoFuoriTarget + " prodotti[P].nodi[nodo:foglia].tipo", CodiceBOMTipoFinitoFuoriTarget + " prodotti[P].nodi[nodo:padre-voce].tipo"}
	if len(percorsi) != len(attesi) || percorsi[0] != attesi[0] || percorsi[1] != attesi[1] {
		t.Fatalf("diagnostiche %q, attese %q", percorsi, attesi)
	}
}
