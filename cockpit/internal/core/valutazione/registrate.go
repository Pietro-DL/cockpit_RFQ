package valutazione

// registrate.go: la fonte «manuale» del Motore A dalle associazioni registrate (direttiva 35 §8 C-4 [U]; piano delle
// dipendenze della direttiva 35 §7.2 C-4 e §12 V-02 [T]; EM §3.6, «Il Motore A nel frattempo usa la fonte transitoria»).
//
// Fino alla direttiva 35 il Motore A conosceva l'associazione a mano di un file solo dalla copia
// documento_proposta.componente_id di una proposta aperta (fotorfq.PropostaAttuale.ComponenteID), letta da quattro punti
// del calcolo: lo smistamento (fileDelThread, x.manuale), le associazioni decise degli ancoraggi (ancoraggi.go), i disegni
// (disegni_thread.go) e la completezza (completezza_thread.go, propostaAperta). Dalla direttiva 35 la fonte è
// file_rfq_associazione (righe attive), letta dal servizio unico accanto al caricatore (Ingressi.Registrate): il
// caricatore e la fotografia restano quelli di A1c.
//
// La regola transitoria [T] (piano §7.2 C-4: «AR attiva, altrimenti copia senza AR (pre-assegnazione legacy)»), in un
// punto solo, prima del calcolo del thread:
//   - per una proposta aperta di un allegato con una AR attiva, il componente è quello della AR (anche se la copia manca
//     o dice un altro componente: la copia non è una seconda fonte);
//   - per una proposta aperta di un allegato senza AR attiva la copia resta com'è: è la pre-assegnazione legacy (le copie
//     senza AR non hanno ancora una fonte autorevole: D-AR14, INC-S2, INC-S3). Senza il ripiego sparirebbero dal Motore A
//     e l'impronta degli ancoraggi cambierebbe;
//   - le proposte non aperte non cambiano (nessun lettore del calcolo le legge come «manuale»);
//   - una AR di un allegato senza proposta non entra (il Motore A, come i lettori legacy, non la vede: INC-G24).
//
// Senza AR (mappa vuota o nil) il thread è restituito com'è: l'esito è quello di prima, byte per byte (la prova
// dell'impronta prima/dopo: registrate_test.go). Con una AR uguale alla copia l'esito non cambia (la copia la scrive il
// servizio unico nella stessa transazione della AR: Prova 11). Il ripiego si toglie solo dopo D-AR14 (con D-AR03 e
// D-AR04 risposte il 19/10), con la prova dell'impronta prima e dopo su dati con le P1.

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
)

// conLeAssociazioniRegistrate: il thread con la fonte «manuale» del Motore A (la regola del commento di testa). ar è la
// mappa allegato → componente delle AR attive del thread. Le proposte del thread sono copiate prima di cambiarle: la
// fotografia di chi chiama non cambia.
func conLeAssociazioniRegistrate(t fotorfq.Thread, ar map[uuid.UUID]uuid.UUID) fotorfq.Thread {
	if len(ar) == 0 {
		return t
	}
	proposte := make([]fotorfq.PropostaAttuale, len(t.Proposte))
	copy(proposte, t.Proposte)
	for i := range proposte {
		p := &proposte[i]
		if p.Stato != statoPropostaAperta {
			continue
		}
		if k, ok := ar[p.AllegatoID]; ok && (p.ComponenteID == nil || *p.ComponenteID != k) {
			p.ComponenteID = &k
		}
	}
	t.Proposte = proposte
	return t
}
