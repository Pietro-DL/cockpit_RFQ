package classificazione

import (
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

// L1 — l'evidenza viene prima delle parole e prima dei punti (revisione del 25/09).

// Una risposta del cliente dentro una nostra RFQ, con la firma che dice «newsletter» o una riga di
// cortesia con «fuori ufficio»: il candidato R0 a 98 dice che la richiesta esiste ed è aperta, e
// l'esito è «aggancia». Prima vinceva NonBusiness, che legge il testo e non i candidati: «ignora, non
// è posta di lavoro» su una risposta con In-Reply-To.
func TestLEvidenzaDiUnaRfqApertaVinceSulleParoleDiNonBusiness(t *testing.T) {
	casi := []struct{ nome, oggetto, corpo string }{
		{"newsletter nella firma", "R: RICHIESTA D'OFFERTA 1234567A",
			"Vi confermiamo la quantità di 200 pezzi.\n\nMario Rossi\nUfficio acquisti\nIscriviti alla nostra newsletter!"},
		{"fuori ufficio in una riga di cortesia", "R: RICHIESTA D'OFFERTA 1234567A",
			"Ecco il disegno aggiornato. Da lunedì sarò fuori ufficio, per urgenze scrivere al collega."},
		{"webinar nel corpo", "R: RFQ 1234567A", "Allego la rev. B. P.S. ci vediamo al webinar di giovedì."},
	}
	r0 := []Candidato{{ThreadID: "t-aperta", Regola: R0Reply, Punteggio: PuntiRegola[R0Reply], Evidenza: "In-Reply-To"}}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			in := IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
				Mittente: "acquisti@acme.example", Oggetto: c.oggetto, Corpo: c.corpo}
			// senza candidati le parole decidono ancora: è il comportamento che resta
			if e := Triage(in); e.Esito != "ignora" || e.Atto != AttoNonBusiness {
				t.Fatalf("senza evidenza il testo resta non di lavoro (altrimenti la prova non prova niente): %s %s %v", e.Esito, e.Atto, e.Motivi)
			}
			in.Candidati = r0
			e := Triage(in)
			if e.Esito != "aggancia" || e.Candidato == nil || e.Candidato.ThreadID != "t-aperta" {
				t.Fatalf("con R0 verso una RFQ aperta l'esito è aggancia, non %q (%v)", e.Esito, e.Motivi)
			}
			if e.Confidenza != PuntiRegola[R0Reply] || e.Legame != LegameRisposta || e.Atto == AttoNonBusiness {
				t.Errorf("confidenza %d, legame %q, atto %q", e.Confidenza, e.Legame, e.Atto)
			}
		})
	}
	// un indirizzo automatico con un indizio DEBOLE resta non di lavoro: R5 non è evidenza
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Mittente: "noreply@acme.example", Oggetto: "Portale fornitori: nuova notifica", Corpo: "Notifica automatica.",
		Candidati: []Candidato{{ThreadID: "t", Regola: R5Buyer, Punteggio: PuntiRegola[R5Buyer], Evidenza: "stesso buyer"}}})
	if e.Esito != "ignora" || e.Atto != AttoNonBusiness {
		t.Errorf("con il solo R5 decide ancora NonBusiness: %s %s", e.Esito, e.Atto)
	}
}

// Il candidato più forte è una RFQ CHIUSA, ma c'è anche una RFQ aperta sopra la soglia: l'esito è
// «aggancia» grazie a quella aperta, ed è quella che si propone. Prima si proponeva la chiusa — cioè si
// chiedeva all'operatore di confermare con un clic l'aggancio che T2 esclude.
func TestSeLaPiuForteEChiusaSiProponeLAperta(t *testing.T) {
	cand := []Candidato{
		{ThreadID: "t-chiusa", Regola: R0Reply, Punteggio: PuntiRegola[R0Reply], Evidenza: "In-Reply-To verso la chiusa", Chiuso: true},
		{ThreadID: "t-aperta", Regola: R3Codice, Punteggio: PuntiRegola[R3Codice], Evidenza: "il codice 1234567A"},
		{ThreadID: "t-debole", Regola: R5Buyer, Punteggio: PuntiRegola[R5Buyer], Evidenza: "stesso buyer"},
	}
	for _, controparte := range []string{"", ControparteCliente, ControparteFornitore, ControparteSconosciuto} {
		e := Triage(IngressoTriage{Direzione: "entrata", Controparte: controparte, ClienteNoto: controparte == ControparteCliente,
			Mittente: "acquisti@acme.example", Oggetto: "R: RFQ 1234567A", Corpo: "Allego la revisione B.", Candidati: cand})
		if e.Esito != "aggancia" {
			t.Errorf("controparte %q: esito %q, atteso aggancia (c'è una RFQ aperta sopra la soglia)", controparte, e.Esito)
			continue
		}
		if e.Candidato == nil || e.Candidato.ThreadID != "t-aperta" {
			t.Errorf("controparte %q: proposto %+v, attesa la RFQ aperta", controparte, e.Candidato)
		}
		if e.Confidenza != PuntiRegola[R3Codice] {
			t.Errorf("controparte %q: confidenza %d, attesa quella del candidato aperto %d", controparte, e.Confidenza, PuntiRegola[R3Codice])
		}
	}
	// nel ramo cliente la chiusa non sparisce dai motivi: si vede, e si dice perché non è la proposta
	e := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Oggetto: "R: RFQ 1234567A", Corpo: "Allego la revisione B.", Candidati: cand})
	if !contieneMotivo(e.Motivi, "CHIUSA") {
		t.Errorf("manca l'avviso sulla richiesta chiusa più simile: %v", e.Motivi)
	}
	// e con la sola chiusa vale T2: niente aggancio, la chiusa resta visibile
	solo := Triage(IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true,
		Oggetto: "R: RFQ 1234567A", Corpo: "Allego la revisione B.", Candidati: cand[:1]})
	if solo.Esito == "aggancia" || solo.Candidato == nil || solo.Candidato.ThreadID != "t-chiusa" {
		t.Errorf("con la sola RFQ chiusa: esito %q, candidato %+v", solo.Esito, solo.Candidato)
	}
	if k, ok := MiglioreCandidatoAperto(cand[:1]); ok {
		t.Errorf("MiglioreCandidatoAperto con la sola chiusa: %+v", k)
	}
}

// Il bonus della famiglia conta solo i codici PROPONIBILI. Un codice di famiglia che sta nella storia
// citata dice a quale richiesta si risponde: contarlo dava dieci punti di «richiesta nuova» proprio
// alle risposte, come i punti del riferimento, che per la storia erano già esclusi.
func TestIlBonusDellaFamigliaNonContaLaStoriaCitata(t *testing.T) {
	r, err := regole.ValidaRegole([]byte(`{"famiglie_codice":[{"regex":"\\b\\d{7}[A-Z]\\b","descrizione":"ACME sette cifre","esempio":"1234567A"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	mo := Compila("ACME", r)
	risposta := IngressoTriage{Direzione: "entrata", Controparte: ControparteCliente, ClienteNoto: true, Motore: mo,
		Oggetto: "R: conferma", Corpo: "Ricevuto, grazie.\n\nDa: Mario Rossi <mario.rossi@acme.example>\nInviato: giovedì 17 settembre 2026 09:12\nOggetto: offerta\n\nVi chiediamo quotazione per 1234567A."}
	e := Triage(risposta)
	if contieneMotivo(e.Motivi, "codici della famiglia") {
		t.Errorf("il codice della sola storia ha dato il bonus della famiglia: %v", e.Motivi)
	}
	// il codice si trova lo stesso: serve all'aggancio
	if len(e.Trovati) == 0 {
		t.Error("il codice della storia è sparito dai trovati: era l'evidenza per agganciare la risposta")
	}
	// lo stesso codice scritto ADESSO il bonus lo dà
	nuovo := risposta
	nuovo.Corpo = "Vi chiediamo quotazione per 1234567A."
	if e := Triage(nuovo); !contieneMotivo(e.Motivi, "codici della famiglia") {
		t.Errorf("un codice di famiglia nel corpo scritto adesso non ha dato il bonus: %v", e.Motivi)
	}
}
