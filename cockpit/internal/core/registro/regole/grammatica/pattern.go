package grammatica

import (
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"
	"unicode"
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// VerificaPattern controlla un pattern RE2 di una grammatica: un segmento della base, una decorazione, un
// segmento o un token di una revisione. Il pattern è un pezzo limitato: il compilatore di motorea lo mette
// dentro gruppi che genera lui, con i confini e le maiuscole dichiarati nella base. Per questo vieta (v3
// §4.3; T1-T12, T16, T20):
//   - gli operatori che RE2 non ha: lookaround, backreference, gruppi atomici, ripetizioni possessive;
//   - \b e \B; le ancore ^, $, \A, \z;
//   - qualunque gruppo, anche (?:…) e con nome; i flag in linea come (?i);
//   - «.» e le classi negate; le classi con caratteri fuori dall'ASCII, come le classi Unicode;
//   - «|»: le alternative si scrivono come letterali;
//   - *, + e {n,}; {n,m} con m oltre max_ripetizione;
//   - un pattern che può riconoscere la stringa vuota;
//   - i caratteri di controllo o di formato e gli spazi, compreso il backspace che un «\b» scritto nel JSON
//     diventa, anche quando li scrive un escape del pattern (\t, \s, \x00, \x{a0}).
//
// L'analisi è quella di prendeIlVuoto e puoEssereVuota del motore legacy (core/registro/regole), copiata e
// resa più severa, non importata (G9). Una parte dei divieti si legge sul testo del pattern, perché
// regexp/syntax semplifica: (?:a) diventa a, (?i) sparisce nei flag delle lettere, a|b diventa [ab].
// Ogni codice compare al più una volta per pattern; le diagnostiche hanno un ordine fisso. I limiti li
// riceve, non li sceglie (R43 B). nil vuol dire valido.
func VerificaPattern(percorso, pattern string, lim Limiti) []evidenze.Diagnostica {
	v := &verificaPattern{percorso: percorso, lim: lim}
	v.verifica(pattern)
	return v.out
}

type verificaPattern struct {
	percorso string
	lim      Limiti
	out      []evidenze.Diagnostica
	visti    []string // i codici già dati: uno per codice
}

// dai aggiunge una diagnostica, se il suo codice non c'è già.
func (v *verificaPattern) dai(d evidenze.Diagnostica) {
	if in(d.Codice, v.visti) {
		return
	}
	v.visti = append(v.visti, d.Codice)
	d.Percorso = v.percorso
	if d.Gravita == "" {
		d.Gravita = evidenze.GravitaErrore
	}
	if d.Natura == "" {
		d.Natura = evidenze.NaturaContratto
	}
	v.out = append(v.out, d)
}

func (v *verificaPattern) verifica(pattern string) {
	if pattern == "" {
		v.dai(evidenze.Diagnostica{Codice: CodicePatternVuoto, Messaggio: "pattern vuoto: riconoscerebbe la stringa vuota"})
		return
	}
	if massimo := v.lim.Grammatica.MaxLunghezzaPattern; len(pattern) > massimo {
		v.dai(evidenze.Diagnostica{
			Codice:    CodiceLimiteSuperato,
			Natura:    evidenze.NaturaLimite,
			Messaggio: fmt.Sprintf("pattern di %d byte: max_lunghezza_pattern dell'indice è %d", len(pattern), massimo),
		})
		return // un pattern oltre il limite non si analizza: il limite protegge anche il tempo del parser
	}
	if strings.ContainsFunc(pattern, vietatoNelPattern) {
		// Prima del parser: un backspace o uno spazio passerebbero come letterali qualunque.
		v.dai(evidenze.Diagnostica{Codice: CodiceCarattereDiControllo, Messaggio: messaggioControllo("pattern", pattern)})
		return
	}
	albero, err := syntax.Parse(pattern, syntax.Perl) // le stesse regole di regexp.Compile
	if err != nil {
		v.erroreDelParser(err)
		return
	}
	v.testo(pattern)
	v.albero(albero)
	if puoEssereVuota(albero) {
		v.dai(evidenze.Diagnostica{Codice: CodicePatternVuoto, Messaggio: "il pattern può riconoscere la stringa vuota: ogni segmento legge almeno un carattere"})
	}
}

// erroreDelParser distingue gli operatori che RE2 non ha da un pattern che semplicemente non compila.
func (v *verificaPattern) erroreDelParser(err error) {
	var se *syntax.Error
	if !errors.As(err, &se) {
		v.dai(evidenze.Diagnostica{Codice: CodicePatternNonCompila, Messaggio: err.Error()})
		return
	}
	nonRE2 := false
	switch se.Code {
	case syntax.ErrInvalidPerlOp: // (?= (?! (?> e simili
		nonRE2 = true
	case syntax.ErrInvalidNamedCapture: // (?<= (?<! (?P= : lookbehind e backreference con nome
		nonRE2 = strings.HasPrefix(se.Expr, "(?<=") || strings.HasPrefix(se.Expr, "(?<!") || strings.HasPrefix(se.Expr, "(?P=")
	case syntax.ErrInvalidEscape: // \1 … \9, \k<nome>, \g{n}: backreference
		nonRE2 = len(se.Expr) == 2 && strings.ContainsAny(se.Expr[1:], "123456789kg")
	case syntax.ErrInvalidRepeatOp: // a++ a*+ a?+ a{2}+: ripetizioni possessive
		nonRE2 = strings.HasSuffix(se.Expr, "+") && len(se.Expr) >= 2
	case syntax.ErrInvalidRepeatSize: // {1001}: oltre il massimo di regexp/syntax, quindi oltre ogni limite
		v.dai(evidenze.Diagnostica{
			Codice:    CodicePatternRipetizioneOltreLimite,
			Messaggio: fmt.Sprintf("ripetizione %s oltre il limite: max_ripetizione dell'indice è %d", se.Expr, v.lim.Grammatica.MaxRipetizione),
		})
		return
	}
	if nonRE2 {
		v.dai(evidenze.Diagnostica{Codice: CodicePatternOperatoreNonRE2, Messaggio: fmt.Sprintf("%s: RE2 non ha lookaround, backreference, gruppi atomici né ripetizioni possessive", se.Expr)})
		return
	}
	v.dai(evidenze.Diagnostica{Codice: CodicePatternNonCompila, Messaggio: fmt.Sprintf("il pattern non compila: %s: %s", se.Code, se.Expr)})
}

// testo legge sul testo del pattern ciò che il parser semplifica: i gruppi, i flag, l'alternanza. Salta gli
// escape, le classi tra parentesi quadre e i tratti \Q…\E, dove «(» e «|» sono caratteri qualunque.
func (v *verificaPattern) testo(p string) {
	for i := 0; i < len(p); {
		switch c := p[i]; {
		case c == '\\':
			if strings.HasPrefix(p[i:], `\Q`) {
				if fine := strings.Index(p[i+2:], `\E`); fine >= 0 {
					i += 2 + fine + 2
				} else {
					i = len(p)
				}
				continue
			}
			i++
			if i < len(p) {
				_, n := utf8.DecodeRuneInString(p[i:])
				i += n
			}
		case c == '[':
			i = fineClasse(p, i)
		case c == '(':
			v.gruppo(p[i:])
			i++
		case c == '|':
			v.dai(evidenze.Diagnostica{Codice: CodicePatternAlternanza, Messaggio: "«|» nel pattern: le alternative si scrivono come letterali"})
			i++
		default:
			i++
		}
	}
}

// gruppo classifica un «(»: un flag in linea «(?i)», un gruppo con flag «(?i:…)», un gruppo qualunque.
func (v *verificaPattern) gruppo(da string) {
	if strings.HasPrefix(da, "(?") {
		j := 2
		for j < len(da) && strings.IndexByte("imsU-", da[j]) >= 0 {
			j++
		}
		if j > 2 && j < len(da) && (da[j] == ')' || da[j] == ':') {
			v.dai(evidenze.Diagnostica{Codice: CodicePatternFlag, Messaggio: "flag in linea nel pattern: le maiuscole si dichiarano nella base (maiuscole), mai con (?i)"})
			if da[j] == ')' {
				return
			}
		}
	}
	v.dai(evidenze.Diagnostica{Codice: CodicePatternGruppo, Messaggio: "gruppo nel pattern: i gruppi li genera il compilatore, anche (?:…) è vietato"})
}

// fineClasse restituisce l'indice dopo la «]» che chiude la classe che comincia in i. Una «]» subito dopo
// «[» o «[^» è un carattere; «[:alpha:]» dentro la classe è un pezzo solo.
func fineClasse(p string, i int) int {
	j := i + 1
	if j < len(p) && p[j] == '^' {
		j++
	}
	if j < len(p) && p[j] == ']' {
		j++
	}
	for j < len(p) {
		switch {
		case p[j] == '\\':
			j += 2
		case strings.HasPrefix(p[j:], "[:"):
			if fine := strings.Index(p[j+2:], ":]"); fine >= 0 {
				j += 2 + fine + 2
			} else {
				j++
			}
		case p[j] == ']':
			return j + 1
		default:
			j++
		}
	}
	return len(p)
}

// albero visita l'albero del parser.
func (v *verificaPattern) albero(re *syntax.Regexp) {
	switch re.Op {
	case syntax.OpCapture:
		v.dai(evidenze.Diagnostica{Codice: CodicePatternGruppo, Messaggio: "gruppo nel pattern: i gruppi li genera il compilatore"})
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		v.dai(evidenze.Diagnostica{Codice: CodicePatternPunto, Messaggio: "«.» nel pattern: si scrive la classe dei caratteri ammessi"})
	case syntax.OpCharClass:
		// Una classe negata (anche \D, \W, \S, \P{…}) arriva fino all'ultimo carattere Unicode.
		// Una classe negata prende anche i controlli: le basta il suo codice.
		if len(re.Rune) > 0 && re.Rune[len(re.Rune)-1] == unicode.MaxRune {
			v.dai(evidenze.Diagnostica{Codice: CodicePatternPunto, Messaggio: "classe negata nel pattern: si scrive la classe dei caratteri ammessi"})
		} else {
			if len(re.Rune) > 0 && re.Rune[len(re.Rune)-1] > unicode.MaxASCII {
				v.dai(evidenze.Diagnostica{Codice: CodicePatternClasseNonASCII, Messaggio: "classe con caratteri fuori dall'ASCII nel pattern (per esempio una classe Unicode)"})
			}
			if classeConControlli(re.Rune) {
				v.dai(evidenze.Diagnostica{Codice: CodiceCarattereDiControllo, Messaggio: messaggioEscape})
			}
		}
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase != 0 {
			v.dai(evidenze.Diagnostica{Codice: CodicePatternFlag, Messaggio: "flag in linea nel pattern: le maiuscole si dichiarano nella base"})
		}
		for _, r := range re.Rune {
			if vietatoNelPattern(r) {
				v.dai(evidenze.Diagnostica{Codice: CodiceCarattereDiControllo, Messaggio: messaggioEscape})
				break
			}
		}
	case syntax.OpAlternate:
		v.dai(evidenze.Diagnostica{Codice: CodicePatternAlternanza, Messaggio: "«|» nel pattern: le alternative si scrivono come letterali"})
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText:
		v.dai(evidenze.Diagnostica{Codice: CodicePatternAncora, Messaggio: "ancora nel pattern (^, $, \\A, \\z): il pattern è un pezzo della forma"})
	case syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		v.dai(evidenze.Diagnostica{Codice: CodicePatternConfine, Messaggio: "\\b o \\B nel pattern: i confini li dichiara la base e li controlla il motore"})
	case syntax.OpStar, syntax.OpPlus:
		v.dai(evidenze.Diagnostica{Codice: CodicePatternRipetizioneIllimitata, Messaggio: "*, + nel pattern: ogni ripetizione ha un massimo, {n,m}"})
	case syntax.OpRepeat:
		if re.Max < 0 {
			v.dai(evidenze.Diagnostica{Codice: CodicePatternRipetizioneIllimitata, Messaggio: "{n,} nel pattern: ogni ripetizione ha un massimo, {n,m}"})
		} else if massimo := v.lim.Grammatica.MaxRipetizione; re.Max > massimo {
			v.dai(evidenze.Diagnostica{
				Codice:    CodicePatternRipetizioneOltreLimite,
				Messaggio: fmt.Sprintf("ripetizione fino a %d: max_ripetizione dell'indice è %d", re.Max, massimo),
			})
		}
	}
	for _, s := range re.Sub {
		v.albero(s)
	}
}

// puoEssereVuota dice se l'espressione può finire senza aver letto un carattere: ogni pezzo di una sequenza
// può, una delle alternative può, il pezzo ripetuto è facoltativo. Ancore e confini non leggono caratteri.
// È puoEssereVuota del legacy (core/registro/regole/regole.go), copiata: il legacy non si importa (G9).
func puoEssereVuota(s *syntax.Regexp) bool {
	switch s.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpStar, syntax.OpQuest:
		return true
	case syntax.OpLiteral:
		return len(s.Rune) == 0
	case syntax.OpCapture, syntax.OpPlus:
		return puoEssereVuota(s.Sub[0])
	case syntax.OpRepeat:
		return s.Min == 0 || puoEssereVuota(s.Sub[0])
	case syntax.OpConcat:
		for _, x := range s.Sub {
			if !puoEssereVuota(x) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, x := range s.Sub {
			if puoEssereVuota(x) {
				return true
			}
		}
	}
	// un carattere, una classe, un punto: leggono sempre qualcosa (OpNoMatch non trova niente)
	return false
}

// spazioOControllo: i caratteri che un codice non contiene, come spazioOControllo del motore legacy
// (core/inbox/classificazione), copiata: lo spazio ASCII e quelli sotto, ogni altro spazio Unicode
// (U+00A0, U+202F…), i controlli e i caratteri di formato.
func spazioOControllo(r rune) bool {
	return r <= ' ' || unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// vietatoNelPattern: in un pattern nessuno spazio, nemmeno U+0020 (lo spazio si scrive in un letterale).
func vietatoNelPattern(r rune) bool { return spazioOControllo(r) }

// messaggioEscape: lo stesso divieto, quando il carattere è scritto con un escape del pattern (\t, \s, \x00,
// \x{a0}…): il testo è pulito, ma il pattern leggerebbe un carattere che un codice non contiene.
const messaggioEscape = "carattere di controllo, di formato o spazio scritto con un escape nel pattern (per esempio \\t, \\s, \\x00): un codice non li contiene, e lo spazio si scrive in un letterale"

// classeConControlli dice se una classe contiene un controllo o uno spazio ASCII (U+0000-U+0020, U+007F).
// Quelli fuori dall'ASCII stanno solo in una classe che ha già il suo codice.
func classeConControlli(intervalli []rune) bool {
	for i := 0; i+1 < len(intervalli); i += 2 {
		da, a := intervalli[i], intervalli[i+1]
		if da <= ' ' || (da <= 0x7f && a >= 0x7f) {
			return true
		}
	}
	return false
}

// vietatoNelLetterale: in un letterale U+0020 resta ammesso, perché i separatori veri lo usano; NBSP,
// U+202F e gli altri spazi no.
func vietatoNelLetterale(r rune) bool { return r != ' ' && spazioOControllo(r) }

// messaggioControllo spiega il carattere vietato; per il backspace suggerisce la causa più probabile: un
// «\b» scritto nel JSON è un backspace, non un confine (T20).
func messaggioControllo(cosa, s string) string {
	if strings.ContainsRune(s, '\b') {
		return fmt.Sprintf("carattere di controllo nel %s: un backspace, cioè un \"\\b\" scritto nel JSON (forse volevi \"\\\\b\"? Ma i confini li dichiara la base)", cosa)
	}
	return fmt.Sprintf("carattere di controllo, di formato o spazio non ammesso nel %s", cosa)
}
