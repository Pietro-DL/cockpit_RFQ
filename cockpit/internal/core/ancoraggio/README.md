---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/ancoraggio` — le proposte del motore A: i prodotti chiesti dalla mail e le strutture dei prodotti

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

## Non appartiene qui

- **La fotografia e il DB**: `ancoraggio` non importa `core/fotorfq` né il caricatore (T-B0-04). La richiesta
  valutata, con i gesti, la compone `core/valutazione` dalla fotografia; qui arriva già fatta.
- **Il target, la sua identità e lo stato della fonte** (R70 A, R79, R65, R76 b, T-E1-09): `core/valutazione`
  (commit P7a).
- **Gli ancoraggi dei file e la catena del codice** (`ProponiAncoraggi`, `AncoraggioFile`, `CandidatoAncoraggio`,
  la disponibilità dei file, i file ancorati ai nodi, l'identità dei nodi, `CatenaCodice`, le decisioni accanto ai
  nodi e agli archi, la riconciliazione, la pre-associazione): arrivano con il commit di B4 (P6b), che userà
  `ProponiStrutture`.
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
  - `FileInterpretato` (allegato, documento, interpretazione; la disponibilità arriva con B4);
  - `RifNodo` e `PrefissoRifNodo`: il riferimento di un nodo, sha256 più chiave (T-E1-04);
  - `ArcoPercorso` con le origini `fatti`, `confermato`, `proposto`; `NodoStruttura`, `StrutturaFile`;
  - `StrutturaDa`, con l'ordine canonico di radici, nodi, letture e archi.
- **`struttura_prodotto.go`** — responsabilità:
  - `ProdottoRichiesto` (con `ComponenteID` e `FonteConfermata`); `ContestoStrutturale`, `ComponenteDeciso`,
    `RigaPropostaLegacy` (il `NodoStrutturale` del 6.4.5, diviso: T-B0-02), `RifComponente`, `RifRigaProposta`;
  - `StatoStruttura`, `NodoProposto`, `ArcoProposto`, `StrutturaProdotto`;
  - `ProponiStrutture` (radici candidate, struttura, stato, diagnostiche), `StatoStrutturaDelTarget`, i controlli di
    contratto.
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che il pacchetto produce (R41 b): `ancoraggio.richiesta_non_valutata`,
    `ancoraggio.righe_stessa_base`, `ancoraggio.quantita_non_intera`, `ancoraggio.alternative_conservate`;
    `ancoraggio.target_senza_struttura`, `ancoraggio.grafo_incompleto` (le strutture).

## Entry point

- **`ProponiProdotti`** — chi lo chiama: da A1c `core/valutazione` (`Calcola`, passo 5), per il banco e, da A1d,
  per l'anteprima.
- **`StrutturaDa`** — chi lo chiama: da A1c `core/valutazione`, per gli STEP del thread (`Calcola`, passo 7: il
  contesto).
- **`ProponiStrutture`** e **`StatoStrutturaDelTarget`** — chi li chiama: da B4 `ProponiAncoraggi` (le radici e i
  figli dove ancorare i file); da A1c `core/valutazione` lo stato della struttura per prodotto (B5, B6).
- **I tipi del vocabolario e della fonte** — chi li usa: `core/valutazione` (la richiesta del thread, il target con
  la sua autorità, la fonte di un prodotto con i documenti candidati); dal commit di B4 gli ancoraggi.
- Oggi, nel codice di prodotto: `core/valutazione` usa i tipi e chiama `ProponiProdotti` (da B1, `ValutaProdotti`);
  `StrutturaDa` e `ProponiStrutture` ancora non li chiama nessuno: lo faranno B4, dentro `ProponiAncoraggi`, e B5 e
  B6 in `core/valutazione`.

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
     del target (stesso namespace; `ConfrontaBasi` uguale, o compatibile parziale) è una radice candidata; di solito
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
- **`SenzaFile`**: le strutture non ancorano file, quindi lo dicono di ogni nodo; lo ricalcola B4 dai file ancorati.
  L'assenza di un file è ammessa e senza diagnostica (3.3.7, P-20).
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
  contesto con l'origine o gli estremi sbagliati. Un limite dei dati sta nell'esito, con le diagnostiche.
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; nessun ordine che dipenda da una
  mappa; messaggi, richiesta e gesti in ordine diverso danno gli stessi byte canonici, e così target, strutture,
  nodi, archi, componenti e righe per le strutture. `time` serve solo per i tipi e per portare in UTC al
  millisecondo i tempi dei gesti nell'impronta (par.3.4.3).
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
    valori e i campi del contratto.
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go`; i codici nell'elenco d'oro
  (A1a-CAT, A1c-L1-32) in `core/estrazione/evidenze/codici_diagnostica_test.go`.
- I clienti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/core/estrazione/README.md`
- `internal/core/estrazione/evidenze/README.md`
- `internal/README.md`
