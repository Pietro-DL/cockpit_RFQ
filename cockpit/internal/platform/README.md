# `platform/` — ciò che il server usa e che non sa niente di RFQ

## Scopo

Database, configurazione, migrazioni, fondazioni, TLS e rete, log, coda dei job, staging, NAS, archivi,
contratti con i worker, utilità per i test. Nessuno di questi package sa che cosa sia una RFQ, un cliente o
un triage.

Questo README tiene le regole comuni; il dettaglio è nei README dei package.

## Non appartiene qui

Regole di classificazione, handler HTTP, prompt dell'agente, decisioni dell'operatore, loop che eseguono
job (l'esecutore sta in `app/runtime`).

## Package posseduti

| Package | Che cosa fa | README |
|---|---|---|
| `config` | legge `cockpit.toml`: valori predefiniti, decodifica, avvisi sulle voci sconosciute o senza effetto (`Config.Avvisi`), percorsi relativi letti dalla cartella del file, validazione e forma canonica delle fondazioni; `[sicurezza]` → le tre capacità (`Capacita`); la rete della riga di comando (`Rete`) | [README](config/README.md) |
| `db` | il codice generato da **sqlc** da `db/queries/*.sql`: 28 file, 438 query, i tipi e gli enum dello schema. **Non si modifica a mano** | [README](db/README.md) |
| `migrazioni` | applica `migrations/*.sql` in ordine, una transazione per file, sotto un lucchetto; verifica statica; rifiuta uno schema più recente del binario; `Elenca` e `Applicate` per chi deve solo guardare | [README](migrazioni/README.md) |
| `fondazioni` | porta in database `[[utenti]]`, `[[casella]]`, `[[postazione]]`, `[[worker]]` senza sovrascrivere ciò che è stato generato altrove; un utente nuovo nasce solo con una password vera | [README](fondazioni/README.md) |
| `rete` | certificato TLS autofirmato generato al primo avvio e la sua impronta; impronte e generazione dei token dei worker; il filtro del listener sulle reti consentite (`SoloDalleReti`) | [README](rete/README.md) |
| `contratti/worker` | i **contratti** JSON fra server e worker (`tipi.go`, `protocollo.go`), specchio di `workers/contratti.py` e `workers/protocollo.py` | [README](contratti/README.md) |
| `coda` | la **coda** in PostgreSQL: accodamento idempotente, claim con lease e tentativo, scheduler (lease scaduti, sync periodico, retention), le tre capacità di scrittura, l'instradamento per postazione e per casella, lo staging di un allegato e l'analisi. Non esegue niente | [README](coda/README.md) |
| `storage/staging` | la **cartella di lavoro** del server: `.parte` con il token del tentativo, `_contenuti` con lo sha256 per nome, le cartelle di estrazione, il custode della cache | [README](storage/README.md) |
| `storage/nas` | unico scrittore sul NAS: `.parte` + hash + rinomina, mai sovrascrive; prefisso long-path oltre i 250 caratteri | [README](storage/README.md) |
| `storage/archivio` | estrazione zip con un budget sui byte e sulle voci, protezione zip-slip, nomi tagliati a caratteri | [README](storage/README.md) |
| `logfile` | il log del server su file con rotazione (il file corrente più 5 copie da 5 MB). Se la rinomina fallisce perché il file è tenuto aperto da un altro processo, si continua a scrivere nel file corrente, si riprova più avanti, e le copie vecchie restano | qui |
| `testutil` | il database di test usa e getta per le prove L4: `Pool`, `SchemaPulito`, `SchemaFinoA`; la guardia `DatabaseDiTest` | qui |

**`testutil` e la guardia del database.** `COCKPIT_TEST_DSN` deve portare a un database il cui nome
contiene «test». Il nome si legge come lo leggerà pgx al momento di connettersi (`pgconn.ParseConfig`), non
dal testo del DSN: vale anche per un DSN `chiave=valore`, per `?dbname=` e per un DSN senza database (dove
decidono `PGDATABASE` o il nome dell'utente). Senza la variabile i test L4 sono SKIP, mai PASS. Ogni test che
apre il database ha il tag `integrazione`: `go test ./...` non lo tocca, anche con la variabile impostata.

## Dipendenze consentite

Solo `platform` e librerie. Mai `core`, mai `ai`, mai `transport`, mai `app`. Gli archi che esistono:
`config` → `db` (gli enum); `fondazioni` → `config`, `db`, `rete`; `coda` → `contratti/worker`, `db`,
`storage/staging`; `storage/staging` → `db`, `storage/nas`; `testutil` → `migrazioni` e il package radice del
modulo (`embed.go`, per le migrazioni incorporate). L'unica freccia verso `core` sta in un test
(`migrazioni/larghezze_db_test.go` → `core/inbox/classificazione`).

## Entry point

`config.CaricaConRete` (e `Carica`), `Config.Capacita`, `migrazioni.Applica` / `ApplicaFinoA` / `Elenca` /
`Applicate`, `fondazioni.SeedUtenti` / `Semina`, `rete.Prepara`, `rete.SoloDalleReti`, `logfile.Apri`,
`coda.Accoda` / `AccodaCon` / `AccodaStage` / `Claim` / `Completa` / `Fallisci` / `ImpostaCapacita` /
`AllineaCoda`, `staging.PercorsoContenuto` / `PulisciParti` / `Cache.Avvia`, `nas.Scrittore`, `nas.UNC`,
`archivio.Estrai`, `testutil.Pool` / `DatabaseDiTest`. Chi li chiama è nel README del package.

## Flussi principali

**La configurazione.** I valori predefiniti si mettono prima (`127.0.0.1:8080`, `info`, 64 MB, `shadow`,
cartelle `Inbox`/`Sent Items`, sync ogni 60 s, lotti da 50, 30 giorni di job, analizzatore versione 1,
staging automatico sotto 20 MB); `toml.DecodeFile` sovrascrive ciò che il file dice; le voci che il file
nomina e il codice non conosce diventano **avvisi** (non errori: un file scritto per un binario di prima
deve partire), come `segreto_sessione` e `[outlook].consenti_invio`, lette ma senza effetto. I percorsi
relativi (`[nas].radice`, `[nas].staging`, `[server].log_file`, `tls_cert`, `tls_key`) si leggono dalla
cartella del file; assoluti, UNC, percorsi che cominciano con una barra o con la lettera di un disco e `"-"`
restano come sono. Poi le `normalizza*` validano (DSN obbligatorio, `max_upload_mb ≥ 1`, rete, modalità,
fondazioni) e lo staging si crea. `[[postazione]]`, `[[casella]]` e `[[worker]]` descrivono chi è
**autorizzato** a che cosa: la connessione la fa il worker chiamando il server con il suo token.

**Le capacità (blocco 4).** Che cosa il server può modificare fuori da sé sono tre capacità:
`outlook_scrittura`, `bozze`, `nas_scrittura`. Tutto il resto legge e basta ed è sempre consentito. Tre
regole in `Capacita()`: `modalita = "shadow"` è un preset che spegne tutto ed è il valore quando la riga
manca; il silenzio vale «non scrivere»; `[nas].dry_run = true` spegne `nas_scrittura` ed è **deprecata**.
`consenti_nas_produzione` è il secondo consenso, richiesto quando `nas_scrittura` è accesa e `[nas].radice`
sta sotto una delle `radici_produzione`: senza, il server non parte.

**TLS e reti.** Senza TLS il server si rifiuta di ascoltare fuori dal loopback, salvo
`consenti_lan_in_chiaro`. Il certificato è autofirmato e i worker lo riconoscono dall'impronta scritta nel
loro `worker.toml`: niente CA. `reti_consentite` ammette solo reti della LAN; `SoloDalleReti` avvolge il
listener e chiude una connessione da fuori elenco prima del TLS; loopback e l'indirizzo di ascolto passano
sempre. `config.Rete` è la stessa rete data dalla riga di comando, e con `-ascolto` vale per intero al posto
delle voci di rete del file.

**La coda.** Un job si accoda una volta per chiave fra i pendenti, si prende con un claim che incrementa i
tentativi e dà un `lease_token`, si chiude solo esibendo quel token. Lease, durata massima, tentativi e
priorità sono per tipo (tabella in `coda/README.md`). Le capacità agiscono in due punti: un job che ne
chiede una spenta non entra in coda, e se c'era già non si consegna; quando una capacità si accende, i job
che avevano aspettato si annullano.

## Invarianti

- Il NAS non si sovrascrive mai: si scrive `.parte`, si verifica l'hash, si rinomina.
- Un percorso oltre i 250 caratteri passa dal prefisso long-path; sotto quella soglia non lo si aggiunge, e
  non lo si aggiunge due volte.
- L'estrazione di un archivio ha un budget sui byte scritti e rifiuta i percorsi che escono dalla cartella.
- `[sicurezza]`: il silenzio vale «non scrivere».
- `db` è generato: una modifica a mano si perde al prossimo `sqlc generate`.
- Una migrazione applicata non si riapplica e non si modifica; uno schema più recente del binario ferma
  l'avvio; un valore aggiunto a un enum non si usa nello stesso file.
- Un tentativo che non vale più non scrive niente.
- Un contenuto in staging si chiama con il proprio sha256: è di tutti gli allegati che lo hanno.
- `testutil` rifiuta un database il cui nome, risolto, non contiene «test».

## Effetti collaterali

`migrazioni` cambia lo schema; `fondazioni` scrive `utente`, `casella`, `postazione`, `worker_credenziale`;
`rete` scrive il certificato al primo avvio; `logfile` scrive su disco; `config` crea la cartella dello
staging; `storage/nas` scrive sul NAS; `storage/archivio` e `storage/staging` scrivono nello staging; `coda`
scrive `job` (e, per lo staging, lo stato di `allegato`). `db` e `contratti/worker` non hanno effetti
propri.

## Test

L1 per `config` (percorsi, esempio, rete, modalità), `rete`, `contratti/worker`, `coda` (le capacità),
`logfile` (rotazione, anche con il file bloccato), `migrazioni` (la verifica statica), `testutil` (la
guardia), `storage/staging`, `storage/nas`, `storage/archivio`. L3 per i contratti contro gli schemi di
`contracts/`. L4 (`-tags integrazione`) per `coda` (lease, tentativo, idempotenza, finestra del sync,
instradamento), `storage/staging` (la cache, la pulizia dei `.parte`), `migrazioni` (le migrazioni vere,
comprese le guardie della 0018, 0020 e 0021) e `fondazioni`.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una chiave TOML nuova | `config/config.go` (il campo, il default in `CaricaConRete`, le `normalizza*`), poi i due `*.example` |
| una query nuova | `db/queries/*.sql`, poi `sqlc generate` |
| una migrazione nuova | `migrations/`, poi `go test ./internal/platform/migrazioni/` (vedi `migrations/README.md`) |
| un tipo di job nuovo | l'enum `tipo_job` in una migrazione; `coda/coda.go` (`WorkerPer`, `LeaseSecondi`, `DurataMassimaS`, `MaxTentativiPer`); `coda/capacita.go` (`CapacitaPer`); `contratti/worker/tipi.go`; chi lo esegue: il `dispatch` del worker Python, oppure `app/runtime/esecutore.go:esegui` (e `vigilanza_nas.go:ScrivePerNas` se vuole il NAS) per un job del server |
| un campo nuovo nel contratto | `contratti/worker/tipi.go`, `workers/contratti.py`, `genera_contratti.py`, i test L3 |
| un utente nuovo, o un ruolo che l'avvio rifiuta | `config` (`[[utenti]]`), poi `fondazioni/utenti.go:SeedUtenti` |
| capire perché un'azione è bloccata | `config.Capacita()` e `coda/capacita.go` |
| capire perché un job non parte, o parte due volte | `coda/coda.go` (chiave di idempotenza, lease, tentativo) |

## Leggi anche

`internal/README.md`, `core/README.md`, `transport/README.md`, `../../migrations/README.md`,
`../../workers/workers_README.md`.
