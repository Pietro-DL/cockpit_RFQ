# `internal/core/rfq/traccia` — il motivo scritto dal server

## Scopo

- Messaggio 46 dell'utente (21/10) [U]: il motivo scritto dall'utente si toglie da **ogni** widget.
- La decisione resta dell'utente: è il gesto, con il suo autore nelle colonne di sempre (`autore_id`, `chiusa_da`,
  `utente_id`). Il testo del motivo, dove lo schema lo vuole, lo scrive il server e lo dice:
  `gesto di <sigla> da «<widget>»: <che cosa>`, più le evidenze del motore dove ci sono.
- Non finge mai che il testo sia dell'utente. Se un client di prima manda comunque un motivo, vale il suo
  (`DellUtente`).
- Nessun DDL: le colonne `motivo` della 0022 e della 0023 sono quelle di sempre.

## Funzioni

- `Gesto(max, sigla, widget, cosa, evidenze...)`: la traccia, su una riga, al più `max` caratteri (rune), con `…` se
  tagliata. Deterministica: lo stesso gesto sugli stessi dati dà lo stesso testo.
- `Sigla(ctx, q, utente)` e `DelGesto(...)`: la sigla dell'utente letta dal database, nella transazione di chi chiama.
- `DellUtente(motivo, traccia)`: il motivo dell'utente, se c'è, altrimenti la traccia.

## Idempotenza

- Il reinvio con la stessa chiave si riconosce confrontando il motivo registrato con la traccia **esatta** che il
  server genererebbe con gli stessi dati, mai con un prefisso.
- Le impronte dei corpi restano quelle del corpo mandato dalla pagina: la traccia non entra nell'impronta, salvo dove
  il corpo letto dal server è già la forma che si impronta (i gesti dei file: lì la traccia è deterministica e il
  reinvio dello stesso corpo dà la stessa impronta).

## Dove si usa

- `core/rfq/sorgente`: S8 (codice della radice diverso, solo prodotto, codice letto dal file diverso, riassociazione),
  revoca della sorgente.
- `core/rfq/filerfq`: «Scarta» (`file_rfq_evento.motivo`, `ck_fre_coerente`).
- `core/rfq/distintabozza`: R15 (`proposta_aggiornata_rifiuto.motivo`), le forme di R2 costruite dal server (abbandono,
  cambio della fonte, riallineamento, conversione), il rebase, R5 (`conferma_strutturale_non_incluso` per «Non includere
  nella BOM», `conferma_strutturale.mono_pezzo_motivo`).
