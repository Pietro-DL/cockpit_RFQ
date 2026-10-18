---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/rfq/fascicolo` — la BOM di una RFQ nel tempo

## Scopo

- Che cosa è la BOM di una RFQ e come cambia:
  - i componenti e gli archi della **working**,
  - le proposte che gli STEP ne fanno,
  - le decisioni che le portano nella working,
  - le versioni congelate con le loro istantanee,
  - il gate che decide se si può congelare,
  - e quello che il sistema prepara da solo prima che qualcuno decida (i prodotti della richiesta, i file da scaricare e analizzare, il piano di conferma).
- I tre strati valgono anche qui.
  - **Fatto**: `analisi_fatti.fatti.struttura`, scritto dal worker.
  - **Interpretazione**: `componente_proposta`, `relazione_proposta`, `rimozione_proposta`, scritte da `ApplicaStruttura` e `AggiornaRimozioni`.
  - **Decisione**: `componente` e `componente_relazione`, scritte solo dai gesti di una persona
    - (e da `AssicuraProdottiDellaRichiesta`, che traduce un codice della richiesta già confermato da una persona).
- Le regole che contano le tiene il database (0018, 0020):
  - identità `(thread, upper(codice))`,
  - working bloccata dopo il congelamento (trigger, errori `BOM01`–`BOM05`),
  - versioni e istantanee immutabili,
  - catena delle revisioni dei documenti.
- Qui si decide **che cosa** scrivere e si dice all'operatore **perché** qualcosa non si può fare, prima che ci sbatta contro il vincolo.

## Non appartiene qui

- Rotte, form, template, la traduzione degli errori `BOM0x` in frasi: `transport/web` (`fascicolo*.go`, `proposte.go`, `codici.go`, `caricamento.go`).
- La conferma di un documento (`documento_proposta` → `documento`, «aggiungi / sostituisce», percorso sul NAS) e «Conferma Fascicolo» che applica il piano:
  - `transport/web/allegati.go:confermaProposta`,
  - `transport/web/fascicolo_conferma.go:applicaPiano`.
- La correzione del codice di un componente (tocca documenti e NAS): `transport/web/fascicolo.go:correggiCodiceComponente`,
  - con la stessa guardia degli altri codici (`classificazione.CodiceAmmissibile`).
- I nomi sul NAS, la copia, il ricognitore: `core/rfq/documenti`.
- Le note sui disegni (`annotazione_pdf`, 0021): `transport/web/fascicolo_gesti_v3.go`.
  - Qui compaiono solo nel messaggio di `RimuoviComponente` quando la FK le nomina.
- La classificazione dei testi e i suffissi decorativi (`Canonico`, `CanonicoNome`, `HaSuffissi`): `core/inbox/classificazione`.
  - Questo package li usa, non li definisce.
- L'esecuzione dei job e l'applicazione dei risultati del worker: `app/runtime`, `transport/workerapi`.
- Il Motore A (`core/inbox/classificazione/motorea`, `core/valutazione`): classifica con le grammatiche dell'indice
  letto all'avvio (`[motore_a]`), non con `cliente.regole`, e in sola lettura.
  - Questo package non lo importa.

## File

- **`fonte_step_v1.go`** (V1, ondata B, corsia persistenza; EM §2.3, §9.10; PM23 R-24)
  - `ScriviFonteProdotto`: la riga della storia della fonte STEP del prodotto (`fonte_step_evento`, ruolo prodotto) che
    «Autorizza» (`autorizzazione`, in `DichiaraStrutturale` per un finito, anche dalla sostituzione con «sì»), «Revoca»
    (`revoca`, con il motivo, in `RevocaStrutturale`) e «Conferma la fonte» (`conferma_documento`, in
    `transport/web/allegati.go`) scrivono nella transazione dell'atto; senza un allegato del documento la riga non si
    scrive e l'atto va avanti (limite dichiarato)
- **`fascicolo.go`**
  - Commento `// Package`.
  - `Rifiuto` (il «no» per l'operatore: chi chiama annulla la transazione; `transport/web` usa lo stesso tipo come suo `rifiuto`)
  - `WorkingBloccata`
  - `SeBloccata` (D26 detto prima del trigger)
  - `componenteDellaRfq` (`FOR UPDATE` e appartenenza alla RFQ)
- **`versioni.go`**
  - `CongelaBom` (gate, V1 al primo congelamento, le quattro istantanee, per una versione `preventivo` il passaggio `FATTIBILITA → SCHEDA_COSTO` con la baseline)
  - `ApriRevisione` e `contestoRevisione` (fase → contesto; in `ACCETTATA` e `DISTINTA_ERP` sceglie chi apre)
  - `AbbandonaBozza` (solo a differenza vuota; una `preventivo` torna alla fase d'apertura)
- **`gate.go`**
  - `Valuta` (pura: requisiti bloccanti, proposte strutturali aperte, errori e anomalie NAS, esito dello STEP di ogni finito, cicli)
  - `LeggiGate`, `Ciclo`, `EtichettaStep` e le costanti `Step*` (gli esiti di `v_step_prodotto`)
- **`differenze.go`**
  - `Differenze` (pura, sulle coppie prima/dopo di `DiffBomWorking`), `DifferenzeWorking`
- **`struttura.go`**
  - `ArchiviaComponente` (per un prodotto chiude anche le rimozioni aperte del suo STEP strutturale)
  - `RipristinaComponente`
  - `RimuoviComponente` (cancellazione fisica; se la FK la rifiuta, `cheCosaLoTiene` dice quale tabella; dalla 0022
    anche conferma strutturale, bozza della distinta e suo registro, storia della fonte STEP, fascicolo attestato,
    copertura documentale; il riepilogo dell'albero lo sa prima, da `ListComponentiConStoria`, e dice «si archivia»)
  - `Step`
  - `ScegliStepStrutturale` (rifiutato su un archiviato)
  - `ConcediDerogaStruttura` / `RevocaDerogaStruttura` (solo una deroga della stessa RFQ; dalla 0022 la concessione
    e la revoca restano in `deroga_struttura_storia`, scritta dal trigger)
  - `Sostituisci` (il nuovo riferimento strutturale rifiutato su un archiviato) / `AnnullaSostituzione`
  - `chiudiProposteDi` (nota tagliata a 200 caratteri)
- **`classifica.go`**
  - `ClassificaNodi` (pura: il codice di ogni PRODUCT dal `Motore` del cliente; famiglia prima di generico, id → nome → descrizione; codici e revisioni fuori misura restano nell'evidenza)
    - la revisione proposta è la formazione dello STEP (`rev_grezza`, cioè `PRODUCT_DEFINITION_FORMATION.id`) quando
      c'è, e vince su quella che la famiglia separa dal codice;
    - il segnaposto «ANY» non è una revisione: resta nell'evidenza (`rev_grezza_segnaposto`) e la revisione resta
      quella della famiglia, se c'è (`RevGrezzaSegnaposto`, A1d P4, B4.10)
  - `RevGrezzaSegnaposto`
  - `MotoreDellaRfq`
- **`proposte.go`**
  - `Pianifica` (pura: il file classificato contro la working → righe aperte o `duplicato`)
  - `Working` / `NuovaWorking` (solo gli archi fra componenti attivi), `Contesto`, `Piano`, `CreerebbeCiclo`
  - `RimozioniFigliDiretti` (pura)
  - `RadiceDelloStep` (la radice del file, se è una sola e il motore del cliente ne legge il codice: un'evidenza del
    codice del documento STEP)
- **`applica.go`**
  - `ApplicaStruttura`: il fan-out idempotente dei fatti di uno STEP in una RFQ
  - `valutaLaRadice` (la radice dello STEP entra come evidenza nella valutazione del codice del documento,
    `classificazione.Valuta`; una radice diversa dal nome rende la dimensione discorde e la colonna tiene il nome, D49;
    prima era `propostaDelDocumento`, la D16)
- **`rimozioni.go`**
  - `AggiornaRimozioni` (solo dallo STEP strutturale letto per intero)
  - `AggiornaTutteLeRimozioni`
- **`decisioni.go`**
  - `AccettaNodo`, `AccettaRelazione`, `AccettaSottoalbero`, `AccettaFile`, `ScartaNodo`, `ScartaRelazione`, `CodiceDelNodo`, `AccettaRimozione`, `ScartaRimozione`
  - A1d P4 (B2.11; D-K6-01 = A [U], direttiva 12 §6): `DecisioneTipo{Assiemi}`, `AccettaRelazioneDecisa`, `AccettaSottoalberoDeciso`, `AccettaFileDeciso`; un particolare non diventa assieme da solo: senza dichiarazione il gesto risponde con `RichiestaConversione` (componente, codice, figlio, `Domanda`), non scrive niente, ed e' anche un `Rifiuto` per chi non sa chiedere la conferma
  - `prepara` (lucchetto della RFQ + D26)
  - `dopoLaDecisione` (ricalcolo delle rimozioni)
  - `componenteConSuffisso`
  - `tagliaNota` / `maxNota` (le note delle proposte stanno in `varchar(200)`)
- **`rianalisi.go`**
  - `RianalizzaRfq`, il gesto «Rianalizza» (rilegge gli STEP che hanno i fatti correnti, accoda gli altri fino al
    limite che il chiamante passa; poi gli STEP da rileggere e i PDF/DXF letti da un analizzatore precedente, con
    quello che resta del limite)
  - `RileggiStepDellaRfq` (anche alla nascita della RFQ e all'aggancio), `AccodaAnalisiMancanti`,
    `AccodaAnalisiMancantiDaSola` (la preparazione: accoda e basta, non riprova un'analisi fallita), `Rianalisi`,
    `inStaging`
- **`rianalisi_step.go`** (LS-01, 17/10/2026)
  - `AccodaStepDaRileggere`: riaccoda gli STEP i cui fatti correnti hanno la struttura in una forma che il server non
    accetta (`worker.StrutturaDaRileggere`: in pratica la v4 del worker di GP-3), e solo quelli; per contenuto, prima
    i compatibili con un prodotto, contati a parte (`Rianalisi.Step*`). Solo dal gesto «Rianalizza», mai dalla
    preparazione: nessuna rianalisi in massa parte da sola. Uno STEP da rileggere non più in staging si riprende con
    «Riscarica» e poi di nuovo «Rianalizza»: «Riscarica» da solo riapplica i fatti presenti (dopoStaging)
- **`candidati.go`**
  - `Unisci` (pura: evidenze per `upper(codice)`, forte o debole, revisioni viste, situazione e gesto)
  - `CandidatiDellaRfq`, `AggiungiDaCodice`, `NomeTipo`, `TipiDaCodice`
- **`albero.go`**
  - `NuovoAlbero` (puro: finiti in cima, figli per codice, il condiviso aperto una volta sola, archiviati a parte, un arco che chiude un giro non si segue, `Totali`)
  - `totali` e `inUnCiclo` (Tarjan: chi sta in un ciclo tiene la sua quantità, chi sta sotto un ciclo la moltiplica, qualunque sia l'ordine dei dati)
- **`modifiche.go`**
  - `ModificaComponente`
  - `Collega` / `Scollega` / `Sposta` (`tieniArcoMessoAMano`: l'arco appena messo non si propone da togliere)
  - `ConcediDeroga` / `RevocaDeroga` (fabbisogno)
  - `RevisioneProponibile`
  - `NotaInterna` / `ChiaveNotaInterna`
  - `RegistraCaricamento` / `Caricato`
    - V1 (EM §10.6, SIC-06): `AdottaContenuto`, dopo `BloccaMessaggio` e prima della riga: sotto `BloccaContenuto` il
      contenuto gia' presente si verifica (sha ricalcolato) e si usa, altrimenti si promuove il parziale della richiesta
      (`Caricato.Parte`); senza parziale e senza contenuto la registrazione rifiuta («ricarica il file»)
  - `MaxQtaArco`
- **`preparazione.go`**
  - `AssicuraProdottiDellaRichiesta`, `PreparaFile` / `RiletturaFatti`, `LavoroInCorso` (B8.7b)
- **`piano.go`**
  - `PianoDelFascicolo` (puro: ogni file aperto `pronto` / `decidere` / `attesa` con le sue domande, le strutture degli STEP, lo STEP strutturale suggerito, `Firma`)
  - `LeggiPianoFascicolo`, `LeggiIngressoPiano` (B8.7b)
- **`voluta.go`**: l'editor della struttura (Fascicolo v3)
  - `StrutturaVoluta`
  - `pianifica` (puro: riferimenti `c:` / `p:` / `k:`, perimetro, dati cambiati nel frattempo, cicli sul grafo finale; limiti `MaxArchiVoluti` archi voluti e quattro volte tanti archi visti, ciascuno con il suo messaggio)
  - `ApplicaStrutturaVoluta`, `conAlias`
  - `componenteNato` (una proposta che ha cambiato codice nel frattempo è un rifiuto «riapri l'editor»)
- **`smistamento_flusso.go`** (il flusso F8): V1 (EM §10.7, «i lettori da adeguare») — `LeggiStatoFlusso` salta le occorrenze
  non attive nella Sorgente (`ListOccorrenzeNonAttiveRfq`): un file in Altri file o scartato col ciclo dei file non riceve una
  destinazione, non fa da ancora e non passa avanti nelle analisi

## Entry point

- **`ApplicaStruttura`, `MotoreDellaRfq`**
  - Chi lo chiama:
    - `transport/workerapi/workerapi.go` (`applicaFattiEsistenti`, `strutturaNelleRfq`, `motoreDelFile`)
    - qui dentro `RileggiStepDellaRfq` (quindi `RianalizzaRfq`), `rileggiIlContenuto` (`autorizzazione.go`: dopo
      un'autorizzazione, una revoca, una riattivazione o un cambio di tipo) e `rileggiIlFile` (`decisioni.go`: dopo
      `AccettaNodo`, `AccettaSottoalbero` e `AccettaFile`, anche nelle forme `…Deciso`)
- **`RianalizzaRfq`**
  - Chi lo chiama: `transport/web/proposte.go` (`rianalizza`, il gesto «Rianalizza»: è il suo solo chiamante, e
    con lui `AccodaStepDaRileggere` e `AccodaPdfDaRileggere`). Aprire una pagina non lo chiama (Smistamento F1, R8:
    nessuna GET scrive); la vecchia `rileggiAllApertura` non c'è più.
- **`RileggiStepDellaRfq`**
  - Chi lo chiama: `RianalizzaRfq` e `transport/web/triage.go:preparaDopoLaDecisione` (nuova RFQ, aggancio)
- **`AccodaAnalisiMancantiDaSola`**
  - Chi lo chiama: `transport/web/fascicolo_preparazione.go:preparaFile` (la POST `…/fascicolo/prepara` che la pagina
    manda aprendosi, solo per chi scrive: accoda e basta)
- **`AccettaNodo`, `ScartaNodo`, `CodiceDelNodo`, `AccettaRelazione`, `ScartaRelazione`, `AccettaSottoalbero`, `AccettaFile`, `AccettaRimozione`, `ScartaRimozione`**
  - Chi lo chiama:
    - `transport/web/proposte.go`
    - `AccettaFile` anche da `fascicolo_conferma.go:applicaPiano`
- **`ApplicaStrutturaVoluta`**
  - Chi lo chiama: `transport/web/fascicolo_gesti_v3.go` (`POST /thread/{id}/fascicolo/bom/applica`)
- **`ModificaComponente`, `Collega`, `Scollega`, `Sposta`, `ArchiviaComponente`, `RimuoviComponente`, `ScegliStepStrutturale`, `ConcediDeroga`, `RevocaDeroga`, `ConcediDerogaStruttura`, `RevocaDerogaStruttura`, `Sostituisci`, `AnnullaSostituzione`, `ApriRevisione`, `AbbandonaBozza`, `CongelaBom`, `AggiornaTutteLeRimozioni`**
  - Chi lo chiama:
    - `transport/web/fascicolo_rotte.go`
    - `Sostituisci` anche da `fascicolo.go:applicaScelta` (la sostituzione scelta assegnando un file, `assegnaAlComponente`, o confermandolo, `allegati.go:confermaProposta`)
    - `ScegliStepStrutturale` da `fascicolo_conferma.go:applicaPiano`
- **`CandidatiDellaRfq`, `Unisci`, `AggiungiDaCodice`, `RipristinaComponente`, `NomeTipo`, `TipiDaCodice`**
  - Chi lo chiama:
    - `transport/web/codici.go`, `fascicolo_pagina.go`, `fascicolo_editor.go`
    - `NomeTipo` e `TipiDaCodice` anche come funzioni di template (`server.go`)
- **`Rifiuto`**
  - Chi lo chiama: `transport/web/fascicolo.go` (`type rifiuto = fascicolo.Rifiuto`: un «no» del web e uno del core si riconoscono con lo stesso `errors.As` in `spiegaErrore`)
- **`LeggiPianoFascicolo`, `PianoFascicolo`, `VoceFile`, `Domanda`, `Voce*`**
  - Chi lo chiama: `transport/web/fascicolo_pagina.go`, `fascicolo_conferma.go`, `fascicolo_documenti.go`, `fascicolo_dettaglio.go`
- **`AssicuraProdottiDellaRichiesta`, `PreparaFile`**
  - Chi lo chiama:
    - `PreparaFile`: `transport/web/fascicolo_preparazione.go:preparaFile` (la POST della preparazione, che la pagina
      manda aprendosi solo per chi scrive; la GET non scrive)
    - tutti e due: `triage.go:preparaDopoLaDecisione` (nuova RFQ, aggancio)
- **`LavoroInCorso`, `NuovoAlbero`, `LeggiGate`, `DifferenzeWorking`, `EtichettaStep`, `Step`, costanti `Step*`**
  - Chi lo chiama: `transport/web/fascicolo_pagina.go`, `fascicolo_bom.go`, `fascicolo_editor.go`
- **`WorkingBloccata`**
  - Chi lo chiama: `transport/web/allegati.go`, `fascicolo.go`, `fascicolo_editor.go`
- **`NotaInterna`, `RegistraCaricamento`, `Caricato`**
  - Chi lo chiama:
    - `transport/web/caricamento.go` (carica a mano)
    - `fascicolo_nas.go` (importa dal NAS)
- **`RevisioneProponibile`**
  - Chi lo chiama: nessuno, né il codice né le prove.
    - La stessa regola (un file caricato a mano non propone la revisione del cliente) la applica
      `classificazione.Valuta` con `IngressoFile.Interno`, che `valutaLaRadice` passa.
- **`Ciclo`, `CreerebbeCiclo`, `Valuta`, `Differenze`, `ClassificaNodi`, `Pianifica`, `RimozioniFigliDiretti`, `RadiceDelloStep`, `PianoDelFascicolo`, `AggiornaRimozioni`, `SeBloccata`, `LeggiIngressoPiano`**
  - Chi lo chiama: solo questo package e le sue prove

## Dati

- **Il package non apre transazioni**: ogni funzione lavora sul `*db.Queries` di chi chiama, e qualunque errore o `Rifiuto` annulla tutto.
- I gesti arrivano da `transport/web` (`gesto`, `inTransazione`), i fatti da `transport/workerapi` nella transazione del risultato.

### Legge

- `thread_offerta`, `fase_log`, `bom_versione`, `componente`, `componente_relazione`, `componente_proposta`, `relazione_proposta`, `rimozione_proposta`, `documento`, `documento_proposta`, `deroga_struttura`, `identificativo_thread`, `cliente.regole`, `allegato`, `messaggio`, `analisi_fatti`, `analizzatore_corrente`, `job` (`UltimoJobPerChiave`, `ListLavoroPendenteRfq`);
- le viste `v_step_prodotto`, `v_fascicolo`, `v_thread_bloccanti` (dentro `GateCongelamento`), `v_codici_candidati_thread`;
- la funzione `struttura_motivo_parziale` (`MotivoParziale`: la completezza di una lettura la dice il database, non il Go).

### Scrive

- **`CongelaBom`, `ApriRevisione`, `AbbandonaBozza`**
  - `bom_versione` (insert, congelamento, delete della bozza)
  - `bom_versione_componente` / `_relazione` / `_documento` / `_deroga` (le istantanee)
  - `fase_log` (chiude la riga aperta e ne apre una)
- **`ApplicaStruttura`**
  - `componente_proposta`, `relazione_proposta` (solo le righe riscrivibili che cambiano)
  - `documento_proposta` (`valutaLaRadice` con `AggiornaValutazioneProposta`: solo una proposta aperta, senza componente
    e con `fonte <> 'operatore'`)
  - poi `AggiornaRimozioni`
- **`AggiornaRimozioni`**
  - `rimozione_proposta` (upsert delle aperte, chiusura di quelle che non valgono più)
- **gesti di `decisioni.go`**
  - `componente` (insert, ripristino)
  - `componente_relazione` (insert, qta, delete per una rimozione)
  - stato delle tre tabelle di proposta (la nota tagliata a 200 caratteri)
- **gesti di `modifiche.go`**
  - `componente` (tipo, rev, descrizione), `componente_relazione`, `deroga_fabbisogno`
  - `rimozione_proposta` (`Collega` e `Sposta` chiudono la rimozione aperta sull'arco appena messo, `TieniArcoDellaRimozione`)
  - `conversazione`, `messaggio`, `allegato` (il contenitore dei caricamenti interni)
- **gesti di `struttura.go`**
  - `componente` (archiviazione, `step_strutturale_id`, delete)
  - `componente_relazione` (archiviando)
  - `deroga_struttura`
  - `documento.sostituito_da`
  - proposte chiuse con la nota (tagliata a 200 caratteri)
  - archiviando un prodotto, le rimozioni aperte del suo STEP strutturale (`ChiudiRimozioniDiUnoStep`, nota «prodotto archiviato»)
- **`AggiungiDaCodice`**
  - `componente` (solo per un codice senza proposta STEP aperta)
- **`AssicuraProdottiDellaRichiesta`**
  - `componente` (+ `RiconciliaProposteNodo` sulle proposte aperte con quel codice)
- **`ApplicaStrutturaVoluta`**
  - tutto quello dei gesti che compone,
  - più i codici scritti e le radici riconosciute su `componente_proposta`,
  - `SetQtaRelazione`,
  - `componente.tipo` (particolare → assieme)
  - e `rimozione_proposta` «tenuto nella struttura confermata»
- **`RianalizzaRfq`, `PreparaFile`**
  - `job` (`analizza_allegato`, `stage_allegato`, `estrai_archivio`, attraverso `platform/coda`)
  - `allegato` (lo stato «in coda», dentro `coda.AccodaStage`)

### Lucchetti

- `BloccaThread` (`thread_offerta FOR UPDATE`) mette in fila i gesti sulla stessa RFQ,
  - e il trigger della working bloccata prende la stessa riga `FOR KEY SHARE`.
- La RFQ si blocca **prima** di componenti, documenti e proposte:
  - con l'ordine rovesciato due transazioni si aspettano a vicenda e PostgreSQL ne ferma una (40P01).
- Lo prendono da sé:
  - le tre funzioni di `versioni.go`, `AssicuraProdottiDellaRichiesta` e `ArchiviaComponente`;
  - `ScartaNodo`, `ScartaRelazione`, `ScartaRimozione` e `CodiceDelNodo`
    - (solo il lucchetto, senza D26: non cambiano la working e valgono anche con la BOM congelata);
  - tutto ciò che passa da `prepara`
    - (`AccettaNodo`, `AccettaRelazione`, `AccettaSottoalbero`, `AccettaFile`, `AccettaRimozione`, `RipristinaComponente`, `RevocaDerogaStruttura`, `AggiungiDaCodice`, i sei gesti di `modifiche.go`, `ApplicaStrutturaVoluta`).
- Gli altri gesti di `struttura.go` (`RimuoviComponente`, `ScegliStepStrutturale`, `ConcediDerogaStruttura`, `Sostituisci`, `AnnullaSostituzione`) lo aspettano da chi chiama:
  - `transport/web/fascicolo_rotte.go:preparaGesto` (anche prima di `ArchiviaComponente` e `RevocaDerogaStruttura`, che lo riprendono senza danno),
  - e `preparaGesto` all'inizio di `transport/web/fascicolo.go:assegnaAlComponente`, di `correggiCodiceComponente` e di `fascicolo_conferma.go:confermaFascicolo`;
  - `allegati.go:confermaProposta` prende la RFQ `FOR KEY SHARE` prima della proposta.
- Un 40P01 che arriva lo stesso diventa per l'operatore «riprova» (`transport/web/fascicolo.go:spiegaErrore`).
- `ApplicaStruttura`, `RianalizzaRfq` e `PreparaFile` non lo prendono.
- Lucchetti di riga:
  - `BloccaComponente`, `BloccaComponenteProposta`, `BloccaRelazioneProposta`, `BloccaRimozioneProposta`,
  - `BloccaDocumento` (in ordine di id, `bloccaDocumenti`),
  - `BloccaMessaggio` (il contenitore dei caricamenti, prima di scegliere l'indice).

### Disco

- Solo lettura: `inStaging` (`os.Stat`) dice se il contenuto di un allegato è ancora nello staging (`AccodaAnalisiMancanti`, `AccodaStepDaRileggere`, `AccodaPdfDaRileggere`, `PreparaFile`, `LeggiIngressoPiano`).
- Nessuna scrittura su disco o sul NAS.

## Flussi principali

### Dai fatti di uno STEP alle proposte

- `ApplicaStruttura`, a ogni risultato dell'analisi, a ogni riuso dei fatti, e con `RileggiStepDellaRfq` quando la RFQ
  nasce o un messaggio vi si aggancia e al gesto «Rianalizza» (`RianalizzaRfq`); mai all'apertura di una pagina
  (Smistamento F1, R8):
  1. nessuna chiave `struttura` nei fatti o nessuno sha256 → niente; un file fuori dal flusso (`FileNelFlusso`) →
     niente, con `FuoriFlusso`;
     - `MotivoParziale` dice se la lettura è completa;
  2. il **portatore**: lo stesso contenuto arrivato due volte nella RFQ propone una volta sola (`PortatoreDelFile`);
  3. `ClassificaNodi` con il `Motore` del cliente;
     - la classificazione del server resta nell'evidenza della riga (`evidenza.classificato`, K2);
     - un codice scritto dall'operatore vince sulla classificazione: codice, rev, famiglia e confidenza della riga
       restano i suoi;
  4. `Pianifica`, con il `Contesto`: la working (`leggiWorking`, solo gli archi fra componenti attivi), i nodi del file
     decisi da una persona (`DecisoDaUnaPersona`) e le sorgenti autorizzate del file (`LeggiDichiarazioni`);
     - una riga non riscrivibile (decisa da una persona: `riscrivibile` vuole una riga aperta, o scartata da un
       automatismo, E33) o già uguale (`stessoNodo`) si salta; le altre vanno in `UpsertComponenteProposta`;
  5. gli archi (`scriviArchi`, con la stessa regola);
     - se le sorgenti sono cambiate durante la lettura, gli archi si ripianificano e si riscrivono;
     - `ancoraAperte` conta le righe che chiedono ancora una decisione;
  6. `valutaLaRadice` (la radice dello STEP come evidenza del codice del documento, `AggiornaValutazioneProposta`) e
     poi `AggiornaRimozioni` per ogni autorizzazione del file (una non valida dà solo `Sospese`).

### Le rimozioni (`AggiornaRimozioni`)

- STEP strutturale scelto e corrente
- → esito `presente_analizzato`
- → struttura leggibile con una radice sola
- → ogni nodo del file ha un componente, oppure un codice nuovo, oppure è scartato;
  - un nodo senza codice ancora aperto **sospende** il calcolo (`EsitoRimozioni.Sospese` dice perché).
- Poi `RimozioniFigliDiretti` (gli archi della working che partono dal componente e che il file non contiene, a
  profondità 1)
- → upsert delle aperte, chiusura di quelle che non valgono più.
- Si ricalcolano dopo ogni decisione (`dopoLaDecisione`, o `dopo` in `transport/web/fascicolo_rotte.go`).
- Le chiudono senza ricalcolo:
  - un arco appena messo a mano con `Collega` o `Sposta` (`tieniArcoMessoAMano`, come fa l'editor: lo STEP quell'arco non ce l'ha, ed è per questo che una persona l'ha messo);
  - un prodotto archiviato, le cui rimozioni nessun ricalcolo guarderebbe più (`ListProdottiConStepStrutturale` elenca solo i prodotti attivi);
  - un cambio di STEP strutturale e la sostituzione di uno STEP, per quelle del file vecchio.
- Una rimozione chiusa così è `scartata` e non si ripropone.

### Una decisione sulle proposte

- `prepara`
- → la proposta bloccata e controllata
- → `accettaNodo` (ritrova il componente per codice, anche con il suffisso decorativo, e lo ripristina se archiviato; altrimenti lo crea)
  - o `accettaRelazione` (i due nodi decisi, niente archiviati, `CreerebbeCiclo`; quantità diversa solo se la proposta l'ha fatta contro la quantità che la working ha ancora)
- → riconciliazione delle altre proposte uguali
- → `dopoLaDecisione`.
- `AccettaFile` / `AccettaSottoalbero`:
  - prima i nodi in ampiezza, poi gli archi fra nodi accettati, in una transazione sola;
  - un rifiuto annulla tutto.

### L'editor della struttura (`ApplicaStrutturaVoluta`)

- `prepara`
- → letture (working, proposte, codici trovati se servono, le regole del cliente con `MotoreDellaRfq` per gli alias dei suffissi: un errore del database qui ferma il gesto)
- → `pianifica`, che rifiuta prima di scrivere (riferimenti, codici, quantità, archi ripetuti, archi non appesi alla radice, archi della working che l'editor non mostrava o che sono cambiati, proposte decise nel frattempo, ciclo nel grafo finale)
  1. → codici scritti, scarti (con gli archi che toccano il nodo), radici dello STEP riconosciute come il prodotto
  2. → componenti nuovi (da una proposta o da un codice trovato; se la proposta ha cambiato codice nel frattempo, `componenteNato` rifiuta con «riapri l'editor») e nodi ritrovati
  3. → archi:
     - prima si tolgono quelli mostrati e non voluti,
     - poi le quantità,
     - poi si aggiungono (accettando la proposta uguale se c'è, altrimenti `collega`)
  4. → proposte di arco uguali a un arco voluto → `duplicato`, quelle mostrate e non volute → chiuse con il motivo
  5. → un particolare con figli diventa assieme → rimozioni ricalcolate
  6. → le rimozioni sugli archi appena confermati si chiudono «tenuto nella struttura confermata».

- Un componente che esce dall'albero porta con sé i suoi figli;
- un componente entrato per codice (`k:`, o un nodo ritrovato) tiene i suoi archi com'erano.

### All'apertura del Fascicolo e alla nascita della RFQ

- Aprire una pagina (`transport/web`, GET) non scrive niente, per nessun ruolo (Smistamento F1, R8): legge, fra
  l'altro, il piano (`LeggiPianoFascicolo`).
  - un errore del database in `MotoreDellaRfq` ferma il piano: senza i suffissi del cliente proporrebbe «X» accanto a «X_PRT»;
  - regole scritte male non sono un errore, restano fuori
- La preparazione è la POST `…/fascicolo/prepara` che la pagina manda aprendosi, solo per chi scrive, e accoda e basta:
  - `PreparaFile`
    - download degli allegati utili, archivi fermi all'estrazione, file fermi all'analisi o alla rilettura dei fatti già calcolati;
    - un job fallito con la stessa chiave non si riaccoda;
    - un allegato il cui messaggio non è in nessuna casella attiva va fra i `Saltati`, un altro errore di `coda.CopiaPerDownload` ferma la preparazione
  - → `AccodaAnalisiMancantiDaSola`, con quello che resta del limite: gli STEP senza i fatti correnti, senza riprovare
    da sola un'analisi fallita. Non rilegge gli STEP già analizzati e non riaccoda gli STEP da rileggere né i PDF:
    nessuna rianalisi parte da sola (la fa solo «Rianalizza», `RianalizzaRfq`).
- Alla nascita della RFQ e all'aggancio (POST del triage, `preparaDopoLaDecisione`):
  - `AssicuraProdottiDellaRichiesta`
    - ogni codice della richiesta confermato da una persona in quel gesto (scelta 6) diventa un componente radice `finito`, se nella RFQ non c'è già un componente con quel codice, anche archiviato;
    - con la BOM congelata niente
  - → `PreparaFile` (senza la rilettura dei fatti)
  - → `RileggiStepDellaRfq`: gli STEP analizzati prima dell'aggancio ricevono le loro proposte in questa RFQ
- «Conferma Fascicolo» (in `transport/web/fascicolo_conferma.go`) ricalcola il piano nella sua transazione e conferma solo se `Firma` è quella che l'operatore aveva davanti.

### Congelare e rivedere

- `CongelaBom`:
  - RFQ bloccata
  - → fase aperta
  - → ultima versione: bozza da congelare, oppure V1 con il contesto dalla fase
  - → `LeggiGate`
  - → istantanee
  - → congelamento
  - → per `preventivo` `SCHEDA_COSTO` con la baseline
- `ApriRevisione` vuole il motivo e la tabella di `contestoRevisione`.
- `AbbandonaBozza` vuole `DifferenzeWorking` vuota.

### I codici della RFQ

- `CandidatiDellaRfq`
- → `Unisci` (nessuna riclassificazione: le evidenze vengono da `v_codici_candidati_thread`)
- → per ogni codice la situazione (proposta STEP aperta, componente, archiviato, codice della richiesta, nuovo)
- → `AggiungiDaCodice` rilegge tutto sotto il lucchetto e crea il componente solo per un codice nuovo o della richiesta, con la revisione scelta fra quelle viste.

### Un caricamento interno

- `NotaInterna` (il messaggio del canale `nota` della RFQ, creato la prima volta e poi ritrovato)
- → `RegistraCaricamento` (allegato di origine `manuale`, già nello staging).
- Da lì fa la strada degli allegati;
- `RevisioneProponibile` impedisce che la sua proposta porti una revisione del cliente.

## Invarianti

- **Nessuna interpretazione scrive la working.**
  - `ApplicaStruttura`, `Pianifica` e `AggiornaRimozioni` scrivono solo le tabelle di proposta e `documento_proposta`;
  - `componente_relazione` la scrivono solo `accettaRelazione`, `collega`, `scollega`, `AccettaRimozione`, `ArchiviaComponente` e `ApplicaStrutturaVoluta` (con le stesse primitive, più `SetQtaRelazione`).
- **Una proposta decisa non si riscrive**:
  - il Go la salta (`ApplicaStruttura` con `riscrivibile`, `AggiornaRimozioni`) e le upsert di nodi e archi riscrivono
    solo una riga aperta o scartata senza `deciso_da`.
  - Un codice scritto dall'operatore resta anche se le regole cambiano (override in `ApplicaStruttura`, `CASE` in `UpsertComponenteProposta`),
  - e `valutaLaRadice` non riscrive la proposta di un documento con `fonte = 'operatore'` (il muro sta nella query
    `AggiornaValutazioneProposta`).
- **Il codice di un componente non si riscrive per i suffissi decorativi**:
  - gli alias si cercano (`decisioni.go:componenteConSuffisso`, `voluta.go:conAlias`, la mappa `alias` di `piano.go:PianoDelFascicolo`, `candidati.go`).
  - Senza le regole del cliente lette non si decide: ogni chiamante restituisce l'errore del database di `MotoreDellaRfq` invece di proseguire con un motore vuoto.
- **Un prodotto archiviato non tiene rimozioni aperte e non riceve un nuovo STEP strutturale**
  - `ArchiviaComponente` chiude quelle del suo STEP;
  - `ScegliStepStrutturale` e `Sostituisci` con il nuovo riferimento lo rifiutano finché non è ripristinato;
  - la sostituzione semplice resta possibile.
- **Una nota su una proposta sta nella colonna**:
  - quelle che contengono un nome (un file fino a 300 caratteri, il nome grezzo di un nodo) si tagliano a 200 caratteri, contati come rune (`tagliaNota`).
- **Un id che arriva da un indirizzo vale solo nella sua RFQ**:
  - `RevocaDerogaStruttura` cancella con `deroga_struttura_id` **e** `thread_id` (come `RevocaDeroga`).
- **Dopo il congelamento la working non cambia**:
  - `SeBloccata` lo dice prima,
  - il trigger `bom_working_modificabile` (0020) è il muro.
- **Niente sotto un particolare** (Distinta, 29/09/2026):
  - sotto un particolare e sotto un particolare commerciale non si mette niente; i pezzi stanno sotto il prodotto e sotto
    gli assiemi (`nuovo.go:Contenitore`).
  - Lo rifiutano `collega` (quindi `Collega` e `Sposta`) per un arco nuovo e `pianifica` per gli archi che l'editor
    manda, con un componente che c'e' o un codice scritto; `MotivoTipoSpento` spegne «particolare» e «particolare
    commerciale» per un pezzo che ha dei figli.
  - Un nodo proposto da uno STEP con dei figli nasce assieme (`tipoVoluto`), e `accettaRelazione` fa diventare assieme
    il padre particolare di un arco dello STEP: lo dice il file, non una persona.
  - `TipiNuovo` comprende il particolare commerciale: la Distinta ha un pulsante per crearlo.
- **Nessun ciclo entra da qui**:
  - ogni nuovo arco passa da `CreerebbeCiclo` (`accettaRelazione`, `collega`) o dal controllo del grafo finale (`pianifica`);
  - il gate rifiuta di congelare una working con un ciclo.
  - Il database non li vieta.
- **Una mancanza in una lettura parziale non è un'informazione**:
  - le rimozioni nascono solo dallo STEP strutturale con esito `presente_analizzato`,
  - una quantità diversa si propone solo da una lettura completa.
- **Revisioni discordanti non si scelgono per punteggio** (`Unisci`, `revisioneScelta`);
  - un file caricato a mano non propone la revisione del cliente (`RevisioneProponibile`).
- **Un componente con storia non si cancella**:
  - `RimuoviComponente` riesce solo se nessuna FK lo tiene;
  - altrimenti si archivia (e archiviare conserva documenti, proposte, deroghe).
- **Il piano è puro e firmato**:
  - `PianoDelFascicolo` non scrive;
  - stessa entrata, stessa `Firma`.
- **L'albero è deterministico**:
  - `NuovoAlbero` dà gli stessi `Totali` in qualunque ordine arrivino componenti e archi, anche con un ciclo nella working.
- **Un identificativo senza `confermato_da` non diventa un componente** (`AssicuraProdottiDellaRichiesta`).

## Dipendenze

- Importa:
  - `core/inbox/classificazione` (il `Motore`, `CodiceAmmissibile`, `RevAmmissibile`, `PropostaDaNome`, `DiFamiglia`)
  - `core/registro/regole` (`LeggiRegole`)
  - `platform/db`
  - `platform/coda` (accodare analisi, download ed estrazioni)
  - `platform/contratti/worker` (`StrutturaSTEP`, `DecodificaStruttura`, `PayloadEstraiArchivio`)
  - `platform/storage/staging` (`FileStaging` per `coda.AccodaStage`).
- È importato da `transport/web` e `transport/workerapi`.
- Tutto dentro la tabella di `internal/README.md` (`core/*` → `core/*`, `platform`).

## Test

- **Livello L1**
  - **`gate_test.go`**: ogni condizione del gate, `Ciclo`, `EtichettaStep`, `Differenze`
  - **`proposte_test.go`**
    - `ClassificaNodi` (famiglia, generico, id contro nome, rev del file, misure)
    - `Pianifica` (tipi, duplicati, radice dichiarata, qta da lettura troncata)
    - `RimozioniFigliDiretti`, `CreerebbeCiclo`, `RadiceDelloStep`
  - **`candidati_test.go`**: `Unisci`: forte/debole, riferimento escluso, storia citata, conflitto di revisioni, situazioni, BOM congelata, ordine delle righe, suffisso decorativo
  - **`albero_test.go`**: `NuovoAlbero`:
    - radici multiple, condiviso, archiviati, ciclo;
    - i `Totali` con un ciclo uguali su 50 ordini a caso dei dati
  - **`piano_test.go`**: `PianoDelFascicolo`: zip, lavoro in corso, ambiguità, fogli insieme, stesso contenuto, nodo senza codice, BOM congelata, STEP strutturale suggerito, `Firma`, suffissi
  - **`voluta_test.go`**
    - `pianifica` (nodi proposti, radice, quantità, perimetro e i due limiti con il loro messaggio, cicli, scarti, dati cambiati, una carta per codice)
    - `fraseVoluta`, `tipoVoluto`
  - **`note_test.go`**: `tagliaNota`: 200 caratteri contati come rune
- **Livello L4**
  - **`fascicolo_db_test.go`**: congelamento, fasi e baseline, revisioni e abbandono, sostituzioni, STEP strutturale, deroghe, archiviazione
  - **`proposte_db_test.go`**: fan-out, rimozioni (troncato, scarti del parser, più radici, fatti v2, nodo senza codice), accettazioni, cicli, rianalisi, riclassificazione, D16, BOM congelata
  - **`candidati_db_test.go`**: la vista dei candidati e `AggiungiDaCodice`
  - **`preparazione_db_test.go`**: prodotti della richiesta, download, file fermi, lavoro in corso, piano dal database
  - **`suffissi_db_test.go`**: i suffissi decorativi nei nodi, nei componenti già nati, in D16, nel piano
  - **`nato_db_test.go`**: `componenteNato`: un codice con cui non è nato niente è un `Rifiuto`, non un `no rows` grezzo
  - **`revisione_db_test.go`**
    - archiviare un prodotto chiude le rimozioni del suo STEP;
    - nota lunga che non ferma la sostituzione;
    - STEP strutturale rifiutato su un archiviato (anche con la sostituzione);
    - scarti e codice del nodo che aspettano la RFQ bloccata;
    - deroga strutturale revocata solo dalla sua RFQ;
    - arco messo a mano non proposto da togliere;
    - D16 che non riscrive una decisione dell'operatore
- I test L4 hanno `//go:build integrazione` e vogliono `COCKPIT_TEST_DSN`.
- `ApplicaStrutturaVoluta`, i gesti di `modifiche.go` e le rotte del Fascicolo sono provati
  - in L4 da `transport/web`:
    - `v3_db_test.go`, `b85_db_test.go`, `b86_db_test.go`, `b87_db_test.go`, `b87b_db_test.go`, `a4_db_test.go`;
    - `gesti_db_test.go` per il codice scritto a mano e l'ordine dei lucchetti,
    - L1 `gesti_test.go` per la quantità a 32 bit, il `Rifiuto` condiviso e il 40P01;
  - e nel browser (L7) da `fascicolo_browser_test.go` e `fascicolo_v3_browser_test.go`.
- `AccodaStepDaRileggere` e la sua parte di `RianalizzaRfq` (LS-01) sono provati in L4 da
  `transport/workerapi/ls01_struttura_db_test.go` (P-10: la riaccodatura mirata, lo staging, la flotta mista, «Riscarica»;
  P-10b: per contenuto, la copia nello staging, i compatibili prima, il limite condiviso con le analisi mancanti e i
  PDF), con i risultati che passano per la strada vera del server (`applicaRisultato`, `dopoStaging`); le frasi del
  gesto da `transport/web/ls01_rianalizza_db_test.go`.

## Stato dell'implementazione

- **Completo**:
  - versioni, gate, differenza, archiviazione, sostituzioni e deroghe (A4, B8.A4a);
  - proposte, rimozioni, decisioni e rianalisi (B8.5);
  - codici della RFQ (B8.6);
  - albero e correzioni a mano (B8.7);
  - preparazione e piano (B8.7b);
  - editor della struttura e suffissi decorativi (Fascicolo v3).
- **Aperto per decisione**: il gate non controlla «zero proposte di sostituzione pendenti» (A4.6),
  - perché il modello non registra una proposta di sostituzione (commento in testa a `gate.go`).
- **Aperto per decisione: la revisione di un nodo STEP** (domanda D-REV-01, in attesa della risposta dell'utente).
  - Qui la formazione dello STEP (`rev_grezza`) diventa la revisione proposta, salvo «ANY» (`ClassificaNodi`, B4.10).
  - Il Motore A non la usa mai come revisione: la conserva come formazione (D1, `core/estrazione/step.go`,
    `core/ancoraggio/struttura_file.go`).
  - La divergenza è registrata nell'audit dei due classificatori del 13/10; fino alla risposta il codice resta com'è.
- **Dal Fascicolo v3 la struttura di uno STEP non entra più con «Conferma Fascicolo»**:
  - `struttureDi` mette ogni struttura con nodi o archi aperti in `decidere` (`DomandaStrutturaEditor`),
  - quindi la mappa `inArrivo` resta vuota e un file che va a un nodo proposto aspetta l'editor (`DaStep` con `DomandaComponente`).
  - Il ramo «struttura pronta» del piano e il suo uso in `transport/web/fascicolo_conferma.go` non scattano più.
- **Limiti noti**:
  - `AssicuraProdottiDellaRichiesta` gira solo alla nascita della RFQ e all'aggancio, con i codici confermati in quel
    gesto (scelta 6; mai all'apertura di una pagina), e guarda solo `identificativo_thread`:
    - un prodotto della richiesta tolto con `RimuoviComponente`, o il cui codice è stato corretto (`transport/web/fascicolo.go:correggiCodiceComponente` non aggiorna l'identificativo), rinasce con il codice della richiesta come radice `finito` senza figli se un gesto conferma di nuovo quel codice.
    - Un prodotto archiviato invece non rinasce.
  - Sempre lì, il componente esistente si cerca solo per codice esatto (`GetComponentePerCodice`), senza gli alias dei suffissi decorativi:
    - con «X_PRT» nato prima della regola, il codice della richiesta «X» fa nascere un secondo componente.
  - `piano.go:struttureDi` riconosce un nodo archiviato e una quantità diversa dal testo della nota della proposta («archiviato», «qta diversa»), non da un campo.
- Lo spostamento sul NAS di un documento già scritto (B8.8) non passa da qui.

## Dove intervenire

- **capire perché una BOM non si congela**
  - Apri: `gate.go:Valuta`, poi `GateCongelamento` e `ListGateStep` in `platform/db/queries/bom.sql`
- **capire perché una revisione non si apre o non si abbandona**
  - Apri: `versioni.go:contestoRevisione`, `AbbandonaBozza`
- **capire perché un nodo di uno STEP ha (o non ha) un codice**
  - Apri: `classifica.go:ClassificaNodi`, `primo`, `alternativo`
- **capire perché un nodo è `duplicato` o una quantità non è proposta**
  - Apri: `proposte.go:Pianifica`, `Contesto.componenteDi`
- **capire perché una rimozione non è stata proposta**
  - Apri: `rimozioni.go:AggiornaRimozioni` (`EsitoRimozioni.Sospese`)
- **cambiare che cosa succede quando si accetta un nodo o un arco**
  - Apri: `decisioni.go:accettaNodo`, `accettaRelazione`
- **capire perché l'editor rifiuta una struttura**
  - Apri: `voluta.go:pianifica`
- **cambiare l'ordine in cui l'editor scrive**
  - Apri: `voluta.go:ApplicaStrutturaVoluta` (passi 1–6)
- **capire perché un file è `decidere` o `attesa` nel piano**
  - Apri: `piano.go:voceFile`, `fratelli`, `struttureDi`
- **capire perché un codice sta fra gli altri riferimenti, o quale gesto offre**
  - Apri: `candidati.go:forte`, `classe`, `situa`
- **capire perché un file non si scarica o non si analizza da solo**
  - Apri: `preparazione.go:PreparaFile`, `giaProvato`
- **un gesto nuovo sulla working**
  - Apri: `modifiche.go` (con `prepara` e `dopoLaDecisione`), poi la rotta in `transport/web/fascicolo_rotte.go`
- **capire perché una rimozione si è chiusa da sola**
  - Apri: `modifiche.go:tieniArcoMessoAMano`, `struttura.go:ArchiviaComponente`, `ScegliStepStrutturale`, `chiudiProposteDi`
- **capire perché i totali dell'albero sono quelli**
  - Apri: `albero.go:totali`, `inUnCiclo`
- **capire un 40P01 o un gesto che aspetta**
  - Apri: il paragrafo «Lucchetti» qui sopra; `transport/web/fascicolo_rotte.go:preparaGesto`, `fascicolo.go:spiegaErrore`
- **un esito nuovo dello STEP del prodotto**
  - Apri: `v_step_prodotto` in migrazione, costanti `Step*` e `EtichettaStep` in `gate.go`, `Valuta`

## Leggi anche

- `internal/core/README.md` (il posto del package fra gli altri)
- `internal/README.md` (architettura e dipendenze)
- `internal/transport/README.md` (le rotte del Fascicolo)
- `internal/core/rfq/documenti` (i nomi sul NAS e la copia)
- `migrations/0018_fascicolo.sql`, `0020_bom_versioni.sql`, `0021_annotazioni_pdf.sql`
- le query in `internal/platform/db/queries/`:
  - `proposte.sql`, `struttura.sql`, `bom.sql`, `editor.sql`, `candidati.sql`, `preparazione.sql`, `schermata.sql`
- e in `cockpit/README.md` la sezione «Il Fascicolo v3».
