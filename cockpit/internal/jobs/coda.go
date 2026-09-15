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
	case db.TipoJobSyncOutlook, db.TipoJobCopiaNas, db.TipoJobBackupDb:
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
func AccodaCon(ctx context.Context, q *db.Queries, tipo db.TipoJob, payload any, chiave string, priorita int16, o Opzioni) (*db.Job, error) {
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

// Claim prova a prendere un job per il worker indicato, con long-poll fino a attesa.
func Claim(ctx context.Context, q *db.Queries, worker db.WorkerTipo, workerID string, attesa time.Duration) (*db.Job, error) {
	scadenza := time.Now().Add(attesa)
	for {
		j, err := q.ClaimJob(ctx, db.ClaimJobParams{WorkerID: pgtype.Text{String: workerID, Valid: true}, WorkerTipo: worker})
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
	Lotto           int
	RetentionGiorni int // 0 = nessuna cancellazione
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
		if err := s.accodaSyncCasella(ctx, casella); err != nil {
			// una casella che non riesce non deve impedire il sync delle altre: è lo stesso principio
			// del lotto che non si ferma al primo elemento rotto
			s.Log.Error("sync non accodato", "casella", casella.Indirizzo, "err", err)
		}
	}
	return nil
}

func (s *Scheduler) accodaSyncCasella(ctx context.Context, casella db.Casella) error {
	cursori, err := s.Q.ListSyncCursoriCasella(ctx, casella.CasellaID)
	if err != nil {
		return err
	}
	perCartella := map[string]*time.Time{}
	var piuVecchio *time.Time
	for _, c := range cursori {
		perCartella[c.Cartella] = c.UltimoReceived
		if c.UltimoReceived != nil {
			if piuVecchio == nil || c.UltimoReceived.Before(*piuVecchio) {
				piuVecchio = c.UltimoReceived
			}
		}
	}
	dal := s.Dal
	if dal.IsZero() {
		if piuVecchio != nil {
			dal = *piuVecchio
		} else {
			dal = time.Now().AddDate(0, -1, 0)
		}
	}
	cid := casella.CasellaID
	p := api.PayloadSyncOutlook{Dal: dal, SovrapposizioneS: 600, Lotto: s.Lotto, CasellaID: &cid}
	for _, c := range s.Cartelle {
		p.Cartelle = append(p.Cartelle, api.CartellaCursore{Cartella: c, UltimoReceived: perCartella[c]})
	}
	_, err = AccodaCon(ctx, s.Q, db.TipoJobSyncOutlook, p, "sync_outlook:"+cid.String(), 5,
		Opzioni{Casella: uuid.NullUUID{UUID: cid, Valid: true}})
	return err
}
