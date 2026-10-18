---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/rfq/distintabozza` — la bozza della distinta salvata sul server (P4)

## Scopo

- Giro 5, A1d, P4, blocco B1 (direttiva 2026-10-12-06; contratto backend A1d v2 **rev. 3** con gli Emendamenti 1 e 2,
  §3 e §6; migrazione `0022_p4_conferme_working.sql`). Correzioni della revisione del codice del 12/10 (RC-01…RC-19)
  fatte il 13/10.
- La bozza formato 2 di un prodotto finito, salvata sul server a ogni versione:
  - lo **snapshot completo** (lo stato desiderato del perimetro: nodi, archi, decisioni, preassociazioni, fonte,
    base) in `distinta_bozza_versione`;
  - il **giornale** delle azioni (azione, prima, dopo, origine, autore, data) in `distinta_bozza_evento`, solo in
    aggiunta;
  - la testata (`distinta_bozza`): aperta, confermata o abbandonata, con la versione ottimistica.
- Il server **riconcilia**: le azioni dichiarate dalla pagina si rifanno in ordine sullo snapshot precedente, ognuna
  deve trovare il suo «prima», ogni stato intermedio deve essere una bozza possibile, e il risultato deve essere
  esattamente lo snapshot nuovo. Nessun evento inventato diventa prova senza confronto, e nessun cambiamento passa
  senza un'azione che lo dica (ratifica 05 §2, direttiva 06 B1).
- La **prima versione** non si adotta com'e' arriva: comincia con `bozza_aperta` e la sua partenza; lo stato di
  partenza che le azioni descrivono si confronta con quello che il server ricostruisce dal database (§6.6).
- Salvare **non** tocca `componente`, `componente_relazione`, proposte, documenti, NAS, F8: non e' una conferma
  (contratto §6.2 «Regole»). Lo prova la L4 con la foto del database.

## Non appartiene qui

- **Le rotte HTTP R1-R3** (`GET`/`POST …/motore-a/bozza`, `…/abbandona`): **NON ANCORA IMPLEMENTATE**, sono di B2
  (`transport/web/motore_a`). Qui ci sono le funzioni che useranno, e i codici d'errore che tradurranno in
  404/409/422.
- **R4-R11** (riepilogo, conferma formato 2, intento, conferme, piano e lotto del fascicolo, correzioni, F8): B2-B4.
- **Le regole di dominio del formato 2** (i blocchi e gli avvisi del contratto §4.4: D1, E37, cicli e duplicati
  sulle identita' risolte, collisioni di codice, `CODICE_DA_ASSEGNARE`, `ARCO_CONDIVISO_QTA`, mono_pezzo con la fonte
  STEP…): il riepilogo di B2/B3. Una bozza salvata puo' avere blocchi: la si salva lo stesso, e il riepilogo li
  mostra. Per questo un ciclo fra chiavi o due nodi sullo stesso componente passano la forma (RC-14).
- **L'appartenenza dei `componente_id` dei nodi, delle righe di proposta e delle proposte preassociate** alla RFQ, e
  l'uguaglianza del codice di un nodo `riusa` con quello registrato: B2, quando risolve le identita' (§3.2, Δ31). Qui
  si controllano il prodotto, la bozza, le conferme delle basi e gli allegati delle fonti.
- **La firma del perimetro** (`firma_base`): B1 la registra com'e'; la ricalcola e la confronta la conferma (B2, §5.1).
  Nel rebase B1 controlla solo l'ultima conferma e il contatore struttura (409 `BASE_SUPERATA`).
- **La forma dei campi delle 13 decisioni**: qui `id` (unico), `tipo` (fra i 13), numeri interi, niente U+0000,
  stringhe NFC, nomi dei campi in minuscolo; i campi ammessi e obbligatori per tipo li controlla B2 (E1.5).
- **Le partenze `conferma` e `step`** della prima versione: vogliono lo snapshot dell'ultima conferma formato 2 e la
  proposta del gruppo del Motore A (B2). Fino ad allora R2 risponde 422 `partenza_non_ricostruibile`.
- **`fonte_step_evento`** (la storia della fonte, con il ruolo e la destinazione di una fonte secondaria, E2.2): la
  scrivono R12 e il rebase in B2/B4. Qui la destinazione temporanea resta nel giornale (chiave della bozza su
  `sorgente_vista` / `sorgente_rifiutata`).
- Le conferme strutturali, l'esito degli intenti, i contatori della working: le loro query sono in `platform/db`
  (`p4_conferme.sql`), le scriveranno B2-B3.

## File

- **`snapshot.go`**
  - `Snapshot` e i suoi tipi (`Nodo`, `Arco`, `Casella`, `Sorgente`, `VersioneBomBase`, `Preassociazione`)
  - `LeggiSnapshot`: lettura stretta (rilettura di `jsoncanonico`: UTF-8 valido, nessuna chiave ripetuta, niente
    dopo il valore; nomi dei campi in minuscolo; poi `DisallowUnknownFields`) e le **regole di forma** del contratto
    §3.2 con i limiti delle colonne (`MaxVoci` 5000, `MaxQta` 100000, codice nuovo ≤ 40 e codice registrato ≤ 60,
    rev, descrizione 200, posizione 20, gruppo 300, rif 400, snapshot 2 MiB), `sorgente` obbligatoria con le caselle
    (Δ43), origine `conferma` solo con una base ≥ 1 (Δ32), un nodo `crea` anche senza codice (E2.3)
  - la **forma canonica** (`Canonico`, `Impronta`) del contratto §6.3
- **`azioni.go`**
  - il **vocabolario 1** delle azioni (`Vocabolario`, `AzioniDelVocabolario`, le costanti `Az*`, le partenze) e
    `Azione`, `Evento`, `Contesto`
  - `Riconcilia`: il rifacimento delle azioni (pura: niente DB, niente orologio; gli eventi gia' salvati li da'
    `Contesto.Precedente`)
- **`partenza.go`**
  - `ControllaPartenza` (lo stato di partenza delle azioni contro quello ricostruito, per identita') e
    `SnapshotDiPartenza` (lo stato di partenza ricostruito, come snapshot)
- **`persistenza.go`**
  - `Salva` (R2: crea o salva), `Abbandona` (R3), `Leggi` (R1), `LeggiBozza`, `LeggiEventi` (a pagine),
    `LeggiStoria`, `VerificaGiornale`
- **`errori.go`**
  - `Errore` con codice, motivo, campo, `Motivi` e `Conflitto`; i codici `CORPO_NON_VALIDO`, `FUORI_RFQ`,
    `RFQ_INESISTENTE`, `BOZZA_VERSIONE`, `BASE_SUPERATA`; i valori da confrontare con `errors.Is`
    (`ErrConflittoVersione`, `ErrAltraBozzaAperta`, `ErrAzioniNonRiconciliate`…); `ErrSnapshotIncoerente`
- **`formato3.go`** (V1, passo 2 del piano della 0023; emendamento di V1, EM §3)
  - `Snapshot3` e i suoi tipi (`Sorgente3` con `importazione`, `Nodo3` con `Identita` decisa e le caselle
    `{allegato_id, gruppo, rif}`, `Arco3` con `step`), `FormatoSnapshot3`, `VocabolarioFormato3`
  - `LeggiSnapshot3`: lettura stretta (come il formato 2) e le regole di forma del formato 3 senza il database: i sei
    modi dell'identita' con l'op e le caselle, la presunzione dell'emittente dove non dipende dal database
    (`emittente_presunto_diverso`), caselle e step con l'occorrenza, il gruppo e lo sha256 della sorgente
    (`rif_fuori_importazione`), un rif in un nodo solo, la radice con la casella se c'e' un registro, `qta` sempre
    (`arco_senza_quantita`), il nodo in modo decisione legato alla sua decisione `identita` (del nodo stesso) o
    `ricodifica` con la stessa identita', il modo proposta solo su un nodo da creare con il codice proposto, una
    `non_includere` per rif e mai sul rif di una casella (EM §7.2)
  - la **rev**: `RevAmmissibile` sempre; senza minuscole ASCII quando la introducono l'ingegnere o il registro (crea,
    modifica, manuale, proposta, correzione con un codice nuovo); byte per byte quando e' copiata da una fonte che le
    ammette (la working di un riusa o di un esclude, la bozza formato 2 di un nodo `formato_2`, il codice manuale
    adottato con `riferimento: manuale_legacy`): `revDallaFonte`, proposta di emendamento P-18
  - la regola dell'**NFC** (N-01): solo descrizioni, motivi e posizioni; l'identita' e le copie dal registro byte per
    byte; la **forma canonica** (`Canonico`, `Impronta`): la chiusura dello scrittore, la stessa normalizzazione del
    lettore (NFC dei testi umani, decisioni rilette con la loro forma chiusa), le regole di forma, il limite di 2 MiB e
    la rilettura dei byte prodotti, che deve dare gli stessi byte (CORPO_NON_VALIDO altrimenti, mai uno snapshot salvato
    che `RileggiVersione` non ritrova); `RegistriCitati`
  - la regola unica della forma dell'emittente: `EmittenteAmmissibile`, `NamespaceAmmissibile` (la grammatica, con il
    tetto di sicurezza del CHECK `ck_csc_identita`), `EmittenteCliente`, `PresunzioneAttesa`, i casi di T-14
    (`CasiT14`, esportati perche' la parte L4 in `platform/migrazioni` usi gli stessi)
- **`decisioni3.go`**
  - `Decisione3`: le 16 decisioni (i 13 tipi del contratto rev. 3, `identita`, `non_includere`, `scollega_documento`)
    con una forma chiusa per tipo (`formeDecisione`: campi obbligatori e facoltativi), `LeggiDecisione3`; `cambia_tipo`
    porta il tipo nuovo in `tipo_nuovo` (proposta di emendamento: «tipo» e' la chiave del tipo della decisione)
  - ondata B (PE-27, decisione [T] dell'integratore del 18/10): `scollega_documento` ha il campo facoltativo
    `da_decisione`, l'id della decisione `escludi` da cui nasce lo svincolo; lo snapshot vuole un'esclusione dello stesso
    snapshot e dello stesso componente (altrimenti 422 `CORPO_NON_VALIDO` sul campo), cosi' «↺ Ripristina» toglie
    l'esclusione e i suoi svincoli nello stesso gesto
- **`vocabolario2.go`** (ondata B)
  - il vocabolario 2 del giornale del formato 3 (EM §9.2): i verbi del vocabolario 1, `proposta_riallineata` (solo del
    server) e `file_lasciato`; la tabella delle forme di prima e dopo per verbo (PE-03, con gli esempi dei casi condivisi
    `transport/web/e2e/distinta_unica_casi.json`); l'elenco chiuso dei motivi di `file_svincolato` in `dopo.motivo`
    (PE-04, PE-22: `nodo_escluso`); il prima di `annulla` e `rifai`, null oppure `{azione_id}` (PE-05)
- **`verifiche3.go`** (ondata B)
  - le verifiche contro un registro, senza il database (EM §6.2): V-5 sul segnale visto (`versione_servizio` =
    `regole.versioni.ancoraggio` del registro, PE-17), V-3 sulla voce intera, V-1 e V-3 della lettura di una decisione
    `identita` (la lettura legata alla voce, la copia byte per byte, il codice manuale), l'identita' di un nodo in modo
    `proposta` (`rev_proposta` vuota = rev assente, PE-11)
- **`lettori.go`**
  - `RileggiVersione` (le versioni nei formati 2 e 3, con l'impronta ricontrollata), `LeggiSnapshotConferma` (le
    conferme nei formati 1, 2 e 3: un contenuto illeggibile torna con `NonLeggibile` e i byte grezzi, mai nascosto)
  - la testata del formato 3 in R1 (`TestataFormato3`, `ImportazioneLetta`, `RegistroCitato`, `VocabolarioDelFormato`)
    e il passaggio fra il DTO del registro e le righe di `distinta_importazione` (`TestataImportazione`,
    `RileggiImportazione`, `ParametriImportazione`)
- **`importazione/`** (pacchetto a parte): il registro d'importazione del Motore A (EM §3.8), una foglia senza database
  e senza `core/rfq`, perche' lo usi anche l'adattatore puro `propostabom` (vedi il suo README)
- **Le rotte della bozza nel formato 3** (V1, ondata B, passo 6 del piano della 0023, prima parte; EM §9):
  - **`applica2.go`**: la regola d'applicazione del vocabolario 2 sullo snapshot in JSON, la stessa della pagina
    (`applica` di `distinta_unica.mjs`, i casi condivisi di T-16), l'inverso di ogni verbo (annulla) e il rebase della
    sola base
  - **`riconcilia3.go`**: `Riconcilia3`, il rifacimento delle azioni del vocabolario 2 (pura): la forma di ogni verbo, gli
    stati intermedi possibili (una radice sola, un rif in un nodo solo e mai in una `non_includere`, `nodo_tolto` senza
    archi ne' preassociazioni, mai la radice), l'evento scritto dal server con l'`azione_id` del gesto, annulla e rifai
    fermi all'ultima versione costruita dal server (`annullamento_non_ammesso`) con il prima `{azione_id}` (PE-05), il
    rebase che cambia solo la base (`rebase_cambia_sorgente`), lo snapshot finale uguale a quello rifatto. Secondo
    passaggio dell'ondata B: in `nodo_modificato` e `nodo_riusato` prima e dopo nominano gli stessi campi, fra quelli del
    verbo (mai la chiave ne' l'origine), cosi' l'evento rifa' sempre la versione (T-10); la chiave degli eventi delle
    decisioni e' quella della decisione, null per i tipi senza `chiave_bozza` (RPC PD-08, raccomandata (a)); i rif
    scaduti del dopo del rebase li scrive il server (`[]`); i riferimenti delle azioni (`Riferimenti`) e i nodi aggiunti
    (`NodiAggiunti`) per le verifiche di chi salva
  - **`vassoio.go`**: `VassoioDa`, il vassoio «Da ricollocare» dal giornale (EM §3.7, T-16)
  - **`stato3.go`**: `RifScaduti` (le cinque condizioni di EM §6.4, con i motivi [T] `allegato_fuori_rfq`,
    `fatti_assenti`, `chiave_assente`, `sha_diverso`, `file_non_attivo`), `BaseDellaBozza`, `LeggiAllegatiCitati`
  - **`leggi3.go`**: `LeggiR1` (R1 nel formato effettivo: testata, snapshot, `n_eventi` sempre ed `eventi[]` con
    `?eventi=1` (PE-02), registro della versione e citati, versione corrente, base, rif scaduti, vassoio, `da_rivedere[]`;
    una bozza formato 2 con `conversione: {necessaria, forma: converti_da}`); gli eventi di R1 sono `EventoR1`, con la
    chiave nel campo `chiave` (EM §9.1, §9.2; RIC-P6)
  - **`richieste3.go`**: `LeggiRichiestaR2` (una e una sola forma; in questa ondata solo `salvataggio`, le altre 422
    `forma_non_disponibile`), `LeggiRichiestaR3`; l'impronta della richiesta sulla forma canonica del corpo
  - **`salva3.go`**: `SalvaFormato3` (R2, forma salvataggio): la chiave globale prima di tutto (riproposto o 409
    `CHIAVE_RIUSATA`), la bozza della RFQ e del prodotto, la versione attesa, `Riconcilia3`, le verifiche dei soli
    riferimenti nuovi o cambiati contro il database e i registri (appartenenza, `FUORI_RFQ`, `FILE_NON_ATTIVO`, V-1,
    V-2, V-3, V-5, la regola unica dell'emittente, l'identita' registrata, i nodi `formato_2` contro la versione 1), il
    rebase (409 `BASE_SUPERATA` con la versione corrente, 422 `rif_scaduti`), versione ed eventi del vocabolario 2.
    Secondo passaggio: gli archi step nuovi o cambiati contro il registro della versione o, senza registro, contro le
    relazioni dei fatti correnti (EM §3.5); la `non_includere` nuova senza importazione contro il registro della versione
    (EM §7.1); il modo dei nodi (`da_decidere` senza codice proposto; i nodi aggiunti con le regole P-2...P-6, EM §6.1);
    gli UUID e i registri che solo le azioni nominano
  - **`intento.go`**: `LeggiIntento` (R6, con la riga della RFQ FOR KEY SHARE e 3 s di attesa: `in_corso`)
  - **`storico.go`**: `LeggiStorico` (R7 nei formati 1, 2 e 3, con `verifica_calcolata`, `confermato_da_nome` (PE-07),
    le attestazioni, la storia della fonte e il dettaglio con i registri); `verifica_calcolata` vale vero per le
    conferme formato 3, falso per le formato 1 e 2 (EM §9.7, MAT §6.2, PM23 R-17)
  - **`conversione.go`**: `Converti` (conversione-1, EM §6.7, U-05): il servizio della forma `converti_da`, che R2
    chiamera' con l'ondata C; gli archi dello STEP che nel formato 3 non uniscono due caselle restano dell'ingegnere,
    con la traccia `archi_senza_step` (proposta [T] della corsia). Secondo passaggio: le fonti secondarie si riportano
    per chiave, nell'ordine (scelte e rifiuti), quelle di una chiave che non e' piu' un nodo senza UUID no
    (`fonti_non_riportate`); la ricodifica del formato 2 prende l'emittente del cliente (`decisioni_completate`); un nodo
    step senza caselle resta dell'ingegnere (`nodi_senza_casella`); i rifiuti con i motivi propri
    (`conversione_casella_fuori_sorgente`, `conversione_senza_occorrenza`, `conversione_decisione_non_convertibile`); la
    risposta elenca le righe cadute dei soli nodi senza casella; una traccia oltre i 128 KiB dell'evento diventa il
    conteggio (proposte [T] della corsia)
  - **`giornale3.go`**: `VerificaGiornale3` (T-10, la parte senza l'adattatore): le versioni della pagina rifatte dai loro
    eventi con `Riconcilia3`, la versione 1 di una conversione rifatta con conversione-1; le versioni costruite con
    l'adattatore restano fra le non verificate, con il motivo (`ErrGiornaleIncoerente` per un giornale che non torna)

## Entry point

- **`Salva(ctx, pool, Salvataggio) (Salvato, error)`**
  - prima della transazione: vocabolario 1, snapshot (forma), prodotto dello snapshot = prodotto della richiesta,
    base nello snapshot (`versione_bom_base`, `firma_base`: Δ06, nessun campo «base» a parte), versione attesa
  - una transazione: `BloccaThread` → prodotto finito attivo della RFQ → (bozza nuova: nessuna aperta; bozza
    esistente: `BloccaDistintaBozza`, aperta, versione attesa, snapshot corrente ricontrollato con l'impronta) →
    `Riconcilia` → (prima versione: stato di partenza dal database) → campo `rebase` coerente con l'azione → fonti e
    basi citate della RFQ → (rebase: base corrente) → testata (solo se nuova) → versione n+1 con snapshot canonico e
    impronta → eventi `n0+1…` → COMMIT
  - stesso snapshot e nessuna azione: nessuna versione nuova (`NuovaVersione` falso); stesso snapshot con azioni:
    versione nuova (§6.2)
  - i rifiuti del database che dipendono dal corpo (jsonb oltre i CHECK di 4 MiB e 256 KiB, U+0000) diventano
    `CORPO_NON_VALIDO` con il campo, mai un errore grezzo (RC-07)
  - chi chiama limita il corpo (B2: 2 MiB + 4 KiB); qui lo snapshot e' al piu' 2 MiB e le azioni al piu' 2 MiB in tutto
- **`Abbandona(ctx, pool, Abbandono) (Abbandonata, error)`**: `BloccaThread` → `BloccaDistintaBozza` → aperta alla
  versione attesa → `AbbandonaDistintaBozza` (confronta e scambia)
- **`Leggi(ctx, q, thread, prodotto) (Lettura, error)`**: RFQ (`RFQ_INESISTENTE`), prodotto finito della RFQ
  (`FUORI_RFQ`; un finito archiviato si legge), poi la bozza aperta o `nessuna_bozza`. «eventi» e' un numero
  (fino alla versione letta); l'elenco con `LeggiEventi` (a pagine, al piu' `MaxEventiPagina`). `versione_corrente`,
  `base` e `rif_scaduti` del §6.4: B2
- **`LeggiBozza`**, **`LeggiStoria`** (tutte le versioni e il giornale: diagnosi e prove, senza tetto),
  **`VerificaGiornale`** (rifa' il giornale versione per versione)
- Chi le chiama: in B1 solo le prove; da B2 `transport/web/motore_a` (R1-R3).
- **Formato 3** (V1, ondata B): `LeggiR1`, `SalvaFormato3` (+ `LeggiRichiestaR2`), `Abbandona` (R3, invariata, per i
  formati 2 e 3), `LeggiIntento` (R6), `LeggiStorico` (R7), `Converti`; le chiama `transport/web/motore_a_bozza_v1.go`
  (R1, R2, R3, R6, R7, R11), montato da chi possiede `registraMotoreA`.

## Il vocabolario 1 delle azioni (contratto rev. 3 §6.6)

- Ogni azione dice che cosa trova (`prima`) e che cosa lascia (`dopo`); il server scrive nel giornale la sua forma
  canonica, non il testo del corpo, e la `chiave_bozza` dedotta (nodo; figlio per un arco; nodo della
  preassociazione; nodo che la decisione nomina; nodo dell'evento annullato). Una chiave dichiarata diversa si rifiuta.
- **`bozza_aperta`**: solo come prima azione della prima versione, `dopo = {partenza}` (`conferma`, `registrato`,
  `step`, `vuota`). Lo stato di partenza si ottiene disfacendo le azioni dallo snapshot nuovo (con le chiavi della
  pagina) e si confronta per identita' con quello ricostruito: `vuota` = la sola radice registrata, `registrato` = il
  perimetro della working con qta e posizione, senza i componenti archiviati e gli archi che li toccano (come il
  perimetro chiuso della conferma). Nodi e archi della partenza hanno origine `registrato`: l'origine `conferma`
  arrivera' con la partenza `conferma` di B2, confrontata con l'ultima conferma. Il `dopo` lo scrive il server:
  partenza, impronta, nodi, archi.
- **Fonte**: `sorgente_scelta` (da nessuna fonte, o da una senza allegato), `sorgente_cambiata` (da una fonte con
  allegato, **motivo obbligatorio**: lo vuole anche il trigger differito della 0022), `sorgente_vista` e
  `sorgente_rifiutata` (informative, con motivo il rifiuto; `chiave_bozza` facoltativa = il nodo di destinazione
  di una fonte secondaria, anche solo nella bozza: E2.2), `rebase` (`prima = {base, sorgente}`, `dopo = {base,
  sorgente, rif_scaduti}`, motivo; uno per salvataggio, mai nella prima versione; con il campo `rebase` della
  richiesta uguale).
- **Nodi**: `nodo_aggiunto`, `nodo_tolto` (il nodo intero; mai la radice; senza archi ne' preassociazioni),
  `nodo_modificato` (gli stessi campi fra codice, rev, descrizione, tipo, op, caselle, righe_proposta; un codice che
  cambia segue la regola dei codici nuovi), `nodo_riusato` (`{op, componente_id[, codice]}`, dopo `op = riusa`).
- **Archi**: `arco_aggiunto` (estremi gia' presenti), `arco_tolto`, `arco_spostato` (stesso figlio, padre diverso,
  qta anche diversa, origine e posizione uguali), `qta_modificata` (`{padre, figlio, qta}`). La posizione si cambia
  togliendo e rimettendo l'arco.
- **Decisioni**: `decisione_aggiunta`, `decisione_tolta` (per cambiarne una: si toglie e si aggiunge). Una
  dichiarazione `mono_pezzo` motivata si salva anche se non e' applicabile (E2.12): la giudicano R4 e R5.
- **Documenti**: `file_preassociato` (a un nodo presente e non escluso), `file_svincolato`.
- **Storia**: `annulla` e `rifai` con `dopo = {n}`: il server applica l'inverso dell'evento n (o lo rifa'), della
  stessa versione o di una precedente; solo per le azioni su nodi, archi, decisioni e documenti.
- Origini che il browser puo' dichiarare: `step`, `utente`. `conferma` e `server` le scrive solo il server.
- Il giornale prova la **coerenza** con gli snapshot, non la provenienza: l'origine di un evento e' quella dichiarata
  dalla pagina, e una coppia aggiungi + togli dello stesso nodo e' coerente (RC-11). L'origine `step` di un arco la
  ricalcola il server in R4/R5 (B2/B3), mai copiata dalla pagina nella conferma.
- Una chiave tolta che torna per un nodo diverso: oggi R2 la accetta (la regola che la vieta e' di B2). Se un evento
  della fonte STEP secondaria la nominava, la conferma che la certifica si rifiuta al COMMIT (`P4X07` della 0022):
  la destinazione temporanea non si risolve sul nodo sbagliato.
- Un'azione nuova o con un significato nuovo vuole il vocabolario 2: gli eventi gia' scritti restano con la loro
  versione, e `VerificaGiornale` rifiuta un evento di un vocabolario che non conosce.

## La forma canonica (contratto rev. 3 §6.3)

- `jsoncanonico`: chiavi ordinate, niente spazi, numeri **interi senza esponente** (nelle decisioni `1e2`, `-0`,
  `1.5` si rifiutano: jsonb li riscriverebbe e l'impronta non tornerebbe, RC-01; al piu' `MaxCifre` = 1000 cifre,
  oltre CORPO_NON_VALIDO e non l'errore 22003 del database, RSC-10), stringhe UTF-8 in **NFC**.
- Elenchi ordinati: nodi per chiave, archi per padre e figlio, decisioni per id, preassociazioni per proposta,
  caselle per gruppo e rif, righe in ordine lessicografico.
- I campi facoltativi di nodi e archi vuoti, assenti o `null` sono la stessa cosa e **non si scrivono**
  (`componente_id`, `codice`, `rev`, `descrizione`, `caselle`, `righe_proposta`, `posizione`); i quattro elenchi di
  testa si scrivono sempre [T].

## Dati

- Scrive: `distinta_bozza`, `distinta_bozza_versione`, `distinta_bozza_evento` (solo con le query di
  `platform/db/queries/p4_conferme.sql`); il registro `distinta_bozza_registro` lo scrive il trigger della 0022.
- Legge: `thread_offerta` (lucchetto ed esistenza), `componente` (il prodotto e il perimetro registrato),
  `componente_relazione` (gli archi della working), `allegato` + `messaggio` (le fonti), `conferma_strutturale`
  (le basi), `working_contatore` (il contatore del rebase).
- Le date le scrive il database (trigger della 0022): il Go non le passa.
- I vincoli che tiene il database e che il Go dice prima con il suo codice: una bozza aperta per prodotto
  (`BOZZA_VERSIONE` / `altra_bozza_aperta`), versione + 1 alla volta (`BOZZA_VERSIONE` / `versione_diversa`), eventi
  solo nella transazione della loro versione, cambio di fonte con un evento motivato, bozza chiusa immutabile
  (`bozza_chiusa`), confermata mai cancellata.

## Invarianti

- Lo snapshot salvato e' canonico e la sua impronta e' quella registrata: ogni rilettura lo ricontrolla
  (`rileggiSnapshot`), e una differenza e' `ErrSnapshotIncoerente`, non un dato.
- Ogni versione e' il risultato delle sue azioni applicate alla versione prima, o allo stato di partenza ricostruito
  (`VerificaGiornale`, `ControllaPartenza`).
- Ogni errore delle letture di controllo che non sia «non trovato» torna com'e' (5xx in B2), mai `FUORI_RFQ` (RC-10).
- Nessuna scrittura fuori dalle tre tabelle della bozza.
- Lucchetti nell'ordine del piano P4-B0 (F22): prima la RFQ (`BloccaThread`), poi la testata.

## Dipendenze

- Importa: `core/inbox/classificazione` (solo `CodiceAmmissibile`, `RevAmmissibile`, `MaxCodice`, `MaxRev`),
  `core/rfq/distintabozza/importazione` (V1), `core/rfq/confermabom` (V1, ondata B: lo stato della RFQ, la versione
  corrente e la validita' delle conferme), `platform/contratti/worker` (la struttura dei fatti per i rif scaduti),
  `platform/db`, `platform/jsoncanonico`, pgx (`pgx`, `pgconn`, `pgtype`), `google/uuid`, `golang.org/x/text/unicode/norm`
  (NFC).
- Le prove del formato 3 importano anche `core/registro/regole/grammatica` (T-14: il validatore delle grammatiche).
- Mai `transport`, `app`, `ai`; mai `core/rfq/fascicolo` nel codice di prodotto (le prove lo importano: i limiti
  `MaxVociBozza` e `MaxQtaArco` e `RimuoviComponente`).
- Non e' nella lista chiusa del Motore A (`core/estrazione/evidenze/dipendenze_test.go`, `pacchettiMotoreA`): ci
  entrera' con B2, quando `transport/web/motore_a` lo importera'.

## Test

- **L1** (`go test ./internal/core/rfq/distintabozza/`):
  - `snapshot_test.go`: limiti; ogni violazione di forma con il suo campo (anche maiuscole, U+0000, numeri non
    interi, caselle senza sorgente, origine conferma con base 0, codici nuovi e registrati); limiti al bordo; forma
    canonica (ordine, campi vuoti omessi, NFC, interi); un nodo nuovo senza codice
  - `azioni_test.go`: il vocabolario del contratto; la scena dei 35 nodi passo per passo con la partenza registrata e
    il giornale rifatto in memoria; eventi inventati e cambiamenti taciuti; la prima versione contro la partenza;
    annulla e rifai verificati; stati intermedi possibili; fonti e basi di passaggio; le altre azioni, la
    destinazione di una fonte secondaria, il mono_pezzo non applicabile
  - `guasti_test.go`: un guasto del database non e' `FUORI_RFQ`; i rifiuti di Salva prima della transazione;
    l'impronta ricontrollata; i codici e i motivi del contratto
  - V1 (passo 2): `formato3_test.go` (lettura e rilettura, le regole di forma con il campo e il motivo, N-01 nella
    parte L1, i registri citati, T-14 contro il validatore delle grammatiche); `decisioni3_test.go` (le 16 forme
    chiuse, assente = null = vuoto, l'NFC solo sui motivi); `lettori_test.go` (formati 1, 2 e 3, lo storico che non
    nasconde, il registro e le sue righe); `conservazione_test.go` (i casi C-01…C-34 della tabella di conservazione,
    eseguiti sulla forma canonica, e T-01, che con `COCKPIT_EM_V1` li confronta con la tabella dell'emendamento);
    gli aiuti in `formato3_aiuti_test.go`
- **L4** (`-tags integrazione`, `COCKPIT_TEST_DSN` su `cockpit_test`, `-p 1`): `persistenza_db_test.go`
  - le query di `p4_conferme.sql` che usa la bozza, una per una, con la rilettura e il perimetro della RFQ
  - **la bozza dei 35 nodi** (partenza dal registrato, fonte e caselle, tre esclusioni, due aggiunte di cui una senza
    codice, due ricodifiche, spostamenti, annulla e rifai anche fra versioni): sei salvataggi, foto del database,
    pool nuovo («riavvio»), snapshot riletto byte per byte, giornale a pagine, storia rifatta dal database
  - conflitti e rifiuti: seconda bozza aperta, versione vecchia, salvataggi simultanei, azioni che non tornano, fonti
    sbagliate o di un'altra RFQ (anche di passaggio), cambio di fonte senza motivo, prodotto fuori, RFQ inesistente,
    base di un'altra conferma, abbandono
  - prima versione contro il database; nodo senza codice e mono_pezzo non applicabile; numeri, pesi jsonb e U+0000;
    R1 e il perimetro; il rebase e `BASE_SUPERATA`
  - la conservazione: una bozza abbandonata da oltre 90 giorni si cancella con versioni ed eventi, il registro resta
    e il prodotto non si rimuove; una confermata non si cancella mai
  - V1 (passo 2): `conservazione_db_test.go`, `TestV1ConservazioneFormato3`: il registro e la versione 1 di una bozza
    formato 3 scritti con le query della 0023 e riletti; i casi C-01…C-34 sul registro e sullo snapshot riletti dal
    database, e N-01 nel testo di jsonb (codice e letterale con l'accento combinante, descrizione in NFC). Conferma e
    storico arrivano con il passo 6
  - V1, ondata B: `ondatab_db_test.go` (R2 forma salvataggio: giornale, chiave, reinvio, `CHIAVE_RIUSATA` anche da
    un'altra RFQ, `BOZZA_VERSIONE`, `BOZZA_ALTRO_PRODOTTO`, annulla e rifai sul database (PE-05), lo svincolo per nodo
    escluso e il vassoio (PE-22), `FUORI_RFQ` e `FILE_NON_ATTIVO`, identita' proposta, registrata ed emittente, il
    segnale visto (PE-17), lo svincolo che cita l'esclusione (PE-27); R1 con registro, rif scaduti e perimetro; R6;
    R7 nei tre formati (U-06, PE-07); la conversione (U-05) e i nodi `formato_2` in R2 (U-05b, parte senza R4))
  - V1, ondata B: `giornale3_db_test.go` (T-10: il giornale delle versioni della pagina e della conversione rifatto, un
    evento cambiato a mano che si vede; R1 su una bozza formato 2 da convertire)
  - V1, ondata B, secondo passaggio: `ondatab_revisione_db_test.go` (le prove dei rilievi della revisione: il giornale
    di un `nodo_modificato`, gli archi step, la `non_includere` senza importazione, il modo dei nodi, la chiave di R1, i
    riferimenti delle azioni, R-19, R-04, R-16, R-22, le fonti secondarie e i rifiuti della conversione, la traccia oltre
    il limite)
- **L1 dell'ondata B**: `ondatab_dto_test.go` (PE-27, PE-11, PE-17, PE-22, PE-05), `riconcilia3_test.go` (il vocabolario
  2 sullo snapshot, la pila d'annullamento, il rebase, «Non includere» nell'ordine della pagina, T-16 con
  `nodo_escluso`; dal secondo passaggio gli stessi campi di `nodo_modificato`, la chiave delle decisioni, i riferimenti
  delle azioni), `richieste3_test.go` (le forme di R2 e il corpo formato 2), `casi_condivisi_test.go` (la meta' del
  server di T-16 sui casi condivisi della pagina, **in parte**: il vassoio con la funzione di R1, le azioni «applica» con
  la sola applicazione strutturale, finche' i casi del file non hanno le forme di EM §9.2 (RIC-I1, RIC-I2, PD-08)),
  `ondatab_revisione_test.go` (il modo atteso dalle regole P-2...P-6)

## Stato dell'implementazione

- Fatto (B1 + correzioni del 13/10): persistenza, riconciliazione con il vocabolario della rev. 3, partenze `vuota` e
  `registrato`, letture, abbandono, le prove L1 e L4.
- V1, passo 2 (18/10): il DTO del formato 3 (snapshot, 16 decisioni, registro d'importazione nel pacchetto
  `importazione`), i lettori dei formati 1-3, la canonicalizzazione con la regola dell'NFC, i validatori di forma e i
  limiti. `Salva`, `Leggi` e le altre funzioni qui sopra restano del formato 2: R1-R7 nel formato 3 sono il passo 6.
- V1, ondata B (18/10): R1, R2 (forma salvataggio), R3, R6, R7 nel formato 3, il vocabolario 2, la conversione
  (servizio), R11 nel transport. Con l'ondata C: le forme `crea`, `cambia_sorgente`, `riallinea_proposta`, `converti_da`
  di R2, R4, R5 e la conferma formato 3 (vogliono l'adattatore del Motore A).
- **NON ANCORA IMPLEMENTATO** (formato 2, storico di B1): le rotte R1-R3 (B2); le partenze `conferma` e `step` (B2); il controllo dei
  `componente_id` e delle proposte della bozza contro la RFQ e del codice registrato (B2); la firma del perimetro
  ricalcolata e i `rif_scaduti` controllati nel rebase (B2); la forma completa delle decisioni (B2);
  `fonte_step_evento` (B2/B4); `versione_corrente`, `base`, `rif_scaduti` in R1 (B2).
- Scelte [T] da portare nel contratto rev. 3 (elenco completo nel resoconto delle correzioni): la riconciliazione per
  rifacimento con le azioni che portano il nodo o l'arco interi; lo stato di partenza disfacendo le azioni e il
  confronto per identita'; `nodo_riusato` con `prima`; `sorgente_scelta` contro `sorgente_cambiata`; il rebase con
  `base = {conferma_id, numero, contatore, firma}`; «eventi» numero e l'elenco a pagine; i quattro elenchi di testa
  sempre scritti; `RFQ_INESISTENTE`.

## Dove intervenire

- **una regola di forma nuova** → `snapshot.go:valida` / `validaNodo` / `validaArco`, e un caso in
  `snapshot_test.go`
- **un'azione nuova** → `azioni.go` (costante, `AzioniDelVocabolario`, `applica`, `inversa` se si annulla),
  `Vocabolario` + 1 se cambia il significato di una esistente, e un caso in `azioni_test.go`
- **una partenza nuova** → `persistenza.go:partenzaDalDatabase` e `partenza.go:ControllaPartenza`
- **un codice d'errore** → `errori.go`, e la tabella HTTP di B2

## Leggi anche

- `migrations/README.md` (la 0022), `internal/platform/db/README.md` (`p4_conferme.sql`)
- `internal/README.md`, `internal/core/README.md`
