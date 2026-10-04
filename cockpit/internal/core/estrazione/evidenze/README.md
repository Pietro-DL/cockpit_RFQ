---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/estrazione/evidenze` — il vocabolario e l'ingresso del motore A

## Scopo

- La **foglia comune** del motore A del giro 5 (R3, R49 C):
  - **dove si è letto**: `Contesto` (undici, chiusi), `VarianteCampo`, `CampoFonte`, `Selettore`, e la
    tabella delle coppie ammesse della parte 1 §4.2;
  - **dove, in byte**: `Intervallo`, `[Inizio, Fine)` in byte UTF-8 sul testo originale;
  - **come si segnala un problema**: il tipo `Diagnostica`, con `Gravita`, `Natura` ed `ErroreContratto`.
- Il documento delle evidenze (che cosa si è letto, quali segmenti valgono come richiesta) e le sue due
  validazioni si aggiungono a questa foglia nel commit successivo della stessa sessione (A1a.1b, R49 C).
- Nessuna interpretazione: la foglia non contiene regole dei clienti (parte 1 §7.2).

## Non appartiene qui

- **Costruire un documento** dai fatti dei worker, dalla mail o da un testo isolato: gli adattatori,
  `core/estrazione` (A1b). Lì si calcola anche il `BundleID`.
- **Il formato delle grammatiche** e la loro validazione: `core/registro/regole/grammatica` (A1a), che di
  qui usa solo il vocabolario dei selettori e il tipo `Diagnostica` (R41 a).
- **Riconoscere i codici** e interpretarli: `core/inbox/classificazione/motorea`.
- **Il catalogo dei codici** di tutto il motore: non c'è. Ogni pacchetto dichiara i codici che produce nel
  suo `codici_diagnostica.go` (R41 b); qui ci sono solo quelli della foglia.
- **Il JSON canonico e le impronte:** `platform/jsoncanonico`, che questa foglia non importa.
- **Che cosa una capacità riservata in A1 sia** (per esempio un selettore generico «cartiglio.*»): lo dice
  la grammatica. La foglia è vocabolario, senza politiche (D-12).

## File

- **`vocabolario.go`** — responsabilità:
  - commento `// Package`;
  - `Contesto` e le undici costanti `Contesto*`; `VarianteCampo` e le quattro costanti `Variante*`;
    `CampoFonte`; `Selettore`;
  - `LeggiSelettore` (la forma testuale degli attesi: «cartiglio.codice», «radice_step.id», «nome_file»),
    `String`, `MarshalText` / `UnmarshalText` (il JSON usa la forma testuale), `Valida`, `CampiAmmessi`;
  - `Intervallo`.
- **`diagnostica.go`** — responsabilità:
  - `Diagnostica`, `Gravita` (`GravitaErrore`, `GravitaAvviso`, `GravitaNota`), `Natura` (`NaturaContratto`,
    `NaturaDati`, `NaturaCapacita`, `NaturaLimite`), `ErroreContratto`.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che la foglia produce: `contratto.selettore_non_ammesso`, `contratto.selettore_generico`
    (selettori).

## Entry point

- **`LeggiSelettore`, `Selettore.Valida`, `CampiAmmessi`** — chi li chiama: dal giro 5, A1a, la validazione
  delle grammatiche; il runner degli attesi; in A1b gli adattatori.
- **`Diagnostica`, `ErroreContratto`** — chi li usa: tutti i pacchetti del motore A.
- Oggi, nel codice di prodotto, ancora nessuno: la foglia nasce prima dei suoi chiamanti (R49 C).

## Invarianti

- **È una foglia**: non importa niente del progetto (solo la libreria standard; con il documento anche
  `uuid`). Così la possono importare grammatiche, adattatori, motore, proposte, valutazione e confronto senza
  cicli.
- **Insiemi chiusi, nessun ripiego.** Un contesto fuori elenco (anche «figlio_step», che solo il runner degli
  attesi traduce in `nodo_step`, R19 a), una coppia fuori tabella, «nessuno» usato come jolly sono errori di
  contratto (`contratto.selettore_non_ammesso`); un generico «<contesto>.*» non è una coppia
  (`contratto.selettore_generico`). Il testo di un selettore si legge com'è: niente spazi tolti, niente
  maiuscole abbassate.
- **Ogni pacchetto dichiara i suoi codici** nel suo `codici_diagnostica.go`, come costanti «<area>.<caso>» in
  minuscolo; un pacchetto che sta sopra può emettere un codice di qui con la sua costante o passare avanti le
  diagnostiche che riceve, mai ridichiararlo. Una `Diagnostica` si costruisce sempre con una costante. Un
  codice pubblicato non cambia nome né significato; se non serve più resta, con il commento «ritirato».
- **Gli offset** (par.3.4.4 del piano A): `Intervallo` è in byte UTF-8, 0-based, con la fine esclusa, sul
  testo originale indicato, mai su un testo normalizzato; cade su un confine di runa.
- Niente orologio, file, rete, goroutine, `uuid.New`: è un pacchetto puro del motore A (G2).

## Dipendenze

- **Importa:** la libreria standard. Niente del progetto.
- **È importato da:** nessun pacchetto di prodotto, per ora (vedi Entry point).

## Test

- **`vocabolario_test.go`, `diagnostica_test.go`** — livello L1 — che cosa copre (A1a-VOC):
  - undici contesti; la tabella delle coppie; `LeggiSelettore` / `String` andata e ritorno, anche nel JSON e
    per «storia»;
  - «figlio_step.id», «cartiglio.id», «nome_file.codice», «nessuno» come jolly, spazi e maiuscole →
    `contratto.selettore_non_ammesso` con il percorso; «cartiglio.*» → `contratto.selettore_generico`;
  - la forma JSON di `Diagnostica`, gravità e nature, `ErroreContratto`.
- **`codici_diagnostica_test.go`** con **`testdata/codici_diagnostica.txt`** — livello L1 — che cosa copre
  (A1a-CAT): i codici di tutti i `codici_diagnostica.go` del motore A, letti dai sorgenti: formato, nessun
  doppione nemmeno fra pacchetti, nessun codice pubblicato sparito, nessun codice nuovo fuori dall'elenco
  d'oro, nessuna `Diagnostica` con il codice scritto sul posto.
- **`dipendenze_test.go`** — livello L1 — che cosa copre: G1 (import consentiti, anche transitivi sul
  progetto), G2 (niente orologio, file, rete, goroutine), G9 (il legacy non cambia import), nessun
  riferimento all'LLM nei sorgenti del motore. Ogni sessione lo estende con i suoi pacchetti.
- I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/platform/jsoncanonico/README.md`
- `internal/README.md`
