// Che cosa significa copiare un documento sul NAS: dove va il file, come si ritrova il suo
// contenuto, che cosa fare se quel contenuto non c'e' piu'.
//
// Fino a B6b stava dentro lo switch dell'esecutore, che e' il posto dove si decide CHI esegue, non
// COSA. L'esecutore resta il chiamante: prende il job, riconosce il tipo, decodifica il payload e
// chiama CopiaSulNas. Il dispatch per tipo non e' cambiato.

package documenti

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
)

// CopiaSulNas copia sul NAS il file di un documento confermato, verificando l'hash.
func CopiaSulNas(ctx context.Context, q *db.Queries, scrittore *nas.Scrittore, j *db.Job, documentoID uuid.UUID) (any, error) {
	d, err := q.GetDocumento(ctx, documentoID)
	if err != nil {
		return nil, err
	}
	t, err := q.GetThread(ctx, d.ThreadID)
	if err != nil {
		return nil, err
	}
	if !t.CartellaRelativa.Valid {
		return nil, fmt.Errorf("thread %s senza cartella_relativa", d.ThreadID)
	}
	src, sparito, err := cercaSorgente(ctx, q, d)
	if sparito != nil {
		// Il contenuto non c'e' piu' nella cache. L'operatore ha gia' deciso che quel file va sul
		// NAS, e il file e' ancora in Outlook o dentro il suo archivio: la ripresa si accoda da
		// sola (Pre-7), e questo tentativo fallisce dicendo che cosa sta succedendo.
		err = RiprendiContenuto(ctx, q, j, *sparito, err)
	}
	if err != nil {
		// Il motivo va SCRITTO SUL DOCUMENTO, non solo nel log: il log lo legge chi sta
		// diagnosticando, il fascicolo lo guarda chi aspetta quel disegno. Nella prova reale la
		// frase giusta — quale contenuto manca e che si riprende con «Riscarica» — e' finita nel
		// log del server mentre nella riga del documento restava l'errore del filesystem di ore
		// prima: due versioni della stessa cosa, e quella sbagliata era l'unica visibile.
		_ = q.SetDocumentoErrore(ctx, db.SetDocumentoErroreParams{
			DocumentoID: d.DocumentoID, ErroreNas: pgtype.Text{String: err.Error(), Valid: true}})
		return nil, err
	}
	dst, err := scrittore.Copia(src, t.CartellaRelativa.String, d.PathRelativo, d.Sha256)
	if err != nil {
		_ = q.SetDocumentoErrore(ctx, db.SetDocumentoErroreParams{DocumentoID: d.DocumentoID, ErroreNas: pgtype.Text{String: err.Error(), Valid: true}})
		return nil, err
	}
	if !scrittore.DryRun {
		if err := q.SetDocumentoScritto(ctx, d.DocumentoID); err != nil {
			return nil, err
		}
	}
	// Il contenuto e' stato USATO, non consumato: resta nella cache (Pre-7, D31), e l'orario
	// rinfrescato dice al custode che serve ancora.
	staging.ToccaContenuto(src)
	return map[string]any{"destinazione": dst, "dry_run": scrittore.DryRun}, nil
}

// ErrContenutoMancante: il documento e' confermato, il suo contenuto non e' piu' nello staging.
//
// Non e' un guasto del NAS e non si risolve riprovando: il file da copiare non c'e'. Si riprende da
// Outlook, che e' dove sta l'originale.
var ErrContenutoMancante = errors.New("contenuto non piu' in staging")

// SorgenteStaging trova il file da copiare: un allegato del thread con lo stesso hash del documento.
//
// Il file viene GUARDATO, non solo letto dal database. `path_staging` dice dove il contenuto e' stato
// messo, non che ci sia ancora, e fra la conferma e la copia puo' passare molto tempo: con le
// capacita' separate un documento confermato mentre `nas_scrittura` era spenta aspetta giorni, e in
// mezzo ci sono la pulizia dello staging, un disco rifatto, una cartella svuotata a mano.
//
// Senza questo controllo il fascicolo si ferma con «open C:\...\_contenuti\73\739f....pdf:
// Impossibile trovare il percorso specificato»: una frase che dice dove il file non c'era e non dice
// a nessuno che cosa fare. Il contenuto si riprende con «Riscarica» sull'allegato, e la frase adesso
// lo dice.
func SorgenteStaging(ctx context.Context, q *db.Queries, d db.Documento) (string, error) {
	src, _, err := cercaSorgente(ctx, q, d)
	return src, err
}

// cercaSorgente e' SorgenteStaging che dice anche QUALE allegato ha perso il file, perche' la
// ripresa (RiprendiContenuto) ha bisogno dell'allegato, non del suo nome. Dal Pre-7 conta anche un
// allegato con l'hash giusto e senza percorso: e' cosi' che lo lascia il custode della cache.
func cercaSorgente(ctx context.Context, q *db.Queries, d db.Documento) (string, *db.Allegato, error) {
	all, err := q.ListAllegatiThread(ctx, uuid.NullUUID{UUID: d.ThreadID, Valid: true})
	if err != nil {
		return "", nil, err
	}
	var sparito *db.Allegato
	for i := range all {
		a := all[i]
		if !a.Sha256.Valid || a.Sha256.String != d.Sha256 {
			continue
		}
		if a.PathStaging.Valid {
			if st, err := os.Stat(a.PathStaging.String); err == nil && !st.IsDir() {
				return a.PathStaging.String, nil, nil
			}
		}
		if sparito == nil {
			sparito = &all[i]
		}
	}
	if sparito != nil {
		return "", sparito, fmt.Errorf("%w: il contenuto di %q non e' piu' nello staging del server. "+
			"Si riprende da Outlook: «Riscarica» sull'allegato nel messaggio, poi «Riprova copie» qui",
			ErrContenutoMancante, sparito.NomeFile)
	}
	return "", nil, fmt.Errorf("%w: nessun allegato in staging con sha256 %s", pgx.ErrNoRows, d.Sha256)
}
