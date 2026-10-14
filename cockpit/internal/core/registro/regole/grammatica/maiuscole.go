package grammatica

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
)

// maiuscole.go: come un letterale o un pattern della base diventa un pezzo di regex, con le maiuscole dichiarate
// nella base (T9). Stava in motorea (piani.go); dal 14/10 sta qui perché lo usano due pacchetti con la stessa
// regola: il compilatore di motorea, che scrive le regex del riconoscimento, e la validazione della tabella delle
// classi (classi.go), che controlla che un valore sia riconosciuto dal segmento come il motore lo compila. Una
// funzione sola: le due regole non possono divergere. Qui non si compila niente per il riconoscimento: si scrive il
// testo della regex, e chi la compila resta motorea.

// LetteraleBase: un letterale della base (segmento o separatore) come pezzo di regex. Con le maiuscole indifferenti
// (piega) ogni lettera ASCII diventa la classe delle sue due maiuscole; le rune fuori dall'ASCII restano come sono.
func LetteraleBase(s string, piega bool) string {
	if !piega {
		return regexp.QuoteMeta(s)
	}
	var b strings.Builder
	for _, r := range s {
		if altra, ok := AltraMaiuscola(r); ok {
			fmt.Fprintf(&b, "[%c%c]", min(r, altra), max(r, altra))
		} else {
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// PatternBase: un pattern della base come pezzo di regex, in un gruppo non catturante. Le maiuscole indifferenti si
// ottengono aggiungendo alle lettere ASCII l'altra maiuscola, mai con (?i), che piega anche lettere fuori dall'ASCII
// (T9).
func PatternBase(p string, piega bool) (string, error) {
	if !piega {
		return "(?:" + p + ")", nil
	}
	albero, err := syntax.Parse(p, syntax.Perl)
	if err != nil {
		return "", err
	}
	return "(?:" + PiegaASCII(albero).String() + ")", nil
}

// AltraMaiuscola: per una lettera ASCII, la stessa lettera nell'altra maiuscola.
func AltraMaiuscola(r rune) (rune, bool) {
	switch {
	case 'a' <= r && r <= 'z':
		return r - ('a' - 'A'), true
	case 'A' <= r && r <= 'Z':
		return r + ('a' - 'A'), true
	}
	return 0, false
}

// PiegaASCII: lo stesso albero, con le lettere ASCII dei letterali e delle classi in tutte e due le maiuscole. Le
// rune fuori dall'ASCII restano come sono: la K del Kelvin non diventa una k (T9).
func PiegaASCII(re *syntax.Regexp) *syntax.Regexp {
	switch re.Op {
	case syntax.OpLiteral:
		subs := make([]*syntax.Regexp, 0, len(re.Rune))
		for _, r := range re.Rune {
			if altra, ok := AltraMaiuscola(r); ok {
				subs = append(subs, &syntax.Regexp{Op: syntax.OpCharClass, Rune: []rune{min(r, altra), min(r, altra), max(r, altra), max(r, altra)}})
			} else {
				subs = append(subs, &syntax.Regexp{Op: syntax.OpLiteral, Rune: []rune{r}})
			}
		}
		if len(subs) == 1 {
			return subs[0]
		}
		return &syntax.Regexp{Op: syntax.OpConcat, Sub: subs}
	case syntax.OpCharClass:
		return &syntax.Regexp{Op: syntax.OpCharClass, Rune: piegaClasse(re.Rune)}
	}
	n := *re
	if len(re.Sub) > 0 {
		n.Sub = make([]*syntax.Regexp, len(re.Sub))
		for i, s := range re.Sub {
			n.Sub[i] = PiegaASCII(s)
		}
	}
	return &n
}

// piegaClasse: gli intervalli di una classe, più quelli delle lettere ASCII con l'altra maiuscola, ordinati e fusi.
func piegaClasse(intervalli []rune) []rune {
	tutti := append([]rune(nil), intervalli...)
	for i := 0; i+1 < len(intervalli); i += 2 {
		lo, hi := intervalli[i], intervalli[i+1]
		if a, b := max(lo, 'a'), min(hi, 'z'); a <= b {
			tutti = append(tutti, a-('a'-'A'), b-('a'-'A'))
		}
		if a, b := max(lo, 'A'), min(hi, 'Z'); a <= b {
			tutti = append(tutti, a+('a'-'A'), b+('a'-'A'))
		}
	}
	coppie := make([][2]rune, 0, len(tutti)/2)
	for i := 0; i+1 < len(tutti); i += 2 {
		coppie = append(coppie, [2]rune{tutti[i], tutti[i+1]})
	}
	sort.Slice(coppie, func(i, j int) bool { return coppie[i][0] < coppie[j][0] })
	var out []rune
	for _, c := range coppie {
		if n := len(out); n > 0 && c[0] <= out[n-1]+1 {
			out[n-1] = max(out[n-1], c[1])
			continue
		}
		out = append(out, c[0], c[1])
	}
	return out
}
