// L1 — messaggio 46 [U] (21/10): la traccia del gesto scritta dal server al posto del motivo dell'utente. Dati inventati.

package traccia

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGesto(t *testing.T) {
	casi := []struct {
		nome, sigla, widget, cosa string
		evidenze                  []string
		max                       int
		atteso                    string
	}{
		{"completa", "PS", "Revoca come sorgente", "lo STEP non è più la sorgente di ACME-1", nil, 0,
			"gesto di PS da «Revoca come sorgente»: lo STEP non è più la sorgente di ACME-1"},
		{"con le evidenze", "PS", "Conferma la sorgente", "codice letto «9120001», prodotto «9120001A»", []string{"nome del file 9120001 (base uguale)", " ", "radice"}, 0,
			"gesto di PS da «Conferma la sorgente»: codice letto «9120001», prodotto «9120001A»; evidenze: nome del file 9120001 (base uguale); radice"},
		{"senza sigla", "", "Scarta", "il file esce dai file attivi", nil, 0, "gesto da «Scarta»: il file esce dai file attivi"},
		{"senza cosa", "AB", "Converti", "", nil, 0, "gesto di AB da «Converti»"},
		{"a capo e spazi", " PS ", " Scarta\n", "riga uno\r\nriga due", nil, 0, "gesto di PS da «Scarta»: riga uno riga due"},
	}
	for _, c := range casi {
		if g := Gesto(c.max, c.sigla, c.widget, c.cosa, c.evidenze...); g != c.atteso {
			t.Errorf("%s: %q, atteso %q", c.nome, g, c.atteso)
		}
		if !strings.HasPrefix(Gesto(c.max, c.sigla, c.widget, c.cosa, c.evidenze...), Prefisso) {
			t.Errorf("%s: la traccia non comincia con %q", c.nome, Prefisso)
		}
	}
}

func TestGestoDeterministicoETroncato(t *testing.T) {
	a := Gesto(500, "PS", "Conferma la sorgente", strings.Repeat("è", 600))
	b := Gesto(500, "PS", "Conferma la sorgente", strings.Repeat("è", 600))
	if a != b {
		t.Fatalf("la stessa traccia due volte è diversa:\n%q\n%q", a, b)
	}
	if n := utf8.RuneCountInString(a); n != 500 || !strings.HasSuffix(a, "…") {
		t.Errorf("troncata a 500 rune con «…»: %d rune, %q…", n, a[len(a)-8:])
	}
	if Tronca("abc", 1) != "…" || Tronca("abc", 3) != "abc" || Tronca("abcd", 3) != "ab…" || Tronca("abc", 0) != "abc" {
		t.Errorf("Tronca: %q %q %q %q", Tronca("abc", 1), Tronca("abc", 3), Tronca("abcd", 3), Tronca("abc", 0))
	}
}

func TestDellUtente(t *testing.T) {
	if DellUtente("  scritto da me ", "gesto di PS") != "scritto da me" {
		t.Error("il motivo dell'utente (client di prima) vince sulla traccia")
	}
	if DellUtente("  ", "gesto di PS") != "gesto di PS" {
		t.Error("senza motivo dell'utente vale la traccia")
	}
}
