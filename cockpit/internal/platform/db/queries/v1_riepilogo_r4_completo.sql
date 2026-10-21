-- V1, direttiva 39, blocco «R4 completo» della corsia persistenza (EM §7.4, §9.4, U2 §5.4, §6.1), file proprio per INC-G21:
-- le letture che R4 formato 3 aggiunge per le esclusioni legacy, le condizioni (c) ed (e) di prima-istanza-1 e i conti della
-- verifica prevista. Le chiama solo core/rfq/distintabozza (riepilogo3_lettura.go). Solo SELECT, senza lucchetti: R4 le
-- legge nella sua transazione REPEATABLE READ READ ONLY e R5 nella connessione S, dopo il lucchetto della RFQ.

-- name: ListRigheStrutturaR4 :many
-- Le righe legacy (componente_proposta) dei contenuti dati, su ogni copia della RFQ: lo STEP sorgente della bozza (le righe
-- da decidere della condizione (c), le esclusioni fatte nelle pagine di prima) e gli STEP secondari autorizzati (la
-- condizione (e)). Il segno di chi ha deciso (deciso_da) distingue una persona da un automatismo.
SELECT cp.proposta_id, cp.allegato_id, cp.sha256::text AS sha256, cp.chiave, cp.stato, cp.deciso_da, cp.deciso_il, cp.nota
  FROM componente_proposta cp
 WHERE cp.thread_id = sqlc.arg(thread_id) AND cp.sha256 = ANY (sqlc.arg(shas)::text[])
 ORDER BY cp.sha256, cp.chiave, cp.allegato_id, cp.proposta_id;
