package ancoraggio

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// I tipi della fonte strutturale di un prodotto (contratto §1.2, §2.2; M1, R57, R65, R76, R85). Qui ci sono solo i
// tipi, che servono agli ingressi e alle uscite delle proposte: lo stato della fonte di un prodotto, il suo motivo e
// la precedenza fra la vista v_step_prodotto e il motore A li calcola valutazione, che legge la fotografia
// (T-B0-04: ancoraggio non importa fotorfq). Gli ancoraggi che li usano arrivano con i commit di B2 e B4.

// RiferimentoFonte: la fonte STEP confermata di un prodotto, il gesto 3 («Autorizza», DichiaraStrutturale): vale per
// quel documento, quella versione (lo sha256) e quella radice (contratto §1.2). È l'ingresso che dice quale
// struttura è la BOM di lavoro proposta: solo quella sotto la radice scelta (R76 A).
//   - Tipo: sempre «step»: in A1c un PDF non è mai fonte (R68 A), e solo uno STEP lo è (T-E1-09).
//   - Radice: la chiave STEP del nodo radice scelto, dalla riga del nodo sorgente della marcatura; "" vuol dire
//     «non registrata» (T-B0-08), mai scelta dal sistema.
//   - Ruolo: il ruolo della marcatura (radice, raggruppamento, delega, come li scrive il gesto); "" senza
//     marcatura.
//   - Forma: smistamento, la marcatura evidenza.strutturale con chi e quando; step_strutturale_id, la sola colonna
//     del finito, senza chi né quando (T-B0-08).
//   - ConfermatoIl: il testo com'è nel JSON della marcatura (T-03); "" se non registrato.
//   - Superato: il documento ha un sostituto (documento.sostituito_da): la fonte resta, superata.
type RiferimentoFonte struct {
	Tipo         string     `json:"tipo"`
	DocumentoID  uuid.UUID  `json:"documento_id"`
	Sha256       string     `json:"sha256"`
	AllegatoID   *uuid.UUID `json:"allegato_id,omitempty"`
	Radice       string     `json:"radice,omitempty"`
	Ruolo        string     `json:"ruolo,omitempty"`
	Forma        string     `json:"forma"`
	ConfermatoDa *uuid.UUID `json:"confermato_da,omitempty"`
	ConfermatoIl string     `json:"confermato_il,omitempty"`
	Superato     bool       `json:"superato"`
}

// I valori di RiferimentoFonte.Tipo e RiferimentoFonte.Forma (contratto §2.2).
const (
	TipoRiferimentoStep = "step"

	FormaRiferimentoSmistamento     = "smistamento"
	FormaRiferimentoStepStrutturale = "step_strutturale_id"
)

// EsitoEstrazione: l'estrazione dei fatti di un documento candidato o del riferimento (contratto §2.2; T-B0-07).
//   - riuscita: i fatti alla terna corrente, con la struttura;
//   - fallita: i fatti in errore. È diversa da «dati insufficienti», che è un grafo incompleto (GrafoCompleto);
//   - in_corso: un lavoro di analisi pendente;
//   - non_analizzata: nessun fatto e nessun lavoro pendente.
type EsitoEstrazione string

const (
	EstrazioneRiuscita      EsitoEstrazione = "riuscita"
	EstrazioneFallita       EsitoEstrazione = "fallita"
	EstrazioneInCorso       EsitoEstrazione = "in_corso"
	EstrazioneNonAnalizzata EsitoEstrazione = "non_analizzata"
)

// OrigineCandidato: da dove viene un documento candidato della fonte (contratto §1.2):
//   - documento_del_prodotto: uno STEP corrente sul prodotto non ancora scelto (v_step_prodotto.n_step_correnti);
//   - motore_a: uno STEP fra gli allegati del thread la cui radice ha la base del prodotto (R76 b A);
//   - proposta_aperta: una proposta aperta di 3D.
type OrigineCandidato string

const (
	CandidatoDaDocumentoDelProdotto OrigineCandidato = "documento_del_prodotto"
	CandidatoDaMotoreA              OrigineCandidato = "motore_a"
	CandidatoDaPropostaAperta       OrigineCandidato = "proposta_aperta"
)

// DocumentoCandidato: il singolo file candidato come fonte (M1). Non è uno stato della fonte: è una proposta, e non
// conferma niente. Un candidato del motore A porta la fonte a «in attesa di conferma» solo da «assente», mai da
// «confermata» (T-B0-07); lo decide valutazione.
//   - AllegatoID, DocumentoID: l'allegato e il documento, quando ci sono (nil = assente).
//   - Radici: le chiavi STEP delle radici del file. RadiceCompatibile: la radice con la base del prodotto; "" se
//     nessuna. Candidato è solo un file la cui radice ha la base del prodotto: un prodotto che compare solo come
//     nodo interno non fa del file una fonte (R76 b A).
//   - Compatibilita: il confronto della base della radice con quella del prodotto (motorea.ConfrontaBasi).
//   - Estrazione: l'esito dell'estrazione dei suoi fatti. GrafoCompleto e MotivoGrafo: la completezza del grafo
//     STEP, con il motivo della 0020 quando non è completo («dati insufficienti»).
type DocumentoCandidato struct {
	Origine           OrigineCandidato      `json:"origine"`
	AllegatoID        *uuid.UUID            `json:"allegato_id,omitempty"`
	DocumentoID       *uuid.UUID            `json:"documento_id,omitempty"`
	Sha256            string                `json:"sha256,omitempty"`
	Radici            []string              `json:"radici,omitempty"`
	RadiceCompatibile string                `json:"radice_compatibile,omitempty"`
	Compatibilita     motorea.Compatibilita `json:"compatibilita,omitempty"`
	Estrazione        EsitoEstrazione       `json:"estrazione"`
	GrafoCompleto     bool                  `json:"grafo_completo"`
	MotivoGrafo       string                `json:"motivo_grafo,omitempty"`
}
