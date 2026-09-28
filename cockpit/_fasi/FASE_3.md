# Blocco 3 — anagrafica: i clienti smettono di essere un elenco di cartelle

**Stato: completato** per quanto riguarda il codice e le prove simulate. Le prove sui dati reali sono
elencate in fondo, e una è **non eseguita** di proposito.

Riepilogo di che cosa introduce questo blocco, come si verifica e che cosa resta **non verificato**.
Come per le fasi precedenti, la documentazione operativa completa (piani, decisioni aperte, registri
degli esiti, dati dei clienti) vive fuori da questo repository.

## Perimetro

Nessun collegamento a caselle di posta reali. Le famiglie di codice sono state **ricavate dai
messaggi già presenti nel database di sviluppo** e verificate lì; nessun dato di cliente — nomi,
domini, indirizzi, regex — è entrato in questo repository. Il seme è un file che sta fuori, e il
Cockpit sa soltanto come si legge. La migrazione precede il codice che la usa.

## Il problema che questo blocco risolve

Tre difetti, di cui due silenziosi.

**L'anagrafica si sovrascriveva da sola.** `UpsertCliente` faceva `ON CONFLICT (cartella_nas) DO
UPDATE`: chi creava un cliente nuovo scrivendo una cartella NAS già presa non riceveva nessun errore. Rinominava il cliente esistente e gli lasciava addosso domini, buyer e
RFQ. `UpsertDominioCliente` faceva lo stesso su `dominio`: assegnare a un cliente il dominio di un
altro lo **spostava**, e siccome `v_inbox` risolve il cliente dal dominio del mittente, cambiavano
cliente anche tutti i messaggi già arrivati. Voce 6.6, test T7 e T8.

**Le regole dei clienti non esistevano.** `cliente.regole` era jsonb dalla 0001 e restava `{}` per
sempre, perché non c'era nessuna schermata in cui scriverle e nessuno schema che dicesse che cosa
metterci. Il riconoscimento dei codici era una regex sola per tutti i clienti del mondo.

**La testata non scalava.** Cinque voci in una barra orizzontale funzionavano; i blocchi da qui al 10
ne aggiungono almeno sei.

## 1. La migrazione `0007_anagrafica`

Tre cose, e nessuna di più.

`cliente.peso smallint` **0–15**, con un `CHECK` (D27). Il peso è un dato di anagrafica, non della
singola richiesta: «questo cliente passa avanti» vale per tutte le sue RFQ. Serve a ordinare la lista
*Richieste*, e **non è** il punteggio di priorità dell'addendum 2 — quella formula non esiste ancora e
qui non se ne inventa una provvisoria.

`cliente.regole` prende un `CHECK (jsonb_typeof(regole) = 'object')`. Lo schema vero lo fa il Go; il
database garantisce l'unica cosa che il Go non può, cioè che dentro ci sia un oggetto e non un `[]`.

`fabbisogno_documento.fonte_attesa` (`cliente` | `portale` | `promatec`). Vedi §5: è l'unica colonna
che serviva davvero.

Il numero: la 0006 aveva già dichiarato che l'anagrafica sarebbe stata la `0007`. Il `0007_igiene` del
piano scala a valle.

## 2. Lo schema delle regole — `regole.Regole` (voce 6.11, D17)

Un tipo Go con i campi dell'addendum: `famiglie_codice[{regex, descrizione, rev_nel_codice, esempio}]`,
`riferimento_rfq{regex, esempio}`, `canale_atteso`, `frasi_portale[]`, `lingua_risposta`,
`richiede_cbd`, `numero_ordine_anticipato`, `finestra_aggancio_gg`.

**Due porte, non una.** Il registro (D17) dice che una regola con l'esempio sbagliato non viene
salvata; l'addendum e il mockup dicono che va segnata ✗ e non usata. Sono le due metà della stessa
cosa e ci sono entrambe:

- `ValidaRegole` è la porta in **scrittura**: rifiuta un campo che non esiste (di solito un nome
  scritto male, che senza `DisallowUnknownFields` sarebbe ignorato in silenzio), una regex che non
  compila, un esempio mancante, un esempio che non corrisponde alla propria regex;
- `LeggiRegole` è la porta in **lettura**: non rifiuta niente — ciò che è già in database va letto
  comunque — e restituisce le regole più una diagnosi riga per riga;
- `Compila` costruisce il motore **solo** con le regole utilizzabili.

**Perché l'esempio è obbligatorio.** Una regex sbagliata non fallisce: non riconosce. Un cliente che
smette di essere riconosciuto non produce nessun errore, produce silenzio, e l'esempio è l'unico modo
per distinguere «non ha trovato niente perché non c'era niente» da «non trova niente e basta».

**La revisione dentro al codice.** `rev_nel_codice: true` obbliga la regex ad avere un gruppo
`(?P<rev>…)`. È l'unico modo di dire dove sta la revisione senza scrivere la convenzione di un cliente
dentro il Go: un codice come `AC12345B` non ha separatori, e nessuna regola generale lo dividerebbe.

## 3. Un motore solo (voce 6.11, AN5)

`classificazione.Riconosci` è l'unico ingresso al riconoscimento: la chiama l'ingest sui messaggi che arrivano
e la chiama il banco di prova della schermata Anagrafica sul testo incollato. Restituisce triage,
riferimenti al portale e scadenza in una volta.

Un banco di prova con funzioni sue assomiglia al sistema finché qualcuno non cambia il sistema: da
quel momento mostra un risultato che nessun messaggio vero avrà mai, e lo mostra proprio alla persona
che sta tarando le regex fidandosi di quello che legge.

**`EstraiCodici` per cliente.** Le famiglie del cliente parlano per prime e ogni codice esce con la
sua provenienza (`famiglia` + descrizione, oppure `generico`). L'estrattore generico **resta** come
rete sotto: un cliente che introduce una famiglia nuova non la dichiara, la manda, e un motore che
guardasse solo le famiglie censite smetterebbe di vedere i codici nuovi proprio mentre il cliente li
introduce. Decide comunque una persona (D9).

Nell'ingest i motori si compilano **una volta per lotto** (`ingest.Motori`): compilare le regex di un
cliente dentro un ciclo su duecento messaggi vuol dire compilarle duecento volte. La cache vive quanto
il lotto, quindi una regola cambiata in Anagrafica vale dal lotto successivo senza che nessuno debba
invalidare niente.

## 4. La navigazione (8.8, D29)

Rail a sinistra, raggruppata in sezioni; testata riservata al contesto operativo (stato delle caselle,
postazione, utente). Sono due cose diverse e stavano nella stessa riga: «dove voglio andare» e «come
sta il sistema adesso».

- operatore: **Inbox · Richieste · Cruscotto**;
- amministratore: le stesse più la sezione **Admin** con *Anagrafica · Postazioni · Coda job · Scarti*.

**D29 — l'Anagrafica è amministrativa.** Il mockup e D21 la mettevano fra le voci operative: la
decisione è superata. Una regex cambiata lì cambia il riconoscimento della posta di tutti, non una
riga di una richiesta. Sta sotto `/admin/*` con lo stesso `soloAdmin` delle altre; un operatore non
vede la voce e prende 403 sulle otto rotte, anche scrivendole a mano.

La rail si costruisce da `navPer`, che chiama `almeno`: la **stessa** funzione del wrapper delle
rotte. Nascondere una voce non è autorizzare — la porta la chiude `soloAdmin` — ma ciò che si vede e
ciò che si apre non devono poter divergere.

*Richieste* (`/richieste`) è la lista delle RFQ aperte ordinata per peso del cliente e poi per
scadenza. *Articoli*, dentro Anagrafica, è uno **schermo vuoto** per scelta: il modello `articolo` è
il blocco 5, e mostrare oggi i `componente` vorrebbe dire mostrare le istanze dentro le singole RFQ
chiamandole articoli — la confusione che il blocco 5 deve togliere.

## 5. Il fabbisogno documentale: che cosa è stato ispezionato e che cosa si è deciso di NON fare

Il modello esistente è stato guardato prima di toccarlo, come chiede il blocco.

**Regge, e meglio di quanto sembrasse.** `v_fascicolo` risolve il fabbisogno **in blocco** per
`tipo_componente`: se un cliente ha anche una sola riga per «sciolto», per lui valgono le sue righe e
**non più** i default. Quindi «questo documento a me non serve» è già esprimibile — si scrive
l'insieme del cliente senza quella riga — e una colonna `richiesto` sarebbe stata un secondo modo di
dire la stessa cosa. Non è stata aggiunta. La regola però non era scritta da nessuna parte: stava
dentro un `LATERAL` di sessanta righe, ed è il genere di cosa che si scopre il giorno in cui un
cliente perde metà del suo fascicolo per una riga aggiunta con le migliori intenzioni. Adesso è un
`COMMENT` sulla tabella, ed è **provata**: `TestLaSchedaFabbisognoDiceQuelloCheIlFascicoloPretende`
confronta quello che la schermata mostra con quello che `v_fascicolo` pretende davvero.

**Mancava davvero una cosa sola: da dove arriva il documento.** «Il DXF lo facciamo noi» non è un
documento mancante, è un documento che nessuno deve sollecitare; «il capitolato sta sul portale» non è
una mail da aspettare, è un link. Oggi questa distinzione esiste solo per RFQ (`riferimento_portale`
nasce da una frase in una mail) e non come proprietà del cliente. Da qui `fonte_attesa`.

**Resta ambiguo, e si segnala invece di risolverlo qui:** `tipo_componente`
(`finito`/`sottoassieme`/`sciolto`/`commerciale`) mescola il **ruolo nella RFQ** e la **natura**
dell'articolo. D19 dice che la natura è `assieme`/`particolare` e che «finito» non è un tipo ma il
ruolo di radice. Rimapparlo adesso vorrebbe dire anticipare `articolo.natura`, cioè il blocco 5, e
creare nel frattempo due sorgenti della stessa verità. La scheda *Fascicolo atteso* mostra quindi
l'asse che esiste davvero, non quello del mockup.

## 6. Il seme (voce 6.6)

`cockpit.exe -semina-anagrafica <file.json>` legge un file, lo **convalida per intero** e poi scrive.

**Il cancello.** Prima di scrivere una riga, tutte le regole di tutti i clienti passano da
`ValidaRegole`. Se una sola ha l'esempio che non corrisponde, il seme non parte affatto — non «quel
cliente viene saltato»: un seme a metà è la cosa più difficile da capire il giorno dopo. È D17
applicata all'altra porta: senza, lo stesso identico errore scritto in un file invece che in un
riquadro entrerebbe in database senza che nessuno lo guardi.

**Non aggiorna.** Un cliente già presente viene saltato per intero. Il file è la fotografia di un
foglio Excel; il database è dove qualcuno ha già corretto a mano quello che il foglio sbagliava, e un
seme che «aggiorna» cancella quelle correzioni a ogni lancio — cioè proprio perché lo si rilancia per
abitudine. I **domini** invece si aggiungono anche a un cliente esistente (aggiungerne uno non
sovrascrive niente, e un dominio mancante è il motivo più comune per cui la posta di un cliente
censito non viene riconosciuta), ma un dominio già di un altro **non si sposta**: si segnala e si va
avanti.

Il file sta fuori dal repository: nomi dei clienti, domini, buyer e forme dei codici sono dati
dell'azienda.

## 7. Le famiglie di codice sono state ricavate dai messaggi, non dai documenti

D21 lo chiedeva («il pattern del mockup non si semina»), e il confronto con i **924 messaggi** già in
database gli ha dato ragione. I dati dei clienti — nomi, domini, forme dei codici — stanno fuori da
questo repository; qui resta il metodo e il suo esito.

**Che cosa è stato fatto.** Per ogni cliente del foglio si sono estratti dai suoi messaggi tutti i
token che somigliano a un codice, ridotti alla loro *firma* (ogni cifra un 9, ogni lettera una A) e
contati. Le firme ricorrenti sono le famiglie candidate. Ogni candidata è poi stata misurata **con il
motore Go**, non con una regex riscritta a parte, su due numeri: quante mail **del** cliente riconosce
e quante mail **di altri clienti** pesca per sbaglio.

**Che cosa ha detto il confronto.**

- per un cliente il mockup mostrava come «famiglia di codice» quello che nei messaggi veri è il
  **riferimento della richiesta** — cioè il numero della RDO, non un codice prodotto: seminarlo così
  avrebbe fatto proporre come articolo una cosa che articolo non è;
- per un altro il foglio dava una forma con un prefisso che nei messaggi compare **solo a volte**, e
  ometteva un suffisso di revisione che invece c'è;
- un cliente su cui il foglio è vuoto ha due famiglie ben visibili nei messaggi: esistono solo lì;
- un cliente che il foglio dà come francese scrive in realtà dal **distributore italiano**, il che
  spiega la nota «corrispondenza in italiano» del foglio stesso. I suoi sei codici, elencati nel
  foglio, non sono confermati da nessun messaggio e **non sono stati seminati**.

**Due candidate sono state scartate dalla misura**, non dal giudizio: una pescava in cinque messaggi
di altri clienti (troppo larga), l'altra non trovava niente perché `_` è un carattere di parola e il
`` finale non scattava mai. Una terza è stata riscritta perché usava un *lookahead*, che Go (RE2)
non supporta: se ne è accorto il convalidatore del seme, non un utente.

**Sei clienti sono stati seminati senza nessuna famiglia.** I loro codici, nei messaggi, sono numeri a
sei o otto cifre senza prefissi: indistinguibili da un CAP, da un importo o da un numero di telefono.
Una regex scritta lì avrebbe riconosciuto tutto, che è un altro modo di non riconoscere niente.

Un dettaglio che la verifica ha reso evidente: un numero che compare nei messaggi di **17 domini
diversi** non è un codice di nessun cliente — è nostro, e sta nel piè di pagina. L'estrattore generico
lo propone; le famiglie no. È in miniatura il motivo per cui le famiglie servono.

## Come si verifica

| ID | Che cosa dimostra | Test |
|---|---|---|
| AN1 | lo schema rifiuta in scrittura undici modi di scrivere una regola che non riconoscerebbe niente senza fallire | `TestAN1…` (L1) |
| AN2 | il seme non parte se una sola regola di un solo cliente ha l'esempio sbagliato | `TestAN2…` (L1) |
| AN3 | una regola rotta si vede (✗ con il motivo) e **non entra** nel motore | `TestAN3…` (L1) |
| AN4 | le famiglie del cliente estraggono con la loro provenienza e la rev dal gruppo `(?P<rev>)`; il generico resta come rete | `TestAN4…` (L1) |
| AN5 | il banco di prova e l'ingest danno lo **stesso** esito sullo stesso testo | `TestAN5…` (L4) |
| T7 | un cliente nuovo su una cartella NAS presa fallisce, dice di chi è, e non tocca l'esistente | `TestT7…` (L4) |
| T8 | un dominio non passa da un cliente a un altro; riassegnarlo al suo non è un errore | `TestT8…` (L4) |
| D29 | otto rotte `/admin/anagrafica*` rispondono 403 a un operatore, in GET e in POST, e niente cambia | `TestUnOperatoreNonEntraInAnagrafica` (L4) |
| W15 | la rail si costruisce da `navPer`: le voci promesse ci sono, quelle di Admin solo per l'admin | `TestW15…` (L1) |
| — | il peso resta 0–15 in UI **e** in database | `TestIlPesoRestaNelSuoDominio` (L4) |
| — | la scheda *Fascicolo atteso* dice quello che `v_fascicolo` pretende | `TestLaSchedaFabbisogno…` (L4) |
| — | il seme crea, è idempotente, non aggiorna e non ruba domini | `internal/anagrafica` (L4) |

**Verificato al contrario: 15 difetti rimessi, 15 prove rosse.** Fra questi: l'upsert sulla cartella
NAS, l'upsert sul dominio, l'esempio che non deve più corrispondere, il campo sconosciuto che torna a
passare, il motore che accetta le regole rotte, il banco che si costruisce un motore proprio, la rail
che mostra Admin a tutti, `/admin/anagrafica` montata con `autenticato`, il `CHECK` sul peso tolto, il
fabbisogno risolto in modo diverso da `v_fascicolo`.

## Che cosa questo blocco NON dimostra

- **il seme sui dati veri**: il file è stato scritto, convalidato e seminato **sul database di test**,
  due volte, per provarne l'idempotenza. Sul database di sviluppo — quello con i 924 messaggi — **non
  è stato lanciato**: serve il via libera. Finché non lo si lancia, la prova «clienti riconosciuti
  prima/dopo» del piano resta **non eseguita**;
- **che le famiglie siano complete**: sono verificate, non esaustive. 127 messaggi su 249 dai domini
  censiti contengono almeno un codice di famiglia; gli altri sono logistica, amministrazione e
  corrispondenza senza part number. Un cliente che introduce una forma nuova la introduce e basta;
- **la schermata in un browser vero**: il ✓/✗, il banco di prova e la rail collassabile sono provati
  dal server, non da un occhio. È una prova L7;
- **il peso come priorità**: `Richieste` ordina per `cliente.peso`, e i pesi seminati sono **tutti 0**
  perché il foglio non li dichiara. Li mette una persona dall'Anagrafica;
- **`articolo`, `distinta`, `v_fascicolo_rfq`**: blocchi 5 e 8. Qui non c'è nemmeno una colonna.
