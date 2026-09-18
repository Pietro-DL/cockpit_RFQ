package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// CP9, CP10 (parte pura) e CP11 — le convenzioni di codice → lavorazione (blocco 7A, D39).
// Nessun suffisso reale: i codici sono inventati.

func conv(modo, espr, esempio, contro string, lav ...string) Convenzione {
	return Convenzione{ID: uuid.New(), Modo: modo, Espressione: espr, Esempio: esempio, Controesempio: contro,
		Descrizione: "convenzione di prova " + espr, Attiva: true, Lavorazioni: lav}
}

func TestCP9LaPortaInScritturaRifiutaUnaConvenzioneCheNonRiconoscerebbeNiente(t *testing.T) {
	casi := []struct {
		nome   string
		c      Convenzione
		motivo string // un pezzo del motivo atteso
	}{
		{"espressione vuota", conv(ModoSuffisso, "  ", "PROVA-1-Q", "", "zincatura"), "manca l'espressione"},
		{"regex che non compila", conv(ModoRegex, "Q($", "PROVA-1-Q", "", "zincatura"), "non compila"},
		{"esempio vuoto", conv(ModoSuffisso, "Q", "", "", "zincatura"), "manca l'esempio"},
		{"esempio che non corrisponde", conv(ModoSuffisso, "Q", "PROVA-1-W", "", "zincatura"), "non corrisponde"},
		{"controesempio che corrisponde: «V» prende anche «-ZV»", conv(ModoSuffisso, "V", "PROVA-1-V", "PROVA-1-ZV", "verniciatura_polvere"), "troppo larga"},
		{"controesempio uguale all'esempio", conv(ModoSuffisso, "Q", "PROVA-1-Q", "PROVA-1-Q", "zincatura"), "uguale all'esempio"},
		{"nessuna lavorazione", conv(ModoSuffisso, "Q", "PROVA-1-Q", ""), "nessuna lavorazione"},
		{"lavorazione vuota", conv(ModoSuffisso, "Q", "PROVA-1-Q", "", "zincatura", " "), "vuota"},
		{"suffisso con uno spazio", conv(ModoSuffisso, "Q X", "PROVA-Q X", "", "zincatura"), "spazi"},
		{"suffisso troppo lungo", conv(ModoSuffisso, "ABCDEFGHIJKLM", "PROVA-ABCDEFGHIJKLM", "", "zincatura"), "al massimo"},
		{"modo inventato", conv("prefisso", "Q", "Q-PROVA", "", "zincatura"), "non previsto"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			err := ValidaConvenzione(c.c)
			if err == nil {
				t.Fatal("doveva essere rifiutata")
			}
			if !strings.Contains(err.Error(), c.motivo) {
				t.Fatalf("il motivo non spiega: %q (atteso «%s»)", err, c.motivo)
			}
		})
	}
	buona := conv(ModoSuffisso, "Q", "PROVA-1-Q", "PROVA-1-QX", "zincatura")
	if err := ValidaConvenzione(buona); err != nil {
		t.Fatalf("una convenzione buona passa: %v", err)
	}
}

func TestCP10LaPortaInLetturaSegnaLaRigaRottaENonLaUsaLeAltreFunzionano(t *testing.T) {
	rotta := conv(ModoRegex, "Q($", "PROVA-1-Q", "", "zincatura")
	buona := conv(ModoSuffisso, "Q", "PROVA-1-Q", "", "zincatura")
	spenta := conv(ModoSuffisso, "W", "PROVA-1-W", "", "cataforesi")
	spenta.Attiva = false
	cs, diag := LeggiConvenzioni([]Convenzione{rotta, buona, spenta})
	if len(diag) != 3 {
		t.Fatalf("una diagnosi per riga: %d", len(diag))
	}
	if diag[0].Ok || !strings.Contains(diag[0].Motivo, "non compila") {
		t.Fatalf("la rotta e' ✗ con il motivo: %+v", diag[0])
	}
	if !diag[1].Ok {
		t.Fatalf("la buona e' ✓: %+v", diag[1])
	}
	if !diag[2].Ok || !strings.Contains(diag[2].Motivo, "spenta") {
		t.Fatalf("la spenta e' ✓ ma dice che non si usa: %+v", diag[2])
	}
	if cs.N() != 1 {
		t.Fatalf("usabile e' solo la buona: %d", cs.N())
	}
	if got := cs.Lavorazioni("PROVA-1-Q"); len(got) != 1 || got[0].Lavorazione != "zincatura" {
		t.Fatalf("la buona continua a funzionare: %+v", got)
	}
	if got := cs.Lavorazioni("PROVA-1-W"); len(got) != 0 {
		t.Fatalf("la spenta non si usa: %+v", got)
	}
}

func TestCP11LaRisoluzioneRestituisceLInsiemeDelleLavorazioniConLEvidenza(t *testing.T) {
	q := conv(ModoSuffisso, "Q", "PROVA-1-Q", "", "zincatura")
	zv := conv(ModoSuffisso, "-ZV", "PROVA-1-ZV", "", "zincatura", "verniciatura_polvere")
	mezzo := conv(ModoRegex, `^PR-[0-9]+-K-`, "PR-12-K-9", "PR-12-9", "cataforesi")
	cs, _ := LeggiConvenzioni([]Convenzione{q, zv, mezzo})

	t.Run("suffisso senza distinguere le maiuscole", func(t *testing.T) {
		got := cs.Lavorazioni("prova-7-q")
		if len(got) != 1 || got[0].Lavorazione != "zincatura" || got[0].ConvenzioneID != q.ID {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("una convenzione con due lavorazioni", func(t *testing.T) {
		got := cs.Lavorazioni("PROVA-7-ZV")
		if len(got) != 2 || got[0].Lavorazione != "verniciatura_polvere" || got[1].Lavorazione != "zincatura" {
			t.Fatalf("insieme in ordine di lavorazione: %+v", got)
		}
	})
	t.Run("regex in mezzo al codice", func(t *testing.T) {
		got := cs.Lavorazioni("PR-33-K-1")
		if len(got) != 1 || got[0].Lavorazione != "cataforesi" || got[0].Descrizione != mezzo.Descrizione {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("due convenzioni sullo stesso codice: unione, evidenza della prima", func(t *testing.T) {
		got := cs.Lavorazioni("PR-33-K-1-Q")
		if len(got) != 2 || got[0].Lavorazione != "cataforesi" || got[1].Lavorazione != "zincatura" {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("nessuna corrispondenza: insieme vuoto, nessun errore", func(t *testing.T) {
		if got := cs.Lavorazioni("ALTRO-1"); len(got) != 0 {
			t.Fatalf("%+v", got)
		}
		if got := cs.Lavorazioni("   "); len(got) != 0 {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("il suffisso e' letterale: il punto non e' un jolly", func(t *testing.T) {
		punto := conv(ModoSuffisso, ".Z", "PROVA.Z", "", "zincatura")
		cs, _ := LeggiConvenzioni([]Convenzione{punto})
		if got := cs.Lavorazioni("PROVAXZ"); len(got) != 0 {
			t.Fatalf("«.Z» non deve prendere «XZ»: %+v", got)
		}
	})
	t.Run("nessuna convenzione: niente, senza errore", func(t *testing.T) {
		var vuote *Convenzioni
		if got := vuote.Lavorazioni("PROVA-1-Q"); got != nil {
			t.Fatalf("%+v", got)
		}
	})
}
