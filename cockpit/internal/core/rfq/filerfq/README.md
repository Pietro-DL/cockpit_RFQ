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
  - **«Metti in Altri file»** (S3): prima la copia verificata nel deposito, poi lo stato (fase A, copia con il battito, fase C);
  - **«Riattiva»** (S4): i byte rimessi nello staging sotto `BloccaContenuto`, lo stato attivo, la strada d'ingresso rifatta, la copia
    da rilasciare.
- Le dipendenze da controllare prima dei gesti (D-1…D-15, D-6b, D-7b) con la loro impronta, il perimetro di un archivio.
- Il modello di S1 (i file della RFQ, i contatori, il dettaglio con le dipendenze e la storia), le sorgenti dei byte di S5.
- Il custode: le rimozioni e i rilasci da riprendere, le copie con il battito fermo, la bonifica giornaliera, i parziali del deposito.
- La registrazione dei file caricati (E1′ e il ramo HTML di «carica»): i doppioni sotto il lucchetto del contenitore, l'evento
  `ricaricato_dopo_scarto`.
- Nome del package [T] della corsia file: l'EM nomina «il servizio del ciclo dei file» senza un package; `filerfq` come le tabelle.

## Non appartiene qui

- **Le rotte, la busta, i permessi, i byte serviti al browser:** `transport/web/sorgente.go`, `sorgente_carica.go`.
- **La strada d'ingresso di un file** (proposta dal nome, analisi, estrazione): `transport/workerapi` (`DopoCaricamento`), passata
  come `Pipeline`.
- **La cache dello staging e il suo custode:** `platform/storage/staging` (`Cache`); qui solo `CopiaNelDeposito` che gli dice se il
  file di una copia verificata c'e'.
- **La configurazione del deposito** (`[conservazione].radice`, i controlli dei percorsi): `platform/config`.
- **I contesti partner:** li passa chi li ha (`app/runtime/ciclo_file.go`), il package non legge file di configurazione.
- **Le guardie negli scrittori legacy** (download, analisi, estrazione, gesti documentali): stanno dove scrivono
  (`transport/workerapi`, `transport/web`, `core/rfq/documenti`, `platform/coda`), con il predicato.

## File

- **`filerfq.go`** — il `Servizio` (pool, staging, deposito, NAS in sola lettura, `Pipeline`, contesti, orologio, tempo massimo del
  tentativo, eta' minima), gli `Agganci` delle prove di concorrenza, gli stati, i gesti, le origini, `Attivo`, le attese crescenti.
- **`errori.go`** — `Rifiuto` (stato HTTP, codice, testo, campi in piu'), `ComeRifiuto`, `ErrConservazioneNonConfigurata`.
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
- **`altrifile.go`** — S3: fase A, copia (parziale, sha calcolato scrivendo, rinomina con la riga presa), battito, fase C.
- **`riattiva.go`** — S4.
- **`rilascio.go`** — `RilasciaCopia` (i byte raggiungibili altrove verificati sotto il lucchetto, poi il file tolto).
- **`deposito.go`** — `PreparaDeposito` (il segnaposto `.cockpit-deposito`, l'avvio che si ferma se le copie sono altrove),
  `PercorsoCopia` (contenimento), `CopiaPresente`, `CopiaNelDeposito`.
- **`custode.go`** — `Custode` (`Avvia`, `Giro`, `Bonifica`).
- **`modello.go`** — S1: `Modello`, `TestoRimozione` (i testi esatti degli stati della rimozione), le azioni sempre presenti con il
  motivo.
- **`contenuto.go`** — `OccorrenzaPerContenuto`, `ApriCopia` (la copia di conservazione dello stesso allegato, anche di un'altra RFQ).
- **`caricamento.go`** — `ScriviParziale`, `RegistraFile`, `CercaDoppione`, `SegnaRicaricato`, `SegnaAnomaliaRiestrazione`.

## Entry point

- **`Servizio.Modello`, `Scarta`, `MettiInAltriFile`, `Riattiva`, `OccorrenzaPerContenuto`, `ApriCopia`** — `transport/web/sorgente.go`
- **`ScriviParziale`, `Servizio.RegistraFile`** — `transport/web/sorgente_carica.go` (E1′)
- **`CercaDoppione`, `SegnaRicaricato`** — `transport/web/caricamento_rfq.go` (il ramo HTML di «carica», origine `legacy`)
- **`SegnaAnomaliaRiestrazione`** — `transport/workerapi/archivi.go` (G-7)
- **`PreparaDeposito`, `Custode.Avvia`, `CopiaNelDeposito`** — `app/runtime` (`ciclo_file.go`, `servizi.go`)

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
- Chi cancella ricontrolla tutto sotto il lucchetto per contenuto; chi adotta verifica il contenuto sotto il lucchetto, nella
  transazione che scrive `path_staging` (EM §10.6).

### Disco

- Lo staging: solo il posto del contenuto (`staging.EContenuto`); il deposito: solo dentro la radice (`PercorsoCopia`); il NAS in
  sola lettura (una sorgente dei byte).
- Nessun percorso esce verso il browser (stati e identificativi; `SenzaPercorsi` per i testi liberi).

## Prove

- L1: `filerfq_test.go` (corpo, impronte, perimetro, ordine dei controlli, testi, attese, contenimento, segnaposto, nessun percorso
  nel modello), `statiche_test.go` (CO-12, CO-13, PP-04).
- L4 (`-tags integrazione`, DB di prova, staging e deposito temporanei): `banco_db_test.go` (il banco), `altrifile_db_test.go`
  (AF-01…AF-16), `scarta_db_test.go` (SC-01…SC-21), `concorrenza_db_test.go` (CO-01…CO-17, CO-07b, CO-08b, CO-14b).
- Le rotte (PP, R-20, R-21, R-30, AF-08, AF-10, CO-04) stanno in `transport/web/sorgente_db_test.go`; il worker (AF-07, CO-01, CO-02,
  CO-03, M-33) in `transport/workerapi/sorgente_db_test.go`.

## Stato

- Ondata B di V1, corsia file (PM23 §14 passo 5). La pagina che usa le rotte e' della corsia interfaccia; la bozza (R2, `NonAttivi`,
  D-6 lato persistenza) dell'ondata C.
