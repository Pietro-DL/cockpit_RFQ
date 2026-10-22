//go:build integrazione

// L4 — messaggio 46 dell'utente (21/10), corsia SORGENTE, rilievo minore 4 della revisione: i testi della revoca che il
// server scrive (EsitoRevoca.Testo, mostrati così come sono nella striscia della pagina) parlano all'ingegnere, senza
// «working» e senza i nomi delle rotte («(S8)», R5). La prova nel browser (DU-B72) legge il testo del banco, che è una
// copia: questa legge quello di Revoca nei suoi tre casi (bozza annullata, nessuna bozza aperta, STEP già nella BOM
// confermata). Fallisce sul codice di 342010f. Dati sintetici della scena di s8_db_test.go (ACME, ACM-1000).

package sorgente

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

// gergoRevoca: le parole che non devono arrivare all'ingegnere nel testo della revoca
var gergoRevoca = regexp.MustCompile(`(?i)\bworking\b|\bS\d{1,2}\b|\bR\d{1,2}\b`)

func revocaPerTesto(t *testing.T, ctx context.Context, p distintabozza.Avviatore, s scenaS8) EsitoRevoca {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": "prova dei testi"})
	r, imp, err := LeggiRichiestaRevoca(raw)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := Revoca(ctx, p, s.thread, s.stepA, s.utente, r, imp)
	if err != nil {
		t.Fatalf("revoca: %v", err)
	}
	return rv
}

func controllaTestoRevoca(t *testing.T, caso string, rv EsitoRevoca) {
	t.Helper()
	if !strings.HasPrefix(rv.Testo, "Sorgente revocata per ACM-1000") {
		t.Errorf("%s: il testo non dice la revoca: %q", caso, rv.Testo)
	}
	if g := gergoRevoca.FindAllString(rv.Testo, -1); len(g) > 0 {
		t.Errorf("%s: il testo della revoca ha il gergo %v: %q", caso, g, rv.Testo)
	}
}

func TestV1UI46TestiDellaRevocaSenzaGergo(t *testing.T) {
	ctx := context.Background()
	t.Run("bozza annullata", func(t *testing.T) {
		p, s := preparaS8(t)
		regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
		if es, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}); err != nil || es.Esito != EsitoCompletata {
			t.Fatalf("S8: %+v %v", es, err)
		}
		rv := revocaPerTesto(t, ctx, p, s)
		if rv.BozzaAbbandonata == nil {
			t.Fatalf("la revoca non ha annullato la bozza: %+v", rv)
		}
		controllaTestoRevoca(t, "bozza annullata", rv)
	})
	t.Run("nessuna bozza aperta", func(t *testing.T) {
		p, s := preparaS8(t)
		regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
		if es, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}); err != nil || es.Esito != EsitoCompletata {
			t.Fatalf("S8: %+v %v", es, err)
		}
		abbandona(t, p, s)
		rv := revocaPerTesto(t, ctx, p, s)
		if rv.BozzaID != nil || rv.UsataDaConferma {
			t.Fatalf("la revoca non è nel caso senza bozza: %+v", rv)
		}
		controllaTestoRevoca(t, "nessuna bozza aperta", rv)
	})
	t.Run("nella BOM confermata", func(t *testing.T) {
		p, s := preparaS8(t)
		primaConferma(t, p, s)
		rv := revocaPerTesto(t, ctx, p, s)
		if !rv.UsataDaConferma {
			t.Fatalf("la revoca non è nel caso della BOM confermata: %+v", rv)
		}
		controllaTestoRevoca(t, "nella BOM confermata", rv)
	})
}
