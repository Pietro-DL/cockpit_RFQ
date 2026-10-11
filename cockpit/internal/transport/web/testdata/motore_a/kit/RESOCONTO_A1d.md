# Resoconto del frontend A1d (v1.1): la Distinta in un solo workspace

Che cosa è la pagina, che cosa è cambiato rispetto al frontend parziale, che cosa si prova e dove, che cosa resta
aperto. Scritto per il kit: niente dati reali, nessun percorso di macchina. Gli esiti delle singole esecuzioni (comandi,
numeri, mutazioni) stanno nel resoconto della consegna di A1d, non qui.

## 1. La pagina

`GET /thread/{id}/motore-a`, solo per l'admin e solo con `[motore_a].anteprima` accesa. Aprirla non scrive niente: il
server legge la RFQ in una transazione di sola lettura, calcola in memoria e rende il template. I pulsanti sono i gesti
che il Cockpit ha già (le stesse rotte del Fascicolo e della Distinta) e scrivono solo quando una persona li preme.

| Regione | Ancora | Che cosa mostra |
|---|---|---|
| R1 testa | `header#ma-testa` | cliente, oggetto, riferimento; fase registrata, stato del thread e fascicolo della RFQ in tre posti diversi; le strisce (errore, thread non valutato, sola lettura) |
| R2 viste e prodotti | `nav#ma-viste`, `nav#ma-prodotti` | Distinta · Documenti · Riepilogo RFQ; lo stato di ogni prodotto |
| R3 conferma della fonte | `section#ma-fonte` | la fonte, una riga per struttura candidata («nessuna è scelta»), gli altri candidati |
| R4 conferma dell'albero | `section#ma-albero` | la BOM da verificare, «vale per tutta la RFQ (n prodotti)», la nota U-07; **sopra** le schede |
| R5 albero a schede | `section#ma-schede-albero` | una scheda per gruppo (`bom`, `cand:…`, `senza`); caselle con tipo, codice, quantità «dal file» e «confermata»; tratteggio = legame del file, pieno = anche relazione confermata |
| R6 anteprima | `aside#ma-anteprima` | prima il disegno (pdf.js, pagina 1), poi alternativi e revisione, poi Codice, Tipo, Posizione («Nel file» e «Relazioni confermate correnti»), Disegni, Motivi, Gesti; resta dopo ogni rilettura; sotto 860 px è un foglio dal basso |
| R7 conflitti della struttura | `details#ma-conflitti-struttura` | i conflitti dell'asse della struttura, con la rimozione da accettare o scartare |
| R8 da smistare | `section#ma-da-smistare` | per file: miniatura, motivo e disponibilità affiancati, «del file» e «dal messaggio», «Associa a…» |
| R9 riepilogo documentale | `section#ma-documenti` | una tabella: Associazione, Conferma documentale, Copia sul NAS; previsti in grigio; un solo «Riprova le copie» |
| R10 riepilogo della RFQ | `div#ma-riepilogo` (solo `vista=rfq`) | i sette assi, i prodotti, il fascicolo, i conflitti, il confronto con il calcolo vecchio |

Le cose da non confondere stanno in posti distinti: struttura candidata (una scheda `cand:…` con «proposta dal file,
non è la BOM»), fonte confermata (R3), BOM di lavoro (scheda `bom`), BOM verificata (R4); e le tre conferme restano tre
gesti diversi: della fonte (R3), dell'albero (R4), documentale (R8, R9).

## 2. Che cosa è cambiato rispetto al frontend parziale

- **Un solo workspace** al posto dei tre passi: `distinta` e `documenti` rendono lo stesso markup; con `documenti` il
  riepilogo documentale è evidenziato, portato in vista e prende il fuoco (anche dopo una rilettura).
- **I tre difetti corretti**: `qta` obbligatoria senza valore già messo (Collega, Sposta, trascinamento); chiavi
  `gruppo|rif` per caselle e pannelli; «Associa a…» con i prodotti del file e `scelta` solo con `documento`.
- **Altre correzioni**: «Da» di Sposta e Scollega dalle relazioni confermate correnti; «Salva rev e descrizione» acceso
  solo con la descrizione registrata; «Ripristina» per un componente archiviato; nessuna rilettura doppia dopo
  «Conferma l'albero»; nessuna casella scelta da sola all'apertura; miniature vere dei PDF (al più due documenti aperti
  insieme, cache e nuovo tentativo dopo 60 s).
- **Tolti**: «Prossimo gesto», passi, binario, viste della distinta, conteggio dei tipi, «dato non ancora disponibile
  (EB7)», i doppioni di «Riprova le copie» e «Rifai l'analisi».

La tabella completa dei gesti è in `GESTI_A1d.md`.

## 3. Che cosa la pagina prende dal backend di A1d

Il dato della pagina (`paginaMotoreA`, con `Scrive` e `Indice` fuori dal JSON), la funzione `etichetta`, l'indice con le
sue ricerche (25, il template ne usa 23), l'anteprima JSON dello STEP, il ramo `motore-a` dell'esito dei gesti, il ponte
`motore_a_gesti.mjs`, la voce `motore_a.html` in `Init`. Che cosa risolve ogni richiesta del frontend è in
`RICHIESTE_backend_A1d.md`.

## 4. Le prove e dove si eseguono

| Prova | Che cosa controlla | Dove si esegue |
|---|---|---|
| QFE-01 casi ACME (`web/motore_a_casi_test.go`) | 17 casi costruiti nei tipi veri e letti in modo stretto | Go L1: PC di sviluppo e PC originale |
| QFE-02 rendering (`web/motore_a_render_test.go`) | template vero, pagina e frammento, ogni caso × vista × prodotto × `Scrive`; controlli generici e 13 funzioni mirate | Go L1: PC di sviluppo e PC originale |
| QFE-03 pagina in jsdom (`kit/prove/prove.cjs`) | 21 gruppi: comportamento di `motore_a.mjs` sul markup reso (README) | Node e jsdom: PC di sviluppo; sul PC originale solo se ci sono Node e jsdom |
| QFE-04 ponte in jsdom (`kit/prove/ponte.cjs`) | il ponte vero con un `fetch` finto | come QFE-03 |
| QFE-05 controlli statici (`web/motore_a_statici_test.go` e `node --check`) | niente HTML dal JS, niente indirizzi esterni, CSS solo dentro la pagina, vecchio impianto tolto | Go L1 e Node |
| prova con il DB (L4) e prova nel browser (BE-19) | gesti veri senza scritture all'apertura, miniature, anteprima dopo la rilettura | solo sul PC originale (DB, browser): scritte e non eseguite sul PC di sviluppo |
| aspetto a 1440 e 420 px, PDF veri, anomalie del NAS, L7 | — | solo sul PC originale |

Le mutazioni che ogni prova deve far cadere (MF01…MF22 per la pagina, MB01…MB08 per il ponte) sono nel piano del
frontend (§9.5); si fanno su copie fuori dal repository.

## 5. Funzioni rinviate (contratto §6)

Visibili, spente, con «non ancora disponibile» e il motivo: conferma cumulativa e conferma della minuteria, carica dalla
riga, «Cerca sul portale», Ciclo, Dati per i costi, Fattibilità, nuova Inbox, passaggi di fase automatici, «Congela la
V1», R-02…R-05, rinomina del prodotto della RFQ (DB7-3), aggiunte di pezzi (bozza vuota). «Conferma il documento» con la
domanda di revisione resta fuori dalla v1.1 (U-08, caso e19).

## 6. Aperto

- I contrasti di dominio di P-14 (elenco in `RICHIESTE_backend_A1d.md`): la pagina mostra il comportamento del server,
  non lo decide.
- U-08; i testi delle etichette che il backend firma (D-RAC-01); la fase in testa (descrizione del catalogo o nome della
  fase, D-RAC-03); l'importmap provato solo con un Go più recente di quello del PC originale (D-RAC-05).
- La ratifica delle scelte tecniche T-A1d-FE-01…06 (chiavi di gruppo opache, colonna «Associazione», miniature come
  `blob:`, container query, pannelli resi dal server, «Associa a…» con i prodotti del file).
- Le prove sulla `_run` e le L7, con la loro scheda.

Le prove del kit **non sono prove di integrazione con il database**: dicono che il template si rende e che la pagina si
comporta come il contratto descrive, non che il backend vero risponde così.
