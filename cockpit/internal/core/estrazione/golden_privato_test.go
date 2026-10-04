//go:build privato

// L1 — la riscrittura dei golden si rifiuta nelle corse con il tag privato (piano A, 5.6.3; A1b-18): questo file
// esiste solo con il tag, e accende il rifiuto in aggiornamentoGolden.
//
// Nessun dato qui: il file dice solo che la corsa è privata. I clienti delle prove di questo pacchetto sono
// inventati (ACME), e un golden non nasce mai da dati reali.
package estrazione

func init() { conTagPrivato = true }
