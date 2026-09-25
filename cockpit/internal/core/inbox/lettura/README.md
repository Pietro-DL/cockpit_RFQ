# `internal/core/inbox/lettura` — il corpo di una mail pronto da leggere

## Scopo

Trasforma il corpo di un messaggio (`corpo_testo` e `corpo_html`, come li ha scritti l'ingest) in una sequenza
di **blocchi** da mostrare: il testo scritto da una persona, le tabelle incollate da Excel come tabelle, il testo
automatico di Outlook e dei server di posta **chiuso** (mai tolto: resta dentro, a un clic), la storia citata a
parte. Dice anche se l'oggetto porta l'etichetta di posta esterna, per mostrarlo senza.

È solo presentazione: il package non scrive niente, non legge il database, non tocca il disco. Il corpo
memorizzato non cambia, e il testo originale resta sempre raggiungibile intatto (`Corpo.Originale`).

## Non appartiene qui

- Il taglio della catena (dove finisce il testo nuovo e comincia la storia citata): `core/inbox/classificazione`
  (`TagliaCatena`), che qui si usa e non si rifà.
- L'interpretazione del testo (codici, triage, aggancio): `core/inbox/classificazione`, `core/inbox/aggancio`.
  Il testo dato all'interpretazione NON passa da qui.
- Il salvataggio del corpo: `core/inbox/ingest` (`corpo_testo` com'è arrivato, NUL sostituiti).
- L'HTML della pagina: i template `corpo_messaggio` / `corpo_blocco` in `web/templates/frammenti.html` e le
  classi `.corpo-*` in `web/static/style.css`; i gestori in `transport/web`.

## File

| File | Responsabilità |
|---|---|
| `lettura.go` | I tipi (`Corpo`, `Blocco`, `Pezzo`, `Tabella`, `RigaTabella`, `Cella`, `Motivo`), i limiti (`LimiteTesto`, `LimiteHTML`), `Presenta` (con il `recover` finale) ed `elabora`; la normalizzazione (`fineRiga`, `perConfronto` per le regole, `perVista` per la schermata); la `sezione` (righe, proprietario di ogni riga, `occupa`, `paragrafi`), `blocchi`, `unisciRumore`, `righeVista`. Commento `// Package` |
| `rumore.go` | Le regole del rumore N1–N7 (`teams`, `primoContatto`, `bannerEsterno`, `outlookMobile`, `ambiente`, `riservatezza` con `clausola` e l'insieme K, `separatori`), le etichette, `èRumoreDiTabella` |
| `collegamenti.go` | Le riscritture in linea I1–I5: `SrotolaSafeLinks`, `srotolaTutto`, `pezziDaRighe` / `pezziRiga` / `pezziSegmento`, `doppione` (annotazione `<mailto:…>` / `<tel:…>` / `<https:…>` uguale al testo dell'ancora), `mostraIndirizzo` |
| `tabelle_html.go` | La lettura del corpo HTML con `golang.org/x/net/html`: `leggiHTML`, `esaminaTabella` (i criteri della tabella di dati), `leggiCella`, `intestazione`, `vista`; `documentoHTML.testo` (il testo dall'HTML quando quello semplice manca) e il ripiego `testoDaToken` |
| `ancoraggio.go` | `ancora`: ogni tabella HTML si mostra solo dove le sue parole compaiono identiche, di seguito, da inizio a fine riga nel testo (`cerca`, Knuth–Morris–Pratt sulle parole) |
| `tabelle_tab.go` | Il ripiego senza HTML: righe con le celle separate da TAB (`celleTab`, `tabelleTab`, `tabellaTab`) |
| `oggetto.go` | `OggettoVisibile`: toglie `[EXTERNAL]`, `[EXT]`, `[ESTERNO]` e simili davanti all'oggetto (dopo `RE:`/`FW:`) |
| `testdata/` | `excel.html` (una tabella incollata da Excel in Word HTML), `excel_tab.txt`, `excel_una_cella_per_riga.txt` |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Presenta(testo, html) Corpo` | `transport/web/routes_inbox.go:caricaMessaggio` (`messaggioDati.Corpo`, pannello dell'Inbox) e `transport/web/thread.go:caricaThread` (`messaggioThread.Corpo`, pagina della RFQ) |
| `OggettoVisibile(oggetto) (visibile, esterno)` | la funzione di template `oggettoVisibile` in `transport/web/server.go` (lista dell'Inbox, pannello, pagina della RFQ) |
| `SrotolaSafeLinks(u) (originale, ok)` | usata dentro il package; esportata per le prove e per chi dovrà srotolare un indirizzo altrove |
| `LimiteTesto`, `LimiteHTML`, i tipi e le costanti `Blocco*`, `Pezzo*`, `Motivo*`, `Origine*` | i template di `frammenti.html` confrontano `Tipo` con le stringhe `testo`, `tabella`, `rumore`, `separatore`, `link`, `immagine` |

## Dati

Nessuno. Il package è puro: non legge né scrive il database, non tocca il disco, non ha stato globale mutabile
(le espressioni regolari si compilano una volta all'avvio). Le funzioni sono deterministiche.

## Flussi principali

**`Presenta(testo, html)`.**
1. Oltre `LimiteTesto` (1 MiB) nessuna elaborazione: `Ridotto = true`, solo `Originale`.
2. L'HTML si legge solo se serve e se non supera `LimiteHTML` (2 MiB): quando contiene almeno una `<table`, o
   quando il testo semplice è vuoto. Nel secondo caso il testo si ricava dall'HTML (`DaHTML = true`).
3. `classificazione.TagliaCatena` divide il testo in parte fresca (`Blocchi`) e storia citata (`Storia`); le due
   sezioni hanno numerazioni di riga separate.
4. Le tabelle HTML di dati si **ancorano** nel testo, prima nella parte fresca, poi nella citata. Una tabella
   che non si ritrova parola per parola non si mostra: le righe restano testo.
5. Su ogni sezione, in quest'ordine: il rumore (Teams, primo contatto, posta esterna, Outlook mobile, invito a
   non stampare, clausola di riservatezza dal fondo), i separatori, il ripiego TAB.
6. Le righe rimaste libere diventano blocchi `testo` con le riscritture in linea; i blocchi di rumore che si
   toccano si uniscono («testo automatico: posta esterna, primo contatto»).

**Il rumore (chiuso, mai tolto).** Ogni regola è stretta di proposito (posizione, lunghezza, un'espressione che
una persona non scrive per caso):

| Motivo | Che cosa | Dove vale |
|---|---|---|
| `banner_esterno` | l'avviso «questa mail viene dall'esterno», o la riga che è solo `[EXTERNAL]` | fra i primi due o gli ultimi due paragrafi, ≤ 6 righe e ≤ 600 rune; la riga-etichetta solo in cima |
| `primo_contatto` | il suggerimento di Microsoft 365 «non ricevi spesso messaggi da…» | paragrafo ≤ 4 righe e ≤ 500 rune, con il link di Microsoft o con la frase e un indirizzo |
| `outlook_mobile` | «Inviato da Outlook per iOS», «Sent from my iPhone» e simili | riga intera |
| `riunione_teams` | l'invito a una riunione Teams | solo se il blocco contiene il link per entrare (anche dentro Safe Links) |
| `riservatezza` | la clausola legale | solo in coda alla sezione, 120–3000 rune e due concetti del gergo legale (uno con la riga-titolo); una clausola seguita da testo di lavoro non si chiude |
| `ambiente` | l'invito a non stampare, con il glifo Webdings sopra | riga ≤ 250 rune |

Le righe di `_`, `-` o `=` (almeno 10) rimaste libere diventano un filetto (`separatore`).

**Le riscritture in linea (dentro un blocco di testo).** Un indirizzo Safe Links si srotola all'indirizzo
originale (fino a tre involucri; solo `http`, `https`, `mailto`, `ftp`; altrimenti resta com'era), con la nota
«la protezione al clic non vale qui». Un'annotazione `<mailto:…>` / `<tel:…>` / `<https:…>` che ripete il testo
dell'ancora si toglie; se è diversa resta, in grigio, senza `mailto:`. `[cid:…]`, il testo alternativo automatico
di Office e un'immagine collegata diventano `[immagine]`, con il nome o il testo alternativo nel `title`. Gli
indirizzi **non** diventano mai cliccabili. In vista gli spazi unificatori diventano spazi, gli spazi finali
spariscono, due o più righe vuote diventano una.

**Le tabelle.** Tabella di dati HTML: nessuna tabella annidata nelle celle (l'esterna è impaginazione, le
interne si esaminano), 2–200 righe non vuote, 2–30 colonne effettive, almeno 2 righe con 2 celle piene, testo
≤ 64 KiB, non un avviso (N1, N2, Teams), né lei né un antenato nascosti (`display:none`, `mso-hide:all`); al più
50 per messaggio. Celle con `Colspan`/`Rowspan` limitati, `Numero` (allineata a destra) dall'allineamento o dal
testo, `Troncata` oltre 500 rune (il confronto usa il testo intero). Intestazione: la riga 0 tutta `<th>`, o
tutta in grassetto quando le righe dopo non lo sono. Ripiego TAB: almeno due righe di fila con almeno due celle
piene, al più 200 righe e 30 colonne, colonne che variano al più di una; mai intestazione.

**`OggettoVisibile`.** Toglie l'etichetta fra parentesi quadre solo in testa all'oggetto, dopo i prefissi
`RE:`/`FW:`/`I:`/`TR:`/`AW:`/`WG:`, ripetuta fino a 20 volte; «Offerta [EXT-2]» resta com'è. Senza etichetta
restituisce l'oggetto intatto.

## Invarianti

- **Il corpo memorizzato non cambia**: nessuna funzione scrive niente; `Corpo.Originale` è `corpo_testo` com'è.
- **Mai HTML della mail nella pagina**: dall'HTML si prende solo testo (celle, o righe quando il testo semplice
  manca). Script, stili, attributi e marcatori non arrivano nel modello; tutto esce come testo semplice e il
  template lo scrive con l'escape.
- **Nessuna parola persa**: ogni riga di una sezione sta in esattamente un blocco (`[Da,A)` coprono la sezione
  senza buchi né sovrapposizioni) e ogni blocco porta le sue righe grezze (`Grezzo`). Le celle di una tabella HTML
  dicono le stesse parole delle righe che sostituiscono.
- **Nel dubbio il testo resta testo**: una regola che sbaglia costa un blocco chiuso di troppo o una tabella
  mancata, mai testo nascosto senza modo di aprirlo.
- **Mai panic**: `Presenta` ha un `recover` finale che ripiega su `Corpo{Originale, Ridotto: true}`; `elabora`
  (senza rete) è quello che provano i test e il fuzz.
- Le regole girano su una copia normalizzata (`perConfronto`: spazi unificatori, caratteri invisibili, apostrofo
  tipografico); quello che si mostra è il testo del messaggio (`perVista` tocca solo gli spazi unificatori).
- Deterministico: stesso input, stesso `Corpo`.

## Dipendenze

Importa: la libreria standard, `golang.org/x/net/html` (il parser HTML5, tollerante), `core/inbox/classificazione`
(solo `TagliaCatena`). È importato da: `transport/web` (`routes_inbox.go`, `thread.go`, `server.go`). Gli archi
`lettura → classificazione` e `transport → lettura` sono nella tabella di `internal/README.md`.

## Test

Tutti L1 (nessun database, nessun tag), in `go test ./internal/core/inbox/lettura/`: 57 test, `FuzzPresenta`,
`BenchmarkPresenta` (un HTML di Word da circa 200 KB: pochi millisecondi) e `BenchmarkPresentaCasiEstremi`.
Dati solo di fantasia (ACME, PZ-001, `@acme.example`).

| File | Che cosa prova |
|---|---|
| `lettura_test.go` | le invarianti su ogni fixture (copertura senza buchi, parole delle celle = parole delle righe, `Originale` intatto), mail semplice = un blocco, vuoto, oltre il limite, determinismo; il fuzz e i benchmark |
| `rumore_test.go` | N1–N7, ognuno con il suo gemello negativo (frase simile a metà mail, prosa che cita Teams, clausola seguita da testo di lavoro); unione dei blocchi; `SoloRumore` |
| `collegamenti_test.go` | Safe Links (singolo, doppio, `javascript:` che resta), annotazioni doppie, immagini, `OggettoVisibile` |
| `tabelle_test.go` | tabelle da Excel (testo a TAB, una cella per riga, solo HTML), colspan/rowspan, impaginazione annidata, avvisi in tabella, testo diverso dall'HTML, seconda tabella nella storia, limiti, HTML malformato, ostile, nascosto; il ripiego TAB e i suoi negativi |
| `catena_test.go` | la risposta citata va in `Storia`, l'inoltro senza commento non ha storia, un ciclo di lavorazione «Da: … A: …» resta testo |

Altrove: `transport/web/corpo_test.go` (i template `corpo_messaggio` / `corpo_blocco` con un corpo di prova).

## Stato dell'implementazione

Completo per il pannello dell'Inbox e la pagina della RFQ: rumore N1–N7, riscritture I1–I6, tabelle HTML ancorate,
ripiego TAB, oggetto senza etichetta di posta esterna.

Non usato (per scelta, oggi): l'interpretazione (`classificazione`, `RilevaPortale`) e l'agente AI lavorano
ancora sul corpo grezzo, Safe Links compresi; i nomi delle cartelle NAS e il confronto dell'oggetto usano
l'oggetto memorizzato, etichetta `[EXTERNAL]` compresa.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| capire perché un pezzo di testo è chiuso | `rumore.go` (la funzione del suo `Motivo`), l'ordine in `sezione.rumore` |
| capire perché un avviso NON è chiuso | la regola in `rumore.go` e i suoi limiti di posizione e lunghezza; aggiungi un caso in `rumore_test.go` prima di allargarla |
| capire perché una tabella non si vede come tabella | `tabelle_html.go:esaminaTabella` (criteri), `ancoraggio.go:ancora` (le parole devono coincidere), `tabelle_tab.go:tabellaTab` |
| cambiare come si mostra un blocco | `web/templates/frammenti.html` (`corpo_messaggio`, `corpo_blocco`) e le classi `.corpo-*` in `web/static/style.css` |
| aggiungere un'etichetta di posta esterna nell'oggetto | `oggetto.go:reEsternoOggetto` |
| cambiare i limiti | le costanti in testa a `lettura.go` |

## Leggi anche

`internal/core/inbox/classificazione/README.md` (`TagliaCatena`), `internal/transport/web/README.md` (il pannello
dell'Inbox e la pagina della RFQ), `web/README.md` (template e CSS), `internal/README.md` (dipendenze ammesse).
