//go:build integrazione

// L4 — giro G39, rilievo bloccante della revisione indipendente della corsia persistenza-sorgente (G39-03 [U]: dopo R5 il
// cambio di sorgente e' una revisione interna, distinta da una revisione del cliente). La «correzione della sorgente» si
// segna solo se la BOM del prodotto era stata confermata con lo STEP revocato (confermabom.UsataDaConferma) o se il corpo di
// S8 la dichiara (correzione_sorgente). Una sorgente sbagliata revocata o spostata prima di R5, e il gesto «sposta la
// sorgente» da solo, non bastano: la revisione che segue resta del cliente. Dati sintetici della scena di s8_db_test.go.
// Le tre prove falliscono sul codice di 36446c9 (R4 rispondeva correzione_sorgente=true).

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

// staffaDi: il componente ACM-1150 nato con la conferma n. 1 (primaConferma).
func staffaDi(t *testing.T, p *pgxpool.Pool, s scenaS8) uuid.UUID {
	t.Helper()
	var staffa uuid.UUID
	if err := p.QueryRow(context.Background(), `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = 'ACM-1150'`, s.thread).Scan(&staffa); err != nil {
		t.Fatal(err)
	}
	return staffa
}

// revisioneConStep: S8 dello STEP dato su ACM-1000 come revisione (sostituisci), con la stessa struttura della conferma n. 1;
// extra si aggiunge al corpo.
func revisioneConStep(t *testing.T, p *pgxpool.Pool, s scenaS8, allegato uuid.UUID, sha string, k1, k2 string, extra map[string]any) EsitoS8 {
	t.Helper()
	figlio, staffa := s.figlio, staffaDi(t, p, s)
	reg := registroConFigli(t, s, allegato, sha, []figlioProva{{chiave: k1, codice: "ACM-1100", componente: &figlio, qta: 1},
		{chiave: k2, codice: "ACM-1150", componente: &staffa, qta: 4}})
	fattiRevisione(t, p, sha, "#1", k1, k2)
	corpo := map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "revisione del cliente"}
	for k, v := range extra {
		corpo[k] = v
	}
	es, err := s8(t, p, s, allegato, corpoStep(t, s, uuid.New(), reg, corpo), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: reg}})
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil {
		t.Fatalf("S8 della revisione: %+v %v", es, err)
	}
	return es
}

// revisioneDelCliente: R4 e R5 della revisione non devono essere segnate come correzione della sorgente (ne' nella firma,
// ne' in origine e delta della conferma, ne' in R7).
func revisioneDelCliente(t *testing.T, p *pgxpool.Pool, s scenaS8, es EsitoS8) {
	t.Helper()
	ctx := context.Background()
	rp := riepilogoRev(t, p, s, es.Bozza.BozzaID, es.Bozza.Versione)
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4 della revisione: %v", codiciR4(rp))
	}
	if rp.Sorgente.Correzione || rp.EffettoSullaFonte.CorrezioneSorgente {
		t.Fatalf("una revisione del cliente segnata come correzione della sorgente: sorgente %+v, effetto %+v", rp.Sorgente, rp.EffettoSullaFonte)
	}
	c2, err := confermaRev(t, p, s, es.Bozza.BozzaID, es.Bozza.Versione, rp.FirmaAnteprima)
	if err != nil || c2.NumeroConferma != 2 || c2.Fonte.CorrezioneSorgente {
		t.Fatalf("R5 della revisione: %+v %v", c2, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM conferma_strutturale WHERE conferma_id = $1 AND (origine ? 'correzione_sorgente' OR origine ? 'revisione'
		OR delta->'effetto_sulla_fonte' ? 'correzione_sorgente')`, c2.ConfermaID); n != 0 {
		t.Fatal("la conferma della revisione del cliente porta la marcatura della correzione")
	}
	st, err := distintabozza.LeggiStorico(ctx, db.New(p), s.thread, s.prodotto, distintabozza.OpzioniR7{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range st.Conferme {
		if c.CorrezioneSorgente || c.Revisione != "" {
			t.Fatalf("R7: la conferma n. %d e' segnata (%v, %q)", c.Numero, c.CorrezioneSorgente, c.Revisione)
		}
	}
}

// Il caso della revisione indipendente: la conferma n. 1 con lo STEP giusto A; la revisione del cliente parte per errore
// dallo STEP B, che si revoca con sorgente_errata prima di R5 (la bozza si annulla); poi S8 dello STEP C, la revisione
// vera. B non e' mai entrato in una conferma: la conferma n. 2 e' una revisione del cliente.
func TestV1G39RevisioneDelClienteDopoUnaSorgenteErrataRevocataPrimaDiR5(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	primaConferma(t, p, s)
	esB := revisioneConStep(t, p, s, s.stepB, s.shaB, "#5", "#6", nil)
	raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "motivo": "lo STEP B non e' la revisione giusta", "sorgente_errata": true})
	r, imp, err := LeggiRichiestaRevoca(raw)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := Revoca(ctx, p, s.thread, s.stepB, s.utente, r, imp)
	if err != nil || rv.UsataDaConferma || rv.BozzaAbbandonata == nil || *rv.BozzaAbbandonata != esB.Bozza.BozzaID {
		t.Fatalf("la revoca di B prima di R5: %+v %v", rv, err)
	}
	esC := revisioneConStep(t, p, s, s.stepC, s.shaC, "#7", "#8", nil)
	if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaC); err != nil || !st.Valida || st.Correzione || st.CorrezioneDichiarata {
		t.Fatalf("la sorgente C: %+v %v", st, err)
	}
	revisioneDelCliente(t, p, s, esC)
}

// Il gesto «sposta la sorgente» durante una revisione, nei due versi:
//   - da: la sorgente di revisione di ACM-1000 (lo STEP B, non ancora in una R5) e' di ACM-6000 e ci si sposta con
//     sposta_da; la revoca su ACM-1000 ha la causa sorgente_errata. Poi S8 dello STEP C su ACM-1000: revisione del cliente;
//   - verso: lo STEP B e' la sorgente S8 (non confermata) di ACM-6000 ed e' la revisione del cliente di ACM-1000: arriva su
//     ACM-1000 con sposta_da. La BOM di ACM-1000 non e' mai stata confermata con uno STEP sbagliato.
func TestV1G39SpostaLaSorgenteInRevisioneNonECorrezione(t *testing.T) {
	t.Run("da", func(t *testing.T) {
		p, s := preparaS8(t)
		ctx := context.Background()
		primaConferma(t, p, s)
		esB := revisioneConStep(t, p, s, s.stepB, s.shaB, "#5", "#6", nil)
		regQ := registroPer(t, registroRadice(t, s, s.stepB, s.shaB, "ACM-1000"), s.altroFinito)
		es, err := s8(t, p, s, s.stepB, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "lo STEP B e' di ACM-6000", "sposta_da": s.prodotto}),
			Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}})
		if err != nil || es.Spostamento == nil || es.Spostamento.Caso != CasoSorgenteS8 || es.Spostamento.BozzaAbbandonata == nil ||
			*es.Spostamento.BozzaAbbandonata != esB.Bozza.BozzaID {
			t.Fatalf("lo spostamento della sorgente di revisione: %+v %v", es, err)
		}
		if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'revoca_sorgente' AND sha256 = $2 AND motivo LIKE 'sorgente_errata:%'`,
			s.prodotto, s.shaB); n != 1 {
			t.Fatalf("la revoca con la causa su ACM-1000: %d", n)
		}
		// su ACM-6000 la sorgente spostata non e' una correzione della sua BOM (non ne ha una)
		if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.altroFinito, s.shaB); err != nil || !st.Valida || st.Correzione {
			t.Fatalf("la sorgente spostata su ACM-6000: %+v %v", st, err)
		}
		esC := revisioneConStep(t, p, s, s.stepC, s.shaC, "#7", "#8", nil)
		revisioneDelCliente(t, p, s, esC)
	})
	t.Run("verso", func(t *testing.T) {
		p, s := preparaS8(t)
		primaConferma(t, p, s)
		regQ := registroPer(t, registroRadice(t, s, s.stepB, s.shaB, "ACM-1000"), s.altroFinito)
		if es, err := s8(t, p, s, s.stepB, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regQ, map[string]any{"motivo": "sorgente scelta per ACM-6000"}),
			Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regQ}}); err != nil || es.Esito != EsitoCompletata {
			t.Fatalf("S8 di B su ACM-6000: %+v %v", es, err)
		}
		esB := revisioneConStep(t, p, s, s.stepB, s.shaB, "#5", "#6", map[string]any{"sposta_da": s.altroFinito, "motivo": "la revisione 2 del cliente di ACM-1000"})
		if esB.Spostamento == nil || esB.Spostamento.Caso != CasoSorgenteS8 || esB.Spostamento.DaProdottoID != s.altroFinito {
			t.Fatalf("lo spostamento verso ACM-1000: %+v", esB.Spostamento)
		}
		if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'conferma_documento' AND sha256 = $2
			AND motivo = 'la revisione 2 del cliente di ACM-1000'`, s.prodotto, s.shaB); n != 1 {
			t.Fatalf("la riga di S8 su ACM-1000 con il motivo scritto, senza la causa: %d", n)
		}
		revisioneDelCliente(t, p, s, esB)
	})
}
