# Fase 2 — più caselle, una sola elaborazione

Riepilogo tecnico di che cosa introduce questa fase, come si verifica e che cosa resta **non
verificato**. Come per [FASE_0.md](FASE_0.md) e [FASE_1.md](FASE_1.md), la documentazione operativa
completa (piani, decisioni aperte, registri degli esiti, richieste all'IT) vive fuori da questo
repository.

**Stato: in corso.** Di questa fase sono chiuse le voci **2.1**, **2.3** e il blocco **2.2 + 2.6 + 2.7**,
più la **correzione del 15/09/2026** (il risultato dei job non arrivava al server). Le voci successive
(TLS e credenziali individuali, 2.8–2.15) non sono state fatte e sono elencate in fondo.

## Perimetro

Nessun collegamento a caselle di posta reali: tutto ciò che segue è stato provato su un database di
prova usa e getta e con un finto server per i worker. Le migrazioni precedono il codice che le usa.
La visibilità delle caselle personali resta al default restrittivo: nessuna regola di autorizzazione
è stata allentata in questa voce.

## 2.1 — messaggio, casella, presenza (migrazione `0004`)

### Il problema

Fino alla versione 3 dello schema, un messaggio aveva **una** riga `messaggio_outlook` con `entry_id`,
`cartella`, `non_letto`, `categorie`. Il modello diceva, senza dirlo, che un messaggio sta in un posto
solo. La stessa mail mandata a Commerciale e a Francesco è invece **un solo messaggio in due posti**,
ognuno con la sua cartella, il suo stato di lettura e il suo EntryID.

Il danno non si vedeva al momento in cui succedeva: l'ingest andava a buon fine e i conteggi
tornavano. Ma la seconda casella sovrascriveva i dati della prima, e da lì in poi «Apri in Outlook» e
«Scarica allegato» usavano l'EntryID dell'altra casella — un'operazione che fallisce settimane dopo,
per un motivo che nessuno collega più al sync di quel giorno.

E c'era il cursore: `sync_cursore` aveva per chiave la sola **cartella**. Due caselle con una «Posta
in arrivo» scrivevano sulla stessa riga e si facevano avanzare il cursore a vicenda; ogni avanzamento
di troppo è una finestra di tempo che l'altra casella non legge mai. Per questo il server, alla
versione 3, **rifiutava di partire** con più di una casella attiva. Applicata la `0004`, quel
controllo smette da solo di intervenire.

### Che cosa cambia

| | Prima | Ora |
|---|---|---|
| dove sta un messaggio | una riga `messaggio_outlook`: un posto solo | una riga `messaggio_casella` per **copia**: entry_id, cartella, stato di lettura, categorie |
| cursore di sync | per **cartella**: due caselle se lo sovrascrivevano | per **(casella, cartella)** |
| avanzamento del cursore | su `data_evento`, che per la Posta inviata è `SentOn` | su `ricevuto_il`, che è il `ReceivedTime` **in quella casella** — la stessa grandezza su cui filtra la scansione (**W2 chiuso**) |
| direzione | dedotta dal worker: «è la Posta inviata?» e «questo indirizzo è mio?» | decisa dal **server**, confrontando il mittente con le caselle censite |
| mail fra colleghi | indistinguibile da un'offerta inviata | `messaggio.interno`, separato dalla direzione (D10) |
| Inbox | `parent_messaggio_id IS NULL` | ha almeno una presenza **oppure** non è figlio di nessuno; mostra l'elenco delle caselle |

### La direzione la decide il server

Il worker sa due cose, e sono due cose fragili: «questa cartella è la Posta inviata del profilo» e
«questo indirizzo è fra i miei». Su una casella condivisa la prima è ambigua — la Posta inviata di
chi? — e la seconda dipende da come quel profilo Outlook è configurato su quel PC.

Una direzione sbagliata non è un dettaglio: cambia la lettura dell'intero messaggio (niente triage,
nessun cliente riconosciuto, proposte diverse sugli allegati) e il difetto viaggia fino al fascicolo.
Ora decide il server: mittente di un nostro dominio ⇒ **uscita**, altrimenti **entrata**. È una
proprietà del messaggio, uguale in tutte le caselle in cui arriva, e non dipende da quale casella ha
sincronizzato per prima. La dichiarazione del worker resta la riserva per i casi in cui l'elenco delle
caselle non aiuta (indirizzo vuoto, o un indirizzo Exchange in forma di DN senza chiocciola), e viene
**comunque validata**: un valore fuori enum è un difetto del worker, e lasciarlo passare perché tanto
il server ricalcola vorrebbe dire nasconderlo per sempre.

`interno` è l'altra metà: mittente e **tutti** i destinatari nostri. Una mail fra colleghi è in uscita
per definizione, ma non è traffico con il cliente. Tenerla distinta dalla direzione, invece di
aggiungere un terzo valore all'enum, evita di rispondere insieme a due domande che restano diverse —
e il triage la tratta come ciò che è: «ti giro questa richiesta» è uno dei modi in cui una RFQ arriva
davvero sul tavolo, quindi una proposta la riceve.

### Il travaso (test S2)

È la prima migrazione che **sposta righe**, non solo colonne, e va provata su un dump della versione 3
**con dati dentro**: su un database vuoto una migrazione di questo tipo riesce sempre.

Le righe esistenti appartengono per forza all'unica casella attiva — alla versione 3 il server non
parte con più di una. Se la `0004` non riesce a dire con certezza di **chi** sono, **si ferma** invece
di assegnarle a caso e dice che cosa fare: una presenza attribuita alla casella sbagliata è un EntryID
che non apre niente, e non lo scoprirebbe nessuno fino al primo clic su «Apri in Outlook».

## 2.3 — l'allegato viaggia dentro il tentativo (`PUT /api/v1/allegati/{id}/file`)

### Il problema

Fino alla fase 1 il worker salvava l'allegato **direttamente nella cartella di staging del server** e
nel result ne dichiarava il percorso. Funzionava per un motivo solo: worker e server erano sullo stesso
PC. Con il worker su un'altra postazione quel percorso è su un altro disco, e il server non ha niente
in mano. E c'era un secondo difetto, meno visibile (N45): il file arrivava sul disco **prima** che il
server verificasse il tentativo. Un tentativo scaduto a metà download, che finiva di scrivere in
ritardo, poteva sovrascrivere il file appena consegnato dal tentativo che gli era subentrato — e il
result di quest'ultimo avrebbe dichiarato un hash che sul disco non c'era più.

### Che cosa cambia

| | Prima | Ora |
|---|---|---|
| dove scrive il worker | nello staging del server, per percorso | in una cartella temporanea propria, poi `PUT` al server |
| che cosa porta il result | `path_staging`, `sha256`, `bytes` | `sha256`, `bytes`: il file c'è già, legato al tentativo |
| nome sul disco | deciso dal worker | deciso dal server: `<indice>_<nome>` sotto la cartella del messaggio |
| chi consegna il file | l'upload | **il result valido dello stesso tentativo**, dopo aver verificato lo sha256 |
| limite | nessuno | `[server].max_upload_mb` (64): `413` prima di leggere il corpo, allegato in errore con il motivo |

Il `PUT` porta `job_id`, `lease_token` e `worker_id`. Il server verifica il tentativo **prima** di
scrivere un byte e **di nuovo** dopo l'ultimo, senza tenere una transazione aperta nel mezzo (un
trasferimento dura quanto dura). Il file va in `<definitivo>.parte.<lease_token>`: il token nel nome
non è un dettaglio, è ciò che tiene separati i file di due tentativi dello stesso job. A diventare
definitivo — rinomina atomica — è solo il file del result che passa il predicato di validità con la
riga del job **bloccata**, così nessun altro tentativo può diventare valido fra la rinomina e il
commit. Un tentativo scaduto durante il trasferimento riceve `409` e il suo `.parte` viene rimosso; il
suo result tardivo riceve `409` prima ancora che il server guardi il disco (M13).

Lo sha256 del file ricevuto si confronta con quello dichiarato dal worker **fuori** dalla transazione,
come l'estrazione degli zip (voce 1.4). Un hash che non torna non è un contenuto sbagliato ma un
trasferimento andato male: il job torna in coda invece di fallire per sempre, e il file sparisce.

I `.parte.<token>` dei tentativi che non esistono più (upload interrotti, result mai arrivati) li
rimuove lo scheduler ogni quarto d'ora, lasciando stare quelli dei tentativi in corso qualunque età
abbiano e quelli più giovani di dieci minuti (N8, parte).

### Che cosa NON cambia

Il worker continua a salvare l'allegato da Outlook con `SaveAsFile` e a calcolarne lo sha256 in locale:
la parte COM è identica. Cambia solo dove il file va dopo, e chi decide che è arrivato.

## 2.2 + 2.6 + 2.7 — su quale PC (migrazione `0005`)

### Il problema

Tre difetti con la stessa radice: il server non sapeva **dove** far succedere le cose.

- Il claim dava un job a qualunque worker del tipo giusto. Con due worker Outlook su due PC, «Apri in
  Outlook» apriva la finestra sul primo che chiedeva lavoro — non sul PC di chi aveva premuto il
  pulsante — e un sync di Commerciale poteva finire su un worker che Commerciale non l'aveva mai vista.
- Lo StoreID viaggiava nei payload e in `messaggio_casella.store_id_locale`. È locale al **profilo**
  Outlook che l'ha letto (N44): su un altro PC non è un riferimento, è un numero a caso. Con due
  postazioni, l'ultima che sincronizzava vinceva e le azioni dell'altra non trovavano l'elemento.
- La testata diceva «outlook attivo» con una riga sola per tipo: con due PC non diceva di chi.

### Che cosa cambia

| | Prima | Ora |
|---|---|---|
| claim | tipo di worker | tipo **e** caselle che il worker serve davvero (dichiarate nel claim ∩ `worker_credenziale.caselle`) **e** postazione della credenziale, se il job ne ha una |
| chi dice le caselle | il file di configurazione del worker | il **server** (`GET /api/v1/worker/caselle`): il worker le risolve nel proprio profilo e dichiara solo quelle trovate |
| StoreID | nei payload e in `messaggio_casella` | in `casella_store` (postazione, casella), scritto dal worker a ogni claim; i payload portano `casella_id + entry_id + Message-ID` |
| job interattivi | al primo worker Outlook | alla **postazione della sessione**, con casella, richiedente e scadenza (10 min; bozza 60). Senza postazione o senza worker idoneo: **nessun job**, motivo esplicito |
| download | alla «prima copia» | alla copia servita dalla postazione della sessione se c'è, altrimenti quella di riferimento; lo esegue qualunque worker autorizzato sulla casella |
| sessione UI | utente | utente **e postazione**: abbinata per IP a un worker attivo di una postazione autorizzata (`ip`), o scelta in testata (`scelta`) |
| presenza | una riga per tipo | una riga per **worker**: postazione, IP dell'ultimo claim, Outlook raggiungibile, caselle risolte, avviso, ultimo arresto |
| testata | «outlook attivo / OFFLINE» | «Sei su: PC-…» + per ogni casella **attiva** su quale PC / **OFFLINE** / **non risolta** / **non configurata** |
| «Annulla» | — | `POST /admin/job/{id}/annulla`: un job pronto o in corso diventa `annullato` (il tentativo in corso riceve 409 al battito) |

Il server **non si fida del JSON** del claim: una casella dichiarata ma non autorizzata viene ignorata,
scritta nell'avviso della presenza e nel log (Q18); un worker non censito, o censito su un'altra
postazione, riceve `403` con il motivo. La postazione di un worker è quella della sua credenziale,
mai quella che dichiara: dichiararne un'altra è un `worker.toml` copiato sul PC sbagliato, e il claim
lo rifiuta.

L'abbinamento per IP della sessione è una comodità, **non un'autorizzazione** (P1): vale solo verso
una postazione attiva e abilitata all'utente (la sua, o qualunque per un admin), registrata da un
worker nelle ultime 24 ore. Un IP sconosciuto non abilita nulla; una postazione altrui scelta a mano è
`403`. `X-Forwarded-For` non viene letto: il server non sta dietro a un proxy e quell'header lo scrive
chiunque. I test simulano gli IP con un gancio (`IndirizzoClient`) che in produzione non esiste.

### D5 resta aperta, e il codice non la chiude

Commerciale può essere una cassetta condivisa con delega o un account con credenziali proprie. Il
worker non assume nessuna delle due forme: per ogni casella censita prova, nell'ordine, un account del
profilo con quello SMTP, uno store del profilo che le corrisponde, `CreateRecipient` +
`GetSharedDefaultFolder`. Il log dice quale strada ha funzionato. Quale sia quella vera per la
Commerciale reale si vede solo con M1 sul profilo vero.

### Il profilo reale come banco di prova

Sul profilo Outlook della postazione di prova sono visibili tre store: Francesco, Commerciale e Filippo. Filippo **non
è censito** e deve restare invisibile al Cockpit: il worker parte dall'elenco del server, non dal
profilo, quindi non lo chiede, non lo risolve, non lo dichiara e non lo legge. Il test L2 riproduce
esattamente questa situazione (tre store finti, due censiti); la prova reale è M1, con
`python worker_outlook.py --caselle`, **da eseguire solo come prova concordata**.

### Che cosa NON cambia

Il predicato di validità del tentativo, l'upload legato al tentativo (2.3), la chiave di sync per
casella (2.1). Il token condiviso resta: le credenziali individuali sono la 2.5.

## Correzione del 15/09/2026 — il risultato non arrivava al server

Prima volta che il worker vero ha parlato con il server vero, ed è uscito subito. Non c'entra il
modello dei dati di questa fase: è un difetto di due righe nel client, e vale la pena scriverlo qui
soprattutto per il motivo per cui nessuna prova lo aveva visto.

### Il sintomo

Nel log del worker, a ogni job: `POST /api/v1/jobs/N/result → 400 {"errore":"worker_id mancante"}`.
Il worker faceva il lavoro — leggeva la posta, scaricava l'allegato, apriva l'elemento — e il server
non lo sapeva. Il job restava `in_corso`, il lease scadeva dopo 120–300 secondi, lo scheduler lo
rimetteva `pronto` e il worker lo rifaceva da capo. Da fuori si vedeva questo: gli stati non
cambiavano mai, lo stesso sync di Commerciale partiva quattro volte, e «Apri in Outlook» apriva la
stessa finestra quattro volte. Persino un errore definitivo veniva ritentato, perché nemmeno il
fallimento riusciva a essere registrato.

### La causa

In `workers/cockpit_client.py`, `Cockpit.risultato`:

```python
corpo.setdefault("worker_id", worker_id or self.worker_id)   # non fa niente
corpo.setdefault("lease_token", lease_token)                 # non fa niente
```

`corpo` arriva da `RisultatoRichiesta.model_dump()`, e il modello dichiara `worker_id: str = ""` e
`lease_token: str = ""`. Le chiavi **esistono già**, vuote: `setdefault` le trova presenti e non le
tocca. Il server riceve `worker_id=""`, `tentativo()` lo rifiuta con 400, e il predicato che protegge
il job — quello che impedisce a un tentativo scaduto di scrivere sopra al lavoro di un altro — non
viene nemmeno raggiunto. La correzione è un'assegnazione al posto di `setdefault`: chi chiama conosce
il tentativo, e vince sempre su ciò che il modello ha lasciato vuoto.

### Perché quattro livelli verdi non l'hanno visto

Perché nessuno di loro faceva parlare il client vero con il server vero.

| Livello | Che cosa provava | Perché passava |
|---|---|---|
| L2 worker | il ciclo del worker contro `server_finto.py` | il server finto accettava qualunque `/result`, senza guardare i campi |
| L3 contratti | che i tipi Go e i modelli pydantic descrivano lo stesso schema | gli schemi erano identici: il difetto non era nel contratto, ma in come il client lo compilava |
| L4 integrazione | il server contro PostgreSQL, con un client Go scritto nel test | il client Go i campi li metteva |
| L1 | compilazione e unitari | fuori tema |

Il difetto stava esattamente nel punto cieco comune: **come il client vero riempie il contratto**.

### Che cosa è cambiato

1. **La correzione**, due righe in `cockpit_client.py`, con una prova di regressione in L2 che passa
   il corpo *come lo passa il worker* (già completo di chiavi vuote) e verifica che l'identità del
   tentativo arrivi comunque.
2. **Il server finto ora rifiuta ciò che rifiuta quello vero.** `claim`, `heartbeat`, `result`,
   `ingest` e l'upload pretendono `worker_id` e `lease_token` (400 senza) e rispondono 409 a chi si
   presenta con un tentativo diverso da quello consegnato dal claim, come il predicato SQL del server.
   `metti_job` assegna sempre un `lease_token`, perché il claim vero lo fa. Un banco di prova che
   accetta ciò che il vero rifiuta non è un banco di prova: è un test che passa per costruzione.
3. **Un livello che non c'era: il client vero contro il server vero.** `TestE2E…` in
   `internal/workerapi` avvia `python workers/prova_e2e.py` — cioè `worker_outlook.main()` con
   `--una-volta`, con la sola classe `Outlook` sostituita — contro i gestori HTTP veri e PostgreSQL,
   e verifica che il job arrivi a `fatto`: un download completo (claim → `PUT` → result → file
   promosso nello staging) e un job interattivo lungo, durante il quale il battito rinnova davvero il
   lease. Rimettendo il `setdefault`, tutti e due diventano rossi con il sintomo di produzione:
   *job in stato in_corso*.
4. **Il server scrive il proprio log su file.** Fino a ieri esisteva solo quello dei worker: del lato
   server — chi ha risposto 400, a chi, quante volte — non restava niente appena si chiudeva la
   finestra, e questo è costato metà della diagnosi. Ora va anche in `<nas.staging>\log\cockpit.log`
   (5 file da 5 MB, come i worker; `[server].log_file` lo sposta, `"-"` lo disattiva), accanto a
   `worker_outlook.log`. E un `result` rifiutato per tentativo non dichiarato viene scritto come
   avviso, con il nome del worker e il job: la prossima volta si legge da questa parte.

### Strumenti per la diagnosi

| | |
|---|---|
| `scripts\query-debug.sql` | le sei domande della diagnosi di un job: la coda come la vede il server, i cursori, chi è collegato e che cosa serve, una mail in due caselle (I3), gli scarti, gli allegati |
| `scripts\azzera-dati.ps1` | ricrea lo schema di **sviluppo** e cancella i log, per ripartire da una riga pulita. Senza `-Conferma` dice soltanto che cosa farebbe; si rifiuta di partire se il server o un worker sono in esecuzione, e non tocca mai NAS, posta e database di test |

### Che cosa questa prova NON dimostra

- **L'avvio di `cockpit.exe`**: il test monta gli stessi gestori HTTP su un server di prova, non
  lancia l'eseguibile. Configurazione, migrazioni e scheduler restano materia di `main.go`.
- **Outlook**: l'adattatore COM è sostituito. Che cosa succede con Outlook vero è L5.
- **Il sync**: qui girano un download e un job interattivo. Il sync completo contro la posta vera
  resta una prova reale concordata.

## Come verificare

```powershell
cd cockpit
powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Avvia
powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1
```

I test d'integrazione condividono un solo database e alcuni ricreano lo schema: `-p 1` non è
un'ottimizzazione, è una condizione di correttezza.

## Che cosa è verificato e che cosa no

| Livello | Copertura di questa voce | Stato |
|---|---|---|
| L1 unitari Go | triage di una mail interna, verifica statica della `0004`; nome nello staging e cartelle rifiutate (2.3); tre stati della testata e autorizzazione alla postazione (M2, P1); rotazione del log del server | eseguiti |
| L2 worker Python | lo stage carica con `PUT` prima del result; `413` → errore definitivo; `409` → nessun result; il worker risolve solo le caselle censite e le dichiara al claim, ignora il terzo store, legge lo store della casella del job, non usa `store_id` dal payload (M1, M12); il result porta il tentativo anche quando il modello lo dichiara vuoto, e il server finto rifiuta come quello vero | eseguiti |
| L3 contratti | `MessaggioIn.ricevuto_il`, `RiferimentoElemento.casella_id`, `RisultatoStage` senza `path_staging`, `ClaimRichiesta` con `caselle_aperte`, payload senza `store_id`, `CasellaServita` sui due lati | eseguiti |
| L4 integrazione | I3, I4, I18, I21, S2 sulla `0004` con dati, cursore per casella, direzione dalle caselle; **M7, M8, M13**; **Q8, Q17, Q18, Q21, M2, M3, M4, M6, M9, M10, M12, W14, P1** | eseguiti |
| L4 end-to-end | il worker **vero** (Python, senza COM) contro il server **vero** su PostgreSQL: un download completo fino a `fatto` e il battito che rinnova il lease durante un job lungo | eseguiti |
| L5–L9 | due caselle vere in Outlook (M1 con `--caselle`), casella condivisa Exchange, due postazioni (M11, upload fra due PC), postazione della sessione da un browser vero (W14 L7) | **non eseguiti** |

Tre precisazioni che valgono anche per chi legge solo questo file:

- i test sono scritti contro i **fatti osservabili** — quante presenze, quale cursore, che cosa resta
  deciso — e non contro le query. Sono stati verificati anche al contrario: rimettendo a mano la
  vecchia regola dell'Inbox, il vecchio cursore su `data_evento` e la vecchia direzione dal worker,
  i tre test corrispondenti diventano rossi con il motivo giusto;
- **I9 resta bloccato**: che la direzione sia giusta su una casella **condivisa Exchange** vera non è
  dimostrato qui. Qui si dimostra che la decisione non dipende più dal profilo Outlook locale, il che
  è la condizione perché I9 possa essere provato — non la prova;
- un test **saltato** non è un test superato.

## Limiti dichiarati di questa voce

**Chi può usare una postazione.** Il proprietario (`postazione.utente_id`) e gli admin. Non esiste
ancora una tabella di abilitazioni per postazione: se servirà «Luigi può lavorare anche da
PC-FRANCESCO», è un'estensione di `puoUsarePostazione`, non un cambio di modello.

**Il worker di analisi non risolve caselle.** Dichiara la postazione e nient'altro: prende job senza
casella né postazione. È corretto per ciò che fa oggi (legge file dallo staging del server).

**La risoluzione dello store è provata solo in forma simulata.** Le tre strade (account, store del
profilo, cassetta condivisa con delega) sono scritte contro l'Object Model di Outlook ma nessuna è
stata eseguita su un profilo vero: M1 è la prova, e resta NON ESEGUITA finché non viene concordata.

**Il filtro per casella nell'Inbox non è un permesso.** È il filtro della schermata. Chi può vedere
che cosa è la voce 2.2 (`puoVedere`), e fino ad allora vale la regola restrittiva di D12: l'aggancio a
una RFQ non amplia la visibilità della posta personale.

**Il worker di analisi e le bozze leggono ancora lo staging del server.** La voce 2.3 porta al server
il file *scaricato da Outlook*; il worker di analisi riceve ancora `path_staging` e lo apre dal disco,
e una bozza con allegati riceve percorsi assoluti. Finché non hanno una via di ritorno (un `GET` del
file), il worker di analisi va sullo stesso PC del server e le bozze con allegati richiedono che il
worker Outlook veda lo staging del server. È un limite dichiarato, non un difetto nascosto: il caso
d'uso della 2.3 è il worker Outlook su un'altra postazione, e quello è coperto.

**`parent_messaggio_id` resta.** Il piano lo elimina insieme alle occorrenze (fase 3, §3.5), e il test
S2 lo dice esplicitamente per la `0005`. Toglierlo adesso cancellerebbe l'unico legame fra un messaggio
annidato e il suo contenitore senza avere ancora ciò che lo sostituisce. Per questo la regola
dell'Inbox della voce 2.1 usa oggi quel campo dove domani userà le occorrenze: il comportamento
dichiarato — un messaggio ricevuto non sparisce perché arriva anche come allegato — è già quello.

## Che cosa resta aperto in questa fase

- **2.4 + 2.5** TLS con impronta in `worker.toml`, credenziale individuale al posto del token
  condiviso, CSRF — test W4, W10, W11. Prima di questa il server resta esposto sulla LAN solo per le
  prove.
- **2.8–2.15** come da piano.
