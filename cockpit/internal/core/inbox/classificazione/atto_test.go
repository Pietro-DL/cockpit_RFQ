package classificazione

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

// Blocco 7B (7B.3) riletto con il vocabolario del 7C.0: l'atto e il legame del messaggio, per ramo.
// IB7 e IB8 a livello puro; le versioni con il database sono in web/richieste_db_test.go.

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
	if e.Atto != AttoNonBusiness {
		t.Fatalf("atto %q, atteso non_business: %v", e.Atto, e.Motivi)
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
	if e.Atto != AttoIncerto {
		t.Fatalf("atto %q, atteso incerto", e.Atto)
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
	if e.Esito != "nuova_rfq" || e.Atto != "" || e.Legame != "" {
		t.Fatalf("esito %s, atto %q, legame %q", e.Esito, e.Atto, e.Legame)
	}
}

func TestIlRamoFornitoreDistingueOffertaDomandaConfermaENonBusiness(t *testing.T) {
	casi := []struct {
		nome, mittente, oggetto, corpo string
		allegati                       []string
		atto                           string
	}{
		{"offerta con pdf", "info@minuterie-esempio.example", "R: RFQ ACME ROSSI 0X001234AB", "In allegato la nostra offerta per i particolari richiesti.", []string{"offerta_123.pdf"}, AttoOfferta},
		{"offerta senza allegato", "info@minuterie-esempio.example", "Quotazione", "Vi confermiamo il prezzo di 12,50 euro al pezzo.", nil, AttoOfferta},
		{"domanda", "info@minuterie-esempio.example", "R: RFQ ACME", "Quale materiale per il particolare 0X001234AB? Servirebbe lo spessore.", nil, AttoDomandaChiarimento},
		{"conferma di ricezione", "info@minuterie-esempio.example", "R: RFQ ACME", "Ricevuto, grazie. Vi rispondiamo entro venerdì.", nil, AttoConfermaRicezione},
		{"comunicazione generica", "info@minuterie-esempio.example", "R: RFQ ACME", "Cordiali saluti, il vostro referente cambia da lunedì.", nil, AttoComunicazioneGenerica},
		{"newsletter", "newsletter@minuterie-esempio.example", "Le nostre novità", "Iscriviti alla newsletter.", nil, AttoNonBusiness},
		{"fuori ufficio", "info@minuterie-esempio.example", "Risposta automatica: R: RFQ ACME", "Sono fuori ufficio fino a lunedì.", nil, AttoNonBusiness},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: c.mittente,
				Oggetto: c.oggetto, Corpo: c.corpo, NomiAllegati: c.allegati})
			if e.Atto != c.atto {
				t.Fatalf("atto %q, atteso %q: %v", e.Atto, c.atto, e.Motivi)
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
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@minuterie-esempio.example",
		Oggetto: "Offerta", Corpo: "Materiale S235JR, tolleranze ISO 2768-mK, viti DIN 933 M8x20. Offerta in allegato.",
		NomiAllegati: []string{"offerta.pdf"}})
	if len(e.Codici) != 0 || len(e.Trovati) != 0 {
		t.Fatalf("codici estratti da un fornitore senza famiglie: %v / %+v", e.Codici, e.Trovati)
	}
	// con la famiglia di un cliente che gli ha mandato richieste, il SUO codice si trova; S235JR no
	r := regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{Regex: `\b0[A-Z]\d{6}[A-Z]{2}\b`, Descrizione: "Beta Sport", Esempio: "0X001234AB"}}}
	e = Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@minuterie-esempio.example",
		Oggetto: "R: RFQ 0X001234AB", Corpo: "Materiale S235JR, ISO 2768. Offerta per 0X001234AB in allegato.",
		NomiAllegati: []string{"offerta.pdf"}, Motore: Compila("fornitore", r)})
	if len(e.Codici) != 1 || e.Codici[0] != "0X001234AB" {
		t.Fatalf("codici: %v, atteso il solo 0X001234AB", e.Codici)
	}
}

// Il ramo fornitore aggancia alla RICHIESTA prima che alla RFQ: un candidato richiesta vince.
func TestLaPostaDiUnFornitoreSiAgganciaAllaRichiesta(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@minuterie-esempio.example",
		Oggetto: "R: RFQ", Corpo: "offerta in allegato", NomiAllegati: []string{"offerta.pdf"},
		CandidatiRichiesta: []CandidatoRichiesta{{RichiestaID: "r1", Regola: RichiestaR3fCodice, Punteggio: 60, Evidenza: "codice"},
			{RichiestaID: "r2", Regola: RichiestaR0Reply, Punteggio: 95, Evidenza: "In-Reply-To"}}})
	if e.Esito != "aggancia" || e.CandidatoRichiesta == nil || e.CandidatoRichiesta.RichiestaID != "r2" || e.Confidenza != 95 {
		t.Fatalf("%+v", e)
	}
	// 7C.0: il legame dice «risposta a un oggetto esistente», l'atto dice «offerta». Sono due cose:
	// la richiesta cambia stato solo se una persona conferma l'atto.
	if e.Legame != LegameRisposta || e.Atto != AttoOfferta {
		t.Fatalf("legame %q / atto %q, attesi risposta / offerta", e.Legame, e.Atto)
	}
	// senza nessun candidato, la stessa offerta non ha legame
	e = Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteFornitore, Mittente: "info@minuterie-esempio.example",
		Oggetto: "Offerta", Corpo: "offerta in allegato", NomiAllegati: []string{"offerta.pdf"}})
	if e.Legame != LegameNessuno || e.Atto != AttoOfferta || e.Esito != "ignora" {
		t.Fatalf("%+v", e)
	}
}

// IB3 (puro): una nostra mail a un fornitore che cita una RFQ aperta propone la richiesta, non un thread.
func TestUnaNostraMailAUnFornitoreCheCitaUnaRfqProponeLaRichiesta(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "uscita", Controparte: ControparteFornitore, Mittente: "commerciale@azienda.example",
		Oggetto: "RFQ BETA SPORT ROSSI 0X001234AB", Corpo: "vi chiediamo offerta",
		RichiesteManuali: []Candidato{{ThreadID: "t1", Regola: RichiestaRFOggetto, Punteggio: 70, Evidenza: "il codice 0X001234AB è della RFQ"}}})
	if e.Esito != "aggancia" || e.Atto != AttoRichiestaOfferta || e.Legame != LegameNuovo || e.Candidato == nil || e.Candidato.ThreadID != "t1" {
		t.Fatalf("%+v", e)
	}
	// senza un codice che corrisponda, resta una nostra mail: atto noto, nessun legame, nessuna proposta
	e = Triage(IngressoTriage{Direzione: "uscita", Controparte: ControparteFornitore, Oggetto: "Buon anno"})
	if e.Esito != "ignora" || e.Atto != AttoRichiestaOfferta || e.Legame != LegameNessuno || e.Candidato != nil {
		t.Fatalf("%+v", e)
	}
	// e a un cliente e' la nostra offerta
	e = Triage(IngressoTriage{Direzione: "uscita", Controparte: ControparteCliente, Oggetto: "Offerta 123"})
	if e.Atto != AttoOfferta {
		t.Fatalf("%+v", e)
	}
}

// La posta «non di lavoro» di un cliente: atto non_business, nessuna proposta. Dal 7C.0 il
// quadrante non cambia (resta Clienti): la via giusta e' censire l'indirizzo automatico come Altro.
func TestUnClienteConUnaNewsletterENonBusinessSenzaProposta(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "noreply@acme.example", Oggetto: "Portale fornitori: nuova notifica", Corpo: "Do not reply to this message."})
	if e.Atto != AttoNonBusiness || e.Legame != LegameNessuno || e.Esito != "ignora" {
		t.Fatalf("%+v", e)
	}
	e = Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "acquisti@acme.example", Oggetto: "RICHIESTA D'OFFERTA", Corpo: "richiesta d'offerta in allegato", NomiAllegati: []string{"x.stp"}})
	if e.Atto != AttoRichiestaOfferta || e.Legame != LegameNuovo || e.Esito != "nuova_rfq" {
		t.Fatalf("%+v", e)
	}
}

// 7C.0: una risposta del cliente dentro una RFQ esistente ha legame «risposta», ma l'atto il
// deterministico non lo sa e lo dice: «incerto», non «richiesta d'offerta» come prima della 0016.
func TestUnaRispostaDelClienteHaIlLegameMaNonLAtto(t *testing.T) {
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "acquisti@acme.example", Oggetto: "R: RICHIESTA D'OFFERTA", Corpo: "Vi inviamo la rev. B.",
		Candidati: []Candidato{{ThreadID: "t1", Regola: "R0_reply", Punteggio: 95, Evidenza: "In-Reply-To"}}})
	if e.Esito != "aggancia" || e.Legame != LegameRisposta || e.Atto != AttoIncerto {
		t.Fatalf("%+v", e)
	}
	// e un collega che gira una mail fa un inoltro
	e = Triage(IngressoTriage{Direzione: "uscita", Interno: true, Controparte: ControparteInterno,
		Oggetto: "I: RICHIESTA D'OFFERTA", Corpo: "vi giro la richiesta d'offerta", NomiAllegati: []string{"pezzo.stp"}})
	if e.Atto != AttoInoltro || e.Legame != LegameNuovo || e.Esito != "nuova_rfq" {
		t.Fatalf("%+v", e)
	}
}
