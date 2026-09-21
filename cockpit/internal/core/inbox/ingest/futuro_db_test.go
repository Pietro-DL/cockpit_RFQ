//go:build integrazione

// L4 — la guardia sul futuro (correzione del 16/09/2026).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/core/inbox/ingest/
//
// Il difetto vero stava nel worker: pywin32 consegna le date di Outlook con i numeri dell'ora locale
// e l'etichetta UTC, quindi ogni `ricevuto_il` arrivava due ore avanti (log del 16/09, 08:52). Il
// danno però non è stato il valore sbagliato in sé — è stato il CURSORE, che è avanzato con lui: da
// quel momento la finestra di lettura cominciava due ore dopo l'orologio e il sync non leggeva più
// niente, senza un errore da nessuna parte.
//
// Questi test non provano la correzione del worker (è in workers/test_ora_outlook.py). Provano che il
// server la smette di credere a un'ora impossibile anche se gliela manda un worker vecchio, un PC con
// l'orologio avanti o un replay di uno scarto di ieri: l'elemento non entra, e soprattutto il cursore
// non si muove. Un elemento scartato si rilegge; un cursore nel futuro non lo corregge nessuno.
package ingest

import (
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/platform/contratti/api"
)

// I due ore del 16/09: lo scarto esatto fra l'ora di Roma e l'UTC in settembre.
const dueOre = 2 * time.Hour

func TestUnMessaggioNelFuturoNonEntraEIlCursoreNonAvanza(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	futuro := time.Now().Add(dueOre).UTC().Truncate(time.Second)

	// Il secondo elemento arriva con l'ora sbagliata, e con lui il cursore del lotto: è così che si
	// presenta il difetto: il worker calcola il cursore sulle stesse date dei messaggi.
	msg := tre(func(m *api.MessaggioIn) { m.DataEvento = futuro; m.RicevutoIl = &futuro })
	res, err := s.Ingerisci(ctx, Lotto{
		Casella: casella, Messaggi: msg,
		Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: futuro},
	})
	if err != nil {
		t.Fatalf("il lotto deve andare a buon fine: l'elemento impossibile è uno solo: %v", err)
	}
	if res.Inseriti != 2 || res.Falliti != 1 {
		t.Fatalf("inseriti=%d falliti=%d, attesi 2 e 1: gli altri due elementi non c'entrano niente", res.Inseriti, res.Falliti)
	}

	sc := scarti(t, p)
	if len(sc) != 1 {
		t.Fatalf("%d scarti, atteso 1", len(sc))
	}
	if sc[0].EntryID != "ENTRY-PP-2" || sc[0].Origine != "ingest" {
		t.Fatalf("scarto sbagliato: %s / %s", sc[0].EntryID, sc[0].Origine)
	}
	// Il motivo deve poter essere letto in /admin/scarti da chi non sa niente di pywin32.
	for _, atteso := range []string{"nel futuro", "outlook_com._utc"} {
		if !strings.Contains(sc[0].Errore, atteso) {
			t.Errorf("il motivo dello scarto non dice %q: %s", atteso, sc[0].Errore)
		}
	}
	if len(sc[0].Payload) == 0 {
		t.Error("lo scarto è senza payload: non sarebbe rileggibile quando l'ora torna giusta")
	}

	// La parte che conta.
	if cur := cursore(t, p); cur != nil {
		t.Fatalf("il cursore è avanzato a %v: la prossima finestra comincerebbe nel futuro e il sync smetterebbe di leggere", cur)
	}
}

func TestUnOrologioAvantiDiDueMinutiNonEUnProblema(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	// Fra il PC del worker e il server qualche minuto di differenza è normale, e non è un difetto:
	// la tolleranza esiste per non trasformare due orologi diversi in posta scartata.
	poco := time.Now().Add(2 * time.Minute).UTC().Truncate(time.Second)

	msg := tre(func(m *api.MessaggioIn) { m.DataEvento = poco; m.RicevutoIl = &poco })
	res, err := s.Ingerisci(ctx, Lotto{
		Casella: casella, Messaggi: msg,
		Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: poco},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Inseriti != 3 || res.Falliti != 0 {
		t.Fatalf("inseriti=%d falliti=%d, attesi 3 e 0: due minuti non sono un'ora impossibile", res.Inseriti, res.Falliti)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(poco) {
		t.Fatalf("il cursore è %v, atteso %v", cur, poco)
	}
}

func TestIlCursoreNelFuturoNonSiScriveNemmenoConIMessaggiInRegola(t *testing.T) {
	// L'altra metà della guardia, isolata: gli elementi sono a posto e il cursore no. Può succedere
	// con un worker che legge le date in un modo e calcola il cursore in un altro, ed è il caso in cui
	// non ci sarebbe nessuno scarto a segnalare niente.
	p, s, casella, ctx := preparaPP(t)
	futuro := time.Now().Add(dueOre).UTC().Truncate(time.Second)

	res, err := s.Ingerisci(ctx, Lotto{
		Casella: casella, Messaggi: tre(nil),
		Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: futuro},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Inseriti != 3 || res.Falliti != 0 {
		t.Fatalf("inseriti=%d falliti=%d: i messaggi erano in regola", res.Inseriti, res.Falliti)
	}
	if cur := cursore(t, p); cur != nil {
		t.Fatalf("il cursore è avanzato a %v: doveva restare fermo", cur)
	}

	// E il lotto successivo, con un cursore plausibile, lo scrive regolarmente: la guardia ferma il
	// valore impossibile, non la casella.
	if _, err := s.Ingerisci(ctx, Lotto{
		Casella: casella, Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	}); err != nil {
		t.Fatal(err)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
		t.Fatalf("il cursore è %v, atteso %v", cur, cursoreFinale)
	}
}
