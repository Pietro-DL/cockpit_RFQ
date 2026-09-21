package classificazione

import (
	"strings"
	"testing"
)

// CP1 (parte pura) — un fornitore in entrata NON puo' produrre `nuova_rfq`, per costruzione: non e'
// un punteggio da superare, e' un ramo che non conta i punti (blocco 7A, D33).

func ingressoDaFornitore() IngressoTriage {
	return IngressoTriage{
		Oggetto:      "RICHIESTA D'OFFERTA n. 77 - TG FIORE",
		Corpo:        "Richiesta d'offerta: quotazione per i codici 0.056.8238.3 e 0.056.8239.1, vedi disegni allegati. Preventivo urgente.",
		NomiAllegati: []string{"RDO_77.pdf", "0.056.8238.3.stp", "disegni.zip"},
		Direzione:    "entrata",
		Controparte:  ControparteFornitore,
	}
}

func TestCP1UnFornitoreInEntrataNonDiventaMaiUnaRFQ(t *testing.T) {
	// Con lo stesso messaggio, senza controparte, il triage propone una RFQ nuova: parole, allegati
	// tecnici, codici. E' il caso che ha aperto il blocco 7.
	senza := ingressoDaFornitore()
	senza.Controparte = ""
	if e := Triage(senza); e.Esito != "nuova_rfq" {
		t.Fatalf("senza la controparte il messaggio sembra una RFQ nuova (%s %d): altrimenti la prova non prova niente", e.Esito, e.Confidenza)
	}
	e := Triage(ingressoDaFornitore())
	if e.Esito == "nuova_rfq" {
		t.Fatalf("un fornitore in entrata ha prodotto nuova_rfq: %+v", e.Motivi)
	}
	if e.Esito != "ignora" {
		t.Fatalf("senza evidenza di una nostra richiesta non si propone niente: %s", e.Esito)
	}
	if !contieneFrase(e.Motivi, "fornitore") {
		t.Fatalf("il motivo dice perche': %v", e.Motivi)
	}
}

func TestCP1LOffertaDelFornitoreSiAggancia(t *testing.T) {
	in := ingressoDaFornitore()
	in.Candidati = []Candidato{{ThreadID: "t1", Regola: R0Reply, Punteggio: PuntiRegola[R0Reply], Evidenza: "In-Reply-To: <nostra-richiesta>"}}
	e := Triage(in)
	if e.Esito != "aggancia" || e.Candidato == nil || e.Candidato.ThreadID != "t1" {
		t.Fatalf("la risposta del fornitore alla nostra richiesta si propone in aggancio: %s %+v", e.Esito, e.Candidato)
	}
	// e con un indizio debole resta «ignora», mai «nuova_rfq»
	in.Candidati = []Candidato{{ThreadID: "t1", Regola: R5Buyer, Punteggio: PuntiRegola[R5Buyer], Evidenza: "stesso buyer"}}
	if e := Triage(in); e.Esito != "ignora" {
		t.Fatalf("indizio debole: %s", e.Esito)
	}
}

func TestUnaControparteAmbiguaNonProduceNessunaProposta(t *testing.T) {
	in := ingressoDaFornitore()
	in.Controparte = ControparteAmbiguo
	e := Triage(in)
	if e.Esito != "ignora" || e.Confidenza != 0 || !contieneFrase(e.Motivi, "ambigua") {
		t.Fatalf("%s %d %v", e.Esito, e.Confidenza, e.Motivi)
	}
}

// Un cliente riconosciuto dalla controparte si comporta come prima: la prova di regressione.
func TestUnClienteRestaUnClientePerIlTriage(t *testing.T) {
	in := ingressoDaFornitore()
	in.Controparte, in.ClienteNoto = ControparteCliente, true
	if e := Triage(in); e.Esito != "nuova_rfq" {
		t.Fatalf("%s %v", e.Esito, e.Motivi)
	}
}

func contieneFrase(motivi []string, pezzo string) bool {
	for _, m := range motivi {
		if strings.Contains(m, pezzo) {
			return true
		}
	}
	return false
}
