# `ai/` — l'analisi semantica come proposta, mai come decisione

## Scopo

Leggere un messaggio già in database con un modello di linguaggio e proporre un'interpretazione — intento,
richiesta a cui somiglia, codici, tipi degli allegati, che cosa manca, una bozza di risposta — che una persona
poi usa o ignora. Niente di ciò che esce da qui è una decisione: il risultato vive in `analisi_messaggio` e in
un riquadro del pannello del messaggio.

Un solo package, `agente`. Per decisione del 21/09 il client HTTP del fornitore **non** si divide in un
package a parte.

## Non appartiene qui

Le regole deterministiche (`core/inbox/classificazione`) e i candidati di aggancio (`core/inbox/aggancio`),
che l'agente legge e non sostituisce; gli handler HTTP (`transport/web/agente.go`); la coda e chi esegue il job
(`platform/coda`, `app/runtime/esecutore.go`); la configurazione `[agente]` (`platform/config`).

## File

| File | Righe | Responsabilità |
|---|---|---|
| `agente/agente.go` | 357 | `VersioneAnalizzatore` («1»), `VersionePrompt` («p1»), gli intenti ammessi, `Proposta` (`CandidatoAI`, `CodiceAI`, `AllegatoAI`), `Scarto`, `Contesto` (`RichiestaNota`), `ErrSchema`, `Valida` (schema stretto), `Verifica` (il grounding), `Contesto.Impronta`, l'interfaccia `Modello`, `Esito`, `Analizza` (il giro senza database), `ripulisci` |
| `agente/prompt.go` | 93 | `istruzioni` (il testo di sistema), `Prompt(Contesto)`, `taglia` (corpo a 12000 byte) |
| `agente/servizio.go` | 220 | `Servizio{Modello, Attivo, Caselle}`, `ErrSpento`, `Consentito`, `Servizio.Contesto` (che cosa esce, letto dal database), `Servizio.Analizza` (idempotenza e riga in `analisi_messaggio`), `LeggiProposta`, `PropostaLeggibile`; il controllo all'avvio che i ruoli del dominio siano quelli che l'agente accetta |
| `agente/api.go` | 130 | `ClientAPI`, `DaAmbiente` (nil senza chiave o senza modello; URL vuoto = quello predefinito del fornitore), `Chiedi` (una POST JSON, nessuna ripetizione) |
| `agente/agente_test.go` | 244 | L1 con un modello finto |

## Entry point

| Simbolo | Chi lo usa |
|---|---|
| `Servizio` | costruito in `app/runtime/servizi.go:CostruisciServizi`: `Attivo` = `[agente].attivo`, `Modello` = `DaAmbiente([agente].url, [agente].modello, [agente].chiave_env)`, `Caselle` = `[agente].caselle` in minuscolo. Va a `EsecutoreServer.Agente` e a `web.Server.Agente` |
| `Servizio.Analizza` | `app/runtime/esecutore.go:esegui`, job `analizza_messaggio_ai` |
| `Servizio.Consentito` | `transport/web/agente.go`: `analisiPer` (il pulsante ha senso?) e `chiediAnalisi` (`POST /messaggio/{id}/analizza` accoda il job, chiave `analisi-ai:<messaggio>`) |
| `LeggiProposta`, `Proposta`, `Scarto` | `transport/web/agente.go:analisiPer`, per il riquadro del pannello |
| `Analizza`, `Valida`, `Verifica`, `Prompt`, `Contesto.Impronta` | interni al package e ai test |
| `PropostaLeggibile` | nessun chiamante |

## Dati

| Chi | Legge | Scrive |
|---|---|---|
| `Servizio.Contesto` | `messaggio` (oggetto, `corpo_testo`, mittente), `v_inbox` (cliente), `allegato` (nomi di natura `file` ed `elemento_outlook`), `candidato_aggancio` ⋈ `thread_offerta` ⋈ `cliente` (`ListCandidatiAggancio`), `candidato_codice` del messaggio (`ListCandidatiCodice`) | — |
| `Servizio.Analizza` | `analisi_messaggio` (`GetAnalisiPerInput`) | `analisi_messaggio` (`ApriAnalisi` in `in_corso`, poi `ChiudiAnalisi`) |
| `LeggiProposta` | `analisi_messaggio` (`UltimaAnalisiCompletata`) | — |

`analisi_messaggio` (migrazione 0008) è unica per `(messaggio_id, input_hash, prompt, modello)`; `stato` è
`in_corso`, `completata`, `rifiutata` o `errore`; `risultato` è la proposta dopo schema e grounding, `grezzo`
la risposta così come è arrivata (JSON, oppure stringa JSON se non lo era), `scartato` gli scarti con il
motivo, più token e durata. Nessuna transazione: `ApriAnalisi`, la chiamata al servizio esterno e
`ChiudiAnalisi` sono tre passi separati sul pool. Niente file su disco.

## Flussi principali

1. `POST /messaggio/{id}/analizza` (web): le caselle in cui il messaggio è presente → `Consentito` (acceso, con
   modello, e almeno una casella elencata) → `coda.Accoda` di `analizza_messaggio_ai`, con il solo
   `messaggio_id` nel payload.
2. L'esecutore del server prende il job → `Servizio.Analizza`: spento → `ErrSpento`, e il job fallisce in modo
   definitivo, senza altri tentativi (`app/runtime/esecutore.go:definitivo`).
3. `Servizio.Contesto`: oggetto, corpo intero, mittente, cliente, nomi degli allegati, richieste candidate
   (id, oggetto, «regola — evidenza»), codici candidati del messaggio.
4. `Impronta` = sha256 del `Contesto` in JSON. Se c'è già una riga per (messaggio, impronta, prompt, modello) si
   restituisce quella, qualunque sia il suo stato, senza chiamare nessuno. Altrimenti `ApriAnalisi`; se un
   altro l'ha aperta nel frattempo, si rilegge la sua.
5. `Analizza`: `Prompt` (istruzioni + contesto, corpo tagliato a 12000 byte) → `Modello.Chiedi` → `ripulisci`
   (toglie il recinto markdown) → `Valida` → `Verifica`.
6. `ChiudiAnalisi`: `completata` con la proposta verificata; `rifiutata` se lo schema non regge; `errore` per un
   errore del servizio o della rete. Negli ultimi due `risultato` è `{}`. Il job si chiude `fatto` in tutti e
   tre i casi, con `analisi_id`, `stato`, il numero degli elementi scartati e i token nel risultato.
7. Il pannello del messaggio mostra l'ultima analisi `completata` (`LeggiProposta`) e i suoi scarti.

## Invarianti

- **Spento** senza `[agente].attivo`, senza modello o senza chiave (`DaAmbiente` restituisce nil, e un
  `Servizio` senza `Modello` non chiama); un elenco di caselle vuoto vale «nessuna». La casella si controlla
  quando il job si accoda (`web/agente.go:chiediAnalisi`); l'esecutore controlla solo acceso e modello.
- La chiave non sta nel toml: `chiave_env` è il **nome** di una variabile d'ambiente.
- Esce solo ciò che sta nel `Contesto`: oggetto, corpo, mittente, cliente, nomi degli allegati, candidati. Mai
  il contenuto dei file.
- Lo schema è stretto: un campo sconosciuto, testo dopo l'oggetto JSON, un intento, un ruolo o un tipo di
  allegato fuori elenco, un punteggio fuori da 0–100 fanno rifiutare **tutta** la risposta, senza effetti parziali.
- Il grounding sta in Go (`Verifica`), non nel prompt: un codice deve comparire nel testo, nei nomi degli
  allegati o fra i codici candidati del messaggio; il riferimento nel testo; un candidato fra quelli passati;
  un allegato con il suo nome esatto; un codice contenuto nel riferimento resta fuori se non ha il ruolo
  `riferimento_rfq`. La bozza cade intera se cita un gruppo con la forma di un codice (almeno tre cifre) senza
  riscontro. Lo scartato si conserva, con il motivo.
- Scrive solo `analisi_messaggio`: niente `thread_id`, niente NAS, niente Outlook, nessun'altra tabella.
- `temperature` 0: lo stesso messaggio deve dare lo stesso esito.
- I ruoli dei codici sono quelli del dominio: un ruolo di `classificazione` che l'agente non accetta fa
  fallire l'avvio del processo (panic in inizializzazione).
- **Nessuna chiamata reale nei test.**

## Dipendenze

Importa `core/inbox/classificazione` (solo per il controllo dei ruoli) e `platform/db`: consentito. È importato
da `app/runtime` (`servizi.go`, `esecutore.go`) e da `transport/web` (`server.go`, `agente.go`). Non importa
`transport` né `app`.

## Test

L1, `go test ./internal/ai/...`, senza rete e senza chiave: il `Modello` è un finto locale.

| Test | Che cosa prova |
|---|---|
| `TestLoSchemaRifiutaCioCheIlSistemaNonSaLeggere` | intento, ruolo o tipo di allegato inventati, campo sconosciuto, punteggio fuori scala, testo dopo il JSON, risposta non JSON: tutti rifiutati; una risposta conforme passa intera |
| `TestIlGroundingScartaCioCheNonEsiste` | riferimento, codici, candidati e allegati senza riscontro tolti, ognuno con il suo scarto |
| `TestUnaBozzaConUnCodiceInventatoCadeIntera` | la bozza con un codice inesistente sparisce; quella con i soli codici veri resta |
| `TestIlRiferimentoNonDiventaCodiceNemmenoDallAgente` | il numero della richiesta non passa come codice prodotto |
| `TestAnalizzaFaIlGiroCompleto` | recinto markdown, grounding, grezzo e token conservati, che cosa finisce nel prompt |
| `TestUnErroreDiReteNonProduceProposte` | errore → nessuna proposta |
| `TestLImprontaDistingueGliInput` | stessa impronta per lo stesso contesto, diversa per un carattere |
| `TestIlPromptDescriveLoSchemaCheIlCodiceLegge` | ogni campo e ogni intento sono nominati nelle istruzioni |

Senza prove: la parte con il database (`Servizio.Contesto`, `Servizio.Analizza`, `LeggiProposta`), `Consentito`,
le rotte web dell'agente, `ClientAPI` (di proposito).

## Stato dell'implementazione

- Completo il giro del checkpoint 3R §9: pulsante e riquadro nel pannello, job, contesto, prompt, schema,
  grounding, idempotenza, client. **Spento di default**: il gate per accenderlo — la verifica del taglio della
  catena sul corpus reale (condizione A) e l'autorizzazione IT/ISO per casella — non è chiuso.
- In attesa (dopo il 7C.0): il Dossier, cioè il pacchetto di fatti e candidati da mandare all'agente con lo
  stato di codici e revisioni nella RFQ candidata, e il contratto nuovo dell'agente. Oggi il prompt riceve
  `messaggio.corpo_testo` intero, tagliato a 12000 byte, non il testo dopo il taglio della catena.
- Limiti noti:
  - l'idempotenza non guarda lo stato: un'analisi finita in `errore` (o rimasta `in_corso` dopo un arresto del
    processo) viene restituita a ogni nuova richiesta con lo stesso input, prompt e modello, e il servizio non
    viene richiamato;
  - `Chiedi` non ripete: timeout di 60 secondi, poi `errore`;
  - l'esecutore del server è una goroutine sola: una chiamata lenta ferma per quel tempo anche le copie sul
    NAS e le estrazioni;
  - `taglia` conta byte, non caratteri.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| un campo nuovo nella Proposta | `agente.go` (tipo e `Valida`), `Verifica`, le istruzioni in `prompt.go`, il test dello schema; e alza `VersionePrompt` |
| cambiare le istruzioni | `prompt.go`, alzando `VersionePrompt` insieme |
| cambiare che cosa esce verso il servizio | `servizio.go:Contesto`, l'unico punto |
| un provider nuovo | un altro `Modello`; `api.go` è quello di oggi (ticket a parte) |
| capire perché l'agente non parte | `[agente]` nel toml (acceso, modello, `chiave_env`, caselle), la variabile d'ambiente, il log di avvio di `app/runtime/servizi.go` |
| capire perché una proposta non compare | la riga in `analisi_messaggio`: `stato`, `errore`, `scartato` |

## Leggi anche

`internal/README.md`, `internal/core/README.md` (le regole deterministiche che l'agente non sostituisce),
`internal/platform/config/README.md` (`[agente]` in configurazione), `internal/app/README.md` (l'esecutore),
`internal/transport/README.md` (`POST /messaggio/{id}/analizza`).
