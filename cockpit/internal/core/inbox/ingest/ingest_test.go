//go:build integrazione

package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// Test d'integrazione su DB reale (SPEC blocco 2), L4 come gli altri *_db_test di questo pacchetto:
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/core/inbox/ingest/
//
// Senza COCKPIT_TEST_DSN vengono saltati; il DSN passa dalla guardia di testutil, che rifiuta un
// database il cui nome non contiene "test". Le righe create hanno message_id con prefisso
// "<test-ingest-" e vengono rimosse alla fine.

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testutil.Pool(t)         // salta senza DSN, rifiuta un database che non e' di test, chiude alla fine
	testutil.SchemaPresente(t, p) // l'ordine dei pacchetti non è garantito: lo schema può essere stato ricreato
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = p.Exec(ctx, `DELETE FROM ingest_scarto WHERE casella_id IN (SELECT casella_id FROM casella WHERE indirizzo = 'prova-ingest@azienda.example')`)
		_, _ = p.Exec(ctx, `DELETE FROM messaggio_aggancio_log WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM job WHERE chiave_idempotenza LIKE 'stage:%' AND payload->>'entry_id' LIKE 'ENTRY-TEST-%'`)
		_, _ = p.Exec(ctx, `DELETE FROM proposta_triage WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		// Tabelle del checkpoint 3R. Vanno prima del messaggio come tutte le altre: una pulizia che
		// non le conosce fa fallire in silenzio la DELETE sul messaggio, e la suite passa solo
		// finche' qualcun altro azzera lo schema.
		_, _ = p.Exec(ctx, `DELETE FROM candidato_aggancio WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM candidato_codice WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM analisi_messaggio WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM riferimento_portale WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		// `documento_proposta` PRIMA di `allegato`: ogni allegato non inline ne genera una, e senza
		// questa riga la DELETE sull'allegato falliva sulla chiave esterna — in silenzio, perche'
		// l'errore non si guarda — e con lei restavano in tabella anche i messaggi. Il test tornava
		// a funzionare solo perche' qualche altro pacchetto, prima o poi, azzerava lo schema.
		_, _ = p.Exec(ctx, `DELETE FROM documento_proposta WHERE allegato_id IN (SELECT allegato_id FROM allegato a JOIN messaggio m USING (messaggio_id) WHERE m.chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM allegato WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM messaggio_outlook WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		// `messaggio_casella` e' arrivata con la 0004 (una presenza per casella) e questa pulizia e'
		// piu' vecchia di lei: senza questa riga la DELETE sul messaggio falliva sulla chiave
		// esterna, e i messaggi restavano in tabella fino al prossimo azzeramento dello schema.
		_, _ = p.Exec(ctx, `DELETE FROM messaggio_casella WHERE messaggio_id IN (SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%')`)
		_, _ = p.Exec(ctx, `DELETE FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%'`)
		_, _ = p.Exec(ctx, `DELETE FROM conversazione WHERE chiave_esterna LIKE 'CONV-TEST-%'`)
		// il pool lo chiude testutil.Pool: i Cleanup girano al contrario, quindi dopo questa pulizia
	})
	return p
}

// clienteDiProva censisce un cliente con il suo dominio, e lo toglie alla fine: dal 7B la
// proposta di una RFQ nuova nasce solo da un mittente censito come cliente (7B.3), quindi i test
// che la aspettano devono dirlo.
func clienteDiProva(t *testing.T, p *pgxpool.Pool, cartella, dominio string) {
	t.Helper()
	ctx := context.Background()
	q := db.New(p)
	c, err := q.InsertCliente(ctx, db.InsertClienteParams{CartellaNas: cartella, RagioneSociale: cartella, Regole: json.RawMessage("{}")})
	if err != nil {
		if existing, e := q.GetClientePerCartella(ctx, cartella); e == nil {
			c = existing
		} else {
			t.Fatalf("cliente di prova: %v", err)
		}
	}
	if err := q.InsertDominioCliente(ctx, db.InsertDominioClienteParams{Lower: dominio, ClienteID: c.ClienteID}); err != nil {
		if _, e := q.GetClientePerDominio(ctx, dominio); e != nil {
			t.Fatalf("dominio di prova: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, `UPDATE messaggio SET controparte_tipo = 'sconosciuto', controparte_cliente_id = NULL WHERE controparte_cliente_id = $1`, c.ClienteID)
		_, _ = p.Exec(ctx, `UPDATE proposta_triage SET cliente_proposto = NULL WHERE cliente_proposto = $1`, c.ClienteID)
		_, _ = p.Exec(ctx, `DELETE FROM dominio_cliente WHERE cliente_id = $1`, c.ClienteID)
		_, _ = p.Exec(ctx, `DELETE FROM cliente WHERE cliente_id = $1`, c.ClienteID)
	})
}

// casellaProva assicura che esista la casella a cui appartengono i lotti dei test. Dalla fase 1 un
// lotto senza casella non esiste: la casella è il perimetro entro cui il messaggio viene acquisito.
func casellaProva(t *testing.T, p *pgxpool.Pool) db.Casella {
	t.Helper()
	ctx := context.Background()
	q := db.New(p)
	c, err := q.UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: "prova-ingest@azienda.example", Nome: "Prova ingest",
	})
	if err != nil {
		t.Fatalf("casella di prova: %v", err)
	}
	return c
}

func lotto() []worker.MessaggioIn {
	t0 := time.Date(2026, 9, 8, 9, 30, 0, 0, time.UTC)
	return []worker.MessaggioIn{
		{
			MessageID: "<test-ingest-1@acme.example>", EntryID: "ENTRY-TEST-1", StoreID: "STORE-TEST", ConversationID: "CONV-TEST-1",
			Cartella: "Inbox", Direzione: "entrata", DataEvento: t0, MittenteNome: "Mario Rossi", MittenteIndirizzo: "mario.rossi@acme.example",
			Oggetto: "RFQ 1234567A supporto cofano", CorpoTesto: "Buongiorno, richiesta d'offerta per il codice 1234567A rev 4.\r\nVi abbiamo caricato sul portale i CAD dei codici 1234568B e 1234569C. Risposta entro il 15/09/2026.",
			Riferimenti: []string{}, Categorie: []string{},
			Allegati: []worker.AllegatoIn{
				{Indice: 1, NomeFile: "1234567A_4.pdf", Estensione: "pdf", Natura: "file", Bytes: 120000},
				{Indice: 2, NomeFile: "image001.png", Estensione: "png", Natura: "inline", Bytes: 4000, ContentID: "image001.png@01"},
			},
		},
		{
			MessageID: "<test-ingest-2@acme.example>", EntryID: "ENTRY-TEST-2", StoreID: "STORE-TEST", ConversationID: "CONV-TEST-1",
			Cartella: "Inbox", Direzione: "entrata", DataEvento: t0.Add(time.Hour), MittenteNome: "Mario Rossi", MittenteIndirizzo: "mario.rossi@acme.example",
			Oggetto: "R: RFQ 1234567A supporto cofano", CorpoTesto: "Dimenticavo lo STEP.", Riferimenti: []string{}, Categorie: []string{},
			Allegati: []worker.AllegatoIn{{Indice: 1, NomeFile: "1234567A.stp", Estensione: "stp", Natura: "file", Bytes: 900000}},
		},
	}
}

const stageInCoda = `SELECT count(*) FROM job WHERE tipo = 'stage_allegato' AND stato IN ('pronto','in_corso')`

func TestIngestIdempotente(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	s := &Servizio{Pool: p, Log: slog.Default()}
	// Dal 7B una RFQ si propone solo a un mittente censito come cliente: acme.example lo e'.
	clienteDiProva(t, p, "ACME IDEMPOTENTE", "acme.example")

	conta := func() (msg, all, rif, tri int) {
		_ = p.QueryRow(ctx, `SELECT count(*) FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-%'`).Scan(&msg)
		_ = p.QueryRow(ctx, `SELECT count(*) FROM allegato a JOIN messaggio m USING (messaggio_id) WHERE m.chiave_esterna LIKE '<test-ingest-%'`).Scan(&all)
		_ = p.QueryRow(ctx, `SELECT count(*) FROM riferimento_portale r JOIN messaggio m USING (messaggio_id) WHERE m.chiave_esterna LIKE '<test-ingest-%'`).Scan(&rif)
		_ = p.QueryRow(ctx, `SELECT count(*) FROM proposta_triage r JOIN messaggio m USING (messaggio_id) WHERE m.chiave_esterna LIKE '<test-ingest-%'`).Scan(&tri)
		return
	}

	casella := casellaProva(t, p)
	// Quanti `stage_allegato` c'erano PRIMA. Si misura la differenza e non il totale: `pool` qui non
	// azzera lo schema, e un job lasciato da un'altra prova faceva cadere questa accusando l'ingest
	// di un download automatico che non aveva fatto.
	var stagePrima int
	_ = p.QueryRow(ctx, stageInCoda).Scan(&stagePrima)

	r1, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: lotto()})
	if err != nil {
		t.Fatal(err)
	}
	if r1.Inseriti != 2 || r1.Aggiornati != 0 {
		t.Fatalf("primo ingest: %+v", r1)
	}
	// nessun download automatico: zero job stage_allegato, ma una proposta (dal nome) per ogni allegato non inline
	var nStage, nProposte int
	_ = p.QueryRow(ctx, stageInCoda).Scan(&nStage)
	nStage -= stagePrima
	_ = p.QueryRow(ctx, `SELECT count(*) FROM documento_proposta d JOIN allegato a USING (allegato_id) JOIN messaggio m USING (messaggio_id) WHERE m.chiave_esterna LIKE '<test-ingest-%'`).Scan(&nProposte)
	if nStage != 0 || nProposte != 2 {
		t.Errorf("staging automatico=%d (atteso 0), proposte=%d (attese 2: inline escluso)", nStage, nProposte)
	}
	msg, all, rif, tri := conta()
	if msg != 2 || all != 3 || rif != 2 || tri != 2 {
		t.Fatalf("dopo il primo ingest: messaggi=%d allegati=%d rif_portale=%d triage=%d", msg, all, rif, tri)
	}

	// stesso lotto altre due volte → nessuna riga in più
	for i := 0; i < 2; i++ {
		r, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: lotto()})
		if err != nil {
			t.Fatal(err)
		}
		if r.Inseriti != 0 || r.Aggiornati != 2 {
			t.Fatalf("ingest ripetuto %d: %+v", i+2, r)
		}
	}
	if m2, a2, r2, t2 := conta(); m2 != msg || a2 != all || r2 != rif || t2 != tri {
		t.Fatalf("il replay ha creato righe: %d/%d %d/%d %d/%d %d/%d", m2, msg, a2, all, r2, rif, t2, tri)
	}

	// EntryID cambiato (elemento spostato di cartella) → aggiornato, non duplicato
	mosso := lotto()
	mosso[0].EntryID = "ENTRY-TEST-1-MOSSO"
	mosso[0].Cartella = "RFQ archiviate"
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: mosso[:1]}); err != nil {
		t.Fatal(err)
	}
	// dalla 0004 entry_id e cartella stanno nella PRESENZA: sono fatti della copia in quella casella
	var entry, cart string
	_ = p.QueryRow(ctx, `SELECT mc.entry_id, mc.cartella FROM messaggio_casella mc JOIN messaggio m USING (messaggio_id) WHERE m.chiave_esterna = '<test-ingest-1@acme.example>'`).Scan(&entry, &cart)
	if entry != "ENTRY-TEST-1-MOSSO" || cart != "RFQ archiviate" {
		t.Errorf("entry_id non aggiornato: %s %s", entry, cart)
	}

	// triage deterministico: RFQ con codici, allegato tecnico e scadenza rilevata
	q := db.New(p)
	m, err := q.GetMessaggioPerChiave(ctx, db.GetMessaggioPerChiaveParams{Canale: db.CanaleOutlook, ChiaveEsterna: "<test-ingest-1@acme.example>"})
	if err != nil {
		t.Fatal(err)
	}
	var esito string
	var conf int16
	var scad *time.Time
	var ident []string
	_ = p.QueryRow(ctx, `SELECT esito, confidenza, scadenza_proposta, identificativi FROM proposta_triage WHERE messaggio_id = $1`, m.MessaggioID).Scan(&esito, &conf, &scad, &ident)
	if esito != "nuova_rfq" || conf < 50 || scad == nil || scad.Day() != 15 {
		t.Errorf("triage: esito=%s conf=%d scad=%v ident=%v", esito, conf, scad, ident)
	}
	if len(ident) == 0 || ident[0] != "1234567A" {
		t.Errorf("identificativi proposti: %v", ident)
	}
}
