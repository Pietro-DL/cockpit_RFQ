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
  stretta, la forma canonica del registro (`FormaCanonica`: `jsoncanonico`, **mai NFC** — intervalli in byte,
  letterale = testo[inizio:fine] —, i membri null, "", [] e {} non si scrivono a ogni livello salvo `regole.partner`,
  voci in ordine di (rif, voce) e archi in ordine di (padre, figlio, allegato)), il limite di 12 MiB di testo canonico
  (`IMPORTAZIONE_TROPPO_GRANDE`), la completezza del contesto (§3.8.2, `PROPOSTA_NON_DISPONIBILE` /
  `registro_incompleto`), le quattro impronte con l'elenco chiuso del §3.8.1, la coerenza con le colonne della testata
  e la copia compatta di una lettura (§3.6.1) e il segnale visto (§7.1).

## Non appartiene qui

- **Costruire il registro dall'esito del Motore A**: e' l'adattatore `propostabom` (corsia motore, passo 3), che
  riempie `Contenuto` e chiama `Componi`.
- **Scriverlo e citarlo**: R2 nel formato 3 (corsia persistenza, passo 6), con `distintabozza.ParametriImportazione`
  e le query di `platform/db/queries/v1_formato3.sql`.
- **Le regole che vogliono il database** (V-1…V-7: la voce citata da una casella, la lettura legata alla voce, il
  partner attivo nel registro della decisione): chi chiama, con `VocePerRif`, `TrovaLettura`, `PartnerAttivi`.
- **Il perimetro del contesto**: quali file stanno nel perimetro del contesto risolto lo sa il motore, non il registro;
  per questo `contesto_id` e `impronta_contesto` dei file nel perimetro (§3.8.2) li controlla l'adattatore, che con un
  file del perimetro senza i due campi risponde `PROPOSTA_NON_DISPONIBILE` / `registro_incompleto`. Il DTO controlla il
  resto della completezza: partner, contesto delle regole, copia del contesto e `calcolo.impronta_indice`.

## File

- **`registro.go`**: `Contenuto` e i suoi livelli (`Gruppo`, `Regole`, `Versioni`, `Proposta`, `Voce`, `ArcoRegistro`,
  `FileRegistro`, `Relazione`, `FuoriPerDecisione`), `Testata` (le colonne), `Registro` (contenuto, byte canonici,
  impronta); `Componi` (porta il contenuto nella forma canonica, calcola le impronte parziali e chiude il registro),
  `Leggi` (rilettura stretta, null vietati nei campi del §3.8.2, forma canonica), `FormaCanonica`, `SenzaVuoti`,
  `Rileggi` e `VerificaColonne` (contro le colonne salvate: `ErrRegistroIncoerente`); `VocePerRif`, `FuoriPerRif`,
  `EVoceDellaRadice`, `PartiDellaVoce`, `PartnerAttivi`, `Grezzo`; `CodiceAmmissibile` e `RevAmmissibile` (le regole di
  `classificazione`, ripetute qui perche' la foglia non la importa: una prova di `distintabozza` le tiene uguali).
  Le regole di forma: voci con l'origine coerente (rif = nodo:<sha256>:<chiave>), codice e rev proposti ammissibili
  (la rev in maiuscole ASCII), la radice del gruppo fra le voci, gli estremi degli archi fra le voci o fra i fuori per
  decisione, ogni lettura con la sua `chiave_lettura` dell'allegato dell'unita' letta e unica nel registro (la stessa
  chiave porta la stessa lettura, byte per byte).
- **`impronte.go`**: l'elenco chiuso (le tabelle `campi*`, `Classificazione`, `CampiDellaRadiceFraLeEvidenze`) e
  `ImpronteParziali`: impronta_contenuto (struttura e identita'), impronta_evidenze (il resto della proposta, solo
  informativa), impronta_regole. La voce della radice (`origine.rif` = `gruppo.radice`) passa alle evidenze i campi che
  vengono dallo stato della RFQ (O-01). Le impronte si calcolano sulla forma canonica: un vuoto o un ordine diverso
  non le cambiano.
- **`lettura.go`**: `LetturaRegistro`, `LetturaCompatta` (la copia di una decisione `identita`, con la stessa forma
  canonica da tutte e due le parti, vuoti tolti anche dentro qualificatori e revisione: `StessaLettura` per V-1),
  `TrovaLettura`, `SegnaleVisto` e `SegnaliDellaVoce`.

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
    esponente, U+0000, chiavi ripetute, formato, gruppo `senza`, radice (anche senza la sua voce), voci (origine,
    codice e rev proposti), archi verso un rif che non e' una voce, letture (chiave assente, di un altro allegato, due
    letture con la stessa chiave), allegati citati, relazioni, segni, null nei campi del §3.8.2, impronte ritoccate o
    non ricalcolate;
  - la forma canonica: lo stesso contenuto con vuoti annidati (anche nel confronto) e gli elenchi al rovescio da' gli
    stessi byte e le stesse impronte, e Componi(Leggi(x).Contenuto) gli stessi byte di Leggi(x);
  - l'elenco chiuso (la parte strutturale di T-13): ogni campo tipizzato ha la sua classe, nessuna classe nomina un
    campo che non c'e', nessun orario o ID di calcolo nel confronto;
  - che cosa cambia quale impronta (C-15, C-19, O-01), null come assente;
  - il limite di peso, la completezza del contesto (anche `calcolo.impronta_indice` e i null con il contesto attivo),
    la copia compatta byte per byte (N-01), i segnali, le voci.
- Le prove con il database stanno in `distintabozza` (`conservazione_db_test.go`).

## Leggi anche

- `internal/core/rfq/distintabozza/README.md`, `migrations/README.md` (la 0023), `internal/platform/db/README.md`
  (`v1_formato3.sql`)
