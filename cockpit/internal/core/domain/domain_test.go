package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEstraiCodici(t *testing.T) {
	got := EstraiCodici("RICHIESTA D'OFFERTA 123456789 - codice 6674611A rev 4", "vedi allegato 6674611A_4.pdf e il 12/09/2026 alle 10:30")
	atteso := []string{"123456789", "6674611A", "6674611A_4"}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("EstraiCodici = %v, atteso %v", got, atteso)
	}
	c, r := CodiceRev("6674611A_4")
	if c != "6674611A" || r != "4" {
		t.Errorf("CodiceRev = %q %q", c, r)
	}
}

func TestRilevaPortale(t *testing.T) {
	corpo := "Buongiorno,\r\nvi abbiamo caricato sul portale i CAD dei codici 6674611A e 6674612B. Grazie.\r\nCordiali saluti"
	rif := RilevaPortale(corpo)
	if len(rif) != 1 || len(rif[0].Codici) != 2 {
		t.Fatalf("RilevaPortale = %+v", rif)
	}
}

func TestRilevaScadenza(t *testing.T) {
	rifer := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	d, ok := RilevaScadenza("Vi chiediamo cortesemente risposta entro il 15/09/2026.", rifer)
	if !ok || d.Day() != 15 || d.Month() != 9 {
		t.Errorf("RilevaScadenza = %v %v", d, ok)
	}
	if _, ok := RilevaScadenza("inviato il 01/09/2026", rifer); ok {
		t.Errorf("data senza parola chiave non deve essere una scadenza")
	}
}

func TestTriage(t *testing.T) {
	e := Triage(IngressoTriage{Oggetto: "RFQ 6674611A", Corpo: "in allegato i disegni", NomiAllegati: []string{"6674611A_4.pdf"}, BuyerNoto: true, Direzione: "entrata"})
	// 80 e non piu' 95: un PDF non vale piu' «allegato tecnico» (25) ma «allegato di tipo da
	// determinare» (10), perche' nessuno l'ha aperto. L'esito non cambia, ed e' l'esito che conta.
	if e.Esito != "nuova_rfq" || e.Confidenza < 70 {
		t.Errorf("triage RFQ = %+v", e)
	}
	e = Triage(IngressoTriage{Oggetto: "Newsletter settembre", Corpo: "offerte del mese", Direzione: "entrata"})
	if e.Esito != "ignora" {
		t.Errorf("triage newsletter = %+v", e)
	}
	e = Triage(IngressoTriage{Oggetto: "R: Tirocinio per tesi", Corpo: "mi ricordo bene, Bernardo", NomiAllegati: []string{"Screenshot_20260903_090239_Chrome.jpg"}, Direzione: "entrata"})
	if e.Esito != "ignora" || len(e.Codici) != 0 {
		t.Errorf("triage falso positivo rdo/screenshot = %+v", e)
	}
	e = Triage(IngressoTriage{Direzione: "uscita", Oggetto: "RFQ"})
	if e.Esito != "ignora" {
		t.Errorf("triage uscita = %+v", e)
	}

	// Voce 2.1, D10: una mail INTERNA è in uscita (parte da un nostro indirizzo) ma non è roba già
	// vista da noi: «ti giro questa richiesta» è uno dei modi in cui una RFQ arriva sul tavolo.
	// Ignorarla per il mittente vorrebbe dire non proporre niente proprio sui messaggi che un collega
	// ha inoltrato apposta perché qualcuno li guardasse.
	e = Triage(IngressoTriage{Direzione: "uscita", Interno: true, Oggetto: "I: RFQ 6674611A",
		Corpo: "ti giro la richiesta", NomiAllegati: []string{"6674611A_4.pdf"}})
	if e.Esito == "ignora" {
		t.Errorf("triage di una mail interna ignorato per il mittente: %+v", e)
	}
	var detto bool
	for _, m := range e.Motivi {
		if strings.Contains(m, "interna") {
			detto = true
		}
	}
	if !detto {
		t.Errorf("il triage non dice che si tratta di una mail interna: %+v", e.Motivi)
	}
}
