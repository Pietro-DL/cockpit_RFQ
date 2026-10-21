//go:build integrazione

// L4 — direttiva 39, corsia motore: S9 sulla rotta HTTP (GET /thread/{id}/motore-a/dati?sorgente=1) con e senza S8, con il
// calcolo vero del Motore A (regole ACME, scena B87 di motore_a_db_test.go):
//   - senza S8 (D-AR11 B [U]): nessuna riga del motore e nessun «salva nella bozza» dal motore per il prodotto; i file che
//     il motore abbinerebbe restano visibili nella chip «attende la sorgente» con il motivo sorgente_non_confermata; la PB
//     manuale della bozza senza STEP resta salvato_nella_bozza; lo STEP storico univoco del prodotto è un legame storico in
//     sola lettura, non una sorgente confermata; la conferma ammessa è S8;
//   - con la conferma di S8 (passo 2: conferma_documento con la chiave): lo STEP è legato_come_sorgente in sola lettura, le
//     proposte del motore ci sono (provenienza motore, priorità del candidato), i suggerimenti dopo l'apertura stanno in
//     preassociazioni_proposte[] (mai in da_rivedere), la PB manuale resta com'era;
//   - nessuna scrittura.
//
// I codici 777xxxxx sono quelli della scena B87; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

func TestL4V1D39S9ConESenzaS8(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MS9D")
	mwAccendi(t, b, s.regole)
	leggi := func() motorea_web.ProdottoSorgente {
		t.Helper()
		prima := fotoDelDatabase(t, b)
		resp, corpo := s.ad.fai(http.MethodGet, s.dati()+"?sorgente=1", nil, false)
		if resp.StatusCode != 200 {
			t.Fatalf("S9: %d %.400s", resp.StatusCode, corpo)
		}
		if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
			t.Errorf("S9 ha scritto: %s", d)
		}
		var so motorea_web.Sorgente
		if err := json.Unmarshal([]byte(corpo), &so); err != nil {
			t.Fatal(err)
		}
		for _, p := range so.Prodotti {
			if p.ProdottoID != nil && *p.ProdottoID == s.prodotto {
				return p
			}
		}
		t.Fatalf("nessuna sezione del prodotto: %.400s", corpo)
		return motorea_web.ProdottoSorgente{}
	}
	righe := func(p motorea_web.ProdottoSorgente, allegato uuid.UUID) []motorea_web.RigaSorgente {
		var out []motorea_web.RigaSorgente
		for _, r := range p.Righe {
			if r.AllegatoID == allegato {
				out = append(out, r)
			}
		}
		return out
	}

	// la bozza manuale senza STEP: il prodotto e un nodo nuovo con una PB manuale
	fileM, _ := s.allegato("77799150.pdf")
	em := distintabozza.EmittenteCliente(uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread))
	s.s9Bozza3(t, s.prodotto, map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"nodi":  []any{s9Nodo("k-p", &s.prodotto, "finito", "77722757", em), s9Nodo("k-n", nil, "commerciale", "77799150", em)},
		"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-n", "qta": 1, "origine": "utente"}}, "decisioni": []any{},
		"documenti_preassociati": []any{map[string]any{"allegato_id": fileM.String(), "chiave_bozza": "k-n"}}}, "")

	// 1. senza S8
	p := leggi()
	t.Logf("senza S8: conti %+v, chip %+v, fonte %+v", p.Conti, p.InAttesaSorgente, p.Fonte)
	for _, r := range p.Righe {
		if r.Origine == motorea_web.OrigineRigaMotore || r.Stato == motorea_web.StatoRigaProposto || r.Azioni.SalvaNellaBozza.Disponibile {
			t.Errorf("senza S8, una riga del motore: %+v", r)
		}
	}
	if p.Conti.Proposti != 0 || len(p.PreassociazioniProposte) != 0 {
		t.Errorf("senza S8: proposti %d, suggerimenti %+v", p.Conti.Proposti, p.PreassociazioniProposte)
	}
	inAttesa := map[uuid.UUID]bool{}
	for _, c := range p.InAttesaSorgente {
		if c.Motivo != motorea_web.MotivoAttesaNonConfermata {
			t.Errorf("senza S8, una chip senza il motivo: %+v", c)
		}
		inAttesa[c.AllegatoID] = true
	}
	if rr := righe(p, fileM); len(rr) != 1 || rr[0].Stato != motorea_web.StatoRigaSalvato || rr[0].Destinazione.ChiaveBozza != "k-n" {
		t.Errorf("la PB manuale senza STEP: %+v", rr)
	}
	if rr := righe(p, s.allStep); len(rr) != 1 || rr[0].Stato != motorea_web.StatoRigaLegameStorico || rr[0].Azioni.Sposta.Disponibile {
		t.Errorf("lo STEP storico univoco: %+v", rr)
	}
	if p.Fonte.SorgenteConfermata || p.Fonte.SorgenteS8 != nil || p.Fonte.LegameStorico == nil || !p.Fonte.ConfermaAmmessa || p.Fonte.Gesto != motorea_web.GestoConfermaSorgente {
		t.Errorf("la fonte senza S8: %+v", p.Fonte)
	}

	// 2. la conferma di S8 (passo 2)
	s.esegui(`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, 'bom', $5, $6, $7)`, s.thread, s.prodotto, s.allStep, s.shaDoc(s.step), s.step, uuid.New(), s.utente)
	p = leggi()
	t.Logf("con S8: conti %+v, chip %+v, suggerimenti %+v", p.Conti, p.InAttesaSorgente, p.PreassociazioniProposte)
	if rr := righe(p, s.allStep); len(rr) != 1 || rr[0].Stato != motorea_web.StatoRigaLegatoComeSorgente || rr[0].Motivo != motorea_web.MotivoRigaConfermaS8 ||
		rr[0].Azioni.Sposta.Disponibile || rr[0].Azioni.TogliDallaBozza.Disponibile {
		t.Errorf("lo STEP legato come sorgente: %+v", rr)
	}
	if !p.Fonte.SorgenteConfermata || p.Fonte.ConfermaAmmessa || p.Fonte.LegameStorico != nil || p.Conti.LegatiComeSorgente != 1 {
		t.Errorf("la fonte con S8: %+v, conti %+v", p.Fonte, p.Conti)
	}
	motore := 0
	for _, r := range p.Righe {
		if r.Origine != motorea_web.OrigineRigaMotore {
			continue
		}
		motore++
		if r.Provenienza != motorea_web.ProvenienzaMotore {
			t.Errorf("una riga del motore senza la provenienza: %+v", r)
		}
		if r.Stato == motorea_web.StatoRigaProposto && r.PrioritaAnteprima != motorea_web.PrioritaCandidatoMotore {
			t.Errorf("la priorità del candidato del motore: %+v", r)
		}
	}
	if len(inAttesa) > 0 && motore == 0 {
		t.Errorf("con S8 le proposte del motore per i file in attesa (%v) non ci sono", inAttesa)
	}
	for _, x := range p.PreassociazioniProposte {
		if x.Provenienza != motorea_web.ProvenienzaMotore || (x.Motivo != motorea_web.MotivoSuggerimentoApertura && x.Motivo != motorea_web.MotivoSuggerimentoPBToccata &&
			x.Motivo != motorea_web.MotivoSuggerimentoARAttiva && x.Motivo != motorea_web.MotivoSuggerimentoConflitto) {
			t.Errorf("un suggerimento fuori forma: %+v", x)
		}
	}
	if rr := righe(p, fileM); len(rr) != 1 || rr[0].Stato != motorea_web.StatoRigaSalvato || rr[0].Destinazione.ChiaveBozza != "k-n" {
		t.Errorf("la PB manuale con S8: %+v", rr)
	}
}
