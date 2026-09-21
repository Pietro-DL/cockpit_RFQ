// Package regole è lo SCHEMA di ciò che un cliente dichiara di sé: le famiglie di codice, il
// riferimento della richiesta, le frasi del portale, le convenzioni di codice. Qui stanno le due
// porte — quella in scrittura che rifiuta, quella in lettura che segna ✓/✗ — e la diagnosi che la
// schermata Anagrafica mostra riga per riga.
//
// Che cosa il Cockpit FA di queste regole non sta qui: il motore che le compila e le applica a un
// testo è `core/domain`, che importa questo package e non il contrario.
package regole

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// Le regole di riconoscimento di un cliente (voce 6.11, D17)
//
// `cliente.regole` è jsonb e resta jsonb: i clienti sono una ventina, e una tabella generica di
// regole per una ventina di righe costa più di quel che rende. Il prezzo del jsonb è che il
// database non sa dire se dentro c'è quello che ci deve essere: lo schema è QUESTO file, e la
// convalida avviene in scrittura.
//
// # L'esempio obbligatorio
//
// Ogni regola che contiene una regex porta con sé un esempio, e l'esempio deve corrispondere. Non
// è una cortesia verso chi legge: una regex sbagliata non fallisce, non riconosce — e un cliente
// che smette di essere riconosciuto non produce nessun errore, produce silenzio. L'esempio è
// l'unico modo per distinguere «questa regola non ha trovato niente perché non c'era niente» da
// «questa regola non trova niente e basta».
//
// # Due porte, non una
//
// Il registro delle decisioni (D17) dice che una regola il cui esempio non corrisponde NON VIENE
// SALVATA; l'addendum v2 e il mockup dicono che una regola così va segnata ✗ e non usata. Non è
// una contraddizione: sono le due metà della stessa cosa, e servono entrambe.
//
//   - `Valida` è la porta in SCRITTURA: rifiuta, e si scusa spiegando cosa c'è che non va. È
//     quella che impedisce di creare una regola rotta dalla schermata Anagrafica.
//   - `Leggi` è la porta in LETTURA: non rifiuta niente, perché ciò che è già in database va
//     letto comunque — lo ha scritto un seed, o una mano in psql, o una versione precedente di
//     questo file. Restituisce le regole USABILI più una diagnosi riga per riga (✓/✗).
//
// Il motore lavora solo su ciò che `Leggi` ha dichiarato usabile. Una regola ✗ non riconosce e
// non propone: non riconoscere è già il suo comportamento, la differenza è che adesso si vede.
type Regole struct {
	// FamiglieCodice descrive come sono fatti i codici prodotto di questo cliente.
	FamiglieCodice []FamigliaCodice `json:"famiglie_codice,omitempty"`
	// RiferimentoRFQ è il numero con cui il cliente chiama la richiesta (RDO, Anfrage, ODA…).
	// Non è un codice prodotto: è l'identificativo della richiesta, e vive in un campo suo
	// perché confonderlo con un codice significa proporre un articolo che non esiste.
	RiferimentoRFQ *Riferimento `json:"riferimento_rfq,omitempty"`
	// CanaleAtteso è in che forma arriva di solito la richiesta ("mail con .msg annidato",
	// "PDF d'ordine", "portale"). Testo libero: serve all'operatore, non a un ramo di codice.
	CanaleAtteso string `json:"canale_atteso,omitempty"`
	// FrasiPortale si aggiungono a quelle generiche di `RilevaPortale`, non le sostituiscono:
	// «caricato sul portale» resta vero anche per un cliente che ne ha di sue.
	FrasiPortale []string `json:"frasi_portale,omitempty"`
	// LinguaRisposta è la lingua in cui si risponde a questo cliente (it, en, de, fr…), che non
	// è sempre quella del paese: un cliente francese puo' scrivere in italiano, se la
	// corrispondenza passa dal suo distributore.
	LinguaRisposta string `json:"lingua_risposta,omitempty"`
	// RichiedeCBD: l'offerta va accompagnata dal cost breakdown.
	RichiedeCBD bool `json:"richiede_cbd,omitempty"`
	// NumeroOrdineAnticipato: questo cliente emette il numero d'ordine PRIMA di ricevere
	// l'offerta. Per questi clienti una mail con la parola «ordine» è una richiesta d'offerta,
	// non una conferma, e chi non lo sa la archivia.
	NumeroOrdineAnticipato bool `json:"numero_ordine_anticipato,omitempty"`
	// FinestraAggancioGG è entro quanti giorni un messaggio con lo stesso codice si considera
	// ancora la stessa richiesta. 0 = non dichiarata.
	FinestraAggancioGG int `json:"finestra_aggancio_gg,omitempty"`
	// DatiRichiesti sono le informazioni che questo cliente pretende NELL'OFFERTA, oltre al prezzo.
	//
	// Non è una curiosità: un cliente scrive in fondo alla richiesta, evidenziato in giallo,
	// «l'offerta dovrà essere completa dei dati sotto citati: Paese di Origine, Codice Nomenclatura
	// Doganale, Peso Kg», e un'offerta che arriva senza quei dati viene rimandata indietro. Chi
	// prepara l'offerta lo scopre rileggendo la mail; qui lo scopre guardando il cliente.
	//
	// Testo libero, un elemento per voce: serve a una persona, non a un ramo di codice. Un giorno
	// diventerà una lista di controllo prima dell'invio — non oggi.
	DatiRichiesti []string `json:"dati_richiesti,omitempty"`
	// RispostaEntroGG è entro quanti giorni LAVORATIVI questo cliente si aspetta l'offerta. 0 = non
	// dichiarato. È la finestra che il cliente dà a noi, ed è un'altra cosa da FinestraAggancioGG,
	// che è la memoria del riconoscimento: confonderle vorrebbe dire agganciare le risposte con lo
	// stesso numero con cui si misura il ritardo.
	RispostaEntroGG int `json:"risposta_entro_gg,omitempty"`
}

// FamigliaCodice è una forma di codice prodotto del cliente, con l'esempio che la dimostra.
type FamigliaCodice struct {
	Regex       string `json:"regex"`
	Descrizione string `json:"descrizione,omitempty"`
	// RevNelCodice: la revisione è dentro il codice invece che accanto. Quando è vero la regex
	// DEVE avere un gruppo con nome `rev` (e può averne uno `codice`): è l'unico modo di dire
	// dove sta la revisione senza scrivere la convenzione di un cliente dentro il Go.
	//
	//	"AC12345B"  → (?P<codice>AC\d{5})(?P<rev>[A-Z])
	//	"104453_A4" → (?P<codice>\d{6})_(?P<rev>A\d?)
	RevNelCodice bool   `json:"rev_nel_codice,omitempty"`
	Esempio      string `json:"esempio"`
	// Ruolo dice che cosa sono i codici di questa famiglia: "prodotto" (il valore predefinito) o
	// "parte", cioe' sottoassiemi e particolari. Lo dichiara l'anagrafica del cliente, non una
	// regola scritta in Go su un cliente preciso: e' la differenza fra un sistema configurabile e
	// un sistema con dentro i nomi dei clienti.
	//
	// «Finito» non e' un valore possibile, ed e' voluto (D19): finito e' il RUOLO di un articolo
	// dentro una richiesta — la radice — e lo si sa dalla distinta, non dalla forma del codice.
	Ruolo string `json:"ruolo,omitempty"`
}

// Riferimento è il numero della richiesta del cliente, con il suo esempio.
type Riferimento struct {
	Regex       string `json:"regex"`
	Descrizione string `json:"descrizione,omitempty"`
	Esempio     string `json:"esempio"`
}

// Diagnostica è una riga del ✓/✗ della schermata Anagrafica: quale regola, se è usabile, e — se
// non lo è — perché, in una frase che si possa leggere senza aprire il codice.
type Diagnostica struct {
	Regola    string // "famiglia 1", "riferimento RFQ", "frase portale 2"
	Dettaglio string
	Esempio   string
	Ok        bool
	Motivo    string
}

const (
	maxFamiglie     = 20
	maxFrasiPortale = 20
	maxFinestraGG   = 3650 // dieci anni: oltre, è un errore di battitura, non una politica
)

// Valida legge il JSON con lo schema stretto e rifiuta tutto ciò che non lo rispetta: un campo che
// non esiste (di solito un nome scritto male, che senza questo controllo verrebbe ignorato in
// silenzio e la regola non esisterebbe), una regex che non compila, un esempio mancante o che non
// corrisponde alla propria regex. È la porta in scrittura.
func ValidaRegole(raw []byte) (Regole, error) {
	var r Regole
	if len(bytes.TrimSpace(raw)) == 0 {
		return r, nil
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, fmt.Errorf("regole: %w", pulisciErroreJSON(err))
	}
	if d.More() {
		return r, fmt.Errorf("regole: dopo l'oggetto JSON c'è dell'altro testo")
	}
	for _, d := range r.Verifica() {
		if !d.Ok {
			return r, fmt.Errorf("regole: %s: %s", d.Regola, d.Motivo)
		}
	}
	return r, nil
}

// Leggi è la porta in lettura: non rifiuta niente e restituisce sempre qualcosa di usabile. Se il
// JSON è illeggibile restituisce regole vuote e una diagnosi che lo dice; se è leggibile ma
// qualche regola è rotta, restituisce tutto e la diagnosi dice quali.
//
// Le regole rotte restano nella struttura — la schermata deve poterle mostrare per farle
// correggere — ma `Compila` non le mette nel motore.
func LeggiRegole(raw []byte) (Regole, []Diagnostica) {
	var r Regole
	if len(bytes.TrimSpace(raw)) == 0 {
		return r, nil
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return Regole{}, []Diagnostica{{Regola: "JSON", Ok: false,
			Motivo: "non è un oggetto JSON leggibile: " + pulisciErroreJSON(err).Error()}}
	}
	return r, r.Verifica()
}

// Verifica produce il ✓/✗ riga per riga. È la stessa funzione che usa `Valida` per decidere se
// salvare e la stessa che usa la schermata per disegnare le spunte: se fossero due, la spunta
// verde e il salvataggio riuscito smetterebbero di voler dire la stessa cosa.
func (r Regole) Verifica() []Diagnostica {
	var out []Diagnostica
	if n := len(r.FamiglieCodice); n > maxFamiglie {
		out = append(out, Diagnostica{Regola: "famiglie_codice", Ok: false,
			Motivo: fmt.Sprintf("sono %d: il limite è %d, e un cliente con venti famiglie di codice di solito è due clienti", n, maxFamiglie)})
	}
	for i, f := range r.FamiglieCodice {
		d := verificaRegex(fmt.Sprintf("famiglia %d", i+1), f.Descrizione, f.Regex, f.Esempio, f.RevNelCodice)
		// Il ruolo, se dichiarato, deve essere uno dei due previsti. Un valore inventato passerebbe
		// come «prodotto» in silenzio, e nessuno saprebbe che la dichiarazione non ha avuto effetto.
		if d.Ok && f.Ruolo != "" && f.Ruolo != "prodotto" && f.Ruolo != "parte" {
			d.Ok = false
			d.Motivo = fmt.Sprintf("ruolo «%s» non previsto: i valori sono «prodotto» e «parte». "+
				"«finito» non c'è di proposito: e' il ruolo di un articolo dentro una richiesta, e lo dice la distinta (D19)", f.Ruolo)
		}
		out = append(out, d)
	}
	if r.RiferimentoRFQ != nil {
		out = append(out, verificaRegex("riferimento RFQ", r.RiferimentoRFQ.Descrizione, r.RiferimentoRFQ.Regex, r.RiferimentoRFQ.Esempio, false))
	}
	if n := len(r.FrasiPortale); n > maxFrasiPortale {
		out = append(out, Diagnostica{Regola: "frasi_portale", Ok: false,
			Motivo: fmt.Sprintf("sono %d: il limite è %d", n, maxFrasiPortale)})
	}
	for i, f := range r.FrasiPortale {
		d := Diagnostica{Regola: fmt.Sprintf("frase portale %d", i+1), Dettaglio: f, Ok: true}
		switch s := strings.TrimSpace(f); {
		case s == "":
			d.Ok, d.Motivo = false, "è vuota"
		case len(s) < 4:
			d.Ok, d.Motivo = false, fmt.Sprintf("«%s» è troppo corta: comparirebbe dentro parole qualunque e segnerebbe come «sul portale» mezza casella", s)
		}
		out = append(out, d)
	}
	if l := r.LinguaRisposta; l != "" {
		d := Diagnostica{Regola: "lingua_risposta", Dettaglio: l, Ok: true}
		if len(l) != 2 || strings.ToLower(l) != l {
			d.Ok, d.Motivo = false, "va scritta con due lettere minuscole (it, en, de, fr)"
		}
		out = append(out, d)
	}
	for i, d := range r.DatiRichiesti {
		dg := Diagnostica{Regola: fmt.Sprintf("dato richiesto %d", i+1), Dettaglio: d, Ok: true}
		if strings.TrimSpace(d) == "" {
			dg.Ok, dg.Motivo = false, "è vuoto"
		} else if len(d) > 120 {
			dg.Ok, dg.Motivo = false, "è lungo più di 120 caratteri: è una voce di elenco, non una nota"
		}
		out = append(out, dg)
	}
	if g := r.RispostaEntroGG; g != 0 {
		d := Diagnostica{Regola: "risposta_entro_gg", Dettaglio: fmt.Sprint(g), Ok: true}
		if g < 0 || g > 365 {
			d.Ok, d.Motivo = false, fmt.Sprintf("sono giorni lavorativi: %d non è un numero possibile (da 1 a 365)", g)
		}
		out = append(out, d)
	}
	if g := r.FinestraAggancioGG; g != 0 {
		d := Diagnostica{Regola: "finestra_aggancio_gg", Dettaglio: fmt.Sprint(g), Ok: true}
		if g < 0 || g > maxFinestraGG {
			d.Ok, d.Motivo = false, fmt.Sprintf("sono giorni: %d non è un numero di giorni possibile (da 1 a %d)", g, maxFinestraGG)
		}
		out = append(out, d)
	}
	return out
}

// verificaRegex è il controllo che vale per ogni regola con una regex: compila, c'è l'esempio,
// l'esempio corrisponde. E, quando la revisione sta dentro il codice, che la regex dica DOVE.
func verificaRegex(nome, descrizione, espressione, esempio string, revNelCodice bool) Diagnostica {
	d := Diagnostica{Regola: nome, Dettaglio: descrizione, Esempio: esempio}
	if strings.TrimSpace(espressione) == "" {
		d.Motivo = "manca la regex"
		return d
	}
	if descrizione == "" {
		d.Dettaglio = espressione
	}
	re, err := regexp.Compile(espressione)
	if err != nil {
		d.Motivo = "la regex non compila: " + messaggioRegex(err)
		return d
	}
	if revNelCodice && !contiene(re.SubexpNames(), "rev") {
		d.Motivo = "rev_nel_codice è vero ma la regex non dice dove sta la revisione: serve un gruppo (?P<rev>…)"
		return d
	}
	if strings.TrimSpace(esempio) == "" {
		d.Motivo = "manca l'esempio, e senza esempio nessuno si accorge il giorno in cui la regex smette di riconoscere"
		return d
	}
	if !re.MatchString(esempio) {
		d.Motivo = fmt.Sprintf("l'esempio «%s» non corrisponde alla regex: uno dei due è sbagliato, e finché non si sa quale la regola non si usa", esempio)
		return d
	}
	d.Ok = true
	return d
}

// ---------------------------------------------------------------- minuterie

func contiene(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// messaggioRegex accorcia l'errore di regexp.Compile, che per default ripete tutta l'espressione.
func messaggioRegex(err error) string {
	s := err.Error()
	if i := strings.Index(s, ": "); i > 0 && strings.HasPrefix(s, "error parsing regexp") {
		s = strings.TrimSpace(s[i+2:])
	}
	return s
}

// pulisciErroreJSON traduce i due errori che si incontrano davvero in qualcosa di leggibile.
func pulisciErroreJSON(err error) error {
	s := err.Error()
	if strings.HasPrefix(s, "json: unknown field ") {
		campo := strings.Trim(strings.TrimPrefix(s, "json: unknown field "), `"`)
		return fmt.Errorf("il campo %q non esiste (i campi ammessi sono: %s)", campo, strings.Join(CampiRegole(), ", "))
	}
	if t, ok := err.(*json.UnmarshalTypeError); ok {
		return fmt.Errorf("il campo %q vuole un %s, non un %s", t.Field, t.Type, t.Value)
	}
	return err
}

// CampiRegole elenca i nomi ammessi, letti dai tag della struct. Serve al messaggio d'errore e
// alla schermata: un elenco scritto a mano si allontanerebbe dalla struct al primo campo nuovo, e
// il messaggio d'errore comincerebbe a mentire proprio a chi sta cercando di capire dove sbaglia.
func CampiRegole() []string {
	t := reflect.TypeOf(Regole{})
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		nome, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if nome != "" && nome != "-" {
			out = append(out, nome)
		}
	}
	sort.Strings(out)
	return out
}
