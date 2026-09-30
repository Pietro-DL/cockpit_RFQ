package classificazione

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"promatec/cockpit/internal/core/registro/regole"
)

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
	suffissi    []string // i suffissi decorativi del cliente, in maiuscolo, il piu' lungo prima
	// Smistamento 4.13b: il numero d'ordine nella forma del cliente e i mittenti di sistema (solo le
	// righe ✓, gli indirizzi prima degli host)
	ordine     *regexp.Regexp
	ordineNome string
	mittenti   []regole.MittenteSistema
	Regole     regole.Regole
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
func Compila(cliente string, r regole.Regole) *Motore {
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
	for i, x := range r.SuffissiDecorativi {
		if ok(fmt.Sprintf("suffisso decorativo %d", i+1)) {
			m.suffissi = append(m.suffissi, strings.ToUpper(strings.TrimSpace(x)))
		}
	}
	// il piu' lungo prima: «_PRT_ASM» non deve perdere solo «_ASM»
	sort.SliceStable(m.suffissi, func(i, j int) bool { return len(m.suffissi[i]) > len(m.suffissi[j]) })
	if r.NumeroOrdine != nil && ok(regole.RegolaNumeroOrdine) {
		m.ordine = regexp.MustCompile(r.NumeroOrdine.Regex)
		if m.ordineNome = r.NumeroOrdine.Descrizione; m.ordineNome == "" {
			m.ordineNome = "regex del cliente"
		}
	}
	for i, x := range r.MittentiSistema {
		if ok(regole.RegolaMittenteSistema(i)) {
			m.mittenti = append(m.mittenti, x)
		}
	}
	// l'indirizzo prima dell'host: «ordini@server.acme.example» dice di più di «server.acme.example»
	sort.SliceStable(m.mittenti, func(i, j int) bool {
		return strings.Contains(m.mittenti[i].Mittente, "@") && !strings.Contains(m.mittenti[j].Mittente, "@")
	})
	return m
}

// ---------------------------------------------------------------- posta: le voci della 4.13b

// MittenteDiSistema dice se il mittente di una mail è uno dei mittenti di sistema del cliente, e quale
// riga lo prende (Smistamento 4.13b). Un motore nil, o senza la voce, non ne ha.
func (m *Motore) MittenteDiSistema(mittente string) (regole.MittenteSistema, bool) {
	if m == nil {
		return regole.MittenteSistema{}, false
	}
	for _, x := range m.mittenti {
		if x.Corrisponde(mittente) {
			return x, true
		}
	}
	return regole.MittenteSistema{}, false
}

// OrdineAnticipato è `numero_ordine_anticipato`: il cliente emette il numero d'ordine prima dell'offerta.
func (m *Motore) OrdineAnticipato() bool { return m != nil && m.Regole.NumeroOrdineAnticipato }

// OrdineDaVerificare è `ordine_da_verificare`, la voce più debole: un ordine di questo cliente è probabile.
func (m *Motore) OrdineDaVerificare() bool { return m != nil && m.Regole.OrdineDaVerificare }

// NumeriOrdine sono i numeri d'ordine nella forma del cliente (`numero_ordine`) trovati in un testo, nel
// loro ordine. Il testo si legge com'è, con le maiuscole: la regex è del cliente.
//
// Una corrispondenza vuota non è un numero d'ordine, e si salta (4.13b, ritocco). La convalida rifiuta già
// una regex che prende il vuoto (regole.Verifica), e Compila non la usa; questa è la seconda difesa, per
// un motore che la regola la riceva per un'altra strada. Senza, «(ODA_\d{7})?» dava un vuoto a ogni
// lettera, e il primo vuoto nascondeva l'ordine vero che veniva dopo (trovaNumeroOrdine legge il primo).
func (m *Motore) NumeriOrdine(t string) []string {
	if m == nil || m.ordine == nil {
		return nil
	}
	var out []string
	for _, n := range m.ordine.FindAllString(senzaURL(t), -1) {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// NomeNumeroOrdine è la descrizione della voce `numero_ordine`, per l'evidenza.
func (m *Motore) NomeNumeroOrdine() string {
	if m == nil {
		return ""
	}
	return m.ordineNome
}

// senzaNumeriOrdine è il testo senza i numeri d'ordine del cliente: ogni occorrenza diventa uno spazio. Un
// codice si legge solo fuori da lì: in «ordine ODA_7120001 per il pezzo 7120001» il pezzo resta, quello
// dentro l'ordine no. Senza la voce il testo resta com'è.
//
// Una corrispondenza vuota non toglie niente e non aggiunge lo spazio (4.13b, ritocco): con una regex che
// prende il vuoto, ReplaceAllString metteva uno spazio fra ogni carattere («7120001» → « 7 1 2 0 0 0 1 »), e
// dalla mail e dai nomi dei file spariva ogni codice. Come in NumeriOrdine, è la seconda difesa.
func (m *Motore) senzaNumeriOrdine(t string) string {
	if m == nil || m.ordine == nil {
		return t
	}
	return m.ordine.ReplaceAllStringFunc(t, func(n string) string {
		if n == "" {
			return ""
		}
		return " "
	})
}

// dentroUnOrdine dice se un codice letto in un testo (il nome di un file) sta soltanto dentro i numeri
// d'ordine del cliente: è uno di loro o un loro pezzo (il confronto largo del riferimento, contieneStringa), e
// tolti i numeri d'ordine dal testo non c'è più. Un codice che il testo cita anche fuori dall'ordine resta un
// codice. Serve dove il codice è già stato letto (la valutazione del nome, i `codici_nel_nome` già scritti);
// il testo di una mail invece si legge già senza i numeri d'ordine (Estrai).
func (m *Motore) dentroUnOrdine(testo, codice string) bool {
	for _, o := range m.NumeriOrdine(testo) {
		if contieneStringa(o, codice) {
			return !contieneStringa(m.senzaNumeriOrdine(testo), codice)
		}
	}
	return false
}

// CitatiNelNome sono i codici che il nome di un file cita senza esserlo (PropostaDaNome.CodiciNelNome, o
// quelli già scritti in `codici_nel_nome`) come si conservano e si suggeriscono: senza il suffisso decorativo
// del cliente, e senza i suoi numeri d'ordine (4.13b). «Ordine ODA_0001234.pdf» non cita un codice: cita
// l'ordine. Nil se non ne resta nessuno.
func (m *Motore) CitatiNelNome(nome string, citati []string) []string {
	base := strings.TrimSuffix(nome, path.Ext(nome))
	var out []string
	for _, c := range citati {
		if m.dentroUnOrdine(base, c) {
			continue
		}
		out = append(out, m.CanonicoNome(c))
	}
	return out
}

// togliSuffisso toglie una volta, senza distinguere maiuscole, il primo suffisso decorativo del cliente che
// chiude il codice. ok = false se non ce n'e'.
func (m *Motore) togliSuffisso(codice string) (string, bool) {
	if m == nil || len(m.suffissi) == 0 {
		return codice, false
	}
	c := strings.TrimSpace(codice)
	for _, s := range m.suffissi {
		if len(c) > len(s) && strings.EqualFold(c[len(c)-len(s):], s) {
			return c[:len(c)-len(s)], true
		}
	}
	return codice, false
}

// senzaSuffissi toglie da un testo i suffissi decorativi del cliente attaccati in coda a una parola («77720000_PRT.pdf»
// → «77720000.pdf»): prima della coda una lettera o una cifra, dopo nessuna. Senza suffissi il testo resta com'e'.
func (m *Motore) senzaSuffissi(t string) string {
	if m == nil || len(m.suffissi) == 0 {
		return t
	}
	alnum := func(b byte) bool { return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }
	var out strings.Builder
	for i := 0; i < len(t); {
		tolto := false
		if i > 0 && alnum(t[i-1]) {
			for _, s := range m.suffissi {
				if j := i + len(s); j <= len(t) && strings.EqualFold(t[i:j], s) && (j == len(t) || !alnum(t[j])) {
					i, tolto = j, true
					break
				}
			}
		}
		if !tolto {
			out.WriteByte(t[i])
			i++
		}
	}
	return out.String()
}

// Canonico toglie dal codice letto in un nome di file, in un testo o nel risultato di un'analisi un suffisso
// decorativo del cliente: i CAD attaccano al nome del pezzo il tipo di file («…_PRT», «…_ASM») e il pezzo
// resta lo stesso. Se la revisione non e' nota, quello che resta si rilegge con CodiceRev («X_B_PRT» → X,
// rev B). Quello che resta deve avere ancora la forma di un codice (sembraCodice): «1234_PRT» resta com'e'.
// Un motore nil, o senza suffissi, lascia il codice com'e': per chi non li dichiara non cambia niente.
func (m *Motore) Canonico(codice, rev string) (string, string) {
	base, ok := m.togliSuffisso(codice)
	if !ok || !sembraCodice(base) {
		return codice, rev
	}
	if rev == "" {
		if k, r := CodiceRev(base); r != "" && sembraCodice(k) {
			return k, r
		}
	}
	return base, rev
}

// CanonicoNome e' Canonico per un codice letto in un nome di file, dove il suffisso puo' stare anche prima
// della revisione: «X_PRT_B» e' X (la revisione, se c'e', la dice chi legge il nome). Senza suffissi, com'e'.
func (m *Motore) CanonicoNome(codice string) string {
	if k, _ := m.Canonico(codice, ""); !strings.EqualFold(k, strings.TrimSpace(codice)) {
		return k
	}
	if k, r := CodiceRev(codice); r != "" {
		if b, _ := m.Canonico(k, r); !strings.EqualFold(b, k) {
			return b
		}
	}
	return codice
}

// HaSuffissi dice se il cliente dichiara suffissi decorativi utilizzabili.
func (m *Motore) HaSuffissi() bool { return m != nil && len(m.suffissi) > 0 }

// SuffissiDecorativi sono i suffissi decorativi utilizzabili del cliente, in maiuscolo.
func (m *Motore) SuffissiDecorativi() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.suffissi...)
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
	// Dove è l'evidenza leggibile: «oggetto», «corpo», «allegato 7654321A_1.zip».
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
	// NessunProdotto dice perché nessun codice di questo messaggio si propone come prodotto: la mail
	// viene da un mittente di sistema del cliente (Smistamento 4.13b). I codici restano tutti visti
	// (Altri), con il ruolo «non classificato», e per entrare serve un clic. Vuoto = la regola di sempre.
	NessunProdotto string
}

// senzaProdotti è l'estrazione di un mittente di sistema: gli stessi codici, nessuno proponibile.
func (e Estrazione) senzaProdotti(perche string) Estrazione {
	e.NessunProdotto = perche
	cc := make([]CodiceTrovato, len(e.Codici))
	for i, c := range e.Codici {
		c.Ruolo = RuoloIgnoto
		cc[i] = c
	}
	e.Codici = cc
	return e
}

// Proponibili sono i codici che possono diventare identificativi della RFQ se l'operatore li spunta.
//
// La regola, dal banco reale: se il cliente HA famiglie dichiarate, l'estrattore generico non
// propone niente. Un cliente censito che manda un codice di forma nuova è un caso da guardare, non
// da indovinare; e nel frattempo ogni CAP, numero d'ordine, data e misura del piè di pagina smette
// di presentarsi come codice prodotto. I numeri generici restano visibili sotto «altri numeri
// trovati», non spuntati: si vedono, e per entrare serve un clic.
func (e Estrazione) Proponibili() []CodiceTrovato {
	if e.NessunProdotto != "" {
		return nil
	}
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

// Altri sono i numeri visti ma non proponibili: si mostrano, non si spuntano. Per un mittente di
// sistema sono tutti (NessunProdotto).
func (e Estrazione) Altri() []CodiceTrovato {
	if e.NessunProdotto != "" {
		return e.Codici
	}
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
				// la famiglia legge il testo senza i suffissi decorativi: con un gruppo della revisione, «X_PRT»
				// darebbe la revisione P
				for _, g := range f.re.FindAllStringSubmatch(m.senzaSuffissi(senzaURL(t.Corpo)), -1) {
					c := CodiceTrovato{Codice: g[0], Origine: "famiglia", Famiglia: f.nome,
						Ruolo: f.ruolo, Punteggio: PuntiFamiglia, Dove: t.Dove}
					if f.iCodice > 0 && f.iCodice < len(g) {
						c.Codice = g[f.iCodice]
					}
					if f.rev && f.iRev > 0 && f.iRev < len(g) {
						c.Rev = g[f.iRev]
					}
					// una famiglia dice gia' dov'e' il codice: si toglie solo il suffisso, senza rileggere la coda
					if b, ok := m.togliSuffisso(c.Codice); ok && b != "" {
						c.Codice = b
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
			cod, rev = m.Canonico(cod, rev)
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
// internal/core/inbox/aggancio). Nil-safe come tutto il resto del motore: un cliente sconosciuto è il caso normale.
//
// Un valore fuori da 1..maxFinestraGG vale come non dichiarato, come ogni regola ✗ del motore (Compila):
// le regole lette dal database non sono passate per forza dal convalidatore, e una finestra di
// novantamila giorni — o negativa — non è una politica del cliente, è un errore di battitura che
// trasformerebbe ogni riquotazione di dieci anni fa in un candidato, o nessun messaggio in uno.
func (m *Motore) Finestra() int {
	if m == nil {
		return 0
	}
	if g := m.Regole.FinestraAggancioGG; g >= 1 && g <= maxFinestraGG {
		return g
	}
	return 0
}

// maxFinestraGG è lo stesso limite con cui regole.Verifica rifiuta `finestra_aggancio_gg`: dieci anni.
const maxFinestraGG = 3650

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
// «RICHIESTA D'OFFERTA 400012345» conteneva un codice prodotto 400012345 che non esiste: è il numero
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
	// Smistamento 4.13b: il numero d'ordine nella forma del cliente («ODA_0001234») non è un codice
	// prodotto, come il riferimento. L'estrattore generico lo prendeva intero e lo proponeva. A differenza
	// del riferimento, che si confronta per testo (contieneStringa), l'ordine si toglie per posizione: i
	// codici si leggono nel testo senza i numeri d'ordine, e un 7120001 citato anche fuori da «ODA_7120001»
	// resta un codice. Il testo con l'ordine serve solo a chi legge l'ordine (E4), non a chi legge i codici.
	testiCodici := testi
	if m != nil && m.ordine != nil {
		testiCodici = make([]Testo, len(testi))
		for i, t := range testi {
			t.Corpo = m.senzaNumeriOrdine(t.Corpo)
			testiCodici[i] = t
		}
	}
	for _, c := range m.CodiciDa(testiCodici...) {
		if e.Riferimento != "" && contieneStringa(e.Riferimento, c.Codice) {
			continue // è il riferimento della richiesta, o un suo pezzo: non è un codice prodotto
		}
		e.Codici = append(e.Codici, c)
	}
	return e
}

// contieneStringa dice se `codice` è il riferimento o ne è una parte. Il confronto è largo apposta:
// un riferimento «RDO 400012345» e un codice «400012345» sono lo stesso numero visto due volte, e
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

func giaVisto(visti map[string]bool, codice string) bool {
	for k := range visti {
		if strings.HasPrefix(k, codice+"\x00") {
			return true
		}
	}
	return false
}

func senzaURL(t string) string { return reURL.ReplaceAllString(t, " ") }

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
