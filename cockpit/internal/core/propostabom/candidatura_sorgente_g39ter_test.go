// L1 — giro G39-ter (21/10), corsia motore: la regola unica dello STEP candidato del finito (candidatura_sorgente.go) sui
// rilievi del gate della direttiva 39 (P39-08):
//   - D-2 (c) [U]: con il sottoassieme 12345678-100 e il finito 12345678Z, gli STEP «12345678-100A.stp»,
//     «12345678-100(B).stp», «12345678-100rev2.stp» e «12345678_100.stp», ancorati al finito con la sola base, non sono
//     mai candidati univoci del finito: la parentesi è un separatore (lo STEP è del sottoassieme, la candidatura è tolta);
//     una lettera o una cifra subito dopo il codice del sottoassieme, o il suo codice scritto con un separatore diverso,
//     rendono la candidatura non univoca (il motore non sa decidere, G39-06 [U]);
//   - G39-02 [U]: lo STEP con una sua struttura con la radice del finito e il nome di un figlio resta candidato, non
//     univoco, invece di diventare un file ordinario.
//
// Falliscono su 9eb44e3. Codici inventati.
package propostabom_test

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

// esitoG39ter: "tolta", "univoca" o "non_univoca" per la candidatura dell'allegato verso il finito dato.
func esitoG39ter(t *testing.T, cc []propostabom.CandidaturaSorgente, a, fin uuid.UUID) string {
	t.Helper()
	esito := "tolta"
	for _, c := range cc {
		if c.AllegatoID != a {
			continue
		}
		if c.ProdottoID != fin {
			t.Errorf("candidatura di un altro prodotto: %+v", c)
			continue
		}
		esito = map[bool]string{true: "univoca", false: "non_univoca"}[c.Univoca]
	}
	return esito
}

func TestG39terNomiDelSottoassiemeSenzaSeparatore(t *testing.T) {
	casi := []struct{ nome, esito string }{
		{"12345678-100A.stp", "non_univoca"},
		{"12345678-100(B).stp", "tolta"},
		{"12345678-100rev2.stp", "non_univoca"},
		{"12345678_100.stp", "non_univoca"},
		{"12345678 100.stp", "non_univoca"},
		{"12345678-100[2].stp", "tolta"},
		{"12345678-100#2.stp", "tolta"},
		// i riscontri: il nome del finito e la base con un testo che non è il codice di un altro componente
		{"12345678Z.stp", "univoca"},
		{"12345678Z_rev2.stp", "univoca"},
		{"12345678-001.stp", "univoca"},
		{"12345678.stp", "univoca"},
	}
	fin, sotto := uuid.New(), uuid.New()
	rif := "componente:" + fin.String()
	stp := "stp"
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			a := uuid.New()
			et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{{Rif: rif, ComponenteID: &fin, CodiceRichiesto: "12345678Z"}}}
			et.Ancoraggi.File = []ancoraggio.AncoraggioFile{{AllegatoID: a,
				Candidati: []ancoraggio.CandidatoAncoraggio{{Target: rif, Livello: ancoraggio.LivelloProdotto}}}}
			th := &fotorfq.Thread{Componenti: []fotorfq.Componente{{ID: fin, Codice: "12345678Z", Tipo: "finito"}, {ID: sotto, Codice: "12345678-100", Tipo: "sottoassieme"}},
				Allegati: []fotorfq.Allegato{{ID: a, NomeFile: c.nome, Estensione: &stp}}}
			cc := propostabom.CandidatureSorgenteDelFinito(et, th, func(uuid.UUID) bool { return true })
			if got := esitoG39ter(t, cc, a, fin); got != c.esito {
				t.Errorf("lo STEP «%s» è %s, atteso %s: %+v", c.nome, got, c.esito, cc)
			}
		})
	}
}

func TestG39terStrutturaPropriaColNomeDelFiglio(t *testing.T) {
	fin, sotto := uuid.New(), uuid.New()
	rif := "componente:" + fin.String()
	stp := "stp"
	for _, c := range []struct{ nome, esito string }{
		{"assieme.stp", "univoca"},
		{"12345678-100.stp", "non_univoca"},
		{"12345678-100_rev2.stp", "non_univoca"},
		{"12345678-100A.stp", "non_univoca"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			a := uuid.New()
			et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{{Rif: rif, ComponenteID: &fin, CodiceRichiesto: "12345678Z",
				StruttureCandidate: []valutazione.StrutturaCandidata{{AllegatoID: a, RadiceDelFile: true}}}}}
			et.Ancoraggi.File = []ancoraggio.AncoraggioFile{{AllegatoID: a, Candidati: []ancoraggio.CandidatoAncoraggio{
				{Target: rif, Livello: ancoraggio.LivelloProdotto},
				{Target: "componente:" + sotto.String(), Livello: ancoraggio.LivelloComponente, RadiciRaggiungibili: []string{rif}}}}}
			th := &fotorfq.Thread{Componenti: []fotorfq.Componente{{ID: fin, Codice: "12345678Z", Tipo: "finito"}, {ID: sotto, Codice: "12345678-100", Tipo: "sottoassieme"}},
				Allegati: []fotorfq.Allegato{{ID: a, NomeFile: c.nome, Estensione: &stp}}}
			cc := propostabom.CandidatureSorgenteDelFinito(et, th, func(uuid.UUID) bool { return true })
			if got := esitoG39ter(t, cc, a, fin); got != c.esito {
				t.Errorf("lo STEP «%s» con la struttura propria con la radice del finito è %s, atteso %s: %+v", c.nome, got, c.esito, cc)
			}
		})
	}
	// senza la struttura propria il nome del figlio toglie ancora la candidatura (D-2 (c) [U], riscontro)
	a := uuid.New()
	et := &valutazione.EsitoThread{ProdottiValutati: []valutazione.ProdottoValutato{{Rif: rif, ComponenteID: &fin, CodiceRichiesto: "12345678Z"}}}
	et.Ancoraggi.File = []ancoraggio.AncoraggioFile{{AllegatoID: a, Candidati: []ancoraggio.CandidatoAncoraggio{{Target: rif, Livello: ancoraggio.LivelloProdotto}}}}
	th := &fotorfq.Thread{Componenti: []fotorfq.Componente{{ID: fin, Codice: "12345678Z", Tipo: "finito"}, {ID: sotto, Codice: "12345678-100", Tipo: "sottoassieme"}},
		Allegati: []fotorfq.Allegato{{ID: a, NomeFile: "12345678-100.stp", Estensione: &stp}}}
	if got := esitoG39ter(t, propostabom.CandidatureSorgenteDelFinito(et, th, func(uuid.UUID) bool { return true }), a, fin); got != "tolta" {
		t.Errorf("senza la struttura propria lo STEP «12345678-100.stp» è %s, atteso tolta", got)
	}
}
