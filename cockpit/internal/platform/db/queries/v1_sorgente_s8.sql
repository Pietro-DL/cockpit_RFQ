-- V1, direttiva 39 (S8, revoca della sorgente, RFQ formato 3): le query della corsia persistenza in un file proprio
-- (INC-G21). Solo letture: le scritture di S8 passano dalle query che ci sono gia' (documento, fonte_step_evento,
-- esito_intento, R2).

-- name: RfqFormato3 :one
-- «RFQ formato 3» (direttiva 39 §6 [U]; NOMI (i) [T-39]): almeno una bozza o una conferma formato 3, in qualunque
-- momento, su un prodotto della RFQ. L'unica definizione e' quella del database (rfq_e_formato3, dalla variante 20261019c;
-- vigente la 20261019d): un segno per RFQ scritto alla nascita della prima bozza formato 3, che la conservazione delle
-- bozze abbandonate non tocca.
SELECT rfq_e_formato3(sqlc.arg(thread_id)::uuid)::bool AS formato3;

-- name: ListFonteStepPerChiave :many
-- Gli eventi della storia della fonte scritti con una chiave d'idempotenza (S8 passo 2: la riga conferma_documento con
-- k2; la revoca con la sua chiave), della RFQ, in ordine.
SELECT evento_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id, motivo, chiave_idempotenza, autore_id, quando
  FROM fonte_step_evento
 WHERE thread_id = sqlc.arg(thread_id) AND chiave_idempotenza = sqlc.arg(chiave)::uuid
 ORDER BY evento_id;
