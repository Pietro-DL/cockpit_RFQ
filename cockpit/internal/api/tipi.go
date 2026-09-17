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
	MessageID       string `json:"message_id"`
	ParentMessageID string `json:"parent_message_id,omitempty"`
	EntryID         string `json:"entry_id"`
	// StoreID è accettato per compatibilità con un worker più vecchio e IGNORATO dal server: lo
	// StoreID è del profilo Outlook della postazione, non della copia (voce 2.6, N44), e il server
	// lo apprende dal claim del worker (casella_store), non dal lotto.
	StoreID           string    `json:"store_id,omitempty"`
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

// TolleranzaFuturo è quanto un messaggio può dichiarare di essere arrivato «dopo adesso» prima che
// il server smetta di credergli.
//
// Un po' di futuro è normale: l'orologio del PC del worker e quello del server non sono lo stesso, e
// fra la lettura in Outlook e la scrittura in database passa qualche istante. Due ore no. Il
// 16/09/2026 ci sono arrivate esattamente due ore — pywin32 consegna le date di Outlook con i numeri
// dell'ora locale e l'etichetta UTC — e il danno non è stato il valore sbagliato in sé: è stato il
// CURSORE, che è avanzato con loro. Un cursore nel futuro apre una finestra che comincia fra due ore,
// e da quel momento il sync non legge più niente senza che niente lo segnali.
//
// La correzione sta nel worker (`_utc` in outlook_com.py). Questa è la guardia: il server è l'ultimo
// posto in cui il difetto si può fermare prima che diventi un cursore, e deve fermarlo anche quando
// arriva da un worker più vecchio, o da un PC con l'orologio sbagliato.
const TolleranzaFuturo = 5 * time.Minute

// NelFuturo: `quando` è più avanti di `adesso` di quanto sia spiegabile con due orologi diversi. Un
// istante vuoto non è nel futuro: è un dato che manca, e lo trattano gli altri controlli.
//
// Sta qui, in un file che altrimenti contiene solo tipi, perché la stessa domanda se la fanno tre
// punti lontani fra loro — l'elemento in ingresso, il cursore che il lotto porta con sé, il cursore
// già scritto in database quando si accoda il sync successivo — e la risposta deve essere una sola.
func NelFuturo(quando, adesso time.Time) bool {
	return !quando.IsZero() && quando.After(adesso.Add(TolleranzaFuturo))
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

// CasellaAperta: come il worker vede una casella censita nel PROPRIO profilo Outlook (voce 2.6).
// Lo store_id ha senso solo su quella postazione: il server lo registra in casella_store e non lo
// mette mai in un payload.
type CasellaAperta struct {
	CasellaID uuid.UUID `json:"casella_id"`
	StoreID   string    `json:"store_id"`
}

// CasellaServita è una casella che il server chiede al worker di risolvere nel proprio profilo:
// risposta di GET /api/v1/worker/caselle. Il worker tocca solo queste; uno store del profilo che non
// è qui viene ignorato, non censito d'ufficio (M1).
type CasellaServita struct {
	CasellaID uuid.UUID `json:"casella_id"`
	Indirizzo string    `json:"indirizzo"`
	Nome      string    `json:"nome"`
	Condivisa bool      `json:"condivisa"`
}

type ClaimRichiesta struct {
	Worker   string `json:"worker"`    // outlook | analisi
	WorkerID string `json:"worker_id"` // es. "outlook@PC-FRANCESCO": la chiave di worker_credenziale
	AttesaS  int    `json:"attesa_s"`  // long-poll massimo (il server tronca a 25 s)
	// Postazione è il nome host da cui il worker gira. Il server NON la usa per il routing — usa la
	// postazione della credenziale — ma la confronta: un worker.toml copiato su un altro PC farebbe
	// eseguire su PC-B i job interattivi di PC-A, e il claim lo rifiuta.
	Postazione string `json:"postazione,omitempty"`
	// OutlookOk: false = il worker gira ma COM non risponde. La testata lo mostra così com'è.
	OutlookOk bool `json:"outlook_ok"`
	// CaselleAperte: le caselle censite che il worker ha risolto nel proprio profilo. Il server le
	// interseca con worker_credenziale.caselle (Q18) e assegna solo job di quelle (M12).
	CaselleAperte []CasellaAperta `json:"caselle_aperte,omitempty"`
	// UltimoArresto: motivo dell'ultima uscita forzata (C16), letto dal marcatore al riavvio.
	UltimoArresto string `json:"ultimo_arresto,omitempty"`
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
	// PostazioneID: job interattivo destinato a QUESTA postazione (voce 2.2). Il claim lo ha già
	// filtrato; qui è informazione per il log del worker.
	PostazioneID *uuid.UUID `json:"postazione_id,omitempty"`
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
	// Saltati: elementi visti e non consegnati (non-mail, proprieta illeggibile). Quelli che hanno
	// un EntryID arrivano anche in IngestRichiesta.Saltati e diventano scarti di lettura; questo e
	// il conto di tutti, compresi quelli che non hanno detto nemmeno chi fossero.
	Saltati int    `json:"saltati"`
	Errore  string `json:"errore,omitempty"`
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

// Riferimento a un elemento Outlook: IDENTITÀ LOGICHE, mai uno StoreID (voce 2.6, M12).
//
// casella_id dice in quale casella cercare: il worker la traduce nello store del PROPRIO profilo
// (casella_store), che è l'unico modo in cui uno StoreID può avere senso su una postazione diversa da
// quella che ha sincronizzato. entry_id è la via rapida (GetItemFromID); se l'elemento è stato spostato
// l'EntryID non vale più e il worker lo ricerca per message_id (Internet Message-ID) DENTRO quello
// store. messaggio_id e casella_id servono anche al server per riallineare la PRESENZA giusta con
// l'EntryID nuovo: dalla 0004 lo stesso messaggio ha un EntryID diverso in ogni casella.
type RiferimentoElemento struct {
	MessaggioID *uuid.UUID `json:"messaggio_id,omitempty"`
	CasellaID   *uuid.UUID `json:"casella_id,omitempty"`
	MessageID   string     `json:"message_id,omitempty"`
}

// RisultatoElemento è restituito dai job che toccano un elemento: EntryID/cartella dove è stato
// trovato davvero. Nessuno store_id: al server non direbbe niente.
type RisultatoElemento struct {
	EntryID  string `json:"entry_id,omitempty"`
	Cartella string `json:"cartella,omitempty"`
}

type PayloadStageAllegato struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	EntryID    string    `json:"entry_id"`
	Indice     int       `json:"indice"`
	NomeFile   string    `json:"nome_file"`
	Cartella   string    `json:"cartella"` // sottocartella di staging (hash del message_id)
	RiferimentoElemento
}

// RisultatoStage chiude un download. Il FILE non viaggia qui: il worker lo ha già caricato con
// PUT /api/v1/allegati/{id}/file, legato allo stesso tentativo (voce 2.3, D8), e il server lo tiene
// come .parte.<lease_token> finché questo result — valido — non lo promuove a definitivo dopo aver
// verificato lo sha256. Non c'è più un path_staging dichiarato dal worker: un percorso sul disco di
// un altro PC non dice niente al server, e un worker sullo stesso PC non ha motivo di essere un caso
// a parte.
type RisultatoStage struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Sha256     string    `json:"sha256"`
	Bytes      int64     `json:"bytes"`
	RisultatoElemento
}

type PayloadCreaBozza struct {
	BozzaID     uuid.UUID      `json:"bozza_id"`
	Tipo        string         `json:"tipo"` // risposta | rispondi_tutti | inoltro | nuovo | sollecito
	EntryID     string         `json:"entry_id,omitempty"`
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
	RiferimentoElemento
}

type PayloadSpostaCartella struct {
	EntryID  string `json:"entry_id"`
	Cartella string `json:"cartella"`
	RiferimentoElemento
}

type RisultatoSposta struct {
	EntryID string `json:"entry_id"`
}

type PayloadSegnaLetto struct {
	EntryID string `json:"entry_id"`
	Letto   bool   `json:"letto"`
	RiferimentoElemento
}

type PayloadCopiaNAS struct {
	DocumentoID uuid.UUID `json:"documento_id"`
}

type PayloadCreaCartellaThread struct {
	ThreadID uuid.UUID `json:"thread_id"`
}

// PayloadAnalizzaMessaggioAI: l'analisi semantica di UN messaggio gia' in database (checkpoint 3R §9).
//
// Porta solo l'identificativo. Che cosa esca verso il servizio esterno lo decide il server leggendo
// il database (agente.Servizio.Contesto), non chi accoda il job: se il testo viaggiasse nel payload,
// la stessa decisione sarebbe presa in ogni punto che accoda, e prima o poi in uno sarebbe diversa.
type PayloadAnalizzaMessaggioAI struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
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
