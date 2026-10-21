// Package associazioni e' il servizio unico delle associazioni registrate (AR) file → componente (direttiva 35 §2, §3
// [U]; piano delle dipendenze della direttiva 35, §3.6, blocco C-1c [T]): l'unico scrittore di file_rfq_associazione, del
// suo giornale in associazione_evento e della copia derivata di compatibilita' documento_proposta.componente_id (con il
// suo codice), sempre nella stessa transazione.
//
// Che cosa e' una AR (direttiva 35 §1 [U]): l'associazione di un'occorrenza (RFQ, allegato) a un componente della working,
// applicata da un atto autorizzato. Non e' una proposta del Motore A (PA), non e' una preassociazione di una bozza (PB),
// non e' il legame dello STEP sorgente (§3: distinguerlo; vedi sorgente.go), non e' un documento confermato.
//
// Lo stato di una riga e' attiva | chiusa (base [T] del reviewer adottata dall'orchestratore): dice solo se la relazione
// registrata e' quella corrente. Una riga si chiude una volta sola (chi, quando, motivo) e resta; riassociare e'
// sempre una riga nuova; confermare di nuovo senza cambiamenti non tocca le righe (la conferma d'origine resta in
// conferma_id). Le letture della working usano solo le righe attive; associazione_evento e' il giornale.
//
// PRECONDIZIONE SUI LUCCHETTI (piano §3.6, INC-G25): il servizio non prende lucchetti propri. Chi chiama ha gia' bloccato
// la RFQ (BloccaThread FOR UPDATE, oppure BloccaThreadCondiviso FOR KEY SHARE) prima di qualunque proposta, con l'ordine
// fisso RFQ → proposta → AR. Due gesti che tengono la RFQ solo FOR KEY SHARE non si serializzano fra loro: il secondo
// trova la riga gia' chiusa (o l'indice ux_fra_attiva_allegato lo ferma con un 23505), mai in silenzio.
//
// LE DECISIONI (direttiva 39; [U] parola dell'utente, [T-39] scelta tecnica della delega; variante 20261019d della 0023,
// che e' la 20261019c con la copia derivata accesa):
//   - D-AR01 A [U]: una AR di R5 nasce dalla conferma del wizard: Registra, Sostituisci e Riassocia vogliono la conferma
//     formato 3 della stessa transazione (Atto.Conferma) e la PB applicata con la sua provenienza (D-PB03 A [U]), che il
//     database verifica contro lo snapshot confermato (provenienza_pb, importazione_id);
//   - D-AR03 D [U]: nelle RFQ interamente legacy i gesti atomici passano da qui con l'origine gesto_legacy
//     (RegistraLegacy, SostituisciLegacy; chiusura sgancio_legacy); nelle RFQ formato 3 (RFQFormato3) non scrivono AR;
//   - D-AR04 [U]: nessun backfill di AR (niente RegistraBackfill);
//   - D-ASS-04 A [T-39]: una sola AR attiva per allegato (ux_fra_attiva_allegato; la copia ha un solo valore);
//   - D-AR07 A [T-39]: il blocco del documento confermato dello stesso contenuto su un altro componente;
//   - D-AR02 A [U]: il legame della sorgente fuori dalle AR (LegaSorgente, scritto e non cablato: S8 non lo usa).
//
// Gli scrittori legacy della copia passano dal servizio in C-6b (corsia file); fino ad allora trg_proposta_copia_derivata
// resta spento.
package associazioni

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// Gli stati di una riga (ck_fra_stato).
const (
	StatoAttiva = "attiva"
	StatoChiusa = "chiusa"
)

// Le origini di una AR (ck_fra_origine): R5 applica una PB con la conferma formato 3 (D-AR01 A [U]); un gesto atomico di
// una RFQ interamente legacy (D-AR03 D [U]).
const (
	OrigineConfermaBOM = "conferma_bom"
	OrigineGestoLegacy = "gesto_legacy"
)

// Le provenienze di una PB (ck_fra_provenienza_pb; D-PB03 A [U]).
const (
	ProvenienzaMotore   = "motore"
	ProvenienzaCorretta = "corretta"
	ProvenienzaManuale  = "manuale"
	ProvenienzaLegacy   = "legacy"
)

var provenienzeAmmesse = map[string]bool{ProvenienzaMotore: true, ProvenienzaCorretta: true, ProvenienzaManuale: true, ProvenienzaLegacy: true}

// I motivi di chiusura (ck_fra_chiusura).
const (
	// MotivoSvincoloConferma: R5 applica la decisione scollega_documento della bozza (con la conferma).
	MotivoSvincoloConferma = "svincolo_conferma"
	// MotivoSostituita: la riga e' rimpiazzata da un'altra dello stesso allegato su un altro componente (Sostituisci).
	MotivoSostituita = "sostituita"
	// MotivoScollegaFile: «Scollega e scarta», «Scollega e metti in Altri file» (V1-Q22, V1-Q42 [U]).
	MotivoScollegaFile = "scollega_file"
	// MotivoFileGenerale: «Documentazione generale» del Fascicolo legacy.
	MotivoFileGenerale = "file_generale"
	// MotivoScartoLegacy: il vecchio «Scarta il file».
	MotivoScartoLegacy = "scarto_legacy"
	// MotivoSgancioLegacy: «Togli» di una RFQ interamente legacy (D-AR03 D [U]); il database lo rifiuta in una RFQ
	// formato 3.
	MotivoSgancioLegacy = "sgancio_legacy"
)

// Le azioni del giornale che il servizio scrive (ck_associazione_azione).
const (
	AzionePreassociazione = "preassociazione"
	AzioneSostituisci     = "sostituisci"
	AzioneSgancia         = "sgancia"
	AzioneAssegna         = "assegna"
)

// Le origini degli eventi del giornale (ck_associazione_origine). conversione_legacy (direttiva 39, D-AR14 A [T-39]) e'
// la conversione di una pre-assegnazione aperta alla presa in carico del prodotto, l'unica con l'autore nullo.
const (
	OrigineEventoConfermaBOM       = "conferma_bom"
	OrigineEventoPonte             = "ponte"
	OrigineEventoLegacy            = "legacy"
	OrigineEventoConversioneLegacy = "conversione_legacy"
)

// I codici dei rifiuti del servizio. Sono condizioni di dominio che chi chiama traduce (R4 li mostra prima con Piano,
// R5 li riceve solo se qualcosa e' cambiato sotto). I nomi gia_associata_altrove e documento_confermato_altrove sono
// quelli del motivo di PREASSOCIAZIONE_NON_APPLICABILE del piano (§3.6, §7.2 C-1a); gli altri sono [T] di questa corsia.
const (
	CodiceAttoNonValido             = "atto_non_valido"
	CodiceConfermaObbligatoria      = "conferma_obbligatoria"
	CodiceGiaAssociataAltrove       = "gia_associata_altrove"
	CodiceDocumentoConfermatoAltrui = "documento_confermato_altrove"
	CodiceComponenteNonDellaRfq     = "componente_non_della_rfq"
	CodiceComponenteArchiviato      = "componente_archiviato"
	CodiceNessunaAssociazioneAttiva = "nessuna_associazione_attiva"
	CodiceAssociazioneDiversa       = "associazione_diversa"
	CodiceRiassociazioneSenzaStoria = "riassociazione_senza_storia"
	CodiceMotivoNonAmmesso          = "motivo_non_ammesso"
	CodiceVincolo                   = "vincolo_associazione"
	CodiceBomCongelata              = "bom_congelata"
	// CodiceRFQFormato3: un gesto legacy in una RFQ formato 3 (D-AR03 D [U]): cambia la PB della bozza con R2.
	CodiceRFQFormato3 = "rfq_formato3"
	// CodiceProvenienzaNonValida: la PB applicata da R5 senza una provenienza ammessa, o con un registro d'origine e una
	// provenienza manuale o legacy (D-PB03 A [U]).
	CodiceProvenienzaNonValida = "provenienza_non_valida"
)

// Rifiuto e' un rifiuto di dominio del servizio. Con CodiceVincolo il database ha fermato la scrittura (P4X10, 23503,
// 23505): la transazione e' annullata e chi chiama la chiude.
type Rifiuto struct {
	Codice    string
	Messaggio string
	// Altrove: per gia_associata_altrove, il componente che tiene gia' il file (dalla AR attiva o dalla copia).
	Altrove uuid.NullUUID
}

func (r *Rifiuto) Error() string { return r.Codice + ": " + r.Messaggio }

func rifiuto(codice, formato string, args ...any) *Rifiuto {
	return &Rifiuto{Codice: codice, Messaggio: fmt.Sprintf(formato, args...)}
}

// Atto e' chi scrive e con quale atto: la RFQ, l'autore, la chiave d'idempotenza dell'atto (per R5 quella della
// conferma), la conferma formato 3 della stessa transazione quando l'atto e' R5, e una nota facoltativa.
type Atto struct {
	Thread   uuid.UUID
	Autore   uuid.UUID
	Chiave   uuid.UUID
	Conferma uuid.NullUUID
	Nota     string
}

func (a Atto) valida() error {
	switch {
	case a.Thread == uuid.Nil:
		return rifiuto(CodiceAttoNonValido, "manca la RFQ")
	case a.Autore == uuid.Nil:
		return rifiuto(CodiceAttoNonValido, "manca l'autore: una AR non nasce e non si chiude senza chi")
	case a.Chiave == uuid.Nil:
		return rifiuto(CodiceAttoNonValido, "manca la chiave d'idempotenza dell'atto")
	case len([]rune(a.Nota)) > 1000:
		return rifiuto(CodiceAttoNonValido, "la nota supera 1000 caratteri")
	}
	return nil
}

// Associazione e' una riga di file_rfq_associazione.
type Associazione struct {
	ID                uuid.UUID
	Thread            uuid.UUID
	Allegato          uuid.UUID
	Componente        uuid.UUID
	Stato             string
	Origine           string
	Conferma          uuid.NullUUID
	ChiaveBozza       string
	ProvenienzaPB     string
	Importazione      uuid.NullUUID
	Sostituisce       uuid.NullUUID
	Motivo            string
	ChiaveIdempotenza uuid.UUID
	CreataDa          uuid.UUID
	CreataIl          time.Time
	ChiusaIl          *time.Time
	ChiusaDa          uuid.NullUUID
	MotivoChiusura    string
	ChiusaConferma    uuid.NullUUID
	ChiusaChiave      uuid.NullUUID
	ChiusaNota        string
}

// Attiva dice se la riga e' la relazione registrata corrente.
func (x Associazione) Attiva() bool { return x.Stato == StatoAttiva }

func daRiga(r db.FileRfqAssociazione) Associazione {
	return Associazione{ID: r.AssociazioneID, Thread: r.ThreadID, Allegato: r.AllegatoID, Componente: r.ComponenteID, Stato: r.Stato,
		Origine: r.Origine, Conferma: r.ConfermaID, ChiaveBozza: r.ChiaveBozza.String, ProvenienzaPB: r.ProvenienzaPb.String,
		Importazione: r.ImportazioneID, Sostituisce: r.SostituisceID, Motivo: r.Motivo.String,
		ChiaveIdempotenza: r.ChiaveIdempotenza, CreataDa: r.CreataDa, CreataIl: r.CreataIl, ChiusaIl: r.ChiusaIl, ChiusaDa: r.ChiusaDa,
		MotivoChiusura: r.MotivoChiusura.String, ChiusaConferma: r.ChiusaConfermaID, ChiusaChiave: r.ChiusaChiave, ChiusaNota: r.ChiusaNota.String}
}

func dalleRighe(rr []db.FileRfqAssociazione) []Associazione {
	out := make([]Associazione, 0, len(rr))
	for _, r := range rr {
		out = append(out, daRiga(r))
	}
	return out
}

// ------------------------------------------------------------------ letture

// Attiva e' la AR attiva dell'allegato nella RFQ, o nil (provvisorio, base [T] D-ASS-04 A: al piu' una).
func Attiva(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) (*Associazione, error) {
	rr, err := q.ListAssociazioniAttiveDiAllegato(ctx, db.ListAssociazioniAttiveDiAllegatoParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil || len(rr) == 0 {
		return nil, err
	}
	x := daRiga(rr[0])
	return &x, nil
}

// AttiveDellaRfq sono le AR attive della RFQ, per allegato e componente.
func AttiveDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]Associazione, error) {
	rr, err := q.ListAssociazioniAttiveDellaRfq(ctx, thread)
	if err != nil {
		return nil, err
	}
	return dalleRighe(rr), nil
}

// StoriaDellAllegato sono tutte le AR dell'allegato nella RFQ, attive e chiuse, dalla prima.
func StoriaDellAllegato(ctx context.Context, q *db.Queries, thread, allegato uuid.UUID) ([]Associazione, error) {
	rr, err := q.ListStoriaAssociazioniDiAllegato(ctx, db.ListStoriaAssociazioniDiAllegatoParams{ThreadID: thread, AllegatoID: allegato})
	if err != nil {
		return nil, err
	}
	return dalleRighe(rr), nil
}

// ------------------------------------------------------------------ scritture: nascita

// Registrazione e' la PB che l'atto applica: l'allegato, il componente risolto dalla mappa, il nodo della bozza, la
// provenienza della PB e il registro d'importazione d'origine (solo con motore o corretta), copiati dallo snapshot
// confermato (D-PB03 A [U]; il database li verifica al COMMIT, invariante (b) di trg_fra_invarianti).
type Registrazione struct {
	Allegato     uuid.UUID
	Componente   uuid.UUID
	ChiaveBozza  string
	Provenienza  string
	Importazione uuid.NullUUID
}

func (r Registrazione) valida() error {
	switch {
	case r.Allegato == uuid.Nil:
		return rifiuto(CodiceAttoNonValido, "manca l'allegato")
	case r.Componente == uuid.Nil:
		return rifiuto(CodiceAttoNonValido, "manca il componente")
	case r.ChiaveBozza == "":
		return rifiuto(CodiceAttoNonValido, "manca il nodo della bozza (chiave_bozza) della preassociazione applicata")
	case !provenienzeAmmesse[r.Provenienza]:
		return rifiuto(CodiceProvenienzaNonValida, "la preassociazione applicata vuole la sua provenienza (motore, corretta, manuale, legacy), non %q", r.Provenienza)
	case r.Importazione.Valid && r.Provenienza != ProvenienzaMotore && r.Provenienza != ProvenienzaCorretta:
		return rifiuto(CodiceProvenienzaNonValida, "il registro d'importazione d'origine vale solo per una preassociazione motore o corretta, non %s", r.Provenienza)
	}
	return nil
}

// Esito e' il risultato di Registra, Sostituisci e Riassocia.
type Esito struct {
	// Associazione: la riga attiva dopo l'atto (nata ora, gia' nata con la stessa chiave, o gia' attiva).
	Associazione Associazione
	// Ripetuto: lo stesso atto (stessa chiave) aveva gia' fatto nascere questa riga: nessuna scrittura.
	Ripetuto bool
	// Invariata: la stessa coppia era gia' attiva da un atto precedente: confermare di nuovo non tocca la riga.
	Invariata bool
	// Chiusa: con Sostituisci, la riga chiusa come «sostituita».
	Chiusa *Associazione
	// Precedente: con Riassocia, l'ultima riga chiusa dell'allegato prima di questa.
	Precedente *Associazione
	// Copia: la proposta aperta dell'allegato la cui copia ora dice il componente della AR (nessuna senza proposta
	// aperta: i lettori legacy restano ciechi su quella AR in modo dichiarato, INC-G24).
	Copia uuid.NullUUID
	// EventoID: l'evento del giornale scritto (0 senza scrittura).
	EventoID int64
}

// Registra fa nascere la AR (allegato → componente) con l'atto: la riga, il suo evento «preassociazione» e la copia
// della proposta aperta, nella transazione del chiamante. Provvisorio, base [T] D-AR01 A: l'atto e' R5, con la conferma.
//
// Idempotenza (direttiva 35 §2.4): con la stessa chiave la stessa coppia non rinasce (Ripetuto), anche se nel frattempo
// e' stata chiusa; la stessa coppia gia' attiva da un atto precedente resta com'e' (Invariata). Un'altra AR attiva
// dell'allegato, o una copia senza AR su un altro componente (legame della sorgente, pre-assegnazione legacy), e' il
// rifiuto gia_associata_altrove: la sostituzione e' sempre esplicita (Sostituisci).
func Registra(ctx context.Context, q *db.Queries, a Atto, r Registrazione) (Esito, error) {
	if err := controllaNascita(a, r); err != nil {
		return Esito{}, err
	}
	if e, ok, err := ripetuto(ctx, q, a, r); err != nil || ok {
		return e, err
	}
	att, err := Attiva(ctx, q, a.Thread, r.Allegato)
	if err != nil {
		return Esito{}, err
	}
	if att != nil {
		if att.Componente == r.Componente {
			return Esito{Associazione: *att, Invariata: true}, nil
		}
		return Esito{}, altrove(r.Allegato, att.Componente, "una AR attiva")
	}
	return nasce(ctx, q, a, r, nil, false)
}

// Riassocia fa nascere una AR nuova per un allegato che ne ha avute (tutte chiuse): e' Registra, con la riga chiusa piu'
// recente in Esito.Precedente. La riga chiusa non si riapre («Riattiva» non riapre una AR chiusa): ne nasce un'altra.
func Riassocia(ctx context.Context, q *db.Queries, a Atto, r Registrazione) (Esito, error) {
	if err := controllaNascita(a, r); err != nil {
		return Esito{}, err
	}
	storia, err := StoriaDellAllegato(ctx, q, a.Thread, r.Allegato)
	if err != nil {
		return Esito{}, err
	}
	var prec *Associazione
	for i := range storia {
		if !storia[i].Attiva() {
			x := storia[i]
			prec = &x
		}
	}
	if prec == nil {
		return Esito{}, rifiuto(CodiceRiassociazioneSenzaStoria, "l'allegato %s non ha associazioni chiuse da cui ripartire: e' una registrazione", r.Allegato)
	}
	e, err := Registra(ctx, q, a, r)
	if err != nil {
		return e, err
	}
	if !e.Invariata && e.Associazione.ID != prec.ID {
		e.Precedente = prec
	}
	return e, nil
}

// Sostituisci chiude la AR attiva dell'allegato come «sostituita» e fa nascere quella sul componente nuovo, con un solo
// evento «sostituisci» che le nomina tutte e due, e sposta la copia. Mai silenziosa (direttiva 35 §8 C-5 [U]): senza una
// AR attiva e' nessuna_associazione_attiva (si registra); sullo stesso componente non cambia niente (Invariata).
func Sostituisci(ctx context.Context, q *db.Queries, a Atto, r Registrazione) (Esito, error) {
	if err := controllaNascita(a, r); err != nil {
		return Esito{}, err
	}
	if e, ok, err := ripetuto(ctx, q, a, r); err != nil || ok {
		return e, err
	}
	att, err := Attiva(ctx, q, a.Thread, r.Allegato)
	if err != nil {
		return Esito{}, err
	}
	if att == nil {
		return Esito{}, rifiuto(CodiceNessunaAssociazioneAttiva, "l'allegato %s non ha una AR attiva da sostituire", r.Allegato)
	}
	if att.Componente == r.Componente {
		return Esito{Associazione: *att, Invariata: true}, nil
	}
	return nasce(ctx, q, a, r, att, false)
}

// RegistraLegacy fa nascere la AR di un gesto atomico di una RFQ interamente legacy (D-AR03 D [U]; origine
// gesto_legacy): l'«Assegna» del Fascicolo o della vecchia pagina del Motore A quando la RFQ non e' formato 3. Senza
// conferma, senza nodo della bozza e senza provenienza; l'evento e' un «assegna» con l'origine legacy, la copia segue la
// AR. In una RFQ formato 3 il rifiuto rfq_formato3: il gesto cambia la PB della bozza con R2 (e il database rifiuta
// comunque la riga). Il resto come Registra: la stessa chiave non scrive di nuovo (Ripetuto), la stessa coppia gia'
// attiva resta (Invariata), un'altra AR attiva dell'allegato e' gia_associata_altrove (lo spostamento e'
// SostituisciLegacy, esplicito).
func RegistraLegacy(ctx context.Context, q *db.Queries, a Atto, allegato, componente uuid.UUID) (Esito, error) {
	r, err := controllaLegacy(ctx, q, a, allegato, componente)
	if err != nil {
		return Esito{}, err
	}
	if e, ok, err := ripetuto(ctx, q, a, r); err != nil || ok {
		return e, err
	}
	att, err := Attiva(ctx, q, a.Thread, r.Allegato)
	if err != nil {
		return Esito{}, err
	}
	if att != nil {
		if att.Componente == r.Componente {
			return Esito{Associazione: *att, Invariata: true}, nil
		}
		return Esito{}, altrove(r.Allegato, att.Componente, "una AR attiva")
	}
	return nasce(ctx, q, a, r, nil, true)
}

// SostituisciLegacy: come Sostituisci, per un gesto atomico di una RFQ interamente legacy (origine gesto_legacy, la
// riga di prima chiusa «sostituita» senza conferma, un evento «sostituisci» con l'origine legacy).
func SostituisciLegacy(ctx context.Context, q *db.Queries, a Atto, allegato, componente uuid.UUID) (Esito, error) {
	r, err := controllaLegacy(ctx, q, a, allegato, componente)
	if err != nil {
		return Esito{}, err
	}
	if e, ok, err := ripetuto(ctx, q, a, r); err != nil || ok {
		return e, err
	}
	att, err := Attiva(ctx, q, a.Thread, r.Allegato)
	if err != nil {
		return Esito{}, err
	}
	if att == nil {
		return Esito{}, rifiuto(CodiceNessunaAssociazioneAttiva, "l'allegato %s non ha una AR attiva da sostituire", r.Allegato)
	}
	if att.Componente == r.Componente {
		return Esito{Associazione: *att, Invariata: true}, nil
	}
	return nasce(ctx, q, a, r, att, true)
}

// controllaLegacy: l'atto senza conferma, l'allegato e il componente, la RFQ interamente legacy.
func controllaLegacy(ctx context.Context, q *db.Queries, a Atto, allegato, componente uuid.UUID) (Registrazione, error) {
	if err := a.valida(); err != nil {
		return Registrazione{}, err
	}
	switch {
	case a.Conferma.Valid:
		return Registrazione{}, rifiuto(CodiceAttoNonValido, "un gesto legacy non ha una conferma: con la conferma l'atto e' R5 (Registra)")
	case allegato == uuid.Nil:
		return Registrazione{}, rifiuto(CodiceAttoNonValido, "manca l'allegato")
	case componente == uuid.Nil:
		return Registrazione{}, rifiuto(CodiceAttoNonValido, "manca il componente")
	}
	f3, err := RFQFormato3(ctx, q, a.Thread)
	if err != nil {
		return Registrazione{}, err
	}
	if f3 {
		return Registrazione{}, rifiuto(CodiceRFQFormato3,
			"la RFQ e' formato 3: il gesto non scrive associazioni registrate, cambia la preassociazione della bozza con R2 (D-AR03 D)")
	}
	return Registrazione{Allegato: allegato, Componente: componente}, nil
}

func controllaNascita(a Atto, r Registrazione) error {
	if err := a.valida(); err != nil {
		return err
	}
	if err := r.valida(); err != nil {
		return err
	}
	if !a.Conferma.Valid {
		return rifiuto(CodiceConfermaObbligatoria,
			"una AR di R5 nasce solo dalla conferma formato 3 della stessa transazione (D-AR01 A); un gesto legacy passa da RegistraLegacy")
	}
	return nil
}

// ripetuto: la stessa chiave ha gia' fatto nascere questa coppia (anche se poi chiusa), o ha fatto nascere una AR dello
// stesso allegato su un altro componente che e' ancora attiva (lo stesso atto non associa lo stesso file due volte).
func ripetuto(ctx context.Context, q *db.Queries, a Atto, r Registrazione) (Esito, bool, error) {
	rr, err := q.ListAssociazioniPerChiave(ctx, db.ListAssociazioniPerChiaveParams{ThreadID: a.Thread, ChiaveIdempotenza: a.Chiave, AllegatoID: r.Allegato})
	if err != nil {
		return Esito{}, false, err
	}
	for _, x := range rr {
		if x.ComponenteID == r.Componente {
			return Esito{Associazione: daRiga(x), Ripetuto: true}, true, nil
		}
	}
	return Esito{}, false, nil
}

func altrove(allegato, componente uuid.UUID, che string) *Rifiuto {
	return &Rifiuto{Codice: CodiceGiaAssociataAltrove, Altrove: uuid.NullUUID{UUID: componente, Valid: true},
		Messaggio: fmt.Sprintf("l'allegato %s e' gia' legato al componente %s (%s): la sostituzione e' esplicita", allegato, componente, che)}
}

// nasce scrive la riga (dopo aver chiuso quella di prima, con Sostituisci), l'evento e la copia. legacy: l'origine
// gesto_legacy (RegistraLegacy, SostituisciLegacy).
func nasce(ctx context.Context, q *db.Queries, a Atto, r Registrazione, prima *Associazione, legacy bool) (Esito, error) {
	c, err := q.GetComponenteDellaRfqPerAR(ctx, db.GetComponenteDellaRfqPerARParams{ComponenteID: r.Componente, ThreadID: a.Thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return Esito{}, rifiuto(CodiceComponenteNonDellaRfq, "il componente %s non e' della RFQ %s", r.Componente, a.Thread)
	}
	if err != nil {
		return Esito{}, err
	}
	if c.ArchiviatoIl != nil {
		return Esito{}, rifiuto(CodiceComponenteArchiviato, "il componente %s (%s) e' archiviato: un file non vi si associa", c.Codice, r.Componente)
	}
	prop, err := propostaDi(ctx, q, r.Allegato)
	if err != nil {
		return Esito{}, err
	}
	// la copia senza AR su un altro componente (legame della sorgente, pre-assegnazione legacy, «Assegna» non passato
	// dal servizio): il punto 5 di trg_fra_nasce la fermerebbe; qui il rifiuto e' leggibile
	if prop != nil && prop.Stato == db.StatoPropostaAperta && prop.ComponenteID.Valid && prop.ComponenteID.UUID != r.Componente &&
		(prima == nil || prima.Componente != prop.ComponenteID.UUID) {
		return Esito{}, altrove(r.Allegato, prop.ComponenteID.UUID, "la copia della proposta, senza AR")
	}
	if altrui, err := q.EsisteDocumentoConfermatoAltrove(ctx, db.EsisteDocumentoConfermatoAltroveParams{ThreadID: a.Thread,
		AllegatoID: r.Allegato, ComponenteID: nuid(r.Componente)}); err != nil {
		return Esito{}, err
	} else if altrui {
		return Esito{}, rifiuto(CodiceDocumentoConfermatoAltrui,
			"il contenuto dell'allegato %s e' un documento confermato di un altro componente (provvisorio, base [T] D-AR07 A)", r.Allegato)
	}
	out := Esito{}
	if prima != nil {
		ch, err := q.ChiudiAssociazione(ctx, db.ChiudiAssociazioneParams{AssociazioneID: prima.ID, ThreadID: a.Thread,
			ChiusaDa: nuid(a.Autore), MotivoChiusura: testo(MotivoSostituita), ChiusaConfermaID: a.Conferma, ChiusaChiave: nuid(a.Chiave),
			ChiusaNota: testo(a.Nota)})
		if err != nil {
			return Esito{}, traduci(err)
		}
		x := daRiga(ch)
		out.Chiusa = &x
	}
	ins := db.InsertAssociazioneParams{ThreadID: a.Thread, AllegatoID: r.Allegato, ComponenteID: r.Componente, Origine: OrigineConfermaBOM,
		ConfermaID: a.Conferma, ChiaveBozza: testo(r.ChiaveBozza), ProvenienzaPb: testo(r.Provenienza), ImportazioneID: r.Importazione,
		Motivo: testo(a.Nota), ChiaveIdempotenza: a.Chiave, CreataDa: a.Autore}
	ev := db.InsertEventoAssociazioneParams{ThreadID: a.Thread, AllegatoID: nuid(r.Allegato), Azione: AzionePreassociazione,
		ComponenteDopo: nuid(r.Componente), ConfermaID: a.Conferma, Motivo: testo(a.Nota), Origine: OrigineEventoConfermaBOM,
		ChiaveIdempotenza: nuid(a.Chiave), AutoreID: a.Autore}
	if legacy {
		// D-AR03 D [U]: il gesto legacy, senza conferma, senza nodo e senza provenienza; la nascita e' un «assegna»
		ins.Origine, ins.ConfermaID, ins.ChiaveBozza, ins.ProvenienzaPb, ins.ImportazioneID =
			OrigineGestoLegacy, uuid.NullUUID{}, pgtype.Text{}, pgtype.Text{}, uuid.NullUUID{}
		ev.Azione, ev.ConfermaID, ev.Origine = AzioneAssegna, uuid.NullUUID{}, OrigineEventoLegacy
	}
	if prima != nil {
		ins.SostituisceID = nuid(prima.ID)
	}
	riga, err := q.InsertAssociazione(ctx, ins)
	if err != nil {
		return Esito{}, traduci(err)
	}
	out.Associazione = daRiga(riga)
	ev.AssociazioneID = nuid(riga.AssociazioneID)
	if prima != nil {
		ev.Azione, ev.AssociazionePrecedenteID, ev.ComponentePrima = AzioneSostituisci, nuid(prima.ID), nuid(prima.Componente)
	}
	if prop != nil {
		ev.PropostaID = nuid(prop.PropostaID)
	}
	if out.EventoID, err = q.InsertEventoAssociazione(ctx, ev); err != nil {
		return Esito{}, traduci(err)
	}
	ids, err := q.ScriviCopiaAssociazione(ctx, db.ScriviCopiaAssociazioneParams{AllegatoID: r.Allegato, ComponenteID: r.Componente, ThreadID: a.Thread})
	if err != nil {
		return Esito{}, traduci(err)
	}
	if len(ids) > 0 {
		out.Copia = nuid(ids[0])
	}
	return out, nil
}

// ------------------------------------------------------------------ scritture: chiusura

// Scollegamento e' la chiusura chiesta: l'allegato, il componente atteso (uuid.Nil = quello della AR attiva, qualunque) e
// il motivo.
type Scollegamento struct {
	Allegato   uuid.UUID
	Componente uuid.UUID
	Motivo     string
}

// Gli esiti di Scollega.
const (
	// EsitoChiusa: la AR attiva e' chiusa ora (con la copia tolta).
	EsitoChiusa = "chiusa"
	// EsitoCopiaTolta: non c'era una AR, ma la proposta aperta aveva la copia (pre-assegnazione legacy): tolta, con un
	// evento «sgancia» senza riga (piano §3.6, «copia senza AR»).
	EsitoCopiaTolta = "copia_tolta"
	// EsitoSvincoloGiaChiuso: lo svincolo di R5 riguarda una AR gia' chiusa dalla corsia file dopo lo svincolo R2
	// (D-ASS-03 / V1-Q42 [U]): soddisfatto, nessuna scrittura (piano §3.6).
	EsitoSvincoloGiaChiuso = "svincolo_gia_chiuso"
	// EsitoNessunLegame: niente da scollegare (nessuna AR attiva, nessuna copia): nessuna scrittura.
	EsitoNessunLegame = "nessun_legame"
)

// EsitoChiusura e' il risultato di Scollega.
type EsitoChiusura struct {
	Esito string
	// Associazione: la riga chiusa (ora o, con Ripetuto e svincolo_gia_chiuso, prima).
	Associazione *Associazione
	// Ripetuto: la stessa chiave aveva gia' chiuso questa riga (o tolto questa copia): nessuna scrittura.
	Ripetuto bool
	// Copia: la proposta aperta a cui la copia e' stata tolta.
	Copia    uuid.NullUUID
	EventoID int64
}

// motiviDiChiusura: per ogni motivo, se vuole la conferma (R5) e l'origine dell'evento. svincolo_conferma e' di R5
// (origine conferma_bom); scollega_file e' della pagina nuova e della corsia file (origine ponte, come gli eventi di
// filerfq); file_generale, scarto_legacy e sgancio_legacy delle pagine legacy (origine legacy; sgancio_legacy solo in una
// RFQ interamente legacy, D-AR03 D [U]). sostituita la scrive solo Sostituisci.
var motiviDiChiusura = map[string]struct {
	conferma bool
	origine  string
}{
	MotivoSvincoloConferma: {true, OrigineEventoConfermaBOM},
	MotivoScollegaFile:     {false, OrigineEventoPonte},
	MotivoFileGenerale:     {false, OrigineEventoLegacy},
	MotivoScartoLegacy:     {false, OrigineEventoLegacy},
	MotivoSgancioLegacy:    {false, OrigineEventoLegacy},
}

// OrigineDelMotivo e' l'origine dell'evento «sgancia» scritto per un motivo di chiusura ("" se il motivo non si usa con
// Scollega).
func OrigineDelMotivo(motivo string) string { return motiviDiChiusura[motivo].origine }

// Scollega chiude la AR attiva dell'allegato con il motivo (la riga, l'evento «sgancia» e la copia tolta, nella
// transazione del chiamante). Senza AR attiva:
//   - svincolo_conferma: se l'ultima riga della coppia e' chiusa, svincolo_gia_chiuso (nessuna scrittura); altrimenti
//     nessuna_associazione_attiva;
//   - gli altri motivi: la copia senza AR (pre-assegnazione legacy) si toglie con un evento «sgancia» senza riga
//     (copia_tolta); senza copia, nessun_legame.
//
// Idempotenza: la stessa chiave che ha gia' chiuso la riga (o tolto la copia) per l'allegato non scrive di nuovo
// (Ripetuto). Con un componente atteso diverso da quello della AR attiva: associazione_diversa.
func Scollega(ctx context.Context, q *db.Queries, a Atto, s Scollegamento) (EsitoChiusura, error) {
	if err := a.valida(); err != nil {
		return EsitoChiusura{}, err
	}
	if s.Allegato == uuid.Nil {
		return EsitoChiusura{}, rifiuto(CodiceAttoNonValido, "manca l'allegato")
	}
	m, ok := motiviDiChiusura[s.Motivo]
	if !ok {
		return EsitoChiusura{}, rifiuto(CodiceMotivoNonAmmesso, "motivo di chiusura %q non ammesso (sostituita la scrive solo Sostituisci)", s.Motivo)
	}
	if m.conferma && s.Componente == uuid.Nil {
		return EsitoChiusura{}, rifiuto(CodiceAttoNonValido, "lo svincolo di R5 cita il componente della AR (scollega_documento {allegato_id, componente_id})")
	}
	if m.conferma != a.Conferma.Valid {
		if m.conferma {
			return EsitoChiusura{}, rifiuto(CodiceConfermaObbligatoria, "lo svincolo di R5 vuole la conferma formato 3 della transazione")
		}
		return EsitoChiusura{}, rifiuto(CodiceMotivoNonAmmesso, "il motivo %s non e' una conferma: senza conferma", s.Motivo)
	}
	if s.Motivo == MotivoSgancioLegacy {
		if f3, err := RFQFormato3(ctx, q, a.Thread); err != nil {
			return EsitoChiusura{}, err
		} else if f3 {
			return EsitoChiusura{}, rifiuto(CodiceRFQFormato3,
				"la RFQ e' formato 3: «Togli» cambia la preassociazione della bozza con R2, non chiude la AR (D-AR03 D)")
		}
	}
	// il reinvio della stessa chiusura
	chiuse, err := q.ListAssociazioniChiusePerChiave(ctx, db.ListAssociazioniChiusePerChiaveParams{ThreadID: a.Thread, ChiusaChiave: nuid(a.Chiave),
		AllegatoID: s.Allegato})
	if err != nil {
		return EsitoChiusura{}, err
	}
	for _, x := range chiuse {
		if s.Componente == uuid.Nil || x.ComponenteID == s.Componente {
			r := daRiga(x)
			return EsitoChiusura{Esito: EsitoChiusa, Associazione: &r, Ripetuto: true}, nil
		}
	}
	att, err := Attiva(ctx, q, a.Thread, s.Allegato)
	if err != nil {
		return EsitoChiusura{}, err
	}
	if att != nil && (s.Componente == uuid.Nil || att.Componente == s.Componente) {
		return chiudi(ctx, q, a, s, m.origine, *att)
	}
	if m.conferma {
		// lo svincolo e' soddisfatto se la coppia non e' piu' attiva e c'e' stata (anche se il file ha ora una AR su un
		// altro componente): e' il controllo di trg_conferma_associazioni
		storia, err := StoriaDellAllegato(ctx, q, a.Thread, s.Allegato)
		if err != nil {
			return EsitoChiusura{}, err
		}
		for i := len(storia) - 1; i >= 0; i-- {
			if storia[i].Componente == s.Componente && !storia[i].Attiva() {
				x := storia[i]
				return EsitoChiusura{Esito: EsitoSvincoloGiaChiuso, Associazione: &x}, nil
			}
		}
		return EsitoChiusura{}, rifiuto(CodiceNessunaAssociazioneAttiva, "l'allegato %s non ha una AR sul componente %s da svincolare", s.Allegato, s.Componente)
	}
	if att != nil {
		return EsitoChiusura{}, &Rifiuto{Codice: CodiceAssociazioneDiversa, Altrove: nuid(att.Componente),
			Messaggio: fmt.Sprintf("la AR attiva dell'allegato %s e' sul componente %s, non su %s", s.Allegato, att.Componente, s.Componente)}
	}
	return togliCopiaSenzaAR(ctx, q, a, s, m.origine)
}

func chiudi(ctx context.Context, q *db.Queries, a Atto, s Scollegamento, origine string, att Associazione) (EsitoChiusura, error) {
	p := db.ChiudiAssociazioneParams{AssociazioneID: att.ID, ThreadID: a.Thread, ChiusaDa: nuid(a.Autore), MotivoChiusura: testo(s.Motivo),
		ChiusaChiave: nuid(a.Chiave), ChiusaNota: testo(a.Nota)}
	if s.Motivo == MotivoSvincoloConferma {
		p.ChiusaConfermaID = a.Conferma
	}
	ch, err := q.ChiudiAssociazione(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		// chiusa da un altro gesto fra la lettura e la scrittura (due gesti FOR KEY SHARE non si serializzano fra loro)
		return EsitoChiusura{}, rifiuto(CodiceNessunaAssociazioneAttiva, "la AR %s dell'allegato %s non e' piu' attiva", att.ID, s.Allegato)
	}
	if err != nil {
		return EsitoChiusura{}, traduci(err)
	}
	x := daRiga(ch)
	out := EsitoChiusura{Esito: EsitoChiusa, Associazione: &x}
	prop, err := propostaDi(ctx, q, s.Allegato)
	if err != nil {
		return EsitoChiusura{}, err
	}
	ev := db.InsertEventoAssociazioneParams{ThreadID: a.Thread, AllegatoID: nuid(s.Allegato), AssociazioneID: nuid(att.ID), Azione: AzioneSgancia,
		ComponentePrima: nuid(att.Componente), Motivo: testo(a.Nota), Origine: origine, ChiaveIdempotenza: nuid(a.Chiave), AutoreID: a.Autore}
	if s.Motivo == MotivoSvincoloConferma {
		ev.ConfermaID = a.Conferma
	}
	if prop != nil {
		ev.PropostaID = nuid(prop.PropostaID)
	}
	if out.EventoID, err = q.InsertEventoAssociazione(ctx, ev); err != nil {
		return EsitoChiusura{}, traduci(err)
	}
	ids, err := q.TogliCopiaAssociazione(ctx, db.TogliCopiaAssociazioneParams{AllegatoID: s.Allegato, ComponenteID: nuid(att.Componente)})
	if err != nil {
		return EsitoChiusura{}, traduci(err)
	}
	if len(ids) > 0 {
		out.Copia = nuid(ids[0])
	}
	return out, nil
}

// ConvertiCopiaLegacy converte una pre-assegnazione legacy aperta (P1: la copia di documento_proposta su un componente,
// senza AR) alla presa in carico del prodotto (direttiva 39, D-AR14 A [T-39]; EM I.9), nella transazione di chi chiama:
// la copia si toglie con un evento «sgancia» con l'origine conversione_legacy e componente_prima, senza AR ne' conferma
// (ck_associazione_coerente). Nessuna AR di backfill: la PB legacy «da verificare» la scrive chi chiama nella versione 1
// della bozza. L'autore e' chi prende in carico il prodotto [T-39] (il nullo, ammesso solo qui, non serve). Idempotenza
// per chiave e allegato come Scollega. Con una AR attiva dell'allegato non e' una P1: associazione_diversa; senza la
// copia su quel componente: nessun_legame, nessuna scrittura.
func ConvertiCopiaLegacy(ctx context.Context, q *db.Queries, a Atto, allegato, componente uuid.UUID) (EsitoChiusura, error) {
	if err := a.valida(); err != nil {
		return EsitoChiusura{}, err
	}
	if allegato == uuid.Nil || componente == uuid.Nil {
		return EsitoChiusura{}, rifiuto(CodiceAttoNonValido, "la conversione cita l'allegato e il componente della copia")
	}
	if a.Conferma.Valid {
		return EsitoChiusura{}, rifiuto(CodiceMotivoNonAmmesso, "la conversione di una pre-assegnazione non e' una conferma")
	}
	att, err := Attiva(ctx, q, a.Thread, allegato)
	if err != nil {
		return EsitoChiusura{}, err
	}
	if att != nil {
		return EsitoChiusura{}, &Rifiuto{Codice: CodiceAssociazioneDiversa, Altrove: nuid(att.Componente),
			Messaggio: fmt.Sprintf("l'allegato %s ha una AR attiva sul componente %s: non e' una pre-assegnazione", allegato, att.Componente)}
	}
	return togliCopiaSenzaAR(ctx, q, a, Scollegamento{Allegato: allegato, Componente: componente}, OrigineEventoConversioneLegacy)
}

// togliCopiaSenzaAR: la proposta aperta ha la copia ma nessuna AR (pre-assegnazione legacy, INC-S2): la copia si toglie
// con un evento «sgancia» senza riga, cosi' anche la corsia file e «Documentazione generale» passano dal servizio prima
// di D-AR04 (piano §3.6).
func togliCopiaSenzaAR(ctx context.Context, q *db.Queries, a Atto, s Scollegamento, origine string) (EsitoChiusura, error) {
	ee, err := q.ListEventiAssociazionePerChiave(ctx, db.ListEventiAssociazionePerChiaveParams{ThreadID: a.Thread, ChiaveIdempotenza: nuid(a.Chiave),
		AllegatoID: nuid(s.Allegato)})
	if err != nil {
		return EsitoChiusura{}, err
	}
	for _, e := range ee {
		if e.Azione == AzioneSgancia && !e.AssociazioneID.Valid && (s.Componente == uuid.Nil || e.ComponentePrima.UUID == s.Componente) {
			return EsitoChiusura{Esito: EsitoCopiaTolta, Ripetuto: true, Copia: e.PropostaID, EventoID: e.EventoID}, nil
		}
	}
	prop, err := propostaDi(ctx, q, s.Allegato)
	if err != nil {
		return EsitoChiusura{}, err
	}
	if prop == nil || prop.Stato != db.StatoPropostaAperta || !prop.ComponenteID.Valid {
		return EsitoChiusura{Esito: EsitoNessunLegame}, nil
	}
	if s.Componente != uuid.Nil && prop.ComponenteID.UUID != s.Componente {
		return EsitoChiusura{}, &Rifiuto{Codice: CodiceAssociazioneDiversa, Altrove: prop.ComponenteID,
			Messaggio: fmt.Sprintf("la copia dell'allegato %s dice il componente %s, non %s", s.Allegato, prop.ComponenteID.UUID, s.Componente)}
	}
	ids, err := q.TogliCopiaAssociazione(ctx, db.TogliCopiaAssociazioneParams{AllegatoID: s.Allegato, ComponenteID: prop.ComponenteID})
	if err != nil {
		return EsitoChiusura{}, traduci(err)
	}
	out := EsitoChiusura{Esito: EsitoCopiaTolta}
	if len(ids) > 0 {
		out.Copia = nuid(ids[0])
	}
	if out.EventoID, err = q.InsertEventoAssociazione(ctx, db.InsertEventoAssociazioneParams{ThreadID: a.Thread, PropostaID: nuid(prop.PropostaID),
		AllegatoID: nuid(s.Allegato), Azione: AzioneSgancia, ComponentePrima: prop.ComponenteID, Motivo: testo(a.Nota), Origine: origine,
		ChiaveIdempotenza: nuid(a.Chiave), AutoreID: a.Autore}); err != nil {
		return EsitoChiusura{}, traduci(err)
	}
	return out, nil
}

// ------------------------------------------------------------------ la copia

// AllineaCodiceCopia: dopo la ricodifica del componente (D-ASS-01), le copie di quel componente prendono il suo codice
// corrente (la FK a tre campi). La AR non cambia. Chi ricodifica ha differito fk_proposta_componente nella transazione.
func AllineaCodiceCopia(ctx context.Context, q *db.Queries, thread, componente uuid.UUID) (int64, error) {
	return q.AllineaCodiceCopieDelComponente(ctx, db.AllineaCodiceCopieDelComponenteParams{ComponenteID: componente, ThreadID: thread})
}

// ------------------------------------------------------------------ aiuti

type proposta struct {
	PropostaID   uuid.UUID
	Stato        db.StatoProposta
	ComponenteID uuid.NullUUID
}

func propostaDi(ctx context.Context, q *db.Queries, allegato uuid.UUID) (*proposta, error) {
	rr, err := q.ListProposteDiAllegatoPerCopia(ctx, allegato)
	if err != nil || len(rr) == 0 {
		return nil, err
	}
	return &proposta{PropostaID: rr[0].PropostaID, Stato: rr[0].Stato, ComponenteID: rr[0].ComponenteID}, nil
}

// traduci: i rifiuti del database sulle AR diventano Rifiuto (la transazione e' comunque annullata). 23505 su
// ux_fra_attiva_allegato o ux_fra_attiva_coppia e' la corsa con un altro gesto: gia_associata_altrove.
func traduci(err error) error {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return err
	}
	switch {
	case pe.Code == "23505" && (pe.ConstraintName == "ux_fra_attiva_allegato" || pe.ConstraintName == "ux_fra_attiva_coppia"):
		return &Rifiuto{Codice: CodiceGiaAssociataAltrove, Messaggio: "un altro gesto ha registrato nel frattempo una AR attiva dello stesso allegato (" + pe.ConstraintName + ")"}
	case pe.Code == "BOM01":
		return &Rifiuto{Codice: CodiceBomCongelata, Messaggio: pe.Message}
	case pe.Code == "P4X10" || pe.Code == "23505" || pe.Code == "23503" || pe.Code == "23514":
		return &Rifiuto{Codice: CodiceVincolo, Messaggio: pe.Message}
	}
	return err
}

func nuid(u uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: u, Valid: true} }

func testo(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
