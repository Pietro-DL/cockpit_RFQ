# I worker — come parlano con `cockpit.exe`

Documentazione dei due client Python (`worker_outlook.py`, `worker_analisi.py`) e del modulo comune
(`cockpit_client.py`). Scritta sul codice a `0bdea9f`. Quando il codice cambia, cambia questo file.

## 1. Il principio: il server non chiama mai nessuno

Il server Go non conosce l'indirizzo di nessuna postazione e non apre connessioni verso i PC. Sono i worker
a collegarsi, sempre in uscita, verso un solo indirizzo (`server_url` in `worker.toml`). Per questo:

- serve **una** regola di firewall, in ingresso sul server, e nessuna sui PC;
- un worker spento è semplicemente un worker che non chiama; il server lo capisce dal tempo passato
  dall'ultimo contatto (`worker_presenza.ultimo_contatto`), non da un ping;
- "chi fa cosa" è deciso dal server al momento del **claim**: il worker chiede *«c'è lavoro per me?»* e il
  server risponde con il primo job compatibile con il suo tipo, la sua postazione e le sue caselle.

Tutto ciò che un worker scrive nel database passa dal server, mai da una connessione diretta a PostgreSQL.

## 2. Il ciclo di vita di un job

Un job è una riga di `job` (`stato`, `tipo`, `payload` JSON, `casella_id`, `postazione_id`, `lease_*`).
La riga la **crea sempre il server**: o su richiesta della UI (un clic dell'operatore) o dallo scheduler.
Il worker non inserisce mai job; li prende, li esegue, ne riporta l'esito.

```
                     SERVER (Go)                                        WORKER (Python)
  UI: POST /messaggio/{id}/scarica  ──► INSERT job(stato='pronto')
  scheduler ogni intervallo_sync_s  ──► INSERT job sync_outlook (uno per casella, chiave fissa)

                                            ◄── POST /api/v1/jobs/claim  {worker, worker_id, postazione,
                                                                          outlook_ok, caselle_aperte, attesa_s}
  ContattoWorker (presenza: sono vivo)
  long-poll fino a attesa_s (≤25 s)
  UPDATE job SET stato='in_corso',
     lease_token=uuid, lease_fino_a=now()+lease_s,
     avviato_il=now()  … FOR UPDATE SKIP LOCKED
                                            ──► {job_id, tipo, payload, lease_token, lease_s, durata_max_s}
                                                                            esegue (dispatch per tipo)
                                            ◄── POST /jobs/{id}/heartbeat   ogni lease_s/4, da un THREAD SEPARATO
  predicato di validità (vedi sotto)         ──► 200  | 409 se il tentativo non vale più
                                            ◄── POST /api/v1/ingest/messaggi (solo sync, un lotto per volta)
                                            ◄── PUT  /api/v1/allegati/{id}/file (solo stage)
                                            ◄── POST /jobs/{id}/result  {esito, dati, errore, definitivo,
                                                                         worker_id, lease_token}
  UPDATE job SET stato='fatto' | 'fallito' (+ non_prima_di per il retry)
  applica il risultato (allegato, bozza, cursore…)
```

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
con la stessa chiave (`sync_outlook:<casella>`, `analizza:<sha256>:<versione>:<config>`, `analisi-ai:<id>`).

## 3. L'avvio di un worker

1. `cockpit_client.carica_config()` legge `worker.toml`: `server_url`, `impronta` del certificato (se
   TLS), sezione `[outlook]` o `[analisi]` con `worker_id` e `token`. Il token è **individuale**: viene
   generato dalla pagina *Admin › Postazioni › Rigenera credenziali* insieme al pacchetto; un token vuoto
   ferma il worker con un messaggio che dice dove prenderlo.
2. Il worker Outlook chiede `GET /api/v1/worker/caselle?worker_id=…`: il server risponde con le caselle
   che quel worker è **autorizzato** a servire (`worker_credenziale.caselle`), non con quelle del profilo.
3. `outlook_com.risolvi_caselle()` cerca ogni casella autorizzata nel profilo Outlook locale (account, store
   del profilo, o cartella condivisa) e ne annota lo `store_id` **locale**. Uno store del profilo che non è
   fra le caselle autorizzate viene ignorato (es. la casella di un collega presente nel profilo).
4. Da lì in poi ogni claim dichiara `postazione`, `outlook_ok` e `caselle_aperte` (`casella_id` +
   `store_id`): il server interseca con le autorizzazioni e registra lo store in `casella_store` per quella
   postazione. È il server a scegliere il job, il worker a eseguirlo.

Il worker analisi salta i punti 2–4: ha solo `worker_id`, `token` e il percorso dello staging.

## 4. Gli endpoint (tutti sotto `/api/v1`, tutti autenticati con `X-Cockpit-Token`)

| Metodo e percorso | Chi | Quando | Corpo → Risposta |
|---|---|---|---|
| `POST /jobs/claim` | entrambi | in ciclo, appena liberi | `ClaimRichiesta` → `Job` oppure 204 dopo `attesa_s` |
| `GET /worker/caselle` | outlook | all'avvio e ogni riavvio | `?worker_id=` → elenco caselle autorizzate |
| `POST /jobs/{id}/heartbeat` | entrambi | ogni `lease_s/4` dal thread `Battito` | `{worker_id, lease_token}` → 200 / 409 |
| `POST /jobs/{id}/result` | entrambi | fine del job | `RisultatoRichiesta` → 200 / 409 / 422 |
| `POST /ingest/messaggi` | outlook | durante `sync_outlook`, un lotto per volta | `IngestRichiesta` → `IngestRisposta` (inseriti, aggiornati, falliti, cursore) |
| `PUT /allegati/{id}/file` | outlook | durante `stage_allegato` | binario + `?job_id&lease_token&worker_id&sha256` → 200 / 409 / 413 / 422 |
| `GET /sync/cursori` | outlook | inizio di `sync_outlook` | i cursori (casella, cartella) del server |

Il contratto dei corpi è in `contratti.py` (pydantic) e in `internal/api/tipi.go`; `genera_contratti.py`
produce gli schemi JSON in `contracts/` e i test dei due lati li confrontano (L3).

## 5. Le funzioni del worker Outlook

`worker_outlook.py:dispatch()` sceglie per `job.tipo`. Per ogni tipo: chi crea il job, con quale payload,
cosa fa il worker, cosa riporta, cosa cambia nel database quando il server applica il risultato.

| Tipo | Chi lo crea | Payload | Cosa fa il worker | Risultato → effetto |
|---|---|---|---|---|
| `sync_outlook` | scheduler (`intervallo_sync_s`), «Aggiorna ora» (`POST /inbox/aggiorna`), «Carica precedenti» (`POST /inbox/sync-storico`), prima apertura dell'Inbox | `PayloadSyncOutlook{casella_id, cartelle[], dal, al?, sovrapposizione_s, lotto}` | legge la casella con `Restrict` (self-test alla prima lettura), converte gli elementi, manda lotti a `/ingest/messaggi` | `RisultatoSync{cartelle[]{n_messaggi, n_falliti, saltati}}`; i cursori li ha già scritti l'ingest lotto per lotto |
| `rileggi_elemento` | «Riprova» su uno scarto `lettura` (`POST /admin/scarti/{id}/riprova`) | `{casella_id, entry_id}` | rilegge quel solo elemento e manda un lotto da uno | come sopra |
| `stage_allegato` | «Scarica» (`POST /messaggio/{id}/scarica`), «Riscarica», staging automatico all'ingest (D30) | `PayloadStageAllegato{casella_id, entry_id, message_id, indice, percorso?}` | apre l'elemento nello store locale (fallback per Message-ID), `SaveAsFile` in temp, sha256, `PUT` al server | `RisultatoStage{sha256, byte}` → `allegato.stato='in_staging'`, poi il server accoda `analizza_allegato` |
| `apri_elemento_outlook` | «Apri in Outlook» | `{casella_id, entry_id, message_id}` | `Display()` sull'elemento | nessun effetto nel DB |
| `crea_bozza_outlook` | «Rispondi» | `PayloadCreaBozza{…, bozza_id, testo}` | `Reply()`, inserisce il testo sopra la firma, `UserProperties["CockpitBozza"]=bozza_id` (idempotenza), `Save()`, mai `Send()` (`consenti_invio=false`) | `RisultatoBozza{entry_id}` → `bozza.stato='aperta'` |
| `segna_letto` | «Segna letto» | `{casella_id, entry_id, letto}` | `UnRead = not letto` | `messaggio_casella.non_letto` |
| `sposta_in_cartella` | (previsto, nessuna rotta UI oggi) | `PayloadSpostaCartella` | `Move()` | `messaggio_casella.cartella` |

I job interattivi (`apri`, `bozza`, `letto`) portano `postazione_id` = la postazione dell'operatore che ha
cliccato: li prende solo il worker di quel PC. `sync` e `stage` portano solo `casella_id`: li prende
qualunque worker autorizzato a quella casella.

In modalità shadow (`[server].modalita = "shadow"`) `bozza`, `letto` e `sposta` non vengono né accodati né
consegnati al claim; `apri`, `sync` e `stage` sì.

### 5.1 Il sync in dettaglio

Tre situazioni, due algoritmi:

| Modalità | Quando | Finestra | Ordine | Cursore |
|---|---|---|---|---|
| **LIVE** | scheduler, «Aggiorna ora», con cursore presente | `[cursore − sovrapposizione_s, now]` | vecchio → nuovo | avanza **per lotto**, nella stessa transazione dell'ingest |
| **BOOTSTRAP** | casella/cartella senza cursore | `[now − giorni_sync_iniziale, now]` con `al` fisso | come lo storico | scritto **al completamento** della finestra |
| **STORICO** | «Carica precedenti» | 2 giorni all'indietro dal confine più vecchio già acquisito, `al` fisso | come lo storico | non tocca il cursore live; sposta il confine dello storico |

Perché due regole sul cursore: nel LIVE i lotti sono cronologici e ogni 200 rende durevole ciò che
precede; se il worker muore, si riparte dall'ultimo lotto confermato senza buchi. Nel BOOTSTRAP/STORICO
la finestra è chiusa da `al`, quindi si può leggere in qualunque ordine, ma il cursore si scrive solo
quando tutta la finestra è entrata: altrimenti un'interruzione a metà salterebbe i giorni non letti.

Per ogni lotto: `IngestRichiesta{casella_id, messaggi[], …}` → il server apre **una transazione per lotto**
con un `SAVEPOINT` per elemento; un elemento rifiutato va in `ingest_scarto` (origine `ingest`, con il
payload intero) senza fermare gli altri; il commit rende durevoli messaggi, scarti e cursore insieme; su
5xx il worker ripete il lotto identico (l'ingest è idempotente per Message-ID). Un elemento che il worker
non riesce a **convertire** (COM che non risponde, oggetto senza `ReceivedTime`) va in `saltati` e diventa
uno scarto `lettura`, riprovabile con `rileggi_elemento`.

`Restrict`: alla prima lettura di ogni cartella il worker confronta l'insieme degli EntryID restituiti dal
filtro DASL (in UTC) con quello della scansione lineare sulla stessa finestra; se differiscono il filtro
viene spento per quella cartella. `python worker_outlook.py --restrict 7` fa la stessa misura a mano e
stampa i tempi.

Ore: `ReceivedTime` di pywin32 arriva con `tzinfo` UTC ma **numeri locali**; `_utc()` lo tratta come ora
locale del PC e converte. Il server rifiuta un `ricevuto_il` più avanti di `now() + tolleranza` (scarto,
cursore fermo).

## 6. Il worker analisi

`worker_analisi.py` esegue un solo tipo, `analizza_allegato`, creato dal server quando un file entra nello
staging (`jobs.AccodaAnalisi`, chiave `analizza:<sha256>:<versione>:<hash_config>`: un solo job per
contenuto, anche se lo stesso file arriva da tre messaggi).

| | |
|---|---|
| Dove gira | sulla stessa macchina del server (legge `path_staging` dal filesystem) |
| Payload | `PayloadAnalizzaAllegato{allegato_id, path_staging, nome_file, cliente_id?, parametri}` — i `parametri` (dizionari dei termini) li manda il server dalla `[analisi]` di `cockpit.toml` |
| Cosa fa | `analizza_file()`: PDF con PyMuPDF (testo, termini di cartiglio, righe che sembrano codici), STEP (PRODUCT, occorrenze), zip già estratto dal server prima |
| Risultato | `RisultatoAnalisi{versione_analizzatore, fatti{…}}` → il server scrive `analisi_fatti(sha256, versione, hash_config)` e ricalcola la **proposta** (`documento_proposta`) per tutti gli allegati aperti con quello sha256, ciascuno con le regole del proprio cliente |

Il Python riporta **fatti**; la decisione su tipo, codice e revisione è del Go (`domain.ProponiDaAnalisi`).
Oggi il worker usa ancora i propri dizionari accanto a quelli ricevuti: allineamento previsto nella fase 5.

## 7. Errori e ripresa

`cockpit_client.classifica()` distingue: **rete** (backoff crescente, riprova), **credenziale** 401 e
**autorizzazione** 403 (ogni 30 s, senza backoff: chi corregge il file deve trovare il worker vivo),
**tentativo** 409 (il lavoro corrente non vale più: nessun result), **server** 5xx (riprova), **richiesta**
4xx (errore definitivo: il server registra il fallimento).

Il thread `Battito`: gira accanto al lavoro, manda heartbeat, e quando riceve 409 alza il flag di arresto.
Il lavoro che può fermarsi lo fa ai punti di ripresa (fra un elemento e l'altro). Il lavoro bloccato dentro
COM non può: dopo 15 s il processo esce con `os._exit(3)` e l'attività pianificata lo riavvia; il job, lato
server, è già tornato `pronto` e sarà ripreso da un tentativo con token nuovo.

Un solo worker Outlook per PC (mutex); Outlook classico aperto nella sessione dell'utente (COM non gira
senza sessione interattiva).

## 8. Comandi diagnostici

```
python worker_outlook.py --cartelle       # albero delle cartelle di ogni store del profilo, con conteggi
python worker_outlook.py --caselle        # caselle autorizzate → store locale risolto; la dichiarazione al claim
python worker_outlook.py --restrict 7     # Restrict vs scansione lineare sugli ultimi 7 giorni, con i tempi
python worker_outlook.py --una-volta      # un solo job ed esce (usato dai test E2E)
python worker_analisi.py --una-volta
```

## 9. Test

`workers/test_*.py` (pytest, L2): ciclo del worker contro `server_finto.py` (che rifiuta ciò che rifiuta il
server vero), `Battito` e arresto forzato, finestra DASL in UTC, analisi su fixture. `prova_e2e.py` avvia il
worker vero con la sola classe `Outlook` sostituita contro `cockpit.exe` sul database di test (L4/E2E). Le
prove su Outlook e Exchange reali vivono solo in `docs/esiti/esiti_reali.md`.
