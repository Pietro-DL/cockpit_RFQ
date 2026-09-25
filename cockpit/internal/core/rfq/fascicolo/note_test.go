package fascicolo

// L1 — le note delle proposte stanno in colonne da 200 caratteri: una nota costruita con un nome di file
// (fino a 300) o con il nome grezzo di un nodo si taglia, contando i caratteri e non i byte.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLaNotaSiTagliaAllaMisuraDellaColonna(t *testing.T) {
	lunga := "superata da " + strings.Repeat("disegno è già ", 30) + ".pdf"
	n := tagliaNota(lunga)
	if got := utf8.RuneCountInString(n); got != maxNota {
		t.Errorf("la nota tagliata ha %d caratteri, attesi %d", got, maxNota)
	}
	if !utf8.ValidString(n) {
		t.Errorf("la nota tagliata non e' UTF-8 valido: il taglio ha spezzato un carattere")
	}
	if !strings.HasPrefix(lunga, n) {
		t.Errorf("il taglio non tiene l'inizio della nota: %q", n)
	}
	if corta := "superata da X.pdf"; tagliaNota(corta) != corta {
		t.Errorf("una nota corta e' cambiata: %q", tagliaNota(corta))
	}
}
