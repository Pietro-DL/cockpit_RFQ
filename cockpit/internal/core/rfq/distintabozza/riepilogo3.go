package distintabozza

// R4 nel formato 3: il riepilogo della conferma, senza scritture (EM §9.4, §7.4, §7.7, §8.1-§8.3, App. H.7, App. I; C3 §4;
// direttiva 39 §1, §2, §4, §5, §6 [U + T-39]). E' la funzione condivisa che R5 ricalcolera' sotto il lucchetto nella sua
// connessione S (strategia A, EM §9.5): stessi ingressi, stessa firma (anteprima-3). Chi chiama apre la transazione (R4:
// REPEATABLE READ READ ONLY, chiusa con ROLLBACK) e Riepilogo3 fa solo SELECT.
//
// L'invariante della direttiva 39 §1 [U]: una bozza senza STEP e' interamente riepilogabile. R4 calcola tutte le verifiche
// che non dipendono dallo STEP, le differenze, le provenienze, i blocchi e gli avvisi, e aggiunge il blocco firmato
// STEP_SORGENTE_NON_CONFERMATA quando la sorgente della versione non e' confermata validamente da S8 (EM App. I.2,
// confermabom.SorgenteValida): non e' un errore della lettura e non impedisce la consultazione; R5 lo trasformera' in 422
// BLOCCHI. MONO_PEZZO_SENZA_STEP nel formato 3 non si somma al gate (R39-20): il gate porta mono_pezzo: true; con una
// sorgente valida non scatta. Il caso a zero figli (STEP S8 valido con zero figli) non fa nascere ne' mono_pezzo ne' una verifica
// positiva. Una fonte A della working diversa dalla sorgente B della bozza non e' da sola BASE_SUPERATA (D39 §7): la base
// resta quella di BaseDellaBozza.
//
// Il corpo (EM §9.4): i campi dello snapshot formato 3 piu' bozza_id?, bozza_versione?, chiave_idempotenza?. Un corpo
// formato 2 e' 422 CORPO_NON_VALIDO motivo formato_da_convertire. La provenienza delle PB del corpo non e' un'autorita':
// R4 la ricalcola con la regola di R2 (provenienza3.go: decisa e PropostaNelRegistro) contro la versione salvata indicata
// (bozza_id e bozza_versione, la corrente se manca la versione); senza una bozza salvata ogni PB vale manuale, salvo un
// suggerimento con il registro del prodotto che la propone (motore) [T-39].
//
// I gruppi D-PB-04 A [U]: «motore intatta», «corretta», «manuale», «legacy da verificare», «proposte rimosse» e gli
// spostamenti di AR esistenti (sostituzioni e svincoli), piu' le AR che restano senza cambiare (escluso «Mantieni»,
// ricodifiche D-ASS-01). La riga «legato come sorgente» dello STEP della bozza sta in sorgente_legata, in sola lettura,
// fuori dalle PB (D39 §5 [U]).
//
// Il completamento (blocco «R4 completo», 20/10): il canonico del cliente (riepilogo3_canonico.go), gli avvisi del registro,
// gli effetti e le origini degli esclusi, le condizioni (c) ed (e) di prima-istanza-1 (riepilogo3_completamento.go), gli
// avvisi e i conflitti del Motore A con la condizione (f) (riepilogo3_motore.go). Restano fuori: RIF_AMBIGUO (non si
// calcola in V1, EM §8.1) e STEP_SECONDARIO_DISPONIBILE (le sorgenti secondarie sono V5).

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// VersioneAnteprima3: il nome della firma del riepilogo formato 3 (EM §8.3).
const VersioneAnteprima3 = "anteprima-3"

// I codici dei blocchi del formato 3 (EM §8.1: 31 in V1).
const (
	BloccoBaseSuperata                = "BASE_SUPERATA"
	BloccoAssenzaNonDecisa            = "ASSENZA_NON_DECISA"
	BloccoD1UltimoFiglio              = "D1_ULTIMO_FIGLIO"
	BloccoD1ParticolareConFigli       = "D1_PARTICOLARE_CON_FIGLI"
	BloccoE37NonContenitore           = "E37_NON_CONTENITORE"
	BloccoCiclo                       = "CICLO"
	BloccoArcoDuplicato               = "ARCO_DUPLICATO_DOPO_RISOLUZIONE"
	BloccoFiglioSenzaPadre            = "FIGLIO_SENZA_PADRE"
	BloccoProdottoProtetto            = "PRODOTTO_PROTETTO"
	BloccoCreaSuEsistente             = "CREA_SU_ESISTENTE"
	BloccoCanonicoCollide             = "CANONICO_COLLIDE"
	BloccoCanonicoAmbiguo             = "CANONICO_AMBIGUO"
	BloccoDueCreaStessoCodice         = "DUE_CREA_STESSO_CODICE"
	BloccoRiusoIncompatibile          = "RIUSO_INCOMPATIBILE"
	BloccoRicodificaCollide           = "RICODIFICA_COLLIDE"
	BloccoRicodificaFileNAS           = "RICODIFICA_FILE_NAS"
	BloccoArcoCondivisoQta            = "ARCO_CONDIVISO_QTA"
	BloccoRifAmbiguo                  = "RIF_AMBIGUO"
	BloccoPreassociazioneNonApplicab  = "PREASSOCIAZIONE_NON_APPLICABILE"
	BloccoCongelata                   = "CONGELATA"
	BloccoTipoEffettoCambiato         = "TIPO_EFFETTO_CAMBIATO"
	BloccoCodiceDaAssegnare           = "CODICE_DA_ASSEGNARE"
	BloccoMonoPezzoSenzaStep          = "MONO_PEZZO_SENZA_STEP"
	BloccoMonoPezzoConFigli           = "MONO_PEZZO_CON_FIGLI"
	BloccoCollisioneEmittenti         = "COLLISIONE_EMITTENTI"
	BloccoNonInclusoHaComponente      = "NON_INCLUSO_HA_COMPONENTE"
	BloccoNonInclusoConFigli          = "NON_INCLUSO_CON_FIGLI"
	BloccoNonInclusoNonApplicabile    = "NON_INCLUSO_NON_APPLICABILE"
	BloccoIdentitaIncompatibile       = "IDENTITA_INCOMPATIBILE"
	BloccoOrigineTroppoGrande         = "ORIGINE_TROPPO_GRANDE"
	BloccoStepSorgenteNonConfermata   = "STEP_SORGENTE_NON_CONFERMATA"
	AvvisoDaValutareAperto            = "DA_VALUTARE_APERTO"
	AvvisoProdottiToccati             = "PRODOTTI_TOCCATI"
	AvvisoEsclusoConDocumenti         = "ESCLUSO_CON_DOCUMENTI"
	AvvisoProdottoSenzaFigli          = "PRODOTTO_SENZA_FIGLI"
	AvvisoVerificaDopoCommit          = "VERIFICA_DOPO_COMMIT"
	AvvisoEsclusiSenzaMotivo          = "ESCLUSI_SENZA_MOTIVO"
	AvvisoFileContesoFraProdotti      = "FILE_CONTESO_FRA_PRODOTTI"
	AvvisoSegnalatiInclusi            = "SEGNALATI_INCLUSI"
	AvvisoSorgenteNonAutorizzata      = "SORGENTE_NON_AUTORIZZATA"
	AvvisoEmittenteCertificatoSuperat = "EMITTENTE_CERTIFICATO_SUPERATO"
)

// I motivi del blocco STEP_SORGENTE_NON_CONFERMATA (EM App. I.3, forma [T-39 corsia contratto], adottata).
const (
	SorgenteMotivoSenzaSorgente = "senza_sorgente"
	SorgenteMotivoNonConfermata = confermabom.SorgenteNessunaConfermaS8
	SorgenteMotivoRevocata      = confermabom.SorgenteRevocata
	SorgenteMotivoRifiutata     = confermabom.SorgenteRifiutata
	SorgenteMotivoSuperata      = confermabom.SorgenteSuperata
	SorgenteMotivoNonCoerente   = confermabom.SorgenteDocumentoAssente
)

// I gruppi D-PB-04 A [U] di una PB (documenti_preassociati[].gruppo).
const (
	GruppoMotoreIntatta      = "motore_intatta"
	GruppoCorretta           = "corretta"
	GruppoManuale            = "manuale"
	GruppoLegacyDaVerificare = "legacy_da_verificare"
	// GruppoLegacyVerificata: una PB legacy riconosciuta con file_riconosciuto (R39-15 [T-39]): la provenienza resta legacy
	GruppoLegacyVerificata = "legacy_verificata"
)

// I codici dell'errore del motore negli avvisi (EM §8.2).
const (
	MotoreErroreCalcolo = "ERRORE_CALCOLO"
)

// MaxOrigine: il tetto di origine in conferma_strutturale (64 KiB): l'unione delle evidenze degli archi oltre questo peso e'
// ORIGINE_TROPPO_GRANDE (U2 §7.7).
const MaxOrigine = 64 << 10

// RichiestaR4 e' il corpo di R4 letto.
type RichiestaR4 struct {
	Snapshot          Snapshot3
	BozzaID           *uuid.UUID
	BozzaVersione     int // 0 = non indicata
	ChiaveIdempotenza *uuid.UUID
}

// OpzioniR4 sono i parametri di R4 che non stanno nel corpo.
type OpzioniR4 struct {
	// UtenteID: chi chiede il riepilogo, cioe' chi confermerebbe (l'autore delle voci del registro senza un evento, C3 §4.3)
	UtenteID uuid.UUID
	// AvvisiMotore: ?avvisi_motore=1
	AvvisiMotore bool
	// ConIngressi: R4 aggiunge ingressi (le parti di R1 ?ingressi=1) con l'impronta delle regole; la S di R5 no
	ConIngressi    bool
	ImprontaRegole string
	// Motore: il calcolo del motore fatto dall'adattatore (solo con AvvisiMotore; riepilogo3_motore.go)
	Motore *EsitoMotoreR4
}

// LeggiRichiestaR4 legge il corpo di R4 (commento di testa): lettura stretta, poi lo snapshot formato 3.
func LeggiRichiestaR4(raw []byte) (RichiestaR4, error) {
	var out RichiestaR4
	var m map[string]json.RawMessage
	if e := decodificaStretta("richiesta", raw, &m); e != nil {
		return out, e
	}
	if m == nil {
		return out, corpo("richiesta", "il corpo è un oggetto JSON con la bozza formato 3")
	}
	if f, ok := m["formato"]; ok {
		var n int
		if json.Unmarshal(f, &n) == nil && n == FormatoSnapshot {
			return out, corpoMotivo("formato", MotivoFormatoDaConvertire,
				"la bozza è formato 2: si converte con converti_da (R2) prima del riepilogo")
		}
	}
	if b, ok := m["bozza_id"]; ok {
		delete(m, "bozza_id")
		if presenteValore(b) {
			var id uuid.UUID
			if json.Unmarshal(b, &id) != nil || id == uuid.Nil {
				return out, corpo("bozza_id", "bozza_id è un UUID")
			}
			out.BozzaID = &id
		}
	}
	if v, ok := m["bozza_versione"]; ok {
		delete(m, "bozza_versione")
		if presenteValore(v) {
			var n int
			if json.Unmarshal(v, &n) != nil || n < 1 {
				return out, corpo("bozza_versione", "bozza_versione è un intero da 1")
			}
			out.BozzaVersione = n
		}
	}
	if k, ok := m["chiave_idempotenza"]; ok {
		delete(m, "chiave_idempotenza")
		if presenteValore(k) {
			var id uuid.UUID
			if json.Unmarshal(k, &id) != nil || id == uuid.Nil {
				return out, corpo("chiave_idempotenza", "chiave_idempotenza è un UUID")
			}
			out.ChiaveIdempotenza = &id
		}
	}
	if out.BozzaVersione > 0 && out.BozzaID == nil {
		return out, corpo("bozza_versione", "bozza_versione vale solo con bozza_id")
	}
	resto, err := json.Marshal(m)
	if err != nil {
		return out, corpo("richiesta", "il corpo non si rilegge: %v", err)
	}
	s, err := LeggiSnapshot3(resto)
	if err != nil {
		return out, err
	}
	out.Snapshot = s
	return out, nil
}

// ------------------------------------------------------------------ la risposta

// RiepilogoR4 e' la risposta 200 di R4 (C3 §4.2 con le aggiunte di EM §9.4 e della direttiva 39). Tutto e' dentro la
// firma (firma_anteprima, anteprima-3), compresi gli avvisi e i conflitti del motore (U2 §6.3.5), salvo
// avvisi_motore_errore, file_in_analisi, ingressi e la firma stessa (EM §8.2, §8.3; U2 §6.3.7).
type RiepilogoR4 struct {
	Formato               int                              `json:"formato"`
	ProdottoID            uuid.UUID                        `json:"prodotto_id"`
	BozzaID               *uuid.UUID                       `json:"bozza_id,omitempty"`
	BozzaVersione         *int                             `json:"bozza_versione,omitempty"`
	VersioneCorrente      confermabom.VersioneCorrente     `json:"versione_corrente"`
	VersioneBomBase       *VersioneBomBase                 `json:"versione_bom_base,omitempty"`
	FirmaBase             string                           `json:"firma_base,omitempty"`
	Base                  StatoBase                        `json:"base"`
	RifScaduti            []RifScaduto                     `json:"rif_scaduti"`
	Importazione          *ImportazioneR4                  `json:"importazione,omitempty"`
	Sorgente              SorgenteR4                       `json:"sorgente"`
	SorgenteLegata        *SorgenteLegataR4                `json:"sorgente_legata,omitempty"`
	LegamiStorici         []LegameStoricoR4                `json:"legami_storici"`
	EffettoSullaFonte     EffettoFonteR4                   `json:"effetto_sulla_fonte"`
	Identita              []IdentitaR4                     `json:"identita"`
	Archi                 []ArcoR4                         `json:"archi"`
	Fuori                 []FuoriR4                        `json:"fuori"`
	DaRicollocare         []VoceVassoio                    `json:"da_ricollocare"`
	Esclusi               []EsclusoR4                      `json:"esclusi"`
	DocumentiPreassociati []PreassociazioneR4              `json:"documenti_preassociati"`
	Svincoli              []associazioni.EsitoSvincolo     `json:"svincoli"`
	EffettiDocumentali    EffettiDocumentaliR4             `json:"effetti_documentali"`
	Condivisione          []CondivisioneR4                 `json:"condivisione"`
	RegistroDecisioni     []VoceRegistroR4                 `json:"registro_decisioni"`
	CriterioPrevisto      CriterioR4                       `json:"criterio_previsto"`
	VerificaPrevista      confermabom.VerificaPrimaIstanza `json:"verifica_prevista"`
	UgualeAllaBase        bool                             `json:"uguale_alla_base"`
	DifferenzeDallaBase   []DifferenzaAtto                 `json:"differenze_dalla_base"`
	Blocchi               []Rilievo                        `json:"blocchi"`
	Avvisi                []Rilievo                        `json:"avvisi"`
	Confermabile          bool                             `json:"confermabile"`
	AvvisiMotore          []json.RawMessage                `json:"avvisi_motore"`
	AvvisiMotoreCalcolati bool                             `json:"avvisi_motore_calcolati"`
	// ConflittiMotore: i conflitti aperti del motore sul prodotto (condizione (f) di prima-istanza-1), firmati
	ConflittiMotore    []string        `json:"conflitti_motore"`
	AvvisiMotoreErrore *ErroreMotoreR4 `json:"avvisi_motore_errore,omitempty"`
	FileInAnalisi      []FileInAnalisi `json:"file_in_analisi"`
	// Ingressi: le parti di R1 ?ingressi=1 calcolate nella stessa transazione (EM §9.4), fuori dalla firma; solo nella rotta
	Ingressi       *Ingressi `json:"ingressi,omitempty"`
	FirmaAnteprima string    `json:"firma_anteprima"`
	// per R5 (la stessa funzione nella connessione S): gli esiti del Piano da applicare e la risoluzione delle chiavi
	esitiPiano  associazioni.EsitiPiano
	risoluzione map[string]string
}

// ImportazioneR4: il registro della versione (EM §9.4).
type ImportazioneR4 struct {
	Impronta          string `json:"impronta"`
	ImprontaContenuto string `json:"impronta_contenuto"`
}

// SorgenteR4: la sorgente della versione e il suo stato rispetto al gate di R5 (EM App. I.2).
type SorgenteR4 struct {
	AllegatoID    *uuid.UUID `json:"allegato_id,omitempty"`
	Sha256        string     `json:"sha256,omitempty"`
	Gruppo        string     `json:"gruppo,omitempty"`
	Valida        bool       `json:"valida"`
	Motivo        string     `json:"motivo,omitempty"`
	DocumentoID   *uuid.UUID `json:"documento_id,omitempty"`
	LegameStorico bool       `json:"legame_storico,omitempty"`
	// Correzione: la sorgente valida corregge una sorgente sbagliata (confermabom.StatoSorgente.Correzione; G39-03 [U],
	// forma [T-39])
	Correzione bool `json:"correzione_sorgente,omitempty"`
}

// SorgenteLegataR4: la riga «legato come sorgente» (D39 §5 [U]; MAT SG-20): lo STEP della bozza, in sola lettura, fuori
// dalle PB; R5 non ne chiede la AR. Lo stato e' quello del gate.
type SorgenteLegataR4 struct {
	AllegatoID  uuid.UUID  `json:"allegato_id"`
	Sha256      string     `json:"sha256"`
	NomeFile    string     `json:"nome_file,omitempty"`
	DocumentoID *uuid.UUID `json:"documento_id,omitempty"`
	Confermata  bool       `json:"confermata"`
	Motivo      string     `json:"motivo,omitempty"`
	Testo       string     `json:"testo"`
}

// LegameStoricoR4: uno STEP del prodotto confermato dalle pagine legacy senza un evento di S8 (D-AR04 (iii) [U]): si
// mostra, ma non soddisfa il gate (l'ingegnere lo riconferma da S8).
type LegameStoricoR4 struct {
	DocumentoID uuid.UUID `json:"documento_id"`
	Sha256      string    `json:"sha256"`
	NomeFile    string    `json:"nome_file,omitempty"`
	Testo       string    `json:"testo"`
}

// EffettoFonteR4: che cosa farebbe R5 sulla fonte della working (EM App. I.6): la promozione della sorgente valida in
// componente.step_strutturale_id e la sostituzione documentale del vecchio STEP.
type EffettoFonteR4 struct {
	Promozione     bool       `json:"promozione"`
	StepPrima      *uuid.UUID `json:"step_prima,omitempty"`
	StepDopo       *uuid.UUID `json:"step_dopo,omitempty"`
	ShaPrima       string     `json:"sha256_prima,omitempty"`
	ShaDopo        string     `json:"sha256_dopo,omitempty"`
	SostituzioneDi *uuid.UUID `json:"sostituzione_di,omitempty"`
	// CorrezioneSorgente: la conferma sara' una revisione interna, una correzione della sorgente (G39-03 [U]: «non sono
	// revisioni dei clienti»): la bozza parte da una conferma e la sua sorgente valida corregge una sorgente sbagliata
	// [T-39]. R5 la scrive in origine della conferma; R7 la rilegge.
	CorrezioneSorgente bool   `json:"correzione_sorgente,omitempty"`
	Testo              string `json:"testo"`
}

// Rilievo e' un blocco o un avviso (EM §8.1, §8.2): il codice e, secondo il codice, i campi che lo spiegano.
type Rilievo struct {
	Codice        string            `json:"codice"`
	Motivo        string            `json:"motivo,omitempty"`
	Motivi        []string          `json:"motivi,omitempty"`
	Variante      string            `json:"variante,omitempty"`
	ChiaveBozza   string            `json:"chiave_bozza,omitempty"`
	Chiavi        []string          `json:"chiavi,omitempty"`
	ComponenteID  *uuid.UUID        `json:"componente_id,omitempty"`
	AllegatoID    *uuid.UUID        `json:"allegato_id,omitempty"`
	Prodotti      []uuid.UUID       `json:"prodotti,omitempty"`
	Sorgente      *RiferimentoFonte `json:"sorgente,omitempty"`
	DocumentoID   *uuid.UUID        `json:"documento_id,omitempty"`
	LegameStorico bool              `json:"legame_storico,omitempty"`
	MonoPezzo     bool              `json:"mono_pezzo,omitempty"`
	ChiaveWorking string            `json:"chiave_working,omitempty"`
	Parti         []ParteIdentita   `json:"parti,omitempty"`
	Differenze    []string          `json:"differenze,omitempty"`
	// Valore: il codice o l'emittente che l'avviso del registro nomina (il P/N K4, il codice del cartiglio, l'emittente
	// della lettura del file)
	Valore string   `json:"valore,omitempty"`
	Testo  string   `json:"testo"`
	Rimedi []string `json:"rimedi,omitempty"`
}

// RiferimentoFonte: l'allegato e lo sha256 di una sorgente.
type RiferimentoFonte struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Sha256     string    `json:"sha256"`
}

// ParteIdentita: una parte di COLLISIONE_EMITTENTI o di IDENTITA_INCOMPATIBILE (EM §5.3).
type ParteIdentita struct {
	ChiaveBozza       string     `json:"chiave_bozza,omitempty"`
	ComponenteID      *uuid.UUID `json:"componente_id,omitempty"`
	Codice            string     `json:"codice"`
	Emittente         string     `json:"emittente"`
	EmittentePresunto bool       `json:"emittente_presunto,omitempty"`
	Namespace         string     `json:"namespace,omitempty"`
	Origine           string     `json:"origine"`
}

// PrimaDopo: un valore prima (nella working) e dopo (con la conferma).
type PrimaDopo struct {
	Prima *string `json:"prima"`
	Dopo  string  `json:"dopo"`
}

// IdentitaR4: una voce di identita[] (C3 §4.3 slide 1; EM §9.4).
type IdentitaR4 struct {
	ChiaveBozza          string          `json:"chiave_bozza"`
	ComponenteID         *uuid.UUID      `json:"componente_id"`
	Azione               string          `json:"azione"`
	Origine              string          `json:"origine"`
	Codice               PrimaDopo       `json:"codice"`
	Rev                  PrimaDopo       `json:"rev"`
	Descrizione          PrimaDopo       `json:"descrizione"`
	Tipo                 PrimaDopo       `json:"tipo"`
	Decisa               Identita        `json:"decisa"`
	EsitoIdentita        string          `json:"esito_identita"`
	Proposta             json.RawMessage `json:"proposta,omitempty"`
	Collisioni           []string        `json:"collisioni"`
	Documenti            DocumentiNodoR4 `json:"documenti"`
	NonAncoraConfermato  bool            `json:"non_ancora_confermato"`
	DifferenzeDallaWork  []string        `json:"differenze_da_ultima_conferma"`
	Padri                int             `json:"padri"`
	CodiceRisoltoGrafico string          `json:"-"`
}

// DocumentiNodoR4: le PB e le AR del nodo.
type DocumentiNodoR4 struct {
	Preassociati int `json:"preassociati"`
	ARAttive     int `json:"ar_attive"`
}

// EstremoArcoR4: un estremo di un arco.
type EstremoArcoR4 struct {
	ChiaveBozza  string     `json:"chiave_bozza,omitempty"`
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	Codice       string     `json:"codice"`
	Tipo         string     `json:"tipo"`
}

// ArcoR4: un arco voluto o tolto (C3 §4.3 slide 2).
type ArcoR4 struct {
	Padre         EstremoArcoR4  `json:"padre"`
	Figlio        EstremoArcoR4  `json:"figlio"`
	QtaPrecedente *int           `json:"qta_precedente"`
	QtaVoluta     *int           `json:"qta_voluta"`
	Azione        string         `json:"azione"`
	Origine       string         `json:"origine,omitempty"`
	PadrePrima    *EstremoArcoR4 `json:"padre_prima,omitempty"`
	Decisione     string         `json:"decisione,omitempty"`
}

// FuoriR4: un componente della working del prodotto che esce con la conferma (escluso) o che la bozza non porta.
type FuoriR4 struct {
	ComponenteID uuid.UUID `json:"componente_id"`
	Codice       string    `json:"codice"`
	Tipo         string    `json:"tipo"`
	Motivo       string    `json:"motivo"`
	Decisione    string    `json:"decisione,omitempty"`
}

// EsclusoR4: una voce di esclusi[] (EM §7.4 con U2 §6.1).
type EsclusoR4 struct {
	Origine        string                     `json:"origine"`
	ID             string                     `json:"id,omitempty"`
	Casella        *Casella3                  `json:"casella,omitempty"`
	ComponenteID   *uuid.UUID                 `json:"componente_id,omitempty"`
	Codice         string                     `json:"codice,omitempty"`
	MotivoCodice   string                     `json:"motivo_codice,omitempty"`
	Motivo         string                     `json:"motivo,omitempty"`
	MotivoMancante bool                       `json:"motivo_mancante,omitempty"`
	AzioneID       *uuid.UUID                 `json:"azione_id,omitempty"`
	SegnaleVisto   *importazione.SegnaleVisto `json:"segnale_visto,omitempty"`
	// SegnaleRegistro: il segnale della voce nel registro della versione (EM §7.4), firmato; nil = non segnalata
	SegnaleRegistro *importazione.SegnaleVisto `json:"segnale_registro,omitempty"`
	// AutoreID: chi ha escluso il nodo nelle pagine di prima (origine legacy); nil = non noto
	AutoreID *uuid.UUID        `json:"autore_id,omitempty"`
	Effetti  *EffettiEsclusoR4 `json:"effetti,omitempty"`
}

// EffettiEsclusoR4: gli effetti di un'esclusione che R4 sa dal database (EM §7.4): le occorrenze toccate e i prodotti
// con lo stesso STEP, firmati.
type EffettiEsclusoR4 struct {
	OccorrenzeToccate       []uuid.UUID          `json:"occorrenze_toccate"`
	ProdottiConLoStessoStep []ProdottoStessoStep `json:"prodotti_con_lo_stesso_step"`
	// le righe e gli archi legacy che R5 chiudera', i figli (nessuna cascata) e i fabbisogni tolti dopo la conferma (EM §7.4;
	// riepilogo3_completamento.go)
	RigheDaChiudere []RigaDaChiudere  `json:"righe_da_chiudere"`
	ArchiDaChiudere []ArcoDaChiudere  `json:"archi_da_chiudere"`
	Figli           []string          `json:"figli"`
	FabbisogniTolti []json.RawMessage `json:"fabbisogni_tolti_dopo_conferma"`
}

// ProdottoStessoStep: un prodotto finito che ha lo stesso sha256 come STEP autorizzato, fonte di una bozza o di una
// conferma.
type ProdottoStessoStep struct {
	ProdottoID uuid.UUID `json:"prodotto_id"`
	Come       string    `json:"come"`
}

// PreassociazioneR4: una voce di documenti_preassociati[]: l'esito del Piano, la provenienza decisa dal server, il gruppo
// D-PB-04, il componente di destinazione (un registrato; nil per un nodo da creare).
type PreassociazioneR4 struct {
	associazioni.EsitoPB
	Provenienza  string     `json:"provenienza"`
	Importazione string     `json:"importazione,omitempty"`
	Verificata   bool       `json:"verificata,omitempty"`
	Gruppo       string     `json:"gruppo"`
	NomeFile     string     `json:"nome_file,omitempty"`
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	Effetto      string     `json:"effetto"`
}

// EffettiDocumentaliR4: gli effetti documentali della conferma, raggruppati (D-PB-04 A [U]; EM App. I.8).
type EffettiDocumentaliR4 struct {
	MotoreIntatta      []VoceGruppoR4      `json:"motore_intatta"`
	Corretta           []VoceGruppoR4      `json:"corretta"`
	Manuale            []VoceGruppoR4      `json:"manuale"`
	LegacyDaVerificare []VoceGruppoR4      `json:"legacy_da_verificare"`
	LegacyVerificate   []VoceGruppoR4      `json:"legacy_verificate"`
	ProposteRimosse    []PropostaRimossaR4 `json:"proposte_rimosse"`
	SpostamentiAR      []SpostamentoARR4   `json:"spostamenti_ar"`
	ARInvariate        []ARInvariataR4     `json:"ar_invariate"`
}

// VoceGruppoR4: una PB in un gruppo.
type VoceGruppoR4 struct {
	AllegatoID   uuid.UUID `json:"allegato_id"`
	ChiaveBozza  string    `json:"chiave_bozza"`
	Importazione string    `json:"importazione,omitempty"`
	Esito        string    `json:"esito"`
	Motivo       string    `json:"motivo,omitempty"`
}

// PropostaRimossaR4: una PB del motore entrata nella bozza (alla nascita o in un comando esplicito) e poi tolta
// dall'ingegnere: nessuna AR nascera' per lei (D-PB-04 A [U], «proposte rimosse»).
type PropostaRimossaR4 struct {
	AllegatoID   uuid.UUID `json:"allegato_id"`
	ChiaveBozza  string    `json:"chiave_bozza"`
	Importazione string    `json:"importazione,omitempty"`
}

// SpostamentoARR4: una AR esistente che la conferma sposta (sostituzione) o chiude (svincolo).
type SpostamentoARR4 struct {
	Tipo         string     `json:"tipo"`
	AllegatoID   uuid.UUID  `json:"allegato_id"`
	Associazione *uuid.UUID `json:"associazione_id,omitempty"`
	DaComponente *uuid.UUID `json:"da_componente_id,omitempty"`
	AChiaveBozza string     `json:"a_chiave_bozza,omitempty"`
	AComponente  *uuid.UUID `json:"a_componente_id,omitempty"`
	Esito        string     `json:"esito"`
	Motivo       string     `json:"motivo,omitempty"`
	DaDecisione  string     `json:"da_decisione,omitempty"`
}

// ARInvariataR4: una AR attiva che resta com'e' con la conferma, con il motivo per cui e' mostrata (escluso «Mantieni»;
// ricodifica del componente con D-ASS-01 «mantenere»: R5 scrive l'evento mantieni e allinea la copia).
type ARInvariataR4 struct {
	AllegatoID   uuid.UUID `json:"allegato_id"`
	ComponenteID uuid.UUID `json:"componente_id"`
	Associazione uuid.UUID `json:"associazione_id"`
	Motivo       string    `json:"motivo"`
	Testo        string    `json:"testo"`
}

// CondivisioneR4: un altro prodotto della RFQ toccato dall'atto (EM §7.7, D-TAB-03 A [T-39]): gli elementi condivisi, gli
// effetti, la conseguenza attribuita a chi conferma.
type CondivisioneR4 struct {
	ProdottoID     uuid.UUID           `json:"prodotto_id"`
	Codice         string              `json:"codice"`
	Elementi       []ElementoCondiviso `json:"elementi"`
	ConfermaValida bool                `json:"conferma_valida_prima"`
	Conseguenza    string              `json:"conseguenza"`
}

// ElementoCondiviso: un componente o lo STEP che l'altro prodotto condivide, con l'effetto dell'atto. Tipo: componente
// (effetto strutturale), step (lo stesso STEP sorgente) o documento (un componente condiviso di cui l'atto cambia le
// associazioni: svincolo o AR nuova; EM H.5, dir. 35 Prova 8 [T-39 corsia persistenza]).
type ElementoCondiviso struct {
	Tipo         string     `json:"tipo"`
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	Codice       string     `json:"codice,omitempty"`
	Sha256       string     `json:"sha256,omitempty"`
	Effetto      string     `json:"effetto"`
}

// VoceRegistroR4: una voce di registro_decisioni[] (C3 §4.3; EM §8.3).
type VoceRegistroR4 struct {
	Voce         string          `json:"voce"`
	OrigineVoce  string          `json:"origine_voce"`
	Tipo         string          `json:"tipo"`
	Rimando      json.RawMessage `json:"rimando"`
	AutoreID     *uuid.UUID      `json:"autore_id"`
	Quando       *time.Time      `json:"quando,omitempty"`
	Motivo       string          `json:"motivo,omitempty"`
	Importazione string          `json:"importazione,omitempty"`
	Effetto      string          `json:"effetto"`
}

// CriterioR4: il criterio che la conferma registrerebbe (C3 §4.3), con il motivo; non e' la verifica.
type CriterioR4 struct {
	Criterio string `json:"criterio"`
	Motivo   string `json:"motivo"`
}

// ErroreMotoreR4: perche' gli avvisi del motore non sono calcolati (EM §8.2).
type ErroreMotoreR4 struct {
	Codice string `json:"codice"`
	Testo  string `json:"testo"`
}
