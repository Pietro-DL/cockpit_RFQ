package workerapi

// sorgente_guardie.go: lo stato dell'occorrenza per le guardie degli scrittori del worker (V1, EM §10.7: G-1, G-2,
// G-4, G-5, G-7). Lo stato lo scrive solo il servizio del ciclo dei file (core/rfq/filerfq); qui si legge e basta.

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// statoOccorrenza: «attivo», «altri_file» o «scartato» per l'occorrenza dell'allegato nella RFQ del suo messaggio. Il
// predicato e' quello di file_rfq_attivo (uno scarto col vecchio «Scarta il file» vale attivo: le proposte scartate le
// escludono gia' i lettori legacy). Un allegato che non c'e' vale attivo: lo dice l'errore di chi lo legge dopo.
func statoOccorrenza(ctx context.Context, q *db.Queries, allegato uuid.UUID) (string, error) {
	st, err := q.StatoOccorrenzaAllegato(ctx, allegato)
	if errors.Is(err, pgx.ErrNoRows) {
		return "attivo", nil
	}
	if err != nil {
		return "", err
	}
	return st.Stato, nil
}
