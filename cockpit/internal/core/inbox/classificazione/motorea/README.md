---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/inbox/classificazione/motorea` — il motore A: compilatore, riconoscimento per forma, router, interpretazione, compositore e revisione registrata

## Scopo

- Il **motore A** del giro 5 (R3, R40 b). In A1a fa due cose:
  - **compila** la grammatica v1 di un cliente (`grammatica.SnapshotRegole`) in un `Motore` immutabile e ne
    **verifica tutti gli esempi** con lo stesso riconoscimento (`CompilaVerificato`); per tutti i clienti
    dell'indice, con i controlli di file, sha256 e cliente (`CompilaInsieme`, `MotoreDi`);
  - **riconosce le forme** su un testo, per un selettore: ogni coppia (famiglia, forma) ha le sue regex,
    compilate una volta, mai tutte in alternanza (v3 §4.3), e restituisce le **letture di forma** con gli intervalli in byte
    (`Riconosci`, `LetturaForma`).
- Confronta basi e revisioni senza inventare niente (`ConfrontaBasi`, `ConfrontaRevisioni`): una base
  parziale resta compatibile con tutti i suoi completamenti (A-C07), una revisione si confronta solo con le
  equivalenze dichiarate.
- Da A1b.9, il **router** (R23; piano A, 5.4.6), `router-2` dalla correzione di A1b.10: la tabella fissa e
  versionata che dal selettore, dall'uso del segmento e dalla sua origine dà la **funzione** di una lettura
  (`Instrada`), i **ruoli che la funzione ammette** (`RuoliAmmessi`) e l'**intersezione** con i ruoli della
  famiglia, con il motivo per esteso. Le categorie restano un'annotazione e non toccano l'intersezione (v3 §2;
  R7). Un uso pertinente conferma solo con origine «operatore» o «scenario»; di origine «riconoscimento» è un
  candidato, non una conferma, e vale come da valutare (E2).
- Da A1b.10, **`Interpreta`** (parte 1 §7.3; piano A, 5.4.6): legge un `DocumentoEvidenze` con l'uso dei
  segmenti dato e restituisce un'`Interpretazione`: letture per occorrenza, con funzione, ruoli candidati,
  categorie, trasformazioni, qualità e motivi; la deduplica solo della stessa occorrenza; le alternative e le
  annidate con la diagnostica; gli attributi legati all'entità (revisione in campo separato, formazione STEP,
  titolo, scala e materiale grezzi, quantità dalla colonna dichiarata); nessuna relazione in A1 (E3); la
  copertura dei selettori senza forme; lo stato; `ID` e `Impronta`. `ImprontaUso` dà l'impronta canonica di
  un uso dei segmenti.
- Da A1c (B3), il **compositore** (R63 B; contratto di A1c §2.1; T-B0-17, T-B0-37):
  `ComponiCodiceDocumentale` porta una lettura (per esempio di un nodo STEP o del nome del file: base 9123456,
  marcatore A, revisione 2 da «9123456A_2») nella forma documentale della stessa famiglia, la forma attiva su
  `cartiglio.codice`: «9123456A2». Il risultato è un `CodiceComposto`: la stringa, oppure nessuna stringa con
  uno dei sette motivi `MotivoComposizione*`, quando la grammatica non la determina in modo univoco (R87: senza
  revisione nessuna stringa canonica). Versione propria: `VersioneComposizione` (`composizione-1`).
- Da A1c (B6b), la **revisione registrata** (R113 B ratificata; emendamento E2 §2.6; come T-B0-17, un metodo solo):
  `LeggiRevisioneRegistrata(famiglia, testo)` legge una revisione scritta a parte nel DB (la colonna `rev` del vecchio
  motore) con le sole regole di revisione `campo_separato` della famiglia, e restituisce una `RevisioneRegistrata`:
  l'originale, la lettura, lo stato (`letta`, `non_interpretabile`, `nessuna_regola`, `ambigua`), la regola
  («famiglia/regola») e il motivo. Mai «codice + rev» composti. Versione propria: `VersioneRevisioneRegistrata`
  (`revisione-registrata-1`).
- **Il principio della provenance** (5.0, 04/10 sera): il motore non indovina relazioni. Due cose si
  collegano solo se la provenance lo consente (stesso segmento, stessa entità, stessa riga di tabella, stesso
  nodo STEP, legame dichiarato nei fatti, uso esplicito) **e** una regola semantica lo fa nascere (una forma o
  una regola attiva della grammatica, una riga del router, una regola del piano). Mai per vicinanza nel testo,
  somiglianza di stringhe o ordine di apparizione. Ogni punto di collegamento nel codice scrive quale
  provenance lo consente e quale regola lo fa nascere.
- Tre esiti per ogni elemento della grammatica (C-10): errore (nessun motore), riservato con diagnosi (non
  entra nel motore, il resto sì), attivo.

## Non appartiene qui

- **Niente `Minuteria`, niente motore legacy** (R7): `motorea` sta nella cartella di
  `core/inbox/classificazione` ma **non la importa**, e `classificazione` non importa `motorea`. La categoria
  «minuteria» di una lettura viene solo dalla famiglia dichiarata nella grammatica, mai da un riconoscitore
  sul nome. Lo garantiscono gli import (G1, G9).
- **Il formato delle grammatiche**, la lettura stretta, la validazione, la forma canonica, l'hash, l'indice e
  l'elenco delle capacità riservate: `core/registro/regole/grammatica`. Qui si chiede a quel pacchetto se un
  selettore, una decorazione o una proiezione sono attivi (`SelettoreAttivo`, `DecorazioneAttiva`,
  `ProiezioneAttiva`): l'elenco resta uno solo.
- **Leggere i file** (indice, grammatiche) e il disco in generale: chi compone il processo (il banco, da A1d
  l'avvio dell'anteprima). Qui arrivano i byte.
- **Costruire il documento** dai fatti (STEP, PDF, nome del file, mail): `core/estrazione`, che `motorea`
  non importa. Le prove dall'adattatore a `Interpreta` stanno nel QA di `estrazione` (freccia solo nei test).
- **Decidere quali segmenti sono pertinenti**: è `UsoSegmenti`, un ingresso (operatore, file dei casi).
  `Interpreta` non promuove mai la storia da sola (R48 A).
- **Legare un prodotto alla sua quantità** attraverso la riga, scegliere un prodotto, valutare i target:
  A1c. Qui la quantità resta l'attributo della riga.
- **Le proposte di prodotti e ancoraggi**, la valutazione, il confronto con l'atteso: A1c.
- **L'identità del nodo e la catena del codice** (`IdentitaNodo` con l'identità parziale, i candidati di
  revisione come quella del nome del file STEP, `CatenaCodice`, la riconciliazione con il cartiglio):
  `core/ancoraggio`, da B4 (R86, R87; T-B0-37). Il compositore riceve una lettura sola e non la completa con
  altre: la revisione del nome del file non entra nel codice proposto di un nodo.
- **Il riferimento al caso degli attesi** (`rif_caso`) che un esempio può portare: è un metadato opaco, lo
  legge solo il banco; qui non si legge mai (R47 b).
- **I dati dei clienti**: grammatiche, indice e limiti stanno nel dataset privato, mai nel repository.

## File

- **`motore.go`** — responsabilità:
  - commento `// Package`; `VersioneAlgoritmo` (`motorea-1`);
  - `Motore`, `CompilaVerificato`, `Motore.Snapshot`, `Motore.Riconosci`. Da A1b, il `Motore` porta anche
    le regole d'interpretazione e la versione dei limiti ricevuti, calcolate una volta in
    `CompilaVerificato` (E4);
  - l'abbassamento della grammatica nel motore: quali forme entrano (attive, con almeno un selettore attivo,
    non proiezioni riservate, senza decorazioni o revisioni riservate, con al più un'etichetta, un
    marcatore, un token e una revisione) e su quali selettori.
- **`piani.go`** — responsabilità:
  - `pianoForma`: due regex compilate una volta per coppia (famiglia, forma), la sequenza con il confine
    destro e la stessa ancorata alla fine, che serve solo ad allungare una lettura (più una per ogni regola
    di revisione con token sospesi); mai tutte in alternanza, mai compilazioni per unità. I gruppi li genera
    il compilatore («g0», «g1»…) e ognuno porta l'operazione da fare sul testo catturato;
  - le parti (etichetta, affisso, base, ripetizione della base, revisione, decorazione con la sequenza
    interna di un suffisso, separatore, marcatore, token), le alternative letterali con `QuoteMeta`, le
    maiuscole indifferenti ASCII senza `(?i)`;
  - le revisioni in campo separato (D-07); le rune con cui una lettura può cominciare;
  - da A1c (B3), per ogni piano le parti come le legge il compositore (`scrittura`), calcolate una volta in
    `compilaForma`.
- **`componi.go`** — responsabilità (A1c, B3; R63 B):
  - `VersioneComposizione`, `CodiceComposto`, le costanti `MotivoComposizione*`;
  - `Motore.ComponiCodiceDocumentale`: la forma del cartiglio della famiglia (una sola, compilata), la lettura
    nel suo insieme, la composizione parte per parte, la rilettura della stringa con `Riconosci`;
  - `parteScritta` e `scritturaDi`: la forma ridotta a ciò che serve per scriverla.
- **`registrata.go`** — responsabilità (A1c, B6b; R113 B):
  - `VersioneRevisioneRegistrata`, `RevisioneRegistrata`, le costanti `StatoRevisioneRegistrata*` e
    `MotivoRevisioneRegistrata*`;
  - `Motore.LeggiRevisioneRegistrata`: le regole `campo_separato` della famiglia da tutti i selettori, senza doppioni
    per ID (le attive compilate nel motore e le riservate), lo stato, la regola applicata al testo intero (D-07).
- **`scansione.go`** — responsabilità:
  - `scandisci`: ogni inizio di runa con il confine sinistro rispettato, match ancorato su una finestra
    lunga quanto la lettura più lunga più il confine, ripartenza di una runa; l'allungamento della lettura quando il confine destro ha consumato un carattere che poteva essere
    suo.
- **`confini.go`** — responsabilità:
  - le quattro classi di confine (D-05), con il caso che giustifica ognuna: la regex del confine destro e i
    due controlli in Go.
- **`letture.go`** — responsabilità:
  - `LetturaForma`, `BaseLetta`, `SegmentoLetto`, `ParteLetta`, `AffissoLetto`, `RevisioneLetta`,
    `DecorazioneLetta`, `RipetizioneLetta` e le costanti degli stati;
  - la costruzione di una lettura dai gruppi: codice richiesto, attribuzione degli affissi, ripetizioni
    concordanti o discordanti, revisione letta o token sospeso.
- **`confronta.go`** — responsabilità:
  - `Compatibilita` e le sue costanti, `ConfrontaBasi`, `ConfrontaRevisioni`.
- **`esempi.go`** — responsabilità:
  - la verifica di tutti gli esempi sul motore intero del cliente: mancante, in eccesso, diverso, selettore
    inattivo, collisione di due forme della stessa famiglia, non verificato.
- **`insieme.go`** — responsabilità:
  - `InsiemeRegole`, `CompilaInsieme`, `InsiemeRegole.MotoreDi`.
- **`limiti.go`** — responsabilità:
  - l'applicazione dei limiti di riconoscimento ricevuti dall'indice (`grammatica.Limiti`): byte dell'unità,
    letture per unità, risultato parziale, `limite.superato`. Nessun valore proprio.
- **`router.go`** — responsabilità (A1b.9; R23):
  - `VersioneRouter` (`router-2`), `Funzione` e le sue costanti, le costanti degli usi, `Instradamento`;
  - `Instrada`: le righe del router come dati (una tabella fissa), con un motivo stabile per riga. Le celle
    di una tabella passano dalle righe 1-5 con il selettore e l'uso del loro segmento (riga 6); un selettore
    o un uso fuori tabella dà una menzione, mai una richiesta;
  - il candidato di E2 (`usoPerIlRouter`, `daConfermare`): su oggetto, corpo e storia un uso pertinente senza
    origine «operatore» o «scenario» (una lista bianca) vale come da valutare, e il motivo lo dice;
  - `RuoliAmmessi` e l'intersezione con i ruoli della famiglia, con il motivo per esteso.
- **`interpretazione.go`** — responsabilità (A1b.10):
  - `VersioneRisultato` (1); i tipi del risultato: `Interpretazione`, `LetturaCodice` (con `AltreUnita`,
    `Uso`, `OrigineUso`), `AttributoLetto` (con `Evidenze`), `RelazioneSemantica`, `Trasformazione`, e le
    costanti di stati, qualità, tipi e operazioni. `RiferimentoLetto` e `MenzioneGenerica` non nascono
    (R20 c; R25 e = A).
- **`interpreta.go`** — responsabilità (A1b.10):
  - `Motore.Interpreta`, passo per passo (5.4.6 punti 1-17): validazione del documento e dell'uso, limiti
    del documento, riconoscimento per unità in ordine di ID, posizioni assolute, funzione e ruoli, categorie,
    trasformazioni, qualità e motivi (fonte OCR, etichetta fuori zona), deduplica della stessa occorrenza,
    alternative e annidate, id e nome discordi, copertura, pertinenza ignota, stato, `ID` e `Impronta`;
  - le regole d'interpretazione (`regoleDa`), che `CompilaVerificato` congela nel `Motore` (E4);
  - `ImprontaUso`.
- **`attributi.go`** — responsabilità (A1b.10):
  - la revisione in campo separato con la `RegolaRevisione` della famiglia letta nella stessa entità; senza
    nessuna regola sul selettore, la revisione del cartiglio è `non_interpretabile`, con l'originale conservato
    e nessun valore (il default prudente di C-34, R21 e = A); la formazione STEP mai confrontata, titolo, scala
    e materiale grezzi, la quantità dalla colonna che una `quantita_tabellare` attiva dichiara.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che il pacchetto produce (R41 b): `grammatica.esempio_*`, `forma.collisione_esempi`,
    `regole.assenti`, `regole.non_valide`, `regole.file_assente`, `regole.sha256_discorde`,
    `regole.cliente_discorde`, `revisione.da_verificare`; da A1b.10 `motore.letture_alternative`,
    `motore.letture_annidate`, `motore.id_nome_discordi`, `motore.ripetizioni_discordanti`,
    `motore.pertinenza_ignota`, `revisione.formazione_non_confrontabile`, `revisione.non_interpretabile`,
    `revisione.discordante`, `quantita.non_interpretabile`.

## Entry point

- **`CompilaInsieme`, `InsiemeRegole.MotoreDi`** — chi li chiama: da A1a il banco (`app/bancoa`), da A1d
  l'avvio dell'anteprima.
- **`CompilaVerificato`** — chi lo chiama: `CompilaInsieme`; il banco, per il rapporto delle regole.
- **`Motore.Riconosci`** — chi lo chiama: la verifica degli esempi; il banco (modalità `casi`); da A1b
  `Interpreta`. È la sola funzione di riconoscimento: non c'è un secondo motore.
- **`ConfrontaBasi`, `ConfrontaRevisioni`** — chi li chiama: il banco; da A1c l'ancoraggio.
- **`Instrada`, `RuoliAmmessi`** — chi li chiama: `Interpreta`, per ogni lettura; le prove di A-C04 li
  chiamano anche per le righe dei selettori riservati in A1.
- **`Motore.Interpreta`, `ImprontaUso`** — chi li chiama: da A1b.11 il banco (modo `casi`, con
  `estrazione.DaTesto` e l'uso sconosciuto); da A1c le proposte; da A1d l'anteprima.
- **`Motore.ComponiCodiceDocumentale`, `VersioneComposizione`** — chi li chiama: da A1c solo la valutazione (il
  passo 7a del 6.4.6, T-08). `ancoraggio` non lo chiama: ne riceve il risultato in
  `ContestoStrutturale.CodiciProposti`, per ID di lettura, e lo mette in `CatenaCodice.Proposto` (B4). La
  versione va nell'impronta dell'esito (B6). Alla fine di B3 nessun codice di prodotto lo chiama ancora.
- **`Motore.LeggiRevisioneRegistrata`, `VersioneRevisioneRegistrata`** — chi li chiama: da A1c (B6b) solo la
  valutazione, per la colonna `rev` del vecchio di ogni file (`VecchioPiatto.Revisione` e `RevisioneDa`), con la famiglia
  della lettura del codice registrato. La versione va nell'impronta dell'esito.
- Oggi, nel codice di prodotto, il banco (`app/bancoa`, da A1a.5); l'anteprima arriva in A1d.

## Invarianti

- **I limiti vengono dall'indice** (R43 B): `CompilaVerificato` li riceve, `CompilaInsieme` li prende
  dall'indice stesso. Il pacchetto non ha valori suoi e nessun `LimitiPredefiniti`. Superare un limite di
  riconoscimento dà `limite.superato` (avviso) e un risultato parziale, mai un successo vuoto. Nessun limite
  di tempo: romperebbe il determinismo.
- **Budget di complessità** (par.12): nessun tipo, costante, ramo o tabella per un cliente. Il runtime è
  privato e minimo: piani indicizzati per selettore, regex RE2 compilate, gruppi catturati con poche
  operazioni (conservare, normalizzare, attribuire, confrontare la ripetizione con la base). Fra i tipi della
  grammatica e quelli del motore non c'è una corrispondenza 1:1. Un cliente nuovo che usa capacità già
  previste entra con il suo file v1 e la sua voce nell'indice, senza codice Go. Una capacità riservata non
  ha piani né codice runtime.
- **Un elemento è attivo solo se la grammatica lo dichiara**: nessuna semantica inventata. Un affisso si
  attribuisce solo sui selettori della sua attribuzione; un token si conserva e non si attribuisce mai; un
  livello o uno stato PDM sono decorazioni, mai revisioni; una forma parziale non ha segmenti per i
  mancanti.
- **Confini**: il sinistro si controlla in Go sul testo intero, il destro lo scrive il compilatore fuori
  dalla lettura; dopo un inizio si riparte dalla runa successiva, mai dalla fine del match (T2, T4, T5, T13).
  A parità d'inizio vince la lettura più lunga che rispetta il confine (`Longest`, T6).
- **I pattern dei clienti non hanno gruppi**: li genera il compilatore, con nomi unici (T12); sempre
  `regexp.Compile`, mai `MustCompile` (T18).
- **Offset in byte UTF-8**, 0-based, fine esclusa, su un confine di runa, sul testo dato: `Originale` è
  sempre `testo[Inizio:Fine]`.
- **Il router è una tabella fissa e versionata** (`router-2`): stessi ingressi, stessa funzione. Non guarda
  la zona del cartiglio né la fonte OCR, e non nomina nessun cliente; cambiare una riga vuol dire cambiare
  `VersioneRouter`, che sta nell'identità dell'interpretazione e non nello snapshot (R41 c): `router-1`
  diventa `router-2` con E2. Le righe dei selettori riservati in A1 ci sono lo stesso: la tabella è il router
  (R20 a).
- **Un riconoscimento automatico non conferma una richiesta** (E2): su oggetto, corpo e storia un uso
  pertinente conferma solo con origine «operatore» o «scenario» (una lista bianca); di origine
  «riconoscimento», o con un'origine non dichiarata, vale come da valutare. Oggetto e corpo danno una richiesta
  da confermare, con l'incertezza nella lettura e `motore.pertinenza_ignota`; la storia dà una menzione, mai una
  richiesta (5.1; R48 A). La lettura conserva uso e origine veri. Lo scenario è l'ingresso del banco che simula
  il caso atteso, e nell'anteprima o nel prodotto non vale come una decisione dell'operatore. Nessun punteggio
  e nessuna tassonomia degli intenti: la distinzione fra candidato e confermato usa gli usi che ci sono già.
- **Per chi legge il risultato** (E2): una lettura con funzione «richiesta» è confermata solo con l'uso
  «pertinente» e l'origine «operatore» o «scenario»; ogni altra richiesta è da confermare, anche con l'uso
  «pertinente» e l'origine «riconoscimento». Il segmento da cui viene ha `motore.pertinenza_ignota`.
- **`Interpreta` non riceve i target** (A-C11): solo documento e uso. L'errore è di contratto
  (`*evidenze.ErroreContratto`), oppure di un motore nullo; un limite dei dati dà lo stato `parziale`, mai un
  successo vuoto.
- **Lo stato** guarda solo le capacità di lettura (`testo`, `cartiglio`, `struttura`, `contenuto`), i limiti
  superati e le unità troncate dal worker. Firma, segmentazione, tabelle, elenco PDF e grafo completo restano
  qualità della fonte (R32 b): un PDF senza testo è `parziale`, mai «completa con zero letture» (A-C09).
- **La deduplica è solo della stessa occorrenza** (A-C10): stesso testo originale e stesso intervallo, o una
  cella agganciata per righe e la lettura del segmento della sua riga dentro quelle righe, con lo stesso
  contenuto. Le righe della cella sono righe del corpo: valgono solo per l'unità del segmento con il selettore
  della cella, mai per l'oggetto. La principale è la lettura con l'entità; con due candidate non si unisce
  niente.
- **Gli attributi stanno con la loro entità** (A-C02): un'unità senza entità non ne dà. La revisione in campo
  separato vale solo con la regola della famiglia letta nella stessa entità (o l'unica regola attiva, senza
  letture); una regola riservata conta solo per una famiglia che sul selettore non ne ha di attive. La
  formazione STEP non si confronta mai (D1); la quantità vale solo con una `quantita_tabellare` attiva per il
  selettore delle celle, e resta della riga (R28 a). Per un'entità o una tabella non lette per intero (un
  limite superato) non nascono né la revisione in campo separato né la quantità: lo dice `limite.superato`
  con le entità in `Rif`.
- **Le diagnostiche del documento restano nel documento**: `Interpreta` non le copia (per esempio
  `step.formazioni_alternative`, che dà l'adattatore). Riusa con le loro costanti `limite.superato` e
  `capacita.non_supportata` di `grammatica`; la seconda come nota, «campo ricevuto, non letto» (5.4.6
  punto 15), non come l'avviso con cui `grammatica` la emette per un elemento riservato del file regole.
- **Il `Motore` è immutabile e autosufficiente** (E4): della grammatica, oltre ai piani, `Interpreta` usa
  solo i ruoli delle famiglie, le revisioni in campo separato riservate e le `quantita_tabellare` attive.
  `CompilaVerificato` le calcola una volta, dalla stessa grammatica normalizzata dei piani, e le copia nel
  `Motore`: cambiare dopo la compilazione le slice dello snapshot del chiamante non cambia l'interpretazione,
  e nessuna guardia gira a ogni chiamata. Le regole restano per selettore: lo stesso cliente può avere forme
  diverse nel corpo della mail, nel nome del file e nel cartiglio, ognuna con le sue regole dello stesso
  snapshot, mai una regola sola per tutti i selettori né una fusione per somiglianza.
- **Limite noto, conseguenza voluta di E4**: `HashSnapshot` è quello dello snapshot ricevuto da
  `CompilaVerificato`, che non lo ricalcola. Uno `SnapshotRegole` non nato da `NuovoSnapshot` (hash vuoto o
  incoerente con la grammatica) dà quindi un'interpretazione con quell'hash, mentre prima la guardia per
  chiamata dava un errore. `CompilaInsieme` prende lo snapshot sempre da `NuovoSnapshot`.
- **La versione dei limiti è quella dei valori** (E4): `Interpretazione.VersioneLimiti` e `ImprontaLimiti`
  vengono dai limiti che `CompilaVerificato` ha ricevuto, anche quando lo snapshot è stato validato con
  un'altra versione.
- **Valori che dichiarano gli adattatori** e che `motorea` ripete, perché non importa `core/estrazione`: il
  testo `messaggio.corpo_testo` su cui si misura `PosTabella.Esatto`; i nomi delle capacità di lettura
  (`testo`, `cartiglio`, `struttura`, `contenuto`) e lo stato `disponibile`; l'uso `valutato`; la
  localizzazione `esatta`, il metodo `ocr`, la zona `pagina`; i tipi di localizzatore `testo`, `tabella`,
  `nome_file`; i campi `id`, `nome`, `revisione`, `titolo`, `scala`, `materiale`. Se un adattatore li cambia,
  lo dicono le prove di `core/estrazione/interpreta_test.go` (A-C09, A-C10, A-C05), non un errore qui.
- **Le relazioni** (5.4.6 punto 13): in A1 nessun codice (E3 = A; par.12, nessun runtime per una capacità
  soltanto riservata). Restano il tipo `RelazioneSemantica`, il campo `Relazioni`, sempre vuoto, la riga 12
  del router e le sue prove su `Instrada`. Il runtime nascerà quando una forma attiva su
  `cartiglio.particolare_simile` potrà davvero produrle.
- **Le collisioni osservate sugli esempi sono errori**: nessuna precedenza per ordine di array. Gli esempi
  delle forme che non entrano nel motore restano non verificati, mai passati.
- **Il compositore non cambia niente di A1a e A1b** (T-B0-17, D2): è un metodo nuovo; nessuna firma, nessun
  campo di `LetturaForma` o di `LetturaCodice`, nessuna costante e nessun comportamento di riconoscimento e
  interpretazione cambia (`VersioneAlgoritmo` resta `motorea-1`); la lettura che riceve resta com'è. Nessun
  codice di diagnostica nuovo (T-15): il motivo sta nel `CodiceComposto`.
- **La stringa non si inventa mai** (R63 B, R87; workflow, B3 A3.3). Si compone solo con la forma della famiglia
  compilata nel motore su `cartiglio.codice`: una forma riservata, o con una parte riservata, non conta; con più
  forme nessuna scelta. Parte per parte:
  - base e ripetizione della base: la base normalizzata della lettura, che deve essere completa e della
    famiglia; una proiezione sul cartiglio non è il codice;
  - marcatore, token, affissi: dalla lettura, se la forma li ammette (i letterali sono esatti; un affisso per
    regola). Una parte facoltativa senza valore resta fuori solo se l'assenza è letta, cioè se la forma della
    lettura aveva quel posto;
  - revisione: dalla lettura, della stessa regola, zeri compresi, con il separatore della regola se è uno solo;
    una lettura senza revisione non dà stringa se la forma chiede o ammette la revisione (T-B3-01);
  - separatori: fanno parte del codice; si scrivono solo se il testo è uno solo, mai presi dalla lettura;
  - etichette e decorazioni della forma: stanno fuori dall'identità e la stringa proposta servirà alla
    rinomina, quindi non vi entrano; una forma del cartiglio che chiede un'etichetta o una decorazione non dà
    stringa, motivo `parte_non_determinata` (T-B3-02; anche facoltative, perché la scelta non si fa);
  - della lettura restano fuori solo etichetta, decorazioni e ripetizioni concordanti; un affisso, un
    marcatore, un token o una revisione letti senza posto nella forma bloccano la stringa. Le parti interne di
    una decorazione contano come parti della lettura: un token dentro un `suffisso_documento` è il token della
    lettura e blocca la stringa.
  La stringa composta si rilegge con `Riconosci`: la stessa forma la deve leggere per intero, completa, con le
  stesse parti; se no, nessuna stringa.
- **La revisione registrata non cambia niente di A1a, A1b e del compositore** (R113 B; E2 §2.6; come T-B0-17): è un
  metodo nuovo, con una versione propria; nessuna firma, campo, costante o comportamento esistente cambia, e nessun
  codice di diagnostica nuovo: lo stato e il motivo stanno nella `RevisioneRegistrata`. Le regole, nell'ordine:
  - valgono le sole regole `campo_separato` della famiglia, da tutti i selettori e senza doppioni per ID, perché il
    testo registrato non ha un selettore; le regole in linea non contano;
  - nessuna regola, né attiva né riservata: `nessuna_regola` (il default prudente di C-34 non si rifà: il testo non è
    un campo del cartiglio); più regole attive: `ambigua`, senza provarle; solo riservate: `non_interpretabile` (Q1);
  - una regola attiva si applica al testo intero (D-07), con la stessa regex della revisione in campo separato di
    `Interpreta`: letta, oppure `non_interpretabile` per un token sospeso (conservato), un testo non letto per intero
    o vuoto;
  - mai «codice + rev» composti; il testo non si ripulisce (gli spazi ai bordi li toglie chi chiama); il confronto
    con altre revisioni resta di `ConfrontaRevisioni`, con le sole equivalenze dichiarate.
- **L'ordine dei motivi è fisso**: forma assente, forme multiple, lettura non completa, revisione ambigua,
  revisione non determinata, parte non determinata, qualificatore non trasferibile. La revisione non
  determinata viene prima delle altre parti: è il segno dell'identità parziale di R87.
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; nessun ordine dipende da
  una mappa. Letture in ordine di (inizio, fine, famiglia, forma); la grammatica si rende canonica anche qui,
  quindi famiglie e forme permutate danno le stesse letture e le stesse diagnostiche.

## Dipendenze

- **Importa:** `core/registro/regole/grammatica`, `core/estrazione/evidenze`, `platform/jsoncanonico`
  (l'impronta dell'indice), `github.com/google/uuid`, la libreria standard. Mai il motore legacy
  (`core/inbox/classificazione`, `core/registro/regole`).
- **È importato da:** `internal/app/bancoa`, il banco; da A1c `core/ancoraggio` e `core/valutazione` (vedi
  Entry point).

## Test

- Nel ramo `-qa` (piano A, par.4.3, Q4), tutti L1 e sintetici, con le grammatiche ACME dai nomi neutri
  (`acme-prefisso`, `acme-marcatore`, `acme-documento`, `acme-punti`, `acme-etichetta`,
  `acme-campo-separato`):
  - confini e offset (A1a-CNF); prefisso fuori dal selettore e nessuna rimozione globale (A-C06); forma
    parziale e completamenti (A-C07); etichetta obbligatoria (A-C08);
  - revisioni (A1a-REV), decorazioni (A1a-DCR), marcatori e categorie (A1a-MRK), esempi (A1a-ESE),
    determinismo (A1a-DET), limiti (A1a-LIM), insieme delle regole (A1a-INS);
  - da A1b.9, `router_test.go`: tutti i rami del router, con le famiglie {prodotto, componente},
    {componente} e {prodotto}, gli insiemi vuoti e le categorie fuori dall'intersezione (A-C04); il
    candidato da riconoscimento automatico (E2);
  - da A1b.10, `interpreta_test.go` e `determinismo_test.go`, su documenti costruiti in Go: la firma senza
    target (A-C11), alternative e annidate, limiti, determinismo (A1b-21, A1b-22);
  - da A1c (B3), `componi_test.go`, con la famiglia ACME del compositore (`acme-compositore`) e
    `acme-maiuscole`: l'esempio del workflow, PO-06, un caso per motivo, la verifica avversariale (parti
    facoltative, separatori, etichette, decorazioni, maiuscole, revisioni con gli zeri, basi parziali, letture
    costruite a mano), le firme e i campi di A1a e A1b invariati, il determinismo, l'autosufficienza (E4);
  - da A1c (B6b), `registrata_test.go`, con `acme-campo-separato` e le sue varianti: una regola legge la colonna,
    nessuna regola (anche con una regola in linea), due regole, un token sospeso, una regola riservata, un testo non
    letto per intero o vuoto, la stessa regola su più selettori, la regola di un'altra famiglia, mai «codice + rev»
    composti, la versione, la firma e il determinismo.
- Dall'adattatore a `Interpreta`, sulle fixture sintetiche: `core/estrazione/interpreta_test.go` (A-C02,
  A-C03, A-C05, A-C09, A-C10, A-C12, D1, HASH-CONFLITTO, A1b-19, A1b-23).
- Il controllo degli import (G1, con il divieto del legacy), la guardia sul riferimento al caso degli
  attesi e i codici (A1a-CAT) stanno in `core/estrazione/evidenze`.
- I clienti dei test sono inventati (ACME); i test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/registro/regole/grammatica/README.md`
- `internal/core/estrazione/evidenze/README.md`
- `internal/core/inbox/classificazione/README.md` (il motore legacy, che qui non si usa)
- `internal/README.md`
