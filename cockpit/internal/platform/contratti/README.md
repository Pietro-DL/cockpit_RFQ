---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `platform/contratti/worker`

- i contratti JSON fra server e worker

## Scopo

- I tipi Go di ciò che viaggia fra `cockpit.exe` e i worker Python:
  - richieste e risposte dell'API worker
  - payload e risultati per tipo di job
  - la struttura STEP letta dal worker di analisi
  - e i tempi del protocollo (claim, presenza, battito).
- È lo specchio di `workers/contratti.py` e `workers/protocollo.py`;
  - `contracts/*.schema.json` e `contracts/tempi_protocollo.json` sono generati dal lato Python (`workers/genera_contratti.py`)
  - e i test di questo package li confrontano con i tipi Go.
- Contiene solo tipi, costanti e tre funzioni pure:
  - `NelFuturo`
  - `PayloadSyncOutlook.ModoEffettivo`
  - `DecodificaStruttura`
- Nessun effetto.

## Non appartiene qui

- Chi riceve e valida le richieste (`transport/workerapi`)
- chi accoda i job (`platform/coda`, `core/*`, `transport/web`)
- che cosa significhi un risultato (`core/inbox/ingest`, `core/rfq/*`).

## File

- **`worker/tipi.go`** — responsabilità:
  - **ingest:** `MessaggioIn`, `AllegatoIn`, `Destinatario`, `IngestRichiesta`, `IngestRisposta`, `CursoreLotto`, `ElementoSaltato`, `EsitoMessaggio`
  - **coda:** `ClaimRichiesta`, `CasellaAperta`, `CasellaServita`, `Job`, `HeartbeatRichiesta`, `RisultatoRichiesta`
  - **payload e risultati dei job:**
    - `PayloadSyncOutlook` con `ModoEffettivo` e i tre `Modo*`
    - `CartellaCursore`, `CartellaEsito`, `RisultatoSync`
    - `PayloadStageAllegato`, `RisultatoStage`
    - `PayloadCreaBozza`, `RisultatoBozza`
    - `PayloadApriElemento`, `PayloadSpostaCartella`, `PayloadSegnaLetto`, `PayloadRileggiElemento`
    - `PayloadAnalizzaAllegato`, `RisultatoAnalisi`
    - e quelli dei job del server: `PayloadEstraiArchivio`, `PayloadCopiaNAS`, `PayloadCreaCartellaThread`, `PayloadAnalizzaMessaggioAI`
  - **la struttura STEP:** `StrutturaSTEP`, `NodoSTEP`, `RelazioneSTEP`, `ScartiSTEP`, `LimitiSTEP`, `DecodificaStruttura`
    - LS-01: `VersioneStruttura` (la forma accettata, 3), `VersioneDiagnostica` (1: geometria, istanze, stato),
      `VersioneStrutturaGP3` (la 4 ritirata), `DiagnosticaSTEP` (la porta per chi legge la diagnostica),
      `StrutturaDaRileggere` (il predicato di «Rianalizza»)
  - `TolleranzaFuturo` / `NelFuturo`
  - `Salute` (`/healthz`)
- **`worker/protocollo.go`** — responsabilità:
  - `AttesaClaim`
  - `AttesaClaimMax`
  - `PresenzaOnlineEntro` (= 2 × `AttesaClaim`)
  - `BattitoMax`

## Entry point

- **i tipi di richiesta e risposta**
  - Chi lo usa: `transport/workerapi` (claim, heartbeat, result, ingest, caselle)
- **`MessaggioIn`, `CursoreLotto`, `NelFuturo`, `PayloadSyncOutlook.ModoEffettivo`**
  - Chi lo usa: `core/inbox/ingest`, `platform/coda`, `transport/workerapi`
- **i `Payload*`**
  - Chi lo usa:
    - chi accoda:
      - `platform/coda` (sync, stage, copia)
      - `transport/web` (bozze, apri, segna letto, cartella, analisi AI)
      - `core/rfq/*` (estrai archivio)
      - `core/inbox/ingest` (rilettura)
    - chi esegue i job del server: `app/runtime/esecutore.go`
- **`StrutturaSTEP`, `DecodificaStruttura`**
  - Chi lo usa: `core/rfq/fascicolo`, `transport/web`, `transport/workerapi`; dal giro 5 (A1b) `core/estrazione`,
    l'adattatore STEP del motore A
- **`StrutturaDaRileggere`, `VersioneStruttura`, `VersioneDiagnostica`**
  - Chi lo usa: `core/rfq/fascicolo` (`AccodaStepDaRileggere`, solo dal gesto «Rianalizza»), `transport/workerapi`
    (gli avvisi nel log, con il worker: per una forma diversa da `VersioneStruttura`, e per la forma 3 con il campo
    `versione_diagnostica` assente o 0, la riga che dice anche l'attesa, `VersioneDiagnostica`); LS-01
- **`TestoPDF`, `DecodificaTestoPDF`**
  - Chi lo usa: `core/inbox/classificazione` (`testoDelPDF`); dal giro 5 (A1b) `core/estrazione`, l'adattatore PDF
    del motore A
- **`PresenzaOnlineEntro`, `AttesaClaim`, `AttesaClaimMax`**
  - Chi lo usa: `transport/web` (testata, postazioni), `transport/workerapi` (long-poll)
- **`Salute`**
  - Chi lo usa: `transport/web` (`/healthz`)

## Invarianti

- Ogni schema di `contracts/` ha un tipo Go corrispondente, campo per campo, e viceversa:
  - lo verifica `TestOgniSchemaCorrispondeAlTipoGo` insieme a `TestTuttiGliSchemiSonoConfrontati`.
- I valori degli enum del contratto (`direzione`, `natura`, `worker`, tipo di bozza) sono validi per l'enum del database;
  - dove il database è l'elenco completo, il contratto li ha tutti.
- I tempi del protocollo sono uguali nei due linguaggi;
  - `PresenzaOnlineEntro` è due attese di claim;
  - `BattitoMax` non supera `AttesaClaim`.
- Un campo sconosciuto in arrivo si ignora (compatibilità in avanti).
- Una struttura STEP di versione < 2 non si legge:
  - i codici scritti dal worker nella v1 non entrano.
- La versione della forma della struttura è una sola (LS-01): `VERSIONE_STRUTTURA` del worker Python, il default dello
  schema, `VersioneStruttura`, `valutazione.VersioneStrutturaStep` e il letterale di `struttura_motivo_parziale` nella
  migrazione che la definisce per ultima.
  - lo verificano `valutazione/versione_struttura_test.go` (L1) e, con il worker vero e il database,
    `transport/workerapi/ls01_struttura_db_test.go` (L4).
  - La diagnostica ha un numero suo (`versione_diagnostica`, default 0 = nessuna): non cambia mai `versione`, e la
    completezza non la guarda.
- Nessun percorso del disco del server nei payload verso i worker (`PayloadAnalizzaAllegato`, `RisultatoStage`):
  - i byte viaggiano con le rotte dei contenuti.

## Dipendenze

- Solo librerie (`encoding/json`, `time`, `google/uuid`);
  - il test importa `platform/db` per gli enum.
- Importato da:
  - `platform/coda`
  - `core/inbox/ingest`
  - `core/rfq/documenti`
  - `core/rfq/fascicolo`
  - `core/estrazione` (i fatti dello STEP e del testo dei PDF per il motore A)
  - `transport/web`
  - `transport/workerapi`
  - `app/runtime`
- Nessuna violazione.

## Test

- `worker/contratti_test.go`, senza tag:
  - è la parte Go dei test L3 e gira anche con `go test ./...`.
  - Legge `contracts/` con un percorso relativo (`../../../../contracts`).

### Che cosa prova

- **`TestOgniSchemaCorrispondeAlTipoGo`, `TestTuttiGliSchemiSonoConfrontati`**
  - K1/K2: stessi campi e stessa forma JSON, modelli annidati con lo stesso nome
  - nessuno schema senza tipo
- **`TestITempiDelProtocolloSonoQuelliDeiWorker`**
  - i tempi di `tempi_protocollo.json` contro `protocollo.go`
- **`TestGliEnumDelContrattoSonoQuelliDelDatabase`**
  - K3
- **`TestUnCampoSconosciutoNonRompeLaLettura`**
  - K4
- **`TestLaStrutturaNonPortaCodici`, `TestDecodificaStrutturaTolleraCampiSconosciuti`, `TestUnaStrutturaTroncataDiceControQualeTetto`, `TestUnaStrutturaVecchiaNonSiLegge`, `TestLaStrutturaV3PortaGliScartiInNumeri`**
  - `StrutturaSTEP` v2/v3 e `DecodificaStruttura`
- **`TestStrutturaV3ConDiagnostica`** (LS-01; prima `TestStrutturaV4`)
  - la v3 con la diagnostica 1 si legge intera e non si rilegge; la v3 senza diagnostica non si rilegge; la v4 di GP-3,
    una v5, una v2 e i fatti senza struttura si rileggono; una diagnostica di un'altra versione si ignora

### Che cosa non si confronta

- Non si confronta l'obbligatorietà dei campi (in Go non esiste).
- I payload dei job eseguiti dal server non hanno schema: non viaggiano verso i worker.
  - `PayloadEstraiArchivio`, `PayloadCopiaNAS`, `PayloadCreaCartellaThread`, `PayloadAnalizzaMessaggioAI`
  - e `Salute`

## Stato dell'implementazione

- **Completo** fino a B8.5 (struttura STEP v3 con gli scarti).
- **LS-01** (17/10/2026): la diagnostica di GP-3 (geometria, istanze, stato) viaggia nella forma 3 con
  `versione_diagnostica` = 1; la forma 4 di GP-3 è ritirata (`VersioneStrutturaGP3`) e si rilegge con «Rianalizza».
- **In attesa:**
  - `PayloadSpostaCartella` / `RisultatoSposta` sono nel contratto
    - ma il server non accoda mai un `sposta_in_cartella`.
  - `PayloadRileggiElemento` lo accoda il server (`core/inbox/ingest/replay.go`),
    - ma il worker Outlook non esegue `rileggi_elemento`.
  - `EsitoMessaggio.AllegatiDaStage` non viene mai valorizzato (resta 0 anche con lo staging automatico).

## Dove intervenire

- **Voglio… un campo nuovo in un contratto**
  - Apro:
    - `worker/tipi.go`
    - `workers/contratti.py`
    - `python workers/genera_contratti.py`
    - poi questo test e `pytest` in `workers/`
- **Voglio… un tipo di job nuovo con worker esterno**
  - Apro:
    - un `Payload*` / `Risultato*` qui
    - lo schema dal lato Python
    - una voce in `tipiContratto`
- **Voglio… un job eseguito dal server**
  - Apro:
    - solo il `Payload*` qui (nessuno schema)
    - l'esecuzione in `app/runtime/esecutore.go`
- **Voglio… cambiare i tempi del claim o della presenza**
  - Apro:
    - `worker/protocollo.go` e `workers/protocollo.py`
    - poi rigenerare `tempi_protocollo.json`

## Leggi anche

- `internal/transport/README.md` (le rotte worker)
- `internal/platform/README.md`
- `workers/workers_README.md`
- `contracts/`
