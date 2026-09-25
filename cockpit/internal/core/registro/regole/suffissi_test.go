package regole

import (
	"encoding/json"
	"strings"
	"testing"
)

// L1 — Fascicolo v3: i suffissi decorativi di un cliente («X_PRT» e «X» sono lo stesso pezzo). Lo schema li
// accetta solo se si possono togliere senza togliere altro: un separatore davanti, almeno due caratteri,
// niente spazi, non piu' lunghi di MaxSuffisso, e mai una coda che il riconoscimento legge come revisione.
// Che cosa il motore ne fa sta in core/inbox/classificazione (suffissi_test.go).

func TestISuffissiDecorativiBuoniEntranoInteri(t *testing.T) {
	r, err := ValidaRegole([]byte(`{"suffissi_decorativi": ["_PRT", "-asm", ".DRW", "_PRT_ASM"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.SuffissiDecorativi, " ") != "_PRT -asm .DRW _PRT_ASM" {
		t.Errorf("suffissi: %q", r.SuffissiDecorativi)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"suffissi_decorativi":["_PRT","-asm",".DRW","_PRT_ASM"]`) {
		t.Errorf("il JSON riscritto: %s", b)
	}
	if r2, err := ValidaRegole(b); err != nil || len(r2.SuffissiDecorativi) != 4 {
		t.Errorf("il giro di scrittura e rilettura: %v %+v", err, r2)
	}
	// un cliente senza suffissi non ne scrive la chiave
	vuoto, _ := json.Marshal(Regole{})
	if strings.Contains(string(vuoto), "suffissi") {
		t.Errorf("la chiave vuota si scrive: %s", vuoto)
	}
}

func TestUnSuffissoCheToglierebbeAltroSiRifiuta(t *testing.T) {
	casi := []struct{ suffisso, atteso string }{
		{"", "è vuoto"},
		{"  ", "è vuoto"},
		{"PRT", "deve cominciare con _ - oppure ."},
		{"_P", "troppo corto"},
		{"_PR T", "contiene spazi"},
		{"_" + strings.Repeat("X", MaxSuffisso), "più lungo di"},
		// code che il riconoscimento legge come revisione: toglierle toglierebbe la revisione
		{"_1", "troppo corto"},
		{"_12", "si legge come una revisione"},
		{"-B", "troppo corto"},
		{"_R2", "si legge come una revisione"},
		{"_REV1", "si legge come una revisione"},
		{"_rev12", "si legge come una revisione"},
		{"-RT", "si legge come una revisione"},
		{".R9", "si legge come una revisione"},
	}
	for _, c := range casi {
		d := verificaSuffisso("suffisso decorativo 1", c.suffisso)
		if d.Ok {
			t.Errorf("%q accettato", c.suffisso)
			continue
		}
		if !strings.Contains(d.Motivo, c.atteso) {
			t.Errorf("%q: il motivo %q non dice %q", c.suffisso, d.Motivo, c.atteso)
		}
		// e la porta in scrittura rifiuta le regole intere, dicendo quale
		j, _ := json.Marshal(map[string]any{"suffissi_decorativi": []string{"_PRT", c.suffisso}})
		if _, err := ValidaRegole(j); err == nil || !strings.Contains(err.Error(), "suffisso decorativo 2") {
			t.Errorf("%q: la convalida delle regole: %v", c.suffisso, err)
		}
	}
	// codici veri di tre caratteri dopo il separatore restano buoni, anche se cominciano per R
	for _, buono := range []string{"_PRT", "_RAW", "-R12X", "_ASM", "_REVISIONE"} {
		if d := verificaSuffisso("x", buono); !d.Ok {
			t.Errorf("%q rifiutato: %s", buono, d.Motivo)
		}
	}
}

func TestISuffissiDecorativiHannoUnLimite(t *testing.T) {
	troppi := make([]string, maxSuffissi+1)
	for i := range troppi {
		troppi[i] = "_S" + string(rune('A'+i)) + "X"
	}
	j, _ := json.Marshal(map[string]any{"suffissi_decorativi": troppi})
	_, err := ValidaRegole(j)
	if err == nil || !strings.Contains(err.Error(), "il limite è 10") {
		t.Errorf("undici suffissi: %v", err)
	}
	j, _ = json.Marshal(map[string]any{"suffissi_decorativi": troppi[:maxSuffissi]})
	if _, err := ValidaRegole(j); err != nil {
		t.Errorf("dieci suffissi: %v", err)
	}
}
