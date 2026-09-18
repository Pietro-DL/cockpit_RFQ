//go:build integrazione

// L4 — blocco 4A: «Carica precedenti» non scarica gli allegati.
//
// Il difetto si vedeva nel log di una prova reale: alle 11:14:57 qualcuno preme «Carica precedenti»,
// e pochi secondi dopo partono da soli zip, PDF, DWG e TIF di due giorni di archivio; poi il worker
// di analisi esegue decine di job in pochi secondi. Un clic per rendere consultabile la posta vecchia
// diventava ore di lavoro, qualche giga di disco e un'apertura di Outlook per ogni allegato.
//
// La causa era un'assenza: nessuna delle condizioni dello staging automatico guardava PERCHE' il
// messaggio stesse entrando. Un messaggio del 7 settembre che arriva oggi perche' lo si e' chiesto
// allo storico era indistinguibile da uno arrivato stamattina.
//
// Il modo lo dichiara il payload del job, che ha scritto il server all'accodamento (blocco 3). Questi
// test lo consegnano come lo consegna il worker vero: con un job accodato, preso in carico, e il
// tentativo che il lotto esibisce. Un test che passasse il modo a mano proverebbe la switch e non la
// strada che il modo percorre davvero.
package ingest

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/jobs"
)

// bancoStorico prepara cliente riconosciuto, casella e pulizia dei job di questi test.
func bancoStorico(t *testing.T, p *pgxpool.Pool) db.Casella {
	t.Helper()
	casella := casellaProva(t, p)
	bancoCandidati(t, p)
	ctx := context.Background()
	pulisci := func() {
		_, _ = p.Exec(ctx, `DELETE FROM job WHERE payload->>'entry_id' LIKE 'ENTRY-TEST-3R-M%'
			OR chiave_idempotenza LIKE 'prova-modo:%'`)
	}
	pulisci()
	t.Cleanup(pulisci)
	return casella
}

// consegna accoda un job del tipo e del payload indicati, lo fa prendere in carico da un worker, e
// consegna il lotto DENTRO quel tentativo — cioe' esattamente il giro che fa il worker vero.
func consegna(t *testing.T, s *Servizio, casella db.Casella, tipo db.TipoJob, payload any, chiave string, messaggi []api.MessaggioIn) {
	t.Helper()
	ctx := context.Background()
	q := db.New(s.Pool)
	j, err := jobs.AccodaCon(ctx, q, tipo, payload, chiave, 5,
		jobs.Opzioni{Casella: uuid.NullUUID{UUID: casella.CasellaID, Valid: true}})
	if err != nil {
		t.Fatalf("accodamento del job di prova: %v", err)
	}
	if j == nil {
		t.Fatalf("job di prova non accodato: chiave %q gia' in coda", chiave)
	}
	preso, err := jobs.Claim(ctx, q, db.WorkerTipoOutlook, "prova-modo",
		jobs.Destinazione{Caselle: []uuid.UUID{casella.CasellaID}}, 2*time.Second)
	if err != nil || preso == nil {
		t.Fatalf("claim del job di prova: %v", err)
	}
	if preso.JobID != j.JobID {
		t.Fatalf("il claim ha preso il job %d invece del %d: nella coda e' rimasto altro", preso.JobID, j.JobID)
	}
	if _, err := s.Ingerisci(ctx, Lotto{
		Casella:   casella,
		Tentativo: &Tentativo{JobID: preso.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "prova-modo"},
		Messaggi:  messaggi,
	}); err != nil {
		t.Fatalf("ingest del lotto: %v", err)
	}
}

// scaricati conta i download accodati per gli elementi di questi test.
func scaricati(t *testing.T, p *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(),
		`SELECT count(*) FROM job WHERE tipo = 'stage_allegato' AND payload->>'entry_id' LIKE 'ENTRY-TEST-3R-M%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// msgConAllegato e' un messaggio di un cliente RICONOSCIUTO con un PDF sotto soglia: tutte le
// condizioni dello staging automatico sono soddisfatte, tranne eventualmente il modo.
func msgConAllegato(n string) api.MessaggioIn {
	return api.MessaggioIn{
		MessageID: "<test-ingest-3r-m" + n + "@prova3r.example>", EntryID: "ENTRY-TEST-3R-M" + n,
		StoreID: "S", ConversationID: "CONV-TEST-3R-M", Cartella: "Inbox", Direzione: "entrata",
		DataEvento:        time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC),
		MittenteIndirizzo: "buyer@prova3r.example", Oggetto: "PROVA-3R modo " + n,
		CorpoTesto: "in allegato", Riferimenti: []string{}, Categorie: []string{},
		Allegati: []api.AllegatoIn{{Indice: 1, NomeFile: "6674611A.pdf", Estensione: "pdf", Natura: "file", Bytes: 100_000}},
	}
}

func payloadSync(modo string, al *time.Time) api.PayloadSyncOutlook {
	dal := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	return api.PayloadSyncOutlook{Modo: modo, Dal: dal, Al: al, Lotto: 10,
		Cartelle: []api.CartellaCursore{{Cartella: "Inbox", Dal: &dal, Al: al}}}
}

// A — l'aggiornamento ordinario scarica come prima: questo blocco non spegne D30.
func TestAggiornamentoScaricaComePrima(t *testing.T) {
	p := pool(t)
	casella := bancoStorico(t, p)
	s := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true}
	al := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	consegna(t, s, casella, db.TipoJobSyncOutlook, payloadSync(api.ModoAggiornamento, &al), "prova-modo:agg",
		[]api.MessaggioIn{msgConAllegato("1")})

	if n := scaricati(t, p); n != 1 {
		t.Errorf("aggiornamento ordinario: %d download accodati, atteso 1", n)
	}
}

// B — LO STORICO NON SCARICA NIENTE. E' il difetto che il blocco 4A toglie.
func TestStoricoNonScaricaGliAllegati(t *testing.T) {
	p := pool(t)
	casella := bancoStorico(t, p)
	s := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true}
	al := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)

	consegna(t, s, casella, db.TipoJobSyncOutlook, payloadSync(api.ModoStorico, &al), "prova-modo:sto",
		[]api.MessaggioIn{msgConAllegato("2")})

	if n := scaricati(t, p); n != 0 {
		t.Errorf("«Carica precedenti» ha accodato %d download: lo storico rende consultabile la posta vecchia, non ne scarica gli allegati", n)
	}
	// e il messaggio e' comunque entrato: lo storico serve proprio a questo
	var visti int
	if err := p.QueryRow(context.Background(),
		`SELECT count(*) FROM messaggio WHERE chiave_esterna = '<test-ingest-3r-m2@prova3r.example>'`).Scan(&visti); err != nil {
		t.Fatal(err)
	}
	if visti != 1 {
		t.Errorf("lo storico non ha acquisito il messaggio: %d righe (attesa 1)", visti)
	}
}

// C — il bootstrap scarica soltanto se e' stato dichiarato. Spento e' il valore predefinito.
func TestBootstrapScaricaSoloSeDichiarato(t *testing.T) {
	p := pool(t)
	casella := bancoStorico(t, p)
	al := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	prudente := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true}
	consegna(t, prudente, casella, db.TipoJobSyncOutlook, payloadSync(api.ModoBootstrap, &al), "prova-modo:boot1",
		[]api.MessaggioIn{msgConAllegato("3")})
	if n := scaricati(t, p); n != 0 {
		t.Errorf("bootstrap con staging.bootstrap assente: %d download accodati, atteso 0", n)
	}

	dichiarato := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true, StagingBootstrap: true}
	consegna(t, dichiarato, casella, db.TipoJobSyncOutlook, payloadSync(api.ModoBootstrap, &al), "prova-modo:boot2",
		[]api.MessaggioIn{msgConAllegato("4")})
	if n := scaricati(t, p); n != 1 {
		t.Errorf("bootstrap con staging.bootstrap = true: %d download accodati, atteso 1", n)
	}
}

// D — un payload accodato PRIMA del blocco 3 non dichiara il modo. La scaletta di compatibilita' e'
// una sola (api.PayloadSyncOutlook.ModoEffettivo) e legge «al valorizzato» come storico: un job
// rimasto in coda durante l'aggiornamento non deve scaricare l'archivio appena il server riparte.
func TestPayloadSenzaModoConAlValeStorico(t *testing.T) {
	p := pool(t)
	casella := bancoStorico(t, p)
	s := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true}
	al := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)

	consegna(t, s, casella, db.TipoJobSyncOutlook, payloadSync("", &al), "prova-modo:vecchio",
		[]api.MessaggioIn{msgConAllegato("5")})

	if n := scaricati(t, p); n != 0 {
		t.Errorf("payload senza modo con al valorizzato: %d download accodati, atteso 0 (vale storico)", n)
	}
}

// E — la rilettura di un elemento e' un gesto di una persona su un elemento solo: scarica.
//
// Serve a dire che la regola guarda il MODO DEL SYNC, e non «non e' un sync, quindi no»: chi riprova
// uno scarto di lettura vuole quell'elemento, allegati compresi.
func TestLaRiletturaDiUnElementoScarica(t *testing.T) {
	p := pool(t)
	casella := bancoStorico(t, p)
	s := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true}

	consegna(t, s, casella, db.TipoJobRileggiElemento,
		api.PayloadRileggiElemento{CasellaID: casella.CasellaID, EntryID: "ENTRY-TEST-3R-M6", Cartella: "Inbox"},
		"prova-modo:rileggi", []api.MessaggioIn{msgConAllegato("6")})

	if n := scaricati(t, p); n != 1 {
		t.Errorf("rilettura di un elemento: %d download accodati, atteso 1", n)
	}
}
