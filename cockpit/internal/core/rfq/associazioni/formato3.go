package associazioni

// formato3.go: «RFQ formato 3», una funzione sola del core (direttiva 39 §6 [U]; NOMI (i) [T-39]; D-AR03 D [U]): una
// RFQ e' formato 3 se ha avuto, in qualunque momento, almeno una bozza o una conferma formato 3 su un suo prodotto.
// L'abbandono della bozza non la riporta a legacy. Decide quale strada prende un gesto atomico (D-AR03 D): nelle RFQ
// formato 3 cambia solo le PB della bozza con R2 e non scrive AR; nelle RFQ interamente legacy passa dal servizio unico
// (origine gesto_legacy, con la variante successiva della 0023).
//
// La definizione e' quella del database (query RfqFormato3 → rfq_e_formato3): il segno rfq_formato3_segno, una riga per
// RFQ scritta alla nascita della prima bozza formato 3, che nessuno cambia o cancella; la conservazione delle bozze
// abbandonate non lo tocca, quindi il limite dei 90 giorni delle varianti di prima non c'e' piu' (dalla variante
// 20261019c della 0023; vigente la 20261019d).

import (
	"context"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// RFQFormato3 dice se la RFQ e' formato 3 (commento di testa). Sola lettura, nella transazione di chi chiama.
func RFQFormato3(ctx context.Context, q *db.Queries, thread uuid.UUID) (bool, error) {
	return q.RfqFormato3(ctx, thread)
}
