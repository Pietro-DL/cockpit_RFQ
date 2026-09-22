//go:build integrazione

// L4 — I15 (voce 1.11): il sync non annulla le decisioni già prese.
//
// Il sync ripassa continuamente sugli stessi elementi: la finestra di sovrapposizione è di dieci
// minuti per costruzione, e un elemento spostato o modificato in Outlook torna comunque. Ogni ripasso
// riesegue l'ingest dello stesso messaggio, e l'ingest scrive anche le INTERPRETAZIONI — la proposta
// di triage, le proposte sui documenti, i riferimenti al portale.
//
// Il difetto che questo test impedisce è di quelli che nessuno segnala come un errore: un messaggio
// deciso la mattina — agganciato a una RFQ, o chiuso come «ignora» — che il pomeriggio ricompare fra
// gli orfani perché una scansione gli ha rimesso sopra una proposta nuova. Chi lavora lo decide una
// seconda volta, e la RFQ finisce doppia. Nessun errore, nessun log: solo lavoro rifatto.
//
// Questo test è scritto contro le decisioni, non contro l'implementazione: non verifica che ci sia
// una clausola in una query, verifica che dopo un risync le decisioni siano dove l'operatore le ha
// lasciate. Se qualcuno riscrive l'ingest in un altro modo, il test resta valido.
package ingest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// statoDecisione è la fotografia di ciò che l'operatore ha deciso su un messaggio.
type statoDecisione struct {
	threadID      uuid.NullUUID
	aggancio      string
	triageStato   string
	triageEsito   string
	nTriage       int
	nProposte     int
	statiProposte []string
	nRiferimenti  int
}

func fotografia(t *testing.T, ctx context.Context, p *pgxpool.Pool, chiave string) statoDecisione {
	t.Helper()
	var s statoDecisione
	var msgID uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id, thread_id, aggancio::text FROM messaggio WHERE chiave_esterna = $1`,
		chiave).Scan(&msgID, &s.threadID, &s.aggancio); err != nil {
		t.Fatalf("messaggio %s: %v", chiave, err)
	}
	if err := p.QueryRow(ctx, `SELECT count(*) FROM proposta_triage WHERE messaggio_id = $1`, msgID).Scan(&s.nTriage); err != nil {
		t.Fatal(err)
	}
	if s.nTriage > 0 {
		if err := p.QueryRow(ctx, `SELECT stato::text, esito::text FROM proposta_triage WHERE messaggio_id = $1`,
			msgID).Scan(&s.triageStato, &s.triageEsito); err != nil {
			t.Fatal(err)
		}
	}
	righe, err := p.Query(ctx, `SELECT d.stato::text FROM documento_proposta d JOIN allegato a USING (allegato_id)
		WHERE a.messaggio_id = $1 ORDER BY a.indice`, msgID)
	if err != nil {
		t.Fatal(err)
	}
	defer righe.Close()
	for righe.Next() {
		var st string
		if err := righe.Scan(&st); err != nil {
			t.Fatal(err)
		}
		s.statiProposte = append(s.statiProposte, st)
	}
	s.nProposte = len(s.statiProposte)
	if err := p.QueryRow(ctx, `SELECT count(*) FROM riferimento_portale WHERE messaggio_id = $1`, msgID).Scan(&s.nRiferimenti); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIlRisyncNonAnnullaLeDecisioni(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	q := db.New(p)

	// primo passaggio del sync: tre messaggi orfani, ognuno con la sua proposta di triage
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: tre(nil)}); err != nil {
		t.Fatalf("primo ingest: %v", err)
	}

	// --- l'operatore decide -------------------------------------------------
	// il primo va su una RFQ nuova, il secondo viene ignorato, il terzo resta orfano (controllo)
	var clienteID, threadID uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME','Acme SpA')
		RETURNING cliente_id`).Scan(&clienteID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
		VALUES ($1, 'outlook', now(), 'RFQ di prova 1', 'ACME\WIP\2026 09 10 RFQ di prova 1')
		RETURNING thread_id`, clienteID).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	primo, err := q.GetMessaggioPerChiave(ctx, db.GetMessaggioPerChiaveParams{Canale: db.CanaleOutlook, ChiaveEsterna: "<pp-1@acme.example>"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{
		MessaggioID: primo.MessaggioID, ThreadID: uuid.NullUUID{UUID: threadID, Valid: true}, Aggancio: db.AggancioOperatore,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.DecidiTriage(ctx, db.DecidiTriageParams{MessaggioID: primo.MessaggioID, Stato: db.StatoTriageAccettata}); err != nil {
		t.Fatal(err)
	}
	// e scarta la proposta sull'allegato: è una decisione come le altre, e ricrearla sarebbe un
	// documento che l'operatore aveva detto di non volere
	if _, err := p.Exec(ctx, `UPDATE documento_proposta SET stato = 'scartata'
		WHERE allegato_id IN (SELECT allegato_id FROM allegato WHERE messaggio_id = $1)`, primo.MessaggioID); err != nil {
		t.Fatal(err)
	}

	secondo, err := q.GetMessaggioPerChiave(ctx, db.GetMessaggioPerChiaveParams{Canale: db.CanaleOutlook, ChiaveEsterna: "<pp-2@acme.example>"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.IgnoraMessaggio(ctx, db.IgnoraMessaggioParams{MessaggioID: secondo.MessaggioID}); err != nil {
		t.Fatal(err)
	}

	prima := map[string]statoDecisione{}
	for _, k := range []string{"<pp-1@acme.example>", "<pp-2@acme.example>", "<pp-3@acme.example>"} {
		prima[k] = fotografia(t, ctx, p, k)
	}
	if prima["<pp-1@acme.example>"].threadID.UUID != threadID {
		t.Fatal("il primo messaggio non risulta agganciato: la premessa del test non regge")
	}
	if prima["<pp-2@acme.example>"].triageStato != "rifiutata" {
		t.Fatalf("il secondo non risulta ignorato: stato = %q", prima["<pp-2@acme.example>"].triageStato)
	}

	// --- il sync ripassa ----------------------------------------------------
	// due volte, e la seconda con l'elemento spostato di cartella e con il flag cambiato: è ciò che
	// succede davvero quando qualcuno archivia o legge il messaggio in Outlook.
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: tre(nil)}); err != nil {
		t.Fatalf("risync: %v", err)
	}
	mosso := tre(nil)
	for i := range mosso {
		mosso[i].Cartella = "RFQ archiviate"
		mosso[i].EntryID = mosso[i].EntryID + "-MOSSO"
		mosso[i].NonLetto = false
	}
	r, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: mosso})
	if err != nil {
		t.Fatalf("risync dopo lo spostamento: %v", err)
	}
	if r.Inseriti != 0 || r.Aggiornati != 3 {
		t.Fatalf("risync: inseriti=%d aggiornati=%d (attesi 0 e 3)", r.Inseriti, r.Aggiornati)
	}

	// --- niente è cambiato --------------------------------------------------
	for _, k := range []string{"<pp-1@acme.example>", "<pp-2@acme.example>", "<pp-3@acme.example>"} {
		dopo := fotografia(t, ctx, p, k)
		p0 := prima[k]
		if dopo.threadID != p0.threadID {
			t.Errorf("%s: thread_id passato da %v a %v", k, p0.threadID, dopo.threadID)
		}
		if dopo.aggancio != p0.aggancio {
			t.Errorf("%s: aggancio passato da %q a %q", k, p0.aggancio, dopo.aggancio)
		}
		if dopo.nTriage != p0.nTriage {
			t.Errorf("%s: proposte di triage %d → %d (il sync ne ha aggiunta una)", k, p0.nTriage, dopo.nTriage)
		}
		if dopo.triageStato != p0.triageStato || dopo.triageEsito != p0.triageEsito {
			t.Errorf("%s: triage riaperto dal sync: (%q,%q) → (%q,%q)", k, p0.triageStato, p0.triageEsito, dopo.triageStato, dopo.triageEsito)
		}
		if dopo.nProposte != p0.nProposte {
			t.Errorf("%s: proposte sui documenti %d → %d", k, p0.nProposte, dopo.nProposte)
		}
		for i := range dopo.statiProposte {
			if i < len(p0.statiProposte) && dopo.statiProposte[i] != p0.statiProposte[i] {
				t.Errorf("%s: proposta %d riaperta dal sync: %q → %q", k, i, p0.statiProposte[i], dopo.statiProposte[i])
			}
		}
		if dopo.nRiferimenti != p0.nRiferimenti {
			t.Errorf("%s: riferimenti al portale %d → %d", k, p0.nRiferimenti, dopo.nRiferimenti)
		}
	}

	// e il messaggio agganciato non è tornato fra gli orfani: è la forma in cui il difetto si
	// vedrebbe da chi lavora, ed è l'unica che conti davvero.
	var orfani int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM messaggio WHERE thread_id IS NULL
		AND chiave_esterna = '<pp-1@acme.example>'`).Scan(&orfani); err != nil {
		t.Fatal(err)
	}
	if orfani != 0 {
		t.Error("il messaggio agganciato è tornato orfano dopo il sync")
	}
}
