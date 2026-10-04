---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `core/` — le regole del mestiere

## Scopo

- Che cosa significa una mail, a chi appartiene, che cosa propone; come si legge; che cosa diventa una RFQ sul NAS e nella sua distinta.
- Senza HTTP, senza TLS, senza lettura della configurazione.
- Il database lo toccano solo i package che lo dichiarano qui sotto.
- Questo README tiene le regole che valgono per tutta l'area — dipendenze, invarianti, effetti —
  - e rimanda al README di ogni package per il dettaglio (file, entry point, dati, test, stato).

## Non appartiene qui

- Handler e template
- lettura del `cockpit.toml`
- client verso servizi esterni
- scritture sul NAS fatte a mano (passano da `platform/storage/nas`)
- I loop di processo stanno in `app/runtime` e in `platform`, con un'eccezione dichiarata:
  - il giro del ricognitore dell'integrità NAS (`rfq/documenti/integrita.go:Ricognitore.Avvia`).

## Package posseduti

- **`inbox/classificazione`**
  - Che cosa fa: l'**interpretazione** pura:
    - controparte (il resolver su una `Rubrica`)
    - taglio della catena
    - codici e triage (risposta > candidati > nuova RFQ)
    - atto e legame (7C.0)
    - il **motore** delle regole del cliente con i suffissi decorativi (`Canonico`, `CanonicoNome`)
    - la revisione dei nostri nomi NAS (`CodiceRev`)
    - proposta del tipo di documento dal nome
    - oggetto ripulito
    - punteggi di aggancio
  - DB: no
  - README: [README](inbox/classificazione/README.md)
- **`inbox/ingest`**
  - Che cosa fa:
    - un lotto di messaggi → fatti e prime interpretazioni in una transazione, con un savepoint per elemento
    - scarti e replay
    - cursore
    - staging automatico (D30)
    - marcatori della nostra posta (7B)
    - controparte e ritriage mirato (7A)
    - Scrive solo per la casella del job che consegna il lotto
  - DB: sì
  - README: [README](inbox/ingest/README.md)
- **`inbox/aggancio`**
  - Che cosa fa: i **candidati** con evidenza:
    - di aggancio a una RFQ (R0–R5)
    - di codice
    - verso una richiesta a un fornitore (R0/R1/R3f, `RF_oggetto`)
  - DB: sì
  - README: [README](inbox/aggancio/README.md)
- **`inbox/lettura`**
  - Che cosa fa: il corpo di una mail pronto da leggere:
    - rumore chiuso e non tolto
    - tabelle di Excel come tabelle
    - storia citata a parte
    - testo originale intatto
    - oggetto senza `[EXTERNAL]`
    - Puro
  - DB: no
  - README: [README](inbox/lettura/README.md)
- **`rfq/documenti`**
  - Che cosa fa:
    - i nomi sul NAS (cartella della RFQ, `<CODICE>_REV_<REV>`, il progressivo `_2`)
    - la copia di un documento
    - la ripresa di un contenuto sparito
    - la cartella della RFQ
    - il ricognitore dell'integrità
    - la verifica di chi serve i byte di un file (anteprima)
  - DB: sì
  - README: [README](rfq/documenti/README.md)
- **`rfq/fascicolo`**
  - Che cosa fa:
    - la **BOM nel tempo** (versioni, gate, revisioni, archiviazione, STEP strutturale, deroghe, sostituzioni)
    - le proposte di struttura dagli STEP e le decisioni che le portano nella working
    - i codici della RFQ
    - l'albero della schermata
    - le modifiche a mano
    - la preparazione automatica (B8.7b)
    - il piano di «Conferma Fascicolo»
    - l'editor della struttura (v3)
    - le note e i caricamenti interni
  - DB: sì
  - README: [README](rfq/fascicolo/README.md)
- **`registro/regole`**
  - Che cosa fa:
    - lo **schema** di ciò che un cliente dichiara di sé (`cliente.regole`, suffissi decorativi compresi) e le convenzioni di codice
    - con la porta in scrittura che rifiuta e quella in lettura che segna ✓/✗
    - Non applica niente
  - DB: no
  - README: [README](registro/README.md)
- **`estrazione/evidenze`**
  - Che cosa fa: la **foglia comune del motore A** (giro 5):
    - il vocabolario: dove si è letta un'osservazione (contesti, campi, selettori, intervalli)
    - il tipo delle diagnostiche; i codici li dichiara ogni package che li produce
    - il resto del contratto d'ingresso del motore è descritto nel README del package
    - Puro, senza interpretazione dei clienti
  - DB: no
  - README: [README](estrazione/evidenze/README.md)
- **`registro/regole/grammatica`**
  - Che cosa fa: le **grammatiche v1 del motore A** (giro 5), separate dallo schema legacy di `cliente.regole`:
    - i tipi delle grammatiche, la lettura stretta e la validazione, con le capacità riservate in A1
    - i limiti dichiarati dall'indice (nel codice solo i tetti), la forma canonica con l'hash, l'indice per UUID
    - Puro; non compila regex per il riconoscimento
  - DB: no
  - README: [README](registro/regole/grammatica/README.md)
- **`registro/anagrafica`**
  - Che cosa fa:
    - il seme dei clienti (una volta, senza sovrascrivere)
    - `NomeCognome`
  - DB: sì
  - README: [README](registro/README.md)
- **`registro/fornitori`**
  - Che cosa fa: l'import del seme dei fornitori:
    - `Leggi` convalida
    - `Calcola` fa l'anteprima senza scrivere
    - `Applica` scrive solo ciò che è risolto
  - DB: sì
  - README: [README](registro/README.md)

## Dipendenze consentite

- Gli archi che esistono davvero (`go list`), tutti dentro la regola «`core/*` → altri `core/*` e `platform`»:
  - **`registro/regole`** importa: **niente** del progetto
    - è lo schema, e uno schema non dipende da chi lo usa
  - **`estrazione/evidenze`** importa: **niente** del progetto
    - è la foglia del motore A: la importano i suoi package, e lei non importa nessuno di loro
  - **`registro/regole/grammatica`** importa:
    - `estrazione/evidenze` (solo il vocabolario dei selettori e il tipo `Diagnostica`: G8)
    - `platform/jsoncanonico`
    - mai `registro/regole`, il legacy: i due formati non si mescolano
  - **`inbox/classificazione`** importa:
    - `registro/regole` (il motore lavora sullo schema)
  - **`inbox/lettura`** importa:
    - `inbox/classificazione` (solo `TagliaCatena`)
  - **`inbox/aggancio`** importa:
    - `inbox/classificazione`
    - `platform/db`
  - **`inbox/ingest`** importa:
    - `inbox/aggancio`
    - `inbox/classificazione`
    - `registro/regole`
    - `platform/db`
    - `platform/coda`
    - `platform/contratti/worker`
    - `platform/storage/staging`
  - **`rfq/documenti`** importa:
    - `inbox/classificazione` (`OggettoPulito`: il nome della cartella nasce dall'oggetto ripulito)
    - `platform/db`
    - `platform/coda`
    - `platform/contratti/worker`
    - `platform/storage/nas`
    - `platform/storage/staging`
  - **`rfq/fascicolo`** importa:
    - `inbox/classificazione`
    - `registro/regole` (il motore del cliente che classifica i nodi degli STEP)
    - `platform/db`
    - `platform/coda`
    - `platform/contratti/worker`
    - `platform/storage/staging`
  - **`registro/anagrafica`** importa:
    - `registro/regole`
    - `platform/db`
  - **`registro/fornitori`** importa:
    - `platform/db`
- Mai `transport`, mai `ai`, mai `app`.
- Nessun package di `platform` importa `core`
  - l'unica eccezione è un test: `platform/migrazioni/larghezze_db_test.go` usa `inbox/classificazione` per confrontare le larghezze delle colonne con i limiti del codice.

## Entry point

- I principali; chi li chiama è nel README del package.
- `classificazione.Triage`, `RisolviControparte`, `TagliaCatena`, `PropostaDaNome`, `Motore.Canonico`, `CodiceRev`;
- `ingest.Servizio.Ingerisci`, `Servizio.Ritriage` / `RitriageMolti`, `RicalcolaControparti`, `Servizio.Riprova`;
- `aggancio.CalcolaESalva`, `CalcolaRichieste`, `SalvaCandidatiCodice`;
- `lettura.Presenta`, `OggettoVisibile`;
- `documenti.CartellaThread`, `NomeSicuro`, `ScegliPercorso`, `CopiaSulNas`, `RiprendiContenuto`, `CreaCartellaThread`, `Ricognitore.Giro` / `Avvia`, `AllineaDocumento`, `PercorsoSulNas`, `DentroLaRadice`, `VerificaFileAperto`, `Segnala`;
- `fascicolo.CongelaBom`, `ApriRevisione`, `AbbandonaBozza`, `LeggiGate`, `ApplicaStruttura`, `RianalizzaRfq`, i gesti di decisione (`AccettaNodo`, `AccettaRelazione`, `AccettaFile`, `ScartaNodo`, `CodiceDelNodo`, …), `CandidatiDellaRfq`, `AggiungiDaCodice`, `NuovoAlbero`, `ModificaComponente`, `Collega` / `Scollega` / `Sposta`, `AssicuraProdottiDellaRichiesta`, `PreparaFile`, `PianoDelFascicolo`, `ApplicaStrutturaVoluta`, `NotaInterna`, `RegistraCaricamento`;
- `regole.ValidaRegole` / `LeggiRegole` / `ValidaConvenzione` / `LeggiConvenzioni`, `anagrafica.Semina`,
- `fornitori.Leggi` / `Calcola` / `Applica`.

## Flussi principali

- **L'ingest di un lotto**:
  - tipo e casella del job verificati con la riga del job bloccata
  - modo del sync letto dal **job** e non dal lotto
  - identità del messaggio = Message-ID
  - controparte risolta e scritta
  - catena tagliata solo nei testi dati all'interpretazione
  - triage, proposte e candidati
  - cursore nella stessa transazione.
  - Il ritriage mirato tocca solo i messaggi **non decisi**.
- **La lettura di una mail**:
  - `lettura.Presenta` sul corpo memorizzato, a ogni apertura del pannello o della pagina della RFQ
  - non scrive niente.
- **Dagli STEP alla BOM**:
  - i fatti del worker diventano proposte di nodi, archi, quantità e rimozioni (`ApplicaStruttura`, per ogni RFQ che ha quel contenuto)
  - una persona le accetta o le scarta, oppure le porta nella working dall'editor (`ApplicaStrutturaVoluta`).
- **La preparazione** (B8.7b):
  - i codici confermati della richiesta diventano prodotti finiti
  - i file utili si scaricano, si estraggono e si analizzano (`AssicuraProdottiDellaRichiesta`, `PreparaFile`).
- **La conferma**:
  - il piano (`PianoDelFascicolo`) divide i file in pronti, da decidere e in attesa
  - la conferma di un file crea il `documento` e accoda la copia (il gesto sta ancora in `transport/web`).
- **La copia sul NAS e l'integrità**:
  - `CopiaSulNas` (eseguita dall'esecutore del server)
  - la ripresa di un contenuto sparito
  - il ricognitore che confronta i documenti con i file veri.

## Invarianti

- **I tre strati**:
  - fatto (che cosa è arrivato, e chi c'è dall'altra parte secondo l'anagrafica di quel momento)
  - interpretazione (che cosa probabilmente significa, con punteggio ed evidenza)
  - decisione (che cosa l'operatore ha deciso).
  - Un'interpretazione non scrive mai una decisione.
- `messaggio.thread_id` non si scrive dall'ingest, con **un'eccezione sola**:
  - il marcatore `CockpitRichiestaFornitore` sulla nostra posta in uscita, verso una richiesta che esiste, su un messaggio che non sta già in un'altra RFQ (`ingest/marcatori.go`).
- Un fornitore, un ambiguo o uno sconosciuto **in entrata non produce mai `nuova_rfq`** (7B.3, D34):
  - è per costruzione, non per un controllo aggiunto dopo.
- Precedenza del triage: risposta > candidati > nuova RFQ.
  - Le regole del cliente marcate ✗ non entrano nel motore.
- Il corpo originale non si modifica:
  - il taglio decide solo che cosa legge l'interpretazione
  - `lettura` decide solo come si mostra
  - e il testo originale resta raggiungibile.
- «Da validare» = sconosciuto + ambiguo.
  - Un fornitore senza domini e senza contatti non sposta nessuna mail:
    - la posta si riconosce dall'indirizzo, non dalla ragione sociale.
- **Niente sul NAS senza una decisione**:
  - un `documento` nasce solo da una conferma, e solo lui accoda la copia.
  - Il NAS non si sovrascrive mai, e un documento `scritto` non torna indietro.
- La BOM working cambia solo:
  - per un gesto di una persona (decisioni, modifiche a mano, editor)
  - o per i prodotti della richiesta già confermati da una persona (preparazione);
  - una versione congelata non cambia più (i trigger `BOM01`–`BOM05` della 0020).
- I gesti che toccano una RFQ bloccano **prima la RFQ**, poi il resto:
  - un ordine solo, niente stalli fra due gesti.

## Effetti collaterali

- `ingest` scrive i fatti e le prime interpretazioni, e accoda gli `stage_allegato` automatici;
- `aggancio` scrive i candidati;
- `registro/*` scrive le anagrafiche.
- `rfq/documenti`
  - scrive:
    - `nas_anomalia`
    - lo stato NAS dei documenti
    - e — in `CopiaSulNas` e `CreaCartellaThread` — i file sul NAS attraverso `platform/storage/nas`;
  - la ripresa accoda download e riestrazioni.
- `rfq/fascicolo` scrive:
  - le versioni della BOM e le loro istantanee, l'archiviazione, lo STEP strutturale, le deroghe e la catena delle revisioni;
  - dai fatti degli STEP scrive `componente_proposta`, `relazione_proposta` e `rimozione_proposta` (interpretazione),
  - e `componente` / `componente_relazione` nei gesti di decisione, nell'editor e nella preparazione;
  - accoda `stage_allegato`, `estrai_archivio` e `analizza_allegato` (preparazione e rianalisi);
  - crea il messaggio «nota interna» della RFQ e i suoi allegati per i caricamenti interni.
- `inbox/classificazione`, `inbox/lettura` e `registro/regole` non hanno effetti.

## Test

- **L1** (senza database, `go test ./...`):
  - sugli oracoli di `inbox/classificazione`
  - su tutto `inbox/lettura` (con fuzz e benchmark)
  - su `inbox/aggancio` (le chiavi citate, i tagli per carattere, la revisione della storia)
  - su `registro/regole`
  - e sulle parti pure di `rfq/documenti` e `rfq/fascicolo` (nomi, gate, cicli, differenza, classificazione dei nodi, unione dei codici, albero, piano, editor, note).
- **L4** (`-tags integrazione`, PostgreSQL di prova) per tutto ciò che scrive:
  - `ingest`
  - `aggancio`
  - `registro/fornitori` e `registro/anagrafica`
  - `rfq/documenti` (contro file veri)
  - `rfq/fascicolo`
  - compresi gli invarianti I4/I5 del contratto di classificazione.
- Ogni test che usa il database ha il tag `integrazione`.

## Dove intervenire

- Voglio…
  - **una regola di triage nuova**
    - Apri: `inbox/classificazione/codici.go:Triage`, `Motore.Codici`
  - **un campo nuovo in `cliente.regole`**
    - Apri: `registro/regole/regole.go`, poi il motore in `inbox/classificazione/regole.go`
  - **un suffisso decorativo che non viene riconosciuto**
    - Apri: `registro/regole/regole.go` (la validazione), `inbox/classificazione/regole.go:Canonico`
  - **un atto nuovo**
    - Apri: `inbox/classificazione/atto.go` e la tabella `atto_business`
  - **una convenzione di codice**
    - Apri: `registro/regole/convenzioni.go`
  - **capire perché un pezzo di una mail è chiuso, o una tabella non si vede**
    - Apri: `inbox/lettura` (il README dice quale regola)
  - **il nome di una cartella o di un file sul NAS**
    - Apri: `rfq/documenti/path.go`, `nomi_nas.go`
  - **capire perché una BOM non si congela**
    - Apri: `rfq/fascicolo/gate.go:Valuta`
  - **capire perché un nodo di uno STEP ha (o non ha) un codice**
    - Apri: `rfq/fascicolo/classifica.go:ClassificaNodi`
  - **capire perché l'editor rifiuta una struttura**
    - Apri: `rfq/fascicolo/voluta.go:pianifica`
  - **capire che cosa chiede «Conferma Fascicolo»**
    - Apri: `rfq/fascicolo/piano.go:PianoDelFascicolo`
  - **capire perché un file non si prepara da solo**
    - Apri: `rfq/fascicolo/preparazione.go:PreparaFile`
  - **capire perché un documento risulta mancante o in conflitto**
    - Apri: `rfq/documenti/integrita.go`
  - **capire perché una copia sul NAS non parte o si ripete**
    - Apri: `rfq/documenti/copia_nas.go:CopiaSulNas`, `ripresa.go:RiprendiContenuto`
  - **capire perché un fornitore non apre una RFQ**
    - Apri: `inbox/classificazione/controparte.go:RisolviControparte`, `inbox/classificazione/codici.go`
  - **capire perché un'offerta propone quella richiesta**
    - Apri: `inbox/aggancio/richieste.go:CalcolaRichieste`
  - **capire che cosa scrive (e non scrive) l'import dei fornitori**
    - Apri: `registro/fornitori/seme.go`

## Leggi anche

- `internal/README.md` (i flussi per intero, lo stato di ogni package)
- `platform/README.md` (query e migrazioni)
- `transport/README.md` (chi chiama questi entry point e le eccezioni ancora aperte).
