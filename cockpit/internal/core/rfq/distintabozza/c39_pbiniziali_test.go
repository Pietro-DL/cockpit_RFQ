package distintabozza

// L1 — direttiva 39 §4 (D-PB01 A, D-PB02 A, D-AR11 B [U]; pbiniziali3.go): le PB iniziali del server dal registro
// d'importazione. Solo con la sorgente confermata da S8; le proposte univoche diventano PB motore con il registro
// d'origine; le ambigue no (ne' PB ne' suggerimento); i file gia' decisi altrove o toccati dall'ingegnere vanno fra i
// suggerimenti (preassociazioni_proposte[]) con il loro motivo, mai in da_rivedere[]; una PB gia' sullo stesso nodo resta
// com'e' (il suo registro d'origine non cambia).

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// registroConProposte: il registro di prova con le proposte d'associazione date, per rif della voce (l'allegato deve
// essere fra gli allegati citati: qui il PDF della prova).
func registroConProposte(t *testing.T, id idProva3, proposte map[string][]uuid.UUID) importazione.Registro {
	t.Helper()
	c := registroProva3(t, id)
	for i := range c.Proposta.Nodi {
		v := &c.Proposta.Nodi[i]
		v.AssociazioniProposte = nil
		var aa []any
		for _, a := range proposte[v.Origine.Rif] {
			aa = append(aa, map[string]any{"allegato_id": a.String(), "motivo": "nome", "livello": "alto"})
		}
		if len(aa) > 0 {
			v.AssociazioniProposte = rawDi(t, aa)
		}
	}
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func fattiPreassociabili(aa ...uuid.UUID) FattiPB {
	f := FattiPB{SorgenteValida: true, Allegati: map[uuid.UUID]allegatoPreassociabile{}, ARAttiva: map[uuid.UUID]uuid.UUID{}, ARChiusa: map[uuid.UUID]bool{},
		AltreBozze: map[uuid.UUID]bool{}, CopiaSenzaAR: map[uuid.UUID]uuid.UUID{}, Documento: map[uuid.UUID]uuid.UUID{}, Visti: map[uuid.UUID]bool{},
		Ricollocabili: map[uuid.UUID]bool{}}
	for _, a := range aa {
		f.Allegati[a] = allegatoPreassociabile{DellaRfq: true, Attivo: true, Preassociabile: true}
	}
	return f
}

func TestV1D39LePBInizialiDalRegistro(t *testing.T) {
	id := idSintetici3()
	in, _ := ingressoStepProva(id)
	k7, k9 := ChiaveDellaVoce(id.allegato, id.rif("#7")), ChiaveDellaVoce(id.allegato, id.rif("#9"))
	root := ChiaveDelComponente(id.prodotto)

	// una proposta univoca su #7 e una su #1 (la radice)
	reg := registroConProposte(t, id, map[string][]uuid.UUID{id.rif("#7"): {id.pdf}})
	s, _, err := VersioneDaRegistro(reg, in)
	if err != nil {
		t.Fatal(err)
	}
	// D-AR11 B: senza la sorgente confermata da S8 niente, ne' PB ne' suggerimenti
	f := fattiPreassociabili(id.pdf)
	f.SorgenteValida = false
	if pb, pp := ClassificaProposte(s, &reg, f); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("senza S8: %+v %+v", pb, pp)
	}
	// con S8: la PB motore con il registro d'origine
	pb, pp := ClassificaProposte(s, &reg, fattiPreassociabili(id.pdf))
	if len(pb) != 1 || len(pp) != 0 || pb[0] != (Preassociazione3{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaMotore, Importazione: reg.Impronta}) {
		t.Fatalf("univoca: %+v %+v", pb, pp)
	}
	// la PB entra nello snapshot e lo snapshot resta scrivibile
	s1 := s.copia()
	aggiungiPB(&s1, pb)
	if _, err := s1.Canonico(); err != nil {
		t.Fatalf("snapshot con la PB iniziale: %v", err)
	}

	// ambigua (due voci propongono lo stesso file): resta da associare, ne' PB ne' suggerimento
	amb := registroConProposte(t, id, map[string][]uuid.UUID{id.rif("#7"): {id.pdf}, id.rif("#9"): {id.pdf}})
	sa, _, err := VersioneDaRegistro(amb, in)
	if err != nil {
		t.Fatal(err)
	}
	if pb, pp := ClassificaProposte(sa, &amb, fattiPreassociabili(id.pdf)); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("ambigua: %+v %+v", pb, pp)
	}

	casi := []struct {
		nome   string
		prep   func(s *Snapshot3, f *FattiPB)
		pb     int
		motivo string
	}{
		{"gia' PB sullo stesso nodo: resta com'e' (registro d'origine invariato)", func(s *Snapshot3, f *FattiPB) {
			s.DocumentiPreassociati = []Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaMotore, Importazione: impA}}
		}, 0, ""},
		{"gia' PB su un altro nodo: pb_toccata", func(s *Snapshot3, f *FattiPB) {
			s.DocumentiPreassociati = []Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: root, Provenienza: ProvenienzaManuale}}
		}, 0, PropostaPBToccata},
		{"AR attiva altrove: ar_attiva", func(s *Snapshot3, f *FattiPB) { f.ARAttiva[id.pdf] = uuid.New() }, 0, PropostaARAttiva},
		{"AR chiusa (una persona ha deciso): pb_toccata", func(s *Snapshot3, f *FattiPB) { f.ARChiusa[id.pdf] = true }, 0, PropostaPBToccata},
		{"toccato dall'ingegnere nel giornale: pb_toccata", func(s *Snapshot3, f *FattiPB) { f.Visti[id.pdf] = true }, 0, PropostaPBToccata},
		{"PB motore svincolata dal comando stesso: torna PB", func(s *Snapshot3, f *FattiPB) { f.Visti[id.pdf], f.Ricollocabili[id.pdf] = true, true }, 1, ""},
		{"nella bozza di un altro prodotto: conflitto", func(s *Snapshot3, f *FattiPB) { f.AltreBozze[id.pdf] = true }, 0, PropostaConflitto},
		{"copia senza AR su un altro componente: conflitto", func(s *Snapshot3, f *FattiPB) { f.CopiaSenzaAR[id.pdf] = uuid.New() }, 0, PropostaConflitto},
		{"documento confermato di un altro componente: conflitto", func(s *Snapshot3, f *FattiPB) { f.Documento[id.pdf] = uuid.New() }, 0, PropostaConflitto},
		{"STEP legato come sorgente: niente", func(s *Snapshot3, f *FattiPB) {
			f.Allegati[id.pdf] = allegatoPreassociabile{DellaRfq: true, Attivo: true, Preassociabile: true, LegatoComeSorgente: true}
		}, 0, ""},
		{"file non attivo: niente", func(s *Snapshot3, f *FattiPB) {
			f.Allegati[id.pdf] = allegatoPreassociabile{DellaRfq: true, Preassociabile: true}
		}, 0, ""},
		{"nodo escluso: niente", func(s *Snapshot3, f *FattiPB) {
			for i := range s.Nodi {
				if s.Nodi[i].ChiaveBozza == k7 {
					s.Nodi[i].Op = OpEsclude
				}
			}
		}, 0, ""},
	}
	for _, c := range casi {
		sc, f := s.copia(), fattiPreassociabili(id.pdf)
		c.prep(&sc, &f)
		pb, pp := ClassificaProposte(sc, &reg, f)
		if len(pb) != c.pb {
			t.Errorf("%s: PB %+v", c.nome, pb)
		}
		switch {
		case c.motivo == "" && len(pp) != 0:
			t.Errorf("%s: suggerimenti inattesi %+v", c.nome, pp)
		case c.motivo != "" && (len(pp) != 1 || pp[0].Motivo != c.motivo || pp[0].ChiaveBozza != k7 || pp[0].Provenienza != ProvenienzaMotore ||
			pp[0].Importazione != reg.Impronta || pp[0].AllegatoID != id.pdf):
			t.Errorf("%s: suggerimenti %+v", c.nome, pp)
		}
	}
	// la PB su un nodo senza la voce (fuori dalla bozza): niente
	fuori := registroConProposte(t, id, map[string][]uuid.UUID{id.rif("#9"): {id.pdf}})
	sf := s.copia()
	var nodi []Nodo3
	for _, n := range sf.Nodi {
		if n.ChiaveBozza != k9 {
			nodi = append(nodi, n)
		}
	}
	sf.Nodi = nodi
	if pb, pp := ClassificaProposte(sf, &fuori, fattiPreassociabili(id.pdf)); len(pb) != 0 || len(pp) != 0 {
		t.Fatalf("voce senza nodo: %+v %+v", pb, pp)
	}
	// i suggerimenti non sono da_rivedere: la forma ha il proprio campo, e un JSON della lista non ha chiavi di da_rivedere
	b, _ := json.Marshal(PreassociazioneProposta3{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaMotore, Importazione: reg.Impronta, Motivo: PropostaDopoApertura})
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || len(m) != 5 || m["motivo"] != "dopo_apertura" {
		t.Fatalf("forma del suggerimento: %s", b)
	}

	// PropostaNelRegistro: la regola di R2 per un suggerimento accettato
	nodo := func(k string) Nodo3 {
		for _, n := range s.Nodi {
			if n.ChiaveBozza == k {
				return n
			}
		}
		return Nodo3{}
	}
	if !PropostaNelRegistro(reg.Contenuto, nodo(k7), id.pdf) || PropostaNelRegistro(reg.Contenuto, nodo(k9), id.pdf) ||
		PropostaNelRegistro(reg.Contenuto, nodo(k7), uuid.New()) || PropostaNelRegistro(reg.Contenuto, Nodo3{}, id.pdf) {
		t.Fatal("PropostaNelRegistro")
	}
	// ricollocabiliDi: solo le PB motore svincolate dal comando
	prima := Snapshot3{DocumentiPreassociati: []Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaMotore},
		{AllegatoID: id.allegato, ChiaveBozza: k7, Provenienza: ProvenienzaCorretta}}}
	r := ricollocabiliDi(prima, []string{id.pdf.String(), id.allegato.String()})
	if len(r) != 1 || !r[id.pdf] {
		t.Fatalf("ricollocabili: %v", r)
	}
	// gli eventi del server delle PB iniziali portano provenienza e registro
	ee := eventiPBDelServer([]Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: k7, Provenienza: ProvenienzaMotore, Importazione: reg.Impronta}})
	if len(ee) != 1 || ee[0].Azione != AzFilePreassociato || ee[0].ChiaveBozza != k7 {
		t.Fatalf("eventi: %+v", ee)
	}
	var x Preassociazione3
	if err := json.Unmarshal(ee[0].Dopo, &x); err != nil || x.Provenienza != ProvenienzaMotore || x.Importazione != reg.Impronta {
		t.Fatalf("dopo dell'evento: %s", ee[0].Dopo)
	}
}
