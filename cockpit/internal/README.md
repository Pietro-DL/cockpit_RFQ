# `internal/` — com'è diviso `cockpit.exe`

## Scopo

Cinque aree, una regola sola per capire dove va un pezzo di codice: **chi può importare chi**.

```
                  cmd/cockpit   (i flag, quale cockpit.toml)
                       │
                       ▼
                  app/runtime   (l'avvio, il cablaggio, l'esecutore dei job del server)
                       │ monta
        ┌──────────────┴───────────────┐
        ▼                              ▼
   transport/web                  transport/workerapi
   browser: sessione, ruoli,      worker: X-Cockpit-Token individuale;
   pagine e frammenti htmx        claim, battito, result, ingest, upload,
        │                         contenuto; la pipeline dopo lo staging
        │                              │
        ├──────────────┬───────────────┤
        │              ▼               │
        │         ai/agente            │   (LLM, spento di default; lo usano web e runtime)
        ▼                              ▼
   core/inbox/     ingest · classificazione · aggancio · lettura
   core/rfq/       documenti · fascicolo
   core/registro/  regole · anagrafica · fornitori
        │
        ▼
   platform/       db (sqlc) ──▶ PostgreSQL · coda · config · fondazioni · migrazioni
                   rete · logfile · contratti/worker · storage/{staging,nas,archivio} · testutil
```

(i nomi nel disegno sono relativi a `internal/`)

Un solo listener (`[server].indirizzo`), due famiglie di rotte, due autenticazioni: la UI con la sessione
(`sessione` in DB, cookie `SameSite=Lax`, ruoli ordinati), l'API worker con il token individuale (sha256 in
`worker_credenziale`, da cui il server ricava nome, tipo, postazione e caselle). Il server **non apre mai
connessioni** verso i PC: sono i worker a chiamare (vedi `workers/workers_README.md`).

## Mappa di navigazione

| Cerco… | Area |
|---|---|
| riconoscimento della posta, triage, candidati di aggancio | `core/inbox/` (`classificazione`, `ingest`, `aggancio`) |
| come si mostra il corpo di una mail | `core/inbox/lettura` |
| lo schema di `cliente.regole`, le convenzioni di codice, i suffissi decorativi | `core/registro/regole` |
| anagrafiche, fornitori, i loro semi | `core/registro/` |
| il nome di una cartella o di un file sul NAS, la copia, l'integrità | `core/rfq/documenti` |
| la BOM, le proposte dagli STEP, il piano, la preparazione, l'editor | `core/rfq/fascicolo` |
| una rotta del browser, una schermata | `transport/web` (e `web/` per template e statici) |
| il protocollo con i worker | `transport/workerapi` + `platform/contratti/worker` |
| la coda, le capacità, l'instradamento dei job | `platform/coda` |
| DB, config, TLS, NAS, staging, migrazioni | `platform/` |
| l'assistente semantico | `ai/agente` |
| avvio, comandi della riga di comando, esecutore dei job del server | `app/runtime`, `cmd/cockpit/main.go` |

## Mappa dei README

| README | Di che cosa parla |
|---|---|
| [`../README.md`](../README.md) | il Cockpit per chi lo installa e lo usa: rami, installazione, configurazione, rete, prove |
| [`README.md`](README.md) | questo: le aree, le dipendenze, i flussi, i livelli di prova, lo stato |
| [`app/README.md`](app/README.md) | l'avvio passo per passo, i comandi, l'esecutore, che cosa ferma l'avvio |
| [`ai/README.md`](ai/README.md) | l'agente AI |
| [`core/README.md`](core/README.md) | le regole comuni di `core` |
| [`core/inbox/ingest/README.md`](core/inbox/ingest/README.md) | il lotto del worker, scarti, cursore, marcatori, ritriage |
| [`core/inbox/classificazione/README.md`](core/inbox/classificazione/README.md) | controparte, catena, codici, triage, atto, motore delle regole |
| [`core/inbox/aggancio/README.md`](core/inbox/aggancio/README.md) | i candidati di aggancio, di codice, verso una richiesta |
| [`core/inbox/lettura/README.md`](core/inbox/lettura/README.md) | il corpo di una mail pronto da leggere |
| [`core/registro/README.md`](core/registro/README.md) | regole del cliente, anagrafica, fornitori |
| [`core/rfq/documenti/README.md`](core/rfq/documenti/README.md) | nomi sul NAS, copia, ripresa, integrità |
| [`core/rfq/fascicolo/README.md`](core/rfq/fascicolo/README.md) | la BOM nel tempo, proposte, decisioni, piano, preparazione, editor |
| [`platform/README.md`](platform/README.md) | le regole comuni di `platform`, `logfile` e `testutil` |
| [`platform/config/README.md`](platform/config/README.md) | `cockpit.toml`, voce per voce |
| [`platform/db/README.md`](platform/db/README.md) | il codice sqlc, le query per file |
| [`platform/migrazioni/README.md`](platform/migrazioni/README.md) | il migratore |
| [`platform/fondazioni/README.md`](platform/fondazioni/README.md) | utenti, caselle, postazioni, credenziali dei worker |
| [`platform/rete/README.md`](platform/rete/README.md) | certificato, filtro del listener, token |
| [`platform/contratti/README.md`](platform/contratti/README.md) | i contratti JSON con i worker |
| [`platform/coda/README.md`](platform/coda/README.md) | la coda dei job, le tabelle per tipo, le capacità |
| [`platform/storage/README.md`](platform/storage/README.md) | staging, NAS, archivi |
| [`transport/README.md`](transport/README.md) | le regole comuni e la tabella completa delle rotte |
| [`transport/web/README.md`](transport/web/README.md) | le pagine dell'operatore, modulo per modulo |
| [`transport/workerapi/README.md`](transport/workerapi/README.md) | l'API dei worker e la pipeline dopo lo staging |
| [`../web/README.md`](../web/README.md) | template, CSS, `fascicolo.mjs`, pdf.js |
| [`../migrations/README.md`](../migrations/README.md) | lo schema, migrazione per migrazione |
| [`../workers/workers_README.md`](../workers/workers_README.md) | i worker Python e il protocollo visto da loro |
| `../_fasi/*.md` | **storici**: le fasi 0–3 e il checkpoint 3R come erano alla loro chiusura; i percorsi che citano sono quelli di prima della ristrutturazione di `internal/` |

## Dipendenze consentite

| Area | Può importare |
|---|---|
| `core/registro/regole` | **niente** del progetto |
| `core/inbox/classificazione` | `core/registro/regole` (il motore lavora sullo schema) |
| `core/inbox/lettura` | `core/inbox/classificazione` (solo `TagliaCatena`) |
| `core/rfq/documenti` | `core/inbox/classificazione` (`OggettoPulito`: il nome della cartella nasce dall'oggetto ripulito), `platform` |
| `core/*` (aggancio, ingest, registro, rfq) | gli altri `core/*`, `platform` |
| `platform/*` | solo `platform` e librerie (`testutil` anche il package radice, per le migrazioni incorporate) |
| `platform/coda` | `platform/storage/staging` (l'interfaccia `Staging`, per la guardia del doppio download) |
| `ai/agente` | `core`, `platform` |
| `transport/*` | `core` (compreso `core/inbox/lettura`, usato da `transport/web`), `ai`, `platform` |
| `app/runtime` | `core`, `ai`, `platform`, `transport` — è l'unica area che le può conoscere tutte insieme, perché è quella che monta il processo |
| `cmd/cockpit` | `app/runtime`, `platform/config` |

Frecce che esistono solo nei test: `transport/web` → `transport/workerapi` (le prove L4 di B8.7 e delle
postazioni) e `platform/migrazioni` → `core/inbox/classificazione` (le larghezze delle colonne). A runtime
`transport/web` riceve la pipeline dei worker come interfaccia, collegata da `app/runtime`.

## Eccezioni dichiarate

Logica di dominio che vive ancora nel livello HTTP, dichiarata perché esiste e non perché va bene (la
destinazione è una decisione dell'architetto; elenco completo in `transport/README.md`):

- **`transport/web`**: la conferma di una proposta (`allegati.go:confermaProposta`) e l'applicazione del
  piano di «Conferma Fascicolo» (`fascicolo_conferma.go:applicaPiano`); l'assegnazione a un componente e la
  correzione del codice con il percorso dei documenti (`fascicolo.go:assegnaAlComponente`,
  `correggiCodiceComponente`, `ripercorri`); la nuova RFQ, l'aggancio, «ignora» e il nome libero della
  cartella (`triage.go:nuovaRFQ`, `agganciaMessaggioAThread`, `ignora`, `cartellaLibera`); le scritture
  dell'anagrafica (`anagrafica.go:CreaCliente`, `AggiungiDominio`, `censisci.go`); la risposta di un
  fornitore (`richieste.go:rispostaFornitore`); le finestre di «Carica precedenti»
  (`routes_inbox.go:finestraStorico`).
- **`transport/workerapi`**: la pipeline dopo lo staging (`applicaRisultato`, `dopoStaging`,
  `propostaDaAnalisi`, `scriviProposta`, `motoreDelFile`, `codiceRevSicuri`, `applicaFattiEsistenti`,
  `strutturaNelleRfq`, `DopoCaricamento`, `archivi.go:EstraiArchivio`), riusata dal Fascicolo e
  dall'esecutore.
- **`core/rfq/documenti`**: il giro del ricognitore (`integrita.go:Ricognitore.Avvia`) è un loop di processo
  che sta in `core`.
- **`core/inbox/ingest`**: scrive `messaggio.thread_id` in un caso solo, il marcatore
  `CockpitRichiestaFornitore` sulla nostra posta in uscita (`marcatori.go`).

## Entry point e avvio

`cmd/cockpit/main.go` legge i flag, sceglie il file (con `-config` quello; senza, `cockpit.toml` della
cartella corrente se c'è, altrimenti quello accanto a `cockpit.exe`), compila `runtime.Opzioni` e
`config.Rete` e chiama **`runtime.Esegui`**, che è l'avvio:

1. `config.CaricaConRete` (valori predefiniti, file, avvisi, percorsi relativi dalla cartella del file, rete
   della riga di comando, validazione, staging);
2. il log (`ApriLog`: stdout e file), con gli avvisi della configurazione; da qui un errore d'avvio si scrive
   anche nel file;
3. il contesto, chiuso da Ctrl+C e da SIGTERM;
4. `-conta-anagrafiche` / `-anteprima-fornitori`: database in sola lettura, niente migrazioni, e si esce;
5. `migrazioni.Applica` (uno schema più recente del binario ferma l'avvio);
6. `Semina`: `fondazioni.SeedUtenti` e `fondazioni.Semina`, l'analizzatore corrente, la regola di una sola
   casella attiva sotto la 0004;
7. capacità (`coda.ImpostaCapacita`, `coda.AllineaCoda`);
8. `ingest.RicalcolaControparti`;
9. `-semina-anagrafica`, `-importa-fornitori`, `-migra`: e si esce;
10. servizi (`CostruisciServizi`: scheduler e cache partono qui);
11. TLS (`PreparaTLS`);
12. `Ascolta`: `web.Server` e `workerapi.Server`, la pipeline al Fascicolo, l'estrattore all'esecutore,
    **partono esecutore e ricognitore** (il guardiano del NAS solo con `nas_scrittura`), le rotte, poi
    `net.Listen`, il filtro delle reti, `Serve` o `ServeTLS`.

**Il server non semina mai l'anagrafica da solo**, nemmeno all'avvio. I due comandi che la scrivono
chiudono con `ingest.RitriageMolti` sulle sole chiavi appena scritte. Dettaglio in `app/README.md`.

## Flussi principali

- **Sync** — scheduler / «Aggiorna ora» / prima apertura dell'Inbox / «Carica precedenti» → job
  `sync_outlook` con il modo e una finestra chiusa per cartella → claim del worker → lotti su
  `/ingest/messaggi`, accettati solo per la casella del job → `ingest` scrive i fatti e la controparte;
  `classificazione` taglia la catena nei testi dell'interpretazione e fa il triage; `aggancio` calcola i
  candidati → la frontiera avanza solo se la cartella è stata percorsa per intero. Package:
  `transport/workerapi`, `core/inbox/ingest`, `core/inbox/classificazione`, `core/inbox/aggancio`, `platform/coda`.
- **Leggere una mail** — pannello dell'Inbox o pagina della RFQ → `lettura.Presenta` sul corpo memorizzato:
  rumore chiuso, tabelle, storia citata, testo originale a un clic. Package: `transport/web`, `core/inbox/lettura`.
- **Censisci dall'Inbox** — «Da validare» → `censisci` → fornitore o cliente in anagrafica →
  `ingest.Ritriage(indirizzo, dominio)` sui soli messaggi **non decisi**. Package: `transport/web`,
  `core/registro`, `core/inbox/ingest`.
- **Nuova RFQ / Aggancia** — `FOR UPDATE` sul messaggio → `thread_offerta` con la cartella da
  `rfq/documenti/path.go` (la seconda con lo stesso nome prende « (2)») → identificativi selezionati → job
  `crea_cartella_thread` → i download scelti nel form → la preparazione del Fascicolo, nella stessa
  transazione. «Nuova RFQ» non vale per la posta di un fornitore. Package: `transport/web`,
  `core/inbox/classificazione`, `core/rfq/fascicolo`, `platform/coda`.
- **Preparazione del Fascicolo** (B8.7b) — alla nascita e all'aggancio di una RFQ, e all'apertura del
  Fascicolo (per chi è almeno `operatore`): i codici confermati della richiesta diventano prodotti finiti;
  i file utili scendono nello staging, si estraggono e si analizzano; un poll mostra l'avanzamento. Niente
  arriva sul NAS. Package: `transport/web`, `core/rfq/fascicolo`, `platform/coda`.
- **Allegato → NAS** — «Scarica», staging automatico o preparazione → `stage_allegato` → contenuto in
  `_contenuti` con lo sha256 per nome → proposta dal nome e regola del rumore → `estrai_archivio` (solo per
  uno zip) o `analizza_allegato` (solo se i fatti di quel contenuto non ci sono già) → `documento_proposta`
  → conferma (la proposta dal pannello, «✓ Conferma» o «Conferma Fascicolo») → `documento` + `copia_nas`,
  eseguita dal server. Un file caricato a mano («Carica nuova versione interna», «Importa dal NAS») entra
  come allegato `manuale` della nota interna della RFQ e fa la stessa strada (`DopoCaricamento`), senza
  download. Package: `transport/web`, `transport/workerapi`, `core/rfq/*`, `app/runtime`, `platform/storage`.
- **Dagli STEP alla BOM** — il result dell'analisi di uno STEP → `strutturaNelleRfq` →
  `fascicolo.ApplicaStruttura` in ogni RFQ che ha quel contenuto (codici passati da `Canonico`) → proposte
  di nodi, archi, quantità e rimozioni → le decide una persona, nella schermata o nell'editor
  (`POST …/bom/applica` → `ApplicaStrutturaVoluta`). Package: `transport/workerapi`, `transport/web`,
  `core/rfq/fascicolo`.
- **Pagina Richieste** — `GET /richieste`: una card per RFQ, le schede dei prodotti con l'anteprima 2D, un
  poll che risponde 204 quando niente è cambiato. Package: `transport/web`, `platform/db`.
- **Integrità NAS** — il ricognitore confronta `documento` con i file veri e scrive `nas_anomalia`. Package:
  `core/rfq/documenti`, `platform/storage/nas`.
- **Apri in Outlook** — job con il `postazione_id` della sessione: solo il worker di quella postazione lo prende.
- **Login e postazione** — `POST /login` → bcrypt → `sessione` → abbinamento per IP al worker che ha fatto claim.
- **Richiesta a un fornitore (7B)** — `POST /thread/{id}/richiesta` → `richiesta_fornitore` (bozza) → bozza
  marcata in Outlook → il sync della Posta inviata riporta i marcatori → `ingest/marcatori.go` la segna
  inviata. Dal blocco 8 nessuna pagina ha più il form che chiamava la rotta: la richiesta tornerà nella parte
  della fattibilità, per lavorazione di un componente. La rotta resta. Il flusso a livello di RFQ è tolto
  **di proposito**: non va reintrodotto prima del modello delle lavorazioni per componente.

## Invarianti

I **tre strati** — fatto / interpretazione / decisione — valgono ovunque: un'interpretazione non scrive mai
una decisione. Nessun `thread_id` dall'ingest (l'unica eccezione è la nostra firma: il marcatore sulla
posta in uscita); nessun `documento` senza conferma; niente sul NAS senza una decisione; nessun invio da
`crea_bozza`; nessuna sovrascrittura sul NAS. Il corpo originale di un messaggio non si modifica mai: il
taglio della catena decide che cosa viene **dato in pasto** all'interpretazione, `lettura` decide come si
mostra, e mai l'HTML della mail entra nella pagina. Dettaglio in `core/README.md` e `transport/README.md`.

## Test

| Livello | Che cos'è | Comando |
|---|---|---|
| L1 | test Go puri: **nessun database**, anche con `COCKPIT_TEST_DSN` impostata (ogni test che lo usa ha il tag `integrazione`); più build, `go vet`, compilazione per Linux, sintassi degli script | `go test ./...` |
| L2 | test Python dei worker | `python -m pytest -q workers` |
| L3 | contratto worker ↔ server contro gli schemi di `contracts/` | `go test -count=1 ./internal/platform/contratti/worker/` e `python -m pytest -q workers/tests/test_contratti.py` |
| L4 | integrazione su PostgreSQL di prova, compresi i cinque TestE2E con i worker Python veri | `COCKPIT_TEST_DSN=… go test -tags integrazione -count=1 -p 1 ./...` |
| L7 | nel browser (Playwright su Edge): Inbox, anteprima dei PDF, Richieste, Fascicolo, e lo ZIP dalla mail al Fascicolo con il `cockpit.exe` vero | `go test -tags "integrazione browser" -count=1 -run TestL7 ./internal/transport/web/ ./cmd/cockpit/` |
| L5, L6, L8, L9 | prove reali (Outlook ed Exchange veri, il replay dell'agente, il banco a due PC e la share del NAS): si fanno a mano e si annotano nel registro degli esiti reali | — |

`-p 1` non è un vezzo: i test L4 condividono un database. `COCKPIT_TEST_DSN` deve portare a un database il
cui nome **risolto** contiene «test» (`testutil.DatabaseDiTest`, con `pgconn.ParseConfig`: vale anche per un
DSN `chiave=valore` o con `?dbname=`), altrimenti il test si ferma; senza la variabile i test L4 sono SKIP,
mai PASS. `COCKPIT_TEST_SENZA_PYTHON=1` salta i TestE2E. `scripts/prova-tutto.ps1` lancia L1, L3, L4 e L2 e
non lancia L7. I test Go stanno nel ramo `-qa` (vedi il README principale, «I rami del repository»).

## Stato dell'implementazione

Una riga per package. «In attesa» vuol dire che una parte esiste in codice o in schema ma non è ancora
usata; il dettaglio e i limiti noti stanno nel README del package.

| Package | Completo | Parziale o in attesa |
|---|---|---|
| `app/runtime` | avvio, comandi (quelli in sola lettura non migrano), errore d'avvio anche nel log, SIGTERM; esecutore di `copia_nas`, `crea_cartella_thread`, `estrai_archivio`, `analizza_messaggio_ai`, con i fallimenti definitivi; guardiano del NAS | `sposta_nas` instradato al server ma non eseguito (B8.8); `backup_db` non lo accoda nessuno; l'esecutore non rinnova il lease ed è una goroutine sola |
| `ai/agente` | il giro del checkpoint 3R §9: pulsante, job, contesto, prompt, schema, grounding, idempotenza | **spento** finché il gate non è chiuso; il Dossier e il contratto nuovo dell'agente non esistono ancora; il prompt riceve il corpo grezzo |
| `core/inbox/classificazione` | controparte, catena (IT/EN/DE/FR, Thunderbird), codici e triage con la precedenza, atto e legame, motore delle regole con i suffissi, `CodiceRev` sui nostri nomi NAS | parte del vocabolario degli atti; `altro` senza un ramo suo; verifica del taglio sul corpus reale; il parser delle intestazioni citate |
| `core/inbox/ingest` | lotto transazionale legato al job e alla casella, scarti e replay, presenze per casella, D30, controparte e ritriage, marcatori | la controparte `manuale` non la scrive nessuno; solo il canale Outlook; portale e scadenza si cercano sul corpo intero |
| `core/inbox/aggancio` | R0–R5, candidati di codice e verso una richiesta, tagli per carattere | R0/R1 non guardano `unito_in` |
| `core/inbox/lettura` | pannello dell'Inbox e pagina della RFQ: rumore, tabelle, storia, oggetto | non la usano l'interpretazione, l'agente e i nomi delle cartelle NAS (per scelta) |
| `core/registro` | regole (D17, D39, suffissi), seme dei clienti, import dei fornitori con i doppioni risolti nell'anteprima | alcuni campi delle regole sono solo mostrati; le note del cliente non le scrive nessuno |
| `core/rfq/documenti` | nomi e cartelle, copia, ripresa, integrità, verifica per l'anteprima | lo spostamento sul NAS (B8.8): `AccodaSpostamento` senza chiamanti, `nas_orfano` e `nas_creazione` non scritti; `RimuoviCartella` senza rotta |
| `core/rfq/fascicolo` | versioni e gate, archiviazione, sostituzioni, deroghe, proposte e decisioni, rianalisi, codici della RFQ, albero, modifiche a mano, preparazione, piano, editor, suffissi | un prodotto della richiesta tolto o corretto torna alla preparazione successiva (l'identificativo non si aggiorna) |
| `platform/config` | tutte le voci, avvisi sulle voci sconosciute, percorsi dalla cartella del file, `log_livello` a quattro valori | l'esempio differisce di proposito dai default su tre voci (sync ogni 30 s, analizzatore 3, staging automatico) |
| `platform/db` | 438 query in 28 file | 47 query senza chiamanti e 10 solo dai test: funzioni non ancora costruite (anagrafica «Altro», B8.8, lettura di una versione congelata, …) |
| `platform/migrazioni` | schema alla 0021; verifica statica; rifiuto di uno schema più recente | nessun controllo che un file già applicato non sia cambiato |
| `platform/fondazioni` | seed di utenti, caselle, postazioni, credenziali; password vera obbligatoria per un utente nuovo | il cambio password dalla UI non c'è; il seed non disattiva chi non è più nel file (lo dice nel log) |
| `platform/coda` | accodamento, claim, lease, tentativi e priorità per tipo, capacità, instradamento, scheduler | `sposta_nas` senza chi lo accoda; `rileggi_elemento` accodato ma non eseguito dal worker Outlook |
| `platform/rete`, `platform/contratti`, `platform/storage`, `platform/logfile`, `platform/testutil` | completi | il contratto `RisultatoSposta` non lo usa nessuno (il worker risponde con `RisultatoElemento`) |
| `transport/web` | Inbox, triage, pagina della RFQ, Richieste, Fascicolo v3, anteprima, amministrazione, postazioni | la schermata dell'anagrafica «Altro» e «Censisci come Altro» non esistono; il form delle richieste ai fornitori è tolto; il frammento del divieto (403) non si vede sulle richieste htmx; logica di dominio ancora qui (eccezioni sopra) |
| `transport/workerapi` | le otto rotte, credenziali individuali, upload e download legati al tentativo, lotti legati al job, pipeline dopo lo staging | `/sync/cursori` non lo chiama nessuno; nessuno accoda `sposta_in_cartella`; il worker Python tratta un 403 sull'ingest come un errore qualunque |
| `web/` (template e statici) | Fascicolo v3, B8.7b, Richieste, corpo delle mail | nessun visore 3D |
| `workers/` (Python) | sync, download, apri, segna letto, bozza, analisi di PDF e STEP | `rileggi_elemento` non c'è nel `dispatch`; i `parametri` dell'analizzatore non si usano |

## Dove intervenire

| Voglio… | Vai in |
|---|---|
| aggiungere una rotta | `transport/README.md` |
| aggiungere un tipo di job | `platform/README.md` (enum, contratto), `platform/coda` e `app/runtime` |
| cambiare una regola del cliente o del triage | `core/README.md` |
| cambiare come si legge una mail | `core/inbox/lettura/README.md` |
| aggiungere una migrazione o una query | `platform/README.md`, `../migrations/README.md` |
| toccare l'agente | `ai/README.md` |
| capire perché il server non parte | `app/README.md` («Che cosa ferma l'avvio») |

## Leggi anche

`core/README.md`, `platform/README.md`, `transport/README.md`, `ai/README.md`, `app/README.md`,
`../workers/workers_README.md`, `../README.md`.
