package evidenze

import (
	"time"

	"github.com/google/uuid"
)

// ---- il documento (R49 C: la foglia nasce intera; gli adattatori che la riempiono sono di A1b) ----

// VersioneSchemaDocumento: la versione della struttura di DocumentoEvidenze. Entra nel BundleID.
const VersioneSchemaDocumento = 1

// DocumentoEvidenze: la fotografia versionata di ciò che si è osservato su una fonte (parte 1 §7.2).
// È immutabile, e nessuna regola cliente è applicata ai suoi testi. BundleID è l'impronta del contenuto
// canonico: la calcola chi costruisce il documento (estrazione), non questo pacchetto.
type DocumentoEvidenze struct {
	BundleID           string           `json:"bundle_id"`           // sha256 del canonico del documento con BundleID vuoto e senza Provenienza.Bytes (R51 A)
	VersioneSchema     int              `json:"versione_schema"`     // VersioneSchemaDocumento
	VersioneAdattatore string           `json:"versione_adattatore"` // la versione degli adattatori che l'hanno costruito
	Fonte              Fonte            `json:"fonte"`
	Testi              []TestoOriginale `json:"testi"` // i testi immutabili su cui puntano gli intervalli
	Segmenti           []Segmento       `json:"segmenti,omitempty"`
	Entita             []EntitaLocale   `json:"entita,omitempty"`
	Unita              []UnitaEvidenza  `json:"unita"`
	Legami             []LegameFonte    `json:"legami,omitempty"`
	Qualita            QualitaFonte     `json:"qualita"`
}

// Tipi della fonte (Fonte.Tipo).
const (
	fonteMessaggio = "messaggio"
	fonteAllegato  = "allegato"
	// fonteTesto: la fonte senza origine nel DB (gli esempi delle grammatiche, un testo isolato). OrigineID
	// è uuid.Nil e RiferimentoFatti.Tipo è «nessuno»: un documento così non si salva mai.
	fonteTesto = "testo"
)

// Fonte: un messaggio o un allegato acquisito (parte 1 §3.1), oppure un testo isolato. L'ID contiene il
// tipo: «allegato:<uuid>». Lo stesso sha256 non fonde due fonti: due acquisizioni restano due fonti.
type Fonte struct {
	ID               string           `json:"id"`   // "messaggio:<uuid>" | "allegato:<uuid>"
	Tipo             string           `json:"tipo"` // messaggio | allegato | testo
	OrigineID        uuid.UUID        `json:"origine_id"`
	ContenitoreID    string           `json:"contenitore_id,omitempty"` // la Fonte dello zip o del messaggio contenitore; "" = ignoto
	Provenienza      Provenienza      `json:"provenienza"`
	RiferimentoFatti RiferimentoFatti `json:"riferimento_fatti"`
}

// Provenienza: canale, nome e percorso ricevuti, autore e data, dove ci sono. Ciò che la fonte non può dire
// si elenca in Ignoti, mai si inventa. Nome e percorso sono quelli già trasformati dall'acquisizione (nome
// troncato e ripulito, percorso pulito): l'adattatore lo dichiara.
//
// Bytes è fotografato ma resta fuori dal BundleID: l'ingest riporta la dimensione a quella che dice la
// posta a ogni risincronizzazione, e un'impronta che cambiasse per questo non direbbe niente (R51 A). La
// formula la applica chi costruisce il documento.
type Provenienza struct {
	Canale            string    `json:"canale,omitempty"`
	OrigineDichiarata string    `json:"origine_dichiarata,omitempty"`
	NomeRicevuto      string    `json:"nome_ricevuto,omitempty"`
	PercorsoRicevuto  string    `json:"percorso_ricevuto,omitempty"`
	Autore            string    `json:"autore,omitempty"`
	ContentType       string    `json:"content_type,omitempty"`
	Data              time.Time `json:"data,omitzero"`   // UTC, al millisecondo
	Bytes             *int64    `json:"bytes,omitempty"` // fuori dal BundleID (R51 A)
	Ignoti            []string  `json:"ignoti,omitempty"`
}

// Tipi del riferimento ai fatti (RiferimentoFatti.Tipo).
const (
	riferimentoAnalisiFile = "analisi_file"
	riferimentoMessaggio   = "messaggio"
	riferimentoNessuno     = "nessuno"
)

// RiferimentoFatti: da quali fatti viene il documento.
//   - Per un file: la terna dell'analisi, più ciò che la terna non garantisce, cioè il momento del calcolo e
//     il digest del payload usato (v3 §2, §10.3).
//   - Per un messaggio: il digest dei testi acquisiti (parte 1 §3.1).
//   - Per un testo isolato: «nessuno».
type RiferimentoFatti struct {
	Tipo                 string    `json:"tipo"` // analisi_file | messaggio | nessuno
	Sha256               string    `json:"sha256,omitempty"`
	VersioneAnalizzatore int       `json:"versione_analizzatore,omitempty"`
	HashConfigurazione   string    `json:"hash_configurazione,omitempty"`
	CalcolatoIl          time.Time `json:"calcolato_il,omitzero"`
	DigestPayload        string    `json:"digest_payload,omitempty"`
	SottoversioneSTEP    int       `json:"sottoversione_step,omitempty"`
	SottoversionePDF     int       `json:"sottoversione_pdf,omitempty"`
	DigestTesti          string    `json:"digest_testi,omitempty"` // messaggio: sha256 di oggetto, corpo_testo e corpo_html usati
}

// TestoOriginale: un testo immutabile del documento, su cui si misurano gli intervalli (piano A, par.3.4.4:
// mai su un testo già normalizzato).
type TestoOriginale struct {
	ID      string `json:"id"` // "messaggio.oggetto", "messaggio.corpo_testo", "allegato.nome_file", ...
	Testo   string `json:"testo"`
	Origine string `json:"origine"` // la colonna o il percorso nei fatti; "derivato_da_html" per il testo ricavato dall'HTML
}

// Tipi dei segmenti (parte 1 §3.2). In A1 l'adattatore produce soltanto «corrente», «citazione» e
// «inoltro»: «firma» e «sezione_tecnica» restano nel vocabolario, e la loro assenza si dichiara in
// QualitaFonte, mai si finge. Un inoltro senza confine nel corpo è un solo segmento «inoltro», in contesto
// storia (R48 A).
const (
	SegmentoCorrente       = "corrente"
	SegmentoCitazione      = "citazione"
	SegmentoInoltro        = "inoltro"
	SegmentoFirma          = "firma"
	SegmentoSezioneTecnica = "sezione_tecnica"
)

// Segmento: una porzione di un messaggio, con confini e origine (parte 1 §3.2).
//   - Con l'indizio d'inoltro nell'oggetto e nessun confine nel corpo, la storia non è separabile: tutto il
//     corpo è un segmento «inoltro», letto in contesto storia, e nessun uso automatico lo promuove a
//     richiesta (R48 A). La richiesta viene dal file dei casi o da una scelta dell'operatore.
//   - Origine: mittente_del_messaggio | ignota.
type Segmento struct {
	ID                string        `json:"id"` // "s:corrente", "s:storia"
	Tipo              string        `json:"tipo"`
	MessaggioLogicoID string        `json:"messaggio_logico_id"` // "m0" = oggetto e parte corrente; "m1" = la storia
	PadreID           string        `json:"padre_id,omitempty"`
	Origine           string        `json:"origine"`
	Posizione         Localizzatore `json:"posizione"` // l'intervallo sul corpo_testo come sta nel DB
}

// EntitaLocale: l'entità a cui appartengono codice e attributi (parte 1 §3.2).
//   - Tipi: disegno | nodo_step | riga | sezione | unita_isolata.
//   - «unita_isolata» conserva un'osservazione senza fingere una correlazione con le altre.
type EntitaLocale struct {
	ID              string        `json:"id"` // "e:step:#19", "e:pdf:p1", "e:tab:1:r3"
	Tipo            string        `json:"tipo"`
	SegmentoID      string        `json:"segmento_id,omitempty"`
	ChiaveOriginale string        `json:"chiave_originale"` // "#19", "pagina 1", "tabella 1 riga 3"
	Posizione       Localizzatore `json:"posizione"`
}

// UnitaEvidenza: una singola osservazione di un'entità o di un segmento (parte 1 §3.2). Testo è il testo
// grezzo del campo, mai normalizzato, e può contenere più occorrenze di codice: unità e occorrenza sono due
// livelli distinti.
type UnitaEvidenza struct {
	ID             string         `json:"id"` // "u:step:#19:id", "u:pdf:cartiglio:3", "u:nome", "u:corpo:s:corrente"
	FonteID        string         `json:"fonte_id"`
	SegmentoID     string         `json:"segmento_id,omitempty"`
	EntitaID       string         `json:"entita_id,omitempty"`
	Selettore      Selettore      `json:"selettore"`
	Testo          string         `json:"testo"`
	CampoOriginale CampoOriginale `json:"campo_originale"`
	Posizione      Localizzatore  `json:"posizione"`
	Qualita        QualitaUnita   `json:"qualita"`
}

// CampoOriginale: come il campo è arrivato. L'etichetta letta dal worker o il nome dell'attributo STEP, il
// nome del campo nei fatti, la versione della mappatura.
type CampoOriginale struct {
	Etichetta string `json:"etichetta,omitempty"`
	Parser    string `json:"parser,omitempty"`
	Mappatura string `json:"mappatura,omitempty"`
}

// QualitaUnita: quanto è precisa la localizzazione (esatta | parziale | assente, con il motivo), se il worker
// ha troncato il testo, e con quale metodo è stato letto (nativo | ocr | parser | adattatore).
type QualitaUnita struct {
	Localizzazione string `json:"localizzazione"`
	Motivo         string `json:"motivo,omitempty"`
	Metodo         string `json:"metodo,omitempty"`
	Troncata       bool   `json:"troncata,omitempty"`
}

// LegameFonte: un legame fra parti del documento che viene dai fatti, mai dalla somiglianza fra codici
// (parte 1 §3.4).
//   - Tipi: contiene_segmento | campo_di_entita | cella_di_riga | intestazione_di_cella | padre_figlio_step
//     | occorrenza_step.
//   - Quantita appartiene alla relazione e vale solo per padre_figlio_step: è il numero di occorrenze della
//     coppia nei fatti, senza unità di misura (che i fatti non hanno).
type LegameFonte struct {
	ID       string   `json:"id"`
	Tipo     string   `json:"tipo"`
	Da       string   `json:"da"`
	A        string   `json:"a"`
	Quantita *int     `json:"quantita,omitempty"`
	Evidenze []string `json:"evidenze,omitempty"`
	Altre    int      `json:"altre,omitempty"` // le occorrenze oltre quelle elencate dai fatti
}

// QualitaFonte: che cosa il documento sa e non sa dire (parte 1 §3.4).
//   - Stato: disponibile | parziale | non_disponibile | errore.
//   - Metodo: nativo | ocr | parser | adattatore. Mappatura: verificata | plausibile | sconosciuta.
//   - «non_disponibile» non vuol dire «assente verificato»: un PDF senza testo nativo e senza OCR non dice
//     «nessun codice» (A-C09).
//   - Lo stesso per la mail: «nessuna storia» non vuol dire «verificato che non c'è storia». I riconoscitori
//     dei confini non coprono tutte le lingue, e questo limite si dichiara qui, nella capacità della
//     segmentazione, mai in silenzio (R48 A).
type QualitaFonte struct {
	Stato        string        `json:"stato"`
	Metodo       string        `json:"metodo,omitempty"`
	Mappatura    string        `json:"mappatura,omitempty"`
	Capacita     []Capacita    `json:"capacita,omitempty"`
	Limiti       []string      `json:"limiti,omitempty"`
	Diagnostiche []Diagnostica `json:"diagnostiche,omitempty"`
}

// Capacita: una capacità del documento, con lo stato (disponibile | parziale | non_disponibile) e il motivo.
// Fra le altre: «testo», «cartiglio», «struttura», «grafo_completo», «firma», «storia_annidata»,
// «elenco_pdf», «segmentazione» (parziale con la storia non separabile; il motivo dice anche la copertura
// dei riconoscitori dei confini).
type Capacita struct {
	Nome   string `json:"nome"`
	Stato  string `json:"stato"`
	Motivo string `json:"motivo,omitempty"`
}
