# confermadoc

La conferma di una proposta in documento del Fascicolo, resa funzione del core (direttiva 35, blocco C-3, INC-G9;
direttiva 39 §2, passo 2 di S8). Fino al blocco C-3 stava nel web (`transport/web/allegati.go`, `conferma` e
`confermaProposta`) e leggeva la richiesta HTTP.

## Che cosa fa

- `Conferma(ctx, q, Richiesta)`, sulla transazione di chi chiama (la RFQ gia' presa, anche `FOR KEY SHARE`, e la proposta
  bloccata dopo): il documento con la sua provenienza, la proposta chiusa (o «duplicato», se lo stesso file e' gia' un
  documento della RFQ), la riga `conferma_documento` della storia della fonte per uno STEP `cad_3d` del finito, la scelta
  della revisione (aggiungi / sostituisce), la copia sul NAS in coda. I testi sono quelli di sempre (`Esito.Messaggio`).
- La guardia di INC-G9: con una associazione registrata attiva dell'allegato il documento entra solo sotto il componente
  della AR; un altro componente, o nessuno, e' un rifiuto (`fascicolo.Rifiuto`). La strada «Sostituisci» del servizio
  unico non si apre qui (vuole una conferma della BOM formato 3; D-AR01 A, D-AR03 D).
- `Fonte`: S8 passa la chiave del passo 2, il gruppo dell'anteprima e il motivo, che vanno nella riga della fonte
  (`ScriviConfermaDocumento`); le pagine legacy la lasciano vuota e la riga e' quella di prima.
- Le regole della scelta della revisione (`VerificaScelta`, `VerificaSostituzione`, `ApplicaScelta`,
  `NotaSostituzione`, `CorrentiDelloStessoTipo`, `CodiceDaComponente`) stanno qui; le pagine legacy del web le chiamano
  con i loro nomi di sempre (`fascicolo.go`).

## Chi la chiama

- `transport/web/allegati.go` («Conferma il documento»), `fascicolo_conferma.go` (il lotto), `fascicolo_gesti_v3.go`
  («Documentazione generale»), attraverso `confermaProposta`;
- `core/rfq/sorgente` (S8, passo 2), con il componente = il prodotto, il tipo `cad_3d` e la scelta «aggiungi» (S8 non
  sostituisce mai il documento della working).

## Prove

- L1 `conferma_test.go` (le regole pure);
- L4 `transport/web/conferma_core_db_test.go` (la guardia di INC-G9 dalla rotta legacy) e tutte le prove legacy della
  conferma del package web, invariate.
