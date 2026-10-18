package distintabozza

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/confermabom"
)

// I codici degli errori del package, con i nomi del contratto backend A1d v2 rev. 3 (§6.2, §6.5, §6.7, §15.5-§15.7).
// Il transport (B2) li traduce in HTTP: i conflitti (EConflitto) in 409, la RFQ che non c'e' in 404, il resto in 422.
// Il catalogo dei 409 e' quello della matrice 07 (§5 n. 8): BOZZA_VERSIONE copre anche la seconda bozza aperta
// (motivo altra_bozza_aperta, Δ08), e BASE_SUPERATA in R2 vale solo per il rebase (§6.7). Le azioni che non tornano
// sono un CORPO_NON_VALIDO con motivo azioni_non_riconciliate (§6.6, §15.6). RFQ_INESISTENTE non e' nel catalogo
// della rev. 3, che dice solo «404 RFQ inesistente» (§0 n. 7, §15.7): il nome resta una proposta di B1.
const (
	// CodiceCorpoNonValido: lo snapshot, un'azione o la richiesta violano una regola di forma, oppure le azioni non
	// danno lo snapshot (Motivo azioni_non_riconciliate) o lo stato di partenza non si ricostruisce (Motivo
	// partenza_non_ricostruibile) (422).
	CodiceCorpoNonValido = "CORPO_NON_VALIDO"
	// CodiceFuoriRFQ: un prodotto, una bozza, una conferma o un allegato non sono della RFQ del percorso (422, mai 404:
	// non si rivela che esistono altrove; contratto §1.3).
	CodiceFuoriRFQ = "FUORI_RFQ"
	// CodiceRFQInesistente: la RFQ del percorso non c'e' (404).
	CodiceRFQInesistente = "RFQ_INESISTENTE"
	// CodiceBozzaVersione: la versione attesa non e' la corrente, la bozza e' chiusa, oppure c'e' gia' un'altra bozza
	// aperta per il prodotto (409; Motivo dice quale dei tre, Conflitto lo stato corrente).
	CodiceBozzaVersione = "BOZZA_VERSIONE"
	// CodiceBaseSuperata: un rebase porta una base che non e' quella corrente del prodotto (409, solo con il rebase:
	// contratto §6.7, §15.5; Motivi dice che cosa e' cambiato).
	CodiceBaseSuperata = "BASE_SUPERATA"
)

// I motivi (contratto rev. 3 §6.2, §6.6, §15.5, §15.6, §7.6).
const (
	MotivoAzioniNonRiconciliate    = "azioni_non_riconciliate"
	MotivoPartenzaNonRicostruibile = "partenza_non_ricostruibile"
	MotivoVersioneDiversa          = "versione_diversa"
	MotivoBozzaChiusa              = "bozza_chiusa"
	MotivoAltraBozzaAperta         = "altra_bozza_aperta"
	MotivoConfermaCambiata         = "conferma_cambiata"
	MotivoStrutturaCambiata        = "struttura_cambiata"
)

// Errore e' il rifiuto del package: un codice, il motivo (quando il contratto lo prevede), il campo (percorso JSON,
// quando c'e') e la frase per chi legge.
type Errore struct {
	Codice    string
	Motivo    string
	Campo     string
	Messaggio string
	// Motivi: per BASE_SUPERATA, che cosa e' cambiato (conferma_cambiata, struttura_cambiata).
	Motivi []string
	// Conflitto: per BOZZA_VERSIONE, lo stato corrente della bozza (versione, chi e quando). Nei due casi che il
	// database ferma da solo sotto i lucchetti (indice della bozza aperta, PK della versione) non c'e': la
	// transazione e' gia' annullata e non si rilegge niente (RC-15, motivato nel resoconto delle correzioni).
	Conflitto *Conflitto
	// VersioneCorrente: per BASE_SUPERATA del rebase del formato 3, la versione corrente del prodotto (EM §9.2).
	VersioneCorrente *confermabom.VersioneCorrente
	// AllegatoID: per gruppo_su_altra_occorrenza, l'allegato del gruppo (EM §6.1); per le forme che lo dicono.
	AllegatoID *uuid.UUID
}

// Conflitto descrive la bozza corrente quando un salvataggio o un abbandono arrivano su una versione superata.
type Conflitto struct {
	BozzaID      uuid.UUID
	Stato        string
	Versione     int
	AggiornataDa uuid.UUID
	AggiornataIl time.Time
}

func (e *Errore) Error() string {
	testa := e.Codice
	if e.Motivo != "" {
		testa += "/" + e.Motivo
	}
	if e.Campo != "" {
		return fmt.Sprintf("%s (%s): %s", testa, e.Campo, e.Messaggio)
	}
	return testa + ": " + e.Messaggio
}

// Is confronta il codice e, se il bersaglio ne ha uno, il motivo: errors.Is(err, ErrConflittoVersione) vale per ogni
// BOZZA_VERSIONE, errors.Is(err, ErrAltraBozzaAperta) solo per quella con motivo altra_bozza_aperta.
func (e *Errore) Is(target error) bool {
	t, ok := target.(*Errore)
	if e == nil || !ok || t == nil {
		return false
	}
	return t.Codice == e.Codice && (t.Motivo == "" || t.Motivo == e.Motivo)
}

// EConflitto dice se l'errore e' un conflitto con lo stato corrente (409 in B2), non un corpo sbagliato.
func (e *Errore) EConflitto() bool {
	return e.Codice == CodiceBozzaVersione || e.Codice == CodiceBaseSuperata
}

// Gli errori da confrontare con errors.Is.
var (
	ErrConflittoVersione        = &Errore{Codice: CodiceBozzaVersione}
	ErrAltraBozzaAperta         = &Errore{Codice: CodiceBozzaVersione, Motivo: MotivoAltraBozzaAperta}
	ErrBaseSuperata             = &Errore{Codice: CodiceBaseSuperata}
	ErrCorpoNonValido           = &Errore{Codice: CodiceCorpoNonValido}
	ErrAzioniNonRiconciliate    = &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoAzioniNonRiconciliate}
	ErrPartenzaNonRicostruibile = &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoPartenzaNonRicostruibile}
	ErrFuoriRFQ                 = &Errore{Codice: CodiceFuoriRFQ}
	ErrRFQInesistente           = &Errore{Codice: CodiceRFQInesistente}
)

// ErrSnapshotIncoerente: uno snapshot salvato non si rilegge o non ha piu' l'impronta registrata. Non e' un
// rifiuto del corpo (non e' un *Errore): e' un guasto dei dati salvati, che B2 tratta come errore interno (RC-01).
var ErrSnapshotIncoerente = errors.New("snapshot salvato non coerente con la sua impronta")

func corpo(campo, formato string, a ...any) *Errore {
	return &Errore{Codice: CodiceCorpoNonValido, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

func nonRiconciliate(campo, formato string, a ...any) *Errore {
	return &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoAzioniNonRiconciliate, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

func fuori(campo, formato string, a ...any) *Errore {
	return &Errore{Codice: CodiceFuoriRFQ, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}
