package grammatica

import (
	"fmt"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Limiti: le soglie del motore A. NON sono costanti del codice e non stanno nella grammatica del cliente: le
// dichiara l'indice delle regole, con la loro versione, e le legge LeggiIndice (R43 B; parte 1 §7.5:
// «configurazioni server versionate, non valori modificabili dalla grammatica cliente»). Il banco e
// l'anteprima leggono lo stesso indice, quindi lavorano con gli stessi limiti. Il tipo sta qui perché qui si
// decodifica l'indice: motorea li applica, ma non ne ha di suoi (grammatica non può importare motorea, D-13).
// Nessun limite di tempo per singola regex (v3 §2): il tempo massimo riguarda solo una richiesta
// dell'anteprima, e lo usa A1d.
//
// Non c'è nessun valore predefinito: un indice senza limiti non attiva niente (Valida).
type Limiti struct {
	Versione       string               `json:"versione_limiti"` // cambia a ogni cambio di un valore
	Grammatica     LimitiGrammatica     `json:"grammatica"`      // validazione di una grammatica
	Riconoscimento LimitiRiconoscimento `json:"riconoscimento"`  // elaborazione di Riconosci e Interpreta
	Anteprima      LimitiAnteprima      `json:"anteprima"`       // il tempo massimo di una richiesta dell'anteprima (A1d)
}

// LimitiGrammatica: famiglie, forme, parti, esempi, pattern e letterali di un file regole. Le lunghezze sono
// in byte.
type LimitiGrammatica struct {
	MaxFamiglie           int `json:"max_famiglie"`
	MaxFormePerFamiglia   int `json:"max_forme_per_famiglia"`
	MaxPartiPerForma      int `json:"max_parti_per_forma"` // per una forma, e per la sequenza interna di una decorazione
	MaxEsempi             int `json:"max_esempi"`          // per famiglia
	MaxLunghezzaPattern   int `json:"max_lunghezza_pattern"`
	MaxRipetizione        int `json:"max_ripetizione"` // il massimo di un {n,m}
	MaxLunghezzaLetterale int `json:"max_lunghezza_letterale"`
}

// LimitiRiconoscimento: byte di un'unità, unità di un documento, letture per unità e per documento. In A1a li
// applica Riconosci; le unità del documento le usa Interpreta (A1b).
type LimitiRiconoscimento struct {
	MaxByteUnita        int `json:"max_byte_unita"`
	MaxUnitaDocumento   int `json:"max_unita_documento"`
	MaxLetturePerUnita  int `json:"max_letture_per_unita"`
	MaxLettureDocumento int `json:"max_letture_documento"`
}

// LimitiAnteprima: il tempo massimo, in millisecondi, che il server dà a una richiesta del riquadro. Sta
// nell'indice da subito, così il formato dell'indice non cambia in A1d; in A1a si controlla solo contro il
// tetto.
type LimitiAnteprima struct {
	TempoMassimoMs int `json:"tempo_massimo_ms"`
}

// TettiLimiti: i valori più alti che il codice accetta. Sono l'unica soglia scritta nel codice e non sono una
// taratura: proteggono memoria e tempo da un indice sbagliato. Si alzano solo con un commit che lo dichiara
// (par.3.4.1). La versione non ha un tetto: deve solo esserci, quindi qui resta vuota.
//
// Perché questi numeri, in astratto (P-18): ognuno lascia almeno il doppio di spazio sopra i valori iniziali
// dell'indice, che a loro volta stanno larghi sopra le grammatiche v1 private (la forma più ricca ha meno di
// dieci parti, il pattern più lungo e il letterale più lungo meno di venti byte, il testo più lungo visto
// negli export meno di un decimo di max_byte_unita). max_ripetizione resta sotto i 1000 che regexp/syntax
// accetta comunque.
func TettiLimiti() Limiti {
	return Limiti{
		Grammatica: LimitiGrammatica{
			MaxFamiglie:           64,
			MaxFormePerFamiglia:   64,
			MaxPartiPerForma:      32,
			MaxEsempi:             256,
			MaxLunghezzaPattern:   256,
			MaxRipetizione:        100,
			MaxLunghezzaLetterale: 128,
		},
		Riconoscimento: LimitiRiconoscimento{
			MaxByteUnita:        8 << 20,
			MaxUnitaDocumento:   100000,
			MaxLetturePerUnita:  10000,
			MaxLettureDocumento: 200000,
		},
		Anteprima: LimitiAnteprima{TempoMassimoMs: 60000},
	}
}

// valoreLimite: un valore dei limiti, con la sua chiave nell'indice e il suo tetto.
type valoreLimite struct {
	percorso      string
	valore, tetto int
}

// valori: i valori dei limiti nell'ordine dell'indice, così le diagnostiche hanno un ordine fisso.
func (l Limiti) valori() []valoreLimite {
	t := TettiLimiti()
	return []valoreLimite{
		{"limiti.grammatica.max_famiglie", l.Grammatica.MaxFamiglie, t.Grammatica.MaxFamiglie},
		{"limiti.grammatica.max_forme_per_famiglia", l.Grammatica.MaxFormePerFamiglia, t.Grammatica.MaxFormePerFamiglia},
		{"limiti.grammatica.max_parti_per_forma", l.Grammatica.MaxPartiPerForma, t.Grammatica.MaxPartiPerForma},
		{"limiti.grammatica.max_esempi", l.Grammatica.MaxEsempi, t.Grammatica.MaxEsempi},
		{"limiti.grammatica.max_lunghezza_pattern", l.Grammatica.MaxLunghezzaPattern, t.Grammatica.MaxLunghezzaPattern},
		{"limiti.grammatica.max_ripetizione", l.Grammatica.MaxRipetizione, t.Grammatica.MaxRipetizione},
		{"limiti.grammatica.max_lunghezza_letterale", l.Grammatica.MaxLunghezzaLetterale, t.Grammatica.MaxLunghezzaLetterale},
		{"limiti.riconoscimento.max_byte_unita", l.Riconoscimento.MaxByteUnita, t.Riconoscimento.MaxByteUnita},
		{"limiti.riconoscimento.max_unita_documento", l.Riconoscimento.MaxUnitaDocumento, t.Riconoscimento.MaxUnitaDocumento},
		{"limiti.riconoscimento.max_letture_per_unita", l.Riconoscimento.MaxLetturePerUnita, t.Riconoscimento.MaxLetturePerUnita},
		{"limiti.riconoscimento.max_letture_documento", l.Riconoscimento.MaxLettureDocumento, t.Riconoscimento.MaxLettureDocumento},
		{"limiti.anteprima.tempo_massimo_ms", l.Anteprima.TempoMassimoMs, t.Anteprima.TempoMassimoMs},
	}
}

// Valida rifiuta limiti assenti, nulli o oltre i tetti: con un indice così non si attiva nessuna grammatica
// (R43 B). Codici: contratto.campo_obbligatorio (i limiti mancano, manca la versione, un valore è zero o
// negativo), limite.oltre_tetto (un valore supera il tetto). I percorsi sono quelli dell'indice («limiti.…»).
// nil vuol dire validi.
func (l Limiti) Valida() []evidenze.Diagnostica {
	if l == (Limiti{}) {
		return []evidenze.Diagnostica{{
			Codice:    CodiceCampoObbligatorio,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  "limiti",
			Messaggio: "l'indice non dichiara i limiti: il codice non ha valori suoi, e senza limiti nessuna grammatica si attiva (R43 B)",
		}}
	}
	var out []evidenze.Diagnostica
	if l.Versione == "" {
		out = append(out, evidenze.Diagnostica{
			Codice:    CodiceCampoObbligatorio,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  "limiti.versione_limiti",
			Messaggio: "manca la versione dei limiti: entra nell'impronta dei risultati e cambia a ogni cambio di un valore",
		})
	}
	for _, v := range l.valori() {
		switch {
		case v.valore <= 0:
			out = append(out, evidenze.Diagnostica{
				Codice:    CodiceCampoObbligatorio,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  v.percorso,
				Messaggio: fmt.Sprintf("valore %d: un limite manca, è zero o è negativo", v.valore),
			})
		case v.valore > v.tetto:
			out = append(out, evidenze.Diagnostica{
				Codice:    CodiceLimiteOltreTetto,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  v.percorso,
				Messaggio: fmt.Sprintf("valore %d oltre il tetto del codice, %d: un tetto si alza solo con un commit che lo dichiara", v.valore, v.tetto),
			})
		}
	}
	return out
}
