# Cockpit RFQ — repository

Implementazione di `SPEC_Architettura_Cockpit_RFQ.md` (blocchi 0, 1 e 2, più il canale di ritorno verso Outlook).
Stato al 13/09/2026: server Go, schema PostgreSQL, worker Outlook (COM) funzionanti sul PC di sviluppo
contro la casella `pietro.spinozzi@studio.unibo.it` in **Outlook classico**.

## Avvio in sviluppo

Prerequisiti già presenti su questo PC: Go 1.27, PostgreSQL 18 (DB `cockpit_dev`, ruolo `cockpit`),
`sqlc` in `%USERPROFILE%\go\bin`, Python 3.11 con `pywin32` e `pydantic`, Outlook classico con profilo configurato.

```powershell
cd C:\promatec\cockpit
go build -o cockpit.exe .\cmd\cockpit      # compila (template, statici e migrazione sono embedded)
.\cockpit.exe -config cockpit.toml         # http://127.0.0.1:8080  — login: PS / cockpit

# in un secondo terminale, con Outlook classico aperto:
python .\workers\worker_outlook.py         # long-poll della coda; --una-volta esegue un job ed esce; --cartelle elenca le cartelle
```

Il server accoda `sync_outlook` ogni 60 s (`[outlook].intervallo_sync_s`); il worker legge `Inbox` e `Sent Items`
dal cursore (prima volta: da `[outlook].dal`), invia i messaggi a `POST /api/v1/ingest/messaggi`, poi esegue
gli `stage_allegato` che il server ha accodato. Dall'Inbox del browser: **Apri in Outlook**, **Segna letto**,
**Rispondi → bozza in Outlook** (l'invio resta manuale: `consenti_invio = false`).

Utilità:

```powershell
bash scripts/db-reset.sh                   # ricrea lo schema da zero (una sola migrazione fino alla produzione)
sqlc generate                              # rigenera internal/db dopo aver toccato migrations/ o internal/db/queries/
go test ./...                              # unitari (dominio, NAS)
$env:COCKPIT_TEST_DSN="postgres://cockpit:cockpit_dev@localhost:5432/cockpit_dev"; go test ./internal/ingest/   # integrazione su DB
python workers/genera_contratti.py         # rigenera contracts/*.schema.json dai modelli pydantic
```

## Struttura

```
cmd/cockpit/main.go        avvio: config, pool, migrazione, seed utenti, scheduler, esecutore server, router
embed.go                   embed.FS di migrations/, web/templates, web/static
internal/config            cockpit.toml
internal/api               contratti JSON worker ↔ server (tipi Go; speculari a workers/contratti.py)
internal/db                sqlc: queries/*.sql → codice generato (non modificare a mano)
internal/domain            regole pure + test: codici, portale, scadenza, triage, convenzione percorsi NAS
internal/ingest            FATTO (messaggio, allegato) + aggancio automatico + prime INTERPRETAZIONI (triage, portale)
internal/jobs              coda: accoda idempotente, claim/lease, scheduler, esecutore dei job 'server' (NAS)
internal/nas               scrittore NAS: .parte + verifica hash, mai sovrascrive, dry-run
internal/workerapi         /api/v1/jobs/{claim,heartbeat,result}, /api/v1/ingest/messaggi (token X-Cockpit-Token)
internal/web               HTML+HTMX: login/sessioni, /inbox, /messaggio/{id}, /cruscotto, /admin/job, /healthz
web/templates, web/static  template html/template, style.css, htmx 2.0.4
migrations/0001_schema.sql l'unica migrazione (29 tabelle, 5 viste, 31 enum)
contracts/*.schema.json    JSON Schema generati da workers/contratti.py
workers/                   worker_outlook.py (loop), outlook_com.py (COM), contratti.py (pydantic), worker.toml
```

## Come si parlano i pezzi

```
Outlook classico ◀─COM─ worker_outlook.py ─HTTP 127.0.0.1:8080─▶ cockpit.exe ◀─pgx─▶ PostgreSQL
                                                                    │
                                   browser (HTML+HTMX) ◀────────────┘  (cookie di sessione, DB)
```

- **Identità del messaggio** = Internet Message-ID (`messaggio.chiave_esterna`); `entry_id`/`store_id` stanno nel
  satellite `messaggio_outlook` e vengono riallineati a ogni sync (un elemento spostato di cartella non duplica).
- **Coda job** in PostgreSQL: `FOR UPDATE SKIP LOCKED`, lease per tipo (120 s / 300 s), 5 tentativi con backoff,
  `chiave_idempotenza` unica. Priorità 1 = azione dell'utente (apri, bozza), 4 = staging, 5 = sync, 6 = analisi.
- **Tre strati per i file**: `allegato` (FATTO, scritto solo dal worker) → `documento_proposta` (INTERPRETAZIONE;
  oggi da estensione/nome file/rumore lato server, domani raffinata dal worker-analisi finché la proposta è `aperta`)
  → `documento` (DECISIONE dell'operatore; solo questa accoda `copia_nas`). `v_fascicolo` calcola la completezza.

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
| `sync_cursore` | SPEC §3.2 passo 1: «chiede al server il cursore della casella» |
| `bozza` | Le mail preparate dal Cockpit (risposte, solleciti) vanno tracciate: stato, EntryID, poi collegate alla mail inviata |
| `messaggio.corpo_html` | Per rendere il Cockpit un vero frontend di Outlook serve l'HTML (da sanificare prima del rendering) |
| `messaggio.parent_messaggio_id` | SPEC §3.2: i `.msg` annidati producono messaggi figli |
| `messaggio_outlook.categorie/non_letto/flag_stato` | Stato di lettura e flag visibili in Inbox senza aprire Outlook |
| `thread_offerta.cartella_creata` | Esito del job `crea_cartella_thread` |
| `documento.stato_nas/errore_nas/scritto_il` | SPEC §4.2 e §5.3 (in_coda → scritto/errore) |
| `buyer.telefono` | Note WhatsApp/telefono (SPEC §6.2) |
| `v_thread_fase`, `v_thread_bloccanti` | Fase corrente con SLA; contatore bloccanti che abilita «Avvia fattibilità» |

**Lasciate fuori dalla fase 1 (da decidere, non per dimenticanza):**

| Del vecchio `docs/0001_schema.sql` | Motivo |
|---|---|
| `Prodotto` | La SPEC mette `componente` direttamente sotto il thread (`tipo='finito'`, `padre_id NULL` = radice) e i codici finiti in `identificativo_thread`. `fase_log` è quindi per thread. Se servirà una fase per singolo prodotto dentro la stessa RFQ, si aggiunge `fase_log.componente_id`. |
| `Lacuna`, `Segnale`, `Fatto` | Sostituiti da ciò che la SPEC calcola: `v_fascicolo` (lacune), `deroga_fabbisogno` (deroga), `proposta_triage` (segnali sulle mail), `bozza` (sollecito). Nessuna colonna di stato scritta a mano. |
| `Cartiglio`, `Revisione_CAD` | Blocco 3+ (worker-analisi): il cartiglio letto finisce in `documento_proposta.dettagli`; le revisioni in `documento.sostituito_da`. Da riaprire quando si progetta la fattibilità di Luigi. |
| schemi `int.*` (Materiale, Processo, Articolo, Ciclo, Ordine) e `mexal.*` | Fasi SCHEDA_COSTO → PRODUZIONE: fuori dallo scope «ricezione → fascicolo completo → FATTIBILITÀ». Il vincolo «nessun codice Mexal nei cataloghi interni» resta valido quando si aggiungeranno. |
| `Offerta`, `Ordine_Cliente`, `Riga_Ordine_Cliente`, `Articolo_Esterno` | Idem: dopo OFFERTA_INVIATA. |

Punto ancora aperto dalla SPEC §0: `scadenza_origine` è incluso come enum {mail, buyer, portale, stimata}; se la
distinzione non serve al cruscotto, si toglie prima della produzione.

## Prossimi passi (ordine consigliato)

1. **Blocco 4 — triage in UI**: `POST /inbox/{id}/triage` (Nuova RFQ / Aggancia / Ignora), creazione buyer inline,
   chip degli identificativi proposti, `crea_cartella_thread`. Le query (`InsertThread`, `UpsertIdentificativo`,
   `AgganciaMessaggio`, `CercaThreadAperti`, `ApriFase`) sono già generate.
2. **Blocco 3 — worker-analisi.py**: consuma `analizza_allegato` (già accodati: zip → allegati figli, STEP `PRODUCT`,
   cartiglio PDF con `archivia_rfq`) e aggiorna `documento_proposta` via un endpoint `POST /api/v1/ingest/proposte`.
3. **Blocco 5 — smistamento**: schermata C su `ListProposteAperteThread`, conferma → `documento` + `copia_nas`
   (esecutore server già pronto, oggi in dry-run su `C:\promatec\_nas_test`).
4. Corpo HTML delle mail nel pannello (sanificazione con `bluemonday`), citazioni rimosse da `corpo_testo`,
   collegamento `bozza.inviata_messaggio_id` quando il sync vede la mail in Posta inviata.
