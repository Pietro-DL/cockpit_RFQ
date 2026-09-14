-- Fondazioni (migrazione 0002): caselle, postazioni, credenziali dei worker, store locali.
-- Il seed da cockpit.toml è non distruttivo: aggiorna l'esistente per chiave naturale e non
-- disattiva mai ciò che non è più nel file (lo segnala soltanto).

-- name: UpsertCasella :one
INSERT INTO casella (canale, indirizzo, nome, condivisa, utente_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (canale, indirizzo) DO UPDATE SET
    nome = EXCLUDED.nome, condivisa = EXCLUDED.condivisa,
    utente_id = EXCLUDED.utente_id, attiva = true, aggiornato_il = now()
RETURNING *;

-- name: GetCasella :one
SELECT * FROM casella WHERE casella_id = $1;

-- name: GetCasellaPerIndirizzo :one
SELECT * FROM casella WHERE canale = $1 AND indirizzo = $2;

-- name: ListCaselle :many
SELECT * FROM casella ORDER BY nome;

-- name: ListCaselleAttive :many
SELECT * FROM casella WHERE attiva ORDER BY nome;

-- name: UpsertPostazione :one
INSERT INTO postazione (nome_host, descrizione, utente_id)
VALUES ($1, $2, $3)
ON CONFLICT (nome_host) DO UPDATE SET
    descrizione = EXCLUDED.descrizione, utente_id = EXCLUDED.utente_id,
    attiva = true, aggiornato_il = now()
RETURNING *;

-- name: GetPostazionePerHost :one
SELECT * FROM postazione WHERE nome_host = $1;

-- name: ListPostazioni :many
SELECT * FROM postazione ORDER BY nome_host;

-- name: UpsertWorkerCredenziale :one
INSERT INTO worker_credenziale (worker_nome, worker_tipo, token_hash, postazione_id, caselle)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (worker_nome) DO UPDATE SET
    worker_tipo = EXCLUDED.worker_tipo, token_hash = EXCLUDED.token_hash,
    postazione_id = EXCLUDED.postazione_id, caselle = EXCLUDED.caselle,
    attivo = true, aggiornato_il = now()
RETURNING *;

-- name: GetWorkerCredenziale :one
SELECT * FROM worker_credenziale WHERE worker_nome = $1 AND attivo;

-- name: ListWorkerCredenziali :many
SELECT * FROM worker_credenziale ORDER BY worker_nome;

-- name: UpsertCasellaStore :one
INSERT INTO casella_store (postazione_id, casella_id, store_id)
VALUES ($1, $2, $3)
ON CONFLICT (postazione_id, casella_id) DO UPDATE SET
    store_id = EXCLUDED.store_id, rilevato_il = now()
RETURNING *;

-- name: GetCasellaStore :one
SELECT * FROM casella_store WHERE postazione_id = $1 AND casella_id = $2;

-- name: ListCasellaStorePerPostazione :many
SELECT cs.*, c.indirizzo, c.nome AS casella_nome
FROM casella_store cs JOIN casella c ON c.casella_id = cs.casella_id
WHERE cs.postazione_id = $1
ORDER BY c.nome;
