// Package traccia scrive il motivo di un gesto quando l'utente non lo scrive (messaggio 46 dell'utente, 21/10 [U]: «la
// conferma con motivo non mi piace, anzi dobbiamo rimuoverlo da ogni widget in cui appare»).
//
// La decisione resta dell'utente: e' il gesto, con il suo autore (autore_id, chiusa_da, utente_id delle righe). Il testo
// lo scrive il server, e lo dice: «gesto di <sigla> da «<widget>»: <che cosa>», piu' le evidenze del motore dove ci sono
// (come la traccia del Motore A di S8, 9eb44e3). Non finge mai che l'abbia scritto l'utente. Si scrive solo dove lo schema
// vuole un motivo (i CHECK della 0022 e della 0023: fonte_step_evento revoca e rifiuto, file_rfq_evento scarta,
// proposta_aggiornata_rifiuto, conferma_strutturale_non_incluso, conferma_strutturale mono_pezzo, l'evento motivato del
// cambio di fonte di una bozza) o dove una regola del contratto lo voleva per tracciare il gesto (la correzione del codice
// di S8, la riassociazione, la fonte con il codice della radice diverso): senza DDL, nelle colonne motivo di sempre.
//
// La traccia e' deterministica: lo stesso gesto della stessa persona sugli stessi dati da' lo stesso testo, cosi' il
// reinvio con la stessa chiave si riconosce confrontando il motivo registrato con la traccia esatta che il server
// genererebbe (mai con un prefisso). Se l'utente manda comunque un motivo (i client di prima), vale il suo.
package traccia

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// Prefisso: l'inizio di ogni traccia scritta dal server (le pagine lo riconoscono per dirlo «scritto dal sistema»).
const Prefisso = "gesto "

// Gesto compone la traccia: «gesto di <sigla> da «<widget>»: <che cosa>; evidenze: <e1>; <e2>», al piu' max caratteri
// (rune; con max <= 0 nessun limite). La sigla vuota da' «gesto da «<widget>»…»; cosa e le evidenze vuote si tolgono. Gli
// spazi ai bordi di ogni parte si tolgono e i ritorni a capo diventano spazi (il motivo e' una riga di testo).
func Gesto(max int, sigla, widget, cosa string, evidenze ...string) string {
	var b strings.Builder
	b.WriteString("gesto")
	if s := pulito(sigla); s != "" {
		b.WriteString(" di " + s)
	}
	if w := pulito(widget); w != "" {
		b.WriteString(" da «" + w + "»")
	}
	if c := pulito(cosa); c != "" {
		b.WriteString(": " + c)
	}
	var ev []string
	for _, e := range evidenze {
		if e = pulito(e); e != "" {
			ev = append(ev, e)
		}
	}
	if len(ev) > 0 {
		b.WriteString("; evidenze: " + strings.Join(ev, "; "))
	}
	return Tronca(b.String(), max)
}

// Tronca: il testo in al piu' max rune, con «…» in fondo se e' tagliato (max <= 0: com'e').
func Tronca(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

// pulito: una parte della traccia su una riga, senza spazi ai bordi.
func pulito(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	return strings.TrimSpace(s)
}

// Sigla: la sigla dell'utente (la stessa del login), "" se l'utente non c'e' (un gesto del server). Legge il database
// nella transazione di chi chiama.
func Sigla(ctx context.Context, q *db.Queries, utente uuid.UUID) (string, error) {
	if utente == uuid.Nil {
		return "", nil
	}
	u, err := q.GetUtente(ctx, utente)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(u.Sigla), nil
}

// DelGesto: Gesto con la sigla letta dal database (Sigla).
func DelGesto(ctx context.Context, q *db.Queries, utente uuid.UUID, max int, widget, cosa string, evidenze ...string) (string, error) {
	s, err := Sigla(ctx, q, utente)
	if err != nil {
		return "", err
	}
	return Gesto(max, s, widget, cosa, evidenze...), nil
}

// DellUtente: il motivo dell'utente se c'e' (i client di prima), altrimenti la traccia.
func DellUtente(motivo, traccia string) string {
	if m := strings.TrimSpace(motivo); m != "" {
		return m
	}
	return traccia
}
