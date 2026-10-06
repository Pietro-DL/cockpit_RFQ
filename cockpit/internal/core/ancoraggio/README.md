---
markmap:
  initialExpandLevel: -1
  maxWidth: 520
  colorFreezeLevel: 2
---

# `internal/core/ancoraggio` — le proposte del motore A: i prodotti chiesti dalla mail

## Scopo

- I **servizi di proposta** del motore A (giro 5; piano A, par.3.3.7, 6.4.4): la classificazione dice che cosa
  significa un'evidenza, qui si decide a quali prodotti candidarla, conservando ambiguità, alternative, esclusioni
  e motivi. È puro: nessuna scrittura, nessun DB.
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

## Non appartiene qui

- **La fotografia e il DB**: `ancoraggio` non importa `core/fotorfq` né il caricatore (T-B0-04). La richiesta
  valutata, con i gesti, la compone `core/valutazione` dalla fotografia; qui arriva già fatta.
- **Il target, la sua identità e lo stato della fonte** (R70 A, R79, R65, R76 b, T-E1-09): `core/valutazione`
  (commit P7a).
- **Gli ancoraggi dei file, le strutture e la BOM di lavoro** (`ProponiAncoraggi`, `StrutturaProdotto`,
  `NodoProposto`, catena del codice, riconciliazione): arrivano con i commit di B2 (P6a) e di B4 (P6b).
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
- **`codici_diagnostica.go`** — responsabilità:
  - i codici che il pacchetto produce (R41 b): `ancoraggio.richiesta_non_valutata`,
    `ancoraggio.righe_stessa_base`, `ancoraggio.quantita_non_intera`, `ancoraggio.alternative_conservate`.

## Entry point

- **`ProponiProdotti`** — chi lo chiama: da A1c `core/valutazione` (`Calcola`, passo 5), per il banco e, da A1d,
  per l'anteprima.
- **I tipi del vocabolario e della fonte** — chi li usa: `core/valutazione` (la richiesta del thread, il target con
  la sua autorità, la fonte di un prodotto con i documenti candidati); dai commit di B2 e B4 gli ancoraggi.
- Oggi, nel codice di prodotto, ancora nessuno: il pacchetto nasce prima del suo chiamante.

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
- **L'errore è solo di contratto** (`*evidenze.ErroreContratto`, con i codici della foglia
  `documento.riferimento_pendente` e `documento.id_ripetuto`): un messaggio ripetuto, un documento che non è
  quello del messaggio, un'interpretazione di un altro documento, una lettura o un attributo su un'unità che non
  c'è, un gesto ripetuto. Un limite dei dati sta nell'esito, con le diagnostiche.
- **Determinismo**: niente orologio, file, DB, rete, goroutine, `uuid.New`, LLM; nessun ordine che dipenda da una
  mappa; messaggi, richiesta e gesti in ordine diverso danno gli stessi byte canonici. `time` serve solo per i tipi
  e per portare in UTC al millisecondo i tempi dei gesti nell'impronta (par.3.4.3).
- `VersioneServizio` e i valori delle costanti del contratto si cambiano solo con un commit che lo dichiara; la
  prova che li fissa si riscrive con «Riscritta per …».

## Dipendenze

- **Importa:** `core/inbox/classificazione/motorea` (le interpretazioni e le letture), `core/registro/regole/grammatica`
  (i ruoli), `core/estrazione/evidenze` (il documento, le diagnostiche), `platform/jsoncanonico` (le impronte),
  `github.com/google/uuid`, la libreria standard. Mai `core/fotorfq`, `core/estrazione`, `core/valutazione`,
  `core/confronto`, il caricatore, il DB (grafo del par.3.2.1).
- **Solo nei test:** `core/estrazione` e `core/fotorfq`, per costruire il documento della mail con
  `estrazione.DaMessaggio` e arrivare a `ProponiProdotti` dall'adattatore. Nessun ciclo: `core/estrazione` non
  importa `ancoraggio`.
- **È importato da:** ancora nessuno nel prodotto; da A1c `core/valutazione`.

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
- G1, G2 e MOTORE-SENZA-LLM in `core/estrazione/evidenze/dipendenze_test.go`; i codici nell'elenco d'oro
  (A1a-CAT, A1c-L1-32) in `core/estrazione/evidenze/codici_diagnostica_test.go`.
- I clienti delle prove sono inventati (ACME). I test stanno nel ramo `-qa`.

## Leggi anche

- `internal/core/README.md`
- `internal/core/inbox/classificazione/motorea/README.md`
- `internal/core/estrazione/README.md`
- `internal/core/estrazione/evidenze/README.md`
- `internal/README.md`
