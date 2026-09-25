# `migrations/` — lo schema PostgreSQL del Cockpit, un file per versione

## Scopo

Lo schema del database in file numerati `NNNN_nome.sql`, dalla 0001 alla 0021. Il binario li porta
dentro di sé (`embed.go`) e li applica da solo all'avvio, in ordine, uno per transazione
(`internal/platform/migrazioni`). Gli stessi file sono lo schema che sqlc legge per generare
`internal/platform/db`: una migrazione nuova è anche codice Go nuovo.

Stato alla 0021 (l'ultima): 69 tabelle, 11 viste, 47 enum, 14 funzioni, 21 trigger. PostgreSQL 16 o
successivo, come dice il README principale: le prove L4 girano sulla 16 (`scripts/db-test.ps1`), e
`UNIQUE NULLS NOT DISTINCT` vuole almeno la 15.

## Non appartiene qui

- Il ritorno indietro: il migratore non ha il verso «giù». Il ritorno è il backup
  (`scripts/backup-db.ps1`) più il binario di prima; per la 0018 e la 0021 c'è anche uno script manuale
  in `scripts/` (sotto, «Tornare indietro»).
- Le query: `internal/platform/db/queries/`.
- La configurazione di un'installazione (utenti, caselle, postazioni, credenziali dei worker): la
  semina `platform/fondazioni` da `cockpit.toml` a ogni avvio.
- Le anagrafiche (clienti, fornitori): `-semina-anagrafica`, `-importa-fornitori` e la schermata Admin.
  Le migrazioni seminano solo i cataloghi: `fase_catalogo`, `transizione`, `cartella_documento`, i
  default di `fabbisogno_documento`, `lavorazione`, `atto_business`.

## File

| File | Blocco | Che cosa introduce |
|---|---|---|
| `0001_schema.sql` | piano 0.2 | lo schema iniziale: 31 enum, 30 tabelle (utenti e sessioni, anagrafica clienti, cataloghi, RFQ, messaggi e allegati, interpretazione, decisione, bozze, `job`, `worker_presenza`), i trigger `trg_stato_thread`, `trg_ultimo_messaggio`, `trg_n_allegati`, `trg_tocca_job`, le viste `v_thread_fase`, `v_fascicolo`, `v_thread_bloccanti`, `v_inbox`, `v_cruscotto`, `pgcrypto` |
| `0002_fondazioni.sql` | piano 0.6 | `casella`, `postazione`, `worker_credenziale`, `casella_store` |
| `0003_coda_ingest.sql` | piano fase 1 | il tentativo sul `job` (`lease_token`, `avviato_il`, `durata_max_s`, `casella_id`, `postazione_id`, `richiesto_da`, `scade_il`); `chiave_esterna` di `messaggio` e `conversazione` diventa `text`; `ingest_scarto`, `messaggio_aggancio_log`, `analisi_fatti`; valori `stato_job.annullato`, `tipo_job.rileggi_elemento` |
| `0004_caselle_presenza.sql` | piano 2.1 | `messaggio_casella` (una riga per copia) con il travaso da `messaggio_outlook`; `sync_cursore` per `(casella_id, cartella)`; `messaggio.interno`; `v_inbox` con le caselle. Si ferma se non sa a quale casella assegnare le righe esistenti |
| `0005_postazioni_presenza.sql` | piano 2.2, 2.6, 2.7 | `worker_presenza` riscritta per worker (`worker_nome`); `sessione.postazione_id`, `postazione_origine`; via `messaggio_casella.store_id_locale` |
| `0006_inbox_viva.sql` | addendum, blocco 1 | `utente.ultima_vista_inbox` |
| `0007_anagrafica.sql` | addendum, blocco 3 | `cliente.peso` (0–15), `CHECK` che `cliente.regole` sia un oggetto, enum `fonte_fabbisogno` e `fabbisogno_documento.fonte_attesa` |
| `0008_interpretazione.sql` | Checkpoint 3R | enum `regola_aggancio`, `ruolo_codice`, `origine_codice`, `stato_analisi`; `candidato_aggancio`, `candidato_codice`, `analisi_messaggio`; `thread_offerta.riferimento_cliente`; valori `tipo_documento.da_determinare`, `origine_identificativo.proposta_famiglia`/`proposta_generico`, `tipo_job.analizza_messaggio_ai` |
| `0009_presenza_contatto.sql` | 3R, blocco 2 | `worker_presenza.ultimo_contatto`; `ultimo_claim` diventa annullabile |
| `0010_copertura_sync.sql` | 3R, blocco 3 | `sync_cursore.coperto_fino_a` (la frontiera recente) |
| `0011_sync_apertura_inbox.sql` | 3R, blocco 3 | `sessione.sync_inbox_il` |
| `0012_estrai_archivio.sql` | 3R, blocco 4A | valore `tipo_job.estrai_archivio` |
| `0013_integrita_nas.sql` | 3R, blocco 5B | enum `problema_nas`, `documento.verificato_il`, `nas_anomalia` |
| `0014_fornitori.sql` | 7A | enum `tipo_fornitore`, `tipo_controparte`, `via_controparte`, `modo_convenzione`; `fornitore`, `dominio_fornitore`, `contatto_fornitore`, `lavorazione` (13 righe), `fornitore_lavorazione`, `cliente_fornitore_lavorazione`, `convenzione_codice`, `convenzione_codice_lavorazione`; la controparte su `messaggio` |
| `0015_richiesta_fornitore.sql` | 7B | enum `stato_richiesta_fornitore`, `regola_richiesta` (e `intento_messaggio`, tolto dalla 0017); `richiesta_fornitore`, `candidato_richiesta`; `messaggio.richiesta_fornitore_id`, `bozza.richiesta_fornitore_id`, `proposta_triage.richiesta_proposta`/`fornitore_proposto` |
| `0016_classificazione.sql` | 7C.0 | valore `tipo_controparte.altro`; `soggetto_altro`, `recapito_altro`, `messaggio.controparte_altro_id`; `risposta` → `offerta_ricevuta` e `declinata` sulla richiesta al fornitore (si ferma se ci sono richieste in `risposta`); `atto_business` (16 righe) e `proposta_triage.atto`; enum `legame_operativo` e `proposta_triage.legame` |
| `0017_controparte_altro.sql` | 7C.0 | il `CHECK` della controparte con `altro`; via `proposta_triage.intento` e l'enum; `v_inbox` con `triage_atto`, `triage_legame`, `quadrante` |
| `0018_fascicolo.sql` | B8.2 | `componente` diventa l'identità `(thread_id, upper(codice))` con fusione dei duplicati; `componente_relazione`, `componente_proposta`, `relazione_proposta`; FK composite con `thread_id` da `documento`, `documento_proposta`, `deroga_fabbisogno`; default nuovi del fabbisogno; `v_fascicolo` v2, `v_componente_albero`, `v_codici_candidati_thread`. 13 guardie |
| `0019_sposta_nas.sql` | B8.A4a | valore `tipo_job.sposta_nas`; enum `motivo_orfano`; `nas_orfano`, `nas_creazione` |
| `0020_bom_versioni.sql` | B8.A4a | la catena delle revisioni di `documento`; archiviazione e STEP strutturale di `componente`; `deroga_struttura`, `rimozione_proposta`, `bom_versione` e le quattro istantanee, `analizzatore_corrente`; enum `stato_bom`, `contesto_bom`; i trigger `BOM01`–`BOM05`; `struttura_motivo_parziale`, `struttura_completa`; `v_documento_storia`, `v_step_prodotto`, `v_bom_versioni`, `v_thread_da_riesaminare`; 14 transizioni. 5 guardie |
| `0021_annotazioni_pdf.sql` | Fascicolo v3 | `annotazione_pdf` |

## Lo schema per dominio

Fra parentesi la migrazione che ha creato la tabella e, dopo la freccia, quelle che l'hanno cambiata.

**Infrastruttura.** `schema_versione` (0001): una riga per migrazione applicata.

**Utenti e sessioni.** `utente` (0001 → 0006): `sigla` UNIQUE, `ruolo`, `password_hash` (NULL = niente
login). `sessione` (0001 → 0005, 0011): `token` char(64) PK; `postazione_id` e `postazione_origine`
(`ip` | `scelta`) insieme o tutti e due NULL (`ck_sessione_postazione_origine`).

**Postazioni e worker.** `casella` (0002): `(canale, indirizzo)` UNIQUE, indirizzo minuscolo, una
condivisa non ha utente. `postazione` (0002): `nome_host` UNIQUE e maiuscolo. `worker_credenziale`
(0002): `worker_nome` PK, `token_hash` sha256 esadecimale (il token non sta mai nel database),
`caselle uuid[]` autorizzate. `casella_store` (0002): lo StoreID di una casella nel profilo di una
postazione, PK `(postazione_id, casella_id)`. `worker_presenza` (0001, riscritta 0005 → 0009): una riga
per `worker_nome`, `ultimo_contatto` decide online/offline, `ultimo_claim` è diagnostica.

**Coda.** `job` (0001 → 0003): `ux_job_chiave_pendente` = al più un job `pronto`/`in_corso` per
`chiave_idempotenza`; il tentativo è `lease_token` + `avviato_il` + `durata_max_s`; `trg_tocca_job`
aggiorna `aggiornato_il`. `nas_creazione` (0019) segue il job (`ON DELETE CASCADE`), `nas_orfano`
(0019) gli sopravvive (`ON DELETE SET NULL`).

**Fatti: posta e allegati.** `conversazione` e `messaggio` (0001): `(canale, chiave_esterna)` UNIQUE;
`messaggio.thread_id` è la decisione di aggancio, mai dedotta; `data_evento` senza default (è il tempo
del fatto). Su `messaggio` si sono aggiunti `interno` (0004), la controparte (0014, 0016:
`controparte_tipo` più al massimo una fra `controparte_cliente_id`, `controparte_fornitore_id`,
`controparte_altro_id`, legate al tipo da `ck_messaggio_controparte`, 0017), `richiesta_fornitore_id`
(0015). `messaggio_outlook` (0001 → 0004): solo ciò che è del messaggio (`in_reply_to`, `riferimenti`).
`messaggio_casella` (0004 → 0005): una riga per copia, con `entry_id`, `cartella`, `ricevuto_il`,
`non_letto`. `sync_cursore` (0001 → 0004, 0010): PK `(casella_id, cartella)`, `coperto_fino_a` e
`storico_fino_a` sono le due frontiere. `allegato` (0001): `(messaggio_id, contenitore_id, indice)`
UNIQUE NULLS NOT DISTINCT, `contenitore_id` per le voci di un archivio, `sha256` indicizzato.
`hash_rumore` (0001), `ingest_scarto` (0003, una riga per `(casella_id, entry_id)`),
`messaggio_aggancio_log` (0003 → 0008).

**Anagrafica clienti.** `cliente` (0001 → 0007): `cartella_nas` UNIQUE, `regole` jsonb che deve essere un
oggetto, `peso` 0–15. `dominio_cliente` (0001): il dominio è la PK, cioè di un cliente solo. `buyer`
(0001): `email` UNIQUE e minuscola. `fabbisogno_documento` (0001 → 0007, 0018): risolto in blocco per
`tipo_componente` (le righe del cliente, se ne ha, altrimenti i default con `cliente_id` NULL).
`cartella_documento` (0001): la sottocartella sul NAS per tipo di documento.

**Fornitori e «Altro».** `fornitore` (0014, `lower(ragione_sociale)` UNIQUE), `dominio_fornitore`,
`contatto_fornitore` (`(fornitore_id, email)` UNIQUE), `lavorazione` (catalogo), `fornitore_lavorazione`
(che cosa sa fare), `cliente_fornitore_lavorazione` (la qualifica: FK composita verso
`fornitore_lavorazione`, quindi solo per una lavorazione che il fornitore fa), `convenzione_codice` e
`convenzione_codice_lavorazione` (suffisso ≤ 12 caratteri senza spazi, esempio obbligatorio,
controesempio diverso). `soggetto_altro`, `recapito_altro` (0016): un indirizzo o un dominio, minuscolo.

**Interpretazione (proposte, mai decisioni).** `proposta_triage` (0001 → 0015, 0016, 0017): UNIQUE
`(messaggio_id, fonte)`, con `atto` (FK verso `atto_business`, 0016) e `legame`. `candidato_aggancio`,
`candidato_codice` (0008), `candidato_richiesta` (0015): più righe per messaggio, nessuna scelta.
`analisi_messaggio` (0008): UNIQUE `(messaggio_id, input_hash, prompt, modello)`. `analisi_fatti` (0003):
PK `(sha256, versione_analizzatore, hash_configurazione)`. `analizzatore_corrente` (0020): una riga sola
(`unico` PK con `CHECK (unico)`). `documento_proposta` (0001 → 0018): una per allegato, FK composita
`(thread_id, componente_id, codice)` verso `componente`, e senza thread niente componente.
`riferimento_portale` (0001). `regola` (0001): nessuna riga, nessun codice la scrive.

**RFQ e fasi.** `thread_offerta` (0001 → 0008): `stato` è una cache scritta solo da `trg_stato_thread`,
`ultimo_aggiornamento` da `trg_ultimo_messaggio`. `identificativo_thread` (0001): PK `(thread_id,
codice)`. `fase_catalogo`, `transizione` (0001 → 0020). `fase_log` (0001 → 0020): una sola fase aperta
per RFQ (`ux_fase_aperta`), `bom_versione_id` solo su `SCHEDA_COSTO`. `richiesta_fornitore` (0015 →
0016): una per `(thread, fornitore, lavorazione)`. `bozza` (0001 → 0015).

**Fascicolo e BOM.** `componente` (0001 → 0018, 0020): l'identità è `(thread_id, upper(codice))`
(`ux_componente_thread_codice`), `confermato_da` obbligatorio, codice non vuoto; `step_strutturale_id`
solo per un `finito`; archiviazione tutta o niente (`ck_componente_archiviazione`).
`componente_relazione` (0018): PK `(padre_id, figlio_id)`, niente autoanelli, FK composite con
`thread_id` (due RFQ non si legano); i cicli li rifiuta il Go. `componente_proposta`,
`relazione_proposta` (0018 → 0020), `rimozione_proposta` (0020): proposte dai file, una decisa non si
riscrive. `documento` (0001 → 0013, 0018, 0020): UNIQUE `(thread_id, sha256)` e
`(thread_id, lower(path_relativo))`; un documento tecnico ha il codice; uno agganciato ha il codice del
suo componente (FK a tre colonne `DEFERRABLE`); la catena `sostituito_da` resta nella stessa RFQ, nello
stesso componente e nello stesso tipo, e cresce solo in fondo. `documento_provenienza` (0001),
`deroga_fabbisogno` (0001 → 0018, UNIQUE `(componente_id, tipo)`), `deroga_struttura` (0020, non si
modifica). `bom_versione` (0020): numeri senza salti, una sola bozza per RFQ, la V1 nasce solo in
`FATTIBILITA` (preventivo) o in `ORDINE`/`PRODUZIONE` (tecnica); le istantanee
`bom_versione_componente`, `bom_versione_relazione`, `bom_versione_documento`, `bom_versione_deroga`.
`annotazione_pdf` (0021): punto normalizzato 0..1, pagina 1–10000, testo fino a 2000 caratteri.

**Integrità del NAS.** `nas_anomalia` (0013): una aperta per documento. `nas_orfano` (0019): una aperta
per `(thread_id, lower(percorso))`. `nas_creazione` (0019).

### Enum

I più usati e quelli cambiati dopo la creazione:

| Enum | Valori | Migrazioni |
|---|---|---|
| `tipo_job` | `sync_outlook`, `stage_allegato`, `analizza_allegato`, `copia_nas`, `crea_cartella_thread`, `crea_bozza_outlook`, `apri_elemento_outlook`, `sposta_in_cartella`, `segna_letto`, `backup_db`, `rileggi_elemento`, `analizza_messaggio_ai`, `estrai_archivio`, `sposta_nas` | 0001, 0003, 0008, 0012, 0019 |
| `stato_job` | `pronto`, `in_corso`, `fatto`, `fallito`, `annullato` | 0001, 0003 |
| `worker_tipo` | `server`, `outlook`, `analisi` | 0001 |
| `tipo_documento` | `cad_3d`, `disegno_2d`, `sviluppo_dxf`, `capitolato`, `distinta_cliente`, `commerciale`, `offerta_fornitore`, `offerta_<azienda>` (la nostra offerta al cliente; il valore porta il nome dell'azienda), `ordine_cliente`, `corrispondenza`, `rumore`, `altro`, `da_determinare` | 0001, 0008 |
| `tipo_controparte` | `cliente`, `fornitore`, `interno`, `altro`, `sconosciuto`, `ambiguo` | 0014, 0016 |
| `stato_richiesta_fornitore` | `bozza`, `inviata`, `offerta_ricevuta`, `declinata`, `scaduta`, `annullata` | 0015, 0016 |
| `origine_identificativo` | `proposta_oggetto`, `proposta_corpo`, `proposta_nome_file`, `proposta_step`, `manuale`, `proposta_famiglia`, `proposta_generico` | 0001, 0008 |
| `fase` | `RICEVUTA` … `PRODUZIONE`, `PERSA`, `RESPINTA`, `SCADUTA` (13) | 0001 |
| `aggancio` | `nessuno`, `auto_conversazione`, `auto_identificativo`, `operatore` (i due `auto_*` non li produce più nessuno dalla 0008: nessuna query né codice Go li scrive) | 0001 |

Gli altri 38, tutti rimasti come sono nati: `ruolo_utente`, `tipo_buyer`,
`origine_anagrafica`, `canale`, `direzione`, `stato_thread`, `scadenza_origine`, `tipo_componente`,
`origine_componente`, `esito_fattibilita`, `esito_fase`, `natura_allegato`, `origine_allegato`,
`stato_allegato`, `fonte_proposta`, `stato_proposta`, `stato_rif_portale`, `esito_triage`,
`fonte_triage`, `stato_triage`, `stato_nas`, `tipo_bozza`, `stato_bozza`, `modo_regola` (0001);
`fonte_fabbisogno` (0007); `regola_aggancio`, `ruolo_codice`, `origine_codice`, `stato_analisi` (0008);
`problema_nas` (0013); `tipo_fornitore`, `via_controparte`, `modo_convenzione` (0014); `regola_richiesta`
(0015); `legame_operativo` (0016); `motivo_orfano` (0019); `stato_bom`, `contesto_bom` (0020). I valori
Go stanno in `internal/platform/db/models.go`.

Alcune classificazioni sono testo con un vincolo invece di un enum: `messaggio_aggancio_log.azione` e
`ingest_scarto.origine` (`CHECK`), `sessione.postazione_origine` (`CHECK`), `proposta_triage.atto` (FK
verso `atto_business`, scelta motivata nella 0016).

### Trigger e funzioni

| Trigger | Tabella | Che cosa fa | Migrazione |
|---|---|---|---|
| `trg_stato_thread` → `aggiorna_stato_thread()` | `fase_log` | `thread_offerta.stato` = `CHIUSA` se la fase aperta è terminale | 0001 |
| `trg_ultimo_messaggio` → `aggiorna_ultimo_messaggio()` | `messaggio` (INSERT, UPDATE OF `thread_id`) | `thread_offerta.ultimo_aggiornamento` al più recente fra i messaggi agganciati | 0001 |
| `trg_n_allegati` → `aggiorna_n_allegati()` | `allegato` | `messaggio.n_allegati` = allegati diretti `file` ed `elemento_outlook` | 0001 |
| `trg_tocca_job` → `tocca_job()` | `job` | `aggiornato_il = now()` | 0001 |
| `trg_documento_catena_revisioni` → `documento_catena_revisioni()` | `documento` | `BOM03`: un documento non nasce sostituito; si sostituisce solo con un successore corrente (letto `FOR UPDATE`); si annulla solo l'ultima sostituzione | 0020 |
| `trg_componente_step_strutturale` → `componente_step_strutturale()` | `componente` | `BOM04`: lo STEP strutturale è un `cad_3d` `.stp`/`.step` corrente (letto `FOR SHARE`) | 0020 |
| `trg_deroga_struttura_immutabile` | `deroga_struttura` | `BOM05`: nessun UPDATE | 0020 |
| `trg_bom_versione_immutabile` → `bom_versione_immutabile()` | `bom_versione` | `BOM02`: nasce bozza; congelata non cambia né si cancella; una bozza cambia solo per congelarsi | 0020 |
| `trg_bv{c,r,d,g}_immutabile` → `bom_istantanea_immutabile()` | le quattro istantanee | `BOM02`: niente scritture se la versione è congelata (lettura `FOR SHARE`) | 0020 |
| `trg_bv{c,r,d,g}_non_si_svuota` → `bom_istantanea_non_si_svuota()` | le quattro istantanee | `BOM02` su `TRUNCATE` | 0020 |
| `trg_bom_working_{componente,relazione,deroga,deroga_struttura,documento}` → `bom_working_modificabile()` | working | `BOM01`: con l'ultima versione congelata, niente scritture sulla BOM working; su `documento` solo i campi tecnici (lo stato del NAS, `nome_file` e `nota` restano liberi) | 0020 |

Funzioni senza trigger (0020): `bom_working_bloccata(uuid)` (prende `thread_offerta` `FOR KEY SHARE`,
restituisce il numero della versione che blocca), `struttura_motivo_parziale(jsonb)` e
`struttura_completa(jsonb)` (`IMMUTABLE`: perché una lettura dello STEP non è completa; il Go la chiede
con la query `MotivoParziale`).

### Viste

| Vista | Che cosa | Ultima definizione | Chi la legge |
|---|---|---|---|
| `v_thread_fase` | fase aperta di una RFQ con giorni e semaforo SLA | 0001 | `v_cruscotto` |
| `v_fascicolo` | una riga per (componente attivo, documento richiesto) con l'esito | 0020 | `v_thread_bloccanti`, `ListFascicolo` |
| `v_thread_bloccanti` | contatori del fascicolo per RFQ | 0018 | `v_cruscotto`, `GetBloccantiThread`, `GateCongelamento` |
| `v_cruscotto` | una riga per RFQ non fusa | 0018 | `ListRichiestePanoramica`, `CercaThreadAperti`, `GetCruscottoRiga` |
| `v_inbox` | un messaggio per riga con cliente, caselle, triage deterministico, controparte, quadrante | 0017 | `ListInbox`, `ContaInbox`, `ContaQuadranti`, `GetInboxRiga` |
| `v_componente_albero` | l'albero dai componenti radice, un percorso per riga | 0020 | nessuna query (solo le prove) |
| `v_codici_candidati_thread` | i codici visti da una RFQ, una riga per evidenza | 0018 | `ListCodiciCandidatiThread` |
| `v_documento_storia` | la catena delle revisioni di ogni documento | 0020 | `ListStoriaDocumento` |
| `v_step_prodotto` | lo STEP di ogni finito attivo e l'esito | 0020 | `ListStepProdotto`, `GetStepProdotto`, `ListGateStep`, `SnapshotComponenti` |
| `v_bom_versioni` | versioni con `superata` e `corrente` derivate | 0020 | `ListBomVersioni` |
| `v_thread_da_riesaminare` | che cosa va riguardato dopo una revisione della BOM | 0020 | `ListThreadDaRiesaminare` |

`v_cruscotto` dipende da `v_thread_bloccanti`, che dipende da `v_fascicolo`: cambiare le colonne di
`v_fascicolo` vuol dire togliere e ricreare le tre (la 0018 lo fa); `CREATE OR REPLACE VIEW` può solo
aggiungere colonne in coda (la 0014, la 0015 e la 0020 lo usano così).

## Flussi principali

**All'avvio del server.** `app/runtime/avvio.go:ApriDatabase` → `migrazioni.Applica(ctx, pool,
cockpit.FS, log)`, anche con `-migra` (che poi esce), `-semina-anagrafica` e `-importa-fornitori`. I
comandi che leggono soltanto (`-conta-anagrafiche`, `-anteprima-fornitori`) non migrano:
`app/runtime/avvio.go:ApriDatabaseInLettura` apre il database in sola lettura e, se lo schema non è
all'ultima migrazione del binario, si ferma con un errore che dice che cosa fare. `Applica`:

1. `Elenca` legge `migrations/*.sql` dal filesystem incorporato: il nome deve essere
   `^\d{4}_[a-z0-9_]+\.sql$` e le versioni devono essere 1..n senza buchi;
2. `VerificaStatica` controlla tutti i file senza database (sotto, «Invarianti»);
3. lucchetto consultivo di sessione `pg_advisory_lock(0x434f434b)`: due server non migrano insieme;
4. `Applicate` legge `schema_versione`; se il database è più avanti del binario si ferma («aggiornare
   cockpit.exe»); se `schema_versione` ha un buco si ferma («DB incoerente»);
5. per ogni file mancante: una transazione, il file intero, la verifica che la riga di
   `schema_versione` ci sia, il commit. Se qualcosa fallisce non resta niente di quel file.

**Nelle prove.** `platform/testutil`: `SchemaPulito` e `SchemaPresente` applicano tutto,
`SchemaFinoA(v)` ricrea lo schema e si ferma alla versione `v`, per mettere dati «di prima» e poi
applicare la migrazione da provare. Le prove che le usano portano il tag `integrazione`, e `testutil`
accetta solo un database che ha «test» nel nome, letto dal DSN come lo legge pgx
(`testutil.go:DatabaseDiTest`): lo schema si distrugge.

**Per sqlc.** `sqlc.yaml` ha `schema: "migrations"`: dopo ogni migrazione, `sqlc generate` dalla
cartella `cockpit/` (vedi `internal/platform/db/README.md`).

## Scrivere una migrazione nuova

1. **Il nome.** `migrations/NNNN_nome.sql` con il numero successivo all'ultimo (oggi `0022`), quattro
   cifre, minuscole e `_`. I numeri seguono l'ordine in cui le migrazioni si applicano, non quello dei
   blocchi del piano (0006, 0007).
2. **Niente da registrare in Go.** `embed.go` incorpora `migrations/*.sql`: il file entra nel binario
   alla prossima build, e il server lo applica al primo avvio.
3. **La versione.** Il file contiene `INSERT INTO schema_versione (versione) VALUES (N);` con il suo N,
   per convenzione come ultima istruzione. Senza, la verifica statica lo rifiuta e il migratore annulla
   la transazione.
4. **Una transazione.** Niente `BEGIN` né `COMMIT` nel file, niente `CREATE INDEX CONCURRENTLY` e niente
   altro che non possa stare in un blocco di transazione.
5. **Enum.** Un valore aggiunto con `ALTER TYPE … ADD VALUE` non si usa nello stesso file: PostgreSQL non
   lo accetta prima del commit. La verifica è testuale (il valore fra apici compare altrove nel file) e
   quindi prudente: se serve, due file (0016 aggiunge `altro`, 0017 lo usa). Un enum nuovo con
   `CREATE TYPE` si usa subito.
6. **Riferimenti.** Ogni `REFERENCES t` punta a una tabella creata con `CREATE TABLE t` in questo file o
   in uno precedente. La verifica cerca il testo `CREATE TABLE`: una tabella nata con
   `ALTER TABLE … RENAME TO` non la vede.
7. **Dati esistenti.** Una migrazione che sposta o reinterpreta righe comincia con le guardie: un blocco
   `DO` che conta, elenca al massimo 20 righe per guardia, le raccoglie tutte e poi `RAISE EXCEPTION` con
   un `HINT`, prima di qualunque modifica (0004, 0016, 0018, 0020). Nessuna guardia corregge da sola.
   Le tabelle di appoggio stanno in `CREATE TEMP TABLE … ON COMMIT DROP` dentro un `DO`, così sqlc non
   le scambia per tabelle vere (0018).
8. **Viste.** Colonne nuove in coda con `CREATE OR REPLACE VIEW`; altrimenti `DROP` e `CREATE` di tutta
   la catena che ne dipende. Per una vista con `COALESCE` o `LEFT JOIN LATERAL`, l'`overrides` di
   nullabilità in `sqlc.yaml`.
9. **Trigger che rifiutano.** Un codice SQLSTATE proprio (`BOM01`…), che il Go riconosce senza leggere
   il testo.
10. **Dopo.** `sqlc generate`; `go test ./internal/platform/migrazioni/` (L1, la verifica statica sulle
    migrazioni vere); una prova L4 nella stessa cartella che parta da `testutil.SchemaFinoA(N-1)` con dei
    dati, applichi `migrazioni.ApplicaFinoA(N)` e guardi il risultato (e le guardie, se ce ne sono).
11. **Un file applicato non si tocca più.** Il migratore non ricalcola niente di un file già registrato:
    la modifica non arriverebbe ai database che lo hanno già, e nessuno se ne accorgerebbe.
12. **Sul database vero.** Il server applica la migrazione da solo all'avvio: prima il backup
    (`scripts/backup-db.ps1`). Dopo, il binario di prima rifiuta di partire su quel database.

## Tornare indietro

Non esiste un verso «giù» nel migratore. Il ritorno è il backup preso subito prima, più il binario
precedente. Quando serve anche un ritorno manuale, si scrive `scripts/NNNN_indietro.sql` come la 0018 e
la 0021:

- **mai in `migrations/`**: il nome passerebbe la regola `NNNN_nome.sql`, il binario lo incorporerebbe
  come una seconda versione N e non partirebbe più;
- `BEGIN` e `COMMIT` nel file, da lanciare a mano con il server fermo;
- un blocco `DO` che controlla `max(versione) = N` e si ferma su ciò che andrebbe perso (0018: un
  componente con più padri; 0021: delle note, a meno di `SET LOCAL cockpit.perdi_le_note = 'true'`);
- gli oggetti tolti in ordine inverso di dipendenza, le viste di prima rimesse com'erano, e alla fine
  `DELETE FROM schema_versione WHERE versione = N`;
- una prova L4 del giro completo N-1 → N → N-1 → N che legge lo script da `scripts/`
  (`TestIlRitornoManualeDalla18Alla17`, `TestIlRitornoManualeDalla21Alla20`).

| Script | Da → a | Che cosa non riporta |
|---|---|---|
| `scripts/0018_indietro.sql` | 18 → 17 | le fusioni dei duplicati, le maiuscole dei codici allineate, il contenuto di `componente_proposta` e `relazione_proposta`; `componente.padre_id` torna in fondo alla tabella. Vale solo su un database alla 18 esatta |
| `scripts/0021_indietro.sql` | 21 → 20 | le note sui disegni, se si sceglie di perderle |

La 0019 e la 0020 non hanno un ritorno manuale: solo il backup.

## Invarianti

- Versioni 1..n senza buchi, un file per versione, nome `NNNN_nome.sql` (`migrazioni.Elenca`).
- Ogni file registra la propria versione, e la registra giusta (`VerificaStatica`, e `applicaUna` prima
  del commit).
- Nessuna `REFERENCES` in avanti; nessun valore di enum aggiunto e usato nello stesso file
  (`VerificaStatica`).
- Una transazione per file; un file fallito non lascia niente e non risulta applicato.
- Un binario non parte su un database più avanti di lui.
- `thread_offerta.stato` lo scrive solo `trg_stato_thread`: nessuna query lo aggiorna.
- Dopo il congelamento la BOM working e le versioni congelate non si modificano (trigger della 0020,
  anche fuori dal Go).

La regola «un file applicato non si modifica» è scritta (qui e in `README.md`), ma il migratore non la
controlla: `schema_versione` non tiene un'impronta del file.

## Dipendenze

`embed.go` (package `cockpit`, radice del modulo) incorpora i file in `cockpit.FS`; li leggono
`platform/migrazioni` (tramite `app/runtime` e `platform/testutil`) e sqlc. Le prove di
`platform/migrazioni` leggono anche `scripts/0018_indietro.sql` e `scripts/0021_indietro.sql` dal disco.

## Test

| File (`internal/platform/migrazioni/`) | Livello | Che cosa |
|---|---|---|
| `migrazioni_test.go` | L1 | verifica statica delle migrazioni vere; rifiuto di `REFERENCES` in avanti, enum usato nello stesso file, versione non registrata, buchi di numerazione, nome non valido |
| `migrazioni_db_test.go` | L4 | da vuoto e idempotente (S1), migrazione fallita a metà (S3), file che non si registra, database più recente del binario, fondazioni su dati, vincoli della 0002 |
| `caselle_db_test.go` | L4 | 0004 su una versione 3 con dati: il travaso delle presenze e dei cursori, e l'arresto con più caselle attive |
| `classificazione_db_test.go` | L4 | 0016 si ferma sulle richieste in `risposta` e passa dopo la riconciliazione; `v_inbox` della 0017 |
| `fascicolo0018_db_test.go` | L4 | le 13 guardie (anche tutte in un giro), la fusione, le FK composite, `SET CONSTRAINTS … DEFERRED`, i default del fabbisogno, `v_fascicolo`, l'albero, il ritorno 18 → 17 |
| `fascicolo0020_db_test.go` | L4 | le guardie, la catena delle revisioni, lo STEP strutturale, la deroga strutturale, `struttura_completa`, versioni e istantanee immutabili, il confine delle revisioni, la working bloccata, l'archiviazione, `analizzatore_corrente`, le tabelle della 0019 |
| `fascicolo0021_db_test.go` | L4 | il ritorno 21 → 20 e il suo arresto se ci sono note |
| `larghezze_db_test.go` | L4 | `codice` e `rev` larghe almeno quanto `classificazione.MaxCodice` / `MaxRev` |

L4: `COCKPIT_TEST_DSN=… go test -tags integrazione -count=1 -p 1 ./internal/platform/migrazioni/`.

## Stato dell'implementazione

Lo schema è completo fino alla 0021. Alcune parti esistono prima del codice che le usa:

- `nas_orfano` e `nas_creazione` (0019): le leggono `PercorsoOccupato` e `CartellaReferenziata`, le
  scrivono solo le prove; il job `sposta_nas` lo accodano solo le prove (`AccodaSpostamento`) e non ha
  ancora un esecutore (B8.8): l'esecutore del server lo chiuderebbe come fallimento definitivo;
- `regola` (0001): nessuna riga, e `documento_proposta.regola_id` resta NULL;
- `transizione`: seminata (0001, 0020) e mai letta per decidere, come dice la 0020;
- `soggetto_altro`, `recapito_altro` (0016): li legge il riconoscimento della controparte, ma nessuna
  schermata li scrive;
- `v_componente_albero` (0018, 0020): nessuna query la legge;
- `componente.esito_fattibilita` e `note_fattibilita`: nessun codice le scrive.

**Indici che le query non trovano** (fatti letti sullo schema e sulle query; il peso vero va misurato con
`EXPLAIN` su dati reali):

| Query | Filtra su | Indice oggi |
|---|---|---|
| `ContaNuoveDallaVisita`, `ContaNuovePerCasella`, `ListMessaggiNuoviDa` (Inbox e testata, ogni 15 s per scheda aperta) | `messaggio.registrato_il` | nessuno |
| `UltimoJobPerChiave` (preparazione del Fascicolo, ripresa dei documenti), `JobPendenteConPrefisso`, `UltimoJobPerChiavePrefisso` | `job.chiave_idempotenza` in qualunque stato, o con `LIKE 'prefisso%'` | solo `ux_job_chiave_pendente` (job pendenti) |
| `UltimoSyncPerCasella` (testata, ogni 15 s per scheda) | `job.tipo = 'sync_outlook' AND stato = 'fatto'` | nessuno su `tipo` |
| `ThreadPerCodiciCliente`, `ThreadApertePerCodici`, `RichiestePerCodiciFornitore` | `upper(identificativo_thread.codice)` | `ix_ident_codice` su `codice`, che nessuna query usa |
| `SetCodiceProposteComponente` e il controllo di `fk_proposta_componente` quando cambia o sparisce un componente | `documento_proposta.componente_id` | nessuno |
| `ListDocumentiDaVerificare`, `UltimoControlloNas` | `documento.verificato_il` in ordine | nessuno |
| `ListDocumentiMessaggio` | `documento_provenienza.messaggio_id` | nessuno |

Nessuna query filtra su `messaggio_outlook.conversation_id` (`ix_msgout_conv`). Due costi non dipendono
dagli indici: `v_documento_storia` è un `WITH RECURSIVE`, e PostgreSQL non vi porta dentro il filtro di
`ListStoriaDocumento`, quindi ricostruisce le catene di tutti i documenti; `v_cruscotto` passa da
`v_thread_bloccanti`, che è un aggregato di `v_fascicolo` su tutte le RFQ, e la leggono
`ListRichiestePanoramica` (con il poll della pagina Richieste) e `CercaThreadAperti`.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una tabella o una colonna nuova | `migrations/NNNN_nome.sql` (sopra, «Scrivere una migrazione nuova»), poi `sqlc generate` |
| un valore nuovo in un enum | un file con solo `ALTER TYPE … ADD VALUE`, l'uso dal file successivo o dal codice |
| capire perché la migrazione non parte | il messaggio della guardia (lettera e righe), `internal/platform/migrazioni/migrazioni.go` |
| capire perché una scrittura sulla BOM è rifiutata | i trigger della 0020 (`BOM01`–`BOM05`) |
| tornare alla versione di prima | il backup; per la 0018 e la 0021 `scripts/NNNN_indietro.sql`, a server fermo |
| provare una migrazione su dati «di prima» | `testutil.SchemaFinoA`, `migrazioni.ApplicaFinoA`, le prove di `internal/platform/migrazioni/` |
| l'elenco delle colonne e dei tipi Go | `internal/platform/db/models.go` |

## Leggi anche

`internal/platform/db/README.md`, `internal/platform/README.md` (`migrazioni`, `testutil`),
`README.md` («Manutenzione», «Aggiungere una migrazione»), `sqlc.yaml`, `embed.go`.
