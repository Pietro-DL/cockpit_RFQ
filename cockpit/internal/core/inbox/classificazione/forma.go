package classificazione

import (
	"strings"
	"unicode"
)

// La forma di una stringa (Smistamento, giro 4, fase 4.17a: il censimento delle forme). Le regole di un cliente
// si scrivono guardando come sono fatti i suoi codici, e non i codici uno per uno: «7120001A_1» e «7120011A_2»
// sono la stessa cosa per una regex, e lo si vede solo se tutti e due diventano «9999999A_9». La forma serve a
// contare, non a riconoscere: nessuna regola del motore la usa.

// Forma e' la sagoma di una stringa: ogni cifra diventa 9, ogni lettera A, i separatori restano dove sono
// («7120001A_1» → «9999999A_9», «0.712.0011.3/10» → «9.999.9999.9/99»). Gli spazi in testa e in coda si
// tolgono, quelli in mezzo (anche a capo e tabulazioni) diventano uno spazio solo: «RDO  12345» e «RDO 12345»
// sono la stessa forma.
func Forma(s string) string {
	return sagoma(s, func(r rune) rune { return 'A' })
}

// FormaCifre e' la forma che tiene le lettere, in maiuscolo: solo le cifre diventano 9 («7120010X1» →
// «9999999X9»). Serve dove la lettera e' il segno di una famiglia: la X della minuteria accanto alla A dei
// pezzi, con le stesse cifre intorno, e con Forma sarebbero uguali.
func FormaCifre(s string) string {
	return sagoma(s, unicode.ToUpper)
}

// sagoma sostituisce le cifre con 9 e le lettere con quello che dice lettera; il resto resta, con gli spazi
// ripuliti come dice Forma.
func sagoma(s string, lettera func(rune) rune) string {
	var b strings.Builder
	spazio := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsSpace(r):
			spazio = true
			continue
		case unicode.IsDigit(r):
			r = '9'
		case unicode.IsLetter(r):
			r = lettera(r)
		}
		if spazio {
			b.WriteByte(' ')
			spazio = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
