// L1 — la struttura del prodotto nell'esito di Calcola (A1c-L1-20, la parte di valutazione; contratto §7: la prova
// nominava RadiceScenario, che è diventata StrutturaProdotto, T-B0-02): la struttura del prodotto target sotto la radice
// dello STEP, prodotta da ancoraggio, arriva intera nell'esito del thread, con i figli senza file e i nodi senza lettura.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/valutazione"
)

// TestL120LaStrutturaDelProdottoNellEsito (A1c-L1-20; T-B0-02; F0-04): la BOM di lavoro del finito, sotto la radice
// scelta con il gesto 3: la radice dello STEP non è senza file; il figlio 7120200A non ha un file, e lo dice; il figlio
// con un id che la grammatica non legge è senza lettura. L'esito porta gli ancoraggi di ValutaProdotti, con le loro
// diagnostiche e la loro impronta.
func TestL120LaStrutturaDelProdottoNellEsito(t *testing.T) {
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "ACME-VITE-M6", ""}}, []arcoF{{"#1", "#2", 1}, {"#1", "#3", 4}})
	f := fotografiaDi(th)
	r := insiemeACME(t)
	e := calcola(t, f, r, valutazione.Ingressi{})
	et := esitoThread(t, e, threadACME)
	v, err := valutazione.ValutaProdotti(f, th, r.Motori[clienteACME], nil)
	if err != nil {
		t.Fatal(err)
	}
	if canonicoDi(t, et.Ancoraggi) != canonicoDi(t, v.Ancoraggi) || et.Ancoraggi.Impronta == "" {
		t.Fatal("l'esito non porta gli ancoraggi di ValutaProdotti")
	}
	var s *ancoraggio.StrutturaProdotto
	for i := range et.Ancoraggi.Strutture {
		if x := &et.Ancoraggi.Strutture[i]; x.Target == rifProdB && x.Stato == ancoraggio.StatoBOMDiLavoroProposta {
			s = x
		}
	}
	if s == nil || s.Radice != nodoB("#1") {
		t.Fatalf("nessuna BOM di lavoro del finito sotto la radice #1 fra %d strutture", len(et.Ancoraggi.Strutture))
	}
	for _, c := range []struct {
		chiave               string
		senzaFile, leggibile bool
	}{{"#1", false, true}, {"#2", true, true}, {"#3", true, false}} {
		n := nodoIn(t, *s, nodoB(c.chiave))
		if n.SenzaFile != c.senzaFile || n.Leggibile != c.leggibile {
			t.Errorf("nodo %s: senza file %v (atteso %v), leggibile %v (atteso %v)", c.chiave, n.SenzaFile, c.senzaFile, n.Leggibile, c.leggibile)
		}
	}
	if len(s.Archi) != 2 {
		t.Errorf("archi della struttura %d", len(s.Archi))
	}
}
