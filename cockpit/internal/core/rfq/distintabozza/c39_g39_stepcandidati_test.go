package distintabozza

// L1 — G39-02 [U] e G39-01 [U] (20/10 sera), corsia persistenza parte A: il riconoscimento dello STEP candidato sorgente
// del finito sul calcolo del motore (StepCandidatiDalMotore, puro), il calcolo di adesso (CalcoloAdessoDa) e l'unione dei
// suggerimenti. La lettura vera del motore (il nome e la radice con il marcatore) e' provata in L4 con le regole ACME in
// transport/web; qui le tre strade del riconoscimento, una per una, e i casi che non contano. Dati sintetici.

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

func TestV1G39StepCandidatiDalMotore(t *testing.T) {
	if StepCandidatiDalMotore(nil, nil) != nil {
		t.Fatal("senza calcolo: nil")
	}
	p1, p2 := uuid.New(), uuid.New()
	aNome, aRadice, aStruttura, aDoc, aProposta, aConflitto, aComponente, aInterna := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	et := &valutazione.EsitoThread{
		ProdottiValutati: []valutazione.ProdottoValutato{
			{Rif: "componente:" + p1.String(), ComponenteID: &p1,
				Fonte: valutazione.FonteProdotto{Candidati: []ancoraggio.DocumentoCandidato{
					{Origine: ancoraggio.CandidatoDaMotoreA, AllegatoID: &aRadice, RadiceCompatibile: "#1"},
					{Origine: ancoraggio.CandidatoDaDocumentoDelProdotto, AllegatoID: &aDoc},
					// la proposta aperta legacy senza una radice compatibile non basta (non e' la lettura del motore)
					{Origine: ancoraggio.CandidatoDaPropostaAperta, AllegatoID: &aProposta},
				}},
				StruttureCandidate: []valutazione.StrutturaCandidata{
					{AllegatoID: aStruttura, RadiceDelFile: true},
					// il prodotto come nodo interno di un altro STEP: non e' lo STEP del finito
					{AllegatoID: aInterna, RadiceDelFile: false},
				}},
			// un prodotto dello scenario senza componente non conta
			{Rif: "scenario:x"},
			{Rif: "componente:" + p2.String(), ComponenteID: &p2},
		},
		Ancoraggi: ancoraggio.EsitoAncoraggi{File: []ancoraggio.AncoraggioFile{
			{AllegatoID: aNome, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: "componente:" + p2.String(), Livello: ancoraggio.LivelloProdotto}}},
			{AllegatoID: aConflitto, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: "componente:" + p2.String(), Livello: ancoraggio.LivelloProdotto,
				Conflitti: []string{"marcatore"}}}},
			{AllegatoID: aComponente, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: "componente:" + p2.String(), Livello: ancoraggio.LivelloComponente}}},
			{AllegatoID: aRadice, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: "componente:" + p2.String(), Livello: ancoraggio.LivelloProdotto}}},
		}},
	}
	s := StepCandidatiDalMotore(et, nil)
	if !s.DalMotore {
		t.Fatal("DalMotore")
	}
	// aRadice: candidato della fonte di p1 e dell'ancoraggio di p2, due prodotti finiti: due candidature non univoche,
	// tutte e due STEP candidati per R2, PB iniziali e R4, nessuna fra le univoche (giro G39-bis, revisione indipendente)
	attesi := map[uuid.UUID][]uuid.UUID{aDoc: {p1}, aStruttura: {p1}, aNome: {p2}, aRadice: {p1, p2}}
	for a, pp := range attesi {
		if len(s.Prodotti[a]) != len(pp) {
			t.Fatalf("allegato %s: %v, attesi %v", a, s.Prodotti[a], pp)
		}
		for _, p := range pp {
			trovato := false
			for _, x := range s.Prodotti[a] {
				trovato = trovato || x == p
			}
			if !trovato {
				t.Fatalf("allegato %s: manca il prodotto %s (%v)", a, p, s.Prodotti[a])
			}
		}
	}
	univoche := map[uuid.UUID]uuid.UUID{aDoc: p1, aStruttura: p1, aNome: p2}
	if len(s.Univoche) != len(univoche) {
		t.Fatalf("le univoche: %v, attese %v", s.Univoche, univoche)
	}
	for a, p := range univoche {
		if !s.UnivocaDel(a, p) {
			t.Fatalf("allegato %s: non e' il candidato univoco di %s (%v)", a, p, s.Univoche)
		}
	}
	if s.UnivocaDel(aRadice, p1) || s.UnivocaDel(aRadice, p2) || s.UnivocaDel(aDoc, p2) {
		t.Fatalf("univoca per una candidatura non univoca o per un altro prodotto: %v", s.Univoche)
	}
	// le evidenze delle sole univoche, per la traccia di S8 (giro G39-bis, integrazione)
	for a := range univoche {
		if _, ok := s.Evidenze[a]; !ok {
			t.Fatalf("allegato %s: la candidatura univoca senza la voce delle evidenze", a)
		}
	}
	for _, a := range []uuid.UUID{aDoc, aStruttura} { // l'ancoraggio di aNome non ha letture: nessuna evidenza
		if len(s.EvidenzeUnivocaDel(a, p1)) == 0 {
			t.Fatalf("allegato %s: la candidatura univoca senza le evidenze", a)
		}
	}
	if s.EvidenzeUnivocaDel(aRadice, p1) != nil || s.EvidenzeUnivocaDel(aDoc, p2) != nil || len(s.Evidenze) != len(univoche) {
		t.Fatalf("evidenze di una candidatura non univoca o di un altro prodotto: %v", s.Evidenze)
	}
	if (&StepCandidati{Prodotti: s.Prodotti, Univoche: s.Univoche}).UnivocaDel(aDoc, p1) {
		t.Fatal("UnivocaDel senza il motore (ripiego)")
	}
	for _, a := range []uuid.UUID{aProposta, aConflitto, aComponente, aInterna} {
		if s.Di(a) {
			t.Fatalf("allegato %s candidato: %v", a, s.Prodotti[a])
		}
	}
	if got := s.DelProdotto(p2); len(got) != 2 {
		t.Fatalf("DelProdotto: %v", got)
	}
	var nulla *StepCandidati
	if nulla.Di(aNome) || nulla.DelProdotto(p1) != nil || nulla.UnivocaDel(aNome, p2) {
		t.Fatal("nil")
	}
}

// L1 — una sola regola dello STEP candidato del finito (rilievo della revisione della corsia motore, giro G39; giro
// G39-bis, 21/10, e revisione indipendente): le coppie (allegato, prodotto) della persistenza sono quelle di TUTTE le
// candidature di propostabom.CandidatureSorgenteDelFinito (anche le non univoche: lo STEP legato a due prodotti finiti
// resta uno STEP candidato per R2, le PB iniziali e R4, G39-02 [U]); Univoche ha le sole univoche; una candidatura tolta
// dall'evidenza discordante (un nodo nell'ancoraggio; il nome che e' il codice di un altro componente) non c'e'.
func TestV1G39StepCandidatiUnaSolaRegolaColMotore(t *testing.T) {
	p1, p2 := uuid.New(), uuid.New()
	aDue, aAltro, aSolo := uuid.New(), uuid.New(), uuid.New()
	r1, r2 := "componente:"+p1.String(), "componente:"+p2.String()
	et := &valutazione.EsitoThread{
		ProdottiValutati: []valutazione.ProdottoValutato{
			{Rif: r1, ComponenteID: &p1, StruttureCandidate: []valutazione.StrutturaCandidata{{AllegatoID: aDue, RadiceDelFile: true}}},
			{Rif: r2, ComponenteID: &p2},
		},
		Ancoraggi: ancoraggio.EsitoAncoraggi{File: []ancoraggio.AncoraggioFile{
			{AllegatoID: aDue, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: r2, Livello: ancoraggio.LivelloProdotto}}},
			{AllegatoID: aAltro, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: r1, Livello: ancoraggio.LivelloProdotto},
				{Target: "nodo:x", Livello: ancoraggio.LivelloComponente}}},
			{AllegatoID: aSolo, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: r2, Livello: ancoraggio.LivelloProdotto}}},
		}},
	}
	cc := propostabom.CandidatureSorgenteDelFinito(et, nil, func(uuid.UUID) bool { return true })
	attese := map[[2]uuid.UUID]bool{} // (allegato, prodotto) -> univoca
	for _, c := range cc {
		attese[[2]uuid.UUID{c.AllegatoID, c.ProdottoID}] = c.Univoca
	}
	// aDue: p1 (struttura) e p2 (ancoraggio), due finiti: non univoche; aAltro tolto (un nodo nell'ancoraggio); aSolo univoca
	if len(attese) != 3 || attese[[2]uuid.UUID{aDue, p1}] || attese[[2]uuid.UUID{aDue, p2}] || !attese[[2]uuid.UUID{aSolo, p2}] {
		t.Fatalf("candidature del motore: %+v", cc)
	}
	s := StepCandidatiDalMotore(et, nil)
	n := 0
	for a, pp := range s.Prodotti {
		for _, p := range pp {
			n++
			if _, ok := attese[[2]uuid.UUID{a, p}]; !ok {
				t.Fatalf("coppia %s/%s non e' una candidatura del motore", a, p)
			}
		}
	}
	if n != len(attese) {
		t.Fatalf("coppie %d, attese %d (%v)", n, len(attese), s.Prodotti)
	}
	if len(s.Univoche) != 1 || !s.UnivocaDel(aSolo, p2) {
		t.Fatalf("le univoche: %v", s.Univoche)
	}
}

func TestV1G39CalcoloAdessoDa(t *testing.T) {
	if c, err := CalcoloAdessoDa(nil, nil); c != nil || err != nil {
		t.Fatalf("senza ingresso: %v %v", c, err)
	}
	step := &StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{}}
	importati := 0
	in := &IngressoMotoreR4{StepCandidati: step, Importa: func(string) (importazione.Registro, error) {
		importati++
		return importazione.Registro{Impronta: "x"}, nil
	}}
	// senza sorgente STEP: niente registro, gli STEP si'
	c, err := CalcoloAdessoDa(in, nil)
	if err != nil || c.Registro != nil || c.Step != step || importati != 0 {
		t.Fatalf("senza sorgente: %+v %v", c, err)
	}
	// il motore non propone piu' il gruppo della sorgente: niente registro, nessun errore
	a := uuid.New()
	c, err = CalcoloAdessoDa(in, &propostabom.SorgenteBozza{AllegatoID: a, Sha256: "s", Gruppo: "bom"})
	if err != nil || c.Registro != nil || importati != 0 {
		t.Fatalf("gruppo non proposto: %+v %v", c, err)
	}
	if c.registro() != nil || (*CalcoloAdesso)(nil).step() != nil || (*CalcoloAdesso)(nil).registro() != nil {
		t.Fatal("accessori")
	}
}

func TestV1G39UnisciSuggerimenti(t *testing.T) {
	a, b, c := uuid.MustParse("00000000-0000-4000-8000-000000000001"), uuid.MustParse("00000000-0000-4000-8000-000000000002"),
		uuid.MustParse("00000000-0000-4000-8000-000000000003")
	v := []PreassociazioneProposta3{{AllegatoID: c, Motivo: PropostaPBToccata}}
	x := []PreassociazioneProposta3{{AllegatoID: b, Motivo: PropostaDopoApertura}, {AllegatoID: c, Motivo: PropostaDopoApertura}, {AllegatoID: a, Motivo: PropostaConflitto}}
	out := unisciSuggerimenti(v, x)
	if len(out) != 3 || out[0].AllegatoID != a || out[1].AllegatoID != b || out[2].AllegatoID != c || out[2].Motivo != PropostaPBToccata {
		t.Fatalf("unione: %+v", out)
	}
}

// L1 — giro G39-bis (21/10): StepCandidatiDalMotore con la fotografia. Lo STEP «12345678-100.stp» del sottoassieme
// 12345678-100, che il motore ancora al finito 12345678Z con la sola base, non e' uno STEP candidato (D-2 (c) [U]); lo
// STEP «12345678Z.stp», che il motore non legge, lo e' per la rete del nome esatto (G39-02 [U], [T-39]). Codici inventati.
func TestV1G39bisStepCandidatiConLaFotografia(t *testing.T) {
	fin, sotto := uuid.New(), uuid.New()
	aSotto, aNome, aBase := uuid.New(), uuid.New(), uuid.New()
	rif := "componente:" + fin.String()
	et := &valutazione.EsitoThread{
		ProdottiValutati: []valutazione.ProdottoValutato{{Rif: rif, ComponenteID: &fin, CodiceRichiesto: "12345678Z"}},
		Ancoraggi: ancoraggio.EsitoAncoraggi{File: []ancoraggio.AncoraggioFile{
			{AllegatoID: aSotto, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: rif, Livello: ancoraggio.LivelloProdotto}}},
			{AllegatoID: aBase, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: rif, Livello: ancoraggio.LivelloProdotto}}},
			{AllegatoID: aNome},
		}},
	}
	stp := "stp"
	th := &fotorfq.Thread{
		Componenti: []fotorfq.Componente{{ID: fin, Codice: "12345678Z", Tipo: "finito"}, {ID: sotto, Codice: "12345678-100", Tipo: "sottoassieme"}},
		Allegati: []fotorfq.Allegato{{ID: aSotto, NomeFile: "12345678-100.stp", Estensione: &stp}, {ID: aNome, NomeFile: "12345678Z.stp", Estensione: &stp},
			{ID: aBase, NomeFile: "12345678-001.stp", Estensione: &stp}},
	}
	s := StepCandidatiDalMotore(et, th)
	if s.Di(aSotto) {
		t.Errorf("lo STEP del sottoassieme e' candidato: %v", s.Prodotti[aSotto])
	}
	for _, a := range []uuid.UUID{aNome, aBase} {
		if pp := s.Prodotti[a]; len(pp) != 1 || pp[0] != fin {
			t.Errorf("allegato %s: %v, atteso il finito", a, pp)
		}
	}
	// senza la fotografia (nessun nome): lo STEP del sottoassieme resta legato, la rete del nome non c'e'
	s = StepCandidatiDalMotore(et, nil)
	if !s.Di(aSotto) || s.Di(aNome) {
		t.Errorf("senza la fotografia: %v", s.Prodotti)
	}
}
