---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/valutazione` — il percorso puro del motore A: richiesta, target, fonte, strutture, verifica della BOM, disegni 2D, completezza documentale, e da B6 `Calcola`, l'esito, il vecchio, i record piatti, lo smistamento, i nodi, lo stato del prodotto, il fascicolo e l'impronta

## Scopo

- Il **percorso puro comune del banco e dell'anteprima** (giro 5; piano A, par.3.3.8, 6.4.6; R42 B): dalla
  fotografia già chiusa produce il «nuovo» del motore A e, per ogni prodotto target della RFQ, i suoi assi (R79).
  Non confronta e non vede mai l'atteso (R2).
- In B1 (commit P7a):
  - **gli ingressi del file dei casi** (`LeggiIngressi`, `Ingressi`, `IngressoCaso`, `SegmentoDichiarato`): la
    decodifica stretta, con i segmenti dello scenario sempre di origine «scenario» (R29 a A, b C; R75 A);
  - **la richiesta del thread** (`RichiestaDelThread`): una per thread, il gesto 1 per messaggio (T-B0-06),
    `valutata` solo con il gesto dell'operatore, l'entrata e la controparte del cliente del thread; il triage solo
    evidenza (R29 c); l'uso dei segmenti dal caso o dal riconoscimento tracciato della parte corrente (R48 A); le
    decisioni come impronta dei gesti (T-16);
  - **i prodotti target** con l'asse 1, l'identità (R60 A, R70 A, R75 A, R78): `ProdottoValutato` con i soli campi di
    B1, `StatoIdentitaTarget`, il predicato `TargetConfermato`;
  - **la fonte strutturale STEP**, l'asse 2 (R65 A, R76 b A; T-B0-07, T-B0-08, T-E1-09, T-12): `FonteProdotto`,
    `StatoFonte`, `MotivoFonte`;
  - il punto d'ingresso **`ValutaProdotti`**, quello che `Calcola` (B6) userà per ogni thread.
- In B5 (commit P7b, fase 1):
  - **il collegamento con `ancoraggio`** (6.4.6 passi 7-9; le NOTE per B5 dei resoconti di B4): `ancoraggio` non
    importa la fotografia (T-B0-04), quindi qui si convertono i file del thread (`FileInterpretato`, con la
    disponibilità e il segno del 2D), i target (`ProdottoRichiesto`, con il componente, il marcatore, la revisione e la
    fonte solo confermata, sull'allegato che porta le righe legacy di quel contenuto: T-B5-15) e il contesto
    (`ContestoStrutturale`), e si chiama `ancoraggio.ProponiAncoraggi` una volta per thread: le strutture e la BOM di
    lavoro, la catena del codice, gli ancoraggi dei file, la riconciliazione escono in `ValutazioneProdotti.Ancoraggi`;
  - **la fonte allineata alla regola unica del marcatore** di `ancoraggio` (T-B4-30, T-B2-02): un candidato del motore A
    con il marcatore scritto da tutte e due le parti e diverso non è candidato;
  - **nomenclatura e gerarchia**, gli assi 3 e 4 (R80; contratto §1.0 righe 3 e 4, §1.3, §2.3, §2.6): la regola
    `VerificaDellaBOM` su due ingressi astratti, `GestiVerificaBOM` e `StrutturaDaVerificare`, separata
    dall'adattatore legacy di «Conferma l'albero» (T-B0-22); `VerificaBOM`, `VerificaAsse`, `StatoVerificaAsse`,
    `GestoVerifica`; `ProdottoValutato.Struttura` (`ancoraggio.StatoStrutturaDelTarget`) e `ProdottoValutato.BOM`;
    `bom.fonte_non_registrata` (R61 A); la lettura A di K-02, confermata dall'utente [U], in una funzione sola; la
    gerarchia legacy con la fonte confermata ma senza la BOM di lavoro, `bom_di_lavoro_assente`, in un'altra funzione,
    fuori da K-02 (T-B5-17 [T], da confermare dall'utente);
  - **i conflitti di nomenclatura e di gerarchia come pezzi** (`Conflitto`, T-B0-24, R95 A, T-E1-15): dalle discordanze
    della riconciliazione di `ancoraggio` con l'effetto conflitto, senza doppioni (`ConflittiDellaNomenclatura`); le
    quantità decise che la BOM di lavoro contraddice, sommate per coppia di componenti sotto lo stesso nodo padre (una
    somma parziale minore non contraddice: T-B5-16), e le rimozioni aperte. B6 li compone nell'esito.
- In B5 (commit P7b, fase 2) **i disegni 2D** (R82, R83, R84, R104; contratto §1.6, §2.3, §2.5, §2.6):
  - **la validità** dal contenuto, con il formato come proprietà: l'elenco chiuso dei formati (`FormatiDisegno2D`,
    `FormatoDi`, `VersioneFormati2D = 1`: PDF, TIFF, PNG; T-B0-30), la regola pura `ValiditaDisegno` su
    `ContenutoDisegno`, separata dall'adattatore dei fatti dei PDF (T-B0-22, T-B0-31; TIFF e PNG oggi
    `contenuto_non_verificabile`, LD-01); il cartiglio (`letto`, dalle unità del cartiglio anche dell'OCR, come in
    `ancoraggio`: T-B5-37; `non_leggibile`, `formato_senza_lettura`) e l'anteprima (`RiferimentoAnteprima`, oggi solo il
    PDF che si apre: LD-03, T-B5-38) come campi a sé (T-E1-21);
  - **i 2D di ogni componente** attivo (`Disegno2D`, `DisegniDelComponente` in `ValutazioneProdotti.Disegni`): i
    documenti `disegno_2d` confermati sul componente, correnti e no, le proposte aperte con «assegna», i candidati del
    motore A dalle posizioni degli ancoraggi (T-B5-95); un contenuto per gruppo; lo stesso 2D in più componenti
    (T-B5-96);
  - **il gruppo e il primario** (`Primario`, `GruppoDisegni2D`, `RelazioneDisegni`): niente si scarta, niente si fonde;
    il corrente e confermato, la compatibilità di codice e revisione con il componente, il formato (prima la
    validità: T-B5-39), il più recente, lo sha256 (R84, T-B0-26, T-E1R-06); le relazioni da confermare con il tipo solo
    quando le evidenze lo sostengono (T-E1-08);
  - **la revisione del documento** (R104, E1R §5; T-E1R-05…09): le evidenze lette (`EvidenzaRevisione`), la proposta
    (`PropostaDiRevisione`, `RevisioneProposta`: cartiglio leggibile → STEP dell'entità → nome del file), documento.rev
    registrata con la provenienza, la decisione sul documento (la parte `documento` di `ancoraggio.DecisioneIdentita`,
    senza adattatore: LD-27), lo stato, le discordanze sempre calcolate (`DiscordanzaRevisione`), il conflitto
    `identita_documento` come pezzo (`ValutaDisegno`, su ingressi astratti: `DisegnoDaValutare`, `FonteDelDisegno`,
    `ComponenteDaConfrontare`).
- In B5 (commit P7b, fase 3) **la classificazione e la completezza documentale**, l'asse 6 (R62, R72 D [R], R82, R93,
  R99 A, R102 A, R103 C; contratto §1.0 riga 6, §1.6, §2.3, §2.5, §2.6; T-E1-10, T-E1-17, T-E1-18, T-E1R-01, T-E1R-03):
  - **la classificazione** (`Classificazione`, `Classifica`, `EsenteDal2D`, `ConfermaCategoria`, `ComponenteClassificato`
    in `ValutazioneProdotti.Classificazioni`): ruolo e categoria separati; la categoria dal tipo confermato o da una
    `ConfermaCategoria` (l'ingresso astratto, che in A1c nessun adattatore produce: LD-19); quella della grammatica in
    `Proposta`, che non esenta mai;
  - **i fabbisogni** (`FabbisogniDelTipo`, `FabbisogniDelComponente`): la base della vista per i componenti confermati
    (R62 d C), le regole effettive per i nodi proposti e per i componenti senza righe nella vista; gli invarianti in Go
    (il 2D di ogni componente del perimetro, anche del `commerciale`, con `NotaRegola`); **la lettura A di K-01**,
    confermata dall'utente [U], in una funzione sola (`rigaNonBloccanteDel2DK01`); l'unica esenzione, il solo 2D della
    minuteria confermata, mai il finito;
  - **la voce del 2D** (`VoceDelDisegno`) su tutto il gruppo della fase 2, e gli altri tipi dalla vista (`EsitoDallaVista`);
  - **il perimetro e lo stato** (`ChiusuraDelPerimetro`, `StatoDellaCompletezza`, `Completezza`, `CompletezzaDocumentale`
    in `ProdottoValutato.Documenti`): le voci certe, i previsti, i non bloccanti, `PerimetroChiuso` con il motivo;
    `documenti.deroga_non_sostituisce_2d`;
  - **il ritocco della fase 2** (T-B5-46 [T]): nel cartiglio il testo non interpretato della revisione si confronta con la
    revisione letta esatto dopo gli spazi ai bordi, come `stessaRevisione` (R-14).
- In B6 (commit P7c, fase V1; fase 0 di B6 congelata, F.2, CP.2, IM.1–IM.2, con le decisioni F0-01…F0-18 e T-B6-01…12):
  - **il punto d'ingresso `Calcola`** (6.4.6, la sequenza dei passi 1–11): `ValidaFotografia` e l'impronta della
    fotografia; per ogni thread, in ordine di ID, il motore della grammatica del cliente, il caso del file dei casi,
    `ValutaProdotti`, il vecchio e i record piatti; i messaggi fuori RFQ dei casi di censimento (R34); l'impronta
    dell'esito per ultima. Un problema di un thread non ferma gli altri;
  - **l'esito** (`Esito`, `EsitoThread`, `EsitoFuoriRFQ`, `EvidenzaFile`, `MotivoThread`, `VersioneValutazione`): il DTO
    che il banco legge e, da A1d, l'anteprima, con i campi del contratto (§2.3). Il contenitore interno `ValutazioneProdotti`
    non è un DTO: `Calcola` lo consuma e ne compone i campi nel thread (T-B6-10); i conflitti di B5 entrano con il prodotto
    e senza doppioni (`componiConflitti`, F0-13); le diagnostiche di `ancoraggio` restano in `Ancoraggi` (F0-04);
  - **gli involucri** `FileInterpretato` e `MessaggioInterpretato` (F0-03): gli stessi campi dei tipi di `ancoraggio`,
    con i tag snake_case, e un record per ogni allegato del thread, anche per quelli che l'adattatore lascia fuori, con il
    motivo (`MotivoFile*`). Attenzione al nome: `valutazione.FileInterpretato` non è `ancoraggio.FileInterpretato` (ha i
    tag e il `Motivo`), e così `MessaggioInterpretato` (D-V1-6 della revisione di V1);
  - **il vecchio** (T-B6-06): `LeggiCodiceRegistrato` e `LetturaRegistrata`, l'involucro esportato della regola di B1 con
    il motivo di una lettura mancata; il vecchio di ogni file (`vecchioLetto`, interno: F0-10), dalla proposta attuale e,
    se il file è deciso, dal documento confermato (registro §10.2); `confronto.codice_registrato_non_leggibile`;
  - **i record piatti** per `confronto` (R53 B; CP.1–CP.3, F0-09): `FileConfrontabile`, `VecchioPiatto`, `NuovoPiatto`,
    `CandidatoPiatto`, `ProdottoConfrontabile`, gemelli dei DTO di `confronto`, con le scelte fatte qui una volta sola (le
    basi, la lettura del vecchio, il confronto delle revisioni con la grammatica);
  - **i tipi degli assi 5 e 7, dei nodi e del fascicolo** che l'esito porta (`VerificaSmistamento`, `AssociazioneFile`,
    `FileDaSmistare`, `NodoBOM`, `StatoProdotto`, `MotivoProdotto`, `StatoFascicolo`, …) e `ProdottoValutato` completo, con
    i valori prudenti dei campi che le fasi V2 e V3 calcolano (F.3): mai «verificato» per difetto;
  - **l'impronta dell'esito** (`VersioneImprontaProdotto = 1`, IM.1); `valutazione.errore_valutazione` (F0-05).
- In B6 (commit P7c, fase V2) **lo smistamento**, l'asse 5 (R81, R85, R91, R93 [U][R], R95 A, R105 [U]; contratto §1.0
  riga 5, §1.5, §1.7, §2.3, §2.5, §2.6; E1R §6; T-B0-12, T-B0-25, T-B0-29, T-B0-32; T-E1-03, T-E1-05, T-E1-11,
  T-E1-12, T-E1-13, T-E1-15; T-E1R-08, T-E1R-10, T-E1R-11; T-B6-07, T-B6-08, F0-12, F0-17, F0-18):
  - **le associazioni dei file** (`AssociazioneFile`, una per allegato in `EsitoThread.Associazioni`): la proposta del
    motore A, la destinazione F8, l'«assegna», il documento confermato, l'origine più forte, «proposta e accettata» o
    «corretta a mano», l'esclusione (solo il gesto «scarta»), lo stato terminale, il perimetro di R93 (b), la pertinenza;
  - **la pertinenza per evidenza** e **il contesto del messaggio**, con R106 B e R107 come l'utente le ha precisate il
    07/10 (domande-a1c.md; R107 non coincide con una lettera, e in A1c resta la formula dell'opzione A, lettura [T]),
    ognuna in una funzione sola (`contestoApplicabile`, `prodottiNominati`); dopo le risposte, il contesto con la sua
    provenienza per ogni file che conta (`AssociazioneFile.Contesto`, `ContestoMessaggio`) e la contraddizione visibile
    (`valutazione.contesto_discorde`);
  - **gli orfani** e **«da smistare»** (`FileDaSmistare` in `EsitoThread.DaSmistare`), con R108 A, confermata
    dall'utente con il requisito operativo per lo spazio di verifica, in una funzione sola (`voceDaSmistare`); gli orfani
    come avviso del fascicolo (`StatoFascicolo.Orfani`, `.Avvisi`, F0-18);
  - **i conflitti dell'asse smistamento** (associazione, con i motivi `componente_diverso` e `codice_confermato`;
    `nuovo_file`; `identita_documento` dai pezzi dei 2D, con i prodotti a cui il documento è pertinente), composti con
    quelli di B5 (`componiConflitti`);
  - **la regola dello smistamento** (`Smistamento`, su `IngressoSmistamento` e `FileDelloSmistamento`, con i percorsi di
    revisione tecnica come ingresso: `PercorsoRevisione`, `OriginePercorso`; in A1c nessun adattatore, LD-18) e lo
    smistamento di ogni prodotto in `ProdottoValutato.Smistamento`.
- In B6 (commit P7c, fase V3) **i nodi, lo stato del prodotto, il fascicolo e l'impronta** (R78, R79, R88, R89, R90 [U],
  R91, R94 A precisata [R], R96 (a) A, (b) B, (c) A [R], R100 A; contratto §0.3, §1.0 riga 7, §1.3, §1.7, §2.3, §2.5;
  T-B0-21, T-B0-23, T-B0-28, T-B0-29, T-B0-39; T-E1-01, T-E1-14, T-E1-16, T-E1-19; T-E1R-04, T-E1R-12; LD-10, LD-23;
  F0-06, F0-07, F0-08; T-B6-09, T-B6-11, T-B6-12):
  - **i nodi della BOM di lavoro** (`NodoBOM` in `ProdottoValutato.Nodi`, solo per la BOM di lavoro: T-B6-11): la
    parentela, i 2D del componente deciso o candidati del nodo, l'origine dell'associazione del primario, i motivi dei
    file del nodo, la classificazione, gli altri prodotti con lo stesso componente (`AncheIn`), la descrizione;
  - **l'impronta del prodotto** (`ImprontaProdotto` sui dati decisi, `DatiDecisiProdotto` e i suoi tipi): si calcola per
    ogni prodotto, anche non verificato; la salverà A2 (R100 A);
  - **il composto e lo stato del prodotto**, l'asse 7 (`StatoDelProdotto`, con l'ingresso `CondizioniNuove`): un solo
    predicato, `Verificato`, e lo stato che ne deriva, con `da_riesaminare` secondo T-E1-14 e R111 A, precisata
    dall'utente il 07/10, in una funzione sola (`vociNuoveDopoLaFonteSuperataR111`); il confronto semantico fra due STEP
    che la risposta chiede esce da A1c come limite dichiarato (E2 §3.4: il primo pezzo dopo A1c);
  - **il fascicolo** (`Fascicolo`, con l'ingresso `IngressoFascicolo` e il gesto di congelamento del modello nuovo,
    `GestoCongelamento`, senza adattatore: LD-23) e il congelamento legacy a parte (`CongelamentoLegacy`, dalla
    bom_versione).

## Non appartiene qui

- **Il DB**: la fotografia la legge `core/fotorfq/caricatore`, prima e altrove; qui niente DB, file, orologio, rete.
- **I candidati della mail**: li propone `core/ancoraggio` (`ProponiProdotti`); qui si chiamano, e non diventano mai
  target (R60 A). Lo stesso per i tipi della fonte (`RiferimentoFonte`, `DocumentoCandidato`, …), che stanno in
  `ancoraggio`.
- **Il confronto con le proposte attuali e con l'atteso**: `core/confronto` e `app/bancoa`. `valutazione` non importa
  `confronto`, il caricatore, la libreria YAML. Qui si preparano i record piatti; badge, indicatore di revisione,
  correzioni manuali ed esiti li calcola `confronto`, e la copia nei suoi DTO la fa chi chiama (`app/bancoa`,
  `passaggio.go`).
- **Il gesto di congelamento del modello nuovo e il suo servizio** (R89; LD-09, LD-23): il DB di oggi non lo ha; lo
  registrerà lo spazio di verifica (SV). Qui è solo un ingresso della regola (`Calcola` passa nil), e il fascicolo non è
  mai congelato sui dati veri (`gesto_non_registrato`). Così **l'avvio della fattibilità per prodotto** (LD-10): qui
  `pronto_fattibilita` è un risultato calcolato, che non fa avanzare niente; la fase è del thread (`FaseThread`).
- **Il salvataggio dell'impronta del prodotto e il confronto con un'impronta salvata**: A2 (R100 A, T-E1R-12). Qui
  l'impronta si calcola soltanto.
- **I nodi delle strutture candidate come `NodoBOM`**: il contratto dà `PV.Nodi` solo alla BOM di lavoro (T-B6-11); sui
  dati veri i nodi stanno negli ancoraggi (`EsitoThread.Ancoraggi.Strutture`). Se la UI e il riquadro di A1d ne hanno
  bisogno, lo valuta B7.
- **I percorsi di revisione tecnica e il gesto di scarto come scritture**: il DB di oggi non ha percorsi (LD-18), e la
  pulizia fisica delle copie scartate è di un servizio futuro (LD-26, E1R §6.3). Qui il percorso è solo un ingresso della
  regola (`Calcola` passa nil), lo scarto si legge com'è (la proposta scartata con `deciso_da`), e niente si cancella.
- **L'adattatore della conferma della categoria** (`ConfermaCategoria`): il DB non ha la categoria (LD-19); la
  scriverà lo spazio di verifica (SV). Qui la regola si prova con conferme sintetiche. La distinta del PDF di assieme e
  lo STEP multibody come evidenza (RC-01) sono fuori dal contratto congelato: niente qui.
- **Le regole delle strutture, degli ancoraggi, della catena del codice e della riconciliazione**: `core/ancoraggio`.
  Qui si sceglie solo che cosa della fotografia entra, e si leggono i suoi segnali (le discordanze, `QuantitaDiscorde`).
- **Le azioni sui conflitti** (accettare, mantenere con un motivo, correggere, sistemare l'associazione) e la loro chiusura
  persistente: la UI e il servizio futuro (T-E1-15, R95 A). Qui i conflitti si calcolano, con le evidenze dei due lati.
- **L'adattatore delle decisioni sull'identità di un documento** (`DecisioneIdentita` con l'oggetto documento): il DB di
  oggi non le ha (LD-27); le scriverà lo spazio di verifica (SV). Qui la regola si prova con decisioni sintetiche.
- **La decodifica delle immagini, il cartiglio raster, l'anteprima comune** (LD-01, LD-03, LD-04): il worker e A1d.
- **Il significato delle codifiche dei clienti**: le grammatiche, lette da `motorea`.

## File

- **`valutazione.go`** — responsabilità:
  - commento `// Package`; `ValutazioneProdotti`, `ValutaProdotti`: i documenti dei messaggi, la richiesta,
    l'interpretazione, i candidati della mail, i target, la fonte di ogni target; da B5 gli ancoraggi del thread, lo
    stato della struttura e la verifica della BOM di ogni target, i conflitti, i 2D dei componenti, la completezza di
    ogni target e la classificazione dei componenti; le diagnostiche in ordine; da B6 `valutaThread`, la stessa con il
    collegamento del thread, (V2) lo smistamento del thread e dei prodotti e (V3) i nodi, l'impronta e lo stato di ogni
    prodotto.
- **`calcola.go`** (B6, V1) — responsabilità:
  - `Calcola`; l'esito di un thread (`esitoDelThread`: il motore e il motivo, l'errore della valutazione, i record dei
    file, le evidenze, i record piatti, il vecchio dei prodotti; da V2 le associazioni, i file da smistare, i conflitti
    dello smistamento composti con quelli di B5; da V3 il fascicolo, con gli orfani di V2); i messaggi fuori RFQ dei casi
    di censimento (`fuoriRFQ`, `valutaFuoriRFQ`); l'ordine delle diagnostiche.
- **`esito.go`** (B6, V1) — responsabilità:
  - `VersioneValutazione`, `Esito`, `EsitoThread`, `MotivoThread`, `EsitoFuoriRFQ`, `EvidenzaFile`; gli involucri
    `FileInterpretato` e `MessaggioInterpretato` (F0-03); i motivi di un file non valutato (`MotivoFile*`).
- **`vecchio.go`** (B6, V1) — responsabilità:
  - `LetturaRegistrata`, i motivi `MotivoLettura*`, `LeggiCodiceRegistrato`; `vecchioLetto` e la scelta della proposta e
    del documento di ogni allegato; la lettura dei dettagli (la lettura del vecchio motore, la destinazione F8);
    `confronto.codice_registrato_non_leggibile`; il vecchio dei prodotti (`vecchiProdotti`); da B6b la revisione vecchia
    interpretata (`revisioneVecchia`, `leggiRevisioneVecchia`: il codice, oppure la colonna `rev` letta con
    `motorea.LeggiRevisioneRegistrata`; R113 B).
- **`confrontabile.go`** (B6, V1) — responsabilità:
  - i record piatti `FileConfrontabile`, `VecchioPiatto`, `NuovoPiatto`, `CandidatoPiatto`, `ProdottoConfrontabile`; i
    valori `Revisioni*`, `RevisioneDa*` e `MotivoRevisioni*`; la costruzione: il nuovo di ogni allegato (le basi, i
    candidati con la base del target, del componente deciso o del nodo, le radici, la revisione del file e il confronto
    con la revisione vecchia, del codice o della colonna), i candidati prodotto piatti.
- **`impronta.go`** (B6, V1 e V3) — responsabilità:
  - `VersioneImprontaProdotto`; l'impronta dell'esito (`improntaEsito`);
  - (V3) l'impronta del prodotto: l'ingresso astratto (`DatiDecisiProdotto`, `ImprontaFonte`, `ImprontaComponente`,
    `ImprontaRelazione`, `ImprontaDocumento`), la regola `ImprontaProdotto`, l'adattatore (`datiDecisiDelProdotto`) e le
    sezioni di T-12 (`improntaNonDeterminabile`, con quelle della fonte: R-82; `unaSezioneAssente`, l'aiuto che usa
    anche il fascicolo).
- **`smistamento.go`** (B6, V1 per i tipi, V2 per la regola) — responsabilità:
  - i tipi dell'asse 5 che l'esito porta: `StatoSmistamento`, `MotivoSmistamento`, `VerificaSmistamento`, i valori di
    `Perimetro`, `AssociazioneFile`, `FileDaSmistare`;
  - i percorsi di revisione tecnica (`OriginePercorso`, `PercorsoRevisione`, l'ingresso astratto); l'ingresso della regola
    (`IngressoSmistamento`, `FileDelloSmistamento`) e la regola `Smistamento`, con i motivi di un file e di un conflitto.
- **`associazioni.go`** (B6, V2) — responsabilità:
  - l'adattatore dello smistamento di un thread (`smistamentoThread`): per ogni allegato il perimetro, la proposta, il
    documento, l'esclusione, lo stato terminale, l'«assegna», la destinazione F8 (`fileDelThread`); le associazioni
    (`associazione`, `destinazioneCoincide`); i conflitti di associazione, `nuovo_file` e `identita_documento`; l'ingresso
    della regola per ogni prodotto (`ingressoDelProdotto`).
- **`pertinenza.go`** (B6, V2) — responsabilità:
  - la pertinenza per evidenza (`evidenzeDiPertinenza`, `nomina`), il contesto del messaggio e gli orfani (`pertinenza`);
    le tre risposte del 07/10 in una funzione sola ognuna: R106 B, precisata (`contestoApplicabile`), R107, precisata
    (`prodottiNominati`, con la provenienza), R108 A, confermata con il requisito operativo (`voceDaSmistare`); i prodotti
    con cui il contesto si confronta (`prodottiCollegati`) e `valutazione.contesto_discorde`
    (`diagnosticaContestoDiscorde`); gli avvisi degli orfani (`avvisiDegliOrfani`).
- **`nodi.go`** (B6, V1 per i tipi, V3) — responsabilità:
  - `NodoBOM`, `Parentela`; i nodi della BOM di lavoro di un prodotto (`nodiThread`, `nodiDellaBOM`): la raggiungibilità
    senza gli archi tolti, il componente del nodo, la descrizione, la parentela, i 2D, l'origine del primario, i motivi
    (`motiviDelNodo`), la classificazione, `AncheIn`.
- **`prodotti.go`** (B6, V1 per i tipi, V3) — responsabilità:
  - `StatoProdotto`, `MotivoProdotto`, `CondizioniNuove`; la regola `StatoDelProdotto` con i motivi
    (`motiviDelProdotto`), le condizioni nuove dello smistamento e della completezza (T-E1-14); l'adattatore delle
    condizioni nuove della BOM (`condizioniNuoveDellaBOM`); R111 A, precisata dall'utente il 07/10, in una funzione sola
    (`vociNuoveDopoLaFonteSuperataR111`).
- **`fascicolo.go`** (B6, V1 per i tipi, V3) — responsabilità:
  - `StatoFascicolo` con i valori di `MotivoNonCongelabile`, `MotivoNonCongelato` e `ConflittiCongelamento`,
    `ProdottoBloccato`, `CongelamentoLegacy`, `GestoCongelamento`, `IngressoFascicolo`; la regola `Fascicolo`; le sezioni
    di T-12 (`fascicoloNonDeterminabile`, con quelle dello smistamento: la nota di V2); il congelamento legacy dalla
    fotografia (`congelamentoLegacy`).
- **`ingressi.go`** — responsabilità:
  - `VersioneCasi` (1), `Ingressi`, `IngressoCaso`, `SegmentoDichiarato`, `Ingressi.CasoDelThread`, `LeggiIngressi`;
  - la porta stretta: BOM, UTF-8, surrogati soli, versione, chiavi sconosciute, ripetute o con le maiuscole diverse,
    null, numeri non interi, testo dopo l'oggetto; poi i controlli del contenuto (ID, cliente, thread, uso, origine,
    autorità). I codici sono quelli «contratto.*» della grammatica.
- **`richiesta.go`** — responsabilità:
  - `RichiestaDelThread`: lo stato di ogni messaggio e del thread, l'evento del triage come evidenza, l'uso dei
    segmenti con il caso o con il riconoscimento, le decisioni (T-16).
- **`target.go`** — responsabilità:
  - `StatoIdentitaTarget`, `ProdottoValutato` (Rif, Autorita, ComponenteID, CodiceRichiesto, Base, Identita, Fonte;
    da B5 Struttura, BOM e Documenti), `TargetConfermato`;
  - i target del thread (R70 A, R75 A), la lettura del codice registrato con la grammatica (6.4.6, nell'ordine fisso
    nome_file, cartiglio.codice, nodo_step.id, corpo), `target.possibile_rinomina` (T-E1-24); da B5 la lettura principale della mail per intero per un prodotto
    dello scenario (con il marcatore: T-B4-30).
- **`fonte.go`** — responsabilità:
  - `StatoFonte`, `MotivoFonte` (i sedici valori), `FonteProdotto`;
  - gli STEP del thread letti dai fatti con gli adattatori di `estrazione` e con le letture della grammatica, il
    confronto delle basi con `motorea.ConfrontaBasi` e, da B5, con la regola unica del marcatore (T-B4-30);
  - la fonte di un prodotto: la tabella di R65 sugli esiti della vista, il gesto 3, i candidati, l'ordine dei motivi
    di una fonte assente, T-12.
- **`ancoraggi.go`** (B5, fase 1) — responsabilità:
  - il collegamento con `ancoraggio`: i file del thread (documento, interpretazione, `disponibilitaDi`, il tipo
    documentale e il segno del 2D), `prodottiRichiesti` con `allegatoPortatore` (T-B5-15), `contestoStrutturale`
    (componenti, relazioni, righe aperte e
    decise, codici composti e dei messaggi, associazioni decise), `ancoraggiDelThread` (che restituisce anche il
    collegamento, per i 2D); le copie profonde.
- **`bom.go`** (B5, fase 1) — responsabilità:
  - `GestiVerificaBOM`, `GestoVerifica`, `StatoVerificaAsse`, i motivi `MotivoAsse*`, `VerificaAsse`, `VerificaBOM`,
    `StrutturaDaVerificare`, `RifArco`;
  - la regola `VerificaDellaBOM`, con le voci da decidere di ogni asse (`vociDaDecidere`: da B6, V3, anche per lo stato
    del prodotto), la gerarchia legacy senza la BOM di lavoro (`gerarchiaSenzaBOMDiLavoro`, T-B5-17) e la lettura A di
    K-02 (`bomSenzaFonteConfermataK02`);
  - l'adattatore legacy (`gestiDaConfermaLAlbero`), la struttura del prodotto (`strutturaDaVerificare`,
    `struttureDellaVerifica`), il perimetro della BOM confermata, `bom.fonte_non_registrata`.
- **`conflitti.go`** (B5, fase 1) — responsabilità:
  - `Conflitto`, `TipoConflitto`, `AsseConflitto`, `EvidenzaDecisione`, `EvidenzaProposta`, i motivi;
  - `ConflittiDellaNomenclatura`, i conflitti di gerarchia (quantità e rimozioni), l'ordine canonico; da B6 la
    composizione dei pezzi nell'esito del thread (`componiConflitti`).
- **`disegni.go`** (B5, fase 2) — responsabilità:
  - `Formato2D`, `VersioneFormati2D`, `FormatoDisegno`, `FormatiDisegno2D`, `FormatoDi`; `ValiditaDisegno2D`,
    `MotivoFabbisogno` (i nove valori del contratto), `ContenutoDisegno`, la regola `ValiditaDisegno`;
  - i valori del cartiglio, `RiferimentoAnteprima`, `Disegno2D` con i valori di `FonteIdentita` e `ConfrontatoCon`;
  - `GruppoDisegni2D`, `RelazioneDisegni` con i loro valori; `Primario` (l'ordine dei criteri, la regola che decide, le
    relazioni da confermare).
- **`revisione2d.go`** (B5, fase 2) — responsabilità:
  - `EvidenzaRevisione`, `RevisioneProposta`, `DiscordanzaRevisione`, gli stati della revisione del documento, i motivi
    delle evidenze e della proposta; gli ingressi astratti `FonteDelDisegno`, `DisegnoDaValutare`,
    `ComponenteDaConfrontare`; `RifDocumento`;
  - `PropostaDiRevisione`; `ValutaDisegno`: la decisione sul documento, lo stato, l'identità della compatibilità, la
    compatibilità con il componente, le discordanze, il conflitto `identita_documento`.
- **`disegni_thread.go`** (B5, fase 2) — responsabilità:
  - `DisegniDelComponente`; l'adattatore dei 2D dalla fotografia e dagli ancoraggi: le voci di ogni componente
    (documenti, «assegna», candidati), una per contenuto, il contenuto di un documento (dal suo allegato, da un file
    con lo stesso sha256, dai suoi fatti), l'adattatore dei fatti al `ContenutoDisegno`, il cartiglio, le fonti
    d'identità (il cartiglio, lo STEP dell'entità, il nome del file, il documento), la provenienza di documento.rev, la
    rev_diversa della vista, il lato del componente; per la fase 3 i 2D candidati di un nodo senza decisione
    (`gruppoDelNodo`) e i candidati da_determinare di un componente (`candidatiDaDeterminare`).
- **`classificazione.go`** (B5, fase 3) — responsabilità:
  - `Classificazione` con i valori di ruolo, categoria, origine e motivo, `ConfermaCategoria`,
    `ComponenteDaClassificare`; la regola `Classifica`, l'esenzione `EsenteDal2D`.
- **`completezza.go`** (B5, fase 3) — responsabilità:
  - `StatoDocumenti`, `EsitoFabbisogno`, i valori di `NotaRegola`, i motivi della completezza, del perimetro e dei
    previsti; `VoceFabbisogno`, `FabbisognoPrevisto`, `FabbisognoInformativo`, `CompletezzaDocumentale`;
  - gli ingressi astratti `IngressoCompletezza`, `RigaDelPerimetro`, `FabbisognoDellaRiga`, `DisegnoDellaVoce`,
    `NodoDaPrevedere`, `RegolaFabbisogno`, `FabbisognoRisolto`;
  - le regole `FabbisogniDelTipo`, `FabbisogniDelComponente` (con la lettura A di K-01 in `rigaNonBloccanteDel2DK01`),
    `VoceDelDisegno`, `EsitoDallaVista`, `ChiusuraDelPerimetro`, `StatoDellaCompletezza`, `Completezza`.
- **`completezza_thread.go`** (B5, fase 3) — responsabilità:
  - l'adattatore: per ogni target le righe certe del perimetro con i fabbisogni della vista o delle regole effettive
    (con l'esito calcolato come la vista senza righe), la voce del 2D, le righe con una rimozione aperta, i nodi da
    prevedere con il tipo proposto; le sezioni assenti (T-12); `documenti.deroga_non_sostituisce_2d`;
    `ComponenteClassificato` e la classificazione dei componenti attivi.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici di B1: `ancoraggio.target_non_leggibile`, `target.possibile_rinomina`,
    `fonte_strutturale.riferimento_incoerente`, `fonte_strutturale.radice_non_registrata`; di B5:
    `bom.fonte_non_registrata`, `documenti.deroga_non_sostituisce_2d`; di B6: `valutazione.errore_valutazione`,
    `confronto.codice_registrato_non_leggibile`, `valutazione.messaggio_fuori_rfq_senza_caso` (R-63 della revisione di
    V1); dopo le risposte del 07/10, `valutazione.contesto_discorde` (R106 B, precisata).

## Entry point

- **`Calcola`** (B6) — chi lo chiama: il banco (`app/bancoa`, P9) e, da A1d, l'anteprima (`transport/web`), con la
  stessa fotografia, le stesse regole e lo stesso file dei casi; oggi le prove.
- **`LeggiCodiceRegistrato`** (B6) — chi lo chiama: `Calcola` per il vecchio, attraverso la regola privata; chi vuole la
  lettura di un codice registrato con il motivo (A1d).
- **`Smistamento`** (B6, V2) — chi la usa: `ValutaProdotti` (attraverso l'adattatore), senza percorsi; la regola si prova
  con ingressi sintetici e percorsi sintetici (PO-24, PO-25); dopo A1c lo spazio di verifica, con i percorsi veri.
- **`StatoDelProdotto`**, **`Fascicolo`**, **`ImprontaProdotto`** (B6, V3) — chi le usa: `ValutaProdotti` e `Calcola`
  (attraverso gli adattatori), senza il gesto di congelamento; le regole si provano con ingressi sintetici (PO-24, PO-31,
  il gesto sintetico); dopo A1c lo spazio di verifica (il gesto) e A2 (l'impronta salvata). Il banco (P9) legge i loro
  risultati nella sezione `prodotti` del rapporto, solo come informazione (T-B0-16).
- **`ValutaProdotti`** — chi lo chiama: `Calcola`, per ogni thread (attraverso `valutaThread`); le prove.
- **`LeggiIngressi`, `Ingressi.CasoDelThread`** — chi li chiama: il banco, con la voce `casi` del manifest;
  l'anteprima, con il file che l'indice delle regole dichiara (R29 b C): lo stesso file.
- **`RichiestaDelThread`, `TargetConfermato`** — chi li usa: `ValutaProdotti`; da B6 lo stato del prodotto
  (`ProdottoVerificato` comincia da `TargetConfermato`, R79).
- **`VerificaDellaBOM`** — chi la usa: `ValutaProdotti`, con i gesti dell'adattatore di «Conferma l'albero»; dopo A1c
  lo spazio di verifica, con il gesto nuovo (R100 A). **`ConflittiDellaNomenclatura`**, **`RifArco`** — chi li usa:
  `ValutaProdotti`; da B6 la composizione del `Conflitto` e lo stato del prodotto.
- **`ValiditaDisegno`**, **`FormatoDi`**, **`FormatiDisegno2D`** — chi li usa: l'adattatore dei 2D; la regola si prova
  con contenuti sintetici (PO-08, PO-09). **`Primario`**, **`ValutaDisegno`**, **`PropostaDiRevisione`** — chi li usa:
  `ValutaProdotti` (attraverso l'adattatore); dalla fase 3 la completezza (`VoceFabbisogno.Disegni`), da B6
  `NodoBOM.Disegni` e lo smistamento (il conflitto `identita_documento`); dopo A1c lo spazio di verifica, con la
  decisione sul documento.
- **`Completezza`**, **`FabbisogniDelTipo`**, **`FabbisogniDelComponente`**, **`VoceDelDisegno`**, **`EsitoDallaVista`**,
  **`ChiusuraDelPerimetro`**, **`StatoDellaCompletezza`** — chi li usa: `ValutaProdotti` (attraverso l'adattatore); la
  regola si prova con ingressi sintetici (PO-28 con `ConfermaCategoria`, PO-33, PO-36); da B6 lo stato del prodotto
  (`DocumentazioneCompleta`, T-E1-14). **`Classifica`**, **`EsenteDal2D`** — chi li usa: la completezza; da B6 i nodi
  della BOM (`NodoBOM.Classificazione`); dopo A1c lo spazio di verifica, con la conferma della categoria.
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **I target sono solo i confermati e lo scenario** (R60 A):
  - confermato (R70 A, R78): un identificativo con `confermato_da`, con il suo componente (stesso codice, senza
    maiuscole: il legame è per codice, come nel DB, LD-21) se c'è e non è archiviato; un finito attivo di origine
    «manuale». `componente.confermato_da` non conta mai. Rif: `componente:<uuid>` con il componente,
    `identificativo:<codice>` senza (emendamento E1 §4.2);
  - scenario (R75 A): con un caso di autorità «scenario», i prodotti letti dalla mail in un segmento che il caso ha
    scelto, sempre `da_confermare`, mai `TargetConfermato`, qualunque sia il gesto 1. Rif: `scenario:<caso>:<n>`.
    Uno con la base di un target confermato non si ripete;
  - i candidati della mail non sono mai target, e restano da confermare.
- **La fonte non si conferma mai da sola** (contratto §0 n.6): `confermata` viene solo dal gesto 3, letto attraverso
  la vista (R65 A, l'unica definizione). Un candidato del motore A porta una fonte assente in attesa di conferma, mai
  una confermata né una in attesa (T-B0-07). Candidato è solo uno STEP la cui **radice** ha la base del prodotto
  (R76 b A); un prodotto solo come nodo interno dà `solo_nodo_interno`. Solo lo STEP è fonte, mai un PDF (R68 A,
  T-E1-09).
- **L'ordine dei motivi di una fonte assente** (T-E1-09): `estrazione_fallita`, `analisi_in_corso`,
  `step_presente_non_analizzato`, poi `solo_nodo_interno`, `indicata_sul_portale`, `solo_altro_3d`,
  `prodotto_senza_componente` (un target senza componente, T-B0-07), `nessuna_fonte`.
- **Il collegamento con `ancoraggio`** (B5; T-B0-33, T-E1-22, T-08, T-E1-06, T-B4-31, T-B4-33, T-B0-31):
  - un file è un allegato di natura «file» che non è un contenitore; un errore dell'adattatore o dell'interpretazione
    lascia fuori quel file, non il thread;
  - la disponibilità, il primo che vale: `mancante` (senza sha256), `pendente` (analisi in attesa), `illeggibile`
    (`pdf.illeggibile`), `errore` (la qualità «errore»), `senza_testo` (`pdf.senza_testo`: una scansione, T-B0-31),
    `illeggibile` (la qualità «non_disponibile», che gli adattatori oggi non danno), `parziale`, `disponibile`;
  - il 2D è il tipo `disegno_2d` del documento confermato che porta il file (prima i correnti), altrimenti della sua
    proposta;
  - il target porta la fonte confermata solo con lo stato `confermata` della vista: una fonte in attesa, anche con il
    riferimento, non fa la BOM di lavoro (R76 A, R65 A);
  - la BOM di lavoro sta sull'allegato **portatore** delle righe legacy di quel contenuto (T-B5-15; il legacy scrive le
    righe dello stesso STEP arrivato due volte su un allegato solo): quello del riferimento se le porta, altrimenti il
    primo in ordine di ID, fra quelli con la struttura di quel contenuto, che le porta; se nessuno le porta, quello del
    riferimento. `ProdottoValutato.Fonte.Riferimento` di B1 resta com'è (il primo allegato del documento): il
    riferimento mostrato e l'allegato portatore possono essere due allegati dello stesso contenuto, con lo stesso
    sha256 e la stessa struttura;
  - il codice manuale viene solo da una riga aperta con `origine_codice = operatore`; le righe decise non portano mai
    il loro codice né la loro revisione; i componenti e le relazioni archiviati non entrano; `LettureDecise` e
    `DecisioniIdentita` restano vuote (LD-27);
  - le associazioni decise: i documenti confermati correnti sul componente, e le proposte aperte con «assegna».
- **La verifica della BOM** (R80; T-B0-07, T-B0-22, T-B0-24; R61 A, R95 A), per ognuno dei due assi il primo che vale:
  `conflitto`; `non_verificabile` per un target senza componente e, nel legacy, per un prodotto senza figli né
  confermati né proposti; `da_verificare` senza il gesto (nel modello nuovo anche senza struttura); `da_verificare` con
  qualcosa da decidere (nel modello nuovo anche le relazioni confermate che il gesto non copre, come nel legacy:
  T-B5-99); nel modello nuovo la gerarchia vuota, cioè senza archi nelle strutture e senza relazioni confermate,
  aspetta la nomenclatura della radice (una gerarchia non vuota no); altrimenti `verificata`. Il conflitto prevale anche per il target senza componente (T-B5-09 [T], da
  ratificare dall'utente). Poi, nel legacy con la fonte confermata ma senza la BOM di lavoro (l'analisi in corso, la
  radice non registrata, lo STEP senza struttura letta, nessuna struttura), la gerarchia che sarebbe verificata è
  `da_verificare`, `bom_di_lavoro_assente` (T-B5-17 [T], da confermare dall'utente: la gerarchia dipende dalla BOM di
  lavoro; non è K-02). Poi K-02, lettura A: senza la fonte confermata la gerarchia che sarebbe verificata è
  `da_verificare`, `fonte_non_confermata`. `Verificata` = tutti e due `verificata`; `FonteRegistrata` mai con il legacy.
- **L'adattatore di «Conferma l'albero»** (R80, R61 A): il gesto è il segno sulle righe d'arco tenute (confermate o
  duplicato, decise da una persona, con i due componenti) il cui padre è nel perimetro del prodotto (il suo componente
  e i componenti attivi raggiungibili per le relazioni confermate); da decidere restano le righe dei nodi senza la
  decisione di una persona, **esclusa la riga della radice** della struttura (T-B4-24), i nodi che non hanno nessuna
  riga legacy del loro contenuto su nessun allegato (la rete di sicurezza di T-B5-15: il segno non copre un nodo che il
  legacy non ha mai proposto), le relazioni confermate che il segno non copre e, per la gerarchia, le righe d'arco
  aperte o decise da un automatismo e gli archi che non hanno nessuna riga legacy del loro contenuto su nessun allegato
  (la rete degli archi, simmetrica a quella dei nodi: R-21; vale anche per il perimetro della completezza). Contano la BOM di lavoro, se c'è, altrimenti tutte le strutture candidate del
  prodotto (R71 A, R76 A).
- **I conflitti** (T-B0-24, R95 A, T-E1-15): bloccano l'asse e la decisione resta il valore corrente; nomenclatura dalle
  discordanze con l'effetto conflitto sulle strutture che contano (gli indicatori non bloccano: R97 B, T-B4-39), senza
  i doppioni dello stesso nodo in più strutture (due 2D diversi restano due conflitti); gerarchia dalla somma delle
  quantità per coppia di componenti sotto lo stesso nodo padre, solo sulla BOM di lavoro, e dalle rimozioni aperte sul
  perimetro. Una somma parziale non contraddice (T-B5-16): sotto un nodo padre con un figlio non ancora deciso (senza
  la decisione accanto e non scartato da una persona) una somma minore della quantità decisa non si confronta, una
  maggiore resta un conflitto. L'ordine canonico è (asse, Rif, motivo, origine, decisione, proposta) e poi l'allegato,
  il documento e l'unità dell'evidenza.
- **I limiti noti della gerarchia legacy** (T-B5-13 [T]; R61 A):
  - un arco deciso che lo STEP confermato non contiene più si legge solo dalla `rimozione_proposta` aperta del legacy,
    calcolata con le sue condizioni (lettura completa, una radice, figli diretti decisi da una persona, profondità 1):
    qui non si ricalcola. Su un DB in cui le rimozioni non sono state ricalcolate dopo un nuovo STEP, la gerarchia può
    risultare `verificata` con un arco assente;
  - il segno di «Conferma l'albero» non registra la fonte, quindi vale da qualunque versione dello STEP: una relazione
    confermata con uno STEP vecchio resta coperta, e l'assenza dell'arco nella versione nuova la segnala solo la
    rimozione del legacy.
- **I 2D** (R82, R83, R84; T-B0-26, T-B0-30, T-B0-31, T-E1-08, T-E1-21):
  - il formato è l'estensione dichiarata, nell'elenco chiuso; la validità guarda il contenuto, mai l'identità: un PDF
    aperto è valido anche senza testo, errore_pdf è `contenuto_non_aperto`, i fatti senza testo né errore
    `contenuto_non_letto`, nessun fatto `contenuto_non_analizzato`, TIFF e PNG `contenuto_non_verificabile`, DWG, DFT e
    JPG `formato_non_configurato`;
  - il cartiglio è `letto` con almeno un'unità con il selettore del cartiglio, dal testo nativo o dall'OCR, come lo
    legge `ancoraggio` sullo stesso file (T-B5-37 [T]; E1R §5.2, R83, LD-04): la capacità `cartiglio` dell'adattatore
    dice il testo nativo e non conta; il testo del worker di prima resta `non_leggibile` (i suoi campi sono unità
    `testo_pdf`);
  - l'anteprima: oggi solo per il PDF, tranne quello che il worker non apre (`contenuto_non_aperto`: T-B5-38); un PDF
    non analizzato o non letto la ha (A1c non vede i byte);
  - i 2D di un componente: i documenti `disegno_2d` confermati sul componente (correnti e no), «assegna», i candidati del
    motore A con una posizione su un nodo del componente (la decisione per UUID del nodo, o la radice che rappresenta
    il prodotto); un contenuto una volta sola, la voce più forte (documento corrente, «assegna», candidato, documento
    non corrente: un'associazione aperta non sparisce dietro un documento sostituito, T-B5-33, contratto §1.6 e R62 b
    A); un componente archiviato non ha 2D;
  - il primario: corrente e confermato; compatibile per codice e poi per revisione; il formato, prima la validità
    (`valido`, poi `da_verificare`, poi `formato_non_configurato`) e poi PDF, TIFF, PNG, con la regola `formato` (il
    PDF è il preferito solo fra documenti compatibili, validi e correnti: la precisazione E1 su R84, T-B5-39); il
    confermato più di recente; lo sha256. Le relazioni: una per coppia, sempre da confermare; `revisione_diversa` (solo
    senza un codice discordante), `rappresentazione_alternativa` o `non_determinata` con le letture;
  - le revisioni si confrontano esatte dopo gli spazi ai bordi, come in `ancoraggio` e come le tiene la grammatica
    («a» e «A» sono due revisioni: T-B5-34); `upper(btrim())` è solo della `rev_diversa` della vista, che si riporta
    com'è. Anche nel cartiglio il testo non interpretato del campo a sé si confronta così con la revisione letta in
    linea: «a» accanto a «A» dà `revisione_non_interpretabile`, senza valore (T-B5-46 [T]).
- **La revisione del documento** (R104, E1R §5; T-E1R-05…09):
  - le evidenze, una per fonte: il cartiglio (le letture di `cartiglio.codice` e la revisione del campo a sé; un
    cartiglio leggibile senza revisione è un'evidenza senza valore), lo STEP dell'entità (la revisione interpretata del
    nodo del componente associato o dell'unico candidato, mai `rev_grezza`, mai propagata; più candidati o più nodi di
    revisione diversa: `entita_non_univoca`), il nome del file, documento.rev;
  - la proposta: cartiglio, poi STEP dell'entità, poi nome del file, poi `assente`, con il motivo della fonte prima;
    documento.rev mai;
  - lo stato: `confermata` (una decisione sul documento), `registrata` (documento.rev, con la provenienza), `proposta`,
    `assente`;
  - la compatibilità: dal lato del 2D la revisione confermata, poi il cartiglio, il nome del file, documento.rev;
    dal lato del componente l'identità proposta dai suoi nodi, poi componente.rev;
  - le discordanze, sempre: `documento_componente` (in Go e quella della vista), `proposta_registrata` (indicatore),
    `proposta_confermata` (conflitto solo con un'evidenza nuova);
  - il conflitto `identita_documento`: una decisione sul documento contro un'evidenza nuova che la contraddice, sul
    codice o sulla revisione; mai da documento.rev né da una fonte non interpretabile; la decisione resta;
  - **limite noto** (T-B5-42, T-B4-22): la coppia fonte-valore del cartiglio è il solo testo di `cartiglio.codice`.
    Una revisione nuova solo nel campo a sé (`cartiglio.revisione`) non rende nuova l'evidenza, quindi contro una
    decisione sul documento quel conflitto non nascerebbe. Oggi non tocca niente (nessuna decisione sui documenti,
    LD-27); per lo spazio di verifica la coppia dovrà comprendere anche il campo della revisione.
- **La classificazione** (T-E1-18, T-E1R-01; emendamento E1 §6.3, E1R §4.2):
  - ruolo `prodotto` per il componente di un prodotto target, qualunque sia il suo tipo (R-23), e per il finito anche
    quando non è un target, `componente` per gli altri; categoria `fabbricato` (derivata) per finito,
    sottoassieme e sciolto, `commerciale` (confermata: il tipo lo scrive solo una persona) per il commerciale,
    `non_determinata` per un nodo senza tipo deciso;
  - una `ConfermaCategoria` (la più recente) sostituisce la categoria; fuori da fabbricato, commerciale, minuteria non
    conta; sul prodotto la minuteria confermata si mostra, ma il 2D resta (`finito_chiede_sempre_il_2d`);
  - la categoria della grammatica va in `Proposta` (`minuteria`), mai in `Categoria`: una minuteria proposta non toglie
    niente;
  - `EsenteDal2D`: solo la minuteria confermata che non è il prodotto (il finito o il componente del target: R99 A,
    R-23), e solo dal 2D. È l'unico controllo dell'esenzione: **B6 usa solo `EsenteDal2D`** (per esempio nei nodi della
    BOM), mai `Categoria == minuteria && Confermata` da solo.
- **La completezza documentale** (contratto §1.6; R72 D [R], R102 A, T-E1-10; T-E1-17, R103 C, K-01 A):
  - le righe certe sono il componente del prodotto e i componenti attivi raggiungibili per le relazioni confermate, di
    qualunque origine (R62 e A, T-B5-91); fuori dal perimetro e archiviati non hanno voci;
  - i fabbisogni di una riga certa vengono dalla vista (R62 d C), con `CalcolataDa = vista`; senza righe nella vista
    dalle regole effettive del suo tipo, con l'esito calcolato come la vista e `CalcolataDa = go`; `RegolaCliente` dalle
    regole effettive (`RigaFabbisogno.Proprio`);
  - il 2D è un invariante (`Invariante`) di ogni componente del perimetro: senza la riga si aggiunge con
    `schema_senza_2d` o `regola_cliente_senza_2d`; la riga esplicita non bloccante del cliente si rispetta sui
    componenti (fra i non bloccanti) e non sul prodotto, il finito o il componente del target qualunque sia il suo tipo
    (resta bloccante: R99 A, R-23), con `regola_cliente_2d_non_bloccante`: è la
    lettura A di K-01, confermata dall'utente [U], tutta in `rigaNonBloccanteDel2DK01`. Una riga di default non
    bloccante sul 2D, che lo schema non ha, non toglie l'invariante (T-B5-54). Lo STEP strutturale del finito non è una
    voce: è l'asse della fonte;
  - la voce del 2D guarda tutto il gruppo: presente con un 2D corrente, confermato e valido (anche senza cartiglio);
    da verificare per il contenuto, poi per l'associazione («assegna», un candidato del motore A, la proposta aperta
    della vista, un da_determinare candidato: LD-17), mai per un file scartato da una persona (T-E1R-10) né per un
    file in un formato non configurato; manca con la deroga (`derogato_non_sostituisce_2d`, R62 D.4), con un documento
    in un formato non configurato (`formato_non_configurato`), altrimenti `nessun_documento`. Gli altri tipi bloccanti
    come nella vista (`derogato` presente: S3; `da_confermare` e `sul_portale` da verificare);
  - una riga che il prodotto raggiunge solo attraverso archi con una rimozione aperta non dà voci certe (potrebbe
    uscire dal perimetro): i suoi fabbisogni bloccanti vanno fra i previsti (`rimozione_aperta`). Un figlio di una
    rimozione aperta raggiungibile anche per un'altra via resta fra le voci certe, e una sua mancanza certa non si perde
    (T-B5-56, deciso con la raggiungibilità: R-22). Fra i previsti vanno anche i nodi senza decisione delle strutture che
    contano (`nodo_proposto`, con il tipo proposto della riga legacy o quello che il legacy scriverebbe) e i nodi decisi
    fuori dal perimetro; non un nodo che la radice raggiunge solo attraverso archi tolti da una persona (T-B5-67);
  - `PerimetroChiuso`: la fonte confermata (il predicato dell'asse 2), la BOM di lavoro (una BOM di lavoro assente non
    chiude il perimetro: T-B5-92), nessun nodo né arco da decidere (esclusa la radice), nessuna rimozione aperta;
  - lo stato: `incompleta` con una mancanza certa, o a perimetro chiuso con una voce non presente; `completa` a perimetro
    chiuso con tutto presente, qualunque sia la nomenclatura; `non_calcolabile` altrimenti, per il target senza
    componente (T-B0-07) e con una sezione della fotografia assente (T-12).
- **«Estrazione fallita» non è «dati insufficienti»** (T-B0-07): la prima è il worker che non ha letto il file, la
  seconda un grafo incompleto della vista o di un candidato.
- **T-12**: con una sezione della fonte assente la fonte non si calcola: niente stato, motivo `non_determinabile`.
- **Il T-12 allargato** (lettura dell'orchestratore T-B1-11): senza grammatica o con il codice del target non letto,
  quando uno STEP analizzato del thread potrebbe essere candidato (o è la radice di una delega), la fonte non si
  calcola (`non_determinabile`) e l'esito della vista resta accanto; uno stato già dato dalla vista resta, salvo la
  delega con la radice non confrontabile.
- **Il triage è solo evidenza** (R29 c): sta in `Evento` e nel motivo del riconoscimento, mai nello stato.
- **La storia non si promuove**: con il caso valgono solo i segmenti dichiarati; senza, il riconoscimento tocca solo
  la parte corrente di un messaggio valutato (R48 A, E2).
- **L'esito di `Calcola`** (B6, V1; 6.4.6; fase 0, F.2, F.3, IM.1; T-B6-09, F0-03, F0-04, F0-05, F0-13, F0-14):
  - l'errore di `Calcola` è solo di contratto: una diagnostica di gravità errore di `ValidaFotografia` (le altre vanno in
    `Esito.Diagnostiche`), un thread o un messaggio fuori RFQ ripetuto nella fotografia (`contratto.id_ripetuto`: R-66
    della revisione di V1, perché l'esito dipenderebbe dall'ordine), o un'impronta della fotografia che non si calcola
    (`contratto.json_non_valido`); allora l'esito è vuoto;
  - un thread senza motore non è valutato, con il motivo della grammatica: `senza_grammatica_a` (nessuna voce
    nell'indice, `regole.assenti`, o nessun insieme di regole), `grammatica_scartata`, `ragione_sociale_discorde`, con le
    diagnostiche di `MotoreDi`. `ValutaProdotti` si chiama lo stesso, e i prodotti sono informazione (T-B6-09);
  - un errore della valutazione rende il thread non valutato, con `errore_valutazione` se il motore c'è (altrimenti
    resta il motivo della grammatica); se l'errore porta un errore di contratto se ne copiano le diagnostiche, altrimenti
    `valutazione.errore_valutazione` (F0-05). Niente del nuovo; i record dei file e il vecchio ci sono lo stesso;
  - **un record per allegato**, in ordine di `AllegatoID`, in `File` e in `Confrontabili`, anche per i contenitori, per
    le nature che non sono «file» e per i thread non valutati (F0-03; CP.2). Il motivo di un file non valutato, il primo
    che vale: `contenitore`, `natura_non_file`, `thread_non_valutato` (solo nel nuovo piatto), `documento_non_leggibile`;
    allora l'associazione è `non_valutata` (A1c-L1-14). **L'errore dell'adattatore o dell'interpretazione su un file**
    (R-61 della revisione di V1; F0-05) è una diagnostica di gravità **avviso**, nel thread o nel messaggio fuori RFQ, con
    il percorso `file[<id>]` e l'allegato nei riferimenti: le diagnostiche dell'errore di contratto che porta (la natura
    resta la loro), altrimenti `valutazione.errore_valutazione`. Il thread prosegue, e il file resta
    `documento_non_leggibile`;
  - **la deviazione `senza_testo`** (T-B6-02): il 6.4.6, passo 8, e A1c-L1-14 davano `illeggibile` alla scansione (un PDF
    senza testo e senza OCR); vale il contratto (T-B0-31, LD-05): `senza_testo`, e `illeggibile` resta per un PDF che non
    si apre. Lo stesso per l'esito atteso di A1c-L4D-07 (Q10);
  - i messaggi fuori RFQ (R34): per ogni caso senza thread, ogni suo messaggio, cercato fra quelli della fotografia e
    valutato con la grammatica del cliente del caso come un thread di un messaggio solo (il documento, l'interpretazione
    con i segmenti del caso, i file, i prodotti della mail), mai con target né ancoraggi. Un messaggio del caso che la
    fotografia non ha è un record non valutato con `valutazione.errore_valutazione`; un messaggio della fotografia che
    nessun caso senza thread elenca è un avviso con il suo codice, `valutazione.messaggio_fuori_rfq_senza_caso` (R-63 della
    revisione di V1; mai silenzio). Per chi chiama: fra i fuori RFQ (`caricatore.Richiesta.Messaggi`) vanno solo i
    messaggi dei casi **senza** thread; i messaggi della richiesta di un caso con il thread stanno nel thread;
  - i conflitti di B5 entrano nell'esito con il loro prodotto, senza doppioni (F0-13, con la chiave allargata al
    documento e all'unità dell'evidenza della proposta: T-B6-23), in ordine canonico e poi di prodotto e tipo;
  - i campi di V3 (i nodi, il composto, lo stato, i motivi, l'impronta, il fascicolo) li calcola V3: vedi «I nodi, lo
    stato del prodotto, il fascicolo e l'impronta». **Mai «verificato» per difetto**;
  - **l'impronta dell'esito** è lo sha256 del canonico dell'esito con l'impronta vuota: copre le cinque versioni (da
    B6b anche `VersioneRevisioneRegistrata` di `motorea`, R113 B),
    l'impronta della fotografia, quella dell'indice (con i limiti: R43 B), ogni thread per intero, i messaggi fuori RFQ,
    le diagnostiche. Fuori per costruzione: `PresaIl` e `Sorgente`, ogni orologio, l'atteso (R2), l'esito di `confronto`;
  - **l'impronta per thread** (F0-14): l'esito è di tutta la fotografia, quindi l'impronta del banco e quella
    dell'anteprima coincidono, per lo stesso thread, solo se il banco gira su quel thread solo (come A1d-L4-09: `Carica`
    con quel thread). Con `-tutti` non esiste un'impronta per thread.
- **Il vecchio e i record piatti** (B6, V1; 6.4.6; R31 c, R53 B; registro §10.2; T-B0-14, T-B1-07; F0-02, F0-09, F0-10):
  - la proposta di un allegato è una (UNIQUE nel DB; con due righe vince la prima per ID); il documento è quello che lo
    porta in `documento_provenienza`, prima i correnti, poi per ID. Un file deciso prende codice, revisione, componente,
    documento e sostituzione dal documento, mai dalla proposta; la proposta dà stato, fonte, «assegna», la decisione e i
    dettagli;
  - il codice si legge con la grammatica (`LeggiCodiceRegistrato`, la regola di B1), mai per stringa: un codice che non
    si legge ha la base vuota e, con la grammatica e un codice non vuoto, `confronto.codice_registrato_non_leggibile`;
  - `CodiceLetto` viene da `dettagli.valutazione.codice` (T-B0-14; F0-02), il valore della dimensione del codice, salvo
    la regola «operatore», che è la decisione di una persona (T-B6-22). Si conoscono solo le versioni 1 e 2 di
    `dettagli.valutazione` (`v`, come `LeggiValutazione` del legacy; R-64 della revisione di V1): un'altra versione, o
    nessuna, lascia `CodiceLetto` vuoto. **Per la `v: 1` vale il valore registrato allora**, cioè la lettura del vecchio
    motore di quel giorno: il legacy ricompone le dimensioni di una riga v1, e qui una regola del legacy non si rifà;
  - `CodiceLettoBase` (F0-19, emendamento di CP.2) è la base di `CodiceLetto` letta con la stessa grammatica
    (`LeggiCodiceRegistrato(m, CodiceLetto).Base`), vuota se `CodiceLetto` è vuoto o non si legge: serve al «prima» delle
    correzioni manuali di `confronto` (R30 f), che così confronta basi, non stringhe. **`CodiceLettoMarcatore`** (il
    gemello per R114, precisata dall'utente il 07/10) è il marcatore della stessa lettura
    (`LeggiCodiceRegistrato(m, CodiceLetto).Marcatore`, cioè `LetturaForma.Marcatore`), vuoto se il marcatore non c'è, se
    `CodiceLetto` è vuoto o se non si legge: `confronto` conta a parte la stessa base con due marcatori scritti e diversi,
    senza sommarla ai cambi di base. Quale dei due conteggi faccia da titolo è una scelta dell'utente (D-R114, aperta in
    domande-a1c.md; non è la D1 della revisione di P8);
  - la destinazione F8 sono le chiavi dei candidati di `dettagli.destinazione`, in ordine, senza doppioni; negli export,
    senza dettagli, vuote;
  - i record piatti sono gemelli dei DTO di `confronto` (stessi nomi, ordine, tipi Go, tag), fatti solo di tipi delle
    foglie: nessun tipo con nome del motore, nemmeno un enumerato. La base di un candidato: quella del target per il
    livello prodotto, del codice del componente deciso letto con la grammatica, della forma del nodo senza decisione.
    **Per `componente:<uuid>` `CandidatoPiatto.Base` è la base del codice del componente, non quella letta nel file** (D1
    della revisione di P8, chiusa; lo dice anche il README di `confronto`);
  - **le radici** sono le basi dei target da cui il candidato si raggiunge, in ordine di byte, senza doppioni; **una o più
    radici la cui base non si legge restano un solo `""`** (R-62 della revisione di V1): una radice in più non sparisce,
    e `confronto` conta `""` come una radice che non coincide con nessuna base attesa, cioè una radice «in più»;
  - **la revisione vecchia** (R113 B ratificata; E2 §2.6; B6b): il valore originale resta in `Codice` e `Rev`;
    l'interpretazione è `VecchioPiatto.Revisione` e la provenienza `RevisioneDa` (il gemello subito dopo
    `CodiceLettoMarcatore`: 19 campi nel vecchio). Il primo caso che vale (`leggiRevisioneVecchia`):
    - il codice che non si legge non ha una famiglia, e la colonna non si legge: nessuna revisione,
      `revisione_vecchia_non_letta` (non determinabile); lo stesso per un codice con una revisione non letta (un token
      sospeso);
    - il codice con la revisione letta: quella, da `codice`. Se la colonna, letta con la regola in campo separato della
      famiglia, non concorda (`ConfrontaRevisioni` discordante), il confronto non si fa: `revisione_vecchia_discorde`,
      senza scegliere; il valore resta quello del codice. Una colonna che non si legge non lo contraddice;
    - il codice senza revisione: con la colonna vuota `revisione_vecchia_non_letta`; con la colonna letta
      (`motorea.LeggiRevisioneRegistrata`, con la famiglia della lettura del codice, senza gli spazi ai bordi) quella, da
      `colonna`; con la colonna che non si legge un motivo per stato, perché il banco la conti per stato (T-B6-202):
      `revisione_solo_in_colonna` (nessuna regola: R-65), `revisione_colonna_non_interpretabile`,
      `revisione_colonna_ambigua`;
    - `Revisione` è vuota se e solo se `RevisioneDa` è vuoto; `RevisioneDa` dice la provenienza del valore, mai una
      scelta fra codice e colonna (T-B6-201). Mai «codice + rev» composti;
    - **limite noto** (T-B6-207; R-148 della revisione di B6b): una colonna `rev` che non si legge (nessuna regola, non
      interpretabile, ambigua) accanto a una revisione letta nel codice non lascia traccia nel record piatto: vale il
      codice, senza motivo, e il banco non la conta fra le colonne per stato.
  - le revisioni si confrontano con la grammatica (`motorea.ConfrontaRevisioni`): la revisione vecchia interpretata
    contro quella comune alle letture d'identità del file, con le sole equivalenze dichiarate dalle regole. I motivi di
    `non_confrontabili` vengono nell'ordine: il file non valutato, il lato vecchio, il lato nuovo.
- **Lo smistamento** (B6, V2; R81, R93, R95 A, R105; contratto §1.5; E1R §6):
  - **il perimetro di R93 (b)** (T-E1-12), il primo che vale: `inline`, `elemento_outlook` (un .msg come contenitore),
    `collegamento`, `contenitore_estratto` (contano le sue voci), `messaggio_in_uscita` (non interno: l'inoltro interno
    conta), `altra_controparte` (un fornitore, un altro cliente), altrimenti `dentro` (anche le controparti sconosciuto,
    ambiguo, interno, altro, e i file proposti come rumore). Un file fuori perimetro non è pertinente, non entra in «da
    smistare» né fra gli orfani, non toglie nessun fabbisogno;
  - **lo scarto è solo un gesto** (T-E1R-10): `Esclusa` solo con la proposta `scartata` con `deciso_da`. **Terminale**
    (T-B0-32): un documento confermato che porta il file (su un componente o generale), il duplicato con `deciso_da`,
    lo scarto. Un file scartato non è da smistare, non è orfano, non tiene aperto niente e non soddisfa nessuna voce;
  - **la pertinenza per evidenza** (T-E1-11), per un file che conta: un candidato del motore A su una struttura del
    prodotto (le radici raggiungibili e le posizioni); la collocazione `non_determinabile` per un target senza strutture
    il cui codice il file legge; la destinazione F8 (ogni chiave: componente del perimetro, nodo di una riga, codice di un
    target), l'«assegna» o il documento su un componente del perimetro (R62 e A); uno STEP candidato della fonte
    (`motore_a`, R76 b). È l'**elenco chiuso** delle sei evidenze che impediscono il ripiego sul contesto (R106 B,
    precisata dall'utente il 07/10), ognuna solo se porta a un prodotto target; **non lo impediscono** l'estensione o la
    natura del file, il tipo proposto, la collocazione `fuori_richiesta` proposta, un codice letto senza candidati (salvo
    la collocazione non determinabile di un target senza struttura), la destinazione `generale`, la disponibilità («una
    caratteristica generica del file, come l'estensione PDF, non è un collegamento a un prodotto»). Poi, solo per un file
    che conta, non è terminale e non ha evidenze, il **contesto** come ripiego: le letture del motore A del messaggio
    dell'allegato esterno, nei segmenti che la richiesta considera, con il ruolo «prodotto», che nominano un target
    (R107, precisata dall'utente il 07/10; in A1c la formula dell'opzione A, alla lettera [T]: R-71 della revisione di
    V2). I
    segmenti sono il corrente di ogni messaggio (oggetto e corpo), anche senza il gesto 1, con la controparte sconosciuta
    o la direzione non nota, più quelli che un caso dichiara pertinenti: le letture con la funzione «richiesta» del
    router. Il caso aggiunge, non toglie il corrente; un corrente che il caso dichiara escluso è menzione e non conta
    (dubbio T-B6-94); la storia conta solo se un caso la dichiara pertinente (R48 A). Un solo prodotto nominato:
    pertinente, con `PertinenzaContesto`; più d'uno: orfano, con `ProdottiContesto` e l'avviso; nessuno: niente. Il
    contesto non è mai un candidato né una conferma. **Il confine di R107 in A1c**: il contesto sono le letture
    deterministiche del segmento corrente e dei segmenti di storia che un caso dichiara pertinenti; nessuna catena di
    risposte, citazioni o inoltri, nessuna risposta senza codice ricostruita: sono del blocco nuovo dell'analisi dei
    messaggi;
  - **il contesto con la provenienza** (R106 B e R107, precisate il 07/10): per **ogni** file che conta, anche terminale
    o con evidenze, `AssociazioneFile.Contesto` porta il messaggio dell'allegato esterno (`MessaggioID`), gli ID delle
    letture che nominano un target (`Letture`: l'ID porta l'unità, quindi il segmento) e i prodotti nominati
    (`Prodotti`); nil se il messaggio non nomina nessun target. La regola della pertinenza non cambia
    (`PertinenzaContesto` resta l'unico effetto, solo come ripiego); i riferimenti di un messaggio che nomina più prodotti
    si conservano anche quando il file non è orfano. Il contesto si calcola anche quando la pertinenza non si calcola per
    T-12, perché `PertinenzaContesto` c'è anche lì (dubbio T-B6-171);
  - **la contraddizione visibile** (`valutazione.contesto_discorde`, avviso, dati): i prodotti nominati non hanno niente
    in comune con quelli a cui il file è già collegato, cioè quelli delle evidenze o, per un file terminale, quelli con il
    componente del documento confermato nel perimetro (`prodottiCollegati`; un documento generale, un duplicato o uno
    scarto non collegano: dubbio T-B6-172). I riferimenti: l'allegato, `contesto:<rif>` per i nominati, `evidenza:<rif>`
    (o `decisione:<rif>`) per i collegati (dubbio T-B6-173). Solo quando la pertinenza si calcola; non blocca niente, non
    cambia la pertinenza né la decisione, e non entra nell'impronta del prodotto. Un collegamento verso un elemento che
    non è in nessun target non è un'evidenza e non dà la diagnostica (R106-3: la sua visibilità esce da A1c come limite
    dichiarato, E2 §3.4);
  - **orfano**: conta, non è terminale, non è pertinente a nessuno; sta in «da smistare» e fa l'avviso
    `orfano:<allegato_id>` (con il contesto `…:prodotti_contesto:<rif>|<rif>`), senza bloccare niente. **Solo quando la
    pertinenza si calcola** (R-75 della revisione di V2): con una sezione dello smistamento assente (T-12: lo scarto, la
    destinazione o un documento non si vedono) o in un thread senza grammatica (T-B6-61: niente candidati né contesto)
    nessun file è orfano, `Fascicolo.Orfani` è 0 e il fascicolo non ha avvisi `orfano:`; i file senza evidenze stanno in
    «da smistare» con il loro motivo, e lo smistamento dei prodotti non è calcolato. Una sezione assente solo della
    completezza non tocca la pertinenza;
  - **«da smistare»** (R108 A, confermata dall'utente il 07/10 con il requisito operativo per lo spazio di verifica: il
    requisito, cioè vedere, correggere e confermare insieme, e distinguere assenza di destinazione, proposta da
    confermare, analisi pendente o fallita, è dello spazio di verifica e del contratto dei DTO di B7): i file che contano,
    non terminali, con uno dei cinque motivi (dai due assi
    della proposta), più gli orfani; la pre-associazione e i file in conflitto si vedono nello smistamento del prodotto.
    Un orfano senza candidati segue la tabella di E1 (§2.5) anche se è discordante (R-74 della revisione di V2):
    `fuori_richiesta_non_confermato` con la collocazione fuori richiesta, altrimenti `nessun_candidato`; la discordanza
    resta in `AssociazioneFile.Associazione`. Con R108 A un `candidato_unico` non entra mai, quindi
    `FileDaSmistare.Proposta` è sempre vuota (nota per B7). La prima condizione dello smistamento (il documento presente
    non confermato) non dipende dal perimetro (R93; T-B6-73, conforme): una voce certa con il file candidato fuori
    perimetro ferma lo smistamento con `associazione_non_confermata` senza file non terminali, e il file si legge in
    `VoceFabbisogno.FileCandidato`, perché fuori perimetro non compare fra i file dello smistamento;
  - **i conflitti** dell'asse smistamento, uno per prodotto (F0-13), senza prodotti uno con il prodotto vuoto (mai
    perso). Di associazione: il documento corrente o l'«assegna» su K, e l'identità letta adesso porta ad altri candidati
    (`componente_diverso`) o un candidato su K non è più compatibile con il codice confermato di K (`codice_confermato`,
    la parola di ancoraggio: T-B6-66); mai senza candidati (R62 g A); va ai prodotti con K nel perimetro e a quelli a cui
    il file è pertinente. Il documento confermato dà il conflitto anche fuori perimetro (R-72 della revisione di V2,
    decisione [T]: il perimetro di R93 vale per la seconda e la terza condizione, non per una decisione contraddetta, e il
    documento soddisfa la voce della completezza); l'«assegna» solo per un file che conta (dubbio T-B6-95). `nuovo_file`:
    un file che conta, non terminale, candidato per K che ha già un documento corrente dello stesso tipo, salvo il 2D; va
    solo ai prodotti con K nel perimetro (R-73), mai fuori perimetro (PO-32). `identita_documento`: i pezzi dei 2D, sui
    prodotti a cui il documento è pertinente. La decisione resta; il documento resta nella completezza. Il riferimento di
    un allegato senza documento è `allegato:<uuid>` (`rifAllegato`, l'aiuto solo del pacchetto);
  - **la regola** (`Smistamento`): le cinque condizioni con i loro motivi (il documento presente non confermato: T-B6-08;
    ambiguo, discordante, non determinabile; i file non terminali; i conflitti; i percorsi aperti), poi in_revisione,
    conflitto, verificato, da_verificare. **Non calcolata** (sempre da_verificare; F.2): il target senza componente
    (T-B6-07, F0-17), una sezione assente (T-12, anche della completezza), il thread senza grammatica (T-B6-61, confermato
    dalla revisione di V2: senza grammatica non ci sono candidati né contesto, e un prodotto con tutti i file noti
    terminali risulterebbe verificato in silenzio, R90). Con un errore della valutazione niente associazioni né file da
    smistare.
- **I nodi, lo stato del prodotto, il fascicolo e l'impronta** (B6, V3; R79, R88, R89, R90, R91, R96; contratto §1.0,
  §1.3, §1.7):
  - **i nodi** (T-B6-11): solo quelli della BOM di lavoro, cioè della struttura sotto la radice scelta dello STEP
    confermato; sui dati veri, senza STEP confermato, `PV.Nodi` è vuoto. Solo i nodi che la radice raggiunge senza un arco
    tolto da una persona (la raggiungibilità dei previsti, T-B5-67); un nodo scartato ma raggiunto resta, con la sua riga
    decisa. Il componente del nodo è la decisione per UUID, o il componente del target per la radice scelta (T-B4-38). I 2D
    del nodo deciso sono il gruppo del componente, quelli del nodo senza decisione i candidati del nodo (`gruppoDelNodo`);
    `Associazione` è la provenienza del primario in quel gruppo. I motivi vengono dai file del nodo (i candidati con una
    posizione sul nodo e, per il nodo deciso, i file associati al componente) che contano e non sono terminali, e dai
    conflitti dello smistamento del prodotto su quei file o sul componente. La classificazione del nodo senza decisione è
    `non_determinata` (nessun tipo deciso), con la minuteria solo proposta: l'esenzione dal 2D la dice solo `EsenteDal2D`.
    `AncheIn` sono gli altri prodotti la cui BOM confermata contiene il componente: lo stesso perimetro dell'impronta
    (T-E1-19). La descrizione è quella del componente deciso, altrimenti quella del PRODUCT dello STEP;
  - **l'impronta del prodotto** (R90 [U]; T-B0-29; IM.3): lo sha256 del canonico dei dati decisi, con gli elenchi in
    ordine: la fonte del gesto 3 (documento, sha256, radice, `Superato`; mai l'allegato), il perimetro confermato (codici e
    revisioni registrate com'è), le relazioni confermate con la quantità, i documenti confermati correnti dei componenti
    del perimetro (tipo, sha256, revisione registrata). Fuori: i documenti generali, le proposte, l'«assegna», F8, i
    candidati, le deroghe, le conferme della categoria, i gesti di verifica, i percorsi, le versioni del motore, il codice
    manuale di una riga aperta (F0-08). Con T-12 (componenti, relazioni, documenti, provenienze, step_prodotto) è vuota;
    è vuota anche con T-12 su una sezione della fonte (allegati, righe_componente_proposta, righe_relazione_proposta,
    lavoro_pendente, proposte_documento), perché senza il riferimento della fonte sembrerebbe l'impronta dei dati decisi
    e non lo sarebbe (R-82 della revisione di V3; scostamento [T] dall'elenco di IM.3, dubbio T-B6-151). Senza grammatica
    (T-B1-11) la fonte si calcola, e l'impronta c'è. Un componente condiviso entra nell'impronta di ogni prodotto che lo
    contiene (PO-26);
  - **lo stato del prodotto** (R79; T-E1-01, T-E1-14): `Verificato` è il solo booleano (JSON `prodotto_verificato`), e
    `pronto_fattibilita` vale se e solo se il prodotto è verificato (PO-35); un prodotto verificato non ha motivi, uno non
    verificato ne ha uno per condizione che manca, nell'ordine dei valori (con `revisione_tecnica_aperta` quando c'è un
    percorso, e `conflitto` con un conflitto aperto su un asse). `da_riesaminare` quando mancano solo condizioni nuove: un
    conflitto aperto, un file pertinente non terminale, la fonte superata, una rimozione aperta (il perimetro aperto per
    lei), un percorso aperto; la gerarchia ferma solo per `fonte_non_confermata` dopo una fonte superata (K-02); il
    perimetro aperto per la fonte superata. Per nomenclatura e gerarchia conta lo stato di base dell'asse, senza i
    conflitti (`condizioniNuoveDellaBOM`). Un thread senza grammatica, un target senza componente e T-12 hanno lo
    smistamento non calcolato, quindi `non_pronto` (T-B6-09, T-B6-61);
  - **le voci certe e il perimetro aperto** (R-81 della revisione di V3; T-E1-14, «non_calcolabile solo perché il
    perimetro è aperto»; R94 A, «tutto il resto ci sarebbe»): con il perimetro aperto da una condizione nuova il prodotto
    è `da_riesaminare` solo se ogni voce certa della completezza è `presente`, cioè se a perimetro chiuso sarebbe
    `completa`. Una voce non presente per una ragione vecchia (il 2D confermato e mai analizzato, il documento non
    confermato anche su un file pertinente non terminale, il formato non configurato) tiene il prodotto `non_pronto` anche
    dietro il perimetro aperto, come a perimetro chiuso (`incompleta`). Lo smistamento non guarda le voci: il giudizio
    sulle voci sta solo nella completezza, uguale per tutti i motivi (dubbio T-B6-82, riscritto da T-B6-150: prima una
    voce `associazione_non_confermata` su un file non terminale contava come condizione nuova, e le voci degli altri
    motivi passavano);
  - **R111 A, precisata dall'utente il 07/10**, in una funzione sola: dopo una fonte superata contano come condizioni
    nuove anche le voci da decidere della nuova fonte, cioè gli effetti della fonte superata (i nodi e gli archi, con le
    chiavi del contratto, che lo STEP confermato prima non aveva). **Fra i due STEP oggi non c'è nessuna
    corrispondenza**: le chiavi contengono lo sha256 del documento (T-E1-04, una chiave valida solo dentro un file), e il
    confronto semantico che l'utente chiede (stessi padri e figli, occorrenze e quantità; gli affissi e le revisioni prima
    dei codici nuovi; un criterio di corrispondenza esplicito, con i casi ambigui irrisolti) **esce da A1c come limite
    dichiarato**: con la decisione dell'orchestratore del 07/10 sera (E2 §3.4; un contrasto isolato con R111, dichiarato
    lì) diventa il primo pezzo dopo A1c, prerequisito del riesame nello spazio di verifica. In A1c lo stato non cambia.
    Il nuovo STEP è il documento corrente in fondo alla catena delle sostituzioni del documento del riferimento. **Il
    limite dichiarato (il limite B previsto dalla R111 A)**: vale solo se la fotografia porta tutte e due le
    versioni, cioè se il prodotto ha una struttura dello STEP di prima e una del nuovo (il caricatore legge tutti i
    documenti del thread, anche quelli sostituiti, e i fatti dei loro contenuti alla terna corrente; la struttura c'è se il
    file dello STEP è fra gli allegati e i suoi fatti si leggono). Se una delle due manca (lo STEP di prima analizzato con
    un'altra terna, o senza allegato nel thread), le voci da decidere non sono mai nuove, e il prodotto è `non_pronto`
    salvo che gli assi siano verificati o fermi per le condizioni che T-E1-14 elenca. **La regola effettiva** (R-85 della
    revisione di V3): le chiavi dei nodi e degli archi comprendono lo sha256 del documento (T-E1-04), quindi ogni nodo e
    ogni arco del nuovo STEP è nuovo, anche con lo stesso codice o la stessa struttura: con le due strutture leggibili,
    dopo una fonte superata, sono nuove tutte e sole le voci del nuovo STEP. Lo STEP di prima serve solo a tre cose: la
    sua struttura deve esserci (altrimenti vale il limite B); le sue voci, e quelle di un terzo STEP, restano vecchie; con lo
    stesso sha256 nessuna voce è nuova. **La conseguenza per l'utente** (D-2 della revisione di V3): con il gesto 3 sul
    nuovo STEP la fonte torna confermata, R111 non vale più, e le voci ancora da decidere tornano condizioni vecchie: il
    prodotto passa da `da_riesaminare` a `non_pronto`, dopo un gesto che fa avanzare, e resta `non_pronto` finché le
    voci nuove non sono decise. Il riesame calcolato non apre da solo un percorso di revisione tecnica;
  - **il limite noto di T-B2-02** (dubbio T-B6-89; R-83 della revisione di V3): con la fonte non confermata (superata),
    `ancoraggio` propone come fonte di una struttura candidata anche lo STEP del riferimento superato, mentre per
    `valutazione` quello STEP non è un candidato: sta in `Fonte.Riferimento`, con `Superato`. Per T-B2-02 fanno fede i
    candidati di valutazione, e `ancoraggio` è chiuso; quella struttura è proprio la «versione di prima» che la funzione di
    R111 usa. La «prova generale» lo dichiara come sola eccezione, e solo con la fonte non confermata. Nota per B7: la UI
    non deve proporre come fonte sceglibile la struttura dello STEP superato;
  - **il fascicolo** (R88, R89; F0-06; T-B6-12): i conteggi, i pronti e i bloccati in ordine di Rif; congelabile se il
    thread è valutato, il fascicolo calcolato e tutti i target, almeno uno, sono verificati (R96 a A). Il motivo, il primo
    che vale: `thread_non_valutato` (anche per un errore della valutazione), `nessun_target`,
    `bom_versione_con_prodotti_non_verificati` (la versione corrente della BOM legacy è congelata, e un prodotto non è
    verificato), `prodotti_non_pronti`; gli altri che valgono insieme vanno in `Avvisi` come
    `non_congelabile:<motivo>`, dopo gli avvisi degli orfani (che non cambiano `Congelabile`: T-E1-16). Non calcolato con
    un errore della valutazione, con una sezione assente fra identificativi, componenti e versione della BOM (T-12), o fra
    quelle dello smistamento (allegati, messaggi, proposte_documento, documenti, provenienze, relazioni,
    righe_componente_proposta): senza, la pertinenza non si calcola e gli orfani non ci sono (R-75 della revisione di
    V2), e un fascicolo calcolato direbbe «nessun orfano» senza saperlo. È più largo di F.2 («falso solo con T-12 sulle
    sezioni dei target»): scostamento [T] da ratificare (dubbio T-B6-86, allargato da T-B6-152). Anche senza grammatica il
    fascicolo non è calcolato, con lo stesso criterio della pertinenza (gli orfani non si calcolano: R-75), e il motivo
    resta `thread_non_valutato` (R-86 della controprova di V3, che chiude T-B6-152);
  - **il congelamento**: senza il gesto del modello nuovo (in A1c sempre: LD-23) `Congelato` è falso, con
    `gesto_non_registrato`, anche con una bom_versione congelata (R96 b B), che si mostra in `Legacy` con la versione
    corrente, l'ultima congelata e `Riaperta` per una bozza dopo una congelata (R96 c A). Con il gesto (sintetico, nelle
    prove: PO-31) `Congelato` è vero con chi e quando, il gesto resta, e il fascicolo mostra i conflitti, mai in silenzio
    (R96 [R]): `non_piu_congelabile`, `impronta_cambiata:<rif>` (anche per un target del gesto che oggi non c'è più:
    F0-07), `target_nuovo:<rif>`, in quest'ordine e poi per Rif. Lo stato del prodotto non cambia per il gesto. Con un
    gesto sintetico la regola non distingue un fatto da una prudenza (D-1 della revisione di V3): il fascicolo non
    calcolato dà `non_piu_congelabile`, e le impronte vuote per T-12 danno `impronta_cambiata` per i loro target. È
    prudente, perché non tace mai; la forma definitiva la decide A2, con l'adattatore del gesto;
  - **la diagnostica della deroga** (`documenti.deroga_non_sostituisce_2d`, T-B5-61, T-B6-31): resta una per prodotto,
    come i conflitti (F0-13), con il percorso della completezza di ognuno; nessuna diagnostica si fonde.
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; gli elenchi della fotografia si
  leggono in un ordine fisso, e la fotografia di chi chiama non cambia. Due esecuzioni sugli stessi ingressi, anche
  permutati, danno gli stessi byte canonici.
- **Valori che dichiarano altri pacchetti** e che `valutazione` ripete, perché non li esportano: gli enum del DB
  (aggancio, direzione, controparte, tipo e origine del componente, tipo del documento, natura dell'allegato), gli
  esiti di `v_step_prodotto`, il ruolo «delega» della marcatura, le capacità «struttura» e «grafo_completo» degli
  adattatori, l'elenco degli altri 3D del legacy; da B5, fase 2, il selettore del nome del file, i campi del codice e della revisione del cartiglio, la regola della radice che rappresenta il prodotto di
  `ancoraggio`; da B5, fase 3, i tipi di componente, il tipo documentale da_determinare, gli stati del NAS e gli esiti di
  `v_fascicolo`. Se cambiano, lo dicono le prove del pacchetto.

## Dipendenze

- **Importa:** `core/fotorfq` (la fotografia), `core/estrazione` (i documenti dei messaggi e dei file),
  `core/ancoraggio` (le proposte, i tipi della fonte e, da B5, le strutture, gli ancoraggi e la riconciliazione), `core/inbox/classificazione/motorea` (l'interpretazione, le
  letture, il confronto delle basi), `core/registro/regole/grammatica` (i codici «contratto.*» della porta stretta),
  `core/estrazione/evidenze` (il documento, le diagnostiche), `platform/jsoncanonico` (l'impronta delle decisioni),
  `github.com/google/uuid`, la libreria standard. Mai `core/confronto`, il caricatore, il DB, la libreria YAML (grafo
  del par.3.2.1).
- **Solo nei test:** il caricatore e `platform/testutil`, nella L4 sul database di prova.
- **È importato da:** ancora nessuno nel prodotto; da B6 il banco (P9), da A1d l'anteprima.

## Test

- **`ingressi_test.go`** — L1 — A1c-L1-21: il file `testdata/casi_acme.v1.json` letto per intero; ogni violazione
  della porta stretta e del contenuto con il suo codice e il suo percorso.
- **`richiesta_test.go`** — L1 — A1c-L1-17 riscritta: lo stato di ogni messaggio (gesto, direzione, controparte;
  il triage che non conta), lo stato del thread, le decisioni, il caso e il riconoscimento, la mail inoltrata senza
  confine senza caso, banco e anteprima con lo stesso file.
- **`target_test.go`** — L1 — PO-19 (parte B1), R70 A in tutte le sue forme, R75 A, `target.possibile_rinomina`,
  il codice non letto, senza grammatica, il determinismo con gli elenchi permutati.
- **`fonte_test.go`** — L1 — ogni riga della tabella di R65, il gesto 3 con la marcatura e nelle forme rare
  (T-B0-08), la delega, R76 b A con il nodo interno, T-B0-07, PO-34 con l'ordine dei motivi coppia per coppia, un PDF
  mai fonte, T-12, i valori del contratto.
- **`fonte_db_test.go`** — L4 (`-tags integrazione`, il database di prova) — la scena ACME scritta con SQL, letta con il
  caricatore: PO-19 sul DB, la fonte del finito dalla vista e dal gesto 3, la fonte dell'identificativo dallo STEP
  del thread; il database non cambia.
- **`scena_test.go`** — gli aiuti: la grammatica ACME, i messaggi, i fatti STEP.
- **`ancoraggi_test.go`** (B5) — L1 — il collegamento sulla scena ACME: la BOM di lavoro, la catena con le decisioni
  accanto, il codice manuale solo dall'operatore, le righe decise senza codice, i codici composti e dei messaggi, le
  associazioni decise e il 2D, il documento sostituito, la disponibilità di ogni file, gli archi con la decisione
  accanto; il determinismo, anche con due rimozioni aperte dello stesso arco permutate; la fonte con la regola del
  marcatore (T-B4-30, T-B2-02); il prodotto dello scenario con la sua lettura e con l'origine «scenario» in
  `HashTarget` (T-B5-06).
- **`bom_test.go`** (B5) — L1 — PO-03 (la regola e l'adattatore legacy), K-02 lettura A, PO-15 e le varianti
  dell'adattatore, la BOM di lavoro solo con la fonte confermata, il target senza componente (T-B0-07), i conflitti di
  gerarchia (la quantità, la somma per coppia, due esemplari dello stesso padre, la rimozione, l'ordine, niente sulle
  candidate), la precedenza del conflitto (anche per il target senza componente, T-B5-09), la gerarchia non vuota del
  modello nuovo, lo STEP arrivato due volte e il nodo mai proposto (T-B5-15), l'arco mai proposto (R-21), la somma
  parziale (T-B5-16), la gerarchia legacy senza la BOM di lavoro (T-B5-17), le relazioni confermate nel modello nuovo
  (T-B5-99), i valori e i campi del contratto.
- **`conflitti_test.go`** (B5) — L1 — PO-23 e PO-37 nelle parti di B5 (anche la variante della decisione tracciata), il
  codice manuale contraddetto, i doppioni dello stesso nodo, due 2D diversi sullo stesso nodo.
- **`scena_bom_test.go`** (B5) — gli aiuti: la famiglia ACME acme-catena, i fatti STEP con le formazioni e i PDF con
  il cartiglio, la scena della BOM con il segno di «Conferma l'albero».
- **`disegni_test.go`** (B5, fase 2) — L1 — la validità sulla regola e dai fatti di oggi (anche il documento senza
  allegato nel thread; l'anteprima del PDF che non si apre), i formati, PO-08, PO-09, PO-10, PO-11, PO-12, PO-22 nella
  parte di B5, il primario (con la validità nel criterio del formato) e le relazioni (il codice discordante, «a» e
  «A») sulla regola criterio per criterio, una voce per contenuto (la voce viva prima del documento non corrente) e lo
  stesso 2D in più componenti (T-B5-96), il determinismo, i valori e i campi del contratto.
- **`revisione2d_test.go`** (B5, fase 2) — L1 — PO-38 per intero: la proposta sulla regola e dalla fotografia (il figlio
  e la radice, `entita_non_univoca`, il candidato unico e discordante, il cartiglio senza revisione, il campo a sé
  della revisione, nessuna fonte), documento.rev registrata con la provenienza e la vista, la decisione sul documento
  con il conflitto `identita_documento`, il primario e `documento_componente`; i valori e i campi.
- **`disegni_confine_test.go`** (B5, fase 2) — L1 — i casi di confine della regola (un corrente non confermato,
  l'entità solo dallo STEP, il confronto senza la revisione del componente, il namespace e il marcatore, «a» e «A», la
  fonte senza coppia) e dell'adattatore (le fonti dell'identità in ordine, «assegna» di un file che non è un 2D, il nodo
  senza decisione, la radice scelta senza la base del prodotto, la voce più forte, il cartiglio letto dall'OCR e quello
  del worker di prima, il contenuto che l'adattatore non legge, i campi del cartiglio anche con la revisione
  facoltativa, documento.rev e la formazione di un altro componente o di una riga duplicato).
- **`scena_disegni_test.go`** (B5, fase 2) — gli aiuti: la famiglia ACME con la revisione negli id dello STEP, i fatti
  dei PDF e delle immagini, i documenti 2D.
- **`bom_db_test.go`** (B5) — L4 (`-tags integrazione`, il database di prova) — «Conferma l'albero» scritto con SQL,
  letto con il caricatore: la BOM verificata dall'adattatore legacy con la fonte non registrata, poi la quantità e la
  rimozione che portano la gerarchia in conflitto; il database non cambia.
- **`completezza_test.go`** (B5, fase 3) — L1 — sulla regola: PO-28 per intero con `ConfermaCategoria`, la
  classificazione, PO-33 e K-01 lettura A, la voce del 2D (PO-08 nella seconda fotografia, la deroga, lo scarto, il
  da_determinare, il formato non configurato), gli esiti della vista, il perimetro (T-B5-92) e PO-36, il determinismo,
  i valori e i campi del contratto.
- **`completezza_scena_test.go`** (B5, fase 3) — L1 — dalla fotografia: PO-36, le righe del perimetro (T-B5-91), il
  commerciale e le regole del cliente (PO-33), gli esiti della vista e il calcolo come la vista, la voce del 2D, i
  previsti, la classificazione con la famiglia della minuteria (PO-28), le sezioni assenti, i casi di confine (anche la
  rimozione aperta su un figlio raggiungibile per un'altra via, R-22), il prodotto non finito (R-23), il nodo con
  l'arco tolto (T-B5-67), il determinismo.
- **`scena_completezza_test.go`** (B5, fase 3) — gli aiuti: i fabbisogni di default, la vista calcolata come la calcola
  v_fascicolo, la famiglia ACME della minuteria.
- **`completezza_db_test.go`** (B5, fase 3) — L4 (`-tags integrazione`, il database di prova) — la L4 di equivalenza
  (T-E1R-03): tre RFQ con regole del cliente e di default scritte con SQL, lette con il caricatore; l'insieme
  (componente, tipo, bloccante) di A1c contro `v_fascicolo` con le esclusioni nominate E1-E5, la risoluzione delle
  regole di Go contro la vista, PO-33 come L4 sintetica, le deroghe; il database non cambia.
- **`disegni_confine_test.go`** contiene anche il ritocco T-B5-46: il testo non interpretato con un'altra maiuscola.
- **`valutazione_test.go`** (B6, V1) — L1 — A1c-L1-16: il percorso completo con quattro RFQ (valutata, senza voce
  nell'indice, scartata, ragione sociale discorde), un adattatore in errore che non ferma il thread, gli errori della
  valutazione (F0-05, T-B6-09), nessun insieme di regole, la fotografia non valida, i messaggi fuori RFQ (R34), il
  determinismo con gli ingressi permutati; A1c-L1-14 riscritta su `senza_testo` (T-B6-02), con la deviazione dal piano
  nominata; i record piatti del nuovo; i conflitti composti (T-B6-10, F0-13), anche sulla regola con pezzi sintetici.
- **`vecchio_test.go`** (B6, V1) — L1 — A1c-L1-30: la lettura del codice registrato (con la A, la B, la X, una maiuscola
  diversa, due spazi di codici, senza grammatica), il vecchio dal documento e dalla proposta, i dettagli, il documento
  sostituito e quello corrente, le revisioni confrontate con la grammatica con ogni motivo, il codice che non si legge;
  il vecchio dei prodotti (F0-18).
- **`revisione_vecchia_test.go`** (B6b, R113 B ratificata) — L1 — la colonna letta con la regola della famiglia (uguale,
  diversa, con gli spazi ai bordi), il codice con la revisione, concorde e discorde con la colonna (in tutti e due i
  versi), la colonna che non si legge accanto al codice, il codice che non si legge, la colonna che non si legge per
  stato (nessuna regola, non letta per intero, riservata, token sospeso, due regole), la provenienza e l'invariante
  «Revisione vuota se e solo se RevisioneDa vuoto», la versione della lettura nell'esito e nella sua impronta.
- **`scenario_test.go`** (B6, V1) — L1 — A1c-L1-20, la parte di valutazione: la struttura del prodotto (era
  `RadiceScenario`: T-B0-02) nell'esito, con i figli senza file e il nodo senza lettura.
- **`esito_test.go`** (B6, V1) — L1 — i valori e i campi del contratto dei tipi nuovi, le versioni fissate, i codici di V1
  (A1c-L1-32, la parte di valutazione), i record piatti fatti solo di tipi delle foglie (CP.2), nessun tipo dell'atteso
  raggiungibile dall'esito né dagli ingressi (R2).
- **`revisione_v1_test.go`** (B6, correzioni della revisione di V1) — L1 — R-61 (l'errore su un file, avviso, nel thread
  e nel messaggio fuori RFQ), R-62 (la radice che non si legge, con due target), R-63, R-66 e un caso per ognuna delle
  mutazioni vive di R-67 (i motivi e la disponibilità dei file di un thread non valutato, l'equivalenza delle revisioni,
  il token sospeso, il caso con il thread che elenca i suoi messaggi, l'ordine dei conflitti composti). R-64, R-65 e
  F0-19 sono in `vecchio_test.go`.
- **`risposte_test.go`** (B6, le correzioni dopo le risposte del 07/10) — L1 — R106 B, precisata: la contraddizione
  visibile (il file con un candidato nel secondo prodotto, nel messaggio che nomina solo il primo, con
  `valutazione.contesto_discorde` e niente altro che cambia), il file terminale (la decisione resta), il file terminale
  con un candidato nel prodotto nominato (si confronta con la sola decisione: T-B6-172, R-133 della revisione delle
  correzioni), il messaggio che nomina tutti e due con l'evidenza in uno (i riferimenti conservati, nessuna
  contraddizione), il messaggio che nomina due prodotti, nessuno collegato al file (tre target: R-133), le caratteristiche
  generiche del file (il tipo e la destinazione generale non sono evidenze); R107, precisata: la provenienza (il messaggio
  dell'allegato esterno anche per la voce di un archivio, le sole letture che nominano un target, la storia solo con il
  caso); R114, precisata: il marcatore della lettura del vecchio motore che non c'è. Il marcatore letto e quello vuoto
  sono anche in `vecchio_test.go`; gli invarianti del contesto e della diagnostica nell'aiuto `calcola`.
- **`revisione_v2_test.go`** (B6, correzioni della revisione di V2) — L1 — R-71 (il contesto con la formula
  dell'opzione A di R107, alla lettera: lettura [T], R107 precisata il 07/10, senza lettera; il corrente senza il gesto
  1, con la controparte sconosciuta e la direzione non nota; il caso che aggiunge la storia e non toglie il corrente; il
  corrente escluso), R-72 (il documento confermato fuori perimetro, il prodotto vuoto,
  l'«assegna» fuori perimetro, il conflitto anche al prodotto a cui il file è pertinente: R-78), R-73, R-74 (le due
  collocazioni), R-75 (T-12 su due sezioni, senza grammatica, la sezione solo della completezza), e un caso per ognuna
  delle dodici mutazioni vive di R-76. L'invariante degli orfani dell'aiuto `calcola` segue R-75.
- **`calcola_db_test.go`** (B6, V1) — L4 (`-tags integrazione`, il database di prova) — A1c-L4S-07, la parte di
  `Calcola`: la fotografia conservata ridà l'esito di prima dopo un'analisi nuova alla stessa terna; `Calcola` non parla
  con il database.
- **`scena_calcola_test.go`** (B6, V1 e V2) — gli aiuti: l'insieme delle regole ACME dalla porta del prodotto, i thread
  degli altri clienti, la fotografia con più thread, il messaggio fuori RFQ, la permutazione; gli invarianti di ogni esito,
  con quelli dello smistamento e, dopo le risposte del 07/10, del contesto. **`export_test.go`**: la composizione dei
  conflitti, le funzioni di R106 B e R108 A e l'attribuzione dei conflitti `identita_documento`, la funzione di R111 A,
  l'adattatore delle condizioni
  nuove e i motivi del nodo, per le prove della regola.
- **`smistamento_test.go`** (B6, V2) — L1 — la regola dello smistamento sugli ingressi astratti: le cinque condizioni con i
  motivi, T-B6-08, PO-24 (in_revisione con un percorso sintetico, che vince sul conflitto), PO-25 nella parte dello
  smistamento (anche il percorso `inbox_nuovo_cad` solo per i coinvolti), lo smistamento non calcolato (T-B6-07, F0-17,
  T-12), il determinismo; R106 e R108 sulle loro funzioni; l'attribuzione dei conflitti `identita_documento` (T-E1R-08).
- **`associazioni_test.go`** (B6, V2) — L1 — dalla fotografia, attraverso `Calcola`: PO-15 (parte B6), PO-04, PO-13,
  PO-14 e PO-39, PO-21 (parte B6), PO-25, PO-27, PO-32, PO-40; R106, R107 (anche la storia con e senza il caso) e R108
  sulla scena; la destinazione F8; le evidenze senza il ruolo «prodotto»; lo smistamento non calcolato; il determinismo.
- **`scena_smistamento_test.go`** (B6, V2) — gli aiuti: il prodotto altrimenti verificato, il secondo prodotto, i messaggi
  e i file con le proposte, la famiglia che legge la storia, le letture dell'esito.
- **`associazioni_db_test.go`** (B6, V2) — L4 (`-tags integrazione`, il database di prova) — PO-14, PO-27 e PO-39: lo
  scarto legacy scritto su `cockpit_test` come lo scrive il gesto «scarta», l'orfano con l'avviso, lo stesso orfano
  scartato; `Calcola` non parla con il database.
- **`prodotti_test.go`** (B6, V3) — L1 — lo stato del prodotto sulla regola condizione per condizione (PO-24 nella parte
  dello stato, T-E1-14, K-02, le voci certe dietro il perimetro aperto: R-81), PO-35 sui campi del tipo, PO-18 (anche
  PO-02 e PO-22 nella parte di B6), PO-25 e PO-19 nella parte dello stato; R-81 dalla fotografia (il 2D del finito mai
  analizzato dietro una rimozione aperta e dietro la fonte superata, con i controlli); R111 nelle due direzioni dalla
  fotografia e sulla funzione; l'adattatore delle condizioni nuove; il determinismo.
- **`fascicolo_test.go`** (B6, V3) — L1 — PO-31 sulla regola con il gesto sintetico, PO-01, PO-02 con e senza la
  bom_versione congelata, il thread non valutato e quello con l'errore, T-12 (anche sulle sezioni dello smistamento); la
  deroga ripetuta per il componente condiviso (T-B5-61).
- **`nodi_test.go`** (B6, V3) — L1 — i nodi solo della BOM di lavoro (T-B6-11), PO-20 nella parte di B6, `AncheIn`, gli
  archi tolti, la descrizione, la classificazione del nodo senza decisione, i motivi del nodo (anche sulla regola, con
  ingressi sintetici).
- **`impronta_prodotto_test.go`** (B6, V3) — L1 — l'impronta sulla regola e dalla fotografia (che cosa la copre e che
  cosa no, la fonte superata), T-12 (anche sulle sezioni della fonte: R-82), senza grammatica, il prodotto dello
  scenario, PO-26.
- **`scena_prodotti_test.go`** (B6, V3) — gli aiuti: gli invarianti di ogni prodotto e di ogni fascicolo, che gli aiuti
  `valuta` e `calcola` controllano su tutti i casi delle prove (PO-35, T-B6-09, T-B6-11; il composto e le precondizioni
  di `da_riesaminare`, R-84; il fascicolo non calcolato con T-12), e la «prova generale» della coerenza fra
  `FonteProdotto.Candidati` e `StrutturaProdotto.Fonti` (T-B2-02), con la sola eccezione dichiarata (R-83).
- **`prodotti_db_test.go`** (B6, V3) — L4 (`-tags integrazione`, il database di prova) — PO-01, PO-02 con e senza la
  bom_versione congelata, PO-19 e PO-31 in L4 sintetica; `Calcola` non parla con il database, e la lettura non lo cambia.
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go`; i codici nell'elenco d'oro (A1a-CAT,
  A1c-L1-32) in `core/estrazione/evidenze/codici_diagnostica_test.go`.
- I clienti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/ancoraggio/README.md`
- `internal/core/fotorfq/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/README.md`
