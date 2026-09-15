# Cockpit RFQ

Il Cockpit trasforma le richieste d'offerta che arrivano via mail in fascicoli ordinati: legge la
posta da Outlook classico, registra messaggi e allegati, propone a quale RFQ appartengono e, su
decisione dell'operatore, copia i file nella cartella del NAS. È fatto di tre pezzi:

```
Outlook classico ◀─COM─ worker_outlook.py ─HTTP─▶ cockpit.exe ◀─pgx─▶ PostgreSQL
                        worker_analisi.py ─HTTP─▶     │
                                   browser (HTML+HTMX) ◀┘   (cookie di sessione, DB)

                         ┌─────────────────────┐
                         │       Browser       │
                         │ Francesco / utenti  │
                         └─────────┬───────────┘
                                   │ HTTP
                                   ▼
                         ┌─────────────────────┐
                         │    internal/web     │
                         │ UI + decisioni RFQ  │
                         └─────────┬───────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
             internal/jobs                 internal/db
             crea "ordini"                 legge/scrive
             di lavoro                     PostgreSQL
                    │                             ▲
                    ▼                             │
             ┌──────────────┐                     │
             │ tabella job  │─────────────────────┘
             └──────┬───────┘
                    │
                    ▼
             internal/workerapi
             API per i worker
                    │
             ┌──────┴────────────┐
             ▼                   ▼
       worker_outlook.py   worker_analisi.py
             │                   │
             ▼                   ▼
         Outlook COM        PDF / STEP / file
             │                   │
             └───── risultato ───┘
                    │
                    ▼
             internal/workerapi
                    │
                    ├── internal/ingest
                    ├── internal/archivio
                    ├── internal/jobs
                    └── internal/db
```

`cockpit.exe` (Go) è l'unico che parla con il database e con il NAS. I worker Python non hanno
credenziali del database: chiedono lavoro al server, lo eseguono e riportano il risultato.

Che cosa introduce ogni fase di lavoro e che cosa resta non verificato:
[FASE_0.md](_fasi/FASE_0.md) (fondazioni, migrazioni, ambiente di prova),
[FASE_1.md](_fasi/FASE_1.md) (coda, tentativo, ingest a prova di poison pill) e
[FASE_2.md](_fasi/FASE_2.md) (più caselle, presenza, cursore per casella — in corso).

**Regola cardine:** nessun file viene scaricato automaticamente. Il sync registra gli allegati come
fatto e una proposta dal solo nome; sul disco vanno solo i file che l'operatore spunta dentro una RFQ.

---

## Prerequisiti

| Serve | Versione | Come si verifica | Note |
|---|---|---|---|
| Go | 1.26 o successivo (toolchain 1.27) | `go version` | solo per compilare; in produzione basta `cockpit.exe` |
| PostgreSQL | 16 o successivo | `psql --version` | server raggiungibile, un database e un ruolo per il Cockpit |
| Python | 3.11 o successivo | `python --version` | sui PC dove gira un worker |
| Outlook | classico (desktop), con profilo configurato | deve essere **aperto** | solo dove gira `worker_outlook.py` |
| sqlc | 1.31 o successivo | `sqlc version` | solo se si toccano `migrations/` o `internal/db/queries/` |

Le dipendenze Python sono tre: `pip install -r workers\requirements.txt` (`pywin32` per COM,
`pydantic` per i contratti, `pymupdf` per leggere i PDF).

---

## Installazione da zero

### 1. Database

```powershell
psql -U postgres -c "CREATE ROLE cockpit LOGIN PASSWORD 'scegli-una-password';"
psql -U postgres -c "CREATE DATABASE cockpit_dev OWNER cockpit;"
```

Lo schema non va creato a mano: lo applica il server al primo avvio, migrazione per migrazione.

### 2. Configurazione del server: `cockpit.toml`

```powershell
copy cockpit.toml.example cockpit.toml
```

`cockpit.toml` sta accanto a `cockpit.exe` (oppure lo si indica con `-config`) e **non va nel
repository**: è già in `.gitignore`, perché contiene la password del database e i token dei worker.

Due regole di scrittura, prima del contenuto:

- i percorsi Windows vanno fra **apici singoli** (`'C:\cartella'`): in TOML sono stringhe letterali e i
  backslash non vanno raddoppiati. Fra virgolette doppie, `"C:\nas"` diventerebbe un'altra cosa;
- il file si legge tutto all'avvio e **un errore qui impedisce l'avvio**, di proposito: meglio non
  partire che partire con un routing sbagliato e accorgersene fra una settimana.

#### Le sezioni, una per una

**`[db].dsn`** — il database creato al passo 1. È obbligatorio: senza, il server esce subito con
`config: [db].dsn mancante`.

```toml
[db]
dsn = "postgres://cockpit:la-password@localhost:5432/cockpit_dev"
```

**`[server]`**

| Campo | Che cosa mettere |
|---|---|
| `indirizzo` | `127.0.0.1:8080` in sviluppo. `0.0.0.0:8080` espone il Cockpit in LAN: finché non arrivano TLS e credenziali individuali (voci 2.4 e 2.5) è una cosa da fare solo per le prove |
| `token_worker` | un segreto qualsiasi, lungo. È lo stesso che va in `workers\worker.toml`: se i due non coincidono il worker logga `401` e non prende lavoro |
| `log_livello` | `info`; `debug` stampa anche ogni claim |
| `max_upload_mb` | limite di un singolo allegato caricato dal worker (`PUT /api/v1/allegati/{id}/file`). Default 64. Oltre, il server risponde `413` prima di ricevere il file e l'allegato compare in errore con il motivo |

**`[nas]`**

| Campo | Che cosa mettere |
|---|---|
| `radice` | **è** la cartella «PREVENTIVI DA FARE», non la cartella che la contiene: sotto nascono `<cliente.cartella_nas>\WIP\<aaaa mm gg Cognome Oggetto>`. In sviluppo una cartella locale, in produzione il percorso UNC |
| `dry_run` | `true` calcola i percorsi e li scrive nel log senza toccare il disco: è il modo di provare la copia sul NAS aziendale senza scriverci |
| `staging` | cartella locale **del server** dove atterrano gli allegati che i worker caricano. Se manca, il server ne crea una accanto al file di configurazione. Dalla voce 2.3 non deve più coincidere con niente: il worker manda il file con `PUT`, non lo scrive qui |

**`[outlook]`**

| Campo | Che cosa mettere |
|---|---|
| `cartelle` | i nomi **come si vedono in Outlook**, nella lingua del profilo: su un Outlook italiano `["Posta in arrivo", "Posta inviata"]`, non `["Inbox", "Sent Items"]`. Una cartella scritta male non è un errore di avvio: è un sync che non legge niente da lì, in silenzio |
| `intervallo_sync_s` | ogni quanto accodare un sync. 60 va bene: i sync non si accumulano, ne resta al più uno pendente per casella |
| `dal` | `"2026-08-01"`: da quando leggere **al primo avvio**, quando il cursore è vuoto. Dopo non conta più, comanda il cursore |
| `lotto` | quanti messaggi per invio. 50 è il compromesso fra una transazione corta e troppe chiamate |
| `consenti_invio` | lasciare `false`. `true` permetterebbe al worker di premere Invia al posto dell'operatore |
| `casella_default` | la casella attribuita a un lotto che non dichiara la propria. Dalla fase 2 il lotto porta sempre il `casella_id` del job: questa è il ripiego per un worker più vecchio del server, non il modo normale |

**`[retention].giorni_job`** — per quanti giorni si tengono i job già chiusi. `0` = non cancellare
niente; una coda che non si svuota mai diventa illeggibile.

**`[analisi]`** — `versione` e `[analisi.parametri]` dicono **con che cosa** si analizza. Il loro hash,
insieme a quello del file, è la chiave sotto cui i fatti vengono conservati: lo stesso disegno in tre
RFQ fa partire una sola analisi. Cambiare un termine qui fa rianalizzare tutto senza toccare il
codice — ed è il motivo per cui i termini stanno qui e non dentro il worker.

**`[[utenti]]`** — almeno uno, con `ruolo = "admin"`. `password` serve solo al primo seed: nel
database va l'hash bcrypt, e chi ha già una password non se la vede sovrascritta.

```toml
[[utenti]]
sigla = "NC"          # è il nome utente del login
nome = "Nome Cognome"
ufficio = "Commerciale"
ruolo = "admin"       # admin | operatore | tecnico | consultazione
password = "password-iniziale"
```

**`[[casella]]`, `[[postazione]]`, `[[worker]]`** — le *fondazioni*: quali caselle il Cockpit conosce,
su quali PC girano i worker e con quale token ciascuno.

```toml
# Una casella condivisa Exchange NON ha proprietario: `utente` va lasciato fuori.
[[casella]]
indirizzo = "commerciale@azienda.it"
nome      = "Commerciale"
condivisa = true

# Una personale ha come proprietario la sigla di un utente dichiarato in [[utenti]].
[[casella]]
indirizzo = "nome.cognome@azienda.it"
nome      = "Nome Cognome"
utente    = "NC"
# attiva  = false   # la tiene censita ma fuori uso, senza cancellarla

[[postazione]]
nome_host   = "PC-NOME"    # il nome del PC, come lo stampa `hostname`
descrizione = "portatile commerciale"
utente      = "NC"

# Un worker per postazione e per tipo. Il token sta solo qui: in database va il suo sha256.
[[worker]]
nome       = "outlook@PC-NOME"     # <tipo>@<NOME_HOST>
tipo       = "outlook"             # outlook | analisi
token      = "segreto-di-questo-worker"
postazione = "PC-NOME"
caselle    = ["nome.cognome@azienda.it", "commerciale@azienda.it"]
```

Che cosa il server verifica all'avvio, e perché rifiuta di partire invece di arrangiarsi:

| Controllo | Perché |
|---|---|
| indirizzi normalizzati in minuscolo, nomi host in maiuscolo | la stessa casella scritta in due modi resterebbe due caselle, con due cursori e due volte lo stesso messaggio |
| una casella `condivisa = true` non può avere `utente` | una cassetta condivisa non ha un proprietario: dargliene uno falserebbe le autorizzazioni |
| `utente`, `postazione` e `caselle` devono esistere altrove nel file | un worker autorizzato su una casella mai dichiarata è autorizzato su niente, e lo si scoprirebbe solo quando un job non parte |
| la stessa casella dichiarata due volte | idem: due righe, due identità |

**Più di una casella attiva** si può, dalla migrazione `0004` (fase 2, voce 2.1): ognuna ha il proprio
cursore. Con lo schema fermo alla `0003` il server rifiutava l'avvio dicendo quale disattivare, perché
due caselle si sarebbero sovrascritte il cursore a vicenda **perdendo messaggi in silenzio**.

Il seed di queste sezioni è **non distruttivo**: togliere una riga dal file non disattiva nulla nel
database, lo scrive soltanto nel log. Per spegnere una casella si usa `attiva = false`, che invece
viene applicato.

#### Provare il file senza mettersi in ascolto

```powershell
.\cockpit.exe -config cockpit.toml -migra
```

Legge la configurazione, applica le migrazioni, semina utenti e fondazioni e **esce**. È il modo di
verificare che il file sia giusto — ed è anche il modo giusto di aggiornare il database prima di
sostituire il binario su una postazione.

### 3. Configurazione dei worker

```powershell
copy workers\worker.toml.example workers\worker.toml
```

`token` deve coincidere con `[server].token_worker`. `staging` è una cartella locale **del worker**: ci
passano i file temporanei (l'allegato salvato da Outlook, il tempo di caricarlo al server) e ci resta
il log; può stare su un PC diverso dal server e non deve coincidere con `[nas].staging`. `worker_id`,
se presente, deve coincidere con `[[worker]].nome` di `cockpit.toml`.

### 4. Compilazione

```powershell
go build -o cockpit.exe .\cmd\cockpit
```

---

## Avviare il server Go

```powershell
.\cockpit.exe -config cockpit.toml
```

All'avvio il server, in quest'ordine: legge la configurazione; applica le migrazioni mancanti (una
transazione per file, in ordine, saltando quelle già registrate in `schema_versione`); semina utenti,
caselle, postazioni e credenziali dei worker; avvia lo scheduler e l'esecutore dei job; si mette in
ascolto su `[server].indirizzo` (`http://127.0.0.1:8080` in sviluppo).

Il browser si apre su quell'indirizzo: login con la sigla e la password di `[[utenti]]`.

Opzioni:

```powershell
.\cockpit.exe -config cockpit.toml -migra   # applica migrazioni e seed, poi esce (nessun ascolto HTTP)
.\cockpit.exe -h                            # elenco delle opzioni
```

`-migra` è il modo giusto di aggiornare il database prima di sostituire il binario su una postazione.

## Avviare i worker Python

Ogni worker è un processo a sé e si può fermare e riavviare in qualsiasi momento: chiede lavoro al
server, non riceve comandi. Se il server è fermo aspetta e riprova, senza terminare.

```powershell
cd workers
python worker_outlook.py                # legge la posta e agisce su Outlook
python worker_analisi.py                # legge PDF e STEP degli allegati scaricati
```

Opzioni comuni a entrambi:

| Opzione | Che cosa fa |
|---|---|
| `--config <percorso>` | usa un `worker.toml` diverso da quello accanto allo script |
| `--una-volta` | esegue al più un job ed esce (utile per provare) |
| `--debug` | log più fitto sulla console |
| `--cartelle` | *(solo Outlook)* stampa l'albero delle cartelle Outlook ed esce |

Variabili d'ambiente che vincono sul file: `COCKPIT_URL`, `COCKPIT_TOKEN`, `COCKPIT_STAGING`,
`COCKPIT_WORKER_ID`.

**Il worker Outlook richiede che Outlook classico sia aperto**, con il profilo giusto, nella sessione
dello stesso utente. Se Outlook è chiuso i job falliscono con `COM:` e il server li rimette in coda:
basta aprirlo, senza riavviare nulla.

Il log di entrambi finisce sulla console e in `<staging>\log\worker_*.log` (5 file da 5 MB a
rotazione), così resta leggibile anche dopo aver chiuso il terminale.

### Tutto insieme, in sviluppo

```powershell
powershell -ExecutionPolicy Bypass -File scripts\avvia-dev.ps1               # server + worker analisi
powershell -ExecutionPolicy Bypass -File scripts\avvia-dev.ps1 -ConOutlook   # anche il worker Outlook
powershell -ExecutionPolicy Bypass -File scripts\ferma-dev.ps1               # ferma tutto
```

Il worker Outlook non parte da solo: si attacca via COM alla casella vera del profilo di questo
PC, quindi avviarlo è un accesso alla posta reale e serve chiederlo esplicitamente con
`-ConOutlook`.

### In produzione, all'accensione del PC

```powershell
powershell -ExecutionPolicy Bypass -File scripts\installa-attivita.ps1            # mostra cosa farebbe
powershell -ExecutionPolicy Bypass -File scripts\installa-attivita.ps1 -Installa  # crea le attività pianificate
powershell -ExecutionPolicy Bypass -File scripts\installa-attivita.ps1 -Mostra    # stato e ultimo esito
```

Le attività girano nella sessione interattiva dell'utente, perché Outlook classico lo richiede, e
riavviano il worker se termina. `-Installa` mette in avvio automatico **anche** il worker Outlook:
va fatto solo su una postazione dove leggere quella casella è già stato autorizzato.

---

## Come funziona, in breve

Il server accoda `sync_outlook` ogni `[outlook].intervallo_sync_s`; il worker legge le cartelle
indicate in `[outlook].cartelle` a partire dal cursore (la prima volta da `[outlook].dal`) e manda i
messaggi a lotti. Dall'Inbox del browser l'operatore decide: **Nuova RFQ**, **Aggancia a…**,
**Ignora**. Dentro una RFQ spunta gli allegati che servono: vengono scaricati in staging, analizzati e
proposti; con **Conferma → NAS** diventano documenti copiati nella cartella della RFQ con verifica
dell'hash. Restano manuali **Apri in Outlook**, **Segna letto** e **Rispondi**, che prepara una bozza:
l'invio non è mai automatico.

---

## Prove

```powershell
go test ./...                        # unitari: dominio, zip, NAS, template, migrazioni, configurazione
python -m pytest -q workers          # unitari Python: modulo comune, ciclo dei worker, analisi
```

I test che hanno bisogno di PostgreSQL vengono **saltati** se manca `COCKPIT_TEST_DSN`. Un test
saltato non è un test passato: per eseguirli serve il database di prova.

```powershell
powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Installa   # scarica PostgreSQL portatile (una volta)
powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Avvia      # cluster isolato sulla porta 5433
$env:COCKPIT_TEST_DSN = "postgres://cockpit_test:cockpit_test@127.0.0.1:5433/cockpit_test"
go test -tags integrazione -count=1 -p 1 ./...
```

`-p 1` non è un dettaglio: i pacchetti condividono un solo database e alcuni test ricreano lo schema,
quindi devono girare in serie. Senza, si distruggono lo schema a vicenda e gli errori che ne escono
non hanno niente a che vedere con il codice in prova.

È un cluster tutto suo, su una porta diversa, sotto `%LOCALAPPDATA%`: i test distruggono e ricreano lo
schema a ogni esecuzione e non possono toccare il database di sviluppo. Per sicurezza il codice di
test rifiuta un DSN il cui nome di database non contiene «test».

Tutto in una volta, con il registro degli esiti:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1
```

Scrive `docs\esiti\esiti_simulati.md` con il commit completo, le versioni degli strumenti e
l'esito di ogni prova, distinguendo PASSATO, FALLITO, SALTATO e NON ESEGUITO. Le prove che
richiedono Outlook, Exchange o due postazioni vere si annotano a mano in `docs\esiti\esiti_reali.md` e
non si deducono mai da una prova simulata.

Due avvertenze sulla lettura degli esiti:

- `go build` dimostra che il codice compila, **non** che i tipi Go e i modelli pydantic rispettino gli
  schemi di `contracts/`: quello è il livello L3, e ha un test suo in due metà (`internal/api` per il
  Go, `workers/test_contratti.py` per la premessa che gli schemi su disco siano quelli dei modelli di
  oggi). Confronta i campi e i loro generi, non l'obbligatorietà;
- un test **saltato** non è un test superato: è una verifica che non è stata fatta.

## Manutenzione

```powershell
sqlc generate                                       # dopo aver toccato migrations/ o internal/db/queries/
python workers\genera_contratti.py                  # rigenera contracts/*.schema.json dai modelli pydantic
powershell -File scripts\backup-db.ps1 -Dsn "..."   # backup + prova di ripristino vera
powershell -File scripts\db-test.ps1 -Ricrea        # svuota il database di prova
```

### Aggiungere una migrazione

1. creare `migrations/NNNN_nome.sql` con il numero successivo, senza buchi;
2. l'ultima riga del file deve essere `INSERT INTO schema_versione (versione) VALUES (NNNN);` — senza,
   la migrazione viene annullata e il server non parte;
3. non usare, nello stesso file, un valore di enum aggiunto con `ALTER TYPE … ADD VALUE`: PostgreSQL
   non lo accetta prima del commit, va usato dal file successivo;
4. non referenziare tabelle create in un file successivo;
5. `sqlc generate`, poi `go test ./internal/migrazioni/`, che controlla i punti 2, 3 e 4 senza database.

Un file già applicato non va più modificato: una migrazione registrata non viene riapplicata.

### Se qualcosa non va

| Sintomo | Causa probabile | Rimedio |
|---|---|---|
| `config: [db].dsn mancante` | manca `cockpit.toml` accanto all'eseguibile | copiarlo dall'esempio o passare `-config` |
| `migrazioni: il DB è alla versione N…` | database aggiornato da un binario più recente | aggiornare `cockpit.exe` |
| il worker logga `COM:` in continuazione | Outlook chiuso o su un altro utente | aprire Outlook nella stessa sessione |
| il worker logga `401` | `token` diverso da `[server].token_worker` | allineare i due file |
| la testata del browser dice OFFLINE | nessun claim da oltre un minuto | il worker è fermo: vedere il suo log |
| i job restano `pronto` | nessun worker di quel tipo è in esecuzione | avviare il worker corrispondente |

---

## Struttura

```
cmd/cockpit/main.go        avvio: config, pool, migrazioni, seed utenti e fondazioni, scheduler, esecutore server, router
embed.go                   embed.FS di migrations/, web/templates, web/static
internal/config            cockpit.toml: lettura, normalizzazione e verifica di caselle, postazioni, worker
internal/api               contratti JSON worker ↔ server (tipi Go; speculari a workers/contratti.py)
internal/db                sqlc: queries/*.sql → codice generato (non modificare a mano)
internal/migrazioni        applica migrations/*.sql in ordine, una transazione per file; verifica statica
internal/fondazioni        seed non distruttivo di caselle, postazioni e credenziali dei worker da cockpit.toml
internal/testutil          pool e schema pulito per i test d'integrazione (COCKPIT_TEST_DSN)
internal/domain            regole pure + test: codici, proposta dal nome file, portale, scadenza, triage, nome/cognome, percorsi NAS
internal/ingest            FATTO (messaggio, allegato) + proposta economica + aggancio automatico + triage/portale
internal/archivio          estrazione zip in staging (zip-slip, limiti) → allegati figli
internal/jobs              coda: accoda idempotente (un solo job PENDENTE per chiave), claim/lease, scheduler, esecutore 'server' (NAS), stage/analisi
internal/nas               scrittore NAS: .parte + verifica hash, mai sovrascrive, long-path
internal/workerapi         /api/v1/jobs/{claim,heartbeat,result}, /api/v1/ingest/messaggi, PUT /api/v1/allegati/{id}/file (token X-Cockpit-Token);
                           il file caricato resta .parte.<lease_token> finché il result valido non lo promuove; dopo-staging (zip, rumore, analisi)
internal/web               HTML+HTMX: login, /inbox, /messaggio/{id} (+triage, scarica), /thread/{id}, /proposta/{id}/{conferma,scarta}, /cruscotto, /admin/job
web/templates, web/static  template html/template, style.css, htmx 2.0.4
migrations/                0001_schema.sql (30 tabelle, 5 viste, 31 enum), 0002_fondazioni.sql (caselle, postazioni, worker),
                           0003_coda_ingest.sql (tentativo con lease_token, ingest_scarto, analisi_fatti),
                           0004_caselle_presenza.sql (messaggio_casella, cursore per casella, messaggio.interno, v_inbox)
contracts/*.schema.json    JSON Schema generati da workers/contratti.py
workers/                   cockpit_client.py (client, config, log, battito), worker_outlook.py, worker_analisi.py,
                           outlook_com.py (COM), contratti.py (pydantic), server_finto.py (prove senza server), worker.toml
scripts/                   avvia-dev.ps1, ferma-dev.ps1, db-test.ps1 (DB di prova isolato), prova-tutto.ps1,
                           backup-db.ps1 (con prova di ripristino), installa-attivita.ps1, db-reset.sh
```

## Come si parlano i pezzi

```
Outlook classico ◀─COM─ worker_outlook.py ─HTTP 127.0.0.1:8080─▶ cockpit.exe ◀─pgx─▶ PostgreSQL
                        worker_analisi.py ─HTTP────────────────▶     │
                                   browser (HTML+HTMX) ◀────────────┘  (cookie di sessione, DB)
```

- **Identità del messaggio** = Internet Message-ID (`messaggio.chiave_esterna`). Dove quel messaggio SI TROVA è
  un'altra cosa: `messaggio_casella` ha una riga per ogni casella in cui è arrivato, con l'EntryID, la cartella e
  lo stato di lettura di quella copia (migrazione 0004). I job verso Outlook portano anche il Message-ID: se
  l'EntryID non vale più (elemento spostato) il worker lo ritrova per Message-ID e il server riallinea la presenza
  di quella casella. `messaggio_outlook` conserva solo ciò che è del messaggio: catena di conversazione,
  `in_reply_to`, `riferimenti`.
- **Caselle e postazioni** (dalla migrazione 0002): una casella è una sola riga anche quando più PC la
  aprono; `casella_store` registra come ogni postazione la vede nel proprio profilo Outlook, perché lo
  StoreID appartiene al profilo e non è un riferimento valido su un altro PC.
- **Coda job** in PostgreSQL: `FOR UPDATE SKIP LOCKED`, lease per tipo (120 s / 300 s), 5 tentativi con backoff
  (50 per le scritture sul NAS, che non falliscono perché sono sbagliate ma perché il NAS in quel momento non c'è),
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
| `sync_cursore` (+ `storico_fino_a`) | SPEC §3.2 passo 1: «chiede al server il cursore della casella»; il sync storico ricorda fin dove è arrivato. Dalla 0004 la chiave è **(casella, cartella)**: con la sola cartella due caselle si sovrascrivevano il cursore a vicenda |
| `worker_presenza` | ultimo claim per worker: la UI segnala «OFFLINE» invece di lasciar crescere la coda in silenzio |
| `bozza` | Le mail preparate dal Cockpit (risposte, solleciti) vanno tracciate: stato, EntryID, poi collegate alla mail inviata |
| `messaggio.corpo_html` | Per rendere il Cockpit un vero frontend di Outlook serve l'HTML (da sanificare prima del rendering) |
| `messaggio.parent_messaggio_id` | SPEC §3.2: i `.msg` annidati producono messaggi figli |
| `messaggio_casella` (0004) | La stessa mail in due caselle è un messaggio e due presenze: EntryID, cartella, stato di lettura, categorie e `ricevuto_il` sono di ogni copia. Con una riga sola la seconda casella sovrascriveva la prima |
| `messaggio.interno` (0004) | «Da noi» e «fra noi» sono cose diverse: una mail fra colleghi è in uscita, ma non è traffico con il cliente (D10) |
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
