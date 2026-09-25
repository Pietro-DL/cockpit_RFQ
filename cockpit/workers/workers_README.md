# I worker — come parlano con `cockpit.exe`

Documentazione dei due client Python (`worker_outlook.py`, `worker_analisi.py`), dei moduli comuni
(`cockpit_client.py`, `contratti.py`, `protocollo.py`, `outlook_com.py`, `step_struttura.py`), dei comandi
diagnostici `diagnostica_step.py` e `prova_lettura.py` e di `installa-postazione.ps1`. Il lato server dello stesso
protocollo sta in `internal/transport/workerapi` (le rotte) e in `internal/platform/coda` (la coda). Il file descrive
il codice com'è: il sync legge una finestra chiusa decisa dal server e le frontiere avanzano solo a finestra percorsa;
ciò che il server modifica fuori da sé passa dalle capacità di `[sicurezza]`; i file scaricati stanno nella cache per
contenuto del server e ogni worker ha una cartella sua; gli archivi li scompatta il server.
Quando il codice cambia, cambia questo file.

## 1. Il principio: il server non chiama mai nessuno

Il server Go non conosce l'indirizzo di nessuna postazione e non apre connessioni verso i PC. Sono i worker
a collegarsi, sempre in uscita, verso un solo indirizzo (`server_url` in `worker.toml`). Per questo:

- serve **una** regola di firewall, in ingresso sul server, e nessuna sui PC;
- un worker spento è semplicemente un worker che non chiama; il server lo capisce dal tempo passato
  dall'ultimo contatto (`worker_presenza.ultimo_contatto`, 0009), non da un ping e non dall'ultimo claim: un
  worker che sta lavorando da tre minuti è online. `ultimo_contatto` si aggiorna a ogni chiamata autenticata, ma
  con un UPDATE: la riga della presenza nasce al primo claim **accettato**, così un claim rifiutato con 403 non fa
  comparire in testata un worker che non servirà niente;
- "chi fa cosa" è deciso dal server al momento del **claim**: il worker chiede *«c'è lavoro per me?»* e il
  server risponde con il primo job (per `priorita`, poi per `job_id`) compatibile con il suo tipo, la sua
  postazione, le sue caselle e le capacità di scrittura accese.

Tutto ciò che un worker scrive nel database passa dal server, mai da una connessione diretta a PostgreSQL.

## 2. Il ciclo di vita di un job

Un job è una riga di `job` (`stato`, `tipo`, `payload` JSON, `casella_id`, `postazione_id`, `lease_*`).
La riga la **crea sempre il server**: o su richiesta della UI (un clic dell'operatore) o dallo scheduler.
Il worker non inserisce mai job; li prende, li esegue, ne riporta l'esito.

```
                     SERVER (Go)                                        WORKER (Python)
  UI: POST /messaggio/{id}/scarica  ──► INSERT job(stato='pronto')
  scheduler ogni intervallo_sync_s  ──► INSERT job sync_outlook (uno per casella, chiave fissa)
  (0 = mai da solo; resta il sync di apertura dell'Inbox, una volta per sessione)

                                            ◄── POST /api/v1/jobs/claim  {worker, worker_id, postazione,
                                                                          outlook_ok, caselle_aperte, attesa_s,
                                                                          ultimo_arresto?}
  prova di vita (auth: ultimo_contatto)
  presenza dichiarata PRIMA dell'attesa
  long-poll fino a attesa_s (≤ 0 o > 25 → 20 s)
  UPDATE job SET stato='in_corso', tentativi=tentativi+1,
     worker_id, lease_token=uuid, lease_fino_a=now()+lease_s,
     avviato_il=now()  … FOR UPDATE SKIP LOCKED
     (esclusi i tipi che le capacità bloccano)
                                            ──► {job_id, tipo, payload, tentativi, lease_token, lease_s,
                                                 durata_max_s, casella_id?, postazione_id?}  | 204 senza job
                                                                            esegue (dispatch per tipo)
                                            ◄── POST /jobs/{id}/heartbeat   ogni min(max(5, lease_s/4), 20) s,
                                                                            da un THREAD SEPARATO
  predicato di validità (vedi sotto)         ──► 204  | 409 se il tentativo non vale più
                                            ◄── POST /api/v1/ingest/messaggi (solo sync, un lotto per volta)
                                            ◄── PUT  /api/v1/allegati/{id}/file (solo stage)
                                            ◄── GET  /api/v1/allegati/{id}/contenuto (solo analisi)
                                            ◄── POST /jobs/{id}/result  {esito, dati, errore, definitivo,
                                                                         worker_id, lease_token}
  UPDATE job SET stato='fatto'  |  'pronto' + non_prima_di (errore: si ritenta)  |  'fallito'
  applica il risultato (contenuto, proposte, frontiere, bozza…)
```

I tempi del protocollo stanno in **un posto per lato** e i test li confrontano: `protocollo.py` ↔
`internal/platform/contratti/worker/protocollo.go`, attraverso `contracts/tempi_protocollo.json`. Attesa del claim
20 s; tetto 25 s, ma un `attesa_s` minore o uguale a zero o sopra 25 il server non lo tronca: lo sostituisce con 20
(`workerapi.go:claim`). Online = contatto entro 40 s; battito mai più lento di 20 s. Prima erano tre numeri in tre
file, e insieme dicevano che un worker occupato era spento.

**Il predicato di validità del tentativo** è uno solo e vale per heartbeat, result (ok ed errore), ingest,
upload e download del contenuto:

```
stato = 'in_corso' AND lease_token = $token AND worker_id = $worker
AND lease_fino_a > now() AND now() <= avviato_il + durata_max_s
```

Zero righe → 409 e **nulla** viene applicato. Serve a questo: se il tentativo A perde il lease e B prende il
job, il risultato tardivo di A non può sovrascrivere il lavoro di B. Se il motivo è la durata massima, il job torna
subito `pronto` con l'errore scritto (`coda.go:ChiudiSeDurataSuperata`), invece di restare «in corso» fino alla
scadenza del lease.

Stati: `pronto → in_corso → fatto | fallito | annullato`. `tentativi + 1` si conta al **claim** (`job.sql:ClaimJob`), non al
fallimento. Un lease scaduto riporta il job `pronto` (scheduler `lease`, ogni 30 s, `job.sql:RilasciaLeaseScaduti`), o
`fallito` se i tentativi sono esauriti. Un result di errore non definitivo lo riporta `pronto` con
`non_prima_di = now() + min(600, 15·2^tentativi)` secondi; uno definitivo, o uno che arriva a tentativi esauriti, lo
chiude `fallito` (`job.sql:FallisciJob`). Il massimo dei tentativi è per tipo (`coda.go:MaxTentativiPer`): 5, e 50 per
`copia_nas` e `crea_cartella_thread`, che falliscono perché il NAS manca e non perché siano sbagliate; quelle
esaurite tornano in coda da sole al ritorno del NAS. I job interattivi hanno anche `scade_il`
(`coda.go:ScadenzaInterattiva`): 10 minuti per «Apri in Outlook» e «Segna letto», 60 per una bozza. Oltre, il claim
non li assegna più e lo scheduler li chiude `annullato`, anche se erano già in corso: così un "Apri" chiesto ieri non
si esegue a sorpresa.

Idempotenza: `AccodaCon(…, chiave, …)` non crea un secondo job se ne esiste già uno `pronto`/`in_corso`
con la stessa chiave (indice unico parziale). Le chiavi: `sync_outlook:<casella>`, `sync_storico:<casella>`,
`stage:<allegato>`, `estrai:<allegato>`, `analizza:<sha256>:<versione>:<config>`, `rileggi:<casella>:<entry>`,
`bozza:<bozza>`, `nas:<documento>` (copia sul NAS), `sposta:<documento>` (spostamento sul NAS),
`cartella:<thread>` (cartella della RFQ sul NAS), `analisi-ai:<messaggio>`. «Apri in Outlook» e «Segna letto»
non hanno chiave: due clic sono due job.

Priorità (si prende prima la più bassa): 1 «Apri in Outlook», bozze, download chiesti dall'operatore, cartella,
copia e spostamento sul NAS; 2 «Segna letto», download della preparazione del Fascicolo e della ripresa di un
contenuto sparito (e la riestrazione di un archivio nella ripresa); 3 download automatico all'ingest e rilettura
di un elemento; 4 estrazione di un archivio; 5 sync ordinario e analisi semantica; 6 analisi di un allegato;
9 «Carica precedenti» (`coda.go:PrioritaSyncStorico`).

**Capacità di scrittura** (`[sicurezza]` in `cockpit.toml`, blocco 4). Tre voci decidono che cosa il server
può modificare fuori da sé: `outlook_scrittura` (segna letto, sposta in cartella), `bozze` (crea bozza),
`nas_scrittura` (cartella, copia e spostamento sul NAS: lavori del server che il worker non vede).
`modalita = "shadow"` è il preset che le spegne tutte; il silenzio vale «spenta». Un job di un tipo bloccato non
viene accodato e, se era già in coda, **non viene consegnato al claim**: il worker non lo vede. Tutto il resto —
sync, stage, «Apri in Outlook» — legge e basta ed è sempre consentito.

## 3. L'avvio di un worker

1. `cockpit_client.py:carica_config` legge `worker.toml`: in cima `server_url`, `impronta` del certificato (se
   TLS) e `staging`; nella sezione del worker (`[outlook]` o `[analisi]`) `worker_id` e `token`, che vincono su
   quelli in cima. Le variabili d'ambiente `COCKPIT_URL`, `COCKPIT_TOKEN`, `COCKPIT_IMPRONTA`, `COCKPIT_STAGING` e
   `COCKPIT_WORKER_ID` vincono sul file (le usano i test E2E). Il token è **individuale**: viene generato dalla
   pagina *Postazioni* («genera e scarica il pacchetto», o «Rigenera credenziali della postazione») insieme al
   pacchetto (`worker.toml`, `ISTRUZIONI.txt`, `installa-postazione.ps1`, `requirements.txt`, `cert.pem` se il
   server è in HTTPS, e i file del worker), che sta fuori dal repository; un token vuoto ferma il worker con un
   messaggio che dice dove prenderlo. Senza `worker_id` il nome è `<tipo>@<NOME-HOST>`.
2. `controlla_staging()`: lo `staging` del worker è una cartella **sua** (file temporanei, `log`,
   `restrict.json`, `ultimo_arresto.txt`) e non deve stare dentro lo staging del server, in particolare mai dentro
   `_contenuti` o `_parti`. Un worker puntato lì dentro non dà errore e cancella i contenuti che i documenti
   confermati aspettano: è successo sul banco, e da allora è un rifiuto all'avvio. All'avvio entrambi i worker
   svuotano anche la propria `tmp\` (`svuota_tmp`): ciò che c'è dentro è di tentativi finiti.
3. Il worker Outlook chiede `GET /api/v1/worker/caselle?worker_id=…`: il server risponde con le caselle
   Outlook che quel worker è **autorizzato** a servire (`worker_credenziale.caselle`), non con quelle del profilo.
4. `outlook_com.py:risolvi_caselle` cerca ogni casella autorizzata nel profilo Outlook locale (account, store
   del profilo, o cartella condivisa) e ne annota lo `store_id` **locale**. Uno store del profilo che non è
   fra le caselle autorizzate viene ignorato (es. la casella di un collega presente nel profilo). La risoluzione
   si ripete ogni `risolvi_caselle_ogni_s` e quando la casella di un job non risulta risolta.
5. Da lì in poi ogni claim dichiara `postazione`, `outlook_ok` e `caselle_aperte` (`casella_id` +
   `store_id`), più `ultimo_arresto` se c'è: il server interseca con le autorizzazioni e registra lo store in
   `casella_store` per quella postazione. È il server a scegliere il job, il worker a eseguirlo.

Il worker analisi salta i punti 3–5: al claim dichiara solo `postazione` (il nome host) e, se c'è,
`ultimo_arresto`; ha `worker_id`, `token` e una cartella `staging` sua, dove scarica il contenuto da analizzare
(dal 7C.1 non legge più lo staging del server: vedi §6).

Le altre chiavi di `worker.toml`:

| Chiave | Default | Chi la legge |
|---|---|---|
| `arresto_forzato_s` | 15 | entrambi: i secondi concessi al lavoro per fermarsi dopo un 409, prima dell'uscita forzata (§7) |
| `postazione` | nome host, in maiuscolo | solo il worker Outlook; il worker analisi dichiara sempre il nome host |
| `risolvi_caselle_ogni_s` | 300 | Outlook |
| `usa_restrict` | `true` | Outlook: `false` = solo scansione lineare (§5.1) |
| `autoprova_restrict_ore` | 24 (o `autoprova_restrict_giorni` × 24, chiave storica) | Outlook |
| `autoprova_restrict_valida_ore` | 168 | Outlook: validità dell'esito ricordato in `restrict.json` |
| `cartelle` | Posta in arrivo / Inbox, Posta inviata / Sent Items | solo `--restrict` |
| `consenti_invio` | `false` | Outlook: `Send()` solo se è `true` e il payload lo chiede; il server manda sempre `invia = false` |

**Avvio automatico (7C.1).** Il pacchetto della postazione contiene `installa-postazione.ps1`: ferma ciò che
gira, installa le dipendenze, crea lo `staging` del worker, importa `cert.pem` fra le autorità radice
dell'utente e registra un'attività pianificata per ogni sezione di `worker.toml` (`[outlook]`, `[analisi]`),
all'accesso dell'utente, istanza singola (`-MultipleInstances IgnoreNew`), riavvio automatico, `pythonw` senza
finestra (sotto `pythonw` `configura_log` non apre la console: il log è solo su file). `-Ferma` prima di rigenerare il
pacchetto, `-Mostra` per lo stato, `-Disinstalla` per togliere tutto; `-SenzaCertificato` non importa `cert.pem`,
`-SenzaDipendenze` salta l'installazione delle dipendenze, `-Prefisso` cambia il prefisso dei nomi delle attività
(default `Cockpit`: «Cockpit - worker Outlook», «Cockpit - worker analisi»).

## 4. Gli endpoint (tutti sotto `/api/v1`, tutti autenticati con `X-Cockpit-Token`)

Un token mancante, sconosciuto, condiviso da due worker o mai generato riceve 401 su qualunque rotta. Il
`worker_id` dichiarato nel corpo o nella query, se c'è, deve essere quello della credenziale: altrimenti 403.

| Metodo e percorso | Chi | Quando | Corpo → Risposta |
|---|---|---|---|
| `POST /jobs/claim` | entrambi | in ciclo, appena liberi | `ClaimRichiesta` → 200 `Job` / 204 dopo l'attesa senza job / 400 corpo, `worker` o `worker_id` non validi / 403 tipo o postazione diversi da quelli della credenziale |
| `GET /worker/caselle` | outlook | all'avvio, poi ogni `risolvi_caselle_ogni_s` e quando una casella non è risolta | `?worker_id=` → caselle autorizzate con canale Outlook |
| `POST /jobs/{id}/heartbeat` | entrambi | dal thread `Battito` | `{worker_id, lease_token}` → 204 / 400 senza tentativo / 403 / 409 |
| `POST /jobs/{id}/result` | entrambi | fine del job | `RisultatoRichiesta` → 204 / 400 senza tentativo / 403 / 404 job inesistente / 409 / 422 risultato non applicabile: il job fallisce in modo definitivo, tranne l'hash di un download che non torna, che si ritenta |
| `POST /ingest/messaggi` | outlook | durante `sync_outlook`, un lotto per volta | `IngestRichiesta` → `IngestRisposta{inseriti, aggiornati, falliti, esiti[], durata_ms}` / 400 / 403 casella non autorizzata per la credenziale, o lotto che il job del tentativo non autorizza / 409 / 422 casella non censita o disattivata (il server chiude il job in modo definitivo) / 5xx niente è stato scritto |
| `PUT /allegati/{id}/file` | outlook | durante `stage_allegato` | binario + `?job_id&lease_token&worker_id` → 204 / 400 / 403 / 409 / 413 oltre `max_upload_mb` / 422 il job non è il download di questo allegato / 500 trasferimento interrotto |
| `GET /allegati/{id}/contenuto` | analisi | durante `analizza_allegato` (7C.1) | `?job_id&lease_token&worker_id` → binario con `X-Cockpit-Sha256` / 400 / 403 / 409 tentativo non valido / 410 contenuto non in staging, sparito dalla cache o con un percorso fuori dalla cartella di staging del server (Riscarica) / 422 il job non è l'analisi di questo allegato |
| `GET /sync/cursori` | (nessun worker) | diagnosi a mano; la prova W11 la usa come chiamata qualunque | `[{cartella, ultimo_received, coperto_fino_a}]` per ogni coppia (casella, cartella), ordinate per indirizzo, ma **senza dire di quale casella**: con due caselle, le due «Inbox» non si distinguono. Il worker non lo chiama: la finestra arriva già nel payload |

L'ingest verifica la casella prima di scrivere. Un lotto senza `casella_id` usa quella del suo job
(`ingest.go:CasellaDelJob`); `[outlook].casella_default` resta il ripiego solo per un job che non ne nomina nessuna.
La credenziale deve essere autorizzata su quella casella (403, e il job non fallisce: è il lotto a non essere suo), e
il job del tentativo deve essere un `sync_outlook` o un `rileggi_elemento` della stessa casella
(`ingest.ErrLottoNonDelJob` → 403): il lease prova il tentativo, non il diritto di scrivere la posta di un collega.

Ogni chiamata autenticata è anche una prova di vita: `ultimo_contatto` si scrive nel wrapper `auth`
(`workerapi.go:contatto`), prima dell'handler, così un claim appeso in long-poll o un job lungo non fanno sparire il
worker dalla testata. È un UPDATE e non crea la riga. La riga la crea, o la riscrive, il claim accettato, all'ingresso
e prima dell'attesa: tipo, postazione della credenziale, IP, caselle intersecate, avviso (`DichiarazioneWorker`); alla
fine dell'attesa `ClaimConcluso` scrive `ultimo_claim`.

Il contratto dei corpi è in `contratti.py` (pydantic) e in `internal/platform/contratti/worker/tipi.go`; `genera_contratti.py`
produce gli schemi JSON in `contracts/` e i test dei due lati li confrontano (L3).

## 5. Le funzioni del worker Outlook

`worker_outlook.py:dispatch` sceglie per `job.tipo`. Per ogni tipo: chi crea il job, con quale payload,
cosa fa il worker, cosa riporta, cosa cambia nel database quando il server applica il risultato.

| Tipo | Chi lo crea | Payload | Cosa fa il worker | Risultato → effetto |
|---|---|---|---|---|
| `sync_outlook` | scheduler (`intervallo_sync_s` > 0), «Aggiorna ora», prima apertura dell'Inbox nella sessione, «Carica precedenti» | `PayloadSyncOutlook{casella_id, modo, cartelle[]{cartella, dal, al, bootstrap, ultimo_received, coperto_fino_a}, dal, al?, sovrapposizione_s, lotto}` | per ogni cartella legge la finestra `[dal, al]` **decisa dal server**, dal più recente al più vecchio, con `Restrict` (self-test alla prima lettura); manda lotti a `/ingest/messaggi`; dichiara `completa` solo se l'enumerazione è finita da sola | `RisultatoSync{cartelle[]{cartella, n_messaggi, saltati, ultimo_received, completa, errore}, tempi}` → `ultimo_received` si scrive comunque; la frontiera (`coperto_fino_a` o `storico_fino_a`) avanza solo se `completa` |
| `stage_allegato` | «Scarica», «Riscarica» e gli allegati spuntati nel form di triage (priorità 1); la preparazione del Fascicolo (B8.7b) e la ripresa di una copia sul NAS che trova il contenuto sparito (priorità 2); lo staging automatico all'ingest (D30, priorità 3) | `PayloadStageAllegato{allegato_id, entry_id, indice, nome_file, cartella, messaggio_id, casella_id, message_id}` | apre l'elemento nello store locale (fallback per Message-ID), `SaveAsFile` in `tmp\`, sha256, `PUT` al server | `RisultatoStage{allegato_id, sha256, bytes, entry_id?, cartella?}` → il server verifica sha256 e byte del `.parte` di questo tentativo (se non tornano: 422, si ritenta), lo promuove da `_parti/<allegato>.parte.<token>` in `_contenuti/<ab>/<sha256>.<ext>` (o lo butta, se quel contenuto c'era già), `allegato.stato='in_staging'`, riallinea l'EntryID se il worker ha trovato l'elemento altrove. Poi `workerapi.go:dopoStaging`: prima la proposta dal nome del file e la regola del rumore (hash già scartato per quel dominio, o immagine vista almeno 3 volte dallo stesso dominio: niente analisi); poi `estrai_archivio` (server) solo per uno `.zip`; altrimenti `analizza_allegato`, che non si accoda se i fatti di quel contenuto ci sono già: allora il server li applica subito (`workerapi.go:applicaFattiEsistenti`) |
| `apri_elemento_outlook` | «Apri in Outlook» | `{casella_id, entry_id, message_id, messaggio_id}` | `Display()` sull'elemento | `RisultatoElemento{entry_id, cartella}` → se l'elemento è stato trovato con un EntryID diverso, il server riallinea `messaggio_casella.entry_id` (e `cartella`) di quella copia |
| `crea_bozza_outlook` | «Rispondi» (capacità `bozze`). Il tipo `nuovo` (destinatari = contatti del fornitore, oggetto `RFQ <cliente> <buyer> <codici>`) lo accoda ancora la rotta `POST /thread/{id}/richiesta` (`richieste.go:nuovaRichiestaFornitore`), ma nessuna pagina la chiama più: il box «Nuova richiesta a un fornitore» è stato tolto dalla pagina della RFQ prima di B8.2 | `PayloadCreaBozza{bozza_id, tipo, entry_id?, destinatari[], oggetto, corpo_html, corpo_testo, allegati[], mostra, invia, marcatori{}}` | `Reply()`/`ReplyAll()`/`Forward()` o `CreateItem` per `nuovo`, destinatari, oggetto, `Display()` prima del corpo (così Outlook mette la firma), testo sopra la firma, poi i **marcatori**: `UserProperties["CockpitBozza"]=bozza_id` sempre, più quelli del payload che iniziano per `Cockpit` (`CockpitRichiestaFornitore`=richiesta_id); `Save()`. `Send()` solo con `invia` e `consenti_invio`, e il server manda sempre `invia = false`. Un marcatore che non si riesce a scrivere non ferma la bozza (log) | `RisultatoBozza{entry_id, inviata}` → `bozza.stato='aperta'` |
| `segna_letto` | «Segna letto» (capacità `outlook_scrittura`) | `{casella_id, entry_id, letto, message_id, messaggio_id}` | `UnRead = not letto`, `Save()` | `messaggio_casella.non_letto` lo scrive la rotta della UI quando accoda il job (`routes_inbox.go:segnaLetto`), per la copia su cui si agisce; il risultato (`RisultatoElemento`) riallinea soltanto l'EntryID |
| `sposta_in_cartella` | (previsto, nessuna rotta lo accoda oggi; capacità `outlook_scrittura`) | `PayloadSpostaCartella` | `Move()` | `RisultatoElemento{entry_id, cartella}` → riallinea `entry_id` e `cartella` di `messaggio_casella` se l'EntryID è cambiato. Il contratto `RisultatoSposta` esiste ma il worker non lo usa |

**I marcatori tornano con il sync (7B, IB2).** `_converti` legge le `UserProperties` dell'elemento che iniziano
per `Cockpit` (`marcatori_di`) e le manda in `MessaggioIn.marcatori`; qualunque anomalia COM su quelle proprietà
vale «nessun marcatore», non un elemento saltato. Quando la mail di una richiesta compare nella Posta inviata, il
server lega la mail alla richiesta e alla RFQ dal marcatore, senza guardare l'oggetto. I marcatori valgono solo
sulla posta in uscita: su una mail in entrata (la risposta di un fornitore che si porta dietro le proprietà della
nostra) il server li ignora e lo scrive nel log (`marcatori.go:applicaMarcatori`). Un worker più vecchio non manda
`marcatori`: il server tratta l'assenza come «nessuno» e la richiesta si conferma a mano dall'Inbox.

**`rileggi_elemento` non c'è.** Il server lo accoda da «Riprova» su uno scarto di tipo `lettura`
(`ingest/replay.go`, chiave `rileggi:<casella>:<entry>`, worker `outlook`), ma `dispatch()` non ha quel ramo:
il worker lo chiude come errore definitivo («tipo job sconosciuto»). Non è mai stato implementato lato worker.
Difetto noto, da correggere con un incarico; uno scarto `lettura` oggi si recupera solo con un sync che riprenda
quella finestra.

I job interattivi (`apri`, `bozza`, `letto`) portano `postazione_id` = la postazione dell'operatore che ha
cliccato: li prende solo il worker di quel PC. `sync` e `stage` portano solo `casella_id`: li prende
qualunque worker autorizzato a quella casella. Il payload porta sempre `casella_id` + `entry_id` + Message-ID,
mai uno `store_id`: lo store è del profilo locale e lo risolve il worker.

### 5.1 Il sync in dettaglio

Tre modi, **un solo algoritmo** (blocco 3 del 3R). I modi decidono soltanto la prima finestra; da lì in poi
tutti leggono un intervallo chiuso `[dal, al]`, fissato dal server all'accodamento e diverso per ogni cartella,
dal più recente al più vecchio, e scrivono la propria frontiera solo quando la finestra è stata percorsa per
intero.

| Modo | Quando | Finestra per cartella | Frontiera che avanza (solo se `completa`) |
|---|---|---|---|
| **aggiornamento** | scheduler, «Aggiorna ora», apertura dell'Inbox; la cartella ha già una copertura | `[coperto_fino_a − 10 min, adesso]` | `coperto_fino_a` → `al` |
| **bootstrap** | la cartella non ha ancora nessuna copertura (o ne ha una nel futuro, che si ignora) | `[adesso − giorni_sync_iniziale, adesso]` (7 se assente; `[outlook].dal` è l'override) | `coperto_fino_a` → `al` (e nasce `storico_fino_a`) |
| **storico** | «Carica precedenti» | `[storico_fino_a − 2 giorni, storico_fino_a]`; senza uno storico riuscito, dalla mail più vecchia che **quella cartella** ha (o da adesso, se non ne ha) | `storico_fino_a` → `dal` |

```
        PASSATO                                                          PRESENTE
             storico_fino_a  [ quello che il Cockpit ha ]  coperto_fino_a
                    <- «Carica precedenti»            «Aggiorna» ->
```

Perché la frontiera non avanza per lotto: leggendo dal più recente al più vecchio, il primo lotto contiene già
la mail più nuova; se la frontiera saltasse lì e il worker morisse subito dopo, tutto ciò che sta sotto
resterebbe invisibile per sempre. Il server sposta la frontiera solo sulla dichiarazione esplicita
`completa = True` della cartella, non «se il job non è esploso». `ultimo_received` (la mail più recente che
abbiamo) viaggia invece con ogni lotto, nella stessa transazione dell'ingest: è un fatto sui messaggi
consegnati, non una promessa su ciò che è stato guardato, e nello storico non si tocca.

Per ogni lotto: `IngestRichiesta{casella_id, messaggi[], …}` → il server apre **una transazione per lotto**
con un `SAVEPOINT` per elemento; un elemento rifiutato va in `ingest_scarto` (origine `ingest`, con il
payload intero) senza fermare gli altri; il commit rende durevoli messaggi, scarti e `ultimo_received` insieme.
Su un 5xx il server non ha scritto niente e ripetere il lotto identico sarebbe sicuro (l'ingest è idempotente per
Message-ID), ma il worker non lo ripete: qualunque errore dell'ingest diverso da 409 esce dal sync, il job fallisce
in modo non definitivo e il tentativo successivo rilegge la finestra (§7). Un elemento che il worker
non riesce a **convertire** (COM che non risponde, oggetto senza `ReceivedTime`) va in `saltati` e diventa
uno scarto `lettura`. Una cartella che non si riesce a leggere per intero resta un problema di quella cartella:
le altre della stessa casella si leggono lo stesso, e quella si rilegge al giro dopo. Un errore dell'ingest invece
ferma tutto il job, cartelle successive comprese.

**Gli allegati scendono da soli?** Lo decide il server dal modo del **job** (non dal lotto): `aggiornamento`
sì, se `[staging].automatico`; `bootstrap` solo con `[staging].bootstrap = true`; `storico` mai: «Carica
precedenti» rende consultabile la posta vecchia, non ne scarica gli allegati. Anche quando il modo lo consente,
scende solo un file che la proposta dal nome pre-spunta, sotto `[staging].max_mb`, di un messaggio visto per la
prima volta, di un cliente riconosciuto, in entrata o interno.

`Restrict`: alla prima lettura di ogni cartella il worker confronta l'insieme degli EntryID restituiti dal
filtro DASL (in UTC) con quello della scansione lineare sulla stessa finestra; se differiscono il filtro
viene spento per quella cartella. `python worker_outlook.py --restrict 7` fa la stessa misura a mano e
stampa i tempi.

Ore: `ReceivedTime` di pywin32 arriva con `tzinfo` UTC ma **numeri locali**; `_utc()` lo tratta come ora
locale del PC e converte. Il server rifiuta un `ricevuto_il` più avanti di `now() + tolleranza` (scarto,
cursore fermo).

## 6. Il worker analisi

`worker_analisi.py` esegue un solo tipo, `analizza_allegato` (priorità 6), chiave
`analizza:<sha256>:<versione>:<hash_config>`: un solo job per contenuto, anche se lo stesso file arriva da tre
messaggi. Lo accoda il server (`stage.go:AccodaAnalisi`) quando un contenuto entra in `_contenuti`, per ogni voce
che l'estrazione di un archivio tira fuori, nella preparazione del Fascicolo per i file fermi in staging (B8.7b) e
nella rianalisi degli STEP (all'apertura della RFQ, pochi per volta, e con «Rianalizza»). Se i fatti di quella chiave
ci sono già non si accoda niente.

| | |
|---|---|
| Dove gira | su qualunque postazione (7C.1, P0). Il payload non porta percorsi del server: il worker scarica i byte con `GET /allegati/{id}/contenuto` dentro il proprio tentativo, li scrive in `<staging del worker>\tmp\<job>\`, verifica lo sha256 e i byte del payload, analizza e cancella. Fino al banco a due macchine del 20/09/2026 portava `path_staging`, il percorso sul disco del server, e il worker sull'altro PC falliva con «file non trovato in staging» |
| Payload | `PayloadAnalizzaAllegato{allegato_id, sha256, bytes, nome_file, thread_id?, messaggio_id, versione_analizzatore, hash_configurazione, parametri}` — i `parametri` (dizionari dei termini) li manda il server dalla `[analisi]` di `cockpit.toml`. Un `path_staging` in un job vecchio ancora in coda viene ignorato |
| Cosa fa | `analizza_file()`: PDF con PyMuPDF (testo, termini di cartiglio, righe che sembrano codici); STEP con `step_struttura.py` (tokenizer Part 21, nessuna dipendenza nuova): il grafo `PRODUCT` → `NEXT_ASSEMBLY_USAGE_OCCURRENCE` finisce in `dettagli.struttura` come nodi, relazioni e quantita'. **Gli archivi non li vede**: li scompatta il server (`estrai_archivio`) e ogni voce diventa un allegato figlio con il proprio job di analisi |
| Risultato | `RisultatoAnalisi{allegato_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli, versione_analizzatore, hash_configurazione}` → il server risponde 422, e il job fallisce in modo definitivo, a un risultato per un allegato diverso da quello del job, con una versione o una configurazione diverse da quelle chieste, o con `tipo_proposto`/`fonte` fuori enum. Altrimenti scrive `analisi_fatti(sha256, versione, hash_config)` (con la lettura del worker accanto ai dettagli, per riusarla), una **proposta** (`documento_proposta`) per l'allegato del job e per ogni altro allegato con una proposta aperta sullo stesso sha256, ciascuna con la direzione del suo messaggio e le regole del suo cliente, e le proposte di **struttura** in ogni RFQ che contiene quel contenuto (`workerapi.go:strutturaNelleRfq`), anche dove il documento è già confermato |

Il Python riporta ciò che ha letto nel file; la proposta scritta in database la compone il Go
(`workerapi.go:propostaDaAnalisi`), una per allegato. Codice e revisione passano da `regole.go:Canonico` (il
motore del cliente toglie i suffissi decorativi), da `workerapi.go:codiceRevSicuri` (un valore fuori misura non entra
nella colonna: resta nei dettagli come `codice_scartato`/`rev_scartata`) e da `modifiche.go:RevisioneProponibile`
(un file caricato a mano
non propone una revisione: quella letta resta nei dettagli come `rev_letta`). Il worker oggi **ignora `parametri`** e
usa i propri dizionari: i parametri viaggiano ma non vengono letti, e l'allineamento resta da fare.

**La struttura non porta codici** (blocco 8, A1.2). Ogni nodo porta gli attributi grezzi dell'entità —
`id_grezzo`, `nome_grezzo`, `descrizione_grezza`, `rev_grezza` — e l'evidenza di quale riga del file li ha
prodotti. Decidere se `1234567A_B` è un codice, e quale ne sia la revisione, dipende dalle famiglie del
CLIENTE di quella richiesta: il worker non sa nemmeno di che cliente si tratti, e quella lettura la fa il
server con le regole già scritte per oggetto, corpo e nomi dei file. `struttura.versione` lo dice: la 1 (mai
entrata in produzione) metteva i codici nei nodi, la 2 porta i grezzi, la **3** (B8.5, addendum A4.4, D35)
aggiunge `scarti`, quattro interi: `prodotti_senza_definizione`, `occorrenze_non_risolte`,
`occorrenze_su_se_stesse`, `testi_troncati`. Il server decide dai numeri, non dalle frasi di `avvisi`, se una
lettura è completa (la funzione SQL `struttura_motivo_parziale` della 0020): solo una lettura completa dello STEP
strutturale di un prodotto può proporre quantità diverse e rimozioni. Le occorrenze di un pezzo dentro se
stesso si contano ma non tolgono completezza: sono archi impossibili. Gli `avvisi` restano, per la schermata.

I campi `codice`/`rev` di primo livello del risultato restano quelli di sempre — un'ipotesi dal nome del file
o dal primo `PRODUCT` — e servono a `documento_proposta`. Sono un suggerimento, non la sorgente della
struttura. `[analisi] versione = 3` in `cockpit.toml` (come nell'esempio; senza la chiave la versione è 1) è la
chiave sotto cui i fatti nuovi vengono conservati: quelli delle versioni precedenti restano dove sono, e gli STEP si
rianalizzano quando si riapre la loro RFQ o con «Rianalizza». Un worker non aggiornato che risponde con una
struttura v2 non rompe niente: il server la tiene, e la considera incompleta.

**Tre tetti, e viaggiano con il risultato.** `nodi_max` (2000) ferma l'assieme con troppi pezzi distinti,
`occorrenze_max` (50000) quello con pochi pezzi ripetuti moltissime volte — mille bulloni uguali sono mille
`NEXT_ASSEMBLY_USAGE_OCCURRENCE` e un nodo solo — e `tempo_max_s` (120) il file lento per una ragione non
prevista. Il tempo si controlla anche mentre si scorre la geometria, dove per minuti non si incontra nessuna
delle due cose che si contano. `limiti` torna indietro con i tetti in vigore, i byte letti, il tempo
impiegato, `troncato` e `motivo` (`nodi`, `occorrenze` o `tempo`): un albero parziale va spiegato con il
limite di quel giorno, e la configurazione di oggi fra un anno non dirà più quale fosse.

**Come si guarda un corpus vero.** `python workers/diagnostica_step.py [cartella] [--albero]` attraversa dei
file STEP e stampa, per ciascuno, schema, nodi, relazioni, occorrenze, radici, profondità, nodi con più di un
padre, gli scarti in numeri (`PRODUCT` orfani, occorrenze irrisolte, anelli, testi troncati), archi che tornano indietro, nodi che nessuna radice raggiunge,
troncamento e tempo; con `--albero` disegna l'albero con
`id_grezzo | nome_grezzo | rev_grezza`. Legge e basta: non scrive niente e non tocca il database. I CAD non
stanno nel repository — la cartella predefinita è `docs/step_files`, che è fuori.

**Scostamento registrato: gli spazi ai bordi dei campi grezzi.** A1.2 dice che il worker consegna gli attributi
GREZZI e non interpreta. `_testo()` fa una cosa in piu': dopo aver decodificato le sequenze di escape toglie gli
spazi ai BORDI del valore. Dentro la stringa non tocca niente, e non tocca nessun altro campo. La ragione sta nel
corpus vero: un esportatore scrive la revisione assente come `' '` e un altro come `''`, e senza questo `strip()`
`rev_grezza` sarebbe «vuota» in un file e «uno spazio» in un altro — due valori diversi per lo stesso fatto, e
ogni lettore piu' avanti dovrebbe sapere che sono uguali. E' una NORMALIZZAZIONE e non una classificazione,
quindi non intacca la regola di A1.2 sui codici; ma e' comunque un valore che non e' piu' letteralmente quello
scritto nel file, ed e' registrato qui perche' chi decide su A1.2 lo veda e possa dire di no. Il posto in cui si
toglie e' uno solo — `_testo()` in `step_struttura.py` — e la prova che lo fissa e'
`test_una_revisione_fatta_di_spazi_e_una_revisione_assente`.

## 7. Errori e ripresa

**Il ciclo del claim.** `cockpit_client.py:diagnosi` classifica un errore in una di sette categorie e dice quanto
aspettare: **rete** (connessione non riuscita), **server** (5xx) e **tentativo** (409) usano il backoff del ciclo,
5, 10, 20, 30 s; **credenziale** (401) e **autorizzazione** (403) si riprovano ogni 30 s senza backoff, perché chi
corregge il file deve trovare il worker vivo; **richiesta** (gli altri 4xx) ogni 30 s, perché ripeterla identica non
cambia l'esito; **certificato** (impronta sbagliata) ogni 60 s. Le ultime quattro sono «gravi»: vanno nel log come
errore e, con `--una-volta`, il worker esce invece di riprovare. Il ciclo è lo stesso nei due worker
(`esegui_per_sempre`).

**Dentro un job** ogni chiamata ha la sua regola:

| Chiamata | 409 | Altri 4xx | 5xx e rete |
|---|---|---|---|
| heartbeat (`Battito`) | flag di arresto, poi uscita forzata | log, il battito continua | il battito continua |
| ingest (`worker_outlook.py:_invia`) | arresto, nessun result | l'eccezione esce dal sync: result di errore NON definitivo | come gli altri 4xx |
| upload (`worker_outlook.py:_carica`) | arresto, nessun result | 413 e gli altri 4xx: errore definitivo | errore non definitivo, si ritenta |
| download del contenuto (`worker_analisi.py:analizza`) | arresto, nessun result | 404, 410, 422: errore definitivo «usa Riscarica»; 400 e 403: errore non definitivo | errore non definitivo; così anche uno sha256 o dei byte che non tornano |
| result (`cockpit_client.py:riporta_risultato`) | tace: il job è chiuso o è di un altro tentativo | log di errore, non si ripete | la rete si riprova fino a tre volte, 2 s fra l'una e l'altra; un 5xx non si ripete |

Il resto del dispatch nel worker Outlook: `ErroreDefinitivo` (elemento sparito, allegato non salvabile, sync senza
casella, tipo di job sconosciuto) chiude il job in modo definitivo; una casella non risolta nel profilo
(`ErroreStoreLocale`) e un errore COM no, e dopo un errore COM Outlook si riapre al job successivo; qualunque altra
eccezione è un errore non definitivo. Nel worker analisi ogni eccezione è non definitiva, tranne i 404/410/422 del
download.

**Limite noto: il 403 sull'ingest.** Il server risponde 403 a un lotto di una casella che la credenziale non
autorizza, o che il job del tentativo non autorizza (`ingest.ErrLottoNonDelJob`), e ripeterlo darebbe lo stesso
rifiuto. Il worker non lo distingue: per lui è un errore generico, il job fallisce in modo non definitivo e si ritenta
fino a esaurire i tentativi. Il 422 di una casella non censita finisce allo stesso modo, ma lì il server ha già chiuso
il job in modo definitivo, e il result del worker riceve 409 e tace.

Il thread `Battito`: gira accanto al lavoro in **tutti e due** i worker, manda heartbeat ogni
`min(max(5, lease_s/4), 20)` secondi, e quando riceve 409 alza il flag di arresto. Il lavoro che può fermarsi
lo fa ai punti di ripresa (fra un elemento e l'altro nel worker Outlook; fra il download e l'analisi in quello
analisi). Il lavoro bloccato dentro COM — o dentro la lettura di un PDF che non ritorna — non può: dopo
`arresto_forzato_s` (15 s) il processo esce con `os._exit(3)` e l'attività pianificata lo riavvia; il job, lato server,
è già tornato `pronto` e sarà ripreso da un tentativo con token nuovo. Il motivo dell'uscita resta in
`ultimo_arresto.txt` nello staging del worker: al riavvio il primo claim lo riporta e il server lo conserva in
`worker_presenza.ultimo_arresto`.

Un solo worker Outlook per PC: lo garantiscono l'attività pianificata (istanza singola) e `installa-postazione.ps1`,
che ferma i processi worker prima di ripartire; il worker in sé non ha un mutex, e uno avviato a mano accanto
all'attività non viene fermato. Outlook classico aperto nella sessione dell'utente (COM non gira senza sessione
interattiva).

**Result dopo un buco di rete (7C.1).** `cockpit_client.py:riporta_risultato` ripete il POST del result fino a
tre volte se la RETE cade (al banco del 20/09/2026 un `RemoteDisconnected` ha fatto rifare il lavoro da capo
dopo la scadenza del lease). Se il primo era arrivato, il secondo riceve 409 — il job è chiuso — e il worker
tace, come per un lease perso: il server non applica due volte niente (upload e result ripetuti dallo stesso
tentativo lasciano un contenuto, un job fatto, un tentativo). Un errore HTTP diverso da 409 non si ripete.

**Tempi del sync (7C.1).** `RisultatoSync.tempi` e una riga di log alla fine di ogni sync dicono dove è
passato il tempo: `com` (enumerazione e conversione in Outlook), `serializzazione`, `https` (chiamate di
ingest andata e ritorno) e `server` (la parte dichiarata dal server in `IngestRisposta.durata_ms`); la
differenza fra `https` e `server` è la rete.

## 8. Comandi diagnostici

```
python worker_outlook.py --cartelle       # albero delle cartelle di ogni store del profilo, con conteggi
python worker_outlook.py --caselle        # caselle autorizzate → store locale risolto; la dichiarazione al claim
python worker_outlook.py --restrict 7     # Restrict vs scansione lineare sugli ultimi 7 giorni, con i tempi
python worker_outlook.py --una-volta      # un solo job ed esce (usato dai test E2E)
python worker_analisi.py --una-volta
python prova_lettura.py --cartella "Posta inviata" --giorni 7 [--casella indirizzo@azienda.example] [--vecchio-modo]
```

I due worker accettano `--config <file>` e `--debug`.

`prova_lettura.py` (L5) legge una cartella Outlook vera con il codice di oggi e non scrive niente: non parla con il
server, non apre il database, non segna, non sposta e non crea bozze. Stampa quanti messaggi ha letto, quanti
elementi ha saltato (i primi dieci con EntryID, cartella e motivo) e la mail più recente; esce con 0 se la cartella è
stata letta per intero, 1 se la lettura si è fermata, 2 se la casella non è nel profilo. Senza `--casella` legge lo
store predefinito del profilo; `--senza-restrict` usa solo la scansione lineare. `--vecchio-modo` rifà prima la
scansione com'era prima della correzione del confine COM: se quella cade e la nuova no, l'elemento anomalo c'è
davvero; se non cade nessuna delle due, la prova non ha dimostrato niente, e il comando lo dice.

## 9. Test

`workers/tests/test_*.py` (pytest, L2, con `conftest.py` e `finti_outlook.py`): ciclo del worker contro
`server_finto.py` (che rifiuta ciò che rifiuta il server vero), `Battito` e arresto forzato, presenza e battito,
credenziali, finestra e confine DASL in UTC, ora di Outlook, elementi saltati, marcatori, staging del worker (rifiuto
se dentro quello del server, `tmp\` svuotata), analisi su fixture, struttura STEP e `diagnostica_step`, contratti.

Lato Go, con il tag `integrazione` e il database di prova, il worker vero parla con il server vero: gli stessi
gestori HTTP di `cockpit.exe` su un server di prova, con la configurazione del worker passata dalle variabili
`COCKPIT_*`.

- `internal/transport/workerapi/e2e_worker_db_test.go` avvia `prova_e2e.py`, cioè `worker_outlook.py` con la sola
  classe `Outlook` sostituita (L4/E2E): il job si chiude, il battito rinnova il lease.
- `internal/transport/workerapi/contenuto_db_test.go` avvia `worker_analisi.py --una-volta` con lo staging del worker
  in un'altra cartella: i byte devono passare da `GET …/contenuto`, e uno STEP vero deve tornare con la sua struttura.
- `internal/transport/workerapi/rete_db_test.go` (W11) fa girare un frammento di `cockpit_client` contro un server
  TLS: passa solo con l'impronta giusta, e un'impronta senza https viene rifiutata.
- `cmd/cockpit/zip_browser_test.go` (L7, tag `integrazione browser`) compila `cockpit.exe`, lo avvia con
  `prova_e2e.py` in modo continuo (posta finta con uno ZIP) e con `worker_analisi.py`, e segue la richiesta nel
  browser fino al Fascicolo.

Le prove su Outlook e Exchange reali (L5, fra cui `prova_lettura.py`) vivono solo in `docs/esiti/esiti_reali.md`,
fuori dal repository.

Leggi anche: `internal/transport/workerapi/README.md` (le rotte), `internal/platform/coda/README.md` (la coda),
`internal/core/inbox/ingest/README.md` (l'ingest), `internal/platform/contratti/README.md` (i contratti).
