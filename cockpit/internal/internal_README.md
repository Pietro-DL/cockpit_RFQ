# `internal/` — come il server usa i suoi pacchetti

Visione d'insieme di `cockpit.exe`: quali pacchetti esistono, chi chiama chi, e come i dati attraversano il
server nei flussi principali. Scritta sul codice a `0bdea9f`.

## 1. Il server ha due facce

```
   browser (operatore)                          worker Python (postazioni)
        │ sessione (cookie)                            │ X-Cockpit-Token individuale
        ▼                                              ▼
   internal/web  ──────────────┐          ┌──── internal/workerapi
   (HTMX, ruoli, form)         │          │      (claim, heartbeat, result, ingest, upload)
                               ▼          ▼
                           internal/jobs  (coda, lease, scheduler, esecutore server, shadow)
                               │          │
                     internal/ingest   internal/domain   internal/aggancio   internal/agente
                     (fatti → DB)      (interpretazione) (candidati)         (LLM, spento di default)
                               │
                           internal/db  (sqlc)  ──►  PostgreSQL
                               │
                 internal/nas · internal/archivio · internal/logfile · internal/rete · internal/config
```

Un solo listener (`[server].indirizzo`), due famiglie di rotte, due autenticazioni: la UI con la sessione
(`sessione` in DB, cookie `SameSite=Lax`, ruoli), l'API worker con il token individuale (sha256 in
`worker_credenziale`, da cui il server ricava nome, tipo, postazione e caselle). Il server **non apre mai
connessioni** verso i PC: sono i worker a chiamare (vedi `workers/README.md`).

## 2. Mappa dei pacchetti

| Pacchetto | Cosa fa | Chi lo chiama | Cosa usa |
|---|---|---|---|
| `config` | legge `cockpit.toml`, mette i default, valida, normalizza percorsi e fondazioni (caselle, postazioni, worker, utenti) | `cmd/cockpit/main.go` | `rete` (TLS) |
| `migrazioni` | applica `migrations/*.sql` in ordine, una transazione per file, con verifica statica (nessun riferimento a tabelle future, enum non usati nello stesso file) | `main.go` all'avvio | `db` |
| `fondazioni` | semina da config le tabelle "fondazioni" (`casella`, `postazione`, `worker_credenziale`, `utente`) senza sovrascrivere ciò che è stato generato in UI; diagnosi delle credenziali | `main.go`, `web/postazioni_admin.go` | `db`, `rete` |
| `anagrafica` | seme dei clienti da `seme_anagrafica.json` (`-semina-anagrafica`), una volta e senza sovrascrivere | `main.go` | `db`, `domain` |
| `db` | codice generato da **sqlc** da `db/queries/*.sql`: tipi, enum, una funzione per query. Non si modifica a mano | tutti | pgx |
| `api` | i **contratti** JSON fra server e worker (`tipi.go`): payload dei job, richieste e risposte; specchio di `workers/contratti.py` | `workerapi`, `jobs`, `web` | — |
| `jobs` | la coda: `Accoda*` (con chiave di idempotenza e opzioni), `Claim` (un UPDATE con `SKIP LOCKED`, lease_token, filtri per tipo/casella/postazione), scheduler (lease scaduti, sync per casella, sessioni, `.parte` orfani, retention), `EsecutoreServer` (job che esegue il server stesso), `shadow.go` (cosa non si accoda/consegna in shadow), `stage.go`/`upload.go` (guardie di staging e analisi) | `web`, `workerapi`, `ingest`, `main.go` | `db`, `api`, `nas`, `agente` |
| `workerapi` | le sette rotte `/api/v1/*`; autenticazione; applica i risultati dei job (allegati, bozze, cursori, fatti dell'analisi); ingest dei lotti; upload legato al tentativo | worker | `jobs`, `ingest`, `domain`, `archivio` |
| `ingest` | un lotto di messaggi → `messaggio`, `messaggio_casella`, `allegato`, `conversazione`, `riferimento_portale`, `proposta_triage`; transazione per lotto con savepoint; scarti; cursore; staging automatico; replay degli scarti | `workerapi` | `db`, `domain`, `aggancio`, `jobs` |
| `domain` | l'**interpretazione** pura, senza DB: `codici.go` (famiglie del cliente, riferimento RFQ, triage con precedenza risposta > candidati > nuova RFQ), `regole.go` (schema di `cliente.regole`, ✓/✗), `proposta.go` (tipo del documento da nome, estensione e fatti), `path.go` (nome cartella NAS), `anagrafica.go`, `aggancio.go` (punteggi R1–R5) | `ingest`, `web`, `workerapi` | — |
| `aggancio` | calcola i **candidati** di aggancio con evidenza (In-Reply-To, conversazione, codici/articoli, buyer) e li scrive come proposte; mai `thread_id` | `ingest`, `web/triage.go` | `db`, `domain` |
| `agente` | l'assistente semantico: `Modello` (interfaccia), `ClientAPI` (Anthropic, chiave da variabile d'ambiente), `prompt.go` (oggetto, corpo ≤ 12k, nomi allegati, cliente), grounding e idempotenza (`analisi_messaggio`). **Spento** senza chiave; parte solo da `POST /messaggio/{id}/analizza` | `jobs/server.go` | rete HTTP |
| `web` | la UI: rotte, sessioni, ruoli (`soloAdmin`), postazione della sessione, Inbox e triage, thread, allegati e conferma, admin (job, scarti, postazioni, anagrafica), template HTMX | browser | `jobs`, `ingest`, `domain`, `aggancio`, `nas`, `db` |
| `nas` | unico scrittore sul NAS: `.parte` + hash + rinomina, mai sovrascrive, `dry_run` | `jobs/server.go` (`copia_nas`, `crea_cartella_thread`) | — |
| `archivio` | estrazione zip con budget sui byte scritti e protezione zip-slip (fuori dalla transazione) | `workerapi` | — |
| `rete` | certificato TLS autofirmato generato al primo avvio, impronta per i `worker.toml`, hash dei token | `config`, `fondazioni`, `main.go` | — |
| `logfile` | log rotante del server (`_staging/log/cockpit.log`) | `main.go` | — |
| `testutil` | DB di test usa e getta (`COCKPIT_TEST_DSN`, solo nomi con "test") | test L4 | — |

Regola di dipendenza: `domain` non importa nulla del progetto; `db` è generato; `web` e `workerapi` sono gli
unici a parlare HTTP; `nas` è l'unico a scrivere sul NAS; `jobs` è l'unico a inserire in `job`.

## 3. Le rotte e i pacchetti che toccano

### 3.1 UI (`web/web.go`, sessione + ruolo)

| Rotta | Handler | Effetto | Pacchetti |
|---|---|---|---|
| `GET /inbox`, `/messaggio/{id}`, `/thread/{id}`, `/cruscotto`, `/richieste`, `/stato/worker` | lettura | query e template | `db`, viste `v_inbox`, `v_thread_fase`, `v_fascicolo` |
| `POST /inbox/aggiorna` | `aggiornaOra` | un `sync_outlook` per casella attiva (chiave fissa) | `jobs.AccodaSyncCasella` |
| `POST /inbox/sync-storico` | `syncStorico` | un job storico di 2 giorni a ritroso per casella | `jobs` |
| `POST /messaggio/{id}/apri` · `/bozza` · `/letto` | `apriInOutlook`, `bozza`, `segnaLetto` | job interattivi con `postazione_id` della sessione, `scade_il`; rifiutati in shadow (tranne `apri`) | `jobs.OpzioniInterattive`, `shadow.go` |
| `POST /messaggio/{id}/scarica`, `/allegato/{id}/riscarica` | `scarica`, `riscarica` | `stage_allegato` solo se il file non è già in staging con lo stesso hash | `jobs.AccodaStage` |
| `GET /messaggio/{id}/triage`, `POST /messaggio/{id}/rfq` · `/aggancia` · `/ignora` | `triageForm`, `nuovaRFQ`, `agganciaEsistente`, `ignora` | decisioni con `FOR UPDATE` sul messaggio; `nuovaRFQ` crea `thread_offerta`, gli identificativi **selezionati**, e accoda `crea_cartella_thread`; log in `messaggio_aggancio_log` | `domain`, `aggancio`, `jobs` |
| `POST /proposta/{id}/conferma` · `/scarta` | `conferma`, `scarta` | `documento` + `copia_nas` (in shadow: `in_coda`, "in attesa di produzione") | `domain.path`, `jobs`, `nas` |
| `POST /messaggio/{id}/analizza` | `chiediAnalisi` | job `analizza_messaggio_ai` (server) | `jobs`, `agente` |
| `POST /sessione/postazione` | `scegliPostazione` | postazione della sessione scelta a mano (altrimenti abbinata per IP a un worker) | `web/postazione.go` |
| `/admin/job/*`, `/admin/scarti/*` | `soloAdmin` | riaccoda/annulla; riprova scarti (`ingest` → dal payload, `lettura` → job `rileggi_elemento`) | `jobs`, `ingest/replay.go` |
| `/admin/postazioni/*` | `soloAdmin` | stato dei worker; pacchetto con credenziali nuove (invalida le vecchie) | `fondazioni`, `rete` |
| `/admin/anagrafica/*` | `soloAdmin` | clienti, domini, buyer, fabbisogno, regole (form e JSON), banco di prova (= la stessa funzione del triage) | `domain/regole.go`, `domain/codici.go` |

### 3.2 API worker (`workerapi/workerapi.go`, token individuale)

| Rotta | Cosa fa il server |
|---|---|
| `POST /api/v1/jobs/claim` | `ContattoWorker` (presenza), interseca `caselle_aperte` con le autorizzazioni, registra `casella_store`, `jobs.Claim` con long-poll, aggiorna `worker_presenza` |
| `GET /api/v1/worker/caselle` | le caselle di `worker_credenziale.caselle` |
| `POST /api/v1/jobs/{id}/heartbeat` | rinnova il lease **e** il contatto, se il tentativo vale |
| `POST /api/v1/jobs/{id}/result` | verifica il tentativo, `applicaRisultato` per tipo: stage → `allegato` + accoda analisi; analisi → `analisi_fatti` + proposte per ogni allegato con quello sha256; bozza → `bozza`; sync → chiude; poi `CompletaJob`/`FallisciJob` |
| `POST /api/v1/ingest/messaggi` | `ingest.Ingerisci`: casella verificata prima della transazione, una tx per lotto, savepoint per elemento, scarti, cursore, staging automatico, candidati di aggancio |
| `PUT /api/v1/allegati/{id}/file` | `.parte.<lease_token>` nello staging del server, sha256 verificato, promozione solo dal result valido; 413 oltre `max_upload_mb` |
| `GET /api/v1/sync/cursori` | cursori per (casella, cartella) |

## 4. I flussi principali

**Avvio.** `main.go`: `config.Carica` → `logfile` → `migrazioni.Applica` → `fondazioni.Semina` (utenti,
caselle, postazioni, credenziali: senza sovrascrivere i token generati in UI) → `fondazioni.UnaSolaCasellaAttiva`
finché lo schema è < 4 → `rete` (TLS se `[server].tls_cert/tls_key`, altrimenti solo loopback salvo
`consenti_lan_in_chiaro`) → `jobs.Scheduler.Avvia` → `EsecutoreServer` → listener con `web` + `workerapi`.
Se `-semina-anagrafica file.json`: valida, semina, esce.

**Sync.** scheduler/UI → `job sync_outlook` (chiave `sync_outlook:<casella>`) → worker claim → lotti su
`/ingest/messaggi` → `ingest`: `messaggio` (identità = Message-ID), `messaggio_casella` (una per casella),
allegati con natura, `conversazione`, riferimenti portale, `domain.Triage` con le regole del cliente →
`proposta_triage` e candidati di aggancio (`aggancio`) → cursore nella stessa transazione → result.

**Nuova RFQ.** `POST /messaggio/{id}/rfq` → `FOR UPDATE` sul messaggio → `thread_offerta` con
`cartella_relativa` da `domain/path.go` → `identificativo_thread` per i codici spuntati (origine `proposta`
con punteggio, `manuale` se digitati) → `componente` → job `crea_cartella_thread` (server, `nas`) → log.

**Allegato → NAS.** `scarica` → `stage_allegato` (worker Outlook: `SaveAsFile` + `PUT`) → `allegato.in_staging`
→ `analizza_allegato` (worker analisi) → `analisi_fatti` → `domain.ProponiDaAnalisi` → `documento_proposta` →
operatore conferma → `documento` + `copia_nas` (server, `nas`: `.parte`, hash, rinomina) → `stato_nas='scritto'`.
In shadow o `dry_run` si ferma a `in_coda`.

**Apri in Outlook.** UI → job con `postazione_id` della sessione (abbinata per IP al worker che ha fatto
claim da quell'IP, o scelta a mano) → solo il worker di quella postazione lo prende → `Display()`.

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
- **`[outlook]`**: `cartelle` sono le cartelle Outlook da leggere per ogni casella (default `Inbox`, `Sent
  Items`); `giorni_sync_iniziale` è la finestra del primo caricamento quando non c'è cursore; `intervallo_sync_s`
  il periodo dello scheduler (0 = solo manuale); `lotto` la dimensione dei lotti; `dal` un override
  esplicito per import controllati; `sync_apertura_inbox` fa accodare un sync alla prima apertura
  dell'Inbox.
- **`[nas]`**: `radice` (dove si scrive), `dry_run` (calcola i percorsi, non scrive), `radici_produzione`
  (radici che il server rifiuta in shadow), `staging` (lo staging **del server**, dove arrivano gli upload dei
  worker e dove legge il worker analisi).
- **`[server].modalita`**: `shadow` (default se assente) blocca accodamento e claim di `crea_bozza_outlook`,
  `segna_letto`, `sposta_in_cartella`, `copia_nas`, `crea_cartella_thread` e forza `dry_run`; al passaggio a
  `produzione` i job di quei tipi rimasti in coda vengono annullati.
- **`[agente]`**: fase 4; spento senza `chiave_env`/`modello`.

## 6. La coda in dettaglio (`jobs`)

- `AccodaCon(tipo, payload, chiave, priorità, opzioni)`: chiave vuota = job sempre nuovo; chiave piena =
  nessun duplicato pendente. `Opzioni` porta `casella_id`, `postazione_id`, `richiesto_da`, `scade_il`.
- `Claim(worker_tipo, worker_id, destinazione{postazione, caselle}, attesa)`: un solo UPDATE su
  `stato='pronto' AND non_prima_di <= now() AND (casella_id IS NULL OR = ANY(caselle)) AND (postazione_id IS
  NULL OR = $postazione) AND (scade_il IS NULL OR > now())`, ordinato per priorità, `FOR UPDATE SKIP LOCKED`;
  imposta `lease_token`, `lease_fino_a`, `avviato_il`; in attesa, ripete fino ad `attesa`.
- Scheduler (`coda.go:Avvia`): `lease` ogni 30 s (scaduti → `pronto`, oltre 5 tentativi → `fallito`;
  interattivi scaduti → `annullato`), `sync_outlook` ogni `intervallo_sync_s`, `sessioni` ogni ora,
  `parti` ogni 15 min (`.parte.*` orfani), `retention` ogni 6 h.
- `EsecutoreServer` (`server.go`): esegue in-process `copia_nas`, `crea_cartella_thread`,
  `analizza_messaggio_ai` (worker di tipo `server`, con lo stesso lease).

## 7. I tre strati

Ovunque nel server vale la stessa separazione, ed è il criterio per capire dove va un pezzo di codice:

| Strato | Domanda | Dove |
|---|---|---|
| **Fatto** | cosa è arrivato, da chi, quando, con quali allegati e hash | `ingest`, tabelle `messaggio*`, `allegato`, `analisi_fatti` |
| **Interpretazione** | cosa probabilmente significa: cliente, codici, riferimento, tipo documento, candidati di aggancio, con punteggio ed evidenza | `domain`, `aggancio`, `agente`; tabelle `proposta_triage`, `documento_proposta`, `analisi_messaggio` |
| **Decisione** | cosa l'operatore ha deciso | `web/triage.go`, `web/allegati.go`; `thread_id`, `identificativo_thread`, `documento`, `messaggio_aggancio_log` |

Un'interpretazione non scrive mai una decisione: nessun `thread_id` dall'ingest, nessun `documento` senza
conferma, nessun invio da `crea_bozza`. È la regola che rende il sistema verificabile: se un aggancio è
sbagliato, o l'ha deciso una persona o è un bug in una proposta, mai una via di mezzo.

## 8. Dove guardare per…

| Voglio capire… | Apri |
|---|---|
| come si legge il toml e cosa viene rifiutato | `config/config.go:Carica`, `normalizza*` |
| chi crea un certo job | `grep -rn "TipoJob<Nome>" internal/web internal/ingest internal/jobs` |
| cosa succede quando un job finisce | `workerapi/workerapi.go:applicaRisultato` |
| come si sceglie il cliente e i codici di una mail | `domain/codici.go:Triage`, `Motore.Codici` |
| come si calcolano i candidati di aggancio | `aggancio/aggancio.go`, punteggi in `domain/aggancio.go` |
| perché un'azione è bloccata | `jobs/shadow.go`, `web/ruoli.go` |
| il nome della cartella sul NAS | `domain/path.go` |
| lo schema | `migrations/`, `db/queries/*.sql`, poi `sqlc generate` |
| i contratti con i worker | `api/tipi.go` ↔ `workers/contratti.py`, `contracts/*.schema.json` |
