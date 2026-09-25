# `internal/platform/coda` — la coda dei job in PostgreSQL

## Scopo

La tabella `job` e le regole con cui ci si entra e se ne esce: accodamento idempotente, claim con lease e
tentativo, chiusura solo con il token del tentativo, rinvio senza consumare il tentativo, scheduler dei
lavori periodici, le tre **capacità di scrittura** del processo, l'instradamento per casella e per
postazione, e gli accodamenti che hanno bisogno di un controllo prima (download in staging, analisi,
sync di una casella).

Il package **non esegue niente**: i job di tipo `server` li esegue `app/runtime` (`EsecutoreServer`), gli
altri i worker Python attraverso `transport/workerapi`.

## Non appartiene qui

- Che cosa fa un job: i corpi stanno in `core/rfq/documenti` (copia, cartella), `transport/workerapi`
  (estrazione degli archivi, risultati dei worker), `ai/agente` (analisi del messaggio).
- Chi esegue i job del server e il rinvio quando il NAS manca: `app/runtime`.
- La lettura di `[sicurezza]`: `platform/config` (`Config.Capacita`). Qui arrivano tre booleani già risolti.
- I file dello staging: `platform/storage/staging` (qui si usa `Staging.Presente`, `CartellaStaging`,
  `PulisciParti`).
- Il protocollo HTTP del claim, dell'heartbeat e del risultato: `transport/workerapi`.

## File

| File | Responsabilità |
|---|---|
| `coda.go` | Commento `// Package`. Le tabelle per tipo: `WorkerPer`, `LeaseSecondi`, `DurataMassimaS`, `MaxTentativiPer`. `Opzioni`, `Accoda`, `AccodaCon` (blocco per capacità, poi `InsertJob`), `AccodaCopia`, le chiavi `ChiaveCopia`, `ChiaveSpostamento`, `ChiaveSyncCasella`. Il tentativo: `Destinazione`, `Claim` (long-poll), `Tentativo`, `Completa`, `Fallisci`, `Rinvia`, `Verifica`, `Blocca`, `Batte`, `ChiudiSeDurataSuperata`, `ErrTentativoNonValido`; `InJob` (riga → contratto). `Scheduler` (`Avvia`, `loop`, `accodaSync`) e il sync ordinario: `SyncOpzioni`, `AccodaSyncCasella`, `GiorniSyncInizialeDefault`, `SovrapposizioneSync`, `PrioritaSyncStorico` |
| `capacita.go` | `Capacita` (`Ha`, `Consente`, `Attive`, `Spente`, `TuttoSpento`), `CapacitaPer`, i nomi `CapOutlookScrittura` / `CapBozze` / `CapNasScrittura`, lo stato del processo (`ImpostaCapacita`, `CapacitaAttuali`, `TipiBloccatiOra`, `TipiBloccati`, `Consentito`), `ErrCapacitaSpenta` / `CapacitaMancante`, `AllineaCoda` |
| `routing.go` | Su quale copia di un messaggio agisce un job: `Copia`, `CopiaPerPostazione` (job interattivi, mai un ripiego su un altro PC), `CopiaPerDownload`, `ErrNessunaPostazione`, `ErrNessunWorkerIdoneo` (`Motivo`), `ScadenzaInterattiva`, `OpzioniInterattive` |
| `stage.go` | `AccodaStage` con i due controlli prima del download (`EsitoStage`: `StageAccodato`, `StageGiaInCoda`, `StageGiaPresente`, `StageRiusato`); `Analizzatore` (`Hash`), `AccodaAnalisi`, `ChiaveAnalisi` |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Accoda` | `core/rfq/documenti` (riestrazione; lo spostamento `AccodaSpostamento` lo chiamano solo le prove), `core/rfq/fascicolo` (estrazione), `transport/web` (cartella della RFQ, analisi AI), `transport/workerapi` (estrazione) |
| `AccodaCon`, `Opzioni` | `core/inbox/ingest/replay.go` (rilettura di un elemento), `transport/web` (job interattivi, bozze, sync storico) |
| `AccodaCopia`, `ChiaveCopia` | `transport/web` (conferma di una proposta, «Riprova copie», riconciliazione dell'integrità); `ChiaveCopia` anche `core/rfq/documenti` |
| `ChiaveSpostamento` | `core/rfq/documenti/nomi_nas.go:AccodaSpostamento` |
| `AccodaSyncCasella`, `SyncOpzioni` | `transport/web/inbox_viva.go` («Aggiorna ora», prima apertura dell'Inbox), `Scheduler` |
| `PrioritaSyncStorico` | `transport/web/routes_inbox.go` («Carica precedenti», accodato con `AccodaCon` e chiave `sync_storico:<casella>`) |
| `AccodaStage`, `CopiaPerDownload`, `Stage*` | `core/inbox/ingest` (staging automatico), `core/rfq/documenti/ripresa.go`, `core/rfq/fascicolo`, `transport/web` |
| `AccodaAnalisi`, `ChiaveAnalisi`, `Analizzatore` | `core/rfq/fascicolo` (preparazione, rianalisi), `transport/workerapi` (dopo l'upload, voci di un archivio); `Analizzatore` anche `app/runtime` e `transport/web` |
| `Claim`, `Destinazione`, `Tentativo`, `Completa`, `Fallisci` | `app/runtime/esecutore.go`, `transport/workerapi` |
| `Rinvia` | `app/runtime/vigilanza_nas.go:rinviaSeNasAssente` |
| `Verifica`, `Blocca`, `Batte`, `ChiudiSeDurataSuperata`, `InJob`, `ErrTentativoNonValido` | `transport/workerapi` |
| `Scheduler`, `ImpostaCapacita`, `AllineaCoda`, `TutteLeCapacita`, `TipiBloccatiOra` | `app/runtime` (avvio) |
| `CapacitaAttuali` | `core/rfq/documenti/integrita.go`, `transport/web/integrita_admin.go` |
| `CapacitaPer`, `ErrCapacitaSpenta`, `CapacitaMancante` | `transport/web` (il messaggio all'operatore quando un'azione è spenta) |
| `CopiaPerPostazione`, `OpzioniInterattive`, `ErrNessunaPostazione`, `ErrNessunWorkerIdoneo` | `transport/web` («Apri in Outlook», bozze, segna letto) |
| `DurataMassimaS` | `core/rfq/documenti` |

Usati solo dentro il package o dalle prove: `WorkerPer`, `LeaseSecondi`, `MaxTentativiPer`,
`ScadenzaInterattiva`, `TipiBloccati`, `SovrapposizioneSync`, `Consentito`.

### Le tabelle per tipo

| Tipo | Worker (`WorkerPer`) | Lease s | Durata max s | Tentativi | Capacità (`CapacitaPer`) |
|---|---|---|---|---|---|
| `sync_outlook` | outlook | 300 | 1800 | 5 | — |
| `stage_allegato`, `apri_elemento_outlook`, `rileggi_elemento` | outlook | 120 | 600 | 5 | — |
| `segna_letto`, `sposta_in_cartella` | outlook | 120 | 600 | 5 | `outlook_scrittura` |
| `crea_bozza_outlook` | outlook | 120 | 600 | 5 | `bozze` |
| `analizza_allegato` | analisi | 120 | 600 | 5 | — |
| `copia_nas` | server | 300 | 3600 | 50 | `nas_scrittura` |
| `crea_cartella_thread` | server | 120 | 600 | 50 | `nas_scrittura` |
| `sposta_nas` | server | 120 | 600 | 5 | `nas_scrittura` |
| `estrai_archivio` | server | 300 | 600 | 5 | — |
| `analizza_messaggio_ai` | server | 120 | 600 | 5 | — |
| `backup_db` | server | 300 | 3600 | 5 | — |

`Opzioni` può cambiare lease, durata e tentativi di un job; zero vale «come da tipo».

**Priorità.** Il claim prende il numero più basso (`ORDER BY priorita, job_id`); la priorità la sceglie chi
accoda:

| Priorità | Chi |
|---|---|
| 1 | copia sul NAS (`AccodaCopia`), cartella della RFQ, download chiesto dall'operatore, «Apri in Outlook», bozze; lo spostamento sul NAS (`AccodaSpostamento`, solo prove) |
| 2 | «segna letto»; download della preparazione del Fascicolo; download e riestrazione della ripresa dei documenti |
| 3 | download dello staging automatico dell'ingest; rilettura di un elemento scartato (`ingest/replay.go`) |
| 4 | estrazione di un archivio (preparazione del Fascicolo, zip appena caricato dal worker) |
| 5 | sync ordinario (`AccodaSyncCasella`); analisi AI di un messaggio |
| 6 | analisi di un contenuto (`AccodaAnalisi`) |
| 9 | «Carica precedenti» (`PrioritaSyncStorico`) |

## Dati

Il package non apre transazioni: ogni funzione lavora sul `*db.Queries` che riceve, quindi dentro la
transazione del chiamante quando il chiamante ne ha una.

**Scrive** `job`:

- `InsertJob` — `ON CONFLICT (chiave_idempotenza) WHERE stato IN ('pronto','in_corso') DO NOTHING`
  (indice parziale `ux_job_chiave_pendente`): una chiave è unica solo fra i job pendenti; zero righe →
  `(nil, nil)`, «c'era già».
- `ClaimJob` — un solo `UPDATE` con sottoselezione `FOR UPDATE SKIP LOCKED`, ordine `priorita, job_id`;
  genera `lease_token`, fissa `avviato_il`, incrementa `tentativi`; esclude i tipi di `TipiBloccatiOra`,
  i job scaduti, quelli di una casella non servita o di un'altra postazione.
- `HeartbeatJob`, `CompletaJob`, `FallisciJob`, `RinviaJob` e `BloccaTentativo` (`SELECT … FOR UPDATE`,
  lucchetto sulla riga fino alla fine della transazione del chiamante) portano tutti lo stesso predicato
  di validità: `stato='in_corso'`, stesso token, stesso worker, lease non scaduto, durata massima non
  superata. `VerificaTentativo` è lo stesso predicato senza lucchetto.
- `FallisciJob` — backoff `LEAST(600, 15·2^tentativi)` secondi; `fallito` a tentativi esauriti o con
  `definitivo`. `RinviaJob` restituisce il tentativo consumato dal claim.
- `ScadutoPerDurataMassima`, e dallo scheduler `RilasciaLeaseScaduti`, `AnnullaJobScaduti`,
  `EliminaJobVecchi`.
- `AllineaCoda`: `MarcaJobInAttesaDiProduzione` (aggiunge `[in attesa di produzione]` a `errore`) e
  `AnnullaJobDellaShadow` (annulla i marcati dei tipi ora consentiti).

**Scrive anche, fuori da `job`:**

- `allegato` — `AccodaStage` mette `stato='grezzo'`, `errore='in coda'` prima di accodare, oppure
  `SetAllegatoStaging` con il percorso di un gemello con lo stesso sha256 (`StageRiusato`);
- `sessione` — lo `Scheduler` cancella ogni ora le sessioni scadute (`EliminaSessioniScadute`);
- file `.parte` orfani nello staging — lo `Scheduler` ogni 15 minuti con `staging.PulisciParti`.

**Legge:** `casella` (attive), `sync_cursore` (cursori e copertura per cartella), `messaggio_casella`,
`worker_credenziale`, `postazione` (instradamento), `analisi_fatti` (analisi già fatte per la terna
sha256 / versione / configurazione), `allegato` (gemello già in staging).

**Stato del processo:** `capacita` (`atomic.Value`), una sola per processo, fissata all'avvio da
`ImpostaCapacita`. Non impostata vale **tutto acceso** (vedi Invarianti).

## Flussi principali

**Accodamento.** `AccodaCon` → se il tipo nomina una capacità spenta, errore che avvolge
`ErrCapacitaSpenta` e nomina la capacità (non `(nil, nil)`) → payload in JSON → lease, durata e tentativi
dal tipo se non dati → `InsertJob`. Con la chiave già pendente: `(nil, nil)`.

**Claim.** `Claim(tipo worker, id, Destinazione, attesa)` → `ClaimJob` ogni secondo fino ad `attesa` o
alla chiusura del contesto → `(nil, nil)` se non c'è niente. `Destinazione{}` prende solo job senza
casella e senza postazione (server, analisi). Le caselle vuote diventano un array vuoto, non `NULL`.

**Vita del tentativo.** Il worker batte (`Batte`) e chiude (`Completa` / `Fallisci`) esibendo il token;
un token che non vale più riceve `ErrTentativoNonValido` e il chiamante risponde 409. `Verifica` prima e
dopo un upload, `Blocca` nella transazione che promuove un file. `Rinvia` per un job che non è nemmeno
cominciato (il NAS assente). `ChiudiSeDurataSuperata` riporta a `pronto` un job caduto per durata.

**Scheduler** (`Scheduler.Avvia`, una goroutine per compito; ognuno gira subito e poi a intervallo):

| Compito | Ogni | Che cosa |
|---|---|---|
| `lease` | 30 s | `RilasciaLeaseScaduti` (pronto, o fallito se esauriti), poi `AnnullaJobScaduti` (interattivi oltre `scade_il`) |
| `sync_outlook` | `IntervalloSync` (se > 0) | `accodaSync`: un `AccodaSyncCasella` per ogni casella attiva di canale outlook; una casella che fallisce non ferma le altre |
| `sessioni` | 1 h | cancella le sessioni scadute |
| `parti` | 15 min (se `Staging` non è vuoto) | `.parte` orfani più vecchi di 10 minuti |
| `retention` | 6 h (se `RetentionGiorni` > 0) | job chiusi più vecchi di `RetentionGiorni` |

**Sync di una casella** (`AccodaSyncCasella`). `al` = adesso, uguale per tutte le cartelle. Per ogni
cartella il limite inferiore è, in quest'ordine: la copertura (`coperto_fino_a`) meno
`SovrapposizioneSync` (10 minuti), se c'è e non è nel futuro; altrimenti `SyncOpzioni.Dal`
(`[outlook].dal`); altrimenti `al − GiorniIniziali` (default 7). Cartelle assenti = `Inbox` e
`Sent Items`. Modo `aggiornamento` se almeno una cartella ha copertura, altrimenti `bootstrap`. Chiave
fissa per casella (`sync_outlook:<casella>`), priorità 5: i tick non accumulano coda.

**Download** (`AccodaStage`). File di questo allegato già al suo posto → `StageGiaPresente`, niente
job. Stesso sha256 già in staging per un altro allegato → si riusa il percorso, `StageRiusato`.
Altrimenti allegato a `grezzo` / «in coda» e job `stage_allegato` con chiave `stage:<allegato>`,
vincolato alla casella della copia scelta dal chiamante (`CopiaPerDownload`).

**Analisi** (`AccodaAnalisi`). Se `analisi_fatti` ha già la terna, `(nil, nil)`. Altrimenti job
`analizza_allegato`, priorità 6, chiave `ChiaveAnalisi` = `analizza:<sha256>:<versione>:<hash
configurazione>`: un job per contenuto, non per allegato. Il payload non porta il percorso dello
staging: il worker prende i byte dal server.

**Job interattivi.** `CopiaPerPostazione` sceglie la copia in una casella attiva servita da un worker
Outlook della postazione del richiedente (prima la casella personale del richiedente, poi la più
vecchia); senza postazione `ErrNessunaPostazione`, senza copia servibile `*ErrNessunWorkerIdoneo` con
il motivo. `OpzioniInterattive` mette insieme casella, postazione, richiedente e scadenza (bozza 60
minuti, il resto 10).

**Allineamento all'avvio** (`AllineaCoda`). Capacità spenta → i job pendenti di quei tipi vengono
marcati e restano in coda senza partire. Capacità accesa → i job marcati di quei tipi vengono annullati
(i documenti restano `in_coda`, si rimettono in copia con «Riprova copie»). Senza marcatore non si
annulla niente.

## Invarianti

- **Un tentativo che non vale più non scrive niente**: ogni query che chiude, rinnova, rinvia o blocca
  un job porta il predicato di validità completo (token, worker, lease, durata massima).
- **Una chiave di idempotenza è unica fra i job pendenti**, non per sempre: dopo la chiusura lo stesso
  lavoro si può riaccodare.
- **Il blocco delle capacità sta in due punti**: all'accodamento (`AccodaCon`) e al claim
  (`TipiBloccatiOra` → `tipi_esclusi`). Un job già in coda di un tipo spento non parte.
- **Un nome di capacità sconosciuto vale no** (`Capacita.Ha`); un tipo che non nomina capacità è una
  lettura ed è sempre consentito.
- **`TipiBloccati` è vuoto e non nil**, e le caselle di `Claim` sono un array vuoto e non `NULL`:
  `<> ALL (NULL)` e `= ANY (NULL)` non sono mai veri.
- **Capacità di default: tutto acceso.** Il default sicuro sta in `config` (il silenzio vale «non
  scrivere»); il binario le imposta prima di scheduler ed esecutore (`app/runtime/avvio.go`).
- **Un job interattivo non si dirotta**: nessun ripiego su un'altra postazione o sulla prima copia.
- **Nessuno `store_id` nei payload**: il job porta casella ed `entry_id`, lo store lo risolve il worker.
- **Il sync ha una finestra chiusa**: `al` si decide all'accodamento; una copertura nel futuro si ignora.

## Dipendenze

Importa: `platform/db`, `platform/contratti/worker` (payload e `worker.Job`), `platform/storage/staging`
(`Staging`, `CartellaStaging`, `PulisciParti`). Nessuna violazione della tabella di `internal/README.md`,
che però cita solo l'interfaccia `Staging`.

È importato da: `app/runtime`, `core/inbox/ingest`, `core/rfq/documenti`, `core/rfq/fascicolo`,
`transport/web`, `transport/workerapi`.

## Test

| File | Livello | Che cosa copre |
|---|---|---|
| `capacita_test.go` | L1 | nome sconosciuto = no, ogni tipo nomina una capacità che esiste, quale capacità governa quale tipo, attive/spente, `TipiBloccati` vuoto e non nil |
| `capacita_db_test.go` | L4 | SH1 (tutto spento: si accodano solo le letture), SH3 («Apri» senza capacità), NAS acceso senza Outlook, bozze a parte, l'errore nomina la capacità, SH2 (marcati e poi annullati), `AllineaCoda` non annulla ciò che non ha aspettato |
| `coda_db_test.go` | L4 | lease scaduto → riaccodato → fallito, risultato e fallimento con token vecchio rifiutati, durata massima, job scaduto mai assegnato, riaccodo con chiave pendente, retention, sync che non si accumula per casella, idempotenza solo fra pendenti |
| `concorrenza_db_test.go` | L4 | claim esclusivo sotto concorrenza, il battito tiene vivo il tentativo, doppio risultato, solo il tentativo in corso chiude |
| `finestra_db_test.go`, `futuro_db_test.go` | L4 | finestra iniziale (SI1–SI4: default, configurata, il cursore vince, `dal` override), copertura nel futuro ignorata |
| `routing_db_test.go` | L4 | Q8, M12, M3, M10, M6, Q17, Q21: casella e postazione, nessuno store_id, nessun ripiego, download senza postazione, destinazione vuota |
| `stage_db_test.go` | L4 | niente download se il file c'è, stesso contenuto non riscaricato, file sparito → riscarica, un solo download pendente |
| `analisi_db_test.go` | L4 | un job per contenuto/versione/configurazione, hash della configurazione stabile, allegato senza hash |

Non coperti da prove dirette: `Scheduler.Avvia` e `loop` (si prova `accodaSync`, non i ticker), `Rinvia`
(provato da `app/runtime/nas_db_test.go`), `Blocca`, `InJob`.

## Stato dell'implementazione

- Completo: accodamento, claim, tentativo, scheduler, capacità (blocco 4), instradamento (fase 2),
  finestra chiusa del sync (3R, blocco 3), analisi per contenuto (A15), `ChiaveAnalisi` (B8.7b).
- `sposta_nas` (B8.A4a): tipo, capacità e `ChiaveSpostamento` ci sono; nessun codice di produzione lo
  accoda (`AccodaSpostamento` lo chiamano solo le prove) e l'esecuzione arriva con B8.8. Fino ad allora
  `WorkerPer` lo manda al server, e l'esecutore (`app/runtime/esecutore.go:esegui`) chiude un tipo che
  non gestisce come fallimento definitivo, al primo tentativo.
- `backup_db`: valore dell'enum con lease e durata, mai accodato da nessuno; al server varrebbe come
  `sposta_nas`.
- `rileggi_elemento`: lo accoda il server (`core/inbox/ingest/replay.go`), ma il worker Outlook non ha un
  caso per lui e lo chiude come errore definitivo.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| un tipo di job nuovo | migrazione (enum `tipo_job`), poi `coda.go` (`WorkerPer`, `LeaseSecondi`, `DurataMassimaS`, `MaxTentativiPer`), `capacita.go` (`CapacitaPer`), e chi lo esegue (`app/runtime/esecutore.go:esegui` o il worker) |
| capire perché un job non parte | `capacita.go` (`TipiBloccatiOra`, `AllineaCoda`), la query `ClaimJob` in `platform/db/queries/job.sql` |
| capire perché un job parte due volte o non si accoda | la chiave di idempotenza (`Chiave*`), `InsertJob` |
| cambiare la finestra del sync | `coda.go:AccodaSyncCasella` |
| cambiare dove va un job interattivo | `routing.go:CopiaPerPostazione`, query `CopiaPerPostazione` |
| aggiungere un lavoro periodico | `coda.go:Scheduler.Avvia` |
| cambiare backoff o scadenza dei lease | `platform/db/queries/job.sql` (`FallisciJob`, `RilasciaLeaseScaduti`) |

## Leggi anche

`internal/README.md`, `internal/platform/README.md`, `internal/app/README.md`,
`internal/transport/README.md`, `workers/workers_README.md`.
