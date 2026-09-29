// Package regole è lo SCHEMA di ciò che un cliente dichiara di sé: le famiglie di codice, il
// riferimento della richiesta, le frasi del portale, le convenzioni di codice. Qui stanno le due
// porte — quella in scrittura che rifiuta, quella in lettura che segna ✓/✗ — e la diagnosi che la
// schermata Anagrafica mostra riga per riga.
//
// Che cosa il Cockpit FA di queste regole non sta qui: il motore che le compila e le applica a un
// testo è `core/inbox/classificazione`, che importa questo package e non il contrario.
package regole

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"regexp/syntax"
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
	//
	// Dalla 4.13b l'evento la legge: con una parola di richiesta accanto all'ordine, l'ordine non
	// decide. La mail si legge come senza l'ordine (una richiesta nuova, ma anche un sollecito o una
	// revisione dei disegni), e l'ordine resta un'evidenza. Senza la parola di richiesta, l'ordine
	// resta un ordine.
	//
	// L'eccezione: «SI RFQ» e «SI CTE», l'oggetto delle mail del portale di questi clienti, valgono
	// come parola di richiesta SOLO accanto all'ordine, e solo per chi ha questa chiave. Da soli non
	// fanno una richiesta: una CTE può essere un cambio tecnico o un phase out (la legge la 4.16).
	// «SI RFQ» contiene già «rfq», che è una parola di richiesta per tutti: l'eccezione, in pratica,
	// cambia solo «SI CTE».
	NumeroOrdineAnticipato bool `json:"numero_ordine_anticipato,omitempty"`
	// OrdineDaVerificare è la voce più debole di NumeroOrdineAnticipato (Smistamento 4.13b, domanda
	// 22a = A): una mail di questo cliente con la parola «ordine» può essere una richiesta, e va
	// verificata. L'ordine non sparisce e non diventa una richiesta: scende da «chiaro» a
	// «probabile», e sceglie l'operatore. È per il cliente di cui si sa solo questo.
	OrdineDaVerificare bool `json:"ordine_da_verificare,omitempty"`
	// NumeroOrdine è la forma del numero d'ordine di questo cliente («ODA_0001234»), con il suo
	// esempio (Smistamento 4.13b, domanda 22b = A). Un numero così non è un codice prodotto — non si
	// propone e non vota come codice — ed è una parola d'ordine: la mail che lo porta è un ORDINE.
	// È una voce del cliente e non una regola di tutti, perché la stessa forma per un altro cliente
	// può essere un codice vero.
	NumeroOrdine *Riferimento `json:"numero_ordine,omitempty"`
	// MittentiSistema sono gli indirizzi (o gli host) da cui questo cliente manda posta che nessuna
	// persona ha scritto: avvisi di un gestionale documentale, notifiche automatiche, ordini generati
	// da un sistema (Smistamento 4.13b). Per loro l'evento è quello dichiarato qui, letto prima delle
	// parole del testo, e la mail non propone né una RFQ nuova né i suoi codici come prodotti: un
	// avviso con la parola «RFQ» nell'oggetto non è una richiesta d'offerta.
	MittentiSistema []MittenteSistema `json:"mittenti_sistema,omitempty"`
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
	// SuffissiDecorativi sono le code che i CAD di questo cliente attaccano al codice del pezzo senza
	// cambiarlo («_PRT» di una parte, «_ASM» di un assieme): «X_PRT» e «X» sono lo stesso pezzo. Si
	// tolgono dal codice letto nei nomi dei file, nei PRODUCT degli STEP e nei testi, solo per questo
	// cliente: per un altro «_PRT» puo' essere una parte vera del codice.
	SuffissiDecorativi []string `json:"suffissi_decorativi,omitempty"`
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
	//	"123456_A4" → (?P<codice>\d{6})_(?P<rev>A\d?)
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

// Riferimento è il numero della richiesta del cliente, con il suo esempio. La stessa forma vale
// per il numero d'ordine (NumeroOrdine): una regex, una descrizione, un esempio che le corrisponde.
type Riferimento struct {
	Regex       string `json:"regex"`
	Descrizione string `json:"descrizione,omitempty"`
	Esempio     string `json:"esempio"`
}

// MittenteSistema è una riga dei mittenti di sistema: chi (un indirizzo intero o un host), quale
// evento, e un esempio.
//
// L'indirizzo vale da solo («avvisi@acme.example»); l'host vale per tutta la posta che viene da
// lì, e solo da lì («server.acme.example» prende «rfq@server.acme.example», non «x@acme.example»
// né «x@altro.server.acme.example»). Un host largo quanto il dominio del cliente farebbe di ogni
// sua mail un avviso: per questo non ci sono caratteri jolly, e l'esempio obbligatorio è un
// indirizzo, non un host — si scrive da chi arriva davvero la posta. E per lo stesso motivo un
// host che è un dominio intero («acme.example») non si accetta (Verifica), né un host da cui
// scrivono i contatti del cliente o che è un suo dominio principale (MittentiSulCliente, al
// salvataggio dall'Anagrafica, che ha il database).
type MittenteSistema struct {
	Mittente    string `json:"mittente"`
	Evento      string `json:"evento"`
	Descrizione string `json:"descrizione,omitempty"`
	Esempio     string `json:"esempio"`
}

// Gli eventi che un mittente di sistema può dichiarare. Sono le stesse stringhe di
// classificazione.EventoAltro e classificazione.EventoOrdine: questo package non importa quello
// (è il contrario), e una prova là controlla che coincidano.
//
// Solo due, di proposito. Un avviso automatico e un ordine generato da un sistema sono le due
// cose che un mittente di sistema manda davvero (Smistamento 4.13b), ed entrambe non sono una
// richiesta nuova: la mail non propone una RFQ né i suoi codici come prodotti. Un evento come
// SOLLECITO o REVISIONE_CAD dichiarato per un mittente vorrebbe dire anche che cosa fa l'esito,
// ed è la fase che fa decidere l'esito dall'evento (4.15), non questa.
const (
	EventoMittenteAltro  = "ALTRO"
	EventoMittenteOrdine = "ORDINE"
)

// EventiMittenteSistema elenca i valori ammessi, per il messaggio d'errore e per la schermata.
var EventiMittenteSistema = []string{EventoMittenteAltro, EventoMittenteOrdine}

// NormaIndirizzo porta un mittente alla forma che si confronta: senza spazi, senza le parentesi
// angolari di un'intestazione («<a@b>»), in minuscolo. È la stessa per la regola, per l'esempio e
// per il mittente vero della mail: se fossero due, l'esempio passerebbe e la posta no.
func NormaIndirizzo(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(s), "<>"))
}

// Corrisponde dice se il mittente di una mail è questa riga: per un indirizzo l'uguaglianza, per
// un host l'uguaglianza con la parte dopo la chiocciola. Senza distinguere maiuscole.
func (m MittenteSistema) Corrisponde(mittente string) bool {
	regola, x := NormaIndirizzo(m.Mittente), NormaIndirizzo(mittente)
	if regola == "" || x == "" {
		return false
	}
	if strings.Contains(regola, "@") {
		return x == regola
	}
	i := strings.LastIndex(x, "@")
	return i > 0 && x[i+1:] == regola
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
	maxSuffissi     = 10
	maxMittenti     = 20
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
	if n := len(r.SuffissiDecorativi); n > maxSuffissi {
		out = append(out, Diagnostica{Regola: "suffissi_decorativi", Ok: false,
			Motivo: fmt.Sprintf("sono %d: il limite è %d", n, maxSuffissi)})
	}
	for i, x := range r.SuffissiDecorativi {
		out = append(out, verificaSuffisso(fmt.Sprintf("suffisso decorativo %d", i+1), x))
	}
	if g := r.FinestraAggancioGG; g != 0 {
		d := Diagnostica{Regola: "finestra_aggancio_gg", Dettaglio: fmt.Sprint(g), Ok: true}
		if g < 0 || g > maxFinestraGG {
			d.Ok, d.Motivo = false, fmt.Sprintf("sono giorni: %d non è un numero di giorni possibile (da 1 a %d)", g, maxFinestraGG)
		}
		out = append(out, d)
	}
	if r.NumeroOrdine != nil {
		d := verificaRegex(RegolaNumeroOrdine, r.NumeroOrdine.Descrizione, r.NumeroOrdine.Regex, r.NumeroOrdine.Esempio, false)
		// Il numero d'ordine è la sola regex del cliente che TOGLIE testo: il motore legge i codici nel
		// testo senza i numeri d'ordine (4.13b). Una regex che prende anche il testo vuoto («(ODA_\d{7})?»,
		// «\d*») passa l'esempio, e toglierebbe «il vuoto» fra ogni lettera: dalla posta e dai nomi dei
		// file di questo cliente sparirebbero tutti i codici, senza un errore. È un errore di battitura
		// (un «?» o un «*» di troppo), e la convalida è l'unica difesa (4.13b, ritocco).
		if d.Ok && prendeIlVuoto(r.NumeroOrdine.Regex) {
			d.Ok = false
			d.Motivo = "la regex prende anche il testo vuoto: il motore toglierebbe un numero d'ordine vuoto fra ogni " +
				"lettera, e dalla posta e dai nomi dei file di questo cliente sparirebbero tutti i codici. Un «?», un «*», " +
				"un «{0,…}» o un'alternativa vuota rendono facoltativo tutto il numero: la regex deve leggere almeno un " +
				"carattere (per esempio «\\bODA_\\d{7}\\b»)"
		}
		out = append(out, d)
	}
	if n := len(r.MittentiSistema); n > maxMittenti {
		out = append(out, Diagnostica{Regola: "mittenti_sistema", Ok: false,
			Motivo: fmt.Sprintf("sono %d: il limite è %d", n, maxMittenti)})
	}
	visti := map[string]int{}
	for i, m := range r.MittentiSistema {
		d := verificaMittente(RegolaMittenteSistema(i), m)
		// la stessa riga due volte, con due eventi, direbbe due cose della stessa posta: vale la prima,
		// e la seconda si segna
		if k := NormaIndirizzo(m.Mittente); d.Ok {
			if j, gia := visti[k]; gia {
				d.Ok, d.Motivo = false, fmt.Sprintf("«%s» è già il mittente di sistema %d", k, j+1)
			} else {
				visti[k] = i
			}
		}
		out = append(out, d)
	}
	return out
}

// RegolaNumeroOrdine e RegolaMittenteSistema sono i nomi delle righe della diagnosi delle voci della
// 4.13b: il motore li usa per sapere quali righe sono ✓ (classificazione.Compila).
const RegolaNumeroOrdine = "numero d'ordine"

func RegolaMittenteSistema(i int) string { return fmt.Sprintf("mittente di sistema %d", i+1) }

// verificaMittente: un mittente è un indirizzo intero o un host, senza spazi e senza jolly; l'evento
// è uno dei due ammessi; l'esempio è un indirizzo, e la riga lo prende.
func verificaMittente(nome string, m MittenteSistema) Diagnostica {
	d := Diagnostica{Regola: nome, Dettaglio: m.Descrizione, Esempio: m.Esempio}
	x := NormaIndirizzo(m.Mittente)
	if d.Dettaglio == "" {
		d.Dettaglio = x + " → " + m.Evento
	}
	switch {
	case x == "":
		d.Motivo = "manca il mittente: un indirizzo («avvisi@cliente.example») o un host («server.cliente.example»)"
		return d
	case strings.ContainsAny(x, " \t*?"):
		d.Motivo = fmt.Sprintf("«%s» contiene spazi o caratteri jolly: si scrive un indirizzo intero o un host intero", x)
		return d
	case strings.HasPrefix(x, "@"):
		d.Motivo = fmt.Sprintf("«%s»: un host si scrive senza la chiocciola («%s»)", x, strings.TrimPrefix(x, "@"))
		return d
	case strings.Count(x, "@") > 1:
		d.Motivo = fmt.Sprintf("«%s» non è un indirizzo: ha più di una chiocciola", x)
		return d
	}
	host := x
	if i := strings.Index(x, "@"); i >= 0 {
		host = x[i+1:]
	}
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		d.Motivo = fmt.Sprintf("«%s» non è un host: serve un nome con il punto («server.cliente.example»)", host)
		return d
	}
	if !contiene(EventiMittenteSistema, m.Evento) {
		d.Motivo = fmt.Sprintf("l'evento «%s» non è previsto: i valori sono %s. Un mittente di sistema manda avvisi "+
			"o ordini, e nessuno dei due è una richiesta nuova", m.Evento, strings.Join(EventiMittenteSistema, " e "))
		return d
	}
	e := NormaIndirizzo(m.Esempio)
	if e == "" {
		d.Motivo = "manca l'esempio, e senza esempio nessuno si accorge il giorno in cui il mittente non corrisponde più"
		return d
	}
	if i := strings.LastIndex(e, "@"); i <= 0 || i == len(e)-1 {
		d.Motivo = fmt.Sprintf("l'esempio «%s» non è un indirizzo: si scrive da chi arriva davvero la posta", m.Esempio)
		return d
	}
	if !m.Corrisponde(e) {
		d.Motivo = fmt.Sprintf("l'esempio «%s» non corrisponde al mittente «%s»: uno dei due è sbagliato, e finché non si sa quale la regola non si usa", m.Esempio, x)
		return d
	}
	// Un host che è un dominio intero, senza un nome davanti («acme.example», non «server.acme.example»):
	// l'esempio «avvisi@acme.example» lo mostra come il dominio da cui scrive chi ha quell'indirizzo, cioè
	// di solito anche il buyer. La riga prenderebbe tutta la posta del cliente, anche le richieste, e ne
	// farebbe avvisi ignorati o ordini (4.13b, ritocco). Per un indirizzo solo si scrive l'indirizzo.
	if !strings.Contains(x, "@") && dominioIntero(x) {
		d.Motivo = fmt.Sprintf("«%s» è un dominio intero, non l'host di un sistema: prende ogni indirizzo @%s come l'esempio «%s», "+
			"anche quelli dei buyer, e farebbe di tutta la posta del cliente, richieste comprese, %s. Scrivi l'indirizzo "+
			"del sistema («%s») o il nome intero del suo host («server.%s»)", x, x, m.Esempio, cheCosaDiventa(m.Evento), e, x)
		return d
	}
	d.Ok = true
	return d
}

// cheCosaDiventa è la posta di un mittente di sistema a parole, per i motivi: l'evento dichiarato come lo
// vede l'operatore.
func cheCosaDiventa(evento string) string {
	if evento == EventoMittenteOrdine {
		return "un ordine"
	}
	return "un avviso ignorato"
}

// suffissiComposti sono le parti di mezzo dei domini nazionali in cui il nome registrato ne ha tre
// («acme.co.uk», «acme.com.br»): lì anche il nome con tre parti è il dominio intero, non un host.
var suffissiComposti = map[string]bool{"co": true, "com": true, "net": true, "org": true, "gov": true, "edu": true, "ac": true}

// dominioIntero dice se un host è un dominio intero, senza un sottodominio: due parti («acme.example»),
// o tre con un suffisso nazionale composto («acme.co.uk»). È una regola di forma: un dominio del cliente
// più lungo lo riconosce il salvataggio dall'Anagrafica, che ha il database (MittentiSulCliente).
func dominioIntero(host string) bool {
	p := strings.Split(host, ".")
	switch len(p) {
	case 0, 1, 2:
		return true
	case 3:
		return len(p[2]) == 2 && suffissiComposti[p[1]]
	}
	return false
}

// ChiScriveDalCliente è ciò che l'Anagrafica sa di chi scrive per un cliente: i suoi domini (Anagrafica ›
// Domini, `dominio_cliente`) e i suoi contatti (`buyer`). Lo legge chi ha il database, il salvataggio
// dall'Anagrafica, e lo passa a MittentiSulCliente.
type ChiScriveDalCliente struct {
	Domini   []string
	Contatti []Contatto
}

// Contatto è una persona censita del cliente: il nome come lo legge l'operatore, e il suo indirizzo.
type Contatto struct {
	Nome, Indirizzo string
}

// MittentiSulCliente è il controllo dei mittenti di sistema che Verifica non può fare, perché vuole il
// database (4.13b, ritocco): un host da cui scrivono anche le persone del cliente. Le regole le scrive una
// persona dall'Anagrafica (domanda 14 = A), e contro un errore di battitura c'è solo la convalida. Rifiuta un
// host che è:
//   - il dominio dell'indirizzo di un contatto del cliente: la riga prenderebbe anche la sua posta;
//   - un dominio del cliente che nessun altro suo dominio contiene: è un suo dominio principale, quello da
//     cui scrivono i buyer, anche con più di due parti («acme-italia.gruppo.example»).
//
// Un dominio del cliente che sta dentro un altro suo dominio («server.acme.example» con «acme.example») è
// invece il nome di un sistema, e deve poterci stare: il riconoscimento della controparte legge il dominio
// intero del mittente, senza salire di livello, e la posta di «rfq@server.acme.example» arriva alle regole
// del cliente (e alla riga del mittente di sistema) solo se «server.acme.example» è fra i suoi domini, o se
// l'indirizzo è un suo contatto. Per lo stesso motivo non si guardano i mittenti della posta già attribuita
// al cliente: la posta del sistema ci sta per forza. Le regole arrivano già convalidate (ValidaRegole); nil
// se nessuna riga è un host.
func MittentiSulCliente(r Regole, c ChiScriveDalCliente) error {
	domini := map[string]bool{}
	for _, d := range c.Domini {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			domini[d] = true
		}
	}
	for i, m := range r.MittentiSistema {
		host := NormaIndirizzo(m.Mittente)
		if host == "" || strings.Contains(host, "@") {
			continue // un indirizzo vale da solo: non prende la posta di nessun altro
		}
		for _, p := range c.Contatti {
			ind := NormaIndirizzo(p.Indirizzo)
			if j := strings.LastIndex(ind, "@"); j > 0 && ind[j+1:] == host {
				return fmt.Errorf("regole: %s: «%s» è il dominio di %s (%s), un contatto di questo cliente: la riga prenderebbe "+
					"anche la sua posta, e le sue richieste diventerebbero %s. Scrivi l'indirizzo del sistema, non l'host",
					RegolaMittenteSistema(i), host, p.Nome, ind, cheCosaDiventa(m.Evento))
			}
		}
		if domini[host] && !dentroUnAltroDominio(host, domini) {
			return fmt.Errorf("regole: %s: «%s» è un dominio di questo cliente (Anagrafica › Domini) e non sta dentro un altro "+
				"suo dominio: è quello da cui scrivono i buyer, e la riga farebbe di tutta la loro posta, richieste comprese, %s. "+
				"Scrivi l'indirizzo del sistema («avvisi@%s»), o il nome del suo host dentro il dominio («server.%s»)",
				RegolaMittenteSistema(i), host, cheCosaDiventa(m.Evento), host, host)
		}
	}
	return nil
}

// dentroUnAltroDominio dice se l'host sta dentro un altro dei domini dati: «server.acme.example» dentro
// «acme.example». Non «xacme.example»: il confine è il punto.
func dentroUnAltroDominio(host string, domini map[string]bool) bool {
	for d := range domini {
		if d != host && strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// codaDaRevisione e' la coda che il riconoscimento dei codici legge come revisione (classificazione.CodiceRev):
// «_1», «-B», «_R2», «_REV1», e anche «_RT», dove la R e' il prefisso della revisione T. Un suffisso
// decorativo fatto cosi' toglierebbe una revisione vera.
var codaDaRevisione = regexp.MustCompile(`(?i)^[_\-.](?:REV|R)?(?:[0-9]{1,2}|[A-Z])$`)

// verificaSuffisso: un suffisso decorativo comincia con un separatore (_ - .), ha almeno due caratteri dopo,
// non ha spazi, sta in MaxSuffisso e non e' una coda che si legge come revisione.
func verificaSuffisso(nome, x string) Diagnostica {
	d := Diagnostica{Regola: nome, Dettaglio: x, Ok: true}
	s := strings.TrimSpace(x)
	switch {
	case s == "":
		d.Ok, d.Motivo = false, "è vuoto"
	case strings.ContainsAny(s, " \t"):
		d.Ok, d.Motivo = false, "contiene spazi"
	case !strings.ContainsAny(s[:1], "_-."):
		d.Ok, d.Motivo = false, fmt.Sprintf("«%s» deve cominciare con _ - oppure . (la coda attaccata al codice)", s)
	case len(s) < 3:
		d.Ok, d.Motivo = false, fmt.Sprintf("«%s» è troppo corto: dopo il separatore servono almeno due caratteri", s)
	case len(s) > MaxSuffisso:
		d.Ok, d.Motivo = false, fmt.Sprintf("«%s» è più lungo di %d caratteri", s, MaxSuffisso)
	case codaDaRevisione.MatchString(s):
		d.Ok, d.Motivo = false, fmt.Sprintf("«%s» si legge come una revisione (_1, _B, _R2, _REV1): toglierlo toglierebbe la revisione", s)
	}
	return d
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

// prendeIlVuoto dice se una regex (che compila) può trovare il testo vuoto. Non basta chiederlo al testo
// vuoto (MatchString("")): «\b\d*\b» sul testo vuoto non trova niente, perché lì non c'è un confine di
// parola, ma in «pezzo 7120001» trova il vuoto a ogni confine. Si guarda allora la forma della regex: può
// finire senza aver letto un carattere se ogni pezzo di una sequenza può, se può una delle alternative, se il
// pezzo ripetuto è facoltativo («?», «*», «{0,n}»). Le ancore e i confini («^», «$», «\b») non leggono
// caratteri: si contano come vuoti, perché da qualche parte in un testo sono veri.
func prendeIlVuoto(espressione string) bool {
	re, err := regexp.Compile(espressione)
	if err != nil {
		return false // una regex che non compila la segna già verificaRegex
	}
	if re.MatchString("") {
		return true
	}
	s, err := syntax.Parse(espressione, syntax.Perl) // le stesse regole di regexp.Compile
	return err == nil && puoEssereVuota(s)
}

func puoEssereVuota(s *syntax.Regexp) bool {
	switch s.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpStar, syntax.OpQuest:
		return true
	case syntax.OpLiteral:
		return len(s.Rune) == 0
	case syntax.OpCapture, syntax.OpPlus:
		return puoEssereVuota(s.Sub[0])
	case syntax.OpRepeat:
		return s.Min == 0 || puoEssereVuota(s.Sub[0])
	case syntax.OpConcat:
		for _, x := range s.Sub {
			if !puoEssereVuota(x) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, x := range s.Sub {
			if puoEssereVuota(x) {
				return true
			}
		}
	}
	// un carattere, una classe, un punto: leggono sempre qualcosa (OpNoMatch non trova niente)
	return false
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
