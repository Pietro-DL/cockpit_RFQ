# `internal/` — com'è diviso `cockpit.exe`

Cinque aree, una regola sola per capire dove va un pezzo di codice: **chi può importare chi**.

```
   browser (operatore)                          worker Python (postazioni)
        │ sessione (cookie)                            │ X-Cockpit-Token individuale
        ▼                                              ▼
   transport/web  ─────────────┐          ┌──── transport/workerapi
   (HTMX, ruoli, form)         │          │      (claim, heartbeat, result, ingest, upload, archivi)
                               ▼          ▼
                              jobs  (coda, lease, scheduler, capacità, esecutore server,
                               │          │   cache dei contenuti, ricognitore NAS)
              core/inbox/ingest  core/domain  core/inbox/aggancio  ai/agente
                     (fatti → DB) (interpretazione) (candidati)    (LLM, spento di default)
                               │
                        platform/db  (sqlc)  ──►  PostgreSQL
                               │
        platform/storage · platform/logfile · platform/rete · platform/config
```

(i nomi nel disegno sono relativi a `internal/`)

Un solo listener (`[server].indirizzo`), due famiglie di rotte, due autenticazioni: la UI con la sessione
(`sessione` in DB, cookie `SameSite=Lax`, ruoli ordinati), l'API worker con il token individuale (sha256 in
`worker_credenziale`, da cui il server ricava nome, tipo, postazione e caselle). Il server **non apre mai
connessioni** verso i PC: sono i worker a chiamare (vedi `workers/workers_README.md`).

## Mappa di navigazione

| Cerco… | Area |
|---|---|
| riconoscimento della posta, triage, candidati di aggancio | `core/` (`domain`, `inbox/ingest`, `inbox/aggancio`) |
| una rotta del browser | `transport/web` |
| il protocollo con i worker | `transport/workerapi` + `platform/contratti/api` |
| DB, config, TLS, NAS, staging, migrazioni | `platform/` |
| l'assistente semantico | `ai/agente` |
| anagrafiche, regole del cliente, lavorazioni, fornitori | `core/registro/` |
| la coda, le capacità, l'esecutore del server, l'integrità NAS | `internal/jobs` |
| avvio e cablaggio | `cmd/cockpit/main.go` |

## Dipendenze consentite

| Area | Può importare |
|---|---|
| `core/domain` | **niente** del progetto |
| `core/*` (aggancio, ingest, registro) | `core/domain`, `platform` |
| `platform/*` | solo `platform` e librerie |
| `ai/agente` | `core`, `platform` |
| `transport/*` | `core`, `ai`, `platform`, `jobs` |
| `jobs` | `core`, `ai`, `platform` |
| `cmd/cockpit` | tutto |

Eccezioni ancora aperte, dichiarate perché esistono e non perché vanno bene: `transport/workerapi/archivi.go`,
`applicaRisultato` e `propostaDaAnalisi` prendono decisioni di dominio dentro il livello HTTP.

## Entry point

`cmd/cockpit/main.go`: `config.Carica` → `logfile` → `migrazioni.Applica` → `fondazioni.Semina` → capacità
(`jobs.ImpostaCapacita`, `jobs.AllineaCoda`) → `ingest.RicalcolaControparti` → `rete` (TLS) →
`jobs.Scheduler.Avvia` → `jobs.Cache.Avvia` → `EsecutoreServer.Avvia` → `Ricognitore.Avvia` → listener con
`web` + `workerapi`.

I **lavori amministrativi** della riga di comando si fermano prima del listener e poi escono, uno alla volta:
`-semina-anagrafica`, `-anteprima-fornitori`, `-importa-fornitori`, `-conta-anagrafiche`, `-migra`. I due che
scrivono chiudono con `ingest.RitriageMolti` sulle sole chiavi appena scritte. **Il server non semina mai da
solo**, nemmeno all'avvio.

## Flussi principali

- **Sync** — scheduler / «Aggiorna ora» / prima apertura dell'Inbox / «Carica precedenti» → job `sync_outlook`
  con il modo e una finestra chiusa per cartella → claim del worker → lotti su `/ingest/messaggi` → `ingest`
  scrive i fatti, risolve la controparte, taglia la catena, chiede il triage a `domain`, calcola i candidati →
  la frontiera avanza solo se la cartella è stata percorsa per intero. Package: `transport/workerapi`,
  `core/inbox/ingest`, `core/domain`, `core/inbox/aggancio`, `jobs`.
- **Censisci dall'Inbox** — «Da validare» → `censisci` → fornitore o cliente in anagrafica →
  `ingest.Ritriage(indirizzo, dominio)` sui soli messaggi **non decisi**. Package: `transport/web`,
  `core/registro`, `core/inbox/ingest`.
- **Nuova RFQ** — `FOR UPDATE` sul messaggio → `thread_offerta` con la cartella da `domain/path.go` →
  identificativi selezionati → job `crea_cartella_thread`. Package: `transport/web`, `core/domain`, `jobs`.
- **Allegato → NAS** — «Scarica» o staging automatico → `stage_allegato` → contenuto in `_contenuti` con lo
  sha256 per nome → eventuale `estrai_archivio` → `analizza_allegato` → `documento_proposta` → conferma →
  `documento` + `copia_nas`. Package: `jobs`, `platform/storage`, `transport/workerapi`.
- **Integrità NAS** — il ricognitore confronta `documento` con i file veri e scrive `nas_anomalia`. Package:
  `jobs/integrita.go`, `platform/storage/nas`.
- **Apri in Outlook** — job con il `postazione_id` della sessione: solo il worker di quella postazione lo prende.
- **Login e postazione** — `POST /login` → bcrypt → `sessione` → abbinamento per IP al worker che ha fatto claim.
- **Richiesta a un fornitore (7B)** — RFQ › «Nuova richiesta» → `richiesta_fornitore` (bozza) → bozza marcata in
  Outlook → il sync della Posta inviata riporta i marcatori → `ingest/marcatori.go` la segna inviata.

## Invarianti

I **tre strati** — fatto / interpretazione / decisione — valgono ovunque: un'interpretazione non scrive mai una
decisione. Nessun `thread_id` dall'ingest (l'unica eccezione è la nostra firma: il marcatore sulla posta in
uscita); nessun `documento` senza conferma; nessun invio da `crea_bozza`; nessuna sovrascrittura sul NAS.
Il corpo originale di un messaggio non si modifica mai: il taglio della catena decide che cosa viene **dato in
pasto** all'interpretazione, non che cosa resta in `messaggio.corpo_testo`. Dettaglio in `core/README.md`.

## Test

| Livello | Che cos'è | Comando |
|---|---|---|
| L1 | test puri, senza database | `go test ./...` |
| L3 | contratto worker ↔ server contro gli schemi di `contracts/` | `go test -count=1 ./internal/platform/contratti/api/` e `pytest` in `workers/` |
| L4 | integrazione su PostgreSQL di prova | `COCKPIT_TEST_DSN=… go test -tags integrazione -count=1 -p 1 ./...` |
| L7 | l'Inbox in un browser vero (Playwright su Edge) | `go test -tags "integrazione browser" -count=1 -run TestL7 ./internal/transport/web/` |

`-p 1` non è un vezzo: i test L4 condividono un database. `COCKPIT_TEST_DSN` deve contenere «test» nel nome,
altrimenti `platform/testutil` si rifiuta; senza la variabile i test L4 sono SKIP, mai PASS.

## Dove intervenire

| Voglio… | Vai in |
|---|---|
| aggiungere una rotta | `transport/README.md` |
| aggiungere un tipo di job | `platform/README.md` (enum, contratto) e `internal/jobs` |
| cambiare una regola del cliente o del triage | `core/README.md` |
| aggiungere una migrazione o una query | `platform/README.md` |
| toccare l'agente | `ai/README.md` |

## Leggi anche

`core/README.md`, `platform/README.md`, `transport/README.md`, `ai/README.md`, `app/README.md`,
`workers/workers_README.md`, `README.md`.

---

**Cambia in B**: `core/domain` si divide in `core/inbox/classificazione`, `core/registro/regole` e
`core/rfq/documenti`; `internal/jobs` si divide fra `platform/coda`, `platform/storage/staging`,
`core/rfq/documenti` e `app/runtime`; `platform/contratti/api` diventa `platform/contratti/worker`;
`cmd/cockpit/main.go` si svuota in `app/runtime`.
