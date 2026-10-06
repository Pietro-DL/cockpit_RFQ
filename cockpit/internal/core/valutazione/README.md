---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/valutazione` — il percorso puro del motore A: richiesta, target e fonte

## Scopo

- Il **percorso puro comune del banco e dell'anteprima** (giro 5; piano A, par.3.3.8, 6.4.6; R42 B): dalla
  fotografia già chiusa produce il «nuovo» del motore A e, per ogni prodotto target della RFQ, i suoi assi (R79).
  Non confronta e non vede mai l'atteso (R2).
- In B1 (commit P7a):
  - **gli ingressi del file dei casi** (`LeggiIngressi`, `Ingressi`, `IngressoCaso`, `SegmentoDichiarato`): la
    decodifica stretta, con i segmenti dello scenario sempre di origine «scenario» (R29 a A, b C; R75 A);
  - **la richiesta del thread** (`RichiestaDelThread`): una per thread, il gesto 1 per messaggio (T-B0-06),
    `valutata` solo con il gesto dell'operatore, l'entrata e la controparte del cliente del thread; il triage solo
    evidenza (R29 c); l'uso dei segmenti dal caso o dal riconoscimento tracciato della parte corrente (R48 A); le
    decisioni come impronta dei gesti (T-16);
  - **i prodotti target** con l'asse 1, l'identità (R60 A, R70 A, R75 A, R78): `ProdottoValutato` con i soli campi di
    B1, `StatoIdentitaTarget`, il predicato `TargetConfermato`;
  - **la fonte strutturale STEP**, l'asse 2 (R65 A, R76 b A; T-B0-07, T-B0-08, T-E1-09, T-12): `FonteProdotto`,
    `StatoFonte`, `MotivoFonte`;
  - il punto d'ingresso **`ValutaProdotti`**, quello che `Calcola` (B6) userà per ogni thread.

## Non appartiene qui

- **Il DB**: la fotografia la legge `core/fotorfq/caricatore`, prima e altrove; qui niente DB, file, orologio, rete.
- **I candidati della mail**: li propone `core/ancoraggio` (`ProponiProdotti`); qui si chiamano, e non diventano mai
  target (R60 A). Lo stesso per i tipi della fonte (`RiferimentoFonte`, `DocumentoCandidato`, …), che stanno in
  `ancoraggio`.
- **Il confronto con le proposte attuali e con l'atteso**: `core/confronto` e `app/bancoa`. `valutazione` non importa
  `confronto`, il caricatore, la libreria YAML.
- **Gli altri assi** (nomenclatura, gerarchia, smistamento, completezza), lo stato del prodotto, il fascicolo,
  l'impronta, `Calcola` ed `Esito`: B5 e B6, nello stesso pacchetto, dopo.
- **Il significato delle codifiche dei clienti**: le grammatiche, lette da `motorea`.

## File

- **`valutazione.go`** — responsabilità:
  - commento `// Package`; `ValutazioneProdotti`, `ValutaProdotti`: i documenti dei messaggi, la richiesta,
    l'interpretazione, i candidati della mail, i target, la fonte di ogni target, le diagnostiche in ordine.
- **`ingressi.go`** — responsabilità:
  - `VersioneCasi` (1), `Ingressi`, `IngressoCaso`, `SegmentoDichiarato`, `Ingressi.CasoDelThread`, `LeggiIngressi`;
  - la porta stretta: BOM, UTF-8, surrogati soli, versione, chiavi sconosciute, ripetute o con le maiuscole diverse,
    null, numeri non interi, testo dopo l'oggetto; poi i controlli del contenuto (ID, cliente, thread, uso, origine,
    autorità). I codici sono quelli «contratto.*» della grammatica.
- **`richiesta.go`** — responsabilità:
  - `RichiestaDelThread`: lo stato di ogni messaggio e del thread, l'evento del triage come evidenza, l'uso dei
    segmenti con il caso o con il riconoscimento, le decisioni (T-16).
- **`target.go`** — responsabilità:
  - `StatoIdentitaTarget`, `ProdottoValutato` (Rif, Autorita, ComponenteID, CodiceRichiesto, Base, Identita, Fonte),
    `TargetConfermato`;
  - i target del thread (R70 A, R75 A), la lettura del codice registrato con la grammatica (6.4.6, nell'ordine fisso
    nome_file, cartiglio.codice, nodo_step.id, corpo), `target.possibile_rinomina` (T-E1-24).
- **`fonte.go`** — responsabilità:
  - `StatoFonte`, `MotivoFonte` (i sedici valori), `FonteProdotto`;
  - gli STEP del thread letti dai fatti con gli adattatori di `estrazione` e con le letture della grammatica, il
    confronto delle basi con `motorea.ConfrontaBasi`;
  - la fonte di un prodotto: la tabella di R65 sugli esiti della vista, il gesto 3, i candidati, l'ordine dei motivi
    di una fonte assente, T-12.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici di B1: `ancoraggio.target_non_leggibile`, `target.possibile_rinomina`,
    `fonte_strutturale.riferimento_incoerente`, `fonte_strutturale.radice_non_registrata`.

## Entry point

- **`ValutaProdotti`** — chi lo chiama: da B6 `Calcola`, per ogni thread, nel banco (`app/bancoa`) e da A1d
  nell'anteprima (`transport/web`); in B1 le prove.
- **`LeggiIngressi`, `Ingressi.CasoDelThread`** — chi li chiama: il banco, con la voce `casi` del manifest;
  l'anteprima, con il file che l'indice delle regole dichiara (R29 b C): lo stesso file.
- **`RichiestaDelThread`, `TargetConfermato`** — chi li usa: `ValutaProdotti`; da B6 lo stato del prodotto
  (`ProdottoVerificato` comincia da `TargetConfermato`, R79).
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **I target sono solo i confermati e lo scenario** (R60 A):
  - confermato (R70 A, R78): un identificativo con `confermato_da`, con il suo componente (stesso codice, senza
    maiuscole: il legame è per codice, come nel DB, LD-21) se c'è e non è archiviato; un finito attivo di origine
    «manuale». `componente.confermato_da` non conta mai. Rif: `componente:<uuid>` con il componente,
    `identificativo:<codice>` senza (emendamento E1 §4.2);
  - scenario (R75 A): con un caso di autorità «scenario», i prodotti letti dalla mail in un segmento che il caso ha
    scelto, sempre `da_confermare`, mai `TargetConfermato`, qualunque sia il gesto 1. Rif: `scenario:<caso>:<n>`.
    Uno con la base di un target confermato non si ripete;
  - i candidati della mail non sono mai target, e restano da confermare.
- **La fonte non si conferma mai da sola** (contratto §0 n.6): `confermata` viene solo dal gesto 3, letto attraverso
  la vista (R65 A, l'unica definizione). Un candidato del motore A porta una fonte assente in attesa di conferma, mai
  una confermata né una in attesa (T-B0-07). Candidato è solo uno STEP la cui **radice** ha la base del prodotto
  (R76 b A); un prodotto solo come nodo interno dà `solo_nodo_interno`. Solo lo STEP è fonte, mai un PDF (R68 A,
  T-E1-09).
- **L'ordine dei motivi di una fonte assente** (T-E1-09): `estrazione_fallita`, `analisi_in_corso`,
  `step_presente_non_analizzato`, poi `solo_nodo_interno`, `indicata_sul_portale`, `solo_altro_3d`,
  `prodotto_senza_componente` (un target senza componente, T-B0-07), `nessuna_fonte`.
- **«Estrazione fallita» non è «dati insufficienti»** (T-B0-07): la prima è il worker che non ha letto il file, la
  seconda un grafo incompleto della vista o di un candidato.
- **T-12**: con una sezione della fonte assente la fonte non si calcola: niente stato, motivo `non_determinabile`.
- **Il T-12 allargato** (lettura dell'orchestratore T-B1-11): senza grammatica o con il codice del target non letto,
  quando uno STEP analizzato del thread potrebbe essere candidato (o è la radice di una delega), la fonte non si
  calcola (`non_determinabile`) e l'esito della vista resta accanto; uno stato già dato dalla vista resta, salvo la
  delega con la radice non confrontabile.
- **Il triage è solo evidenza** (R29 c): sta in `Evento` e nel motivo del riconoscimento, mai nello stato.
- **La storia non si promuove**: con il caso valgono solo i segmenti dichiarati; senza, il riconoscimento tocca solo
  la parte corrente di un messaggio valutato (R48 A, E2).
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; gli elenchi della fotografia si
  leggono in un ordine fisso, e la fotografia di chi chiama non cambia. Due esecuzioni sugli stessi ingressi, anche
  permutati, danno gli stessi byte canonici.
- **Valori che dichiarano altri pacchetti** e che `valutazione` ripete, perché non li esportano: gli enum del DB
  (aggancio, direzione, controparte, tipo e origine del componente, tipo del documento, natura dell'allegato), gli
  esiti di `v_step_prodotto`, il ruolo «delega» della marcatura, le capacità «struttura» e «grafo_completo» degli
  adattatori, l'elenco degli altri 3D del legacy. Se cambiano, lo dicono le prove del pacchetto.

## Dipendenze

- **Importa:** `core/fotorfq` (la fotografia), `core/estrazione` (i documenti dei messaggi e degli STEP),
  `core/ancoraggio` (le proposte e i tipi della fonte), `core/inbox/classificazione/motorea` (l'interpretazione, le
  letture, il confronto delle basi), `core/registro/regole/grammatica` (i codici «contratto.*» della porta stretta),
  `core/estrazione/evidenze` (il documento, le diagnostiche), `platform/jsoncanonico` (l'impronta delle decisioni),
  `github.com/google/uuid`, la libreria standard. Mai `core/confronto`, il caricatore, il DB, la libreria YAML (grafo
  del par.3.2.1).
- **Solo nei test:** il caricatore e `platform/testutil`, nella L4 sul database di prova.
- **È importato da:** ancora nessuno nel prodotto; da B6 il banco, da A1d l'anteprima.

## Test

- **`ingressi_test.go`** — L1 — A1c-L1-21: il file `testdata/casi_acme.v1.json` letto per intero; ogni violazione
  della porta stretta e del contenuto con il suo codice e il suo percorso.
- **`richiesta_test.go`** — L1 — A1c-L1-17 riscritta: lo stato di ogni messaggio (gesto, direzione, controparte;
  il triage che non conta), lo stato del thread, le decisioni, il caso e il riconoscimento, la mail inoltrata senza
  confine senza caso, banco e anteprima con lo stesso file.
- **`target_test.go`** — L1 — PO-19 (parte B1), R70 A in tutte le sue forme, R75 A, `target.possibile_rinomina`,
  il codice non letto, senza grammatica, il determinismo con gli elenchi permutati.
- **`fonte_test.go`** — L1 — ogni riga della tabella di R65, il gesto 3 con la marcatura e nelle forme rare
  (T-B0-08), la delega, R76 b A con il nodo interno, T-B0-07, PO-34 con l'ordine dei motivi coppia per coppia, un PDF
  mai fonte, T-12, i valori del contratto.
- **`fonte_db_test.go`** — L4 (`-tags integrazione`, il database di prova) — la scena ACME scritta con SQL, letta con il
  caricatore: PO-19 sul DB, la fonte del finito dalla vista e dal gesto 3, la fonte dell'identificativo dallo STEP
  del thread; il database non cambia.
- **`scena_test.go`** — gli aiuti: la grammatica ACME, i messaggi, i fatti STEP.
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go`; i codici nell'elenco d'oro (A1a-CAT,
  A1c-L1-32) in `core/estrazione/evidenze/codici_diagnostica_test.go`.
- I clienti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/ancoraggio/README.md`
- `internal/core/fotorfq/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/README.md`
