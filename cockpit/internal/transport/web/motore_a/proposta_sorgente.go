package motore_a

// proposta_sorgente.go: lo STEP candidato del prodotto finito nella fonte, come PROPOSTA di sorgente (giro G39, 20/10;
// G39-02 [U]: «prima di S8 la pagina propone "il motore ha trovato che questo file è molto probabilmente lo STEP
// strutturale: lo accetti?"»; S8 è la conferma dell'utente; lo STEP del finito non è mai una PB né una AR ordinaria).
//
// La regola è una sola, nel core: propostabom.CandidatureSorgenteDelFinito, la lettura del Motore A sull'esito dello
// stesso calcolo (l'ancoraggio del file a livello prodotto: il codice letto dal nome del file o dalla radice dello STEP
// contro il codice del prodotto, con la regola unica del marcatore). Qui si sceglie solo che cosa della fotografia
// entra (gli allegati STEP attivi del thread) e come si mostra [T-39, corsia motore]:
//   - /dati.fonte e S9 prodotti[].fonte: proposte_sorgente[], solo prima di S8 (il prodotto senza una sorgente di S8
//     valida: dopo, cambiare la sorgente è una revisione, dalle candidate di step[]), le candidature univoche del
//     prodotto, con le evidenze, il testo per la pagina e sorgente_di quando lo STEP è la sorgente valida di S8 di un
//     altro prodotto (G39-03: lo spostamento della sorgente è un gesto a parte);
//   - S9: lo STEP candidato (univoco o no) non è mai una riga proposto né in_conflitto del motore, in nessuna sezione,
//     né la chip «attende la sorgente»; uno STEP candidato univoco non è in da_associare (è nella fonte del prodotto).
// Nessuna scrittura: S8 resta il solo gesto che registra la sorgente (fonte_step_evento).

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
)

// TestoPropostaSorgente: il testo della proposta di sorgente per la pagina, con il codice del prodotto (G39-02 [U]).
const TestoPropostaSorgente = "Il motore ha trovato che questo file è molto probabilmente lo STEP strutturale di %s: lo confermi come sorgente?"

// PropostaSorgente: uno STEP che il Motore A propone come sorgente del prodotto (commento di testa).
type PropostaSorgente struct {
	AllegatoID     uuid.UUID                         `json:"allegato_id"`
	NomeFile       string                            `json:"nome_file,omitempty"`
	CodiceProdotto string                            `json:"codice_prodotto"`
	Evidenze       []propostabom.EvidenzaCandidatura `json:"evidenze"`
	SorgenteDi     *uuid.UUID                        `json:"sorgente_di,omitempty"`
	Testo          string                            `json:"testo"`
}

// CandidatureSorgente: le candidature degli STEP attivi del thread (propostabom.CandidatureSorgenteDelFinito sull'esito
// di questo calcolo); vuote senza esito.
func (r *Risultato) CandidatureSorgente() []propostabom.CandidaturaSorgente {
	if r == nil || r.Thread == nil || r.EsitoThread == nil {
		return nil
	}
	step := map[uuid.UUID]bool{}
	for _, a := range r.Thread.Allegati {
		est := ""
		if a.Estensione != nil {
			est = *a.Estensione
		}
		if _, na := r.V1.NonAttivi[a.ID]; !na && estensioneSTEP(est, a.NomeFile) {
			step[a.ID] = true
		}
	}
	return propostabom.CandidatureSorgenteDelFinito(r.EsitoThread, r.Thread, func(a uuid.UUID) bool { return step[a] })
}

// proposteSorgente: le proposte di sorgente del prodotto (commento di testa).
func proposteSorgente(r *Risultato, prodotto uuid.UUID, cc []propostabom.CandidaturaSorgente) []PropostaSorgente {
	out := []PropostaSorgente{}
	if s, ok := r.V1.SorgentiS8[prodotto]; ok && s.Stato.Valida {
		return out // la sorgente è già confermata: niente proposte (commento di testa)
	}
	nomi := map[uuid.UUID]string{}
	if r.Thread != nil {
		for _, a := range r.Thread.Allegati {
			nomi[a.ID] = a.NomeFile
		}
	}
	// la sorgente valida di S8 degli altri prodotti: lo STEP si propone con sorgente_di
	sorgenteDi := map[uuid.UUID]uuid.UUID{}
	ids := make([]uuid.UUID, 0, len(r.V1.SorgentiS8))
	for p := range r.V1.SorgentiS8 {
		ids = append(ids, p)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, p := range ids {
		if s := r.V1.SorgentiS8[p]; s.Stato.Valida {
			if _, ok := sorgenteDi[s.AllegatoID]; !ok {
				sorgenteDi[s.AllegatoID] = p
			}
		}
	}
	for _, c := range cc {
		if !c.Univoca || c.ProdottoID != prodotto {
			continue
		}
		x := PropostaSorgente{AllegatoID: c.AllegatoID, NomeFile: nomi[c.AllegatoID], CodiceProdotto: c.CodiceProdotto, Evidenze: c.Evidenze,
			Testo: fmt.Sprintf(TestoPropostaSorgente, c.CodiceProdotto)}
		if altro, ok := sorgenteDi[c.AllegatoID]; ok {
			a := altro
			x.SorgenteDi = &a
		}
		out = append(out, x)
	}
	return out
}
