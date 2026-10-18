-- V1 (giro 5, A1d; direttiva 23): le letture del modello della pagina della Distinta (/motore-a/dati, emendamento di V1
-- §9.9; piano della 0023 §14 passo 3, corsia motore). Solo SELECT, nello snapshot REPEATABLE READ READ ONLY del modello,
-- insieme alla fotografia: le occorrenze non attive (il ciclo dei file), le bozze aperte formato 3 con le colonne del
-- loro registro (il gruppo corrente della sorgente, mai il contenuto) e lo stato del rismistamento F8. L'identità
-- certificata dei componenti la legge ListIdentitaRegistrate (v1_formato3.sql).
--
-- Regole (le stesse di p4_conferme.sql e v1_formato3.sql): il perimetro sta nel WHERE (la RFQ del percorso); le colonne
-- tx non si leggono; nessuna scrittura.

-- name: ListFileRfqNonAttivi :many
-- Le occorrenze della RFQ in «Altri file» o scartate, con lo stato (EM §10.8: NonAttivi del modello e dell'adattatore).
-- Un'occorrenza senza riga in file_rfq è attiva.
SELECT allegato_id, stato
  FROM file_rfq
 WHERE thread_id = sqlc.arg(thread_id) AND stato <> 'attivo'
 ORDER BY allegato_id;

-- name: ListSorgentiBozzeAperte :many
-- Le bozze aperte formato 3 della RFQ, una per prodotto, con la versione corrente e, dal registro della versione, il
-- gruppo, la fonte, la radice e l'impronta del contenuto; mai il contenuto (EM §6.4.1, §9.9: bozza_del_prodotto). Senza
-- registro le colonne del registro sono NULL e vale la fonte della testata.
SELECT b.bozza_id, b.prodotto_id, b.versione, b.fonte_allegato_id, b.fonte_sha256, b.fonte_gruppo, v.importazione_id,
       i.gruppo AS registro_gruppo, i.fonte_allegato_id AS registro_fonte_allegato_id, i.fonte_sha256 AS registro_fonte_sha256,
       i.fonte_radice AS registro_fonte_radice, i.impronta_contenuto AS registro_impronta_contenuto
  FROM distinta_bozza b
  JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
  LEFT JOIN distinta_importazione i ON i.importazione_id = v.importazione_id
 WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta' AND b.formato = 3
 ORDER BY b.prodotto_id, b.bozza_id;

-- name: GetStatoF8DelThread :one
-- Lo stato del rismistamento F8 della RFQ (f8_stato, 0022): quante righe in errore, quante in attesa e quante in tutto.
-- Nessuna riga = F8 mai previsto.
SELECT count(*) FILTER (WHERE stato = 'errore')::int    AS in_errore,
       count(*) FILTER (WHERE stato = 'in_attesa')::int AS in_attesa,
       count(*)::int                                     AS totale
  FROM f8_stato
 WHERE thread_id = sqlc.arg(thread_id);
