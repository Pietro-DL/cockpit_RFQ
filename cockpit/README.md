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
  
powershell -ExecutionPolicy Bypass -File scripts\azzera-dati.ps1 -Conferma 2>&1 | Select-Object -Last 15
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
| `indirizzo` | `127.0.0.1:8080` in sviluppo. Per esporre il Cockpit fuori da questo PC serve TLS: **senza `tls_cert` il server si rifiuta di partire** su un indirizzo non di loopback (voce 2.4) |
| `tls_cert`, `tls_key` | i due file PEM del listener. Se **non esistono**, il server ne genera uno autofirmato al primo avvio, li scrive e mette l'impronta sha256 nel log e nella pagina *Postazioni*: è quella che i worker verificano. Percorsi relativi a `cockpit.toml` |
| `tls_nomi` | i nomi e gli IP per cui vale il certificato generato. Assente = nome host della macchina e l'indirizzo di ascolto, se è un IP |
| `consenti_lan_in_chiaro` | la via d'uscita dichiarata: ascoltare in chiaro fuori da questo PC. Ha senso solo se il collegamento è già cifrato da altro (un tunnel). Il server lo ripete a ogni avvio |
| `token_worker` | **non autentica più niente** (voce 2.4): ogni worker ha il suo token in `[[worker]]`. Se la riga è ancora nel file il server lo dice all'avvio, e va tolta |
| `modalita` | `shadow` o `produzione` (voce 9.5). In shadow il Cockpit legge Outlook e il NAS ma non li modifica: bozze, «segna letto», spostamenti e copie sul NAS non si accodano e non si eseguono, nemmeno se erano già in coda; «Apri in Outlook» sì, e `dry_run` è forzato a `true`. **Assente = shadow**: il default sicuro è quello che non tocca niente. Al ritorno in produzione i job rimasti in coda durante la shadow vengono annullati |
| `log_livello` | `info`; `debug` stampa anche ogni claim |
| `log_file` | dove il server scrive il proprio log, oltre che nella finestra da cui è stato avviato (5 file da 5 MB a rotazione). Assente = `<nas.staging>\log\cockpit.log`, accanto a quelli dei worker; `"-"` = solo a schermo |
| `max_upload_mb` | limite di un singolo allegato caricato dal worker (`PUT /api/v1/allegati/{id}/file`). Default 64. Oltre, il server risponde `413` prima di ricevere il file e l'allegato compare in errore con il motivo |

**`[nas]`**

| Campo | Che cosa mettere |
|---|---|
| `radice` | **è** la cartella «PREVENTIVI DA FARE», non la cartella che la contiene: sotto nascono `<cliente.cartella_nas>\WIP\<aaaa mm gg Cognome Oggetto>`. In sviluppo una cartella locale, in produzione il percorso UNC |
| `dry_run` | `true` calcola i percorsi e li scrive nel log senza toccare il disco: è il modo di provare la copia sul NAS aziendale senza scriverci |
| `radici_produzione` | elenco dei percorsi UNC delle radici **vere**. In shadow il server si rifiuta di partire se `radice` è una di queste o una loro sottocartella: una prova in shadow sul NAS di produzione non è una prova in shadow |
| `staging` | cartella locale **del server** dove atterrano gli allegati che i worker caricano. Se manca, il server ne crea una accanto al file di configurazione. Dalla voce 2.3 non deve più coincidere con niente: il worker manda il file con `PUT`, non lo scrive qui |

**`[outlook]`**

| Campo | Che cosa mettere |
|---|---|
| `cartelle` | i nomi **come si vedono in Outlook**, nella lingua del profilo: su un Outlook italiano `["Posta in arrivo", "Posta inviata"]`, non `["Inbox", "Sent Items"]`. Una cartella scritta male non è un errore di avvio: è un sync che non legge niente da lì, in silenzio |
| `intervallo_sync_s` | ogni quanto accodare un sync per casella. `0` = mai (restano «Aggiorna ora» e «Carica precedenti»). Con il `Restrict` della voce 2.9 un sync ordinario costa uno o due secondi, quindi **30** è sostenibile su quattro caselle; i sync non si accumulano, ne resta al più uno pendente per casella |
| `giorni_sync_iniziale` | da quanti giorni indietro parte una **(casella, cartella) senza cursore**. Default **7**. Vale una volta sola: appena il primo sync scrive un cursore comanda il cursore, e un riavvio non riporta indietro la casella. Sette e non trenta perché il primo caricamento è l'unico in cui il worker scarica davvero tutto (corpo e allegati), e finché è occupato non apre elementi e non scarica allegati per chi sta lavorando |
| `dal` | `"2026-08-01"`: **override esplicito** della finestra iniziale, per import controllati. Non tocca le cartelle che hanno già un cursore, e scritto male ferma l'avvio. Lasciato scritto, tiene ferma la finestra iniziale a quella data mentre i giorni passano: per l'archivio più vecchio si usa «Carica precedenti» |
| `lotto` | quanti messaggi per invio. 50 è il compromesso fra una transazione corta e troppe chiamate |
| `consenti_invio` | lasciare `false`. `true` permetterebbe al worker di premere Invia al posto dell'operatore |
| `casella_default` | la casella attribuita a un lotto che non dichiara la propria. Dalla fase 2 il lotto porta sempre il `casella_id` del job: questa è il ripiego per un worker più vecchio del server, non il modo normale |

**«Carica precedenti»** non si configura: scarica **due giorni per clic e per casella**, a partire da
dove era arrivato il clic precedente. Il worker Outlook è uno per PC ed è seriale: finché macina un
job storico non apre elementi in Outlook e non scarica allegati, quindi le finestre sono piccole e
fra l'una e l'altra torna a disposizione. Finché il job storico di una casella è in coda, premere
ancora non ne accoda un altro; e in coda i job storici stanno in fondo, dietro anche al sync
ordinario.

**`[retention].giorni_job`** — per quanti giorni si tengono i job già chiusi. `0` = non cancellare
niente; una coda che non si svuota mai diventa illeggibile.

**`[analisi]`** — `versione` e `[analisi.parametri]` dicono **con che cosa** si analizza. Il loro hash,
insieme a quello del file, è la chiave sotto cui i fatti vengono conservati: lo stesso disegno in tre
RFQ fa partire una sola analisi. Cambiare un termine qui fa rianalizzare tutto senza toccare il
codice — ed è il motivo per cui i termini stanno qui e non dentro il worker.

**`[[utenti]]`** — chi entra e che cosa può fare. Le autorizzazioni dipendono da **`ruolo`** e solo
da quello: `ufficio` è organigramma, la `sigla` è il nome utente del login.

| ruolo | che cosa apre |
|---|---|
| `operatore` | l'interfaccia di lavoro: Inbox, Cruscotto, messaggi e thread, triage, RFQ, allegati, «Aggiorna ora» |
| `admin` | tutto quello dell'operatore **più** le schermate tecniche: *Coda job*, *Scarti*, *Postazioni* (e quindi il pacchetto dei worker) |
| `tecnico` | oggi quanto l'operatore; esiste da adesso perché le azioni della fattibilità e dell'albero saranno sue |
| `consultazione` | sola lettura: nessun POST, in nessuna schermata |

**Almeno uno deve essere `admin`**, e il server **non parte** senza: il pacchetto dei worker lo genera
solo lui, e senza nessuno che possa aprire *Postazioni* non si aggiunge più un PC. Anche un ruolo
scritto male ferma l'avvio invece di diventare `operatore` in silenzio, e dice quale utente e quali
parole sono ammesse. (Un file **senza nessun** `[[utenti]]` parte: è un file a cui non sono ancora
stati aggiunti, e lo dice l'impossibilità di entrare.)

`password` serve solo a far **nascere** l'utente. Appena in database c'è un hash bcrypt valido, il
file non lo sostituisce più — nemmeno riavviando con una password diversa scritta qui, e il server
lo scrive nel log invece di lasciare qualcuno a chiedersi perché non entra.

```toml
[[utenti]]
sigla = "NC"          # è il nome utente del login
nome = "Nome Cognome"
ufficio = "Commerciale"
ruolo = "operatore"   # admin | operatore | tecnico | consultazione
password = "password-iniziale"

[[utenti]]
sigla = "AM"
nome = "Nome Cognome"
ufficio = "IT"
ruolo = "admin"
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

#### Seminare l'anagrafica dei clienti (blocco 3)

```powershell
.\cockpit.exe -config cockpit.toml -semina-anagrafica seme_anagrafica.json
```

Legge un file JSON di clienti, domini e buyer, lo convalida **per intero** e poi scrive, e **esce**.

Il file non sta nel repository: nomi dei clienti, domini, indirizzi dei buyer e forme dei loro codici
sono dati dell'azienda. Lo tiene chi amministra il Cockpit, accanto a `cockpit.toml`.

```json
{
  "clienti": [
    {
      "ragione_sociale": "ACME S.p.A.",
      "cartella_nas": "ACME",
      "lingua": "it",
      "peso": 12,
      "domini": ["acme.example"],
      "buyer": [{"cognome": "Rossi", "nome": "Mario", "email": "mario.rossi@acme.example", "tipo": "buyer"}],
      "regole": {
        "famiglie_codice": [
          {"regex": "\\bAC\\d{5}[A-Z]\\b", "descrizione": "codici ACME", "esempio": "AC12345B"}
        ],
        "lingua_risposta": "it",
        "richiede_cbd": true
      }
    }
  ]
}
```

Due comportamenti da conoscere prima di lanciarlo.

**Se una sola regola ha l'esempio sbagliato, non parte niente.** Non «quel cliente viene saltato»:
l'intero seme si ferma e dice quale cliente e quale regola. Una regex sbagliata non fallisce, non
riconosce — e senza questo controllo entrerebbe in database e ci resterebbe, in silenzio.

**Un cliente già presente viene saltato per intero, non aggiornato.** Il file è la fotografia di un
foglio; il database è dove qualcuno ha già corretto a mano quello che il foglio sbagliava. Rilanciarlo
è quindi sicuro: aggiunge solo ciò che manca. I **domini** si aggiungono anche ai clienti esistenti,
ma un dominio già assegnato a un altro cliente **non viene spostato**: il seme lo segnala e prosegue.

Da lì in avanti si lavora dalla schermata **Admin → Anagrafica**, che è anche l'unico posto in cui si
scrivono peso, portale e regole di riconoscimento.

### 3. Configurazione dei worker

```powershell
copy workers\worker.toml.example workers\worker.toml
```

Il modo consigliato non è compilarlo a mano: dalla pagina **Postazioni** del Cockpit si scarica il
pacchetto di quel PC (`cockpit-worker-<nome>.zip`), che contiene un `worker.toml` già pronto —
indirizzo del server, impronta del certificato e i token dei worker di quella postazione — le
istruzioni e i file del worker presi dal server. Vedi «[Aggiungere una postazione](#aggiungere-una-postazione)».

**Un token per worker (voce 2.4).** Il token non autorizza soltanto: *identifica*. Il server ne cerca
lo sha256 in `worker_credenziale` e da lì ricava nome, postazione e caselle autorizzate; il
`worker_id` dichiarato deve solo coincidere, e se non coincide è `403`. Due worker con lo **stesso**
token ricevono `401` con i nomi di tutti e due: un segreto che è di due non identifica nessuno. Sullo
stesso PC girano due worker, quindi i due token stanno nelle tabelle `[outlook]` e `[analisi]` di
`worker.toml`, che vincono sulle chiavi in cima al file.

**`impronta`** è lo sha256 del certificato del server: con `server_url` https il worker parla solo con
chi presenta quel certificato e si ferma con un errore esplicito se ne trova un altro. È ciò che
sostituisce la verifica di una CA, che in azienda non c'è. Toglierla per far ripartire un worker fermo
significa non sapere più con chi si sta parlando.

`staging` è una cartella locale **del worker**: ci
passano i file temporanei (l'allegato salvato da Outlook, il tempo di caricarlo al server) e ci resta
il log; può stare su un PC diverso dal server e non deve coincidere con `[nas].staging`. `worker_id`,
se presente, deve coincidere con `[[worker]].nome` di `cockpit.toml`: **è la chiave con cui il server
riconosce il worker**. Un nome che non compare in `[[worker]]` riceve `403` al claim e non prende
lavoro; un worker il cui nome host non è la postazione della sua credenziale riceve `403` anche lui
(è il caso di un `worker.toml` copiato su un altro PC).

Il worker Outlook non decide da solo che cosa leggere: all'avvio chiede al server le caselle su cui
la sua credenziale è autorizzata (`GET /api/v1/worker/caselle`), le cerca nel profilo Outlook del PC
e dichiara al claim solo quelle che ha trovato, con lo StoreID locale. Uno store presente nel profilo
ma non censito in `[[casella]]` **viene ignorato**: non viene letto e non viene censito d'ufficio.
`python worker_outlook.py --caselle` mostra questa risoluzione senza prendere nessun job.

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

Il log del server va sulla finestra **e** su `<nas.staging>\log\cockpit.log` (5 file da 5 MB a
rotazione, come i worker): chiusa la finestra, di ciò che il server ha risposto ai worker resta
comunque traccia, ed è metà della diagnosi di un job. Si cambia con `[server].log_file`.

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
| `--caselle` | *(solo Outlook)* chiede al server le caselle da servire, le risolve nel profilo Outlook e stampa l'esito (M1) senza prendere job |
| `--restrict [GIORNI]` | *(solo Outlook)* C2/C3: legge due volte gli ultimi GIORNI giorni di ogni cartella servita — una con il filtro `Restrict` in UTC e una scorrendo tutto — e confronta gli **insiemi** di EntryID, stampando anche i due tempi. Non prende job e non tocca niente. Default 7 giorni |

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

## Mettere il Cockpit in rete

Fino al blocco 1 il Cockpit girava tutto su un PC: server, worker e browser sullo stesso `127.0.0.1`,
un token condiviso, nessuna cifratura. Da qui in poi il server sta da una parte e i worker dall'altra,
e questo cambia tre cose: **come si cifra**, **come si riconosce un worker** e **come si aggiunge un
PC**.

### Perché il server non parte in chiaro fuori da questo PC

`[server].indirizzo` diverso da loopback e nessun `tls_cert` = il server si ferma con un errore. Non è
zelo: su quel collegamento passano la posta dell'azienda, il **token di ogni worker** e il **cookie di
sessione** dell'operatore. In chiaro su una LAN aziendale li legge chiunque sia attaccato allo stesso
switch, e non c'è niente — un log, un errore, un rallentamento — che lo segnali. È il tipo di difetto
che si scopre dopo.

Le due uscite sono dichiarate e si leggono nel file: `tls_cert` + `tls_key`, oppure
`consenti_lan_in_chiaro = true` se il collegamento è già cifrato da qualcos'altro.

### Il certificato: autofirmato, e i worker ne verificano l'impronta

Non c'è una CA interna, e aspettarne una significherebbe restare in chiaro. Quindi:

1. si scrivono `tls_cert` e `tls_key` in `[server]`, puntando a due file che **non esistono ancora**;
2. al primo avvio il server genera un certificato autofirmato, lo scrive e mette l'impronta nel log:

   ```
   level=WARN msg="certificato TLS generato ora (autofirmato)" cert=... nomi=[...] scade=16/09/2036
   level=INFO msg="TLS attivo" impronta=9f3c... scade=16/09/2036
   ```
3. quell'impronta finisce nel `worker.toml` di ogni postazione (la mette da sola la pagina
   *Postazioni*). Da quel momento **il worker parla solo con quel certificato**.

È il modello di SSH, ed è più forte di una catena che nessuno verifica: un impostore dovrebbe
presentare *quel* certificato, non un certificato qualunque firmato da qualcuno di cui il PC si fida.
Il browser invece dirà «connessione non privata» la prima volta: si accetta l'eccezione, oppure si
installa il certificato fra quelli attendibili del PC.

Se il certificato viene rifatto, i worker si fermano con un messaggio esplicito e vogliono un
pacchetto nuovo. È voluto: un worker che riparte da solo dopo un cambio di certificato è un worker che
non sta verificando niente.

### Aggiungere una postazione

Le **autorizzazioni** stanno in `cockpit.toml` — è un file versionato, leggibile, uguale a ogni avvio —
e i **segreti** li genera il server. Nell'ordine:

1. in `cockpit.toml`, un `[[postazione]]` con il nome host del PC e un `[[worker]]` per tipo, con
   `caselle` = quelle che quel PC può leggere e **`token = ""`**;
2. riavviare il server (le sezioni si applicano all'avvio);
3. da *Postazioni*, come amministratore: **«genera e scarica il pacchetto»**. Esce
   `cockpit-worker-<pc>.zip` con `worker.toml` (indirizzo, impronta, i token), `ISTRUZIONI.txt` e i
   file del worker presi dal server;
4. sul PC nuovo: scompattare, `python -m pip install -r requirements.txt`, poi
   `python worker_outlook.py --caselle` per vedere che risolva le sue caselle e **solo** quelle.

I token si vedono **una volta sola**: in database c'è il loro sha256. Rigenerare il pacchetto
invalida i precedenti — il worker di quel PC riceverà `401` finché non gli si copia il file nuovo — e
il pulsante lo chiede prima di farlo.

Un `token` scritto a mano in `[[worker]]` vince a ogni avvio: va bene sul banco di prova, dove il file
è la verità e non c'è niente da proteggere. Lasciarlo vuoto significa «il segreto lo tiene il
database», che è quello che serve in azienda.

**Se si arriva da un token condiviso** — un solo segreto copiato in tutti i `[[worker]]`, com'era
prima della voce 2.4 — svuotare i `token` in `cockpit.toml` è la mossa giusta ma **non basta**: il
seed non riscrive un token che il file non dichiara (è la regola che tiene in vita il pacchetto
scaricato ieri), quindi le impronte duplicate restano in database e i due worker continuano a
ricevere `401`. Serve il passo 3: **«Rigenera credenziali della postazione»**. La pagina *Postazioni*
segnala da sola i PC in quello stato — `token condiviso` e `credenziale da generare` — invece di
lasciarlo scoprire al primo `401`.

### Banco a due PC

La prova minima che il blocco 2 regge: server su un PC, worker su un altro.

| | |
|---|---|
| Server | `indirizzo = "0.0.0.0:8443"`, `tls_cert`/`tls_key` come sopra. Aprire la porta 8443 nel firewall di Windows **in ingresso** |
| Worker | il pacchetto scaricato da *Postazioni*, scompattato sul secondo PC |
| Da guardare | il worker deve fare claim (`/admin/postazioni` mostra «ultimo contatto» e l'IP), e «Apri in Outlook» deve aprire la finestra **su quel PC**, non sul primo |
| Da provare al contrario | cambiare una cifra dell'impronta nel `worker.toml`: il worker deve fermarsi con «il server ha presentato un certificato diverso da quello atteso» e **non** mandare il token |

### Server su una VM Linux, worker sui PC (D23)

Il posto del server è una VM Linux: PostgreSQL, il worker di analisi e la copia sul NAS via SMB stanno
lì, e non hanno bisogno di una sessione interattiva. I worker Outlook restano sui PC degli operatori,
perché COM non gira senza qualcuno collegato.

```bash
GOOS=linux GOARCH=amd64 go build -o cockpit ./cmd/cockpit
```

Il binario porta dentro migrazioni, template e i file dei worker: si copia un file solo.

- **NAS**: la share si monta sulla VM (`cifs`, credenziali in un file a `0600`) e `[nas].radice`
  diventa un percorso POSIX, per esempio `/mnt/nas/TECNICO - PREVENTIVI/PREVENTIVI DA FARE`. Il
  confronto con `radici_produzione` ignora maiuscole, barre e barra finale, quindi la protezione della
  shadow vale anche scritta alla maniera di Linux;
- **VPN fra gli impianti**: serve **una sola regola di firewall**, in ingresso sulla VM, sulla porta
  del Cockpit. I worker si collegano in uscita e il server non chiama nessuno: non c'è niente da
  aprire sui PC degli operatori;
- **la chiave privata** (`tls_key`) sta sulla VM con i permessi `0600` che il server le dà quando la
  genera. Su Windows quei permessi non si applicano e restano le ACL della cartella: è un motivo in
  più perché il posto del server sia la VM.

**Non ancora provato:** la copia sul NAS attraverso una share SMB montata su Linux (N1 su share) e il
banco a due PC vero. Il codice compila per Linux e i percorsi POSIX sono coperti dai test, ma né
l'uno né l'altro è stato eseguito: sono prove reali, e finché non si fanno restano `NON ESEGUITO` in
`docs\esiti\esiti_reali.md`.


---

## Come funziona, in breve

Il server accoda `sync_outlook` ogni `[outlook].intervallo_sync_s`; il worker legge le cartelle
indicate in `[outlook].cartelle` a partire dal cursore (la prima volta da `[outlook].dal`) e manda i
messaggi a lotti. Dall'Inbox del browser l'operatore decide: **Nuova RFQ**, **Aggancia a…**,
**Ignora**. Dentro una RFQ spunta gli allegati che servono: vengono scaricati in staging, analizzati e
proposti; con **Conferma → NAS** diventano documenti copiati nella cartella della RFQ con verifica
dell'hash. Restano manuali **Apri in Outlook**, **Segna letto** e **Rispondi**, che prepara una bozza:
l'invio non è mai automatico.

**L'anagrafica dei clienti.** *Admin → Anagrafica* e' dove un cliente prende un peso (0-15, che ordina
la lista **Richieste**), i suoi domini e le sue **regole di riconoscimento**: le famiglie dei suoi
codici, il formato del suo riferimento di richiesta, le frasi con cui dice «e' sul portale». Ogni
regola con una regex porta un **esempio che deve corrispondere**, perche' una regex sbagliata non da'
errore: smette di riconoscere, e in silenzio. Le regole con l'esempio sbagliato compaiono con una ✗ e
il motore non le usa; un JSON che non rispetta lo schema non viene salvato.

Nella stessa schermata c'e' un **banco di prova**: si incolla una mail e si vede che cosa il Cockpit
ne capirebbe. Non e' una simulazione — chiama la stessa funzione che lavora sui messaggi veri.

E' una schermata amministrativa: una regex cambiata li' cambia il riconoscimento della posta di tutti.
Un operatore non ne vede la voce e riceve 403 se ne scrive l'indirizzo.

**Su quale PC.** Un job va solo a un worker che serve la casella del job — cioè che l'ha trovata nel
proprio profilo Outlook ed è autorizzato a leggerla — e, se è un'azione interattiva (Apri, Segna
letto, Bozza), solo al worker della **postazione da cui l'operatore sta lavorando**. La sessione del
browser sa qual è: al login, se un worker attivo di una postazione autorizzata ha fatto claim dallo
stesso indirizzo IP, la prende da lì; altrimenti l'operatore la sceglie dalla testata («Sei su: …»).
Senza postazione le azioni su Outlook sono disabilitate con il motivo: una finestra aperta su un altro
PC non è un'azione riuscita. La testata dice anche, per ogni casella, se c'è un worker che la serve
(**attiva** su quale PC, **OFFLINE**, **non risolta** nel profilo, **non configurata**).

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

Fra i test d'integrazione ce n'è uno che non prova il server ma i due insieme: `TestE2E…` in
`internal/workerapi` avvia il **worker vero** (`python workers\prova_e2e.py`, cioè worker_outlook
con `--una-volta` e il solo adattatore COM sostituito) contro i gestori HTTP veri e PostgreSQL. È
l'unico livello in cui il client e il server si parlano davvero: gli altri provano una metà sola,
e un difetto che sta nel modo in cui il client compila il contratto — non nel contratto — passa
indisturbato attraverso tutti (è successo il 15/09/2026). Richiede Python; con
`prova-tutto.ps1 -SenzaPython` viene saltato e il registro lo annota come non verificato.

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
| il worker logga `401 credenziale non riconosciuta` | dalla voce 2.4 il token è individuale, e quello del worker non è in `worker_credenziale` | scaricare il pacchetto di quel PC dalla pagina *Postazioni*, oppure scrivere il token in `[[worker]].token` e riavviare il server |
| il worker logga `401 questo token è di più worker` | lo stesso segreto è di due `[[worker]]`: non identifica nessuno | dare a ciascuno il suo (o lasciare `token = ""` e generare il pacchetto dalla pagina *Postazioni*) |
| la pagina *Postazioni* dice `credenziale da generare` | quel `[[worker]]` ha `token = ""` e il pacchetto non è mai stato scaricato: la credenziale esiste, dice quali caselle serve, e non autentica nessuno | **«Rigenera credenziali della postazione»** su quel PC, e copiarci il pacchetto |
| la pagina *Postazioni* dice `token condiviso` | due `[[worker]]` hanno la stessa impronta in database — di solito si arriva dal token unico di prima. Svuotare i `token` nel file non la cancella | **«Rigenera credenziali della postazione»**: è l'unico passo che cambia davvero i segreti |
| il worker si ferma con `il server ha presentato un certificato diverso da quello atteso` | il certificato del server è stato rifatto, oppure dall'altra parte c'è qualcun altro | scaricare il pacchetto nuovo da *Postazioni*. **Non** togliere `impronta` da `worker.toml`: senza, non si sa più con chi si parla |
| il server non parte: «ascolta fuori da questo PC e tls_cert non c'è» | si sta esponendo il Cockpit in chiaro sulla LAN (voce 2.4) | indicare `tls_cert`/`tls_key` (se i file non esistono li genera lui), o dichiarare `consenti_lan_in_chiaro = true` se il collegamento è già cifrato |
| il browser dice «connessione non privata» | il certificato è autofirmato e il PC non lo conosce | accettare l'eccezione, o installare `cert.pem` fra i certificati attendibili. I worker non passano di qui: verificano l'impronta |
| il worker logga `403` | il suo nome non è in `[[worker]]`, o gira su un PC diverso dalla sua `postazione` | correggere `cockpit.toml` o `worker_id` in `worker.toml` |
| in testata mancano *Coda job*, *Scarti*, *Postazioni* | sono schermate dell'amministratore: quell'utente è `operatore` (voce 6.9) | entrare con un utente `admin`; il ruolo si cambia in `[[utenti]]` e vale dal riavvio successivo |
| una schermata `/admin/...` risponde **403 Non autorizzato** | stessa cosa scritta a mano nella barra degli indirizzi: nascondere la voce non era il controllo, il controllo è sulla rotta | come sopra. Se è sparito l'ultimo `admin`, rimetterne uno in `[[utenti]]` e riavviare |
| un utente non può premere nessun pulsante | ha `ruolo = "consultazione"`, che è sola lettura | cambiare ruolo in `[[utenti]]` |
| una password cambiata in `cockpit.toml` non ha effetto | è voluto: il file fa nascere l'utente, poi il segreto è in database (il log lo dice a ogni avvio) | finché non c'è la schermata del profilo (voce 6.4), azzerare a mano `utente.password_hash` e riavviare |
| una casella in testata è **OFFLINE** | il worker che la serve non fa claim da oltre un minuto | il worker di quel PC è fermo: vedere il suo log |
| una casella in testata è **non risolta** | il worker è attivo ma non trova la casella nel profilo Outlook del suo PC (o Outlook non risponde) | aggiungere la casella al profilo, o aprire Outlook; `python worker_outlook.py --caselle` dice che cosa vede |
| una casella in testata è **non configurata** | nessun `[[worker]]` la elenca fra le proprie `caselle` | aggiungerla al worker della postazione che deve servirla |
| «Apri in Outlook» dice *nessuna postazione* | la sessione non è abbinata a nessun PC | scegliere il PC dalla testata («Sei su:») |
| «Apri in Outlook» dice *non viene dirottata* | il worker della postazione scelta non serve nessuna casella in cui il messaggio è presente | autorizzare quella casella al worker, o lavorare dalla postazione che la serve |
| i job restano `pronto` | nessun worker che serva quella casella (o quella postazione) è in esecuzione | avviare il worker corrispondente; la testata dice quale |
| un job torna `in_corso` e si ripete, il worker logga `400 worker_id mancante` | il worker riporta il risultato senza dire quale tentativo sta chiudendo | difetto corretto il 15/09/2026: aggiornare i worker insieme al server |
| in testata c'è **SHADOW** e metà dei pulsanti dice «in attesa di produzione» | il server gira in sola lettura (voce 9.5) | è voluto finché si prova sulla posta vera; per riattivare le scritture: `[server].modalita = "produzione"` e riavvio |
| una casella resta **in corso…** per minuti | il sync di quella casella è in coda o in esecuzione | normale al primo giro su una casella grande; se non finisce mai, il worker è fermo: vedere il suo log e `/admin/job` |
| il sync dura minuti invece di secondi | `Restrict` è spento su quella cartella | il log del worker dice perché: `self-test Restrict FALLITO` (il filtro perdeva elementi) o `Restrict non disponibile`. `python worker_outlook.py --restrict 7` lo rimisura |
| lo stesso sync parte più volte con la stessa finestra | il `result` non viene accettato, il lease scade e lo scheduler riaccoda | leggere `cockpit.log`: il server scrive il motivo del rifiuto con il nome del worker |

---

## Struttura

```
cmd/cockpit/main.go        avvio: config, pool, migrazioni, seed utenti e fondazioni, scheduler, esecutore server, router
embed.go                   embed.FS di migrations/, web/templates, web/static e workers/ (il pacchetto della postazione)
internal/config            cockpit.toml: lettura, normalizzazione e verifica di caselle, postazioni, worker
internal/api               contratti JSON worker ↔ server (tipi Go; speculari a workers/contratti.py)
internal/db                sqlc: queries/*.sql → codice generato (non modificare a mano)
internal/migrazioni        applica migrations/*.sql in ordine, una transazione per file; verifica statica
internal/fondazioni        seed non distruttivo di caselle, postazioni e credenziali dei worker da cockpit.toml
internal/testutil          pool e schema pulito per i test d'integrazione (COCKPIT_TEST_DSN)
internal/domain            regole pure + test: codici, proposta dal nome file, portale, scadenza, triage, nome/cognome, percorsi NAS;
                           regole.go: lo schema di `cliente.regole` (famiglie di codice con esempio obbligatorio) e l'UNICO
                           ingresso al riconoscimento, `Riconosci`, che usano sia l'ingest sia il banco di prova dell'Anagrafica
internal/anagrafica        seme di clienti, domini e buyer da file: convalida tutto prima di scrivere, crea cio' che manca
                           e non tocca cio' che c'e' (voce 6.6)
internal/ingest            FATTO (messaggio, allegato) + proposta economica + aggancio automatico + triage/portale
internal/archivio          estrazione zip in staging (zip-slip, limiti) → allegati figli
internal/jobs              coda: accoda idempotente (un solo job PENDENTE per chiave), claim/lease, scheduler, esecutore 'server' (NAS), stage/analisi;
                           shadow.go: la modalita di sola lettura (che cosa non si accoda e non si esegue, e che cosa si annulla al ritorno in produzione)
internal/nas               scrittore NAS: .parte + verifica hash, mai sovrascrive, long-path
internal/rete              TLS del listener: carica o genera il certificato autofirmato e ne calcola l'impronta;
                           impronta e generazione dei token dei worker (voce 2.4)
internal/workerapi         /api/v1/jobs/{claim,heartbeat,result}, GET /api/v1/worker/caselle, /api/v1/ingest/messaggi, PUT /api/v1/allegati/{id}/file
                           (X-Cockpit-Token con il token INDIVIDUALE del worker: il server lo cerca per sha256 e da lì sa chi chiama);
                           il claim interseca le caselle dichiarate con la credenziale e registra presenza e casella_store;
                           il file caricato resta .parte.<lease_token> finché il result valido non lo promuove; dopo-staging (zip, rumore, analisi)
internal/web               HTML+HTMX: login (postazione per IP), /sessione/postazione, /inbox, /messaggio/{id} (+triage, scarica; apri/letto/bozza
                           instradati alla postazione della sessione), /thread/{id}, /proposta/{id}/{conferma,scarta}, /cruscotto, /admin/job (+annulla);
                           inbox_viva.go: «Aggiorna ora», stato del sync per casella in testata, «nuove dall'ultima visita» (voce 2.16);
                           postazioni_admin.go: /admin/postazioni, il pacchetto del worker con token e impronta (voce 2.4, D22)
web/templates, web/static  template html/template, style.css, htmx 2.0.4
migrations/                0001_schema.sql (30 tabelle, 5 viste, 31 enum), 0002_fondazioni.sql (caselle, postazioni, worker),
                           0003_coda_ingest.sql (tentativo con lease_token, ingest_scarto, analisi_fatti),
                           0004_caselle_presenza.sql (messaggio_casella, cursore per casella, messaggio.interno, v_inbox),
                           0005_postazioni_presenza.sql (worker_presenza per worker, sessione.postazione_id, via store_id_locale),
                           0006_inbox_viva.sql (utente.ultima_vista_inbox)
internal/logfile           il log del server su file, con rotazione (5 x 5 MB)
contracts/*.schema.json    JSON Schema generati da workers/contratti.py
workers/                   cockpit_client.py (client, config, log, battito), worker_outlook.py, worker_analisi.py,
                           outlook_com.py (COM), contratti.py (pydantic), server_finto.py (prove senza server),
                           prova_e2e.py (il worker vero senza COM, per il test end-to-end), worker.toml.
                           Questi file viaggiano anche dentro cockpit.exe: sono il pacchetto che la pagina Postazioni scarica
scripts/                   avvia-dev.ps1, ferma-dev.ps1, db-test.ps1 (DB di prova isolato), prova-tutto.ps1,
                           azzera-dati.ps1 (riga di partenza pulita), query-debug.sql (le query della diagnosi),
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
