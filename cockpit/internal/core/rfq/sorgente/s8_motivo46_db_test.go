//go:build integrazione

// L4 — messaggio 46 [U] (21/10), corsia MOTIVO: la conferma della sorgente (S8) con il codice letto dalla radice dello STEP
// che non e' quello del prodotto, SENZA il motivo dell'utente. Prima: 422 MOTIVO_OBBLIGATORIO al passo 0, niente scritto.
// Adesso: S8 completa; la riga conferma_documento di S8 e la scelta della fonte della bozza (fonte_step_evento «scelta» e
// l'evento sorgente_scelta) hanno come motivo la traccia del gesto scritta dal server; il reinvio dello stesso corpo e'
// riproposto. Dati inventati (ACME).

package sorgente

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1M46S8LaRadiceDiversaSenzaMotivo(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regX := registroRadice(t, s, s.stepA, s.shaA, "ACM-1999")
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regX}}
	chiave := uuid.New()
	corpo := corpoStep(t, s, chiave, regX, nil)
	es, err := s8(t, p, s, s.stepA, corpo, prep)
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil {
		t.Fatalf("S8 con la radice diversa, senza motivo: %+v %v", es, err)
	}
	atteso := "gesto di S8P da «Conferma la sorgente»: lo STEP «ACM-1000.stp» è la sorgente di ACM-1000; codice della radice dello STEP diverso («ACM-1999»)"
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'conferma_documento' AND motivo = $2`, s.prodotto, atteso); n != 1 {
		t.Fatalf("la riga di S8 con la traccia del gesto: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE bozza_id = $1 AND azione = 'scelta' AND motivo = $2`, es.Bozza.BozzaID, atteso); n != 1 {
		t.Fatalf("la scelta della fonte con la traccia del gesto: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_scelta' AND motivo = $2`, es.Bozza.BozzaID, atteso); n != 1 {
		t.Fatalf("l'evento della scelta della fonte con la traccia del gesto: %d", n)
	}
	// il reinvio dello stesso corpo con la stessa chiave: riproposto, niente di nuovo
	righe := conta(t, p, `SELECT count(*) FROM fonte_step_evento`)
	es2, err := s8(t, p, s, s.stepA, corpo, prep)
	if err != nil || !es2.Riproposto || es2.Esito != EsitoCompletata {
		t.Fatalf("reinvio: %+v %v", es2, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento`); n != righe {
		t.Fatalf("il reinvio ha scritto: %d -> %d", righe, n)
	}
	// la stessa chiave con il motivo di un client di prima e' un altro corpo
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, chiave, regX, map[string]any{"motivo": "scritto"}), prep)
	codiceDi(t, err, distintabozza.CodiceChiaveRiusata, "")
	_ = ctx
}
