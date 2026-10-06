---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/valutazione` — il percorso puro del motore A: richiesta, target, fonte, strutture, verifica della BOM, disegni 2D e completezza documentale

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

## Non appartiene qui

- **Il DB**: la fotografia la legge `core/fotorfq/caricatore`, prima e altrove; qui niente DB, file, orologio, rete.
- **I candidati della mail**: li propone `core/ancoraggio` (`ProponiProdotti`); qui si chiamano, e non diventano mai
  target (R60 A). Lo stesso per i tipi della fonte (`RiferimentoFonte`, `DocumentoCandidato`, …), che stanno in
  `ancoraggio`.
- **Il confronto con le proposte attuali e con l'atteso**: `core/confronto` e `app/bancoa`. `valutazione` non importa
  `confronto`, il caricatore, la libreria YAML.
- **Lo smistamento** (l'asse 5), lo stato del prodotto, il fascicolo, l'impronta, `NodoBOM` (con `NodoBOM.Disegni` e
  `NodoBOM.Classificazione`), `Calcola` ed `Esito`: B6, nello stesso pacchetto, dopo.
- **L'adattatore della conferma della categoria** (`ConfermaCategoria`): il DB non ha la categoria (LD-19); la
  scriverà lo spazio di verifica (SV). Qui la regola si prova con conferme sintetiche. La distinta del PDF di assieme e
  lo STEP multibody come evidenza (RC-01) sono fuori dal contratto congelato: niente qui.
- **Le regole delle strutture, degli ancoraggi, della catena del codice e della riconciliazione**: `core/ancoraggio`.
  Qui si sceglie solo che cosa della fotografia entra, e si leggono i suoi segnali (le discordanze, `QuantitaDiscorde`).
- **Il `Conflitto` composto per l'esito** (con lo smistamento, l'associazione, il file nuovo) e la pertinenza dei
  documenti ai prodotti: B6. Qui escono i pezzi di nomenclatura e di gerarchia, per prodotto, e il pezzo
  `identita_documento` dalla regola `ValutaDisegno`, per i prodotti che il chiamante le dà.
- **L'adattatore delle decisioni sull'identità di un documento** (`DecisioneIdentita` con l'oggetto documento): il DB di
  oggi non le ha (LD-27); le scriverà lo spazio di verifica (SV). Qui la regola si prova con decisioni sintetiche.
- **La decodifica delle immagini, il cartiglio raster, l'anteprima comune** (LD-01, LD-03, LD-04): il worker e A1d.
- **Il significato delle codifiche dei clienti**: le grammatiche, lette da `motorea`.

## File

- **`valutazione.go`** — responsabilità:
  - commento `// Package`; `ValutazioneProdotti`, `ValutaProdotti`: i documenti dei messaggi, la richiesta,
    l'interpretazione, i candidati della mail, i target, la fonte di ogni target; da B5 gli ancoraggi del thread, lo
    stato della struttura e la verifica della BOM di ogni target, i conflitti, i 2D dei componenti, la completezza di
    ogni target e la classificazione dei componenti; le diagnostiche in ordine.
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
  - la regola `VerificaDellaBOM`, la gerarchia legacy senza la BOM di lavoro (`gerarchiaSenzaBOMDiLavoro`, T-B5-17) e
    la lettura A di K-02 (`bomSenzaFonteConfermataK02`);
  - l'adattatore legacy (`gestiDaConfermaLAlbero`), la struttura del prodotto (`strutturaDaVerificare`,
    `struttureDellaVerifica`), il perimetro della BOM confermata, `bom.fonte_non_registrata`.
- **`conflitti.go`** (B5, fase 1) — responsabilità:
  - `Conflitto`, `TipoConflitto`, `AsseConflitto`, `EvidenzaDecisione`, `EvidenzaProposta`, i motivi;
  - `ConflittiDellaNomenclatura`, i conflitti di gerarchia (quantità e rimozioni), l'ordine canonico.
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
    `bom.fonte_non_registrata`, `documenti.deroga_non_sostituisce_2d`.

## Entry point

- **`ValutaProdotti`** — chi lo chiama: da B6 `Calcola`, per ogni thread, nel banco (`app/bancoa`) e da A1d
  nell'anteprima (`transport/web`); in B1 le prove.
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
- **È importato da:** ancora nessuno nel prodotto; da B6 il banco, da A1d l'anteprima.

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
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go`; i codici nell'elenco d'oro (A1a-CAT,
  A1c-L1-32) in `core/estrazione/evidenze/codici_diagnostica_test.go`.
- I clienti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/ancoraggio/README.md`
- `internal/core/fotorfq/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/README.md`
