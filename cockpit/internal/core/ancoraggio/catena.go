package ancoraggio

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// La catena del codice di un nodo (piano A, 6.0.6, 6.4.5; contratto §1.4, §2.2, §2.5, §2.6; commit P6b, B4, fase 1):
// grezzo → lettura → identità → codice proposto → codice manuale → codice confermato, ognuno al suo posto, mai uno al
// posto dell'altro (I-6; R86, R87). Il grezzo non cambia mai; la lettura è quella della grammatica del cliente; il
// codice proposto lo compone valutazione con motorea.ComponiCodiceDocumentale e arriva qui già fatto
// (ContestoStrutturale.CodiciProposti, T-08); le decisioni stanno accanto, mai fuse (R95 A, T-E1-05). Il codice
// documentale del cartiglio di un 2D associato e la sua riconciliazione (Documentale) li aggiunge ProponiAncoraggi,
// dopo gli ancoraggi dei file (riconciliazione.go, fase 3).

// ---- i tipi della catena ----

// CatenaCodice: la catena del codice di un nodo proposto (contratto §2.2; 6.0.6, «grezzo, lettura, proposto,
// confermato»).
//   - Grezzo: il valore com'è nel file, con il suo localizzatore: quello del campo della lettura scelta, o l'id del
//     nodo (poi il nome) quando nessuna lettura è scelta. Mai normalizzato (I-6).
//   - Lettura, Forma, MotivoLettura: l'ID della lettura d'identità scelta (locale al documento del file), la sua forma
//     (motorea.LetturaForma, che non cambia: D2) e, quando nessuna lettura è scelta, il perché (MotivoLettura*).
//   - Identita: l'identità del nodo con le parti separate (R86, R87).
//   - Proposto, MotivoProposto: il codice nella forma documentale della famiglia (R63 B), dal compositore; "" vuol dire
//     «non determinato», e il motivo lo dice (motorea.MotivoComposizione*, MotivoPropostoNonComposto o, senza
//     lettura, il MotivoLettura). Una stringa non si inventa mai (T-B0-37).
//   - Manuale: la correzione dell'operatore su una riga aperta (origine_codice = operatore), senza chi né quando
//     (T-B0-33, LD-13). Confermato: il componente confermato legato al nodo per UUID, letto con la grammatica (R31 c),
//     con la provenienza della revisione (T-B0-34, T-E1-22, E1R); sulla BOM di lavoro, per la radice scelta senza una
//     decisione propria, il componente del target (T-B4-38).
//   - Documentale: i codici del cartiglio dei 2D associati, con la riconciliazione, in ordine di allegato (fase 3:
//     li calcola ProponiAncoraggi; ProponiStrutture, che non vede i file, lo lascia vuoto).
type CatenaCodice struct {
	Grezzo         ValoreGrezzo          `json:"grezzo"`
	Lettura        string                `json:"lettura,omitempty"`
	Forma          *motorea.LetturaForma `json:"forma,omitempty"`
	MotivoLettura  string                `json:"motivo_lettura,omitempty"`
	Identita       IdentitaNodo          `json:"identita"`
	Proposto       string                `json:"proposto"`
	MotivoProposto string                `json:"motivo_proposto,omitempty"`
	Manuale        *CodiceDeciso         `json:"manuale,omitempty"`
	Confermato     *CodiceDeciso         `json:"confermato,omitempty"`
	Documentale    []CodiceDocumentale   `json:"documentale,omitempty"`
}

// ValoreGrezzo: il grezzo con il suo localizzatore (contratto §2.2): il testo del campo com'è nei fatti (id o nome del
// PRODUCT), il campo, l'unità del documento e la posizione. Vuoto se il nodo non ha né id né nome.
type ValoreGrezzo struct {
	Testo     string                 `json:"testo"`
	Campo     string                 `json:"campo,omitempty"`
	UnitaID   string                 `json:"unita_id,omitempty"`
	Posizione evidenze.Localizzatore `json:"posizione,omitzero"`
}

// I campi del grezzo (ValoreGrezzo.Campo): l'id e il nome del PRODUCT (contratto §2.2).
const (
	CampoGrezzoID   = "id"
	CampoGrezzoNome = "nome"
)

// I motivi di una catena senza lettura scelta (CatenaCodice.MotivoLettura; lettura dell'orchestratore T-B4-01):
//   - nessuna_lettura: nessuna famiglia legge l'id o il nome del nodo (le saldature, D10);
//   - letture_discordanti: le letture d'identità del nodo non danno tutte la stessa identità (famiglia, namespace,
//     base, marcatore, revisione), per esempio un id e un nome che non concordano (motorea, punto 11): nessuna si
//     sceglie (T-E1-07, «a parità c'è ambiguo, mai una scelta»).
//
// Lo stesso valore va in MotivoProposto (senza lettura niente codice proposto) e nel Motivo dell'identità.
const (
	MotivoLetturaAssente     = "nessuna_lettura"
	MotivoLettureDiscordanti = "letture_discordanti"
)

// MotivoPropostoNonComposto: c'è la lettura, ma il contesto non porta il suo codice composto (valutazione non l'ha
// calcolato, per esempio senza il motore del cliente): niente stringa, e la parzialità dell'identità non si dice
// (T-08; lettura dell'orchestratore T-B4-02).
const MotivoPropostoNonComposto = "non_composto"

// IdentitaNodo: l'identità del nodo con le parti separate, sopra la lettura, che non cambia (R86, R87; T-B0-37;
// contratto §2.2, §2.5).
//   - Base, Marcatore, Revisione: le parti della lettura scelta. Revisione nil vuol dire «non determinata»: nessuna
//     revisione si inventa, nemmeno da un candidato.
//   - Parziale: la forma documentale della famiglia chiede o ammette una revisione che la lettura non ha (T-B3-01): il
//     compositore lo dice con il motivo revisione_non_determinata, e allora non c'è nessuna stringa canonica (R87).
//   - Provenienza: radice_step per una radice del file, nodo_step per gli altri nodi (6.0.6, il selettore).
//   - Qualita: la Qualita della lettura scelta.
//   - Motivo: perché l'identità non è completa, il primo che vale: senza lettura il MotivoLettura; poi
//     lettura_non_completa (base parziale o lettura discordante: A-C07, D2), revisione_ambigua (la revisione c'è ma
//     non è letta: D5), revisione_non_determinata (Parziale). "" per un'identità completa.
//   - CandidatiRevisione: gli indizi di revisione, ognuno legato alla sua entità, mai propagati ai figli, mai una
//     decisione (R87, T-E1-06); quello del cartiglio di un 2D associato lo aggiunge ProponiAncoraggi (fase 3).
//     FontiSenzaRevisione: le fonti d'indizio che ci sono, ma senza una revisione letta: il candidato non c'è, con il
//     motivo (T-E1-06).
//   - StatoRevisione: confermata solo da una decisione del modello nuovo (DecisioneIdentita sul componente: E1R); sui
//     dati veri non compare, e componente.rev del legacy non la produce (R97 B). Candidata: una revisione c'è, ma
//     solo proposta (dalla lettura del nodo o dai candidati, anche del cartiglio). Assente: nessuna (statoRevisione).
type IdentitaNodo struct {
	Base                string                `json:"base,omitempty"`
	Marcatore           string                `json:"marcatore,omitempty"`
	Revisione           *string               `json:"revisione,omitempty"`
	Parziale            bool                  `json:"parziale"`
	Provenienza         string                `json:"provenienza"`
	Qualita             string                `json:"qualita,omitempty"`
	Motivo              string                `json:"motivo,omitempty"`
	CandidatiRevisione  []CandidatoRevisione  `json:"candidati_revisione,omitempty"`
	FontiSenzaRevisione []FonteSenzaRevisione `json:"fonti_senza_revisione,omitempty"`
	StatoRevisione      string                `json:"stato_revisione"`
}

// I valori di IdentitaNodo.Provenienza (contratto §2.2).
const (
	ProvenienzaNodoSTEP   = "nodo_step"
	ProvenienzaRadiceSTEP = "radice_step"
)

// I valori di IdentitaNodo.StatoRevisione (contratto §2.5).
const (
	StatoRevisioneAssente    = "assente"
	StatoRevisioneCandidata  = "candidata"
	StatoRevisioneConfermata = "confermata"
)

// CandidatoRevisione: un indizio di revisione, evidenza secondaria, mai una decisione (R87; contratto §2.2, §2.5;
// T-E1-06). Ognuno porta la sua entità (Entita: il Rif del prodotto, del componente confermato o del nodo a cui si
// riferisce) e non passa mai ai figli.
//   - Valore: la revisione letta dalla grammatica nella fonte, com'è (zeri compresi: D4).
//   - Fonte: nome_file_step (il nome del file STEP, solo per una radice del file: T-B0-27, T-B0-35), cartiglio (il
//     cartiglio di un 2D associato, per l'entità del nodo: fase 3), codice_target (il codice del target confermato,
//     per il prodotto), messaggio (un codice letto dal motore A nel testo di un messaggio del thread).
//   - AllegatoID, MessaggioID: il file o il messaggio da cui viene; nil quando non c'è (il codice del target viene da
//     una riga del DB). Posizione: il localizzatore dell'occorrenza o del campo; vuoto per il codice del target.
type CandidatoRevisione struct {
	Valore      string                 `json:"valore"`
	Fonte       string                 `json:"fonte"`
	AllegatoID  *uuid.UUID             `json:"allegato_id,omitempty"`
	MessaggioID *uuid.UUID             `json:"messaggio_id,omitempty"`
	Posizione   evidenze.Localizzatore `json:"posizione,omitzero"`
	Entita      string                 `json:"entita"`
}

// I valori di CandidatoRevisione.Fonte (contratto §2.2, §2.5).
const (
	FonteRevisioneNomeFileSTEP = "nome_file_step"
	FonteRevisioneCartiglio    = "cartiglio"
	FonteRevisioneCodiceTarget = "codice_target"
	FonteRevisioneMessaggio    = "messaggio"
)

// FonteSenzaRevisione: una fonte d'indizio che riguarda l'entità, ma da cui la grammatica non legge una revisione: il
// candidato non c'è, con il motivo (T-E1-06; T-B4-03, ratificata dalla revisione: campo in più di IdentitaNodo). Fonte,
// Entita, AllegatoID e MessaggioID come in CandidatoRevisione.
type FonteSenzaRevisione struct {
	Fonte       string     `json:"fonte"`
	Entita      string     `json:"entita"`
	AllegatoID  *uuid.UUID `json:"allegato_id,omitempty"`
	MessaggioID *uuid.UUID `json:"messaggio_id,omitempty"`
	Motivo      string     `json:"motivo"`
}

// I motivi di FonteSenzaRevisione (T-B4-03, ratificata; T-B4-06 rivisto):
//   - revisione_non_letta: la fonte ha l'identità dell'entità (stessaIdentita), ma nessuna revisione;
//   - revisione_ambigua: la revisione c'è, ma non è letta (un token sospeso: D5);
//   - marcatore_diverso: la fonte ha lo stesso namespace e la stessa base completa, ma un altro marcatore, scritto da
//     tutte e due le parti: è un'altra identità (R86), e la sua revisione non è dell'entità;
//   - nome_non_letto: la grammatica non legge il nome del file STEP nel namespace della radice;
//   - nome_discordante: il nome del file STEP ha un'altra base della radice: fra la radice e il nome la discordanza si
//     guarda solo su base e marcatore, e il nome non è un indizio della radice (T-B0-35).
const (
	MotivoSenzaRevisioneNonLetta         = "revisione_non_letta"
	MotivoSenzaRevisioneAmbigua          = "revisione_ambigua"
	MotivoSenzaRevisioneMarcatoreDiverso = "marcatore_diverso"
	MotivoSenzaRevisioneNomeNonLetto     = "nome_non_letto"
	MotivoSenzaRevisioneNomeDiscorde     = "nome_discordante"
)

// CodiceDeciso: una decisione sul codice accanto alla proposta, mai fusa con lei (contratto §2.2, §2.5, §2.6).
//   - ComponenteID: il componente, per il codice confermato; nil per il codice manuale di una riga aperta.
//   - Codice, Rev: il codice e la revisione decisi, com'è nel DB o nella decisione; Rev nil = nessuna revisione
//     registrata.
//   - Base: la base del codice deciso letta con la grammatica del cliente (R31 c); vuota se non si legge.
//   - Origine: confermato (componente.codice, o una DecisioneIdentita sul componente) o manuale (T-B0-33, T-17).
//   - RevProvenienza: da dove viene Rev, quando c'è. non_registrata: il legacy non lo dice, e può averci messo la
//     formazione STEP (T-B0-34); coincide_con_formazione_step: componente.rev è uguale alla formazione di almeno un
//     nodo STEP la cui riga legacy decisa porta quel componente (T-E1-22: un indizio, che non dimostra niente; il
//     trattamento resta R97 B); decisione_tracciata: codice e revisione vengono da una DecisioneIdentita sul
//     componente (E1R, T-E1R-08), e allora vale R95 A, non R97 B; anche senza revisione, perché l'assenza è decisa
//     (Rev nil: T-B4-21). Per i valori registrati, "" senza revisione.
type CodiceDeciso struct {
	ComponenteID   *uuid.UUID        `json:"componente_id,omitempty"`
	Codice         string            `json:"codice"`
	Rev            *string           `json:"rev,omitempty"`
	Base           motorea.BaseLetta `json:"base"`
	Origine        OrigineDato       `json:"origine"`
	RevProvenienza string            `json:"rev_provenienza,omitempty"`
}

// I valori di CodiceDeciso.RevProvenienza (contratto §2.2, §2.5, §2.6).
const (
	RevProvenienzaNonRegistrata      = "non_registrata"
	RevProvenienzaFormazioneSTEP     = "coincide_con_formazione_step"
	RevProvenienzaDecisioneTracciata = "decisione_tracciata"
)

// ---- i tipi della riconciliazione (il campo Documentale) ----

// EsitoRiconciliazione: il confronto fra il codice documentale del cartiglio e quello proposto dallo STEP (R64, R87;
// T-B0-11, T-B0-27; contratto §1.4, §2.2). Il tipo nasce con la catena perché la catena ne ha il campo; la regola è
// confrontaConLoSTEP (riconciliazione.go, fase 3 di B4):
//   - concorda: stessa base, marcatore compatibile, stessa revisione;
//   - completamento_proposto: base e marcatore concordano, e il cartiglio aggiunge la revisione che lo STEP non dà;
//   - correzione_proposta: il codice documentale è diverso (base, marcatore scritto o revisione);
//   - discordante: il file ha identità che non concordano, o il cartiglio più letture diverse, senza scelta (R64 A);
//   - non_verificabile: nessun codice documentale da confrontare (un raster, un PDF senza testo: LD-04), con il motivo.
type EsitoRiconciliazione string

const (
	RiconciliazioneConcorda              EsitoRiconciliazione = "concorda"
	RiconciliazioneCompletamentoProposto EsitoRiconciliazione = "completamento_proposto"
	RiconciliazioneCorrezioneProposta    EsitoRiconciliazione = "correzione_proposta"
	RiconciliazioneDiscordante           EsitoRiconciliazione = "discordante"
	RiconciliazioneNonVerificabile       EsitoRiconciliazione = "non_verificabile"
)

// CodiceDocumentale: il codice letto dal cartiglio di un 2D associato al nodo, con la riconciliazione (contratto §1.4,
// §2.2; oggi solo dai PDF: LD-04). Associazione e riconciliazione restano due campi (T-B0-11): l'associazione del file
// sta nel suo ancoraggio (AncoraggioFile.Associazione).
//   - AllegatoID, DocumentoID: il 2D; il documento solo con un'associazione decisa che lo porta (la confermata).
//   - OrigineAssociazione: proposto (un candidato del file con una posizione sul nodo), manuale o confermato (le
//     associazioni decise sul componente del nodo, AssociazioneDecisa); la più forte. Associazione (campo in più,
//     lettura T-B4-39): lo stato dell'associazione del file, quello del suo ancoraggio (candidato_unico, ambiguo,
//     discordante, nessun_candidato), perché B5 e B6 vedano accanto al codice documentale quanto è certo che il 2D sia
//     del nodo.
//   - Lettura, Originale, Base, Revisione, UnitaID, Posizione: la lettura di cartiglio.codice scelta, il testo grezzo
//     del campo, la base e la revisione della lettura, l'unità e il suo localizzatore; vuoti senza una lettura scelta.
//   - Esito, Motivo: la riconciliazione con l'identità del nodo proposta dallo STEP, con il motivo per non_verificabile
//     e discordante (MotivoDocumentale*, letture_discordanti, identita_discordanti).
//   - Correzione: la correzione o il completamento accanto, per correzione_proposta e completamento_proposto, e per
//     discordante quando il cartiglio propone un codice (R64 A: «con la correzione accanto»); mai applicata.
//   - Discordanze (campo in più, letture T-B4-34 e T-B4-40): i segnali della discordanza con le decisioni del nodo,
//     uno per decisione contraddetta (il codice confermato e il codice manuale sono due decisioni, e nessuna precedenza
//     ne nasconde una), con le evidenze dei due lati (T-E1-15), in ordine di origine della decisione (confermato, poi
//     manuale); vuoto senza decisioni o senza discordanze.
type CodiceDocumentale struct {
	AllegatoID          uuid.UUID               `json:"allegato_id"`
	DocumentoID         *uuid.UUID              `json:"documento_id,omitempty"`
	OrigineAssociazione OrigineDato             `json:"origine_associazione"`
	Associazione        Associazione            `json:"associazione"`
	Lettura             string                  `json:"lettura,omitempty"`
	Originale           string                  `json:"originale,omitempty"`
	Base                motorea.BaseLetta       `json:"base"`
	Revisione           *motorea.RevisioneLetta `json:"revisione,omitempty"`
	UnitaID             string                  `json:"unita_id,omitempty"`
	Posizione           evidenze.Localizzatore  `json:"posizione,omitzero"`
	Esito               EsitoRiconciliazione    `json:"esito"`
	Correzione          *CorrezioneProposta     `json:"correzione,omitempty"`
	Motivo              string                  `json:"motivo,omitempty"`
	Discordanze         []DiscordanzaDecisione  `json:"discordanze,omitempty"`
}

// CorrezioneProposta: la correzione o il completamento proposti dal cartiglio, mai applicati (R64 A, R87; contratto
// §2.2, §2.5): lo dice CodiceDocumentale.Esito. Codice: il codice com'è letto nel cartiglio; Base, Revisione: della
// lettura; AllegatoID, UnitaID, Posizione: la provenienza (6.0.6: il file, l'unità, il localizzatore).
// RevisioneInferiore: la revisione proposta è minore di quella decisa, il codice confermato o manuale del nodo (T-E1-20;
// revisioneInferiore): resta una proposta; contro una decisione tracciata è il segnale di un conflitto (R95 A), contro
// componente.rev del legacy un indicatore (R97 B), come dicono CodiceDocumentale.Discordanze: vero se la revisione è
// minore di almeno una delle revisioni decise.
type CorrezioneProposta struct {
	Codice             string                 `json:"codice"`
	Base               motorea.BaseLetta      `json:"base"`
	Revisione          *string                `json:"revisione,omitempty"`
	AllegatoID         uuid.UUID              `json:"allegato_id"`
	UnitaID            string                 `json:"unita_id,omitempty"`
	Posizione          evidenze.Localizzatore `json:"posizione,omitzero"`
	RevisioneInferiore bool                   `json:"revisione_inferiore"`
}

// ---- gli ingressi della catena ----

// LetturaConPosizione: una lettura del motore A con il localizzatore del campo o dell'occorrenza da cui viene. Serve
// al nome del file STEP (StrutturaFile.NomeFile).
type LetturaConPosizione struct {
	Lettura   motorea.LetturaCodice  `json:"lettura"`
	Posizione evidenze.Localizzatore `json:"posizione"`
}

// CodiceDiMessaggio: un codice letto dal motore A nel testo di un messaggio del thread, senza LLM (T-E1-06), con il
// messaggio e il localizzatore dell'occorrenza: un indizio di revisione per le entità con la stessa identità, mai una
// decisione. Lo prepara valutazione dalle interpretazioni dei messaggi (le stesse di ProponiProdotti), con la
// posizione dell'occorrenza (Assoluto, o la posizione dell'unità): la forma più semplice (lettura dell'orchestratore
// T-B4-04).
type CodiceDiMessaggio struct {
	MessaggioID uuid.UUID              `json:"messaggio_id"`
	Lettura     motorea.LetturaCodice  `json:"lettura"`
	Posizione   evidenze.Localizzatore `json:"posizione"`
}

// ChiaveCodiceProposto: la chiave di ContestoStrutturale.CodiciProposti, «<bundle>:<lettura>». Gli ID delle letture
// sono locali al documento (par.3.4.2: l'identità completa è (bundle, ID locale)), e due STEP con le stesse chiavi
// danno le stesse unità: per questo la chiave porta il documento (T-B4-05, ratificata dalla revisione).
func ChiaveCodiceProposto(bundleID, letturaID string) string { return bundleID + ":" + letturaID }

// ---- il calcolo della catena ----

// letturaDelNodo sceglie la lettura d'identità di un nodo, senza inventare (T-B4-01). Le letture dei campi id e nome
// danno un'identità ciascuna (famiglia, namespace, base, marcatore, revisione): se a due a due danno la stessa identità
// con la regola unica del marcatore (lettureCompatibili: un marcatore scritto da una parte sola è compatibile, T-B4-06
// rivisto, T-B4-30), vale la precedenza di sempre, il campo id, poi l'ID minore; se due non la danno, nessuna si
// sceglie (letture_discordanti); senza letture, nessuna_lettura. Il confronto è a due a due: «A», nessun marcatore e
// «B» sono discordanti, perché «A» e «B» lo sono.
func letturaDelNodo(letture []motorea.LetturaCodice) (*motorea.LetturaCodice, string) {
	if len(letture) == 0 {
		return nil, MotivoLetturaAssente
	}
	for i := range letture {
		for j := i + 1; j < len(letture); j++ {
			if !lettureCompatibili(letture[i].Forma, letture[j].Forma) {
				return nil, MotivoLettureDiscordanti
			}
		}
	}
	var scelta *motorea.LetturaCodice
	for i := range letture {
		if l := &letture[i]; scelta == nil || precedeLettura(*l, *scelta) {
			scelta = l
		}
	}
	return scelta, ""
}

// lettureCompatibili: due letture danno la stessa identità: la stessa famiglia, lo stesso namespace, la stessa base
// normalizzata e la stessa revisione (stato e valore), e il marcatore compatibile con la regola unica (uguale, o scritto
// da una parte sola: T-B4-06 rivisto, T-B4-30).
func lettureCompatibili(a, b motorea.LetturaForma) bool {
	return chiaveSenzaMarcatore(a) == chiaveSenzaMarcatore(b) && marcatoriCompatibili(marcatoreDi(a), marcatoreDi(b))
}

// chiaveSenzaMarcatore: l'identità di una lettura senza il marcatore, che lettureCompatibili confronta a parte.
func chiaveSenzaMarcatore(f motorea.LetturaForma) string {
	k := f.Famiglia + "\x00" + f.Namespace + "\x00" + f.Base.Normalizzata + "\x00"
	if f.Revisione != nil {
		k += f.Revisione.Stato + "\x01" + f.Revisione.Normalizzata
	}
	return k
}

// precedeLettura: il campo id prima del nome, poi l'ID minore.
func precedeLettura(a, b motorea.LetturaCodice) bool {
	ia, ib := a.Forma.Selettore.Campo.Valore == CampoGrezzoID, b.Forma.Selettore.Campo.Valore == CampoGrezzoID
	if ia != ib {
		return ia
	}
	return a.ID < b.ID
}

// grezzoDelNodo: il grezzo del campo della lettura scelta; senza lettura l'id del nodo, poi il nome.
func grezzoDelNodo(grezzi []ValoreGrezzo, l *motorea.LetturaCodice) ValoreGrezzo {
	if l != nil {
		for _, g := range grezzi {
			if g.UnitaID == l.UnitaID {
				return g
			}
		}
	}
	for _, campo := range []string{CampoGrezzoID, CampoGrezzoNome} {
		for _, g := range grezzi {
			if g.Campo == campo {
				return g
			}
		}
	}
	return ValoreGrezzo{}
}

// revisioneLetta: la revisione letta di una lettura, se c'è ed è «letta».
func revisioneLetta(f motorea.LetturaForma) (string, bool) {
	if f.Revisione == nil || f.Revisione.Stato != motorea.StatoRevisioneLetta || f.Revisione.Normalizzata == "" {
		return "", false
	}
	return f.Revisione.Normalizzata, true
}

// letturaNonCompleta: la lettura non è un articolo intero (A-C07, D2): base parziale o lettura discordante, come la
// vede il compositore (motorea, motivoDellaLettura).
func letturaNonCompleta(f motorea.LetturaForma) bool {
	if !f.Base.Completa || len(f.Base.Mancanti) > 0 || len(f.Base.Segmenti) == 0 {
		return true
	}
	for _, r := range f.Ripetizioni {
		if !r.Concorda {
			return true
		}
	}
	return f.Stato != motorea.StatoCompleta && f.Stato != motorea.StatoDaVerificare
}

// nodoDellaCatena: ciò che serve alla catena di un nodo della struttura.
type nodoDellaCatena struct {
	t     ProdottoRichiesto
	f     *fileIndicizzato
	s     *StrutturaProdotto
	n     *NodoProposto
	letto NodoStruttura
}

// catena calcola la catena del codice di un nodo (6.0.6; contratto §1.4). La decisione accanto (Decisione,
// RigaLegacy) è già sul nodo.
func (c *contestoIndicizzato) catena(x nodoDellaCatena) CatenaCodice {
	l, motivo := letturaDelNodo(x.letto.Letture)
	cat := CatenaCodice{Grezzo: copiaGrezzo(grezzoDelNodo(x.letto.Grezzi, l)), MotivoLettura: motivo}
	cat.Identita.Provenienza = ProvenienzaNodoSTEP
	if x.f.radici[x.n.Rif] {
		cat.Identita.Provenienza = ProvenienzaRadiceSTEP
	}

	if l == nil {
		cat.MotivoProposto, cat.Identita.Motivo = motivo, motivo
	} else {
		forma := copiaLetturaForma(l.Forma)
		cat.Lettura, cat.Forma = l.ID, &forma
		id := &cat.Identita
		id.Base, id.Qualita = l.Forma.Base.Normalizzata, l.Qualita
		if l.Forma.Marcatore != nil {
			id.Marcatore = l.Forma.Marcatore.Valore
		}
		if r, ok := revisioneLetta(l.Forma); ok {
			id.Revisione = &r
		}
		composto, ok := c.codiciProposti[ChiaveCodiceProposto(x.f.s.BundleID, l.ID)]
		if ok {
			cat.Proposto, cat.MotivoProposto = composto.Testo, composto.Motivo
			// Parziale si ricava dal motivo del compositore (T-B3-01); la revisione mancante si legge dalla lettura.
			id.Parziale = composto.Testo == "" && composto.Motivo == motorea.MotivoComposizioneRevisioneNonDeterminata
		} else {
			cat.MotivoProposto = MotivoPropostoNonComposto
		}
		switch {
		case letturaNonCompleta(l.Forma):
			id.Motivo = motorea.MotivoComposizioneLetturaNonCompleta
		case l.Forma.Revisione != nil && id.Revisione == nil:
			id.Motivo = motorea.MotivoComposizioneRevisioneAmbigua
		case id.Parziale:
			id.Motivo = motorea.MotivoComposizioneRevisioneNonDeterminata
		case !ok && id.Revisione == nil:
			// Senza il codice composto non si sa se la forma documentale chiede la revisione: l'identità senza revisione
			// non si dice completa (T-B4-02).
			id.Motivo = MotivoPropostoNonComposto
		}
		id.CandidatiRevisione, id.FontiSenzaRevisione = c.candidati(x, l.Forma)
	}

	cat.Manuale = codiceManuale(x.n.RigaLegacy)
	cat.Confermato = c.codiceConfermato(x.n.Decisione)
	cat.Identita.StatoRevisione = statoRevisione(cat)
	return cat
}

// codiceManuale: il codice dell'operatore di una riga aperta (T-B0-33): origine manuale, senza chi né quando. La sua
// revisione, se c'è, ha la provenienza non registrata: la scrive l'operatore, e il modulo può avergli lasciato la
// formazione.
func codiceManuale(r *RigaPropostaLegacy) *CodiceDeciso {
	if r == nil || r.CodiceManuale == nil {
		return nil
	}
	m := &CodiceDeciso{Codice: *r.CodiceManuale, Rev: copiaTesto(r.RevManuale), Origine: OrigineManuale}
	if r.LetturaManuale != nil {
		m.Base = copiaBase(r.LetturaManuale.Base)
	}
	if conTesto(r.RevManuale) {
		m.RevProvenienza = RevProvenienzaNonRegistrata
	}
	return m
}

// codiceConfermato: il codice del componente confermato legato al nodo (R31 c). Con una DecisioneIdentita sul
// componente codice e revisione sono quelli della decisione, con la provenienza decisione_tracciata (E1R, T-E1R-08):
// il codice c'è sempre (un codice vuoto è un errore di contratto), la revisione vuota vuol dire «decisa senza
// revisione» e dà Rev nil, con la stessa provenienza (T-B4-21). La base è quella della lettura del codice deciso che
// valutazione mette nel contesto (ContestoStrutturale.LettureDecise, fase 3: T-B4-32), altrimenti quella della lettura
// del componente solo se il codice deciso è lo stesso (la grammatica qui non legge), altrimenti vuota. Senza decisione
// tracciata componente.codice e componente.rev, con la provenienza di T-E1-22.
func (c *contestoIndicizzato) codiceConfermato(d *ComponenteDeciso) *CodiceDeciso {
	if d == nil {
		return nil
	}
	id := d.ComponenteID
	k := &CodiceDeciso{ComponenteID: &id, Codice: d.Codice, Rev: copiaTesto(d.Rev), Origine: OrigineConfermato}
	if d.Lettura != nil {
		k.Base = copiaBase(d.Lettura.Base)
	}
	if dec, ok := c.decisioniComponente[d.ComponenteID]; ok {
		k.Codice, k.Rev, k.RevProvenienza = dec.Codice, nil, RevProvenienzaDecisioneTracciata
		if strings.TrimSpace(dec.Revisione) != "" {
			rev := dec.Revisione
			k.Rev = &rev
		}
		k.Base = motorea.BaseLetta{}
		if l := c.letturaDecisa(d); l != nil {
			k.Base = copiaBase(l.Base)
		}
		return k
	}
	if conTesto(d.Rev) {
		k.RevProvenienza = RevProvenienzaNonRegistrata
		if c.coincideConFormazione(d.ComponenteID, *d.Rev) {
			k.RevProvenienza = RevProvenienzaFormazioneSTEP
		}
	}
	return k
}

// coincideConFormazione (T-E1-22): la revisione registrata del componente è uguale (senza gli spazi ai bordi, come la
// copia il legacy) alla formazione di almeno un nodo STEP la cui riga legacy decisa porta quel componente. Delle righe
// decise legge solo il componente, lo sha256 e la chiave; la formazione viene dai fatti.
func (c *contestoIndicizzato) coincideConFormazione(componente uuid.UUID, rev string) bool {
	rev = strings.TrimSpace(rev)
	for _, nodo := range c.nodiDelComponente[componente] {
		for _, f := range c.formazioni[nodo] {
			if strings.TrimSpace(f) == rev {
				return true
			}
		}
	}
	return false
}

// ---- i candidati di revisione (T-E1-06) ----

// entitaDelNodo: l'entità che il nodo rappresenta in questa struttura, per i suoi indizi (T-E1-06): il prodotto per
// la radice della struttura con la base del target (regola 1), il componente confermato per un nodo con la decisione,
// altrimenti il nodo stesso.
func entitaDelNodo(x nodoDellaCatena) string {
	switch {
	case x.n.Rif == x.s.Radice && compatibile(x.s.Compatibilita):
		return x.t.Rif
	case x.n.Decisione != nil:
		return RifComponente(x.n.Decisione.ComponenteID)
	}
	return x.n.Rif
}

// esitoIdentita: come una fonte d'indizio sta rispetto all'identità di un'entità (T-B4-06 rivisto).
type esitoIdentita int

const (
	identitaDiversa          esitoIdentita = iota // un altro namespace, una base non completa o diversa: un'altra entità
	identitaStessa                                // lo stesso namespace, la stessa base completa, il marcatore compatibile
	identitaMarcatoreDiverso                      // lo stesso namespace e la stessa base, due marcatori scritti e diversi
)

// confrontaIdentita: la regola unica degli indizi di revisione (T-B4-06 rivisto: messaggi, nome del file, codice del
// target) e del candidato a livello prodotto (fase 2): lo stesso namespace, la stessa base completa (ConfrontaBasi
// uguale) e il marcatore compatibile, cioè uguale quando c'è da tutte e due le parti, oppure scritto da una parte sola.
// Due marcatori scritti e diversi sono un'altra identità (R86): una revisione di «A» non è di «B».
func confrontaIdentita(entita, fonte motorea.LetturaForma) esitoIdentita {
	if entita.Namespace != fonte.Namespace || motorea.ConfrontaBasi(entita.Base, fonte.Base) != motorea.CompatibilitaUguale {
		return identitaDiversa
	}
	if !marcatoriCompatibili(marcatoreDi(entita), marcatoreDi(fonte)) {
		return identitaMarcatoreDiverso
	}
	return identitaStessa
}

// marcatoriCompatibili: due marcatori sono compatibili se sono uguali o se uno dei due non è scritto (T-B4-06 rivisto).
// È la regola unica del marcatore (T-B4-06 rivisto, T-B4-30): la usano anche le identità raggiungibili, i
// candidati a livello prodotto e componente, le identità discordanti di un file, la compatibilità con le decisioni e le
// radici candidate (compatibilitaMarcatori dà lo stesso confronto come Compatibilita).
func marcatoriCompatibili(a, b string) bool {
	return compatibilitaMarcatori(a, b) != motorea.CompatibilitaDiscordante
}

// compatibilitaMarcatori: la regola del marcatore come Compatibilita, per DimensioneMarcatore (T-B4-30): uguale se sono
// scritti tutti e due e uguali, o se nessuno dei due è scritto; compatibile_parziale se è scritto da una parte sola;
// discordante se sono scritti tutti e due e diversi (R86: un altro marcatore è un'altra identità).
func compatibilitaMarcatori(a, b string) motorea.Compatibilita {
	switch {
	case a == b:
		return motorea.CompatibilitaUguale
	case a == "" || b == "":
		return motorea.CompatibilitaParziale
	}
	return motorea.CompatibilitaDiscordante
}

// conMarcatore: la compatibilità delle basi corretta con la regola del marcatore: due basi compatibili con due
// marcatori scritti e diversi sono discordanti (R86, T-B4-30). Un altro esito delle basi resta com'è.
func conMarcatore(basi motorea.Compatibilita, a, b string) motorea.Compatibilita {
	if compatibile(basi) && !marcatoriCompatibili(a, b) {
		return motorea.CompatibilitaDiscordante
	}
	return basi
}

// stessaIdentita: la fonte ha l'identità dell'entità (confrontaIdentita).
func stessaIdentita(entita, fonte motorea.LetturaForma) bool {
	return confrontaIdentita(entita, fonte) == identitaStessa
}

// letturaDelTarget: il codice del target come lettura da confrontare, con il marcatore che valutazione ha letto
// (ProdottoRichiesto.Marcatore) e la revisione.
func letturaDelTarget(t ProdottoRichiesto) motorea.LetturaForma {
	l := motorea.LetturaForma{Namespace: t.Namespace, Base: t.Base, Revisione: t.Revisione}
	if t.Marcatore != "" {
		l.Marcatore = &motorea.ParteLetta{Valore: t.Marcatore}
	}
	return l
}

// candidati: gli indizi di revisione del nodo, ognuno con la sua entità, mai propagati ai figli (T-E1-06), con la
// regola unica di confrontaIdentita (T-B4-06 rivisto):
//   - il nome del file STEP, solo per una radice del file (T-B0-27): una lettura del nome con l'identità della radice;
//     un nome con un'altra base non è un indizio della radice (nome_discordante, T-B0-35);
//   - il codice del target confermato, per il prodotto: solo sulla radice della struttura; lo scenario non è un
//     target confermato;
//   - i codici letti nel testo dei messaggi del thread, per ogni nodo con la stessa identità: più entità della stessa
//     base, un candidato per ognuna, mai una scelta.
//
// Una fonte con la stessa base e un altro marcatore scritto non dà il candidato: va fra le fonti senza revisione con
// marcatore_diverso. Una fonte senza revisione letta non lo dà nemmeno lei, con il suo motivo. Il cartiglio di un 2D
// associato lo aggiunge ProponiAncoraggi (candidatiDelCartiglio, fase 3). Gli elenchi sono in ordine canonico, senza
// doppioni.
func (c *contestoIndicizzato) candidati(x nodoDellaCatena, identita motorea.LetturaForma) ([]CandidatoRevisione, []FonteSenzaRevisione) {
	entita := entitaDelNodo(x)
	var cand []CandidatoRevisione
	var senza []FonteSenzaRevisione
	scarta := func(fonte string, allegato, messaggio *uuid.UUID) {
		senza = append(senza, FonteSenzaRevisione{Fonte: fonte, Entita: entita, AllegatoID: copiaUUID(allegato), MessaggioID: copiaUUID(messaggio),
			Motivo: MotivoSenzaRevisioneMarcatoreDiverso})
	}
	aggiungi := func(f motorea.LetturaForma, fonte string, allegato, messaggio *uuid.UUID, pos evidenze.Localizzatore) {
		switch r, ok := revisioneLetta(f); {
		case ok:
			cand = append(cand, CandidatoRevisione{Valore: r, Fonte: fonte, AllegatoID: copiaUUID(allegato), MessaggioID: copiaUUID(messaggio),
				Posizione: copiaLocalizzatore(pos), Entita: entita})
		case f.Revisione != nil:
			senza = append(senza, FonteSenzaRevisione{Fonte: fonte, Entita: entita, AllegatoID: copiaUUID(allegato), MessaggioID: copiaUUID(messaggio),
				Motivo: MotivoSenzaRevisioneAmbigua})
		default:
			senza = append(senza, FonteSenzaRevisione{Fonte: fonte, Entita: entita, AllegatoID: copiaUUID(allegato), MessaggioID: copiaUUID(messaggio),
				Motivo: MotivoSenzaRevisioneNonLetta})
		}
	}

	if x.f.radici[x.n.Rif] {
		allegato := x.f.s.AllegatoID
		lette, stessaBase := 0, 0
		for _, ln := range x.f.s.NomeFile {
			if ln.Lettura.Forma.Namespace != identita.Namespace || !ln.Lettura.Forma.Base.Completa {
				continue
			}
			lette++
			switch confrontaIdentita(identita, ln.Lettura.Forma) {
			case identitaStessa:
				stessaBase++
				aggiungi(ln.Lettura.Forma, FonteRevisioneNomeFileSTEP, &allegato, nil, ln.Posizione)
			case identitaMarcatoreDiverso:
				stessaBase++
				scarta(FonteRevisioneNomeFileSTEP, &allegato, nil)
			}
		}
		switch {
		case lette == 0:
			senza = append(senza, FonteSenzaRevisione{Fonte: FonteRevisioneNomeFileSTEP, Entita: entita, AllegatoID: &allegato, Motivo: MotivoSenzaRevisioneNomeNonLetto})
		case stessaBase == 0:
			senza = append(senza, FonteSenzaRevisione{Fonte: FonteRevisioneNomeFileSTEP, Entita: entita, AllegatoID: &allegato, Motivo: MotivoSenzaRevisioneNomeDiscorde})
		}
	}

	// Il codice del target: la radice con la base del target è il prodotto (entitaDelNodo), quindi l'entità è il target.
	if t := x.t; x.n.Rif == x.s.Radice && t.Autorita == AutoritaConfermata {
		switch lt := letturaDelTarget(t); confrontaIdentita(identita, lt) {
		case identitaStessa:
			aggiungi(lt, FonteRevisioneCodiceTarget, nil, nil, evidenze.Localizzatore{})
		case identitaMarcatoreDiverso:
			scarta(FonteRevisioneCodiceTarget, nil, nil)
		}
	}

	for _, m := range c.messaggi {
		msg := m.MessaggioID
		switch confrontaIdentita(identita, m.Lettura.Forma) {
		case identitaStessa:
			aggiungi(m.Lettura.Forma, FonteRevisioneMessaggio, nil, &msg, m.Posizione)
		case identitaMarcatoreDiverso:
			scarta(FonteRevisioneMessaggio, nil, &msg)
		}
	}
	return ordinaCandidatiRevisione(cand), ordinaSenzaRevisione(senza)
}

// ordinaCandidatiRevisione: l'ordine canonico dei candidati (fonte, entità, valore, allegato, messaggio, posizione), senza i
// doppioni esatti (due famiglie che leggono la stessa occorrenza con la stessa revisione).
func ordinaCandidatiRevisione(c []CandidatoRevisione) []CandidatoRevisione {
	if len(c) == 0 {
		return nil
	}
	chiave := func(x CandidatoRevisione) string {
		return x.Fonte + "\x00" + x.Entita + "\x00" + x.Valore + "\x00" + testoUUID(x.AllegatoID) + "\x00" + testoUUID(x.MessaggioID) + "\x00" + chiavePosizione(x.Posizione)
	}
	sort.SliceStable(c, func(i, j int) bool { return chiave(c[i]) < chiave(c[j]) })
	out := c[:1]
	for _, x := range c[1:] {
		if chiave(x) != chiave(out[len(out)-1]) {
			out = append(out, x)
		}
	}
	return out
}

// ordinaSenzaRevisione: l'ordine canonico delle fonti senza revisione, senza i doppioni esatti.
func ordinaSenzaRevisione(s []FonteSenzaRevisione) []FonteSenzaRevisione {
	if len(s) == 0 {
		return nil
	}
	chiave := func(x FonteSenzaRevisione) string {
		return x.Fonte + "\x00" + x.Entita + "\x00" + testoUUID(x.AllegatoID) + "\x00" + testoUUID(x.MessaggioID) + "\x00" + x.Motivo
	}
	sort.SliceStable(s, func(i, j int) bool { return chiave(s[i]) < chiave(s[j]) })
	out := s[:1]
	for _, x := range s[1:] {
		if chiave(x) != chiave(out[len(out)-1]) {
			out = append(out, x)
		}
	}
	return out
}

// chiavePosizione: il localizzatore come JSON canonico, per l'ordine.
func chiavePosizione(l evidenze.Localizzatore) string {
	b, err := jsoncanonico.Codifica(l)
	if err != nil {
		return ""
	}
	return string(b)
}

// ---- le copie ----

func testoUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func copiaUUID(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}

func copiaTesto(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

// conTesto: c'è un testo, senza gli spazi ai bordi.
func conTesto(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }

func copiaGrezzo(g ValoreGrezzo) ValoreGrezzo {
	g.Posizione = copiaLocalizzatore(g.Posizione)
	return g
}

// copiaLocalizzatore: il localizzatore con la sua variante copiata, anche i puntatori dentro la variante, così
// l'uscita non condivide memoria con il documento.
func copiaLocalizzatore(l evidenze.Localizzatore) evidenze.Localizzatore {
	if l.STEP != nil {
		s := *l.STEP
		s.Riferimenti, s.Occorrenze = copiaStringhe(s.Riferimenti), copiaStringhe(s.Occorrenze)
		l.STEP = &s
	}
	if l.Testo != nil {
		t := *l.Testo
		l.Testo = &t
	}
	if l.NomeFile != nil {
		n := *l.NomeFile
		n.Stem, n.Estensione = copiaIntervallo(n.Stem), copiaIntervallo(n.Estensione)
		l.NomeFile = &n
	}
	if l.PDF != nil {
		p := *l.PDF
		if p.RiquadroDecimi != nil {
			r := *p.RiquadroDecimi
			p.RiquadroDecimi = &r
		}
		p.Intervallo = copiaIntervallo(p.Intervallo)
		l.PDF = &p
	}
	if l.Tabella != nil {
		t := *l.Tabella
		if t.RigheTesto != nil {
			r := *t.RigheTesto
			t.RigheTesto = &r
		}
		t.Esatto = copiaIntervallo(t.Esatto)
		l.Tabella = &t
	}
	return l
}

func copiaIntervallo(i *evidenze.Intervallo) *evidenze.Intervallo {
	if i == nil {
		return nil
	}
	v := *i
	return &v
}

// copiaParte: una parte letta (marcatore, token, etichetta) in un puntatore nuovo.
func copiaParte(p *motorea.ParteLetta) *motorea.ParteLetta {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// copiaLetturaForma: la lettura con gli elenchi e i puntatori copiati, così la forma della catena non condivide
// memoria con l'interpretazione. Le equivalenze dichiarate della revisione sono un campo privato
// di motorea, che nessuno fuori da lì può cambiare: restano condivise.
func copiaLetturaForma(f motorea.LetturaForma) motorea.LetturaForma {
	f.Base = copiaBase(f.Base)
	f.Marcatore, f.Token, f.Etichetta = copiaParte(f.Marcatore), copiaParte(f.Token), copiaParte(f.Etichetta)
	if f.Affissi != nil {
		affissi := make([]motorea.AffissoLetto, len(f.Affissi))
		for i, a := range f.Affissi {
			if a.Valore != nil {
				v := *a.Valore
				a.Valore = &v
			}
			affissi[i] = a
		}
		f.Affissi = affissi
	}
	if f.Revisione != nil {
		r := *f.Revisione
		r.Segmenti = append([]motorea.SegmentoLetto(nil), r.Segmenti...)
		if len(r.Segmenti) == 0 {
			r.Segmenti = nil
		}
		r.Token = copiaParte(r.Token)
		f.Revisione = &r
	}
	if f.Decorazioni != nil {
		f.Decorazioni = append([]motorea.DecorazioneLetta(nil), f.Decorazioni...)
	}
	if f.Ripetizioni != nil {
		ripetizioni := make([]motorea.RipetizioneLetta, len(f.Ripetizioni))
		for i, r := range f.Ripetizioni {
			r.Base = copiaBase(r.Base)
			ripetizioni[i] = r
		}
		f.Ripetizioni = ripetizioni
	}
	if f.Categorie != nil {
		f.Categorie = append(f.Categorie[:0:0], f.Categorie...)
	}
	return f
}
