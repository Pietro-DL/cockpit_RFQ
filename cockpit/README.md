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
                         │    transport/web    │
                         │ UI + decisioni RFQ  │
                         └─────────┬───────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                             ▼
             platform/coda                 platform/db
             crea "ordini"                 legge/scrive
             di lavoro                     PostgreSQL
                    │                             ▲
                    ▼                             │
             ┌──────────────┐                     │
             │ tabella job  │─────────────────────┘
             └──────┬───────┘
                    │
                    ▼
             transport/workerapi
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
             transport/workerapi
                    │
                    ├── core/inbox/ingest
                    ├── platform/storage/archivio
                    ├── platform/coda
                    └── platform/db
```

(i nomi nel disegno sono relativi a `internal/`)

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
| sqlc | 1.31 o successivo | `sqlc version` | solo se si toccano `migrations/` o `internal/platform/db/queries/` |

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
| `tls_nomi` | i nomi e gli IP per cui vale il certificato generato. Assente = nome host della macchina e l'indirizzo di ascolto, se è un IP; dal 7C.1 anche l'host di `url_pubblico` |
| `url_pubblico` | (7C.1) l'indirizzo con cui worker e browser **chiamano** il server, es. `https://10.0.0.7:8443`: finisce nel `worker.toml` del pacchetto e nelle istruzioni della postazione. Assente = derivato dal bind, cioè il nome host della macchina, che da un altro PC della LAN può non risolversi. Lo schema deve essere quello che il server parla davvero, altrimenti non parte |
| `consenti_lan_in_chiaro` | la via d'uscita dichiarata: ascoltare in chiaro fuori da questo PC. Ha senso solo se il collegamento è già cifrato da altro (un tunnel). Il server lo ripete a ogni avvio |
| `token_worker` | **non autentica più niente** (voce 2.4): ogni worker ha il suo token in `[[worker]]`. Se la riga è ancora nel file il server lo dice all'avvio, e va tolta |
| `modalita` | `shadow` o `produzione` (voce 9.5). **`shadow` è un preset di `[sicurezza]`**: spegne tutte e tre le capacità di scrittura, qualunque cosa dica quella sezione. «Apri in Outlook» resta consentito. **Assente = shadow**: il default sicuro è quello che non tocca niente. `produzione` NON accende niente da sola: serve `[sicurezza]`. Quando una capacità si accende, i job che avevano aspettato vengono annullati, non eseguiti |
| `log_livello` | `info`; `debug` stampa anche ogni claim |
| `log_file` | dove il server scrive il proprio log, oltre che nella finestra da cui è stato avviato (5 file da 5 MB a rotazione). Assente = `<nas.staging>\log\cockpit.log`, accanto a quelli dei worker; `"-"` = solo a schermo |
| `max_upload_mb` | limite di un singolo allegato caricato dal worker (`PUT /api/v1/allegati/{id}/file`). Default 64. Oltre, il server risponde `413` prima di ricevere il file e l'allegato compare in errore con il motivo |

**`[nas]`**

| Campo | Che cosa mettere |
|---|---|
| `radice` | **è** la cartella «PREVENTIVI DA FARE», non la cartella che la contiene: sotto nascono `<cliente.cartella_nas>\WIP\<aaaa mm gg Cognome Oggetto>`. In sviluppo una cartella locale, in produzione il percorso UNC |
| `dry_run` | **deprecata** (blocco 4): era esattamente «non scrivere sul NAS», che ora si dice con `[sicurezza].nas_scrittura = false`. Resta letta per i file già scritti — `true` spegne `nas_scrittura` — ma va tolta, e il server lo ripete nel log a ogni avvio |
| `radici_produzione` | elenco dei percorsi UNC delle radici **vere**. In shadow il server si rifiuta di partire se `radice` è una di queste o una loro sottocartella: una prova in shadow sul NAS di produzione non è una prova in shadow. In **produzione** non è vietata — è il posto dove il Cockpit lavorerà davvero — ma con `nas_scrittura = true` serve la seconda dichiarazione `[sicurezza].consenti_nas_produzione`. Il confronto ignora maiuscole, barre e barra finale |
| `intervallo_integrita_s` | ogni quanti secondi il ricognitore confronta i documenti con i file veri sul NAS (blocco 5B). Assente = 900. `0` = nessuna passata automatica, e «Controlla ora» in *Admin > Integrità NAS* continua a funzionare. Ogni documento costa la **lettura intera** del file per ricalcolarne l'hash: su una condivisione lenta conviene allungarlo |
| `staging` | cartella locale **del server** dove atterrano gli allegati che i worker caricano. Se manca, il server ne crea una accanto al file di configurazione. Dalla voce 2.3 non deve più coincidere con niente: il worker manda il file con `PUT`, non lo scrive qui |

**`[sicurezza]`** — che cosa questo server può **modificare fuori da sé** (blocco 4).

Prima c'era un interruttore solo, `[server].modalita`, e quindi una domanda sola: «tocchiamo il
mondo, sì o no?». Per provare la copia sul NAS di prova quella domanda si sdoppia — si vuole scrivere
un file in una cartella di prova, e **non** si vuole che una mail vera diventi letta, si sposti o
generi una bozza — e con un interruttore solo le due cose sono la stessa cosa.

| Campo | Che cosa governa |
|---|---|
| `outlook_scrittura` | `segna_letto`, `sposta_in_cartella`, e ogni altra modifica a Outlook **tranne** le bozze |
| `bozze` | `crea_bozza_outlook` |
| `nas_scrittura` | `crea_cartella_thread`, `copia_nas` |
| `consenti_nas_produzione` | la **seconda** dichiarazione, e serve solo quando le altre insieme varrebbero «scrivi nel fascicolo vero di un cliente»: `nas_scrittura = true` con `[nas].radice` dentro una `radici_produzione`. Senza, il server non parte e dice quale radice ha riconosciuto; con, parte e l'avvio lo annuncia nel log |

**Sempre consentiti**, e non chiedono nessuna capacità: sync di Outlook, lettura, download in
staging, estrazione degli archivi, analisi, «Apri in Outlook». Un tipo di job che non nomina una
capacità è per definizione una lettura, e una capacità che il server non riconosce vale **no**.

Tre regole, in quest'ordine: `modalita = "shadow"` è un **preset** e spegne tutte e tre qualunque
cosa dica questa sezione; **il silenzio vale «non scrivere»**, quindi un file in `produzione` senza
`[sicurezza]` non accende niente e il server lo scrive nel log con la riga da aggiungere;
`[nas].dry_run = true` spegne `nas_scrittura` per compatibilità, ed è deprecata.

Il blocco agisce in **due punti**: un job che chiede una capacità spenta non entra nemmeno in coda, e
se c'era già non viene consegnato a nessun worker. Quando una capacità **si accende**, i job che
avevano aspettato vengono **annullati**, non eseguiti: nulla si mette in moto perché qualcuno ha
cambiato una riga in un file. Le copie che servono si rimettono in coda dal fascicolo, con «Riprova
copie».

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
ordinario. E **non scarica allegati**: lo storico rende consultabile la posta vecchia, non porta sul
disco gli zip di due giorni di archivio per scompattarli e analizzarli. Se poi qualcuno apre una
vecchia richiesta e vuole quei file, li scarica con «Scarica».

**`[retention].giorni_job`** — per quanti giorni si tengono i job già chiusi. `0` = non cancellare
niente; una coda che non si svuota mai diventa illeggibile.

**`[retention].cache_gg`** e **`cache_max_mb`** — la politica della **cache dei contenuti** (Pre-7,
D31). Un contenuto si toglie da `_contenuti` solo se nessuno ne ha bisogno adesso — nessun documento
che aspetta la copia, nessuna proposta aperta, nessun job pendente, nessuna anomalia NAS — **e** da
`cache_gg` giorni non lo tocca nessuno (assente = 30; `0` = mai per età), **oppure** quando la cache
supera `cache_max_mb` (assente = nessun limite): allora si parte dal meno usato, e si toglie il
minimo che basta. Un file tolto si riprende da Outlook o dall'archivio da cui era uscito, e la copia
sul NAS lo fa da sola. `giorni_staging`, la voce di prima, vale come `cache_gg` se `cache_gg` manca.

### Com'è fatto lo staging

Dal blocco 4A lo staging è organizzato **per contenuto**, non per messaggio, e dal Pre-7 `_contenuti`
è dichiaratamente una **cache**, non una coda:

```
_staging\
  _parti\        <allegato>.parte.<lease_token>   un trasferimento in corso
  _contenuti\
    ad\          ad644f0c….pdf                    un contenuto verificato: il nome è il suo sha256
  log\
```

Prima ogni messaggio aveva la sua cartella e dentro ci finivano i suoi allegati, più una
sottocartella con lo zip estratto. Lo stesso disegno allegato a cinque richieste stava sul disco
cinque volte; uno zip da 6 MB con dentro 30 MB di file ne occupava 36 **per ogni messaggio** in cui
compariva. Ora lo stesso contenuto è un file solo, e gli allegati che lo condividono condividono il
percorso: chi cancella quel file li lascia senza tutti insieme, e tutti hanno «Riscarica».

| Cartella | Natura | Politica |
|---|---|---|
| `_parti\` | temporaneo (`<allegato>.parte.<token>`, `zip.<token>\`) | vuota a regime: lo scheduler toglie gli orfani dei tentativi finiti |
| `_contenuti\` | **cache** per sha256 | **resta dopo la copia sul NAS**; il custode la svuota per età o per capienza, mai sotto a chi la usa |
| `tmp\` (nello staging del **worker**) | di passaggio | svuotata a ogni avvio del worker |
| `log\`, `restrict.json` | log e stato del self-test | non si toccano |

Perché la cache resta dopo la copia: serve a non riscaricare da Outlook lo stesso file ricevuto
un'altra volta, a rianalizzarlo quando cambia il dizionario, e a **ricopiarlo sul NAS** se un giorno
il file sparisce (Admin › Integrità NAS). Prima del blocco 7 la pulizia toglieva solo i contenuti
che nessun allegato nominava più — e gli allegati non si cancellano mai, quindi non toglieva niente.

**I pin.** Il custode (`staging.Cache`, una passata ogni sei ore) non toglie un contenuto finché: un
documento confermato con quell'hash è `in_coda` o in `errore`; una proposta su un allegato con
quell'hash è aperta; un job pendente lo cita (per hash, per allegato o per documento); un'anomalia
NAS aperta riguarda un documento con quell'hash. E un **archivio** resta finché una sua voce è
pinnata — anche una voce che sul disco non c'è più: è da lui che si riestrae. Alla rimozione tutti
gli `allegato.path_staging` che puntavano a quel contenuto vanno a `NULL` nella stessa transazione;
lo stato dell'allegato resta com'è. Ogni contenuto tolto lascia una riga di log con hash, byte,
motivo e allegati toccati.

**La ripresa.** Se una copia sul NAS trova il contenuto sparito, non si ferma a chiedere
«Riscarica»: se la voce viene da un archivio ancora in cache accoda la riestrazione, altrimenti
accoda il download da Outlook, e riprova da sola al tentativo dopo. Insiste una volta sola per copia:
se quel download è già stato provato ed è fallito, torna il messaggio con «Riscarica», perché a quel
punto serve una persona.

Conseguenza da conoscere: dopo un «Riscarica» che porta byte diversi, il contenuto vecchio **resta
sul disco** finché non passa il custode — il file nuovo ha un nome nuovo, perché il nome è il
contenuto. È il prezzo di non averne mai due copie.

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
cd cockpit
.\cockpit.exe -config cockpit.toml -semina-anagrafica ..\docs\seme_anagrafica.json
```

Legge un file JSON di clienti, domini e buyer, lo convalida **per intero** e poi scrive, e **esce**.

**Il percorso del seme è relativo alla cartella da cui si lancia, non a `cockpit.toml`.** Lanciato da
`cockpit\` con il solo nome del file, il programma cerca `cockpit\seme_anagrafica.json`: se il seme
sta altrove il comando fa tutto il resto — migrazioni, utenti, fondazioni — e poi si ferma con

```
errore: open seme_anagrafica.json: Impossibile trovare il file specificato.
```

Non è un errore di configurazione e non ha scritto niente dell'anagrafica: il file si legge e si
convalida prima di aprire la transazione. Si rilancia con il percorso giusto.

Il file non sta nel repository: nomi dei clienti, domini, indirizzi dei buyer e forme dei loro codici
sono dati dell'azienda. In questo ambiente sta in `docs\seme_anagrafica.json` (la cartella `docs\`
è fuori dal repository pubblico); su una postazione lo tiene chi amministra il Cockpit, accanto a
`cockpit.toml`.

#### Seminare i fornitori (blocco 7A.4)

```powershell
.\cockpit.exe -config cockpit.toml -anteprima-fornitori ..\docs\seme_fornitori.json   # guarda e NON scrive
.\cockpit.exe -config cockpit.toml -importa-fornitori   ..\docs\seme_fornitori.json   # scrive
```

È **lo stesso motore** della schermata Admin › Anagrafica › Fornitori › Importa: stesso lettore del
file, stessa anteprima, stessa scrittura. Non esiste un secondo importatore. L'anteprima dice che
cosa verrebbe creato, che cosa c'è già e che cosa **non** si riesce a risolvere (una lavorazione che
non esiste, un cliente che in anagrafica non c'è): quelle righe non vengono scritte, né adesso né
alla conferma. Un fornitore che c'è già non viene toccato, nemmeno un campo.

#### I numeri dell'anagrafica

```powershell
.\cockpit.exe -config cockpit.toml -conta-anagrafiche
```

Stampa `clienti=…`, `domini_cliente=…`, `buyer=…`, `fornitori=…`, `domini_fornitore=…`,
`contatti_fornitore=…`, `lavorazioni_fornitore=…`, `qualifiche=…`, una riga per voce, ed esce.

#### Tutto il bootstrap in un comando

```powershell
powershell -ExecutionPolicy Bypass -File scripts\semina-anagrafiche.ps1            # chiede conferma prima dei fornitori
powershell -ExecutionPolicy Bypass -File scripts\semina-anagrafiche.ps1 -Conferma  # non chiede niente
powershell -ExecutionPolicy Bypass -File scripts\semina-anagrafiche.ps1 -SoloAnteprima
```

Verifica che i due file esistano, dice su quale database sta per scrivere (senza la password),
applica le migrazioni, carica clienti/domini/buyer, mostra l'anteprima dei fornitori, li scrive solo
dopo conferma esplicita e mostra i numeri finali. Rilanciarlo non raddoppia niente. Se uno dei due
file è invalido esce con codice ≠ 0 senza aver scritto la parte che segue.

**Lo lancia anche `scripts\avvia-dev.ps1`**, prima di mettere in piedi server e worker: dopo un
`azzera-dati.ps1` il database ha lo schema ma non sa chi è nessuno, e senza anagrafiche ogni mail
arriva da uno sconosciuto. Con `-NoSemina` si salta. **Il server non semina niente da solo**: né
all'avvio né mai. Caricare un'anagrafica è una decisione, non un effetto collaterale.

**Un fornitore senza domini e senza contatti non fa cambiare quadrante a nessuna mail**: la posta si
riconosce dall'indirizzo, non dalla ragione sociale. Il seme dei fornitori nasce da un foglio dove
gli indirizzi non ci sono: o si aggiungono i domini al file, o si censiscono dall'Inbox con
«Censisci come fornitore», che è la strada pensata per questo.

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
          {"regex": "\bAC\d{5}[A-Z]\b", "descrizione": "codici ACME", "esempio": "AC12345B"}
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

### Su una postazione vera: il pacchetto e `installa-postazione.ps1` (7C.1)

`avvia-dev.ps1` è il banco di sviluppo su un PC solo e non il launcher delle postazioni. Una
postazione si installa con il pacchetto scaricato da *Admin › Postazioni* («genera e scarica il
pacchetto» di QUEL PC), che contiene `worker.toml` con i token, `ISTRUZIONI.txt`, i file del worker,
`installa-postazione.ps1` e, con TLS, `cert.pem` (il solo certificato pubblico). Sul PC, dalla
cartella in cui lo si è scompattato:

```powershell
powershell -ExecutionPolicy Bypass -File installa-postazione.ps1            # installa/aggiorna e avvia
powershell -ExecutionPolicy Bypass -File installa-postazione.ps1 -Mostra    # stato delle attività e log
powershell -ExecutionPolicy Bypass -File installa-postazione.ps1 -Ferma     # prima di rigenerare il pacchetto
```

Lo script ferma ciò che gira, installa le dipendenze Python, crea lo staging del worker, importa
`cert.pem` fra le autorità radice dell'utente (Edge e Chrome smettono di avvisare; Windows chiede
una conferma), registra un'attività pianificata per ogni worker dichiarato in `worker.toml`
(all'accesso dell'utente, istanza singola, riavvio automatico, `pythonw` senza finestra, log in
`<staging>\log`) e le avvia. Le attività girano nella sessione interattiva dell'utente perché Outlook
classico lo richiede. **Rigenerare il pacchetto invalida il precedente** nel momento in cui si preme
il pulsante: la sequenza è `-Ferma` sul PC → genera sul Cockpit → scompatta sopra → rilancia lo script.

`scripts\installa-attivita.ps1` resta per il banco (registra le stesse attività a partire dalla
cartella `workers\` del repository).
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

### Il taglio della catena di risposta

Una mail alla quarta risposta contiene quattro messaggi, e tre sono già stati letti da qualcuno.
L'interpretazione deterministica però li leggeva tutti insieme: da lì arrivano quasi tutti i falsi
multi-codice — «questa richiesta parla di sei codici» quando ne nomina uno e cita gli altri cinque
dalla conversazione di settembre.

**Il corpo originale non viene mai modificato.** Resta intero in `messaggio.corpo_testo`, nella
schermata e in Outlook. Quello che cambia è quale pezzo viene dato in pasto all'interpretazione:
`classificazione.TagliaCatena` divide il corpo in *quello che è stato scritto adesso* e *la storia citata*, e
`CorpoUtilePerInterpretazione` restituisce il primo.

Il taglio scatta solo su qualcosa di non ambiguo:

| Forma | Esempio |
|---|---|
| separatore esplicito | `-----Messaggio originale-----`, `-----Original Message-----`, `---------- Forwarded message ----------` |
| apertura di citazione | `Il giorno … ha scritto:`, `On … wrote:`, `Am … schrieb:` — prefisso **e** chiusura |
| blocco di intestazione | **due** intestazioni di ruolo diverso di seguito (`Da:`/`Inviato:`/`A:`/`Oggetto:`) che portano un indirizzo, una data o l'oggetto |
| testo marcato | due righe consecutive che cominciano con `>` |

Una riga `Da:` **da sola non taglia niente**, e nemmeno due che non portano né indirizzo né data né
oggetto: `Da: tornitura` / `A: rettifica` è un ciclo di lavorazione, non un'intestazione citata. È la
differenza fra togliere il rumore e far sparire in silenzio il messaggio di chi scrive così.

Dove finiscono i due pezzi:

- il corpo utile va nell'estrazione con l'etichetta `corpo`, e i suoi codici sono **proponibili**;
- la storia citata va nell'estrazione con l'etichetta `storia citata`: i suoi codici si **vedono**
  fra gli altri numeri trovati, e per entrare nella RFQ serve un clic. Non si buttano, perché sono
  l'evidenza migliore per agganciare una risposta alla richiesta giusta;
- il triage non conta né le parole (`richiesta d'offerta`) né il riferimento che stanno **solo** nella
  storia: ci sono in ogni catena, e conterebbero trentacinque punti di «sembra una richiesta nuova» a
  ogni «ricevuto, grazie»;
- **non si applica ai nomi degli allegati**: lì non c'è nessuna catena di risposta.

Un inoltro senza commento — «ti giro questa» e sotto la richiesta del cliente — diventerebbe un
messaggio vuoto: in quel caso si tiene il corpo intero. Meglio un codice di troppo, che si vede e si
toglie, che una richiesta che non arriva sul tavolo di nessuno.

> Le prove del repository usano un corpus **sintetico** con le forme vere. La verifica sul corpus
> reale — che sta fuori dal repository — è la condizione **A** del gate dell'agente AI, e resta
> aperta.

---

### Integrità NAS (Admin)

`stato_nas = 'scritto'` è una promessa fatta **una volta sola**, nel momento in cui la copia è
riuscita. Da allora la cartella può essere stata spostata, il file cancellato o sostituito a mano con
un contenuto diverso: il fascicolo continuerebbe a dire di sì. E al contrario, un documento `in_coda`
da tre settimane — perché la scrittura era spenta, o perché la copia è fallita e nessuno se n'è
accorto — guardando la riga non si distingue da uno confermato cinque minuti fa.

Un ricognitore va a guardare i file veri ogni `[nas].intervallo_integrita_s`, e quello che non torna
finisce in *Admin > Integrità NAS*. Prende i documenti **controllati meno di recente per primi**, al
massimo 200 per passata: a giro si arriva a tutti senza rileggere ogni volta l'intero fascicolo.

| Problema | Che cosa vuol dire | Azione |
|---|---|---|
| `in_attesa` | confermato da più di mezz'ora, nessuna copia in coda. Il dettaglio dice se è perché `nas_scrittura` è spenta | **riaccoda** |
| `errore` | la copia ha provato e non ce l'ha fatta; il dettaglio porta il motivo | **riaccoda** |
| `mancante` | il database dice `scritto`, il file non c'è | **riaccoda** |
| `gia_presente` | il file è già sul NAS **con l'hash giusto** e il documento risulta ancora da copiare: non c'è niente da copiare | **allinea** |
| `conflitto` | sul NAS c'è un file **diverso** da questo documento | **nessuna**: va guardato |
| `illeggibile` | il file c'è ma non si riesce a leggerlo, oppure al suo posto c'è una cartella con lo stesso nome | **nessuna**: va guardato |

Tre regole che non cambiano:

- **un conflitto non si sovrascrive mai.** Non c'è nessun pulsante che decida quale dei due file sia
  quello buono: sovrascrivere vorrebbe dire buttare via il file di qualcun altro senza sapere di chi
  fosse. La stessa cosa vale per la copia vera — anche riaccodandola a mano, il copiatore rifiuta;
- **se il NAS non è raggiungibile la passata non si fa.** Farla direbbe che mancano *tutti* i file, e
  da quel momento la volta in cui ne manca uno davvero non si distinguerebbe più dalle altre. La
  schermata lo dice: «NON controllato»;
- **il ricognitore non ripara niente da solo.** Scrive `documento.verificato_il` e le righe di
  `nas_anomalia`, e basta. Le due azioni sono gesti di una persona, come «Riprova copie».

Legge il NAS anche con `nas_scrittura` **spenta** — leggere è sempre consentito, e un server che non
scrive può benissimo accorgersi che un file dichiarato nel fascicolo non c'è più. Con
`intervallo_integrita_s = 0` non gira da solo, e resta «Controlla ora».

La stessa notizia arriva **in fondo alla pagina della RFQ**, perché chi aspetta quel disegno guarda
quella, non l'Admin.

---

## Come funziona, in breve

Il server accoda `sync_outlook` ogni `[outlook].intervallo_sync_s`; il worker legge le cartelle
indicate in `[outlook].cartelle` a partire dal cursore (la prima volta da `[outlook].dal`) e manda i
messaggi a lotti. A ogni messaggio l'ingest scrive **chi c'è dall'altra parte** (la controparte,
blocco 7A): cliente, fornitore, interno, sconosciuto o ambiguo, risolta dall'anagrafica in
quest'ordine — contatto esatto, poi dominio. L'Inbox è divisa in cinque **quadranti** (7C.0): **Clienti**,
**Fornitori** (offerte, domande, le nostre richieste), **Interni** (posta fra i nostri indirizzi), **Altro**
(soggetti censiti come diversi: corrieri, banche, notifiche) e **Da validare** (chi non è censito, chi sta
in più anagrafiche); la direzione ↓/↑ è un filtro, e il quadrante dipende **solo** dalla controparte. Dal
quadrante Clienti l'operatore decide: **Nuova RFQ**, **Aggancia a…**, **Ignora**. Un fornitore non
apre mai una RFQ cliente: la sua posta si aggancia. Da «Da validare» si fa **Censisci come
fornitore / cliente** con l'indirizzo e il dominio già scritti: al salvataggio i messaggi non ancora
decisi con lo stesso indirizzo o dominio vengono ricalcolati (quadrante, controparte, proposta); le
decisioni prese non si toccano. Dentro una RFQ spunta gli allegati che servono: vengono scaricati in staging, analizzati e
proposti; con **Conferma → NAS** diventano documenti copiati nella cartella della RFQ con verifica
dell'hash. Restano manuali **Apri in Outlook**, **Segna letto** e **Rispondi**, che prepara una bozza:
l'invio non è mai automatico.

**Su quale PC.** Un job va solo a un worker che serve la casella del job — cioè che l'ha trovata nel
proprio profilo Outlook ed è autorizzato a leggerla — e, se è un'azione interattiva (Apri, Segna
letto, Bozza), solo al worker della **postazione da cui l'operatore sta lavorando**. La sessione del
browser sa qual è: al login, se un worker attivo di una postazione autorizzata ha fatto claim dallo
stesso indirizzo IP, la prende da lì; altrimenti l'operatore la sceglie dalla testata («Sei su: …»).
Senza postazione le azioni su Outlook sono disabilitate con il motivo: una finestra aperta su un altro
PC non è un'azione riuscita. La testata dice anche, per ogni casella, se c'è un worker che la serve
(**attiva** su quale PC, **OFFLINE**, **non risolta** nel profilo, **non configurata**).

---

## Fornitori, controparte e convenzioni di codice (blocco 7A, migrazione 0014)

**Anagrafica › Fornitori** (`/admin/fornitori`, amministratore): ragione sociale (unica, a meno delle
maiuscole), tipo (`materie_prime` | `processi` | `verniciatore`), lingua, note; **domini** (uno
appartiene a un fornitore solo, come per i clienti: se è già di un altro la schermata dice di chi è)
e **contatti** esatti (per chi scrive da gmail, o per un gruppo il cui dominio è anche di un cliente:
il contatto esatto vince sul dominio); le **lavorazioni** che sa fare (caselle sulla tabella
`lavorazione`: tornitura, fresatura, taglio laser, piega, curvatura tubi, saldatura, zincatura,
cataforesi, verniciatura a polvere, lavaggio zinco, sabbiatura, trattamento termico, attrezzature); le
**qualifiche per cliente** (`cliente_fornitore_lavorazione`: «questo cliente lo ha abilitato a fare
questa lavorazione»; la chiave esterna composta impedisce una qualifica su una lavorazione che il
fornitore non dichiara, e impedisce di togliere la capacità finché una qualifica la usa). Niente
JSON: un fornitore non ha regole di codice.

**Importa da file** (`/admin/fornitori/importa`): un JSON `{"fornitori": [...]}` con anteprima
delle differenze — fornitori da creare, righe da aggiungere, righe già presenti, **non risolti**
(un cliente che non esiste con quella cartella NAS, una lavorazione fuori catalogo, una qualifica su
una capacità non dichiarata) — e conferma esplicita. L'import non sovrascrive un fornitore che c'è
già, non sposta un dominio, non inventa: i non risolti restano tali. Un secondo import dello stesso
file non scrive niente.

**La controparte del messaggio** (`messaggio.controparte_*`, D33) è un fatto scritto dall'ingest,
non un calcolo della vista: `interno` se il mittente è una nostra casella (in uscita si guarda il
primo destinatario esterno), poi il contatto esatto (fornitore o buyer; in entrambe le anagrafiche →
`ambiguo`), poi il dominio (fornitore o cliente; in entrambi → `ambiguo`), altrimenti `sconosciuto`.
Per costruzione un fornitore in entrata non produce mai la proposta `nuova_rfq`. Una controparte
decisa da una persona (`via = manuale`) non viene sovrascritta dal ricalcolo. Al primo avvio dopo la
0014 il server ricalcola a lotti i messaggi già in database (log: conteggi per tipo prima e dopo).
Aggiungere un dominio, un contatto o un buyer dall'anagrafica — o censire dall'Inbox — ricalcola i
messaggi **non decisi** dello stesso indirizzo o dominio e dice quanti.

**Convenzioni di codice** (scheda cliente › «Lavorazioni e fornitori», D39): come il cliente scrive la
lavorazione superficiale nel codice del pezzo. Ogni convenzione ha un modo (`suffisso`, confrontato
in fondo al codice senza distinguere le maiuscole, al massimo 12 caratteri senza spazi; oppure
`regex`), un esempio che deve corrispondere, un controesempio facoltativo che non deve, e almeno una
lavorazione. Le convenzioni passano dalla stessa doppia porta delle regole di riconoscimento: in
scrittura ciò che non torna viene rifiutato (`regole.ValidaConvenzione`), in lettura una riga rotta
scritta a mano in database si vede con ✗ e non si usa (`regole.LeggiConvenzioni`). Il risultato per
un codice è l'**insieme** delle lavorazioni di tutte le convenzioni attive che corrispondono, con
l'evidenza: nessuna precedenza nascosta. «Prova un codice» nella scheda cliente mostra lavorazioni e
fornitori qualificati con lo stesso codice che userà l'ingest. Nessun suffisso reale è nel seme: si
scrivono dall'anagrafica, cliente per cliente.

## Richieste ai fornitori e intento del messaggio (blocco 7B, migrazione 0015)

> Dal 7C.0 (sotto) l'**intento** non esiste più: al suo posto ci sono l'**atto** e il **legame**. Questo
> paragrafo resta per la storia del 7B; le regole valide sono quelle del 7C.0.

**L'intento** era che cosa il messaggio è (`proposta_triage.intento`, enum): `rfq_cliente`, `offerta_promatec`,
`rfq_fornitore`, `offerta_fornitore`, `risposta_fornitore`, `domanda_fornitore`, `inoltro_interno`, `non_rfq`,
`incerto`. L'esito resta che cosa si propone di fare. Il ramo lo decide la controparte: da un **cliente** il
triage di sempre; da un **fornitore** l'intento viene dal testo e l'esito è «aggancia» verso
la nostra richiesta a lui, mai `nuova_rfq`; da un mittente **sconosciuto** o **ambiguo** solo `incerto` o
`non_rfq`, senza proposta di RFQ: prima si decide chi è (Censisci), poi che cosa vuole (D34). Nella posta di un
fornitore si cercano codici **solo** con le famiglie dei clienti che gli hanno mandato richieste: S235JR, ISO
2768, DIN 933 non sono codici di nessuno.

**Le richieste ai fornitori** (`richiesta_fornitore`, figlie della RFQ cliente: fornitore, lavorazione, codici,
stato `bozza` → `inviata` → `risposta` | `scaduta` | `annullata`). **Prima di B8.2 il box è stato tolto dalla
pagina della RFQ**, e non passa al Fascicolo. Creava una richiesta per l'intera RFQ partendo dai suoi codici,
mentre una richiesta a un fornitore nasce da una lavorazione di un componente che si è deciso di fare fuori.
Tornerà nel tab Luigi, quando ci sarà il modello delle lavorazioni per componente. **Il flusso a livello di
RFQ è stato tolto di proposito: non va reintrodotto, né sulla pagina della RFQ né nel Fascicolo, prima che
esista il modello delle lavorazioni per componente.** Anche `caricaThread` non carica più richieste, fornitori
e lavorazioni per quella pagina. Tabelle, rotte
(`POST /thread/{id}/richiesta…`) e logica restano, per lo storico e la migrazione; quanto segue descrive che
cosa fanno. Il form «Nuova richiesta a un fornitore» (la tendina diceva per quali lavorazioni il cliente lo ha
qualificato) creava la richiesta e, se si voleva, la **bozza in
Outlook** con oggetto `RFQ <cliente> <buyer> <codici>`, i contatti del fornitore come destinatari e il marcatore
`CockpitRichiestaFornitore` scritto dal worker (UserProperties). Quando la mail compare nella Posta inviata il
sync rilegge il marcatore: la richiesta prende la sua mail e passa a `inviata`, la mail entra nella RFQ, la
bozza risulta partita (`CockpitBozza`). Nessuna euristica sull'oggetto. Se la mail è stata **mandata a mano**,
una nostra mail a un fornitore che cita un codice identificativo di una RFQ aperta propone «richiesta a X per la
RFQ Y» (regola `RF_oggetto`) e l'operatore conferma: nasce la richiesta, mai un thread.

**La risposta del fornitore**: candidati verso la richiesta, sempre come proposta e tutti visibili — `R0`
(In-Reply-To/References verso la nostra mail, 95), `R1` (stessa conversazione, 80), `R3f` (un codice cliente che
appartiene a una RFQ con una richiesta a quel fornitore, 60; due RFQ con quel codice → due candidati). «È la
risposta a questa richiesta» aggancia la mail alla RFQ cliente, porta la richiesta a `risposta` e propone gli
allegati (PDF, fogli, documenti) come `offerta_fornitore`, il tipo che alla conferma li copia in
`OFFERTE FORNITORI`.

Richiede `[sicurezza].bozze` per la bozza; la richiesta nasce comunque e la frase dice perché la bozza no.
L'invio resta manuale: la bozza si apre in Outlook, si rilegge, si preme Invia lì.

## Inbox coerente e bootstrap ripetibile (checkpoint 7B.5)

**I comandi e la lista tornano insieme.** L'Inbox rende un pezzo solo, `inbox_stato`: linguette dei
quadranti, direzione, filtro, selettore della casella, contatori e righe. Ogni link lo sostituisce
per intero (`hx-target="#inbox-stato" hx-swap="outerHTML"`). Prima ogni link ripuntava alla sola
`#lista` e il server rispondeva con la sola lista, mentre le linguette stavano fuori: le righe
cambiavano quadrante e i comandi restavano quelli di prima — sullo schermo risultava acceso
«Fornitori» con sotto le righe di «Da validare». La schermata diceva una cosa e ne mostrava
un'altra, che è esattamente il difetto che l'Inbox dovrebbe aiutare a non fare.

Lo stato è **uno solo: la barra degli indirizzi**. I link lo portano scritto dentro; il poll ogni 15
secondi lo rilegge da lì (`cockpitStatoInbox()` in `layout.html`) invece di portarsi dietro i
parametri congelati all'ultimo rendering. Aprire un messaggio rifà la colonna (`HX-Trigger-After-Settle:
inbox-aggiorna`), così la riga si accende subito e i contatori si aggiornano. Il pannello di destra
resta fuori dal pezzo che si ricarica — cambiare quadrante non chiude il messaggio che si sta
leggendo — e la colonna che scorre resta la stessa, quindi il poll non fa saltare la lettura.

**Dopo un seed i messaggi già arrivati cambiano quadrante subito.** Dalla 0014 la controparte è un
fatto scritto sul messaggio all'ingest, non una domanda che l'Inbox rifà a ogni lettura: senza
ricalcolo, censire un fornitore lasciava la sua posta fra gli sconosciuti finché non arrivava una
mail nuova. Adesso l'import dei fornitori (schermata e riga di comando) e il seme dei clienti
chiudono con un **ritriage mirato** sulle sole chiavi appena scritte — domini e indirizzi — e dicono
quanti messaggi hanno riguardato. Le decisioni prese non si toccano: un messaggio già in una RFQ, o
già ignorato, non viene nemmeno letto.

## Il contratto di classificazione (blocco 7C.0, migrazioni 0016–0017)

Il 7C non collega ancora nessun modello: prima si fissa **che cosa** il server sa dire di una mail, e in
che forma. Tre domande, tre risposte separate, che il triage fino a qui mescolava.

**Chi scrive** è la **controparte**, un fatto scritto sul messaggio, ora a sei valori: `cliente`,
`fornitore`, `interno`, **`altro`**, `sconosciuto`, `ambiguo`. «Altro» è un soggetto censito
esplicitamente come diverso dagli altri tre (un corriere, una banca, le notifiche di un portale):
un'anagrafica piccola, `soggetto_altro` con un'etichetta e i suoi `recapito_altro` (un indirizzo o un
dominio, lo distingue la chiocciola). **Non è il cestino degli sconosciuti**: uno sconosciuto resta
sconosciuto finché una persona non lo censisce, e nessun automatismo lo sposta lì. Il resolver applica la
stessa scala di sempre — indirizzo esatto, poi dominio — e a parità di specificità due anagrafiche che
riconoscono lo stesso recapito danno `ambiguo`, mai una precedenza silenziosa. Basta questo perché
`newsletter@cliente.example` censito come Altro non finisca fra i Clienti: l'indirizzo vince sul dominio.

**Che cosa sta facendo il mittente** è l'**atto business** (`proposta_triage.atto`, tabella
`atto_business`): `richiesta_offerta`, `offerta`, `revisione_documenti`, `documenti_aggiuntivi`,
`domanda_chiarimento`, `risposta_chiarimento`, `sollecito`, `ordine`, `accettazione`, `rifiuto`,
`conferma_ricezione`, `inoltro`, `notifica`, `comunicazione_generica`, `non_business`, `incerto`. È neutro
rispetto alla direzione: «cliente in entrata + richiesta_offerta» è una RFQ, «fornitore in uscita +
richiesta_offerta» è la nostra richiesta a lui. Un vocabolario solo, per il deterministico e per l'agente:
la 0015 ne aveva uno e il pacchetto agente un altro. È una **tabella** e non un enum perché il vocabolario è
in evoluzione: un atto nuovo è un INSERT, un atto ritirato resta leggibile sulle proposte vecchie. Il
deterministico dice quello che sa: una mail che sembra una richiesta nuova è `richiesta_offerta`; una
risposta del cliente dentro una RFQ ha il legame ma l'atto è `incerto`, perché dal punteggio non si capisce
se è una revisione, una domanda o un sollecito (prima la chiamava `rfq_cliente`, che era falso).

**A che cosa appartiene** è il **legame operativo** (`proposta_triage.legame`, enum): `nuovo`, `risposta`,
`aggiornamento`, `inoltro`, `nessuno`, `incerto`, con il bersaglio nelle colonne che già c'erano
(`thread_proposto`, `richiesta_proposta`). È una **proposta**: il fatto resta `thread_id` /
`richiesta_fornitore_id` dopo la decisione. L'**esito** del triage (`nuova_rfq`, `aggancia`, `ignora`) resta
la proposta di azione per la schermata e non è una seconda rappresentazione del significato.

**Il quadrante lo calcola la vista.** `v_inbox.quadrante` (`clienti`, `fornitori`, `interni`, `altro`,
`validare`) dipende solo dalla controparte; Da validare = sconosciuto + ambiguo. Le tre query di
`messaggi.sql` e il Go filtrano su quella colonna: il CASE non è più scritto in due posti. La regola 7B.3 che
mandava in Da validare anche la newsletter dal dominio di un cliente **non c'è più**: quella posta sta fra i
Clienti con il chip `non_business`, e la via giusta è censire l'indirizzo automatico come Altro. `q=buyer`
nei link salvati apre ancora Clienti. La vista legge la proposta **deterministica**: prima sceglieva quella
dell'agente quando c'era, una preferenza implicita che non spetta a una vista; il confronto fra le due fonti
sarà una lettura dedicata.

**Una risposta di un fornitore non è un'offerta** (invariante 8). Fino alla 0015 «È la risposta a questa
richiesta» portava la richiesta a `risposta` qualunque cosa il fornitore avesse scritto: «ricevuto, vi
rispondiamo domani» la chiudeva. Ora la conferma **lega** la mail alla richiesta e basta; lo stato cambia
solo con l'atto scelto nel pannello: **offerta** → `offerta_ricevuta` (e gli allegati proposti come offerta
del fornitore), **declinata** → `declinata` («non quotiamo»). Lo stato `risposta` è stato rinominato
`offerta_ricevuta` (`risposta_il` → `offerta_ricevuta_il`); la 0016 **si ferma** se trova richieste già in
quello stato, perché non sa se erano offerte: si riconciliano a mano. Il 18/09/2026 erano zero.

**Gli invarianti provati** (`ingest/invarianti_7c_db_test.go`): R0 e R1 sulla posta di un fornitore sono
candidati e non legami (I4); il marcatore `CockpitRichiestaFornitore` scrive il legame **solo** sulla nostra
posta in uscita, verso una richiesta che esiste, su un messaggio che nessuno ha già messo in un'altra RFQ
(I5). Il punto (b) di I5 era rosso: un marcatore su una mail **in entrata** veniva applicato. Corretto in
`ingest/marcatori.go`.

**Che cosa il 7C.0 non fa ancora**, e viene dopo: il Dossier (`internal/core/inbox/classificazione`, il pacchetto di
fatti e candidati che si manderebbe all'agente, con lo stato di codici e revisioni nella RFQ candidata), il
parser deterministico delle intestazioni citate (`origine_citata` di un inoltro), «Censisci come Altro»
dall'Inbox, la schermata che mostra il JSON `classificazione_email.v1`, il segnale spam di Outlook nel
worker, e il contratto nuovo dell'agente. Nessuna chiamata a un modello.

## Prove

```powershell
go test ./...                        # unitari: dominio, zip, NAS, template, migrazioni, configurazione
python -m pytest -q workers          # unitari Python: stanno in workers\tests\ (i moduli provati sono una cartella sopra)
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
`internal/transport/workerapi` avvia il **worker vero** (`python workers\prova_e2e.py`, cioè worker_outlook
con `--una-volta` e il solo adattatore COM sostituito) contro i gestori HTTP veri e PostgreSQL. È
l'unico livello in cui il client e il server si parlano davvero: gli altri provano una metà sola,
e un difetto che sta nel modo in cui il client compila il contratto — non nel contratto — passa
indisturbato attraverso tutti (è successo il 15/09/2026). Richiede Python; con
`prova-tutto.ps1 -SenzaPython` viene saltato e il registro lo annota come non verificato.

### Le prove nel browser (L7)

```powershell
python -m pip install playwright                 # una volta; il browser è Microsoft Edge, già installato
$env:COCKPIT_TEST_DSN = "postgres://cockpit_test:cockpit_test@127.0.0.1:5433/cockpit_test"
go test -tags "integrazione browser" -count=1 -run TestL7 .\internal\transport\web\
```

Il tag `browser` le tiene fuori dalla corsa normale: senza Playwright il test **salta** e lo dice.
Il banco è quello di tutti gli altri test web — PostgreSQL vero, server vero su una porta vera — e
sopra ci gira Edge: `internal\web\e2e\inbox_quadranti.py` apre l'Inbox, clicca le linguette, cambia
direzione e filtro, aspetta un poll intero e usa indietro/avanti del browser, e ogni volta verifica
che **la linguetta accesa, le righe mostrate e la barra degli indirizzi dicano la stessa cosa**.

Serve perché a L4 si chiede al server un frammento e si legge l'HTML che torna. Il 18/09/2026 erano
tutti verdi mentre nel browser le linguette restavano indietro: il difetto non stava in una risposta
sbagliata, stava nel fatto che la pagina ne sostituiva soltanto un pezzo. Un difetto che vive fra due
risposte giuste lo vede solo chi guarda la pagina intera.

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
  schemi di `contracts/`: quello è il livello L3, e ha un test suo in due metà (`internal/platform/contratti/worker` per il
  Go, `workers/tests/test_contratti.py` per la premessa che gli schemi su disco siano quelli dei
  modelli di oggi). Confronta i campi e i loro generi, non l'obbligatorietà;
- un test **saltato** non è un test superato: è una verifica che non è stata fatta.

## Manutenzione

```powershell
sqlc generate                                       # dopo aver toccato migrations/ o internal/platform/db/queries/
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
5. `sqlc generate`, poi `go test ./internal/platform/migrazioni/`, che controlla i punti 2, 3 e 4 senza database.

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
| una casella in testata è **OFFLINE** | quel worker non si fa sentire da oltre 40 secondi (due giri di claim, `worker.PresenzaOnlineEntro`) | il worker di quel PC è fermo: vedere il suo log. Non conta da quanto non *conclude* un claim: un worker dentro un sync di tre minuti non ne conclude nessuno e resta online grazie al battito |
| una casella in testata è **non risolta** | il worker è attivo ma non trova la casella nel profilo Outlook del suo PC (o Outlook non risponde) | aggiungere la casella al profilo, o aprire Outlook; `python worker_outlook.py --caselle` dice che cosa vede |
| una casella in testata è **non configurata** | nessun `[[worker]]` la elenca fra le proprie `caselle` | aggiungerla al worker della postazione che deve servirla |
| «Apri in Outlook» dice *nessuna postazione* | la sessione non è abbinata a nessun PC | scegliere il PC dalla testata («Sei su:») |
| «Apri in Outlook» dice *non viene dirottata* | il worker della postazione scelta non serve nessuna casella in cui il messaggio è presente | autorizzare quella casella al worker, o lavorare dalla postazione che la serve |
| i job restano `pronto` | nessun worker che serva quella casella (o quella postazione) è in esecuzione | avviare il worker corrispondente; la testata dice quale |
| un job torna `in_corso` e si ripete, il worker logga `400 worker_id mancante` | il worker riporta il risultato senza dire quale tentativo sta chiudendo | difetto corretto il 15/09/2026: aggiornare i worker insieme al server |
| in testata c'è **SHADOW** e metà dei pulsanti dice «in attesa di produzione» | il server gira in sola lettura (voce 9.5) | è voluto finché si prova sulla posta vera; per riattivare le scritture: `[server].modalita = "produzione"` e riavvio |
| una casella resta **in corso…** per minuti | il sync di quella casella è in coda o in esecuzione | normale al primo giro su una casella grande; se non finisce mai, il worker è fermo: vedere il suo log e `/admin/job` |
| il sync dura minuti invece di secondi | `Restrict` è spento su quella cartella | il log del worker dice perché: `self-test Restrict FALLITO` (il filtro perdeva elementi) o `Restrict non disponibile`. `python worker_outlook.py --restrict 7` lo rimisura. L'esito è ricordato in `_staging\restrict.json`: cancellarlo fa rifare le prove |
| una cartella porta molti meno messaggi di quelli che ha | un elemento anomalo fermava la scansione | corretto nel checkpoint 3R. Per vederlo su una cartella vera, senza server e senza database: `python prova_lettura.py --casella indirizzo --cartella "Posta in arrivo" --giorni 30 --vecchio-modo` — se il vecchio modo cade e il nuovo no, l'elemento c'è ed era lui |
| in `/admin/scarti` compaiono elementi che non entreranno mai | erano `origine = lettura` di elementi non-mail | dal 3R un elemento non-mail (rapporto di consegna, invito, appuntamento) viene contato e scritto nel log, non messo in scarto: in scarto ci va solo ciò che ha senso rileggere |
| lo stesso sync parte più volte con la stessa finestra | il `result` non viene accettato, il lease scade e lo scheduler riaccoda | leggere `cockpit.log`: il server scrive il motivo del rifiuto con il nome del worker |

---

## Struttura

```
internal/README.md                  com'è diviso cockpit.exe, la regola di dipendenza, i flussi, i livelli di prova;
                                    un README per area: internal/{core,platform,transport,ai,app}/README.md
cmd/cockpit/main.go                 la riga di comando: flag, runtime.Opzioni, codice di uscita
embed.go                            embed.FS di migrations/, web/templates, web/static e workers/ (il pacchetto della postazione)
internal/platform/config            cockpit.toml: lettura, normalizzazione e verifica di caselle, postazioni, worker
internal/platform/contratti/worker  contratti JSON worker ↔ server (tipi Go; speculari a workers/contratti.py)
internal/platform/db                sqlc: queries/*.sql → codice generato (non modificare a mano)
internal/platform/migrazioni        applica migrations/*.sql in ordine, una transazione per file; verifica statica
internal/platform/fondazioni        seed non distruttivo di caselle, postazioni e credenziali dei worker da cockpit.toml;
                                    utenti.go: gli utenti e i loro ruoli, con la password del file che serve a nascere e non a riscrivere
internal/platform/rete              TLS del listener: carica o genera il certificato autofirmato e ne calcola l'impronta;
                                    impronta e generazione dei token dei worker (voce 2.4)
internal/platform/logfile           il log del server su file, con rotazione (5 x 5 MB)
internal/platform/coda              accoda idempotente (un solo job PENDENTE per chiave), claim/lease/tentativo, scheduler, instradamento per
                                    postazione e casella; capacita.go: le tre capacità di scrittura (`[sicurezza]`), che cosa non si accoda e che
                                    cosa si annulla quando si accendono (blocco 4)
internal/platform/storage/staging   lo staging per contenuto (_parti con il token del tentativo, _contenuti con lo sha256 per nome, le cartelle
                                    di estrazione); cache.go: il custode della cache (Pre-7, D31)
internal/platform/storage/nas       scrittore NAS: .parte + verifica hash, mai sovrascrive, long-path
internal/platform/storage/archivio  estrazione zip (zip-slip, limiti); le voci finiscono fra i contenuti, con il proprio sha256 per nome
internal/platform/testutil          pool e schema pulito per i test d'integrazione (COCKPIT_TEST_DSN)
internal/core/inbox/classificazione regole pure + test: codici, proposta dal nome file, portale, scadenza, triage, oggetto ripulito dai RE:/FW:,
                                    taglio della catena di risposta (catena.go); controparte.go: il resolver cliente/fornitore/interno/ambiguo (D33);
                                    atto.go: l'atto business e il legame operativo (7C.0), le euristiche pure per ramo; i candidati verso una richiesta;
                                    regole.go: il motore che compila le regole del cliente e le applica a un testo
internal/core/inbox/ingest          FATTO (messaggio, allegato) + proposta economica + aggancio automatico + triage/portale;
                                    controparte.go: la controparte scritta sul messaggio, il ritriage mirato, il ricalcolo all'avvio;
                                    marcatori.go: CockpitRichiestaFornitore e CockpitBozza letti dalla Posta inviata (7B)
internal/core/inbox/aggancio        i candidati di aggancio R0–R5 con evidenza (mai thread_id); richieste.go: R0/R1/R3f verso una richiesta
                                    a un fornitore e RF_oggetto per la richiesta mandata a mano (7B)
internal/core/registro/fornitori    l'import del seme dei fornitori con anteprima e conferma (7A.4)
internal/core/registro/anagrafica   il seme dei clienti da seme_anagrafica.json (-semina-anagrafica), una volta e senza sovrascrivere;
                                    anagrafica.go: NomeCognome, il precompilato del buyer dal display name o dall'indirizzo
internal/core/registro/regole       lo schema di cliente.regole: la porta in scrittura che rifiuta, quella in lettura che segna ✓/✗;
                                    convenzioni.go: suffisso/regex → lavorazioni, con esempio e controesempio verificati (D39)
internal/core/rfq/documenti         il fascicolo di una RFQ. path.go: i nomi sul NAS (cartella della RFQ, sottocartella per tipo e codice, nome di
                                    file sicuro; sempre relativi alla radice); integrita.go: il ricognitore che confronta i documenti con i file
                                    veri e scrive nas_anomalia, e Allinea che su un conflitto rifiuta (blocco 5B); copia_nas.go: che cosa
                                    significa copiare un documento sul NAS, e dove se ne ritrova il contenuto; ripresa.go: il contenuto sparito
                                    dalla cache che si riprende da solo (Pre-7); cartella_thread.go: la cartella di una RFQ e le sue sottocartelle
internal/ai/agente                  l'assistente semantico: Modello (interfaccia), prompt, grounding e idempotenza (analisi_messaggio).
                                    SPENTO senza [agente].attivo, modello e chiave, e solo sulle caselle elencate; nessuna chiamata
                                    reale nei test
internal/transport/web              HTML+HTMX: login (postazione per IP), /sessione/postazione, /inbox, /messaggio/{id} (+triage, scarica; apri/letto/bozza
                                    instradati alla postazione della sessione), /thread/{id}, /proposta/{id}/{conferma,scarta}, /cruscotto, /admin/job (+annulla);
                                    server.go: il tipo Server, i template, Registra (che chiama le quattro registra* delle aree), la sessione, il rendering;
                                    routes_inbox.go, routes_rfq.go, routes_admin.go: le tre aree, ognuna con le sue rotte;
                                    inbox_viva.go: «Aggiorna ora», stato del sync per casella in testata, «nuove dall'ultima visita» (voce 2.16);
                                    postazioni_admin.go: /admin/postazioni, il pacchetto del worker con token e impronta (voce 2.4, D22);
                                    anagrafica.go, anagrafica_admin.go, convenzioni_admin.go: /admin/anagrafica (clienti, con «Lavorazioni e fornitori»);
                                    fornitori_admin.go: /admin/fornitori e /admin/fornitori/importa; censisci.go: «Censisci come fornitore / cliente» dal pannello;
                                    richieste.go: le richieste ai fornitori dalla RFQ (con la bozza marcata), le conferme dall'Inbox (7B);
                                    integrita_admin.go: /admin/nas;
                                    e2e/inbox_quadranti.py: le prove dell'Inbox in un browser vero, lanciate da inbox_browser_test.go (tag `browser`)
internal/transport/workerapi        /api/v1/jobs/{claim,heartbeat,result}, GET /api/v1/worker/caselle, /api/v1/ingest/messaggi, PUT /api/v1/allegati/{id}/file
                                    (X-Cockpit-Token con il token INDIVIDUALE del worker: il server lo cerca per sha256 e da lì sa chi chiama);
                                    il claim interseca le caselle dichiarate con la credenziale e registra presenza e casella_store PRIMA del long-poll;
                                    `auth` è anche il punto in cui ogni richiesta autenticata aggiorna `worker_presenza.ultimo_contatto` (online/offline);
                                    il file caricato resta in _parti finché il result valido non lo promuove fra i contenuti; dopo-staging (rumore,
                                    analisi, e per un archivio l'accodamento di estrai_archivio); archivi.go: l'estrazione vera, eseguita dal server
internal/app/runtime                esegui.go: l'avvio nel suo ordine, un passo per riga; avvio.go: ApriLog, ApriDatabase, Semina, ImpostaCapacita;
                                    comandi.go: i lavori della riga di comando; servizi.go: Servizi e CostruisciServizi; ascolto.go: PreparaTLS, Ascolta;
                                    esecutore.go: i job di tipo 'server' — prende il job, riconosce il tipo e chiama chi sa farlo (core/rfq/documenti
                                    per il fascicolo, transport/workerapi per gli archivi, ai/agente per l'analisi);
                                    vigilanza_nas.go: quali job vogliono il NAS, il rinvio quando non c'e', il ritorno in coda quando torna
web/templates, web/static           template html/template, style.css, htmx 2.0.4
migrations/                         0001_schema.sql (30 tabelle, 5 viste, 31 enum), 0002_fondazioni.sql (caselle, postazioni, worker),
                                    0003_coda_ingest.sql (tentativo con lease_token, ingest_scarto, analisi_fatti),
                                    0004_caselle_presenza.sql (messaggio_casella, cursore per casella, messaggio.interno, v_inbox),
                                    0005_postazioni_presenza.sql (worker_presenza per worker, sessione.postazione_id, via store_id_locale),
                                    0006_inbox_viva.sql (utente.ultima_vista_inbox), 0007_anagrafica.sql, 0008_interpretazione.sql (candidati, niente aggancio automatico),
                                    0009_presenza_contatto.sql (worker_presenza.ultimo_contatto: vivo ≠ ha appena concluso un claim)
                                    0010_copertura_sync.sql   (sync_cursore.coperto_fino_a: fin dove si è GUARDATO ≠ qual è la mail più recente)
                                    0011_sync_apertura_inbox.sql (sessione.sync_inbox_il: un aggiornamento alla prima apertura, una volta per sessione),
                                    0012_estrai_archivio.sql (tipo_job: scompattare uno zip è un job dell'esecutore interno, non un pezzo della richiesta HTTP),
                                    0013_integrita_nas.sql (nas_anomalia: il ricognitore dell'integrità NAS, blocco 5B),
                                    0014_fornitori.sql (fornitore, domini e contatti, lavorazione, capacità e qualifiche, convenzioni di codice,
                                    la controparte sul messaggio; v_inbox con controparte_tipo e controparte),
                                    0015_richiesta_fornitore.sql (richiesta_fornitore, candidato_richiesta, intento e bersaglio della proposta,
                                    messaggio/bozza.richiesta_fornitore_id; v_inbox con triage_intento),
                                    0016_classificazione.sql (controparte `altro` + soggetto_altro/recapito_altro, atto_business al posto di
                                    intento_messaggio, legame_operativo, richiesta risposta → offerta_ricevuta + declinata),
                                    0017_controparte_altro.sql (CHECK con altro; v_inbox con triage_atto, triage_legame e quadrante)
contracts/*.schema.json             JSON Schema generati da workers/contratti.py
workers/                            cockpit_client.py (client, config, log, battito), worker_outlook.py, worker_analisi.py,
                                    outlook_com.py (COM), contratti.py (pydantic), server_finto.py (prove senza server),
                                    prova_e2e.py (il worker vero senza COM, per il test end-to-end),
                                    prova_lettura.py (legge una cartella vera, sola lettura, senza server né database), worker.toml.
                                    Questi file viaggiano anche dentro cockpit.exe: sono il pacchetto che la pagina Postazioni scarica
workers/tests/                      i test dei worker, con conftest.py che mette la cartella sopra in sys.path e
                                    finti_outlook.py (la cartella Outlook finta, condivisa fra i test)
scripts/                            avvia-dev.ps1 (semina le anagrafiche, poi avvia server e worker), ferma-dev.ps1,
                                    semina-anagrafiche.ps1 (bootstrap: clienti, buyer, fornitori, con anteprima e conferma),
                                    db-test.ps1 (DB di prova isolato), prova-tutto.ps1,
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
  `worker_presenza` tiene due tempi diversi per ogni worker: `ultimo_contatto` (l'ultima richiesta autenticata di
  qualunque tipo — ingresso del claim, battito, ingest, upload — ed è l'unica cosa su cui si decide online/offline)
  e `ultimo_claim` (l'ultimo claim concluso, NULL finché non se n'è concluso nessuno: diagnosi, non liveness).
  I tempi del protocollo stanno in `internal/platform/contratti/worker/protocollo.go` e in `workers/protocollo.py`, e un test di contratto
  verifica che le due copie coincidano.
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
| `sync_cursore` | SPEC §3.2 passo 1: «chiede al server il cursore della casella». Dalla 0004 la chiave è **(casella, cartella)**: con la sola cartella due caselle si sovrascrivevano il cursore a vicenda. Dalla 0010 ci sono **due frontiere e una misura**: `coperto_fino_a` è fin dove Outlook è stato scandito per intero (avanza solo a finestra conclusa, ed è lei a decidere la finestra successiva), `storico_fino_a` è fin dove indietro è arrivato «Carica precedenti», `ultimo_received` è la mail più recente che abbiamo — avanza per lotto e non decide niente |
| `worker_presenza` | ultimo contatto e ultimo claim per worker: la UI segnala «OFFLINE» invece di lasciar crescere la coda in silenzio |
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
