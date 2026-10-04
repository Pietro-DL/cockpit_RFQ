// Package fotorfq contiene i dati di una o più RFQ come li ha letti il caricatore, in una transazione, o il
// lettore degli export (piano A, par.3.3.6; nome e sede -> R40 d). I tipi sono senza database: si riempiono
// da fuori e si leggono soltanto. Niente DB, file né orologio.
//
// In A1b ci sono solo i record che leggono gli adattatori di core/estrazione: Messaggio, Allegato, Terna,
// Fatti, più ImprontaPayload, l'unico modo di calcolare Fatti.Digest (5.4.2). La fotografia intera, il
// caricatore e l'apertura in lettura arrivano in A1c (par.6).
//
// I record dicono che cosa c'è nel DB, non che cosa significa: nessuna interpretazione, nessuna decisione
// dentro un record di messaggio o di allegato (I-2, par.3.8.3).
package fotorfq

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// VersioneSchema: la versione della struttura dei record della fotografia (par.3.4.1). Si cambia solo con un
// commit che lo dichiara, e la prova che la fissa si riscrive con «Riscritta per …».
const VersioneSchema = 1

// Messaggio: le colonne del messaggio. Canale "" vuol dire «non esportato». Oggetto, CorpoTesto e CorpoHTML a
// nil vogliono dire «assente nel DB»; negli export l'assenza la dicono le sezioni della fotografia (A1c).
// CorpoTesto è il corpo_testo come sta nel DB: gli offset degli adattatori si contano su questi byte.
type Messaggio struct {
	ID                   uuid.UUID  `json:"id"`
	ConversazioneID      uuid.UUID  `json:"conversazione_id"`
	ThreadID             *uuid.UUID `json:"thread_id,omitempty"`
	ParentID             *uuid.UUID `json:"parent_id,omitempty"`
	Canale               string     `json:"canale,omitempty"`
	Direzione            string     `json:"direzione,omitempty"`
	DataEvento           time.Time  `json:"data_evento"` // UTC, al millisecondo: export e DB si confrontano così
	MittenteNome         string     `json:"mittente_nome,omitempty"`
	MittenteIndirizzo    string     `json:"mittente_indirizzo,omitempty"`
	Oggetto              *string    `json:"oggetto,omitempty"`
	CorpoTesto           *string    `json:"corpo_testo,omitempty"`
	CorpoHTML            *string    `json:"corpo_html,omitempty"`
	ControparteTipo      string     `json:"controparte_tipo,omitempty"`
	ControparteClienteID *uuid.UUID `json:"controparte_cliente_id,omitempty"`
	Interno              bool       `json:"interno,omitempty"`
}

// Allegato: le colonne dell'allegato. NomeFile è com'è nel DB, già troncato e ripulito dall'acquisizione:
// l'adattatore lo dichiara e non lo ricostruisce. ContenitoreID è lo zip (o il messaggio) da cui la voce
// viene; PathInterno è il percorso della voce dentro l'archivio.
type Allegato struct {
	ID            uuid.UUID  `json:"id"`
	MessaggioID   uuid.UUID  `json:"messaggio_id"`
	ContenitoreID *uuid.UUID `json:"contenitore_id,omitempty"`
	Indice        int16      `json:"indice"`
	NomeFile      string     `json:"nome_file"`
	PathInterno   *string    `json:"path_interno,omitempty"`
	Estensione    *string    `json:"estensione,omitempty"`
	ContentType   *string    `json:"content_type,omitempty"`
	Natura        string     `json:"natura,omitempty"`
	Origine       string     `json:"origine,omitempty"`
	Stato         string     `json:"stato,omitempty"`
	Bytes         *int64     `json:"bytes,omitempty"`
	Sha256        *string    `json:"sha256,omitempty"`
	RicevutoIl    time.Time  `json:"ricevuto_il"`
}

// Terna: la chiave di analisi_fatti, cioè versione dell'analizzatore e hash della configurazione (lo sha256
// del contenuto sta nei Fatti).
type Terna struct {
	Versione           int16  `json:"versione"`
	HashConfigurazione string `json:"hash_configurazione"`
}

// Fatti: i fatti di un contenuto alla terna data, come li ha scritti il worker.
//
// Digest si calcola sempre con ImprontaPayload, da chiunque riempia un Fatti: il caricatore, il lettore degli
// export, le prove. MotivoParziale viene da struttura_motivo_parziale: "" = completa; nil = non
// determinabile, per esempio da un export (-> R32 b: è una qualità della fonte, non del grafo interpretato).
type Fatti struct {
	Sha256         string          `json:"sha256"`
	Terna          Terna           `json:"terna"`
	CalcolatoIl    time.Time       `json:"calcolato_il"`
	Payload        json.RawMessage `json:"payload"` // i fatti come li ha dati il DB, o la stringa dell'export
	Digest         string          `json:"digest,omitempty"`
	MotivoParziale *string         `json:"motivo_parziale,omitempty"`
}

// ImprontaPayload: lo sha256 del JSON canonico dei fatti (jsoncanonico), con i numeri copiati come testo. Non
// dipende dall'ordine delle chiavi, dagli spazi né dalla forma dell'escape delle stringhe («\u00e8» ed «è»
// sono lo stesso testo); «800» e «800.0» restano due letterali diversi, quindi due impronte. È il valore di
// Fatti.Digest: chi riempie un Fatti lo calcola qui, sempre nello stesso modo (par.3.4.2), così i byte del
// jsonb e la stringa di un export danno la stessa impronta [+3, 5.4.2: il par.3.3.6 descrive il campo ma non
// dice chi lo calcola].
//
// Un payload vuoto o che non è un JSON valido per jsoncanonico (chiavi ripetute, UTF-8 non valido, surrogati
// soli, testo dopo il valore) è un errore: un'impronta non si inventa.
func ImprontaPayload(payload json.RawMessage) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("fotorfq: payload vuoto: i fatti non hanno un'impronta")
	}
	h, err := jsoncanonico.ImprontaDi(payload)
	if err != nil {
		return "", fmt.Errorf("fotorfq: impronta del payload: %w", err)
	}
	return h, nil
}
