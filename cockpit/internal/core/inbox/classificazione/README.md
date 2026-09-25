# `internal/core/inbox/classificazione` — l'interpretazione pura di un messaggio

## Scopo

Che cosa il Cockpit capisce di una mail e dei suoi allegati **senza decidere niente**: chi c'è dall'altra
parte (la controparte), che cosa è stato scritto adesso e che cosa è storia citata, quali numeri sono codici
del cliente e quale è il riferimento della richiesta, che esito proporre (`nuova_rfq`, `aggancia`, `ignora`)
con l'atto e il legame del 7C.0, che tipo di documento sembra un allegato dal solo nome. Qui sta anche il
**motore** che compila le regole di un cliente (`cliente.regole`) e le applica a un testo, compresi i suffissi
decorativi del Fascicolo v3.

Il package è puro: nessun database, nessun HTTP, nessun disco. Prende struct e restituisce struct. L'unico
accesso all'anagrafica passa da un'interfaccia (`Rubrica`) che implementa chi chiama.

## Non appartiene qui

- Lo **schema** di `cliente.regole` e la diagnosi ✓/✗: `core/registro/regole` (qui si legge `Verifica` e basta).
- Le interrogazioni dei candidati di aggancio R0–R5 e dei candidati verso una richiesta, e la loro scrittura:
  `core/inbox/aggancio` (qui stanno solo nomi, punteggi, precedenza).
- La scrittura della proposta di triage, della controparte, dei candidati di codice, il ritriage, la cache dei
  motori per cliente e per fornitore (`Motori`), l'implementazione vera della `Rubrica` (`rubricaDB`):
  `core/inbox/ingest`.
- La presentazione del corpo di una mail (blocchi, tabelle, rumore): `core/inbox/lettura`, che usa
  `TagliaCatena` per separare la storia citata.
- Le decisioni dell'operatore (Nuova RFQ, aggancia, ignora, censisci): `transport/web`.
- Il nome delle cartelle e dei file sul NAS: `core/rfq/documenti` (usa `OggettoPulito`).
- L'unione dei codici della RFQ per il Fascicolo e la classificazione dei nodi degli STEP: `core/rfq/fascicolo`
  (usa `Motore`, `CodiceAmmissibile`, `DoveStoria`).

## File

| File | Responsabilità |
|---|---|
| `codici.go` | Commento `// Package`. L'estrattore generico (`EstraiCodici`, `sembraCodice`, `CodiceRev` con la forma dei nostri nomi sul NAS `reRevNas` prima della convenzione generica `reRev`), i limiti di un codice (`MaxCodice` = 40, `MaxRev` = 10, `CodiceAmmissibile`, `RevAmmissibile`, `spazioOControllo`, 7C.1 P0), il portale (`RilevaPortale`, `RiferimentoPortale`), la scadenza (`RilevaScadenza`, `spezzaFrasi`), il **triage** (`IngressoTriage`, `IngressoTriage.Testi`, `EsitoTriage`, `Triage`, `reParoleRFQ`, i rami `triageUscita`, `triageFornitore`, `triageSconosciuto`, `attoELegameCliente`) |
| `regole.go` | Il **motore**: `Compila` (solo le regole ✓), `Motore` (`CodiciDa`, `Codici`, `Estrai`, `Riferimento`, `FrasiPortale`, `HaFamiglie`, `Finestra` con `maxFinestraGG` = 3650), i suffissi decorativi (`Canonico`, `CanonicoNome`, `HaSuffissi`, `SuffissiDecorativi`, `senzaSuffissi`, `togliSuffisso`), `CodiceTrovato` con ruolo e punteggio (`RuoloRiferimento`/`RuoloProdotto`/`RuoloParte`/`RuoloIgnoto`, `PuntiFamiglia` 80, `PuntiGenerico` 30, `PuntiRiferimen` 90), `Testo`/`Testi`, `Estrazione` (`Proponibili`, `Altri`), `DiFamiglia`, `SoloCodici`, e `Riconosci` (triage + portale + scadenza in una chiamata) |
| `catena.go` | Il **taglio della catena di risposta** (blocco 6 del 3R): `TagliaCatena` → (utile, storia), `CorpoUtilePerInterpretazione`, `DoveStoria`; i quattro riconoscitori (`separatoreEsplicito`, `aperturaDiCitazione` con le forme di `apertureCitazione`, `intestazioneCitata`, `testoMarcato`) e `arretra` |
| `atto.go` | Il vocabolario del 7C.0: le costanti `Atto*` (righe della tabella `atto_business`), `Atti`, `AttoValido`, le costanti `Legame*` (enum `legame_operativo`); le regole verso una richiesta a un fornitore (`RichiestaR0Reply`, `RichiestaR1Conversazione`, `RichiestaR3fCodice`, `RichiestaRFOggetto`, `PuntiRichiesta`, `CandidatoRichiesta`, `MiglioreCandidatoRichiesta`); `NonBusiness` e `AttoFornitore` (offerta, domanda, conferma di ricezione, non di lavoro, generica) |
| `controparte.go` | Il **resolver** della controparte (7A/D33, `altro` dal 7C.0): `RisolviControparte`, `IngressoControparte`, `Controparte`, `Voce`, l'interfaccia `Rubrica` e la sua versione in memoria `RubricaFissa`; le costanti `Controparte*` e `Via*`; `DominioPubblico` (l'elenco dei domini di posta pubblici, l'unico del programma) |
| `proposta.go` | La prima interpretazione di un allegato dal solo nome: `PropostaDaNome` → `Proposta` (tipo, fonte, confidenza, codice, rev, `PreSpunta`, `CodiciNelNome`), `TipoDaEstensione`, `EstImmagine`, `SogliaStagingAutomatico` (20 MB, D30) |
| `oggetto.go` | `OggettoPulito`: i prefissi `RE:`/`R:`/`FW:`/`FWD:`/`I:`/`TR:`/`AW:`/`WG:` ripetuti tolti dall'oggetto |
| `aggancio.go` | La parte pura delle regole di aggancio R0–R5: `R0Reply` … `R5Buyer`, `PuntiRegola`, `ordineRegola`, `SogliaEvidenza` (50), `Candidato`, `OrdinaCandidati`, `EvidenzaDiRFQEsistente`, `CandidatoChiuso`, `MiglioreCandidato`, `MiglioreCandidatoAperto` (il più forte fra quelli sopra soglia verso una RFQ non chiusa) |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Triage`, `IngressoTriage`, `EsitoTriage` | `core/inbox/ingest/controparte.go:interpreta` (primo ingresso e ritriage) |
| `IngressoTriage.Testi`, `Motore.Estrai`, `DiFamiglia`, `SoloCodici` | `ingest/controparte.go:interpreta`, per calcolare i candidati di aggancio **prima** del triage sullo stesso testo |
| `Motore.Finestra` | `ingest/controparte.go:interpreta` (la finestra di R2/R3 passata ad `aggancio`) |
| `RilevaScadenza` | `ingest/controparte.go:interpreta` |
| `RilevaPortale`, `PropostaDaNome`, `SogliaStagingAutomatico` | `ingest/ingest.go` (`uno`, `sogliaStaging`) |
| `Compila` | `ingest/ingest.go` (`Motori.Per`, `Motori.PerFornitore`), `rfq/fascicolo/classifica.go:MotoreDellaRfq`, `transport/web/triage.go:datiTriage` (solo `HaFamiglie`), `transport/web/anagrafica.go:bancoProva`, `transport/workerapi/workerapi.go:motoreDelFile` |
| `Motore.Canonico`, `Motore.CanonicoNome`, `Motore.HaSuffissi` | `ingest/ingest.go:uno`, `rfq/fascicolo` (`candidati.go`, `decisioni.go`, `piano.go`, `proposte.go`, `voluta.go`), `transport/workerapi/workerapi.go` (`propostaDaAnalisi`, `scriviProposta`) |
| `Motore.Codici`, `Motore.Estrai`, `Testo` | `rfq/fascicolo/applica.go:propostaDelDocumento`, `rfq/fascicolo/classifica.go:ClassificaNodi` |
| `RisolviControparte`, `IngressoControparte`, `Controparte`, `Voce`, `Controparte*`, `ViaContatto` | `ingest/controparte.go` (`risolviControparte`, `rubricaDB`, `parametriControparte`, `clienteDallaControparte`) |
| `Riconosci`, `Riconoscimento` | `transport/web/anagrafica.go:bancoProva` (il banco di prova dell'Anagrafica) |
| `OggettoPulito` | `rfq/documenti/path.go:CartellaThread`, `ingest/controparte.go:interpreta` (R2), `transport/web/triage.go` |
| `TagliaCatena` | `core/inbox/lettura/lettura.go:elabora` (la storia citata mostrata chiusa nel corpo della mail) |
| `R0Reply` … `R5Buyer`, `PuntiRegola`, `PuntiRiferimen`, `Candidato`, `OrdinaCandidati`, `Estrazione`, `CodiceTrovato`, `RuoloRiferimento` | `core/inbox/aggancio/aggancio.go` (`Calcola`, `SalvaCandidatiCodice`, `perSalvare`) |
| `CandidatoRichiesta`, `PuntiRichiesta`, `Richiesta*` | `core/inbox/aggancio/richieste.go` |
| `CodiceAmmissibile`, `RevAmmissibile`, `MaxCodice`, `MaxRev` | `rfq/fascicolo` (`candidati.go`, `classifica.go`, `decisioni.go`, `modifiche.go`, `preparazione.go`, `voluta.go`), `transport/web` (`allegati.go:confermaProposta`, `fascicolo.go:correggiCodiceComponente`, `fascicolo_conferma.go`), `transport/workerapi/workerapi.go:codiceRevSicuri` |
| `PropostaDaNome`, `Proposta` | anche `transport/workerapi/archivi.go:EstraiArchivio` (le voci di uno zip), `workerapi.go`, `rfq/fascicolo/piano.go` |
| `DoveStoria` | `rfq/fascicolo/candidati.go` (`forte`, e il motivo «nella storia citata»), `core/inbox/aggancio/aggancio.go:perSalvare`, `transport/web/triage.go:triageDati.Spuntato` (il pannello non pre-spunta i codici della storia) |
| `TipoDaEstensione` | `transport/web/caricamento.go:tecnicoCaricabile` |
| `EstImmagine` | `transport/workerapi/workerapi.go:dopoStaging` |
| `DominioPubblico` | `transport/web/censisci.go:datiCensisci`, `transport/web/triage.go:clienteDaForm` |
| `AttoRichiestaOfferta` | `transport/web/routes_inbox.go:caricaMessaggio` |
| `Ruolo*` | `ai/agente/servizio.go` (un controllo all'inizializzazione del package: un ruolo del dominio che l'agente non accetta fa `panic`) |

Senza chiamanti fuori dal package: `CorpoUtilePerInterpretazione` (la usano `Testi`, `Triage`, `NonBusiness`,
`AttoFornitore`), `NonBusiness`, `AttoFornitore`, `MiglioreCandidatoAperto`, `EstraiCodici`, `RubricaFissa` (solo
le prove), `Motore.Riferimento` (solo le prove); `Ordina`, `AttoValido` e `Atti` non li chiama nessuno.

## Dati

Il package non legge né scrive tabelle o file. Le stringhe però **devono coincidere** con il database, che le
rifiuta se non tornano:

| Costanti | In database |
|---|---|
| `Controparte*` | enum `tipo_controparte` (0014, `altro` dalla 0016) |
| `Via*` | enum `via_controparte` |
| `Atto*` / `Atti` | righe di `atto_business` (0016), chiave esterna di `proposta_triage.atto` |
| `Legame*` | enum `legame_operativo` (0016) |
| `Ruolo*` | enum `ruolo_codice` (0008) |
| `Proposta.Tipo`, `Proposta.Fonte` | enum `tipo_documento`, `fonte_proposta` |
| `EsitoTriage.Esito` | enum `esito_triage` |
| `MaxCodice`, `MaxRev` | mai più larghe delle colonne `codice varchar(60)`, `rev varchar(10)`: lo controlla `platform/migrazioni/larghezze_db_test.go` (L4). Le applica chi scrive (`CodiceAmmissibile`, `RevAmmissibile`), non il motore |

La `Rubrica` vera (`ingest/controparte.go:rubricaDB`) legge contatti dei fornitori, buyer, domini di fornitori e
clienti, recapiti di `soggetto_altro`: un errore del database arriva a chi chiama, non diventa `sconosciuto`.

## Flussi principali

**Triage di un messaggio orfano** (lo chiama `ingest.interpreta`, sul messaggio senza `thread_id`):

1. `IngressoTriage.Testi` divide il messaggio in pezzi etichettati: `oggetto`, `corpo` (quello scritto adesso,
   da `TagliaCatena`), `storia citata` se c'è, e un `allegato <nome>` per ogni nome di file (senza estensione, mai
   tagliato).
2. `Motore.Estrai` cerca prima il riferimento RFQ del cliente, poi i codici: le famiglie ✓ del cliente sul testo
   senza URL e senza suffissi decorativi, poi l'estrattore generico (URL tolti, `CodiceRev`, `Canonico`). Un codice
   che è il riferimento, o un suo pezzo, non entra fra i codici.
3. `ingest` calcola con quel testo i candidati R0–R5 (e, per un fornitore, i candidati verso una richiesta o le
   richieste mandate a mano) e li mette in `IngressoTriage`.
4. `Triage` sceglie il ramo:
   - **uscita non interna**: `triageUscita`. Verso un fornitore l'atto è `richiesta_offerta`; con un candidato
     `RF_oggetto` l'esito è `aggancia` e il legame `nuovo` (nasce una richiesta, non una RFQ).
   - **fornitore** in entrata: `triageFornitore`. Solo codici di famiglia (IB8), atto da `AttoFornitore`; `aggancia`
     verso il candidato richiesta migliore, altrimenti — con evidenza — verso la RFQ aperta più forte
     (`MiglioreCandidatoAperto`), altrimenti `ignora` a 0. Mai `nuova_rfq`.
   - **ambiguo**: `ignora`, confidenza 0, atto e legame `incerto`.
   - **sconosciuto**: `triageSconosciuto`. Codici estratti, atto `incerto` (o `non_business`), `aggancia` solo con
     evidenza e verso la RFQ aperta più forte; mai `nuova_rfq`.
   - **cliente**: se **non** c'è evidenza di una RFQ aperta (`EvidenzaDiRFQEsistente`) e `NonBusiness` riconosce
     posta non di lavoro → `ignora`, atto `non_business`; con l'evidenza le parole non contano e decide la
     precedenza. Poi, come per **interno**, **altro** e controparte non dichiarata: i punti del contenuto (parole
     da RFQ nel solo corpo utile 35, allegati tecnici 25, PDF/TIF 10, buyer 25 o dominio cliente 15, codici
     proponibili 10, codici di famiglia fra i proponibili 10, riferimento fuori dalla storia 15), poi la
     precedenza: se c'è un candidato ≥ `SogliaEvidenza` verso una RFQ aperta → `aggancia` verso il più forte di
     questi (`MiglioreCandidatoAperto`) con il suo punteggio, e un motivo in più se il più forte in assoluto è una
     RFQ chiusa; altrimenti ≥ 50 → `nuova_rfq`, sotto → `ignora`. Atto e legame da `attoELegameCliente`.
5. `ingest` scrive `proposta_triage` (esito, atto, legame, `identificativi` = `EsitoTriage.Codici`) e i candidati
   di codice da `EsitoTriage.Estrazione`.

**Il motore di un cliente.** `Compila(ragione sociale, regole)` chiama `regole.Verifica` e tiene solo le famiglie,
il riferimento, le frasi del portale e i suffissi con la riga ✓; i suffissi in maiuscolo, il più lungo prima. Si
costruisce una volta per cliente e si passa in giro; un `*Motore` nil è il cliente sconosciuto e ogni metodo lo
accetta. `Finestra` restituisce `finestra_aggancio_gg` solo se sta in 1..3650 (lo stesso limite di
`regole.Verifica`); fuori, o non dichiarata, 0 = la finestra predefinita di `aggancio`.

**La controparte.** `RisolviControparte`: se il mittente è nostro si risolve il primo destinatario esterno
(nessuno → `interno`, via `casella`); poi l'indirizzo esatto (contatto di fornitore, buyer, recapito `altro`), poi
il dominio (fornitore, cliente, `altro`), altrimenti `sconosciuto`. Più categorie allo stesso livello, o lo stesso
contatto in due fornitori, danno `ambiguo`. Ogni risposta porta il `Motivo`.

**Un allegato dal nome.** `PropostaDaNome`: tipo dall'estensione (`TipoDaEstensione`; PDF/TIF = `da_determinare`),
codice e revisione solo se il nome senza estensione **è** un codice (`CodiceRev` + `sembraCodice`), altrimenti i
codici trovati dentro vanno in `CodiciNelNome`; `SO …pdf` in uscita è la nostra offerta (il tipo delle offerte nostre, non `offerta_fornitore`); immagini piccole o nomi
da telefono sono `rumore`; `PreSpunta` da `daScaricare` (un `da_determinare` fino a `SogliaStagingAutomatico`). Chi
ha il motore del cliente passa poi codice e nomi per `Canonico` / `CanonicoNome`. `CodiceRev` rilegge prima i
nostri nomi sul NAS (`<CODICE>_REV_<REV>`, con `_n` in coda per il secondo file con lo stesso nome, e `_REV_ND`
= revisione vuota), poi la convenzione generica (`_n`, `-n`, `_REVn`, `_Rn`, `_B`, `-B`).

**Il banco di prova dell'Anagrafica.** `Riconosci` = `Triage` + `RilevaPortale` con le frasi del cliente +
`RilevaScadenza`, sul testo incollato e con il motore compilato come lo compila l'ingest.

## Invarianti

- **Nessun DB, nessun HTTP**: gli import sono la libreria standard, `google/uuid` e `core/registro/regole`.
- **Il corpo originale non si tocca**: `TagliaCatena` restituisce due stringhe; se il taglio non lascia niente
  (un inoltro senza commento) il corpo resta intero. Una riga `Da:` da sola non taglia: servono due intestazioni
  di ruolo diverso e, fra i valori, un indirizzo, una data o `Oggetto:`. Un'apertura di citazione vale solo con
  prefisso **e** chiusura: le forme tedesca con il nome dopo «schrieb» e di Thunderbird in italiano («Il
  12/09/26 10:00, Nome ha scritto:») chiedono in più una data e i due punti in fondo alla riga, così una frase
  che comincia allo stesso modo resta testo.
- **Un fornitore in entrata non produce mai `nuova_rfq`**, un ambiguo non produce nessuna proposta, uno
  sconosciuto non produce `nuova_rfq`: sono rami di `Triage` che non contano i punti.
- **L'evidenza viene prima del contenuto e delle parole**: dove si contano i punti, un candidato ≥
  `SogliaEvidenza` verso una RFQ aperta dà `aggancia` e `nuova_rfq` non si valuta; nel ramo cliente la stessa
  evidenza viene prima di `NonBusiness`. Una RFQ chiusa si vede (motivo con «CHIUSA») e non si propone mai
  come aggancio: con un'aperta sopra soglia si propone l'aperta, senza si mostra la chiusa in `Candidato` e
  l'esito non è `aggancia`. R5 (35) da solo non basta mai.
- **Il riferimento RFQ non è un codice prodotto** (`Estrai`), e non è mai proponibile.
- **Proponibili** (`Estrazione.Proponibili`): con famiglie ✓ solo i codici di famiglia; mai quelli della storia
  citata. Le parole da RFQ, un riferimento e i codici di famiglia trovati solo nella storia non fanno punti.
- **Le regole ✗ riga per riga non entrano nel motore** (`famiglia n`, `riferimento RFQ`, `frase portale n`,
  `suffisso decorativo n`); una `finestra_aggancio_gg` fuori da 1..3650 vale come non dichiarata.
- **Un PDF non è un disegno prima dell'analisi**: `da_determinare`, e nel triage vale 10 («tipo da
  determinare»), non 25.
- **Un nome che contiene un codice non è un codice** (7C.1 P0): `Proposta.Codice` sta dentro `MaxCodice` e non ha
  spazi.
- **Un codice non contiene spazi di nessun tipo**: `CodiceAmmissibile`, `RevAmmissibile` e `sembraCodice`
  rifiutano gli spazi ASCII e Unicode (spazio unificatore, spazio stretto), i caratteri di controllo e quelli di
  formato (spazio a larghezza zero).
- **«richiesta d'offerta» con l'apostrofo tipografico** (U+2019) vale come con quello dritto.
- **I suffissi decorativi valgono solo per chi li dichiara**: si tolgono solo in coda, il più lungo prima, e
  `Canonico` toglie solo se ciò che resta ha ancora la forma di un codice.
- **Controparte**: l'indirizzo esatto vince sul dominio; a parità di livello nessuna precedenza silenziosa
  (`ambiguo`); un errore della `Rubrica` interrompe la risoluzione.

## Dipendenze

**Importa:** `core/registro/regole` (lo schema che il motore compila), `github.com/google/uuid`, libreria standard.
Conforme alla tabella di `internal/README.md`.

**È importato da:** `core/inbox/ingest`, `core/inbox/aggancio`, `core/inbox/lettura` (`TagliaCatena`),
`core/rfq/documenti` (`OggettoPulito`), `core/rfq/fascicolo`, `ai/agente` (solo le costanti `Ruolo*`),
`transport/web`, `transport/workerapi`. Fra i test anche `platform/migrazioni/larghezze_db_test.go` (L4, pacchetto
esterno `migrazioni_test`): è l'unico punto in cui qualcosa sotto `platform/` nomina `core`, e solo in una prova.

## Test

Tutte le prove sono **L1** (nessun tag, nessun database): `go test ./internal/core/inbox/classificazione/`.
Copertura delle istruzioni 93,9% (misurata il 25/09/2026).

| File | Che cosa prova |
|---|---|
| `classificazione_test.go` | `EstraiCodici`, `CodiceRev`, `RilevaPortale`, `RilevaScadenza`, il triage di base (RFQ, newsletter, falsi positivi `rdo`/screenshot, uscita, mail interna) |
| `codici_test.go` | `CodiceRev` sui nostri nomi del NAS (`_REV_B`, `_REV_B_2`, `_REV_ND`); codici e revisioni con uno spazio Unicode non ammissibili; l'apostrofo tipografico; `spezzaFrasi` con l'espressione compilata una volta; `Motore.Finestra` fuori da 1..3650 |
| `evidenza_test.go` | l'evidenza di una RFQ aperta vince sulle parole di `NonBusiness` (e R5 da solo no); con la più forte chiusa si propone l'aperta, in tutti i rami; il bonus della famiglia non conta la storia citata |
| `precedenza_test.go` | Checkpoint 3R: T22 (una risposta non diventa richiesta nuova, per ciascuna di R0–R4), R5 debole, T2 (RFQ chiusa), ordinamento dei candidati, AN6 (riferimento ≠ codice), AN7 (con famiglie il generico non propone; `Altri`), punteggi per provenienza, PDF ≠ disegno, ruolo `parte`, motivi serializzabili, `dati_richiesti` / `risposta_entro_gg` |
| `catena_test.go` | Il taglio: mail nuova, risposta italiana e inglese, quattro risposte concatenate, testi legittimi con `Da:`/`On`/`Il giorno`, elenco a due righe, codici solo nella storia, codice nuovo contro codici vecchi, parole e riferimento della storia senza punti, inoltro senza commento, corpo vuoto, `\r\n`, spazio unificatore; le aperture tedesca («Am … schrieb Name:») e di Thunderbird in italiano, anche spezzate su due righe, e la prosa che comincia allo stesso modo |
| `regole_test.go` | AN3 (regola ✗ vista e non usata), AN4 (famiglie con provenienza e revisione), motore nil, riferimento con il nome della regola, AN5 metà L1 (`Riconosci`), frasi del cliente in aggiunta alle generiche |
| `suffissi_test.go` | Fascicolo v3: `Canonico` / `CanonicoNome` (maiuscole, il più lungo prima, revisione in coda, ciò che resta deve essere un codice), senza suffissi niente cambia, suffisso rotto non usato, `CodiciDa` con famiglie e generico, `senzaSuffissi` |
| `proposta_test.go` | `PropostaDaNome` su 18 nomi (tipo, codice, rev, pre-spunta, limiti), nome lungo → `CodiciNelNome`, `CodiceAmmissibile` / `RevAmmissibile` |
| `controparte_test.go` | CP6: la tabella dei 20 casi del resolver (contatto, dominio, interno, ambiguo, `altro`, normalizzazione), senza `Nostro`, errore della rubrica |
| `controparte_triage_test.go` | CP1 puro: un fornitore in entrata non apre mai una RFQ, la sua offerta si aggancia, ambiguo senza proposta, cliente come prima |
| `atto_test.go` | 7B/7C.0: IB7 (newsletter da sconosciuto), sconosciuto che sembra RFQ, controparte non dichiarata, atti del ramo fornitore, IB8 (materiali e norme non sono codici), aggancio alla richiesta, RF_oggetto, newsletter di un cliente, risposta del cliente (legame sì, atto `incerto`), inoltro interno |

Le prove con il database che passano di qui stanno negli altri package: `ingest/candidati_db_test.go`,
`ingest/invarianti_7c_db_test.go`, `transport/web/candidati_db_test.go`, `transport/workerapi/misura_db_test.go`,
`platform/migrazioni/larghezze_db_test.go` (L4).

Non provati direttamente: `OggettoPulito` (solo attraverso `rfq/documenti/path_test.go`), `DominioPubblico`,
`TipoDaEstensione` fuori da `PropostaDaNome`, il triage con controparte `altro`, la corrispondenza fra le
costanti e i valori del database.

## Stato dell'implementazione

- **Completo**: triage deterministico con la precedenza del 3R e i rami per controparte (7A, 7B, 7C.0); taglio
  della catena (blocco 6) con le aperture italiane, inglesi, tedesche, francesi e di Thunderbird; resolver della
  controparte con `altro` (7C.0); motore con famiglie, riferimento, frasi del portale e suffissi decorativi
  (Fascicolo v3); proposta dal nome con i limiti del 7C.1 P0; parte pura di R0–R5 e delle regole verso una
  richiesta.
- **Parziale**:
  - il deterministico produce solo una parte del vocabolario: `richiesta_offerta`, `offerta`,
    `domanda_chiarimento`, `conferma_ricezione`, `comunicazione_generica`, `inoltro`, `non_business`, `incerto`; i
    legami `aggiornamento` e `inoltro` non li produce mai. Il resto è per l'agente e per i blocchi successivi;
  - la controparte `altro` non ha un ramo suo in `Triage`: passa per il conteggio dei punti come un cliente senza
    anagrafica, e atto e legame restano vuoti;
  - `PropostaDaNome` non conosce il motore del cliente: un codice di famiglia che il generico non riconosce (per
    esempio con due o più punti fra sole cifre) non arriva in `Proposta.Codice`.
- **Limiti noti**:
  - senza evidenza di una RFQ aperta, una parola di `NonBusiness` nel corpo utile — firma compresa — basta a
    proporre `ignora` per la mail di un cliente;
  - due righe d'intestazione di ruolo diverso con una data o `Oggetto:` fra i valori aprono la storia citata anche
    dentro il testo nuovo (una lettera con «Oggetto:» e «Data:» in testa, un «Da: gennaio / A: dicembre»).
- **In attesa** (7C, dopo il 7C.0): il Dossier per l'agente, il parser deterministico delle intestazioni citate
  (`origine_citata` di un inoltro). La verifica del taglio sul corpus reale (fuori dal repository) è la condizione
  A del gate dell'agente e resta aperta: il corpus delle prove è sintetico.
- **Senza chiamanti**: `Ordina`, `AttoValido`, `Atti`; `Motore.Riferimento` e `RubricaFissa` solo nelle prove; il
  ramo `ControparteCliente` di `triageUscita` non lo raggiunge l'ingest (`ingest.daInterpretare` non interpreta la
  posta in uscita verso un cliente).

## Dove intervenire

| Voglio… | Apri |
|---|---|
| cambiare quando si propone `nuova_rfq` o `aggancia` | `codici.go:Triage` (e i rami `triageFornitore`, `triageSconosciuto`, `triageUscita`) |
| capire verso quale RFQ si propone l'aggancio | `aggancio.go:MiglioreCandidatoAperto`, `EvidenzaDiRFQEsistente`, `CandidatoChiuso` |
| cambiare atto e legame proposti | `codici.go:attoELegameCliente`, `atto.go:AttoFornitore`, `atto.go:NonBusiness` |
| aggiungere un atto | `atto.go` (costanti e `Atti`) e una riga in `atto_business` con una migrazione |
| capire perché un codice c'è, non c'è o non è spuntabile | `regole.go:Motore.Estrai`, `CodiciDa`, `Estrazione.Proponibili`; `codici.go:sembraCodice`, `spazioOControllo` |
| capire codice e revisione letti da un nome di file | `codici.go:CodiceRev` (`reRevNas`, poi `reRev`) |
| capire perché un testo è finito nella storia citata | `catena.go:primaRigaDellaStoria` e i quattro riconoscitori (`apertureCitazione` per «ha scritto», «wrote», «schrieb», «a écrit») |
| usare un campo nuovo di `cliente.regole` nel motore | `core/registro/regole/regole.go` (schema e `Verifica`), poi `regole.go:Compila` |
| cambiare i suffissi decorativi | `regole.go:Canonico`, `CanonicoNome`, `senzaSuffissi`; lo schema in `core/registro/regole` |
| capire perché un mittente è cliente, fornitore, altro o ambiguo | `controparte.go:RisolviControparte`, `risolviIndirizzo` |
| aggiungere un dominio di posta pubblico | `controparte.go:dominiPubblici` (vale anche per il form del triage e per «Censisci») |
| cambiare tipo, codice o pre-spunta di un allegato | `proposta.go:PropostaDaNome`, `TipoDaEstensione`, `daScaricare` |
| cambiare punteggi o precedenza di R0–R5 | `aggancio.go` (qui) e le query in `core/inbox/aggancio` |
| cambiare i limiti di un codice | `codici.go:MaxCodice`, `MaxRev`, e la prova `platform/migrazioni/larghezze_db_test.go` |

## Leggi anche

`internal/core/README.md`, `internal/README.md` (flussi e tabella delle dipendenze), `core/registro/regole`
(lo schema), `core/inbox/ingest/controparte.go` (chi chiama `Triage` e scrive la proposta), `core/inbox/aggancio`
(i candidati dal database), `core/inbox/lettura` (il corpo mostrato), `cockpit/README.md` (§ «Il taglio della
catena di risposta», «Il contratto di classificazione», «I suffissi decorativi del cliente»).
