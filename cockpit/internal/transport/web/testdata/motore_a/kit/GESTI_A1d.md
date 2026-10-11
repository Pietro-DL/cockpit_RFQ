# I gesti della Distinta nuova (A1d v1.1)

Una riga per gesto: rotta, campi, obblighi e quando è spento, domanda prima dell'invio, posto nella pagina (regioni
R1–R10 del piano del frontend §2). È il §5 del piano del frontend, nella forma della v1.1.

- Tutti `POST` con campi di form attraverso il ponte (`data-ma-gesto` su un `button` o su un `form`, oppure
  `Gesti.esegui` da `motore_a.mjs` per lo STEP e il tipo), salvo «Conferma l'albero» (JSON, `Gesti.confermaAlbero`).
- `{id}` = `.Dati.ThreadID`; il ponte rifiuta, senza nessuna richiesta, una rotta fuori dalla lista chiusa (sotto), di
  un'altra RFQ o di un'altra origine.
- **Spento** = `disabled` sul pulsante o `<fieldset disabled>` nel form quando `.Dati.Scrive` è falso: con `Scrive` falso
  tutto è spento, anche il trascinamento.
- **Risposta**: `200` con un solo `<div data-ma-esito="ok|no">testo</div>`; «ok» senza testo diventa «Fatto: il server
  non ha aggiunto altro.» (P-09); un 4xx/5xx o una risposta senza `data-ma-esito` è «errore». L'esito va in
  `#ma-avviso[data-esito]`. Le frasi di rifiuto che il server manda con HTTP 200 sono «no» per decisione del server
  (P-07 b, elenco chiuso T-A1d-11): il ponte non classifica il testo.
- **Rilettura**: solo dopo «ok», e la fa il ponte (`htmx.ajax GET` della pagina in `#ma`), anche dopo «Conferma
  l'albero» (X-08). Dopo «no» o «errore» niente si rilegge e niente cambia nella pagina, salvo `#ma-avviso`.
- Nessun gesto parte all'apertura (DB7-6: niente «Prepara i file» automatico); nessun `hx-post`, nessun `hx-trigger`.
- Gli ID vengono solo dai campi del contratto (§1.9) e dalle ricerche dell'indice, **mai** da un `Rif` spezzato:
  nessuna rotta e nessun campo contiene `nodo:` o `componente:`.

## A. Thread e fonte

| Gesto | Rotta | Campi | Obblighi e spento | Domanda | Dove |
|---|---|---|---|---|---|
| Prepara i file | `/thread/{id}/fascicolo/prepara` | — (mai `auto`) | mai al caricamento (DB7-6) | — | R1, menu del thread |
| Rifai lo smistamento | `/thread/{id}/smistamento/aggiorna` | — | — | — | R1, menu del thread |
| Rifai l'analisi | `/thread/{id}/fascicolo/rianalizza` | — | — | — | R3, una sola volta |
| Autorizza lo STEP strutturale… | prima la lettura `GET /thread/{id}/motore-a/anteprima/step-strutturale?componente={cid}&documento=…` oppure `&nodo=…`, `&sorgente=…`; poi `POST /thread/{id}/fascicolo/componente/{cid}/step-strutturale` | `documento` **o** `nodo`, `sorgente`, `firma`, `autorizza=1`, `presa_d_atto=1` se chiesta, `raggruppamento[]`, `delega[]` | `cid` = `ComponenteID` del prodotto; gli ID da `Indice.AutorizzazioneDiStruttura` (prodotto, allegato, radice); senza componente o senza risultato: spento con il motivo (D-RAC-02, P-14). Nella finestra «Autorizza» si accende solo con `inviabile` vero, la `firma`, la casella «autorizzo» spuntata da chi guarda (mai in partenza) e la presa d'atto se chiesta; una delega spenta non si sceglie | la finestra dell'anteprima: frasi, figli diretti, deleghe con il motivo | R3, una per struttura candidata |
| Revoca l'autorizzazione | `/thread/{id}/fascicolo/componente/{cid}/step-strutturale/revoca` | `sha` = `Fonte.Riferimento.Sha256`, `motivo` facoltativo (al più 200 caratteri) | solo con `ComponenteID` e la fonte confermata | «Revocare lo STEP autorizzato di {codice}? I figli diretti tornano guida e la BOM di lavoro si svuota. Le decisioni già prese restano.» | R3 |

Dopo un «no» dell'autorizzazione la finestra resta aperta con il motivo, l'anteprima si rilegge e «autorizzo» torna
vuota; dopo un «ok» la finestra si chiude e la pagina si rilegge.

## B. Struttura

`pid` = `Nodo.RigaLegacy.ID` o `Nodo.RigaDecisa.ID`; `cid` = `Nodo.Decisione.ComponenteID`.

| Gesto | Rotta | Campi | Obblighi e spento | Domanda | Dove e note |
|---|---|---|---|---|---|
| Leggi il riepilogo e conferma l'albero… | prima `GET /thread/{id}/distinta/albero/riepilogo`, poi `Gesti.confermaAlbero` → `POST /thread/{id}/distinta/albero/conferma` | JSON `{"firma": r.firma, "bozza": {"formato": 1, "base": r.albero_firma}}` (bozza vuota in v1) | «Conferma l'albero» acceso solo con `confermabile` vero e la `firma` | il riepilogo è la domanda | R4, **sopra** le schede; vale per tutta la RFQ (n prodotti); 422 = errore nella finestra e riepilogo riletto; 200 = la finestra si chiude e rilegge il ponte |
| Accetta il nodo | `/thread/{id}/fascicolo/nodo/{pid}/accetta` | `tipo` fra `""`, `finito`, `sottoassieme`, `sciolto` (mai `commerciale`) | con `RigaLegacy` | — | R6; la nota U-07 sopra il modulo |
| Correggi il codice (nodo) | `…/fascicolo/nodo/{pid}/codice` | `codice` (obbligatorio, precompilato con il codice a mano), `rev` (si manda sempre, precompilata con la rev a mano) | con `RigaLegacy` | — | R6; sotto: «la rev si manda sempre: un campo vuoto cancella quella che c'è» |
| Scarta il nodo | `…/fascicolo/nodo/{pid}/scarta` | — | con `RigaLegacy` | «Scartare il nodo {catena}? Lo scarto è definitivo: la proposta esce dalla BOM di lavoro; i figli non vengono scartati.» | R6, gesti pericolosi |
| Riapri il nodo | `…/fascicolo/nodo/{RigaDecisa.ID}/riapri` | — | solo con la riga decisa `scartata` | — | R6 |
| Correggi il codice del componente | `/thread/{id}/fascicolo/componente/{cid}/codice` | `codice` (obbligatorio) | **spento per il componente del prodotto della RFQ** (DB7-3), con il motivo | «Cambiare il codice del componente {codice}? Cambia anche sui documenti non ancora copiati.» | R6 |
| Salva rev e descrizione | `…/componente/{cid}/modifica` | `rev` (al più 10 caratteri), `descrizione` (al più 200): **sempre tutti e due**, nessun `tipo` | acceso e precompilato **solo** se `Indice.Componente` ha la descrizione registrata; altrimenti spento con il motivo: «la descrizione registrata del componente non c'è: quella mostrata può venire dallo STEP e il gesto la riscriverebbe» | — | R6 (nuovo, caso e16) |
| Vedi l'effetto del cambio… (tipo) | prima `GET /thread/{id}/distinta/tipo?componente={cid}&tipo=…`, poi `Gesti.esegui` → `POST …/componente/{cid}/tipo` | `tipo`, `firma` | `select name=tipo` obbligatorio con «scegli» spento e selezionato: nessuna preselezione | l'anteprima dell'effetto | R6 |
| Collega | `…/componente/{cid}/collega` | `padre` (obbligatorio, «scegli» spento e selezionato; i componenti del prodotto tranne sé), **`qta` (`type=number required min=1 step=1`, senza valore né segnaposto)** | con `Decisione` | — | R6; testo: «Scrivi la quantità: non c'è un valore già messo. Su un legame che c'è già, Collega cambia la quantità.» |
| Sposta | `…/componente/{cid}/sposta` | `da`: senza relazioni confermate correnti un campo nascosto vuoto e il testo «Da: la radice (nessuna relazione confermata)», altrimenti un `select` obbligatorio con «scegli» spento e selezionato e un'opzione per relazione confermata corrente (codice del padre, «✓×qta», «anche nel file» se c'è); `a` obbligatorio («scegli», poi «— diventa radice» con valore vuoto, mai preselezionata, RAC-16); **`qta` come in Collega** | con `Decisione` | — | R6 e trascinamento (solo nelle schede `bom` e `senza`, il dialogo ha la stessa `qta`); «Da» viene da `Indice.Legami(…).Confermati`, non dai legami del file |
| Scollega da {codice} | `…/componente/{cid}/scollega` | `padre` = il padre della relazione confermata | un modulo per ogni relazione confermata corrente | «Togliere {codice} da sotto {codice del padre}? Il componente resta; se non ha altri padri diventa una radice.» | R6 |
| Archivia | `…/componente/{cid}/archivia` | `motivo` (obbligatorio, al più 200 caratteri) | con `Decisione` | «Archiviare {codice}? Esce dalla BOM di lavoro con i suoi legami; documenti, proposte e storia restano.» | R6, gesti pericolosi |
| Rimuovi | `…/componente/{cid}/rimuovi` | — | con `Decisione` | «Rimuovere del tutto {codice}? Il componente e i suoi legami spariscono e non si recuperano. Riesce solo se non ha storia; altrimenti usa «Archivia».» | R6, gesti pericolosi |
| Ripristina | `/thread/{id}/fascicolo/componente/{cid}/ripristina` | — | solo per il componente archiviato della riga decisa (`Indice.Componente(…).Archiviato`; il `{cid}` viene da `RigaDecisa.ComponenteID`) | — | R6 (nuovo, caso e16) |
| Accetta il legame | `/thread/{id}/fascicolo/relazione/accetta` | `allegato`, `padre`, `figlio` da `Indice.RelazioneProposta` | solo per un legame del file non deciso con una riga aperta; senza: «nessuna proposta aperta per questo legame» | — | R6 Posizione; la nota U-07 sopra |
| Scarta il legame | `…/fascicolo/relazione/scarta` | idem | idem | «Scartare il legame proposto? Il legame non entra nella BOM.» | R6 Posizione |
| Accetta la rimozione | `/thread/{id}/fascicolo/rimozione/accetta` | `step`, `padre`, `figlio` da `Indice.Rimozione` | solo per un conflitto `arco`/`rimozione` con il risultato | «Togliere questo legame dalla BOM di lavoro? Lo STEP autorizzato non lo contiene più.» | R7 |
| Scarta la rimozione | `…/fascicolo/rimozione/scarta` | idem | idem | — | R7, «il legame resta» |
| Sostituisci il principale con questo | `/thread/{id}/fascicolo/documento/{did}/sostituisci` | `vecchio` = il documento del principale, `motivo` | solo fra due documenti; altrimenti spento con il motivo | «{alternativo} sostituisce {principale} come disegno corrente? Il vecchio resta nella storia ma non è più il corrente.» | R6 Disegni, cassetto di R9 |

**La nota U-07, alla lettera** (R4 sotto il pulsante; R6 sopra «Accetta il nodo» e «Accetta il legame»): «La strada
principale è "Conferma l'albero", dopo il riepilogo. Le decisioni nodo per nodo non rendono la BOM verificata e, prese
prima del riepilogo, possono lasciarla da verificare.»

**Su una scheda candidata**, in testa al pannello: «Proposta dal file, non è la BOM. I gesti dei nodi possono essere
rifiutati finché lo STEP non è autorizzato: lo dice il server.» Nelle schede candidate non c'è trascinamento.

## C. Documenti

`voce` = `VoceFabbisogno.PropostaAperta`, oppure `Indice.PropostaAperta` dell'allegato per un file da smistare.

| Gesto | Rotta | Campi | Obblighi e spento | Domanda | Dove e note |
|---|---|---|---|---|---|
| Conferma il documento | `/thread/{id}/fascicolo/conferma` | **esattamente una `voce`** (`data-ma-campi='{"voce":"<uuid>"}'`) | il pulsante c'è solo con la proposta aperta: mai senza `voce`, mai due | R9: «Confermare {file} per {codice}? La copia sul NAS parte subito[; la struttura non è ancora verificata]. Dopo la copia codice e documento non si correggono più.»; R8: «Confermare {file} con il tipo e il componente che propone? La copia sul NAS parte subito[; la struttura non è ancora verificata].» La parte fra quadre c'è **solo** con la BOM del prodotto non verificata (DB7-2) | R8, R9; confermato non vuol dire copiato |
| Scarta il file | `/proposta/{pid}/scarta` | — (il ponte aggiunge `ritorna_thread`) | con la proposta aperta | «Scartare il file {nome}? Lo scarto è definitivo: nessun gesto lo riporta indietro.» | R8, cassetto di R9 (DB7-8) |
| Associa a un componente (file da smistare) | `/thread/{id}/fascicolo/assegna` | `proposta`, `componente` (obbligatorio; «scegli il componente» spento e selezionato) | le opzioni sono i componenti **dei prodotti del file**: «del file», poi «dal messaggio»; senza prodotti quelli del prodotto scelto, con l'avviso. **Nessuna `scelta`**: per una proposta la domanda la fa la conferma | — | R8; pulsante «Associa a questo componente» |
| Associa a un componente (voce, con la proposta) | idem | `proposta`, `componente` (i componenti del prodotto) | idem | — | cassetto di R9 |
| Associa a un componente (voce, con il documento) | idem | `documento`, `componente` (obbligatorio, `data-ma-scelte`); per ogni componente con documenti correnti dello stesso tipo (`Indice.Correnti`) un `fieldset[data-ma-scelta-per=<componente>]` spento e nascosto con le risposte `scelta` (obbligatorie, nessuna spuntata): «aggiungi» e un `sostituisci:<documento>` per corrente; `motivo` facoltativo | il modulo accende solo il `fieldset` del componente scelto; senza JS restano spenti e il server rifiuta elencando i correnti (nessuna scrittura sbagliata). `nuovo_riferimento` non è offerto | — | cassetto di R9 |
| Togli dal componente | idem, `componente` vuoto | `proposta` o `documento` | idem | «Togliere {file} da {codice}? Il file resta nel thread e non viene scartato: perde solo il componente.» | cassetto di R9 (disassociazione) |
| Deroga | `/thread/{id}/fascicolo/componente/{cid}/deroga` | `tipo` = il tipo del documento, `motivo` (obbligatorio, al più 200 caratteri) | senza deroga e con la voce non presente | — | cassetto di R9; «Una deroga non sostituisce il 2D.» |
| Revoca la deroga | `/thread/{id}/fascicolo/deroga/{did}/revoca` | — | con la deroga | «Revocare la deroga? Il requisito torna a contare.» | cassetto di R9 |
| Riprova le copie sul NAS | `/thread/{id}/riprova-copie` | — | **un solo pulsante nella pagina** | — | R9 |

**Tre operazioni da non confondere**, con pulsanti, testi e domande diversi: lo **scarto del nodo** (R6,
`nodo/{pid}/scarta`), lo **scarto documentale** (R8 e cassetto di R9, `/proposta/{pid}/scarta`), la **disassociazione**
(cassetto di R9, `assegna` senza componente).

## D. Sola lettura (nessuna scrittura)

| Gesto | Che cosa fa |
|---|---|
| Aggiorna | `Gesti.aggiorna()`: rilegge `#ma`; l'unica chiamata di `aggiorna` dal modulo |
| `{ } Dati` | `Gesti.leggi("/thread/{id}/motore-a/dati")`: il JSON dell'esito, mai con htmx |
| Apri il visore | `GET /allegato/{allegato}/anteprima?v=<sha256>` disegnato con pdf.js in una finestra fuori da `#ma`; mai un `<iframe>` (la risposta ha `Content-Security-Policy: sandbox`) |
| Miniature e disegno dell'anteprima | la stessa GET con una `fetch` semplice, **senza** intestazioni `HX-*` (con `HX-Request` il server risponde 400), solo per gli elementi con `data-ma-allegato`, al più due documenti insieme; 404, 409, 415 → «anteprima non disponibile: <testo della risposta>» |
| Viste e prodotti | `hx-get` della pagina con `hx-push-url` |

## E. Spenti, con il motivo «non ancora disponibile» (contratto §6), ognuno in un posto solo

Conferma cumulativa, conferma della minuteria (R-08), carica dalla riga (R-06), «Cerca sul portale» → testata di R9;
Ciclo, Dati per i costi, Fattibilità, nuova Inbox, passaggi di fase, R-02…R-05, «Congela la V1» → R10; rinomina del
prodotto (DB7-3) → R6 Gesti; aggiunte di pezzi (bozza vuota, DB7-5, DB7-7) → strumenti dell'albero. «Conferma il
documento» con la domanda di revisione resta fuori dalla v1.1 (U-08): il server la rifiuta e la scelta si fa nel
Fascicolo (caso e19).

## F. La lista chiusa delle rotte

La stessa per il ponte (fra `// ROTTE:inizio` e `// ROTTE:fine` di `motore_a_gesti.mjs`, che le prove leggono di là) e
per i controlli del rendering. `{U}` = un UUID; il primo è la RFQ della pagina.

```
^/thread/{U}/fascicolo/prepara$
^/thread/{U}/smistamento/aggiorna$
^/thread/{U}/fascicolo/rianalizza$
^/thread/{U}/fascicolo/componente/{U}/step-strutturale(/revoca)?$
^/thread/{U}/fascicolo/nodo/{U}/(accetta|scarta|codice|riapri)$
^/thread/{U}/fascicolo/componente/{U}/(codice|modifica|tipo|collega|sposta|scollega|archivia|rimuovi|ripristina|deroga)$
^/thread/{U}/fascicolo/relazione/(accetta|scarta)$
^/thread/{U}/fascicolo/rimozione/(accetta|scarta)$
^/thread/{U}/fascicolo/documento/{U}/sostituisci$
^/thread/{U}/fascicolo/(conferma|assegna)$
^/thread/{U}/fascicolo/deroga/{U}/revoca$
^/thread/{U}/riprova-copie$
^/proposta/{U}/scarta$
(confermaAlbero: /thread/{U}/distinta/albero/conferma, JSON, solo dalla sua funzione)
```

Letture JSON (lista `LETTURE`): `/thread/{U}/motore-a/dati`, `/thread/{U}/motore-a/anteprima/step-strutturale`,
`/thread/{U}/distinta/tipo`, `/thread/{U}/distinta/albero/riepilogo`.

Restano fuori per costruzione: `fascicolo/bom/applica` (DB7-5), `congela`, `revisione/*`, `carica`, `nas/importa`,
`proposta/{pid}/decidi`, `file/{aid}/accetta`, `deroga-struttura`, `step-strutturale/riattiva`, `prepara` con `auto`,
`/proposta/{id}/conferma`. In tutto 29 POST più `confermaAlbero`, e 4 letture JSON (più la pagina e il frammento).

## G. Che cosa è cambiato rispetto al frontend parziale

| # | Prima | Adesso |
|---|---|---|
| 1 | `qta` facoltativa con il segnaposto «1» in Collega, Sposta e nel dialogo del trascinamento (un vuoto valeva 1) | `type=number required min=1 step=1`, senza valore né segnaposto, nei tre posti; il testo chiede la quantità |
| 2 | pannello dell'anteprima con la chiave `Rif`: lo stesso `Rif` in due strutture apriva il primo trovato | chiave `gruppo\|rif` su casella e pannello; la scelta segna le comparse nello stesso gruppo e mostra solo quel pannello; `sessionStorage` salva la chiave intera |
| 3 | «Associa a…» con i componenti del prodotto scelto, nessuna `scelta` | i componenti dei prodotti del file; `scelta` solo con `documento`, con le risposte da `Indice.Correnti` |
| 4 | «Da» di Sposta e Scollega dai legami decisi del file | dalle relazioni confermate correnti (`Indice.Legami(…).Confermati`), anche dopo uno «Sposta» (caso e14) |
| 5 | «Salva rev e descrizione» sempre spento | acceso e precompilato con la descrizione registrata (caso e16) |
| 6 | nessun «Ripristina» | per il componente archiviato della riga decisa (caso e16) |
| 7 | `Gesti.aggiorna()` dopo «Conferma l'albero» (doppia rilettura) | tolto: rilegge il ponte (X-08) |
| 8 | la frase DB7-2 nella domanda di «Conferma il documento» | invariata nel testo, solo con la BOM non verificata; ora la fissa una prova |
| 9 | «Prossimo gesto», passi, binario, viste della distinta, conteggio dei tipi, «dato non ancora disponibile (EB7)», «Riprova le copie» e «Rifai l'analisi» ripetuti | tolti (FE-01…FE-03, U-06, X-07) |
