package distintabozza

// Le prove L1 di cambio-fonte-1 (cambia_sorgente; direttiva 35, blocco C-3; EM §6.6; MAT §4.3 riga 12; base B di V1-Q30):
// la stessa sorgente rifiutata; verso un'altra copia dello stesso STEP nessun nodo esce o entra, le caselle e gli archi step
// passano sulla copia nuova; verso un altro STEP i nodi della fonte vecchia escono (le PB nel vassoio con fonte_cambiata,
// le decisioni che li citano cadono) e le voci nuove entrano come nella partenza step; fra due copie un nodo proposto con
// l'identita' cambiata vuole recepisci o mantieni. Gli aiuti sono in c3_riallinea_test.go.

import (
	"strings"
	"testing"
)

func TestV1C3IlCambioDellaFonte(t *testing.T) {
	id := idSintetici3()
	in, _ := ingressoStepProva(id)
	vecchio := componiProva3(t, id)
	prima := versioneProva(t, id, vecchio)
	k7, k31 := ChiaveDellaVoce(id.allegato, id.rif("#7")), ChiaveDellaVoce(id.allegato, id.rif("#31"))
	prima.Decisioni = []Decisione3{{ID: "dv-1", Tipo: DecisioneDaValutare, ChiaveBozza: k31}}
	// la stessa struttura sulla stessa occorrenza: stessa_sorgente
	_, err := Cambia(prima, &vecchio, vecchio, nil, "x", in)
	if e := errore3(t, err); e.Motivo != MotivoStessaSorgente {
		t.Fatalf("stessa sorgente: %+v", e)
	}
	// un'altra copia dello stesso STEP: nessun nodo esce o entra, le caselle e gli archi passano sulla copia
	copia := idDi("c3-copia")
	cc := spostaRegistro(t, registroProva3(t, id), id.allegato, copia, "", "")
	nuovo := componi(t, cc)
	es, err := Cambia(prima, &vecchio, nuovo, nil, "la BOM di lavoro sta sulla copia", in)
	if err != nil {
		t.Fatal(err)
	}
	s := es.Snapshot
	if *s.Sorgente.AllegatoID != copia || s.Sorgente.Importazione != nuovo.Impronta || len(s.Nodi) != len(prima.Nodi) || len(es.Effetto.FuoriPerCambioFonte) != 0 ||
		len(es.Entrati) != 0 || len(s.DocumentiPreassociati) != 1 {
		t.Fatalf("copia: %+v entrati %v effetto %+v", s.Sorgente, es.Entrati, es.Effetto)
	}
	for _, n := range s.Nodi {
		for _, x := range n.Caselle {
			if x.AllegatoID != copia {
				t.Fatalf("casella non riscritta: %+v", x)
			}
		}
	}
	if len(es.Effetto.CaselleRiscritte) == 0 || len(es.Effetto.ArchiRiscritti) != 4 {
		t.Fatalf("elenchi: %+v", es.Effetto)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
	// un altro STEP: i nodi della fonte vecchia escono (la PB di #31 nel vassoio con fonte_cambiata, la decisione cade), le
	// voci nuove entrano come nella partenza step
	altro := idDi("c3-altro")
	ca := spostaRegistro(t, registroProva3(t, id), id.allegato, altro, id.sha, shaProva('e'))
	nuovo = componi(t, ca)
	es, err = Cambia(prima, &vecchio, nuovo, nil, "revisione CAD", in)
	if err != nil {
		t.Fatal(err)
	}
	s = es.Snapshot
	if _, ok := nodoC3(s, k7); ok {
		t.Fatal("il nodo della fonte vecchia e' rimasto")
	}
	if n, ok := nodoC3(s, ChiaveDellaVoce(altro, "nodo:"+shaProva('e')+":#7")); !ok || n.Identita.Modo != ModoProposta || n.Identita.Codice != "ACM-1100" {
		t.Fatalf("la voce nuova non e' entrata: %+v", n)
	}
	if len(s.DocumentiPreassociati) != 0 || len(es.Vassoio) != 1 || len(s.Decisioni) != 0 || len(es.Effetto.DecisioniCadute) != 1 {
		t.Fatalf("vassoio e decisioni: %+v %v %+v", s.DocumentiPreassociati, es.Vassoio, s.Decisioni)
	}
	for _, e := range es.eventi {
		if e.Azione == AzFileSvincolato && !strings.Contains(string(e.Dopo), `"motivo":"fonte_cambiata"`) {
			t.Fatalf("file_svincolato: %s", e.Dopo)
		}
	}
	root, _ := nodoC3(s, ChiaveDelComponente(id.prodotto))
	if len(root.Caselle) != 1 || root.Caselle[0].AllegatoID != altro || root.Caselle[0].Rif != "nodo:"+shaProva('e')+":#1" {
		t.Fatalf("la radice: %+v", root.Caselle)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
	// fra due copie un nodo proposto con l'identita' cambiata vuole recepisci o mantieni (base B di V1-Q30)
	cd := spostaRegistro(t, registroProva3(t, id), id.allegato, copia, "", "")
	cd.Proposta.Nodi[1].CodiceProposto = "ACM-1199"
	_, err = Cambia(prima, &vecchio, componi(t, cd), nil, "copia", in)
	if e := errore3(t, err); e.Motivo != MotivoRiallineamentoIncompleto || !strings.Contains(e.Messaggio, k7) {
		t.Fatalf("scelta fra due copie: %+v", e)
	}
	es, err = Cambia(prima, &vecchio, componi(t, cd), []SceltaR2{{ChiaveBozza: k7, Scelta: SceltaRecepisci}}, "copia", in)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := nodoC3(es.Snapshot, k7); n.Identita.Codice != "ACM-1199" || len(es.Aggiornate) != 1 {
		t.Fatalf("recepisci fra due copie: %+v", n.Identita)
	}
}
