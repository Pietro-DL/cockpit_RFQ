# `app/` — il cablaggio e i processi di lungo periodo

## Scopo

Mettere insieme i pezzi e tenerli accesi: leggere la configurazione, applicare le migrazioni, seminare le
fondazioni, fissare le capacità, avviare scheduler, esecutore, cache e ricognitore, poi ascoltare.

## Stato

Da B6c `app/runtime` esiste e contiene l'**esecutore dei job del server**. Il cablaggio dell'avvio sta ancora
in `cmd/cockpit/main.go` e arriva qui in B10a.

Gli altri processi di lungo periodo stanno dove sta la cosa che governano: `Scheduler` in `platform/coda`,
`Cache` in `platform/storage/staging`, `Ricognitore` in `core/rfq/documenti`.

## Non appartiene qui

Logica di dominio, handler HTTP, query. `esecutore.go` non decide niente: riconosce il tipo del job e chiama
chi sa farlo.

## Package posseduti

| Package | Che cosa fa |
|---|---|
| `runtime` | `esecutore.go`: prende dalla coda i job con `worker_tipo='server'` e li esegue in una goroutine — il fascicolo a `core/rfq/documenti`, gli archivi a `transport/workerapi` (interfaccia `Estrattore`), l'analisi a `ai/agente`. Vigila sul NAS assente: un job che tocca il NAS quando il NAS non c'è si **rinvia**, non fallisce, e al ritorno le copie esaurite tornano in coda |

## Invariante dell'esecutore

Un NAS irraggiungibile non è un errore del job: è una condizione del mondo. Far fallire quei job
consumerebbe i tentativi e chiuderebbe copie che non hanno niente che non va.

## Dipendenze consentite

Tutto. È l'unica area che può importare ogni altra.

## Entry point

`runtime.EsecutoreServer.Avvia` e `RiaccodaAlRitornoDelNas`. L'avvio è ancora `cmd/cockpit/main.go`;
diventa `runtime.Esegui` in B10a.

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
ritorno del NAS, l'estrazione affidata all'estrattore. Avvio reale sul database di prova a ogni commit che
tocca l'avvio.

## Dove intervenire

| Voglio… | Apri (oggi) |
|---|---|
| aggiungere un servizio da avviare | `cmd/cockpit/main.go` |
| aggiungere un tipo di job eseguito dal server | `runtime/esecutore.go:esegui` e chi sa farlo |
| aggiungere un flag della riga di comando | `cmd/cockpit/main.go` |
| capire perché un job non parte all'avvio | `platform/coda/capacita.go:AllineaCoda` |

## Leggi anche

`internal/README.md`, `platform/README.md`, `transport/README.md`.

---

**Cambia in B**: `EsecutoreServer` è arrivato (B6c). Resta l'avvio: `main.go` si svuota in `runtime.Esegui`
(B10a) e poi il `run` si scompone in avvio, comandi e servizi (B10b).
