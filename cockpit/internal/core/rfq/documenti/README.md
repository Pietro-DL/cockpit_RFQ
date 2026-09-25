# `internal/core/rfq/documenti` — il fascicolo di una RFQ sul NAS

## Scopo

Come si chiamano le cartelle e i file di una RFQ sul NAS, che cosa significa copiarci un documento
confermato e riprenderne il contenuto quando la cache lo ha perso, e se quello che il database dichiara
scritto c'è davvero (il ricognitore dell'integrità, la verifica di chi sta per servire i byte).

Il package decide il **cosa**; il **chi** e il **quando** stanno altrove: i job li esegue
`app/runtime` (`EsecutoreServer`), i gesti dell'operatore arrivano da `transport/web`. Le scritture
fisiche sul disco passano tutte da `platform/storage/nas` (`nas.Scrittore`).

## Non appartiene qui

- La coda, il claim, il lease, il rinvio con il NAS assente: `platform/coda`, `app/runtime`.
- La scrittura vera del file (`.parte.<token>`, verifica dell'hash, promozione senza sostituzione,
  prefisso long-path `\\?\`): `platform/storage/nas`.
- La cache dei contenuti e il suo custode: `platform/storage/staging`.
- La BOM, i componenti, le revisioni dei documenti, le versioni: `core/rfq/fascicolo`.
- La correzione di un codice (quali documenti, quale percorso nuovo, quando si rifiuta): oggi sta in
  `transport/web/fascicolo.go` (`percorsi`, `ripercorri`), che usa i mattoni di qui.
- La cartella libera di una RFQ che nasce (« (2)», « (3)»… se un'altra RFQ ha già il nome calcolato da
  `CartellaThread`, sotto un advisory lock sul nome): `transport/web/triage.go:cartellaLibera`.

## File

| File | Responsabilità |
|---|---|
| `path.go` | I nomi puri. `NomeSicuro` (nome Windows valido, troncato; punti e spazi finali tolti anche dopo il taglio), `CartellaThread` (`<cliente>\WIP\<aaaa mm gg> <cognome buyer> <oggetto ripulito>`), `LayoutDocumento`, `CartellaDocumento` (sottocartella del tipo + cartella del codice, `ErrCodiceMancante`), `PathDocumento`, `NellaCartella`, `NomeNelPercorso`, `NomeFileSicuro` (estensione in minuscolo, idempotente). Commento `// Package` (che cosa il package usa della coda) |
| `nomi_nas.go` | I nomi dopo A4. `Tecnico`, `NomeTecnico` (`<CODICE>_REV_<REV>.<ext>`, `ND` se la revisione manca), `NomeSulNas`, `ConProgressivo` (`_2`, `_3`…); `ScegliPercorso` / `RiscegliPercorso` / `BloccaCartella` (il nome libero sotto il lucchetto della cartella); `AccodaSpostamento`, `PayloadSpostamento`, `ErrSpostamentoInCorso` (passo 0 di `sposta_nas`: il commento dice di non chiamarlo prima di B8.8) |
| `cartelle.go` | `RimuoviCartella`: toglie una cartella del fascicolo solo se nessuno la nomina e se è vuota; `ErrCartellaReferenziata`, `ErrCartellaNonVuota` |
| `cartella_thread.go` | `CreaCartellaThread`: il corpo del job `crea_cartella_thread` |
| `copia_nas.go` | `CopiaSulNas`: il corpo del job `copia_nas` (un documento già `scritto` con il file giusto al suo posto esce subito); `SorgenteStaging` / `cercaSorgente` (l'allegato del thread con lo stesso sha256 e il file ancora presente; per un caricamento a mano sparito il messaggio dice di ricaricarlo dal Fascicolo); `PulisciPartiScadute`; `ErrContenutoMancante` |
| `ripresa.go` | `RiprendiContenuto`: contenuto sparito dalla cache → riestrazione dell'archivio o download da Outlook accodato; per un file caricato a mano nessun job e il messaggio «ricaricalo dal Fascicolo» (`contenutoCaricatoAMano`); resa se il download è già fallito per questa copia. Accetta `j` nil |
| `integrita.go` | `Ricognitore` (`Avvia`, `Giro`, `PerPassata`; il campo interno `mentreGuarda` serve alle prove), `EsitoRicognizione`, `controllo.esamina` (il verdetto su un documento), `AllineaDocumento` (gesto dell'amministratore) |
| `nas_percorso.go` | `PercorsoSulNas` (radice + cartella della RFQ + `path_relativo`, verificato dentro la radice), `Percorso` (relativo e assoluto), `DentroLaRadice` |
| `verifica.go` | `VerificaFileAperto` (hash ricalcolato sul file già aperto, prima di servirlo), `Segnala` (apre o aggiorna la riga di `nas_anomalia`), `ErrNonCorrisponde` |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `CreaCartellaThread` | `app/runtime/esecutore.go:esegui` (job `crea_cartella_thread`, accodato da `transport/web/triage.go` alla creazione della RFQ) |
| `CopiaSulNas` | `app/runtime/esecutore.go:esegui` (job `copia_nas`) |
| `Ricognitore` | costruito in `app/runtime/servizi.go:CostruisciServizi`, avviato da `Servizi.Avvia`; `Giro` e `PerPassata` anche da `transport/web/integrita_admin.go` («Controlla ora») |
| `AllineaDocumento` | `transport/web/integrita_admin.go:allineaDocumento`, solo per un'anomalia `gia_presente` |
| `PercorsoSulNas`, `VerificaFileAperto`, `Segnala`, `ErrNonCorrisponde`, `DentroLaRadice`, `NomeFileSicuro` | `transport/web/anteprima.go` (`dalNas`, `nelloStaging`, il nome nel `Content-Disposition`) |
| `DentroLaRadice` | anche `transport/workerapi/contenuto.go:nelloStaging` (`GET /api/v1/allegati/{id}/contenuto` serve solo file sotto lo staging) |
| `CartellaThread` | `transport/web/triage.go`: anteprima del nome; alla nascita della RFQ il nome passa da `triage.go:cartellaLibera`, che aggiunge « (2)», « (3)»… se un'altra RFQ ha già quella cartella (senza maiuscole), poi `InsertThread` |
| `NomeSicuro` | `transport/web/triage.go`, `anagrafica.go`, `censisci.go` (la cartella NAS di un cliente, 80 caratteri) |
| `CartellaDocumento`, `ScegliPercorso`, `NomeSulNas`, `Tecnico` | `transport/web/allegati.go:confermaProposta`; `Tecnico` anche da `fascicolo_conferma.go:decidiProposta` |
| `CartellaDocumento`, `Tecnico`, `NomeTecnico`, `NomeNelPercorso`, `RiscegliPercorso`, `BloccaCartella` | `transport/web/fascicolo.go` (assegnazione e correzione del codice) |
| `SorgenteStaging`, `ErrContenutoMancante` | solo le prove L4 di `app/runtime` |
| `PathDocumento`, `AccodaSpostamento`, `RimuoviCartella` | solo le prove di questo package (vedi Stato) |

## Dati

**Legge:** `documento`, `thread_offerta` (`cartella_relativa`), `cartella_documento` (layout per tipo,
`crea_sempre`), `allegato` e `messaggio` (la sorgente di una copia, `ListAllegatiThread`), `job` (copie
pendenti, spostamenti pendenti, lease token vivi, ultimo job per chiave), `nas_orfano` (righe aperte,
dentro `PercorsoOccupato` e `CartellaReferenziata`).

**Scrive:**

| Chi | Che cosa |
|---|---|
| `CopiaSulNas` | `documento.stato_nas`/`scritto_il`/`errore_nas` (`SetDocumentoScritto` con `WHERE path_relativo = <percorso copiato>`; `SetDocumentoErrore`, che ha `WHERE stato_nas <> 'scritto'` e quindi non tocca mai un documento già scritto) |
| `CreaCartellaThread` | `thread_offerta.cartella_creata` (non in dry-run) |
| `Ricognitore.Giro` | `nas_anomalia` (apre/aggiorna con `Segnala`, o chiude) e `documento.verificato_il` per **ogni** documento guardato, qualunque sia l'esito |
| `VerificaFileAperto` | `documento.verificato_il` **solo se** l'hash corrisponde; altrimenti `nas_anomalia` (`conflitto` o `illeggibile`) |
| `Segnala` | `nas_anomalia` (upsert sulla riga aperta del documento) |
| `AllineaDocumento` | `documento` → `scritto`, chiusura di `nas_anomalia` |
| `RiprendiContenuto` | `job` `estrai_archivio` o `stage_allegato` |
| `AccodaSpostamento` | `job` `sposta_nas` |

**Transazioni.** Il package non apre transazioni: lavora sul `*db.Queries` che riceve. La conferma, la
correzione di un codice e `RimuoviCartella` girano nella transazione di chi chiama; l'esecutore e il
ricognitore passano un `Queries` sul pool (una scrittura per istruzione).

**Lucchetti.** `BloccaCartella` = `pg_advisory_xact_lock` su `thread_id:lower(cartella)`: vale solo
dentro una transazione, fino al COMMIT. Chi tocca più cartelle le blocca in ordine di percorso (lo fa
`transport/web/fascicolo.go:bloccaCartelle` prima di `RiscegliPercorso`). `AccodaSpostamento` blocca
anche lo spostamento pendente del documento (`BloccaSpostamentoPendente`, `FOR UPDATE`).

**Disco.** Tutti i percorsi del database sono relativi alla radice NAS, con `\`. Sul NAS: `MkdirAll`
della cartella della RFQ e delle sottocartelle `crea_sempre`, copia via `nas.Scrittore.Copia`,
rimozione dei `.parte.<token>` scaduti (token non vivo **e** più vecchio di
`coda.DurataMassimaS(copia_nas)`), `os.Remove` di una cartella vuota. Letture: `os.Stat` e sha256 del file
(ricognitore, `AllineaDocumento`), sha256 dall'handle già aperto (`VerificaFileAperto`). Nello
staging: `os.Stat` della sorgente e `staging.ToccaContenuto` dopo la copia (il contenuto resta in cache).

## Flussi principali

- **Nome di un file confermato** — `CartellaDocumento(layout, cartella_per_codice, codice)` →
  `NomeSulNas(tipo, codice, rev, ext, originale)` → `ScegliPercorso`: lucchetto della cartella, poi il
  primo nome libero fra `nome`, `nome_2`… fino a 999. Il nome originale resta in `documento.nome_file`.
- **Copia** (`CopiaSulNas`) — documento e thread → se il documento è già `scritto` (di solito una copia
  esaurita che l'avvio o il ritorno del NAS riaccodano) e il file al suo percorso ha lo sha256 giusto,
  esce subito con `gia_scritto`, senza cercare il contenuto nella cache; se il file manca o è un altro si
  prosegue → sorgente nello staging
  per sha256 → se il contenuto è sparito, `RiprendiContenuto` e il tentativo fallisce (la coda riprova)
  → `PulisciPartiScadute` nella cartella di destinazione → `nas.Scrittore.Copia` (mai sovrascrive:
  `nas.ErrConflitto`) → `SetDocumentoScritto` sul percorso letto all'inizio; zero righe = il percorso è
  cambiato, il tentativo fallisce e si ripete → `ToccaContenuto`. Gli errori della sorgente, della
  ripresa e della scrittura finiscono in `documento.errore_nas` con lo stato `errore`
  (`SetDocumentoErrore`), tranne su un documento già `scritto`, che resta tale.
- **Ripresa** (`RiprendiContenuto`) — voce di un archivio ancora in cache → `estrai_archivio`; file (o
  archivio che lo conteneva) caricato a mano nel Fascicolo (origine `manuale`) → nessun job, e il
  messaggio dice di ricaricarlo dal Fascicolo («+ Aggiungi file», poi «Riprova copie»), non «Riscarica»;
  altrimenti `stage_allegato` dell'allegato o del suo archivio (`coda.CopiaPerDownload`,
  `coda.AccodaStage`). Se l'ultimo `stage:` è fallito dopo la creazione di questa copia, non si
  riaccoda; con `j` nil (una copia fuori dalla coda) questo controllo non si fa e il download si accoda.
  Nessuna presenza del messaggio in una casella attiva → «nessuna casella attiva»; un altro errore di
  `CopiaPerDownload` → «la ripresa da Outlook non è partita», con l'errore.
- **Ricognizione** (`Ricognitore.Giro`) — NAS irraggiungibile → `Saltato`, nessuna scrittura; altrimenti
  `PerPassata` documenti (200 di default), i meno guardati prima → `esamina`: RFQ senza cartella,
  cartella al posto del file (`illeggibile`), file illeggibile, hash giusto ma non ancora `scritto`
  (`gia_presente`), hash diverso (`conflitto`), file assente con `scritto` (`mancante`), copia in coda
  (niente), `errore`, attesa oltre `Attesa` (`in_attesa`, 30 minuti di default). Prima di un
  `gia_presente` la riga del documento si rilegge (`GetDocumento`): se stato o percorso sono cambiati
  (una copia finita durante la passata) si esamina di nuovo, e un documento appena scritto non diventa
  un'anomalia → apre (`Segnala`, la stessa riga dell'anteprima e della verifica) o chiude l'anomalia.
  `Avvia` fa la prima passata dopo un minuto (o `Ogni`, se minore); `Ogni <= 0` = spento.
- **Anteprima dal NAS** — `transport/web/anteprima.go` compone con `PercorsoSulNas`, apre il file e, se
  dimensione o data fanno dubitare, chiama `VerificaFileAperto` sullo stesso handle; un file mancante o
  illeggibile lo segnala con `Segnala`.
- **Allinea** — `AllineaDocumento`: sha256 ricalcolato adesso sul file; uguale → `scritto` sul percorso
  verificato e anomalia chiusa; diverso → errore, e nessuna anomalia aperta.

## Invarianti

- **Mai sovrascrivere sul NAS.** `CopiaSulNas` passa da `nas.Scrittore.Copia`, che su un file diverso
  restituisce `nas.ErrConflitto` (l'esecutore lo chiude come definitivo); ricognitore e verifica
  segnalano e basta.
- **«Scritto» vale per il percorso su cui il file è finito** (`SetDocumentoScritto` con il percorso;
  `CopiaSulNas` e `AllineaDocumento` passano quello letto all'inizio).
- **Un tentativo di copia fallito non abbassa uno «scritto»**: `SetDocumentoErrore` salta i documenti
  `scritto`. Che il file sul NAS manchi o sia un altro lo dice il ricognitore, con la sua anomalia.
- **Un nome tagliato non finisce con un punto o uno spazio** (`NomeSicuro`, `NomeFileSicuro`): Windows
  li toglierebbe da solo e la cartella sul disco non si chiamerebbe come nel database.
- **Un nome nuovo si sceglie sotto il lucchetto della cartella**, e un percorso è occupato (senza
  distinguere maiuscole) se lo dichiara un documento del thread, il `da`/`a` di uno `sposta_nas`
  pendente o una riga aperta di `nas_orfano`.
- **Un file tecnico si chiama come il pezzo** (`<CODICE>_REV_<REV|ND>`), e il tipo che vuole la cartella
  del codice senza codice non riceve un percorso (`ErrCodiceMancante`).
- **`NomeFileSicuro` è idempotente**: ripassato sul proprio risultato lo restituisce uguale; anche
  `NomeTecnico` e `ConProgressivo` producono nomi che non cambia.
- **Chi apre un file del NAS per servirlo lo contiene nella radice** (`PercorsoSulNas`: niente `..`, `.`,
  pezzi vuoti, `:`, byte zero; confronto senza maiuscole; il prefisso long-path arriva dopo il controllo).
- **Il ricognitore non guarda un NAS irraggiungibile**; una copia già in coda, o finita durante la
  passata, non è un'anomalia; un file che non si legge è `illeggibile`, mai `conflitto` (anche in
  `VerificaFileAperto`).
- **Un file caricato a mano non si riprende da Outlook**: in Outlook non c'è, e né `cercaSorgente` né
  `RiprendiContenuto` rimandano a «Riscarica» o accodano un download.
- **Un documento ha al più uno spostamento pendente** (`ErrSpostamentoInCorso`, più l'indice dei job).
- **Una cartella non si cancella mai da sola**: `RimuoviCartella` usa `os.Remove`, mai `RemoveAll`, e solo
  se nessuno la nomina (le istantanee delle baseline non contano).

## Dipendenze

Importa: `core/inbox/classificazione` (solo `OggettoPulito`, in `CartellaThread`), `platform/db`,
`platform/coda`, `platform/storage/nas`, `platform/storage/staging`, `platform/contratti/worker`
(`PayloadEstraiArchivio`). Della coda usa l'accodamento (ripresa, spostamento) e la lettura dei job di
copia pendenti; il giro dei job, il claim e i tentativi restano di `app/runtime`. È importato da
`app/runtime`, `transport/web` e `transport/workerapi` (solo `DentroLaRadice`). Nessuna violazione della
tabella di `internal/README.md`. Il formato dei nomi tecnici (`<CODICE>_REV_<REV>`, `_2`, `ND`) lo rilegge
anche `core/inbox/classificazione` (`codici.go:CodiceRev`), senza importare questo package.

## Test

| Livello | File | Che cosa |
|---|---|---|
| L1 | `path_test.go` | `CartellaThread` (anche con il taglio a 60 e a 80 che cade dopo un punto), `NomeSicuro` che dopo il taglio non finisce con un punto o uno spazio e ripassato resta uguale, `PathDocumento` e `ErrCodiceMancante`, cartella + nome che ricompongono il percorso, `NomeFileSicuro` idempotente (casi scritti, ogni rune come estensione, 50 000 nomi a caso, tutti i nomi di file delle prove del modulo) |
| L1 | `nas_percorso_test.go` | `PercorsoSulNas`: composizione, nove modi di uscire dalla radice, `..` come pezzo e non come sottostringa, la radice stessa, long-path dopo il controllo, maiuscole |
| L1 | `integrita_test.go` | `controllo.esamina` su file veri in una cartella temporanea e con l'orologio finto; `durata` |
| L4 | `comune_db_test.go` | impalcatura: `preparaDB`, `conCapacita` |
| L4 | `integrita_db_test.go` | `Giro`: file assente con `scritto`, file già giusto, NAS irraggiungibile, copia in coda; `AllineaDocumento` su un conflitto |
| L4 | `verifica_db_test.go` | `AllineaDocumento` non apre anomalie (comportamento fissato); `VerificaFileAperto`: conflitto, corrispondenza che chiude, lettura interrotta |
| L4 | `nomi_nas_db_test.go` | nomi con la revisione e `_2`, indice unico, revisione che non tocca la vecchia, due padri una copia, orfani che riservano, lucchetto con due transazioni (con e senza), secondo spostamento rifiutato, `RimuoviCartella`, `.parte` vivi |
| L4 | `scritto_db_test.go` | `SetDocumentoScritto` con il percorso vecchio non scrive |
| L4 | `revisione_db_test.go` | una copia vecchia per un documento già scritto con il file giusto finisce subito; con il file sparito fallisce ma il documento resta `scritto`; una copia che finisce durante la passata (`mentreGuarda`) non apre `gia_presente`; la ripresa con `j` nil non si ferma; un file caricato a mano dice «ricaricalo dal Fascicolo» e non accoda download |

I test L4 hanno `//go:build integrazione` e vogliono `COCKPIT_TEST_DSN`.

Altrove: `app/runtime` (`copianas_db_test.go`, `contenutomancante_db_test.go`, `ripresa_db_test.go`,
`nas_db_test.go`: la copia e la ripresa eseguite dall'esecutore, il rinvio con il NAS assente), `transport/web` (`integrita_db_test.go`,
`anteprima_db_test.go`, `fascicolo_db_test.go`, `triage_db_test.go` per due RFQ con lo stesso nome di
cartella, L7 `anteprima_browser_test.go`). Nessuna prova chiama direttamente `CreaCartellaThread`.

## Stato dell'implementazione

- **Completi:** nomi e cartelle (B3, B8.2, B8.3, B8.A4-0, B8.A4a), copia e ripresa (B6b–B6d, Pre-7),
  ricognitore e «Allinea» (5B, B6a), contenimento e verifica per l'anteprima (B8.1).
- **In attesa di B8.8 (lo spostamento):** `AccodaSpostamento` fa solo il passo 0 e nessun codice di
  prodotto lo chiama, e non va chiamato prima: `sposta_nas` è un job del worker `server`, e
  `app/runtime/esecutore.go:esegui` non ha il suo ramo, quindi lo chiude subito come fallimento
  definitivo («tipo job non gestito dal server»). Un documento `scritto` che dovrebbe cambiare cartella
  viene rifiutato da `transport/web/fascicolo.go`. Nessun codice di prodotto scrive `nas_orfano` né
  `nas_creazione` (`InsertNasOrfano` la usano solo le prove, `InsertNasCreazione` nessuno; il `creato`
  di `nas.Scrittore.Copia` è ignorato).
- **Senza chiamante di prodotto:** `RimuoviCartella` (nessuna rotta), `PathDocumento` (la conferma usa
  `CartellaDocumento` + `ScegliPercorso`; per i tipi tecnici `PathDocumento` darebbe ancora il nome
  originale, non quello di A4).
- **Limiti noti:** `CopiaSulNas`, `AllineaDocumento` e il ricognitore compongono il percorso con
  `nas.UNC` e non passano da `PercorsoSulNas`. `NomeSicuro` e `NomeFileSicuro` non riconoscono i nomi
  riservati di Windows (`CON`, `NUL`, `COM1`…). `Ricognitore.Avvia` è un giro periodico (una
  goroutine con il timer) che sta in `core`; lo fa partire `app/runtime` (`Servizi.Avvia`).

## Dove intervenire

| Voglio… | Apri |
|---|---|
| cambiare il nome della cartella di una RFQ | `path.go:CartellaThread`, `NomeSicuro` (il progressivo « (2)»: `transport/web/triage.go:cartellaLibera`) |
| cambiare la sottocartella di un tipo o la cartella del codice | `path.go:CartellaDocumento` (e la tabella `cartella_documento`) |
| cambiare il nome di un file tecnico | `nomi_nas.go:NomeTecnico`, `NomeSulNas` (e `core/inbox/classificazione/codici.go:CodiceRev`, che rilegge quei nomi) |
| capire perché un file ha preso `_2` | `nomi_nas.go:primoLiberoPer` e la query `PercorsoOccupato` |
| capire perché una copia non parte, fallisce o si ripete | `copia_nas.go:CopiaSulNas`, `cercaSorgente`; `ripresa.go:RiprendiContenuto`, `contenutoCaricatoAMano` |
| capire perché un documento risulta mancante, in conflitto, in attesa | `integrita.go:controllo.esamina`, `Ricognitore.Giro` (la rilettura prima di `gia_presente`) |
| aggiungere un controllo al ricognitore | `integrita.go:esamina`, e l'enum `problema_nas` in migrazione |
| capire perché l'anteprima rifiuta un percorso | `nas_percorso.go:relativoSicuro`, `dentroLaRadice` |
| cambiare che cosa succede quando l'anteprima trova un file diverso | `verifica.go:VerificaFileAperto` |
| eseguire lo spostamento (B8.8) | `nomi_nas.go:AccodaSpostamento`, `app/runtime/esecutore.go:esegui` |
| togliere una cartella del fascicolo | `cartelle.go:RimuoviCartella` |

## Leggi anche

`internal/core/README.md` (il package fra gli altri di `core`), `internal/README.md` (i flussi
«Allegato → NAS» e «Integrità NAS»), `internal/app/README.md` (l'esecutore e il ricognitore),
`internal/transport/README.md` (le rotte dell'anteprima, del fascicolo e di Admin → Integrità),
`internal/platform/storage/nas/nas.go` (lo scrittore).
