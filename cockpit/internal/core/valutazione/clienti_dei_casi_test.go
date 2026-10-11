// L1 — il messaggio fuori RFQ di un caso senza thread e il suo cliente nella fotografia (D-Q10-4 = A ristretta, decisa
// con l'utente l'11/10): con il cliente del caso fra i clienti della fotografia, con la ragione sociale della grammatica,
// il messaggio si valuta; senza, resta non valutato con ragione_sociale_discorde, qualunque cosa dica il messaggio. È il
// comportamento che la correzione del caricatore usa, e non cambia: la prova lo fissa.
package valutazione_test

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

func TestDQ104IlClienteDelCasoNellaFotografia(t *testing.T) {
	m := uid(0x6f9)
	in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{{ID: "censimento-acme", ClienteID: clienteACME,
		Messaggi: []uuid.UUID{m}}}}
	f := fotografiaDi(scenaCollegamento(t))
	f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{messaggioFuori(m, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil)}
	e := calcola(t, f, insiemeACME(t), in)
	if len(e.FuoriRFQ) != 1 || !e.FuoriRFQ[0].Valutato || e.FuoriRFQ[0].Motivo != "" {
		t.Fatalf("con il cliente nella fotografia: %+v", e.FuoriRFQ)
	}

	f = fotografiaDi(scenaCollegamento(t))
	f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{messaggioFuori(m, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil)}
	var altri []fotorfq.Cliente
	for _, c := range f.Clienti {
		if c.ID != clienteACME {
			altri = append(altri, c)
		}
	}
	if len(altri) == len(f.Clienti) {
		t.Fatalf("la scena non ha il cliente ACME fra i clienti della fotografia")
	}
	f.Clienti = altri
	e = calcola(t, f, insiemeACME(t), in)
	if len(e.FuoriRFQ) != 1 || e.FuoriRFQ[0].Valutato || e.FuoriRFQ[0].Motivo != valutazione.MotivoThreadRagioneSocialeDiscorde {
		t.Fatalf("senza il cliente nella fotografia: %+v", e.FuoriRFQ)
	}
}
