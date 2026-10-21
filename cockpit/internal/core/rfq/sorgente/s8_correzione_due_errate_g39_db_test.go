//go:build integrazione

// L4 — giro G39, rilievo bloccante della revisione indipendente 3 della corsia interfaccia (G39-03 [U]: dopo R5 il cambio
// di sorgente e' una revisione interna, mai del cliente). La correzione della sorgente vale per tutta la revisione in
// corso: dopo la revoca dello STEP sbagliato con cui la BOM era confermata, un'altra sorgente sbagliata revocata o
// spostata prima della sorgente giusta non la fa perdere. E vale una volta sola: dopo la R5 della correzione, la sorgente
// giusta non corregge piu' nulla, e la revisione che segue e' del cliente. Dati sintetici della scena di s8_db_test.go.
// TestV1G39CorrezioneConPiuSorgentiErrateNellaRevisione e TestV1G39LaCorrezioneValeUnaVolta falliscono sul codice di
// a582168 (confermabom.SorgenteValida guardava solo l'ultima revoca prima della S8, e non la base della revisione).

package sorgente

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// revocaErrata: la revoca della sorgente con la causa sorgente_errata su ACM-1000.
func revocaErrata(t *testing.T, p *pgxpool.Pool, s scenaS8, allegato uuid.UUID, motivo string) EsitoRevoca {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": motivo, "sorgente_errata": true})
	r, imp, err := LeggiRichiestaRevoca(raw)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := Revoca(context.Background(), p, s.thread, allegato, s.utente, r, imp)
	if err != nil {
		t.Fatalf("revoca: %+v %v", rv, err)
	}
	return rv
}

// laCorrezione: R4 e R5 della bozza segnate come correzione della sorgente (numero della conferma atteso n).
func laCorrezione(t *testing.T, p *pgxpool.Pool, s scenaS8, es EsitoS8, n int) distintabozza.ConfermaR5 {
	t.Helper()
	rp := riepilogoRev(t, p, s, es.Bozza.BozzaID, es.Bozza.Versione)
	if len(rp.Blocchi) != 0 || !rp.Confermabile || !rp.Sorgente.Correzione || !rp.EffettoSullaFonte.CorrezioneSorgente {
		t.Fatalf("R4 della correzione: %v %+v %+v", codiciR4(rp), rp.Sorgente, rp.EffettoSullaFonte)
	}
	c, err := confermaRev(t, p, s, es.Bozza.BozzaID, es.Bozza.Versione, rp.FirmaAnteprima)
	if err != nil || c.NumeroConferma != n || !c.Fonte.CorrezioneSorgente {
		t.Fatalf("R5 della correzione: %+v %v", c, err)
	}
	if k := conta(t, p, `SELECT count(*) FROM conferma_strutturale WHERE conferma_id = $1 AND origine->>'correzione_sorgente' = 'true'
		AND origine->>'revisione' = 'interna'`, c.ConfermaID); k != 1 {
		t.Fatalf("la conferma n. %d non e' segnata come correzione della sorgente", n)
	}
	return c
}

// La BOM n. 1 confermata con lo STEP sbagliato A; revoca di A con la causa; S8 di B, sbagliato anche lui, che si toglie
// prima di R5 (revoca con la causa, oppure il gesto «sposta la sorgente» di ACM-6000); S8 di C, la sorgente giusta. La
// conferma n. 2 e' la correzione della sorgente (prima si perdeva: contava solo la revoca di B, mai in una conferma).
func TestV1G39CorrezioneConPiuSorgentiErrateNellaRevisione(t *testing.T) {
	for _, via := range []string{"due_revoche", "revoca_e_sposta"} {
		t.Run(via, func(t *testing.T) {
			p, s := preparaS8(t)
			ctx := context.Background()
			primaConferma(t, p, s)
			if rv := revocaErrata(t, p, s, s.stepA, "la BOM e' stata confermata con lo STEP sbagliato"); !rv.UsataDaConferma {
				t.Fatalf("revoca di A: %+v", rv)
			}
			esB := revisioneConStep(t, p, s, s.stepB, s.shaB, "#5", "#6", map[string]any{"motivo": "la sorgente giusta, si crede"})
			if via == "due_revoche" {
				rv := revocaErrata(t, p, s, s.stepB, "anche B e' sbagliato")
				if rv.UsataDaConferma || rv.BozzaAbbandonata == nil || *rv.BozzaAbbandonata != esB.Bozza.BozzaID {
					t.Fatalf("revoca di B prima di R5: %+v", rv)
				}
			} else {
				regQ := registroPer(t, registroRadice(t, s, s.stepB, s.shaB, "ACM-1000"), s.altroFinito)
				es, err := s8(t, p, s, s.stepB, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "lo STEP B e' di ACM-6000", "sposta_da": s.prodotto}),
					Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}})
				if err != nil || es.Spostamento == nil || es.Spostamento.BozzaAbbandonata == nil || *es.Spostamento.BozzaAbbandonata != esB.Bozza.BozzaID {
					t.Fatalf("B spostato su ACM-6000: %+v %v", es, err)
				}
			}
			esC := revisioneConStep(t, p, s, s.stepC, s.shaC, "#7", "#8", map[string]any{"motivo": "la sorgente giusta"})
			if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaC); err != nil || !st.Valida || !st.Correzione || st.CorrezioneDichiarata {
				t.Fatalf("la sorgente C: %+v %v", st, err)
			}
			laCorrezione(t, p, s, esC, 2)
			// la correzione l'ha fatta la R5 n. 2: C non corregge piu' nulla
			if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaC); err != nil || !st.Valida || st.Correzione {
				t.Fatalf("la sorgente C dopo la R5 della correzione: %+v %v", st, err)
			}
		})
	}
}

// La correzione vale una volta: BOM n. 1 con A (sbagliato), revoca con la causa (o S8 con correzione_sorgente), S8 di B,
// R5 n. 2 (correzione). Poi la revisione del cliente con lo STEP C: la conferma n. 3 non e' segnata. Sul codice di
// a582168, dopo la R5 n. 2 la sorgente B restava «correzione» (la revoca di A o il motivo dichiarato, di una revisione
// gia' chiusa, contavano ancora).
func TestV1G39LaCorrezioneValeUnaVolta(t *testing.T) {
	for _, via := range []string{"revoca", "corpo"} {
		t.Run(via, func(t *testing.T) {
			p, s := preparaS8(t)
			ctx := context.Background()
			primaConferma(t, p, s)
			extra := map[string]any{"motivo": "lo STEP giusto"}
			if via == "revoca" {
				revocaErrata(t, p, s, s.stepA, "confermato lo STEP sbagliato")
			} else {
				extra["correzione_sorgente"] = true
			}
			esB := revisioneConStep(t, p, s, s.stepB, s.shaB, "#5", "#6", extra)
			laCorrezione(t, p, s, esB, 2)
			if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaB); err != nil || !st.Valida || st.Correzione {
				t.Fatalf("la sorgente B dopo la R5 della correzione: %+v %v", st, err)
			}
			esC := revisioneConStep(t, p, s, s.stepC, s.shaC, "#7", "#8", map[string]any{"motivo": "revisione 3 del cliente"})
			if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaC); err != nil || !st.Valida || st.Correzione {
				t.Fatalf("la sorgente C della revisione del cliente: %+v %v", st, err)
			}
			rp := riepilogoRev(t, p, s, esC.Bozza.BozzaID, esC.Bozza.Versione)
			if len(rp.Blocchi) != 0 || rp.Sorgente.Correzione || rp.EffettoSullaFonte.CorrezioneSorgente {
				t.Fatalf("R4 della revisione 3: %v %+v %+v", codiciR4(rp), rp.Sorgente, rp.EffettoSullaFonte)
			}
			c3, err := confermaRev(t, p, s, esC.Bozza.BozzaID, esC.Bozza.Versione, rp.FirmaAnteprima)
			if err != nil || c3.NumeroConferma != 3 || c3.Fonte.CorrezioneSorgente {
				t.Fatalf("R5 della revisione 3: %+v %v", c3, err)
			}
			st, err := distintabozza.LeggiStorico(ctx, db.New(p), s.thread, s.prodotto, distintabozza.OpzioniR7{})
			if err != nil {
				t.Fatal(err)
			}
			segnate := map[int]bool{}
			for _, c := range st.Conferme {
				segnate[c.Numero] = c.CorrezioneSorgente
			}
			if segnate[1] || !segnate[2] || segnate[3] {
				t.Fatalf("R7: %v", segnate)
			}
		})
	}
}
