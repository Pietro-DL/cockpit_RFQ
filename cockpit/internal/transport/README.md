# `transport/` — le due facce HTTP

## Scopo

Le rotte del browser e quelle dei worker. Traduzione fra HTTP e il resto del server: leggere la richiesta,
controllare chi chiama, chiamare il pezzo giusto, rendere la risposta.

Questo README tiene le regole comuni, la tabella completa delle rotte e le eccezioni ancora aperte; il
dettaglio (file, dati, flussi, test, stato) è nei README dei due package.

## Non appartiene qui

Le decisioni di dominio. Quelle che oggi ci stanno ancora sono elencate sotto, in «Eccezioni aperte», come
debito dichiarato e non come esempio da seguire.

## Package posseduti

| Package | Che cosa fa | README |
|---|---|---|
| `web` | la UI per l'operatore: sessione, ruoli, Inbox a quadranti e triage, pagina della RFQ, pagina Richieste, Fascicolo (schermata, gesti, vista Documenti, note, editor, preparazione, «Importa dal NAS», caricamento), anteprima dei PDF, schermate dell'amministratore, pacchetto delle postazioni. Circa 36 file, divisi in moduli | [README](web/README.md) |
| `workerapi` | le otto rotte `/api/v1/*` dei worker: autenticazione per token individuale e prova di vita, claim, battito, result, ingest dei lotti, upload e download legati al tentativo; e la **pipeline dopo lo staging** (proposta dal nome, analisi, estrazione degli archivi, strutture degli STEP nelle RFQ), riusata dal Fascicolo e dall'esecutore | [README](workerapi/README.md) |

Il front-end (template, CSS, `fascicolo.mjs`, pdf.js) è in [`web/README.md`](../../web/README.md).

## Dipendenze consentite

`core`, `ai`, `platform`. Quelle che esistono davvero:

- `web` → `ai/agente`; `core/inbox/{classificazione,ingest,lettura}`, `core/registro/{anagrafica,fornitori,regole}`,
  `core/rfq/{documenti,fascicolo}`; `platform/{coda,contratti/worker,db,fondazioni,rete,storage/nas,storage/staging}`.
- `workerapi` → `core/inbox/{classificazione,ingest}`, `core/registro/regole`, `core/rfq/{documenti,fascicolo}`;
  `platform/{coda,contratti/worker,db,rete,storage/archivio,storage/nas,storage/staging}`.

`web` **non** importa `workerapi`: il Fascicolo riceve la pipeline come interfaccia (`web.Server.Pipeline`), e
chi la collega è `app/runtime/ascolto.go:Ascolta`. Lo fanno solo i test L4 di `web`. Niente in `core` o
`platform` importa `transport`.

## Regole comuni

- **Due autenticazioni**, mai mescolate. Il browser: cookie di sessione `cockpit_sess` (HttpOnly,
  SameSite=Lax, Secure con il TLS, 12 ore), sessione in database; una richiesta htmx senza sessione riceve
  401 con `HX-Redirect: /login`, le altre 302. I worker: `X-Cockpit-Token` individuale, che **identifica**
  (nome, tipo, postazione e caselle vengono dalla credenziale, non dal corpo); 401 se manca, non esiste, è di
  due credenziali o non è mai stato generato.
- **Ruoli**: consultazione < operatore < tecnico < admin. `/admin/*` passa da `soloAdmin`; tutto il resto è
  `autenticato`, con una regola sola per metodo: un metodo che scrive vuole almeno `operatore`
  (`server.go:autenticato`).
- **CSRF**: `http.CrossOriginProtection` (sulle intestazioni `Origin` / `Sec-Fetch-Site`, non un token)
  avvolge tutto il mux, rotte dei worker comprese (`web.ProtezioneCSRF`).
- **`web` risponde HTML** (pagina intera o frammento htmx, secondo `HX-Request`), con eccezioni dichiarate:
  JSON per `GET /healthz` e `GET /thread/{id}/fascicolo/bom/dati`; PDF per `GET /allegato/{id}/anteprima`;
  zip per il pacchetto delle postazioni; i file statici; 204 senza corpo per un poll che non ha niente di
  nuovo e per i gesti che rispondono con `HX-Refresh`; testo semplice per alcuni errori. **`workerapi`
  risponde solo JSON** (o 204).
- **Pagina o frammento**: alcune rotte esistono solo come frammento (`/messaggio/{id}/triage`, `/censisci`,
  `/anagrafica/buyer`, `/thread/cerca`, `/stato/worker`, `/inbox/sync-storico/stato`,
  `/richieste/{id}/prodotti`, le parti del Fascicolo); una richiesta non htmx a `/messaggio/{id}` viene
  rimandata all'Inbox, nel quadrante del messaggio e con il messaggio selezionato.
- **Mai HTML della mail nella pagina**: il corpo passa da `core/inbox/lettura` e dall'escape dei template;
  nessun template usa `template.HTML` per testo che viene da fuori.
- **Che cosa entra fra i pezzi della pagina**: il parametro `sel` dell'Inbox e della pagina della RFQ passa
  solo se è un UUID (riscritto dal server); `GET /allegato/{id}/anteprima` rifiuta con 400 una richiesta
  htmx (un file si apre come documento, mai dentro la pagina); `layout.html` ha un ascoltatore globale
  `htmx:beforeSwap` che non innesta una risposta 2xx che non sia `text/html`; `stato` e `origine` delle
  schermate della coda e degli scarti tornano nel poll solo se riconosciuti.
- **Una transazione per gesto**, e i gesti su una RFQ bloccano prima la RFQ; uno stallo (40P01) diventa
  «riprova».

## Le rotte del browser (`web`)

Le monta `web.Server.Registra`, che tiene le rotte senza area e chiama i `registra*` delle aree; le rotte
del Fascicolo sono montate da `routes_rfq.go:registraRFQ` attraverso `registraProposte`, `registraCodici`,
`registraFascicolo`, `registraConferma` e `registraFascicoloV3`. Autenticazione: `—` = nessuna,
`A` = `autenticato`, `Adm` = `soloAdmin`.

**Senza area** (`server.go`)

| Rotta | Auth | Effetto |
|---|---|---|
| `GET /static/` | — | file statici; quelli con l'impronta `?v=` e pdf.js in cache per un anno; una cartella risponde 404, mai l'elenco |
| `GET /healthz` | — | JSON: stato del database, del NAS, versione dello schema |
| `GET /login`, `POST /login`, `POST /logout` | — | login con sigla e password (bcrypt), sessione, postazione abbinata per IP |
| `GET /` | A | rimanda a `/inbox` |

**Inbox, messaggi, triage** (`routes_inbox.go:registraInbox`)

| Rotta | Effetto | Gestore |
|---|---|---|
| `GET /inbox` | i quadranti (`q`, `dir`, `filtro`, `casella`, `sel`); la pagina intera segna la visita e accoda l'aggiornamento di apertura, una volta per sessione | `routes_inbox.go:inbox` |
| `POST /inbox/aggiorna` | «Aggiorna ora»: un `sync_outlook` per casella | `inbox_viva.go:aggiornaOra` |
| `POST /inbox/sync-storico` · `GET /inbox/sync-storico/stato` | «Carica precedenti» (due giorni per clic e per casella, priorità 9) e il suo stato (204 se non c'è niente) | `routes_inbox.go:syncStorico`, `syncStoricoStato` |
| `GET /stato/worker` | la testata: postazione, caselle, worker | `routes_inbox.go:statoWorker` |
| `GET /messaggio/{id}` | il pannello: corpo da `lettura`, allegati, proposte, analisi AI | `routes_inbox.go:messaggio` |
| `POST /messaggio/{id}/apri` · `/bozza` · `/letto` | job interattivi per la postazione della sessione; `bozza` vuole la capacità `bozze`, `letto` `outlook_scrittura` | `routes_inbox.go:apriInOutlook`, `bozza`, `segnaLetto` |
| `POST /messaggio/{id}/analizza` | `analizza_messaggio_ai`, solo se l'agente è acceso per quella casella | `agente.go:chiediAnalisi` |
| `GET /messaggio/{id}/triage` | il form di triage (i codici della sola storia citata non sono pre-spuntati) | `triage.go:triageForm` |
| `POST /messaggio/{id}/rfq` | Nuova RFQ: `thread_offerta` (la seconda cartella con lo stesso nome prende « (2)»), identificativi, `crea_cartella_thread`, download scelti, preparazione del Fascicolo; rifiutata per la posta di un fornitore | `triage.go:nuovaRFQ` |
| `POST /messaggio/{id}/aggancia` · `/ignora` | aggancio a una RFQ esistente (con la preparazione) o «ignora» | `triage.go:agganciaEsistente`, `ignora` |
| `GET/POST /messaggio/{id}/censisci` | «Censisci come fornitore / cliente» e il ritriage mirato | `censisci.go:censisciForm`, `censisci` |
| `POST /messaggio/{id}/risposta-fornitore` · `/richiesta-fornitore` | «è la risposta a questa richiesta», «è la richiesta mandata a mano» (7B) | `richieste.go:rispostaFornitore`, `richiestaFornitoreManuale` |
| `GET /anagrafica/buyer` · `GET /thread/cerca` | frammenti del triage: i buyer del cliente, la ricerca di una RFQ | `triage.go:buyerSelect`, `cercaThread` |
| `POST /messaggio/{id}/scarica` · `POST /allegato/{id}/riscarica` | `stage_allegato` (priorità 1), salvo contenuto già in staging | `allegati.go:scarica`, `riscarica` |
| `GET /allegato/{id}/anteprima` | i byte del PDF, dallo staging (solo sotto la sua radice) o dal NAS; `Range`/`ETag`; CSP `sandbox`; 409 con un'anomalia aperta, 503 con il NAS assente, 400 a una richiesta htmx | `anteprima.go:anteprima` |

**Pagina della RFQ, proposte, Richieste** (`routes_rfq.go:registraRFQ`)

| Rotta | Effetto | Gestore |
|---|---|---|
| `GET /thread/{id}` | la pagina della RFQ; all'apertura rilegge gli STEP (`RianalizzaRfq`, fino a 5 accodati) | `thread.go:thread` |
| `POST /thread/{id}/riprova-copie` | riaccoda `copia_nas` per i documenti in coda | `thread.go:riprovaCopie` |
| `POST /thread/{id}/richiesta` · `/richiesta/{rid}/annulla` | la richiesta a un fornitore e la sua bozza marcata (7B). **Nessun form la chiama più**: tolta di proposito, non va reintrodotta prima del modello delle lavorazioni per componente | `richieste.go:nuovaRichiestaFornitore`, `annullaRichiestaFornitore` |
| `POST /proposta/{id}/conferma` · `/scarta` | la conferma: `documento` + `copia_nas` (con la scelta «aggiungi / sostituisce»); non crea componenti | `allegati.go:conferma`, `scarta` |
| `POST /thread/{id}/fascicolo/assegna` | documenti e proposte a un componente, o sganciati; codice diverso solo con `correggi_codice` | `fascicolo.go:assegna` |
| `POST /thread/{id}/fascicolo/componente/{cid}/codice` | corregge il codice di un componente (validato come altrove) e lo porta su documenti e proposte | `fascicolo.go:correggiCodice` |
| `GET /cruscotto` | 303 verso `/richieste` | `routes_rfq.go:cruscotto` |
| `GET /richieste` · `GET /richieste/{id}/prodotti` | la pagina Richieste (una card per RFQ, filtri nell'indirizzo, poll con firma: 204 se niente è cambiato) e «+ N altri» (404 senza prodotti) | `panoramica.go:richieste`, `richiestaProdotti` |

**Fascicolo: proposte e codici** (`proposte.go:registraProposte`, `codici.go:registraCodici`)

| Rotta | Effetto |
|---|---|
| `POST /thread/{id}/fascicolo/rianalizza` | rilegge gli STEP della RFQ, fino a 20 analisi accodate |
| `POST …/fascicolo/nodo/{pid}/accetta` · `/scarta` · `/codice` | un nodo proposto diventa componente, si scarta, o riceve il codice dall'operatore |
| `POST …/fascicolo/relazione/accetta` · `/scarta` | un arco proposto (mai se chiude un ciclo) |
| `POST …/fascicolo/file/{aid}/accetta` | tutto il file, o il sottoalbero di un nodo; tutto o niente |
| `POST …/fascicolo/rimozione/accetta` · `/scarta` | una rimozione proposta dallo STEP strutturale |
| `POST …/fascicolo/codice/aggiungi` | «+ Prodotto / + Assieme / + Particolare» per un codice davvero nuovo |
| `POST …/fascicolo/componente/{cid}/ripristina` | un componente archiviato torna nella working |

**Fascicolo: schermata e gesti** (`fascicolo_rotte.go:registraFascicolo`)

| Rotta | Effetto |
|---|---|
| `GET /thread/{id}/fascicolo` | la schermata (lo stato è l'indirizzo); per chi è almeno `operatore` prima prepara il Fascicolo (B8.7b) |
| `GET …/fascicolo/parti` · `/anteprima` · `/vista` | i pannelli fuori banda, il pannello di destra, la sola area centrale; non scrivono |
| `POST …/componente/{cid}/modifica` · `/collega` · `/scollega` · `/sposta` | tipo, revisione, descrizione; gli archi a mano, con i cicli rifiutati e la quantità a 32 bit |
| `POST …/componente/{cid}/archivia` · `/rimuovi` | archiviare (con motivo; chiude le rimozioni aperte del suo STEP) o togliere un componente senza storia |
| `POST …/componente/{cid}/step-strutturale` | lo STEP corrente del prodotto finito diventa la sua distinta |
| `POST …/componente/{cid}/deroga` · `POST …/deroga/{did}/revoca` | la deroga del fabbisogno e la sua revoca |
| `POST …/componente/{cid}/deroga-struttura` · `POST …/deroga-struttura/{did}/revoca` | la deroga strutturale e la sua revoca, solo dentro la sua RFQ |
| `POST …/documento/{did}/sostituisci` · `/annulla-sostituzione` | la catena delle revisioni di un documento |
| `POST …/revisione/apri` · `/abbandona` · `POST …/congela` | le versioni della BOM e il congelamento con il gate |
| `POST …/fascicolo/carica` | «Carica nuova versione interna» (multipart, `max_upload_mb`), gestore `caricamento.go:caricaVersioneInterna` |

**Fascicolo: preparazione, NAS, conferma** (`fascicolo_conferma.go:registraConferma`)

| Rotta | Effetto | Gestore |
|---|---|---|
| `GET …/fascicolo/avanzamento` | il poll della preparazione; non scrive | `fascicolo_preparazione.go:fascicoloAvanzamento` |
| `GET …/fascicolo/nas` · `POST …/fascicolo/nas/importa` | sfogliare e cercare sotto `[nas].radice`; importare un file nello staging (l'originale non si tocca) | `fascicolo_nas.go:fascicoloNas`, `importaDalNas` |
| `POST …/fascicolo/conferma` | «Conferma Fascicolo»: il piano ricalcolato in transazione, tutto o niente; parte la copia sul NAS | `fascicolo_conferma.go:confermaFascicolo` |
| `POST …/fascicolo/proposta/{pid}/decidi` | tipo, codice e revisione di una proposta dal pannello | `fascicolo_conferma.go:decidiProposta` |

**Fascicolo v3** (`fascicolo_gesti_v3.go:registraFascicoloV3`)

| Rotta | Effetto | Gestore |
|---|---|---|
| `POST …/fascicolo/nota` · `/nota/{nid}/modifica` · `/nota/{nid}/elimina` | le note sui disegni (su un PDF scaricato della RFQ; modifica e cancellazione solo dell'autore) | `fascicolo_gesti_v3.go:nuovaNota`, `modificaNota`, `eliminaNota` |
| `POST …/fascicolo/bom/applica` | l'editor della struttura: `struttura` in JSON nel form, `ApplicaStrutturaVoluta` in una transazione, esito in `HX-Trigger` | `fascicolo_gesti_v3.go:applicaStruttura` |
| `POST …/fascicolo/file/{pid}/generale` | la proposta `{pid}` confermata come documento generale (`altro`, `capitolato`), senza componente | `fascicolo_gesti_v3.go:fileGenerale` |
| `GET …/fascicolo/bom/dati` | JSON per l'editor, `Cache-Control: no-store` | `fascicolo_editor.go:fascicoloDatiEditor` |
| `GET …/fascicolo/sezione` | la sezione del componente scelto nella vista Documenti | `fascicolo_documenti.go:fascicoloSezione` |

**Postazioni** (`postazioni_admin.go:registraPostazioni`)

| Rotta | Auth | Effetto |
|---|---|---|
| `POST /sessione/postazione` | A | la postazione della sessione scelta a mano (`postazione.go:scegliPostazione`) |
| `GET /admin/postazioni` · `POST /admin/postazioni/{host}/pacchetto` | Adm | le postazioni e il pacchetto zip di un PC (token nuovi: quelli di prima smettono di valere) |

**Amministrazione** (`routes_admin.go:registraAdmin`, tutte `soloAdmin`)

| Rotta | Effetto |
|---|---|
| `GET /admin/job` · `POST /admin/job/{id}/riaccoda` · `/annulla` | la coda; riaccodare un job fallito o annullato senza un gemello pendente |
| `GET /admin/nas` · `POST /admin/nas/controlla` · `/{id}/riaccoda` · `/{id}/allinea` | Integrità NAS (`integrita_admin.go`) |
| `GET /admin/scarti` · `POST /admin/scarti/{id}/riprova` | gli scarti dell'ingest e il replay |
| `GET /admin/anagrafica` · `/articoli` · `POST /admin/anagrafica` · `/{id}` · `/{id}/regole` · `/{id}/regole/form` · `/{id}/prova` · `/{id}/dominio` · `/{id}/dominio/elimina` | i clienti, le regole di riconoscimento (JSON e form), il banco di prova, i domini (`anagrafica.go`, `anagrafica_admin.go`); `/articoli` è un segnaposto |
| `POST /admin/anagrafica/{id}/buyer` · `/buyer/elimina` · `/fabbisogno` · `/fabbisogno/elimina` | i buyer (si elimina solo un buyer di quel cliente, e mai uno citato da una proposta di triage) e il fabbisogno documentale (`anagrafica_admin.go`) |
| `POST /admin/anagrafica/{id}/convenzione` · `/convenzione/elimina` · `/convenzione/attiva` · `/qualifica` · `/qualifica/elimina` · `/lavorazioni/prova` | convenzioni di codice e qualifiche dei fornitori per il cliente (`convenzioni_admin.go`) |
| `GET /admin/fornitori` · `POST /admin/fornitori` · `GET/POST /admin/fornitori/importa` · `POST /admin/fornitori/{id}` · `/{id}/dominio` · `/{id}/dominio/elimina` · `/{id}/contatto` · `/{id}/contatto/elimina` · `/{id}/lavorazioni` · `/{id}/qualifica` · `/{id}/qualifica/elimina` | i fornitori e l'import del seme (`fornitori_admin.go`). Stanno sotto `/admin/fornitori` e non sotto `/admin/anagrafica/fornitori/{id}`: il mux considera quel pattern in conflitto con `/admin/anagrafica/{id}/regole` |

## Le rotte dei worker (`workerapi`)

Ogni rotta passa da `auth` (token → credenziale → prova di vita: `worker_presenza.ultimo_contatto`) e
risponde 401 se il token non identifica nessuno.

| Rotta | Che cosa fa il server | Risposte |
|---|---|---|
| `POST /api/v1/jobs/claim` | interseca `caselle_aperte` con la credenziale, registra presenza e `casella_store`, `coda.Claim` con long-poll (un'attesa ≤ 0 o > 25 s vale 20), esclude i tipi che le capacità bloccano | 200, 204, 400, 403, 500 |
| `GET /api/v1/worker/caselle` | le caselle della credenziale | 200, 403, 500 |
| `POST /api/v1/jobs/{id}/heartbeat` | rinnova il lease, se il tentativo vale | 204, 400, 403, 409, 500 |
| `POST /api/v1/jobs/{id}/result` | applica l'esito per tipo; un risultato di analisi per un allegato diverso da quello del job è un 422 definitivo | 204, 400, 403, 404, 409, 422, 500 |
| `POST /api/v1/ingest/messaggi` | `ingest.Ingerisci`, solo per una casella autorizzata dalla credenziale e solo con il tentativo di un `sync_outlook` / `rileggi_elemento` di quella casella (`ErrLottoNonDelJob`); un lotto senza casella prende quella del job | 200, 400, 403, 409, 422, 500 |
| `PUT /api/v1/allegati/{id}/file` | `_parti/<allegato>.parte.<lease_token>`, sha256 verificato, promozione solo dal result valido | 204, 400, 403, 409, 413, 422, 500 |
| `GET /api/v1/allegati/{id}/contenuto` | i byte di un allegato al solo tentativo valido di un `analizza_allegato` di quell'allegato, e solo da dentro lo staging | 200, 400, 403, 409, 410, 422, 500 |
| `GET /api/v1/sync/cursori` | i cursori per (casella, cartella); nessun worker lo chiama | 200, 500 |

## Eccezioni aperte

Logica di dominio che vive ancora nel livello HTTP. Sono dichiarate perché esistono, non perché vanno bene;
la destinazione è una decisione dell'architetto.

In `web`:
- la conferma di una proposta (`allegati.go:confermaProposta`) e l'applicazione del piano di «Conferma
  Fascicolo» (`fascicolo_conferma.go:applicaPiano`);
- l'assegnazione a un componente e la correzione del codice, con il percorso dei documenti che segue il
  codice (`fascicolo.go:assegnaAlComponente`, `correggiCodiceComponente`, `ripercorri`, `bloccaCartelle`);
- la nuova RFQ, l'aggancio e «ignora», con il log delle decisioni (`triage.go:nuovaRFQ`, `agganciaEsistente`,
  `agganciaMessaggioAThread`, `ignora`, `logDecisione`) e il nome libero della cartella (`triage.go:cartellaLibera`);
- le scritture dell'anagrafica (`anagrafica.go:CreaCliente`, `AggiungiDominio`, `censisci.go:censisci`,
  `AggiungiDominioFornitore`);
- la risposta di un fornitore a una richiesta (`richieste.go:rispostaFornitore`);
- le finestre di «Carica precedenti» (`routes_inbox.go:finestraStorico`).

In `workerapi`, la pipeline dopo lo staging: `applicaRisultato`, `dopoStaging`, `propostaDaAnalisi`,
`scriviProposta`, `motoreDelFile`, `codiceRevSicuri`, `applicaFattiEsistenti`, `strutturaNelleRfq`,
`DopoCaricamento` (usata da `web` come `Pipeline`) e `archivi.go:EstraiArchivio` (usata dall'esecutore come
`Estrattore`).

Altri debiti noti: il controllo «dentro lo staging» (`nelloStaging`) è scritto due volte, in
`web/anteprima.go` e in `workerapi/contenuto.go`.

## Invarianti

- Il token **identifica**: nome, tipo, postazione e caselle vengono dalla credenziale, non dal corpo della
  richiesta.
- Un result, un battito, un upload, un download e un lotto valgono solo per il tentativo vivo; un file
  caricato resta in `_parti` finché il result non lo promuove.
- Un lotto scrive solo nella casella del job che lo consegna, e solo se la credenziale la serve.
- `consultazione` non usa mai un metodo che scrive; `/admin/*` è dell'amministratore.
- Nessuna pagina innesta HTML della mail, né una risposta che non sia HTML.

## Effetti collaterali

Cookie di sessione, righe in `sessione` e `worker_presenza`, file in `_parti` e nello staging, job accodati,
e tutto ciò che scrivono i package di `core` chiamati dai gestori. Alcune letture hanno effetti: la prima
apertura dell'Inbox accoda un aggiornamento, `GET /thread/{id}` rilegge gli STEP, `GET /thread/{id}/fascicolo`
prepara il Fascicolo (per chi è almeno `operatore`), l'anteprima può aprire un'anomalia.

## Test

L1 per le parti pure (template, ruoli, statici, corpo delle mail, gesti, triage); L4 per i gestori
(`-tags integrazione`, `./internal/transport/web/` e `./internal/transport/workerapi/`), compresi i test
W4/W10/W11 sull'autenticazione e i cinque TestE2E che fanno girare i worker Python veri; L7 nel browser
(`-tags "integrazione browser"`) per Inbox, anteprima dei PDF, pagina Richieste e Fascicolo. Il dettaglio
è nei README dei package.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una rotta nuova | il `registra*` dell'area, il gestore nel file dell'area, il template; poi la tabella qui sopra |
| un campo nuovo nel contratto con i worker | `platform/contratti/worker/tipi.go`, `workers/contratti.py`, `genera_contratti.py`, i test L3 |
| capire che cosa succede quando un job finisce | `workerapi/workerapi.go:applicaRisultato` |
| capire perché un lotto è rifiutato con 403 | `workerapi/workerapi.go:ingest`, `core/inbox/ingest/ingest.go:lottoDelJob` |
| capire perché un messaggio sta in quel quadrante | la colonna `quadrante` di `v_inbox`, `web/routes_inbox.go:quadranteValido` |
| capire perché una risposta di un fornitore non chiude la richiesta | `web/richieste.go:rispostaFornitore` |

## Leggi anche

`internal/README.md`, `core/README.md`, `platform/README.md`, `web/README.md`, `workerapi/README.md`,
`../../web/README.md`, `../../workers/workers_README.md`.
