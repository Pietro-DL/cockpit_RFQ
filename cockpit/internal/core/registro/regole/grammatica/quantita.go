package grammatica

import (
	"errors"
	"fmt"
	"sort"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// RegolaQuantitaPrimaRigaSopra: l'unica regola di QuantitaTabellare in A1. L'intestazione è la prima riga non
// vuota sopra la prima riga della tabella che ha una lettura di codice; la colonna è quella della cella
// d'intestazione che vale uno dei letterali (5.4.6 punto 12). Che cosa fare con la regola lo dice motorea: qui
// c'è solo il nome, da un elenco chiuso.
const RegolaQuantitaPrimaRigaSopra = "prima_riga_non_vuota_sopra_le_righe_con_codice"

// regoleQuantita: l'elenco chiuso delle regole di QuantitaTabellare.
var regoleQuantita = []string{RegolaQuantitaPrimaRigaSopra}

// QuantitaTabellare: come si riconosce, nelle tabelle della mail, la colonna della quantità richiesta. La
// dichiara il file regole del cliente, con i letterali osservati: il codice non ne inventa (v3 D10; R28 a). Una
// regola senza fixture resta riservata (v3 §2). Il DTO del par.3.3.3 non aveva il campo che R28 (a) chiede:
// nasce qui, in A1b.8 [+3].
//
// La regola autorizza solo il legame fra una cella e la sua riga: la cella della colonna dichiarata dà
// l'attributo «quantità» della riga che la contiene. Chi lega quantità e prodotto attraverso la riga è A1c.
type QuantitaTabellare struct {
	ID           string   `json:"id"`
	Intestazioni []string `json:"intestazioni"` // letterali della cella d'intestazione, confrontati con la cella dopo TrimSpace
	Regola       string   `json:"regola"`       // RegolaQuantitaPrimaRigaSopra: l'unica in A1
	Selettori    []string `json:"selettori"`    // corpo, storia: il selettore delle celle della tabella
	Stato        string   `json:"stato"`        // attiva | riservata
}

// quantita controlla le regole quantita_tabellare (5.4.7), con i codici che la grammatica dichiara già in A1a:
//   - ID non vuoti e unici (contratto.id_vuoto, contratto.id_ripetuto);
//   - almeno un'intestazione (contratto.campo_obbligatorio); ognuna non vuota, senza caratteri di controllo o di
//     formato, entro la lunghezza massima dei letterali, senza duplicati (grammatica.letterale_vuoto,
//     grammatica.carattere_di_controllo, limite.superato, contratto.insieme_con_duplicati);
//   - una regola dell'elenco chiuso (contratto.enum_ignoto);
//   - almeno un selettore (contratto.campo_obbligatorio); ognuno letto con evidenze.LeggiSelettore, che dà
//     contratto.selettore_non_ammesso o contratto.selettore_generico, passati avanti con il percorso nella
//     grammatica; fra le coppie valide sono ammessi solo corpo e storia, cioè i selettori che le celle di una
//     tabella della mail possono avere (contratto.enum_ignoto); senza duplicati;
//   - lo stato: attiva | riservata (contratto.enum_ignoto).
//
// I selettori restano stringhe: della foglia si usa solo LeggiSelettore, già nell'elenco chiuso di G8 (R41 a).
func (v *validatore) quantita(qs []QuantitaTabellare) {
	const base = "quantita_tabellare"
	ids := make([]string, len(qs))
	for i := range qs {
		ids[i] = qs[i].ID
	}
	v.ids(base, ids)
	for i, q := range qs {
		p := elemento(base, q.ID, i)
		if len(q.Intestazioni) == 0 {
			v.obbligatorio(p+".intestazioni", "almeno un letterale della cella d'intestazione")
		}
		v.letterali(p+".intestazioni", q.Intestazioni)
		v.enum(p+".regola", "regola della quantità", q.Regola, regoleQuantita)
		if len(q.Selettori) == 0 {
			v.obbligatorio(p+".selettori", "l'elenco dei selettori")
		}
		for j, s := range q.Selettori {
			v.selettoreQuantita(fmt.Sprintf("%s.selettori[%d]", p, j), s)
		}
		v.duplicati(p+".selettori", q.Selettori)
		v.enum(p+".stato", "stato della regola della quantità", q.Stato, stati)
	}
}

// selettoreQuantita: un selettore di quantita_tabellare, letto con evidenze.LeggiSelettore. Le sue diagnostiche
// passano avanti con il percorso nella grammatica, senza ridichiararle (R41 b); una coppia valida diversa da
// corpo e storia non è ammessa qui.
func (v *validatore) selettoreQuantita(percorso, s string) {
	sel, err := evidenze.LeggiSelettore(s)
	if err != nil {
		var ec *evidenze.ErroreContratto
		if errors.As(err, &ec) {
			for _, d := range ec.Diagnostiche {
				d.Percorso = percorso
				v.aggiungi(d)
			}
		}
		return
	}
	if sel.Contesto != evidenze.ContestoCorpo && sel.Contesto != evidenze.ContestoStoria {
		v.nonAmmesso(percorso, fmt.Sprintf("selettore %q: una quantità tabellare si dichiara solo su corpo e storia, i selettori delle celle di una tabella della mail", s))
	}
}

// normalizzaQuantita: le regole quantita_tabellare in ordine di ID, con intestazioni e selettori come insiemi
// in ordine di byte; nil se non ce ne sono, così un file che non usa il campo ha lo stesso canonico e lo stesso
// hash di prima (omitempty; par.3.4.3).
func normalizzaQuantita(qs []QuantitaTabellare) []QuantitaTabellare {
	var out []QuantitaTabellare
	for _, q := range qs {
		q.Intestazioni = insieme(q.Intestazioni)
		q.Selettori = insieme(q.Selettori)
		out = append(out, q)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
