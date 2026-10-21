package distintabozza

// L1 — direttiva 39 §5 [U] (rilievo della revisione del 20/10; pbiniziali3.go, scelta [T-39] della corsia persistenza):
// lo STEP candidato come sorgente di un finito non diventa una PB iniziale del motore ne' un suggerimento. Candidato: uno
// STEP proposto dal registro sul nodo del prodotto, o uno STEP che una proposta di documento dice cad_3d del finito
// (StepDelFinito). Lo STEP di un particolare su un nodo non finito resta una PB come un PDF (D-2 (c) [U]); un PDF sulla
// radice resta una PB. Dati sintetici.

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

func TestV1D39LoStepDelFinitoNonEUnaPBIniziale(t *testing.T) {
	id := idSintetici3()
	in, _ := ingressoStepProva(id)
	k7 := ChiaveDellaVoce(id.allegato, id.rif("#7"))
	root := ChiaveDelComponente(id.prodotto)
	conEstensione := func(ext string) FattiPB {
		f := fattiPreassociabili(id.pdf)
		x := f.Allegati[id.pdf]
		x.Estensione = ext
		f.Allegati[id.pdf] = x
		return f
	}
	sulla := func(rif string) (Snapshot3, importazione.Registro) {
		reg := registroConProposte(t, id, map[string][]uuid.UUID{rif: {id.pdf}})
		s, _, err := VersioneDaRegistro(reg, in)
		if err != nil {
			t.Fatal(err)
		}
		return s, reg
	}

	// lo STEP proposto sulla radice: ne' PB ne' suggerimento
	s, r := sulla(id.rif("#1"))
	if pb, pp := ClassificaProposte(s, &r, conEstensione("stp")); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("STEP sulla radice: %+v %+v", pb, pp)
	}
	if pb, pp := ClassificaProposte(s, &r, conEstensione("STEP")); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("STEP (maiuscole) sulla radice: %+v %+v", pb, pp)
	}
	// nemmeno come suggerimento quando la regola darebbe pb_toccata
	sp := s.copia()
	sp.DocumentiPreassociati = []Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaManuale}}
	if pb, pp := ClassificaProposte(sp, &r, conEstensione("stp")); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("STEP sulla radice, gia' PB altrove: %+v %+v", pb, pp)
	}
	// un PDF sulla radice resta una PB
	if pb, _ := ClassificaProposte(s, &r, conEstensione("pdf")); len(pb) != 1 || pb[0].ChiaveBozza != root {
		t.Fatalf("PDF sulla radice: %+v", pb)
	}

	// lo STEP di un particolare (nodo non finito): PB come un PDF
	s7, r7 := sulla(id.rif("#7"))
	if pb, _ := ClassificaProposte(s7, &r7, conEstensione("stp")); len(pb) != 1 || pb[0].ChiaveBozza != k7 || pb[0].Provenienza != ProvenienzaMotore {
		t.Fatalf("STEP documentale del particolare: %+v", pb)
	}
	// lo stesso STEP che una proposta di documento dice cad_3d del finito: candidato di S8, ne' PB ne' suggerimento
	f := conEstensione("stp")
	f.StepDelFinito = map[uuid.UUID]bool{id.pdf: true}
	if pb, pp := ClassificaProposte(s7, &r7, f); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("STEP candidato del finito su un particolare: %+v %+v", pb, pp)
	}
}
