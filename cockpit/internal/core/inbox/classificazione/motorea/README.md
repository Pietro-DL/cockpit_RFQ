---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/inbox/classificazione/motorea` — il motore A: compilatore e riconoscimento per forma

## Scopo

- Il **motore A** del giro 5 (R3, R40 b). In A1a fa due cose:
  - **compila** la grammatica v1 di un cliente (`grammatica.SnapshotRegole`) in un `Motore` immutabile e ne
    **verifica tutti gli esempi** con lo stesso riconoscimento (`CompilaVerificato`); per tutti i clienti
    dell'indice, con i controlli di file, sha256 e cliente (`CompilaInsieme`, `MotoreDi`);
  - **riconosce le forme** su un testo, per un selettore: ogni coppia (famiglia, forma) ha le sue regex,
    compilate una volta, mai tutte in alternanza (v3 §4.3), e restituisce le **letture di forma** con gli intervalli in byte
    (`Riconosci`, `LetturaForma`).
- Confronta basi e revisioni senza inventare niente (`ConfrontaBasi`, `ConfrontaRevisioni`): una base
  parziale resta compatibile con tutti i suoi completamenti (A-C07), una revisione si confronta solo con le
  equivalenze dichiarate.
- Tre esiti per ogni elemento della grammatica (C-10): errore (nessun motore), riservato con diagnosi (non
  entra nel motore, il resto sì), attivo.

## Non appartiene qui

- **Niente `Minuteria`, niente motore legacy** (R7): `motorea` sta nella cartella di
  `core/inbox/classificazione` ma **non la importa**, e `classificazione` non importa `motorea`. La categoria
  «minuteria» di una lettura viene solo dalla famiglia dichiarata nella grammatica, mai da un riconoscitore
  sul nome. Lo garantiscono gli import (G1, G9).
- **Il formato delle grammatiche**, la lettura stretta, la validazione, la forma canonica, l'hash, l'indice e
  l'elenco delle capacità riservate: `core/registro/regole/grammatica`. Qui si chiede a quel pacchetto se un
  selettore, una decorazione o una proiezione sono attivi (`SelettoreAttivo`, `DecorazioneAttiva`,
  `ProiezioneAttiva`): l'elenco resta uno solo.
- **Leggere i file** (indice, grammatiche) e il disco in generale: chi compone il processo (il banco, da A1d
  l'avvio dell'anteprima). Qui arrivano i byte.
- **Interpretare un documento** (`Interpreta`, il router, la funzione, i ruoli candidati, l'intersezione con
  i ruoli della famiglia): A1b (R23). Una lettura di forma non ha funzione né ruoli.
- **Le proposte di prodotti e ancoraggi**, la valutazione, il confronto con l'atteso: A1c.
- **Il riferimento al caso degli attesi** (`rif_caso`) che un esempio può portare: è un metadato opaco, lo
  legge solo il banco; qui non si legge mai (R47 b).
- **I dati dei clienti**: grammatiche, indice e limiti stanno nel dataset privato, mai nel repository.

## File

- **`motore.go`** — responsabilità:
  - commento `// Package`; `VersioneAlgoritmo` (`motorea-1`);
  - `Motore`, `CompilaVerificato`, `Motore.Snapshot`, `Motore.Riconosci`;
  - l'abbassamento della grammatica nel motore: quali forme entrano (attive, con almeno un selettore attivo,
    non proiezioni riservate, senza decorazioni o revisioni riservate, con al più un'etichetta, un
    marcatore, un token e una revisione) e su quali selettori.
- **`piani.go`** — responsabilità:
  - `pianoForma`: due regex compilate una volta per coppia (famiglia, forma), la sequenza con il confine
    destro e la stessa ancorata alla fine, che serve solo ad allungare una lettura (più una per ogni regola
    di revisione con token sospesi); mai tutte in alternanza, mai compilazioni per unità. I gruppi li genera
    il compilatore («g0», «g1»…) e ognuno porta l'operazione da fare sul testo catturato;
  - le parti (etichetta, affisso, base, ripetizione della base, revisione, decorazione con la sequenza
    interna di un suffisso, separatore, marcatore, token), le alternative letterali con `QuoteMeta`, le
    maiuscole indifferenti ASCII senza `(?i)`;
  - le revisioni in campo separato (D-07); le rune con cui una lettura può cominciare.
- **`scansione.go`** — responsabilità:
  - `scandisci`: ogni inizio di runa con il confine sinistro rispettato, match ancorato su una finestra
    lunga quanto la lettura più lunga più il confine, ripartenza di una runa; l'allungamento della lettura quando il confine destro ha consumato un carattere che poteva essere
    suo.
- **`confini.go`** — responsabilità:
  - le quattro classi di confine (D-05), con il caso che giustifica ognuna: la regex del confine destro e i
    due controlli in Go.
- **`letture.go`** — responsabilità:
  - `LetturaForma`, `BaseLetta`, `SegmentoLetto`, `ParteLetta`, `AffissoLetto`, `RevisioneLetta`,
    `DecorazioneLetta`, `RipetizioneLetta` e le costanti degli stati;
  - la costruzione di una lettura dai gruppi: codice richiesto, attribuzione degli affissi, ripetizioni
    concordanti o discordanti, revisione letta o token sospeso.
- **`confronta.go`** — responsabilità:
  - `Compatibilita` e le sue costanti, `ConfrontaBasi`, `ConfrontaRevisioni`.
- **`esempi.go`** — responsabilità:
  - la verifica di tutti gli esempi sul motore intero del cliente: mancante, in eccesso, diverso, selettore
    inattivo, collisione di due forme della stessa famiglia, non verificato.
- **`insieme.go`** — responsabilità:
  - `InsiemeRegole`, `CompilaInsieme`, `InsiemeRegole.MotoreDi`.
- **`limiti.go`** — responsabilità:
  - l'applicazione dei limiti di riconoscimento ricevuti dall'indice (`grammatica.Limiti`): byte dell'unità,
    letture per unità, risultato parziale, `limite.superato`. Nessun valore proprio.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che il pacchetto produce (R41 b): `grammatica.esempio_*`, `forma.collisione_esempi`,
    `regole.assenti`, `regole.non_valide`, `regole.file_assente`, `regole.sha256_discorde`,
    `regole.cliente_discorde`, `revisione.da_verificare`.

## Entry point

- **`CompilaInsieme`, `InsiemeRegole.MotoreDi`** — chi li chiama: da A1a il banco (`app/bancoa`), da A1d
  l'avvio dell'anteprima.
- **`CompilaVerificato`** — chi lo chiama: `CompilaInsieme`; il banco, per il rapporto delle regole.
- **`Motore.Riconosci`** — chi lo chiama: la verifica degli esempi; il banco (modalità `casi`); da A1b
  `Interpreta`. È la sola funzione di riconoscimento: non c'è un secondo motore.
- **`ConfrontaBasi`, `ConfrontaRevisioni`** — chi li chiama: il banco; da A1c l'ancoraggio.
- Oggi, nel codice di prodotto, il banco (`app/bancoa`, da A1a.5); l'anteprima arriva in A1d.

## Invarianti

- **I limiti vengono dall'indice** (R43 B): `CompilaVerificato` li riceve, `CompilaInsieme` li prende
  dall'indice stesso. Il pacchetto non ha valori suoi e nessun `LimitiPredefiniti`. Superare un limite di
  riconoscimento dà `limite.superato` (avviso) e un risultato parziale, mai un successo vuoto. Nessun limite
  di tempo: romperebbe il determinismo.
- **Budget di complessità** (par.12): nessun tipo, costante, ramo o tabella per un cliente. Il runtime è
  privato e minimo: piani indicizzati per selettore, regex RE2 compilate, gruppi catturati con poche
  operazioni (conservare, normalizzare, attribuire, confrontare la ripetizione con la base). Fra i tipi della
  grammatica e quelli del motore non c'è una corrispondenza 1:1. Un cliente nuovo che usa capacità già
  previste entra con il suo file v1 e la sua voce nell'indice, senza codice Go. Una capacità riservata non
  ha piani né codice runtime.
- **Un elemento è attivo solo se la grammatica lo dichiara**: nessuna semantica inventata. Un affisso si
  attribuisce solo sui selettori della sua attribuzione; un token si conserva e non si attribuisce mai; un
  livello o uno stato PDM sono decorazioni, mai revisioni; una forma parziale non ha segmenti per i
  mancanti.
- **Confini**: il sinistro si controlla in Go sul testo intero, il destro lo scrive il compilatore fuori
  dalla lettura; dopo un inizio si riparte dalla runa successiva, mai dalla fine del match (T2, T4, T5, T13).
  A parità d'inizio vince la lettura più lunga che rispetta il confine (`Longest`, T6).
- **I pattern dei clienti non hanno gruppi**: li genera il compilatore, con nomi unici (T12); sempre
  `regexp.Compile`, mai `MustCompile` (T18).
- **Offset in byte UTF-8**, 0-based, fine esclusa, su un confine di runa, sul testo dato: `Originale` è
  sempre `testo[Inizio:Fine]`.
- **Le collisioni osservate sugli esempi sono errori**: nessuna precedenza per ordine di array. Gli esempi
  delle forme che non entrano nel motore restano non verificati, mai passati.
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; nessun ordine dipende da
  una mappa. Letture in ordine di (inizio, fine, famiglia, forma); la grammatica si rende canonica anche qui,
  quindi famiglie e forme permutate danno le stesse letture e le stesse diagnostiche.

## Dipendenze

- **Importa:** `core/registro/regole/grammatica`, `core/estrazione/evidenze`, `platform/jsoncanonico`
  (l'impronta dell'indice), `github.com/google/uuid`, la libreria standard. Mai il motore legacy
  (`core/inbox/classificazione`, `core/registro/regole`).
- **È importato da:** `internal/app/bancoa`, il banco (vedi Entry point).

## Test

- Nel ramo `-qa` (piano A, par.4.3, Q4), tutti L1 e sintetici, con le grammatiche ACME dai nomi neutri
  (`acme-prefisso`, `acme-marcatore`, `acme-documento`, `acme-punti`, `acme-etichetta`,
  `acme-campo-separato`):
  - confini e offset (A1a-CNF); prefisso fuori dal selettore e nessuna rimozione globale (A-C06); forma
    parziale e completamenti (A-C07); etichetta obbligatoria (A-C08);
  - revisioni (A1a-REV), decorazioni (A1a-DCR), marcatori e categorie (A1a-MRK), esempi (A1a-ESE),
    determinismo (A1a-DET), limiti (A1a-LIM), insieme delle regole (A1a-INS).
- Il controllo degli import (G1, con il divieto del legacy), la guardia sul riferimento al caso degli
  attesi e i codici (A1a-CAT) stanno in `core/estrazione/evidenze`.
- I clienti dei test sono inventati (ACME); i test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/registro/regole/grammatica/README.md`
- `internal/core/estrazione/evidenze/README.md`
- `internal/core/inbox/classificazione/README.md` (il motore legacy, che qui non si usa)
- `internal/README.md`
