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

## Non appartiene qui

- **Il caricatore** (`core/fotorfq/caricatore`), l'unico pacchetto del motore A con il DB, in sola lettura, e
  **la fotografia intera** (`Fotografia`, `Thread`, le decisioni attuali, `Ordina`, `ImprontaFotografia`):
  arrivano in A1c (par.6).
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

## Entry point

- **`Messaggio`, `Allegato`, `Terna`, `Fatti`** — chi li usa: da A1b gli adattatori di `core/estrazione`
  (`DaAllegato`, `DaMessaggio`) e le loro prove; da A1c il caricatore e il lettore degli export, che li
  riempiono.
- **`ImprontaPayload`** — chi lo chiama: chi riempie un `Fatti` (le prove degli adattatori in A1b; il
  caricatore e il lettore degli export in A1c, che in bozza la chiamavano `DigestFatti`: il nome è uno solo).
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **Niente DB, file né orologio**: `time` serve solo per i tipi (`DataEvento`, `RicevutoIl`, `CalcolatoIl`),
  `uuid` solo per gli ID che arrivano da fuori. Lo controllano G1 e G2.
- **Un puntatore nil vuol dire «assente nel DB»**, mai «vuoto»: `Oggetto`, `CorpoTesto`, `CorpoHTML`,
  `PathInterno`, `Sha256`, … `Fatti.MotivoParziale` nil vuol dire «completezza non determinabile» (-> R32 b),
  "" vuol dire completa.
- **L'impronta dei fatti non dipende dalla forma del JSON**: ordine delle chiavi, spazi, escape delle stringhe
  («\u00e8» ed «è») non contano; i numeri restano i letterali scritti («800» e «800.0» sono due impronte).
- `VersioneSchema` si cambia solo con un commit che lo dichiara; la prova che la fissa si riscrive con
  «Riscritta per …».

## Dipendenze

- **Importa:** `platform/jsoncanonico` (l'impronta del payload), `github.com/google/uuid`, la libreria
  standard. La tabella di `internal/README.md` ammette anche `core/estrazione/evidenze`, per le diagnostiche
  della fotografia di A1c; in A1b non serve.
- **È importato da:** ancora nessuno nel prodotto; da A1b `core/estrazione`, da A1c il caricatore.

## Test

- **`fotorfq_test.go`** — livello L1 — che cosa copre (A1b-02):
  - chiavi permutate, spazi e a capo, «\u00e8» contro «è» → la stessa impronta, uguale allo sha256 del
    canonico scritto a mano;
  - «800» contro «800.0» → impronte diverse;
  - payload vuoto, non JSON, con una chiave ripetuta, UTF-8 non valido, surrogato solo, testo dopo il valore →
    errore;
  - la `VersioneSchema` fissa.
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go` (A1b-03, parte `fotorfq`).
- I fatti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/platform/jsoncanonico/README.md`
- `internal/README.md`
