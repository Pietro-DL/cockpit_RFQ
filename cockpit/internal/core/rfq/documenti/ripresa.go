// La ripresa di un contenuto sparito dalla cache (Pre-7).
//
// Sta accanto alla copia perche' e' la copia a chiamarla, ma e' una storia sua — riestrazione di un
// archivio in cache, download da Outlook, oppure resa dichiarata — e nel file della copia la
// seppelliva.

package documenti

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
// Quattro esiti:
//   - la voce viene da un archivio ancora in cache → si riaccoda l'estrazione, che rimette la voce
//     al suo posto (il nome e' l'hash) senza tornare in Outlook;
//   - il file (o l'archivio che lo conteneva) e' stato caricato a mano nel Fascicolo: in Outlook non
//     c'e', e il sistema non ha da dove riprenderlo. Il messaggio dice di ricaricarlo dal Fascicolo, e
//     non nomina «Riscarica», che su quel file non porterebbe da nessuna parte;
//   - altrimenti si accoda il download da Outlook dell'allegato, o dell'archivio che lo conteneva;
//   - se per QUESTA copia il download e' gia' stato provato ed e' fallito, non si insiste: resta il
//     messaggio di prima, con «Riscarica», perche' a quel punto serve una persona. Senza questo
//     limite una mail cancellata da Outlook farebbe accodare un download a ogni tentativo della
//     copia, cioe' per ore.
//
// In tutti i casi questo tentativo della copia FALLISCE, e la coda lo riprova da sola con il suo
// rinvio: il contenuto arriva fra qualche secondo o qualche minuto, e il tentativo dopo lo trova.
//
// j puo' essere nil (una copia chiamata fuori dalla coda): allora non c'e' un «questo tentativo» a
// cui legare il limite, e non c'e' nemmeno un giro di tentativi da fermare — il download si accoda.
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
	if bersaglio.Origine == db.OrigineAllegatoManuale {
		return contenutoCaricatoAMano(a, bersaglio)
	}
	chiave := "stage:" + bersaglio.AllegatoID.String()
	if ultimo, err := q.UltimoJobPerChiave(ctx, pgtype.Text{String: chiave, Valid: true}); err == nil && j != nil &&
		ultimo.Stato == db.StatoJobFallito && ultimo.ChiusoIl != nil && ultimo.ChiusoIl.After(j.CreatoIl) {
		return fmt.Errorf("%w (il download da Outlook e' gia' stato provato per questa copia ed e' fallito: %s)",
			originale, ultimo.Errore.String)
	}
	m, err := q.GetMessaggio(ctx, bersaglio.MessaggioID)
	if err != nil {
		return originale
	}
	copia, err := coda.CopiaPerDownload(ctx, q, m.MessaggioID, uuid.NullUUID{}, uuid.Nil)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// nessuna presenza del messaggio in una casella attiva: e' l'unico caso in cui «nessuna casella»
		// e' vero
		return fmt.Errorf("%w (nessuna casella attiva da cui riscaricarlo)", originale)
	case err != nil:
		return fmt.Errorf("%w (la ripresa da Outlook non e' partita: %v)", originale, err)
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

// contenutoCaricatoAMano e' il motivo per un contenuto sparito che era stato caricato a mano nel
// Fascicolo (dal PC o dal NAS): in Outlook non c'e', e l'unica strada e' ricaricarlo. a e' la voce che
// manca, caricato e' il file caricato (la voce stessa, o l'archivio che la conteneva).
func contenutoCaricatoAMano(a, caricato db.Allegato) error {
	if caricato.AllegatoID == a.AllegatoID {
		return fmt.Errorf("%w: il contenuto di %q, caricato a mano nel Fascicolo, non e' piu' nello staging del server "+
			"e in Outlook non c'e': si ricarica il file dal Fascicolo («+ Aggiungi file»), poi «Riprova copie»",
			ErrContenutoMancante, a.NomeFile)
	}
	return fmt.Errorf("%w: il contenuto di %q non e' piu' nello staging del server, e nemmeno l'archivio %q da cui "+
		"era uscito, caricato a mano nel Fascicolo: in Outlook non c'e', si ricarica l'archivio dal Fascicolo "+
		"(«+ Aggiungi file»), poi «Riprova copie»", ErrContenutoMancante, a.NomeFile, caricato.NomeFile)
}
