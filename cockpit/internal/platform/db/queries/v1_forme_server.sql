-- V1, direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: le query delle forme di R2 costruite dal server (crea con
-- partenza step, cambia_sorgente, riallinea_proposta) che le altre non hanno. File proprio della corsia (INC-G21).
--
-- Chi le chiama: core/rfq/distintabozza (importa3.go), dentro la transazione di R2, dopo BloccaThread.
-- Regole (le stesse di v1_formato3.sql): il perimetro della RFQ sta nel WHERE; nessun ON CONFLICT.

-- name: GetDocumentoFonteConfermata :one
-- La fonte confermata del prodotto (U2 §2.8, base [T] U2-P38): il documento STEP (cad_3d) confermato, e non sostituito, del
-- prodotto con quello sha256. Nessuna riga = la sorgente dichiarata non e' lo STEP confermato del prodotto
-- (422 FONTE_NON_CONFERMATA). Il confronto e' per contenuto, mai per allegato.
SELECT d.documento_id, d.confermato_il
  FROM documento d
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.componente_id = sqlc.arg(componente_id)::uuid AND d.sha256 = sqlc.arg(sha256)::text
   AND d.tipo = 'cad_3d' AND d.sostituito_da IS NULL;
