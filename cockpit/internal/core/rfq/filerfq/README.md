---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/rfq/filerfq` — il ciclo dei file della Sorgente

## Scopo

- Lo stato di un'occorrenza (RFQ, allegato) nella Distinta unica (V1, EM §10): **attiva**, **in Altri file**, **scartata**.
  - La RFQ e' sempre `messaggio.thread_id`; nessuna riga in `file_rfq` = attiva, versione 0.
  - Il predicato SQL gemello e' `file_rfq_attivo` (migrazione 0023); qui `Attivo`.
- I tre gesti con chiave d'intento della Sorgente e i loro protocolli:
  - **«Scarta»** (S2): in due tempi; il tempo 1 e' la decisione (stato, evento, scollegamento chiesto, proposta chiusa, download ed
    estrazioni pendenti annullati, riga della rimozione, copie di conservazione da rilasciare); il tempo 2 e' la rimozione dallo
    staging, tracciata e idempotente (`EseguiRimozione`);
  - **«Metti in Altri file»** (S3): prima la copia verificata nel posto degli Altri file (il `Conservatore`), poi lo stato (fase A,
    copia con il battito fuori transazione, registrazione della prova, fase C);
  - **«Riattiva»** (S4): i byte rimessi nello staging sotto `BloccaContenuto`, lo stato attivo, la strada d'ingresso rifatta, la copia
    da rilasciare.
- Le dipendenze da controllare prima dei gesti (D-1…D-15, D-6b, D-7b) con la loro impronta, il perimetro di un archivio.
- Il modello di S1 (i file della RFQ, i contatori, il dettaglio con le dipendenze e la storia), le sorgenti dei byte di S5.
- Il custode: le rimozioni e i rilasci da riprendere, le copie con il battito fermo, la bonifica giornaliera, i parziali delle copie.
- La registrazione dei file caricati (E1′ e il ramo HTML di «carica»): i doppioni sotto il lucchetto del contenitore, l'evento
  `ricaricato_dopo_scarto`.
- Nome del package [T] della corsia file: l'EM nomina «il servizio del ciclo dei file» senza un package; `filerfq` come le tabelle.

## Non appartiene qui

- **Le rotte, la busta, i permessi, i byte serviti al browser:** `transport/web/sorgente.go`, `sorgente_carica.go`.
- **La strada d'ingresso di un file** (proposta dal nome, analisi, estrazione): `transport/workerapi` (`DopoCaricamento`), passata
  come `Pipeline`.
- **La cache dello staging e il suo custode:** `platform/storage/staging` (`Cache`); qui solo `CopiaNelDeposito` che gli dice se il
  file di una copia verificata c'e'.
- **Le operazioni sui file delle copie** (parziale, promozione senza sostituzione, lettura con lo sha verificato, rimozione con la
  rinomina): `platform/storage/nas` (`nas.Conservatore`), l'unico scrittore del NAS. Qui l'interfaccia `Conservatore` e il protocollo
  con il database.
- **Il deposito locale `[conservazione]`** delle prove dell'ondata B non esiste piu' (U2 §3.13): `platform/config` ferma l'avvio se
  la sezione compare.
- **I contesti partner:** li passa chi li ha (`app/runtime/ciclo_file.go`), il package non legge file di configurazione.
- **Le guardie negli scrittori legacy** (download, analisi, estrazione, gesti documentali): stanno dove scrivono
  (`transport/workerapi`, `transport/web`, `core/rfq/documenti`, `platform/coda`), con il predicato.

## File

- **`filerfq.go`** — il `Servizio` (pool, staging, `Conservatore`, NAS in sola lettura, `Pipeline`, contesti, orologio, tempo massimo del
  tentativo, eta' minima), gli `Agganci` delle prove di concorrenza, gli stati, i gesti, le origini, `Attivo`, le attese crescenti.
- **`errori.go`** — `Rifiuto` (stato HTTP, codice, testo, campi in piu'), `ComeRifiuto`, `conservazioneNonDisponibile` (503
  `NON_CONFIGURATO` con il motivo `conservazione` | `nas` | `nas_scrittura`), `TestoNonDisponibile`.
- **`conservatore.go`** (V1, ondata C) — l'interfaccia `Conservatore` (U2 §3.13: `Disponibile`, `RadiceID`, `Copia`, `Leggi`, `Apri`,
  `Stato`, `Rimuovi`, `PulisciPartiCopie`, `Cartelle`), implementata da `nas.Conservatore`; `postoDi`/`postoDellaRiga` (il posto
  di una riga: la radice, la cartella della RFQ fotografata in `cartella_thread`, il percorso `ALTRO\<nome>`), `CopiaNelDeposito`
  per la cache.
- **`conservatoreprova/`** — il doppio delle prove (`Locale`, il ConservatoreLocale di U2): `nas.Conservatore` su una cartella
  temporanea, con `File`, `Parti`, `NomiDiServizio`, `Percorso`. Solo nelle prove (prova statica).
- **`corpo.go`** — `CorpoGesto`, `LeggiCorpo` (JSON stretto, 64 KiB, chiave UUID v4, motivo in NFC, nessun campo percorso),
  `SenzaPercorsi`.
- **`intento.go`** — `EsitoGesto`, l'intento (`esito_intento`: stessa chiave e stesso corpo = riproposto; un'altra RFQ = 422
  FUORI_RFQ; altrimenti 409 CHIAVE_RIUSATA), gli stati correnti di rimozioni e copie.
- **`occorrenze.go`** — `Occorrenza`, il perimetro di un archivio (`Perimetro`, `NVoci`, `Impronta`), il 409 `voci_archivio`.
- **`dipendenze.go`** — `Dipendenza`, `ElencoDipendenze` (un elenco solo per S1 e per i gesti), `controllaDipendenze` (blocchi,
  risoluzioni, impronta), `unicheCopie` (D-14 in blocco).
- **`gesti.go`** — `prepara` (l'ordine fisso dei controlli sotto `BloccaThread`), la fotografia, lo stato, gli eventi, lo scollegamento.
- **`scarta.go`** — S2, il tempo 1, il tentativo dopo il COMMIT, la risposta ricomposta.
- **`rimozione.go`** — `EseguiRimozione` (i passi 1-10 del tempo 2 in una transazione, `os.Remove` prima del COMMIT).
- **`altrifile.go`** — S3: il 503 con la scrittura spenta prima di tutto; fase A (sotto `BloccaCartella(thread, «ALTRO»)` il nome
  riservato `ALTRO\<nome>` con il progressivo, `scegliNome`; l'eredita' di una copia trattenuta, U2 §3.8); la copia con il
  `Conservatore` fuori transazione, con il parziale del token della riga (una sorgente dopo l'altra: staging, altra occorrenza,
  altra copia letta dal `Conservatore`, documento sul NAS; la cartella della RFQ che manca la crea `CreaCartellaThread`); la
  registrazione della prova (`registra`: `creato_dal_cockpit`, anche su una riga gia' chiusa da uno «Scarta»); il battito in
  `ultimo_battito`; fase C.
- **`riattiva.go`** — S4.
- **`rilascio.go`** — `RilasciaCopia` (U2 §3.7, fuori transazione): passo 1 in una transazione breve (la copia in volo aspetta;
  base B di U2-Q01: le cause diverse dallo scarto vanno a `trattenuta`, `trattieni`; scrittura spenta, radice, cartella, prova;
  il percorso di un documento; poi `in_rimozione` con il token della colonna `token_rimozione`); passo 2 `Conservatore.Rimuovi`;
  passo 3 l'esito in `esito_rimozione` e nell'evento. `TestoRimozioneNAS` (i testi di U2 §3.7).
- **`riprova.go`** — «Riprova la rimozione dalla cartella ALTRO» (U2 §3.5).
- **`riestrazione.go`** — `ArchivioNelloStaging`: i byte di un archivio in Altri file dalla sua copia in ALTRO (U2-P45).
- **`custode.go`** — `Custode` (`Avvia`, `Giro`, `Bonifica`): ogni passata prende TUTTE le righe scadute (rimozioni, copie,
  errori non ritentabili per la bonifica), una pagina di `Limite` righe alla volta, per chiave; toglie i parziali delle copie piu'
  vecchi di 24 ore nelle cartelle delle righe recenti (`ListConservazioniRecenti`) e quelli dello staging del ciclo dei file rimasti
  da un processo morto (`PulisciPartiCiclo`).
- **`modello.go`** — S1: `Modello`, `TestoRimozione` (i testi esatti degli stati della rimozione), le azioni sempre presenti con il
  motivo (`Azione`; `AzioneContenuto` con `richiede[]` sempre presente). `byte.staging` guarda il disco solo nel posto del
  contenuto; `byte.conservazione` dice lo stato registrato senza toccare il posto delle copie (U2 §3.12); l'apertura e' disponibile
  solo per le sorgenti che S5 serve (staging `EContenuto`, copia, NAS scritto); `configurazione.conservazione` e
  `motivo_conservazione` dicono se «Metti in Altri file» si puo' fare; gli esiti degli eventi passano da `SenzaPercorsi`.
- **`contenuto.go`** — `OccorrenzaPerContenuto`, `ApriCopia` (la copia di conservazione dello stesso allegato, anche di un'altra RFQ,
  con lo sha verificato dallo stesso handle; per un file in Altri file il motivo `COPIA_NAS_MANCANTE` o `COPIA_NAS_CAMBIATA`).
- **`caricamento.go`** — `ScriviParziale` (il parziale `<staging>\_parti\ciclo.<uuid>`, `PrefissoParteCiclo`: non ha la forma dei
  tentativi dei worker, e la pulizia della coda non lo tocca mentre la richiesta arriva), `PulisciPartiCiclo`, `RegistraFile` (la
  risposta usa per ogni file l'origine della richiesta, `portale` | `interna`), `CercaDoppione`, `SegnaRicaricato`,
  `SegnaAnomaliaRiestrazione`.

## Entry point

- **`Servizio.Modello`, `Scarta`, `MettiInAltriFile`, `Riattiva`, `OccorrenzaPerContenuto`, `ApriCopia`** — `transport/web/sorgente.go`
- **`ScriviParziale`, `Servizio.RegistraFile`** — `transport/web/sorgente_carica.go` (E1′)
- **`CercaDoppione`, `SegnaRicaricato`** — `transport/web/caricamento_rfq.go` (il ramo HTML di «carica», origine `legacy`)
- **`SegnaAnomaliaRiestrazione`** — `transport/workerapi/archivi.go` (G-7)
- **`Custode.Avvia`, `CopiaNelDeposito`** — `app/runtime` (`ciclo_file.go`, `servizi.go`); il `Conservatore` di produzione
  (`nas.Conservatore` sulla radice del NAS, con la capacita' `nas_scrittura` riletta a ogni gesto) e' cablato li'
  (`conservatoreNAS`; prova statica `TestNessunServizioConConservatoreInProduzione`)
- **`Servizio.RiprovaRimozione`** — `transport/web/sorgente.go` (POST `.../riprova-rimozione`)
- **`ArchivioNelloStaging`** — `transport/workerapi/archivi.go` (G-7, `Server.ArchiviInAltriFile`)

## Dati

### Scrive (solo qui: prova statica CO-12)

- `file_rfq`, `file_rfq_evento`, `file_rfq_rimozione`, `file_rfq_conservazione`, `esito_intento` (gesti `scarta_file`,
  `altri_file`, `riattiva_file`).
- Nel tempo 1 e nella fase C: `documento_proposta` (chiusa, o scollegata), `associazione_evento` (`scarta`, `sgancia`), `job`
  (download ed estrazioni pendenti annullati, SKIP LOCKED).
- `allegato.path_staging` (tempo 2, «Riattiva») sempre dopo `BloccaContenuto` (prova statica CO-13).

### Legge

- `allegato`, `messaggio`, `documento_proposta`, `documento`, `documento_provenienza`, `componente`, `componente_proposta`, `job`,
  `annotazione_pdf`, `nas_anomalia`, `fonte_step_evento`, `conferma_strutturale`, le bozze aperte (D-6), `utente` (le sigle).

### Lucchetti

- La RFQ (`BloccaThread`), poi le righe (le proposte prima degli allegati `FOR SHARE`, le rimozioni e le copie `FOR UPDATE`), per
  ultimo il contenuto (`BloccaContenuto`, piu' sha in ordine crescente).
- Le proposte dell'occorrenza e di tutte le sue voci si prendono in `prepara`, PRIMA di leggere gli stati e di calcolare le
  dipendenze, e l'occorrenza si rilegge dopo: un gesto legacy che tiene una proposta (la conferma, «Documentazione generale», il
  vecchio «Scarta il file») finisce prima, e il gesto vede quello che ha scritto. Controllo e azione stanno sotto gli stessi
  lucchetti.
- Le copie (`ListConservazioniVive` nella fase A di S3, `GetConservazionePerAggiornare` nella fase C di S4) si prendono prima del
  contenuto. **Eccezione dichiarata:** l'allegato di cui si scrive `path_staging` si prende DOPO il contenuto, nella transazione che
  lo verifica (EM §10.6, «chi adotta»): lo fanno tutti gli scrittori di `path_staging` (il risultato dello stage, il riuso del
  gemello, il caricamento, la cache, il tempo 2, la «Riattiva»). Prenderlo prima incrocerebbe chi tiene il contenuto e aspetta
  l'allegato.
- Chi cancella ricontrolla tutto sotto il lucchetto per contenuto; chi adotta verifica il contenuto sotto il lucchetto, nella
  transazione che scrive `path_staging` (EM §10.6).
- La registrazione di un file caricato (`RegistraFile`, E1′; giro G39-bis) prende la RFQ `FOR UPDATE` per prima, dentro
  `fascicolo.NotaPortale` / `NotaInterna`, poi il messaggio della nota, il contenuto e le righe nuove: lo stesso ordine di
  `EstraiArchivio` e dei risultati del worker (`transport/workerapi/lucchetti.go`). Prima teneva il messaggio della nota e
  chiedeva la RFQ dopo, e con l'estrazione di un archivio della stessa nota era un 40P01 (prova d1). Un 40P01 o un 40001
  rimasti rifanno la transazione del file da capo, al piu' `maxTentativiRegistrazione` volte, prima del 5xx «da verificare».
- Il tempo 2 calcola i pin con `PinDelFile` (i pin del file) e con la regola archivio → voci della cache (`staging.VociDiArchivi`,
  `staging.PinDiContenuti`): un archivio resta finche' una sua voce, per sha, ha un pin.

### Lo scarto col gesto precedente

- Proposta scartata da una persona, occorrenza attiva per il ciclo dei file (nessuna riga o la riga in attivo dopo una
  «Riattiva») e nessun evento `scarta` del ciclo per l'allegato, in nessuna RFQ. Per il ciclo dei file vale scartato
  (`StatoCiclo`): solo «Scarta»; E1′ non lo conta come doppione. Per il predicato `file_rfq_attivo` resta attivo.

### Disco

- Lo staging: solo il posto del contenuto (`staging.EContenuto`); le copie degli Altri file solo attraverso il `Conservatore` (il
  contenimento e la rinomina senza sostituzione stanno in `storage/nas`; nessun `os.Rename`: prova statica); il NAS dei documenti
  in sola lettura (una sorgente dei byte).
- Nessun percorso esce verso il browser (stati e identificativi; `SenzaPercorsi` per i testi liberi: il percorso intero, anche con
  gli spazi, fino a «: », a una virgoletta o alla fine).

## Prove

- L1: `filerfq_test.go` (corpo, impronte, perimetro, ordine dei controlli, testi, attese, la verifica della cache, nessun percorso
  nel modello), `statiche_test.go` (CO-12, CO-13, PP-04), `conservatore_statiche_test.go` (il doppio solo nelle prove, nessun
  `Conservatore` impostato in produzione prima della 0023, nessun `os.Rename`, il deposito locale tolto), `rilievi_test.go` (i
  rilievi della revisione senza DB: `SenzaPercorsi` con gli spazi, gli esiti degli eventi, S1 allineata a S5, i parziali del ciclo
  dei file).
- L4 (`-tags integrazione`, DB di prova, staging e conservatore delle prove in cartelle temporanee): `banco_db_test.go` (il banco), `altrifile_db_test.go`
  (AF-01…AF-16; AF-15 e' la radice cambiata), `scarta_db_test.go` (SC-01…SC-21), `concorrenza_db_test.go` (CO-01…CO-17, CO-07b, CO-08b, CO-14b),
  `rilievi_db_test.go` (i rilievi della revisione: la bonifica oltre la prima pagina, il custode a lotti, la conferma legacy durante
  lo «Scarta», lo scarto legacy durante la fase C e dopo una «Riattiva», il tentativo chiuso su un archivio nella fase C e nella
  copia, il parziale di E1′ e la pulizia della coda, la regola archivio → voci del tempo 2, G-15).
- Gli agganci delle prove (`Agganci`): `PrimaDiCancellare`, `DopoLaCancellazione`, `DopoLaCopia`, `PrimaDiCopiare`,
  `PrimaDellaFaseC`, `SaltaTentativi`; nil in produzione.
- Le rotte (PP, R-20, R-21, R-30, AF-08, AF-10, CO-04) stanno in `transport/web/sorgente_db_test.go`; il worker (AF-07, CO-01, CO-02,
  CO-03, M-33) in `transport/workerapi/sorgente_db_test.go`.

## Stato

- Ondata B di V1, corsia file (PM23 §14 passo 5). La pagina che usa le rotte e' della corsia interfaccia; la bozza (R2, `NonAttivi`,
  D-6 lato persistenza) dell'ondata C.
- Ondata C (U2 §3): il deposito locale e' tolto; le copie passano dal `Conservatore` (promozione senza sostituzione fuori
  transazione, prova registrata, rimozione con la rinomina, letture con lo sha verificato, S1 senza accessi al disco delle copie,
  S5 con `COPIA_NAS_MANCANTE` e `COPIA_NAS_CAMBIATA`, «Riattiva» con i byte da una parte sola).
- Direttiva 35, blocco C-6a: sulle colonne della 0023 la cartella ALTRO della RFQ come posto (cartella della RFQ e percorso
  `ALTRO\<nome>` nella riga), la riserva dei nomi con i documenti (`PercorsoOccupato`), `creato_dal_cockpit` come prova,
  `token`, `token_rimozione`, `ultimo_battito`, `esito_rimozione`, gli stati `in_rimozione`, `trattenuta`, `ceduta` (la base B
  di U2-Q01, provvisoria [T]: sul NAS si toglie solo con lo scarto), i parziali con i token uniti, «Riprova la rimozione», la
  riestrazione di un archivio in Altri file, PF-27, D-15b, il cablaggio di `nas.Conservatore` in `app/runtime`.
