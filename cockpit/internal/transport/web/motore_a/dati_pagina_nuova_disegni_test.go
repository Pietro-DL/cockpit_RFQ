package motore_a

// Messaggio 46 dell'utente (21/10), corsia ALBERO, la metà del server: le anteprime dei 2D nei nodi dell'organigramma
// della Distinta unica (disegniDelProdotto, web/static/distinta_unica.mjs) si leggono dal JSON vero di /dati, dove la
// pagina di prima le prendeva: esito.prodotti_valutati[].nodi[] (e strutture_candidate[].nodi[], confermati_senza_step[])
// con nodo.rif nella forma della casella della bozza (nodo:<sha>:<chiave>), nodo.decisione.componente_id e
// disegni.primario con nome_file e anteprima {allegato_id, disponibile}. File di sola prova della corsia interfaccia.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDatiIDisegniCheLOrganigrammaLegge(t *testing.T) {
	p := ComponiPagina(maRisultatoACME(), RichiestaPagina{Thread: maUUID(1), Scrive: true})
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var j map[string]any
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	esito, _ := j["esito"].(map[string]any)
	pv, _ := esito["prodotti_valutati"].([]any)
	if len(pv) == 0 {
		t.Fatal("esito senza prodotti_valutati")
	}
	trovati := 0
	for _, x := range pv {
		pr, _ := x.(map[string]any)
		nodi, _ := pr["nodi"].([]any)
		for _, s := range elenco(pr["strutture_candidate"]) {
			sm, _ := s.(map[string]any)
			nodi = append(nodi, elenco(sm["nodi"])...)
		}
		nodi = append(nodi, elenco(pr["confermati_senza_step"])...)
		for _, y := range nodi {
			n, _ := y.(map[string]any)
			nodo, _ := n["nodo"].(map[string]any)
			rif, _ := nodo["rif"].(string)
			if !strings.HasPrefix(rif, "nodo:") && !strings.HasPrefix(rif, "componente:") {
				t.Errorf("nodo.rif %q: la pagina lo cerca come la casella della bozza (nodo:<sha>:<chiave>) o come componente:<uuid>", rif)
			}
			dis, _ := n["disegni"].(map[string]any)
			pri, _ := dis["primario"].(map[string]any)
			if pri == nil {
				continue
			}
			an, _ := pri["anteprima"].(map[string]any)
			if _, ok := an["disponibile"].(bool); !ok {
				t.Errorf("%s: disegni.primario.anteprima senza disponibile: %v", rif, pri)
			}
			if a, _ := an["allegato_id"].(string); an["disponibile"] == true && len(a) != 36 {
				t.Errorf("%s: anteprima disponibile senza allegato_id: %v", rif, an)
			}
			if _, ok := pri["nome_file"].(string); !ok {
				t.Errorf("%s: disegni.primario senza nome_file", rif)
			}
			if d, _ := nodo["decisione"].(map[string]any); d != nil {
				if c, _ := d["componente_id"].(string); len(c) != 36 {
					t.Errorf("%s: nodo.decisione senza componente_id: %v", rif, d)
				}
			}
			trovati++
		}
	}
	if trovati == 0 {
		t.Fatal("nessun nodo con disegni.primario nel JSON di /dati della scena ACME: la prova non prova niente")
	}
}

func elenco(v any) []any {
	l, _ := v.([]any)
	return l
}
