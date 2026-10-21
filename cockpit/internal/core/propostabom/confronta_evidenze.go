package propostabom

import (
	"bytes"
	"sort"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Le differenze delle evidenze fra due registri (U2 §5.2; DEC-B PD-05: differenze.evidenze[]): informative, con le
// letture nuove o cambiate dei file candidati, i riferimenti documentali della radice e gli altri campi di evidenza.
// Si calcolano sulla proiezione di impronta_evidenze della foglia, mai con una copia dell'elenco chiuso, e non cambiano
// le impronte: niente 409, niente PROPOSTA_CAMBIATA.

// differenzeEvidenze: le evidenze che cambiano fra due registri (DifferenzaEvidenza), in ordine di voce. Un contenuto che
// non si proietta (non capita con registri letti dalla foglia) non dà differenze di evidenza: sono solo informazione.
func differenzeEvidenze(vecchio, nuovo importazione.Contenuto) []DifferenzaEvidenza {
	pv, err1 := proiezioneEvidenze(vecchio)
	pn, err2 := proiezioneEvidenze(nuovo)
	if err1 != nil || err2 != nil {
		return nil
	}
	var out []DifferenzaEvidenza
	for _, parte := range []string{"fonte", "gruppo"} {
		if campi := campiCambiati(oggetto(pv[parte]), oggetto(pn[parte])); len(campi) > 0 {
			out = append(out, DifferenzaEvidenza{Voce: parte, Tipo: DifferenzaCambiato, Campi: campi})
		}
	}
	if !stessoValore(pv["diagnostiche"], pn["diagnostiche"]) {
		out = append(out, DifferenzaEvidenza{Voce: "diagnostiche", Tipo: DifferenzaCambiato})
	}
	for _, parte := range []struct {
		nome       string
		soloCambio bool // un elemento che entra o esce lo dicono già le differenze dei nodi
	}{{"nodi", true}, {"fuori_per_decisione", true}, {"file", false}, {"relazioni", false}} {
		a, b := perVoce(pv[parte.nome]), perVoce(pn[parte.nome])
		chiavi := map[string]bool{}
		for k := range a {
			chiavi[k] = true
		}
		for k := range b {
			chiavi[k] = true
		}
		for _, k := range ordinate(chiavi) {
			x, inA := a[k]
			y, inB := b[k]
			switch {
			case inA && inB:
				if campi := campiCambiati(x, y); len(campi) > 0 {
					out = append(out, DifferenzaEvidenza{Voce: k, Tipo: DifferenzaCambiato, Campi: campi})
				}
			case parte.soloCambio:
			case inB:
				out = append(out, DifferenzaEvidenza{Voce: k, Tipo: DifferenzaAggiunto})
			default:
				out = append(out, DifferenzaEvidenza{Voce: k, Tipo: DifferenzaTolto})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Voce < out[j].Voce })
	return out
}

func proiezioneEvidenze(c importazione.Contenuto) (map[string]any, error) {
	b, err := jsoncanonico.Codifica(c)
	if err != nil {
		return nil, err
	}
	return importazione.ProiezioneEvidenze(b)
}

func oggetto(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// perVoce: gli elementi di un elenco della proiezione per la loro chiave «voce» (una voce, un file, una relazione).
func perVoce(v any) map[string]map[string]any {
	out := map[string]map[string]any{}
	l, _ := v.([]any)
	for _, x := range l {
		m, ok := x.(map[string]any)
		if !ok {
			continue
		}
		if k, ok := m["voce"].(string); ok && k != "" {
			out[k] = m
		}
	}
	return out
}

// campiCambiati: i campi (salvo «voce») con un valore diverso, o presenti da una parte sola, in ordine.
func campiCambiati(a, b map[string]any) []string {
	tutti := map[string]bool{}
	for k := range a {
		tutti[k] = true
	}
	for k := range b {
		tutti[k] = true
	}
	var out []string
	for _, k := range ordinate(tutti) {
		if k == "voce" {
			continue
		}
		if !stessoValore(a[k], b[k]) {
			out = append(out, k)
		}
	}
	return out
}

func stessoValore(a, b any) bool {
	x, err1 := jsoncanonico.Codifica(a)
	y, err2 := jsoncanonico.Codifica(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}
