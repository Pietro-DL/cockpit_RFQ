# Fase 0 — fondazioni, migrazioni, ambiente di prova

Riepilogo tecnico di che cosa introduce questa fase, come si verifica e che cosa resta **non
verificato**. È il documento pubblico del ramo: la documentazione operativa completa (piani,
decisioni aperte, registri degli esiti, richieste all'IT) vive fuori da questo repository.

## Perimetro

Questa fase prepara il terreno per le successive e **non** cambia il comportamento del prodotto:
nessuna nuova funzionalità nell'interfaccia, nessuna modifica alla tabella `messaggio`, nessun
collegamento a caselle di posta reali. Tutto ciò che segue è stato provato su un database di prova
usa e getta.

## 1. Migrazioni incrementali — `internal/migrazioni`

Prima esisteva un solo file di schema applicato all'avvio: un secondo file veniva semplicemente
ignorato. Ora `migrations/NNNN_nome.sql` viene applicato in ordine, **una transazione per file**,
saltando ciò che risulta già registrato in `schema_versione`.

Tre difese, ognuna con il test che la dimostra:

| Difesa | Quando interviene | Perché |
|---|---|---|
| verifica statica | prima di aprire il database | rifiuta un file che non registra la propria versione, che referenzia una tabella creata in un file successivo, o che aggiunge un valore di enum e lo usa nello stesso file (PostgreSQL non lo accetta prima del commit) |
| controllo dentro la transazione | prima del commit di ogni file | un file che si dimentica di registrarsi viene annullato, invece di lasciare il registro incoerente |
| lock consultivo | prima di iniziare | due istanze non migrano insieme; un database più avanti del binario viene rifiutato invece di essere «riparato» |

`cockpit.exe -migra` applica migrazioni e seed e poi esce: è il modo di aggiornare un database prima
di sostituire il binario su una postazione.

### Aggiungere una migrazione

1. il nome è `NNNN_descrizione.sql`, con `NNNN` progressivo e senza salti;
2. l'ultima istruzione è `INSERT INTO schema_versione (versione) VALUES (NNNN);`
3. non si modifica un file già applicato altrove: se ne aggiunge uno nuovo;
4. niente `REFERENCES` verso tabelle che nascono in un file successivo;
5. un valore di enum aggiunto in un file non può essere usato nello stesso file.

Le regole sono verificate dal test statico, che gira dentro `go test ./...` sulle migrazioni vere del
repository: sbagliarle fa fallire la suite, non il primo aggiornamento in campo.

## 2. `0002_fondazioni` — quattro tabelle

| Tabella | A che serve |
|---|---|
| `casella` | una casella di posta: un solo identificativo anche quando più postazioni la aprono |
| `postazione` | un PC con un worker; base del routing dei job interattivi |
| `worker_credenziale` | credenziale individuale per worker (in database solo l'impronta sha256 del token) ed elenco delle caselle autorizzate |
| `casella_store` | come **ogni** postazione vede una casella nel proprio profilo locale |

L'ultima tabella esiste per un motivo preciso: l'identificativo di archivio di Outlook appartiene al
profilo, non alla casella. Rilevato su un PC, non è un riferimento valido su un altro; per questo i
payload dei job portano `casella_id` più l'identificativo dell'elemento, mai un archivio altrui.

I vincoli non sono decorativi e ognuno ha il suo test: indirizzi solo minuscoli, nomi host solo
maiuscoli, una casella condivisa non può avere un proprietario, l'impronta del token deve essere
esadecimale di 64 caratteri, la coppia canale + indirizzo è unica.

L'autorizzazione di un worker è un dato del server: l'elenco delle caselle vive in
`worker_credenziale` e viene intersecato con quanto il worker dichiara. Il contenuto della richiesta
non è una fonte di autorizzazione.

## 3. Configurazione e seed non distruttivo

`cockpit.toml` guadagna `[[casella]]`, `[[postazione]]`, `[[worker]]` e `[outlook].casella_default`
(vedere `cockpit.toml.example`). La lettura normalizza maiuscole e minuscole e **rifiuta l'avvio**
quando un riferimento non torna: un worker su una postazione non dichiarata, una casella predefinita
non censita, una condivisa con un proprietario, la stessa casella scritta in due modi diversi. Meglio
non partire che partire con un routing sbagliato.

Il seed è **non distruttivo**: aggiorna per chiave naturale, non cancella e **non disattiva** ciò che
sparisce dal file — lo segnala nel log e lascia decidere a una persona. Il token resta nel file di
configurazione; in database finisce soltanto la sua impronta.

## 4. Modulo comune dei worker — `workers/cockpit_client.py`

Raccoglie ciò che i due worker avevano copiato: client HTTP, configurazione, log rotante e il thread
di battito con flag di arresto. Gli errori HTTP ora si distinguono: `409` significa «questo tentativo
non vale più» e ferma il lavoro in corso al primo punto di ripresa, `4xx` «inutile ritentare», `5xx`
«riprova». Il nome del worker è stabile (`<tipo>@<HOST>`): prima conteneva il PID e cambiava a ogni
riavvio, il che lo rendeva inutilizzabile come chiave di una credenziale.

## 5. Strumenti di prova

| Strumento | Che cosa fa |
|---|---|
| `scripts/db-test.ps1` | cluster PostgreSQL isolato sotto `%LOCALAPPDATA%`, porta 5433, usa e getta |
| `internal/testutil` | pool, schema pulito, schema fino alla versione N; rifiuta un DSN il cui database non contenga «test» |
| `workers/server_finto.py` | finto server dell'API worker: nessun database, nessun Outlook |
| `scripts/prova-tutto.ps1` | esegue le prove automatiche e scrive il registro degli esiti |
| `scripts/backup-db.ps1` | backup **con prova di ripristino vera** in un database usa e getta e confronto dei conteggi |
| `scripts/installa-attivita.ps1` | attività pianificate per i worker, con riavvio automatico |

## 6. Come verificare

```powershell
cd cockpit
go build ./... ; go vet ./... ; go test ./...
powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Installa   # una volta: scarica PostgreSQL portatile
powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1
```

L'ultimo comando scrive un registro con l'esito di ogni prova, il commit completo e le versioni degli
strumenti usati.

I test d'integrazione condividono un solo database e alcuni ricreano lo schema: vanno eseguiti in
serie. Il flag `-p 1` non è un'ottimizzazione, è una condizione di correttezza.

```powershell
$env:COCKPIT_TEST_DSN = "postgres://cockpit_test:cockpit_test@127.0.0.1:5433/cockpit_test"
go test -tags integrazione -count=1 -p 1 ./...
```

## 7. Che cosa è verificato e che cosa no

| Livello | Copertura | Stato |
|---|---|---|
| L1 unitari Go | dominio, percorsi NAS, archivi, template, verifica statica delle migrazioni, configurazione | eseguiti |
| L2 unitari Python | modulo comune, ciclo dei worker, analisi documenti | eseguiti; i casi sul corpus riservato risultano **saltati**, perché il corpus non sta nel repository |
| L3 contratti | conformità fra gli schemi JSON di `contracts/`, i tipi Go e i modelli pydantic | **non eseguito**: il test non esiste ancora |
| L4 integrazione | migrazioni, seed, ingest e interfaccia su PostgreSQL di prova | eseguiti |
| L5–L9 | Outlook via COM, Exchange, due postazioni, posta reale | **non eseguiti**: richiedono account e macchine non disponibili qui |

Due precisazioni che valgono anche per chi legge solo questo file:

- `go build` dimostra che il codice compila. Non dimostra la conformità agli schemi di `contracts/`:
  i tipi Go e i modelli pydantic possono compilare benissimo e descrivere contratti diversi. Finché
  il test L3 non esiste, quel livello resta scoperto.
- un test **saltato** non è un test superato. Se il corpus o il database mancano, la verifica
  corrispondente non è stata fatta, e il registro lo scrive con quella parola.
