// L1 — gli aiuti: le funzioni interne di valutazione che le prove esterne chiamano direttamente, per provarne la regola
// con ingressi sintetici (la composizione dei conflitti dell'esito: T-B6-10, F0-13; R106 B e R108 A, le risposte del
// 07/10; l'attribuzione dei conflitti identita_documento ai prodotti: T-E1R-08; le diagnostiche degli errori dei file:
// R-61 della revisione di V1; da V3 la funzione di R111 A, precisata il 07/10, e l'adattatore delle condizioni nuove,
// T-E1-14; le sezioni dello smistamento per l'invariante degli orfani: R-75 della revisione di V2; da P7e il documento
// che decide la voce del 2D, EB7-4 A). Nessun cliente qui.
package valutazione

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// DiagnosticheDeiFilePerProva: diagnosticheDeiFile, con gli errori dei file dati (R-61).
func DiagnosticheDeiFilePerProva(errori map[uuid.UUID]error) []evidenze.Diagnostica {
	return diagnosticheDeiFile(&collegamento{errori: errori})
}

// ComponiConflittiPerProva: componiConflitti, per le prove della regola con pezzi sintetici.
var ComponiConflittiPerProva = componiConflitti

// ContestoApplicabilePerProva: contestoApplicabile, R106 B, precisata dall'utente il 07/10.
var ContestoApplicabilePerProva = contestoApplicabile

// VoceDaSmistarePerProva: voceDaSmistare, R108 A (confermata il 07/10), su un file sintetico: il perimetro, lo stato
// terminale, l'orfano, i due assi della proposta, il numero dei candidati e delle letture d'identità; un'associazione
// vuota è un file che l'adattatore non legge (nessun ancoraggio).
func VoceDaSmistarePerProva(perimetro string, terminale, orfano bool, associazione ancoraggio.Associazione, collocazione ancoraggio.Collocazione,
	candidati, letture int) (MotivoSmistamento, bool) {
	x := &fileDelThread{perimetro: perimetro, terminale: terminale, orfano: orfano}
	if associazione != "" {
		af := &ancoraggio.AncoraggioFile{Associazione: associazione, Collocazione: collocazione}
		for i := 0; i < candidati; i++ {
			af.Candidati = append(af.Candidati, ancoraggio.CandidatoAncoraggio{})
		}
		for i := 0; i < letture; i++ {
			af.Letture = append(af.Letture, "l")
		}
		x.af = af
	}
	return voceDaSmistare(x)
}

// AttribuisciIdentitaPerProva: conflittiIdentita, con i documenti del thread dati e la pertinenza di ogni allegato; torna
// anche gli allegati segnati in conflitto.
func AttribuisciIdentitaPerProva(pezzi []Conflitto, documenti []fotorfq.DocumentoConfermato, pertinenza map[uuid.UUID][]string) ([]Conflitto, map[uuid.UUID]bool) {
	s := &smistamentoThread{t: fotorfq.Thread{Documenti: documenti}}
	file := map[uuid.UUID]*fileDelThread{}
	for a, p := range pertinenza {
		file[a] = &fileDelThread{a: fotorfq.Allegato{ID: a}, perimetro: PerimetroDentro, pertinente: p}
	}
	out := s.conflittiIdentita(pezzi, file)
	segnati := map[uuid.UUID]bool{}
	for a, x := range file {
		segnati[a] = x.conflitto
	}
	return out, segnati
}

// VociNuoveDopoLaFonteSuperataR111PerProva: vociNuoveDopoLaFonteSuperataR111, R111 A, precisata dall'utente il 07/10.
var VociNuoveDopoLaFonteSuperataR111PerProva = vociNuoveDopoLaFonteSuperataR111

// CondizioniNuoveDellaBOMPerProva: condizioniNuoveDellaBOM, l'adattatore delle condizioni nuove di T-E1-14.
var CondizioniNuoveDellaBOMPerProva = condizioniNuoveDellaBOM

// MotiviDelNodoPerProva: motiviDelNodo, sugli ancoraggi, le associazioni (in ordine di allegato) e i conflitti dati.
func MotiviDelNodoPerProva(prodotto string, n ancoraggio.NodoProposto, componente *uuid.UUID, ancoraggi []ancoraggio.AncoraggioFile,
	associazioni []AssociazioneFile, conflitti []Conflitto) []MotivoSmistamento {
	x := &nodiThread{ancoraggi: ancoraggi, associazioni: associazioni, perAllegato: map[uuid.UUID]*AssociazioneFile{}, conflitti: conflitti}
	for i := range associazioni {
		x.perAllegato[associazioni[i].AllegatoID] = &associazioni[i]
	}
	return x.motiviDelNodo(ProdottoValutato{Rif: prodotto}, n, componente)
}

// SezioniDelloSmistamentoPerProva: le sezioni della fotografia da cui lo smistamento dipende (T-12), per l'invariante
// degli orfani dell'aiuto calcola (R-75 della revisione di V2).
var SezioniDelloSmistamentoPerProva = sezioniDelloSmistamento

// DocumentoDellaVoceDel2DPerProva: documentoDellaVoceDel2D, il documento che decide la voce del 2D (EB7-4 A, P7e), per la
// prova della regola su gruppi sintetici.
var DocumentoDellaVoceDel2DPerProva = documentoDellaVoceDel2D
