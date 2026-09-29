//go:build integrazione

// L4 — Smistamento 4.13: la rilettura di un elemento, dal «Riprova» al lotto da uno.
//
// «Riprova» su uno scarto di lettura accodava `rileggi_elemento`, e il worker non aveva il ramo: il job
// moriva «tipo sconosciuto» e lo scarto restava lì. Ora il worker rilegge l'elemento (per EntryID, o per
// Message-ID se è stato spostato) e lo consegna in un lotto da uno dentro il tentativo del job. Qui si
// prova che cosa il server ne fa: il messaggio entra, lo scarto si chiude anche quando l'EntryID è
// cambiato, e il cursore della casella non si muove, nemmeno se il lotto ne portasse uno.
package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// cursoriComeTesto è tutta la tabella dei cursori, riga per riga: un cursore che non si muove è una
// tabella che resta uguale, non solo un ultimo_received che resta uguale.
func cursoriComeTesto(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	var s string
	if err := p.QueryRow(context.Background(),
		`SELECT coalesce(json_agg(row_to_json(c) ORDER BY c.cartella)::text, '') FROM sync_cursore c`).Scan(&s); err != nil {
		t.Fatalf("cursori: %v", err)
	}
	return s
}

func TestLaRiletturaDiUnElementoEntraENonMuoveICursori(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	q := db.New(p)
	quando := ppBase.Add(30 * time.Minute)

	// il sync: tre elementi entrano, uno non si converte, e il cursore arriva in fondo alla finestra
	if _, err := s.Ingerisci(ctx, Lotto{
		Casella:  casella,
		Messaggi: tre(nil),
		Saltati: []worker.ElementoSaltato{{
			EntryID: "ENTRY-PP-SALTATO", Cartella: cartellaPP, MessageID: "<pp-saltato@acme.example>",
			RicevutoIl: &quando, Oggetto: "elemento non convertito", Errore: "com_error su Body",
		}},
		Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	}); err != nil {
		t.Fatal(err)
	}
	prima := cursoriComeTesto(t, p)
	if c := cursore(t, p); c == nil || !c.Equal(cursoreFinale) {
		t.Fatalf("cursore dopo il sync = %v", c)
	}

	// «Riprova» accoda la rilettura, e il worker Outlook la prende
	var scartoID int64
	if err := p.QueryRow(ctx, `SELECT scarto_id FROM ingest_scarto`).Scan(&scartoID); err != nil {
		t.Fatal(err)
	}
	if msg, err := s.Riprova(ctx, scartoID); err != nil || !strings.Contains(msg, "rilettura accodata") {
		t.Fatalf("riprova: %q %v", msg, err)
	}
	preso, err := coda.Claim(ctx, q, db.WorkerTipoOutlook, "outlook@PC-PROVA",
		coda.Destinazione{Caselle: []uuid.UUID{casella.CasellaID}}, 2*time.Second)
	if err != nil || preso == nil || preso.Tipo != db.TipoJobRileggiElemento {
		t.Fatalf("claim della rilettura: %+v %v", preso, err)
	}

	// Il worker l'ha ritrovato per Message-ID in un'altra cartella: l'EntryID è nuovo. Il lotto porta anche
	// un cursore più avanti di quello del sync, come farebbe un worker scritto male: non deve contare.
	dopo := cursoreFinale.Add(3 * time.Hour)
	riletto := worker.MessaggioIn{
		MessageID: "<pp-saltato@acme.example>", EntryID: "ENTRY-PP-SPOSTATO", StoreID: "STORE-PP",
		ConversationID: "CONV-PP-S", Cartella: "Archivio ACME", Direzione: "entrata",
		DataEvento: quando, RicevutoIl: &dopo, MittenteNome: "Mario Rossi", MittenteIndirizzo: "mario.rossi@acme.example",
		Oggetto: "elemento non convertito", CorpoTesto: "Richiesta d'offerta per 7120001.",
		Riferimenti: []string{}, Categorie: []string{},
	}
	r, err := s.Ingerisci(ctx, Lotto{
		Casella:   casella,
		Tentativo: &Tentativo{JobID: preso.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "outlook@PC-PROVA"},
		Messaggi:  []worker.MessaggioIn{riletto},
		Cursore:   &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: dopo},
	})
	if err != nil {
		t.Fatalf("lotto della rilettura: %v", err)
	}
	if r.Inseriti != 1 || r.Falliti != 0 {
		t.Fatalf("il lotto da uno non è entrato: %+v", r)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 4 {
		t.Errorf("messaggi dopo la rilettura = %d, attesi 4", n)
	}
	// lo scarto si chiude anche se l'EntryID è cambiato: il job porta quello vecchio
	if sc := scarti(t, p); len(sc) != 0 {
		t.Errorf("lo scarto della rilettura è rimasto: %+v", sc)
	}
	// e i cursori sono quelli del sync, riga per riga: né quello della cartella del sync, né uno nuovo
	// per la cartella in cui l'elemento è stato ritrovato
	if dopoRilettura := cursoriComeTesto(t, p); dopoRilettura != prima {
		t.Errorf("la rilettura ha mosso i cursori:\nprima %s\ndopo  %s", prima, dopoRilettura)
	}
	if c := cursore(t, p); c == nil || !c.Equal(cursoreFinale) {
		t.Errorf("cursore dopo la rilettura = %v, atteso %v", c, cursoreFinale)
	}

	// Il controllo guarda il JOB, non il lotto: lo stesso cursore consegnato da un sync si scrive.
	sync := jobInCorso(t, p, db.TipoJobSyncOutlook, uuid.NullUUID{UUID: casella.CasellaID, Valid: true},
		worker.PayloadSyncOutlook{CasellaID: &casella.CasellaID})
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella, Tentativo: sync, Messaggi: []worker.MessaggioIn{},
		Cursore: &worker.CursoreLotto{Cartella: cartellaPP, UltimoReceived: dopo}}); err != nil {
		t.Fatal(err)
	}
	if c := cursore(t, p); c == nil || !c.Equal(dopo) {
		t.Errorf("il cursore di un sync non si è mosso: %v, atteso %v", c, dopo)
	}
}
