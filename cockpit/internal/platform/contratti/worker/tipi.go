// Package worker definisce i contratti JSON fra cockpit.exe e i worker Python.
// La fonte parallela è workers/contratti.py (pydantic); contracts/*.schema.json è generato da lì
// e verificato dai test di entrambe le parti. I nomi dei campi JSON sono in italiano, snake_case.
package worker

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
	// Marcatori sono le UserProperties `Cockpit*` dell'elemento, che il Cockpit stesso ha scritto
	// creando una bozza (blocco 7B): `CockpitBozza` = bozza_id, `CockpitRichiestaFornitore` =
	// richiesta_id. Quando la mail compare nella Posta inviata, il marcatore la lega a ciò che l'ha
	// generata senza euristiche. Assente (worker vecchio) = nessun marcatore.
	Marcatori map[string]string `json:"marcatori,omitempty"`
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
	// DurataMs: quanto il SERVER ha impiegato a scrivere il lotto (7C.1, P1). Il worker lo mette
	// accanto al tempo della chiamata HTTPS: la differenza e' la rete. Senza, «il bootstrap e' lento»
	// resta una frase, e si da' la colpa alla rete senza averla misurata.
	DurataMs int64 `json:"durata_ms"`
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
	AttesaS  int    `json:"attesa_s"`  // long-poll chiesto; ≤0 o oltre 25 s il server usa 20 s
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

// I tre modi di un sync_outlook (blocco 3 del 3R). Non sono tre meccanismi: sono tre modi di
// decidere la PRIMA finestra. Da lì in poi tutti e tre leggono un intervallo chiuso [dal, al] fissato
// all'accodamento, dal più recente al più vecchio, e scrivono la propria frontiera solo quando la
// finestra è stata percorsa per intero.
//
//	ModoAggiornamento  esiste già una copertura: dal = coperto_fino_a - sovrapposizione, al = adesso;
//	ModoBootstrap      nessuna cartella di questa casella ha una copertura: al = adesso, dal = al - giorni_sync_iniziale.
//	                   Bootstrap NON vuol dire «riavvio»: vuol dire «di questa (casella, cartella) non
//	                   sappiamo ancora niente». Appena una finestra si chiude, non si torna più qui;
//	ModoStorico        «Carica precedenti»: al = storico_fino_a, dal = al - GiorniStorico.
//
// Il modo decide QUALE frontiera avanza a fine finestra, e solo quella: storico muove
// `storico_fino_a` verso il passato, gli altri due `coperto_fino_a` verso il presente.
const (
	ModoAggiornamento = "aggiornamento"
	ModoBootstrap     = "bootstrap"
	ModoStorico       = "storico"
)

// CartellaCursore è una cartella da leggere con la SUA finestra. `Dal` è calcolato dal server
// all'accodamento e non si ricava sul worker: due cartelle della stessa casella possono essere a
// punti diversi (una sincronizzata da mesi, una aggiunta stamattina), e il limite inferiore di
// ciascuna dipende dalla sua copertura, non da quella della vicina.
type CartellaCursore struct {
	Cartella string `json:"cartella"`
	// Dal: limite inferiore di QUESTA cartella, sovrapposizione già sottratta. Zero solo nei payload
	// accodati prima della 0010 e ancora in coda: il worker ripiega su PayloadSyncOutlook.Dal.
	Dal *time.Time `json:"dal,omitempty"`
	// Al: limite superiore di QUESTA cartella. Uguale per tutte nell'aggiornamento e nel bootstrap
	// (è l'istante in cui il job è stato accodato), diverso per ciascuna nello storico, dove ognuna
	// riprende da dove è arrivato il proprio `storico_fino_a`. Prendere il più vecchio dei limiti
	// storici e applicarlo a tutte — come si faceva — lasciava scoperto, nella cartella più avanti,
	// l'intervallo fra il proprio limite e quello della vicina, e lo dichiarava coperto lo stesso.
	Al *time.Time `json:"al,omitempty"`
	// Bootstrap: questa cartella non aveva ancora una frontiera di copertura, quindi `Dal` è la
	// finestra iniziale e non un cursore. Serve a dirlo nel log e nei test: una cartella aggiunta
	// oggi a una casella sincronizzata da mesi è in bootstrap mentre le sue vicine non lo sono.
	Bootstrap bool `json:"bootstrap,omitempty"`
	// UltimoReceived e CopertoFinoA viaggiano per diagnosi: il worker li scrive nel log della
	// cartella. Il primo è la mail più recente che abbiamo, il secondo fin dove si era guardato.
	UltimoReceived *time.Time `json:"ultimo_received,omitempty"`
	CopertoFinoA   *time.Time `json:"coperto_fino_a,omitempty"`
}

type PayloadSyncOutlook struct {
	// La casella da sincronizzare: il worker la rimanda nell'ingest, così il lotto ha una casella
	// sola e verificabile invece di ereditarla da una configurazione locale.
	CasellaID *uuid.UUID        `json:"casella_id,omitempty"`
	Modo      string            `json:"modo,omitempty"` // aggiornamento | bootstrap | storico (vuoto = aggiornamento)
	Cartelle  []CartellaCursore `json:"cartelle"`
	// Dal: il limite inferiore della finestra COMPLESSIVA (il più vecchio dei `Cartelle[].Dal`). È
	// quello che l'operatore legge nel badge; il worker usa quello della singola cartella, e ripiega
	// su questo solo per un payload accodato prima della 0010.
	Dal time.Time `json:"dal"`
	// Al: limite superiore, FISSATO all'accodamento e uguale per tutte le cartelle. Dal blocco 3
	// c'è sempre, anche per l'aggiornamento ordinario: una finestra il cui estremo superiore è
	// «adesso» cambia mentre il job gira, e allora non si può dire di averla conclusa. Resta
	// opzionale nel contratto per i payload già in coda, dove nil vuol dire «fino a adesso».
	Al               *time.Time `json:"al,omitempty"`
	SovrapposizioneS int        `json:"sovrapposizione_s"` // rilettura di sicurezza dietro alla copertura
	Lotto            int        `json:"lotto"`
}

// ModoEffettivo e' il modo di questo sync, risolvendo i payload che non lo dichiarano.
//
// Un payload accodato prima del blocco 3 non ha il campo: li' l'unico segnale era il limite
// superiore, presente solo nello storico. La regola vecchia si legge ancora per i job rimasti in
// coda durante l'aggiornamento; i nuovi lo dichiarano.
//
// Sta qui, sul tipo, e non in chi lo legge, perche' a leggerlo sono in tre — chi applica il
// risultato, chi decide se gli allegati scendono da soli, e il worker in Python con
// `modo_effettivo()` — e tre scalette scritte a mano sono tre occasioni di discordare su un payload
// vecchio, cioe' proprio sul caso che nessuno riprodurra' mai in prova.
func (p PayloadSyncOutlook) ModoEffettivo() string {
	if p.Modo != "" {
		return p.Modo
	}
	if p.Al != nil {
		return ModoStorico
	}
	return ModoAggiornamento
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
	// Completa: il worker dichiara di aver percorso la finestra [dal, al] per INTERO. È l'unica
	// condizione che fa avanzare una frontiera, ed è una dichiarazione positiva, non l'assenza di un
	// errore: un'enumerazione COM che si interrompe a metà può finire senza eccezioni e senza
	// riempire `Errore`, e in lettura dal più recente al più vecchio ciò che resta fuori è la parte
	// VECCHIA della finestra — esattamente quella che nessuno andrebbe più a rileggere.
	//
	// Assente = false = la frontiera non si muove. Un worker che non conosce questo campo fa
	// rileggere la finestra; il contrario avrebbe fatto perdere posta.
	Completa bool `json:"completa,omitempty"`
}

type RisultatoSync struct {
	Cartelle []CartellaEsito `json:"cartelle"`
	// Tempi, in secondi, di dove il sync ha passato il suo tempo (7C.1, P1): `com` (l'enumerazione
	// e la lettura degli elementi in Outlook), `serializzazione` (da elemento COM a MessaggioIn),
	// `https` (le chiamate di ingest, andata e ritorno) e `server` (la parte di `https` passata
	// dentro il server, che la dichiara in IngestRisposta.durata_ms). Al banco del 20/09/2026 un
	// bootstrap di 324 messaggi ha impiegato 346 s e nessuno sapeva dove.
	Tempi map[string]float64 `json:"tempi,omitempty"`
}

// PayloadRileggiElemento: rilettura mirata di un solo elemento di una casella, dopo che il worker non
// era riuscito a convertirlo. Il worker risponde con un lotto da uno.
//
// Dalla 4.13 dello Smistamento il worker ha il ramo (`Worker.rileggi`): cerca l'elemento per EntryID e,
// se è stato spostato, per MessageID dentro lo store della casella; «non trovato» è un errore
// definitivo. Il lotto non porta un cursore, e il server non ne scriverebbe uno (ingest.Ingerisci): una
// rilettura non dice niente della finestra da cui l'elemento viene. Il result è un RisultatoElemento.
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

// PayloadEstraiArchivio: scompatta un archivio gia' in staging (blocco 4A).
//
// Non viaggia verso nessun worker esterno: lo esegue l'esecutore interno del server, che sta accanto
// al disco. Porta il solo identificativo dell'allegato perche' tutto il resto — dove sta il file, di
// che messaggio e', a quale richiesta appartiene — e' in database e puo' essere cambiato fra
// l'accodamento e l'esecuzione.
type PayloadEstraiArchivio struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
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
	// Marcatori: UserProperties `Cockpit*` da scrivere sulla bozza (blocco 7B), oltre a
	// `CockpitBozza` che il worker mette sempre. Il sync della Posta inviata le rilegge.
	Marcatori map[string]string `json:"marcatori,omitempty"`
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

// PayloadAnalizzaAllegato: l'analisi di un contenuto gia' in staging, fatta da un worker che puo'
// stare su un altro PC (7C.1, P0).
//
// NESSUN PERCORSO DEL SERVER. Fino al banco a due macchine del 20/09/2026 il payload portava
// `path_staging`, cioe' `C:\promatec\_staging\_contenuti\...` sul disco del server: il worker
// analisi sull'altro PC lo cercava sul PROPRIO disco e falliva con «file non trovato in staging»,
// e dopo l'estrazione di uno zip da 83 voci fallivano tutti. Ora il job dice CHE COSA analizzare
// (allegato, sha256, byte, nome) e il worker si prende i byte con
// GET /api/v1/allegati/{id}/contenuto dentro il proprio tentativo, li salva in una cartella sua,
// verifica lo sha256, analizza e cancella. Un worker sullo stesso PC del server fa lo stesso giro:
// non e' un caso a parte, come non lo e' per l'upload.
type PayloadAnalizzaAllegato struct {
	AllegatoID  uuid.UUID  `json:"allegato_id"`
	Sha256      string     `json:"sha256"`
	Bytes       int64      `json:"bytes"`
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

// ---------------------------------------------------------------- struttura STEP (FATTO)

// StrutturaSTEP è il grafo letto da un file STEP, dentro `RisultatoAnalisi.dettagli["struttura"]`.
//
// È un FATTO del contenuto: quali PRODUCT ci sono, quale contiene quale e quante volte. Non è una
// proposta e non è una decisione — la proposta la compone il server per ogni RFQ, con le regole del
// cliente di quella RFQ, e la decisione la prende l'ingegnere.
//
// Nei nodi non ci sono `codice` e `rev`, e non è una dimenticanza: che cosa sia un codice dipende
// dalle famiglie del cliente, che il worker non conosce. La `versione` dice proprio questo — la 1
// portava i codici scritti dal worker, la 2 porta i grezzi, la 3 aggiunge gli scarti in numeri — e il
// Go non deve leggere `nodi[].codice` mai più (addendum B8, A1.2 e A1.6.3).
type StrutturaSTEP struct {
	Versione int    `json:"versione"`
	Schema   string `json:"schema"` // AP214, AP203, AP242… da FILE_SCHEMA; "" se non leggibile
	// Radici: i nodi senza padre nel file. Un file di sola parte ne ha uno; un file con due assiemi
	// ne ha due, e non si sceglie per conto di nessuno quale sia «il» prodotto.
	Radici    []string        `json:"radici"`
	Nodi      []NodoSTEP      `json:"nodi"`
	Relazioni []RelazioneSTEP `json:"relazioni"`
	Avvisi    []string        `json:"avvisi"`
	// Scarti c'è dalla versione 3 (A4.4, D35); in una v2 manca, ed è nil.
	Scarti *ScartiSTEP `json:"scarti"`
	Limiti LimitiSTEP  `json:"limiti"`
	// Dalla versione 4 (GP-3, direttiva 21 §5.3): lo stato della struttura (Struttura…) e il suo motivo. In una
	// struttura di prima è "": lo stato non si deduce, si rilegge (StrutturaDaRileggere).
	Stato       string `json:"stato"`
	MotivoStato string `json:"motivo_stato"`
}

// VersioneStruttura è la versione della forma di `struttura` che il worker di oggi scrive (StrutturaSTEP.Versione). La
// 4 (GP-3) aggiunge la geometria per nodo (diagnostica), i nomi d'istanza delle NAUO e lo stato della struttura.
const VersioneStruttura = 4

// Gli stati della struttura di uno STEP (StrutturaSTEP.Stato, dalla v4). StrutturaAssente NON vuol dire «pezzo unico»:
// dice che nel file non c'è nessuna occorrenza d'assieme. Sono diagnostica: nessun nodo si toglie per questo.
const (
	StrutturaValida        = "valida"
	StrutturaParziale      = "parziale"       // un tetto, occorrenze non risolte, prodotti senza definizione
	StrutturaAssente       = "assente"        // letta, nessuna NAUO
	StrutturaNonEstraibile = "non_estraibile" // non Part 21, lettura fallita
)

// GeometriaSTEP è il riassunto della geometria di un nodo (v4), dalle rappresentazioni della sua forma e da quelle
// legate senza trasformazione: diagnostica, MAI motivo di rimozione. Vuota: nessun elemento geometrico (solo
// posizionamenti, o nessuna rappresentazione); un nodo vuoto con figli è un contenitore CAD.
type GeometriaSTEP struct {
	Solidi           int  `json:"solidi"`
	Superfici        int  `json:"superfici"`
	Curve            int  `json:"curve"`
	Mappati          int  `json:"mappati"`
	Rappresentazioni int  `json:"rappresentazioni"`
	Vuota            bool `json:"vuota"`
}

// IstanzaSTEP: un'occorrenza NAUO di una relazione, con l'id e il nome d'istanza grezzi (v4).
type IstanzaSTEP struct {
	Rif  string `json:"rif"`
	ID   string `json:"id"`
	Nome string `json:"nome"`
}

// StrutturaDaRileggere dice se «Rianalizza» deve riaccodare uno STEP con questi fatti dell'analizzatore corrente: la
// struttura manca o è di una versione di prima di VersioneStruttura (la geometria per nodo e lo stato non ci sono).
// Fino a GP-3 gli STEP si rileggevano solo con una versione nuova dell'analizzatore; l'idoneità per estensione (stp,
// step) è del chiamante (fascicolo, da estendere: consegna al core).
func StrutturaDaRileggere(fatti json.RawMessage) bool {
	if len(fatti) == 0 {
		return true
	}
	var involucro struct {
		Struttura *struct {
			Versione int `json:"versione"`
		} `json:"struttura"`
	}
	if err := json.Unmarshal(fatti, &involucro); err != nil || involucro.Struttura == nil {
		return true
	}
	return involucro.Struttura.Versione < VersioneStruttura
}

// ScartiSTEP: che cosa il lettore ha visto e lasciato fuori, in numeri (struttura v3).
//
// Se una lettura è COMPLETA non lo decide questo tipo: lo decide la funzione SQL
// struttura_motivo_parziale della 0020, che il server interroga (A4.4: «la definizione è una sola»).
// Qui ci sono solo i fatti che quella funzione guarda.
type ScartiSTEP struct {
	ProdottiSenzaDefinizione int `json:"prodotti_senza_definizione"`
	OccorrenzeNonRisolte     int `json:"occorrenze_non_risolte"`
	OccorrenzeSuSeStesse     int `json:"occorrenze_su_se_stesse"` // archi impossibili: non tolgono completezza
	TestiTroncati            int `json:"testi_troncati"`
}

// NodoSTEP è un PRODUCT del file con i suoi attributi grezzi e l'evidenza di dove stanno.
//
// `Chiave` è il riferimento dell'entità nel file (`#12`): identifica il nodo dentro QUEL file, ed è
// ciò che le relazioni collegano. Fuori dal file non significa niente.
type NodoSTEP struct {
	Chiave            string         `json:"chiave"`
	IDGrezzo          string         `json:"id_grezzo"`          // PRODUCT.id, primo attributo
	NomeGrezzo        string         `json:"nome_grezzo"`        // PRODUCT.name, secondo attributo
	DescrizioneGrezza string         `json:"descrizione_grezza"` // PRODUCT.description, terzo
	RevGrezza         string         `json:"rev_grezza"`         // PRODUCT_DEFINITION_FORMATION.id
	Evidenza          map[string]any `json:"evidenza"`           // entità e righe da cui viene
	// Geometria: il riassunto della geometria (v4, diagnostica); nil in una struttura di prima.
	Geometria *GeometriaSTEP `json:"geometria"`
}

// RelazioneSTEP: «il nodo padre contiene il nodo figlio, Qta volte».
//
// Una riga per COPPIA, non per occorrenza. Un nodo con due padri dà due relazioni, ed è il caso per
// cui padre e figlio non stanno sulla riga del nodo (A1.1).
type RelazioneSTEP struct {
	Padre    string         `json:"padre"`
	Figlio   string         `json:"figlio"`
	Qta      int            `json:"qta"`
	Evidenza map[string]any `json:"evidenza"`
	// Istanze: le prime occorrenze con id e nome d'istanza (v4, al più 20); vuota in una struttura di prima.
	Istanze []IstanzaSTEP `json:"istanze"`
}

// LimitiSTEP dice fin dove si è letto. `Troncato` vero significa che il grafo è PARZIALE: la
// schermata deve dirlo, perché un albero incompleto che si presenta come completo è peggio di un
// albero assente.
//
// I tre tetti tornano indietro con il risultato, e `Motivo` dice quale ha fermato la lettura. Sono
// nel fatto e non nella configurazione perché un fatto resta archiviato per anni: quando qualcuno
// riaprirà questa analisi, il `nodi_max` del worker di oggi non sarà più recuperabile da nessuna
// parte, e senza di esso «troncato» non si sa più che cosa volesse dire.
type LimitiSTEP struct {
	NodiMax       int     `json:"nodi_max"`
	OccorrenzeMax int     `json:"occorrenze_max"`
	TempoMaxS     float64 `json:"tempo_max_s"`
	ByteLetti     int64   `json:"byte_letti"`
	TempoS        float64 `json:"tempo_s"`
	Troncato      bool    `json:"troncato"`
	Motivo        string  `json:"motivo"` // "" se completo, altrimenti "nodi", "occorrenze" o "tempo"
}

// DecodificaStruttura estrae la struttura dai dettagli di un'analisi.
//
// Il secondo valore è falso quando la struttura non c'è (fatti v1, PDF, DXF) o quando la versione è
// più vecchia di quella che questo server sa leggere: i nodi della v1 portavano un `codice` deciso
// dal worker, e prenderlo per buono vorrebbe dire far entrare dalla finestra la classificazione che
// A1.2 ha tolto dalla porta. Quei fatti tornano utili dopo una rianalisi, non prima.
//
// Tollera i campi sconosciuti: un worker più nuovo del server può aggiungere attributi, e il server
// legge quelli che conosce senza rompersi.
func DecodificaStruttura(dettagli json.RawMessage) (*StrutturaSTEP, bool) {
	if len(dettagli) == 0 {
		return nil, false
	}
	var involucro struct {
		Struttura *StrutturaSTEP `json:"struttura"`
	}
	if err := json.Unmarshal(dettagli, &involucro); err != nil || involucro.Struttura == nil {
		return nil, false
	}
	if involucro.Struttura.Versione < 2 {
		return nil, false
	}
	return involucro.Struttura, true
}

// ---------------------------------------------------------------- testo dei PDF (Smistamento F9)

// Le zone di un frammento o di un campo del cartiglio (FrammentoPDF.Zona, CampoCartiglio.Zona).
const (
	// ZonaBassoDestra: la parte della pagina 1 con x0 >= 0,5 della larghezza e y0 >= 0,6 dell'altezza, sulla
	// pagina come si vede (rotazione applicata). È un fatto geometrico; il server lo chiama «in basso a destra,
	// probabile cartiglio».
	ZonaBassoDestra = "basso_destra"
	// ZonaPagina: tutto il resto.
	ZonaPagina = "pagina"
)

// Da dove viene il testo di un frammento o di un campo (FrammentoPDF.Fonte, CampoCartiglio.Fonte).
const (
	FonteTestoNativo = "nativo" // scritto nel file
	FonteTestoOCR    = "ocr"    // letto dall'immagine con l'OCR: un'evidenza più debole
)

// Gli stati dell'OCR di un PDF (OCRPDF.Stato).
const (
	OCRNonNecessario  = "non_necessario"  // il testo nativo bastava
	OCRSpento         = "spento"          // configurazione del worker
	OCRNonDisponibile = "non_disponibile" // il motore non c'è sulla postazione del worker
	OCREseguito       = "eseguito"        // almeno un passaggio ha letto qualcosa
	OCRFallito        = "fallito"
	OCRScaduto        = "scaduto"
	OCRIlleggibile    = "illeggibile"
)

// Le voci dei campi del cartiglio (CampoCartiglio.Etichetta).
const (
	CampoNumeroDisegno = "numero_disegno"
	CampoCodice        = "codice"
	CampoRevisione     = "revisione"
	CampoTitolo        = "titolo"
	CampoScala         = "scala"
	CampoMateriale     = "materiale"
	// CampoParticolareSimile: «PARTICOLARE SIMILE / SIMILAR PART» (giro 4, fase 4.6). Il valore e' il codice di
	// un pezzo gia' fatto di cui questo e' parente: per il server una nota («simile a X»), mai il codice del file.
	CampoParticolareSimile = "particolare_simile"
	// CampoRiferimento3D: «ref.3D doc. / Ref.3D Dok.» (K4), il documento 3D della stessa parte, letto nel cartiglio
	// di un DXF (TestoDXF) o con l'OCR di un PDF. È un dato del cartiglio, mai il codice del file: che cosa lega lo
	// dice il motore (ancoraggio per equivalenza), con la provenienza.
	CampoRiferimento3D = "riferimento_3d"
)

// VersioneTestoPDF e' la sottoversione del testo dei PDF che il worker di oggi scrive (TestoPDF.Versione, la
// forma di `testo_pdf`, non la versione dell'analizzatore). La 2 (giro 4, fase 4.6) legge il cartiglio meglio della
// 1: le etichette bilingui intere («DENOMINAZIONE / NAME»), «PART N°» e «PART NR» fra quelle del codice, «NAME»
// come gemella del titolo (da sola no), il campo del particolare simile (con il segnaposto del modello che non da'
// niente), e l'intestazione di una tabella («Pos. Part Number Q.ty», «N. CODICE DESCRIZIONE QTA») che non e' un
// campo. Un testo di una sottoversione precedente si legge lo stesso, ma il suo cartiglio non e' contenuto (chiavi
// e indizi, fase 4.6r) e «Rianalizza» lo riaccoda (classificazione.TestoPDFDaRileggere): la versione
// dell'analizzatore non cambia (domanda 21 del giro 4 senza risposta), e cosi' non decade niente di quello che e'
// legato alla chiave dei fatti. La fase 4.12 (l'elenco particolari) usera' la stessa sottoversione: un solo
// pacchetto della postazione, una sola rianalisi.
//
// La 3 (GP-3, direttiva 21 §5.1-5.2) aggiunge le tabelle (TabellaPDF: righe con colonne, valori grezzi e riquadri,
// da qualunque PDF con testo nativo e un'intestazione riconoscibile) e la loro completezza (CompletezzaPDF), e il
// worker confronta i termini della distinta senza dieresi. Il cartiglio si legge come nella 2: per questo la soglia
// del cartiglio «di prima» e' VersioneCartiglioPDF, non questa. «Rianalizza» riaccoda i PDF della 2 per le tabelle
// (classificazione.TestoPDFDaRileggere), senza declassarne il cartiglio.
const VersioneTestoPDF = 3

// VersioneCartiglioPDF e' la sottoversione piu' vecchia il cui cartiglio si legge come quello di oggi (fase 4.6: la
// 2). Un testo con una sottoversione minore ha il cartiglio «di prima» (classificazione.testoDiPrima,
// estrazione.contenutoPDF): chiavi e indizi, mai il codice del file. Si separa da VersioneTestoPDF dalla 3, che
// aggiunge le tabelle e non cambia il cartiglio.
const VersioneCartiglioPDF = 2

// CartiglioPDFDiPrima dice se il cartiglio di questo testo e' stato letto con le regole di prima della fase 4.6.
func CartiglioPDFDiPrima(t *TestoPDF) bool {
	return t != nil && t.Versione < VersioneCartiglioPDF
}

// TestoPDF è la lettura strutturata del testo di un PDF, dentro `RisultatoAnalisi.dettagli["testo_pdf"]`
// dall'analizzatore 4 (addendum A5.13.8, decisioni del 27/09 «ter»).
//
// Sono FATTI del contenuto, come la struttura di uno STEP, e per la stessa ragione non portano codici: il worker
// non sa per quale RFQ analizza, e che cosa sia un codice lo dicono le famiglie del cliente di ciascuna RFQ
// (A5.13.7). I codici li cerca il server (classificazione.EvidenzeTestoPDF), e chi li usa fuori dalla
// classificazione legge le evidenze normalizzate (classificazione.LettureDelPDF), non questo tipo.
//
// Il testo integrale di un documento grande non c'è: ci sono i frammenti utili con pagina e riquadro, i campi
// del probabile cartiglio, i metadati, l'esito dell'OCR selettivo e i limiti in vigore quando è stato letto.
// `Estraibile` falso vuol dire che nelle pagine lette non c'è testo NATIVO (curve, scansioni): è una risposta,
// non un errore, e l'OCR non lo cambia. I riquadri sono [x0, y0, x1, y1] in punti sulla pagina vista.
type TestoPDF struct {
	Versione       int              `json:"versione"`
	Estraibile     bool             `json:"estraibile"`
	Pagine         int              `json:"pagine"`
	PagineLette    int              `json:"pagine_lette"`
	Caratteri      int              `json:"caratteri"`
	Troncato       bool             `json:"troncato"`
	FormatoPagina1 []float64        `json:"formato_pagina1"`
	Frammenti      []FrammentoPDF   `json:"frammenti"`
	Cartiglio      []CampoCartiglio `json:"cartiglio"`
	Metadati       MetadatiPDF      `json:"metadati"`
	OCR            OCRPDF           `json:"ocr"`
	Limiti         LimitiTestoPDF   `json:"limiti"`
	// Dalla sottoversione 3 (GP-3): le tabelle e la completezza della loro lettura. In un testo della 2 mancano:
	// Tabelle e' nil e Completezza e' nil, e nil non vuol dire «nessuna tabella» ne' «letto tutto».
	Tabelle     []TabellaPDF    `json:"tabelle"`
	Completezza *CompletezzaPDF `json:"completezza"`
}

// I ruoli tipici di una colonna (ColonnaTabella.Ruolo, CellaTabella.Ruolo; workers/colonne.py): un aiuto di lettura,
// non un'interpretazione. RuoloNessuno = nessun ruolo riconosciuto, l'intestazione resta.
const (
	RuoloNessuno     = ""
	RuoloPosizione   = "posizione"
	RuoloLivello     = "livello"
	RuoloCodice      = "codice"
	RuoloDescrizione = "descrizione"
	RuoloQuantita    = "quantita"
	RuoloUnita       = "unita"
	RuoloMateriale   = "materiale"
	RuoloRevisione   = "revisione"
	RuoloDocumento   = "documento"
	RuoloNota        = "nota"
)

// Dove stanno le righe di una tabella rispetto all'intestazione (TabellaPDF.Verso).
const (
	VersoSotto = "sotto"
	VersoSopra = "sopra"
)

// Gli stati della completezza delle tabelle di un PDF (CompletezzaPDF.Stato).
const (
	CompletezzaCompleta     = "completa"      // tutte le pagine lette, tutte con testo nativo, nessun tetto
	CompletezzaParziale     = "parziale"      // pagine oltre il tetto o senza testo nativo, righe troncate (Motivo)
	CompletezzaNonLeggibile = "non_leggibile" // nessun testo nativo nelle pagine lette
)

// TabellaPDF e' una tabella riconosciuta dalla sua intestazione su una pagina (workers/tabelle_testo.py). Il worker
// estrae le righe e non dice che cosa siano: prodotti richiesti, componenti o menzioni lo decide il server con il
// profilo del cliente (PM-35R). Riquadri in punti sulla pagina vista, come i frammenti.
type TabellaPDF struct {
	Pagina int `json:"pagina"`
	Indice int `json:"indice"`
	// Verso: le righe stanno sotto l'intestazione (VersoSotto: un modulo, una distinta) o sopra (VersoSopra: l'elenco
	// particolari di un disegno, con l'intestazione appoggiata al cartiglio); numerate dall'intestazione in fuori.
	Verso                string           `json:"verso"`
	RiquadroIntestazione []float64        `json:"riquadro_intestazione"`
	Colonne              []ColonnaTabella `json:"colonne"`
	Righe                []RigaTabella    `json:"righe"`
}

// ColonnaTabella: l'intestazione com'e' scritta, il ruolo tipico, il riquadro. Omessa: una colonna personale
// (disegnato, controllato, firma): i suoi valori non viaggiano.
type ColonnaTabella struct {
	Indice       int       `json:"indice"`
	Intestazione string    `json:"intestazione"`
	Ruolo        string    `json:"ruolo"`
	Omessa       bool      `json:"omessa"`
	Riquadro     []float64 `json:"riquadro"`
}

// RigaTabella: una riga di dati. Continuazione: senza la cella della posizione (o del codice, se manca la colonna
// della posizione), di solito il seguito della riga sopra; mai fusa dal worker.
type RigaTabella struct {
	Indice        int            `json:"indice"`
	Riquadro      []float64      `json:"riquadro"`
	Continuazione bool           `json:"continuazione"`
	Celle         []CellaTabella `json:"celle"`
}

// CellaTabella: il valore grezzo di una colonna in una riga, con il riquadro. Sorgente: gli offset delle entita' nel
// file (righe d'elenco dei DWG); vuota per i PDF.
type CellaTabella struct {
	Colonna      int       `json:"colonna"`
	Intestazione string    `json:"intestazione"`
	Ruolo        string    `json:"ruolo"`
	Valore       string    `json:"valore"`
	Riquadro     []float64 `json:"riquadro"`
	Omessa       bool      `json:"omessa"`
	Sorgente     []int64   `json:"sorgente"`
}

// CompletezzaPDF dice fin dove le tabelle coprono il documento: con Stato diverso da CompletezzaCompleta il server dice
// «verifica incompleta», non «codice assente» (direttiva 21 §5.2).
type CompletezzaPDF struct {
	Stato            string `json:"stato"`
	Pagine           int    `json:"pagine"`
	PagineLette      int    `json:"pagine_lette"`
	PagineSenzaTesto int    `json:"pagine_senza_testo"`
	RigheTroncate    bool   `json:"righe_troncate"`
	Motivo           string `json:"motivo"`
}

// FrammentoPDF è un pezzo di testo con il suo posto: un blocco del file (o dell'OCR) dentro una zona. La
// Confidenza (0–100) c'è solo per l'OCR, e solo se il motore la dà.
type FrammentoPDF struct {
	Pagina     int       `json:"pagina"`
	Zona       string    `json:"zona"`
	Fonte      string    `json:"fonte"`
	Testo      string    `json:"testo"`
	Riquadro   []float64 `json:"riquadro"`
	Confidenza *float64  `json:"confidenza"`
}

// CampoCartiglio è un campo del probabile cartiglio della pagina 1, come etichetta → valore («DISEGNO N.» →
// «7120010»): Etichetta è la voce riconosciuta (Campo…), Letta com'è scritta (con la gemella inglese dopo la
// barra, dalla sottoversione 2: «DENOMINAZIONE / NAME»), Valore il testo GREZZO accanto, Riquadro quello del
// valore, Zona quella dell'etichetta.
type CampoCartiglio struct {
	Etichetta  string    `json:"etichetta"`
	Letta      string    `json:"letta"`
	Valore     string    `json:"valore"`
	Pagina     int       `json:"pagina"`
	Zona       string    `json:"zona"`
	Fonte      string    `json:"fonte"`
	Riquadro   []float64 `json:"riquadro"`
	Confidenza *float64  `json:"confidenza"`
}

// OCRPDF è quello che ha fatto l'OCR selettivo: lo stato (OCR…), il motivo di quello che non è andato, il
// motore e i passaggi. L'OCR non fa mai fallire l'analisi: quello che non va si scrive qui.
type OCRPDF struct {
	Stato     string         `json:"stato"`
	Motivo    string         `json:"motivo"`
	Motore    string         `json:"motore"`
	Tentativi []TentativoOCR `json:"tentativi"`
}

// TentativoOCR è un passaggio dell'OCR su una pagina intera o sulla zona in basso a destra della pagina 1, con
// l'esito (letto, vuoto, fallito, scaduto, saltato).
type TentativoOCR struct {
	Pagina    int     `json:"pagina"`
	Zona      string  `json:"zona"`
	Esito     string  `json:"esito"`
	Motivo    string  `json:"motivo"`
	Caratteri int     `json:"caratteri"`
	Secondi   float64 `json:"secondi"`
}

// MetadatiPDF sono i metadati del documento, grezzi. L'autore non c'è, di proposito: è il nome di una persona,
// non dice che cosa sia il file e non deve restare archiviato nei fatti.
type MetadatiPDF struct {
	Titolo       string `json:"titolo"`
	Soggetto     string `json:"soggetto"`
	ParoleChiave string `json:"parole_chiave"`
	Creatore     string `json:"creatore"`
	Produttore   string `json:"produttore"`
}

// LimitiTestoPDF: i tetti e le soglie in vigore QUANDO il testo è stato letto (come LimitiSTEP), perché
// «troncato» o «OCR non necessario» fra un anno significhino ancora qualcosa.
type LimitiTestoPDF struct {
	PagineMax          int `json:"pagine_max"`
	FrammentiMax       int `json:"frammenti_max"`
	FrammentoMax       int `json:"frammento_max"`
	CaratteriMax       int `json:"caratteri_max"`
	CampiMax           int `json:"campi_max"`
	OCRSogliaPagina    int `json:"ocr_soglia_pagina"`
	OCRSogliaCartiglio int `json:"ocr_soglia_cartiglio"`
	OCRPagineMax       int `json:"ocr_pagine_max"`
	OCRTempoMaxS       int `json:"ocr_tempo_max_s"`
	OCRDpi             int `json:"ocr_dpi"`
	// dalla sottoversione 3: i tetti delle tabelle
	TabellePagineMax int `json:"tabelle_pagine_max"`
	TabelleMax       int `json:"tabelle_max"`
	RigheMax         int `json:"righe_max"`
	CellaMax         int `json:"cella_max"`
}

// DecodificaTestoPDF estrae la lettura del testo dai dettagli di un'analisi.
//
// Il secondo valore è falso quando il testo non c'è (fatti di un analizzatore prima del 4, un worker vecchio
// che ha risposto a un job v4, uno STEP, un PDF che non si apre) o quando la versione è sconosciuta (< 1). Falso
// NON vuol dire «il PDF non ha testo»: quello è `Estraibile` falso in un testo decodificato. Un PDF senza
// `testo_pdf` è un PDF il cui testo non è stato letto, e va rianalizzato, non dichiarato muto (A5.13.3).
//
// Tollera i campi sconosciuti, come DecodificaStruttura.
func DecodificaTestoPDF(dettagli json.RawMessage) (*TestoPDF, bool) {
	if len(dettagli) == 0 {
		return nil, false
	}
	var involucro struct {
		Testo *TestoPDF `json:"testo_pdf"`
	}
	if err := json.Unmarshal(dettagli, &involucro); err != nil || involucro.Testo == nil {
		return nil, false
	}
	if involucro.Testo.Versione < 1 {
		return nil, false
	}
	return involucro.Testo, true
}

// ---------------------------------------------------------------- testo dei DXF (K4)

// Gli esiti della lettura di un DXF (TestoDXF.Esito).
const (
	DXFLetto    = "letto"     // il file si è letto: un cartiglio vuoto è una risposta
	DXFNonLetto = "non_letto" // un DXF binario, un file che non è un DXF ASCII, un errore di lettura (Motivo)
)

// VersioneTestoDXF è la versione della forma di `testo_dxf` che il worker di oggi scrive (TestoDXF.Versione, non la
// versione dell'analizzatore). Un DXF con i fatti correnti senza `testo_dxf`, o con una versione di prima, si
// riaccoda con «Rianalizza» (fascicolo.AccodaPdfDaRileggere, TestoDXFDaRileggere), come i PDF della sottoversione
// di prima: la versione dell'analizzatore non cambia.
//
// La 2 (GP-3, direttiva 21 §5.1) segue gli INSERT annidati ricorsivamente, con la trasformazione composta, la
// profondità massima e la guardia sui cicli, e segue anche gli INSERT ruotati (la regola del cartiglio nel sistema del
// blocco). Annidati sono da qui gli INSERT dentro un blocco NON seguiti; AnnidatiSeguiti, OltreProfondita, Cicli e
// Specchiati dicono il resto, e Parziale è vero quando qualcosa non è stato letto.
const VersioneTestoDXF = 2

// TestoDXF è la lettura del testo di un DXF ASCII, dentro `RisultatoAnalisi.dettagli["testo_dxf"]` (K4,
// workers/dxf_testo.py). Come TestoPDF sono FATTI, senza codici: solo i campi del cartiglio che il worker riconosce
// (CampoCartiglio, con Zona «pagina», Fonte «nativo» e il Riquadro del valore in unità del disegno, con la y verso
// l'alto), mai gli altri testi del disegno. Testi sono i testi risolti dopo gli INSERT, Annidati gli INSERT dentro un
// blocco che non si seguono. Motivo con Esito DXFNonLetto dice perché; con DXFLetto dice i testi non letti per una
// ragione che non è un tetto (gli INSERT ruotati, che il worker non segue), vuoto altrimenti.
type TestoDXF struct {
	Versione    int              `json:"versione"`
	Esito       string           `json:"esito"`
	Motivo      string           `json:"motivo"`
	VersioneDXF string           `json:"versione_dxf"`
	Codifica    string           `json:"codifica"`
	Testi       int              `json:"testi"`
	Annidati    int              `json:"annidati"`
	Troncato    bool             `json:"troncato"`
	Cartiglio   []CampoCartiglio `json:"cartiglio"`
	Limiti      LimitiTestoDXF   `json:"limiti"`
	// dalla v2 (GP-3): i conti degli INSERT e la parzialità; in una v1 sono zero e falso
	AnnidatiSeguiti int  `json:"annidati_seguiti"`
	OltreProfondita int  `json:"oltre_profondita"`
	Cicli           int  `json:"cicli"`
	Specchiati      int  `json:"specchiati"`
	Parziale        bool `json:"parziale"`
}

// LimitiTestoDXF: i tetti in vigore QUANDO il DXF è stato letto (come LimitiTestoPDF).
type LimitiTestoDXF struct {
	ByteMax  int `json:"byte_max"`
	TestiMax int `json:"testi_max"`
	TestoMax int `json:"testo_max"`
	CampiMax int `json:"campi_max"`
	// dalla v2: i livelli di INSERT seguiti e gli INSERT visitati al massimo
	ProfonditaMax int `json:"profondita_max"`
	InsertiMax    int `json:"inserti_max"`
}

// DecodificaTestoDXF estrae la lettura del testo di un DXF dai dettagli di un'analisi, come DecodificaTestoPDF: falso
// quando non c'è (un DXF analizzato prima del K4 ha i dettagli vuoti: il suo testo non è stato letto) o quando la
// versione è sconosciuta (< 1). Un DXF binario si decodifica: è una risposta (Esito DXFNonLetto). Tollera i campi
// sconosciuti.
func DecodificaTestoDXF(dettagli json.RawMessage) (*TestoDXF, bool) {
	if len(dettagli) == 0 {
		return nil, false
	}
	var involucro struct {
		Testo *TestoDXF `json:"testo_dxf"`
	}
	if err := json.Unmarshal(dettagli, &involucro); err != nil || involucro.Testo == nil {
		return nil, false
	}
	if involucro.Testo.Versione < 1 {
		return nil, false
	}
	return involucro.Testo, true
}

// TestoDXFDaRileggere dice se «Rianalizza» deve riaccodare un DXF con questi fatti dell'analizzatore corrente: il
// testo non c'è (un worker di prima del K4) o è stato LETTO con una versione di prima di VersioneTestoDXF. GP-3: un DXF
// non letto (binario, non ASCII) di una versione di prima no: la v2 lo legge allo stesso modo, e rileggerlo non
// cambia la risposta.
func TestoDXFDaRileggere(fatti json.RawMessage) bool {
	t, ok := DecodificaTestoDXF(fatti)
	return !ok || (t.Versione < VersioneTestoDXF && t.Esito == DXFLetto)
}

// ---------------------------------------------------------------- testo dei DWG R12 (GP-3, G-8.4)

// Gli esiti della lettura di un DWG (TestoDWG.Esito).
const (
	DWGLetto         = "letto"          // R11/R12 (AC1009): un cartiglio vuoto è una risposta
	DWGNonSupportato = "non_supportato" // un'altra versione DWG (VersioneDWG la dice): limite dichiarato
	DWGNonLetto      = "non_letto"      // non è un DWG, o la lettura è fallita (Motivo)
)

// VersioneTestoDWG è la versione della forma di `testo_dwg` che il worker di oggi scrive. Un DWG analizzato prima ha i
// dettagli vuoti: TestoDWGDaRileggere lo riaccoda.
const VersioneTestoDWG = 1

// TestoDWG è il testo di un DWG (workers/dwg_testo.py): come TestoDXF, solo i campi del cartiglio riconosciuti (con la
// sorgente nel file) e le righe delle tabelle riconosciute (la forma di TabellaPDF, riquadri in unità del disegno con
// la y verso l'alto, offset delle entità in CellaTabella.Sorgente), mai gli altri testi. NonDecodificate: entità di
// testo o INSERT la cui lunghezza non torna, mai indovinate; Parziale con i motivi.
type TestoDWG struct {
	Versione           int                 `json:"versione"`
	Esito              string              `json:"esito"`
	Motivo             string              `json:"motivo"`
	VersioneDWG        string              `json:"versione_dwg"`
	Codifica           string              `json:"codifica"`
	Entita             int                 `json:"entita"`
	Testi              int                 `json:"testi"`
	CancellateIgnorate int                 `json:"cancellate_ignorate"`
	NonDecodificate    int                 `json:"non_decodificate"`
	ConDatiEstesi      int                 `json:"con_dati_estesi"`
	Annidati           int                 `json:"annidati"`
	AnnidatiSeguiti    int                 `json:"annidati_seguiti"`
	OltreProfondita    int                 `json:"oltre_profondita"`
	Cicli              int                 `json:"cicli"`
	Specchiati         int                 `json:"specchiati"`
	Troncato           bool                `json:"troncato"`
	Parziale           bool                `json:"parziale"`
	Cartiglio          []CampoCartiglioDWG `json:"cartiglio"`
	Tabelle            []TabellaPDF        `json:"tabelle"`
	Limiti             LimitiTestoDWG      `json:"limiti"`
}

// CampoCartiglioDWG è un CampoCartiglio con la sorgente nel file DWG.
type CampoCartiglioDWG struct {
	CampoCartiglio
	Sorgente SorgenteDWG `json:"sorgente"`
}

// SorgenteDWG: l'offset dell'entità del valore (e di quella dell'etichetta) nel file, la sezione (ENTITIES, BLOCKS,
// EXTRA), il blocco che la contiene e il tipo d'entità (TEXT, ATTRIB). -1 = non noto.
type SorgenteDWG struct {
	OffsetEntita    int64  `json:"offset_entita"`
	Sezione         string `json:"sezione"`
	Blocco          string `json:"blocco"`
	TipoEntita      string `json:"tipo_entita"`
	OffsetEtichetta int64  `json:"offset_etichetta"`
}

// LimitiTestoDWG: i tetti in vigore quando il DWG è stato letto.
type LimitiTestoDWG struct {
	ByteMax       int `json:"byte_max"`
	EntitaMax     int `json:"entita_max"`
	TestiMax      int `json:"testi_max"`
	TestoMax      int `json:"testo_max"`
	CampiMax      int `json:"campi_max"`
	ProfonditaMax int `json:"profondita_max"`
	InsertiMax    int `json:"inserti_max"`
	RigheMax      int `json:"righe_max"`
}

// DecodificaTestoDWG estrae `testo_dwg` dai dettagli di un'analisi: falso quando manca (un DWG analizzato prima di GP-3
// ha i dettagli vuoti) o la versione è sconosciuta (< 1). Un DWG non supportato si decodifica: è una risposta.
func DecodificaTestoDWG(dettagli json.RawMessage) (*TestoDWG, bool) {
	if len(dettagli) == 0 {
		return nil, false
	}
	var involucro struct {
		Testo *TestoDWG `json:"testo_dwg"`
	}
	if err := json.Unmarshal(dettagli, &involucro); err != nil || involucro.Testo == nil || involucro.Testo.Versione < 1 {
		return nil, false
	}
	return involucro.Testo, true
}

// TestoDWGDaRileggere dice se «Rianalizza» deve riaccodare un DWG con questi fatti dell'analizzatore corrente: il testo
// non c'è (un DWG analizzato prima di GP-3) o è di una versione di prima. Un DWG non supportato della versione di oggi
// no: rileggerlo non cambia la risposta. L'idoneità per estensione (dwg) è del chiamante (fascicolo, da estendere:
// consegna al core).
func TestoDWGDaRileggere(fatti json.RawMessage) bool {
	t, ok := DecodificaTestoDWG(fatti)
	return !ok || t.Versione < VersioneTestoDWG
}

// ---------------------------------------------------------------- fogli di calcolo (GP-3, G-8.2)

// Gli esiti della lettura di un foglio (FoglioDati.Esito).
const (
	FoglioLetto         = "letto"
	FoglioParziale      = "parziale"       // un tetto raggiunto: Motivo, Troncato
	FoglioNonSupportato = "non_supportato" // BIFF di Excel 97-2003, HTML, cartella cifrata: limite dichiarato
	FoglioNonLetto      = "non_letto"      // il file non si legge (Motivo)
)

// VersioneFoglioDati e' la versione della forma di `foglio_dati` che il worker di oggi scrive. Un foglio (xls, xlsx,
// xlsm, csv) analizzato prima ha i dettagli vuoti: FoglioDatiDaRileggere lo riaccoda.
const VersioneFoglioDati = 1

// FoglioDati e' il fatto `foglio_dati` (workers/foglio_dati.py): le celle non vuote con il testo ESATTO del file, il
// tipo del valore memorizzato, la formula separata, lo stile che puo' portare uno stato (indice in Stili) e il
// collegamento ridotto a nome ed estensione. Nessuna interpretazione: «rosso = obsoleto» lo dice una regola del
// cliente. Sha256 e' la versione della cartella: due versioni dello stesso file sono due fatti.
type FoglioDati struct {
	Versione              int                  `json:"versione"`
	Esito                 string               `json:"esito"`
	Motivo                string               `json:"motivo"`
	Formato               string               `json:"formato"`
	Sha256                string               `json:"sha256"`
	Codifica              string               `json:"codifica"`
	Fogli                 []FoglioInfo         `json:"fogli"`
	Celle                 []CellaFoglio        `json:"celle"`
	Stili                 []StileCella         `json:"stili"`
	IntestazioniCandidate []IntestazioneFoglio `json:"intestazioni_candidate"`
	MacroPresenti         bool                 `json:"macro_presenti"`
	CollegamentiEsterni   int                  `json:"collegamenti_esterni"`
	Troncato              bool                 `json:"troncato"`
	Limiti                LimitiFoglio         `json:"limiti"`
}

// FoglioInfo: un foglio della cartella (posizione, nome, visibilita', dimensione dichiarata, celle lette, unite).
type FoglioInfo struct {
	Indice     int      `json:"indice"`
	Nome       string   `json:"nome"`
	Stato      string   `json:"stato"` // visibile | nascosto | molto_nascosto
	Dimensione string   `json:"dimensione"`
	Celle      int      `json:"celle"`
	Unite      []string `json:"unite"`
	Troncato   bool     `json:"troncato"`
}

// CellaFoglio: una cella non vuota. Tipo del valore memorizzato: s, n, b, d, e. Grezzo e' il testo come sta nel file
// (zeri iniziali e spazi compresi; il numero come e' scritto). DaFormula con Formula (vuota per una formula condivisa
// che non la ripete): Grezzo e' il valore memorizzato, non ricalcolato. Stile e' l'indice in FoglioDati.Stili (nil:
// predefinito).
type CellaFoglio struct {
	Foglio       int                `json:"foglio"`
	Ind          string             `json:"ind"`
	R            int                `json:"r"`
	C            int                `json:"c"`
	Tipo         string             `json:"tipo"`
	Grezzo       string             `json:"grezzo"`
	Formula      string             `json:"formula"`
	DaFormula    bool               `json:"da_formula"`
	Stile        *int               `json:"stile"`
	Collegamento *CollegamentoCella `json:"collegamento"`
}

// StileCella: lo stile che puo' portare uno stato. ColoreFont e Riempimento: ARGB («FFFF0000») o «theme:N[:tinta]» o
// «indexed:N»; Apice: l'apostrofo iniziale (quotePrefix); FormatoNumero: il codice del formato, vuoto per il generale.
type StileCella struct {
	ColoreFont    string `json:"colore_font"`
	Grassetto     bool   `json:"grassetto"`
	Corsivo       bool   `json:"corsivo"`
	Barrato       bool   `json:"barrato"`
	Sottolineato  bool   `json:"sottolineato"`
	Riempimento   string `json:"riempimento"`
	Apice         bool   `json:"apice"`
	FormatoNumero string `json:"formato_numero"`
}

// CollegamentoCella: il tipo (relativo, assoluto, url, interno, posta) e il nome del file con l'estensione; mai
// cartelle, host o indirizzi, e mai seguito.
type CollegamentoCella struct {
	Tipo       string `json:"tipo"`
	NomeBase   string `json:"nome_base"`
	Estensione string `json:"estensione"`
}

// IntestazioneFoglio: una riga candidata intestazione (nomi di colonna con un ruolo tipico): un aiuto di lettura.
type IntestazioneFoglio struct {
	Foglio  int             `json:"foglio"`
	Riga    int             `json:"riga"`
	Colonne []ColonnaFoglio `json:"colonne"`
}

// ColonnaFoglio: una cella dell'intestazione candidata, con il ruolo tipico (Ruolo…).
type ColonnaFoglio struct {
	C     int    `json:"c"`
	Ind   string `json:"ind"`
	Testo string `json:"testo"`
	Ruolo string `json:"ruolo"`
}

// LimitiFoglio: i tetti in vigore quando il foglio e' stato letto.
type LimitiFoglio struct {
	ByteMax     int `json:"byte_max"`
	ParteMax    int `json:"parte_max"`
	RapportoMax int `json:"rapporto_max"`
	FogliMax    int `json:"fogli_max"`
	CelleMax    int `json:"celle_max"`
	TestoMax    int `json:"testo_max"`
	UniteMax    int `json:"unite_max"`
}

// DecodificaFoglioDati estrae il fatto `foglio_dati` dai dettagli di un'analisi: falso quando manca (un foglio
// analizzato prima di GP-3 ha i dettagli vuoti) o la versione e' sconosciuta (< 1). Un BIFF non supportato si
// decodifica: e' una risposta. Tollera i campi sconosciuti.
func DecodificaFoglioDati(dettagli json.RawMessage) (*FoglioDati, bool) {
	if len(dettagli) == 0 {
		return nil, false
	}
	var involucro struct {
		Foglio *FoglioDati `json:"foglio_dati"`
	}
	if err := json.Unmarshal(dettagli, &involucro); err != nil || involucro.Foglio == nil || involucro.Foglio.Versione < 1 {
		return nil, false
	}
	return involucro.Foglio, true
}

// FoglioDatiDaRileggere dice se «Rianalizza» deve riaccodare un foglio (xls, xlsx, xlsm, csv) con questi fatti
// dell'analizzatore corrente: il fatto manca o e' di una versione di prima di VersioneFoglioDati. Un foglio non
// supportato (BIFF) no: e' una risposta, che una rilettura non cambia. L'idoneita' per estensione e' del chiamante
// (fascicolo.AccodaPdfDaRileggere, da estendere: consegna al core).
func FoglioDatiDaRileggere(fatti json.RawMessage) bool {
	f, ok := DecodificaFoglioDati(fatti)
	return !ok || f.Versione < VersioneFoglioDati
}

// ---------------------------------------------------------------- healthz

type Salute struct {
	DB       string `json:"db"`
	NAS      string `json:"nas"`
	Versione int    `json:"schema_versione"`
}
