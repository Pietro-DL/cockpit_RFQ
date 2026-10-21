//go:build integrazione

// L4 — direttiva 39 §5 [U] e D-PB01 A / D-AR11 B (rilievo importante della revisione del 20/10), corsia persistenza: uno
// STEP del prodotto finito che non e' (ancora) la sorgente confermata da S8 — la rev 2 arrivata prima della prima
// conferma — non diventa una PB iniziale del motore ne' un suggerimento, anche se il registro lo propone sulla radice;
// quindi R5 non ne scrive una AR ordinaria e S8 su quello STEP resta ammessa. Prima della correzione la PB «motore» sulla
// radice nasceva con la bozza, R5 la registrava come AR conferma_bom e S8 la rifiutava (gia_associata_altrove). Dati
// sintetici: la scena di s8_db_test.go, con registri di prova al posto del calcolo del motore.

package sorgente

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// conProposta: il registro con la proposta d'associazione dell'allegato sulla voce della chiave data.
func conProposta(t *testing.T, reg importazione.Registro, chiave string, allegato uuid.UUID) importazione.Registro {
	t.Helper()
	c := reg.Contenuto
	trovata := false
	for i := range c.Proposta.Nodi {
		if c.Proposta.Nodi[i].Origine.Chiave == chiave {
			c.Proposta.Nodi[i].AssociazioniProposte = rawT(t, []any{map[string]any{"allegato_id": allegato.String(), "motivo": "nome", "livello": "alto"}})
			trovata = true
		}
	}
	if !trovata {
		t.Fatalf("voce %s assente", chiave)
	}
	c.AllegatiCitati = append(c.AllegatiCitati, allegato.String())
	sort.Strings(c.AllegatiCitati)
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

func TestV1D39StepDelFinitoNonDiventaPBNeAR(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	// il figlio della working e' un pezzo sciolto (un sottoassieme senza figli e' un blocco D1 di R4)
	esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
	// il registro di A propone lo STEP B (un altro STEP del prodotto, proposta cad_3d con il codice del finito) sulla
	// radice, e un PDF sulla staffa
	pdf := pdfDi(t, p, s, "ACM-1150", '6')
	regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#3", codice: "ACM-1150", qta: 4}})
	regA = conProposta(t, regA, "#1", s.stepB)
	regA = conProposta(t, regA, "#3", pdf)
	fattiRevisione(t, p, s.shaA, "#1", "#3")
	esA, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"motivo": "prima sorgente"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	if err != nil || esA.Bozza == nil || esA.Bozza.Versione != 1 {
		t.Fatalf("S8 su A: %+v %v", esA, err)
	}
	bozza := esA.Bozza.BozzaID
	s1, _ := snapshotVersione(t, p, bozza, 1)
	pbPDF := false
	for _, x := range s1.DocumentiPreassociati {
		if x.AllegatoID == s.stepB {
			t.Fatalf("lo STEP del finito e' diventato una PB iniziale: %+v", x)
		}
		pbPDF = pbPDF || (x.AllegatoID == pdf && x.Provenienza == distintabozza.ProvenienzaMotore)
	}
	if !pbPDF {
		t.Fatalf("la PB iniziale del PDF (la regola resta per i documenti): %+v", s1.DocumentiPreassociati)
	}
	var dopo []byte
	if err := p.QueryRow(ctx, `SELECT dopo FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'bozza_aperta'`, bozza).Scan(&dopo); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dopo), s.stepB.String()) {
		t.Fatalf("lo STEP del finito fra le PB iniziali o i suggerimenti della nascita: %s", dopo)
	}
	// R1: nemmeno fra i suggerimenti dopo l'apertura
	r1, err := distintabozza.LeggiR1(ctx, db.New(p), s.thread, s.prodotto, distintabozza.OpzioniR1{})
	if err != nil || r1.PreassociazioniProposte == nil {
		t.Fatalf("R1: %+v %v", r1, err)
	}
	for _, x := range *r1.PreassociazioniProposte {
		if x.AllegatoID == s.stepB {
			t.Fatalf("lo STEP del finito fra i suggerimenti di R1: %+v", x)
		}
	}
	rp := riepilogoRev(t, p, s, bozza, 1)
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4: %v", codiciR4(rp))
	}
	c1, err := confermaRev(t, p, s, bozza, 1, rp.FirmaAnteprima)
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, s.stepB); n != 0 {
		t.Fatalf("R5 ha registrato lo STEP del finito come AR ordinaria: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND stato = 'attiva' AND conferma_id = $2`, pdf, c1.ConfermaID); n != 1 {
		t.Fatal("la AR del PDF della staffa")
	}
	// S8 sullo STEP B resta ammessa: da-conferma-1 e cambio della fonte su B
	regB := registroConFigli(t, s, s.stepB, s.shaB, []figlioProva{{chiave: "#3", codice: "ACM-1150", qta: 4}})
	fattiRevisione(t, p, s.shaB, "#1", "#3")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sostituisci": map[string]any{"conferma": true}, "motivo": "revisione 2"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Esito != EsitoCompletata || esB.Bozza == nil {
		t.Fatalf("S8 su B dopo la conferma: %+v %v", esB, err)
	}
}
