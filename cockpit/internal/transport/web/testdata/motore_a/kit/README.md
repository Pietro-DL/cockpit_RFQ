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
