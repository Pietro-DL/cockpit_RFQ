package estrazione

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Gli offset del motore A (par.3.4.4; 5.4.8): byte UTF-8, 0-based, fine esclusa, sul testo originale indicato da
// TestoOriginale.ID, mai su un testo passato da TrimSpace, normalizza o ToUpper. Ogni intervallo cade su un
// confine di runa. Dai fatti del worker non arriva nessun offset: i suoi conteggi sono in code point Python,
// e non si confrontano mai con len() di Go.

// intero: l'intervallo di tutto il testo, [0, len(s)).
func intero(s string) evidenze.Intervallo { return evidenze.Intervallo{Inizio: 0, Fine: len(s)} }

// lunghezzaPython: la lunghezza di s come la conta il worker, in code point Python. Una runa sopra U+FFFF conta
// 2, perché il worker la ricostruisce da una coppia surrogata di \X2\ (step_struttura.py:535-560); le altre
// contano 1. Con \X4\ il conteggio vero può essere 1: per questo un testo che arriva al tetto del worker è
// solo «candidato» troncato (5.4.8). Una sequenza non UTF-8 conta un byte alla volta, come la decodifica di Go.
func lunghezzaPython(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// runeDi: la lunghezza in rune, come la misura l'acquisizione ([]rune, ingest.go:640-645).
func runeDi(s string) int { return utf8.RuneCountInString(s) }

// daCoppia: l'intervallo di una coppia [inizio, fine) in byte, come la danno il taglio e i livelli della storia
// di classificazione (che non importa la foglia, quindi usa [2]int).
func daCoppia(iv [2]int) evidenze.Intervallo { return evidenze.Intervallo{Inizio: iv[0], Fine: iv[1]} }

// ripulito: l'intervallo iv di s senza gli spazi ai bordi, con gli spazi di strings.TrimSpace (unicode.IsSpace).
// Un intervallo di soli spazi diventa vuoto alla sua fine. Serve per il testo di un'unità, che non porta gli
// spazi intorno al suo segmento; il segmento resta intero.
func ripulito(s string, iv [2]int) evidenze.Intervallo {
	t := s[iv[0]:iv[1]]
	a := iv[0] + len(t) - len(strings.TrimLeftFunc(t, unicode.IsSpace))
	b := iv[0] + len(strings.TrimRightFunc(t, unicode.IsSpace))
	if b < a {
		b = a
	}
	return evidenze.Intervallo{Inizio: a, Fine: b}
}
