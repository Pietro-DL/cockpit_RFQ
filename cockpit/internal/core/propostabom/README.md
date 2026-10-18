# `internal/core/propostabom` — dalla proposta del Motore A alla bozza della Distinta unica

## Scopo

- Il passaggio dalla proposta del Motore A alla bozza formato 3 della Distinta unica (V1: emendamento di V1 §3.8,
  §3.8.1, §3.8.2, §6.1, §6.4.1, §6.5, §6.6, §9.9, §10.9; piano della 0023 §14 passo 3). È puro: legge l'esito già
  calcolato della valutazione, non legge il DB, non scrive e non decide.
- Che cosa c'è:
  - **la chiave dei gruppi di un prodotto** (`GruppoBOM`, `GruppoCandidata`, `GruppoSenzaSTEP`, `ChiaveCandidata`,
    `GruppiDelProdotto`): «bom», «cand:<allegato_id>:<radice>», «senza», con l'allegato, lo sha256 e la radice della
    struttura del gruppo. `Indice.Gruppi` della pagina la chiama e aggiunge solo i dati della pagina. Una chiave non si
    scompone mai;
  - **il gruppo corrente della sorgente** di una bozza (`GruppoCorrenteDellaSorgente`, `GruppoCorrenteDelProdotto`; la
    regola di O-01): fra i gruppi del prodotto, sulle occorrenze attive (`NonAttivi`), per occorrenza, sha256 e radice
    (`SorgenteBozza`; senza la radice, il ripiego per una chiave «cand:…»), in quest'ordine: la candidata della stessa
    occorrenza, la BOM di lavoro della stessa occorrenza, la BOM di lavoro su un'altra occorrenza dello stesso contenuto;
    altrimenti `assente`, con il motivo `nessun_gruppo`, `occorrenza_non_attiva` (la BOM di lavoro della stessa
    struttura su un'occorrenza non attiva, **o la candidata della propria occorrenza non attiva**: DECA P-4 (a) e (b)) o
    `senza_radice`. Lo usano la striscia della pagina, R1 `?confronto=1`, l'avviso `PROPOSTA_CAMBIATA` di R4 e
    `riallinea_proposta`;
  - **l'adattatore del registro d'importazione** (`ImportaGruppo`): dall'esito, dal prodotto, dal gruppo, dalle regole di
    lettura del thread (`valutazione.RegoleDiLettura`) e dalle occorrenze non attive, il contenuto del registro (il DTO di
    `core/rfq/distintabozza/importazione`), chiuso con `importazione.Componi`: una sola implementazione delle impronte,
    quella della foglia (P-11, P-19). L'identità proposta viene da `CatenaCodice.Forma` (mai `IdentitaNodo`) con
    l'emittente esplicito (`cliente:<uuid>` / `partner:<id>`); `codice_proposto` e `rev_proposta` sono le proiezioni
    importazione-1 (`ProiezioneCodice`, `ProiezioneRev`), anche su ogni lettura del registro (DECA PE-08); i nodi tolti o
    non raggiunti stanno fra i nodi fuori per decisione; relazioni con i due estremi, file e letture citate, diagnostiche
    per nodo o in testata, contesto e perimetro del §3.8.2. Una diagnostica dell'ancoraggio o della valutazione entra solo
    se parla del gruppo: il prodotto, e ogni occorrenza che cita è lo STEP del gruppo o un file attivo del registro, mai
    un'altra copia dello STEP (il rif di un nodo è uguale su ogni copia) né un file non attivo (contato nella nota
    `importazione.evidenze_file_non_attivi`). I rifiuti del §6.1 hanno codice e motivo
    (`CORPO_NON_VALIDO`/`gruppo_non_importabile`, `FILE_NON_ATTIVO`, `PROPOSTA_NON_DISPONIBILE` con
    `errore_valutazione` o `registro_incompleto`); il peso lo controlla la foglia;
  - **il confronto fra due registri** (`Confronta`, `ConfrontaCaselle`): `uguale`, `regole_cambiate`,
    `proposta_cambiata`, con le differenze di PE-18 (`{nodi[], archi[]}` più i campi del gruppo) calcolate con la
    proiezione della foglia (`importazione.ImpronteParziali`) su un contenuto ridotto alla voce o al campo: una differenza
    c'è se e solo se il campo cambia `impronta_contenuto`; con `uguale` le differenze sono vuote per costruzione. La
    radice del gruppo serve solo a classificare la voce della radice: non entra nel confronto delle altre voci.
    `ConfrontaCaselle` (la bozza senza registro) non dà mai «aggiunta» la voce della radice, che il nodo del prodotto
    rappresenta (P-2), né una voce fuori per decisione;
  - **l'effetto di un cambio della fonte** (`EffettoDelCambioFonte`, cambio-fonte-1 in sola lettura sulla `VistaBozza`):
    gli elenchi della risposta di R2 con i suoi nomi (caselle e archi riscritti, nodi fuori per cambio fonte, identità
    aggiornate, da rivedere con il motivo, decisioni cadute, differenze) e quelli della sola anteprima (nodi che restano
    ed entrano, preassociazioni nel vassoio). Un nodo escluso da una `non_includere` lo rappresenta la decisione: la sua
    voce non rientra, e un arco verso di lui non è fra le differenze. Con la stessa struttura sulla stessa occorrenza il
    JSON è solo `{"stessa_sorgente": true}`; gli elenchi vuoti dell'effetto, e le differenze vuote, sono assenti;
  - **il contenuto di S6** (`AnteprimaDelloSTEP`): i gruppi dello STEP per il prodotto con l'impronta_contenuto di
    adesso, la candidata in vista, la proposta (i byte del registro), il confronto del codice della radice con il codice
    del componente del prodotto nella working (`IngressoAnteprima.CodiceProdotto`, che passa chi chiama), le associazioni
    previste, lo STEP secondario e, con una bozza, l'effetto del cambio della fonte. Il confronto e il motivo della scelta
    della fonte sono una funzione sola con R2 (`ConfrontoDelCodice`, `MotivoFonteObbligatorio`: il motivo serve con
    «diverso» e, per prudenza, con «non determinabile»);
  - **le aggiunte di V1 al modello di `/dati`** (`Modello`, EM §9.9; PE-01, PE-10, PE-15, PE-16, PE-28): i gruppi con
    l'impronta_contenuto di un'importazione fatta adesso e i nodi, ognuno con la completezza del 2D
    (`{requisito, esito, deroga?}`), le diagnostiche correnti (quelle della voce del registro di adesso) e
    `decisione_esistente.identita_registrata` (`IdentitaDelRegistrato`: la certificazione valida, oppure il cliente
    presunto); per gruppo le diagnostiche della testata (quelle di più nodi, MAT PN-07); lo STEP secondario; il motivo
    della mancanza di gruppi. Le occorrenze non attive non entrano fra i gruppi, nemmeno la BOM di lavoro: se non resta un
    gruppo con uno STEP il motivo è `step_non_attivo`, con gli allegati (`StepNonAttivi` per il motivo della fonte). Un
    errore o un panic dell'adattatore restano nel gruppo (`importazione` con `ERRORE_CALCOLO`), e la pagina non cade.

## Non appartiene qui

- Il DTO della bozza e del registro, le query, le rotte e la scrittura del registro: `core/rfq/distintabozza` (con la
  foglia `importazione`) e il web. `distintabozza` importa questo pacchetto, mai il contrario.
- Le regole di lettura del thread (partner attivi, contesto risolto, perimetro, impronta dell'indice):
  `valutazione.RegoleDiLettura`.
- Lo stato di partenza della bozza (P-1…P-8) e lo snapshot nuovo del cambio della fonte: corsia persistenza (R2).
- Le letture del database del modello di `/dati` (occorrenze non attive, bozze aperte, F8, identità certificate):
  `transport/web/motore_a` (`dati_v1.go`), nello snapshot della fotografia.

## File

- **`gruppi.go`** — commento `// Package`; le chiavi e i tipi dei gruppi, `ChiaveCandidata`, `Gruppo`,
  `GruppiDelProdotto`.
- **`gruppo_corrente.go`** — `SorgenteBozza`, `NonAttivi`, `GruppoCorrente` con i modi (`Come*`) e i motivi
  (`Motivo*`), `GruppoCorrenteDellaSorgente`, `GruppoCorrenteDelProdotto`, il ripiego per chiave.
- **`importa.go`** — `Ingresso`, `ImportaGruppo`, i codici e i motivi dei rifiuti, il gruppo, il contesto (§3.8.2), la
  testata, `regole` e `calcolo`.
- **`importa_voci.go`** — le voci e i nodi fuori per decisione (disgiunti, come vuole la foglia), l'identità proposta, le
  alternative, i riferimenti (lettura_ruolo, funzione_relazione, equivalente_k4, documentale, candidato_revisione), gli
  attributi, le associazioni proposte, i fabbisogni.
- **`importa_file.go`** — gli archi, le relazioni (sorgente nodo o file), i file del registro con il perimetro, le
  diagnostiche (una diagnostica sta in una voce solo se cita un nodo solo; altrimenti nella testata, una volta) e quelle
  dell'adattatore.
- **`importa_letture.go`** — la lettura del registro (sempre la stessa funzione: la stessa chiave_lettura ha gli stessi
  byte ovunque), `ProiezioneCodice`, `ProiezioneRev`, il testo della struttura da rileggere (LS-01).
- **`codici_diagnostica.go`** — i codici `importazione.*` dell'adattatore, nell'elenco d'oro.
- **`confronta.go`** — `Confronto`, `Differenze`, `DifferenzaNodo`, `DifferenzaArco`, `Confronta`, `ConfrontaCaselle`.
- **`cambio_fonte.go`** — `VistaBozza` e i suoi tipi, `EffettoCambioFonte`, `EffettoDelCambioFonte`.
- **`anteprima.go`** — `IngressoAnteprima`, `Anteprima`, `AnteprimaDelloSTEP`, `ConfrontoDelCodice`,
  `MotivoFonteObbligatorio`.
- **`modello.go`** — `IngressoModello`, `ModelloDelProdotto`, `GruppoDelModello`, `NodoDelModello`,
  `CompletezzaDelNodo`, `IdentitaRegistrata`, `IdentitaDelRegistrato`, `StepSecondario`, `MotivoSenzaGruppi`, `Modello`,
  `StepSecondarioDelProdotto`, `CodiceErroreCalcolo`, `importaProtetto`.

## Invarianti

- Nessuna chiave si scompone: la regola guarda allegato, sha256 e radice dei gruppi; il ripiego confronta la chiave
  intera.
- Un gruppo di un altro prodotto non vale mai (la chiave non contiene il prodotto); una candidata di un'altra occorrenza
  non vale mai; una candidata su un'occorrenza non attiva non c'è (ma la propria dice `occorrenza_non_attiva`).
- Il registro: stesso ingresso, stessi byte e stesse impronte; nessun orario, `BundleID`, ID o impronta
  d'interpretazione nella parte confrontata (stanno in `calcolo`); un'occorrenza non attiva non entra (la conta la
  diagnostica `importazione.evidenze_file_non_attivi`); un file nel perimetro del contesto senza `contesto_id` e
  `impronta_contesto` è un registro incompleto (422), mai un campo `null`.
- Pura e deterministica: niente DB, file, orologio, `uuid.New`; gli ingressi di chi chiama non cambiano.

## Dipendenze

- **Importa:** `core/valutazione` (l'esito, il prodotto valutato, le regole di lettura), `core/ancoraggio` (le
  strutture, la catena del codice, i segnali), `core/inbox/classificazione/motorea` (le letture e le relazioni),
  `core/estrazione/evidenze` (il documento, le diagnostiche), `core/rfq/distintabozza/importazione` (il DTO del
  registro, foglia: P-10), `platform/jsoncanonico`, `github.com/google/uuid`. La lista chiusa di
  `core/estrazione/evidenze/dipendenze_test.go` lo fissa, con l'eccezione del nome di `importazione` fra i vietati ai
  puri.
- **È importato da:** `transport/web/motore_a` (i gruppi, il modello di `/dati`). Lo useranno R2, R1 `?confronto=1`,
  `riallinea_proposta` e S6 (ondata C).

## Test

- **`gruppi_test.go`** — L1 — le costanti; `GruppiDelProdotto`; T-09: i quattro esiti e i motivi di `assente`, con le
  occorrenze non attive (anche la candidata della propria occorrenza, P-4 b), il ripiego per chiave, la BOM di lavoro
  con un'altra struttura; la forma JSON del gruppo corrente.
- **`importa_letture_interna_test.go`** — L1 — le proiezioni (A-24: «…_rev_b» → `B`, la rev con spazi o di 11
  caratteri, il token sospeso; i tratti con affissi, NFD, base parziale, token nel tratto, ripetizione discordante,
  spazio, base ripetuta) e i due campi di una lettura (A-25: la rev solo con il codice).
- **`modello_interna_test.go`** — L1 — un errore o un panic dell'adattatore restano nel gruppo (`ERRORE_CALCOLO`) nel
  modello di `/dati` e in S6.
- Le prove dell'adattatore sulle scene della valutazione (A-01…A-08, A-10…A-19, A-23, A-25, A-26…A-29, T-13 sui campi,
  sulle proiezioni e i sottocasi a scena, il confronto A-18 con la quantità, il passaggio fra voci e fuori e le relazioni,
  `ConfrontaCaselle`, il cambio della fonte con la `non_includere`, S6, i file non attivi, la copia dello STEP, le
  diagnostiche dell'adattatore): in `core/valutazione/importazione_*_scena_test.go`, perché usano le scene della
  valutazione; ogni registro dell'adattatore passa `importazione.Leggi`, `Componi(Leggi(x).Contenuto)` e
  `importazione.Rileggi` con le sue colonne.
- T-09, la sostituzione dello STEP con una radice di riserva e il cancello del passo 3 bis («Revoca», due radici, la
  sostituzione non autorizzata): in `core/valutazione/gruppo_corrente_sostituzione_test.go`; P-4 (a) e (b) con i fatti
  calcolati in `importazione_modello_scena_test.go` (`TestA28LoSTEPNonAttivo`).
- A-09, la parità con `Indice.Gruppi`, e A-28 sui nomi JSON di `/dati`: in `transport/web/motore_a`.

## Leggi anche

- `internal/core/rfq/distintabozza/importazione/README.md` (il DTO del registro)
- `internal/core/valutazione/README.md`
- `internal/core/ancoraggio/README.md`
- `internal/transport/web/README.md` (la pagina del Motore A, `motore_a`, e `/dati`)
