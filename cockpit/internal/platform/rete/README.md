# `platform/rete` — certificato, filtro del listener, impronte dei token

## Scopo

Tre cose che servono perché server e worker si riconoscano senza una CA e senza segreti in chiaro:

- il **materiale TLS** del listener: carica il certificato e la chiave, o li genera autofirmati al primo
  avvio, e ne calcola l'impronta sha256 che i worker scrivono nel loro `worker.toml` (modello SSH);
- il **filtro delle reti consentite**, che chiude una connessione da fuori elenco appena accettata,
  prima del TLS;
- l'**impronta dei token** dei worker (`ImprontaToken`), la sola forma in cui un token sta in database, e
  la generazione di un token nuovo.

## Non appartiene qui

La decisione se usare TLS e quali reti ammettere (`platform/config`: `ETLS`, `leggiRete`), la
configurazione `tls.Config` e il listener vero (`app/runtime/ascolto.go`), l'autenticazione dei worker
(`transport/workerapi`), il pacchetto della postazione (`transport/web`).

## File

| File | Responsabilità |
|---|---|
| `tls.go` | `Materiale` (certificato, `Impronta`, `Nomi`, `Scadenza`, `Generato`, `PEM`) e `Copre`; `Prepara` (carica o genera), `carica`, `genera`; `DurataCertificato`; `ImprontaDi`; `NomiPredefiniti`; `ImprontaToken`, `ImprontaNonGenerata`, `TokenNuovo` |
| `filtro.go` | `SoloDalleReti` (avvolge un `net.Listener`), `Ammessa` (la regola), il tipo `filtro` con il freno al log dei rifiuti |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Prepara(cert, key, nomi)` | `app/runtime.PreparaTLS` (con `NomiPredefiniti` se `tls_nomi` è vuoto, più l'host di `url_pubblico`) |
| `Materiale` | `app/runtime.Ascolta` (in `tls.Config`), `transport/web` (impronta nella pagina Postazioni, `PEM` nel pacchetto) |
| `SoloDalleReti(ln, reti, rifiutata)` | `app/runtime.Ascolta`, solo se `[server].reti_consentite` non è vuota |
| `ImprontaToken` | `platform/fondazioni` (seed), `transport/workerapi` (autenticazione), `transport/web` (rigenerazione) |
| `ImprontaNonGenerata` | `platform/fondazioni`, `transport/workerapi` |
| `TokenNuovo` | `transport/web/postazioni_admin.go` |

## Dati

Nessuna tabella. Sul disco, solo quando i due file **mancano entrambi**: scrive la chiave privata
(PKCS#8, PEM, permessi `0600`) e poi il certificato (`0644`), creando le cartelle. Su Windows i permessi
POSIX non si applicano: restano le ACL della cartella.

## Flussi principali

**Il certificato** (`Prepara`):

1. i due file ci sono → `carica`: `tls.LoadX509KeyPair`, impronta del primo certificato, nomi (DNS e IP)
   e scadenza letti dal certificato; nessun controllo della scadenza;
2. ce n'è uno solo → errore che dice quale manca (rigenerare sovrascriverebbe l'altro);
3. nessuno dei due → `genera`: chiave ECDSA P-256, numero di serie casuale a 128 bit, validità da un'ora
   fa a dieci anni, `CommonName = cockpit`, SAN dai nomi (gli IP separati dai nomi DNS),
   `ExtKeyUsageServerAuth`, e `IsCA = true` con `KeyUsageCertSign` (è radice di se stesso).

Un certificato esistente non si rigenera mai da solo: se non nomina l'host di `url_pubblico` lo dice
`app/runtime.PreparaTLS` nel log (`Copre`). `Copre` confronta i nomi senza maiuscole e gli indirizzi per
valore.

**Il riconoscimento.** Il worker confronta lo sha256 del certificato (DER) con l'impronta del suo
`worker.toml`, e non verifica il nome. Il browser verifica la catena: `installa-postazione.ps1` mette il
`cert.pem` del pacchetto fra le autorità radice dell'utente. Il livello minimo del protocollo
(TLS 1.2) lo fissa `app/runtime.Ascolta`.

**Il filtro** (`SoloDalleReti`, `Ammessa`):

1. `Accept` del listener vero; indirizzo remoto e locale letti dalla connessione TCP (mai da un header);
2. entrambi riportati a IPv4 se sono in forma 4in6, e senza zona;
3. passa se il remoto è loopback, se è uguale all'indirizzo locale (il server che chiama se stesso), o
   se sta in una delle reti; un indirizzo illeggibile non passa;
4. altrimenti la connessione si chiude subito, prima dell'handshake, e `rifiutata` viene chiamata al
   massimo una volta al minuto per indirizzo (la mappa si svuota oltre mille voci); `Accept` riprende
   ad aspettare.

**I token.** `ImprontaToken` è lo sha256 esadecimale del token senza spazi ai lati; `ImprontaNonGenerata`
è l'impronta della stringa vuota, che nessuna richiesta può presentare perché l'autenticazione scarta
l'header vuoto prima di calcolarla. `TokenNuovo`: 32 byte casuali in base64url.

## Invarianti

- La chiave privata non si sovrascrive: con un solo file presente, `Prepara` si ferma.
- La chiave privata non esce da qui: `Materiale.PEM` contiene solo il certificato.
- Un rifiuto del filtro avviene prima del TLS: il client non riceve il certificato.
- Loopback e il server stesso passano sempre il filtro.
- Chi semina e chi verifica un token calcolano l'impronta con la stessa funzione.

## Dipendenze

Solo libreria standard. Importato da `app/runtime`, `platform/fondazioni`, `transport/web`,
`transport/workerapi`. Nessuna violazione.

## Test

L1, nessun tag:

| File | Che cosa prova |
|---|---|
| `tls_test.go` | un certificato generato porta il PEM pubblico (senza chiave), copre i nomi dati e non altri; ricaricato dal disco è lo stesso |
| `filtro_test.go` | `Ammessa` su IPv4, 4in6, IPv6, loopback, il server stesso, le zone; sul listener vero una connessione rifiutata non vede il TLS, una ammessa arriva a HTTP, il rifiuto si annota una volta |

Fuori dal package: `transport/workerapi/rete_db_test.go` e `transport/web/pacchetto_postazione_db_test.go`
(L4). Non provati: il caso di un solo file presente, un certificato scaduto.

## Stato dell'implementazione

Completo per la voce 2.4 (TLS e credenziali individuali), 7C.1 (PEM nel pacchetto, `Copre`) e l'avvio in
rete (`filtro.go`). Limiti noti: il certificato generato è una CA senza vincoli sui nomi, installata fra
le radici degli utenti; la scadenza di un certificato caricato non viene guardata; i nomi di un
certificato esistente non si aggiornano (si cancellano i due file e si rigenerano i pacchetti, perché
cambia l'impronta).

## Dove intervenire

| Voglio… | Apro |
|---|---|
| cambiare come nasce il certificato | `tls.go:genera` (poi i pacchetti delle postazioni vanno rigenerati) |
| cambiare i nomi predefiniti | `tls.go:NomiPredefiniti` |
| cambiare chi passa il filtro | `filtro.go:Ammessa` (e `config.leggiRete` per che cosa si può scrivere nel file) |
| cambiare come si conserva un token | `tls.go:ImprontaToken` (tutti i token esistenti smettono di valere) |

## Leggi anche

`internal/platform/config/README.md`, `internal/platform/fondazioni/README.md`, `internal/app/README.md`,
il README principale («Mettere il Cockpit in rete», «Avvio in rete»), `workers/workers_README.md`.
