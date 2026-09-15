// Package api definisce i contratti JSON fra cockpit.exe e i worker Python.
// La fonte parallela è workers/contratti.py (pydantic); contracts/*.schema.json è generato da lì
// e verificato dai test di entrambe le parti. I nomi dei campi JSON sono in italiano, snake_case.
package api

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------- ingest (FATTO)

type Destinatario struct {
	Nome      string `json:"nome"`
	Indirizzo string `json:"indirizzo"`
	Tipo      string `json:"tipo"` // a | cc | ccn
}

type AllegatoIn struct {
	Indice      int    `json:"indice"`
	NomeFile    string `json:"nome_file"`
	Estensione  string `json:"estensione"`
	ContentType string `json:"content_type,omitempty"`
	Natura      string `json:"natura"` // file | inline | elemento_outlook | collegamento
	Bytes       int64  `json:"bytes"`
	ContentID   string `json:"content_id,omitempty"`
}

// MessaggioIn è un elemento Outlook letto via COM. L'identità è message_id (Internet Message-ID),
// non entry_id, che cambia quando l'elemento viene spostato di cartella.
type MessaggioIn struct {
	MessageID         string    `json:"message_id"`
	ParentMessageID   string    `json:"parent_message_id,omitempty"`
	EntryID           string    `json:"entry_id"`
	StoreID           string    `json:"store_id"`
	ConversationID    string    `json:"conversation_id,omitempty"`
	ConversationIndex string    `json:"conversation_index,omitempty"`
	InReplyTo         string    `json:"in_reply_to,omitempty"`
	Riferimenti       []string  `json:"riferimenti"`
	Cartella          string    `json:"cartella"`
	Direzione         string    `json:"direzione"` // entrata | uscita — il server lo ricalcola dalle caselle censite
	DataEvento        time.Time `json:"data_evento"`
	// RicevutoIl è il ReceivedTime dell'elemento IN QUESTA CASELLA, sempre, anche per la posta
	// inviata. Chiude W2: il cursore avanzava su data_evento, che per la Posta inviata è SentOn,
	// mentre il filtro della scansione usa ReceivedTime. Sono grandezze diverse, e una mail scritta
	// lunedì e inviata giovedì poteva spingere il cursore oltre elementi non ancora letti.
	// Assente (worker vecchio) = si usa data_evento, che è ciò che si faceva prima.
	RicevutoIl        *time.Time     `json:"ricevuto_il,omitempty"`
	MittenteNome      string         `json:"mittente_nome"`
	MittenteIndirizzo string         `json:"mittente_indirizzo"`
	Destinatari       []Destinatario `json:"destinatari"`
	Oggetto           string         `json:"oggetto"`
	CorpoTesto        string         `json:"corpo_testo"`
	CorpoHTML         string         `json:"corpo_html,omitempty"`
	Importanza        int            `json:"importanza"`
	NonLetto          bool           `json:"non_letto"`
	FlagStato         int            `json:"flag_stato"`
	Categorie         []string       `json:"categorie"`
	Allegati          []AllegatoIn   `json:"allegati"`
}

// CursoreLotto: fin dove arriva questo lotto. Il server lo scrive nella stessa transazione degli
// elementi, quindi o avanzano insieme o non avanza niente.
type CursoreLotto struct {
	Cartella       string    `json:"cartella"`
	UltimoReceived time.Time `json:"ultimo_received"`
}

// ElementoSaltato: il worker ha visto l'elemento ma non è riuscito a convertirlo (com_error su Body,
// elemento non-mail, ...). Non è un dato da buttare: finisce in scarto con origine 'lettura' e si
// rilegge con un job dedicato, perché il payload qui non c'è.
type ElementoSaltato struct {
	EntryID    string     `json:"entry_id"`
	Cartella   string     `json:"cartella,omitempty"`
	MessageID  string     `json:"message_id,omitempty"`
	RicevutoIl *time.Time `json:"ricevuto_il,omitempty"`
	Oggetto    string     `json:"oggetto,omitempty"`
	Errore     string     `json:"errore"`
}

type IngestRichiesta struct {
	Messaggi []MessaggioIn `json:"messaggi"`
	// Il lotto appartiene a una casella sola: casella_id sta qui, non nei singoli elementi, e viene
	// verificato prima di aprire la transazione.
	CasellaID *uuid.UUID `json:"casella_id,omitempty"`
	// Il tentativo che sta consegnando il lotto. Senza, il server non ha modo di distinguere un
	// tentativo vivo da uno scaduto e accetterebbe scritture di un tentativo che non esiste più.
	JobID      int64             `json:"job_id"`
	LeaseToken string            `json:"lease_token"`
	WorkerID   string            `json:"worker_id"`
	Cursore    *CursoreLotto     `json:"cursore,omitempty"`
	Saltati    []ElementoSaltato `json:"saltati,omitempty"`
}

type EsitoMessaggio struct {
	MessageID       string     `json:"message_id"`
	MessaggioID     uuid.UUID  `json:"messaggio_id"`
	Inserito        bool       `json:"inserito"`
	ThreadID        *uuid.UUID `json:"thread_id,omitempty"`
	Aggancio        string     `json:"aggancio"`
	AllegatiDaStage int        `json:"allegati_da_stage"` // sempre 0 dal 13/09: lo staging è su richiesta dell'operatore
	Errore          string     `json:"errore,omitempty"`  // valorizzato = elemento scartato, non acquisito
}

type IngestRisposta struct {
	Inseriti   int              `json:"inseriti"`
	Aggiornati int              `json:"aggiornati"`
	Falliti    int              `json:"falliti"`
	Esiti      []EsitoMessaggio `json:"esiti"`
}

// ---------------------------------------------------------------- coda job

type ClaimRichiesta struct {
	Worker   string `json:"worker"`    // outlook | analisi
	WorkerID string `json:"worker_id"` // es. "outlook@PC-PIETRO#1234"
	AttesaS  int    `json:"attesa_s"`  // long-poll massimo (il server tronca a 25 s)
}

type Job struct {
	JobID     int64           `json:"job_id"`
	Tipo      string          `json:"tipo"`
	Payload   json.RawMessage `json:"payload"`
	Tentativi int             `json:"tentativi"`
	LeaseS    int             `json:"lease_s"`
	// LeaseToken identifica QUESTO tentativo. Va rimandato indietro in heartbeat, result e ingest:
	// è l'unica cosa che distingue il tentativo in corso da uno scaduto che sta ancora lavorando.
	LeaseToken string     `json:"lease_token"`
	DurataMaxS int        `json:"durata_max_s"` // oltre questa durata il tentativo non vale più, lease fresco o no
	CasellaID  *uuid.UUID `json:"casella_id,omitempty"`
}

type HeartbeatRichiesta struct {
	WorkerID   string `json:"worker_id"`
	LeaseToken string `json:"lease_token"`
}

type RisultatoRichiesta struct {
	Esito      string          `json:"esito"` // ok | errore
	Dati       json.RawMessage `json:"dati,omitempty"`
	Errore     string          `json:"errore,omitempty"`
	Definitivo bool            `json:"definitivo,omitempty"` // true = non ritentare
	WorkerID   string          `json:"worker_id"`
	LeaseToken string          `json:"lease_token"`
}

// ---------------------------------------------------------------- payload e risultati per tipo di job

type CartellaCursore struct {
	Cartella       string     `json:"cartella"`
	UltimoReceived *time.Time `json:"ultimo_received,omitempty"`
}

type PayloadSyncOutlook struct {
	// La casella da sincronizzare: il worker la rimanda nell'ingest, così il lotto ha una casella
	// sola e verificabile invece di ereditarla da una configurazione locale.
	CasellaID        *uuid.UUID        `json:"casella_id,omitempty"`
	Cartelle         []CartellaCursore `json:"cartelle"`
	Dal              time.Time         `json:"dal"`               // limite inferiore assoluto (cursore vuoto)
	Al               *time.Time        `json:"al,omitempty"`      // limite superiore opzionale (sync storico)
	SovrapposizioneS int               `json:"sovrapposizione_s"` // rilettura di sicurezza dietro al cursore
	Lotto            int               `json:"lotto"`
}

type CartellaEsito struct {
	Cartella       string     `json:"cartella"`
	UltimoReceived *time.Time `json:"ultimo_received,omitempty"`
	NMessaggi      int        `json:"n_messaggi"`
	Errore         string     `json:"errore,omitempty"`
}

type RisultatoSync struct {
	Cartelle []CartellaEsito `json:"cartelle"`
}

// PayloadRileggiElemento: rilettura mirata di un solo elemento di una casella, dopo che il worker non
// era riuscito a convertirlo. Il worker risponde con un lotto da uno.
type PayloadRileggiElemento struct {
	CasellaID uuid.UUID `json:"casella_id"`
	EntryID   string    `json:"entry_id"`
	Cartella  string    `json:"cartella,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
}

// Riferimento a un elemento Outlook. entry_id/store_id sono la via rapida (GetItemFromID); se l'elemento è
// stato spostato l'EntryID non vale più e il worker lo ricerca per message_id (Internet Message-ID) in tutte
// le cartelle. messaggio_id e casella_id servono al server per riallineare la PRESENZA giusta con l'EntryID
// nuovo: dalla 0004 lo stesso messaggio ha un EntryID diverso in ogni casella, e scrivere quello trovato
// nella copia sbagliata significherebbe rompere l'accesso all'elemento nell'altra casella.
type RiferimentoElemento struct {
	MessaggioID *uuid.UUID `json:"messaggio_id,omitempty"`
	CasellaID   *uuid.UUID `json:"casella_id,omitempty"`
	MessageID   string     `json:"message_id,omitempty"`
}

// RisultatoElemento è restituito dai job che toccano un elemento: EntryID/cartella dove è stato trovato davvero.
type RisultatoElemento struct {
	EntryID  string `json:"entry_id,omitempty"`
	StoreID  string `json:"store_id,omitempty"`
	Cartella string `json:"cartella,omitempty"`
}

type PayloadStageAllegato struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	EntryID    string    `json:"entry_id"`
	StoreID    string    `json:"store_id"`
	Indice     int       `json:"indice"`
	NomeFile   string    `json:"nome_file"`
	Cartella   string    `json:"cartella"` // sottocartella di staging (hash del message_id)
	RiferimentoElemento
}

type RisultatoStage struct {
	AllegatoID  uuid.UUID `json:"allegato_id"`
	PathStaging string    `json:"path_staging"`
	Sha256      string    `json:"sha256"`
	Bytes       int64     `json:"bytes"`
	RisultatoElemento
}

type PayloadCreaBozza struct {
	BozzaID     uuid.UUID      `json:"bozza_id"`
	Tipo        string         `json:"tipo"` // risposta | rispondi_tutti | inoltro | nuovo | sollecito
	EntryID     string         `json:"entry_id,omitempty"`
	StoreID     string         `json:"store_id,omitempty"`
	Destinatari []Destinatario `json:"destinatari"`
	Oggetto     string         `json:"oggetto,omitempty"`
	CorpoHTML   string         `json:"corpo_html,omitempty"`
	CorpoTesto  string         `json:"corpo_testo,omitempty"`
	Allegati    []string       `json:"allegati"` // percorsi assoluti leggibili dal worker
	Mostra      bool           `json:"mostra"`   // Display() in Outlook
	Invia       bool           `json:"invia"`    // Send(): solo se il worker ha consenti_invio
	RiferimentoElemento
}

type RisultatoBozza struct {
	EntryID string `json:"entry_id"`
	Inviata bool   `json:"inviata"`
}

type PayloadApriElemento struct {
	EntryID string `json:"entry_id"`
	StoreID string `json:"store_id"`
	RiferimentoElemento
}

type PayloadSpostaCartella struct {
	EntryID  string `json:"entry_id"`
	StoreID  string `json:"store_id"`
	Cartella string `json:"cartella"`
	RiferimentoElemento
}

type RisultatoSposta struct {
	EntryID string `json:"entry_id"`
	StoreID string `json:"store_id,omitempty"`
}

type PayloadSegnaLetto struct {
	EntryID string `json:"entry_id"`
	StoreID string `json:"store_id"`
	Letto   bool   `json:"letto"`
	RiferimentoElemento
}

type PayloadCopiaNAS struct {
	DocumentoID uuid.UUID `json:"documento_id"`
}

type PayloadCreaCartellaThread struct {
	ThreadID uuid.UUID `json:"thread_id"`
}

type PayloadAnalizzaAllegato struct {
	AllegatoID  uuid.UUID  `json:"allegato_id"`
	PathStaging string     `json:"path_staging"`
	Sha256      string     `json:"sha256"`
	NomeFile    string     `json:"nome_file"`
	ThreadID    *uuid.UUID `json:"thread_id,omitempty"`
	MessaggioID uuid.UUID  `json:"messaggio_id"`
	// Con che cosa va analizzato (voce 1.12). Il worker li rimanda indietro tali e quali: servono al
	// server per sapere sotto quale chiave conservare i fatti, e per accorgersi se a rispondere è
	// stato un worker con una configurazione diversa da quella richiesta.
	VersioneAnalizzatore int            `json:"versione_analizzatore"`
	HashConfigurazione   string         `json:"hash_configurazione"`
	Parametri            map[string]any `json:"parametri,omitempty"`
}

type RisultatoAnalisi struct {
	AllegatoID   uuid.UUID       `json:"allegato_id"`
	TipoProposto string          `json:"tipo_proposto"`
	Codice       string          `json:"codice,omitempty"`
	Rev          string          `json:"rev,omitempty"`
	Confidenza   int             `json:"confidenza"`
	Fonte        string          `json:"fonte"`
	Dettagli     json.RawMessage `json:"dettagli,omitempty"`
	// Rimandati indietro dal payload: identificano la combinazione (contenuto, versione,
	// configurazione) sotto cui questi fatti valgono. Un risultato che dichiara una combinazione
	// diversa da quella chiesta non viene applicato: sarebbe un fatto archiviato sotto la chiave
	// sbagliata, e verrebbe riusato per file che non c’entrano.
	VersioneAnalizzatore int    `json:"versione_analizzatore"`
	HashConfigurazione   string `json:"hash_configurazione"`
}

// ---------------------------------------------------------------- healthz

type Salute struct {
	DB       string `json:"db"`
	NAS      string `json:"nas"`
	Versione int    `json:"schema_versione"`
}
