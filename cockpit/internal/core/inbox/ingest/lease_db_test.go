//go:build integrazione

// L4 — revisione del 25/09: il lease autorizza il lotto del SUO job, non un lotto qualunque.
//
// Il tentativo si verificava (token, worker, scadenza) e basta. Con il lease di un download, o del
// sync di un'altra casella, si scrivevano messaggi, presenze e cursori dove quel job non era mai stato
// mandato. Qui si prova il controllo che ora sta dentro Ingerisci, con la riga del job bloccata; il
// controllo della credenziale sta nel gestore HTTP (workerapi).
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// jobInCorso inserisce un job già preso da un worker, con il suo lease valido, e restituisce il
// tentativo con cui quel worker consegnerebbe un lotto. `casella` NULL = colonna vuota (i job
// accodati prima della 0004 avevano la casella solo nel payload).
func jobInCorso(t *testing.T, p *pgxpool.Pool, tipo db.TipoJob, casella uuid.NullUUID, payload any) *Tentativo {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	var token uuid.UUID
	if err := p.QueryRow(context.Background(), `
		INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, casella_id, stato, worker_id,
		                 lease_token, avviato_il, lease_fino_a)
		VALUES ($1, $2, $3, 300, 1800, $4, 'in_corso', 'outlook@PC-PROVA', gen_random_uuid(), now(), now() + interval '5 minutes')
		RETURNING job_id, lease_token`, string(tipo), string(coda.WorkerPer(tipo)), raw, casella).Scan(&id, &token); err != nil {
		t.Fatalf("job %s in corso: %v", tipo, err)
	}
	return &Tentativo{JobID: id, LeaseToken: token, WorkerID: "outlook@PC-PROVA"}
}

// nienteScritto è l'affermazione che conta per un lotto rifiutato: il database è com'era.
func nienteScritto(t *testing.T, p *pgxpool.Pool, perche string) {
	t.Helper()
	for _, tabella := range []string{"messaggio", "messaggio_casella", "allegato", "ingest_scarto", "sync_cursore"} {
		if n := testutil.Conta(t, p, tabella); n != 0 {
			t.Errorf("%s: %d righe in %s — un lotto rifiutato non deve scrivere niente", perche, n, tabella)
		}
	}
}

func TestIlLeaseDiUnJobCheNonConsegnaPostaNonScriveNulla(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	perCasella := uuid.NullUUID{UUID: casella.CasellaID, Valid: true}
	for _, tipo := range []db.TipoJob{db.TipoJobStageAllegato, db.TipoJobApriElementoOutlook, db.TipoJobSegnaLetto} {
		tent := jobInCorso(t, p, tipo, perCasella, map[string]any{"entry_id": "ENTRY-PP-1"})
		_, err := s.Ingerisci(ctx, Lotto{Casella: casella, Tentativo: tent, Messaggi: tre(nil),
			Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}})
		if !errors.Is(err, ErrLottoNonDelJob) {
			t.Fatalf("lease di un job %s: errore = %v, atteso ErrLottoNonDelJob (403)", tipo, err)
		}
		nienteScritto(t, p, "lease di un "+string(tipo))
	}
}

func TestIlLeaseDiUnSyncNonScriveInUnAltraCasella(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	altra, err := db.New(p).UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: "acquisti@azienda.example", Nome: "Acquisti"})
	if err != nil {
		t.Fatal(err)
	}
	perCasella := uuid.NullUUID{UUID: casella.CasellaID, Valid: true}
	cid := casella.CasellaID
	casi := []struct {
		nome string
		tent *Tentativo
	}{
		{"sync con la casella nella colonna", jobInCorso(t, p, db.TipoJobSyncOutlook, perCasella, worker.PayloadSyncOutlook{CasellaID: &cid})},
		{"sync di prima della 0004, casella solo nel payload", jobInCorso(t, p, db.TipoJobSyncOutlook, uuid.NullUUID{}, worker.PayloadSyncOutlook{CasellaID: &cid})},
		{"rilettura di un elemento", jobInCorso(t, p, db.TipoJobRileggiElemento, uuid.NullUUID{},
			worker.PayloadRileggiElemento{CasellaID: cid, EntryID: "ENTRY-PP-2"})},
	}
	for _, c := range casi {
		// il job è di Commerciale, il lotto dichiara Acquisti
		_, err := s.Ingerisci(ctx, Lotto{Casella: altra, Tentativo: c.tent, Messaggi: tre(nil),
			Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}})
		if !errors.Is(err, ErrLottoNonDelJob) {
			t.Fatalf("%s: errore = %v, atteso ErrLottoNonDelJob", c.nome, err)
		}
		nienteScritto(t, p, c.nome)
	}
	// lo stesso lotto per la casella del job entra: il rifiuto riguardava la casella, non il dato
	r, err := s.Ingerisci(ctx, Lotto{Casella: casella, Tentativo: casi[0].tent, Messaggi: tre(nil),
		Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}})
	if err != nil || r.Inseriti != 3 {
		t.Fatalf("lotto della casella del job: %+v %v", r, err)
	}
	if c := cursore(t, p); c == nil || !c.Equal(cursoreFinale) {
		t.Errorf("cursore = %v", c)
	}
}
