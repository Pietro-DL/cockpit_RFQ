-- Scarti dell'ingest e log delle decisioni di aggancio (migrazione 0003).

-- name: UpsertIngestScarto :one
-- Lo stesso elemento può fallire più volte: si tiene una riga per (casella, entry_id) con il conteggio
-- dei tentativi, non una riga per tentativo, altrimenti un elemento rotto riempie la tabella da solo.
INSERT INTO ingest_scarto (casella_id, entry_id, cartella, message_id, ricevuto_il, oggetto, origine, payload, errore)
VALUES (sqlc.arg(casella_id), sqlc.arg(entry_id), sqlc.narg(cartella), sqlc.narg(message_id),
        sqlc.narg(ricevuto_il), sqlc.narg(oggetto), sqlc.arg(origine), sqlc.arg(payload), sqlc.arg(errore))
ON CONFLICT (casella_id, entry_id) DO UPDATE SET
    cartella    = EXCLUDED.cartella,
    message_id  = EXCLUDED.message_id,
    ricevuto_il = EXCLUDED.ricevuto_il,
    oggetto     = EXCLUDED.oggetto,
    origine     = EXCLUDED.origine,
    payload     = EXCLUDED.payload,
    errore      = EXCLUDED.errore,
    tentativi   = ingest_scarto.tentativi + 1,
    ultimo_il   = now()
RETURNING *;

-- name: GetIngestScarto :one
SELECT * FROM ingest_scarto WHERE scarto_id = $1;

-- name: ListIngestScarti :many
SELECT * FROM ingest_scarto
WHERE (sqlc.narg(origine)::varchar IS NULL OR origine = sqlc.narg(origine)::varchar)
ORDER BY ultimo_il DESC LIMIT $1;

-- name: ContaIngestScartiPerOrigine :many
SELECT origine, count(*) AS n, max(ultimo_il) AS ultimo FROM ingest_scarto GROUP BY origine ORDER BY origine;

-- name: EliminaIngestScarto :execrows
DELETE FROM ingest_scarto WHERE scarto_id = $1;

-- name: EliminaScartoPerElemento :execrows
DELETE FROM ingest_scarto WHERE casella_id = $1 AND entry_id = $2;

-- name: InsertAgganciaLog :exec
-- Ogni decisione di aggancio lascia una traccia: automatica (utente NULL) o di una persona.
INSERT INTO messaggio_aggancio_log (messaggio_id, thread_id, azione, utente_id, motivo)
VALUES (sqlc.arg(messaggio_id), sqlc.narg(thread_id), sqlc.arg(azione), sqlc.narg(utente_id), sqlc.narg(motivo));

-- name: ListAgganciaLog :many
SELECT * FROM messaggio_aggancio_log WHERE messaggio_id = $1 ORDER BY eseguito_il DESC;
