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

// Opzioni sono i vincoli di destinazione e di durata di un job. Lo zero vale «come da tipo».
type Opzioni struct {
	Casella     uuid.NullUUID // NULL = nessun vincolo di casella
	Postazione  uuid.NullUUID // job interattivo: la postazione del richiedente, senza fallback
	RichiestoDa uuid.NullUUID
	ScadeIl     *time.Time // oltre questa ora il job non serve più: annullato, mai eseguito
	LeaseS      int
	DurataMaxS  int
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
	j, err := q.InsertJob(ctx, db.InsertJobParams{
		Tipo: tipo, WorkerTipo: WorkerPer(tipo), Payload: raw, ChiaveIdempotenza: ch, Priorita: priorita,
		LeaseS: int32(o.LeaseS), DurataMaxS: int32(o.DurataMaxS),
		CasellaID: o.Casella, PostazioneID: o.Postazione, RichiestoDa: o.RichiestoDa, ScadeIl: o.ScadeIl,
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
	CasellaDefault  string // indirizzo: il sync periodico è per casella (chiave fissa)
	RetentionGiorni int    // 0 = nessuna cancellazione
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

// accodaSync crea il job sync_outlook con i cursori correnti.
//
// La chiave è FISSA per casella, non per finestra temporale: se il worker è fermo dieci minuti, i tick
// dello scheduler non accumulano dieci job da smaltire uno dopo l'altro: ne resta al più uno pendente,
// e al tick successivo alla chiusura se ne accoda uno nuovo con i cursori aggiornati (N48, test Q13).
func (s *Scheduler) accodaSync(ctx context.Context) error {
	if s.CasellaDefault == "" {
		return nil // nessuna casella censita: niente da sincronizzare
	}
	casella, err := s.Q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: s.CasellaDefault})
	if errors.Is(err, pgx.ErrNoRows) {
		s.Log.Warn("sync non accodato: casella predefinita non censita", "casella", s.CasellaDefault)
		return nil
	}
	if err != nil {
		return err
	}
	cursori, err := s.Q.ListSyncCursori(ctx)
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
