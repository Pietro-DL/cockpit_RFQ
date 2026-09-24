package documenti

// Le cartelle del fascicolo sul NAS non si cancellano mai da sole (addendum A4.3). Toglierne una e' un
// gesto esplicito dell'amministratore, verificato dal server subito prima di rimuovere.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// ErrCartellaReferenziata: un documento, uno spostamento pendente o una riga aperta di nas_orfano
// nominano qualcosa sotto la cartella. Le istantanee delle baseline non contano (R1.8).
var ErrCartellaReferenziata = errors.New("la cartella è ancora nominata dal fascicolo")

// ErrCartellaNonVuota: sul disco la cartella ha ancora qualcosa, anche un .parte di un tentativo vivo.
var ErrCartellaNonVuota = errors.New("la cartella non è vuota")

// RimuoviCartella toglie una cartella del fascicolo della RFQ (relativa alla cartella del thread)
// solo se nessuno la nomina e se, tolti i .parte.<token> scaduti, e' vuota su disco. os.Remove, mai
// RemoveAll: in dubbio si archivia, non si cancella. Prende il lucchetto della cartella, cosi' non
// corre con chi ci sta scegliendo un nome.
func RimuoviCartella(ctx context.Context, q *db.Queries, scrittore *nas.Scrittore, thread uuid.UUID, cartella string) error {
	cartella = strings.Trim(cartella, `\`)
	if cartella == "" {
		return errors.New("la cartella della RFQ non si toglie da qui")
	}
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return err
	}
	if !t.CartellaRelativa.Valid {
		return fmt.Errorf("thread %s senza cartella", thread)
	}
	if err := BloccaCartella(ctx, q, thread, cartella); err != nil {
		return err
	}
	ref, err := q.CartellaReferenziata(ctx, db.CartellaReferenziataParams{ThreadID: thread, Cartella: cartella})
	if err != nil {
		return err
	}
	if ref.Bool {
		return fmt.Errorf("%w: %s", ErrCartellaReferenziata, cartella)
	}
	sulNas := strings.TrimRight(t.CartellaRelativa.String, `\`) + `\` + cartella
	if _, err := PulisciPartiScadute(ctx, q, scrittore, sulNas); err != nil {
		return err
	}
	if err := scrittore.RimuoviCartellaVuota(sulNas); err != nil {
		return fmt.Errorf("%w: %s (%v)", ErrCartellaNonVuota, cartella, err)
	}
	return nil
}
