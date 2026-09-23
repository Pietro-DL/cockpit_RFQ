# `transport/` — le due facce HTTP

## Scopo

Le rotte del browser e quelle dei worker. Traduzione fra HTTP e il resto del server: leggere la richiesta,
controllare chi chiama, chiamare il pezzo giusto, rendere la risposta.

## Non appartiene qui

Le decisioni di dominio. Quelle che oggi ci stanno ancora sono elencate fra gli invarianti come **eccezioni
aperte**, non come esempio da seguire: `workerapi/archivi.go`, `applicaRisultato`, `propostaDaAnalisi`.

## Package posseduti

| Package | Che cosa fa |
|---|---|
| `web` | la UI, in file per area: `server.go` (Server, template, `Registra`, sessione, rendering, avvisi), `routes_inbox.go`, `routes_rfq.go`, `routes_admin.go`, più i file già per tema (`triage.go`, `censisci.go`, `thread.go`, `richieste.go`, `allegati.go`, `postazione.go`, `postazioni_admin.go`, `anagrafica*.go`, `convenzioni_admin.go`, `fornitori_admin.go`, `integrita_admin.go`, `ruoli.go`, `navigazione.go`, `inbox_viva.go`). Rotte, sessioni, ruoli (`ruoli.go`: consultazione < operatore < tecnico < admin, un solo `soloRuolo`), postazione della sessione, Inbox a quadranti e triage, `censisci.go`, thread, allegati e conferma, admin (job, scarti, postazioni, anagrafica clienti, fornitori, Integrità NAS), template HTMX, `e2e/inbox_quadranti.py` per le prove nel browser |
| `workerapi` | le rotte `/api/v1/*`: autenticazione e prova di vita del worker, applicazione dei risultati, ingest dei lotti, upload legato al tentativo, `archivi.go` |

## Dipendenze consentite

`core`, `ai`, `platform` (`coda` e `storage/staging` comprese). Mai il contrario: niente in `core` o `platform` importa `transport`.

## Entry point

`web.Server.Registra`, `workerapi.Server.Registra`.

## Flussi principali — le rotte UI

L'Inbox rende **un pezzo solo** (`inbox_stato`): linguette dei quadranti, direzione, filtro, selettore della
casella, contatori e righe. `GET /inbox` con `HX-Request` risponde con quel frammento; ogni link lo sostituisce
per intero, così ciò che si vede acceso e ciò che si legge nella lista vengono sempre dalla stessa risposta. Il
poll di quindici secondi sta su un elemento **fratello** (`hx-vals` si eredita, e sul contenitore avrebbe
avvelenato ogni link lì dentro) e rilegge i parametri dalla barra degli indirizzi, l'unico posto dove lo stato
della schermata vive. `frammentoRichiesto` distingue una richiesta HTMX normale dal ritorno dalla cronologia
del browser, che vuole la pagina intera.

Le rotte sono montate per **area**, e ogni area ha il suo `registra*` nel file dei suoi gestori. `Registra`
tiene solo quelle che non sono di nessuna area — statici, `/healthz`, login, logout, la radice — e chiama le
altre quattro:

| Area | Monta | Sta in |
|---|---|---|
| inbox | `/inbox*`, `/messaggio/{id}*`, `/allegato/{id}/{riscarica,anteprima}`, `/stato/worker`, `/anagrafica/buyer`, `/thread/cerca` | `routes_inbox.go:registraInbox`; gestori anche in `triage.go`, `censisci.go`, `allegati.go`, `richieste.go`, `agente.go` |
| RFQ | `/thread/{id}*`, `/proposta/{id}/{conferma,scarta}`, `/cruscotto`, `/richieste` | `routes_rfq.go:registraRFQ`; gestori anche in `thread.go`, `richieste.go`, `allegati.go` |
| postazioni | `/sessione/postazione`, `/admin/postazioni*` | `postazioni_admin.go:registraPostazioni`, `postazione.go` |
| admin | `/admin/job*`, `/admin/scarti*`, `/admin/nas*`, `/admin/anagrafica*`, `/admin/fornitori*` | `routes_admin.go:registraAdmin`; gestori anche in `integrita_admin.go`, `anagrafica*.go`, `convenzioni_admin.go`, `fornitori_admin.go` |

| Rotta | Effetto | Package toccati |
|---|---|---|
| `GET /inbox`, `/messaggio/{id}`, `/thread/{id}`, `/thread/cerca`, `/cruscotto`, `/richieste`, `/stato/worker`, `/anagrafica/buyer` | lettura: query e template. Il quadrante viene da `v_inbox.controparte_tipo`; predefinito Buyer | `platform/db`, viste `v_inbox`, `v_thread_fase`, `v_fascicolo` |
| `GET /inbox` (prima volta nella sessione), `POST /inbox/aggiorna`, `POST /inbox/sync-storico` | accodano un `sync_outlook` (di apertura, per casella, in modo storico) | `platform/coda` |
| `POST /messaggio/{id}/apri` · `/bozza` · `/letto` | job interattivi con il `postazione_id` della sessione; `bozza` richiede la capacità `bozze`, `letto` la capacità `outlook_scrittura`; `apri` è sempre consentito | `platform/coda` |
| `POST /messaggio/{id}/scarica`, `/allegato/{id}/riscarica` | `stage_allegato` solo se il contenuto non è già in `_contenuti` | `platform/coda` |
| `GET /allegato/{id}/anteprima` (B8.1) | serve i byte del PDF: **staging** se il contenuto c'è, altrimenti il file del documento sul **NAS**. Nella URL non c'è nessun percorso: quello si compone dal database e passa da `documenti.PercorsoSulNas`. Si serve solo ciò che comincia per `%PDF-`, con `Range`/`If-Range`/`ETag` (`http.ServeContent`) e **senza** ricalcolare l'hash. Un'anomalia aperta ⇒ 409. Se la dimensione non torna o il file è stato toccato dopo l'ultimo sguardo, e SOLO allora, si rilegge una volta con `documenti.VerificaFileAperto`, che su un conflitto **apre l'anomalia** e non fa uscire un byte. Lo staging si legge solo se `path_staging` sta sotto `Server.Staging` (la radice dichiarata, via `documenti.DentroLaRadice`); fuori radice ⇒ 500 e nessun byte, radice non dichiarata ⇒ si passa al NAS. Riga che non c'è ⇒ 404; errore del database o lettura della testa fallita ⇒ 500, mai scambiato per «non c'è» o per «non è un PDF» (415). Il nome va nel `Content-Disposition` in ASCII, con `filename*=UTF-8''…` quando l'originale non lo è | `core/rfq/documenti`, `platform/storage/nas` |
| `GET /messaggio/{id}/triage`, `POST /messaggio/{id}/rfq` · `/aggancia` · `/ignora` | decisioni con `FOR UPDATE`; `nuovaRFQ` crea `thread_offerta`, gli identificativi selezionati e accoda `crea_cartella_thread`. Un messaggio con controparte `fornitore` non ha «Nuova RFQ» | `core/inbox/classificazione`, `core/inbox/aggancio`, `platform/coda` |
| `POST /messaggio/{id}/risposta-fornitore`, `/richiesta-fornitore` | «è la risposta a questa richiesta» e «è la richiesta mandata a mano» (7B) | `core/inbox/aggancio`, `platform/db` |
| `POST /thread/{id}/richiesta` (+ `/annulla`) | la richiesta a un fornitore e, con `bozza=1`, la bozza «nuovo» in Outlook con `Marcatori{CockpitRichiestaFornitore}`; capacità `bozze` | `platform/coda`, `platform/contratti/worker` |
| `GET/POST /messaggio/{id}/censisci` | crea il fornitore o il cliente e fa il ritriage mirato. Senza transazione, di proposito: le scritture non distruttive rispondono «di chi è», e in una transazione abortita non potrebbero | `core/inbox/ingest`, `core/inbox/classificazione` |
| `POST /proposta/{id}/conferma` · `/scarta` | `documento` + `copia_nas`. Con `nas_scrittura` spenta il documento resta `in_coda` | `core/inbox/classificazione`, `platform/coda`, `platform/storage/nas` |
| `POST /thread/{id}/riprova-copie` | riaccoda `copia_nas` per i documenti `in_coda` | `platform/coda` |
| `POST /messaggio/{id}/analizza` | job `analizza_messaggio_ai`, solo se l'agente è acceso per quella casella | `platform/coda`, `ai/agente` |
| `POST /sessione/postazione` | postazione della sessione scelta a mano | `web/postazione.go` |
| `/admin/job/*`, `/admin/scarti/*`, `/admin/nas/*`, `/admin/postazioni/*`, `/admin/anagrafica/*`, `/admin/fornitori*` | `soloAdmin`. I fornitori stanno sotto `/admin/fornitori` e **non** sotto `/admin/anagrafica/fornitori/{id}`: il mux di Go 1.22 considera quel pattern in conflitto con `/admin/anagrafica/{id}/regole` | `platform/coda`, `core/registro`, `platform/fondazioni`, `platform/rete` |

## Flussi principali — le rotte worker

Ogni rotta passa da `auth`: token riconosciuto → **prova di vita** (`ContattoWorker`: online/offline si decide
sull'ultimo contatto, non sull'ultimo claim) → handler.

| Rotta | Che cosa fa il server |
|---|---|
| `POST /api/v1/jobs/claim` | interseca `caselle_aperte` con le autorizzazioni, registra `casella_store`, `coda.Claim` con long-poll, **esclude i tipi che le capacità bloccano**, aggiorna IP e postazione |
| `GET /api/v1/worker/caselle` | le caselle di `worker_credenziale.caselle` |
| `POST /api/v1/jobs/{id}/heartbeat` | rinnova il lease, se il tentativo vale |
| `POST /api/v1/jobs/{id}/result` | verifica il tentativo e applica per tipo: **sync** → `ultimo_received` sempre, le frontiere solo per le cartelle dichiarate `completa`; **stage** → promuove `.parte` in `_contenuti`, poi `estrai_archivio` o `analizza_allegato`; **analisi** → `analisi_fatti` e una `documento_proposta` per ogni allegato aperto con quello sha256; **bozza** → `bozza`. Codice e revisione che arrivano da fuori passano da `codiceRevSicuri` (7C.1): fuori misura → non in colonna, grezzi in `dettagli` |
| `GET /api/v1/allegati/{id}/contenuto` | i byte di un allegato in staging al **solo** tentativo valido di un job `analizza_allegato` di quell'allegato: 409 tentativo non valido, 422 job di un altro allegato, 410 contenuto sparito |
| `POST /api/v1/ingest/messaggi` | `ingest.Ingerisci`: casella verificata prima della transazione, modo del sync letto dal job |
| `PUT /api/v1/allegati/{id}/file` | `_parti/<allegato>.parte.<lease_token>`, sha256 verificato, promozione solo dal result valido; 413 oltre `max_upload_mb` |
| `GET /api/v1/sync/cursori` | cursori per (casella, cartella) |

## Invarianti

- `web` serve solo HTML, `workerapi` solo JSON.
- Ogni rotta UI esiste in due forme: pagina intera e frammento.
- Ruoli ordinati, un solo `soloRuolo` per rotta; CSRF su tutto ciò che scrive.
- Il token **identifica**: nome, tipo, postazione e caselle vengono da lì, non dal corpo della richiesta.
- Un result si applica solo al tentativo valido; un file caricato resta in `_parti` finché il result non lo promuove.
- **Difetto noto.** «Riprova» su uno scarto di tipo `lettura` accoda un job `rileggi_elemento`, ma
  `worker_outlook.py:dispatch()` non ha quel tipo: il worker lo chiude come errore definitivo. Non è mai stato
  implementato lato worker; è scritto qui perché il README non deve descrivere una cosa che il codice non fa.
- **Eccezioni aperte**: `archivi.go`, `applicaRisultato` e `propostaDaAnalisi` decidono cose di dominio dentro
  il livello HTTP.

## Effetti collaterali

Cookie di sessione, righe in `sessione` e `worker_presenza`, file in `_parti`, job accodati.

## Test

L4 per gli handler (`./internal/transport/web/`, `./internal/transport/workerapi/`), compresi i test W4/W10/W11
sull'autenticazione e i TestE2E che fanno girare il worker Python vero. L7 nel browser per l'Inbox.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una rotta nuova | il `registra*` dell'area, l'handler nel file dell'area, il template |
| un campo nuovo nel contratto con i worker | `platform/contratti/worker/tipi.go`, `workers/contratti.py`, `genera_contratti.py`, i test L3 |
| capire che cosa succede quando un job finisce | `workerapi/workerapi.go:applicaRisultato` |
| capire perché un messaggio sta in quel quadrante | la colonna `quadrante` di `v_inbox`, `web/routes_inbox.go:quadranteValido` |
| capire perché una risposta di un fornitore non chiude la richiesta | `web/richieste.go:rispostaFornitore` |

## Leggi anche

`internal/README.md`, `core/README.md`, `platform/README.md`, `workers/workers_README.md`.
