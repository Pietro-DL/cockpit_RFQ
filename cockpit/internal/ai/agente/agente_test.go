package agente

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// L1 — checkpoint 3R §9. L'agente propone, e il server controlla che ciò che propone esista davvero.
//
// Queste prove non chiamano nessun servizio esterno e non hanno bisogno di una chiave: il modello è
// un'interfaccia, e ciò che qui viene provato — lo schema, il grounding, l'idempotenza — è tutto
// codice nostro. È anche il motivo per cui il grounding è in Go: una regola scritta nel prompt si
// può provare solo pagando, e un controllo che si prova solo pagando non si prova.

type modelloFinto struct {
	risposta string
	err      error
	sistema  string
	utente   string
}

func (m *modelloFinto) Chiedi(_ context.Context, sistema, utente string) ([]byte, int, int, error) {
	m.sistema, m.utente = sistema, utente
	return []byte(m.risposta), 100, 50, m.err
}
func (m *modelloFinto) Nome() string { return "finto" }

func contestoDiProva() Contesto {
	return Contesto{
		Oggetto:      "R: RICHIESTA OFFERTA RDO 490020618",
		Corpo:        "Buongiorno, vi confermiamo la richiesta per il codice 6743449A1. Manca il 3D.",
		Mittente:     "buyer@cliente.example",
		Cliente:      "Cliente Di Prova",
		NomiAllegati: []string{"6743449A_1.zip", "listino.xlsx"},
		Candidati: []RichiestaNota{
			{ThreadID: "11111111-1111-1111-1111-111111111111", Oggetto: "Pedale frizione", Perche: "R0"},
		},
		CodiciNoti: []string{"6743449A1"},
	}
}

// Uno schema sbagliato viene RIFIUTATO, senza effetti parziali.
func TestLoSchemaRifiutaCioCheIlSistemaNonSaLeggere(t *testing.T) {
	casi := map[string]string{
		"intento inventato":   `{"intento":"forse","evidenze":[]}`,
		"campo che non c'è":   `{"intento":"nuova_rfq","evidenze":[],"azione":"aggancia"}`,
		"ruolo inventato":     `{"intento":"nuova_rfq","evidenze":[],"codici":[{"codice":"X12345","ruolo":"finito"}]}`,
		"tipo allegato falso": `{"intento":"nuova_rfq","evidenze":[],"allegati":[{"nome":"a.pdf","tipo":"disegno_3d"}]}`,
		"punteggio fuori":     `{"intento":"nuova_rfq","evidenze":[],"candidati":[{"thread_id":"x","punteggio":180,"perche":""}]}`,
		"testo dopo il JSON":  `{"intento":"nuova_rfq","evidenze":[]} ecco fatto`,
		"non è JSON":          `mi dispiace, non posso aiutarti`,
	}
	for nome, raw := range casi {
		if _, err := Valida([]byte(raw)); err == nil {
			t.Errorf("%s: accettato, atteso rifiuto", nome)
		} else if !errors.Is(err, ErrSchema) {
			t.Errorf("%s: errore %v, atteso ErrSchema", nome, err)
		}
	}
	// e una risposta buona passa intera
	buona := `{"intento":"risposta_rfq","riferimento_rfq":"RDO 490020618","evidenze":["R: nell'oggetto"],
	           "codici":[{"codice":"6743449A1","ruolo":"prodotto","dove":"corpo"}],
	           "cosa_manca":["manca il 3D"]}`
	p, err := Valida([]byte(buona))
	if err != nil {
		t.Fatalf("una risposta conforme è stata rifiutata: %v", err)
	}
	if p.Intento != IntentoRispostaRFQ || len(p.Codici) != 1 || p.Riferimento != "RDO 490020618" {
		t.Errorf("risposta letta male: %+v", p)
	}
}

// Il grounding: ciò che l'agente cita e nel messaggio non c'è viene tolto, con il motivo.
func TestIlGroundingScartaCioCheNonEsiste(t *testing.T) {
	c := contestoDiProva()
	p := Proposta{
		Intento:     IntentoRispostaRFQ,
		Riferimento: "RDO 999999999", // non c'è
		Codici: []CodiceAI{
			{Codice: "6743449A1", Ruolo: "prodotto"}, // c'è
			{Codice: "AB99999", Ruolo: "prodotto"},   // inventato
		},
		Candidati: []CandidatoAI{
			{ThreadID: "11111111-1111-1111-1111-111111111111", Punteggio: 90},
			{ThreadID: "22222222-2222-2222-2222-222222222222", Punteggio: 95}, // mai passato
		},
		Allegati: []AllegatoAI{
			{Nome: "6743449A_1.zip", Tipo: "altro"},
			{Nome: "disegno_che_non_esiste.pdf", Tipo: "da_determinare"},
		},
		Evidenze: []string{"x"},
	}
	out, scarti := Verifica(p, c)
	if out.Riferimento != "" {
		t.Errorf("un riferimento inventato è passato: %q", out.Riferimento)
	}
	if len(out.Codici) != 1 || out.Codici[0].Codice != "6743449A1" {
		t.Errorf("codici dopo il grounding: %+v", out.Codici)
	}
	if len(out.Candidati) != 1 || out.Candidati[0].ThreadID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("un candidato non passato nel contesto è sopravvissuto: %+v", out.Candidati)
	}
	if len(out.Allegati) != 1 || out.Allegati[0].Nome != "6743449A_1.zip" {
		t.Errorf("un allegato inesistente è sopravvissuto: %+v", out.Allegati)
	}
	if len(scarti) != 4 {
		t.Errorf("scarti registrati: %d (attesi 4)\n%+v", len(scarti), scarti)
	}
	for _, s := range scarti {
		if s.Motivo == "" || s.Campo == "" {
			t.Errorf("uno scarto senza motivo non serve a niente: %+v", s)
		}
	}
}

// Una bozza che cita un codice che non esiste cade INTERA. Una risposta al cliente con un part
// number sbagliato è un danno vero, e non si corregge a metà.
func TestUnaBozzaConUnCodiceInventatoCadeIntera(t *testing.T) {
	c := contestoDiProva()
	p := Proposta{Intento: IntentoRispostaRFQ, Evidenze: []string{"x"},
		Bozza: "Buongiorno, confermiamo la quotazione del codice 6743449A1 e del codice 9988776."}
	out, scarti := Verifica(p, c)
	if out.Bozza != "" {
		t.Error("la bozza con un codice inventato è arrivata all'operatore")
	}
	if len(scarti) != 1 || scarti[0].Campo != "bozza_risposta" {
		t.Errorf("scarti: %+v", scarti)
	}
	// una bozza che cita solo codici veri resta
	p.Bozza = "Buongiorno, confermiamo la quotazione del codice 6743449A1. Ci manca il 3D."
	out, scarti = Verifica(p, c)
	if out.Bozza == "" {
		t.Error("una bozza corretta è stata buttata")
	}
	if len(scarti) != 0 {
		t.Errorf("scarti su una bozza corretta: %+v", scarti)
	}
}

// Un riferimento non diventa un codice prodotto nemmeno se lo dice l'agente (§4).
func TestIlRiferimentoNonDiventaCodiceNemmenoDallAgente(t *testing.T) {
	c := contestoDiProva()
	p := Proposta{Intento: IntentoRispostaRFQ, Riferimento: "RDO 490020618", Evidenze: []string{"x"},
		Codici: []CodiceAI{{Codice: "490020618", Ruolo: "prodotto"}}}
	out, scarti := Verifica(p, c)
	for _, k := range out.Codici {
		if k.Codice == "490020618" && k.Ruolo == "prodotto" {
			t.Error("il numero della RDO è passato come codice prodotto")
		}
	}
	if len(scarti) == 0 {
		t.Error("lo scarto deve essere registrato: è la misura di quanto il modello inventa")
	}
}

// Analizza: il giro completo, con un modello finto. Nessuna rete, nessuna chiave.
func TestAnalizzaFaIlGiroCompleto(t *testing.T) {
	c := contestoDiProva()
	m := &modelloFinto{risposta: "```json\n" + `{"intento":"risposta_rfq","evidenze":["R: nell'oggetto"],
		"codici":[{"codice":"6743449A1","ruolo":"prodotto"},{"codice":"ZZ11111","ruolo":"prodotto"}],
		"cosa_manca":["manca il 3D"]}` + "\n```"}
	e := Analizza(context.Background(), m, c)
	if e.Errore != nil {
		t.Fatalf("errore: %v", e.Errore)
	}
	if e.Proposta.Intento != IntentoRispostaRFQ {
		t.Errorf("intento %q", e.Proposta.Intento)
	}
	if len(e.Proposta.Codici) != 1 {
		t.Errorf("il grounding non ha tolto il codice inventato: %+v", e.Proposta.Codici)
	}
	if len(e.Scarti) != 1 {
		t.Errorf("scarti: %+v", e.Scarti)
	}
	if len(e.Grezzo) == 0 {
		t.Error("il grezzo si conserva sempre: è la misura di quanto il modello inventa")
	}
	if e.TokenIn == 0 || e.TokenOut == 0 {
		t.Error("i token non sono stati registrati: senza, «costa poco» resta un'impressione")
	}
	// ciò che è stato mandato è SOLO ciò che il contesto contiene
	if strings.Contains(m.utente, "listino.xlsx") == false {
		t.Error("i nomi degli allegati devono essere nel prompt")
	}
	for _, vietato := range []string{"password", "PDF:", "base64"} {
		if strings.Contains(m.utente, vietato) {
			t.Errorf("il prompt contiene %q: esce solo testo del messaggio", vietato)
		}
	}
}

// Un errore di rete non produce effetti: l'analisi risulta fallita e basta.
func TestUnErroreDiReteNonProduceProposte(t *testing.T) {
	m := &modelloFinto{err: errors.New("connessione rifiutata")}
	e := Analizza(context.Background(), m, contestoDiProva())
	if e.Errore == nil {
		t.Fatal("errore perso per strada")
	}
	if e.Proposta.Intento != "" || len(e.Proposta.Codici) != 0 {
		t.Errorf("proposta parziale dopo un errore: %+v", e.Proposta)
	}
}

// L'impronta dell'input è la chiave dell'idempotenza: stesso contesto, stessa impronta; un carattere
// diverso, impronta diversa.
func TestLImprontaDistingueGliInput(t *testing.T) {
	a := contestoDiProva()
	b := contestoDiProva()
	if a.Impronta() != b.Impronta() {
		t.Error("due contesti uguali danno impronte diverse: la rianalisi ripagherebbe ogni volta")
	}
	b.Corpo += "."
	if a.Impronta() == b.Impronta() {
		t.Error("due contesti diversi danno la stessa impronta: un'analisi vecchia verrebbe riusata")
	}
	if len(a.Impronta()) != 64 {
		t.Errorf("impronta di %d caratteri, attesi 64 (sha256)", len(a.Impronta()))
	}
}

// Lo schema che il prompt promette e quello che il codice legge devono essere lo stesso: i nomi dei
// campi JSON compaiono tutti nelle istruzioni.
func TestIlPromptDescriveLoSchemaCheIlCodiceLegge(t *testing.T) {
	b, err := json.Marshal(Proposta{Intento: IntentoIncerto, Evidenze: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	_ = b
	campi := []string{"intento", "riferimento_rfq", "candidati", "codici", "allegati", "evidenze",
		"cosa_manca", "bozza_risposta"}
	for _, c := range campi {
		if !strings.Contains(istruzioni, c) {
			t.Errorf("il prompt non nomina il campo %q: il modello non può indovinarlo", c)
		}
	}
	for _, i := range []string{IntentoNuovaRFQ, IntentoRispostaRFQ, IntentoDocumenti, IntentoNonRFQ, IntentoIncerto} {
		if !strings.Contains(istruzioni, i) {
			t.Errorf("il prompt non elenca l'intento %q", i)
		}
	}
}
