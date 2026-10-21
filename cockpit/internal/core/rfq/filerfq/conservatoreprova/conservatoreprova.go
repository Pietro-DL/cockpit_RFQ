// Package conservatoreprova e' il doppio delle prove del posto degli «Altri file» (V1, U2 §3.13: il ConservatoreLocale
// che ha preso il posto del deposito locale dell'ondata B): lo scrittore di produzione, nas.Conservatore, su una
// cartella temporanea, con gli aiuti per guardarla. Solo nelle prove L1 e L4: nessun file di produzione lo importa
// (prova statica TestConservatoreLocaleSoloNelleProve in core/rfq/filerfq), e nessuna configurazione lo accende.
package conservatoreprova

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"promatec/cockpit/internal/platform/storage/nas"
)

// Locale e' il conservatore delle prove: nas.Conservatore su una cartella temporanea.
type Locale struct {
	*nas.Conservatore
}

// Nuovo: il conservatore delle prove sulla cartella data (di solito t.TempDir()).
func Nuovo(radice string) *Locale {
	return &Locale{Conservatore: &nas.Conservatore{Radice: radice}}
}

// Percorso: il file assoluto dei pezzi relativi alla radice (la cartella della RFQ e il percorso ALTRO\<nome>
// registrati nella riga).
func (l *Locale) Percorso(pezzi ...string) string {
	p := []string{l.Radice}
	for _, x := range pezzi {
		p = append(p, filepath.FromSlash(strings.ReplaceAll(x, `\`, "/")))
	}
	return filepath.Join(p...)
}

// File: i file definitivi sotto la radice, relativi a lei e ordinati, senza i parziali ne' i nomi di servizio della
// rimozione.
func (l *Locale) File() []string {
	return l.elenca(func(nome string) bool { return !parziale(nome) && !servizio(nome) })
}

// Parti: i parziali <nome>.parte.<token> rimasti sotto la radice.
func (l *Locale) Parti() []string { return l.elenca(parziale) }

// NomiDiServizio: i nomi di servizio della rimozione rimasti sotto la radice.
func (l *Locale) NomiDiServizio() []string { return l.elenca(servizio) }

func (l *Locale) elenca(tieni func(string) bool) []string {
	var out []string
	_ = filepath.WalkDir(l.Radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !tieni(d.Name()) {
			return nil
		}
		rel, _ := filepath.Rel(l.Radice, p)
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out
}

func parziale(nome string) bool {
	_, ok := nas.TokenDiParte(nome)
	return ok
}

func servizio(nome string) bool {
	return strings.HasPrefix(nome, ".") && strings.Contains(nome, ".cockpit-rimozione.")
}

// Esiste: il file relativo c'e'.
func (l *Locale) Esiste(rel string) bool {
	_, err := os.Stat(l.Percorso(rel))
	return err == nil
}
