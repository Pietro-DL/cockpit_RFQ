package valutazione

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// esito.go: l'esito di Calcola (piano A, par.3.3.8 e 6.4.6; contratto §2.3, «Campi nuovi di EsitoThread»; fase 0 di B6,
// F.2, con le decisioni F0-02…F0-05 e F0-10). È il DTO che il banco legge e, da A1d, l'anteprima: tag JSON snake_case,
// nessuna mappa, ogni elenco in un ordine che non dipende dagli ingressi. Il contenitore interno di B1–B5
// (ValutazioneProdotti) non è un DTO: Calcola lo consuma e ne compone i campi qui (T-B6-10).

// VersioneValutazione: la versione del percorso di valutazione (6.4.6). Entra in Esito, quindi nella sua impronta:
// cambia con un commit che lo dichiara, e la prova che la fissa si riscrive.
//   - valutazione-2 (P7e, 08/10): gli emendamenti EB7 ratificati dall'utente l'08/10 aggiungono campi all'esito, e quindi
//     ne cambiano il canonico e l'impronta: le strutture candidate e i confermati senza STEP (EB7-1 B), il tipo del nodo
//     con l'origine (EB7-2 A), il documento e lo stato del NAS nella voce del 2D (EB7-4 A). Stato, assi, motivi,
//     autorità e impronta del prodotto (R90) non cambiano.
const VersioneValutazione = "valutazione-2"

// Esito: la valutazione di una fotografia (par.3.3.8; contratto §2.3; IM.1).
//   - Le cinque versioni, l'impronta della fotografia, quella dell'indice delle regole (che copre anche i limiti: R43 B)
//     e la versione dei limiti: con loro l'impronta dice con quale motore e quali regole l'esito è stato calcolato.
//     VersioneRevisioneRegistrata (motorea, R113 B; E2 §2.6) è la versione della lettura della colonna rev del vecchio,
//     come VersioneComposizione per il compositore.
//   - Thread: in ordine di ThreadID. FuoriRFQ: i messaggi senza thread dei casi di censimento (R34), in ordine di
//     (Caso, MessaggioID).
//   - Diagnostiche: quelle fuori da un thread (le diagnostiche di ValidaFotografia sotto la gravità «errore», i messaggi
//     fuori RFQ senza un caso), in ordine di (codice, percorso, riferimenti).
//   - Impronta: lo sha256 del canonico dell'esito con Impronta vuota (improntaEsito). Nessun orario dentro.
type Esito struct {
	VersioneValutazione         string                 `json:"versione_valutazione"`
	VersioneImprontaProdotto    int                    `json:"versione_impronta_prodotto"`
	VersioneFormati2D           int                    `json:"versione_formati_2d"`
	VersioneComposizione        string                 `json:"versione_composizione"`
	VersioneRevisioneRegistrata string                 `json:"versione_revisione_registrata"`
	ImprontaFotografia          string                 `json:"impronta_fotografia"`
	ImprontaIndice              string                 `json:"impronta_indice"`
	VersioneLimiti              string                 `json:"versione_limiti"`
	Thread                      []EsitoThread          `json:"thread,omitempty"`
	FuoriRFQ                    []EsitoFuoriRFQ        `json:"fuori_rfq,omitempty"`
	Diagnostiche                []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
	Impronta                    string                 `json:"impronta"`
}

// EsitoThread: la valutazione di un thread (par.3.3.8; contratto §2.3; fase 0, F.2).
//   - Valutato, Motivo: un thread senza grammatica A, con la grammatica scartata, con la ragione sociale discorde o con
//     un errore della valutazione non è valutato (6.4.6 passo 2; T-B6-09). Senza grammatica i prodotti si calcolano lo
//     stesso, come informazione, con lo stato non_pronto; con un errore non c'è niente del nuovo.
//   - HashSnapshot: lo snapshot della grammatica usata; "" senza motore.
//   - Richiesta, Prodotti, Ancoraggi: quelli di B1–B5 (la richiesta con i gesti, i candidati della mail, le strutture e
//     gli ancoraggi dei file con le loro diagnostiche e la loro impronta: F0-04).
//   - File: un record per allegato, in ordine di AllegatoID, con il documento, l'interpretazione e la disponibilità; un
//     allegato che l'adattatore lascia fuori ha il suo record con il motivo (F0-03).
//   - VecchiProdotti: il vecchio dei prodotti, «identificativo:<codice>» per ogni identificativo confermato e
//     «candidato:<codice>» per ogni candidato di codice della fotografia (che non porta mai quelli dell'agente), in
//     ordine, senza doppioni (F0-18).
//   - Evidenze: per allegato, da dove viene il nuovo (le letture d'identità del file), da mostrare (R12).
//   - Confrontabili: il vecchio e il nuovo di ogni allegato, piatti, l'ingresso di confronto (R53 B); uno per allegato,
//     anche per i contenitori e per i thread non valutati, in ordine di AllegatoID.
//   - ProdottiConfrontabili: i candidati prodotto della mail, piatti, nell'ordine dei candidati.
//   - ProdottiValutati, Associazioni, DaSmistare, Conflitti, Fascicolo: i campi del contratto (§2.3). I conflitti di B5
//     sono composti con il prodotto e senza doppioni (componiConflitti); associazioni, file da smistare, fascicolo e i
//     campi di B6 dei prodotti li calcolano le fasi V2 e V3, e fino ad allora restano al valore prudente (F.3).
//   - Diagnostiche: quelle di valutazione per il thread (il motore della grammatica, B1–B5, il vecchio, l'errore della
//     valutazione), in ordine di (codice, percorso, riferimenti). Quelle di ancoraggio restano in Ancoraggi (F0-04).
type EsitoThread struct {
	ThreadID              uuid.UUID                    `json:"thread_id"`
	ClienteID             uuid.UUID                    `json:"cliente_id"`
	Valutato              bool                         `json:"valutato"`
	Motivo                MotivoThread                 `json:"motivo,omitempty"`
	HashSnapshot          string                       `json:"hash_snapshot,omitempty"`
	Richiesta             ancoraggio.RichiestaValutata `json:"richiesta"`
	File                  []FileInterpretato           `json:"file,omitempty"`
	Prodotti              ancoraggio.EsitoProdotti     `json:"prodotti"`
	Ancoraggi             ancoraggio.EsitoAncoraggi    `json:"ancoraggi"`
	VecchiProdotti        []string                     `json:"vecchi_prodotti,omitempty"`
	Evidenze              []EvidenzaFile               `json:"evidenze,omitempty"`
	Confrontabili         []FileConfrontabile          `json:"confrontabili,omitempty"`
	ProdottiConfrontabili []ProdottoConfrontabile      `json:"prodotti_confrontabili,omitempty"`
	ProdottiValutati      []ProdottoValutato           `json:"prodotti_valutati,omitempty"`
	Associazioni          []AssociazioneFile           `json:"associazioni,omitempty"`
	DaSmistare            []FileDaSmistare             `json:"da_smistare,omitempty"`
	Conflitti             []Conflitto                  `json:"conflitti,omitempty"`
	Fascicolo             StatoFascicolo               `json:"fascicolo"`
	Diagnostiche          []evidenze.Diagnostica       `json:"diagnostiche,omitempty"`
}

// MotivoThread: perché un thread, o un messaggio fuori RFQ, non è valutato (6.4.6 passo 2; T-B6-09; F0-05).
//   - senza_grammatica_a: nessuna voce per il cliente nell'indice delle regole (regole.assenti), o nessun insieme di
//     regole (A1c-L4D-11: «non valutato: nessuna regola A»);
//   - grammatica_scartata: il cliente è fra gli scartati dell'insieme delle regole;
//   - ragione_sociale_discorde: la ragione sociale della grammatica non è quella del DB (regole.ragione_sociale_discorde);
//   - errore_valutazione: con la grammatica, la valutazione del thread dà un errore (valutazione.errore_valutazione, o le
//     diagnostiche dell'errore di contratto che l'errore porta).
type MotivoThread string

const (
	MotivoThreadSenzaGrammatica        MotivoThread = "senza_grammatica_a"
	MotivoThreadGrammaticaScartata     MotivoThread = "grammatica_scartata"
	MotivoThreadRagioneSocialeDiscorde MotivoThread = "ragione_sociale_discorde"
	MotivoThreadErroreValutazione      MotivoThread = "errore_valutazione"
)

// EsitoFuoriRFQ: un messaggio senza thread di un caso di censimento (R34; 6.4.6, «FuoriRFQ»): il caso, il messaggio e il
// cliente del caso, valutato con la grammatica di quel cliente come un thread (con gli stessi motivi), con il documento e
// l'interpretazione del messaggio, i suoi file e i prodotti che la mail chiede. Nessun target, nessun ancoraggio: il
// messaggio non ha una RFQ.
type EsitoFuoriRFQ struct {
	Caso         string                   `json:"caso"`
	MessaggioID  uuid.UUID                `json:"messaggio_id"`
	ClienteID    uuid.UUID                `json:"cliente_id"`
	Valutato     bool                     `json:"valutato"`
	Motivo       MotivoThread             `json:"motivo,omitempty"`
	HashSnapshot string                   `json:"hash_snapshot,omitempty"`
	Messaggio    MessaggioInterpretato    `json:"messaggio"`
	File         []FileInterpretato       `json:"file,omitempty"`
	Prodotti     ancoraggio.EsitoProdotti `json:"prodotti"`
	Diagnostiche []evidenze.Diagnostica   `json:"diagnostiche,omitempty"`
}

// EvidenzaFile: un'evidenza del nuovo per un allegato (par.3.3.8; 6.4.6): l'unità, la lettura, il testo dell'unità,
// l'intervallo in byte su quel testo, il selettore e il localizzatore. Sono i campi che il riquadro di A1d chiede (7.2).
type EvidenzaFile struct {
	AllegatoID uuid.UUID              `json:"allegato_id"`
	UnitaID    string                 `json:"unita_id"`
	LetturaID  string                 `json:"lettura_id"`
	Testo      string                 `json:"testo"`
	Intervallo evidenze.Intervallo    `json:"intervallo"`
	Selettore  evidenze.Selettore     `json:"selettore"`
	Posizione  evidenze.Localizzatore `json:"posizione"`
}

// FileInterpretato: l'involucro di ancoraggio.FileInterpretato per l'esito (F0-03): gli stessi campi, con i tag
// snake_case, perché ancoraggio è chiuso e il suo tipo non ha tag. In più il motivo di un allegato che l'adattatore
// lascia fuori (Motivo*): allora il documento e l'interpretazione sono vuoti. Un file di un thread senza grammatica ha il
// documento e un'interpretazione vuota del documento (solo il bundle).
type FileInterpretato struct {
	AllegatoID      uuid.UUID                  `json:"allegato_id"`
	Documento       evidenze.DocumentoEvidenze `json:"documento"`
	Interpretazione motorea.Interpretazione    `json:"interpretazione"`
	Disponibilita   ancoraggio.Disponibilita   `json:"disponibilita,omitempty"`
	Disegno         bool                       `json:"disegno"`
	Motivo          string                     `json:"motivo,omitempty"`
}

// MessaggioInterpretato: l'involucro di ancoraggio.MessaggioInterpretato per l'esito (F0-03), con i tag snake_case.
type MessaggioInterpretato struct {
	MessaggioID     uuid.UUID                  `json:"messaggio_id"`
	Documento       evidenze.DocumentoEvidenze `json:"documento"`
	Interpretazione motorea.Interpretazione    `json:"interpretazione"`
}

// I motivi di un file che non è valutato (FileInterpretato.Motivo, NuovoPiatto.Motivo; fase 0, CP.2 [T]). Un file lo
// è quando l'adattatore lo legge e il suo thread è valutato.
//   - contenitore: un allegato da cui vengono altre voci (uno zip, un messaggio); le voci sono file a sé;
//   - natura_non_file: un allegato che non è un file (inline, elemento di Outlook, collegamento);
//   - documento_non_leggibile: l'adattatore o l'interpretazione danno un errore su quel file (6.4.6 passo 3);
//   - thread_non_valutato: il thread non è valutato (solo NuovoPiatto: il record di FileInterpretato dice del file).
const (
	MotivoFileContenitore           = "contenitore"
	MotivoFileNaturaNonFile         = "natura_non_file"
	MotivoFileDocumentoNonLeggibile = "documento_non_leggibile"
	MotivoFileThreadNonValutato     = "thread_non_valutato"
)
