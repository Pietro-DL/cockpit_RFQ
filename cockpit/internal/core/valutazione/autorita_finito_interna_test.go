// L1 — la regola dell'autorità di «finito» proposto (D-P7e-1 A, D-CHIUS-2 C, D-CHIUS-4 B, decisioni dell'utente
// dell'11/10), provata sulla funzione: con Calcola la radice scelta nella BOM di lavoro del suo target ha sempre il
// componente del target (T-B4-38), quindi il ramo che dà l'autorità non si raggiunge da fuori. «finito» vale solo per la
// radice scelta di una BOM di lavoro, nella BOM di lavoro del suo target; gli altri valori dell'enum valgono sempre.
package valutazione

import "testing"

func TestDP7e1LAutoritaDelFinitoNellaSuaBOM(t *testing.T) {
	b := &bomDelThread{radici: map[string]radiceDiTarget{}, radiciDelTarget: map[string]map[string]bool{}}
	b.radici["nodo:r"] = radiceDellaBOMDiLavoro
	b.radiciDelTarget["nodo:r"] = map[string]bool{"componente:B": true}
	b.radici["nodo:c"] = radiceCandidata
	for _, c := range []struct {
		rif, tipo, bom string
		atteso         bool
	}{
		{"nodo:r", tipoFinito, "componente:B", true},
		{"nodo:r", tipoFinito, "componente:A", false},
		{"nodo:r", tipoFinito, "", false},
		{"nodo:c", tipoFinito, "componente:B", false},
		{"nodo:x", tipoFinito, "componente:B", false},
		{"nodo:x", tipoCommerciale, "", true},
		{"nodo:x", tipoSottoassieme, "componente:A", true},
	} {
		if got := b.propostaConAutorita(c.rif, c.tipo, c.bom); got != c.atteso {
			t.Errorf("%s %s nella BOM di %q: %v, atteso %v", c.rif, c.tipo, c.bom, got, c.atteso)
		}
	}
}
