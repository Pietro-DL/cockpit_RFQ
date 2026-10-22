// Package sorgente: S8 «Conferma questo STEP come sorgente di ‹prodotto›» e la revoca della sorgente, nel core
// (direttiva 39 §2, §3 [U + T-39]; U2 §2.2 e EM §10.9 «S8» per la forma, superati dove la direttiva li supera).
//
// S8 conferma la fonte della BOZZA, mai della working: passi idempotenti, ognuno nella sua transazione, con la chiave
// d'intento del corpo e le chiavi dei passi derivate (UUID v5):
//   - passo 0, senza scrivere (Controlla, e di nuovo sotto il lucchetto nel passo 2): STEP cad_3d di un allegato della
//     RFQ, attivo, scaricato, con lo sha256 dichiarato; prodotto finito non archiviato; nessuna AR del file (lo STEP
//     sorgente non e' mai una AR: D-AR02 A, direttiva 39 §5); il caso (g) (lo stesso contenuto e' documento di un altro
//     finito); lo stato della bozza; poi, con il calcolo del motore fatto da chi chiama (Preparazione): V-7 sul registro,
//     il motivo (MotivoFonteObbligatorio; con solo_prodotto sempre, B-10), lo stato dell'analisi con la presa d'atto;
//   - passo 1: non c'e' piu' (il legame dello STEP sorgente non e' una AR e non passa dalla copia; risposta «non_serve»);
//   - passo 2 (k2): il documento confermato con confermadoc.Conferma (componente = prodotto, tipo cad_3d, scelta
//     «aggiungi»: mai la sostituzione del documento della working, direttiva 39 §2), la riga conferma_documento di
//     fonte_step_evento (ruolo prodotto, con il documento, k2, il gruppo e il motivo) e la copia sul NAS del solo STEP
//     sorgente (il lavoro copia_nas di oggi; eccezione D-AR13 A; mai il Conservatore). Un documento gia' presente del
//     prodotto con quel contenuto (anche storico, D-AR04 (iii)) riceve solo la riga di S8;
//   - passo 3 (k3): la bozza, con R2 del core: nessuna bozza aperta → CreaFormato3 partenza step (anche con la sola radice)
//     o solo_prodotto; con un'ultima conferma valida → da-conferma-1 (k3a) e poi, se la fonte e' un'altra, il cambio della
//     fonte (k3b), senza toccare la working; bozza aperta su un'altra sorgente STEP → CambiaSorgenteFormato3; bozza aperta
//     manuale (senza sorgente, o con la sorgente solo_prodotto) → la riconciliazione sulla stessa bozza: con la partenza
//     step CambiaSorgenteFormato3 (cambio-fonte-2, distintabozza/riconciliazione3.go), con la partenza solo_prodotto su
//     una bozza senza sorgente SoloProdottoSuBozzaFormato3; mai una seconda bozza attiva (direttiva 39 §3). Il corpo
//     dichiara la bozza (bozza_id, versione_attesa) come per il cambio della fonte: l'ingegnere ha visto l'effetto.
//
// S8 non scrive componente.step_strutturale_id, non marca sostituito_da, non chiude righe legacy, non crea AR, non tocca
// la working (lo fa solo R5).
//
// L'intento (gesto conferma_sorgente in esito_intento) si scrive nella transazione dell'ultimo passo (PrimaDelCommit
// delle forme di R2) quando la procedura e' completa; dopo un rifiuto definitivo di un passo, in una transazione propria,
// con esito «parziale» (lo stesso reinvio riceve «riproposto»); dopo un errore temporaneo (40P01, 55P03) non si scrive, e
// lo stesso reinvio riprende dal primo passo non fatto.
//
// Messaggio 46 [U] (21/10): il motivo scritto dall'utente si toglie da ogni widget, anche dalla conferma della sorgente.
// Dove il contratto lo voleva (il codice letto dalla radice dello STEP non e' quello del prodotto, la partenza dal solo
// prodotto, il codice letto dal file da correggere con quello del prodotto, la riassociazione da un altro prodotto) il
// corpo non lo chiede piu': la decisione e' il gesto dell'utente, e il motivo della riga di S8 e della bozza e' la traccia
// del gesto scritta dal server (TracciaS8: «gesto di <sigla> da «Conferma la sorgente»: …», con le evidenze del Motore A
// dove la candidatura e' univoca). Se un client di prima manda un motivo, vale il suo. Il reinvio dello stesso corpo con la
// stessa chiave riconosce la riga del passo 2 confrontando il motivo con la traccia esatta che il server genera con gli
// stessi dati (stessoMotivoDelPasso2), mai con un prefisso.
package sorgente

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/confermadoc"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/core/rfq/traccia"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I codici propri di S8 e della revoca (gli altri sono quelli di distintabozza: CORPO_NON_VALIDO, FUORI_RFQ,
// FILE_NON_ATTIVO, MOTIVO_OBBLIGATORIO, ANTEPRIMA_SUPERATA, CHIAVE_RIUSATA, BOZZA_VERSIONE, RFQ_INESISTENTE).
const (
	// 422: lo stesso contenuto e' gia' il documento (la sorgente) di un altro finito (caso (g), base A di V1-Q37).
	CodiceStepSorgenteDiAltroProdotto = "STEP_SORGENTE_DI_ALTRO_PRODOTTO"
	// 422: lo stesso contenuto e' un documento della RFQ non del prodotto (di un altro componente, senza componente, o
	// sostituito).
	CodiceGiaDocumento = "GIA_DOCUMENTO"
	// 422: il file non e' uno STEP.
	CodiceNonStep = "NON_STEP"
	// 409: la bozza aperta del prodotto non si puo' riallineare alla sorgente nuova con S8 (formato 2: prima si converte;
	// solo_prodotto su una bozza o una conferma con un'altra sorgente STEP) [T-39]. La bozza manuale si riconcilia
	// (direttiva 39 §3): il motivo bozza_manuale non si emette piu'.
	CodiceRiconciliazioneNonDisponibile = "RICONCILIAZIONE_NON_DISPONIBILE"
)

// I motivi di S8 e della revoca.
const (
	// MotivoBozzaManuale: non piu' emesso (la bozza manuale si riconcilia, direttiva 39 §3); resta per i lettori delle
	// risposte gia' date.
	MotivoBozzaManuale           = "bozza_manuale"
	MotivoBozzaDaConvertire      = "bozza_da_convertire"
	MotivoSoloProdottoSuBozza    = "solo_prodotto_su_bozza"
	MotivoSoloProdottoSuConferma = "solo_prodotto_su_conferma"
	MotivoGiaAssociata           = "gia_associata_altrove"
	MotivoPropostaAssente        = "proposta_assente"
	MotivoPropostaDecisa         = "proposta_decisa"
	MotivoShaDiverso             = "sha_diverso"
	MotivoPresaDAttoMancante     = "presa_d_atto_mancante"
	MotivoNessunaSorgente        = "nessuna_sorgente_da_revocare"
	MotivoProdottoNonAmmesso     = "prodotto_non_ammesso"
	MotivoConfermaRifiutata      = "conferma_rifiutata"
)

// Le partenze di S8 e gli stati dell'analisi dello STEP (direttiva 39 §3).
const (
	PartenzaStep         = distintabozza.PartenzaStep
	PartenzaSoloProdotto = distintabozza.PartenzaSoloProdotto

	AnalisiPronta      = "pronta"
	AnalisiInCorso     = "in_analisi"
	AnalisiErrore      = "errore"
	AnalisiNonValutato = "non_valutato"
)

// I passi e i loro esiti (U2 §2.2).
const (
	PassoAssociazione      = "associazione"
	PassoConfermaDocumento = "conferma_documento"
	PassoBozza             = "bozza"

	EsitoFatto      = "fatto"
	EsitoGiaFatto   = "gia_fatto"
	EsitoNonServe   = "non_serve"
	EsitoRifiutato  = "rifiutato"
	EsitoNonTentato = "non_tentato"

	EsitoCompletata = "completata"
	EsitoParziale   = "parziale"

	GestoConfermaSorgente = "conferma_sorgente"
)

// spazioChiavi: il namespace fisso delle chiavi dei passi (U2 §2.2: UUID v5 di (chiave d'intento, passo)).
var spazioChiavi = uuid.MustParse("6f1c2a4e-39d8-5b7a-9c1e-5a0d3e7b8c21")

// ChiavePasso: la chiave di un passo di S8 (UUID v5, deterministica: lo stesso reinvio da' le stesse chiavi).
func ChiavePasso(chiave uuid.UUID, passo string) uuid.UUID {
	return uuid.NewSHA1(spazioChiavi, []byte(chiave.String()+"|"+passo))
}

// BozzaS8: la bozza aperta che il corpo dichiara (caso (d)).
type BozzaS8 struct {
	BozzaID        uuid.UUID `json:"bozza_id"`
	VersioneAttesa int       `json:"versione_attesa"`
}

// RichiestaS8: il corpo di S8 [T-39]. Partenza step (predefinita): gruppo e impronta_contenuto dell'anteprima (V-7).
// Partenza solo_prodotto: stato_analisi mostrato e presa_d_atto vera (il motivo, dal messaggio 46 [U], e' la traccia del
// server se il corpo non lo porta). codice_atteso e
// componente_atteso sono facoltativi (il prodotto che la pagina mostrava). «sostituisci» ({conferma: true}) resta solo
// come presa d'atto, senza effetto sul documento: S8 non sostituisce mai (D-TAB-02 B rivista; R39-05 della corsia
// contratto, adottata [T-39]); niente campi per l'autorizzazione.
type RichiestaS8 struct {
	ChiaveIdempotenza uuid.UUID                `json:"chiave_idempotenza"`
	ProdottoID        uuid.UUID                `json:"prodotto_id"`
	Partenza          string                   `json:"partenza,omitempty"`
	Gruppo            string                   `json:"gruppo,omitempty"`
	ImprontaContenuto string                   `json:"impronta_contenuto,omitempty"`
	Sha256            string                   `json:"sha256,omitempty"`
	CodiceAtteso      string                   `json:"codice_atteso,omitempty"`
	ComponenteAtteso  *uuid.UUID               `json:"componente_atteso,omitempty"`
	Motivo            string                   `json:"motivo,omitempty"`
	StatoAnalisi      string                   `json:"stato_analisi,omitempty"`
	PresaDAtto        bool                     `json:"presa_d_atto,omitempty"`
	Bozza             *BozzaS8                 `json:"bozza,omitempty"`
	Scelte            []distintabozza.SceltaR2 `json:"scelte,omitempty"`
	Sostituisci       *PresaDAttoS8            `json:"sostituisci,omitempty"`
	// SpostaDa: il gesto «Sposta la sorgente su questo prodotto» (G39-03 [U]; forma [T-39], sposta.go): lo STEP e' legato
	// all'altro prodotto indicato (sorgente S8 non usata da una R5, documento o copia P1) e passa a questo, con la revoca
	// motivata e l'annullamento della bozza la'. Se lo STEP non e' di un altro prodotto il campo non ha effetto.
	SpostaDa *uuid.UUID `json:"sposta_da,omitempty"`
	// CorrezioneSorgente: la sorgente nuova corregge una sorgente sbagliata (G39-03 [U]: dopo R5 il cambio di sorgente e' una
	// revisione interna, non del cliente): il motivo della riga di S8 prende la causa sorgente_errata [T-39].
	CorrezioneSorgente bool `json:"correzione_sorgente,omitempty"`
	// traccia: il motivo scritto dal server quando il gesto ne vuole uno e il corpo non lo porta (messaggio 46 [U];
	// conTraccia); tracciaBase: la stessa traccia senza le ragioni, per gli eventi che vogliono sempre un motivo (il cambio
	// della fonte della bozza, motivoCambio). Non sono del corpo: non entrano nell'impronta.
	traccia, tracciaBase string
}

// motivoRegistrato: il motivo della riga di S8 e della bozza (quello dell'utente o la traccia del server); con la
// correzione della sorgente dichiarata, con la causa sorgente_errata in testa (confermabom.MotivoConCausa). Il gesto «sposta
// la sorgente» (sposta_da) non la scrive qui: la causa sta sulla revoca dell'altro prodotto (notaSpostamento), e per il
// prodotto che riceve lo STEP la conferma che segue non e' una correzione della sua BOM
// (confermabom.StatoSorgente.Correzione).
func (r RichiestaS8) motivoRegistrato() string {
	m := r.motivoEffettivo()
	if r.CorrezioneSorgente {
		return confermabom.MotivoConCausa(confermabom.CausaSorgenteErrata, m)
	}
	return m
}

// motivoEffettivo: il motivo dell'utente o la traccia, senza la causa ("" se il gesto non ne ha bisogno e il corpo non lo
// porta).
func (r RichiestaS8) motivoEffettivo() string {
	return traccia.DellUtente(r.Motivo, r.traccia)
}

// PresaDAttoS8: «sostituisci» del corpo, la presa d'atto che il prodotto ha gia' un altro STEP confermato; non cambia
// niente nel passo 2 (il documento di prima resta corrente: lo sostituisce R5).
type PresaDAttoS8 struct {
	Conferma bool `json:"conferma"`
}

func errCorpo(campo, motivo, formato string, a ...any) error {
	return &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: campo, Motivo: motivo, Messaggio: fmt.Sprintf(formato, a...)}
}

func errCodice(codice, campo, formato string, a ...any) error {
	return &distintabozza.Errore{Codice: codice, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

func reSha(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// LeggiRichiestaS8 legge il corpo in modo stretto (campi sconosciuti, nomi non minuscoli, qualcosa dopo il valore: 422)
// e restituisce la richiesta con l'impronta della sua forma canonica.
func LeggiRichiestaS8(raw []byte) (RichiestaS8, string, error) {
	var r RichiestaS8
	can, err := jsoncanonico.Codifica(json.RawMessage(raw))
	if err != nil {
		return r, "", errCorpo("richiesta", "", "il JSON non si legge: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return r, "", errCorpo("richiesta", "", "il JSON non ha la forma attesa: %v", err)
	}
	if dec.More() {
		return r, "", errCorpo("richiesta", "", "dopo il JSON c'e' altro")
	}
	if r.ChiaveIdempotenza.Version() != 4 {
		return r, "", errCorpo("chiave_idempotenza", "", "manca la chiave d'intento, un UUID v4")
	}
	if r.ProdottoID == uuid.Nil {
		return r, "", errCorpo("prodotto_id", "", "manca il prodotto")
	}
	if r.Partenza == "" {
		r.Partenza = PartenzaStep
	}
	r.Motivo = strings.TrimSpace(r.Motivo)
	if n := len([]rune(r.Motivo)); n > 500 {
		return r, "", errCorpo("motivo", "", "il motivo sta in 500 caratteri")
	}
	if r.Sha256 != "" && !reSha(r.Sha256) {
		return r, "", errCorpo("sha256", "", "lo sha256 e' di 64 cifre esadecimali minuscole")
	}
	switch r.Partenza {
	case PartenzaStep:
		if strings.TrimSpace(r.Gruppo) == "" || len(r.Gruppo) > 300 {
			return r, "", errCorpo("gruppo", "", "la partenza step vuole il gruppo dell'anteprima")
		}
		if r.Gruppo == propostabom.GruppoSenzaSTEP || r.Gruppo == distintabozza.GruppoSoloProdotto {
			return r, "", errCorpo("gruppo", distintabozza.MotivoGruppoNonImportabile, "il gruppo «%s» non si importa: per uno STEP senza struttura la partenza e' solo_prodotto", r.Gruppo)
		}
		if !reSha(r.ImprontaContenuto) {
			return r, "", errCorpo("impronta_contenuto", "", "la partenza step vuole l'impronta del contenuto dell'anteprima (64 cifre esadecimali)")
		}
		if r.StatoAnalisi != "" || r.PresaDAtto {
			return r, "", errCorpo("stato_analisi", "", "stato_analisi e presa_d_atto sono della partenza solo_prodotto")
		}
	case PartenzaSoloProdotto:
		if r.Gruppo != "" || r.ImprontaContenuto != "" || len(r.Scelte) > 0 {
			return r, "", errCorpo("gruppo", "", "la partenza solo_prodotto non ha gruppo, impronta_contenuto ne' scelte: non c'e' una struttura da importare")
		}
		switch r.StatoAnalisi {
		case AnalisiPronta, AnalisiInCorso, AnalisiErrore, AnalisiNonValutato:
		default:
			return r, "", errCorpo("stato_analisi", "", "stato_analisi e' quello mostrato: pronta, in_analisi, errore o non_valutato")
		}
		if !r.PresaDAtto {
			return r, "", errCorpo("presa_d_atto", MotivoPresaDAttoMancante,
				"senza struttura importabile la bozza parte dal solo prodotto: serve la presa d'atto esplicita dello stato dell'analisi")
		}
	default:
		return r, "", errCorpo("partenza", "", "la partenza e' step oppure solo_prodotto")
	}
	if r.Sostituisci != nil && !r.Sostituisci.Conferma {
		return r, "", errCorpo("sostituisci", "", "sostituisci, se c'e', e' {conferma: true}: una presa d'atto, S8 non sostituisce il documento")
	}
	if r.Bozza != nil && (r.Bozza.BozzaID == uuid.Nil || r.Bozza.VersioneAttesa < 1) {
		return r, "", errCorpo("bozza", "", "bozza vuole bozza_id e versione_attesa (almeno 1)")
	}
	if r.SpostaDa != nil && (*r.SpostaDa == uuid.Nil || *r.SpostaDa == r.ProdottoID) {
		return r, "", errCorpo("sposta_da", "", "sposta_da e' l'altro prodotto da cui la sorgente passa a questo")
	}
	if len(r.Scelte) > 0 && r.Bozza == nil {
		return r, "", errCorpo("scelte", "", "le scelte recepisci/mantieni servono solo al cambio della fonte di una bozza aperta")
	}
	for i, s := range r.Scelte {
		if s.Scelta != distintabozza.SceltaRecepisci && s.Scelta != distintabozza.SceltaMantieni {
			return r, "", errCorpo(fmt.Sprintf("scelte[%d].scelta", i), "", "nel cambio della fonte le scelte sono recepisci o mantieni")
		}
	}
	return r, jsoncanonico.Impronta(can), nil
}

// gruppoFonte: il gruppo della riga della storia della fonte (quello dell'anteprima, o il riservato solo_prodotto).
func (r RichiestaS8) gruppoFonte() string {
	if r.Partenza == PartenzaSoloProdotto {
		return distintabozza.GruppoSoloProdotto
	}
	return r.Gruppo
}

// Preparazione: cio' che chi chiama ha calcolato prima della transazione con il motore (sola lettura, semaforo): il
// registro del gruppo dichiarato gia' confrontato con l'impronta (partenza step), lo stato dell'analisi dello STEP e se
// una struttura importabile c'e' (partenza solo_prodotto).
type Preparazione struct {
	Proposta     *distintabozza.PropostaCalcolata
	StatoAnalisi string
	Importabile  bool
}

// PassoS8, BozzaS8Esito, EsitoS8: la risposta 200 (U2 §2.2).
type PassoS8 struct {
	Passo  string `json:"passo"`
	Esito  string `json:"esito"`
	Codice string `json:"codice,omitempty"`
	Testo  string `json:"testo"`
}

type BozzaS8Esito struct {
	BozzaID      uuid.UUID  `json:"bozza_id"`
	Versione     int        `json:"versione"`
	Importazione *uuid.UUID `json:"importazione"`
}

type EsitoS8 struct {
	Esito             string        `json:"esito"`
	Riproposto        bool          `json:"riproposto"`
	Ritentabile       bool          `json:"ritentabile,omitempty"`
	ChiaveIdempotenza uuid.UUID     `json:"chiave_idempotenza"`
	Passi             []PassoS8     `json:"passi"`
	Bozza             *BozzaS8Esito `json:"bozza,omitempty"`
	CopiaNas          string        `json:"copia_nas"`
	// Spostamento: che cosa il passo 2 ha fatto sull'altro prodotto (G39-03; sposta.go)
	Spostamento *EsitoSpostamento `json:"spostamento,omitempty"`
	// Sostituzione: la bozza precedente, nata da un'altra sorgente S8 prima di R5, sostituita dal passo 3 (G39-03;
	// sostituzione.go)
	Sostituzione *EsitoSostituzione `json:"sostituzione,omitempty"`
	Testo        string             `json:"testo"`
}

// Contesto: cio' che il passo 0 ha letto.
type Contesto struct {
	Prodotto db.GetProdottoDellaRfqRow
	Sha256   string
	// NomeFile: il nome dello STEP, com'e' arrivato (per la traccia del gesto, messaggio 46 [U])
	NomeFile string
	// Documento: il documento del prodotto con lo stesso contenuto, se c'e' gia'.
	Documento *db.Documento
	// Aperta: la bozza aperta del prodotto; Propria: e' quella nata dal passo 3 di questa stessa chiave (la ripresa).
	Aperta  *db.DistintaBozza
	Propria bool
	// StessaSorgente: la bozza aperta e' gia' su questa sorgente (sha256 e gruppo).
	StessaSorgente bool
	// Manuale: la bozza aperta non ha una sorgente (la partenza vuota, o da-conferma-1 di una conferma senza fonte)
	Manuale bool
	// ConfermaValida: il prodotto ha un'ultima conferma valida (la partenza da-conferma-1).
	ConfermaValida bool
	// Spostamento: lo STEP e' di un altro componente e passa a questo nel passo 2 (sposta.go)
	Spostamento *Spostamento
	// Sostituzione: la bozza aperta e' nata da un'altra sorgente S8 prima di R5 e si sostituisce (sostituzione.go)
	Sostituzione *Sostituzione
}

// IntentoRegistrato: l'esito gia' scritto di una chiave d'intento di S8 nella RFQ (con la stessa impronta: riproposto);
// con un'altra impronta, o di un altro gesto, 409 CHIAVE_RIUSATA; nil se non c'e'.
func IntentoRegistrato(ctx context.Context, q *db.Queries, thread, chiave uuid.UUID, impronta, gesto string) (json.RawMessage, error) {
	ei, err := q.GetEsitoIntentoPerChiave(ctx, chiave)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if ei.ThreadID != thread {
		return nil, errCodice(distintabozza.CodiceFuoriRFQ, "chiave_idempotenza", "la chiave %s e' di un'altra RFQ", chiave)
	}
	if ei.Gesto != gesto || strings.TrimSpace(ei.ImprontaCorpo) != impronta {
		return nil, errCodice(distintabozza.CodiceChiaveRiusata, "chiave_idempotenza", "questa chiave d'intento ha gia' registrato un altro corpo: per riprovare serve una chiave nuova")
	}
	return ei.Esito, nil
}

// Controlla: il passo 0 senza scrivere (commento di testa), nella transazione di chi chiama; lo stesso controllo sotto
// il lucchetto nel passo 2.
func Controlla(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID, r RichiestaS8) (Contesto, error) {
	var out Contesto
	ar, err := q.GetAllegatoDellaRfq(ctx, db.GetAllegatoDellaRfqParams{AllegatoID: allegato, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errCodice(distintabozza.CodiceFuoriRFQ, "allegato", "il file non e' di questa RFQ")
	}
	if err != nil {
		return out, err
	}
	a, err := q.GetAllegato(ctx, allegato)
	if err != nil {
		return out, err
	}
	out.NomeFile = a.NomeFile
	if !confermadoc.FileStep(a.Estensione.String, a.NomeFile) {
		return out, errCodice(CodiceNonStep, "allegato", "%s non e' uno STEP: la sorgente strutturale del prodotto e' uno STEP (.stp, .step)", a.NomeFile)
	}
	if !ar.Sha256.Valid || strings.TrimSpace(ar.Sha256.String) == "" || !a.PathStaging.Valid {
		return out, errCorpo("allegato", "", "lo STEP non e' ancora scaricato")
	}
	out.Sha256 = strings.TrimSpace(ar.Sha256.String)
	if r.Sha256 != "" && r.Sha256 != out.Sha256 {
		return out, errCorpo("sha256", MotivoShaDiverso, "lo sha256 dichiarato non e' quello dello STEP")
	}
	if st, err := q.StatoOccorrenzaAllegato(ctx, allegato); err == nil {
		if st.Stato != filerfq.StatoAttivo || st.ScartatoLegacy {
			return out, errCodice(distintabozza.CodiceFileNonAttivo, "allegato", "lo STEP non e' un file attivo della RFQ (%s)", st.Stato)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	p, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: thread, ComponenteID: r.ProdottoID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errCodice(distintabozza.CodiceFuoriRFQ, "prodotto_id", "il prodotto non e' di questa RFQ")
	}
	if err != nil {
		return out, err
	}
	if p.Tipo != db.TipoComponenteFinito || p.ArchiviatoIl != nil {
		return out, errCorpo("prodotto_id", MotivoProdottoNonAmmesso, "%s non e' un prodotto finito attivo: la sorgente STEP e' di un finito non archiviato", p.Codice)
	}
	out.Prodotto = p
	if r.CodiceAtteso != "" && !strings.EqualFold(strings.TrimSpace(r.CodiceAtteso), strings.TrimSpace(p.Codice)) {
		return out, &distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Campo: "codice_atteso", Anteprima: "prodotto",
			Messaggio: "il codice del prodotto e' cambiato dopo l'anteprima: rileggi la pagina"}
	}
	if r.ComponenteAtteso != nil && *r.ComponenteAtteso != r.ProdottoID {
		return out, errCorpo("componente_atteso", "", "componente_atteso e' il prodotto della richiesta")
	}
	// il contenuto e' gia' un documento della RFQ?
	if d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{ThreadID: thread, Sha256: out.Sha256}); err == nil {
		switch {
		case d.SostituitoDa.Valid:
			return out, rifiutoSostituito(ctx, q, d, p)
		case d.ComponenteID.Valid && d.ComponenteID.UUID == r.ProdottoID:
			dd := d
			out.Documento = &dd
		case d.ComponenteID.Valid:
			c, err := q.GetComponente(ctx, d.ComponenteID.UUID)
			if err != nil {
				return out, err
			}
			if c.Tipo != db.TipoComponenteFinito {
				return out, errCodice(CodiceGiaDocumento, "allegato", "questo STEP e' gia' un documento di %s nel Fascicolo: non diventa la sorgente di %s", c.Codice, p.Codice)
			}
			// il caso (g): lo STEP e' un documento di un altro finito: si sposta, si riassocia con il motivo o si rifiuta
			// (sposta.go, G39-03 [U])
			sp, err := valutaAltroProdotto(ctx, q, thread, r, p, d, c, out.Sha256)
			if err != nil {
				return out, err
			}
			out.Spostamento = sp
		default:
			return out, errCodice(CodiceGiaDocumento, "allegato", "questo STEP e' gia' un documento del Fascicolo senza componente: si assegna dal Fascicolo, non con S8")
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	} else {
		// nessun documento: la copia P1 su un altro componente si riassocia con il motivo (sposta.go, G39-03 [U])
		sp, err := valutaCopiaP1(ctx, q, allegato, r, p)
		if err != nil {
			return out, err
		}
		out.Spostamento = sp
	}
	// lo STEP sorgente non e' mai una AR (D-AR02 A; direttiva 39 §5)
	if x, err := associazioni.Attiva(ctx, q, thread, allegato); err != nil {
		return out, err
	} else if x != nil {
		return out, errCorpo("allegato", MotivoGiaAssociata, "lo STEP ha un'associazione registrata come documento: lo STEP sorgente del finito non e' un documento associato")
	}
	// la bozza
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: r.ProdottoID})
	switch {
	case err == nil:
		out.Aperta = &b
	case !errors.Is(err, pgx.ErrNoRows):
		return out, err
	}
	if out.Aperta != nil {
		if v, err := q.GetVersionePerChiaveGlobale(ctx, uuid.NullUUID{UUID: ChiavePasso(r.ChiaveIdempotenza, "3a"), Valid: true}); err == nil && v.BozzaID == b.BozzaID {
			out.Propria = true
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		out.StessaSorgente = b.FonteSha256.Valid && strings.TrimSpace(b.FonteSha256.String) == out.Sha256 && b.FonteGruppo.String == r.gruppoFonte()
		out.Manuale = !b.FonteAllegatoID.Valid
		if !out.Propria && !out.StessaSorgente {
			switch {
			case b.Formato != distintabozza.FormatoSnapshot3:
				return out, &distintabozza.Errore{Codice: CodiceRiconciliazioneNonDisponibile, Motivo: MotivoBozzaDaConvertire, Campo: "bozza",
					Messaggio: "la bozza aperta del prodotto e' nel formato 2: prima si converte"}
			case r.Partenza == PartenzaSoloProdotto && !out.Manuale:
				return out, &distintabozza.Errore{Codice: CodiceRiconciliazioneNonDisponibile, Motivo: MotivoSoloProdottoSuBozza, Campo: "partenza",
					Messaggio: "la bozza aperta ha un'altra sorgente STEP: il passaggio a uno STEP senza struttura non e' disponibile"}
			case r.Bozza == nil && out.Manuale:
				return out, &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Motivo: distintabozza.MotivoAltraBozzaAperta, Campo: "bozza",
					Messaggio: fmt.Sprintf("il prodotto ha una bozza manuale aperta (versione %d): lo STEP la riconcilia, e il corpo la dichiara", b.Versione),
					Conflitto: &distintabozza.Conflitto{BozzaID: b.BozzaID, Stato: b.Stato, Versione: int(b.Versione), AggiornataDa: b.AggiornataDa, AggiornataIl: b.AggiornataIl}}
			case r.Bozza == nil:
				return out, &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Motivo: distintabozza.MotivoAltraBozzaAperta, Campo: "bozza",
					Messaggio: fmt.Sprintf("il prodotto ha una bozza aperta su un'altra sorgente (versione %d): il corpo la dichiara per il cambio della fonte", b.Versione),
					Conflitto: &distintabozza.Conflitto{BozzaID: b.BozzaID, Stato: b.Stato, Versione: int(b.Versione), AggiornataDa: b.AggiornataDa, AggiornataIl: b.AggiornataIl}}
			case r.Bozza.BozzaID != b.BozzaID || r.Bozza.VersioneAttesa != int(b.Versione):
				return out, &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Motivo: distintabozza.MotivoVersioneDiversa, Campo: "bozza",
					Messaggio: fmt.Sprintf("la bozza aperta del prodotto e' alla versione %d: rileggila", b.Versione),
					Conflitto: &distintabozza.Conflitto{BozzaID: b.BozzaID, Stato: b.Stato, Versione: int(b.Versione), AggiornataDa: b.AggiornataDa, AggiornataIl: b.AggiornataIl}}
			}
			// la bozza nata da un'altra sorgente S8 prima di R5 si sostituisce, con la presa d'atto (sostituzione.go)
			if !out.Manuale {
				if out.Sostituzione, err = valutaSostituzione(ctx, q, thread, r, b, out.Sha256); err != nil {
					return out, err
				}
				if out.Sostituzione != nil {
					if err := controllaSostituzione(r, p, out.Sostituzione); err != nil {
						return out, err
					}
				}
			}
		}
		return out, nil
	}
	if r.Bozza != nil {
		return out, &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Motivo: distintabozza.MotivoBozzaChiusa, Campo: "bozza",
			Messaggio: "la bozza dichiarata non e' piu' aperta: rileggi la pagina"}
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return out, err
	}
	if _, v, ok, err := st.ValiditaUltima(r.ProdottoID); err != nil {
		return out, err
	} else if ok && v.Valida {
		out.ConfermaValida = true
		if r.Partenza == PartenzaSoloProdotto {
			return out, &distintabozza.Errore{Codice: CodiceRiconciliazioneNonDisponibile, Motivo: MotivoSoloProdottoSuConferma, Campo: "partenza",
				Messaggio: "il prodotto ha una conferma valida: partire dal solo prodotto con uno STEP senza struttura non e' ancora disponibile"}
		}
	}
	return out, nil
}

// controllaPreparazione: i controlli del passo 0 che vogliono il calcolo del motore (commento di testa).
func controllaPreparazione(allegato uuid.UUID, r RichiestaS8, c Contesto, p Preparazione) error {
	if r.Partenza == PartenzaStep {
		if p.Proposta == nil {
			return errCorpo("richiesta", "", "la partenza step vuole la proposta calcolata dal server")
		}
		reg := p.Proposta.Registro
		f := reg.Contenuto.Proposta.Fonte
		if f.AllegatoID != allegato.String() {
			a, _ := uuid.Parse(f.AllegatoID)
			return &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Motivo: distintabozza.MotivoGruppoSuAltraOccorrenza, Campo: "gruppo", AllegatoID: &a,
				Messaggio: "il gruppo calcolato sta su un'altra occorrenza dello STEP"}
		}
		if f.Sha256 != c.Sha256 {
			return errCorpo("sha256", MotivoShaDiverso, "il registro calcolato non e' di questo contenuto")
		}
		if err := distintabozza.VerificaImprontaDichiarata("impronta_contenuto", reg, r.ImprontaContenuto); err != nil {
			return err
		}
		// il codice della radice diverso non chiede piu' il motivo all'utente (messaggio 46 [U]): lo dice la traccia
		// (conTraccia); il controllo resta per un chiamante che non l'ha preparata
		if r.motivoEffettivo() == "" && propostabom.MotivoFonteObbligatorio(reg.Contenuto, c.Prodotto.Codice) {
			return &distintabozza.Errore{Codice: distintabozza.CodiceMotivoObbligatorio, Campo: "motivo",
				Messaggio: "il codice letto dalla radice dello STEP non coincide con quello del prodotto (" + propostabom.ConfrontoDelCodice(reg.Contenuto, c.Prodotto.Codice) +
					"): manca la traccia del gesto"}
		}
		return nil
	}
	if p.Importabile {
		return &distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Campo: "stato_analisi", Anteprima: "stato_analisi",
			Messaggio: "adesso lo STEP ha una struttura importabile: rileggi l'anteprima e conferma con la partenza step"}
	}
	if p.StatoAnalisi != r.StatoAnalisi {
		return &distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Campo: "stato_analisi", Anteprima: "stato_analisi",
			Messaggio: fmt.Sprintf("lo stato dell'analisi dello STEP e' cambiato (%s, non %s): rileggi e prendi atto dello stato di adesso", p.StatoAnalisi, r.StatoAnalisi)}
	}
	if r.motivoEffettivo() == "" {
		return &distintabozza.Errore{Codice: distintabozza.CodiceMotivoObbligatorio, Campo: "motivo",
			Messaggio: "senza struttura importabile il codice della radice non e' determinabile: manca la traccia del gesto"}
	}
	return nil
}

// ConfermaS8: la procedura (commento di testa). pool apre le transazioni dei passi; prep viene dal calcolo del motore
// fatto da chi chiama dopo Controlla. Gli errori del passo 0 tornano come errori (niente scritto); da li' in poi la
// risposta e' un EsitoS8 (completata o parziale), salvo un guasto (5xx da verificare).
func ConfermaS8(ctx context.Context, pool distintabozza.Avviatore, thread, allegato, utente uuid.UUID, r RichiestaS8, impronta string, prep Preparazione) (EsitoS8, error) {
	out := EsitoS8{ChiaveIdempotenza: r.ChiaveIdempotenza, CopiaNas: confermadoc.CopiaNonPrevista,
		Passi: []PassoS8{{Passo: PassoAssociazione, Esito: EsitoNonServe,
			Testo: "Il legame dello STEP sorgente non è un'associazione: lo registra la conferma della sorgente."}}}
	// passo 0 con il motore (di nuovo il contesto, in sola lettura, per i controlli che vogliono la proposta)
	c, err := contestoInLettura(ctx, pool, thread, allegato, r)
	if err != nil {
		return out, err
	}
	// la traccia del gesto al posto del motivo dell'utente (messaggio 46 [U]), con gli stessi dati a ogni reinvio
	if r, err = conTraccia(ctx, pool, allegato, utente, r, c, prep); err != nil {
		return out, err
	}
	if err := controllaPreparazione(allegato, r, c, prep); err != nil {
		return out, err
	}
	// passo 2
	esito2, copia, spostato, err := passo2(ctx, pool, thread, allegato, utente, r)
	if err != nil {
		return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoConfermaDocumento, err)
	}
	out.CopiaNas = copia
	out.Spostamento = spostato
	out.Passi = append(out.Passi, PassoS8{Passo: PassoConfermaDocumento, Esito: esito2, Testo: testoPasso2(esito2)})
	// passo 3
	return passo3(ctx, pool, thread, allegato, utente, r, impronta, prep, out)
}

func testoPasso2(esito string) string {
	if esito == EsitoGiaFatto {
		return "Lo STEP era già confermato come sorgente di questo prodotto."
	}
	return "Lo STEP è confermato come sorgente della bozza del prodotto (la working non cambia)."
}

// contestoInLettura: Controlla in una transazione di sola lettura.
func contestoInLettura(ctx context.Context, pool distintabozza.Avviatore, thread, allegato uuid.UUID, r RichiestaS8) (Contesto, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Contesto{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sola lettura
	return Controlla(ctx, db.New(tx), thread, allegato, r)
}

// transitorio: un errore temporaneo di PostgreSQL (stallo, lucchetto non preso in tempo): il passo si ritenta con la
// stessa chiave.
func transitorio(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (pg.Code == "40P01" || pg.Code == "55P03" || pg.Code == "40001")
}

// definitivo: un rifiuto di un passo (distintabozza.Errore); il resto e' un guasto.
func definitivo(err error) (*distintabozza.Errore, bool) {
	var e *distintabozza.Errore
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// chiudiParziale: un passo non riuscito. Temporaneo: parziale ritentabile, nessun intento. Rifiuto definitivo: l'intento
// parziale in una transazione propria. Altro: l'errore (5xx).
func chiudiParziale(ctx context.Context, pool distintabozza.Avviatore, thread, utente uuid.UUID, r RichiestaS8, impronta string, out EsitoS8, passo string, err error) (EsitoS8, error) {
	codice := ""
	switch e, ok := definitivo(err); {
	case transitorio(err):
		out.Ritentabile = true
		codice = "RIPROVA"
	case ok:
		codice = e.Codice
	default:
		return out, err
	}
	testo := err.Error()
	out.Passi = append(out.Passi, PassoS8{Passo: passo, Esito: EsitoRifiutato, Codice: codice, Testo: testo})
	if passo == PassoConfermaDocumento {
		out.Passi = append(out.Passi, PassoS8{Passo: PassoBozza, Esito: EsitoNonTentato, Testo: "La bozza non è stata creata."})
	}
	out.Esito = EsitoParziale
	out.Testo = testoParziale(out)
	if out.Ritentabile {
		return out, nil
	}
	return scriviIntentoSeparato(ctx, pool, thread, utente, r, impronta, out)
}

func testoParziale(out EsitoS8) string {
	var fatti, no, manca []string
	for _, p := range out.Passi {
		switch p.Esito {
		case EsitoFatto, EsitoGiaFatto:
			fatti = append(fatti, p.Passo)
		case EsitoRifiutato:
			no = append(no, p.Passo)
		case EsitoNonTentato:
			manca = append(manca, p.Passo)
		}
	}
	t := "Riuscito: " + strings.Join(vuoto(fatti), ", ") + ". Non riuscito: " + strings.Join(vuoto(no), ", ") + ". Manca: " + strings.Join(vuoto(manca), ", ") + "."
	if out.Ritentabile {
		return t + " Riprova: il server riprende dal primo passo non fatto."
	}
	return t + " Per riprovare rileggi l'anteprima: serve una richiesta nuova."
}

func vuoto(xx []string) []string {
	if len(xx) == 0 {
		return []string{"niente"}
	}
	return xx
}

// candidaturaDelMotore: lo STEP e' il candidato univoco del prodotto per il Motore A (candidatoDelMotore), con le evidenze
// della candidatura.
type candidaturaDelMotore struct {
	univoca  bool
	evidenze []propostabom.EvidenzaCandidatura
}

// candidatoDelMotore (G39-02 [U]; scelta [T-39 integrazione], ristretta nel giro G39-bis): lo STEP e' il candidato
// sorgente UNIVOCO di questo prodotto nello stesso calcolo del Motore A che prepara S8
// (distintabozza.StepCandidati.UnivocaDel: la candidatura che la pagina propone in S9). Dal messaggio 46 [U] non decide
// piu' se serve il motivo (il motivo dell'utente non si chiede mai): le sue evidenze entrano nella traccia del gesto quando
// il codice letto dal file si corregge con quello del prodotto (TracciaS8).
func candidatoDelMotore(prep Preparazione, allegato, prodotto uuid.UUID) candidaturaDelMotore {
	if prep.Proposta == nil || !prep.Proposta.StepCandidati.UnivocaDel(allegato, prodotto) {
		return candidaturaDelMotore{}
	}
	return candidaturaDelMotore{univoca: true, evidenze: prep.Proposta.StepCandidati.EvidenzeUnivocaDel(allegato, prodotto)}
}

// MaxTracciaS8: la traccia di S8 sta nei 500 caratteri del motivo dell'utente di prima.
const MaxTracciaS8 = 500

// WidgetConfermaSorgente: il gesto della pagina che la traccia di S8 nomina.
const WidgetConfermaSorgente = "Conferma la sorgente"

// RagioniS8: i dati che fanno della conferma della sorgente un gesto da tracciare (messaggio 46 [U]), letti dal corpo e
// da dati che il gesto non cambia (il codice del prodotto, il nome del file, il codice letto dalla proposta del file, il
// registro della proposta), cosi' la stessa richiesta da' la stessa traccia anche dopo il passo 2.
type RagioniS8 struct {
	Sigla, NomeFile, CodiceProdotto string
	// SoloProdotto: la partenza solo_prodotto, con lo stato dell'analisi mostrato (quello del corpo)
	SoloProdotto bool
	StatoAnalisi string
	// Radice: il confronto del codice della radice dello STEP con il prodotto, se non coincide (diverso,
	// non_determinabile), con il codice letto dalla radice
	Radice, CodiceRadice string
	// Letto: il codice letto dal file (documento_proposta.codice), se non e' quello del prodotto: il documento prende il
	// codice del prodotto
	Letto string
	// Univoca, Evidenze: la candidatura univoca del Motore A, con le sue evidenze, se c'e'
	Univoca  bool
	Evidenze []propostabom.EvidenzaCandidatura
	// LasciaA: il codice dell'altro prodotto da cui lo STEP passa a questo (sposta_da del corpo)
	LasciaA string
}

// Serve: almeno una ragione vuole la traccia nel motivo della riga di S8.
func (g RagioniS8) Serve() bool {
	return g.SoloProdotto || g.Radice != "" || g.Letto != "" || g.LasciaA != ""
}

// TracciaS8: la traccia del gesto, per esempio «gesto di PS da «Conferma la sorgente»: lo STEP «ACME-1.stp» è la sorgente
// di 9120001A; codice letto dal file «9120001», il documento prende quello del prodotto; candidatura univoca dello STEP del
// finito; evidenze: nome_file 9120001 (base uguale, marcatore compatibile)». Senza ragioni (Serve falso) e' la traccia
// base, che dice solo il gesto.
func TracciaS8(g RagioniS8) string {
	parti := []string{"lo STEP «" + strings.TrimSpace(g.NomeFile) + "» è la sorgente di " + strings.TrimSpace(g.CodiceProdotto)}
	if g.SoloProdotto {
		parti = append(parti, "partenza dal solo prodotto (analisi: "+strings.TrimSpace(g.StatoAnalisi)+")")
	}
	if g.Radice != "" {
		t := "codice della radice dello STEP " + strings.ReplaceAll(g.Radice, "_", " ")
		if c := strings.TrimSpace(g.CodiceRadice); c != "" {
			t += " («" + c + "»)"
		}
		parti = append(parti, t)
	}
	if g.Letto != "" {
		parti = append(parti, "codice letto dal file «"+strings.TrimSpace(g.Letto)+"», il documento prende quello del prodotto")
	}
	if g.LasciaA != "" {
		parti = append(parti, "lo STEP lascia "+strings.TrimSpace(g.LasciaA))
	}
	var ev []string
	if g.Letto != "" && g.Univoca {
		parti = append(parti, "candidatura univoca dello STEP del finito")
		for _, e := range g.Evidenze {
			var esiti []string
			if e.EsitoBase != "" {
				esiti = append(esiti, "base "+e.EsitoBase)
			}
			if e.EsitoMarcatore != "" {
				esiti = append(esiti, "marcatore "+e.EsitoMarcatore)
			}
			x := strings.TrimSpace(strings.TrimSpace(e.Selettore) + " " + strings.TrimSpace(e.CodiceLetto))
			if len(esiti) > 0 {
				x = strings.TrimSpace(x + " (" + strings.Join(esiti, ", ") + ")")
			}
			ev = append(ev, x)
		}
	}
	return traccia.Gesto(MaxTracciaS8, g.Sigla, WidgetConfermaSorgente, strings.Join(parti, "; "), ev...)
}

// codiceDellaRadice: il codice proposto della voce della radice del registro ("" se non c'e').
func codiceDellaRadice(c importazione.Contenuto) string {
	for _, v := range c.Proposta.Nodi {
		if c.EVoceDellaRadice(v) {
			return v.CodiceProposto
		}
	}
	return ""
}

// ragioniS8: le ragioni della traccia (RagioniS8), lette in una transazione di sola lettura.
func ragioniS8(ctx context.Context, pool distintabozza.Avviatore, allegato, utente uuid.UUID, r RichiestaS8, c Contesto, prep Preparazione) (RagioniS8, error) {
	g := RagioniS8{NomeFile: c.NomeFile, CodiceProdotto: c.Prodotto.Codice}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return g, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sola lettura
	q := db.New(tx)
	if g.Sigla, err = traccia.Sigla(ctx, q, utente); err != nil {
		return g, err
	}
	if r.Partenza == PartenzaSoloProdotto {
		g.SoloProdotto, g.StatoAnalisi = true, r.StatoAnalisi
	} else if prep.Proposta != nil {
		reg := prep.Proposta.Registro
		if propostabom.MotivoFonteObbligatorio(reg.Contenuto, c.Prodotto.Codice) {
			g.Radice = propostabom.ConfrontoDelCodice(reg.Contenuto, c.Prodotto.Codice)
			g.CodiceRadice = codiceDellaRadice(reg.Contenuto)
		}
	}
	pr, err := q.GetPropostaDocumentoDiAllegato(ctx, allegato)
	switch {
	case err == nil:
		if l := strings.TrimSpace(pr.Codice.String); l != "" && !strings.EqualFold(l, strings.TrimSpace(c.Prodotto.Codice)) {
			g.Letto = l
			m := candidatoDelMotore(prep, allegato, r.ProdottoID)
			g.Univoca, g.Evidenze = m.univoca, m.evidenze
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return g, err
	}
	if r.SpostaDa != nil {
		a, err := q.GetComponente(ctx, *r.SpostaDa)
		switch {
		case err == nil:
			g.LasciaA = a.Codice
		case errors.Is(err, pgx.ErrNoRows):
			g.LasciaA = r.SpostaDa.String()
		default:
			return g, err
		}
	}
	return g, nil
}

// conTraccia: la richiesta con la traccia del gesto (messaggio 46 [U]) se il corpo non porta il motivo: la traccia piena
// quando una ragione la vuole (RagioniS8.Serve), e sempre la traccia base per gli eventi che vogliono un motivo.
func conTraccia(ctx context.Context, pool distintabozza.Avviatore, allegato, utente uuid.UUID, r RichiestaS8, c Contesto, prep Preparazione) (RichiestaS8, error) {
	if r.Motivo != "" {
		return r, nil
	}
	g, err := ragioniS8(ctx, pool, allegato, utente, r, c, prep)
	if err != nil {
		return r, err
	}
	r.tracciaBase = TracciaS8(RagioniS8{Sigla: g.Sigla, NomeFile: g.NomeFile, CodiceProdotto: g.CodiceProdotto})
	if g.Serve() {
		r.traccia = TracciaS8(g)
	}
	return r, nil
}

// stessoMotivoDelPasso2: la riga di S8 gia' scritta con la chiave del passo ha il motivo di questo corpo: quello
// dell'utente, oppure la traccia esatta che il server genera con gli stessi dati (il reinvio dello stesso corpo e' «gia'
// fatto»; un altro motivo e' CHIAVE_RIUSATA). Mai un confronto per prefisso.
func (r RichiestaS8) stessoMotivoDelPasso2(registrato string) bool {
	return registrato == r.motivoRegistrato()
}

// passo2: il documento confermato e la riga conferma_documento di S8 (commento di testa), con lo spostamento dello STEP da
// un altro componente se serve (sposta.go). Restituisce l'esito del passo, la copia sul NAS e lo spostamento. Il codice
// della vecchia lettura del file, se non e' quello del prodotto, prende quello del prodotto: la conferma e' il gesto
// esplicito dell'utente, e la riga di S8 ha come motivo il suo o la traccia del server (messaggio 46 [U]; conTraccia).
func passo2(ctx context.Context, pool distintabozza.Avviatore, thread, allegato, utente uuid.UUID, r RichiestaS8) (string, string, *EsitoSpostamento, error) {
	k2 := ChiavePasso(r.ChiaveIdempotenza, "2")
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil, errCodice(distintabozza.CodiceRFQInesistente, "", "la RFQ non c'e'")
		}
		return "", "", nil, err
	}
	// la chiave del passo gia' usata: lo stesso corpo e' «gia' fatto», un altro e' CHIAVE_RIUSATA (l'impronta del corpo
	// la controllano poi le chiavi del passo 3)
	ee, err := q.ListFonteStepPerChiave(ctx, db.ListFonteStepPerChiaveParams{ThreadID: thread, Chiave: k2})
	if err != nil {
		return "", "", nil, err
	}
	if len(ee) > 0 {
		e := ee[0]
		if e.ProdottoID != r.ProdottoID || e.AllegatoID != allegato || e.Gruppo.String != r.gruppoFonte() || !r.stessoMotivoDelPasso2(e.Motivo.String) {
			return "", "", nil, errCodice(distintabozza.CodiceChiaveRiusata, "chiave_idempotenza", "questa chiave d'intento ha gia' confermato un'altra sorgente")
		}
		return EsitoGiaFatto, confermadoc.CopiaNonPrevista, nil, nil
	}
	c, err := Controlla(ctx, q, thread, allegato, r)
	if err != nil {
		return "", "", nil, err
	}
	// lo STEP di un altro componente passa a questo, nella stessa transazione della riga di S8 (sposta.go)
	var spostato *EsitoSpostamento
	if c.Spostamento != nil {
		if spostato, err = eseguiSpostamento(ctx, q, thread, allegato, utente, r, &c, ChiavePasso(r.ChiaveIdempotenza, "2s")); err != nil {
			return "", "", nil, err
		}
		if c.Spostamento.Documento != nil {
			d, err := q.GetDocumento(ctx, c.Spostamento.Documento.DocumentoID)
			if err != nil {
				return "", "", nil, err
			}
			c.Documento = &d
		}
	}
	// la bozza nata da un'altra sorgente S8 prima di R5: la revoca di quella sorgente prima della riga di S8 (la storia
	// della fonte e' revoca, poi conferma: messaggio 43 [U]; sostituzione.go)
	if c.Sostituzione != nil {
		if _, err := scriviRevocaSostituzione(ctx, q, thread, utente, r, c.Sostituzione, c.Sha256, ChiavePasso(r.ChiaveIdempotenza, "2r")); err != nil {
			return "", "", nil, err
		}
	}
	fonte := confermadoc.Fonte{Chiave: k2, Gruppo: r.gruppoFonte(), Motivo: r.motivoRegistrato()}
	esito, copia := EsitoFatto, confermadoc.CopiaNonPrevista
	if c.Documento != nil {
		st, err := confermabom.SorgenteValida(ctx, q, thread, r.ProdottoID, c.Sha256)
		if err != nil {
			return "", "", nil, err
		}
		if st.Valida {
			return EsitoGiaFatto, confermadoc.CopiaNonPrevista, nil, tx.Commit(ctx)
		}
		// il documento c'e' (anche storico, D-AR04 (iii)): solo la riga di S8, scritta adesso da chi conferma
		if err := confermadoc.ScriviConfermaDocumento(ctx, q, thread, r.ProdottoID, c.Documento.DocumentoID, allegato, c.Sha256, utente, fonte); err != nil {
			return "", "", nil, err
		}
	} else {
		pr, err := q.GetPropostaDocumentoDiAllegato(ctx, allegato)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil, errCorpo("allegato", MotivoPropostaAssente, "lo STEP non ha una proposta di documento: aspetta che il file sia letto")
		}
		if err != nil {
			return "", "", nil, err
		}
		p, err := q.BloccaProposta(ctx, pr.PropostaID)
		if err != nil {
			return "", "", nil, err
		}
		if p.Stato != db.StatoPropostaAperta {
			return "", "", nil, errCorpo("allegato", MotivoPropostaDecisa, "la proposta dello STEP e' gia' decisa (%s)", p.Stato)
		}
		a, err := q.GetAllegato(ctx, allegato)
		if err != nil {
			return "", "", nil, err
		}
		m, err := q.GetMessaggio(ctx, a.MessaggioID)
		if err != nil {
			return "", "", nil, err
		}
		// la correzione del codice: con il motivo dell'utente (client di prima) o con la traccia del gesto, che il server
		// scrive quando il codice letto dal file non e' quello del prodotto (messaggio 46 [U]; conTraccia)
		correggi := r.motivoEffettivo() != ""
		es, err := confermadoc.Conferma(ctx, q, confermadoc.Richiesta{Utente: utente, Proposta: p, Allegato: a, Messaggio: m, Tipo: db.TipoDocumentoCad3d,
			Codice: p.Codice.String, Rev: p.Rev.String, Componente: uuid.NullUUID{UUID: r.ProdottoID, Valid: true}, Correggi: correggi,
			Scelta: confermadoc.Scelta{Data: true}, Fonte: fonte})
		if err != nil {
			var rf fascicolo.Rifiuto
			if errors.As(err, &rf) {
				if strings.HasPrefix(string(rf), "codice diverso") {
					// il codice letto dal file e' cambiato dopo la traccia (conTraccia legge fuori dal lucchetto): si rilegge
					return "", "", nil, &distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Campo: "allegato", Anteprima: "prodotto",
						Messaggio: "il codice letto dal file e' cambiato mentre confermavi: rileggi la pagina (" + string(rf) + ")"}
				}
				return "", "", nil, errCorpo("allegato", MotivoConfermaRifiutata, "%s", string(rf))
			}
			return "", "", nil, err
		}
		if !es.FonteScritta || es.Duplicato {
			return "", "", nil, fmt.Errorf("S8 passo 2: la conferma del documento non ha scritto la riga della fonte")
		}
		copia = es.Copia
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", nil, err
	}
	return esito, copia, spostato, nil
}

// impronta di un sotto-passo di R2 (la chiave del salvataggio e' di S8: l'impronta della richiesta e' quella del corpo di
// S8 col nome del passo, cosi' lo stesso reinvio e' «riproposto» e un altro corpo e' CHIAVE_RIUSATA).
func improntaPasso(impronta, passo string) string {
	h := sha256.Sum256([]byte(impronta + "|" + passo))
	return hex.EncodeToString(h[:])
}

// passo3: la bozza (commento di testa).
func passo3(ctx context.Context, pool distintabozza.Avviatore, thread, allegato, utente uuid.UUID, r RichiestaS8, impronta string, prep Preparazione, out EsitoS8) (EsitoS8, error) {
	c, err := contestoInLettura(ctx, pool, thread, allegato, r)
	if err != nil {
		return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
	}
	finale := out
	var scritto bool
	scriviFinale := func(ctx context.Context, q *db.Queries) error {
		raw, err := json.Marshal(finale)
		if err != nil {
			return err
		}
		_, err = q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: r.ChiaveIdempotenza, ThreadID: thread, Gesto: GestoConfermaSorgente,
			UtenteID: utente, ImprontaCorpo: impronta, Esito: raw})
		scritto = err == nil
		return err
	}
	gancio := func(ctx context.Context, q *db.Queries, bozza uuid.UUID, versione int, imp *distintabozza.ImportazioneR2) error {
		finale = completa(out, c.Prodotto.Codice, bozza, versione, imp)
		return scriviFinale(ctx, q)
	}
	comune := func(passo string, bozza uuid.UUID, versione int) distintabozza.ComuneR2 {
		return distintabozza.ComuneR2{ProdottoID: r.ProdottoID, BozzaID: bozza, VersioneAttesa: versione,
			ChiaveSalvataggio: ChiavePasso(r.ChiaveIdempotenza, passo), ImprontaRichiesta: improntaPasso(impronta, passo)}
	}
	sorgente := distintabozza.SorgenteDichiarata{AllegatoID: allegato, Sha256: c.Sha256, Gruppo: r.gruppoFonte()}
	cambia := func(passo string, bozza uuid.UUID, versione int) (*distintabozza.VersioneDelServer, error) {
		v, err := distintabozza.CambiaSorgenteFormato3(ctx, pool, distintabozza.CambioFonte3{ThreadID: thread, UtenteID: utente,
			Comune:         comune(passo, bozza, versione),
			CambiaSorgente: distintabozza.CambiaSorgenteR2{Sorgente: sorgente, ImprontaContenuto: r.ImprontaContenuto, Motivo: motivoCambio(r), Scelte: r.Scelte},
			Proposta:       prep.Proposta, PrimaDelCommit: gancio})
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	var bozzaFinale *BozzaS8Esito
	esito3 := EsitoFatto
	switch {
	case c.Aperta != nil && c.StessaSorgente:
		esito3 = EsitoGiaFatto
		bozzaFinale = &BozzaS8Esito{BozzaID: c.Aperta.BozzaID, Versione: int(c.Aperta.Versione)}
	case c.Aperta != nil && r.Partenza == PartenzaSoloProdotto:
		// la bozza manuale e uno STEP senza struttura importabile: la sorgente solo_prodotto sulla stessa bozza (Controlla
		// ammette solo la bozza senza sorgente)
		v, err := distintabozza.SoloProdottoSuBozzaFormato3(ctx, pool, distintabozza.SoloProdottoSuBozza3{ThreadID: thread, UtenteID: utente,
			Comune: comune("3", c.Aperta.BozzaID, int(c.Aperta.Versione)), Sorgente: sorgente, Motivo: r.motivoRegistrato(), PresaDAttoAnalisi: r.StatoAnalisi,
			PrimaDelCommit: gancio})
		if err != nil {
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		bozzaFinale = esitoBozza(v.BozzaID, v.Versione, nil)
	case c.Aperta != nil && c.Propria:
		// la ripresa dopo da-conferma-1 (k3a gia' scritto): il cambio della fonte (k3b)
		v, err := cambia("3b", c.Aperta.BozzaID, int(c.Aperta.Versione))
		if err != nil {
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		bozzaFinale = esitoBozza(v.BozzaID, v.Versione, v.Importazione)
	case c.Aperta != nil && c.Sostituzione != nil:
		// la bozza nata da un'altra sorgente S8 prima di R5 (G39-03 [U], messaggio 43 caso 1 [U]; sostituzione.go): si
		// abbandona e la nuova nasce dalla proposta, con l'evento riassegnazione_sorgente e l'intento nella stessa transazione
		sx := c.Sostituzione
		motivo := motivoSostituzione(r, c.Sha256)
		crea := distintabozza.CreaR2{Partenza: PartenzaStep, Sorgente: &sorgente, ImprontaContenuto: r.ImprontaContenuto, Motivo: r.motivoRegistrato(),
			Abbandona: &distintabozza.AbbandonaR2{BozzaID: sx.Vecchia.BozzaID, VersioneAttesa: int(sx.Vecchia.Versione), Motivo: troncato(motivo, distintabozza.MaxMotivo)}}
		sostituisci := func(ctx context.Context, q *db.Queries, bozza uuid.UUID, versione int, imp *distintabozza.ImportazioneR2) error {
			revoca, err := scriviRevocaSostituzione(ctx, q, thread, utente, r, sx, c.Sha256, ChiavePasso(r.ChiaveIdempotenza, "3r"))
			if err != nil {
				return err
			}
			if err := distintabozza.ScriviRiassegnazione(ctx, q, thread, bozza, versione, utente, distintabozza.Riassegnazione{
				BozzaAbbandonata: sx.Vecchia.BozzaID, VersioneAbbandonata: int(sx.Vecchia.Versione),
				Prima:        distintabozza.SorgenteRiassegnata{AllegatoID: sx.Allegato, Sha256: sx.Sha256, Gruppo: sx.Gruppo},
				Dopo:         distintabozza.SorgenteRiassegnata{AllegatoID: allegato, Sha256: c.Sha256, Gruppo: r.gruppoFonte()},
				RevocaEvento: revoca, Motivo: motivo}); err != nil {
				return err
			}
			base := out
			base.Sostituzione = esitoSostituzione(c, r, sx, revoca)
			finale = completa(base, c.Prodotto.Codice, bozza, versione, imp)
			finale.Testo = testoCompletaSostituzione(c.Prodotto.Codice)
			return scriviFinale(ctx, q)
		}
		cr, err := distintabozza.CreaFormato3(ctx, pool, distintabozza.Creazione3{ThreadID: thread, UtenteID: utente, Comune: comune("3", uuid.Nil, 0),
			Crea: crea, Proposta: prep.Proposta, PrimaDelCommit: sostituisci, Chiusura: motivo})
		if err != nil {
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		bozzaFinale = esitoBozza(cr.BozzaID, cr.Versione, cr.Importazione)
	case c.Aperta != nil:
		// caso (d): la bozza aperta su un'altra sorgente STEP (nata da una conferma, la revisione del messaggio 41, o con
		// una conferma della BOM del prodotto; la bozza nata da un'altra sorgente S8 prima di R5 si sostituisce, sopra); la
		// bozza manuale (senza sorgente o solo_prodotto) si riconcilia con lo stesso comando (cambio-fonte-2)
		v, err := cambia("3", c.Aperta.BozzaID, int(c.Aperta.Versione))
		if err != nil {
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		bozzaFinale = esitoBozza(v.BozzaID, v.Versione, v.Importazione)
	case c.ConfermaValida:
		// caso (f): da-conferma-1, poi il cambio della fonte se la sorgente e' un'altra
		cr, err := distintabozza.CreaFormato3(ctx, pool, distintabozza.Creazione3{ThreadID: thread, UtenteID: utente, Comune: comune("3a", uuid.Nil, 0),
			Crea: distintabozza.CreaR2{Partenza: distintabozza.PartenzaConferma}})
		if err != nil {
			if b, ok := bozzaNataNellaGara(ctx, pool, thread, allegato, r, err); ok {
				esito3, bozzaFinale = EsitoGiaFatto, &BozzaS8Esito{BozzaID: b.BozzaID, Versione: int(b.Versione)}
				break
			}
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		if sorgenteDi(cr.Snapshot) == c.Sha256+"|"+r.gruppoFonte() {
			bozzaFinale = esitoBozza(cr.BozzaID, cr.Versione, cr.Importazione)
			break
		}
		v, err := cambia("3b", cr.BozzaID, cr.Versione)
		if err != nil {
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		bozzaFinale = esitoBozza(v.BozzaID, v.Versione, v.Importazione)
	default:
		crea := distintabozza.CreaR2{Partenza: r.Partenza, Sorgente: &sorgente, ImprontaContenuto: r.ImprontaContenuto, Motivo: r.motivoRegistrato()}
		if r.Partenza == PartenzaSoloProdotto {
			crea.PresaDAttoAnalisi = r.StatoAnalisi
		}
		cr, err := distintabozza.CreaFormato3(ctx, pool, distintabozza.Creazione3{ThreadID: thread, UtenteID: utente, Comune: comune("3", uuid.Nil, 0),
			Crea: crea, Proposta: prep.Proposta, PrimaDelCommit: gancio})
		if err != nil {
			if b, ok := bozzaNataNellaGara(ctx, pool, thread, allegato, r, err); ok {
				esito3, bozzaFinale = EsitoGiaFatto, &BozzaS8Esito{BozzaID: b.BozzaID, Versione: int(b.Versione)}
				break
			}
			return chiudiParziale(ctx, pool, thread, utente, r, impronta, out, PassoBozza, err)
		}
		bozzaFinale = esitoBozza(cr.BozzaID, cr.Versione, cr.Importazione)
	}
	if scritto {
		return finale, nil
	}
	// gia' fatto, o il reinvio di un R2 gia' scritto senza l'intento: l'intento in una transazione propria
	out.Passi = append(out.Passi, PassoS8{Passo: PassoBozza, Esito: esito3, Testo: testoBozza(esito3)})
	out.Bozza = bozzaFinale
	out.Esito = EsitoCompletata
	out.Testo = testoCompleta(c.Prodotto.Codice)
	return scriviIntentoSeparato(ctx, pool, thread, utente, r, impronta, out)
}

// bozzaNataNellaGara: la creazione della bozza del passo 3 e' rifiutata perche' intanto un'altra S8 sulla stessa RFQ ha
// aperto la bozza del prodotto (il contesto del passo 3 si legge fuori dal lucchetto: la gara fra due S8, blocco «R4
// completo» della corsia persistenza, punto 7). Se quella bozza e' gia' sulla stessa sorgente (sha256 e gruppo) il passo 3
// e' «gia' fatto», come nel caso sequenziale; altrimenti resta il rifiuto (esito parziale: la conferma del passo 2 c'e', la
// bozza e' di un'altra sorgente e si cambia con il corpo che la dichiara) [T-39].
func bozzaNataNellaGara(ctx context.Context, pool distintabozza.Avviatore, thread, allegato uuid.UUID, r RichiestaS8, err error) (*db.DistintaBozza, bool) {
	var e *distintabozza.Errore
	if !errors.As(err, &e) || e.Codice != distintabozza.CodiceBozzaVersione || e.Motivo != distintabozza.MotivoAltraBozzaAperta {
		return nil, false
	}
	c, err := contestoInLettura(ctx, pool, thread, allegato, r)
	if err != nil || c.Aperta == nil || !c.StessaSorgente {
		return nil, false
	}
	return c.Aperta, true
}

func motivoCambio(r RichiestaS8) string {
	if m := r.motivoRegistrato(); m != "" {
		return m
	}
	// l'evento del cambio della fonte vuole sempre un motivo (trg_dbv_fonte_motivata): la traccia base del gesto
	if r.tracciaBase != "" {
		return r.tracciaBase
	}
	return "conferma della sorgente (S8)"
}

func esitoBozza(bozza uuid.UUID, versione int, imp *distintabozza.ImportazioneR2) *BozzaS8Esito {
	b := &BozzaS8Esito{BozzaID: bozza, Versione: versione}
	if imp != nil && imp.ImportazioneID != uuid.Nil {
		id := imp.ImportazioneID
		b.Importazione = &id
	}
	return b
}

func completa(out EsitoS8, codice string, bozza uuid.UUID, versione int, imp *distintabozza.ImportazioneR2) EsitoS8 {
	x := out
	x.Passi = append(append([]PassoS8(nil), out.Passi...), PassoS8{Passo: PassoBozza, Esito: EsitoFatto, Testo: testoBozza(EsitoFatto)})
	x.Bozza = esitoBozza(bozza, versione, imp)
	x.Esito = EsitoCompletata
	x.Testo = testoCompleta(codice)
	return x
}

func testoBozza(esito string) string {
	if esito == EsitoGiaFatto {
		return "La bozza del prodotto era già su questa sorgente."
	}
	return "La bozza del prodotto è pronta da rivedere."
}

func testoCompleta(codice string) string {
	return "Sorgente confermata: la bozza di " + codice + " è pronta da rivedere. La BOM non è confermata."
}

// sorgenteDi: «sha256|gruppo» della sorgente di uno snapshot formato 3 ("" senza).
func sorgenteDi(raw json.RawMessage) string {
	var s struct {
		Sorgente *struct {
			Sha256 string `json:"sha256"`
			Gruppo string `json:"gruppo"`
		} `json:"sorgente"`
	}
	if json.Unmarshal(raw, &s) != nil || s.Sorgente == nil {
		return ""
	}
	return s.Sorgente.Sha256 + "|" + s.Sorgente.Gruppo
}

// scriviIntentoSeparato: l'intento in una transazione propria (dopo un rifiuto definitivo, o un passo 3 gia' fatto). Se
// un'altra richiesta con la stessa chiave l'ha gia' scritto, vale quello (riproposto).
func scriviIntentoSeparato(ctx context.Context, pool distintabozza.Avviatore, thread, utente uuid.UUID, r RichiestaS8, impronta string, out EsitoS8) (EsitoS8, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		return out, err
	}
	if gia, err := IntentoRegistrato(ctx, q, thread, r.ChiaveIdempotenza, impronta, GestoConfermaSorgente); err != nil {
		return out, err
	} else if gia != nil {
		var x EsitoS8
		if err := json.Unmarshal(gia, &x); err != nil {
			return out, err
		}
		x.Riproposto = true
		return x, nil
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if _, err := q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: r.ChiaveIdempotenza, ThreadID: thread, Gesto: GestoConfermaSorgente,
		UtenteID: utente, ImprontaCorpo: impronta, Esito: raw}); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

// Riproposto: l'esito gia' registrato, riletto per la risposta (riproposto vero).
func Riproposto(raw json.RawMessage) (EsitoS8, error) {
	var x EsitoS8
	if err := json.Unmarshal(raw, &x); err != nil {
		return x, err
	}
	x.Riproposto = true
	return x, nil
}

// rifiutoSostituito: lo STEP e' identico a un documento gia' sostituito (GIA_DOCUMENTO). Se il documento e' di un altro
// prodotto finito e' lo STEP rimasto nella storia della sua BOM dopo una revisione (G39-03 dopo R5): in V1 non diventa la
// sorgente di questo prodotto (documento e' UNIQUE (thread_id, sha256), la storia dell'altro resta com'e'; cambio di schema
// aperto), motivo sostituito_in_altro_prodotto con altro_prodotto. Il testo non propone «annulla la sostituzione»: nel
// formato 3 la sostituzione dello STEP strutturale la scrive solo R5.
func rifiutoSostituito(ctx context.Context, q *db.Queries, d db.Documento, p db.GetProdottoDellaRfqRow) error {
	succ := ""
	if s, err := q.GetDocumento(ctx, d.SostituitoDa.UUID); err == nil {
		succ = s.NomeFile
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if d.ComponenteID.Valid && d.ComponenteID.UUID != p.ComponenteID {
		c, err := q.GetComponente(ctx, d.ComponenteID.UUID)
		if err != nil {
			return err
		}
		if c.Tipo == db.TipoComponenteFinito {
			altro := c.ComponenteID
			return &distintabozza.Errore{Codice: CodiceGiaDocumento, Motivo: MotivoSostituitoInAltroProdotto, Campo: "allegato", AltroProdotto: &altro,
				Messaggio: fmt.Sprintf("Questo STEP è nella storia della BOM di %s (con la revisione è stato sostituito da %s): in V1 non può diventare la sorgente di %s. Niente è cambiato.",
					c.Codice, succ, p.Codice)}
		}
	}
	if succ == "" {
		return errCodice(CodiceGiaDocumento, "allegato", "il file è identico a %s, che risulta sostituito: ricarica la pagina", d.NomeFile)
	}
	return errCodice(CodiceGiaDocumento, "allegato", "il file è identico a %s, che %s ha sostituito: S8 non riporta come sorgente uno STEP sostituito", d.NomeFile, succ)
}
