package web

// L1 — «Carica precedenti»: con quali nomi una cartella del sync si ritrova fra le copie dei messaggi.
// Il cursore porta il nome della configurazione («Inbox»), la copia quello che Outlook mostra («Posta in
// arrivo»): la mail piu' vecchia di UNA cartella si cerca con tutti e due, e mai con quelli di un'altra.

import (
	"slices"
	"testing"
)

func TestUnaCartellaSiRitrovaConISuoiNomiENonConQuelliDiUnAltra(t *testing.T) {
	for _, c := range []struct {
		nome   string
		dentro []string
		fuori  []string
	}{
		{"Inbox", []string{"inbox", "posta in arrivo"}, []string{"sent items", "posta inviata"}},
		{"Sent Items", []string{"sent items", "posta inviata"}, []string{"inbox", "posta in arrivo"}},
		{"Posta inviata", []string{"sent items", "posta inviata"}, []string{"inbox"}},
		{`Commerciale\Ordini`, []string{`commerciale\ordini`}, []string{"inbox", "sent items"}},
	} {
		nomi := nomiDellaCartella(c.nome)
		for _, n := range c.dentro {
			if !slices.Contains(nomi, n) {
				t.Errorf("%s: manca %q in %v", c.nome, n, nomi)
			}
		}
		for _, n := range c.fuori {
			if slices.Contains(nomi, n) {
				t.Errorf("%s: c'e' %q, che e' un'altra cartella (%v)", c.nome, n, nomi)
			}
		}
	}
}
