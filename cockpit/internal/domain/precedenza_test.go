package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// Le prove L1 del checkpoint 3R. Ognuna riproduce un sintomo visto sulla posta vera, non un caso
// costruito: il messaggio che diventava una richiesta nuova mentre era una risposta, il numero della
// RDO che diventava un codice prodotto, il CAP che diventava un identificativo, il PDF che diventava
// un disegno CAD prima che qualcuno lo aprisse.

// regoleDiProva sono le regole di un cliente inventato con due famiglie e un riferimento. I nomi non
// sono quelli di nessun cliente vero (vincolo del repository pubblico).
const regoleDiProva = `{
  "famiglie_codice": [
    {"regex": "\\b\\d{7}[A-Z]\\d?\\b", "descrizione": "7 cifre + lettera", "esempio": "6743449A"},
    {"regex": "\\b0\\.\\d{3}\\.\\d{4}\\.\\d\\b", "descrizione": "0.nnn.nnnn.n", "esempio": "0.056.8238.3"}
  ],
  "riferimento_rfq": {"regex": "\\bRDO\\s*\\d{9}\\b", "descrizione": "RDO a nove cifre", "esempio": "RDO 490020618"},
  "finestra_aggancio_gg": 45
}`

func motoreDiProva(t *testing.T) *Motore {
	t.Helper()
	r, err := ValidaRegole([]byte(regoleDiProva))
	if err != nil {
		t.Fatalf("le regole di prova non passano il loro stesso convalidatore: %v", err)
	}
	return Compila("Cliente Di Prova", r)
}

// T22 — una risposta a una richiesta che esiste non diventa MAI una richiesta nuova.
//
// Il sintomo, dal banco reale: «R: RICHIESTA OFFERTA COD 0.056.8238.3» con un PDF allegato faceva
// 35 (la parola «offerta») + 25 (l'allegato contato come «tecnico») = 60, superava la soglia di 50 e
// veniva proposto come `nuova_rfq`. Era la risposta di un fornitore a una richiesta nostra.
//
// Il difetto non era la soglia: era che il punteggio del CONTENUTO poteva sovrascrivere un esito
// deciso dall'EVIDENZA. Sono due cose di natura diversa, e una non può retrocedere l'altra.
func TestT22UnaRispostaNonDiventaUnaRichiestaNuova(t *testing.T) {
	// il contenuto, da solo, griderebbe «richiesta nuova»
	senza := Triage(IngressoTriage{
		Oggetto: "R: RICHIESTA OFFERTA COD 0.056.8238.3", Corpo: "Vi giro la richiesta d'offerta, in allegato i disegni.",
		NomiAllegati: []string{"0.056.8238.3.pdf", "disegni.zip"}, Direzione: "entrata", BuyerNoto: true,
	})
	if senza.Esito != "nuova_rfq" {
		t.Fatalf("senza candidati il contenuto deve pur proporre qualcosa: %+v", senza)
	}

	// con l'evidenza dell'header, lo stesso identico messaggio è una risposta
	for _, regola := range []string{R0Reply, R1Conversazione, R4Riferimento, R3Codice, R2Oggetto} {
		con := Triage(IngressoTriage{
			Oggetto: "R: RICHIESTA OFFERTA COD 0.056.8238.3", Corpo: "Vi giro la richiesta d'offerta, in allegato i disegni.",
			NomiAllegati: []string{"0.056.8238.3.pdf", "disegni.zip"}, Direzione: "entrata", BuyerNoto: true,
			Candidati: []Candidato{{ThreadID: "t1", Regola: regola, Punteggio: PuntiRegola[regola],
				Evidenza: "prova"}},
		})
		if con.Esito != "aggancia" {
			t.Errorf("%s: esito %q (atteso aggancia): il punteggio del contenuto ha retrocesso una risposta", regola, con.Esito)
		}
		if con.Confidenza != PuntiRegola[regola] {
			t.Errorf("%s: confidenza %d, attesa quella della regola %d", regola, con.Confidenza, PuntiRegola[regola])
		}
		if con.Candidato == nil || con.Candidato.ThreadID != "t1" {
			t.Errorf("%s: il candidato più forte non arriva alla schermata: %+v", regola, con.Candidato)
		}
	}
}

// R5 da sola («stesso buyer, di recente») NON impedisce di proporre una richiesta nuova: se bastasse,
// nessuna richiesta di un buyer conosciuto verrebbe mai proposta come nuova — cioè proprio quelle che
// arrivano tutti i giorni.
func TestUnIndizioDeboleNonImpedisceUnaRichiestaNuova(t *testing.T) {
	e := Triage(IngressoTriage{
		Oggetto: "Richiesta d'offerta pedale frizione", Corpo: "Siamo a richiedere offerta per il seguente nostro codice.",
		NomiAllegati: []string{"disegni.zip"}, Direzione: "entrata", BuyerNoto: true,
		Candidati: []Candidato{{ThreadID: "t1", Regola: R5Buyer, Punteggio: PuntiRegola[R5Buyer], Evidenza: "stesso buyer"}},
	})
	if e.Esito != "nuova_rfq" {
		t.Errorf("esito %q: R5 da sola non è evidenza che la richiesta esista già (%+v)", e.Esito, e.Motivi)
	}
	if e.Candidato == nil {
		t.Error("il candidato debole deve comunque arrivare alla schermata: si vede, non decide")
	}
}

// T2 — una richiesta CHIUSA si vede, ma non impedisce di proporne una nuova.
//
// In questo mestiere lo stesso particolare viene riquotato: stesso codice, due anni dopo, ed è una
// richiesta nuova. Quello che non deve succedere è che l'operatore non sappia che la precedente
// esiste, e ne faccia un doppione alla cieca.
func TestT2UnaRichiestaChiusaAvvisaMaNonDecide(t *testing.T) {
	e := Triage(IngressoTriage{
		Oggetto: "Richiesta d'offerta 6743449A", Corpo: "Siamo a richiedere offerta.", NomiAllegati: []string{"6743449A.stp"},
		Direzione: "entrata", BuyerNoto: true,
		Candidati: []Candidato{{ThreadID: "t1", Regola: R3Codice, Punteggio: PuntiRegola[R3Codice],
			Evidenza: "il codice 6743449A è già un identificativo di questa richiesta", Chiuso: true}},
	})
	if e.Esito != "nuova_rfq" {
		t.Errorf("esito %q: una richiesta chiusa è finita, e la nuova va proposta (%+v)", e.Esito, e.Motivi)
	}
	if !contieneMotivo(e.Motivi, "CHIUSA") {
		t.Errorf("manca l'avviso che ne esiste una chiusa: %v", e.Motivi)
	}
	if e.Candidato == nil || e.Candidato.ThreadID != "t1" {
		t.Error("il candidato chiuso deve restare visibile")
	}
	// ma se la stessa richiesta è APERTA, l'esito cambia
	aperta := Triage(IngressoTriage{
		Oggetto: "Richiesta d'offerta 6743449A", Corpo: "Siamo a richiedere offerta.", NomiAllegati: []string{"6743449A.stp"},
		Direzione: "entrata", BuyerNoto: true,
		Candidati: []Candidato{{ThreadID: "t1", Regola: R3Codice, Punteggio: PuntiRegola[R3Codice], Evidenza: "x"}},
	})
	if aperta.Esito != "aggancia" {
		t.Errorf("con la richiesta aperta l'esito deve essere aggancia, non %q", aperta.Esito)
	}
}

// I candidati si presentano TUTTI e in ordine: il più forte in cima, e a parità di punteggio vince la
// regola che viene prima nella precedenza. Sceglierne uno per l'operatore è ciò che questo checkpoint
// toglie (T16).
func TestICandidatiSiOrdinanoESiVedonoTutti(t *testing.T) {
	c := []Candidato{
		{ThreadID: "debole", Regola: R5Buyer, Punteggio: PuntiRegola[R5Buyer]},
		{ThreadID: "oggetto", Regola: R2Oggetto, Punteggio: PuntiRegola[R2Oggetto]},
		{ThreadID: "reply", Regola: R0Reply, Punteggio: PuntiRegola[R0Reply]},
		{ThreadID: "codice", Regola: R3Codice, Punteggio: PuntiRegola[R3Codice]},
	}
	o := OrdinaCandidati(c)
	if len(o) != 4 {
		t.Fatalf("l'ordinamento ha perso dei candidati: %d", len(o))
	}
	atteso := []string{"reply", "codice", "oggetto", "debole"}
	for i, a := range atteso {
		if o[i].ThreadID != a {
			t.Errorf("posizione %d: %s, atteso %s", i, o[i].ThreadID, a)
		}
	}
	k, ok := MiglioreCandidato(c)
	if !ok || k.ThreadID != "reply" {
		t.Errorf("MiglioreCandidato = %+v", k)
	}
	// a parità di punteggio decide la precedenza della regola, non l'ordine di inserimento
	pari := OrdinaCandidati([]Candidato{
		{ThreadID: "b", Regola: R3Codice, Punteggio: 80},
		{ThreadID: "a", Regola: R1Conversazione, Punteggio: 80},
	})
	if pari[0].ThreadID != "a" {
		t.Errorf("a parità di punteggio deve vincere la regola più forte, non la prima arrivata: %+v", pari)
	}
}

// AN6 — un riferimento della richiesta non diventa MAI un codice prodotto.
//
// «RICHIESTA D'OFFERTA 490020618» contiene un numero che l'estrattore generico chiamava codice. Non
// è un codice: è il numero della RDO, cioè il nome che il cliente dà alla richiesta. Finiva fra gli
// identificativi della RFQ, e di lì nel nome della cartella sul NAS e nelle ricerche per codice.
func TestAN6IlRiferimentoNonDiventaUnCodiceProdotto(t *testing.T) {
	m := motoreDiProva(t)
	e := m.Estrai(Testo{Dove: "oggetto", Corpo: "RICHIESTA D'OFFERTA RDO 490020618"},
		Testo{Dove: "corpo", Corpo: "Siamo a richiedere offerta per il nostro codice 6743449A1."})
	if e.Riferimento != "RDO 490020618" {
		t.Fatalf("riferimento non riconosciuto: %q", e.Riferimento)
	}
	for _, c := range e.Codici {
		if strings.Contains("RDO 490020618", c.Codice) {
			t.Errorf("il riferimento (o un suo pezzo) è finito fra i codici prodotto: %+v", c)
		}
	}
	// il codice vero invece c'è, con la sua famiglia
	trovato := false
	for _, c := range e.Codici {
		if c.Codice == "6743449A1" && c.Origine == "famiglia" && c.Ruolo == RuoloProdotto {
			trovato = true
		}
	}
	if !trovato {
		t.Errorf("il codice della famiglia non è stato riconosciuto: %+v", e.Codici)
	}
	// e il riferimento non entra fra ciò che si può spuntare come identificativo
	for _, c := range e.Proponibili() {
		if c.Ruolo == RuoloRiferimento {
			t.Errorf("un riferimento è proponibile come identificativo: %+v", c)
		}
	}
}

// AN7 — se il cliente ha famiglie dichiarate, l'estrattore generico non propone identificativi.
//
// Il sintomo: ogni numero con tre cifre diventava un «codice». Un CAP, una data compatta, un numero
// d'ordine, la partita IVA nel piè di pagina. E al submit del form diventavano identificativi
// confermati. I numeri restano visibili — sotto «altri numeri trovati» — ma non spuntati.
func TestAN7ConLeFamiglieIlGenericoNonPropone(t *testing.T) {
	m := motoreDiProva(t)
	e := m.Estrai(Testo{Dove: "oggetto", Corpo: "Offerta 6743449A1"},
		Testo{Dove: "corpo", Corpo: "Spett.le PROMATEC SRL, 61032 Fano PU, P.IVA 01234567890, tel 0721 123456."})
	if !e.HaFamiglie {
		t.Fatal("il motore di prova ha due famiglie")
	}
	for _, c := range e.Proponibili() {
		if c.Origine != "famiglia" {
			t.Errorf("con le famiglie dichiarate, il generico non deve essere proponibile: %+v", c)
		}
	}
	if len(e.Altri()) == 0 {
		t.Error("i numeri generici devono restare VISIBILI sotto «altri»: nasconderli sarebbe perdere informazione")
	}
	// senza famiglie (cliente non censito: il caso normale del primo giorno) il generico torna a valere
	var vuoto *Motore
	s := vuoto.Estrai(Testo{Dove: "corpo", Corpo: "il nostro codice 6743449A1"})
	if s.HaFamiglie {
		t.Error("un motore nil non ha famiglie")
	}
	if len(s.Proponibili()) == 0 {
		t.Error("senza famiglie il generico è tutto ciò che c'è, e deve essere proponibile")
	}
	if len(s.Altri()) != 0 {
		t.Error("senza famiglie non esiste la categoria «altri»: sarebbero tutti")
	}
}

// Il punteggio di un codice dice da dove viene. «Questo è un codice di questo cliente» e «questo ha
// la forma di un codice» non possono arrivare all'operatore con lo stesso numero accanto.
func TestLaProvenienzaDiUnCodiceHaUnPunteggioDiverso(t *testing.T) {
	m := motoreDiProva(t)
	e := m.Estrai(Testo{Dove: "oggetto", Corpo: "6743449A1 e il numero 987654321"})
	for _, c := range e.Codici {
		switch c.Origine {
		case "famiglia":
			if c.Punteggio != PuntiFamiglia || c.Famiglia == "" || c.Dove != "oggetto" {
				t.Errorf("famiglia: %+v", c)
			}
		case "generico":
			if c.Punteggio != PuntiGenerico || c.Ruolo != RuoloIgnoto {
				t.Errorf("generico: %+v", c)
			}
		}
	}
}

// Un PDF non è un disegno finché qualcuno non l'ha aperto (checkpoint 3R §5).
func TestUnPdfNonEUnDisegnoFinchePrimaDellAnalisi(t *testing.T) {
	casi := []struct{ nome, tipo string }{
		{"6674611A_4.pdf", "da_determinare"},
		{"capitolato.pdf", "da_determinare"},
		{"scansione.tif", "da_determinare"},
		{"6674611A.dwg", "disegno_2d"}, // un DWG è davvero un disegno CAD
		{"6674611A.stp", "cad_3d"},
		{"12-34567.dxf", "sviluppo_dxf"},
	}
	for _, c := range casi {
		if p := PropostaDaNome(c.nome, 300_000, "entrata"); p.Tipo != c.tipo {
			t.Errorf("%s: tipo %q, atteso %q", c.nome, p.Tipo, c.tipo)
		}
	}
	// e il punteggio del triage non conta più un PDF come «allegato tecnico»
	pdf := Triage(IngressoTriage{Oggetto: "documento", Corpo: "in allegato", NomiAllegati: []string{"x.pdf"}, Direzione: "entrata"})
	step := Triage(IngressoTriage{Oggetto: "documento", Corpo: "in allegato", NomiAllegati: []string{"x.stp"}, Direzione: "entrata"})
	if pdf.Confidenza >= step.Confidenza {
		t.Errorf("un PDF (%d) non può valere quanto uno STEP (%d) prima che qualcuno l'abbia aperto", pdf.Confidenza, step.Confidenza)
	}
	for _, m := range pdf.Motivi {
		if strings.Contains(m, "allegati tecnici") {
			t.Errorf("un PDF viene ancora contato come allegato tecnico: %v", pdf.Motivi)
		}
	}
}

// Le regole del cliente possono dichiarare che una famiglia è di sottoassiemi: è l'anagrafica a
// dirlo, non una regola scritta in Go su un cliente preciso.
func TestLoSchemaAccettaSoloIRuoliPrevisti(t *testing.T) {
	buono := `{"famiglie_codice":[{"regex":"\\bAC\\d{5}\\b","esempio":"AC12345","ruolo":"parte"}]}`
	r, err := ValidaRegole([]byte(buono))
	if err != nil {
		t.Fatalf("ruolo «parte» rifiutato: %v", err)
	}
	if r.FamiglieCodice[0].Ruolo != "parte" {
		t.Fatalf("ruolo non letto: %+v", r.FamiglieCodice[0])
	}
	if got := Compila("X", r).Estrai(Testo{Corpo: "AC12345"}).Codici[0].Ruolo; got != RuoloParte {
		t.Errorf("il ruolo dichiarato non arriva al candidato: %q", got)
	}
	cattivo := `{"famiglie_codice":[{"regex":"\\bAC\\d{5}\\b","esempio":"AC12345","ruolo":"finito"}]}`
	if _, err := ValidaRegole([]byte(cattivo)); err == nil {
		t.Error("un ruolo inventato deve essere rifiutato: «finito» non è una natura, è un ruolo nella RFQ (D19)")
	}
}

// I motivi del triage finiscono in jsonb: devono restare serializzabili anche quando contengono le
// virgolette basse e gli apostrofi delle frasi italiane.
func TestIMotiviSonoSerializzabili(t *testing.T) {
	e := Triage(IngressoTriage{Oggetto: "RDO 490020618 richiesta d'offerta", Corpo: "«urgente»",
		Direzione: "entrata", BuyerNoto: true, Motore: motoreDiProva(t),
		Candidati: []Candidato{{ThreadID: "t", Regola: R4Riferimento, Punteggio: 90, Evidenza: "il riferimento «RDO 490020618» è quello di questa richiesta"}}})
	if _, err := json.Marshal(e.Motivi); err != nil {
		t.Fatalf("motivi non serializzabili: %v", err)
	}
}

func contieneMotivo(motivi []string, frammento string) bool {
	for _, m := range motivi {
		if strings.Contains(m, frammento) {
			return true
		}
	}
	return false
}
