package regole

import (
	"encoding/json"
	"strings"
	"testing"
)

// L1 — lo schema delle regole di riconoscimento di un cliente (voce 6.11, D17): AN1.
//
// AN3 e AN4 provano che cosa il MOTORE fa di queste regole: stanno con il motore,
// in `core/inbox/classificazione`.
//
// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri
// sono un dato dell'azienda e questo repository è pubblico, e un test che dipendesse da esse
// diventerebbe rosso il giorno in cui un cliente cambia convenzione — cioè per un motivo che non
// ha niente a che vedere con il codice che sta provando.

// Le stesse regole buone servono di qua (entrano intere) e di là (il motore le applica): la
// fixture è ripetuta apposta in `core/inbox/classificazione`, perché nessuno dei due package debba
// leggere i test
// dell'altro. Se cambia una, cambiano tutte e due.
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
