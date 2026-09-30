// Package censimento e' la raccolta delle forme (Smistamento, giro 4, fase 4.17a): per ogni cliente, come sono
// fatti i codici, i nomi dei file, gli oggetti e i riferimenti che arrivano, e che cosa ne hanno deciso le
// persone.
//
// Perche' esiste. Le regole di un cliente (famiglie, suffissi, riferimenti) si scrivevano guardando i casi uno
// per uno, e un caso non dice se e' la regola o il refuso. Le risposte 2, 5, 13 e 14 del 29/09 chiedono di
// censire le forme di ogni cliente e di scrivere solo cio' che e' sicuro: qui si contano, per fonte, e si
// guardano accanto alle decisioni delle persone. Si rilancia prima e dopo ogni commit che cambia l'estrazione
// (4.8, 4.9, 4.14, 4.16): i dati nel database sono gli stessi, il motore di oggi no, e la differenza fra i due
// export (Markdown) e' il «prima e dopo» del codice. Dentro lo stesso export il motore di oggi sta anche accanto a
// quello dell'arrivo: i candidati di codice e le proposte di file salvati allora, riletti con il codice di adesso.
//
// Che cosa NON fa. Non scrive niente e non propone niente: le code dopo un codice deciso sono «alias
// candidati» da guardare, le parole dei nomi dei commerciali sono suggerimenti, mai un tipo. Il tipo di un
// pezzo lo decide una persona (decisioni del 27/09 «ter», risposta 30 del 29/09); qui si conta che cosa ha
// deciso, e quando la 4.4a.1 proporra' il tipo «particolare commerciale» si contera' anche quante proposte ha
// scartato (Minuteria).
//
// Le parti: Leggi legge il database in una transazione di sola lettura e restituisce l'Ingresso; Aggrega,
// pura, fa i conti; Markdown scrive l'export. Nessuna ha un'ora dentro, tranne quella che le si passa.
package censimento

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

// Le fonti di una stringa, nell'ordine delle colonne.
const (
	FonteMail      = "mail"      // un codice che il motore di oggi trova nell'oggetto o nel corpo di una mail del cliente
	FonteNome      = "nome"      // il nome di un file tecnico del cliente, senza estensione
	FonteStep      = "step"      // il PRODUCT di uno STEP: l'id, o il nome se l'id manca
	FonteCartiglio = "cartiglio" // il codice o il numero di disegno letto nel probabile cartiglio di un PDF
	FonteDistinta  = "distinta"  // un codice deciso da una persona: pezzo, prodotto confermato, documento
)

// Fonti sono le fonti in ordine di colonna.
var Fonti = []string{FonteMail, FonteNome, FonteStep, FonteCartiglio, FonteDistinta}

// Quante voci si mostrano per elenco: il resto si conta (Resto). Sono gli stessi numeri per la pagina e per
// l'export, cosi' che quello che si legge in pagina sia quello che si confronta fra due export.
const (
	MaxForme       = 40
	MaxAggiunte    = 30
	MaxOggetti     = 25
	MaxRiferimenti = 25
	MaxMittenti    = 20
	MaxParole      = 30
	MaxFirme       = 20
	MaxLetture     = 25
	MaxDifferenze  = 20
	MaxMinuteria   = 30
	numEsempi      = 3
	// minDeciso e' la lunghezza minima di un codice deciso per cercarlo dentro un'altra stringa: un codice
	// di quattro caratteri sta dentro troppi nomi per dire qualcosa della sua coda.
	minDeciso = 5
	// maxCodiceSalvato e' la larghezza di candidato_codice.codice: l'ingest taglia li' (aggancio.SalvaCandidatiCodice).
	maxCodiceSalvato = 60
)

// ---------------------------------------------------------------- l'ingresso (quello che Leggi legge)

// Ingresso e' tutto quello che il censimento guarda, gia' attribuito a un cliente.
type Ingresso struct {
	Clienti  []Cliente
	Messaggi []Messaggio
	File     []File
	Nodi     []Nodo
	Campi    []Campo
	Pezzi    []Pezzo
	Decisi   []Deciso
	// DominiNonCensiti sono i domini dei mittenti che l'anagrafica non conosce, gia' contati.
	DominiNonCensiti []Voce
	// SolaLettura e' `transaction_read_only` letto dentro la transazione di Leggi.
	SolaLettura bool
}

// Cliente e' un cliente dell'anagrafica con le sue regole, com'e' in database.
type Cliente struct {
	ID      uuid.UUID
	Nome    string // la cartella sul NAS: il nome breve con cui lo si cerca
	Ragione string
	Attivo  bool
	Regole  json.RawMessage
}

// Messaggio e' una mail che il cliente ha scritto (in entrata, non di un fornitore). Thread e' uuid.Nil per un
// messaggio non ancora in una RFQ.
type Messaggio struct {
	Cliente, Thread uuid.UUID
	Oggetto, Corpo  string
	Troncato        bool     // il corpo e' stato tagliato: dei codici salvati restano quelli dell'oggetto e degli allegati
	Allegati        []string // i nomi dei file diretti
	Mittente        string
	ViaDominio      bool // del cliente per il dominio, non come buyer censito
	// Interpretato: l'ingest l'ha letto all'arrivo. Estratto: di quella lettura resta un'estrazione (un esito
	// diverso da «ignora», o almeno un candidato di codice salvato); i rami del triage che rispondono «ignora»
	// senza estrarre non salvano niente, e il loro messaggio non si confronta (Mail.SenzaEstrazione). Salvati
	// sono i codici che ha trovato allora (con le regole e il codice di allora), senza il riferimento della
	// richiesta.
	Interpretato bool
	Estratto     bool
	Salvati      []string
}

// File e' un file del cliente, con la decisione di una persona sullo stesso contenuto nella stessa RFQ e la
// proposta di file salvata. Contenuto e' lo sha256 ("" per un file non ancora scaricato): lo stesso nome con
// un contenuto nuovo nella stessa RFQ (una revisione rimandata) e' un altro file, con la sua decisione.
type File struct {
	Cliente, Thread uuid.UUID
	Nome            string
	Contenuto       string
	Deciso          bool
	Codice, Rev     string // del documento deciso; vuoti se non c'e' o se non li porta
	Proposto        bool
	CodiceProposto  string
	RevProposta     string
}

// Nodo e' un PRODUCT di uno STEP proposto in una RFQ.
type Nodo struct {
	Cliente, Thread uuid.UUID
	ID, Nome        string
}

// Campo e' un campo del probabile cartiglio di un PDF (codice o numero di disegno), come lo da' la
// classificazione (CampiIdentificativiDelPDF). Contenuto e' lo sha256 del file: la stessa lettura dello stesso
// file nella stessa RFQ si conta una volta, e la lettura dell'OCR e quella del testo nativo sono due letture.
type Campo struct {
	Cliente, Thread uuid.UUID
	Contenuto       string
	Etichetta       string
	Valore          string
	OCR             bool
}

// Pezzo e' un pezzo della distinta deciso da una persona. Deciso e' il tipo che ha scelto; Proposto e Motivo
// sono il tipo che il motore aveva proposto e perche': vuoti finche' il motore non propone il tipo (la
// proposta «particolare commerciale» arriva con la fase 4.4a.1). Le due colonne stanno insieme apposta:
// «proposto commerciale, deciso particolare» e' una proposta scartata dall'ingegnere, il «mancato» che si
// misura per cliente come per la cache (risposta 30 del 29/09 sera).
type Pezzo struct {
	Cliente, Thread uuid.UUID
	Codice, Rev     string
	Nomi            []string // la descrizione e i PRODUCT dei nodi accettati per questo pezzo
	Deciso          db.TipoComponente
	Proposto        db.TipoComponente
	Motivo          string
}

// Deciso e' un altro codice deciso da una persona: un prodotto della richiesta confermato, il codice di un
// documento confermato.
type Deciso struct {
	Cliente, Thread uuid.UUID
	Codice          string
}

// ---------------------------------------------------------------- il risultato

// Censimento e' il risultato: una scheda per cliente, nell'ordine dell'anagrafica, piu' i domini che
// l'anagrafica non conosce.
type Censimento struct {
	Schede           []Scheda
	DominiNonCensiti []Voce
	SolaLettura      bool
}

// Voce e' una riga di un elenco: il testo, quante volte, e qualche esempio (i primi in ordine alfabetico:
// due export dello stesso database sono uguali riga per riga, qualunque sia l'ordine delle righe lette).
type Voce struct {
	Testo  string
	N      int
	Esempi []string
}

// Resto e' quello che un elenco non mostra: quante voci e quante occorrenze.
type Resto struct {
	Voci, N int
}

// Scheda e' il censimento di un cliente.
type Scheda struct {
	Cliente Cliente
	// Quante righe ha guardato, per fonte. NFile sono i file tecnici, uno per (RFQ, nome com'e' scritto,
	// contenuto): lo stesso file arrivato due volte e' uno, lo stesso nome con un contenuto nuovo due. NCampiOCR
	// sono i campi del cartiglio letti con l'OCR: si contano qui e non altrove, perche' una lettura OCR e' un
	// indizio (F9) e una forma sbagliata di una cifra diventerebbe una forma nuova del cliente.
	NMessaggi, NFile, NNodi, NCampi, NCampiOCR, NPezzi int

	// Forme per fonte. Ogni stringa si conta una volta per RFQ (per i messaggi e i file non ancora in una
	// RFQ, una volta per cliente): una catena di dieci risposte con lo stesso codice non lo conta dieci volte.
	Forme      []FormaVista
	AltreForme Resto

	// Teste e code intorno a un codice deciso da una persona nella stessa RFQ, nei nomi dei file, negli
	// STEP e nei cartigli. Uguali sono le stringhe identiche a un codice deciso.
	Code, Teste           []Aggiunta
	AltreCode, AltreTeste Resto
	Uguali                int

	Mail      Mail
	Nome      LettureNome
	Firme     Firme
	Minuteria Minuteria

	Oggetti          []Voce
	AltriOggetti     Resto
	Riferimenti      []Voce
	AltriRiferimenti Resto
	// Mittenti sono gli indirizzi del dominio del cliente che non sono buyer censiti.
	Mittenti      []Voce
	AltriMittenti Resto
}

// FormaVista e' una forma con i suoi conteggi per fonte.
type FormaVista struct {
	Forma  string
	Per    map[string]int
	Totale int
	Esempi []string
}

// Aggiunta e' una testa o una coda intorno a un codice deciso: «_PRT» dopo «7120001», «P» prima di
// «7120001». Alias dice che e' un alias candidato: vista con almeno 2 pezzi e in almeno 2 RFQ, cioe' non un
// refuso di una volta (piano del giro 4, 4.17a).
type Aggiunta struct {
	Testo  string
	Forma  string
	N      int
	Pezzi  int
	RFQ    int
	Fonti  []string
	Esempi []string
	Alias  bool
}

// Mail e' quello che il motore di oggi legge nella posta del cliente: il «prima e dopo» della triage.
type Mail struct {
	Messaggi int
	// ConCodice: messaggi con almeno un codice proponibile. Proponibili e Altri sono i codici che la triage
	// propone e quelli che mostra senza proporre (Estrazione.Proponibili e Altri), sommati sui messaggi.
	ConCodice, Proponibili, Altri int
	// ConRiferimento: messaggi in cui la regola del riferimento del cliente trova il numero della richiesta.
	ConRiferimento   int
	FormeRiferimento []Voce
	// Il motore di oggi accanto a quello dell'arrivo, sui messaggi di cui l'ingest ha salvato un'estrazione
	// (Interpretati): i codici salvati allora (Salvati), quelli che oggi si trovano e allora no (Nuovi) e quelli
	// che allora c'erano e oggi no (Persi), con qualche esempio. E' il «prima e dopo» che non chiede due export:
	// la differenza fra il codice (e le regole) di quando la mail e' arrivata e quelli di adesso.
	Interpretati, Salvati, Nuovi, Persi int
	EsempiNuovi, EsempiPersi            []string
	// SenzaEstrazione: i messaggi letti all'arrivo con «ignora» e senza un codice salvato. Restano fuori dal
	// confronto: il triage puo' non averli estratti (controparte ambigua, posta non di lavoro), e ogni codice che
	// il motore di oggi ci trova sembrerebbe nuovo anche con lo stesso motore.
	SenzaEstrazione int
}

// LettureNome e' quello che il motore di oggi legge nei nomi dei file tecnici (solo dal nome, come l'ingest),
// e il confronto con il codice e la revisione che una persona ha deciso per lo stesso contenuto.
type LettureNome struct {
	// File: i nomi letti, uno per (RFQ, nome com'e' scritto): la lettura dipende solo dal nome.
	File         int
	Letture      []Voce // «forma del codice · rev forma della rev»
	AltreLetture Resto
	// Confrontati: i file con un documento deciso che porta un codice, uno per (RFQ, nome, contenuto): ogni
	// contenuto con la sua decisione, anche quando due contenuti hanno lo stesso nome.
	Confrontati, CodiceUguale, RevUguale int
	Diversi                              []Differenza
	AltriDiversi                         int
	// ConProposta: i file con una proposta di file salvata, uno per (RFQ, nome, contenuto, proposta);
	// PropostaUguale quelli in cui codice e revisione della proposta sono la lettura del nome di oggi. La
	// proposta puo' venire anche dallo STEP o dal cartiglio: una differenza non e' per forza un cambio del
	// motore, ma un cambio del motore si vede qui.
	ConProposta, PropostaUguale int
	ProposteDiverse             []Differenza
	AltreProposteDiverse        int
}

// Differenza e' un file il cui nome il motore legge diverso dalla decisione.
type Differenza struct {
	Nome, Letto, Deciso string
}

// Firme sono i segni dei pezzi decisi commerciali accanto a quelli degli altri pezzi decisi: la forma del
// codice (con le lettere: la X della minuteria) e le parole dei nomi. Suggerimenti per scrivere le famiglie
// del cliente con una persona, mai un tipo.
type Firme struct {
	Commerciali, Altri int
	Forme              []Firma
	AltreForme         Resto
	Parole             []Firma
	AltreParole        Resto
}

// Firma e' una forma o una parola con quanti pezzi commerciali e quanti altri la portano.
type Firma struct {
	Testo              string
	Commerciali, Altri int
	Esempi             []string
}

// Minuteria e' il confronto fra il tipo proposto e quello deciso. Finche' il motore non propone il tipo
// (Proposte falso) si contano solo le decisioni: tutti i commerciali decisi sono «non proposti».
type Minuteria struct {
	Proposte bool
	// Confermate: proposto commerciale e deciso commerciale. Scartate: proposto commerciale, deciso altro (il
	// mancato da guardare uno per uno). NonProposte: deciso commerciale senza proposta. Altri: ne' proposto
	// ne' deciso commerciale.
	Confermate, Scartate, NonProposte, Altri int
	Scarti                                   []RigaMinuteria
	NonViste                                 []RigaMinuteria
	AltriScarti, AltriNonVisti               int
}

// RigaMinuteria e' un pezzo del confronto.
type RigaMinuteria struct {
	Codice, Nome     string
	Deciso, Proposto db.TipoComponente
	Motivo           string
}

// NomeDeciso e NomeProposto sono i due tipi con le parole della Distinta (NomeTipo), per la pagina.
func (r RigaMinuteria) NomeDeciso() string   { return NomeTipo(r.Deciso) }
func (r RigaMinuteria) NomeProposto() string { return NomeTipo(r.Proposto) }

// ---------------------------------------------------------------- le forme di una mail

// FormaOggetto e' la forma dell'oggetto di una mail: senza i prefissi di risposta e d'inoltro, in maiuscolo, con
// le cifre ridotte a 9 e le lettere tenute (FormaCifre: «R: RDO 7120001 del 12/09» → «RDO 9999999 DEL 99/99»).
// Le parole sono il modo in cui il cliente chiama la richiesta («RICHIESTA D'OFFERTA», «ANFRAGE_»), e una
// regola del riferimento si scrive leggendole.
func FormaOggetto(oggetto string) string {
	return classificazione.FormaCifre(classificazione.OggettoPulito(oggetto))
}

// reRiferimento trova un numero accanto a una parola di riferimento («RDO 400012345», «Anfrage_4500000001»,
// «CTE n. 26999999», «ordine nr: 4500000001»). Il confine prima della parola e' scritto a mano perche' \b non
// vede «_» come un separatore; il numero deve avere almeno tre cifre (lo controlla Riferimenti).
var reRiferimento = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])(RDO|RDA|RFQ|RFI|ANFRAGE|CTE|ECR|ECO|ECN|ODA|ODL|PO|ORDINE|OFFERTA|BESTELLUNG|COMMANDE|DEVIS)` +
	`(?:[\s:.#°º/_-]*(?:NUMERO|NUM|NR|NO|N)\.?)?[\s:.#°º/_-]*([\p{L}\p{N}][\p{L}\p{N}._/-]*)`)

// Riferimenti sono i numeri accanto alle parole di riferimento in un testo, come «PAROLA forma» → numeri
// letti: ognuno una volta sola per testo.
func Riferimenti(testo string) map[string][]string {
	out := map[string][]string{}
	visti := map[string]bool{}
	for _, g := range reRiferimento.FindAllStringSubmatch(testo, -1) {
		numero := strings.ToUpper(strings.TrimRight(g[2], "._/-"))
		cifre := 0
		for _, r := range numero {
			if unicode.IsDigit(r) {
				cifre++
			}
		}
		if cifre < 3 {
			continue
		}
		chiave := strings.ToUpper(g[1]) + " " + classificazione.Forma(numero)
		if visti[chiave+"\x00"+numero] {
			continue
		}
		visti[chiave+"\x00"+numero] = true
		out[chiave] = append(out[chiave], numero)
	}
	return out
}

// ---------------------------------------------------------------- le parole dei nomi

// Parole sono le parole di un nome, in maiuscolo, una volta sola: le sequenze di sole lettere di almeno due
// caratteri («VITE TE M8X20 UNI 5739 8.8» → VITE, TE, UNI). Una misura come M8X20 non e' una parola: e' la
// forma che dice che cosa sia, e sta nel nome per intero.
func Parole(nome string) []string {
	var out []string
	visti := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToUpper(nome), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len([]rune(t)) < 2 || strings.IndexFunc(t, unicode.IsDigit) >= 0 || visti[t] {
			continue
		}
		visti[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- i file tecnici

// estensioniTecniche sono i file di cui si guarda il nome: disegni, modelli e archivi che li portano. Le
// immagini della firma e i documenti d'ufficio non dicono niente dei codici del cliente, e riempirebbero le
// forme di «image001».
var estensioniTecniche = map[string]bool{"pdf": true, "stp": true, "step": true, "dxf": true, "dwg": true, "igs": true, "iges": true,
	"x_t": true, "x_b": true, "sldprt": true, "sldasm": true, "slddrw": true, "prt": true, "asm": true, "catpart": true,
	"catproduct": true, "catdrawing": true, "jt": true, "stl": true, "3dxml": true, "tif": true, "tiff": true, "zip": true,
	"7z": true, "rar": true}

// Tecnico dice se un file, dal nome, e' un disegno, un modello o un archivio.
func Tecnico(nome string) bool {
	return estensioniTecniche[strings.ToLower(strings.TrimPrefix(path.Ext(strings.TrimSpace(nome)), "."))]
}

// senzaEstensione e' il nome senza l'ultima estensione.
func senzaEstensione(nome string) string {
	nome = strings.TrimSpace(nome)
	return strings.TrimSuffix(nome, path.Ext(nome))
}

// motore compila le regole di un cliente come l'ingest (LeggiRegole, poi Compila: le regole ✗ restano fuori).
func motore(c Cliente) *classificazione.Motore {
	lette, _ := regole.LeggiRegole(c.Regole)
	return classificazione.Compila(c.Ragione, lette)
}
