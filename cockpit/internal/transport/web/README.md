# `internal/transport/web` — le pagine dell'operatore

Il livello HTTP del browser: legge la richiesta, controlla chi chiama, chiama il pezzo giusto di `core` e rende HTML
(pagina intera o frammento htmx). Circa 36 file e 14 mila righe: qui sono divisi in **moduli**, cioè gruppi di file che
servono la stessa schermata. Ogni modulo ha la sua sezione con rotte, dati, prove e limiti.

## Regole che valgono per tutto il package
- **Una sola autenticazione:** il cookie di sessione (`autenticato`). Le rotte `/admin/*` passano da `soloAdmin`;
  `consultazione` non usa mai un metodo che scrive (la regola è per metodo, in un punto solo).
- **Protezione cross-site** (`ProtezioneCSRF`, cioè `http.CrossOriginProtection`) su tutto il mux, rotte dei worker comprese.
- **HTML, salvo le eccezioni dichiarate.** Rispondono con altro:
  - `GET /healthz`: JSON `worker.Salute` (`server.go:healthz`);
  - `GET /thread/{id}/fascicolo/bom/dati`: JSON con `Cache-Control: no-store` (`fascicolo_editor.go:fascicoloDatiEditor`),
    chiesto da `fascicolo.mjs` con `fetch`, mai da htmx;
  - `GET /allegato/{id}/anteprima`: i byte del PDF (`anteprima.go:anteprima`), che si aprono solo come documento (iframe,
    pdf.js, scheda nuova);
  - `POST /admin/postazioni/{host}/pacchetto`: lo zip del pacchetto, in download (`postazioni_admin.go:pacchettoWorker`);
  - `GET /static/`: CSS, JavaScript, wasm, font e dati di pdf.js (`server.go:statici`);
  - risposte senza corpo: 204 al poll di `/richieste` con la stessa firma (`panoramica.go:richieste`) e a
    `/inbox/sync-storico/stato` senza job storico (`routes_inbox.go:syncStoricoStato`); 204 con `HX-Refresh` dopo la scelta
    della postazione (`postazione.go:dopoScelta`) e dopo «riaccoda»/«annulla» di un job (`routes_admin.go:riaccodaJob`,
    `annullaJob`); 401 con `HX-Redirect` per una sessione scaduta chiesta da htmx (`server.go:aLogin`, da `autenticato`);
  - gli errori tecnici (400, 404, 500) escono da `http.Error`, in testo semplice.
- **Pagina o frammento:** `rendi` sceglie il layout completo o il frammento a seconda di `HX-Request`; molte rotte di
  navigazione del Fascicolo esistono solo come frammento, ma i collegamenti portano l'`href` della pagina intera.
- **Mai HTML della mail nella pagina:** il corpo di un messaggio passa da `core/inbox/lettura.Presenta` e arriva ai template
  come testo e tabelle estratte (`frammenti.html:corpo_messaggio`); l'oggetto passa da `oggettoVisibile`. I template non usano
  `template.HTML`: tutto passa dall'escape di `html/template`. Un indirizzo scritto nella mail non diventa mai un collegamento.
- **Nella pagina torna solo ciò che il server riconosce:** `sel` dell'Inbox e della pagina RFQ entra solo se è un UUID, riscritto
  nella forma canonica (`routes_inbox.go:inbox`, `thread.go:thread`); lo `stato` della coda job e l'`origine` degli scarti tornano
  nel poll solo se sono valori noti (`routes_admin.go:adminJob`, `adminScarti`).
- **Nessuna risposta non HTML entra fra i pezzi della pagina:** `layout.html` ha un ascoltatore globale `htmx:beforeSwap` che non
  innesta una risposta 2xx con un corpo il cui `Content-Type` non comincia per `text/html`; `GET /allegato/{id}/anteprima`
  risponde 400 a una richiesta htmx (`HX-Request`).
- **Una transazione per gesto:** `gesto`/`gestoPoi` (`proposte.go`), oppure `Begin`/`defer Rollback`/`Commit` (o
  `pgx.BeginFunc`) nel gestore.
- **La RFQ si blocca per prima:** i gesti sul Fascicolo prendono il lucchetto della RFQ prima di componenti, documenti e
  proposte (`fascicolo_rotte.go:preparaGesto` in `fascicolo.go:assegnaAlComponente` e `correggiCodiceComponente`; in
  `allegati.go:conferma` un `FOR KEY SHARE` sulla RFQ del messaggio prima di `BloccaProposta`). Un deadlock che capita lo stesso
  (SQLSTATE 40P01) arriva all'operatore come «riprova» (`fascicolo.go:spiegaErrore`).
- **Logica di dominio ancora qui** (da portare in `core`, decisione dell'architetto): la conferma di una proposta
  (`allegati.go:confermaProposta`), l'assegnazione e la correzione del codice (`fascicolo.go`), l'applicazione del piano
  (`fascicolo_conferma.go:applicaPiano`), la nuova RFQ e gli agganci (`triage.go`), il nome libero della cartella NAS di una
  RFQ nuova (`triage.go:cartellaLibera`), le scritture dell'anagrafica (`anagrafica.go`, `censisci.go`), le finestre dello
  storico e la loro partenza per cartella (`routes_inbox.go:finestraStorico`, `partenzaDellaCartella`,
  `nomiCartellaPredefinita`). SQL scritto a mano, fuori da sqlc: `triage.go:cartellaLibera`,
  `routes_inbox.go:partenzaDellaCartella`, `allegati.go:conferma` (il lucchetto sulla RFQ),
  `fascicolo.go:correggiCodiceComponente` (`SET CONSTRAINTS … DEFERRED`), `server.go:healthz`.

## Moduli

| Modulo | File | Schermata |
|---|---|---|
| Server e ruoli | `server.go`, `ruoli.go`, `navigazione.go` | guscio, login, divieto, statici |
| Inbox | `routes_inbox.go`, `inbox_viva.go`, `agente.go` | l'Inbox a quadranti e la testata |
| Triage | `triage.go` | Nuova RFQ, Aggancia, Ignora |
| Anagrafica e censimento | `censisci.go`, `anagrafica.go`, `anagrafica_admin.go`, `convenzioni_admin.go`, `fornitori_admin.go` | «Da validare», Admin › Anagrafica e Fornitori |
| Postazioni | `postazione.go`, `postazioni_admin.go` | la postazione della sessione, i pacchetti dei worker |
| Admin tecnico | `routes_admin.go`, `integrita_admin.go` | coda dei job, scarti, integrità del NAS |
| Pagina RFQ | `routes_rfq.go`, `thread.go`, `proposte.go`, `codici.go` | `/thread/{id}` e i gesti B8.5/B8.6 |
| Richieste ai fornitori (7B) | `richieste.go` | bozza marcata, risposta del fornitore |
| Pagina Richieste | `panoramica.go` | `/richieste`: una card per RFQ |
| Fascicolo: schermata e viste | `fascicolo_pagina.go`, `fascicolo_rotte.go`, `fascicolo_bom.go`, `fascicolo_dettaglio.go` | `/thread/{id}/fascicolo` |
| Fascicolo: gesti | `fascicolo_rotte.go`, `fascicolo.go`, `fascicolo_conferma.go` | assegna, codice, struttura, revisioni, conferma |
| Fascicolo: vista Documenti e note (v3) | `fascicolo_documenti.go`, `fascicolo_gesti_v3.go` | pdf.js, filmstrip, note sui disegni |
| Fascicolo: editor della struttura | `fascicolo_editor.go`, `fascicolo_gesti_v3.go` | l'editor a nodi |
| Fascicolo: preparazione e avanzamento | `fascicolo_preparazione.go` | preparazione all'apertura e poll |
| Fascicolo: Importa dal NAS | `fascicolo_nas.go` | il cassetto NAS |
| Allegati, anteprima, caricamento | `allegati.go`, `anteprima.go`, `caricamento.go` | download, conferma, PDF, caricamento interno |

Il front-end (template, CSS, `fascicolo.mjs`, pdf.js) è descritto in `web/README.md`.

## Modulo Server e ruoli (`server.go`, `ruoli.go`, `navigazione.go`)

L'infrastruttura comune del package: il tipo `Server`, i template, il montaggio delle rotte, la sessione, i ruoli e la rail.

### Scopo
- Caricare i template una volta (`Init`) e rendere una vista come pagina intera o come frammento (`rendi`, `frammento`, `frammentoRichiesto`).
- Riconoscere l'utente dal cookie (`autenticato`), riabbinare la sessione alla postazione e negare ciò che il ruolo non consente (`soloRuolo`, `soloAdmin`, `nega`).
- Servire gli statici con l'impronta del contenuto (`statico`, `statici`; solo file, mai l'elenco di una cartella), la salute
  del server, il login e il logout.

### Non appartiene qui
Le rotte delle aree: ognuna ha il suo `registra*` nel file dei suoi gestori. Qui restano solo `/static/`, `/healthz`, `/login`, `/logout` e la radice.

### File
| File | Responsabilità |
|---|---|
| `server.go` | `Server` (configurazione iniettata da `app/runtime`), `funzioni` (FuncMap dei template, con `oggettoVisibile`), `Init`, `ProtezioneCSRF`, `Registra`, `statici`/`statico`, `vista` (con `Serve`, il ruolo che il divieto dice che basterebbe), `rendi`, `frammento`, `healthz`, `autenticato`, `riabbinaPostazione`, `login`, `logout`, `avviso`, `avvisoErrore` |
| `ruoli.go` | `rango` (consultazione < operatore < tecnico < admin; un ruolo sconosciuto vale 0), `almeno`, `soloRuolo`, `soloAdmin`, `metodoCheScrive`, `nega` (il 403 in due forme; dice il ruolo che servirebbe, o nessuno quando il divieto non dipende dal ruolo) |
| `navigazione.go` | `navPer`: la rail per utente (Inbox, Richieste; per l'admin la sezione Admin), `vista.Nav` |
| `web/templates/layout.html`, `login.html`, `vietato.html` | guscio con testata e rail (più la guardia `htmx:beforeSwap` e la pulizia delle bozze dell'editor all'uscita), form di accesso, pagina e frammento del divieto |

### Entry point
- `Server.Init` e `Server.Registra`: li chiama `app/runtime/ascolto.go:Ascolta`.
- `ProtezioneCSRF`: sempre in `Ascolta`, che avvolge tutto il mux, rotte dei worker comprese (i worker non mandano `Sec-Fetch-Site` e passano).
- `Server.URLServer`: la pagina Postazioni e il log di avvio.

| Metodo e percorso | Ruolo | Gestore |
|---|---|---|
| `GET /static/` | nessuno | `statici`: solo file (una cartella risponde 404, niente elenco); `Cache-Control: immutable` per `pdfjs-*` e per `?v=`; tipi MIME decisi dal server |
| `GET /healthz` | nessuno | `healthz` (JSON `worker.Salute`: DB, versione dello schema, NAS) |
| `GET /login`, `POST /login` | nessuno | `loginForm`, `login` |
| `POST /logout` | nessuno | `logout` |
| `GET /` | autenticato | reindirizza a `/inbox` |

### Dati
- `utente`: `GetUtentePerSigla`, solo utenti attivi; la password si confronta con bcrypt.
- `sessione`: `CreaSessioneConPostazione` con un token casuale di 32 byte e scadenza 12 h; `GetSessione` scarta le sessioni scadute e gli utenti disattivati; `ToccaSessione` a ogni richiesta autenticata; `SetSessionePostazione` nel riabbinamento; `EliminaSessione` al logout.
- Cookie `cockpit_sess`: `HttpOnly`, `SameSite=Lax`, `Secure` solo se il server parla TLS, durata 12 h.
- `schema_versione`: letta da `healthz`.

### Flussi principali
1. **Login.** Sigla in maiuscolo → bcrypt → abbinamento per IP (`abbinaPostazione`) → la sessione nasce con origine `ip` o senza postazione → cookie → `/inbox`.
2. **Ogni richiesta autenticata.**
   - Cookie → `GetSessione`. Se manca o è scaduta: redirect a `/login`, oppure `HX-Redirect` con 401 se la richiesta viene da HTMX.
   - Poi `ToccaSessione` e `riabbinaPostazione`, solo per le sessioni senza origine.
   - Utente e sessione vanno nel contesto.
   - Un metodo che scrive con ruolo `consultazione` → `nega`.
3. **Rendering.** `rendi` sceglie `layout` (pagina intera, con testata `stato` e rail) oppure il frammento indicato, quando la richiesta è HTMX e non un ritorno dalla cronologia (`HX-History-Restore-Request`).
4. **Divieto.** `nega` scrive il 403 come pagina o come frammento `vietato`, e lo registra nel log. Il ruolo che la pagina dice
   che servirebbe arriva da chi nega: admin da `soloRuolo`/`soloAdmin`, operatore da `autenticato` (un metodo che scrive) e da
   `fascicolo_nas.go:fascicoloNas`, nessuno da `postazione.go:scegliPostazione` (il motivo è la postazione, non il ruolo).

### Invarianti
- Ogni rotta `/admin/*` è montata con `soloAdmin` (autenticazione, poi ruolo). La rail chiama lo stesso `almeno` del wrapper.
- `consultazione` non scrive: la regola è per metodo, in `autenticato`, per tutte le rotte autenticate.
- La protezione cross-site vale per tutto ciò che il listener serve.
- `Secure` sul cookie solo con TLS; `SameSite=Lax` sempre.
- Ogni pagina carica `style.css`, `htmx.min.js` (e il Fascicolo `fascicolo.mjs`) con l'impronta del contenuto (`{{statico}}`):
  un binario nuovo porta file nuovi anche con la cache `immutable`.
- `/static/` serve file, non cartelle: la radice e le cartelle (anche `pdfjs-*/`) rispondono 404.

### Dipendenze
Importa `platform/db`, `platform/rete`, `platform/storage/nas`, `platform/coda`, `platform/contratti/worker`, `core/inbox/ingest`, `core/inbox/lettura` (`oggettoVisibile`), `core/rfq/{documenti,fascicolo}`, `ai/agente` (tipi dei campi di `Server`). Lo importa solo `app/runtime`.

### Test
- **L1**
  - `ruoli_test.go`: ordine dei ruoli, metodi che scrivono, rail per ruolo, pagina del divieto (`TestIlDivietoDiceIlRuoloCheServeDavvero`: operatore per chi consulta, nessun ruolo per una postazione).
  - `web_test.go`, `templates_test.go`: i template si caricano e i frammenti si eseguono.
  - `statici_test.go`: tipi MIME e cache, copia di pdf.js; `TestLeCartelleStaticheNonSiElencano`; `TestIlLayoutCaricaGliStaticiConLImpronta` (anche la guardia `htmx:beforeSwap`); `TestLaBozzaDellEditorEDiChiLaScrive`.
  - `stile_test.go`: `[hidden]` nasconde davvero.
- **L4**
  - `rbac_db_test.go`: operatore fuori dall'Admin, admin ovunque, testata dal vivo, consultazione in sola lettura, password dopo il riavvio, seme con ruolo sconosciuto.
  - `rete_db_test.go`: POST da un altro sito respinto, worker ammesso.

### Stato dell'implementazione
- Completo.
- Il ruolo `tecnico` oggi può esattamente ciò che può `operatore`: nessuna rotta usa `soloRuolo` con un minimo diverso da admin. La matrice ruolo × azione è rimandata di proposito (commento in `ruoli.go`).
- Il frammento del divieto chiesto da htmx non si vede: `nega` risponde 403, e htmx 2.0.4 con la configurazione predefinita
  (`responseHandling`) non innesta le risposte 4xx; nessun template la cambia. Solo i gesti mandati da `fascicolo.mjs` (note,
  conferma dell'editor: la funzione `gesto()`) leggono il 403 e lo dicono con una frase loro.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| aggiungere una pagina al package | `Init` (elenco delle pagine), il `registra*` dell'area, il template |
| una voce nuova nella rail | `navigazione.go:navPer` |
| cambiare chi può fare che cosa | `ruoli.go` (`rango`, `soloRuolo`) |
| una funzione nuova nei template | `server.go:funzioni` |
| cambiare cookie o durata della sessione | `server.go:login` |
| cambiare che cosa può entrare fra i pezzi della pagina | `layout.html` (l'ascoltatore `htmx:beforeSwap`) |
| cambiare la pagina del divieto | `ruoli.go:nega`, `vietato.html` |

### Leggi anche
`transport/README.md`, `internal/README.md` (livelli di test), `app/README.md` (`Ascolta`).

---

## Modulo Inbox (`routes_inbox.go`, `inbox_viva.go`, `agente.go`)

La schermata A: quadranti, sync ordinario e storico, pannello del messaggio, azioni su Outlook e testata viva.

### Scopo
- Mostrare la posta per quadrante (dalla colonna `quadrante` di `v_inbox`), direzione, filtro e casella.
- Mostrare il messaggio scelto da leggere: l'oggetto senza le etichette di posta esterna, il corpo come testo con le tabelle
  estratte, il testo automatico di Outlook chiuso e la storia citata a parte (`core/inbox/lettura`).
- Tenere la testata aggiornata: postazione, stato delle caselle, novità.
- Accodare i job interattivi sulla copia servita dalla postazione della sessione.
- Accodare il sync: «Aggiorna ora», prima apertura della sessione, «Carica precedenti».

### Non appartiene qui
- Le decisioni su che cosa è un messaggio: `triage.go`, `censisci.go`, `richieste.go`.
- Allegati, download e anteprima: `allegati.go`, `anteprima.go`.
- Il calcolo del quadrante, che vive in `v_inbox`.
- La lettura del corpo (rumore, tabelle, storia citata, Safe Links): `core/inbox/lettura`, un package puro. Qui si chiama
  `lettura.Presenta` e basta.

### File
| File | Responsabilità |
|---|---|
| `routes_inbox.go` | `registraInbox`, `inbox` (frammento `inbox_stato`: comandi e lista insieme; `sel` solo se UUID), `quadranteValido`, `direzioneValida`, `syncStorico`, `finestraStorico`, `nomiCartellaPredefinita`/`nomiDellaCartella`, `partenzaDellaCartella`, `syncStoricoStato`, `badgeStorico`, `messaggio`, `caricaMessaggio` (`messaggioDati.Corpo` = `lettura.Presenta(corpo_testo, corpo_html)`), `copiaInterattiva`, `accodaInterattivo`, `motivoCapacita`, `apriInOutlook`, `segnaLetto`, `bozza`, `rif`/`rifIn`, `statoWorker` |
| `inbox_viva.go` | `novitaPer` (fino a 500 pallini), `lavoroSync`, `conSync`, `aggiornaOra`, `testata`, `syncAllApertura`, `cEUnWorkerOutlookVivo`, `segnaVista`, `avvisoShadow` |
| `agente.go` | `analisiPer` (proposta dell'agente già verificata), `chiediAnalisi` |
| `web/templates/inbox.html` | pagina, `inbox_stato`, `inbox_lista` (oggetto con `oggettoVisibile`), `controparte_riga` |
| `frammenti.html` | `messaggio_pannello` (oggetto con `oggettoVisibile`, corpo con `corpo_messaggio`), `corpo_messaggio`, `corpo_blocco`, `stato_worker`, `chip_controparte`, `candidati_aggancio` |

### Entry point
Tutte le rotte sono `autenticato`: i GET da consultazione in su, i POST da operatore in su.

| Metodo e percorso | Gestore |
|---|---|
| `GET /inbox` | `inbox` (param `q`, `dir`, `filtro`, `casella`, `sel`; `sel` passa solo se è un UUID) |
| `POST /inbox/aggiorna` | `aggiornaOra` (risponde con la testata) |
| `POST /inbox/sync-storico`, `GET /inbox/sync-storico/stato` | `syncStorico`, `syncStoricoStato` |
| `GET /stato/worker` | `statoWorker` |
| `GET /messaggio/{id}` | `messaggio` (senza HTMX reindirizza all'Inbox con `sel`) |
| `POST /messaggio/{id}/apri`, `/letto`, `/bozza` | `apriInOutlook`, `segnaLetto`, `bozza` |
| `POST /messaggio/{id}/analizza` | `chiediAnalisi` |

`registraInbox` monta anche le rotte di triage, censimento, richieste ai fornitori, download e anteprima; i gestori stanno nei loro file.

### Dati
- **Letture:**
  - `v_inbox` (`ListInbox`, fino a 200 righe; `ContaInbox`, `ContaQuadranti`, `GetInboxRiga`), `messaggio`, `messaggio_casella` (`ListPresenze`);
  - `candidato_aggancio` e le richieste candidate, `proposta_triage`, `richiesta_fornitore`, `riferimento_portale` e `bozza`;
  - `sync_cursore`, `job`, `worker_presenza`, `worker_credenziale`, `casella`, l'ultima analisi dell'agente;
  - `messaggio.corpo_testo` e `corpo_html`, solo per la presentazione: il corpo memorizzato non cambia;
  - `messaggio_casella` (`min(ricevuto_il)` per casella e cartella) per la partenza di «Carica precedenti».
- **Scritture:**
  - job `sync_outlook`: stessa chiave dello scheduler, oppure `sync_storico:<casella>` con finestre per cartella e lotto 50;
  - job `apri_elemento_outlook` (priorità 1), `segna_letto` (priorità 2), `crea_bozza_outlook` (chiave `bozza:<id>`), tutti con postazione, casella, richiedente e scadenza;
  - job `analizza_messaggio_ai` (chiave `analisi-ai:<id>`);
  - riga `bozza`, nella stessa transazione del suo job;
  - `messaggio_casella.non_letto`, solo per la copia su cui si è agito;
  - `utente.ultima_vista_inbox`; `sessione.sync_inbox_il`.

### Flussi principali
1. **Apertura della pagina (non HTMX).**
   - Si rende la lista.
   - Poi `segnaVista` sposta l'ultima visita.
   - Poi `syncAllApertura`, una volta per sessione, solo se un worker Outlook è vivo e la configurazione lo consente.
2. **Poll.** Un elemento fratello rilegge i parametri dalla barra degli indirizzi ogni 15 s, o all'evento `inbox-aggiorna`, e sostituisce `#inbox-stato` per intero. La testata si ricarica ogni 15 s, ogni 3 s mentre un sync gira.
3. **Carica precedenti.**
   - Per ogni casella Outlook attiva si fa un solo job storico.
   - La finestra è di `GiorniStorico` (2) giorni per cartella, sotto il suo `storico_fino_a`.
   - Una cartella che non ha ancora un limite parte dalla SUA mail più vecchia (`partenzaDellaCartella`), mai dal limite di
     un'altra cartella né dalla mail più vecchia della casella; senza posta, da adesso. Il nome del cursore («Inbox») e quello
     della copia («Posta in arrivo») si riconoscono come la stessa cartella con `nomiCartellaPredefinita` (Inbox, Sent Items,
     Drafts, Deleted Items, Junk e i nomi italiani); una cartella con un altro nome si cerca con il suo nome e basta.
   - Il badge si ricarica ogni 3 s finché il job non chiude.
4. **Pannello del messaggio.** `caricaMessaggio` prepara `Corpo` con `lettura.Presenta`; `corpo_messaggio` lo disegna:
   - i blocchi di testo, con gli indirizzi (Safe Links srotolati) in grigio e mai cliccabili e le immagini come «[immagine]»;
   - le tabelle incollate da Excel come tabelle (`.corpo-tabella`, numeri allineati a destra);
   - il testo automatico di Outlook (avviso di posta esterna, firma di Outlook mobile, invito Teams, riservatezza…) chiuso in un
     `<details>` con la sua etichetta, mai tolto;
   - la storia citata chiusa in «messaggi precedenti citati»;
   - il «testo originale» (`corpo_testo` com'è) a un clic, tranne quando il testo viene dall'HTML perché quello semplice era vuoto;
   - un corpo oltre il limite di `lettura` (`Ridotto`) si mostra com'è; un corpo vuoto dice «Nessun testo.».
5. **Azione interattiva.**
   - `copiaInterattiva` usa `coda.CopiaPerPostazione`.
   - Senza postazione, o senza un worker idoneo, non si accoda niente e l'avviso dice perché.
   - Una capacità spenta produce un avviso che nomina la voce di `[sicurezza]`.
6. **Bozza.** `InsertBozza` e il job stanno in una transazione. Se la capacità `bozze` è spenta la bozza non resta: rollback.

### Invarianti
- La lista e i suoi comandi vengono dalla stessa risposta (`inbox_stato`). Il poll e la visita sono distinti: il poll non conta come visita e non accoda il sync d'apertura.
- Un job interattivo parte solo verso la copia della postazione della sessione, mai alla «prima copia disponibile».
- Il sync d'apertura parte al massimo una volta per sessione (UPDATE condizionato).
- «Aggiorna ora» accoda al massimo un job per casella.
- `chiediAnalisi` verifica da sé che l'agente sia acceso per le caselle del messaggio: il pulsante nascosto non basta.
- Nessuna bozza viene inviata dal Cockpit: `Invia: false`.
- La pagina non mostra mai l'HTML della mail: solo il testo e le tabelle che `lettura` ne estrae, con l'escape. Gli indirizzi
  della mail non sono mai collegamenti.
- `sel` entra nella pagina (nell'`hx-get` del pannello e nei collegamenti) solo come UUID riscritto dal server; un altro valore
  vale «nessun messaggio scelto».
- Una cartella dichiarata coperta da «Carica precedenti» è stata letta davvero: ogni finestra parte dal limite o dalla mail più
  vecchia di QUELLA cartella.

### Dipendenze
`platform/coda`, `platform/contratti/worker`, `platform/db`, `core/inbox/classificazione`, `core/inbox/lettura`, `ai/agente`.

### Test
- **L1**
  - `inbox_viva_test.go`: chip della testata con sync e novità, passo del poll, frase di «Aggiorna ora», avviso shadow.
  - `corpo_test.go`: il corpo nel pannello e nella pagina RFQ (tabella, testo automatico chiuso, storia citata, «testo
    originale», escape di uno script, nessun collegamento), l'oggetto senza l'etichetta di posta esterna, «Nessun testo.».
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
Completo. Una cartella con un nome che non è né quello della configurazione né uno dei nomi noti di Outlook, se la copia porta
un nome diverso dal cursore, al primo «Carica precedenti» riparte da adesso (`partenzaDellaCartella`): rilegge ciò che il sync
ordinario ha già portato, senza dichiarare coperto niente che non abbia letto.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| capire perché un messaggio sta in quel quadrante | `v_inbox.quadrante`, `routes_inbox.go:quadranteValido` |
| cambiare l'ampiezza di «Carica precedenti» | `routes_inbox.go:GiorniStorico`, `finestraStorico` |
| da dove parte «Carica precedenti» per una cartella nuova | `routes_inbox.go:partenzaDellaCartella`, `nomiCartellaPredefinita` |
| cambiare la testata | `postazione.go:stato`, `inbox_viva.go:conSync`, `frammenti.html:stato_worker` |
| un'azione nuova su Outlook | `routes_inbox.go:accodaInterattivo`, il payload in `platform/contratti/worker` |
| il pannello del messaggio | `routes_inbox.go:caricaMessaggio`, `frammenti.html:messaggio_pannello` |
| come si legge il corpo di una mail | `core/inbox/lettura` (le regole), `frammenti.html:corpo_messaggio`, `corpo_blocco` (il disegno) |

### Leggi anche
`core/README.md` (triage, controparte), `platform/README.md` (coda, capacità), `transport/README.md`.

---

## Modulo Triage (`triage.go`)

Nuova RFQ, Aggancia a… e Ignora: le decisioni dell'operatore su un messaggio.

### Scopo
- Precompilare il form: mittente, cliente, buyer, candidati di codice divisi per ruolo, riferimento del cliente, candidati di aggancio, allegati, cartella NAS e anteprima della destinazione.
- Applicare la decisione in una transazione con il messaggio bloccato.

### Non appartiene qui
- Il triage deterministico e i candidati: `core/inbox`.
- Il nome della cartella: `core/rfq/documenti.CartellaThread` (qui solo il progressivo « (2)», `cartellaLibera`).
- L'elenco dei domini pubblici: `classificazione.DominioPubblico`, l'unico.
- La preparazione del Fascicolo: `core/rfq/fascicolo`, chiamata qui.

### File
| File | Responsabilità |
|---|---|
| `triage.go` | `triageDati` (`Spuntato`: che cosa nasce spuntato), `triageForm`, `datiTriage`, `cartella` (per un cliente censito vale sempre l'anagrafica), `buyerSelect`, `cercaThread`, `messaggioDaDecidere`, `nuovaRFQ`, `preparaDopoLaDecisione`, `agganciaEsistente`, `ignora`, `avvisoRFQEsistente`, `logDecisione`, `agganciaMessaggioAThread`, `clienteDaForm`, `buyerDaForm`, `downloadDaForm`, `triageErrore`, `splitCodici`, `motivoFornitoreSenzaRFQ`, `cartellaLibera` |
| `panoramica.go` | `testoLetterale`: `%`, `_` e `\` della ricerca di «Aggancia a…» presi alla lettera |
| `frammenti.html` | `triage_form`, `triage_allegati`, `cartella_cliente`, `cliente_nuovo`, `anteprima_cartella`, `buyer_select`, `thread_risultati`, `candidati_codice`, `candidati_aggancio` |

### Entry point
| Metodo e percorso | Ruolo | Gestore |
|---|---|---|
| `GET /messaggio/{id}/triage?azione=nuova\|aggancia` | autenticato | `triageForm` (per la posta di un fornitore `nuova` diventa `aggancia`, con il motivo) |
| `POST /messaggio/{id}/rfq` | ≥ operatore | `nuovaRFQ` (rifiutata per la posta di un fornitore) |
| `POST /messaggio/{id}/aggancia` | ≥ operatore | `agganciaEsistente` |
| `POST /messaggio/{id}/ignora` | ≥ operatore | `ignora` |
| `GET /anagrafica/buyer` | autenticato | `buyerSelect` (buyer, cartella e anteprima fuori banda) |
| `GET /thread/cerca?q=` | autenticato | `cercaThread` (RFQ aperte, fino a 30; il testo si prende alla lettera, `testoLetterale`) |

### Dati
Una transazione per decisione, aperta con `BloccaMessaggio` (`SELECT … FOR UPDATE`).
- **Nuova RFQ.**
  - `cliente` e `dominio_cliente`, solo se il cliente è nuovo, con scritture non distruttive (un dominio pubblico secondo
    `classificazione.DominioPubblico` non diventa del cliente); `buyer`.
  - `thread_offerta`, con `cartella_relativa` libera (`cartellaLibera`: quella calcolata, oppure la stessa con « (2)», « (3)»…
    fino a 99, se un'altra RFQ ce l'ha già; confronto senza maiuscole, sotto un advisory lock sul nome fino al COMMIT) e
    `riferimento_cliente` di 60 caratteri al massimo.
  - `identificativo_thread`: i codici spuntati entrano con l'origine della proposta (`proposta_famiglia` o `proposta_generico`) e il loro punteggio; quelli scritti a mano come `manuale` con 100.
  - `fase_log` (RICEVUTA).
  - L'aggancio, il job `crea_cartella_thread` (chiave `cartella:<thread>`), i download spuntati e la preparazione del Fascicolo.
- **Aggancio** (`agganciaMessaggioAThread`).
  - `messaggio` (`aggancio = operatore`), `messaggio_aggancio_log`, `documento_proposta` e `riferimento_portale` del messaggio.
  - `proposta_triage` passa ad accettata; `conversazione` viene collegata.
  - Per gli altri orfani della stessa conversazione: un `candidato_aggancio` R1 e `proposta_triage` aggiornata, **non** un aggancio.
  - Il buyer sul messaggio e sulla RFQ (`SetBuyerMessaggio`, `SetBuyerThread`): un errore ferma l'aggancio.
- **Ignora:** `IgnoraMessaggio` e il log.

### Flussi principali
1. Form → `datiTriage`.
   - Nasce spuntato solo un codice di famiglia trovato nel testo nuovo (`triageDati.Spuntato`): un codice di famiglia trovato
     solo nella storia citata (`classificazione.DoveStoria`) si vede, con la sua evidenza, ma per entrare vuole un clic.
   - Se il cliente non ha famiglie di codice, i numeri generici diventano spuntabili ma non nascono spuntati.
   - Il riferimento del cliente ha un campo suo e non diventa mai un codice.
   - Per la posta di un fornitore il form di «Nuova RFQ» non si offre: `triageForm` passa ad «Aggancia» con il motivo.
2. Nuova RFQ.
   - Blocco del messaggio.
   - Già agganciato → avviso esplicito (`avvisoRFQEsistente`).
   - Controparte `fornitore` (letta dal messaggio appena bloccato) → rifiuto con `motivoFornitoreSenzaRFQ`, niente scritto.
   - Altrimenti cliente, buyer, cartella libera, thread, codici, fase, aggancio, job della cartella (anche con `nas_scrittura`
     spenta la RFQ nasce e la cartella resta in attesa), download, `preparaDopoLaDecisione`, commit, pannello con l'avviso.
3. Aggancia: stessi passi senza la creazione del thread; il buyer viene creato solo con `crea_buyer=1`, e un suo errore (logico
   o del database) ferma l'aggancio e torna nel form (`triageErrore`).

### Invarianti
- Una decisione per messaggio: tutte le strade passano da `messaggioDaDecidere`, e chi perde la corsa riceve un esito esplicito.
- Un codice non proposto non entra dalla spunta; un riferimento RFQ non diventa un identificativo nemmeno se spuntato.
- Per un cliente censito la cartella viene dall'anagrafica, mai dal campo digitato.
- Due RFQ non condividono una cartella del NAS: la seconda con lo stesso nome prende « (2)».
- La posta di un fornitore non apre una RFQ cliente: la regola è nel pannello, nel form e nel POST.
- Agganciare un messaggio non aggancia gli altri orfani della conversazione: ricevono solo un candidato R1 (T21).

### Dipendenze
`core/inbox/classificazione`, `core/registro/{anagrafica,regole}`, `core/rfq/{documenti,fascicolo}`, `platform/{coda,contratti/worker,db}`.

### Test
- **L1:** `triage_test.go` (un codice della storia citata non nasce spuntato; `chip_controparte` con `altro`; la ricerca di
  «Aggancia a…» alla lettera).
- **L4**
  - `candidati_db_test.go`: T21, T23, codice inventato.
  - `cartella_db_test.go`: cartella dall'anagrafica e destinazione.
  - `campicliente_db_test.go`: i campi del cliente nuovo compaiono e spariscono.
  - `decisioni_db_test.go`: due nuove RFQ concorrenti ne creano una sola.
  - `b87b_db_test.go`: preparazione dopo la decisione e aggancio.
  - `richieste_db_test.go`: IB6, ignora.
  - `triage_db_test.go`: un webmail non diventa il dominio di un cliente nuovo; un no sul buyer ferma l'aggancio (`crea_buyer`);
    la posta di un fornitore non apre una RFQ nemmeno a mano; due RFQ con lo stesso nome prendono « (2)» e « (3)».

### Stato dell'implementazione
Completo. L'anteprima della destinazione nel form (`anteprima_cartella`) mostra il nome calcolato: il progressivo « (2)» si
decide solo quando la RFQ nasce.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| cambiare che cosa succede alla creazione di una RFQ | `triage.go:nuovaRFQ`, `preparaDopoLaDecisione` |
| cambiare la propagazione alla conversazione | `triage.go:agganciaMessaggioAThread` |
| cambiare il form | `triage.go:datiTriage`, `frammenti.html:triage_form` |
| cambiare che cosa nasce spuntato | `triage.go:triageDati.Spuntato`, `frammenti.html:candidati_codice` |
| capire la destinazione sul NAS | `triage.go:cartella`, `cartellaLibera`, `core/rfq/documenti` |

### Leggi anche
`core/README.md`, `internal/README.md` (flusso «Nuova RFQ»).

---

## Modulo Anagrafica e censimento (`censisci.go`, `anagrafica.go`, `anagrafica_admin.go`, `convenzioni_admin.go`, `fornitori_admin.go`)

Clienti e fornitori: censiti dal pannello del messaggio (operatore) o amministrati in Admin › Anagrafica.

### Scopo
- **Censimento dall'Inbox:** fornitore o cliente con dominio e/o indirizzo, poi il ricalcolo dei soli messaggi non decisi.
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
| File | Responsabilità |
|---|---|
| `censisci.go` | `censisciForm`, `datiCensisci`, `censisci`, `censisciFornitore`, `censisciCliente`, `cognomeENome`, `ritriagePer`, `AggiungiDominioFornitore` |
| `anagrafica.go` | `vincoloViolato`, `CreaCliente`, `AggiungiDominio`, `rendiAnagrafica`, `nuovoCliente`, `salvaCliente` (peso 0–15), `salvaRegole`, `aggiungiDominioCliente`, `eliminaDominioCliente`, `bancoProva`, `txtN` |
| `anagrafica_admin.go` | `salvaRegoleDalForm` («Rev nel codice» letta per numero di riga), `scriviRegole`, `nuovoBuyerCliente`, `eliminaBuyerCliente` (solo un buyer del cliente della pagina), `aggiungiFabbisogno` (riga e fonte attesa in una transazione), `eliminaFabbisogno`, tendine |
| `convenzioni_admin.go` | `nuovaConvenzione` (in transazione), `eliminaConvenzione`, `attivaConvenzione`, `qualificaFornitore`, `eliminaQualificaCliente`, `provaCodiceCliente` |
| `fornitori_admin.go` | `rendiFornitori`, `nuovoFornitore`, `salvaFornitore`, domini e contatti, `salvaLavorazioniFornitore` (tutto o niente, in una transazione), qualifiche, `importaFornitoriForm`, `importaFornitori`, `testoDelSeme` (4 MB al massimo) |
| template | `anagrafica.html`, `fornitori.html`, `importa.html`, `frammenti.html:censisci_form` |

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

I fornitori stanno sotto `/admin/fornitori` e non sotto `/admin/anagrafica/fornitori/{id}`: con il mux di Go quel pattern andrebbe in conflitto con `/admin/anagrafica/{id}/regole`.

### Dati
- **Tabelle scritte:**
  - lato cliente: `cliente` (insert, update, `regole`), `dominio_cliente`, `buyer` (`EliminaBuyer` cancella solo un buyer del cliente della pagina, e non uno citato da messaggi, RFQ o proposte di triage), `fabbisogno_documento` (si tolgono solo le righe del cliente, non i predefiniti), `convenzione_codice` e `convenzione_codice_lavorazione`;
  - lato fornitore: `cliente_fornitore_lavorazione`, `fornitore`, `dominio_fornitore`, `contatto_fornitore`, `fornitore_lavorazione`.
- **Scritture non distruttive:** un 23505 diventa una frase che dice di chi è la cartella o il dominio.
- **Transazioni:** nessuna nelle scritture singole, di proposito, perché la domanda «di chi è» deve poter rispondere. Stanno in
  una transazione le scritture di più righe: `nuovaConvenzione`, `aggiungiFabbisogno` (la riga e la sua fonte attesa),
  `salvaLavorazioniFornitore` (un rifiuto a metà giro lascia le lavorazioni com'erano).
- **Ricalcolo:** dopo l'aggiunta di un dominio, di un contatto, di un buyer con e-mail, dopo un censimento o l'applicazione del seme → `ingest.Ritriage` o `RitriageMolti`, sui soli messaggi non decisi.

### Flussi principali
1. **Censisci.**
   - `ingest.IndirizzoDaCensire`: in entrata il mittente, in uscita il primo destinatario esterno.
   - Su un dominio pubblico il predefinito è l'indirizzo.
   - Fornitore: si riusa quello con la stessa ragione sociale e gli si aggiungono dominio, contatto e lavorazioni. Cliente: `CreaCliente`, `AggiungiDominio`, buyer.
   - Poi il ricalcolo mirato e il pannello con l'esito.
2. **Regole.**
   - Il form ricostruisce `regole.Regole`; il JSON arriva così com'è scritto. Le famiglie arrivano come array paralleli, tranne
     «Rev nel codice»: è una casella di spunta, il browser manda solo quelle spuntate, e ognuna porta il numero della sua riga
     (`fam_rev` = indice in `anagrafica.html`).
   - Entrambi passano da `regole.ValidaRegole`; ciò che non passa torna nel riquadro con la diagnosi riga per riga.
   - Le regole valgono dal prossimo lotto di posta.
3. **Import del seme.** Anteprima con `fornitori.Calcola`; «applica» con `fornitori.Applica`, poi il ricalcolo sulle chiavi scritte.

### Invarianti
- Una cartella NAS, un profilo o un dominio già di un altro soggetto non si spostano: l'operazione fallisce e dice di chi sono (T7, T8).
- La pagina di un cliente cancella solo le persone di quel cliente; una persona citata da messaggi, RFQ o proposte di triage resta.
- Il ricalcolo tocca solo i messaggi non decisi.
- Una regola o una convenzione che non rispetta lo schema non entra in database.
- Il banco di prova e «prova un codice» usano lo stesso motore dell'ingest.

### Dipendenze
`core/registro/{regole,fornitori}`, `core/inbox/{classificazione,ingest}`, `core/rfq/documenti` (`NomeSicuro`), `platform/db`.

### Test
- **L4**
  - `anagrafica_db_test.go`: T7, T8, regola rotta, banco di prova, operatore escluso, peso, scheda del fabbisogno.
  - `fornitori_db_test.go`: CP5 censisci come fornitore, censisci come cliente, schede e scritture non distruttive, convenzioni, CP14 import, l'anagrafica ricalcola i messaggi.
  - `bootstrap_db_test.go`: l'import ricalcola i messaggi già arrivati.
  - `regole_form_db_test.go`: «Rev nel codice» resta sulla famiglia giusta (`POST …/regole/form`); la riga del fabbisogno nasce con la sua fonte (`…/fabbisogno`).
  - `buyer_db_test.go`: un buyer si cancella solo dalla pagina del suo cliente, e non se una proposta di triage lo cita (`…/buyer/elimina`).
  - `transazioni_admin_db_test.go:TestLeLavorazioniDiUnFornitoreSiSalvanoTutteONiente`.

### Stato dell'implementazione
Completo; la scheda Articoli è uno scheletro (blocco 5). Limite noto: togliere un dominio, un contatto o un buyer non ricalcola
i messaggi.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| un campo nuovo nelle regole del cliente | `core/registro/regole`, `anagrafica_admin.go:salvaRegoleDalForm`, `anagrafica.html` |
| cambiare la regola «non distruttiva» | `anagrafica.go:CreaCliente`, `AggiungiDominio`, `censisci.go:AggiungiDominioFornitore` |
| cambiare il censimento dall'Inbox | `censisci.go`, `frammenti.html:censisci_form` |
| l'import dei fornitori | `fornitori_admin.go:importaFornitori`, `core/registro/fornitori` |

### Leggi anche
`core/README.md` (registro, regole, ricalcolo), `internal/README.md` (flusso «Censisci dall'Inbox»).

---

## Modulo Postazioni (`postazione.go`, `postazioni_admin.go`)

La postazione della sessione, la testata e la pagina che genera i pacchetti dei worker.

### Scopo
- Sapere da quale PC lavora la sessione: abbinamento per IP, riabbinamento, scelta in testata. Così i job interattivi aprono le finestre sul PC giusto.
- Dire in testata lo stato di ogni casella e del worker di analisi.
- Generare segreti e pacchetto di una postazione.

### Non appartiene qui
- Quali postazioni e quali worker esistono e che caselle servono: si dichiara in `cockpit.toml` e lo semina `platform/fondazioni`.
- L'autenticazione dei worker: `transport/workerapi`.

### File
| File | Responsabilità |
|---|---|
| `postazione.go` | `sessioneUI`, `sessioneDa`, `puoUsarePostazione` (postazione attiva e dell'utente, oppure admin), `postazioniScegliibili`, `indirizzoDi` (solo `RemoteAddr`, `X-Forwarded-For` mai), `abbinaPostazione` (IP visto entro 24 h, una sola postazione autorizzata), `scegliPostazione` (una postazione non autorizzata → `nega`, senza ruolo), `dopoScelta`, `statoUI`, `stato`, `statoCaselle`, `statoAnalisi`, `durataBreve` |
| `postazioni_admin.go` | `URLServer` (`[server].url_pubblico`, altrimenti derivato dal bind), `adminPostazioni`, `rendiPostazioni` (stato delle credenziali con `fondazioni.StatoCredenziali`), `pacchettoWorker` (token e zip in una transazione), `workerTOML`, `istruzioni`, `servePostazione`, `zipPacchetto`, `registraPostazioni` |
| template | `postazioni.html`, `frammenti.html:stato_worker` |

### Entry point
| Metodo e percorso | Ruolo | Gestore |
|---|---|---|
| `POST /sessione/postazione` | ≥ operatore | `scegliPostazione` (vuoto = «non lo so», l'origine si azzera; da htmx 204 con `HX-Refresh`, altrimenti redirect a `/inbox`) |
| `GET /admin/postazioni` | admin | `adminPostazioni` |
| `POST /admin/postazioni/{host}/pacchetto` | admin | `pacchettoWorker` (POST perché invalida i token; risponde con lo zip, `application/zip` in download) |

`stato` lo chiamano `rendi`, `nega`, `statoWorker` e `testata`.

### Dati
- **Letture:** `postazione`, `worker_presenza` (vivo o spento si decide su `ultimo_contatto` contro `worker.PresenzaOnlineEntro`), `worker_credenziale`, `casella`.
- **Scritture:**
  - `sessione.postazione_id` e `postazione_origine`, che vale `ip` o `scelta`;
  - `worker_credenziale.token_hash`: lo sha256 del token nuovo, `rete.ImprontaToken`.
- **Pacchetto:** zip in memoria con
  - `worker.toml`: URL, impronta TLS, un token per worker;
  - `ISTRUZIONI.txt`;
  - `cert.pem`, solo con TLS e solo il certificato pubblico;
  - i file di `workers/` incorporati nel binario, meno i test e gli strumenti di sviluppo.

### Flussi principali
1. **Login e ogni richiesta.**
   - Una sessione senza origine prova il riabbinamento per IP.
   - Una scelta fatta in testata non viene mai sovrascritta.
2. **Testata.** Stato di ogni casella: `attiva`, `in_corso`, `offline`, `non_risolta`, `non_configurata`. Si aggiungono novità e ultimo sync riuscito (`conSync`).
3. **Pacchetto.** In una transazione: per ogni worker della postazione token nuovo e impronta in database; poi lo zip, costruito
   PRIMA del COMMIT. Se lo zip non nasce, rollback: i token di prima valgono ancora. Dopo il COMMIT il download; i token
   precedenti smettono di valere subito.
4. **Scelta non autorizzata.** `scegliPostazione` risponde con `nega` (403, pagina o frammento del divieto) e un motivo che dice
   dove si abilita la postazione (`[[postazione]]` in `cockpit.toml`); la pagina non nomina un ruolo.

### Invarianti
- L'IP è un indizio su quale postazione, mai un permesso: sia l'abbinamento sia la scelta passano da `puoUsarePostazione`.
- Il riabbinamento tocca solo le sessioni senza origine.
- In database c'è solo l'impronta dei token; i segreti si vedono una volta sola, nello zip.
- I token di una postazione cambiano tutti insieme, e solo se lo zip che li consegna è nato.
- Il pacchetto lo genera solo un amministratore.

### Dipendenze
`platform/{db,fondazioni,rete,contratti/worker}`.

### Test
- **L1:** `postazione_test.go` (M2, i tre stati; presenza dal contatto; `puoUsarePostazione`).
- **L4**
  - `postazione_db_test.go`: W14 sessione, riabbinamento, P1, M2 testata.
  - `presenza_db_test.go`: 4 test.
  - `rete_db_test.go`: PK1 token che funziona, solo admin, indirizzo del server.
  - `pacchetto_postazione_db_test.go`: `cert.pem` e script.
  - `migrazione_credenziali_db_test.go`: PK2, credenziale mai generata, stato delle credenziali.
  - `transazioni_admin_db_test.go:TestUnPacchettoNonCostruitoNonCambiaITokenDellaPostazione`.
- La pagina del divieto senza ruolo: `ruoli_test.go:TestIlDivietoDiceIlRuoloCheServeDavvero` (L1).

### Stato dell'implementazione
Completo. Limiti noti al 25/09:
- la scelta parte con htmx dalla testata, e un 403 da htmx non si innesta (vedi il modulo Server e ruoli): il frammento del
  divieto non compare, resta la riga nel log. La tendina offre solo le postazioni scegliibili, quindi ci si arriva con una
  lista rimasta vecchia o con una richiesta scritta a mano;
- il ramo del template «solo un amministratore» non si raggiunge, perché la pagina è `soloAdmin`.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| cambiare lo stato di una casella in testata | `postazione.go:statoCaselle`, `inbox_viva.go:conSync` |
| cambiare l'abbinamento per IP | `postazione.go:abbinaPostazione`, `finestraAbbinamentoIP` |
| cambiare il contenuto del pacchetto | `postazioni_admin.go:workerTOML`, `zipPacchetto`, `servePostazione`, `embed.go` |

### Leggi anche
`workers/workers_README.md`, `platform/README.md` (`rete`, `fondazioni`).

---

## Modulo Admin tecnico (`routes_admin.go`, `integrita_admin.go`)

Coda job, scarti dell'ingest, Integrità NAS e il montaggio di tutte le rotte `/admin/*`, tranne Postazioni.

### Scopo
- Guardare e correggere il motore: riaccodare o annullare un job, riprovare uno scarto.
- Confrontare ciò che il database promette con i file veri sul NAS, e offrire solo le azioni sicure.

### Non appartiene qui
- Il ricognitore e l'allineamento: `core/rfq/documenti` (`Ricognitore.Giro`, `AllineaDocumento`).
- La riprova degli scarti: `core/inbox/ingest` (`Servizio.Riprova`).

### File
| File | Responsabilità |
|---|---|
| `routes_admin.go` | `registraAdmin`: tutte le rotte `/admin/{job,nas,scarti,anagrafica,fornitori}` con `soloAdmin`; `adminJob`, `riaccodaJob`, `annullaJob`, `adminScarti`, `riprovaScarto` |
| `integrita_admin.go` | `integritaDati`, `rigaIntegrita` (`StatoFilesystem`, `Azione`), `adminIntegrita`, `datiIntegrita`, `integritaFrammento`, `controllaIntegrita`, `frasePassata`, `riaccodaDocumento`, `allineaDocumento` |
| template | `job.html` (poll 5 s), `scarti.html` (poll 10 s), `integrita.html` |

### Entry point
Tutte `soloAdmin`:
- `GET /admin/job?stato=`, `POST /admin/job/{id}/riaccoda`, `POST /admin/job/{id}/annulla` (riuscite: 204 con `HX-Refresh`);
- `GET /admin/scarti?origine=ingest|lettura`, `POST /admin/scarti/{id}/riprova`;
- `GET /admin/nas`, `POST /admin/nas/controlla`, `POST /admin/nas/{id}/riaccoda`, `POST /admin/nas/{id}/allinea` (`{id}` è il `documento_id`).

`stato` vale solo se è uno stato di job, `origine` solo se è `ingest` o `lettura`; un altro valore vale «tutti». Nel poll della
pagina (`hx-get` di `job.html` e `scarti.html`) torna il valore riconosciuto, mai il testo dell'indirizzo.

### Dati
- `job`: fino a 100 righe per stato; `RiaccodaJob` solo per job falliti o annullati e senza un gemello pendente con la stessa chiave; `AnnullaJob` solo per job pronti o in corso.
- `ingest_scarto`: fino a 200 righe.
- `nas_anomalia`: segnalazioni aperte e conteggi.
- `documento`: `coda.AccodaCopia` rispetta `nas_scrittura`; `AllineaDocumento` ricalcola l'hash sul file vero.

### Flussi principali
1. **«Controlla ora».** `Ricognitore.Giro`, la stessa funzione del giro automatico, poi la tabella ridisegnata per intero con la frase dell'esito.
2. **Azioni sulle segnalazioni.**
   - `riaccoda` per le copie in attesa, in errore o mancanti.
   - `allinea` per un file già presente con l'hash giusto.
   - Niente per un conflitto o un file illeggibile.

### Invarianti
- Un conflitto o un file illeggibile non si riaccodano: il gestore rifiuta anche se il pulsante venisse forzato. Il Cockpit non sovrascrive mai un file sul NAS.
- Si allinea solo `gia_presente`, dopo aver ricalcolato l'hash.
- Riaccodare non viola l'indice unico delle chiavi di idempotenza: il caso diventa un avviso.

### Dipendenze
`core/rfq/documenti`, `core/inbox/ingest` (tramite `Server.Ingest`), `platform/{coda,db}`.

### Test
- **L4**
  - `integrita_db_test.go`: 7 test (mai guardato, file mancante, conflitto, allineamento, avviso nella RFQ, scrittura spenta, solo admin).
  - `rbac_db_test.go`: le schermate tecniche con admin e con operatore.
  - `sicurezza_db_test.go:TestIlPollDellAdminNonRiportaIlTestoDellIndirizzo`.

### Stato dell'implementazione
Completo. Il difetto noto di «riprova» sugli scarti di origine `lettura` è descritto in `transport/README.md`.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| una schermata admin nuova | `routes_admin.go:registraAdmin` (con `soloAdmin`), `navigazione.go:navPer` |
| cambiare le azioni sulle anomalie | `integrita_admin.go:rigaIntegrita.Azione`, `riaccodaDocumento`, `allineaDocumento` |

### Leggi anche
`core/README.md` (integrità NAS), `platform/README.md` (coda).

---

## Modulo Pagina RFQ (`routes_rfq.go`, `thread.go`, `proposte.go`, `codici.go`)

La pagina di una richiesta, `/thread/{id}`, e i gesti sulle proposte di struttura (B8.5) e sui codici (B8.6).

### Scopo
- Mostrare testata, messaggi agganciati, documenti sul NAS con i loro stati, anomalie, riepilogo del Fascicolo, pannello dei codici e bozze.
  Il testo di ogni messaggio è quello del pannello dell'Inbox: `messaggioThread.Corpo` da `lettura.Presenta`, disegnato con
  `corpo_messaggio`; l'oggetto con `oggettoVisibile`.
- Rileggere gli STEP all'apertura.
- Eseguire i gesti in una transazione ciascuno, rispondendo con la pagina o con la schermata del Fascicolo.

### Non appartiene qui
- La schermata del Fascicolo e i suoi gesti: `fascicolo*.go`.
- Conferma e scarto delle proposte di allegato: `allegati.go`.
- Le regole sulle proposte: `core/rfq/fascicolo`.

### File
| File | Responsabilità |
|---|---|
| `routes_rfq.go` | `registraRFQ`: monta anche le rotte di `fascicolo*.go` e `allegati.go`; `cruscotto` reindirizza con 303 a `/richieste` |
| `thread.go` | `threadDati`, `messaggioThread` (con `Corpo`), `thread` (`sel` solo se UUID), `threadFrammento` (se il gesto viene dal Fascicolo risponde con `rispondiFascicolo`), `riprovaCopie`, `rimettiInCoda`, `esitoCopie`, `caricaThread` |
| `proposte.go` | `registraProposte`, `gesto`/`gestoPoi` (tutto o niente, «Niente è cambiato: …»), `idDa`, `rianalizza` (fino a `MaxAccodatiSuRichiesta` = 20), `rileggiAllApertura`, accetta/scarta per nodo, relazione, file o sottoalbero, rimozione |
| `codici.go` | `registraCodici`, `aggiungiCodice`, `ripristinaComponente`, `rigaCodice`, `etichettaTipo`, `BersaglioCodici`, `codiciDellaRfq` |
| template | `thread.html`, `frammenti.html:codici_rfq`, `codice_riga`, `corpo_messaggio` |

### Entry point
Tutte `autenticato`; le POST richiedono almeno operatore.
- **Pagina e copie:** `GET /thread/{id}` (param `sel`, solo UUID), `POST /thread/{id}/riprova-copie`.
- **Proposte di struttura:**
  - `POST /thread/{id}/fascicolo/rianalizza`;
  - `…/nodo/{pid}/accetta|scarta|codice`;
  - `…/relazione/accetta|scarta` (campi `allegato`, `padre`, `figlio`);
  - `…/file/{aid}/accetta` (con `chiave` solo il sottoalbero);
  - `…/rimozione/accetta|scarta` (campi `step`, `padre`, `figlio`).
- **Codici:** `POST /thread/{id}/fascicolo/codice/aggiungi` (campi `codice`, `tipo`, `rev`), `…/componente/{cid}/ripristina`.
- **Redirect:** `GET /cruscotto`.

### Dati
- **Letture:**
  - `thread_offerta`, `v_cruscotto` (anche `n_da_smistare`), `cliente`, `identificativo_thread`;
  - `documento`, `v_fascicolo`, `bozza`, `componente`, `nas_anomalia`;
  - messaggi con presenze e allegati (il corpo solo per la presentazione); `fascicolo.CandidatiDellaRfq`.
- **Scritture:**
  - all'apertura `fascicolo.RianalizzaRfq`, in una transazione sua, se `[analisi]` è configurata;
  - i gesti tramite le funzioni di `core/rfq/fascicolo`, in una transazione;
  - `riprova-copie`: `coda.AccodaCopia` per ogni documento `in_coda` o `errore`.

### Flussi principali
1. **Apertura.**
   - `rileggiAllApertura`: se fallisce la pagina si apre comunque e il log lo dice.
   - Poi `caricaThread` e `rendi`.
2. **Gesto.**
   - `gestoPoi`: transazione → funzione di dominio → commit, oppure rollback con la spiegazione (`spiegaErrore`).
   - Risposta con `threadFrammento`: la pagina della RFQ, oppure i pannelli del Fascicolo se `HX-Current-URL` è quello della schermata.
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
`core/rfq/fascicolo`, `core/inbox/lettura`, `platform/{coda,db}`.

### Test
- **L1:** `codici_test.go` (ogni codice al suo gesto; BOM congelata), `esitocopie_test.go`, `templates_test.go`, `corpo_test.go` (il corpo in `thread_corpo`).
- **L4**
  - `b85_db_test.go`: rilettura all'apertura, gesti che rispondono con la pagina, scelte.
  - `b86_db_test.go`: pannello dei codici.
  - `copie_db_test.go`, `copieerrore_db_test.go`, `copieparziali_db_test.go`, `chiavecopia_db_test.go`.
  - `integrita_db_test.go`: la RFQ dice che un suo documento è segnalato.
- Le rotte `relazione/*` e `rimozione/*` non hanno test HTTP in questo package.

### Stato dell'implementazione
- Completo.
- La rotta della richiesta a un fornitore resta senza form (vedi il modulo Richieste ai fornitori).
- L'apertura rilegge gli STEP e scrive anche quando la pagina la apre un utente in consultazione.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| un gesto nuovo sulle proposte | `proposte.go` (con `gesto`), la funzione in `core/rfq/fascicolo` |
| cambiare che cosa mostra la pagina | `thread.go:caricaThread`, `thread.html` |
| cambiare il pannello dei codici | `codici.go`, `frammenti.html:codici_rfq`, `codice_riga` |

### Leggi anche
`transport/README.md` (rotte del Fascicolo), `core/README.md` (`fascicolo`).

---

## Modulo Richieste ai fornitori, blocco 7B (`richieste.go`)

Le richieste d'offerta a un fornitore e il loro legame con la posta. Da non confondere con la pagina Richieste, che sta in `panoramica.go`.

### Scopo
- Creare una richiesta, con la bozza marcata in Outlook se richiesta.
- Confermare dall'Inbox:
  - «è la risposta a questa richiesta», con un atto: collegata, offerta, declinata;
  - «è la richiesta mandata a mano».

### Non appartiene qui
Il riconoscimento del marcatore e dei candidati (`core/inbox/ingest`, `core/inbox/aggancio`).

### File
| File | Responsabilità |
|---|---|
| `richieste.go` | `nuovaRichiestaFornitore`, `bozzaPerRichiesta`, `oggettoRichiesta` («RFQ <cartella> <buyer> <codici>»), `corpoRichiesta`, `annullaRichiestaFornitore`, `rispostaFornitore`, `richiestaFornitoreManuale` |

### Entry point
Tutte `autenticato`, POST da operatore in su:
- `POST /thread/{id}/richiesta` (campi `fornitore_id`, `lavorazione`, `codice`, `note`, `bozza=1`) — nessun form la chiama più;
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
1. **Nuova richiesta.** Riga `bozza` → con `bozza=1`, la bozza «nuovo» sul PC della sessione, dalla prima casella della RFQ servita da quella postazione.
2. **Risposta.** Aggancio alla RFQ, se il messaggio non era deciso → legame con la richiesta → stato secondo l'atto.

### Invarianti
- Lo stato della richiesta cambia solo con l'atto confermato: una risposta qualunque non la chiude.
- Da una mail mandata a mano non nasce mai una RFQ nuova.
- Nessun invio dal Cockpit.

### Dipendenze
`core/inbox/ingest` (`MarcatoreRichiesta`), `platform/{coda,contratti/worker,db}`.

### Test
- **L4:** `richieste_db_test.go` (IB2 marcatore, richiesta senza bozza, IB3–IB5, IB6, IB7).
- **L1:** `templates_test.go` (`TestLaRFQNonCreaRichiesteAiFornitori`).

### Stato dell'implementazione
- La rotta di creazione resta per lo storico.
- Il form nella pagina della RFQ è stato tolto di proposito: tornerà nella scheda delle lavorazioni, per lavorazione di un componente.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| capire perché una risposta non chiude la richiesta | `richieste.go:rispostaFornitore` |
| cambiare oggetto o corpo della bozza | `richieste.go:oggettoRichiesta`, `corpoRichiesta` |

### Leggi anche
`internal/README.md` (flusso «Richiesta a un fornitore»).

---

## Modulo Pagina Richieste (`panoramica.go`)

`GET /richieste`: una card per RFQ con le schede dei prodotti, e i filtri nell'indirizzo.

### Scopo
Elencare le RFQ, aperte per impostazione predefinita, con filtri e ordinamento. Per ogni RFQ mostrare fino a sei prodotti con l'anteprima del 2D, poi gli altri a richiesta. Aggiornare l'elenco con un poll che non ricarica niente se niente è cambiato.

### Non appartiene qui
- Le richieste ai fornitori: `richieste.go`.
- Il PDF dell'anteprima: `anteprima.go`.

### File
| File | Responsabilità |
|---|---|
| `panoramica.go` | `filtriRichieste`, `leggiFiltriRichieste` (`q`, `cliente`, `fase`, `stato`, `bloccanti`, `smistare`, `sla`, `sort`, `n`), `valori`/`URL`/`Altre`, `modelloRicerca` e `testoLetterale` (`%`, `_` e `\` presi alla lettera; `testoLetterale` serve anche alla ricerca di «Aggancia a…»), `costruisciPanoramica`, `raggruppaProdotti`, `sceglieAnteprima`, `firmaPanoramica`, `caricaPanoramica`, `opzioniBarra`, `richieste`, `richiestaProdotti` |
| `web/templates/richieste.html` | barra dei filtri, `richieste_elenco`, `richiesta_card`, `richiesta_prodotti` |

### Entry point
- `GET /richieste`, autenticato: pagina intera, oppure l'elenco quando cambia un filtro (con `HX-Push-Url`), oppure il poll con `firma`.
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
2. **Poll.** Ogni 60 s, o ogni `PollRichieste`. Con la stessa firma (sha256 di filtri, totale, card e prodotti) il server risponde 204 e il browser non tocca niente.

### Invarianti
- Lo stato della pagina sta nell'indirizzo.
- Il poll non ridisegna se l'elenco non è cambiato.
- L'anteprima di un prodotto è il documento confermato prima della proposta; fra più candidati il più recente; a parità l'allegato con l'identificativo minore.

### Dipendenze
Solo `platform/db`.

### Test
- **L1:** `panoramica_test.go` (filtri dall'indirizzo, URL pulito, ricerca alla lettera, scelta dell'anteprima, schede di una card, firma, template).
- **L4:** `panoramica_db_test.go` (lista di lavoro, filtri, anteprime, sei schede e poi le altre, HTMX indirizzo e poll, «Mostra altre», 100 RFQ in meno di 1 s).
- **L7:** `richieste_browser_test.go` con `e2e/richieste.py`.

### Stato dell'implementazione
Completo. L'ordinamento «priorità» è peso del cliente, poi scadenza: il punteggio dell'addendum 2 non esiste ancora.

### Dove intervenire
| Voglio… | Apro |
|---|---|
| un filtro nuovo | `panoramica.go:filtriRichieste` (lettura, `valori`, `parametri`), `panoramica.sql`, `richieste.html` |
| cambiare la scelta dell'anteprima | `panoramica.go:sceglieAnteprima` |
| cambiare quante card o schede | le costanti in testa a `panoramica.go` |

### Leggi anche
`platform/README.md` (query), `transport/README.md`.

---

## Modulo Fascicolo: la schermata e le sue viste (`fascicolo_pagina.go`, `fascicolo_rotte.go` per le GET, `fascicolo_bom.go`, `fascicolo_dettaglio.go`)

### Scopo
La schermata `/thread/{id}/fascicolo` ha tre viste: Documenti (la predefinita, modulo seguente), Struttura BOM (la BOM visuale a
card), Completezza (la matrice 3D/2D/DXF/STEP). Poi le viste di servizio Elenco file, Componenti e Albero, il pannello di destra
(file, componente o nodo proposto), il piano in fondo e i cassetti Da verificare, Rivedi, Importa dal NAS, Codici e Avvisi.
Template: `web/templates/fascicolo.html`.

### Rotte
Tutte `autenticato`; le GET le apre anche `consultazione`.

| Metodo e percorso | Gestore | Risposta |
|---|---|---|
| `GET /thread/{id}/fascicolo` | `fascicoloPagina` | la pagina, o `fasc_corpo`. Prima: per operatore o superiore `preparaFascicolo`, per `consultazione` `rileggiAllApertura` |
| `GET …/fascicolo/parti` | `fascicoloParti` → `rispondiFascicolo` | `fasc_parti`, con `HX-Push-Url` dello stato |
| `GET …/fascicolo/anteprima` | `fascicoloAnteprima` | `fasc_anteprima` (pannello di destra più il resto fuori banda) |
| `GET …/fascicolo/vista` | `fascicoloVista` | `fasc_vista_frammento` (la ricerca: non rifà la testata) |

### Lo stato nell'indirizzo (`statoFascicolo`, `leggiStatoFascicolo`, `Con`)

| Parametro | Valori |
|---|---|
| `nodo`, `prop`, `file`, `doc` | gli id scelti |
| `scheda` | `3d`, `2d`, `dxf`, `altri`, `storico` |
| `filtro` | `nodo`, `candidati`, `tutti`, `da_scaricare`, `rumore`; vuoto = non assegnati |
| `tipo` | un tipo di documento, per il filtro dei candidati |
| `vista` | vuoto (Documenti), `bom`, `completezza`, `file`, `componenti`, `albero`; alias `documenti`→`file`, `griglia`→`completezza` |
| `gruppo` | l'uuid di un prodotto, `fuori`, `file`, `rfq` |
| `cassetto` | `verifica`, `piano`, `nas`, `codici`, `avvisi` |
| `scartate` | `1` |
| `q`, `nas_cerca` | al massimo 60 caratteri |
| `nas` | la cartella aperta nel cassetto |

Un valore non valido cade sul predefinito. `Con("nodo", …)` toglie `tipo`, `doc`, `scheda` e `prop`. `dalFascicolo` rilegge lo
stato da `HX-Current-URL`: è così che un gesto sa di venire da qui.

### Flusso
`caricaFascicolo` legge tutta la schermata, in quest'ordine:
1. thread, cliente, riga del cruscotto, fase;
2. versioni della BOM, differenze e gate (`versioniFascicolo`);
3. componenti e archi → `fascicolo.NuovoAlbero`;
4. `v_fascicolo` → `cellaDa`; `v_step_prodotto`;
5. documenti, proposte di struttura (`proposteFascicolo`: in linea sotto il padre, oppure nel blocco del file), rimozioni;
6. file (`fileFascicolo`: allegato + proposta + documento; `os.Stat` di `path_staging`) → `filtraFile`;
7. scheda del nodo; codici (solo con il cassetto `codici`); avvisi;
8. `LeggiPianoFascicolo` e `LavoroInCorso`;
9. per la vista `bom` `costruisciBom`, per `componenti` `componentiInCard`, per Documenti `vistaDocumenti`, altrimenti `pannelloDestro` + `chiaveCorpo`;
10. il cassetto NAS, poi `avanzamento` con `firmaDi`.

Un pezzo che non si legge (codici, piano, lavoro) diventa un avviso; una query di base fallita dà 404.

### Invarianti
- La risposta di una navigazione o di un gesto rifà i pannelli fuori banda; il corpo del pannello di destra (`#anteprima-corpo`)
  solo se `corpo_chiave` è cambiato: il PDF aperto resta aperto.
- `fascicolo_bom.go` e `fascicolo_dettaglio.go` sono puri: nessuna query.

### Test
L1 `fascicolo_test.go`, `fascicolo_b87b_test.go` (`TestLoStatoNellIndirizzo`, `TestLaChiaveDelCorpoDiceQuandoRifarlo`,
`TestLaBomVisualeDisegnaIlProdottoELeProposteSottoDiLui`). L4 `b87_db_test.go` (`TestLaSchermataDelFascicoloSiApreConTuttiIPannelli`,
`TestUnGestoDalFascicoloRifaIPannelliSenzaToccareLAnteprima`, `TestCentoAllegatiSiDisegnanoInMenoDiUnSecondo`),
`b87b_db_test.go:TestAprireIlFascicoloLoPreparaPerChiLavora`. L7 `e2e/fascicolo.py`, prove A, B, C, G, H. `GET …/vista` non ha un test HTTP.

### Stato dell'implementazione
Ogni richiesta ricarica tutta la schermata (più di 20 query e un `os.Stat` per allegato; provato con cento allegati sotto il
secondo). Nessun visore 3D: per STEP e DXF si mostra il riepilogo dell'analisi. Per `consultazione` l'apertura rilegge comunque
gli STEP e può accodare analisi.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| un parametro nuovo nell'indirizzo | `statoFascicolo`, `leggiStatoFascicolo`, `valori`, `Con` |
| un dato nuovo nella schermata | `fascicoloDati`, `caricaFascicolo` |
| una card della BOM | `fascicolo_bom.go:riempiComponente`, `fasc_carta` |
| il pannello di destra | `fascicolo_dettaglio.go`, `fasc_anteprima_testa`, `fasc_anteprima_corpo` |

## Modulo Fascicolo: i gesti sulla BOM e sui documenti (`fascicolo_rotte.go` per i POST, `fascicolo.go`, `fascicolo_conferma.go`)

### Rotte
Tutte POST, ruolo operatore o superiore (`autenticato` rifiuta i metodi che scrivono a `consultazione`).

| Percorso (sotto `/thread/{id}/fascicolo`) | Campi | Chiama |
|---|---|---|
| `/assegna` | `componente` (vuoto = sgancia), `documento`… e `proposta`…, `correggi_codice`, `scelta_<doc>`/`scelta`, `nuovo_riferimento_<doc>`, `motivo_<doc>` | `assegnaAlComponente`, in una transazione sua, con la RFQ bloccata per prima |
| `/componente/{cid}/codice` | `codice` | `correggiCodiceComponente` (`classificazione.CodiceAmmissibile`, RFQ bloccata per prima, FK differite al COMMIT) |
| `/componente/{cid}/modifica` · `collega` · `scollega` · `sposta` | `tipo` `rev` `descrizione` · `padre` `qta` · `padre` · `da` `a` `qta` | `fascicolo.ModificaComponente`, `Collega`, `Scollega`, `Sposta` (`qta` letta a 32 bit da `qtaDal`: vuota vale 1, fuori intervallo è un rifiuto) |
| `/componente/{cid}/archivia` · `rimuovi` | `motivo` | `ArchiviaComponente`, `RimuoviComponente`, poi `AggiornaTutteLeRimozioni` |
| `/componente/{cid}/step-strutturale` | `documento` | `ScegliStepStrutturale` |
| `/componente/{cid}/deroga`, `/deroga/{did}/revoca` | `tipo`, `motivo` | `ConcediDeroga`, `RevocaDeroga` (solo una deroga di questa RFQ) |
| `/componente/{cid}/deroga-struttura`, `/deroga-struttura/{did}/revoca` | `motivo` | `ConcediDerogaStruttura`, `RevocaDerogaStruttura` (solo una deroga strutturale di questa RFQ) |
| `/documento/{did}/sostituisci` · `annulla-sostituzione` | `vecchio`, `nuovo_riferimento`, `motivo` | `verificaSostituzione` + `Sostituisci` + `notaSostituzione` · `AnnullaSostituzione` |
| `/revisione/apri` · `abbandona` · `/congela` | `contesto`, `motivo` | `ApriRevisione` (D25c), `AbbandonaBozza`, `CongelaBom` |
| `/conferma` | `firma` (tutto il pronto), oppure `voce`… e `strutturale`… | `LeggiPianoFascicolo`, `selezioneDal`, `applicaPiano` (→ `confermaProposta`, `ScegliStepStrutturale`) |
| `/proposta/{pid}/decidi` | `tipo`, `codice`, `rev` | `DecidiPropostaDocumento` (fonte operatore; codice e revisione con `CodiceAmmissibile`/`RevAmmissibile`) |

### Flusso comune
`s.gesto`/`gestoPoi` (`proposte.go`): `ParseForm`, una transazione, e sul rifiuto «Niente è cambiato: » + `spiegaErrore`, che
traduce `rifiuto` (lo stesso tipo di `fascicolo.Rifiuto`: un alias), gli SQLSTATE da BOM01 a BOM05, il deadlock 40P01 («un'altra
operazione sulla stessa RFQ era in corso nello stesso momento: riprova») e i vincoli per nome. La risposta passa da
`threadFrammento`: dal Fascicolo (`HX-Current-URL`) è `rispondiFascicolo` (avviso + pannelli fuori banda), da altrove la pagina
della RFQ. `preparaGesto` blocca la RFQ (`BloccaThread`); il core fa lo stesso nei gesti che cambiano la struttura.

### Regole applicate (A1.4, A4.10, D18, D26, D32)
- Un codice o una revisione scritti a mano (correzione del codice, conferma di un file, decisione di una proposta) passano dalla
  stessa guardia dei nodi, dell'editor e del worker: `classificazione.CodiceAmmissibile` (non vuoto, al massimo `MaxCodice` = 40
  byte, senza spazi, anche Unicode, né caratteri di controllo) e `RevAmmissibile` (`MaxRev` = 10).
- La RFQ si blocca prima di componenti, documenti e proposte (`preparaGesto` in `assegnaAlComponente` e
  `correggiCodiceComponente`): è l'ordine del congelamento e degli altri gesti, e due gesti concorrenti si mettono in fila sulla
  RFQ invece di aspettarsi a vicenda.
- Il codice di un file segue quello del componente; un codice diverso passa solo con `correggi_codice=1`.
- Il percorso cambia solo per i documenti `in_coda` o in `errore`, con la copia bloccata (`BloccaCopiaPendente`): copia in corso ⇒
  «riprova fra poco»; documento `scritto` in un'altra cartella ⇒ rifiuto fino a B8.8. I lucchetti delle cartelle si prendono in
  ordine di percorso.
- Un file che entra in un componente con un documento corrente dello stesso tipo vuole «aggiungi» oppure il predecessore da
  sostituire; se il predecessore è lo STEP strutturale serve la risposta sul riferimento; una versione interna sostituisce solo con un motivo.
- Con la BOM congelata non si assegna. Uno STEP registrato in una baseline, lo STEP strutturale attuale o un documento già sostituito
  non cambia componente (`restaAlSuoComponente`).
- «Conferma Fascicolo» senza elenco vale solo se la `firma` è quella del piano che l'operatore aveva davanti; con un elenco, ogni
  voce deve essere ancora pronta.

### Test
L4 `fascicolo_db_test.go`, `b85_db_test.go`, `b87_db_test.go` (struttura, archivia/togli/ripristina, deroghe e STEP strutturale,
sostituzioni, congelamento e revisioni), `b87b_db_test.go` (`TestConfermaFascicoloPortaNelFascicoloSoloIlPianoVisto`,
`TestDecidereUnFileNonSiFaRiscrivereDaUnaLettura`), `a4_db_test.go`, `codice_tecnico_db_test.go`, `gesti_db_test.go`
(`TestIlCodiceScrittoAManoPassaDallaStessaGuardia`, `TestIGestiSulFascicoloBloccanoPrimaLaRFQ`). L1 `fascicolo_b87b_test.go`,
`gesti_test.go` (quantità a 32 bit, un rifiuto del core e del web è lo stesso, il deadlock dice «riprova»).
L7 `e2e/fascicolo.py`, prove D, E, F, I.

### Stato dell'implementazione
Lo spostamento sul NAS di un documento già scritto è B8.8. Dal Fascicolo v3 una struttura di STEP non è mai «pronta» (si conferma
nell'editor).

### Dove intervenire

| Voglio… | Apri |
|---|---|
| un gesto nuovo sulla working | la funzione nel core `fascicolo`, poi il gestore in `fascicolo_rotte.go` con `s.gesto` |
| le regole di assegnazione e del codice | `fascicolo.go` |
| la conferma del piano | `fascicolo_conferma.go` |
| la frase di un vincolo | `spiegaErrore` |

## Modulo Fascicolo: la vista Documenti (v3) e le note sui disegni (`fascicolo_documenti.go`, `fascicolo_gesti_v3.go`)

Template `fasc_docv`, `fasc_doc_sezione`, `fasc_doc_file`; `web/static/fascicolo.mjs`.

### Rotte

| Metodo e percorso (sotto `/thread/{id}/fascicolo`) | Ruolo | Che cosa fa |
|---|---|---|
| `GET /sezione` | tutti | `fascicoloSezione`: il pannello dell'elemento scelto (`fasc_doc_sezione_frammento`), con lo stato dell'indirizzo e `vista` forzata a Documenti |
| `POST /nota` | operatore | campi `componente` (facoltativo, della RFQ), `allegato` (della RFQ, un PDF già scaricato con sha256), `pagina` (1–10000), `x` e `y` (0–1), `testo` (non vuoto, al massimo `MaxTestoNota` = 2000). Ammessa anche con la BOM congelata |
| `POST /nota/{nid}/modifica` · `/elimina` | solo l'autore | l'UPDATE o il DELETE filtrano per autore e RFQ: niente righe ⇒ rifiuto |
| `POST /file/{pid}/generale` | operatore | campo `tipo` (`altro` o `capitolato`): sgancia la proposta, se era agganciata, e la conferma senza componente (`confermaProposta`) |

### Costruzione (`costruisciDocumenti`, pura)
- i gruppi: un prodotto per ogni radice di tipo finito, «Fuori dalla struttura», «Da associare», «Documenti della RFQ»;
- l'albero di ogni gruppo, ripercorso da capo: un sottoassieme condiviso si vede sotto ogni prodotto;
- il filmstrip (`tileDi`: il 2D confermato, poi quello in arrivo, poi «formato», poi «manca»);
- il JSON dell'indice per il visore (`indiceDoc`: elementi, note, revisione precedente, scelto, file, scrive, bloccata, gruppo);
- la sezione dell'elemento scelto (`fileDoc`: voce del piano, servibile, spostabile/`Fermo`, sostituibili);
- il riquadro degli STEP con una struttura da rivedere.

Un documento arrivato più volte si mostra una volta. Le note stanno sul contenuto (sha256) di un file, cioè su quella revisione; la
revisione nuova dice quante note c'erano sulla precedente («Vedi»). Il disegno lo disegna `fascicolo.mjs` con pdf.js nello stage
`#doc-stage` (`hx-preserve`); scegliere un altro componente cambia l'indirizzo con `replaceState` e chiede solo `/sezione`.

### Dati
`annotazione_pdf` (migrazione 0021): `InsertAnnotazione`, `ModificaAnnotazione`, `DeleteAnnotazione`, `ListAnnotazioniThread`; `AllegatoDelThread`.

### Test
L1 `documenti_v3_test.go`. L4 `v3_db_test.go` (`TestLeNoteSuiDisegni`, `TestUnFileEntraSenzaComponente`,
`TestLaSezioneEIDatiDellEditor`, `TestAssegnareIlFileAlPezzoConIlSuffisso`). L7 `e2e/fascicolo_v3.py`, prove J (visore,
filmstrip, scelta senza ricaricare), K (note, anche dopo F5), M (associazione dal pannello).

### Stato dell'implementazione
Un documento già scritto sul NAS non si sposta su un altro componente da qui (`spostabile`: B8.8).

### Dove intervenire

| Voglio… | Apri |
|---|---|
| i gruppi, l'albero, le miniature, l'indice del visore | `costruisciDocumenti`, `tileDi` |
| le regole delle note | `fascicolo_gesti_v3.go` e i CHECK della migrazione 0021 |
| il comportamento nel browser | `fascicolo.mjs` (vista Documenti, note) |

## Modulo Fascicolo: l'editor della struttura (`fascicolo_editor.go`, `fascicolo_gesti_v3.go:applicaStruttura`)

Nel browser: `fascicolo.mjs`, `class Editor`.

### Rotte

| Metodo e percorso (sotto `/thread/{id}/fascicolo`) | Ruolo | Che cosa fa |
|---|---|---|
| `GET /bom/dati?prodotto=<componente>&step=<allegato>` | tutti | risponde **in JSON** (`Cache-Control: no-store`), una delle eccezioni all'HTML elencate in testa; la chiede `fascicolo.mjs` con `fetch` |
| `POST /bom/applica` | operatore | campo `struttura`: JSON di `fascicolo.StrutturaVoluta`, al massimo 2 MiB |

### Dati dell'editor (`datiEditor`)

| Chiave | Che cos'è |
|---|---|
| `nodi` | `c:<componente>` per i componenti attivi; `p:<proposta>` per le proposte aperte, unite per codice in una carta sola (senza codice: una carta per proposta); `k:<codice>` per i codici nuovi trovati nella RFQ (`CandidatiDellaRfq`) |
| `archi` | gli archi working |
| `proposti` | gli archi proposti (allegato, chiavi `pk`/`fk`, file) |
| `rimozioni` | le rimozioni proposte |
| `prodotti`, `prodotto` | i prodotti finiti e quello di partenza: quello da cui si raggiunge la struttura dello `step` richiesto, altrimenti quello di una proposta qualsiasi, altrimenti il primo |
| `step`, `analisi`, `bloccata`, `scrive` | l'esito dello STEP del prodotto, le analisi in corso, la versione congelata, se chi guarda scrive |

### La conferma
`ApplicaStrutturaVoluta` blocca la RFQ e rifiuta con la BOM congelata; si accorge dei cambiamenti avvenuti nel frattempo (`visti`,
`relazioni_viste`). La risposta porta `HX-Trigger: bom-esito` con `{ok, testo}`, scritto tutto in ASCII da `eventoAscii`
(`\uXXXX`, coppie surrogate comprese). L'editor si chiude solo se la conferma è riuscita.

### Test
L4 `v3_db_test.go` (`TestLEditorPrendeLaPropostaDelloStep` e le varianti su sposta/condividi/togli, rifiuto senza scrivere,
codici trovati, scarti e rimozioni, cambiamenti nel frattempo, una carta per tutti gli STEP, nodo aperto con il codice di un
componente; `TestIDatiDellEditorSiApronoSulProdottoDelloStep`). L7 `e2e/fascicolo_v3.py`, prove L e N; `e2e/fascicolo.py`, prova D.

### La bozza
La bozza dell'editor vive nel `sessionStorage` della scheda, con la chiave `cockpit.editor.<utente>.<thread>.<prodotto>`:
l'utente lo dichiara la pagina (`data-utente` sulla radice `#fascicolo`), e chi entra dopo sulla stessa scheda non trova la bozza
di un altro. All'uscita (il form «esci» della testata) `layout.html` cancella tutte le chiavi `cockpit.editor.*`. Una bozza si
riprende solo sulla BOM con la stessa impronta. Prova: `statici_test.go:TestLaBozzaDellEditorEDiChiLaScrive` (L1).

### Stato dell'implementazione
Il limite di 40 caratteri del codice è ripetuto nel JS (`Editor.scriviCodice`, che conta i caratteri JavaScript; il server,
`CodiceAmmissibile`, conta i byte). Una sessione che scade senza passare da «esci» lascia la bozza nella scheda, sotto la chiave
del suo utente.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| i dati che l'editor riceve | `datiEditor` |
| le regole della conferma | `core/rfq/fascicolo/voluta.go` |
| i gesti nell'editor | `fascicolo.mjs`, `class Editor` |

## Modulo Fascicolo: preparazione e avanzamento (`fascicolo_preparazione.go`)

### Scopo
Aprire il Fascicolo lo prepara; finché c'è lavoro sui file, la testata lo dice e la pagina si aggiorna da sola.

### Flusso
`preparaFascicolo` parte dalla GET della pagina, per operatore o superiore, in tre transazioni separate i cui errori vanno solo
nel log (`inTransazione`): `fascicolo.AssicuraProdottiDellaRichiesta`; `fascicolo.PreparaFile` (con `Analizzatore`,
`maxCaricamentoEffettivo()`, `MaxPreparatiPerApertura` = 20 e `rileggiFatti` = `Pipeline.DopoCaricamento`); `rileggiAllApertura`.
`triage.go:preparaDopoLaDecisione` usa le stesse costanti e gli stessi aiuti.

`GET …/fascicolo/avanzamento?firma=…&giro=…` (tutti; non scrive): lo stato viene da `HX-Current-URL`; `firmaDi` riassume
componenti, proposte, versione bloccata, file, completezza, piano e lavoro; con la stessa firma il giro cresce e l'attesa si
allunga (`intervalliPoll`: 2, 2, 3, 3, 5, 5, 8, 10 s); con una firma diversa rifà testata, vista e piano, e il cassetto salvo
quello del NAS; il corpo del pannello solo se `corpo_chiave` è cambiato; senza lavoro l'elemento torna senza poll.

### Test
L1 `fascicolo_b87b_test.go` (il poll finché c'è lavoro, la firma della schermata). L4 `b87b_db_test.go`
(`TestAprireIlFascicoloLoPreparaPerChiLavora`, `TestIlPollRifaIPannelliSoloSeQualcosaECambiato`,
`TestCreareLaRfqFaDelCodiceUnProdottoEPreparaLoZip`, `TestAgganciareUnMessaggioPreparaIFileEAssicuraIProdotti`). L7 `e2e/fascicolo.py`, prova A.

### Stato dell'implementazione
La GET della pagina scrive (preparazione). Ogni poll ricarica tutta la schermata.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| che cosa si prepara | `core/rfq/fascicolo/preparazione.go` |
| la cadenza del poll | `intervalliPoll` |
| che cosa fa cambiare la firma | `firmaDi` |

## Modulo Fascicolo: «Importa dal NAS» (`fascicolo_nas.go`)

Template `fasc_nas`, `fasc_nas_voce`.

### Rotte

| Metodo e percorso (sotto `/thread/{id}/fascicolo`) | Ruolo | Che cosa fa |
|---|---|---|
| `GET /nas?nas=<cartella>&nas_cerca=<testo>` | operatore (controllo esplicito: 403 per `consultazione`) | rifà solo il cassetto (`fasc_cassetto_frammento`) |
| `POST /nas/importa` | operatore | campo `percorso`, relativo alla radice |

### Flusso
`percorsoNas` accetta solo percorsi relativi (niente assoluti o di unità, niente `..` che esce, `fs.ValidPath`); poi
`os.OpenRoot([nas].radice)` rifiuta anche i collegamenti che escono dalla radice. Si parte dalla cartella della RFQ se c'è già sul
NAS, altrimenti da quella del cliente, altrimenti dalla radice. L'elenco mostra al massimo 500 voci, senza i nomi che cominciano per
`.` o `~$`, prima le cartelle. La ricerca parte dalla cartella del cliente e vuole almeno 3 caratteri; si ferma a 20000 voci
guardate, 100 risultati o 3 s. Si importa un file regolare, non vuoto, entro `maxCaricamentoEffettivo`, e `tecnicoCaricabile`
(CAD 3D, disegni, DXF, PDF, TIF): `nelloStagingCaricato`, poi in una transazione `fascicolo.NotaInterna` → `RegistraCaricamento`
(con `PathInterno` = «NAS: <percorso>») → `Pipeline.DopoCaricamento`. Sul NAS della RFQ il file va solo con la conferma.

### Test
L1 `fascicolo_b87b_test.go:TestIPercorsiDelNasStannoSottoLaRadice`. L4 `b87b_db_test.go:TestImportaDalNasRestaSottoLaRadice`.
L'elenco e la ricerca via HTTP non sono provati.

### Stato dell'implementazione
Dopo una ricerca i collegamenti del cassetto perdono lo stato della pagina (`vista`, `filtro`…): il form manda solo `nas` e
`nas_cerca`. Le risposte di `/nas` mostrano «Da verificare (0)» e «Rivedi (0)» perché il piano non viene calcolato. Il NAS si
legge direttamente con `os`, non con `platform/storage/nas`.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| i limiti di elenco e ricerca | le costanti in testa a `fascicolo_nas.go` |
| i tipi importabili | `caricamento.go:tecnicoCaricabile` |

## Modulo Allegati, anteprima PDF, caricamento interno (`allegati.go`, `anteprima.go`, `caricamento.go`)

Template `frammenti.html` (`allegati_tabella`, `allegato_riga`, `conferma_form`), `fasc_file_testa`, `fasc_anteprima_corpo`.

### Rotte
Tutte `autenticato`; i POST vogliono il ruolo operatore o superiore.

| Metodo e percorso | Campi | Effetto |
|---|---|---|
| `POST /messaggio/{id}/scarica` | `allegato_id`… | solo un messaggio agganciato a una RFQ; in una transazione `accodaDownload` → `coda.AccodaStage` (la guardia del doppio download sta lì); la copia da cui scaricare la sceglie `coda.CopiaPerDownload` |
| `POST /allegato/{id}/riscarica` | `ritorna_thread` | lo stesso, per un allegato solo |
| `POST /proposta/{id}/conferma` | `tipo`, `codice`, `rev`, `nota`, `componente_id`, `correggi_codice`, `scelta`, `nuovo_riferimento`, `motivo`, `ritorna_thread` | la RFQ del messaggio `FOR KEY SHARE`, poi `BloccaProposta` + `confermaProposta` |
| `POST /proposta/{id}/scarta` | `rumore`, `ritorna_thread` | la proposta scartata e, con `rumore=1`, lo sha256 ricordato per il dominio del mittente, nella stessa transazione |
| `GET /allegato/{id}/anteprima` | — | i byte del PDF (tutti i ruoli); 400 a una richiesta htmx |
| `POST /thread/{id}/fascicolo/carica` | multipart `file` | la versione interna |

### `confermaProposta` (la chiamano `conferma`, `applicaPiano` e `fileGenerale`)
1. Proposta aperta, tipo diverso da `da_determinare`, codice e revisione scritti ammissibili (`classificazione.CodiceAmmissibile`,
   `RevAmmissibile`), messaggio agganciato, sha256 e staging presenti, RFQ con una cartella.
2. Contenuto identico a un documento della RFQ: si registra solo la provenienza e la proposta diventa `duplicato` (un errore in
   una delle due scritture annulla il gesto); se quel documento è stato sostituito, rifiuto (`identicoAUnaRevisioneSostituita`).
3. Il componente deve essere della stessa RFQ; il codice è quello del componente (o `correggi_codice`); con la BOM congelata si
   rifiuta; con un corrente dello stesso tipo vale `verificaScelta`. Un file tecnico (`documenti.Tecnico`) senza codice si rifiuta.
4. Il percorso: `documenti.CartellaDocumento` + `ScegliPercorso(NomeSulNas…)`.
5. `InsertDocumento` (in `nome_file` il nome originale), la provenienza, la proposta confermata, `applicaScelta`, `coda.AccodaCopia`.
   Con `nas_scrittura` spenta il documento resta `in_coda` e la frase dice «IN ATTESA».

`pannelloConAvviso` risponde con `HX-Trigger: inbox-aggiorna`; con `ritorna_thread` risponde come la pagina della RFQ (o come il
Fascicolo, se il gesto viene da lì).

L'ordine dei lucchetti in `conferma`: prima la RFQ del messaggio dell'allegato (`FOR KEY SHARE`, lo stesso lucchetto che la
scrittura del documento prenderebbe dopo), poi la proposta. Così una conferma aspetta un congelamento o un gesto sulla struttura
senza tenere già la proposta; due conferme dello stesso file restano una corsa che decide il vincolo (thread, sha256).

### Anteprima
Una richiesta htmx (`HX-Request` con qualunque valore) si rifiuta con 400 prima di leggere l'allegato: i byte di un file si aprono
solo come documento (l'iframe, pdf.js, una scheda nuova), mai fra i pezzi della pagina.
Nell'URL c'è solo l'id; si serve solo ciò che comincia per `%PDF-`. Prima lo staging, se `path_staging` è assoluto e sta sotto
`Server.Staging`; poi il NAS: il documento `scritto` con lo stesso sha256 nella RFQ (anomalia aperta ⇒ 409; NAS che non risponde ⇒
503; file mancante, illeggibile o cartella ⇒ `documenti.Segnala` e 409; dimensione diversa o file toccato dopo la scrittura ⇒ una
rilettura con `VerificaFileAperto`). Intestazioni: `application/pdf`; `Content-Disposition: inline` con `filename=` ASCII e
`filename*=UTF-8''…`; `nosniff`; `Content-Security-Policy: sandbox; default-src 'none'`; `ETag` = sha256; `http.ServeContent`
(Range, If-Range).

### Caricamento interno
Servono `Pipeline` e `Staging`; limite `MaxBytesReader` pari al massimo + 1 MiB; `tecnicoCaricabile`. `nelloStagingCaricato`: una
`.parte` con un token, lo sha256 calcolato mentre si scrive, il rifiuto di un file vuoto o troppo grande, poi `Promuovi` in
`_contenuti` (o si butta la parte se il contenuto c'è già). Poi, in una transazione: `NotaInterna` (il messaggio del canale `nota`,
uno per RFQ) → `RegistraCaricamento` (allegato con origine `manuale`) → `Pipeline.DopoCaricamento`. Ammesso anche con la BOM
congelata; diventa un documento solo con la conferma.

### Test
L1 `anteprima_test.go` (anche `TestLAnteprimaNonSiInnestaNellaPagina`). L4 `anteprima_db_test.go`, `a4_db_test.go`,
`codice_tecnico_db_test.go`, `decisioni_db_test.go:TestDueConfermeConcorrentiDecidonoUnaSolaVolta`, `chiavecopia_db_test.go`,
`fascicolo_db_test.go`, `b85_db_test.go:TestLaConfermaConComponenteVuoleLaScelta`, `b87_db_test.go` (versioni interne),
`sicurezza_db_test.go:TestLAnteprimaRifiutaLeRichiesteHtmx`, `gesti_db_test.go` (codice della conferma, ordine dei lucchetti).
L7 `e2e/anteprima_pdf.py`.
`riscarica` non ha un test HTTP.

### Stato dell'implementazione
Il ritorno a una revisione più vecchia della precedente non è gestito. La GET dell'anteprima può scrivere anomalie.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| che cosa succede quando si conferma | `allegati.go:confermaProposta` |
| da dove si servono i byte di un PDF | `anteprima.go:sorgenteAnteprima` |
| che cosa si può caricare a mano | `caricamento.go` |

## Le prove nel browser (`e2e/`)

| Script | Lanciato da | Prove |
|---|---|---|
| `inbox_quadranti.py` | `inbox_browser_test.go` | l'Inbox a quadranti (A–G) |
| `richieste.py` | `richieste_browser_test.go` | la pagina Richieste (A–F) |
| `anteprima_pdf.py` | `anteprima_browser_test.go` | a–g: il pulsante, la scheda che si apre, il Range servito a pezzi, il secondo giro senza byte |
| `fascicolo.py` | `fascicolo_browser_test.go` | A–I: Struttura BOM, dettaglio, PDF, nodo ed editor senza F5, tre file con un gesto, completezza, cento file, cassetti, Rivedi e Conferma |
| `fascicolo_v3.py` | `fascicolo_v3_browser_test.go` | J–N: vista Documenti con pdf.js, note, editor, associazione dal pannello, editor senza STEP; fallisce anche su un errore JavaScript o su una risposta ≥ 400 |

Si lanciano con `go test -tags "integrazione browser" -count=1 -run TestL7 ./internal/transport/web/`, con `COCKPIT_TEST_DSN`.
Serve Playwright per Python con Edge; senza Python o Playwright il test è SKIP. Il test Go sceglie le prove con `--prove`; con
`COCKPIT_FOTO_DIR` lo script salva le fotografie della pagina.

## Leggi anche

`internal/transport/README.md` (le rotte in tabella), `web/README.md` (template e statici), `internal/core/README.md` (chi fa che cosa dietro le rotte), `internal/core/inbox/lettura/README.md` (il corpo di una mail pronto da leggere), `internal/README.md` (livelli di test).
