# I worker — come parlano con `cockpit.exe`

Documentazione dei due client Python (`worker_outlook.py`, `worker_analisi.py`) e dei moduli comuni
(`cockpit_client.py`, `contratti.py`, `protocollo.py`, `step_struttura.py`). Aggiornata al ramo `blocco-7` dopo il Pre-7 (commit
`1fc5584`): rispetto alla prima stesura (`0bdea9f`) sono cambiati il sync (finestra chiusa decisa dal server,
frontiere che avanzano solo a finestra percorsa), la sicurezza (capacità al posto della modalità shadow), lo
staging (cache per contenuto sul server, cartella propria del worker), gli archivi (li scompatta il server).
Quando il codice cambia, cambia questo file.

## 1. Il principio: il server non chiama mai nessuno

Il server Go non conosce l'indirizzo di nessuna postazione e non apre connessioni verso i PC. Sono i worker
a collegarsi, sempre in uscita, verso un solo indirizzo (`server_url` in `worker.toml`). Per questo:

- serve **una** regola di firewall, in ingresso sul server, e nessuna sui PC;
- un worker spento è semplicemente un worker che non chiama; il server lo capisce dal tempo passato
  dall'ultimo contatto (`worker_presenza.ultimo_contatto`, scritto a ogni chiamata autenticata, 0009), non da
  un ping e non dall'ultimo claim: un worker che sta lavorando da tre minuti è online;
- "chi fa cosa" è deciso dal server al momento del **claim**: il worker chiede *«c'è lavoro per me?»* e il
  server risponde con il primo job compatibile con il suo tipo, la sua postazione, le sue caselle e le
  capacità di scrittura accese.

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
                                                                          outlook_ok, caselle_aperte, attesa_s}
  prova di vita (auth: sono vivo)
  long-poll fino a attesa_s (20 s, max 25)
  UPDATE job SET stato='in_corso',
     lease_token=uuid, lease_fino_a=now()+lease_s,
     avviato_il=now()  … FOR UPDATE SKIP LOCKED
     (esclusi i tipi che le capacità bloccano)
                                            ──► {job_id, tipo, payload, lease_token, lease_s, durata_max_s}
                                                                            esegue (dispatch per tipo)
                                            ◄── POST /jobs/{id}/heartbeat   ogni min(max(5, lease_s/4), 20) s,
                                                                            da un THREAD SEPARATO
  predicato di validità (vedi sotto)         ──► 200  | 409 se il tentativo non vale più
                                            ◄── POST /api/v1/ingest/messaggi (solo sync, un lotto per volta)
                                            ◄── PUT  /api/v1/allegati/{id}/file (solo stage)
                                            ◄── POST /jobs/{id}/result  {esito, dati, errore, definitivo,
                                                                         worker_id, lease_token}
  UPDATE job SET stato='fatto' | 'fallito' (+ non_prima_di per il retry)
  applica il risultato (contenuto, proposte, frontiere, bozza…)
```

I tempi del protocollo stanno in **un posto per lato** e i test li confrontano: `protocollo.py` ↔
`internal/platform/contratti/worker/protocollo.go` (attesa del claim 20 s, massimo 25; online = contatto entro 40 s; battito mai più
lento di 20 s). Prima erano tre numeri in tre file, e insieme dicevano che un worker occupato era spento.

**Il predicato di validità del tentativo** è uno solo e vale per heartbeat, result (ok ed errore), ingest e
upload:

```
stato = 'in_corso' AND lease_token = $token AND worker_id = $worker
AND lease_fino_a > now() AND now() <= avviato_il + durata_max_s
```

Zero righe → 409 e **nulla** viene applicato. Serve a questo: se il tentativo A perde il lease e B prende il
job, il risultato tardivo di A non può sovrascrivere il lavoro di B.

Stati: `pronto → in_corso → fatto | fallito | annullato`, con `in_corso → pronto` ogni volta che un lease
scade (scheduler `lease`, ogni 30 s: `tentativi + 1`, dopo 5 → `fallito`). I job interattivi (`apri`,
`bozza`) hanno anche `scade_il`: oltre, `annullato`, così un "Apri" chiesto ieri non si esegue a sorpresa.

Idempotenza: `AccodaCon(…, chiave, …)` non crea un secondo job se ne esiste già uno `pronto`/`in_corso`
con la stessa chiave (`sync_outlook:<casella>`, `stage:<allegato>`, `estrai:<allegato>`,
`analizza:<sha256>:<versione>:<config>`, `rileggi:<casella>:<entry>`, `analisi-ai:<id>`).

**Capacità di scrittura** (`[sicurezza]` in `cockpit.toml`, blocco 4). Tre voci decidono che cosa il server
può modificare fuori da sé: `outlook_scrittura` (segna letto, sposta in cartella), `bozze` (crea bozza),
`nas_scrittura` (cartella e copia sul NAS, che il worker non vede). `modalita = "shadow"` è il preset che le
spegne tutte; il silenzio vale «spenta». Un job di un tipo bloccato non viene accodato e, se era già in coda,
**non viene consegnato al claim**: il worker non lo vede. Tutto il resto — sync, stage, «Apri in Outlook» —
legge e basta ed è sempre consentito.

## 3. L'avvio di un worker

1. `cockpit_client.carica_config()` legge `worker.toml`: `server_url`, `impronta` del certificato (se
   TLS), sezione `[outlook]` o `[analisi]` con `worker_id` e `token`, e `staging`. Il token è **individuale**:
   viene generato dalla pagina *Admin › Postazioni › Rigenera credenziali* insieme al pacchetto (`worker.toml`
   + `ISTRUZIONI.txt`, entrambi fuori dal repository); un token vuoto ferma il worker con un messaggio che dice
   dove prenderlo.
2. `controlla_staging()`: lo `staging` del worker è una cartella **sua** (file temporanei, `log`,
   `restrict.json`) e non deve stare dentro lo staging del server, in particolare mai dentro `_contenuti` o
   `_parti`. Un worker puntato lì dentro non dà errore e cancella i contenuti che i documenti confermati
   aspettano: è successo sul banco, e da allora è un rifiuto all'avvio. All'avvio il worker Outlook svuota
   anche la propria `tmp\` (`svuota_tmp`, Pre-7): ciò che c'è dentro è di tentativi finiti.
3. Il worker Outlook chiede `GET /api/v1/worker/caselle?worker_id=…`: il server risponde con le caselle
   che quel worker è **autorizzato** a servire (`worker_credenziale.caselle`), non con quelle del profilo.
4. `outlook_com.risolvi_caselle()` cerca ogni casella autorizzata nel profilo Outlook locale (account, store
   del profilo, o cartella condivisa) e ne annota lo `store_id` **locale**. Uno store del profilo che non è
   fra le caselle autorizzate viene ignorato (es. la casella di un collega presente nel profilo).
5. Da lì in poi ogni claim dichiara `postazione`, `outlook_ok` e `caselle_aperte` (`casella_id` +
   `store_id`): il server interseca con le autorizzazioni e registra lo store in `casella_store` per quella
   postazione. È il server a scegliere il job, il worker a eseguirlo.

Il worker analisi salta i punti 3–5: ha solo `worker_id`, `token` e una cartella `staging` sua, dove scarica
il contenuto da analizzare (dal 7C.1 non legge più lo staging del server: vedi §6).

**Avvio automatico (7C.1).** Il pacchetto della postazione contiene `installa-postazione.ps1`: ferma ciò che
gira, installa le dipendenze, crea lo `staging` del worker, importa `cert.pem` fra le autorità radice
dell'utente e registra un'attività pianificata per ogni sezione di `worker.toml` (`[outlook]`, `[analisi]`),
all'accesso dell'utente, istanza singola, riavvio automatico, `pythonw` senza finestra (sotto `pythonw`
`configura_log` non apre la console: il log è solo su file). `-Ferma` prima di rigenerare il pacchetto,
`-Mostra` per lo stato, `-Disinstalla` per togliere tutto.

## 4. Gli endpoint (tutti sotto `/api/v1`, tutti autenticati con `X-Cockpit-Token`)

| Metodo e percorso | Chi | Quando | Corpo → Risposta |
|---|---|---|---|
| `POST /jobs/claim` | entrambi | in ciclo, appena liberi | `ClaimRichiesta` → `Job` oppure 204 dopo `attesa_s` |
| `GET /worker/caselle` | outlook | all'avvio e ogni riavvio | `?worker_id=` → elenco caselle autorizzate |
| `POST /jobs/{id}/heartbeat` | entrambi | dal thread `Battito` | `{worker_id, lease_token}` → 200 / 409 |
| `POST /jobs/{id}/result` | entrambi | fine del job | `RisultatoRichiesta` → 200 / 409 / 422 |
| `POST /ingest/messaggi` | outlook | durante `sync_outlook`, un lotto per volta | `IngestRichiesta` → `IngestRisposta` (inseriti, aggiornati, falliti, cursore) |
| `PUT /allegati/{id}/file` | outlook | durante `stage_allegato` | binario + `?job_id&lease_token&worker_id&sha256` → 200 / 409 / 413 / 422 |
| `GET /allegati/{id}/contenuto` | analisi | durante `analizza_allegato` (7C.1) | `?job_id&lease_token&worker_id` → binario con `X-Cockpit-Sha256` / 409 tentativo non valido / 410 contenuto sparito dalla cache (Riscarica) / 422 il job non è l'analisi di questo allegato |
| `GET /sync/cursori` | (nessuno, oggi) | diagnosi a mano | i cursori (casella, cartella) del server: `ultimo_received`, `coperto_fino_a`. Il worker non lo chiama più: la finestra arriva già nel payload |

Ogni chiamata autenticata è anche una prova di vita: la presenza si scrive nel wrapper `auth`, prima
dell'handler, così un claim appeso in long-poll o un job lungo non fanno sparire il worker dalla testata.

Il contratto dei corpi è in `contratti.py` (pydantic) e in `internal/platform/contratti/worker/tipi.go`; `genera_contratti.py`
produce gli schemi JSON in `contracts/` e i test dei due lati li confrontano (L3).

## 5. Le funzioni del worker Outlook

`worker_outlook.py:dispatch()` sceglie per `job.tipo`. Per ogni tipo: chi crea il job, con quale payload,
cosa fa il worker, cosa riporta, cosa cambia nel database quando il server applica il risultato.

| Tipo | Chi lo crea | Payload | Cosa fa il worker | Risultato → effetto |
|---|---|---|---|---|
| `sync_outlook` | scheduler (`intervallo_sync_s` > 0), «Aggiorna ora», prima apertura dell'Inbox nella sessione, «Carica precedenti» | `PayloadSyncOutlook{casella_id, modo, cartelle[]{cartella, dal, al, bootstrap, ultimo_received, coperto_fino_a}, dal, al?, sovrapposizione_s, lotto}` | per ogni cartella legge la finestra `[dal, al]` **decisa dal server**, dal più recente al più vecchio, con `Restrict` (self-test alla prima lettura); manda lotti a `/ingest/messaggi`; dichiara `completa` solo se l'enumerazione è finita da sola | `RisultatoSync{cartelle[]{cartella, n_messaggi, saltati, ultimo_received, completa, errore}}` → `ultimo_received` si scrive comunque; la frontiera (`coperto_fino_a` o `storico_fino_a`) avanza solo se `completa` |
| `stage_allegato` | «Scarica», «Riscarica», staging automatico all'ingest (D30), ripresa automatica di una copia sul NAS che trova il contenuto sparito (Pre-7) | `PayloadStageAllegato{allegato_id, entry_id, indice, nome_file, cartella, messaggio_id, casella_id, message_id}` | apre l'elemento nello store locale (fallback per Message-ID), `SaveAsFile` in `tmp\`, sha256, `PUT` al server | `RisultatoStage{allegato_id, sha256, bytes}` → il server promuove `_parti/<allegato>.parte.<token>` in `_contenuti/<ab>/<sha256>.<ext>`, `allegato.stato='in_staging'`; poi accoda `estrai_archivio` (server) se è un archivio, altrimenti `analizza_allegato` |
| `apri_elemento_outlook` | «Apri in Outlook» | `{casella_id, entry_id, message_id}` | `Display()` sull'elemento | nessun effetto nel DB |
| `crea_bozza_outlook` | «Rispondi» (capacità `bozze`); dal 7B anche «Nuova richiesta a un fornitore» dalla RFQ (tipo `nuovo`, destinatari = contatti del fornitore, oggetto `RFQ <cliente> <buyer> <codici>`) | `PayloadCreaBozza{bozza_id, tipo, entry_id?, destinatari[], oggetto, corpo_html, corpo_testo, allegati[], mostra, invia, marcatori{}}` | `Reply()`/`ReplyAll()`/`Forward()` o `CreateItem` per `nuovo`, destinatari, oggetto, `Display()` prima del corpo (così Outlook mette la firma), testo sopra la firma, poi i **marcatori**: `UserProperties["CockpitBozza"]=bozza_id` sempre, più quelli del payload che iniziano per `Cockpit` (`CockpitRichiestaFornitore`=richiesta_id); `Save()`, mai `Send()` (`consenti_invio=false`). Un marcatore che non si riesce a scrivere non ferma la bozza (log) | `RisultatoBozza{entry_id}` → `bozza.stato='aperta'` |
| `segna_letto` | «Segna letto» (capacità `outlook_scrittura`) | `{casella_id, entry_id, letto, message_id}` | `UnRead = not letto` | `messaggio_casella.non_letto` |
| `sposta_in_cartella` | (previsto, nessuna rotta UI oggi; capacità `outlook_scrittura`) | `PayloadSpostaCartella` | `Move()` | `messaggio_casella.cartella` |

**I marcatori tornano con il sync (7B, IB2).** `_converti` legge le `UserProperties` dell'elemento che iniziano
per `Cockpit` (`marcatori_di`) e le manda in `MessaggioIn.marcatori`; qualunque anomalia COM su quelle proprietà
vale «nessun marcatore», non un elemento saltato. Quando la mail di una richiesta compare nella Posta inviata, il
server lega la mail alla richiesta e alla RFQ dal marcatore, senza guardare l'oggetto. Un worker più vecchio non
manda `marcatori`: il server tratta l'assenza come «nessuno» e la richiesta si conferma a mano dall'Inbox.

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
| **bootstrap** | la cartella non ha ancora nessuna copertura | `[adesso − giorni_sync_iniziale, adesso]` (7 se assente; `[outlook].dal` è l'override) | `coperto_fino_a` → `al` (e nasce `storico_fino_a`) |
| **storico** | «Carica precedenti» | `[storico_fino_a − 2 giorni, storico_fino_a]` (senza uno storico riuscito, dalla mail più vecchia che abbiamo) | `storico_fino_a` → `dal` |

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
payload intero) senza fermare gli altri; il commit rende durevoli messaggi, scarti e `ultimo_received` insieme;
su 5xx il worker ripete il lotto identico (l'ingest è idempotente per Message-ID). Un elemento che il worker
non riesce a **convertire** (COM che non risponde, oggetto senza `ReceivedTime`) va in `saltati` e diventa
uno scarto `lettura`. Una cartella che non si riesce a leggere per intero resta un problema di quella cartella:
le altre della stessa casella si leggono lo stesso, e quella si rilegge al giro dopo.

**Gli allegati scendono da soli?** Lo decide il server dal modo del **job** (non dal lotto): `aggiornamento`
sì, se `[staging].automatico`; `bootstrap` solo con `[staging].bootstrap = true`; `storico` mai: «Carica
precedenti» rende consultabile la posta vecchia, non ne scarica gli allegati.

`Restrict`: alla prima lettura di ogni cartella il worker confronta l'insieme degli EntryID restituiti dal
filtro DASL (in UTC) con quello della scansione lineare sulla stessa finestra; se differiscono il filtro
viene spento per quella cartella. `python worker_outlook.py --restrict 7` fa la stessa misura a mano e
stampa i tempi.

Ore: `ReceivedTime` di pywin32 arriva con `tzinfo` UTC ma **numeri locali**; `_utc()` lo tratta come ora
locale del PC e converte. Il server rifiuta un `ricevuto_il` più avanti di `now() + tolleranza` (scarto,
cursore fermo).

## 6. Il worker analisi

`worker_analisi.py` esegue un solo tipo, `analizza_allegato`, creato dal server quando un contenuto entra in
`_contenuti` (`coda.AccodaAnalisi`, chiave `analizza:<sha256>:<versione>:<hash_config>`: un solo job per
contenuto, anche se lo stesso file arriva da tre messaggi).

| | |
|---|---|
| Dove gira | su qualunque postazione (7C.1, P0). Il payload non porta percorsi del server: il worker scarica i byte con `GET /allegati/{id}/contenuto` dentro il proprio tentativo, li scrive in `<staging del worker>\tmp\<job>\`, verifica lo sha256 del payload, analizza e cancella. Fino al banco a due macchine del 20/09/2026 portava `path_staging`, il percorso sul disco del server, e il worker sull'altro PC falliva con «file non trovato in staging» |
| Payload | `PayloadAnalizzaAllegato{allegato_id, sha256, bytes, nome_file, thread_id?, messaggio_id, versione_analizzatore, hash_configurazione, parametri}` — i `parametri` (dizionari dei termini) li manda il server dalla `[analisi]` di `cockpit.toml`. Un `path_staging` in un job vecchio ancora in coda viene ignorato |
| Cosa fa | `analizza_file()`: PDF con PyMuPDF (testo, termini di cartiglio, righe che sembrano codici); STEP con `step_struttura.py` (tokenizer Part 21, nessuna dipendenza nuova): il grafo `PRODUCT` → `NEXT_ASSEMBLY_USAGE_OCCURRENCE` finisce in `dettagli.struttura` come nodi, relazioni e quantita'. **Gli archivi non li vede**: li scompatta il server (`estrai_archivio`) e ogni voce diventa un allegato figlio con il proprio job di analisi |
| Risultato | `RisultatoAnalisi{allegato_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli, versione_analizzatore, hash_configurazione}` → il server esige la stessa versione e configurazione che aveva chiesto (altrimenti il job fallisce in modo definitivo), scrive `analisi_fatti(sha256, versione, hash_config)` e una **proposta** (`documento_proposta`) per ogni allegato aperto con quello sha256, ciascuno con la direzione del suo messaggio e le regole del suo cliente |

Il Python riporta ciò che ha letto nel file; la proposta scritta in database la compone il Go
(`workerapi.propostaDaAnalisi`), una per RFQ. Il worker oggi **ignora `parametri`** e usa i propri dizionari: i
parametri viaggiano ma non vengono letti, e l'allineamento resta da fare.

**La struttura non porta codici** (blocco 8, A1.2). Ogni nodo porta gli attributi grezzi dell'entità —
`id_grezzo`, `nome_grezzo`, `descrizione_grezza`, `rev_grezza` — e l'evidenza di quale riga del file li ha
prodotti. Decidere se `52922757_B` è un codice, e quale ne sia la revisione, dipende dalle famiglie del
CLIENTE di quella richiesta: il worker non sa nemmeno di che cliente si tratti, e quella lettura la fa il
server con le regole già scritte per oggetto, corpo e nomi dei file. `struttura.versione = 2` dice
esattamente questo: la 1 (mai entrata in produzione) metteva i codici nei nodi.

I campi `codice`/`rev` di primo livello del risultato restano quelli di sempre — un'ipotesi dal nome del file
o dal primo `PRODUCT` — e servono a `documento_proposta`. Sono un suggerimento, non la sorgente della
struttura. `[analisi] versione = 2` in `cockpit.toml` è la chiave sotto cui i fatti nuovi vengono conservati:
quelli della 1 restano dove sono.

## 7. Errori e ripresa

`cockpit_client.classifica()` distingue: **rete** (backoff crescente, riprova), **credenziale** 401 e
**autorizzazione** 403 (ogni 30 s, senza backoff: chi corregge il file deve trovare il worker vivo),
**tentativo** 409 (il lavoro corrente non vale più: nessun result), **server** 5xx (riprova), **richiesta**
4xx (errore definitivo: il server registra il fallimento).

Il thread `Battito`: gira accanto al lavoro in **tutti e due** i worker, manda heartbeat ogni
`min(max(5, lease_s/4), 20)` secondi, e quando riceve 409 alza il flag di arresto. Il lavoro che può fermarsi
lo fa ai punti di ripresa (fra un elemento e l'altro nel worker Outlook; fra il download e l'analisi in quello
analisi). Il lavoro bloccato dentro COM — o dentro la lettura di un PDF che non ritorna — non può: dopo 15 s
il processo esce con `os._exit(3)` e l'attività pianificata lo riavvia; il job, lato server, è già tornato
`pronto` e sarà ripreso da un tentativo con token nuovo.

Un solo worker Outlook per PC (mutex); Outlook classico aperto nella sessione dell'utente (COM non gira
senza sessione interattiva).

**Result dopo un buco di rete (7C.1).** `cockpit_client.riporta_risultato()` ripete il POST del result fino a
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
```

Entrambi accettano `--config <file>` e `--debug`.

## 9. Test

`workers/tests/test_*.py` (pytest, L2, con `conftest.py` e `finti_outlook.py`): ciclo del worker contro
`server_finto.py` (che rifiuta ciò che rifiuta il server vero), `Battito` e arresto forzato, presenza e battito,
credenziali, finestra e confine DASL in UTC, elementi saltati, staging del worker (rifiuto se dentro quello del
server, `tmp\` svuotata), analisi su fixture, contratti. `prova_e2e.py` avvia il worker vero con la sola classe
`Outlook` sostituita contro `cockpit.exe` sul database di test (L4/E2E). Le prove su Outlook e Exchange reali
vivono solo in `docs/esiti/esiti_reali.md`, fuori dal repository.
