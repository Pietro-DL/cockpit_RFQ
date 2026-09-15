//go:build integrazione

// L4 — la coda contro PostgreSQL vero (fase 1, voci 1.5 e 1.6).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/jobs/
//
// Il filo conduttore è uno solo: un tentativo che non vale più non deve poter scrivere niente. Ogni
// test qui prova un punto in cui, prima della fase 1, ci sarebbe riuscito.
package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/testutil"
)

func preparaDB(t *testing.T) (*pgxpool.Pool, *db.Queries, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	return p, db.New(p), context.Background()
}

// accoda mette un job pronto e restituisce la riga.
func accoda(t *testing.T, ctx context.Context, q *db.Queries, tipo db.TipoJob, chiave string, o Opzioni) db.Job {
	t.Helper()
	j, err := AccodaCon(ctx, q, tipo, map[string]any{"prova": true}, chiave, 5, o)
	if err != nil {
		t.Fatalf("accoda %s: %v", tipo, err)
	}
	if j == nil {
		t.Fatalf("accoda %s: nessun job (chiave %q già pendente?)", tipo, chiave)
	}
	return *j
}

func claim(t *testing.T, ctx context.Context, q *db.Queries, worker db.WorkerTipo, id string) *db.Job {
	t.Helper()
	j, err := Claim(ctx, q, worker, id, Destinazione{}, 0)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return j
}

func statoDi(t *testing.T, ctx context.Context, q *db.Queries, id int64) db.Job {
	t.Helper()
	j, err := q.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("get job %d: %v", id, err)
	}
	return j
}

// Q4 — il lease scaduto rimette il job in coda; esaurite le prove diventa fallito, non eterno.
func TestLeaseScadutoRiaccodaPoiFallisce(t *testing.T) {
	p, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "q4", Opzioni{})
	if _, err := p.Exec(ctx, `UPDATE job SET max_tentativi = 2 WHERE job_id = $1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	for giro := 1; giro <= 2; giro++ {
		c := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC")
		if c == nil {
			t.Fatalf("giro %d: nessun job da prendere", giro)
		}
		if _, err := p.Exec(ctx, `UPDATE job SET lease_fino_a = now() - interval '1 minute' WHERE job_id = $1`, j.JobID); err != nil {
			t.Fatal(err)
		}
		if _, err := q.RilasciaLeaseScaduti(ctx); err != nil {
			t.Fatal(err)
		}
		atteso := db.StatoJobPronto
		if giro == 2 {
			atteso = db.StatoJobFallito
		}
		if s := statoDi(t, ctx, q, j.JobID); s.Stato != atteso {
			t.Fatalf("giro %d: stato %s, atteso %s (errore: %q)", giro, s.Stato, atteso, s.Errore.String)
		}
	}
}

// Q15 — un risultato che arriva dal tentativo A dopo che il job è passato al tentativo B non deve
// essere applicato. Prima della fase 1 il predicato guardava solo worker_id: con lo stesso worker
// che riprende lo stesso job, A e B erano indistinguibili.
func TestRisultatoTardivoConTokenVecchioRifiutato(t *testing.T) {
	p, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "q15", Opzioni{})
	a := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC")
	tentativoA := Tentativo{JobID: j.JobID, LeaseToken: a.LeaseToken.UUID, WorkerID: "analisi@PC"}

	// il lease di A scade e lo scheduler rimette il job in coda; B lo riprende (stesso worker!)
	if _, err := p.Exec(ctx, `UPDATE job SET lease_fino_a = now() - interval '1 minute' WHERE job_id = $1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RilasciaLeaseScaduti(ctx); err != nil {
		t.Fatal(err)
	}
	b := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC")
	if b == nil {
		t.Fatal("il tentativo B non ha potuto prendere il job")
	}
	if b.LeaseToken.UUID == tentativoA.LeaseToken {
		t.Fatal("il claim ha riusato lo stesso lease_token: i due tentativi sarebbero indistinguibili")
	}

	if _, err := Completa(ctx, q, tentativoA, json.RawMessage(`{"da":"A"}`)); err != ErrTentativoNonValido {
		t.Fatalf("il risultato di A è stato accettato (err=%v)", err)
	}
	if s := statoDi(t, ctx, q, j.JobID); s.Stato != db.StatoJobInCorso || s.Risultato != nil {
		t.Fatalf("il job non è più in corso con B: stato=%s risultato=%v", s.Stato, s.Risultato)
	}

	tentativoB := Tentativo{JobID: j.JobID, LeaseToken: b.LeaseToken.UUID, WorkerID: "analisi@PC"}
	if _, err := Completa(ctx, q, tentativoB, json.RawMessage(`{"da":"B"}`)); err != nil {
		t.Fatalf("il risultato di B doveva essere applicato: %v", err)
	}
	if s := statoDi(t, ctx, q, j.JobID); s.Stato != db.StatoJobFatto {
		t.Fatalf("stato finale %s, atteso fatto", s.Stato)
	}
}

// Q19 — anche il FALLIMENTO riportato da un tentativo scaduto va rifiutato: altrimenti il tentativo
// vecchio incrementa i tentativi e marca in errore l'entità su cui quello nuovo sta lavorando bene.
func TestFallimentoConTokenVecchioRifiutato(t *testing.T) {
	p, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "q19", Opzioni{})
	a := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC")
	tentativoA := Tentativo{JobID: j.JobID, LeaseToken: a.LeaseToken.UUID, WorkerID: "analisi@PC"}
	if _, err := p.Exec(ctx, `UPDATE job SET lease_fino_a = now() - interval '1 minute' WHERE job_id = $1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RilasciaLeaseScaduti(ctx); err != nil {
		t.Fatal(err)
	}
	b := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC")
	prima := statoDi(t, ctx, q, j.JobID)

	if _, err := Fallisci(ctx, q, tentativoA, "errore di A", true); err != ErrTentativoNonValido {
		t.Fatalf("il fallimento di A è stato accettato (err=%v)", err)
	}
	dopo := statoDi(t, ctx, q, j.JobID)
	// l'errore atteso è ancora quello lasciato dal rilascio del lease, non «errore di A»
	if dopo.Stato != db.StatoJobInCorso || dopo.Tentativi != prima.Tentativi || dopo.Errore.String != prima.Errore.String {
		t.Fatalf("il fallimento di A ha toccato il job: stato=%s tentativi=%d→%d errore=%q (prima %q)",
			dopo.Stato, prima.Tentativi, dopo.Tentativi, dopo.Errore.String, prima.Errore.String)
	}
	if dopo.LeaseToken.UUID != b.LeaseToken.UUID {
		t.Fatal("il token del tentativo in corso è cambiato")
	}
}

// Q16/Q20 — durata massima. Un worker che rinnova il lease ogni 30 s terrebbe il job per sempre:
// oltre avviato_il + durata_max_s il tentativo non vale più, né per il heartbeat né per il result.
func TestDurataMassimaFermaIlTentativoAncheConLeaseFresco(t *testing.T) {
	p, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "q16", Opzioni{DurataMaxS: 1, LeaseS: 300})
	c := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC")
	tent := Tentativo{JobID: j.JobID, LeaseToken: c.LeaseToken.UUID, WorkerID: "analisi@PC"}
	if err := Batte(ctx, q, tent); err != nil {
		t.Fatalf("il primo battito doveva passare: %v", err)
	}

	// si sposta indietro l'inizio del tentativo invece di aspettare: la prova è sul predicato
	if _, err := p.Exec(ctx, `UPDATE job SET avviato_il = now() - interval '10 seconds' WHERE job_id = $1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	if err := Batte(ctx, q, tent); err != ErrTentativoNonValido {
		t.Fatalf("il battito oltre la durata massima è passato (err=%v)", err)
	}
	if s := statoDi(t, ctx, q, j.JobID); s.LeaseFinoA == nil || !s.LeaseFinoA.After(time.Now()) {
		t.Fatal("il battito rifiutato ha comunque rinnovato il lease")
	}
	if _, err := Completa(ctx, q, tent, json.RawMessage(`{}`)); err != ErrTentativoNonValido {
		t.Fatalf("il risultato oltre la durata massima è stato applicato (err=%v)", err)
	}

	// chi riceve il 409 riporta il job in coda con il motivo scritto, invece di lasciarlo appeso
	if !ChiudiSeDurataSuperata(ctx, q, j.JobID) {
		t.Fatal("il job non è stato riaccodato per durata massima")
	}
	s := statoDi(t, ctx, q, j.JobID)
	if s.Stato != db.StatoJobPronto || s.Errore.String != "durata massima superata" || s.LeaseToken.Valid {
		t.Fatalf("dopo la durata massima: stato=%s errore=%q token_ancora_presente=%v", s.Stato, s.Errore.String, s.LeaseToken.Valid)
	}
	if nuovo := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC"); nuovo == nil {
		t.Fatal("il job riaccodato non è ripartibile")
	}
}

// Q21 — un job interattivo già scaduto non deve essere assegnato a nessuno, nemmeno se lo scheduler
// non è ancora passato ad annullarlo: aprire una finestra su un messaggio chiesto mezz'ora fa è
// peggio che non aprirla.
func TestJobScadutoNonVieneAssegnatoEPoiAnnullato(t *testing.T) {
	_, q, ctx := preparaDB(t)
	passato := time.Now().Add(-time.Minute)
	scaduto := accoda(t, ctx, q, db.TipoJobApriElementoOutlook, "q21-scaduto", Opzioni{ScadeIl: &passato})
	futuro := time.Now().Add(time.Hour)
	vivo := accoda(t, ctx, q, db.TipoJobApriElementoOutlook, "q21-vivo", Opzioni{ScadeIl: &futuro})

	c := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC")
	if c == nil || c.JobID != vivo.JobID {
		t.Fatalf("il claim ha restituito %v, atteso il job non scaduto %d", c, vivo.JobID)
	}
	n, err := q.AnnullaJobScaduti(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("annullati %d job, atteso 1", n)
	}
	if s := statoDi(t, ctx, q, scaduto.JobID); s.Stato != db.StatoJobAnnullato {
		t.Fatalf("il job scaduto è in stato %s, atteso annullato", s.Stato)
	}
}

// Q10 — riaccodare un job fallito quando ne esiste già uno pendente con la stessa chiave violerebbe
// l'indice unico parziale. Prima era un 500 in faccia all'operatore; ora è zero righe, cioè un avviso.
func TestRiaccodaConChiavePendenteNonEsplode(t *testing.T) {
	p, q, ctx := preparaDB(t)
	vecchio := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "analizza:x", Opzioni{})
	if _, err := p.Exec(ctx, `UPDATE job SET stato = 'fallito', chiuso_il = now() WHERE job_id = $1`, vecchio.JobID); err != nil {
		t.Fatal(err)
	}
	nuovo := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "analizza:x", Opzioni{}) // stessa chiave, ora pendente

	if _, err := q.RiaccodaJob(ctx, vecchio.JobID); err == nil {
		t.Fatal("il riaccodo è riuscito: avrebbe creato due job pendenti con la stessa chiave")
	} else if err.Error() != "no rows in result set" {
		t.Fatalf("atteso «nessuna riga», ottenuto un errore diverso: %v", err)
	}
	if s := statoDi(t, ctx, q, vecchio.JobID); s.Stato != db.StatoJobFallito {
		t.Fatalf("il job fallito è stato toccato: %s", s.Stato)
	}

	// chiuso il pendente, il riaccodo funziona
	if _, err := p.Exec(ctx, `UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1`, nuovo.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RiaccodaJob(ctx, vecchio.JobID); err != nil {
		t.Fatalf("riaccodo legittimo rifiutato: %v", err)
	}
	if s := statoDi(t, ctx, q, vecchio.JobID); s.Stato != db.StatoJobPronto || s.Tentativi != 0 {
		t.Fatalf("dopo il riaccodo: stato=%s tentativi=%d", s.Stato, s.Tentativi)
	}
}

// Q12 — retention: i job chiusi da più dei giorni configurati spariscono, quelli recenti e quelli
// ancora aperti restano. Una coda che non si svuota mai diventa illeggibile.
func TestRetentionEliminaSoloIVecchiChiusi(t *testing.T) {
	p, q, ctx := preparaDB(t)
	for i := 0; i < 5; i++ {
		j := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "", Opzioni{})
		if _, err := p.Exec(ctx, `UPDATE job SET stato='fatto', chiuso_il = now() - interval '20 days' WHERE job_id=$1`, j.JobID); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		j := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "", Opzioni{})
		if _, err := p.Exec(ctx, `UPDATE job SET stato='fallito', chiuso_il = now() - interval '1 day' WHERE job_id=$1`, j.JobID); err != nil {
			t.Fatal(err)
		}
	}
	accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "", Opzioni{}) // pronto: non si tocca

	n, err := q.EliminaJobVecchi(ctx, 14)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("eliminati %d job, attesi 5", n)
	}
	if rimasti := testutil.Conta(t, p, "job"); rimasti != 4 {
		t.Fatalf("rimasti %d job, attesi 4 (3 recenti + 1 pronto)", rimasti)
	}
}

// Q13 — lo scheduler non accumula sync. Con la chiave per finestra temporale, dieci minuti di worker
// fermo lasciavano dieci job da smaltire uno dopo l'altro; con la chiave fissa per casella ne resta
// al più uno pendente.
func TestSyncNonSiAccumulaPerCasella(t *testing.T) {
	p, q, ctx := preparaDB(t)
	var casellaID uuid.UUID
	if err := p.QueryRow(ctx,
		`INSERT INTO casella (canale, indirizzo, nome) VALUES ('outlook','prova@azienda.it','Prova') RETURNING casella_id`,
	).Scan(&casellaID); err != nil {
		t.Fatal(err)
	}
	s := &Scheduler{Q: q, Log: testutil.LogSilenzioso(), Cartelle: []string{"Inbox"}, Lotto: 50}

	for tick := 0; tick < 5; tick++ {
		if err := s.accodaSync(ctx); err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
	}
	var pendenti int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM job WHERE tipo='sync_outlook' AND stato IN ('pronto','in_corso')`).Scan(&pendenti); err != nil {
		t.Fatal(err)
	}
	if pendenti != 1 {
		t.Fatalf("%d sync pendenti dopo 5 tick, atteso 1", pendenti)
	}
	j, err := q.JobPendentePerChiave(ctx, pgtype.Text{String: "sync_outlook:" + casellaID.String(), Valid: true})
	if err != nil {
		t.Fatalf("chiave attesa 'sync_outlook:<casella>': %v", err)
	}
	if !j.CasellaID.Valid || j.CasellaID.UUID != casellaID {
		t.Fatalf("il job non porta la casella: %v", j.CasellaID)
	}

	// chiuso il job, il tick successivo ne accoda uno nuovo: la coda riparte, non resta ferma
	if _, err := p.Exec(ctx, `UPDATE job SET stato='fatto', chiuso_il=now() WHERE job_id=$1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	if err := s.accodaSync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM job WHERE tipo='sync_outlook' AND stato='pronto'`).Scan(&pendenti); err != nil {
		t.Fatal(err)
	}
	if pendenti != 1 {
		t.Fatalf("dopo la chiusura il sync non è ripartito: %d pendenti", pendenti)
	}
}

// Q7 — l'idempotenza vale fra i job PENDENTI: un job chiuso non impedisce di riaccodarne uno uguale.
func TestIdempotenzaSoloFraPendenti(t *testing.T) {
	p, q, ctx := preparaDB(t)
	primo := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "k", Opzioni{})
	doppio, err := AccodaCon(ctx, q, db.TipoJobAnalizzaAllegato, map[string]any{}, "k", 5, Opzioni{})
	if err != nil {
		t.Fatal(err)
	}
	if doppio != nil {
		t.Fatal("accodati due job pendenti con la stessa chiave")
	}
	if _, err := p.Exec(ctx, `UPDATE job SET stato='fatto', chiuso_il=now() WHERE job_id=$1`, primo.JobID); err != nil {
		t.Fatal(err)
	}
	if terzo, err := AccodaCon(ctx, q, db.TipoJobAnalizzaAllegato, map[string]any{}, "k", 5, Opzioni{}); err != nil || terzo == nil {
		t.Fatalf("dopo la chiusura la chiave deve tornare libera (job=%v err=%v)", terzo, err)
	}
}
