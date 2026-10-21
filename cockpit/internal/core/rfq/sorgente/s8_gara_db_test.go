//go:build integrazione

// L4 — la gara fra due S8 sulla stessa RFQ (direttiva 39 §2, §3; blocco «R4 completo» della corsia persistenza, punto 7),
// sul database di prova: le due procedure partono insieme mentre una terza transazione tiene il lucchetto della RFQ, e si
// contendono BloccaThread quando lo lascia. Che cosa vale in ogni ordine:
//   - mai due bozze aperte per lo stesso prodotto;
//   - S8 non tocca working, STEP strutturale, sostituzioni, conferme, AR, righe legacy (fotoVietate invariata);
//   - stesso STEP con due chiavi: tutte e due completate, un documento, una sola conferma di S8 (la seconda e' «gia' fatto»),
//     una sola copia sul NAS in coda, la sorgente valida;
//   - STEP A e STEP B (due pagine): una crea la bozza sulla sua sorgente; l'altra non apre una seconda bozza (rifiuto
//     BOZZA_VERSIONE altra_bozza_aperta prima del passo 2, oppure esito parziale con il passo 3 rifiutato); la sorgente della
//     bozza aperta e' una delle due, e il gate di R5 la vede valida oppure superata (mai una conferma senza S8 valida).
// Dati sintetici.

package sorgente

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// esitoGara: l'esito di una delle due S8.
type esitoGara struct {
	es  EsitoS8
	err error
}

// gara: due S8 (allegato e corpo) partite insieme dietro il lucchetto della RFQ tenuto da una terza transazione.
func gara(t *testing.T, p *pgxpool.Pool, s scenaS8, a1 uuid.UUID, c1 []byte, prep1 Preparazione, a2 uuid.UUID, c2 []byte, prep2 Preparazione) (esitoGara, esitoGara) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(tx).BloccaThread(ctx, s.thread); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var e1, e2 esitoGara
	parti := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-parti
		e1.es, e1.err = s8(t, p, s, a1, c1, prep1)
	}()
	go func() {
		defer wg.Done()
		<-parti
		e2.es, e2.err = s8(t, p, s, a2, c2, prep2)
	}()
	close(parti)
	time.Sleep(500 * time.Millisecond) // tutte e due arrivano al lucchetto della RFQ
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	return e1, e2
}

func bozzeAperte(t *testing.T, p *pgxpool.Pool, s scenaS8) int {
	t.Helper()
	return conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto)
}

func TestV1D39S8GaraSulloStessoStep(t *testing.T) {
	p, s := preparaS8(t)
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	prima := fotoVietate(t, p)
	e1, e2 := gara(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prep, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prep)
	for i, e := range []esitoGara{e1, e2} {
		if e.err != nil || e.es.Esito != EsitoCompletata || e.es.Bozza == nil {
			t.Fatalf("S8 %d: %+v %v", i+1, e.es, e.err)
		}
	}
	if e1.es.Bozza.BozzaID != e2.es.Bozza.BozzaID || bozzeAperte(t, p, s) != 1 {
		t.Fatalf("una sola bozza: %v %v (aperte %d)", e1.es.Bozza.BozzaID, e2.es.Bozza.BozzaID, bozzeAperte(t, p, s))
	}
	gia := 0
	for _, e := range []esitoGara{e1, e2} {
		if e.es.Passi[1].Esito == EsitoGiaFatto {
			gia++
		}
	}
	if gia != 1 {
		t.Fatalf("il passo 2 «gia' fatto» esattamente in una delle due: %+v / %+v", e1.es.Passi, e2.es.Passi)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE thread_id = $1 AND sha256 = $2`, s.thread, s.shaA); n != 1 {
		t.Fatalf("documenti dello STEP A: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'conferma_documento' AND chiave_idempotenza IS NOT NULL AND sha256 = $1`, s.shaA); n != 1 {
		t.Fatalf("conferme di S8: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 1 {
		t.Fatalf("copie in coda: %d", n)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("la gara ha toccato la working: %s -> %s", prima, dopo)
	}
	if sv, err := confermabom.SorgenteValida(context.Background(), db.New(p), s.thread, s.prodotto, s.shaA); err != nil || !sv.Valida {
		t.Fatalf("sorgente valida: %+v %v", sv, err)
	}
}

func TestV1D39S8GaraFraDueStep(t *testing.T) {
	p, s := preparaS8(t)
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	prima := fotoVietate(t, p)
	e1, e2 := gara(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}},
		s.stepB, corpoStep(t, s, uuid.New(), regB, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if bozzeAperte(t, p, s) != 1 {
		t.Fatalf("bozze aperte: %d (mai due)", bozzeAperte(t, p, s))
	}
	vinte := 0
	for i, e := range []esitoGara{e1, e2} {
		var de *distintabozza.Errore
		switch {
		case e.err == nil && e.es.Esito == EsitoCompletata && e.es.Bozza != nil:
			vinte++
		case e.err == nil && e.es.Esito == EsitoParziale:
			t.Logf("S8 %d parziale (passo 3 rifiutato): %+v", i+1, e.es.Passi)
		case errors.As(e.err, &de) && de.Codice == distintabozza.CodiceBozzaVersione && de.Motivo == distintabozza.MotivoAltraBozzaAperta:
			t.Logf("S8 %d rifiutata prima del passo 2: %s/%s", i+1, de.Codice, de.Motivo)
		default:
			t.Fatalf("S8 %d: esito inatteso %+v %v", i+1, e.es, e.err)
		}
	}
	if vinte != 1 {
		t.Fatalf("una sola S8 crea la bozza: %d (%+v %v / %+v %v)", vinte, e1.es, e1.err, e2.es, e2.err)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("la gara ha toccato la working: %s -> %s", prima, dopo)
	}
	// la sorgente della bozza aperta e' una delle due; il gate di R5 la vede valida o superata, e la sorgente valida del
	// prodotto e' al piu' una
	var sha string
	if err := p.QueryRow(context.Background(), `SELECT fonte_sha256 FROM distinta_bozza WHERE prodotto_id = $1 AND stato = 'aperta'`, s.prodotto).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	if sha != s.shaA && sha != s.shaB {
		t.Fatalf("la sorgente della bozza: %s", sha)
	}
	valide := 0
	for _, x := range []string{s.shaA, s.shaB} {
		sv, err := confermabom.SorgenteValida(context.Background(), db.New(p), s.thread, s.prodotto, x)
		if err != nil {
			t.Fatal(err)
		}
		if sv.Valida {
			valide++
		}
		if x == sha && !sv.Valida && sv.Motivo != confermabom.SorgenteSuperata {
			t.Fatalf("la sorgente della bozza non e' valida per un motivo inatteso: %+v", sv)
		}
	}
	if valide != 1 {
		t.Fatalf("sorgenti valide del prodotto: %d (attesa una)", valide)
	}
}
