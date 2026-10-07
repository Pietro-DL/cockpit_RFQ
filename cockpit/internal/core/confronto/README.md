---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/confronto` — il vecchio, il nuovo e l'atteso dei file di un thread: badge, indicatore di revisione, correzioni manuali, esiti

## Scopo

- **Mette a fianco** il «vecchio» (le proposte e le decisioni di adesso, con i codici già letti dalla grammatica del
  cliente), il «nuovo» (il motore A) e, solo nel banco, l'«atteso» (l'oracolo privato), per i file di un thread (giro 5,
  A1c, commit P8; piano A, par.3.3.8 e 6.4.6; R3: «DTO e logica di confronto … Riceve DTO»; R42 B, R53 B).
- **Riceve DTO propri e piatti** (`File`, `Vecchio`, `Nuovo`, `Candidato`, `ProdottoNuovo`): chi chiama li copia campo per
  campo dai record piatti di `valutazione` (`FileConfrontabile`, `VecchioPiatto`, `NuovoPiatto`, `CandidatoPiatto`,
  `ProdottoConfrontabile`), senza scelte. I campi sono quelli congelati nella fase 0 di B6 (CP.2, con `CodiceLettoBase`
  di F0-19 e `CodiceLettoMarcatore` di R114: 18 campi in `Vecchio`), identici nei due pacchetti: stessi nomi, ordine,
  tipi e tag, solo tipi delle foglie.
- **Calcola** (`Confronta`):
  - per ogni file il **badge** (`uguale`, `nuovo_ancoraggio`, `diverso`, `regressione_su_confermata`, `fuori_richiesta`)
    con il motivo, dalla tavola del 6.4.6 (R31 d A), solo dallo stato del DB e dalle basi lette, mai dall'atteso (R31 a A,
    ristretta dalla R2: P-14);
  - l'**indicatore di revisione** (`uguale`, `diversa`, `non_determinabile`), separato dal badge: la sola revisione non è
    mai una regressione (R31 b A; E-19). Il confronto lo fa `valutazione` con la grammatica; qui si traduce;
  - i **conteggi per badge** e le **correzioni manuali** prima e dopo sui file decisi (R30 f A), con la definizione di
    R114 (precisata dall'utente il 07/10, `domande-a1c.md`): un indicatore ricostruito delle correzioni necessarie, con
    denominatore, copertura, esclusi e non determinabili;
  - con l'atteso (solo nel banco), gli **esiti per file** contro le voci degli attesi (`ConfrontaConAtteso`, R30 a, b, d,
    e A) e per prodotto (`ConfrontaProdotti`);
  - l'**impronta** dell'esito, senza le parti contro l'atteso: la stessa nel banco e nell'anteprima.

## Non appartiene qui

- **La lettura dei codici con la grammatica** (`LeggiCodiceRegistrato`, `confronto.codice_registrato_non_leggibile`), la
  scelta di basi, candidati e revisioni, il confronto delle revisioni (`motorea.ConfrontaRevisioni`): `core/valutazione`,
  che ha il motore. Qui il vecchio arriva già letto.
- **La copia dai record piatti ai DTO** (`passaggio.go`) e la prova che i due insiemi di tipi coincidono (A1c-L1-31): chi
  chiama, `app/bancoa` in A1c e `transport/web` in A1d.
- **La lettura degli attesi** (YAML) e la traduzione nei DTO di `Atteso`, con i nomi neutri delle sezioni (M-21): il
  runner, `app/bancoa`. Anche i controlli del runner sulle righe della baseline e della C5 (decisioni preservate, base
  letta, invarianti) e il gate: qui si portano `BaseLetta`, `Decisione` e `Invarianti` nel DTO, ma non si valutano.
- Gli assi del prodotto, lo stato del prodotto e il fascicolo: restano in `valutazione.Esito`, fuori dal gate e dagli
  attesi (contratto §2.4; R69 A).
- Il nome stampato degli esiti nel rapporto («senza risposta» per `mancante`, R30 c): il runner.

## File

- **`confronto.go`** — responsabilità:
  - commento `// Package`; `VersioneConfronto` (`confronto-2`: la misura di R114 e `CodiceLettoMarcatore`);
  - `Esito` (righe, conteggi, correzioni, esiti contro l'atteso, diagnostiche, impronta), `EsitoFile`;
  - `Confronta`; l'impronta (`jsoncanonico.ImprontaDi`) e l'ordine dei file e delle diagnostiche.
- **`dto.go`** — responsabilità:
  - i DTO piatti `File`, `Vecchio`, `Nuovo`, `Candidato`, `ProdottoNuovo`;
  - i valori stringa che le regole leggono (stati della proposta, fonte, collocazioni, associazioni, autorità, la forma
    `componente:<uuid>`, i valori di `Revisioni`), riscritti come stringhe perché il pacchetto non importa il motore.
- **`badge.go`** — responsabilità:
  - `Badge` e i cinque valori; i motivi (`Motivo*`); la tavola dei badge (`badgeDi`), nell'ordine del 6.4.6;
  - `IndicatoreRevisione`, i valori `Revisione*`, la traduzione di `Nuovo.Revisioni` con la diagnostica.
- **`correzioni.go`** — responsabilità:
  - `CorrezioniManuali` ed `EsclusiCorrezioni` (con `Totale`), la somma campo per campo (`Aggiungi`, T-B6-186);
  - la misura sui file decisi: i tre esiti di «prima», i marcatori, gli esclusi di «dopo», le tre parti di «dopo».
- **`atteso.go`** — responsabilità:
  - `Atteso`, `FileAtteso`, `ProdottoAtteso`; le sezioni neutre (`Sezione*`: scenario, baseline, reali, da_rivedere;
    P-10), i valori di `Atteso` e di `StatoAtteso`;
  - `EsitoAtteso` e i nove valori; il peso nel gate (`Peso*`, `PesoNelGate`); i motivi (`MotivoAtteso*`);
  - `EsitoFileAtteso`, `EsitoProdottoAtteso`; `ConfrontaConAtteso`, `ConfrontaProdotti`.
- **`codici_diagnostica.go`** — responsabilità: `CodiceRevisioneNonConfrontabile` (`confronto.revisione_non_confrontabile`,
  6.4.10).

## Entry point

- **`Confronta(file []File, prodotti []ProdottoNuovo, atteso *Atteso) Esito`** — chi lo chiama:
  - in A1c il banco (`app/bancoa`, commit P9), dopo `valutazione.Calcola` e la copia di `passaggio.go`, con l'atteso
    tradotto dal runner;
  - in A1d l'anteprima (`transport/web`), con la stessa copia e `atteso` nil;
  - oggi le prove del pacchetto e le L4 A1c-L4S-08 e -09.
- **`ConfrontaConAtteso`, `ConfrontaProdotti`, `PesoNelGate`** — li chiama `Confronta`; il runner può usarli da soli.
- **I tipi** `Esito`, `EsitoFile`, `IndicatoreRevisione`, `CorrezioniManuali`, `EsclusiCorrezioni`, `EsitoFileAtteso`,
  `EsitoProdottoAtteso` — li legge il rapporto del banco; A2 salverà l'esito (par.8).
- **`CorrezioniManuali.Aggiungi`** — la usa il rapporto del banco per sommare le misure dei thread di un cliente.

## Invarianti

- **«Deciso» vuol dire che un documento confermato porta il file** (`Vecchio.Documento`, dalla provenienza): è la
  definizione di `valutazione`, che da lì prende il codice e il componente del vecchio (registro §10.2), e qui vale la
  stessa (R-43). Lo stato della proposta non conta: il documento nasce solo da un gesto di conferma (`confermato_da`),
  anche se la proposta è rimasta aperta o non c'è. Una proposta `confermata` o `duplicato` senza il documento in
  provenienza non è decisa: la tavola la tratta come non decisa.
- **La tavola dei badge**, nell'ordine (6.4.6; R31 d A):
  1. file deciso e il motore non propone il componente del documento fra i candidati, oppure gli dà un'altra base,
     oppure lo dice fuori richiesta, oppure non risponde: `regressione_su_confermata`, con il motivo. Se un altro
     candidato ha la stessa base, il motivo è `stessa_base_altro_target` (T-B6-51): il badge non cambia, perché un altro
     target con la stessa base è un legame che nessuno ha deciso;
  2. file deciso e il componente del documento fra i candidati con la stessa base: `uguale` (l'associazione `ambiguo`
     resta visibile nel nuovo della riga, e si conta nella misura come `Ambiguita`: R114);
  3. file non deciso e collocazione `fuori_richiesta`: `fuori_richiesta`;
  4. vecchio senza destinazione (nessun componente, nessun «assegna», nessun candidato F8, proposta non scartata) e nuovo
     con almeno un candidato: `nuovo_ancoraggio`;
  5. stessa base e stesso target (le destinazioni della riga 4: il componente, l'«assegna», i candidati F8), oppure tutti
     e due senza ancoraggio con la stessa base: `uguale`;
  6. il resto, compresa la proposta scartata a cui il motore dà un candidato e il codice registrato non leggibile:
     `diverso`, con il motivo.
- **La base vecchia è sempre `Vecchio.Base`**, letta da `valutazione` con la grammatica, mai la stringa (R31 c): «7120100A»
  e la base 7120100 sono la stessa base. Una base vuota non è mai uguale a niente.
- **La base di un candidato `componente:<uuid>` è la base del codice del componente**, non quella letta nel file
  (`Candidato.Base`, come la riempie `valutazione`; la D1 della revisione di P8, chiusa, che non è D-R114). Per un file
  deciso «la stessa base» della riga 2 è quindi quasi sempre vera, e «un'altra base» della riga 1 scatta solo se il
  codice del documento e quello del componente hanno basi diverse. Una discordanza fra la base letta nel file e il
  componente deciso resta nelle letture (`Nuovo.Basi`) e nei conflitti di `valutazione`, non nel badge.
  - Il vincolo `fk_documento_componente` (`migrations/0018_fascicolo.sql`) rende uguali il codice del documento e quello
    del suo componente, quindi con la stessa grammatica «un'altra base» non scatta mai: per un file deciso `uguale` vuol
    dire «il motore propone il componente deciso» (R-53).
- **I target si confrontano solo nella stessa forma** (`componente:<uuid>` per un componente, la forma di
  `ancoraggio.RifComponente`): nessuna inferenza fra forme diverse (per esempio una chiave F8 `identificativo:…` e un
  target `componente:…`).
- **Il badge e l'indicatore non dipendono dall'atteso**, e nemmeno l'impronta: banco e anteprima danno lo stesso. Le righe
  C5 hanno il badge della tavola; «da rivedere» è un esito contro l'atteso, mai un badge.
- **La sola revisione non è mai una regressione**: l'indicatore sta a parte, e una revisione diversa con il badge `uguale`
  conta in `CorrezioniManuali.SoloRevisione`, sui valutabili.
- **Gli esiti contro l'atteso** (R30 a, b, d, e A), con T = la base attesa del target e C = i candidati del file valutato,
  con le loro basi. Un candidato la cui base non si legge sta in C ma non è mai in T (R-41):
  - `da_rivedere` (sezione C5) e `riservato` prima di tutto;
  - l'atteso «fuori» dà `falsa_associazione` con candidati, `fuori_richiesta` se il motore lo dice fuori richiesta,
    altrimenti `mancante`;
  - senza T, `falsa_associazione` con candidati; un file non valutato è nessuna risposta, `mancante` (R-42); altrimenti
    `corretto` (C = T = ∅);
  - nessuna risposta (non valutato, nessun candidato) con T dà `mancante`; nessuna base di C in T `errato`;
  - un'associazione ambigua o discordante con T in C dà `ambiguo`, con nel motivo `radici_in_piu` e `autorita_diversa`
    del candidato con la base attesa, se ci sono (R-49);
  - un candidato in più (un'altra base, o una base che non si legge) senza altre letture ammesse `falsa_associazione`;
  - con le basi giuste, la collocazione attesa, l'autorità attesa (`candidato_scenario` vuole `candidato_unico` con
    autorità `scenario`) e, per un figlio, le radici: in più `falsa_associazione`, mancanti `mancante`; altrimenti
    `corretto`.

  Un file senza voci è `non_coperto`; una voce senza il suo file `mancante`.
- **Una radice che non si legge è sempre una radice in più** (R-62). `valutazione` mette in `Candidato.Radici` un solo
  `""` per una o più radici illeggibili, come i candidati senza base di R-41. Qui `""` non coincide con nessuna base
  attesa, nemmeno con un `""` scritto fra le attese, e non sparisce mai. Vale come una radice in più leggibile:
  - con l'associazione ambigua o discordante, l'esito è `ambiguo` con `radici_in_piu` nel motivo;
  - con le basi giuste, è `falsa_associazione` con il motivo `radici_in_piu`;
  - se prima si applica un'altra riga della tavola, vale quella (per esempio `errato`).
- **Il peso nel gate** (R30 b A): `errato` e `falsa_associazione` bloccano; `ambiguo` e `mancante` sono astensioni;
  `corretto` e `fuori_richiesta` copertura; `non_coperto` e `da_rivedere` fuori dal gate; `riservato` mai «passato».
- **Un campo non nominato dall'atteso non si controlla** (R25 c-d): in `ProdottoAtteso` il vuoto vuol dire «non nominato».
- **I prodotti si legano in due passate** (R-47): prima gli attesi che nominano il codice richiesto, con la stessa base e
  lo stesso codice; poi gli altri, per base. Un atteso senza codice non porta via il candidato di quello con il codice.
- **La misura delle correzioni manuali** (`CorrezioniManuali`; R30 f A, con la definizione di R114, precisata
  dall'utente il 07/10, `domande-a1c.md`):
  - **che cos'è**: un indicatore ricostruito delle correzioni necessarie, sui soli file decisi. Non è il tempo
    risparmiato, e non porta misure di tempo. Distingue la proposta del vecchio motore (`CodiceLetto`, con la base e il
    marcatore letti dalla grammatica), la decisione di riferimento (il documento confermato) e la proposta del nuovo
    (`Nuovo`);
  - **il denominatore** è `Valutabili`: i decisi con «prima» e «dopo» tutti e due determinabili, lo stesso campione per
    le due parti. **La copertura** è `Valutabili` su `Decisi`. Ogni deciso è valutabile oppure escluso, con un motivo
    solo: `Decisi = Valutabili + Esclusi.Totale()`;
  - **gli esclusi, cioè i non determinabili**, per motivo, nell'ordine in cui si guardano:
    - `solo_origine_manuale`: la sola fonte `operatore`, senza la lettura del vecchio motore. La sola origine manuale
      non dimostra un errore corretto;
    - `senza_lettura_vecchia`: `CodiceLetto` vuoto, per esempio negli export. Un'informazione storica mancante non vale
      zero;
    - `codice_non_leggibile`: una base che la grammatica non legge da un lato, con due stringhe diverse; oppure il
      componente deciso proposto dal nuovo, con la base del codice deciso che non si legge (il badge ha il motivo
      `codice_registrato_non_leggibile`);
    - `documento_senza_componente`: il documento deciso non ha componente (un documento commerciale), e con la
      decisione non c'è niente da mettere a fianco della proposta del nuovo (T-B6-183);
    - `nuovo_non_valutato`: il motore A non ha valutato il thread del file. Un file non valutato dentro un thread
      valutato resta nel campione, come astensione;
  - **«prima»** ha tre esiti: correzione (le due basi lette sono diverse), nessuna correzione (le due basi lette sono
    uguali, oppure le due stringhe sono identiche), non determinabile (gli esclusi sopra). Non c'è un ripiego sul
    confronto per stringa (R31 c A; R113: né un'uguaglianza inventata né un conflitto artificiale), e la fonte
    `operatore` da sola non è mai una correzione;
  - **il marcatore** si guarda solo con la stessa base letta da tutte e due le parti, e si conta a parte:
    `PrimaMarcatore` (due marcatori scritti e diversi: per la grammatica un'altra identità, T-B4-30) e
    `MarcatoreSoloDaUnLato` (un marcatore da un lato solo: compatibile, come nella regola di nomina). `PrimaMarcatore`
    **non si somma** a `Prima`: se il titolo della misura sia la sola base o la base con il marcatore lo sceglie
    l'utente (D-R114, aperta in `domande-a1c.md`). I due numeri restano separati e visibili;
  - **«dopo»** = `FalseAssociazioni` (il nuovo propone candidati, ma non il componente deciso con la sua base) +
    `Ambiguita` (il componente deciso fra più candidati, o l'associazione ambigua o discordante, anche con il badge
    `uguale`) + `Astensioni` (nessun candidato, anche fuori richiesta, oppure il file non valutato in un thread
    valutato). Il badge non cambia: l'ambiguità si conta qui, e «dopo» non è più il numero delle regressioni;
  - **la sola revisione** (`SoloRevisione`) sta a parte, sui valutabili.
- **Determinismo**: righe in ordine di `AllegatoID` (e d'impronta per due righe con lo stesso allegato), esiti contro
  l'atteso per allegato e sezione, prodotti per base e codice richiesto, diagnostiche per (codice, percorso, riferimenti,
  messaggio). Lo stesso ingresso, comunque permutato, dà gli stessi byte canonici. Nessun orario, nessun ordine da una
  map.
- **Puro**: nessun DB, file, orologio, rete; delle foglie solo `evidenze.Diagnostica` con le costanti `Gravita*` e
  `Natura*`, e `jsoncanonico.ImprontaDi` (F0-11).
- **L'impronta** è lo sha256 del canonico dell'esito con `Impronta` vuota e senza `ControAtteso` e
  `ProdottiControAtteso`; comprende la versione, le righe, i conteggi, le correzioni e le diagnostiche. **Vuota vuol dire
  «non calcolata»**: l'esito non ha un canonico (una stringa non UTF-8 o un tempo fuori dall'intervallo di JSON negli
  ingressi). Due impronte vuote non dicono che due esiti coincidono: chi chiama la tratta come un errore, e il banco esce
  con 1 (R-48).

## Limiti noti

- **La misura delle correzioni manuali** si regge sulla lettura del vecchio motore registrata nella proposta: con gli
  export, e con le proposte senza `dettagli.valutazione`, i decisi escono tutti con `senza_lettura_vecchia` e il
  denominatore è zero. Il titolo della misura (`Prima` da solo, oppure con `PrimaMarcatore`) resta da scegliere
  (D-R114). `CodiceLettoMarcatore` lo riempie `valutazione` e lo copia il chiamante (`passaggio.go`): se la copia manca,
  `PrimaMarcatore` resta a zero e un marcatore del solo codice deciso conta come `MarcatoreSoloDaUnLato`.
- **Le righe riscritte con la regola `operatore`** (la dimensione del codice di `dettagli.valutazione` scritta con la
  regola `operatore`, cioè la decisione di una persona registrata come lettura) arrivano con `CodiceLetto` vuoto,
  perché `valutazione` lo svuota (T-B6-22), e con una fonte della proposta che non è `operatore`. La misura le esclude
  con `senza_lettura_vecchia`, non con `solo_origine_manuale`: restano fuori dal denominatore, ma con l'altro motivo.
  Distinguerle vorrebbe un campo in più nel record piatto di `valutazione` (T-B6-188).
- **La diagnostica dell'indicatore** si dà solo se c'era qualcosa da confrontare (R-46): il file è valutato, il vecchio
  ha un codice o una rev registrata, e almeno un lato porta una revisione (`Vecchio.Revisione`, `Vecchio.Rev`,
  `Nuovo.Revisione`), oppure le letture del file ne danno di discordi. Per gli altri file l'indicatore è
  `non_determinabile` con il motivo di `valutazione`, senza nota.
- **Un candidato del livello componente con il target `identita:…`** (il componente deciso sta fra i nodi di
  un'identità che ha anche nodi non decisi) dà regressione: il record piatto non porta le posizioni del candidato, e il
  legame con il componente non si vede (T-B6-51, variante; va a B7).
- **`Decisione`, `Invarianti`, `BaseLetta`** dell'atteso non si valutano qui: li controlla il runner.

## Dipendenze

- **Importa:** `core/estrazione/evidenze` (la diagnostica, con gravità e natura), `platform/jsoncanonico` (l'impronta),
  `github.com/google/uuid`; della libreria standard `time` solo come tipo.
- **Mai:** i pacchetti del motore (`motorea`, `grammatica`, `ancoraggio`, `estrazione`), `fotorfq`, il caricatore,
  `valutazione`, la libreria YAML (grafo del par.3.2.1). Che `valutazione` non importi `confronto` lo controlla la sua
  voce in `dipendenze_test.go`.
- **È importato da:** in P9 `app/bancoa`; in A1d `transport/web`.

## Test

- **`badge_test.go`** — livello L1 — A1c-L1-18: la tavola dei badge riga per riga, con i motivi; l'ambiguo visibile; le
  righe «come C5»; l'indicatore di revisione con la diagnostica.
- **`atteso_test.go`** — livello L1 — A1c-L1-19: la tavola degli esiti riga per riga con il peso; la voce che dipende da
  una domanda aperta; i non coperti e le voci senza file; le sezioni neutre; `ConfrontaProdotti`; l'atteso non cambia
  l'impronta.
- **`correzioni_test.go`** — livello L1 — A1c-L1-20, la parte di `confronto`, con la definizione di R114: il
  denominatore e gli esclusi per motivo; la sola origine manuale e la lettura mancante fuori dal denominatore; «prima»
  con le basi lette (F0-19), senza il ripiego per stringa; l'ambiguità contata con il badge `uguale`; il marcatore a
  parte, non sommato; lo stesso campione per prima e dopo; la somma campo per campo.
- **`confronto_test.go`** — livello L1 — il determinismo (permutazioni, impronta ricalcolata, la stessa senza atteso), la
  versione, l'esito vuoto, A1c-L1-32 (la parte dei codici emessi), la forma dei DTO piatti (CP.2 con F0-19 e con
  `CodiceLettoMarcatore` di R114).
- **`scena_db_test.go`** — livello L4 (tag `integrazione`) — la scena ACME scritta con SQL sul database di prova, la
  grammatica e l'insieme delle regole ACME, il percorso del banco e dell'anteprima (`caricatore.Carica`,
  `valutazione.Calcola`, la copia nei DTO via JSON, `Confronta`), comuni alle due L4.
- **`motore_senza_llm_db_test.go`, `ro_dominio_db_test.go`** — livello L4 (tag `integrazione`, `cockpit_test`) —
  A1c-L4S-08 e -09: `caricatore.Carica`, `valutazione.Calcola` e `Confronta` sulla scena ACME, con i suggerimenti
  dell'agente e senza, e il DB prima e dopo. Stanno nel pacchetto di prova `confronto_test`, che importa anche
  `valutazione`, il caricatore e `testutil`: le frecce delle prove non contano nel grafo (G1 legge i soli file non di
  prova), e sono dichiarate in `internal/README.md`.
- **Fuori di qui:** A1c-L1-28 (la voce di `confronto` e la prova sui selettori delle foglie) in
  `core/estrazione/evidenze/dipendenze_test.go`; A1c-L1-32 (l'elenco d'oro) in `codici_diagnostica_test.go`; A1c-L1-31
  (i gemelli e la copia) in `app/bancoa/passaggio_test.go` (Q9).

## Leggi anche

- `internal/core/valutazione/README.md` (i record piatti, il vecchio letto con la grammatica)
- `internal/core/README.md`
- `internal/README.md`
