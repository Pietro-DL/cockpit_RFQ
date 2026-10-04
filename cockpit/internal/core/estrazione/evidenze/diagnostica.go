package evidenze

import "strings"

// ---- il tipo delle diagnostiche (R41 b) ----

// Diagnostica: un problema, con un codice stabile (parte 1 §7.3). È l'unico tipo di diagnostica del motore A:
// lo usano grammatiche, adattatori, motore, proposte, fotografia, valutazione e confronto, e QualitaFonte.
//
// I codici non stanno qui. Ogni pacchetto del motore A dichiara in un suo file (codici_diagnostica.go) i
// codici che produce: una costante «<area>.<caso>» in minuscolo, con natura, gravità predefinita e
// significato nel commento. L'area dice di che cosa parla il codice, non quale pacchetto lo dichiara.
// Un codice pubblicato non cambia significato; se non serve più resta dichiarato, con il commento «ritirato».
// Una diagnostica si costruisce sempre con una di quelle costanti, mai con una stringa scritta sul posto: una
// prova del QA raccoglie i codici di tutti i pacchetti del motore A e controlla che siano unici, nel formato
// giusto e stabili (parte 1 §7.3: «codice stabile»). Niente catalogo unico in questo pacchetto (R41 b = B).
type Diagnostica struct {
	Codice    string   `json:"codice"`             // «<area>.<caso>», dichiarato da chi lo produce
	Gravita   Gravita  `json:"gravita"`            // errore | avviso | nota
	Natura    Natura   `json:"natura"`             // contratto | dati | capacita | limite
	Percorso  string   `json:"percorso,omitempty"` // il campo interessato: "famiglie_codice[acme].forme[nome].parti[2]"
	Messaggio string   `json:"messaggio"`          // in italiano; senza testi del cliente se finisce nel log
	Rif       []string `json:"rif,omitempty"`      // gli ID di fonte, unità, regola o lettura a cui si riferisce
}

// Gravita:
//   - errore: blocca l'attivazione o il risultato operativo;
//   - avviso: il risultato c'è, ma va guardato;
//   - nota: un'informazione.
type Gravita string

const (
	GravitaErrore Gravita = "errore"
	GravitaAvviso Gravita = "avviso"
	GravitaNota   Gravita = "nota"
)

// Natura (parte 1 §7.3: «distinguere errore di contratto e limite dei dati»):
//   - contratto: l'ingresso non rispetta il contratto;
//   - dati: i dati non bastano (PDF senza testo, grafo incompleto);
//   - capacita: un campo senza fixture o una capacità non supportata (v3 §2);
//   - limite: una soglia di elaborazione superata (parte 1 §7.5).
//
// Per ciò che il motore non sa non ci sono altri stati né altri enum: un significato riservato, un campo che
// l'adattatore non porta, un valore inaffidabile, una fonte o un campo fuori vocabolario si dicono con una di
// queste quattro nature e con il codice «<area>.<caso>» di chi lo produce.
type Natura string

const (
	NaturaContratto Natura = "contratto"
	NaturaDati      Natura = "dati"
	NaturaCapacita  Natura = "capacita"
	NaturaLimite    Natura = "limite"
)

// ErroreContratto: l'errore restituito quando l'ingresso viola il contratto. Porta le diagnostiche.
type ErroreContratto struct{ Diagnostiche []Diagnostica }

// Error mette in fila codice, percorso e messaggio di ogni diagnostica: chi vuole i dati li prende da
// Diagnostiche, non dal testo.
func (e *ErroreContratto) Error() string {
	if e == nil || len(e.Diagnostiche) == 0 {
		return "errore di contratto"
	}
	parti := make([]string, 0, len(e.Diagnostiche))
	for _, d := range e.Diagnostiche {
		p := d.Codice
		if d.Percorso != "" {
			p += " [" + d.Percorso + "]"
		}
		if d.Messaggio != "" {
			p += ": " + d.Messaggio
		}
		parti = append(parti, p)
	}
	return "errore di contratto: " + strings.Join(parti, "; ")
}
