---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/platform/dataset` — il manifest del dataset privato e i suoi sha256

## Scopo

- Legge il **manifest del dataset privato** della consegna A (giro 5): un JSON che sta fuori dai due rami e
  che elenca, per nome logico, i file privati con il loro percorso, il ruolo, lo sha256 e i byte.
- Dà un file a chi lo chiede **solo se è quello del manifest**: prima di restituire i byte controlla sha256 e
  byte. Un file che manca o che è cambiato è un errore tipizzato, che chi lo riceve mostra come
  **NON ESEGUITO** (R44): mai un salto, mai un file dato lo stesso.
- Tiene il legame **profilo degli attesi → cliente** (`Profili`, D-09): il codice pubblico non contiene nomi di
  clienti, e il runner lega i casi alle grammatiche attraverso il manifest.
- Controlla che una cartella stia **fuori dal modulo** (`FuoriDalModulo`): il dataset e le uscite del banco non
  entrano mai nel repository.

## Non appartiene qui

- **Il formato dei file elencati**: attesi (YAML, solo `app/bancoa`), indice delle regole e grammatiche
  (`core/registro/regole/grammatica`), elenchi del controllo prima del push (lo script `scripts/controlla-privati.ps1`).
- **Le diagnostiche del motore A** (`evidenze.Diagnostica`): `platform` non importa `core`. Qui ci sono solo
  errori.
- **Qualunque valore del dataset**: percorsi, impronte, profili, UUID, il nome della copia del dump. Stanno nel
  manifest, che è privato (R2); nemmeno le prove li contengono.
- **La copia del dump e i suoi controlli**: qui ci sono solo i tipi che il manifest riempie (`CopiaAttesa` per
  la copia intatta e per la copia `_run`); i controlli sul database li fa `platform/testutil` (`PoolDump`,
  `ControllaCopia`, `PoolCopiaDelDump`), da A1c.
- **La formula delle impronte di contenuto** e il controllo del loro contenuto (versione, nomi, sha256): li ha
  `platform/testutil` (`VersioneImpronta`, `ControllaCopia`). Qui c'è solo la forma della sezione `impronte`.

## File

- **`dataset.go`** — responsabilità:
  - commento `// Package`; `VersioneManifest` (1);
  - `Manifest`, `Voce`, `CopiaAttesa`, `RigaStorico`; le costanti dei ruoli (`attesi`, `regole`, `casi`,
    `dump`, `export`, `fixture`, `storico`, `controllo`);
  - da A1c (6.4.8): `Manifest.CopiaRun` (tag `copia_run`, la copia `_run` del dump, scrivibile: nome, ruolo,
    schema, sentinelle, con la forma di `copia` senza le impronte) e `Manifest.Export` (tag `export`,
    `*ExportDichiarato`: la terna dei fatti, `versione` e `hash_configurazione`, e lo `schema` del DB da cui vengono
    gli export, che gli export non dicono da soli; se la sezione c'è, ha tutte e tre le chiavi);
  - da A1c, R117 b (E2 §2.10): `CopiaAttesa.Impronte` (tag `impronte`, `*ImpronteCopia`, facoltativo): le impronte
    di contenuto della copia intatta, oltre ai conteggi delle sentinelle. `ImpronteCopia` ha `versione_impronta`
    (la versione della formula, `testutil.VersioneImpronta`) e `tabelle`, per nome di tabella; `ImprontaTabella`
    ha `colonne`, `ordine` e `sha256`. Forma nel manifest:
    `"copia": {…, "sentinelle": {"t": 3}, "impronte": {"versione_impronta": 1, "tabelle": {"t": {"colonne": ["id",
    "codice"], "ordine": ["id"], "sha256": "<64 cifre esadecimali>"}}}}`;
  - `Leggi`: la decodifica stretta (prima i token contro lo schema delle chiavi, poi `DisallowUnknownFields`) e
    la validazione delle voci e dei profili;
  - `Manifest.LeggiFile`, `Manifest.PercorsoDi`; gli errori `ErrFileMancante` ed `ErrImprontaDiversa`;
  - `FuoriDalModulo`.

## Entry point

- **`Leggi`, `Manifest.LeggiFile`, `Manifest.PercorsoDi`** — chi li chiama: il banco (`app/bancoa`), da A1a; da
  A1c gli aiuti delle prove private (`platform/testutil`: `DatasetA`, `FileDelDataset`).
- **`Manifest.Copia`, `Manifest.CopiaRun`** — chi li legge: da A1c le prove private, attraverso
  `testutil.PoolDump` e `testutil.PoolCopiaDelDump`; `Manifest.Copia.Impronte` solo `testutil.ControllaCopia` (il
  banco non le legge e non le calcola: T-B0-36). **`Manifest.Export`** — chi la legge: il lettore degli export
  del banco (A1c).
- **`FuoriDalModulo`** — chi lo chiama: il banco, per il manifest e per la cartella dei rapporti.
- **`ErrFileMancante`, `ErrImprontaDiversa`** — chi li controlla: il banco (uscita 3, NON ESEGUITO); da A1c le
  prove private (`NonEseguita`).

## Invarianti

- **Lettura stretta**, come le grammatiche: niente BOM, UTF-8 valido, nessun surrogato solo, un solo valore,
  nessuna chiave sconosciuta, ripetuta o con le maiuscole diverse dal tag, nessun null, solo numeri interi,
  le chiavi obbligatorie presenti. Una sezione nuova entra con il suo campo, non passa in silenzio: quelle di A1c
  sono `copia_run` ed `export`.
- **La sezione `export`**, se c'è, ha `versione` positiva, `hash_configurazione` di 64 cifre esadecimali e
  `schema` positivo: una terna a metà non si legge.
- **La sezione `impronte`** sta solo in `copia`: nella `copia_run` è una chiave sconosciuta, perché ogni ambiente
  ha la sua identità e le sue condizioni attese, mai condivise in automatico (R117). Se c'è, ha `versione_impronta`
  e `tabelle`, e ogni tabella ha `colonne`, `ordine` e `sha256`: un'impronta a metà non si legge. Il contenuto lo
  controlla `testutil`, che le calcola: un manifest con un'impronta che non si sa calcolare si legge, e la prova
  della copia lo dice NON ESEGUITA.
- **Limite della formula, versione 1** (`testutil.VersioneImpronta`; R-141, T-B6-214 precisata): si rendono solo i
  tipi di un elenco chiuso (bool, int2, int4, int8, numeric, text, varchar, bpchar, uuid, json, jsonb, date,
  timestamp, inet, gli enum e gli array di questi; timestamptz e timetz in UTC; un dominio per il suo tipo di base,
  a un livello). Ogni altro tipo (float, interval, bytea, money, range e multirange, compositi, array con un tempo
  con il fuso, un dominio su un dominio, time, …) dà l'errore «tipo non reso dalla versione 1», e l'impronta della
  sua tabella è una parte non eseguita: mai un'impronta che cambia con la sessione. Le altre tabelle si controllano
  lo stesso. Nello schema di oggi non c'è nessuna colonna così; renderne una vuole una versione nuova della formula
  e le impronte ricalcolate.
- **Un'impronta o una sentinella cambiata** non si aggiorna con il valore appena trovato: vuole una diagnosi e una
  riga di `storico` con il motivo (R117; per un'impronta, `voce` = `copia.impronte.<tabella>` e `sha256_prima` =
  l'impronta di prima).
- **Voci**: nome presente e unico, percorso relativo alla cartella del manifest, ruolo dell'elenco, sha256 di
  64 cifre esadecimali, byte non negativi.
- **Ogni lettura controlla sha256 e byte**; un file mancante o cambiato non si dà. Una voce che il manifest non
  ha è un file mancante.
- **I messaggi d'errore dicono la voce, mai il contenuto** del file.
- Nessun ordine dipende da una mappa: i profili si controllano in ordine di nome.

## Dipendenze

- **Importa:** solo la libreria standard. Niente del progetto, nessuna libreria esterna.
- **È importato da:** `internal/app/bancoa` e, da A1c, `internal/platform/testutil` (gli aiuti delle prove
  private, mai per gli attesi).

## Test

- **`dataset_test.go`** — livello L1 — che cosa copre (A1a-DS):
  - un manifest valido, con una voce del ruolo `controllo`, i profili e lo storico;
  - la decodifica stretta: BOM, UTF-8, surrogato, testo dopo l'oggetto, chiavi sconosciute, ripetute e con le
    maiuscole diverse, null, decimali, versione 2 o assente, chiave obbligatoria assente, ruolo, sha256,
    percorso assoluto, voce ripetuta, byte negativi, profilo senza cliente;
  - file mancante, sha256 o byte cambiati, voce assente → `ErrFileMancante`, `ErrImprontaDiversa`;
  - `FuoriDalModulo` dentro e fuori dal modulo, anche per una cartella che non esiste ancora;
  - la `VersioneManifest` fissa;
  - da A1c (A1c-L1-29): le sezioni `copia_run` ed `export` lette, con le loro chiavi strette (sconosciute,
    assenti, null, valori fuori dominio rifiutati);
  - da A1c, R117 b: la sezione `impronte` di `copia` letta (facoltativa: senza, `nil`), con le chiavi strette
    (sconosciute, assenti, ripetute, null, tipi sbagliati rifiutati), e rifiutata nella `copia_run`.
- I file del manifest nascono in `t.TempDir()`, con il cliente inventato ACME. I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/platform/README.md`
- `internal/app/bancoa/README.md`
- `internal/README.md`
