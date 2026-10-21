//go:build integrazione

// L4 — G39-02 [U] (20/10 sera), corsia persistenza parte A: lo STEP del prodotto finito e' un candidato sorgente, mai una
// PB ne' una AR ordinaria. R2 rifiuta lo STEP candidato come documento di una PB (422 STEP_CANDIDATO_DEL_FINITO), anche
// in una bozza manuale; R4 blocca una PB del genere rimasta da una versione di prima (PREASSOCIAZIONE_NON_APPLICABILE,
// motivo step_candidato_del_finito). Lo STEP di un particolare resta una PB (D-2 (c) [U]). Le prove di questo file usano
// solo il ripiego senza il motore (la lettura legacy con il confronto del marcatore: ACM-2000 letto contro il prodotto
// ACM-2000Z) e i codici come stringhe, cosi' girano anche sul codice di prima, dove falliscono (la PB veniva salvata e R4
// non bloccava). Le prove con il riconoscimento del Motore A sono in c39_g39_motore_db_test.go. Dati sintetici.

package distintabozza

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// manualeConPB: la bozza vuota del prodotto con un pezzo nuovo (k-man) e la PB dell'allegato salvata con R2: l'errore di R2.
func manualeConPB(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto uuid.UUID, a uuid.UUID) (uuid.UUID, error) {
	t.Helper()
	c, err := creaDB(t, p, s, prodotto, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	id := map[string]any{"emittente": radice["identita"].(map[string]any)["emittente"], "emittente_presunto": true, "codice": "ACM-6500", "modo": "manuale"}
	nodo := map[string]any{"chiave_bozza": "k-man", "op": "crea", "tipo": "sciolto", "origine": "utente", "identita": id}
	arco := map[string]any{"padre": radice["chiave_bozza"], "figlio": "k-man", "qta": 2, "origine": "utente"}
	m["nodi"] = append(m["nodi"].([]any), nodo)
	m["archi"] = []any{arco}
	x := map[string]any{"allegato_id": a, "chiave_bozza": "k-man"}
	m["documenti_preassociati"] = []any{x}
	sc := scena3DB{scenaDB: s, p: p}
	_, err = sc.salva3Su(t, c.BozzaID, prodotto, 1, uuid.New(), m, []Azione{
		{Azione: AzNodoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(nodo), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(arco), Origine: "utente"},
		{Azione: AzFilePreassociato, ChiaveBozza: "k-man", Dopo: mustJSON(x), Origine: "utente"}})
	return c.BozzaID, err
}

// finitoConMarcatore: un secondo prodotto finito della RFQ con il codice che finisce con il marcatore (ACM-2000Z) e il suo
// STEP, che la proposta di documento legge cad_3d con il codice senza marcatore (ACM-2000).
func finitoConMarcatore(t *testing.T, p *pgxpool.Pool, s scenaDB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	fin := uuid.New()
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-2000Z', 'finito', 'manuale', $3)`,
		fin, s.thread, s.utente)
	step := stpDocumentaleDB(t, p, s.thread, "ACM-2000-A.stp", "ACM-2000", '4')
	return fin, step
}

func TestV1G39_02R2RifiutaLoStepCandidatoDelFinitoDB(t *testing.T) {
	p, s := poolPulito(t)
	_, step := finitoConMarcatore(t, p, s)
	prima := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`)
	// la PB manuale dello STEP candidato (riconosciuto con il marcatore): 422 STEP_CANDIDATO_DEL_FINITO, niente scritto
	_, err := manualeConPB(t, p, s, s.altroFinito, step)
	var e *Errore
	if !errors.As(err, &e) || e.Codice != "STEP_CANDIDATO_DEL_FINITO" || !strings.Contains(e.Campo, "documenti_preassociati") ||
		e.AllegatoID == nil || *e.AllegatoID != step {
		t.Fatalf("la PB manuale dello STEP candidato del finito: atteso 422 STEP_CANDIDATO_DEL_FINITO, avuto %v", err)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`); n != prima+1 {
		t.Fatalf("il rifiuto ha scritto una versione: %d -> %d (solo la versione 1 della crea)", prima, n)
	}
	// lo STEP di un particolare (proposta cad_3d con il suo codice) resta una PB come un PDF (D-2 (c) [U])
	parte := stpDocumentaleDB(t, p, s.thread, "ACM-6500.stp", "ACM-6500", '6')
	bozza, err := manualeConPB(t, p, s, s.prodotto, parte)
	if err != nil {
		t.Fatalf("lo STEP di un particolare: %v", err)
	}
	_, raw := snapshotDellaVersione(t, p, bozza, 2)
	if pb := pbDi(t, raw, parte); pb.Provenienza != ProvenienzaManuale {
		t.Fatalf("la PB dello STEP del particolare: %+v", pb)
	}
}

func TestV1G39_02R4BloccaLaPBDelloStepCandidatoDB(t *testing.T) {
	p, s := poolPulito(t)
	esegui3(t, p, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, 'ACM-2000Z', 'finito', 'manuale', $2)`, s.thread, s.utente)
	// la PB salvata quando lo STEP non era ancora un candidato riconoscibile (una versione di prima)
	step := stepDB(t, p, s.thread, "ACM-2000-B.stp", '5')
	bozza, err := manualeConPB(t, p, s, s.altroFinito, step)
	if err != nil {
		t.Fatalf("la PB dello STEP prima del riconoscimento: %v", err)
	}
	// ora la lettura dice lo STEP cad_3d del finito ACM-2000Z (senza il marcatore): R4 blocca, R5 non la registra
	esegui3(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', 'ACM-2000', 80, 'nome_file')`,
		step, s.thread)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, 2), s.utente)
	trovato := false
	for _, r := range rp.Blocchi {
		trovato = trovato || (r.Codice == BloccoPreassociazioneNonApplicab && r.Motivo == "step_candidato_del_finito" && r.AllegatoID != nil && *r.AllegatoID == step)
	}
	if !trovato || rp.Confermabile {
		t.Fatalf("R4 con la PB dello STEP candidato del finito: %v", codici(rp.Blocchi))
	}
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, bozza, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	// R5 rifiuta proprio per lo STEP candidato del finito (rilievo minore 8 della revisione G39 parte A): la bozza ha anche il
	// gate STEP_SORGENTE_NON_CONFERMATA, quindi il solo codice BLOCCHI non basterebbe a provarlo
	rif := rifiutoR5Di(t, err, CodiceBlocchi)
	if g := trovaMotivo(rif.Blocchi, BloccoPreassociazioneNonApplicab, "step_candidato_del_finito"); len(g) != 1 || g[0].AllegatoID == nil || *g[0].AllegatoID != step {
		t.Fatalf("R5 senza il blocco step_candidato_del_finito dello STEP: %v", codici(rif.Blocchi))
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, step); n != 0 {
		t.Fatalf("AR dello STEP candidato: %d", n)
	}
}
