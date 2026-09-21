//go:build integrazione

// L4 — SI1–SI4: da dove parte un sync, e chi lo decide (checkpoint del 16/09/2026).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/jobs/
//
// La precedenza è una sola, e sta in AccodaSyncCasella: il cursore della cartella vince sempre;
// senza cursore vale `dal` se è dichiarato nel file, altrimenti la finestra iniziale. Quello che
// questi test tengono fermo è soprattutto la parte che non si vede: che un riavvio non riporti una
// casella alla finestra iniziale. Il difetto, se ci fosse, non darebbe errore — rileggerebbe una
// settimana a ogni avvio, e su una casella viva sono ore di worker occupato per niente.
package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
)

// attorno confronta due istanti con una tolleranza di un minuto: `adesso - 7 giorni` calcolato nel
// test e calcolato nel codice non cadono nello stesso microsecondo, e pretenderlo sarebbe un test
// rosso a caso. Un minuto è largo per il calcolo e stretto per ciò che si vuole escludere (un mese,
// il cursore di un'altra cartella, un valore non impostato).
func attorno(t *testing.T, ottenuto, atteso time.Time, cosa string) {
	t.Helper()
	if d := ottenuto.Sub(atteso); d > time.Minute || d < -time.Minute {
		t.Errorf("%s: %v, atteso %v (differenza %v)", cosa, ottenuto.Local(), atteso.Local(), d)
	}
}

func payloadSync(t *testing.T, j *db.Job) api.PayloadSyncOutlook {
	t.Helper()
	if j == nil {
		t.Fatal("nessun job accodato: non c'è niente da esaminare")
	}
	var p api.PayloadSyncOutlook
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatalf("payload del job %d: %v", j.JobID, err)
	}
	return p
}

// finestreNelPayload è ciò che il worker riceve per cartella: la finestra che deve leggere, già
// fatta. Dal blocco 3 è questo il dato che conta — `UltimoReceived` viaggia ancora, ma per la
// diagnosi: chi decide da dove si riparte è `Dal`, e chi lo decide è il server.
func finestreNelPayload(p api.PayloadSyncOutlook) map[string]api.CartellaCursore {
	per := map[string]api.CartellaCursore{}
	for _, c := range p.Cartelle {
		per[c.Cartella] = c
	}
	return per
}

// dalDi è il limite inferiore di una cartella del payload, con un messaggio utile se non c'è.
func dalDi(t *testing.T, p api.PayloadSyncOutlook, cartella string) time.Time {
	t.Helper()
	c, ok := finestreNelPayload(p)[cartella]
	if !ok {
		t.Fatalf("%s non è nel payload: %v", cartella, p.Cartelle)
	}
	if c.Dal == nil {
		t.Fatalf("%s: nessun limite inferiore nel payload. Il worker non lo calcola più da sé: senza, leggerebbe la cartella intera", cartella)
	}
	return *c.Dal
}

// chiudi porta un job a `fatto` senza passare da un worker: qui interessa solo che la chiave di
// idempotenza si liberi, perché un secondo sync della stessa casella non si accoda finché il primo
// è pendente (ed è giusto così).
func chiudi(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	if _, err := pool.Exec(ctx, "UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1", jobID); err != nil {
		t.Fatalf("chiusura del job %d: %v", jobID, err)
	}
}

// scriviCopertura simula un sync riuscito su quella cartella: la mail più recente consegnata E la
// frontiera di copertura, che è quella su cui si decide la finestra successiva. Scriverne una sola
// sarebbe uno stato che il sistema non produce.
func scriviCopertura(t *testing.T, ctx context.Context, q *db.Queries, casella db.Casella, cartella string, quando time.Time) {
	t.Helper()
	if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{
		CasellaID: casella.CasellaID, Cartella: cartella, UltimoReceived: &quando,
	}); err != nil {
		t.Fatalf("cursore %s: %v", cartella, err)
	}
	if err := q.SetCopertoFinoA(ctx, db.SetCopertoFinoAParams{
		CasellaID: casella.CasellaID, Cartella: cartella, CopertoFinoA: &quando,
	}); err != nil {
		t.Fatalf("copertura %s: %v", cartella, err)
	}
}

// SI1 — una casella nuova legge la finestra iniziale, e nient'altro.
//
// Prima erano trenta giorni. Sembra generoso e non lo è: il primo sync è l'unico momento in cui il
// worker scarica davvero tutto (corpo e allegati, non la sola enumerazione), e su una casella viva
// un mese di archivio è il worker occupato per ore — durante le quali, essendo seriale, non apre
// elementi in Outlook e non scarica allegati per chi sta lavorando.
func TestSI1SenzaCursoreSiParteDallaFinestraIniziale(t *testing.T) {
	_, q, ctx := preparaDB(t)
	casella := casellaDiProva(t, ctx, q, "nuova@azienda.example")

	j, err := AccodaSyncCasella(ctx, q, casella, SyncOpzioni{Cartelle: []string{"Inbox", "Sent Items"}, Lotto: 50})
	if err != nil {
		t.Fatal(err)
	}
	p := payloadSync(t, j)
	attorno(t, p.Dal, time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), "finestra iniziale")
	// A: database nuovo = bootstrap. Non è un riavvio, non è un aggiornamento: di queste due cartelle
	// non si sa ancora niente, e il modo lo deve dire — è quello che un operatore legge nel log
	// quando si chiede perché il primo sync ci mette molto più degli altri.
	if p.Modo != api.ModoBootstrap {
		t.Errorf("modo = %q su una casella mai sincronizzata, atteso %q", p.Modo, api.ModoBootstrap)
	}
	// Il limite superiore è FISSATO all'accodamento, anche nell'aggiornamento ordinario: una finestra
	// che finisce a «adesso» si allunga mentre il job gira, e allora non esiste nessun istante di cui
	// si possa dire «scandito fino a qui». Senza quello, nessuna frontiera potrebbe avanzare.
	if p.Al == nil {
		t.Fatal("il payload non ha un limite superiore: la finestra non è chiusa e non si può dichiarare conclusa")
	}
	attorno(t, *p.Al, time.Now(), "limite superiore fissato all'accodamento")
	for cartella, c := range finestreNelPayload(p) {
		if !c.Bootstrap {
			t.Errorf("%s: non dichiarata in bootstrap su una casella mai sincronizzata", cartella)
		}
		if c.CopertoFinoA != nil {
			t.Errorf("%s: copertura %v su una casella mai sincronizzata", cartella, c.CopertoFinoA)
		}
		attorno(t, dalDi(t, p, cartella), time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), cartella+": finestra iniziale")
	}
}

// SI2 — la finestra iniziale è configurabile ([outlook].giorni_sync_iniziale).
func TestSI2LaFinestraInizialeSiConfigura(t *testing.T) {
	_, q, ctx := preparaDB(t)
	casella := casellaDiProva(t, ctx, q, "configurata@azienda.example")

	j, err := AccodaSyncCasella(ctx, q, casella, SyncOpzioni{Cartelle: []string{"Inbox"}, GiorniIniziali: 30, Lotto: 50})
	if err != nil {
		t.Fatal(err)
	}
	attorno(t, payloadSync(t, j).Dal, time.Now().AddDate(0, 0, -30), "finestra iniziale a 30 giorni")
}

// SI3 — il cursore vince sulla finestra iniziale, e il RIAVVIO non lo perde.
//
// È il test che conta di questo gruppo. Il cursore sta in database, non nel processo: un server
// riavviato deve continuare da dove la casella era arrivata. Se lo perdesse non se ne accorgerebbe
// nessuno — nessun errore, nessun buco, solo una settimana riletta a ogni avvio — e la deduplica per
// Message-ID nasconderebbe il sintomo lasciando il costo.
func TestSI3IlCursoreVinceEIlRiavvioNonRiportaAllaFinestraIniziale(t *testing.T) {
	pool, q, ctx := preparaDB(t)
	casella := casellaDiProva(t, ctx, q, "avviata@azienda.example")
	opzioni := SyncOpzioni{Cartelle: []string{"Inbox", "Sent Items"}, Lotto: 50}

	// il primo sync: nessun cursore, finestra iniziale
	primo, err := AccodaSyncCasella(ctx, q, casella, opzioni)
	if err != nil {
		t.Fatal(err)
	}
	attorno(t, payloadSync(t, primo).Dal, time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), "finestra del primo sync")

	// il worker lo esegue e lascia la copertura dell'Inbox; la Posta inviata non ha ancora niente
	arrivato := time.Now().Add(-90 * time.Minute).UTC().Truncate(time.Second)
	scriviCopertura(t, ctx, q, casella, "Inbox", arrivato)
	chiudi(t, ctx, pool, primo.JobID)

	// il riavvio: un processo nuovo, quindi Queries nuove, e le stesse opzioni del file
	dopoIlRiavvio := db.New(pool)
	secondo, err := AccodaSyncCasella(ctx, dopoIlRiavvio, casella, opzioni)
	if err != nil {
		t.Fatal(err)
	}
	p := payloadSync(t, secondo)
	// B: esiste una copertura, quindi è un AGGIORNAMENTO e non un bootstrap. La distinzione non è
	// cosmetica: bootstrap vuol dire «di questa cartella non sappiamo niente», e se il riavvio la
	// riportasse lí, ogni avvio del server rileggerebbe una settimana.
	if p.Modo != api.ModoAggiornamento {
		t.Errorf("modo = %q dopo il riavvio, atteso %q", p.Modo, api.ModoAggiornamento)
	}
	per := finestreNelPayload(p)
	if per["Inbox"].Bootstrap {
		t.Error("dopo il riavvio l'Inbox è tornata in bootstrap: la copertura sta in database, non nel processo")
	}
	// la copertura MENO la sovrapposizione, e la sottrazione la fa il server
	if d := dalDi(t, p, "Inbox"); !d.Equal(arrivato.Add(-SovrapposizioneSync)) {
		t.Errorf("dopo il riavvio l'Inbox riparte da %v, atteso %v (copertura %v meno la sovrapposizione)",
			d, arrivato.Add(-SovrapposizioneSync), arrivato)
	}
	if !per["Sent Items"].Bootstrap {
		t.Error("la Posta inviata non è mai stata sincronizzata: doveva essere in bootstrap")
	}
	// J: le due frontiere sono indipendenti. Una cartella senza copertura parte dalla finestra
	// iniziale e non da quella dell'altra, che su di lei non dice niente.
	attorno(t, dalDi(t, p, "Sent Items"), time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), "Posta inviata, mai sincronizzata")
	attorno(t, p.Dal, time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), "inviluppo dopo il riavvio")
}

// SI4 — `dal` è un override esplicito: vale per le cartelle senza cursore, e non ne sposta nessuna
// che ne abbia uno. Serve agli import controllati, non al funzionamento normale.
func TestSI4DalEUnOverrideEsplicito(t *testing.T) {
	_, q, ctx := preparaDB(t)
	casella := casellaDiProva(t, ctx, q, "import@azienda.example")
	arrivato := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	scriviCopertura(t, ctx, q, casella, "Inbox", arrivato)

	voluto := time.Date(2026, 1, 15, 0, 0, 0, 0, time.Local)
	j, err := AccodaSyncCasella(ctx, q, casella, SyncOpzioni{Cartelle: []string{"Inbox", "Sent Items"}, Dal: voluto, Lotto: 50})
	if err != nil {
		t.Fatal(err)
	}
	p := payloadSync(t, j)
	if !p.Dal.Equal(voluto) {
		t.Errorf("dal = %v, atteso l'override %v", p.Dal, voluto)
	}
	if d := dalDi(t, p, "Sent Items"); !d.Equal(voluto) {
		t.Errorf("la Posta inviata, che non ha copertura, non ha usato l'override: %v", d)
	}
	if d := dalDi(t, p, "Inbox"); !d.Equal(arrivato.Add(-SovrapposizioneSync)) {
		t.Errorf("l'override ha scavalcato la copertura dell'Inbox: %v, attesa %v", d, arrivato.Add(-SovrapposizioneSync))
	}
	if finestreNelPayload(p)["Inbox"].Bootstrap {
		t.Error("l'Inbox ha una copertura: l'override non deve rimetterla in bootstrap")
	}
}
