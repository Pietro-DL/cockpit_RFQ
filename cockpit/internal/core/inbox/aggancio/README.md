# `core/inbox/aggancio` — i candidati di aggancio di un messaggio orfano

## Scopo

Dire **a quale RFQ** (o a quale richiesta a un fornitore) un messaggio orfano potrebbe appartenere, con la
regola che lo dice, il punteggio e la frase che l'operatore legge. Ogni regola produce una riga di candidato;
nessuna scrive un legame. `messaggio.thread_id` e `messaggio.richiesta_fornitore_id` li scrive solo un gesto
dell'operatore (checkpoint 3R, D9; blocco 7B.2 per le richieste).

Qui stanno le **interrogazioni** al database e la scrittura dei candidati. La parte pura — i nomi delle regole,
i punteggi, la precedenza, la soglia di evidenza — sta in `core/inbox/classificazione/aggancio.go` e `atto.go`.

## Non appartiene qui

La lettura del testo e l'estrazione dei codici (`classificazione.Motore.Estrai`), il triage e la scelta
dell'esito (`classificazione.Triage`), la proposta in `proposta_triage` (`ingest`), la decisione di agganciare
(`transport/web`). Questo package riceve fatti già estratti e non interpreta niente.

## File

| File | Responsabilità |
|---|---|
| `aggancio.go` | le regole R0–R5 verso una RFQ: `Ingresso`, `Calcola` (legge, non scrive), `Salva` (sostituisce le righe di `candidato_aggancio`), `CalcolaESalva`; `SalvaCandidatiCodice` (sostituisce le righe di `candidato_codice`, con ruolo e origine) e `perSalvare` (i codici della storia citata scritti per primi); `ChiaviCitate` (In-Reply-To e References normalizzati, al più `maxChiaviCitate` = 40); `tronca` (taglio a caratteri per le colonne `varchar`); `FinestraDefault` (60 giorni), `FinestraBuyer` (14 giorni) |
| `richieste.go` | le regole verso una richiesta a un fornitore (7B.2): `IngressoRichieste`, `CalcolaRichieste` (R0, R1, R3f), `SalvaCandidatiRichiesta` (sostituisce le righe di `candidato_richiesta`), `RichiesteManuali` (RF_oggetto: una nostra mail a un fornitore che cita un identificativo di una RFQ aperta) |

## Entry point

Tutti chiamati da un solo posto, `core/inbox/ingest/controparte.go:interpreta`, che li usa sia al primo
ingresso del messaggio (`ingest.go`) sia nel ritriage mirato (`controparte.go:ritriageUno`).

| Funzione | Quando la chiama `interpreta` |
|---|---|
| `CalcolaESalva(ctx, q, Ingresso)` | sempre, per ogni messaggio orfano da interpretare |
| `CalcolaRichieste` + `SalvaCandidatiRichiesta` | controparte **fornitore**, direzione **entrata** |
| `SalvaCandidatiRichiesta` senza candidati | in tutti gli altri casi: cancella i candidati verso una richiesta di una lettura precedente (un mittente ricensito come cliente, o diventato ambiguo) |
| `RichiesteManuali(ctx, q, codici)` | controparte **fornitore**, direzione **uscita**, con tutti i codici estratti (non solo quelli di famiglia); il risultato va in `IngressoTriage.RichiesteManuali`, non in una tabella |
| `SalvaCandidatiCodice(ctx, q, id, Estrazione)` | sempre, dopo il triage, con `EsitoTriage.Estrazione` |

`Calcola` e `Salva` sono esportate ma fuori dal package si usa solo la coppia `CalcolaESalva`.

## Dati

Nessuna transazione propria: tutte le funzioni lavorano sul `*db.Queries` del chiamante, cioè dentro la
transazione del lotto (con il savepoint per messaggio) o dentro quella del ritriage di un messaggio. Nessun
lucchetto. Le query stanno in `platform/db/queries/aggancio.sql` e `richieste.sql`.

| Regola | Punti | Sorgente (query) | Condizioni |
|---|---|---|---|
| `R0_reply` | 98 | `ThreadPerChiaviCitate`: `messaggio.chiave_esterna` fra le chiavi citate, canale `outlook` (la condizione che fa usare l'indice unico `(canale, chiave_esterna)`), con `thread_id` già scritto | nessun cliente richiesto, nessuna finestra |
| `R1_conversazione` | 95 | `ThreadDellaConversazioneConStato`: `conversazione.thread_id` (lo scrive solo un operatore) | nessuna finestra |
| `R4_riferimento` | 90 | `ThreadPerRiferimentoCliente`: `thread_offerta.riferimento_cliente` uguale senza maiuscole | stesso cliente, RFQ non unita, nessuna finestra, al più 5 |
| `R3_codice` | 80 | `ThreadPerCodiciCliente`: `identificativo_thread.codice` fra i codici passati | stesso cliente, non unita, `data_inizio` dentro la finestra, al più 10 |
| `R2_oggetto` | 55 | `ThreadPerOggettoCliente`: oggetto pulito uguale senza maiuscole, almeno 8 byte | stesso cliente, non unita, dentro la finestra, al più 5 |
| `R5_buyer` | 35 | `ThreadRecentiBuyer`: stesso `buyer_id` | RFQ `APERTA`, non unita, ultimi `FinestraBuyer` giorni, al più 5 |

La finestra di R2/R3 è `Ingresso.FinestraGG` (da `cliente.regole.finestra_aggancio_gg` via
`Motore.Finestra`, che restituisce 0 per un valore fuori da 1..3650), oppure `FinestraDefault` se è ≤ 0. R2, R3,
R4 richiedono `ClienteID`; R5 richiede `BuyerID`. I punteggi vengono da `classificazione.PuntiRegola`;
`Candidato.Chiuso` è lo stato della RFQ al momento del calcolo e finisce in `candidato_aggancio.thread_stato`.

| Regola verso una richiesta | Punti | Sorgente | Condizioni |
|---|---|---|---|
| `R0_reply` | 95 | `RichiestePerChiaviCitate`: la chiave della nostra mail di richiesta | stesso fornitore |
| `R1_conversazione` | 80 | `RichiestePerConversazione`: la conversazione della nostra mail | stesso fornitore |
| `R3f_codice` | 60 | `RichiestePerCodiciFornitore`: codice fra gli identificativi della RFQ o fra `richiesta_fornitore.codici` | stesso fornitore, richiesta `inviata` o `offerta_ricevuta`, al più 20 |
| `RF_oggetto` | 70 | `ThreadApertePerCodici`: identificativo di una RFQ `APERTA` non unita, di qualunque cliente | al più 20; non scritto in tabella |

Scritture (sempre «cancella tutto il messaggio, poi inserisci»):

| Tabella | Chi | Chiave |
|---|---|---|
| `candidato_aggancio` | `Salva` | `(messaggio_id, thread_id, regola)` |
| `candidato_richiesta` | `SalvaCandidatiRichiesta` (evidenza tagliata a 500 caratteri) | `(messaggio_id, richiesta_id, regola)` |
| `candidato_codice` | `SalvaCandidatiCodice`: prima il riferimento (`riferimento_rfq`, origine `riferimento`), poi i codici trovati nella storia citata, poi gli altri nell'ordine dell'estrazione (origine `famiglia` o `generico`); codice ≤ 60, rev ≤ 10, famiglia ≤ 120 caratteri | `(messaggio_id, codice)`, e sul conflitto vince la riga scritta per ultima |

I tagli sono a **caratteri**, non a byte (`tronca`): le colonne sono `varchar(n)`, che PostgreSQL conta in
caratteri, e un taglio dentro una lettera accentata lascerebbe UTF-8 non valido.

## Flussi principali

1. **Candidati verso una RFQ** — `interpreta` estrae i codici con il motore del cliente, tiene solo quelli di
   famiglia (`classificazione.DiFamiglia`) e chiama `CalcolaESalva` con oggetto pulito
   (`classificazione.OggettoPulito`), riferimento, buyer e finestra. `Calcola` interroga R0, R1, poi — con un
   cliente — R4, R3, R2, poi R5; una mappa `(thread, regola)` impedisce che la stessa regola parli due volte dello
   stesso thread; `classificazione.OrdinaCandidati` mette in cima i più forti. `Salva` cancella e riscrive.
2. **Le chiavi di R0** — `ChiaviCitate` prova ogni identificativo con e senza `<>`, prima quelli
   dell'In-Reply-To e poi le References. Il tetto di 40 forme si paga sulle References, tenendo le più recenti
   (in fondo all'header); l'In-Reply-To non viene mai tagliato, salvo che da solo superi il tetto.
3. **Candidati verso una richiesta** (posta di un fornitore in entrata) — `CalcolaRichieste` con gli stessi codici
   di famiglia (il motore del fornitore è l'unione delle famiglie dei clienti che gli hanno richieste aperte),
   ordinamento per punteggio, `SalvaCandidatiRichiesta` cancella e riscrive. Per ogni altro messaggio
   interpretato `SalvaCandidatiRichiesta` cancella soltanto.
4. **Richiesta mandata a mano** (nostra mail a un fornitore) — `RichiesteManuali` restituisce un candidato per
   RFQ aperta; il triage ne fa la proposta «richiesta a X per la RFQ Y», e la conferma crea la richiesta.
5. **Candidati di codice** — dopo il triage, `SalvaCandidatiCodice` scrive ogni numero trovato con il suo ruolo.
   L'estrazione tiene lo stesso codice due volte se le revisioni sono diverse; `perSalvare` scrive per primi i
   codici della storia citata, così la revisione del testo scritto adesso vince su quella citata sotto.

## Invarianti

- Nessuna funzione scrive `messaggio.thread_id`, `conversazione.thread_id`, `messaggio.richiesta_fornitore_id`
  o `proposta_triage`: le sole scritture sono le tre tabelle di candidati.
- I candidati sono una fotografia: ogni `Salva*` cancella le righe del messaggio prima di inserire, anche quando
  non ha niente da inserire.
- Tutti i candidati si scrivono, nessuno viene scelto (T16); quelli verso una RFQ chiusa restano, con
  `thread_stato = CHIUSA` (T2).
- R2, R3, R4 valgono solo per lo stesso cliente (T4); R2 e R3 solo dentro la finestra (T3).
- R3 e R3f usano i codici che il chiamante passa: `ingest` passa solo quelli di famiglia.
- L'In-Reply-To di un messaggio arriva sempre fra le chiavi di R0, anche in una catena con decine di References.
- Ciò che si scrive in una colonna `varchar` è UTF-8 valido e sta nella colonna: un taglio non fa rifiutare
  la riga, e con lei il messaggio.
- In `candidato_codice` la revisione della storia citata non copre quella del testo nuovo.

## Dipendenze

Importa `core/inbox/classificazione` (tipi `Candidato`, `CandidatoRichiesta`, `CodiceTrovato`, punteggi,
ordinamento, nomi delle regole, `Estrazione`, `DoveStoria`) e `platform/db`. È importato solo da
`core/inbox/ingest`. Coerente con la tabella di `internal/README.md` (`core/*` → altri `core/*`, `platform`).

## Test

| File | Livello | Che cosa prova |
|---|---|---|
| `aggancio_test.go` | L1 | `ChiaviCitate`: l'In-Reply-To (nelle due forme) sopravvive al tetto con 60 References, delle References restano le ultime, con poche chiavi l'ordine resta; `tronca` a caratteri con le lettere accentate; `perSalvare`: la revisione del corpo vince su quella della storia |
| `core/inbox/ingest/candidati_db_test.go` | L4 (`integrazione`) | R1 propone e non decide (T1), R0 da In-Reply-To (T22), finestra e cliente per R2/R3 (T3/T4), tutti i candidati e ordinati (T16, con R3, R4, R2), ruoli dei candidati di codice |
| `core/inbox/ingest/robustezza_db_test.go` | L4 (`integrazione`) | una famiglia accentata oltre 120 byte non manda il messaggio in scarto; il ritriage che toglie i candidati verso una richiesta quando il mittente non è più un fornitore |
| `core/inbox/ingest/invarianti_7c_db_test.go` | L4 (`integrazione`) | I4: R0 e R1 verso una richiesta sono candidati al 95 e all'80, non legami |
| `transport/web/richieste_db_test.go` | L4 (`integrazione`) | IB3–IB5: RF_oggetto, R0 verso la richiesta, due candidati R3f |
| `core/inbox/classificazione/precedenza_test.go`, `atto_test.go`, `evidenza_test.go` | L1 | punteggi, soglia di evidenza, R5 da sola, RFQ chiusa, RF_oggetto nel triage, l'aperta proposta quando la più forte è chiusa |

Non coperti: R5 dal database, `thread_stato` scritto come `CHIUSA`.

## Stato dell'implementazione

Completo per il checkpoint 3R e per il 7B.2. Limiti noti:

- R0 e R1 non escludono le RFQ con `unito_in` (R2–R5 sì); oggi nessuna query scrive `unito_in`.
- `candidato_codice` ha una riga per codice: fuori dalla storia citata, due revisioni dello stesso codice nello
  stesso messaggio (per esempio nel corpo e nel nome di un allegato) lasciano solo l'ultima nell'ordine
  dell'estrazione (oggetto, corpo, allegati).
- R0 verso una richiesta (`RichiestePerChiaviCitate`) non ha la condizione sul canale.
- In `SalvaCandidatiRichiesta` un candidato con un `RichiestaID` che non si legge come UUID si salta senza
  avviso (in `Salva` lo stesso caso è un errore).

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una regola nuova verso una RFQ | `aggancio.go:Calcola`, la query in `platform/db/queries/aggancio.sql`, nome e punti in `classificazione/aggancio.go`, l'enum `regola_aggancio` in migrazione |
| cambiare la finestra di default | `aggancio.go:FinestraDefault` (per cliente: `finestra_aggancio_gg` in `core/registro/regole`) |
| capire perché un messaggio non ha il candidato R0 | `aggancio.go:ChiaviCitate`, `ThreadPerChiaviCitate` |
| capire perché un'offerta propone quella richiesta | `richieste.go:CalcolaRichieste` |
| cambiare i ruoli dei candidati di codice | `aggancio.go:SalvaCandidatiCodice` |
| capire quale revisione è finita in `candidato_codice` | `aggancio.go:perSalvare` |
| capire che cosa fa il triage dei candidati | `core/inbox/classificazione/codici.go:Triage`, `core/inbox/classificazione/aggancio.go:EvidenzaDiRFQEsistente`, `MiglioreCandidatoAperto` |

## Leggi anche

`internal/core/README.md`, `internal/README.md` (flusso Sync), `internal/core/inbox/classificazione` (la parte
pura delle regole), `platform/db/queries/aggancio.sql` e `richieste.sql`.
