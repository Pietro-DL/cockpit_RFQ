package motorea

import (
	"fmt"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// GP-1 (revisione indipendente del contratto, voce 4): una codifica dichiarata nel contesto spegne le forme
// dell'altro emittente su quel campo, ma una prova contraria non deve diventare «nessuna lettura». Le forme spente
// dalla sola codifica si provano lo stesso, in ombra: le loro letture non entrano nell'interpretazione, ma se ce ne
// sono la diagnostica contesto.codifica_contraddetta le elenca (direttiva 21 §3.2: «una contraddizione resta un
// conflitto da mostrare»).

// ombre: le letture in ombra di un'unità, come diagnostica.
func (it *interprete) ombre(u evidenze.UnitaEvidenza) {
	a := it.att
	if a == nil {
		return
	}
	cod := a.codificaDi(u.Selettore)
	if cod == "" || cod == grammatica.CodificaMista {
		return
	}
	misto := &attivazione{partner: a.partner, modelli: a.modelli, codifiche: map[string]string{u.Selettore.String(): grammatica.CodificaMista}}
	var lette []string
	for _, p := range it.m.piani[u.Selettore] {
		if p.attivo(u.Selettore, a) || !p.attivo(u.Selettore, misto) || !p.estensioneAmmessa(u.Selettore, u.Testo) {
			continue
		}
		ls, _ := scandisci(p, u.Selettore, u.Testo, it.m.lim)
		for _, l := range ls {
			lette = append(lette, fmt.Sprintf("%s/%s «%s» [%d,%d)", l.Famiglia, l.Forma, l.Originale, l.Intervallo.Inizio, l.Intervallo.Fine))
		}
	}
	if len(lette) == 0 {
		return
	}
	it.diag = append(it.diag, evidenze.Diagnostica{
		Codice:    CodiceContestoCodificaContraddetta,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  "unita[" + u.ID + "]",
		Messaggio: fmt.Sprintf("il contesto dichiara il campo %s codificato da %q, ma vi si leggono anche codici di un altro emittente (in ombra, non entrano): %s", u.Selettore.String(), cod, strings.Join(lette, "; ")),
		Rif:       []string{u.ID},
	})
}

// suffissoNonLetto (G-5): il suffisso attaccato dopo la fine della lettura, se c'è: «-» o «.» seguito da un carattere
// alfanumerico ASCII, fino al primo spazio. Per un nome di file l'estensione finale («.pdf») non è un suffisso.
func suffissoNonLetto(u evidenze.UnitaEvidenza, fine int) string {
	if fine < 0 || fine+1 >= len(u.Testo) {
		return ""
	}
	resto := u.Testo[fine:]
	if (resto[0] != '-' && resto[0] != '.') || !alnumASCII(resto[1]) {
		return ""
	}
	if i := strings.IndexByte(resto, ' '); i > 0 {
		resto = resto[:i]
	}
	if u.Selettore.Contesto == evidenze.ContestoNomeFile {
		if resto[0] == '.' && !strings.Contains(resto[1:], ".") && !strings.Contains(resto[1:], "-") {
			return "" // solo l'estensione
		}
		if j := strings.LastIndexByte(resto, '.'); j > 0 {
			resto = resto[:j] // l'estensione finale non fa parte del suffisso
		}
	}
	return resto
}

func alnumASCII(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
