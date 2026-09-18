# `internal/` — come il server usa i suoi pacchetti

Visione d'insieme di `cockpit.exe`: quali pacchetti esistono, chi chiama chi, e come i dati attraversano il
server nei flussi principali. Aggiornata al ramo `blocco-7` dopo il Pre-7 (commit `1fc5584`): rispetto alla
prima stesura (`0bdea9f`) sono cambiati la sicurezza (capacità al posto della modalità shadow), il sync (finestra
chiusa e frontiere di copertura), lo staging (cache per contenuto), gli archivi (estrazione come job), l'integrità
del NAS e il taglio della catena di risposta. Quando il codice cambia, cambia questo file.

## 1. Il server ha due facce

```
   browser (operatore)                          worker Python (postazioni)
        │ sessione (cookie)                            │ X-Cockpit-Token individuale
        ▼                                              ▼
   internal/web  ──────────────┐          ┌──── internal/workerapi
   (HTMX, ruoli, form)         │          │      (claim, heartbeat, result, ingest, upload, archivi)
                               ▼          ▼
                           internal/jobs  (coda, lease, scheduler, capacità, esecutore server,
                               │          │   cache dei contenuti, ricognitore NAS)
                     internal/ingest   internal/domain   internal/aggancio   internal/agente
                     (fatti → DB)      (interpretazione) (candidati)         (LLM, spento di default)
                               │
                           internal/db  (sqlc)  ──►  PostgreSQL
                               │
                 internal/nas · internal/archivio · internal/logfile · internal/rete · internal/config
```

Un solo listener (`[server].indirizzo`), due famiglie di rotte, due autenticazioni: la UI con la sessione
(`sessione` in DB, cookie `SameSite=Lax`, ruoli ordinati), l'API worker con il token individuale (sha256 in
`worker_credenziale`, da cui il server ricava nome, tipo, postazione e caselle). Il server **non apre mai
connessioni** verso i PC: sono i worker a chiamare (vedi `workers/workers_README.md`).

## 2. Mappa dei pacchetti

| Pacchetto | Cosa fa | Chi lo chiama | Cosa usa |
|---|---|---|---|
| `config` | legge `cockpit.toml`, mette i default, valida, normalizza percorsi e fondazioni (caselle, postazioni, worker, utenti); risolve `[sicurezza]` nelle tre capacità (`Capacita()`), `[staging]`, `[retention]` (cache), `[nas].intervallo_integrita_s` | `cmd/cockpit/main.go` | `rete` (TLS) |
| `migrazioni` | applica `migrations/*.sql` in ordine, una transazione per file, con verifica statica (ogni file registra la sua versione; ogni `REFERENCES` punta a una tabella già creata; un valore aggiunto a un enum non si usa nello stesso file) | `main.go` all'avvio | `db` |
| `fondazioni` | semina da config le tabelle "fondazioni" (`casella`, `postazione`, `worker_credenziale`, `utente`) senza sovrascrivere ciò che è stato generato in UI; diagnosi delle credenziali | `main.go`, `web/postazioni_admin.go` | `db`, `rete` |
| `anagrafica` | seme dei clienti da `seme_anagrafica.json` (`-semina-anagrafica`), una volta e senza sovrascrivere | `main.go` | `db`, `domain` |
| `db` | codice generato da **sqlc** da `db/queries/*.sql`: tipi, enum, una funzione per query. Non si modifica a mano | tutti | pgx |
| `api` | i **contratti** JSON fra server e worker (`tipi.go`: payload dei job, richieste e risposte; `protocollo.go`: i tempi del claim e della presenza); specchio di `workers/contratti.py` e `workers/protocollo.py` | `workerapi`, `jobs`, `web` | — |
| `jobs` | la coda: `Accoda*` (chiave di idempotenza e opzioni), `Claim` (un UPDATE con `SKIP LOCKED`, lease_token, filtri per tipo/casella/postazione, **esclusi i tipi bloccati dalle capacità**), scheduler (`coda.go`: lease scaduti, sync per casella, sessioni, `_parti` orfane, retention dei job), `capacita.go` (le tre capacità di scrittura e `AllineaCoda`), `server.go` (`EsecutoreServer`: job eseguiti dal server, vigilanza sul NAS assente), `stage.go`/`upload.go` (download in staging, `_parti` e `_contenuti`), `cache.go` (il custode della cache dei contenuti, Pre-7), `integrita.go` (il ricognitore del NAS, blocco 5B), `routing.go` (quale copia di un messaggio usare per un download o per un'azione sulla postazione) | `web`, `workerapi`, `ingest`, `main.go` | `db`, `api`, `nas`, `agente` |
| `workerapi` | le sette rotte `/api/v1/*`; autenticazione e prova di vita del worker; applica i risultati dei job (contenuti, proposte, cursori e frontiere, bozze); ingest dei lotti; upload legato al tentativo; `archivi.go` (estrazione di un archivio come job del server) | worker, `jobs/server.go` | `jobs`, `ingest`, `domain`, `archivio` |
| `ingest` | un lotto di messaggi → `messaggio`, `messaggio_casella`, `allegato`, `conversazione`, `riferimento_portale`, `proposta_triage`; transazione per lotto con savepoint; scarti; cursore; staging automatico **deciso dal modo del sync** (D30); replay degli scarti | `workerapi` | `db`, `domain`, `aggancio`, `jobs` |
| `domain` | l'**interpretazione** pura, senza DB: `codici.go` (famiglie del cliente, riferimento RFQ, triage con precedenza risposta > candidati > nuova RFQ), `catena.go` (taglio della catena di risposta: l'interpretazione legge quello che è stato scritto adesso, blocco 6), `regole.go` (schema di `cliente.regole`, ✓/✗), `proposta.go` (tipo del documento da nome ed estensione), `path.go` (nome cartella NAS), `anagrafica.go`, `aggancio.go` (punteggi R1–R5) | `ingest`, `web`, `workerapi` | — |
| `aggancio` | calcola i **candidati** di aggancio con evidenza (In-Reply-To, conversazione, codici/articoli, buyer) e li scrive come proposte; mai `thread_id` | `ingest`, `web/triage.go` | `db`, `domain` |
| `agente` | l'assistente semantico: `Modello` (interfaccia), `ClientAPI` (Anthropic, chiave da variabile d'ambiente), `prompt.go` (oggetto, corpo ≤ 12k, nomi allegati, cliente), grounding e idempotenza (`analisi_messaggio`). **Spento** senza `[agente].attivo`, modello e chiave, e solo sulle caselle elencate; parte solo da `POST /messaggio/{id}/analizza`. Il gate per accenderlo (corpus, autorizzazione IT/ISO) non è chiuso | `jobs/server.go` | rete HTTP |
| `web` | la UI: rotte, sessioni, ruoli (`ruoli.go`: consultazione < operatore < tecnico < admin, un solo `soloRuolo`), postazione della sessione, Inbox e triage, thread (con «Riprova copie»), allegati e conferma, admin (job, scarti, postazioni, anagrafica, **Integrità NAS**), template HTMX | browser | `jobs`, `ingest`, `domain`, `aggancio`, `nas`, `db` |
| `nas` | unico scrittore sul NAS: `.parte` + hash + rinomina, mai sovrascrive | `jobs/server.go` (`copia_nas`, `crea_cartella_thread`), `jobs/integrita.go` (in lettura) | — |
| `archivio` | estrazione zip con budget sui byte scritti e protezione zip-slip | `workerapi/archivi.go` (dentro il job `estrai_archivio`) | — |
| `rete` | certificato TLS autofirmato generato al primo avvio, impronta per i `worker.toml`, hash dei token | `config`, `fondazioni`, `main.go` | — |
| `logfile` | log rotante del server (`<staging>/log/cockpit.log`) | `main.go` | — |
| `testutil` | DB di test usa e getta (`COCKPIT_TEST_DSN`, solo nomi con "test"; senza variabile i test L4 sono SKIP, mai PASS) | test L4 | — |

Regola di dipendenza: `domain` non importa nulla del progetto; `db` è generato; `web` e `workerapi` sono gli
unici a parlare HTTP; `nas` è l'unico a scrivere sul NAS; `jobs` è l'unico a inserire in `job`.

## 3. Le rotte e i pacchetti che toccano

### 3.1 UI (`web/web.go`, sessione + ruolo)

| Rotta | Handler | Effetto | Pacchetti |
|---|---|---|---|
| `GET /inbox`, `/messaggio/{id}`, `/thread/{id}`, `/thread/cerca`, `/cruscotto`, `/richieste`, `/stato/worker`, `/anagrafica/buyer` | lettura | query e template | `db`, viste `v_inbox`, `v_thread_fase`, `v_fascicolo` |
| `GET /inbox` (prima volta nella sessione) | `inbox` | accoda un `sync_outlook` di apertura, una volta per sessione (`sessione.sync_inbox_il`, 0011), se `[outlook].sync_apertura_inbox` non è `false` | `jobs.AccodaSyncCasella` |
| `POST /inbox/aggiorna` | `aggiornaOra` | un `sync_outlook` per casella attiva (chiave fissa `sync_outlook:<casella>`) | `jobs.AccodaSyncCasella` |
| `POST /inbox/sync-storico`, `GET /inbox/sync-storico/stato` | `syncStorico`, `syncStoricoStato` | un job in modo `storico` per casella: `GiorniStorico` (2) a ritroso da `storico_fino_a`, per ciascuna cartella la sua finestra; lo stato per il badge | `jobs` |
| `POST /messaggio/{id}/apri` · `/bozza` · `/letto` | `apriInOutlook`, `bozza`, `segnaLetto` | job interattivi con `postazione_id` della sessione, `scade_il`; `bozza` richiede la capacità `bozze`, `letto` la capacità `outlook_scrittura`; `apri` è sempre consentito (apre una finestra, non modifica niente). Con la capacità spenta l'accodamento rifiuta e l'avviso dice quale riga di `[sicurezza]` manca | `jobs.OpzioniInterattive`, `jobs/capacita.go` |
| `POST /messaggio/{id}/scarica`, `/allegato/{id}/riscarica` | `scarica`, `riscarica` | `stage_allegato` solo se il contenuto non è già in `_contenuti` (per questo allegato o per un altro con lo stesso sha256) | `jobs.AccodaStage`, `jobs/routing.go` (`CopiaPerDownload`) |
| `GET /messaggio/{id}/triage`, `POST /messaggio/{id}/rfq` · `/aggancia` · `/ignora` | `triageForm`, `nuovaRFQ`, `agganciaEsistente`, `ignora` | decisioni con `FOR UPDATE` sul messaggio; `nuovaRFQ` crea `thread_offerta`, gli identificativi **selezionati**, e accoda `crea_cartella_thread`; log in `messaggio_aggancio_log` | `domain`, `aggancio`, `jobs` |
| `POST /proposta/{id}/conferma` · `/scarta` | `conferma`, `scarta` | `documento` + `copia_nas`. Con `nas_scrittura` spenta il job non entra in coda: il documento resta `in_coda` e lo si rimette in copia con «Riprova copie» o da Admin › Integrità NAS | `domain.path`, `jobs`, `nas` |
| `POST /thread/{id}/riprova-copie` | `riprovaCopie` | riaccoda `copia_nas` per i documenti `in_coda` della RFQ (blocco 5A): è il gesto di una persona, e con la capacità spenta rifiuta con il nome della capacità | `jobs` |
| `POST /messaggio/{id}/analizza` | `chiediAnalisi` | job `analizza_messaggio_ai` (server), solo se l'agente è acceso per quella casella | `jobs`, `agente` |
| `POST /sessione/postazione` | `scegliPostazione` | postazione della sessione scelta a mano (altrimenti abbinata per IP a un worker) | `web/postazione.go` |
| `/admin/job/*`, `/admin/scarti/*` | `soloAdmin` | riaccoda/annulla (i job «in attesa di produzione» sono quelli marcati da `AllineaCoda`); riprova scarti: `ingest` → dal payload, `lettura` → job `rileggi_elemento` (vedi la nota qui sotto) | `jobs`, `ingest/replay.go` |
| `/admin/nas`, `/admin/nas/controlla`, `/admin/nas/{id}/riaccoda` · `/allinea` | `soloAdmin` | Integrità NAS (blocco 5B): le anomalie del ricognitore; «Controlla ora»; Riaccoda (in attesa / errore / mancante) e Allinea (già presente con l'hash giusto). Il conflitto non ha un pulsante di proposito | `jobs/integrita.go`, `nas` |
| `/admin/postazioni/*` | `soloAdmin` | stato dei worker; pacchetto con credenziali nuove (invalida le vecchie) | `fondazioni`, `rete` |
| `/admin/anagrafica/*` | `soloAdmin` | clienti (cartella NAS letta da qui, 4A), domini, buyer, fabbisogno, regole (form e JSON), articoli, banco di prova (= la stessa funzione del triage) | `domain/regole.go`, `domain/codici.go` |

**Nota su `rileggi_elemento`.** «Riprova» su uno scarto di tipo `lettura` accoda un job `rileggi_elemento` per il
worker Outlook, ma `worker_outlook.py:dispatch()` non ha quel tipo: il worker lo chiude come errore definitivo
(«tipo job sconosciuto»). Non è mai stato implementato lato worker. È un difetto noto, registrato qui perché il
README non deve descrivere una cosa che il codice non fa; si corregge con un incarico.

### 3.2 API worker (`workerapi/workerapi.go`, token individuale)

Ogni rotta passa da `auth`: token riconosciuto → **prova di vita** (`ContattoWorker`, 0009: online/offline si
decide sull'ultimo contatto, non sull'ultimo claim) → handler.

| Rotta | Cosa fa il server |
|---|---|
| `POST /api/v1/jobs/claim` | interseca `caselle_aperte` con le autorizzazioni, registra `casella_store`, `jobs.Claim` con long-poll (≤ `api.AttesaClaimMax`), **esclude i tipi che le capacità bloccano** (`TipiBloccatiOra`), aggiorna IP e postazione in `worker_presenza` |
| `GET /api/v1/worker/caselle` | le caselle di `worker_credenziale.caselle` |
| `POST /api/v1/jobs/{id}/heartbeat` | rinnova il lease, se il tentativo vale |
| `POST /api/v1/jobs/{id}/result` | verifica il tentativo, `applicaRisultato` per tipo: **sync** → `ultimo_received` sempre, le frontiere (`coperto_fino_a` o `storico_fino_a`) solo per le cartelle dichiarate `completa`; **stage** → promuove `_parti/<allegato>.parte.<token>` in `_contenuti/<ab>/<sha256>.<ext>`, poi accoda `estrai_archivio` (server) se è un archivio, altrimenti `analizza_allegato`; **analisi** → `analisi_fatti(sha256, versione, hash_config)` e una `documento_proposta` per ogni allegato aperto con quello sha256, ciascuno con le regole del proprio cliente (`propostaDaAnalisi`); **bozza** → `bozza`; poi `CompletaJob`/`FallisciJob` |
| `POST /api/v1/ingest/messaggi` | `ingest.Ingerisci`: casella verificata prima della transazione, il modo del sync letto dal **job** (non dal lotto), una tx per lotto, savepoint per elemento, scarti, cursore, staging automatico se il modo lo consente, candidati di aggancio |
| `PUT /api/v1/allegati/{id}/file` | `_parti/<allegato>.parte.<lease_token>` nello staging del server, sha256 verificato, promozione solo dal result valido; 413 oltre `max_upload_mb` |
| `GET /api/v1/sync/cursori` | cursori per (casella, cartella): `ultimo_received`, `coperto_fino_a` |

## 4. I flussi principali

**Avvio.** `main.go`: `config.Carica` → `logfile` → `migrazioni.Applica` → `fondazioni.Semina` (utenti, caselle,
postazioni, credenziali: senza sovrascrivere i token generati in UI) → `fondazioni.UnaSolaCasellaAttiva` finché lo
schema è < 4 → **capacità**: `jobs.ImpostaCapacita` dalle tre voci risolte, `jobs.AllineaCoda` (capacità spenta →
i job di quei tipi già in coda vengono marcati «in attesa di produzione»; capacità accesa → quelli marcati vengono
**annullati**, niente parte da solo perché qualcuno ha cambiato una riga), log con attive/spente/bloccati e gli
avvisi di `config.Capacita()` → `rete` (TLS se `[server].tls_cert/tls_key`, altrimenti solo loopback salvo
`consenti_lan_in_chiaro`) → `jobs.Scheduler.Avvia` → `jobs.Cache.Avvia` → `EsecutoreServer.Avvia` (con la vigilanza
sul NAS) → `Ricognitore.Avvia` → listener con `web` + `workerapi`. Se `-semina-anagrafica file.json`: valida, semina,
esce.

**Sync.** scheduler / «Aggiorna ora» / prima apertura dell'Inbox / «Carica precedenti» → `job sync_outlook` con il
**modo** (`aggiornamento` | `bootstrap` | `storico`) e, per ogni cartella, una finestra chiusa `[dal, al]` decisa
dal server all'accodamento (`aggiornamento`: da `coperto_fino_a − 10 min` ad adesso; `bootstrap`: gli ultimi
`giorni_sync_iniziale`, 7 se assente; `storico`: `GiorniStorico` prima di `storico_fino_a`) → worker claim → legge
dal più recente al più vecchio, lotti su `/ingest/messaggi` → `ingest`: `messaggio` (identità = Message-ID),
`messaggio_casella` (una per casella), allegati con natura, `conversazione`, riferimenti portale, `domain.Triage`
sul corpo **tagliato** (`catena.go`) con le regole del cliente → `proposta_triage` e candidati di aggancio →
`ultimo_received` nella stessa transazione → result: la frontiera avanza **solo se la cartella è stata percorsa per
intero**. Le riletture costano una deduplica per Message-ID; i buchi non sono ammessi.

**Nuova RFQ.** `POST /messaggio/{id}/rfq` → `FOR UPDATE` sul messaggio → `thread_offerta` con
`cartella_relativa` da `domain/path.go` (la cartella del cliente viene dall'Anagrafica) → `identificativo_thread`
per i codici spuntati (origine `proposta` con punteggio, `manuale` se digitati) → `componente` → job
`crea_cartella_thread` (server, `nas`, capacità `nas_scrittura`) → log.

**Allegato → NAS.** «Scarica», oppure lo staging automatico se il modo del sync lo consente (`aggiornamento` sì;
`bootstrap` solo con `[staging].bootstrap = true`; `storico` mai) → `stage_allegato` (worker Outlook: `SaveAsFile`
+ `PUT`) → il contenuto sta in `_contenuti` con lo sha256 per nome (`allegato.in_staging`) → se è un archivio,
`estrai_archivio` (server) mette ogni voce fra i contenuti e la registra come allegato figlio → `analizza_allegato`
(worker analisi) → `analisi_fatti` → `documento_proposta` → operatore conferma → `documento` + `copia_nas`
(server, `nas`: `.parte`, hash, rinomina) → `stato_nas='scritto'`. **La cache non si consuma**: il contenuto
resta in `_contenuti` dopo la copia (Pre-7, D31); se una copia lo trova sparito, il server riaccoda da solo la
riestrazione (archivio ancora in cache) o il download da Outlook, una volta per copia, e riprova al tentativo
dopo. Se il NAS manca, il job viene rinviato senza consumare tentativi e la vigilanza lo rimette in coda al ritorno.

**Integrità NAS.** Il ricognitore (`jobs/integrita.go`, ogni `intervallo_integrita_s`, 15 min se assente, la
prima passata non immediata) confronta `documento` con i file veri: `in_attesa` (in_coda da troppo senza copia in
coda), `errore`, `mancante`, `gia_presente`, `conflitto` (file diverso: nessuna azione automatica), `illeggibile` → righe di
`nas_anomalia` → Admin › Integrità NAS. Legge il NAS anche con `nas_scrittura` spenta.

**Apri in Outlook.** UI → job con `postazione_id` della sessione (abbinata per IP al worker che ha fatto claim da
quell'IP, o scelta a mano) → solo il worker di quella postazione lo prende → `Display()`.

**Login e postazione.** `POST /login` → bcrypt → `sessione` → `web/postazione.go` cerca un worker con
`indirizzo_ip` uguale al remote address della sessione; se non c'è ancora, la riabbina alla prima richiesta
successiva a un claim dallo stesso IP.

## 5. `config`: le domande che ti sei fatto

- **`Carica` non è hardcoded.** Le righe `c.Server.Indirizzo = "127.0.0.1:8080"` ecc. sono i **default**;
  subito dopo `toml.DecodeFile(percorso, c)` sovrascrive ogni campo presente nel file. Poi le `normalizza*`
  validano: DSN obbligatorio, `max_upload_mb ≥ 1`, modalità fra le due ammesse, ruoli utente fra quelli
  ammessi (un ruolo sconosciuto ferma l'avvio), percorsi resi assoluti rispetto alla cartella del toml,
  fondazioni in forma canonica (indirizzi in minuscolo, host in maiuscolo, ogni worker legato a una
  postazione e a caselle esistenti).
- **Postazioni e caselle non "si connettono".** `[[postazione]]`, `[[casella]]`, `[[worker]]` descrivono chi
  è **autorizzato** a cosa; `fondazioni.Semina` li scrive in DB; la connessione la fa il worker chiamando il
  server con il suo token, e il server lo riconosce dall'hash. Nessuna porta sui PC.
- **TLS e `consenti_lan_in_chiaro`.** `ETLS()` è vero se ci sono sia il certificato sia la chiave. Senza TLS il
  server si rifiuta di ascoltare su un indirizzo che non sia loopback, perché in chiaro passerebbero posta,
  token e cookie; `consenti_lan_in_chiaro = true` è l'eccezione dichiarata (banco di prova), non un
  default. Con `rete` il certificato è autofirmato, generato al primo avvio, e i worker lo riconoscono
  dall'impronta scritta nel loro `worker.toml` (modello SSH): `http://` diventa `https://` senza CA.
- **`[sicurezza]` e `[server].modalita`: le capacità (blocco 4).** Che cosa il server è autorizzato a
  **modificare fuori da sé** non è più un interruttore solo ma tre capacità: `outlook_scrittura` (segna letto,
  sposta in cartella), `bozze` (crea_bozza_outlook), `nas_scrittura` (crea_cartella_thread, copia_nas). Tutto il
  resto legge e basta ed è sempre consentito (sync, stage, analisi, estrazione, «Apri in Outlook»). Tre regole
  in `Capacita()`: (1) `modalita = "shadow"` è un **preset**: spegne tutto, qualunque cosa dica `[sicurezza]`, ed
  è il valore quando la riga manca; (2) il silenzio vale «non scrivere»: `modalita = "produzione"` senza la
  sezione `[sicurezza]` non accende niente, e il server lo scrive nel log con la riga da aggiungere; (3)
  `[nas].dry_run = true` spegne `nas_scrittura` (voce **deprecata**, va tolta). In più `consenti_nas_produzione`
  è il secondo consenso, richiesto quando `nas_scrittura` è accesa e `[nas].radice` sta sotto una delle
  `radici_produzione` dichiarate; senza, il server non parte. Il blocco vale in due punti: all'accodamento (chi
  ha premuto il pulsante se lo sente dire) e al claim (un job già in coda non si esegue).
- **`[outlook]`**: `cartelle` sono le cartelle Outlook da leggere per ogni casella (default `Inbox`, `Sent
  Items`); `giorni_sync_iniziale` è la finestra del bootstrap (7 se assente); `intervallo_sync_s` il periodo dello
  scheduler (0 = nessun sync accodato da solo; restano «Aggiorna ora», «Carica precedenti» e il sync di
  apertura); `lotto` la dimensione dei lotti; `dal` un override esplicito della finestra iniziale, solo per le
  cartelle senza cursore; `sync_apertura_inbox` (assente = acceso) fa accodare un sync alla prima apertura
  dell'Inbox di ogni sessione.
- **`[staging]`** (D30): `automatico` scarica da solo gli allegati dei messaggi in entrata di un cliente censito
  durante l'aggiornamento ordinario; `bootstrap = true` lo estende alla prima sincronizzazione di una casella
  (assente = false: è l'ordine di scaricare l'archivio); per lo storico non esiste una voce, di proposito;
  `max_mb` la soglia (20 se assente).
- **`[nas]`**: `radice` (dove si scrive), `staging` (lo staging **del server**: `_parti`, `_contenuti`, `log`),
  `radici_produzione` (le radici vere, per il secondo consenso), `intervallo_integrita_s` (il ricognitore: assente
  = 900, 0 = solo «Controlla ora»), `dry_run` deprecata.
- **`[retention]`**: `giorni_job` toglie righe di coda chiuse; `cache_gg` (assente = 30, 0 = mai per età) e
  `cache_max_mb` (assente = nessun limite) governano la cache `_contenuti`; `giorni_staging` è deprecata e vale
  come `cache_gg` se questa manca.
- **`[agente]`**: `attivo`, `modello`, `url`, `chiave_env` (il NOME della variabile d'ambiente), `caselle` (vuoto
  = nessuna). Spento se non lo si accende qui.

## 6. La coda in dettaglio (`jobs`)

- `AccodaCon(tipo, payload, chiave, priorità, opzioni)`: chiave vuota = job sempre nuovo; chiave piena =
  nessun duplicato pendente. `Opzioni` porta `casella_id`, `postazione_id`, `richiesto_da`, `scade_il`. Un tipo
  la cui capacità è spenta non entra in coda: `ErrCapacitaSpenta`, con il nome della capacità nel messaggio.
- `Claim(worker_tipo, worker_id, destinazione{postazione, caselle}, attesa)`: un solo UPDATE su
  `stato='pronto' AND non_prima_di <= now() AND (casella_id IS NULL OR = ANY(caselle)) AND (postazione_id IS
  NULL OR = $postazione) AND (scade_il IS NULL OR > now()) AND tipo::text <> ALL($tipi_esclusi)`, ordinato per
  priorità, `FOR UPDATE SKIP LOCKED`; imposta `lease_token`, `lease_fino_a`, `avviato_il`; in attesa, ripete fino
  ad `attesa`.
- Scheduler (`coda.go:Avvia`): `lease` ogni 30 s (scaduti → `pronto`, oltre 5 tentativi → `fallito`;
  interattivi scaduti → `annullato`), `sync_outlook` ogni `intervallo_sync_s` (se > 0), `sessioni` ogni ora,
  `parti` ogni 15 min (`_parti` orfane più vecchie di 10 min), `retention` ogni 6 h (solo job chiusi).
- `Cache` (`cache.go`): una passata 2 min dopo l'avvio e poi ogni 6 h. Toglie un contenuto di `_contenuti` solo
  se **nessuno lo sta usando** (documento `in_coda`/`errore`, proposta aperta, job pendente che lo cita, anomalia
  NAS aperta; un archivio resta finché una sua voce è pinnata, anche una voce non più sul disco) **e** è fermo
  da `cache_gg` giorni, **oppure** la cache supera `cache_max_mb` (dal meno usato, il minimo che basta). Alla
  rimozione i `path_staging` vanno a NULL nella stessa transazione. `ToccaContenuto` rinfresca l'orario a ogni
  uso (la copia sul NAS lo fa).
- `EsecutoreServer` (`server.go`): esegue in-process `estrai_archivio`, `analizza_messaggio_ai`,
  `crea_cartella_thread`, `copia_nas` (worker di tipo `server`, con lo stesso lease). Se il NAS non c'è il job
  viene rinviato senza consumare il tentativo; `vigilaNas` rimette `pronto` le scritture che avevano esaurito i
  tentativi quando il NAS torna, e la prima verifica è all'avvio.
- `Ricognitore` (`integrita.go`): ogni `intervallo_integrita_s`; scrive solo `documento.verificato_il` e
  `nas_anomalia`; non sovrascrive mai un file diverso.

## 7. I tre strati

Ovunque nel server vale la stessa separazione, ed è il criterio per capire dove va un pezzo di codice:

| Strato | Domanda | Dove |
|---|---|---|
| **Fatto** | cosa è arrivato, da chi, quando, con quali allegati e hash; che cosa c'è davvero sul NAS | `ingest`, `jobs/integrita.go`, tabelle `messaggio*`, `allegato`, `analisi_fatti`, `nas_anomalia` |
| **Interpretazione** | cosa probabilmente significa: cliente, codici, riferimento, tipo documento, candidati di aggancio, con punteggio ed evidenza | `domain`, `aggancio`, `agente`; tabelle `proposta_triage`, `documento_proposta`, `analisi_messaggio` |
| **Decisione** | cosa l'operatore ha deciso | `web/triage.go`, `web/allegati.go`, `web/thread.go`, `web/integrita_admin.go`; `thread_id`, `identificativo_thread`, `documento`, `messaggio_aggancio_log` |

Un'interpretazione non scrive mai una decisione: nessun `thread_id` dall'ingest, nessun `documento` senza
conferma, nessun invio da `crea_bozza`, nessuna sovrascrittura sul NAS dal ricognitore. È la regola che rende il
sistema verificabile: se un aggancio è sbagliato, o l'ha deciso una persona o è un bug in una proposta, mai una
via di mezzo. E il corpo originale di un messaggio non si modifica mai: il taglio della catena decide che cosa
viene **dato in pasto** all'interpretazione, non che cosa resta in `messaggio.corpo_testo`.

## 8. Dove guardare per…

| Voglio capire… | Apri |
|---|---|
| come si legge il toml e cosa viene rifiutato | `config/config.go:Carica`, `normalizza*`, `Capacita` |
| chi crea un certo job | `grep -rn "TipoJob<Nome>" internal/web internal/ingest internal/jobs internal/workerapi` |
| cosa succede quando un job finisce | `workerapi/workerapi.go:applicaRisultato` |
| come si sceglie il cliente e i codici di una mail | `domain/codici.go:Triage`, `Motore.Codici`, `domain/catena.go:TagliaCatena` |
| come si calcolano i candidati di aggancio | `aggancio/aggancio.go`, punteggi in `domain/aggancio.go` |
| perché un'azione è bloccata | `jobs/capacita.go`, `web/ruoli.go` |
| da dove parte una finestra di sync e quando avanza una frontiera | `jobs/coda.go:AccodaSyncCasella`, `api/tipi.go` (i tre modi), `workerapi.go:applicaRisultato` (sync) |
| perché un contenuto è ancora in staging, o non c'è più | `jobs/cache.go`, `db/queries/cache.sql` (i pin) |
| perché un documento è `in_coda` da giorni | Admin › Integrità NAS, `jobs/integrita.go`, `jobs/server.go:vigilaNas` |
| il nome della cartella sul NAS | `domain/path.go` |
| lo schema | `migrations/`, `db/queries/*.sql`, poi `sqlc generate` |
| i contratti con i worker | `api/tipi.go` ↔ `workers/contratti.py`, `api/protocollo.go` ↔ `workers/protocollo.py`, `contracts/*.schema.json` |
