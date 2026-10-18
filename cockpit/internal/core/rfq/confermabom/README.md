---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/rfq/confermabom` — firma del perimetro e validità delle conferme (P4)

## Scopo

- Giro 5, A1d, P4, blocco B2.3, **parte pura** (piano esecutivo B2-B5 §1.1; decisione D-T-09 a: un package
  nuovo; contratto backend A1d v2 rev. 3 §5.1).
- Una sola definizione, per la conferma formato 2 (R5, via `distintabozza`) e per la conferma formato 1
  (`core/rfq/fascicolo`), di:
  - **perimetro** di un prodotto (`CalcolaPerimetro`): il prodotto e i componenti raggiungibili per gli archi
    verso figli non archiviati, come il controllo al COMMIT della 0022 (`conferma_strutturale_completa`, (c));
  - **firma del perimetro** (`FirmaPerimetro`, `Perimetro.Firma`, `FormaCanonica`): sha256 della forma canonica
    (`jsoncanonico`, versione `firma-perimetro-2`) di id, **codice letterale**, tipo, archiviato,
    `step_strutturale_id` dei componenti e padre, figlio, qta degli archi. **Non** entrano rev, descrizione,
    posizione e il **codice canonico**. L'ordine degli ingressi non conta;
  - **regole del cliente fuori dalla firma** (D-FIRMA-01 = b [U], direttiva 12 §5): la firma non riceve il
    `classificazione.Motore` e il package non lo importa. Un cambio delle regole di canonizzazione (per esempio
    un suffisso decorativo in più o in meno) lascia la firma com'è, e una conferma strutturale immutata resta
    valida anche quando il contatore si muove per un altro prodotto della RFQ. `firma-perimetro-1` (con il
    canonico calcolato dal Motore) non è mai stata registrata: nessuna firma da migrare. Collisioni di identità e
    incoerenze di nomenclatura si controllano altrove (`ux_componente_thread_codice`, il riepilogo della conferma
    dell'albero in `core/rfq/fascicolo`), non con questa firma;
  - **validità dell'ultima conferma** (`Ultima`, `ConfermaValida`, `UltimaValida`): ultima e (contatore uguale
    oppure firma uguale);
  - **controllo della base** (`ControllaBase`): i motivi `conferma_cambiata` e `struttura_cambiata` di
    `BASE_SUPERATA`.

## Lo stato della RFQ (V1, ondata B)

- **`base.go`**: `StatoRFQ` (`LeggiStatoRFQ`: componenti, archi, conferme e fonti autorizzate di tutta la RFQ, con le
  query di `db/queries/v1_bozza_rotte.sql`; `NuovoStatoRFQ` in memoria), `VersioneCorrente` (ultima conferma,
  contatore, firma del perimetro ricalcolata, fonte autorizzata: la `versione_corrente` di R1), `ValiditaUltima`,
  `ControllaBase`. Recuperato dalla tappa del 13/10 (V0 §2.3: «riusabile, da adattare»); e' l'unico file del package che
  legge il database, nella transazione di chi chiama (R1 in sola lettura, R2 dopo `BloccaThread`). Le regole restano
  quelle pure di `firma.go` e `validita.go`.

## Non appartiene qui

- La scrittura di `conferma_strutturale` e dei suoi componenti e archi (B2.7), i segni sulle righe STEP (B2.9).
- I motivi della base che dipendono dalla fonte STEP (`fonte_rifiutata`, `rif_scaduti`): li aggiunge chi chiama.

## Prove

- L1: `firma_test.go`, `validita_test.go` (forma canonica scritta per intero, ordine indifferente anche per un
  perimetro scritto a mano, rev, descrizione e posizione esclusi, qta, tipo, archiviazione,
  `step_strutturale_id`, codice letterale com'è, Δ44, ingressi impossibili, validità per contatore o per firma,
  base; sotto lucchetto la firma della working è obbligatoria).
- Scenario F della direttiva 12: `TestScenarioFLaFirmaPortaIlCodiceLetterale`,
  `TestScenarioFLeRegoleDelClienteNonCambianoLaFirma` (nessun import delle regole del cliente fuori dalle prove),
  `TestScenarioFValiditaDopoUnCambioDiRegoleODiStruttura` (regole cambiate → conferma valida per firma; cambio
  vero di un componente o di un arco → non valida, `struttura_cambiata`).
- I nomi dei file della parte pura sono `firma.go` e `validita.go`. `base_test.go` (L1, V1 ondata B): lo stato della
  RFQ in memoria usa la firma e la validita' del package; la lettura dal database la provano le L4 delle rotte (R1).
