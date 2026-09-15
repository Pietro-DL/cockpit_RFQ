# Fase 2 — più caselle, una sola elaborazione

Riepilogo tecnico di che cosa introduce questa fase, come si verifica e che cosa resta **non
verificato**. Come per [FASE_0.md](FASE_0.md) e [FASE_1.md](FASE_1.md), la documentazione operativa
completa (piani, decisioni aperte, registri degli esiti, richieste all'IT) vive fuori da questo
repository.

**Stato: in corso.** Di questa fase sono chiuse le voci **2.1** e **2.3**. Le voci successive (claim per
postazione, risoluzione dello store locale, postazione della sessione, TLS e credenziali individuali)
non sono state fatte e sono elencate in fondo.

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
| L1 unitari Go | triage di una mail interna, verifica statica della `0004`; nome nello staging e cartelle rifiutate (2.3) | eseguiti |
| L2 worker Python | lo stage carica con `PUT` prima del result; `413` → errore definitivo; `409` → nessun result | eseguiti |
| L3 contratti | `MessaggioIn.ricevuto_il`, `RiferimentoElemento.casella_id`, `RisultatoStage` senza `path_staging` sui due lati | eseguiti |
| L4 integrazione | I3, I4, I18, I21, S2 sulla `0004` con dati, cursore per casella, direzione dalle caselle; **M7, M8, M13**, hash diverso, result senza upload, pulizia dei `.parte` orfani | eseguiti |
| L5–L9 | due caselle vere in Outlook, casella condivisa Exchange, due postazioni (anche l'upload fra due PC) | **non eseguiti** |

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

**Lo store locale.** Lo StoreID è locale al **profilo** Outlook che lo ha letto (N44). Sta in
`messaggio_casella.store_id_locale` ed è un **ponte**, non il modello: il piano non lo vuole lì proprio
perché non è universale. Tenerlo su `messaggio` sarebbe però già sbagliato oggi, con due caselle dello
stesso profilo che hanno due store diversi. Finché a sincronizzare è **una sola postazione** il valore
è giusto; con due, l'ultima che sincronizza vince e le azioni dell'altra non trovano l'elemento. Lo
sostituisce la **voce 2.6** risolvendo la casella nello store locale di ogni postazione
(`casella_store`).

**Quale copia si apre.** Con più presenze, «Apri in Outlook», «Scarica» e «Segna letto» agiscono sulla
copia più vecchia (a parità, la casella con l'uuid minore): arbitraria ma **ripetibile**, così due
clic di seguito aprono lo stesso elemento. Quale copia aprire davvero diventa una decisione della
postazione del richiedente con le **voci 2.2 e 2.7**.

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

- **2.2 + 2.6 + 2.7** claim con postazione e caselle intersecate con `worker_credenziale`,
  `casella_store` risolto nello store locale, job interattivi alla postazione del richiedente senza
  ripieghi, postazione della sessione UI — test Q8, Q18, M1–M4, M9, M10, M12, W14.
- **2.4 + 2.5** TLS con impronta in `worker.toml`, credenziale individuale al posto del token
  condiviso, CSRF — test W4, W10, W11. Prima di questa il server resta esposto sulla LAN solo per le
  prove.
- **2.8–2.15** come da piano.
