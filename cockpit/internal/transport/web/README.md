---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/transport/web` — le pagine dell'operatore

- Il livello HTTP del browser: legge la richiesta, controlla chi chiama, chiama il pezzo giusto di `core` e rende HTML
  (pagina intera o frammento htmx).
- Circa 36 file e 14 mila righe: qui sono divisi in **moduli**, cioè gruppi di file che servono la stessa schermata.
- Ogni modulo ha la sua sezione con rotte, dati, prove e limiti.

## Regole che valgono per tutto il package

- **Una sola autenticazione:** il cookie di sessione (`autenticato`).
  - Le rotte `/admin/*` passano da `soloAdmin`;
  - `consultazione` non usa mai un metodo che scrive (la regola è per metodo, in un punto solo).
- **Protezione cross-site** (`ProtezioneCSRF`, cioè `http.CrossOriginProtection`) su tutto il mux, rotte dei worker comprese.
- **HTML, salvo le eccezioni dichiarate.** Rispondono con altro:
  - `GET /healthz`: JSON `worker.Salute` (`server.go:healthz`);
  - `GET /thread/{id}/fascicolo/bom/dati`: JSON con `Cache-Control: no-store` (`fascicolo_editor.go:fascicoloDatiEditor`),
    chiesto da `fascicolo.mjs` e da `distinta.mjs` con `fetch`, mai da htmx;
  - `GET /thread/{id}/distinta/dati` (i PDF, le note e la completezza della Distinta) e `GET /thread/{id}/distinta/tipo`
    (l'anteprima di un cambio di tipo, con la firma): JSON con `Cache-Control: no-store` (`distinta.go`), chiesti da
    `distinta.mjs` con `fetch`;
  - `GET /thread/{id}/motore-a/dati` e `GET /thread/{id}/motore-a/anteprima/step-strutturale` (la Distinta nuova del
    motore A, da A1d-P2): JSON con `Cache-Control: no-store` e `nosniff` in ogni risposta dei gestori di `motore_a.go`,
    2xx e 4xx/5xx; solo per l'admin e solo a `[motore_a].anteprima` accesa; si chiedono con `fetch`, mai con htmx. Non
    sono JSON le risposte della catena che non arrivano al gestore: il 404 a flag spento (quello del router), il
    302/401 di `aLogin` e il 403 di `nega` (HTML legacy), il 405 (testo, con `Allow`). Vedi «Il client» nel modulo;
  - `GET /allegato/{id}/anteprima`: i byte del PDF (`anteprima.go:anteprima`), che si aprono solo come documento (iframe,
    pdf.js, scheda nuova);
  - `POST /admin/postazioni/{host}/pacchetto`: lo zip del pacchetto, in download (`postazioni_admin.go:pacchettoWorker`);
  - `GET /static/`: CSS, JavaScript, wasm, font e dati di pdf.js (`server.go:statici`);
  - risposte senza corpo:
    - 204 al poll di `/richieste` con la stessa firma (`panoramica.go:richieste`) e a `/inbox/sync-storico/stato` senza
      job storico (`routes_inbox.go:syncStoricoStato`);
    - 204 con `HX-Refresh` dopo la scelta della postazione (`postazione.go:dopoScelta`) e dopo «riaccoda»/«annulla» di un
      job (`routes_admin.go:riaccodaJob`, `annullaJob`);
    - 401 con `HX-Redirect` per una sessione scaduta chiesta da htmx (`server.go:aLogin`, da `autenticato`);
  - gli errori tecnici (400, 404, 500) escono da `http.Error`, in testo semplice.
- **Pagina o frammento:** `rendi` sceglie il layout completo o il frammento a seconda di `HX-Request`.
  - Molte rotte di navigazione del Fascicolo esistono solo come frammento, ma i collegamenti portano l'`href` della pagina
    intera.
- **Mai HTML della mail nella pagina:**
  - il corpo di un messaggio passa da `core/inbox/lettura.Presenta` e arriva ai template come testo e tabelle estratte
    (`frammenti.html:corpo_messaggio`);
  - l'oggetto passa da `oggettoVisibile`.
  - I template non usano `template.HTML`: tutto passa dall'escape di `html/template`.
  - Un indirizzo scritto nella mail non diventa mai un collegamento.
- **Nella pagina torna solo ciò che il server riconosce:**
  - `sel` dell'Inbox e della pagina RFQ entra solo se è un UUID, riscritto nella forma canonica (`routes_inbox.go:inbox`,
    `thread.go:thread`);
  - lo `stato` della coda job e l'`origine` degli scarti tornano nel poll solo se sono valori noti
    (`routes_admin.go:adminJob`, `adminScarti`).
- **Nessuna risposta non HTML entra fra i pezzi della pagina:**
  - `layout.html` ha un ascoltatore globale `htmx:beforeSwap` che non innesta una risposta 2xx con un corpo il cui
    `Content-Type` non comincia per `text/html`;
  - `GET /allegato/{id}/anteprima` risponde 400 a una richiesta htmx (`HX-Request`).
- **Una transazione per gesto:** `gesto`/`gestoPoi` (`proposte.go`), oppure `Begin`/`defer Rollback`/`Commit` (o
  `pgx.BeginFunc`) nel gestore.
- **La RFQ si blocca per prima:** i gesti sul Fascicolo prendono il lucchetto della RFQ prima di componenti, documenti e
  proposte
  - `fascicolo_rotte.go:preparaGesto` in `fascicolo.go:assegnaAlComponente` e `correggiCodiceComponente`;
  - in `allegati.go:conferma` un `FOR KEY SHARE` sulla RFQ del messaggio prima di `BloccaProposta`.
  - Un deadlock che capita lo stesso (SQLSTATE 40P01) arriva all'operatore come «riprova» (`fascicolo.go:spiegaErrore`).
- **Logica di dominio ancora qui** (da portare in `core`, decisione dell'architetto):
  - la conferma di una proposta (`allegati.go:confermaProposta`),
  - l'assegnazione e la correzione del codice (`fascicolo.go`),
  - l'applicazione del piano (`fascicolo_conferma.go:applicaPiano`),
  - la nuova RFQ e gli agganci (`triage.go`),
  - il nome libero della cartella NAS di una RFQ nuova (`triage.go:cartellaLibera`),
  - le scritture dell'anagrafica (`anagrafica.go`, `censisci.go`),
  - le finestre dello storico e la loro partenza per cartella (`routes_inbox.go:finestraStorico`, `partenzaDellaCartella`,
    `nomiCartellaPredefinita`).
  - **SQL scritto a mano, fuori da sqlc:**
    - `triage.go:cartellaLibera`,
    - `routes_inbox.go:partenzaDellaCartella`,
    - `allegati.go:conferma` (il lucchetto sulla RFQ),
    - `fascicolo.go:correggiCodiceComponente` (`SET CONSTRAINTS … DEFERRED`),
    - `server.go:healthz`.

## Moduli

- **Server e ruoli**
  - File: `server.go`, `ruoli.go`, `navigazione.go`
  - Schermata: guscio, login, divieto, statici
- **Inbox**
  - File: `routes_inbox.go`, `inbox_viva.go`, `agente.go`
  - Schermata: l'Inbox a quadranti e la testata
- **Triage**
  - File: `triage.go`
  - Schermata: Nuova RFQ, Aggancia, Ignora
- **Anagrafica e censimento**
  - File: `censisci.go`, `anagrafica.go`, `anagrafica_admin.go`, `convenzioni_admin.go`, `fornitori_admin.go`
  - Schermata: «Da validare», Admin › Anagrafica e Fornitori
- **Postazioni**
  - File: `postazione.go`, `postazioni_admin.go`
  - Schermata: la postazione della sessione, i pacchetti dei worker
- **Admin tecnico**
  - File: `routes_admin.go`, `integrita_admin.go`
  - Schermata: coda dei job, scarti, integrità del NAS
- **Pagina RFQ**
  - File: `routes_rfq.go`, `thread.go`, `proposte.go`, `codici.go`
  - Schermata: `/thread/{id}` e i gesti B8.5/B8.6
- **Richieste ai fornitori (7B)**
  - File: `richieste.go`
  - Schermata: bozza marcata, risposta del fornitore
- **Pagina Richieste**
  - File: `panoramica.go`
  - Schermata: `/richieste`: una card per RFQ
- **Distinta (quattro passi)**
  - File: `distinta.go`
  - Schermata: `/thread/{id}/distinta?passo=richiesta|distinta|documenti|fattibilita`: la stessa RFQ del Fascicolo
    nell'ordine del lavoro (proposta in `cockpit/_fasi/PROPOSTA_DISTINTA.md`).
    - Legge `caricaThread` e `caricaFascicolo` e non scrive niente; i gesti sono le rotte del Fascicolo, e quando
      `HX-Current-URL` e' la Distinta `threadFrammento` risponde con il suo corpo (`dallaDistinta`, `rispondiDistinta`).
    - La struttura si salva con `POST /fascicolo/bom/applica`, come l'editor; il visore e le note sono quelli della vista
      Documenti (`/allegato/{id}/anteprima`, `/fascicolo/nota`).
- **Fascicolo: schermata e viste**
  - File: `fascicolo_pagina.go`, `fascicolo_rotte.go`, `fascicolo_bom.go`, `fascicolo_dettaglio.go`
  - Schermata: `/thread/{id}/fascicolo`
- **Fascicolo: gesti**
  - File: `fascicolo_rotte.go`, `fascicolo.go`, `fascicolo_conferma.go`
  - Schermata: assegna, codice, struttura, revisioni, conferma
- **Fascicolo: vista Documenti e note (v3)**
  - File: `fascicolo_documenti.go`, `fascicolo_gesti_v3.go`
  - Schermata: pdf.js, filmstrip, note sui disegni
- **Fascicolo: editor della struttura**
  - File: `fascicolo_editor.go`, `fascicolo_gesti_v3.go`
  - Schermata: l'editor a nodi
- **Fascicolo: preparazione e avanzamento**
  - File: `fascicolo_preparazione.go`
  - Schermata: la preparazione (la POST che la pagina manda aprendosi, solo per chi scrive: accoda e basta) e il poll
- **Fascicolo: Importa dal NAS**
  - File: `fascicolo_nas.go`
  - Schermata: il cassetto NAS
- **Allegati, anteprima, caricamento**
  - File: `allegati.go`, `anteprima.go`, `caricamento.go`
  - Schermata: download, conferma, PDF, caricamento interno
- **Distinta nuova del motore A (giro 5, A1d)**
  - Sottopackage: `motore_a/` (modelli, calcolo in sola lettura, indice, etichette, anteprima dello STEP; non importa
    questo package né `app/*`); adattatore: `motore_a.go` (da A1d-P2). Vedi «Modulo Distinta nuova del motore A».
  - A1d-P1: il tipo delle regole caricate all'avvio (`motore_a.Regole`, campo `Server.MotoreA`). A1d-P2: il servizio
    e le due rotte JSON. A1d-P3: il ramo dei gesti di `threadFrammento` (`dalMotoreA`, `rispondiMotoreA`) e il ponte
    statico `web/static/motore_a_gesti.mjs`. La pagina arriva con A1d-P4.
- Il front-end (template, CSS, `fascicolo.mjs`, pdf.js) è descritto in `web/README.md`.

## Modulo Distinta nuova del motore A (giro 5, A1d)

- Adattatore: `motore_a.go` (l'unico file Go di A1d in questo package, direttiva A1d dell'11/10, §1).
- Sottopackage: `motore_a/` (package `motore_a`, importato con l'alias `motorea_web` per non confonderlo con
  `core/inbox/classificazione/motorea`). Derivato dalla PR #18 di Stefano Valo (c543e89, 67dcc02), ristrutturato.

### Chi fa che cosa

| Responsabilità | Dove |
|---|---|
| Rotte, porta del flag, `no-store`, `soloAdmin`, codici HTTP, JSON, HEAD | `motore_a.go` |
| Il ramo dei gesti di `threadFrammento`: `dalMotoreA`, `esitoMotoreA` (= `avvisoNegativo`), `rispondiMotoreA`, `scriviEsitoMotoreA` (A1d-P3) | `motore_a.go`; in `thread.go` le sole righe del ramo |
| Gli aiuti privati del Fascicolo legacy per l'anteprima (`richiestaAutorizzazione`, `spiegaErrore`, `rifiuto`, `almeno`, `utenteDa`) | restano in questo package, usati da `motore_a.go` |
| Le regole caricate all'avvio (`Regole`, `Stato`, `Casi`) | `motore_a/regole.go` (le costruisce `app/runtime`) |
| Il servizio (`Servizio{Regole, Pool, Log}`, immutabile, costruito per richiesta), gli errori distinti, il tetto dei calcoli (`MassimoCalcoli`) e il limite di ripiego (`LimiteDiRipiego`) | `motore_a/servizio.go` |
| Lo snapshot unico REPEATABLE READ READ ONLY (esistenza, fase, fotografia con un savepoint per il caricatore, dichiarazioni STEP) | `motore_a/snapshot.go` |
| Il calcolo: `caricatore.Carica` → `valutazione.Calcola` → passaggio → `confronto.Confronta(…, nil)` | `motore_a/calcolo.go` |
| La pagina e il JSON di `/dati` (contratto §2.2, §2.4) | `motore_a/modelli.go` |
| Le aggiunte di V1 a `/dati` (EM §9.9; PE-01, PE-10, PE-15, PE-16, PE-28): le letture nello stesso snapshot (occorrenze non attive, bozze aperte formato 3 con le colonne del registro, F8, identità certificate; `queries/v1_motore.sql`) e la composizione con `propostabom.Modello` (gruppi con l'impronta_contenuto di adesso e i nodi, fonte, F8, bozza del prodotto con il gruppo corrente) | `motore_a/dati_v1.go`; prove `modelli_test.go` (A-28 sui nomi) e `dati_v1_db_test.go` (L4) |
| La copia di `app/bancoa/passaggio.go` | `motore_a/passaggio.go` |
| L'indice delle ricerche (contratto §2.3) | `motore_a/indice.go` |
| La voce della tabella delle classi del cliente (14/10): sotto il codice della casella «categoria · descrizione · fonte» (`ma_classe_breve`), la riga «Normativa» dell'anteprima, il segno «minuteria ?» per il commerciale proposto con la minuteria proposta, l'etichetta di `proposto_dalla_grammatica` | `web/templates/motore_a.html`, `motore_a/etichette.go`; prova `motore_a_classi_test.go` |
| Il perché dell'ancoraggio detto all'operatore (`Spiega`, `DiagnosticheAncoraggio`, `DiagnosticheAncoraggioDellaRFQ`, `MotiviAncoraggio`, il badge `OrigineStruttura`; WP-U, notte 14/10) | `motore_a/spiegazioni.go` |
| Gli STEP della RFQ di un prodotto senza strutture (`StepDelThread`, `IndirizzoFascicolo`) e gli elementi della mail (`ElementoDellaMail`, `ElementiDellaMail`; WP-U) | `motore_a/step_del_thread.go` |
| La pagina `GET /thread/{id}/motore-a` e il suo frammento (B5.1); il collegamento «Distinta nuova (Motore A)» delle pagine Richieste e RFQ con la guardia della rotta (`distintaNuovaVisibile`, WP-U) | `motore_a_pagina.go`; i campi `DistintaNuova` in `panoramica.go` e `thread.go` |
| Le etichette (`Etichetta`, 67 domini) | `motore_a/etichette.go` |
| L'anteprima dello STEP (`fascicolo.EffettoAutorizzazione`, DTO proprio) | `motore_a/anteprima.go` |

### Le rotte (A1d-P2)

- **`GET /thread/{id}/motore-a/dati`** (`Adm`, dietro il flag)
  - 200: il JSON del contratto §2.4 (`motore_a.Pagina`: `thread_id`, `vista`, `prodotto`, `testa`, `esito`,
    `confronto`, `errore` sempre vuoto, `calcolato`); l'indice non c'è (Q-A1d-11 = A, da confermare). Da V1 (EM
    §9.9) anche `gruppi` (con i nodi), `motivo_senza_gruppi`, `fonte`, `f8_stato` e `bozza_del_prodotto`, tutti
    `omitempty`: la pagina composta senza V1 resta byte per byte quella di prima.
  - 400 id non valido; 404 RFQ inesistente; 503 regole non pronte (testo generico di `Regole.Errore()`, delle regole
    del servizio della richiesta) o troppi calcoli in corso (`ErrOccupato`, testo generico); 504 tempo scaduto
    (`limiti.anteprima.tempo_massimo_ms` dell'indice); 500 ogni altro errore, testo generico e dettaglio nel log, anche
    un valore che non si codifica in JSON. Corpo `{"errore": "…"}`.
  - HEAD: id, regole pronte e `GetThread` in sola lettura, nessun calcolo: 200, 400, 404, 503, 504, come la GET.
- **`GET /thread/{id}/motore-a/anteprima/step-strutturale?componente={cid}&documento=…|nodo=…&sorgente=…`** (`Adm`,
  dietro il flag)
  - 200: `motore_a.AnteprimaSTEP` (`motore`; di che cosa parla: `componente` con `id` e `codice`, `delega`,
    `titolare`, `allegato`, `sha256`, `nome_file`, `documento_id`, in modo delega il file del padre; poi `spento`,
    `gia`, `autorizzabile`, `inviabile`, `frasi`, `sorgente`, `figli`, `sorgenti`, `raggruppamenti`, `deleghe` con
    `spenta` e `revoca`, `guida`, `guida_sotto`, `rimozioni`, `rimozioni_sospese`, `revoche`, `avvisi`, `presa_d_atto`,
    `bottone`, `firma` solo dal ruolo operatore); un rifiuto della richiesta o del Fascicolo è 200 con `errore`, come il
    modulo legacy (lì `componente`, `allegato` e `documento_id` sono `null`). Con `sha256` e la `chiave` di un nodo si
    ricompone il Rif del motore A (`ancoraggio.RifNodo`): la discordanza di Q-A1d-05 si calcola dal JSON.
  - 400 id, componente o form non validi; 404 RFQ inesistente (diversamente dal legacy); 504 tempo scaduto; 500 errore
    del database.
  - HEAD: id e componente validati come la GET (400), poi solo `GetThread`, nessun effetto calcolato (200, 404, 504).
- Ogni GET ha una riga gemella senza metodo sullo stesso indirizzo: a flag spento 404, a flag acceso 405 con
  `Allow: GET, HEAD`.
- La pagina `GET /thread/{id}/motore-a` non è montata: arriva con A1d-P4, insieme a `Init` e alla funzione `etichetta`
  del template.

### La porta

- Flag spento (`[motore_a].anteprima = false`, il valore zero di `Server.MotoreA`): `http.NotFound` a tutti, con ogni
  metodo, PRIMA della sessione (nessun `GetSessione` né `ToccaSessione`) e senza intestazioni in più: la rotta non si
  distingue da un indirizzo che non esiste (R36; Q-A1d-12 = A, da confermare).
- Flag acceso: `Cache-Control: no-store` su ogni risposta (anche il 302/401 di `aLogin`, il 403 di `nega`, il 405),
  poi `soloAdmin`. I 403 e i rimandi al login restano quelli HTML legacy.

### Il client (fetch)

- Le due rotte si chiedono con `fetch`. Il client manda **`HX-Request: true`**: così una sessione scaduta risponde
  401 con `HX-Redirect: /login` invece del 302 verso la pagina di login, che `fetch` seguirebbe dando un 200 HTML.
- Una risposta con un `Content-Type` diverso da `application/json` (il 403 HTML di `nega`, il 404 a flag spento, il
  405 di testo) o con `res.redirected` vero non è un errore dei dati: è accesso negato, sessione scaduta o rotta spenta.
  Solo una risposta JSON ha `{"errore": "…"}`.
- Nessun cambiamento ad `aLogin` né a `nega` (legacy, HTTP-05 della revisione di A1d-P2): è un obbligo del client, che
  arriva con A1d-P4.

### Sola lettura e snapshot

- Nessuna scrittura di dominio, nessun job, nessuna cache, nessuno stato del dominio fra le richieste: lo stato è
  `Server.MotoreA`, preparato una volta all'avvio, più il semaforo dei calcoli (`MassimoCalcoli`, un limite di
  risorse che non tiene dati). La guardia statica `TestLaDistintaNuovaLeggeSoltanto`
  (`core/estrazione/evidenze/dipendenze_test.go`) tiene il sottopackage in sola lettura: di `core/rfq/fascicolo` solo
  le letture riusate, di `*db.Queries` solo `GetThread`, `GetFaseCorrente`, `ListFaseCatalogo`, mai `Exec`, `Commit`,
  `pgx.ReadWrite` né un testo SQL suo.
- `/dati` legge tutto in **una** transazione REPEATABLE READ READ ONLY: `GetThread`, la fase (`GetFaseCorrente`,
  `ListFaseCatalogo`), la fotografia (`caricatore.Carica` con un `Iniziatore` che apre un savepoint della transazione
  esterna: i due `SHOW` del caricatore leggono «on» e «repeatable read»; il suo ROLLBACK diventa ROLLBACK TO SAVEPOINT)
  e le dichiarazioni dello STEP (`fascicolo.LeggiDichiarazioni`). Il caricatore non cambia.
- L'anteprima dello STEP apre una transazione REPEATABLE READ READ ONLY sua: è una seconda lettura, in un istante
  successivo a quello della pagina.
- Il ROLLBACK di ogni transazione usa un contesto staccato dalla richiesta, con un limite di 2 s: un limite scaduto o
  una richiesta chiusa non costano una connessione distrutta.

### I limiti

- **Il tempo**: `/dati` ha il limite dell'indice (`Regole.Limite`); l'anteprima e gli HEAD lo stesso, e con le regole
  non pronte (indice assente o rifiutato: nessun limite letto) `LimiteDiRipiego` (30 s) [T]: nessuna lettura è senza
  scadenza. Al limite la risposta è 504, anche per l'anteprima e per HEAD.
- **I calcoli insieme**: al più `MassimoCalcoli` (2) [T] nel processo, contando i calcoli orfani (oltre il limite ma non
  ancora finiti, perché `valutazione.Calcola` non guarda il contesto). Oltre, 503 subito, senza leggere. Un calcolo
  orfano che finisce va nel log: a Info se riuscito, a Error con la pila se in panic o in errore.
- La composizione della pagina (`ComponiPagina`, `NuovoIndice`) gira dentro il calcolo: un suo panic è un 500 JSON con
  la pila nel log, non una connessione interrotta.

### I gesti: il ponte e il ramo di `threadFrammento` (A1d-P3)

- **Il ponte** è `web/static/motore_a_gesti.mjs` (deriva dalla PR #18, c543e89; vedi `web/README.md`). È l'unico punto
  della pagina nuova che chiama le rotte che scrivono, e solo quelle di una lista chiusa: le 27 POST della mappa dei
  gesti A1d-P3 (§2.1), più «Conferma l'albero» a parte (`POST /thread/{id}/distinta/albero/conferma`, JSON
  `{firma, bozza: {formato: 1, base}}`, esito dallo stato HTTP: 200 ok, 422 no, altro errore) e le quattro GET JSON
  (`/motore-a/dati`, `/motore-a/anteprima/step-strutturale`, `/distinta/tipo`, `/distinta/albero/riepilogo`). Fuori:
  «Accetta la relazione» (Q-A1d-06 = A [U]) e «Scarta la relazione» (Q-P3-01, base di lavoro); fuori anche
  `bom/applica`, `file/{aid}/accetta`, `proposta/{pid}/decidi`, `/proposta/{id}/conferma`, `riattiva`, le note,
  `congela`, `revisione/*`, `deroga-struttura`, `annulla-sostituzione`.
- Ogni richiesta del ponte porta `HX-Request: true` e `HX-Current-URL` = `/thread/{id}/motore-a`; i gesti sono
  `application/x-www-form-urlencoded`, campi ripetuti senza `[]`; i valori dei campi solo stringhe e numeri finiti
  (`null`, `undefined`, oggetti: il gesto non parte). Le guardie di invio sui nomi dei campi (Q-P3-02, base di lavoro
  A); un campo che il gestore legge con `r.FormValue` (il primo valore) si controlla com'è letto: una volta sola, e non
  vuoto dove vuoto non è ammesso (un campo ripetuto non parte mai):
  - niente `tipo=commerciale` in «Accetta il nodo»;
  - niente `tipo`, e sempre `rev` e `descrizione`, una volta sola, in «Rev e descrizione»;
  - `qta` scritta, una volta sola, in «Collega»;
  - in «Sposta» `da` e `a` sempre, una volta sola, anche vuoti come scelta esplicita (`da` vuoto = era una radice:
    aggiunge un padre; `a` vuoto = diventa una radice); `qta` scritta solo con `a` non vuoto: con `a` vuoto il gestore
    non la usa e il ponte non la chiede (allineata al legacy, [T], da ratificare dentro Q-P3-02);
  - una sola `voce` e nessuno di `struttura`, `strutturale`, `firma`, `conferma`, `selezione` in «Conferma il
    documento»; in «Assegna» niente `nuovo_riferimento*`, e `correggi_codice` solo insieme a `correzione_confermata`,
    tutti e due una volta sola e non vuoti (li manda soltanto «Usa come STEP del prodotto…», dopo la sua finestra: WP-U,
    notte 14/10, decisione autonoma dell'orchestratore su Q-P3-03 [T, da ratificare]; il server ignora
    `correzione_confermata`, e con la correzione la finestra manda anche `codice_atteso` e `componente_atteso`, che il
    server controlla contro la proposta bloccata: revisione della notte 14/10); niente `nuovo_riferimento*` in
    «Sostituisci»; niente `rumore` nello scarto del file;
    niente `auto` in «Prepara i file».
- **Gli esiti nel ponte**: si legge l'esito solo da un 200 `text/html` che sia un solo `<div data-ma-esito="ok|no">`;
  ogni altro 2xx, o un corpo d'altro tipo, è «errore»; 4xx e 5xx «errore» con il testo; la sessione scaduta si dice in
  `#ma-avviso` e si va al login. `#ma-avviso` è una regione viva (`role="status"`, `aria-live="polite"` se il template
  non dice altro; se manca, il ponte la crea vuota in fondo al `body`).
- **La rilettura** (dopo un «ok»): il corpo della risposta di un gesto non entra mai nella pagina, ma la rilettura sì:
  è uno swap `innerHTML` di htmx in `#ma` con la GET della pagina stessa. È un'eccezione ammessa solo a queste
  condizioni, che il ponte controlla prima dello swap (`htmx:beforeSwap`) e che sono il contratto del template di P4:
  - la GET HX di `/thread/{id}/motore-a` risponde con il solo frammento del corpo di `#ma`, dallo stesso percorso
    (nessun rimando seguito altrove), entro il limite di tempo del ponte (60 s, `htmx:configRequest`);
  - il frammento ha un solo elemento con `data-ma-pagina="{id}"` (l'id della RFQ);
  - dentro `#ma` nessun `hx-get`/`hx-post`/`hx-put`/`hx-patch`/`hx-delete`, nessun `hx-swap-oob`, nessun `hx-on*` e
    nessuno `<script>`; `#ma` e `#ma-avviso` non sono nel frammento, e `#ma-avviso` sta fuori da `#ma` (lo swap
    cancellerebbe l'esito appena scritto).
  Altrimenti niente swap e l'avviso «la pagina non si è riletta». Durante la rilettura `#ma` ha `aria-busy` e la
  pagina resta occupata: un altro gesto non parte finché non finisce.
- **Il ramo**: in `threadFrammento` (thread.go) il primo controllo è `s.MotoreA.Accesa && dalMotoreA(r, id)`:
  `HX-Current-URL` esattamente `/thread/{id}/motore-a`, come lo manda il ponte: niente barra finale, query, frammento
  o `/` codificata; un indirizzo assoluto solo con lo stesso host della richiesta e lo schema http/https; l'id nella
  forma canonica di 36 caratteri, uguale al thread senza badare alle maiuscole. Allora `rispondiMotoreA`: toglie le
  intestazioni `HX-*` già messe (oggi l'`HX-Trigger: inbox-aggiorna` di `pannelloConAvviso` su `/proposta/{u}/scarta`,
  che resta nel gestore legacy), legge solo se la RFQ esiste (`GetThread`, la prima lettura di `caricaThread`: se non
  c'è, 404 «thread non trovato» come il legacy, mai «ok») e risponde 200, `text/html`, `Cache-Control: no-store`, un
  solo `<div data-ma-esito="ok|no">testo</div>` con l'escape di `html/template`, nessun `HX-*`. A flag spento, da
  un'altra pagina, da un'altra RFQ o con un indirizzo non esatto il ramo non scatta: risponde la vecchia interfaccia,
  che il ponte legge «errore» anche dopo una scrittura riuscita (TEC-01). Il ramo guarda `Accesa`, non `Pronta`: con
  le regole in errore scatta lo stesso.
- **Vincolo per P4**: htmx manda da solo `HX-Current-URL` con l'indirizzo della pagina. Dalla pagina motore-a le rotte
  che passano da `threadFrammento` si chiamano solo tramite il ponte: un `hx-post` del template verso una di queste
  riceverebbe il div compatto al posto della vecchia interfaccia. Il template non ha `hx-post`/`hx-put`/`hx-delete`
  verso `/thread/{id}/…` o `/proposta/…` (prova statica di Q4).
- **L'esito** è `avvisoNegativo` (distinta.go) così com'è (Q-A1d-09 = A): «no» se la frase comincia con «Niente è
  cambiato», «Assegnazione non riuscita», «Conferma non riuscita», «Correzione non riuscita», «Preparazione dei file
  non riuscita», «La proposta era già stata decisa», o se contiene «nessun cambiamento» (minuscolo). Il
  `rifiutiMotoreA` della PR non c'è. Il problema degli esiti HTTP NON è risolto: un contratto strutturato di risposta
  ai gesti va proposto a parte.
- **Le frasi ambigue note** (caratterizzate in `motore_a_gesti_test.go`, mappa §3; nessuna corretta):
  - rifiuti con 200 letti «ok»: «Nessun file scelto: niente da assegnare.», «Copie NON rimesse in coda: …»,
    «Le proposte di una RFQ chiusa / unita a un'altra RFQ non si aggiornano.» (TEC-04);
  - errori tecnici letti «no», cioè come un rifiuto: ogni errore dentro `s.gesto` / `gestoPoi` (anche un 40P01 o un
    Commit fallito) diventa «Niente è cambiato: …»; così «Correzione non riuscita…», «Assegnazione non riuscita…», gli
    errori di aggiorna e prepara, la conferma del documento con HX (TEC-02); «Nessuna copia nuova (… non accodat…)»
    è invece un errore dentro un «ok»;
  - successi con testo negativo letti «no»: una deroga con «nessun cambiamento» nel motivo, un nome grezzo che lo
    contiene (TEC-06): il gesto è scritto e la pagina non si rilegge;
  - idempotenti: «Nessun cambiamento: …» (maiuscolo) e «… è già …» sono «ok»; «La proposta era già stata decisa:
    nessun cambiamento.» è «no» (TEC-07);
  - successi parziali letti «ok» senza distinzione: «l'autorizzazione non vale», «rimozioni non calcolate», le copie
    in parte non accodate, la preparazione con file rimandati (TEC-05);
  - il rismistamento gira dopo ogni risposta < 400, anche dopo un rifiuto con 200 (TEC-03; la L4 lo misura: dopo il
    rifiuto di «Rianalizza» la destinazione di un disegno è scritta).
- **La mappa in breve**: 30 gesti nella mappa (G-01…G-30, con i nove campi e i disallineamenti), 28 nel ponte (27 POST
  più «Conferma l'albero»). Fuori dal ponte i due della relazione: «Accetta la relazione» (Q-A1d-06 = A [U]: la BOM si
  conferma dal riepilogo con «Conferma l'albero», e il gesto applicherebbe E37 senza anteprima né firma) e «Scarta la
  relazione» (Q-P3-01: sospesa finché non è decisa; lo scarto è definitivo). I 28 del ponte: nodi (accetta, scarta,
  riapri, codice del nodo), codici e dati (codice del componente, rev e descrizione, tipo con la firma di
  `GET /distinta/tipo`), struttura (collega, sposta, scollega, conferma l'albero), STEP e rimozioni (autorizza con la
  firma della GET JSON di P2, revoca, accetta e scarta la rimozione), componenti (archivia, rimuovi, ripristina,
  deroga e revoca), documenti (conferma una voce, assegna, sostituisci, scarta il file), operazioni (riprova copie,
  prepara, rianalizza, aggiorna smistamento). Nessuna scrittura nuova: sono i gestori legacy, invariati.
- **Disallineamenti B7 che il ponte non corregge** (registrati nella mappa §4; le L4 li fissano dove indicato):
  - R-B7-04: la relazione accettata da sola resta senza il segno di «Conferma l'albero» e la BOM non risulta verificata
    (CR-01…05d); dopo «Conferma l'albero» lo scollega (anche dell'arco d'autorità) lascia la BOM verificata e la riga
    orfana col segno, il cambio di quantità la toglie solo per il conflitto con lo STEP (DOM-10);
  - E37: «Accetta la relazione» con un padre sciolto lo rende sottoassieme; la rotta è fuori dal ponte, ma l'avviso
    dell'anteprima dello STEP lo promette ancora (nessuna prova: la scena con un padre sciolto non c'è);
  - D1 (DOM-03): lo scollega dell'ultimo figlio lascia il sottoassieme senza figli, con «ok» e nessun avviso (L4);
  - DB7-3 (DOM-07): «Correggi il codice» rinomina anche il prodotto della RFQ e stacca il legame per codice: la sola
    protezione è il template di P4 (L4).

### La pagina: il perché, gli STEP della RFQ, gli elementi della mail (WP-U, notte 14/10)

Diagnosi UI del 14/10 (G1, G2, G4). Sola presentazione e gesti esistenti: nessuna rotta nuova, nessuno schema nuovo.

- **U1, il collegamento.** «Distinta nuova (Motore A)» nella card della pagina Richieste (`richieste.html`,
  `richiestaCard.DistintaNuova`) e nella pagina della RFQ (`thread.html`, testata e riepilogo del Fascicolo,
  `threadDati.DistintaNuova`), solo con la guardia della rotta: `distintaNuovaVisibile` = flag `[motore_a].anteprima`
  acceso e ruolo admin (`almeno`, come `soloAdmin`). Il campo della card non entra nella firma del poll.
- **U2, il perché.** Le diagnostiche dell'ancoraggio (`esito.ancoraggi.diagnostiche`) del prodotto (il suo Rif fra i
  Rif della diagnostica) in «Analisi del prodotto» (`#ma-perche-prodotto`), quelle di nessun prodotto in un
  `<details>` (`#ma-perche-rfq`); i motivi dell'ancoraggio di un file (`esito.ancoraggi.file[].motivi`) sulla sua scheda
  di «Da smistare» e sotto gli STEP della RFQ. Titolo e frase in italiano piano da `motore_a/spiegazioni.go`, il codice
  grezzo nel `title`, il messaggio del server così com'è. Un codice sconosciuto non va mai fra quadre: titolo
  dall'etichetta della diagnostica o dal codice. La tabella porta già i codici di WP-A (`radice_dal_nome_file`,
  `namespace_non_confrontabile`). Il badge dell'origine di una struttura candidata («radice dal nome del file» per
  `nome_file_step`, «radice dallo STEP associato» per `associazione_step`) legge il campo `Origine` per nome (reflect),
  dalla struttura di ancoraggio o dalla candidata: compila e si rende prima e dopo l'unione con WP-A.
- **U3, il prodotto senza strutture candidate.** Con struttura «nessuna», sotto l'analisi del prodotto, «STEP della
  RFQ» (`#ma-step-rfq`): ogni STEP con entità `nodo_step` nel documento delle evidenze, con il testo delle radici
  (contesto `radice_step`, campo id, poi nome, poi la chiave) e il gesto che `StepDelThread` gli dà:
  - il file è già documento corrente del componente del prodotto: «Autorizza come STEP di …», il dialogo esistente
    dell'anteprima (`data-ma-azione="step"` con `data-ma-documento`, senza sorgente: la radice si sceglie
    nell'anteprima del server, con presa d'atto e firma);
  - il file ha una proposta aperta di 3D e il componente del prodotto non ha un 3D corrente: «Usa come STEP di …»
    (`data-ma-azione="usa-step"`): la finestra di
    `motore_a.mjs` (`apriUsaStep`) elenca che cosa si scrive, chiede la presa d'atto e manda nell'ordine due gesti
    esistenti del ponte, ognuno con il suo esito: «assegna» al componente del prodotto (con `correggi_codice` e
    `correzione_confermata` solo se il codice della proposta è scritto e diverso da quello del componente) e «Conferma
    il documento» della stessa proposta; se il primo non riesce il secondo non parte. Dopo la rilettura il file è un
    documento del prodotto e il gesto diventa «Autorizza come STEP di …»: il terzo passo resta di chi guarda.
    Revisione della notte 14/10: con la correzione «assegna» riceve anche `codice_atteso` e `componente_atteso`
    (`data-ma-codice-file`, `data-ma-componente-attuale`: lo stato che la finestra ha mostrato) e rifiuta «la proposta
    è cambiata nel frattempo: rileggi la pagina» se un collega l'ha cambiata; mentre la catena è in corso la finestra
    non si chiude (annulla e ✕ spenti, Esc fermato), e se si è chiusa lo stesso la conferma non parte e l'avviso dice
    che il file è associato ma non confermato;
  - altrimenti il motivo (prodotto senza componente, documento di un altro componente, nessuna proposta aperta di 3D,
    il componente con un 3D corrente: la conferma del file singolo chiederebbe se si aggiunge o si sostituisce, e la
    catena si fermerebbe a metà; revisione della notte 14/10), e sempre «Apri nel Fascicolo» sul componente del
    prodotto (`?nodo=`).
  «Autorizza come STEP di …» c'è anche su un documento del prodotto fra gli «Altri documenti candidati alla fonte»
  (origine `documento_del_prodotto`, con il documento). Le verifiche restano quelle delle rotte (FK, codice del
  componente, scelta della revisione, anteprima e firma dell'autorizzazione).
- **U4, gli elementi della mail.** Gli allegati con `motivo = natura_non_file` (inline, elementi di Outlook,
  collegamenti) portano `data-ma-elemento-mail` in «Da smistare» e in «Fuori perimetro»; il CSS li nasconde finché
  `#ma-mostra-mail` («mostra elementi della mail (N)», nella testata di «Da smistare») non è spuntato (`:has()`, anche
  senza JavaScript); `motore_a.mjs` ricorda la scelta per la RFQ (sessionStorage) e la rimette dopo la rilettura.

### Differenze dichiarate

- **Il banco**: la pagina non passa gli ingressi del file dei casi (Q-A1d-10 = B, da confermare). Per un thread senza
  un caso di scenario l'`EsitoThread` canonico e `confronto.Esito.Impronta` sono quelli del banco; con un caso di
  scenario possono differire, per scelta. `Esito.Impronta` ed `Esito.ImprontaFotografia` sono sempre diverse: la
  fotografia della pagina ha il solo thread, nessun messaggio fuori RFQ e il solo cliente del thread.
- **Il Fascicolo legacy**: l'anteprima dello STEP calcola gli effetti con `fascicolo.EffettoAutorizzazione`, che
  classifica i nodi con il motore legacy (`MotoreDellaRfq`, `cliente.regole`); la pagina e `/dati` usano il motore A
  (`valutazione-5` dal 14/10, le grammatiche dell'indice). Codici e tipi possono differire: il campo `motore` dell'anteprima vale
  `fascicolo_legacy` (Q-A1d-05, da confermare); la resa della discordanza è di A1d-P4.
- **Il tempo**: `valutazione.Calcola` non riceve un contesto; oltre il limite la risposta è 504 e il calcolo orfano
  finisce in memoria, senza scritture (con il tetto dei calcoli insieme: «I limiti»).
- **Le copie di regole del core** (nessuna modifica a `core/` in A1d):
  - `motore_a/passaggio.go`, copia di `app/bancoa/passaggio.go` e degli aiuti del banco che chiama: la confronta
    `TestMotoreAPassaggioUgualeAlBanco` (i corpi stampati; gli aiuti si trovano dalle chiamate, non da un elenco);
  - `Indice.componenteDelNodo` (`motore_a/indice.go`), copia di `componenteDelNodo` di `core/valutazione/nodi.go`
    (T-B4-38: la radice della BOM di lavoro prende il componente del target, quella di una candidata no): la confronta
    con l'esito vero la L4 `TestL4MotoreACalcoloVero` (`motore_a/servizio_db_test.go`); chi cambia la regola nel core
    la trova cadere;
  - `estensioneDi` (`motore_a/indice.go`), la lettura dell'estensione di un allegato come la fa `valutazione` per la
    fonte. Il `Disponibile` dell'anteprima di un allegato che non è un 2D dell'esito è sempre falso: la regola di
    `valutazione` (`anteprimaDisponibile`) non si ricopia.
- **Le quantità confermate e i padri confermati, per gruppo**: `QuantitaConfermata(prodotto, gruppo, padre, figlio)`
  legge solo la struttura del gruppo (il contratto §2.3 non aveva il gruppo: da ratificare con la v1.1), e in una
  candidata il padre confermato che il gruppo non rappresenta è `componente:<uuid>`, mai un Rif della BOM di lavoro.

### Dipendenze

- Il sottopackage `motore_a/` importa `core/inbox/classificazione/motorea`, `core/valutazione`, `core/confronto`,
  `core/fotorfq`, `core/fotorfq/caricatore`, `core/ancoraggio`, `core/rfq/fascicolo` (solo le letture: vedi «Sola
  lettura e snapshot»), `platform/db`, `uuid`, `pgx` e `pgconn`; mai `transport/web` né `app/*` (la lista chiusa di
  `pacchettiMotoreA` in `core/estrazione/evidenze/dipendenze_test.go`).
- Lo importano `app/runtime` (le regole all'avvio) e questo package (`motore_a.go`); nelle sole prove anche
  `app/bancoa` (la L4 di equivalenza, `motore_a_equivalenza_db_test.go`), freccia dichiarata in `internal/README.md`.
- Le L4 di questo package, del sottopackage e di `app/bancoa` leggono le regole ACME da `app/runtime/testdata/motore_a`
  (le testdata della PR #18): una dipendenza dal file system, dichiarata in `internal/README.md`.

### Test

- **L1 del sottopackage** (`motore_a/`)
  - `indice_test.go`, `indice_a1d_test.go`: le 29 ricerche (più le 8 di WP-U) sulla scena ACME, il tipo per
    casella (D-CHIUS-4), `ComponenteDelNodo`, `Figlio.Origine`, `NelFile`, `CandidatiFuori`, la candidata con gli stessi Rif della BOM
    (quantità confermata e padri confermati per gruppo), `Marcatura`, `StepProdotto`, `Autorizzazioni`.
  - `etichette_test.go`: ogni costante di ogni dominio ha il suo testo (letta con go/parser), anche i codici delle
    diagnostiche del confronto e delle regole; il tipo solo nello `switch`.
  - `passaggio_test.go`: la copia di passaggio uguale a quella del banco (i byte JSON e i corpi delle funzioni).
  - `calcolo_test.go`, `limiti_test.go`: regole non pronte prima di leggere, nessun ingresso, gli errori distinti, il
    tempo, la versione, l'esito consegnato a limite scaduto, il tetto dei calcoli, il log dopo il limite, il ROLLBACK
    staccato, il limite di ripiego, la composizione protetta.
  - `snapshot_test.go`, `anteprima_test.go`, `modelli_test.go`: la transazione e il savepoint, l'anteprima (le frasi,
    di che cosa parla, la forma del JSON), i modelli JSON di `/dati`.
  - `spiegazioni_test.go`, `step_del_thread_test.go` (WP-U): le spiegazioni (note, sconosciute, mai fra quadre; ogni
    motivo e codice delle strutture di `core/ancoraggio` ne ha una, letti con go/parser), le diagnostiche per prodotto
    e della RFQ, i motivi di un file, il badge dell'origine letto per nome, gli STEP della RFQ con i gesti e le radici
    («Usa come STEP» solo senza un 3D corrente del componente: revisione della notte 14/10), la determinazione, gli elementi della mail, l'indirizzo del Fascicolo, il vocabolario ripetuto come testo.
- **L1 di questo package** (`motore_a_test.go`, httptest): la porta, i ruoli, le rotte montate, i codici (anche 504
  dell'anteprima e di HEAD, 400 di HEAD, 503 del tetto, il JSON che non si codifica), la deriva dell'anteprima contro
  il modulo legacy.
  - `motore_a_gesti_test.go` (A1d-Q3): `dalMotoreA` (solo l'indirizzo esatto), `scriviEsitoMotoreA` (un solo div,
    escape, no-store, intestazioni `HX-*` tolte), il ramo di `threadFrammento` (a flag spento, da un'altra pagina, da
    un'altra RFQ o con un indirizzo non esatto non scatta; a flag acceso dalla pagina toglie `HX-Trigger` e legge
    l'esistenza della RFQ prima della vecchia interfaccia; la risposta con il database è nelle L4), la
    caratterizzazione di `avvisoNegativo` sulle frasi dei gesti (mappa §3, CQ-01) e la guardia dei testi, che li cerca
    nei sorgenti di questo package e di `core/rfq/fascicolo`.
  - `motore_a_ponte_test.go` (A1d-Q3): prove statiche sul sorgente del ponte: la lista chiusa (27 POST, mondo chiuso
    rispetto alle POST che il server registra, le due della relazione registrate ed escluse), le rotte nel mux, niente
    HTML inserito salvo la rilettura htmx di `#ma` alle sue condizioni (mai la risposta di un gesto), nessun altro
    canale d'invio, nessuna rotta aggiunta fuori dal blocco, niente resti dimostrativi né import, intestazioni, le righe
    che decidono per testo esatto (esiti, altra RFQ, guardie, rilettura, stato occupato), nessuna quantità implicita;
    `statici_test.go`: tipo, cache e impronta di `motore_a_gesti.mjs`. Sono prove sul testo: il comportamento nel DOM lo
    proverà il kit jsdom (Node, che qui non c'è) con A1d-P4.
- **L1 della pagina, WP-U** (`motore_a_wpu_test.go`): il collegamento con la guardia (`distintaNuovaVisibile`, i due
  template), il perché del prodotto, della RFQ e del file, gli STEP della RFQ con i due gesti (spenti in sola lettura),
  «Autorizza» su un documento del prodotto fra i candidati, gli elementi della mail e l'interruttore, la regola del CSS
  e l'ordine dei due gesti in `apriUsaStep`; sui casi ACME cambiati in memoria, con i controlli generici della resa.
  Dalla revisione della notte 14/10: «Usa come STEP» solo senza il 3D corrente del prodotto
  (`TestMotoreAWPUUsaSenza3DCorrente`, con `data-ma-componente-attuale`), la finestra che non si chiude a catena in
  corso e lo stato atteso mandato con la correzione (`TestMotoreAWPUStatici`), il confronto di `codice_atteso` e
  `componente_atteso` con la proposta bloccata, senza database (`TestAssegnaConLaPropostaAttesa`).
- **L1 altrove**: `TestLaDistintaNuovaLeggeSoltanto` (`core/estrazione/evidenze`), la guardia di deriva della prova di
  equivalenza `TestEquivalenzaMotoreAIlPercorsoDelBancoNonDeriva` (`app/bancoa/motore_a_equivalenza_test.go`).
- **L4** (tag `integrazione`, `cockpit_test`)
  - `motore_a/servizio_db_test.go`: il calcolo vero con l'esito pieno e la prova gemella di T-B4-38, lo snapshot dal
    registro SQL, la sonda 25006, la RFQ inesistente, l'anteprima come il legacy, il tempo scaduto all'apertura e
    durante la lettura.
  - `motore_a_db_test.go`: le rotte con le sessioni vere (flag spento, solo l'admin, RFQ inesistente, stessa firma del
    legacy); le due righe di `TestNessunaGetScrive` (`nessunaget_db_test.go`).
  - `motore_a_gesti_db_test.go` (A1d-Q3): i gesti come li manda il ponte (un rifiuto noto → «no» e nessuna tabella
    cambiata, un successo → «ok» e l'effetto riletto dal database, ogni risposta senza `HX-*`). Coperte: P3-L4-00…04,
    -08…13, -17, -19…25, -27, -28, CQ-03, CR-01…05d (meno CR-05e), la RFQ inesistente (404), DB7-3 e D1. Non coperte
    (NON ESEGUITE negli esiti di Q3): P3-L4-05, -06, -07, -14, -15, -16, -18, -26, e il controllo «GET /motore-a/dati
    mostra l'effetto» della mappa §5.1, fatto solo per la BOM.
  - `app/bancoa/motore_a_equivalenza_db_test.go`: l'equivalenza col banco (`-thread`, `-tutti`, con un caso di
    scenario).

### Non appartiene qui

- Regole di classificazione, di autorizzazione o di BOM: le dicono `core/valutazione` e `core/rfq/fascicolo`.
- Il template, il CSS, `motore_a.mjs`, il rendering, la registrazione della pagina e `Init` (A1d-P4).
- Le correzioni degli esiti legacy (`avvisoNegativo`, i gestori): sono decisioni separate (Q-A1d-09).

## Modulo Server e ruoli

- `server.go`, `ruoli.go`, `navigazione.go`
- L'infrastruttura comune del package: il tipo `Server`, i template, il montaggio delle rotte, la sessione, i ruoli e la
  rail.

### Scopo

- Caricare i template una volta (`Init`) e rendere una vista come pagina intera o come frammento (`rendi`, `frammento`,
  `frammentoRichiesto`).
- Riconoscere l'utente dal cookie (`autenticato`), riabbinare la sessione alla postazione e negare ciò che il ruolo non
  consente (`soloRuolo`, `soloAdmin`, `nega`).
- Servire gli statici con l'impronta del contenuto (`statico`, `statici`; solo file, mai l'elenco di una cartella), la
  salute del server, il login e il logout.

### Non appartiene qui

- Le rotte delle aree: ognuna ha il suo `registra*` nel file dei suoi gestori.
- Qui restano solo `/static/`, `/healthz`, `/login`, `/logout` e la radice.

### File

- **`server.go`**
  - `Server` (configurazione iniettata da `app/runtime`)
  - `funzioni` (FuncMap dei template, con `oggettoVisibile`)
  - `Init`, `ProtezioneCSRF`, `Registra`, `statici`/`statico`
  - `vista` (con `Serve`, il ruolo che il divieto dice che basterebbe)
  - `rendi`, `frammento`, `healthz`, `autenticato`, `riabbinaPostazione`, `login`, `logout`, `avviso`, `avvisoErrore`
- **`ruoli.go`**
  - `rango` (consultazione < operatore < tecnico < admin; un ruolo sconosciuto vale 0)
  - `almeno`, `soloRuolo`, `soloAdmin`, `metodoCheScrive`
  - `nega` (il 403 in due forme; dice il ruolo che servirebbe, o nessuno quando il divieto non dipende dal ruolo)
- **`navigazione.go`**
  - `navPer`: la rail per utente (Inbox, Richieste; per l'admin la sezione Admin), `vista.Nav`
- **`web/templates/layout.html`, `login.html`, `vietato.html`**
  - guscio con testata e rail (più la guardia `htmx:beforeSwap` e la pulizia delle bozze dell'editor all'uscita)
  - form di accesso
  - pagina e frammento del divieto

### Entry point

- `Server.Init` e `Server.Registra`: li chiama `app/runtime/ascolto.go:Ascolta`.
- `ProtezioneCSRF`: sempre in `Ascolta`, che avvolge tutto il mux, rotte dei worker comprese (i worker non mandano
  `Sec-Fetch-Site` e passano).
- `Server.URLServer`: la pagina Postazioni e il log di avvio.
- **`GET /static/`**
  - Ruolo: nessuno
  - Gestore: `statici`: solo file (una cartella risponde 404, niente elenco); `Cache-Control: immutable` per `pdfjs-*` e
    per `?v=`; tipi MIME decisi dal server
- **`GET /healthz`**
  - Ruolo: nessuno
  - Gestore: `healthz` (JSON `worker.Salute`: DB, versione dello schema, NAS)
- **`GET /login`, `POST /login`**
  - Ruolo: nessuno
  - Gestore: `loginForm`, `login`
- **`POST /logout`**
  - Ruolo: nessuno
  - Gestore: `logout`
- **`GET /`**
  - Ruolo: autenticato
  - Gestore: reindirizza a `/inbox`

### Dati

- `utente`: `GetUtentePerSigla`, solo utenti attivi; la password si confronta con bcrypt.
- `sessione`:
  - `CreaSessioneConPostazione` con un token casuale di 32 byte e scadenza 12 h;
  - `GetSessione` scarta le sessioni scadute e gli utenti disattivati;
  - `ToccaSessione` a ogni richiesta autenticata;
  - `SetSessionePostazione` nel riabbinamento;
  - `EliminaSessione` al logout.
- Cookie `cockpit_sess`: `HttpOnly`, `SameSite=Lax`, `Secure` solo se il server parla TLS, durata 12 h.
- `schema_versione`: letta da `healthz`.

### Flussi principali

1. **Login.** Sigla in maiuscolo → bcrypt → abbinamento per IP (`abbinaPostazione`) → la sessione nasce con origine `ip`
   o senza postazione → cookie → `/inbox`.
2. **Ogni richiesta autenticata.**
   - Cookie → `GetSessione`. Se manca o è scaduta: redirect a `/login`, oppure `HX-Redirect` con 401 se la richiesta
     viene da HTMX.
   - Poi `ToccaSessione` e `riabbinaPostazione`, solo per le sessioni senza origine.
   - Utente e sessione vanno nel contesto.
   - Un metodo che scrive con ruolo `consultazione` → `nega`.
3. **Rendering.** `rendi` sceglie `layout` (pagina intera, con testata `stato` e rail) oppure il frammento indicato,
   quando la richiesta è HTMX e non un ritorno dalla cronologia (`HX-History-Restore-Request`).
4. **Divieto.** `nega` scrive il 403 come pagina o come frammento `vietato`, e lo registra nel log.
   - Il ruolo che la pagina dice che servirebbe arriva da chi nega:
     - admin da `soloRuolo`/`soloAdmin`,
     - operatore da `autenticato` (un metodo che scrive) e da `fascicolo_nas.go:fascicoloNas`,
     - nessuno da `postazione.go:scegliPostazione` (il motivo è la postazione, non il ruolo).

### Invarianti

- Ogni rotta `/admin/*` è montata con `soloAdmin` (autenticazione, poi ruolo). La rail chiama lo stesso `almeno` del
  wrapper.
- `consultazione` non scrive: la regola è per metodo, in `autenticato`, per tutte le rotte autenticate.
- La protezione cross-site vale per tutto ciò che il listener serve.
- `Secure` sul cookie solo con TLS; `SameSite=Lax` sempre.
- Ogni pagina carica `style.css`, `htmx.min.js` (e il Fascicolo `fascicolo.mjs`) con l'impronta del contenuto
  (`{{statico}}`):
  - un binario nuovo porta file nuovi anche con la cache `immutable`.
- `/static/` serve file, non cartelle: la radice e le cartelle (anche `pdfjs-*/`) rispondono 404.

### Dipendenze

- Importa `platform/db`, `platform/rete`, `platform/storage/nas`, `platform/coda`, `platform/contratti/worker`,
  `core/inbox/ingest`, `core/inbox/lettura` (`oggettoVisibile`), `core/rfq/{documenti,fascicolo}`, `ai/agente` (tipi dei
  campi di `Server`).
- Lo importa solo `app/runtime`.

### Test

- **L1**
  - `ruoli_test.go`: ordine dei ruoli, metodi che scrivono, rail per ruolo, pagina del divieto
    (`TestIlDivietoDiceIlRuoloCheServeDavvero`: operatore per chi consulta, nessun ruolo per una postazione).
  - `web_test.go`, `templates_test.go`: i template si caricano e i frammenti si eseguono.
  - `statici_test.go`:
    - tipi MIME e cache, copia di pdf.js;
    - `TestLeCartelleStaticheNonSiElencano`;
    - `TestIlLayoutCaricaGliStaticiConLImpronta` (anche la guardia `htmx:beforeSwap`);
    - `TestLaBozzaDellEditorEDiChiLaScrive`.
  - `stile_test.go`: `[hidden]` nasconde davvero.
- **L4**
  - `rbac_db_test.go`: operatore fuori dall'Admin, admin ovunque, testata dal vivo, consultazione in sola lettura,
    password dopo il riavvio, seme con ruolo sconosciuto.
  - `rete_db_test.go`: POST da un altro sito respinto, worker ammesso.

### Stato dell'implementazione

- Completo.
- Il ruolo `tecnico` oggi può esattamente ciò che può `operatore`: nessuna rotta usa `soloRuolo` con un minimo diverso da
  admin.
  - La matrice ruolo × azione è rimandata di proposito (commento in `ruoli.go`).
- Il frammento del divieto chiesto da htmx non si vede:
  - `nega` risponde 403, e htmx 2.0.4 con la configurazione predefinita (`responseHandling`) non innesta le risposte 4xx;
    nessun template la cambia.
  - Solo i gesti mandati da `fascicolo.mjs` (note, conferma dell'editor: la funzione `gesto()`) leggono il 403 e lo dicono
    con una frase loro.

### Dove intervenire

- **aggiungere una pagina al package**
  - Apro: `Init` (elenco delle pagine), il `registra*` dell'area, il template
- **una voce nuova nella rail**
  - Apro: `navigazione.go:navPer`
- **cambiare chi può fare che cosa**
  - Apro: `ruoli.go` (`rango`, `soloRuolo`)
- **una funzione nuova nei template**
  - Apro: `server.go:funzioni`
- **cambiare cookie o durata della sessione**
  - Apro: `server.go:login`
- **cambiare che cosa può entrare fra i pezzi della pagina**
  - Apro: `layout.html` (l'ascoltatore `htmx:beforeSwap`)
- **cambiare la pagina del divieto**
  - Apro: `ruoli.go:nega`, `vietato.html`

### Leggi anche

- `transport/README.md`
- `internal/README.md` (livelli di test)
- `app/README.md` (`Ascolta`)

## Modulo Inbox

- `routes_inbox.go`, `inbox_viva.go`, `agente.go`
- La schermata A: quadranti, sync ordinario e storico, pannello del messaggio, azioni su Outlook e testata viva.

### Scopo

- Mostrare la posta per quadrante (dalla colonna `quadrante` di `v_inbox`), direzione, filtro e casella.
- Mostrare il messaggio scelto da leggere:
  - l'oggetto senza le etichette di posta esterna,
  - il corpo come testo con le tabelle estratte,
  - il testo automatico di Outlook chiuso e la storia citata a parte (`core/inbox/lettura`).
- Tenere la testata aggiornata: postazione, stato delle caselle, novità.
- Accodare i job interattivi sulla copia servita dalla postazione della sessione.
- Accodare il sync: «Aggiorna ora», prima apertura della sessione, «Carica precedenti».

### Non appartiene qui

- Le decisioni su che cosa è un messaggio: `triage.go`, `censisci.go`, `richieste.go`.
- Allegati, download e anteprima: `allegati.go`, `anteprima.go`.
- Il calcolo del quadrante, che vive in `v_inbox`.
- La lettura del corpo (rumore, tabelle, storia citata, Safe Links): `core/inbox/lettura`, un package puro.
  - Qui si chiama `lettura.Presenta` e basta.

### File

- **`routes_inbox.go`**
  - `registraInbox`
  - `inbox` (frammento `inbox_stato`: comandi e lista insieme; `sel` solo se UUID)
  - `quadranteValido`, `direzioneValida`
  - `syncStorico`, `finestraStorico`, `nomiCartellaPredefinita`/`nomiDellaCartella`, `partenzaDellaCartella`,
    `syncStoricoStato`, `badgeStorico`
  - `messaggio`, `caricaMessaggio` (`messaggioDati.Corpo` = `lettura.Presenta(corpo_testo, corpo_html)`)
  - `copiaInterattiva`, `accodaInterattivo`, `motivoCapacita`
  - `apriInOutlook`, `segnaLetto`, `bozza`, `rif`/`rifIn`, `statoWorker`
- **`inbox_viva.go`**
  - `novitaPer` (fino a 500 pallini), `lavoroSync`, `conSync`, `aggiornaOra`, `testata`, `syncAllApertura`,
    `cEUnWorkerOutlookVivo`, `segnaVista`, `avvisoShadow`
- **`agente.go`**
  - `analisiPer` (proposta dell'agente già verificata), `chiediAnalisi`
- **`web/templates/inbox.html`**
  - pagina, `inbox_stato`, `inbox_lista` (oggetto con `oggettoVisibile`), `controparte_riga`
- **`frammenti.html`**
  - `messaggio_pannello` (oggetto con `oggettoVisibile`, corpo con `corpo_messaggio`)
  - `corpo_messaggio`, `corpo_blocco`, `stato_worker`, `chip_controparte`, `candidati_aggancio`

### Entry point

- Tutte le rotte sono `autenticato`: i GET da consultazione in su, i POST da operatore in su.
- **`GET /inbox`**
  - Gestore: `inbox` (param `q`, `dir`, `filtro`, `casella`, `sel`; `sel` passa solo se è un UUID)
- **`POST /inbox/aggiorna`**
  - Gestore: `aggiornaOra` (risponde con la testata)
- **`POST /inbox/sync-storico`, `GET /inbox/sync-storico/stato`**
  - Gestore: `syncStorico`, `syncStoricoStato`
- **`GET /stato/worker`**
  - Gestore: `statoWorker`
- **`GET /messaggio/{id}`**
  - Gestore: `messaggio` (senza HTMX reindirizza all'Inbox con `sel`)
- **`POST /messaggio/{id}/apri`, `/letto`, `/bozza`**
  - Gestore: `apriInOutlook`, `segnaLetto`, `bozza`
- **`POST /messaggio/{id}/analizza`**
  - Gestore: `chiediAnalisi`
- `registraInbox` monta anche le rotte di triage, censimento, richieste ai fornitori, download e anteprima; i gestori
  stanno nei loro file.

### Dati

- **Letture:**
  - `v_inbox` (`ListInbox`, fino a 200 righe; `ContaInbox`, `ContaQuadranti`, `GetInboxRiga`), `messaggio`,
    `messaggio_casella` (`ListPresenze`);
  - `candidato_aggancio` e le richieste candidate, `proposta_triage`, `richiesta_fornitore`, `riferimento_portale` e
    `bozza`;
  - `sync_cursore`, `job`, `worker_presenza`, `worker_credenziale`, `casella`, l'ultima analisi dell'agente;
  - `messaggio.corpo_testo` e `corpo_html`, solo per la presentazione: il corpo memorizzato non cambia;
  - `messaggio_casella` (`min(ricevuto_il)` per casella e cartella) per la partenza di «Carica precedenti».
- **Scritture:**
  - job `sync_outlook`: stessa chiave dello scheduler, oppure `sync_storico:<casella>` con finestre per cartella e lotto
    50;
  - job `apri_elemento_outlook` (priorità 1), `segna_letto` (priorità 2), `crea_bozza_outlook` (chiave `bozza:<id>`),
    tutti con postazione, casella, richiedente e scadenza;
  - job `analizza_messaggio_ai` (chiave `analisi-ai:<id>`);
  - riga `bozza`, nella stessa transazione del suo job;
  - `messaggio_casella.non_letto`, solo per la copia su cui si è agito;
  - `utente.ultima_vista_inbox`; `sessione.sync_inbox_il`.

### Flussi principali

1. **Apertura della pagina (non HTMX).**
   - Si rende la lista.
   - Poi `segnaVista` sposta l'ultima visita.
   - Poi `syncAllApertura`, una volta per sessione, solo se un worker Outlook è vivo e la configurazione lo consente.
2. **Poll.**
   - Un elemento fratello rilegge i parametri dalla barra degli indirizzi ogni 15 s, o all'evento `inbox-aggiorna`, e
     sostituisce `#inbox-stato` per intero.
   - La testata si ricarica ogni 15 s, ogni 3 s mentre un sync gira.
3. **Carica precedenti.**
   - Per ogni casella Outlook attiva si fa un solo job storico.
   - La finestra è di `[outlook].giorni_carica_precedenti` giorni per cartella (assente = `GiorniStoricoDefault`, 2), sotto il suo `storico_fino_a`.
   - Una cartella che non ha ancora un limite parte dalla SUA mail più vecchia (`partenzaDellaCartella`), mai dal limite
     di un'altra cartella né dalla mail più vecchia della casella; senza posta, da adesso.
     - Il nome del cursore («Inbox») e quello della copia («Posta in arrivo») si riconoscono come la stessa cartella con
       `nomiCartellaPredefinita` (Inbox, Sent Items, Drafts, Deleted Items, Junk e i nomi italiani);
     - una cartella con un altro nome si cerca con il suo nome e basta.
   - Il badge si ricarica ogni 3 s finché il job non chiude.
4. **Pannello del messaggio.** `caricaMessaggio` prepara `Corpo` con `lettura.Presenta`; `corpo_messaggio` lo disegna:
   - i blocchi di testo, con gli indirizzi (Safe Links srotolati) in grigio e mai cliccabili e le immagini come
     «[immagine]»;
   - le tabelle incollate da Excel come tabelle (`.corpo-tabella`, numeri allineati a destra);
   - il testo automatico di Outlook (avviso di posta esterna, firma di Outlook mobile, invito Teams, riservatezza…) chiuso
     in un `<details>` con la sua etichetta, mai tolto;
   - la storia citata chiusa in «messaggi precedenti citati»;
   - il «testo originale» (`corpo_testo` com'è) a un clic, tranne quando il testo viene dall'HTML perché quello semplice
     era vuoto;
   - un corpo oltre il limite di `lettura` (`Ridotto`) si mostra com'è; un corpo vuoto dice «Nessun testo.».
5. **Azione interattiva.**
   - `copiaInterattiva` usa `coda.CopiaPerPostazione`.
   - Senza postazione, o senza un worker idoneo, non si accoda niente e l'avviso dice perché.
   - Una capacità spenta produce un avviso che nomina la voce di `[sicurezza]`.
6. **Bozza.**
   - `InsertBozza` e il job stanno in una transazione.
   - Se la capacità `bozze` è spenta la bozza non resta: rollback.

### Invarianti

- La lista e i suoi comandi vengono dalla stessa risposta (`inbox_stato`).
- Il poll e la visita sono distinti: il poll non conta come visita e non accoda il sync d'apertura.
- Un job interattivo parte solo verso la copia della postazione della sessione, mai alla «prima copia disponibile».
- Il sync d'apertura parte al massimo una volta per sessione (UPDATE condizionato).
- «Aggiorna ora» accoda al massimo un job per casella.
- `chiediAnalisi` verifica da sé che l'agente sia acceso per le caselle del messaggio: il pulsante nascosto non basta.
- Nessuna bozza viene inviata dal Cockpit: `Invia: false`.
- La pagina non mostra mai l'HTML della mail: solo il testo e le tabelle che `lettura` ne estrae, con l'escape.
  - Gli indirizzi della mail non sono mai collegamenti.
- `sel` entra nella pagina (nell'`hx-get` del pannello e nei collegamenti) solo come UUID riscritto dal server; un altro
  valore vale «nessun messaggio scelto».
- Una cartella dichiarata coperta da «Carica precedenti» è stata letta davvero: ogni finestra parte dal limite o dalla mail
  più vecchia di QUELLA cartella.

### Dipendenze

- `platform/coda`, `platform/contratti/worker`, `platform/db`, `core/inbox/classificazione`, `core/inbox/lettura`,
  `ai/agente`.

### Test

- **L1**
  - `inbox_viva_test.go`: chip della testata con sync e novità, passo del poll, frase di «Aggiorna ora», avviso shadow.
  - `corpo_test.go`:
    - il corpo nel pannello e nella pagina RFQ (tabella, testo automatico chiuso, storia citata, «testo originale», escape
      di uno script, nessun collegamento),
    - l'oggetto senza l'etichetta di posta esterna,
    - «Nessun testo.».
  - `storico_test.go`: i nomi con cui una cartella si ritrova fra le copie.
  - `triage_test.go:TestIlPannelloDiceAltroComeLaLista`: `chip_controparte` con la controparte `altro`.
- **L4**
  - `inbox_viva_db_test.go`: SV1, SV2, SV3.
  - `apertura_db_test.go`: sync all'apertura, refresh e poll, senza worker, spento.
  - `storico_db_test.go`: SS1–SS7 (SS7: una cartella senza limite riparte dalla sua mail più vecchia).
  - `sicurezza_db_test.go:TestNellInboxEntraSoloUnSelCheEUnMessaggio`.
  - `bootstrap_db_test.go`: il frammento porta comandi e lista.
  - `postazione_db_test.go` e `storico_db_test.go` usano `/apri` e `/letto`.
  - `fornitori_db_test.go`: TestIB1, i quadranti.
- **L7:** `inbox_browser_test.go` con `e2e/inbox_quadranti.py`.
- **Senza test HTTP:** `/bozza`, `/analizza`, `/inbox/sync-storico/stato`.

### Stato dell'implementazione

- Completo.
- Una cartella con un nome che non è né quello della configurazione né uno dei nomi noti di Outlook, se la copia porta un
  nome diverso dal cursore, al primo «Carica precedenti» riparte da adesso (`partenzaDellaCartella`):
  - rilegge ciò che il sync ordinario ha già portato, senza dichiarare coperto niente che non abbia letto.

### Dove intervenire

- **capire perché un messaggio sta in quel quadrante**
  - Apro: `v_inbox.quadrante`, `routes_inbox.go:quadranteValido`
- **cambiare l'ampiezza di «Carica precedenti»**
  - Apro: `[outlook].giorni_carica_precedenti`, `routes_inbox.go:giorniStorico`, `finestraStorico`
- **mostrare più (o meno) righe nella lista dell'Inbox**
  - Apro: `[outlook].inbox_max_messaggi`, `routes_inbox.go:righeInbox`, `inboxDati.Piena`
- **da dove parte «Carica precedenti» per una cartella nuova**
  - Apro: `routes_inbox.go:partenzaDellaCartella`, `nomiCartellaPredefinita`
- **cambiare la testata**
  - Apro: `postazione.go:stato`, `inbox_viva.go:conSync`, `frammenti.html:stato_worker`
- **un'azione nuova su Outlook**
  - Apro: `routes_inbox.go:accodaInterattivo`, il payload in `platform/contratti/worker`
- **il pannello del messaggio**
  - Apro: `routes_inbox.go:caricaMessaggio`, `frammenti.html:messaggio_pannello`
- **come si legge il corpo di una mail**
  - Apro: `core/inbox/lettura` (le regole), `frammenti.html:corpo_messaggio`, `corpo_blocco` (il disegno)

### Leggi anche

- `core/README.md` (triage, controparte)
- `platform/README.md` (coda, capacità)
- `transport/README.md`

## Modulo Triage

- `triage.go`
- Nuova RFQ, Aggancia a… e Ignora: le decisioni dell'operatore su un messaggio.

### Scopo

- Precompilare il form: mittente, cliente, buyer, candidati di codice divisi per ruolo, riferimento del cliente,
  candidati di aggancio, allegati, cartella NAS e anteprima della destinazione.
- Applicare la decisione in una transazione con il messaggio bloccato.

### Non appartiene qui

- Il triage deterministico e i candidati: `core/inbox`.
- Il nome della cartella: `core/rfq/documenti.CartellaThread` (qui solo il progressivo « (2)», `cartellaLibera`).
- L'elenco dei domini pubblici: `classificazione.DominioPubblico`, l'unico.
- La preparazione del Fascicolo: `core/rfq/fascicolo`, chiamata qui.

### File

- **`triage.go`**
  - `triageDati` (`Spuntato`: che cosa nasce spuntato), `triageForm`, `datiTriage`
  - `cartella` (per un cliente censito vale sempre l'anagrafica)
  - `buyerSelect`, `cercaThread`, `messaggioDaDecidere`, `nuovaRFQ`, `preparaDopoLaDecisione`, `agganciaEsistente`,
    `ignora`, `avvisoRFQEsistente`, `logDecisione`, `agganciaMessaggioAThread`
  - `clienteDaForm`, `buyerDaForm`, `downloadDaForm`, `triageErrore`, `splitCodici`, `motivoFornitoreSenzaRFQ`,
    `cartellaLibera`
- **`panoramica.go`**
  - `testoLetterale`: `%`, `_` e `\` della ricerca di «Aggancia a…» presi alla lettera
- **`frammenti.html`**
  - `triage_form`, `triage_allegati`, `cartella_cliente`, `cliente_nuovo`, `anteprima_cartella`, `buyer_select`,
    `thread_risultati`, `candidati_codice`, `candidati_aggancio`

### Entry point

- **`GET /messaggio/{id}/triage?azione=nuova|aggancia`**
  - Ruolo: autenticato
  - Gestore: `triageForm` (per la posta di un fornitore `nuova` diventa `aggancia`, con il motivo)
- **`POST /messaggio/{id}/rfq`**
  - Ruolo: ≥ operatore
  - Gestore: `nuovaRFQ` (rifiutata per la posta di un fornitore)
- **`POST /messaggio/{id}/aggancia`**
  - Ruolo: ≥ operatore
  - Gestore: `agganciaEsistente`
- **`POST /messaggio/{id}/ignora`**
  - Ruolo: ≥ operatore
  - Gestore: `ignora`
- **`GET /anagrafica/buyer`**
  - Ruolo: autenticato
  - Gestore: `buyerSelect` (buyer, cartella e anteprima fuori banda)
- **`GET /thread/cerca?q=`**
  - Ruolo: autenticato
  - Gestore: `cercaThread` (RFQ aperte, fino a 30; il testo si prende alla lettera, `testoLetterale`)

### Dati

- Una transazione per decisione, aperta con `BloccaMessaggio` (`SELECT … FOR UPDATE`).
- **Nuova RFQ.**
  - `cliente` e `dominio_cliente`, solo se il cliente è nuovo, con scritture non distruttive (un dominio pubblico secondo
    `classificazione.DominioPubblico` non diventa del cliente); `buyer`.
  - `thread_offerta`, con `cartella_relativa` libera e `riferimento_cliente` di 60 caratteri al massimo.
    - `cartellaLibera`: quella calcolata, oppure la stessa con « (2)», « (3)»… fino a 99, se un'altra RFQ ce l'ha già;
    - confronto senza maiuscole, sotto un advisory lock sul nome fino al COMMIT.
  - `identificativo_thread`: i codici spuntati entrano con l'origine della proposta (`proposta_famiglia` o
    `proposta_generico`) e il loro punteggio; quelli scritti a mano come `manuale` con 100.
  - `fase_log` (RICEVUTA).
  - L'aggancio, il job `crea_cartella_thread` (chiave `cartella:<thread>`), i download spuntati e la preparazione del
    Fascicolo.
- **Aggancio** (`agganciaMessaggioAThread`).
  - `messaggio` (`aggancio = operatore`), `messaggio_aggancio_log`, `documento_proposta` e `riferimento_portale` del
    messaggio.
  - `proposta_triage` passa ad accettata; `conversazione` viene collegata.
  - Per gli altri orfani della stessa conversazione: un `candidato_aggancio` R1 e `proposta_triage` aggiornata, **non** un
    aggancio.
  - Il buyer sul messaggio e sulla RFQ (`SetBuyerMessaggio`, `SetBuyerThread`): un errore ferma l'aggancio.
- **Ignora:** `IgnoraMessaggio` e il log.

### Flussi principali

1. Form → `datiTriage`.
   - Nasce spuntato solo un codice di famiglia trovato nel testo nuovo (`triageDati.Spuntato`):
     - un codice di famiglia trovato solo nella storia citata (`classificazione.DoveStoria`) si vede, con la sua evidenza,
       ma per entrare vuole un clic.
   - Se il cliente non ha famiglie di codice, i numeri generici diventano spuntabili ma non nascono spuntati.
   - Il riferimento del cliente ha un campo suo e non diventa mai un codice.
   - Per la posta di un fornitore il form di «Nuova RFQ» non si offre: `triageForm` passa ad «Aggancia» con il motivo.
2. Nuova RFQ.
   - Blocco del messaggio.
   - Già agganciato → avviso esplicito (`avvisoRFQEsistente`).
   - Controparte `fornitore` (letta dal messaggio appena bloccato) → rifiuto con `motivoFornitoreSenzaRFQ`, niente
     scritto.
   - Altrimenti cliente, buyer, cartella libera, thread, codici, fase, aggancio, job della cartella, download,
     `preparaDopoLaDecisione`, commit, pannello con l'avviso.
     - Anche con `nas_scrittura` spenta la RFQ nasce e la cartella resta in attesa.
3. Aggancia: stessi passi senza la creazione del thread.
   - Il buyer viene creato solo con `crea_buyer=1`, e un suo errore (logico o del database) ferma l'aggancio e torna nel
     form (`triageErrore`).

### Invarianti

- Una decisione per messaggio: tutte le strade passano da `messaggioDaDecidere`, e chi perde la corsa riceve un esito
  esplicito.
- Un codice non proposto non entra dalla spunta; un riferimento RFQ non diventa un identificativo nemmeno se spuntato.
- Per un cliente censito la cartella viene dall'anagrafica, mai dal campo digitato.
- Due RFQ non condividono una cartella del NAS: la seconda con lo stesso nome prende « (2)».
- La posta di un fornitore non apre una RFQ cliente: la regola è nel pannello, nel form e nel POST.
- Agganciare un messaggio non aggancia gli altri orfani della conversazione: ricevono solo un candidato R1 (T21).

### Dipendenze

- `core/inbox/classificazione`, `core/registro/{anagrafica,regole}`, `core/rfq/{documenti,fascicolo}`,
  `platform/{coda,contratti/worker,db}`.

### Test

- **L1:** `triage_test.go`
  - un codice della storia citata non nasce spuntato;
  - `chip_controparte` con `altro`;
  - la ricerca di «Aggancia a…» alla lettera.
- **L4**
  - `candidati_db_test.go`: T21, T23, codice inventato.
  - `cartella_db_test.go`: cartella dall'anagrafica e destinazione.
  - `campicliente_db_test.go`: i campi del cliente nuovo compaiono e spariscono.
  - `decisioni_db_test.go`: due nuove RFQ concorrenti ne creano una sola.
  - `b87b_db_test.go`: preparazione dopo la decisione e aggancio.
  - `richieste_db_test.go`: IB6, ignora.
  - `triage_db_test.go`:
    - un webmail non diventa il dominio di un cliente nuovo;
    - un no sul buyer ferma l'aggancio (`crea_buyer`);
    - la posta di un fornitore non apre una RFQ nemmeno a mano;
    - due RFQ con lo stesso nome prendono « (2)» e « (3)».

### Stato dell'implementazione

- Completo.
- L'anteprima della destinazione nel form (`anteprima_cartella`) mostra il nome calcolato: il progressivo « (2)» si
  decide solo quando la RFQ nasce.

### Dove intervenire

- **cambiare che cosa succede alla creazione di una RFQ**
  - Apro: `triage.go:nuovaRFQ`, `preparaDopoLaDecisione`
- **cambiare la propagazione alla conversazione**
  - Apro: `triage.go:agganciaMessaggioAThread`
- **cambiare il form**
  - Apro: `triage.go:datiTriage`, `frammenti.html:triage_form`
- **cambiare che cosa nasce spuntato**
  - Apro: `triage.go:triageDati.Spuntato`, `frammenti.html:candidati_codice`
- **capire la destinazione sul NAS**
  - Apro: `triage.go:cartella`, `cartellaLibera`, `core/rfq/documenti`

### Leggi anche

- `core/README.md`
- `internal/README.md` (flusso «Nuova RFQ»)

## Modulo Anagrafica e censimento

- `censisci.go`, `anagrafica.go`, `anagrafica_admin.go`, `convenzioni_admin.go`, `fornitori_admin.go`
- Clienti e fornitori: censiti dal pannello del messaggio (operatore) o amministrati in Admin › Anagrafica.

### Scopo

- **Censimento dall'Inbox:** fornitore o cliente con dominio e/o indirizzo, poi il ricalcolo dei soli messaggi non
  decisi.
- **Admin › Anagrafica › Clienti:**
  - le schede generale, contatti, riconoscimento, fabbisogno, lavorazioni, prova;
  - regole come form o come JSON, sempre convalidate;
  - banco di prova con il motore vero;
  - convenzioni di codice, qualifiche dei fornitori, «prova un codice».
- **Admin › Fornitori:** le schede generale, contatti, lavorazioni, posta, e l'import del seme con anteprima.

### Non appartiene qui

- Lo schema e la convalida delle regole: `core/registro/regole`.
- Il riconoscimento: `core/inbox/classificazione`.
- Il ricalcolo: `core/inbox/ingest`.
- La logica del seme dei fornitori: `core/registro/fornitori`.

### File

- **`censisci.go`**
  - `censisciForm`, `datiCensisci`, `censisci`, `censisciFornitore`, `censisciCliente`, `cognomeENome`, `ritriagePer`,
    `AggiungiDominioFornitore`
- **`anagrafica.go`**
  - `vincoloViolato`, `CreaCliente`, `AggiungiDominio`, `rendiAnagrafica`, `nuovoCliente`
  - `salvaCliente` (peso 0–15)
  - `salvaRegole`, `aggiungiDominioCliente`, `eliminaDominioCliente`, `bancoProva`, `txtN`
- **`anagrafica_admin.go`**
  - `salvaRegoleDalForm` («Rev nel codice» letta per numero di riga), `scriviRegole`
  - `nuovoBuyerCliente`, `eliminaBuyerCliente` (solo un buyer del cliente della pagina)
  - `aggiungiFabbisogno` (riga e fonte attesa in una transazione), `eliminaFabbisogno`
  - tendine
- **`convenzioni_admin.go`**
  - `nuovaConvenzione` (in transazione), `eliminaConvenzione`, `attivaConvenzione`, `qualificaFornitore`,
    `eliminaQualificaCliente`, `provaCodiceCliente`
- **`fornitori_admin.go`**
  - `rendiFornitori`, `nuovoFornitore`, `salvaFornitore`, domini e contatti
  - `salvaLavorazioniFornitore` (tutto o niente, in una transazione)
  - qualifiche, `importaFornitoriForm`, `importaFornitori`, `testoDelSeme` (4 MB al massimo)
- **template**
  - `anagrafica.html`, `fornitori.html`, `importa.html`, `frammenti.html:censisci_form`

### Entry point

- **Censimento, operatore:** `GET` e `POST /messaggio/{id}/censisci`.
- **Clienti, `soloAdmin`:**
  - `GET /admin/anagrafica`, `GET /admin/anagrafica/articoli` (scheletro);
  - `POST /admin/anagrafica`, `POST /admin/anagrafica/{id}`;
  - `…/{id}/regole`, `…/regole/form`;
  - `…/buyer`, `…/buyer/elimina`;
  - `…/fabbisogno`, `…/fabbisogno/elimina`;
  - `…/dominio`, `…/dominio/elimina`;
  - `…/prova`;
  - `…/convenzione`, `…/convenzione/elimina`, `…/convenzione/attiva`;
  - `…/qualifica`, `…/qualifica/elimina`;
  - `…/lavorazioni/prova`.
- **Fornitori, `soloAdmin`:**
  - `GET` e `POST /admin/fornitori`;
  - `GET` e `POST /admin/fornitori/importa`;
  - `POST /admin/fornitori/{id}`;
  - `…/dominio`, `…/dominio/elimina`;
  - `…/contatto`, `…/contatto/elimina`;
  - `…/lavorazioni`;
  - `…/qualifica`, `…/qualifica/elimina`.
- I fornitori stanno sotto `/admin/fornitori` e non sotto `/admin/anagrafica/fornitori/{id}`:
  - con il mux di Go quel pattern andrebbe in conflitto con `/admin/anagrafica/{id}/regole`.

### Dati

- **Tabelle scritte:**
  - lato cliente:
    - `cliente` (insert, update, `regole`), `dominio_cliente`;
    - `buyer` (`EliminaBuyer` cancella solo un buyer del cliente della pagina, e non uno citato da messaggi, RFQ o
      proposte di triage);
    - `fabbisogno_documento` (si tolgono solo le righe del cliente, non i predefiniti);
    - `convenzione_codice` e `convenzione_codice_lavorazione`;
  - lato fornitore: `cliente_fornitore_lavorazione`, `fornitore`, `dominio_fornitore`, `contatto_fornitore`,
    `fornitore_lavorazione`.
- **Scritture non distruttive:** un 23505 diventa una frase che dice di chi è la cartella o il dominio.
- **Transazioni:** nessuna nelle scritture singole, di proposito, perché la domanda «di chi è» deve poter rispondere.
  - Stanno in una transazione le scritture di più righe:
    - `nuovaConvenzione`,
    - `aggiungiFabbisogno` (la riga e la sua fonte attesa),
    - `salvaLavorazioniFornitore` (un rifiuto a metà giro lascia le lavorazioni com'erano).
- **Ricalcolo:** dopo l'aggiunta di un dominio, di un contatto, di un buyer con e-mail, dopo un censimento o
  l'applicazione del seme → `ingest.Ritriage` o `RitriageMolti`, sui soli messaggi non decisi.

### Flussi principali

1. **Censisci.**
   - `ingest.IndirizzoDaCensire`: in entrata il mittente, in uscita il primo destinatario esterno.
   - Su un dominio pubblico il predefinito è l'indirizzo.
   - Fornitore: si riusa quello con la stessa ragione sociale e gli si aggiungono dominio, contatto e lavorazioni.
   - Cliente: `CreaCliente`, `AggiungiDominio`, buyer.
   - Poi il ricalcolo mirato e il pannello con l'esito.
2. **Regole.**
   - Il form ricostruisce `regole.Regole`; il JSON arriva così com'è scritto.
   - Le famiglie arrivano come array paralleli, tranne «Rev nel codice»:
     - è una casella di spunta, il browser manda solo quelle spuntate, e ognuna porta il numero della sua riga
       (`fam_rev` = indice in `anagrafica.html`).
   - Entrambi passano da `regole.ValidaRegole`; ciò che non passa torna nel riquadro con la diagnosi riga per riga.
   - Le regole valgono dal prossimo lotto di posta.
3. **Import del seme.**
   - Anteprima con `fornitori.Calcola`;
   - «applica» con `fornitori.Applica`, poi il ricalcolo sulle chiavi scritte.

### Invarianti

- Una cartella NAS, un profilo o un dominio già di un altro soggetto non si spostano: l'operazione fallisce e dice di chi
  sono (T7, T8).
- La pagina di un cliente cancella solo le persone di quel cliente; una persona citata da messaggi, RFQ o proposte di
  triage resta.
- Il ricalcolo tocca solo i messaggi non decisi.
- Una regola o una convenzione che non rispetta lo schema non entra in database.
- Il banco di prova e «prova un codice» usano lo stesso motore dell'ingest.

### Dipendenze

- `core/registro/{regole,fornitori}`, `core/inbox/{classificazione,ingest}`, `core/rfq/documenti` (`NomeSicuro`),
  `platform/db`.

### Test

- **L4**
  - `anagrafica_db_test.go`: T7, T8, regola rotta, banco di prova, operatore escluso, peso, scheda del fabbisogno.
  - `fornitori_db_test.go`: CP5 censisci come fornitore, censisci come cliente, schede e scritture non distruttive,
    convenzioni, CP14 import, l'anagrafica ricalcola i messaggi.
  - `bootstrap_db_test.go`: l'import ricalcola i messaggi già arrivati.
  - `regole_form_db_test.go`:
    - «Rev nel codice» resta sulla famiglia giusta (`POST …/regole/form`);
    - la riga del fabbisogno nasce con la sua fonte (`…/fabbisogno`).
  - `buyer_db_test.go`: un buyer si cancella solo dalla pagina del suo cliente, e non se una proposta di triage lo cita
    (`…/buyer/elimina`).
  - `transazioni_admin_db_test.go:TestLeLavorazioniDiUnFornitoreSiSalvanoTutteONiente`.

### Stato dell'implementazione

- Completo; la scheda Articoli è uno scheletro (blocco 5).
- Limite noto: togliere un dominio, un contatto o un buyer non ricalcola i messaggi.

### Dove intervenire

- **un campo nuovo nelle regole del cliente**
  - Apro: `core/registro/regole`, `anagrafica_admin.go:salvaRegoleDalForm`, `anagrafica.html`
- **cambiare la regola «non distruttiva»**
  - Apro: `anagrafica.go:CreaCliente`, `AggiungiDominio`, `censisci.go:AggiungiDominioFornitore`
- **cambiare il censimento dall'Inbox**
  - Apro: `censisci.go`, `frammenti.html:censisci_form`
- **l'import dei fornitori**
  - Apro: `fornitori_admin.go:importaFornitori`, `core/registro/fornitori`

### Leggi anche

- `core/README.md` (registro, regole, ricalcolo)
- `internal/README.md` (flusso «Censisci dall'Inbox»)

## Modulo Postazioni

- `postazione.go`, `postazioni_admin.go`
- La postazione della sessione, la testata e la pagina che genera i pacchetti dei worker.

### Scopo

- Sapere da quale PC lavora la sessione: abbinamento per IP, riabbinamento, scelta in testata.
  - Così i job interattivi aprono le finestre sul PC giusto.
- Dire in testata lo stato di ogni casella e del worker di analisi.
- Generare segreti e pacchetto di una postazione.

### Non appartiene qui

- Quali postazioni e quali worker esistono e che caselle servono: si dichiara in `cockpit.toml` e lo semina
  `platform/fondazioni`.
- L'autenticazione dei worker: `transport/workerapi`.

### File

- **`postazione.go`**
  - `sessioneUI`, `sessioneDa`
  - `puoUsarePostazione` (postazione attiva e dell'utente, oppure admin), `postazioniScegliibili`
  - `indirizzoDi` (solo `RemoteAddr`, `X-Forwarded-For` mai)
  - `abbinaPostazione` (IP visto entro 24 h, una sola postazione autorizzata)
  - `scegliPostazione` (una postazione non autorizzata → `nega`, senza ruolo), `dopoScelta`
  - `statoUI`, `stato`, `statoCaselle`, `statoAnalisi`, `durataBreve`
- **`postazioni_admin.go`**
  - `URLServer` (`[server].url_pubblico`, altrimenti derivato dal bind), `adminPostazioni`
  - `rendiPostazioni` (stato delle credenziali con `fondazioni.StatoCredenziali`)
  - `pacchettoWorker` (token e zip in una transazione)
  - `workerTOML`, `istruzioni`, `servePostazione`, `zipPacchetto`, `registraPostazioni`
- **template**
  - `postazioni.html`, `frammenti.html:stato_worker`

### Entry point

- **`POST /sessione/postazione`**
  - Ruolo: ≥ operatore
  - Gestore: `scegliPostazione` (vuoto = «non lo so», l'origine si azzera; da htmx 204 con `HX-Refresh`, altrimenti
    redirect a `/inbox`)
- **`GET /admin/postazioni`**
  - Ruolo: admin
  - Gestore: `adminPostazioni`
- **`POST /admin/postazioni/{host}/pacchetto`**
  - Ruolo: admin
  - Gestore: `pacchettoWorker` (POST perché invalida i token; risponde con lo zip, `application/zip` in download)
- `stato` lo chiamano `rendi`, `nega`, `statoWorker` e `testata`.

### Dati

- **Letture:**
  - `postazione`,
  - `worker_presenza` (vivo o spento si decide su `ultimo_contatto` contro `worker.PresenzaOnlineEntro`),
  - `worker_credenziale`, `casella`.
- **Scritture:**
  - `sessione.postazione_id` e `postazione_origine`, che vale `ip` o `scelta`;
  - `worker_credenziale.token_hash`: lo sha256 del token nuovo, `rete.ImprontaToken`.
- **Pacchetto:** zip in memoria con
  - `worker.toml`: URL, impronta TLS, un token per worker; nella tabella `[analisi]` anche l'OCR della postazione
    (`ocr`, `ocr_lingua`, `ocr_dpi`, `ocr_tessdata`), se la sua `[[postazione]]` nel `cockpit.toml` lo dichiara: arriva
    in `Server.OCRPostazioni` (`OCRPostazione`, per host maiuscolo) da `app/runtime`, senza database. Solo le chiavi
    dichiarate; `ocr_tessdata` fra apici singoli (stringa letterale TOML, i backslash restano tali);
  - `ISTRUZIONI.txt`;
  - `cert.pem`, solo con TLS e solo il certificato pubblico;
  - i file di `workers/` incorporati nel binario, meno i test e gli strumenti di sviluppo.

### Flussi principali

1. **Login e ogni richiesta.**
   - Una sessione senza origine prova il riabbinamento per IP.
   - Una scelta fatta in testata non viene mai sovrascritta.
2. **Testata.**
   - Stato di ogni casella: `attiva`, `in_corso`, `offline`, `non_risolta`, `non_configurata`.
   - Si aggiungono novità e ultimo sync riuscito (`conSync`).
3. **Pacchetto.**
   - In una transazione: per ogni worker della postazione token nuovo e impronta in database; poi lo zip, costruito PRIMA
     del COMMIT.
   - Se lo zip non nasce, rollback: i token di prima valgono ancora.
   - Dopo il COMMIT il download; i token precedenti smettono di valere subito.
4. **Scelta non autorizzata.**
   - `scegliPostazione` risponde con `nega` (403, pagina o frammento del divieto) e un motivo che dice dove si abilita la
     postazione (`[[postazione]]` in `cockpit.toml`);
   - la pagina non nomina un ruolo.

### Invarianti

- L'IP è un indizio su quale postazione, mai un permesso: sia l'abbinamento sia la scelta passano da
  `puoUsarePostazione`.
- Il riabbinamento tocca solo le sessioni senza origine.
- In database c'è solo l'impronta dei token; i segreti si vedono una volta sola, nello zip.
- I token di una postazione cambiano tutti insieme, e solo se lo zip che li consegna è nato.
- Il pacchetto lo genera solo un amministratore.

### Dipendenze

- `platform/{db,fondazioni,rete,contratti/worker}`.

### Test

- **L1:** `postazione_test.go` (M2, i tre stati; presenza dal contatto; `puoUsarePostazione`);
  `pacchetto_ocr_test.go` (l'OCR della postazione solo sotto `[analisi]`, TOML valido, valori riletti identici).
- **L4**
  - `postazione_db_test.go`: W14 sessione, riabbinamento, P1, M2 testata.
  - `presenza_db_test.go`: 4 test.
  - `rete_db_test.go`: PK1 token che funziona, solo admin, indirizzo del server.
  - `pacchetto_postazione_db_test.go`: `cert.pem` e script.
  - `migrazione_credenziali_db_test.go`: PK2, credenziale mai generata, stato delle credenziali.
  - `transazioni_admin_db_test.go:TestUnPacchettoNonCostruitoNonCambiaITokenDellaPostazione`.
- La pagina del divieto senza ruolo: `ruoli_test.go:TestIlDivietoDiceIlRuoloCheServeDavvero` (L1).

### Stato dell'implementazione

- Completo.
- **Limiti noti al 25/09:**
  - la scelta parte con htmx dalla testata, e un 403 da htmx non si innesta (vedi il modulo Server e ruoli):
    - il frammento del divieto non compare, resta la riga nel log.
    - La tendina offre solo le postazioni scegliibili, quindi ci si arriva con una lista rimasta vecchia o con una
      richiesta scritta a mano;
  - il ramo del template «solo un amministratore» non si raggiunge, perché la pagina è `soloAdmin`.

### Dove intervenire

- **cambiare lo stato di una casella in testata**
  - Apro: `postazione.go:statoCaselle`, `inbox_viva.go:conSync`
- **cambiare l'abbinamento per IP**
  - Apro: `postazione.go:abbinaPostazione`, `finestraAbbinamentoIP`
- **cambiare il contenuto del pacchetto**
  - Apro: `postazioni_admin.go:workerTOML`, `zipPacchetto`, `servePostazione`, `embed.go`
- **aggiungere una chiave del worker.toml che si dichiara per postazione** (come l'OCR)
  - Apro: `platform/config` (`Postazione`, `validaOCR`), `app/runtime/ascolto.go:ocrDellePostazioni`,
    `postazioni_admin.go:OCRPostazione.scrivi`

### Leggi anche

- `workers/workers_README.md`
- `platform/README.md` (`rete`, `fondazioni`)

## Modulo Admin tecnico

- `routes_admin.go`, `integrita_admin.go`
- Coda job, scarti dell'ingest, Integrità NAS e il montaggio di tutte le rotte `/admin/*`, tranne Postazioni.

### Scopo

- Guardare e correggere il motore: riaccodare o annullare un job, riprovare uno scarto.
- Confrontare ciò che il database promette con i file veri sul NAS, e offrire solo le azioni sicure.

### Non appartiene qui

- Il ricognitore e l'allineamento: `core/rfq/documenti` (`Ricognitore.Giro`, `AllineaDocumento`).
- La riprova degli scarti: `core/inbox/ingest` (`Servizio.Riprova`).

### File

- **`routes_admin.go`**
  - `registraAdmin`: tutte le rotte `/admin/{job,nas,scarti,anagrafica,fornitori}` con `soloAdmin`
  - `adminJob`, `riaccodaJob`, `annullaJob`, `adminScarti`, `riprovaScarto`
- **`integrita_admin.go`**
  - `integritaDati`, `rigaIntegrita` (`StatoFilesystem`, `Azione`), `adminIntegrita`, `datiIntegrita`,
    `integritaFrammento`, `controllaIntegrita`, `frasePassata`, `riaccodaDocumento`, `allineaDocumento`
- **template**
  - `job.html` (poll 5 s), `scarti.html` (poll 10 s), `integrita.html`

### Entry point

- Tutte `soloAdmin`:
  - `GET /admin/job?stato=`, `POST /admin/job/{id}/riaccoda`, `POST /admin/job/{id}/annulla` (riuscite: 204 con
    `HX-Refresh`);
  - `GET /admin/scarti?origine=ingest|lettura`, `POST /admin/scarti/{id}/riprova`;
  - `GET /admin/nas`, `POST /admin/nas/controlla`, `POST /admin/nas/{id}/riaccoda`, `POST /admin/nas/{id}/allinea`
    (`{id}` è il `documento_id`).
- `stato` vale solo se è uno stato di job, `origine` solo se è `ingest` o `lettura`; un altro valore vale «tutti».
  - Nel poll della pagina (`hx-get` di `job.html` e `scarti.html`) torna il valore riconosciuto, mai il testo
    dell'indirizzo.

### Dati

- `job`: fino a 100 righe per stato;
  - `RiaccodaJob` solo per job falliti o annullati e senza un gemello pendente con la stessa chiave;
  - `AnnullaJob` solo per job pronti o in corso.
- `ingest_scarto`: fino a 200 righe.
- `nas_anomalia`: segnalazioni aperte e conteggi.
- `documento`: `coda.AccodaCopia` rispetta `nas_scrittura`; `AllineaDocumento` ricalcola l'hash sul file vero.

### Flussi principali

1. **«Controlla ora».** `Ricognitore.Giro`, la stessa funzione del giro automatico, poi la tabella ridisegnata per intero
   con la frase dell'esito.
2. **Azioni sulle segnalazioni.**
   - `riaccoda` per le copie in attesa, in errore o mancanti.
   - `allinea` per un file già presente con l'hash giusto.
   - Niente per un conflitto o un file illeggibile.

### Invarianti

- Un conflitto o un file illeggibile non si riaccodano: il gestore rifiuta anche se il pulsante venisse forzato.
  - Il Cockpit non sovrascrive mai un file sul NAS.
- Si allinea solo `gia_presente`, dopo aver ricalcolato l'hash.
- Riaccodare non viola l'indice unico delle chiavi di idempotenza: il caso diventa un avviso.

### Dipendenze

- `core/rfq/documenti`, `core/inbox/ingest` (tramite `Server.Ingest`), `platform/{coda,db}`.

### Test

- **L4**
  - `integrita_db_test.go`: 7 test (mai guardato, file mancante, conflitto, allineamento, avviso nella RFQ, scrittura
    spenta, solo admin).
  - `rbac_db_test.go`: le schermate tecniche con admin e con operatore.
  - `sicurezza_db_test.go:TestIlPollDellAdminNonRiportaIlTestoDellIndirizzo`.

### Stato dell'implementazione

- Completo.
- Il difetto noto di «riprova» sugli scarti di origine `lettura` è descritto in `transport/README.md`.

### Dove intervenire

- **una schermata admin nuova**
  - Apro: `routes_admin.go:registraAdmin` (con `soloAdmin`), `navigazione.go:navPer`
- **cambiare le azioni sulle anomalie**
  - Apro: `integrita_admin.go:rigaIntegrita.Azione`, `riaccodaDocumento`, `allineaDocumento`

### Leggi anche

- `core/README.md` (integrità NAS)
- `platform/README.md` (coda)

## Modulo Pagina RFQ

- `routes_rfq.go`, `thread.go`, `proposte.go`, `codici.go`
- La pagina di una richiesta, `/thread/{id}`, e i gesti sulle proposte di struttura (B8.5) e sui codici (B8.6).

### Scopo

- Mostrare testata, messaggi agganciati, documenti sul NAS con i loro stati, anomalie, riepilogo del Fascicolo, pannello
  dei codici e bozze.
  - Il testo di ogni messaggio è quello del pannello dell'Inbox: `messaggioThread.Corpo` da `lettura.Presenta`, disegnato
    con `corpo_messaggio`; l'oggetto con `oggettoVisibile`.
- Aprirsi senza scrivere niente (Smistamento F1, R8: nessuna GET scrive), per nessun ruolo. Per chi scrive la pagina
  manda da sola la preparazione (`POST …/fascicolo/prepara`, `auto=1`), che accoda e basta: vedi il modulo della
  preparazione del Fascicolo. Gli STEP già analizzati si rileggono solo con il gesto «Rianalizza» e quando la RFQ nasce
  o un messaggio vi si aggancia (`triage.go:preparaDopoLaDecisione`).
- Eseguire i gesti in una transazione ciascuno, rispondendo con la pagina o con la schermata del Fascicolo.

### Non appartiene qui

- La schermata del Fascicolo e i suoi gesti: `fascicolo*.go`.
- Conferma e scarto delle proposte di allegato: `allegati.go`.
- Le regole sulle proposte: `core/rfq/fascicolo`.

### File

- **`routes_rfq.go`**
  - `registraRFQ`: monta anche le rotte di `fascicolo*.go` e `allegati.go`
  - `cruscotto` reindirizza con 303 a `/richieste`
- **`thread.go`**
  - `threadDati`, `messaggioThread` (con `Corpo`), `thread` (`sel` solo se UUID)
  - `threadFrammento` (se il gesto viene dal Fascicolo risponde con `rispondiFascicolo`)
  - `riprovaCopie`, `rimettiInCoda`, `esitoCopie`, `caricaThread`
- **`proposte.go`**
  - `registraProposte`, `gesto`/`gestoPoi` (tutto o niente, «Niente è cambiato: …»), `idDa`
  - `rianalizza` (fino a `MaxAccodatiSuRichiesta` = 20: chiama `fascicolo.RianalizzaRfq`, e ne è il solo chiamante), `fraseRianalisi`
    (LS-01: gli STEP da rileggere contati a parte, senza un perché; «Riscarica, poi di nuovo Rianalizza» per quelli non
    più in staging)
  - accetta/scarta per nodo, relazione, file o sottoalbero, rimozione
  - `gestoConDomanda`: per `relazione/accetta` e `file/{aid}/accetta` (campo `assieme`, ripetuto), la conferma
    contestuale del cambio di tipo (A1d P4, D-K6-01 = A [U], direttiva 12 §6)
- **`fascicolo_conversione.go`**
  - `confermaTipo`, `domandaConversione`, `decisioneTipoDal`, `soloDalFascicolo`: la domanda «Il componente B è
    attualmente un particolare…» al posto dell'avviso (`fasc_avviso`), solo dalla schermata del Fascicolo e non
    dalla vista Documenti; «Sì» rimanda lo stesso gesto con l'id di quel componente, «No» chiude la domanda nel
    browser. Altrove il rifiuto che nomina la scheda
  - `stradaInPiuDellaConversione`, `dallaVistaDocumenti`, `stradaDallaVistaAlbero`: dalla vista Documenti, dove `fascicolo.mjs`
    svuota l'avviso al primo clic e il «Sì» non partirebbe, il rifiuto nomina anche la vista Albero (revisione D-E
    del 13/10, rilievo 1)
- **`codici.go`**
  - `registraCodici`, `aggiungiCodice`, `ripristinaComponente`, `rigaCodice`, `etichettaTipo`, `BersaglioCodici`,
    `codiciDellaRfq`
- **template**
  - `thread.html`, `frammenti.html:codici_rfq`, `codice_riga`, `corpo_messaggio`

### Entry point

- Tutte `autenticato`; le POST richiedono almeno operatore.
- **Pagina e copie:** `GET /thread/{id}` (param `sel`, solo UUID), `POST /thread/{id}/riprova-copie`.
- **Proposte di struttura:**
  - `POST /thread/{id}/fascicolo/rianalizza`;
  - `…/nodo/{pid}/accetta|scarta|codice`;
  - `…/relazione/accetta|scarta` (campi `allegato`, `padre`, `figlio`);
  - `…/file/{aid}/accetta` (con `chiave` solo il sottoalbero);
  - `…/rimozione/accetta|scarta` (campi `step`, `padre`, `figlio`).
- **Codici:** `POST /thread/{id}/fascicolo/codice/aggiungi` (campi `codice`, `tipo`, `rev`),
  `…/componente/{cid}/ripristina`.
- **Redirect:** `GET /cruscotto`.

### Dati

- **Letture:**
  - `thread_offerta`, `v_cruscotto` (anche `n_da_smistare`), `cliente`, `identificativo_thread`;
  - `documento`, `v_fascicolo`, `bozza`, `componente`, `nas_anomalia`;
  - messaggi con presenze e allegati (il corpo solo per la presentazione); `fascicolo.CandidatiDellaRfq`.
- **Scritture:**
  - all'apertura nessuna: `GET /thread/{id}` legge e basta (`TestAprireLaRfqNonRileggeGliStepEIGestiRispondonoConLaPagina`);
  - i gesti tramite le funzioni di `core/rfq/fascicolo`, in una transazione; «Rianalizza» con `fascicolo.RianalizzaRfq`
    (se `[analisi]` è configurata), solo su richiesta;
  - `riprova-copie`: `coda.AccodaCopia` per ogni documento `in_coda` o `errore`.

### Flussi principali

1. **Apertura.**
   - `caricaThread` e `rendi`: niente si scrive, niente si rilegge.
   - Per chi scrive, `thread.html` porta la POST della preparazione (`hx-trigger="load"`, `auto=1`, `hx-swap="none"`):
     `fascicolo.PreparaFile` e `fascicolo.AccodaAnalisiMancantiDaSola` accodano download, estrazioni e analisi
     mancanti, senza rileggere gli STEP già analizzati (modulo della preparazione del Fascicolo).
2. **Gesto.**
   - `gestoPoi`: transazione → funzione di dominio → commit, oppure rollback con la spiegazione (`spiegaErrore`).
   - Risposta con `threadFrammento`: la pagina della RFQ, oppure i pannelli del Fascicolo se `HX-Current-URL` è quello
     della schermata.
3. **Riprova copie.**
   - Si conta documento per documento: accodati, già in coda, errori, già sul NAS.
   - Una capacità spenta ferma il giro.

### Invarianti

- Un gesto è tutto o niente.
- Un documento che non si accoda non ferma gli altri.
- Il numero «da smistare» è uno solo, quello di `v_cruscotto`.
- La pagina non crea richieste ai fornitori (`TestLaRFQNonCreaRichiesteAiFornitori`).
- Il corpo di un messaggio è solo testo e tabelle estratte, come nell'Inbox; `sel` entra nella pagina solo come UUID.

### Dipendenze

- `core/rfq/fascicolo`, `core/inbox/lettura`, `platform/{coda,db}`.

### Test

- **L1:**
  - `codici_test.go` (ogni codice al suo gesto; BOM congelata),
  - `esitocopie_test.go`,
  - `templates_test.go`,
  - `corpo_test.go` (il corpo in `thread_corpo`).
- **L4**
  - `b85_db_test.go`: aprire la RFQ non scrive e non rilegge gli STEP (la preparazione accoda, «Rianalizza» rilegge),
    gesti che rispondono con la pagina, scelte.
  - `ls01_rianalizza_db_test.go` (LS-01): le frasi vere di «Rianalizza» su uno STEP letto nella forma 4 di GP-3, nello
    staging e non più nello staging, prima e dopo il risultato del worker corretto.
  - `b86_db_test.go`: pannello dei codici.
  - `copie_db_test.go`, `copieerrore_db_test.go`, `copieparziali_db_test.go`, `chiavecopia_db_test.go`.
  - `integrita_db_test.go`: la RFQ dice che un suo documento è segnalato.
- Le rotte `relazione/*` e `rimozione/*` non hanno test HTTP in questo package.

### Stato dell'implementazione

- Completo.
- La rotta della richiesta a un fornitore resta senza form (vedi il modulo Richieste ai fornitori).
- Fino a B8.7b l'apertura rileggeva gli STEP e scriveva anche per un utente in consultazione; dallo Smistamento F1 (R8)
  non scrive per nessuno.

### Dove intervenire

- **un gesto nuovo sulle proposte**
  - Apro: `proposte.go` (con `gesto`), la funzione in `core/rfq/fascicolo`
- **cambiare che cosa mostra la pagina**
  - Apro: `thread.go:caricaThread`, `thread.html`
- **cambiare il pannello dei codici**
  - Apro: `codici.go`, `frammenti.html:codici_rfq`, `codice_riga`

### Leggi anche

- `transport/README.md` (rotte del Fascicolo)
- `core/README.md` (`fascicolo`)

## Modulo Richieste ai fornitori, blocco 7B

- `richieste.go`
- Le richieste d'offerta a un fornitore e il loro legame con la posta.
- Da non confondere con la pagina Richieste, che sta in `panoramica.go`.

### Scopo

- Creare una richiesta, con la bozza marcata in Outlook se richiesta.
- Confermare dall'Inbox:
  - «è la risposta a questa richiesta», con un atto: collegata, offerta, declinata;
  - «è la richiesta mandata a mano».

### Non appartiene qui

- Il riconoscimento del marcatore e dei candidati (`core/inbox/ingest`, `core/inbox/aggancio`).

### File

- **`richieste.go`**
  - `nuovaRichiestaFornitore`, `bozzaPerRichiesta`
  - `oggettoRichiesta` («RFQ <cartella> <buyer> <codici>»), `corpoRichiesta`
  - `annullaRichiestaFornitore`, `rispostaFornitore`, `richiestaFornitoreManuale`

### Entry point

- Tutte `autenticato`, POST da operatore in su:
  - `POST /thread/{id}/richiesta` (campi `fornitore_id`, `lavorazione`, `codice`, `note`, `bozza=1`) — nessun form la
    chiama più;
  - `POST /thread/{id}/richiesta/{rid}/annulla`;
  - `POST /messaggio/{id}/risposta-fornitore` (campi `richiesta_id`, `atto`);
  - `POST /messaggio/{id}/richiesta-fornitore` (campi `thread_id`, `fornitore_id`, `lavorazione`).

### Dati

- **Tabelle scritte:**
  - `richiesta_fornitore`: stato `bozza` o `inviata`, poi `annullata`, `offerta_ricevuta` o declinata;
  - `bozza`, con il job `crea_bozza_outlook` e `Marcatori{MarcatoreRichiesta}`;
  - `messaggio.richiesta_fornitore_id`;
  - `documento_proposta`: con l'atto «offerta» gli allegati ancora aperti diventano offerta del fornitore.
- **Transazioni:**
  - le conferme dall'Inbox stanno in una transazione con il messaggio bloccato;
  - la creazione della richiesta è una scrittura sola; la bozza ha una transazione sua.

### Flussi principali

1. **Nuova richiesta.** Riga `bozza` → con `bozza=1`, la bozza «nuovo» sul PC della sessione, dalla prima casella della
   RFQ servita da quella postazione.
2. **Risposta.** Aggancio alla RFQ, se il messaggio non era deciso → legame con la richiesta → stato secondo l'atto.

### Invarianti

- Lo stato della richiesta cambia solo con l'atto confermato: una risposta qualunque non la chiude.
- Da una mail mandata a mano non nasce mai una RFQ nuova.
- Nessun invio dal Cockpit.

### Dipendenze

- `core/inbox/ingest` (`MarcatoreRichiesta`), `platform/{coda,contratti/worker,db}`.

### Test

- **L4:** `richieste_db_test.go` (IB2 marcatore, richiesta senza bozza, IB3–IB5, IB6, IB7).
- **L1:** `templates_test.go` (`TestLaRFQNonCreaRichiesteAiFornitori`).

### Stato dell'implementazione

- La rotta di creazione resta per lo storico.
- Il form nella pagina della RFQ è stato tolto di proposito: tornerà nella scheda delle lavorazioni, per lavorazione di un
  componente.

### Dove intervenire

- **capire perché una risposta non chiude la richiesta**
  - Apro: `richieste.go:rispostaFornitore`
- **cambiare oggetto o corpo della bozza**
  - Apro: `richieste.go:oggettoRichiesta`, `corpoRichiesta`

### Leggi anche

- `internal/README.md` (flusso «Richiesta a un fornitore»)

## Modulo Pagina Richieste

- `panoramica.go`
- `GET /richieste`: una card per RFQ con le schede dei prodotti, e i filtri nell'indirizzo.

### Scopo

- Elencare le RFQ, aperte per impostazione predefinita, con filtri e ordinamento.
- Per ogni RFQ mostrare fino a sei prodotti con l'anteprima del 2D, poi gli altri a richiesta.
- Aggiornare l'elenco con un poll che non ricarica niente se niente è cambiato.

### Non appartiene qui

- Le richieste ai fornitori: `richieste.go`.
- Il PDF dell'anteprima: `anteprima.go`.

### File

- **`panoramica.go`**
  - `filtriRichieste`
  - `leggiFiltriRichieste` (`q`, `cliente`, `fase`, `stato`, `bloccanti`, `smistare`, `sla`, `sort`, `n`)
  - `valori`/`URL`/`Altre`
  - `modelloRicerca` e `testoLetterale` (`%`, `_` e `\` presi alla lettera; `testoLetterale` serve anche alla ricerca di
    «Aggancia a…»)
  - `costruisciPanoramica`, `raggruppaProdotti`, `sceglieAnteprima`, `firmaPanoramica`, `caricaPanoramica`,
    `opzioniBarra`, `richieste`, `richiestaProdotti`
- **`web/templates/richieste.html`**
  - barra dei filtri, `richieste_elenco`, `richiesta_card`, `richiesta_prodotti`
  - nella card, «Distinta nuova (Motore A)» solo con `richiestaCard.DistintaNuova` (la guardia della rotta motore-a,
    `distintaNuovaVisibile`; giro 5, A1d, WP-U)

### Entry point

- `GET /richieste`, autenticato: pagina intera, oppure l'elenco quando cambia un filtro (con `HX-Push-Url`), oppure il
  poll con `firma`.
- `GET /richieste/{id}/prodotti?meno=1`, autenticato.

### Dati

- **Letture:**
  - `ListRichiestePanoramica`: `v_cruscotto`, `thread_offerta` e `cliente`, con il totale prima di LIMIT;
  - `ListProdottiPanoramica`: `identificativo_thread`;
  - `ListAnteprimePanoramica`: i 2D correnti con provenienza e le proposte 2D aperte;
  - per la barra `ListClientiTutti` e `ListFaseCatalogo`.
- **Scritture:** nessuna.

### Flussi principali

1. **Pagina.**
   - Tre letture, qualunque sia il numero di RFQ.
   - 30 card, poi «Mostra altre» a passi di 30, fino a 600.
   - Anteprime in iframe con caricamento lento.
2. **Poll.** Ogni 60 s, o ogni `PollRichieste`.
   - Con la stessa firma (sha256 di filtri, totale, card e prodotti) il server risponde 204 e il browser non tocca
     niente.

### Invarianti

- Lo stato della pagina sta nell'indirizzo.
- Il poll non ridisegna se l'elenco non è cambiato.
- L'anteprima di un prodotto è il documento confermato prima della proposta;
  - fra più candidati il più recente;
  - a parità l'allegato con l'identificativo minore.

### Dipendenze

- Solo `platform/db`.

### Test

- **L1:** `panoramica_test.go` (filtri dall'indirizzo, URL pulito, ricerca alla lettera, scelta dell'anteprima, schede
  di una card, firma, template).
- **L4:** `panoramica_db_test.go` (lista di lavoro, filtri, anteprime, sei schede e poi le altre, HTMX indirizzo e poll,
  «Mostra altre», 100 RFQ in meno di 1 s).
- **L7:** `richieste_browser_test.go` con `e2e/richieste.py`.

### Stato dell'implementazione

- Completo.
- L'ordinamento «priorità» è peso del cliente, poi scadenza: il punteggio dell'addendum 2 non esiste ancora.

### Dove intervenire

- **un filtro nuovo**
  - Apro: `panoramica.go:filtriRichieste` (lettura, `valori`, `parametri`), `panoramica.sql`, `richieste.html`
- **cambiare la scelta dell'anteprima**
  - Apro: `panoramica.go:sceglieAnteprima`
- **cambiare quante card o schede**
  - Apro: le costanti in testa a `panoramica.go`

### Leggi anche

- `platform/README.md` (query)
- `transport/README.md`

## Modulo Fascicolo: la schermata e le sue viste

- `fascicolo_pagina.go`, `fascicolo_rotte.go` per le GET, `fascicolo_bom.go`, `fascicolo_dettaglio.go`

### Scopo

- La schermata `/thread/{id}/fascicolo` ha tre viste:
  - Documenti (la predefinita, modulo seguente),
  - Struttura BOM (la BOM visuale a card),
  - Completezza (la matrice 3D/2D/DXF/STEP).
- Poi:
  - le viste di servizio Elenco file, Componenti e Albero,
  - il pannello di destra (file, componente o nodo proposto),
  - il piano in fondo,
  - i cassetti Da verificare, Rivedi, Importa dal NAS, Codici e Avvisi.
- Template: `web/templates/fascicolo.html`.

### Rotte

- Tutte `autenticato`; le GET le apre anche `consultazione`.
- **`GET /thread/{id}/fascicolo`**
  - Gestore: `fascicoloPagina`
  - Risposta: la pagina, o `fasc_corpo`. Non scrive niente, per nessun ruolo (Smistamento F1, R8): per chi scrive la
    pagina porta la POST della preparazione (`POST …/fascicolo/prepara`, `hx-trigger="load"`, `auto=1`), per
    `consultazione` no (e la stessa POST gli risponde 403)
- **`GET …/fascicolo/parti`**
  - Gestore: `fascicoloParti` → `rispondiFascicolo`
  - Risposta: `fasc_parti`, con `HX-Push-Url` dello stato
- **`GET …/fascicolo/anteprima`**
  - Gestore: `fascicoloAnteprima`
  - Risposta: `fasc_anteprima` (pannello di destra più il resto fuori banda)
- **`GET …/fascicolo/vista`**
  - Gestore: `fascicoloVista`
  - Risposta: `fasc_vista_frammento` (la ricerca: non rifà la testata)

### Lo stato nell'indirizzo

- `statoFascicolo`, `leggiStatoFascicolo`, `Con`
- **`nodo`, `prop`, `file`, `doc`**
  - Valori: gli id scelti
- **`scheda`**
  - Valori: `3d`, `2d`, `dxf`, `altri`, `storico`
- **`filtro`**
  - Valori: `nodo`, `candidati`, `tutti`, `da_scaricare`, `rumore`; vuoto = non assegnati
- **`tipo`**
  - Valori: un tipo di documento, per il filtro dei candidati
- **`vista`**
  - Valori: vuoto (Documenti), `bom`, `completezza`, `file`, `componenti`, `albero`; alias `documenti`→`file`,
    `griglia`→`completezza`
- **`gruppo`**
  - Valori: l'uuid di un prodotto, `fuori`, `file`, `rfq`
- **`cassetto`**
  - Valori: `verifica`, `piano`, `nas`, `codici`, `avvisi`
- **`scartate`**
  - Valori: `1`
- **`q`, `nas_cerca`**
  - Valori: al massimo 60 caratteri
- **`nas`**
  - Valori: la cartella aperta nel cassetto
- Un valore non valido cade sul predefinito.
- `Con("nodo", …)` toglie `tipo`, `doc`, `scheda` e `prop`.
- `dalFascicolo` rilegge lo stato da `HX-Current-URL`: è così che un gesto sa di venire da qui.

### Flusso

- `caricaFascicolo` legge tutta la schermata, in quest'ordine:
  1. thread, cliente, riga del cruscotto, fase;
  2. versioni della BOM, differenze e gate (`versioniFascicolo`);
  3. componenti e archi → `fascicolo.NuovoAlbero`;
  4. `v_fascicolo` → `cellaDa`; `v_step_prodotto`;
  5. documenti, proposte di struttura (`proposteFascicolo`: in linea sotto il padre, oppure nel blocco del file),
     rimozioni;
  6. file (`fileFascicolo`: allegato + proposta + documento; `os.Stat` di `path_staging`) → `filtraFile`;
  7. scheda del nodo; codici (solo con il cassetto `codici`); avvisi;
  8. `LeggiPianoFascicolo` e `LavoroInCorso`;
  9. per la vista `bom` `costruisciBom`, per `componenti` `componentiInCard`, per Documenti `vistaDocumenti`, altrimenti
     `pannelloDestro` + `chiaveCorpo`;
  10. il cassetto NAS, poi `avanzamento` con `firmaDi`.
- Un pezzo che non si legge (codici, piano, lavoro) diventa un avviso; una query di base fallita dà 404.

### Invarianti

- La risposta di una navigazione o di un gesto rifà i pannelli fuori banda;
  - il corpo del pannello di destra (`#anteprima-corpo`) solo se `corpo_chiave` è cambiato: il PDF aperto resta aperto.
- `fascicolo_bom.go` e `fascicolo_dettaglio.go` sono puri: nessuna query.

### Test

- **L1**
  - `fascicolo_test.go`,
  - `fascicolo_b87b_test.go` (`TestLoStatoNellIndirizzo`, `TestLaChiaveDelCorpoDiceQuandoRifarlo`,
    `TestLaBomVisualeDisegnaIlProdottoELeProposteSottoDiLui`).
- **L4**
  - `b87_db_test.go` (`TestLaSchermataDelFascicoloSiApreConTuttiIPannelli`,
    `TestUnGestoDalFascicoloRifaIPannelliSenzaToccareLAnteprima`, `TestCentoAllegatiSiDisegnanoInMenoDiUnSecondo`),
  - `b87b_db_test.go:TestAprireIlFascicoloLoPreparaPerChiLavora`.
- **L7** `e2e/fascicolo.py`, prove A, B, C, G, H.
- `GET …/vista` non ha un test HTTP.

### Stato dell'implementazione

- Ogni richiesta ricarica tutta la schermata (più di 20 query e un `os.Stat` per allegato; provato con cento allegati
  sotto il secondo).
- Nessun visore 3D: per STEP e DXF si mostra il riepilogo dell'analisi.
- Fino a B8.7b, per `consultazione` l'apertura rileggeva comunque gli STEP e poteva accodare analisi; dallo Smistamento
  F1 (R8) la GET non scrive per nessuno, e chi consulta non manda la preparazione (`TestNessunaGetScrive`,
  `TestAprireIlFascicoloLoPreparaPerChiLavora`).

### Dove intervenire

- **un parametro nuovo nell'indirizzo**
  - Apri: `statoFascicolo`, `leggiStatoFascicolo`, `valori`, `Con`
- **un dato nuovo nella schermata**
  - Apri: `fascicoloDati`, `caricaFascicolo`
- **una card della BOM**
  - Apri: `fascicolo_bom.go:riempiComponente`, `fasc_carta`
- **il pannello di destra**
  - Apri: `fascicolo_dettaglio.go`, `fasc_anteprima_testa`, `fasc_anteprima_corpo`

## Modulo Fascicolo: i gesti sulla BOM e sui documenti

- `fascicolo_rotte.go` per i POST, `fascicolo.go`, `fascicolo_conferma.go`

### Rotte

- Tutte POST, ruolo operatore o superiore (`autenticato` rifiuta i metodi che scrivono a `consultazione`).
- Percorso (sotto `/thread/{id}/fascicolo`):
  - **`/assegna`**
    - Campi: `componente` (vuoto = sgancia), `documento`… e `proposta`…, `correggi_codice`, `scelta_<doc>`/`scelta`,
      `nuovo_riferimento_<doc>`, `motivo_<doc>`; facoltativi, con una proposta sola, `codice_atteso` e
      `componente_atteso` (`attesaProposta`, revisione della notte 14/10: lo stato che la pagina ha mostrato; se la
      proposta bloccata non lo ha più, il rifiuto «la proposta è cambiata nel frattempo: rileggi la pagina»; assenti,
      come prima: il Fascicolo legacy non li manda)
    - Chiama: `assegnaAlComponente`, in una transazione sua, con la RFQ bloccata per prima
  - **`/componente/{cid}/codice`**
    - Campi: `codice`
    - Chiama: `correggiCodiceComponente` (`classificazione.CodiceAmmissibile`, RFQ bloccata per prima, FK differite al
      COMMIT)
  - **`/componente/{cid}/modifica` · `collega` · `scollega` · `sposta`**
    - Campi: `tipo` `rev` `descrizione` · `padre` `qta` · `padre` · `da` `a` `qta`
    - Chiama: `fascicolo.ModificaComponente`, `Collega`, `Scollega`, `Sposta` (`qta` letta a 32 bit da `qtaDal`: vuota
      vale 1, fuori intervallo è un rifiuto)
  - **`/componente/{cid}/archivia` · `rimuovi`**
    - Campi: `motivo`
    - Chiama: `ArchiviaComponente`, `RimuoviComponente`, poi `AggiornaTutteLeRimozioni`
  - **`/componente/{cid}/step-strutturale`**
    - Campi: `documento`
    - Chiama: `ScegliStepStrutturale`
  - **`/componente/{cid}/deroga`, `/deroga/{did}/revoca`**
    - Campi: `tipo`, `motivo`
    - Chiama: `ConcediDeroga`, `RevocaDeroga` (solo una deroga di questa RFQ)
  - **`/componente/{cid}/deroga-struttura`, `/deroga-struttura/{did}/revoca`**
    - Campi: `motivo`
    - Chiama: `ConcediDerogaStruttura`, `RevocaDerogaStruttura` (solo una deroga strutturale di questa RFQ)
  - **`/documento/{did}/sostituisci` · `annulla-sostituzione`**
    - Campi: `vecchio`, `nuovo_riferimento`, `motivo`
    - Chiama: `verificaSostituzione` + `Sostituisci` + `notaSostituzione` · `AnnullaSostituzione`
  - **`/revisione/apri` · `abbandona` · `/congela`**
    - Campi: `contesto`, `motivo`
    - Chiama: `ApriRevisione` (D25c), `AbbandonaBozza`, `CongelaBom`
  - **`/conferma`**
    - Campi: `firma` (tutto il pronto), oppure `voce`… e `strutturale`…
    - Chiama: `LeggiPianoFascicolo`, `selezioneDal`, `applicaPiano` (→ `confermaProposta`, `ScegliStepStrutturale`)
  - **`/proposta/{pid}/decidi`**
    - Campi: `tipo`, `codice`, `rev`
    - Chiama: `DecidiPropostaDocumento` (fonte operatore; codice e revisione con
      `CodiceAmmissibile`/`RevAmmissibile`)

### Flusso comune

- `s.gesto`/`gestoPoi` (`proposte.go`): `ParseForm`, una transazione, e sul rifiuto «Niente è cambiato: » +
  `spiegaErrore`, che traduce
  - `rifiuto` (lo stesso tipo di `fascicolo.Rifiuto`: un alias),
  - gli SQLSTATE da BOM01 a BOM05,
  - il deadlock 40P01 («un'altra operazione sulla stessa RFQ era in corso nello stesso momento: riprova»)
  - e i vincoli per nome.
- La risposta passa da `threadFrammento`:
  - dal Fascicolo (`HX-Current-URL`) è `rispondiFascicolo` (avviso + pannelli fuori banda),
  - da altrove la pagina della RFQ.
- `preparaGesto` blocca la RFQ (`BloccaThread`); il core fa lo stesso nei gesti che cambiano la struttura.

### Regole applicate (A1.4, A4.10, D18, D26, D32)

- Un codice o una revisione scritti a mano (correzione del codice, conferma di un file, decisione di una proposta) passano
  dalla stessa guardia dei nodi, dell'editor e del worker:
  - `classificazione.CodiceAmmissibile` (non vuoto, al massimo `MaxCodice` = 40 byte, senza spazi, anche Unicode, né
    caratteri di controllo)
  - e `RevAmmissibile` (`MaxRev` = 10).
- La RFQ si blocca prima di componenti, documenti e proposte (`preparaGesto` in `assegnaAlComponente` e
  `correggiCodiceComponente`):
  - è l'ordine del congelamento e degli altri gesti, e due gesti concorrenti si mettono in fila sulla RFQ invece di
    aspettarsi a vicenda.
- Il codice di un file segue quello del componente; un codice diverso passa solo con `correggi_codice=1`.
- Il percorso cambia solo per i documenti `in_coda` o in `errore`, con la copia bloccata (`BloccaCopiaPendente`):
  - copia in corso ⇒ «riprova fra poco»;
  - documento `scritto` in un'altra cartella ⇒ rifiuto fino a B8.8.
  - I lucchetti delle cartelle si prendono in ordine di percorso.
- Un file che entra in un componente con un documento corrente dello stesso tipo vuole «aggiungi» oppure il predecessore
  da sostituire;
  - se il predecessore è lo STEP strutturale serve la risposta sul riferimento;
  - una versione interna sostituisce solo con un motivo.
- Con la BOM congelata non si assegna.
  - Uno STEP registrato in una baseline, lo STEP strutturale attuale o un documento già sostituito non cambia componente
    (`restaAlSuoComponente`).
- «Conferma Fascicolo» senza elenco vale solo se la `firma` è quella del piano che l'operatore aveva davanti;
  - con un elenco, ogni voce deve essere ancora pronta.

### Test

- **L4**
  - `fascicolo_db_test.go`, `b85_db_test.go`,
  - `b87_db_test.go` (struttura, archivia/togli/ripristina, deroghe e STEP strutturale, sostituzioni, congelamento e
    revisioni),
  - `b87b_db_test.go` (`TestConfermaFascicoloPortaNelFascicoloSoloIlPianoVisto`,
    `TestDecidereUnFileNonSiFaRiscrivereDaUnaLettura`),
  - `a4_db_test.go`, `codice_tecnico_db_test.go`,
  - `gesti_db_test.go` (`TestIlCodiceScrittoAManoPassaDallaStessaGuardia`, `TestIGestiSulFascicoloBloccanoPrimaLaRFQ`).
- **L1**
  - `fascicolo_b87b_test.go`,
  - `gesti_test.go` (quantità a 32 bit, un rifiuto del core e del web è lo stesso, il deadlock dice «riprova»).
- **L7** `e2e/fascicolo.py`, prove D, E, F, I.

### Stato dell'implementazione

- Lo spostamento sul NAS di un documento già scritto è B8.8.
- Dal Fascicolo v3 una struttura di STEP non è mai «pronta» (si conferma nell'editor).

### Dove intervenire

- **un gesto nuovo sulla working**
  - Apri: la funzione nel core `fascicolo`, poi il gestore in `fascicolo_rotte.go` con `s.gesto`
- **le regole di assegnazione e del codice**
  - Apri: `fascicolo.go`
- **la conferma del piano**
  - Apri: `fascicolo_conferma.go`
- **la frase di un vincolo**
  - Apri: `spiegaErrore`

## Modulo Fascicolo: la vista Documenti (v3) e le note sui disegni

- `fascicolo_documenti.go`, `fascicolo_gesti_v3.go`
- Template `fasc_docv`, `fasc_doc_sezione`, `fasc_doc_file`; `web/static/fascicolo.mjs`.

### Rotte

- Metodo e percorso (sotto `/thread/{id}/fascicolo`):
  - **`GET /sezione`**
    - Ruolo: tutti
    - Che cosa fa: `fascicoloSezione`: il pannello dell'elemento scelto (`fasc_doc_sezione_frammento`), con lo stato
      dell'indirizzo e `vista` forzata a Documenti
  - **`POST /nota`**
    - Ruolo: operatore
    - Che cosa fa: campi `componente` (facoltativo, della RFQ), `allegato` (della RFQ, un PDF già scaricato con sha256),
      `pagina` (1–10000), `x` e `y` (0–1), `testo` (non vuoto, al massimo `MaxTestoNota` = 2000). Ammessa anche con la
      BOM congelata
  - **`POST /nota/{nid}/modifica` · `/elimina`**
    - Ruolo: solo l'autore
    - Che cosa fa: l'UPDATE o il DELETE filtrano per autore e RFQ: niente righe ⇒ rifiuto
  - **`POST /file/{pid}/generale`**
    - Ruolo: operatore
    - Che cosa fa: campo `tipo` (`altro` o `capitolato`): sgancia la proposta, se era agganciata, e la conferma senza
      componente (`confermaProposta`)

### Costruzione (`costruisciDocumenti`, pura)

- i gruppi: un prodotto per ogni radice di tipo finito, «Fuori dalla struttura», «Da associare», «Documenti della RFQ»;
- l'albero di ogni gruppo, ripercorso da capo: un sottoassieme condiviso si vede sotto ogni prodotto;
- il filmstrip (`tileDi`: il 2D confermato, poi quello in arrivo, poi «formato», poi «manca»);
- il JSON dell'indice per il visore (`indiceDoc`: elementi, note, revisione precedente, scelto, file, scrive, bloccata,
  gruppo);
- la sezione dell'elemento scelto (`fileDoc`: voce del piano, servibile, spostabile/`Fermo`, sostituibili);
- il riquadro degli STEP con una struttura da rivedere.
- Un documento arrivato più volte si mostra una volta.
- Le note stanno sul contenuto (sha256) di un file, cioè su quella revisione;
  - la revisione nuova dice quante note c'erano sulla precedente («Vedi»).
- Il disegno lo disegna `fascicolo.mjs` con pdf.js nello stage `#doc-stage` (`hx-preserve`);
  - scegliere un altro componente cambia l'indirizzo con `replaceState` e chiede solo `/sezione`.

### Dati

- `annotazione_pdf` (migrazione 0021): `InsertAnnotazione`, `ModificaAnnotazione`, `DeleteAnnotazione`,
  `ListAnnotazioniThread`; `AllegatoDelThread`.

### Test

- **L1** `documenti_v3_test.go`.
- **L4** `v3_db_test.go` (`TestLeNoteSuiDisegni`, `TestUnFileEntraSenzaComponente`, `TestLaSezioneEIDatiDellEditor`,
  `TestAssegnareIlFileAlPezzoConIlSuffisso`).
- **L7** `e2e/fascicolo_v3.py`, prove
  - J (visore, filmstrip, scelta senza ricaricare),
  - K (note, anche dopo F5),
  - M (associazione dal pannello).

### Stato dell'implementazione

- Un documento già scritto sul NAS non si sposta su un altro componente da qui (`spostabile`: B8.8).

### Dove intervenire

- **i gruppi, l'albero, le miniature, l'indice del visore**
  - Apri: `costruisciDocumenti`, `tileDi`
- **le regole delle note**
  - Apri: `fascicolo_gesti_v3.go` e i CHECK della migrazione 0021
- **il comportamento nel browser**
  - Apri: `fascicolo.mjs` (vista Documenti, note)

## Modulo Fascicolo: l'editor della struttura

- `fascicolo_editor.go`, `fascicolo_gesti_v3.go:applicaStruttura`
- Nel browser: `fascicolo.mjs`, `class Editor`.

### Rotte

- Metodo e percorso (sotto `/thread/{id}/fascicolo`):
  - **`GET /bom/dati?prodotto=<componente>&step=<allegato>`**
    - Ruolo: tutti
    - Che cosa fa: risponde **in JSON** (`Cache-Control: no-store`), una delle eccezioni all'HTML elencate in testa; la
      chiede `fascicolo.mjs` con `fetch`
  - **`POST /bom/applica`**
    - Ruolo: operatore
    - Che cosa fa: campo `struttura`: JSON di `fascicolo.StrutturaVoluta`, al massimo 2 MiB

### Dati dell'editor (`datiEditor`)

- **`nodi`**
  - Che cos'è:
    - `c:<componente>` per i componenti attivi;
    - `p:<proposta>` per le proposte aperte, unite per codice in una carta sola (senza codice: una carta per proposta);
    - `k:<codice>` per i codici nuovi trovati nella RFQ (`CandidatiDellaRfq`)
- **`archi`**
  - Che cos'è: gli archi working
- **`proposti`**
  - Che cos'è: gli archi proposti (allegato, chiavi `pk`/`fk`, file)
- **`rimozioni`**
  - Che cos'è: le rimozioni proposte
- **`prodotti`, `prodotto`**
  - Che cos'è: i prodotti finiti e quello di partenza: quello da cui si raggiunge la struttura dello `step` richiesto,
    altrimenti quello di una proposta qualsiasi, altrimenti il primo
- **`step`, `analisi`, `bloccata`, `scrive`**
  - Che cos'è: l'esito dello STEP del prodotto, le analisi in corso, la versione congelata, se chi guarda scrive

### La conferma

- `ApplicaStrutturaVoluta` blocca la RFQ e rifiuta con la BOM congelata;
  - si accorge dei cambiamenti avvenuti nel frattempo (`visti`, `relazioni_viste`).
- La risposta porta `HX-Trigger: bom-esito` con `{ok, testo}`, scritto tutto in ASCII da `eventoAscii` (`\uXXXX`, coppie
  surrogate comprese).
- L'editor si chiude solo se la conferma è riuscita.

### Test

- **L4** `v3_db_test.go`
  - `TestLEditorPrendeLaPropostaDelloStep` e le varianti su sposta/condividi/togli, rifiuto senza scrivere, codici
    trovati, scarti e rimozioni, cambiamenti nel frattempo, una carta per tutti gli STEP, nodo aperto con il codice di un
    componente;
  - `TestIDatiDellEditorSiApronoSulProdottoDelloStep`.
- **L7** `e2e/fascicolo_v3.py`, prove L e N; `e2e/fascicolo.py`, prova D.

### La bozza

- La bozza dell'editor vive nel `sessionStorage` della scheda, con la chiave `cockpit.editor.<utente>.<thread>.<prodotto>`:
  - l'utente lo dichiara la pagina (`data-utente` sulla radice `#fascicolo`),
  - e chi entra dopo sulla stessa scheda non trova la bozza di un altro.
- All'uscita (il form «esci» della testata) `layout.html` cancella tutte le chiavi `cockpit.editor.*`.
- Una bozza si riprende solo sulla BOM con la stessa impronta.
- Prova: `statici_test.go:TestLaBozzaDellEditorEDiChiLaScrive` (L1).

### Stato dell'implementazione

- Il limite di 40 caratteri del codice è ripetuto nel JS (`Editor.scriviCodice`, che conta i caratteri JavaScript; il
  server, `CodiceAmmissibile`, conta i byte).
- Una sessione che scade senza passare da «esci» lascia la bozza nella scheda, sotto la chiave del suo utente.

### Dove intervenire

- **i dati che l'editor riceve**
  - Apri: `datiEditor`
- **le regole della conferma**
  - Apri: `core/rfq/fascicolo/voluta.go`
- **i gesti nell'editor**
  - Apri: `fascicolo.mjs`, `class Editor`

## Modulo Fascicolo: preparazione e avanzamento

- `fascicolo_preparazione.go`

### Scopo

- Aprire il Fascicolo (o la pagina della RFQ, o la Distinta) lo fa preparare a chi scrive: la GET non scrive niente
  (Smistamento F1, R8), e la pagina manda da sola la POST della preparazione;
- finché c'è lavoro sui file, la testata lo dice e la pagina si aggiorna da sola.

### Flusso

- `prepara` (`POST /thread/{id}/fascicolo/prepara`): la pagina la manda al caricamento solo a chi scrive
  (`hx-trigger="load"`, `auto=1`), con il bottone «Prepara i file» di riserva; chi consulta riceve 403.
  - `preparaFile`, in una transazione sola (tutto o niente):
    - `fascicolo.PreparaFile` (con `Analizzatore`, `maxCaricamentoEffettivo()`, `MaxPreparatiPerApertura` = 20 e
      `rileggiFatti` = `Pipeline.DopoCaricamento`);
    - poi, con quello che resta del limite, `fascicolo.AccodaAnalisiMancantiDaSola`: gli STEP senza i fatti
      dell'analizzatore corrente, senza riprovare da sola un'analisi fallita.
  - Accoda e basta: non fa i prodotti della richiesta (li fa il triage), non rilegge gli STEP già analizzati e non
    riaccoda gli STEP da rileggere né i PDF (lo fa solo «Rianalizza», `fascicolo.RianalizzaRfq`).
  - Risposte: 204 per quella automatica che non ha messo in moto niente, non è riuscita o viene dalla pagina della RFQ;
    con htmx l'avviso e i pannelli; senza htmx un 303, o la pagina intera con l'avviso se non è riuscita
    (`preparazioneNonRiuscita`).
- `triage.go:preparaDopoLaDecisione` usa le stesse costanti e `fascicolo.PreparaFile` (senza rilettura dei fatti), con
  `fascicolo.AssicuraProdottiDellaRichiesta` e `fascicolo.RileggiStepDellaRfq`: è, con «Rianalizza», la sola strada
  che rilegge gli STEP già analizzati.
- `GET …/fascicolo/avanzamento?firma=…&giro=…` (tutti; non scrive):
  - lo stato viene da `HX-Current-URL`;
  - `firmaDi` riassume componenti, proposte, versione bloccata, file, completezza, piano e lavoro;
  - con la stessa firma il giro cresce e l'attesa si allunga (`intervalliPoll`: 2, 2, 3, 3, 5, 5, 8, 10 s);
  - con una firma diversa rifà testata, vista e piano, e il cassetto salvo quello del NAS;
    - il corpo del pannello solo se `corpo_chiave` è cambiato;
  - senza lavoro l'elemento torna senza poll.

### Test

- **L1** `fascicolo_b87b_test.go` (il poll finché c'è lavoro, la firma della schermata).
- **L4** `b87b_db_test.go`
  - `TestAprireIlFascicoloLoPreparaPerChiLavora`,
  - `TestIlPollRifaIPannelliSoloSeQualcosaECambiato`,
  - `TestCreareLaRfqFaDelCodiceUnProdottoEPreparaLoZip`,
  - `TestAgganciareUnMessaggioPreparaIFileEAssicuraIProdotti`.
- **L7** `e2e/fascicolo.py`, prova A.

### Stato dell'implementazione

- La GET della pagina non scrive, per nessun ruolo (Smistamento F1, R8): la preparazione è la POST che la pagina manda
  da sola solo a chi scrive (`auto=1`), con il bottone «Prepara i file» di riserva.
- Ogni poll ricarica tutta la schermata.

### Dove intervenire

- **che cosa si prepara**
  - Apri: `core/rfq/fascicolo/preparazione.go`
- **la cadenza del poll**
  - Apri: `intervalliPoll`
- **che cosa fa cambiare la firma**
  - Apri: `firmaDi`

## Modulo Fascicolo: «Importa dal NAS»

- `fascicolo_nas.go`
- Template `fasc_nas`, `fasc_nas_voce`.

### Rotte

- Metodo e percorso (sotto `/thread/{id}/fascicolo`):
  - **`GET /nas?nas=<cartella>&nas_cerca=<testo>`**
    - Ruolo: operatore (controllo esplicito: 403 per `consultazione`)
    - Che cosa fa: rifà solo il cassetto (`fasc_cassetto_frammento`)
  - **`POST /nas/importa`**
    - Ruolo: operatore
    - Che cosa fa: campo `percorso`, relativo alla radice

### Flusso

- `percorsoNas` accetta solo percorsi relativi (niente assoluti o di unità, niente `..` che esce, `fs.ValidPath`);
  - poi `os.OpenRoot([nas].radice)` rifiuta anche i collegamenti che escono dalla radice.
- Si parte dalla cartella della RFQ se c'è già sul NAS, altrimenti da quella del cliente, altrimenti dalla radice.
- L'elenco mostra al massimo 500 voci, senza i nomi che cominciano per `.` o `~$`, prima le cartelle.
- La ricerca parte dalla cartella del cliente e vuole almeno 3 caratteri; si ferma a 20000 voci guardate, 100 risultati o
  3 s.
- Si importa un file regolare, non vuoto, entro `maxCaricamentoEffettivo`, e `tecnicoCaricabile` (CAD 3D, disegni, DXF,
  PDF, TIF):
  - `nelloStagingCaricato`,
  - poi in una transazione `fascicolo.NotaInterna` → `RegistraCaricamento` (con `PathInterno` = «NAS: <percorso>») →
    `Pipeline.DopoCaricamento`.
- Sul NAS della RFQ il file va solo con la conferma.

### Test

- **L1** `fascicolo_b87b_test.go:TestIPercorsiDelNasStannoSottoLaRadice`.
- **L4** `b87b_db_test.go:TestImportaDalNasRestaSottoLaRadice`.
- L'elenco e la ricerca via HTTP non sono provati.

### Stato dell'implementazione

- Dopo una ricerca i collegamenti del cassetto perdono lo stato della pagina (`vista`, `filtro`…): il form manda solo
  `nas` e `nas_cerca`.
- Le risposte di `/nas` mostrano «Da verificare (0)» e «Rivedi (0)» perché il piano non viene calcolato.
- Il NAS si legge direttamente con `os`, non con `platform/storage/nas`.

### Dove intervenire

- **i limiti di elenco e ricerca**
  - Apri: le costanti in testa a `fascicolo_nas.go`
- **i tipi importabili**
  - Apri: `caricamento.go:tecnicoCaricabile`

## Modulo Allegati, anteprima PDF, caricamento interno

- `allegati.go`, `anteprima.go`, `caricamento.go`
- Template `frammenti.html` (`allegati_tabella`, `allegato_riga`, `conferma_form`), `fasc_file_testa`,
  `fasc_anteprima_corpo`.

### Rotte

- Tutte `autenticato`; i POST vogliono il ruolo operatore o superiore.
- **`POST /messaggio/{id}/scarica`**
  - Campi: `allegato_id`…
  - Effetto: solo un messaggio agganciato a una RFQ; in una transazione `accodaDownload` → `coda.AccodaStage` (la guardia
    del doppio download sta lì); la copia da cui scaricare la sceglie `coda.CopiaPerDownload`
- **`POST /allegato/{id}/riscarica`**
  - Campi: `ritorna_thread`
  - Effetto: lo stesso, per un allegato solo
- **`POST /proposta/{id}/conferma`**
  - Campi: `tipo`, `codice`, `rev`, `nota`, `componente_id`, `correggi_codice`, `scelta`, `nuovo_riferimento`, `motivo`,
    `ritorna_thread`
  - Effetto: la RFQ del messaggio `FOR KEY SHARE`, poi `BloccaProposta` + `confermaProposta`
- **`POST /proposta/{id}/scarta`**
  - Campi: `rumore`, `ritorna_thread`
  - Effetto: la proposta scartata e, con `rumore=1`, lo sha256 ricordato per il dominio del mittente, nella stessa
    transazione
- **`GET /allegato/{id}/anteprima`**
  - Campi: —
  - Effetto: i byte del PDF (tutti i ruoli); 400 a una richiesta htmx
- **`POST /thread/{id}/fascicolo/carica`**
  - Campi: multipart `file`
  - Effetto: la versione interna

### `confermaProposta`

- La chiamano `conferma`, `applicaPiano` e `fileGenerale`.

1. Proposta aperta, tipo diverso da `da_determinare`, codice e revisione scritti ammissibili
   (`classificazione.CodiceAmmissibile`, `RevAmmissibile`), messaggio agganciato, sha256 e staging presenti, RFQ con una
   cartella.
2. Contenuto identico a un documento della RFQ: si registra solo la provenienza e la proposta diventa `duplicato` (un
   errore in una delle due scritture annulla il gesto);
   - se quel documento è stato sostituito, rifiuto (`identicoAUnaRevisioneSostituita`).
3. Il componente deve essere della stessa RFQ; il codice è quello del componente (o `correggi_codice`);
   - con la BOM congelata si rifiuta;
   - con un corrente dello stesso tipo vale `verificaScelta`.
   - Un file tecnico (`documenti.Tecnico`) senza codice si rifiuta.
4. Il percorso: `documenti.CartellaDocumento` + `ScegliPercorso(NomeSulNas…)`.
5. `InsertDocumento` (in `nome_file` il nome originale), la provenienza, la proposta confermata, `applicaScelta`,
   `coda.AccodaCopia`.
   - Con `nas_scrittura` spenta il documento resta `in_coda` e la frase dice «IN ATTESA».

- `pannelloConAvviso` risponde con `HX-Trigger: inbox-aggiorna`;
  - con `ritorna_thread` risponde come la pagina della RFQ (o come il Fascicolo, se il gesto viene da lì).
- L'ordine dei lucchetti in `conferma`:
  - prima la RFQ del messaggio dell'allegato (`FOR KEY SHARE`, lo stesso lucchetto che la scrittura del documento
    prenderebbe dopo),
  - poi la proposta.
  - Così una conferma aspetta un congelamento o un gesto sulla struttura senza tenere già la proposta;
  - due conferme dello stesso file restano una corsa che decide il vincolo (thread, sha256).

### Anteprima

- Una richiesta htmx (`HX-Request` con qualunque valore) si rifiuta con 400 prima di leggere l'allegato:
  - i byte di un file si aprono solo come documento (l'iframe, pdf.js, una scheda nuova), mai fra i pezzi della pagina.
- Nell'URL c'è solo l'id; si serve solo ciò che comincia per `%PDF-`.
- Prima lo staging, se `path_staging` è assoluto e sta sotto `Server.Staging`;
- poi il NAS: il documento `scritto` con lo stesso sha256 nella RFQ
  - anomalia aperta ⇒ 409;
  - NAS che non risponde ⇒ 503;
  - file mancante, illeggibile o cartella ⇒ `documenti.Segnala` e 409;
  - dimensione diversa o file toccato dopo la scrittura ⇒ una rilettura con `VerificaFileAperto`.
- **Intestazioni:**
  - `application/pdf`;
  - `Content-Disposition: inline` con `filename=` ASCII e `filename*=UTF-8''…`;
  - `nosniff`;
  - `Content-Security-Policy: sandbox; default-src 'none'`;
  - `ETag` = sha256;
  - `http.ServeContent` (Range, If-Range).

### Caricamento interno

- Servono `Pipeline` e `Staging`; limite `MaxBytesReader` pari al massimo + 1 MiB; `tecnicoCaricabile`.
- `nelloStagingCaricato`:
  - una `.parte` con un token,
  - lo sha256 calcolato mentre si scrive,
  - il rifiuto di un file vuoto o troppo grande,
  - poi `Promuovi` in `_contenuti` (o si butta la parte se il contenuto c'è già).
- Poi, in una transazione: `NotaInterna` (il messaggio del canale `nota`, uno per RFQ) → `RegistraCaricamento` (allegato
  con origine `manuale`) → `Pipeline.DopoCaricamento`.
- Ammesso anche con la BOM congelata; diventa un documento solo con la conferma.

### Test

- **L1** `anteprima_test.go` (anche `TestLAnteprimaNonSiInnestaNellaPagina`).
- **L4**
  - `anteprima_db_test.go`, `a4_db_test.go`, `codice_tecnico_db_test.go`,
  - `decisioni_db_test.go:TestDueConfermeConcorrentiDecidonoUnaSolaVolta`,
  - `chiavecopia_db_test.go`, `fascicolo_db_test.go`,
  - `b85_db_test.go:TestLaConfermaConComponenteVuoleLaScelta`,
  - `b87_db_test.go` (versioni interne),
  - `sicurezza_db_test.go:TestLAnteprimaRifiutaLeRichiesteHtmx`,
  - `gesti_db_test.go` (codice della conferma, ordine dei lucchetti).
- **L7** `e2e/anteprima_pdf.py`.
- `riscarica` non ha un test HTTP.

### Stato dell'implementazione

- Il ritorno a una revisione più vecchia della precedente non è gestito.
- La GET dell'anteprima può scrivere anomalie.

### Dove intervenire

- **che cosa succede quando si conferma**
  - Apri: `allegati.go:confermaProposta`
- **da dove si servono i byte di un PDF**
  - Apri: `anteprima.go:sorgenteAnteprima`
- **che cosa si può caricare a mano**
  - Apri: `caricamento.go`

## Le prove nel browser (`e2e/`)

- **`inbox_quadranti.py`**
  - Lanciato da: `inbox_browser_test.go`
  - Prove: l'Inbox a quadranti (A–G)
- **`richieste.py`**
  - Lanciato da: `richieste_browser_test.go`
  - Prove: la pagina Richieste (A–F)
- **`anteprima_pdf.py`**
  - Lanciato da: `anteprima_browser_test.go`
  - Prove: a–g: il pulsante, la scheda che si apre, il Range servito a pezzi, il secondo giro senza byte
- **`fascicolo.py`**
  - Lanciato da: `fascicolo_browser_test.go`
  - Prove: A–I: Struttura BOM, dettaglio, PDF, nodo ed editor senza F5, tre file con un gesto, completezza, cento file,
    cassetti, Rivedi e Conferma
- **`fascicolo_v3.py`**
  - Lanciato da: `fascicolo_v3_browser_test.go`
  - Prove: J–N: vista Documenti con pdf.js, note, editor, associazione dal pannello, editor senza STEP; fallisce anche su
    un errore JavaScript o su una risposta ≥ 400
- Si lanciano con `go test -tags "integrazione browser" -count=1 -run TestL7 ./internal/transport/web/`, con
  `COCKPIT_TEST_DSN`.
- Serve Playwright per Python con Edge; senza Python o Playwright il test è SKIP.
- Il test Go sceglie le prove con `--prove`; con `COCKPIT_FOTO_DIR` lo script salva le fotografie della pagina.

## Leggi anche

- `internal/transport/README.md` (le rotte in tabella)
- `web/README.md` (template e statici)
- `internal/core/README.md` (chi fa che cosa dietro le rotte)
- `internal/core/inbox/lettura/README.md` (il corpo di una mail pronto da leggere)
- `internal/README.md` (livelli di test)
