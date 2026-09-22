# `app/` — il cablaggio e i processi di lungo periodo

## Scopo

Mettere insieme i pezzi e tenerli accesi: leggere la configurazione, applicare le migrazioni, seminare le
fondazioni, fissare le capacità, avviare scheduler, cache, esecutore e ricognitore, poi ascoltare.

Gli altri processi di lungo periodo stanno dove sta la cosa che governano: `Scheduler` in `platform/coda`,
`Cache` in `platform/storage/staging`, `Ricognitore` in `core/rfq/documenti`. Qui c'è chi li costruisce e
chi li fa partire, non come funzionano.

## Non appartiene qui

Logica di dominio, handler HTTP, query. `esecutore.go` non decide niente: riconosce il tipo del job e chiama
chi sa farlo.

## Package posseduti

| Package | Che cosa fa |
|---|---|
| `runtime` | l'avvio e l'esecutore dei job del server, in sette file (sotto) |

L'avvio:

| File | Che cosa tiene |
|---|---|
| `esegui.go` | `Opzioni` e `Esegui(cfgPath, Opzioni)`: la sequenza dell'avvio, un passo per riga, e niente altro |
| `avvio.go` | `ApriLog`, `ApriDatabase`, `Semina`, `ImpostaCapacita`: i passi che vengono prima che esista un servizio |
| `comandi.go` | `SeminaAnagrafica`, `SemeFornitori`, `ContaAnagrafiche`: i lavori della riga di comando, che fanno il loro e poi escono |
| `servizi.go` | `Servizi` e `CostruisciServizi`: scrittore NAS, scheduler, cache, agente, esecutore, ricognitore, ingest |
| `ascolto.go` | `PreparaTLS`, `Ascolta`: il materiale del certificato, il mux con le rotte di `web` e `workerapi`, il listener |

L'esecuzione:

| File | Che cosa tiene |
|---|---|
| `esecutore.go` | `EsecutoreServer`: prende dalla coda i job con `worker_tipo='server'` e li esegue in una goroutine — il fascicolo a `core/rfq/documenti`, gli archivi a `transport/workerapi` (interfaccia `Estrattore`), l'analisi a `ai/agente` |
| `vigilanza_nas.go` | `ScrivePerNas` (quali tipi di job vogliono il NAS), il **rinvio** senza consumare il tentativo quando non c'è, il guardiano che vede quando torna, le copie esaurite che tornano in coda |

`cmd/cockpit/main.go` non è di quest'area e non fa niente di tutto questo: legge i flag, compila `Opzioni`,
chiama `runtime.Esegui` e traduce l'errore in un codice di uscita.

## Dipendenze consentite

Tutto. È l'unica area che può importare ogni altra — `core`, `ai`, `platform` e `transport` insieme — perché
è quella che monta il processo. Nessuno la importa: solo `cmd/cockpit`.

## Entry point

`runtime.Esegui` (chiamata da `cmd/cockpit/main.go`), `runtime.EsecutoreServer.Avvia` e
`RiaccodaAlRitornoDelNas`.

## Flussi principali

**L'avvio**, nell'ordine in cui `Esegui` lo scrive:

configurazione → log (stdout + file) → pool e migrazioni → semina degli utenti e delle fondazioni →
capacità (`ImpostaCapacita`, `AllineaCoda`) → ricalcolo delle controparti → **eventuale comando
amministrativo, e qui si esce** → servizi (scheduler e cache partono subito) → TLS → ascolto, dove nascono
`web.Server` e `workerapi.Server`, l'esecutore riceve l'estrattore e parte insieme al ricognitore.

**Il job del server**: `coda.Claim` con `worker_tipo='server'` → se il job vuole il NAS e il NAS non c'è, si
rinvia senza consumare il tentativo → `esegui` riconosce il tipo e chiama chi sa farlo → `coda.Completa` o
`coda.Fallisci`.

## Invarianti

- Le capacità si fissano **prima** che scheduler ed esecutore partano: nessun job parte con una capacità non
  ancora letta.
- Il server non semina mai da solo: seminare è un comando esplicito che si ferma prima del listener.
- Senza TLS non si ascolta fuori dal loopback, salvo `consenti_lan_in_chiaro` dichiarato.
- I comandi amministrativi che scrivono chiudono con il ritriage delle sole chiavi appena scritte.
- Un NAS irraggiungibile non è un errore del job: è una condizione del mondo. Far fallire quei job
  consumerebbe i tentativi e chiuderebbe copie che non hanno niente che non va.

## Effetti collaterali

Tutti quelli dei servizi che avvia, più i suoi: il file di log, il pool verso PostgreSQL, il certificato
generato al primo avvio, il listener sulla porta dichiarata.

## Test

L4 su `runtime`: l'esecuzione della copia sul NAS, la ripresa di un contenuto sparito, il NAS assente e il
ritorno del NAS, l'estrazione affidata all'estrattore. Quelle prove restano qui, e non con
`core/rfq/documenti`, perché guardano l'esecutore e il fascicolo insieme: è l'integrazione fra i due che
dev'essere verde.

L'avvio non ha test automatici: si verifica a mano sul database di prova, `-migra` e poi avvio completo con
`GET /healthz`, confrontando il log riga per riga con quello di prima. Va fatto a ogni commit che tocca
l'avvio.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| capire in che ordine nasce il server | `runtime/esegui.go:Esegui`, che si legge dall'alto in basso |
| aggiungere un servizio da avviare | `runtime/servizi.go:CostruisciServizi`, e `Avvia` se deve partire dopo le rotte |
| aggiungere un tipo di job eseguito dal server | `runtime/esecutore.go:esegui` e chi sa farlo |
| dire che un tipo di job ha bisogno del NAS | `runtime/vigilanza_nas.go:ScrivePerNas` |
| aggiungere un flag della riga di comando | `cmd/cockpit/main.go` (il flag), `runtime.Opzioni` (il campo), `runtime/comandi.go` (che cosa fa) |
| cambiare una rotta o il suo montaggio | `runtime/ascolto.go:Ascolta`, poi `transport/web` |
| capire perché un job non parte all'avvio | `platform/coda/capacita.go:AllineaCoda` |

## Leggi anche

`internal/README.md`, `platform/README.md`, `transport/README.md`.
