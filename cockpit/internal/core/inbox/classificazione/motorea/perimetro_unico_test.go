package motorea

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — la regola unica del perimetro di un contesto (OB-20 delle decisioni dell'ondata B di V1; RF RQ-6):
// OccorrenzaCoincide è il criterio dell'attivazione (occorrenzeDi, quindi NelPerimetro e il perimetro
// dell'importazione) e della dipendenza D-13 del ciclo dei file (filerfq.citaOccorrenza la chiama). Qui i casi della
// regola: per allegato, per sha256 e nome (o il nome finale del percorso dentro un archivio), per contenitore, mai per
// sola impronta; e la parità con occorrenzeDi su ogni caso.
func TestOccorrenzaCoincideLaRegolaUnica(t *testing.T) {
	a, k := uuid.MustParse("00000000-0000-4000-8000-0000000000a1"), uuid.MustParse("00000000-0000-4000-8000-0000000000c1")
	sha := strings.Repeat("c", 64)
	file := FileDelPerimetro{AllegatoID: a, Sha256: sha, Nome: "telaio.stp", Percorso: `cartella\telaio.stp`, Contenitore: k.String()}
	casi := []struct {
		nome string
		o    OccorrenzaContesto
		f    FileDelPerimetro
		si   bool
	}{
		{"l'allegato", OccorrenzaContesto{AllegatoID: strings.ToUpper(a.String())}, file, true},
		{"un altro allegato", OccorrenzaContesto{AllegatoID: k.String()}, file, false},
		{"l'allegato senza origine", OccorrenzaContesto{AllegatoID: uuid.Nil.String()}, FileDelPerimetro{Sha256: sha, Nome: "telaio.stp"}, false},
		{"sha256 e nome", OccorrenzaContesto{Sha256: strings.ToUpper(sha), Nome: "telaio.stp"}, file, true},
		{"la sola impronta", OccorrenzaContesto{Sha256: sha}, file, false},
		{"il solo nome", OccorrenzaContesto{Nome: "telaio.stp"}, FileDelPerimetro{Nome: "telaio.stp"}, false},
		{"un altro nome", OccorrenzaContesto{Sha256: sha, Nome: "altro.stp"}, file, false},
		{"il nome finale del percorso", OccorrenzaContesto{Sha256: sha, Nome: "telaio.stp"},
			FileDelPerimetro{Sha256: sha, Nome: "x.stp", Percorso: `a/b\telaio.stp`}, true},
		{"il contenitore", OccorrenzaContesto{Sha256: sha, Nome: "telaio.stp", Contenitore: strings.ToUpper(k.String())}, file, true},
		{"un altro contenitore", OccorrenzaContesto{Sha256: sha, Nome: "telaio.stp", Contenitore: a.String()}, file, false},
		{"il contenitore senza contenitore", OccorrenzaContesto{Sha256: sha, Nome: "telaio.stp", Contenitore: k.String()},
			FileDelPerimetro{Sha256: sha, Nome: "telaio.stp"}, false},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			if got := OccorrenzaCoincide(c.o, c.f); got != c.si {
				t.Errorf("OccorrenzaCoincide = %v, atteso %v", got, c.si)
			}
			r := &ContestoRisolto{perimetro: []OccorrenzaContesto{c.o}}
			fonte := evidenze.Fonte{Tipo: "allegato", OrigineID: c.f.AllegatoID, ContenitoreID: c.f.Contenitore,
				Provenienza:      evidenze.Provenienza{NomeRicevuto: c.f.Nome, PercorsoRicevuto: c.f.Percorso},
				RiferimentoFatti: evidenze.RiferimentoFatti{Sha256: c.f.Sha256}}
			if got := len(r.occorrenzeDi(fonte)) > 0; got != c.si {
				t.Errorf("occorrenzeDi = %v, atteso %v: l'attivazione non usa la regola unica", got, c.si)
			}
		})
	}
}
