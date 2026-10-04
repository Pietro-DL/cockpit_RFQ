package motorea

import (
	"fmt"
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L'applicazione dei limiti di riconoscimento. I valori non sono di questo pacchetto: li dichiara l'indice
// delle regole, con la loro versione, e arrivano a CompilaVerificato come grammatica.Limiti (R43 B). Qui non
// c'è nessun valore predefinito. Nessun limite di tempo, che romperebbe il determinismo (v3 §2): solo
// conteggi. Superare un limite dà limite.superato (la costante di grammatica, come avviso, natura limite) e
// un risultato parziale, mai un successo vuoto (E-23).

// fineInizi: fin dove si cercano gli inizi delle letture. Con un testo oltre max_byte_unita si provano solo
// gli inizi nei primi max_byte_unita byte, su un confine di runa; una lettura che comincia lì si legge per
// intero, così il taglio non inventa un codice più corto. Il secondo valore dice se il limite è superato.
func fineInizi(testo string, lim grammatica.LimitiRiconoscimento) (int, bool) {
	massimo := lim.MaxByteUnita
	if massimo <= 0 || len(testo) <= massimo {
		return len(testo), false
	}
	for massimo > 0 && !utf8.RuneStart(testo[massimo]) {
		massimo--
	}
	return massimo, true
}

// diagnosticaByte: il testo supera max_byte_unita.
func diagnosticaByte(n, massimo int) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    grammatica.CodiceLimiteSuperato,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaLimite,
		Percorso:  "limiti.riconoscimento.max_byte_unita",
		Messaggio: fmt.Sprintf("testo di %d byte, max_byte_unita dell'indice è %d: letture cercate solo nei primi byte, risultato parziale", n, massimo),
	}
}

// diagnosticaLetture: le letture su un'unità superano max_letture_per_unita.
func diagnosticaLetture(massimo int) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    grammatica.CodiceLimiteSuperato,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaLimite,
		Percorso:  "limiti.riconoscimento.max_letture_per_unita",
		Messaggio: fmt.Sprintf("più di %d letture sul testo (max_letture_per_unita dell'indice): restano le prime %d in ordine di posizione, risultato parziale", massimo, massimo),
	}
}

// tagliaLetture: al più max_letture_per_unita letture, le prime nell'ordine di Riconosci. Il secondo valore
// dice se ne sono state tolte.
func tagliaLetture(l []LetturaForma, lim grammatica.LimitiRiconoscimento) ([]LetturaForma, bool) {
	if lim.MaxLetturePerUnita <= 0 || len(l) <= lim.MaxLetturePerUnita {
		return l, false
	}
	return l[:lim.MaxLetturePerUnita], true
}
