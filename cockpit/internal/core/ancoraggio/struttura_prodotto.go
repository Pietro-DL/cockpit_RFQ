package ancoraggio

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// Le strutture del prodotto (piano A, 6.4.5, regole 1 e 2; contratto §0 punto 1, §1.2, §1.3, §2.2; commit P6a, B2):
// per ogni prodotto target, una struttura per STEP e radice (StrutturaProdotto, era RadiceScenario: T-B0-02), con i
// nodi raggiungibili, gli archi con quantità e occorrenze, gli archi del contesto con la loro origine. Prima della
// conferma della fonte si calcola e si mostra tutto, e non si promuove niente (R59 A): ogni struttura è
// struttura_candidata. Solo quella sotto la radice scelta dello STEP confermato è bom_di_lavoro_proposta, nello stesso
// calcolo (R76 A, R85), e resta una proposta: la conferma della fonte cambia l'autorità della fonte, non della BOM
// (R59, R85; 6.0.4). La BOM verificata è un'altra cosa (R80, valutazione, B5).

// ---- i target ----

// ProdottoRichiesto: un target delle proposte (par.3.3.7, 6.4.5; contratto §2.2), con la sua autorità. Lo compone
// valutazione dai prodotti target (R60 A: solo i confermati dal gesto 2 e lo scenario; R70 A, R75 A).
//   - Rif: il riferimento del target, lo stesso di valutazione: «componente:<uuid>», «identificativo:<codice>»,
//     «scenario:<caso>:<n>» (contratto §1.1; emendamento E1 §4.2).
//   - Autorita: confermata o scenario, mai proposta: un candidato della mail non è un target (R60 A).
//   - Namespace, Base: la lettura del codice del target con la grammatica del cliente. Base vuota vuol dire «non
//     letto»: la base non si inventa (T-B1-07), e nessuna radice si confronta.
//   - Marcatore (B4): il marcatore della stessa lettura, "" se non c'è; lo riempie valutazione (T-B4-20 della fase 2,
//     accolta con T-B4-06 rivisto). Serve alla regola unica dell'identità: il codice del target come indizio di
//     revisione e il candidato a livello prodotto non valgono per un'identità con un altro marcatore scritto.
//   - CodiceRichiesto: l'originale, com'è scritto. Revisione e Qualificatori: della lettura, quando ci sono.
//   - Origine, VersioneDecisione: da dove viene il target e l'impronta della decisione che lo fa target (par.3.3.7).
//   - ComponenteID: il componente del target, se c'è: la radice delle relazioni confermate (contratto §2.2).
//   - FonteConfermata: la fonte STEP confermata dal gesto 3 (contratto §1.2: documento, sha256, radice, chi e quando),
//     se c'è. Dice quale struttura è la BOM di lavoro proposta: solo quella sotto la radice scelta (R76 A). La mette
//     valutazione quando la fonte del prodotto è confermata.
type ProdottoRichiesto struct {
	Rif               string                  `json:"rif"`
	Autorita          Autorita                `json:"autorita"`
	ClienteID         uuid.UUID               `json:"cliente_id"`
	Namespace         string                  `json:"namespace,omitempty"`
	CodiceRichiesto   string                  `json:"codice_richiesto"`
	Base              motorea.BaseLetta       `json:"base"`
	Marcatore         string                  `json:"marcatore,omitempty"`
	Revisione         *motorea.RevisioneLetta `json:"revisione,omitempty"`
	Qualificatori     []motorea.AffissoLetto  `json:"qualificatori,omitempty"`
	Origine           string                  `json:"origine,omitempty"`
	VersioneDecisione string                  `json:"versione_decisione,omitempty"`
	ComponenteID      *uuid.UUID              `json:"componente_id,omitempty"`
	FonteConfermata   *RiferimentoFonte       `json:"fonte_confermata,omitempty"`
}

// ---- il contesto ----

// ContestoStrutturale: il grafo che le proposte usano per radici e figli (6.4.5; v3 §10.4), come ingresso già
// convertito: lo prepara valutazione dalla fotografia, che ancoraggio non importa (T-B0-04). Ognuno con la sua
// autorità e la sua origine: è confermato solo ciò che l'operatore ha confermato (R29 e); «Conferma l'albero»
// conferma la BOM, non la fonte (R61 A). Non si materializza nessuna BOM (R29 e).
//   - Strutture: le strutture STEP dei fatti, una per allegato (StrutturaDa).
//   - Confermato e ArchiConfermati: i componenti della RFQ e le relazioni confermate (componente_relazione), con
//     l'origine «confermato».
//   - Proposto e ArchiProposti: le righe aperte della tabella legacy dei nodi proposti (componente_proposta) e gli
//     archi aperti (relazione_proposta), con l'origine «proposto». Gli archi stanno in un elenco a parte e non dentro
//     le righe, come nel 6.4.5: un arco aperto può avere il figlio in una riga già decisa, e non deve sparire.
//
// Il NodoStrutturale del 6.4.5 si divide in ComponenteDeciso e RigaPropostaLegacy (T-B0-02). Con il commit di B4 il
// contesto porta anche ciò che serve alla catena del codice e alle decisioni accanto ai nodi, tutto già convertito da
// valutazione, che legge la fotografia:
//   - Decise: le righe già decise della tabella legacy (confermata, duplicato, scartata), per legare il nodo alla sua
//     decisione per UUID (emendamento E1 §4.2; T-E1-05) e per la provenienza della revisione registrata (T-E1-22);
//   - CodiciProposti: i codici che valutazione compone con motorea.ComponiCodiceDocumentale, uno per lettura
//     d'identità, con la chiave ChiaveCodiceProposto (T-08): ancoraggio non chiama il compositore;
//   - CodiciMessaggi: i codici letti dal motore A nel testo dei messaggi del thread, indizi di revisione (T-E1-06);
//   - DecisioniIdentita: le decisioni del modello nuovo su codice e revisione (E1R; in A1c nessun adattatore le
//     produce: LD-27). Qui conta la parte componente.
//
// Per la riconciliazione (B4, fase 3), sempre già convertiti da valutazione:
//   - LettureDecise: il codice di una DecisioneIdentita sul componente letto con la grammatica del cliente (R31 c, come
//     ComponenteDeciso.Lettura), per Rif del componente (RifComponente). Serve quando il codice deciso non è quello del
//     componente: senza, la sua base non si conosce, e il confronto con il cartiglio sul codice non si può fare
//     (lettura T-B4-32);
//   - AssociazioniDecise: le associazioni dei file ai componenti decise nel DB, manuali o confermate (contratto §1.5),
//     per l'origine dell'associazione del codice documentale (lettura T-B4-33).
//
// Il codice del target confermato, l'altro indizio di T-E1-06, è già nel ProdottoRichiesto (Revisione).
type ContestoStrutturale struct {
	Strutture          []StrutturaFile                   `json:"strutture,omitempty"`
	Confermato         []ComponenteDeciso                `json:"confermato,omitempty"`
	ArchiConfermati    []ArcoPercorso                    `json:"archi_confermati,omitempty"`
	Proposto           []RigaPropostaLegacy              `json:"proposto,omitempty"`
	ArchiProposti      []ArcoPercorso                    `json:"archi_proposti,omitempty"`
	Decise             []RigaDecisaLegacy                `json:"decise,omitempty"`
	CodiciProposti     map[string]motorea.CodiceComposto `json:"codici_proposti,omitempty"`
	CodiciMessaggi     []CodiceDiMessaggio               `json:"codici_messaggi,omitempty"`
	DecisioniIdentita  []DecisioneIdentita               `json:"decisioni_identita,omitempty"`
	LettureDecise      map[string]motorea.LetturaForma   `json:"letture_decise,omitempty"`
	AssociazioniDecise []AssociazioneDecisa              `json:"associazioni_decise,omitempty"`
}

// ComponenteDeciso: una riga di componente confermata della RFQ, com'è nel DB (T-B0-02). Codice è la decisione
// normalizzata dal legacy, per mostrarla: mai il grezzo (I-6). Autorità «confermata», origine «confermato».
//   - Rev (B4): componente.rev, la revisione registrata, con la provenienza non registrata (T-B0-34); nil = nessuna.
//   - Lettura (B4): Codice letto con la grammatica del cliente (R31 c; valutazione, LetturaRegistrata); nil = non letto.
type ComponenteDeciso struct {
	ComponenteID uuid.UUID             `json:"componente_id"`
	Codice       string                `json:"codice"`
	Autorita     Autorita              `json:"autorita"`
	Origine      OrigineDato           `json:"origine"`
	Rev          *string               `json:"rev,omitempty"`
	Lettura      *motorea.LetturaForma `json:"lettura,omitempty"`
}

// RigaPropostaLegacy: una riga aperta di componente_proposta, la tabella legacy letta e mai scritta (M2, T-B0-02).
// È dello stesso nodo dello STEP con lo stesso sha256 e la stessa chiave (RifNodo; emendamento E1 §4.2): la chiave
// non dipende dal codice. Autorità «proposta», origine «proposto»: il codice del motore legacy non entra (R29 e).
//   - CodiceManuale, RevManuale, LetturaManuale (B4): il codice e la revisione che l'operatore ha scritto sulla riga
//     aperta (origine_codice = operatore), senza chi né quando, e il codice letto con la grammatica del cliente; nil
//     per ogni altra riga: il codice del motore legacy non si legge mai (T-B0-33, LD-13). Una correzione cambia il
//     codice, non la chiave: il nodo resta lo stesso.
type RigaPropostaLegacy struct {
	ID             uuid.UUID             `json:"id"`
	AllegatoID     uuid.UUID             `json:"allegato_id"`
	Sha256         string                `json:"sha256"`
	Chiave         string                `json:"chiave"`
	Autorita       Autorita              `json:"autorita"`
	Origine        OrigineDato           `json:"origine"`
	CodiceManuale  *string               `json:"codice_manuale,omitempty"`
	RevManuale     *string               `json:"rev_manuale,omitempty"`
	LetturaManuale *motorea.LetturaForma `json:"lettura_manuale,omitempty"`
}

// RigaDecisaLegacy: una riga già decisa di componente_proposta (B4; emendamento E1 §4.2), letta dalla fotografia. Ha
// solo ciò che lega il nodo alla sua decisione: l'allegato, lo sha256 e la chiave del nodo, lo stato, il componente
// e chi ha deciso. Mai il codice né la revisione che il legacy ci ha scritto (T-E1-22; contratto §3).
//   - Stato: confermata o duplicato (il nodo è quel componente: ComponenteID c'è sempre, come vuole il DB) oppure
//     scartata.
//   - DecisoDa: la persona; nil per un aggancio automatico per codice (un duplicato di prima dello Smistamento).
type RigaDecisaLegacy struct {
	ID           uuid.UUID  `json:"id"`
	AllegatoID   uuid.UUID  `json:"allegato_id"`
	Sha256       string     `json:"sha256"`
	Chiave       string     `json:"chiave"`
	Stato        string     `json:"stato"`
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	DecisoDa     *uuid.UUID `json:"deciso_da,omitempty"`
}

// I valori di RigaDecisaLegacy.Stato (stato_proposta, 0001; senza «aperta», che è RigaPropostaLegacy).
const (
	StatoRigaConfermata = "confermata"
	StatoRigaDuplicato  = "duplicato"
	StatoRigaScartata   = "scartata"
)

// I prefissi dei riferimenti del contesto, gli stessi di valutazione e del 6.4.5.
const (
	prefissoRifComponente    = "componente:"
	prefissoRifRigaProposta  = "componente_proposta:"
	percorsoContesto         = "contesto"
	percorsoTarget           = "target"
	percorsoStrutture        = "strutture"
	messaggioRigaNonProposta = "una riga aperta della tabella legacy è una proposta, con l'origine «proposto» (R29 e; R61 A: le righe di «Conferma l'albero» non sono aperte)"
	messaggioSenzaFile       = "un nodo si riconosce solo con l'allegato, lo sha256 del file e la chiave, perché fuori dal suo file la chiave non significa niente (T-E1-04, emendamento E1 §4.2)"
)

// RifComponente: il riferimento di un componente della RFQ, «componente:<uuid>».
func RifComponente(id uuid.UUID) string { return prefissoRifComponente + id.String() }

// RifRigaProposta: il riferimento di una riga di componente_proposta, «componente_proposta:<uuid>».
func RifRigaProposta(id uuid.UUID) string { return prefissoRifRigaProposta + id.String() }

// ---- le uscite ----

// StatoStruttura: lo stato di una struttura (R59, R76, R85; contratto §1.3).
//   - nessuna: un target senza nessuna struttura (StatoStrutturaDelTarget); una StrutturaProdotto non lo ha mai;
//   - struttura_candidata: il grafo di uno STEP sotto una radice candidata, finché il prodotto non ha la fonte
//     confermata, e dopo per ogni altra struttura;
//   - bom_di_lavoro_proposta: la struttura sotto la radice scelta dello STEP confermato, al più una per prodotto
//     (R76 A). È ancora una proposta: niente qui la dice verificata (R80, B5).
type StatoStruttura string

const (
	StatoStrutturaNessuna    StatoStruttura = "nessuna"
	StatoStrutturaCandidata  StatoStruttura = "struttura_candidata"
	StatoBOMDiLavoroProposta StatoStruttura = "bom_di_lavoro_proposta"
)

// NodoProposto: un nodo di una struttura del prodotto (M2; contratto §1.4, §2.2), proposto dal file.
//   - Rif: sha256 del file più chiave dell'entità (RifNodo), mai il codice (T-E1-04).
//   - AllegatoID: l'allegato della struttura. EntitaID: l'entità nodo_step del suo documento.
//   - Padri: i Rif dei padri immediati dentro la struttura, in ordine; più d'uno per un figlio condiviso
//     (FIGLIO-CONDIVISO). Vuoto per la radice in un grafo senza cicli, come quello di uno STEP: se la radice è un nodo
//     interno del file, i suoi padri restano fuori.
//   - Leggibile: almeno una lettura d'identità; falso per un nodo che nessuna famiglia legge (le saldature, D10), che
//     si mostra ma non è mai una radice candidata né un candidato.
//   - SenzaFile: nessun file ancorato al nodo (3.3.7, P-20: l'assenza di un CAD è ammessa e senza diagnostica).
//     ProponiStrutture non ancora file, e lo dice di ogni nodo; ProponiAncoraggi (B4, fase 2) lo ricalcola dai file
//     candidati, e la radice di una struttura che è la radice del suo STEP non è senza file.
//
// Dal commit di B4 (fase 1), nello stesso calcolo della struttura, anche della BOM di lavoro (R85):
//   - Codice: la catena del codice (CatenaCodice), con l'identità del nodo, anche parziale (R86, R87).
//   - Decisione: il componente confermato legato al nodo per UUID: il componente_id della riga legacy decisa dello
//     stesso nodo (stesso allegato, sha256 e chiave; confermata o duplicato), se è fra i componenti del contesto
//     (emendamento E1 §4.2, T-E1-05). Una rinomina del componente non la stacca; accanto, mai fusa con la proposta.
//   - RigaLegacy: la riga aperta della tabella legacy dello stesso nodo, per (allegato, RifNodo), con il codice
//     manuale se l'operatore l'ha corretta (T-B0-33): una correzione non cambia il nodo.
//   - RigaDecisa: la riga già decisa dello stesso nodo, quando c'è (campo in più, lettura dell'orchestratore
//     T-B4-07): dice lo stato anche di un nodo scartato, che non ha una Decisione.
//   - DecisoDaPersona: la riga decisa del nodo ha chi l'ha decisa; falso per un aggancio automatico per codice o senza
//     riga decisa.
//   - AbbinamentoPerBase (B4, fase 2; campo in più, lettura dell'orchestratore T-B4-12): senza la decisione per UUID, i
//     componenti confermati con la stessa identità del nodo, come proposta (emendamento E1 §4.2, punto 2); con la
//     decisione, il motivo quando la base del componente non è più compatibile con l'identità del nodo o non si può
//     verificare; con la riga decisa che porta un componente fuori dal contesto, il motivo e l'abbinamento accanto
//     (T-B4-26); sul nodo scartato, nodo_scartato e nessuna proposta (T-B4-27). Mai una decisione: Decisione resta
//     quella per UUID.
type NodoProposto struct {
	Rif                string              `json:"rif"`
	AllegatoID         uuid.UUID           `json:"allegato_id"`
	EntitaID           string              `json:"entita_id"`
	Codice             CatenaCodice        `json:"codice"`
	Padri              []string            `json:"padri,omitempty"`
	Leggibile          bool                `json:"leggibile"`
	SenzaFile          bool                `json:"senza_file"`
	Decisione          *ComponenteDeciso   `json:"decisione,omitempty"`
	RigaLegacy         *RigaPropostaLegacy `json:"riga_legacy,omitempty"`
	RigaDecisa         *RigaDecisaLegacy   `json:"riga_decisa,omitempty"`
	DecisoDaPersona    bool                `json:"deciso_da_persona"`
	AbbinamentoPerBase *AbbinamentoPerBase `json:"abbinamento_per_base,omitempty"`
}

// ArcoProposto: un arco dell'albero proposto di una struttura del prodotto (M2; contratto §1.4): dai fatti dello STEP,
// fra due nodi della struttura, con la quantità (le occorrenze della coppia) e le occorrenze elencate.
//   - Decisione (B4; T-14): l'arco confermato accanto, con la sua quantità: la relazione confermata fra i componenti
//     decisi del padre e del figlio (Decisione dei due nodi). Sulla BOM di lavoro, per gli archi della radice scelta
//     con il gesto 3, il padre vale il componente del target (T-B4-23). Accanto, mai fuso: con una quantità diversa
//     resta il valore corrente, e QuantitaDiscorde ne dà il segnale (R95 A, T-B0-24).
type ArcoProposto struct {
	Padre      string        `json:"padre"`
	Figlio     string        `json:"figlio"`
	Quantita   *int          `json:"quantita,omitempty"`
	Occorrenze []string      `json:"occorrenze,omitempty"`
	Decisione  *ArcoPercorso `json:"decisione,omitempty"`
}

// StrutturaProdotto: la struttura di un prodotto target in uno STEP, sotto una radice (era RadiceScenario: T-B0-02;
// contratto §2.2; 6.4.5, regole 1 e 2). Una per STEP e radice: lo stesso target può averne più d'una (più STEP, più
// radici candidate nello stesso STEP), e lo stesso contenuto in due allegati ne dà due (nessuna fusione).
//   - Target: il Rif del ProdottoRichiesto. AllegatoID, Sha256: il file. Radice: il Rif del nodo radice.
//   - RadiceDelFile: la radice è una radice del file; falso quando il prodotto compare come nodo interno di uno STEP
//     più grande, e allora vale il suo sottoalbero (R76 b A: la fonte resta quella di valutazione).
//   - Compatibilita: il confronto della base del target con le letture della radice (motorea.ConfrontaBasi, nello
//     stesso namespace, la migliore). Uguale o compatibile parziale per una radice candidata (regola 1); la BOM di
//     lavoro sta sotto la radice scelta dall'operatore, qualunque sia (R76 A), e qui si vede se ha la base.
//   - Stato: struttura_candidata o bom_di_lavoro_proposta (R59 A, R76 A).
//   - Nodi: la radice, poi i nodi raggiungibili dalla radice negli archi dei fatti, in ordine di Rif (regola 2).
//   - Archi: gli archi dei fatti fra i nodi della struttura, in ordine di (padre, figlio).
//   - ArchiContesto: gli archi del contesto con la loro origine (regola 2): quelli proposti (relazione_proposta) con il
//     padre fra i nodi della struttura, e le relazioni confermate raggiungibili dal componente del target. Accanto,
//     mai fusi con l'albero proposto (contratto §1.4).
//   - Fonti: il file come documento candidato della fonte del target (M1), solo se lo è: la sua radice ha la base del
//     prodotto (R76 b A, lo stesso criterio di valutazione) e non è lo STEP della fonte confermata (T-B0-07). Vuoto
//     per un prodotto che compare solo come nodo interno.
//   - GrafoCompleto, MotivoGrafo: la completezza del grafo del file, una qualità della fonte (R32 b), mai
//     «completa» da sola.
//   - RigaRadice, RigheDaDecidere: le righe aperte della tabella legacy dei nodi della struttura. Quella della radice
//     non si decide mai (la radice del prodotto non è una riga decisa: «Conferma l'albero»), si mostra a parte e non
//     conta fra le righe da decidere (R80).
type StrutturaProdotto struct {
	Target          string                `json:"target"`
	AllegatoID      uuid.UUID             `json:"allegato_id"`
	Sha256          string                `json:"sha256"`
	Radice          string                `json:"radice"`
	RadiceDelFile   bool                  `json:"radice_del_file"`
	Compatibilita   motorea.Compatibilita `json:"compatibilita"`
	Stato           StatoStruttura        `json:"stato"`
	Nodi            []NodoProposto        `json:"nodi,omitempty"`
	Archi           []ArcoProposto        `json:"archi,omitempty"`
	ArchiContesto   []ArcoPercorso        `json:"archi_contesto,omitempty"`
	Fonti           []DocumentoCandidato  `json:"fonti,omitempty"`
	GrafoCompleto   bool                  `json:"grafo_completo"`
	MotivoGrafo     string                `json:"motivo_grafo,omitempty"`
	RigaRadice      string                `json:"riga_radice,omitempty"`
	RigheDaDecidere []string              `json:"righe_da_decidere,omitempty"`
}

// ---- ProponiStrutture ----

// ProponiStrutture: le strutture dei prodotti target (6.4.5, regole 1 e 2, che ProponiAncoraggi userà prima di
// proporre i file). È pura e deterministica: l'ordine degli ingressi non conta, e gli ingressi di chi chiama non
// cambiano.
//
//  1. Radici candidate. Per ogni target e ogni struttura del contesto: un nodo con una lettura d'identità compatibile
//     con la base del target (stesso namespace; ConfrontaBasi uguale, o compatibile parziale per una forma parziale)
//     e il marcatore compatibile con ProdottoRichiesto.Marcatore (T-B4-30: due marcatori scritti e diversi no)
//     è una radice candidata. Di solito è la radice del file; può essere un nodo interno, e allora vale il suo
//     sottoalbero. Un nodo senza letture non lo è mai.
//  2. La struttura. Dalla radice, i nodi raggiungibili negli archi dei fatti, con i padri, gli archi, le quantità e le
//     occorrenze; accanto gli archi del contesto confermato e proposto, ognuno con la sua origine. La gerarchia viene
//     solo dallo STEP (R68 A): gli archi del contesto non aggiungono nodi.
//  3. Lo stato. Tutto è struttura_candidata (R59 A). Con la fonte confermata del target (FonteConfermata: tipo step,
//     sha256, radice registrata, non superata), la struttura dello STEP con quello sha256 sotto la radice scelta è
//     bom_di_lavoro_proposta, nello stesso calcolo, con la sua gerarchia e le sue quantità (R76 A, R85): se la radice
//     scelta non è fra le candidate (la sua base non è quella del target, o il target non è letto), la struttura si
//     costruisce lo stesso, perché la radice l'ha scelta l'operatore. Le altre strutture dello stesso prodotto, anche
//     dello stesso STEP sotto un'altra radice, restano candidate. A parità di contenuto (lo stesso sha256 in due
//     allegati) vale l'allegato del riferimento, se c'è fra le strutture, altrimenti il primo in ordine di ID: le
//     due strutture sono uguali. Niente si promuove oltre: nessuna struttura è verificata (R80).
//     Senza BOM di lavoro, e tutto candidato: la radice non registrata (T-B0-08: non la sceglie il sistema), la fonte
//     superata (la conferma valeva per un'altra versione: R65, riferimento_superato), lo STEP confermato senza una
//     struttura letta, o una radice che nel file non c'è.
//  4. Le diagnostiche: ancoraggio.target_senza_struttura per un target senza strutture, ancoraggio.grafo_incompleto
//     per una struttura con il grafo non completo, con il motivo. La completezza dei file dei figli («solo parti») non
//     conta: contano le strutture dei target (regola 2).
//  5. I nodi e gli archi, nello stesso calcolo (B4, fase 1; R85): ogni nodo con la decisione accanto per UUID, la riga
//     legacy dello stesso nodo e la catena del codice con l'identità, anche parziale (6.0.6; R86, R87; T-B0-33,
//     T-E1-05, T-E1-06, T-E1-22, T-E1R-08); ogni arco con l'arco confermato accanto. Niente si fonde e niente si
//     applica: le decisioni restano il valore corrente (R95 A). Dalla fase 2, l'abbinamento per base di ogni nodo,
//     come proposta accanto alla decisione per UUID (emendamento E1 §4.2, punto 2; T-B4-12).
//
// Le strutture escono in ordine di (target, sha256, allegato, radice), le diagnostiche in ordine di (codice,
// percorso, riferimenti). L'errore è solo di contratto (*evidenze.ErroreContratto): vedi controllaContesto.
func ProponiStrutture(target []ProdottoRichiesto, ctx ContestoStrutturale) ([]StrutturaProdotto, []evidenze.Diagnostica, error) {
	if d := controllaContesto(target, ctx); len(d) > 0 {
		return nil, nil, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	c := indicizza(ctx)
	targets := append([]ProdottoRichiesto(nil), target...)
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].Rif < targets[j].Rif })

	var out []StrutturaProdotto
	var diag []evidenze.Diagnostica
	for _, t := range targets {
		var proprie []StrutturaProdotto
		for _, f := range c.file {
			for _, n := range f.s.Nodi {
				if comp := compatibilitaDelNodo(t, n); compatibile(comp) {
					proprie = append(proprie, c.struttura(t, f, n, comp))
				}
			}
		}
		if f, n, ok := c.radiceScelta(t.FonteConfermata); ok {
			trovata := false
			for i := range proprie {
				if proprie[i].AllegatoID == f.s.AllegatoID && proprie[i].Radice == n.Rif {
					proprie[i].Stato, trovata = StatoBOMDiLavoroProposta, true
					c.archiDellaRadiceScelta(t, &proprie[i])
					c.confermatoDellaRadiceScelta(t, &proprie[i])
				}
			}
			if !trovata {
				s := c.struttura(t, f, n, compatibilitaDelNodo(t, n))
				s.Stato = StatoBOMDiLavoroProposta
				c.archiDellaRadiceScelta(t, &s)
				c.confermatoDellaRadiceScelta(t, &s)
				proprie = append(proprie, s)
			}
		}

		if len(proprie) == 0 {
			diag = append(diag, evidenze.Diagnostica{
				Codice:    CodiceTargetSenzaStruttura,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  percorsoTarget + "[" + t.Rif + "]",
				Messaggio: messaggioSenzaStruttura(t),
				Rif:       []string{t.Rif},
			})
		}
		for _, s := range proprie {
			if s.GrafoCompleto {
				continue
			}
			diag = append(diag, evidenze.Diagnostica{
				Codice:    CodiceGrafoIncompleto,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  percorsoStrutture + "[" + t.Rif + "]",
				Messaggio: "il grafo dello STEP della struttura non è completo: «" + s.MotivoGrafo + "»; la mancanza di un arco non è un'informazione",
				Rif:       []string{t.Rif, idFonteAllegato(s.AllegatoID), s.Radice},
			})
		}
		out = append(out, proprie...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		for _, x := range [][2]string{{a.Target, b.Target}, {a.Sha256, b.Sha256}, {a.AllegatoID.String(), b.AllegatoID.String()}, {a.Radice, b.Radice}} {
			if x[0] != x[1] {
				return x[0] < x[1]
			}
		}
		return false
	})
	sort.SliceStable(diag, func(i, j int) bool { return chiaveDiagnostica(diag[i]) < chiaveDiagnostica(diag[j]) })
	return out, diag, nil
}

// StatoStrutturaDelTarget: lo stato della struttura di un target fra le strutture date (contratto §1.3):
// bom_di_lavoro_proposta se una sua struttura lo è, struttura_candidata se ne ha, altrimenti nessuna.
func StatoStrutturaDelTarget(strutture []StrutturaProdotto, target string) StatoStruttura {
	stato := StatoStrutturaNessuna
	for _, s := range strutture {
		if s.Target != target {
			continue
		}
		if s.Stato == StatoBOMDiLavoroProposta {
			return StatoBOMDiLavoroProposta
		}
		stato = StatoStrutturaCandidata
	}
	return stato
}

// messaggioSenzaStruttura: perché un target non ha strutture.
func messaggioSenzaStruttura(t ProdottoRichiesto) string {
	if len(t.Base.Segmenti) == 0 {
		return "il codice del target non è letto: nessuna radice degli STEP si confronta, e nessuna fonte confermata dà una radice che si trova fra le strutture lette"
	}
	return "nessun nodo degli STEP letti ha la base del target, e nessuna fonte confermata dà una radice che si trova fra le strutture lette: i file non si collocano"
}

// idFonteAllegato: l'ID della fonte di un allegato, come lo scrive l'adattatore («allegato:<uuid>»).
func idFonteAllegato(id uuid.UUID) string { return "allegato:" + id.String() }

// ---- il contesto con i suoi indici ----

// fileIndicizzato: una struttura del contesto con i suoi indici. Le mappe servono solo a cercare, mai a scorrere: si
// scorrono i nodi in ordine di Rif e i figli in ordine di arco.
type fileIndicizzato struct {
	s      StrutturaFile
	nodi   map[string]NodoStruttura
	figli  map[string][]ArcoPercorso
	radici map[string]bool
}

// contestoIndicizzato: il contesto con gli indici: le strutture in ordine di (sha256, allegato), gli archi confermati
// e proposti per padre, le righe aperte per allegato. Per la catena e le decisioni (B4): le righe aperte e decise per
// (allegato, nodo), i componenti, le decisioni sui componenti, le relazioni confermate per coppia, le formazioni per
// nodo, i nodi delle righe decise per componente, i codici proposti, i codici dei messaggi in ordine; per la
// riconciliazione (fase 3) le letture dei codici decisi.
type contestoIndicizzato struct {
	file       []*fileIndicizzato
	confermati map[string][]ArcoPercorso
	proposti   map[string][]ArcoPercorso
	righe      map[uuid.UUID][]RigaPropostaLegacy

	aperte              map[string]RigaPropostaLegacy
	decise              map[string]RigaDecisaLegacy
	componenti          map[uuid.UUID]ComponenteDeciso
	decisioniComponente map[uuid.UUID]DecisioneIdentita
	coppieConfermate    map[string]ArcoPercorso
	formazioni          map[string][]string
	nodiDelComponente   map[uuid.UUID][]string
	codiciProposti      map[string]motorea.CodiceComposto
	messaggi            []CodiceDiMessaggio
	confermatiInOrdine  []ComponenteDeciso              // per l'abbinamento per base (fase 2), in ordine di componente
	lettureDecise       map[string]motorea.LetturaForma // per la riconciliazione (fase 3), per Rif del componente
}

// chiaveRigaNodo: la riga legacy di un nodo si cerca per (allegato, RifNodo): lo stesso contenuto in due allegati ha
// due righe (emendamento E1 §4.2).
func chiaveRigaNodo(allegato uuid.UUID, rifNodo string) string {
	return allegato.String() + "\x00" + rifNodo
}

func indicizza(ctx ContestoStrutturale) *contestoIndicizzato {
	c := &contestoIndicizzato{confermati: map[string][]ArcoPercorso{}, proposti: map[string][]ArcoPercorso{},
		righe: map[uuid.UUID][]RigaPropostaLegacy{}, aperte: map[string]RigaPropostaLegacy{}, decise: map[string]RigaDecisaLegacy{},
		componenti: map[uuid.UUID]ComponenteDeciso{}, decisioniComponente: map[uuid.UUID]DecisioneIdentita{},
		coppieConfermate: map[string]ArcoPercorso{}, formazioni: map[string][]string{}, nodiDelComponente: map[uuid.UUID][]string{},
		codiciProposti: ctx.CodiciProposti, lettureDecise: ctx.LettureDecise}
	strutture := append([]StrutturaFile(nil), ctx.Strutture...)
	sort.SliceStable(strutture, func(i, j int) bool {
		if strutture[i].Sha256 != strutture[j].Sha256 {
			return strutture[i].Sha256 < strutture[j].Sha256
		}
		return strutture[i].AllegatoID.String() < strutture[j].AllegatoID.String()
	})
	for _, s := range strutture {
		f := &fileIndicizzato{s: s, nodi: map[string]NodoStruttura{}, figli: map[string][]ArcoPercorso{}, radici: map[string]bool{}}
		f.s.Nodi = append([]NodoStruttura(nil), s.Nodi...)
		sort.SliceStable(f.s.Nodi, func(i, j int) bool { return f.s.Nodi[i].Rif < f.s.Nodi[j].Rif })
		f.s.Radici = append([]string(nil), s.Radici...)
		sort.Strings(f.s.Radici)
		f.s.Archi = append([]ArcoPercorso(nil), s.Archi...)
		ordinaArchi(f.s.Archi)
		for _, n := range f.s.Nodi {
			f.nodi[n.Rif] = n
		}
		for _, r := range f.s.Radici {
			f.radici[r] = true
		}
		for _, a := range f.s.Archi {
			f.figli[a.Padre] = append(f.figli[a.Padre], a)
		}
		c.file = append(c.file, f)
	}
	for _, a := range archiInOrdine(ctx.ArchiConfermati) {
		c.confermati[a.Padre] = append(c.confermati[a.Padre], a)
	}
	for _, a := range archiInOrdine(ctx.ArchiProposti) {
		c.proposti[a.Padre] = append(c.proposti[a.Padre], a)
	}
	righe := append([]RigaPropostaLegacy(nil), ctx.Proposto...)
	sort.SliceStable(righe, func(i, j int) bool { return righe[i].ID.String() < righe[j].ID.String() })
	for _, r := range righe {
		c.righe[r.AllegatoID] = append(c.righe[r.AllegatoID], r)
		c.aperte[chiaveRigaNodo(r.AllegatoID, RifNodo(r.Sha256, r.Chiave))] = r
	}

	// B4: le decisioni accanto ai nodi e la catena del codice.
	for _, f := range c.file {
		for _, n := range f.s.Nodi {
			if len(n.Formazioni) > 0 && c.formazioni[n.Rif] == nil {
				c.formazioni[n.Rif] = n.Formazioni // lo stesso contenuto dà le stesse formazioni
			}
		}
	}
	decise := append([]RigaDecisaLegacy(nil), ctx.Decise...)
	sort.SliceStable(decise, func(i, j int) bool { return decise[i].ID.String() < decise[j].ID.String() })
	for _, r := range decise {
		rif := RifNodo(r.Sha256, r.Chiave)
		c.decise[chiaveRigaNodo(r.AllegatoID, rif)] = r
		if r.ComponenteID != nil {
			c.nodiDelComponente[*r.ComponenteID] = append(c.nodiDelComponente[*r.ComponenteID], rif)
		}
	}
	for _, k := range ctx.Confermato {
		c.componenti[k.ComponenteID] = k
	}
	c.confermatiInOrdine = append([]ComponenteDeciso(nil), ctx.Confermato...)
	sort.SliceStable(c.confermatiInOrdine, func(i, j int) bool {
		return c.confermatiInOrdine[i].ComponenteID.String() < c.confermatiInOrdine[j].ComponenteID.String()
	})
	for _, d := range ctx.DecisioniIdentita {
		if d.Oggetto == OggettoDecisioneComponente {
			c.decisioniComponente[d.ID] = d
		}
	}
	for _, a := range ctx.ArchiConfermati {
		c.coppieConfermate[a.Padre+"\x00"+a.Figlio] = a
	}
	c.messaggi = append([]CodiceDiMessaggio(nil), ctx.CodiciMessaggi...)
	sort.SliceStable(c.messaggi, func(i, j int) bool {
		if c.messaggi[i].MessaggioID != c.messaggi[j].MessaggioID {
			return c.messaggi[i].MessaggioID.String() < c.messaggi[j].MessaggioID.String()
		}
		return c.messaggi[i].Lettura.ID < c.messaggi[j].Lettura.ID
	})
	return c
}

// archiInOrdine: una copia degli archi in ordine canonico, senza i doppioni esatti (lo stesso contenuto in due
// allegati porta gli stessi archi proposti, con gli stessi riferimenti).
func archiInOrdine(archi []ArcoPercorso) []ArcoPercorso {
	out := make([]ArcoPercorso, 0, len(archi))
	for _, a := range archi {
		out = append(out, copiaArco(a))
	}
	ordinaArchi(out)
	var unici []ArcoPercorso
	for i, a := range out {
		if i > 0 && chiaveArco(a) == chiaveArco(out[i-1]) {
			continue
		}
		unici = append(unici, a)
	}
	return unici
}

// compatibilitaDelNodo: il confronto della base del target con le letture d'identità di un nodo, nello stesso
// namespace: la migliore fra uguale, compatibile parziale, discordante, non determinabile (come la fonte in
// valutazione, R76 b). Senza letture del nodo nel namespace del target, o con il target non letto: non determinabile.
// Dalla fase 2 di B4 vale anche la regola unica del marcatore (T-B4-30): con il marcatore del target e quello del
// nodo scritti e diversi la base compatibile è discordante, e il nodo non è una radice candidata; un marcatore scritto
// da una parte sola non cambia niente.
func compatibilitaDelNodo(t ProdottoRichiesto, n NodoStruttura) motorea.Compatibilita {
	migliore := motorea.CompatibilitaNonDeterminabile
	for _, l := range n.Letture {
		if l.Forma.Namespace != t.Namespace {
			continue
		}
		if c := conMarcatore(motorea.ConfrontaBasi(t.Base, l.Forma.Base), t.Marcatore, marcatoreDi(l.Forma)); rangoCompatibilita(c) > rangoCompatibilita(migliore) {
			migliore = c
		}
	}
	return migliore
}

// rangoCompatibilita: l'ordine delle compatibilità, dalla più debole.
func rangoCompatibilita(c motorea.Compatibilita) int {
	switch c {
	case motorea.CompatibilitaDiscordante:
		return 1
	case motorea.CompatibilitaParziale:
		return 2
	case motorea.CompatibilitaEquivalente:
		return 3
	case motorea.CompatibilitaUguale:
		return 4
	}
	return 0
}

// compatibile: la base del nodo è quella del target (uguale, o compatibile con una forma parziale: A-C07).
func compatibile(c motorea.Compatibilita) bool {
	return c == motorea.CompatibilitaUguale || c == motorea.CompatibilitaEquivalente || c == motorea.CompatibilitaParziale
}

// radiceScelta: il file e il nodo della radice scelta della fonte confermata (R76 A), se la fonte la dice tutta e la
// radice si trova fra le strutture: tipo step (controllato prima), sha256, radice registrata (T-B0-08), non superata.
// A parità di contenuto vale l'allegato del riferimento, altrimenti il primo in ordine di ID.
func (c *contestoIndicizzato) radiceScelta(f *RiferimentoFonte) (*fileIndicizzato, NodoStruttura, bool) {
	if f == nil || f.Superato || f.Sha256 == "" || f.Radice == "" {
		return nil, NodoStruttura{}, false
	}
	rif := RifNodo(f.Sha256, f.Radice)
	var scelto *fileIndicizzato
	for _, x := range c.file {
		if _, ok := x.nodi[rif]; !ok || x.s.Sha256 != f.Sha256 {
			continue
		}
		if f.AllegatoID != nil && x.s.AllegatoID == *f.AllegatoID {
			return x, x.nodi[rif], true
		}
		if scelto == nil {
			scelto = x
		}
	}
	if scelto == nil {
		return nil, NodoStruttura{}, false
	}
	return scelto, scelto.nodi[rif], true
}

// struttura costruisce la struttura candidata di un target in un file, sotto una radice (regola 2).
func (c *contestoIndicizzato) struttura(t ProdottoRichiesto, f *fileIndicizzato, radice NodoStruttura, comp motorea.Compatibilita) StrutturaProdotto {
	s := StrutturaProdotto{Target: t.Rif, AllegatoID: f.s.AllegatoID, Sha256: f.s.Sha256, Radice: radice.Rif,
		RadiceDelFile: f.radici[radice.Rif], Compatibilita: comp, Stato: StatoStrutturaCandidata,
		GrafoCompleto: f.s.GrafoCompleto, MotivoGrafo: f.s.MotivoGrafo}

	// I nodi raggiungibili dalla radice negli archi dei fatti, in ampiezza, con i figli in ordine di arco. Ogni nodo
	// entra una volta sola, anche con più padri o con un ciclo; ogni arco una volta sola, quando si visita il padre.
	dentro := map[string]bool{radice.Rif: true}
	padri := map[string][]string{}
	coda := []string{radice.Rif}
	for len(coda) > 0 {
		r := coda[0]
		coda = coda[1:]
		for _, a := range f.figli[r] {
			s.Archi = append(s.Archi, ArcoProposto{Padre: a.Padre, Figlio: a.Figlio, Quantita: copiaIntero(a.Quantita), Occorrenze: copiaStringhe(a.Occorrenze)})
			padri[a.Figlio] = append(padri[a.Figlio], a.Padre)
			if !dentro[a.Figlio] {
				dentro[a.Figlio] = true
				coda = append(coda, a.Figlio)
			}
		}
	}
	altri := make([]string, 0, len(dentro))
	for r := range dentro {
		if r != radice.Rif {
			altri = append(altri, r)
		}
	}
	sort.Strings(altri)
	rifs := append([]string{radice.Rif}, altri...)
	for _, r := range rifs {
		n := f.nodi[r]
		p := append([]string(nil), padri[r]...)
		sort.Strings(p)
		s.Nodi = append(s.Nodi, NodoProposto{Rif: r, AllegatoID: f.s.AllegatoID, EntitaID: n.EntitaID, Padri: copiaStringhe(p),
			Leggibile: len(n.Letture) > 0, SenzaFile: true})
	}
	sort.SliceStable(s.Archi, func(i, j int) bool {
		if s.Archi[i].Padre != s.Archi[j].Padre {
			return s.Archi[i].Padre < s.Archi[j].Padre
		}
		return s.Archi[i].Figlio < s.Archi[j].Figlio
	})
	c.decisioniENodi(t, f, &s)

	// Gli archi del contesto: i proposti con il padre nella struttura, i confermati raggiungibili dal componente.
	var contesto []ArcoPercorso
	for _, r := range rifs {
		for _, a := range c.proposti[r] {
			contesto = append(contesto, copiaArco(a))
		}
	}
	if t.ComponenteID != nil {
		contesto = append(contesto, c.confermatiDa(RifComponente(*t.ComponenteID))...)
	}
	ordinaArchi(contesto)
	s.ArchiContesto = contesto

	// Le righe aperte dei nodi della struttura: quella della radice a parte (R80).
	for _, riga := range c.righe[f.s.AllegatoID] {
		nodo := RifNodo(riga.Sha256, riga.Chiave)
		switch {
		case !dentro[nodo]:
		case nodo == radice.Rif:
			s.RigaRadice = RifRigaProposta(riga.ID)
		default:
			s.RigheDaDecidere = append(s.RigheDaDecidere, RifRigaProposta(riga.ID))
		}
	}
	sort.Strings(s.RigheDaDecidere)

	// Il file come documento candidato della fonte del target, se lo è (R76 b A; T-B0-07).
	if s.RadiceDelFile && compatibile(comp) && (t.FonteConfermata == nil || t.FonteConfermata.Sha256 != f.s.Sha256) {
		aid := f.s.AllegatoID
		var radici []string
		for _, r := range f.s.Radici {
			radici = append(radici, f.nodi[r].Chiave)
		}
		sort.Strings(radici)
		s.Fonti = []DocumentoCandidato{{Origine: CandidatoDaMotoreA, AllegatoID: &aid, Sha256: f.s.Sha256, Radici: radici,
			RadiceCompatibile: radice.Chiave, Compatibilita: comp, Estrazione: EstrazioneRiuscita,
			GrafoCompleto: f.s.GrafoCompleto, MotivoGrafo: f.s.MotivoGrafo}}
	}
	return s
}

// decisioniENodi (B4, fase 1): per ogni nodo della struttura la riga legacy dello stesso nodo, aperta o decisa, la
// decisione per UUID e la catena del codice; per ogni arco l'arco confermato accanto. Le decisioni prima delle catene,
// perché l'entità degli indizi di un nodo dipende dalla sua decisione (T-E1-06).
func (c *contestoIndicizzato) decisioniENodi(t ProdottoRichiesto, f *fileIndicizzato, s *StrutturaProdotto) {
	perRif := map[string]*NodoProposto{}
	for i := range s.Nodi {
		n := &s.Nodi[i]
		perRif[n.Rif] = n
		k := chiaveRigaNodo(f.s.AllegatoID, n.Rif)
		if r, ok := c.aperte[k]; ok {
			n.RigaLegacy = copiaRigaAperta(r)
		}
		r, ok := c.decise[k]
		if !ok {
			continue
		}
		n.RigaDecisa = copiaRigaDecisa(r)
		n.DecisoDaPersona = r.DecisoDa != nil
		if r.ComponenteID == nil || (r.Stato != StatoRigaConfermata && r.Stato != StatoRigaDuplicato) {
			continue
		}
		if d, ok := c.componenti[*r.ComponenteID]; ok {
			n.Decisione = copiaComponente(d)
		}
	}
	for i := range s.Nodi {
		n := &s.Nodi[i]
		n.Codice = c.catena(nodoDellaCatena{t: t, f: f, s: s, n: n, letto: f.nodi[n.Rif]})
		n.AbbinamentoPerBase = c.abbinamento(n)
	}
	for i := range s.Archi {
		a := &s.Archi[i]
		p, q := perRif[a.Padre], perRif[a.Figlio]
		if p == nil || q == nil || p.Decisione == nil || q.Decisione == nil {
			continue
		}
		if d, ok := c.coppieConfermate[RifComponente(p.Decisione.ComponenteID)+"\x00"+RifComponente(q.Decisione.ComponenteID)]; ok {
			x := copiaArco(d)
			a.Decisione = &x
		}
	}
}

// archiDellaRadiceScelta (T-B4-23): sulla bom_di_lavoro_proposta la radice è quella che l'operatore ha scelto con il
// gesto 3 per il componente del target (ProdottoRichiesto.ComponenteID): solo per gli archi, la radice vale quel
// componente, e un arco radice→figlio con il figlio deciso prende accanto la relazione confermata fra il componente del
// target e il componente del figlio. NodoProposto.Decisione della radice resta nil (nessuna riga decisa la porta),
// una radice con la sua riga decisa segue la regola di sempre, e sulla struttura candidata non si lega niente (R59 A).
func (c *contestoIndicizzato) archiDellaRadiceScelta(t ProdottoRichiesto, s *StrutturaProdotto) {
	if t.ComponenteID == nil || s.Stato != StatoBOMDiLavoroProposta {
		return
	}
	nodi := map[string]*NodoProposto{}
	for i := range s.Nodi {
		nodi[s.Nodi[i].Rif] = &s.Nodi[i]
	}
	if r := nodi[s.Radice]; r == nil || r.Decisione != nil {
		return
	}
	padre := RifComponente(*t.ComponenteID)
	for i := range s.Archi {
		a := &s.Archi[i]
		q := nodi[a.Figlio]
		if a.Padre != s.Radice || a.Decisione != nil || q == nil || q.Decisione == nil {
			continue
		}
		if d, ok := c.coppieConfermate[padre+"\x00"+RifComponente(q.Decisione.ComponenteID)]; ok {
			x := copiaArco(d)
			a.Decisione = &x
		}
	}
}

// confermatoDellaRadiceScelta (T-B4-38; R76 A): sulla bom_di_lavoro_proposta la radice scelta con il gesto 3
// rappresenta il prodotto, quindi la sua catena del codice porta accanto il codice confermato del componente del target
// (ProdottoRichiesto.ComponenteID), letto come quello degli altri componenti (codiceConfermato: componente.codice e
// componente.rev con la provenienza di T-E1-22, o la DecisioneIdentita su quel componente con LettureDecise, che dà lo
// stato confermata). Così la discordanza fra il cartiglio del 2D del prodotto e la decisione si calcola. La Decisione
// della radice resta nil (T-B4-23): nessuna riga decisa la lega al componente; una radice con la sua riga decisa segue
// la regola di sempre; sulla struttura candidata niente (R59 A); un componente del target che non è fra i confermati del
// contesto non dà niente.
func (c *contestoIndicizzato) confermatoDellaRadiceScelta(t ProdottoRichiesto, s *StrutturaProdotto) {
	if t.ComponenteID == nil || s.Stato != StatoBOMDiLavoroProposta {
		return
	}
	d, ok := c.componenti[*t.ComponenteID]
	if !ok {
		return
	}
	for i := range s.Nodi {
		n := &s.Nodi[i]
		if n.Rif != s.Radice || n.Decisione != nil {
			continue
		}
		n.Codice.Confermato = c.codiceConfermato(copiaComponente(d))
		n.Codice.Identita.StatoRevisione = statoRevisione(n.Codice)
	}
}

// abbinamento (B4, fase 2; emendamento E1 §4.2, punto 2; T-B4-12, T-B4-26, T-B4-27): l'abbinamento per base del nodo,
// dopo la catena, perché usa la lettura scelta (Codice.Forma).
//   - Con la decisione per UUID nel contesto: il motivo solo quando la base del componente non è più compatibile
//     (decisione_non_compatibile) o non si può verificare (decisione_non_verificabile: il codice non è letto, è letto in
//     un altro namespace, o il nodo non ha un'identità); nessuna proposta per base.
//   - Con il nodo scartato: nodo_scartato, nessuna proposta (T-B4-27).
//   - Con la riga decisa che porta un componente che non è fra i confermati del contesto: decisione_non_verificabile con
//     quel componente, e accanto i componenti con la stessa identità, con il motivo distinto (T-B4-26).
//   - Altrimenti i componenti confermati del contesto con la stessa identità (stessaIdentita, T-B4-06 rivisto), in
//     ordine di componente, come proposta.
func (c *contestoIndicizzato) abbinamento(n *NodoProposto) *AbbinamentoPerBase {
	forma := n.Codice.Forma
	if d := n.Decisione; d != nil {
		id := d.ComponenteID
		if forma == nil || d.Lettura == nil {
			return &AbbinamentoPerBase{Decisione: &id, MotivoDecisione: MotivoAbbinamentoDecisioneNonVerificabile, Compatibilita: motorea.CompatibilitaNonDeterminabile}
		}
		switch comp := compatibilitaIdentita(*forma, *d.Lettura); {
		case compatibile(comp):
			return nil
		case comp == motorea.CompatibilitaNonDeterminabile:
			return &AbbinamentoPerBase{Decisione: &id, MotivoDecisione: MotivoAbbinamentoDecisioneNonVerificabile, Compatibilita: comp}
		default:
			return &AbbinamentoPerBase{Decisione: &id, MotivoDecisione: MotivoAbbinamentoDecisioneNonCompatibile, Compatibilita: comp}
		}
	}
	r := n.RigaDecisa
	if r != nil && r.Stato == StatoRigaScartata {
		return &AbbinamentoPerBase{Motivo: MotivoNodoScartato}
	}
	var ids []uuid.UUID
	if forma != nil {
		for _, k := range c.confermatiInOrdine {
			if k.Lettura != nil && stessaIdentita(*forma, *k.Lettura) {
				ids = append(ids, k.ComponenteID)
			}
		}
	}
	if r != nil && r.ComponenteID != nil {
		a := &AbbinamentoPerBase{Decisione: copiaUUID(r.ComponenteID), MotivoDecisione: MotivoAbbinamentoDecisioneNonVerificabile,
			Compatibilita: motorea.CompatibilitaNonDeterminabile}
		if len(ids) > 0 {
			a.Componenti, a.Motivo = ids, MotivoAbbinamentoAccantoAllaRigaDecisa
		}
		return a
	}
	switch len(ids) {
	case 0:
		return nil
	case 1:
		return &AbbinamentoPerBase{Componenti: ids, Motivo: MotivoAbbinamentoPerBase}
	}
	return &AbbinamentoPerBase{Componenti: ids, Motivo: MotivoAbbinamentoComponentiStessaBase}
}

func copiaRigaAperta(r RigaPropostaLegacy) *RigaPropostaLegacy {
	r.CodiceManuale, r.RevManuale = copiaTesto(r.CodiceManuale), copiaTesto(r.RevManuale)
	if r.LetturaManuale != nil {
		l := copiaLetturaForma(*r.LetturaManuale)
		r.LetturaManuale = &l
	}
	return &r
}

func copiaRigaDecisa(r RigaDecisaLegacy) *RigaDecisaLegacy {
	r.ComponenteID, r.DecisoDa = copiaUUID(r.ComponenteID), copiaUUID(r.DecisoDa)
	return &r
}

func copiaComponente(d ComponenteDeciso) *ComponenteDeciso {
	d.Rev = copiaTesto(d.Rev)
	if d.Lettura != nil {
		l := copiaLetturaForma(*d.Lettura)
		d.Lettura = &l
	}
	return &d
}

// confermatiDa: le relazioni confermate raggiungibili da un componente, in ampiezza, ognuna una volta sola.
func (c *contestoIndicizzato) confermatiDa(radice string) []ArcoPercorso {
	visti := map[string]bool{radice: true}
	coda := []string{radice}
	var out []ArcoPercorso
	for len(coda) > 0 {
		r := coda[0]
		coda = coda[1:]
		for _, a := range c.confermati[r] {
			out = append(out, copiaArco(a))
			if !visti[a.Figlio] {
				visti[a.Figlio] = true
				coda = append(coda, a.Figlio)
			}
		}
	}
	return out
}

// copiaArco: l'arco con gli elenchi e i puntatori copiati.
func copiaArco(a ArcoPercorso) ArcoPercorso {
	a.Quantita = copiaIntero(a.Quantita)
	a.Occorrenze = copiaStringhe(a.Occorrenze)
	return a
}

// ---- i controlli di contratto ----

// controllaContesto: i controlli di contratto di ProponiStrutture, tutti insieme, nell'ordine degli ingressi. Con i
// codici della foglia, come ProponiProdotti:
//   - documento.id_ripetuto: due target con lo stesso Rif; due strutture dello stesso allegato; due nodi con lo stesso
//     Rif o lo stesso arco due volte in una struttura (i fatti aggregano per coppia); due componenti uguali; la stessa
//     relazione confermata due volte; due righe con lo stesso ID o con lo stesso (allegato, chiave);
//   - documento.enum_ignoto: un target con l'autorità proposta (R60 A); una fonte confermata che non è uno STEP (R68
//     A, T-E1-09); un componente che non è confermato, una riga che non è proposta (R29 e); un arco con l'origine
//     sbagliata per il suo elenco;
//   - documento.riferimento_pendente: un target senza Rif; una struttura senza allegato o senza sha256, un nodo senza
//     chiave, una riga senza allegato, sha256 o chiave (fuori dal suo file la chiave non significa niente: T-E1-04,
//     emendamento E1 §4.2); un nodo il cui Rif non è sha256 più chiave; una radice o un estremo d'arco che non è un nodo
//     della struttura; un arco confermato fra riferimenti che non sono componenti, un arco proposto fra riferimenti
//     che non sono nodi; una riga di un altro contenuto della struttura del suo allegato.
//
// Con B4, gli stessi codici per gli ingressi della catena e delle decisioni: una revisione o una lettura del codice
// manuale senza il codice manuale; una riga decisa ripetuta (anche con l'ID di una aperta), un secondo nodo con lo
// stesso (allegato, chiave), senza allegato, sha256 o chiave, con uno stato che non è confermata, duplicato o
// scartata, confermata o duplicato senza componente, di un altro contenuto; un codice di un messaggio senza il
// messaggio o ripetuto; una decisione sull'identità con un oggetto ignoto, senza oggetto, senza chi o quando, senza il
// codice (T-B4-21), con un'evidenza vista di una fonte fuori elenco, o ripetuta per lo stesso oggetto. Dalla fase 3:
// la lettura di un codice deciso con una chiave che non è il Rif di un componente con una decisione sull'identità
// (T-B4-32). Le associazioni decise le controlla ProponiAncoraggi, che ha i file.
func controllaContesto(target []ProdottoRichiesto, ctx ContestoStrutturale) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	// d porta solo il codice, scritto da chi chiama con la sua costante (A1a-CAT, regola 5); qui il resto.
	errore := func(d evidenze.Diagnostica, percorso, messaggio string, rif ...string) {
		d.Gravita, d.Natura, d.Percorso, d.Messaggio, d.Rif = evidenze.GravitaErrore, evidenze.NaturaContratto, percorso, messaggio, rif
		out = append(out, d)
	}

	visti := map[string]bool{}
	for i, t := range target {
		p := fmt.Sprintf("%s[%d]", percorsoTarget, i)
		switch {
		case t.Rif == "":
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".rif", "un target senza riferimento")
		case visti[t.Rif]:
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".rif", fmt.Sprintf("il target %s compare due volte", t.Rif), t.Rif)
		}
		visti[t.Rif] = true
		if t.Autorita != AutoritaConfermata && t.Autorita != AutoritaScenario {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".autorita",
				fmt.Sprintf("autorità %q: un target è confermato o di scenario, mai proposto (R60 A)", t.Autorita), t.Rif)
		}
		if f := t.FonteConfermata; f != nil && f.Tipo != TipoRiferimentoStep {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".fonte_confermata.tipo",
				fmt.Sprintf("tipo %q: solo uno STEP è fonte strutturale (R68 A, T-E1-09)", f.Tipo), t.Rif)
		}
	}

	allegati := map[uuid.UUID]string{} // allegato → sha256 della sua struttura
	for i, s := range ctx.Strutture {
		p := fmt.Sprintf("%s.strutture[%d]", percorsoContesto, i)
		if _, ok := allegati[s.AllegatoID]; ok {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".allegato_id", fmt.Sprintf("due strutture dell'allegato %s", s.AllegatoID))
		}
		allegati[s.AllegatoID] = s.Sha256
		if s.AllegatoID == uuid.Nil {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".allegato_id", "una struttura senza allegato: "+messaggioSenzaFile)
		}
		if s.Sha256 == "" {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".sha256", "una struttura senza sha256: "+messaggioSenzaFile)
		}
		nodi := map[string]bool{}
		for j, n := range s.Nodi {
			pn := fmt.Sprintf("%s.nodi[%d]", p, j)
			if n.Chiave == "" {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, pn+".chiave", "un nodo senza chiave: "+messaggioSenzaFile, n.Rif)
			}
			if n.Rif != RifNodo(s.Sha256, n.Chiave) {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, pn+".rif", "il riferimento del nodo non è lo sha256 del file più la chiave (T-E1-04)", n.Rif)
			}
			if nodi[n.Rif] {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, pn+".rif", "lo stesso nodo due volte nella struttura", n.Rif)
			}
			nodi[n.Rif] = true
		}
		for j, r := range s.Radici {
			if !nodi[r] {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, fmt.Sprintf("%s.radici[%d]", p, j), "una radice che non è un nodo della struttura", r)
			}
		}
		coppie := map[string]bool{}
		for j, a := range s.Archi {
			pa := fmt.Sprintf("%s.archi[%d]", p, j)
			if a.Origine != OrigineArcoFatti {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, pa+".origine", fmt.Sprintf("origine %q: gli archi di una struttura vengono dai fatti", a.Origine))
			}
			if !nodi[a.Padre] || !nodi[a.Figlio] {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, pa, "un arco con un estremo che non è un nodo della struttura", a.Padre, a.Figlio)
			}
			if k := a.Padre + "\x00" + a.Figlio; coppie[k] {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, pa, "lo stesso arco due volte: i fatti aggregano per coppia", a.Padre, a.Figlio)
			} else {
				coppie[k] = true
			}
		}
	}

	componenti := map[uuid.UUID]bool{}
	for i, c := range ctx.Confermato {
		p := fmt.Sprintf("%s.confermato[%d]", percorsoContesto, i)
		if componenti[c.ComponenteID] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".componente_id", "lo stesso componente due volte", RifComponente(c.ComponenteID))
		}
		componenti[c.ComponenteID] = true
		if c.Autorita != AutoritaConfermata || c.Origine != OrigineConfermato {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p, fmt.Sprintf("autorità %q e origine %q: un componente deciso è confermato, e confermato è solo ciò che l'operatore ha confermato (R29 e)",
				c.Autorita, c.Origine), RifComponente(c.ComponenteID))
		}
	}
	controllaArchi(ctx.ArchiConfermati, percorsoContesto+".archi_confermati", OrigineArcoConfermato, prefissoRifComponente, true, errore)

	righe := map[uuid.UUID]bool{}
	chiavi := map[string]bool{}
	for i, r := range ctx.Proposto {
		p := fmt.Sprintf("%s.proposto[%d]", percorsoContesto, i)
		if righe[r.ID] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".id", "la stessa riga due volte", RifRigaProposta(r.ID))
		}
		righe[r.ID] = true
		if k := r.AllegatoID.String() + "\x00" + r.Chiave; chiavi[k] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".chiave", "due righe aperte per lo stesso nodo dello stesso allegato", RifRigaProposta(r.ID))
		} else {
			chiavi[k] = true
		}
		for _, x := range []struct {
			vuoto bool
			campo string
		}{{r.AllegatoID == uuid.Nil, "allegato_id"}, {r.Sha256 == "", "sha256"}, {r.Chiave == "", "chiave"}} {
			if x.vuoto {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+"."+x.campo, "una riga senza "+x.campo+": "+messaggioSenzaFile, RifRigaProposta(r.ID))
			}
		}
		if r.Autorita != AutoritaProposta || r.Origine != OrigineProposto {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p, fmt.Sprintf("autorità %q e origine %q: %s", r.Autorita, r.Origine, messaggioRigaNonProposta), RifRigaProposta(r.ID))
		}
		if sha, ok := allegati[r.AllegatoID]; ok && sha != r.Sha256 {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".sha256", "la riga è di un altro contenuto della struttura del suo allegato", RifRigaProposta(r.ID))
		}
		if r.CodiceManuale == nil && (r.RevManuale != nil || r.LetturaManuale != nil) {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".codice_manuale",
				"una revisione o una lettura del codice manuale senza il codice manuale (T-B0-33)", RifRigaProposta(r.ID))
		}
	}
	controllaArchi(ctx.ArchiProposti, percorsoContesto+".archi_proposti", OrigineArcoProposto, PrefissoRifNodo, false, errore)

	// B4: le righe decise, i codici dei messaggi, le decisioni sull'identità.
	for i, r := range ctx.Decise {
		p := fmt.Sprintf("%s.decise[%d]", percorsoContesto, i)
		if righe[r.ID] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".id", "la stessa riga due volte, fra le aperte o fra le decise", RifRigaProposta(r.ID))
		}
		righe[r.ID] = true
		if k := r.AllegatoID.String() + "\x00" + r.Chiave; chiavi[k] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".chiave", "due righe per lo stesso nodo dello stesso allegato: una riga legacy per nodo (UNIQUE allegato, chiave)", RifRigaProposta(r.ID))
		} else {
			chiavi[k] = true
		}
		for _, x := range []struct {
			vuoto bool
			campo string
		}{{r.AllegatoID == uuid.Nil, "allegato_id"}, {r.Sha256 == "", "sha256"}, {r.Chiave == "", "chiave"}} {
			if x.vuoto {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+"."+x.campo, "una riga decisa senza "+x.campo+": "+messaggioSenzaFile, RifRigaProposta(r.ID))
			}
		}
		switch r.Stato {
		case StatoRigaConfermata, StatoRigaDuplicato:
			if r.ComponenteID == nil {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".componente_id",
					fmt.Sprintf("una riga %s senza componente: il nodo è quel componente (CHECK della 0018)", r.Stato), RifRigaProposta(r.ID))
			}
		case StatoRigaScartata:
		default:
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".stato",
				fmt.Sprintf("stato %q: una riga decisa è confermata, duplicato o scartata; una riga aperta sta fra le proposte", r.Stato), RifRigaProposta(r.ID))
		}
		if sha, ok := allegati[r.AllegatoID]; ok && sha != r.Sha256 {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".sha256", "la riga decisa è di un altro contenuto della struttura del suo allegato", RifRigaProposta(r.ID))
		}
	}
	letture := map[string]bool{}
	for i, m := range ctx.CodiciMessaggi {
		p := fmt.Sprintf("%s.codici_messaggi[%d]", percorsoContesto, i)
		if m.MessaggioID == uuid.Nil {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".messaggio_id", "un codice di un messaggio senza il messaggio (T-E1-06)", m.Lettura.ID)
		}
		if k := m.MessaggioID.String() + "\x00" + m.Lettura.ID; letture[k] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".lettura", "la stessa lettura dello stesso messaggio due volte", m.Lettura.ID)
		} else {
			letture[k] = true
		}
	}
	decisioni := map[string]bool{}
	for i, d := range ctx.DecisioniIdentita {
		p := fmt.Sprintf("%s.decisioni_identita[%d]", percorsoContesto, i)
		if d.Oggetto != OggettoDecisioneComponente && d.Oggetto != OggettoDecisioneDocumento {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".oggetto", fmt.Sprintf("oggetto %q: una decisione sull'identità è di un componente o di un documento (E1R)", d.Oggetto))
		}
		if d.ID == uuid.Nil {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".id", "una decisione sull'identità senza il suo oggetto")
		}
		if d.Da == uuid.Nil || d.Il.IsZero() {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".da",
				"una decisione sull'identità senza chi o quando: una decisione è sempre un gesto tracciato (R29 e, E1R)", d.ID.String())
		}
		if strings.TrimSpace(d.Codice) == "" {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".codice",
				"una decisione sull'identità senza il codice deciso: non cancella il codice dell'oggetto (T-B4-21)", d.ID.String())
		}
		for j, e := range d.EvidenzeViste {
			if !fontiEvidenza[e.Fonte] {
				errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, fmt.Sprintf("%s.evidenze_viste[%d].fonte", p, j),
					fmt.Sprintf("fonte %q: un'evidenza vista è del cartiglio, dell'entità STEP, del nome del file o del documento (contratto §2.6)", e.Fonte), d.ID.String())
			}
		}
		if k := d.Oggetto + ":" + d.ID.String(); decisioni[k] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".id", "due decisioni sull'identità dello stesso oggetto: la decisione è il valore corrente, una sola", k)
		} else {
			decisioni[k] = true
		}
	}
	// B4, fase 3: la lettura di un codice deciso è di una decisione sull'identità di un componente, per il suo Rif
	// (T-B4-32). Le chiavi si guardano in ordine, perché una mappa non ne ha uno.
	chiaviLetture := make([]string, 0, len(ctx.LettureDecise))
	for k := range ctx.LettureDecise {
		chiaviLetture = append(chiaviLetture, k)
	}
	sort.Strings(chiaviLetture)
	for _, k := range chiaviLetture {
		id, err := uuid.Parse(strings.TrimPrefix(k, prefissoRifComponente))
		if err != nil || k != RifComponente(id) || !decisioni[OggettoDecisioneComponente+":"+id.String()] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, percorsoContesto+".letture_decise["+k+"]",
				"la lettura di un codice deciso è di una decisione sull'identità di un componente, per il suo riferimento «componente:<uuid>» (T-B4-32)", k)
		}
	}
	return out
}

// controllaArchi: i controlli di un elenco di archi del contesto: l'origine dell'elenco, gli estremi con il prefisso
// giusto e, se unici, nessuna coppia ripetuta (una relazione confermata è una per coppia; gli archi proposti di due
// allegati con lo stesso contenuto hanno gli stessi riferimenti, e i doppioni esatti si tolgono).
func controllaArchi(archi []ArcoPercorso, percorso, origine, prefisso string, unici bool, errore func(d evidenze.Diagnostica, percorso, messaggio string, rif ...string)) {
	coppie := map[string]bool{}
	for i, a := range archi {
		p := fmt.Sprintf("%s[%d]", percorso, i)
		if a.Origine != origine {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".origine", fmt.Sprintf("origine %q in un elenco di archi «%s»", a.Origine, origine))
		}
		if !strings.HasPrefix(a.Padre, prefisso) || !strings.HasPrefix(a.Figlio, prefisso) || a.Padre == prefisso || a.Figlio == prefisso {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p, fmt.Sprintf("gli estremi di un arco «%s» sono riferimenti «%s…»", origine, prefisso), a.Padre, a.Figlio)
		}
		if !unici {
			continue
		}
		if k := a.Padre + "\x00" + a.Figlio; coppie[k] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p, "la stessa relazione confermata due volte", a.Padre, a.Figlio)
		} else {
			coppie[k] = true
		}
	}
}
