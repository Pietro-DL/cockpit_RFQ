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
	MessageID         string         `json:"message_id"`
	ParentMessageID   string         `json:"parent_message_id,omitempty"`
	EntryID           string         `json:"entry_id"`
	StoreID           string         `json:"store_id"`
	ConversationID    string         `json:"conversation_id,omitempty"`
	ConversationIndex string         `json:"conversation_index,omitempty"`
	InReplyTo         string         `json:"in_reply_to,omitempty"`
	Riferimenti       []string       `json:"riferimenti"`
	Cartella          string         `json:"cartella"`
	Direzione         string         `json:"direzione"` // entrata | uscita
	DataEvento        time.Time      `json:"data_evento"`
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

type IngestRichiesta struct {
	Messaggi []MessaggioIn `json:"messaggi"`
}

type EsitoMessaggio struct {
	MessageID       string     `json:"message_id"`
	MessaggioID     uuid.UUID  `json:"messaggio_id"`
	Inserito        bool       `json:"inserito"`
	ThreadID        *uuid.UUID `json:"thread_id,omitempty"`
	Aggancio        string     `json:"aggancio"`
	AllegatiDaStage int        `json:"allegati_da_stage"`
}

type IngestRisposta struct {
	Inseriti   int              `json:"inseriti"`
	Aggiornati int              `json:"aggiornati"`
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
}

type HeartbeatRichiesta struct {
	WorkerID string `json:"worker_id"`
}

type RisultatoRichiesta struct {
	Esito      string          `json:"esito"` // ok | errore
	Dati       json.RawMessage `json:"dati,omitempty"`
	Errore     string          `json:"errore,omitempty"`
	Definitivo bool            `json:"definitivo,omitempty"` // true = non ritentare
}

// ---------------------------------------------------------------- payload e risultati per tipo di job

type CartellaCursore struct {
	Cartella       string     `json:"cartella"`
	UltimoReceived *time.Time `json:"ultimo_received,omitempty"`
}

type PayloadSyncOutlook struct {
	Cartelle         []CartellaCursore `json:"cartelle"`
	Dal              time.Time         `json:"dal"`               // limite inferiore assoluto (cursore vuoto)
	Al               *time.Time        `json:"al,omitempty"`     // limite superiore opzionale (sync storico)
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

type PayloadStageAllegato struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	EntryID    string    `json:"entry_id"`
	StoreID    string    `json:"store_id"`
	Indice     int       `json:"indice"`
	NomeFile   string    `json:"nome_file"`
	Cartella   string    `json:"cartella"` // sottocartella di staging (hash del message_id)
}

type RisultatoStage struct {
	AllegatoID  uuid.UUID `json:"allegato_id"`
	PathStaging string    `json:"path_staging"`
	Sha256      string    `json:"sha256"`
	Bytes       int64     `json:"bytes"`
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
}

type RisultatoBozza struct {
	EntryID string `json:"entry_id"`
	Inviata bool   `json:"inviata"`
}

type PayloadApriElemento struct {
	EntryID string `json:"entry_id"`
	StoreID string `json:"store_id"`
}

type PayloadSpostaCartella struct {
	EntryID  string `json:"entry_id"`
	StoreID  string `json:"store_id"`
	Cartella string `json:"cartella"`
}

type RisultatoSposta struct {
	EntryID string `json:"entry_id"`
}

type PayloadSegnaLetto struct {
	EntryID string `json:"entry_id"`
	StoreID string `json:"store_id"`
	Letto   bool   `json:"letto"`
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
}

type RisultatoAnalisi struct {
	AllegatoID   uuid.UUID       `json:"allegato_id"`
	TipoProposto string          `json:"tipo_proposto"`
	Codice       string          `json:"codice,omitempty"`
	Rev          string          `json:"rev,omitempty"`
	Confidenza   int             `json:"confidenza"`
	Fonte        string          `json:"fonte"`
	Dettagli     json.RawMessage `json:"dettagli,omitempty"`
}

// ---------------------------------------------------------------- healthz

type Salute struct {
	DB       string `json:"db"`
	NAS      string `json:"nas"`
	Versione int    `json:"schema_versione"`
}
