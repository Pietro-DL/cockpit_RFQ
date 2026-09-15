# Fase 1 — coda e ingest che non si bloccano e non si duplicano

Riepilogo tecnico di che cosa introduce questa fase, come si verifica e che cosa resta **non
verificato**. Come per [FASE_0.md](FASE_0.md), la documentazione operativa completa (piani, decisioni
aperte, registri degli esiti, richieste all'IT) vive fuori da questo repository.

## Perimetro

Nessun collegamento a caselle di posta reali: tutto ciò che segue è stato provato su un database di
prova usa e getta e con un finto server per i worker. Le migrazioni precedono il codice che le usa.
La visibilità delle caselle personali resta al default restrittivo.

## Il problema che questa fase risolve

Due difetti, entrambi silenziosi.

**Il poison pill.** L'ingest usciva al primo errore e rispondeva 422 sull'intero lotto. Un solo
messaggio che il database rifiutava — una direzione fuori enum, un identificativo troppo lungo, un
byte che PostgreSQL non accetta — fermava il sync; il cursore non avanzava, quindi alla scansione
successiva si ripresentava lo stesso messaggio e il ciclo ricominciava identico. Non serviva un
attacco: bastava una mail malformata, e nessuna delle mail successive entrava più.

**Il tentativo senza identità.** Un job «in corso» era identificato dal solo `worker_id`. Un tentativo
scaduto e uno nuovo erano quindi indistinguibili, e il risultato tardivo del primo si applicava sopra
al lavoro del secondo: un job chiuso da chi non lo stava più eseguendo, un allegato segnato in errore
mentre un altro worker lo stava scaricando bene, un cursore fatto avanzare da un tentativo morto.

## 1. Il tentativo diventa un'entità — migrazione `0003`

Ogni claim genera un `lease_token` e fissa `avviato_il`. Da lì nasce un **predicato unico di validità**,
che compare identico in tutti i punti in cui un worker scrive:

```sql
stato = 'in_corso' AND lease_token = $token AND worker_id = $worker
AND lease_fino_a > now() AND now() <= avviato_il + durata_max_s
```

Zero righe significa «questo tentativo non vale più»: il server risponde 409 e **non applica niente**.
Il predicato sta in `internal/db/queries/job.sql` e vale per heartbeat, risultato riuscito, risultato
fallito e ingest. Non è un controllo ripetuto per prudenza: basta un punto scoperto perché il lavoro di
un tentativo scaduto si applichi a quello che gli è subentrato.

`durata_max_s` limita l'intero tentativo, non il singolo lease: un worker che rinnova il lease ogni 30
secondi terrebbe altrimenti il job per sempre, e un job bloccato dentro una chiamata COM che non
ritorna resterebbe «in corso» finché qualcuno non se ne accorge.

## 2. Il lotto è una transazione, l'elemento è un savepoint

`Ingerisci` apre **una** transazione per lotto e un `SAVEPOINT` per elemento. Un elemento rifiutato
viene annullato fino al suo savepoint e registrato in `ingest_scarto`; gli altri entrano; il cursore
avanza nella stessa transazione.

Così **«200 ⇔ tutto durevole»** è una proprietà del codice, non una convenzione:

- se la risposta è 200, messaggi, scarti e cursore sono tutti in database;
- se il commit non riesce, la risposta è un errore e in database non resta **niente** di quel lotto,
  nemmeno gli elementi andati a buon fine — quindi il worker può ripetere il lotto identico.

Il savepoint non è un dettaglio implementativo: un errore di PostgreSQL aborta la transazione intera,
e senza un savepoint da cui ripartire il primo elemento rotto porterebbe giù tutti gli altri. Il test
che conta è quello con un errore vero del server (un byte nullo nel corpo), non uno simulato in Go.

Gli scarti hanno due origini e si riprovano in due modi diversi:

| Origine | Cosa è successo | Come si riprova |
|---|---|---|
| `ingest` | il database ha rifiutato l'elemento; il payload completo è in DB | si rifà l'ingest dal payload, **senza Outlook** — l'unico modo che funzioni anche quando l'elemento nel frattempo è stato spostato o eliminato |
| `lettura` | il worker non è riuscito a convertirlo; un payload non c'è | si accoda un job `rileggi_elemento` per quel solo elemento |

Una casella non censita o disattivata è un'altra cosa ancora: è un errore di configurazione, riguarda
tutto il lotto, si vede **prima** di aprire la transazione e non produce scarti.

## 3. Il recupero di un worker bloccato in COM

Una chiamata COM non è interrompibile dall'esterno. Il battito sta quindi su un thread suo e non tocca
mai COM: quando riceve 409 alza un flag di arresto e concede 15 secondi al lavoro per fermarsi da solo.

- Il lavoro che **può** fermarsi lo fa ai punti di ripresa (fra un elemento e l'altro della scansione).
- Il lavoro che **non può** fermarsi — Outlook fermo su una finestra modale — fa terminare il processo
  con codice 3, dopo aver scritto nel log quale job e quale fase lo tenevano bloccato; l'attività
  pianificata lo riavvia. Il job, lato server, è già tornato in coda.

Dopo un 409 il worker **non riporta più nulla**: riportare un esito da un tentativo scaduto
significherebbe far fallire il job di qualcun altro.

## 4. Elaborazione singola

- L'analisi è deduplicata per **contenuto, versione dell'analizzatore e configurazione**, non per
  allegato: lo stesso disegno in tre richieste di due clienti fa partire un solo job. I fatti finiscono
  in `analisi_fatti` e vengono distribuiti a tutte le proposte ancora aperte con quel contenuto —
  ciascuna con le regole della propria RFQ. Le proposte già decise non si toccano.
  Cambiare un termine in `[analisi].parametri` cambia la chiave e fa rianalizzare, senza toccare codice.
- I sync non si accumulano: chiave `sync_outlook:<casella>` fissa, un solo sync pendente per casella.
- Due operatori che decidono insieme sullo stesso messaggio non creano due RFQ: il messaggio viene
  bloccato per la durata della transazione e il secondo riceve un esito esplicito che dice dove il
  messaggio è finito. Stessa cosa per due conferme sullo stesso allegato. Ogni decisione lascia una
  riga in `messaggio_aggancio_log`.
- Due allegati con lo stesso indice nello stesso messaggio non sono più un doppione innocuo: il
  secondo sovrascriveva il primo e un allegato spariva in silenzio. Ora l'elemento va in scarto.
- Lo stesso contenuto non si scarica due volte. Prima di accodare un download il server guarda se il
  file di quell'allegato è già in staging, e se non c'è cerca un altro allegato con lo stesso sha256
  che ce l'abbia: ogni download è un giro in COM su Outlook, la parte più lenta e più fragile della
  catena. La guardia sta nell'accodamento, non nei gestori HTTP, perché i punti che chiedono un
  download sono tre e in due su tre nessuno si sarebbe accorto del doppione: il secondo scaricamento
  riesce benissimo.
- Il sync non annulla le decisioni già prese. Un messaggio agganciato o ignorato la mattina non torna
  fra gli orfani il pomeriggio perché una scansione gli ha rimesso sopra una proposta nuova.

## 5. Altre correzioni

| Cosa | Prima | Ora |
|---|---|---|
| estrazione zip | dentro la transazione del risultato | prima di aprirla: la transazione contiene solo scritture in database |
| budget dello zip | sulla dimensione **dichiarata** nell'header | sui byte **effettivamente scritti**, consumati fra una voce e l'altra |
| percorso dentro lo staging | confronto per prefisso di stringa | `filepath.Rel` |
| enum in arrivo dai worker | cast diretto: a rifiutare era PostgreSQL | `.Valid()` esplicito, errore leggibile, job fallito in modo definitivo |
| identificativi | `varchar(255)`: un Message-ID più lungo veniva troncato, cioè due messaggi diversi diventavano lo stesso | `text`; oltre 1000 caratteri l'elemento va in scarto |
| copie sul NAS | 5 tentativi, consumati anche quando il NAS non c'era: poco più di mezz'ora di copertura, poi la copia risultava persa in silenzio | 50 tentativi, nessuno consumato mentre il NAS è irraggiungibile, e riaccodo automatico quando torna |

## 6. Un limite dichiarato: una sola casella attiva

Alla versione 3 dello schema `sync_cursore` ha per chiave la sola **cartella**. Il lotto porta già
`casella_id` e il resto è pronto, ma il cursore no: due caselle con una cartella «Inbox» scriverebbero
sulla stessa riga, farebbero avanzare il cursore l'una dell'altra, e ogni avanzamento di troppo è una
finestra di tempo che la seconda casella non leggerà mai — messaggi mai acquisiti, senza nessun errore
da nessuna parte.

Il server quindi **rifiuta di partire** con più di una casella attiva finché lo schema non è arrivato
alla `0004`. È una condizione temporanea, verificata a ogni avvio: applicata la `0004` (fase 2), il
controllo smette da solo di intervenire.

## 7. Come verificare

```powershell
cd cockpit
powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Avvia
powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1
```

I test d'integrazione condividono un solo database e alcuni ricreano lo schema: `-p 1` non è
un'ottimizzazione, è una condizione di correttezza.

## 8. Che cosa è verificato e che cosa no

| Livello | Copertura | Stato |
|---|---|---|
| L1 unitari Go | dominio, percorsi NAS, archivi (budget e percorsi), template, verifica statica delle migrazioni, configurazione | eseguiti |
| L2 unitari Python | modulo comune, ciclo dei worker, **arresto e uscita forzata**, analisi documenti | eseguiti; i casi sul corpus riservato risultano **saltati** |
| L3 contratti | conformità fra gli schemi JSON di `contracts/`, i tipi Go e i modelli pydantic | eseguiti (anticipati dalla fase 5) |
| L4 integrazione | coda e tentativo, ingest e scarti, decisioni concorrenti, deduplica dell'analisi, migrazioni e seed | eseguiti |
| L5–L9 | Outlook via COM, Exchange, due postazioni, posta reale, caos | **non eseguiti**: richiedono account e macchine non disponibili qui |

Tre precisazioni che valgono anche per chi legge solo questo file:

- l'uscita forzata del worker è provata sostituendo `os._exit` con una funzione osservabile: si
  dimostra che venga chiamata, con quale codice e dopo quanta attesa. Che il processo muoia davvero e
  che l'attività pianificata lo riavvii si verifica solo su una postazione vera;
- il commit fallito è provato abortendo la transazione con un errore SQL vero, non fermando
  PostgreSQL: la garanzia «niente di parziale» è dimostrata, la prova con il servizio fermato è L8 e
  non è stata fatta;
- il confronto dei contratti è sui campi e sui loro generi, non sull'obbligatorietà: pydantic sa dire
  «questo campo non ha un default», in Go ogni campo ha il suo zero e la differenza non esiste.
  Confrontare i `required` darebbe una lista di disallineamenti finti, che è il modo più rapido per
  far ignorare un test;
- un test **saltato** non è un test superato. Se il corpus o il database mancano, la verifica
  corrispondente non è stata fatta, e il registro lo scrive con quella parola.

## 9. Che cosa resta aperto

Le voci 1.1–1.12 sono chiuse. Restano due cose dichiarate, nessuna delle due è un lavoro della fase 1:

**W2** — il cursore del worker avanza su `data_evento`, che per la Posta inviata è `SentOn`, mentre il
filtro della scansione usa `ReceivedTime`. È annotato nel codice nel punto in cui si trova e si chiude
in fase 2 con `MessaggioIn.ricevuto_il` (voce 2.1), insieme a `messaggio_casella.ricevuto_il`.

**I3 e I18** — la stessa mail in due o quattro caselle, elaborata una sola volta. Il piano li elenca
sotto la voce 1.11, ma senza `messaggio_casella` lo stesso messaggio in due caselle non è nemmeno
rappresentabile: appartengono alla fase 2 e non sono un debito di questa. La guardia che la 1.11 chiede
— nessun download se il file è già in staging con lo stesso hash — c'è già e vale anche per loro.
