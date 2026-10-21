-- V1, direttiva 35, blocco C-4 (parte persistenza; corsia persistenza, file proprio per INC-G21): la lettura delle bozze
-- di tutti i prodotti della RFQ per S9 (U2 §2.3, §12 punto 6; EM §10.9 «S9»). Solo SELECT, senza lucchetti: valgono
-- dentro la transazione REPEATABLE READ READ ONLY di chi legge (il modello di /motore-a/dati e S9), insieme alla
-- fotografia. Le chiama solo core/rfq/distintabozza (bozze_rfq.go: LeggiBozzeDellaRfq).
--
-- Regole (le stesse di p4_conferme.sql e v1_motore.sql): il perimetro sta nel WHERE (la RFQ del percorso), versioni ed
-- eventi con una JOIN sulla testata; le colonne tx non si leggono; gli eventi fino alla versione corrente della testata
-- (versioni ed eventi non si riscrivono). Nessuna lettura di documento_proposta: le PB sono {allegato_id, chiave_bozza}
-- (direttiva 35 §1.1 [U], DIR35-PB) e le AR le legge il servizio unico (v1_associazioni.sql), mai la copia.

-- name: ListBozzeAperteDellaRfq :many
-- Le bozze aperte della RFQ, una per prodotto (ux_distinta_bozza_aperta), con la versione corrente: lo snapshot salvato e
-- la sua impronta, chi l'ha salvata e quando. Tutti i formati: chi legge rilegge lo snapshot con il formato della
-- testata.
SELECT b.bozza_id, b.prodotto_id, b.formato, b.versione, v.snapshot, v.impronta, v.salvata_da, v.salvata_il
  FROM distinta_bozza b
  JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
 WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta'
 ORDER BY b.prodotto_id, b.bozza_id;

-- name: ListEventiFileBozzeAperteDellaRfq :many
-- Gli eventi delle azioni date (quelle che la regola del vassoio «Da ricollocare» guarda: file_svincolato,
-- file_preassociato, file_lasciato, annulla, rifai; EM §3.7) nel giornale delle bozze aperte della RFQ, fino alla
-- versione corrente di ogni bozza, in ordine di bozza e di numero.
SELECT e.* FROM distinta_bozza_evento e
  JOIN distinta_bozza b ON b.bozza_id = e.bozza_id
 WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta'
   AND e.versione <= b.versione
   AND e.azione = ANY (sqlc.arg(azioni)::text[])
 ORDER BY e.bozza_id, e.n;
