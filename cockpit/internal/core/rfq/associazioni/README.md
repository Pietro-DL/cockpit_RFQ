---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/rfq/associazioni` — il servizio unico delle associazioni registrate (AR)

## Scopo

- V1, direttiva 35 (§2, §3 [U]), blocco C-1c del piano delle dipendenze (§3.6 [T]); tabelle della 0023, sezione 9d
  (variante `0023-v1-formato3-file-20261019d`, direttiva 39: la `20261019c` con la copia derivata accesa).
- **L'unico scrittore** di:
  - `file_rfq_associazione` (le AR: un'occorrenza → un componente della working, applicata da un atto autorizzato);
  - il suo giornale in `associazione_evento` (nascita `preassociazione` di R5 o `assegna` di un gesto legacy,
    sostituzione `sostituisci`, chiusura `sgancia`, sgancio di una copia senza AR, legame della sorgente `assegna` senza
    riga);
  - la **copia derivata di compatibilita'** `documento_proposta.componente_id` (con il codice, per la FK a tre campi),
    sempre nella stessa transazione e mai fonte.
- Le operazioni:
  - **`Registra`** (la PB applicata da R5: riga, evento, copia), **`Riassocia`** (riga nuova dopo una chiusura),
    **`Sostituisci`** (chiusura «sostituita» e nascita in un gesto, sempre esplicita). La `Registrazione` porta la
    **provenienza della PB** (`motore`, `corretta`, `manuale`, `legacy`) e, con `motore` o `corretta`, il registro
    d'importazione d'origine (`Importazione`), copiati dallo snapshot confermato: il database li verifica al COMMIT
    (D-PB03 A [U]; rifiuto leggibile `provenienza_non_valida`);
  - **`RegistraLegacy`**, **`SostituisciLegacy`** (direttiva 39, D-AR03 D [U]): un gesto atomico di una RFQ
    interamente legacy, origine `gesto_legacy`, senza conferma, nodo e provenienza; evento `assegna` (o `sostituisci`)
    con l'origine `legacy`; in una RFQ formato 3 il rifiuto `rfq_formato3`;
  - **`Scollega`** con il motivo: `svincolo_conferma` (R5), `scollega_file` (pagina nuova, corsia file),
    `file_generale`, `scarto_legacy` e `sgancio_legacy` («Togli» di una RFQ interamente legacy; `rfq_formato3` in una
    RFQ formato 3) delle pagine legacy; senza AR toglie la copia di una pre-assegnazione legacy (`copia_tolta`); uno
    svincolo di R5 gia' chiuso dalla corsia file e' `svincolo_gia_chiuso`;
  - **`ConvertiCopiaLegacy`** (direttiva 39, D-AR14 A [T-39]): alla presa in carico del prodotto una pre-assegnazione
    legacy aperta (P1, la copia senza AR) si toglie con l'evento `sgancia` di origine `conversione_legacy`, con
    `componente_prima`, senza AR ne' conferma; l'autore e' chi prende in carico; la PB `legacy` la scrive la bozza
    (`distintabozza/conversionep1.go`); con una AR attiva `associazione_diversa`;
  - le letture **`Attiva`**, **`AttiveDellaRfq`**, **`StoriaDellAllegato`**;
  - **`AllineaCodiceCopia`** dopo una ricodifica (D-ASS-01);
  - **`LegaSorgente`** (D-AR02 A [U]): scritta e **non cablata** (S8 lega lo STEP sorgente con `fonte_step_evento` e il
    documento, senza passo 1).
- **Idempotenza per chiave** (direttiva 35 §2.4): lo stesso atto non fa nascere ne' chiude due volte (`Ripetuto`),
  anche dopo una chiusura; confermare di nuovo senza cambiamenti non tocca la riga (`Invariata`).
- **La copia** conserva la lettura del codice in `dettagli.codice_letto` prima di scrivere il codice del componente
  (DIR35-DP, INC-G4) e la rimette quando si toglie.

## Non appartiene qui

- **I lucchetti:** il servizio non ne prende. Chi chiama blocca la RFQ (`BloccaThread`, o `BloccaThreadCondiviso`
  FOR KEY SHARE) prima di ogni proposta: ordine RFQ → proposta → AR (piano §3.6, INC-G25).
- **R4/R5 e `Piano`:** gli esiti per R4 (`applicabile`, `gia_applicata`, `sostituzione`, `gia_associata_altrove`,
  `legato_come_sorgente`, `documento_confermato_altrove`, `non_richiesta`, `svincolo_gia_chiuso`) sono del blocco C-5.
- **Le rotte:** nessuna. R5 (C-5), la corsia file (C-6b) e le pagine legacy chiamano il servizio dal loro gesto.
- **Gli scrittori legacy della copia** (`assegnaProposta`, «Usa come STEP», «Documentazione generale», vecchio «Scarta
  il file», correzione del codice): passano di qui in C-6b (corsia file), con `RegistraLegacy`/`SostituisciLegacy`/
  `Scollega(sgancio_legacy)` nelle RFQ interamente legacy. Dopo C-6b nessuno scrive la copia fuori da qui: dalla
  variante `20261019d` della 0023 `trg_proposta_copia_derivata` e' acceso (un cambio della copia senza un evento dello
  stesso allegato nella stessa transazione e' rifiutato al COMMIT, P4X10). `SetComponenteProposta` e
  `SetCodiceProposteComponente` (query del Fascicolo) sono senza chiamanti nel prodotto. Le prove preparano i dati legacy
  di prima del trigger con `testutil.CopiaLegacy`.
- **Il backfill:** nessuno (D-AR04 [U]): `RegistraBackfill` non c'e'. La conversione di una pre-assegnazione aperta
  alla presa in carico (D-AR14 A [T-39], evento `conversione_legacy`, l'unico con l'autore nullo) ha il supporto nella
  0023 e la funzione `ConvertiCopiaLegacy`, chiamata da `distintabozza` alla nascita della bozza formato 3.
- **Le verifiche del database:** le sei verifiche della direttiva §2.1 e gli invarianti al COMMIT sono nella 0023
  (`trg_fra_nasce`, `trg_fra_invarianti`, `trg_conferma_associazioni`, ...). Il servizio fa prima gli stessi controlli
  che servono a un rifiuto leggibile (`gia_associata_altrove`, `documento_confermato_altrove`, componente archiviato o
  di un'altra RFQ) e traduce gli altri rifiuti del database in `Rifiuto` (`vincolo_associazione`, `bom_congelata`).

## Le decisioni (direttiva 39)

- **D-AR01 A [U]:** una AR di R5 nasce solo dalla conferma formato 3 della stessa transazione: `Atto.Conferma`
  obbligatoria per `Registra`, `Riassocia`, `Sostituisci` (`conferma_obbligatoria`).
- **D-AR03 D [U]:** i gesti atomici nelle RFQ interamente legacy passano dal servizio (`gesto_legacy`), nelle RFQ
  formato 3 no (anche il database li rifiuta, P4X10).
- **D-ASS-04 A [T-39]:** una sola AR attiva per allegato (`Attiva` legge al piu' una riga; la copia ha un solo valore).
- **D-AR07 A [T-39]:** il documento confermato dello stesso contenuto su un altro componente ferma la nascita.
- **D-2 (c) [U]:** uno STEP documentale diventa AR come un PDF; lo STEP legato come sorgente del finito no (punto 1 di
  `trg_fra_nasce`, `allegato_sorgente_del_finito`).
- **D-AR02 A [U]:** il legame della sorgente non e' una AR (`LegaSorgente`, evento `assegna` con origine `ponte`;
  l'origine `sorgente_step` non c'e').

## File

- **`associazioni.go`** — tipi (`Atto`, `Associazione`, `Registrazione`, `Scollegamento`, `Esito`, `EsitoChiusura`,
  `Rifiuto`), letture, `Registra`, `Riassocia`, `Sostituisci`, `RegistraLegacy`, `SostituisciLegacy`, `Scollega`,
  `ConvertiCopiaLegacy`, `AllineaCodiceCopia`, la traduzione dei rifiuti del database (la L4 della conversione e' in
  `distintabozza/c39_pb_db_test.go`).
- **`sorgente.go`** — `LegaSorgente` (non cablata).
- **`piano.go`** — il Piano (EM App. H.7; direttiva 39 §4-§6): `Piano`, puro sopra `FattiPiano`, e `LeggiFattiPiano`
  (le occorrenze con la lettura del codice, tutte le AR attive della RFQ, le AR chiuse e i documenti degli allegati, le
  copie legacy senza AR; query di `v1_riepilogo_r4.sql`). Per ogni PB l'esito (`applicabile`, `gia_applicata`,
  `sostituzione`, `non_richiesta`, `in_attesa_codice`, `in_conflitto` con il motivo: `natura_non_preassociabile`,
  `legato_come_sorgente`, `step_candidato_del_finito` (G39-02 [U]: lo STEP candidato sorgente di un finito, non ancora
  confermato da S8; il fatto `OccorrenzaPiano.StepCandidatoDelFinito` lo mette chi chiama, `distintabozza` con il
  riconoscimento del Motore A), `file_non_attivo`, `legacy_da_verificare` (una PB legacy non verificata: `VocePB.Verificata`,
  R39-15), `gia_associata_altrove` con la variante
  `stesso_prodotto` | `altro_prodotto` | `copia_legacy`, `documento_senza_componente`, `documento_confermato_altrove`,
  `codice_non_letto`, `codice_diverso`); per ogni svincolo (`applicabile`, `svincolo_gia_chiuso`, `in_conflitto` con
  `svincolo_su_documento`, `svincolo_componente_diverso`, `svincolo_senza_associazione`); per le AR dei registrati che
  escono (`mantenuta` per un escluso, «Mantieni» di D-AR09; `nodo_uscente_con_ar` per un tolto senza decisione). Mai una
  sovrascrittura: la sostituzione vuole lo svincolo nella stessa bozza e un componente di questo prodotto. R4 lo mostra
  (`distintabozza/riepilogo3*.go`), R5 lo applichera'. Il codice letto si confronta senza maiuscole ne' spazi [T-39].
- **`piano_test.go`** — L1 del Piano: ogni esito e ogni motivo sopra fatti costruiti a mano, l'ordine.
- **`associazioni_test.go`** — L1: i controlli dell'atto e della provenienza prima di ogni lettura, quelli del gesto
  legacy, i motivi di chiusura e la loro origine, la traduzione dei rifiuti.
- **`associazioni_db_test.go`** — L4 (tag `integrazione`), con i trigger della 0023 accesi e una conferma formato 3
  vera per ogni R5: la nascita con la copia (Prova 2 per l'allegato senza proposta), l'unicita' della AR attiva, la
  chiusura, la riassociazione e la sostituzione con la storia, lo svincolo e `svincolo_gia_chiuso`, l'idempotenza,
  la copia sempre uguale alla fonte (Prova 11, lato servizio), la concorrenza (due R5, due gesti FOR KEY SHARE, R5
  contro un gesto legacy: nessuno stallo), il legame della sorgente, l'allineamento del codice.

## Query

- `internal/platform/db/queries/v1_associazioni.sql` (file proprio della corsia, INC-G21), senza `ON CONFLICT` e
  senza lucchetti.

## Direttiva 39: «RFQ formato 3» (`formato3.go`)

`RFQFormato3(ctx, q, thread)`: la RFQ ha avuto, in qualunque momento, una bozza o una conferma formato 3 su un suo
prodotto; l'abbandono non la riporta a legacy. Legge l'unica definizione del database, `rfq_e_formato3` (variante
`20261019c`): il segno `rfq_formato3_segno` scritto alla nascita della prima bozza formato 3, che la conservazione delle
bozze abbandonate non tocca (il limite dei 90 giorni della variante `20261019b` non c'e' piu'). Decide la strada dei
gesti atomici (D-AR03 D: nelle RFQ formato 3 solo PB con R2; nelle RFQ interamente legacy il servizio unico). Prove
nella L4 di `core/rfq/sorgente` e di `platform/migrazioni` (`TestV1D39M50LeOriginiLegacy`).
