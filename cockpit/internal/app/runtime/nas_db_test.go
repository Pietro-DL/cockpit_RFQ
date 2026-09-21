//go:build integrazione

// L4 — voce 1.7: le copie sul NAS non si perdono perché il NAS non c'era (difetto N8, test N3).
//
// Il difetto non si vedeva mai al momento in cui succedeva. Cinque tentativi con il backoff limitato a
// dieci minuti coprono poco più di mezz'ora: un NAS fermo una notte faceva chiudere la copia come
// 'fallito', il documento restava 'in_coda' e il file non arrivava nel fascicolo. Nessun errore a
// schermo, nessuno che se ne accorgesse finché qualcuno non andava a cercare quel file — settimane
// dopo, quando il registro dei job era già stato ripulito dalla retention.
//
// La correzione è di tre pezzi, e qui si prova che ci siano tutti e tre:
//   - il budget dei tentativi è 50 per chi scrive sul NAS, 5 per tutti gli altri;
//   - un tentativo trovato senza NAS non viene consumato (altrimenti i 50 finiscono lo stesso);
//   - al ritorno del NAS le copie esaurite tornano in coda da sole, senza che nessuno prema niente.
package runtime

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// esecutore costruisce l'esecutore server con una radice NAS che possiamo far sparire e riapparire.
func esecutore(t *testing.T, radice string) *EsecutoreServer {
	t.Helper()
	return &EsecutoreServer{
		NAS: &nas.Scrittore{Radice: radice},
		Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
}

// scriviJobChiuso scrive direttamente un job già 'fallito': riproduce lo stato in cui il difetto N8
// lasciava le copie, senza dover aspettare cinquanta fallimenti veri. tentativi < max = fallimento
// dichiarato definitivo (per esempio un conflitto di hash sulla destinazione); tentativi >= max =
// tentativi esauriti, cioè il caso del NAS che non c'era.
func scriviJobChiuso(t *testing.T, ctx context.Context, p *pgxpool.Pool, tipo db.TipoJob, chiave string, tentativi, max int) int64 {
	t.Helper()
	var id int64
	err := p.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, stato, tentativi,
		max_tentativi, lease_s, durata_max_s, chiuso_il, errore)
		VALUES ($1, 'server', '{}'::jsonb, $2, 'fallito', $3, $4, 300, 3600, now(), 'prova')
		RETURNING job_id`, tipo, chiave, tentativi, max).Scan(&id)
	if err != nil {
		t.Fatalf("job chiuso %s: %v", chiave, err)
	}
	return id
}

// subitoDisponibile toglie l'attesa a un job rinviato, per non far dormire il test.
func subitoDisponibile(t *testing.T, ctx context.Context, p *pgxpool.Pool, id int64) {
	t.Helper()
	if _, err := p.Exec(ctx, `UPDATE job SET non_prima_di = now() WHERE job_id = $1`, id); err != nil {
		t.Fatal(err)
	}
}

func TestChiScriveSulNasHaCinquantaTentativi(t *testing.T) {
	_, q, ctx := preparaDB(t)

	casi := []struct {
		tipo   db.TipoJob
		attesi int32
	}{
		{db.TipoJobCopiaNas, 50},
		{db.TipoJobCreaCartellaThread, 50},
		{db.TipoJobSyncOutlook, 5},
		{db.TipoJobStageAllegato, 5},
	}
	for _, c := range casi {
		j := accoda(t, ctx, q, c.tipo, string(c.tipo)+":"+uuid.NewString(), coda.Opzioni{})
		if j.MaxTentativi != c.attesi {
			t.Errorf("%s: max_tentativi = %d, attesi %d", c.tipo, j.MaxTentativi, c.attesi)
		}
	}

	// coda.Opzioni esplicite devono poter scavalcare il default per tipo (lo usa chi accoda a mano).
	j := accoda(t, ctx, q, db.TipoJobCopiaNas, "copia:"+uuid.NewString(), coda.Opzioni{MaxTentativi: 3})
	if j.MaxTentativi != 3 {
		t.Errorf("max_tentativi esplicito ignorato: %d", j.MaxTentativi)
	}
}

// Il tentativo che non è nemmeno cominciato non si conta. Senza questo, i 50 tentativi servirebbero a
// poco: un NAS fermo per la notte li brucerebbe comunque tutti, uno ogni dieci minuti di backoff.
func TestNasAssenteRimetteInCodaSenzaConsumareIlTentativo(t *testing.T) {
	p, q, ctx := preparaDB(t)
	e := esecutore(t, filepath.Join(t.TempDir(), "nas-che-non-c-e"))
	e.RitardoNasAssente = 30 * time.Second

	j := accoda(t, ctx, q, db.TipoJobCopiaNas, "copia:"+uuid.NewString(), coda.Opzioni{})
	preso := claim(t, ctx, q, db.WorkerTipoServer, "server")
	if preso == nil {
		t.Fatal("il job non è stato assegnato")
	}
	if preso.Tentativi != 1 {
		t.Fatalf("tentativi dopo il claim = %d, atteso 1", preso.Tentativi)
	}
	tent := coda.Tentativo{JobID: preso.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "server"}

	if !e.rinviaSeNasAssente(ctx, q, tent, preso) {
		t.Fatal("con il NAS irraggiungibile il job doveva essere rinviato, non eseguito")
	}
	dopo := statoDi(t, ctx, q, j.JobID)
	if dopo.Stato != db.StatoJobPronto {
		t.Errorf("stato = %s, atteso pronto", dopo.Stato)
	}
	if dopo.Tentativi != 0 {
		t.Errorf("tentativi = %d: il tentativo doveva essere restituito, non consumato", dopo.Tentativi)
	}
	if dopo.LeaseToken.Valid || dopo.WorkerID.Valid {
		t.Errorf("il lease non è stato rilasciato: token=%v worker=%v", dopo.LeaseToken.Valid, dopo.WorkerID.Valid)
	}
	if !dopo.NonPrimaDi.After(time.Now().Add(20 * time.Second)) {
		t.Errorf("non_prima_di = %v: il job deve aspettare prima di riprovare", dopo.NonPrimaDi)
	}
	if dopo.ChiusoIl != nil {
		t.Error("un rinvio non chiude il job")
	}

	// finché l'attesa non è passata il job non viene riassegnato: è la differenza fra riprovare e
	// martellare il NAS assente dieci volte al secondo.
	if preso2 := claim(t, ctx, q, db.WorkerTipoServer, "server"); preso2 != nil {
		t.Fatal("il job rinviato non doveva essere riassegnato prima della sua attesa")
	}

	// e con il NAS al suo posto il rinvio non deve scattare, altrimenti il job non partirebbe mai.
	subitoDisponibile(t, ctx, p, j.JobID)
	radice := t.TempDir()
	e2 := esecutore(t, radice)
	preso3 := claim(t, ctx, q, db.WorkerTipoServer, "server")
	if preso3 == nil {
		t.Fatal("il job doveva tornare disponibile")
	}
	t3 := coda.Tentativo{JobID: preso3.JobID, LeaseToken: preso3.LeaseToken.UUID, WorkerID: "server"}
	if e2.rinviaSeNasAssente(ctx, q, t3, preso3) {
		t.Errorf("con il NAS raggiungibile (%s) il job non va rinviato", radice)
	}
}

// N3: il NAS torna e le copie riprendono da sole. «Senza intervento» è la parte che conta: se per
// recuperarle servisse che qualcuno se ne accorga e prema «riprova», il difetto N8 sarebbe ancora lì,
// solo spostato dall'automatismo alla memoria di chi lavora.
func TestAlRitornoDelNasLeCopieEsauriteTornanoInCoda(t *testing.T) {
	p, q, ctx := preparaDB(t)

	// tre job chiusi come 'fallito', per tre motivi diversi
	esaurito := scriviJobChiuso(t, ctx, p, db.TipoJobCopiaNas, "copia:esaurito", 50, 50)
	definitivo := scriviJobChiuso(t, ctx, p, db.TipoJobCopiaNas, "copia:conflitto", 2, 50)
	cartella := scriviJobChiuso(t, ctx, p, db.TipoJobCreaCartellaThread, "cartella:esaurita", 50, 50)
	altro := scriviJobChiuso(t, ctx, p, db.TipoJobSyncOutlook, "sync:esaurito", 5, 5)

	radice := t.TempDir()
	e := esecutore(t, radice)
	n := e.RiaccodaAlRitornoDelNas(ctx, q)
	if n != 2 {
		t.Fatalf("riaccodati %d job, attesi 2 (la copia esaurita e la cartella esaurita)", n)
	}

	if j := statoDi(t, ctx, q, esaurito); j.Stato != db.StatoJobPronto || j.Tentativi != 0 || j.ChiusoIl != nil {
		t.Errorf("copia esaurita: stato=%s tentativi=%d chiuso=%v", j.Stato, j.Tentativi, j.ChiusoIl)
	}
	if j := statoDi(t, ctx, q, cartella); j.Stato != db.StatoJobPronto {
		t.Errorf("cartella esaurita: stato=%s", j.Stato)
	}
	// Un conflitto di hash sulla destinazione è chiuso prima di esaurire i tentativi: il NAS che torna
	// non lo risolve, e rimetterlo in coda significherebbe farlo fallire di nuovo all'infinito.
	if j := statoDi(t, ctx, q, definitivo); j.Stato != db.StatoJobFallito {
		t.Errorf("fallimento definitivo: stato=%s, doveva restare fallito", j.Stato)
	}
	if j := statoDi(t, ctx, q, altro); j.Stato != db.StatoJobFallito {
		t.Errorf("job che non scrive sul NAS: stato=%s, doveva restare fallito", j.Stato)
	}

	// idempotente: chiamarla di nuovo non trova più niente da riaccodare
	if n := e.RiaccodaAlRitornoDelNas(ctx, q); n != 0 {
		t.Errorf("secondo giro: riaccodati %d, attesi 0", n)
	}
}

// Se nel frattempo esiste già un job pendente con la stessa chiave, riaccodare violerebbe l'indice
// unico parziale: il job esaurito resta dov'è, e il lavoro lo fa quello pendente.
func TestIlRiaccodoNonCreaDueJobConLaStessaChiave(t *testing.T) {
	p, q, ctx := preparaDB(t)
	esaurito := scriviJobChiuso(t, ctx, p, db.TipoJobCopiaNas, "copia:doppia", 50, 50)
	accoda(t, ctx, q, db.TipoJobCopiaNas, "copia:doppia", coda.Opzioni{})

	e := esecutore(t, t.TempDir())
	if n := e.RiaccodaAlRitornoDelNas(ctx, q); n != 0 {
		t.Fatalf("riaccodati %d: con una chiave già pendente non si riaccoda niente", n)
	}
	if j := statoDi(t, ctx, q, esaurito); j.Stato != db.StatoJobFallito {
		t.Errorf("stato = %s, atteso fallito", j.Stato)
	}
}
