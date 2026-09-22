# `app/` — il cablaggio e i processi di lungo periodo

## Scopo

Mettere insieme i pezzi e tenerli accesi: leggere la configurazione, applicare le migrazioni, seminare le
fondazioni, fissare le capacità, avviare scheduler, esecutore, cache e ricognitore, poi ascoltare.

## Stato

Da B6c `app/runtime` contiene l'**esecutore dei job del server**, in due file: chi esegue (`esecutore.go`) e
la vigilanza sul NAS assente (`vigilanza_nas.go`). Da B10a contiene anche l'**avvio**: `esegui.go` tiene
`Opzioni` e `Esegui`, ciò che era `run` in `cmd/cockpit/main.go`. Il main è rimasto di trentasei righe: i
flag, le Opzioni che ne nascono, il codice di uscita.

Gli altri processi di lungo periodo stanno dove sta la cosa che governano: `Scheduler` in `platform/coda`,
`Cache` in `platform/storage/staging`, `Ricognitore` in `core/rfq/documenti`.

## Non appartiene qui

Logica di dominio, handler HTTP, query. `esecutore.go` non decide niente: riconosce il tipo del job e chiama
chi sa farlo.

## Package posseduti

| Package | Che cosa fa |
|---|---|
| `runtime` | `esegui.go`: `Esegui(cfgPath, Opzioni)`, l'avvio nel suo ordine — configurazione, log, database e migrazioni, semi, capacità, i comandi che escono prima dell'ascolto, servizi, TLS, listener. `esecutore.go`: prende dalla coda i job con `worker_tipo='server'` e li esegue in una goroutine — il fascicolo a `core/rfq/documenti`, gli archivi a `transport/workerapi` (interfaccia `Estrattore`), l'analisi a `ai/agente`. `vigilanza_nas.go`: quali tipi di job vogliono il NAS (`ScrivePerNas`), il **rinvio** senza consumare il tentativo quando non c'è, il guardiano che vede quando torna e le copie esaurite che tornano in coda |

## Invariante dell'esecutore

Un NAS irraggiungibile non è un errore del job: è una condizione del mondo. Far fallire quei job
consumerebbe i tentativi e chiuderebbe copie che non hanno niente che non va.

## Dipendenze consentite

Tutto. È l'unica area che può importare ogni altra.

## Entry point

`runtime.Esegui` (chiamata da `cmd/cockpit/main.go`), `runtime.EsecutoreServer.Avvia` e
`RiaccodaAlRitornoDelNas`.

## Flussi principali

L'ordine dell'avvio: config → log → db e migrazioni → semina delle fondazioni → capacità
(`ImpostaCapacita`, `AllineaCoda`) → ricalcolo delle controparti → eventuale comando amministrativo (e uscita)
→ TLS → scheduler, cache, esecutore, ricognitore → ascolto con `web` e `workerapi`.

## Invarianti

- Le capacità si fissano **prima** che scheduler ed esecutore partano: nessun job parte con una capacità non
  ancora letta.
- Il server non semina mai da solo: seminare è un comando esplicito che si ferma prima del listener.
- Senza TLS non si ascolta fuori dal loopback, salvo `consenti_lan_in_chiaro` dichiarato.
- I comandi amministrativi che scrivono chiudono con il ritriage delle sole chiavi appena scritte.

## Effetti collaterali

Tutti quelli dei servizi che avvia.

## Test

L4 su `runtime`: l'esecuzione della copia sul NAS, la ripresa di un contenuto sparito, il NAS assente e il
ritorno del NAS, l'estrazione affidata all'estrattore. Quelle prove restano qui, e non con `core/rfq/documenti`,
perché guardano l'esecutore e il fascicolo insieme: è l'integrazione fra i due che dev'essere verde. Avvio reale sul database di prova a ogni commit che
tocca l'avvio.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| aggiungere un servizio da avviare | `runtime/esegui.go:Esegui` |
| aggiungere un tipo di job eseguito dal server | `runtime/esecutore.go:esegui` e chi sa farlo |
| dire che un tipo di job ha bisogno del NAS | `runtime/vigilanza_nas.go:ScrivePerNas` |
| aggiungere un flag della riga di comando | `cmd/cockpit/main.go` (il flag) e `runtime.Opzioni` (il campo) |
| capire perché un job non parte all'avvio | `platform/coda/capacita.go:AllineaCoda` |

## Leggi anche

`internal/README.md`, `platform/README.md`, `transport/README.md`.

---

**Cambia in B**: `EsecutoreServer` è arrivato (B6c) e `main.go` si è svuotato in `runtime.Esegui` (B10a).
Resta la scomposizione di `Esegui` in avvio, comandi e servizi (B10b).
