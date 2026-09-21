//go:build integrazione

// L4 — blocco 3 del 3R: le due frontiere, e che cosa le muove.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/workerapi/
//
// Il modello che questi test tengono fermo:
//
//	PASSATO                                                          PRESENTE
//	     storico_fino_a  [ quello che il Cockpit ha ]  coperto_fino_a
//	            ← «Carica precedenti»            «Aggiorna» →
//
// e la regola che lo rende sicuro: una frontiera si sposta SOLO quando il worker dichiara di aver
// percorso per intero la finestra [dal, al] che il server gli aveva dato. Mai per lotto, mai perché
// il job è finito senza errori.
//
// Il caso che ha fatto nascere il blocco è la notte: il Cockpit è allineato alle 17:00, i PC si
// spengono, alle 09:00 i worker ripartono. Si deve leggere solo il buco — più una sovrapposizione di
// sicurezza — e si deve leggere dalle 09:00 all'indietro, perché è l'ordine in cui la posta serve.
// Ma leggere dal più recente vuol dire che il primo lotto contiene già la mail più nuova: se la
// frontiera avanzasse lì, un worker che muore subito dopo lascerebbe invisibile per sempre tutto
// quello che sta sotto. Per questo qui si prova soprattutto ciò che NON deve succedere.
package workerapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

type bancoCopertura struct {
	t    *testing.T
	pool *pgxpool.Pool
	q    *db.Queries
	ctx  context.Context
	s    *Server
}

func preparaCopertura(t *testing.T) *bancoCopertura {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	log := testutil.LogSilenzioso()
	return &bancoCopertura{t: t, pool: pool, q: db.New(pool), ctx: context.Background(),
		s: &Server{Pool: pool, Log: log, Ingest: &ingest.Servizio{Pool: pool, Log: log}}}
}

func (b *bancoCopertura) casella(indirizzo string) db.Casella {
	b.t.Helper()
	c, err := b.q.UpsertCasella(b.ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: indirizzo, Nome: indirizzo, Condivisa: true,
	})
	if err != nil {
		b.t.Fatalf("casella %s: %v", indirizzo, err)
	}
	return c
}

var cartelleDiProva = []string{"Inbox", "Sent Items"}

// accoda chiede al server la finestra successiva di quella casella, come fanno «Aggiorna ora» e lo
// scheduler. Il job precedente va chiuso prima: la chiave di idempotenza è fissa per casella.
func (b *bancoCopertura) accoda(c db.Casella) (*db.Job, api.PayloadSyncOutlook) {
	b.t.Helper()
	j, err := jobs.AccodaSyncCasella(b.ctx, b.q, c, jobs.SyncOpzioni{Cartelle: cartelleDiProva, Lotto: 50})
	if err != nil {
		b.t.Fatalf("accoda: %v", err)
	}
	if j == nil {
		b.t.Fatal("nessun job accodato: ce n'era già uno pendente e non è stato chiuso")
	}
	var p api.PayloadSyncOutlook
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		b.t.Fatalf("payload: %v", err)
	}
	return j, p
}

func (b *bancoCopertura) chiudi(j *db.Job) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, "UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1", j.JobID); err != nil {
		b.t.Fatalf("chiusura del job %d: %v", j.JobID, err)
	}
}

// riporta applica il risultato del worker come fa la rotta /result: è il punto in cui il server
// decide se una frontiera si muove.
func (b *bancoCopertura) riporta(j *db.Job, cartelle ...api.CartellaEsito) {
	b.t.Helper()
	dati, err := json.Marshal(api.RisultatoSync{Cartelle: cartelle})
	if err != nil {
		b.t.Fatal(err)
	}
	if err := b.s.applicaRisultato(b.ctx, b.q, j, dati, nil); err != nil {
		b.t.Fatalf("applicaRisultato: %v", err)
	}
	b.chiudi(j)
}

// consegnaUnLotto fa quello che fa il worker mentre legge: un POST /ingest con gli elementi e il
// cursore del lotto. Passa dal servizio vero, cioe' dalla transazione dove il cursore avanza insieme
// agli elementi: e' li' che un avanzamento di troppo si pagherebbe.
func (b *bancoCopertura) consegnaUnLotto(c db.Casella, cartella string, quando time.Time) {
	b.t.Helper()
	m := api.MessaggioIn{
		MessageID: "<lotto-" + quando.Format("150405.000000") + "@prova>", EntryID: "E-" + quando.Format("150405.000000"),
		StoreID: "S", Cartella: cartella, Direzione: "entrata", DataEvento: quando, RicevutoIl: &quando,
		Oggetto: "il piu' recente della finestra", MittenteIndirizzo: "cliente@acme.example",
	}
	if _, err := b.s.Ingest.Ingerisci(b.ctx, ingest.Lotto{
		Casella: c, Messaggi: []api.MessaggioIn{m},
		Cursore: &api.CursoreLotto{Cartella: cartella, UltimoReceived: quando},
	}); err != nil {
		b.t.Fatalf("lotto: %v", err)
	}
}

func (b *bancoCopertura) cursore(c db.Casella, cartella string) db.SyncCursore {
	b.t.Helper()
	cur, err := b.q.GetSyncCursore(b.ctx, db.GetSyncCursoreParams{CasellaID: c.CasellaID, Cartella: cartella})
	if err != nil {
		b.t.Fatalf("cursore %s: %v", cartella, err)
	}
	return cur
}

// coperturaDi è la frontiera recente di una cartella, o nil se non ne ha ancora una.
func (b *bancoCopertura) coperturaDi(c db.Casella, cartella string) *time.Time {
	b.t.Helper()
	cur, err := b.q.GetSyncCursore(b.ctx, db.GetSyncCursoreParams{CasellaID: c.CasellaID, Cartella: cartella})
	if err != nil {
		return nil
	}
	return cur.CopertoFinoA
}

func finestraDi(t *testing.T, p api.PayloadSyncOutlook, cartella string) (time.Time, time.Time) {
	t.Helper()
	for _, c := range p.Cartelle {
		if c.Cartella != cartella {
			continue
		}
		if c.Dal == nil || c.Al == nil {
			t.Fatalf("%s: finestra aperta nel payload (dal=%v al=%v): il worker non la chiude da sé", cartella, c.Dal, c.Al)
		}
		return *c.Dal, *c.Al
	}
	t.Fatalf("%s non è nel payload", cartella)
	return time.Time{}, time.Time{}
}

// tutteComplete è il risultato di un sync andato bene: ogni cartella dichiara di aver percorso la
// sua finestra per intero, e `ultimo_received` è la mail più recente che ha consegnato.
func tutteComplete(p api.PayloadSyncOutlook, ultimo *time.Time, n int) []api.CartellaEsito {
	out := make([]api.CartellaEsito, 0, len(p.Cartelle))
	for _, c := range p.Cartelle {
		out = append(out, api.CartellaEsito{Cartella: c.Cartella, UltimoReceived: ultimo, NMessaggi: n, Completa: true})
	}
	return out
}

// uguali confronta due istanti al microsecondo, che e' la risoluzione di un timestamptz. Un
// time.Time appena creato in Go ha i nanosecondi; lo stesso valore riletto dal database non li ha
// piu'. Pretendere l'uguaglianza esatta sarebbe un test rosso per una cifra che il database non
// conserva — e allargare oltre il microsecondo nasconderebbe uno scostamento vero.
func uguali(t *testing.T, ottenuto, atteso time.Time, cosa string) {
	t.Helper()
	if !ottenuto.Truncate(time.Microsecond).Equal(atteso.Truncate(time.Microsecond)) {
		t.Errorf("%s: %v, atteso %v", cosa, ottenuto.Local(), atteso.Local())
	}
}

// ---------------------------------------------------------------- C: la notte

// La UX che il blocco esiste per ottenere: ieri alle 17:00 eravamo allineati, i PC si spengono, alle
// 09:00 si riparte. Si legge il buco e solo il buco — più la sovrapposizione — e lo si legge
// dall'alto.
func TestCopertura3CLaNotteSiLeggeSoloIlBuco(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("commerciale@azienda.example")

	ieriAlle17 := time.Now().Add(-16 * time.Hour).UTC().Truncate(time.Second)
	j, p := b.accoda(c)
	if p.Modo != api.ModoBootstrap {
		t.Fatalf("la prima volta è un bootstrap, non %q", p.Modo)
	}
	b.riporta(j, tutteComplete(p, &ieriAlle17, 40)...)
	// il primo sync ha coperto fino al suo `al`, non fino all'ultima mail che ha trovato
	uguali(t, *b.coperturaDi(c, "Inbox"), *p.Al, "copertura dopo il primo sync")

	// la notte passa: si finge riportando indietro la copertura alle 17:00 di ieri
	if _, err := b.pool.Exec(b.ctx, "UPDATE sync_cursore SET coperto_fino_a = $1", ieriAlle17); err != nil {
		t.Fatal(err)
	}

	_, mattina := b.accoda(c)
	if mattina.Modo != api.ModoAggiornamento {
		t.Errorf("modo = %q: una casella già coperta non è in bootstrap", mattina.Modo)
	}
	dal, al := finestraDi(t, mattina, "Inbox")
	uguali(t, dal, ieriAlle17.Add(-jobs.SovrapposizioneSync), "la finestra della mattina parte dalla copertura meno la sovrapposizione")
	if d := time.Since(al); d > time.Minute || d < -time.Minute {
		t.Errorf("il limite superiore non è l'istante dell'accodamento: %v", al)
	}
	// e non è la finestra iniziale: quella rileggerebbe una settimana ogni mattina
	if dal.Before(time.Now().AddDate(0, 0, -2)) {
		t.Errorf("la mattina si rilegge da %v: è tornata alla finestra iniziale", dal)
	}
}

// ---------------------------------------------------------------- E: il crash a metà

// IL TEST DEL BLOCCO. Il worker muore a metà della finestra notturna, dopo aver consegnato la parte
// più recente. Al riavvio, la parte che non ha letto deve essere ancora acquisibile.
//
// È il caso critico del mandato, con le 17/16/15/14 rese in ore: il worker prende le due più
// recenti e viene terminato; le due più vecchie DEVONO restare acquisibili. Sono ammesse riletture e
// deduplicazioni; non è ammesso nessun buco.
func TestCopertura3ECrashAMetaAggiornamentoNonLasciaBuchi(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("commerciale@azienda.example")

	ieriAlle17 := time.Now().Add(-16 * time.Hour).UTC().Truncate(time.Second)
	j0, p0 := b.accoda(c)
	b.riporta(j0, tutteComplete(p0, &ieriAlle17, 10)...)
	if _, err := b.pool.Exec(b.ctx, "UPDATE sync_cursore SET coperto_fino_a = $1", ieriAlle17); err != nil {
		t.Fatal(err)
	}

	// il job della mattina: [ieri 17:00 - 10 min, adesso]
	j, p := b.accoda(c)
	dal, _ := finestraDi(t, p, "Inbox")

	// il worker legge dall'alto, consegna il lotto con la mail più recente della notte… e muore.
	// Il risultato arriva lo stesso (il lease scade e il job viene ripreso, oppure il worker riporta
	// quello che ha fatto): la cartella NON è completa.
	appenaAdesso := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	b.consegnaUnLotto(c, "Inbox", appenaAdesso)
	b.riporta(j,
		api.CartellaEsito{Cartella: "Inbox", UltimoReceived: &appenaAdesso, NMessaggi: 1, Completa: false},
		api.CartellaEsito{Cartella: "Sent Items", UltimoReceived: nil, NMessaggi: 0, Completa: false},
	)

	// la mail più recente è stata consegnata, e `ultimo_received` lo dice: è il suo mestiere
	cur := b.cursore(c, "Inbox")
	if cur.UltimoReceived == nil || !cur.UltimoReceived.Equal(appenaAdesso) {
		t.Errorf("ultimo_received = %v: i messaggi consegnati ci sono, la colonna deve dirlo", cur.UltimoReceived)
	}
	// ma la COPERTURA non si è mossa di un secondo
	if cur.CopertoFinoA == nil || !cur.CopertoFinoA.Equal(ieriAlle17) {
		t.Fatalf("la copertura è avanzata a %v su una finestra mai conclusa: tutto ciò che sta sotto "+
			"è appena diventato invisibile", cur.CopertoFinoA)
	}

	// il riavvio: la finestra ricomincia da dove ricominciava prima. Si rilegge — e va bene.
	_, dopo := b.accoda(c)
	dal2, _ := finestraDi(t, dopo, "Inbox")
	uguali(t, dal2, dal, "dopo il crash la finestra riparte dallo stesso punto")
	if dal2.After(ieriAlle17) {
		t.Errorf("la finestra riparte da %v, cioè DOPO la copertura di ieri (%v): in mezzo c'è un buco", dal2, ieriAlle17)
	}
}

// F — lo stesso, ma la (casella, cartella) non era mai stata sincronizzata. Un bootstrap interrotto
// non deve saltare la parte non letta, e non deve nemmeno smettere di essere un bootstrap.
func TestCopertura3FCrashAMetaBootstrapNonLasciaBuchi(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("nuova@azienda.example")

	j, p := b.accoda(c)
	if p.Modo != api.ModoBootstrap {
		t.Fatalf("modo = %q su una casella mai sincronizzata", p.Modo)
	}
	dal, _ := finestraDi(t, p, "Inbox")

	appenaAdesso := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	b.consegnaUnLotto(c, "Inbox", appenaAdesso)
	b.riporta(j,
		api.CartellaEsito{Cartella: "Inbox", UltimoReceived: &appenaAdesso, NMessaggi: 1, Completa: false},
		api.CartellaEsito{Cartella: "Sent Items", NMessaggi: 0, Completa: false},
	)
	if cop := b.coperturaDi(c, "Inbox"); cop != nil {
		t.Fatalf("un bootstrap interrotto ha scritto una copertura (%v): i sette giorni sotto non "+
			"verrebbero letti mai", cop)
	}

	_, dopo := b.accoda(c)
	if dopo.Modo != api.ModoBootstrap {
		t.Errorf("modo = %q: il bootstrap non era finito", dopo.Modo)
	}
	dal2, _ := finestraDi(t, dopo, "Inbox")
	if dal2.After(dal.Add(time.Minute)) {
		t.Errorf("il secondo tentativo parte da %v invece che da %v: la parte non letta è stata saltata", dal2, dal)
	}
}

// ---------------------------------------------------------------- H: il caso del revisore

// Enumerazione interrotta con `al` finito: il job HTTP finisce senza panic, la cartella non riporta
// nemmeno un errore — e la finestra NON deve risultare conclusa.
//
// Prima la condizione era `c.Errore == ""`, cioè l'ASSENZA di un guasto. Ma un'enumerazione COM che
// si ferma a metà può chiudersi senza eccezioni e senza riempire quel campo: il server vedeva una
// cartella silenziosa e la trattava come una cartella completa.
func TestCopertura3HUnaFinestraIncompletaSenzaErroreNonMuoveNiente(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("commerciale@azienda.example")

	j, _ := b.accoda(c)
	b.riporta(j,
		// nessun errore, nessun messaggio, `completa` assente: è il silenzio
		api.CartellaEsito{Cartella: "Inbox", NMessaggi: 0, Errore: ""},
		api.CartellaEsito{Cartella: "Sent Items", NMessaggi: 0, Errore: ""},
	)
	for _, cartella := range cartelleDiProva {
		if cop := b.coperturaDi(c, cartella); cop != nil {
			t.Errorf("%s: copertura scritta a %v da una cartella che non ha dichiarato niente. "+
				"«Nessun errore» non vuol dire «ho guardato tutto»", cartella, cop)
		}
	}
	_, dopo := b.accoda(c)
	if dopo.Modo != api.ModoBootstrap {
		t.Errorf("modo = %q: nessuna finestra è stata conclusa, la casella è ancora al punto di partenza", dopo.Modo)
	}
}

// ---------------------------------------------------------------- I: la coda muta della finestra

// Il motivo per cui `coperto_fino_a` esiste come colonna separata. Fra le 17:00 e le 09:00 non è
// arrivato niente: `ultimo_received` non si muove, perché non c'è nessuna mail più recente. Ma la
// notte È stata guardata, e se il sistema non ha modo di ricordarlo la rispedisce alla lettura ogni
// volta, per sempre.
func TestCopertura3ILaParteVuotaDellaFinestraResta(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("silenziosa@azienda.example")

	j, p := b.accoda(c)
	// finestra percorsa tutta, zero messaggi, nessuna mail più recente da dichiarare
	b.riporta(j, tutteComplete(p, nil, 0)...)

	cur := b.cursore(c, "Inbox")
	if cur.UltimoReceived != nil {
		t.Errorf("ultimo_received = %v senza nemmeno una mail: non è quello il suo mestiere", cur.UltimoReceived)
	}
	if cur.CopertoFinoA == nil {
		t.Fatal("una finestra percorsa per intero senza trovare posta non ha lasciato traccia: verrà riletta per sempre")
	}
	uguali(t, *cur.CopertoFinoA, *p.Al, "la copertura è il limite superiore della finestra, non l'ultima mail")

	_, dopo := b.accoda(c)
	dal, _ := finestraDi(t, dopo, "Inbox")
	uguali(t, dal, p.Al.Add(-jobs.SovrapposizioneSync), "il sync successivo riparte da dove si era guardato")
	if dopo.Modo != api.ModoAggiornamento {
		t.Errorf("modo = %q: la finestra era stata conclusa", dopo.Modo)
	}
}

// ---------------------------------------------------------------- J: frontiere indipendenti

// Due caselle e due cartelle: quattro frontiere, e nessuna sa niente delle altre. Una cartella che
// va bene non deve far avanzare quella che si è rotta, e una casella non deve muovere l'altra —
// sarebbe il difetto che la 0004 ha chiuso per il cursore, rifatto sulla copertura.
func TestCopertura3JQuattroFrontiereIndipendenti(t *testing.T) {
	b := preparaCopertura(t)
	uno := b.casella("commerciale@azienda.example")
	due := b.casella("francesco@azienda.example")

	jDue, pDue := b.accoda(due)
	b.riporta(jDue, tutteComplete(pDue, nil, 0)...)

	jUno, _ := b.accoda(uno)
	b.riporta(jUno,
		api.CartellaEsito{Cartella: "Inbox", NMessaggi: 7, Completa: true},
		api.CartellaEsito{Cartella: "Sent Items", NMessaggi: 0, Errore: "LetturaIncompleta: enumerazione interrotta", Completa: false},
	)

	if b.coperturaDi(uno, "Inbox") == nil {
		t.Error("la cartella che ha concluso la sua finestra non è avanzata")
	}
	if cop := b.coperturaDi(uno, "Sent Items"); cop != nil {
		t.Errorf("la cartella rotta è avanzata a %v insieme alla vicina", cop)
	}
	// l'altra casella è ferma dove l'aveva lasciata il suo sync, non dove è arrivata questa
	for _, cartella := range cartelleDiProva {
		cop := b.coperturaDi(due, cartella)
		if cop == nil {
			t.Fatalf("%s della seconda casella: copertura sparita", cartella)
		}
		uguali(t, *cop, *pDue.Al, cartella+" della seconda casella, spostata dal sync di un'altra")
	}
}

// ---------------------------------------------------------------- K: la sovrapposizione

// La sovrapposizione è la misura di quanto due orologi e l'indice di Outlook possono non essere
// d'accordo su «quando è arrivata questa mail». Senza, una mail che si materializza appena sotto la
// frontiera resta sotto per sempre; con, si rilegge un tratto e la deduplica per Message-ID lo
// assorbe. Riletture ammesse, perdite no.
func TestCopertura3KLaSovrapposizioneRileggeUnTrattoOgniVolta(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("commerciale@azienda.example")

	j, p := b.accoda(c)
	b.riporta(j, tutteComplete(p, nil, 0)...)

	_, dopo := b.accoda(c)
	dal, _ := finestraDi(t, dopo, "Inbox")
	coperto := *b.coperturaDi(c, "Inbox")
	if !dal.Before(coperto) {
		t.Fatalf("la finestra successiva comincia a %v, cioè esattamente dove finiva la precedente (%v): "+
			"una mail consegnata sul confine non verrebbe letta da nessuna delle due", dal, coperto)
	}
	uguali(t, dal, coperto.Add(-jobs.SovrapposizioneSync), "sovrapposizione applicata")
	if jobs.SovrapposizioneSync < time.Minute {
		t.Errorf("sovrapposizione di %v: troppo stretta per due orologi diversi", jobs.SovrapposizioneSync)
	}
}

// ---------------------------------------------------------------- le due frontiere non si toccano

// «Aggiorna» non modifica il limite storico, «Carica precedenti» non modifica quello recente. Sono
// due direzioni, e l'unico momento in cui nascono insieme è il primo sync della cartella.
func TestCopertura3LeDueFrontiereNonSiToccano(t *testing.T) {
	b := preparaCopertura(t)
	c := b.casella("commerciale@azienda.example")

	// un «Carica precedenti» già riuscito: la frontiera passata è a due giorni fa
	dueGiorniFa := time.Now().AddDate(0, 0, -2).UTC().Truncate(time.Second)
	for _, cartella := range cartelleDiProva {
		if err := b.q.SetStoricoFinoA(b.ctx, db.SetStoricoFinoAParams{
			CasellaID: c.CasellaID, Cartella: cartella, StoricoFinoA: &dueGiorniFa,
		}); err != nil {
			t.Fatal(err)
		}
	}

	j, p := b.accoda(c)
	if p.Modo == api.ModoStorico {
		t.Fatal("«Aggiorna» ha accodato un job storico")
	}
	b.riporta(j, tutteComplete(p, nil, 3)...)

	cur := b.cursore(c, "Inbox")
	if cur.StoricoFinoA == nil || !cur.StoricoFinoA.Equal(dueGiorniFa) {
		t.Errorf("l'aggiornamento ha spostato il limite storico a %v (era %v)", cur.StoricoFinoA, dueGiorniFa)
	}
	if cur.CopertoFinoA == nil {
		t.Error("l'aggiornamento non ha spostato la copertura")
	}
}
