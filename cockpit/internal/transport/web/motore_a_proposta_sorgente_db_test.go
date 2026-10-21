//go:build integrazione

// L4 — giro G39, corsia motore (G39-02 [U]; aperto (b) del verificatore del 20/10): lo STEP del prodotto finito
// riconosciuto con il Motore A e proposto come sorgente, con il calcolo vero e le regole ACME di prova (scenaG39: il
// prodotto finito 9110001A, che la grammatica legge con il marcatore A):
//   - lo STEP «9110001.stp» (il nome senza il marcatore, come il codice del cliente scritto senza il marcatore del
//     prodotto) con la proposta di documento del vecchio lettore SENZA codice: il riconoscimento non passa da
//     documento_proposta.codice;
//   - S9 (GET /dati?sorgente=1) e /dati: nella fonte del prodotto proposte_sorgente[] con lo STEP, il testo, le evidenze
//     della lettura (il nome del file letto «9110001», il marcatore compatibile parziale); nessuna riga dello STEP fra le
//     righe del prodotto (mai proposto né salvabile), lo STEP non è da associare; le GET non scrivono;
//   - dopo S8 sullo STEP (la conferma dell'utente, rotta vera): nessuna proposta (è la sorgente), la riga
//     legato_come_sorgente.
//
// I codici 91xxxxxA sono quelli della grammatica ACME di prova; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
)

func TestL4V1G39LoSTEPCandidatoEUnaPropostaDiSorgente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaG39(t, "G39P")
	step, _ := s.stepG39("9110001.stp", "", fattiDi("#1", []string{"#1=9110001A", "#2=9120101A", "#3=9130202A"}, []string{"#1>#2*2", "#1>#3*1"}))
	base := "/thread/" + s.thread.String()
	type proposta struct {
		AllegatoID     uuid.UUID                         `json:"allegato_id"`
		NomeFile       string                            `json:"nome_file"`
		CodiceProdotto string                            `json:"codice_prodotto"`
		Evidenze       []propostabom.EvidenzaCandidatura `json:"evidenze"`
		SorgenteDi     *uuid.UUID                        `json:"sorgente_di"`
		Testo          string                            `json:"testo"`
	}
	type sorgente struct {
		Prodotti []struct {
			ProdottoID *uuid.UUID `json:"prodotto_id"`
			Fonte      struct {
				ProposteSorgente []proposta `json:"proposte_sorgente"`
			} `json:"fonte"`
			Righe []struct {
				AllegatoID uuid.UUID `json:"allegato_id"`
				Stato      string    `json:"stato"`
				Motivo     string    `json:"motivo"`
			} `json:"righe"`
		} `json:"prodotti"`
		DaAssociare []struct {
			AllegatoID uuid.UUID `json:"allegato_id"`
		} `json:"da_associare"`
	}
	leggi := func(percorso string, x any) {
		t.Helper()
		resp, corpo := s.ad.fai(http.MethodGet, percorso, nil, false)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %.600s", percorso, resp.StatusCode, corpo)
		}
		if err := json.Unmarshal([]byte(corpo), x); err != nil {
			t.Fatalf("%s: %v %.300s", percorso, err, corpo)
		}
	}
	s9 := func() sorgente {
		t.Helper()
		var x sorgente
		leggi(base+"/motore-a/dati?sorgente=1", &x)
		for _, p := range x.Prodotti {
			if p.ProdottoID != nil && *p.ProdottoID == s.prodotto {
				return x
			}
		}
		t.Fatalf("S9 senza la sezione del prodotto: %+v", x)
		return x
	}
	sezione := func(x sorgente) (pp []proposta, righe []string) {
		for _, p := range x.Prodotti {
			if p.ProdottoID == nil || *p.ProdottoID != s.prodotto {
				continue
			}
			pp = p.Fonte.ProposteSorgente
			for _, r := range p.Righe {
				if r.AllegatoID == step {
					righe = append(righe, r.Stato+"/"+r.Motivo)
				}
			}
		}
		return pp, righe
	}

	prima := fotoDelDatabase(t, b)
	x := s9()
	pp, righe := sezione(x)
	if len(pp) != 1 || pp[0].AllegatoID != step || pp[0].NomeFile != "9110001.stp" || pp[0].CodiceProdotto != "9110001A" || pp[0].SorgenteDi != nil ||
		!strings.Contains(pp[0].Testo, "molto probabilmente lo STEP strutturale di 9110001A") {
		t.Fatalf("la proposta di sorgente in S9: %+v", pp)
	}
	t.Logf("evidenze: %+v", pp[0].Evidenze)
	dalNome := false
	for _, e := range pp[0].Evidenze {
		if e.Selettore == "nome_file" && e.BaseLetta == "9110001" && e.MarcatoreLetto == "" && e.EsitoMarcatore == "compatibile_parziale" {
			dalNome = true
		}
	}
	if !dalNome {
		t.Errorf("manca l'evidenza del nome del file (9110001, marcatore compatibile parziale): %+v", pp[0].Evidenze)
	}
	if len(righe) != 0 {
		t.Errorf("lo STEP candidato ha righe nel prodotto: %v", righe)
	}
	for _, d := range x.DaAssociare {
		if d.AllegatoID == step {
			t.Error("lo STEP candidato è da associare")
		}
	}
	var d struct {
		Fonte *struct {
			ProposteSorgente []proposta `json:"proposte_sorgente"`
		} `json:"fonte"`
	}
	leggi(base+"/motore-a/dati", &d)
	if d.Fonte == nil || len(d.Fonte.ProposteSorgente) != 1 || d.Fonte.ProposteSorgente[0].AllegatoID != step {
		t.Errorf("/dati.fonte: %+v", d.Fonte)
	}
	if diff := differenze(prima, fotoDelDatabase(t, b)); diff != "" {
		t.Errorf("le GET hanno scritto: %s", diff)
	}

	// S8: la conferma dell'utente; poi lo STEP è la sorgente, non più una proposta
	var gruppi struct {
		Gruppi []struct {
			Gruppo            string `json:"gruppo"`
			ImprontaContenuto string `json:"impronta_contenuto"`
		} `json:"gruppi"`
	}
	leggi(base+"/sorgente/step/"+step.String()+"/anteprima?prodotto="+s.prodotto.String(), &gruppi)
	gruppo, impronta := "", ""
	for _, g := range gruppi.Gruppi {
		if gruppo == "" && g.ImprontaContenuto != "" {
			gruppo, impronta = g.Gruppo, g.ImprontaContenuto
		}
	}
	corpo, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "partenza": "step", "gruppo": gruppo,
		"impronta_contenuto": impronta, "motivo": "proposta del motore confermata"})
	resp, testo := s.ad.chiediJSON(http.MethodPost, base+"/sorgente/step/"+step.String()+"/conferma-sorgente", string(corpo))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) {
		t.Fatalf("S8: %d %.1500s", resp.StatusCode, testo)
	}
	pp, righe = sezione(s9())
	if len(pp) != 0 || len(righe) != 1 || righe[0] != "legato_come_sorgente/conferma_s8" {
		t.Errorf("dopo S8: proposte %+v, righe dello STEP %v", pp, righe)
	}
}
