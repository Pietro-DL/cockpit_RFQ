# `platform/storage` — i file: archivi, NAS, staging

## Scopo

Tre package che toccano il disco e non sanno che cosa sia una RFQ:

| Package | Che cosa fa |
|---|---|
| `archivio` | scompatta uno zip in voci singole, con un budget sui byte scritti e senza uscire dalla cartella |
| `nas` | l'unico scrittore del NAS: copia per `.parte.<token>`, verifica dell'hash, promozione senza sostituire |
| `staging` | la cartella di lavoro del server: i `.parte` dei tentativi, i contenuti per sha256, le estrazioni, il custode della cache |

## Non appartiene qui

Il nome di una cartella o di un file sul NAS, il confinamento dei percorsi letti e il ricognitore
dell'integrità (`core/rfq/documenti`); chi decide quando copiare, estrarre o scaricare (`app/runtime`,
`platform/coda`, `transport/workerapi`); la cartella di staging dichiarata (`platform/config`,
`[nas].staging`).

---

## `storage/archivio`

### File

| File | Responsabilità |
|---|---|
| `zip.go` | `Voce`, `ErrLimite`; `Estrai` (1000 voci, 500 MB), `EstraiCon` (limiti espliciti); `dentro` (confronto con `filepath.Rel`), `ignora` (`__MACOSX`, `.DS_Store`, `Thumbs.db`, `desktop.ini`, `._*`), `scrivi` (copia con budget e sha256), `nomeSicuro` |

### Entry point

`Estrai(zipPath, destDir)`: `transport/workerapi/archivi.go`, che scompatta in
`staging.PercorsoEstrazione` e poi porta ogni voce fra i contenuti.

### Dati

Solo disco: ogni voce diventa `destDir/NNN_<nome ripulito>` (NNN = posizione nello zip), con lo sha256
calcolato mentre si scrive.

### Flussi principali

Per ogni voce dello zip: salta cartelle e file di sistema; oltre `limiteVoci` → `ErrLimite`; percorso
interno normalizzato (`\` → `/`, `path.Clean`) e scartato se esce (`../`) o è assoluto; nome finale
(`nomeSicuro`) con i caratteri vietati da Windows sostituiti con `_`, senza spazi ai lati, al massimo 120
**caratteri** (non byte: il taglio non cade a metà di una lettera accentata e il nome resta UTF-8 valido),
`voce` se resta vuoto; `scrivi` legge al più `restante + 1` byte:
se arriva quel byte in più il file parziale si toglie e l'estrazione si ferma con `ErrLimite`. Le voci già
estratte vengono restituite anche con l'errore.

### Invarianti

- Il budget si misura sui byte scritti, non sulle dimensioni dichiarate nello zip.
- Nessuna voce esce da `destDir`.
- Un file troncato non resta sul disco.
- Il nome di una voce sul disco è UTF-8 valido.

### Test (L1)

`zip_test.go` (estrazione, file ignorati), `limiti_test.go` (budget, budget esatto, numero di voci,
`dentro` che non si fa ingannare dal prefisso, zip-slip), `nome_test.go` (`nomeSicuro` taglia a 120
caratteri anche con lettere accentate, tiene l'inizio del nome, sostituisce i caratteri vietati, `voce`
per un nome vuoto).

### Stato

Completo per lo zip; 7z e rar non sono gestiti.

---

## `storage/nas`

### File

| File | Responsabilità |
|---|---|
| `nas.go` | `Scrittore{Radice, DryRun}`; `CreaCartella`, `Copia` (con `creaParte`, `promuovi`, `copiaEsclusiva`, `giaPresente`), `PartiNellaCartella`, `PulisciParti`, `RimuoviCartellaVuota`, `Raggiungibile`; `UNC`; `Sha256File`, `Sha256Da`; `TokenDiParte`, `Parte`; `ErrConflitto` |

### Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Scrittore` | costruito da `app/runtime.CostruisciServizi` con `DryRun = !nas_scrittura` |
| `CreaCartella` | `core/rfq/documenti/cartella_thread.go` |
| `Copia` | `core/rfq/documenti/copia_nas.go` |
| `PulisciParti` | `core/rfq/documenti/copia_nas.go` (prima di scrivere nella stessa cartella) |
| `RimuoviCartellaVuota` | `core/rfq/documenti/cartelle.go` |
| `Raggiungibile` | `app/runtime` (avvio, `vigilanza_nas.go`), `core/rfq/documenti/integrita.go`, `transport/web` (`/healthz`, anteprima, Integrità NAS) |
| `UNC` | `core/rfq/documenti` (`nas_percorso.go`, `integrita.go`) |
| `Sha256File`, `Sha256Da` | `core/rfq/documenti`, `transport/workerapi`, `platform/storage/staging` |
| `ErrConflitto` | `app/runtime/esecutore.go` (un conflitto è un fallimento definitivo) |

### Dati

Solo il NAS (o la cartella che ne fa le veci): cartelle della RFQ, file definitivi,
`<destinazione>.parte.<token>` durante la copia. Nessuna tabella.

### Flussi principali

**`Copia(src, relativoThread, relativoDoc, shaAtteso, token)`**:

1. destinazione = `UNC(radice, relativoThread\relativoDoc)`; con `DryRun` si restituisce e basta;
2. la destinazione esiste → stesso hash: niente da fare (`creato = false`); hash diverso: `ErrConflitto`,
   il file non si tocca;
3. cartelle create; copia in `<destinazione>.parte.<token>` aperto con `O_EXCL`;
4. hash del `.parte` ricalcolato: diverso dall'atteso → `.parte` tolto, errore;
5. `promuovi`: hard link `.parte` → destinazione (fallisce se la destinazione è comparsa nel frattempo),
   poi il `.parte` si toglie; dove il link non si può fare, creazione esclusiva della destinazione, copia
   e nuova verifica (e se non torna la si toglie, perché l'ha creata questa chiamata).

**`UNC(radice, relativo)`**: radice senza `\` finale + `\` + relativo senza `\` iniziale; da 250 caratteri
in su aggiunge `\\?\` (o `\\?\UNC\` per `\\server\share`), mai due volte. Non verifica che il relativo
resti sotto la radice.

**`PulisciParti(relativa, vivi, limite)`**: toglie da una cartella i `.parte.<uuid>` il cui token non è
fra i vivi **e** più vecchi di `limite`; un `.parte` senza token valido non si tocca.

### Invarianti

- Il NAS non si sovrascrive: `.parte` esclusivo, verifica dell'hash, promozione per hard link o creazione
  esclusiva.
- Due tentativi dello stesso job non condividono un `.parte`.
- `DryRun` non scrive e non toglie niente (`CreaCartella`, `Copia`, `PulisciParti`, `RimuoviCartellaVuota`).
- Una cartella si toglie solo se è vuota (`os.Remove`, mai `RemoveAll`).

### Test (L1)

`nas_test.go`: copia idempotente e conflitto, la promozione che non sostituisce un file comparso, due
tentativi con due `.parte`, i `.parte` scaduti tolti e quelli vivi no, `DryRun`, `UNC` (corto, locale,
lungo di rete, lungo locale, niente prefisso doppio).

### Stato

Completo su Windows (percorsi locali e UNC). I separatori sono `\` scritti nel codice: sul server Linux
previsto dal D23 (NAS montato con un percorso POSIX) la composizione dei percorsi non è stata provata.

---

## `storage/staging`

### File

| File | Responsabilità |
|---|---|
| `upload.go` | `CartellaParti` (`_parti`), `CartellaContenuti` (`_contenuti`); `PercorsoParte`, `PercorsoContenuto` (`_contenuti/<ab>/<sha256>.<ext>`, `ErrSha256`), `ContenutoGiaPresente` (ricalcola l'hash), `TokenDiParte`, `Promuovi` (`os.Rename`), `RimuoviParte`, `PulisciParti` e `pulisciZipInterrotti`, `PercorsoEstrazione` (`_parti/zip.<token>`) |
| `cache.go` | `Cache` (il custode di `_contenuti`) con `Avvia`, `Giro`, `raccogli`, `interroga`, `rimuovi`; `EsitoCache`; `ToccaContenuto` |
| `stage.go` | l'interfaccia `Staging` e `FileStaging` (il file c'è ancora?); `CartellaStaging` (hash breve del Message-ID, solo per il log del worker) |

### Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `PercorsoParte`, `PercorsoContenuto`, `ContenutoGiaPresente`, `Promuovi`, `RimuoviParte` | `transport/workerapi` (upload e result di `stage_allegato`, estrazione), `transport/web/caricamento.go` (caricamento dal Fascicolo) |
| `PercorsoEstrazione` | `transport/workerapi/archivi.go` |
| `PulisciParti` | lo scheduler di `platform/coda`, ogni 15 minuti, con età minima 10 minuti |
| `Cache.Avvia` | `app/runtime.CostruisciServizi`, con `RetentionCache()` e `CacheMaxByte()` della configurazione |
| `ToccaContenuto` | `core/rfq/documenti/copia_nas.go`, dopo una copia |
| `Staging`, `FileStaging` | `platform/coda.AccodaStage` e chi lo chiama (`ingest`, `web`, `core/rfq`), `transport/workerapi` |
| `CartellaStaging` | `platform/coda/stage.go` (payload di `stage_allegato`) |

### Dati

Disco, sotto `[nas].staging` (un valore relativo si legge dalla cartella di `cockpit.toml`; assente =
`<cartella di cockpit.toml>\staging`):

```
<staging>\_parti\<allegato>.parte.<lease_token>   trasferimento in corso
<staging>\_parti\zip.<lease_token>\               estrazione in corso
<staging>\_contenuti\<ab>\<sha256>.<ext>           contenuto verificato (estensione dal nome, altrimenti .bin)
```

Database (solo `Cache` e `PulisciParti`):

| Query | Che cosa |
|---|---|
| `ListLeaseTokenInCorso` (`job.sql`) | i token dei job `in_corso`: i loro `.parte` non si toccano |
| `UltimoUsoContenuti` (`cache.sql`) | l'uso più recente di un hash in `allegato` e `documento` |
| `ListVociDiArchivio` | le voci estratte da un archivio (un archivio resta se una voce è pinnata) |
| `ListContenutiPinnati` | i motivi per non togliere: documento in attesa di copia, anomalia NAS aperta, proposta aperta, job `pronto` o `in_corso` che lo cita |
| `AzzeraPathStagingPerSha` | `allegato.path_staging = NULL` per quell'hash, nella stessa transazione della cancellazione |

### Flussi principali

**Un upload** (chi chiama è `workerapi`): il worker carica in `PercorsoParte` con il token del suo
tentativo; il result valido, dentro la transazione che blocca il job, calcola `PercorsoContenuto` dallo
sha256 verificato; se il contenuto c'è già (`ContenutoGiaPresente`) il `.parte` si toglie, altrimenti
`Promuovi` lo rinomina.

**La pulizia dei `.parte`** (`PulisciParti`): legge i token vivi; toglie le cartelle `_parti/zip.<token>`
orfane e più vecchie dell'età minima; poi percorre **tutto** lo staging e toglie i file
`*.parte.<uuid>` orfani e vecchi.

**La cache** (`Cache.Giro`, prima passata dopo 2 minuti, poi ogni `Ogni`, 6 ore di default):

1. `raccogli`: i soli file di `_contenuti` il cui nome è uno sha256; ultimo uso = orario del file;
2. `interroga`: ultimo uso dal database (vince il più recente), pin per hash, pin ereditato
   dall'archivio da una sua voce;
3. dal meno usato: si salta ciò che è pinnato e ciò che è più giovane di `EtaMinima` (10 minuti); si
   toglie ciò che è fermo oltre `Retention`, oppure, finché la cache supera `MaxByte`, il meno usato;
4. `rimuovi`: in una transazione, azzera `path_staging` degli allegati con quell'hash, cancella il file,
   commit; se il file non si cancella il database non cambia.

### Invarianti

- Un contenuto si chiama con il proprio sha256 ed è di tutti gli allegati che lo hanno; il nome non può
  uscire dallo staging (`reSha256`, `reEstensione`).
- Un `.parte` di un tentativo vivo non si tocca, qualunque età abbia.
- Un contenuto già presente si riusa solo se il suo hash ricalcolato torna.
- La cache non entra nello staging vecchio per messaggio e non tocca un file che non si chiama come un hash.
- Un contenuto pinnato, o nato da meno di `EtaMinima`, non si toglie nemmeno oltre la capienza.
- Chi serve un file dello staging fuori dal server lo prende solo se `allegato.path_staging` sta sotto la
  radice dello staging: i byte a un worker (`transport/workerapi/contenuto.go:nelloStaging`, altrimenti
  410) e l'anteprima nel browser (`transport/web/anteprima.go:nelloStaging`). La regola sta lì, non in
  questo package.

### Test

| File | Livello | Che cosa prova |
|---|---|---|
| `upload_test.go` | L1 | `PercorsoContenuto`, l'estensione, `PercorsoParte`, `ContenutoGiaPresente` che ricalcola l'hash |
| `contenuti_db_test.go` | L4 | la cache: i quattro pin (ST1), rimozione per età con i percorsi azzerati (ST2), archivio pinnato da una voce (ST3), capienza (ST4), retention zero, contenuto appena nato, staging vecchio |
| `parti_db_test.go` | L4 (package `staging_test`) | `PulisciParti`: solo gli orfani vecchi, le estrazioni interrotte |
| `comune_db_test.go`, `comune_esterno_db_test.go` | L4 | l'impalcatura dei due package di test (interno ed esterno, per poter accodare con `coda` senza ciclo) |

### Stato

Completo per il blocco 4A (staging per contenuto), la voce 2.3 (upload legato al tentativo) e Pre-7 (la
cache). `CartellaStaging` sopravvive solo come diagnosi nel payload di `stage_allegato`.

---

## Dipendenze

| Package | Importa | È importato da |
|---|---|---|
| `archivio` | solo libreria standard | `transport/workerapi` |
| `nas` | libreria standard, `google/uuid` | `app/runtime`, `core/rfq/documenti`, `platform/storage/staging`, `transport/web`, `transport/workerapi` |
| `staging` | `platform/db`, `platform/storage/nas` | `platform/coda` (eccezione dichiarata: l'interfaccia `Staging`), `app/runtime`, `core/inbox/ingest`, `core/rfq/documenti`, `core/rfq/fascicolo`, `transport/web`, `transport/workerapi` |

Nessuna violazione della tabella di `internal/README.md`.

## Dove intervenire

| Voglio… | Apro |
|---|---|
| cambiare i limiti di un archivio | `archivio/zip.go` (`maxVoci`, `maxByteTotali`) |
| gestire un altro formato di archivio | `archivio` (oggi solo zip) e `transport/workerapi/archivi.go` |
| cambiare come si scrive sul NAS | `nas/nas.go:Copia` e `promuovi` |
| cambiare la soglia del prefisso long-path | `nas/nas.go:UNC` |
| capire perché un contenuto è sparito dallo staging | il log «contenuto rimosso dalla cache», `cache.go:Giro`, `ListContenutiPinnati` in `cache.sql` |
| capire perché un contenuto non se ne va | `ListContenutiPinnati` (il motivo è nel log della passata come «pinnati») |
| cambiare dove sta un contenuto | `staging/upload.go:PercorsoContenuto` (i percorsi già scritti in `allegato.path_staging` restano quelli) |

## Leggi anche

`internal/platform/README.md`, `internal/core/README.md` (`rfq/documenti`), `internal/transport/README.md`
(le rotte di upload e dei contenuti), `internal/app/README.md` (l'esecutore), il README principale
(«Com'è fatto lo staging», «Integrità NAS»).
