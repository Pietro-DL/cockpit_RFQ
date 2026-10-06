---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `platform/migrazioni`

- lo schema portato all'ultima versione del binario

## Scopo

- Applica in ordine i file `migrations/NNNN_nome.sql` incorporati nel binario (`embed.go`, `risorse.FS`):
  - un file, una transazione;
  - prima di toccare il database verifica i file senza database (`VerificaStatica`).
- Rifiuta un database più recente del binario e uno con buchi nelle versioni.
- Da A1c (giro 5, P-02) apre anche il database **in sola lettura**, senza migrare (`lettura.go`): il pool con
  `default_transaction_read_only`, il controllo dello schema contro l'ultima migrazione incorporata, la
  destinazione di un DSN senza password e il controllo dei permessi di un collegamento che deve solo leggere.
  Serve al server (attraverso gli involucri di `app/runtime`), al banco del motore A e agli aiuti delle prove,
  che così non dipendono da `app/runtime`.

## Non appartiene qui

- Il contenuto delle migrazioni (`migrations/`),
- le query (`db/queries`, `platform/db`),
- il ritorno indietro (non esiste nel migratore: backup e script manuali in `scripts/`),
- il seed (`platform/fondazioni`).
- L'apertura del pool scrivibile del comando U5 (`ApriDatabaseSenzaMigrare`): resta in `app/runtime` (P-02), e
  per lo schema usa `UltimaApplicata` e `SchemaDiverso` di qui.
- Il testo del rimedio a uno schema diverso: `SchemaDiverso` è neutro, il Cockpit lo traduce in «backup, poi
  `-migra`» o «serve il cockpit.exe aggiornato» (`app/runtime`), il banco del motore A (A1c) in «una copia
  migrata dal proprietario».

## File

- **`migrazioni.go`**
  - Responsabilità:
    - `Migrazione`;
    - `Elenca` (legge e ordina, versioni 1..n senza buchi);
    - `Applicate` (le versioni in `schema_versione`);
    - `Applica`, `ApplicaFinoA`, `ApplicaElenco`;
    - `applicaUna` (una transazione per file);
    - `VerificaStatica` con le sue espressioni regolari
- **`lettura.go`** (A1c, P-02: per **aprire**, non per migrare; non chiama mai `Applica`, non aggiunge
  migrazioni: in A1 l'ultima resta la 0021, G10)
  - Responsabilità:
    - `ApriInLettura` (il pool con `default_transaction_read_only=on`; schema diverso → `*SchemaDiverso` e pool
      chiuso);
    - `UltimaApplicata` (il massimo delle versioni registrate; 0 = nessuna);
    - `SchemaDiverso` (database e binario a versioni diverse; testo neutro);
    - `Destinazione` («host:porta/nome come utente», con il nome risolto come lo legge pgx, senza password; un
      DSN illeggibile dà un errore che non lo ripete);
    - `Collegamento` e `ControllaSolaLettura` (dal catalogo, in quest'ordine: `default_transaction_read_only`,
      `current_user` = `session_user`, nessun attributo da superutente, nessuna appartenenza in
      `pg_auth_members`, nessun privilegio di scrittura su una relazione di `public` né `CREATE` sullo schema,
      nessuna tabella esclusa leggibile; il primo controllo che non passa dà un errore che lo nomina)

## Entry point

- **`Applica(ctx, pool, fsys, log)`**
  - Chi lo chiama:
    - `app/runtime.ApriDatabase` a ogni avvio (anche con `-migra`, `-semina-anagrafica` e `-importa-fornitori`);
    - `platform/testutil` (`SchemaPulito`, `SchemaPresente`).
    - **Non** la chiamano i comandi che leggono soltanto (`-conta-anagrafiche`, `-anteprima-fornitori`)
- **`Applicate(ctx, c)`**
  - Chi lo chiama:
    - `app/runtime.ApriDatabase`, per sapere la versione (serve a `fondazioni.UnaSolaCasellaAttiva` e all'analizzatore corrente dalla 20);
    - `ApriInLettura` e `app/runtime.ApriDatabaseSenzaMigrare`, per confrontarla con quella del binario;
    - da A1c il caricatore del motore A, sulla sua transazione (`Fotografia.SchemaDB`)
- **`Elenca(fsys)`**
  - Chi lo chiama: `ApriInLettura` e `app/runtime.ApriDatabaseSenzaMigrare`:
    - l'ultima migrazione incorporata è la versione del binario;
    - con uno schema diverso i comandi in sola lettura si fermano con un errore che dice che cosa fare (backup e `-migra`, o il binario aggiornato) e non toccano il database
- **`ApriInLettura(ctx, dsn, fsys)`**
  - Chi lo chiama: `app/runtime.ApriDatabaseInLettura` (i comandi `-conta-anagrafiche`, `-anteprima-fornitori`,
    `-calibrazione` e l'anteprima del comando U5), che traduce `SchemaDiverso` nel testo di sempre; da A1c il
    banco del motore A e `platform/testutil.PoolDump`
- **`UltimaApplicata`, `SchemaDiverso`**
  - Chi lo chiama: `ApriInLettura`; `app/runtime` (`ultimaApplicata`, `ApriDatabaseSenzaMigrare`,
    `spiegaSchema`); da A1c il caricatore del motore A
- **`Destinazione(dsn)`**
  - Chi lo chiama: `app/runtime.DestinazioneDelDSN` (la prima riga del comando U5), con il suo testo
    d'errore; da A1c il banco del motore A
- **`ControllaSolaLettura(ctx, c, escluse)`**
  - Chi lo chiama: da A1c il banco del motore A (`-dsn`) e `platform/testutil.PoolDump`, prima di qualunque
    lettura di dominio
- **`ApplicaFinoA`**
  - Chi lo chiama: `platform/testutil.SchemaFinoA` (test S2: dati alla versione precedente, poi la migrazione)
- **`ApplicaElenco`, `VerificaStatica`**
  - Chi lo chiama: i test di questo package (e `Applica` / `ApplicaFinoA`)

## Dati

- **`schema_versione (versione int PRIMARY KEY, applicata_il timestamptz)`**
  - creata dalla 0001;
  - ogni file inserisce la propria riga.
  - È l'**unica** traccia di che cosa è stato applicato: non si conserva un hash del file.
- **Lucchetto consultivo di sessione** `pg_advisory_lock(0x434f434b)` («COCK»)
  - preso su una connessione dedicata del pool per tutta la durata e rilasciato in `defer`:
    - due server che partono insieme non migrano in parallelo.
- **Una transazione per file** (`applicaUna`)
  - il testo intero passa in un solo `Exec` senza parametri;
  - prima del `COMMIT` si verifica che la riga della versione ci sia.

## Flussi principali

### L'avvio (`Applica` → `ApplicaElenco`)

1. `Elenca`:
   - ogni file di `migrations/` deve chiamarsi `NNNN_[a-z0-9_]+.sql`, altrimenti errore;
   - ordinati per versione, devono essere 1..n senza buchi;
2. `VerificaStatica` su **tutti** i file, anche quelli già applicati;
3. lucchetto consultivo;
4. `Applicate`: se `schema_versione` non esiste il database è vuoto;
5. versione massima del database **maggiore** dell'ultima del binario → errore «il DB è alla versione N ma il binario conosce solo la M: aggiornare cockpit.exe»:
   - il server non parte e non tocca niente;
6. una versione mancante sotto la massima → errore «DB incoerente, serve intervento manuale»;
7. ogni file non registrato e `≤ finoA` si applica in ordine;
   - il primo che fallisce annulla la propria transazione e ferma l'avvio (i precedenti restano applicati).

### La verifica statica (sul testo senza i commenti `--`)

- il file registra la propria versione (`INSERT INTO schema_versione (versione) VALUES (N)`, N uguale al numero del nome);
- ogni `REFERENCES t` punta a una tabella creata (`CREATE TABLE`) in quel file o in uno precedente;
- un valore aggiunto con `ALTER TYPE … ADD VALUE 'v'` non compare altrove nello stesso file come `'v'`.

### Fine riga

- Nessun hash, quindi un checkout CRLF (Windows con `core.autocrlf`) e uno LF danno lo stesso esito di verifica:
  - le espressioni regolari accettano `\r` come spazio.
- Il testo applicato è quello incorporato al momento della compilazione, CR compresi:
  - PostgreSQL li tratta come spazi,
  - e cambiano soltanto i testi letterali su più righe (i `COMMENT ON` lunghi) e il sorgente delle funzioni memorizzato nel catalogo.
- Le sequenze `E'\n'` non dipendono dal file.

## Invarianti

- Un file è una transazione: un errore a metà non lascia niente di quel file.
- Un file che non registra la propria versione non viene applicato (controllato prima, staticamente, e dopo, in transazione).
- Nessuna `REFERENCES` in avanti; nessun valore di enum usato nello stesso file in cui nasce (con il limite detto sopra: il controllo è testuale).
- Un database più recente del binario, o con buchi, non viene toccato.
- Due istanze non migrano insieme.
- **Dichiarato nel README principale e _non_ verificato dal codice:** «un file già applicato non va più modificato».
  - Una modifica a un file già registrato passa inosservata e non viene riapplicata.
- **L'apertura in lettura non migra e non scrive** (P-02): `lettura.go` non chiama mai `Applica`, e il pool di
  `ApriInLettura` ha `default_transaction_read_only=on` in ogni connessione. Con uno schema diverso il pool si
  chiude prima di qualunque lettura.
- **Nessun DSN e nessuna password negli errori di `Destinazione`**; `ControllaSolaLettura` fa solo letture del
  catalogo, e il primo controllo che non passa ferma tutto.

## Dipendenze

- Importa solo `pgx/v5` (`pgx`, `pgxpool`) e la libreria standard (da A1c anche `net` e `strconv`, per
  `Destinazione`): nessun pacchetto del progetto (G9).
- È importato da `app/runtime` e `platform/testutil`; da A1c anche dal caricatore del motore A
  (`core/fotorfq/caricatore`) e, con il banco sul DB, da `app/bancoa`.
- Nessuna violazione.

## Test

- **`migrazioni_test.go`**
  - Livello: L1
  - Che cosa prova:
    - la verifica statica delle migrazioni vere;
    - riferimento in avanti, enum usato nello stesso file, versione non registrata, versioni con buchi, nome non valido
- **`migrazioni_db_test.go`**
  - Livello: L4
  - Che cosa prova:
    - da vuoto e idempotente (S1),
    - fallimento a metà (S3),
    - file senza registrazione annullato,
    - database più recente del binario,
    - 0002 su dati esistenti (S2),
    - vincoli delle fondazioni
- **`caselle_db_test.go`**
  - Livello: L4
  - Che cosa prova: la 0004 su dati esistenti e il suo rifiuto con caselle ambigue
- **`classificazione_db_test.go`**
  - Livello: L4
  - Che cosa prova: la 0016 si ferma sulle richieste in risposta
- **`fascicolo0018_db_test.go`**
  - Livello: L4
  - Che cosa prova: le 13 guardie della 0018, fusioni, vincoli nuovi, `scripts/0018_indietro.sql`
- **`fascicolo0020_db_test.go`**
  - Livello: L4
  - Che cosa prova: guardie e trigger della 0020 (revisioni, BOM congelata, deroghe, analizzatore corrente, spostamento)
- **`fascicolo0021_db_test.go`**
  - Livello: L4
  - Che cosa prova: `scripts/0021_indietro.sql` e il suo rifiuto con note presenti
- **`larghezze_db_test.go`**
  - Livello: L4
  - Che cosa prova: le colonne `codice` e `rev` larghe almeno quanto `classificazione` dichiara
- **`lettura_test.go`** (A1c-L1-01)
  - Livello: L1
  - Che cosa prova:
    - `SchemaDiverso` neutro, `UltimaApplicata`, `Destinazione` (senza password, errore senza il DSN);
    - `ControllaSolaLettura` con un lettore finto, un controllo fallito per volta, ognuno con l'errore che lo nomina;
    - `lettura.go` non ha `ApriSenzaMigrare` e non chiama mai le funzioni che applicano le migrazioni
- **`lettura_db_test.go`** (A1c-L4S-01)
  - Livello: L4
  - Che cosa prova: `ApriInLettura` rifiuta la scrittura dal pool (25006) e uno schema alla versione prima
    (`SchemaDiverso`, senza migrare); `ControllaSolaLettura` sul database di prova legge il catalogo e ferma un
    ruolo che può scrivere
- **I test L4**
  - stanno nel package esterno `migrazioni_test`,
  - partono da `testutil.SchemaFinoA(N-1)` quando provano una migrazione su dati esistenti,
  - e girano con `-p 1`.
  - Portano il tag `integrazione` e distruggono lo schema:
    - `platform/testutil` li lascia partire solo se il database del DSN ha «test» nel nome, letto come lo legge pgx (`testutil.go:DatabaseDiTest`).
- **Fuori dal package**
  - `app/runtime/lettura_db_test.go` (L4) prova che i comandi in sola lettura non migrano e non scrivono,
  - e `app/runtime/esegui_test.go` (L1) il rifiuto di uno schema diverso da quello del binario.
- **Non provati:**
  - il lucchetto consultivo con due processi,
  - una migrazione modificata dopo l'applicazione.

## Stato dell'implementazione

- Completo, schema fino alla 0021 (Fascicolo v3).
- Nessun verso «giù»: il ritorno è il backup, e per la 0018 e la 0021 uno script manuale in `scripts/`.
- **Limiti:**
  - niente hash dei file applicati;
  - la verifica statica è per espressioni regolari (non riconosce, per esempio, `REFERENCES schema.tabella` né identificatori fra virgolette);
  - un file che richiede di stare fuori da una transazione (`CREATE INDEX CONCURRENTLY`) non si può scrivere.

## Dove intervenire

- **Voglio… una migrazione nuova**
  - Apro: `migrations/NNNN_nome.sql` (ultima riga: la versione), poi `sqlc generate` e `go test ./internal/platform/migrazioni/`
- **Voglio… una regola statica nuova**
  - Apro: `VerificaStatica` e un test negativo in `migrazioni_test.go`
- **Voglio… capire perché il server non parte dopo un aggiornamento**
  - Apro: l'errore di `ApplicaElenco` (versione più recente, buchi, file fallito)
- **Voglio… capire perché `-conta-anagrafiche` o `-anteprima-fornitori` rifiutano il database**
  - Apro: `lettura.go:ApriInLettura` (schema del database diverso da `Elenca` del binario) e il testo di
    `app/runtime/avvio.go:spiegaSchema`
- **Voglio… capire perché un collegamento «in sola lettura» viene rifiutato**
  - Apro: `lettura.go:ControllaSolaLettura` (l'errore nomina il controllo che non passa)
- **Voglio… provare una migrazione su dati esistenti**
  - Apro: un `*_db_test.go` con `testutil.SchemaFinoA(t, p, N-1)`

## Leggi anche

- `internal/platform/README.md`
- `internal/platform/testutil` (commento del package)
- il README principale («Aggiungere una migrazione», «Manutenzione»)
- `scripts/0018_indietro.sql`
- `scripts/0021_indietro.sql`
