package ancoraggio

import (
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// GP-1 (addendum «contesto partner», §6): l'ancoraggio dei riferimenti dei partner e la regola «richiede
// abbinamento». Un riferimento letto sulla radice o su un nodo (per esempio il numero del disegno del fornitore
// scritto nell'id della radice STEP del cliente) non è un'identità: dà candidati proposti con il motivo
// riferimento_partner, e un file che ha solo riferimenti, o che cita un prodotto richiesto, non va mai fuori richiesta.

// sostegniDaRiferimentoRadice: le letture del file che coincidono con un riferimento letto sulla radice di una
// struttura che rappresenta il prodotto t (stesso namespace, base compatibile, marcatore compatibile). Il sostegno
// porta la radice e la lettura del riferimento, così le dimensioni del candidato dicono il confronto.
func (a *ancoratore) sostegniDaRiferimentoRadice(t ProdottoRichiesto, ident []motorea.LetturaCodice) []sostegno {
	var out []sostegno
	for _, s := range a.strutture {
		if s.Target != t.Rif || !radiceProdotto(s) {
			continue
		}
		for _, r := range a.riferimenti[s.AllegatoID][s.Radice] {
			for _, l := range ident {
				if l.Forma.Namespace != r.Forma.Namespace || !marcatoriCompatibili(marcatoreDi(r.Forma), marcatoreDi(l.Forma)) {
					continue
				}
				for _, b := range basiDi(l.Forma) {
					if compatibile(motorea.ConfrontaBasi(r.Forma.Base, b)) {
						out = append(out, sostegno{l: l, base: b, radice: s.Radice, lr: r.Forma})
						break
					}
				}
			}
		}
	}
	return out
}

// soloRiferimentiOVarianti (addendum §6, regola 3): senza candidati, il file ha solo riferimenti di un partner
// (nessuna identità propria letta) oppure solo varianti di una tavola comune: è pertinente ma non associabile da solo.
// Chi chiama lo mette dopo i casi con candidati e prima di «nessuna lettura»; i motivi dello scenario restano.
func soloRiferimentiOVarianti(f FileInterpretato, ident []motorea.LetturaCodice) bool {
	_, riferimenti := DividiRiferimenti(f.Interpretazione.Letture)
	if len(ident) == 0 {
		return len(riferimenti) > 0
	}
	for _, l := range ident {
		if l.Ruolo != grammatica.RuoloLetturaVariante {
			return false
		}
	}
	return true
}

// citaUnTarget (addendum §6, regola 4; direttiva 21 §5.3): una lettura del file che non è un'identità (una menzione nel
// testo, un attributo) ha il namespace, il marcatore compatibile e una base compatibile con un prodotto richiesto.
// Citare il prodotto è un indizio di pertinenza, non una prova d'identità: vale solo al posto di «fuori richiesta».
func (a *ancoratore) citaUnTarget(f FileInterpretato) bool {
	// La citazione conta solo per le letture che non sono un'identità del file (menzioni nel testo, attributi): una
	// lettura d'identità con un altro marcatore è un'altra identità (T-B4-30), e resta con il suo esito.
	for _, l := range f.Interpretazione.Letture {
		if l.Funzione == motorea.FunzIdentitaFile {
			continue
		}
		for _, t := range a.target {
			if t.Namespace == "" || l.Forma.Namespace != t.Namespace || !marcatoriCompatibili(t.Marcatore, marcatoreDi(l.Forma)) {
				continue
			}
			if compatibile(motorea.ConfrontaBasi(t.Base, l.Forma.Base)) {
				return true
			}
		}
	}
	return false
}
