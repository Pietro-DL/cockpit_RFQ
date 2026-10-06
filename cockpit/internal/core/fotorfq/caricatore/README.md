---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/fotorfq/caricatore` — la fotografia di una RFQ, letta dal database in sola lettura

## Scopo

- È **l'unico pacchetto del motore A che tocca il database** (giro 5, A1c; piano A, 3.3.6 e 6.4.2; contratto di
  A1c, §3). Legge in **una sola transazione `REPEATABLE READ READ ONLY`** tutti gli ingressi coerenti di una o
  più RFQ e restituisce una `fotorfq.Fotografia` in memoria, ordinata.
- **Nessuna interpretazione e nessuna proposta**: chiude la transazione con `ROLLBACK`, poi converte le righe nei
  tipi puri di `fotorfq`. Il calcolo lo fa dopo `valutazione.Calcola`, chiamata da chi chiama.
- Porta nella fotografia i tre gesti dell'operatore come il DB li ha salvati: il gesto 1
  (`AggancioMessaggio`, da `messaggio.aggancio`, `agganciato_da`, `agganciato_il`), il gesto 2
  (`identificativo_thread.confermato_da`), il gesto 3 (`componente.step_strutturale_id` e
  `evidenza.strutturale` delle righe di `componente_proposta`); in più il segno di «Conferma l'albero»
  (`evidenza.albero`), le viste `v_step_prodotto` e `v_fascicolo`, i fabbisogni effettivi del cliente, le
  deroghe, le rimozioni aperte, l'ultima versione della BOM e l'ultima congelata.

## Non appartiene qui

- **I tipi della fotografia** (`Fotografia`, `Thread`, i record): stanno in `core/fotorfq`, senza DB.
- **Qualunque calcolo**: target, fonte, ancoraggi, valutazione (`core/ancoraggio`, `core/valutazione`).
- **La scrittura**: nessuna, mai. Nemmeno una tabella di prova.
- **L'apertura del pool**: la fa chi chiama (`migrazioni.ApriInLettura` per il banco; il pool del server per
  l'anteprima). Qui serve solo un `Iniziatore`.
- **Il lettore degli export**: sta nel banco (`app/bancoa`), e riempie gli stessi tipi.

## File

- **`caricatore.go`** — responsabilità:
  - commento `// Package`; `Iniziatore`, `Richiesta`, `ErrNonInSolaLettura`;
  - `apriInSolaLettura`: `BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY`, poi `SHOW transaction_read_only`
    (deve essere `on`) e `SHOW transaction_isolation` (deve essere `repeatable read`); se no, `ROLLBACK` ed
    `ErrNonInSolaLettura`, prima di qualunque lettura di dominio;
  - `Carica`: la sequenza del 6.4.2 (schema con `migrazioni.Applicate` sulla transazione, `SELECT now()`,
    l'analizzatore corrente, il perimetro, le letture per RFQ, i clienti, i messaggi fuori RFQ, le sigle), poi
    il `ROLLBACK` esplicito e, a transazione chiusa, la conversione;
  - `leggiThread`, `leggiFuori`: le letture di una RFQ e di un messaggio senza RFQ.
- **`conversioni.go`** — responsabilità:
  - dalle righe sqlc ai record di `fotorfq`: NULL → puntatore nil; tempi UTC al millisecondo; enum → testo;
  - di `evidenza` solo `strutturale` e `albero`, in record tipizzati (T-03): `sospesa` vale «c'è la chiave»;
    un JSON che non ha la forma attesa ferma la fotografia, non si indovina;
  - `Fatti.Digest` con `fotorfq.ImprontaPayload`; `MotivoParziale` solo per i fatti con la struttura STEP;
  - le sezioni tutte `completa`, salvo i fatti senza un analizzatore corrente (`assente`, con la diagnosi
    `fotografia.nessun_analizzatore`).

## Entry point

- **`Carica(ctx, p, r)`** — chi lo chiama: il banco del motore A (`app/bancoa`, modo `-dsn`) e, in A1d,
  l'anteprima della Distinta; le prove L4 del pacchetto.
- **`ErrNonInSolaLettura`** — chi lo controlla: chi chiama `Carica`, per dire che la fotografia non c'è.

## Le query (contratto §3.4: elenco chiuso, 29)

- **Riusate, già nel 6.4.2** (17): `GetAnalizzatoreCorrente`, `ListRfqPerLaRiapertura` (con l'argomento NULL,
  sulla transazione), `GetThread`, `ListMessaggiThread`, `ListAllegatiThread`, `ListProposteDocumentoThread`,
  `ListDocumentiThread`, `ListProvenienzeThread`, `ListIdentificativi`, `ListComponentiThread`,
  `ListRelazioniDellaRfq`, `ListComponenteProposteThread`, `ListRelazioneProposteThread`, `GetUltimaVersione`,
  `ListLavoroPendenteRfq`, `GetMessaggio`, `ListAllegatiMessaggio`.
- **Riusate, nuove nell'elenco** (6): `ListStepProdotto`, `ListFascicolo`, `ListDerogheThread`,
  `ListRimozioniAperte`, `ListFabbisognoEffettivo` (con il cliente del thread), `GetUltimaCongelata`.
- **Nuove** (6, `queries/fotografia.sql`): `ListFattiDelThread` (i contenuti degli allegati e dei documenti del
  thread, T-04), `ListFattiPerSha`, `ListClientiDellaFotografia`, `ListSigleUtenti`,
  `ListCandidatiCodiceThread`, `ListTriageThread`.
- **SQL scritto a mano**: solo `SHOW transaction_read_only`, `SHOW transaction_isolation`, `SELECT now()`.
- **Mai**: le `Blocca*`, `ListDocumentiComponente`, `WorkingBloccataNelDatabase`, le query dell'agente,
  `ListUtenti`/`GetUtente`/`GetUtentePerSigla` (`SELECT *` su `utente`), `GetCliente` (`SELECT *` con `regole`),
  `CensimentoCartigli`, i lettori del Fascicolo legacy, i job di analisi falliti (LD-08), il registro degli
  agganci (T-06), ogni scrittura.

## Invarianti

- **Una transazione, una connessione**: `db.New` riceve solo la transazione, il pool non esegue nessuna query;
  l'ultimo testo SQL è il `ROLLBACK`.
- **Sola lettura dichiarata dal database**, non solo chiesta: senza `on` e `repeatable read` non si legge niente.
- **I fatti alla terna esatta** di `analizzatore_corrente`, letta nella stessa transazione: mai «la riga più
  recente». Senza terna, nessun fatto.
- **Determinismo**: la stessa scena dà la stessa `fotorfq.ImprontaFotografia` (fuori `PresaIl` e `Sorgente`);
  niente orologio di Go (l'ora è `SELECT now()` del DB), niente goroutine, niente file.
- **Un thread o un messaggio chiesti che non ci sono sono un errore**, mai saltati.
- **I record di A1b non cambiano** (I-2): il gesto 1 sta in `AggancioMessaggio`, non nel `Messaggio`.

## Dipendenze

- **Importa:** `core/fotorfq`, `core/estrazione/evidenze` (le diagnostiche), `platform/db`,
  `platform/migrazioni` (`Applicate`, `UltimaApplicata`), `pgx/v5`, `google/uuid`, la libreria standard.
- **Mai**: un altro `core/*`, `ai`, `app`, `transport`, `platform/config`, `coda`, `storage`, la libreria YAML
  (G1, G5).
- **È importato da:** `app/bancoa` (A1c) e, in A1d, `transport/web`.

## Test

- **`guardia_test.go`** — livello L1 — (A1c-L1-04): dai sorgenti, con `go/parser`, i metodi di `db.Queries`
  chiamati sono esattamente le 29 query del contratto; il loro SQL generato è solo lettura, senza lucchetti né
  tabelle escluse; `db.New` riceve solo la transazione; SQL scritto a mano solo per i due `SHOW` e `SELECT now()`;
  le conversioni da righe sqlc sintetiche ai tipi puri, senza perdite.
- **`caricatore_db_test.go`**, **`query_db_test.go`**, con la scena di **`scena_db_test.go`** — livello L4, tag
  `integrazione` — (A1c-L4S-02…07, -10): la
  transazione e il registro SQL, il rifiuto di un iniziatore non in sola lettura, la scrittura rifiutata (25006),
  la terna esatta, la completezza delle strutture, due fotografie della stessa scena con la stessa impronta e una
  dopo un'analisi nuova con un'impronta diversa; le letture nuove (gesti, viste, deroghe, rimozioni, fabbisogni,
  versione della BOM), l'agente escluso, nessuna `password_hash`, nessuna `regole`.
- Le scene sono inventate (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/fotorfq/README.md`
- `internal/platform/db/README.md` (`fotografia.sql`)
- `internal/platform/migrazioni/README.md`
- `internal/core/README.md`
