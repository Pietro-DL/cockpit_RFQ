package classificazione

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// La tabella degli score di regola `S1` (Smistamento F4, A5.14.3). Ogni numero che la valutazione di un file
// scrive viene da una riga di qui, con il suo perche': la stessa evidenza ha lo stesso score da chiunque la
// scriva (prima la stessa lettura del nome valeva 40, 50, 80, 85 o 90 secondo lo scrittore e il tipo).
//
// Gli score sono quelli della raccomandazione A della Domanda 4, che l'utente puo' ancora cambiare: stanno
// tutti qui, in una sola tabella versionata. Il massimo di una regola e' 95; 100 vuol dire «l'ha deciso una
// persona» (RegolaOperatore). Uno score ordina, non decide (U7), e non e' una probabilita': finche' la
// calibrazione non l'ha misurato non si mostra come percentuale.
//
// Chi cambia uno score (o aggiunge, toglie, sposta una regola) cambia l'impronta: la prova della tabella
// glielo dice, e con lo score va cambiata la versione TabellaPunteggi, che sta in ogni valutazione e separa
// per la calibrazione i periodi in cui gli score erano diversi.

// TabellaPunteggi e' la versione della tabella degli score di regola.
const TabellaPunteggi = "S1"

// ImprontaPunteggi e' lo sha256 della tabella (regola, dimensione, fonte, score, nell'ordine): la versione
// S1 e' QUESTI numeri. Le parole e i perche' si possono ritoccare senza cambiarla.
const ImprontaPunteggi = "707df1283354f7fbe33e5114aae5a5e7af4ce7c9113d1b3104bdc4cc6544b836"

// Le dimensioni di una lettura. La destinazione la calcola solo il flusso dentro una RFQ (F8): qui ci sono
// i suoi score, perche' la tabella sia una sola.
const (
	DimTipo         = "tipo"
	DimCodice       = "codice"
	DimRev          = "rev"
	DimDestinazione = "destinazione"
)

// RegolaScore e' una riga della tabella: di quale dimensione parla, con che fonte (un valore dell'enum
// fonte_proposta, vuoto per la destinazione), che score, come la si dice all'operatore e perche' vale quanto
// vale.
type RegolaScore struct {
	Dimensione string // tipo | codice | rev | destinazione ("" per la decisione di una persona)
	Fonte      string // valore dell'enum fonte_proposta; "" per la destinazione
	Score      int    // 0–95; 100 solo per RegolaOperatore
	Parole     string // la regola in parole, per la schermata (U-C2)
	ConTesto   bool   // se accanto alle parole va il frammento letto («estensione .stp», «convenzione generica _1»)
	Perche     string // perche' questo score (A5.14.3)
}

// regolaS1 e' una riga con il suo nome: l'ordine delle righe e' la precedenza, che decide a parita' di score.
type regolaS1 struct {
	id string
	RegolaScore
}

// tabellaS1 e' la tabella, in ordine di precedenza dentro ogni dimensione (l'ordine di A5.14.3).
var tabellaS1 = []regolaS1{
	// ---- tipo
	{"ext_3d", RegolaScore{DimTipo, "estensione", 95, "estensione", true,
		"il formato e' il tipo: un file STEP non contiene altro; non 100, che vuol dire «l'ha deciso una persona»"}},
	{"step_letto", RegolaScore{DimTipo, "step", 95, "contenuto dello STEP letto", false,
		"il contenuto conferma il formato, ed e' una fonte indipendente dall'estensione"}},
	{"ext_dwg", RegolaScore{DimTipo, "estensione", 85, "estensione", true,
		"formato di disegno CAD; puo' contenere anche un 3D, di rado"}},
	{"ext_dxf", RegolaScore{DimTipo, "estensione", 80, "estensione", true,
		"2D CAD: nel mestiere quasi sempre lo sviluppo, a volte un disegno quotato"}},
	{"ext_posta", RegolaScore{DimTipo, "estensione", 90, "estensione", true, "il formato e' una mail"}},
	{"nome_so_uscita", RegolaScore{DimTipo, "direzione", 90, "nostra offerta «SO …» in uscita", false,
		"nostra convenzione per i nomi, su una mail che mandiamo noi"}},
	{"pdf_termini_offerta", RegolaScore{DimTipo, "cartiglio", 70, "termini d'offerta nel testo", false,
		"i termini d'offerta stanno anche nelle condizioni allegate a un disegno"}},
	{"pdf_termini_offerta_entrata", RegolaScore{DimTipo, "cartiglio", 50, "termini d'offerta nel testo, in entrata", false,
		"un'offerta che arriva non e' la nostra: e' un documento commerciale di chi la manda"}},
	{"pdf_termini_cartiglio", RegolaScore{DimTipo, "cartiglio", 75, "termini da cartiglio nel testo", false,
		"«SCALA» e «TOLLERANZE» dicono un disegno ma stanno anche nei capitolati; il cartiglio non si legge"}},
	{"pdf_termini_capitolato", RegolaScore{DimTipo, "cartiglio", 65, "termini da capitolato nel testo", false,
		"parole di requisiti, non di un pezzo: indizi del testo"}},
	{"pdf_termini_distinta", RegolaScore{DimTipo, "cartiglio", 65, "termini da distinta nel testo", false,
		"due indizi del testo, nessuna tabella letta"}},
	{"ext_foglio", RegolaScore{DimTipo, "estensione", 40, "estensione", true,
		"spesso offerte e listini, ma anche distinte del cliente"}},
	{"ext_altro", RegolaScore{DimTipo, "estensione", 20, "estensione", true, "il formato non dice il tipo"}},
	{"rumore_hash_dominio", RegolaScore{DimTipo, "rumore", 90, "stesso contenuto gia' scartato per questo mittente", false,
		"l'ha detto una persona sullo stesso contenuto, per lo stesso mittente; mai per un file tecnico (P14)"}},
	{"rumore_immagine_ricorrente", RegolaScore{DimTipo, "rumore", 70, "immagine ricorrente dello stesso mittente", false,
		"firme e loghi tornano in ogni mail"}},
	{"rumore_immagine", RegolaScore{DimTipo, "rumore", 60, "immagine piccola o da telefono", false,
		"spesso un logo, a volte la foto del pezzo"}},
	{"risposta_fornitore", RegolaScore{DimTipo, "direzione", 70, "gesto «e' la risposta del fornitore»", false,
		"la mail e' un'offerta, il singolo PDF puo' essere il disegno rimandato"}},
	{"ext_pdf", RegolaScore{DimTipo, "estensione", 0, "PDF non ancora letto", false,
		"un PDF e' un contenitore: prima di leggerlo il tipo non si sa (checkpoint 3R §5)"}},
	{"ext_archivio", RegolaScore{DimTipo, "estensione", 0, "archivio", false,
		"un archivio contiene file: le voci si propongono una per una"}},
	{"pdf_nessun_termine", RegolaScore{DimTipo, "cartiglio", 0, "PDF letto, nessun termine riconosciuto", false,
		"il testo non dice che cosa sia"}},
	{"pdf_illeggibile", RegolaScore{DimTipo, "cartiglio", 0, "PDF letto, non si apre", false,
		"un PDF che non si apre non dice niente del suo tipo"}},

	// ---- codice
	{"pdf_testo_famiglia", RegolaScore{DimCodice, "cartiglio", 85, "codice di famiglia nel testo del PDF, dove sta il cartiglio", false,
		"scritto dal cliente dentro il file, in basso a destra della prima pagina (testo del PDF, F9)"}},
	{"step_radice_famiglia", RegolaScore{DimCodice, "regola_cliente", PuntiFamiglia, "radice dello STEP, codice di famiglia del cliente", false,
		"lo stesso motore delle mail e dei nodi: «e' un codice di questo cliente», scritto dentro il file"}},
	{"nome_codice_famiglia", RegolaScore{DimCodice, "nome_file", 70, "nome del file, codice di famiglia del cliente", false,
		"il nome e' il supporto piu' manipolato (rinomine, «_1», «A», «_PRT»): vale meno dello stesso codice scritto dentro il file"}},
	{"nome_codice_generico", RegolaScore{DimCodice, "nome_file", 45, "nome del file", false,
		"la forma di un codice, un token solo: piu' di un numero pescato in un corpo, meno di una famiglia"}},
	{"pdf_metadati", RegolaScore{DimCodice, "cartiglio", 40, "titolo o soggetto del PDF", false,
		"spesso e' il nome del file"}},
	{"step_radice_generico", RegolaScore{DimCodice, "step", PuntiGenerico, "radice dello STEP", false,
		"il PRODUCT dei CAD e' spesso il nome del file"}},
	{"step_primo_product", RegolaScore{DimCodice, "step", PuntiGenerico, "primo PRODUCT dello STEP", false,
		"il primo PRODUCT dei primi 100 KB non e' detto che sia la radice, ed e' spesso il nome del file"}},
	{"nome_contiene_codice", RegolaScore{DimCodice, "nome_file", 25, "codice citato nel nome del file", false,
		"«Offerta 7120012 per staffe»: il codice di cui il file parla, non il suo"}},

	// ---- rev
	{"rev_nas", RegolaScore{DimRev, "nome_file", 80, "nome nella forma _REV_ del NAS", true,
		"l'abbiamo scritta noi, da una decisione"}},
	{"rev_step_nodo", RegolaScore{DimRev, "step", 70, "rev del nodo radice dello STEP", false,
		"attributo del file; negli STEP veri manca quasi sempre"}},
	{"rev_famiglia_cliente", RegolaScore{DimRev, "regola_cliente", 65, "rev separata dalla famiglia del cliente", false,
		"convenzione dichiarata dal cliente"}},
	{"rev_esplicita_nome", RegolaScore{DimRev, "nome_file", 55, "rev esplicita nel nome", true,
		"la parola «rev» c'e'"}},
	{"rev_step_product", RegolaScore{DimRev, "step", 45, "rev dal PRODUCT dello STEP", false,
		"stesso testo del nome, quasi sempre"}},
	{"rev_suffisso_nome", RegolaScore{DimRev, "nome_file", 40, "convenzione generica", true,
		"«_1», «-B»: puo' essere il foglio, la variante, la copia"}},
	{"rev_trattenuta_interno", RegolaScore{DimRev, "nome_file", 0, "file caricato a mano: la rev letta non e' del cliente", false,
		"un file caricato a mano non propone una revisione del cliente (B8.7)"}},

	// ---- destinazione (la calcola il flusso ancorato al prodotto, F8; la preselezione non dipende dallo score)
	{"dest_nodo_diretto_contenuto", RegolaScore{DimDestinazione, "", 85, "figlio diretto di uno STEP autorizzato, dal contenuto", false,
		"il codice scritto nel file e' un figlio diretto nell'autorita' di un file autorizzato"}},
	{"dest_componente_contenuto", RegolaScore{DimDestinazione, "", 80, "componente deciso, dal contenuto", false,
		"il codice scritto nel file e' un componente deciso raggiungibile"}},
	{"dest_radice_uguale", RegolaScore{DimDestinazione, "", 80, "radice dello STEP uguale al componente", false,
		"lo STEP compatibile col prodotto: candidato da autorizzare"}},
	{"dest_nodo_diretto_nome", RegolaScore{DimDestinazione, "", 75, "figlio diretto di uno STEP autorizzato, dal nome", false,
		"come dal contenuto, ma il codice viene solo dal nome"}},
	{"dest_3d_sottoassieme", RegolaScore{DimDestinazione, "", 70, "3D di un sottoassieme dell'indice", false,
		"la radice dello STEP e' una voce dell'indice dei codici"}},
	{"dest_generale_contenuto", RegolaScore{DimDestinazione, "", 70, "documento generale, dal contenuto", false,
		"un tipo non tecnico letto nel contenuto"}},
	{"dest_componente_nome", RegolaScore{DimDestinazione, "", 65, "componente deciso, dal nome", false,
		"il codice viene solo dal nome"}},
	{"dest_identificativo", RegolaScore{DimDestinazione, "", 60, "codice della richiesta senza componente", false,
		"un identificativo confermato ancora senza componente: «Crea il prodotto»"}},
	{"dest_assegnazione_precedente", RegolaScore{DimDestinazione, "", 60, "assegnazione precedente", false,
		"un gesto di prima senza autore registrato"}},
	{"dest_nodo_non_autorizzato", RegolaScore{DimDestinazione, "", 60, "figlio di uno STEP non autorizzato", false,
		"guida, bloccato finche' lo STEP non e' autorizzato"}},
	{"dest_radice_vicina", RegolaScore{DimDestinazione, "", 45, "radice dello STEP vicina", false,
		"«variante o revisione?» lo decide chi guarda: bloccato"}},
	{"dest_nodo_profondo", RegolaScore{DimDestinazione, "", 40, "nodo profondo della guida", false,
		"fuori dall'autorita' di ogni file: bloccato"}},
	{"dest_generale_estensione", RegolaScore{DimDestinazione, "", 30, "documento generale, dall'estensione", false,
		"un tipo non tecnico detto dalla sola estensione"}},
	{"dest_quasi_uguale", RegolaScore{DimDestinazione, "", 25, "codice quasi uguale", false,
		"quasi uguale non e' uguale (P4): bloccato"}},
	{"dest_famiglia_cliente", RegolaScore{DimDestinazione, "", 20, "famiglia del prodotto", false,
		"dice «e' di questo cliente», non «e' questo pezzo»"}},

	// ---- una decisione
	{RegolaOperatore, RegolaScore{"", "operatore", 100, "deciso da una persona", false,
		"una decisione non ha score: il 100 in colonna resta per chi lo legge (U-C4)"}},
}

// Punteggi e' la tabella per nome della regola.
var Punteggi = func() map[string]RegolaScore {
	m := make(map[string]RegolaScore, len(tabellaS1))
	for _, r := range tabellaS1 {
		m[r.id] = r.RegolaScore
	}
	return m
}()

// ordineS1 e' la precedenza di ogni regola: la posizione nella tabella.
var ordineS1 = func() map[string]int {
	m := make(map[string]int, len(tabellaS1))
	for i, r := range tabellaS1 {
		m[r.id] = i
	}
	return m
}()

// precedenza di una regola: piu' bassa vince a parita' di score. Una regola che la tabella non conosce viene
// dopo tutte.
func precedenza(regola string) int {
	if i, ok := ordineS1[regola]; ok {
		return i
	}
	return len(tabellaS1)
}

// improntaTabella e' lo sha256 della tabella: una riga «regola|dimensione|fonte|score» per regola,
// nell'ordine della tabella. Le parole e i perche' non ci entrano.
func improntaTabella() string {
	var b strings.Builder
	for _, r := range tabellaS1 {
		fmt.Fprintf(&b, "%s|%s|%s|%d\n", r.id, r.Dimensione, r.Fonte, r.Score)
	}
	s := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(s[:])
}
