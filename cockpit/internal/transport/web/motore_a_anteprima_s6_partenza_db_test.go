//go:build integrazione

// L4 — messaggio 46 [U] (21/10, «le anteprime di quando si conferma uno step non compaiono»), collaudo UI46, rilievo B1:
// S6 dà proposta.stato_di_partenza, la versione 1 che la partenza step costruirebbe dal registro della proposta, così la
// pagina disegna l'albero proposto nell'anteprima e nel dialogo di S8. Su 342010f S6 dava solo proposta.{gruppo,
// impronta_contenuto, contenuto} e la pagina diceva «Scegli una struttura dello STEP per vederla.» anche con la
// candidatura univoca in vista. La scena è la B87 di motore_a_db_test.go con il calcolo vero del Motore A e le regole ACME.
//
// I codici 777xxxxx sono quelli della scena B87; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestL4UI46S6DaLoStatoDiPartenza(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MS6P")
	mwAccendi(t, b, s.regole)
	s.analisiCorrente(s.shaDoc(s.step), fattiDi("#1", []string{"#1=77722757", "#2=9120101A", "#3=9130202A"}, []string{"#1>#2*2", "#2>#3*3"}))
	s6 := func(extra string) map[string]json.RawMessage {
		t.Helper()
		p := "/thread/" + s.thread.String() + "/sorgente/step/" + s.allStep.String() + "/anteprima?prodotto=" + s.prodotto.String() + extra
		resp, corpo := s.ad.fai(http.MethodGet, p, nil, false)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %.800s", p, resp.StatusCode, corpo)
		}
		var x map[string]json.RawMessage
		if err := json.Unmarshal([]byte(corpo), &x); err != nil {
			t.Fatalf("%v: %.300s", err, corpo)
		}
		return x
	}
	conteggio := contaSQL(t, b, `SELECT (SELECT count(*) FROM distinta_bozza) + (SELECT count(*) FROM distinta_importazione) + (SELECT count(*) FROM fonte_step_evento)`)
	var gruppi []struct {
		Gruppo            string `json:"gruppo"`
		ImprontaContenuto string `json:"impronta_contenuto"`
	}
	if err := json.Unmarshal(s6("")["gruppi"], &gruppi); err != nil {
		t.Fatal(err)
	}
	gruppo := ""
	for _, g := range gruppi {
		if gruppo == "" && g.ImprontaContenuto != "" {
			gruppo = g.Gruppo
		}
	}
	if gruppo == "" {
		t.Fatalf("lo STEP non ha un gruppo importabile: %+v", gruppi)
	}
	var proposta struct {
		Gruppo          string `json:"gruppo"`
		StatoDiPartenza *struct {
			Formato    int    `json:"formato"`
			ProdottoID string `json:"prodotto_id"`
			Sorgente   *struct {
				AllegatoID string `json:"allegato_id"`
				Gruppo     string `json:"gruppo"`
			} `json:"sorgente"`
			Nodi []struct {
				ChiaveBozza  string  `json:"chiave_bozza"`
				ComponenteID *string `json:"componente_id"`
				Caselle      []struct {
					Rif string `json:"rif"`
				} `json:"caselle"`
				Identita struct {
					Codice string `json:"codice"`
					Modo   string `json:"modo"`
				} `json:"identita"`
			} `json:"nodi"`
			Archi []struct {
				Padre  string `json:"padre"`
				Figlio string `json:"figlio"`
				Qta    int    `json:"qta"`
			} `json:"archi"`
		} `json:"stato_di_partenza"`
	}
	if err := json.Unmarshal(s6("&gruppo=" + url.QueryEscape(gruppo))["proposta"], &proposta); err != nil {
		t.Fatal(err)
	}
	sp := proposta.StatoDiPartenza
	if sp == nil {
		t.Fatalf("S6 non dà proposta.stato_di_partenza: la pagina non ha l'albero da mostrare (collaudo UI46, B1)")
	}
	if sp.Formato != 3 || sp.ProdottoID != s.prodotto.String() || sp.Sorgente == nil || sp.Sorgente.AllegatoID != s.allStep.String() || sp.Sorgente.Gruppo != gruppo {
		t.Errorf("lo stato di partenza non è la partenza step di questo STEP e di questo gruppo: formato %d, prodotto %s, sorgente %+v", sp.Formato, sp.ProdottoID, sp.Sorgente)
	}
	radice, codici := "", map[string]string{}
	for _, n := range sp.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == s.prodotto.String() {
			radice = n.ChiaveBozza
		}
		codici[n.ChiaveBozza] = n.Identita.Codice
	}
	if radice == "" || len(sp.Nodi) != 3 {
		t.Fatalf("lo stato di partenza vuole il prodotto come radice e i tre pezzi dello STEP: radice %q, nodi %d", radice, len(sp.Nodi))
	}
	figli := map[string]int{}
	for _, a := range sp.Archi {
		figli[codici[a.Padre]+">"+codici[a.Figlio]] = a.Qta
	}
	if len(sp.Archi) != 2 || figli[codici[radice]+">9120101A"] != 2 || figli["9120101A>9130202A"] != 3 {
		t.Errorf("gli archi dello stato di partenza non sono quelli dello STEP: %v", figli)
	}
	if n := contaSQL(t, b, `SELECT (SELECT count(*) FROM distinta_bozza) + (SELECT count(*) FROM distinta_importazione) + (SELECT count(*) FROM fonte_step_evento)`); n != conteggio {
		t.Errorf("S6 ha scritto: bozze, registri ed eventi della sorgente da %d a %d", conteggio, n)
	}
}
