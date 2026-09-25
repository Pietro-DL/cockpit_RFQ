# `internal/core/rfq/fascicolo` — la BOM di una RFQ nel tempo

## Scopo

Che cosa è la BOM di una RFQ e come cambia: i componenti e gli archi della **working**, le proposte che
gli STEP ne fanno, le decisioni che le portano nella working, le versioni congelate con le loro
istantanee, il gate che decide se si può congelare, e quello che il sistema prepara da solo prima che
qualcuno decida (i prodotti della richiesta, i file da scaricare e analizzare, il piano di conferma).

I tre strati valgono anche qui. **Fatto**: `analisi_fatti.fatti.struttura`, scritto dal worker.
**Interpretazione**: `componente_proposta`, `relazione_proposta`, `rimozione_proposta`, scritte da
`ApplicaStruttura` e `AggiornaRimozioni`. **Decisione**: `componente` e `componente_relazione`, scritte
solo dai gesti di una persona (e da `AssicuraProdottiDellaRichiesta`, che traduce un codice della richiesta
già confermato da una persona).

Le regole che contano le tiene il database (0018, 0020): identità `(thread, upper(codice))`, working
bloccata dopo il congelamento (trigger, errori `BOM01`–`BOM05`), versioni e istantanee immutabili, catena
delle revisioni dei documenti. Qui si decide **che cosa** scrivere e si dice all'operatore **perché** qualcosa
non si può fare, prima che ci sbatta contro il vincolo.

## Non appartiene qui

- Rotte, form, template, la traduzione degli errori `BOM0x` in frasi: `transport/web` (`fascicolo*.go`,
  `proposte.go`, `codici.go`, `caricamento.go`).
- La conferma di un documento (`documento_proposta` → `documento`, «aggiungi / sostituisce», percorso sul
  NAS) e «Conferma Fascicolo» che applica il piano: `transport/web/allegati.go:confermaProposta`,
  `transport/web/fascicolo_conferma.go:applicaPiano`.
- La correzione del codice di un componente (tocca documenti e NAS): `transport/web/fascicolo.go:correggiCodiceComponente`,
  con la stessa guardia degli altri codici (`classificazione.CodiceAmmissibile`).
- I nomi sul NAS, la copia, il ricognitore: `core/rfq/documenti`.
- Le note sui disegni (`annotazione_pdf`, 0021): `transport/web/fascicolo_gesti_v3.go`. Qui compaiono
  solo nel messaggio di `RimuoviComponente` quando la FK le nomina.
- La classificazione dei testi e i suffissi decorativi (`Canonico`, `CanonicoNome`, `HaSuffissi`):
  `core/inbox/classificazione`. Questo package li usa, non li definisce.
- L'esecuzione dei job e l'applicazione dei risultati del worker: `app/runtime`, `transport/workerapi`.

## File

| File | Responsabilità |
|---|---|
| `fascicolo.go` | Commento `// Package`. `Rifiuto` (il «no» per l'operatore: chi chiama annulla la transazione; `transport/web` usa lo stesso tipo come suo `rifiuto`), `WorkingBloccata`, `SeBloccata` (D26 detto prima del trigger), `componenteDellaRfq` (`FOR UPDATE` e appartenenza alla RFQ) |
| `versioni.go` | `CongelaBom` (gate, V1 al primo congelamento, le quattro istantanee, per una versione `preventivo` il passaggio `FATTIBILITA → SCHEDA_COSTO` con la baseline), `ApriRevisione` e `contestoRevisione` (fase → contesto; in `ACCETTATA` e `DISTINTA_ERP` sceglie chi apre), `AbbandonaBozza` (solo a differenza vuota; una `preventivo` torna alla fase d'apertura) |
| `gate.go` | `Valuta` (pura: requisiti bloccanti, proposte strutturali aperte, errori e anomalie NAS, esito dello STEP di ogni finito, cicli), `LeggiGate`, `Ciclo`, `EtichettaStep` e le costanti `Step*` (gli esiti di `v_step_prodotto`) |
| `differenze.go` | `Differenze` (pura, sulle coppie prima/dopo di `DiffBomWorking`), `DifferenzeWorking` |
| `struttura.go` | `ArchiviaComponente` (per un prodotto chiude anche le rimozioni aperte del suo STEP strutturale), `RipristinaComponente`, `RimuoviComponente` (cancellazione fisica; se la FK la rifiuta, `cheCosaLoTiene` dice quale tabella), `Step`, `ScegliStepStrutturale` (rifiutato su un archiviato), `ConcediDerogaStruttura` / `RevocaDerogaStruttura` (solo una deroga della stessa RFQ), `Sostituisci` (il nuovo riferimento strutturale rifiutato su un archiviato) / `AnnullaSostituzione`, `chiudiProposteDi` (nota tagliata a 200 caratteri) |
| `classifica.go` | `ClassificaNodi` (pura: il codice di ogni PRODUCT dal `Motore` del cliente; famiglia prima di generico, id → nome → descrizione; codici e revisioni fuori misura restano nell'evidenza), `MotoreDellaRfq` |
| `proposte.go` | `Pianifica` (pura: il file classificato contro la working → righe aperte o `duplicato`), `Working` / `NuovaWorking` / `conCanonici`, `Contesto`, `Piano`, `CreerebbeCiclo`, `Rimozioni` (pura), `RadiceDiFamiglia` (D16) |
| `applica.go` | `ApplicaStruttura`: il fan-out idempotente dei fatti di uno STEP in una RFQ; `propostaDelDocumento` (la radice riconosciuta da una famiglia corregge la proposta del documento, fonte `regola_cliente`) |
| `rimozioni.go` | `AggiornaRimozioni` (solo dallo STEP strutturale letto per intero), `AggiornaTutteLeRimozioni`, `rileggiLoStep` |
| `decisioni.go` | `AccettaNodo`, `AccettaRelazione`, `AccettaSottoalbero`, `AccettaFile`, `ScartaNodo`, `ScartaRelazione`, `CodiceDelNodo`, `AccettaRimozione`, `ScartaRimozione`; `prepara` (lucchetto della RFQ + D26), `dopoLaDecisione` (ricalcolo delle rimozioni), `componenteConSuffisso`, `tagliaNota` / `maxNota` (le note delle proposte stanno in `varchar(200)`) |
| `rianalisi.go` | `RianalizzaRfq` (rilegge gli STEP che hanno i fatti correnti, accoda gli altri fino a un limite), `MaxAccodatiPerApertura`, `inStaging` |
| `candidati.go` | `Unisci` (pura: evidenze per `upper(codice)`, forte o debole, revisioni viste, situazione e gesto), `CandidatiDellaRfq`, `AggiungiDaCodice`, `NomeTipo`, `TipiDaCodice` |
| `albero.go` | `NuovoAlbero` (puro: finiti in cima, figli per codice, il condiviso aperto una volta sola, archiviati a parte, un arco che chiude un giro non si segue, `Totali`); `totali` e `inUnCiclo` (Tarjan: chi sta in un ciclo tiene la sua quantità, chi sta sotto un ciclo la moltiplica, qualunque sia l'ordine dei dati) |
| `modifiche.go` | `ModificaComponente`, `Collega` / `Scollega` / `Sposta` (`tieniArcoMessoAMano`: l'arco appena messo non si propone da togliere), `ConcediDeroga` / `RevocaDeroga` (fabbisogno), `RevisioneProponibile`, `NotaInterna` / `ChiaveNotaInterna`, `RegistraCaricamento` / `Caricato`, `MaxQtaArco` |
| `preparazione.go` | `AssicuraProdottiDellaRichiesta`, `PreparaFile` / `RiletturaFatti`, `LavoroInCorso` (B8.7b) |
| `piano.go` | `PianoDelFascicolo` (puro: ogni file aperto `pronto` / `decidere` / `attesa` con le sue domande, le strutture degli STEP, lo STEP strutturale suggerito, `Firma`), `LeggiPianoFascicolo`, `LeggiIngressoPiano` (B8.7b) |
| `voluta.go` | L'editor della struttura (Fascicolo v3): `StrutturaVoluta`, `pianifica` (puro: riferimenti `c:` / `p:` / `k:`, perimetro, dati cambiati nel frattempo, cicli sul grafo finale; limiti `MaxArchiVoluti` archi voluti e quattro volte tanti archi visti, ciascuno con il suo messaggio), `ApplicaStrutturaVoluta`, `conAlias`, `componenteNato` (una proposta che ha cambiato codice nel frattempo è un rifiuto «riapri l'editor») |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `ApplicaStruttura`, `MotoreDellaRfq` | `transport/workerapi/workerapi.go` (`applicaFattiEsistenti`, `strutturaNelleRfq`, `motoreDelFile`); qui dentro `RianalizzaRfq` e `rileggiLoStep` |
| `RianalizzaRfq` | `transport/web/proposte.go` (`rianalizza`; `rileggiAllApertura` con `MaxAccodatiPerApertura`) |
| `AccettaNodo`, `ScartaNodo`, `CodiceDelNodo`, `AccettaRelazione`, `ScartaRelazione`, `AccettaSottoalbero`, `AccettaFile`, `AccettaRimozione`, `ScartaRimozione` | `transport/web/proposte.go`; `AccettaFile` anche da `fascicolo_conferma.go:applicaPiano` |
| `ApplicaStrutturaVoluta` | `transport/web/fascicolo_gesti_v3.go` (`POST /thread/{id}/fascicolo/bom/applica`) |
| `ModificaComponente`, `Collega`, `Scollega`, `Sposta`, `ArchiviaComponente`, `RimuoviComponente`, `ScegliStepStrutturale`, `ConcediDeroga`, `RevocaDeroga`, `ConcediDerogaStruttura`, `RevocaDerogaStruttura`, `Sostituisci`, `AnnullaSostituzione`, `ApriRevisione`, `AbbandonaBozza`, `CongelaBom`, `AggiornaTutteLeRimozioni` | `transport/web/fascicolo_rotte.go`; `Sostituisci` anche da `fascicolo.go:applicaScelta` (la sostituzione scelta assegnando un file, `assegnaAlComponente`, o confermandolo, `allegati.go:confermaProposta`), `ScegliStepStrutturale` da `fascicolo_conferma.go:applicaPiano` |
| `CandidatiDellaRfq`, `Unisci`, `AggiungiDaCodice`, `RipristinaComponente`, `NomeTipo`, `TipiDaCodice` | `transport/web/codici.go`, `fascicolo_pagina.go`, `fascicolo_editor.go`; `NomeTipo` e `TipiDaCodice` anche come funzioni di template (`server.go`) |
| `Rifiuto` | `transport/web/fascicolo.go` (`type rifiuto = fascicolo.Rifiuto`: un «no» del web e uno del core si riconoscono con lo stesso `errors.As` in `spiegaErrore`) |
| `LeggiPianoFascicolo`, `PianoFascicolo`, `VoceFile`, `Domanda`, `Voce*` | `transport/web/fascicolo_pagina.go`, `fascicolo_conferma.go`, `fascicolo_documenti.go`, `fascicolo_dettaglio.go` |
| `AssicuraProdottiDellaRichiesta`, `PreparaFile` | `transport/web/fascicolo_preparazione.go:preparaFascicolo` (all'apertura) e `triage.go:preparaDopoLaDecisione` (nuova RFQ, aggancio) |
| `LavoroInCorso`, `NuovoAlbero`, `LeggiGate`, `DifferenzeWorking`, `EtichettaStep`, `Step`, costanti `Step*` | `transport/web/fascicolo_pagina.go`, `fascicolo_bom.go`, `fascicolo_editor.go` |
| `WorkingBloccata` | `transport/web/allegati.go`, `fascicolo.go`, `fascicolo_editor.go` |
| `NotaInterna`, `RegistraCaricamento`, `Caricato` | `transport/web/caricamento.go` (carica a mano), `fascicolo_nas.go` (importa dal NAS) |
| `RevisioneProponibile` | `transport/workerapi/workerapi.go` (`propostaDaAnalisi`, `scriviProposta`); qui dentro `propostaDelDocumento` |
| `Ciclo`, `CreerebbeCiclo`, `Valuta`, `Differenze`, `ClassificaNodi`, `Pianifica`, `Rimozioni`, `RadiceDiFamiglia`, `PianoDelFascicolo`, `AggiornaRimozioni`, `SeBloccata`, `LeggiIngressoPiano` | solo questo package e le sue prove |

## Dati

**Il package non apre transazioni**: ogni funzione lavora sul `*db.Queries` di chi chiama, e qualunque
errore o `Rifiuto` annulla tutto. I gesti arrivano da `transport/web` (`gesto`, `inTransazione`), i fatti
da `transport/workerapi` nella transazione del risultato.

**Legge:** `thread_offerta`, `fase_log`, `bom_versione`, `componente`, `componente_relazione`,
`componente_proposta`, `relazione_proposta`, `rimozione_proposta`, `documento`, `documento_proposta`,
`deroga_struttura`, `identificativo_thread`, `cliente.regole`, `allegato`, `messaggio`, `analisi_fatti`,
`analizzatore_corrente`, `job` (`UltimoJobPerChiave`, `ListLavoroPendenteRfq`); le viste `v_step_prodotto`,
`v_fascicolo`, `v_thread_bloccanti` (dentro `GateCongelamento`), `v_codici_candidati_thread`; la funzione
`struttura_motivo_parziale` (`MotivoParziale`: la completezza di una lettura la dice il database, non il Go).

**Scrive:**

| Chi | Che cosa |
|---|---|
| `CongelaBom`, `ApriRevisione`, `AbbandonaBozza` | `bom_versione` (insert, congelamento, delete della bozza), `bom_versione_componente` / `_relazione` / `_documento` / `_deroga` (le istantanee), `fase_log` (chiude la riga aperta e ne apre una) |
| `ApplicaStruttura` | `componente_proposta`, `relazione_proposta` (solo le righe che cambiano), `documento_proposta` (`PropostaDocumentoDaRadice`: solo una proposta aperta, senza componente e con `fonte <> 'operatore'`), poi `AggiornaRimozioni` |
| `AggiornaRimozioni` | `rimozione_proposta` (upsert delle aperte, chiusura di quelle che non valgono più) |
| gesti di `decisioni.go` | `componente` (insert, ripristino), `componente_relazione` (insert, qta, delete per una rimozione), stato delle tre tabelle di proposta (la nota tagliata a 200 caratteri) |
| gesti di `modifiche.go` | `componente` (tipo, rev, descrizione), `componente_relazione`, `deroga_fabbisogno`; `rimozione_proposta` (`Collega` e `Sposta` chiudono la rimozione aperta sull'arco appena messo, `TieniArcoDellaRimozione`); `conversazione`, `messaggio`, `allegato` (il contenitore dei caricamenti interni) |
| gesti di `struttura.go` | `componente` (archiviazione, `step_strutturale_id`, delete), `componente_relazione` (archiviando), `deroga_struttura`, `documento.sostituito_da`, proposte chiuse con la nota (tagliata a 200 caratteri); archiviando un prodotto, le rimozioni aperte del suo STEP strutturale (`ChiudiRimozioniDiUnoStep`, nota «prodotto archiviato») |
| `AggiungiDaCodice` | `componente` (solo per un codice senza proposta STEP aperta) |
| `AssicuraProdottiDellaRichiesta` | `componente` (+ `RiconciliaProposteNodo` sulle proposte aperte con quel codice) |
| `ApplicaStrutturaVoluta` | tutto quello dei gesti che compone, più i codici scritti e le radici riconosciute su `componente_proposta`, `SetQtaRelazione`, `componente.tipo` (particolare → assieme) e `rimozione_proposta` «tenuto nella struttura confermata» |
| `RianalizzaRfq`, `PreparaFile` | `job` (`analizza_allegato`, `stage_allegato`, `estrai_archivio`, attraverso `platform/coda`), `allegato` (lo stato «in coda», dentro `coda.AccodaStage`) |

**Lucchetti.** `BloccaThread` (`thread_offerta FOR UPDATE`) mette in fila i gesti sulla stessa RFQ, e il
trigger della working bloccata prende la stessa riga `FOR KEY SHARE`. La RFQ si blocca **prima** di
componenti, documenti e proposte: con l'ordine rovesciato due transazioni si aspettano a vicenda e
PostgreSQL ne ferma una (40P01). Lo prendono da sé:
- le tre funzioni di `versioni.go`, `AssicuraProdottiDellaRichiesta` e `ArchiviaComponente`;
- `ScartaNodo`, `ScartaRelazione`, `ScartaRimozione` e `CodiceDelNodo` (solo il lucchetto, senza D26: non
  cambiano la working e valgono anche con la BOM congelata);
- tutto ciò che passa da `prepara` (`AccettaNodo`, `AccettaRelazione`, `AccettaSottoalbero`, `AccettaFile`,
  `AccettaRimozione`, `RipristinaComponente`, `RevocaDerogaStruttura`, `AggiungiDaCodice`, i sei gesti di
  `modifiche.go`, `ApplicaStrutturaVoluta`).

Gli altri gesti di `struttura.go` (`RimuoviComponente`, `ScegliStepStrutturale`, `ConcediDerogaStruttura`,
`Sostituisci`, `AnnullaSostituzione`) lo aspettano da chi chiama: `transport/web/fascicolo_rotte.go:preparaGesto`
(anche prima di `ArchiviaComponente` e `RevocaDerogaStruttura`, che lo riprendono senza danno), e
`preparaGesto` all'inizio di `transport/web/fascicolo.go:assegnaAlComponente`, di `correggiCodiceComponente`
e di `fascicolo_conferma.go:confermaFascicolo`; `allegati.go:confermaProposta` prende la RFQ `FOR KEY SHARE`
prima della proposta. Un 40P01 che arriva lo stesso diventa per l'operatore «riprova»
(`transport/web/fascicolo.go:spiegaErrore`). `ApplicaStruttura`, `RianalizzaRfq` e `PreparaFile` non lo
prendono. Lucchetti di riga: `BloccaComponente`, `BloccaComponenteProposta`, `BloccaRelazioneProposta`,
`BloccaRimozioneProposta`, `BloccaDocumento` (in ordine di id, `bloccaDocumenti`), `BloccaMessaggio` (il
contenitore dei caricamenti, prima di scegliere l'indice).

**Disco.** Solo lettura: `inStaging` (`os.Stat`) dice se il contenuto di un allegato è ancora nello staging
(`RianalizzaRfq`, `PreparaFile`, `LeggiIngressoPiano`). Nessuna scrittura su disco o sul NAS.

## Flussi principali

**Dai fatti di uno STEP alle proposte** (`ApplicaStruttura`, a ogni risultato dell'analisi, a ogni riuso
dei fatti, a ogni apertura della RFQ con `RianalizzaRfq`):
1. nessuna chiave `struttura` nei fatti o nessuno sha256 → niente; `MotivoParziale` dice se la lettura è
   completa;
2. il **portatore**: lo stesso contenuto arrivato due volte nella RFQ propone una volta sola
   (`PortatoreDelFile`);
3. `ClassificaNodi` con il `Motore` del cliente; un codice scritto dall'operatore sostituisce la
   classificazione;
4. la working (`leggiWorking` + `conCanonici` per i suffissi decorativi), gli identificativi della
   richiesta, il prodotto di cui il file è lo STEP strutturale (`ProdottoDelloStepStrutturale`);
5. `Pianifica` → upsert solo delle righe aperte che cambiano;
6. `propostaDelDocumento` (D16) e, se il file è uno STEP strutturale, `AggiornaRimozioni`.

**Le rimozioni** (`AggiornaRimozioni`): STEP strutturale scelto e corrente → esito `presente_analizzato` →
struttura leggibile con una radice sola → ogni nodo del file ha un componente, oppure un codice nuovo,
oppure è scartato; un nodo senza codice ancora aperto **sospende** il calcolo (`EsitoRimozioni.Sospese` dice
perché). Poi `Rimozioni` (gli archi della working raggiungibili dal prodotto che il file non contiene) →
upsert delle aperte, chiusura di quelle che non valgono più. Si ricalcolano dopo ogni decisione
(`dopoLaDecisione`, o `dopo` in `transport/web/fascicolo_rotte.go`). Le chiudono senza ricalcolo: un arco
appena messo a mano con `Collega` o `Sposta` (`tieniArcoMessoAMano`, come fa l'editor: lo STEP quell'arco
non ce l'ha, ed è per questo che una persona l'ha messo); un prodotto archiviato, le cui rimozioni nessun
ricalcolo guarderebbe più (`ListProdottiConStepStrutturale` elenca solo i prodotti attivi); un cambio di
STEP strutturale e la sostituzione di uno STEP, per quelle del file vecchio. Una rimozione chiusa così è
`scartata` e non si ripropone.

**Una decisione sulle proposte**: `prepara` → la proposta bloccata e controllata → `accettaNodo` (ritrova il
componente per codice, anche con il suffisso decorativo, e lo ripristina se archiviato; altrimenti lo crea)
o `accettaRelazione` (i due nodi decisi, niente archiviati, `CreerebbeCiclo`; quantità diversa solo se la
proposta l'ha fatta contro la quantità che la working ha ancora) → riconciliazione delle altre proposte uguali
→ `dopoLaDecisione`. `AccettaFile` / `AccettaSottoalbero`: prima i nodi in ampiezza, poi gli archi fra nodi
accettati, in una transazione sola; un rifiuto annulla tutto.

**L'editor della struttura** (`ApplicaStrutturaVoluta`): `prepara` → letture (working, proposte, codici
trovati se servono, le regole del cliente con `MotoreDellaRfq` per gli alias dei suffissi: un errore del
database qui ferma il gesto) → `pianifica`, che rifiuta prima di scrivere (riferimenti, codici, quantità, archi
ripetuti, archi non appesi alla radice, archi della working che l'editor non mostrava o che sono cambiati,
proposte decise nel frattempo, ciclo nel grafo finale) → 1. codici scritti, scarti (con gli archi che
toccano il nodo), radici dello STEP riconosciute come il prodotto → 2. componenti nuovi (da una proposta o
da un codice trovato; se la proposta ha cambiato codice nel frattempo, `componenteNato` rifiuta con
«riapri l'editor») e nodi ritrovati → 3. archi: prima si tolgono quelli mostrati e non voluti, poi le
quantità, poi si aggiungono (accettando la proposta uguale se c'è, altrimenti `collega`) → 4. proposte di
arco uguali a un arco voluto → `duplicato`, quelle mostrate e non volute → chiuse con il motivo → 5. un
particolare con figli diventa assieme → rimozioni ricalcolate → 6. le rimozioni sugli archi appena
confermati si chiudono «tenuto nella struttura confermata». Un componente che esce dall'albero porta con sé i
suoi figli; un componente entrato per codice (`k:`, o un nodo ritrovato) tiene i suoi archi com'erano.

**All'apertura del Fascicolo e alla nascita della RFQ** (`transport/web`): `AssicuraProdottiDellaRichiesta`
(ogni identificativo confermato da una persona diventa un componente radice `finito`, se nella RFQ non c'è
già un componente con quel codice, anche archiviato; con la BOM congelata niente) → `PreparaFile` (download degli allegati utili, archivi fermi all'estrazione, file fermi
all'analisi o alla rilettura dei fatti già calcolati; un job fallito con la stessa chiave non si riaccoda; un
allegato il cui messaggio non è in nessuna casella attiva va fra i `Saltati`, un altro errore di
`coda.CopiaPerDownload` ferma la preparazione) → `RianalizzaRfq` → `LeggiPianoFascicolo` (un errore del
database in `MotoreDellaRfq` ferma il piano: senza i suffissi del cliente proporrebbe «X» accanto a
«X_PRT»; regole scritte male non sono un errore, restano fuori). «Conferma Fascicolo» (in `transport/web/fascicolo_conferma.go`)
ricalcola il piano nella sua transazione e conferma solo se `Firma` è quella che l'operatore aveva davanti.

**Congelare e rivedere**: `CongelaBom` (RFQ bloccata → fase aperta → ultima versione: bozza da congelare,
oppure V1 con il contesto dalla fase → `LeggiGate` → istantanee → congelamento → per `preventivo`
`SCHEDA_COSTO` con la baseline). `ApriRevisione` vuole il motivo e la tabella di `contestoRevisione`.
`AbbandonaBozza` vuole `DifferenzeWorking` vuota.

**I codici della RFQ**: `CandidatiDellaRfq` → `Unisci` (nessuna riclassificazione: le evidenze vengono da
`v_codici_candidati_thread`) → per ogni codice la situazione (proposta STEP aperta, componente, archiviato,
codice della richiesta, nuovo) → `AggiungiDaCodice` rilegge tutto sotto il lucchetto e crea il componente solo
per un codice nuovo o della richiesta, con la revisione scelta fra quelle viste.

**Un caricamento interno**: `NotaInterna` (il messaggio del canale `nota` della RFQ, creato la prima volta
e poi ritrovato) → `RegistraCaricamento` (allegato di origine `manuale`, già nello staging). Da lì fa la
strada degli allegati; `RevisioneProponibile` impedisce che la sua proposta porti una revisione del cliente.

## Invarianti

- **Nessuna interpretazione scrive la working.** `ApplicaStruttura`, `Pianifica` e `AggiornaRimozioni`
  scrivono solo le tabelle di proposta e `documento_proposta`; `componente_relazione` la scrivono solo
  `accettaRelazione`, `collega`, `scollega`, `AccettaRimozione`, `ArchiviaComponente` e `ApplicaStrutturaVoluta`
  (con le stesse primitive, più `SetQtaRelazione`).
- **Una proposta decisa non si riscrive**: il Go la salta (`ApplicaStruttura`, `AggiornaRimozioni`) e le
  upsert hanno `WHERE stato = 'aperta'`. Un codice scritto dall'operatore resta anche se le regole cambiano
  (override in `ApplicaStruttura`, `CASE` in `UpsertComponenteProposta`), e D16 non riscrive la proposta
  di un documento con `fonte = 'operatore'` (il muro sta nella query `PropostaDocumentoDaRadice`).
- **Il codice di un componente non si riscrive per i suffissi decorativi**: gli alias si cercano
  (`proposte.go:Working.conCanonici`, `decisioni.go:componenteConSuffisso`, `voluta.go:conAlias`, la mappa
  `alias` di `piano.go:PianoDelFascicolo`, `candidati.go`). Senza le regole del cliente lette non si
  decide: ogni chiamante restituisce l'errore del database di `MotoreDellaRfq` invece di proseguire con un
  motore vuoto.
- **Un prodotto archiviato non tiene rimozioni aperte e non riceve un nuovo STEP strutturale**
  (`ArchiviaComponente` chiude quelle del suo STEP; `ScegliStepStrutturale` e `Sostituisci` con il nuovo
  riferimento lo rifiutano finché non è ripristinato; la sostituzione semplice resta possibile).
- **Una nota su una proposta sta nella colonna**: quelle che contengono un nome (un file fino a 300
  caratteri, il nome grezzo di un nodo) si tagliano a 200 caratteri, contati come rune (`tagliaNota`).
- **Un id che arriva da un indirizzo vale solo nella sua RFQ**: `RevocaDerogaStruttura` cancella con
  `deroga_struttura_id` **e** `thread_id` (come `RevocaDeroga`).
- **Dopo il congelamento la working non cambia**: `SeBloccata` lo dice prima, il trigger
  `bom_working_modificabile` (0020) è il muro.
- **Nessun ciclo entra da qui**: ogni nuovo arco passa da `CreerebbeCiclo` (`accettaRelazione`, `collega`) o
  dal controllo del grafo finale (`pianifica`); il gate rifiuta di congelare una working con un ciclo. Il
  database non li vieta.
- **Una mancanza in una lettura parziale non è un'informazione**: le rimozioni nascono solo dallo STEP
  strutturale con esito `presente_analizzato`, una quantità diversa si propone solo da una lettura completa.
- **Revisioni discordanti non si scelgono per punteggio** (`Unisci`, `revisioneScelta`); un file caricato a
  mano non propone la revisione del cliente (`RevisioneProponibile`).
- **Un componente con storia non si cancella**: `RimuoviComponente` riesce solo se nessuna FK lo tiene;
  altrimenti si archivia (e archiviare conserva documenti, proposte, deroghe).
- **Il piano è puro e firmato**: `PianoDelFascicolo` non scrive; stessa entrata, stessa `Firma`.
- **L'albero è deterministico**: `NuovoAlbero` dà gli stessi `Totali` in qualunque ordine arrivino
  componenti e archi, anche con un ciclo nella working.
- **Un identificativo senza `confermato_da` non diventa un componente** (`AssicuraProdottiDellaRichiesta`).

## Dipendenze

Importa `core/inbox/classificazione` (il `Motore`, `CodiceAmmissibile`, `RevAmmissibile`, `PropostaDaNome`,
`DiFamiglia`), `core/registro/regole` (`LeggiRegole`), `platform/db`, `platform/coda` (accodare analisi,
download ed estrazioni), `platform/contratti/worker` (`StrutturaSTEP`, `DecodificaStruttura`,
`PayloadEstraiArchivio`), `platform/storage/staging` (`FileStaging` per `coda.AccodaStage`).
È importato da `transport/web` e `transport/workerapi`. Tutto dentro la tabella di `internal/README.md`
(`core/*` → `core/*`, `platform`).

## Test

| Livello | File | Che cosa |
|---|---|---|
| L1 | `gate_test.go` | ogni condizione del gate, `Ciclo`, `EtichettaStep`, `Differenze` |
| L1 | `proposte_test.go` | `ClassificaNodi` (famiglia, generico, id contro nome, rev del file, misure), `Pianifica` (tipi, duplicati, radice dichiarata, qta da lettura troncata), `Rimozioni`, `CreerebbeCiclo`, `RadiceDiFamiglia` |
| L1 | `candidati_test.go` | `Unisci`: forte/debole, riferimento escluso, storia citata, conflitto di revisioni, situazioni, BOM congelata, ordine delle righe, suffisso decorativo |
| L1 | `albero_test.go` | `NuovoAlbero`: radici multiple, condiviso, archiviati, ciclo; i `Totali` con un ciclo uguali su 50 ordini a caso dei dati |
| L1 | `piano_test.go` | `PianoDelFascicolo`: zip, lavoro in corso, ambiguità, fogli insieme, stesso contenuto, nodo senza codice, BOM congelata, STEP strutturale suggerito, `Firma`, suffissi |
| L1 | `voluta_test.go` | `pianifica` (nodi proposti, radice, quantità, perimetro e i due limiti con il loro messaggio, cicli, scarti, dati cambiati, una carta per codice), `fraseVoluta`, `tipoVoluto` |
| L1 | `note_test.go` | `tagliaNota`: 200 caratteri contati come rune |
| L4 | `fascicolo_db_test.go` | congelamento, fasi e baseline, revisioni e abbandono, sostituzioni, STEP strutturale, deroghe, archiviazione |
| L4 | `proposte_db_test.go` | fan-out, rimozioni (troncato, scarti del parser, più radici, fatti v2, nodo senza codice), accettazioni, cicli, rianalisi, riclassificazione, D16, BOM congelata |
| L4 | `candidati_db_test.go` | la vista dei candidati e `AggiungiDaCodice` |
| L4 | `preparazione_db_test.go` | prodotti della richiesta, download, file fermi, lavoro in corso, piano dal database |
| L4 | `suffissi_db_test.go` | i suffissi decorativi nei nodi, nei componenti già nati, in D16, nel piano |
| L4 | `nato_db_test.go` | `componenteNato`: un codice con cui non è nato niente è un `Rifiuto`, non un `no rows` grezzo |
| L4 | `revisione_db_test.go` | archiviare un prodotto chiude le rimozioni del suo STEP; nota lunga che non ferma la sostituzione; STEP strutturale rifiutato su un archiviato (anche con la sostituzione); scarti e codice del nodo che aspettano la RFQ bloccata; deroga strutturale revocata solo dalla sua RFQ; arco messo a mano non proposto da togliere; D16 che non riscrive una decisione dell'operatore |

I test L4 hanno `//go:build integrazione` e vogliono `COCKPIT_TEST_DSN`. `ApplicaStrutturaVoluta`, i gesti di
`modifiche.go` e le rotte del Fascicolo sono provati in L4 da `transport/web` (`v3_db_test.go`,
`b85_db_test.go`, `b86_db_test.go`, `b87_db_test.go`, `b87b_db_test.go`, `a4_db_test.go`; `gesti_db_test.go`
per il codice scritto a mano e l'ordine dei lucchetti, L1 `gesti_test.go` per la quantità a 32 bit, il
`Rifiuto` condiviso e il 40P01), e nel browser (L7) da `fascicolo_browser_test.go` e
`fascicolo_v3_browser_test.go`.

## Stato dell'implementazione

- **Completo**: versioni, gate, differenza, archiviazione, sostituzioni e deroghe (A4, B8.A4a); proposte,
  rimozioni, decisioni e rianalisi (B8.5); codici della RFQ (B8.6); albero e correzioni a mano (B8.7);
  preparazione e piano (B8.7b); editor della struttura e suffissi decorativi (Fascicolo v3).
- **Aperto per decisione**: il gate non controlla «zero proposte di sostituzione pendenti» (A4.6), perché il
  modello non registra una proposta di sostituzione (commento in testa a `gate.go`).
- **Dal Fascicolo v3 la struttura di uno STEP non entra più con «Conferma Fascicolo»**: `struttureDi` mette
  ogni struttura con nodi o archi aperti in `decidere` (`DomandaStrutturaEditor`), quindi la mappa `inArrivo`
  resta vuota e un file che va a un nodo proposto aspetta l'editor (`DaStep` con `DomandaComponente`). Il
  ramo «struttura pronta» del piano e il suo uso in `transport/web/fascicolo_conferma.go` non scattano più.
- **Limiti noti**:
  - `AssicuraProdottiDellaRichiesta` gira a ogni apertura e guarda solo `identificativo_thread`: un prodotto
    della richiesta tolto con `RimuoviComponente`, o il cui codice è stato corretto
    (`transport/web/fascicolo.go:correggiCodiceComponente` non aggiorna l'identificativo), rinasce con il
    codice della richiesta come radice `finito` senza figli. Un prodotto archiviato invece non rinasce.
  - Sempre lì, il componente esistente si cerca solo per codice esatto (`GetComponentePerCodice`), senza gli
    alias dei suffissi decorativi: con «X_PRT» nato prima della regola, il codice della richiesta «X» fa
    nascere un secondo componente.
  - `piano.go:struttureDi` riconosce un nodo archiviato e una quantità diversa dal testo della nota della
    proposta («archiviato», «qta diversa»), non da un campo.
- Lo spostamento sul NAS di un documento già scritto (B8.8) non passa da qui.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| capire perché una BOM non si congela | `gate.go:Valuta`, poi `GateCongelamento` e `ListGateStep` in `platform/db/queries/bom.sql` |
| capire perché una revisione non si apre o non si abbandona | `versioni.go:contestoRevisione`, `AbbandonaBozza` |
| capire perché un nodo di uno STEP ha (o non ha) un codice | `classifica.go:ClassificaNodi`, `primo`, `alternativo` |
| capire perché un nodo è `duplicato` o una quantità non è proposta | `proposte.go:Pianifica`, `Contesto.componenteDi` |
| capire perché una rimozione non è stata proposta | `rimozioni.go:AggiornaRimozioni` (`EsitoRimozioni.Sospese`) |
| cambiare che cosa succede quando si accetta un nodo o un arco | `decisioni.go:accettaNodo`, `accettaRelazione` |
| capire perché l'editor rifiuta una struttura | `voluta.go:pianifica` |
| cambiare l'ordine in cui l'editor scrive | `voluta.go:ApplicaStrutturaVoluta` (passi 1–6) |
| capire perché un file è `decidere` o `attesa` nel piano | `piano.go:voceFile`, `fratelli`, `struttureDi` |
| capire perché un codice sta fra gli altri riferimenti, o quale gesto offre | `candidati.go:forte`, `classe`, `situa` |
| capire perché un file non si scarica o non si analizza da solo | `preparazione.go:PreparaFile`, `giaProvato` |
| un gesto nuovo sulla working | `modifiche.go` (con `prepara` e `dopoLaDecisione`), poi la rotta in `transport/web/fascicolo_rotte.go` |
| capire perché una rimozione si è chiusa da sola | `modifiche.go:tieniArcoMessoAMano`, `struttura.go:ArchiviaComponente`, `ScegliStepStrutturale`, `chiudiProposteDi` |
| capire perché i totali dell'albero sono quelli | `albero.go:totali`, `inUnCiclo` |
| capire un 40P01 o un gesto che aspetta | il paragrafo «Lucchetti» qui sopra; `transport/web/fascicolo_rotte.go:preparaGesto`, `fascicolo.go:spiegaErrore` |
| un esito nuovo dello STEP del prodotto | `v_step_prodotto` in migrazione, costanti `Step*` e `EtichettaStep` in `gate.go`, `Valuta` |

## Leggi anche

`internal/core/README.md` (il posto del package fra gli altri), `internal/README.md` (architettura e
dipendenze), `internal/transport/README.md` (le rotte del Fascicolo), `internal/core/rfq/documenti` (i nomi
sul NAS e la copia), `migrations/0018_fascicolo.sql`, `0020_bom_versioni.sql`, `0021_annotazioni_pdf.sql`,
le query in `internal/platform/db/queries/` (`proposte.sql`, `struttura.sql`, `bom.sql`, `editor.sql`,
`candidati.sql`, `preparazione.sql`, `schermata.sql`), e in `cockpit/README.md` la sezione «Il Fascicolo v3».
