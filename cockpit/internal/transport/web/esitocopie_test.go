// L1 — blocco 5A: «Riprova copie» deve RACCONTARE che cosa e' successo a ciascun documento.
//
// Quattro numeri e non uno: accodati, gia' in coda, non accodati, gia' sul NAS. Prima ce n'erano due,
// e i due mancanti erano esattamente i casi in cui l'operatore resta senza informazioni: un documento
// che non si e' potuto accodare usciva come errore 500 dell'intera azione (e le altre copie non
// partivano), e i documenti gia' sul NAS sparivano dal conto, cosi' «Nessuna copia nuova» poteva
// voler dire tanto «e' tutto a posto» quanto «non ho guardato niente».
//
// Sta a L1 perche' e' una funzione pura su quattro interi: non serve un database per sapere se una
// frase dice la verita'.
package web

import (
	"strings"
	"testing"
)

func TestFraseDelleCopieDiceQualCosaEeSuccesso(t *testing.T) {
	casi := []struct {
		nome    string
		e       esitoCopie
		attesi  []string
		assenti []string
	}{
		{"capacita' spenta: nomina la riga del file da cambiare",
			esitoCopie{Spenta: "nas_scrittura"},
			[]string{"NON rimesse in coda", "[sicurezza].nas_scrittura"}, []string{"rimesse in coda."}},
		{"due accodate e basta",
			esitoCopie{Accodati: 2},
			[]string{"2 copie rimesse in coda."}, []string{"gia'"}},
		{"una sola: singolare",
			esitoCopie{Accodati: 1},
			[]string{"1 copia rimessa in coda."}, nil},
		{"secondo clic: nessuna nuova, e lo dice",
			esitoCopie{GiaInCoda: 2},
			[]string{"Nessuna copia nuova", "2 gia' in attesa"}, nil},
		{"tutte gia' sul NAS: il numero c'e'",
			esitoCopie{NonApplicabili: 3},
			[]string{"Nessun documento in attesa", "gia' sul NAS (3)"}, nil},
		{"nessun documento confermato non e' la stessa cosa di tutti gia' copiati",
			esitoCopie{},
			[]string{"non ha documenti confermati"}, []string{"gia' sul NAS"}},
		{"un documento non accodato non fa sparire gli altri, e porta il motivo",
			esitoCopie{Accodati: 2, Errori: 1, Motivo: "connessione chiusa dal database"},
			[]string{"2 copie rimesse in coda", "1 non accodato", "connessione chiusa dal database"}, nil},
		{"tutti e quattro insieme",
			esitoCopie{Accodati: 1, GiaInCoda: 2, Errori: 3, NonApplicabili: 4, Motivo: "boom"},
			[]string{"1 copia rimessa in coda", "2 gia' in attesa", "3 non accodati", "4 gia' sul NAS"}, nil},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			f := c.e.Frase()
			for _, a := range c.attesi {
				if !strings.Contains(f, a) {
					t.Errorf("la frase non dice %q:\n  %s", a, f)
				}
			}
			for _, a := range c.assenti {
				if strings.Contains(f, a) {
					t.Errorf("la frase dice %q e non dovrebbe:\n  %s", a, f)
				}
			}
		})
	}
}

// Un errore su un documento non deve poter cancellare gli altri numeri: e' il caso che prima non
// arrivava nemmeno in pagina, perche' l'azione usciva con un 500 e la RFQ restava com'era.
func TestUnErroreNonCancellaLeCopiePartite(t *testing.T) {
	f := esitoCopie{Accodati: 5, Errori: 1, Motivo: "x"}.Frase()
	if !strings.Contains(f, "5 copie rimesse in coda") {
		t.Errorf("le cinque copie accodate non compaiono piu':\n  %s", f)
	}
}
