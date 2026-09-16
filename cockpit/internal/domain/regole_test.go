package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// L1 — le regole di riconoscimento di un cliente (voce 6.11, D17): AN1, AN3, AN4.
//
// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri
// sono un dato dell'azienda e questo repository è pubblico, e un test che dipendesse da esse
// diventerebbe rosso il giorno in cui un cliente cambia convenzione — cioè per un motivo che non
// ha niente a che vedere con il codice che sta provando.

const regoleBuone = `{
  "famiglie_codice": [
    {"regex": "\\bAC\\d{5}[A-Z]\\b", "descrizione": "codici ACME", "esempio": "AC12345B"}
  ],
  "riferimento_rfq": {"regex": "\\bRDO-\\d{4}\\b", "descrizione": "numero RDO", "esempio": "RDO-7781"},
  "canale_atteso": "mail con PDF allegato",
  "frasi_portale": ["nel nostro portale fornitori"],
  "lingua_risposta": "it",
  "richiede_cbd": true,
  "numero_ordine_anticipato": false,
  "finestra_aggancio_gg": 30
}`

// AN1 — lo schema in scrittura rifiuta, e dice che cosa. Ogni riga di questa tabella è un modo
// diverso di scrivere una regola che non riconoscerebbe niente SENZA fallire: è il motivo per cui
// la convalida esiste, perché il sintomo di tutti questi errori è identico — silenzio.
func TestAN1LoSchemaRifiutaCioCheNonRiconoscerebbeNiente(t *testing.T) {
	casi := []struct {
		nome, jsonIn, attesoNelMessaggio string
	}{
		{"campo inesistente (di solito un nome scritto male)",
			`{"famiglie_codici": []}`, "famiglie_codici"},
		{"regex che non compila",
			`{"famiglie_codice":[{"regex":"AC[0-9","esempio":"AC1"}]}`, "non compila"},
		{"esempio mancante",
			`{"famiglie_codice":[{"regex":"\\bAC\\d{5}\\b"}]}`, "manca l'esempio"},
		{"esempio che non corrisponde alla propria regex",
			`{"famiglie_codice":[{"regex":"\\bAC\\d{5}\\b","esempio":"XY99"}]}`, "non corrisponde"},
		{"rev_nel_codice senza dire dove sta la rev",
			`{"famiglie_codice":[{"regex":"\\bAC\\d{5}\\b","esempio":"AC12345","rev_nel_codice":true}]}`, "(?P<rev>"},
		{"riferimento RFQ con esempio sbagliato",
			`{"riferimento_rfq":{"regex":"\\bRDO-\\d{4}\\b","esempio":"RDO-XX"}}`, "non corrisponde"},
		{"lingua di risposta scritta come non si scrive",
			`{"lingua_risposta":"Italiano"}`, "due lettere"},
		{"finestra di aggancio negativa",
			`{"finestra_aggancio_gg":-3}`, "giorni"},
		{"frase portale troppo corta per non pescare mezza casella",
			`{"frasi_portale":["su"]}`, "troppo corta"},
		{"un campo con il tipo sbagliato",
			`{"richiede_cbd":"si"}`, "richiede_cbd"},
		{"non un oggetto",
			`["famiglie_codice"]`, "regole"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, err := ValidaRegole([]byte(c.jsonIn))
			if err == nil {
				t.Fatalf("accettato: %s", c.jsonIn)
			}
			if !strings.Contains(err.Error(), c.attesoNelMessaggio) {
				t.Errorf("l'errore non dice %q: %v", c.attesoNelMessaggio, err)
			}
		})
	}
}

// L'altra metà: le regole buone passano, e passano INTERE. Un campo che si perde per strada è un
// campo che non varrà mai, e non lo scoprirebbe nessuno.
func TestRegoleBuoneEntranoIntere(t *testing.T) {
	r, err := ValidaRegole([]byte(regoleBuone))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.FamiglieCodice) != 1 || r.FamiglieCodice[0].Descrizione != "codici ACME" {
		t.Errorf("famiglie: %+v", r.FamiglieCodice)
	}
	if r.RiferimentoRFQ == nil || r.RiferimentoRFQ.Esempio != "RDO-7781" {
		t.Errorf("riferimento: %+v", r.RiferimentoRFQ)
	}
	if !r.RichiedeCBD || r.NumeroOrdineAnticipato || r.LinguaRisposta != "it" || r.FinestraAggancioGG != 30 {
		t.Errorf("campi semplici persi: %+v", r)
	}
	if r.CanaleAtteso == "" || len(r.FrasiPortale) != 1 {
		t.Errorf("canale/frasi persi: %+v", r)
	}
	// Ciò che è stato validato deve poter tornare in database e rileggersi uguale: il salvataggio
	// riscrive il JSON normalizzato, e un giro che perde pezzi li perde in silenzio.
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := ValidaRegole(b)
	if err != nil {
		t.Fatalf("il JSON riscritto dal server non passa più la sua stessa convalida: %v", err)
	}
	if len(r2.FamiglieCodice) != 1 || r2.FinestraAggancioGG != 30 || !r2.RichiedeCBD {
		t.Errorf("il giro di scrittura e rilettura ha perso qualcosa: %+v", r2)
	}
}

// Una configurazione vuota non è un errore: è un cliente appena censito, che è come nascono tutti.
func TestUnClienteSenzaRegoleVaBene(t *testing.T) {
	for _, vuoto := range []string{"", "{}", "  "} {
		if _, err := ValidaRegole([]byte(vuoto)); err != nil {
			t.Errorf("%q rifiutato: %v", vuoto, err)
		}
	}
}

// AN3 — una regola il cui esempio non corrisponde è SEGNALATA e NON viene usata.
//
// Il caso è quello di una regola già in database — seminata da un file, o scritta in psql, o
// rimasta da una versione precedente dello schema: la porta in scrittura non l'ha vista passare.
// Il motore deve comportarsi come se non ci fosse, e la schermata deve dire perché.
func TestAN3UnaRegolaRottaSiVedeENonSiUsa(t *testing.T) {
	// due famiglie: la prima buona, la seconda con l'esempio che non le corrisponde
	const misto = `{"famiglie_codice":[
	  {"regex":"\\bAC\\d{5}[A-Z]\\b","descrizione":"buona","esempio":"AC12345B"},
	  {"regex":"\\bZZ\\d{4}\\b","descrizione":"rotta","esempio":"non-corrisponde"}]}`

	regole, diag := LeggiRegole([]byte(misto))
	if len(diag) != 2 {
		t.Fatalf("diagnosi: %d righe, attese 2: %+v", len(diag), diag)
	}
	if !diag[0].Ok {
		t.Errorf("la famiglia buona è segnata rotta: %+v", diag[0])
	}
	if diag[1].Ok {
		t.Fatal("la famiglia rotta è segnata buona: la schermata mostrerebbe una spunta verde su una regola che non riconosce niente")
	}
	if !strings.Contains(diag[1].Motivo, "non corrisponde") {
		t.Errorf("il motivo non aiuta a correggerla: %q", diag[1].Motivo)
	}

	// e adesso il punto: il motore non la usa. `ZZ1234` c'è nel testo e ha la forma della regola
	// rotta; se il motore la usasse, uscirebbe attribuito alla famiglia «rotta».
	m := Compila("ACME", regole)
	for _, c := range m.Codici("preventivo AC12345B e ZZ1234") {
		if c.Famiglia == "rotta" {
			t.Errorf("il motore ha usato la regola con l'esempio sbagliato: %+v", c)
		}
	}
	if len(m.famiglie) != 1 {
		t.Errorf("nel motore sono entrate %d famiglie invece di 1", len(m.famiglie))
	}
}

// AN4 — le famiglie del cliente estraggono i codici del cliente, con la loro provenienza, e la
// revisione quando la famiglia dice dove sta.
func TestAN4LeFamiglieDelClienteEstraggonoConLaLoroProvenienza(t *testing.T) {
	const regole = `{"famiglie_codice":[
	  {"regex":"\\bAC(?P<codice>\\d{5})(?P<rev>[A-Z])\\b","descrizione":"ACME con rev in coda","esempio":"AC12345B","rev_nel_codice":true}]}`
	r, err := ValidaRegole([]byte(regole))
	if err != nil {
		t.Fatal(err)
	}
	m := Compila("ACME", r)

	trovati := m.Codici("Oggetto: offerta AC12345B", "vedere anche AC98765C e il disegno 7781234")
	var famiglia, generici int
	for _, c := range trovati {
		switch c.Origine {
		case "famiglia":
			famiglia++
			if c.Famiglia != "ACME con rev in coda" {
				t.Errorf("provenienza senza nome della famiglia: %+v", c)
			}
			if c.Rev == "" {
				t.Errorf("la famiglia dichiara la rev nel codice ma %+v non ne porta una", c)
			}
		case "generico":
			generici++
		default:
			t.Errorf("provenienza sconosciuta: %+v", c)
		}
	}
	if famiglia != 2 {
		t.Errorf("la famiglia ha riconosciuto %d codici invece di 2: %+v", famiglia, trovati)
	}
	// il primo: codice separato dalla revisione secondo la regola del cliente, non secondo una
	// convenzione scritta nel Go (AC12345B non ha separatori: `CodiceRev` non lo dividerebbe)
	if trovati[0].Codice != "12345" || trovati[0].Rev != "B" {
		t.Errorf("codice e rev separati male: %+v", trovati[0])
	}
	// e la rete sotto resta: 7781234 non è di nessuna famiglia ma ha la forma di un codice, e un
	// cliente che introduce una famiglia nuova non la dichiara, la manda
	if generici == 0 {
		t.Error("l'estrattore generico non ha trovato niente: una famiglia nuova non censita sarebbe invisibile")
	}
}

// Un cliente senza regole, o sconosciuto: il riconoscimento continua a funzionare. È il caso
// normale del primo giorno, e un nil che fa crollare il triage lo scoprirebbe l'operatore.
func TestSenzaMotoreIlRiconoscimentoFunzionaLoStesso(t *testing.T) {
	var nessuno *Motore
	if len(nessuno.Codici("richiesta per AC12345B e 7781234")) == 0 {
		t.Error("senza motore non esce nessun codice")
	}
	if rif, _ := nessuno.Riferimento("RDO-7781"); rif != "" {
		t.Errorf("senza motore è uscito un riferimento: %q", rif)
	}
	if nessuno.FrasiPortale() != nil {
		t.Error("senza motore sono uscite delle frasi portale")
	}
}

// Il riferimento della richiesta non è un codice prodotto: sta in un campo suo, e ci arriva con
// il nome della regola che l'ha riconosciuto.
func TestIlRiferimentoRFQNonEUnCodiceProdotto(t *testing.T) {
	r, err := ValidaRegole([]byte(regoleBuone))
	if err != nil {
		t.Fatal(err)
	}
	m := Compila("ACME", r)
	rif, nome := m.Riferimento("RE: RDO-7781 richiesta d'offerta")
	if rif != "RDO-7781" {
		t.Fatalf("riferimento: %q", rif)
	}
	if nome != "numero RDO" {
		t.Errorf("senza il nome della regola la proposta non è verificabile: %q", nome)
	}
}

// AN5 (metà L1) — il banco di prova e l'ingest passano dalla STESSA funzione. Qui si prova che
// quella funzione esiste e fa tutto: triage, portale e scadenza in una chiamata sola. L'altra
// metà — che la schermata chiami proprio questa — è in `internal/web` (L4).
func TestRiconosciFaTuttoInUnPostoSolo(t *testing.T) {
	r, err := ValidaRegole([]byte(regoleBuone))
	if err != nil {
		t.Fatal(err)
	}
	in := IngressoTriage{
		Oggetto:   "RDO-7781 richiesta d'offerta",
		Corpo:     "Buongiorno, i disegni di AC12345B sono nel nostro portale fornitori. Risposta entro il 30/09/2026.",
		Direzione: "entrata", ClienteNoto: true, Motore: Compila("ACME", r),
	}
	got := Riconosci(in, time.Date(2026, 9, 16, 0, 0, 0, 0, time.Local))

	if got.Triage.Esito != "nuova_rfq" {
		t.Errorf("esito: %q (punteggio %d, motivi %v)", got.Triage.Esito, got.Triage.Confidenza, got.Triage.Motivi)
	}
	if got.Triage.Riferimento != "RDO-7781" {
		t.Errorf("riferimento perso: %q", got.Triage.Riferimento)
	}
	// «nel nostro portale fornitori» è una frase DEL CLIENTE: senza le sue regole non la vedrebbe
	// nessuna delle frasi generiche
	if len(got.Portale) == 0 {
		t.Error("la frase portale del cliente non è stata riconosciuta")
	}
	if got.Scadenza == nil || got.Scadenza.Day() != 30 {
		t.Errorf("scadenza: %v", got.Scadenza)
	}
}

// Le frasi del cliente si AGGIUNGONO a quelle generiche. Un cliente con una frase sua non deve
// smettere di riconoscere «caricato sul portale», che è la frase di tutti.
func TestLeFrasiDelClienteNonSostituisconoQuelleGeneriche(t *testing.T) {
	corpo := "I file sono stati caricati sul portale come al solito."
	if len(RilevaPortale(corpo, "nel nostro portale fornitori")) == 0 {
		t.Error("con le frasi del cliente si è persa la frase generica")
	}
}
