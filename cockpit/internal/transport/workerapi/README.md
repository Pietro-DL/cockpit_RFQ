# `internal/transport/workerapi` — le rotte `/api/v1/*` dei worker Python

## Scopo

Il lato server del protocollo con i worker delle postazioni (Outlook e analisi): prendere un job dalla coda,
tenerlo vivo, riportarne l'esito, consegnare i lotti di posta, caricare un allegato, scaricare i byte da
analizzare. Risponde solo JSON (byte grezzi per `/contenuto`), e ogni rotta passa dal token individuale in
`X-Cockpit-Token`.

Oltre alle rotte ci sono due pezzi che il resto del server chiama senza passare da HTTP:

- la **strada di un file appena arrivato in staging** (`dopoStaging`): proposta dal nome, rumore, estrazione
  dello zip o analisi. Il caricamento interno del Fascicolo la percorre tale e quale (`DopoCaricamento`);
- l'**estrazione di un archivio** (`EstraiArchivio`), che esegue l'esecutore interno per il job `estrai_archivio`.

## Non appartiene qui

- La coda (claim, lease, predicato del tentativo, capacità, idempotenza): `platform/coda`.
- I tipi JSON del contratto: `platform/contratti/worker`, con la copia pydantic in `workers/contratti.py` e gli
  schemi in `contracts/`.
- La scrittura dei lotti di posta (fatti, controparte, catena, triage, candidati) e il controllo che il job del
  tentativo possa consegnare quel lotto: `core/inbox/ingest`, chiamato con `ingest.Servizio.Ingerisci`
  (`ErrLottoNonDelJob`).
- Chi esegue i job del server: `app/runtime/esecutore.go`. Qui c'è solo che cosa significa scompattare un archivio.
- Le decisioni di dominio. Ci stanno ancora, come eccezione aperta dichiarata in `internal/README.md`:
  `archivi.go`, `applicaRisultato`, `propostaDaAnalisi`, e con loro il resto della pipeline dopo lo staging
  (`dopoStaging` con la regola del rumore, `scriviProposta`, `motoreDelFile`, `codiceRevSicuri`,
  `applicaFattiEsistenti`, `strutturaNelleRfq`, `EstraiArchivio`, `DopoCaricamento`). È logica di dominio che
  vive nel livello di trasporto: per ora resta qui.

## File

| File | Righe | Responsabilità |
|---|---|---|
| `workerapi.go` | 1479 | `Server`, `Registra`. Autenticazione: `auth` (token → `worker_credenziale`), `contatto` (prova di vita), `stessoWorker`, `casellaAutorizzata` (la regola delle caselle della credenziale, per il claim e per l'ingest). Coda: `claim` (`risolviDestinazione`, `casella_store`, presenza), `caselleWorker`, `heartbeat`, `result` (`tentativo`, `nonValido`, `risultatoNonApplicabile`, `fallimentoDefinitivo`, `preparaStage`, `applicaRisultato`). Proposte: `propostaDaAnalisi`, `scriviProposta`, `motoreDelFile`, `codiceRevSicuri`, `enumValido`, `conEsito`/`separaEsito`, `applicaFattiEsistenti`, `strutturaNelleRfq`. `riallineaEntryID`, `DopoCaricamento`/`dopoStaging`, `ingest`, `cursori` |
| `upload.go` | 195 | `PUT /allegati/{id}/file`: `caricaFile`, `stageDelJob` (il job è il download di QUESTO allegato?), `parteDi`, `maxUpload`, `oltreIlLimite` |
| `contenuto.go` | 171 | `GET /allegati/{id}/contenuto`: `scaricaContenuto`, `nelloStaging` (il file sta sotto la radice dello staging?), `analisiDelJob`, `payloadAnalisi` |
| `archivi.go` | 167 | `EstraiArchivio` (voci fra i contenuti, allegati figli, proposte e analisi in una transazione), `estraiInContenuti` |

## Entry point

| Simbolo | Chi lo usa |
|---|---|
| `Server{Pool, Log, Ingest, Staging, CasellaDefault, Analizzatore, MaxUpload, IndirizzoClient}` | costruito in `app/runtime/ascolto.go:Ascolta`: `Staging` = radice dello staging del server, `CasellaDefault` = `[outlook].casella_default`, `Analizzatore` = versione e parametri di `[analisi]`, `MaxUpload` = `[server].max_upload_mb` (zero = 64 MB). `IndirizzoClient` nil = `RemoteAddr`: `X-Forwarded-For` non si legge mai; i test lo sostituiscono per simulare due PC |
| `Server.Registra(mux)` | `Ascolta`, sullo stesso mux di `web.Server.Registra` |
| `Server.EstraiArchivio(ctx, allegato, token)` | l'esecutore (`runtime.Estrattore`, `s.Esecutore.Archivi = wa`) per il job `estrai_archivio` |
| `Server.DopoCaricamento(ctx, q, allegato)` | `web.Server.Pipeline`: `caricamento.go`, `fascicolo_nas.go`, `fascicolo_preparazione.go`; nella transazione del chiamante |
| `ImprontaToken` | rinvio a `rete.ImprontaToken`: `auth` e i test |

Le rotte. Tutte passano da `auth`, che prima dell'handler risponde 401 (token assente, sconosciuto, condiviso
da due credenziali, credenziale mai generata) o 500 (lettura della credenziale fallita); gli stati sotto sono
quelli dell'handler.

| Rotta | Handler | Corpo → risposta | Stati |
|---|---|---|---|
| `POST /api/v1/jobs/claim` | `claim` | `ClaimRichiesta` → `Job` | 200, 204 (nessun job entro l'attesa), 400, 403, 500 |
| `GET /api/v1/worker/caselle?worker_id=` | `caselleWorker` | → `[]CasellaServita`: caselle attive autorizzate, solo canale Outlook | 200, 403, 500 |
| `POST /api/v1/jobs/{id}/heartbeat` | `heartbeat` | `HeartbeatRichiesta` | 204, 400, 403, 409, 500 |
| `POST /api/v1/jobs/{id}/result` | `result` | `RisultatoRichiesta`, `dati` secondo il tipo del job | 204, 400, 403, 404 (job inesistente), 409, 422, 500 (anche un errore del database nel leggere il job) |
| `POST /api/v1/ingest/messaggi` | `ingest` | `IngestRichiesta` → `IngestRisposta` (con `durata_ms` del server) | 200, 400, 403 (worker diverso, casella non autorizzata dalla credenziale, lotto non del job), 409, 422 (casella non censita), 500 |
| `PUT /api/v1/allegati/{id}/file?job_id=&lease_token=&worker_id=` | `caricaFile` | il file, binario | 204, 400, 403, 409, 413, 422, 500 |
| `GET /api/v1/allegati/{id}/contenuto?job_id=&lease_token=&worker_id=` | `scaricaContenuto` | → i byte, con `X-Cockpit-Sha256` | 200, 400, 403, 409, 410 (file assente o fuori dallo staging), 422, 500 |
| `GET /api/v1/sync/cursori` | `cursori` | → `[]CartellaCursore` di tutte le caselle | 200, 500 |

I corpi JSON si leggono con un tetto di 64 MB (`leggi`).

## Dati

| Chi | Legge | Scrive |
|---|---|---|
| `auth` | `worker_credenziale` (`CredenzialiPerToken`: per sha256, solo attive, al più due righe) | `worker_presenza.ultimo_contatto` (`ContattoWorker`, UPDATE: non fa nascere la riga) |
| `claim` | `postazione` (`GetPostazione`) | `casella_store` (`UpsertCasellaStore`, solo con postazione e `store_id`), `worker_presenza` (`DichiarazioneWorker` prima del long-poll, `ClaimConcluso` dopo), `job` (`coda.Claim`) |
| `caselleWorker` | `casella` ⋈ `worker_credenziale` (`ListCaselleAutorizzate`) | — |
| `heartbeat` | — | `job` (`coda.Batte`; `ScadutoPerDurataMassima` da `nonValido`) |
| `result` sync | `job` | `sync_cursore` (`UpsertSyncCursore`; `SetCopertoFinoA` o `SetStoricoFinoA` solo per le cartelle `completa`) |
| `result` stage | `job`, `allegato`, `messaggio`, `hash_rumore`, `analisi_fatti` | `allegato` (`SetAllegatoStaging` → `in_staging`; `analizzato` per il rumore), `messaggio_casella` (`SetEntryIDPresenza`), `documento_proposta` (`UpsertProposta`), `job` (`estrai_archivio` o `analizza_allegato`) |
| `result` analisi | `job`, `allegato`, `messaggio`, `cliente`, `documento_proposta` aperte con lo stesso sha256 | `analisi_fatti` (`UpsertAnalisiFatti`, dettagli + `esito`), `documento_proposta`, `allegato` → `analizzato`, le proposte di struttura di ogni RFQ con quel contenuto (`fascicolo.ApplicaStruttura`) |
| `result` bozza | `job` | `bozza` (`SetBozzaAperta`) |
| `result` apri / letto / sposta | `job` | `messaggio_casella` (`SetEntryIDPresenza`), solo se l'EntryID è cambiato e il payload ha `casella_id` |
| errore definitivo | — | `allegato` → `errore` (stage, analisi) o `bozza` → `errore` (`SetBozzaErrore`) |
| `ingest` | `job` (`GetJob`, solo per un lotto senza `casella_id`), `casella` (`ingest.RisolviCasella`) | tutto dentro `ingest.Servizio.Ingerisci` (vedi `internal/core/inbox/ingest/README.md`); `job` → fallito definitivo per una casella non censita |
| `caricaFile` | `job`, `allegato` | `allegato` → `errore` solo oltre il limite |
| `scaricaContenuto` | `job`, `allegato` | — |
| `EstraiArchivio` | `allegato`, `messaggio`, `analisi_fatti` | `allegato` (figli con `contenitore_id`), `documento_proposta`, `job` (`analizza_allegato`) |
| `cursori` | `sync_cursore` ⋈ `casella` | — |

**Transazioni.** `result`: la riga del job si legge fuori; per uno stage riuscito anche `coda.Verifica` e
`preparaStage` (sha256 del file, `MkdirAll`) stanno fuori. Poi una transazione sola: `coda.Blocca` (il
predicato del tentativo con `FOR UPDATE`) **prima di ogni scrittura**, gli effetti, `coda.Completa` o
`coda.Fallisci`, il commit. Un errore di `applicaRisultato` è rollback, poi `risultatoNonApplicabile` chiude il job
in una transazione nuova. `ingest`: la transazione è quella di `Ingerisci` (un lotto, un savepoint per
elemento, il tentativo bloccato e confrontato con il lotto). `caricaFile` e `scaricaContenuto`: nessuna
transazione, il tentativo si verifica senza bloccare. `EstraiArchivio`: disco fuori, righe in una transazione.
`DopoCaricamento`: quella del chiamante.

**Disco** (sotto `Server.Staging`):

| Percorso | Chi lo scrive | Chi lo toglie |
|---|---|---|
| `_parti/<allegato>.parte.<lease_token>` | `caricaFile` | promosso da `result` con una rinomina; rimosso per tentativo non valido, hash diverso, contenuto già presente, trasferimento interrotto |
| `_contenuti/<ab>/<sha256>.<ext>` | `result` (rinomina), `estraiInContenuti` | la cache dello staging (non questo package) |
| `_parti/zip.<lease_token>/` | `estraiInContenuti` | lei stessa, alla fine |

`scaricaContenuto` ed `EstraiArchivio` leggono `allegato.path_staging`; `scaricaContenuto` lo apre solo se
`nelloStaging` conferma che sta sotto `Server.Staging` (`documenti.DentroLaRadice`).

## Flussi principali

**Autenticazione (`auth`).** Header vuoto → 401. Sha256 del token → `CredenzialiPerToken` (errore → 500): una riga
→ se è il segnaposto `rete.ImprontaNonGenerata` 401, altrimenti `contatto` (errore solo nel log) e credenziale nel
contesto; nessuna → 401; due → 401 con i nomi dei worker che condividono il segreto. `stessoWorker` confronta
il `worker_id` dichiarato con il nome della credenziale, senza distinguere le maiuscole: diverso → 403; vuoto → passa.

**Claim.** Tipo valido e diverso da `server` (400) → `worker_id` presente (400) → `stessoWorker` → tipo della
credenziale (403) → `postazione` dichiarata uguale al `nome_host` della postazione della credenziale (403: il
`worker.toml` copiato su un altro PC) → `risolviDestinazione`: caselle dichiarate ∩ `worker_credenziale.caselle`
(`casellaAutorizzata`), le altre in avviso → `UpsertCasellaStore` → `DichiarazioneWorker` (postazione dalla
credenziale, IP, `outlook_ok`, caselle, `ultimo_arresto`, avviso) → `coda.Claim` in long-poll: `attesa_s` fuori da
(0, 25] diventa 20 → `ClaimConcluso` → 204 o il job (`coda.InJob`).

**Result.** `job_id`, `worker_id` e `lease_token` (UUID) obbligatori (400, e il log dice che il lavoro andrà
rifatto). `GetJob`: job inesistente → 404; errore del database → 500 (il worker ripete). Per uno stage `ok`:
`Verifica` (non valido → il `.parte` di quel token si toglie, 409), `preparaStage` (il `.parte` deve esistere;
sha256 e byte devono coincidere, altrimenti 422 **ritentabile**; destinazione per hash; `riusato` se quel
contenuto c'è già). Poi `Blocca`. Esito `errore`: `Fallisci` (con `definitivo`) e, se definitivo, lo stato
d'errore sull'entità del job. Esito `ok`: `applicaRisultato` per tipo, `Completa`, commit, 204.

**Dopo lo staging (`dopoStaging`).** Promozione del `.parte` (o scarto, se `riusato`) → `SetAllegatoStaging` →
`riallineaEntryID` → proposta dal nome (`classificazione.PropostaDaNome`) → rumore: hash già scartato per il
dominio del mittente (confidenza 95) o immagine vista in almeno tre messaggi dello stesso dominio (85) →
`scriviProposta` (motore del file: RFQ, altrimenti cliente controparte, altrimenti clienti del fornitore;
`Canonico` toglie i suffissi decorativi; codici nel nome nei dettagli; `codiceRevSicuri`;
`fascicolo.RevisioneProponibile` per un file caricato a mano) → rumore: `analizzato`, fine; estensione `zip`:
`estrai_archivio` (chiave `estrai:<allegato>`); altrimenti `coda.AccodaAnalisi`, e se non accoda niente
`applicaFattiEsistenti` applica i fatti già calcolati per quel contenuto.

**Analisi.** L'allegato del risultato deve essere quello del job (`PayloadAnalizzaAllegato.AllegatoID`),
altrimenti 422 definitivo → versione e hash di configurazione devono essere quelli del payload (422 definitivo)
→ `tipo_proposto` e `fonte` dentro l'enum (`enumValido`) → `analisi_fatti` per (sha256, versione,
configurazione) con i dettagli e l'`esito` → una proposta per l'allegato e per ogni proposta aperta con lo
stesso contenuto (`propostaDaAnalisi`: direzione del messaggio — un'offerta nostra in entrata senza «SO » nel
nome diventa `commerciale`, confidenza −30 —, `Canonico`, `codiceRevSicuri`, `RevisioneProponibile`) →
`strutturaNelleRfq`: se i dettagli hanno una struttura STEP, proposte di nodi, archi e rimozioni in ogni RFQ con
quel contenuto, con il motore del cliente di ciascuna. Nessuna riga della BOM cambia.

**Sync.** Casella dal job, altrimenti dal payload; senza, il risultato non è applicabile. Per ogni cartella
dell'esito `UpsertSyncCursore` sempre; la frontiera solo se `completa`: nello storico `storico_fino_a` = `dal`
della cartella (o del payload), negli altri modi `coperto_fino_a` = `al` della cartella (o del payload); senza
`al` la copertura non avanza.

**Upload.** Tentativo dichiarato (400) → `Verifica` (non valido: via il `.parte` del token, 409) →
`stageDelJob` (422 se il job non è il download di quell'allegato) → `Content-Length` oltre il limite:
allegato in errore e 413 senza leggere → scrittura in `_parti` con `MaxBytesReader` (oltre: 413; interruzione:
500) → di nuovo `Verifica` (non valido: file rimosso, 409) → 204. Né `path_staging` né lo stato si toccano.

**Contenuto.** L'upload al contrario: `Verifica` (409) → `analisiDelJob` (422 se il job non è l'analisi di
quell'allegato) → `path_staging` vuoto → 410 «usa Riscarica» → `nelloStaging`: un percorso non assoluto, fuori
dalla radice dello staging, o con `Server.Staging` non dichiarata → 410 con la stessa frase, e un errore nel
log con il percorso → file assente → 410 → byte con `Content-Length` e `X-Cockpit-Sha256`. Lo stato
dell'allegato non cambia: se il worker non riesce, lo dice con il suo result.

**Ingest.** Tentativo dichiarato (400) → la casella: `casella_id` del lotto; se manca, quella che il job nomina
(`GetJob` + `ingest.CasellaDelJob`: colonna `casella_id`, altrimenti il payload; job inesistente → 409);
`CasellaDefault` solo se il job non ne nomina nessuna → `ingest.RisolviCasella` fuori transazione: non censita o
disattivata → job fallito in modo definitivo e 422 → `casellaAutorizzata`: la casella deve stare in
`worker_credenziale.caselle` (un elenco vuoto non ne autorizza nessuna), altrimenti 403, niente scritto e il job
non fallisce → `Ingerisci`, che con la riga del job bloccata controlla che il job sia un `sync_outlook` o un
`rileggi_elemento` della stessa casella (`ErrLottoNonDelJob` → 403, niente scritto) → tentativo non valido 409,
guasto 500 (il worker ripete il lotto identico) → risposta con `durata_ms`.

**Estrazione di un archivio.** File non in staging → allegato in errore «usa Riscarica», job chiuso →
`estraiInContenuti` (cartella `zip.<token>`, ogni voce in `_contenuti` per hash, le voci già presenti non si
riscrivono) → archivio illeggibile → allegato in errore, job chiuso; oltre i limiti di `archivio` → si registra
ciò che è uscito, con `troncato` → in una transazione: per ogni voce un allegato figlio (`indice` = posizione
+ 1, origine dello zip), staging, proposta dal nome, analisi o fatti già presenti; lo zip riceve la proposta
`altro` e lo stato `analizzato`.

## Invarianti

- **Il token identifica.** Nome, tipo, postazione e caselle del claim vengono dalla credenziale; il `worker_id`
  del corpo deve solo coincidere. Un segreto di due credenziali non autentica nessuno; una credenziale mai
  generata non entra.
- **Le caselle della credenziale valgono per il claim e per l'ingest**, con la stessa regola
  (`casellaAutorizzata`): un worker riceve job e consegna posta solo per le caselle della sua credenziale.
- **Un lotto di posta si scrive solo con il tentativo di un job che consegna posta** (`sync_outlook`,
  `rileggi_elemento`) **per la stessa casella**; un lotto rifiutato (403) non lascia niente in database.
- Ogni richiesta autenticata aggiorna `ultimo_contatto` prima dell'handler; se non ci riesce, la richiesta
  prosegue.
- Heartbeat, result, ingest, upload e contenuto chiedono il tentativo (`job_id`, `lease_token`, `worker_id`); un
  predicato che non regge è 409, e un tentativo caduto per durata massima rimette il job `pronto`.
- Nel result la riga del job è bloccata prima di ogni scrittura, compresa la rinomina del file; ciò che
  `applicaRisultato` scrive sparisce con il rollback se `Completa` non passa.
- Nessun I/O lungo dentro la transazione del result: hash e cartelle prima, dentro solo la rinomina.
- Un file caricato resta in `_parti` finché il result valido dello stesso tentativo non lo promuove; un
  contenuto si chiama con il suo sha256 e non si scrive due volte.
- Upload e contenuto valgono solo per il job di **quell'**allegato (`stage_allegato`, `analizza_allegato`); il
  risultato di un'analisi vale solo per l'allegato del suo job.
- `/contenuto` consegna solo file che stanno sotto la radice dello staging del server.
- Un risultato che non si riesce ad applicare fa fallire il job in modo definitivo (422); l'unica eccezione
  ritentabile è l'hash del file che non torna.
- Codice e revisione che vengono da fuori non entrano in colonna oltre `classificazione.MaxCodice` e `MaxRev`:
  vanno interi nei dettagli (`codice_scartato`, `rev_scartata`), mai troncati.
- Un valore fuori enum non si converte a niente di «vicino»: è un errore.
- Le frontiere del sync avanzano solo per le cartelle dichiarate `completa`; `ultimo_received` si scrive sempre.
- Un EntryID nuovo senza `casella_id` nel payload non si scrive.
- I fatti di un'analisi sono del contenuto; la proposta è di ogni allegato, con la direzione del suo messaggio e
  le regole del suo cliente.

## Dipendenze

Importa `core/inbox/classificazione`, `core/inbox/ingest`, `core/registro/regole`, `core/rfq/documenti` (solo
`DentroLaRadice`, in `nelloStaging`), `core/rfq/fascicolo`, `platform/coda`, `platform/contratti/worker`,
`platform/db`, `platform/rete`, `platform/storage/archivio`, `platform/storage/nas`, `platform/storage/staging`:
tutte consentite dalla tabella di `internal/README.md`.

È importato da `app/runtime` (`ascolto.go`). `transport/web` lo usa a runtime solo attraverso l'interfaccia
`web.Server.Pipeline`, senza importarlo; lo importano però due suoi test L4 (`b87_db_test.go`,
`postazione_db_test.go`), cioè un `transport` che importa l'altro, fuori dalla tabella anche se solo nei test.

## Test

Tutti L4 (`//go:build integrazione`): `COCKPIT_TEST_DSN=… go test -tags integrazione -count=1 -p 1
./internal/transport/workerapi/`. I test E2E avviano il worker Python vero (`python` nel `PATH`); con
`COCKPIT_TEST_SENZA_PYTHON` sono SKIP. Nessun test L1.

| File | Che cosa prova |
|---|---|
| `rete_db_test.go` | W4: senza token, token sconosciuto, token di due worker; W11: il client Python rifiuta un certificato con un'impronta diversa; l'impronta senza HTTPS |
| `claim_db_test.go` | Q18 caselle dichiarate e non autorizzate, M2 presenza per worker, worker non censito o su un'altra postazione, `/worker/caselle`, il claim HTTP e la casella del job |
| `ingest_db_test.go` | un lotto per una casella che la credenziale non autorizza, o da una credenziale senza caselle (403, niente scritto, job ancora in corso); il lease del sync di un'altra casella o di un download (403); un lotto senza casella prende quella del suo job e non `casella_default`; il risultato di un'analisi per un altro allegato (422, job fallito); `result` distingue un job inesistente (404) da un errore del database (500); `/contenuto` con un `path_staging` fuori dallo staging (410) |
| `upload_db_test.go` | M7 upload legato al tentativo e promosso dal result, M8 oltre il limite, M13 token vecchio, hash diverso, result senza upload o su un altro allegato (contiene anche l'impianto di prova comune) |
| `caduta_db_test.go` | caduta dopo upload o result: niente duplicati |
| `contenuti_db_test.go` | stesso contenuto un file solo; voci uguali dentro lo zip |
| `contenuto_db_test.go` | payload senza percorsi del server; `/contenuto` solo al tentativo valido della sua analisi; E2E con il worker analisi vero, staging diverso, struttura STEP |
| `analisi_db_test.go` | i fatti arrivano a tutte le proposte aperte; configurazione diversa → non applicato |
| `riuso_fatti_db_test.go` | il secondo allegato con lo stesso contenuto riceve la proposta; fatti senza `esito` non riscrivono |
| `b85_db_test.go` | la struttura di uno STEP diventa proposte in ogni RFQ; E2E con il worker vero |
| `a4_db_test.go` | uno STEP nuovo non modifica la BOM |
| `copertura_db_test.go` | frontiere del sync (3C, 3E, 3F, 3H–3K e le due frontiere che non si toccano): buco notturno, crash a metà aggiornamento o bootstrap, finestra incompleta senza errore, parte vuota della finestra, quattro frontiere indipendenti, sovrapposizione |
| `misura_db_test.go` | nome di file lungo e codice fuori misura non rompono il result |
| `suffissi_db_test.go` | suffisso decorativo tolto in staging, nel caricamento interno, nell'analisi; posta del fornitore con le regole dei suoi clienti |
| `e2e_worker_db_test.go` | il worker Outlook vero (`prova_e2e.py`) chiude il job; il suo battito rinnova il lease |

Non provati qui: il risultato di `crea_bozza_outlook` e `riallineaEntryID`, `GET /sync/cursori` oltre
l'autenticazione (i test W4 e W11 la usano solo come sonda), un errore del database dentro `applicaRisultato`.
`EstraiArchivio` ha prove anche in `app/runtime/estrazione_db_test.go`; il controllo del lotto contro il job
anche in `core/inbox/ingest/lease_db_test.go`.

## Stato dell'implementazione

- Complete: credenziali individuali (voce 2.4), upload legato al tentativo (2.3), presenza all'ingresso di ogni
  rotta (0009), frontiere per cartella (3R, blocco 3), estrazione come job (4A), fatti per contenuto e loro riuso
  (1.12, B8.0), contenuto scaricabile dal worker analisi remoto e guardia sulle misure (7C.1), struttura STEP
  nelle RFQ (B8.5), caricamento interno (B8.7), suffissi decorativi (Fascicolo v3), ingest legato alla
  credenziale e al job del tentativo.
- `GET /api/v1/sync/cursori` non la chiama nessun worker: la finestra arriva nel payload. Restituisce le cartelle
  di tutte le caselle senza dire di quale casella sono.
- `rileggi_elemento`: il server lo accoda da «Riprova» su uno scarto di lettura, ma il worker Outlook non ha quel
  tipo e lo chiude come errore definitivo (difetto noto, `workers/workers_README.md` §5).
- `sposta_in_cartella`: il risultato si applica (EntryID), ma nessuna rotta accoda il job.
- `crea_bozza_outlook`: le rotte accodano sempre `allegati` vuoto; il campo porta percorsi del disco del worker.

Limiti noti:
- Il worker Outlook non ha un trattamento suo per il 403 dell'ingest: finisce fra gli errori generici del job
  (`workers/worker_outlook.py`, `Worker.esegui`) e diventa un result di errore non definitivo, quindi il job si
  ritenta e riceve lo stesso rifiuto finché non esaurisce i tentativi. Il server, da parte sua, non fa fallire
  il job per un 403.
- Ogni errore di `applicaRisultato` diventa un 422 definitivo, anche quando è un guasto transitorio (un deadlock,
  un file bloccato).
- `nelloStaging` esiste due volte, qui e in `transport/web/anteprima.go`, con la stessa regola.
  `EstraiArchivio` legge `path_staging` senza passare da quel controllo.
- `heartbeat` ignora un errore di decodifica del corpo: senza tentativo leggibile risponde 400 per il tentativo
  mancante, non per il JSON.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| cambiare l'effetto di un job riuscito | `workerapi.go:applicaRisultato` |
| una rotta nuova per i worker | `workerapi.go:Registra` e l'handler; il contratto in `platform/contratti/worker/tipi.go`, `workers/contratti.py`, `workers/genera_contratti.py` e i test L3 |
| capire un 401 o un 403 di un worker | `workerapi.go:auth`, `stessoWorker`, `claim`, `casellaAutorizzata`; per l'ingest anche `core/inbox/ingest/ingest.go:lottoDelJob` |
| capire perché un worker non riceve un job | `risolviDestinazione`, poi `coda.Claim` (capacità, casella, postazione, scadenza) |
| capire un 409 | il predicato in `platform/coda` e `nonValido` |
| capire perché un job è fallito con 422 | `risultatoNonApplicabile` e il motivo scritto nel job |
| capire un 410 di `/contenuto` | `contenuto.go:scaricaContenuto`, `nelloStaging` (il motivo è nel log del server) |
| la proposta dal nome o la regola del rumore | `dopoStaging`, `classificazione.PropostaDaNome` |
| la proposta che viene dall'analisi | `propostaDaAnalisi`, `applicaFattiEsistenti` |
| la struttura STEP nella RFQ | `strutturaNelleRfq`, `core/rfq/fascicolo` (`ApplicaStruttura`) |
| il limite di upload | `[server].max_upload_mb`, `upload.go:maxUpload` |
| l'estrazione degli zip | `archivi.go`, `platform/storage/archivio` |
| la frontiera del sync | il ramo `TipoJobSyncOutlook` di `applicaRisultato` |
| la casella di un lotto di posta | `workerapi.go:ingest` (casella del lotto, del job, predefinita), `core/inbox/ingest/ingest.go:CasellaDelJob` |

## Leggi anche

`internal/transport/README.md`, `internal/README.md`, `internal/platform/README.md` (coda, staging,
contratti), `internal/core/README.md` (ingest, fascicolo), `internal/core/inbox/ingest/README.md`,
`internal/app/README.md` (esecutore), `workers/workers_README.md`.
