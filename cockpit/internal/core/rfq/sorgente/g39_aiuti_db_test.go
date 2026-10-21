//go:build integrazione

// L4 — G39-01 [U] (20/10 sera), corsia persistenza parte A: gli aiuti delle prove sui file arrivati dopo l'apertura della
// bozza (g39_dopo_apertura_db_test.go) e la prova della PB spostata su un altro nodo con il registro gia' scritto, che usa
// solo le funzioni di prima: gira anche sul codice di prima, dove fallisce (422 proposta_non_corrispondente invece di
// «corretta»). Dati sintetici.

package sorgente

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// conProposteG39: il registro con le proposte d'associazione date, per chiave della voce (sostituiscono quelle di prima).
func conProposteG39(t *testing.T, reg importazione.Registro, proposte map[string][]uuid.UUID) importazione.Registro {
	t.Helper()
	c := reg.Contenuto
	// le fette si copiano: il registro di partenza (anche usato per due calcoli diversi) non cambia
	c.Proposta.Nodi = append(c.Proposta.Nodi[:0:0], c.Proposta.Nodi...)
	c.AllegatiCitati = append(c.AllegatiCitati[:0:0], c.AllegatiCitati...)
	citati := map[string]bool{}
	for _, a := range c.AllegatiCitati {
		citati[a] = true
	}
	for i := range c.Proposta.Nodi {
		v := &c.Proposta.Nodi[i]
		aa := proposte[v.Origine.Chiave]
		if aa == nil {
			continue
		}
		var xx []any
		for _, a := range aa {
			xx = append(xx, map[string]any{"allegato_id": a.String(), "motivo": "nome", "livello": "alto"})
			citati[a.String()] = true
		}
		v.AssociazioniProposte = rawT(t, xx)
	}
	c.AllegatiCitati = c.AllegatiCitati[:0]
	for a := range citati {
		c.AllegatiCitati = append(c.AllegatiCitati, a)
	}
	sort.Strings(c.AllegatiCitati)
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

func suggerimentoDi(l distintabozza.LetturaR1, a uuid.UUID) (distintabozza.PreassociazioneProposta3, bool) {
	if l.PreassociazioniProposte == nil {
		return distintabozza.PreassociazioneProposta3{}, false
	}
	for _, x := range *l.PreassociazioniProposte {
		if x.AllegatoID == a {
			return x, true
		}
	}
	return distintabozza.PreassociazioneProposta3{}, false
}

// chiaviG39: la chiave del nodo della radice (il prodotto) e quella del nodo con la casella della voce data.
func chiaviG39(t *testing.T, sn distintabozza.Snapshot3, prodotto uuid.UUID, rif string) (radice, nodo string) {
	t.Helper()
	for _, n := range sn.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == prodotto {
			radice = n.ChiaveBozza
		}
		for _, c := range n.Caselle {
			if c.Rif == rif {
				nodo = n.ChiaveBozza
			}
		}
	}
	if radice == "" || nodo == "" {
		t.Fatalf("nodi della bozza: radice %q, nodo %q", radice, nodo)
	}
	return radice, nodo
}

func codiceG39(t *testing.T, err error, codice, motivo string) {
	t.Helper()
	var e *distintabozza.Errore
	if !errors.As(err, &e) || e.Codice != codice || (motivo != "" && e.Motivo != motivo) {
		t.Fatalf("atteso %s/%s, avuto %v", codice, motivo, err)
	}
}

func pbDiG39(sn distintabozza.Snapshot3, a uuid.UUID) distintabozza.Preassociazione3 {
	for _, x := range sn.DocumentiPreassociati {
		if x.AllegatoID == a {
			return x
		}
	}
	return distintabozza.Preassociazione3{}
}

// avvioG39: S8 su A con il registro che propone il PDF della staffa sul figlio: la bozza aperta, versione 1, con la PB
// iniziale del PDF.
func avvioG39(t *testing.T) (*pgxpool.Pool, scenaS8, importazione.Registro, uuid.UUID, uuid.UUID) {
	t.Helper()
	p, s := preparaS8(t)
	esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
	pdf := pdfDi(t, p, s, "ACM-1150", '6')
	regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#3", codice: "ACM-1150", qta: 4}})
	regA = conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf}})
	fattiRevisione(t, p, s.shaA, "#1", "#3")
	esA, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"motivo": "prima sorgente"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esA.Bozza == nil || esA.Bozza.Versione != 1 {
		t.Fatalf("S8 su A: %+v %v", esA, err)
	}
	return p, s, regA, esA.Bozza.BozzaID, pdf
}

// richiestaPBG39: il corpo di R2 salvataggio con una PB in piu' (il registro dichiarato, se dato), gia' letto.
func richiestaPBG39(t *testing.T, p *pgxpool.Pool, s scenaS8, bozza uuid.UUID, versione int, a uuid.UUID, chiave, registro string) distintabozza.Salvataggio3 {
	t.Helper()
	var raw []byte
	if err := p.QueryRow(context.Background(), `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	pb := map[string]any{"allegato_id": a, "chiave_bozza": chiave}
	if registro != "" {
		pb["importazione"] = registro
	}
	pp, _ := m["documenti_preassociati"].([]any)
	m["documenti_preassociati"] = append(pp, pb)
	corpo, err := json.Marshal(map[string]any{"bozza_id": bozza, "versione_attesa": versione, "prodotto_id": s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m, "azioni": []distintabozza.Azione{{Azione: distintabozza.AzFilePreassociato, ChiaveBozza: chiave,
			Dopo: rawT(t, map[string]any{"allegato_id": a, "chiave_bozza": chiave}), Origine: "utente"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := distintabozza.LeggiRichiestaR2(corpo)
	if err != nil {
		t.Fatalf("corpo di R2: %v", err)
	}
	sv := r.Salvataggio
	sv.ThreadID, sv.UtenteID = s.thread, s.utente
	return sv
}

// G39-01 [U]: la proposta del motore spostata dall'ingegnere su un altro componente e' «corretta», con il riferimento al
// calcolo che l'aveva proposta (qui il registro di adesso gia' scritto in distinta_importazione, come lo lascia R2 dopo il
// primo suggerimento accettato). Sul codice di prima: 422 proposta_non_corrispondente.
func TestV1G39_01SuggerimentoSpostatoECorrettaDB(t *testing.T) {
	p, s, regA, bozza, pdf := avvioG39(t)
	s1, _ := snapshotVersione(t, p, bozza, 1)
	radice, _ := chiaviG39(t, s1, s.prodotto, "nodo:"+s.shaA+":#3")
	f2 := pdfDi(t, p, s, "ACM-1150-F3", '8')
	adesso := conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf, f2}})
	if _, err := db.New(p).InsertDistintaImportazione(context.Background(), distintabozza.ParametriImportazione(adesso, s.utente)); err != nil {
		t.Fatalf("il registro di adesso: %v", err)
	}
	sv := richiestaPBG39(t, p, s, bozza, 1, f2, radice, adesso.Impronta)
	if _, err := distintabozza.SalvaFormato3(context.Background(), p, sv); err != nil {
		t.Fatalf("il suggerimento spostato sulla radice: %v", err)
	}
	s2, _ := snapshotVersione(t, p, bozza, 2)
	if x := pbDiG39(s2, f2); x.Provenienza != distintabozza.ProvenienzaCorretta || x.Importazione != adesso.Impronta || x.ChiaveBozza != radice {
		t.Fatalf("la PB spostata: %+v", x)
	}
}
