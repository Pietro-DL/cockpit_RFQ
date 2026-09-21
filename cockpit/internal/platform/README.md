# `platform/` — ciò che il server usa e che non sa niente di RFQ

## Scopo

Database, configurazione, migrazioni, TLS, log, NAS, archivi, contratti con i worker, utilità per i test.
Nessuno di questi package sa che cosa sia una RFQ, un cliente o un triage.

## Non appartiene qui

Regole di classificazione, handler HTTP, prompt dell'agente, decisioni dell'operatore.

## Package posseduti

| Package | Che cosa fa |
|---|---|
| `config` | legge `cockpit.toml`, mette i default, valida, normalizza percorsi e fondazioni; risolve `[sicurezza]` nelle tre capacità (`Capacita()`), `[staging]`, `[retention]`, `[nas].intervallo_integrita_s` |
| `db` | codice generato da **sqlc** da `db/queries/*.sql`: tipi, enum, una funzione per query. **Non si modifica a mano** |
| `migrazioni` | applica `migrations/*.sql` in ordine, una transazione per file, con verifica statica (ogni file registra la sua versione; ogni `REFERENCES` punta a una tabella già creata; un valore aggiunto a un enum non si usa nello stesso file) |
| `fondazioni` | semina da config `casella`, `postazione`, `worker_credenziale`, `utente` senza sovrascrivere ciò che è stato generato in UI; diagnosi delle credenziali |
| `rete` | certificato TLS autofirmato generato al primo avvio, impronta per i `worker.toml`, hash dei token |
| `logfile` | log rotante del server (`<staging>/log/cockpit.log`, 5 × 5 MB) |
| `contratti/worker` | i **contratti** JSON fra server e worker (`tipi.go`: payload dei job, richieste e risposte; `protocollo.go`: i tempi del claim e della presenza); specchio di `workers/contratti.py` e `workers/protocollo.py` |
| `storage/nas` | unico scrittore sul NAS: `.parte` + hash + rinomina, mai sovrascrive, long-path |
| `storage/archivio` | estrazione zip con budget sui byte scritti e protezione zip-slip |
| `testutil` | database di test usa e getta (`COCKPIT_TEST_DSN`, solo nomi con «test»; senza variabile i test L4 sono SKIP, mai PASS) |

## Dipendenze consentite

Solo `platform` e librerie. Mai `core`, mai `ai`, mai `transport`, mai `jobs`.

## Entry point

`config.Carica`, `config.Capacita`, `migrazioni.Applica`, `fondazioni.Semina`, `rete.CaricaOGenera`,
`nas.Scrittore`, `archivio.Estrai`, `testutil.Pool`.

## Flussi principali

**La configurazione.** Le righe `c.Server.Indirizzo = "127.0.0.1:8080"` sono **default**: subito dopo,
`toml.DecodeFile` sovrascrive ogni campo presente nel file, e le `normalizza*` validano (DSN obbligatorio,
`max_upload_mb ≥ 1`, ruoli utente fra quelli ammessi, percorsi resi assoluti rispetto alla cartella del toml,
fondazioni in forma canonica). `[[postazione]]`, `[[casella]]` e `[[worker]]` descrivono chi è **autorizzato**
a cosa: la connessione la fa il worker chiamando il server con il suo token. Nessuna porta sui PC.

**Le capacità (blocco 4).** Che cosa il server può modificare fuori da sé sono tre capacità:
`outlook_scrittura`, `bozze`, `nas_scrittura`. Tutto il resto legge e basta ed è sempre consentito. Tre regole
in `Capacita()`: `modalita = "shadow"` è un preset che spegne tutto ed è il valore quando la riga manca; il
silenzio vale «non scrivere» (`produzione` senza `[sicurezza]` non accende niente, e il log dice quale riga
aggiungere); `[nas].dry_run = true` spegne `nas_scrittura` ed è **deprecata**. `consenti_nas_produzione` è il
secondo consenso, richiesto quando `nas_scrittura` è accesa e `[nas].radice` sta sotto una delle
`radici_produzione`: senza, il server non parte.

**TLS.** `ETLS()` è vero se ci sono certificato e chiave. Senza TLS il server si rifiuta di ascoltare su un
indirizzo che non sia loopback: in chiaro passerebbero posta, token e cookie. `consenti_lan_in_chiaro = true` è
l'eccezione dichiarata per il banco di prova, non un default. Il certificato è autofirmato e i worker lo
riconoscono dall'impronta scritta nel loro `worker.toml` (modello SSH): niente CA.

## Invarianti

- Il NAS non si sovrascrive mai: si scrive `.parte`, si verifica l'hash, si rinomina.
- L'estrazione di un archivio ha un budget sui byte scritti e rifiuta i percorsi che escono dalla cartella.
- `[sicurezza]`: il silenzio vale «non scrivere».
- `db` è generato: una modifica a mano si perde al prossimo `sqlc generate`.
- `testutil` rifiuta un DSN che non contenga «test» nel nome del database.
- Una migrazione non usa nello stesso file un valore appena aggiunto a un enum.

## Effetti collaterali

`migrazioni` cambia lo schema; `fondazioni` scrive quattro tabelle; `rete` scrive il certificato al primo
avvio; `logfile` scrive su disco; `storage/nas` scrive sul NAS; `storage/archivio` scrive nello staging.
`config`, `db` e `contratti/worker` non hanno effetti propri.

## Test

L1 per `config`, `rete`, `contratti/worker`. L3 per i contratti contro gli schemi di `contracts/`
(`go test -count=1 ./internal/platform/contratti/worker/`, più `pytest` in `workers/`). L4 per `migrazioni`,
`fondazioni` e `testutil`.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una chiave TOML nuova | `config/config.go:Carica`, `normalizza*`, poi i due `*.example` |
| una query nuova | `db/queries/*.sql`, poi `sqlc generate` |
| una migrazione nuova | `migrations/`, poi `go test ./internal/platform/migrazioni/` |
| un campo nuovo nel contratto | `contratti/worker/tipi.go`, `workers/contratti.py`, `genera_contratti.py`, i test L3 |
| capire perché un'azione è bloccata | `config.Capacita()` e `internal/jobs/capacita.go` |

## Leggi anche

`internal/README.md`, `core/README.md`, `transport/README.md`, `workers/workers_README.md`.

---

**Cambia in B**: arrivano `coda` (la coda oggi in `internal/jobs`) e `storage/staging` (oggi
`jobs/stage.go`, `upload.go`, `cache.go`).
