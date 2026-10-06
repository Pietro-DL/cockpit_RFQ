package ancoraggio

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// Le decisioni accanto alle proposte (contratto §1.4, §2.2, §2.6; R95 A, T-E1-05, T-E1R-08; commit P6b, B4, fase 1).
// Una decisione resta il valore corrente anche quando una proposta la contraddice: qui c'è il segnale della
// discordanza, il Conflitto composto lo fanno B5 e B6 in valutazione.

// DecisioneIdentita: una decisione del modello nuovo su codice e revisione di un componente o di un documento (E1R
// §5.3; contratto §2.6; T-E1R-08). È un ingresso astratto, come GestiVerificaBOM: in A1c nessun adattatore la produce,
// perché il DB non ha una decisione tracciata sull'identità (LD-27); la regola si prova con ingressi sintetici.
//   - Oggetto: componente o documento. Qui si usa la parte componente: la decisione riempie CodiceDeciso con
//     RevProvenienza = decisione_tracciata e dà IdentitaNodo.StatoRevisione = confermata; la parte documento la usa
//     B5 in valutazione (Disegno2D.Confermata, CodiceConfermato).
//   - ID: il componente o il documento. Codice, Revisione: i valori decisi. Il codice c'è sempre (vuoto: errore di
//     contratto); la revisione vuota vuol dire «decisa senza revisione» (T-B4-21).
//   - EvidenzeViste: le evidenze che l'operatore aveva davanti, coppie fonte-valore. Un'evidenza è nuova se la sua
//     coppia non c'era (EvidenzaNuova): una correzione fatta contro il cartiglio non diventa un conflitto permanente,
//     lo diventa solo se dopo arriva un'evidenza diversa (E1R §5.3).
//   - Da, Il: chi e quando: una decisione è sempre un gesto tracciato (R29 e).
type DecisioneIdentita struct {
	Oggetto       string          `json:"oggetto"`
	ID            uuid.UUID       `json:"id"`
	Codice        string          `json:"codice"`
	Revisione     string          `json:"revisione"`
	EvidenzeViste []EvidenzaVista `json:"evidenze_viste,omitempty"`
	Da            uuid.UUID       `json:"da"`
	Il            time.Time       `json:"il"`
}

// I valori di DecisioneIdentita.Oggetto (contratto §2.6).
const (
	OggettoDecisioneComponente = "componente"
	OggettoDecisioneDocumento  = "documento"
)

// EvidenzaVista: un'evidenza che l'operatore aveva davanti quando ha deciso, una coppia fonte-valore (contratto §2.6).
// La coppia si costruisce sempre con EvidenzaDa, dal testo grezzo della fonte (T-B4-22).
type EvidenzaVista struct {
	Fonte  string `json:"fonte"`
	Valore string `json:"valore"`
}

// I valori di EvidenzaVista.Fonte (contratto §2.6). Una fonte fuori elenco è un errore di contratto
// (documento.enum_ignoto).
const (
	FonteEvidenzaCartiglio  = "cartiglio"
	FonteEvidenzaStepEntita = "step_entita"
	FonteEvidenzaNomeFile   = "nome_file"
	FonteEvidenzaDocumento  = "documento"
)

// fontiEvidenza: l'elenco chiuso delle fonti di un'evidenza vista.
var fontiEvidenza = map[string]bool{FonteEvidenzaCartiglio: true, FonteEvidenzaStepEntita: true, FonteEvidenzaNomeFile: true, FonteEvidenzaDocumento: true}

// EvidenzaDa: la coppia (fonte, valore) di un'evidenza, l'unica forma con cui si scrive in EvidenzeViste e si confronta
// (T-B4-22): il valore è il testo grezzo della fonte, senza gli spazi ai bordi, mai normalizzato altrimenti.
//   - cartiglio: il testo originale di cartiglio.codice del 2D;
//   - step_entita: il grezzo del nodo dell'entità, il suo id (ValoreGrezzo.Testo);
//   - nome_file: il nome del file, com'è;
//   - documento: il codice e la revisione registrati del documento, nella forma «codice» o «codice rev» (due parti).
//
// Le parti vuote non entrano; più parti si uniscono con uno spazio. La usano EvidenzaNuova, la riconciliazione (fase 3
// di B4) e B5, e chi produrrà le decisioni.
func EvidenzaDa(fonte string, parti ...string) EvidenzaVista {
	var valori []string
	for _, p := range parti {
		if p = strings.TrimSpace(p); p != "" {
			valori = append(valori, p)
		}
	}
	return EvidenzaVista{Fonte: fonte, Valore: strings.Join(valori, " ")}
}

// EvidenzaNuova: l'evidenza non era fra quelle viste dalla decisione, cioè la sua coppia fonte-valore non c'era (E1R
// §5.3; T-E1R-08). Le due parti si riportano alla forma di EvidenzaDa (senza gli spazi ai bordi: T-B4-22) e poi si
// confrontano per uguaglianza. Serve alla riconciliazione (fase 3 di B4) e a B5: contro una decisione su un componente
// un'evidenza nuova e contraria è un conflitto di nomenclatura (R95 A, T-E1-20), contro una decisione su un documento
// un conflitto identita_documento; un'evidenza già vista non lo è mai.
func EvidenzaNuova(d DecisioneIdentita, e EvidenzaVista) bool {
	nuova := EvidenzaDa(e.Fonte, e.Valore)
	for _, v := range d.EvidenzeViste {
		if EvidenzaDa(v.Fonte, v.Valore) == nuova {
			return false
		}
	}
	return true
}

// AbbinamentoPerBase: la seconda chiave della decisione del nodo (emendamento E1 §4.2, riga «decisione del nodo»,
// punto 2; workflow, passo 6: «ogni nodo che ha la base di un componente confermato mostra la decisione accanto alla
// proposta, senza fonderle»). Il contratto non ha il campo: lo aggiunge B4 (lettura dell'orchestratore T-B4-12).
// Due parti, una per chiave: la proposta per base (Componenti, Motivo) e il controllo della decisione per UUID
// (Decisione, MotivoDecisione, Compatibilita), che possono stare insieme (T-B4-26).
//   - La decisione per UUID è nel contesto (NodoProposto.Decisione): resta il valore corrente (R95 A), e la proposta per
//     base non c'è. Se la base del componente non è più compatibile con l'identità del nodo (una base discordante, un
//     altro marcatore scritto: la regola unica del marcatore, T-B4-30), decisione_non_compatibile, con la compatibilità;
//     se non si può dire (il codice del componente non è letto, è letto in un altro namespace, o il nodo non ha
//     un'identità), decisione_non_verificabile. Con la decisione compatibile, nil.
//   - La riga decisa del nodo porta un componente che non è fra i confermati del contesto (per esempio archiviato), quindi
//     nessuna Decisione (T-B4-26): decisione_non_verificabile con il componente della riga; accanto, se ci sono, i
//     componenti con la stessa identità, con il motivo distinto abbinamento_accanto_alla_riga_decisa: la riga ha già una
//     decisione, e la proposta non le passa sopra.
//   - Il nodo è scartato (la riga decisa è scartata, T-B4-27): niente proposta per base, motivo nodo_scartato.
//   - Altrimenti, senza una decisione: i componenti confermati della RFQ con la stessa identità del nodo (stesso
//     namespace, base completa uguale, marcatore compatibile: confrontaIdentita, T-B4-06 rivisto), come proposta, mai
//     come decisione: NodoProposto.Decisione resta nil. Uno: abbinamento_per_base; più d'uno: componenti_stessa_base,
//     tutti, senza una scelta (T-E1-07). Nessuno: niente abbinamento (nil).
//
// Si ricalcola a ogni fotografia, legato alla base. Un limite: la perdita di un abbinamento senza decisione non si vede,
// perché A1c non ha storia (R100); con la decisione per UUID la perdita di compatibilità si vede sempre.
type AbbinamentoPerBase struct {
	Componenti      []uuid.UUID           `json:"componenti,omitempty"`
	Motivo          string                `json:"motivo,omitempty"`
	Decisione       *uuid.UUID            `json:"decisione,omitempty"`
	MotivoDecisione string                `json:"motivo_decisione,omitempty"`
	Compatibilita   motorea.Compatibilita `json:"compatibilita,omitempty"`
}

// I valori di AbbinamentoPerBase.Motivo (T-B4-12, T-B4-26, T-B4-27) e di AbbinamentoPerBase.MotivoDecisione (T-B4-12,
// T-B4-26). MotivoNodoScartato è anche il motivo di una posizione di un candidato su un nodo scartato.
const (
	MotivoAbbinamentoPerBase                  = "abbinamento_per_base"
	MotivoAbbinamentoComponentiStessaBase     = "componenti_stessa_base"
	MotivoAbbinamentoAccantoAllaRigaDecisa    = "abbinamento_accanto_alla_riga_decisa"
	MotivoNodoScartato                        = "nodo_scartato"
	MotivoAbbinamentoDecisioneNonCompatibile  = "decisione_non_compatibile"
	MotivoAbbinamentoDecisioneNonVerificabile = "decisione_non_verificabile"
)

// compatibilitaIdentita: la compatibilità fra l'identità di un nodo e il codice letto di un componente: un altro
// namespace non è determinabile; con le basi compatibili, due marcatori scritti e diversi sono discordanti, uno scritto
// da una parte sola no (la regola unica del marcatore, T-B4-30).
func compatibilitaIdentita(nodo, componente motorea.LetturaForma) motorea.Compatibilita {
	if nodo.Namespace != componente.Namespace {
		return motorea.CompatibilitaNonDeterminabile
	}
	return conMarcatore(motorea.ConfrontaBasi(nodo.Base, componente.Base), marcatoreDi(nodo), marcatoreDi(componente))
}

// QuantitaDiscorde: il segnale della discordanza fra un arco proposto e l'arco confermato accanto (R95 A, T-B0-24):
// tutte e due le quantità sono date e sono diverse. L'arco confermato resta il valore corrente; una quantità che i
// fatti non danno non è un'informazione e non discorda. Il conflitto della gerarchia lo compone B5.
//
// Il limite: il segnale è per arco. Due nodi figli decisi come lo stesso componente sotto lo
// stesso padre (due duplicati) hanno ognuno la stessa relazione confermata accanto, e il segnale vale su tutti e due
// anche quando la somma delle loro quantità coincide con quella confermata: B5 confronta la somma per coppia di
// componenti prima di comporre il conflitto.
func QuantitaDiscorde(a ArcoProposto) bool {
	return a.Decisione != nil && a.Quantita != nil && a.Decisione.Quantita != nil && *a.Quantita != *a.Decisione.Quantita
}
