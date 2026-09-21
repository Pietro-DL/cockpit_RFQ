package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
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

// ---------------------------------------------------------------- il motore

// Motore sono le regole di un cliente già compilate e già ripulite di quelle rotte. Si costruisce
// una volta per messaggio (o una volta per prova nel banco) e si passa in giro: compilare una
// regex dentro un ciclo su novecento messaggi è il modo più semplice di rendere lento il triage.
type Motore struct {
	Cliente     string // ragione sociale, per l'evidenza; vuoto se il cliente non è noto
	famiglie    []famigliaCompilata
	riferimento *regexp.Regexp
	rifNome     string
	frasi       []string
	Regole      Regole
}

type famigliaCompilata struct {
	re      *regexp.Regexp
	nome    string
	ruolo   string
	rev     bool
	iCodice int // indice del gruppo `codice`, -1 se assente
	iRev    int
}

// Compila costruisce il motore con le sole regole utilizzabili. Le regole ✗ vengono lasciate
// fuori: è il «non deve essere utilizzata silenziosamente come valida», e qui «silenziosamente»
// vale in tutte e due le direzioni — non si usa, e la schermata dice perché.
func Compila(cliente string, r Regole) *Motore {
	m := &Motore{Cliente: cliente, Regole: r}
	diag := r.Verifica()
	ok := func(nome string) bool {
		for _, d := range diag {
			if d.Regola == nome {
				return d.Ok
			}
		}
		return false
	}
	for i, f := range r.FamiglieCodice {
		if !ok(fmt.Sprintf("famiglia %d", i+1)) {
			continue
		}
		re := regexp.MustCompile(f.Regex) // già compilata da Verifica: qui non può fallire
		nome := f.Descrizione
		if nome == "" {
			nome = f.Regex
		}
		ruolo := RuoloProdotto
		if f.Ruolo == "parte" {
			ruolo = RuoloParte
		}
		m.famiglie = append(m.famiglie, famigliaCompilata{
			re: re, nome: nome, ruolo: ruolo, rev: f.RevNelCodice,
			iCodice: indiceGruppo(re, "codice"), iRev: indiceGruppo(re, "rev"),
		})
	}
	if r.RiferimentoRFQ != nil && ok("riferimento RFQ") {
		m.riferimento = regexp.MustCompile(r.RiferimentoRFQ.Regex)
		m.rifNome = r.RiferimentoRFQ.Descrizione
	}
	for i, f := range r.FrasiPortale {
		if ok(fmt.Sprintf("frase portale %d", i+1)) {
			m.frasi = append(m.frasi, strings.ToLower(strings.TrimSpace(f)))
		}
	}
	return m
}

// CodiceTrovato è un codice con la sua provenienza. La provenienza non è un ornamento: è ciò che
// permette all'operatore di credere o non credere alla proposta, ed è la sola differenza fra «il
// sistema propone» e «il sistema decide» (D9).
type CodiceTrovato struct {
	Codice   string // il codice come si scrive nel sistema del cliente
	Rev      string // la revisione, se la famiglia dice che sta dentro al codice
	Origine  string // "famiglia" | "generico" | "riferimento"
	Famiglia string // descrizione della famiglia che l'ha riconosciuto; vuota se generico
	// Ruolo è che cosa questo numero È, non solo da dove viene: "riferimento_rfq" (il nome che il
	// cliente dà alla richiesta), "prodotto" (una famiglia del cliente), "non_classificato"
	// (l'estrattore generico ha visto qualcosa con la forma di un codice, e nient'altro).
	Ruolo string
	// Punteggio è quanto vale l'affermazione. «Questo è un codice DI QUESTO CLIENTE» e «questo ha
	// la forma di un codice» non possono presentarsi con lo stesso numero accanto.
	Punteggio int
	// Dove è l'evidenza leggibile: «oggetto», «corpo», «allegato 6743449A_1.zip».
	Dove string
}

// Ruoli di un candidato di codice. Sono le stesse stringhe dell'enum `ruolo_codice` in database:
// il dominio non importa il pacchetto db, ma i valori sono uno solo e stanno scritti qui.
const (
	RuoloRiferimento = "riferimento_rfq"
	RuoloProdotto    = "prodotto"
	RuoloParte       = "parte" // mai proposto dal deterministico: serve la distinta (blocco 8)
	RuoloIgnoto      = "non_classificato"
)

// Punteggi con cui si presenta un codice, per provenienza.
const (
	PuntiFamiglia  = 80
	PuntiGenerico  = 30
	PuntiRiferimen = 90
)

// Testo è un pezzo di messaggio con l'etichetta di dove sta. L'etichetta non è un lusso: senza,
// l'evidenza di una proposta diventa «l'ho trovato da qualche parte», che non è un'evidenza.
type Testo struct {
	Dove  string
	Corpo string
}

// Testi costruisce l'elenco con una sola etichetta, per i casi in cui non serve distinguere.
func Testi(dove string, corpi ...string) []Testo {
	out := make([]Testo, 0, len(corpi))
	for _, c := range corpi {
		out = append(out, Testo{Dove: dove, Corpo: c})
	}
	return out
}

// Estrazione è tutto ciò che un messaggio dice in fatto di numeri, già separato per ruolo.
type Estrazione struct {
	Riferimento     string // il numero della richiesta secondo il cliente
	RiferimentoNome string // la regola che l'ha riconosciuto
	RiferimentoDove string
	Codici          []CodiceTrovato // riferimento escluso: un riferimento non è un codice prodotto
	HaFamiglie      bool            // il cliente ha almeno una famiglia utilizzabile
}

// Proponibili sono i codici che possono diventare identificativi della RFQ se l'operatore li spunta.
//
// La regola, dal banco reale: se il cliente HA famiglie dichiarate, l'estrattore generico non
// propone niente. Un cliente censito che manda un codice di forma nuova è un caso da guardare, non
// da indovinare; e nel frattempo ogni CAP, numero d'ordine, data e misura del piè di pagina smette
// di presentarsi come codice prodotto. I numeri generici restano visibili sotto «altri numeri
// trovati», non spuntati: si vedono, e per entrare serve un clic.
func (e Estrazione) Proponibili() []CodiceTrovato {
	var out []CodiceTrovato
	for _, c := range e.Codici {
		if e.HaFamiglie && c.Origine != "famiglia" {
			continue
		}
		// Blocco 6: un codice che viene dalla catena di risposta precedente non si propone da solo.
		// Era già stato deciso in un altro messaggio, e riproporlo a ogni risposta è il modo in cui
		// un «ricevuto, grazie» diventa una richiesta di sei pezzi. Resta visibile fra gli altri
		// numeri trovati: si vede, e per entrare serve un clic.
		if c.Dove == DoveStoria {
			continue
		}
		out = append(out, c)
	}
	return out
}

// Altri sono i numeri visti ma non proponibili: si mostrano, non si spuntano.
func (e Estrazione) Altri() []CodiceTrovato {
	if !e.HaFamiglie {
		return nil
	}
	var out []CodiceTrovato
	for _, c := range e.Codici {
		if c.Origine != "famiglia" {
			out = append(out, c)
		}
	}
	return out
}

// Codici estrae i codici prodotto da uno o più testi usando PRIMA le famiglie del cliente e POI
// l'estrattore generico.
//
// Perché tutti e due. Le famiglie dicono che cosa è certamente un codice di questo cliente, e
// quello va in cima con il nome della famiglia accanto. Ma un cliente che introduce una famiglia
// nuova non la dichiara: la manda e basta, e un motore che guardasse solo le famiglie censite
// smetterebbe di vedere i codici nuovi proprio mentre il cliente li introduce — senza errori, con
// la schermata che dice «nessun codice». L'estrattore generico resta la rete sotto, marcata come
// tale, e a decidere è comunque una persona.
func (m *Motore) Codici(testi ...string) []CodiceTrovato {
	return m.CodiciDa(Testi("testo", testi...)...)
}

// CodiciDa è la stessa cosa con l'etichetta di provenienza per ogni pezzo di testo.
func (m *Motore) CodiciDa(testi ...Testo) []CodiceTrovato {
	visti := map[string]bool{}
	var out []CodiceTrovato
	if m != nil {
		for _, f := range m.famiglie {
			for _, t := range testi {
				for _, g := range f.re.FindAllStringSubmatch(senzaURL(t.Corpo), -1) {
					c := CodiceTrovato{Codice: g[0], Origine: "famiglia", Famiglia: f.nome,
						Ruolo: f.ruolo, Punteggio: PuntiFamiglia, Dove: t.Dove}
					if f.iCodice > 0 && f.iCodice < len(g) {
						c.Codice = g[f.iCodice]
					}
					if f.rev && f.iRev > 0 && f.iRev < len(g) {
						c.Rev = g[f.iRev]
					}
					chiave := c.Codice + "\x00" + c.Rev
					if c.Codice == "" || visti[chiave] {
						continue
					}
					visti[chiave] = true
					out = append(out, c)
				}
			}
		}
	}
	for _, t := range testi {
		for _, c := range EstraiCodici(t.Corpo) {
			cod, rev := c, ""
			if k, r := CodiceRev(c); r != "" {
				cod, rev = k, r
			}
			if visti[cod+"\x00"+rev] || visti[c+"\x00"] {
				continue
			}
			// un codice già riconosciuto da una famiglia, con o senza la sua revisione, non si ripete
			if giaVisto(visti, cod) {
				continue
			}
			visti[cod+"\x00"+rev] = true
			out = append(out, CodiceTrovato{Codice: cod, Rev: rev, Origine: "generico",
				Ruolo: RuoloIgnoto, Punteggio: PuntiGenerico, Dove: t.Dove})
		}
	}
	return out
}

// HaFamiglie dice se il cliente ha almeno una famiglia di codice utilizzabile (dichiarata e ✓).
func (m *Motore) HaFamiglie() bool { return m != nil && len(m.famiglie) > 0 }

// Finestra è `finestra_aggancio_gg` del cliente, 0 se non dichiarata (e allora vale il default di
// internal/aggancio). Nil-safe come tutto il resto del motore: un cliente sconosciuto è il caso normale.
func (m *Motore) Finestra() int {
	if m == nil {
		return 0
	}
	return m.Regole.FinestraAggancioGG
}

// DiFamiglia filtra i codici riconosciuti da una famiglia del cliente. È l'insieme su cui vale la regola
// di aggancio R3: un codice pescato dall'estrattore generico non dice che quella richiesta esiste.
func DiFamiglia(in []CodiceTrovato) []CodiceTrovato {
	var out []CodiceTrovato
	for _, c := range in {
		if c.Origine == "famiglia" {
			out = append(out, c)
		}
	}
	return out
}

// Estrai è l'unico posto in cui un messaggio diventa candidati di codice. Fa una cosa che il vecchio
// `EstraiCodici` non faceva e che il banco reale ha reso obbligatoria: cerca PRIMA il riferimento
// della richiesta secondo il cliente, e poi toglie quel numero dai codici prodotto.
//
// «RICHIESTA D'OFFERTA 490020618» conteneva un codice prodotto 490020618 che non esiste: è il numero
// della RDO. Finiva fra gli identificativi della RFQ, e da lì nel nome della cartella e nelle
// ricerche per codice. Sono due campi diversi perché sono due cose diverse.
func (m *Motore) Estrai(testi ...Testo) Estrazione {
	e := Estrazione{HaFamiglie: m.HaFamiglie()}
	if m != nil && m.riferimento != nil {
		for _, t := range testi {
			if s := m.riferimento.FindString(t.Corpo); s != "" {
				e.Riferimento, e.RiferimentoDove = s, t.Dove
				if e.RiferimentoNome = m.rifNome; e.RiferimentoNome == "" {
					e.RiferimentoNome = "regex del cliente"
				}
				break
			}
		}
	}
	for _, c := range m.CodiciDa(testi...) {
		if e.Riferimento != "" && contieneStringa(e.Riferimento, c.Codice) {
			continue // è il riferimento della richiesta, o un suo pezzo: non è un codice prodotto
		}
		e.Codici = append(e.Codici, c)
	}
	return e
}

// contieneStringa dice se `codice` è il riferimento o ne è una parte. Il confronto è largo apposta:
// un riferimento «RDO 490020618» e un codice «490020618» sono lo stesso numero visto due volte, e
// tenerne uno solo è il punto.
func contieneStringa(riferimento, codice string) bool {
	r, c := strings.ToUpper(riferimento), strings.ToUpper(codice)
	return c != "" && (r == c || strings.Contains(r, c))
}

// Riferimento cerca il numero della richiesta del cliente (RDO, Anfrage, ODA). Restituisce il
// primo trovato e la descrizione della regola che l'ha trovato.
func (m *Motore) Riferimento(testi ...string) (string, string) {
	if m == nil || m.riferimento == nil {
		return "", ""
	}
	for _, t := range testi {
		if s := m.riferimento.FindString(t); s != "" {
			nome := m.rifNome
			if nome == "" {
				nome = "regex del cliente"
			}
			return s, nome
		}
	}
	return "", ""
}

// FrasiPortale sono le frasi del cliente IN PIÙ a quelle generiche.
func (m *Motore) FrasiPortale() []string {
	if m == nil {
		return nil
	}
	return m.frasi
}

// SoloCodici è la comodità per chi vuole l'elenco piatto (il triage, che ragiona per stringhe).
func SoloCodici(in []CodiceTrovato) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, c.Codice)
	}
	return out
}

// ---------------------------------------------------------------- minuterie

func indiceGruppo(re *regexp.Regexp, nome string) int {
	for i, n := range re.SubexpNames() {
		if n == nome {
			return i
		}
	}
	return -1
}

func contiene(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func giaVisto(visti map[string]bool, codice string) bool {
	for k := range visti {
		if strings.HasPrefix(k, codice+"\x00") {
			return true
		}
	}
	return false
}

func senzaURL(t string) string { return reURL.ReplaceAllString(t, " ") }

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

// ---------------------------------------------------------------- l'unico ingresso

// Riconoscimento è tutto ciò che il Cockpit capisce di un messaggio senza che nessuno decida:
// che cosa propone il triage, se qualcuno ha detto «è sul portale», e se c'è una scadenza.
type Riconoscimento struct {
	Triage   EsitoTriage
	Portale  []RiferimentoPortale
	Scadenza *time.Time
}

// Riconosci è l'UNICO ingresso al riconoscimento, e il motivo per cui esiste è una regola del
// blocco 3: «il banco di prova deve utilizzare il vero motore; non costruire un secondo motore
// solo per la schermata di test».
//
// Un banco di prova che chiama funzioni sue assomiglia al sistema finché qualcuno non cambia il
// sistema: da quel momento mostra un risultato che nessun messaggio vero avrà mai, e lo mostra
// proprio alla persona che sta tarando le regex fidandosi di quello che legge. Qui ce n'è una
// sola: la chiama l'ingest sui messaggi che arrivano e la chiama la schermata Anagrafica sul
// testo incollato nella casella di prova.
func Riconosci(in IngressoTriage, riferimento time.Time) Riconoscimento {
	r := Riconoscimento{
		Triage:  Triage(in),
		Portale: RilevaPortale(in.Corpo, in.Motore.FrasiPortale()...),
	}
	if d, ok := RilevaScadenza(in.Corpo, riferimento); ok {
		r.Scadenza = &d
	}
	return r
}
