# Kit del frontend A1d: la Distinta in un solo workspace

Le prove della pagina `/thread/{id}/motore-a` (la Distinta nuova del motore A, solo per l'admin, dietro
`[motore_a].anteprima`) che non sono prove Go: la pagina in jsdom (QFE-03), il ponte dei gesti in jsdom (QFE-04), i loro
doppi di prova e i PDF sintetici. Contratto A1d v1 con le modifiche della v1.1 (un solo workspace, ricerche dell'indice
in più, la rilettura dopo «Conferma l'albero» fatta dal ponte).

**Sta solo nel ramo QA** (commit A1d-Q2 e A1d-Q3), sotto `testdata/` (P-17, RAC-03): lo strumento dei commit lo
riconosce come prova, il go tool lo ignora. Nel kit **nessun file `.go`**, nessun `go.mod` o `go.sum`, nessun
`package.json`, nessun `node_modules`. Solo dati di fantasia ACME: codici della famiglia 712xxxx, cliente «ACME S.p.A.»,
dominio `acme.example`, UUID `00000000-0000-4000-8000-0000000000nn`.

**La pagina mostra e basta.** Stati, assi, motivi, tipi, quantità, fabbisogni e prontezza arrivano dal server; il
template sceglie solo come mostrarli; `motore_a.mjs` fa solo presentazione; ogni scrittura passa dal ponte
`motore_a_gesti.mjs`; dopo un «ok» la pagina si rilegge dal server, un «no» o un errore non cambiano niente. Le prove del
kit controllano proprio questo, sulla pagina resa dal template vero.

## I file

| File | Commit | Che cosa |
|---|---|---|
| `README.md` | A1d-Q2 (il ponte), A1d-Q3 (il resto) | questo file |
| `GESTI_A1d.md` | A1d-Q3 | ogni gesto della pagina: rotta, campi, obblighi, domanda, dove sta; la lista chiusa delle rotte; i tre difetti corretti |
| `RESOCONTO_A1d.md` | A1d-Q3 | la pagina v1.1 in breve: che cosa è cambiato, che cosa si prova dove, che cosa resta aperto |
| `RICHIESTE_backend_A1d.md` | A1d-Q3 | le richieste del frontend (R-FE-01…R-FE-19): quali risolve il backend di A1d e come, quali sono rinviate |
| `prove/prove.cjs` | A1d-Q3 | QFE-03: la pagina in jsdom, sulle pagine rese dal rendering L1 (21 gruppi di controlli) |
| `prove/ponte_finto.mjs` | A1d-Q3 | il doppio del ponte usato **solo** da `prove.cjs`: stessa API, nessuna rete, le risposte da `window.__copione` (K-05: **mai** in `cockpit/web/static/`) |
| `prove/pdfjs_finto.cjs` | A1d-Q3 | il doppio di pdf.js usato da `prove.cjs`: conta worker, documenti aperti insieme e chiusi; aperture «a comando» |
| `pdf/acme_disegno_a4.pdf`, `pdf/acme_disegno_2pagine.pdf`, `pdf/LEGGIMI.md` | A1d-Q3 | due PDF sintetici ACME (una e due pagine), senza metadati, e come sono fatti |
| `prove/ponte.cjs`, `prove/risposte_acme.json` | A1d-Q2 | QFE-04: il ponte vero in jsdom con un `fetch` finto (vedi «Il ponte») |

`web/` sotto = `cockpit/internal/transport/web/`. Da `kit/prove/` sette livelli in su c'è `cockpit/`.

## La pagina v1.1 in breve

- **Un solo workspace** (`.ma-ws[data-ma-ws]`): `vista=distinta` e `vista=documenti` rendono lo stesso markup; con
  `documenti` cambiano solo `data-ma-vista`, la voce corrente in `#ma-viste` e la classe `ma-evidenza` su
  `#ma-documenti`, e il modulo porta in vista il riepilogo documentale con il fuoco sul suo titolo (`#ma-documenti-t`).
  `vista=rfq` è il riepilogo della RFQ, senza workspace.
- **Al centro**: la conferma della fonte (R3), la barra «Conferma l'albero» **sopra** le schede (R4), l'albero a schede
  (R5, `role=tablist`: `bom`, una scheda `cand:<allegato>:<radice>` per struttura candidata, `senza`), nessuna scheda o
  candidata scelta in partenza. **Di lato** l'anteprima persistente (R6) e «Da smistare» (R8). **In fondo** il
  riepilogo documentale (R9) con colonne distinte Associazione, Conferma documentale, Copia sul NAS.
- **I tre difetti del frontend parziale, corretti**:
  1. `qta` obbligatoria (`type=number required min=1 step=1`, senza valore e senza segnaposto) in Collega, Sposta e nel
     dialogo del trascinamento: prima un campo vuoto partiva e il server lo leggeva come 1;
  2. chiavi `gruppo|rif` per caselle, pannelli dell'anteprima e `sessionStorage`: prima lo stesso `Rif` in due
     strutture candidate dello stesso file apriva il pannello sbagliato;
  3. «Associa a…» con i componenti **dei prodotti del file** (del file, poi dal messaggio; senza prodotti quelli del
     prodotto scelto, con l'avviso) e `scelta` (`aggiungi` o `sostituisci:<id>`) **solo** quando si associa un
     `documento`, con le risposte prese da `Indice.Correnti`.
- **Tolti**: passi e binario, «Prossimo gesto», viste della distinta, conteggio dei tipi, le scritte «dato non ancora
  disponibile (EB7)» (la pagina vuole `valutazione-3`, X-07, D-CHIUS-3), il doppio «Riprova le copie», il
  `Gesti.aggiorna()` dopo «Conferma l'albero» (la rilettura la fa il ponte, X-08), la scelta automatica di una casella
  all'apertura.

Il dettaglio dei gesti è in `GESTI_A1d.md`, il resto in `RESOCONTO_A1d.md`.

## Come si prova

Tutto dalla cartella `cockpit/`. Nessuna rete, nessun DB, nessun browser.

**1. Le pagine (QFE-02, prova Go del pacchetto `web`)**: il rendering L1 con il template vero, per ogni caso ACME
(`web/testdata/motore_a/e*_*.json`), vista, prodotto e `Scrive`, pagina intera e frammento. Con `MA_HTML` scrive anche
le pagine per jsdom:

```sh
MA_HTML=<cartella fuori dal repository> go test -count=1 -run '^TestMotoreAPaginaEFrammentoPerOgniCaso$' ./internal/transport/web/
```

Nella cartella: `<caso>__<vista>__<k>.html`, `<caso>__<vista>__<k>.frammento.html` e `casi.json`
(`[{pagina, frammento, caso, vista, prodotto, scrive}, …]`).

**2. La pagina in jsdom (QFE-03)**:

```sh
NODE_PATH=<cartella con node_modules/jsdom> node internal/transport/web/testdata/motore_a/kit/prove/prove.cjs <cartella di MA_HTML>
```

- Uscita **0** = ogni controllo passa; **1** = almeno un controllo fallito (le righe `FALLITA [gruppo] …`); **2** = non
  si può provare (manca la cartella, `casi.json`, jsdom, `motore_a.mjs`, un PDF del kit, o la riga d'import attesa in
  `motore_a.mjs`): mai un passaggio silenzioso. I risultati vanno in `<cartella>/prove.json`, **mai** nel repository.
- Variabili: `MA_NEGATI=<file FUORI dal repository, un nome per riga>` (i nomi da non trovare mai nelle pagine, nei casi
  e nel kit; si stampano solo mascherati; un file dentro il repository è rifiutato); `SOLO=<regex dei gruppi>`;
  `MA_COCKPIT=<cartella cockpit/>` (predefinita: sette livelli sopra `kit/prove/`); `MA_CASI=<cartella dei casi>`
  (predefinita: `web/testdata/motore_a/`).
- `node …/prove.cjs --solo-kit` controlla soltanto i dati privati nei file del kit e nei casi ACME (senza pagine).
- I 21 gruppi: apertura (nessuna scrittura, nessuna rete, ogni `data-ma-gesto` nella lista chiusa del ponte),
  workspace, documenti, candidate, schede, chiavi, qta, miniature, anteprima, foglio, associa, conferma documento,
  albero, step e tipo, esiti, tre operazioni, vecchio impianto, sola lettura, tastiera dell'albero, e14 ed e16, privati.
  La tabella completa è nel piano del frontend (§9.2); i controlli sono scritti nel file, uno per riga.
- In più, alla chiusura di ogni pagina provata (revisione di A1d-P3, S-01…S-03): ogni `htmx.ajax` è una GET (una
  rilettura), nessun `sendBeacon` e nessun `form.submit()`; nessun attributo `on…` nella pagina; in `sessionStorage`
  solo le chiavi `ma.<thread>.*` del modulo, con valori corti fatti di id, chiavi e cammini.
- Come carica i moduli: jsdom non esegue `<script type="module">`. La pagina si apre con `runScripts: "outside-only"`
  (non parte nessuno script della pagina, né htmx né quelli del layout) e i moduli si valutano con `window.eval` dopo una
  trasformazione del testo: dal ponte finto si tolgono gli `export` e l'API va in `window.__Gesti`; in `motore_a.mjs` la
  riga `import * as Gesti from "./motore_a_gesti.mjs";` diventa `const Gesti = window.__Gesti;` e l'unica `import(…)`
  (il caricatore di pdf.js) diventa `window.__importa(…)`, che restituisce il pdf.js finto.
- La lista chiusa delle rotte non è copiata qui: `prove.cjs` la legge dal ponte vero, fra `// ROTTE:inizio` e
  `// ROTTE:fine` (e `LETTURE`), una fonte sola (RAC-13).
- I doppi che jsdom non ha: `IntersectionObserver` a comando, `ResizeObserver` assente, `URL.createObjectURL` e
  `revokeObjectURL` spie, `toBlob` del canvas, `showModal` e `close` dei `dialog`, `scrollIntoView` spia, `matchMedia`
  e larghezze impostate dalla prova, un orologio finto per il nuovo tentativo delle miniature dopo 60 s, `fetch` spia
  che risponde solo alle GET `/allegato/<id>/anteprima?v=<sha>` (con `HX-Request` risponde 400, come il server),
  `htmx.ajax` che rilegge `#ma` con il frammento dello stesso caso (`htmx:beforeSwap`, sostituzione, `htmx:afterSwap`).

**3. Il ponte vero in jsdom (QFE-04)**: vedi «Il ponte».

**4. La sintassi**:

```sh
node --check web/static/motore_a.mjs
node --check web/static/motore_a_gesti.mjs
node --check internal/transport/web/testdata/motore_a/kit/prove/prove.cjs
node --check internal/transport/web/testdata/motore_a/kit/prove/ponte_finto.mjs
node --check internal/transport/web/testdata/motore_a/kit/prove/pdfjs_finto.cjs
node --check internal/transport/web/testdata/motore_a/kit/prove/ponte.cjs
```

**jsdom si installa fuori dal repository** (per esempio `npm install jsdom@24` in una cartella a parte) e si indica con
`NODE_PATH`. Le prove sono state scritte con Node 18 e jsdom 24.1.3. Sul PC originale Node e jsdom non sono accertati:
finché non si ripetono là, QFE-03 e QFE-04 restano «eseguite solo sul PC di sviluppo».

**Che cosa non sono.** Il ponte finto e il pdf.js finto sostituiscono rete e PDF veri: le prove del kit dicono che la
pagina si comporta come il contratto descrive sulle pagine rese dal template vero, non che il backend vero risponde così.
Le prove con il DB (L4), la prova nel browser (BE-19) e l'aspetto a 1440 e 420 px sono altrove e si eseguono sul PC
originale.

## Il ponte

`cockpit/web/static/motore_a_gesti.mjs` è il ponte dei gesti della Distinta nuova (giro 5, A1d; contratto A1d
§3.1-§3.3; piano A1d backend §5.7): l'unico punto della pagina `/thread/{id}/motore-a` che chiama le rotte che
scrivono. Il kit lo prova **vero**, così com'è nel repository, con due file di `prove/` (A1d-Q2, QFE-04):

- `prove/ponte.cjs` — apre una pagina jsdom all'indirizzo
  `http://cockpit.acme.example/thread/00000000-0000-4000-8000-000000000001/motore-a`, con `#ma-avviso`, `#ma` e
  pulsanti e form con `data-ma-gesto`; carica il ponte togliendo le parole `export` (il ponte non ha `import` né
  `await` fuori dalle funzioni) e lo valuta nella finestra con `runScripts: "outside-only"`. Le spie: un `fetch` finto,
  `htmx.ajax` che sostituisce il figlio di `#ma` con nodi nuovi (oppure lascia tutto com'è, rifiuta, manca, o mette in
  coda come la coda «last» di htmx 2.0.4), `confirm` finto, i setter di `innerHTML`/`outerHTML` e `insertAdjacentHTML`
  contati, la navigazione verso `/login` letta dal `VirtualConsole`. 24 casi: i 20 del piano del frontend §9.3, «fuori
  dalla pagina `motore-a` nessun gesto parte», «`prepara` con `auto` → errore, nessuna fetch» (piano A1d backend §7.3),
  l'UUID della pagina in maiuscolo (RAC-15) e due gesti finiti vicini con una rilettura alla volta (R-05 della
  revisione di A1d-P2).
- `prove/risposte_acme.json` — le risposte del `fetch` finto: gli esiti del ramo `motore-a` di `threadFrammento`
  (`<div data-ma-esito="ok|no">`), le 9 frasi dell'elenco chiuso dei rifiuti con 200 (T-A1d-11), le forme che non
  dicono l'esito (senza `data-ma-esito`, due `div`, una pagina intera, l'HTML della vecchia interfaccia), 401/302 verso
  il login, 403, 422, 500, 400, 404, le risposte JSON di «Conferma l'albero» e delle letture. Solo dati inventati
  (ACME, 712xxxx, UUID `…0000000000nn`, `acme.example`). Le voci con `avviso` sono **byte per byte** quello che scrive
  `rispondiMotoreA`: lo controlla `TestMotoreALeRisposteDelKitSonoQuelleDelServer` (`motore_a_test.go`), con
  `go test`.

Come si lancia, dalla cartella `cockpit/` (Node e jsdom **non** stanno nel repository: si installano fuori, e
`NODE_PATH` porta alla cartella `node_modules` che contiene `jsdom`):

```
node --check web/static/motore_a_gesti.mjs
node --check internal/transport/web/testdata/motore_a/kit/prove/ponte.cjs
NODE_PATH=<cartella>/node_modules node internal/transport/web/testdata/motore_a/kit/prove/ponte.cjs
```

Esce con 0 se ogni controllo passa (`ponte.cjs: 24 casi, … controlli, 0 falliti`), con 1 altrimenti, e scrive i
controlli e i casi caduti. `MA_COCKPIT=<cartella cockpit di una copia>` prova il ponte di quella copia: le mutazioni
(MB01…MB08 del piano del frontend §9.5, M-A1d-07 e M-A1d-21 del piano del backend §10) si fanno così, su una copia
fuori dal repository, mai qui. Provato con Node 18 e jsdom 24.1.3; sul PC originale Node e jsdom non sono accertati.

Le prove statiche del ponte stanno in Go, senza Node: `motore_a_ponte_test.go` (nessuna API che inserisca HTML; la
lista chiusa fra `// ROTTE:inizio` e `// ROTTE:fine`, che è la sola fonte delle rotte, RAC-13; ogni rotta registrata
nel server; ogni `fetch` con `HX-Request` e `HX-Current-URL`; l'API `esegui`, `leggi`, `confermaAlbero`, `aggiorna`).

## Che cosa il kit non contiene più

Il kit di lavoro del frontend parziale aveva altre parti. Restano **fuori dal repository**, per questi motivi:

| Parte | Perché resta fuori |
|---|---|
| il banco di rendering e i tipi specchio di `valutazione`, `ancoraggio`, `confronto`, `motorea` | erano copie ferme a una base vecchia, con campi grezzi: derivano dai tipi veri (K-02). Il rendering vero lo fa QFE-02 nel pacchetto `web`. La tabella delle etichette del banco è diventata la proposta di testo per `etichetta` (BE-10), non un file |
| i comandi `cmd/banco`, `cmd/casi`, `cmd/demo`, `cmd/demowasm` e il `go.mod`/`go.sum` del kit | il kit era un modulo Go suo, che non può importare `internal/` del Cockpit (K-03); i casi ora si costruiscono nei tipi veri (QFE-01, `web/motore_a_casi_test.go`) |
| il server demo (stato, dominio, albero, gesti) e il copione del percorso a tappe | simulava le regole del dominio (K-07): una seconda implementazione delle regole non deve stare nel repository. Le forme delle risposte che servono alle prove del ponte sono dati di `prove/risposte_acme.json` (K-06) |
| la vetrina pubblicabile (pagina statica con la demo) | serviva solo a mostrare la demo (K-07) |
| i casi `e0`, `e6`, `e7a`, `e7b` del kit vecchio | il loro contenuto entra nei casi nuovi `e3`, `e13`, `e17` (D-PF-11) |

## Gli altri documenti

- `GESTI_A1d.md`: ogni gesto, con rotta, campi, obblighi, domanda e posto nella pagina.
- `RESOCONTO_A1d.md`: che cosa è la pagina v1.1, che cosa si prova dove, che cosa resta aperto.
- `RICHIESTE_backend_A1d.md`: le richieste al backend, risolte o rinviate.
