# `internal/platform/db` — il codice generato da sqlc: i tipi dello schema e una funzione per query

## Scopo

Rende chiamabili da Go, con tipi controllati, lo schema di `migrations/` e le query di `queries/*.sql`:
un modello per ogni tabella e vista (`models.go`), un tipo per ogni enum, una funzione
`(*Queries).<Nome>` per ogni query. È il posto dell'SQL applicativo: chi ha bisogno di leggere o
scrivere il database aggiunge una query qui e la chiama con `db.New(pool|tx).<Nome>(ctx, …)`.

È l'unico package di `platform` che conosce le parole del dominio (RFQ, fascicolo, BOM…): non per
scelta, ma perché è lo schema tradotto in Go. Non contiene logica: ogni file è generato.

## Non appartiene qui

- Lo schema e le sue regole: `migrations/` (vedi `migrations/README.md`). sqlc lo legge da lì.
- Le transazioni: le apre chi chiama (`pool.Begin`, poi `db.New(tx)`). Qui ci sono solo le query che
  prendono i lucchetti (`Blocca*`, `FOR UPDATE`) e quelle che confrontano e scambiano.
- Le decisioni di dominio: stanno in `core/*` (e, per le eccezioni dichiarate in `internal/README.md`,
  in `transport/*`). Le query leggono e scrivono quello che il chiamante ha deciso.
- Codice scritto a mano: nessun `.go` di questa cartella lo è. Una modifica si perde al prossimo
  `sqlc generate`.

## File

| File | Responsabilità |
|---|---|
| `db.go` | `DBTX` (`Exec`, `Query`, `QueryRow`: lo soddisfano `*pgxpool.Pool`, `pgx.Tx`, `*pgx.Conn`), `New`, `Queries`, `WithTx` (nessun chiamante: si usa `db.New(tx)`) |
| `models.go` | 47 enum (`TipoJob`, `StatoNas`, `TipoDocumento`, `Fase`…) con `Null<Enum>`, `Valid()` e `All<Enum>Values()`; una struct per ognuna delle 69 tabelle e delle 11 viste (`Messaggio`, `Documento`, `VInbox`, `VFascicolo`, `VStepProdotto`…) |
| `queries/<nome>.sql` | la sorgente: una o più query `-- name: <Nome> :one\|:many\|:exec\|:execrows` |
| `<nome>.sql.go` | generato da `queries/<nome>.sql`: la costante SQL, `<Nome>Params`, `<Nome>Row`, il metodo su `*Queries` |
| `sqlc.yaml` (nella cartella `cockpit/`) | la configurazione: sorgenti, `pgx/v5`, `rename` dei singolari latini, `overrides` di tipo e di nullabilità |

### Le query per file

28 file, 438 query. «Senza chiamanti» = nessun chiamante, né in produzione né nei test; il «+n» fra
parentesi conta quelle chiamate soltanto dai test. L'elenco per nome è in «Stato dell'implementazione».

| File | Dominio | Tabelle e viste | Query | Senza chiamanti | Chi le chiama |
|---|---|---|---|---|---|
| `agente.sql` | analisi semantica dell'agente | `analisi_messaggio` | 6 | 2 | `ai/agente` |
| `aggancio.sql` | candidati di aggancio (R0–R5) e di codice | `candidato_aggancio`, `candidato_codice`, `messaggio`, `conversazione`, `thread_offerta`, `identificativo_thread`, `proposta_triage` | 16 | 1 | `core/inbox/aggancio`, `core/inbox/ingest`, `transport/web`, `ai/agente` |
| `allegati.sql` | allegati, hash di rumore | `allegato`, `hash_rumore`, `messaggio` | 11 | 1 | `transport/workerapi`, `platform/coda`, `core/rfq/fascicolo`, `core/rfq/documenti`, `core/inbox/ingest`, `transport/web`, `ai/agente` |
| `altro.sql` | anagrafica «Altro» (7C.0) | `soggetto_altro`, `recapito_altro` | 8 | 5 (+2) | `core/inbox/ingest` |
| `anagrafica.sql` | clienti, domini, buyer, fabbisogno, cataloghi | `cliente`, `dominio_cliente`, `buyer`, `fabbisogno_documento`, `fase_catalogo`, `transizione`, `regola` | 26 | 4 | `transport/web`, `core/registro/anagrafica`, `core/registro/fornitori`, `core/inbox/ingest`, `core/rfq/fascicolo`, `transport/workerapi` |
| `analisi.sql` | fatti dell'analisi, analizzatore corrente | `analisi_fatti`, `analizzatore_corrente`, `documento_proposta`, `allegato` | 5 | 0 | `transport/workerapi`, `platform/coda`, `core/rfq/fascicolo`, `transport/web`, `app/runtime` |
| `annotazioni.sql` | note sui PDF (0021) | `annotazione_pdf`, `allegato`, `messaggio`, `utente`, `componente` | 5 | 0 | `transport/web` |
| `bom.sql` | versioni della BOM, istantanee, gate del congelamento | `bom_versione`, `bom_versione_{componente,relazione,documento,deroga}`, `v_bom_versioni`, `v_step_prodotto`, `v_thread_bloccanti`, working e proposte | 21 | 5 | `core/rfq/fascicolo`, `transport/web` |
| `cache.sql` | cache dei contenuti dello staging | `allegato`, `documento`, `nas_anomalia`, `documento_proposta`, `job` | 5 | 0 | `platform/storage/staging`, `core/rfq/documenti`, `core/rfq/fascicolo` |
| `candidati.sql` | codici candidati di una RFQ (B8.6) | `v_codici_candidati_thread`, `documento_proposta`, `componente_proposta` | 2 | 0 | `core/rfq/fascicolo` |
| `editor.sql` | editor della struttura (Fascicolo v3) | `rimozione_proposta` | 1 | 0 | `core/rfq/fascicolo` |
| `fascicolo.sql` | proposte di documento, documenti, percorsi e lucchetti del NAS | `v_fascicolo`, `v_thread_bloccanti`, `v_documento_storia`, `v_step_prodotto`, `documento_proposta`, `documento`, `documento_provenienza`, `cartella_documento`, `componente`, `job`, `nas_orfano` | 38 | 3 | `transport/web`, `core/rfq/documenti`, `core/rfq/fascicolo`, `core/inbox/ingest`, `transport/workerapi` |
| `fondazioni.sql` | caselle, postazioni, credenziali dei worker | `casella`, `postazione`, `worker_credenziale`, `casella_store` | 23 | 1 (+3) | `platform/fondazioni`, `transport/web`, `transport/workerapi`, `core/inbox/ingest`, `platform/coda` |
| `fornitori.sql` | fornitori, lavorazioni, qualifiche, convenzioni di codice, controparte | `fornitore`, `dominio_fornitore`, `contatto_fornitore`, `lavorazione`, `fornitore_lavorazione`, `cliente_fornitore_lavorazione`, `convenzione_codice(_lavorazione)`, `messaggio` | 35 | 1 | `transport/web`, `core/registro/fornitori`, `core/inbox/ingest`, `app/runtime` |
| `inbox.sql` | Inbox viva (voce 2.16) | `utente`, `messaggio`, `messaggio_casella`, `job` | 6 | 0 | `transport/web` |
| `integrita.sql` | integrità del NAS, orfani, prove di creazione | `documento`, `nas_anomalia`, `nas_orfano`, `nas_creazione`, `job` | 17 | 4 (+2) | `core/rfq/documenti`, `transport/web` |
| `interpretazione.sql` | portale, deroghe, triage, bozze | `riferimento_portale`, `deroga_fabbisogno`, `proposta_triage`, `bozza` | 14 | 3 | `core/inbox/ingest`, `transport/web`, `transport/workerapi`, `core/rfq/fascicolo` |
| `job.sql` | coda dei job, presenza dei worker | `job`, `worker_presenza`, `postazione` | 31 | 1 (+1) | `platform/coda`, `transport/web`, `transport/workerapi`, `app/runtime`, `core/rfq/documenti`, `core/inbox/ingest`, `platform/storage/staging` |
| `messaggi.sql` | messaggi, presenze per casella, cursori, Inbox | `conversazione`, `messaggio`, `messaggio_outlook`, `messaggio_casella`, `sync_cursore`, `v_inbox` | 34 | 4 (+1) | `core/inbox/ingest`, `transport/web`, `transport/workerapi`, `platform/coda`, `core/rfq/fascicolo`, `core/rfq/documenti`, `ai/agente` |
| `panoramica.sql` | pagina Richieste | `v_cruscotto`, `identificativo_thread`, `componente`, `documento`, `documento_proposta` | 3 | 0 | `transport/web` |
| `preparazione.sql` | preparazione automatica del Fascicolo (B8.7b) | `allegato`, `messaggio`, `documento_proposta`, `job` | 4 | 0 | `core/rfq/fascicolo`, `transport/web` |
| `proposte.sql` | proposte di struttura dagli STEP (B8.5) | `componente_proposta`, `relazione_proposta`, `rimozione_proposta`, `analisi_fatti`, `analizzatore_corrente` | 25 | 0 | `core/rfq/fascicolo`, `transport/web`, `transport/workerapi` |
| `richieste.sql` | richieste d'offerta ai fornitori (7B) | `richiesta_fornitore`, `candidato_richiesta`, `messaggio`, `proposta_triage`, `bozza` | 22 | 4 | `core/inbox/aggancio`, `core/inbox/ingest`, `transport/web` |
| `scarti.sql` | scarti dell'ingest, log degli agganci | `ingest_scarto`, `messaggio_aggancio_log` | 8 | 1 (+1) | `core/inbox/ingest`, `transport/web` |
| `schermata.sql` | letture e piccoli gesti della schermata del Fascicolo | `allegato`, `documento_proposta`, `documento_provenienza`, `componente_proposta`, `relazione_proposta`, `deroga_*`, `componente`, `messaggio` (nota interna) | 15 | 0 | `core/rfq/fascicolo`, `transport/web` |
| `struttura.sql` | BOM working: archiviazione, STEP strutturale, deroga strutturale, archi | `componente`, `componente_relazione`, `deroga_struttura`, `rimozione_proposta`, `v_step_prodotto` | 18 | 0 | `core/rfq/fascicolo`, `transport/web` |
| `thread.sql` | RFQ, identificativi, componenti, fasi | `thread_offerta`, `v_cruscotto`, `identificativo_thread`, `componente`, `fase_log`, `v_thread_fase`, `v_thread_da_riesaminare` | 28 | 6 | `core/rfq/fascicolo`, `transport/web`, `core/rfq/documenti` |
| `utenti.sql` | utenti e sessioni | `utente`, `sessione`, `postazione` | 11 | 1 | `transport/web`, `platform/fondazioni`, `platform/coda` |

Alcuni file ospitano query di un altro dominio: `interpretazione.sql` mescola portale, deroghe, triage e
bozze; `cache.sql` ha `UltimoJobPerChiave` (coda); `fornitori.sql` ha `ContaAnagrafiche`; `richieste.sql`
ha `SetAttoTriage`, `InsertBozzaRichiesta`, `SetBozzaInviata`, `RiproponiAllegatiComeOffertaFornitore`;
`schermata.sql` ha `UpsertNotaInterna` (un `messaggio`) e `ContaAnalisiInCorso` (`job`).

## Entry point

| Simbolo | Chi lo usa |
|---|---|
| `New(DBTX) *Queries` e i 438 metodi | `ai/agente`, `app/runtime`, `core/inbox/aggancio`, `core/inbox/ingest`, `core/registro/anagrafica`, `core/registro/fornitori`, `core/rfq/documenti`, `core/rfq/fascicolo`, `platform/coda`, `platform/fondazioni`, `platform/storage/staging`, `transport/web`, `transport/workerapi` |
| i tipi enum (`RuoloUtente`, `AllRuoloUtenteValues`…) | anche `platform/config` (validazione dei ruoli di `[[utenti]]`) e il test L3 di `platform/contratti/worker` (i valori dei contratti contro gli enum) |
| i modelli (`db.Messaggio`, `db.Documento`, `db.VInbox`…) | tutti i package sopra, e i template di `transport/web` attraverso i dati delle pagine |

Il package non ha un'interfaccia `Querier` (`emit_interface` non è acceso): chi chiama dipende da
`*db.Queries`, e le prove passano da un database vero (L4).

## Dati

Tutte le tabelle e le viste dello schema, ciascuna letta o scritta dalle query del suo file. Il package
non apre transazioni e non tiene stato.

**Lucchetti presi dalle query** (valgono dentro la transazione di chi chiama):

| Query | Lucchetto |
|---|---|
| `BloccaMessaggio`, `BloccaThread`, `BloccaComponente`, `BloccaDocumento`, `BloccaProposta`, `BloccaComponenteProposta`, `BloccaRelazioneProposta`, `BloccaRimozioneProposta`, `ListDocumentiComponente` | `FOR UPDATE` sulla riga (o sulle righe) |
| `BloccaTentativo`, `BloccaCopiaPendente`, `BloccaSpostamentoPendente` | `FOR UPDATE` sul job; un job `pronto` bloccato viene saltato dal claim |
| `ClaimJob` | `FOR UPDATE SKIP LOCKED LIMIT 1` dentro un solo `UPDATE` |
| `BloccaCartella` | `pg_advisory_xact_lock(hashtextextended('<thread>:<cartella in minuscolo>', 0))` |

I trigger della 0020 prendono i loro (`FOR UPDATE`, `FOR SHARE`, `FOR KEY SHARE`): vedi
`migrations/README.md`.

**Confronta e scambia.** Le `:execrows` con lo stato nel `WHERE` restituiscono 0 righe quando lo stato
è cambiato nel frattempo, e il chiamante lo tratta come un esito, non come un errore: `DecidiProposta`,
`DecidiComponenteProposta`, `CongelaBomVersione`, `EliminaBozza`, `SetSostituitoDa`,
`AnnullaSostituzione`, `SetDocumentoScritto` (vale solo per il percorso passato), `SetPathDocumentoInCoda`,
`PrendiSyncAperturaInbox`, e le sei query con il predicato di validità del tentativo (`HeartbeatJob`,
`CompletaJob`, `FallisciJob`, `RinviaJob`, `BloccaTentativo`, `VerificaTentativo`, commento in testa a
`queries/job.sql`).

**Il perimetro sta nel `WHERE`.** Quando l'id arriva da un form o da un indirizzo, la query porta anche
la riga a cui deve appartenere, e fuori da quella tocca 0 righe: `EliminaBuyer` (il buyer del cliente
della pagina, `cliente_id`; e non cancella un buyer citato da un messaggio, da una RFQ o da una proposta
di triage, `proposta_triage.buyer_proposto`), `DeleteDeroga` e `DeleteDerogaStruttura` (la deroga della
RFQ, `thread_id`). Due scritture non scavalcano uno stato più forte: `SetDocumentoErrore` non tocca un
documento già `scritto`, `PropostaDocumentoDaRadice` non riscrive una proposta con `fonte = 'operatore'`.
`ThreadPerChiaviCitate` (R0) filtra anche `canale = 'outlook'`, così usa l'indice unico
`(canale, chiave_esterna)`.

**Upsert su indici parziali o su espressioni.** `InsertJob` (`ux_job_chiave_pendente`: un solo job
pendente per chiave), `ApriAnomaliaNas` (una aperta per documento), `InsertNasOrfano` (una aperta per
`(thread, lower(percorso))`), `InsertComponente` (`(thread_id, upper(codice))`). La clausola
`ON CONFLICT … WHERE` deve ripetere il predicato dell'indice alla lettera.

**Vincoli differiti.** `SetCodiceComponente` funziona solo in una transazione che ha eseguito
`SET CONSTRAINTS fk_documento_componente, fk_proposta_componente DEFERRED` (lo fa
`transport/web/fascicolo.go`), perché documenti e proposte si allineano al codice nuovo nelle `UPDATE`
successive.

**Errori dei trigger.** Le scritture sulla BOM possono fallire con gli SQLSTATE `BOM01`–`BOM05` della
0020; li traduce `transport/web/fascicolo.go`.

## Flussi principali

**Rigenerare il codice.** Dalla cartella `cockpit/` (dove sta `sqlc.yaml`):

```
sqlc generate      # riscrive models.go, db.go e ogni <nome>.sql.go
sqlc diff          # confronta senza scrivere: dice se il codice generato è indietro
```

sqlc legge come schema tutti i file di `migrations/` in ordine di nome (i blocchi `DO` li salta: per
questo la 0018 crea le sue tabelle temporanee dentro un `DO`) e le query di `queries/`. Il codice di
questo ramo è generato con sqlc v1.31.1 (intestazione dei file); `README.md` chiede la 1.31 o
successiva e `scripts/prova-tutto.ps1` lo cerca nel `PATH` o in `%LOCALAPPDATA%\cockpit_rfq_test\sqlc\`.
Dopo: `go build ./...`, `go vet ./internal/platform/db/`, e le prove L4 dei package che usano le query
toccate.

**Aggiungere una query.**

1. sceglierne il file per dominio (tabella sopra), o crearne uno nuovo in `queries/`;
2. `-- name: <Nome> :one|:many|:exec|:execrows` e, subito dopo, i commenti che diventano la
   documentazione Go del metodo; i commenti prima di `-- name:` restano nel file e basta;
3. parametri: `$n`, oppure `sqlc.arg(nome)` / `@nome`, e `sqlc.narg(nome)` quando il valore può essere
   NULL; un cast (`::uuid[]`, `::text`, `::stato_job`) fissa il tipo che sqlc non inferisce;
   `sqlc.embed(t)` mette la struct della tabella dentro la riga;
4. se si legge una vista con `COALESCE`, `LEFT JOIN LATERAL` o `array_agg`, sqlc sbaglia la nullabilità:
   si corregge con un `overrides` per colonna in `sqlc.yaml`, con il motivo accanto;
5. `sqlc generate`, poi la chiamata: `db.New(pool).<Nome>(ctx, …)` fuori da una transazione,
   `db.New(tx).<Nome>(ctx, …)` dentro.

**Una colonna o una tabella nuova.** Prima la migrazione (`migrations/README.md`), poi
`sqlc generate`: cambiano `models.go` e ogni riga che usa `SELECT *` o `RETURNING *` su quella tabella.

## Invarianti

- I 30 file `.go` portano l'intestazione `Code generated by sqlc. DO NOT EDIT.`.
- Le query di `queries/` e i metodi di `*Queries` sono gli stessi 438 nomi; ogni `queries/<nome>.sql` è
  stato committato insieme al suo `<nome>.sql.go`, e l'ultimo commit che tocca `migrations/` è anche
  l'ultimo che tocca `models.go`.
- Tipi: `uuid` → `uuid.UUID` / `uuid.NullUUID`; `timestamptz` e `date` → `time.Time` / `*time.Time`;
  `jsonb` → `json.RawMessage` / `*json.RawMessage`; `inet` → `*netip.Addr`; `numeric` → `pgtype.Numeric`;
  gli array vuoti tornano come slice vuote (`emit_empty_slices`).
- La nullabilità delle colonne di `v_fascicolo`, `v_step_prodotto`, `v_bom_versioni`,
  `v_documento_storia`, `v_thread_da_riesaminare`, `v_cruscotto`, `v_inbox`, `v_componente_albero` è
  quella di `sqlc.yaml`, non quella che sqlc dedurrebbe.
- `rename` in `sqlc.yaml` corregge i singolari latini di sqlc (`documento_propostum` →
  `DocumentoProposta` e simili). Non copre `candidato_richiesta` e `nas_anomalia`: i modelli si chiamano
  `CandidatoRichiestum` e `NasAnomalium`.
- L'SQL applicativo fuori da `queries/` è poco e noto: `SET CONSTRAINTS … DEFERRED`
  (`transport/web/fascicolo.go`), `min(ricevuto_il)` di una casella e le letture di
  `riferimento_portale` e `bozza` per il pannello del messaggio (`transport/web/routes_inbox.go`),
  `max(versione)` per `/healthz` (`transport/web/server.go`), il `FOR KEY SHARE` sulla RFQ prima di
  bloccare la proposta da confermare (la RFQ per prima, come negli altri gesti sul Fascicolo:
  `transport/web/allegati.go`), il lucchetto consultivo `cartella_rfq:<nome>` e la ricerca di una
  cartella NAS già presa da un'altra RFQ, che fa nascere la seconda con « (2)»
  (`transport/web/triage.go:cartellaLibera`); il migratore esegue i file di `migrations/`.

## Dipendenze

Importa solo librerie: `pgx/v5` (`pgx`, `pgconn`, `pgtype`), `google/uuid`, `encoding/json`,
`net/netip`, `time`. È importato dai 14 package elencati in «Entry point» (compreso `platform/config`
per gli enum). Nessuna violazione della tabella di `internal/README.md`: `ai/*`, `core/*`,
`transport/*`, `app/*` possono importare `platform`.

## Test

Nessun file di prova nel package: il codice è generato. Lo provano:

| Livello | Dove | Che cosa |
|---|---|---|
| L1 | `platform/migrazioni/migrazioni_test.go` | lo schema che sqlc legge passa la verifica statica (versioni, `REFERENCES`, enum) |
| L3 | `platform/contratti/worker` | i valori dei contratti coincidono con gli enum generati (`Direzione`, `NaturaAllegato`…) |
| L4 | `*_db_test.go` di `core/inbox/ingest`, `core/rfq/documenti`, `core/rfq/fascicolo`, `core/registro/*`, `platform/coda`, `platform/fondazioni`, `platform/storage/staging`, `transport/web`, `transport/workerapi`, `app/runtime` | le query contro PostgreSQL vero, attraverso le funzioni che le usano |

Delle 381 query con un chiamante in produzione, 97 sono chiamate per nome anche dai test; le altre sono
provate, quando lo sono, attraverso le funzioni che le usano.

Ogni prova che apre il database porta il tag `integrazione`, anche `core/inbox/ingest/ingest_test.go`, che
non ha `_db` nel nome: `go test ./...` senza tag (L1) non tocca il database nemmeno con `COCKPIT_TEST_DSN`
impostata. Con il tag, `platform/testutil` rifiuta un DSN il cui database non ha «test» nel nome, e il
nome lo legge come lo leggerà pgx (`testutil.go:DatabaseDiTest`, con `pgconn.ParseConfig`: vale anche per
un DSN chiave=valore, per `?dbname=` e per un DSN senza database), perché le prove distruggono e
ricreano lo schema.

## Stato dell'implementazione

381 query hanno un chiamante in produzione. Le altre 47 non ne hanno nessuno, e 10 sono chiamate solo
dai test. Raggruppate per motivo:

| Motivo | Query |
|---|---|
| funzione non ancora costruita | anagrafica «Altro» senza schermata: `ListSoggettiAltro`, `GetSoggettoAltro`, `GetSoggettoAltroPerEtichetta`, `ListRecapitiAltro`, `SetRecapitoAltroAttivo` (solo test: `InsertSoggettoAltro`, `InsertRecapitoAltro`); spostamento sul NAS (B8.8): `SetPathDocumentoSeUguale`, `ListNasOrfaniAperti`, `RisolviNasOrfaniDelJob`, `InsertNasCreazione`, `GetNasCreazione` (solo test: `InsertNasOrfano`, `RisolviNasOrfano`); lettura di una versione congelata: `GetUltimaCongelata`, `ListBomComponenti`, `ListBomRelazioni`, `ListBomDocumenti`, `ListBomDeroghe`; regole che si spengono da sole: `ListRegole`, `IncrementaRegola` (la tabella `regola` non viene mai popolata); transizioni: `ListTransizioniDa`; portale: `ListRiferimentiPortaleThread`, `SetRiferimentoPortaleStato`; sgancio di un messaggio: `SgangiaMessaggio`; fattibilità: `SetNoteComponente`; RFQ: `SetCartellaThread`, `SetScadenzaThread`, `GetFaseCorrente`, `ListFaseLog`; buyer: `UpdateBuyer` |
| tolte di proposito dalla schermata | richieste ai fornitori a livello di RFQ (blocco 8, `internal/README.md`): `ListRichiesteThread`, `ListRichiesteFornitore`, `ContaRichiesteAperte`, `SetAttoTriage` |
| senza chiamante e senza un motivo scritto | `GetUtente`, `ThreadDellaConversazione`, `ThreadPerCodiceCliente`, `ListProposteAperteThread`, `ListProposteBulk`, `EliminaIngestScarto`, `ListAnalisiMessaggio`, `ContaAnalisiPerStato`, `ContaCandidatiAggancio`, `ListAllegatiSenzaStaging`, `GetConvenzione`, `ListCredenzialiOutlookAttive`, `GetBozza`, `EsisteJobPronto`, `GetPresenza`, `ListMessaggiConversazione` |
| solo test, lettura di verifica | `GetWorkerCredenziale`, `GetCasellaStore`, `ListCasellaStorePerPostazione`, `GetWorkerPresenza`, `GetSyncCursore`, `ListAgganciaLog` |

`BloccaSpostamentoPendente` ha un chiamante (`core/rfq/documenti/nomi_nas.go:AccodaSpostamento`), ma
`AccodaSpostamento` lo chiamano solo le prove: finché B8.8 non arriva, il job `sposta_nas` non ha un
esecutore in `app/runtime` (l'esecutore del server chiude un tipo che non gestisce come fallimento
definitivo).

Per rifare il conto dalla cartella `cockpit/`: il comando elenca le query senza chiamanti in produzione
(oggi 57, cioè le 47 e le 10). Sei nomi coincidono con funzioni di altri package (per esempio
`fascicolo.ArchiviaComponente`), e il comando li conta come usati anche quando a chiamarli è solo
quella funzione: vanno guardati a mano.

```
grep -h -- '-- name:' internal/platform/db/queries/*.sql | awk '{print $3}' | while read n; do
  grep -rqE "\.$n\(" --include='*.go' --exclude='*_test.go' --exclude-dir=db internal cmd || echo "$n"
done
```

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una query nuova | `queries/<dominio>.sql`, poi `sqlc generate` |
| cambiare il tipo Go di una colonna o la nullabilità di una vista | `sqlc.yaml` (`overrides`), poi `sqlc generate` |
| rinominare un modello con un singolare sbagliato | `sqlc.yaml` (`rename`), poi i chiamanti |
| una colonna o una tabella nuova | una migrazione (`migrations/README.md`), poi `sqlc generate` |
| capire perché una scrittura sulla BOM è rifiutata | i trigger della 0020 (`BOM01`–`BOM05`) e `transport/web/fascicolo.go` |
| capire che cosa fa un lucchetto | la query `Blocca*` in `queries/` e il commento sopra |
| sapere se il codice generato è in pari | `sqlc diff` dalla cartella `cockpit/` |

## Leggi anche

`migrations/README.md`, `internal/platform/README.md`, `internal/README.md`, `README.md`
(«Manutenzione»), `sqlc.yaml`.
