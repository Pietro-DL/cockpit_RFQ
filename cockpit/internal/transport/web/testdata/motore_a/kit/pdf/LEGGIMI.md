# I PDF sintetici del kit

Due PDF di prova ACME, scritti a mano (PDF 1.4, testo semplice): nessun dato reale, nessun dizionario `/Info`, nessun
metadato XMP (né autore, né produttore, né date), nessun carattere incorporato (Helvetica, uno dei 14 standard), nessun
JavaScript, nessuna immagine. Pagina A4 orizzontale (842 × 595 punti), una cornice e due o tre righe di testo ACME.

| File | Pagine | Byte | sha256 | Testo |
|---|---|---|---|---|
| `acme_disegno_a4.pdf` | 1 | 833 | `c1e9400cb8c0cc5676824a393cb134d1aac571424707d3aa6123a3f71a338686` | «ACME 7120110 - disegno di prova» |
| `acme_disegno_2pagine.pdf` | 2 | 1206 | `a853b832f2f20362345856baed56c684a31831692d60cea2f430836fcae7ede8` | «ACME 7120100 - disegno di prova, pagina 1 di 2» e «… 2 di 2» |

Le tabelle `xref` hanno gli scostamenti giusti e le `/Length` dei flussi sono esatte: un lettore PDF li apre senza
riparazioni.

**Chi li usa.** `prove/prove.cjs`: sono i byte che il `fetch` finto restituisce per `GET /allegato/<id>/anteprima?v=<sha>`
(quello di una pagina per le miniature; quello di due pagine per il disegno dell'anteprima, così la pagina deve
scrivere «pagina 1 di 2», e per il visore, che pdf.js finto apre per indirizzo). Il pdf.js finto (`prove/pdfjs_finto.cjs`) legge dai byte solo l'intestazione
`%PDF-`, il numero di pagine e la `/MediaBox`. Le risposte 404, 409, 415, il rinvio al login e «non è un PDF» le
simula il `fetch` finto, senza file.

Si possono usare anche per provare a mano il visore della pagina con un PDF che sicuramente non contiene dati reali.
