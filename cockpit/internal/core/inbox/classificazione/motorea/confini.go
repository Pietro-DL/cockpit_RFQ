package motorea

import (
	"unicode/utf8"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// Le classi di confine (D-05; R25 f): che cosa non può stare accanto al codice. Servono a evitare le letture
// annidate della stessa famiglia, e ognuna ha un caso che la giustifica (piano A, par.4.4.5):
//   - alnum_ascii: né lettera né cifra ASCII. È il valore predefinito, per tutte le famiglie;
//   - parola_ascii: in più niente «_». Serve dove «_» comincia un pezzo che la forma non legge: senza, una
//     forma corta leggerebbe la base dentro «<base>_<token>» (ACME: «9123456A» dentro «9123456A_PRT»);
//   - alnum_ascii_o_spazio: in più niente spazio U+0020. Serve dove lo spazio comincia il resto di una forma
//     più lunga della stessa famiglia: senza, la forma corta leggerebbe il codice anche dentro il nome con il
//     token («ACME-030P7120100 00 IN_WORK.stp»);
//   - parola_ascii_o_punto_cifra: come parola_ascii, e nemmeno «.» seguito da una cifra. Solo a destra (la
//     validazione lo vieta a sinistra): la forma parziale non deve leggere dentro la forma completa
//     («9.123.4567» dentro «9.123.4567.3», A-C07).
//
// Il confine sinistro lo controlla il motore in Go sul testo intero, perché la ricerca su testo[i:] non vede
// la runa prima (T4). Il destro lo scrive il compilatore in fondo alla regex, fuori dalla lettura; lo stesso
// controllo in Go serve ad allungare una lettura (scansione.go). Le lettere e le cifre fuori dall'ASCII, lo
// spazio non separabile e gli altri spazi sono confini per tutte le classi: le classi sono solo ASCII (T3,
// T10). Una classe nuova è una modifica del contratto, con un commit dichiarato (4.10 n.2).

// classeNota: la classe è una delle quattro del contratto.
func classeNota(c grammatica.ClasseConfine) bool {
	switch c {
	case grammatica.ConfineAlnumASCII, grammatica.ConfineParolaASCII, grammatica.ConfineAlnumASCIIOSpazio,
		grammatica.ConfineParolaASCIIOPuntoCifra:
		return true
	}
	return false
}

// frammentoDestro: la regex del confine destro, con l'alternativa della fine del testo. Il carattere del
// confine entra nel match ma non nella lettura, che è il gruppo g0.
func frammentoDestro(c grammatica.ClasseConfine) (string, bool) {
	switch c {
	case grammatica.ConfineAlnumASCII:
		return `(?:[^0-9A-Za-z]|\z)`, true
	case grammatica.ConfineParolaASCII:
		return `(?:[^0-9A-Z_a-z]|\z)`, true
	case grammatica.ConfineAlnumASCIIOSpazio:
		return `(?:[^ 0-9A-Za-z]|\z)`, true
	case grammatica.ConfineParolaASCIIOPuntoCifra:
		return `(?:[^.0-9A-Z_a-z]|\.[^0-9]|\.\z|\z)`, true
	}
	return "", false
}

// vietataAccanto: la runa non può stare accanto al codice con quella classe. Il «.» seguito da una cifra lo
// guarda confineDestro.
func vietataAccanto(c grammatica.ClasseConfine, r rune) bool {
	alnum := ('0' <= r && r <= '9') || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
	switch c {
	case grammatica.ConfineParolaASCII, grammatica.ConfineParolaASCIIOPuntoCifra:
		return alnum || r == '_'
	case grammatica.ConfineAlnumASCIIOSpazio:
		return alnum || r == ' '
	}
	return alnum
}

// confineSinistro: la runa prima di i, sul testo intero, rispetta la classe. All'inizio del testo sì.
func confineSinistro(c grammatica.ClasseConfine, testo string, i int) bool {
	if i == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(testo[:i])
	return !vietataAccanto(c, r)
}

// confineDestro: ciò che segue j rispetta la classe. Alla fine del testo sì. È lo stesso confine di
// frammentoDestro, scritto in Go.
func confineDestro(c grammatica.ClasseConfine, testo string, j int) bool {
	if j >= len(testo) {
		return true
	}
	r, w := utf8.DecodeRuneInString(testo[j:])
	if vietataAccanto(c, r) {
		return false
	}
	if c == grammatica.ConfineParolaASCIIOPuntoCifra && r == '.' && j+w < len(testo) {
		if d, _ := utf8.DecodeRuneInString(testo[j+w:]); '0' <= d && d <= '9' {
			return false
		}
	}
	return true
}
