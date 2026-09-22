package documenti

import (
	"context"
	"errors"
	"fmt"
	"io"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// LA VERIFICA DI CHI STA PER SERVIRE I BYTE (blocco 8, B8.1).
//
// `AllineaDocumento` qui sopra e' un GESTO DI UNA PERSONA: un amministratore guarda una segnalazione,
// decide che il file sul NAS e' quello buono e chiede di allineare il database. Fa esattamente quello
// e niente di piu'. In particolare — ed e' il motivo per cui questa funzione esiste — quando l'hash
// NON corrisponde:
//
//   - restituisce un errore e NON apre nessuna anomalia. Va benissimo li': chi ha premuto e' davanti
//     allo schermo e legge il messaggio, e nove volte su dieci l'anomalia su quel documento e' gia'
//     aperta, perche' e' da quell'elenco che si arriva ad «Allinea»;
//   - ricompone il percorso da solo, con `nas.UNC`, senza passare da `PercorsoSulNas`: il
//     contenimento nella radice non lo fa.
//
// Nessuna delle due cose va bene per l'anteprima, che e' l'opposto: nessuno sta guardando quel
// documento in particolare, si arriva li' da un clic su un allegato, e se il file sul NAS non e'
// quello del fascicolo la scoperta deve RESTARE — scritta come anomalia, visibile a chi amministra —
// anche quando chi ha cliccato chiude la finestra e non lo dice a nessuno. Una verifica che trova un
// conflitto e lo tiene per se' e' una verifica che non e' stata fatta.
//
// Quindi l'anteprima non chiama `AllineaDocumento`: chiama questa, che ricalcola l'hash SUL FILE GIA'
// APERTO dal percorso gia' contenuto, e:
//
//	corrisponde     → `verificato_il = now()`, e l'eventuale anomalia si chiude (il file e' a posto)
//	non corrisponde → anomalia «conflitto» aperta con l'hash trovato, ed ErrNonCorrisponde
//	non si legge    → anomalia «illeggibile» aperta, e l'errore di lettura
//
// Il contenimento e' garantito anche DURANTE la verifica proprio perche' il file e' gia' aperto: i
// byte che si contano sono gli stessi che si serviranno, dallo stesso handle. Verificare per nome e
// servire per nome sono due aperture, e fra le due — su una condivisione di rete — il file puo'
// cambiare: si direbbe di si' a proposito di byte che nessuno mandera' mai a nessuno.
//
// Quello che questa funzione NON fa e' sempre lo stesso di tutto il blocco 5B: non ripara, non
// sovrascrive, non cancella. Scrive `documento.verificato_il` e righe di `nas_anomalia`.

// ErrNonCorrisponde: il file c'e', si legge, e non e' questo documento. E' un conflitto, e un
// conflitto non si risolve da solo: si segnala e lo guarda una persona.
var ErrNonCorrisponde = errors.New("il file sul NAS non corrisponde al documento")

// VerificaFileAperto rilegge INTERO il file gia' aperto e confronta il suo hash con quello del
// documento. `relativo` e' il percorso relativo alla radice del NAS (quello che finisce nella riga di
// anomalia): lo si passa perche' chi ha aperto il file l'ha gia' ricavato e contenuto, e ricavarlo
// una seconda volta qui dentro vorrebbe dire poter ricavarlo diverso.
//
// Alla fine il lettore e' riavvolto all'inizio in ogni caso, anche quando la verifica fallisce: chi
// l'ha aperto decide se chiuderlo, non questa funzione.
func VerificaFileAperto(ctx context.Context, q *db.Queries, d db.Documento, relativo string, f io.ReadSeeker) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	sha, letti, err := nas.Sha256Da(f)
	if _, errS := f.Seek(0, io.SeekStart); errS != nil && err == nil {
		err = errS
	}
	if err != nil {
		// Il file c'e' e non si e' riusciti a leggerlo fino in fondo. Non si puo' dire ne' che sia
		// quello giusto ne' che sia un altro: dire «conflitto» qui vorrebbe dire accusare qualcuno di
		// aver sostituito un file sulla base di una lettura interrotta.
		if errA := Segnala(ctx, q, d, relativo, db.ProblemaNasIlleggibile, "",
			"il file c'e' ma non si riesce a leggerlo fino in fondo: "+err.Error()); errA != nil {
			return errA
		}
		return err
	}
	if sha != d.Sha256 {
		dettaglio := fmt.Sprintf("sul NAS c'e' un file diverso da questo documento (%d byte, sha256 %.12s… "+
			"invece di %.12s…). Non viene sovrascritto e non viene mostrato: decidere quale dei due e' "+
			"quello buono e' una cosa da fare guardandoli", letti, sha, d.Sha256)
		if err := Segnala(ctx, q, d, relativo, db.ProblemaNasConflitto, sha, dettaglio); err != nil {
			return err
		}
		return fmt.Errorf("%w: %s", ErrNonCorrisponde, dettaglio)
	}
	// Corrisponde: l'abbiamo guardato adesso, e byte per byte. E' la stessa scrittura che fa il
	// ricognitore quando passa — `verificato_il` e' «quando l'abbiamo guardato», non «quando l'abbiamo
	// copiato» — e se c'era una segnalazione aperta su questo documento, adesso non c'e' piu' ragione.
	if err := q.SetDocumentoVerificato(ctx, d.DocumentoID); err != nil {
		return err
	}
	_, err = q.ChiudiAnomaliaNas(ctx, d.DocumentoID)
	return err
}

// Segnala apre (o aggiorna) l'anomalia di un documento. E' la stessa riga che scrive il ricognitore,
// con le stesse parole: chi guarda l'elenco in Admin non deve poter capire da che cosa e' stata
// scritta una segnalazione, perche' il fatto e' lo stesso.
//
// Esportata perche' l'anteprima e' il secondo posto da cui si guarda un file vero: un documento che
// dice «scritto» e non c'e' piu' e' una scoperta, chiunque la faccia, e una scoperta che non lascia
// traccia e' una scoperta persa.
func Segnala(ctx context.Context, q *db.Queries, d db.Documento, relativo string,
	problema db.ProblemaNas, shaTrovato, dettaglio string) error {
	_, err := q.ApriAnomaliaNas(ctx, db.ApriAnomaliaNasParams{
		DocumentoID: d.DocumentoID, ThreadID: d.ThreadID, Problema: problema,
		StatoDb: d.StatoNas, Percorso: relativo, ShaAtteso: d.Sha256,
		ShaTrovato: ptesto(shaTrovato), Dettaglio: dettaglio,
	})
	return err
}
