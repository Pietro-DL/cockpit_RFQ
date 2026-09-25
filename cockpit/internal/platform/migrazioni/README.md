# `platform/migrazioni` — lo schema portato all'ultima versione del binario

## Scopo

Applica in ordine i file `migrations/NNNN_nome.sql` incorporati nel binario (`embed.go`, `risorse.FS`):
un file, una transazione; prima di toccare il database verifica i file senza database
(`VerificaStatica`). Rifiuta un database più recente del binario e uno con buchi nelle versioni.

## Non appartiene qui

Il contenuto delle migrazioni (`migrations/`), le query (`db/queries`, `platform/db`), il ritorno
indietro (non esiste nel migratore: backup e script manuali in `scripts/`), il seed (`platform/fondazioni`).

## File

| File | Responsabilità |
|---|---|
| `migrazioni.go` | `Migrazione`; `Elenca` (legge e ordina, versioni 1..n senza buchi); `Applicate` (le versioni in `schema_versione`); `Applica`, `ApplicaFinoA`, `ApplicaElenco`; `applicaUna` (una transazione per file); `VerificaStatica` con le sue espressioni regolari |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Applica(ctx, pool, fsys, log)` | `app/runtime.ApriDatabase` a ogni avvio (anche con `-migra`, `-semina-anagrafica` e `-importa-fornitori`); `platform/testutil` (`SchemaPulito`, `SchemaPresente`). **Non** la chiamano i comandi che leggono soltanto (`-conta-anagrafiche`, `-anteprima-fornitori`) |
| `Applicate(ctx, c)` | `app/runtime.ApriDatabase`, per sapere la versione (serve a `fondazioni.UnaSolaCasellaAttiva` e all'analizzatore corrente dalla 20); `app/runtime.ApriDatabaseInLettura`, per confrontarla con quella del binario |
| `Elenca(fsys)` | `app/runtime.ApriDatabaseInLettura`: l'ultima migrazione incorporata è la versione del binario; con uno schema diverso i comandi in sola lettura si fermano con un errore che dice che cosa fare (backup e `-migra`, o il binario aggiornato) e non toccano il database |
| `ApplicaFinoA` | `platform/testutil.SchemaFinoA` (test S2: dati alla versione precedente, poi la migrazione) |
| `ApplicaElenco`, `VerificaStatica` | i test di questo package (e `Applica` / `ApplicaFinoA`) |

## Dati

- `schema_versione (versione int PRIMARY KEY, applicata_il timestamptz)`: creata dalla 0001; ogni file
  inserisce la propria riga. È l'**unica** traccia di che cosa è stato applicato: non si conserva un
  hash del file.
- **Lucchetto consultivo di sessione** `pg_advisory_lock(0x434f434b)` («COCK»), preso su una connessione
  dedicata del pool per tutta la durata e rilasciato in `defer`: due server che partono insieme non
  migrano in parallelo.
- **Una transazione per file** (`applicaUna`): il testo intero passa in un solo `Exec` senza
  parametri; prima del `COMMIT` si verifica che la riga della versione ci sia.

## Flussi principali

**L'avvio** (`Applica` → `ApplicaElenco`):

1. `Elenca`: ogni file di `migrations/` deve chiamarsi `NNNN_[a-z0-9_]+.sql`, altrimenti errore;
   ordinati per versione, devono essere 1..n senza buchi;
2. `VerificaStatica` su **tutti** i file, anche quelli già applicati;
3. lucchetto consultivo;
4. `Applicate`: se `schema_versione` non esiste il database è vuoto;
5. versione massima del database **maggiore** dell'ultima del binario → errore
   «il DB è alla versione N ma il binario conosce solo la M: aggiornare cockpit.exe»: il server non
   parte e non tocca niente;
6. una versione mancante sotto la massima → errore «DB incoerente, serve intervento manuale»;
7. ogni file non registrato e `≤ finoA` si applica in ordine; il primo che fallisce annulla la propria
   transazione e ferma l'avvio (i precedenti restano applicati).

**La verifica statica** (sul testo senza i commenti `--`):

- il file registra la propria versione (`INSERT INTO schema_versione (versione) VALUES (N)`, N uguale
  al numero del nome);
- ogni `REFERENCES t` punta a una tabella creata (`CREATE TABLE`) in quel file o in uno precedente;
- un valore aggiunto con `ALTER TYPE … ADD VALUE 'v'` non compare altrove nello stesso file come `'v'`.

**Fine riga.** Nessun hash, quindi un checkout CRLF (Windows con `core.autocrlf`) e uno LF danno lo
stesso esito di verifica: le espressioni regolari accettano `\r` come spazio. Il testo applicato è
quello incorporato al momento della compilazione, CR compresi: PostgreSQL li tratta come spazi, e
cambiano soltanto i testi letterali su più righe (i `COMMENT ON` lunghi) e il sorgente delle funzioni
memorizzato nel catalogo. Le sequenze `E'\n'` non dipendono dal file.

## Invarianti

- Un file è una transazione: un errore a metà non lascia niente di quel file.
- Un file che non registra la propria versione non viene applicato (controllato prima, staticamente, e
  dopo, in transazione).
- Nessuna `REFERENCES` in avanti; nessun valore di enum usato nello stesso file in cui nasce (con il
  limite detto sopra: il controllo è testuale).
- Un database più recente del binario, o con buchi, non viene toccato.
- Due istanze non migrano insieme.

Dichiarato nel README principale e **non** verificato dal codice: «un file già applicato non va più
modificato». Una modifica a un file già registrato passa inosservata e non viene riapplicata.

## Dipendenze

Importa solo `pgx/v5`. È importato da `app/runtime` e `platform/testutil`. Nessuna violazione.

## Test

| File | Livello | Che cosa prova |
|---|---|---|
| `migrazioni_test.go` | L1 | la verifica statica delle migrazioni vere; riferimento in avanti, enum usato nello stesso file, versione non registrata, versioni con buchi, nome non valido |
| `migrazioni_db_test.go` | L4 | da vuoto e idempotente (S1), fallimento a metà (S3), file senza registrazione annullato, database più recente del binario, 0002 su dati esistenti (S2), vincoli delle fondazioni |
| `caselle_db_test.go` | L4 | la 0004 su dati esistenti e il suo rifiuto con caselle ambigue |
| `classificazione_db_test.go` | L4 | la 0016 si ferma sulle richieste in risposta |
| `fascicolo0018_db_test.go` | L4 | le 13 guardie della 0018, fusioni, vincoli nuovi, `scripts/0018_indietro.sql` |
| `fascicolo0020_db_test.go` | L4 | guardie e trigger della 0020 (revisioni, BOM congelata, deroghe, analizzatore corrente, spostamento) |
| `fascicolo0021_db_test.go` | L4 | `scripts/0021_indietro.sql` e il suo rifiuto con note presenti |
| `larghezze_db_test.go` | L4 | le colonne `codice` e `rev` larghe almeno quanto `classificazione` dichiara |

I test L4 stanno nel package esterno `migrazioni_test`, partono da `testutil.SchemaFinoA(N-1)` quando
provano una migrazione su dati esistenti, e girano con `-p 1`. Portano il tag `integrazione` e distruggono
lo schema: `platform/testutil` li lascia partire solo se il database del DSN ha «test» nel nome, letto
come lo legge pgx (`testutil.go:DatabaseDiTest`). Fuori dal package, `app/runtime/lettura_db_test.go`
(L4) prova che i comandi in sola lettura non migrano e non scrivono, e `app/runtime/esegui_test.go` (L1)
il rifiuto di uno schema diverso da quello del binario. Non provati: il lucchetto consultivo con due
processi, una migrazione modificata dopo l'applicazione.

## Stato dell'implementazione

Completo, schema fino alla 0021 (Fascicolo v3). Nessun verso «giù»: il ritorno è il backup, e per la
0018 e la 0021 uno script manuale in `scripts/`. Limiti: niente hash dei file applicati; la verifica
statica è per espressioni regolari (non riconosce, per esempio, `REFERENCES schema.tabella` né identificatori
fra virgolette); un file che richiede di stare fuori da una transazione (`CREATE INDEX CONCURRENTLY`) non
si può scrivere.

## Dove intervenire

| Voglio… | Apro |
|---|---|
| una migrazione nuova | `migrations/NNNN_nome.sql` (ultima riga: la versione), poi `sqlc generate` e `go test ./internal/platform/migrazioni/` |
| una regola statica nuova | `VerificaStatica` e un test negativo in `migrazioni_test.go` |
| capire perché il server non parte dopo un aggiornamento | l'errore di `ApplicaElenco` (versione più recente, buchi, file fallito) |
| capire perché `-conta-anagrafiche` o `-anteprima-fornitori` rifiutano il database | `app/runtime/avvio.go:ApriDatabaseInLettura` (schema del database diverso da `Elenca` del binario) |
| provare una migrazione su dati esistenti | un `*_db_test.go` con `testutil.SchemaFinoA(t, p, N-1)` |

## Leggi anche

`internal/platform/README.md`, `internal/platform/testutil` (commento del package), il README principale
(«Aggiungere una migrazione», «Manutenzione»), `scripts/0018_indietro.sql`, `scripts/0021_indietro.sql`.
