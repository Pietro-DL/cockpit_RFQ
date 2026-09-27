package fascicolo

// «Quasi uguale» (Smistamento, addendum A5.5 P4): due codici che una persona puo' confondere, o che sono
// lo stesso pezzo scritto in un altro modo. `7120001A` accanto a `7120001`, `7120012_F2` accanto a
// `7120012`, `007120001` accanto a `7120001`. Il caso che previene e' un pezzo creato due volte con due
// codici: chi scrive un codice quasi uguale a uno che la RFQ ha gia' deve dire esplicitamente che e' un
// pezzo diverso.
//
// Due codici della stessa numerazione che differiscono per una cifra NON sono vicini: `7120001` e
// `7120010`, `7120011` e `7120012` sono pezzi diversi della stessa distinta, e chiedere ogni volta «sei
// sicuro?» su codici progressivi insegnerebbe soltanto a rispondere si' senza leggere. Per questo le regole
// della bozza su «distanza 1» e «cifre adiacenti scambiate» non ci sono: rendevano vicini proprio i due casi
// negativi che la stessa P4 fissa in prova.

import (
	"strings"
	"unicode"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// Motivi di QuasiUguale: dicono perche' due codici sono vicini, con le parole della schermata.
const (
	VicinoSuffissoCliente = "stesso codice senza il suffisso decorativo del cliente"
	VicinoSeparatori      = "stesso codice senza i separatori"
	VicinoZeri            = "stesso codice senza gli zeri in testa"
	VicinoLetteraFinale   = "lettera finale"
	VicinoSuffisso        = "separatore e suffisso"
)

// minVicino e' la lunghezza minima del codice piu' corto perche' un prefisso conti: sotto, «A1» sarebbe
// vicino a «A1-B» per caso.
const minVicino = 4

// QuasiUguale dice se a e b sono due codici diversi che una persona puo' confondere, e perche'. Due codici
// uguali (senza distinguere le maiuscole) non sono «quasi» uguali: sono lo stesso, e lo dice chi chiama.
// Pura, simmetrica. m sono le regole del cliente (i suffissi decorativi); puo' essere nil.
//
// Vicini, nell'ordine in cui si controllano:
//   - lo stesso codice senza il suffisso decorativo del cliente (`7120001_PRT`, `7120001`);
//   - lo stesso codice senza i separatori `-_./` e gli spazi (`712-0001`, `7120001`);
//   - lo stesso codice senza gli zeri in testa (`007120001`, `7120001`);
//   - uno e' l'altro con 1-2 lettere attaccate in fondo, dopo una cifra (`7120001A`, `7120001`);
//   - uno e' l'altro con un separatore e un suffisso di 1-4 lettere o cifre (`7120012_F2`, `7120001_1`,
//     `7120001-A`, `7120001_PRT`).
func QuasiUguale(a, b string, m *classificazione.Motore) (bool, string) {
	a, b = strings.ToUpper(strings.TrimSpace(a)), strings.ToUpper(strings.TrimSpace(b))
	if a == "" || b == "" || a == b {
		return false, ""
	}
	if m.HaSuffissi() {
		ca, _ := m.Canonico(a, "")
		cb, _ := m.Canonico(b, "")
		if strings.EqualFold(ca, cb) {
			return true, VicinoSuffissoCliente
		}
	}
	sa, sb := senzaSeparatori(a), senzaSeparatori(b)
	if sa != "" && sa == sb {
		return true, VicinoSeparatori
	}
	if za, zb := strings.TrimLeft(sa, "0"), strings.TrimLeft(sb, "0"); za != "" && za == zb {
		return true, VicinoZeri
	}
	corto, lungo := a, b
	if len(corto) > len(lungo) {
		corto, lungo = lungo, corto
	}
	if len(corto) < minVicino || !strings.HasPrefix(lungo, corto) {
		return false, ""
	}
	resto := lungo[len(corto):]
	ultima := rune(corto[len(corto)-1])
	switch {
	case len(resto) <= 2 && soloLettere(resto) && unicode.IsDigit(ultima):
		return true, VicinoLetteraFinale
	case len(resto) >= 2 && len(resto) <= 5 && separatore(rune(resto[0])) && soloAlfanumerici(resto[1:]):
		return true, VicinoSuffisso
	}
	return false, ""
}

// Vicino e' un codice della RFQ quasi uguale a quello scritto, con il motivo.
type Vicino struct {
	Codice string `json:"codice"`
	Motivo string `json:"motivo"`
}

// Vicini sono i codici fra quelli dati quasi uguali a codice, ciascuno una volta, nell'ordine dato. Uno
// uguale non c'e': e' lo stesso codice, non un vicino.
func Vicini(codice string, codici []string, m *classificazione.Motore) []Vicino {
	var out []Vicino
	visti := map[string]bool{}
	for _, c := range codici {
		k := strings.ToUpper(strings.TrimSpace(c))
		if visti[k] {
			continue
		}
		visti[k] = true
		if ok, motivo := QuasiUguale(codice, c, m); ok {
			out = append(out, Vicino{Codice: strings.TrimSpace(c), Motivo: motivo})
		}
	}
	return out
}

func separatore(r rune) bool { return r == '-' || r == '_' || r == '.' || r == '/' }

func senzaSeparatori(s string) string {
	return strings.Map(func(r rune) rune {
		if separatore(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func soloLettere(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return s != ""
}

func soloAlfanumerici(s string) bool {
	for _, r := range s {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return s != ""
}
