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
- **R5-R11** (conferma, intento, conferme, piano e lotto del fascicolo, correzioni, F8): B2-B4. R4 formato 3 e' qui
  (`riepilogo3*.go`, direttiva 39); la sua rotta sta in `transport/web/motore_a_bozza_v1_r4.go`.
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
- **`evidenze3.go`** (ondata C, U2 §6.4.2, §7.7)
  - `EvidenzaDocumentale` `{allegato_id, sha256, pagine?, riga?, nota?}`: nella `correzione` (`evidenza_documentale`), nei
    nodi `manuale` (`evidenze_identita[]`), negli archi `utente` (`evidenze_arco[]`); pagine in ordine e senza doppioni,
    nota gia' in NFC (mai normalizzata dal server), al piu' 20 per nodo o arco; lo sha lo manda il browser e lo verifica
    il salvataggio (`sha_diverso`, `FILE_NON_ATTIVO`, `FUORI_RFQ`)
  - `conferma_categoria`, il diciassettesimo tipo: categoria nel vocabolario chiuso (`categoria_non_ammessa`), mai sul
    finito (`categoria_sul_finito`), una decisione viva per nodo e categoria, `ingresso` editor | wizard, `da_conferma`
    per le decisioni riportate; con `classe_grammatica` la classe e' verificata contro il registro (V-1)
  - nel vocabolario 2: `nodo_modificato` tocca anche `evidenze_identita` e `tipo_confermato`; verbo nuovo `arco_modificato`
    (le sole `evidenze_arco` di un arco `utente`); `arco_spostato` senza `posizione` (DEC-B PD-09 (c)); una chiave tolta
    non torna per un nodo diverso (DEC-B PB-34, `ChiaveRiusabile`); una sola `cambia_tipo` per componente (PE-B06); su un
    nodo riusa/modifica il codice cambia solo con una ricodifica (U2-P35)
- **`ingressi.go`** (ondata C, U2 §6.3.3): `LeggiIngressi`, R1 `?ingressi=1`, e le quattro parti di
  `impronta-ingressi-1` (`ParteBomDi`, `ParteBozzaDi`, `PartePropostaDi`, `ParteAnalisiDi`, una istruzione SQL ciascuna,
  `queries/v1_ingressi.sql`), che R5 riusera' per H_S/H_T. Dalla direttiva 35 (blocco C-1c, INC-G12) l'algoritmo e'
  `impronta-ingressi-2`: la parte proposta esclude le colonne di copia di `documento_proposta` (entra la lettura
  `dettagli.codice_letto`) e le righe che R5 segna (il segno e l'elenco `RigheDiR5`), e comprende le AR degli allegati
  citati dalla bozza del prodotto, viste come prima della transazione corrente; `PartePropostaDi` prende il prodotto e
  l'elenco di R5 (nil per R1). Prova L4 `c1c_ingressi_db_test.go`
- **`bozze_rfq.go`** (direttiva 35, blocco C-4, parte persistenza; U2 §12 punto 6): `LeggiBozzeDellaRfq`, la lettura
  delle bozze di tutti i prodotti della RFQ per S9, sulla transazione di chi legge (REPEATABLE READ READ ONLY, solo
  SELECT). Per ogni bozza aperta la versione e l'impronta, le PB `{allegato_id, chiave_bozza}` con il nodo
  (`NodoDellaBozza`: codice e rev dell'identita', componente di un nodo riusato, la casella dello STEP sorgente se c'e'),
  gli svincoli pendenti (`scollega_documento`, con `AssociazioneAttiva`) e il vassoio «Da ricollocare» (la regola di
  R1, con il nodo d'uscita se c'e' ancora); una bozza formato 2 con `DaConvertire` e senza elenchi; e le AR attive
  della RFQ dal servizio unico (`associazioni.AttiveDellaRfq`). Mai `documento_proposta.componente_id`: le query
  (`queries/v1_lettura_bozze.sql`) non leggono `documento_proposta`. Prove `bozze_rfq_test.go` (L1) e
  `bozze_rfq_db_test.go` (L4)
- **`intento.go`**: dalla direttiva 35 (C-1c) l'evento di associazione con la chiave vale `associa_step` solo se e' del
  gesto di S7 (`assegna`, origine `ponte`, senza riga della AR): `GetAssociazioneEventoPerChiave` ristretta
- **`contenutoatto.go`** (ondata C, U2 §6.2): `ContenutoAtto1`, la regola unica di X-02 (`uguale_alla_base`,
  `differenze_dalla_base[]`, `NESSUNA_MODIFICA`), pura, con `ContenutoDaSnapshot` per le parti che lo snapshot dice da
  solo; le parti del database (archiviazioni, radice autorizzata, criterio, quantita' dei fatti, preassociazioni ed
  effetti di R5) le aggiunge chi chiama. Prove CA-01…CA-07 (L1)
- **`rifiuto.go`** (ondata C, U2 §5.7): R15, `LeggiRichiestaR15` e `RifiutaPropostaAggiornata`: la proposta di adesso
  la calcola chi chiama prima della transazione; sotto `BloccaThread` la chiave d'intento, il prodotto senza bozza, la
  conferma ultima valida formato 3, l'impronta di adesso (409 `ANTEPRIMA_SUPERATA`), una riga di
  `proposta_aggiornata_rifiuto` e l'esito dell'intento, nient'altro
- **`vocabolario2.go`** (ondata B)
  - il vocabolario 2 del giornale del formato 3 (EM §9.2): i verbi del vocabolario 1, `proposta_riallineata` (solo del
    server), `file_lasciato` e, dal giro G39-ter, `riassegnazione_sorgente` (solo del server, `riassegnazione3.go`); la tabella delle forme di prima e dopo per verbo (PE-03, con gli esempi dei casi condivisi
    `transport/web/e2e/distinta_unica_casi.json`); l'elenco chiuso dei motivi di `file_svincolato` in `dopo.motivo`
    (PE-04, PE-22: `nodo_escluso`); il prima di `annulla` e `rifai`, null oppure `{azione_id}` (PE-05)
- **`riassegnazione3.go`** (giro G39-ter; G39-03 [U] prima di R5, messaggio 43 caso 1 [U]; forma [T-39], senza DDL)
  - `ScriviRiassegnazione`: l'evento `riassegnazione_sorgente` (origine server, vocabolario 2) sulla versione 1 della
    bozza nata da S8 al posto di una bozza nata da un'altra sorgente S8 prima di R5: prima `{bozza_id, versione,
    sorgente}` della bozza abbandonata, dopo `{sorgente, revoca_evento}`, il motivo della revoca. Si scrive nella
    transazione della creazione (`Creazione3.PrimaDelCommit`): il trigger `distinta_bozza_evento_nasce` della 0022 vuole la
    bozza aperta alla versione corrente e la transazione che l'ha salvata. Non cambia lo snapshot e non si annulla
    (`AzioniSoloDelServer2`); lo chiama `sorgente` (S8, `sostituzione.go`)
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
  - **`vassoio.go`**: `VassoioDa`, il vassoio «Da ricollocare» dal giornale (EM §3.7, T-16); `AzioniDelVassoio`, le
    sole azioni che la regola guarda (la lettura delle bozze di tutti i prodotti legge solo quelle)
  - **`stato3.go`**: `RifScaduti` (le cinque condizioni di EM §6.4, con i motivi [T] `allegato_fuori_rfq`,
    `fatti_assenti`, `chiave_assente`, `sha_diverso`, `file_non_attivo`), `BaseDellaBozza`, `LeggiAllegatiCitati`
  - **`leggi3.go`**: `LeggiR1` (R1 nel formato effettivo: testata, snapshot, `n_eventi` sempre ed `eventi[]` con
    `?eventi=1` (PE-02), registro della versione e citati, versione corrente, base, rif scaduti, vassoio, `da_rivedere[]`;
    una bozza formato 2 con `conversione: {necessaria, forma: converti_da}`); gli eventi di R1 sono `EventoR1`, con la
    chiave nel campo `chiave` (EM §9.1, §9.2; RIC-P6)
  - **`richieste3.go`**: `LeggiRichiestaR2` (una e una sola forma; direttiva 35, blocco C-3: le quattro forme costruite
    dal server si leggono per intero, con `ComuneR2`, `CreaR2` (partenze, sorgente e impronta della partenza step,
    `abbandona`), `CambiaSorgenteR2`, `RiallineaR2` (le scelte `togli`, `tieni_senza_casella`, `recepisci`, `mantieni`),
    `ConvertiR2`; bozza_id e versione_attesa forma per forma; i motivi obbligatori 422 `MOTIVO_OBBLIGATORIO`),
    `Conversione3Da` (la richiesta di `Converti` dal corpo), `LeggiRichiestaR3`; l'impronta della richiesta sulla forma
    canonica del corpo
  - **`partenza3.go`** (direttiva 35, C-3; EM §9.2, §6.12): i costruttori puri della versione 1 delle partenze senza
    il motore: `PartenzaRegistrata` (vuota: il solo prodotto; registrato: il perimetro della working con i suoi archi) e
    `PartenzaDaConferma` (da-conferma-1: nodi, archi e caselle dell'ultima conferma valida, con le sue chiavi, in modo
    registrato; solo `mono_pezzo` e `conferma_categoria` con `da_conferma`; la sorgente della conferma senza registro);
    `IdentitaRegistrataDi`, la regola unica dell'identita' registrata (la usa anche la verifica di R2); le chiavi del
    server `ChiaveDelComponente` («k-c-<uuid>») e `ChiaveDellaVoce` («k-v-<hex>»); `TracciaPartenza` (descrizioni e
    posizioni oltre il limite, nodi della conferma fuori dalla working: mai troncati in silenzio). Nessuna preassociazione
    nasce in una versione 1
  - **`crea3.go`** (direttiva 35, C-3): `CreaFormato3`, R2 forma crea: la chiave globale prima di tutto (riproposto con
    lo snapshot, `CHIAVE_RIUSATA`), il prodotto, la ripartenza (`abbandona`: la bozza di prima chiusa nella stessa
    transazione, «ripartita … nella bozza <id>», con davanti la causa del server `Creazione3.Chiusura` se c'e' (giro
    G39-ter: S8 che sostituisce una bozza nata da un'altra sorgente S8), il motivo nell'evento `bozza_aperta`) o
    nessun'altra bozza aperta, le
    partenze (registrato con una conferma formato 3 valida: 422 `partenza_non_ammessa`, DEC-B PD-02; conferma senza una
    conferma valida: 422 `partenza_non_ammessa`), la testata, la versione 1 e l'evento `bozza_aperta` del server;
    `RiproponiFormaDelServer`, la ricerca della chiave in sola lettura prima del calcolo del motore; la partenza step con
    il registro calcolato da chi chiama (`PropostaCalcolata`): sha256 dichiarato, V-7, `FONTE_NON_CONFERMATA` (la
    ripartenza dalla stessa sorgente passa [T]), `MOTIVO_OBBLIGATORIO`, le occorrenze, il registro, gli eventi
    `bozza_aperta` e `sorgente_scelta` del server e `fonte_step_evento` scelta. Direttiva 39 (S8): la partenza
    `solo_prodotto` (`preparaSoloProdotto`; solo da S8, la rotta di R2 la rifiuta con `partenza_non_ammessa`): il solo
    prodotto con la sorgente STEP reale e il gruppo riservato `solo_prodotto`, senza registro, algoritmo `solo-prodotto-1`,
    fonte confermata e motivo obbligatori; `PrimaDelCommit` (anche in `CambioFonte3`): S8 vi scrive il suo intento nella
    transazione dell'ultimo passo; le chiavi del server possono essere UUID v5 (le chiavi dei passi di S8). Direttiva 39
    §1, §3: la partenza `vuota` resta senza sorgente, registro e `fonte_step_evento`; si rifiuta (422
    `partenza_non_ammessa`, R39-19) solo con una conferma formato 3 valida (la strada e' da-conferma-1) o con uno STEP
    confermato da S8 e ancora valido (la strada e' S8)
  - **`soloprodotto3.go`** (direttiva 39 §3): `SoloProdottoSuBozzaFormato3`, la forma del server che S8 usa quando arriva
    uno STEP senza struttura importabile su una bozza manuale aperta senza sorgente: la versione n+1 della stessa bozza con
    la sorgente reale e il gruppo riservato `solo_prodotto`, nient'altro cambiato, niente registro; `sorgente_cambiata`
    con `prima` null e la presa d'atto dell'analisi, `fonte_step_evento` scelta `solo_prodotto`. La rotta di R2 non la
    espone
  - **`partenzastep3.go`** (direttiva 35, C-3; EM §6.1): `VersioneDaRegistro`, la versione 1 della partenza step
    (importazione-1, P-1…P-8): la radice al nodo del prodotto, il riuso per `decisione_esistente` (salvo il
    raggruppamento, sottoassieme), le voci proposte e da decidere con la casella, il tipo proposto o la regola di sempre,
    gli archi con la quantita' dei fatti (422 `arco_senza_quantita`), i confermati senza STEP del gruppo bom (P-6, dalla
    working); nessuna decisione (P-7); `FuoriDelRegistro` (P-8). Le PB iniziali le aggiunge la presa in carico
    (`presainc3.go`, direttiva 39 §4), non `VersioneDaRegistro`
  - **`importa3.go`** (direttiva 35, C-3; EM §6.1-§6.3): le parti comuni delle forme che importano. Prima della
    transazione, per chi calcola il motore: `GruppoDaImportare` (il gruppo dichiarato fra quelli del prodotto:
    `gruppo_su_altra_occorrenza` con l'allegato, `stessa_sorgente`, il 409 `ANTEPRIMA_SUPERATA` con il gruppo ritrovato
    per impronta, `gruppo_non_importabile`), `VerificaImprontaDichiarata` (V-7), `ErroreDellAdattatore`; sotto il
    lucchetto: le occorrenze del registro (409), la fonte confermata (`FONTE_NON_CONFERMATA`, il documento cad_3d del
    prodotto non sostituito con lo sha256 della fonte), il motivo della scelta della fonte
    (`propostabom.MotivoFonteObbligatorio`), il registro trovato per impronta o scritto. `PropostaCalcolata`,
    `ImportazioneR2` (la testata del registro e gli elenchi della risposta), `GruppoRitrovato`
  - **`rifonte3.go`** (direttiva 35, C-3): gli attrezzi comuni a riancoraggio-1 e cambio-fonte-1: la scelta `recepisci` /
    `mantieni` per un nodo proposta con l'identita' cambiata (mantieni scrive la decisione identita alternativa sulla
    lettura di prima, con il registro vecchio, autore e motivo del gesto; un da_decidere prende da solo l'identita' nuova
    [T]); gli eventi del server (`nodo_tolto`, `arco_tolto`, `file_svincolato` con il motivo, `decisione_tolta`); i
    da_rivedere; le scelte che mancano (`riallineamento_incompleto`) o che non servono (422)
  - **`riallinea3.go`** (direttiva 35, C-3; EM §6.5): `Riancora` (riancoraggio-1, puro) e `RiallineaFormato3`, R2 forma
    riallinea_proposta: stessa sorgente, registro nuovo del gruppo corrente (`GruppoDelRiallineamento`, `SorgenteDellaBozza`
    per chi calcola il motore), caselle e archi sulla chiave nuova, le scelte, i nuovi nodi proposti, le differenze
    (`propostabom.Confronta`); la versione n+1 con il registro, gli eventi del server e per ultimo `proposta_riallineata`
    (la pila d'annullamento si ferma li'); `VersioneDelServer`, la risposta delle forme che riscrivono una bozza aperta,
    con gli elenchi in `importazione`; il reinvio dai dati salvati
  - **`cambiafonte3.go`** (direttiva 35, C-3; EM §6.6): `Cambia` (cambio-fonte-1, puro) e `CambiaSorgenteFormato3`, R2
    forma cambia_sorgente: gli elenchi sono quelli di `propostabom.EffettoDelCambioFonte` (la stessa funzione
    dell'anteprima di S6), e la versione n+1 li applica: il nodo del prodotto con la casella della radice nuova, i nodi
    della stessa voce con la casella sull'occorrenza nuova (fra due copie `recepisci` / `mantieni` per un nodo proposta
    con l'identita' cambiata, base B di V1-Q30), gli altri nodi della fonte vecchia fuori con le loro PB nel vassoio
    (`fonte_cambiata`, nessun ricollegamento per codice), le voci nuove che entrano come nella partenza step (P-6 per il
    gruppo bom), le decisioni che cadono; `FONTE_NON_CONFERMATA`; gli eventi del server e per ultimo `sorgente_cambiata`;
    `fonte_step_evento` sostituzione (scelta, se la bozza non aveva una fonte STEP); `VistaDellaBozza`. Direttiva 39 §3:
    da una versione senza struttura (nessuna sorgente, o `solo_prodotto`: `SenzaStruttura`) vale **cambio-fonte-2**
  - **`riconciliazione3.go`** (direttiva 39 §3 [U + T-39]; EM App. I.7): cambio-fonte-2 = cambio-fonte-1 con la
    riconciliazione della bozza manuale (`RiconciliaEffetto`, pura, utilizzabile dall'anteprima di S6 dopo
    `propostabom.EffettoDelCambioFonte`): 3-bis, il nodo senza casella (registrato, manuale o deciso) si abbina alla voce
    solo con l'identita' sostenuta (registrato: `decisione_esistente` o codice, rev ed emittente uguali; manuale: codice
    byte per byte, rev, emittente, voce senza `decisione_esistente` verso un altro componente, senza alternative ne'
    `motivo_lettura`, nodo senza `evidenze_identita`) e univoca nei due versi: tiene chiave, PB, decisioni, tipo, origine e
    acquisisce la casella (`caselle_riscritte[]` con `allegato_prima` null; il manuale passa al modo proposta con la stessa
    identita', in `identita_aggiornate[]`); 3-ter, i casi non certi in `abbinamenti_proposti[]` (`ambiguo`,
    `incompatibile`, `evidenze_discordanti`), fuori da `da_rivedere[]`, con le voci trattenute; gli archi (P-5 e l'arco
    verso un abbinato senza padre; il resto in `differenze.archi[]`: aggiunto, cambiato quantita, tolto), il tipo diverso
    in `differenze.nodi[]`, niente P-6; le decisioni che citano un abbinato e `mono_pezzo` con figli in `da_rivedere[]`
    con `fonte_cambiata`, mai tolte. La regola dello «stesso nodo» (DEC-B PB-34) estesa: il nodo abbinato resta lo stesso
    nodo (stessa chiave) e da li' «lo stesso nodo» e' la casella acquisita
  - **la revisione con un secondo STEP** (`preparaRevisione`, `PreparaRevisione`, `dopoLaRevisione` in
    `riconciliazione3.go`; rilievo bloccante della revisione del 20/10) [T-39]: in una versione con la base confermata
    (numero almeno 1) che passa a un'altra sorgente STEP, i nodi registrati (non il prodotto) le cui caselle il registro
    nuovo non ha per rif (il rif contiene lo sha256: fra due STEP diversi non coincide mai) non escono piu' con
    cambio-fonte-1: si riconciliano per identita' come i nodi senza casella di cambio-fonte-2 (3-bis/3-ter, niente P-6).
    Abbinato: stessa chiave, PB e decisioni, casella di B, origine di prima, archi step di nuovo su B se il registro li
    ha; non certo: nodo senza casella (origine conferma) e voce trattenuta in `abbinamenti_proposti[]`; senza candidati:
    esce come prima. Evento `sorgente_cambiata` con `algoritmo: cambio-fonte-2` e
    `conteggi.registrati_della_fonte_precedente`. Prove: L1 `c39_revisione_test.go`; L4 in
    `sorgente/s8_revisione_db_test.go` (S8 → R4 → R5 → S8 di B → R4 → R5, con i guasti iniettati)
  - **lo STEP del finito fuori dalle PB iniziali** (`pbiniziali3.go`; direttiva 39 §5 [U], rilievo della revisione del
    20/10) [T-39]: uno STEP candidato come sorgente di un finito (proposto dal registro sul nodo del prodotto, o detto
    `cad_3d` di un finito da una proposta di documento: `ListStepCandidatiDelFinito`) non diventa una PB «motore» ne' un
    suggerimento (alla nascita, nei comandi espliciti, in R1): segue solo S8. Lo STEP di un particolare resta una PB come
    un PDF. Prove: L1 `c39_pbstep_test.go`; L4 `sorgente/s8_step_del_finito_db_test.go`
  - **G39-02 [U] (20/10 sera), lo STEP del finito e' un candidato sorgente** (`stepcandidati3.go`): si riconosce con il
    Motore A (`StepCandidatiDalMotore`, puro, con la fotografia dello stesso calcolo, che prende le coppie di tutte le
    candidature di `propostabom.CandidatureSorgenteDelFinito`, univoche o no, e in `Univoche` / `UnivocaDel` quelle
    univoche, le sole che S9 propone e che l'esenzione dal motivo di S8 guarda (`sorgente.candidatoDelMotore`), con le loro
    evidenze in `Evidenze` / `EvidenzeUnivocaDel` (la traccia che S8 scrive nel motivo quando corregge il codice: giro
    G39-bis, integrazione, [T-39 da ratificare]): la regola e' scritta una volta sola,
    nel motore; giro G39-bis: senza un'evidenza discordante, cioe' il nome del file che e' o comincia con il codice di un
    altro componente che non e' un finito o un candidato di un componente che non e' un finito nell'ancoraggio, D-2 (c)
    [U], e con la rete del nome uguale al codice del finito; candidato a livello prodotto dell'ancoraggio senza dimensioni discordanti,
    candidato della fonte con la radice compatibile o trovato dal motore o documento STEP del prodotto, struttura
    candidata con il prodotto come radice del file); `StepCandidatiDelFinito` e' la funzione unica «questo allegato e' uno
    STEP candidato del finito» (con il motore se c'e', altrimenti il ripiego [T-39] `ListStepCandidatiDelFinito`, la
    lettura legacy con il confronto del marcatore). Usata da: PB iniziali e suggerimenti (`FattiDellaRfqCon`; il calcolo
    arriva con `PropostaCalcolata.StepCandidati`; ogni candidatura, univoca o no), R2 (`preassociazioni3.go`: 422
    `STEP_CANDIDATO_DEL_FINITO`, anche in una bozza manuale o «solo prodotto»), R4/R5 (`riepilogo3_lettura.go`,
    `stepCandidatiNellePB`: conflitto `step_candidato_del_finito`, blocco `PREASSOCIAZIONE_NON_APPLICABILE`; il calcolo
    con `IngressoMotoreR4.StepCandidati`). R2 e R4 con `StepCandidati.BloccaPB` e `NodoVersoUnFinito` (giro G39-ter,
    G39-06 [U]: quando il motore non sa decidere sceglie l'utente; lettura [T] dell'orchestratore): esclusa la PB su una
    candidatura univoca (o del ripiego, che non distingue le univoche [T-39]) e la PB verso un prodotto finito (il nodo
    del prodotto della bozza, un nodo di tipo finito o il nodo di un componente finito della RFQ: li' la strada e' S8);
    la PB su una candidatura non univoca verso una parte o un sottoassieme si salva e non e' in conflitto. Prove: L1
    `c39_g39ter_bloccapb_test.go`; L4 `transport/web/motore_a_bozza_v1_g39ter_db_test.go`;
    R1 (`step_candidati[]` con `?suggerimenti_motore=1`, per l'avviso della pagina). Prove: L1
    `c39_g39_stepcandidati_test.go`; L4 `c39_g39_step_db_test.go` (gira anche sul codice di prima, dove fallisce),
    `c39_g39_motore_db_test.go`, `transport/web/motore_a_bozza_v1_g39_db_test.go` (il motore vero, regole ACME),
    `transport/web/motore_a_bozza_v1_g39_d2c_db_test.go` (D-2 (c) [U] con il motore vero: gli STEP di un sottoassieme e di
    una parte con il loro codice restano PB, R2 200 e nessun motivo `step_candidato_del_finito` in R4). La nascita
    da-conferma-1 riceve gli STEP candidati del motore dall'adattatore (`Creazione3.Proposta.StepCandidati`), come la
    partenza step
  - **G39-01 [U] (20/10 sera), i file arrivati dopo l'apertura** (`adesso3.go`): il calcolo del motore di adesso
    (`CalcoloAdesso`: il registro del gruppo corrente della sorgente importato adesso, non salvato, e gli STEP candidati;
    `CalcoloAdessoDa` lo compone dall'ingresso del motore). R1 con `OpzioniR1.Adesso` (`?suggerimenti_motore=1`)
    aggiunge i suggerimenti del calcolo di adesso per i file che il registro della versione non propone in modo univoco
    (`suggerimentiDelCalcolo`, la regola di `ClassificaProposte`: le applicabili `dopo_apertura` con `importazione` =
    l'impronta del registro di adesso); R2 con `Salvataggio3.Adesso` (`MotoreDelSalvataggio` dice all'adattatore quando
    serve) riconosce quell'impronta: «motore» sul nodo proposto (proposta univoca), «corretta» su un altro nodo, e scrive
    il registro in `distinta_importazione` nella stessa transazione (`registroDellaVersione`). La proposta ambigua non e'
    mai un suggerimento e, dichiarata, e' 422 `proposta_non_corrispondente`. Un file arrivato dopo R5 va nella revisione
    successiva (la bozza confermata e' chiusa). Se il calcolo cambia fra R1 e R2, vale il registro di adesso solo se
    propone il file, univoco, sul nodo della PB («motore»); se lo propone su un altro nodo il suggerimento letto e'
    superato: 422 `proposta_non_corrispondente`, non «corretta» (`registroDiAdessoPer`). R4 con il motore
    (`OpzioniR4.Motore`) usa lo stesso registro di adesso in `provenienzeR4`: una PB accettata nel corpo e non ancora
    salvata si vede come la salverebbe R2. Prove: L4 `sorgente/g39_dopo_apertura_db_test.go`,
    `sorgente/g39_aiuti_db_test.go` (la PB spostata «corretta», gira anche sul codice di prima, dove fallisce)
  - **`forme3.go`** (direttiva 35, C-3): i codici e i motivi delle forme costruite dal server (`FONTE_NON_CONFERMATA`,
    `PROPOSTA_NON_DISPONIBILE`, `IMPORTAZIONE_TROPPO_GRANDE`; `gruppo_non_importabile`, `gruppo_su_altra_occorrenza`,
    `stessa_sorgente`, `partenza_non_ammessa`, `riallineamento_incompleto`) e i nomi degli algoritmi
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
  - **`storico.go`**: `LeggiStorico` (R7 nei formati 1, 2 e 3, con `confermato_da_nome` (PE-07), le attestazioni, la
    storia della fonte e il dettaglio con i registri); ondata C (U2 §5.4): `verifica_calcolata` esce, e una conferma
    formato 3 porta i `prerequisiti` registrati (criterio, regola `prima-istanza-1`, fonte, firma, `mono_pezzo`,
    gerarchia e deroga da `origine`, i campi che mancano in `mancanti`), mai un si' o un no; R6 conosce anche i gesti
    `conferma_sorgente` e `rifiuta_proposta`
  - **`conversione.go`**: `Converti` (conversione-1, EM §6.7, U-05): il servizio della forma `converti_da`, che R2
    chiama dalla direttiva 35, blocco C-3; gli archi dello STEP che nel formato 3 non uniscono due caselle restano dell'ingegnere,
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
- **Le preassociazioni per `allegato_id`** (direttiva 35 §1.1 [U], DIR35-PB; blocco C-2; EM §3.6.2, §3.7):
  - il tipo e' sdoppiato (PD35 INC-G1): `Preassociazione3 {allegato_id, chiave_bozza}` nel formato 3,
    `Preassociazione2 {proposta_id, chiave_bozza}` nel formato 2 e nel vocabolario 1, che si rileggono come prima
    (`azioni.go` resta invariato nel comportamento: deviazione [T] dichiarata dall'elenco della direttiva §8 C-2);
    l'unica traduzione e' nella conversione (`traduciPreassociazioni`, 1:1 per `documento_proposta.allegato_id` UNIQUE;
    422 `conversione_proposta_non_risolta`; l'occorrenza non attiva o non preassociabile si copia e si segnala in
    `preassociazioni_segnalate`, il conteggio in `preassociazioni_tradotte`);
  - nello snapshot formato 3 le PB sono in ordine di allegato; lo stesso allegato non e' preassociato due volte
    (provvisorio, base [T] D-ASS-04 A); `scollega_documento` e' `{allegato_id, componente_id}` (provvisorio, base [T]
    D-4 A); `file_preassociato`, `file_svincolato` e `file_lasciato` del vocabolario 2 citano `allegato_id`, emendati sul
    posto (provvisorio, base [T] D-5); il vassoio e' per allegato (`VoceVassoio {allegato_id, motivo, chiave_bozza}`);
  - **`preassociazioni3.go`**: le verifiche di R2 sulle PB e sugli svincoli nuovi o cambiati, sull'occorrenza e senza
    bisogno di `documento_proposta` (Prova 2): `FUORI_RFQ`; 422 `CORPO_NON_VALIDO` `natura_non_preassociabile` (file ed
    elementi di Outlook, ne' contenitori ne' archivi: la regola del punto 1 di `trg_fra_nasce`); 422
    `CORPO_NON_VALIDO` `legato_come_sorgente` per lo STEP legato come sorgente strutturale del finito (direttiva 39 §5,
    D-2 (c) [U]: gli altri STEP si preassociano come un PDF; il predicato e' `allegato_sorgente_del_finito` del
    database, la stessa conversione formato 2 → 3 lo segnala `legato_come_sorgente`); la provenienza della PB la decide
    il server (`provenienza3.go`); `FILE_NON_ATTIVO` anche per lo scarto col vecchio «Scarta il file»; lo svincolo cita una AR attiva
    di `file_rfq_associazione` (422 `svincolo_senza_associazione`). Le query sono in `queries/v1_preassociazioni.sql`;
  - `contenutoatto.go`: `VocePreassociazione` e `VoceSvincolo`, le voci del punto 5 con l'occorrenza.
- Direttiva 39 §4 (D-PB01…04 [U], D-AR11 B [U], D-AR14 A [T-39]): PB iniziali e provenienza
  - **`provenienza3.go`**: la provenienza delle PB in R2 salvataggio. `controllaDichiarate` (prima della
    riconciliazione): la pagina rimanda provenienza e registro come li ha letti o li omette; non li mette ne' li cambia
    (422 `provenienza_dal_server`), salvo il registro di un suggerimento accettato. La riconciliazione delle azioni
    guarda solo `{allegato_id, chiave_bozza}` (`senzaProvenienza`, `statoV2.mettiPre`). `provenienzeDecise` (dopo le
    verifiche): rimasta → com'era (manuale se era senza); spostata da motore/corretta → corretta con il registro di prima;
    legacy spostata → manuale; nuova → manuale; nuova o spostata con il registro dichiarato che propone il file su quel
    nodo e solo li' (`PropostaNelRegistro`, univoca) → motore con quel registro; che lo propone, univoco, su un altro nodo
    → corretta con quel registro (G39-01 [U]); un registro che il database non ha e' quello del calcolo di adesso
    (`registroDiAdesso`, G39-01: con un'altra impronta solo se lo propone sul nodo della PB); altrimenti 422 `proposta_non_corrispondente` / `importazione_sconosciuta`. `conProvenienzaNegliEventi`: `file_preassociato.dopo` e `file_svincolato.prima`
    portano provenienza e registro. `validaProvenienza`: la forma (come `ck_dbv_preassociazioni`) nella lettura stretta
  - **`pbiniziali3.go`**: `ClassificaProposte` (pura) e `FattiDellaRfq`: le proposte univoche del registro
    (`associazioni_proposte[]` delle voci) diventano PB motore con il registro d'origine; niente senza la sorgente S8
    valida; le ambigue restano da associare; gli altri casi sono suggerimenti `preassociazioni_proposte[]`
    (`PreassociazioneProposta3`, motivi `pb_toccata`, `ar_attiva`, `conflitto`, `dopo_apertura`), mai `da_rivedere[]`.
    `PBInizialiDelServer`, `eventiPBDelServer` (file_preassociato del server nei comandi espliciti), `ricollocabiliDi`
  - **`conversionep1.go`**: `convertiPreassegnazioni`: alla presa in carico le pre-assegnazioni legacy aperte (P1) dei
    nodi della versione 1 diventano PB `legacy` (da verificare), con `associazioni.ConvertiCopiaLegacy` (evento
    `conversione_legacy`, copia tolta, nessuna AR); le altre restano con il motivo (`legato_come_sorgente`,
    `step_del_finito`, `natura_non_preassociabile`, `file_non_attivo`, `componente_fuori_bozza`). La ripresa dopo
    l'abbandono (rilievo della revisione del 20/10) [T-39]: alla nascita di ogni bozza successiva del prodotto (crea,
    converti_da) le conversioni non consumate (`ListConversioniP1DaRiprendere`: ultimo evento dell'allegato = la
    conversione, nessuna AR attiva ne' copia, nessuna conferma formato 3 del prodotto dopo) tornano PB `legacy` sul nodo
    di `componente_prima`, con le stesse esclusioni, senza scrivere nel giornale delle AR; in `conversioni_legacy[]` con
    `ripresa: true` e `evento_conversione`. L'abbandono non scrive niente. Prove: L4 `c39_conversione_p1_db_test.go`
    (conversione → abbandono → nuova crea, due volte; consumata da una conferma; la conversione dentro converti_da) e
    `c39_classi_p1_db_test.go` (giro G39, G39-04: una occorrenza per classe della vista diagnostica della 0023, C1, C1s,
    D1, D1s, P1, P1s, P2, P3, comprese le sintetiche assenti dai dump; si converte la sola P1; gli invarianti di D-AR04:
    nessuna AR, nessun evento ne' autore fittizio, documenti, provenienze, storia della fonte e altre proposte invariati;
    la ripresa senza scritture nel giornale). Nessuno script di backfill in blocco (G39-04 [T, delegata]).
  - **`presainc3.go`**: `presaInCarico` (crea: conversione P1, poi PB iniziali da step o da-conferma-1) e
    `PresaInCarico.nelDopo` (gli elenchi in `bozza_aperta.dopo`, nella risposta di crea, `sorgente_cambiata` e
    `proposta_riallineata`). Le query sono in `queries/v1_provenienza_pb.sql`
  - nei comandi espliciti (`cambiafonte3.go`, `riallinea3.go`) le PB iniziali entrano dopo la versione n+1 con eventi
    `file_preassociato` del server prima dell'evento finale; R1 (`leggi3.go`) da' `preassociazioni_proposte[]` dal
    registro della versione (le applicabili come `dopo_apertura`); `converti_da` converte le P1 e da' alle PB del
    formato 2 la provenienza `manuale`; `bozze_rfq.go` porta provenienza e registro di ogni PB per S9
  - **`suggerimenti_s9.go`** (aggancio della corsia motore, rilievo della revisione della direttiva 39):
    `SuggerimentiDellaBozzaAperta` da' a S9 gli stessi `preassociazioni_proposte[]` di R1 per la bozza aperta del prodotto
    (la stessa `suggerimentiDopoApertura`, chiave_bozza e `importazione` sempre), nella transazione del caricatore del
    motore: una sola sorgente e una sola forma dei suggerimenti dopo l'apertura [T-39]
- Direttiva 39 §1, §2, §4-§6 (EM §9.4, §7.4, §7.7, §8.1-§8.3, App. H.7, App. I): **R4 formato 3**, il riepilogo senza
  scritture, la funzione condivisa che R5 ricalcolera' nella connessione S
  - **`riepilogo3.go`**: `LeggiRichiestaR4` (i campi dello snapshot piu' `bozza_id?`, `bozza_versione?`,
    `chiave_idempotenza?`; un corpo formato 2 e' 422 `formato_da_convertire`), i codici dei 31 blocchi e degli avvisi, i
    motivi del gate (`senza_sorgente`, `non_confermata`, `revocata`, `rifiutata`, `superata`, `non_coerente`), i gruppi
    D-PB-04, `RiepilogoR4` e le sue sezioni (`Rilievo`, `SorgenteR4`, `SorgenteLegataR4`, `LegameStoricoR4`,
    `EffettoFonteR4`, `IdentitaR4`, `ArcoR4`, `FuoriR4`, `EsclusoR4`, `PreassociazioneR4`, `EffettiDocumentaliR4`,
    `CondivisioneR4`, `VoceRegistroR4`, `CriterioR4`)
  - **`riepilogo3_lettura.go`**: `Riepilogo3` e le letture (solo SELECT, nella transazione di chi chiama): la RFQ e il
    prodotto, lo stato della RFQ, i riferimenti della RFQ (FUORI_RFQ), la versione salvata indicata (provenienza e
    giornale), la provenienza delle PB con la regola di R2 senza i 422 (`provenienzeR4`), le certificazioni, i rif
    scaduti e la base, il registro della versione, la sorgente e il gate (`confermabom.SorgenteValida`, l'occorrenza, gli
    STEP del prodotto per il legame storico, la gerarchia dello STEP sorgente), la BOM congelata, i fatti del Piano
    (`associazioni.LeggiFattiPiano` con il perimetro del prodotto e i prodotti di ogni componente), le bozze degli altri
    prodotti, i documenti sul NAS dei ricodificati, l'effetto di adesso dei cambi di tipo (`fascicolo.EffettoCambioTipo`),
    le righe legacy e i prodotti con lo stesso STEP dei non inclusi, la conferma di base, i file in analisi
  - **`riepilogo3_regole.go`**: `calcolaR4`, la parte pura, e la firma `anteprima-3` (`firmaR4`: tutta la risposta con
    lo snapshot canonico, con gli avvisi e i conflitti del motore, senza `avvisi_motore_errore`, `file_in_analisi`,
    `ingressi` e la firma). I blocchi calcolati: BASE_SUPERATA
    (con i motivi di `BaseDellaBozza`: la sola fonte A della working diversa da B non lo e'), STEP_SORGENTE_NON_CONFERMATA
    (con `mono_pezzo: true` quando c'e' la dichiarazione: MONO_PEZZO_SENZA_STEP nel formato 3 non si somma, R39-20),
    PRODOTTO_PROTETTO, FIGLIO_SENZA_PADRE, D1_ULTIMO_FIGLIO, D1_PARTICOLARE_CON_FIGLI, E37_NON_CONTENITORE,
    CODICE_DA_ASSEGNARE, MONO_PEZZO_CON_FIGLI, ARCO_DUPLICATO_DOPO_RISOLUZIONE, ASSENZA_NON_DECISA (componenti e archi
    della working senza decisione; un arco spostato non e' un'assenza), CICLO (grafo della bozza unito agli archi della
    working fuori dal perimetro, senza gli esclusi), COLLISIONE_EMITTENTI e IDENTITA_INCOMPATIBILE (W1-W4),
    CREA_SU_ESISTENTE, DUE_CREA_STESSO_CODICE, RIUSO_INCOMPATIBILE, RICODIFICA_COLLIDE, RICODIFICA_FILE_NAS,
    TIPO_EFFETTO_CAMBIATO, ARCO_CONDIVISO_QTA, NON_INCLUSO_HA_COMPONENTE, NON_INCLUSO_CON_FIGLI,
    NON_INCLUSO_NON_APPLICABILE, ORIGINE_TROPPO_GRANDE, CONGELATA, PREASSOCIAZIONE_NON_APPLICABILE (dal Piano, con il
    motivo, la variante e i rimedi). Gli avvisi: VERIFICA_DOPO_COMMIT, PRODOTTO_SENZA_FIGLI, ESCLUSO_CON_DOCUMENTI,
    FILE_CONTESO_FRA_PRODOTTI (finche' non c'e' la AR), SORGENTE_NON_AUTORIZZATA (informativo, mai con il gate),
    PRODOTTI_TOCCATI, ESCLUSI_SENZA_MOTIVO, DA_VALUTARE_APERTO, SEGNALATI_INCLUSI, EMITTENTE_CERTIFICATO_SUPERATO. Le
    sezioni: identita[], archi[] (aggiunto, invariato, modificato, spostato con il padre di prima, tolto con la
    decisione), fuori[], esclusi[] (non_includere, escludi, assenza), documenti_preassociati[] con esito, provenienza e
    gruppo, svincoli[], effetti_documentali (i gruppi D-PB-04, le proposte rimosse dal giornale, gli spostamenti di AR,
    le AR che restano: escluso «Mantieni», ricodifica D-ASS-01), sorgente_legata («legato come sorgente», sola lettura,
    fuori dalle PB), legami_storici[] (il solo legame storico univoco di D-AR04 (iii), per un prodotto senza conferme
    di S8: `confermabom.LegameStoricoUnivoco`, la regola di /dati; non soddisfa il gate), effetto_sulla_fonte (promozione e
    sostituzione che R5 farebbe; dal giro G39 `correzione_sorgente` quando la bozza parte da una conferma e la sua
    sorgente valida corregge una sorgente sbagliata, `sorgente.correzione_sorgente`: R5 la scrive in origine della
    conferma, `correzione_sorgente: true` e `revisione: "interna"`, e in `fonte.correzione_sorgente` della risposta; R7
    la rilegge in `correzione_sorgente` e `revisione` di ogni conferma; G39-03 [U], forma [T-39]), condivisione[] (componenti condivisi e stesso STEP, attribuiti a chi conferma; gli
    elementi `documento` dicono i componenti condivisi di cui l'atto cambia le associazioni, svincolo o AR nuova, senza
    PRODOTTI_TOCCATI: dir. 35 Prova 8),
    registro_decisioni[], criterio_previsto (mai mono_pezzo dedotto: caso a zero figli), verifica_prevista
    (`confermabom.PrimaIstanza`, la (b) sullo stato dopo R5, mai «si'» senza i conflitti del motore),
    uguale_alla_base e differenze_dalla_base[] (`ContenutoAtto1`)
  - **Il completamento di R4** (blocco «R4 completo», 20/10):
    - **`riepilogo3_canonico.go`**: CANONICO_COLLIDE e CANONICO_AMBIGUO con le regole del cliente lette nella stessa
      transazione (`fascicolo.MotoreDellaRfq`: i suffissi decorativi), solo fra identita' del cliente; «toccati dalla
      bozza» = da una risoluzione per canonico (un nodo da creare, un riusa_per_canonico); RICODIFICA_COLLIDE anche per
      canonico; un riusa_per_canonico che non torna e' RIUSO_INCOMPATIBILE «canonico»
    - **`riepilogo3_completamento.go`**: gli avvisi del registro della versione (EQUIVALENTE_NON_ADOTTATO,
      CORREZIONE_CARTIGLIO_NON_ADOTTATA, PREASSOCIAZIONE_EMITTENTE_DIVERSO, NON_INCLUSO_SEGNALE_CAMBIATO, con
      `segnale_registro` nei non inclusi); gli effetti dei non inclusi (righe e archi che R5 chiudera', con
      `padre_nella_bozza`; i fabbisogni tolti dal registro); le esclusioni con origine legacy (righe dello STEP sorgente
      scartate da una persona, con la voce `esclusione_legacy`) ed ereditata (le non_includere della conferma di base);
      le condizioni (c) ed (e) di prima-istanza-1 sullo stato dopo R5; le letture nuove in `v1_riepilogo_r4_completo.sql`
    - **`riepilogo3_motore.go`**: gli avvisi e i conflitti del Motore A, calcolato dall'adattatore web
      (`IngressoMotoreR4`): PROPOSTA_CAMBIATA (contenuto, altra_occorrenza, gruppo_non_proposto), NON_INCLUSO_CONDIVISO, i
      conflitti aperti del prodotto (`conflitti_motore`, condizione (f)); il motore che non risponde in R4 (CALCOLO_OCCUPATO,
      CALCOLO_SCADUTO, ERRORE_CALCOLO) lascia 200 con `avvisi_motore_calcolati` falso; il riepilogo leggero (senza
      `?avvisi_motore=1`) non e' confermabile e ha un'altra firma; `ingressi` (le parti di R1 ?ingressi=1) fuori firma
    - **Non calcolati** (dichiarati): RIF_AMBIGUO (non si calcola in V1), STEP_SECONDARIO_DISPONIBILE (le sorgenti
      secondarie sono V5)
    - Prove: `riepilogo3_completamento_test.go`, `riepilogo3_completamento_registro_test.go`, `riepilogo3_motore_test.go`,
      `riepilogo3_costo_test.go` (L1, con la misura del costo con 5000 nodi), `riepilogo3_completamento_db_test.go`,
      `riepilogo3_costo_db_test.go` (L4)
  - Prove: `riepilogo3_test.go` (L1: bozza senza STEP, mono_pezzo e gate, zero figli, fonte A diversa da B e revoca e
    legame storico, gruppi di provenienza, STEP sorgente fuori dalle PB e STEP documentale, Prove 4, 5, 7, 8 della
    direttiva 35 nella parte di R4, assenze ed esclusi, identita', struttura, file conteso, firma, corpo) e
    `riepilogo3_db_test.go` (L4: bozza senza STEP con PB manuale salvata da R2 e nessuna scrittura, assenze della working,
    sorgente: storico, S8 con la sola radice, revoca; gruppi sulla bozza salvata e proposta rimossa; Prova 7 con una AR
    legacy; CICLO) e `c39_gate_prove_db_test.go` (L4 del gate della direttiva 39: P35-05, lo spostamento con piu' padri
    sposta solo l'arco indicato e non tocca le AR; P35-08, la seconda bozza trova gia_applicata in R4 e R5 e lo svincolo
    da un prodotto mostra l'effetto sull'altro in condivisione[])
- **R5 formato 3, la conferma atomica** (direttiva 39 §1, §2, §7 [U + T-39]; EM §9.5, §7.5, App. I.6):
  - **`conferma3.go`**: `LeggiRichiestaR5` (`{firma_anteprima, bozza, presa_d_atto_in_analisi?}`; la bozza con il lettore di
    R4 piu' gli obblighi `bozza_id` (motivo `bozza_obbligatoria`), `bozza_versione`, `chiave_idempotenza`; l'impronta del
    corpo senza `bozza_versione`, con la presa d'atto in ordine), `Conferma3` (la transazione T, la connessione S
    `REPEATABLE READ READ ONLY` dopo il lucchetto con un'attesa di 5 s, H_S/H_T sulle parti proposta e analisi con gli
    elenchi del piano, il secondo controllo, 40P01/55P03/40001 come 409 `riprova`), `controlliR5` (base → il gate
    `STEP_SORGENTE_NON_CONFERMATA` → firma → blocchi → presa d'atto → `NESSUNA_MODIFICA` → `R5_NON_APPLICABILE`),
    `gateInT` (la sorgente riletta in T sotto il lucchetto), `RifiutoR5` (BLOCCHI con i blocchi, ANTEPRIMA_SUPERATA con il
    riepilogo, CONTROLLO_FINALE e CALCOLO_OCCUPATO non registrati)
  - **`conferma3_piano.go`**: `pianificaR5`, la stessa funzione in S e in T (i due piani si confrontano): nodi inclusi,
    mappa verso i componenti esistenti, nodi da creare, archi voluti, esclusi da archiviare o condivisi, la promozione
    della sorgente e la sostituzione documentale, gli elenchi delle righe da bloccare, segnare e togliere da H, le parti
    che R5 non saprebbe applicare (la difesa: R4 li blocca prima); `tipoEffettivo3` (la regola unica del tipo dopo la conferma, anche per R4)
  - **`conferma3_scritture.go`**: le scritture nell'ordine di App. I.6: working (componenti nuovi con una INSERT senza
    `ON CONFLICT`, revisione e descrizione, archi tolti con la loro decisione, cambiati e nuovi, esclusi archiviati o
    condivisi, la promozione `componente.step_strutturale_id = B` con l'evento `autorizzazione`) → `A.sostituito_da = B`
    → testata (formato 3, firma del perimetro e contatori dopo le scritture, registri citati) → righe e archi certificati
    e righe dei non inclusi, con le tracce delle esclusioni senza decisione propria (`assenza` con le righe e gli archi
    chiusi, `escludi` per ogni casella del nodo escluso, `legacy` ed `ereditata` solo come traccia; una riga per
    (sha256, chiave), la decisione citata solo se e' dello snapshot) → AR dal servizio unico (`Registra`, `Sostituisci`, `Scollega(svincolo_conferma)`) con
    `provenienza_pb` e `importazione_id` risolto dall'impronta della PB → segni legacy (righe dei nodi e archi STEP
    tenuti, chiusura dei non inclusi e delle voci assenti dalla bozza con la nota fissa «esclusa con la conferma
    cumulativa n. N, senza motivo scritto», chiusura delle righe aperte e delle rimozioni di A) → bozza `confermata`, intento
    `conferma_bom_v3` con la risposta, `f8_stato` in attesa
  - **La ricodifica** (blocco «R5 completo»; D-ASS-01 [U]; EM §9.5 passo 5, H.5): in R4 `RICODIFICA_FILE_NAS` vale per
    ogni documento confermato del componente (motivi `scritto_sul_nas` e `documento_confermato`, [T-39]: R5 non scrive
    `documento`), e il nodo di un registrato porta il codice della working o quello della ricodifica (altrimenti
    `IDENTITA_INCOMPATIBILE` per un nodo modifica, `RIUSO_INCOMPATIBILE` «codice» per un riuso); in R5, prima dei
    componenti nuovi, il codice nuovo nella working (solo se e' ancora `codice_prima`; 23505 = 409 `BASE_SUPERATA`
    `codice_occupato`) con `fk_proposta_componente` differita, poi al passo delle AR `AllineaCodiceCopia` e una riga di
    `correzione_documento` (`ricodifica_bom`, con `associazione_id`) per ogni AR attiva del componente: la AR resta sulla
    stessa riga («Mantieni»; l'azione `mantieni` non c'e' in `ck_associazione_azione`: la traccia e' la correzione,
    [T-39]). H_S e H_T non contano il codice delle copie legacy dei componenti ricodificati (`IngressiProposta` (2-ter))
  - **L'ordine dei lucchetti di R5** (da usare uguale in C-6b e nei gesti legacy che toccano gli stessi oggetti, per
    non avere 40P01): `BloccaThread` (RFQ) → `BloccaDistintaBozza` (la bozza) → `documento` di A e di B `FOR UPDATE` per
    `documento_id` crescente → `componente` del prodotto `FOR UPDATE` → `documento_proposta` degli allegati citati (PB e
    svincoli) per `proposta_id` → `componente_proposta` delle righe da segnare per `proposta_id` → `relazione_proposta`
    per (allegato, padre, figlio); `SET LOCAL lock_timeout = '5s'` prima dei lucchetti delle righe. Il servizio unico
    delle AR non prende lucchetti propri. Il motore si calcola dentro S (`OpzioniR5.Motore`, un savepoint della
    transazione S, con il semaforo dei calcoli del servizio): occupato o non pronto 503 CALCOLO_OCCUPATO (con il gate o la
    base superata prima, dal riepilogo leggero), scaduto 504 CALCOLO_SCADUTO, un guasto 500 ERRORE_CALCOLO, tutti non
    registrati; prove in `c39_r5_motore_db_test.go`
  - **I registrati** (blocco «R5 completo», [T-39]; EM §5.2 W1, W3): il tipo dopo `cambia_tipo` o D1 del nodo del
    componente si scrive nella working (`tipi_cambiati`), tranne da o verso commerciale per un componente con
    un'autorizzazione STEP del Fascicolo e verso finito (in R4 `TIPO_EFFETTO_CAMBIATO` motivo `effetto_non_applicabile`:
    il gesto della scheda sospende l'autorizzazione e ricalcola le rimozioni, R5 no); il riuso di un archiviato lo
    ripristina (`identita_ripristinate`), tranne con un'autorizzazione STEP (`RIUSO_INCOMPATIBILE`
    `archiviato_con_autorizzazione`); due pezzi nuovi con lo stesso codice sono un componente solo (lo crea la prima chiave,
    le altre sono alias) e i nodi dello stesso componente si scrivono e si certificano una volta (la chiave del primo
    nodo), con la stessa identita' e gli stessi dati (in R4 `COLLISIONE_EMITTENTI`, `IDENTITA_INCOMPATIBILE`,
    `RIUSO_INCOMPATIBILE` `stesso_componente`); un crea risolto su un registrato prende il tipo del componente; D1 ed E37
    contano i figli del componente, non del solo nodo. `R5_NON_APPLICABILE` resta come difesa (nessun caso lo fa nascere
    dopo un R4 confermabile)
  - Prove: `conferma3_test.go` (L1: lettura del corpo, tipo effettivo, ordine dei controlli, rifiuti del COMMIT, righe
    decidibili) e `c39_r5_db_test.go` (L4: bozza senza STEP e STEP storico; prima conferma con le AR, il reinvio e i
    rifiuti; revisione con il guasto iniettato dopo ogni passo; ordine rovesciato P4X01; zero figli e mono_pezzo; revoca
    anche in concorrenza; Prova 7; spostamento e storia di due conferme; ricodifica non applicata)

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
- **`LeggiBozzeDellaRfq(ctx, q, thread) (BozzeDellaRfq, error)`** (direttiva 35, C-4): le bozze aperte di tutti i
  prodotti della RFQ e le AR attive, per S9 (la chiamera' `transport/web/motore_a`, corsia motore); ogni bozza formato 3 porta anche
  tutti i suoi nodi (`Nodi`, nell'ordine dello snapshot: richiesta 2 della corsia motore, direttiva 39), e un errore di
  lettura di uno snapshot nomina la bozza e il prodotto
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
  `core/rfq/distintabozza/importazione` (V1), `core/rfq/associazioni` (direttiva 35, C-4: le AR attive per S9),
  `core/rfq/confermabom` (V1, ondata B: lo stato della RFQ, la versione
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
  - direttiva 35, C-4: `bozze_rfq_db_test.go` (la lettura delle bozze di tutti i prodotti: due prodotti con la bozza
    formato 3 e un terzo formato 2, PB con il nodo e la casella, lo stesso file in due bozze, svincoli con la AR attiva
    e chiusa, vassoio uguale a R1 anche per un nodo tolto, le AR dal servizio e non dalla copia, il perimetro)
  - direttiva 39 §4: `c39_pb_db_test.go` (PB motore iniziali solo con S8, la PB manuale senza S8, l'ambigua non
    applicata, `ar_attiva`, la correzione → corretta con gli eventi, R1 `pb_toccata`, il suggerimento accettato e
    `proposta_non_corrispondente`, `importazione_sconosciuta`, `provenienza_dal_server`, il salvataggio che omette i
    campi del server, la PB rimasta con il registro A dopo il passaggio a B, la conversione P1 alla presa in carico con
    il reinvio e la legacy spostata a mano)
- **L1 della direttiva 39 §4**: `c39_provenienza_test.go` (i campi dichiarati, la decisione, gli eventi, la forma e il
  round-trip dello snapshot), `c39_pbiniziali_test.go` (`ClassificaProposte` caso per caso, `PropostaNelRegistro`,
  `ricollocabiliDi`, gli eventi del server)
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
  (servizio), R11 nel transport. Direttiva 35, blocco C-3: le forme `crea`, `cambia_sorgente`, `riallinea_proposta`,
  `converti_da` di R2. Restavano R4, R5 e la conferma formato 3 (C-5) e S8 (il passo 1 sospeso: INC-S1): R4 e S8 dalla
  direttiva 39 (sotto).
- Direttiva 39 §4 (20/10): PB iniziali del server dal registro (crea step, da-conferma-1, cambia_sorgente,
  riallinea_proposta) solo con S8, provenienza decisa dal server in ogni forma, `preassociazioni_proposte[]` (forme del
  server e R1), conversione P1 alla presa in carico. Il riconoscimento di una PB legacy (R39-15, blocco «R4 completo»):
  l'azione `file_riconosciuto` del vocabolario 2 (non si annulla), la PB resta legacy e diventa `verificata` (decisa dal
  server, `provenienzeDecise`); R4 la mette in `legacy_verificate` e il Piano non la blocca piu'; prove
  `c39_riconoscimento_test.go` (L1) e `c39_riconoscimento_db_test.go` (L4, fino alla AR con provenienza_pb legacy).
- Direttiva 39, R4 formato 3 (20/10): `Riepilogo3` con il gate della sorgente, i gruppi D-PB-04 e il blocco
  `legacy_da_verificare` (vedi sopra, con l'elenco di cio' che non e' calcolato).
- Direttiva 39, R5 formato 3 (20/10): `Conferma3` (vedi sopra), con le parti non applicate dichiarate
  (`R5_NON_APPLICABILE`). Blocco «R5 completo» (20/10): le tracce delle esclusioni senza decisione propria (assenza,
  escludi, legacy, ereditata) e la chiusura delle righe delle voci assenti, dichiarate in R4 (`esclusi[].effetti`,
  condizione (c)); la ricodifica (Prova 4); il tipo, il ripristino e i nodi dello stesso componente; prove
  `c39_r5_completo_db_test.go`, `c39_r5_ricodifica_db_test.go`, `c39_r5_registrati_db_test.go` (L4) e
  `c39_r5_registrati_test.go` (L1); `c39_r5_prove_db_test.go` (L4: Prova 8 con l'escluso condiviso, due R5 in
  concorrenza con due chiavi e con la stessa, la connessione S che non arriva = 503 non registrato, Prova 10 con
  `NESSUNA_MODIFICA`).
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
