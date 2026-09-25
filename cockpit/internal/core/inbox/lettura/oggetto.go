package lettura

import "regexp"

// S1 — l'etichetta «[EXTERNAL]» nell'oggetto. La mette il server davanti all'oggetto, dopo i
// prefissi RE:/FW: che già c'erano; si toglie solo lì, fra parentesi quadre, e solo nelle sue
// forme note: «Offerta [EXT-2]» è un oggetto scritto da qualcuno e resta com'è.
var reEsternoOggetto = regexp.MustCompile(`(?i)^((?:\s*(?:re|r|fw|fwd|i|tr|aw|wg)\s*:)*\s*)\[\s*(?:external|ext|extern|esterno|esterna|external\s+(?:email|sender)|e-?mail\s+esterna|mail\s+esterna)\s*\]\s*`)

// OggettoVisibile toglie l'etichetta di posta esterna dall'oggetto, per la sola vista (l'oggetto
// memorizzato non cambia), e dice se c'era.
func OggettoVisibile(oggetto string) (visibile string, esterno bool) {
	s := perVista(oggetto)
	// Più di un'etichetta capita (una per ogni passaggio da un server che la mette); oltre venti
	// l'oggetto è costruito apposta e il resto si lascia com'è.
	for range 20 {
		m := reEsternoOggetto.FindStringSubmatchIndex(s)
		if m == nil {
			break
		}
		s = s[:m[3]] + s[m[1]:]
		esterno = true
	}
	if !esterno {
		return oggetto, false
	}
	return s, true
}
