# Checkpoint 3R — Stabilizzazione Inbox / RFQ Intelligence

17/09/2026. Migrazione `0008_interpretazione`.

La sequenza dell'addendum v2 si ferma **dopo il blocco 3**. Il blocco 4 (annidati) non parte.

## Perché esiste questo checkpoint

Il blocco 3 è implementato e i suoi test sono verdi. Poi il sistema è stato messo su **924 messaggi
veri**, ed è successo quello che i test simulati non possono far succedere: la posta vera è ambigua, e
ogni automatismo lasciato acceso «per il primo giorno» ha prodotto danni visibili in un pomeriggio.

Non è una deriva del piano. Aggancio automatico da togliere, famiglie per cliente al posto
dell'estrattore generico, tipo del PDF dall'analisi e non dall'estensione: erano i blocchi 6, 9 e 5.
Quello che è cambiato è l'**ordine**: la fase 4 viene anticipata, più quattro correzioni puntuali,
prima di costruire articolo, distinta e fascicolo sopra dati non abbastanza affidabili. L'addendum v2
resta intatto per dopo.

## I nove sintomi, verificati nel sorgente prima di toccare codice

| # | Sintomo | Dov'era |
|---|---|---|
| 1 | l'aggancio di un messaggio trascinava tutti gli orfani della stessa conversazione | `triage.go:412` `AgganciaOrfaniConversazione` |
| 2 | l'ingest agganciava da solo per ConversationID o per codice generico | `ingest.go:665`, `:748`, `:677` |
| 3 | «nuova RFQ» sovrascriveva «risposta»: 35 (parola RFQ) + 25 (allegato «tecnico») = 60 ≥ 50 | `codici.go:295-300` |
| 4 | l'estrattore generico parlava anche quando il cliente aveva famiglie dichiarate | `codici.go:201` |
| 5 | i codici precompilati diventavano identificativi `manuale` al submit | `triage.go:85`, `:219` |
| 6 | `pdf` ⇒ `disegno_2d` per estensione, +20 se il nome sembrava un codice | `proposta.go:34` |
| 7 | il poll dell'Inbox chiedeva solo `filtro`: perdeva casella, selezione e posizione | `inbox.html:22` |
| 8 | *Richieste* e *Cruscotto* erano due liste della stessa cosa | due template con le stesse colonne |
| 9 | il fascicolo girava sul modello vecchio (`componente` con `padre_id`) | `v_fascicolo` della `0001` |

I primi otto sono difetti e sono stati corretti. Il nono **non** è un difetto: è l'addendum v2 non
ancora fatto, e la correzione è dirlo nella schermata invece di rattoppare il modello vecchio.

---

## 1. Richieste e Cruscotto: una lista, e il cruscotto è della richiesta

*Richieste* è l'**elenco** delle RFQ aperte. Aprirne una porta al suo cruscotto — la pagina di lavoro
di quella richiesta — dove nei blocchi 5, 8 e 10 arriveranno Struttura, Smistamento e Fascicolo.

`/cruscotto` reindirizza a `/richieste`: chi aveva il segnalibro trova qualcosa, non un 404. La rail
diventa **Inbox · Richieste**, più la sezione **Admin** per l'amministratore.

Due tabelle globali quasi identiche non sono due funzioni: sono una funzione e un doppione, e il
doppione invecchia da solo perché nessuno si ricorda di aggiornare tutt'e due.

## 2. Aggancio: solo per decisione (fase 4.1 anticipata)

`messaggio.thread_id` non viene più scritto dall'ingest **in nessun caso**. Al suo posto,
`candidato_aggancio`: una riga per (messaggio, richiesta, regola), con punteggio ed evidenza.

| Regola | Che cosa afferma | Punti |
|---|---|---|
| **R0** `reply` | `In-Reply-To`/`References` puntano a un messaggio già agganciato | 98 |
| **R1** `conversazione` | il ConversationID è di una conversazione collegata **da un operatore** | 95 |
| **R4** `riferimento` | il riferimento del cliente (RDO, Anfrage, ODA) è quello di una richiesta | 90 |
| **R3** `codice` | un codice di **famiglia** è già identificativo di una richiesta dello stesso cliente | 80 |
| **R2** `oggetto` | stesso oggetto, stesso cliente, dentro `finestra_aggancio_gg` | 55 |
| **R5** `buyer` | stesso buyer, di recente | 35 |

R0 è l'unico legame che scrive il programma di posta e non una persona: non si sbaglia per caso.
R1 invece sì: «Rispondi» su una mail vecchia per parlare d'altro conserva il ConversationID.

La **conversazione** continua a essere collegata quando un operatore aggancia un messaggio — quella è
una sua decisione, ed è ciò su cui si regge R1 — ma gli altri orfani della conversazione **non
seguono**: ricevono un candidato R1 a 95, la loro proposta diventa «aggancia», e restano in Inbox.
Nel log degli agganci l'azione si chiama `candidato`, non `propaga`.

## 3. Precedenza: prima si guarda se la richiesta esiste, poi se ne sembra una nuova

L'esito del triage non è più il risultato di una somma che supera una soglia.

1. c'è un candidato **sopra 50 verso una richiesta APERTA** → `aggancia`, e `nuova_rfq` non viene
   nemmeno valutata; la confidenza è quella della regola;
2. il candidato forte punta a una richiesta **CHIUSA** → si mostra con l'avviso, e `nuova_rfq` torna
   in gioco: in questo mestiere lo stesso pezzo viene riquotato, e una richiesta chiusa è finita (T2);
3. il candidato è **debole** (R5) → si mostra e basta: ogni richiesta nuova di un buyer noto ne avrebbe
   uno, e se bastasse quello nessuna richiesta nuova verrebbe mai proposta come nuova;
4. nessun candidato → si contano i punti del contenuto, e sopra 50 si propone `nuova_rfq`.

## 4. Riferimenti RFQ e codici prodotto sono due cose diverse

`candidato_codice` tiene ogni numero trovato con il suo **ruolo** e la sua **provenienza**. La chiave
primaria è `(messaggio, codice)` senza il ruolo, apposta: lo stesso testo non può essere insieme il
nome della richiesta e un codice prodotto.

| ruolo | chi lo propone |
|---|---|
| `riferimento_rfq` | la regola `riferimento_rfq` del cliente. Va in `thread_offerta.riferimento_cliente`, **mai** in `identificativo_thread` |
| `prodotto` | una famiglia del cliente (punteggio 80) |
| `parte` | nessuno, oggi: distinguere un finito da un sottoassieme richiede la distinta (blocco 8). Una famiglia può dichiararlo con `"ruolo": "parte"` |
| `non_classificato` | l'estrattore generico (punteggio 30) |

**Se il cliente ha famiglie dichiarate, il generico non propone identificativi.** I numeri restano
visibili sotto «altri numeri trovati», non spuntati: si vedono, e per entrare serve un clic. Il
generico da solo vale per un cliente non censito, che è il caso normale del primo giorno.

Nel form *Nuova RFQ* è sparita la casella di testo precompilata. Ci sono caselle da spuntare, con
origine ed evidenza accanto; ciò che entra spuntato conserva **origine della proposta** e punteggio
(`proposta_famiglia` / `proposta_generico`), e `manuale` resta per ciò che si digita. Spuntare un
codice che non era fra i candidati non lo fa entrare.

## 5. PDF non significa disegno (D30)

`tipo_documento` guadagna **`da_determinare`**. Un PDF, un TIF e un TIFF sono `da_determinare` finché
il worker-analisi non li legge; un DWG, uno STEP e un DXF no, perché lì l'estensione dice davvero che
cosa c'è dentro. Il codice nel nome si conserva — è un indizio utile — ma non cambia il tipo.

Il worker-analisi distingue ora, dal contenuto: `offerta_promatec`, `disegno_2d` (cartiglio),
`capitolato`, `distinta_cliente`, e `da_determinare` quando non basta. Un PDF che non si apre è
`da_determinare`, non «un disegno con confidenza 40».

Nel triage un PDF non vale più «allegato tecnico» (25) ma «allegato di tipo da determinare» (10).
Confermare una proposta `da_determinare` viene rifiutato con una frase: un tipo ignoto non ha una
destinazione sul NAS.

**D30 — staging automatico.** Perché l'analisi possa dire che cosa sia un PDF, il file deve scendere;
ma finché il download parte solo da un clic, per sapere se vale la pena scaricarlo bisogna scaricarlo.
Con `[staging] automatico = true` gli allegati di un mittente **riconosciuto**, sotto `max_mb`
(20 MB), arrivano nello staging del server appena il messaggio entra. Lo staging è una cartella del
server: la regola «niente sul NAS senza una decisione» (D24) riguarda il NAS, e resta intatta.
Predefinito nel codice: **spento**.

## 6. Inbox: il poll non fa perdere il segno

Il poll porta con sé `filtro`, `casella` e `sel`; i link dei filtri e il selettore delle caselle
conservano gli altri due; ogni riga ha un id DOM stabile `msg-<uuid>`.

La colonna che scorre è il **genitore** di `#lista` (`.colonna` ha `overflow:auto`), e sostituire il
contenuto di un figlio azzera lo scorrimento del genitore appena l'altezza cambia: chi stava leggendo
il ventesimo messaggio tornava in cima da solo ogni quindici secondi. Ora `scrollTop` viene salvato
prima dello swap e rimesso nel frame successivo — insieme a `window.scrollY`, perché sotto i 900 px
scorre la pagina e non la colonna.

Refresh incrementale: **non fatto**, e non per dimenticanza. Prima serve una misura di `ListInbox` su
un corpus vero (blocco 10, voce 8.1): aggiungere complessità senza un numero è il modo più sicuro di
farne di nuova.

## 7. Anagrafica a sezioni

Cinque schede: **Generale · Domini e Buyer · Riconoscimento · Fabbisogno documentale · Banco di
prova**. L'elenco vive in Go (`sezioniAnagrafica`), non nell'HTML.

Le regole si scrivono in un **form**: famiglie in tabella (regex, descrizione, esempio, rev nel
codice, ruolo), riferimento, canale atteso, lingua, finestra di aggancio, frasi portale, CBD, numero
d'ordine anticipato. Il riquadro JSON resta sotto «Avanzato» — per una regex complicata è ancora il
modo più rapido, ed è l'unico modo di vedere che cosa c'è davvero in database. **Le due strade
passano dallo stesso `domain.ValidaRegole`**: se divergessero, il form potrebbe salvare ciò che il
riquadro rifiuta.

Buyer e fabbisogno sono **amministrabili**. Prima la tabella del fabbisogno si vedeva con scritto «si
modificano dal database»: una tabella che sembra configurabile e non lo è è peggio di una tabella
assente. Due guardie: una persona citata da messaggi o richieste non si cancella, e i predefiniti
(`cliente_id IS NULL`) non si tolgono da lì. La scheda dice a voce alta che la risoluzione è **in
blocco**: aggiungere una riga per un tipo di componente **sostituisce** i predefiniti di quel tipo.

## 8. Fascicolo e sottoassiemi: dichiarati non disponibili

`v_fascicolo` e `componente` **non** vengono rattoppati. Nel cruscotto della richiesta, Struttura e
Fascicolo dicono «non disponibili in questa versione» e spiegano perché: il modello di oggi tiene un
albero per richiesta con un padre solo, e un sottoassieme condiviso fra due assiemi non si può
rappresentare senza raccontare una struttura falsa. Il fabbisogno calcolato sul modello attuale resta
leggibile in un `<details>`, dichiarato parziale.

Una funzione non ancora costruita e una funzione rotta si distinguono solo se qualcuno lo scrive.

## 9. Analisi semantica: un terzo interprete, non un sostituto

```
Outlook → ingest dei fatti → candidati deterministici → analisi semantica → PROPOSTE → operatore
```

Output strutturato e versionato in `analisi_messaggio`: `input_hash`, versione dell'analizzatore, del
prompt e del modello, risultato, **grezzo**, **scartato**, token e durata. Stessa terna = nessuna
seconda chiamata; rianalizzare è una scelta, non l'effetto di un job ripetuto.

**Il grounding è in Go, non nel prompt.** Ogni codice, riferimento, candidato e nome di allegato
citati vengono ricontrollati contro il testo del messaggio, i nomi degli allegati e i dati già in
database; ciò che non trova riscontro viene scartato **con il motivo** e non raggiunge la schermata.
Una bozza di risposta che cita un numero inesistente cade intera: una risposta al cliente con un part
number sbagliato è un danno vero, e non si corregge a metà.

Un output non conforme allo schema è `rifiutata`: nessun effetto, e l'analisi resta come prova.

Vincoli rispettati: niente `thread_id`, niente conferme di articoli o distinta, niente NAS, niente
invio. Le bozze passano dalla `crea_bozza_outlook` che c'è già, con `consenti_invio = false`.

**Riservatezza.** Il testo delle mail dei clienti uscirebbe verso un servizio esterno. È una decisione
da mettere per iscritto con l'IT e con chi segue la ISO 27001 — per casella, con la possibilità di
spegnerla — e **non è una decisione tecnica**. Perciò: spenta se non accesa in `[agente]`, spenta se
manca la chiave (che sta in una variabile d'ambiente, non nel file), e permessa solo sulle caselle
elencate. Escono oggetto, corpo, nomi degli allegati, ragione sociale del cliente e i candidati già
calcolati. Mai il contenuto dei file.

**Nessuna chiamata è mai stata fatta.** Tutto ciò che questo checkpoint prova dell'agente — schema,
grounding, idempotenza, prompt — è provato con un modello finto, senza rete e senza chiave.

---

## Prove

| ID | Che cosa dimostra | Livello |
|---|---|---|
| T1 | conversazione già collegata → **nessun aggancio**, candidato R1 a 95 con evidenza | L4 |
| T2 | richiesta CHIUSA → avviso, e `nuova_rfq` resta proponibile | L1 |
| T3 | stesso oggetto/codice fuori dalla finestra → nessun candidato | L4 |
| T4 | stesso codice, cliente diverso → nessun candidato | L4 |
| T16 | tre candidati → si vedono tutti, ordinati, e nessuno viene scelto | L4 |
| T21 | l'aggancio di un messaggio **non** trascina gli altri della conversazione | L4 |
| T22 | risposta con `In-Reply-To` + parola RFQ + allegato → mai `nuova_rfq` (L1 su tutte le regole, L4 sul giro vero) | L1+L4 |
| T23 | solo i codici spuntati diventano identificativi, con l'origine della proposta; il riferimento va nel suo campo | L4 |
| AN6 | un riferimento RFQ non diventa mai un codice prodotto | L1 |
| AN7 | con le famiglie dichiarate il generico non propone; senza, sì | L1 |
| — | un codice mai proposto non entra spedendo il suo nome nel form | L4 |
| — | PDF/TIF → `da_determinare`; DWG/STEP/DXF no; un PDF non vale più «allegato tecnico» | L1 |
| — | PDF generato: generico → non CAD; cartiglio → `disegno_2d`; capitolato; distinta; PDF rotto → `da_determinare` | L1 (Python) |
| — | agente: schema rifiuta sette forme non conformi; grounding scarta ciò che non esiste; bozza con codice inventato cade intera; errore di rete = nessuna proposta | L1 |
| **UI4** | scroll a metà → poll → posizione invariata; filtro, casella e selezione conservati | **L7, non eseguita** |

### UI4 — procedura (da eseguire in un browser vero)

1. entrare come operatore, aprire `/inbox` con almeno due caselle censite e più messaggi del viewport;
2. scegliere una casella dal selettore e aprire un messaggio (la barra deve mostrare `casella=` e `sel=`);
3. scorrere la lista **a metà** e prendere nota della riga in cima;
4. aspettare almeno **due** poll (30 s), senza toccare nulla;
5. atteso: la casella è ancora quella scelta, il messaggio è ancora selezionato, la riga in cima è la
   stessa a meno di pochi pixel, e la barra dell'indirizzo non è cambiata;
6. ripetere cambiando filtro (orfani → tutti): casella e selezione devono sopravvivere al cambio.

---

## Che cosa questo checkpoint non dimostra

- **niente di tutto ciò in un browser vero**: UI4, i candidati con l'evidenza, l'Anagrafica a sezioni,
  il riquadro dell'agente. Sono prove L7, **non eseguite**;
- **nessuna chiamata all'agente**: il modello dei test è finto. Che le proposte siano *utili* non è
  stato misurato, e la misura è il replay sul corpus (L6) con `atteso.csv`, che va fatto prima di
  accendere qualunque cosa;
- **i 924 messaggi già ingeriti non sono stati rianalizzati**: i candidati di aggancio e di codice
  nascono all'ingest, quindi i messaggi vecchi non ne hanno. Rifare l'ingest sullo storico è
  un'operazione a parte, e tocca proposte che un operatore potrebbe aver già deciso;
- **gli agganci automatici già scritti restano**: i messaggi con `aggancio = auto_conversazione` o
  `auto_identificativo` conservano il loro `thread_id`. Toglierli sarebbe riscrivere decisioni prese
  dal sistema quando quel comportamento era quello previsto; i valori restano nell'enum per questo;
- **lo staging automatico non è stato provato su Outlook vero**: accende job veri su una casella vera,
  ed è una prova L5.
