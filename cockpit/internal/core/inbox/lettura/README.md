---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/inbox/lettura`

- il corpo di una mail pronto da leggere

## Scopo

- Trasforma il corpo di un messaggio (`corpo_testo` e `corpo_html`, come li ha scritti l'ingest) in una sequenza
  di **blocchi** da mostrare:
  - il testo scritto da una persona,
  - le tabelle incollate da Excel come tabelle,
  - il testo automatico di Outlook e dei server di posta **chiuso** (mai tolto: resta dentro, a un clic),
  - la storia citata a parte.
- Dice anche se l'oggetto porta l'etichetta di posta esterna, per mostrarlo senza.
- È solo presentazione: il package non scrive niente, non legge il database, non tocca il disco.
- Il corpo memorizzato non cambia, e il testo originale resta sempre raggiungibile intatto (`Corpo.Originale`).

## Non appartiene qui

- Il taglio della catena (dove finisce il testo nuovo e comincia la storia citata): `core/inbox/classificazione`
  (`TagliaCatena`; per le tabelle con l'origine, il `Taglio` di `TagliaCatenaConPosizioni`)
  - che qui si usa e non si rifà.
- Il documento delle evidenze di una mail (segmenti, unità, diagnostiche `email.*`): l'adattatore di
  `core/estrazione` (A1b.7), che usa `TabelleConOrigine` e `TestoDaHTML`. Qui nessuna diagnostica e nessun tipo
  del motore A.
- L'interpretazione del testo (codici, triage, aggancio): `core/inbox/classificazione`, `core/inbox/aggancio`.
  - Il testo dato all'interpretazione NON passa da qui.
- Il salvataggio del corpo: `core/inbox/ingest` (`corpo_testo` com'è arrivato, NUL sostituiti).
- L'HTML della pagina:
  - i template `corpo_messaggio` / `corpo_blocco` in `web/templates/frammenti.html`
  - e le classi `.corpo-*` in `web/static/style.css`;
  - i gestori in `transport/web`.

## File

- **`lettura.go`**
  - **Responsabilità:** I tipi (`Corpo`, `Blocco`, `Pezzo`, `Tabella`, `RigaTabella`, `Cella`, `Motivo`),
  - i limiti (`LimiteTesto`, `LimiteHTML`),
  - `Presenta` (con il `recover` finale) ed `elabora`;
  - la normalizzazione (`fineRiga`, `perConfronto` per le regole, `perVista` per la schermata);
  - la `sezione` (righe, proprietario di ogni riga, `occupa`, `paragrafi`), `blocchi`, `unisciRumore`, `righeVista`.
  - Commento `// Package`
- **`rumore.go`**
  - **Responsabilità:** Le regole del rumore N1–N7 (`teams`, `primoContatto`, `bannerEsterno`, `outlookMobile`, `ambiente`,
    `riservatezza` con `clausola` e l'insieme K, `separatori`),
  - le etichette,
  - `èRumoreDiTabella`
- **`collegamenti.go`**
  - **Responsabilità:** Le riscritture in linea I1–I5:
    - `SrotolaSafeLinks`,
    - `srotolaTutto`,
    - `pezziDaRighe` / `pezziRiga` / `pezziSegmento`,
    - `doppione` (annotazione `<mailto:…>` / `<tel:…>` / `<https:…>` uguale al testo dell'ancora),
    - `mostraIndirizzo`
- **`tabelle_html.go`**
  - **Responsabilità:** La lettura del corpo HTML con `golang.org/x/net/html`:
    - `leggiHTML`,
    - `esaminaTabella` (i criteri della tabella di dati),
    - `leggiCella`,
    - `intestazione`,
    - `vista`;
  - `documentoHTML.testo` (il testo dall'HTML quando quello semplice manca) e il ripiego `testoDaToken`
- **`tabelle_origine.go`** (giro 5, A1b.3; piano A, 5.4.4)
  - **Responsabilità:** le **tabelle con la loro origine**, per il motore A:
    - `TabelleConOrigine` → `[]TabellaOrigine` / `CellaOrigine`: le stesse tabelle di dati e gli stessi testi di
      `vista()`, più gli indici (tabella, riga dopo la pulizia e `<tr>` originale, cella, colonna nella griglia),
      il percorso nel DOM, le righe del `Taglio` a cui ogni cella si aggancia e `Coincide`; le tabelle non
      agganciate e quelle a cavallo del taglio (`ACavallo`)
    - `TestoDaHTML`: il testo che `Presenta` ricava dall'HTML quando il testo semplice manca
  - `tabelle_html.go` non cambia: la lettura delle celle è una variante del ciclo di `esaminaTabella`
    (`esaminaTabellaConOrigine`) con gli stessi aiuti e limiti; l'aggancio è quello di `ancora`, sulle righe del
    corpo originale (`agganciaConOrigine`). L'alternativa dei campi in più in `cellaHTML` è esclusa: toccherebbe
    `tabelle_html.go` (5.10 n.5)
- **`ancoraggio.go`**
  - **Responsabilità:** `ancora`: ogni tabella HTML si mostra solo dove le sue parole compaiono identiche, di seguito, da inizio a fine
    riga nel testo
  - (`cerca`, Knuth–Morris–Pratt sulle parole)
- **`tabelle_tab.go`**
  - **Responsabilità:** Il ripiego senza HTML: righe con le celle separate da TAB (`celleTab`, `tabelleTab`, `tabellaTab`)
- **`oggetto.go`**
  - **Responsabilità:** `OggettoVisibile`: toglie `[EXTERNAL]`, `[EXT]`, `[ESTERNO]` e simili davanti all'oggetto (dopo `RE:`/`FW:`)
- **`testdata/`**
  - **Responsabilità:** `excel.html` (una tabella incollata da Excel in Word HTML),
  - `excel_tab.txt`,
  - `excel_una_cella_per_riga.txt`

## Entry point

- **`Presenta(testo, html) Corpo`**
  - **Chi lo chiama:**
    - `transport/web/routes_inbox.go:caricaMessaggio` (`messaggioDati.Corpo`, pannello dell'Inbox)
    - e `transport/web/thread.go:caricaThread` (`messaggioThread.Corpo`, pagina della RFQ)
- **`OggettoVisibile(oggetto) (visibile, esterno)`**
  - **Chi lo chiama:** la funzione di template `oggettoVisibile` in `transport/web/server.go` (lista dell'Inbox,
    pannello, pagina della RFQ)
- **`SrotolaSafeLinks(u) (originale, ok)`**
  - **Chi lo chiama:** usata dentro il package;
  - esportata per le prove e per chi dovrà srotolare un indirizzo altrove
- **`TabelleConOrigine(testo, html, taglio) []TabellaOrigine`, `TestoDaHTML(html) string`**
  - **Chi lo chiama:** ancora nessuno nel prodotto; li userà l'adattatore della mail di `core/estrazione`
    (A1b.7), che di questo package può usare solo questi nomi e i tipi `TabellaOrigine`, `CellaOrigine` (G4)
- **`LimiteTesto`, `LimiteHTML`, i tipi e le costanti `Blocco*`, `Pezzo*`, `Motivo*`, `Origine*`**
  - **Chi lo chiama:** i template di `frammenti.html` confrontano `Tipo` con le stringhe `testo`, `tabella`,
    `rumore`, `separatore`, `link`, `immagine`

## Dati

- Nessuno.
- Il package è puro:
  - non legge né scrive il database,
  - non tocca il disco,
  - non ha stato globale mutabile (le espressioni regolari si compilano una volta all'avvio).
- Le funzioni sono deterministiche.

## Flussi principali

### `Presenta(testo, html)`

1. Oltre `LimiteTesto` (1 MiB) nessuna elaborazione: `Ridotto = true`, solo `Originale`.
2. L'HTML si legge solo se serve e se non supera `LimiteHTML` (2 MiB):
   - quando contiene almeno una `<table`,
   - o quando il testo semplice è vuoto.
   - Nel secondo caso il testo si ricava dall'HTML (`DaHTML = true`).
3. `classificazione.TagliaCatena` divide il testo in parte fresca (`Blocchi`) e storia citata (`Storia`);
   - le due sezioni hanno numerazioni di riga separate.
4. Le tabelle HTML di dati si **ancorano** nel testo, prima nella parte fresca, poi nella citata.
   - Una tabella che non si ritrova parola per parola non si mostra: le righe restano testo.
5. Su ogni sezione, in quest'ordine:
   - il rumore (Teams, primo contatto, posta esterna, Outlook mobile, invito a non stampare, clausola di
     riservatezza dal fondo),
   - i separatori,
   - il ripiego TAB.
6. Le righe rimaste libere diventano blocchi `testo` con le riscritture in linea;
   - i blocchi di rumore che si toccano si uniscono («testo automatico: posta esterna, primo contatto»).

### Il rumore (chiuso, mai tolto)

- Ogni regola è stretta di proposito (posizione, lunghezza, un'espressione che una persona non scrive per caso):
  - **`banner_esterno`**
    - Che cosa: l'avviso «questa mail viene dall'esterno», o la riga che è solo `[EXTERNAL]`
    - Dove vale: fra i primi due o gli ultimi due paragrafi, ≤ 6 righe e ≤ 600 rune; la riga-etichetta solo in cima
  - **`primo_contatto`**
    - Che cosa: il suggerimento di Microsoft 365 «non ricevi spesso messaggi da…»
    - Dove vale: paragrafo ≤ 4 righe e ≤ 500 rune, con il link di Microsoft o con la frase e un indirizzo
  - **`outlook_mobile`**
    - Che cosa: «Inviato da Outlook per iOS», «Sent from my iPhone» e simili
    - Dove vale: riga intera
  - **`riunione_teams`**
    - Che cosa: l'invito a una riunione Teams
    - Dove vale: solo se il blocco contiene il link per entrare (anche dentro Safe Links)
  - **`riservatezza`**
    - Che cosa: la clausola legale
    - Dove vale: solo in coda alla sezione, 120–3000 rune e due concetti del gergo legale (uno con la riga-titolo);
      una clausola seguita da testo di lavoro non si chiude
  - **`ambiente`**
    - Che cosa: l'invito a non stampare, con il glifo Webdings sopra
    - Dove vale: riga ≤ 250 rune
- Le righe di `_`, `-` o `=` (almeno 10) rimaste libere diventano un filetto (`separatore`).

### Le riscritture in linea (dentro un blocco di testo)

- Un indirizzo Safe Links si srotola all'indirizzo originale
  - (fino a tre involucri; solo `http`, `https`, `mailto`, `ftp`; altrimenti resta com'era),
  - con la nota «la protezione al clic non vale qui».
- Un'annotazione `<mailto:…>` / `<tel:…>` / `<https:…>` che ripete il testo dell'ancora si toglie;
  - se è diversa resta, in grigio, senza `mailto:`.
- `[cid:…]`, il testo alternativo automatico di Office e un'immagine collegata diventano `[immagine]`,
  - con il nome o il testo alternativo nel `title`.
- Gli indirizzi **non** diventano mai cliccabili.
- In vista:
  - gli spazi unificatori diventano spazi,
  - gli spazi finali spariscono,
  - due o più righe vuote diventano una.

### Le tabelle

- **Tabella di dati HTML:**
  - nessuna tabella annidata nelle celle (l'esterna è impaginazione, le interne si esaminano),
  - 2–200 righe non vuote,
  - 2–30 colonne effettive,
  - almeno 2 righe con 2 celle piene,
  - testo ≤ 64 KiB,
  - non un avviso (N1, N2, Teams),
  - né lei né un antenato nascosti (`display:none`, `mso-hide:all`);
  - al più 50 per messaggio.
- **Celle:**
  - con `Colspan`/`Rowspan` limitati,
  - `Numero` (allineata a destra) dall'allineamento o dal testo,
  - `Troncata` oltre 500 rune (il confronto usa il testo intero).
- **Intestazione:** la riga 0 tutta `<th>`, o tutta in grassetto quando le righe dopo non lo sono.
- **Ripiego TAB:**
  - almeno due righe di fila con almeno due celle piene,
  - al più 200 righe e 30 colonne,
  - colonne che variano al più di una;
  - mai intestazione.

### Le tabelle con l'origine (motore A, giro 5)

- `TabelleConOrigine(testo, html, taglio)`:
  - `testo` è quello su cui il `Taglio` è calcolato: il `corpo_testo`, o `TestoDaHTML(html)` quando è vuoto, come
    in `Presenta`; stessi limiti di `Presenta` (`LimiteTesto`, `LimiteHTML`, almeno una `<table`).
  - Le tabelle di dati sono quelle di `leggiHTML`, con gli stessi criteri, nello stesso ordine.
  - L'aggancio è quello di `ancora`, prima nella parte corrente poi nella storia, ma sulle righe del `Taglio`, cioè
    sul corpo originale: le righe di una cella sono righe di `corpo_testo`.
  - Una cella vuota non ha righe; una cella «coincide» quando le sue parole sono esattamente quelle delle sue
    righe, e solo allora l'adattatore le dà un intervallo esatto.
  - Una tabella che non si ritrova resta non agganciata; una che si ritrova solo a cavallo del taglio si segnala
    (`ACavallo`) e non si aggancia. La diagnostica la scrive l'adattatore.
- Con l'inoltro senza confine (R48 A) l'adattatore passa lo stesso `Taglio` (`nessuna_storia`): le righe delle
  celle non cambiano, cambia solo il segmento a cui l'adattatore le assegna.

### `OggettoVisibile`

- Toglie l'etichetta fra parentesi quadre solo in testa all'oggetto,
  - dopo i prefissi `RE:`/`FW:`/`I:`/`TR:`/`AW:`/`WG:`,
  - ripetuta fino a 20 volte;
  - «Offerta [EXT-2]» resta com'è.
- Senza etichetta restituisce l'oggetto intatto.

## Invarianti

- **Il corpo memorizzato non cambia**:
  - nessuna funzione scrive niente;
  - `Corpo.Originale` è `corpo_testo` com'è.
- **Mai HTML della mail nella pagina**:
  - dall'HTML si prende solo testo (celle, o righe quando il testo semplice manca).
  - Script, stili, attributi e marcatori non arrivano nel modello;
  - tutto esce come testo semplice e il template lo scrive con l'escape.
- **Nessuna parola persa**:
  - ogni riga di una sezione sta in esattamente un blocco (`[Da,A)` coprono la sezione senza buchi né
    sovrapposizioni)
  - e ogni blocco porta le sue righe grezze (`Grezzo`).
  - Le celle di una tabella HTML dicono le stesse parole delle righe che sostituiscono.
- **Nel dubbio il testo resta testo**:
  - una regola che sbaglia costa un blocco chiuso di troppo o una tabella mancata,
  - mai testo nascosto senza modo di aprirlo.
- **Mai panic**:
  - `Presenta` ha un `recover` finale che ripiega su `Corpo{Originale, Ridotto: true}`;
  - `elabora` (senza rete) è quello che provano i test e il fuzz.
- Le regole girano su una copia normalizzata (`perConfronto`: spazi unificatori, caratteri invisibili, apostrofo
  tipografico);
  - quello che si mostra è il testo del messaggio (`perVista` tocca solo gli spazi unificatori).
- Deterministico: stesso input, stesso `Corpo`.
- **Le tabelle con l'origine non cambiano la vista** (giro 5): `tabelle_html.go` e `ancoraggio.go` sono gli
  stessi di prima, e `TabelleConOrigine` dà le stesse tabelle e gli stessi testi di `vista()` (troncati a 500
  rune solo nella vista: qui il testo è intero) e le stesse tabelle agganciate di `Presenta`. Il ciclo di
  `esaminaTabella` è duplicato: una modifica ai criteri va riportata in `esaminaTabellaConOrigine`, e la ferma
  l'equivalenza di `tabelle_origine_test.go` (A1b-10).
- Le righe di una cella sono indici in `Taglio.Righe`, sul corpo originale; nessun offset nei byte di
  `corpo_html`, solo il percorso nel DOM (che dipende dalla versione del parser HTML).

## Dipendenze

- **Importa:**
  - la libreria standard,
  - `golang.org/x/net/html` (il parser HTML5, tollerante),
  - `core/inbox/classificazione` (solo `TagliaCatena` e il taglio con posizioni: `Taglio`, `RigaTesto`).
- **È importato da:** `transport/web` (`routes_inbox.go`, `thread.go`, `server.go`).
- Gli archi `lettura → classificazione` e `transport → lettura` sono nella tabella di `internal/README.md`.

## Test

- Tutti L1 (nessun database, nessun tag), in `go test ./internal/core/inbox/lettura/`:
  - 60 test,
  - `FuzzPresenta`,
  - `BenchmarkPresenta` (un HTML di Word da circa 200 KB: pochi millisecondi)
  - e `BenchmarkPresentaCasiEstremi`.
- Dati solo di fantasia (ACME, PZ-001, `@acme.example`).
- **`lettura_test.go`**
  - **Che cosa prova:** le invarianti su ogni fixture (copertura senza buchi, parole delle celle = parole delle righe, `Originale`
    intatto),
  - mail semplice = un blocco,
  - vuoto,
  - oltre il limite,
  - determinismo;
  - il fuzz e i benchmark
- **`rumore_test.go`**
  - **Che cosa prova:** N1–N7, ognuno con il suo gemello negativo (frase simile a metà mail, prosa che cita Teams, clausola seguita da
    testo di lavoro);
  - unione dei blocchi;
  - `SoloRumore`
- **`collegamenti_test.go`**
  - **Che cosa prova:** Safe Links (singolo, doppio, `javascript:` che resta),
  - annotazioni doppie,
  - immagini,
  - `OggettoVisibile`
- **`tabelle_test.go`**
  - **Che cosa prova:** tabelle da Excel (testo a TAB, una cella per riga, solo HTML),
  - colspan/rowspan,
  - impaginazione annidata,
  - avvisi in tabella,
  - testo diverso dall'HTML,
  - seconda tabella nella storia,
  - limiti,
  - HTML malformato, ostile, nascosto;
  - il ripiego TAB e i suoi negativi
- **`catena_test.go`**
  - **Che cosa prova:** la risposta citata va in `Storia`,
  - l'inoltro senza commento non ha storia,
  - un ciclo di lavorazione «Da: … A: …» resta testo
- **`tabelle_origine_test.go`** (giro 5, A1b.3)
  - **Che cosa prova:** A1b-10, le tabelle con l'origine sono quelle di `vista()` (stessi testi, span, numeri,
    intestazione) e le agganciate sono quelle di `Presenta`, su `excel.html` (testo a TAB, una cella per riga,
    solo HTML) e su tabelle sintetiche; le parole delle righe sono quelle delle celle;
  - A1b-11, la forma di F-MAIL-4 (una cella per riga, CRLF, «Q.TA» nella seconda riga, la riga fantasma di Word):
    righe, `Coincide`, `RigaHTML` prima della pulizia, colonna e percorso di ogni cella; le righe a TAB, che non
    coincidono; una cella su due righe; le colonne con il rowspan;
  - A1b-12, F-MAIL-5: la tabella che il testo non dice, quella a cavallo del taglio, il testo marcato, il corpo
    vuoto con solo HTML, il ripiego a TAB, i separatori tedesco e francese, i limiti, un taglio di un altro testo
- **Altrove:** `transport/web/corpo_test.go` (i template `corpo_messaggio` / `corpo_blocco` con un corpo di prova).

## Stato dell'implementazione

- **Completo** per il pannello dell'Inbox e la pagina della RFQ:
  - rumore N1–N7,
  - riscritture I1–I6,
  - tabelle HTML ancorate,
  - ripiego TAB,
  - oggetto senza etichetta di posta esterna.
- **Non usato (per scelta, oggi):**
  - l'interpretazione (`classificazione`, `RilevaPortale`) e l'agente AI lavorano ancora sul corpo grezzo, Safe
    Links compresi;
  - i nomi delle cartelle NAS e il confronto dell'oggetto usano l'oggetto memorizzato, etichetta `[EXTERNAL]`
    compresa.

## Dove intervenire

- **Voglio… capire perché un pezzo di testo è chiuso**
  - Apri: `rumore.go` (la funzione del suo `Motivo`), l'ordine in `sezione.rumore`
- **Voglio… capire perché un avviso NON è chiuso**
  - Apri: la regola in `rumore.go` e i suoi limiti di posizione e lunghezza;
  - aggiungi un caso in `rumore_test.go` prima di allargarla
- **Voglio… capire perché una tabella non si vede come tabella**
  - Apri:
    - `tabelle_html.go:esaminaTabella` (criteri),
    - `ancoraggio.go:ancora` (le parole devono coincidere),
    - `tabelle_tab.go:tabellaTab`
- **Voglio… cambiare come si mostra un blocco**
  - Apri: `web/templates/frammenti.html` (`corpo_messaggio`, `corpo_blocco`) e le classi `.corpo-*` in
    `web/static/style.css`
- **Voglio… aggiungere un'etichetta di posta esterna nell'oggetto**
  - Apri: `oggetto.go:reEsternoOggetto`
- **Voglio… cambiare i limiti**
  - Apri: le costanti in testa a `lettura.go`

## Leggi anche

- `internal/core/inbox/classificazione/README.md` (`TagliaCatena`)
- `internal/transport/web/README.md` (il pannello dell'Inbox e la pagina della RFQ)
- `web/README.md` (template e CSS)
- `internal/README.md` (dipendenze ammesse)
