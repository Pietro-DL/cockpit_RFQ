//go:build integrazione

// L4 — giro G39-ter: SostituzioneDellaBozza, la regola della sostituzione della bozza (sostituzione.go) che l'anteprima S6
// con ?bozza= espone in effetto_sulla_bozza.sostituzione, perche' il dialogo di S8 dica prima che cosa succede: con lo
// STEP di un'altra sorgente S8 la bozza nata prima di R5 si sostituisce (con le modifiche dell'utente dichiarate); con la
// stessa sorgente no (la bozza manuale e la bozza nata da una conferma restano alle prove della riconciliazione e della
// revisione, che non cambiano). Sola lettura. Dati
// sintetici: la scena di s8_db_test.go.

package sorgente

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// anteprimaSostituzione: SostituzioneDellaBozza in una transazione di sola lettura (niente scritto: i conteggi prima e dopo).
func anteprimaSostituzione(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, sha string) *AnteprimaSostituzione {
	t.Helper()
	ctx := context.Background()
	c0 := contiG39(t, p)
	defer func() {
		if c1 := contiG39(t, p); c1 != c0 {
			t.Errorf("l'anteprima ha scritto: %s -> %s", c0, c1)
		}
	}()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // sola lettura
	q := db.New(tx)
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: bozza, ThreadID: s.thread})
	if err != nil {
		t.Fatal(err)
	}
	x, err := SostituzioneDellaBozza(ctx, q, s.thread, b, sha)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestV1G39terAnteprimaDellaSostituzione(t *testing.T) {
	p, s := preparaS8(t)
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Bozza == nil {
		t.Fatalf("S8 di B: %+v %v", esB, err)
	}
	if x := anteprimaSostituzione(t, p, s, esB.Bozza.BozzaID, s.shaA); x == nil || x.BozzaID != esB.Bozza.BozzaID || x.Versione != 1 ||
		x.AllegatoPrecedente != s.stepB || x.NomeFile != "ACM-1000_revB.stp" || x.ModificheUtente || !strings.Contains(x.Testo, "annullata") {
		t.Fatalf("la sostituzione con lo STEP A: %+v", x)
	}
	if x := anteprimaSostituzione(t, p, s, esB.Bozza.BozzaID, s.shaB); x != nil {
		t.Fatalf("la stessa sorgente non sostituisce: %+v", x)
	}
	// con una modifica dell'utente lo dice (la presa d'atto)
	radice := radiceDellaBozza(t, p, esB.Bozza.BozzaID, 1, s.prodotto)
	if _, err := aggiungiPBG39(t, p, s, esB.Bozza.BozzaID, 1, s.pdf, radice, "", nil); err != nil {
		t.Fatalf("la modifica dell'utente: %v", err)
	}
	if x := anteprimaSostituzione(t, p, s, esB.Bozza.BozzaID, s.shaA); x == nil || !x.ModificheUtente || x.Versione != 2 || !strings.Contains(x.Testo, "presa d'atto") {
		t.Fatalf("la sostituzione della bozza modificata: %+v", x)
	}
}
