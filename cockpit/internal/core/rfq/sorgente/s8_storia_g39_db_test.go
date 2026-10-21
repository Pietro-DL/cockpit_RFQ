//go:build integrazione

// L4 — giro G39, rilievo importante della revisione indipendente della corsia persistenza-sorgente. Lo STEP confermato per
// errore nella BOM di ACM-1000 (R5) e' in realta' di ACM-6000. Il rimedio [U] e' «apri la revisione di ACM-1000»; dopo
// quella revisione lo STEP resta nella storia di ACM-1000 come documento sostituito e, senza un cambio di schema (documento e'
// UNIQUE (thread_id, sha256)), non diventa la sorgente di ACM-6000: aperto, domanda all'utente. Qui si prova che il server lo
// dice senza promettere strade che non funzionano: il rifiuto bom_confermata non promette lo spostamento dopo la
// revisione, e S8 su ACM-6000 dopo la revisione risponde GIA_DOCUMENTO motivo sostituito_in_altro_prodotto con
// altro_prodotto, senza «annulla la sostituzione» e senza scritture. Fallisce sul codice di 36446c9 (motivo vuoto, il
// testo proponeva di annullare la sostituzione). Dati sintetici della scena di s8_db_test.go.

package sorgente

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1G39DopoLaRevisioneLoStepRestaNellaStoriaDelProdotto(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA, _, docA := primaConferma(t, p, s)
	regQ := registroPer(t, regA, s.altroFinito)
	prepQ := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}}
	// prima della revisione: il rifiuto bom_confermata dice il rimedio e il suo limite
	_, err := s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "lo STEP e' di ACM-6000"}), prepQ)
	e := codiceDi(t, err, CodiceStepSorgenteDiAltroProdotto, MotivoBomConfermata)
	if !strings.Contains(e.Messaggio, "Apri la revisione di ACM-1000") || !strings.Contains(e.Messaggio, "resta nella storia di ACM-1000") {
		t.Fatalf("il testo del rifiuto bom_confermata: %q", e.Messaggio)
	}
	// la revisione di ACM-1000: revoca sorgente_errata dello STEP A, S8 dello STEP giusto B, R4, R5 (correzione della sorgente)
	raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": "lo STEP e' di ACM-6000", "sorgente_errata": true})
	r, imp, err := LeggiRichiestaRevoca(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rv, err := Revoca(ctx, p, s.thread, s.stepA, s.utente, r, imp); err != nil || !rv.UsataDaConferma {
		t.Fatalf("la revoca dopo R5: %+v %v", rv, err)
	}
	esB := revisioneConStep(t, p, s, s.stepB, s.shaB, "#5", "#6", map[string]any{"motivo": "lo STEP giusto"})
	rp := riepilogoRev(t, p, s, esB.Bozza.BozzaID, esB.Bozza.Versione)
	if !rp.EffettoSullaFonte.CorrezioneSorgente {
		t.Fatalf("la revisione dopo la revoca di uno STEP della BOM e' una correzione: %+v", rp.EffettoSullaFonte)
	}
	if c2, err := confermaRev(t, p, s, esB.Bozza.BozzaID, esB.Bozza.Versione, rp.FirmaAnteprima); err != nil || !c2.Fonte.CorrezioneSorgente {
		t.Fatalf("R5 della correzione: %+v %v", c2, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE documento_id = $1 AND componente_id = $2 AND sostituito_da IS NOT NULL`, docA, s.prodotto); n != 1 {
		t.Fatal("lo STEP A resta nella storia di ACM-1000, sostituito")
	}
	// dopo la revisione: S8 dello STEP A su ACM-6000 (con il motivo, e con sposta_da) dice il limite, senza scritture
	prima := fotoRev(t, p)
	for _, extra := range []map[string]any{{"motivo": "lo STEP e' di ACM-6000"}, {"motivo": "lo STEP e' di ACM-6000", "sposta_da": s.prodotto}} {
		_, err := s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, extra), prepQ)
		e := codiceDi(t, err, CodiceGiaDocumento, MotivoSostituitoInAltroProdotto)
		if e.AltroProdotto == nil || *e.AltroProdotto != s.prodotto || !strings.Contains(e.Messaggio, "nella storia della BOM di ACM-1000") ||
			strings.Contains(strings.ToLower(e.Messaggio), "annulla") {
			t.Fatalf("il rifiuto dopo la revisione (%v): %+v", extra, e)
		}
	}
	dopo := fotoRev(t, p)
	for _, tab := range tabelleRev {
		if dopo[tab] != prima[tab] {
			t.Fatalf("il rifiuto ha scritto in %s", tab)
		}
	}
}
