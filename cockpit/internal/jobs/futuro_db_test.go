//go:build integrazione

// L4 — un cursore nel futuro non blocca la casella (correzione del 16/09/2026).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/jobs/
//
// La guardia dell'ingest impedisce che un cursore impossibile venga SCRITTO. Questo è l'altro lato:
// in database ce ne sono già — quelli del 16/09, due ore avanti — e finché ci sono la finestra del
// sync comincerebbe nel futuro e quella casella non leggerebbe più niente. Ignorarli qui è ciò che
// fa ripartire il sync da solo, senza una correzione a mano su un database di produzione.
package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

func casellaDiProva(t *testing.T, ctx context.Context, q *db.Queries, indirizzo string) db.Casella {
	t.Helper()
	c, err := q.UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: indirizzo, Nome: "Prova", Condivisa: false,
	})
	if err != nil {
		t.Fatalf("casella: %v", err)
	}
	return c
}

func TestUnCursoreNelFuturoNonFermaIlSyncDellaCasella(t *testing.T) {
	_, q, ctx := preparaDB(t)
	casella := casellaDiProva(t, ctx, q, "commerciale@azienda.it")

	futuro := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	sano := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)
	scriviCopertura(t, ctx, q, casella, "Inbox", futuro)    // scritta prima della correzione
	scriviCopertura(t, ctx, q, casella, "Sent Items", sano) // scritta bene

	j, err := AccodaSyncCasella(ctx, q, casella, SyncOpzioni{Cartelle: []string{"Inbox", "Sent Items"}, Lotto: 50})
	if err != nil || j == nil {
		t.Fatalf("il sync non è stato accodato: %v", err)
	}
	var p worker.PayloadSyncOutlook
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	per := finestreNelPayload(p)
	if !per["Inbox"].Bootstrap {
		t.Errorf("l'Inbox riparte dalla copertura nel futuro (%v): la finestra comincerebbe fra due ore", per["Inbox"].Dal)
	}
	if worker.NelFuturo(dalDi(t, p, "Inbox"), time.Now()) {
		t.Errorf("la finestra dell'Inbox comincia nel futuro: %v", dalDi(t, p, "Inbox"))
	}
	if d := dalDi(t, p, "Sent Items"); !d.Equal(sano.Add(-SovrapposizioneSync)) {
		t.Errorf("la copertura buona è stata buttata via: la Posta inviata riparte da %v, atteso %v", d, sano.Add(-SovrapposizioneSync))
	}
	// `dal` è l'inviluppo, e qui lo decide l'Inbox: la sua copertura è stata scartata, quindi è in
	// bootstrap. Dal checkpoint del 16/09/2026 riparte dalla finestra iniziale.
	//
	// Prima ripartiva dal più vecchio dei cursori delle ALTRE cartelle — qui la Posta inviata, mezz'ora
	// fa — che su dove fosse arrivata l'Inbox non dice niente: le avrebbe fatto saltare tutto ciò che
	// era entrato prima, cioè esattamente la posta che il cursore nel futuro aveva già nascosto.
	if worker.NelFuturo(p.Dal, time.Now()) {
		t.Errorf("il limite inferiore della finestra è nel futuro: %v", p.Dal)
	}
	attorno(t, p.Dal, time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), "limite inferiore dell'Inbox senza cursore")
	if p.Dal.After(sano) {
		t.Errorf("dal = %v: l'Inbox è ripartita dal cursore della Posta inviata (%v)", p.Dal, sano)
	}
}

func TestSenzaCursoriUtilizzabiliLaFinestraTornaQuellaPredefinita(t *testing.T) {
	_, q, ctx := preparaDB(t)
	casella := casellaDiProva(t, ctx, q, "francesco@azienda.it")
	futuro := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{
		CasellaID: casella.CasellaID, Cartella: "Inbox", UltimoReceived: &futuro,
	}); err != nil {
		t.Fatal(err)
	}

	j, err := AccodaSyncCasella(ctx, q, casella, SyncOpzioni{Cartelle: []string{"Inbox"}, Lotto: 50})
	if err != nil || j == nil {
		t.Fatalf("il sync non è stato accodato: %v", err)
	}
	var p worker.PayloadSyncOutlook
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatal(err)
	}
	// L'unico cursore era inutilizzabile: si rilegge la finestra iniziale, che costa una deduplica per
	// Message-ID. L'alternativa — fidarsi — costa la posta finché il futuro non è passato.
	attorno(t, p.Dal, time.Now().AddDate(0, 0, -GiorniSyncInizialeDefault), "finestra senza cursori utilizzabili")
	if p.CasellaID == nil || *p.CasellaID != casella.CasellaID {
		t.Errorf("il payload non porta la casella: %v", p.CasellaID)
	}
}
