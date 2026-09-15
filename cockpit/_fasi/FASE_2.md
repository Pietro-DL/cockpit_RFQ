# Fase 2 — più caselle, una sola elaborazione

Riepilogo tecnico di che cosa introduce questa fase, come si verifica e che cosa resta **non
verificato**. Come per [FASE_0.md](FASE_0.md) e [FASE_1.md](FASE_1.md), la documentazione operativa
completa (piani, decisioni aperte, registri degli esiti, richieste all'IT) vive fuori da questo
repository.

**Stato: in corso.** Di questa fase è chiusa la sola **voce 2.1**. Le voci successive (upload degli
allegati legato al tentativo, claim per postazione, risoluzione dello store locale, TLS e credenziali
individuali) non sono state fatte e sono elencate in fondo.

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
| L1 unitari Go | triage di una mail interna, verifica statica della `0004` | eseguiti |
| L3 contratti | `MessaggioIn.ricevuto_il` e `RiferimentoElemento.casella_id` sui due lati | eseguiti |
| L4 integrazione | I3, I4, I18, I21, S2 sulla `0004` con dati, cursore per casella, direzione dalle caselle | eseguiti |
| L5–L9 | due caselle vere in Outlook, casella condivisa Exchange, due postazioni | **non eseguiti** |

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

**`parent_messaggio_id` resta.** Il piano lo elimina insieme alle occorrenze (fase 3, §3.5), e il test
S2 lo dice esplicitamente per la `0005`. Toglierlo adesso cancellerebbe l'unico legame fra un messaggio
annidato e il suo contenitore senza avere ancora ciò che lo sostituisce. Per questo la regola
dell'Inbox della voce 2.1 usa oggi quel campo dove domani userà le occorrenze: il comportamento
dichiarato — un messaggio ricevuto non sparisce perché arriva anche come allegato — è già quello.

## Che cosa resta aperto in questa fase

- **2.3** upload degli allegati al server legato al tentativo (`PUT /api/v1/allegati/{id}/file` con
  `job_id` e `lease_token`, file `.parte.<token>`, `max_upload_mb`) — test M7, M8, M13. È ciò che
  permette al worker di stare su un PC diverso dal server.
- **2.2 + 2.6 + 2.7** claim con postazione e caselle intersecate con `worker_credenziale`,
  `casella_store` risolto nello store locale, job interattivi alla postazione del richiedente senza
  ripieghi, postazione della sessione UI — test Q8, Q18, M1–M4, M9, M10, M12, W14.
- **2.4 + 2.5** TLS con impronta in `worker.toml`, credenziale individuale al posto del token
  condiviso, CSRF — test W4, W10, W11. Prima di questa il server resta esposto sulla LAN solo per le
  prove.
- **2.8–2.15** come da piano.
