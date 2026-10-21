package distintabozza

// La conversione di una bozza formato 2 aperta nel formato 3, algoritmo conversione-1 (EM §6.7; PM23 U-05). Serve un
// comando: il trigger del ciclo vieta di cambiare il formato di una bozza aperta, e D-18 vieta le conversioni in lettura.
// In una transazione:
//  1. la bozza formato 2 si abbandona, con motivo_chiusura «convertita nel formato 3 nella bozza <id nuovo>», senza il
//     motivo libero (sta nei 1000 caratteri di ck_distinta_bozza_chiusura); il motivo intero va nell'evento bozza_aperta
//     della bozza nuova; la riga abbandonata del registro delle bozze la scrive il trigger;
//  2. la bozza formato 3 nasce alla versione 1, costruita dal server: nodi, archi e decisioni copiati, le preassociazioni
//     tradotte (sotto);
//     identita = {cliente, presunto, codice, rev, modo formato_2}; le caselle completate con l'allegato della sorgente o,
//     senza, con l'unico allegato attivo della RFQ con quello sha256 (piu' d'uno: conversione_ambigua); caselle di piu'
//     gruppi: conversione_piu_gruppi; righe_proposta lasciate cadere con la traccia (righe_cadute); nessun registro;
//  3. l'evento bozza_aperta della bozza nuova: partenza formato_2, il motivo, da_bozza {bozza_id, versione, impronta,
//     righe_cadute[], archi_senza_step[], nodi_senza_casella[], decisioni_completate[], fonti_non_riportate[],
//     preassociazioni_tradotte, preassociazioni_segnalate[]};
//  4. le fonti secondarie con una destinazione temporanea sulla bozza formato 2, non ancora risolte: per ogni chiave la
//     sua sequenza (scelte e rifiuti, nell'ordine, con il motivo) rifatta sulla bozza nuova, dopo la versione 1, cosi'
//     una fonte rifiutata resta rifiutata e una sostituzione (un rifiuto piu' una scelta) resta tale.
//
// Le regole che EM §6.7 non scrive e che il formato 2 rende necessarie (proposte di emendamento [T] della corsia
// persistenza, secondo passaggio dell'ondata B, PB-16 e PB-23…PB-27 del resoconto):
//   - un arco dello STEP che nel formato 3 non unisce due caselle resta dell'ingegnere (origine utente), con la traccia
//     archi_senza_step (PB-16); un nodo con origine step senza caselle, che il formato 2 ammette, resta dell'ingegnere
//     (origine utente), con la traccia nodi_senza_casella;
//   - una ricodifica del formato 2 non ha emittente_dopo (obbligatorio nel 3): si completa con il cliente della RFQ, come
//     l'identita' formato_2 dei nodi, con la traccia decisioni_completate; una decisione che anche cosi' non ha la forma
//     del formato 3 ferma la conversione: 422 conversione_decisione_non_convertibile, con l'elenco;
//   - una casella con uno sha256 diverso da quello della sorgente (nel formato 3 di V1 le caselle sono della sola
//     sorgente, EM §3.3): 422 conversione_casella_fuori_sorgente; uno sha256 senza nessun allegato attivo della RFQ (con
//     la sorgente senza allegato): 422 conversione_senza_occorrenza. La bozza formato 2 resta aperta;
//   - la fonte secondaria di una chiave che non e' un nodo senza UUID della versione 1 (il nodo tolto dopo l'evento, o
//     con un componente) non si riporta: la traccia fonti_non_riportate dice quale e perche', e il trigger
//     fonte_step_evento_nasce non si incontra (un P4X07 residuo e' un 422, mai un 500);
//   - la risposta elenca in righe_cadute solo i nodi senza casella che ne avevano (EM §6.7); l'evento le porta tutte, e
//     un elenco che porterebbe il dopo oltre i 128 KiB dell'evento (EM §3.10) diventa il suo conteggio (<elenco>_ridotto),
//     ricalcolabile dalla bozza formato 2 finche' c'e' (90 giorni).
//
// Le preassociazioni (direttiva 35 §1.1 [U], blocco C-2; EM §6.7; PD35 §6.3): la conversione e' l'unico punto di
// traduzione dal formato 2. Ogni {proposta_id, chiave_bozza} diventa {allegato_id, chiave_bozza} con l'allegato della
// proposta, letto nella stessa transazione; la traduzione e' 1:1 per vincolo (documento_proposta.allegato_id e' NOT NULL
// UNIQUE). Una proposta che non e' della RFQ (o non c'e' piu') ferma la conversione: 422 conversione_proposta_non_risolta,
// con l'elenco, e la bozza formato 2 resta aperta. Un'occorrenza oggi non attiva, o di una natura che non si preassocia
// (provvisorio, base [T] D-2 (a)), si copia lo stesso e si segnala (preassociazioni_segnalate, con il motivo
// file_non_attivo o natura_non_preassociabile): e' una PB gia' salvata e scaduta, che R1 e R4 mostrano (EM §6.4); il
// conteggio delle tradotte sta nella traccia di bozza_aperta. Il formato 2 non ha svincoli (scollega_documento e' del
// formato 3): non c'e' altro da tradurre.
//
// Gli snapshot formato 2 restano com'erano. La forma converti_da di R2 che chiama il servizio arriva con l'ondata C, con
// le altre forme costruite dal server; la risoluzione delle fonti riportate alla conferma vuole R5 (ondata C).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// AlgoritmoConversione e' l'algoritmo che costruisce la versione 1 di una bozza convertita (EM §2.5, §6.7).
const AlgoritmoConversione = "conversione-1"

// PartenzaFormato2: la partenza dell'evento bozza_aperta di una bozza convertita.
const PartenzaFormato2 = "formato_2"

// I motivi della conversione (EM §9.2; gli ultimi tre sono proposte [T], vedi il commento di testa) e il codice del motivo
// mancante.
const (
	MotivoConversioneAmbigua         = "conversione_ambigua"
	MotivoConversionePiuGruppi       = "conversione_piu_gruppi"
	MotivoConversioneFuoriSorgente   = "conversione_casella_fuori_sorgente"
	MotivoConversioneSenzaOccorrenza = "conversione_senza_occorrenza"
	MotivoConversioneDecisione       = "conversione_decisione_non_convertibile"
	// MotivoConversionePropostaNonRisolta: una preassociazione formato 2 cita una proposta che non e' della RFQ, e non si
	// traduce in un allegato (direttiva 35, C-2; il nome e' una scelta [T] della corsia, A-07 della corsia contratto).
	MotivoConversionePropostaNonRisolta = "conversione_proposta_non_risolta"
	CodiceMotivoObbligatorio            = "MOTIVO_OBBLIGATORIO"
)

// I motivi di una fonte secondaria non riportata sulla bozza nuova.
const (
	FonteNodoAssente     = "nodo_assente"  // la chiave non e' un nodo della versione 1
	FonteNodoConUUID     = "nodo_con_uuid" // il nodo ha un componente: la destinazione sarebbe componente_id, non la chiave
	fonteRuoloSecondaria = "sottoassieme"
)

// Conversione3 e' una richiesta di conversione (la forma converti_da di R2): la bozza formato 2, la sua versione, il
// motivo, la chiave del salvataggio e l'impronta della richiesta.
type Conversione3 struct {
	ThreadID, ProdottoID, UtenteID uuid.UUID
	BozzaID                        uuid.UUID
	VersioneAttesa                 int
	Motivo                         string
	ChiaveSalvataggio              uuid.UUID
	ImprontaRichiesta              string
}

// RigheCadute: le righe legacy di un nodo che la conversione lascia cadere (EM §6.7: R5 non le decidera').
type RigheCadute struct {
	ChiaveBozza string      `json:"chiave_bozza"`
	Righe       []uuid.UUID `json:"righe"`
}

// ArcoSenzaStep: un arco con origine step nel formato 2 che nel formato 3 non si lega a due caselle (EM §3.3: lo step di un
// arco e' (allegato, rif del padre, rif del figlio); il nodo del prodotto formato 2 non ha caselle, §6.7) e resta come arco
// dell'ingegnere, origine utente, con la stessa qta.
type ArcoSenzaStep struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
}

// DecisioneCompletata: una decisione del formato 2 completata per avere la forma del formato 3 (la ricodifica senza
// emittente_dopo, con il cliente della RFQ).
type DecisioneCompletata struct {
	ID    string `json:"id"`
	Campo string `json:"campo"`
}

// Il motivo di una preassociazione tradotta e segnalata.
const (
	PreassociazioneFileNonAttivo      = "file_non_attivo"           // l'occorrenza e' in Altri file o scartata
	PreassociazioneNaturaNo           = "natura_non_preassociabile" // contenitore, archivio, natura non file
	PreassociazioneLegatoComeSorgente = "legato_come_sorgente"      // lo STEP sorgente del finito (direttiva 39 §5, D-2 (c))
)

// PreassociazioneSegnalata: una preassociazione formato 2 tradotta e copiata nella versione 1, ma su un'occorrenza che
// oggi non si preassocerebbe (il motivo): resta una PB salvata e scaduta, che R1 e R4 mostrano.
type PreassociazioneSegnalata struct {
	PropostaID  uuid.UUID `json:"proposta_id"`
	AllegatoID  uuid.UUID `json:"allegato_id"`
	ChiaveBozza string    `json:"chiave_bozza"`
	Motivo      string    `json:"motivo"`
}

// FonteNonRiportata: gli eventi della fonte secondaria di una chiave della bozza formato 2 che la conversione non riporta,
// con il motivo (FonteNodoAssente, FonteNodoConUUID).
type FonteNonRiportata struct {
	ChiaveBozza string  `json:"chiave_bozza"`
	Eventi      []int64 `json:"eventi"`
	Motivo      string  `json:"motivo"`
}

// Convertita e' l'esito della conversione: la versione 1 della bozza nuova con il suo snapshot e le tracce. RigheCadute
// elenca i soli nodi senza casella che avevano righe (EM §6.7: quelle che R5 non decidera'); l'evento le ha tutte.
type Convertita struct {
	Salvato3
	Snapshot            json.RawMessage       `json:"snapshot"`
	RigheCadute         []RigheCadute         `json:"righe_cadute"`
	ArchiSenzaStep      []ArcoSenzaStep       `json:"archi_senza_step"`
	NodiSenzaCasella    []string              `json:"nodi_senza_casella"`
	DecisioniCompletate []DecisioneCompletata `json:"decisioni_completate"`
	FontiNonRiportate   []FonteNonRiportata   `json:"fonti_non_riportate"`
	// direttiva 35, C-2: quante preassociazioni sono tradotte da proposta_id ad allegato_id, e quelle segnalate
	PreassociazioniTradotte  int                        `json:"preassociazioni_tradotte"`
	PreassociazioniSegnalate []PreassociazioneSegnalata `json:"preassociazioni_segnalate"`
	// direttiva 39 §4 (conversionep1.go): le pre-assegnazioni legacy convertite alla presa in carico e quelle rimaste
	ConversioniLegacy            []ConversioneLegacy3            `json:"conversioni_legacy,omitempty"`
	PreassegnazioniNonConvertite []PreassegnazioneNonConvertita3 `json:"preassegnazioni_non_convertite,omitempty"`
}

// tracciaConversione: quello che conversione-1 lascia cadere, completa o traduce, per l'evento e la risposta.
type tracciaConversione struct {
	RigheCadute              []RigheCadute
	ArchiSenzaStep           []ArcoSenzaStep
	NodiSenzaCasella         []string
	DecisioniCompletate      []DecisioneCompletata
	PreassociazioniTradotte  int
	PreassociazioniSegnalate []PreassociazioneSegnalata
}

// Converti converte una bozza formato 2 aperta (commento di testa).
func Converti(ctx context.Context, pool Avviatore, c Conversione3) (Convertita, error) {
	var out Convertita
	motivo := strings.TrimSpace(nfc(c.Motivo))
	if motivo == "" {
		return out, &Errore{Codice: CodiceMotivoObbligatorio, Campo: "converti_da.motivo", Messaggio: "la conversione vuole il motivo"}
	}
	if !testoLibero(motivo, MaxMotivo) {
		return out, corpo("converti_da.motivo", "il motivo sta in %d caratteri, senza caratteri di controllo", MaxMotivo)
	}
	if c.ChiaveSalvataggio.Version() != 4 || !reSha256.MatchString(c.ImprontaRichiesta) {
		return out, corpo("chiave_salvataggio", "la chiave del salvataggio e' un UUID v4, con l'impronta della richiesta")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, c.ThreadID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	if gia, err := q.GetVersionePerChiaveGlobale(ctx, uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true}); err == nil {
		if gia.ThreadID != c.ThreadID || strings.TrimSpace(gia.ImprontaRichiesta.String) != c.ImprontaRichiesta {
			return out, &Errore{Codice: CodiceChiaveRiusata, Campo: "chiave_salvataggio", Messaggio: "questa chiave di salvataggio ha gia' salvato un altro corpo"}
		}
		return riproponiConversione(ctx, q, c.ThreadID, gia.BozzaID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if _, err := prodottoDellaRfq(ctx, q, c.ThreadID, c.ProdottoID, true); err != nil {
		return out, err
	}
	b, err := q.BloccaDistintaBozza(ctx, db.BloccaDistintaBozzaParams{BozzaID: c.BozzaID, ThreadID: c.ThreadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("converti_da.bozza_id", "la bozza %s non e' di questa RFQ", c.BozzaID)
	}
	if err != nil {
		return out, err
	}
	if b.ProdottoID != c.ProdottoID {
		return out, &Errore{Codice: CodiceBozzaAltroProdotto, Campo: "converti_da.bozza_id", Messaggio: "la bozza e' di un altro prodotto"}
	}
	if b.Stato != StatoAperta || int(b.Versione) != c.VersioneAttesa {
		return out, conflittoVersione(b, c.VersioneAttesa)
	}
	if int(b.Formato) != FormatoSnapshot {
		return out, corpo("converti_da.bozza_id", "la bozza e' gia' nel formato %d: si converte solo una bozza formato 2", b.Formato)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: c.ThreadID, Versione: b.Versione})
	if err != nil {
		return out, err
	}
	vecchio, improntaVecchia, err := rileggiSnapshot(v.Snapshot, v.Impronta)
	if err != nil {
		return out, err
	}
	cliente, err := q.GetClienteDellaRfq(ctx, c.ThreadID)
	if err != nil {
		return out, err
	}
	nuovo, tr, err := convertiSnapshot(ctx, q, c.ThreadID, vecchio, EmittenteCliente(cliente))
	if err != nil {
		return out, err
	}
	// direttiva 39 §4 (conversionep1.go, D-AR14 A [T-39]): la nascita della bozza formato 3 e' la presa in carico del
	// prodotto: le pre-assegnazioni legacy aperte dei nodi diventano PB legacy (una PB del formato 2 dello stesso file
	// resta quella), con l'evento conversione_legacy e la copia tolta
	in, err := ingressoPartenza(ctx, q, c.ThreadID, c.ProdottoID)
	if err != nil {
		return out, err
	}
	per, err := perimetroDi(in)
	if err != nil {
		return out, err
	}
	var presa PresaInCarico
	if presa.Conversioni, presa.NonConvertite, err = convertiPreassegnazioni(ctx, q, c.ThreadID, c.UtenteID, c.ChiaveSalvataggio, &nuovo, per); err != nil {
		return out, err
	}
	for _, x := range presa.Conversioni {
		if !x.PBEsistente {
			presa.Iniziali = append(presa.Iniziali, Preassociazione3{AllegatoID: x.AllegatoID, ChiaveBozza: x.ChiaveBozza, Provenienza: ProvenienzaLegacy})
		}
	}
	canon, err := nuovo.Canonico()
	if err != nil {
		var e *Errore
		if errors.As(err, &e) {
			e.Messaggio = "la bozza formato 2 non si converte: " + e.Messaggio
		}
		return out, err
	}
	impronta := jsonImpronta(canon)
	// le fonti secondarie da riportare, per chiave e nell'ordine, e quelle che non si riportano (con la traccia nell'evento)
	daRiportare, nonRiportate, err := fontiDaRiportare(ctx, q, c, b.BozzaID, nuovo)
	if err != nil {
		return out, err
	}

	// 1. la bozza formato 2 si chiude
	nuovoID := uuid.New()
	n, err := q.AbbandonaDistintaBozza(ctx, db.AbbandonaDistintaBozzaParams{ChiusaDa: uuid.NullUUID{UUID: c.UtenteID, Valid: true},
		Motivo: testo("convertita nel formato 3 nella bozza " + nuovoID.String()), BozzaID: b.BozzaID, ThreadID: c.ThreadID, Versione: b.Versione})
	if err != nil {
		return out, err
	}
	if n != 1 {
		return out, conflittoVersione(b, c.VersioneAttesa)
	}
	// 2. la bozza formato 3, versione 1
	base := baseDelloSnapshot3(nuovo)
	fonteAll, fonteSha, fonteGruppo := colonneFonte3(nuovo.Sorgente)
	if _, err := q.InsertDistintaBozzaConID(ctx, db.InsertDistintaBozzaConIDParams{BozzaID: nuovoID, ThreadID: c.ThreadID, ProdottoID: c.ProdottoID,
		Formato: FormatoSnapshot3, BaseConfermaID: nullUUID(base.ConfermaID), BaseNumero: int32(base.Numero), BaseContatore: base.Contatore,
		FirmaBase: base.Firma, FonteAllegatoID: fonteAll, FonteSha256: fonteSha, FonteGruppo: fonteGruppo, UtenteID: c.UtenteID}); err != nil {
		return out, erroreDiScrittura(err, "converti_da")
	}
	salvataIl, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{
		BozzaID: nuovoID, Versione: 1, Snapshot: canon, Impronta: impronta, BaseConfermaID: nullUUID(base.ConfermaID),
		BaseNumero: int32(base.Numero), BaseContatore: base.Contatore, FirmaBase: base.Firma, FonteAllegatoID: fonteAll,
		FonteSha256: fonteSha, FonteGruppo: fonteGruppo, SalvataDa: c.UtenteID,
		ChiaveSalvataggio: uuid.NullUUID{UUID: c.ChiaveSalvataggio, Valid: true}, ImprontaRichiesta: testo(c.ImprontaRichiesta),
	})
	if err != nil {
		return out, erroreDiScrittura(err, "converti_da")
	}
	// 3. l'evento bozza_aperta (origine server), con il motivo intero e le tracce, nei 128 KiB dell'evento
	daBozza := map[string]any{"bozza_id": b.BozzaID, "versione": b.Versione, "impronta": improntaVecchia, "righe_cadute": tr.RigheCadute,
		"archi_senza_step": tr.ArchiSenzaStep, "nodi_senza_casella": tr.NodiSenzaCasella, "decisioni_completate": tr.DecisioniCompletate,
		"fonti_non_riportate": nonRiportate, "preassociazioni_tradotte": tr.PreassociazioniTradotte,
		"preassociazioni_segnalate": tr.PreassociazioniSegnalate}
	dopo := map[string]any{"partenza": PartenzaFormato2, "algoritmo": AlgoritmoConversione, "impronta": impronta,
		"nodi": len(nuovo.Nodi), "archi": len(nuovo.Archi), "da_bozza": daBozza}
	presa.nelDopo(dopo)
	riduciTracce(dopo, daBozza)
	dopoNeiLimiti(dopo, elenchiDellaPresa...)
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: nuovoID, N: 1, Versione: 1,
		Vocabolario: VocabolarioFormato3, Azione: AzBozzaAperta, Dopo: rawONull(canonico(dopo)), Origine: "server", Motivo: testo(motivo),
		UtenteID: c.UtenteID}); err != nil {
		return out, erroreDiScrittura(err, "converti_da")
	}
	// 4. le fonti secondarie, per chiave e nell'ordine dei loro eventi
	for _, f := range daRiportare {
		if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: c.ThreadID, ProdottoID: c.ProdottoID,
			Ruolo: fonteRuoloSecondaria, ChiaveBozza: f.ChiaveBozza, Azione: f.Azione, AllegatoID: f.AllegatoID, Sha256: f.Sha256,
			Gruppo: f.Gruppo, PropostaID: f.PropostaID, DocumentoID: f.DocumentoID, BozzaID: uuid.NullUUID{UUID: nuovoID, Valid: true},
			Motivo: f.Motivo, AutoreID: c.UtenteID}); err != nil {
			if sqlstate(err) == "P4X07" {
				return out, corpo("converti_da", "il database rifiuta la fonte secondaria di %s sulla bozza nuova (P4X07): la bozza formato 2 resta aperta",
					f.ChiaveBozza.String)
			}
			return out, erroreDiScrittura(err, "converti_da")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if s := sqlstate(err); s == "P4X04" || s == "P4X07" {
			return out, corpo("converti_da", "il database rifiuta la conversione (%s): la bozza formato 2 resta aperta", s)
		}
		return out, err
	}
	out = Convertita{Salvato3: Salvato3{BozzaID: nuovoID, Formato: FormatoSnapshot3, Versione: 1, NuovaVersione: true, Eventi: 1,
		Impronta: impronta, SalvataIl: salvataIl}, Snapshot: canon, RigheCadute: righeSenzaCasella(tr.RigheCadute, nuovo),
		ArchiSenzaStep: tr.ArchiSenzaStep, NodiSenzaCasella: tr.NodiSenzaCasella, DecisioniCompletate: tr.DecisioniCompletate,
		FontiNonRiportate: nonRiportate, PreassociazioniTradotte: tr.PreassociazioniTradotte,
		PreassociazioniSegnalate: tr.PreassociazioniSegnalate, ConversioniLegacy: presa.Conversioni,
		PreassegnazioniNonConvertite: presa.NonConvertite}
	return out, nil
}

// fontiDaRiportare: gli eventi della fonte secondaria con la destinazione temporanea sulla bozza formato 2, non ancora
// risolti (scelte e rifiuti), per chiave e nell'ordine; quelli di una chiave che non e' un nodo senza UUID della versione 1
// della bozza nuova non si riportano, e tornano con il motivo (commento di testa).
func fontiDaRiportare(ctx context.Context, q *db.Queries, c Conversione3, vecchia uuid.UUID, nuovo Snapshot3) ([]db.ListFonteStepProdottoRow, []FonteNonRiportata, error) {
	fonti, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: c.ThreadID, ProdottoID: c.ProdottoID})
	if err != nil {
		return nil, nil, err
	}
	risolti := map[int64]bool{}
	for _, f := range fonti {
		if f.RisolveEventoID.Valid {
			risolti[f.RisolveEventoID.Int64] = true
		}
	}
	nodi := map[string]Nodo3{}
	for _, n := range nuovo.Nodi {
		nodi[n.ChiaveBozza] = n
	}
	var riporta []db.ListFonteStepProdottoRow
	saltate := map[string]*FonteNonRiportata{}
	var ordine []string
	for _, f := range fonti { // in ordine di evento_id
		if f.Ruolo != fonteRuoloSecondaria || (f.Azione != "scelta" && f.Azione != "rifiuto") || !f.ChiaveBozza.Valid || !f.BozzaID.Valid ||
			f.BozzaID.UUID != vecchia || f.ComponenteID.Valid || risolti[f.EventoID] {
			continue
		}
		k := f.ChiaveBozza.String
		n, ok := nodi[k]
		motivo := ""
		switch {
		case !ok:
			motivo = FonteNodoAssente
		case n.ComponenteID != nil:
			motivo = FonteNodoConUUID
		}
		if motivo == "" {
			riporta = append(riporta, f)
			continue
		}
		s, gia := saltate[k]
		if !gia {
			s = &FonteNonRiportata{ChiaveBozza: k, Motivo: motivo}
			saltate[k] = s
			ordine = append(ordine, k)
		}
		s.Eventi = append(s.Eventi, f.EventoID)
	}
	nonRiportate := []FonteNonRiportata{}
	sort.Strings(ordine)
	for _, k := range ordine {
		nonRiportate = append(nonRiportate, *saltate[k])
	}
	return riporta, nonRiportate, nil
}

// riduciTracce tiene il dopo dell'evento nei 128 KiB (MaxPrimaDopo, EM §3.10): gli elenchi piu' lunghi, uno alla volta,
// diventano il loro conteggio (<elenco>_ridotto). Gli elenchi interi si ricalcolano dalla bozza formato 2, che resta 90
// giorni.
func riduciTracce(dopo, daBozza map[string]any) {
	for _, k := range []string{"righe_cadute", "archi_senza_step", "nodi_senza_casella", "fonti_non_riportate", "decisioni_completate",
		"preassociazioni_segnalate"} {
		if len(canonico(dopo)) <= MaxPrimaDopo {
			return
		}
		voci := 0
		if b, err := json.Marshal(daBozza[k]); err == nil {
			var xx []json.RawMessage
			if json.Unmarshal(b, &xx) == nil {
				voci = len(xx)
			}
		}
		delete(daBozza, k)
		daBozza[k+"_ridotto"] = voci
	}
}

// righeSenzaCasella: le righe cadute dei soli nodi che nello snapshot convertito non hanno caselle (EM §6.7: la risposta
// elenca i nodi senza casella che ne avevano, perche' R5 non decidera' quelle righe; le righe dei nodi con la casella R5
// le ritrova per (sha256, chiave)).
func righeSenzaCasella(tutte []RigheCadute, s Snapshot3) []RigheCadute {
	conCasella := map[string]bool{}
	for _, n := range s.Nodi {
		if len(n.Caselle) > 0 {
			conCasella[n.ChiaveBozza] = true
		}
	}
	out := []RigheCadute{}
	for _, r := range tutte {
		if !conCasella[r.ChiaveBozza] {
			out = append(out, r)
		}
	}
	return out
}

// riproponiConversione: il reinvio di una conversione gia' fatta (stessa chiave, stesso corpo): la versione 1 e gli
// elenchi ricalcolati dai dati salvati, senza scrivere. Un elenco ridotto nell'evento si ricalcola dalla bozza formato 2,
// se c'e' ancora.
func riproponiConversione(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID) (Convertita, error) {
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: bozza, ThreadID: thread, Versione: 1})
	if err != nil {
		return Convertita{}, err
	}
	s, imp, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return Convertita{}, err
	}
	can, err := s.codifica()
	if err != nil {
		return Convertita{}, err
	}
	out := Convertita{Salvato3: Salvato3{BozzaID: bozza, Formato: FormatoSnapshot3, Versione: 1, Riproposto: true, Impronta: imp,
		SalvataIl: v.SalvataIl}, Snapshot: can, RigheCadute: []RigheCadute{}, ArchiSenzaStep: []ArcoSenzaStep{}, NodiSenzaCasella: []string{},
		DecisioniCompletate: []DecisioneCompletata{}, FontiNonRiportate: []FonteNonRiportata{}, PreassociazioniTradotte: len(s.DocumentiPreassociati),
		PreassociazioniSegnalate: []PreassociazioneSegnalata{}}
	e, err := q.GetDistintaBozzaEvento(ctx, db.GetDistintaBozzaEventoParams{BozzaID: bozza, ThreadID: thread, N: 1})
	if err != nil || e.Dopo == nil {
		return out, nil
	}
	var d struct {
		Conversioni   []ConversioneLegacy3            `json:"conversioni_legacy"`
		NonConvertite []PreassegnazioneNonConvertita3 `json:"preassegnazioni_non_convertite"`
		DaBozza       struct {
			BozzaID             uuid.UUID                  `json:"bozza_id"`
			Versione            int32                      `json:"versione"`
			RigheCadute         []RigheCadute              `json:"righe_cadute"`
			RigheRidotte        *int                       `json:"righe_cadute_ridotto"`
			ArchiSenzaStep      []ArcoSenzaStep            `json:"archi_senza_step"`
			NodiSenzaCasella    []string                   `json:"nodi_senza_casella"`
			DecisioniCompletate []DecisioneCompletata      `json:"decisioni_completate"`
			FontiNonRiportate   []FonteNonRiportata        `json:"fonti_non_riportate"`
			Tradotte            *int                       `json:"preassociazioni_tradotte"`
			Segnalate           []PreassociazioneSegnalata `json:"preassociazioni_segnalate"`
		} `json:"da_bozza"`
	}
	if json.Unmarshal(*e.Dopo, &d) != nil {
		return out, nil
	}
	out.ConversioniLegacy, out.PreassegnazioniNonConvertite = d.Conversioni, d.NonConvertite
	righe := d.DaBozza.RigheCadute
	if d.DaBozza.RigheRidotte != nil {
		// l'elenco intero non entrava nell'evento: si ricalcola dalla bozza formato 2, se non e' gia' stata cancellata
		if v2, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: d.DaBozza.BozzaID, ThreadID: thread,
			Versione: d.DaBozza.Versione}); err == nil {
			if vecchio, _, err := rileggiSnapshot(v2.Snapshot, v2.Impronta); err == nil {
				righe = righeCaduteDi(vecchio)
			}
		}
	}
	out.RigheCadute = righeSenzaCasella(righe, s)
	if d.DaBozza.ArchiSenzaStep != nil {
		out.ArchiSenzaStep = d.DaBozza.ArchiSenzaStep
	}
	if d.DaBozza.NodiSenzaCasella != nil {
		out.NodiSenzaCasella = d.DaBozza.NodiSenzaCasella
	}
	if d.DaBozza.DecisioniCompletate != nil {
		out.DecisioniCompletate = d.DaBozza.DecisioniCompletate
	}
	if d.DaBozza.FontiNonRiportate != nil {
		out.FontiNonRiportate = d.DaBozza.FontiNonRiportate
	}
	if d.DaBozza.Tradotte != nil {
		out.PreassociazioniTradotte = *d.DaBozza.Tradotte
	}
	if d.DaBozza.Segnalate != nil {
		out.PreassociazioniSegnalate = d.DaBozza.Segnalate
	}
	return out, nil
}

// righeCaduteDi: le righe legacy dei nodi di uno snapshot formato 2 (le coppie di da_bozza.righe_cadute).
func righeCaduteDi(s Snapshot) []RigheCadute {
	out := []RigheCadute{}
	for _, n := range s.Nodi {
		if len(n.RigheProposta) > 0 {
			out = append(out, RigheCadute{ChiaveBozza: n.ChiaveBozza, Righe: append([]uuid.UUID(nil), n.RigheProposta...)})
		}
	}
	return out
}

// convertiSnapshot: lo snapshot formato 3 della versione 1 di una bozza convertita, con le tracce (commento di testa,
// punto 2 e regole).
func convertiSnapshot(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot, cliente string) (Snapshot3, tracciaConversione, error) {
	out := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: s.ProdottoID, VersioneBomBase: s.VersioneBomBase, FirmaBase: s.FirmaBase}
	tr := tracciaConversione{RigheCadute: righeCaduteDi(s), ArchiSenzaStep: []ArcoSenzaStep{}, NodiSenzaCasella: []string{},
		DecisioniCompletate: []DecisioneCompletata{}, PreassociazioniSegnalate: []PreassociazioneSegnalata{}}
	var sorgente *Sorgente3
	if s.Sorgente != nil {
		sorgente = &Sorgente3{AllegatoID: s.Sorgente.AllegatoID, Sha256: s.Sorgente.Sha256, Gruppo: s.Sorgente.Gruppo}
	}
	// le caselle: un gruppo solo (quello della sorgente), lo sha256 della sorgente, e l'allegato da completare
	var piuGruppi, fuoriSorgente, senzaOccorrenza, ambigui []string
	allegatoPerSha := map[string][]uuid.UUID{}
	allegatoDi := func(sha string) ([]uuid.UUID, error) {
		if a, ok := allegatoPerSha[sha]; ok {
			return a, nil
		}
		a, err := q.ListAllegatiAttiviPerSha(ctx, db.ListAllegatiAttiviPerShaParams{ThreadID: thread, Sha256: pgText(sha)})
		if err != nil {
			return nil, err
		}
		allegatoPerSha[sha] = a
		return a, nil
	}
	for _, n := range s.Nodi {
		x := Nodo3{ChiaveBozza: n.ChiaveBozza, ComponenteID: n.ComponenteID, Op: n.Op, Tipo: n.Tipo, Descrizione: n.Descrizione, Origine: n.Origine,
			Identita: Identita{Emittente: cliente, EmittentePresunto: true, Codice: n.Codice, Rev: n.Rev, Modo: ModoFormato2}}
		for _, c := range n.Caselle {
			if sorgente == nil || c.Gruppo != sorgente.Gruppo {
				piuGruppi = append(piuGruppi, n.ChiaveBozza)
				break
			}
			sha, _ := shaEChiave(c.Rif)
			var all uuid.UUID
			switch {
			case sorgente.AllegatoID != nil && sorgente.Sha256 == sha:
				all = *sorgente.AllegatoID
			case sorgente.AllegatoID != nil:
				fuoriSorgente = append(fuoriSorgente, n.ChiaveBozza)
				continue
			default:
				aa, err := allegatoDi(sha)
				if err != nil {
					return out, tr, err
				}
				switch len(aa) {
				case 0:
					senzaOccorrenza = append(senzaOccorrenza, n.ChiaveBozza)
					continue
				case 1:
					all = aa[0]
				default:
					ambigui = append(ambigui, n.ChiaveBozza)
					continue
				}
			}
			x.Caselle = append(x.Caselle, Casella3{AllegatoID: all, Gruppo: c.Gruppo, Rif: c.Rif})
		}
		// un nodo con origine step senza caselle (il formato 2 lo ammette, il 3 no) resta dell'ingegnere, con la traccia
		if x.Origine == OrigineStep && len(n.Caselle) == 0 {
			x.Origine = OrigineUtente
			tr.NodiSenzaCasella = append(tr.NodiSenzaCasella, n.ChiaveBozza)
		}
		out.Nodi = append(out.Nodi, x)
	}
	for _, e := range []struct {
		nodi   []string
		motivo string
		testo  string
	}{
		{piuGruppi, MotivoConversionePiuGruppi, "nodi con caselle di un gruppo diverso da quello della sorgente (il formato 3 ha un gruppo solo)"},
		{fuoriSorgente, MotivoConversioneFuoriSorgente, "nodi con caselle di uno sha256 diverso da quello della sorgente (nel formato 3 le caselle sono della sorgente)"},
		{senzaOccorrenza, MotivoConversioneSenzaOccorrenza, "nodi le cui caselle non hanno nessun allegato attivo della RFQ con quello sha256"},
		{ambigui, MotivoConversioneAmbigua, "nodi le cui caselle hanno piu' di un allegato attivo con quello sha256"},
	} {
		if len(e.nodi) > 0 {
			sort.Strings(e.nodi)
			return out, tr, corpoMotivo("converti_da", e.motivo, "%s: %s", e.testo, strings.Join(unici(e.nodi), ", "))
		}
	}
	// la sorgente senza allegato prende quello delle caselle, se e' uno solo
	if sorgente != nil && sorgente.AllegatoID == nil {
		var all *uuid.UUID
		sha := ""
		for _, n := range out.Nodi {
			for _, c := range n.Caselle {
				a := c.AllegatoID
				if all != nil && *all != a {
					return out, tr, corpoMotivo("converti_da", MotivoConversioneAmbigua, "le caselle stanno su piu' allegati: la sorgente non e' una")
				}
				all = &a
				sha, _ = shaEChiave(c.Rif)
			}
		}
		if all != nil {
			sorgente.AllegatoID, sorgente.Sha256 = all, sha
		}
	}
	out.Sorgente = sorgente
	conCaselle := map[string]bool{}
	for _, n := range out.Nodi {
		if len(n.Caselle) > 0 {
			conCaselle[n.ChiaveBozza] = true
		}
	}
	for _, a := range s.Archi {
		x := Arco3{Padre: a.Padre, Figlio: a.Figlio, Qta: a.Qta, Origine: a.Origine, Posizione: a.Posizione}
		if a.Origine == OrigineStep {
			if sorgente != nil && sorgente.AllegatoID != nil && conCaselle[a.Padre] && conCaselle[a.Figlio] {
				x.Step = &StepArco3{AllegatoID: *sorgente.AllegatoID, Gruppo: sorgente.Gruppo}
			} else {
				x.Origine = OrigineUtente
				tr.ArchiSenzaStep = append(tr.ArchiSenzaStep, ArcoSenzaStep{Padre: a.Padre, Figlio: a.Figlio})
			}
		}
		out.Archi = append(out.Archi, x)
	}
	var nonConvertibili []string
	for i, raw := range s.Decisioni {
		campo := fmt.Sprintf("decisioni[%d]", i)
		r, completata := decisioneDelFormato2(raw, cliente)
		d, e := LeggiDecisione3(campo, r)
		if e != nil {
			nonConvertibili = append(nonConvertibili, fmt.Sprintf("%s (%s: %s)", idDecisione(raw), e.Campo, e.Messaggio))
			continue
		}
		if completata != nil {
			tr.DecisioniCompletate = append(tr.DecisioniCompletate, *completata)
		}
		out.Decisioni = append(out.Decisioni, d)
	}
	if len(nonConvertibili) > 0 {
		return out, tr, corpoMotivo("converti_da", MotivoConversioneDecisione,
			"decisioni della bozza formato 2 che non hanno la forma del formato 3: %s", strings.Join(nonConvertibili, "; "))
	}
	pre, segnalate, err := traduciPreassociazioni(ctx, q, thread, s.DocumentiPreassociati)
	if err != nil {
		return out, tr, err
	}
	out.DocumentiPreassociati = pre
	tr.PreassociazioniTradotte, tr.PreassociazioniSegnalate = len(pre), segnalate
	return out, tr, nil
}

// traduciPreassociazioni: le preassociazioni formato 2 {proposta_id, chiave_bozza} nella forma del formato 3
// {allegato_id, chiave_bozza}, con l'allegato della proposta letto nella transazione della conversione: l'unico punto di
// traduzione (commento di testa). Una proposta che non e' della RFQ ferma la conversione; un'occorrenza non attiva o di
// una natura che non si preassocia si copia e si segnala.
func traduciPreassociazioni(ctx context.Context, q *db.Queries, thread uuid.UUID, pp []Preassociazione2) ([]Preassociazione3, []PreassociazioneSegnalata, error) {
	segnalate := []PreassociazioneSegnalata{}
	if len(pp) == 0 {
		return nil, segnalate, nil
	}
	ids := make([]uuid.UUID, 0, len(pp))
	for _, p := range pp {
		ids = append(ids, p.PropostaID)
	}
	rr, err := q.ListProposteDellaRfq(ctx, db.ListProposteDellaRfqParams{ThreadID: uuid.NullUUID{UUID: thread, Valid: true}, Proposte: ids})
	if err != nil {
		return nil, nil, err
	}
	allegatoDi := make(map[uuid.UUID]uuid.UUID, len(rr))
	allegati := make([]uuid.UUID, 0, len(rr))
	for _, r := range rr {
		allegatoDi[r.PropostaID] = r.AllegatoID
		allegati = append(allegati, r.AllegatoID)
	}
	var mancanti []string
	for _, p := range pp {
		if _, ok := allegatoDi[p.PropostaID]; !ok {
			mancanti = append(mancanti, p.PropostaID.String())
		}
	}
	if len(mancanti) > 0 {
		sort.Strings(mancanti)
		return nil, nil, corpoMotivo("converti_da", MotivoConversionePropostaNonRisolta,
			"preassociazioni della bozza formato 2 con proposte che non sono di questa RFQ (non si traducono in un allegato): %s", strings.Join(mancanti, ", "))
	}
	stato, err := q.ListAllegatiPreassociabili(ctx, db.ListAllegatiPreassociabiliParams{ThreadID: thread, Allegati: allegati})
	if err != nil {
		return nil, nil, err
	}
	statoDi := make(map[uuid.UUID]db.ListAllegatiPreassociabiliRow, len(stato))
	for _, r := range stato {
		statoDi[r.AllegatoID] = r
	}
	out := make([]Preassociazione3, 0, len(pp))
	for _, p := range pp {
		a := allegatoDi[p.PropostaID]
		// direttiva 39 §4 (D-PB03 A [U]): la PB del formato 2 l'ha messa l'ingegnere nella pagina di prima, senza un
		// registro d'importazione: manuale [T-39] (la pre-assegnazione legacy aperta e' un'altra cosa: conversionep1.go)
		out = append(out, Preassociazione3{AllegatoID: a, ChiaveBozza: p.ChiaveBozza, Provenienza: ProvenienzaManuale})
		motivo := ""
		switch r := statoDi[a]; {
		case !r.Preassociabile:
			motivo = PreassociazioneNaturaNo
		case r.LegatoComeSorgente:
			motivo = PreassociazioneLegatoComeSorgente
		case !r.Attivo:
			motivo = PreassociazioneFileNonAttivo
		}
		if motivo != "" {
			segnalate = append(segnalate, PreassociazioneSegnalata{PropostaID: p.PropostaID, AllegatoID: a, ChiaveBozza: p.ChiaveBozza, Motivo: motivo})
		}
	}
	sort.Slice(segnalate, func(i, j int) bool { return segnalate[i].AllegatoID.String() < segnalate[j].AllegatoID.String() })
	return out, segnalate, nil
}

// decisioneDelFormato2: una decisione del formato 2 nella forma del formato 3. La ricodifica del formato 2 (contratto
// rev. 3 §3.3: componente_id, codice_prima, codice_dopo, motivo) non ha emittente_dopo, che il formato 3 vuole (EM §3.6
// riga 10): si completa con il cliente della RFQ, come l'identita' formato_2 dei nodi (proposta [T]); il resto si copia.
func decisioneDelFormato2(raw json.RawMessage, cliente string) (json.RawMessage, *DecisioneCompletata) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return raw, nil
	}
	var tipo string
	_ = json.Unmarshal(m["tipo"], &tipo)
	if tipo != DecisioneRicodifica || presenteValore(m["emittente_dopo"]) {
		return raw, nil
	}
	b, _ := json.Marshal(cliente)
	m["emittente_dopo"] = b
	return canonico(m), &DecisioneCompletata{ID: idDecisione(raw), Campo: "emittente_dopo"}
}

func unici(xx []string) []string {
	out := make([]string, 0, len(xx))
	for i, x := range xx {
		if i == 0 || x != xx[i-1] {
			out = append(out, x)
		}
	}
	return out
}

func pgText(s string) pgtype.Text { return testo(s) }
