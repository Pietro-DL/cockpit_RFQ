//go:build integrazione

// L4 — R39-15 (decisione [T-39] dell'orchestratore) sul database di prova: la pre-assegnazione delle pagine di prima
// convertita alla presa in carico diventa una PB legacy; R4 la blocca (legacy_da_verificare); il salvataggio R2 con
// file_riconosciuto la rende verificata (provenienza legacy) con l'evento nel giornale; la verificata resta nel salvataggio
// della sorgente (S8 passo 3); R4 non la blocca piu'; R5 registra la AR con provenienza_pb legacy. Il riconoscimento di una
// PB manuale e la verificata dichiarata senza riconoscimento sono 422. Dati sintetici.

package distintabozza

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
)

func TestV1D39RiconoscimentoPBLegacyDB(t *testing.T) {
	p, s := poolPulito(t)
	pdf := nuovoAllegato(t, p, idDi("msg-0"), 20, "ACM-6000.pdf", strings.Repeat("d", 64))
	nuovaProposta(t, p, pdf, s.thread, &s.altroFinito)
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	root := ChiaveDelComponente(s.altroFinito)
	if pb := pbDi(t, c.Snapshot, pdf); pb.Provenienza != ProvenienzaLegacy || pb.Verificata || pb.ChiaveBozza != root {
		t.Fatalf("PB legacy alla presa in carico: %+v", pb)
	}
	r := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, 1), s.utente)
	if bb := trova(r.Blocchi, BloccoPreassociazioneNonApplicab); len(bb) != 1 || bb[0].Motivo != associazioni.MotivoLegacyDaVerificare {
		t.Fatalf("R4 con la PB legacy: %v", codici(r.Blocchi))
	}
	sc := scena3DB{scenaDB: s, p: p}
	riconosci := Azione{Azione: AzFileRiconosciuto, ChiaveBozza: root, Prima: mustJSON(map[string]any{"allegato_id": pdf, "chiave_bozza": root}),
		Dopo: mustJSON(map[string]any{"allegato_id": pdf, "chiave_bozza": root, "motivo": "il disegno e' del prodotto"}), Origine: "utente"}
	// la verificata dichiarata dalla pagina senza il riconoscimento: 422
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	m["documenti_preassociati"].([]any)[0].(map[string]any)["verificata"] = true
	nodo := map[string]any{"chiave_bozza": "k-x", "op": "crea", "tipo": "commerciale", "origine": "utente",
		"identita": map[string]any{"emittente": EmittenteCliente(idDi("cliente")), "emittente_presunto": true, "codice": "ACM-6999", "modo": "manuale"}}
	arco := map[string]any{"padre": root, "figlio": "k-x", "qta": 1, "origine": "utente"}
	m["nodi"] = append(m["nodi"].([]any), nodo)
	m["archi"] = append(m["archi"].([]any), arco)
	aggiunte := []Azione{{Azione: AzNodoAggiunto, ChiaveBozza: "k-x", Dopo: mustJSON(nodo), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: "k-x", Dopo: mustJSON(arco), Origine: "utente"}}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, aggiunte); err == nil || err.(*Errore).Motivo != MotivoProvenienzaDelServer {
		t.Fatalf("verificata senza riconoscimento: %v", err)
	}
	// il riconoscimento: versione 2 con la PB legacy verificata e l'evento file_riconosciuto
	m, _ = snapshotDellaVersione(t, p, c.BozzaID, 1)
	sv, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{riconosci})
	if err != nil || sv.Versione != 2 {
		t.Fatalf("R2 con file_riconosciuto: %v %+v", err, sv)
	}
	m2, raw2 := snapshotDellaVersione(t, p, c.BozzaID, 2)
	_ = m2
	if pb := pbDi(t, raw2, pdf); pb.Provenienza != ProvenienzaLegacy || !pb.Verificata {
		t.Fatalf("PB verificata: %+v", pb)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'file_riconosciuto' AND versione = 2`, c.BozzaID); n != 1 {
		t.Fatalf("evento file_riconosciuto: %d", n)
	}
	// S8 e il passo 3 sulla bozza: la PB resta legacy e verificata; R4 senza il blocco legacy; R5 con la AR legacy
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 2, s.allegato2)
	_, raw3 := snapshotDellaVersione(t, p, c.BozzaID, v)
	if pb := pbDi(t, raw3, pdf); pb.Provenienza != ProvenienzaLegacy || !pb.Verificata {
		t.Fatalf("la verificata dopo il passo 3 di S8: %+v", pb)
	}
	r = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	if len(r.Blocchi) != 0 || len(r.EffettiDocumentali.LegacyVerificate) != 1 || r.DocumentiPreassociati[0].Esito != associazioni.EsitoApplicabile {
		t.Fatalf("R4 con la legacy verificata: %v %+v %+v", codici(r.Blocchi), r.EffettiDocumentali, r.DocumentiPreassociati)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, r.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil || out.AssociazioniApplicate != 1 {
		t.Fatalf("R5: %v %+v", err, out)
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND stato = 'attiva' AND origine = 'conferma_bom'
		AND provenienza_pb = 'legacy' AND componente_id = $2`, pdf, s.altroFinito); n != 1 {
		t.Fatalf("AR con provenienza_pb legacy: %d", n)
	}
}
