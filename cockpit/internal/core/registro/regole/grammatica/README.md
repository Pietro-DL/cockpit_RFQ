---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/registro/regole/grammatica` — le grammatiche v1 del motore A

## Scopo

- Il **contratto delle grammatiche cliente** del motore A del giro 5 (R3, R40 a): i file
  `regole/<cliente>.v1.json` del dataset privato.
  - **i tipi versionati**: famiglie di codice con ruoli e categorie, base a segmenti, forme (anche parziali,
    come proiezioni dichiarate), parti, etichette, affissi, revisioni, decorazioni, esempi, profilo con le
    riserve;
  - **la porta stretta di lettura** (`Decodifica`) e **la validazione** (`Grammatica.Valida`), con i
    **vincoli RE2** dei pattern (`VerificaPattern`) e le **capacità riservate in A1** (R20 c);
  - **i limiti**, che non sono costanti del codice: li dichiara l'indice delle regole con la loro versione;
    il codice tiene solo i **tetti** (R43 B).
  - la forma canonica con l'hash e l'indice delle grammatiche arrivano nel passo successivo (A1a.3).
- Tre esiti per ogni elemento (C-10): errore (lo snapshot non nasce), riservato con diagnosi (non attivo, il
  resto sì), attivo.

## Non appartiene qui

- **Il motore legacy** e lo schema di `cliente.regole`: `core/registro/regole`. Questo pacchetto sta nella
  sua cartella ma **non lo importa**, e il legacy non importa questo: i due formati non si mescolano (parte 1
  §8, §9.6). Una grammatica non si legge da `cliente.regole` e non lo modifica; i campi legacy «non
  coinvolti» (frasi del portale, mittenti di sistema…) restano lì, e in un file v1 sono una chiave
  sconosciuta.
- **Compilare e riconoscere**: `core/inbox/classificazione/motorea`, che compila lo snapshot in un motore
  immutabile, verifica gli esempi (`CompilaVerificato`) e applica i limiti del riconoscimento. Qui non si
  compila nessuna regex per il riconoscimento.
- **Leggere i file** (indice, grammatiche) e confrontare gli sha256: chi compone il processo (il banco,
  l'avvio dell'anteprima). Qui arrivano i byte.
- **I dati dei clienti**: le grammatiche vere, l'indice e i loro valori stanno nel dataset privato, mai nel
  repository.
- **Il documento delle evidenze**: `core/estrazione/evidenze`, di cui qui si usa solo il vocabolario dei
  selettori e il tipo `Diagnostica` (R41 a, G8).

## File

- **`grammatica.go`** — responsabilità:
  - commento `// Package`; `VersioneSchema` (1);
  - i tipi del DTO v1: `Grammatica`, `ClienteGrammatica`, `Profilo`, `Riserva`, `FamigliaCodice`, `Ruolo`,
    `Categoria`, `Base`, `ClasseConfine`, `SegmentoBase`, `FormaCodice`, `Parte`, `Etichetta`, `Affisso`,
    `ValoreQualificatore`, `RegolaRevisione`, `SegmentoRevisione`, `TokenSospeso`, `Decorazione`,
    `RegolaRiferimento`, `QualificatoreTestuale`, `EsempioCodice`, `AttesoEsempio`, `LetturaAttesa`.
- **`vocabolario.go`** — responsabilità:
  - gli enum chiusi e i loro elenchi: ruoli, categorie, tipi di parte (anche dentro un suffisso), tipi e
    sottotipi di decorazione, posizione, sorgente, stato, profilo, fase, destinazione, classi di confine,
    maiuscole, normalizzazione, significato dei segmenti di revisione, origine degli esempi, ambito.
- **`decodifica.go`** — responsabilità:
  - `Decodifica`; la lettura stretta comune a grammatiche e indice: BOM, UTF-8, surrogati soli, testo dopo
    l'oggetto, versione, poi un passaggio a token che conosce i tag del DTO (chiavi sconosciute, ripetute o
    con maiuscole diverse, null, numeri non interi, booleani e interi obbligatori assenti, array a lunghezza
    fissa, UUID), infine `json.Decoder` con `DisallowUnknownFields` e `UseNumber`.
- **`valida.go`** — responsabilità:
  - `Grammatica.Valida`: le quindici regole del piano (par.4.4.4), con un codice per ciascuna.
- **`capacita.go`** — responsabilità:
  - `VersioneCapacita` (`capacita-a1-1`) e l'elenco delle capacità riservate in A1 (R20 c): selettori e
    campi, generici «<contesto>.*», decorazioni della parte 1. «storia» non è riservato (R48 A).
- **`pattern.go`** — responsabilità:
  - `VerificaPattern` sull'albero di `regexp/syntax` e sul testo del pattern; i caratteri vietati nei
    pattern e nei letterali.
- **`limiti.go`** — responsabilità:
  - `Limiti` (`versione_limiti`; gruppi `grammatica`, `riconoscimento`, `anteprima`), `TettiLimiti`,
    `Limiti.Valida`. Nessun valore predefinito.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che il pacchetto produce (R41 b): la lettura (`contratto.*` della decodifica), la validazione
    (`contratto.*`), i pattern e i letterali (`grammatica.*`), capacità e riserve, `limite.superato` e
    `limite.oltre_tetto`.

## Entry point

- **`Decodifica`, `Grammatica.Valida`, `VerificaPattern`, `Limiti.Valida`, `TettiLimiti`** — chi li chiama:
  da A1a lo snapshot e l'indice delle grammatiche (in questo pacchetto, al passo successivo), poi `motorea`
  e il banco.
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **Il legacy non cambia e non si mescola**: nessun import fra questo pacchetto e `core/registro/regole`, in
  nessuna direzione (G9, e la tabella del par.3.1 nel controllo degli import).
- **Della foglia `evidenze` solo l'elenco chiuso** (R41 a): `Selettore`, `LeggiSelettore`, `Contesto` e le
  costanti `Contesto*`, `VarianteCampo`, `CampoFonte`, `CampiAmmessi`; `Diagnostica`, `Gravita`, `Natura` e
  le loro costanti, `ErroreContratto`. I DTO tengono i selettori come stringhe: la freccia serve alla
  validazione, non ai dati (G8).
- **Nessun ripiego.** Un valore fuori da un enum, una chiave sconosciuta, una coppia contesto/campo fuori
  tabella sono errori; «parte» e «finito» non sono ruoli (R6). Le diagnostiche della foglia
  (`contratto.selettore_non_ammesso`) passano avanti con il percorso nella grammatica, senza ridichiararle.
- **Ogni diagnostica ha un codice dichiarato** in `codici_diagnostica.go` e il percorso del campo, con l'ID
  degli elementi quando c'è («famiglie_codice[acme].forme[nome].parti[2]»). Le diagnostiche seguono
  l'ordine del file: due chiamate danno lo stesso elenco.
- **I limiti li riceve, non li sceglie** (R43 B): `Valida` e `VerificaPattern` li hanno come parametro; con
  limiti assenti, nulli o oltre i tetti nessuna grammatica si attiva. I tetti si alzano solo
  con un commit che lo dichiara.
- **Il legame con il cliente è per UUID** (`cliente.id`), mai per nome: la ragione sociale serve solo da
  controllo.
- `rif_caso` è un metadato opaco: si legge e si conserva, ma qui non ha effetti, e il motore non lo legge
  mai (R47 b).
- Niente orologio, file, rete, goroutine, `uuid.New`: è un pacchetto puro del motore A (G2). Nessuna mappa
  decide un ordine.

## Dipendenze

- **Importa:** `core/estrazione/evidenze` (solo l'elenco chiuso di G8), `github.com/google/uuid`, la libreria
  standard.
- **È importato da:** nessun pacchetto di prodotto, per ora (vedi Entry point).

## Test

- **`decodifica_test.go`** — livello L1 — che cosa copre (A1a-DEC): chiave sconosciuta (anche un campo legacy),
  ripetuta, con le maiuscole diverse; null; UTF-8 non valido; surrogati soli; BOM; testo dopo l'oggetto;
  numeri con decimali o esponente; booleani e interi obbligatori assenti; tipi sbagliati, UUID illeggibile,
  array a lunghezza fissa; annidamento; tutte le diagnostiche nell'ordine del file; la lettura riuscita.
- **`valida_test.go`** — livello L1 — che cosa copre (A-C01 pubblica e le quindici regole): enum ignoti
  (ruoli «finito» e «parte», categoria, tipo di parte e di decorazione, posizione, classe di confine),
  coppie vietate, `versione_schema` 0, 2, assente, ruoli vuoti; un caso per regola. Qui sta la grammatica
  ACME che usano tutte le prove del pacchetto.
- **`capacita_test.go`** — livello L1 — che cosa copre (A1a-CAP): i tre esiti; ogni capacità riservata con il
  suo avviso e il resto attivo; «storia» attivo; `VersioneCapacita` fissa.
- **`pattern_test.go`** — livello L1 — che cosa copre (A1a-PAT): ogni divieto di `VerificaPattern` con il suo
  codice; i pezzi ammessi; i limiti dall'indice; il «\b» scritto nel JSON.
- **`limiti_test.go`** — livello L1 — che cosa copre (A1a-LIM, validazione e tetti): i tetti fissi; limiti
  assenti, nulli, oltre il tetto; con limiti non validi nessuna grammatica; i limiti di validazione superati.
- Il controllo degli import (G1, G8) e dei codici (A1a-CAT) sta in `core/estrazione/evidenze`.
- I clienti dei test sono inventati (ACME); i test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/estrazione/evidenze/README.md`
- `internal/core/registro/README.md` (il legacy)
- `internal/README.md`
