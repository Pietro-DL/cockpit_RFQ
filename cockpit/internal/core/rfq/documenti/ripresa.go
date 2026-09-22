// La ripresa di un contenuto sparito dalla cache (Pre-7).
//
// Sta accanto alla copia perche' e' la copia a chiamarla, ma e' una storia sua — riestrazione di un
// archivio in cache, download da Outlook, oppure resa dichiarata — e nel file della copia la
// seppelliva.

package documenti

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// RiprendiContenuto rimette in moto la ripresa di un contenuto sparito dalla cache (Pre-7).
//
// Il documento e' confermato: l'operatore ha gia' deciso che quel file va sul NAS, e il file e'
// ancora in Outlook o dentro l'archivio da cui era stato estratto. Chiedergli di premere «Riscarica»
// per una cosa che il sistema sa fare da solo e' un vicolo cieco travestito da pulsante.
//
// Tre esiti:
//   - la voce viene da un archivio ancora in cache → si riaccoda l'estrazione, che rimette la voce
//     al suo posto (il nome e' l'hash) senza tornare in Outlook;
//   - altrimenti si accoda il download da Outlook dell'allegato, o dell'archivio che lo conteneva;
//   - se per QUESTA copia il download e' gia' stato provato ed e' fallito, non si insiste: resta il
//     messaggio di prima, con «Riscarica», perche' a quel punto serve una persona. Senza questo
//     limite una mail cancellata da Outlook farebbe accodare un download a ogni tentativo della
//     copia, cioe' per ore.
//
// In tutti i casi questo tentativo della copia FALLISCE, e la coda lo riprova da sola con il suo
// rinvio: il contenuto arriva fra qualche secondo o qualche minuto, e il tentativo dopo lo trova.
func RiprendiContenuto(ctx context.Context, q *db.Queries, j *db.Job, a db.Allegato, originale error) error {
	bersaglio := a
	if a.ContenitoreID.Valid {
		z, err := q.GetAllegato(ctx, a.ContenitoreID.UUID)
		if err != nil {
			return originale
		}
		if z.PathStaging.Valid && (staging.FileStaging{}).Presente(z.PathStaging.String) {
			if _, err := coda.Accoda(ctx, q, db.TipoJobEstraiArchivio, worker.PayloadEstraiArchivio{AllegatoID: z.AllegatoID},
				"estrai:"+z.AllegatoID.String(), 2); err != nil {
				return originale
			}
			return fmt.Errorf("%w: il contenuto di %q non e' piu' nella cache, ma l'archivio %q si': "+
				"riestrazione accodata, la copia riprova da sola", ErrContenutoMancante, a.NomeFile, z.NomeFile)
		}
		bersaglio = z
	}
	chiave := "stage:" + bersaglio.AllegatoID.String()
	if ultimo, err := q.UltimoJobPerChiave(ctx, pgtype.Text{String: chiave, Valid: true}); err == nil &&
		ultimo.Stato == db.StatoJobFallito && ultimo.ChiusoIl != nil && ultimo.ChiusoIl.After(j.CreatoIl) {
		return fmt.Errorf("%w (il download da Outlook e' gia' stato provato per questa copia ed e' fallito: %s)",
			originale, ultimo.Errore.String)
	}
	m, err := q.GetMessaggio(ctx, bersaglio.MessaggioID)
	if err != nil {
		return originale
	}
	copia, err := coda.CopiaPerDownload(ctx, q, m.MessaggioID, uuid.NullUUID{}, uuid.Nil)
	if err != nil {
		return fmt.Errorf("%w (nessuna casella attiva da cui riscaricarlo: %v)", originale, err)
	}
	esito, _, err := coda.AccodaStage(ctx, q, staging.FileStaging{}, bersaglio, m, copia, 2)
	if err != nil {
		return originale
	}
	switch esito {
	case coda.StageAccodato, coda.StageGiaInCoda:
		return fmt.Errorf("%w: il contenuto di %q non e' piu' nella cache: download da Outlook accodato, "+
			"la copia riprova da sola", ErrContenutoMancante, bersaglio.NomeFile)
	default: // gia' presente o riusato: il file e' ricomparso fra il controllo e adesso
		return fmt.Errorf("%w: il contenuto di %q e' ricomparso: la copia riprova da sola", ErrContenutoMancante, bersaglio.NomeFile)
	}
}
