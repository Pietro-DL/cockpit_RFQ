package lettura

import (
	"regexp"
	"strings"
)

// L'ANCORAGGIO DELLE TABELLE HTML NEL TESTO
//
// Il testo è la spina dorsale: una tabella dell'HTML si mostra solo nel punto del testo dove le sue
// parole compaiono tutte, di seguito e nello stesso ordine, e solo se cominciano all'inizio di una
// riga e finiscono alla fine di un'altra. Allora quelle righe [Da,A) diventano la tabella, e le
// celle dicono esattamente le stesse parole delle righe che sostituiscono: niente si perde e niente
// si aggiunge. Se il testo e l'HTML non dicono la stessa cosa, la tabella non si mostra — il testo
// resta testo.
//
// Le parole si confrontano nella forma delle regole, tolte le annotazioni che esistono solo nel
// testo semplice: «PZ-001<https://…>» nel testo è «PZ-001» nella cella, «[cid:…]» è un'immagine che
// nella cella non ha testo.

var reAnnotazioniParole = regexp.MustCompile(`(?i)<(?:https?|ftp|mailto|tel):[^<>\s]+>|\[cid:[^\]\s]{1,256}\]`)

func parole(s string) []string {
	s = perConfronto(s)
	if strings.ContainsAny(s, "<[") {
		s = reAnnotazioniParole.ReplaceAllString(s, "")
	}
	return strings.Fields(s)
}

// flusso è una sezione come sequenza di parole (già ridotte a numeri), ciascuna con la sua riga e
// con il segno di prima o ultima parola della riga.
type flusso struct {
	id     []int32
	riga   []int32
	apre   []bool
	chiude []bool
}

func (s *sezione) flusso(voc map[string]int32) *flusso {
	f := &flusso{}
	for i, r := range s.grezze {
		ww := parole(r)
		for k, w := range ww {
			id, ok := voc[w]
			if !ok {
				id = int32(len(voc))
				voc[w] = id
			}
			f.id = append(f.id, id)
			f.riga = append(f.riga, int32(i))
			f.apre = append(f.apre, k == 0)
			f.chiude = append(f.chiude, k == len(ww)-1)
		}
	}
	return f
}

// schema sono le parole delle celle, riga per riga; false se sono meno di tre (troppo poche per
// dire che il testo e la tabella sono la stessa cosa) o se una parola non compare nel testo.
func (t *tabellaHTML) schema(voc map[string]int32) ([]int32, bool) {
	var out []int32
	for _, r := range t.righe {
		for _, c := range r {
			for _, w := range parole(c.testo) {
				id, ok := voc[w]
				if !ok {
					return nil, false
				}
				out = append(out, id)
			}
		}
	}
	return out, len(out) >= 3
}

// ancora cerca ogni tabella, in ordine di documento, dopo quella trovata prima: nella parte fresca
// e poi nella citata. Una tabella che non si trova non sposta niente.
func ancora(tabelle []*tabellaHTML, fresca, citata *sezione) {
	if len(tabelle) == 0 {
		return
	}
	voc := map[string]int32{}
	sezioni := [2]*sezione{fresca, citata}
	flussi := [2]*flusso{fresca.flusso(voc), citata.flusso(voc)}
	sez, pos := 0, 0
	for _, t := range tabelle {
		schema, ok := t.schema(voc)
		if !ok {
			continue
		}
		for k := sez; k < 2; k++ {
			da := 0
			if k == sez {
				da = pos
			}
			f := flussi[k]
			inizio := cerca(f, da, schema)
			if inizio < 0 {
				continue
			}
			fine := inizio + len(schema)
			r := regione{tipo: BloccoTabella, da: int(f.riga[inizio]), a: int(f.riga[fine-1]) + 1, tabella: t.vista()}
			if sezioni[k].occupa(r) {
				sez, pos = k, fine
			}
			break
		}
	}
}

// cerca trova, da da in poi, la prima occorrenza di schema che apre una riga e ne chiude un'altra;
// -1 se non c'è. È Knuth–Morris–Pratt sulle parole: lineare anche sul testo costruito apposta per
// essere lento (mille volte la stessa parola).
func cerca(f *flusso, da int, schema []int32) int {
	m := len(schema)
	if m == 0 || len(f.id)-da < m {
		return -1
	}
	pi := make([]int, m)
	for i, k := 1, 0; i < m; i++ {
		for k > 0 && schema[i] != schema[k] {
			k = pi[k-1]
		}
		if schema[i] == schema[k] {
			k++
		}
		pi[i] = k
	}
	k := 0
	for i := da; i < len(f.id); i++ {
		for k > 0 && f.id[i] != schema[k] {
			k = pi[k-1]
		}
		if f.id[i] == schema[k] {
			k++
		}
		if k == m {
			inizio := i - m + 1
			if f.apre[inizio] && f.chiude[i] {
				return inizio
			}
			k = pi[k-1]
		}
	}
	return -1
}
