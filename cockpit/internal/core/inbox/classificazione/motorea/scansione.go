package motorea

import (
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// scandisci: le occorrenze di un piano su un testo, per un selettore (che decide l'attribuzione degli
// affissi).
//   - Si prova ogni inizio di runa dove il confine sinistro è rispettato: la runa prima si legge sul testo
//     intero, perché la ricerca su testo[i:] non la vede (T4). Un inizio con una runa che non può cominciare
//     la forma non si prova nemmeno: è solo un risparmio, non cambia le letture.
//   - Il match è ancorato a quell'inizio; la lettura è il gruppo g0, senza il carattere del confine destro.
//   - Dopo un inizio, con o senza match, si passa alla runa successiva, mai alla fine del match: così il
//     codice accanto non si perde (T2, T5, T13).
//   - Le letture di uno stesso piano che cominciano in punti diversi restano tutte (P1 §7.3).
//
// Gli inizi si cercano nei primi max_byte_unita byte; le letture sono al più max_letture_per_unita, le prime
// nell'ordine del testo, e allora la diagnostica dice che il risultato è parziale (limiti.go). La
// diagnostica del limite dei byte la dà Riconosci, una volta per testo.
func scandisci(p *pianoForma, sel evidenze.Selettore, testo string, lim grammatica.LimitiRiconoscimento) ([]LetturaForma, []evidenze.Diagnostica) {
	fine, _ := fineInizi(testo, lim)
	var out []LetturaForma
	for i := 0; i < fine; {
		r, w := utf8.DecodeRuneInString(testo[i:])
		if p.prime.contiene(r) && confineSinistro(p.confinePrima, testo, i) {
			if loc := p.re.FindStringSubmatchIndex(testo[i:p.fineRicerca(testo, i)]); loc != nil {
				if lim.MaxLetturePerUnita > 0 && len(out) == lim.MaxLetturePerUnita {
					return out, []evidenze.Diagnostica{diagnosticaLetture(lim.MaxLetturePerUnita)}
				}
				out = append(out, p.lettura(sel, testo, i, p.allunga(testo, i, loc)))
			}
		}
		i += w
	}
	return out, nil
}

// margineConfine: i byte che il confine destro può consumare dopo la lettura, al più due rune («.» e una
// runa che non è una cifra).
const margineConfine = 2 * utf8.UTFMax

// fineRicerca: fin dove la regex del piano deve vedere il testo da i. Ogni match sta nei maxByte byte della
// lettura più quelli del confine: con una finestra di quella lunghezza le letture sono le stesse che sul
// testo intero, e il «\z» del taglio non crea un confine falso, perché una lettura non arriva mai fin lì.
// Serve a non far lavorare la regex su tutta la coda del testo a ogni inizio. Senza un limite, tutto il testo.
func (p *pianoForma) fineRicerca(testo string, i int) int {
	if p.maxByte < 0 || len(testo)-i <= p.maxByte+margineConfine {
		return len(testo)
	}
	return i + p.maxByte + margineConfine
}

// allunga: la lettura più lunga che comincia in i e rispetta il confine destro. Con Longest() la regex dà il
// match più lungo, confine compreso; a parità di lunghezza del match, però, la lettura può essere più corta
// di una runa o due, se il confine ha consumato un carattere che poteva stare nella lettura (per esempio una
// parte finale facoltativa fatta di un solo segno). Il confine è lungo al più due rune, quindi la lettura più
// lunga finisce fra la fine della lettura trovata e la fine del match: si provano quelle posizioni, dalla più
// lontana, con la sequenza ancorata alla fine e il confine controllato in Go. Senza un'alternativa resta la
// lettura trovata.
func (p *pianoForma) allunga(testo string, i int, loc []int) []int {
	fineLettura, fineMatch := loc[3], loc[1] // g0 è il gruppo 1
	for j := fineMatch; j > fineLettura; {
		if confineDestro(p.confineDopo, testo, i+j) {
			if alt := p.reIntera.FindStringSubmatchIndex(testo[i : i+j]); alt != nil {
				return alt
			}
		}
		_, w := utf8.DecodeLastRuneInString(testo[i : i+j])
		j -= w
	}
	return loc
}
