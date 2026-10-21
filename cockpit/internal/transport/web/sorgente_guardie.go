package web

// sorgente_guardie.go: le guardie della Sorgente nei gesti legacy di questo package (V1, EM §10.7; D23:204; A2-Q06 A).
// Un file in Altri file o scartato nella Distinta nuova non riceve effetti dalle pagine legacy: «fascicolo/assegna», la
// conferma (voce e lotto), «Documentazione generale» e il vecchio «Scarta il file» su un file in Altri file
// rifiutano con «Niente è cambiato», senza scrivere. «Scarica» salta i file non attivi e lo dice (G-8), «Riscarica»
// rifiuta (G-9). Le pagine legacy restano agli operatori in V1.

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermadoc"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/db"
)

// I testi del rifiuto dei gesti legacy su un file non attivo (EM §10.7): una sola fonte, quella del core della conferma
// (confermadoc.RifiutoFileNonAttivo, corsia persistenza), cosi' i due rifiuti non si separano (richiesta della
// persistenza dal blocco S8, punto 3).
const (
	testoFileInAltriFile = confermadoc.TestoFileInAltriFile
	testoFileScartato    = confermadoc.TestoFileScartato
)

// statoOccorrenza: lo stato dell'occorrenza di un allegato nella RFQ del suo messaggio (attivo senza riga). Il
// predicato e' quello di file_rfq_attivo: uno scarto col vecchio «Scarta il file» vale attivo qui (le pagine legacy lo
// trattano gia' come proposta scartata).
func statoOccorrenza(ctx context.Context, q *db.Queries, allegato uuid.UUID) (string, error) {
	st, err := q.StatoOccorrenzaAllegato(ctx, allegato)
	if errors.Is(err, pgx.ErrNoRows) {
		return filerfq.StatoAttivo, nil
	}
	if err != nil {
		return "", err
	}
	return st.Stato, nil
}

// rifiutoFileNonAttivo: il rifiuto di un gesto documentale legacy su un file non attivo, oppure nil.
func rifiutoFileNonAttivo(ctx context.Context, q *db.Queries, allegato uuid.UUID) error {
	stato, err := statoOccorrenza(ctx, q, allegato)
	if err != nil {
		return err
	}
	switch stato {
	case filerfq.StatoAltriFile:
		return rifiuto(testoFileInAltriFile)
	case filerfq.StatoScartato:
		return rifiuto(testoFileScartato)
	}
	return nil
}
