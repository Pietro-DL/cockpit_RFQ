---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/rfq/distintabozza/importazione` — il registro d'importazione del Motore A (V1)

## Scopo

- Giro 5, A1d, V1 (direttiva 23; emendamento di V1 al contratto rev. 3, EM §3.8; piano della 0023, passo 2 del §14).
- Il **registro d'importazione**: la proposta del Motore A per (RFQ, prodotto, gruppo), immutabile, che la tabella
  `distinta_importazione` della 0023 conserva e che la bozza formato 3 cita con `sorgente.importazione`.
- Qui stanno il DTO del contenuto (nomi minuscoli, un elenco chiuso di campi per ogni livello tipizzato), la lettura
  stretta, la forma canonica (`jsoncanonico`, **mai NFC**: intervalli in byte, letterale = testo[inizio:fine]), il
  limite di 12 MiB di testo canonico (`IMPORTAZIONE_TROPPO_GRANDE`), la completezza del contesto (§3.8.2,
  `PROPOSTA_NON_DISPONIBILE` / `registro_incompleto`), le quattro impronte con l'elenco chiuso del §3.8.1, la coerenza
  con le colonne della testata e la copia compatta di una lettura (§3.6.1) e il segnale visto (§7.1).

## Non appartiene qui

- **Costruire il registro dall'esito del Motore A**: e' l'adattatore `propostabom` (corsia motore, passo 3), che
  riempie `Contenuto` e chiama `Componi`.
- **Scriverlo e citarlo**: R2 nel formato 3 (corsia persistenza, passo 6), con `distintabozza.ParametriImportazione`
  e le query di `platform/db/queries/v1_formato3.sql`.
- **Le regole che vogliono il database** (V-1…V-7: la voce citata da una casella, la lettura legata alla voce, il
  partner attivo nel registro della decisione): chi chiama, con `VocePerRif`, `TrovaLettura`, `PartnerAttivi`.

## File

- **`registro.go`**: `Contenuto` e i suoi livelli (`Gruppo`, `Regole`, `Versioni`, `Proposta`, `Voce`, `ArcoRegistro`,
  `FileRegistro`, `Relazione`, `FuoriPerDecisione`), `Testata` (le colonne), `Registro` (contenuto, byte canonici,
  impronta); `Componi` (calcola le impronte parziali e chiude il registro), `Leggi` (rilettura stretta), `Rileggi` e
  `VerificaColonne` (contro le colonne salvate: `ErrRegistroIncoerente`); `VocePerRif`, `FuoriPerRif`,
  `EVoceDellaRadice`, `PartiDellaVoce`, `PartnerAttivi`, `Grezzo`.
- **`impronte.go`**: l'elenco chiuso (le tabelle `campi*`, `Classificazione`, `CampiDellaRadiceFraLeEvidenze`) e
  `ImpronteParziali`: impronta_contenuto (struttura e identita'), impronta_evidenze (il resto della proposta, solo
  informativa), impronta_regole. La voce della radice (`origine.rif` = `gruppo.radice`) passa alle evidenze i campi che
  vengono dallo stato della RFQ (O-01). Un campo null vale come assente.
- **`lettura.go`**: `LetturaRegistro`, `LetturaCompatta` (la copia di una decisione `identita`, con la stessa forma
  canonica da tutte e due le parti: `StessaLettura` per V-1), `TrovaLettura`, `SegnaleVisto` e `SegnaliDellaVoce`.

## Le proiezioni delle impronte [T]

- Il §3.8.1 dice quali campi entrano in quale impronta, non con che forma. La forma la fissa `impronte.go`: un oggetto
  per impronta, con le chiavi dei livelli del contenuto (`fonte`, `gruppo`, `nodi`, `archi`, `relazioni`,
  `fuori_per_decisione`, `file`, `diagnostiche`), le voci nell'ordine del registro e ognuna con la sua chiave `voce`;
  dei riferimenti documentali della radice, fra le evidenze, `riferimenti_documentali[{indice, discordanze,
  revisione_inferiore}]`.
- Un cambio dell'elenco o della forma vuole `importazione-2` (EM §3.8.1): `Leggi` ricalcola e confronta le impronte
  solo per `importazione-1`.

## Dipendenze

- Importa: la libreria standard, `google/uuid`, `platform/jsoncanonico`. Nient'altro del progetto: niente database,
  niente orologio, niente `core/rfq`.
- **Limite noto**: la lista chiusa dei pacchetti puri (`core/estrazione/evidenze/dipendenze_test.go`, `vietatiAiPuri`)
  vieta a un pacchetto puro tutto `internal/core/rfq/`. Perche' `propostabom` usi questo DTO serve l'eccezione per nome
  in quella lista, oppure lo spostamento del pacchetto in una foglia fuori da `core/rfq` (proposta di emendamento e
  richiesta alla corsia motore nel resoconto della corsia persistenza di V1).

## Test

- **L1** (`go test ./internal/core/rfq/distintabozza/importazione/`), `registro_test.go`:
  - `Componi` e `Leggi` (stessi byte anche dal testo di jsonb, deterministico), le colonne e `VerificaColonne`;
  - la lettura stretta: campi sconosciuti e nomi con la maiuscola a ogni livello tipizzato, numeri non interi o con
    esponente, U+0000, chiavi ripetute, formato, gruppo `senza`, radice, voci, allegati citati, relazioni, segni,
    impronte ritoccate o non ricalcolate;
  - l'elenco chiuso (la parte strutturale di T-13): ogni campo tipizzato ha la sua classe, nessuna classe nomina un
    campo che non c'e', nessun orario o ID di calcolo nel confronto;
  - che cosa cambia quale impronta (C-15, C-19, O-01), null come assente;
  - il limite di peso, la completezza del contesto, la copia compatta byte per byte (N-01), i segnali, le voci.
- Le prove con il database stanno in `distintabozza` (`conservazione_db_test.go`).

## Leggi anche

- `internal/core/rfq/distintabozza/README.md`, `migrations/README.md` (la 0023), `internal/platform/db/README.md`
  (`v1_formato3.sql`)
