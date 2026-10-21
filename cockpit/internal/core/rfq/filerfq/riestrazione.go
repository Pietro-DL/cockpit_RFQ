package filerfq

// riestrazione.go — la riestrazione di un archivio in Altri file (V1, direttiva 35, blocco C-6a; U2 §3.10, U2-P45):
// per un contenitore in Altri file, non scartato, con voci attive, la riestrazione che rimette i byte di voci attive
// gia' esistenti e' ammessa, senza scrivere voci nuove, leggendo il contenitore dallo staging o dalla sua copia nella
// cartella ALTRO, in sola lettura. Qui sta la seconda strada: i byte del contenitore rimessi nello staging dalla sua
// copia, con lo sha verificato scrivendo. L'estrazione (workerapi, G-7) decide il resto.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// ErrArchivioNonInAltriFile: il contenitore non e' in Altri file nella sua RFQ (attivo o scartato): da qui non si
// rimettono i suoi byte.
var ErrArchivioNonInAltriFile = errors.New("l'archivio non e' in Altri file")

// ArchivioNelloStaging rimette nello staging i byte di un contenitore in Altri file dalla sua copia verificata nella
// cartella ALTRO (U2-P45), e ne restituisce il percorso: se il posto del contenuto c'e' gia' con lo sha giusto si usa;
// altrimenti un parziale del ciclo dei file letto dalla copia in sola lettura (sha verificato scrivendo), poi sotto
// BloccaContenuto il controllo che decide, la promozione e path_staging (scritto dopo il contenuto: «chi adotta», EM
// §10.6). Nessuna riga di stato cambia: il contenitore resta in Altri file.
func (s *Servizio) ArchivioNelloStaging(ctx context.Context, allegato uuid.UUID) (string, error) {
	if s.Conservatore == nil || s.Staging == "" {
		return "", errors.New("nessun posto per gli Altri file o nessuno staging su questo server")
	}
	q := db.New(s.Pool)
	st, err := q.StatoOccorrenzaAllegato(ctx, allegato)
	if err != nil {
		return "", err
	}
	if !st.ThreadID.Valid || st.Stato != StatoAltriFile || st.ScartatoLegacy {
		return "", ErrArchivioNonInAltriFile
	}
	c, err := q.GetConservazioneCopiataRiga(ctx, db.GetConservazioneCopiataRigaParams{ThreadID: st.ThreadID.UUID, AllegatoID: allegato})
	if err != nil {
		return "", fmt.Errorf("la copia in ALTRO dell'archivio non c'e': %w", err)
	}
	a, err := q.GetAllegato(ctx, allegato)
	if err != nil {
		return "", err
	}
	sha := c.Sha256
	dest, err := staging.PercorsoContenuto(s.Staging, sha, a.NomeFile)
	if err != nil {
		return "", err
	}
	parte := ""
	if !presenteNelloStaging(s.Staging, dest, sha) {
		in, err := s.Conservatore.Leggi(postoDellaRiga(c))
		if err != nil {
			return "", fmt.Errorf("la copia in ALTRO dell'archivio non si legge: %w", err)
		}
		parte = percorsoParteCiclo(s.Staging)
		causa, err := scriviParziale(in, parte, sha)
		in.Close()
		if err != nil {
			_ = togliFile(parte)
			return "", fmt.Errorf("la copia in ALTRO dell'archivio non si copia nello staging (%s): %w", causa, err)
		}
	}
	defer func() {
		if parte != "" {
			staging.RimuoviParte(parte)
		}
	}()
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.BloccaContenuto(ctx, sha); err != nil {
			return err
		}
		if !staging.ContenutoGiaPresente(dest, sha) {
			if parte == "" {
				return errors.New("il contenuto dell'archivio e' sparito dallo staging: si riprova")
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			if err := staging.Promuovi(parte, dest); err != nil {
				return err
			}
			parte = ""
		} else {
			staging.ToccaContenuto(dest)
		}
		return q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: allegato, PathStaging: testo(dest),
			Sha256: a.Sha256, Bytes: a.Bytes})
	})
	if err != nil {
		return "", err
	}
	return dest, nil
}
