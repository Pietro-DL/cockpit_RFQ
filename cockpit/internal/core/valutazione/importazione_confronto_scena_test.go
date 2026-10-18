// L1 — il confronto per le bozze senza registro e fra registri con radici diverse (V1: emendamento di V1 §6.5, §9.1
// «Senza registro ... le caselle assenti, da scegliere nel riallineamento, e le voci nuove»; §6.4.1, §6.7, §6.12; P-2),
// sui registri veri dell'adattatore. Nate dai rilievi della revisione della corsia motore (ondata B): ConfrontaCaselle
// non aveva prove, e la radice del gruppo entrava nel confronto di ogni voce. Tutto è inventato, come nelle scene.
package valutazione_test

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// vociNonRadice: le voci dei nodi del registro, senza quella della radice e senza quelle date.
func vociNonRadice(k importazione.Contenuto, senza ...string) []string {
	via := map[string]bool{}
	for _, s := range senza {
		via[s] = true
	}
	var out []string
	for _, v := range k.Proposta.Nodi {
		if !k.EVoceDellaRadice(v) && !via[v.Voce] {
			out = append(out, v.Voce)
		}
	}
	return out
}

// TestConfrontaCaselle (EM §9.1, la bozza senza registro: da-conferma-1 e la conversione dal formato 2; §6.5 per
// nuovi_nodi_proposti):
//   - la bozza convertita, con il nodo del prodotto senza caselle e una casella per ogni altra voce: nessuna differenza, la
//     radice la rappresenta il nodo del prodotto (P-2) e non è mai «aggiunta»;
//   - lo stesso con la casella della radice sul nodo del prodotto (da-conferma-1 da una conferma formato 3);
//   - una casella su una voce che il registro non ha: «tolto»; una voce senza casella: «aggiunto»;
//   - una casella su una voce fuori per decisione non è «tolta», e una voce fuori per decisione senza casella non è
//     «aggiunta».
func TestConfrontaCaselle(t *testing.T) {
	k := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	radice := vocePerProva(aStepB, nodoB("#1"))
	if d := propostabom.ConfrontaCaselle(vociNonRadice(k), k); !d.Vuote() {
		t.Errorf("la bozza convertita con tutte le caselle: %+v", d)
	}
	if d := propostabom.ConfrontaCaselle(append(vociNonRadice(k), radice), k); !d.Vuote() {
		t.Errorf("con la casella della radice sul nodo del prodotto: %+v", d)
	}
	sparita := vocePerProva(aStepB, nodoB("#9"))
	v4 := vocePerProva(aStepB, nodoB("#4"))
	d := propostabom.ConfrontaCaselle(append(vociNonRadice(k, v4), sparita), k)
	perVoce := map[string]string{}
	for _, x := range d.Nodi {
		perVoce[x.Voce] = x.Tipo
	}
	if len(perVoce) != 2 || perVoce[sparita] != propostabom.DifferenzaTolto || perVoce[v4] != propostabom.DifferenzaAggiunto {
		t.Errorf("una casella sparita e una voce senza casella: %+v", d)
	}
	if len(d.Archi) != 0 || len(d.Gruppo) != 0 {
		t.Errorf("ConfrontaCaselle dà solo nodi: %+v", d)
	}

	// le voci fuori per decisione (la scena di A-11: #5 tolto, #3 e #4 non raggiunti)
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}, {"#5", "7120500A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#3", "#4", 3}, {"#1", "#5", 1}}
	th := scenaBOM(t, nodi, archi)
	op := operatore
	decidi(t, &th, "#5", "scartata", uuid.Nil, &op)
	a := arcoDi(t, &th, "#1", "#3")
	a.Stato, a.DecisoDa = "scartata", &op
	f := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if len(f.Proposta.FuoriPerDecisione) != 3 {
		t.Fatalf("fuori per decisione: %+v", f.Proposta.FuoriPerDecisione)
	}
	conFuori := append(vociNonRadice(f), vocePerProva(aStepB, nodoB("#5")))
	if d := propostabom.ConfrontaCaselle(conFuori, f); !d.Vuote() {
		t.Errorf("una casella su una voce fuori per decisione: %+v", d)
	}
	if d := propostabom.ConfrontaCaselle(vociNonRadice(f), f); !d.Vuote() {
		t.Errorf("le voci fuori per decisione senza casella: %+v", d)
	}
}

// TestConfrontaConRadiciDiverse (EM §6.5, «Su che cosa si confronta un nodo»): la radice del gruppo serve solo a
// classificare la voce della radice. Due registri dello stesso file che differiscono solo per gruppo.radice danno la
// differenza «radice» del gruppo e nessuna voce «cambiata»: le voci hanno gli stessi valori, anche le due che cambiano
// ruolo (#1 e #2), che si confrontano sui valori. Con un valore cambiato nella voce che cambia ruolo, solo quel campo.
func TestConfrontaConRadiciDiverse(t *testing.T) {
	a := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	b := a
	b.Gruppo.Radice = nodoB("#2")
	x := propostabom.Confronta(a, b)
	if !contieneTesto(x.Differenze.Gruppo, "radice") || x.Esito != propostabom.EsitoPropostaCambiata {
		t.Errorf("la radice cambiata non sta fra le differenze del gruppo: %s %+v", x.Esito, x.Differenze)
	}
	if len(x.Differenze.Nodi) != 0 {
		t.Errorf("con la sola radice diversa una voce risulta cambiata: %+v", x.Differenze.Nodi)
	}
	// la voce di #2, radice in b, con un valore cambiato: solo quel campo, sui valori
	c := b
	c.Proposta.Nodi = append([]importazione.Voce(nil), b.Proposta.Nodi...)
	for i := range c.Proposta.Nodi {
		if c.Proposta.Nodi[i].Origine.Rif == nodoB("#2") {
			c.Proposta.Nodi[i].CodiceProposto = "7120299A"
		}
	}
	y := propostabom.Confronta(a, c)
	if len(y.Differenze.Nodi) != 1 || y.Differenze.Nodi[0].Voce != vocePerProva(aStepB, nodoB("#2")) || len(y.Differenze.Nodi[0].Campi) != 1 ||
		y.Differenze.Nodi[0].Campi[0] != "codice_proposto" {
		t.Errorf("la voce che cambia ruolo con un valore cambiato: %+v", y.Differenze.Nodi)
	}
}
