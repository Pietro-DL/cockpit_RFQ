# Cockpit RFQ — repository

Implementazione di `SPEC_Architettura_Cockpit_RFQ.md`: blocchi 0–3 (toolchain, schema, ingest + worker Outlook,
worker analisi), blocco 4 (triage in UI: Nuova RFQ / Aggancia / Ignora) e il nucleo del blocco 5 (download su
richiesta, conferma → documento → copia sul NAS). Stato al 13/09/2026, verificato sul PC di sviluppo contro la
casella `pietro.spinozzi@studio.unibo.it` in **Outlook classico**. Stato dettagliato in `../specs/STATO.md`.

**Regola cardine dal 13/09:** nessun file viene scaricato automaticamente. Il sync registra gli allegati (FATTO)
e una proposta dal solo nome; sul disco vanno solo i file che l'operatore spunta dentro una RFQ.

## Avvio in sviluppo

Prerequisiti già presenti su questo PC: Go 1.27, PostgreSQL 18 (DB `cockpit_dev`, ruolo `cockpit`),
`sqlc` in `%USERPROFILE%\go\bin`, Python 3.11 con `pywin32`, `pydantic`, `pymupdf`, Outlook classico con profilo configurato.

```powershell
cd C:\promatec\cockpit
powershell -ExecutionPolicy Bypass -File scripts\avvia-dev.ps1   # compila e apre 3 finestre: cockpit.exe, worker Outlook, worker analisi
powershell -ExecutionPolicy Bypass -File scripts\ferma-dev.ps1   # ferma tutto
```

Equivalente a mano: `go build -o cockpit.exe .\cmd\cockpit`, `.\cockpit.exe -config cockpit.toml`
(http://127.0.0.1:8080, login `PS` / `cockpit`), `python workers\worker_outlook.py`, `python workers\worker_analisi.py`.
Log: `..\_staging\log\{cockpit,worker_outlook,worker_analisi}.log`. La testata del browser mostra se i worker
sono **attivi** o **OFFLINE** (nessun claim da più di 60 s); i worker sopravvivono al riavvio del server.

Il server accoda `sync_outlook` ogni 60 s (`[outlook].intervallo_sync_s`); il worker legge `Inbox` e `Sent Items`
dal cursore (prima volta: da `[outlook].dal`) e invia i messaggi a `POST /api/v1/ingest/messaggi`.
**Carica precedenti** in Inbox accoda una finestra di 30 giorni prima di quanto già coperto (cursore `storico_fino_a`).

Flusso operativo dall'Inbox del browser:

1. messaggio orfano → **Nuova RFQ** (cliente/buyer anche creati inline, codici proposti, cartella calcolata,
   checklist allegati pre-spuntata) oppure **Aggancia a…** (ricerca sui thread aperti) oppure **Ignora**;
2. la RFQ nasce con la cartella `<radice>\<CLIENTE>\WIP\<aaaa mm gg Cognome Oggetto>\{ELENCO DISEGNI, OFFERTE FORNITORI}`
   e tutti i messaggi della stessa conversazione la seguono (anche i successivi, in automatico);
3. gli allegati spuntati vengono scaricati in staging (zip estratti voce per voce), analizzati (cartiglio, STEP)
   e proposti; **Conferma → NAS** crea il `documento` e la copia verificata con hash; **Scarta** chiude la proposta;
4. `/thread/{id}` (schermata B) mostra la chat della RFQ, i documenti sul NAS e il fascicolo.

Restano **Apri in Outlook**, **Segna letto**, **Rispondi → bozza in Outlook** (l'invio resta manuale: `consenti_invio = false`).

Utilità:

```powershell
bash scripts/db-reset.sh                   # ricrea lo schema da zero (una sola migrazione fino alla produzione)
sqlc generate                              # rigenera internal/db dopo aver toccato migrations/ o internal/db/queries/
go test ./...                              # unitari (dominio, NAS, zip, template)
$env:COCKPIT_TEST_DSN="postgres://cockpit:cockpit_dev@localhost:5432/cockpit_dev"; go test ./internal/ingest/   # integrazione su DB
python -m pytest -q workers/test_worker_analisi.py
python workers/genera_contratti.py         # rigenera contracts/*.schema.json dai modelli pydantic
```

## Struttura

```
cmd/cockpit/main.go        avvio: config, pool, migrazione, seed utenti, scheduler, esecutore server, router
embed.go                   embed.FS di migrations/, web/templates, web/static
internal/config            cockpit.toml
internal/api               contratti JSON worker ↔ server (tipi Go; speculari a workers/contratti.py)
internal/db                sqlc: queries/*.sql → codice generato (non modificare a mano)
internal/domain            regole pure + test: codici, proposta dal nome file, portale, scadenza, triage, nome/cognome, percorsi NAS
internal/ingest            FATTO (messaggio, allegato) + proposta economica + aggancio automatico + triage/portale
internal/archivio          estrazione zip in staging (zip-slip, limiti) → allegati figli
internal/jobs              coda: accoda idempotente (un solo job PENDENTE per chiave), claim/lease, scheduler, esecutore 'server' (NAS), stage/analisi
internal/nas               scrittore NAS: .parte + verifica hash, mai sovrascrive, long-path
internal/workerapi         /api/v1/jobs/{claim,heartbeat,result}, /api/v1/ingest/messaggi (token X-Cockpit-Token); dopo-staging (zip, rumore, analisi)
internal/web               HTML+HTMX: login, /inbox, /messaggio/{id} (+triage, scarica), /thread/{id}, /proposta/{id}/{conferma,scarta}, /cruscotto, /admin/job
web/templates, web/static  template html/template, style.css, htmx 2.0.4
migrations/0001_schema.sql l'unica migrazione (30 tabelle, 5 viste, 31 enum)
contracts/*.schema.json    JSON Schema generati da workers/contratti.py
workers/                   worker_outlook.py, worker_analisi.py (loop resilienti, log rotante), outlook_com.py (COM), contratti.py (pydantic), worker.toml
scripts/                   avvia-dev.ps1, ferma-dev.ps1, db-reset.sh
```

## Come si parlano i pezzi

```
Outlook classico ◀─COM─ worker_outlook.py ─HTTP 127.0.0.1:8080─▶ cockpit.exe ◀─pgx─▶ PostgreSQL
                        worker_analisi.py ─HTTP────────────────▶     │
                                   browser (HTML+HTMX) ◀────────────┘  (cookie di sessione, DB)
```

- **Identità del messaggio** = Internet Message-ID (`messaggio.chiave_esterna`); `entry_id`/`store_id` stanno nel
  satellite `messaggio_outlook`. I job verso Outlook portano anche il Message-ID: se l'EntryID non vale più
  (elemento spostato) il worker lo ritrova per Message-ID e il server riallinea `messaggio_outlook`.
- **Coda job** in PostgreSQL: `FOR UPDATE SKIP LOCKED`, lease per tipo (120 s / 300 s), 5 tentativi con backoff,
  `chiave_idempotenza` unica **fra i job pendenti** (indice parziale: un job fatto non impedisce di riaccodarne uno
  uguale). Priorità 1 = azione dell'utente (apri, bozza, download, cartella, copia NAS), 2 = sync storico, 5 = sync, 6 = analisi.
  `worker_presenza` registra l'ultimo claim per tipo di worker (badge in testata).
- **Tre strati per i file**: `allegato` (FATTO, scritto dal worker; niente su disco finché l'operatore non chiede)
  → `documento_proposta` (INTERPRETAZIONE: a ingest dal nome file, poi raffinata dopo il download da hash/zip e
  dal worker-analisi finché resta `aperta`) → `documento` (DECISIONE dell'operatore; solo questa accoda `copia_nas`).
  La conferma di un disegno con codice crea il `componente`; `v_fascicolo` calcola la completezza.
- **NAS**: `[nas].radice` È la cartella «PREVENTIVI DA FARE»; sotto, `cliente.cartella_nas\WIP\<aaaa mm gg Cognome Oggetto>`;
  con la cartella nascono solo le sottocartelle con `cartella_documento.crea_sempre` (ELENCO DISEGNI, OFFERTE FORNITORI),
  le altre alla prima copia.

## Riconciliazione dello schema (RFQ_plan §0.2 vs SPEC §4.2)

Dove i due documenti divergono vince la SPEC (più recente, «risponde a RFQ_plan.md»). Identificatori in
`snake_case` minuscolo perché PostgreSQL ripiega comunque gli identificatori non quotati e sqlc genera Go pulito.

**Presenti come da spec/piano:** utente, cliente (+`portale_url`, `portale_note`), dominio_cliente, buyer
(senza `referente_cartella`), fase_catalogo, transizione, regola, thread_offerta (senza backfill/`portale_url`;
`cartella_relativa` invece di `Cartella_NAS` assoluta), identificativo_thread (`origine` enum + `confermato_da`),
componente (FK thread, `padre_id`, note fattibilità), fase_log, conversazione, messaggio, messaggio_outlook,
allegato (con `contenitore_id` per gli zip), riferimento_portale, documento_proposta, proposta_triage, documento,
documento_provenienza, cartella_documento, fabbisogno_documento, deroga_fabbisogno, hash_rumore, job,
viste `v_fascicolo`, `v_inbox`, `v_cruscotto`.

**Aggiunte perché mancavano ma sono necessarie al funzionamento descritto:**

| Tabella / colonna | Perché |
|---|---|
| `schema_versione` | SPEC §4.1: migrazione applicata all'avvio solo se assente |
| `utente.password_hash`, `utente.ruolo`, `sessione` | SPEC §5.4 (login bcrypt, cookie) ma nessuna tabella lo prevedeva |
| `sync_cursore` (+ `storico_fino_a`) | SPEC §3.2 passo 1: «chiede al server il cursore della casella»; il sync storico ricorda fin dove è arrivato |
| `worker_presenza` | ultimo claim per worker: la UI segnala «OFFLINE» invece di lasciar crescere la coda in silenzio |
| `bozza` | Le mail preparate dal Cockpit (risposte, solleciti) vanno tracciate: stato, EntryID, poi collegate alla mail inviata |
| `messaggio.corpo_html` | Per rendere il Cockpit un vero frontend di Outlook serve l'HTML (da sanificare prima del rendering) |
| `messaggio.parent_messaggio_id` | SPEC §3.2: i `.msg` annidati producono messaggi figli |
| `messaggio_outlook.categorie/non_letto/flag_stato` | Stato di lettura e flag visibili in Inbox senza aprire Outlook |
| `thread_offerta.cartella_creata` | Esito del job `crea_cartella_thread` |
| `documento.stato_nas/errore_nas/scritto_il` | SPEC §4.2 e §5.3 (in_coda → scritto/errore) |
| `cartella_documento.crea_sempre` | quali sottocartelle nascono con la cartella RFQ (convenzione: ELENCO DISEGNI, OFFERTE FORNITORI) |
| `buyer.telefono` | Note WhatsApp/telefono (SPEC §6.2) |
| `v_thread_fase`, `v_thread_bloccanti`, `v_inbox.ignorato` | Fase corrente con SLA; contatore bloccanti; filtro «ignorati» dell'Inbox |

**Lasciate fuori dalla fase 1 (da decidere, non per dimenticanza):**

| Del vecchio `docs/0001_schema.sql` | Motivo |
|---|---|
| `Prodotto` | La SPEC mette `componente` direttamente sotto il thread (`tipo='finito'`, `padre_id NULL` = radice) e i codici finiti in `identificativo_thread`. `fase_log` è quindi per thread. Se servirà una fase per singolo prodotto dentro la stessa RFQ, si aggiunge `fase_log.componente_id`. |
| `Lacuna`, `Segnale`, `Fatto` | Sostituiti da ciò che la SPEC calcola: `v_fascicolo` (lacune), `deroga_fabbisogno` (deroga), `proposta_triage` (segnali sulle mail), `bozza` (sollecito). Nessuna colonna di stato scritta a mano. |
| `Cartiglio`, `Revisione_CAD` | Il cartiglio letto finisce in `documento_proposta.dettagli`; le revisioni in `documento.sostituito_da`. Da riaprire quando si progetta la fattibilità di Luigi. |
| schemi `int.*` (Materiale, Processo, Articolo, Ciclo, Ordine) e `mexal.*` | Fasi SCHEDA_COSTO → PRODUZIONE: fuori dallo scope «ricezione → fascicolo completo → FATTIBILITÀ». Il vincolo «nessun codice Mexal nei cataloghi interni» resta valido quando si aggiungeranno. |
| `Offerta`, `Ordine_Cliente`, `Riga_Ordine_Cliente`, `Articolo_Esterno` | Idem: dopo OFFERTA_INVIATA. |

Punto ancora aperto dalla SPEC §0: `scadenza_origine` è incluso come enum {mail, buyer, portale, stimata}; se la
distinzione non serve al cruscotto, si toglie prima della produzione.

## Prossimi passi (ordine consigliato)

1. **Blocco 5 completo**: conferma in blocco («tutte ≥ 80 %»), rinomina file, drop-zone per i file dal portale
   (`riferimento_portale` → documento), `dry_run` e radice NAS aziendale.
2. **Blocco 6 — fascicolo, sollecito, fasi**: transizioni `RICEVUTA → ATTESA_DISEGNI/FATTIBILITA` da `v_thread_bloccanti`,
   testo del sollecito → `bozza` tipo `sollecito`, deroghe, note di fattibilità per componente.
3. **Blocco 7 — consultazione**: viewer PDF dal NAS, ricerca per codice.
4. Rifiniture: corpo HTML sanificato (bluemonday), citazioni rimosse da `corpo_testo`, `bozza.inviata_messaggio_id`,
   `Items.Restrict` DASL per il sync storico su caselle grandi, backup notturno, fuso orario del PC (ora legale spenta).
