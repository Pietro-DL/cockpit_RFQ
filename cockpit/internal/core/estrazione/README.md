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
  userà solo l'elenco chiuso di G4.
- **La mail**: arriva con la sua mappatura (A1b.7).

## File

- **`versione.go`** — responsabilità:
  - commento `// Package`; `VersioneAdattatore` («adattatori-1»), che entra nel `BundleID`;
  - i nomi delle mappature (`mappatura-step-1`, `mappatura-pdf-1`, `mappatura-nome-1`, `mappatura-email-1`),
    scritti in `CampoOriginale.Mappatura` di ogni unità.
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
  (code point Python, 2 per una runa sopra U+FFFF), mai confrontata con `len()`.
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
- **`codici_diagnostica.go`** — responsabilità: i codici `nome.*`, `archivio.*`, `step.*` e `pdf.*` (R41 b).

## Entry point

- **`DaAllegato(a, f, contenitore)`** — chi lo chiama: da A1c la valutazione, sui record della fotografia;
  oggi le prove degli adattatori.
- **`DaTesto(sel, testo)`** — chi lo chiama: da A1b.11 il banco, per i casi di contratto (R52 A).
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima dei suoi chiamanti.

## Invarianti

- **Nessuna regola cliente** è applicata ai testi; nessuna lettura del worker (`esito.codice`, `esito.rev`,
  `product_step`) entra nel documento.
- **Gli ID locali nascono da chiavi del contenuto**: `u:nome`, `u:percorso`, `u:testo`, `e:step:#n`, `u:step:#n:id`,
  `u:step:#n:revisione:2`, `g:step:#p>#f`, `e:pdf:p1`, `u:pdf:cartiglio:codice:1`, `u:pdf:frammento:3`,
  `u:pdf:metadati:titolo`. Le mappe dell'evidenza del worker si leggono per chiave, mai in ordine di mappa.
  Per lo STEP l'ordine dei fatti in ingresso non cambia il documento; per il PDF i numeri sono le posizioni nei
  suoi elenchi (frammenti, campi con la stessa etichetta), che sono un fatto del worker e non l'ordine di una
  mappa.
- **Un'appartenenza sta in un campo solo** (`legami-1`, 5.4.1): l'attributo di un nodo è un'unità con
  `EntitaID`, e così un campo del cartiglio con il disegno; un frammento o un metadato ha un'entità
  `unita_isolata` sua, senza fingere un legame con il disegno. I `LegameFonte` nascono solo per
  `padre_figlio_step` (e, con la mail, `intestazione_di_cella`).
  `occorrenza_step` non nasce: i riferimenti NAUO stanno nel legame della coppia (al più 20, più `Altre`).
- **Niente numeri decimali nel documento**: tempi, riquadri e confidenze del worker restano nel payload,
  coperto da `DigestPayload`.
- **Gli offset** sono in byte UTF-8 sul testo originale (`allegato.nome_file`, `allegato.path_interno`), mai su
  un testo normalizzato. I valori STEP e PDF non hanno offset nel file: localizzazione «parziale», con il
  motivo (per il PDF: riquadro del blocco, testo già normalizzato dal worker); un metadato PDF non ha pagina né
  riquadro, localizzazione «assente».
- **«Non disponibile» non è «vuoto»**: un file senza fatti ha la capacità `contenuto` non disponibile; uno STEP
  non analizzato, con la struttura assente o non letto ha la capacità `struttura` non disponibile e la sua
  diagnostica; la completezza senza motivo dal caricatore è «non determinabile» (`grafo_completo` non
  disponibile, R32 b); un PDF senza testo nativo, non letto, illeggibile o non analizzato ha la capacità `testo`
  non disponibile e la sua diagnostica, e `elenco_pdf` non è mai disponibile.
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
- **Potrà importare** (tabella di `internal/README.md`): `core/inbox/classificazione` e `core/inbox/lettura`,
  solo per l'elenco chiuso di G4, con l'adattatore della mail.
- **Mai:** `grammatica`, `motorea`, il DB, `ancoraggio`, `valutazione`, `confronto`.
- **È importato da:** ancora nessuno nel prodotto.

## Test

- I test stanno nel ramo `-qa`.
- **`golden_test.go`** — livello L1 — che cosa copre:
  - l'aiuto dei golden (5.6.3): confronto dei byte del JSON canonico, riscrittura solo con
    `COCKPIT_AGGIORNA_GOLDEN=1`, che fa fallire la corsa; rifiutata con il tag `privato` o con il dataset
    privato impostato (A1b-18);
  - i golden degli adattatori (A1b-14 per lo STEP, A1b-15 per il PDF), con gli ingressi in
    `testdata/ingressi/` passati dalle funzioni del contratto del worker;
  - la versione degli adattatori e delle mappature, fissata.
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

## Leggi anche

- `internal/core/estrazione/evidenze/README.md`
- `internal/core/fotorfq/README.md`
- `internal/platform/contratti/README.md`
- `internal/README.md`
