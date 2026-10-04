---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/platform/jsoncanonico` — il JSON con un solo modo di essere scritto

## Scopo

- Scrive un valore Go come **JSON canonico**: lo stesso valore dà sempre gli stessi byte, quindi la stessa
  impronta.
  - chiavi degli oggetti in ordine di byte UTF-8, nessuno spazio;
  - stringhe in UTF-8 grezzo, con l'escape solo di «"», «\» e dei controlli U+0000–U+001F; U+2028 e U+2029
    restano grezzi;
  - numeri solo interi, oppure letterali JSON già scritti (`json.Number`, `json.RawMessage`), copiati come
    testo.
- Calcola le **impronte**: sha256 in esadecimale minuscolo, 64 caratteri, come le colonne `char(64)`.
- È la base di ogni impronta del motore A del giro 5: hash delle grammatiche, impronta dell'indice delle
  regole, `BundleID` dei documenti delle evidenze, identità delle interpretazioni.

## Non appartiene qui

- **Che cosa entra in un'impronta** (quali campi, quali ordinamenti): lo decide chi la calcola.
  - La politica canonica delle grammatiche e il loro hash stanno con le grammatiche;
  - il `BundleID` lo calcola chi costruisce il documento delle evidenze.
- **Ordinare gli elenchi senza significato:** tocca a chi chiama, prima di codificare.
- **Leggere** un JSON in un tipo Go: le decodifiche strette degli ingressi stanno nei pacchetti che li
  possiedono.
- Qualunque nozione di RFQ, cliente o motore: è un'utilità senza dominio, per questo sta in `platform`.

## File

- **`jsoncanonico.go`** — responsabilità:
  - `Versione` (`canonico-1`);
  - `Codifica`: struct (tag json, `omitempty` e `omitzero` compresi), mappe con chiave stringa, slice,
    array, puntatori, stringhe, booleani, interi, `json.Number`, `json.RawMessage`; i tipi con
    `MarshalJSON` o `MarshalText` (`time.Time`, `uuid.UUID`) passano dal loro metodo e il risultato si
    ricanonicalizza;
  - la rilettura stretta di un JSON già scritto (`json.RawMessage`, `MarshalJSON`): chiavi ripetute,
    surrogati soli, UTF-8 non valido e testo dopo il valore sono errori;
  - `Impronta`, `ImprontaDi`.

## Entry point

- **`Codifica`, `ImprontaDi`, `Impronta`, `Versione`** — chi li chiama:
  - nel giro 5, A1a, i pacchetti del motore A che calcolano impronte (grammatiche, motore); in A1b i record
    della fotografia (`core/fotorfq`, l'impronta dei fatti) e gli adattatori, per il `BundleID`;
  - oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **Stesso valore, stessi byte**: l'ordine d'iterazione delle mappe di Go non conta, gli spazi e l'escape
  con cui un `json.RawMessage` è arrivato nemmeno.
- **Niente ripieghi silenziosi**: un float di Go, una stringa non UTF-8, una chiave non stringa, una chiave
  ripetuta in un `json.RawMessage`, un surrogato solo (`\ud800`) sono errori. Niente arrotondamenti, niente
  U+FFFD.
- **I numeri restano come sono scritti**: un `json.RawMessage` con `1.50` resta `1.50`.
- **L'ordine degli array è quello dato.** `nil` e la slice vuota restano distinti, come in `encoding/json`
  (`null` e `[]`); `omitempty` li toglie tutti e due.
- **`Versione` entra in ogni impronta**: cambiarla cambia tutti gli hash, e si fa solo con un commit che lo
  dichiara (la prova che la fissa va riscritta con il titolo «Riscritta per …»).
- Non è RFC 8785 completo, perché non scrive numeri decimali: i DTO del motore non ne hanno.
- Niente orologio, file, rete, goroutine: è un pacchetto puro del motore A.

## Dipendenze

- **Importa:** solo la libreria standard.
- **È importato da:** `core/registro/regole/grammatica`, per la forma canonica e l'hash degli snapshot;
  `core/inbox/classificazione/motorea`, per l'impronta dell'indice delle regole; `core/fotorfq`, per
  l'impronta dei fatti (`ImprontaPayload`); `core/estrazione`, per il `BundleID` dei documenti e l'impronta dei
  testi di un messaggio; `internal/app/bancoa`, per il rapporto del banco in JSON canonico.

## Test

- **`jsoncanonico_test.go`** — livello L1 — che cosa copre (A1a-CAN):
  - golden in linea, byte e sha256;
  - chiavi permutate, spazi, `\u00e8` contro «è» → stessi byte;
  - U+2028 e U+2029 grezzi; i controlli in escape;
  - float, UTF-8 non valido, surrogato solo, chiave ripetuta, numero non JSON → errore;
  - mappe ordinate; `RawMessage` ricanonicalizzato con i numeri come testo;
  - `uuid.UUID` dal suo `MarshalText`, `time.Time` dal suo `MarshalJSON`; `omitzero`;
  - riferimento circolare → errore;
  - la `Versione` fissa.
- I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/platform/README.md`
- `internal/core/estrazione/evidenze/README.md`
