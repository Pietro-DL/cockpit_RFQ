package distintabozza

// L1 — V1, ondata B, corsia persistenza, secondo passaggio: il modo dei nodi contro la voce del registro (EM §3.4.2, §6.1;
// PM23 R-03; rilievo 6 della revisione), senza database: le regole P-2...P-6 (modoAtteso) e la verifica di un nodo nuovo
// o cambiato (modoDelNodo), su un registro sintetico (registroProva3) con una decisione_esistente aggiunta a mano.

import (
	"testing"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

func TestV1Rev2IlModoAttesoDalleRegoleDiPartenza(t *testing.T) {
	id := idSintetici3()
	c := registroProva3(t, id)
	comp := idDi("k-p1-1")
	voce := func(k string) int {
		for i, v := range c.Proposta.Nodi {
			if v.Voce == id.voce(k) {
				return i
			}
		}
		t.Fatalf("manca la voce %s", k)
		return -1
	}
	// #7 ha il codice proposto; con una decisione_esistente di un componente diventa registrato (P-3)
	if m := modoAtteso(&c, c.Proposta.Nodi[voce("#7")]); m != ModoProposta {
		t.Errorf("#7 con il codice proposto: %s", m)
	}
	conDecisione := c
	conDecisione.Proposta.Nodi = append([]importazione.Voce(nil), c.Proposta.Nodi...)
	conDecisione.Proposta.Nodi[voce("#7")].DecisioneEsistente = rawDi(t, map[string]any{"componente_id": comp.String(), "codice": "ACM-1101"})
	if m := modoAtteso(&conDecisione, conDecisione.Proposta.Nodi[voce("#7")]); m != ModoRegistrato {
		t.Errorf("#7 con una decisione_esistente: %s", m)
	}
	// il raggruppamento di P-3: la decisione_esistente e' il prodotto, la voce non e' la radice → P-4 (da_decidere, #11
	// non ha il codice proposto)
	conDecisione.Proposta.Nodi[voce("#11")].DecisioneEsistente = rawDi(t, map[string]any{"componente_id": id.prodotto.String(), "codice": "ACM-1000"})
	if m := modoAtteso(&conDecisione, conDecisione.Proposta.Nodi[voce("#11")]); m != ModoDaDecidere {
		t.Errorf("#11 raggruppamento: %s", m)
	}
	// la radice (P-2) e una voce senza codice proposto (P-4)
	if m := modoAtteso(&c, c.Proposta.Nodi[voce("#1")]); m != ModoRegistrato {
		t.Errorf("radice: %s", m)
	}
	if m := modoAtteso(&c, c.Proposta.Nodi[voce("#9")]); m != ModoDaDecidere {
		t.Errorf("#9: %s", m)
	}

	nodo := func(k, modo string) Nodo3 {
		n := Nodo3{ChiaveBozza: "k-" + k, Op: OpCrea, Tipo: "sciolto", Origine: OrigineStep,
			Caselle:  []Casella3{{AllegatoID: id.allegato, Gruppo: gruppoProva3, Rif: id.rif(k)}},
			Identita: Identita{Emittente: id.clienteE(), EmittentePresunto: true, Modo: modo}}
		if modo == ModoProposta {
			n.Identita = Identita{Emittente: id.clienteE(), Namespace: "acme", Codice: "ACM-1100", Rev: "A", Modo: modo}
		}
		return n
	}
	vf := &verifica3{}
	// da_decidere su una voce con il codice proposto: 422 anche se il nodo non e' nuovo (cambiato)
	if e := vf.modoDelNodo("snapshot.nodi[1]", nodo("#7", ModoDaDecidere), &c, false); e == nil || e.Motivo != MotivoPropostaNonCorrispondente ||
		e.Campo != "snapshot.nodi[1].identita.modo" {
		t.Errorf("da_decidere su #7: %+v", e)
	}
	if e := vf.modoDelNodo("snapshot.nodi[2]", nodo("#9", ModoDaDecidere), &c, true); e != nil {
		t.Errorf("da_decidere su #9: %+v", e)
	}
	// importato a mano: proposta su una voce con decisione_esistente → registrato atteso
	if e := vf.modoDelNodo("snapshot.nodi[1]", nodo("#7", ModoProposta), &conDecisione, true); e == nil || e.Motivo != MotivoPropostaNonCorrispondente {
		t.Errorf("proposta su #7 con decisione_esistente: %+v", e)
	}
	// lo stesso nodo gia' salvato (non aggiunto in questo salvataggio) non si rivede
	if e := vf.modoDelNodo("snapshot.nodi[1]", nodo("#7", ModoProposta), &conDecisione, false); e != nil {
		t.Errorf("proposta gia' salvata: %+v", e)
	}
	// un nodo in modo decisione e un nodo senza casella non si guardano
	dec := nodo("#7", ModoDecisione)
	dec.Identita.Decisione = "di-x"
	if e := vf.modoDelNodo("snapshot.nodi[1]", dec, &conDecisione, true); e != nil {
		t.Errorf("modo decisione: %+v", e)
	}
	senza := nodo("#7", ModoManuale)
	senza.Caselle = nil
	if e := vf.modoDelNodo("snapshot.nodi[1]", senza, &conDecisione, true); e != nil {
		t.Errorf("senza casella: %+v", e)
	}
}
