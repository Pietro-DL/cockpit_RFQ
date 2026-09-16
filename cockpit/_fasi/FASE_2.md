# Fase 2 — più caselle, una sola elaborazione

Riepilogo tecnico di che cosa introduce questa fase, come si verifica e che cosa resta **non
verificato**. Come per [FASE_0.md](FASE_0.md) e [FASE_1.md](FASE_1.md), la documentazione operativa
completa (piani, decisioni aperte, registri degli esiti, richieste all'IT) vive fuori da questo
repository.

**Stato: completata** (16/09/2026, dopo il test manuale del RBAC in un browser vero).
Di questa fase sono chiuse le voci **2.1**, **2.3**, il blocco
**2.2 + 2.6 + 2.7**, il **blocco 1 dell'addendum** (2.9, 2.16, 9.5 — con la correzione del fuso del
16/09) e il **blocco 2** (2.4 TLS e credenziali individuali, 2.5 CSRF, VM Linux, pagina *Postazioni*,
con la correzione del 16/09 sul passaggio dal token condiviso), più le correzioni del 15 e del
16/09/2026, il **checkpoint UI/RBAC** (6.9 e metà della 6.4, anticipate) e il **checkpoint sync**
(voce 2.8: finestra iniziale e archivio a pezzi da due giorni).

Completata **non** vuol dire che non resti niente di questi numeri: le voci **2.10–2.15** non sono
state fatte qui perché l'addendum del 16/09 le ha spostate nei blocchi dove servono, e alcune prove
reali restano aperte perché aspettano hardware o Exchange che ancora non c'è (il secondo PC, la VM
Linux con il NAS montato, la casella condivisa). Sono elencate in fondo, una per una.

Le prove reali su `4e2f68b` sono **passate**: M1, C2/C3, TZ2 e CR1 sono righe compilate di
`esiti_reali.md`, che vive fuori da questo repository.

## Perimetro

Tutto ciò che segue è stato provato su un database di prova usa e getta e con un finto server per i
worker. Le migrazioni precedono il codice che le usa. La visibilità delle caselle personali resta al
default restrittivo: nessuna regola di autorizzazione è stata allentata in questa fase.

Le prove **sulle caselle di posta reali** sono un'altra cosa e vivono in un altro posto: si eseguono
solo quando sono concordate, una alla volta, e si annotano a mano in `esiti_reali.md` (fuori da
questo repository). Il 15 e il 16/09/2026 ne sono state eseguite alcune sul profilo Outlook di una
postazione di prova — sono quelle che hanno trovato il difetto del risultato dei job, quello del fuso
e il passaggio alle credenziali individuali. Un esito di questo file non ne sostituisce mai uno di
quello: qui dentro Outlook non c'è.

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

## Blocco 1 dell'addendum (16/09/2026) — Inbox viva e shadow (voci 2.9, 2.16, 9.5)

Tre cose che stanno insieme per un motivo solo: l'Inbox del Cockpit deve essere il posto dove si
guarda la posta, e oggi non lo è. Se mostra le stesse mail di un'ora fa, l'operatore non sa se non è
arrivato niente o se il Cockpit è fermo, e nel dubbio riapre Outlook — e da quel momento il Cockpit
è un doppione che nessuno guarda. La shadow sta nello stesso blocco perché è ciò che permette di
tenere il sync acceso sulla posta vera senza rischiare di modificarla.

### 2.9 — la finestra si chiede a Outlook, non si cerca a mano

Il sync scorreva **tutta** la cartella dal messaggio più recente all'indietro, fermandosi al primo
più vecchio della finestra. Su Commerciale sono circa cento secondi per giro, il che rende
impossibile il sync al minuto: il worker farebbe solo quello.

Ora la finestra si chiede con `Items.Restrict` e un filtro DASL su `urn:schemas:httpmail:datereceived`
scritto **in UTC**. Il fuso non è un dettaglio: con la sintassi Jet (`[ReceivedTime] >= '15/09/2026'`)
la stessa riga significa 15 settembre su un PC italiano e nulla su uno americano, e un filtro che non
corrisponde a niente non dà errore — dà zero messaggi. I due estremi vengono arrotondati **verso
l'esterno** al minuto, perché fra i due errori possibili solo uno si può correggere dopo: un
messaggio letto due volte costa una deduplica per Message-ID, che il server fa comunque; un messaggio
non letto non lo cerca più nessuno.

**Il self-test (C3) è la parte che conta.** Un `Restrict` che perde un elemento non lascia tracce:
nessun errore, nessun conteggio sbagliato, solo una mail che non esiste. Perciò la prima volta che
legge una cartella il worker esegue **entrambi** i modi sulla stessa finestra e confronta gli
**insiemi** di EntryID — non i conteggi, che coinciderebbero anche scambiando un messaggio con un
altro. Se il filtro ne perde uno, viene spento per quella cartella e si continua con la scansione
lineare: più lenta, ma completa. Il confronto tiene separate le due differenze, perché non pesano
uguale: gli elementi *mancanti* bocciano, quelli *in più* (i bordi arrotondati al minuto) si
segnalano e basta.

La prova si fa sugli ultimi sette giorni della finestra (`autoprova_restrict_giorni`), non su tutto
l'archivio: una scansione lineare su anni di posta costerebbe esattamente ciò che la voce 2.9 vuole
evitare, e ciò che c'è da dimostrare — che il fuso e il formato siano quelli giusti — si dimostra su
un campione come su tutto. Due insiemi vuoti non contano come prova superata: l'esito «non
concludente» non viene memorizzato, e si riprova al giro dopo.

Sul profilo vero questo è C2/C3, ed è una prova L5: `python worker_outlook.py --restrict 7` legge due
volte ogni cartella servita, confronta gli insiemi e stampa i due tempi. Non prende job e non tocca
niente.

### 2.16 — l'Inbox dice che cosa sta succedendo

- **«Aggiorna ora»** in testata accoda un `sync_outlook` per ogni casella attiva. Premuto dieci volte
  accoda un job solo: la chiave di idempotenza è fissa per casella, la stessa che usa lo scheduler
  (SV1). E dice sempre che cosa ha fatto, anche quando non ha fatto niente — un pulsante che non
  risponde è un pulsante che si preme di nuovo;
- **lo stato per casella** ha un quarto valore, *in corso*, che compare mentre un sync di quella
  casella è in coda o in esecuzione, e solo su una casella **attiva**: se il worker è spento e un
  sync è in coda, la verità è che è spento, e scrivere «in corso» prometterebbe qualcosa che non sta
  per arrivare. La chip porta anche l'ora dell'ultimo sync **riuscito**, letta dal job e non dal
  cursore (un sync che non trova niente di nuovo — il caso normale a regime — non muove il cursore, e
  la testata direbbe «ultimo sync 12:11» per ore mentre il worker lavora regolarmente);
- **«nuove dall'ultima visita»**: `utente.ultima_vista_inbox` (migrazione `0006`) e il conteggio dei
  messaggi con `registrato_il` successivo, in testata, per casella, e come pallino sulle righe.
  L'orologio si sposta solo quando la pagina viene **aperta**, mai ai poll HTMX ogni 15 secondi:
  altrimenti il contatore direbbe sempre zero. È per utente e non per sessione, perché la visita è un
  fatto della persona, non del cookie;
- la testata si ricarica ogni 3 secondi finché un sync gira e ogni 15 quando non succede niente.

**Numerazione delle migrazioni.** Nell'addendum il blocco 1 risultava senza migrazione e il numero
`0006` era assegnato all'anagrafica del blocco 3. Una colonna serve comunque, e i numeri seguono
l'ordine in cui le migrazioni vengono applicate, non l'ordine dei blocchi: l'anagrafica diventa
`0007_anagrafica`, e a scalare le successive.

### 9.5 — la modalità shadow

`[server].modalita = "shadow" | "produzione"`. In shadow il Cockpit legge il mondo e non lo tocca:
niente bozze, niente «segna letto», niente spostamenti di cartella, niente scritture sul NAS.
`apri_elemento_outlook` resta consentito, perché apre una finestra e non modifica niente.

Il blocco è in **due** punti, e servono entrambi:

- **all'accodamento**: quei job non entrano in coda, e chi ha premuto il pulsante se lo sente dire.
  La decisione dell'operatore però resta registrata: un documento confermato in shadow è confermato,
  ed è la *copia* che aspetta (`stato_nas = 'in_coda'`); una RFQ creata in shadow esiste, ed è la sua
  cartella sul NAS che aspetta;
- **al claim**: quei job non si eseguono **nemmeno se sono già in coda**. La coda sopravvive al cambio
  di modalità, e un `copia_nas` della settimana scorsa scriverebbe sul NAS vero appena un worker lo
  prende. Il blocco all'accodamento da solo è una porta chiusa con la finestra aperta.

All'avvio in shadow i job bloccati già in coda vengono **marcati** «in attesa di produzione»: restano
visibili in admin con scritto perché non partono. Al ritorno in produzione quelli marcati vengono
**annullati**: nulla che abbia aspettato durante la shadow si mette in moto da solo. Le copie che
servono davvero si rimettono in coda a mano, che è un gesto di una persona. Una copia accodata dopo
il ritorno in produzione non c'entra niente con la shadow e parte regolarmente: è il marcatore a
distinguerle, e senza marcatore non si annulla niente — altrimenti ogni riavvio in produzione
butterebbe via le copie in coda.

**Due rifiuti all'avvio.** In shadow `dry_run` è forzato a `true`, e il server **non parte** se
`[nas].radice` è (o sta sotto) una delle `[nas].radici_produzione` dichiarate: una prova in shadow
sul NAS vero non è una prova in shadow, è una prova sulla produzione con un badge rassicurante in
testata. Il confronto ignora maiuscole, barre e barra finale, perché sono i tre modi in cui lo stesso
percorso viene scritto passando da un file all'altro.

**Il default è shadow.** Un `cockpit.toml` scritto prima che questa opzione esistesse non contiene la
riga, e fra le due letture possibili di quel silenzio ce n'è una sola che si può correggere dopo. In
testata compare **SHADOW** con l'elenco di ciò che non succede e dove si cambia: un badge che avvisa
e basta lascia l'operatore a chiedersi se è rotto qualcosa.

### La correzione del 16/09/2026 — l'ora che Outlook consegna non è l'ora che dichiara

Il blocco 1 è stato provato sulle caselle vere il 16 settembre, ed è saltato fuori questo. Nel log del
server delle 08:52 i cursori di sync erano **due ore nel futuro** rispetto all'orologio del PC, e con
loro ogni `ricevuto_il` scritto in database.

`ReceivedTime` e `SentOn` dell'Object Model sono VT_DATE: un numero **senza fuso** che porta l'ora
locale del PC. pywin32 lo converte in un `pywintypes.datetime` e gli attacca `tzinfo` **UTC**, perché
è la convenzione della sua conversione, non perché quel valore sia in UTC. Ne esce un oggetto che
dichiara UTC e porta i numeri di Roma; `_utc` lo prendeva in parola con `timestamp()`, e le 10:52 di
Roma diventavano le 10:52 UTC, cioè le 12:52. Due ore avanti.

Non è un valore brutto in una colonna: è il **cursore**. Il cursore avanza su `ricevuto_il` (W2), e un
cursore nel futuro apre la finestra successiva dopo l'orologio. Da quel momento il sync gira, non trova
niente — non può — e non lo segnala nessuno: nessun errore, nessun conteggio strano, solo posta che non
arriva più. È lo stesso modo di rompersi del `Restrict` che perde un messaggio, ed è il motivo per cui
tutto il blocco 1 è costruito intorno a questa forma di guasto.

La correzione è in tre punti, e i tre non sono ridondanti:

1. **Nel worker** (`outlook_com._utc`): di una data che arriva da COM si prendono i **campi** e si
   leggono nel fuso del PC, con l'ora legale del giorno di quella data. Ciò che non viene da COM
   dichiara il fuso giusto e continua a convertirsi com'era: reinterpretare anche quello vorrebbe dire
   spostare valori corretti;
2. **All'ingresso** (`ingest`): un elemento con `ricevuto_il` oltre `now() + 5 minuti` non entra,
   finisce in `ingest_scarto` con il payload intero e il motivo scritto in chiaro, e **il cursore non
   avanza**. Cinque minuti perché fra due PC un po' di differenza è normale e non deve diventare posta
   scartata; due ore no. Quando l'ora torna plausibile l'elemento rientra dal sync successivo o dal
   replay, e lo scarto sparisce da solo;
3. **Quando si accoda il sync** (`jobs`): un cursore già in database e già nel futuro viene **ignorato**
   e quella cartella riparte dalla finestra predefinita. Senza questo, i cursori scritti il 16/09
   resterebbero lì a bloccare la casella fino a che il futuro non è passato, e la correzione del worker
   non basterebbe a farla ripartire.

La guardia lato server non è il posto dove si corregge il difetto: è il posto dove si smette di
crederci. Vale anche per un worker più vecchio, per un PC con l'orologio avanti e per il replay di uno
scarto di ieri.

### Come è stato verificato

| Prova | Che cosa mostra |
|---|---|
| `workers/test_ora_outlook.py` (6, L1/L2) | una data costruita come la costruisce pywin32 — `pywintypes.datetime` vero, campi dell'ora locale e `tzinfo` UTC — diventa l'istante giusto; un messaggio appena ricevuto non risulta nel futuro (il sintomo del log); l'offset è quello del giorno di quella data e non di oggi; un `datetime` con un fuso vero non viene spostato; e, passando da `_converti`, `ricevuto_il` e `data_evento` arrivano giusti anche per la Posta inviata |
| `internal/ingest` (3, L4) | un elemento nel futuro non entra, finisce in scarto con il motivo leggibile e il payload, e **il cursore resta fermo**; due minuti di differenza fra gli orologi non sono un problema; un cursore nel futuro non si scrive nemmeno quando i messaggi sono in regola, e il lotto dopo lo scrive regolarmente |
| `internal/jobs` (2, L4) | un cursore nel futuro già in database viene ignorato quando si accoda il sync, quello buono di un'altra cartella no; senza cursori utilizzabili la finestra torna quella predefinita |
| `workers/test_outlook_finestra.py` (13, L1/L2) | il filtro è DASL e in UTC anche partendo da un istante in `+02:00`; gli estremi si allargano al minuto; un `Restrict` che perde un messaggio viene smascherato dal self-test e **non** viene usato, e il ripiego li consegna tutti; una finestra vuota non conta come prova; l'ordine è crescente con tutti e due i modi |
| `internal/config` (4, L1) | senza `modalita` vale shadow; una parola scritta male ferma l'avvio; SH3 (b): sei radici, fra cui maiuscole, barra finale e barre al contrario; `dry_run` forzato |
| `internal/web` (4, L1) | SV2 puro: «in corso» solo su casella attiva, un sync in coda su una casella OFFLINE resta OFFLINE; il passo del poll; le parole di «Aggiorna ora»; l'avviso della shadow nomina ciò che non succede |
| `internal/jobs` (4, L4) | SH1: solo i cinque tipi che toccano il mondo non si accodano, i cinque di sola lettura sì; SH2: i job in coda non vengono claimati da nessuno per tutta la shadow e sono `annullato` con `tentativi = 0` al ritorno; SH3 (a): «Apri in Outlook» si esegue; un avvio in produzione senza shadow alle spalle non annulla niente |
| `internal/web` (3, L4) | SV1: dieci clic, un job per casella; SV2 dal vivo: `in corso…`, poi l'ora dell'ultimo sync, e il poll che si stringe e si allarga; SV3: conteggio, pallino, l'orologio che si sposta solo all'apertura e non al poll, e la visita di un utente che non azzera il contatore dell'altro |

Verificate anche **al contrario**, rimettendo il difetto e controllando che il test diventi rosso: il
filtro riscritto in ora locale (5 test rossi), il self-test che non boccia più (1), la chiave del
sync diversa a ogni clic (SV1), il claim che non esclude i tipi bloccati (SH2), la visita segnata
anche ai poll HTMX (SV3), la vecchia conversione delle date di Outlook (5 test rossi su 6) e le tre
guardie sul futuro tolte una per una (l'elemento entra, il cursore avanza, la casella resta ferma).

### Che cosa questo blocco NON dimostra

- **C2 e C3 sul profilo vero: NON ESEGUITI.** Qui Outlook non c'è: c'è una cartella finta che
  interpreta il filtro in UTC come lo interpreterebbe Outlook. È abbastanza per bocciare l'errore che
  conta (l'ora locale nel filtro), non per dire che Outlook si comporti così. Il comando è
  `python worker_outlook.py --restrict 7`, ed è L5.
- **Il tempo del sync**: che una casella vera scenda sotto i 5 secondi per giro è una misura da fare
  sul banco, non un'affermazione di questo blocco.
- **Che l'operatore se ne accorga**: che la chip, il pallino e il badge SHADOW si vedano davvero in un
  browser è L7.
- **`intervallo_sync_s` resta 0** nel `cockpit.toml` di sviluppo. Il valore 30 è nell'esempio e nel
  README; metterlo in funzione sulla posta vera è la prova concordata del blocco.
- **Che l'ora sia giusta su Outlook vero (TZ2): NON DIMOSTRATO.** Qui la data sbagliata è costruita a
  mano, e il fatto che pywin32 consegni VT_DATE in ora locale è documentato, non osservato su quel
  profilo. La prova è guardare la colonna **Ricevuto** in Outlook e la stessa riga in database: è una
  riga del registro reale, e si compila a mano.

## Blocco 2 dell'addendum (16/09/2026) — rete e VM (voci 2.4, 2.5, D22, D23)

Finora il Cockpit girava su un PC solo: server, worker e browser sullo stesso `127.0.0.1`. Il blocco 2
lo prepara a stare su una VM Linux con i worker sui PC degli operatori (D23), e questo cambia tre
domande che prima non si ponevano: **chi ascolta**, **chi sta chiamando** e **come si aggiunge un PC**.

### 2.4 — chi ascolta: il server non si espone in chiaro

`[server].indirizzo` fuori da loopback senza `tls_cert` = il server **non parte**. Su quel
collegamento passano la posta dell'azienda, il token di ogni worker e il cookie di sessione
dell'operatore; in chiaro su una LAN aziendale li legge chiunque sia attaccato allo stesso switch, e
non c'è niente che lo segnali. È il tipo di difetto che si scopre dopo, quando non c'è più niente da
fare. Le due uscite sono dichiarate nel file: `tls_cert`/`tls_key`, oppure `consenti_lan_in_chiaro`
per chi ha già un tunnel che cifra.

**Il certificato lo genera il server.** Se i due file non esistono, al primo avvio ne scrive uno
autofirmato per i nomi dichiarati (o per il nome host della macchina) e mette l'impronta sha256 nel
log e nella pagina *Postazioni*. Generare invece di fermarsi non è comodità: un server che chiede di
procurarsi un certificato prima di partire, su una macchina senza openssl, resta in chiaro — e il
percorso di minor resistenza deve essere quello cifrato.

Non c'è una CA interna e aspettarne una vorrebbe dire restare in chiaro, quindi i worker verificano
l'**impronta**, come fa SSH. È più stretto di una catena, non più largo: un impostore dovrebbe
presentare *quel* certificato, non un certificato qualunque firmato da qualcuno di cui il PC si fida.
Nel client la catena è spenta di proposito e il confronto è su sha256 del DER; un'impronta che non
corrisponde ferma il worker **prima** che mandi il token, che è l'unico errore irreparabile.

### 2.4 — chi sta chiamando: la credenziale è individuale

Il token condiviso autenticava «un worker», non «questo worker»: l'identità la dichiarava il JSON
(`worker_id`), e da lì venivano postazione e caselle autorizzate. Chiunque avesse letto un
`worker.toml` poteva presentarsi come il worker di un altro PC e farsi assegnare i suoi job
interattivi e la sua posta.

Ora il token **è** l'identità: il server ne cerca lo sha256 in `worker_credenziale` e la riga trovata
dice nome, tipo, postazione e caselle. Il `worker_id` del JSON resta nel contratto ma deve solo
coincidere — un `worker.toml` con il nome di un altro è un file copiato, e va detto invece che
assecondato (403). Lo stesso controllo vale su claim, battito, result, ingest e upload.

Due worker con lo **stesso** token ricevono 401 con i nomi di tutti e due: assegnare l'identità al
primo in ordine alfabetico funzionerebbe quasi sempre e sbaglierebbe senza dirlo. `[server].token_worker`
non autentica più niente; se è ancora nel file, il server lo dice a ogni avvio.

**Dove nasce il segreto.** Un `token` scritto in `[[worker]]` vince a ogni avvio: è il file la verità,
e va bene sul banco. Lasciarlo **vuoto** significa «il segreto lo tiene il database», e allora il seed
non lo tocca più: altrimenti il pacchetto scaricato ieri smetterebbe di funzionare stanotte senza che
nessuno abbia cambiato niente.

### 2.5 — CSRF

Il cookie di sessione viaggia da solo: una pagina qualsiasi aperta in un'altra scheda, mentre
l'operatore è collegato, può mandare un POST al Cockpit e il browser ci attacca il cookie. Con
«Conferma», «Apri in Outlook» e «Bozza» dietro a dei POST, basta questo a far succedere cose a nome
suo. `http.CrossOriginProtection` guarda `Sec-Fetch-Site` (e `Origin` contro `Host` per i browser che
non lo mandano) e blocca i metodi che scrivono quando sono dichiarati cross-site. Le letture passano:
bloccarle romperebbe i link senza proteggere niente.

Non è un token da mettere in ogni form, e per questo regge anche sulle pagine che verranno: non c'è
niente da ricordarsi di aggiungere. Il cookie prende `Secure` quando il server parla TLS — e solo
allora, perché metterlo sempre renderebbe impossibile il login in sviluppo, e la cura sarebbe toglierlo.

I worker non sono browser: non mandano `Sec-Fetch-Site` né `Origin` e passano. La loro autenticazione
è il token individuale, che una pagina esterna non ha.

### D22 — la pagina *Postazioni* e il pacchetto del worker

Aggiungere un PC voleva dire tre passaggi a mano — copiare il token condiviso, copiare i file del
worker, scrivere l'indirizzo del server — e ognuno fallisce in silenzio. Ora il pacchetto lo costruisce
il server, che è l'unico a sapere tutte e tre le cose: `cockpit-worker-<pc>.zip` con `worker.toml`
(indirizzo, impronta, i token di quel PC), le istruzioni e i file del worker **presi dal binario**,
così la versione del worker e quella del server non si allontanano.

I token si vedono una volta sola: in database c'è il loro sha256, e una schermata che sapesse
rileggerli sarebbe il posto da cui rubarli tutti insieme. Rigenerare invalida i precedenti, e il
pulsante lo dice prima di farlo.

**Che cosa la pagina NON fa, di proposito:** non crea postazioni né worker. Quale PC esiste e quali
caselle serve resta in `cockpit.toml` — versionato, leggibile, uguale a ogni avvio. Una schermata che
crea autorizzazioni è una schermata da cui si dà accesso alla posta di un collega con due clic e
nessuna traccia nel file.

### D23 — il server su una VM Linux

`GOOS=linux go build ./cmd/cockpit` produce un binario unico, con dentro migrazioni, template e i file
dei worker. Il NAS diventa una share SMB montata (`/mnt/nas/...`) e il confronto con
`radici_produzione` — quello che impedisce a una shadow di partire sul NAS vero — ignora maiuscole,
barre e barra finale, quindi vale anche scritto alla maniera POSIX. Con la VPN fra gli impianti serve
**una sola regola di firewall**, in ingresso sulla VM: i worker si collegano in uscita e il server non
chiama nessuno.

### Come è stato verificato

| Prova | Che cosa mostra |
|---|---|
| `internal/config` (7, L1) | `SuLoopback` su nove forme di indirizzo; il server non parte in chiaro fuori da questo PC e il rifiuto nomina tutte e due le uscite; su loopback in chiaro parte; mezzo TLS (solo cert o solo key) non esiste; i percorsi relativi si leggono da dove sta il file; SH3 (b) con percorsi POSIX, cioè sulla VM |
| `internal/workerapi` (3, L4+L2) | W4: senza header, con header vuoto e con un token sconosciuto è 401 con il motivo; con il token giusto si entra. Un token di due worker è 401 che li nomina tutti e due. W11: il **client Python vero** contro un server TLS vero — con l'impronta giusta passa (401 dal server: il TLS ha retto), con una diversa si ferma e non manda il token, e l'impronta in maiuscolo con i due punti vale come quella minuscola; su un listener TLS una richiesta in chiaro non arriva a nessun gestore |
| `internal/workerapi` (claim, L4) | la credenziale di un worker non permette di presentarsi come un altro (403), né di chiedere le caselle di un altro |
| `internal/web` (5, L4) | W10: lo stesso POST passa dalla stessa origine e prende 403 dichiarato cross-site (con `Sec-Fetch-Site` e con `Origin`), mentre una GET cross-site passa; il claim di un worker non viene toccato dalla protezione. PK1: il pacchetto è uno zip con `worker.toml`, le istruzioni e i file del worker (senza i test), il token dentro è **davvero** la credenziale di quel worker e il claim con quello passa; rigenerare produce un token diverso e il precedente diventa 401; un operatore non amministratore riceve 403 e nessun token cambia |
| `workers/test_credenziali.py` (9, L1/L2) | ogni worker legge il proprio token dalla sua tabella, il token dell'altro non arriva; un `worker.toml` vecchio si legge ancora; l'ambiente vince sul file; senza token il worker dice dove prenderlo; l'impronta si normalizza (due punti, maiuscole), mezza impronta non passa, e un'impronta senza https è un errore |
| `scripts\prova-tutto.ps1` | `GOOS=linux go build ./...`: il server compila per la VM |

Verificate anche **al contrario**, rimettendo il difetto:

| Difetto rimesso | Effetto |
|---|---|
| `auth` che accetta qualunque token non vuoto | rossi i due test W4 |
| il claim che non confronta il nome dichiarato con la credenziale | rosso `TestClaimRifiuta…` (un worker si presenta come un altro e ottiene un job) e rosso `TestCaselleWorker…` (si fa dare le caselle di un altro) |
| il banco senza `ProtezioneCSRF` | rosso W10 su tutti e due i casi |
| il client che non confronta più l'impronta | rosso W11 (accetta un certificato qualunque) |
| il controllo del chiaro in LAN tolto dalla configurazione | rosso `TestInChiaroFuoriDaQuestoPC…` |
| il pacchetto che non rigenera il token | rosso PK1 |

### Che cosa questo blocco NON dimostra

- **Il banco a due PC: NON ESEGUITO.** Qui client e server sono due processi sullo stesso computer, su
  loopback. Che il worker di un altro PC faccia claim attraverso la LAN, che «Apri in Outlook» apra la
  finestra **su quel PC** e che un allegato da decine di MB passi nei tempi del lease è M11/M7 **(2PC)**;
- **il NAS su una share SMB montata su Linux (N1): NON ESEGUITO.** Il codice compila per Linux e i
  percorsi POSIX sono coperti da un test, ma la copia verificata — hash uguale, nessun `.parte`
  rimasto — attraverso `cifs` non è stata eseguita. È L8;
- **la VM: NON ESISTE ancora.** `GOOS=linux go build` dice che compila, non che gira: PostgreSQL, il
  montaggio del NAS e la regola di firewall sono lavoro d'infrastruttura, e vanno nella checklist IT;
- **il browser: NON PROVATO.** Che un certificato autofirmato dia l'avviso atteso, e che
  l'eccezione basti, si vede solo aprendo il Cockpit da un altro PC (L7).

### La correzione del 16/09/2026 — dal token condiviso alle credenziali individuali

Il blocco 2 è stato messo sul banco vero subito dopo, ed è uscito il passaggio che nessun test
copriva: non la sicurezza del meccanismo, ma **come ci si arriva** da ciò che c'era prima.

In database c'erano i due worker di quella postazione — Outlook e analisi — con la **stessa impronta**: il token
unico di prima, copiato in tutti i `[[worker]]`. La mossa giusta — svuotare i `token` in
`cockpit.toml` — è stata fatta, e **non è successo niente**: il seed non riscrive un token che il file
non dichiara (è la regola che impedisce al pacchetto scaricato ieri di morire stanotte), quindi le due
impronte duplicate sono rimaste dov'erano e il vecchio `worker.toml` continuava a prendere `401`.

Il comportamento del server è corretto e non va toccato: **`worker_id` non disambigua due worker che
presentano lo stesso token**. Se lo facesse, il segreto smetterebbe di essere l'identità e tornerebbe
a essere un lasciapassare, che è esattamente il difetto che la voce 2.4 ha chiuso. Quello che mancava
è tutto intorno.

**Tre cose, e una quarta trovata per strada.**

1. **Una credenziale senza segreto adesso si riconosce.** Alla prima creazione con `token = ""` il
   seed scriveva un hash *casuale*: nessuno poteva presentarlo — giusto — ma era indistinguibile da un
   segreto vero, quindi nessuno poteva nemmeno dire «questa credenziale è da generare». Ora ci va un
   segnaposto dichiarato (`rete.ImprontaNonGenerata`, lo sha256 della stringa vuota): resta
   impresentabile — un header vuoto è già «credenziale mancante», prima ancora del calcolo — e in più
   è **leggibile** da chi guarda.
2. **La pagina *Postazioni* dice lo stato.** In cima, i PC da sistemare; accanto a ogni worker,
   `credenziale da generare` oppure `token condiviso con …`; e il pulsante diventa **«Rigenera
   credenziali della postazione»**. Lo stesso conto lo fa il server all'avvio e lo scrive nel log: un
   difetto che si scopre soltanto dal primo `401` di un worker è un difetto che si scopre nel momento
   peggiore, cioè mentre si sta provando altro.
3. **Il log del worker non mente più sul motivo.** `except (ErroreHTTP, *ERRORI_RETE)` scriveva
   *«server non raggiungibile»* per qualunque cosa, compresi i `401` e i `403` — proprio i casi in cui
   il server risponde benissimo e sta dicendo qualcosa di preciso. Sul banco si leggeva
   `server non raggiungibile (POST /api/v1/jobs/claim → 401 …)` e si andava a cercare la rete. Ora la
   classificazione è una funzione sola (`cockpit_client.diagnosi`), uguale per i due worker: rete,
   credenziale (401), autorizzazione (403), tentativo (409), server (5xx), richiesta (4xx),
   certificato. Ciò che non si risolve aspettando viene scritto come **errore** e non fa crescere il
   backoff: si ripete ogni 30 secondi, perché chi corregge la configurazione deve trovare il worker
   ancora vivo.
4. **`scripts\azzera-dati.ps1` diceva di aver azzerato senza azzerare.** Il DSN era il primo argomento
   posizionale di `psql`, che da lì in poi **ignora le opzioni** — `-c` compreso — avvisa su stderr ed
   esce con codice 0. Lo script scriveva «schema public ricreato» e le 43 tabelle erano ancora al loro
   posto. Ora il DSN passa da `-d` e, dopo il drop, lo script **conta le tabelle** e fallisce se non
   sono zero: un azzeramento che riesce senza azzerare è peggio di uno che non c'è.

### Come è stato verificato

| Prova | Che cosa mostra |
|---|---|
| `internal/web` — **PK2** (L4) | l'intero percorso di migrazione in un test solo: due worker sulla stessa postazione con lo stesso token → `401` per tutti e due; svuotati i `token` nel file, le due impronte duplicate **restano** in database e il `401` pure; la pagina segnala lo stato; rigenerato il pacchetto, i due token sono **diversi**, ognuno identifica **una** credenziale, il vecchio token è `401`, il token dell'uno sull'identità dell'altro è `403`, e i due worker fanno claim e `GET /worker/caselle` con le proprie caselle e non con quelle di Luigi; **riavviato il server con i token ancora vuoti, le credenziali appena generate valgono ancora** e la pagina non segnala più niente |
| `internal/web` — credenziale mai generata (L4) | il segnaposto è in database, il claim senza token e con un token di soli spazi è `401`, la pagina lo dice, e dopo il pacchetto quel worker lavora |
| `internal/web` — `StatoCredenziali` (L1) | token condiviso riconosciuto **con i nomi**, credenziale da generare riconosciuta, e una credenziale sana non viene dichiarata guasta perché un worker **disattivato** ha lo stesso token |
| `workers/test_credenziali.py` (8 nuove, L1/L2) | le sei categorie di errore, una per una; e il **ciclo vero** del worker che riceve `401`: lo registra come *errore*, non dice «non raggiungibile», nomina la pagina *Postazioni* e aspetta 30 secondi fissi invece di raddoppiare |

Verificate anche **al contrario**, rimettendo il difetto: il seed che riscrive il token anche quando
il file lo lascia vuoto → PK2 rosso («svuotare i token nel file ha cambiato i segreti in database»);
il pacchetto che genera **un** token per tutta la postazione → PK2 rosso («lo STESSO token ai due
worker»); la diagnosi che non guarda i duplicati → PK2 rosso su tutte e tre le frasi della pagina, e
rosso il test L1; la vecchia riga di log del worker → rosso il test del ciclo, con nel log catturato
esattamente il sintomo del banco: `server non raggiungibile (… → 401 …)`.

Un controllo **non** è verificabile dall'esterno, e va detto: il rifiuto del segnaposto dentro `auth`
è irraggiungibile per costruzione (nessuna richiesta può produrre lo sha256 della stringa vuota,
perché l'header vuoto viene scartato prima). Resta scritto come guardia — se un domani quella
funzione cambia forma, il segnaposto continua a non far entrare nessuno — ma è una dichiarazione, non
un comportamento provato.

### Provato sul banco (non simulato)

Database di sviluppo **azzerato e ricostruito** dalle sei migrazioni, con le fondazioni riseminate da
`cockpit.toml`: 2 caselle, 1 postazione, 2 worker, entrambi con il segnaposto. All'avvio il server ha
scritto *«credenziali da rigenerare … dove=/admin/postazioni»*; la pagina mostrava «Credenziali da
rigenerare», «credenziale da generare» e il pulsante «Rigenera credenziali della postazione»; il
pacchetto scaricato conteneva 8 file e **due token diversi**, uno per worker; dopo la generazione la
pagina non segnalava più niente. Modalità **shadow** e `intervallo_sync_s = 0` per tutto il tempo.

Restano **non eseguiti** i passi che toccano la posta: avvio del worker Outlook con la credenziale
nuova, «Aggiorna ora» su Francesco e Commerciale, i due job a `fatto`, l'Inbox aggiornata e l'avvio
del worker di analisi con la sua credenziale. Sono righe del registro reale.

## Checkpoint UI/RBAC del 16/09/2026 — l'interfaccia di lavoro e quella di amministrazione (6.9, 6.4)

Non è lavoro nuovo: sono la voce **6.9** (ruoli, test W1) e la voce **6.4** (seed non distruttivo e
cambio password, test W12), anticipate prima del blocco 3 perché il blocco 3 aggiunge schermate, e
ogni schermata aggiunta senza una regola è una schermata da ricontrollare dopo.

### Il problema

Le sette rotte `/admin/*` avevano **solo** `autenticato`. L'unico confronto con `admin` in tutto il
server stava dentro la generazione del pacchetto. Un operatore che scriveva `/admin/job` nella barra
degli indirizzi entrava, riaccodava e annullava; `/admin/scarti` idem. La barra di navigazione le
mostrava a tutti, il che rendeva la cosa un invito.

E il seed degli utenti faceva due cose in silenzio: un `ruolo` che non riconosceva diventava
`operatore`, e una `password` scritta nel TOML veniva rihashata e riscritta **a ogni avvio** — cioè
il cambio password, quando arriverà, sarebbe tornato indietro da solo la notte dopo.

### Che cosa cambia

| | Prima | Ora |
|---|---|---|
| le sette rotte `/admin/*` | `autenticato` | `soloAdmin`, cioè autenticato **e** ruolo, montato dove si montano le rotte |
| chi decide il permesso | `u.Ruolo == admin` dentro un gestore | una funzione sola (`almeno`), usata dal wrapper **e** dalla barra: la voce che si vede e la porta che si apre non possono allontanarsi |
| barra di navigazione | tre voci tecniche a tutti | solo a chi le può aprire. La testata (stato delle caselle, «Aggiorna ora») resta a tutti: serve a lavorare |
| il 403 | testo grezzo | frammento HTMX dentro la pagina, pagina intera con la via d'uscita se l'indirizzo è scritto a mano; in tutti e due i casi dice quale ruolo serve, e finisce nel log |
| `consultazione` | poteva tutto ciò che poteva un operatore | nessun metodo che scrive, **per tutte** le rotte dietro all'autenticazione, comprese quelle che non esistono ancora |
| ruolo sconosciuto in `cockpit.toml` | diventava `operatore` | il server **non parte**: dice quale utente, che cosa c'era scritto e quali parole sono ammesse |
| nessun `admin` fra gli `[[utenti]]` | partiva, e *Postazioni* non la apriva più nessuno | il server **non parte** (aggiunto il 16/09 dopo il test manuale: la regola era già scritta nel README e non era imposta) |
| `password` nel TOML | riscritta a ogni avvio | fa **nascere** l'utente; se in database c'è un hash bcrypt valido, vince quello — e chi scrive una password nuova nel file lo legge nel log |
| sessione senza postazione | restava tale fino al logout | si riabbina appena un worker di quel PC fa claim dallo stesso IP (W14 esteso) |

### I quattro ruoli, e perché la matrice è rimandata

`consultazione < operatore < tecnico < admin`. Sono i quattro dell'enum `ruolo_utente`, che esiste
dalla migrazione `0001`: **nessuna migrazione**, qui, e nessun ruolo nuovo.

Sono definiti tutti e quattro adesso anche se oggi se ne usano due, perché la definizione mancante è
ciò che fa entrare l'ufficio tecnico come «operatore» fra un mese, per poi doverlo cambiare. Oggi
`tecnico` può esattamente quanto `operatore`: le azioni della fattibilità e dell'albero, che saranno
sue, non esistono ancora.

La **matrice ruolo × azione** — quale singola azione appartiene a chi — è rimandata di proposito, con
gli utenti ancora da configurare e metà delle schermate da scrivere. Quello che c'è è un **ordine**,
in un file solo (`internal/web/ruoli.go`), e il segno che non basterà più sarà un ruolo che può
qualcosa che il ruolo sopra di lui non può. Fino ad allora una tabella scritta oggi sarebbe una
tabella da riscrivere.

**Ruolo e visibilità per casella restano due cose diverse.** Il ruolo dice *che cosa puoi fare*; la
visibilità della posta personale altrui (D12, voce 2.15, default restrittivo) dice *che cosa puoi
vedere*, e non è questo checkpoint. Un amministratore vede la coda dei job e le postazioni: non per
questo vede la Posta in arrivo di un collega. Quando arriverà, `puoVedere` resterà l'unico punto di
decisione su quello.

### Il browser aperto prima che il worker si accenda (W14 esteso)

È l'ordine normale di una mattina: prima il Cockpit, poi il worker. Al login l'IP non corrispondeva a
nessuna postazione, la sessione nasceva senza, e restava così per dodici ore — con le azioni su
Outlook spente — anche molto dopo che il worker si era acceso. Uscire e rientrare funzionava, ma
nessuno sa che è quello il rimedio.

Ora la sessione si riabbina alla prima richiesta utile. L'autorizzazione non cambia di una virgola:
passa da `abbinaPostazione`, cioè da `puoUsarePostazione` (P1). L'IP dice **quale** postazione, mai
che si possa usarla. E una postazione **scelta** in testata non viene mai sovrascritta: quella è una
decisione, e una schermata che disfa al poll successivo la decisione appena presa è peggio di una che
non aiuta.

**Limite dichiarato:** il «—» della testata vuol dire «non lo so», non «non voglio». Riporta la
sessione allo stato di partenza, e da un PC che ha il suo worker acceso l'IP torna a dirlo. Chi vuole
lavorare da un'altra parte **sceglie** quell'altra postazione. Distinguere le due cose in database
vorrebbe dire un terzo valore in `postazione_origine`, che il `CHECK` della `0005` non ammette: una
migrazione per questo, oggi, sposterebbe la numerazione del blocco 3.

### Come è stato verificato

| Prova | Che cosa mostra |
|---|---|
| `internal/web` — **W1** (L4) | tutte e sette le rotte `/admin/*`, in GET e in POST, rispondono 403 a un operatore — che è anche il proprietario di quel PC —, il 403 è un frammento leggibile, e **niente è successo**: il job che aveva accodato è ancora `pronto` e lo scarto ha ancora un tentativo. L'altra metà: l'admin le apre tutte e sette, e «Annulla» annulla davvero |
| `internal/web` — **W15** (L4 + L1) | dal vivo: l'operatore non trova nella barra `/admin/job`, `/admin/scarti`, `/admin/postazioni`, e trova tutto il resto; l'admin le trova. In L1, il template con e senza il ruolo |
| `internal/web` — **W16** (L4) | `consultazione` legge Inbox, Cruscotto e testata, e riceve 403 su ogni POST provato, senza accodare niente; ma può uscire |
| `internal/web` — **W12** (L4) | riavvio con una password **diversa** nel file: entra ancora quella vecchia, la nuova no, e il log lo dice. Un utente nuovo nasce regolarmente con la sua; nome, ufficio e ruolo restano allineati al file. E il seed rifiuta un ruolo inventato invece di declassarlo |
| `internal/config` — **CF1** (L1) | ruolo inventato, mancante e vuoto: tre rifiuti all'avvio, ognuno con la sigla e i quattro ruoli ammessi. I quattro si configurano, `« Operatore »` si normalizza, `ufficio` no, e la stessa sigla due volte non passa |
| `internal/web` — **W14 (d)** (L4) | la sessione nata senza postazione si riabbina appena il worker fa claim da quel IP, e «Apri» parte; una postazione scelta non si sposta nemmeno dopo tre poll della testata |
| `internal/web` — ordine dei ruoli (L1) | admin > tecnico ≥ operatore > consultazione > un ruolo che non conosciamo; un utente assente non supera nessun controllo; e i metodi che scrivono sono gli stessi che la protezione CSRF chiama non sicuri |

Verificate anche **al contrario**, rimettendo il difetto uno per volta:

| Difetto rimesso | Effetto |
|---|---|
| le rotte `/admin` di nuovo solo `autenticato` | rosso W1 |
| la barra che mostra le voci tecniche a tutti | rosso W15 |
| `consultazione` che scrive come un operatore | rosso W16 |
| il seed che riscrive la password a ogni avvio | rosso W12 |
| il ruolo sconosciuto che torna a diventare `operatore` | rosso CF1 |
| il riabbinamento della sessione tolto | rosso W14 (d) |

### Che cosa questo checkpoint NON dimostra

- ~~il browser~~: **PROVATO** il 16/09/2026 (RB1, registro reale). Con `cockpit.toml` portato ai due
  utenti previsti — PS `admin`, FP `operatore` — e il server riavviato, il RBAC si comporta come
  descritto. I singoli passi non sono stati trascritti uno per uno: nel registro resta ciò che è stato
  riferito;
- **la visibilità per casella (D12, voce 2.15): fuori perimetro.** Resta la regola restrittiva: un
  ruolo alto non amplia la visibilità della posta personale;
- **il cambio password dalla UI (voce 6.4): NON FATTO.** Qui c'è solo la metà che protegge il
  segreto già impostato. Finché `/profilo/password` non esiste, reimpostare una password vuol dire
  azzerare `utente.password_hash` a mano;
- **la matrice ruolo × azione: rimandata**, come sopra.

## Checkpoint sync del 16/09/2026 — il primo caricamento non è l'archivio (voce 2.8)

Le prove reali su `4e2f68b` sono andate a buon fine (M1, C2/C3, TZ2, CR1: sono righe di
`esiti_reali.md`), e hanno reso visibile una cosa che prima era solo un numero in un file: **quanta
posta si chiede al primo giro**, e quanta se ne chiede quando si vuole risalire l'archivio.

### Il problema

Due lavori diversi condividevano un meccanismo solo, e le loro finestre erano tarate sulla misura
sbagliata.

- **Il primo sync** di una casella partiva da **un mese fa**. Il `Restrict` della voce 2.9 ha reso
  veloce l'**enumerazione** — sul profilo vero, il 16/09, da qualche decimo di secondo a circa un
  secondo per cartella — ma l'enumerazione non è l'ingest: corpo, destinatari e allegati di ogni
  elemento sono un altro ordine di grandezza, e il 15/09 una singola finestra di Commerciale erano
  ~100 secondi per 566 elementi. Un mese su una casella viva è il worker occupato per ore.
- **«Carica precedenti»** accodava **trenta giorni** per clic, cioè lo stesso lavoro moltiplicato, e
  la schermata prometteva «premi di nuovo per il mese precedente».

Perché questo pesi più di quanto sembri serve una cosa sola, ed è la forma del worker: **il worker
Outlook è seriale, e ce n'è uno per PC**. Finché macina un job storico non prende «Apri in Outlook»,
non scarica un allegato e non fa il sync ordinario. Chi guarda la schermata non vede un lavoro in
corso: vede dei pulsanti che non fanno niente, per un tempo che nessuno sa dire in anticipo.

E il job storico stava a **priorità 2**, cioè davanti al sync ordinario e **alla pari con «segna
letto»** — a parità vince il job più vecchio, che è sempre l'archivio, perché è in coda da prima.

Una quarta cosa, trovata scrivendo il test delle due finestre consecutive: **`SetStoricoFinoA` era un
`UPDATE`**. La riga di `sync_cursore` nasce con il primo sync *ordinario*; una casella da cui si carica
l'archivio prima di averla mai sincronizzata quella riga non ce l'ha, l'`UPDATE` non scriveva niente e
non lo diceva nessuno. Il clic successivo ricalcolava **la stessa identica finestra**, all'infinito.
Con trenta giorni era lavoro sprecato; con due giorni sarebbe stato un pulsante che non avanza mai.

### Che cosa cambia

| | Prima | Ora |
|---|---|---|
| primo sync di una casella | un mese fa | `[outlook].giorni_sync_iniziale`, **7** di default |
| `dal` | «la data minima al primo avvio» | **override esplicito** per gli import controllati; scritto male, il server non parte |
| ripiego per una cartella senza cursore | il più vecchio dei cursori delle **altre** cartelle | la finestra iniziale |
| quando si calcola la finestra | una volta all'avvio del server | a ogni accodamento |
| «Carica precedenti» | 30 giorni per clic | **2 giorni** per clic e per casella |
| priorità del sync storico | 2 (davanti al sync ordinario, alla pari con «segna letto») | **9**, il fondo della coda |
| `storico_fino_a` | `UPDATE`: sulle caselle mai sincronizzate non scriveva | `INSERT … ON CONFLICT`: la riga nasce quando serve |
| la schermata | «premi di nuovo per il mese precedente» | dice due giorni, e il perché sta nel tooltip |

### La precedenza, in un posto solo

Per ogni **(casella, cartella)**, e in quest'ordine:

1. il **cursore** di quella cartella, quando c'è ed è utilizzabile: **vince sempre**. Viaggia nel
   payload per cartella ed è il worker ad applicarlo, meno la sovrapposizione;
2. **`dal`**, se `[outlook].dal` è scritto nel file: override esplicito;
3. altrimenti la **finestra iniziale**, `adesso - giorni_sync_iniziale`.

I punti 2 e 3 valgono **solo** per le cartelle che un cursore non ce l'hanno. Da qui segue la
proprietà che conta: **un riavvio non riporta nessuna casella alla finestra iniziale**, perché il
cursore sta in database e non nel processo. È il difetto che, se ci fosse, non si vedrebbe: nessun
errore, nessun buco, solo una settimana riletta a ogni avvio e la deduplica per Message-ID a
nascondere il sintomo lasciando il costo.

Il vecchio ripiego — il più vecchio dei cursori delle altre cartelle — sembra prudente e non lo è.
Una cartella aggiunta oggi a una casella sincronizzata da mesi sarebbe ripartita da mesi fa, cioè dal
caricamento lungo che questa voce esiste per evitare; e una cartella il cui cursore è stato scartato
perché nel futuro (la correzione del fuso, qui sopra) sarebbe ripartita dal cursore di un'altra, che
su dove fosse arrivata lei non dice niente.

`dal` ora è validato all'avvio. Prima un `dal = "01/09/2026"` veniva scartato senza una riga da
nessuna parte, e chi credeva di stare importando settembre importava la finestra predefinita: se ne
sarebbe accorto dalle mail che mancavano, che è il modo peggiore.

### Due giorni, e nessuna catena

Ogni clic su «Carica precedenti» copre **due giorni per casella**, a partire da dove era arrivato il
clic precedente (`storico_fino_a`), e le finestre si incastrano esatte: il limite superiore della
seconda è il limite inferiore della prima. Finché il job storico di una casella è in coda o in corso,
premere ancora **non accoda niente** e riporta il badge di quello che sta girando.

Un job storico **non ne accoda un altro**, di proposito. Una catena costruita da sola occuperebbe il
worker per settimane di archivio senza che nessuno l'abbia chiesto: è il difetto di prima con un
vestito nuovo. Chi vuole risalire preme ancora, e fra un pezzo e l'altro il worker torna al claim.

Le finestre piccole da sole non basterebbero: se al claim l'archivio avesse la precedenza, il
pulsante resterebbe muto lo stesso. Per questo il sync storico è l'**ultima** priorità della coda,
dietro anche al sync ordinario — 1 e 2 restano di chi sta davanti allo schermo.

### Come è stato verificato

| Prova | Che cosa mostra |
|---|---|
| `internal/jobs` — **SI1, SI2** (L4) | una casella mai sincronizzata parte da sette giorni e da nient'altro (nessun cursore nel payload, nessun limite superiore); la finestra si configura |
| `internal/jobs` — **SI3** (L4) | il cursore dell'Inbox viaggia nel payload; il worker lo usa e il server lo ritrova **dopo il riavvio** (Queries nuove, stesse opzioni del file); la Posta inviata, che non ha cursore, resta alla finestra iniziale |
| `internal/jobs` — **SI4** (L4) | `dal` scritto nel file vale come limite inferiore e **non** cancella il cursore di chi ce l'ha |
| `internal/jobs` — cursore nel futuro (L4, aggiornato) | l'Inbox con il cursore scartato riparte dalla finestra iniziale e **non** dal cursore della Posta inviata |
| `internal/web` — **SS1, SS2** (L4) | un clic = una finestra di **48 ore esatte** per casella; due clic = due finestre contigue, senza buchi né sovrapposizioni oltre il microsecondo che PostgreSQL arrotonda |
| `internal/web` — **SS3** (L4) | cinque clic mentre il job gira accodano un job per casella, non cinque |
| `internal/web` — **SS4** (L4) | tre claim di fila consegnano «Apri in Outlook», poi «segna letto», poi l'archivio |
| `internal/config` (L1) | la riga assente non porta un 7 di riserva nel file (il numero sta in un posto solo); si configura; negativa ferma l'avvio; `dal` scritto in tre modi sbagliati ferma l'avvio e dice come si scrive; uno valido si legge a mezzanotte locale |
| `workers/test_worker_ciclo.py` (L1/L2) | è **qui** che la precedenza si applica: la cartella con il cursore riparte da lì meno la sovrapposizione, quella senza usa il ripiego del payload; e un job storico non sposta il cursore in avanti, né nel risultato né nei lotti |

Verificate anche **al contrario**, rimettendo il difetto uno per volta — dodici, tutti rossi:

| Difetto rimesso | Effetto |
|---|---|
| la finestra iniziale torna a un mese | rosso SI1 |
| `giorni_sync_iniziale` del file ignorato | rosso SI2 |
| il cursore non viaggia nel payload | rosso SI3 |
| `dal` non fa più da override | rosso SI4 |
| «Carica precedenti» torna a 30 giorni | rosso SS1 |
| `storico_fino_a` torna un `UPDATE` | rosso SS2 |
| la chiave dello storico porta la finestra (un clic, un job) | rosso SS3 |
| il sync storico torna a priorità 2 | rosso SS4 |
| il worker usa `dal` anche dove ha il cursore | rosso il test del worker |
| il sync storico muove il cursore in avanti | rosso il test del worker |
| una finestra iniziale negativa non ferma l'avvio | rosso il test di configurazione |
| un `dal` scritto male torna a passare in silenzio | rosso il test di configurazione |

Due controprove, al primo giro, sono uscite **verdi**, ed è il motivo per cui si fanno: SS1
confrontava l'ampiezza della finestra con `GiorniStorico`, cioè con la costante che avrebbe dovuto
sorvegliare — portata a 30, si spostava anche l'attesa; e SS4 provava il solo «Apri in Outlook»
(priorità 1), che passava davanti all'archivio **anche** con il difetto. Ora il numero due è scritto
a mano nel test, e SS4 prova il caso che discrimina, cioè «segna letto».

### Che cosa questo checkpoint NON dimostra

- **Che sette giorni siano la misura giusta: NON MISURATO.** I tempi del 16/09 (`--restrict 7`:
  decimi di secondo, fino a circa un secondo sulla casella più popolata) sono la **sola
  enumerazione**. Il costo dell'ingest completo — corpo e allegati — è un'altra grandezza, e si vede
  al primo caricamento vero su una casella viva;
- **che due giorni per clic siano comodi da usare: L7.** Che il badge dica la finestra giusta e che
  premere più volte non confonda si vede in un browser;
- **la cartella che fallisce in un sync storico**: `storico_fino_a` avanza solo per le cartelle
  andate bene, e `al` è il minimo fra quelle che un valore ce l'hanno. Una cartella che fallisce
  sempre dal primo clic resta indietro in silenzio. È un limite dichiarato, non un difetto nuovo: il
  rimedio è la stessa finestra ripresa, ma nessuno oggi lo segnala.

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
| L1/L2 + L4 blocco 1 | voce 2.9: filtro DASL in UTC e self-test per insieme (13 prove L1/L2); voce 2.16: SV1, SV2, SV3; voce 9.5: SH1, SH2, SH3 (a) in L4, SH3 (b) in L1; correzione del fuso del 16/09: 6 prove L1/L2 sulla conversione e 5 L4 sulle guardie (elemento, cursore del lotto, cursore già in database) | eseguiti |
| L1 + L4 blocco 2 | voce 2.4: `SuLoopback`, il rifiuto del chiaro in LAN, mezzo TLS, percorsi relativi, SH3 (b) POSIX (7 L1); W4 e il claim che non accetta il nome di un altro; **W11 con il client Python vero contro un server TLS vero**; voce 2.5: W10 (403 cross-site, 200 stessa origine, GET libera, worker non toccati); D22: PK1, il pacchetto e la rotazione del token; 9 prove L1/L2 sulle credenziali lato worker | eseguiti |
| L1 + L4 correzione del 16/09 (credenziali) | **PK2**: token condiviso → token vuoti → rigenerazione → due credenziali individuali, con il riavvio che non le sovrascrive; la credenziale mai generata e il suo segnaposto; `StatoCredenziali` (L1); 8 prove L1/L2 sul log del worker (401 ≠ «server non raggiungibile») | eseguiti |
| L1 + L4 checkpoint UI/RBAC | **W1** (sette rotte /admin, GET e POST, e nessun effetto), **W15** (barra per ruolo, dal vivo e nel template), **W16** (`consultazione` non scrive), **W12** (la password del file non sostituisce quella in database), **CF1** (ruolo sconosciuto → il server non parte), **W14 (d)** (riabbinamento della sessione dopo il claim), ordine dei ruoli (L1) | eseguiti |
| L1 + L4 checkpoint sync | **SI1–SI4** (finestra iniziale, cursore che vince, riavvio che non riporta indietro, `dal` come override), **SS1–SS4** («Carica precedenti» a 48 ore esatte, due finestre contigue, nessun doppione, l'archivio in fondo alla coda), la configurazione (L1) e due prove sul worker (L1/L2: il cursore vince sul ripiego, lo storico non muove il cursore) | eseguiti |
| L5–L9 — eseguite il 16/09 | **RB1** (i due ruoli in un browser vero), **M1** (due caselle vere risolte, la terza ignorata), **C2/C3** (`--restrict 7` sulle due caselle e su Posta in arrivo e Posta inviata: nessun elemento perso), **TZ2** (l'ora di Outlook accanto a quella in UI), **CR1** (worker Outlook con la credenziale nuova: «Aggiorna ora» su due caselle, i due job a `fatto`, l'Inbox aggiornata) e la metà di **CR2** che riguarda le credenziali | **passate**, righe di `esiti_reali.md` |
| L5–L9 — aperte | casella condivisa Exchange (I9, D5), **banco a due PC** (M11, upload fra due PC), **NAS su share SMB montata da Linux** (N1), postazione della sessione da un browser vero (W14, L7), il worker di analisi che esegue un `analizza_allegato` vero | **non eseguite** |

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

Con l'addendum del 16/09/2026 l'ordine non è più quello dei numeri delle voci ma quello dei blocchi:

- ~~**blocco 2 — rete e VM**~~: **fatto**, compresa la correzione del 16/09 sul passaggio dal token
  condiviso. Restano le prove reali che nessun test simulato può dare: il worker Outlook che lavora
  con la credenziale nuova sulla posta vera (CR1), il banco a due PC, il NAS su una share SMB montata
  dalla VM Linux (N1) e il browser davanti a un certificato autofirmato;
- ~~**checkpoint UI/RBAC**~~: **fatto** (6.9) e **provato in un browser vero** il 16/09 (RB1). Della
  6.4 resta la schermata `/profilo/password`: finché non c'è, reimpostare una password vuol dire
  azzerare `utente.password_hash` a mano. Resta fuori la visibilità per casella (D12, voce 2.15), che
  è un'altra domanda e vive nei blocchi successivi;
- ~~**checkpoint sync**~~: **fatto** (voce 2.8). Resta da misurare sul banco quanto costa davvero il
  primo caricamento di sette giorni con corpo e allegati: i tempi del 16/09 sono della sola
  enumerazione;
- **latenza al cambio casella nell'Inbox** — osservata sul banco vero il 16/09, **da chiudere nel
  blocco 9 / UI definitiva**. Non serve introdurre HTMX sulla lista: **c'è già**, il filtro casella e
  i filtri dell'Inbox fanno già uno swap su `#lista`. Quello che manca è il profilo del percorso
  server/database del refresh: `ListInbox`, `ContaInbox`, il calcolo delle novità, la vista
  `v_inbox` e gli indici delle tabelle sotto, con `EXPLAIN ANALYZE` su un volume realistico, per
  ridurre il lavoro rifatto a ogni cambio casella — e semmai separare il refresh dei contatori da
  quello della lista. Non si ottimizza alla cieca adesso;
- **blocco 3 — anagrafica**: 6.6, 6.11 (`cliente.regole` con schema validato ed esempio obbligatorio
  per ogni regola), 8.8, seed dal foglio dei buyer; migrazione `0007_anagrafica`;
- poi annidati, proposte per cliente, ingresso esterno, articoli e distinta, fatti e STEP, le tre
  schermate, e l'igiene in parallelo dal blocco 3.

Delle voci di questa fase restano **2.10–2.15**, che entrano nei blocchi dove servono.
