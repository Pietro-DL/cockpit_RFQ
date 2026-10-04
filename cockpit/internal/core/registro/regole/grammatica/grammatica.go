// Package grammatica descrive le grammatiche del motore A: i file regole/<cliente>.v1.json del dataset
// privato. Contiene i tipi versionati, la decodifica stretta, la validazione, la verifica dei pattern RE2,
// la forma canonica con l'hash, i limiti dichiarati dall'indice e l'indice che lega i clienti ai file.
//
// È separato dalla struct Regole legacy (core/registro/regole), che resta del motore attuale e di
// cliente.regole: i due formati non si mescolano (parte 1 §8, §9.6). Sta nella sua cartella ma non la
// importa, e il legacy non importa questo pacchetto (R40 a). Da evidenze usa solo il vocabolario dei
// selettori e le diagnostiche (R41 a, G8). I tipi sono dichiarativi: servono alla decodifica, alla
// validazione e all'hash. Non compila regex per il riconoscimento: lo fa motorea, che li abbassa in una sua
// rappresentazione privata.
//
// Nessun dato dei clienti sta qui: le grammatiche vere sono file del dataset privato, e i commenti citano
// decisioni ed esempi inventati (ACME), mai clienti, sigle o codici reali (P-18).
package grammatica

import "github.com/google/uuid"

// VersioneSchema: la versione dei file regole che questo codice sa leggere. Un file con un'altra versione,
// o senza, è uno schema ignoto: diagnosi sì, attivazione no (parte 1 §7.1). Nessun adattatore legacy (R6).
const VersioneSchema = 1

// Grammatica: le regole del motore A per un cliente. Non si legge da cliente.regole e non lo modifica.
// I campi legacy «non coinvolti» (frasi del portale, mittenti di sistema, ...) non stanno qui e restano in
// cliente.regole: un file che li contiene viene rifiutato come chiave sconosciuta (R20 c).
type Grammatica struct {
	VersioneSchema int                     `json:"versione_schema"`
	Cliente        ClienteGrammatica       `json:"cliente"`
	Profilo        Profilo                 `json:"profilo"`
	Famiglie       []FamigliaCodice        `json:"famiglie_codice"`
	RiferimentiRFQ []RegolaRiferimento     `json:"riferimenti_rfq,omitempty"`        // parte 1 §7.1; senza fixture restano riservati (R20 c)
	Qualificatori  []QualificatoreTestuale `json:"qualificatori_testuali,omitempty"` // riservati in A1: diagnosi di capacità non supportata
}

// ClienteGrammatica: il legame esplicito con il cliente, per UUID (v3 §10.5). La ragione sociale serve solo
// da controllo contro quella del DB, mai per cercare il cliente (par.3.6.3 del piano A).
type ClienteGrammatica struct {
	ID             uuid.UUID `json:"id"`
	RagioneSociale string    `json:"ragione_sociale"`
}

// Profilo: completo o parziale, con le parti riservate dichiarate (v3 §5 A1a: «profili parziali
// dichiarati»).
type Profilo struct {
	Stato   string    `json:"stato"` // completo | parziale
	Riserve []Riserva `json:"riserve,omitempty"`
}

// Riserva: una parte non attiva, e perché. Per esempio una domanda aperta (Q1), un caso che resta
// riservato, o una regola del seme che non entra nel file (R20 d). Ogni riserva dà una diagnosi, così ciò
// che il file non attiva si vede (v3 §5 A1a).
type Riserva struct {
	ID     string   `json:"id"`
	Motivo string   `json:"motivo"`
	Regole []string `json:"regole,omitempty"` // gli ID delle forme, revisioni o decorazioni toccate, anche fuori dal file
}

// FamigliaCodice: una grammatica del cliente con ID stabile (parte 1 §5.1, con la correzione del v3 §2: ruoli
// e categorie al posto di «classi»). L'ID non è la posizione nell'array: cambiare l'ordine non cambia né
// l'ID né l'hash. Le «classi/categorie» della risposta R3 sono Ruoli e Categorie (P-01): i ruoli entrano
// nell'intersezione (A1b), le categorie viaggiano come annotazione della lettura (v3 §2).
type FamigliaCodice struct {
	ID          string            `json:"id"`
	Namespace   string            `json:"namespace"` // lo spazio di confronto degli articoli del cliente
	Descrizione string            `json:"descrizione,omitempty"`
	Ruoli       []Ruolo           `json:"ruoli"`               // ⊆ {prodotto, componente}, almeno uno (R17 c)
	Categorie   []Categoria       `json:"categorie,omitempty"` // ⊆ {minuteria}: annotazione della lettura (R7)
	Base        Base              `json:"base"`
	Forme       []FormaCodice     `json:"forme"`
	Etichette   []Etichetta       `json:"etichette,omitempty"`
	Affissi     []Affisso         `json:"affissi,omitempty"`
	Revisioni   []RegolaRevisione `json:"revisioni,omitempty"`
	Decorazioni []Decorazione     `json:"decorazioni,omitempty"`
	Esempi      []EsempioCodice   `json:"esempi"`
}

// Ruolo: una capacità della famiglia nell'intersezione (v3 §2): "prodotto" | "componente".
// È un tipo diverso dalle costanti Ruolo* del motore legacy, che rispecchiano l'enum ruolo_codice del DB:
// «parte» e «finito» qui sono valori ignoti, senza adattatore (R6).
type Ruolo string

// Categoria: un'annotazione trasversale della lettura (v3 §2): "minuteria". La minuteria nasce dalla famiglia
// dichiarata, non da un riconoscitore sul nome (R7).
type Categoria string

// Base: la forma completa del codice, a segmenti nominati e ordinati, senza match vuoti (parte 1 §5.1).
type Base struct {
	Segmenti     []SegmentoBase `json:"segmenti"`
	Maiuscole    string         `json:"maiuscole"`     // esatte | indifferenti_ascii; mai il flag (?i) nel pattern
	Normalizza   string         `json:"normalizza"`    // nessuna | maiuscolo
	ConfinePrima ClasseConfine  `json:"confine_prima"` // da un insieme chiuso (D-05); vuoto vuol dire alnum_ascii
	ConfineDopo  ClasseConfine  `json:"confine_dopo"`  // come sopra
}

// ClasseConfine: che cosa non può stare accanto al codice. Il confine sinistro lo controlla il motore in Go
// sul testo intero; il destro lo aggiunge il compilatore in fondo alla regex, senza che entri nella lettura.
//   - "alnum_ascii": né lettera né cifra ASCII (il valore predefinito);
//   - "parola_ascii": né lettera, né cifra ASCII, né «_»;
//   - "alnum_ascii_o_spazio": né lettera, né cifra ASCII, né spazio U+0020;
//   - "parola_ascii_o_punto_cifra": come parola_ascii, e nemmeno «.» seguito da una cifra (solo a destra: la
//     forma parziale non deve leggere dentro una forma completa).
//
// Le classi servono a evitare le letture annidate della stessa famiglia (R25 f, D-05).
type ClasseConfine string

// SegmentoBase: un pezzo della base, letterale o pattern RE2 limitato. Il pattern non ha gruppi, ancore,
// confini, ripetizioni illimitate, flag né «.» (VerificaPattern).
type SegmentoBase struct {
	Nome        string `json:"nome"`                 // per esempio "T": il nome che le forme parziali citano
	Letterale   string `json:"letterale,omitempty"`  // in alternativa al pattern; si confronta come testo
	Pattern     string `json:"pattern,omitempty"`    // in alternativa al letterale
	Separatore  string `json:"separatore,omitempty"` // il letterale che sta prima del segmento («.»)
	Identitario bool   `json:"identitario"`
}

// FormaCodice: una sequenza di parti, abilitata su alcuni selettori (parte 1 §5.1). Una forma parziale è una
// proiezione dichiarata, mai una forma con i pezzi resi facoltativi.
type FormaCodice struct {
	ID               string         `json:"id"`
	Selettori        []string       `json:"selettori"`                   // forma testuale: «cartiglio.codice»
	Stato            string         `json:"stato"`                       // attiva | riservata
	Completa         bool           `json:"completa"`                    // falso solo per una proiezione dichiarata
	SegmentiMancanti []string       `json:"segmenti_mancanti,omitempty"` // solo segmenti identitari in coda alla base, per nome
	Parti            []Parte        `json:"parti"`                       // l'ordine conta e resta nel canonico
	ConfinePrima     *ClasseConfine `json:"confine_prima,omitempty"`     // sostituisce quello della base
	ConfineDopo      *ClasseConfine `json:"confine_dopo,omitempty"`
}

// Parte: un pezzo di una forma, nell'ordine in cui compare nel testo.
//   - Tipi della parte 1 §5.1: etichetta | affisso | base | revisione | decorazione | separatore.
//   - Tipi chiesti da v3 e attesi (R16):
//     marcatore: una lettera fra la base e la revisione, come la A di D1 e la X di R7, parte del testo e
//     non della base;
//     ripetizione_base: la base scritta una seconda volta (D2); deve concordare con la base, e il confronto
//     si fa in Go, perché RE2 non ha backreference;
//     token: un pezzo conservato senza significato attribuito (Q1, D10). Non è una decorazione.
//   - Min è 0 o 1; base, separatore e ripetizione_base hanno sempre Min 1. Max è sempre 1: una ripetizione
//     non ammessa dal contratto è un errore, non un ciclo nascosto.
type Parte struct {
	Tipo      string   `json:"tipo"`
	Rif       string   `json:"rif,omitempty"`       // l'ID della regola (etichetta, affisso, revisione, decorazione)
	Letterali []string `json:"letterali,omitempty"` // per separatore, marcatore e token: alternative letterali
	Rimando   string   `json:"rimando,omitempty"`   // solo per token: «Q1» (domanda aperta) o «D10» (decisione)
	Min       int      `json:"min"`
	Max       int      `json:"max"`
}

// Etichetta: un testo esterno al codice che lo introduce (per esempio «PN»), a distanza limitata.
type Etichetta struct {
	ID           string   `json:"id"`
	Letterali    []string `json:"letterali"`
	SpaziMax     int      `json:"spazi_max"`
	Selettori    []string `json:"selettori"`
	Obbligatoria bool     `json:"obbligatoria"` // A-C08: senza l'etichetta, nessuna lettura della famiglia
}

// Affisso: una parte attaccata al codice, con un significato (parte 1 §5.3). Fuori dai selettori di
// riconoscimento non si toglie niente; l'attribuzione vale solo dove è dichiarata.
type Affisso struct {
	ID             string               `json:"id"`
	Letterali      []string             `json:"letterali"`              // per esempio «P»
	Posizione      string               `json:"posizione"`              // prefisso | suffisso
	Riconoscimento []string             `json:"riconoscimento"`         // selettori
	Attribuzione   []string             `json:"attribuzione,omitempty"` // ⊆ Riconoscimento; vuota = riconosciuto ma non attribuito (D5)
	Valore         *ValoreQualificatore `json:"valore,omitempty"`
}

// ValoreQualificatore: due assi separati (parte 1 §5.3). L'assenza non vuol dire serie.
//   - Fase: prototipo | campionatura | preserie | serie.
//   - Destinazione: ricambio.
type ValoreQualificatore struct {
	Fase         string `json:"fase,omitempty"`
	Destinazione string `json:"destinazione,omitempty"`
}

// RegolaRevisione: una revisione inline o in campo separato, della stessa entità (parte 1 §5.2), con i
// segmenti nominati e il token conservato che v3 e attesi chiedono (D4, D5, D9, Q1; R16). Segmenti può
// essere vuoto se ci sono TokenSospesi (D-06: per «_30» nessuna lettura numerica è autorizzata, D5).
// Equivalenze: solo dichiarate; nessuna grammatica ne dichiara in A1 (parte 1 §5.2: «00.00 e 00 non sono
// equivalenti per ipotesi»).
type RegolaRevisione struct {
	ID           string              `json:"id"`
	Selettori    []string            `json:"selettori"`
	Stato        string              `json:"stato"`                   // attiva | riservata
	Sorgente     string              `json:"sorgente"`                // inline | campo_separato
	Separatori   []string            `json:"separatori,omitempty"`    // per esempio «_», «/»
	Segmenti     []SegmentoRevisione `json:"segmenti,omitempty"`      // per esempio forte e debole (D4); la stringa si conserva sempre
	TokenSospesi []TokenSospeso      `json:"token_sospesi,omitempty"` // per esempio «xx» (D5): conservato, revisione numerica nulla, da verificare
	Equivalenze  [][2]string         `json:"equivalenze,omitempty"`
}

// SegmentoRevisione: un pezzo nominato della revisione, con il suo pattern limitato e il significato
// dichiarato (forte | debole | nessuno).
type SegmentoRevisione struct {
	Nome        string `json:"nome"`
	Pattern     string `json:"pattern"`
	Significato string `json:"significato"`
}

// TokenSospeso: un token conservato senza attribuirgli un valore di revisione (D5, Q1).
type TokenSospeso struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	Riserva string `json:"riserva,omitempty"` // la domanda o la decisione che lo tiene sospeso
}

// Decorazione: una parte accessoria, fuori dall'identità, solo nei selettori dichiarati (parte 1 §5.2).
//   - Tipi della parte 1: foglio | formato | sigla_interna | copia | annotazione. In A1 sono riservati: una
//     grammatica che li usa riceve capacita.non_supportata e quella decorazione non è attiva (R20 c).
//   - Tipi del v3 e degli attesi (R16): involucro | livello_nome_file | stato_pdm | suffisso_documento |
//     descrizione.
//   - Sottotipo vale solo per involucro: riferimento_pacchetto (per esempio «ACME-030», D2) | tecnico (per
//     esempio «ACME_MOD_3D», D8).
//   - Parti vale solo per suffisso_documento: la sequenza interna (separatori, token, ripetizione_base), un
//     solo livello. Le altre decorazioni hanno Letterali o Pattern.
type Decorazione struct {
	ID        string   `json:"id"`
	Tipo      string   `json:"tipo"`
	Sottotipo string   `json:"sottotipo,omitempty"`
	Letterali []string `json:"letterali,omitempty"`
	Pattern   string   `json:"pattern,omitempty"` // limitato come i segmenti della base
	Parti     []Parte  `json:"parti,omitempty"`
	Selettori []string `json:"selettori"`
	Riserva   string   `json:"riserva,omitempty"`
}

// RegolaRiferimento: come si riconosce un riferimento RFQ del cliente (parte 1 §7.1, al plurale). Un
// riferimento non diventa mai un prodotto (parte 1 §7.4). In A1 è riservato (R20 c).
type RegolaRiferimento struct {
	ID        string          `json:"id"`
	Stato     string          `json:"stato"`
	Segmenti  []SegmentoBase  `json:"segmenti"`
	Selettori []string        `json:"selettori"`
	Esempi    []EsempioCodice `json:"esempi,omitempty"`
}

// QualificatoreTestuale: frasi che qualificano codici (parte 1 §5.3). In A1 c'è solo come riservato.
type QualificatoreTestuale struct {
	ID        string              `json:"id"`
	Stato     string              `json:"stato"`
	Ambito    string              `json:"ambito"` // riga | messaggio_logico | cella | sezione
	Testi     []string            `json:"testi"`
	Selettori []string            `json:"selettori"`
	Valore    ValoreQualificatore `json:"valore"`
}

// EsempioCodice: una fixture dichiarativa della grammatica (parte 1 §5.4). La verifica CompilaVerificato,
// con la stessa Riconosci del motore. RifCaso è un metadato opaco: lo legge solo il banco, per controllare
// che l'esempio e il caso che ripete dicano la stessa cosa; il motore non lo legge mai, e nelle grammatiche
// sintetiche i valori sono inventati (R47 b).
type EsempioCodice struct {
	ID        string        `json:"id"`
	Origine   string        `json:"origine"`            // sintetico | caso_reale
	RifCaso   string        `json:"rif_caso,omitempty"` // metadato opaco: lo legge solo il banco, mai motorea (R47 b)
	Selettore string        `json:"selettore"`
	Testo     string        `json:"testo"`
	Atteso    AttesoEsempio `json:"atteso"`
}

// AttesoEsempio: l'insieme esatto delle letture attese, oppure nessuna lettura.
type AttesoEsempio struct {
	Letture      []LetturaAttesa `json:"letture,omitempty"`
	Nessuna      bool            `json:"nessuna,omitempty"`
	AltreAmmesse bool            `json:"altre_ammesse,omitempty"`
}

// LetturaAttesa: famiglia, forma, base normalizzata, segmenti mancanti, marcatore, affissi, revisione e
// decorazioni attesi. I campi vuoti non si controllano. Per un esempio su un selettore che ha solo regole di
// revisione in campo separato, Forma è vuota e Revisione è il valore atteso (D-07).
type LetturaAttesa struct {
	Famiglia    string   `json:"famiglia,omitempty"`
	Forma       string   `json:"forma,omitempty"`
	Base        string   `json:"base,omitempty"`
	Marcatore   string   `json:"marcatore,omitempty"`
	Revisione   string   `json:"revisione,omitempty"`
	Mancanti    []string `json:"mancanti,omitempty"`
	Affissi     []string `json:"affissi,omitempty"`
	Decorazioni []string `json:"decorazioni,omitempty"`
}

// I codici delle diagnostiche di questo pacchetto stanno in codici_diagnostica.go (R41 b).
