package fotorfq

// fotografia.go: il contenitore della fotografia e i record delle decisioni attuali (piano A, A1c, 6.4.1; contratto
// di A1c, parte B, §3, che prevale sul 6.4.1). I tipi sono quelli del contratto, campo per campo: un puntatore nil
// vuol dire «assente nel DB», i tempi sono UTC al millisecondo, i tag JSON snake_case, come nei record di A1b.
//
// I record di A1b (Messaggio, Allegato, Terna, Fatti) non cambiano e VersioneSchema resta 1 (T-01): nessuna
// decisione dentro un record di messaggio o di allegato (I-2). Il gesto 1 sta in un record a parte,
// AggancioMessaggio (T-B0-01). Le righe delle tabelle legacy si chiamano RigaComponenteProposta e
// RigaRelazioneProposta (M2, T-B0-02): i nomi NodoProposto e ArcoProposto sono dei risultati del motore.

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Le origini di una fotografia.
const (
	OrigineDSN    = "dsn"     // letta dal caricatore, in una transazione REPEATABLE READ READ ONLY
	OrigineExport = "exports" // letta dagli export, presi in momenti diversi
)

// Gli stati di una sezione (StatoSezione.Stato). «filtrata» e «parziale» non sono «assente».
const (
	StatoSezioneCompleta = "completa"
	StatoSezioneFiltrata = "filtrata"
	StatoSezioneParziale = "parziale"
	StatoSezioneAssente  = "assente"
)

// Le chiavi di Fotografia.Sezioni (contratto §3.1): una per elenco, per dire «assente» negli export invece di
// «vuoto».
const (
	SezioneMessaggi                = "messaggi"
	SezioneAgganci                 = "agganci"
	SezioneAllegati                = "allegati"
	SezioneFatti                   = "fatti"
	SezioneProposteDocumento       = "proposte_documento"
	SezioneDocumenti               = "documenti"
	SezioneProvenienze             = "provenienze"
	SezioneIdentificativi          = "identificativi"
	SezioneComponenti              = "componenti"
	SezioneRelazioni               = "relazioni"
	SezioneRigheComponenteProposta = "righe_componente_proposta"
	SezioneRigheRelazioneProposta  = "righe_relazione_proposta"
	SezioneRimozioniAperte         = "rimozioni_aperte"
	SezioneStepProdotto            = "step_prodotto"
	SezioneFascicolo               = "fascicolo"
	SezioneDeroghe                 = "deroghe"
	SezioneTriage                  = "triage"
	SezioneCandidatiCodice         = "candidati_codice"
	SezioneLavoroPendente          = "lavoro_pendente"
	SezioneFabbisogni              = "fabbisogni"
	SezioneVersioneBOM             = "versione_bom"
)

// ChiaviSezioni: tutte le chiavi di Fotografia.Sezioni, nell'ordine del contratto.
var ChiaviSezioni = []string{
	SezioneMessaggi, SezioneAgganci, SezioneAllegati, SezioneFatti, SezioneProposteDocumento, SezioneDocumenti,
	SezioneProvenienze, SezioneIdentificativi, SezioneComponenti, SezioneRelazioni, SezioneRigheComponenteProposta,
	SezioneRigheRelazioneProposta, SezioneRimozioniAperte, SezioneStepProdotto, SezioneFascicolo, SezioneDeroghe,
	SezioneTriage, SezioneCandidatiCodice, SezioneLavoroPendente, SezioneFabbisogni, SezioneVersioneBOM,
}

// Fotografia: tutto ciò che il caricatore (o il lettore degli export) ha letto, con il suo stato.
//
// È lo stato corrente che il Cockpit vede, non la storia della RFQ (I-7): con il caricatore è letto in una
// transazione sola (Coerente vero); con gli export è com'erano i file quando sono stati presi (Coerente falso).
// Sorgente e PresaIl restano fuori dall'impronta (ImprontaFotografia).
type Fotografia struct {
	VersioneSchema int    `json:"versione_schema"` // VersioneSchema (1)
	Origine        string `json:"origine"`         // dsn | exports
	Sorgente       string `json:"sorgente,omitempty"`
	// SchemaDB: max(schema_versione.versione), con migrazioni.Applicate sulla transazione; dichiarato dal
	// manifest negli export.
	SchemaDB int       `json:"schema_db"`
	PresaIl  time.Time `json:"presa_il"` // SELECT now() nella transazione; zero negli export
	Coerente bool      `json:"coerente"` // vero con la transazione REPEATABLE READ READ ONLY
	// SolaLettura, Isolamento: come li dicono SHOW transaction_read_only e SHOW transaction_isolation.
	SolaLettura  string                  `json:"sola_lettura,omitempty"`
	Isolamento   string                  `json:"isolamento,omitempty"`
	Analizzatore *Terna                  `json:"analizzatore,omitempty"` // analizzatore_corrente; nil = nessuna analisi corrente
	Sezioni      map[string]StatoSezione `json:"sezioni,omitempty"`
	Clienti      []Cliente               `json:"clienti,omitempty"` // mai cliente.regole
	Thread       []Thread                `json:"thread,omitempty"`
	// FuoriRFQ: i messaggi senza RFQ che il file dei casi elenca (R34), con allegati e fatti.
	FuoriRFQ     []MessaggioFuoriRFQ    `json:"fuori_rfq,omitempty"`
	Utenti       []Utente               `json:"utenti,omitempty"` // mai password_hash
	Diagnostiche []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
}

// StatoSezione: completa | filtrata | parziale | assente, con il motivo.
type StatoSezione struct {
	Stato  string `json:"stato"`
	Motivo string `json:"motivo,omitempty"`
}

// Cliente: cliente.cliente_id e ragione_sociale. La ragione sociale è un controllo contro il file delle regole,
// mai una chiave; cliente.regole non si legge.
type Cliente struct {
	ID             uuid.UUID `json:"id"`
	RagioneSociale string    `json:"ragione_sociale"`
}

// Utente: utente.utente_id e sigla. Mai password_hash.
type Utente struct {
	ID    uuid.UUID `json:"id"`
	Sigla string    `json:"sigla"`
}

// MessaggioFuoriRFQ: un messaggio senza RFQ dei casi del banco (R34), con il suo gesto 1, gli allegati e i fatti
// dei loro contenuti alla terna corrente.
type MessaggioFuoriRFQ struct {
	Messaggio Messaggio         `json:"messaggio"`
	Aggancio  AggancioMessaggio `json:"aggancio"`
	Allegati  []Allegato        `json:"allegati,omitempty"`
	Fatti     map[string]Fatti  `json:"fatti,omitempty"` // per sha256
}

// Thread: una RFQ con i suoi messaggi, allegati, fatti e decisioni attuali (contratto §3.2).
type Thread struct {
	ID          uuid.UUID  `json:"id"`         // thread_offerta.thread_id
	ClienteID   uuid.UUID  `json:"cliente_id"` // thread_offerta.cliente_id
	Stato       string     `json:"stato"`
	UnitoIn     *uuid.UUID `json:"unito_in,omitempty"`
	Oggetto     *string    `json:"oggetto,omitempty"`
	Riferimento *string    `json:"riferimento,omitempty"` // riferimento_cliente
	// CreatoDa: per una RFQ nuova, chi l'ha creata (dato accessorio del gesto 1).
	CreatoDa *uuid.UUID          `json:"creato_da,omitempty"`
	CreatoIl time.Time           `json:"creato_il"`
	Messaggi []Messaggio         `json:"messaggi,omitempty"` // A1b, invariato
	Agganci  []AggancioMessaggio `json:"agganci,omitempty"`  // il gesto 1, uno per messaggio
	Allegati []Allegato          `json:"allegati,omitempty"` // A1b
	// Fatti: per sha256, alla terna di Fotografia.Analizzatore; i contenuti degli allegati del thread e dei
	// documenti del thread (T-04).
	Fatti                   map[string]Fatti         `json:"fatti,omitempty"`
	Proposte                []PropostaAttuale        `json:"proposte,omitempty"`
	Documenti               []DocumentoConfermato    `json:"documenti,omitempty"`
	Identificativi          []Identificativo         `json:"identificativi,omitempty"`
	Componenti              []Componente             `json:"componenti,omitempty"`
	Relazioni               []Relazione              `json:"relazioni,omitempty"`
	RigheComponenteProposta []RigaComponenteProposta `json:"righe_componente_proposta,omitempty"`
	RigheRelazioneProposta  []RigaRelazioneProposta  `json:"righe_relazione_proposta,omitempty"`
	RimozioniAperte         []RimozioneAperta        `json:"rimozioni_aperte,omitempty"` // la gerarchia (R80, T-B0-24)
	StepProdotto            []RigaStepProdotto       `json:"step_prodotto,omitempty"`    // v_step_prodotto (R65)
	Fascicolo               []RigaFascicolo          `json:"fascicolo,omitempty"`        // v_fascicolo (R82)
	// Fabbisogni: i fabbisogni effettivi per il cliente del thread (R82, T-B0-36).
	Fabbisogni []RigaFabbisogno   `json:"fabbisogni,omitempty"`
	Deroghe    []DerogaFabbisogno `json:"deroghe,omitempty"` // anche per i componenti senza riga nella vista
	Triage     []Triage           `json:"triage,omitempty"`  // solo fonte «deterministico»
	// CandidatiCodice: senza l'origine «agente».
	CandidatiCodice []CandidatoCodice `json:"candidati_codice,omitempty"`
	InAttesa        []uuid.UUID       `json:"in_attesa,omitempty"` // allegati con lavoro pendente
	// VersioneBOM: l'ultima versione, bozza o congelata (R89, T-B0-28); UltimaCongelata: l'ultima congelata, il
	// congelamento legacy (R96 c). nil = nessuna.
	VersioneBOM     *VersioneBOM `json:"versione_bom,omitempty"`
	UltimaCongelata *VersioneBOM `json:"ultima_congelata,omitempty"`
}

// AggancioMessaggio: il gesto 1 di un messaggio (I-2: fuori dal record del messaggio). AgganciatoDa nil = aggancio
// automatico. messaggio_aggancio_log non si legge (T-06).
type AggancioMessaggio struct {
	MessaggioID  uuid.UUID  `json:"messaggio_id"`
	Aggancio     string     `json:"aggancio"`
	AgganciatoDa *uuid.UUID `json:"agganciato_da,omitempty"`
	AgganciatoIl *time.Time `json:"agganciato_il,omitempty"`
}

// Identificativo: un codice della richiesta (gesto 2). La tabella non ha un UUID: la chiave è (thread, codice).
// ConfermatoDa non nil = target confermato.
type Identificativo struct {
	Codice       string     `json:"codice"`
	Origine      string     `json:"origine"`
	Confidenza   *int16     `json:"confidenza,omitempty"`
	ConfermatoDa *uuid.UUID `json:"confermato_da,omitempty"`
	CreatoIl     time.Time  `json:"creato_il"`
}

// Componente: un pezzo della working. Il codice è una decisione di una persona, normalizzata dal legacy: mai il
// grezzo. StepStrutturaleID è il gesto 3.
type Componente struct {
	ID                uuid.UUID  `json:"id"`
	Codice            string     `json:"codice"`
	Rev               *string    `json:"rev,omitempty"`
	Descrizione       *string    `json:"descrizione,omitempty"`
	Tipo              string     `json:"tipo"`
	Origine           string     `json:"origine"`
	ConfermatoDa      uuid.UUID  `json:"confermato_da"`
	CreatoIl          time.Time  `json:"creato_il"`
	ArchiviatoIl      *time.Time `json:"archiviato_il,omitempty"`
	StepStrutturaleID *uuid.UUID `json:"step_strutturale_id,omitempty"`
}

// Relazione: un arco confermato della working, anche fra componenti archiviati.
type Relazione struct {
	PadreID      uuid.UUID `json:"padre_id"`
	FiglioID     uuid.UUID `json:"figlio_id"`
	Qta          int       `json:"qta"`
	Posizione    *string   `json:"posizione,omitempty"`
	Origine      string    `json:"origine"`
	ConfermatoDa uuid.UUID `json:"confermato_da"`
	CreatoIl     time.Time `json:"creato_il"`
}

// RigaComponenteProposta: una riga di componente_proposta, tabella legacy letta e mai scritta (M2). Il motore A
// legge IDGrezzo e NomeGrezzo; Codice e Rev del motore legacy si mostrano e non si leggono, salvo su una riga
// aperta con OrigineCodice = operatore, dove sono il codice manuale dell'operatore (T-B0-33, LD-13). Di evidenza
// entrano solo strutturale (Marcatura, il gesto 3) e albero (Albero, R61) (T-03).
type RigaComponenteProposta struct {
	ID            uuid.UUID             `json:"id"`
	AllegatoID    uuid.UUID             `json:"allegato_id"`
	NomeFile      string                `json:"nome_file"`
	Sha256        string                `json:"sha256"`
	Chiave        string                `json:"chiave"`
	IDGrezzo      string                `json:"id_grezzo"`
	NomeGrezzo    string                `json:"nome_grezzo"`
	Descrizione   *string               `json:"descrizione,omitempty"`
	Codice        *string               `json:"codice,omitempty"`
	Rev           *string               `json:"rev,omitempty"`
	Famiglia      string                `json:"famiglia"`
	OrigineCodice *string               `json:"origine_codice,omitempty"`
	TipoProposto  *string               `json:"tipo_proposto,omitempty"`
	Fonte         string                `json:"fonte"`
	Stato         string                `json:"stato"`
	ComponenteID  *uuid.UUID            `json:"componente_id,omitempty"`
	DecisoDa      *uuid.UUID            `json:"deciso_da,omitempty"`
	DecisoIl      *time.Time            `json:"deciso_il,omitempty"`
	Nota          *string               `json:"nota,omitempty"`
	Marcatura     *MarcaturaStrutturale `json:"marcatura,omitempty"`
	Albero        *SegnoAlbero          `json:"albero,omitempty"`
}

// MarcaturaStrutturale: evidenza.strutturale di una riga, il gesto 3 (la forma di fascicolo.Marcatura).
// DichiaratoIl resta il testo del JSON, com'è (T-03). Sospesa: c'è la chiave «sospesa».
type MarcaturaStrutturale struct {
	Versione     int        `json:"versione"`
	Ruolo        string     `json:"ruolo"` // radice | raggruppamento | delega
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	DocumentoID  *uuid.UUID `json:"documento_id,omitempty"`
	DichiaratoDa *uuid.UUID `json:"dichiarato_da,omitempty"`
	DichiaratoIl string     `json:"dichiarato_il,omitempty"`
	PresaDAtto   string     `json:"presa_d_atto,omitempty"`
	Sospesa      bool       `json:"sospesa,omitempty"`
}

// SegnoAlbero: evidenza.albero, il segno di «Conferma l'albero» (la forma di fascicolo.SegnoAlbero, senza
// commerciale). Padre e Figlio stanno sulle righe di arco; Nodo sulla riga di un nodo tolto.
type SegnoAlbero struct {
	Da     uuid.UUID  `json:"da"`
	Il     string     `json:"il"`
	Firma  string     `json:"firma"`
	Padre  *uuid.UUID `json:"padre,omitempty"`
	Figlio *uuid.UUID `json:"figlio,omitempty"`
	Nodo   string     `json:"nodo,omitempty"`
}

// RigaRelazioneProposta: una riga di relazione_proposta, tabella legacy letta e mai scritta (M2).
type RigaRelazioneProposta struct {
	AllegatoID   uuid.UUID    `json:"allegato_id"`
	NomeFile     string       `json:"nome_file"`
	PadreChiave  string       `json:"padre_chiave"`
	FiglioChiave string       `json:"figlio_chiave"`
	Qta          int          `json:"qta"`
	Stato        string       `json:"stato"`
	Nota         *string      `json:"nota,omitempty"`
	DecisoDa     *uuid.UUID   `json:"deciso_da,omitempty"`
	DecisoIl     *time.Time   `json:"deciso_il,omitempty"`
	Albero       *SegnoAlbero `json:"albero,omitempty"`
}

// RimozioneAperta: una rimozione_proposta aperta, che tiene aperta la gerarchia (R80, T-B0-24).
type RimozioneAperta struct {
	StepDocumentoID uuid.UUID `json:"step_documento_id"`
	PadreID         uuid.UUID `json:"padre_id"`
	FiglioID        uuid.UUID `json:"figlio_id"`
	QtaWorking      int       `json:"qta_working"`
}

// DocumentoConfermato: un documento con i file da cui è nato. È la decisione: ComponenteID qui è il componente
// scelto da una persona (nil = documento del thread). Estensione è il formato dichiarato (T-B0-30); il contenuto lo
// dicono i fatti (R83). Corrente se SostituitoDa è nil. StatoNas si mostra a parte e non toglie la presenza.
type DocumentoConfermato struct {
	ID           uuid.UUID   `json:"id"`
	ThreadID     uuid.UUID   `json:"thread_id"`
	ComponenteID *uuid.UUID  `json:"componente_id,omitempty"`
	Tipo         string      `json:"tipo"`
	Codice       *string     `json:"codice,omitempty"`
	Rev          *string     `json:"rev,omitempty"`
	NomeFile     string      `json:"nome_file"`
	Estensione   string      `json:"estensione"`
	Sha256       string      `json:"sha256"`
	StatoNas     string      `json:"stato_nas"`
	ConfermatoDa uuid.UUID   `json:"confermato_da"`
	ConfermatoIl time.Time   `json:"confermato_il"`
	SostituitoDa *uuid.UUID  `json:"sostituito_da,omitempty"`
	Allegati     []uuid.UUID `json:"allegati,omitempty"` // documento_provenienza.allegato_id, in ordine
}

// PropostaAttuale: una riga di documento_proposta, in qualunque stato. È il «vecchio» del confronto e non entra mai
// nel motore. ComponenteID è quello della proposta: l'associazione manuale («assegna»). Dettagli è nil negli
// export (dettagli.destinazione = la proposta F8).
type PropostaAttuale struct {
	ID           uuid.UUID       `json:"id"`
	AllegatoID   uuid.UUID       `json:"allegato_id"`
	ThreadID     *uuid.UUID      `json:"thread_id,omitempty"`
	Tipo         string          `json:"tipo"`
	Codice       *string         `json:"codice,omitempty"`
	Rev          *string         `json:"rev,omitempty"`
	ComponenteID *uuid.UUID      `json:"componente_id,omitempty"`
	Fonte        string          `json:"fonte"`
	Stato        string          `json:"stato"`
	DecisoDa     *uuid.UUID      `json:"deciso_da,omitempty"`
	DecisoIl     *time.Time      `json:"deciso_il,omitempty"`
	Dettagli     json.RawMessage `json:"dettagli,omitempty"`
}

// RigaStepProdotto: una riga di v_step_prodotto (R65), com'è nella vista.
type RigaStepProdotto struct {
	ComponenteID       uuid.UUID  `json:"componente_id"`
	Codice             string     `json:"codice"`
	StepStrutturaleID  *uuid.UUID `json:"step_strutturale_id,omitempty"`
	NStepCorrenti      int32      `json:"n_step_correnti"`
	AnalisiCompleta    bool       `json:"analisi_completa"`
	MotivoParziale     *string    `json:"motivo_parziale,omitempty"`
	DerogaStrutturaID  *uuid.UUID `json:"deroga_struttura_id,omitempty"`
	Altro3DDocumentoID *uuid.UUID `json:"altro_3d_documento_id,omitempty"`
	PropostaAperta     *uuid.UUID `json:"proposta_aperta,omitempty"`
	AttesoDaPortale    *uuid.UUID `json:"atteso_da_portale,omitempty"`
	Esito              string     `json:"esito"`
}

// RigaFascicolo: una riga di v_fascicolo (R62 d = C), com'è nella vista; path_relativo non si porta (T-05).
type RigaFascicolo struct {
	ComponenteID    uuid.UUID  `json:"componente_id"`
	Codice          string     `json:"codice"`
	Rev             *string    `json:"rev,omitempty"`
	TipoComponente  string     `json:"tipo_componente"`
	TipoDocumento   string     `json:"tipo_documento"`
	Bloccante       bool       `json:"bloccante"`
	DocumentoID     *uuid.UUID `json:"documento_id,omitempty"`
	StatoNas        *string    `json:"stato_nas,omitempty"`
	PropostaAperta  *uuid.UUID `json:"proposta_aperta,omitempty"`
	NProposteAperte int64      `json:"n_proposte_aperte"`
	AttesoDaPortale *uuid.UUID `json:"atteso_da_portale,omitempty"`
	DerogaID        *uuid.UUID `json:"deroga_id,omitempty"`
	Esito           string     `json:"esito"`
	FonteAttesa     *string    `json:"fonte_attesa,omitempty"`
	DocumentoRev    *string    `json:"documento_rev,omitempty"`
	RevDiversa      bool       `json:"rev_diversa"`
	AnomaliaID      *int64     `json:"anomalia_id,omitempty"`
}

// DerogaFabbisogno: una riga di deroga_fabbisogno.
type DerogaFabbisogno struct {
	ID           uuid.UUID `json:"id"`
	ComponenteID uuid.UUID `json:"componente_id"`
	Tipo         string    `json:"tipo"`
	Motivo       string    `json:"motivo"`
	UtenteID     uuid.UUID `json:"utente_id"`
	CreataIl     time.Time `json:"creata_il"`
}

// VersioneBOM: una riga di bom_versione (R89, T-B0-28), con chi e quando del congelamento legacy. Non vale come
// FascicoloCongelato (R96 b B).
type VersioneBOM struct {
	ID          uuid.UUID  `json:"id"`
	Numero      int32      `json:"numero"`
	Stato       string     `json:"stato"` // bozza | congelata
	Contesto    string     `json:"contesto"`
	CongelataDa *uuid.UUID `json:"congelata_da,omitempty"`
	CongelataIl *time.Time `json:"congelata_il,omitempty"`
}

// RigaFabbisogno: un fabbisogno effettivo per il cliente del thread, risolto come lo risolve v_fascicolo (R82,
// T-B0-36). Proprio: la riga è del cliente, non del default.
type RigaFabbisogno struct {
	TipoComponente string  `json:"tipo_componente"`
	TipoDocumento  string  `json:"tipo_documento"`
	Bloccante      bool    `json:"bloccante"`
	FonteAttesa    *string `json:"fonte_attesa,omitempty"`
	Proprio        bool    `json:"proprio"`
}

// Triage: la proposta di triage DETERMINISTICA di un messaggio (6.4.1). Una riga con un'altra fonte è un errore di
// contratto della fotografia: la query la esclude.
type Triage struct {
	ID             uuid.UUID  `json:"id"`
	MessaggioID    uuid.UUID  `json:"messaggio_id"`
	Esito          string     `json:"esito"`
	Atto           string     `json:"atto,omitempty"`
	Legame         string     `json:"legame,omitempty"`
	Stato          string     `json:"stato"`
	Identificativi []string   `json:"identificativi,omitempty"`
	CreatoIl       time.Time  `json:"creato_il"`
	DecisoIl       *time.Time `json:"deciso_il,omitempty"`
}

// CandidatoCodice: un codice della mail secondo il vecchio motore, mai di origine «agente» (6.4.1). Serve solo al
// «vecchio» che valutazione prepara per il confronto: Interpreta e ancoraggio non lo vedono mai.
type CandidatoCodice struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
	Codice      string    `json:"codice"`
	Ruolo       string    `json:"ruolo"`
	Rev         string    `json:"rev,omitempty"`
	Origine     string    `json:"origine"`
	Famiglia    string    `json:"famiglia,omitempty"`
	Punteggio   int16     `json:"punteggio"`
}
