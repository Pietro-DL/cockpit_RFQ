package domain

import (
	"strings"
	"testing"
)

// Blocco 7B (7B.3): l'intento del messaggio, per ramo. IB7 e IB8 a livello puro; le versioni con
// il database sono in web/richieste_db_test.go.

func TestIB7UnaNewsletterDaUnDominioSconosciutoENonRfqSenzaProposta(t *testing.T) {
	e := Triage(IngressoTriage{
		// un mittente qualunque: la newsletter si riconosce dal TESTO, non solo dall'indirizzo automatico
		Direzione: "entrata", Controparte: ControparteSconosciuto, Mittente: "marketing@promo.example",
		Oggetto:      "RFQ facili con il nostro portale! Richiesta d'offerta in un clic",
		Corpo:        "Iscriviti al webinar. Per non ricevere più queste mail: unsubscribe.",
		NomiAllegati: []string{"brochure.pdf"},
	})
	if e.Esito == "nuova_rfq" {
		t.Fatalf("una newsletter ha prodotto nuova_rfq: %+v", e.Motivi)
	}
	if e.Intento != IntentoNonRfq {
		t.Fatalf("intento %q, atteso non_rfq: %v", e.Intento, e.Motivi)
	}
}

func TestUnoSconosciutoCheSembraUnaRfqEIncertoENonPropone(t *testing.T) {
	e := Triage(IngressoTriage{
		Direzione: "entrata", Controparte: ControparteSconosciuto, Mittente: "acquisti@nuovo.example",
		Oggetto: "RICHIESTA D'OFFERTA n. 12", Corpo: "Buongiorno, richiesta d'offerta per i particolari in allegato.",
		NomiAllegati: []string{"RDO.pdf", "pezzo.stp"},
	})
	if e.Esito == "nuova_rfq" {
		t.Fatalf("da un mittente non censito non nasce una proposta di RFQ (D34): %+v", e.Motivi)
	}
	if e.Intento != IntentoIncerto {
		t.Fatalf("intento %q, atteso incerto", e.Intento)
	}
	if !strings.Contains(strings.Join(e.Motivi, " "), "Censisci") {
		t.Fatalf("il motivo dice che cosa fare: %v", e.Motivi)
	}
}

// Senza controparte dichiarata (prove che non la conoscono, messaggi di prima della 0014) vale il
// comportamento di sempre: un testo da RFQ con allegati tecnici e' una RFQ nuova.
func TestSenzaControparteDichiarataIlTriageEQuelloDiSempre(t *testing.T) {
	e := Triage(IngressoTriage{
		Direzione: "entrata", Oggetto: "RICHIESTA D'OFFERTA n. 12", Corpo: "richiesta d'offerta per i particolari in allegato.",
		NomiAllegati: []string{"RDO.pdf", "pezzo.stp"},
	})
	if e.Esito != "nuova_rfq" || e.Intento != "" {
		t.Fatalf("esito %s, intento %q", e.Esito, e.Intento)
	}
}

func TestIlRamoFornitoreDistingueOffertaDomandaRispostaENonRfq(t *testing.T) {
	casi := []struct {
		nome, mittente, oggetto, corpo string
		allegati                       []string
		intento                        string
	}{
		{"offerta con pdf", "info@mgm.example", "R: RFQ ACME FIORE 0D002622AD", "In allegato la nostra offerta per i particolari richiesti.", []string{"offerta_123.pdf"}, IntentoOffertaFornitore},
		{"offerta senza allegato", "info@mgm.example", "Quotazione", "Vi confermiamo il prezzo di 12,50 euro al pezzo.", nil, IntentoOffertaFornitore},
		{"domanda", "info@mgm.example", "R: RFQ ACME", "Quale materiale per il particolare 0D002622AD? Servirebbe lo spessore.", nil, IntentoDomandaFornitore},
		{"risposta", "info@mgm.example", "R: RFQ ACME", "Ricevuto, grazie. Vi rispondiamo entro venerdì.", nil, IntentoRispostaFornitore},
		{"newsletter", "newsletter@mgm.example", "Le nostre novità", "Iscriviti alla newsletter.", nil, IntentoNonRfq},
		{"fuori ufficio", "info@mgm.example", "Risposta automatica: R: RFQ ACME", "Sono fuori ufficio fino a lunedì.", nil, IntentoNonRfq},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: c.mittente,
				Oggetto: c.oggetto, Corpo: c.corpo, NomiAllegati: c.allegati})
			if e.Intento != c.intento {
				t.Fatalf("intento %q, atteso %q: %v", e.Intento, c.intento, e.Motivi)
			}
			if e.Esito == "nuova_rfq" {
				t.Fatal("un fornitore non apre mai una RFQ")
			}
		})
	}
}

// IB8 (puro): nella posta di un fornitore un materiale, una norma e una vite non sono codici,
// nemmeno quando il fornitore non ha famiglie con cui leggerli.
func TestIB8MaterialiENormeNonSonoCodiciNellaPostaDiUnFornitore(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@mgm.example",
		Oggetto: "Offerta", Corpo: "Materiale S235JR, tolleranze ISO 2768-mK, viti DIN 933 M8x20. Offerta in allegato.",
		NomiAllegati: []string{"offerta.pdf"}})
	if len(e.Codici) != 0 || len(e.Trovati) != 0 {
		t.Fatalf("codici estratti da un fornitore senza famiglie: %v / %+v", e.Codici, e.Trovati)
	}
	// con la famiglia di un cliente che gli ha mandato richieste, il SUO codice si trova; S235JR no
	r := Regole{FamiglieCodice: []FamigliaCodice{{Regex: `\b0[A-Z]\d{6}[A-Z]{2}\b`, Descrizione: "Technogym", Esempio: "0D002622AD"}}}
	e = Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@mgm.example",
		Oggetto: "R: RFQ 0D002622AD", Corpo: "Materiale S235JR, ISO 2768. Offerta per 0D002622AD in allegato.",
		NomiAllegati: []string{"offerta.pdf"}, Motore: Compila("fornitore", r)})
	if len(e.Codici) != 1 || e.Codici[0] != "0D002622AD" {
		t.Fatalf("codici: %v, atteso il solo 0D002622AD", e.Codici)
	}
}

// Il ramo fornitore aggancia alla RICHIESTA prima che alla RFQ: un candidato richiesta vince.
func TestLaPostaDiUnFornitoreSiAgganciaAllaRichiesta(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@mgm.example",
		Oggetto: "R: RFQ", Corpo: "offerta in allegato", NomiAllegati: []string{"offerta.pdf"},
		CandidatiRichiesta: []CandidatoRichiesta{{RichiestaID: "r1", Regola: RichiestaR3fCodice, Punteggio: 60, Evidenza: "codice"},
			{RichiestaID: "r2", Regola: RichiestaR0Reply, Punteggio: 95, Evidenza: "In-Reply-To"}}})
	if e.Esito != "aggancia" || e.CandidatoRichiesta == nil || e.CandidatoRichiesta.RichiestaID != "r2" || e.Confidenza != 95 {
		t.Fatalf("%+v", e)
	}
}

// IB3 (puro): una nostra mail a un fornitore che cita una RFQ aperta propone la richiesta, non un thread.
func TestUnaNostraMailAUnFornitoreCheCitaUnaRfqProponeLaRichiesta(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "uscita", Controparte: ControparteFornitore, Mittente: "commerciale@azienda.example",
		Oggetto: "RFQ TECHNOGYM FIORE 0D002622AD", Corpo: "vi chiediamo offerta",
		RichiesteManuali: []Candidato{{ThreadID: "t1", Regola: RichiestaRFOggetto, Punteggio: 70, Evidenza: "il codice 0D002622AD è della RFQ"}}})
	if e.Esito != "aggancia" || e.Intento != IntentoRfqFornitore || e.Candidato == nil || e.Candidato.ThreadID != "t1" {
		t.Fatalf("%+v", e)
	}
	// senza un codice che corrisponda, resta una nostra mail: intento noto, nessuna proposta
	e = Triage(IngressoTriage{Direzione: "uscita", Controparte: ControparteFornitore, Oggetto: "Buon anno"})
	if e.Esito != "ignora" || e.Intento != IntentoRfqFornitore || e.Candidato != nil {
		t.Fatalf("%+v", e)
	}
	// e a un cliente e' la nostra offerta
	e = Triage(IngressoTriage{Direzione: "uscita", Controparte: ControparteCliente, Oggetto: "Offerta 123"})
	if e.Intento != IntentoOffertaPromatec {
		t.Fatalf("%+v", e)
	}
}

func TestUnClienteConUnaNewsletterVaInDaValidare(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "noreply@acme.example", Oggetto: "Portale fornitori: nuova notifica", Corpo: "Do not reply to this message."})
	if e.Intento != IntentoNonRfq || e.Esito != "ignora" {
		t.Fatalf("%+v", e)
	}
	e = Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "acquisti@acme.example", Oggetto: "RICHIESTA D'OFFERTA", Corpo: "richiesta d'offerta in allegato", NomiAllegati: []string{"x.stp"}})
	if e.Intento != IntentoRfqCliente || e.Esito != "nuova_rfq" {
		t.Fatalf("%+v", e)
	}
}
