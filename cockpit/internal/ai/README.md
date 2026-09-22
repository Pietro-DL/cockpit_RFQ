# `ai/` — l'analisi semantica come proposta, mai come decisione

## Scopo

Leggere un messaggio con un modello di linguaggio e proporre un'interpretazione, che una persona poi conferma o
scarta. Niente di ciò che esce da qui è una decisione.

## Non appartiene qui

Le regole deterministiche (stanno in `core/inbox/classificazione`), gli handler HTTP, la coda.

## Package posseduti

Solo `agente`: `Servizio`, `Contesto`, `Proposta`, la `Verifica` (grounding), i prompt, e in `api.go` il client
HTTP del provider. Per decisione del 21/09 il client **non** si divide in un package a parte: `ai/` resta con un
package solo.

## Dipendenze consentite

`core`, `platform`. Mai `transport`, mai `app`.

## Entry point

`agente.Servizio.Analizza`, chiamato solo dall'esecutore del server per il job `analizza_messaggio_ai`, che a
sua volta nasce solo da `POST /messaggio/{id}/analizza`.

## Flussi principali

Job `analizza_messaggio_ai` → `Contesto` (oggetto, corpo tagliato a 12k, nomi degli allegati, cliente) → prompt
→ modello → `Proposta` → `Verifica`: ogni codice, riferimento e candidato viene ricontrollato contro i fatti in
database, e ciò che non regge viene scartato e conservato → riga in `analisi_messaggio`.

## Invarianti

- **Spento** senza `[agente].attivo`, senza modello e senza chiave; e solo sulle caselle elencate.
- La chiave non sta nel toml: `chiave_env` è il **nome** di una variabile d'ambiente.
- Ogni codice, riferimento e candidato è riverificato contro i fatti; lo scartato si conserva, non sparisce.
- Non scrive `thread_id`, non tocca il NAS, non tocca Outlook.
- Mai il contenuto dei PDF nel prompt.
- **Nessuna chiamata reale nei test**: il gate per accendere l'agente (corpus, autorizzazione IT/ISO) non è
  chiuso, e finché non lo è il pacchetto resta spento.

## Effetti collaterali

Una riga in `analisi_messaggio` (idempotente per messaggio e versione del prompt). Traffico verso il servizio
esterno **solo** quando è acceso.

## Test

L1, senza rete: il modello è un'interfaccia e nei test è una finzione locale.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| un campo nuovo nella Proposta | lo schema, la `Verifica` e il prompt insieme, e alza `VersionePrompt` |
| un provider nuovo | `api.go` (è un ticket a parte) |
| capire perché l'agente non parte | `[agente]` nel toml, e le caselle elencate |

## Leggi anche

`internal/README.md`, `core/README.md` (le regole deterministiche che l'agente non sostituisce),
`platform/README.md` (`[agente]` in configurazione).
