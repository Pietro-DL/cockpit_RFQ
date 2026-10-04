---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/estrazione` — gli adattatori del motore A

## Scopo

- Porta **i fatti dentro `DocumentoEvidenze`** (giro 5, A1b; piano A, par.3.3.5 e 5.4.5): dai record della
  fotografia (`core/fotorfq`) e dai fatti del worker a un documento con fonte, testi originali, entità, unità,
  localizzatori, legami e qualità, senza applicare nessuna regola cliente.
- **`DaAllegato`**: il nome del file (e, per una voce d'archivio, il percorso) più il contenuto secondo i fatti:
  - lo **STEP** (`mappatura-step-1`): un'entità per nodo, un'unità per attributo grezzo, un legame
    `padre_figlio_step` per coppia con la quantità, le radici da `struttura.radici`, la formazione come dato
    grezzo mai confrontato (D1, E-12);
  - il **PDF** (`mappatura-pdf-1`): i campi del cartiglio della pagina 1 legati all'entità `disegno`, con pagina,
    riquadro in decimi di punto, zona e fonte; ogni frammento come unità `testo_pdf` con un'entità
    `unita_isolata` sua; i metadati; lo stato del testo con la regola di `testoDelPDF`, replicata (un PDF senza
    testo nativo, non letto, illeggibile o non analizzato ha la capacità `testo` non disponibile, mai «nessun
    codice», A-C09); `elenco_pdf` sempre non disponibile; il cartiglio di una sottoversione di prima come
    `testo_pdf`, un indizio;
  - un file senza fatti, o con fatti che nessuna mappatura legge (solo l'esito del worker, una natura diversa
    da «file», un archivio che l'acquisizione non apre), dà **il solo nome**, con la capacità `contenuto` non
    disponibile e il motivo. Non è mai un file «senza codici».
- **`DaMessaggio`** (`mappatura-email-1`): oggetto, segmenti con gli offset sul `corpo_testo` come sta nel DB,
  livelli della storia, tabelle HTML agganciate:
  - `s:corrente` sempre, con l'oggetto; un segmento per livello della storia (`s:storia:<k>`), «inoltro» solo
    per una frase d'inoltro, «citazione» per ogni altro confine (R27 c), al più `maxLivelliStoria` = 8;
  - **l'inoltro senza confine nel corpo** (oggetto con il prefisso d'inoltro, `EInoltro`; nessun confine): la
    storia non è separabile, tutto il corpo è `s:storia:1` di tipo «inoltro», e niente diventa corrente da solo
    (R48 A); lo stesso stato, con un altro motivo, quando un confine lascia vuota la parte corrente (E-22);
  - le tabelle: un'entità `riga` per riga e un'unità per cella non vuota, esatta solo quando la cella coincide
    con le sue righe del testo (R28 b); le tabelle che non si agganciano in un segmento solo non danno unità;
  - firma e sezione tecnica non disponibili (R27 a), la copertura dei riconoscitori dei confini dichiarata.
- **`DaTesto`**: un documento di una sola unità, per gli esempi e per i casi degli attesi (il banco, A1b.11).
- **Il principio della provenance** (5.0): il pacchetto collega due cose solo quando i fatti lo dicono — stessa
  entità (`EntitaID`), stesso nodo STEP, relazione dichiarata dal worker. Mai per vicinanza, somiglianza o
  ordine. Ciò che manca si dichiara (capacità, diagnostiche, `Provenienza.Ignoti`), mai si confonde con «vuoto».

## Non appartiene qui

- **Interpretare** i testi (codici, revisioni, ruoli, funzioni): `core/inbox/classificazione/motorea`. Qui non
  si importano `grammatica` né `motorea` (G1).
- **I tipi del documento** e la sua validazione: la foglia `core/estrazione/evidenze`, che qui si usa e non si
  estende.
- **Leggere il DB o un export**: il caricatore (A1c) riempie i record di `core/fotorfq`; qui arrivano chiusi.
- **Il taglio della catena e le tabelle HTML**: `classificazione` (`TagliaCatenaConPosizioni`,
  `LivelliDellaStoria`, `EInoltro`) e `lettura` (`TabelleConOrigine`, `TestoDaHTML`), di cui questo pacchetto
  usa solo l'elenco chiuso di G4. Nessuna regola nuova dei confini: qui i confini diventano segmenti.
- **Decidere quali segmenti valgono come richiesta**: è `UsoSegmenti`, un ingresso del motore (file dei casi,
  operatore). L'adattatore non promuove mai la storia.

## File

- **`versione.go`** — responsabilità:
  - commento `// Package`; `VersioneAdattatore` («adattatori-1»), che entra nel `BundleID`;
  - i nomi delle mappature (`mappatura-step-1`, `mappatura-pdf-1`, `mappatura-nome-1`, `mappatura-email-1`),
    scritti in `CampoOriginale.Mappatura` di ogni unità;
  - `maxLivelliStoria` = 8, il tetto dei livelli della storia (5.4.3 punto 6), parte della versione.
- **`documento.go`** — responsabilità: il costruttore comune.
  - testi, segmenti, entità, unità (con `FonteID` messo qui), legami, capacità, diagnostiche; lo stato della
    fonte, che una parte può solo peggiorare;
  - `chiudi`: ordine canonico per ID (capacità per nome, diagnostiche per codice e riferimenti), elenchi vuoti
    portati a nil, `ValidaDocumento` (un documento non valido è un errore), `BundleID` = sha256 del canonico
    senza `BundleID` e senza `Provenienza.Bytes` (R51 A); i tempi in UTC al millisecondo.
- **`fonte.go`** — responsabilità:
  - `Fonte`, `Provenienza` e `RiferimentoFatti` da allegato, messaggio e testo isolato;
  - i controlli di coerenza: contenitore, percorso senza contenitore, fatti di un altro sha256, `Digest`
    diverso dall'impronta del payload (`fotorfq.ImprontaPayload`);
  - le frasi fisse di `Provenienza.Ignoti`, che dicono anche le trasformazioni dell'acquisizione.
- **`offset.go`** — responsabilità: gli aiuti degli offset (byte UTF-8 sul testo originale); `lunghezzaPython`
  (code point Python, 2 per una runa sopra U+FFFF), mai confrontata con `len()`; gli intervalli dalle coppie
  del taglio e quelli ripuliti dagli spazi ai bordi (`ripulito`, gli spazi di `strings.TrimSpace`).
- **`nome.go`** — responsabilità:
  - la regola dello stem (`dividiNome`, C-33): l'estensione dopo l'ultimo «.» dell'ultimo pezzo, se il punto
    non è fra i punti iniziali e se dopo c'è almeno un carattere, come `os.path.splitext` del worker salvo
    «nome.»; lo stem è il resto;
  - le unità `u:nome` (selettore `nome_file`) e, per una voce d'archivio, `u:percorso` (selettore
    `voce_archivio`), localizzazione esatta (R19 d);
  - `nome.estensione_discorde`, `nome.forse_troncato`, `archivio.non_estraibile`.
- **`testo.go`** — responsabilità: `DaTesto`. Per i selettori del cartiglio e dello STEP crea l'entità
  (`disegno`, `nodo_step`); per oggetto e corpo il segmento `s:corrente`, per la storia `s:storia:1`; per il
  nome il localizzatore con stem ed estensione.
- **`allegato.go`** — responsabilità: `DaAllegato`; le chiavi del primo livello dei fatti, per sapere quali
  parti ci sono; la capacità `contenuto` dei documenti con il solo nome.
- **`step.go`** — responsabilità: la mappatura STEP dai fatti decodificati con `worker.DecodificaStruttura`;
  la qualità della lettura (troncato, scarti, v2 senza scarti, file non letto); le unità candidate troncate;
  la capacità `grafo_completo` da `Fatti.MotivoParziale`.
- **`pdf.go`** — responsabilità: la mappatura PDF dai fatti decodificati con `worker.DecodificaTestoPDF`:
  - lo stato del testo con la regola di `testoDelPDF` (`classificazione/testo_pdf.go:74-88`), replicata e non
    importata (non è esportata, e G4 non la ammette): letto, senza testo nativo, non letto, illeggibile;
  - il cartiglio (`e:pdf:p1`, `u:pdf:cartiglio:<etichetta>:<n>`), i frammenti (`e:pdf:frammento:<n>`,
    `u:pdf:frammento:<n>`), i metadati (`u:pdf:metadati:<chiave>`); i riquadri in decimi interi
    (`math.Round(x*10)`); il metodo «ocr» per ciò che ha letto l'OCR;
  - la sottoversione di prima (`testoDiPrima`): i campi del cartiglio come `testo_pdf` isolati, mappatura
    «sconosciuta», `pdf.testo_di_prima` (scelta tecnica del 5.10 n.6).
- **`email.go`** — responsabilità: `DaMessaggio`:
  - il testo (`corpo_testo`, o `TestoDaHTML` se manca, con `email.testo_da_html`), il taglio con le posizioni,
    l'indizio d'inoltro (`EInoltro`) per la storia non separabile senza confine (R48 A);
  - i segmenti `s:corrente` e `s:storia:<k>` (`LivelliDellaStoria`, al più `maxLivelliStoria`), le unità
    `u:oggetto`, `u:corpo:s:corrente`, `u:storia:s:storia:<k>`;
  - le tabelle (`TabelleConOrigine`): `e:tab:<t>:r<r>`, `u:tab:<t>:r<r>:c<c>`, `PosTabella.Esatto` solo con
    `Coincide`; i legami `intestazione_di_cella` con la riga d'intestazione che `lettura` riconosce;
  - le capacità `firma`, `sezione_tecnica`, `storia_annidata`, `tabelle`, `segmentazione`.
- **`codici_diagnostica.go`** — responsabilità: i codici `nome.*`, `archivio.*`, `step.*`, `pdf.*` ed `email.*`
  (R41 b).

## Entry point

- **`DaAllegato(a, f, contenitore)`** — chi lo chiama: da A1c la valutazione, sui record della fotografia;
  oggi le prove degli adattatori.
- **`DaMessaggio(m)`** — chi lo chiama: da A1c la valutazione, sui messaggi della fotografia; oggi le prove
  degli adattatori.
- **`DaTesto(sel, testo)`** — chi lo chiama: da A1b.11 il banco, per i casi di contratto (R52 A).
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **Nessuna regola cliente** è applicata ai testi; nessuna lettura del worker (`esito.codice`, `esito.rev`,
  `product_step`) entra nel documento.
- **Gli ID locali nascono da chiavi del contenuto**: `u:nome`, `u:percorso`, `u:testo`, `e:step:#n`, `u:step:#n:id`,
  `u:step:#n:revisione:2`, `g:step:#p>#f`, `e:pdf:p1`, `u:pdf:cartiglio:codice:1`, `u:pdf:frammento:3`,
  `u:pdf:metadati:titolo`, `s:corrente`, `s:storia:2`, `u:storia:s:storia:2`, `e:tab:1:r3`, `u:tab:1:r3:c2`,
  `g:tab:1:r1:c2>r3:c2`. Le mappe dell'evidenza del worker si leggono per chiave, mai in ordine di mappa.
  Per lo STEP l'ordine dei fatti in ingresso non cambia il documento; per il PDF i numeri sono le posizioni nei
  suoi elenchi (frammenti, campi con la stessa etichetta), che sono un fatto del worker e non l'ordine di una
  mappa.
- **Un'appartenenza sta in un campo solo** (`legami-1`, 5.4.1): l'attributo di un nodo è un'unità con
  `EntitaID`, e così un campo del cartiglio con il disegno; un frammento o un metadato ha un'entità
  `unita_isolata` sua, senza fingere un legame con il disegno; una cella appartiene alla sua riga con
  `EntitaID`, e la riga al suo segmento con `SegmentoID` (la cella non ripete il segmento). I `LegameFonte`
  nascono solo per `padre_figlio_step` e, con la riga d'intestazione che `lettura` riconosce,
  `intestazione_di_cella`, dalla cella d'intestazione alle celle della stessa colonna nella griglia dell'HTML.
  `occorrenza_step` non nasce: i riferimenti NAUO stanno nel legame della coppia (al più 20, più `Altre`).
- **Niente numeri decimali nel documento**: tempi, riquadri e confidenze del worker restano nel payload,
  coperto da `DigestPayload`.
- **Gli offset** sono in byte UTF-8 sul testo originale (`allegato.nome_file`, `allegato.path_interno`,
  `messaggio.oggetto`, `messaggio.corpo_testo` com'è nel DB, CRLF compresi), mai su un testo normalizzato. I
  segmenti della mail coprono il corpo senza buchi; le unità sono i segmenti senza gli spazi ai bordi. Il testo
  ricavato dall'HTML ha la localizzazione parziale, e una cella è esatta solo con `Coincide`. I valori STEP e PDF non hanno offset nel file: localizzazione «parziale», con il
  motivo (per il PDF: riquadro del blocco, testo già normalizzato dal worker); un metadato PDF non ha pagina né
  riquadro, localizzazione «assente».
- **«Non disponibile» non è «vuoto»**: un file senza fatti ha la capacità `contenuto` non disponibile; uno STEP
  non analizzato, con la struttura assente o non letto ha la capacità `struttura` non disponibile e la sua
  diagnostica; la completezza senza motivo dal caricatore è «non determinabile» (`grafo_completo` non
  disponibile, R32 b); un PDF senza testo nativo, non letto, illeggibile o non analizzato ha la capacità `testo`
  non disponibile e la sua diagnostica, e `elenco_pdf` non è mai disponibile; nella mail firma e sezione
  tecnica non sono disponibili, e «nessuna storia» dichiara la copertura dei riconoscitori dei confini.
- **Nessuna promozione della storia** (R48 A, E-22): la storia non separabile resta storia, con
  `email.storia_non_separabile` e il motivo; la richiesta la dice l'uso dei segmenti, mai l'adattatore.
- **Il documento passa sempre `ValidaDocumento`**; se non la passa, l'adattatore restituisce l'errore con le
  diagnostiche `documento.*`, mai il documento.
- **Puro**: niente orologio, file, DB, rete, goroutine, `uuid.New` (G2). `VersioneAdattatore` si cambia solo con
  un commit che lo dichiara.

## Il cambio deliberato di una convenzione

- Fino al giro 4 i fatti del worker si leggevano **solo** in `classificazione`
  (`classificazione/testo_pdf.go:19-22`: «i fatti del worker (`worker.TestoPDF`), che si leggono solo qui»).
- Da A1b li legge anche questo pacchetto, con le funzioni del contratto (`worker.DecodificaStruttura`; dal PDF,
  `worker.DecodificaTestoPDF`), per il motore A. È un cambio voluto (5.10 n.11): due lettori dei fatti, ognuno
  per il suo motore.
- **Il percorso esistente non cambia**: `classificazione` continua a leggere i fatti come prima, e il motore
  legacy non sa niente di questo pacchetto (G9).

## Dipendenze

- **Importa:** `core/estrazione/evidenze` (il documento), `core/fotorfq` (i record), `platform/jsoncanonico` (il
  `BundleID` e l'impronta dei testi), `platform/contratti/worker` (i fatti dello STEP e del testo dei PDF),
  `github.com/google/uuid`, la libreria standard.
- **Del legacy, solo l'elenco chiuso di G4** (P-08), per l'adattatore della mail: di `core/inbox/classificazione`
  `TagliaCatenaConPosizioni`, `Taglio`, `RigaTesto`, `StatoTaglio`, `RegolaTaglio`, `LivelliDellaStoria`,
  `LivelloStoria`, `EInoltro` (R48 A); di `core/inbox/lettura` `TabelleConOrigine`, `TabellaOrigine`,
  `CellaOrigine`, `TestoDaHTML`. I limiti della vista (`LimiteTesto`, `LimiteHTML`) non sono nell'elenco: il
  valore è ripetuto in `email.go`, e una prova lo confronta con quello di `lettura`.
- **Mai:** `grammatica`, `motorea`, il DB, `ancoraggio`, `valutazione`, `confronto`.
- **È importato da:** `internal/app/bancoa`, da A1b.11, solo per `DaTesto` (il modo `casi` del banco; F19,
  R52 A).

## Test

- I test stanno nel ramo `-qa`.
- **`golden_test.go`** — livello L1 — che cosa copre:
  - l'aiuto dei golden (5.6.3): confronto dei byte del JSON canonico, riscrittura solo con
    `COCKPIT_AGGIORNA_GOLDEN=1`, che fa fallire la corsa; rifiutata con il tag `privato` o con il dataset
    privato impostato (A1b-18);
  - i golden degli adattatori (A1b-14 per lo STEP, A1b-15 per il PDF, A1b-16 per la mail), con gli ingressi
    in `testdata/ingressi/`, quelli dei file passati dalle funzioni del contratto del worker;
  - la versione degli adattatori, delle mappature e di `maxLivelliStoria`, fissata.
- **`nome_test.go`** — livello L1 — che cosa copre (A1b-13): la tabella dei nomi (stem ed estensione, voce con
  cartella, NFC e NFD, emoji, spazi ai bordi, senza estensione, punto iniziale, più punti, maiuscole, 300 rune,
  `.7z`), con intervalli e diagnostiche.
- **`testo_test.go`** — livello L1 — che cosa copre: `DaTesto` su tutti i contesti, con l'entità o il segmento.
- **`allegato_test.go`** — livello L1 — che cosa copre (A1b-17): senza fatti, solo esito, natura diversa da
  file, voce di zip con contenitore, `.7z` senza voci; i record incoerenti sono errori.
- **`campi_test.go`** — livello L1 — che cosa copre (A1b-04): ogni costante `worker.Campo*` ha la sua voce fra i
  campi del cartiglio della foglia.
- **`step_test.go`** — livello L1 — che cosa copre (A1b-14): gli ingressi F-STEP-1…10 contro i golden; le
  proprietà della mappatura (ordine dei fatti, candidate troncate, `lunghezzaPython`).
- **`pdf_test.go`** — livello L1 — che cosa copre (A1b-15): gli ingressi F-PDF-1…11 contro i golden; lo stato
  del testo uguale a quello di `classificazione.StatoDelTestoPDF` su ogni ingresso; senza testo, non letto,
  illeggibile, non analizzato; il cartiglio con il disegno e i frammenti isolati; il cartiglio di prima; i
  riquadri in decimi.
- **`email_test.go`** — livello L1 — che cosa copre (A1b-16, A1b-25): gli ingressi F-MAIL-1…6 e 4b contro i
  golden; l'inoltro senza confine con la storia non separabile e lo stesso corpo senza prefisso (R48 A); la
  storia senza commento; i segmenti che coprono il corpo; il tetto dei livelli; le celle esatte e parziali; le
  tabelle fuori da un segmento; i limiti della vista uguali a quelli di `lettura`; le capacità dichiarate;
  l'oggetto assente e vuoto.

## Leggi anche

- `internal/core/estrazione/evidenze/README.md`
- `internal/core/fotorfq/README.md`
- `internal/platform/contratti/README.md`
- `internal/core/inbox/classificazione/README.md` (il taglio con le posizioni)
- `internal/core/inbox/lettura/README.md` (le tabelle con la loro origine)
- `internal/README.md`
