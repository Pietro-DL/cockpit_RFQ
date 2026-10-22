package propostabom

// L1 — messaggio 46, collaudo UI46, rilievo B2 [T]: subito dopo «Conferma BOM» la proposta di adesso dice con
// decisione_esistente (e il tipo e l'abbinamento che ne seguono) i componenti che la conferma ha appena creato. Non è una
// proposta aggiornata: SenzaGliEffettiDellaConferma toglie quelle differenze, e senza altre differenze il segno non c'è.
// Una differenza vera resta, anche sulla stessa voce. Gli ID sono inventati.

import (
	"encoding/json"
	"testing"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

func TestUI46SenzaGliEffettiDellaConferma(t *testing.T) {
	const cNuovo, cAltro = "6f1b5c2a-0000-4000-8000-000000000a01", "6f1b5c2a-0000-4000-8000-000000000a02"
	voce := func(v, comp string) importazione.Voce {
		x := importazione.Voce{Voce: v}
		if comp != "" {
			x.DecisioneEsistente = json.RawMessage(`{"componente_id":"` + comp + `"}`)
		}
		return x
	}
	adesso := importazione.Contenuto{}
	adesso.Proposta.Nodi = []importazione.Voce{voce("n:a:#110", cNuovo), voce("n:a:#120", cNuovo), voce("n:a:#130", cAltro), voce("n:a:#140", "")}
	mappa := ComponentiDellaMappa([]byte(`{"k-110":"` + cNuovo + `","k-root":"6F1B5C2A-0000-4000-8000-000000000A09"}`))
	if !mappa[cNuovo] || !mappa["6f1b5c2a-0000-4000-8000-000000000a09"] || len(mappa) != 2 {
		t.Fatalf("i componenti della mappa: %v", mappa)
	}
	pa := func(nodi ...DifferenzaNodo) *PropostaAggiornata {
		d := Differenze{Nodi: nodi}
		return &PropostaAggiornata{Gruppo: "g", ImprontaContenuto: "x", Differenze: d, PerTipo: ClassiDelleDifferenze(d)}
	}
	cambiato := func(v string, campi ...string) DifferenzaNodo {
		return DifferenzaNodo{Voce: v, Tipo: DifferenzaCambiato, Campi: campi}
	}

	// il caso del collaudo: tre voci cambiate solo in decisione_esistente e tipo_proposto verso i componenti della conferma
	if got := SenzaGliEffettiDellaConferma(pa(cambiato("n:a:#110", "decisione_esistente", "tipo_proposto"), cambiato("n:a:#120", "decisione_esistente", "abbinamento_proposto")), adesso, mappa); got != nil {
		t.Errorf("la proposta aggiornata dopo la conferma: %+v, attesa nessuna", got)
	}
	// una differenza vera sulla stessa voce resta, senza i campi della conferma
	got := SenzaGliEffettiDellaConferma(pa(cambiato("n:a:#110", "decisione_esistente", "rev_proposta")), adesso, mappa)
	if got == nil || len(got.Differenze.Nodi) != 1 || len(got.Differenze.Nodi[0].Campi) != 1 || got.Differenze.Nodi[0].Campi[0] != "rev_proposta" || got.PerTipo.Revisione != 1 || got.PerTipo.Identita != 0 {
		t.Errorf("la revisione cambiata sulla voce confermata: %+v", got)
	}
	// un componente che la conferma non ha toccato, una voce senza decisione_esistente, un nodo aggiunto: restano
	for _, d := range []DifferenzaNodo{cambiato("n:a:#130", "decisione_esistente"), cambiato("n:a:#140", "tipo_proposto"), {Voce: "n:a:#110", Tipo: DifferenzaAggiunto}} {
		if got := SenzaGliEffettiDellaConferma(pa(d), adesso, mappa); got == nil || len(got.Differenze.Nodi) != 1 {
			t.Errorf("la differenza %+v si è persa: %+v", d, got)
		}
	}
	// l'abbinamento proposto ai soli componenti della conferma (abbinamento_per_base sui pezzi appena creati)
	ab := adesso
	ab.Proposta.Nodi = append([]importazione.Voce{{Voce: "n:a:#150", AbbinamentoProposto: json.RawMessage(`{"componenti":["` + cNuovo + `"],"motivo":"abbinamento_per_base"}`)},
		{Voce: "n:a:#160", AbbinamentoProposto: json.RawMessage(`{"componenti":["` + cNuovo + `","` + cAltro + `"]}`)}}, adesso.Proposta.Nodi...)
	if got := SenzaGliEffettiDellaConferma(pa(cambiato("n:a:#150", "abbinamento_proposto")), ab, mappa); got != nil {
		t.Errorf("l'abbinamento ai componenti della conferma: %+v, atteso nessun segno", got)
	}
	if got := SenzaGliEffettiDellaConferma(pa(cambiato("n:a:#160", "abbinamento_proposto")), ab, mappa); got == nil {
		t.Errorf("l'abbinamento anche a un componente che la conferma non ha toccato si è perso")
	}
	// senza mappa niente cambia
	p := pa(cambiato("n:a:#110", "decisione_esistente"))
	if got := SenzaGliEffettiDellaConferma(p, adesso, nil); got != p {
		t.Errorf("senza la mappa della conferma la proposta aggiornata cambia: %+v", got)
	}
	// un arco cambiato resta anche quando i nodi si tolgono tutti
	q := pa(cambiato("n:a:#110", "decisione_esistente"))
	q.Differenze.Archi = []DifferenzaArco{{AllegatoID: "a", Padre: "#100", Figlio: "#110", Tipo: DifferenzaCambiato, Campi: []string{"quantita"}}}
	if got := SenzaGliEffettiDellaConferma(q, adesso, mappa); got == nil || len(got.Differenze.Nodi) != 0 || len(got.Differenze.Archi) != 1 || got.PerTipo.Quantita != 1 {
		t.Errorf("l'arco cambiato: %+v", got)
	}
}
