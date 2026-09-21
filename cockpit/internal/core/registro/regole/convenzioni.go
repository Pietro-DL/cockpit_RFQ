package regole

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// LE CONVENZIONI DI CODICE → LAVORAZIONE (blocco 7A, D39)
//
// Nei cartigli di alcuni clienti la lavorazione superficiale e' scritta nel codice del pezzo, di
// solito come suffisso. Da quel suffisso si ricava la lavorazione richiesta e, con le qualifiche
// (`cliente_fornitore_lavorazione`), chi e' qualificato a farla per QUEL cliente.
//
// Stessa filosofia delle regole del cliente (D17), in tabelle e non in jsonb, perche' qui ogni
// convenzione punta a una riga di `lavorazione` con chiave esterna:
//
//   - ogni convenzione porta un ESEMPIO che deve corrispondere e un CONTROESEMPIO facoltativo che
//     NON deve corrispondere. E' il controesempio a scoprire in scrittura una convenzione troppo
//     larga: il suffisso «V» prende anche «...-ZV»;
//   - `ValidaConvenzione` e' la porta in SCRITTURA (rifiuta e spiega), `LeggiConvenzioni` la porta
//     in LETTURA (segna ✗ e non usa: cio' che e' gia' in database va letto comunque);
//   - il risultato e' un INSIEME di lavorazioni, non una sola: un pezzo puo' volere zincatura E
//     verniciatura, e una convenzione puo' dire piu' lavorazioni. Corrispondono tutte le
//     convenzioni attive che corrispondono; nessuna precedenza nascosta.
//
// Nessun suffisso reale e' scritto qui: le convenzioni si scrivono dall'anagrafica del cliente.

const (
	// ModoSuffisso: testo letterale confrontato con la FINE del codice, senza distinguere le
	// maiuscole. Si compila come (?i)<QuoteMeta>$: chi lo scrive non deve sapere niente di regex.
	ModoSuffisso = "suffisso"
	// ModoRegex: un'espressione RE2 completa, per tutto il resto (posizione in mezzo, separatore
	// obbligatorio...). Si usa com'e', con MatchString.
	ModoRegex = "regex"
)

// Convenzione e' una riga di `convenzione_codice` con le sue lavorazioni.
type Convenzione struct {
	ID            uuid.UUID
	Modo          string
	Espressione   string
	Esempio       string
	Controesempio string
	Descrizione   string
	Attiva        bool
	Lavorazioni   []string
}

// MaxSuffisso e' la lunghezza massima di un suffisso: e' anche il CHECK della tabella. Se serve di
// piu', o servono spazi, e' una regex, e va dichiarata tale.
const MaxSuffisso = 12

// RegexDi e' l'espressione compilabile di una convenzione, qualunque sia il modo.
func RegexDi(c Convenzione) (string, error) {
	e := strings.TrimSpace(c.Espressione)
	if e == "" {
		return "", errors.New("manca l'espressione")
	}
	switch c.Modo {
	case ModoSuffisso:
		if strings.ContainsAny(e, " \t\r\n") {
			return "", errors.New("un suffisso non contiene spazi: se serve uno spazio è una regex")
		}
		if len(e) > MaxSuffisso {
			return "", fmt.Errorf("un suffisso è lungo al massimo %d caratteri: «%s» è una regex, e va dichiarata tale", MaxSuffisso, e)
		}
		return "(?i)" + regexp.QuoteMeta(e) + "$", nil
	case ModoRegex:
		return e, nil
	}
	return "", fmt.Errorf("modo «%s» non previsto: i valori sono «%s» e «%s»", c.Modo, ModoSuffisso, ModoRegex)
}

// VerificaConvenzione produce il ✓/✗ di una convenzione. E' la stessa funzione che usa
// ValidaConvenzione per decidere se salvare e la stessa che usa la schermata per disegnare la
// spunta: se fossero due, la spunta verde e il salvataggio riuscito smetterebbero di voler dire la
// stessa cosa (D17).
func VerificaConvenzione(c Convenzione) Diagnostica {
	d := Diagnostica{Regola: c.Descrizione, Dettaglio: c.Modo + " «" + c.Espressione + "»", Esempio: c.Esempio}
	if d.Regola == "" {
		d.Regola = "convenzione"
	}
	espr, err := RegexDi(c)
	if err != nil {
		d.Motivo = err.Error()
		return d
	}
	re, err := regexp.Compile(espr)
	if err != nil {
		d.Motivo = "la regex non compila: " + messaggioRegex(err)
		return d
	}
	if strings.TrimSpace(c.Esempio) == "" {
		d.Motivo = "manca l'esempio, e senza esempio nessuno si accorge il giorno in cui la convenzione smette di riconoscere"
		return d
	}
	if !re.MatchString(c.Esempio) {
		d.Motivo = fmt.Sprintf("l'esempio «%s» non corrisponde: uno dei due è sbagliato, e finché non si sa quale la convenzione non si usa", c.Esempio)
		return d
	}
	if ce := strings.TrimSpace(c.Controesempio); ce != "" {
		if ce == c.Esempio {
			d.Motivo = "il controesempio è uguale all'esempio: non può insieme corrispondere e non corrispondere"
			return d
		}
		if re.MatchString(ce) {
			d.Motivo = fmt.Sprintf("il controesempio «%s» corrisponde: la convenzione è troppo larga, e prenderebbe codici che non deve", ce)
			return d
		}
	}
	if len(c.Lavorazioni) == 0 {
		d.Motivo = "non dice nessuna lavorazione: una convenzione che corrisponde e non dice niente non serve a niente"
		return d
	}
	for _, l := range c.Lavorazioni {
		if strings.TrimSpace(l) == "" {
			d.Motivo = "una delle lavorazioni è vuota"
			return d
		}
	}
	d.Ok = true
	return d
}

// ValidaConvenzione e' la porta in scrittura: rifiuta e dice perche'.
func ValidaConvenzione(c Convenzione) error {
	if d := VerificaConvenzione(c); !d.Ok {
		return errors.New(d.Motivo)
	}
	return nil
}

// Convenzioni sono le convenzioni di un cliente gia' compilate e gia' ripulite di quelle rotte e di
// quelle spente. Si costruisce una volta e si interroga per codice.
type Convenzioni struct {
	compilate []convenzioneCompilata
}

type convenzioneCompilata struct {
	Convenzione
	re *regexp.Regexp
}

// LeggiConvenzioni e' la porta in lettura: non rifiuta niente, restituisce le convenzioni USABILI
// piu' la diagnosi riga per riga. Una convenzione spenta e' ✓ ma non si usa: la diagnosi lo dice.
func LeggiConvenzioni(righe []Convenzione) (*Convenzioni, []Diagnostica) {
	out := &Convenzioni{}
	var diag []Diagnostica
	for i, c := range righe {
		d := VerificaConvenzione(c)
		if d.Regola == "convenzione" {
			d.Regola = fmt.Sprintf("convenzione %d", i+1)
		}
		if d.Ok && !c.Attiva {
			d.Motivo = "spenta: non si usa finché non viene riattivata"
		}
		diag = append(diag, d)
		if !d.Ok || !c.Attiva {
			continue
		}
		espr, _ := RegexDi(c)
		out.compilate = append(out.compilate, convenzioneCompilata{Convenzione: c, re: regexp.MustCompile(espr)})
	}
	return out, diag
}

// LavorazioneTrovata e' una lavorazione richiesta da un codice, con l'evidenza: quale convenzione
// l'ha detto.
type LavorazioneTrovata struct {
	Lavorazione   string
	ConvenzioneID uuid.UUID
	Descrizione   string
	Espressione   string
}

// Lavorazioni e' l'insieme delle lavorazioni che il codice richiede, in ordine di codice
// lavorazione. Se due convenzioni dicono la stessa lavorazione, l'evidenza e' della prima (in
// ordine di lettura); l'insieme non cambia. Un codice che non corrisponde a niente da' un insieme
// vuoto, che non e' un errore: e' la risposta normale per un cliente senza convenzioni.
func (cs *Convenzioni) Lavorazioni(codice string) []LavorazioneTrovata {
	if cs == nil {
		return nil
	}
	codice = strings.TrimSpace(codice)
	if codice == "" {
		return nil
	}
	viste := map[string]bool{}
	var out []LavorazioneTrovata
	for _, c := range cs.compilate {
		if !c.re.MatchString(codice) {
			continue
		}
		for _, l := range c.Lavorazioni {
			if viste[l] {
				continue
			}
			viste[l] = true
			out = append(out, LavorazioneTrovata{Lavorazione: l, ConvenzioneID: c.ID, Descrizione: c.Descrizione, Espressione: c.Espressione})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Lavorazione < out[j].Lavorazione })
	return out
}

// Quante ne sono usabili: per il log e per la schermata.
func (cs *Convenzioni) N() int {
	if cs == nil {
		return 0
	}
	return len(cs.compilate)
}
