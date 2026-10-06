package valutazione

import (
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
)

// La completezza documentale, l'asse 6 (B5, fase 3; R62, R72 D [R], R82, R93, R99 A, R102 A, R103 C; contratto §1.0
// riga 6, §1.6, §2.3, §2.5, §2.6; T-B0-07, T-B0-38, T-E1-10, T-E1-17, T-E1-18, T-E1R-01; K-01 con la lettura A,
// confermata dall'utente [U]). La regola (Completezza) è una funzione pura su un ingresso astratto (IngressoCompletezza):
// le righe confermate del perimetro con i loro fabbisogni, gli esiti della vista e i gruppi dei 2D, i nodi da
// prevedere, la struttura della verifica della BOM (per il perimetro), le conferme della categoria. Il legacy entra solo
// dall'adattatore (completezza_thread.go: T-B0-22). «completa» non dice niente della nomenclatura.

// StatoDocumenti: lo stato della completezza, l'asse 6 (contratto §2.3; R79, T-B0-38).
type StatoDocumenti string

const (
	DocumentiNonCalcolabile StatoDocumenti = "non_calcolabile"
	DocumentiIncompleta     StatoDocumenti = "incompleta"
	DocumentiCompleta       StatoDocumenti = "completa"
)

// EsitoFabbisogno: l'esito di una voce (contratto §2.3; era EsitoPDF).
type EsitoFabbisogno string

const (
	EsitoPresente     EsitoFabbisogno = "presente"
	EsitoDaVerificare EsitoFabbisogno = "da_verificare"
	EsitoManca        EsitoFabbisogno = "manca"
)

// I valori di VoceFabbisogno.NotaRegola e FabbisognoInformativo.NotaRegola (contratto §2.6; E1R §4.2): quale regola
// dello schema o del cliente tocca il 2D della voce. Non vuol dire per forza che A1c si scosta dalla vista:
//   - schema_senza_2d, regola_cliente_senza_2d: la vista non ha la voce, A1c sì (R103 C, R99 A);
//   - regola_cliente_2d_non_bloccante sul finito: la vista la dà non bloccante, A1c bloccante (R99 A); su un componente,
//     con la lettura A di K-01, A1c coincide con la vista e la voce sta fra i non bloccanti.
const (
	NotaSchemaSenza2D               = "schema_senza_2d"
	NotaRegolaClienteSenza2D        = "regola_cliente_senza_2d"
	NotaRegolaCliente2DNonBloccante = "regola_cliente_2d_non_bloccante"
)

// I motivi della completezza (CompletezzaDocumentale.Motivo; il contratto non li fissa: scelte della fase 3, dubbio
// T-B5-52), il primo che vale:
//   - non_determinabile: una sezione della fotografia da cui la completezza dipende è assente (T-12);
//   - target_senza_componente: il target non ha la riga componente, quindi nessuna BOM confermata (T-B0-07);
//   - voce_mancante: una voce certa è «manca» (R72 D: una mancanza certa, anche con il perimetro aperto);
//   - voce_non_presente: il perimetro è chiuso e una voce certa non è «presente»;
//   - perimetro_aperto: il perimetro è aperto e nessuna mancanza è certa (con i Previsti);
//   - "" per «completa».
const (
	MotivoDocumentiNonDeterminabile      = "non_determinabile"
	MotivoDocumentiTargetSenzaComponente = "target_senza_componente"
	MotivoDocumentiVoceMancante          = "voce_mancante"
	MotivoDocumentiVoceNonPresente       = "voce_non_presente"
	MotivoDocumentiPerimetroAperto       = "perimetro_aperto"
)

// I motivi di un perimetro aperto (CompletezzaDocumentale.MotivoPerimetro; contratto §2.5, valori della fase 3, dubbio
// T-B5-52), il primo che vale (ChiusuraDelPerimetro):
//   - target_senza_componente (T-B0-07);
//   - fonte_non_confermata: senza FonteSTEPConfermata il perimetro non si chiude (R102 A);
//   - bom_di_lavoro_assente: la fonte è confermata, ma la BOM di lavoro non c'è (l'analisi in corso, la radice non
//     registrata, il riferimento superato, nessuna struttura): una BOM di lavoro assente non è una BOM di lavoro con la
//     sola radice (T-B5-92);
//   - nodi_da_decidere: un nodo della BOM di lavoro, esclusa la riga della radice, senza la decisione di una persona;
//   - archi_da_decidere: una riga d'arco della BOM di lavoro senza la decisione di una persona;
//   - rimozioni_aperte: una rimozione proposta aperta sul perimetro di P.
//
// Con una sezione della fotografia assente il perimetro non si calcola (MotivoPerimetroNonDeterminabile, nell'adattatore:
// T-12).
const (
	MotivoPerimetroTargetSenzaComponente = "target_senza_componente"
	MotivoPerimetroFonteNonConfermata    = "fonte_non_confermata"
	MotivoPerimetroBOMDiLavoroAssente    = "bom_di_lavoro_assente"
	MotivoPerimetroNodiDaDecidere        = "nodi_da_decidere"
	MotivoPerimetroArchiDaDecidere       = "archi_da_decidere"
	MotivoPerimetroRimozioniAperte       = "rimozioni_aperte"
)

// I motivi di un fabbisogno previsto (FabbisognoPrevisto.Motivo; campo in più della fase 3, dubbio T-B5-53):
//   - nodo_proposto: un nodo della struttura candidata o della BOM di lavoro senza decisione, con il tipo proposto;
//   - nodo_deciso_fuori_perimetro: un nodo deciso come un componente confermato che il perimetro di P non raggiunge (la
//     relazione confermata non c'è);
//   - rimozione_aperta: una riga confermata del perimetro con una rimozione proposta aperta: potrebbe uscire dal
//     perimetro, quindi le sue voci non sono certe (contratto §1.6; decisioni dell'orchestratore sull'analista, punto 3).
const (
	MotivoPrevistoNodoProposto             = "nodo_proposto"
	MotivoPrevistoNodoDecisoFuoriPerimetro = "nodo_deciso_fuori_perimetro"
	MotivoPrevistoRimozioneAperta          = "rimozione_aperta"
)

// Gli esiti di v_fascicolo (0020: ok, ok_in_coda, ok_errore_nas, derogato, da_confermare, sul_portale, manca), ripetuti
// qui perché il legacy non li esporta.
const (
	esitoFascicoloOK           = "ok"
	esitoFascicoloOKInCoda     = "ok_in_coda"
	esitoFascicoloOKErroreNas  = "ok_errore_nas"
	esitoFascicoloDerogato     = "derogato"
	esitoFascicoloDaConfermare = "da_confermare"
	esitoFascicoloSulPortale   = "sul_portale"
	esitoFascicoloManca        = "manca"
)

// VoceFabbisogno: una voce certa, per (componente, tipo di documento) bloccante (contratto §2.3, §2.5, §2.6; R82).
//   - ComponenteID, Codice, TipoComponente, DelProdotto: la riga confermata (DelProdotto: il componente del target).
//   - TipoDocumento, Bloccante: il fabbisogno; RegolaCliente: i fabbisogni del suo tipo sono del cliente
//     (RigaFabbisogno.Proprio sulle righe dello stesso tipo di componente: decisioni sull'analista, punto 8).
//   - Esito, Motivo: per il 2D dal gruppo (VoceDelDisegno); per gli altri tipi dalla vista (EsitoDallaVista).
//   - Disegni: il gruppo dei 2D del componente (solo per il 2D; nil senza 2D). DocumentoID, StatoNas: il documento della
//     vista (gli altri tipi). DerogaID, PropostaAperta: la deroga e la proposta aperta della vista. FileCandidato:
//     l'allegato che porta la voce a da_verificare per un'associazione non confermata.
//   - CalcolataDa: vista (l'esito della vista, gli altri tipi) o go (il 2D, e ciò che la vista non ha).
//   - Invariante: un requisito che le regole del cliente non tolgono (R99, R103 C): il 2D. NotaRegola: quale regola
//     tocca il 2D (NotaSchemaSenza2D, NotaRegolaClienteSenza2D, NotaRegolaCliente2DNonBloccante). Categoria: la
//     categoria della Classificazione del componente.
type VoceFabbisogno struct {
	ComponenteID   uuid.UUID        `json:"componente_id"`
	Codice         string           `json:"codice"`
	TipoComponente string           `json:"tipo_componente"`
	DelProdotto    bool             `json:"del_prodotto"`
	TipoDocumento  string           `json:"tipo_documento"`
	Bloccante      bool             `json:"bloccante"`
	RegolaCliente  bool             `json:"regola_cliente"`
	Esito          EsitoFabbisogno  `json:"esito"`
	Motivo         MotivoFabbisogno `json:"motivo,omitempty"`
	Disegni        *GruppoDisegni2D `json:"disegni,omitempty"`
	DocumentoID    *uuid.UUID       `json:"documento_id,omitempty"`
	StatoNas       string           `json:"stato_nas,omitempty"`
	DerogaID       *uuid.UUID       `json:"deroga_id,omitempty"`
	PropostaAperta *uuid.UUID       `json:"proposta_aperta,omitempty"`
	FileCandidato  *uuid.UUID       `json:"file_candidato,omitempty"`
	CalcolataDa    string           `json:"calcolata_da"`
	Invariante     bool             `json:"invariante"`
	NotaRegola     string           `json:"nota_regola,omitempty"`
	Categoria      string           `json:"categoria"`
}

// FabbisognoPrevisto: un fabbisogno bloccante di ciò che non è una riga certa del perimetro (contratto §2.3; era
// PDFPrevisto): un nodo della struttura candidata o della BOM di lavoro con il suo tipo proposto, o una riga confermata
// con una rimozione aperta. Nodo: il riferimento di testo (il RifNodo, o il RifComponente della riga: T-B5-93).
// CodiceProposto: il codice proposto del nodo (CatenaCodice.Proposto; "" se non determinato) o il codice della riga.
// Disegni: per il 2D, i candidati del nodo (o il gruppo della riga); nil senza. Motivo: MotivoPrevisto* (campo in più,
// dubbio T-B5-53).
type FabbisognoPrevisto struct {
	Nodo           string           `json:"nodo"`
	CodiceProposto string           `json:"codice_proposto"`
	TipoDocumento  string           `json:"tipo_documento"`
	Disegni        *GruppoDisegni2D `json:"disegni,omitempty"`
	Motivo         string           `json:"motivo"`
}

// FabbisognoInformativo: un fabbisogno non bloccante di una riga certa (contratto §2.3, §2.6): mai fra quelli che
// mancano. EsitoVista: l'esito della vista per (componente, tipo), se c'è; DerogaID: la sua deroga. NotaRegola: per il 2D
// reso non bloccante da una riga esplicita del cliente su un componente (K-01, lettura A).
type FabbisognoInformativo struct {
	ComponenteID  uuid.UUID  `json:"componente_id"`
	TipoDocumento string     `json:"tipo_documento"`
	EsitoVista    string     `json:"esito_vista,omitempty"`
	DerogaID      *uuid.UUID `json:"deroga_id,omitempty"`
	NotaRegola    string     `json:"nota_regola,omitempty"`
}

// CompletezzaDocumentale: l'asse 6 di un prodotto (contratto §1.6, §2.3, §2.5): lo stato con il suo motivo, le voci
// certe (Voci, in ordine: prima il componente del prodotto, poi per componente e tipo), i previsti, i non bloccanti, il
// perimetro (PerimetroChiuso, MotivoPerimetro).
type CompletezzaDocumentale struct {
	Stato           StatoDocumenti          `json:"stato"`
	Voci            []VoceFabbisogno        `json:"voci,omitempty"`
	Previsti        []FabbisognoPrevisto    `json:"previsti,omitempty"`
	NonBloccanti    []FabbisognoInformativo `json:"non_bloccanti,omitempty"`
	Motivo          string                  `json:"motivo,omitempty"`
	PerimetroChiuso bool                    `json:"perimetro_chiuso"`
	MotivoPerimetro string                  `json:"motivo_perimetro,omitempty"`
}

// ---- gli ingressi astratti della regola (tipi della fase 3, dubbio T-B5-50) ----

// RegolaFabbisogno: un fabbisogno per un tipo di componente, com'è nella vista o nelle regole effettive: il tipo di
// documento e se è bloccante.
type RegolaFabbisogno struct {
	TipoDocumento string `json:"tipo_documento"`
	Bloccante     bool   `json:"bloccante"`
}

// FabbisognoRisolto: un fabbisogno di un pezzo dopo gli invarianti, K-01 e l'esenzione della minuteria confermata
// (FabbisogniDelComponente).
type FabbisognoRisolto struct {
	TipoDocumento string `json:"tipo_documento"`
	Bloccante     bool   `json:"bloccante"`
	Invariante    bool   `json:"invariante"`
	NotaRegola    string `json:"nota_regola,omitempty"`
	RegolaCliente bool   `json:"regola_cliente"`
}

// FabbisognoDellaRiga: un fabbisogno di una riga confermata com'è nella vista (R62 d C: la vista è la base dei
// componenti confermati), o nelle regole effettive per un componente senza righe nella vista, con l'esito della vista o
// quello calcolato allo stesso modo in Go (CalcolataDa).
type FabbisognoDellaRiga struct {
	TipoDocumento  string     `json:"tipo_documento"`
	Bloccante      bool       `json:"bloccante"`
	EsitoVista     string     `json:"esito_vista,omitempty"`
	CalcolataDa    string     `json:"calcolata_da"`
	DocumentoID    *uuid.UUID `json:"documento_id,omitempty"`
	StatoNas       string     `json:"stato_nas,omitempty"`
	DerogaID       *uuid.UUID `json:"deroga_id,omitempty"`
	PropostaAperta *uuid.UUID `json:"proposta_aperta,omitempty"`
	FileCandidato  *uuid.UUID `json:"file_candidato,omitempty"`
}

// DisegnoDellaVoce: ciò che serve alla voce del 2D di un componente (VoceDelDisegno).
//   - Gruppo: i 2D del componente (fase 2: documenti, «assegna», candidati del motore A); nil senza.
//   - DerogaID: la deroga sul 2D del componente, che non lo sostituisce (R62 D.4).
//   - PropostaAperta, FileProposto: la proposta aperta della vista sul 2D (v_fascicolo.proposta_aperta) e il suo
//     allegato, solo se il file è in un formato configurato.
//   - CandidatiDaDeterminare: gli allegati da_determinare, in un formato configurato, con un candidato del motore A su un
//     nodo del componente (LD-17: la vista non li vede).
//   - Scartati: gli allegati con la proposta scartata da una persona: uno scarto non soddisfa la voce, nemmeno come
//     associazione da verificare (T-E1R-10).
type DisegnoDellaVoce struct {
	Gruppo                 *GruppoDisegni2D `json:"gruppo,omitempty"`
	DerogaID               *uuid.UUID       `json:"deroga_id,omitempty"`
	PropostaAperta         *uuid.UUID       `json:"proposta_aperta,omitempty"`
	FileProposto           *uuid.UUID       `json:"file_proposto,omitempty"`
	CandidatiDaDeterminare []uuid.UUID      `json:"candidati_da_determinare,omitempty"`
	Scartati               []uuid.UUID      `json:"scartati,omitempty"`
}

// RigaDelPerimetro: una riga confermata del perimetro di P (il componente del prodotto e i componenti attivi che le
// relazioni confermate raggiungono da lui, di qualunque origine: R62 e A, T-B5-91).
//   - Categorie: le categorie della grammatica sul codice (Classificazione.Proposta).
//   - RegolaCliente, Fabbisogni: i fabbisogni del suo tipo, con gli esiti.
//   - Disegno: ciò che serve alla voce del 2D.
//   - RimozioneAperta: il prodotto la raggiunge solo attraverso archi con una rimozione proposta aperta, quindi potrebbe
//     uscire dal perimetro: le sue voci vanno fra i previsti (dubbio T-B5-56, deciso con la raggiungibilità: R-22).
type RigaDelPerimetro struct {
	ComponenteID    uuid.UUID             `json:"componente_id"`
	Codice          string                `json:"codice"`
	TipoComponente  string                `json:"tipo_componente"`
	DelProdotto     bool                  `json:"del_prodotto"`
	Categorie       []string              `json:"categorie,omitempty"`
	RegolaCliente   bool                  `json:"regola_cliente"`
	Fabbisogni      []FabbisognoDellaRiga `json:"fabbisogni,omitempty"`
	Disegno         DisegnoDellaVoce      `json:"disegno"`
	RimozioneAperta bool                  `json:"rimozione_aperta"`
}

// NodoDaPrevedere: un nodo delle strutture del prodotto che non è una riga certa del perimetro, con il suo tipo
// proposto (o confermato, per un nodo deciso fuori dal perimetro) e i fabbisogni di quel tipo (FabbisogniDelTipo).
// ComponenteID: il componente deciso, se c'è (per la classificazione). Disegni: i 2D candidati del nodo. Motivo:
// MotivoPrevisto*.
type NodoDaPrevedere struct {
	Nodo           string             `json:"nodo"`
	ComponenteID   *uuid.UUID         `json:"componente_id,omitempty"`
	CodiceProposto string             `json:"codice_proposto"`
	TipoComponente string             `json:"tipo_componente,omitempty"`
	Categorie      []string           `json:"categorie,omitempty"`
	RegolaCliente  bool               `json:"regola_cliente"`
	Regole         []RegolaFabbisogno `json:"regole,omitempty"`
	Disegni        *GruppoDisegni2D   `json:"disegni,omitempty"`
	Motivo         string             `json:"motivo"`
}

// IngressoCompletezza: l'ingresso astratto della regola della completezza.
//   - Struttura: la struttura del prodotto della verifica della BOM (fase 1): il componente, la fonte confermata, la
//     BOM di lavoro, le righe e gli archi da decidere, le rimozioni aperte del perimetro (ChiusuraDelPerimetro).
//   - Righe, Nodi: le righe certe del perimetro (con quelle con una rimozione aperta) e i nodi da prevedere.
//   - Conferme: le conferme della categoria; in A1c nessun adattatore (LD-19).
type IngressoCompletezza struct {
	Struttura StrutturaDaVerificare `json:"struttura"`
	Righe     []RigaDelPerimetro    `json:"righe,omitempty"`
	Nodi      []NodoDaPrevedere     `json:"nodi,omitempty"`
	Conferme  []ConfermaCategoria   `json:"conferme,omitempty"`
}

// ---- le regole ----

// FabbisogniDelTipo: le regole dei fabbisogni di un tipo di componente dai fabbisogni effettivi del cliente del thread
// (ListFabbisognoEffettivo, che li risolve come v_fascicolo: le righe del cliente sostituiscono in blocco quelle di
// default per tipo, R82, T-B0-36), in ordine di tipo di documento; e se sono del cliente (RigaFabbisogno.Proprio:
// decisioni sull'analista, punto 8). È la risoluzione che vale per i nodi proposti e per i componenti senza righe nella
// vista; sui componenti confermati dà le stesse righe della vista (la L4 di equivalenza lo controlla).
func FabbisogniDelTipo(tipo string, fabbisogni []fotorfq.RigaFabbisogno) ([]RegolaFabbisogno, bool) {
	var out []RegolaFabbisogno
	cliente := false
	for _, f := range fabbisogni {
		if f.TipoComponente != tipo {
			continue
		}
		out = append(out, RegolaFabbisogno{TipoDocumento: f.TipoDocumento, Bloccante: f.Bloccante})
		cliente = cliente || f.Proprio
	}
	return regoleInOrdine(out), cliente
}

// FabbisogniDelComponente: i fabbisogni di un pezzo dopo gli invarianti in Go (R99 A, R103 C; T-E1-17, T-E1R-01;
// E1R §4.2). È pura. finito vuol dire che il pezzo è il prodotto: il finito, o il componente del prodotto target
// qualunque sia il suo tipo (R99 A: il 2D del prodotto; R-23 della revisione della fase 3).
//  1. Le regole del suo tipo (dalla vista o dalle regole effettive), una per tipo di documento: gli altri tipi come sono
//     (S2: un bloccante del cliente si aggiunge, oggi il cad_3d del finito).
//  2. Il 2D è sempre richiesto e bloccante (Invariante), anche per il commerciale: se le regole non lo hanno, si aggiunge
//     con la nota schema_senza_2d (le regole sono quelle di default) o regola_cliente_senza_2d (sono del cliente). Una
//     riga esplicita del cliente che lo rende non bloccante segue K-01 (rigaNonBloccanteDel2DK01): sul finito resta
//     bloccante, su un componente va fra i non bloccanti, con la nota regola_cliente_2d_non_bloccante in tutti e due i
//     casi. Una riga di default non bloccante sul 2D, che lo schema oggi non ha, non toglie l'invariante (dubbio T-B5-54).
//  3. La minuteria confermata, mai il prodotto, è esente dal solo 2D (EsenteDal2D): gli altri fabbisogni restano.
//
// Lo STEP strutturale del finito non è una voce: è l'asse della fonte (§1.6, «Il finito chiede»). L'uscita è in ordine
// di tipo di documento.
func FabbisogniDelComponente(finito, regolaCliente bool, regole []RegolaFabbisogno, c Classificazione) []FabbisognoRisolto {
	var out []FabbisognoRisolto
	visti := map[string]bool{}
	con2D := false
	for _, r := range regoleInOrdine(regole) {
		if visti[r.TipoDocumento] {
			continue
		}
		visti[r.TipoDocumento] = true
		f := FabbisognoRisolto{TipoDocumento: r.TipoDocumento, Bloccante: r.Bloccante, RegolaCliente: regolaCliente}
		if r.TipoDocumento == tipoDocumentoDisegno2D {
			con2D = true
			f.Bloccante, f.Invariante = true, true
			if !r.Bloccante && regolaCliente {
				f.Bloccante, f.NotaRegola = rigaNonBloccanteDel2DK01(finito), NotaRegolaCliente2DNonBloccante
			}
		}
		out = append(out, f)
	}
	if !con2D {
		nota := NotaSchemaSenza2D
		if regolaCliente {
			nota = NotaRegolaClienteSenza2D
		}
		out = append(out, FabbisognoRisolto{TipoDocumento: tipoDocumentoDisegno2D, Bloccante: true, Invariante: true, NotaRegola: nota,
			RegolaCliente: regolaCliente})
	}
	if !finito && EsenteDal2D(c) {
		tenuti := out[:0:0]
		for _, f := range out {
			if f.TipoDocumento != tipoDocumentoDisegno2D {
				tenuti = append(tenuti, f)
			}
		}
		out = tenuti
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TipoDocumento < out[j].TipoDocumento })
	return out
}

// rigaNonBloccanteDel2DK01: il 2D di un pezzo quando una riga esplicita del cliente lo rende non bloccante. È la lettura
// A di K-01, confermata dall'utente il 06/10 [U] (E1R §4.4; T-E1R-02): sui componenti (sottoassieme, sciolto,
// commerciale) la riga del cliente si rispetta, e la voce sta fra i fabbisogni non bloccanti (FabbisognoInformativo) con
// la nota regola_cliente_2d_non_bloccante; sul finito no, perché R99 A lo vuole sempre: resta bloccante, con la stessa
// nota. «Unica esenzione» vale per la classificazione (la minuteria confermata); la riga del cliente è una scelta del
// cliente, che il testo di R103 non nomina. Restituisce se il 2D resta bloccante. La lettura sta tutta in questa
// funzione: nient'altro dipende da K-01.
func rigaNonBloccanteDel2DK01(finito bool) bool {
	return finito
}

// VoceDelDisegno: l'esito della voce del 2D di un componente (contratto §1.6, righe «Voce di un fabbisogno», «La
// deroga», «Da verificare»; R62 b A, R62 D.4, R83, R84; LD-17; decisioni sull'analista, punto 4). È pura: guarda tutto il
// gruppo, non solo il primario (T-B5-30). Il primo che vale:
//  1. presente: almeno un 2D corrente, confermato sul componente e valido (un «assegna» senza conferma non chiude il
//     fabbisogno: R62 b A; il cartiglio non conta: un TIFF decodificato senza cartiglio è presente, PO-08);
//  2. da_verificare per il contenuto: un 2D corrente e confermato con la validità da_verificare (il motivo della
//     validità: contenuto_non_analizzato, contenuto_non_aperto, contenuto_non_letto, contenuto_non_verificabile);
//  3. da_verificare per l'associazione (associazione_non_confermata, con FileCandidato): nel gruppo un «assegna» o un
//     candidato del motore A in un formato configurato, poi la proposta aperta della vista, poi un candidato
//     da_determinare del motore A (LD-17). Un file scartato da una persona non conta (T-E1R-10);
//  4. manca: con una deroga sul 2D derogato_non_sostituisce_2d (R62 D.4: la deroga non sostituisce il 2D); con un
//     documento corrente e confermato in un formato non configurato formato_non_configurato (T-B0-30, LD-02); altrimenti
//     nessun_documento.
//
// Una voce da_verificare non è una mancanza certa (R72 D).
func VoceDelDisegno(d DisegnoDellaVoce) (EsitoFabbisogno, MotivoFabbisogno, *uuid.UUID) {
	var tutti []Disegno2D
	if g := d.Gruppo; g != nil {
		if g.Primario != nil {
			tutti = append(tutti, *g.Primario)
		}
		tutti = append(tutti, g.Alternativi...)
	}
	scartato := map[uuid.UUID]bool{}
	for _, a := range d.Scartati {
		scartato[a] = true
	}
	confermatoCorrente := func(x Disegno2D) bool { return x.Provenienza == ancoraggio.OrigineConfermato && x.Corrente }
	for _, x := range tutti {
		if confermatoCorrente(x) && x.Validita == ValiditaValido {
			return EsitoPresente, "", nil
		}
	}
	for _, x := range tutti {
		if confermatoCorrente(x) && x.Validita == ValiditaDaVerificare {
			return EsitoDaVerificare, x.MotivoValidita, nil
		}
	}
	for _, x := range tutti {
		if x.Provenienza != ancoraggio.OrigineConfermato && x.Formato != "" && x.AllegatoID != nil && !scartato[*x.AllegatoID] {
			return EsitoDaVerificare, MotivoFabbisognoAssociazioneNonConfermata, copiaUUID(x.AllegatoID)
		}
	}
	if d.FileProposto != nil && !scartato[*d.FileProposto] {
		return EsitoDaVerificare, MotivoFabbisognoAssociazioneNonConfermata, copiaUUID(d.FileProposto)
	}
	candidati := append([]uuid.UUID(nil), d.CandidatiDaDeterminare...)
	sort.Slice(candidati, func(i, j int) bool { return candidati[i].String() < candidati[j].String() })
	for _, a := range candidati {
		if !scartato[a] {
			return EsitoDaVerificare, MotivoFabbisognoAssociazioneNonConfermata, &a
		}
	}
	if d.DerogaID != nil {
		return EsitoManca, MotivoFabbisognoDerogatoNonSostituisce2D, nil
	}
	for _, x := range tutti {
		if confermatoCorrente(x) && x.Validita == ValiditaFormatoNonConfigurato {
			return EsitoManca, MotivoFabbisognoFormatoNonConfigurato, nil
		}
	}
	return EsitoManca, MotivoFabbisognoNessunDocumento, nil
}

// EsitoDallaVista: l'esito di una voce di un tipo diverso dal 2D dall'esito di v_fascicolo (contratto §1.6, «per gli altri
// tipi bloccanti: come nella vista»; S3; R62 b A; decisioni sull'analista, punto 4):
//   - ok, ok_in_coda, ok_errore_nas (lo stato del NAS non toglie la presenza), derogato (la deroga vale come nella
//     vista: S3): presente;
//   - da_confermare: da_verificare, associazione_non_confermata;
//   - sul_portale: da_verificare, sul_portale (annunciato, non acquisito: non è una mancanza certa);
//   - manca: manca, nessun_documento;
//   - un altro valore, che la vista non ha: da_verificare senza motivo (non si dice né presente né mancante; dubbio
//     T-B5-55).
func EsitoDallaVista(esito string) (EsitoFabbisogno, MotivoFabbisogno) {
	switch esito {
	case esitoFascicoloOK, esitoFascicoloOKInCoda, esitoFascicoloOKErroreNas, esitoFascicoloDerogato:
		return EsitoPresente, ""
	case esitoFascicoloDaConfermare:
		return EsitoDaVerificare, MotivoFabbisognoAssociazioneNonConfermata
	case esitoFascicoloSulPortale:
		return EsitoDaVerificare, MotivoFabbisognoSulPortale
	case esitoFascicoloManca:
		return EsitoManca, MotivoFabbisognoNessunDocumento
	}
	return EsitoDaVerificare, ""
}

// ChiusuraDelPerimetro: PerimetroChiuso del contratto (§1.6; R102 A, T-E1-10), con il motivo quando è aperto
// (MotivoPerimetro*), dalla struttura della verifica della BOM (fase 1). È pura.
//
//	PerimetroChiuso = FonteSTEPConfermata (necessaria, non sufficiente)
//	                  AND la BOM di lavoro c'è (T-B5-92: una BOM di lavoro assente non è una BOM di lavoro con la sola radice)
//	                  AND ogni nodo e ogni arco della BOM di lavoro ha una decisione di una persona (riga confermata,
//	                      scartata o duplicato), esclusa la riga della radice (R80; T-B4-24; i nodi senza riga: T-B5-15)
//	                  AND nessuna rimozione proposta aperta sul perimetro di P
//
// La fonte si legge solo dal predicato dell'asse 2 (StrutturaDaVerificare.FonteConfermata) e la BOM di lavoro dallo
// stato della struttura (StrutturaDaVerificare.BOMDiLavoro): il legame allo STEP resta lì (T-B5-90). Una BOM di lavoro
// di un solo nodo, senza righe né archi da decidere, chiude il perimetro: la vacuità è quella del contratto.
func ChiusuraDelPerimetro(s StrutturaDaVerificare) (bool, string) {
	switch {
	case !s.ConComponente:
		return false, MotivoPerimetroTargetSenzaComponente
	case !s.FonteConfermata:
		return false, MotivoPerimetroFonteNonConfermata
	case !s.BOMDiLavoro:
		return false, MotivoPerimetroBOMDiLavoroAssente
	case len(s.RigheDaDecidere) > 0:
		return false, MotivoPerimetroNodiDaDecidere
	case len(s.ArchiDaDecidere) > 0:
		return false, MotivoPerimetroArchiDaDecidere
	case len(s.RimozioniAperte) > 0:
		return false, MotivoPerimetroRimozioniAperte
	}
	return true, ""
}

// StatoDellaCompletezza: lo stato dell'asse 6 dalle voci certe e dal perimetro (contratto §1.6, il blocco della
// completezza; R72 D [R], R102 A; T-B0-07). È pura.
//
//	non_calcolabile  per un target senza componente (T-B0-07)
//	incompleta       se una voce certa è «manca», oppure PerimetroChiuso e una voce certa non è «presente»
//	completa         se PerimetroChiuso e ogni voce certa è «presente»
//	non_calcolabile  altrimenti (perimetro aperto, nessuna mancanza certa)
//
// Una voce da_verificare non è una mancanza certa. «completa» non dice niente della nomenclatura.
func StatoDellaCompletezza(conComponente bool, voci []VoceFabbisogno, perimetroChiuso bool) (StatoDocumenti, string) {
	if !conComponente {
		return DocumentiNonCalcolabile, MotivoDocumentiTargetSenzaComponente
	}
	tutteAPosto := true
	for _, v := range voci {
		if v.Esito == EsitoManca {
			return DocumentiIncompleta, MotivoDocumentiVoceMancante
		}
		tutteAPosto = tutteAPosto && v.Esito == EsitoPresente
	}
	switch {
	case perimetroChiuso && !tutteAPosto:
		return DocumentiIncompleta, MotivoDocumentiVoceNonPresente
	case perimetroChiuso:
		return DocumentiCompleta, ""
	}
	return DocumentiNonCalcolabile, MotivoDocumentiPerimetroAperto
}

// Completezza: la regola della completezza documentale di un prodotto (contratto §1.6; R72 D [R], T-E1-10, R102 A). È
// pura e deterministica: l'ordine degli ingressi non conta, e gli ingressi di chi chiama non cambiano.
//  1. Il perimetro (ChiusuraDelPerimetro).
//  2. Le righe certe del perimetro: la classificazione (Classifica, con le conferme), i fabbisogni dopo gli invarianti
//     (FabbisogniDelComponente); per ogni fabbisogno bloccante una voce (il 2D con VoceDelDisegno, CalcolataDa = go;
//     gli altri tipi con EsitoDallaVista, con CalcolataDa della loro riga), per ogni non bloccante un FabbisognoInformativo.
//     Le voci sono VociCerte: le voci bloccanti delle righe confermate del perimetro, con gli invarianti, tolto il solo 2D
//     della minuteria confermata.
//  3. Una riga con una rimozione aperta non dà voci certe: i suoi fabbisogni bloccanti vanno fra i previsti
//     (rimozione_aperta), con il gruppo dei 2D.
//  4. I nodi da prevedere: i fabbisogni bloccanti del loro tipo, con gli invarianti (una minuteria solo proposta non
//     toglie niente), fra i previsti, con i 2D candidati.
//  5. Lo stato (StatoDellaCompletezza).
//
// Le uscite sono in ordine: le voci con prima il componente del prodotto, poi per componente e tipo di documento; i
// previsti per nodo e tipo; i non bloccanti per componente e tipo.
func Completezza(in IngressoCompletezza) CompletezzaDocumentale {
	var out CompletezzaDocumentale
	out.PerimetroChiuso, out.MotivoPerimetro = ChiusuraDelPerimetro(in.Struttura)
	// Un previsto per (nodo, tipo di documento); fra due, che l'adattatore non dà, quello con il motivo e il codice minori,
	// così l'ordine degli ingressi non conta.
	previsti := map[string]FabbisognoPrevisto{}
	prevedi := func(p FabbisognoPrevisto) {
		k := p.Nodo + "\x00" + p.TipoDocumento
		if q, ok := previsti[k]; !ok || p.Motivo+"\x00"+p.CodiceProposto < q.Motivo+"\x00"+q.CodiceProposto {
			previsti[k] = p
		}
	}
	for _, r := range in.Righe {
		id := r.ComponenteID
		// Il prodotto è il finito o il componente del target, qualunque sia il suo tipo: mai l'esenzione, e la riga
		// esplicita non bloccante del cliente non toglie il suo 2D (R99 A; R-23 della revisione della fase 3).
		prodotto := r.TipoComponente == tipoFinito || r.DelProdotto
		cl := Classifica(ComponenteDaClassificare{ComponenteID: &id, Tipo: r.TipoComponente, DelProdotto: r.DelProdotto, Categorie: r.Categorie}, in.Conferme)
		var regole []RegolaFabbisogno
		for _, f := range r.Fabbisogni {
			regole = append(regole, RegolaFabbisogno{TipoDocumento: f.TipoDocumento, Bloccante: f.Bloccante})
		}
		for _, f := range FabbisogniDelComponente(prodotto, r.RegolaCliente, regole, cl) {
			riga := fabbisognoDi(r.Fabbisogni, f.TipoDocumento)
			if r.RimozioneAperta {
				if f.Bloccante {
					p := FabbisognoPrevisto{Nodo: ancoraggio.RifComponente(id), CodiceProposto: r.Codice, TipoDocumento: f.TipoDocumento,
						Motivo: MotivoPrevistoRimozioneAperta}
					if f.TipoDocumento == tipoDocumentoDisegno2D {
						p.Disegni = copiaGruppo(r.Disegno.Gruppo)
					}
					prevedi(p)
				}
				continue
			}
			if !f.Bloccante {
				nb := FabbisognoInformativo{ComponenteID: id, TipoDocumento: f.TipoDocumento, NotaRegola: f.NotaRegola}
				if riga != nil {
					nb.EsitoVista, nb.DerogaID = riga.EsitoVista, copiaUUID(riga.DerogaID)
				}
				out.NonBloccanti = append(out.NonBloccanti, nb)
				continue
			}
			v := VoceFabbisogno{ComponenteID: id, Codice: r.Codice, TipoComponente: r.TipoComponente, DelProdotto: r.DelProdotto,
				TipoDocumento: f.TipoDocumento, Bloccante: true, RegolaCliente: f.RegolaCliente, Invariante: f.Invariante, NotaRegola: f.NotaRegola,
				Categoria: cl.Categoria}
			switch {
			case f.TipoDocumento == tipoDocumentoDisegno2D:
				v.Esito, v.Motivo, v.FileCandidato = VoceDelDisegno(r.Disegno)
				v.Disegni, v.DerogaID, v.PropostaAperta = copiaGruppo(r.Disegno.Gruppo), copiaUUID(r.Disegno.DerogaID), copiaUUID(r.Disegno.PropostaAperta)
				v.CalcolataDa = CalcolataDaGo
			case riga != nil:
				v.Esito, v.Motivo = EsitoDallaVista(riga.EsitoVista)
				v.DocumentoID, v.StatoNas, v.DerogaID = copiaUUID(riga.DocumentoID), riga.StatoNas, copiaUUID(riga.DerogaID)
				v.PropostaAperta, v.CalcolataDa = copiaUUID(riga.PropostaAperta), riga.CalcolataDa
				if v.Esito == EsitoDaVerificare && v.Motivo == MotivoFabbisognoAssociazioneNonConfermata {
					v.FileCandidato = copiaUUID(riga.FileCandidato)
				}
			default:
				// un fabbisogno senza la sua riga non nasce (le regole vengono dalle righe): per prudenza, nessun documento.
				v.Esito, v.Motivo, v.CalcolataDa = EsitoManca, MotivoFabbisognoNessunDocumento, CalcolataDaGo
			}
			out.Voci = append(out.Voci, v)
		}
	}
	for _, n := range in.Nodi {
		cl := Classifica(ComponenteDaClassificare{ComponenteID: copiaUUID(n.ComponenteID), Tipo: n.TipoComponente, Categorie: n.Categorie}, in.Conferme)
		for _, f := range FabbisogniDelComponente(n.TipoComponente == tipoFinito, n.RegolaCliente, n.Regole, cl) {
			if !f.Bloccante {
				continue
			}
			p := FabbisognoPrevisto{Nodo: n.Nodo, CodiceProposto: n.CodiceProposto, TipoDocumento: f.TipoDocumento, Motivo: n.Motivo}
			if f.TipoDocumento == tipoDocumentoDisegno2D {
				p.Disegni = copiaGruppo(n.Disegni)
			}
			prevedi(p)
		}
	}
	sort.SliceStable(out.Voci, func(i, j int) bool {
		a, b := out.Voci[i], out.Voci[j]
		if a.DelProdotto != b.DelProdotto {
			return a.DelProdotto
		}
		if a.ComponenteID != b.ComponenteID {
			return a.ComponenteID.String() < b.ComponenteID.String()
		}
		return a.TipoDocumento < b.TipoDocumento
	})
	sort.SliceStable(out.NonBloccanti, func(i, j int) bool {
		a, b := out.NonBloccanti[i], out.NonBloccanti[j]
		if a.ComponenteID != b.ComponenteID {
			return a.ComponenteID.String() < b.ComponenteID.String()
		}
		return a.TipoDocumento < b.TipoDocumento
	})
	chiavi := make([]string, 0, len(previsti))
	for k := range previsti {
		chiavi = append(chiavi, k)
	}
	sort.Strings(chiavi)
	for _, k := range chiavi {
		out.Previsti = append(out.Previsti, previsti[k])
	}
	out.Stato, out.Motivo = StatoDellaCompletezza(in.Struttura.ConComponente, out.Voci, out.PerimetroChiuso)
	return out
}

// regoleInOrdine: le regole in ordine di tipo di documento, prima le bloccanti (fra due righe dello stesso tipo, che il
// DB non ammette, vale la più bloccante).
func regoleInOrdine(regole []RegolaFabbisogno) []RegolaFabbisogno {
	out := append([]RegolaFabbisogno(nil), regole...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TipoDocumento != out[j].TipoDocumento {
			return out[i].TipoDocumento < out[j].TipoDocumento
		}
		return out[i].Bloccante && !out[j].Bloccante
	})
	return out
}

// fabbisognoDi: la riga del fabbisogno di quel tipo di documento; nil senza.
func fabbisognoDi(fabbisogni []FabbisognoDellaRiga, tipo string) *FabbisognoDellaRiga {
	for i := range fabbisogni {
		if fabbisogni[i].TipoDocumento == tipo {
			return &fabbisogni[i]
		}
	}
	return nil
}

// copiaGruppo: il gruppo dei 2D con le voci copiate, così l'uscita non condivide memoria con l'ingresso; nil se vuoto.
func copiaGruppo(g *GruppoDisegni2D) *GruppoDisegni2D {
	if g == nil || g.Primario == nil {
		return nil
	}
	c := GruppoDisegni2D{RegolaPrimario: g.RegolaPrimario}
	p := copiaDisegno(*g.Primario)
	c.Primario = &p
	for _, x := range g.Alternativi {
		c.Alternativi = append(c.Alternativi, copiaDisegno(x))
	}
	for _, r := range g.RelazioniDaConfermare {
		r.Letture = append([]string(nil), r.Letture...)
		if len(r.Letture) == 0 {
			r.Letture = nil
		}
		c.RelazioniDaConfermare = append(c.RelazioniDaConfermare, r)
	}
	return &c
}
