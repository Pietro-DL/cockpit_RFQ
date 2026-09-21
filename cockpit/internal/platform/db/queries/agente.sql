-- Analisi semantica (checkpoint 3R §9). L'agente propone: nessuna di queste query tocca thread_id,
-- documento, componente o il NAS.

-- name: ApriAnalisi :one
INSERT INTO analisi_messaggio (messaggio_id, input_hash, versione, prompt, modello, stato)
VALUES ($1, $2, $3, $4, $5, 'in_corso')
ON CONFLICT (messaggio_id, input_hash, prompt, modello) DO NOTHING
RETURNING *;

-- GetAnalisiPerInput: lo stesso input, con lo stesso prompt e lo stesso modello, e' gia' stato chiesto?
-- Serve a non ripagare due volte la stessa domanda e a rendere la rianalisi una scelta, non un effetto.
-- name: GetAnalisiPerInput :one
SELECT * FROM analisi_messaggio
WHERE messaggio_id = $1 AND input_hash = $2 AND prompt = $3 AND modello = $4;

-- name: ChiudiAnalisi :one
UPDATE analisi_messaggio
SET stato = $2, risultato = $3, grezzo = $4, scartato = $5, errore = sqlc.narg(errore),
    token_in = sqlc.narg(token_in), token_out = sqlc.narg(token_out), durata_ms = sqlc.narg(durata_ms)
WHERE analisi_id = $1
RETURNING *;

-- name: UltimaAnalisiCompletata :one
SELECT * FROM analisi_messaggio
WHERE messaggio_id = $1 AND stato = 'completata'
ORDER BY creato_il DESC LIMIT 1;

-- name: ListAnalisiMessaggio :many
SELECT * FROM analisi_messaggio WHERE messaggio_id = $1 ORDER BY creato_il DESC LIMIT 10;

-- ContaAnalisiPerStato: la metrica del replay L6 e del banco. «Quante proposte dell'agente sono state
-- accettate senza modifica» si legge dal log degli agganci, non da qui.
-- name: ContaAnalisiPerStato :many
SELECT stato, count(*) AS n FROM analisi_messaggio GROUP BY stato ORDER BY stato;
