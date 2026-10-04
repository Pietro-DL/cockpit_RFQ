---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/app/bancoa` — il banco del motore A senza DB

## Scopo

- Il **runner del banco** del motore A (giro 5, R24 a, R40 e). In A1a ha due modalità, tutte e due senza DB:
  - **`regole`** («profili attivi validi», A1a-P1): compila le grammatiche dell'indice con i limiti dell'indice
    (R43 B), ne verifica gli esempi e scrive per cliente hash, famiglie con ruoli e categorie, forme attive e
    riservate, riserve, esempi verificati e non verificati, le **lacune** della copertura minima di P1 §5.4
    (R20 b, informative) e la **coerenza** fra gli esempi con `rif_caso` e i casi degli attesi che ripetono
    (R47 b, CP-17);
  - **`casi`** (A1a-P2): esegue i `casi_contratto` degli attesi sul riconoscimento per forma (`Riconosci`) e
    confronta le letture con l'atteso, chiave per chiave. Esiti: passato, parziale, fallito, riservato,
    rimandato; un parziale, un riservato o un rimandato non è mai un passato.
- Legge tutto **da fuori il repository**, attraverso il manifest del dataset privato (`platform/dataset`):
  attesi, indice delle regole, grammatiche. Ogni file del manifest passa dal controllo di sha256 e byte;
  l'indice controlla gli sha256 delle grammatiche.
- Scrive il **rapporto** con l'esito in testa (R44): «ESITO: ESEGUITO — conforme», «ESITO: ESEGUITO — con
  differenze (n)» o «ESITO: NON ESEGUITO — <motivo>», più l'elenco dei controlli, ciascuno eseguito o non
  eseguito con il motivo. In JSON canonico e in testo, in modo atomico.
- È **l'unico importatore della libreria YAML** del modulo (G3): gli attesi entrano solo qui, tradotti nei DTO
  del runner. La libreria è `gopkg.in/yaml.v3 v3.0.1`, il ripiego offline di R9 (`go.yaml.in/yaml/v3` non è
  nella cache dei moduli; le due hanno la stessa API).

## Non appartiene qui

- **Riconoscere i codici**: lo fa il motore del prodotto (`motorea.CompilaInsieme`, `Motore.Riconosci`). Il
  runner è il giudice, non un secondo motore (R25): legge le letture e le confronta.
- **Nomi di clienti, profili, percorsi o codici reali**: nel codice ci sono solo i nomi neutri delle chiavi
  degli attesi (R47 a, M-21). I profili si legano ai clienti nel manifest (D-09); i valori attesi vengono
  dagli attesi a runtime e compaiono solo nei rapporti privati.
- **Il DB, gli export, la fotografia, le proposte di prodotti, il confronto con l'atteso dei prodotti e il
  gate**: le modalità `dsn` ed `exports` arrivano in A1c. `cockpit.toml` non si legge.
- **L'interpretazione** (`DaTesto`, `Interpreta`, funzione, ruoli): A1b. In A1a le chiavi che ne dipendono sono
  «rimandate», con il nome e la sessione.

## File

- **`bancoa.go`** — responsabilità:
  - commento `// Package`; `VersioneRapporto` (1); le modalità; i nomi delle voci del manifest (`attesi`,
    `regole.indice`);
  - `Opzioni`, `Esito` con `CodiceUscita` (0, 1, 3), `ErroreUso` (uscita 2), `Esegui`: manifest, profili,
    indice, attesi, la modalità, l'esito dai controlli (una differenza prevale su un non eseguito), il rapporto.
- **`attesi.go`** — responsabilità:
  - `LeggiAttesi`: la testata e i `casi_contratto`, con la decodifica stretta (chiavi sconosciute, ripetute,
    secondo documento: errori); le altre sezioni accettate e non lette;
  - `Attesi`, `Testata`, `CasoContratto`, `Precondizioni`, `ChiaveAttesa`, `ValoreAtteso` (il testo come è
    scritto: una revisione «00» resta «00»). Unico file con l'import della libreria YAML.
- **`traduzione.go`** — responsabilità:
  - `TraduciContesto` (`figlio_step.<campo>` → `nodo_step.<campo>`, R19 a; la notazione puntata di
    `evidenze.LeggiSelettore`, R19 b);
  - la tabella «chiave degli attesi → campo o predicato» del par.4.7.5, con le sessioni (A1a, A1b, decaduta):
    `ChiaveNota`, `SessioneChiave`, `ChiaviNote`; i predicati sulle letture.
- **`regole.go`** — responsabilità:
  - `VerificaRegole`, `RapportoRegole` e le sue parti: clienti, famiglie, riserve, esempi, lacune, coerenza.
- **`casi.go`** — responsabilità:
  - `EseguiCasiContratto`, `EsitoCaso`, `EsitoChiave`, `LetturaRapporto`, `ConteggiCasi`, `Conta`.
- **`rapporto.go`** — responsabilità:
  - `Rapporto`, `Versioni`, `Controllo`, `RapportoCasi`; `PrimaRiga`, `Testo`; `ScriviRapporto` (`.tmp`, poi
    `Rename`; rifiuta una cartella dentro il modulo).

## Entry point

- **`Esegui`, `Opzioni`, `Esito.CodiceUscita`, `ErroreUso`** — chi li chiama: `cmd/bancoa/main.go`.
- **`LeggiAttesi`, `VerificaRegole`, `EseguiCasiContratto`, `ScriviRapporto`** — chi li chiama: `Esegui`; da A1c
  le prove private sul dump, che passano dal runner (R54).

### La riga di comando (`cmd/bancoa/main.go`)

- `bancoa -modalita regole|casi -dataset <manifest del dataset privato> -uscita <cartella dei rapporti>`:
  - `-dataset` è lo stesso file di `COCKPIT_DATASET_A`; `-uscita` la cartella dei rapporti, che si crea se
    manca. Tutti e due fuori dal modulo.
- Codici d'uscita, gli stessi del riepilogo delle prove e del controllo prima del push (R44):
  - **0** eseguito e conforme;
  - **1** eseguito, con differenze: un errore di grammatica, un caso fallito, un controllo del runner fallito;
  - **2** uso o configurazione (flag sbagliati o mancanti, uscita o dataset dentro il modulo): nessun rapporto,
    la causa su stderr;
  - **3** NON ESEGUITO: il manifest manca o non si legge, una sua voce manca o ha un'impronta diversa.
- Il riepilogo va anche su stdout. Contiene dati privati: non si incolla in commit, PR o note pubbliche.

## Invarianti

- **Mai «conforme» se un ingresso non è stato letto**: senza dataset non esiste un rapporto verde. Una voce che
  manca o è cambiata dà NON ESEGUITO; un file presente ma non valido dà una differenza (par.3.6.5).
- **1 prevale su 3**: una differenza trovata si vede sempre, anche se qualcosa non è stato eseguito.
- **Una chiave degli attesi che la tabella non conosce rende gli attesi illeggibili**: una chiave nuova non si
  ignora. Una chiave che A1a non sa controllare è «rimandata» con la sessione, mai passata.
- **Confronto** (R25 c, d): esatto per base, basi, marcatore, affisso e revisione; per le altre chiavi il valore
  atteso sta fra quelli letti, e una lista vuota vuole zero valori letti (mai un passato per vuoto); una
  chiave che l'atteso non nomina non si controlla.
- **I casi sull'attributo della revisione in campo separato** (selettore `….revisione` o precondizioni) e le
  chiavi della funzione «menzione» si valutano in A1b; le letture d'identità si controllano in A1a solo
  sullo zero e solo sui selettori d'identità (nome del file, codice del cartiglio, id dello STEP).
- **La coerenza esempio/caso legge ogni chiave come la legge il runner** (`traduzione.go` è l'unica fonte della
  semantica di una chiave; par.4.10 n.3): `letture_identita = 0` contraddice le letture attese da un esempio
  solo sui selettori d'identità; per `basi` un esempio contraddice il caso solo se attende una base che il caso
  non elenca (non sa scrivere le ripetizioni). In un conflitto vincono gli attesi, e si corregge l'esempio.
- **Un caso definito con `dipende_da` si valuta**, e il rapporto lo annota; un caso con `stato_atteso:
  riservato` non si valuta.
- **Nessun ordine dipende da una mappa**: casi nell'ordine degli attesi, chiavi in ordine alfabetico, clienti
  nell'ordine dell'indice. Niente orologio nel rapporto.

## Dipendenze

- **Importa:** `platform/dataset`, `core/registro/regole/grammatica`, `core/inbox/classificazione/motorea`,
  `core/estrazione/evidenze`, `platform/jsoncanonico`, `github.com/google/uuid`, `gopkg.in/yaml.v3` (unico
  importatore), la libreria standard.
- **Non importa:** `app/runtime`, `transport/*`, `ai/*`, `platform/config`, `platform/db`.
- **È importato da:** `cmd/bancoa`.

## Test

- Nel ramo `-qa` (piano A, par.4.3, Q5), tutti L1 e sintetici, con il cliente inventato ACME e i file di
  `testdata/` dai nomi `_acme` (`attesi_acme.yaml`, `regole/indice_acme.v1.json`, `regole/acme.v1.json`,
  `manifest_acme.json`), copiati in `t.TempDir()` con gli sha256 calcolati sui byte copiati (A1a-BA):
  - **`attesi_test.go`** — la testata e i casi; chiavi sconosciute, ripetute o fuori posto; il testo dei valori
    come è scritto; le sezioni non lette;
  - **`traduzione_test.go`** — i contesti (R19); la tabella delle chiavi, completa e neutra; ogni chiave di A1a
    con il valore giusto e con uno sbagliato; la lista attesa vuota («nessuno»); le chiavi rimandate e
    decadute;
  - **`casi_test.go`** — gli esiti dei dieci casi ACME; il caso fallito con atteso, ottenuto e regola; profilo
    senza cliente, cliente scartato, contesto illeggibile; il determinismo;
  - **`regole_test.go`** — il rapporto delle regole; la coerenza con `rif_caso`, con la regola del runner per
    `letture_identita` e `basi`; senza attesi; cliente scartato
    e indice non valido;
  - **`rapporto_test.go`** — esito e codice d'uscita, prima riga, scrittura atomica fuori dal modulo, gli esiti
    di `Esegui` (conforme, con differenze, non eseguito, 1 su 3), gli errori d'uso senza rapporto.
- `cmd/bancoa/main_test.go` prova flag e codici d'uscita del comando (0, 1, 2, 3).
- Il controllo che la libreria YAML stia solo qui (G3) è in `core/estrazione/evidenze/dipendenze_test.go`.

## Leggi anche

- `internal/app/README.md`
- `internal/platform/dataset/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/core/registro/regole/grammatica/README.md`
- `internal/README.md`
