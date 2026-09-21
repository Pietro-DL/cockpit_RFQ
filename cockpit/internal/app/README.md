# `app/` — il cablaggio e i processi di lungo periodo

## Scopo

Mettere insieme i pezzi e tenerli accesi: leggere la configurazione, applicare le migrazioni, seminare le
fondazioni, fissare le capacità, avviare scheduler, esecutore, cache e ricognitore, poi ascoltare.

## Stato alla fine del refactor A

**Questa cartella è vuota.** Oggi il cablaggio sta tutto in `cmd/cockpit/main.go`, e i processi di lungo periodo
`EsecutoreServer` e `Ricognitore` stanno in `internal/jobs`; `Scheduler` in `platform/coda` e `Cache`
in `platform/storage/staging`. Il README esiste da ora
perché il posto dove andranno è già deciso, e perché chi cerca «dove parte il server» non deve trovare una
cartella muta.

## Non appartiene qui (quando ci sarà)

Logica di dominio, handler HTTP, query.

## Package posseduti (previsti)

`runtime`: avvio, comandi amministrativi della riga di comando, `EsecutoreServer`, ascolto.

## Dipendenze consentite

Tutto. È l'unica area che può importare ogni altra.

## Entry point

Oggi: `cmd/cockpit/main.go`. Domani: `runtime.Esegui`, chiamato da un `main.go` di poche righe.

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

Oggi: L4 sull'esecutore e sulla ripresa, dentro `internal/jobs`; avvio a mano con `-migra`.

## Dove intervenire

| Voglio… | Apri (oggi) |
|---|---|
| aggiungere un servizio da avviare | `cmd/cockpit/main.go` |
| aggiungere un flag della riga di comando | `cmd/cockpit/main.go` |
| capire perché un job non parte all'avvio | `platform/coda/capacita.go:AllineaCoda` |

## Leggi anche

`internal/README.md`, `platform/README.md`, `transport/README.md`.

---

**Cambia in B**: nasce `app/runtime` — `main.go` si svuota in `runtime.Esegui` (B10a) e poi il `run` si
scompone in avvio, comandi e servizi (B10b); `EsecutoreServer` si sposta qui da `internal/jobs` (B6c).
