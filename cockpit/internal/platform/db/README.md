---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/platform/db`

- il codice generato da sqlc: i tipi dello schema e una funzione per query

## Scopo

- Rende chiamabili da Go, con tipi controllati, lo schema di `migrations/` e le query di `queries/*.sql`:
  - un modello per ogni tabella e vista (`models.go`)
  - un tipo per ogni enum
  - una funzione `(*Queries).<Nome>` per ogni query
- È il posto dell'SQL applicativo: chi ha bisogno di leggere o scrivere il database aggiunge una query qui e la chiama con `db.New(pool|tx).<Nome>(ctx, …)`.
- È l'unico package di `platform` che conosce le parole del dominio (RFQ, fascicolo, BOM…)
  - non per scelta, ma perché è lo schema tradotto in Go.
- Non contiene logica: ogni file è generato.

## Non appartiene qui

- **Lo schema e le sue regole:** `migrations/` (vedi `migrations/README.md`).
  - sqlc lo legge da lì.
- **Le transazioni:** le apre chi chiama (`pool.Begin`, poi `db.New(tx)`).
  - Qui ci sono solo le query che prendono i lucchetti (`Blocca*`, `FOR UPDATE`) e quelle che confrontano e scambiano.
- **Le decisioni di dominio:** stanno in `core/*` (e, per le eccezioni dichiarate in `internal/README.md`, in `transport/*`).
  - Le query leggono e scrivono quello che il chiamante ha deciso.
- **Codice scritto a mano:** nessun `.go` di questa cartella lo è.
  - Una modifica si perde al prossimo `sqlc generate`.

## File

- **`db.go`**
  - `DBTX` (`Exec`, `Query`, `QueryRow`: lo soddisfano `*pgxpool.Pool`, `pgx.Tx`, `*pgx.Conn`)
  - `New`, `Queries`
  - `WithTx` (nessun chiamante: si usa `db.New(tx)`)
- **`models.go`**
  - 47 enum (`TipoJob`, `StatoNas`, `TipoDocumento`, `Fase`…) con `Null<Enum>`, `Valid()` e `All<Enum>Values()`
  - una struct per ognuna delle 87 tabelle (69 fino alla 0021, 18 della 0022 di P4) e delle 11 viste (`Messaggio`, `Documento`, `VInbox`, `VFascicolo`, `VStepProdotto`…)
- **`queries/<nome>.sql`**
  - la sorgente: una o più query `-- name: <Nome> :one|:many|:exec|:execrows`
- **`<nome>.sql.go`**
  - generato da `queries/<nome>.sql`: la costante SQL, `<Nome>Params`, `<Nome>Row`, il metodo su `*Queries`
- **`sqlc.yaml` (nella cartella `cockpit/`)**
  - la configurazione: sorgenti, `pgx/v5`, `rename` dei singolari latini, `overrides` di tipo e di nullabilità

### Le query per file

- 46 file, 655 query (contati il 20/10 con `ls queries/*.sql` e `grep -h '^-- name:' queries/*.sql`; V1, direttiva 35, blocco C-1c,
  corsia persistenza: + `v1_associazioni.sql` con 17 query; V1, direttiva 35, blocco C-2,
  corsia persistenza: + `v1_preassociazioni.sql` con 2 query; V1, direttiva 39 §4, corsia persistenza: +
  `v1_provenienza_pb.sql` con 4 query; V1, direttiva 39, R4, corsia persistenza: + `v1_riepilogo_r4.sql` con 5 query (47
  file, 660 query); V1, direttiva 39, R5, corsia persistenza: + `v1_conferma_r5.sql` con 13 query (48 file, 673 query);
  V1, direttiva 39, blocco «R5 completo», corsia persistenza: + `v1_r5_completo.sql` con 5 query;
  V1, direttiva 39, rilievi della revisione del 20/10, corsia persistenza: + `v1_step_del_finito.sql` con 1 query (51
  file, 686 query contati il 20/10 sul ramo `v1-persistenza`) e `v1_conversioni_p1.sql` con 1 query (52 file, 687 query);
  V1, ondata C, corsia
  persistenza: + `v1_ingressi.sql` con 4 query e `v1_proposta.sql` con 2; A1c: 33 e 482, A1d-P4-B1: +
  `p4_conferme.sql` con 27 query, 22 piu' le 5 delle correzioni del 13/10; V1: + `v1_formato3.sql` con 43 query e
  `ConfermaDistintaBozza` in `p4_conferme.sql`; V1, ondata B, corsia persistenza: + `v1_bozza_rotte.sql` con 22 query;
  V1, corsia motore: + `v1_motore.sql` con 3 query; V1, ondata B, corsia file: + `v1_file.sql` con 42 query,
  `AzzeraPathStagingPerSha` sostituita da `AzzeraPathStagingPerFile` in `cache.sql`).
- «Senza chiamanti» = nessun chiamante, né in produzione né nei test; il «+n» fra parentesi conta quelle chiamate soltanto dai test.
- L'elenco per nome è in «Stato dell'implementazione».
- **`agente.sql`** — analisi semantica dell'agente
  - Tabelle e viste: `analisi_messaggio`
  - Query: 6; senza chiamanti: 2
  - Chi le chiama: `ai/agente`
- **`aggancio.sql`** — candidati di aggancio (R0–R5) e di codice
  - Tabelle e viste: `candidato_aggancio`, `candidato_codice`, `messaggio`, `conversazione`, `thread_offerta`, `identificativo_thread`, `proposta_triage`
  - Query: 16; senza chiamanti: 1
  - Chi le chiama: `core/inbox/aggancio`, `core/inbox/ingest`, `transport/web`, `ai/agente`
- **`allegati.sql`** — allegati, hash di rumore
  - Tabelle e viste: `allegato`, `hash_rumore`, `messaggio`
  - Query: 11; senza chiamanti: 1
  - Chi le chiama: `transport/workerapi`, `platform/coda`, `core/rfq/fascicolo`, `core/rfq/documenti`, `core/inbox/ingest`, `transport/web`, `ai/agente`
- **`altro.sql`** — anagrafica «Altro» (7C.0)
  - Tabelle e viste: `soggetto_altro`, `recapito_altro`
  - Query: 8; senza chiamanti: 5 (+2)
  - Chi le chiama: `core/inbox/ingest`
- **`anagrafica.sql`** — clienti, domini, buyer, fabbisogno, cataloghi
  - Tabelle e viste: `cliente`, `dominio_cliente`, `buyer`, `fabbisogno_documento`, `fase_catalogo`, `transizione`, `regola`
  - Query: 26; senza chiamanti: 4
  - Chi le chiama: `transport/web`, `core/registro/anagrafica`, `core/registro/fornitori`, `core/inbox/ingest`, `core/rfq/fascicolo`, `transport/workerapi`
- **`analisi.sql`** — fatti dell'analisi, analizzatore corrente
  - Tabelle e viste: `analisi_fatti`, `analizzatore_corrente`, `documento_proposta`, `allegato`
  - Query: 5; senza chiamanti: 0
  - Chi le chiama: `transport/workerapi`, `platform/coda`, `core/rfq/fascicolo`, `transport/web`, `app/runtime`
- **`annotazioni.sql`** — note sui PDF (0021)
  - Tabelle e viste: `annotazione_pdf`, `allegato`, `messaggio`, `utente`, `componente`
  - Query: 5; senza chiamanti: 0
  - Chi le chiama: `transport/web`
- **`bom.sql`** — versioni della BOM, istantanee, gate del congelamento
  - Tabelle e viste: `bom_versione`, `bom_versione_{componente,relazione,documento,deroga}`, `v_bom_versioni`, `v_step_prodotto`, `v_thread_bloccanti`, working e proposte
  - Query: 21; senza chiamanti: 5
  - Chi le chiama: `core/rfq/fascicolo`, `transport/web`
- **`cache.sql`** — cache dei contenuti dello staging
  - Tabelle e viste: `allegato`, `documento`, `nas_anomalia`, `documento_proposta`, `job`
  - Query: 5; senza chiamanti: 0
  - Chi le chiama: `platform/storage/staging`, `core/rfq/documenti`, `core/rfq/fascicolo`
- **`candidati.sql`** — codici candidati di una RFQ (B8.6)
  - Tabelle e viste: `v_codici_candidati_thread`, `documento_proposta`, `componente_proposta`
  - Query: 2; senza chiamanti: 0
  - Chi le chiama: `core/rfq/fascicolo`
- **`editor.sql`** — editor della struttura (Fascicolo v3)
  - Tabelle e viste: `rimozione_proposta`
  - Query: 1; senza chiamanti: 0
  - Chi le chiama: `core/rfq/fascicolo`
- **`fascicolo.sql`** — proposte di documento, documenti, percorsi e lucchetti del NAS
  - Tabelle e viste: `v_fascicolo`, `v_thread_bloccanti`, `v_documento_storia`, `v_step_prodotto`, `documento_proposta`, `documento`, `documento_provenienza`, `cartella_documento`, `componente`, `job`, `nas_orfano`
  - Query: 38; senza chiamanti: 3
  - Chi le chiama: `transport/web`, `core/rfq/documenti`, `core/rfq/fascicolo`, `core/inbox/ingest`, `transport/workerapi`
- **`fotografia.sql`** — la fotografia del motore A (giro 5, A1c, 6.5): le sei query nuove del caricatore in sola lettura
  - Tabelle e viste: `analisi_fatti`, `allegato`, `messaggio`, `documento`, `cliente` (solo `cliente_id` e `ragione_sociale`, mai `regole`), `utente` (solo `utente_id` e `sigla`, mai `password_hash`), `candidato_codice` (senza l'origine `agente`), `proposta_triage` (solo la fonte `deterministico`)
  - Query: 6 (`ListFattiDelThread`, con i contenuti degli allegati e dei documenti del thread; `ListFattiPerSha`; `ListClientiDellaFotografia`; `ListSigleUtenti`; `ListCandidatiCodiceThread`; `ListTriageThread`); senza chiamanti: 0
  - Chi le chiama: `core/fotorfq/caricatore`
  - Solo `SELECT`, senza lucchetti: valgono dentro la transazione `REPEATABLE READ READ ONLY` del caricatore. Per i fatti senza struttura STEP il motivo di completezza non c'è: lo dice `con_struttura` (sqlc non vede che un `CASE` senza `ELSE` può dare `NULL`)
- **`fondazioni.sql`** — caselle, postazioni, credenziali dei worker
  - Tabelle e viste: `casella`, `postazione`, `worker_credenziale`, `casella_store`
  - Query: 23; senza chiamanti: 1 (+3)
  - Chi le chiama: `platform/fondazioni`, `transport/web`, `transport/workerapi`, `core/inbox/ingest`, `platform/coda`
- **`fornitori.sql`** — fornitori, lavorazioni, qualifiche, convenzioni di codice, controparte
  - Tabelle e viste: `fornitore`, `dominio_fornitore`, `contatto_fornitore`, `lavorazione`, `fornitore_lavorazione`, `cliente_fornitore_lavorazione`, `convenzione_codice(_lavorazione)`, `messaggio`
  - Query: 35; senza chiamanti: 1
  - Chi le chiama: `transport/web`, `core/registro/fornitori`, `core/inbox/ingest`, `app/runtime`
- **`inbox.sql`** — Inbox viva (voce 2.16)
  - Tabelle e viste: `utente`, `messaggio`, `messaggio_casella`, `job`
  - Query: 6; senza chiamanti: 0
  - Chi le chiama: `transport/web`
- **`integrita.sql`** — integrità del NAS, orfani, prove di creazione
  - Tabelle e viste: `documento`, `nas_anomalia`, `nas_orfano`, `nas_creazione`, `job`
  - Query: 17; senza chiamanti: 4 (+2)
  - Chi le chiama: `core/rfq/documenti`, `transport/web`
- **`interpretazione.sql`** — portale, deroghe, triage, bozze
  - Tabelle e viste: `riferimento_portale`, `deroga_fabbisogno`, `proposta_triage`, `bozza`
  - Query: 14; senza chiamanti: 3
  - Chi le chiama: `core/inbox/ingest`, `transport/web`, `transport/workerapi`, `core/rfq/fascicolo`
- **`job.sql`** — coda dei job, presenza dei worker
  - Tabelle e viste: `job`, `worker_presenza`, `postazione`
  - Query: 31; senza chiamanti: 1 (+1)
  - Chi le chiama: `platform/coda`, `transport/web`, `transport/workerapi`, `app/runtime`, `core/rfq/documenti`, `core/inbox/ingest`, `platform/storage/staging`
- **`messaggi.sql`** — messaggi, presenze per casella, cursori, Inbox
  - Tabelle e viste: `conversazione`, `messaggio`, `messaggio_outlook`, `messaggio_casella`, `sync_cursore`, `v_inbox`
  - Query: 34; senza chiamanti: 4 (+1)
  - Chi le chiama: `core/inbox/ingest`, `transport/web`, `transport/workerapi`, `platform/coda`, `core/rfq/fascicolo`, `core/rfq/documenti`, `ai/agente`
- **`p4_conferme.sql`** — la 0022 di P4 (giro 5, A1d, B1): la bozza della distinta, le conferme strutturali, l'esito degli intenti, i contatori della working
  - Tabelle e viste: `distinta_bozza`, `distinta_bozza_versione`, `distinta_bozza_evento`, `conferma_strutturale`, `conferma_strutturale_componente`, `conferma_strutturale_arco`, `esito_intento`, `working_contatore`; in lettura `componente` (il prodotto e il perimetro registrato), `componente_relazione` (gli archi della working), `allegato` + `messaggio` (la fonte)
  - Query: 27; senza chiamanti: 0 (+7: `ListDistinteBozzeProdotto`, `ListConfermeStrutturali`, `GetConfermaStrutturale`, `ListConfermaStrutturaleComponenti`, `ListConfermaStrutturaleArchi`, `InsertEsitoIntento`, `GetEsitoIntento`, che useranno B2 e B3)
  - Chi le chiama: `core/rfq/distintabozza`
  - V1 (0023): `InsertDistintaBozza` passa il formato; `InsertDistintaBozzaVersione`, `GetDistintaBozzaVersione` e
    `ListDistintaBozzaVersioni` portano registro e chiave del salvataggio; le letture delle conferme portano
    `importazione_id` e `importazioni_citate`; `ConfermaDistintaBozza` chiude la bozza nella transazione di R5
- **`v1_formato3.sql`** — la 0023 di V1 (giro 5, A1d; direttiva 23): bozza formato 3, registro d'importazione, conferma formato 3, R4/R6/R7, atti della fonte, ciclo dei file della Sorgente
  - Tabelle e viste: `distinta_importazione`, `distinta_bozza`, `distinta_bozza_versione`, `conferma_strutturale` e il suo contenuto, `conferma_strutturale_non_incluso`, `componente` (`InsertComponenteSenzaFusione`, senza ON CONFLICT), `associazione_evento`, `f8_stato`, `fonte_step_evento`, `attestazione_fascicolo`, `componente_proposta` e `relazione_proposta` in lettura, `file_rfq`, `file_rfq_evento`, `file_rfq_rimozione`, `file_rfq_conservazione`, `documento_proposta` in lettura (D-6)
  - Query: 43; senza chiamanti in produzione al primo commit: tutte (le chiamano R1-R7 nel passo 6 del piano della 0023, il motore per `GetSorgenteBozzaAperta`, il servizio dei file per le query dei file); le prova `platform/migrazioni/v1_0023_query_db_test.go`. `ListIdentitaRegistrate` la chiama il modello di `/motore-a/dati` (`transport/web/motore_a/dati_v1.go`)
  - Chi le chiama: per ora solo le prove L4
  - Le colonne `tx` (xid8) non si leggono; le date le scrive il database; ogni lettura porta la RFQ nel `WHERE`: le testate direttamente, versioni ed eventi con una JOIN sulla testata (RC-19); gli eventi si leggono fino a una versione e a pagine
- **`v1_motore.sql`** — le letture di V1 del modello della pagina della Distinta (`/motore-a/dati`, EM §9.9; corsia motore, ondata B), solo SELECT, nello snapshot REPEATABLE READ READ ONLY della fotografia
  - Tabelle: `file_rfq` (le occorrenze non attive), `distinta_bozza`, `distinta_bozza_versione` e `distinta_importazione` (le bozze aperte formato 3 con le colonne del registro della versione, mai il contenuto), `f8_stato` (lo stato del rismistamento)
  - Query: 2 (`ListSorgentiBozzeAperte`, `GetStatoF8DelThread`); le chiama `transport/web/motore_a` (`dati_v1.go:leggiV1`); le prova la L4 `transport/web/motore_a/dati_v1_db_test.go`. Direttiva 39 (corsia motore): `ListEventiFonteDelThread` dice anche `con_chiave` (l'evento ha la chiave d'idempotenza di un intento: la conferma e la revoca di S8 la hanno, gli eventi delle pagine legacy no), per non leggere la revoca della sorgente della bozza come la revoca dell'autorizzazione della working (sqlc v1.31.1: cambia solo `v1_motore.sql.go`). Dall'ondata C le occorrenze non attive le legge la lettura unica `ListOccorrenzeNonAttiveRfq` di `v1_file.sql` (la stessa di F8; decisioni dell'ondata B §5.1): `ListFileRfqNonAttivi`, con lo stesso testo SQL, è tolta (sqlc v1.31.1 su una copia: cambia solo `v1_motore.sql.go`)
- **`v1_bozza_rotte.sql`** — V1, ondata B (corsia persistenza, passo 6 del piano della 0023, prima parte): le letture dello stato della RFQ e delle rotte della bozza nel formato 3, gli aggiornamenti di `f8_stato` di R11, la testata di una bozza convertita, l'allegato della storia della fonte degli atti esistenti
  - Tabelle e viste: `componente`, `componente_relazione`, `conferma_strutturale` (testate), `documento`, `documento_provenienza`, `allegato` + `messaggio` (con `file_rfq_attivo`), `documento_proposta`, `thread_offerta` (anche FOR KEY SHARE per R6), `distinta_bozza` (insert con l'id della conversione), `distinta_bozza_versione`, `esito_intento`, `file_rfq_evento`, `associazione_evento`, `f8_stato` (letture e i due UPDATE di R11), `file_rfq`, `file_rfq_rimozione`, `file_rfq_conservazione`, `utente`, `distinta_importazione`
  - Query: 22; chi le chiama: `core/rfq/confermabom` (lo stato della RFQ), `core/rfq/distintabozza` (R1, R2, R6, R7, conversione), `core/rfq/fascicolo` (`GetAllegatoDelDocumento`) e `transport/web` (R11, con `OraDelDatabase`: l'inizio del riallineamento sull'orologio del database, dal secondo passaggio dell'ondata B). Lo schema non cambia: nessuna migrazione
- **`v1_ingressi.sql`** — V1, ondata C (corsia persistenza): `impronta-ingressi-1` (U2 §6.3.3), le quattro parti degli ingressi del riepilogo che R1 `?ingressi=1` legge per la notifica con il wizard aperto e che R5 rileggera' per il controllo finale (parti proposta e analisi). Un'istruzione sola per parte, solo SELECT, senza lucchetti; il digest e' calcolato nel database sul contenuto delle righe (mai orari, id d'interpretazione o i fatti interi: `analisi_fatti.digest` della 0023)
  - Tabelle e viste: `working_contatore`, `conferma_strutturale`, `componente`, `fonte_step_evento`, `distinta_bozza`, `componente_proposta`, `relazione_proposta`, `rimozione_proposta`, `documento_proposta`, `analisi_fatti`, `file_rfq` (con `file_rfq_attivo` e `file_rfq_scartato_legacy`), `allegato`, `messaggio`, `deroga_struttura`, `deroga_fabbisogno`, `documento`, `job`
  - Query: 4 (`IngressiBom`, `IngressiBozza`, `IngressiProposta`, `IngressiAnalisi`); le chiama `core/rfq/distintabozza` (`ingressi.go`); le prova la L4 `core/rfq/distintabozza/ondatac_formato_db_test.go`. Generate con la stessa v1.31.1 della cache dei moduli: nasce solo `v1_ingressi.sql.go`
  - Direttiva 35, blocco C-1c: `IngressiProposta` e' `impronta-ingressi-2` (INC-G12): fuori le colonne di copia di `documento_proposta` e le righe che R5 segna (il segno e l'elenco passato da R5: `escludi_proposte`, `escludi_archi`), dentro le AR degli allegati citati dalla bozza del prodotto (o `allegati_citati`), viste come prima della transazione corrente; tabelle in piu': `file_rfq_associazione`, `distinta_bozza_versione`. In `v1_formato3.sql` `GetAssociazioneEventoPerChiave` e' ristretta al gesto `associa_step` (`assegna`, origine `ponte`, senza riga della AR)
- **`v1_preassociazioni.sql`** — V1, direttiva 35, blocco C-2 (corsia persistenza, file proprio per INC-G21): le preassociazioni del formato 3 per `allegato_id`. Le occorrenze date con appartenenza alla RFQ, attivita' (`file_rfq_attivo` e non `file_rfq_scartato_legacy`) e natura preassociabile (provvisorio, base [T] D-2 (a): la regola del punto 1 di `trg_fra_nasce`, scritta uguale); le AR attive degli allegati dati (lo svincolo di R2 cita una AR attiva, EM §3.6.2). Solo SELECT
  - Tabelle: `allegato`, `messaggio`, `file_rfq`, `file_rfq_evento`, `documento_proposta` (nelle due funzioni), `file_rfq_associazione`
  - Query: 2 (`ListAllegatiPreassociabili`, che chiama `core/rfq/distintabozza` (`preassociazioni3.go`, `conversione.go`); `ListAssociazioniAttiveDegliAllegati`, che chiama `preassociazioni3.go`); le prova la L4 `core/rfq/distintabozza/c2_preassociazioni_db_test.go`. Generate con la v1.31.1 della cache dei moduli: nasce solo `v1_preassociazioni.sql.go`, e cambia `v1_formato3.sql.go` per D-6 (`ListDipendenzeBozzaPerFile`: PB e svincolo del formato 3 per `allegato_id`, la PB del formato 2 con il join di prima)
- **`v1_associazioni.sql`** — V1, direttiva 35, blocco C-1c (corsia persistenza, file proprio per INC-G21): il servizio unico delle
  associazioni registrate. Le letture delle AR (attive di un allegato e della RFQ, la storia, per chiave d'atto e per chiave di chiusura),
  la nascita e la chiusura di una riga, l'evento del giornale, la copia derivata (scritta con la lettura del codice conservata in
  `dettagli.codice_letto`, tolta rimettendola), l'allineamento del codice delle copie, i controlli prima della scrittura (componente,
  documento confermato altrove), il caricamento della storia della fonte del legame della sorgente (provvisorio D-AR02 A, non cablato).
  Nessuna query prende lucchetti o usa `ON CONFLICT`
  - Tabelle: `file_rfq_associazione`, `associazione_evento`, `documento_proposta`, `componente`, `allegato`, `messaggio`, `documento`,
    `fonte_step_evento`
  - Query: 17; le chiama solo `core/rfq/associazioni`; le prova la L4 `core/rfq/associazioni/associazioni_db_test.go`. Generate con la
    v1.31.1 della cache dei moduli: nasce solo `v1_associazioni.sql.go`
- **`v1_lettura_bozze.sql`** — V1, direttiva 35, blocco C-4 (parte persistenza; corsia persistenza, file proprio per INC-G21):
  la lettura delle bozze di tutti i prodotti della RFQ per S9 (U2 §12 punto 6). Le bozze aperte con la versione corrente
  (snapshot e impronta) e gli eventi delle sole azioni del vassoio «Da ricollocare» fino alla versione corrente di ogni
  bozza. Solo SELECT, senza lucchetti, nella transazione REPEATABLE READ READ ONLY di chi legge; non legge
  `documento_proposta` (prova L1 `TestV1C4LeQueryDellaLetturaNonLeggonoLaCopia`)
  - Tabelle: `distinta_bozza`, `distinta_bozza_versione`, `distinta_bozza_evento`
  - Query: 2 (`ListBozzeAperteDellaRfq`, `ListEventiFileBozzeAperteDellaRfq`); le chiama `core/rfq/distintabozza`
    (`bozze_rfq.go`); le prova la L4 `core/rfq/distintabozza/bozze_rfq_db_test.go`. Generate con la v1.31.1 della cache
    dei moduli: nasce solo `v1_lettura_bozze.sql.go`
- **`v1_forme_server.sql`** — V1, direttiva 35, blocco C-3 (R2 passo 6b; corsia persistenza, file proprio per INC-G21):
  le query delle forme di R2 costruite dal server che le altre non hanno
  - Tabelle: `documento`
  - Query: 1 (`GetDocumentoFonteConfermata`: il documento cad_3d confermato e non sostituito del prodotto con lo sha256
    della fonte, U2 §2.8); la chiama `core/rfq/distintabozza` (`importa3.go`); la prova la L4
    `core/rfq/distintabozza/c3_step_db_test.go`. Generata con la v1.31.1 della cache dei moduli: nasce solo
    `v1_forme_server.sql.go` (la rigenerazione riscrive `v1_file.sql.go` e `v1_motore.sql.go` con soli fine riga
    diversi: ripristinati)
- **`v1_sorgente_s8.sql`** — V1, direttiva 39 (S8, revoca, RFQ formato 3; corsia persistenza, file proprio per INC-G21):
  sole letture
  - Tabelle: `distinta_bozza`, `conferma_strutturale`, `distinta_importazione`, `fonte_step_evento`
  - Query: 2 (`RfqFormato3`: almeno una bozza o una conferma formato 3, o un registro d'importazione, nella storia della
    RFQ; la chiama `core/rfq/associazioni` (`formato3.go`); `ListFonteStepPerChiave`: gli eventi della fonte scritti con
    una chiave, per il «gia' fatto» del passo 2 di S8; la chiama `core/rfq/sorgente`). Le prova la L4
    `core/rfq/sorgente/s8_db_test.go`. Generate con la v1.31.1 della cache dei moduli: nasce solo `v1_sorgente_s8.sql.go`
    (la rigenerazione riscrive `v1_file.sql.go` e `v1_motore.sql.go` con soli fine riga diversi: ripristinati)
- **`v1_provenienza_pb.sql`** — V1, direttiva 39 §4 (PB iniziali, suggerimenti, conversione delle pre-assegnazioni;
  corsia persistenza, file proprio per INC-G21): sole letture, senza lucchetti
  - Tabelle: `file_rfq_associazione`, `distinta_bozza`, `distinta_bozza_versione`, `documento_proposta`, `componente`,
    `allegato`, `messaggio`, `documento`; le funzioni `file_rfq_attivo`, `file_rfq_scartato_legacy`,
    `allegato_sorgente_del_finito` della 0023
  - Query: 4 (`ListAssociazioniDegliAllegati`: le AR attive e chiuse degli allegati; `ListPreassociazioniAltreBozze`: gli
    allegati preassociati nelle bozze aperte formato 3 degli altri prodotti; `ListCopieSenzaARDellaRfq`: le
    pre-assegnazioni legacy aperte con tipo del componente, estensione, attivita', natura e STEP sorgente;
    `ListDocumentiConfermatiDegliAllegati`: il componente del documento confermato con lo stesso contenuto). Le chiama
    `core/rfq/distintabozza` (`pbiniziali3.go`, `conversionep1.go`); le prova la L4
    `core/rfq/distintabozza/c39_pb_db_test.go`. Generate con la v1.31.1 della cache dei moduli: nasce solo
    `v1_provenienza_pb.sql.go` (`v1_file.sql.go` e `v1_motore.sql.go` riscritti con soli fine riga diversi: ripristinati)
- **`v1_riepilogo_r4.sql`** — V1, direttiva 39 (R4 formato 3 e il Piano del servizio unico; corsia persistenza, file
  proprio per INC-G21): sole letture, senza lucchetti
  - Tabelle: `allegato`, `messaggio`, `documento_proposta`, `documento`, `analisi_fatti`, `analizzatore_corrente`,
    `deroga_struttura`; le funzioni `file_rfq_attivo`, `file_rfq_scartato_legacy`, `allegato_sorgente_del_finito` della
    0023 e `struttura_motivo_parziale` della 0020
  - Query: 5 (`ListOccorrenzePiano`: le occorrenze con la lettura del codice della proposta, la regola di
    `v1_ingressi.sql`; `ListDocumentiDegliAllegati`: i documenti con lo stesso contenuto, anche senza componente;
    `ListDocumentiScrittiDeiComponenti`: i documenti gia' sul NAS, per RICODIFICA_FILE_NAS; `GetStrutturaDellaSorgente`:
    il motivo parziale della struttura dello STEP sorgente e la deroga che vale; `ListStepDelProdotto`: i documenti
    `cad_3d` del finito con estensione e nome, per il legame storico univoco che ricava
    `confermabom.LegameStoricoUnivoco`; dal giro del gate del 20/10 il filtro sullo STEP non e' piu' nella query). Le chiamano `core/rfq/associazioni` (`piano.go`) e `core/rfq/distintabozza`
    (`riepilogo3_lettura.go`); le prova la L4 `core/rfq/distintabozza/riepilogo3_db_test.go`. Generate con la v1.31.1
    della cache dei moduli: nasce solo `v1_riepilogo_r4.sql.go` (`v1_file.sql.go` e `v1_motore.sql.go` riscritti con soli
    fine riga diversi: ripristinati). La nullabilita' di una colonna di un LEFT JOIN LATERAL e' scritta con un booleano
    (`con_deroga`), non con l'UUID nullo che sqlc non vede
- **`v1_riepilogo_r4_completo.sql`** — V1, direttiva 39, blocco «R4 completo» (corsia persistenza, file proprio per
  INC-G21): sole letture, senza lucchetti
  - Tabelle: `componente_proposta`
  - Query: 1 (`ListRigheStrutturaR4`: le righe legacy dei contenuti dati, con chi le ha decise: le esclusioni legacy e la
    condizione (c) sullo STEP sorgente, la condizione (e) sugli STEP secondari autorizzati). La chiama
    `core/rfq/distintabozza` (`riepilogo3_lettura.go`); la provano le L4 `riepilogo3_completamento_db_test.go`. Generata
    con la v1.31.1 della cache dei moduli: nasce solo `v1_riepilogo_r4_completo.sql.go` (cinque `.sql.go` riscritti con
    soli fine riga diversi: ripristinati)
- **`v1_conferma_r5.sql`** — V1, direttiva 39 (R5 formato 3, la conferma atomica; corsia persistenza, file proprio per
  INC-G21): le scritture della working che le query di prima non avevano (l'arco nuovo senza `ON CONFLICT`, la posizione
  di un arco), i lucchetti delle righe di proposta nell'ordine documentato (`documento_proposta`, `componente_proposta`,
  `relazione_proposta`), le righe dello STEP A sostituito (lettura e chiusura, tranne quelle che la bozza confermata usa),
  lo stato di F8 della conferma dopo il COMMIT
  - Tabelle: `componente_relazione`, `documento_proposta`, `componente_proposta`, `relazione_proposta`, `allegato`,
    `f8_stato`
  - Query: 13 (`InsertRelazioneR5`, `SetPosizioneRelazioneR5`, `BloccaDocumentiPropostaR5`, `BloccaComponentiPropostaR5`,
    `BloccaRelazioniPropostaR5`, `ListProposteDelloShaR5`, `ListRelazioniDelloShaR5`, `ListRelazioniPropostaR5`,
    `ChiudiProposteDelloShaR5`, `ChiudiRelazioniDelloShaR5`, `SegnaF8DellaConfermaEseguito`, `SegnaF8DellaConfermaErrore`,
    `GetF8StatoDellaConferma`); le chiamano `core/rfq/distintabozza` (`conferma3*.go`) e `transport/web`
    (`motore_a_bozza_v1_r5.go`, F8); le prova la L4 `core/rfq/distintabozza/c39_r5_db_test.go` e
    `transport/web/motore_a_bozza_v1_r5_db_test.go`. Generate con la v1.31.1 della cache dei moduli: nasce solo
    `v1_conferma_r5.sql.go`, cambia `v1_ingressi.sql.go` (sotto; `v1_file.sql.go` e `v1_motore.sql.go` riscritti con
    soli fine riga diversi: ripristinati)
  - In `v1_ingressi.sql`, `IngressiProposta` ha due elenchi in piu' (2-bis): `escludi_documenti` (lo STEP A che R5
    marca sostituito) e `escludi_rimozioni` (gli STEP di cui R5 chiude le rimozioni): scritture di R5, escluse da H_S e
    H_T; R1 passa elenchi vuoti
- **`v1_conversioni_p1.sql`** — V1, direttiva 39 §4 (D-AR14 A [T-39]; rilievo della revisione del 20/10; corsia
  persistenza, file proprio per INC-G21): le conversioni delle pre-assegnazioni legacy che una bozza abbandonata non ha
  consumato
  - Tabelle: `associazione_evento`, `documento_proposta`, `file_rfq_associazione`, `conferma_strutturale` (solo SELECT)
  - Query: 1 (`ListConversioniP1DaRiprendere`: gli allegati il cui ultimo evento del giornale delle AR e' la conversione
    `conversione_legacy`, senza AR attiva ne' copia, senza conferme formato 3 del prodotto dopo la conversione; l'allegato
    anche dalla proposta per gli eventi di prima della 0023). La chiama `core/rfq/distintabozza` (`conversionep1.go`); la
    prova la L4 `c39_conversione_p1_db_test.go`. Generata con la v1.31.1 della cache dei moduli: nasce solo
    `v1_conversioni_p1.sql.go` (quattro `.sql.go` riscritti con soli fine riga diversi: ripristinati)
- **`v1_step_del_finito.sql`** — V1, direttiva 39 §5 [U] (rilievo della revisione del 20/10; corsia persistenza, file
  proprio per INC-G21): gli STEP candidati come sorgente di un finito della RFQ, che le PB iniziali del server e i
  suggerimenti non toccano
  - Tabelle: `allegato`, `messaggio`, `documento_proposta`, `componente` (solo SELECT)
  - Query: 1 (`ListStepCandidatiDelFinito`: fra gli allegati dati, gli STEP della RFQ che una proposta di documento dice
    `cad_3d` di un finito della RFQ, per la copia del componente o per il codice, con il prodotto). La chiama
    `core/rfq/distintabozza` (`stepcandidati3.go`, `StepCandidatiDelFinito`); la prova la L4
    `core/rfq/sorgente/s8_step_del_finito_db_test.go`. Generata con la v1.31.1 della cache dei moduli: nasce solo
    `v1_step_del_finito.sql.go` (quattro `.sql.go` riscritti con soli fine riga diversi: ripristinati)
  - G39-02 [U] (20/10 sera): e' il RIPIEGO [T-39] di quando il calcolo del Motore A non c'e' (con il motore il candidato si
    riconosce dalla sua lettura). Rende anche il prodotto (riga `allegato_id`, `prodotto_id`) e confronta il codice letto
    anche senza il marcatore (il codice del prodotto senza la lettera finale dopo una cifra: 12345678 contro 12345678Z, codici inventati,
    PM-01). Nessun DDL
- **`v1_r5_completo.sql`** — V1, direttiva 39, blocco «R5 completo» (corsia persistenza, file proprio per INC-G21): la
  ricodifica di un componente registrato in R5 (D-ASS-01 [U]; EM §9.5 passo 5, H.5) e il ripristino di un archiviato
  riusato (EM §5.2 W3)
  - Tabelle: `documento`, `file_rfq_associazione`, `allegato`, `documento_proposta`, `correzione_documento`, `componente`
  - Query: 5 (`ListDocumentiDeiComponentiR4`: i documenti confermati dei componenti ricodificati, per
    `RICODIFICA_FILE_NAS` con i motivi `scritto_sul_nas` e `documento_confermato`, solo SELECT; `ListAttiveDelComponenteR5`:
    le AR attive di un componente con l'allegato e la lettura conservata; `InsertCorrezioneRicodificaR5`: la traccia
    `ricodifica_bom` per AR; `RicodificaComponenteR5`: il codice nuovo solo se e' ancora quello della decisione;
    `RipristinaComponenteR5`). Le chiama `core/rfq/distintabozza` (`riepilogo3_lettura.go`, `conferma3_scritture.go`); le
    provano le L4 `c39_r5_ricodifica_db_test.go` e `c39_r5_completo_db_test.go`. Generate con la v1.31.1 della cache dei
    moduli: nasce solo `v1_r5_completo.sql.go`, cambia `v1_ingressi.sql.go` (sotto; quattro `.sql.go` riscritti con soli
    fine riga diversi: ripristinati). `ListAttiveDelComponenteR5` porta l'UUID nullo per la proposta assente (sqlc non vede la
    nullabilita' del LEFT JOIN LATERAL)
  - In `v1_ingressi.sql`, `IngressiProposta` ha un elenco in piu' (2-ter): `escludi_codice_componenti`, i componenti che
    R5 ricodifica: il codice delle loro copie legacy senza lettura conservata non entra (lo riscrive `AllineaCodiceCopia`);
    R1 passa un elenco vuoto
- **`v1_proposta.sql`** — V1, ondata C (corsia persistenza): il rifiuto della proposta aggiornata (R15, U2 §5.7), la tabella in sola aggiunta della 0023 (riga 9)
  - Tabelle: `proposta_aggiornata_rifiuto`, `utente`
  - Query: 2 (`InsertPropostaAggiornataRifiuto`, che chiama `core/rfq/distintabozza` (`rifiuto.go`); `ListRifiutiPropostaProdotto`, per `/dati` della corsia motore: senza chiamanti in produzione al primo commit). Generate con la v1.31.1 della cache dei moduli: nasce solo `v1_proposta.sql.go`
- **`v1_file.sql`** — il ciclo dei file della Sorgente (V1, ondata B, corsia file; EM §10): le occorrenze e la loro fotografia, il perimetro di un archivio, il modello di S1, le dipendenze D-1…D-15 in una passata, l'unica copia (D-14) in blocco, le righe delle rimozioni e delle copie di conservazione prese `FOR UPDATE` o `SKIP LOCKED` (il custode, che le scorre a pagine per chiave: `ListIdRimozioniDaRiprendere`, `ListIdRimozioniNonRitentabili`, `ListIdConservazioniDaRiprendere` con `dopo`), i pin del tempo 2 per file, le guardie degli scrittori legacy (`StatoOccorrenzaAllegato`), l'elenco `NonAttivi` (`ListOccorrenzeNonAttiveRfq`). Lo scarto col vecchio «Scarta il file» (`scartato_legacy`, e lo stato «scartata» delle altre occorrenze) ha la stessa definizione in ogni query: proposta scartata da una persona, nessuna riga o la riga in attivo nella RFQ dell'occorrenza, nessun evento `scarta` del ciclo per l'allegato
  - Tabelle e viste: `file_rfq`, `file_rfq_evento`, `file_rfq_rimozione`, `file_rfq_conservazione`, `esito_intento`, `allegato`, `messaggio`, `documento_proposta`, `documento`, `documento_provenienza`, `componente`, `componente_proposta`, `job`, `annotazione_pdf`, `nas_anomalia`, `fonte_step_evento`, `conferma_strutturale`, `utente`
  - Query: 49 (direttiva 39, rilievi della revisione alla corsia file: + `DocumentoStepDelFinito` e `ListAssociazioniAttiveDelDocumento`, le guardie dei gesti documentali legacy nelle RFQ formato 3 in `core/rfq/fascicolo/formato3_legacy.go`); le chiamano `core/rfq/filerfq` (le scritture del ciclo solo da li': prova statica CO-12), le guardie di `transport/workerapi`, `transport/web` e `core/rfq/documenti`, e il flusso F8 (`ListOccorrenzeNonAttiveRfq`)
  - Lo scarto col vecchio «Scarta il file» (`scartato_legacy`): proposta scartata da una persona e nessuna riga del ciclo dei file per l'allegato in nessuna RFQ
- Il predicato `file_rfq_attivo` (V1, R-4 della persistenza, EM §10.7) nelle letture e nei cambi dello staging fuori dalle viste: `proposte.sql` (le tre query della struttura, `FileNelFlusso`), `analisi.sql` (la distribuzione delle letture), `preparazione.sql` (la preparazione e i file fermi; non `ListLavoroPendenteRfq`, che legge il caricatore del motore), `cache.sql` (i pin), `allegati.sql` (il gemello attivo prima, `AllegatoInStagingPerHash`)
- **`panoramica.sql`** — pagina Richieste
  - Tabelle e viste: `v_cruscotto`, `identificativo_thread`, `componente`, `documento`, `documento_proposta`
  - Query: 3; senza chiamanti: 0
  - Chi le chiama: `transport/web`
- **`preparazione.sql`** — preparazione automatica del Fascicolo (B8.7b)
  - Tabelle e viste: `allegato`, `messaggio`, `documento_proposta`, `job`
  - Query: 4; senza chiamanti: 0
  - Chi le chiama: `core/rfq/fascicolo`, `transport/web`
- **`proposte.sql`** — proposte di struttura dagli STEP (B8.5)
  - Tabelle e viste: `componente_proposta`, `relazione_proposta`, `rimozione_proposta`, `analisi_fatti`, `analizzatore_corrente`
  - Query: 25; senza chiamanti: 0
  - Chi le chiama: `core/rfq/fascicolo`, `transport/web`, `transport/workerapi`
- **`richieste.sql`** — richieste d'offerta ai fornitori (7B)
  - Tabelle e viste: `richiesta_fornitore`, `candidato_richiesta`, `messaggio`, `proposta_triage`, `bozza`
  - Query: 22; senza chiamanti: 4
  - Chi le chiama: `core/inbox/aggancio`, `core/inbox/ingest`, `transport/web`
- **`scarti.sql`** — scarti dell'ingest, log degli agganci
  - Tabelle e viste: `ingest_scarto`, `messaggio_aggancio_log`
  - Query: 8; senza chiamanti: 1 (+1)
  - Chi le chiama: `core/inbox/ingest`, `transport/web`
- **`schermata.sql`** — letture e piccoli gesti della schermata del Fascicolo
  - Tabelle e viste: `allegato`, `documento_proposta`, `documento_provenienza`, `componente_proposta`, `relazione_proposta`, `deroga_*`, `componente`, `messaggio` (nota interna)
  - Query: 15; senza chiamanti: 0
  - Chi le chiama: `core/rfq/fascicolo`, `transport/web`
- **`struttura.sql`** — BOM working: archiviazione, STEP strutturale, deroga strutturale, archi
  - Tabelle e viste: `componente`, `componente_relazione`, `deroga_struttura`, `rimozione_proposta`, `v_step_prodotto`
  - Query: 18; senza chiamanti: 0
  - Chi le chiama: `core/rfq/fascicolo`, `transport/web`
- **`thread.sql`** — RFQ, identificativi, componenti, fasi
  - Tabelle e viste: `thread_offerta`, `v_cruscotto`, `identificativo_thread`, `componente`, `fase_log`, `v_thread_fase`, `v_thread_da_riesaminare`
  - Query: 28; senza chiamanti: 6
  - Chi le chiama: `core/rfq/fascicolo`, `transport/web`, `core/rfq/documenti`
- **`utenti.sql`** — utenti e sessioni
  - Tabelle e viste: `utente`, `sessione`, `postazione`
  - Query: 11; senza chiamanti: 1
  - Chi le chiama: `transport/web`, `platform/fondazioni`, `platform/coda`
- **Alcuni file ospitano query di un altro dominio:**
  - `interpretazione.sql` mescola portale, deroghe, triage e bozze
  - `cache.sql` ha `UltimoJobPerChiave` (coda)
  - `fornitori.sql` ha `ContaAnagrafiche`
  - `richieste.sql` ha `SetAttoTriage`, `InsertBozzaRichiesta`, `SetBozzaInviata`, `RiproponiAllegatiComeOffertaFornitore`
  - `schermata.sql` ha `UpsertNotaInterna` (un `messaggio`) e `ContaAnalisiInCorso` (`job`)

## Entry point

- **`New(DBTX) *Queries` e i 504 metodi delle query**
  - Chi lo usa: `ai/agente`, `app/runtime`, `core/inbox/aggancio`, `core/inbox/ingest`, `core/registro/anagrafica`, `core/registro/fornitori`, `core/rfq/documenti`, `core/rfq/fascicolo`, `platform/coda`, `platform/fondazioni`, `platform/storage/staging`, `transport/web`, `transport/workerapi`; da A1c `core/fotorfq/caricatore` (solo `db.New(tx)`, in sola lettura); da A1d-P4-B1 `core/rfq/distintabozza` (le query della bozza)
- **i tipi enum (`RuoloUtente`, `AllRuoloUtenteValues`…)**
  - Chi lo usa: anche `platform/config` (validazione dei ruoli di `[[utenti]]`) e il test L3 di `platform/contratti/worker` (i valori dei contratti contro gli enum)
- **i modelli (`db.Messaggio`, `db.Documento`, `db.VInbox`…)**
  - Chi lo usa: tutti i package sopra, e i template di `transport/web` attraverso i dati delle pagine
- Il package non ha un'interfaccia `Querier` (`emit_interface` non è acceso):
  - chi chiama dipende da `*db.Queries`, e le prove passano da un database vero (L4).

## Dati

- Tutte le tabelle e le viste dello schema, ciascuna letta o scritta dalle query del suo file.
- Il package non apre transazioni e non tiene stato.

### Lucchetti presi dalle query

- valgono dentro la transazione di chi chiama
- **`BloccaMessaggio`, `BloccaThread`, `BloccaComponente`, `BloccaDocumento`, `BloccaProposta`, `BloccaComponenteProposta`, `BloccaRelazioneProposta`, `BloccaRimozioneProposta`, `ListDocumentiComponente`, `BloccaDistintaBozza`**
  - Lucchetto: `FOR UPDATE` sulla riga (o sulle righe)
- **`BloccaTentativo`, `BloccaCopiaPendente`, `BloccaSpostamentoPendente`**
  - Lucchetto: `FOR UPDATE` sul job; un job `pronto` bloccato viene saltato dal claim
- **`ClaimJob`**
  - Lucchetto: `FOR UPDATE SKIP LOCKED LIMIT 1` dentro un solo `UPDATE`
- **`BloccaCartella`**
  - Lucchetto: `pg_advisory_xact_lock(hashtextextended('<thread>:<cartella in minuscolo>', 0))`
- **`BloccaContenuto`** (V1, `v1_formato3.sql`)
  - Lucchetto: `pg_advisory_xact_lock(hashtextextended('contenuto:<sha256 in minuscolo>', 0))`: chi cancella un contenuto dallo staging e chi lo adotta, nella transazione che scrive `path_staging`, dopo le righe; piu' sha in ordine crescente
- **`BloccaPropostaDiAllegato`, `GetRimozionePerAggiornare`, `GetConservazionePerAggiornare`, `ListConservazioniVive`** (V1, `v1_file.sql`)
  - Lucchetto: `FOR UPDATE`; **`BloccaAllegatiPerFotografia`** `FOR SHARE`; le versioni `...SeLibera` del custode e `AnnullaJobPendentiPerChiave` `FOR UPDATE SKIP LOCKED`
- I trigger della 0020 prendono i loro (`FOR UPDATE`, `FOR SHARE`, `FOR KEY SHARE`): vedi `migrations/README.md`.

### Confronta e scambia

- Le `:execrows` con lo stato nel `WHERE` restituiscono 0 righe quando lo stato è cambiato nel frattempo
  - e il chiamante lo tratta come un esito, non come un errore:
    - `DecidiProposta`
    - `DecidiComponenteProposta`
    - `CongelaBomVersione`
    - `EliminaBozza`
    - `SetSostituitoDa`
    - `AnnullaSostituzione`
    - `SetDocumentoScritto` (vale solo per il percorso passato)
    - `SetPathDocumentoInCoda`
    - `PrendiSyncAperturaInbox`
    - `AbbandonaDistintaBozza` (stato `aperta` e versione attesa nel `WHERE`)
    - e le sei query con il predicato di validità del tentativo (`HeartbeatJob`, `CompletaJob`, `FallisciJob`, `RinviaJob`, `BloccaTentativo`, `VerificaTentativo`, commento in testa a `queries/job.sql`).

### Il perimetro sta nel `WHERE`

- Quando l'id arriva da un form o da un indirizzo, la query porta anche la riga a cui deve appartenere, e fuori da quella tocca 0 righe:
  - `EliminaBuyer` (il buyer del cliente della pagina, `cliente_id`; e non cancella un buyer citato da un messaggio, da una RFQ o da una proposta di triage, `proposta_triage.buyer_proposto`)
  - `DeleteDeroga` e `DeleteDerogaStruttura` (la deroga della RFQ, `thread_id`)
  - tutte le query di `p4_conferme.sql` che leggono una bozza, una conferma, un esito, un prodotto o un allegato (la RFQ,
    `thread_id`).
- Due scritture non scavalcano uno stato più forte:
  - `SetDocumentoErrore` non tocca un documento già `scritto`
  - `PropostaDocumentoDaRadice` non riscrive una proposta con `fonte = 'operatore'`.
- `ThreadPerChiaviCitate` (R0) filtra anche `canale = 'outlook'`, così usa l'indice unico `(canale, chiave_esterna)`.

### Upsert su indici parziali o su espressioni

- `InsertJob` (`ux_job_chiave_pendente`: un solo job pendente per chiave)
- `ApriAnomaliaNas` (una aperta per documento)
- `InsertNasOrfano` (una aperta per `(thread, lower(percorso))`)
- `InsertComponente` (`(thread_id, upper(codice))`).
- La clausola `ON CONFLICT … WHERE` deve ripetere il predicato dell'indice alla lettera.

### Vincoli differiti

- `SetCodiceComponente` funziona solo in una transazione che ha eseguito `SET CONSTRAINTS fk_documento_componente, fk_proposta_componente DEFERRED` (lo fa `transport/web/fascicolo.go`)
  - perché documenti e proposte si allineano al codice nuovo nelle `UPDATE` successive.

### Errori dei trigger

- Le scritture sulla BOM possono fallire con gli SQLSTATE `BOM01`–`BOM05` della 0020
- li traduce `transport/web/fascicolo.go`.
- Dalla 0022 (P4) anche `P4X01`–`P4X07` (conferma, registri in sola aggiunta, esiti e F8, ciclo della bozza,
  contatore: commento in testa a `migrations/0022_p4_conferme_working.sql`); per la bozza li dice prima, con i suoi
  codici, `core/rfq/distintabozza`.

## Flussi principali

### Rigenerare il codice

- Dalla cartella `cockpit/` (dove sta `sqlc.yaml`):

  ```
  sqlc generate      # riscrive models.go, db.go e ogni <nome>.sql.go
  sqlc diff          # confronta senza scrivere: dice se il codice generato è indietro
  ```

- sqlc legge come schema tutti i file di `migrations/` in ordine di nome e le query di `queries/`.
  - i blocchi `DO` li salta: per questo la 0018 crea le sue tabelle temporanee dentro un `DO`
- Il codice di questo ramo è generato con sqlc v1.31.1 (intestazione dei file)
  - `README.md` chiede la 1.31 o successiva
  - `scripts/prova-tutto.ps1` lo cerca nel `PATH` o in `%LOCALAPPDATA%\cockpit_rfq_test\sqlc\`.
  - A1d-P4-B1: `p4_conferme.sql.go` e il `models.go` della 0022 sono generati con la stessa v1.31.1, compilata
    dalla cache dei moduli fuori dal `PATH`; `sqlc generate` su una copia ha cambiato solo `models.go` (15 struct
    nuove) e ha aggiunto `p4_conferme.sql.go`.
  - A1d-P4, emendamento della direttiva 11: la stessa v1.31.1 su una copia ha cambiato `models.go` (3 struct nuove,
    `CoperturaDocumento`, `DistintaBozzaRegistro`, `SchemaVariante`; 4 campi nuovi in `FonteStepEvento`; `CoperturaID`
    in coda a `VFascicolo`; i commenti) e la sola `ListFascicolo` di `fascicolo.sql.go` (`SELECT *` su `v_fascicolo`:
    `copertura_id` in coda); `copertura_id` e' `NullUUID` per l'override di `sqlc.yaml`.
  - V1 (0023): la stessa v1.31.1, compilata offline dalla cache dei moduli (un modulo di appoggio fuori dal worktree,
    `GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off`), su una copia: rigenerare l'albero senza cambiamenti dava gli stessi
    file byte per byte (a meno dei fine riga); con la 0023 cambiano `models.go` (5 struct nuove, le colonne nuove e i
    commenti) e `p4_conferme.sql.go`, e nasce `v1_formato3.sql.go`. sqlc non conosce `unnest` a piu' argomenti: le
    coppie (sha256, chiave) si scrivono con due `unnest` nella lista del SELECT, che PostgreSQL scorre insieme.
  - V1, ondata B: la stessa v1.31.1 su una copia; nasce `v1_bozza_rotte.sql.go`, nient'altro cambia. sqlc non vede la
    nullabilita' delle colonne di un LEFT JOIN LATERAL: `GetStatiCorrentiFile` le porta con `COALESCE`. Nel secondo
    passaggio la query `OraDelDatabase` cambia solo `v1_bozza_rotte.sql.go`.
  - V1, corsia motore (ondata B): la stessa v1.31.1, su una copia fuori dal worktree; nasce solo `v1_motore.sql.go`
    (le tre query non hanno tabelle nuove: `models.go` non cambia).
  - V1, direttiva 35 (blocco C-1b, variante `20261019a` della 0023): la stessa v1.31.1 (l'eseguibile di
    `%LOCALAPPDATA%\cockpit_rfq_test\sqlc\`, che sull'albero senza cambiamenti dava `sqlc diff` pulito); cambia solo
    `models.go`: `FileRfqAssociazione` e `VArBackfillDiagnostica` nuove, quattro campi in coda ad `AssociazioneEvento`,
    uno a `CorrezioneDocumento`, i commenti. Nessuna query nuova in questo blocco (le query delle AR sono del servizio
    unico, C-1c, in un file proprio).
  - V1, direttiva 39 (variante `20261019c` della 0023): la stessa v1.31.1 di `%LOCALAPPDATA%\cockpit_rfq_test\sqlc\`;
    cambiano `models.go` (`RfqFormato3Segno` nuova; `ProvenienzaPb` e `ImportazioneID` in `FileRfqAssociazione`;
    `AssociazioneEvento.AutoreID` diventa `NullUUID`, perche' l'autore manca solo con `conversione_legacy`; i commenti),
    `v1_associazioni.sql.go` (`InsertAssociazione` con la provenienza e il registro d'origine), `v1_preassociazioni.sql.go`
    (`ListAllegatiPreassociabili` senza l'esclusione degli STEP e con `legato_come_sorgente`), `v1_sorgente_s8.sql.go`
    (`RfqFormato3` legge `rfq_e_formato3`) e `v1_formato3.sql.go` (`GetAssociazioneEventoPerChiave` con l'autore
    `NullUUID`). Gli INSERT del giornale (`InsertEventoAssociazione`, `InsertAssociazioneEvento`) tengono l'autore non
    nullo con il cast `::uuid`: i chiamanti delle altre corsie non cambiano. `v1_file.sql.go` e `v1_motore.sql.go`
    riscritti con soli fine riga diversi: ripristinati.
  - V1, direttiva 39, rilievi della revisione alla corsia file: la stessa v1.31.1 di
    `%LOCALAPPDATA%\cockpit_rfq_test\sqlc\`; cambia solo `v1_file.sql.go` (`DocumentoStepDelFinito`,
    `ListAssociazioniAttiveDelDocumento`); gli altri file generati riscritti con soli fine riga diversi: ripristinati.
- **Dopo:** `go build ./...`, `go vet ./internal/platform/db/`, e le prove L4 dei package che usano le query toccate.

### Aggiungere una query

1. sceglierne il file per dominio (tabella sopra), o crearne uno nuovo in `queries/`;
2. `-- name: <Nome> :one|:many|:exec|:execrows` e, subito dopo, i commenti che diventano la documentazione Go del metodo;
   - i commenti prima di `-- name:` restano nel file e basta;
3. parametri: `$n`, oppure `sqlc.arg(nome)` / `@nome`, e `sqlc.narg(nome)` quando il valore può essere NULL;
   - un cast (`::uuid[]`, `::text`, `::stato_job`) fissa il tipo che sqlc non inferisce;
   - `sqlc.embed(t)` mette la struct della tabella dentro la riga;
4. se si legge una vista con `COALESCE`, `LEFT JOIN LATERAL` o `array_agg`, sqlc sbaglia la nullabilità:
   - si corregge con un `overrides` per colonna in `sqlc.yaml`, con il motivo accanto;
5. `sqlc generate`, poi la chiamata:
   - `db.New(pool).<Nome>(ctx, …)` fuori da una transazione,
   - `db.New(tx).<Nome>(ctx, …)` dentro.

### Una colonna o una tabella nuova

- Prima la migrazione (`migrations/README.md`), poi `sqlc generate`:
  - cambiano `models.go` e ogni riga che usa `SELECT *` o `RETURNING *` su quella tabella.

## Invarianti

- I 36 file `.go` portano l'intestazione `Code generated by sqlc. DO NOT EDIT.`.
- Le query di `queries/` e i metodi di `*Queries` (salvo `WithTx`) sono gli stessi 504 nomi
  - ogni `queries/<nome>.sql` è stato committato insieme al suo `<nome>.sql.go`
  - e l'ultimo commit che tocca `migrations/` è anche l'ultimo che tocca `models.go`.
- **Tipi:**
  - `uuid` → `uuid.UUID` / `uuid.NullUUID`
  - `timestamptz` e `date` → `time.Time` / `*time.Time`
  - `jsonb` → `json.RawMessage` / `*json.RawMessage`
  - `inet` → `*netip.Addr`
  - `numeric` → `pgtype.Numeric`
  - gli array vuoti tornano come slice vuote (`emit_empty_slices`).
- La nullabilità delle colonne di `v_fascicolo`, `v_step_prodotto`, `v_bom_versioni`, `v_documento_storia`, `v_thread_da_riesaminare`, `v_cruscotto`, `v_inbox`, `v_componente_albero` è quella di `sqlc.yaml`, non quella che sqlc dedurrebbe.
- `rename` in `sqlc.yaml` corregge i singolari latini di sqlc (`documento_propostum` → `DocumentoProposta` e simili).
  - Non copre `candidato_richiesta` e `nas_anomalia`: i modelli si chiamano `CandidatoRichiestum` e `NasAnomalium`.
  - Dalla 0022 copre `deroga_fabbisogno_storia` (`DerogaFabbisognoStoria`, R11 della revisione SQL della 0022) e
    `deroga_struttura_storia` (`DerogaStrutturaStoria`).
- L'SQL applicativo fuori da `queries/` è poco e noto:
  - `SET CONSTRAINTS … DEFERRED` (`transport/web/fascicolo.go`)
  - `min(ricevuto_il)` di una casella e le letture di `riferimento_portale` e `bozza` per il pannello del messaggio (`transport/web/routes_inbox.go`)
  - `max(versione)` per `/healthz` (`transport/web/server.go`)
  - il `FOR KEY SHARE` sulla RFQ prima di bloccare la proposta da confermare (la RFQ per prima, come negli altri gesti sul Fascicolo: `transport/web/allegati.go`)
  - il lucchetto consultivo `cartella_rfq:<nome>` e la ricerca di una cartella NAS già presa da un'altra RFQ, che fa nascere la seconda con « (2)» (`transport/web/triage.go:cartellaLibera`)
  - il migratore esegue i file di `migrations/`.

## Dipendenze

- Importa solo librerie: `pgx/v5` (`pgx`, `pgconn`, `pgtype`), `google/uuid`, `encoding/json`, `net/netip`, `time`.
- È importato dai 14 package elencati in «Entry point» (compreso `platform/config` per gli enum).
- Nessuna violazione della tabella di `internal/README.md`:
  - `ai/*`, `core/*`, `transport/*`, `app/*` possono importare `platform`.

## Test

- Nessun file di prova nel package: il codice è generato.
- Lo provano:
  - **L1**
    - Dove: `platform/migrazioni/migrazioni_test.go`
    - Che cosa: lo schema che sqlc legge passa la verifica statica (versioni, `REFERENCES`, enum)
  - **L3**
    - Dove: `platform/contratti/worker`
    - Che cosa: i valori dei contratti coincidono con gli enum generati (`Direzione`, `NaturaAllegato`…)
  - **L4**
    - Dove: `*_db_test.go` di `core/inbox/ingest`, `core/rfq/documenti`, `core/rfq/fascicolo`, `core/rfq/distintabozza` (le 15
      query della bozza, per nome), `platform/migrazioni` (`p4_0022_db_test.go`: le conferme e gli esiti di `p4_conferme.sql` e le
      query legacy che scrivono la working), `core/registro/*`, `platform/coda`, `platform/fondazioni`, `platform/storage/staging`, `transport/web`, `transport/workerapi`, `app/runtime`
    - Che cosa: le query contro PostgreSQL vero, attraverso le funzioni che le usano
- Delle 387 query con un chiamante in produzione, 97 sono chiamate per nome anche dai test
  - le altre sono provate, quando lo sono, attraverso le funzioni che le usano.
- Ogni prova che apre il database porta il tag `integrazione`
  - anche `core/inbox/ingest/ingest_test.go`, che non ha `_db` nel nome
  - `go test ./...` senza tag (L1) non tocca il database nemmeno con `COCKPIT_TEST_DSN` impostata.
- Con il tag, `platform/testutil` rifiuta un DSN il cui database non ha «test» nel nome
  - e il nome lo legge come lo leggerà pgx (`testutil.go:DatabaseDiTest`, con `pgconn.ParseConfig`: vale anche per un DSN chiave=valore, per `?dbname=` e per un DSN senza database)
  - perché le prove distruggono e ricreano lo schema.

## Stato dell'implementazione

- 387 query hanno un chiamante in produzione (da A1c anche le sei di `fotografia.sql`, chiamate dal caricatore).
- Le altre 47 non ne hanno nessuno, e 10 sono chiamate solo dai test.
- Raggruppate per motivo:
  - **funzione non ancora costruita**
    - **anagrafica «Altro» senza schermata:** `ListSoggettiAltro`, `GetSoggettoAltro`, `GetSoggettoAltroPerEtichetta`, `ListRecapitiAltro`, `SetRecapitoAltroAttivo`
      - solo test: `InsertSoggettoAltro`, `InsertRecapitoAltro`
    - **spostamento sul NAS (B8.8):** `SetPathDocumentoSeUguale`, `ListNasOrfaniAperti`, `RisolviNasOrfaniDelJob`, `InsertNasCreazione`, `GetNasCreazione`
      - solo test: `InsertNasOrfano`, `RisolviNasOrfano`
    - **lettura di una versione congelata:** `GetUltimaCongelata`, `ListBomComponenti`, `ListBomRelazioni`, `ListBomDocumenti`, `ListBomDeroghe`
    - **regole che si spengono da sole:** `ListRegole`, `IncrementaRegola` (la tabella `regola` non viene mai popolata)
    - **transizioni:** `ListTransizioniDa`
    - **portale:** `ListRiferimentiPortaleThread`, `SetRiferimentoPortaleStato`
    - **sgancio di un messaggio:** `SgangiaMessaggio`
    - **fattibilità:** `SetNoteComponente`
    - **RFQ:** `SetCartellaThread`, `SetScadenzaThread`, `GetFaseCorrente`, `ListFaseLog`
    - **buyer:** `UpdateBuyer`
    - **P4 (B2-B3), solo test:** `ListDistinteBozzeProdotto`, `ListConfermeStrutturali`, `GetUltimaConfermaStrutturale`,
      `ListConfermaStrutturaleComponenti`, `ListConfermaStrutturaleArchi`, `InsertEsitoIntento`, `GetEsitoIntento`,
      `GetWorkingContatore`
  - **tolte di proposito dalla schermata**
    - richieste ai fornitori a livello di RFQ (blocco 8, `internal/README.md`): `ListRichiesteThread`, `ListRichiesteFornitore`, `ContaRichiesteAperte`, `SetAttoTriage`
  - **senza chiamante e senza un motivo scritto**
    - `GetUtente`, `ThreadDellaConversazione`, `ThreadPerCodiceCliente`, `ListProposteAperteThread`, `ListProposteBulk`, `EliminaIngestScarto`, `ListAnalisiMessaggio`, `ContaAnalisiPerStato`, `ContaCandidatiAggancio`, `ListAllegatiSenzaStaging`, `GetConvenzione`, `ListCredenzialiOutlookAttive`, `GetBozza`, `EsisteJobPronto`, `GetPresenza`, `ListMessaggiConversazione`
  - **solo test, lettura di verifica**
    - `GetWorkerCredenziale`, `GetCasellaStore`, `ListCasellaStorePerPostazione`, `GetWorkerPresenza`, `GetSyncCursore`, `ListAgganciaLog`
- `BloccaSpostamentoPendente` ha un chiamante (`core/rfq/documenti/nomi_nas.go:AccodaSpostamento`)
  - ma `AccodaSpostamento` lo chiamano solo le prove
  - finché B8.8 non arriva, il job `sposta_nas` non ha un esecutore in `app/runtime` (l'esecutore del server chiude un tipo che non gestisce come fallimento definitivo).
- **Per rifare il conto** dalla cartella `cockpit/`: il comando elenca le query senza chiamanti in produzione (A1c: 57, cioè le 47 e le 10; dopo A1d-P4-B1 e le correzioni del 13/10 il comando ne elenca 62, fra cui le 7 di `p4_conferme.sql`
  chiamate solo dalle prove).
  - Sei nomi coincidono con funzioni di altri package (per esempio `fascicolo.ArchiviaComponente`)
  - e il comando li conta come usati anche quando a chiamarli è solo quella funzione: vanno guardati a mano.

  ```
  grep -h -- '-- name:' internal/platform/db/queries/*.sql | awk '{print $3}' | while read n; do
    grep -rqE "\.$n\(" --include='*.go' --exclude='*_test.go' --exclude-dir=db internal cmd || echo "$n"
  done
  ```

## Dove intervenire

- **una query nuova**
  - Apri: `queries/<dominio>.sql`, poi `sqlc generate`
- **cambiare il tipo Go di una colonna o la nullabilità di una vista**
  - Apri: `sqlc.yaml` (`overrides`), poi `sqlc generate`
- **rinominare un modello con un singolare sbagliato**
  - Apri: `sqlc.yaml` (`rename`), poi i chiamanti
- **una colonna o una tabella nuova**
  - Apri: una migrazione (`migrations/README.md`), poi `sqlc generate`
- **capire perché una scrittura sulla BOM è rifiutata**
  - Apri: i trigger della 0020 (`BOM01`–`BOM05`) e `transport/web/fascicolo.go`
- **capire che cosa fa un lucchetto**
  - Apri: la query `Blocca*` in `queries/` e il commento sopra
- **sapere se il codice generato è in pari**
  - Apri: `sqlc diff` dalla cartella `cockpit/`

## Leggi anche

- `migrations/README.md`
- `internal/platform/README.md`
- `internal/README.md`
- `README.md` («Manutenzione»)
- `sqlc.yaml`
