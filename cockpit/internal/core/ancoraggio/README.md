---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/ancoraggio` — le proposte del motore A: i prodotti chiesti dalla mail, le strutture dei prodotti, la catena del codice dei nodi, gli ancoraggi dei file e la riconciliazione con il cartiglio

## Scopo

- I **servizi di proposta** del motore A (giro 5; piano A, par.3.3.7, 6.4.4, 6.4.5): la classificazione dice che
  cosa significa un'evidenza, qui si decide a quali prodotti candidarla, conservando ambiguità, alternative,
  esclusioni e motivi. È puro: nessuna scrittura, nessun DB.
- In B1 (commit P5) c'è **`ProponiProdotti`**: dalle interpretazioni dei messaggi di una richiesta ai **prodotti
  candidati della mail**, con il codice richiesto originale separato dalla base letta, i qualificatori attribuiti,
  la quantità dalla colonna che la grammatica dichiara con l'evidenza della cella (R28), le letture escluse con il
  motivo, le diagnostiche, `HashIngresso` e `Impronta`.
- **Un candidato non è mai un target** (R60 A): è sempre proposto e «da confermare». Il target lo decidono il gesto
  2 dell'operatore (R70 A) o lo scenario del file dei casi (R75 A), e lo legge `core/valutazione`.
- Il **vocabolario comune** del contratto di A1c (§2.2): `Autorita`, `OrigineDato`, `StatoRichiesta`,
  `GestoMessaggio`, `RichiestaValutata`, `SegmentoPertinente`; e i **tipi della fonte strutturale** che
  `valutazione` usa per la fonte di un prodotto: `RiferimentoFonte`, `EsitoEstrazione`, `OrigineCandidato`,
  `DocumentoCandidato` (M1). Qui ci sono solo i tipi: il calcolo della fonte sta in `valutazione` (T-B0-04).
- In B2 (commit P6a) ci sono le **strutture** (6.4.5, regole 1 e 2; contratto §1.3, §2.2):
  - **`StrutturaDa`**: dal documento STEP dell'adattatore e dalla sua interpretazione, la **struttura del file**
    (`StrutturaFile`): radici, nodi con le letture d'identità, archi dei fatti con quantità e occorrenze, la
    completezza del grafo con il motivo. Solo lo STEP letto dà una struttura (T-E1-09, R68 A);
  - **`ProponiStrutture`**: per ogni prodotto target (`ProdottoRichiesto`) e il contesto (`ContestoStrutturale`:
    strutture, componenti e relazioni confermati, righe e archi aperti della tabella legacy), una **struttura del
    prodotto per STEP e radice** (`StrutturaProdotto`, era `RadiceScenario`), con `NodoProposto` e `ArcoProposto`
    (M2), gli archi del contesto con la loro origine, le righe da decidere, `GrafoCompleto`;
  - lo **stato** (`StatoStruttura`): tutto è `struttura_candidata` finché la fonte non è confermata (R59 A); solo
    la struttura sotto la radice scelta dello STEP confermato è `bom_di_lavoro_proposta`, nello stesso calcolo (R76 A,
    R85), e resta una proposta. Un target senza strutture è `nessuna` (`StatoStrutturaDelTarget`).
- In B4 (commit P6b, fase 1) ci sono **i nodi, l'identità e la catena del codice** (6.0.6, 6.4.5; contratto §1.4,
  §2.2, §2.5, §2.6), nello stesso calcolo di `ProponiStrutture`, anche sulla BOM di lavoro (R85):
  - la **catena del codice** di ogni `NodoProposto` (`CatenaCodice`): il grezzo con il suo localizzatore
    (`ValoreGrezzo`, I-6), la lettura scelta, l'**identità** (`IdentitaNodo`: base, marcatore, revisione, parziale,
    provenienza, qualità, motivo; R86, R87), il codice proposto dal compositore che valutazione passa nel contesto
    (`CodiciProposti`, T-08), il codice **manuale** di una riga aperta corretta dall'operatore (T-B0-33) e il codice
    **confermato** del componente legato al nodo (R31 c), con la provenienza della revisione (`CodiceDeciso`,
    T-B0-34, T-E1-22, E1R);
  - i **candidati di revisione** (`CandidatoRevisione`), ognuno con la sua entità, mai propagati ai figli (T-E1-06):
    il nome del file STEP per la radice del file, il codice del target confermato per il prodotto, i codici dei
    messaggi per i nodi con la stessa identità (una regola sola: T-B4-06 rivisto); una fonte senza revisione letta, o
    con un altro marcatore scritto, sta fra le `FontiSenzaRevisione`, con il motivo; lo `StatoRevisione`
    (`confermata` solo da una `DecisioneIdentita` sul componente);
  - le **decisioni accanto**, mai fuse (R95 A, T-E1-05): `NodoProposto.Decisione` per UUID (il componente della riga
    legacy decisa dello stesso nodo: emendamento E1 §4.2), `RigaLegacy`, `RigaDecisa`, `DecisoDaPersona`;
    `ArcoProposto.Decisione`, l'arco confermato accanto (sulla BOM di lavoro anche per gli archi della radice scelta:
    T-B4-23), con il segnale `QuantitaDiscorde`;
  - l'ingresso astratto **`DecisioneIdentita`** con le **`EvidenzaVista`** (E1R, T-E1R-08; LD-27), la funzione
    **`EvidenzaDa`**, l'unica forma della coppia fonte-valore (T-B4-22), ed `EvidenzaNuova`, che usano la
    riconciliazione (fase 3) e B5;
  - i tipi della riconciliazione (`CodiceDocumentale`, `CorrezioneProposta`, `EsitoRiconciliazione`); la regola è
    della fase 3.
- In B4 (commit P6b, fase 2) ci sono **gli ancoraggi dei file e la pre-associazione** (6.4.5, regole 3-8; par.3.3.7;
  contratto §1.5, §2.2):
  - **`ProponiAncoraggi`**: per ogni file (`FileInterpretato`, con la `Disponibilita` che dà valutazione, anche
    `senza_testo` per una scansione: T-B0-31), sopra le strutture di `ProponiStrutture`, chiamata dentro (T-B2-01), i
    **candidati** a livello prodotto (collocazione `radice`) e componente (`figlio`), con le radici del file e quelle
    raggiungibili, i padri, i percorsi, gli archi, le posizioni nelle strutture, le dimensioni e i conflitti
    (`CandidatoAncoraggio`, `PosizioneCandidato`, `DimensioneCompatibilita`); l'**associazione** (`nessun_candidato`,
    `candidato_unico`, `ambiguo`, `discordante`) e la **collocazione** (`radice`, `figlio`, `fuori_richiesta`,
    `non_determinabile`), con i motivi (`AncoraggioFile`); le impronte (`EsitoAncoraggi`: `HashIngresso`,
    `HashTarget`, `Impronta`) e le strutture con `SenzaFile` ricalcolato dai file candidati;
  - la **pre-associazione** (R85): prima della conferma della fonte i file si candidano sulle strutture candidate (R59
    A), dopo anche sui nodi della BOM di lavoro, nello stesso calcolo; le proposte non aspettano la nomenclatura e
    portano accanto l'incertezza dell'identità (`IdentitaParziale`, `MotiviIdentita`: T-E1-03, PO-20);
  - la **compatibilità con le decisioni** del nodo (codice confermato o manuale), ricalcolata a ogni fotografia: dopo
    una correzione il nodo e il componente restano gli stessi, e un file non più compatibile lo dice (T-E1-05, PO-21);
  - l'**abbinamento per base** di ogni nodo (`NodoProposto.AbbinamentoPerBase`; emendamento E1 §4.2, punto 2): senza
    la decisione per UUID, i componenti confermati con la stessa identità, come proposta; con la decisione, il motivo
    quando non è più compatibile o non si può verificare; con la riga decisa che porta un componente fuori dal contesto,
    `decisione_non_verificabile` con quel componente e l'abbinamento accanto con il suo motivo (T-B4-26); sul nodo
    scartato niente abbinamento, `nodo_scartato` (T-B4-27);
  - la **regola unica del marcatore** (T-B4-06 rivisto, T-B4-30): uguale, o scritto da una parte sola, è
    compatibile; scritto da tutte e due le parti e diverso è un'altra identità. Vale per le radici candidate, le
    identità raggiungibili, i candidati a livello prodotto (con `ProdottoRichiesto.Marcatore`) e componente, le
    identità discordanti di un file, la compatibilità con le decisioni e l'abbinamento per base;
  - la **radice scelta** della BOM di lavoro senza la base del prodotto (R76 A, T-B4-25): le sue identità danno il
    candidato a livello prodotto, con il motivo `radice_scelta` e la base discordante visibile nelle dimensioni.
- In B4 (commit P6b, fase 3) c'è **la riconciliazione fra lo STEP e il cartiglio** (6.0.6; workflow, passo 10 e il
  caso del §4; contratto §0 punto 4, §1.4, §2.2, §2.5, §2.6), dentro `ProponiAncoraggi`, dopo gli ancoraggi:
  - per ogni nodo con un **2D associato** (un file che valutazione segna come disegno, `FileInterpretato.Disegno`,
    candidato con una posizione sul nodo, o deciso sul componente del nodo: `AssociazioneDecisa`, manuale o
    confermata), un **`CodiceDocumentale`** in `CatenaCodice.Documentale`: la lettura di `cartiglio.codice` (oggi solo
    dai PDF: LD-04), l'origine dell'associazione e lo stato dell'associazione del file (T-B4-39), l'**esito**
    (`concorda`, `completamento_proposto`,
    `correzione_proposta`, `discordante`, `non_verificabile`, con il motivo) e la **correzione** accanto
    (`CorrezioneProposta`, con la provenienza e `RevisioneInferiore`: T-E1-20), mai applicata (R64 A, R87);
  - il cartiglio come **candidato di revisione** per l'entità del nodo (T-E1-06), con lo stato della revisione che ne
    segue;
  - i **segnali della discordanza con le decisioni** del nodo (`CodiceDocumentale.Discordanze`, uno per decisione
    contraddetta: il codice confermato e il codice manuale sono due decisioni, T-B4-40; `DiscordanzaDecisione`, con le
    evidenze dei due lati: T-E1-15): conflitto contro un codice confermato o manuale (T-B0-24; per il manuale anche sulla revisione,
    che l'operatore ha scritto: R95 A) e contro una `DecisioneIdentita` solo con un'evidenza nuova (T-E1R-08, R95 A);
    indicatore con l'evidenza già vista e contro `componente.rev` del legacy (R97 B, mai un conflitto). Sulla BOM di
    lavoro la radice scelta porta accanto il codice confermato del componente del target (T-B4-38), quindi anche il
    cartiglio del 2D del prodotto si confronta con la decisione. Un 2D solo proposto con l'associazione del file
    ambigua o discordante dà solo indicatori, perché forse non è del nodo (T-B4-39). Il `Conflitto` composto lo fanno
    B5 e B6;
  - le diagnostiche `ancoraggio.completamento_documentale` e `ancoraggio.correzione_documentale`, in avviso.

## Non appartiene qui

- **La fotografia e il DB**: `ancoraggio` non importa `core/fotorfq` né il caricatore (T-B0-04). La richiesta
  valutata, con i gesti, la compone `core/valutazione` dalla fotografia; qui arriva già fatta.
- **Il target, la sua identità e lo stato della fonte** (R70 A, R79, R65, R76 b, T-E1-09): `core/valutazione`
  (commit P7a).
- **Quale file è un 2D** (il tipo documentale, i formati di T-B0-30) e **le associazioni del DB** convertite in
  `AssociazioneDecisa`, **la lettura dei codici decisi** (`LettureDecise`): `core/valutazione`, che legge la
  fotografia. Qui arrivano già fatti.
- **La revisione proposta di un documento, le sue discordanze, `Disegno2D`, la parte documento di
  `DecisioneIdentita` e il conflitto `identita_documento`** (R104, T-E1R-05…09): `core/valutazione` (B5). La
  riconciliazione di qui è quella del nodo: il cartiglio contro l'identità proposta dallo STEP e contro la decisione
  del nodo.
- **La pertinenza, il perimetro, «da smistare», gli stati terminali, le associazioni del DB** (`AssociazioneFile`,
  `FileDaSmistare`, R93, T-E1-11, T-E1-12, T-B0-32), **il conflitto composto** e **la disponibilità calcolata dalla
  fotografia** (6.4.6 passo 8): `core/valutazione` (B6). Qui un file ha i suoi candidati qualunque sia il messaggio a
  cui è allegato, e un file non è mai scartato da un calcolo (R105).
- **Il compositore**: lo chiama solo valutazione (T-08); qui arriva il suo risultato, in `ContestoStrutturale.
  CodiciProposti`. **Il conflitto composto** (`Conflitto`, gli assi della nomenclatura e della gerarchia) e la parte
  documento di `DecisioneIdentita` sono di valutazione (B5, B6): qui ci sono le decisioni accanto e i segnali.
- **La verifica della BOM** (nomenclatura e gerarchia, R80), **i nodi della BOM con i 2D** (`NodoBOM`, T-B0-39) e
  lo stato della struttura per prodotto nell'esito (`ProdottoValutato.Struttura`): `core/valutazione` (B5, B6). Qui
  niente si dice verificato.
- **Il router e l'interpretazione**: `core/inbox/classificazione/motorea`. Qui non si rilegge nessun testo: si
  usano le letture, gli attributi e le funzioni che l'interpretazione dà. Le scelte dei segmenti (`UsoSegmenti`)
  le applica `Interpreta`: una storia non scelta dall'operatore o dallo scenario resta menzione (R48 A, E2).
- **Il documento dai fatti**: `core/estrazione`, che `ancoraggio` non importa (freccia solo nei test).
- **L'impronta dei gesti** (`RichiestaValutata.Decisioni`, T-16): la calcola chi compone la richiesta.
- **Il confronto con le proposte attuali e con l'atteso**: `core/confronto`, `app/bancoa`.

## File

- **`prodotti.go`** — responsabilità:
  - commento `// Package`; `VersioneServizio` (`ancoraggio-1`);
  - il vocabolario: `Autorita`, `OrigineDato`, `StatoRichiesta`, `GestoMessaggio`, `RichiestaValutata`,
    `SegmentoPertinente`;
  - `MessaggioInterpretato` (messaggio, documento, interpretazione: il documento dà il segmento di un'unità e la
    riga di una cella, che l'interpretazione non porta);
  - `ProponiProdotti`, `EsitoProdotti`, `CandidatoProdotto` con le sue `EvidenzaProdotto`, `Esclusione`, i motivi
    dei candidati
    (`da_confermare`, `richiesta_non_valutata`) e delle esclusioni (`menzione`, `funzione_non_richiesta`,
    `famiglia_senza_ruolo_prodotto`, `attributo_di_riga`);
  - le regole 1-7 del 6.4.4, la chiusura dell'esito con l'ordine canonico e le impronte.
- **`fonti.go`** — responsabilità:
  - `RiferimentoFonte` (il gesto 3: documento, sha256, radice, ruolo, forma, chi e quando, superato) con le
    costanti di `Tipo` e `Forma`;
  - `EsitoEstrazione`, `OrigineCandidato`, `DocumentoCandidato`: i tipi, con i loro commenti; nessuna funzione.
- **`struttura_file.go`** — responsabilità:
  - `FileInterpretato` (allegato, documento, interpretazione e, da B4, la disponibilità e il segno del 2D);
  - `RifNodo` e `PrefissoRifNodo`: il riferimento di un nodo, sha256 più chiave (T-E1-04);
  - `ArcoPercorso` con le origini `fatti`, `confermato`, `proposto`; `NodoStruttura` (con i grezzi e le formazioni,
    B4), `StrutturaFile` (con le letture del nome del file, B4);
  - `StrutturaDa`, con l'ordine canonico di radici, nodi, letture e archi.
- **`struttura_prodotto.go`** — responsabilità:
  - `ProdottoRichiesto` (con `ComponenteID` e `FonteConfermata`); `ContestoStrutturale`, `ComponenteDeciso`,
    `RigaPropostaLegacy` (il `NodoStrutturale` del 6.4.5, diviso: T-B0-02), `RifComponente`, `RifRigaProposta`; da
    B4 gli ingressi della catena e delle decisioni: `RigaDecisaLegacy` con i suoi stati, le righe decise, i codici
    proposti, i codici dei messaggi, le decisioni sull'identità, il codice manuale, la revisione e la lettura del
    componente; dalla fase 3 le letture dei codici decisi e le associazioni decise;
  - `StatoStruttura`, `NodoProposto` (con la catena e le decisioni accanto, B4, e l'abbinamento per base, B4 fase 2),
    `ArcoProposto` (con l'arco confermato accanto, B4), `StrutturaProdotto`;
  - `ProponiStrutture` (radici candidate, struttura, stato, diagnostiche; da B4 anche le decisioni e le catene dei
    nodi), `StatoStrutturaDelTarget`, i controlli di contratto.
- **`catena.go`** (B4, fase 1) — responsabilità:
  - i tipi della catena: `CatenaCodice`, `ValoreGrezzo`, `IdentitaNodo`, `CandidatoRevisione`, `FonteSenzaRevisione`,
    `CodiceDeciso`, con le costanti di provenienza, stato, fonte, campo e motivo; i tipi della riconciliazione
    (`EsitoRiconciliazione`, `CodiceDocumentale` con l'`Associazione` e le `Discordanze`, `CorrezioneProposta`), la cui
    regola è in
    `riconciliazione.go`;
  - gli ingressi `LetturaConPosizione`, `CodiceDiMessaggio` e la chiave `ChiaveCodiceProposto`;
  - il calcolo: la lettura scelta, il grezzo, l'identità, il codice proposto, manuale e confermato con la provenienza
    della revisione, i candidati con la loro entità.
- **`decisioni.go`** (B4, fase 1) — responsabilità:
  - `DecisioneIdentita` ed `EvidenzaVista` con le loro costanti; `EvidenzaDa`; `EvidenzaNuova`; `QuantitaDiscorde`;
  - dalla fase 2 `AbbinamentoPerBase` con i suoi motivi e la compatibilità fra l'identità di un nodo e il codice letto di
    un componente.
- **`ancoraggi.go`** (B4, fase 2) — responsabilità:
  - i due assi e la collocazione (`Disponibilita`, `Associazione`, `Collocazione`) con le loro costanti; i livelli, i
    motivi dell'ancoraggio e del candidato, le dimensioni; `RifIdentita` e `PrefissoRifIdentita`;
  - `EsitoAncoraggi`, `AncoraggioFile`, `CandidatoAncoraggio`, `PosizioneCandidato`, `DimensioneCompatibilita`;
  - `ProponiAncoraggi`: le identità raggiungibili delle strutture, le letture d'identità e le identità discordanti
    del file, i candidati con le loro dimensioni, l'associazione, la collocazione, `SenzaFile` ricalcolato, la
    riconciliazione (fase 3), i controlli di contratto dei file e delle associazioni decise, le impronte con il
    contesto in ordine canonico.
- **`riconciliazione.go`** (B4, fase 3) — responsabilità:
  - gli ingressi `AssociazioneDecisa` e il segnale `DiscordanzaDecisione`, con le costanti di parte, effetto e motivo;
    i motivi del codice documentale;
  - la riconciliazione (`riconcilia`, chiamata da `ProponiAncoraggi`): i 2D associati a ogni nodo con l'origine più
    forte, la lettura del cartiglio, il confronto con l'identità proposta dallo STEP (`confrontaConLoSTEP`), la
    correzione accanto, i candidati del cartiglio, la discordanza con la decisione, `RevisioneInferiore`
    (`revisioneInferiore`), lo stato della revisione (`statoRevisione`, che usa anche la catena), le diagnostiche.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che il pacchetto produce (R41 b): `ancoraggio.richiesta_non_valutata`,
    `ancoraggio.righe_stessa_base`, `ancoraggio.quantita_non_intera`, `ancoraggio.alternative_conservate`;
    `ancoraggio.target_senza_struttura`, `ancoraggio.grafo_incompleto` (le strutture);
    `ancoraggio.completamento_documentale`, `ancoraggio.correzione_documentale` (la riconciliazione, fase 3).

## Entry point

- **`ProponiProdotti`** — chi lo chiama: da A1c `core/valutazione` (`Calcola`, passo 5), per il banco e, da A1d,
  per l'anteprima.
- **`StrutturaDa`** — chi lo chiama: da A1c `core/valutazione`, per gli STEP del thread (`Calcola`, passo 7: il
  contesto).
- **`ProponiStrutture`** e **`StatoStrutturaDelTarget`** — chi li chiama: `ProponiAncoraggi` (le radici e i figli
  dove ancorare i file); da A1c `core/valutazione` lo stato della struttura per prodotto (B5, B6).
- **`ProponiAncoraggi`** — chi lo chiama: da A1c `core/valutazione` (`Calcola`, i file del thread: B6), che ne compone
  le associazioni, la pertinenza e «da smistare», e dalla riconciliazione dei nodi i conflitti di nomenclatura (B5,
  B6); `RifIdentita` — chi la usa: valutazione, per riconoscere il Target di un candidato su più STEP.
- **I tipi del vocabolario e della fonte** — chi li usa: `core/valutazione` (la richiesta del thread, il target con
  la sua autorità, la fonte di un prodotto con i documenti candidati); dal commit di B4 gli ancoraggi.
- **`EvidenzaDa`**, **`EvidenzaNuova`** e **`QuantitaDiscorde`** — chi li chiama: la riconciliazione (fase 3 di B4:
  `EvidenzaDa` per la coppia del cartiglio, `EvidenzaNuova` contro una decisione tracciata) e `core/valutazione` (B5,
  B6), per comporre i conflitti di nomenclatura e di gerarchia e il conflitto `identita_documento` (R95 A, T-E1R-08); `EvidenzaDa` anche chi produrrà le decisioni, perché le evidenze viste e
  quelle nuove si scrivono nella stessa forma (T-B4-22). **`ChiaveCodiceProposto`** — chi la usa: `core/valutazione`,
  quando mette i codici composti nel contesto (T-08).
- Oggi, nel codice di prodotto: `core/valutazione` usa i tipi e chiama `ProponiProdotti` (da B1, `ValutaProdotti`);
  `ProponiStrutture` la chiama `ProponiAncoraggi`; `StrutturaDa` e `ProponiAncoraggi` ancora non li chiama nessuno:
  lo faranno B5 e B6 in `core/valutazione`.

## Invarianti

- **Le regole di `ProponiProdotti`** (6.4.4, con R60 A):
  1. si considerano solo i messaggi di `RichiestaValutata.Messaggi`; un candidato senza evidenze da un messaggio
     con il gesto 1 (lo stato del suo `GestoMessaggio`, o quello del thread) ha il motivo `richiesta_non_valutata`,
     con `ancoraggio.richiesta_non_valutata`; il triage (`Evento`) è solo evidenza e non cambia niente (R29 c);
  2. entrano, una per una, le letture con funzione «richiesta» e il ruolo «prodotto» fra i ruoli candidati; una
     menzione, una famiglia senza il ruolo prodotto (R17 b) e la cella della colonna quantità sono esclusioni, ognuna
     col suo motivo: l'ammissione è per evidenza;
  3. la chiave del candidato è (namespace, base normalizzata, qualificatori attribuiti, riga di tabella), con la
     riga come (messaggio, entità riga), perché gli ID sono locali al documento: la stessa base in due righe dà due
     candidati con `ancoraggio.righe_stessa_base`; le letture in testo libero con la chiave di una sola riga sono
     evidenze di quella riga, anche da segmenti e messaggi diversi della richiesta; senza righe, o con più righe,
     fanno un candidato solo. La lettura principale: per una riga la sua cella; per il testo libero quella con la
     scelta più forte (pertinente da operatore o scenario, poi da riconoscimento, poi da valutare, poi sconosciuto),
     a parità messaggio e lettura con l'ID minore;
  4. la quantità è l'attributo «quantita» della stessa entità riga, se è un intero; altrimenti nil e
     `ancoraggio.quantita_non_intera`;
  5. attributi e relazioni non producono mai un candidato;
  6. letture di più famiglie o forme sulla stessa occorrenza restano tutte, con le altre in `Alternative` e
     `ancoraggio.alternative_conservate`: nessuna scelta per ordine;
  7. i candidati in ordine: il testo libero, poi le righe per (messaggio, tabella, riga), poi base, codice
     richiesto, namespace e lettura principale; `HashIngresso` e `Impronta` sul canonico.
- **Il principio della provenance** (5.0): un legame nasce solo se la provenance lo consente **e** una regola lo fa
  nascere. Qui le provenance sono la richiesta (i suoi messaggi), l'entità riga e la stessa unità, con la stessa
  identità letta dalla grammatica (namespace, base, qualificatori); le regole sono quelle del 6.4.4. Mai per
  vicinanza nel testo o somiglianza di stringhe. Il candidato cita ogni evidenza che lo sostiene, con il suo
  messaggio, segmento, uso e origine: la deduplica non cancella da dove viene un codice.
- **Gli ID delle letture sono locali al documento** (par.3.4.2: l'identità completa è (bundle, ID locale)): per
  questo evidenze, alternative ed esclusioni portano il `MessaggioID`, e la riga e l'evidenza della quantità di un
  candidato sono del messaggio della sua lettura principale.
- **L'origine si conserva**: ogni evidenza porta segmento, uso e origine della scelta (`operatore`,
  `riconoscimento`, `scenario`, "" senza selezione) come li ha dati `Interpreta` (v3 §10.2); il candidato riporta
  quelli della sua lettura principale.
- **Il riferimento della RFQ non è un prodotto**: in A1 non nasce nessuna lettura di riferimento
  (`riferimenti_rfq` riservato, R20 c), e nessuna famiglia lo legge come codice (C-31).
- **Le regole delle strutture** (6.4.5, regole 1 e 2; R59 A, R76 A, R85):
  1. radici candidate: per ogni target e ogni struttura, un nodo con una lettura d'identità compatibile con la base
     del target (stesso namespace; `ConfrontaBasi` uguale, o compatibile parziale; dalla fase 2 di B4 anche il
     marcatore compatibile con `ProdottoRichiesto.Marcatore`: T-B4-30) è una radice candidata; di solito
     la radice del file, anche un nodo interno (vale il suo sottoalbero: R76 b A); un nodo senza letture (le
     saldature, D10) si mostra e non lo è mai;
  2. la struttura: dalla radice, i nodi raggiungibili negli archi dei fatti, con i padri immediati, gli archi, le
     quantità e le occorrenze (FIGLIO-CONDIVISO: un figlio sotto due padri è un nodo solo, con i due archi); accanto,
     mai fusi, gli archi del contesto con la loro origine; la gerarchia viene solo dallo STEP (R68 A);
  3. lo stato: tutto candidato; con la fonte confermata (tipo step, sha256, radice registrata, non superata), solo
     la struttura sotto la radice scelta diventa BOM di lavoro proposta, al più una per prodotto, anche se la radice
     scelta non ha la base del target; radice non registrata, fonte superata, STEP non letto o radice che nel file
     non c'è: tutto resta candidato;
  4. la riga aperta della radice si mostra a parte e non conta fra le righe da decidere (R80); le fonti di una
     struttura ci sono solo se il file è un documento candidato della fonte del target (R76 b A, T-B0-07);
  5. `ancoraggio.target_senza_struttura` per un target senza strutture, `ancoraggio.grafo_incompleto` per una
     struttura con il grafo non completo; i file «solo parti» dei figli non contano.
- **Le chiavi non sono il codice** (T-E1-04): un nodo è sha256 più chiave (`RifNodo`), un componente il suo UUID,
  una riga legacy il suo UUID e, per il nodo, sha256 più chiave. Lo stesso contenuto in due allegati dà due
  strutture con gli stessi riferimenti: nessuna fusione (6.4.5, regola 7).
- **Confermato solo ciò che l'operatore ha confermato** (R29 e; R61 A): nel contesto un componente è confermato e
  una riga aperta è proposta, altrimenti è un errore di contratto; le relazioni confermate non promuovono le
  strutture.
- **`SenzaFile`**: `ProponiStrutture` non ancora file, quindi lo dice di ogni nodo; `ProponiAncoraggi` lo ricalcola:
  falso per un nodo con almeno un file candidato (anche ambiguo o discordante) e per la radice di una struttura che è
  la radice del suo STEP. L'assenza di un file è ammessa e senza diagnostica (3.3.7, P-20).
- **Le regole degli ancoraggi** (B4, fase 2; 6.4.5, regole 3-8; R59 A, R64 A, R85, T-B0-35, T-E1-03, T-E1-05,
  T-E1-07):
  1. le letture d'identità di un file sono quelle con la funzione `identita_file` (nome, voce d'archivio, codice o
     numero di disegno del cartiglio, id o nome di una radice STEP); le menzioni del testo e le relazioni no;
  2. candidato a livello prodotto: un target con la base di un'identità del file (stesso namespace, base uguale,
     equivalente o compatibile parziale, marcatore compatibile), con il ruolo prodotto; uno per target. Con la BOM di
     lavoro sotto una radice scelta senza la base del prodotto, anche un'identità del file che è quella della radice
     (motivo `radice_scelta`, la base discordante con il target fra le dimensioni e i conflitti: R76 A, T-B4-25);
  3. candidato a livello componente: un'identità raggiungibile (i nodi delle strutture dei target, la radice esclusa
     quando è il prodotto, per namespace, base, marcatore e revisione letta: un nodo senza marcatore o senza revisione
     sta con l'unico marcatore o l'unica revisione letta della sua base, altrimenti a sé), con il marcatore compatibile
     e il ruolo componente; un file con un altro marcatore scritto si abbina lo stesso ai nodi senza marcatore del
     gruppo, con le posizioni solo su quelli, e non finisce fuori richiesta (T-B4-06 rivisto, T-B4-30); uno per
     identità, con tutte le posizioni (FIGLIO-CONDIVISO); revisioni lette diverse dividono, una
     formazione STEP no; il Target è il componente deciso comune, altrimenti il nodo se è uno solo, altrimenti
     l'identità (`RifIdentita`). La chiave di un candidato è (Livello, Target), mai il solo Target (T-B4-28);
  4. nessun candidato debole (E-18); nessuna scelta a parità: più candidati sono `ambiguo`, in ordine di sostegno
     (meno conflitti, poi il livello prodotto, poi il Target), con i motivi (T-E1-07);
  5. identità discordanti nel file (due letture dello stesso soggetto con basi, due marcatori scritti o revisioni
     lette diverse, anche solo la revisione; il nome di uno STEP che non ha base e marcatore compatibili con nessuna
     radice; una base ripetuta che non concorda): `discordante`,
     con i candidati di tutte le identità (R64 A, T-B0-35, D2); radici diverse dello stesso STEP danno candidati
     distinti (C-21);
  6. collocazione dai livelli dei candidati (non determinabile se sono due); senza candidati `fuori_richiesta` solo con
     l'identità letta, i target noti e ogni target con strutture complete, altrimenti `non_determinabile` con i motivi
     (`target_ignoti`, `ancoraggio.grafo_incompleto`, `ancoraggio.target_senza_struttura`, `nessuna_lettura_identita`);
  7. le dimensioni (namespace, base, marcatore, revisione, qualificatori e la compatibilità con il codice confermato o
     manuale del nodo) restano visibili, con i conflitti; le dimensioni «radici» e «tipo documento» del 6.4.5 non si
     calcolano: le radici sono già campi del candidato, il tipo documentale è di valutazione (T-B4-29); l'origine di un
     candidato è sempre «proposto»; una posizione su un nodo scartato porta il motivo `nodo_scartato` (T-B4-27);
  8. un ancoraggio per allegato, mai fusi (HASH-CONFLITTO); `HashIngresso` sui file (allegato, bundle,
     interpretazione, disponibilità), `HashTarget` sui target e sul contesto in ordine canonico, `Impronta`
     sull'esito.
- **I limiti noti degli ancoraggi**:
  - la perdita di un abbinamento per base senza decisione non si vede: A1c non ha storia (R100), e un abbinamento che
    non c'è più non si distingue da uno che non c'è mai stato; con la decisione per UUID la perdita di compatibilità si
    vede sempre;
  - `Percorsi` elenca tutti i percorsi dalla radice al nodo: in un DAG con molti figli condivisi può crescere molto (il
    numero dei percorsi è quello delle occorrenze nella distinta esplosa); da guardare in B8, con il profilo dei limiti.
- **Le regole della catena del codice** (B4, fase 1; 6.0.6; R86, R87; T-B0-33, T-B0-34, T-E1-06, T-E1-22, E1R):
  1. la lettura: fra le letture d'identità del nodo, se a due a due danno la stessa identità (famiglia, namespace,
     base, revisione, e il marcatore compatibile con la regola unica: uno scritto da una parte sola va bene) vale quella
     del campo id, poi l'ID minore; se no nessuna, con `letture_discordanti`; senza letture `nessuna_lettura` (T-B4-01,
     T-B4-06 rivisto, T-B4-30);
  2. il grezzo è il campo della lettura scelta, o l'id (poi il nome), com'è nei fatti (I-6);
  3. l'identità ha le parti della lettura; la revisione nil vuol dire «non determinata» e non si inventa mai;
     `Parziale` viene dal motivo `revisione_non_determinata` del compositore (T-B3-01); il motivo dell'identità è il
     primo fra lettura assente o discordante, `lettura_non_completa`, `revisione_ambigua`, `revisione_non_determinata`,
     e `non_composto` quando manca il codice composto e la lettura non ha la revisione (T-B4-02);
  4. il codice proposto è quello del contesto per (bundle, lettura) (T-B4-05, ratificata); senza, nessuna stringa con
     `non_composto`;
  5. il manuale viene solo dal codice dell'operatore di una riga aperta (T-B0-33); il confermato solo dal componente
     della decisione per UUID, e sulla `bom_di_lavoro_proposta` dal componente del target per la radice scelta senza
     una decisione propria, che rappresenta il prodotto (T-B4-38, R76 A: la `Decisione` della radice resta nil, T-B4-23;
     sulla struttura candidata niente); con una `DecisioneIdentita` sul componente vale la decisione, `decisione_tracciata` e
     lo stato `confermata` (E1R, R95 A): il codice deciso c'è sempre (vuoto: errore di contratto), la revisione vuota
     è decisa e dà `Rev` nil (T-B4-21); altrimenti `componente.rev` con `coincide_con_formazione_step` se è uguale,
     senza gli spazi ai bordi, alla formazione `rev_grezza` (non alle alternative) di almeno un nodo legato al
     componente da una riga decisa (T-E1-22, alla lettera), se no `non_registrata`, e mai lo stato `confermata` (R97 B);
  6. i candidati, con una regola sola per il nome del file, il codice del target e i messaggi (T-B4-06 rivisto): lo
     stesso namespace, la stessa base completa e il marcatore compatibile (uguale quando c'è da tutte e due le parti,
     oppure scritto da una parte sola; il marcatore del target è `ProdottoRichiesto.Marcatore`); due marcatori scritti
     e diversi danno `marcatore_diverso` fra le fonti senza revisione (T-B4-03, ratificata). Il nome del file STEP solo
     per una radice del file (T-B0-27, T-B0-35); il codice del target confermato solo sulla radice della struttura; i
     codici dei messaggi per ogni nodo con la stessa identità, uno per entità, mai una scelta; l'entità è il prodotto
     per la radice della struttura, il componente per un nodo deciso, se no il nodo (T-E1-06); niente passa ai figli,
     niente decide. La stessa regola vale per il candidato a livello prodotto degli ancoraggi e per l'abbinamento per
     base;
  7. le decisioni accanto: la riga legacy del nodo si cerca per (allegato, RifNodo); la decisione è il componente di
     una riga decisa confermata o duplicato, se è fra i componenti del contesto. Anche un duplicato automatico, senza
     chi l'ha deciso, dà una Decisione (la lettera di E1 §4.2): B5 e B6 guardano `DecisoDaPersona` (T-B4-24). L'arco
     confermato accanto è la relazione fra le decisioni dei due nodi; sulla `bom_di_lavoro_proposta`, per gli archi
     della radice scelta con il gesto 3 senza una riga decisa, il padre è il componente del target
     (`ProdottoRichiesto.ComponenteID`), e la `Decisione` della radice resta nil; sulla struttura candidata niente
     (T-B4-23). Una decisione resta il valore corrente; il conflitto lo compone valutazione;
  8. `QuantitaDiscorde` è per arco: due nodi decisi come lo stesso componente sotto lo stesso padre hanno ognuno la
     stessa relazione accanto, e il segnale vale su tutti e due anche se la somma delle quantità coincide; B5
     confronta la somma per coppia di componenti;
  9. la coppia di un'evidenza vista si costruisce con `EvidenzaDa`, dal testo grezzo della fonte senza gli spazi ai
     bordi: il cartiglio com'è, il grezzo dell'id del nodo, il nome del file, «codice» o «codice rev» del documento;
     `EvidenzaNuova` confronta in quella forma; una fonte fuori elenco è un errore di contratto (T-B4-22);
  10. l'uscita non condivide memoria con l'ingresso: le forme, le basi, le letture e i localizzatori si copiano in
     profondità.
- **Le regole della riconciliazione** (B4, fase 3; 6.0.6; workflow, passo 10; R64 A, R87, R95 A, R97 B; T-B0-11,
  T-B0-24, T-B0-27, T-B0-35, T-E1-06, T-E1-15, T-E1-20, T-E1R-08; le letture T-B4-31…T-B4-40):
  1. un 2D è associato a un nodo di una struttura quando un suo candidato ha una posizione sul nodo (origine
     `proposto`), o quando un'associazione decisa lo lega al componente del nodo: la decisione per UUID del nodo e, per
     la radice che rappresenta il prodotto, il componente del target (`manuale`, `confermato`, con il documento); vale
     l'origine più forte (confermato, manuale, proposto), e accanto lo stato dell'associazione del file, quello del suo
     ancoraggio (T-B4-39); un file che valutazione non segna come 2D non ha codice documentale;
  2. il codice documentale è la lettura di `cartiglio.codice` (funzione `identita_file`) nel namespace dell'identità
     del nodo, scelta come per i nodi (la stessa identità, l'ID minore; più identità: `discordante`,
     `letture_discordanti`); senza letture `non_verificabile` (`cartiglio_non_letto`: un raster, un PDF senza testo,
     LD-04); senza identità del nodo o con il cartiglio letto solo in un altro namespace `non_verificabile`, con la
     lettura mostrata;
  3. il confronto con l'identità proposta dallo STEP, con `ConfrontaBasi`, la regola unica del marcatore e
     `ConfrontaRevisioni`, mai le stringhe: un cartiglio non intero (anche una base ripetuta che non concorda) o basi
     non confrontabili, `non_verificabile`; un'altra base, la base del nodo parziale, un altro marcatore scritto o
     un'altra revisione, `correzione_proposta`; base e marcatore che concordano e la revisione che solo il cartiglio
     dà, `completamento_proposto`; la stessa revisione o nessuna, `concorda`; la revisione dello STEP senza quella del
     cartiglio, o con quella del cartiglio sospesa, `non_verificabile`;
  4. il file con identità che non concordano (associazione `discordante`) e un codice del cartiglio: `discordante`,
     `identita_discordanti`, con la correzione accanto se c'è (R64 A); associazione e riconciliazione restano due
     campi (T-B0-11);
  5. la correzione porta il codice letto, la base, la revisione, il file, l'unità e il localizzatore; niente si
     applica: la catena, le decisioni e gli ancoraggi restano quelli di `ProponiStrutture` e degli ancoraggi;
  6. il cartiglio di un 2D associato è un candidato di revisione per l'entità del nodo con la regola unica
     dell'identità (stessa base completa, marcatore compatibile, revisione letta); senza revisione letta, o con un
     altro marcatore scritto, una fonte senza revisione con il motivo; un'altra base non è un indizio;
  7. le discordanze con le decisioni del nodo, una per decisione contraddetta, in ordine (il codice confermato, anche
     quello del prodotto sulla radice scelta, poi il codice manuale: sono due decisioni, e nessuna precedenza ne nasconde
     una, T-B4-40): il codice si confronta
     con la lettura del codice deciso (del componente, `LettureDecise` per una decisione tracciata con un altro codice,
     la lettura manuale), la revisione decisa con quella letta del cartiglio, senza gli spazi ai bordi; una revisione
     decisa assente discorda solo con la decisione tracciata (T-B4-21). Conflitto: codice confermato contraddetto;
     codice manuale contraddetto nel codice o nella revisione (l'ha scritto l'operatore: R95 A, T-B4-34); decisione
     tracciata con un'evidenza nuova. Indicatore: decisione tracciata con l'evidenza già vista; `componente.rev` del
     legacy (R97 B). Un 2D solo proposto con l'associazione del file `ambiguo` o `discordante` dà sempre un
     indicatore, con il suo motivo: forse il disegno non è del nodo (T-B4-39); con un'associazione decisa o un
     candidato unico proposto vale la regola di sopra (PO-23). Con chi e quando solo per la decisione tracciata; la
     coppia dell'evidenza viene dal testo grezzo del campo del cartiglio, non dalla lettura (T-B4-22);
  8. `RevisioneInferiore`: la revisione del cartiglio è minore di quella decisa, solo quando l'ordine è certo (tutte e
     due numeri, gli zeri a sinistra non contano) e il codice non è un altro; altrimenti falso; nella correzione è vero
     se lo è per almeno una delle decisioni;
  9. una diagnostica per ogni codice documentale con la correzione accanto: `completamento_documentale` quando il
     cartiglio aggiunge solo la revisione, `correzione_documentale` altrimenti.
- **I limiti noti della riconciliazione**:
  - sulla struttura candidata la radice non porta il codice del prodotto (R59 A, T-B4-38): la discordanza fra il
    cartiglio e una decisione sul prodotto la vede solo la BOM di lavoro; il codice manuale della riga aperta della
    radice sì, sempre;
  - le revisioni non numeriche non si ordinano: `RevisioneInferiore` resta falso;
  - la revisione di un campo a sé del cartiglio (`cartiglio.revisione`) non è il codice documentale: non si legge
    qui.
- **Per B5 e B6, prima di comporre il `Conflitto`**: lo stesso nodo può stare in più strutture (le strutture di due
  target, la struttura candidata e la BOM di lavoro, lo stesso contenuto in due allegati), e ognuna porta il suo codice
  documentale: la stessa discordanza (lo stesso nodo o componente, lo stesso 2D, la stessa decisione, la stessa parte)
  arriva più volte e va tolta come doppione; lo stesso vale per le diagnostiche della riconciliazione, che sono per
  struttura. Lo stato dell'associazione accanto (`CodiceDocumentale.Associazione`) e l'effetto dicono quanto è certo
  che il disegno sia del nodo.
- **T-B2-01**: `ProponiStrutture` e `StatoStrutturaDelTarget` sono il punto d'ingresso di B2; B4 li chiama dentro
  `ProponiAncoraggi`.
- **T-B2-02**: per la fonte strutturale (asse 2) fanno fede i candidati di valutazione (`FonteProdotto.Candidati`,
  B1); `StrutturaProdotto.Fonti` è una vista per struttura con lo stesso criterio (R76 b), e B6 ne proverà la coerenza.
- **Fonte confermata senza BOM di lavoro** (STEP confermato non letto, radice assente nel file): `ancoraggio` non lo
  dice; lo spiega il `MotivoFonte` di valutazione (B5, B6).
- **L'errore è solo di contratto** (`*evidenze.ErroreContratto`, con i codici della foglia
  `documento.riferimento_pendente`, `documento.id_ripetuto` e, per le strutture, `documento.enum_ignoto`): un
  messaggio ripetuto, un documento che non è quello del messaggio, un'interpretazione di un altro documento, una
  lettura o un attributo su un'unità che non c'è, un gesto ripetuto; per le strutture un target ripetuto, senza Rif
  o con l'autorità proposta, una fonte confermata che non è uno STEP, una struttura senza allegato o sha256, un nodo
  senza chiave, una riga senza allegato, sha256 o chiave (fuori dal suo file la chiave non significa niente: T-E1-04),
  un nodo o un arco incoerente con la sua struttura, un componente non confermato, una riga non proposta, un arco del
  contesto con l'origine o gli estremi sbagliati; per gli ancoraggi un allegato ripetuto o senza ID, un documento di
  un'altra fonte, un'interpretazione di un altro documento, una lettura su un'unità che non c'è, una disponibilità
  fuori elenco, la struttura di un altro contenuto dello stesso allegato; per la riconciliazione un'associazione decisa
  con un'origine che non è manuale né confermato, senza allegato o componente, di un allegato che non è fra i file,
  confermata senza documento o ripetuta, e la lettura di un codice deciso senza una decisione sul componente. Un limite
  dei dati sta nell'esito, con le diagnostiche o i motivi.
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; nessun ordine che dipenda da una
  mappa; messaggi, richiesta e gesti in ordine diverso danno gli stessi byte canonici, e così target, strutture,
  nodi, archi, componenti e righe per le strutture, e righe decise, codici dei messaggi e decisioni sull'identità per
  la catena (i candidati in ordine canonico, senza doppioni), e file (con le loro letture), target e contesto per gli
  ancoraggi, con le associazioni decise per la riconciliazione (i codici documentali in ordine di allegato). `time` serve solo per i tipi (anche `DecisioneIdentita.Il`)
  e per portare in UTC al millisecondo i tempi dei gesti nell'impronta (par.3.4.3).
- `VersioneServizio` e i valori delle costanti del contratto si cambiano solo con un commit che lo dichiara; la
  prova che li fissa si riscrive con «Riscritta per …».

## Dipendenze

- **Importa:** `core/inbox/classificazione/motorea` (le interpretazioni e le letture), `core/registro/regole/grammatica`
  (i ruoli), `core/estrazione/evidenze` (il documento, le diagnostiche), `platform/jsoncanonico` (le impronte),
  `github.com/google/uuid`, la libreria standard. Mai `core/fotorfq`, `core/estrazione`, `core/valutazione`,
  `core/confronto`, il caricatore, il DB (grafo del par.3.2.1).
- **Solo nei test:** `core/estrazione` e `core/fotorfq`, per costruire il documento della mail con
  `estrazione.DaMessaggio` e quello degli STEP con `estrazione.DaAllegato`, e arrivare a `ProponiProdotti` e a
  `StrutturaDa` dall'adattatore. Nessun ciclo: `core/estrazione` non importa `ancoraggio`.
- **È importato da:** `core/valutazione` (da B1: i tipi e `ProponiProdotti`; le strutture da B5 e B6).

## Test

- **`prodotti_test.go`** — livello L1 — che cosa copre, dall'adattatore della mail a `Interpreta` e a
  `ProponiProdotti`, sulle fixture sintetiche:
  - A1c-L1-05: la tabella a quattro righe con il riferimento nel corpo: quattro candidati in ordine di riga, il
    codice con la P separato dalla base, la fase prototipo, la quantità 5 con l'evidenza della cella, nessun
    doppione fra testo e HTML; la quantità che segue la sua riga, il codice dell'oggetto come evidenza della riga,
    la stessa base in due righe, la quantità non intera;
  - A1c-L1-06, riscritta (R60 A, R70 A, R75 A): l'uso sconosciuto senza prodotti dalla storia; la scelta
    dell'operatore o dello scenario con l'origine conservata; il riconoscimento, la scelta da valutare o esclusa
    che non promuovono; lo stesso codice in due segmenti o in due messaggi (un candidato con le evidenze di
    tutti), la menzione di una riga in un altro messaggio; le esclusioni (menzione, famiglia solo componente, cella della
    quantità); l'inoltro senza confine (R48 A); la richiesta non valutata e il gesto del messaggio; solo i messaggi
    della richiesta; nessun candidato è mai un target;
  - la regola 6 (alternative), il determinismo, gli errori di contratto, i valori e i campi del contratto.
- **`struttura_test.go`** — livello L1 — che cosa copre, dai fatti STEP del worker all'adattatore, a `Interpreta`,
  a `StrutturaDa` e a `ProponiStrutture`, sulle fixture sintetiche:
  - la struttura del file: radici, nodi con le letture, archi con quantità e occorrenze, il figlio sotto due padri,
    la saldatura senza letture, il grafo incompleto con il motivo, due radici; il Rif sha256 più chiave anche con
    lo stesso codice in due file, lo stesso contenuto in due allegati; niente struttura da un PDF, un IGS, uno STEP
    senza fatti o illeggibile, un'interpretazione di un altro documento; le guardie di `StrutturaDa` una per una, su
    un documento con i nodi (capacità «struttura» assente o non disponibile, qualità «errore», sha256 vuoto, fatti
    non di un file, nessun nodo);
  - la regola 1 con una radice compatibile parziale (la base a punti ACME) e con un nodo di un altro namespace con la
    stessa base, che non è radice candidata;
  - A1c-L1-07, -08, -09 e -20 nelle parti delle strutture, sulla scena ACME a due assiemi chiesti e uno no: una
    struttura candidata per target, il figlio in comune in due file, i pezzi «solo parti» che non contano; il grafo
    incompleto, il target senza STEP e quello non letto; il DAG a due livelli; figli senza file e nodi senza lettura;
  - PO-05 (parte B2) e R59: senza fonte confermata tutto candidato; con il gesto solo la struttura sotto la radice
    scelta è BOM di lavoro, con la stessa gerarchia; radice diversa, secondo STEP, stesso contenuto in un altro
    allegato restano candidati; radice non registrata, fonte superata, STEP non letto: nessuna BOM; la radice
    scelta senza la base del prodotto;
  - il prodotto come nodo interno; il contesto con l'origine e la riga della radice esclusa; il determinismo; gli
    errori di contratto, con il percorso dove serve (anche strutture e righe senza allegato, sha256 o chiave); i
    valori e i campi del contratto (riscritta per B4 con i campi nuovi).
- **`catena_test.go`** — livello L1 — che cosa copre, sulla famiglia ACME acme-catena (un nodo che legge base e
  marcatore senza revisione, mentre la forma del cartiglio la chiede), con i codici composti come li passerà
  valutazione:
  - la catena di ogni nodo: il grezzo invariato, la lettura, l'identità completa e parziale (nessuna revisione
    inventata, nessuna stringa, motivo `revisione_non_determinata`), il proposto, il manuale di una riga aperta, il
    confermato del componente; due letture discordanti; senza il codice composto (motivo `non_composto`);
  - i candidati con la loro entità: il nome del file solo per la radice (anche discordante o non letto), il codice
    del target per il prodotto (non lo scenario, non un figlio con la stessa base), i messaggi per la stessa
    identità, due entità della stessa base con due candidati, nessuna propagazione; la regola unica dell'identità
    (marcatore compatibile da una parte sola, `marcatore_diverso`);
  - la provenienza della revisione registrata (`non_registrata`, `coincide_con_formazione_step` con almeno una riga
    legata, senza revisione, solo dai nodi legati e solo da `rev_grezza`) e la decisione tracciata con lo stato
    `confermata`, anche senza revisione; R97 B;
  - le letture dello stesso nodo compatibili per la regola unica del marcatore, anche quando la scelta è quella senza
    marcatore, e discordanti a due a due; gli indizi in un altro spazio di codici (messaggi, nome del file), la revisione
    sospesa nel nodo, i doppioni dei candidati;
  - `EvidenzaDa` ed `EvidenzaNuova`; la decisione del nodo per UUID dopo la rinomina e la riga aperta corretta (la
    parte B4 di PO-21), la riga di un altro allegato (anche il codice manuale), la riga scartata, il componente fuori
    dal contesto; l'arco con la decisione accanto e le quantità diverse, gli archi della radice scelta sulla BOM di
    lavoro e il codice confermato del prodotto accanto alla sua radice (T-B4-38); la parzialità solo dal motivo del compositore; l'entità del prodotto prima del componente; l'uscita senza
    memoria condivisa; le identità parziali sulla BOM di lavoro; il determinismo; gli errori di contratto; i valori e
    i campi.
- **`ancoraggi_test.go`** (B4, fase 2) — livello L1 — che cosa copre, dai fatti STEP e PDF del worker all'adattatore,
  a `Interpreta`, a `StrutturaDa` e a `ProponiAncoraggi`, sulla scena ACME delle strutture con i PDF dei pezzi:
  - A1c-L1-07…13 e -15 nelle parti dei file: radice, figlio condiviso e non, fuori richiesta con il grafo completo;
    non determinabile con i motivi (grafo incompleto, target senza struttura, target ignoti, file senza lettura); il
    DAG a due livelli e le revisioni discordanti (e una formazione che non divide, un altro marcatore che non è la
    stessa identità); «SPECCHIATO DI» e il particolare simile; HASH-CONFLITTO con identità discordanti; A-C11 (anche con
    un contesto diverso); A-C07 (forma parziale, due radici, base ripetuta discordante); il determinismo;
  - lo STEP e il nome del suo file (T-B0-35); un file fra i target e fra i figli; PO-20 e PO-21 nella parte di B4; la
    pre-associazione sulla BOM di lavoro (R85); uno STEP in un messaggio qualsiasi; l'abbinamento per base (E1 §4.2);
    la disponibilità; gli errori di contratto; i valori e i campi;
  - la regola unica del marcatore in tutti i punti (a livello prodotto e componente, il nome dello STEP e il cartiglio,
    le identità raggiungibili, la compatibilità con le decisioni, le radici candidate: T-B4-06 rivisto, T-B4-30), anche
    per un file con un altro marcatore e i nodi senza marcatore del gruppo; la radice scelta senza la base del prodotto
    (T-B4-25); la riga decisa con un componente fuori dal contesto (T-B4-26) e il nodo scartato (T-B4-27); i ruoli della
    lettura; le regole che restavano senza prova (T-B0-35, T-E1-07, T-14, A-C07);
  - riscritta per la fase 3: il controllo di ogni esito confronta le strutture con quelle di `ProponiStrutture` senza
    la riconciliazione, e ne controlla gli invarianti.
- **`riconciliazione_test.go`** (B4, fase 3) — livello L1 — che cosa copre, dai fatti STEP e PDF del worker
  all'adattatore, a `Interpreta` e a `ProponiAncoraggi`, sulla famiglia ACME acme-catena:
  - gli invarianti di ogni esito (`controllaRiconciliazione`, chiamata da ogni prova degli ancoraggi);
  - il caso del workflow (§4): t2 e PO-07 (completamento proposto, mai applicato; il cartiglio fra i candidati del
    prodotto; T-B0-35 sul nome dello STEP), t2′ (la revisione 2 accanto al nome del file con la 1; il nome del PDF che
    discorda: associazione e riconciliazione discordanti, il completamento accanto), t2″ (il codice manuale della
    radice resta, con il segnale del conflitto), t2T (un TIFF e una scansione: `non_verificabile`); un file che non è
    un 2D;
  - i cinque esiti e i motivi (anche con letture costruite a mano: una revisione sospesa, una base non completa o non
    confrontabile, due letture diverse dello stesso cartiglio), l'entità del candidato del cartiglio, un altro
    marcatore scritto, un altro spazio di codici, il nodo senza identità, la base ripetuta che non concorda;
  - l'origine dell'associazione (proposta, manuale, confermata con il documento, sul prodotto attraverso la radice);
  - PO-23 e PO-37 nella parte di B4, con la variante di E1R (decisione tracciata, evidenza vista e nuova, revisione
    registrata come indicatore, `RevProvenienza`); la revisione del codice manuale come conflitto; `RevisioneInferiore`
    (T-E1-20); la lettura del codice deciso;
  - un 2D solo proposto e ambiguo o discordante: indicatore, mai conflitto; confermato sul componente: conflitto
    (T-B4-39); due decisioni sulla radice scelta (il prodotto e il manuale), due segnali e `RevisioneInferiore` da
    tutte e due (T-B4-40); il campo del cartiglio più lungo del codice letto, con la coppia dal testo del campo
    (T-B4-22);
  - t2″ sulla BOM di lavoro con il prodotto rinominato: la radice scelta con il codice confermato del prodotto accanto,
    il conflitto con il cartiglio, anche con la decisione tracciata; niente sulla struttura candidata (T-B4-38); il
    candidato del 2D senza il confronto con la radice scelta quando la radice ha la base del prodotto (T-B4-25);
  - il determinismo, gli errori di contratto, i valori e i campi.
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go`; i codici nell'elenco d'oro
  (A1a-CAT, A1c-L1-32) in `core/estrazione/evidenze/codici_diagnostica_test.go`.
- I clienti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/core/estrazione/README.md`
- `internal/core/estrazione/evidenze/README.md`
- `internal/README.md`
