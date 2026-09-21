package anagrafica

import (
	"regexp"
	"strings"
)

var reCoda = regexp.MustCompile(`\s+[-–|(<].*$`) // "Pietro Spinozzi - pietro@..." / "Mario Rossi (ACME)" / "Rossi <mail>"

// NomeCognome ricava nome e cognome dal display name del mittente (o, in mancanza, dalla parte locale
// dell'indirizzo "nome.cognome@"). È un precompilato per il form: l'operatore può correggerlo.
//
//	"Francesco Gabriele Galizia" → ("Francesco Gabriele", "Galizia")
//	"Rossi, Mario"               → ("Mario", "Rossi")
//	"mario.rossi@acme.it"        → ("Mario", "Rossi")
func NomeCognome(display, email string) (nome, cognome string) {
	d := strings.TrimSpace(strings.Trim(display, `"'`))
	d = reCoda.ReplaceAllString(d, "")
	if d == "" || strings.Contains(d, "@") {
		locale := email
		if i := strings.Index(locale, "@"); i > 0 {
			locale = locale[:i]
		}
		parti := strings.FieldsFunc(locale, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
		for i := range parti {
			parti[i] = capitalizza(parti[i])
		}
		d = strings.Join(parti, " ")
	}
	if i := strings.Index(d, ","); i > 0 {
		return strings.TrimSpace(d[i+1:]), strings.TrimSpace(d[:i])
	}
	parole := strings.Fields(d)
	switch len(parole) {
	case 0:
		return "", ""
	case 1:
		return "", parole[0]
	}
	return strings.Join(parole[:len(parole)-1], " "), parole[len(parole)-1]
}

func capitalizza(s string) string {
	if s == "" {
		return s
	}
	r := []rune(strings.ToLower(s))
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}
