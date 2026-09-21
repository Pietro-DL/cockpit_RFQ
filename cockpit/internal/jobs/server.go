// Package jobs ESEGUE i job di tipo 'server': quelli che non hanno un worker dall'altra parte
// perche' il lavoro lo fa il server stesso — la copia sul NAS, la cartella del thread, la ripresa di
// un contenuto sparito — e il ricognitore che confronta i documenti con i file veri.
//
// La coda su cui lavora (accodamento, claim, lease, capacita', instradamento) sta in
// `platform/coda`; la cartella di lavoro sul disco in `platform/storage/staging`. Qui c'e' solo chi
// prende un job e lo porta a termine.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/ai/agente"
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

// Estrattore sa scompattare un archivio gia' in staging e registrarne le voci.
//
// E' un'interfaccia dichiarata QUI, e implementata altrove, per un motivo di dipendenze: il resto
// della pipeline dopo lo staging — proposte, rumore, analisi — vive nel pacchetto che riceve i
// risultati dei worker, e quel pacchetto importa gia' questo. Spostare tutto qui per far girare
// l'estrazione nell'esecutore sarebbe un trasloco molto piu' grande della correzione.
//
// `token` e' il lease del tentativo: la cartella temporanea in cui l'archivio si scompatta porta quel
// token nel nome, cosi' e' roba di QUESTO tentativo e la pulizia sa di chi era se il tentativo non
// arriva in fondo.
type Estrattore interface {
	EstraiArchivio(ctx context.Context, allegatoID uuid.UUID, token uuid.UUID) (int, error)
}

// EsecutoreServer prende i job con worker_tipo='server' (scrittura NAS, estrazione degli archivi) e
// li esegue in una goroutine.
type EsecutoreServer struct {
	Pool *pgxpool.Pool
	NAS  *nas.Scrittore
	Log  *slog.Logger
	// Archivi esegue i job `estrai_archivio`. Nil = quei job falliscono dicendolo, invece di restare
	// in coda a tempo indeterminato mentre gli operatori aspettano le voci di uno zip.
	Archivi Estrattore
	// Agente esegue l'analisi semantica (checkpoint 3R §9). Nil o spento = i job di quel tipo
	// falliscono dicendo che l'analisi non e' attiva, invece di restare in coda a tempo indeterminato.
	Agente *agente.Servizio
	// RitardoNasAssente: fra quanto riprovare un job che tocca il NAS trovato irraggiungibile.
	// Zero = un minuto. È un campo e non una costante perché il test non può aspettare un minuto.
	RitardoNasAssente time.Duration
	// ControlloNas: ogni quanto guardare se il NAS è tornato. Zero = un minuto.
	ControlloNas time.Duration
}

func (e *EsecutoreServer) Avvia(ctx context.Context) {
	q := db.New(e.Pool)
	if !e.NAS.DryRun {
		go e.vigilaNas(ctx, q)
	}
	go func() {
		id := "server"
		for ctx.Err() == nil {
			j, err := coda.Claim(ctx, q, db.WorkerTipoServer, id, coda.Destinazione{}, 20*time.Second)
			if err != nil {
				e.Log.Error("claim server", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if j == nil {
				continue
			}
			// anche l'esecutore interno passa dal tentativo: se il suo lease scade mentre scrive sul
			// NAS, il risultato non deve applicarsi al tentativo che nel frattempo ha ripreso il job
			t := coda.Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: id}
			if e.rinviaSeNasAssente(ctx, q, t, j) {
				continue
			}
			res, err := e.esegui(ctx, q, j, t)
			if err != nil {
				e.Log.Error("job server fallito", "job", j.JobID, "tipo", j.Tipo, "err", err)
				if _, err := coda.Fallisci(ctx, q, t, err.Error(), errors.Is(err, nas.ErrConflitto)); err != nil {
					e.Log.Warn("fallimento non registrato", "job", j.JobID, "err", err)
				}
				continue
			}
			raw, _ := json.Marshal(res)
			if _, err := coda.Completa(ctx, q, t, json.RawMessage(raw)); err != nil {
				e.Log.Error("completa job", "job", j.JobID, "err", err)
			}
		}
	}()
}

// rinviaSeNasAssente restituisce true se il job è stato rimesso in coda senza essere eseguito.
//
// Il tentativo non viene consumato: il job non è fallito, non lo abbiamo nemmeno provato. Contarlo
// come fallimento significherebbe bruciare il budget dei tentativi mentre il problema è che il NAS
// non c'è — e dichiarare persa la copia proprio per aver provato tante volte (voce 1.7, N8).
func (e *EsecutoreServer) rinviaSeNasAssente(ctx context.Context, q *db.Queries, t coda.Tentativo, j *db.Job) bool {
	if e.NAS.DryRun || !ScrivePerNas(j.Tipo) || e.NAS.Raggiungibile() {
		return false
	}
	fra := e.RitardoNasAssente
	if fra <= 0 {
		fra = time.Minute
	}
	if _, err := coda.Rinvia(ctx, q, t, fra, "NAS non raggiungibile: tentativo non consumato"); err != nil {
		e.Log.Error("rinvio non riuscito", "job", j.JobID, "tipo", j.Tipo, "err", err)
		return false // meglio provarci: il peggio che può succedere è un fallimento onesto
	}
	e.Log.Warn("NAS non raggiungibile: job rinviato senza consumare il tentativo",
		"job", j.JobID, "tipo", j.Tipo, "tentativi", j.Tentativi, "fra", fra)
	return true
}

// vigilaNas guarda se il NAS è tornato e, quando torna, rimette in coda le scritture che avevano
// finito i tentativi (N3: «NAS torna → scritto senza intervento»).
//
// Il primo controllo è all'avvio e non aspetta la transizione: se il server viene riavviato dopo che
// il NAS è già tornato, una transizione non ci sarà mai più e le copie resterebbero ferme per sempre.
func (e *EsecutoreServer) vigilaNas(ctx context.Context, q *db.Queries) {
	ogni := e.ControlloNas
	if ogni <= 0 {
		ogni = time.Minute
	}
	presente := e.NAS.Raggiungibile()
	if presente {
		e.RiaccodaAlRitornoDelNas(ctx, q)
	}
	t := time.NewTicker(ogni)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ora := e.NAS.Raggiungibile()
		if ora && !presente {
			e.Log.Info("NAS di nuovo raggiungibile", "radice", e.NAS.Radice)
			e.RiaccodaAlRitornoDelNas(ctx, q)
		}
		if !ora && presente {
			e.Log.Warn("NAS non più raggiungibile: le copie restano in coda", "radice", e.NAS.Radice)
		}
		presente = ora
	}
}

// RiaccodaAlRitornoDelNas rimette 'pronto' le scritture NAS che avevano esaurito i tentativi.
// Esportata perché è il punto che il test N3 chiama: quello che gira in produzione e quello che si
// verifica devono essere la stessa funzione.
func (e *EsecutoreServer) RiaccodaAlRitornoDelNas(ctx context.Context, q *db.Queries) int {
	ids, err := q.RiaccodaScrittureNasEsaurite(ctx)
	if err != nil {
		e.Log.Error("riaccodo al ritorno del NAS", "err", err)
		return 0
	}
	if len(ids) > 0 {
		e.Log.Info("scritture NAS rimesse in coda al ritorno del NAS", "n", len(ids), "job", ids)
	}
	return len(ids)
}

func (e *EsecutoreServer) esegui(ctx context.Context, q *db.Queries, j *db.Job, t coda.Tentativo) (any, error) {
	switch j.Tipo {
	case db.TipoJobEstraiArchivio:
		var p worker.PayloadEstraiArchivio
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		if e.Archivi == nil {
			return nil, errors.New("estrazione degli archivi non configurata su questo server")
		}
		voci, err := e.Archivi.EstraiArchivio(ctx, p.AllegatoID, t.LeaseToken)
		if err != nil {
			return nil, err
		}
		return map[string]any{"allegato_id": p.AllegatoID, "voci": voci}, nil

	case db.TipoJobAnalizzaMessaggioAi:
		var p worker.PayloadAnalizzaMessaggioAI
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		a, err := e.Agente.Analizza(ctx, q, p.MessaggioID)
		if err != nil {
			return nil, err
		}
		// l'esito del job dice che cosa e' successo, compreso «rifiutata»: un output non conforme non
		// e' un errore del sistema, ed e' una misura che va conservata
		return map[string]any{"analisi_id": a.AnalisiID, "stato": a.Stato,
			"scartati": len(a.Scartato), "token_in": a.TokenIn.Int32, "token_out": a.TokenOut.Int32}, nil

	case db.TipoJobCreaCartellaThread:
		var p worker.PayloadCreaCartellaThread
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		t, err := q.GetThread(ctx, p.ThreadID)
		if err != nil {
			return nil, err
		}
		if !t.CartellaRelativa.Valid {
			return nil, fmt.Errorf("thread %s senza cartella_relativa", p.ThreadID)
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
		base, err := e.NAS.CreaCartella(t.CartellaRelativa.String, sotto)
		if err != nil {
			return nil, err
		}
		if !e.NAS.DryRun {
			if err := q.SetCartellaCreata(ctx, p.ThreadID); err != nil {
				return nil, err
			}
		}
		return map[string]any{"cartella": base, "dry_run": e.NAS.DryRun}, nil

	case db.TipoJobCopiaNas:
		var p worker.PayloadCopiaNAS
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return nil, err
		}
		d, err := q.GetDocumento(ctx, p.DocumentoID)
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
		src, sparito, err := e.cercaSorgente(ctx, q, d)
		if sparito != nil {
			// Il contenuto non c'e' piu' nella cache. L'operatore ha gia' deciso che quel file va sul
			// NAS, e il file e' ancora in Outlook o dentro il suo archivio: la ripresa si accoda da
			// sola (Pre-7), e questo tentativo fallisce dicendo che cosa sta succedendo.
			err = e.riprendiContenuto(ctx, q, j, *sparito, err)
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
		dst, err := e.NAS.Copia(src, t.CartellaRelativa.String, d.PathRelativo, d.Sha256)
		if err != nil {
			_ = q.SetDocumentoErrore(ctx, db.SetDocumentoErroreParams{DocumentoID: d.DocumentoID, ErroreNas: pgtype.Text{String: err.Error(), Valid: true}})
			return nil, err
		}
		if !e.NAS.DryRun {
			if err := q.SetDocumentoScritto(ctx, d.DocumentoID); err != nil {
				return nil, err
			}
		}
		// Il contenuto e' stato USATO, non consumato: resta nella cache (Pre-7, D31), e l'orario
		// rinfrescato dice al custode che serve ancora.
		staging.ToccaContenuto(src)
		return map[string]any{"destinazione": dst, "dry_run": e.NAS.DryRun}, nil
	}
	return nil, fmt.Errorf("tipo job non gestito dal server: %s", j.Tipo)
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
func (e *EsecutoreServer) sorgenteStaging(ctx context.Context, q *db.Queries, d db.Documento) (string, error) {
	src, _, err := e.cercaSorgente(ctx, q, d)
	return src, err
}

// cercaSorgente e' sorgenteStaging che dice anche QUALE allegato ha perso il file, perche' la
// ripresa (riprendiContenuto) ha bisogno dell'allegato, non del suo nome. Dal Pre-7 conta anche un
// allegato con l'hash giusto e senza percorso: e' cosi' che lo lascia il custode della cache.
func (e *EsecutoreServer) cercaSorgente(ctx context.Context, q *db.Queries, d db.Documento) (string, *db.Allegato, error) {
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
func (e *EsecutoreServer) riprendiContenuto(ctx context.Context, q *db.Queries, j *db.Job, a db.Allegato, originale error) error {
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
