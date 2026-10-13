package fascicolo

// L1 — A1d P4, B4.10: la formazione grezza «ANY» di un PRODUCT non diventa la rev proposta nel legacy, come gia'
// nel Motore A, che rev_grezza non la prende mai come revisione. Lo shock test del 12/10 (SABC §B.2) l'ha vista su
// un file vero: la rev proposta era «ANY». Qui i codici sono della famiglia di prova (motoreConFamiglia).

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// Con la famiglia che separa la rev dal codice, «ANY» lascia la rev della famiglia; l'evidenza tiene il grezzo.
func TestRevGrezzaANYNonDiventaLaRevProposta(t *testing.T) {
	m := motoreConFamiglia(t)
	for _, grezza := range []string{"ANY", "any", " Any "} {
		s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#1", "", "77722757_B", "", grezza)}}
		n := ClassificaNodi(m, s)[0]
		if n.Codice != "77722757" || n.Rev != "B" {
			t.Errorf("%q: codice %q rev %q, attesi 77722757 e B (la rev della famiglia)", grezza, n.Codice, n.Rev)
		}
		if n.Evidenza["rev_grezza_segnaposto"] != strings.TrimSpace(grezza) {
			t.Errorf("%q: l'evidenza deve tenere il grezzo: %v", grezza, n.Evidenza["rev_grezza_segnaposto"])
		}
		if _, scartata := n.Evidenza["rev_scartata"]; scartata {
			t.Errorf("%q: «ANY» non e' una rev scartata per misura, e' un segnaposto: %v", grezza, n.Evidenza)
		}
	}
}

// Senza una rev della famiglia «ANY» lascia la rev vuota: il pezzo non ha una revisione letta, non ne ha una falsa.
func TestRevGrezzaANYSenzaFamigliaLasciaLaRevVuota(t *testing.T) {
	s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#1", "1234567A", "1234567A", "", "ANY")}}
	n := ClassificaNodi(classificazione.Compila("Senza regole", regole.Regole{}), s)[0]
	if n.Codice == "" || n.Rev != "" {
		t.Fatalf("atteso un codice generico senza rev: %+v", n)
	}
	// e la radice dello STEP, evidenza del codice del documento, non porta «ANY» come rev
	r1 := worker.StrutturaSTEP{Versione: 3, Radici: []string{"#1"}, Nodi: []worker.NodoSTEP{nodo("#1", "", "77722757_B", "", "ANY")}}
	if r, ok := RadiceDelloStep(ClassificaNodi(motoreConFamiglia(t), r1), r1); !ok || r.Rev != "B" {
		t.Errorf("radice con «ANY»: %+v %v, attesa la rev B della famiglia", r, ok)
	}
}

// Le altre formazioni restano revisioni come prima: solo «ANY» e' un segnaposto, e «ANYA» o «A» no.
func TestLeAltreRevGrezzeRestanoRevisioni(t *testing.T) {
	m := motoreConFamiglia(t)
	for grezza, attesa := range map[string]string{"C": "C", "ANYA": "ANYA", "": "B", "  ": "B"} {
		s := worker.StrutturaSTEP{Versione: 3, Nodi: []worker.NodoSTEP{nodo("#1", "", "77722757_B", "", grezza)}}
		n := ClassificaNodi(m, s)[0]
		if n.Rev != attesa {
			t.Errorf("%q: rev %q, attesa %q", grezza, n.Rev, attesa)
		}
		if _, c := n.Evidenza["rev_grezza_segnaposto"]; c {
			t.Errorf("%q: non e' un segnaposto: %v", grezza, n.Evidenza)
		}
	}
	if RevGrezzaSegnaposto("") || RevGrezzaSegnaposto("A") || !RevGrezzaSegnaposto("aNy") {
		t.Error("RevGrezzaSegnaposto: solo «ANY», in qualunque grafia")
	}
}
