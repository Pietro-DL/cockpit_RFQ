//go:build integrazione

// L4 — giro G39, corsia motore, G39-02 [U] in S9 sul calcolo vero (rilievo della revisione del giro G39: «l'esclusione
// di S9 è provata solo in L1»). Scena scenaG39 (il prodotto finito 9110001A, regole ACME di prova), con uno STEP che NON è
// un gruppo del prodotto: «9110001.stp», il nome senza il marcatore, con una struttura letta i cui nodi non hanno codici
// (nessun gruppo del prodotto, quindi niente step[] e niente salto «STEP del prodotto»), ma che il motore ancora al
// prodotto dal nome del file (base 9110001, marcatore compatibile parziale):
//   - TestL4V1G39LoSTEPCandidatoNonEUnFileDaSmistare: in S9 lo STEP è nella fonte del prodotto come proposta di sorgente
//     e non è mai una riga proposto o in_conflitto del motore, né la chip «attende la sorgente», né un file da associare,
//     prima di S8 e dopo S8 su un altro STEP del prodotto. Senza l'esclusione di S9 (mutazione dichiarata nel resoconto)
//     la prova fallisce: lo STEP compare nella chip prima di S8;
//   - TestL4V1G39LaPBSulloSTEPCandidatoEInConflitto: una PB sullo STEP candidato rimasta da prima (salvata con R2 mentre
//     le regole del motore non erano pronte: il ripiego [T-39] di R2 senza il calcolo non la vede, perché la proposta del
//     vecchio lettore non ha codice), poi il motore pronto: in S9 la riga della PB è in_conflitto con il motivo
//     step_candidato_del_finito (mai salvato_nella_bozza), e lo STEP non è una proposta del motore.
//
// I codici 91xxxxxA sono quelli della grammatica ACME di prova; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// sorgenteG39E: le parti di S9 che le due prove guardano.
type sorgenteG39E struct {
	Prodotti []struct {
		ProdottoID *uuid.UUID `json:"prodotto_id"`
		Fonte      struct {
			ProposteSorgente []struct {
				AllegatoID uuid.UUID `json:"allegato_id"`
			} `json:"proposte_sorgente"`
		} `json:"fonte"`
		Righe []struct {
			AllegatoID uuid.UUID `json:"allegato_id"`
			Stato      string    `json:"stato"`
			Motivo     string    `json:"motivo"`
			Origine    string    `json:"origine"`
		} `json:"righe"`
		InAttesaSorgente []struct {
			AllegatoID uuid.UUID `json:"allegato_id"`
		} `json:"in_attesa_sorgente"`
	} `json:"prodotti"`
	DaAssociare []struct {
		AllegatoID uuid.UUID `json:"allegato_id"`
	} `json:"da_associare"`
}

// s9G39E: S9 della scena e, per lo STEP dato, le righe della sezione del prodotto, se è nella chip, se è da associare e
// se è una proposta di sorgente.
func (s *scenaG39) s9G39E(t *testing.T, step uuid.UUID) (righe []string, inAttesa, daAssociare, proposta bool) {
	t.Helper()
	resp, corpo := s.ad.fai(http.MethodGet, "/thread/"+s.thread.String()+"/motore-a/dati?sorgente=1", nil, false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("S9: %d %.800s", resp.StatusCode, corpo)
	}
	var x sorgenteG39E
	if err := json.Unmarshal([]byte(corpo), &x); err != nil {
		t.Fatalf("S9: %v", err)
	}
	trovata := false
	for _, p := range x.Prodotti {
		if p.ProdottoID == nil || *p.ProdottoID != s.prodotto {
			continue
		}
		trovata = true
		for _, r := range p.Righe {
			if r.AllegatoID == step {
				righe = append(righe, r.Stato+"/"+r.Motivo+"/"+r.Origine)
			}
		}
		for _, c := range p.InAttesaSorgente {
			inAttesa = inAttesa || c.AllegatoID == step
		}
		for _, c := range p.Fonte.ProposteSorgente {
			proposta = proposta || c.AllegatoID == step
		}
	}
	if !trovata {
		t.Fatalf("S9 senza la sezione del prodotto: %.600s", corpo)
	}
	for _, d := range x.DaAssociare {
		daAssociare = daAssociare || d.AllegatoID == step
	}
	return righe, inAttesa, daAssociare, proposta
}

// stepSenzaGruppoG39: lo STEP «9110001.stp» con una struttura letta senza codici e la proposta del vecchio lettore senza
// codice (commento di testa).
func (s *scenaG39) stepSenzaGruppoG39() uuid.UUID {
	a, _ := s.stepG39("9110001.stp", "", fattiDi("#1", []string{"#1=ASSIEME GENERALE", "#2=TELAIO SALDATO"}, []string{"#1>#2*1"}))
	return a
}

func TestL4V1G39LoSTEPCandidatoNonEUnFileDaSmistare(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaG39(t, "G39E")
	step := s.stepSenzaGruppoG39()
	prima := fotoDelDatabase(t, b)
	righe, inAttesa, daAssociare, proposta := s.s9G39E(t, step)
	t.Logf("lo STEP in S9: righe %v, chip %v, da associare %v, proposta di sorgente %v", righe, inAttesa, daAssociare, proposta)
	if !proposta {
		t.Errorf("lo STEP candidato non è una proposta di sorgente del prodotto")
	}
	if len(righe) != 0 || inAttesa || daAssociare {
		t.Errorf("lo STEP candidato è un file da smistare: righe %v, chip «attende la sorgente» %v, da associare %v", righe, inAttesa, daAssociare)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S9 ha scritto: %s", d)
	}

	// dopo S8 su un altro STEP del prodotto (con la struttura): il motore propone i file per il prodotto, ma lo STEP
	// candidato non diventa una riga proposto (né in_conflitto del motore) e non è da associare
	sorgente, _ := s.stepG39("9110001A.stp", "9110001A", fattiDi("#1", []string{"#1=9110001A", "#2=9120101A"}, []string{"#1>#2*2"}))
	base := "/thread/" + s.thread.String()
	var gruppi struct {
		Gruppi []struct {
			Gruppo            string `json:"gruppo"`
			ImprontaContenuto string `json:"impronta_contenuto"`
		} `json:"gruppi"`
	}
	resp, testo := s.ad.fai(http.MethodGet, base+"/sorgente/step/"+sorgente.String()+"/anteprima?prodotto="+s.prodotto.String(), nil, false)
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &gruppi) != nil {
		t.Fatalf("S6: %d %.600s", resp.StatusCode, testo)
	}
	gruppo, impronta := "", ""
	for _, g := range gruppi.Gruppi {
		if gruppo == "" && g.ImprontaContenuto != "" {
			gruppo, impronta = g.Gruppo, g.ImprontaContenuto
		}
	}
	corpo, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "partenza": "step", "gruppo": gruppo,
		"impronta_contenuto": impronta, "motivo": "sorgente della prova"})
	resp, testo = s.ad.chiediJSON(http.MethodPost, base+"/sorgente/step/"+sorgente.String()+"/conferma-sorgente", string(corpo))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) {
		t.Fatalf("S8: %d %.1500s", resp.StatusCode, testo)
	}
	righe, inAttesa, daAssociare, proposta = s.s9G39E(t, step)
	t.Logf("dopo S8 sull'altro STEP: righe %v, chip %v, da associare %v, proposta di sorgente %v", righe, inAttesa, daAssociare, proposta)
	if len(righe) != 0 || inAttesa || daAssociare {
		t.Errorf("dopo S8, lo STEP candidato è un file da smistare: righe %v, chip %v, da associare %v", righe, inAttesa, daAssociare)
	}
	if proposta {
		t.Errorf("dopo S8 lo STEP è ancora una proposta di sorgente (la sorgente c'è: si cambia con una revisione)")
	}
}

func TestL4V1G39LaPBSulloSTEPCandidatoEInConflitto(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaG39(t, "G39F")
	c, err := distintabozza.CreaFormato3(b.ctx, b.pool, distintabozza.Creazione3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s.prodotto, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea:   distintabozza.CreaR2{Partenza: distintabozza.PartenzaVuota}})
	if err != nil {
		t.Fatalf("la bozza vuota: %v", err)
	}
	step := s.stepSenzaGruppoG39()

	// la PB sullo STEP salvata con R2 mentre le regole del motore non sono pronte (il ripiego senza il calcolo)
	b.ws.MotoreA = motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente}
	var raw json.RawMessage
	if err := b.pool.QueryRow(b.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, c.BozzaID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	nodi, _ := m["nodi"].([]any)
	if len(nodi) == 0 {
		t.Fatalf("la bozza vuota senza il nodo del prodotto: %s", raw)
	}
	radice, _ := nodi[0].(map[string]any)["chiave_bozza"].(string)
	pb := map[string]any{"allegato_id": step, "chiave_bozza": radice}
	m["documenti_preassociati"] = []any{pb}
	corpo, _ := json.Marshal(map[string]any{"bozza_id": c.BozzaID, "versione_attesa": 1, "prodotto_id": s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m,
		"azioni": []any{map[string]any{"azione": "file_preassociato", "chiave_bozza": radice, "dopo": pb, "origine": "utente"}}})
	resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bozza", string(corpo))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("R2 con il motore non pronto (il ripiego non vede lo STEP): %d %.1200s", resp.StatusCode, testo)
	}

	// il motore pronto: la PB è in conflitto, lo STEP non è una proposta del motore
	mwAccendi(t, b, s.regole)
	prima := fotoDelDatabase(t, b)
	righe, inAttesa, daAssociare, _ := s.s9G39E(t, step)
	t.Logf("lo STEP con la PB in S9: righe %v, chip %v, da associare %v", righe, inAttesa, daAssociare)
	if len(righe) != 1 || righe[0] != "in_conflitto/"+motorea_web.MotivoRigaStepCandidatoDelFinito+"/"+motorea_web.OrigineRigaBozza {
		t.Errorf("la riga della PB sullo STEP candidato: %v (attesa una sola, in_conflitto/step_candidato_del_finito/bozza)", righe)
	}
	if inAttesa || daAssociare {
		t.Errorf("lo STEP con la PB: chip %v, da associare %v", inAttesa, daAssociare)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S9 ha scritto: %s", d)
	}
}
