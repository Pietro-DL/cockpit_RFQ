-- name: GetUtentePerSigla :one
SELECT * FROM utente WHERE sigla = $1 AND attivo;

-- name: GetUtente :one
SELECT * FROM utente WHERE utente_id = $1;

-- name: ListUtenti :many
SELECT * FROM utente ORDER BY sigla;

-- name: UpsertUtente :one
INSERT INTO utente (sigla, nome, ufficio, ruolo, password_hash)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (sigla) DO UPDATE SET nome = EXCLUDED.nome, ufficio = EXCLUDED.ufficio, ruolo = EXCLUDED.ruolo,
    password_hash = COALESCE(EXCLUDED.password_hash, utente.password_hash), attivo = true
RETURNING *;

-- name: CreaSessione :one
INSERT INTO sessione (token, utente_id, scade_il) VALUES ($1, $2, $3) RETURNING *;

-- name: GetSessioneUtente :one
SELECT u.* FROM sessione s JOIN utente u ON u.utente_id = s.utente_id
WHERE s.token = $1 AND s.scade_il > now() AND u.attivo;

-- name: ToccaSessione :exec
UPDATE sessione SET ultimo_accesso = now() WHERE token = $1;

-- name: EliminaSessione :exec
DELETE FROM sessione WHERE token = $1;

-- name: EliminaSessioniScadute :execrows
DELETE FROM sessione WHERE scade_il < now();
