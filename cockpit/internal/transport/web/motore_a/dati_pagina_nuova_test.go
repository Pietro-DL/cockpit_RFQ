package motore_a

// OB-18 (decisioni dell'ondata B, DEC-B §6.3, richiesta RQ-I1 della corsia motore), la metà del server: i campi che la
// pagina nuova della Distinta unica legge da /dati (modelloDa, web/static/distinta_unica.mjs) stanno dove lei li cerca
// nel JSON vero della pagina (ComponiPagina sull'esito ACME delle prove): «esito» è l'esito del thread (EsitoThread),
// con prodotti_valutati[] (rif, componente_id, codice_richiesto, bom.verificata), cliente_id, fascicolo.congelato e
// associazioni[]; l'esito con thread[] che l'ondata A leggeva non c'è. L'altra metà è la L1 statica della pagina
// (TestDistintaUnicaModelloDaLeggeIlModelloVero, transport/web). File di sola prova della corsia interfaccia, aggancio
// minimo nel pacchetto del motore.

import (
	"encoding/json"
	"testing"
)

func TestDatiIlModelloCheLaPaginaNuovaLegge(t *testing.T) {
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
	if esito == nil {
		t.Fatalf("/dati senza esito: %.300s", b)
	}
	if _, ok := esito["thread"]; ok {
		t.Error("esito ha thread[]: la pagina legge l'esito del thread, non l'esito con i thread")
	}
	if c, _ := esito["cliente_id"].(string); c == "" {
		t.Error("esito senza cliente_id")
	}
	if f, _ := esito["fascicolo"].(map[string]any); f == nil {
		t.Error("esito senza fascicolo")
	} else if _, ok := f["congelato"].(bool); !ok {
		t.Error("esito.fascicolo senza congelato")
	}
	pv, _ := esito["prodotti_valutati"].([]any)
	if len(pv) == 0 {
		t.Fatal("esito senza prodotti_valutati")
	}
	for i, x := range pv {
		pr, _ := x.(map[string]any)
		for _, k := range []string{"rif", "componente_id", "codice_richiesto"} {
			if s, _ := pr[k].(string); s == "" {
				t.Errorf("prodotti_valutati[%d] senza %s", i, k)
			}
		}
		bom, _ := pr["bom"].(map[string]any)
		if _, ok := bom["verificata"].(bool); !ok {
			t.Errorf("prodotti_valutati[%d] senza bom.verificata (il calcolo di oggi della verifica, U2 §5.4)", i)
		}
	}
	if as, ok := esito["associazioni"]; ok {
		if _, lista := as.([]any); !lista {
			t.Errorf("esito.associazioni non è un elenco: %T", as)
		}
	}
	for _, k := range []string{"thread_id", "testa", "errore", "calcolato"} {
		if _, ok := j[k]; !ok {
			t.Errorf("/dati senza %s", k)
		}
	}
}
