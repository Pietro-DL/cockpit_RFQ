package distintabozza

// L1 — ondata C di V1, corsia persistenza: le regole nuove della riconciliazione di R2.
//   - OB-08 (parte L1), DEC-B PB-34: una chiave tolta e rimessa per un nodo diverso → 422 sul campo della chiave;
//     «Includi di nuovo» sulla stessa casella con un'identita' diversa → passa; lo stesso con la chiave tolta da un
//     salvataggio precedente (Contesto3.NodiTolti, dal giornale).

import (
	"encoding/json"
	"testing"
)

// senzaChiave: lo snapshot senza il nodo, i suoi archi e le decisioni con quella chiave.
func senzaChiave(t *testing.T, s map[string]any, chiave string) map[string]any {
	t.Helper()
	c := copiaProfonda(t, s)
	var nn, aa, dd []any
	for _, n := range c["nodi"].([]any) {
		if n.(map[string]any)["chiave_bozza"] != chiave {
			nn = append(nn, n)
		}
	}
	for _, a := range c["archi"].([]any) {
		m := a.(map[string]any)
		if m["padre"] != chiave && m["figlio"] != chiave {
			aa = append(aa, a)
		}
	}
	for _, d := range c["decisioni"].([]any) {
		if d.(map[string]any)["chiave_bozza"] != chiave {
			dd = append(dd, d)
		}
	}
	c["nodi"], c["archi"], c["decisioni"] = nn, aa, dd
	return c
}

func TestV1ChiaveToltaNonTornaPerUnNodoDiverso(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	man := nodoIn3(base, "k-man")
	arco := map[string]any{"padre": "k-root", "figlio": "k-man", "qta": 4, "origine": "utente", "posizione": "Pos." + eAccento}
	senza := senzaChiave(t, base, "k-man")

	// un nodo manuale diverso con la stessa chiave, nello stesso salvataggio: 422 sulla chiave
	diverso := nodoManuale3(id, "k-man", "ACM-7777")
	conDiverso := copiaProfonda(t, senza)
	conDiverso["nodi"] = append(conDiverso["nodi"].([]any), diverso)
	_, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, conDiverso), []Azione{
		azione3(t, AzArcoTolto, "k-man", arco, nil),
		azione3(t, AzNodoTolto, "k-man", man, nil),
		azione3(t, AzNodoAggiunto, "k-man", nil, diverso),
	}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || e.Campo != "azioni[2].chiave_bozza" {
		t.Fatalf("chiave riusata per un nodo diverso: %+v", e)
	}
	// lo stesso con la chiave tolta da un salvataggio precedente
	_, err = Riconcilia3(leggiProva3(t, senza), leggiProva3(t, conDiverso), []Azione{azione3(t, AzNodoAggiunto, "k-man", nil, diverso)},
		Contesto3{N0: 6, NodiTolti: map[string][]json.RawMessage{"k-man": {rawDi(t, man)}}})
	if e := errore3(t, err); e.Campo != "azioni[0].chiave_bozza" {
		t.Fatalf("chiave tolta prima, riusata per un nodo diverso: %+v", e)
	}

	// «Includi di nuovo» di un nodo STEP sulla stessa casella, con un'identita' diversa (da decidere): passa
	step := nodoIn3(base, "k-0011")
	senzaStep := senzaChiave(t, base, "k-0011")
	ripreso := map[string]any{"chiave_bozza": "k-0011", "op": "crea", "tipo": "sciolto", "origine": "step", "caselle": step["caselle"],
		"identita": map[string]any{"emittente": id.clienteE(), "emittente_presunto": true, "modo": "da_decidere"}}
	conRipreso := copiaProfonda(t, senzaStep)
	conRipreso["nodi"] = append(conRipreso["nodi"].([]any), ripreso)
	if _, err := Riconcilia3(leggiProva3(t, senzaStep), leggiProva3(t, conRipreso), []Azione{azione3(t, AzNodoAggiunto, "k-0011", nil, ripreso)},
		Contesto3{N0: 8, NodiTolti: map[string][]json.RawMessage{"k-0011": {rawDi(t, step)}}}); err != nil {
		t.Fatalf("Includi di nuovo sulla stessa casella: %v", err)
	}
	// ChiaveRiusabile: un registrato con lo stesso componente e' lo stesso nodo; senza casella ne' componente mai
	reg := Nodo3{ChiaveBozza: "k-x", ComponenteID: &id.prodotto}
	if !ChiaveRiusabile(rawDi(t, map[string]any{"chiave_bozza": "k-x", "componente_id": id.prodotto.String()}), reg) {
		t.Fatal("registrato con lo stesso componente")
	}
	if ChiaveRiusabile(rawDi(t, man), Nodo3{ChiaveBozza: "k-man"}) {
		t.Fatal("nodo manuale rimesso a mano con la stessa chiave")
	}
}
