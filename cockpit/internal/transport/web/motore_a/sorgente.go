package motore_a

// sorgente.go: S9, lo smistamento della Sorgente per tutti i prodotti della RFQ (V1, emendamento U2 §2.3, §2.6; U2 §12,
// corsia motore): GET /thread/{id}/motore-a/dati?sorgente=1. È una variante della lettura di /dati: lo stesso calcolo,
// nella stessa fotografia REPEATABLE READ READ ONLY, composto per ogni prodotto valutato con ComponiPagina (gruppi,
// fonte con date e autori, bozza, modello) e riletto per file. Nessuna scrittura; la composizione è pura (L1).
//
// Che cosa dà [T, U2 §2.3; direttiva 35 §8 C-4 [U]; piano delle dipendenze della direttiva 35 §7.2 C-4 [T]]:
//   - step[]: ogni STEP fra i gruppi dei prodotti, con le candidate (prodotto, gruppo, compatibilità, radice letterale,
//     struttura, nodi segnalati LS-02, impronta_contenuto) e sorgente_di quando lo STEP è già la fonte confermata di un
//     altro prodotto (caso (g), base A di U2-Q15);
//   - prodotti[]: per prodotto il codice, l'identificativo del cliente, la fonte con date e autori, la bozza, lo stato
//     del calcolo («non ancora pronto» con i file in analisi, distinto da «nessun abbinamento»), i conti per le cartelle
//     della direzione A, le righe dei collegamenti file → destinazione con il riferimento stabile (mai il codice), lo
//     stato con il motivo, l'origine, i prodotti con cui il file è condiviso (con il motivo della condivisione) e le
//     azioni per stato ({disponibile, motivo}, calcolate qui: [U] M32, RV3-05), e la chip «attende la sorgente»;
//   - da_associare[]: i file da smistare con il motivo del mancato abbinamento che il motore dà già (FileDaSmistare),
//     senza quelli che una bozza preassocia o che hanno una AR attiva (hanno le loro righe);
//   - altri_file[]: i file in «Altri file».
//
// Le righe di un prodotto (direttiva 35, blocco C-4), per allegato, da fonti tenute distinte:
//   - le proposte del motore (PA): stato proposto (o in_conflitto per un conflitto dello smistamento), origine motore;
//   - le preassociazioni salvate nella bozza del prodotto (PB, dalla lettura delle bozze della persistenza,
//     distintabozza.LeggiBozzeDellaRfq): stato salvato_nella_bozza, origine bozza, destinazione {chiave_bozza, casella}
//     risolta per (allegato_id, chiave_bozza), mai per proposta_id né per codice;
//   - le associazioni registrate (AR) attive, dal servizio unico (file_rfq_associazione, righe attive): stato
//     gia_registrato, origine working, destinazione {componente_id}; il documento confermato senza AR resta per
//     «confermato» (gia_registrato con il motivo documento_confermato, D-AR04 (ii) B [U]). Mai documento_proposta.
//     componente_id come fonte delle AR;
//   - dalla bozza anche gli svincoli pendenti (da_scollegare, con svincolo_gia_chiuso se la AR è già chiusa: D-ASS-03
//     [U]) e i file da ricollocare (da_ricollocare: il vassoio di R1, con la stessa regola sul giornale, EM §3.7; la
//     destinazione è vuota e uscita dice il nodo da cui il file è uscito).
//
// Quale riga vince per un allegato in una sezione [T]: gli svincoli si mostrano sempre; poi la PB (in_conflitto con
// gia_associata_altrove se il file ha una AR attiva su un altro componente senza lo svincolo nella stessa bozza, con
// documento_confermato_altrove se è un documento confermato altrove, con file_conteso_fra_prodotti se un'altra bozza lo
// preassocia a un'altra destinazione; gia_registrato se la PB coincide con la AR o con il documento; altrimenti
// salvato_nella_bozza); poi il vassoio; poi la AR attiva (nelle sezioni dei prodotti per cui il file è pertinente e in
// quella del prodotto che contiene il componente); poi il documento confermato; poi le proposte del motore (mai verso il
// componente che la bozza scollega).
//
// La condivisione: condiviso_con[] dice gli altri prodotti che hanno righe dello stesso file; condivisione[] dice per
// ognuno se il file va allo stesso componente in tutti e due (stesso_componente: il componente condiviso fra prodotti)
// o a destinazioni diverse (file_pertinente: il file condiviso). Nessun byte duplicato.
//
// Direttiva 39 [U + T-39], le regole nuove:
//   - D-AR11 B con la precisazione [U]: per un prodotto senza una sorgente confermata da S8 e valida (la fonte del
//     prodotto, sorgente_s8.valida: confermabom.SorgenteValida) il motore non propone file: nessuna riga proposto (né
//     in_conflitto del motore) e nessun «salva nella bozza» dal motore; i file restano visibili nella chip «attende la
//     sorgente» (in_attesa_sorgente con il motivo sorgente_non_confermata) o in da_associare, e si preassociano a mano; le
//     PB manuali della bozza restano salvato_nella_bozza;
//   - lo STEP sorgente del finito ha la sua riga in sola lettura: legato_come_sorgente (la conferma valida di S8 in
//     fonte_step_evento, motivo conferma_s8) oppure legame_storico (lo STEP storico univoco di D-AR04 (iii), per un
//     prodotto senza conferme di S8: non soddisfa il gate, si riconferma da S8); nessuna azione; una PB su quello STEP è
//     in_conflitto con il motivo legato_come_sorgente (lo STEP sorgente non è mai una PB, EM I.10);
//   - D-PB02 A [U]: i suggerimenti dopo l'apertura della bozza (preassociazioni_proposte[] della sezione, mai in
//     da_rivedere[]) hanno una sola sorgente e una sola forma, quelle di R1 con ?suggerimenti_motore=1 [T-39, rilievi
//     delle revisioni; giro G39, G39-01 [U]]: le proposte univoche del registro della versione che la bozza non applica,
//     più quelle del calcolo di adesso per i file che il registro della versione non propone (un file arrivato dopo
//     l'apertura con una destinazione univoca del motore), con la regola delle PB iniziali sui dati salvati di adesso
//     (distintabozza.SuggerimentiDellaBozzaApertaCon, composta dopo il calcolo: suggerimenti_adesso.go), nella forma
//     {allegato_id, chiave_bozza, provenienza: motore, importazione, motivo}, motivo dopo_apertura | pb_toccata |
//     ar_attiva | conflitto. La chiave_bozza c'è sempre e importazione è l'impronta del registro che propone il file (il
//     registro della versione o quello del calcolo di adesso): accettato com'è con un salvataggio R2, il suggerimento è
//     una PB «motore» (D-PB03 A); spostato su un altro nodo, «corretta». Senza la sorgente della versione confermata da S8
//     l'elenco è vuoto. Le proposte ambigue del calcolo di adesso non sono suggerimenti: restano righe proposto con
//     proposta_ambigua (D-PB01 A) e si salvano a mano (PB manuali);
//   - la provenienza della riga: motore per le proposte; per le PB quella dello snapshot (D-PB03 A [U]: motore, corretta,
//     manuale, legacy; distintabozza.PreassociazioneDellaBozza.Provenienza), vuota per una PB salvata prima che il
//     server la scrivesse;
//   - l'ordine delle anteprime della direttiva 35 §7 [U] (DIR35-V3): priorita_anteprima 1 per il documento ufficiale
//     corrente (AR, documento confermato, STEP sorgente), 2 per ciò che la bozza porterà alla conferma (PB salvate), 3
//     per il miglior candidato del motore (proposto), distinto dai documenti ufficiali; vuota per gli altri stati.
//
// Giro G39 (G39-02 [U]; proposta_sorgente.go): lo STEP candidato del prodotto finito, letto con il Motore A
// (propostabom.CandidatureSorgenteDelFinito), non è mai una riga proposto o in_conflitto del motore né la chip «attende la
// sorgente», in nessuna sezione; se univoco non è in da_associare ed è nella fonte del prodotto come proposta di sorgente
// (fonte.proposte_sorgente[], con le evidenze della lettura e sorgente_di se è la sorgente di S8 di un altro prodotto).
// Giro G39-bis (21/10): una PB su uno STEP candidato è in_conflitto con step_candidato_del_finito, come il rifiuto di R2 e
// il blocco di R4 (distintabozza.StepCandidati.BloccaPB); solo la candidatura univoca si propone come sorgente; la
// candidatura tolta dall'evidenza discordante (D-2 (c) [U]) non c'è e lo STEP è un file come gli altri. Giro G39-ter
// (G39-06 [U]: quando il motore non sa decidere sceglie l'utente; lettura [T] dell'orchestratore): la PB su uno STEP
// candidato NON univoco verso una parte o un sottoassieme è la scelta dell'utente e non è in_conflitto (segue le regole
// di ogni PB); verso un prodotto finito (il prodotto della sezione, un nodo di tipo finito o un componente finito della
// RFQ) resta in_conflitto. Le righe proposte del motore restano escluse per ogni candidatura.
//
// Limiti dichiarati [T]:
//   - una PB di una bozza formato 2 (da convertire) non si legge: le sue righe restano quelle del motore;
//   - un file in «Altri file» o scartato non ha righe, anche se la bozza lo cita ancora (R4 lo dice);
//   - altri_file[] dà l'allegato; nome_sul_nas e stato_copia li legge la corsia file dalla conservazione su ALTRO;
//   - lo stato in_calcolo non c'è: la risposta è già il calcolo finito (con il motore occupato, 503 come /dati);
//   - i suggerimenti si leggono in una transazione di sola lettura aperta dopo il calcolo (come R1 con
//     ?suggerimenti_motore=1): se la bozza cambia fra il calcolo e quella lettura vale la bozza di adesso.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// Sorgente: la risposta di S9.
type Sorgente struct {
	Firma       string              `json:"firma"`
	Step        []StepDellaSorgente `json:"step"`
	Prodotti    []ProdottoSorgente  `json:"prodotti"`
	DaAssociare []FileDaAssociare   `json:"da_associare"`
	AltriFile   []FileInAltriFile   `json:"altri_file"`
	Calcolato   string              `json:"calcolato,omitempty"`
}

// StepDellaSorgente: uno STEP fra i gruppi dei prodotti, con le sue candidate. Analisi: pronta | in_analisi.
type StepDellaSorgente struct {
	AllegatoID uuid.UUID            `json:"allegato_id"`
	NomeFile   string               `json:"nome_file"`
	Analisi    string               `json:"analisi"`
	Candidate  []CandidataDelloStep `json:"candidate"`
}

// CandidataDelloStep: lo STEP come gruppo di un prodotto (i campi di gruppi[] di /dati) e, se c'è, il prodotto di cui è
// già la fonte confermata.
type CandidataDelloStep struct {
	ProdottoID        *uuid.UUID                      `json:"prodotto_id"`
	Prodotto          string                          `json:"prodotto"`
	Gruppo            string                          `json:"gruppo"`
	Compatibilita     string                          `json:"compatibilita,omitempty"`
	RadiceLetterale   string                          `json:"radice_letterale,omitempty"`
	Struttura         *propostabom.StrutturaDelGruppo `json:"struttura,omitempty"`
	Segnalati         int                             `json:"segnalati"`
	ImprontaContenuto string                          `json:"impronta_contenuto,omitempty"`
	SorgenteDi        *uuid.UUID                      `json:"sorgente_di,omitempty"`
}

// ProdottoSorgente: la sezione di un prodotto.
type ProdottoSorgente struct {
	ProdottoID            *uuid.UUID                         `json:"prodotto_id"`
	Prodotto              string                             `json:"prodotto"`
	Codice                string                             `json:"codice"`
	IdentificativoCliente *propostabom.IdentificativoCliente `json:"identificativo_cliente,omitempty"`
	Fonte                 FonteSorgente                      `json:"fonte"`
	Bozza                 *BozzaDellaSorgente                `json:"bozza,omitempty"`
	Calcolo               CalcoloDelProdotto                 `json:"calcolo"`
	Conti                 ContiDelProdotto                   `json:"conti"`
	Righe                 []RigaSorgente                     `json:"righe"`
	InAttesaSorgente      []FileInAttesaSorgente             `json:"in_attesa_sorgente"`
	// PreassociazioniProposte: i suggerimenti dopo l'apertura della bozza (D-PB02 A [U]; EM I.8), mai in da_rivedere[]:
	// gli stessi di R1 per la bozza aperta del prodotto (commento di testa), vuoti senza bozza.
	PreassociazioniProposte []distintabozza.PreassociazioneProposta3 `json:"preassociazioni_proposte"`
}

// FonteSorgente: la fonte del prodotto nella forma di S9 (U2 §2.3, §2.4): lo STEP, se confermata e autorizzata, con date
// e autori (datiDellaFonte, gli stessi di /dati.fonte), e il motivo della conferma non ammessa.
type FonteSorgente struct {
	AllegatoID        *uuid.UUID `json:"allegato_id,omitempty"`
	Confermata        bool       `json:"confermata"`
	ConfermataIl      *time.Time `json:"confermata_il,omitempty"`
	ConfermataDa      *Persona   `json:"confermata_da,omitempty"`
	Autorizzata       bool       `json:"autorizzata"`
	AutorizzataIl     *time.Time `json:"autorizzata_il,omitempty"`
	AutorizzataDa     *Persona   `json:"autorizzata_da,omitempty"`
	AutorizzataMotivo string     `json:"autorizzata_motivo,omitempty"`
	Motivo            string     `json:"motivo,omitempty"`
	// direttiva 39: gli stessi campi di /dati.fonte (FonteDelProdotto)
	ConfermaAmmessa    bool                       `json:"conferma_ammessa"`
	Gesto              string                     `json:"gesto,omitempty"`
	SorgenteConfermata bool                       `json:"sorgente_confermata"`
	SorgenteS8         *SorgenteS8DellaFonte      `json:"sorgente_s8,omitempty"`
	LegameStorico      *LegameStorico             `json:"legame_storico,omitempty"`
	StrutturaSTEP      *propostabom.StrutturaSTEP `json:"struttura_step,omitempty"`
	// giro G39 (G39-02 [U]): le proposte di sorgente del Motore A, le stesse di /dati.fonte (proposta_sorgente.go)
	ProposteSorgente []PropostaSorgente `json:"proposte_sorgente"`
}

// BozzaDellaSorgente: la bozza aperta formato 3 del prodotto e il suo STEP sorgente.
type BozzaDellaSorgente struct {
	BozzaID  uuid.UUID  `json:"bozza_id"`
	Versione int32      `json:"versione"`
	Sorgente *uuid.UUID `json:"sorgente,omitempty"`
}

// CalcoloDelProdotto: pronto | in_attesa_analisi | errore, con il motivo e i file ancora in analisi.
type CalcoloDelProdotto struct {
	Stato         string      `json:"stato"`
	Motivo        string      `json:"motivo,omitempty"`
	FileInAnalisi []uuid.UUID `json:"file_in_analisi"`
}

// ContiDelProdotto: i conti dei collegamenti per la cartella del prodotto (direzione A): nessun conto lo fa il browser.
// Uno per stato delle righe (direttiva 35, blocco C-4): proposti (PA), salvati (PB), registrati (AR attive, anche quelle
// che una PB conferma), confermati (documenti confermati senza AR), da_scollegare, da_ricollocare, in_conflitto.
type ContiDelProdotto struct {
	Proposti      int `json:"proposti"`
	Salvati       int `json:"salvati"`
	Registrati    int `json:"registrati"`
	Confermati    int `json:"confermati"`
	DaScollegare  int `json:"da_scollegare"`
	DaRicollocare int `json:"da_ricollocare"`
	InConflitto   int `json:"in_conflitto"`
	NodiSenzaFile int `json:"nodi_senza_file"`
	// direttiva 39: le righe dello STEP sorgente (legato_come_sorgente) e del legame storico
	LegatiComeSorgente int `json:"legati_come_sorgente"`
	LegamiStorici      int `json:"legami_storici"`
}

// RigaSorgente: un collegamento file → destinazione di un prodotto. Motivo: il perché dello stato (in_conflitto,
// da_scollegare già chiuso, il documento confermato, il motivo dell'uscita di un file da ricollocare). Uscita: per un
// file da ricollocare, il nodo della bozza da cui è uscito (la destinazione è vuota). Condivisione: per ogni prodotto di
// condiviso_con, nello stesso ordine, il motivo.
type RigaSorgente struct {
	AllegatoID            uuid.UUID                          `json:"allegato_id"`
	PropostaID            *uuid.UUID                         `json:"proposta_id,omitempty"`
	Destinazione          DestinazioneRiga                   `json:"destinazione"`
	Uscita                *DestinazioneRiga                  `json:"uscita,omitempty"`
	CodiceMostrato        string                             `json:"codice_mostrato"`
	RevMostrata           string                             `json:"rev_mostrata,omitempty"`
	IdentificativoCliente *propostabom.IdentificativoCliente `json:"identificativo_cliente,omitempty"`
	Stato                 string                             `json:"stato"`
	Motivo                string                             `json:"motivo,omitempty"`
	Origine               string                             `json:"origine"`
	Provenienza           string                             `json:"provenienza,omitempty"`
	PrioritaAnteprima     int                                `json:"priorita_anteprima,omitempty"`
	CondivisoCon          []uuid.UUID                        `json:"condiviso_con"`
	Condivisione          []CondivisioneRiga                 `json:"condivisione"`
	Azioni                AzioniRiga                         `json:"azioni"`
	// componente: il componente a cui la riga porta il file (la AR, il documento, lo svincolo, il nodo riusato della PB,
	// la proposta verso un componente), per il motivo della condivisione; nil per un nodo nuovo. Non va nel JSON.
	componente *uuid.UUID
}

// CondivisioneRiga: un altro prodotto con lo stesso file e il motivo: stesso_componente | file_pertinente.
type CondivisioneRiga struct {
	ProdottoID uuid.UUID `json:"prodotto_id"`
	Motivo     string    `json:"motivo"`
}

// DestinazioneRiga: il riferimento stabile della riga (MAT §4.1, regola 1): la voce del registro prima della bozza, la
// casella con la bozza, la chiave_bozza per una PB (con la casella del nodo dello STEP, se c'è), il componente per un
// registrato. Mai il codice.
type DestinazioneRiga struct {
	ChiaveBozza  string       `json:"chiave_bozza,omitempty"`
	Casella      *CasellaRiga `json:"casella,omitempty"`
	Voce         string       `json:"voce,omitempty"`
	ComponenteID *uuid.UUID   `json:"componente_id,omitempty"`
}

// CasellaRiga: la casella di un nodo dello STEP, {allegato_id, gruppo, rif}.
type CasellaRiga struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Gruppo     string    `json:"gruppo"`
	Rif        string    `json:"rif"`
}

// AzioneSorgente: un'azione di una riga, come in S1 (DEC-A PE-06).
type AzioneSorgente struct {
	Disponibile bool   `json:"disponibile"`
	Motivo      string `json:"motivo,omitempty"`
}

// AzioniRiga: le quattro azioni di una riga, per stato.
type AzioniRiga struct {
	SalvaNellaBozza      AzioneSorgente `json:"salva_nella_bozza"`
	TogliDallaBozza      AzioneSorgente `json:"togli_dalla_bozza"`
	Sposta               AzioneSorgente `json:"sposta"`
	ScollegaAllaConferma AzioneSorgente `json:"scollega_alla_conferma"`
}

// FileInAttesaSorgente: la chip «attende la sorgente» (G-6 di LV4). Motivo (direttiva 39): sorgente_non_confermata per
// un prodotto senza la sorgente confermata da S8 (D-AR11 B [U]: il motore non propone file); vuoto per la regola di prima
// (prodotto senza bozza con il file proposto in tutte le candidate compatibili).
type FileInAttesaSorgente struct {
	AllegatoID     uuid.UUID `json:"allegato_id"`
	Voce           string    `json:"voce"`
	CodiceMostrato string    `json:"codice_mostrato"`
	Motivo         string    `json:"motivo,omitempty"`
}

// FileDaAssociare: un file senza abbinamento, con il motivo del motore.
type FileDaAssociare struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Motivo     string    `json:"motivo"`
}

// FileInAltriFile: un file in «Altri file». NomeSulNas e StatoCopia: della corsia file.
type FileInAltriFile struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	NomeSulNas string    `json:"nome_sul_nas,omitempty"`
	StatoCopia string    `json:"stato_copia,omitempty"`
}

// Gli stati, le origini e i motivi di S9 [T].
const (
	StatoRigaProposto      = "proposto"
	StatoRigaSalvato       = "salvato_nella_bozza"
	StatoRigaRegistrato    = "gia_registrato"
	StatoRigaDaScollegare  = "da_scollegare"
	StatoRigaDaRicollocare = "da_ricollocare"
	StatoRigaInConflitto   = "in_conflitto"
	OrigineRigaMotore      = "motore"
	OrigineRigaBozza       = "bozza"
	OrigineRigaWorking     = "working"
	CalcoloPronto          = "pronto"
	CalcoloInAttesaAnalisi = "in_attesa_analisi"
	CalcoloErrore          = "errore"
	AnalisiPronta          = "pronta"
	AnalisiInCorso         = "in_analisi"

	MotivoCalcoloFileInAnalisi  = "file_in_analisi"
	MotivoCalcoloGruppoInErrore = "gruppo_non_importato"

	// I motivi delle righe (direttiva 35, blocco C-4). gia_associata_altrove e documento_confermato_altrove sono quelli di
	// R4 (PREASSOCIAZIONE_NON_APPLICABILE, piano §7.2 C-1a); file_conteso_fra_prodotti quello dell'avviso di R4
	// (FILE_CONTESO_FRA_PRODOTTI); svincolo_gia_chiuso l'esito di R5 per uno svincolo già soddisfatto (D-ASS-03 [U]).
	// Un file da ricollocare porta il motivo dello svincolo del vassoio (nodo_escluso, nodo_tolto, ...).
	MotivoRigaGiaAssociataAltrove  = "gia_associata_altrove"
	MotivoRigaDocumentoAltrove     = "documento_confermato_altrove"
	MotivoRigaFileConteso          = "file_conteso_fra_prodotti"
	MotivoRigaConflittoSmistamento = "conflitto_di_smistamento"
	MotivoRigaDocumentoConfermato  = "documento_confermato"
	MotivoRigaSvincoloGiaChiuso    = "svincolo_gia_chiuso"
	CondivisioneStessoComponente   = "stesso_componente"
	CondivisioneFilePertinente     = "file_pertinente"

	MotivoAzioneSenzaBozza      = "il prodotto non ha una bozza: conferma prima lo STEP come sorgente"
	MotivoAzioneNonNellaBozza   = "il collegamento non è nella bozza"
	MotivoAzioneNonRegistrato   = "il file non è registrato su questo componente"
	MotivoAzioneGiaRegistrato   = "il file è già registrato su questo componente"
	MotivoAzioneInConflitto     = "il collegamento è in conflitto: scegli la destinazione con «Associa a un nodo…»"
	MotivoAzioneGiaNellaBozza   = "il collegamento è già nella bozza"
	MotivoAzioneDocumento       = "il file è un documento confermato: si cambia dal fascicolo"
	MotivoAzioneSvincolo        = "lo scollegamento è già nella bozza: «Togli dalla bozza» lo annulla"
	MotivoAzioneDaRicollocare   = "il file è uscito dalla bozza: scegli la destinazione con «Associa a un nodo…»"
	MotivoAzioneSvincoloAltrove = "il file è registrato su un altro componente: scollegalo nella stessa bozza o togli la preassociazione"

	// direttiva 39
	StatoRigaLegatoComeSorgente  = "legato_come_sorgente"
	StatoRigaLegameStorico       = "legame_storico"
	OrigineRigaSorgente          = "sorgente"
	ProvenienzaMotore            = "motore"
	MotivoRigaConfermaS8         = "conferma_s8"
	MotivoRigaLegameStorico      = "legame_storico_univoco"
	MotivoRigaLegatoComeSorgente = "legato_come_sorgente"
	MotivoRigaPropostaAmbigua    = "proposta_ambigua"
	MotivoAttesaNonConfermata    = "sorgente_non_confermata"
	MotivoSuggerimentoApertura   = distintabozza.PropostaDopoApertura // i motivi di preassociazioni_proposte[], quelli di R1
	MotivoSuggerimentoPBToccata  = distintabozza.PropostaPBToccata
	MotivoSuggerimentoARAttiva   = distintabozza.PropostaARAttiva
	MotivoSuggerimentoConflitto  = distintabozza.PropostaConflitto
	MotivoAzioneSorgente         = "lo STEP sorgente del prodotto si cambia con «Conferma questo STEP come sorgente» (S8) o con la revoca, non da qui"
	MotivoAzioneLegameStorico    = "lo STEP storico del prodotto non è ancora la sorgente confermata: confermalo con «Conferma questo STEP come sorgente» (S8)"
	PrioritaDocumentoUfficiale   = 1
	PrioritaNellaBozza           = 2
	PrioritaCandidatoMotore      = 3

	// giro G39 (G39-02 [U]): la PB su uno STEP candidato del prodotto finito (la lettura del Motore A) è in conflitto:
	// lo STEP del finito non è mai una PB, si conferma con S8
	MotivoRigaStepCandidatoDelFinito = "step_candidato_del_finito"

	// statoAltriFile: lo stato di un'occorrenza in «Altri file» nella lettura del ciclo dei file.
	statoAltriFile = "altri_file"
)

// Sorgente: S9 per la RFQ (ComponiSorgente), sotto lo stesso limite, lo stesso recover e la stessa fotografia di Pagina.
func (s Servizio) Sorgente(ctx context.Context, thread uuid.UUID) (*Sorgente, error) {
	var out *Sorgente // scritta nella goroutine, letta qui dopo la consegna sul canale (happens-before)
	var errBozze error
	var errAdesso error
	if _, err := s.calcolaProtetto(ctx, thread, func(cx context.Context, r *Risultato) {
		if r.V1.ErroreBozze == nil {
			// giro G39 (G39-01 [U]): i suggerimenti di R1 con il calcolo di adesso (suggerimenti_adesso.go)
			errAdesso = s.suggerimentiDiAdesso(cx, thread, r)
		}
		if errBozze = r.V1.ErroreBozze; errBozze == nil && errAdesso == nil {
			out = ComponiSorgente(r, thread)
		}
	}); err != nil {
		return nil, err
	}
	if errAdesso != nil {
		return nil, errAdesso
	}
	// le PB, gli svincoli e il vassoio di S9 vengono dalle bozze: con uno snapshot che non si rilegge S9 non risponde a metà
	if errBozze != nil {
		return nil, fmt.Errorf("%w: le bozze dei prodotti non si leggono: %w", ErrLettura, errBozze)
	}
	return out, nil
}

// rifComponente: il prefisso del riferimento di un componente deciso (ancoraggio.RifComponente).
const rifComponente = "componente:"

// bozzeDellaSorgente: le letture delle bozze e delle AR della RFQ, per allegato, comuni a tutte le sezioni.
type bozzeDellaSorgente struct {
	bozza map[uuid.UUID]*distintabozza.BozzaDellaRfq // per prodotto, solo formato 3
	ar    map[uuid.UUID]uuid.UUID                    // allegato → componente della AR attiva (una per allegato: D-ASS-04 A)
	pb    map[uuid.UUID][]pbDiUnaBozza               // allegato → le PB di tutte le bozze formato 3
}

// pbDiUnaBozza: una PB di un allegato in una bozza: il prodotto e il componente del nodo (nil per un nodo nuovo).
type pbDiUnaBozza struct {
	prodotto   uuid.UUID
	componente *uuid.UUID
}

func leggiBozzeDellaSorgente(lb distintabozza.BozzeDellaRfq) bozzeDellaSorgente {
	out := bozzeDellaSorgente{bozza: map[uuid.UUID]*distintabozza.BozzaDellaRfq{}, ar: map[uuid.UUID]uuid.UUID{}, pb: map[uuid.UUID][]pbDiUnaBozza{}}
	for i := range lb.Bozze {
		b := &lb.Bozze[i]
		if b.DaConvertire {
			continue
		}
		out.bozza[b.ProdottoID] = b
		for _, p := range b.Preassociazioni {
			out.pb[p.AllegatoID] = append(out.pb[p.AllegatoID], pbDiUnaBozza{prodotto: b.ProdottoID, componente: p.Nodo.ComponenteID})
		}
	}
	for _, a := range lb.Associazioni {
		if a.Attiva() {
			out.ar[a.Allegato] = a.Componente
		}
	}
	return out
}

// contesaAltrove: un'altra bozza preassocia lo stesso file a un'altra destinazione (componenti diversi, o un nodo nuovo
// da una parte o dall'altra: due bozze non creano lo stesso nodo).
func (b bozzeDellaSorgente) contesaAltrove(allegato, prodotto uuid.UUID, componente *uuid.UUID) bool {
	for _, x := range b.pb[allegato] {
		if x.prodotto == prodotto {
			continue
		}
		if componente == nil || x.componente == nil || *componente != *x.componente {
			return true
		}
	}
	return false
}

// nodoDiUnFinito: il nodo della PB è un prodotto finito: il prodotto della sezione, un nodo di tipo finito o il nodo di un
// componente finito della RFQ (la stessa regola di distintabozza.NodoVersoUnFinito, sulla fotografia del calcolo).
func nodoDiUnFinito(r *Risultato, pv valutazione.ProdottoValutato, n distintabozza.NodoDellaBozza) bool {
	if n.Tipo == "finito" {
		return true
	}
	if n.ComponenteID == nil {
		return false
	}
	if pv.ComponenteID != nil && *n.ComponenteID == *pv.ComponenteID {
		return true
	}
	if r.Thread != nil {
		for _, c := range r.Thread.Componenti {
			if c.ID == *n.ComponenteID {
				return c.Tipo == "finito"
			}
		}
	}
	return false
}

// ComponiSorgente: S9 da un calcolo riuscito, pura. I prodotti nell'ordine dei prodotti valutati; gli STEP, i file da
// associare e gli Altri file per allegato; le righe per allegato e destinazione.
func ComponiSorgente(r *Risultato, thread uuid.UUID) *Sorgente {
	out := &Sorgente{Step: []StepDellaSorgente{}, Prodotti: []ProdottoSorgente{}, DaAssociare: []FileDaAssociare{}, AltriFile: []FileInAltriFile{}}
	if r == nil || r.Thread == nil || r.EsitoThread == nil {
		return out
	}
	et := r.EsitoThread
	out.Calcolato = r.Foto.PresaIl.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	// «non ancora pronto»: un file i cui fatti non ci sono ancora (disponibilità pendente o mancante), distinto da un file
	// letto che non si abbina (da_associare con il motivo del motore)
	inAnalisi := map[uuid.UUID]bool{}
	for _, f := range et.File {
		if f.Disponibilita == ancoraggio.DisponibilitaPendente || f.Disponibilita == ancoraggio.DisponibilitaMancante {
			inAnalisi[f.AllegatoID] = true
		}
	}
	bz := leggiBozzeDellaSorgente(r.V1.LetturaBozze)
	// giro G39 (G39-02 [U]): gli STEP candidati del prodotto finito (la lettura del Motore A) non sono mai file da
	// preassociare; quelli univoci stanno nella fonte del loro prodotto (proposte_sorgente[])
	cc := r.CandidatureSorgente()
	candidati := propostabom.StepCandidatiDelFinito(cc)
	univoci := map[uuid.UUID]bool{}
	for _, c := range cc {
		if c.Univoca {
			univoci[c.AllegatoID] = true
		}
	}
	pagine := make([]*Pagina, len(et.ProdottiValutati))
	fonteDi := map[uuid.UUID]uuid.UUID{} // allegato dello STEP confermato come fonte → prodotto
	for i, pv := range et.ProdottiValutati {
		pagine[i] = ComponiPagina(r, RichiestaPagina{Thread: thread, Prodotto: pv.Rif})
		if f := pagine[i].Fonte; f != nil && f.Confermata && f.AllegatoID != nil && pv.ComponenteID != nil {
			fonteDi[*f.AllegatoID] = *pv.ComponenteID
		}
	}
	step := map[uuid.UUID]*StepDellaSorgente{}
	for i, pv := range et.ProdottiValutati {
		p := pagine[i]
		for _, g := range p.Gruppi {
			if g.AllegatoID == nil || g.Tipo == propostabom.GruppoSenzaSTEP {
				continue
			}
			s, ok := step[*g.AllegatoID]
			if !ok {
				s = &StepDellaSorgente{AllegatoID: *g.AllegatoID, NomeFile: g.NomeFile, Analisi: AnalisiPronta, Candidate: []CandidataDelloStep{}}
				if inAnalisi[*g.AllegatoID] {
					s.Analisi = AnalisiInCorso
				}
				step[*g.AllegatoID] = s
			}
			c := CandidataDelloStep{ProdottoID: pv.ComponenteID, Prodotto: pv.Rif, Gruppo: g.Chiave, Compatibilita: g.Compatibilita,
				RadiceLetterale: g.RadiceLetterale, Struttura: g.Struttura, Segnalati: g.Segnalati, ImprontaContenuto: g.ImprontaContenuto}
			if altro, ok := fonteDi[*g.AllegatoID]; ok && (pv.ComponenteID == nil || altro != *pv.ComponenteID) {
				a := altro
				c.SorgenteDi = &a
			}
			s.Candidate = append(s.Candidate, c)
		}
		out.Prodotti = append(out.Prodotti, sezioneDelProdotto(r, p, pv, inAnalisi, bz, candidati, univoci))
	}
	for _, s := range step {
		out.Step = append(out.Step, *s)
	}
	sort.Slice(out.Step, func(i, j int) bool { return out.Step[i].AllegatoID.String() < out.Step[j].AllegatoID.String() })
	condivisi(out)
	for _, d := range et.DaSmistare {
		if _, na := r.V1.NonAttivi[d.AllegatoID]; na || inAnalisi[d.AllegatoID] {
			continue
		}
		// un file che una bozza preassocia o che ha una AR attiva non è più da associare: ha le sue righe [T]
		if _, conAR := bz.ar[d.AllegatoID]; conAR || len(bz.pb[d.AllegatoID]) > 0 {
			continue
		}
		// lo STEP candidato univoco del prodotto finito è una proposta di sorgente, non un file da associare (G39-02)
		if univoci[d.AllegatoID] {
			continue
		}
		out.DaAssociare = append(out.DaAssociare, FileDaAssociare{AllegatoID: d.AllegatoID, Motivo: string(d.Motivo)})
	}
	for a, stato := range r.V1.NonAttivi {
		if stato == statoAltriFile {
			out.AltriFile = append(out.AltriFile, FileInAltriFile{AllegatoID: a})
		}
	}
	sort.Slice(out.AltriFile, func(i, j int) bool {
		return out.AltriFile[i].AllegatoID.String() < out.AltriFile[j].AllegatoID.String()
	})
	out.Firma = firmaDellaSorgente(out)
	return out
}

// sezioneDelProdotto: la sezione di un prodotto, dalla sua pagina, dalle associazioni dei file del thread, dalla sua
// bozza e dalle AR attive (le regole nel commento di testa). candidati: gli STEP candidati del finito, univoci o no: mai
// righe proposte del motore; univoci: quelli univoci. La PB su uno STEP candidato è in_conflitto come in R2 e R4
// (distintabozza.StepCandidati.BloccaPB): se la candidatura è univoca o se la PB va verso un prodotto finito (giro
// G39-bis; G39-06 [U], giro G39-ter).
func sezioneDelProdotto(r *Risultato, p *Pagina, pv valutazione.ProdottoValutato, inAnalisi map[uuid.UUID]bool, bz bozzeDellaSorgente,
	candidati, univoci map[uuid.UUID]bool) ProdottoSorgente {
	et := r.EsitoThread
	out := ProdottoSorgente{ProdottoID: pv.ComponenteID, Prodotto: pv.Rif, Codice: pv.CodiceRichiesto, Righe: []RigaSorgente{},
		InAttesaSorgente: []FileInAttesaSorgente{}, Calcolo: CalcoloDelProdotto{Stato: CalcoloPronto, FileInAnalisi: []uuid.UUID{}},
		PreassociazioniProposte: []distintabozza.PreassociazioneProposta3{}}
	// direttiva 39: la sorgente confermata da S8 e valida (D-AR11 B [U]: senza, il motore non propone file per il prodotto),
	// lo STEP legato come sorgente e lo STEP storico univoco (D-AR04 (iii)), per le righe in sola lettura
	conS8 := false
	var legato, storico *uuid.UUID
	if f := p.Fonte; f != nil {
		out.Fonte = FonteSorgente{AllegatoID: f.AllegatoID, Confermata: f.Confermata, ConfermataIl: f.ConfermataIl, ConfermataDa: f.ConfermataDa,
			Autorizzata: f.Autorizzata, AutorizzataIl: f.AutorizzataIl, AutorizzataDa: f.AutorizzataDa, AutorizzataMotivo: f.AutorizzataMotivo, Motivo: f.Motivo,
			ConfermaAmmessa: f.ConfermaAmmessa, Gesto: f.Gesto, SorgenteConfermata: f.SorgenteConfermata, SorgenteS8: f.SorgenteS8,
			LegameStorico: f.LegameStorico, StrutturaSTEP: f.StrutturaSTEP, ProposteSorgente: f.ProposteSorgente}
		if f.SorgenteS8 != nil && f.SorgenteS8.Valida {
			conS8 = true
			a := f.SorgenteS8.AllegatoID
			legato = &a
		}
		if f.SorgenteS8 == nil && f.LegameStorico != nil && f.LegameStorico.AllegatoID != nil {
			storico = f.LegameStorico.AllegatoID
		}
	}
	cliente := "cliente:" + et.ClienteID.String()
	var bozza *BozzaAperta
	var bozza3 *distintabozza.BozzaDellaRfq
	if pv.ComponenteID != nil {
		for _, c := range r.Thread.Componenti {
			if c.ID == *pv.ComponenteID {
				out.Codice = c.Codice
				rev := ""
				if c.Rev != nil {
					rev = *c.Rev
				}
				id := propostabom.IdentificativoDelCliente(r.Insieme, cliente, "", c.Codice, rev)
				out.IdentificativoCliente = &id
			}
		}
		if b, ok := r.V1.Bozze[*pv.ComponenteID]; ok {
			bozza = &b
			out.Bozza = &BozzaDellaSorgente{BozzaID: b.BozzaID, Versione: b.Versione, Sorgente: b.FonteAllegato}
		}
		bozza3 = bz.bozza[*pv.ComponenteID]
	}
	conBozza := bozza != nil || bozza3 != nil
	stepDelProdotto := map[uuid.UUID]bool{}
	nodi := map[string]nodoDellaSorgente{}
	compatibili := map[string]bool{} // le candidate compatibili, per sha256 dello STEP
	for _, g := range p.Gruppi {
		if g.AllegatoID != nil {
			stepDelProdotto[*g.AllegatoID] = true
		}
		if g.Importazione != nil && g.Importazione.Codice == propostabom.CodiceErroreCalcolo && out.Calcolo.Stato == CalcoloPronto {
			out.Calcolo.Stato, out.Calcolo.Motivo = CalcoloErrore, MotivoCalcoloGruppoInErrore
		}
		var reg *importazione.Contenuto
		if p.Modello != nil {
			if x, ok := p.Modello.Registri[g.Chiave]; ok {
				reg = &x
			}
		}
		for _, n := range g.Nodi {
			if n.Completezza != nil && (n.Completezza.Esito == string(valutazione.EsitoManca) || n.Completezza.Esito == propostabom.EsitoRequisitoPrevisto) {
				out.Conti.NodiSenzaFile++
			}
			if _, ok := nodi[n.Rif]; ok || g.AllegatoID == nil {
				continue
			}
			nd := nodoDellaSorgente{voce: n.Voce, gruppo: g.Chiave, allegato: *g.AllegatoID, identificativo: n.IdentificativoCliente}
			if reg != nil {
				for _, v := range reg.Proposta.Nodi {
					if v.Voce == n.Voce {
						nd.codice, nd.rev = v.CodiceProposto, v.RevProposta
					}
				}
			}
			nodi[n.Rif] = nd
		}
		if g.Tipo == propostabom.GruppoCandidata && g.Compatibilita == propostabom.CompatibilitaCompatibile {
			compatibili[g.Chiave] = true
		}
	}
	// lo stato del calcolo: «non ancora pronto» (file del prodotto ancora in analisi) è un'altra cosa da «nessun
	// abbinamento»; i file del prodotto sono i suoi STEP e i file pertinenti
	inAnalisiDelProdotto := map[uuid.UUID]bool{}
	for a := range stepDelProdotto {
		if inAnalisi[a] {
			inAnalisiDelProdotto[a] = true
		}
	}
	for _, a := range et.Associazioni {
		if inAnalisi[a.AllegatoID] && contiene(a.Pertinente, pv.Rif) {
			inAnalisiDelProdotto[a.AllegatoID] = true
		}
	}
	for a := range inAnalisiDelProdotto {
		out.Calcolo.FileInAnalisi = append(out.Calcolo.FileInAnalisi, a)
	}
	sort.Slice(out.Calcolo.FileInAnalisi, func(i, j int) bool {
		return out.Calcolo.FileInAnalisi[i].String() < out.Calcolo.FileInAnalisi[j].String()
	})
	if len(out.Calcolo.FileInAnalisi) > 0 {
		out.Calcolo.Stato, out.Calcolo.Motivo = CalcoloInAttesaAnalisi, MotivoCalcoloFileInAnalisi
	}
	working := map[uuid.UUID][2]string{}
	for _, c := range r.Thread.Componenti {
		rev := ""
		if c.Rev != nil {
			rev = *c.Rev
		}
		working[c.ID] = [2]string{c.Codice, rev}
	}
	perimetro := perimetroDelProdotto(r, pv.ComponenteID)

	// gli allegati della sezione: i file pertinenti del motore, quelli che la bozza del prodotto cita e quelli con una AR
	// su un componente del prodotto; in ordine di allegato
	assoc := map[uuid.UUID]*valutazione.AssociazioneFile{}
	allegati := map[uuid.UUID]bool{}
	for i := range et.Associazioni {
		if a := &et.Associazioni[i]; contiene(a.Pertinente, pv.Rif) {
			assoc[a.AllegatoID], allegati[a.AllegatoID] = a, true
		}
	}
	pbDi := map[uuid.UUID]*distintabozza.PreassociazioneDellaBozza{}
	svincoliDi := map[uuid.UUID][]distintabozza.SvincoloPendente{}
	vassoioDi := map[uuid.UUID]*distintabozza.VoceVassoioDellaBozza{}
	if bozza3 != nil {
		for i := range bozza3.Preassociazioni {
			x := &bozza3.Preassociazioni[i]
			pbDi[x.AllegatoID], allegati[x.AllegatoID] = x, true
		}
		for _, s := range bozza3.Svincoli {
			svincoliDi[s.AllegatoID], allegati[s.AllegatoID] = append(svincoliDi[s.AllegatoID], s), true
		}
		for i := range bozza3.Vassoio {
			if v := &bozza3.Vassoio[i]; vassoioDi[v.AllegatoID] == nil {
				vassoioDi[v.AllegatoID], allegati[v.AllegatoID] = v, true
			}
		}
	}
	for a, k := range bz.ar {
		if perimetro[k] {
			allegati[a] = true
		}
	}
	for _, a := range []*uuid.UUID{legato, storico} {
		if a != nil {
			allegati[*a] = true
		}
	}
	ordine := make([]uuid.UUID, 0, len(allegati))
	for a := range allegati {
		ordine = append(ordine, a)
	}
	sort.Slice(ordine, func(i, j int) bool { return ordine[i].String() < ordine[j].String() })

	aggiungi := func(riga RigaSorgente) {
		riga.CondivisoCon, riga.Condivisione = []uuid.UUID{}, []CondivisioneRiga{}
		riga.Azioni = azioniDi(riga, conBozza)
		switch riga.Stato { // l'ordine delle anteprime della direttiva 35 §7 [U]
		case StatoRigaRegistrato, StatoRigaLegatoComeSorgente:
			riga.PrioritaAnteprima = PrioritaDocumentoUfficiale
		case StatoRigaSalvato:
			riga.PrioritaAnteprima = PrioritaNellaBozza
		case StatoRigaProposto:
			riga.PrioritaAnteprima = PrioritaCandidatoMotore
		}
		out.Righe = append(out.Righe, riga)
		switch riga.Stato {
		case StatoRigaLegatoComeSorgente:
			out.Conti.LegatiComeSorgente++
		case StatoRigaLegameStorico:
			out.Conti.LegamiStorici++
		case StatoRigaProposto:
			out.Conti.Proposti++
		case StatoRigaSalvato:
			out.Conti.Salvati++
		case StatoRigaRegistrato:
			if riga.Motivo == MotivoRigaDocumentoConfermato {
				out.Conti.Confermati++
			} else {
				out.Conti.Registrati++
			}
		case StatoRigaDaScollegare:
			out.Conti.DaScollegare++
		case StatoRigaDaRicollocare:
			out.Conti.DaRicollocare++
		case StatoRigaInConflitto:
			out.Conti.InConflitto++
		}
	}
	// sulComponente: la riga porta il file a un componente della working (codice, rev e identificativo di adesso)
	sulComponente := func(riga *RigaSorgente, k uuid.UUID) {
		id := k
		w := working[id]
		riga.componente, riga.CodiceMostrato, riga.RevMostrata = &id, w[0], w[1]
		ic := propostabom.IdentificativoDelCliente(r.Insieme, cliente, "", w[0], w[1])
		riga.IdentificativoCliente = &ic
	}
	// sulNodo: il riferimento di un nodo della bozza (chiave_bozza e casella del nodo dello STEP), con l'identità decisa
	sulNodo := func(riga *RigaSorgente, chiave string, n *distintabozza.NodoDellaBozza) *DestinazioneRiga {
		d := &DestinazioneRiga{ChiaveBozza: chiave}
		if n == nil {
			return d
		}
		if n.Casella != nil {
			d.Casella = &CasellaRiga{AllegatoID: n.Casella.AllegatoID, Gruppo: n.Casella.Gruppo, Rif: n.Casella.Rif}
		}
		riga.CodiceMostrato, riga.RevMostrata = n.Codice, n.Rev
		ic := propostabom.IdentificativoDelCliente(r.Insieme, cliente, "", n.Codice, n.Rev)
		riga.IdentificativoCliente = &ic
		if n.ComponenteID != nil {
			id := *n.ComponenteID
			riga.componente = &id
		}
		return d
	}

	// proposteDelMotore: le righe delle proposte del motore per il file, una per destinazione (mai verso un componente che
	// la bozza scollega), con i gruppi delle candidate in cui il file è proposto (commento di testa, punto 6)
	proposteDelMotore := func(a *valutazione.AssociazioneFile, base RigaSorgente, svincolati map[uuid.UUID]bool) ([]RigaSorgente, map[string]bool) {
		var righe []RigaSorgente
		gruppi := map[string]bool{}
		if a == nil || a.Esclusa {
			return nil, gruppi
		}
		for _, chiave := range a.Proposta {
			_, target, ok := strings.Cut(chiave, "/")
			if !ok {
				continue
			}
			riga := base
			riga.Stato, riga.Origine, riga.Provenienza = StatoRigaProposto, OrigineRigaMotore, ProvenienzaMotore
			switch {
			case strings.HasPrefix(target, rifComponente):
				id, err := uuid.Parse(strings.TrimPrefix(target, rifComponente))
				if err != nil || svincolati[id] {
					continue
				}
				riga.Destinazione.ComponenteID = &id
				sulComponente(&riga, id)
			case strings.HasPrefix(target, ancoraggio.PrefissoRifNodo):
				nd, ok := nodi[target]
				if !ok {
					continue
				}
				gruppi[nd.gruppo] = true
				riga.Destinazione.Voce, riga.CodiceMostrato, riga.RevMostrata = nd.voce, nd.codice, nd.rev
				riga.IdentificativoCliente = nd.identificativo
				if conBozza {
					riga.Destinazione.Casella = &CasellaRiga{AllegatoID: nd.allegato, Gruppo: nd.gruppo, Rif: target}
				}
			default:
				continue
			}
			if a.Conflitto {
				riga.Stato, riga.Motivo = StatoRigaInConflitto, MotivoRigaConflittoSmistamento
			}
			righe = append(righe, riga)
		}
		return righe, gruppi
	}
	for _, allegato := range ordine {
		if _, na := r.V1.NonAttivi[allegato]; na {
			continue
		}
		a := assoc[allegato]
		var proposta *uuid.UUID
		if p.Indice != nil {
			proposta = p.Indice.PropostaAperta(allegato)
		}
		base := RigaSorgente{AllegatoID: allegato, PropostaID: proposta}
		k, conAR := bz.ar[allegato]
		svincolati := map[uuid.UUID]bool{}

		// 0. lo STEP sorgente del prodotto (direttiva 39 §5 [U]): una riga in sola lettura, mai una PB
		if (legato != nil && *legato == allegato) || (storico != nil && *storico == allegato) {
			riga := base
			riga.Stato, riga.Motivo, riga.Origine = StatoRigaLegatoComeSorgente, MotivoRigaConfermaS8, OrigineRigaSorgente
			if legato == nil || *legato != allegato {
				riga.Stato, riga.Motivo = StatoRigaLegameStorico, MotivoRigaLegameStorico
			}
			if pv.ComponenteID != nil {
				id := *pv.ComponenteID
				riga.Destinazione = DestinazioneRiga{ComponenteID: &id}
				sulComponente(&riga, id)
			}
			aggiungi(riga)
			if pb := pbDi[allegato]; pb != nil {
				x := base
				x.Origine = OrigineRigaBozza
				n := pb.Nodo
				x.Destinazione = *sulNodo(&x, pb.ChiaveBozza, &n)
				x.Provenienza = pb.Provenienza
				x.Stato, x.Motivo = StatoRigaInConflitto, MotivoRigaLegatoComeSorgente
				aggiungi(x)
			}
			continue
		}

		// 1. gli svincoli pendenti della bozza del prodotto
		for _, s := range svincoliDi[allegato] {
			riga := base
			riga.Stato, riga.Origine = StatoRigaDaScollegare, OrigineRigaBozza
			id := s.ComponenteID
			riga.Destinazione = DestinazioneRiga{ComponenteID: &id}
			sulComponente(&riga, s.ComponenteID)
			if !s.AssociazioneAttiva {
				riga.Motivo = MotivoRigaSvincoloGiaChiuso
			}
			svincolati[s.ComponenteID] = true
			aggiungi(riga)
		}
		documento := a != nil && a.DocumentoID != nil

		// 2. la PB della bozza del prodotto
		if pb := pbDi[allegato]; pb != nil {
			riga := base
			riga.Origine = OrigineRigaBozza
			n := pb.Nodo
			riga.Destinazione = *sulNodo(&riga, pb.ChiaveBozza, &n)
			riga.Provenienza = pb.Provenienza // D-PB03 A [U]: quella dello snapshot (vuota per una PB salvata senza)
			stessoDoc := documento && a.Confermata != nil && n.ComponenteID != nil && *n.ComponenteID == *a.Confermata
			switch {
			case candidati[allegato] && (univoci[allegato] || nodoDiUnFinito(r, pv, n)):
				// G39-02 [U]: lo STEP candidato del prodotto finito non è mai una PB (si conferma con S8); come R2 e R4
				// (distintabozza.StepCandidati.BloccaPB): la candidatura univoca, o la PB verso un prodotto finito; la PB su
				// una candidatura non univoca verso una parte o un sottoassieme è la scelta dell'utente (G39-06 [U])
				riga.Stato, riga.Motivo = StatoRigaInConflitto, MotivoRigaStepCandidatoDelFinito
			case conAR && n.ComponenteID != nil && *n.ComponenteID == k:
				riga.Stato = StatoRigaRegistrato
				id := k
				riga.Destinazione.ComponenteID = &id
			case conAR && !svincolati[k]:
				riga.Stato, riga.Motivo = StatoRigaInConflitto, MotivoRigaGiaAssociataAltrove
			case !conAR && documento && !stessoDoc:
				riga.Stato, riga.Motivo = StatoRigaInConflitto, MotivoRigaDocumentoAltrove
			case !conAR && stessoDoc:
				riga.Stato, riga.Motivo = StatoRigaRegistrato, MotivoRigaDocumentoConfermato
				id := *a.Confermata
				riga.Destinazione.ComponenteID = &id
			case bz.contesaAltrove(allegato, *pv.ComponenteID, n.ComponenteID):
				riga.Stato, riga.Motivo = StatoRigaInConflitto, MotivoRigaFileConteso
			default:
				riga.Stato = StatoRigaSalvato
			}
			aggiungi(riga)
			continue
		}

		// 3. il file da ricollocare (il vassoio della bozza): nessuna destinazione, il nodo d'uscita a parte
		if v := vassoioDi[allegato]; v != nil {
			riga := base
			riga.Stato, riga.Motivo, riga.Origine = StatoRigaDaRicollocare, v.Motivo, OrigineRigaBozza
			riga.Uscita = sulNodo(&riga, v.ChiaveBozza, v.Nodo)
			riga.componente = nil
			aggiungi(riga)
			continue
		}

		// 4. la AR attiva: nella sezione del prodotto che contiene il componente o per cui il file è pertinente. Una AR che
		// la bozza scollega ha già la sua riga da_scollegare: il file può avere proposte verso altre destinazioni.
		if conAR && !svincolati[k] {
			if perimetro[k] || a != nil {
				riga := base
				id := k
				riga.Destinazione = DestinazioneRiga{ComponenteID: &id}
				riga.Stato, riga.Origine = StatoRigaRegistrato, OrigineRigaWorking
				sulComponente(&riga, k)
				aggiungi(riga)
			}
			continue
		}
		// gli STEP del prodotto stanno in step[], non fra le righe del motore né fra i documenti confermati
		if a == nil || a.Esclusa || stepDelProdotto[allegato] {
			continue
		}

		// 5. il documento confermato su un componente, senza AR (D-AR04 (ii) B [U]: il documento resta per «confermato»)
		if a.Confermata != nil && !svincolati[*a.Confermata] {
			riga := base
			id := *a.Confermata
			riga.Destinazione = DestinazioneRiga{ComponenteID: &id}
			riga.Stato, riga.Motivo, riga.Origine = StatoRigaRegistrato, MotivoRigaDocumentoConfermato, OrigineRigaWorking
			sulComponente(&riga, id)
			aggiungi(riga)
			continue
		}

		// lo STEP candidato del prodotto finito (la lettura del Motore A) non è mai una proposta di preassociazione: è un
		// candidato sorgente, che si conferma con S8 (giro G39, G39-02 [U]; aperto (b) del verificatore)
		if candidati[allegato] {
			continue
		}
		// 6. le proposte del motore: una riga per destinazione (mai verso il componente che la bozza scollega)
		righe, gruppi := proposteDelMotore(a, base, svincolati)
		if len(righe) == 0 {
			continue
		}
		// D-AR11 B [U]: senza la sorgente confermata da S8 il motore non propone file per il prodotto; il file resta
		// visibile nella chip «attende la sorgente» e si preassocia a mano
		if !conS8 {
			out.InAttesaSorgente = append(out.InAttesaSorgente, FileInAttesaSorgente{AllegatoID: allegato, Voce: righe[0].Destinazione.Voce,
				CodiceMostrato: righe[0].CodiceMostrato, Motivo: MotivoAttesaNonConfermata})
			continue
		}
		// la chip «attende la sorgente» per un prodotto senza bozza quando il file è proposto in tutte le candidate
		// compatibili (la regola di prima)
		if !conBozza && len(compatibili) > 0 && copreTutte(gruppi, compatibili) && !a.Conflitto {
			out.InAttesaSorgente = append(out.InAttesaSorgente, FileInAttesaSorgente{AllegatoID: allegato, Voce: righe[0].Destinazione.Voce,
				CodiceMostrato: righe[0].CodiceMostrato})
			continue
		}
		if len(righe) > 1 {
			// più destinazioni per lo stesso file: una proposta ambigua non diventa un suggerimento (D-PB01 A [U])
			for i := range righe {
				if righe[i].Stato == StatoRigaProposto {
					righe[i].Motivo = MotivoRigaPropostaAmbigua
				}
			}
		}
		for _, riga := range righe {
			aggiungi(riga)
		}
	}
	// i suggerimenti dopo l'apertura: quelli di R1 per la bozza aperta del prodotto (commento di testa)
	if bozza3 != nil && pv.ComponenteID != nil {
		out.PreassociazioniProposte = append(out.PreassociazioniProposte, r.V1.Suggerimenti[*pv.ComponenteID]...)
	}
	return out
}

// perimetroDelProdotto: il componente del prodotto e i componenti attivi (non archiviati) raggiungibili da lui per le
// relazioni confermate della working (la BOM confermata, come il perimetro della valutazione); vuoto senza componente.
func perimetroDelProdotto(r *Risultato, prodotto *uuid.UUID) map[uuid.UUID]bool {
	dentro := map[uuid.UUID]bool{}
	if prodotto == nil || r.Thread == nil {
		return dentro
	}
	attivi := map[uuid.UUID]bool{}
	for _, c := range r.Thread.Componenti {
		if c.ArchiviatoIl == nil {
			attivi[c.ID] = true
		}
	}
	figli := map[uuid.UUID][]uuid.UUID{}
	for _, rel := range r.Thread.Relazioni {
		if attivi[rel.PadreID] && attivi[rel.FiglioID] {
			figli[rel.PadreID] = append(figli[rel.PadreID], rel.FiglioID)
		}
	}
	dentro[*prodotto] = true
	coda := []uuid.UUID{*prodotto}
	for len(coda) > 0 {
		x := coda[0]
		coda = coda[1:]
		for _, f := range figli[x] {
			if !dentro[f] {
				dentro[f] = true
				coda = append(coda, f)
			}
		}
	}
	return dentro
}

// nodoDellaSorgente: un nodo dei gruppi del prodotto, per tradurre il riferimento del nodo nella voce del registro.
type nodoDellaSorgente struct {
	voce, gruppo, codice, rev string
	allegato                  uuid.UUID
	identificativo            *propostabom.IdentificativoCliente
}

// azioniDellaRiga: le azioni valide per stato [T; U2 §2.3, «Le azioni dipendono dallo stato»].
func azioniDellaRiga(stato string, conBozza bool) AzioniRiga {
	no := func(m string) AzioneSorgente { return AzioneSorgente{Motivo: m} }
	si := AzioneSorgente{Disponibile: true}
	senza := no(MotivoAzioneSenzaBozza)
	out := AzioniRiga{SalvaNellaBozza: senza, TogliDallaBozza: senza, Sposta: senza, ScollegaAllaConferma: senza}
	switch stato {
	case StatoRigaProposto:
		out.TogliDallaBozza, out.ScollegaAllaConferma = no(MotivoAzioneNonNellaBozza), no(MotivoAzioneNonRegistrato)
		if conBozza {
			out.SalvaNellaBozza, out.Sposta = si, si
		}
	case StatoRigaInConflitto:
		out.SalvaNellaBozza, out.TogliDallaBozza, out.ScollegaAllaConferma = no(MotivoAzioneInConflitto), no(MotivoAzioneNonNellaBozza), no(MotivoAzioneNonRegistrato)
		if conBozza {
			out.Sposta = si
		}
	case StatoRigaRegistrato:
		out.SalvaNellaBozza, out.TogliDallaBozza = no(MotivoAzioneGiaRegistrato), no(MotivoAzioneNonNellaBozza)
		if conBozza {
			out.Sposta, out.ScollegaAllaConferma = si, si
		}
	case StatoRigaSalvato:
		out.SalvaNellaBozza, out.ScollegaAllaConferma = no(MotivoAzioneGiaNellaBozza), no(MotivoAzioneNonRegistrato)
		out.TogliDallaBozza, out.Sposta = si, si
	case StatoRigaDaScollegare:
		out.SalvaNellaBozza, out.Sposta, out.ScollegaAllaConferma = no(MotivoAzioneSvincolo), no(MotivoAzioneSvincolo), no(MotivoAzioneSvincolo)
		out.TogliDallaBozza = si
	case StatoRigaDaRicollocare:
		out.SalvaNellaBozza, out.TogliDallaBozza, out.ScollegaAllaConferma = no(MotivoAzioneDaRicollocare), no(MotivoAzioneNonNellaBozza), no(MotivoAzioneNonRegistrato)
		out.Sposta = si
	case StatoRigaLegatoComeSorgente: // direttiva 39 §5 [U]: in sola lettura, si cambia con S8
		x := no(MotivoAzioneSorgente)
		out = AzioniRiga{SalvaNellaBozza: x, TogliDallaBozza: x, Sposta: x, ScollegaAllaConferma: x}
	case StatoRigaLegameStorico:
		x := no(MotivoAzioneLegameStorico)
		out = AzioniRiga{SalvaNellaBozza: x, TogliDallaBozza: x, Sposta: x, ScollegaAllaConferma: x}
	}
	return out
}

// azioniDi: le azioni di una riga, per stato e per ciò che la riga è (direttiva 35, blocco C-4): una riga che sta nella
// bozza (origine bozza: una PB in conflitto o che conferma una AR) si può togliere dalla bozza; un documento confermato
// senza AR non si sposta né si scollega da qui (è del fascicolo, V3); una PB su un file registrato altrove dice il rimedio.
func azioniDi(r RigaSorgente, conBozza bool) AzioniRiga {
	out := azioniDellaRiga(r.Stato, conBozza)
	si := AzioneSorgente{Disponibile: true}
	nellaBozza := r.Origine == OrigineRigaBozza
	switch {
	case r.Stato == StatoRigaRegistrato && r.Motivo == MotivoRigaDocumentoConfermato:
		out.Sposta, out.ScollegaAllaConferma = AzioneSorgente{Motivo: MotivoAzioneDocumento}, AzioneSorgente{Motivo: MotivoAzioneDocumento}
		if nellaBozza {
			out.TogliDallaBozza = si
		}
	case r.Stato == StatoRigaRegistrato && nellaBozza:
		out.SalvaNellaBozza, out.TogliDallaBozza = AzioneSorgente{Motivo: MotivoAzioneGiaNellaBozza}, si
	case r.Stato == StatoRigaInConflitto && nellaBozza:
		out.TogliDallaBozza = si
		if r.Motivo == MotivoRigaGiaAssociataAltrove {
			out.SalvaNellaBozza = AzioneSorgente{Motivo: MotivoAzioneSvincoloAltrove}
		}
	}
	return out
}

// condivisi: per ogni riga, gli altri prodotti che hanno lo stesso file (una riga per prodotto, file e destinazione,
// nessun byte duplicato) e, per ognuno, il motivo: stesso_componente se una riga dell'altro prodotto porta il file allo
// stesso componente (il componente condiviso fra prodotti), file_pertinente altrimenti (il file condiviso).
func condivisi(s *Sorgente) {
	type rigaAltrove struct {
		prodotto   uuid.UUID
		componente *uuid.UUID
	}
	per := map[uuid.UUID][]uuid.UUID{}
	righe := map[uuid.UUID][]rigaAltrove{}
	for _, p := range s.Prodotti {
		if p.ProdottoID == nil {
			continue
		}
		visti := map[uuid.UUID]bool{}
		for _, r := range p.Righe {
			righe[r.AllegatoID] = append(righe[r.AllegatoID], rigaAltrove{prodotto: *p.ProdottoID, componente: r.componente})
			if !visti[r.AllegatoID] {
				visti[r.AllegatoID] = true
				per[r.AllegatoID] = append(per[r.AllegatoID], *p.ProdottoID)
			}
		}
	}
	for i := range s.Prodotti {
		p := &s.Prodotti[i]
		for j := range p.Righe {
			riga := &p.Righe[j]
			for _, altro := range per[riga.AllegatoID] {
				if p.ProdottoID != nil && altro == *p.ProdottoID {
					continue
				}
				motivo := CondivisioneFilePertinente
				for _, x := range righe[riga.AllegatoID] {
					if x.prodotto == altro && x.componente != nil && riga.componente != nil && *x.componente == *riga.componente {
						motivo = CondivisioneStessoComponente
					}
				}
				riga.CondivisoCon = append(riga.CondivisoCon, altro)
				riga.Condivisione = append(riga.Condivisione, CondivisioneRiga{ProdottoID: altro, Motivo: motivo})
			}
		}
	}
}

// firmaDellaSorgente: l'impronta della risposta senza la firma e senza l'ora del calcolo: cambia se e solo se cambia un
// campo che la pagina mostra (U2 §2.3, «Quando si rilegge»).
func firmaDellaSorgente(s *Sorgente) string {
	c := *s
	c.Firma, c.Calcolato = "", ""
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func contiene(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// copreTutte: il file è proposto in tutte le candidate compatibili (per chiave di gruppo).
func copreTutte(gruppi, compatibili map[string]bool) bool {
	for k := range compatibili {
		if !gruppi[k] {
			return false
		}
	}
	return true
}
