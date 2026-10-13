package valutazione

import (
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
)

// La verifica della BOM dall'ultima conferma strutturale valida (P4, passi B3.2, B3.3 e B3.4 del piano esecutivo
// B2-B5; direttiva 11: D-SHOCK-S1 B, D-SHOCK-S4 A con SHOCK-05, D-P4A-02 A, D-P4B1-02 A, tutte [U]; contratto rev. 3,
// Emendamenti 1 e 2, §8.3 e §11.1 n. 3; sintesi ABC §B.3, le cinque precisazioni). È una regola pura: l'ingresso
// (IngressoVerificaConferma) arriva già letto, e la validità della conferma (contatore o firma) la decide il chiamante.
// Nessuna lettura dal DB qui; l'innesto nel calcolo (B3.5) e la lettura (B3.1) sono passi successivi. Per i prodotti
// senza una conferma resta l'adattatore legacy di «Conferma l'albero» (gestiDaConfermaLAlbero), invariato.
//
// Il gesto nuovo (Legacy falso) si costruisce dalla conferma (gestiDallaConferma) e passa dalla regola di A1c
// (VerificaDellaBOM), che resta com'è; poi le condizioni nuove abbassano gli assi, mai li alzano: Verificata è sempre
// calcolata, e la conferma non porta nessun campo «verificata» (la colonna conferma_strutturale.verifica, che è la
// verifica al momento della conferma, qui non entra).
//
// Direttiva 12 del 13/10 (D-ADD-02 = B [U], ratifica con correzioni dell'addendum ad A1c, rev. 1 §8): T-2 è superata
// (uno STEP non analizzabile chiede la deroga come gli altri, e la verifica positiva con la gerarchia ricostruita a mano
// è ammessa); con uno STEP che non dà nodi la struttura da coprire è il perimetro certificato; la provenienza degli archi
// non si deduce (mai manuale per un'origine di trasporto o NULL); le evidenze sopravvenute dopo la conferma portano al
// riesame, le mancanze della prima conferma, le modifiche dopo la conferma, le deroghe invalide e gli errori d'integrità
// no (T-17 superata). I campi [P] (BaseGerarchia, provenienza per arco, Secondarie, diagnostica della deroga, il valore
// indeterminata) sono PROVVISORI: aspettano le domande D-ADD-03 e D-ADD-04 dell'addendum. La versione valutazione-4 e
// l'innesto in Calcola sono di B3.5: qui non cambiano, e una prova (TestNessunInnestoPrimaDelleRisposte) lo fissa finché
// i campi [P] restano provvisori. Revisione indipendente della rev. 1 recepita (addendum rev. 2): il perimetro scatta
// sulla fonte e non sulla struttura vuota (A-1), la fonte confermata senza Fonte è un ingresso incoerente (M-1), mista
// solo fra STEP e persona (M-3).

// CriterioVerifica: su che cosa può poggiare la verifica (conferma_strutturale.criterio_verifica, ck_conferma_criterio
// della 0022; contratto rev. 3 §8.2, §8.3).
type CriterioVerifica string

const (
	CriterioStep      CriterioVerifica = "step"
	CriterioMonoPezzo CriterioVerifica = "mono_pezzo"
	CriterioNessuno   CriterioVerifica = "nessuno"
)

// OrigineGerarchia: da dove vengono gli archi certificati dall'ultima conferma (precisazione c della sintesi ABC §B.3;
// contratto rev. 3 E2.1), aggregata dalla provenienza ricostruita di ogni arco (provenienzaDellArco; addendum ad A1c
// AD-5b, direttiva 12 §4 [U]): step se tutti vengono dallo STEP, manuale se tutti vengono da una persona, mista con archi
// dello STEP e di una persona e nient'altro, indeterminata altrimenti (origineDellaGerarchia). Vuota senza archi (un
// pezzo unico).
//
// indeterminata è un valore PROVVISORIO [P] (domanda D-ADD-03, parte a, dell'addendum, raccomandata A, senza risposta il
// 13/10): la direttiva 12 §4 vieta di presentare come manuale un arco di provenienza ignota e chiede, se il vocabolario
// step | manuale | mista non basta, «l'estensione contrattuale minima oppure un campo diagnostico esplicito». Finché la
// risposta non c'è il valore resta solo nella regola pura (B3.5 non l'ha ancora innestata in Calcola): con la B o la C di
// D-ADD-03 cambia la sola origineDellaGerarchia, e le prove che lo fissano sono marcate «D-ADD-03».
type OrigineGerarchia string

const (
	OrigineGerarchiaStep          OrigineGerarchia = "step"
	OrigineGerarchiaManuale       OrigineGerarchia = "manuale"
	OrigineGerarchiaMista         OrigineGerarchia = "mista"
	OrigineGerarchiaIndeterminata OrigineGerarchia = "indeterminata" // [P] D-ADD-03 a1 e a2, provvisorio
)

// Le provenienze di un arco certificato (addendum ad A1c AD-5b; direttiva 12 §4 [U]: «la provenienza deve derivare dalle
// informazioni registrate nell'azione o nel DTO, non da una deduzione basata sull'assenza di una proposta STEP»):
//   - step: l'arco accetta una proposta dello STEP (R5 lo scrive solo così, con l'identità della proposta);
//   - utente: l'arco l'ha creato una persona, attribuito a lei;
//   - codice_rilevato: un produttore che non è né lo STEP né una persona (origine_componente della 0001/0018: nel DB
//     c'è già); un produttore nuovo si aggiunge a provenienzeDeterminate, mai per difetto;
//   - indeterminata: la provenienza non si ricostruisce. I valori di trasporto conferma e registrato e il NULL non sono
//     una provenienza: dicono che l'arco c'era già, non chi l'ha creato.
const (
	ProvenienzaStep           = "step"
	ProvenienzaUtente         = "utente"
	ProvenienzaCodiceRilevato = "codice_rilevato"
	ProvenienzaIndeterminata  = "indeterminata"
)

// provenienzeDeterminate: le provenienze che la regola accetta come determinate. Ogni altro valore (compreso manuale, il
// default di componente.origine nella 0001, che non prova niente: addendum AD-5b punto 3) è indeterminata [T].
var provenienzeDeterminate = map[string]bool{ProvenienzaStep: true, ProvenienzaUtente: true, ProvenienzaCodiceRilevato: true}

// ---- gli ingressi ----

// ComponenteConfermato: una riga di conferma_strutturale_componente (0022), già letta.
type ComponenteConfermato struct {
	ComponenteID      uuid.UUID  `json:"componente_id"`
	Codice            string     `json:"codice"`
	Tipo              string     `json:"tipo"`
	Radice            bool       `json:"radice,omitempty"`
	StepStrutturaleID *uuid.UUID `json:"step_strutturale_id,omitempty"`
	StepSha256        string     `json:"step_sha256,omitempty"`
}

// ArcoConfermato: una riga di conferma_strutturale_arco (0022), già letta: i due componenti, la qta e l'origine
// (step | utente | conferma | registrato, oppure "" per NULL).
//   - Provenienza, ProvenienzaDa: la provenienza ricostruita dalla lettura (B3.1) quando l'origine è di trasporto
//     (conferma, registrato) o NULL, e da dove viene (per esempio «conferma:3», «componente_relazione»): la catena
//     dell'addendum ad A1c AD-5b punto 3, all'indietro sulle conferme precedenti. La regola pura non legge il DB e non
//     deduce: senza Provenienza un arco di trasporto o NULL è indeterminata (direttiva 12 §4 [U]). Con un'origine step o
//     utente vale l'origine, che è registrata dall'azione; una Provenienza diversa da lei è un'informazione che si
//     contraddice, quindi indeterminata, senza togliere la verifica (certezza della struttura ≠ certezza della
//     provenienza, direttiva 12 §4 «Obiettivo») [T].
type ArcoConfermato struct {
	PadreID       uuid.UUID `json:"padre_id"`
	FiglioID      uuid.UUID `json:"figlio_id"`
	Qta           int       `json:"qta"`
	Origine       string    `json:"origine,omitempty"`
	Provenienza   string    `json:"provenienza,omitempty"`
	ProvenienzaDa string    `json:"provenienza_da,omitempty"`
}

// ConfermaStrutturale: l'ultima conferma strutturale di un prodotto (il numero più alto), con i suoi componenti e i suoi
// archi certificati (tipo [T] di B3.2).
//   - Valida: la decide il chiamante (contratto rev. 3 §5.1: contatore registrato uguale a quello corrente, oppure firma
//     del perimetro ricalcolata uguale a quella registrata). Una conferma non valida non dà un gesto.
//   - ConfermatoDa, ConfermatoIl: chi e quando (il gesto nuovo li porta, come il legacy porta quelli del segno).
//   - MonoPezzo, MonoPezzoMotivo, Criterio: com'erano registrati (D-P4A-02 A, D-P4B1-02 A).
//
// Non c'è un campo «verificata»: la verifica si calcola sempre (direttiva 11 D-SHOCK-S1 B: nessun Verificata scritto).
type ConfermaStrutturale struct {
	ConfermaID      uuid.UUID              `json:"conferma_id"`
	Numero          int                    `json:"numero"`
	Valida          bool                   `json:"valida"`
	ConfermatoDa    uuid.UUID              `json:"confermato_da"`
	ConfermatoIl    string                 `json:"confermato_il"`
	Componenti      []ComponenteConfermato `json:"componenti,omitempty"`
	Archi           []ArcoConfermato       `json:"archi,omitempty"`
	MonoPezzo       bool                   `json:"mono_pezzo,omitempty"`
	MonoPezzoMotivo string                 `json:"mono_pezzo_motivo,omitempty"`
	Criterio        CriterioVerifica       `json:"criterio"`
}

// ArcoCorrente: una relazione confermata corrente del perimetro del prodotto (componente_relazione fra componenti
// attivi raggiungibili), con la sua qta. Due righe della stessa coppia si sommano [T], come nei conflitti di gerarchia.
type ArcoCorrente struct {
	PadreID  uuid.UUID `json:"padre_id"`
	FiglioID uuid.UUID `json:"figlio_id"`
	Qta      int       `json:"qta"`
}

// ChiaveAnalisi: l'analisi di uno STEP come la identifica deroga_struttura (0020): versione dell'analizzatore, hash della
// configurazione e calcolato_il dei fatti (UpsertAnalisiFatti riscrive i fatti con la stessa chiave: calcolato_il
// distingue le due letture). Tutta vuota = STEP senza un'analisi corrente: è la rappresentazione della 0020 per lo STEP
// non analizzato (le tre colonne NULL insieme, ck_deroga_struttura_analisi), e la lettura (B3.1) non inventa mai una
// chiave. Uno STEP con un'analisi corrente la cui struttura non si legge porta la chiave vera di quell'analisi (addendum
// ad A1c AD-4).
type ChiaveAnalisi struct {
	VersioneAnalizzatore int    `json:"versione_analizzatore,omitempty"`
	HashConfigurazione   string `json:"hash_configurazione,omitempty"`
	CalcolataIl          string `json:"calcolata_il,omitempty"`
}

// FonteDellaVerifica: la fonte STEP confermata del prodotto su cui si legge la BOM di lavoro: il documento autorizzato
// (componente.step_strutturale_id), il suo sha256 e l'analisi corrente. Che sia confermata e non superata (K-02) lo
// dice StrutturaDaVerificare.FonteConfermata, com'è in A1c.
type FonteDellaVerifica struct {
	DocumentoID uuid.UUID     `json:"documento_id"`
	Sha256      string        `json:"sha256"`
	Analisi     ChiaveAnalisi `json:"analisi"`
}

// StatoDeroga: lo stato di una deroga_struttura come lo deriva il chiamante dalla vista (0020: la validità non si
// scrive, si deriva): valida, oppure superata (sostituzione dello STEP, analizzatore o configurazione nuovi, rianalisi).
type StatoDeroga string

const (
	StatoDerogaValida   StatoDeroga = "valida"
	StatoDerogaSuperata StatoDeroga = "superata"
)

// DerogaStruttura: una deroga_struttura del prodotto (0020), già letta, nell'ordine del chiamante (la più recente prima,
// come la vista: ORDER BY concessa_il DESC).
//   - MotivoParziale, ConcessaDa, ConcessaIl: lo stato dell'analisi com'era quando si è derogato (motivo_parziale, NOT
//     NULL nella 0020, per esempio «nessuna analisi corrente dello STEP»), chi e quando. Servono solo alla diagnostica
//     (EsitoDeroga) e non cambiano la validità. Campi PROVVISORI [P] (addendum ad A1c §8, P-03), aggiunti per la
//     direttiva 12 §2.1 [U] («La deroga deve identificare inequivocabilmente la fonte, il suo SHA e lo stato dell'analisi
//     disponibile»; «il fatto che lo STEP non sia analizzabile deve continuare a essere visibile»).
type DerogaStruttura struct {
	ID              uuid.UUID     `json:"id"`
	StepDocumentoID uuid.UUID     `json:"step_documento_id"`
	StepSha256      string        `json:"step_sha256"`
	Analisi         ChiaveAnalisi `json:"analisi"`
	Motivo          string        `json:"motivo"`
	Stato           StatoDeroga   `json:"stato"`
	MotivoParziale  string        `json:"motivo_parziale,omitempty"`
	ConcessaDa      uuid.UUID     `json:"concessa_da"`
	ConcessaIl      string        `json:"concessa_il,omitempty"`
}

// DecisioniStep: quanto le decisioni di una persona dicono dei nodi e degli archi STEP della BOM di lavoro (per uno
// STEP che ha relazioni). Le letture del gesto nuovo: un nodo STEP deciso come un componente certificato dalla
// conferma è coperto; un nodo o un arco STEP tolto da una persona è un conflitto deciso, quindi coperto.
//   - Nodi: RifNodo → il componente deciso da una persona (NodoProposto.Decisione con DecisoDaPersona);
//   - NodiTolti, ArchiTolti: RifNodo e RifArco (fra due RifNodo) scartati da una persona.
type DecisioniStep struct {
	Nodi       map[string]uuid.UUID `json:"nodi,omitempty"`
	NodiTolti  []string             `json:"nodi_tolti,omitempty"`
	ArchiTolti []string             `json:"archi_tolti,omitempty"`
}

// RigaSecondariaAperta: una riga aperta rilevante di uno STEP secondario autorizzato nel perimetro (SHOCK-05,
// D-SHOCK-S4 A [U]): quali righe siano rilevanti lo decide il chiamante, con la regola del cancello del congelamento
// (fascicolo/gate.go: le decisioni strutturali aperte contano nell'autorità dei file autorizzati). Arco vero per una
// riga di relazione_proposta, falso per una di componente_proposta.
//   - Sopravvenuta: la riga è arrivata dopo l'ultima conferma valida (la decide la lettura, B3.1: la fonte secondaria è
//     stata scelta, o la riga è nata, dopo conferma_strutturale.confermato_il, con i tempi dello stesso orologio del
//     database; addendum ad A1c §8, consegne a B3.1). Una riga sopravvenuta è un'evidenza nuova e può portare il
//     prodotto a da_riesaminare; una riga già presente alla conferma e lasciata aperta è un requisito della prima
//     conferma che manca, e il prodotto resta non_pronto (direttiva 12 §3.5 [U]; addendum AD-4c). Sulla verifica della
//     BOM non cambia niente: tutte e due contano da_decidere (SHOCK-05, D-SHOCK-S4 A [U]).
type RigaSecondariaAperta struct {
	Rif          string `json:"rif"`
	Arco         bool   `json:"arco,omitempty"`
	Sopravvenuta bool   `json:"sopravvenuta,omitempty"`
}

// IngressoVerificaConferma: l'ingresso della regola della conferma (tipo [T] di B3.2).
//   - Conferma: l'ultima conferma del prodotto; nil se non ne ha (il chiamante usa allora l'adattatore legacy);
//   - Struttura: la struttura del prodotto come la prepara strutturaDaVerificare in A1c. La regola usa al posto di
//     ArchiConfermati gli archi di ArchiCorrenti, che portano la qta; se ArchiConfermati non è vuoto deve dire le stesse
//     coppie, altrimenti l'ingresso è incoerente (ingresso_incoerente: R-03 della revisione di B3, nessun ingresso
//     scartato in silenzio);
//   - ArchiCorrenti: le relazioni confermate correnti del perimetro, con la qta;
//   - Fonte: la fonte STEP confermata (nil senza fonte); la sua ChiaveAnalisi è quella dell'analisi di Analisi (con
//     Analisi.Letta una chiave vuota è un ingresso incoerente: R-07 della revisione di B3);
//   - DecisioniStep: i nodi e gli archi STEP decisi da una persona;
//   - Analisi: gli esiti dell'analisi corrente della fonte (GerarchiaStepDi);
//   - Deroghe: le deroghe strutturali del prodotto;
//   - SecondarieAperte: le righe aperte rilevanti degli STEP secondari autorizzati nel perimetro (SHOCK-05).
type IngressoVerificaConferma struct {
	Conferma         *ConfermaStrutturale   `json:"conferma,omitempty"`
	Struttura        StrutturaDaVerificare  `json:"struttura"`
	ArchiCorrenti    []ArcoCorrente         `json:"archi_correnti,omitempty"`
	Fonte            *FonteDellaVerifica    `json:"fonte,omitempty"`
	DecisioniStep    DecisioniStep          `json:"decisioni_step"`
	Analisi          AnalisiStruttura       `json:"analisi"`
	Deroghe          []DerogaStruttura      `json:"deroghe,omitempty"`
	SecondarieAperte []RigaSecondariaAperta `json:"secondarie_aperte,omitempty"`
}

// ---- l'uscita ----

// DettaglioConferma: che cosa la regola della conferma espone accanto agli assi (direttiva 11 D-SHOCK-S1 B: criterio,
// origine della gerarchia, gerarchia_step, stato e motivo della deroga in R4/R5/R7/R8; questi campi li portano lì i
// passi B3.6 e B4.6).
//   - ConfermaID, Numero, Valida: l'ultima conferma letta;
//   - ConfermatoDa, ConfermatoIl: chi e quando l'ha registrata (per mono_pezzo, l'autore della dichiarazione: contratto
//     rev. 3 §8.3 riga 4; R-06 della revisione di B3);
//   - RadiceMappata: la radice STEP della BOM di lavoro è la radice certificata (precisazione a);
//   - Cambiamenti: le differenze fra gli archi correnti e quelli certificati, nei due versi e con la qta (precisazioni b
//     ed e), in ordine: «aggiunto:<RifArco>», «tolto:<RifArco>», «qta:<RifArco>:<certificata>-><corrente>»;
//   - MonoPezzoMotivo: la dichiarazione registrata, accanto al criterio mono_pezzo;
//   - MotivoParzialeStep: il motivo parziale dell'analisi, intatto (vincolo V2), compreso quello di uno STEP senza
//     un'analisi corrente («nessuna analisi corrente dello STEP», v_step_prodotto della 0020): è la diagnostica che la
//     direttiva 12 §2.1 [U] vuole sempre visibile;
//   - Deroga: solo quando la verifica la richiede (gerarchia_step non_riconosciuta, parziale o non_analizzabile, con
//     figli, il criterio step e la fonte confermata), anche con la verifica positiva;
//   - SecondarieAperte: quante righe di STEP secondari contano da decidere (SHOCK-05);
//   - BaseGerarchia: su che cosa poggia la gerarchia verificata o da verificare (BaseGerarchia*; [P], P-04);
//   - ProvenienzaArchi, Archi: quanti archi certificati per provenienza ricostruita, e la provenienza di ognuno con da
//     dove viene (addendum ad A1c AD-5b punto 4; [P], P-05);
//   - Secondarie: le righe aperte degli STEP secondari, con il segno «sopravvenuta», perché la UI mostri la differenza
//     proposta in modo esplicito (addendum ad A1c AD-6; [P], P-08).
//
// I campi [P] sono PROVVISORI (domande D-ADD-03, D-ADD-04 dell'addendum, senza risposta il 13/10): tutti omitempty e
// dentro Conferma, quindi il JSON di un prodotto senza conferma non cambia. Non entrano nell'impronta del prodotto.
type DettaglioConferma struct {
	ConfermaID         *uuid.UUID             `json:"conferma_id,omitempty"`
	Numero             int                    `json:"numero,omitempty"`
	Valida             bool                   `json:"valida"`
	ConfermatoDa       *uuid.UUID             `json:"confermato_da,omitempty"`
	ConfermatoIl       string                 `json:"confermato_il,omitempty"`
	RadiceMappata      bool                   `json:"radice_mappata"`
	Cambiamenti        []string               `json:"cambiamenti,omitempty"`
	MonoPezzoMotivo    string                 `json:"mono_pezzo_motivo,omitempty"`
	MotivoParzialeStep string                 `json:"motivo_parziale_step,omitempty"`
	Deroga             *EsitoDeroga           `json:"deroga,omitempty"`
	SecondarieAperte   int                    `json:"secondarie_aperte,omitempty"`
	BaseGerarchia      BaseGerarchia          `json:"base_gerarchia,omitempty"`
	ProvenienzaArchi   map[string]int         `json:"provenienza_archi,omitempty"`
	Archi              []ProvenienzaArco      `json:"archi,omitempty"`
	Secondarie         []RigaSecondariaAperta `json:"secondarie,omitempty"`
}

// BaseGerarchia: su che cosa poggia la gerarchia di un prodotto con una conferma (addendum ad A1c AD-4; direttiva 12
// §2.1 [U]: «il sistema deve dichiarare che la gerarchia è stata ricostruita manualmente, non attribuirla falsamente
// allo STEP»). Il criterio step non basta a dirlo: per la 0022 vuol dire «lo STEP autorizzato del prodotto è la fonte
// confermata», e gli archi possono venire dallo STEP o essere decisi da una persona (COMMENT di criterio_verifica).
// Campo PROVVISORIO [P] (D-ADD-03, parte b, raccomandata «sì», senza risposta il 13/10):
//   - step: con figli, il criterio step, la fonte confermata e lo STEP che riconosce la gerarchia (riconosciuta);
//   - ricostruita_a_mano: con figli, il criterio step, la fonte confermata e lo STEP che non la determina
//     (non_riconosciuta, parziale, non_analizzabile): la deroga dice se la ricostruzione è accettata;
//   - mono_pezzo: la dichiarazione del pezzo unico;
//   - vuota: senza la fonte confermata, con una conferma che non conta, senza figli e senza dichiarazione, o con il
//     criterio nessuno.
type BaseGerarchia string

const (
	BaseGerarchiaStep             BaseGerarchia = "step"
	BaseGerarchiaRicostruitaAMano BaseGerarchia = "ricostruita_a_mano"
	BaseGerarchiaMonoPezzo        BaseGerarchia = "mono_pezzo"
)

// ProvenienzaArco: la provenienza ricostruita di un arco certificato (addendum ad A1c AD-5b punto 4; [P], P-05).
//   - Arco: il RifArco fra i due RifComponente;
//   - Origine: il valore registrato in conferma_strutturale_arco.origine, com'è ("" per NULL);
//   - Provenienza: step, utente, un altro produttore determinato, o indeterminata (provenienzaDellArco);
//   - Da: da dove viene la provenienza: «origine» se è il valore registrato, altrimenti il ProvenienzaDa della lettura.
type ProvenienzaArco struct {
	Arco        string `json:"arco"`
	Origine     string `json:"origine,omitempty"`
	Provenienza string `json:"provenienza"`
	Da          string `json:"da,omitempty"`
}

// EsitoDeroga: la deroga che la verifica richiede, con il suo stato e il suo motivo (E2.1).
//   - Stato: applicata (valida per quello STEP, quello sha e quell'analisi), oppure il motivo per cui non vale
//     (MotivoAsseDeroga*, senza il prefisso dell'asse): assente, scaduta, altro_step, altra_analisi, senza_motivo;
//   - ID e Motivo: la deroga considerata (quella applicata, o la più recente che non vale) e il motivo scritto da chi
//     l'ha concessa;
//   - MotivoParziale, ConcessaDa, ConcessaIl: lo stato dell'analisi derogata e chi e quando (PROVVISORI [P], P-03).
type EsitoDeroga struct {
	Stato          string     `json:"stato"`
	ID             *uuid.UUID `json:"id,omitempty"`
	Motivo         string     `json:"motivo,omitempty"`
	MotivoParziale string     `json:"motivo_parziale,omitempty"`
	ConcessaDa     *uuid.UUID `json:"concessa_da,omitempty"`
	ConcessaIl     string     `json:"concessa_il,omitempty"`
}

// Gli stati di EsitoDeroga.
const (
	DerogaApplicata    = "applicata"
	DerogaAssente      = "assente"
	DerogaScaduta      = "scaduta"
	DerogaAltroStep    = "altro_step"
	DerogaAltraAnalisi = "altra_analisi"
	DerogaSenzaMotivo  = "senza_motivo"
)

// I motivi nuovi di un asse (VerificaAsse.Motivo), della regola della conferma (tipi [T] di B3.2-B3.4; i nomi
// struttura_cambiata_dopo_conferma e senza_figli_non_dichiarato vengono dal contratto rev. 3 §8.3 e §11.1 n. 3):
//   - conferma_non_valida: l'ultima conferma non è valida (contatore e firma cambiati): nessun gesto;
//   - conferma_incoerente: la conferma letta non ha una e una sola radice, cita archi fra componenti che non certifica,
//     o il criterio non è coerente con mono_pezzo: blocco prudente, nessun gesto creduto;
//   - ingresso_incoerente: la conferma è coerente, ma gli altri ingressi non lo sono fra loro (Struttura.ArchiConfermati
//     con coppie diverse da ArchiCorrenti; Analisi letta con la chiave d'analisi della fonte vuota): blocco prudente,
//     nessun gesto creduto (R-03 e R-07 della revisione di B3);
//   - struttura_cambiata_dopo_conferma: gli archi correnti non coincidono con quelli certificati (un arco aggiunto, un
//     arco tolto, una qta cambiata), sui due assi;
//   - mono_pezzo_decaduto: la conferma dichiara mono_pezzo e ora il prodotto ha figli (la dichiarazione perde efficacia);
//   - mono_pezzo_non_applicabile: la conferma dichiara mono_pezzo ma la sua radice non è un finito con lo STEP, o il
//     motivo è vuoto: blocco prudente (lo schema lo rifiuta al COMMIT, la regola non ci si affida);
//   - radice_step_non_mappata: la radice STEP della BOM di lavoro non è la radice certificata, o la BOM di lavoro ha più
//     di una radice con lo sha della fonte (precisazione a; R-03 della revisione di B3); con uno STEP che non dà nodi,
//     la radice certificata non porta lo STEP della fonte (sha, e documento se registrato) [T];
//   - senza_figli_non_dichiarato: nessun figlio, né certificato né corrente, e nessuna dichiarazione mono_pezzo;
//   - criterio_conferma_nessuno: la conferma ha figli e il criterio nessuno (confermata senza STEP autorizzato), mentre
//     la fonte è confermata: la conferma non poggiava sullo STEP, serve la n+1 (blocco prudente);
//   - deroga_struttura_*: con la fonte confermata, lo STEP non determina la gerarchia (non_riconosciuta, parziale o
//     non_analizzabile) e la deroga richiesta manca o non vale per quello STEP, quello sha e quell'analisi (D-SHOCK-S1 B;
//     direttiva 12 §2.1 [U]).
//
// Il motivo gerarchia_step_non_analizzabile non c'è più: la scelta T-2 (STEP non_analizzabile blocca anche con una
// deroga) è SUPERATA dalla direttiva 12 §2.1 [U] («uno STEP non analizzabile non impedisce all'ingegnere di ricostruire
// e confermare manualmente la BOM»; addendum ad A1c AD-4). Uno STEP non analizzabile chiede la deroga come gli altri due
// valori; il fatto resta visibile in VerificaBOM.GerarchiaStep, in DettaglioConferma.MotivoParzialeStep e nella deroga.
const (
	MotivoAsseConfermaNonValida       = "conferma_non_valida"
	MotivoAsseConfermaIncoerente      = "conferma_incoerente"
	MotivoAsseIngressoIncoerente      = "ingresso_incoerente"
	MotivoAsseStrutturaCambiata       = "struttura_cambiata_dopo_conferma"
	MotivoAsseMonoPezzoDecaduto       = "mono_pezzo_decaduto"
	MotivoAsseMonoPezzoNonApplicabile = "mono_pezzo_non_applicabile"
	MotivoAsseRadiceNonMappata        = "radice_step_non_mappata"
	MotivoAsseSenzaFigliNonDichiarato = "senza_figli_non_dichiarato"
	MotivoAsseCriterioConfermaNessuno = "criterio_conferma_nessuno"
	MotivoAsseDerogaAssente           = "deroga_struttura_" + DerogaAssente
	MotivoAsseDerogaScaduta           = "deroga_struttura_" + DerogaScaduta
	MotivoAsseDerogaAltroStep         = "deroga_struttura_" + DerogaAltroStep
	MotivoAsseDerogaAltraAnalisi      = "deroga_struttura_" + DerogaAltraAnalisi
	MotivoAsseDerogaSenzaMotivo       = "deroga_struttura_" + DerogaSenzaMotivo
)

// rangoDelMotivo: l'ordine dei motivi di un asse nella regola della conferma: vale il primo (il rango più basso).
// conflitto e target_senza_componente vengono da VerificaDellaBOM e prevalgono su tutto (R95 A: una contraddizione
// aperta blocca e nessuna deroga la converte); poi la conferma che non vale o non è coerente, e gli ingressi incoerenti;
// poi la struttura cambiata (il caso del §8.3 che dice «quello della conferma»); poi la copertura del gesto
// (da_decidere, con le righe dei secondari di SHOCK-05); poi la dichiarazione mancante, il criterio, la gerarchia dello
// STEP e la deroga, che contano solo quando tutto il resto è a posto; per ultimo K-02.
//
// fonte_non_confermata sta in fondo perché vale solo quando è l'unica cosa che manca (A1c T-E1-14 e T-E1R-04, congelati:
// una gerarchia ferma solo per lei dopo una fonte superata è ferma per una condizione nuova). Il criterio, la gerarchia
// dello STEP e la deroga sono proprietà della fonte confermata (D-SHOCK-S1 B: una deroga «per quello STEP, il suo SHA e la
// sua analisi»): la regola li valuta solo con la fonte confermata, quindi non si trovano mai accanto a K-02, e senza la
// fonte confermata il motivo della gerarchia è fonte_non_confermata solo se nessuna mancanza indipendente dalla fonte
// (conflitto, conferma, struttura, copertura, dichiarazione) la precede [T], R-01 della revisione di B3; addendum ad A1c
// AD-4b. Un motivo che qui non c'è (della regola legacy) sta col da_decidere.
var rangoDelMotivo = map[string]int{
	MotivoAsseConflitto:               0,
	MotivoAsseTargetSenzaComponente:   1,
	MotivoAsseConfermaNonValida:       2,
	MotivoAsseConfermaIncoerente:      3,
	MotivoAsseIngressoIncoerente:      3,
	MotivoAsseMonoPezzoNonApplicabile: 3,
	MotivoAsseStrutturaCambiata:       4,
	MotivoAsseMonoPezzoDecaduto:       4,
	MotivoAsseNessunGesto:             5,
	MotivoAsseNessunaStruttura:        5,
	MotivoAsseRadiceNonMappata:        6,
	MotivoAsseDaDecidere:              7,
	MotivoAsseNomenclaturaRadice:      8,
	MotivoAsseSenzaFigliNonDichiarato: 9,
	MotivoAsseCriterioConfermaNessuno: 10,
	MotivoAsseDerogaAssente:           11,
	MotivoAsseDerogaScaduta:           11,
	MotivoAsseDerogaAltroStep:         11,
	MotivoAsseDerogaAltraAnalisi:      11,
	MotivoAsseDerogaSenzaMotivo:       11,
	MotivoAsseFonteNonConfermata:      12,
}

const rangoSconosciuto = 7

func rango(motivo string) int {
	if r, ok := rangoDelMotivo[motivo]; ok {
		return r
	}
	return rangoSconosciuto
}

// abbassa: l'asse diventa da_verificare con il motivo, se è verificata o se il motivo prevale su quello che ha.
// conflitto e non_verificabile non cambiano stato (hanno il rango più basso). Mai verso verificata.
func abbassa(a *VerificaAsse, motivo string) {
	if a.Stato == StatoAsseVerificata || rango(motivo) < rango(a.Motivo) {
		a.Stato, a.Motivo = StatoAsseDaVerificare, motivo
	}
}

// ---- la regola ----

// VerificaDallaConferma: la verifica della BOM di un prodotto che ha una conferma strutturale (direttiva 11
// D-SHOCK-S1 B [U]; contratto rev. 3 §8.3, §11.1 n. 3). Passi:
//  1. il gesto nuovo dall'ultima conferma valida, sui due assi (gestiDallaConferma: precisazioni a e d), e la regola di
//     A1c VerificaDellaBOM su quel gesto e sulla struttura, con le relazioni correnti al posto di ArchiConfermati;
//  2. la conferma che non vale (conferma_non_valida) o non è coerente (conferma_incoerente, mono_pezzo_non_applicabile),
//     e gli ingressi incoerenti fra loro (ingresso_incoerente);
//  3. il confronto degli archi nei due versi e con la qta (precisazioni b ed e): struttura_cambiata_dopo_conferma sui
//     due assi, o mono_pezzo_decaduto per un pezzo unico che ora ha figli;
//  4. la radice STEP non mappata (radice_step_non_mappata, nomenclatura) e le righe aperte degli STEP secondari
//     autorizzati (SHOCK-05: da_decidere, anche se aggiungono soltanto figli);
//  5. il ramo mono_pezzo (D-P4B1-02 A, D-P4A-02 A): vale solo con una dichiarazione motivata, senza figli né archi, sulla
//     radice finito con lo STEP; non salta K-02, che VerificaDellaBOM ha già applicato (fonte_non_confermata resta). Senza
//     figli e senza dichiarazione: senza_figli_non_dichiarato (mai un mono_pezzo automatico, SHOCK-01);
//  6. con figli e con la fonte confermata: il criterio nessuno di una conferma non regge (criterio_conferma_nessuno);
//     poi gerarchia_step (GerarchiaStepDi): non_riconosciuta, parziale e non_analizzabile vogliono una deroga_struttura
//     valida per lo STEP della fonte, il suo sha e la sua analisi (D-SHOCK-S1 B; direttiva 12 §2.1 [U], che supera T-2:
//     con uno STEP non analizzato la deroga porta la chiave d'analisi vuota della 0020), riconosciuta no. Senza la fonte
//     confermata questi controlli non si fanno: sono proprietà della fonte, e K-02 ha già tolto la verifica della
//     gerarchia (R-01 della revisione di B3; addendum ad A1c AD-4b);
//  7. il criterio esposto (§8.3), la base della gerarchia [P] e l'origine della gerarchia dalla provenienza degli archi
//     (precisazione c; addendum AD-5b).
//
// Lo STEP che non dà una struttura (direttiva 12 §2.1 [U], caso A e caso mono_pezzo; addendum ad A1c AD-1, AD-3, §8
// P-02; A-1 della revisione della rev. 1). Quando la conferma conta, la fonte è confermata, lo STEP della fonte non dà
// nodi (senza un'analisi corrente, o con una struttura che non si legge: nessuna BOM di lavoro e nessun suo nodo fra le
// strutture) e non riconosce la gerarchia, la struttura da coprire è il perimetro certificato dalla conferma: i
// componenti certificati come nodi, la radice certificata come radice; le strutture candidate di altri file escono
// (perimetroAlPostoDellaStruttura, T-24). Senza la fonte confermata (caso B) il perimetro vale solo quando non c'è
// nessuna struttura. Il confronto degli archi correnti con quelli certificati (passo 3) e la deroga (passo 6) restano;
// prima la regola ereditava da A1c nessuna_struttura sui due assi (V/bom.go, VerificaDellaBOM, che non cambia) e
// bloccava per sempre il caso A e il pezzo unico. Con la fonte confermata la radice certificata deve portare lo STEP
// della fonte, altrimenti radice_step_non_mappata [T] T-19.
//
// Le condizioni dei passi 2-6 abbassano l'asse solo secondo rangoDelMotivo: una deroga conta solo quando nient'altro
// manca, quindi non converte mai un conflitto aperto, un arco aperto non coperto o una fonte non confermata in verifica
// positiva.
func VerificaDallaConferma(in IngressoVerificaConferma) VerificaBOM {
	v, _, _ := verificaDallaConferma(in, nil)
	return v
}

// verificaDallaConferma: VerificaDallaConferma, con il gesto e la struttura che ha usato (la struttura con le relazioni
// correnti al posto di ArchiConfermati, e con il perimetro certificato quando lo STEP non dà nodi), per l'adattatore
// delle condizioni nuove (condizioniNuoveDallaConferma). coperti: Rif (nodi e archi) che il gesto copre in più; li usa
// solo l'adattatore, per togliere dal conto le voci che sono condizioni nuove (R111) e vedere che cosa resta: nil per la
// verifica vera.
func verificaDallaConferma(in IngressoVerificaConferma, coperti []string) (VerificaBOM, GestiVerificaBOM, StrutturaDaVerificare) {
	s := in.Struttura
	correnti := archiCorrentiPerCoppia(in.ArchiCorrenti)
	s.ArchiConfermati = rifDegliArchiCorrenti(correnti)
	gs := GerarchiaStepDi(in.Analisi)
	c := in.Conferma
	if c == nil {
		v := VerificaDellaBOM(GestiVerificaBOM{}, s)
		v.Criterio, v.GerarchiaStep = CriterioNessuno, gs
		return v, GestiVerificaBOM{}, s
	}

	incoerenza := incoerenzaDellaConferma(c)
	if incoerenza == "" {
		incoerenza = incoerenzaDegliIngressi(in, s.ArchiConfermati)
	}
	conta := c.Valida && incoerenza == ""
	var g GestiVerificaBOM
	radiceMappata := false
	if conta {
		g, radiceMappata = gestiDallaConferma(c, s, in.Fonte, in.DecisioniStep, coperti)
	}
	// lo STEP della fonte che non dà nodi: la struttura da coprire è il perimetro certificato (P-02; A-1 della revisione
	// della rev. 1: il segnale è la fonte, non la struttura vuota), e le strutture candidate degli altri file escono
	perimetroCertificato := conta && perimetroAlPostoDellaStruttura(s, in.Fonte, gs)
	if perimetroCertificato {
		s.Nodi, s.Radici = perimetroDellaConferma(c)
		s.Archi, s.RigheDaDecidere, s.ArchiDaDecidere = nil, nil, nil
	}
	v := VerificaDellaBOM(g, s)
	nom, ger := &v.Nomenclatura, &v.Gerarchia
	// Una gerarchia vuota aspetta la nomenclatura della radice (R80), e qui la radice STEP si copre solo con la mappatura
	// sulla BOM di lavoro, che c'è solo con la fonte confermata: senza la fonte, quando la nomenclatura è ferma soltanto
	// per le radici STEP che non si possono mappare, il motivo vero della gerarchia è K-02 (contratto rev. 3 §8.3,
	// «mono_pezzo ma fonte non confermata: fonte_non_confermata») [T]. Se la nomenclatura ha altro da decidere, il motivo
	// resta quello di A1c (R-01 della revisione di B3: fonte_non_confermata solo quando è l'unica cosa che manca).
	if !s.FonteConfermata && ger.Stato == StatoAsseDaVerificare && ger.Motivo == MotivoAsseNomenclaturaRadice &&
		nom.Stato == StatoAsseDaVerificare && nom.Motivo == MotivoAsseDaDecidere {
		if vociNom, _ := vociDaDecidere(g, s); len(vociNom) > 0 && len(differenza(vociNom, s.Radici)) == 0 {
			ger.Motivo = MotivoAsseFonteNonConfermata
		}
	}
	det := &DettaglioConferma{ConfermaID: copiaUUID(&c.ConfermaID), Numero: c.Numero, Valida: c.Valida, RadiceMappata: radiceMappata,
		MonoPezzoMotivo: c.MonoPezzoMotivo, MotivoParzialeStep: in.Analisi.MotivoParziale, ConfermatoIl: c.ConfermatoIl}
	if c.ConfermatoDa != uuid.Nil {
		det.ConfermatoDa = copiaUUID(&c.ConfermatoDa)
	}

	// 2. la conferma che non vale o non è coerente
	switch {
	case !c.Valida:
		abbassa(nom, MotivoAsseConfermaNonValida)
		abbassa(ger, MotivoAsseConfermaNonValida)
	case incoerenza != "":
		abbassa(nom, incoerenza)
		abbassa(ger, incoerenza)
	}
	if m := monoPezzoNonApplicabile(c); m != "" {
		abbassa(ger, m)
	}

	// 3. gli archi correnti contro quelli certificati, nei due versi e con la qta
	det.Cambiamenti = cambiamentiDopoLaConferma(c.Archi, correnti)
	if len(det.Cambiamenti) > 0 {
		motivo := MotivoAsseStrutturaCambiata
		if c.MonoPezzo && len(correnti) > 0 {
			motivo = MotivoAsseMonoPezzoDecaduto
		}
		abbassa(nom, motivo)
		abbassa(ger, motivo)
	}

	// 4. la radice STEP della BOM di lavoro (o, senza nodi STEP, la radice certificata con lo STEP della fonte) e le righe
	// aperte degli STEP secondari (SHOCK-05)
	switch {
	case perimetroCertificato:
		if s.FonteConfermata && in.Fonte != nil && !radiceDellaFonte(radiceCertificata(c), in.Fonte) {
			abbassa(nom, MotivoAsseRadiceNonMappata)
		}
	case conta && s.BOMDiLavoro && !radiceMappata:
		abbassa(nom, MotivoAsseRadiceNonMappata)
	}
	if n := len(in.SecondarieAperte); n > 0 {
		det.SecondarieAperte = n
		det.Secondarie = append([]RigaSecondariaAperta(nil), in.SecondarieAperte...)
		for _, r := range in.SecondarieAperte {
			if !r.Arco {
				nom.DaDecidere++
			}
			ger.DaDecidere++
		}
		if nom.DaDecidere > 0 {
			abbassa(nom, MotivoAsseDaDecidere)
		}
		abbassa(ger, MotivoAsseDaDecidere)
	}

	// 5. il ramo mono_pezzo e il prodotto senza figli
	senzaFigli := len(c.Archi) == 0 && len(correnti) == 0
	if senzaFigli && !c.MonoPezzo {
		abbassa(ger, MotivoAsseSenzaFigliNonDichiarato)
	}

	// 6. con figli e con la fonte confermata: il criterio della conferma, la gerarchia dello STEP e la deroga (senza la
	// fonte confermata non si valutano: K-02 resta il motivo, se nient'altro di indipendente dalla fonte manca)
	if !c.MonoPezzo && len(c.Archi) > 0 && s.FonteConfermata {
		if c.Criterio == CriterioNessuno {
			abbassa(ger, MotivoAsseCriterioConfermaNessuno)
		}
		if gs != GerarchiaStepRiconosciuta {
			// non_riconosciuta, parziale (T-1 [T] orch.) e non_analizzabile (T-2 superata, direttiva 12 §2.1 [U])
			det.Deroga = derogaApplicabile(in.Deroghe, in.Fonte)
			if det.Deroga.Stato != DerogaApplicata {
				abbassa(ger, "deroga_struttura_"+det.Deroga.Stato)
			}
		}
	}

	// 7. il risultato, il criterio esposto, la base e l'origine della gerarchia
	v.Verificata = nom.Stato == StatoAsseVerificata && ger.Stato == StatoAsseVerificata
	v.GerarchiaStep, v.Conferma = gs, det
	v.OrigineGerarchia, det.ProvenienzaArchi, det.Archi = origineDellaGerarchia(c.Archi)
	v.Criterio = criterioEsposto(c, v, conta, s.FonteConfermata, senzaFigli)
	det.BaseGerarchia = baseDellaGerarchia(c, gs, conta, s.FonteConfermata)
	return v, g, s
}

// baseDellaGerarchia: su che cosa poggia la gerarchia (BaseGerarchia, [P] P-04; addendum ad A1c AD-4). La deroga non
// entra: dice se la ricostruzione a mano è accettata, non se c'è stata.
func baseDellaGerarchia(c *ConfermaStrutturale, gs GerarchiaStep, conta, fonteConfermata bool) BaseGerarchia {
	switch {
	case !conta || !fonteConfermata:
		return ""
	case c.MonoPezzo:
		return BaseGerarchiaMonoPezzo
	case len(c.Archi) == 0 || c.Criterio != CriterioStep:
		return ""
	case gs == GerarchiaStepRiconosciuta:
		return BaseGerarchiaStep
	default:
		return BaseGerarchiaRicostruitaAMano
	}
}

// perimetroAlPostoDellaStruttura: se la struttura da coprire è il perimetro certificato invece della struttura di A1c
// (addendum ad A1c AD-1, «lo STEP che non dà una struttura»; §8 P-02; A-1 della revisione della rev. 1). Con una
// conferma che conta (lo controlla il chiamante):
//   - con la fonte confermata: quando lo STEP della fonte non dà nodi, cioè nessuna BOM di lavoro e nessun nodo con lo
//     sha della fonte fra le strutture, e lo STEP non riconosce la gerarchia. Le strutture candidate di altri file della
//     RFQ che contengono il codice del prodotto non sono la struttura di un prodotto confermato (R76 A: conta la BOM di
//     lavoro della fonte confermata; qui la conferma ne fa le veci) ed escono [T] T-24. Uno STEP che riconosce la
//     gerarchia e non dà nodi è un ingresso che non torna: resta la regola di A1c, nessuna_struttura (T-18); uno STEP
//     della fonte che dà nodi fuori dalla BOM di lavoro resta pure alla regola di A1c, con i suoi nodi da decidere (T-25:
//     una proposta della fonte non si nasconde);
//   - senza la fonte confermata (caso B della direttiva 12 §2.1): solo quando non c'è nessuna struttura. Con una struttura
//     candidata resta la regola di A1c (R71 A, R76 A): le sue voci sono da decidere, comprese quelle del nuovo STEP dopo
//     una fonte superata, che R111 A [U] conta come condizioni nuove e la direttiva 12 §3.3 [U] vuole visibili; il
//     perimetro le nasconderebbe [T] T-24. Il criterio, gerarchia_step e la deroga senza la fonte non contano (T-13).
func perimetroAlPostoDellaStruttura(s StrutturaDaVerificare, fonte *FonteDellaVerifica, gs GerarchiaStep) bool {
	if !s.FonteConfermata {
		return len(s.Nodi) == 0
	}
	if s.BOMDiLavoro || gs == GerarchiaStepRiconosciuta || fonte == nil || fonte.Sha256 == "" {
		return false
	}
	prefisso := ancoraggio.RifNodo(fonte.Sha256, "")
	for _, n := range s.Nodi {
		if strings.HasPrefix(n, prefisso) {
			return false
		}
	}
	return true
}

// perimetroDellaConferma: i nodi e le radici della struttura da coprire quando lo STEP della fonte non dà nodi (P-02):
// i RifComponente dei componenti certificati e quello della radice certificata. Il gesto dalla conferma li copre tutti
// (gestiDallaConferma); le relazioni correnti restano in ArchiConfermati e si confrontano con gli archi certificati.
func perimetroDellaConferma(c *ConfermaStrutturale) (nodi, radici []string) {
	for _, k := range c.Componenti {
		nodi = append(nodi, ancoraggio.RifComponente(k.ComponenteID))
		if k.Radice {
			radici = append(radici, ancoraggio.RifComponente(k.ComponenteID))
		}
	}
	return ordinatiUnici(nodi), ordinatiUnici(radici)
}

// radiceCertificata: la riga radice della conferma (con una conferma coerente ce n'è una sola), o nil.
func radiceCertificata(c *ConfermaStrutturale) *ComponenteConfermato {
	for i := range c.Componenti {
		if c.Componenti[i].Radice {
			return &c.Componenti[i]
		}
	}
	return nil
}

// radiceDellaFonte: la radice certificata porta lo STEP della fonte (precisazione a della sintesi ABC §B.3): lo stesso
// sha256 e, se la conferma lo registra, lo stesso documento.
func radiceDellaFonte(radice *ComponenteConfermato, fonte *FonteDellaVerifica) bool {
	return radice != nil && fonte != nil && fonte.Sha256 != "" && radice.StepSha256 == fonte.Sha256 &&
		(radice.StepStrutturaleID == nil || *radice.StepStrutturaleID == fonte.DocumentoID)
}

// criterioEsposto: il criterio che la verifica espone (contratto rev. 3 §8.3), sempre con lo stato della fonte accanto
// (FonteRegistrata; E1.2: mai il solo «criterio: mono_pezzo»):
//   - nessuno per una conferma che non conta (non valida, incoerente, o con ingressi incoerenti): la sua dichiarazione
//     non vale (R-11 della revisione di B3; il contratto rev. 3 §8.3 non dice niente di questo caso) [T];
//   - mono_pezzo per una conferma che lo dichiara (verificata o no: con la fonte non confermata o la dichiarazione
//     decaduta il motivo lo dice);
//   - step per una verifica positiva con figli;
//   - nessuno con la fonte non confermata, senza figli e senza dichiarazione, o con il criterio nessuno della conferma;
//   - altrimenti quello della conferma (la struttura cambiata, la deroga che manca).
//
// step vuol dire «la fonte STEP autorizzata del prodotto è la fonte confermata», non «la gerarchia viene dallo STEP»: gli
// archi possono venire dallo STEP o essere decisi da una persona (COMMENT di criterio_verifica nella 0022). Su che cosa
// poggia la gerarchia lo dice BaseGerarchia ([P]), da dove vengono gli archi OrigineGerarchia (addendum ad A1c AD-4, §8
// P-09).
func criterioEsposto(c *ConfermaStrutturale, v VerificaBOM, conta, fonteConfermata, senzaFigli bool) CriterioVerifica {
	switch {
	case !conta:
		return CriterioNessuno
	case c.MonoPezzo:
		return CriterioMonoPezzo
	case v.Verificata:
		return CriterioStep
	case !fonteConfermata, senzaFigli, c.Criterio == CriterioNessuno:
		return CriterioNessuno
	case c.Criterio == CriterioStep:
		return CriterioStep
	default:
		return CriterioNessuno
	}
}

// gestiDallaConferma: il produttore non legacy del gesto (Legacy falso) dall'ultima conferma valida (direttiva 11
// D-SHOCK-S1 B; contratto rev. 3 §11.1 n. 3; addendum ad A1c AD-1). Lo stesso contenuto per i due assi (precisazione d:
// gli archi certificati entrano anche nel gesto della nomenclatura, che nel modello nuovo conta le relazioni confermate),
// in due copie. Chi e quando sono quelli della conferma. Copre:
//   - i componenti certificati (RifComponente) e gli archi certificati (RifArco fra due RifComponente);
//   - la radice STEP della BOM di lavoro, se è la radice certificata (precisazione a: la conferma certifica il prodotto
//     con lo STEP della fonte, conferma_strutturale_componente.radice e step_sha256 uguale allo sha della fonte, e il
//     documento uguale se c'è). radiceMappata lo dice;
//   - i nodi STEP decisi da una persona come un componente certificato, e gli archi STEP fra due nodi così (o fra la
//     radice mappata e un nodo così) che corrispondono a un arco certificato;
//   - i nodi e gli archi STEP tolti da una persona (conflitti decisi).
//
// Un nodo o un arco STEP che non rientra in nessuno di questi resta fuori dal gesto, quindi da decidere: una proposta
// aperta che la conferma non ha recepito è una contraddizione aperta, non una verifica.
//
// coperti: Rif in più (RifArco fra gli archi, gli altri fra i nodi), solo per l'adattatore delle condizioni nuove; nil
// per la verifica vera.
func gestiDallaConferma(c *ConfermaStrutturale, s StrutturaDaVerificare, fonte *FonteDellaVerifica, d DecisioniStep, coperti []string) (GestiVerificaBOM, bool) {
	certificati := map[uuid.UUID]bool{}
	var radice *ComponenteConfermato
	var nodi, archi []string
	for i := range c.Componenti {
		k := &c.Componenti[i]
		certificati[k.ComponenteID] = true
		nodi = append(nodi, ancoraggio.RifComponente(k.ComponenteID))
		if k.Radice {
			radice = k
		}
	}
	archiCertificati := map[[2]uuid.UUID]bool{}
	for _, a := range c.Archi {
		archiCertificati[[2]uuid.UUID{a.PadreID, a.FiglioID}] = true
		archi = append(archi, RifArco(ancoraggio.RifComponente(a.PadreID), ancoraggio.RifComponente(a.FiglioID)))
	}

	// i nodi STEP che valgono un componente certificato: le decisioni di una persona, più la radice mappata
	nodiDel := map[uuid.UUID][]string{}
	for rif, comp := range d.Nodi {
		if certificati[comp] {
			nodiDel[comp] = append(nodiDel[comp], rif)
		}
	}
	// la radice si mappa solo se la BOM di lavoro ha una e una sola radice con lo sha della fonte: con più radici del
	// file nessuna è la radice certificata per difetto (R-03 della revisione di B3)
	radiceMappata := false
	if s.BOMDiLavoro && radiceDellaFonte(radice, fonte) {
		prefisso := ancoraggio.RifNodo(fonte.Sha256, "")
		var candidate []string
		for _, r := range ordinatiUnici(s.Radici) {
			if strings.HasPrefix(r, prefisso) {
				candidate = append(candidate, r)
			}
		}
		if len(candidate) == 1 {
			nodiDel[radice.ComponenteID] = append(nodiDel[radice.ComponenteID], candidate[0])
			radiceMappata = true
		}
	}
	for _, rifs := range nodiDel {
		nodi = append(nodi, rifs...)
	}
	for _, a := range c.Archi {
		for _, p := range nodiDel[a.PadreID] {
			for _, f := range nodiDel[a.FiglioID] {
				archi = append(archi, RifArco(p, f))
			}
		}
	}
	nodi = append(nodi, d.NodiTolti...)
	archi = append(archi, d.ArchiTolti...)
	for _, r := range coperti {
		if strings.HasPrefix(r, PrefissoRifArco) {
			archi = append(archi, r)
		} else {
			nodi = append(nodi, r)
		}
	}

	da := c.ConfermatoDa
	gesto := GestoVerifica{Da: &da, Il: c.ConfermatoIl, Nodi: ordinatiUnici(nodi), Archi: ordinatiUnici(archi)}
	altro := gesto
	altro.Da = copiaUUID(gesto.Da)
	altro.Nodi, altro.Archi = append([]string(nil), gesto.Nodi...), append([]string(nil), gesto.Archi...)
	return GestiVerificaBOM{Nomenclatura: &gesto, Gerarchia: &altro}, radiceMappata
}

// incoerenzaDellaConferma: una conferma letta che la regola non può credere (blocco prudente, conferma_incoerente):
// non una e una sola radice; un arco con un estremo che la conferma non certifica, o con qta non positiva; un criterio
// fuori dall'elenco, o mono_pezzo non coerente con il criterio (ck_conferma_criterio). Lo schema lo impedisce già; la
// regola non ci si affida, perché l'ingresso è astratto.
func incoerenzaDellaConferma(c *ConfermaStrutturale) string {
	radici := 0
	certificati := map[uuid.UUID]bool{}
	for _, k := range c.Componenti {
		certificati[k.ComponenteID] = true
		if k.Radice {
			radici++
		}
	}
	if radici != 1 {
		return MotivoAsseConfermaIncoerente
	}
	for _, a := range c.Archi {
		if !certificati[a.PadreID] || !certificati[a.FiglioID] || a.Qta <= 0 || a.PadreID == a.FiglioID {
			return MotivoAsseConfermaIncoerente
		}
	}
	switch c.Criterio {
	case CriterioStep, CriterioMonoPezzo, CriterioNessuno:
	default:
		return MotivoAsseConfermaIncoerente
	}
	if c.MonoPezzo != (c.Criterio == CriterioMonoPezzo) {
		return MotivoAsseConfermaIncoerente
	}
	return ""
}

// incoerenzaDegliIngressi: gli ingressi della regola che non sono coerenti fra loro, con una conferma coerente (blocco
// prudente, ingresso_incoerente; R-03 e R-07 della revisione di B3): la regola non scarta un ingresso in silenzio e non
// crede a due fonti che si contraddicono.
//   - Struttura.ArchiConfermati non vuoto con coppie diverse dagli archi correnti (correnti: i RifArco di ArchiCorrenti):
//     il chiamante ha dimenticato ArchiCorrenti, o li ha presi da un altro perimetro;
//   - un'analisi letta (Analisi.Letta) con la fonte e la sua chiave d'analisi vuota: per la 0020 una chiave vuota è uno
//     STEP non analizzato, quindi l'analisi letta non è quella della fonte, e una deroga con la chiave vuota varrebbe;
//   - la fonte confermata (Struttura.FonteConfermata) senza la fonte (Fonte nil): la regola non saprebbe con quale STEP
//     controllare la radice certificata, e un mono_pezzo passerebbe con la radice di un altro sha (M-1 della revisione
//     della rev. 1; direttiva 12 §2.1 [U]: il mono_pezzo vale «purché la fonte STEP richiesta sia presente, corrente e
//     autorizzata») [T].
func incoerenzaDegliIngressi(in IngressoVerificaConferma, correnti []string) string {
	if in.Struttura.FonteConfermata && in.Fonte == nil {
		return MotivoAsseIngressoIncoerente
	}
	if len(in.Struttura.ArchiConfermati) > 0 {
		dichiarati := ordinatiUnici(in.Struttura.ArchiConfermati)
		if len(dichiarati) != len(correnti) || len(differenza(dichiarati, correnti)) > 0 {
			return MotivoAsseIngressoIncoerente
		}
	}
	if in.Analisi.Letta && in.Fonte != nil && in.Fonte.Analisi == (ChiaveAnalisi{}) {
		return MotivoAsseIngressoIncoerente
	}
	return ""
}

// monoPezzoNonApplicabile: una dichiarazione mono_pezzo che la conferma non regge (D-P4A-02 A, D-P4B1-02 A): motivo vuoto,
// archi certificati, radice che non è un finito o senza lo STEP autorizzato registrato. Il ramo vale solo così; che la
// fonte sia corrente e non superata lo dice K-02, che resta. "" se non dichiara o se la dichiarazione regge.
func monoPezzoNonApplicabile(c *ConfermaStrutturale) string {
	if !c.MonoPezzo {
		return ""
	}
	if strings.TrimSpace(c.MonoPezzoMotivo) == "" || len(c.Archi) > 0 {
		return MotivoAsseMonoPezzoNonApplicabile
	}
	for _, k := range c.Componenti {
		if k.Radice && (k.Tipo != tipoFinito || k.StepSha256 == "") {
			return MotivoAsseMonoPezzoNonApplicabile
		}
	}
	return ""
}

// derogaApplicabile: la deroga_struttura che vale per lo STEP della fonte, il suo sha e la sua analisi (D-SHOCK-S1 B; la
// stessa identità della vista v_step_prodotto della 0020: documento corrente, analisi con la stessa versione, la stessa
// configurazione e lo stesso calcolato_il). La prima che vale, nell'ordine del chiamante, è applicata; senza, l'esito è
// il motivo della prima che non vale (la più recente), o assente. Senza la fonte, nessuna deroga può valere.
//
// La chiave d'analisi vuota è la rappresentazione della 0020 per lo STEP non analizzato (le tre colonne NULL insieme;
// la deroga vale finché nessuna analisi è corrente, e la prima analisi la fa decadere): il confronto della chiave intera
// la tratta già così. Una deroga con la chiave vuota e una fonte con un'analisi (anche illeggibile) è altra_analisi; una
// deroga con una chiave vera e una fonte senza analisi pure. Lo sha cambiato dà altro_step: la deroga vecchia non vale
// mai in automatico per lo STEP nuovo (direttiva 12 §2.1 [U]; addendum ad A1c AD-4). I campi diagnostici della deroga
// ([P], P-03) escono nell'esito e non cambiano la validità.
func derogaApplicabile(deroghe []DerogaStruttura, fonte *FonteDellaVerifica) *EsitoDeroga {
	if len(deroghe) == 0 || fonte == nil {
		return &EsitoDeroga{Stato: DerogaAssente}
	}
	var prima *EsitoDeroga
	for i := range deroghe {
		d := &deroghe[i]
		stato := DerogaApplicata
		switch {
		case d.StepDocumentoID != fonte.DocumentoID || d.StepSha256 != fonte.Sha256:
			stato = DerogaAltroStep
		case d.Stato != StatoDerogaValida:
			stato = DerogaScaduta
		case d.Analisi != fonte.Analisi:
			stato = DerogaAltraAnalisi
		case strings.TrimSpace(d.Motivo) == "":
			stato = DerogaSenzaMotivo
		}
		e := &EsitoDeroga{Stato: stato, ID: copiaUUID(&d.ID), Motivo: d.Motivo, MotivoParziale: d.MotivoParziale, ConcessaIl: d.ConcessaIl}
		if d.ConcessaDa != uuid.Nil {
			e.ConcessaDa = copiaUUID(&d.ConcessaDa)
		}
		if stato == DerogaApplicata {
			return e
		}
		if prima == nil {
			prima = e
		}
	}
	return prima
}

// provenienzaDellArco: la provenienza di un arco certificato, mai dedotta (direttiva 12 §4 [U]; addendum ad A1c AD-5b):
//   - origine step o utente: è il valore registrato dall'azione (R5 lo scrive solo dall'azione, formato 2), e vale lei;
//     una Provenienza ricostruita diversa la contraddice: indeterminata [T];
//   - origine di trasporto (conferma, registrato), NULL o un valore che la regola non conosce: la Provenienza ricostruita
//     dalla lettura, se è una provenienza determinata (provenienzeDeterminate), con il suo ProvenienzaDa; altrimenti
//     indeterminata. Mai manuale per difetto: «Non presentare un arco come manuale per il solo fatto che la provenienza
//     è NULL» (direttiva 12 §4).
//
// Restituisce la provenienza e da dove viene («origine», o il ProvenienzaDa della lettura).
func provenienzaDellArco(a ArcoConfermato) (string, string) {
	switch a.Origine {
	case ProvenienzaStep, ProvenienzaUtente:
		if a.Provenienza != "" && a.Provenienza != a.Origine {
			return ProvenienzaIndeterminata, ""
		}
		return a.Origine, "origine"
	}
	if provenienzeDeterminate[a.Provenienza] {
		return a.Provenienza, a.ProvenienzaDa
	}
	return ProvenienzaIndeterminata, ""
}

// origineDellaGerarchia: l'aggregato della provenienza degli archi certificati (precisazione c; addendum ad A1c AD-5b
// punto 4; direttiva 12 §4 [U]: «La classificazione aggregata origine_gerarchia deve restare coerente con la provenienza
// degli archi»), con i contatori per provenienza e la provenienza di ogni arco ([P], P-05):
//   - vuota senza archi;
//   - indeterminata [P] (D-ADD-03 a1, provvisorio) con almeno un arco di provenienza indeterminata;
//   - step se tutti gli archi vengono dallo STEP; manuale se tutti vengono da una persona;
//   - mista solo con archi dello STEP e archi di una persona, e niente altro;
//   - indeterminata [P] (D-ADD-03 a2, provvisorio) con almeno un arco di un produttore che non è né lo STEP né una persona
//     (codice_rilevato), da solo o accanto ad altri: il vocabolario step | manuale | mista non ha un valore per lui;
//     step lo nasconderebbe e mista direbbe «in parte dallo STEP, in parte a mano», cioè attribuirebbe a una persona
//     archi che non sono di nessuna persona (direttiva 12 §4 [U]: «nessuna evidenza venga falsamente attribuita allo STEP
//     o a un intervento manuale»). I contatori dicono il produttore [T] T-21 (M-3 della revisione della rev. 1).
//
// Un arco di provenienza indeterminata non toglie la verifica: la certezza della struttura confermata e quella della sua
// provenienza restano distinte (direttiva 12 §4, «Obiettivo»).
func origineDellaGerarchia(archi []ArcoConfermato) (OrigineGerarchia, map[string]int, []ProvenienzaArco) {
	if len(archi) == 0 {
		return "", nil, nil
	}
	conti := map[string]int{}
	elenco := make([]ProvenienzaArco, 0, len(archi))
	for _, a := range archi {
		p, da := provenienzaDellArco(a)
		conti[p]++
		elenco = append(elenco, ProvenienzaArco{Arco: RifArco(ancoraggio.RifComponente(a.PadreID), ancoraggio.RifComponente(a.FiglioID)),
			Origine: a.Origine, Provenienza: p, Da: da})
	}
	sort.Slice(elenco, func(i, j int) bool { return elenco[i].Arco < elenco[j].Arco })
	step, utente := conti[ProvenienzaStep], conti[ProvenienzaUtente]
	switch {
	case step+utente != len(archi):
		// un arco indeterminato, o di un produttore che non è né lo STEP né una persona
		return OrigineGerarchiaIndeterminata, conti, elenco
	case utente == 0:
		return OrigineGerarchiaStep, conti, elenco
	case step == 0:
		return OrigineGerarchiaManuale, conti, elenco
	default:
		return OrigineGerarchiaMista, conti, elenco
	}
}

// archiCorrentiPerCoppia: le relazioni correnti per coppia (padre, figlio), con le qta sommate.
func archiCorrentiPerCoppia(archi []ArcoCorrente) map[[2]uuid.UUID]int {
	out := map[[2]uuid.UUID]int{}
	for _, a := range archi {
		out[[2]uuid.UUID{a.PadreID, a.FiglioID}] += a.Qta
	}
	return out
}

func rifDegliArchiCorrenti(correnti map[[2]uuid.UUID]int) []string {
	var out []string
	for k := range correnti {
		out = append(out, RifArco(ancoraggio.RifComponente(k[0]), ancoraggio.RifComponente(k[1])))
	}
	return ordinatiUnici(out)
}

// cambiamentiDopoLaConferma: le differenze fra gli archi certificati e quelli correnti, nei due versi (precisazione e:
// anche un arco tolto toglie la verifica) e con la qta (precisazione b: RifArco non la porta), in ordine.
func cambiamentiDopoLaConferma(certificati []ArcoConfermato, correnti map[[2]uuid.UUID]int) []string {
	prima := map[[2]uuid.UUID]int{}
	for _, a := range certificati {
		prima[[2]uuid.UUID{a.PadreID, a.FiglioID}] += a.Qta
	}
	rif := func(k [2]uuid.UUID) string {
		return RifArco(ancoraggio.RifComponente(k[0]), ancoraggio.RifComponente(k[1]))
	}
	var out []string
	for k, q := range prima {
		switch qc, ok := correnti[k]; {
		case !ok:
			out = append(out, "tolto:"+rif(k))
		case qc != q:
			out = append(out, "qta:"+rif(k)+":"+strconv.Itoa(q)+"->"+strconv.Itoa(qc))
		}
	}
	for k := range correnti {
		if _, ok := prima[k]; !ok {
			out = append(out, "aggiunto:"+rif(k))
		}
	}
	sort.Strings(out)
	return out
}

// ---- le condizioni nuove di un prodotto con una conferma ----

// condizioniNuoveDallaConferma: l'ingresso CondizioniNuove di StatoDelProdotto (A1c T-E1-14) per un prodotto con una
// conferma strutturale, dall'uscita della regola della conferma e non da quella di A1c (R-02 della revisione di B3;
// consegna a B3.5, addendum ad A1c AD-4c). È il gemello di condizioniNuoveDellaBOM, che resta per i prodotti senza
// conferma: se B3.5 innestasse VerificaDallaConferma lasciando l'adattatore di A1c, lo stato del prodotto e la verifica
// della BOM direbbero due cose diverse (per esempio la deroga mancante, che A1c non conosce).
//
// La direttiva 12 §3.5 [U] supera T-17 (nessun motivo nuovo era una condizione nuova): «Distinguere le condizioni nuove
// sopravvenute da quelle che impedivano già la prima verifica»; «Mantenere non_pronto per una BOM che non possiede ancora
// i requisiti della prima conferma»; «Prevedere da_riesaminare per una BOM già confermata quando arrivano nuove evidenze
// pertinenti e irrisolte»; «Non mascherare come semplice riesame una conferma incoerente, una deroga invalida o un errore
// d'integrità». La regola dell'adattatore, quindi: si tolgono dall'ingresso le condizioni nuove e si guarda che cosa
// resta. Un asse è fermo solo per condizioni nuove se, tolte quelle, è verificato (o, per la gerarchia, fermo solo per la
// fonte superata o per la nomenclatura della radice che è a sua volta ferma solo per condizioni nuove). Le condizioni
// nuove che si tolgono (addendum AD-4c, tabella):
//   - i conflitti aperti (T-E1-14);
//   - le righe aperte degli STEP secondari sopravvenute dopo l'ultima conferma valida (RigaSecondariaAperta.Sopravvenuta:
//     evidenza nuova, direttiva 12 §3.3 e §3.5); le righe già presenti alla conferma restano e tengono l'asse fermo;
//   - le voci della struttura principale da decidere, quando sono tutte del nuovo STEP dopo una fonte superata
//     (vociNuoveDopoLaFonteSuperataR111, R111 A, invariata), asse per asse: il gesto le copre e la regola si rifà.
//
// Quello che resta non è mai una condizione nuova, e il prodotto resta non_pronto: un requisito della prima conferma
// (senza_figli_non_dichiarato, criterio_conferma_nessuno, deroga_struttura_assente, _senza_motivo, _altro_step, voci
// vecchie da decidere, righe secondarie vecchie), una deroga invalida (deroga_struttura_scaduta, _altra_analisi: D-ADD-05
// parte 2 aperta, raccomandata A = questo comportamento), la BOM modificata dopo la conferma (struttura_cambiata_dopo_
// conferma, mono_pezzo_decaduto, conferma_non_valida: anche quella non valida per la sola sostituzione della fonte, finché
// D-ADD-05 parte 1 non ha una risposta [T]), un errore d'integrità (conferma_incoerente, ingresso_incoerente,
// mono_pezzo_non_applicabile, radice_step_non_mappata). Togliere le condizioni nuove e rifare la regola, invece di leggere
// il solo primo motivo, fa sì che una mancanza vecchia non resti nascosta dietro un da_decidere nuovo (T-E1-14: «le
// mancanze indipendenti già presenti non devono essere nascoste») [T].
//
// Limite dichiarato [T] T-26 (M-4 della revisione della rev. 1; domanda D-ADD-05 parte 3 dell'addendum, senza risposta):
// le voci da decidere della fonte principale nate dopo la conferma da una rianalisi dello stesso STEP (analizzatore o
// configurazione nuovi, fonte non superata) non si distinguono da quelle che c'erano già: l'ingresso non porta il segno
// «sopravvenuta» per loro, e restano un requisito, quindi non_pronto (il verso prudente). Se la parte 3 diventa A, la
// lettura (B3.1) porterà il segno anche per loro e qui si toglieranno come le righe secondarie sopravvenute.
//
// Senza conferma (in.Conferma nil) nessun asse è fermo solo per condizioni nuove: per quei prodotti vale
// condizioniNuoveDellaBOM con il gesto legacy.
func condizioniNuoveDallaConferma(pv ProdottoValutato, in IngressoVerificaConferma, strutture []ancoraggio.StrutturaProdotto, t fotorfq.Thread) CondizioniNuove {
	if in.Conferma == nil {
		return CondizioniNuove{}
	}
	senza := in
	senza.Struttura.Conflitti = nil
	senza.SecondarieAperte = nil
	for _, r := range in.SecondarieAperte {
		if !r.Sopravvenuta {
			senza.SecondarieAperte = append(senza.SecondarieAperte, r)
		}
	}
	_, g, s := verificaDallaConferma(senza, nil)
	var coperti []string
	if vociNom, vociGer := vociDaDecidere(g, s); g.Nomenclatura != nil {
		if len(vociNom) > 0 && vociNuoveDopoLaFonteSuperataR111(vociNom, pv, strutture, t) {
			coperti = append(coperti, vociNom...)
		}
		if len(vociGer) > 0 && vociNuoveDopoLaFonteSuperataR111(vociGer, pv, strutture, t) {
			coperti = append(coperti, vociGer...)
		}
	}
	resto, _, _ := verificaDallaConferma(senza, coperti)
	var out CondizioniNuove
	out.Nomenclatura = resto.Nomenclatura.Stato == StatoAsseVerificata
	switch {
	case resto.Gerarchia.Stato == StatoAsseVerificata:
		out.Gerarchia = true
	case resto.Gerarchia.Stato == StatoAsseDaVerificare && resto.Gerarchia.Motivo == MotivoAsseFonteNonConfermata:
		out.Gerarchia = fonteSuperata(pv.Fonte)
	case resto.Gerarchia.Stato == StatoAsseDaVerificare && resto.Gerarchia.Motivo == MotivoAsseNomenclaturaRadice:
		out.Gerarchia = out.Nomenclatura
	}
	return out
}
