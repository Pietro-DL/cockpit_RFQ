// Package fascicolo e' la BOM di una RFQ nel tempo (Blocco 8, addendum A4): le versioni congelate e
// le loro istantanee, la revisione che riapre la BOM working, il gate che decide se si puo' congelare,
// e i gesti che cambiano la struttura senza distruggerne la storia (archiviare, sostituire un
// documento, scegliere lo STEP strutturale, derogare a una lettura parziale).
//
// Le regole che contano le tiene il database (0020): una versione nasce bozza e congelata non cambia
// piu', la catena delle revisioni non fa cicli, dopo il congelamento la working si modifica solo
// aprendo una revisione. Qui si decide CHE COSA scrivere, e si dice all'operatore perche' qualcosa non
// si puo' fare prima che ci sbatta contro il vincolo. Ogni funzione lavora nella transazione di chi la
// chiama: qualunque errore annulla tutto.
package fascicolo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// Rifiuto e' un «no» per l'operatore: la transazione si annulla per intero e il testo dice il motivo.
type Rifiuto string

func (r Rifiuto) Error() string { return string(r) }

// WorkingBloccata dice se la BOM working della RFQ e' bloccata: l'ultima versione e' congelata e
// nessuna revisione e' aperta (D26). numero e' la versione che la blocca.
func WorkingBloccata(ctx context.Context, q *db.Queries, thread uuid.UUID) (numero int32, bloccata bool, err error) {
	v, err := q.GetUltimaVersione(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return v.Numero, v.Stato == db.StatoBomCongelata, nil
}

// SeBloccata restituisce il rifiuto da dare a chi vuole cambiare la BOM working congelata: gesto e'
// quello che voleva fare («si archivia», «si assegna»...). nil se la working e' libera.
func SeBloccata(ctx context.Context, q *db.Queries, thread uuid.UUID, gesto string) error {
	n, bloccata, err := WorkingBloccata(ctx, q, thread)
	if err != nil {
		return err
	}
	if bloccata {
		return Rifiuto(fmt.Sprintf("la BOM è congelata nella V%d: %s solo aprendo una revisione", n, gesto))
	}
	return nil
}

// componenteDellaRfq blocca il componente e controlla che sia della RFQ: chi lo cambia parte da qui.
func componenteDellaRfq(ctx context.Context, q *db.Queries, thread, comp uuid.UUID) (db.Componente, error) {
	c, err := q.BloccaComponente(ctx, comp)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ThreadID != thread) {
		return db.Componente{}, Rifiuto("il componente non è di questa RFQ")
	}
	return c, err
}
