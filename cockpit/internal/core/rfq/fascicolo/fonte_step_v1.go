package fascicolo

// fonte_step_v1.go: la riga della storia della fonte STEP del prodotto (fonte_step_evento, ruolo prodotto) che «Autorizza»,
// «Revoca» e «Conferma la fonte» scrivono in V1, nella transazione dell'atto: un atto rifiutato non scrive niente (EM §2.3,
// §2.4, §9.10; PM23 R-24). E' l'unico cambiamento di quegli atti, e lo fa la corsia persistenza (V1, ondata B). R7 la
// mostra in fonte[].

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// Le azioni della storia della fonte che scrivono gli atti esistenti (ck_fonte_azione).
const (
	FonteAutorizzazione    = "autorizzazione"
	FonteRevoca            = "revoca"
	FonteConfermaDocumento = "conferma_documento"
)

// EventoFonte e' un evento della storia della fonte del prodotto scritto da un atto esistente.
type EventoFonte struct {
	Thread, Prodotto, Documento uuid.UUID
	// Allegato: l'occorrenza del file; uuid.Nil = il primo allegato da cui il documento e' arrivato.
	Allegato uuid.UUID
	Sha256   string
	Azione   string
	// Motivo: obbligatorio per la revoca (ck_fonte_motivo).
	Motivo string
	Autore uuid.UUID
}

// ScriviFonteProdotto scrive l'evento nella transazione di q. Senza un allegato (un documento che non e' arrivato da
// nessun allegato: in V1 ogni strada ne crea uno) l'evento non si scrive e l'atto va avanti: scritto e' falso (limite
// dichiarato nel resoconto della corsia).
func ScriviFonteProdotto(ctx context.Context, q *db.Queries, e EventoFonte) (scritto bool, err error) {
	if e.Allegato == uuid.Nil {
		a, err := q.GetAllegatoDelDocumento(ctx, db.GetAllegatoDelDocumentoParams{ThreadID: e.Thread, DocumentoID: e.Documento})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		e.Allegato = a
	}
	motivo := pgtype.Text{}
	if m := strings.TrimSpace(e.Motivo); m != "" {
		if r := []rune(m); len(r) > 1000 {
			m = string(r[:1000])
		}
		motivo = pgtype.Text{String: m, Valid: true}
	}
	if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: e.Thread, ProdottoID: e.Prodotto, Ruolo: "prodotto",
		Azione: e.Azione, AllegatoID: e.Allegato, Sha256: strings.TrimSpace(e.Sha256), DocumentoID: uuid.NullUUID{UUID: e.Documento, Valid: true},
		Motivo: motivo, AutoreID: e.Autore}); err != nil {
		return false, err
	}
	return true, nil
}
