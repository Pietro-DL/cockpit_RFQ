package valutazione

import (
	"fmt"
	"sort"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// I segnali della richiesta (G-6, direttiva 21 §5.2; PM-19R [U]): informazioni, mai decisioni. Non cambiano stato,
// assi, motivi né impronta del prodotto.
//
// codice solo in mail: un codice letto nel corpo o nell'oggetto di un messaggio del thread come MENZIONE (la funzione
// che il router o la funzione massima della forma gli danno: per esempio un sollecito del cliente) e in nessun file del
// thread. Non diventa un target; il segnale dice che è da chiarire (un sollecito su un'altra richiesta, un errore di
// battitura, un documento che manca), senza inventare a quale richiesta appartenga.

// segnaliCodiceSoloInMail: una nota per codice (namespace e base), con i messaggi e le letture che lo citano.
func segnaliCodiceSoloInMail(messaggi []ancoraggio.MessaggioInterpretato, file []ancoraggio.FileInterpretato) []evidenze.Diagnostica {
	neiFile := map[string]bool{}
	for _, f := range file {
		for _, l := range f.Interpretazione.Letture {
			if l.Forma.Base.Completa {
				neiFile[l.Forma.Namespace+"\x00"+l.Forma.Base.Normalizzata] = true
			}
		}
	}
	type citazione struct {
		base string
		rif  []string
	}
	perCodice := map[string]*citazione{}
	var ordine []string
	for _, m := range messaggi {
		for _, l := range m.Interpretazione.Letture {
			s := l.Forma.Selettore.Contesto
			if l.Funzione != motorea.FunzMenzione || !l.Forma.Base.Completa || (s != evidenze.ContestoCorpo && s != evidenze.ContestoOggetto) {
				continue
			}
			k := l.Forma.Namespace + "\x00" + l.Forma.Base.Normalizzata
			if neiFile[k] {
				continue
			}
			c, ok := perCodice[k]
			if !ok {
				c = &citazione{base: l.Forma.Base.Normalizzata}
				perCodice[k] = c
				ordine = append(ordine, k)
			}
			c.rif = append(c.rif, m.MessaggioID.String(), l.ID)
		}
	}
	sort.Strings(ordine)
	var out []evidenze.Diagnostica
	for _, k := range ordine {
		c := perCodice[k]
		out = append(out, evidenze.Diagnostica{
			Codice:    CodiceRichiestaCodiceSoloInMail,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  "richiesta",
			Messaggio: fmt.Sprintf("il codice %s è citato nella mail come menzione e in nessun file del thread: da chiarire (sollecito, altra richiesta, documento mancante); non è un target", c.base),
			Rif:       c.rif,
		})
	}
	return out
}
