package domain

import (
	"fmt"
	"regexp"
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
	Regole      regole.Regole
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
// internal/core/inbox/aggancio). Nil-safe come tutto il resto del motore: un cliente sconosciuto è il caso normale.
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
