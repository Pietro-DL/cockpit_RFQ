-- V1, ondata C, corsia persistenza: il rifiuto della proposta aggiornata (R15; U2 §5.7, 0023 riga 9). La tabella e' in
-- sola aggiunta (P4X02 su UPDATE, DELETE e TRUNCATE): qui solo l'INSERT e le letture.

-- name: InsertPropostaAggiornataRifiuto :one
-- Un rifiuto della proposta aggiornata di un prodotto confermato (R15), nella transazione del gesto, dopo BloccaThread. La
-- data la scrive il database.
INSERT INTO proposta_aggiornata_rifiuto (thread_id, prodotto_id, conferma_id, gruppo, impronta_contenuto, differenze, motivo, autore_id,
                                         chiave_idempotenza)
VALUES (sqlc.arg(thread_id), sqlc.arg(prodotto_id), sqlc.arg(conferma_id), sqlc.arg(gruppo), sqlc.arg(impronta_contenuto),
        sqlc.arg(differenze), sqlc.arg(motivo), sqlc.arg(autore_id), sqlc.arg(chiave_idempotenza))
RETURNING rifiuto_id, quando;

-- name: ListRifiutiPropostaProdotto :many
-- I rifiuti delle proposte aggiornate di un prodotto della RFQ, dal piu' recente, con il nome dell'autore: /dati non
-- mostra il segno per un'impronta rifiutata sulla conferma corrente e dice «proposta del ‹data› rifiutata da ‹utente›»
-- (U2 §5.7). Solo SELECT.
SELECT r.rifiuto_id, r.conferma_id, r.gruppo, r.impronta_contenuto::char(64) AS impronta_contenuto, r.motivo, r.autore_id,
       u.nome AS autore_nome, r.quando
  FROM proposta_aggiornata_rifiuto r JOIN utente u ON u.utente_id = r.autore_id
 WHERE r.thread_id = sqlc.arg(thread_id) AND r.prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY r.quando DESC, r.rifiuto_id;
