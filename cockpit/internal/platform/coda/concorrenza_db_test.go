//go:build integrazione

// L4 — Q1, Q6, Q11: i tre punti della coda che si vedono solo sotto concorrenza.
//
// Un test in fila indiana non li tocca nemmeno: il claim sembra esclusivo perché c'è un solo worker,
// il battito sembra innocuo perché nessuno lavora mentre batte, e il doppio risultato non capita mai
// perché nessuno ripete una chiamata. In esercizio capitano tutti e tre.
package coda

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Q1 — claim esclusivo sotto concorrenza.
//
// La coda si regge su `FOR UPDATE SKIP LOCKED`: ogni job deve finire a un worker e a uno solo. Se
// due worker prendessero lo stesso job, ogni garanzia costruita sopra al tentativo sarebbe inutile,
// perché due tentativi validi contemporaneamente sono, per definizione, due scritture autorizzate
// sullo stesso lavoro.
func TestClaimEsclusivoSottoConcorrenza(t *testing.T) {
	p, q, ctx := preparaDB(t)

	const nJob, nWorker = 200, 8
	for i := 0; i < nJob; i++ {
		accoda(t, ctx, q, db.TipoJobStageAllegato, "", Opzioni{})
	}

	type presa struct {
		job    int64
		worker string
		token  uuid.UUID
	}
	prese := make(chan presa, nJob*2)
	var via sync.WaitGroup
	var fine sync.WaitGroup
	via.Add(1)
	fine.Add(nWorker)
	for w := 0; w < nWorker; w++ {
		go func(w int) {
			defer fine.Done()
			nome := "outlook@PC-" + string(rune('A'+w))
			qw := db.New(p) // ogni worker con la propria connessione dal pool
			via.Wait()
			for {
				j, err := Claim(ctx, qw, db.WorkerTipoOutlook, nome, Destinazione{}, 0)
				if err != nil {
					t.Errorf("claim di %s: %v", nome, err)
					return
				}
				if j == nil {
					return // coda vuota
				}
				prese <- presa{job: j.JobID, worker: nome, token: j.LeaseToken.UUID}
			}
		}(w)
	}
	via.Done()
	fine.Wait()
	close(prese)

	visti := map[int64]presa{}
	token := map[uuid.UUID]bool{}
	doppi := 0
	for pr := range prese {
		if gia, gia_preso := visti[pr.job]; gia_preso {
			doppi++
			t.Errorf("job %d preso due volte: da %s e da %s", pr.job, gia.worker, pr.worker)
			continue
		}
		visti[pr.job] = pr
		if pr.token == uuid.Nil {
			t.Errorf("job %d assegnato senza lease_token: il tentativo non ha identità", pr.job)
		}
		if token[pr.token] {
			t.Errorf("lease_token ripetuto su job diversi: %v", pr.token)
		}
		token[pr.token] = true
	}
	if doppi == 0 && len(visti) != nJob {
		t.Errorf("job assegnati = %d, attesi %d: qualcuno è rimasto in coda", len(visti), nJob)
	}

	// controprova dal database: nessun job è rimasto pronto, ognuno ha un token e un worker
	var pronti, senzaToken int
	_ = p.QueryRow(ctx, `SELECT count(*) FROM job WHERE stato = 'pronto'`).Scan(&pronti)
	_ = p.QueryRow(ctx, `SELECT count(*) FROM job WHERE stato = 'in_corso' AND (lease_token IS NULL OR worker_id IS NULL OR avviato_il IS NULL)`).Scan(&senzaToken)
	if pronti != 0 || senzaToken != 0 {
		t.Errorf("dopo il claim: pronti=%d, in corso senza tentativo completo=%d", pronti, senzaToken)
	}
}

// Q6 — il battito arriva da un thread separato mentre il lavoro è in corso, e il lease non scade.
//
// È la metà lato server del recupero di §2.3: il worker batte da un thread suo perché la chiamata COM
// non è interrompibile. Qui si verifica ciò che il server deve garantirgli: finché il battito arriva,
// il tentativo resta valido anche con un lease molto più corto del lavoro; e nessun altro worker può
// prendere il job nel frattempo.
func TestIlBattitoTieneVivoIlTentativoDurantePiuLease(t *testing.T) {
	p, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobSyncOutlook, "", Opzioni{LeaseS: 2, DurataMaxS: 300})
	preso := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC-A")
	if preso == nil {
		t.Fatal("nessun job preso")
	}
	tent := Tentativo{JobID: j.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "outlook@PC-A"}

	fine := make(chan struct{})
	battiti := make(chan error, 32)
	go func() { // il thread del battito: non tocca il lavoro, tiene solo vivo il lease
		qb := db.New(p)
		for {
			select {
			case <-fine:
				close(battiti)
				return
			case <-time.After(400 * time.Millisecond):
				battiti <- Batte(ctx, qb, tent)
			}
		}
	}()

	// il «lavoro» dura 5 secondi, cioè più di due lease interi
	time.Sleep(5 * time.Second)
	close(fine)
	n := 0
	for err := range battiti {
		n++
		if err != nil {
			t.Fatalf("battito %d rifiutato mentre il lavoro era in corso: %v", n, err)
		}
	}
	if n < 8 {
		t.Fatalf("solo %d battiti in 5 secondi: il test non ha provato niente", n)
	}

	// nessun altro ha potuto prendere il job, e il risultato del tentativo è ancora applicabile
	if altro := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC-B"); altro != nil {
		t.Errorf("un secondo worker ha preso il job %d mentre il primo batteva", altro.JobID)
	}
	if _, err := Completa(ctx, q, tent, json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatalf("il tentativo tenuto vivo dai battiti non riesce a chiudere il job: %v", err)
	}
}

// Q11 — due risultati per lo stesso tentativo: il secondo riceve 409.
//
// Il caso vero è la bozza Outlook: il worker la crea, manda il risultato, la risposta si perde e il
// worker ripete. Senza questo controllo il secondo risultato riaprirebbe un job già chiuso — e
// l'operatore si troverebbe due bozze.
func TestDoppioRisultatoIlSecondoNonPassa(t *testing.T) {
	_, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobCreaBozzaOutlook, "", Opzioni{})
	preso := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC-A")
	if preso == nil {
		t.Fatal("nessun job preso")
	}
	tent := Tentativo{JobID: j.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "outlook@PC-A"}

	if _, err := Completa(ctx, q, tent, json.RawMessage(`{"entry_id":"BOZZA-1"}`)); err != nil {
		t.Fatalf("primo risultato: %v", err)
	}
	if _, err := Completa(ctx, q, tent, json.RawMessage(`{"entry_id":"BOZZA-2"}`)); err != ErrTentativoNonValido {
		t.Fatalf("secondo risultato: err = %v, atteso ErrTentativoNonValido (409)", err)
	}
	dopo := statoDi(t, ctx, q, j.JobID)
	if dopo.Stato != db.StatoJobFatto {
		t.Errorf("stato dopo il doppio risultato = %s, atteso fatto", dopo.Stato)
	}
	// il risultato registrato è il primo: il secondo non deve averlo sovrascritto
	if v := campoRisultato(t, dopo, "entry_id"); v != "BOZZA-1" {
		t.Errorf("entry_id registrato = %q, atteso quello del primo invio", v)
	}

	// anche un fallimento tardivo dello stesso tentativo non deve riaprire il job
	if _, err := Fallisci(ctx, q, tent, "ripensamento tardivo", false); err != ErrTentativoNonValido {
		t.Errorf("fallimento dopo la chiusura: err = %v, atteso ErrTentativoNonValido", err)
	}
	if fin := statoDi(t, ctx, q, j.JobID); fin.Stato != db.StatoJobFatto {
		t.Errorf("il job è stato riaperto da un risultato tardivo: %s", fin.Stato)
	}
}

// Contorno di Q11: due tentativi DIVERSI sullo stesso job. Solo quello in corso può chiudere.
func TestSoloIlTentativoInCorsoPuoChiudere(t *testing.T) {
	p, q, ctx := preparaDB(t)
	j := accoda(t, ctx, q, db.TipoJobStageAllegato, "", Opzioni{LeaseS: 60})
	primo := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC-A")
	if primo == nil {
		t.Fatal("nessun job preso")
	}
	tentA := Tentativo{JobID: j.JobID, LeaseToken: primo.LeaseToken.UUID, WorkerID: "outlook@PC-A"}

	// il lease di A scade e lo scheduler rimette il job in coda
	if _, err := p.Exec(ctx, `UPDATE job SET lease_fino_a = now() - interval '1 second' WHERE job_id = $1`, j.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RilasciaLeaseScaduti(ctx); err != nil {
		t.Fatal(err)
	}
	secondo := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC-B")
	if secondo == nil {
		t.Fatal("il job non è tornato in coda dopo la scadenza del lease")
	}
	tentB := Tentativo{JobID: j.JobID, LeaseToken: secondo.LeaseToken.UUID, WorkerID: "outlook@PC-B"}
	if tentA.LeaseToken == tentB.LeaseToken {
		t.Fatal("i due tentativi hanno lo stesso token: il claim non ne genera uno nuovo")
	}

	if _, err := Completa(ctx, q, tentA, json.RawMessage(`{"da":"A"}`)); err != ErrTentativoNonValido {
		t.Errorf("A ha potuto chiudere il job di B: err = %v", err)
	}
	if _, err := Completa(ctx, q, tentB, json.RawMessage(`{"da":"B"}`)); err != nil {
		t.Errorf("B non riesce a chiudere il proprio job: %v", err)
	}
	dopo := statoDi(t, ctx, q, j.JobID)
	if v := campoRisultato(t, dopo, "da"); v != "B" {
		t.Errorf("risultato registrato da %q, atteso B", v)
	}
}

// campoRisultato legge un campo dal risultato del job. Si passa dal JSON e non dal confronto di
// stringhe perché jsonb normalizza la formattazione: confrontare il testo farebbe fallire il test
// per uno spazio, cioè per un motivo che non ha niente a che vedere con ciò che si sta provando.
func campoRisultato(t *testing.T, j db.Job, campo string) string {
	t.Helper()
	if j.Risultato == nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(*j.Risultato, &m); err != nil {
		t.Fatalf("risultato del job %d illeggibile: %v", j.JobID, err)
	}
	v, _ := m[campo].(string)
	return v
}
