package archivio

// L1 — il nome con cui una voce si scrive nello staging si taglia a caratteri, non a byte: un taglio a meta'
// di una lettera accentata dava un nome di file che non e' UTF-8 valido.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestIlNomeDiUnaVoceSiTagliaACaratteri(t *testing.T) {
	// 119 lettere da un byte e poi lettere da due: il 120° byte cade a meta' della prima «è»
	lungo := strings.Repeat("a", 119) + strings.Repeat("è", 40) + ".pdf"
	n := nomeSicuro(lungo)
	if !utf8.ValidString(n) {
		t.Fatalf("il nome tagliato non e' UTF-8 valido: %q", n)
	}
	if got := utf8.RuneCountInString(n); got != 120 {
		t.Errorf("il nome tagliato ha %d caratteri, attesi 120", got)
	}
	if !strings.HasPrefix(lungo, n) {
		t.Errorf("il taglio non tiene l'inizio del nome: %q", n)
	}
	// un nome tutto accentato tiene 120 caratteri, non 60
	if got := utf8.RuneCountInString(nomeSicuro(strings.Repeat("è", 200))); got != 120 {
		t.Errorf("un nome accentato tagliato a %d caratteri, attesi 120", got)
	}
	if nomeSicuro("  a:b?.pdf ") != "a_b_.pdf" || nomeSicuro("   ") != "voce" {
		t.Errorf("la pulizia del nome e' cambiata: %q %q", nomeSicuro("  a:b?.pdf "), nomeSicuro("   "))
	}
}
