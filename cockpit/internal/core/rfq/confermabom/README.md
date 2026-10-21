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

## La verifica di V1 (ondata C)

- **`primaistanza.go`**: `PrimaIstanza`, la regola `prima-istanza-1` di U2 §5.4, pura: le sei condizioni (criterio,
  fonte confermata, corrente e autorizzata con la sorgente uguale, righe e rimozioni, gerarchia o deroga, secondari,
  conflitti del motore), i motivi nell'ordine, «non calcolata» con i blocchi aperti o senza i conflitti del motore.
  La chiamano R4, la S di R5 e `/dati` con gli stessi ingressi; nessuno la salva positiva: il formato 3 scrive in
  `conferma_strutturale.verifica` solo `ColonnaVerificaFormato3()` (CHECK `ck_conferma_verifica` della 0023). Non e' la
  `BOM.Verificata` del fascicolo: nessuna guardia la usa in V1. Prove: `primaistanza_test.go` (L1).

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

## Direttiva 39: la sorgente valida per R5 (`sorgente.go`)

`SorgenteValida(ctx, q, thread, prodotto, sha256)`: lo sha256 e' una sorgente valida per il gate di R5 se nella storia
della fonte del prodotto c'e' una `conferma_documento` scritta da S8 (con la chiave del passo) non seguita da una
`revoca` o da un `rifiuto` dello stesso contenuto, e il documento c'e', e' un `cad_3d` `.stp`/`.step` del prodotto con
quello sha256 (EM App. I.2 punto 5; dal blocco R4 anche l'estensione) e non e' sostituito. Una `conferma_documento` senza chiave (pagine legacy) e' il legame storico (`Storica`): non basta.
Motivi: `nessuna_conferma_s8`, `revocata`, `rifiutata`, `documento_assente`, `documento_sostituito`. Sola lettura: R4 e
R5 la useranno sullo stesso stato (R5 sotto il lucchetto). Prove nella L4 di `core/rfq/sorgente`.

Giro G39 (G39-03 [U], forma [T-39]): la causa `sorgente_errata` (`CausaSorgenteErrata`, `MotivoConCausa`,
`CausaDelMotivo`) sta in testa al motivo registrato, nei campi che ci sono gia' (revoca della sorgente, riga di S8,
revoca scritta dallo spostamento). `StatoSorgente.Correzione`: l'ultima conferma S8 corregge una sorgente sbagliata con
cui la BOM del prodotto era stata confermata, e solo in due casi: il motivo della riga di S8 ha la causa
(`CorrezioneDichiarata`, il corpo di S8 diceva `correzione_sorgente`; il gesto «sposta la sorgente» non la scrive sulla
riga di S8), oppure nella revisione in corso (fra la base, l'ultima conferma della BOM del prodotto, e la S8) c'e' una
revoca con la causa di un altro STEP entrato in una conferma della BOM del prodotto (`UsataDaConferma`: STEP strutturale,
fonte di una conferma strutturale, baseline). Conta qualunque revoca della revisione, non solo l'ultima prima della S8:
dopo lo STEP sbagliato della BOM si possono togliere altre sorgenti sbagliate (revoca o «sposta la sorgente») prima di
quella giusta (rilievo bloccante della revisione indipendente 3 della corsia interfaccia). Una S8 gia' entrata nella base
non corregge piu' nulla: dopo la R5 della correzione `Correzione` e' falso, e la revisione che segue e' del cliente. Una
sorgente sbagliata revocata o spostata prima di R5 non basta: la revisione che segue resta del cliente (rilievo bloccante
della revisione indipendente del giro G39). R4 e R5 ne fanno la «correzione della sorgente» di una revisione (revisione
interna, non del cliente). `MotivoConferma`: il motivo della riga di S8.

## Direttiva 39: il legame storico univoco (`legame_storico.go`)

`LegameStoricoUnivoco(docs, conS8)`: il legame storico di D-AR04 (iii) A [U] («se esistono piu' candidati o evidenze
discordanti, non si seleziona automaticamente una sorgente»), letto per i prodotti senza conferme di S8 (R39-06): uno e
un solo documento `cad_3d` STEP (estensione `.stp`/`.step` o il nome del file, `EstensioneOFileStep`) corrente, non
sostituito; con zero, con piu' di uno o con una conferma di S8 nessuno. Pura. La usa `legami_storici[]` di R4
(`distintabozza`); `/dati.fonte.legame_storico` del Motore A segue la stessa regola. Prove: L1
`legame_storico_test.go`; L4 `distintabozza/c39_gate_prove_db_test.go` (`TestV1D39R4LegamiStoriciUnivociDB`).
