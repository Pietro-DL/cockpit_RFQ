// I corpi dei job che toccano il fascicolo (blocco 5, Pre-7).
//
// Che cosa significhi «copiare un documento sul NAS» — dove va il file, che cosa fare se il suo
// contenuto non c'e' piu', quando NON copiare — e' una regola del fascicolo, non della coda. Fino a
// B6b stava dentro lo switch dell'esecutore, che e' il posto dove si decide CHI esegue, non COSA.
//
// L'esecutore resta il chiamante: prende il job, riconosce il tipo, decodifica il payload e chiama
// una di queste. Il dispatch per tipo non e' cambiato.
package documenti

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
)

// ScrivePerNas dice se il job ha bisogno che il NAS ci sia. Sono i due che scrivono sotto la radice:
// senza NAS non possono nemmeno cominciare, e provarci non insegna niente a nessuno.
func ScrivePerNas(t db.TipoJob) bool {
	return t == db.TipoJobCopiaNas || t == db.TipoJobCreaCartellaThread
}

// CreaCartellaThread crea sul NAS la cartella di una RFQ con le sottocartelle della convenzione.
func CreaCartellaThread(ctx context.Context, q *db.Queries, scrittore *nas.Scrittore, threadID uuid.UUID) (any, error) {
	t, err := q.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if !t.CartellaRelativa.Valid {
		return nil, fmt.Errorf("thread %s senza cartella_relativa", threadID)
	}
	layout, err := q.ListCartellaDocumento(ctx)
	if err != nil {
		return nil, err
	}
	// solo le sottocartelle della convenzione (ELENCO DISEGNI, OFFERTE FORNITORI); le altre nascono alla prima copia
	var sotto []string
	for _, l := range layout {
		if l.CreaSempre {
			sotto = append(sotto, l.Sottocartella)
		}
	}
	base, err := scrittore.CreaCartella(t.CartellaRelativa.String, sotto)
	if err != nil {
		return nil, err
	}
	if !scrittore.DryRun {
		if err := q.SetCartellaCreata(ctx, threadID); err != nil {
			return nil, err
		}
	}
	return map[string]any{"cartella": base, "dry_run": scrittore.DryRun}, nil
}

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

// sorgenteStaging trova il file da copiare: un allegato del thread con lo stesso hash del documento.
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

// cercaSorgente e' sorgenteStaging che dice anche QUALE allegato ha perso il file, perche' la
// ripresa (riprendiContenuto) ha bisogno dell'allegato, non del suo nome. Dal Pre-7 conta anche un
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

// riprendiContenuto rimette in moto la ripresa di un contenuto sparito dalla cache (Pre-7).
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
