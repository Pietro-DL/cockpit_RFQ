package importazione

// L1 — DECA PE-08 (V1, ondata B, corsia motore; EM §3.8, «Una lettura»; prova A-25 di PM23 §10, la parte della foglia):
// codice_proposto e rev_proposta di una lettura sono facoltativi; con un valore valgono le regole di quelli della voce
// (il codice ammissibile, la rev in maiuscole ASCII, al più 10 byte), e la rev c'è solo con il codice; TrovaLettura li
// restituisce; la copia compatta di V-1 non li porta. I dati sono quelli inventati di contenutoDiProva.

import (
	"errors"
	"strings"
	"testing"
)

// conCodiceProposto: il contenuto di prova con codice e rev di partenza sull'alternativa di #9 (la lettura di BETA).
func conCodiceProposto(t *testing.T, codice, rev string) Contenuto {
	t.Helper()
	return cambiato(t, contenutoDiProva(), func(m map[string]any) {
		alt := ogg(ele(voceIn(m, "#9")["alternative"])[0])
		alt["codice_proposto"], alt["rev_proposta"] = codice, rev
	})
}

func TestPE08IlCodicePropostoDiUnaLettura(t *testing.T) {
	c := conCodiceProposto(t, "400123", "B")
	r := componi(t, c)
	chiave := aProva + "/l:u:step:#9:id:beta-num/sei:0-6"
	k := r.Contenuto
	l, ok := k.TrovaLettura(chiave)
	if !ok || l.CodiceProposto != "400123" || l.RevProposta != "B" {
		t.Fatalf("TrovaLettura: %+v %v", l, ok)
	}
	// la copia compatta non cambia: V-1 confronta la stessa copia con e senza i due campi
	k0 := componi(t, contenutoDiProva()).Contenuto
	senza, ok := k0.TrovaLettura(chiave)
	if !ok {
		t.Fatal("la lettura senza i due campi non si trova")
	}
	a, err := l.Compatta()
	if err != nil {
		t.Fatal(err)
	}
	b, err := senza.Compatta()
	if err != nil {
		t.Fatal(err)
	}
	if !StessaLettura(a, b) {
		t.Error("i due campi cambiano la copia compatta della lettura")
	}
	// rilettura stretta: gli stessi byte
	if x, err := Leggi(r.Canonico); err != nil || string(x.Canonico) != string(r.Canonico) {
		t.Errorf("la rilettura: %v", err)
	}
}

func TestPE08IValoriNonAmmessi(t *testing.T) {
	for _, c := range []struct {
		nome, codice, rev, campo string
	}{
		{"un codice con uno spazio", "400 123", "", "codice_proposto"},
		{"un codice di 41 byte", strings.Repeat("7", 41), "", "codice_proposto"},
		{"una rev in minuscolo", "400123", "b", "rev_proposta"},
		{"una rev di 11 byte", "400123", strings.Repeat("A", 11), "rev_proposta"},
		{"una rev con uno spazio", "400123", "A 1", "rev_proposta"},
		// senza una proiezione ammissibile del codice i due campi sono omessi: la rev da sola no (revisione del 18/10)
		{"una rev senza il codice", "", "B", "rev_proposta"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Componi(conCodiceProposto(t, c.codice, c.rev))
			var e *Errore
			if !errors.As(err, &e) || e.Codice != "" || !strings.HasSuffix(e.Campo, "alternative[0]."+c.campo) {
				t.Errorf("atteso un difetto di forma su %s, avuto %v", c.campo, err)
			}
		})
	}
}
