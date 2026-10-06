---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/fotorfq` — i record della fotografia di una RFQ

## Scopo

- Contiene i **dati di una o più RFQ come li ha letti** il caricatore, in una transazione, o il lettore degli
  export: tipi senza database, che si riempiono da fuori e si leggono soltanto (piano A, par.3.3.6; nome e
  sede -> R40 d).
- In A1b ci sono solo i **record che leggono gli adattatori** di `core/estrazione`:
  - `Messaggio`: le colonne del messaggio; `CorpoTesto` è il `corpo_testo` come sta nel DB, e gli offset degli
    adattatori si contano su quei byte;
  - `Allegato`: le colonne dell'allegato, con il nome già troncato e ripulito dall'acquisizione;
  - `Terna`: versione dell'analizzatore e hash della configurazione, la chiave di `analisi_fatti`;
  - `Fatti`: i fatti di un contenuto alla terna data, con il payload come l'ha dato il DB o l'export.
- Dà **un solo modo di calcolare l'impronta dei fatti**, `ImprontaPayload`: lo sha256 del JSON canonico del
  payload, con i numeri copiati come testo (5.4.2, par.3.4.2). È il valore di `Fatti.Digest`, per chiunque
  riempia un `Fatti`.
- Da A1c (piano A, 6.4.1; **contratto di A1c, §3**, che prevale sul 6.4.1) c'è **la fotografia intera**:
  - `Fotografia`, il contenitore (origine, schema, terna corrente, sezioni, clienti, thread, messaggi fuori RFQ,
    utenti, diagnostiche) e `Thread`, una RFQ con i messaggi, gli allegati, i fatti e le decisioni attuali;
  - i record delle decisioni, campo per campo come il contratto: il gesto 1 (`AggancioMessaggio`, fuori dal
    record del messaggio: I-2), il gesto 2 (`Identificativo`), i componenti con il gesto 3
    (`Componente.StepStrutturaleID`), le relazioni, le righe legacy `RigaComponenteProposta` e
    `RigaRelazioneProposta` (M2) con `MarcaturaStrutturale` e `SegnoAlbero`, le rimozioni aperte, i documenti
    confermati, le proposte, le righe di `v_step_prodotto` e di `v_fascicolo`, le deroghe, i fabbisogni
    effettivi, la versione della BOM, il triage deterministico e i candidati di codice senza l'agente;
  - `Ordina` (un ordine totale per chiave stabile), `ImprontaFotografia` (senza `PresaIl` e `Sorgente`),
    `ValidaFotografia` (i riferimenti, i tempi, l'agente escluso) e i codici `fotografia.*`.

## Non appartiene qui

- **Il caricatore** (`core/fotorfq/caricatore`), l'unico pacchetto del motore A con il DB, in sola lettura: è un
  pacchetto a parte (A1c). Qui i tipi si riempiono da fuori.
- **Qualunque calcolo sulle decisioni**: target, fonte, ancoraggi e valutazione stanno in `core/ancoraggio` e
  `core/valutazione`. La fotografia dice che cosa c'è nel DB, non che cosa significa.
- **L'interpretazione dei record**: i documenti delle evidenze li costruiscono gli adattatori di
  `core/estrazione`; qui nessun record porta una decisione o un'interpretazione (I-2, par.3.8.3).
- **Il codificatore canonico**: sta in `platform/jsoncanonico` (-> R40 c); qui si usa e non si rifà.
- **La lettura dei fatti del worker** (`worker.DecodificaStruttura`, `DecodificaTestoPDF`): la fanno gli
  adattatori. Qui il payload resta un `json.RawMessage` com'è arrivato.

## File

- **`fotorfq.go`** — responsabilità:
  - commento `// Package`; `VersioneSchema` (1);
  - `Messaggio`, `Allegato`, `Terna`, `Fatti`, con i tag JSON delle colonne;
  - `ImprontaPayload`: payload vuoto o senza una forma unica (chiavi ripetute, UTF-8 non valido, surrogati
    soli, testo dopo il valore) → errore, mai un'impronta inventata.
- **`fotografia.go`** (A1c) — responsabilità: i tipi del contratto §3.1–§3.3, con i tag JSON snake_case; le
  origini (`dsn`, `exports`), gli stati delle sezioni, le chiavi di `Sezioni` (`ChiaviSezioni`).
- **`ordina.go`** (A1c) — responsabilità: `Ordina`, l'ordine totale degli elenchi con il confronto dei byte
  (messaggi per data e ID, allegati come `ListAllegatiFascicolo`, il resto per chiave primaria).
- **`impronta.go`** (A1c) — responsabilità: `ImprontaFotografia`, lo sha256 del JSON canonico di una copia
  ordinata, senza `PresaIl` e `Sorgente`.
- **`valida.go`** (A1c) — responsabilità: `ValidaFotografia`, i controlli del 6.4.1 e del contratto §3.5.
- **`codici_diagnostica.go`** (A1c) — responsabilità: i codici `fotografia.*` del 6.4.10.

## Entry point

- **`Messaggio`, `Allegato`, `Terna`, `Fatti`** — chi li usa: da A1b gli adattatori di `core/estrazione`
  (`DaAllegato`, `DaMessaggio`) e le loro prove; da A1c il caricatore e il lettore degli export, che li
  riempiono.
- **`ImprontaPayload`** — chi lo chiama: chi riempie un `Fatti` (le prove degli adattatori in A1b; il
  caricatore e il lettore degli export in A1c, che in bozza la chiamavano `DigestFatti`: il nome è uno solo).
- **`Fotografia`, `Thread` e i record** — chi li riempie: il caricatore (A1c) e il lettore degli export del
  banco; chi li legge: `core/valutazione` (A1c), il banco, l'anteprima (A1d).
- **`Ordina`, `ImprontaFotografia`** — chi li chiama: il caricatore, il lettore degli export, le prove del
  determinismo. **`ValidaFotografia`** — chi la chiama: `valutazione.Calcola`, prima di calcolare.

## Invarianti

- **Niente DB, file né orologio**: `time` serve solo per i tipi (`DataEvento`, `RicevutoIl`, `CalcolatoIl`),
  `uuid` solo per gli ID che arrivano da fuori. Lo controllano G1 e G2.
- **Un puntatore nil vuol dire «assente nel DB»**, mai «vuoto»: `Oggetto`, `CorpoTesto`, `CorpoHTML`,
  `PathInterno`, `Sha256`, … `Fatti.MotivoParziale` nil vuol dire «completezza non determinabile» (-> R32 b),
  "" vuol dire completa.
- **L'impronta dei fatti non dipende dalla forma del JSON**: ordine delle chiavi, spazi, escape delle stringhe
  («\u00e8» ed «è») non contano; i numeri restano i letterali scritti («800» e «800.0» sono due impronte).
- `VersioneSchema` si cambia solo con un commit che lo dichiara; la prova che la fissa si riscrive con
  «Riscritta per …». A1c non la cambia (T-01): la fotografia si aggiunge, i record di A1b restano com'erano.
- **Nessuna decisione dentro `Messaggio` o `Allegato`** (I-2): il gesto 1 sta in `AggancioMessaggio`.
- **Tempi UTC al millisecondo**, puntatore nil = assente nel DB, tag JSON snake_case: in tutti i record di A1c.
- **Due letture della stessa copia danno la stessa impronta**: `ImprontaFotografia` ordina e lascia fuori
  `PresaIl` e `Sorgente`; l'ordine non dipende da una mappa né dalla collazione del database.

## Dipendenze

- **Importa:** `platform/jsoncanonico` (le impronte), `core/estrazione/evidenze` (da A1c: le diagnostiche della
  fotografia), `github.com/google/uuid`, la libreria standard.
- **È importato da:** `core/estrazione` (A1b); da A1c il caricatore (`core/fotorfq/caricatore`) e
  `core/valutazione`.

## Test

- **`fotorfq_test.go`** — livello L1 — che cosa copre (A1b-02):
  - chiavi permutate, spazi e a capo, «\u00e8» contro «è» → la stessa impronta, uguale allo sha256 del
    canonico scritto a mano;
  - «800» contro «800.0» → impronte diverse;
  - payload vuoto, non JSON, con una chiave ripetuta, UTF-8 non valido, surrogato solo, testo dopo il valore →
    errore;
  - la `VersioneSchema` fissa.
- **`fotografia_test.go`** — livello L1 — che cosa copre (A1c-L1-03):
  - `Ordina` totale e `ImprontaFotografia` uguale dopo le permutazioni, senza `PresaIl` e `Sorgente`, e diversa
    per un dato che cambia; la fotografia di chi chiama non cambia;
  - `ValidaFotografia`: una fotografia giusta senza diagnostiche; ogni riferimento che non si risolve, un
    candidato dell'agente, un tempo non UTC al millisecondo, fatti a un'altra terna;
  - `ImprontaPayload` uguale per i byte del jsonb e per la stringa dell'export con lo stesso contenuto;
  - i tag JSON dei record di A1c, snake_case.
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go` (A1b-03, parte `fotorfq`; A1c-L1-28).
- I fatti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/platform/jsoncanonico/README.md`
- `internal/README.md`
