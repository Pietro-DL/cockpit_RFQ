// Package jobs gestisce la coda in PostgreSQL: accodamento idempotente, claim con lease,
// scheduler (lease scaduti, sync_outlook periodico) ed esecuzione dei job di tipo 'server'.
//
// Dalla fase 1 il TENTATIVO è un'entità con un'identità propria: ogni claim genera un lease_token e
// fissa avviato_il. Tutto ciò che un worker scrive dopo — heartbeat, risultato, errore, ingest — deve
// esibire quel token. Un tentativo scaduto che si risveglia trova zero righe e riceve 409, invece di
// applicare il proprio lavoro sopra a quello del tentativo che gli è subentrato.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// WorkerPer restituisce il worker che esegue un tipo di job.
func WorkerPer(t db.TipoJob) db.WorkerTipo {
	switch t {
	case db.TipoJobSyncOutlook, db.TipoJobStageAllegato, db.TipoJobCreaBozzaOutlook,
		db.TipoJobApriElementoOutlook, db.TipoJobSpostaInCartella, db.TipoJobSegnaLetto,
		db.TipoJobRileggiElemento:
		return db.WorkerTipoOutlook
	case db.TipoJobAnalizzaAllegato:
		return db.WorkerTipoAnalisi
	default:
		return db.WorkerTipoServer
	}
}

// LeaseSecondi per tipo: i job brevi hanno lease corto, sync e copie NAS più lungo.
func LeaseSecondi(t db.TipoJob) int {
	switch t {
	case db.TipoJobSyncOutlook, db.TipoJobCopiaNas, db.TipoJobBackupDb, db.TipoJobEstraiArchivio:
		return 300
	default:
		return 120
	}
}

// DurataMassimaS limita l'INTERO tentativo, non il singolo lease. Un worker che rinnova il lease ogni
// 30 secondi lo terrebbe altrimenti per sempre: un job bloccato dentro una chiamata COM che non
// ritorna resterebbe «in corso» finché qualcuno non se ne accorge.
func DurataMassimaS(t db.TipoJob) int {
	switch t {
	case db.TipoJobSyncOutlook:
		return 1800
	case db.TipoJobCopiaNas, db.TipoJobBackupDb:
		return 3600
	default:
		return 600
	}
}

// MaxTentativiPer dice quante volte vale la pena riprovare un job prima di darlo per perso.
//
// Cinque tentativi vanno bene per un lavoro che fallisce perché è sbagliato: riprovarlo all'infinito
// non lo farebbe riuscire. Una scrittura sul NAS invece non fallisce quasi mai per questo, ma perché
// il NAS o la rete in quel momento non ci sono, e con il backoff limitato a dieci minuti cinque
// tentativi coprono poco più di mezz'ora: un fermo notturno del NAS bastava a far chiudere la copia
// come 'fallito', e il file non arrivava mai nel fascicolo senza che nessuno se ne accorgesse (N8).
//
// Cinquanta tentativi coprono circa otto ore. Oltre a quelle c'è il riaccodo automatico al ritorno
// del NAS, e i tentativi non consumati mentre il NAS è irraggiungibile (RinviaJob): è l'insieme dei
// tre a fare la voce 1.7, non il numero da solo.
func MaxTentativiPer(t db.TipoJob) int {
	switch t {
	case db.TipoJobCopiaNas, db.TipoJobCreaCartellaThread:
		return 50
	default:
		return 5
	}
}

// Opzioni sono i vincoli di destinazione e di durata di un job. Lo zero vale «come da tipo».
type Opzioni struct {
	Casella     uuid.NullUUID // NULL = nessun vincolo di casella
	Postazione  uuid.NullUUID // job interattivo: la postazione del richiedente, senza fallback
	RichiestoDa uuid.NullUUID
	ScadeIl     *time.Time // oltre questa ora il job non serve più: annullato, mai eseguito
	LeaseS      int
	DurataMaxS  int
	// MaxTentativi: zero = come da tipo (MaxTentativiPer). Non è un puntatore perché zero tentativi
	// non è una richiesta sensata, quindi non serve distinguerlo da «non l'ho scritto».
	MaxTentativi int
}

// Accoda inserisce un job nella transazione corrente. chiave vuota = nessuna idempotenza.
// Restituisce (nil, nil) se un job con la stessa chiave esiste già.
func Accoda(ctx context.Context, q *db.Queries, tipo db.TipoJob, payload any, chiave string, priorita int16) (*db.Job, error) {
	return AccodaCon(ctx, q, tipo, payload, chiave, priorita, Opzioni{})
}

// AccodaCon è Accoda con i vincoli di destinazione e durata espliciti.
//
// In modalità shadow i job che toccano il mondo fuori dal Cockpit non entrano in coda e la funzione
// restituisce ErrShadow: chi accoda su richiesta di un operatore lo riconosce e glielo dice. Non si
// restituisce (nil, nil) — che qui significa «c'era già» — perché un blocco raccontato come successo
// è un pulsante che non fa niente senza dirlo.
func AccodaCon(ctx context.Context, q *db.Queries, tipo db.TipoJob, payload any, chiave string, priorita int16, o Opzioni) (*db.Job, error) {
	if InShadow() && BloccatoInShadow(tipo) {
		return nil, fmt.Errorf("%w: %s", ErrShadow, tipo)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload %s: %w", tipo, err)
	}
	var ch pgtype.Text
	if chiave != "" {
		ch = pgtype.Text{String: chiave, Valid: true}
	}
	if o.LeaseS <= 0 {
		o.LeaseS = LeaseSecondi(tipo)
	}
	if o.DurataMaxS <= 0 {
		o.DurataMaxS = DurataMassimaS(tipo)
	}
	if o.MaxTentativi <= 0 {
		o.MaxTentativi = MaxTentativiPer(tipo)
	}
	j, err := q.InsertJob(ctx, db.InsertJobParams{
		Tipo: tipo, WorkerTipo: WorkerPer(tipo), Payload: raw, ChiaveIdempotenza: ch, Priorita: priorita,
		LeaseS: int32(o.LeaseS), DurataMaxS: int32(o.DurataMaxS),
		MaxTentativi: pgtype.Int4{Int32: int32(o.MaxTentativi), Valid: true},
		CasellaID:    o.Casella, PostazioneID: o.Postazione, RichiestoDa: o.RichiestoDa, ScadeIl: o.ScadeIl,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // già in coda
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// Destinazione è ciò che un worker può eseguire: le caselle che serve davvero (risolte nel suo
// profilo E autorizzate dalla credenziale) e la postazione su cui gira. Lo zero vale «nessuna
// casella, nessuna postazione»: prende solo job senza vincoli (analisi, server).
type Destinazione struct {
	Caselle    []uuid.UUID
	Postazione uuid.NullUUID
}

// Claim prova a prendere un job per il worker indicato, con long-poll fino a attesa. Il job deve
// essere eseguibile da QUESTA destinazione (voce 2.2): un job di una casella che il worker non serve,
// o di un'altra postazione, non gli viene assegnato nemmeno se è l'unico worker acceso.
func Claim(ctx context.Context, q *db.Queries, worker db.WorkerTipo, workerID string, d Destinazione, attesa time.Duration) (*db.Job, error) {
	scadenza := time.Now().Add(attesa)
	caselle := d.Caselle
	if caselle == nil {
		caselle = []uuid.UUID{} // un array vuoto, non NULL: `= ANY (NULL)` non è mai vero né falso
	}
	for {
		j, err := q.ClaimJob(ctx, db.ClaimJobParams{WorkerID: pgtype.Text{String: workerID, Valid: true}, WorkerTipo: worker,
			Caselle: caselle, Postazione: d.Postazione, TipiEsclusi: TipiBloccatiOra()})
		if err == nil {
			return &j, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if time.Now().After(scadenza) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(1 * time.Second):
		}
	}
}

// Tentativo è la chiave con cui un worker prova di essere ancora quello che sta eseguendo il job.
type Tentativo struct {
	JobID      int64
	LeaseToken uuid.UUID
	WorkerID   string
}

// ErrTentativoNonValido: il job non è più in corso con questo token e questo worker, o ha superato la
// durata massima. Chi riceve questo errore deve rispondere 409 e non applicare nulla.
var ErrTentativoNonValido = errors.New("tentativo non più valido")

func (t Tentativo) token() uuid.NullUUID { return uuid.NullUUID{UUID: t.LeaseToken, Valid: true} }
func (t Tentativo) worker() pgtype.Text  { return pgtype.Text{String: t.WorkerID, Valid: true} }

// Completa chiude il job come riuscito, ma solo se il tentativo è ancora valido.
func Completa(ctx context.Context, q *db.Queries, t Tentativo, risultato json.RawMessage) (db.Job, error) {
	j, err := q.CompletaJob(ctx, db.CompletaJobParams{
		JobID: t.JobID, LeaseToken: t.token(), WorkerID: t.worker(), Risultato: &risultato,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrTentativoNonValido
	}
	return j, err
}

// Fallisci registra l'errore riportato dal worker, ma solo se il tentativo è ancora valido.
func Fallisci(ctx context.Context, q *db.Queries, t Tentativo, errore string, definitivo bool) (db.Job, error) {
	j, err := q.FallisciJob(ctx, db.FallisciJobParams{
		JobID: t.JobID, LeaseToken: t.token(), WorkerID: t.worker(),
		Errore: pgtype.Text{String: errore, Valid: true}, Definitivo: definitivo,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrTentativoNonValido
	}
	return j, err
}

// Rinvia rimette il job in coda senza consumare il tentativo: il lavoro non è nemmeno cominciato.
//
// È diverso da Fallisci con definitivo=false, che invece il tentativo lo conta. La differenza pesa
// solo in un caso, ma è il caso per cui la voce 1.7 esiste: se il NAS manca per ore, contare ogni
// giro a vuoto come un fallimento esaurisce il budget dei tentativi mentre il problema è fuori dal
// nostro programma, e la copia viene dichiarata persa proprio perché ci abbiamo provato tante volte.
func Rinvia(ctx context.Context, q *db.Queries, t Tentativo, fra time.Duration, motivo string) (db.Job, error) {
	j, err := q.RinviaJob(ctx, db.RinviaJobParams{
		JobID: t.JobID, LeaseToken: t.token(), WorkerID: t.worker(),
		FraS: int32(fra / time.Second), Motivo: pgtype.Text{String: motivo, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrTentativoNonValido
	}
	return j, err
}

// Verifica dice se il tentativo vale ancora, senza bloccare la riga. È il controllo dell'upload
// (voce 2.3): prima e dopo un trasferimento che non può stare dentro una transazione.
func Verifica(ctx context.Context, q *db.Queries, t Tentativo) (db.Job, error) {
	j, err := q.VerificaTentativo(ctx, db.VerificaTentativoParams{JobID: t.JobID, LeaseToken: t.token(), WorkerID: t.worker()})
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrTentativoNonValido
	}
	return j, err
}

// Blocca verifica il tentativo E blocca la riga del job fino alla fine della transazione: da qui al
// commit nessun altro tentativo può diventare valido. È ciò che rende sicura la promozione di un
// file da .parte a definitivo dentro il result (voce 2.3): la rinomina non si può annullare con un
// rollback, quindi deve avvenire quando nessun altro può più vincere il job.
func Blocca(ctx context.Context, q *db.Queries, t Tentativo) (db.Job, error) {
	j, err := q.BloccaTentativo(ctx, db.BloccaTentativoParams{JobID: t.JobID, LeaseToken: t.token(), WorkerID: t.worker()})
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrTentativoNonValido
	}
	return j, err
}

// Batte rinnova il lease. Restituisce ErrTentativoNonValido se il tentativo non vale più: è il segnale
// che il worker usa per fermare il lavoro in corso invece di portarlo a termine per niente.
func Batte(ctx context.Context, q *db.Queries, t Tentativo) error {
	n, err := q.HeartbeatJob(ctx, db.HeartbeatJobParams{JobID: t.JobID, LeaseToken: t.token(), WorkerID: t.worker()})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTentativoNonValido
	}
	return nil
}

// ChiudiSeDurataSuperata riporta il job a 'pronto' quando il tentativo è caduto per durata massima.
// Senza, il job resterebbe «in corso» fino alla scadenza del lease pur non avendo più nessuno che lo
// sta eseguendo. Restituisce true se ha agito.
func ChiudiSeDurataSuperata(ctx context.Context, q *db.Queries, jobID int64) bool {
	n, err := q.ScadutoPerDurataMassima(ctx, jobID)
	return err == nil && n > 0
}

// InJob converte una riga db.Job nel contratto verso il worker.
func InJob(j *db.Job) api.Job {
	out := api.Job{
		JobID: j.JobID, Tipo: string(j.Tipo), Payload: j.Payload, Tentativi: int(j.Tentativi),
		LeaseS: int(j.LeaseS), DurataMaxS: int(j.DurataMaxS),
	}
	if j.LeaseToken.Valid {
		out.LeaseToken = j.LeaseToken.UUID.String()
	}
	if j.CasellaID.Valid {
		c := j.CasellaID.UUID
		out.CasellaID = &c
	}
	if j.PostazioneID.Valid {
		p := j.PostazioneID.UUID
		out.PostazioneID = &p
	}
	return out
}

// Scheduler gira nel server: libera i lease scaduti, annulla i job interattivi scaduti, accoda il
// sync_outlook periodico, pulisce le sessioni e applica la retention della coda.
type Scheduler struct {
	Q               *db.Queries
	Log             *slog.Logger
	Cartelle        []string
	IntervalloSync  time.Duration
	Dal             time.Time
	GiorniIniziali  int
	Lotto           int
	RetentionGiorni int // 0 = nessuna cancellazione
	// Staging: dove stanno i file caricati dai worker. Vuoto = nessuna pulizia dei .parte orfani.
	Staging string
	// RetentionStagingGiorni: da quanti giorni un CONTENUTO deve essere fermo perche' la pulizia lo
	// tolga, e solo se nessun allegato porta piu' quel sha256. 0 = non cancellare niente.
	//
	// E' una voce separata da RetentionGiorni perche' le due cose non si somigliano: quella toglie
	// righe di coda gia' chiuse, questa toglie FILE. Un file cancellato per sbaglio si riscarica solo
	// se l'elemento e' ancora in Outlook, quindi la soglia deve poterla decidere chi conosce
	// l'azienda, e in assenza vale «non cancellare».
	RetentionStagingGiorni int
}

func (s *Scheduler) Avvia(ctx context.Context) {
	go s.loop(ctx, 30*time.Second, "lease", func(ctx context.Context) error {
		n, err := s.Q.RilasciaLeaseScaduti(ctx)
		if n > 0 {
			s.Log.Warn("lease scaduti rilasciati", "n", n)
		}
		if err != nil {
			return err
		}
		a, err := s.Q.AnnullaJobScaduti(ctx)
		if a > 0 {
			s.Log.Info("job interattivi scaduti annullati", "n", a)
		}
		return err
	})
	if s.IntervalloSync > 0 {
		go s.loop(ctx, s.IntervalloSync, "sync_outlook", s.accodaSync)
	}
	go s.loop(ctx, time.Hour, "sessioni", func(ctx context.Context) error {
		_, err := s.Q.EliminaSessioniScadute(ctx)
		return err
	})
	if s.Staging != "" {
		go s.loop(ctx, 15*time.Minute, "parti", func(ctx context.Context) error {
			n, err := PulisciParti(ctx, s.Q, s.Staging, 10*time.Minute, s.Log)
			if n > 0 {
				s.Log.Info("file parziali orfani rimossi dallo staging", "n", n)
			}
			return err
		})
	}
	if s.Staging != "" && s.RetentionStagingGiorni > 0 {
		// Sei ore, come la retention della coda: e' una pulizia, non una reazione. L'attesa di dieci
		// minuti protegge il contenuto appena promosso, che per un istante non e' nominato da nessuno.
		go s.loop(ctx, 6*time.Hour, "contenuti", func(ctx context.Context) error {
			n, liberati, err := PulisciContenuti(ctx, s.Q, s.Staging, s.RetentionStagingGiorni, 10*time.Minute, s.Log)
			if n > 0 {
				s.Log.Info("contenuti rimossi dallo staging", "n", n, "MB", liberati>>20, "giorni", s.RetentionStagingGiorni)
			}
			return err
		})
	}
	if s.RetentionGiorni > 0 {
		go s.loop(ctx, 6*time.Hour, "retention", func(ctx context.Context) error {
			n, err := s.Q.EliminaJobVecchi(ctx, int32(s.RetentionGiorni))
			if n > 0 {
				s.Log.Info("job chiusi eliminati per retention", "n", n, "giorni", s.RetentionGiorni)
			}
			return err
		})
	}
}

func (s *Scheduler) loop(ctx context.Context, ogni time.Duration, nome string, f func(context.Context) error) {
	t := time.NewTicker(ogni)
	defer t.Stop()
	if err := f(ctx); err != nil {
		s.Log.Error("scheduler", "task", nome, "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := f(ctx); err != nil {
				s.Log.Error("scheduler", "task", nome, "err", err)
			}
		}
	}
}

// accodaSync crea un job sync_outlook PER CASELLA ATTIVA, con i cursori di quella casella.
//
// Dalla 0004 le caselle attive possono essere più di una, e ognuna ha i suoi cursori: un job solo,
// con i cursori di un'altra casella, farebbe rileggere la finestra sbagliata — o salterebbe del
// tutto i messaggi arrivati nel frattempo nella casella non sincronizzata.
//
// La chiave resta FISSA per casella, non per finestra temporale: se il worker è fermo dieci minuti,
// i tick dello scheduler non accumulano dieci job da smaltire uno dopo l'altro; ne resta al più uno
// pendente per casella, e al tick successivo alla chiusura se ne accoda uno nuovo con i cursori
// aggiornati (N48, test Q13).
func (s *Scheduler) accodaSync(ctx context.Context) error {
	caselle, err := s.Q.ListCaselleAttive(ctx)
	if err != nil {
		return err
	}
	if len(caselle) == 0 {
		return nil // nessuna casella attiva: niente da sincronizzare
	}
	for _, casella := range caselle {
		if casella.Canale != db.CanaleOutlook {
			continue
		}
		if _, err := AccodaSyncCasella(ctx, s.Q, casella, SyncOpzioni{Cartelle: s.Cartelle, Dal: s.Dal, GiorniIniziali: s.GiorniIniziali, Lotto: s.Lotto}); err != nil {
			// una casella che non riesce non deve impedire il sync delle altre: è lo stesso principio
			// del lotto che non si ferma al primo elemento rotto
			s.Log.Error("sync non accodato", "casella", casella.Indirizzo, "err", err)
		}
	}
	return nil
}

// GiorniSyncInizialeDefault è la finestra del PRIMO sync di una (casella, cartella): sette giorni.
//
// Sette e non trenta perché il primo caricamento è l'unico momento in cui il worker legge davvero
// tutto — corpo e allegati di ogni elemento, non la sola enumerazione, che è veloce — e su una
// casella viva sono ore durante le quali il worker, che è seriale, non apre finestre in Outlook e
// non scarica allegati per chi sta usando il Cockpit. L'archivio si prende dopo, a pezzi piccoli,
// con «Carica precedenti» (voce 2.8).
const GiorniSyncInizialeDefault = 7

// SovrapposizioneSync è quanto si riparte INDIETRO rispetto alla copertura già raggiunta, a ogni
// aggiornamento ordinario. Dieci minuti.
//
// Non è prudenza generica: è la misura di quanto due orologi e un indice di Outlook possono non
// essere d'accordo su «quando è arrivata questa mail». Il `ReceivedTime` che il worker legge, l'ora
// del server che fissa il limite superiore della finestra e il momento in cui Exchange consegna
// davvero non coincidono, e senza sovrapposizione una mail che si materializza appena sotto la
// frontiera resta sotto per sempre.
//
// Il costo di riprenderla è una rilettura di dieci minuti di posta, che la deduplica per Message-ID
// assorbe: la stessa mail rivista è un UPDATE. Il costo di non riprenderla è una mail persa in
// silenzio, e questo blocco esiste perché quel costo non sia pagabile.
const SovrapposizioneSync = 10 * time.Minute

// PrioritaSyncStorico è l'ULTIMA priorità della coda: si claima in ordine crescente
// (`ORDER BY j.priorita, j.job_id`), e l'archivio è ciò che può aspettare.
//
// Accanto ci sono 1 per i job interattivi («Apri in Outlook», bozza, copia sul NAS), 2 per «segna
// letto» e 5 per il sync ordinario. Il sync storico stava a 2, cioè davanti al sync ordinario e alla
// pari con i job di un operatore che sta guardando la schermata: con un worker Outlook solo per PC,
// quello è un pulsante che non risponde finché l'archivio non ha finito.
const PrioritaSyncStorico int16 = 9

// SyncOpzioni è ciò che serve per accodare un sync ordinario: le cartelle da leggere, la finestra da
// cui parte una cartella che non ha ancora un cursore e la dimensione del lotto. Vengono da
// [outlook] di cockpit.toml e sono le stesse per lo scheduler e per «Aggiorna ora» — di proposito:
// due sync della stessa casella con cartelle diverse farebbero avanzare il cursore su una finestra
// che l'altro non ha letto.
type SyncOpzioni struct {
	Cartelle []string
	// Dal: l'override esplicito [outlook].dal, per un import controllato. Zero = nessun override.
	// Non tocca MAI una cartella che ha già un cursore: quello che si vuole importare a mano è la
	// posta di prima, e la posta di prima è «Carica precedenti».
	Dal time.Time
	// GiorniIniziali: quanto indietro parte una cartella SENZA cursore. 0 = GiorniSyncInizialeDefault.
	GiorniIniziali int
	Lotto          int
}

// ChiaveSyncCasella è la chiave di idempotenza del sync ordinario: FISSA per casella, senza la
// finestra temporale. È il motivo per cui «Aggiorna ora» premuto dieci volte in dieci secondi accoda
// un job solo (SV1), e per cui i tick dello scheduler non accumulano coda mentre il worker è fermo.
func ChiaveSyncCasella(casella uuid.UUID) string { return "sync_outlook:" + casella.String() }

// AccodaSyncCasella accoda l'aggiornamento di UNA casella. Restituisce (nil, nil) se ce n'è già uno
// in coda o in corso: non è un errore, è la stessa richiesta.
//
// LA FINESTRA SI DECIDE QUI, TUTTA, E NON CAMBIA PIÙ (blocco 3 del 3R). Prima il payload portava un
// limite inferiore e nessun limite superiore: «da qui a adesso», dove «adesso» era l'istante in cui
// il worker guardava l'orologio, cioè un estremo che si sposta mentre il job gira. Una finestra che
// si allunga da sola non si può dichiarare conclusa, e senza quella dichiarazione non c'è nessun
// punto in cui sia lecito far avanzare una frontiera.
//
// Adesso: `al` è fissato qui e vale per tutte le cartelle; `dal` è calcolato qui per CIASCUNA
// cartella, dalla sua copertura meno la sovrapposizione. Il worker non calcola più niente: esegue
// l'intervallo che gli è stato dato e dice se l'ha percorso tutto.
//
// La precedenza del limite inferiore, scritta in un posto solo:
//
//  1. la COPERTURA di quella cartella (`coperto_fino_a`), quando c'è ed è utilizzabile: vince
//     sempre, meno la sovrapposizione. È anche il motivo per cui un riavvio non riporta nessuna
//     casella alla finestra iniziale: la copertura sta in database, non nel processo;
//  2. `dal`, se [outlook].dal è scritto nel file: override esplicito, per un import controllato;
//  3. altrimenti la finestra iniziale, `al - giorni_sync_iniziale`.
//
// I punti 2 e 3 valgono SOLO per le cartelle senza copertura — quelle in bootstrap. Ciò che si vuole
// importare a mano è la posta di prima, e la posta di prima è «Carica precedenti».
//
// Il ripiego era «il più vecchio dei cursori delle ALTRE cartelle». Sembra prudente e non lo è: una
// cartella aggiunta oggi a una casella sincronizzata da mesi sarebbe ripartita da mesi fa — cioè dal
// caricamento lungo che la finestra iniziale esiste per evitare — e una cartella la cui copertura è
// stata scartata perché nel futuro sarebbe ripartita da quella di un'altra, che su dove fosse
// arrivata lei non dice niente.
func AccodaSyncCasella(ctx context.Context, q *db.Queries, casella db.Casella, o SyncOpzioni) (*db.Job, error) {
	cursori, err := q.ListSyncCursoriCasella(ctx, casella.CasellaID)
	if err != nil {
		return nil, err
	}
	adesso := time.Now()
	perCartella := map[string]db.SyncCursore{}
	for _, c := range cursori {
		perCartella[c.Cartella] = c
	}
	cartelle := o.Cartelle
	if len(cartelle) == 0 {
		cartelle = []string{"Inbox", "Sent Items"}
	}
	iniziale := o.Dal
	if iniziale.IsZero() {
		giorni := o.GiorniIniziali
		if giorni < 1 {
			giorni = GiorniSyncInizialeDefault
		}
		iniziale = adesso.AddDate(0, 0, -giorni)
	}

	al := adesso
	cid := casella.CasellaID
	p := api.PayloadSyncOutlook{Modo: api.ModoBootstrap, Al: &al, CasellaID: &cid,
		SovrapposizioneS: int(SovrapposizioneSync / time.Second), Lotto: o.Lotto}
	for _, nome := range cartelle {
		c := api.CartellaCursore{Cartella: nome, Al: &al}
		cur, censita := perCartella[nome]
		coperto := (*time.Time)(nil)
		if censita {
			c.UltimoReceived = cur.UltimoReceived
			coperto = cur.CopertoFinoA
		}
		// Una copertura nel futuro non è utilizzabile: aprirebbe una finestra che comincia dopo
		// l'orologio, e quella cartella non leggerebbe più niente finché quel futuro non è passato.
		// Ce ne sono in database, scritte prima della correzione del 16/09/2026 (date di Outlook
		// prese per UTC quando erano ora locale: due ore avanti). Qui si ignorano, così la cartella
		// riparte dalla finestra iniziale e si rilegge — una rilettura costa una deduplica per
		// Message-ID, che il server fa comunque; fidarsi di quella copertura costerebbe la posta di
		// due ore.
		if coperto != nil && api.NelFuturo(*coperto, adesso) {
			coperto = nil
		}
		dal := iniziale
		if coperto != nil {
			c.CopertoFinoA, dal = coperto, coperto.Add(-SovrapposizioneSync)
			p.Modo = api.ModoAggiornamento
		} else {
			c.Bootstrap = true
		}
		c.Dal = &dal
		if p.Dal.IsZero() || dal.Before(p.Dal) {
			p.Dal = dal
		}
		p.Cartelle = append(p.Cartelle, c)
	}
	return AccodaCon(ctx, q, db.TipoJobSyncOutlook, p, ChiaveSyncCasella(cid), 5,
		Opzioni{Casella: uuid.NullUUID{UUID: cid, Valid: true}})
}
