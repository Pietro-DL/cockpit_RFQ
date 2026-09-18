-- L'anagrafica «Altro» (7C.0, 0016): i soggetti che non sono clienti, fornitori ne' interni, e i
-- loro recapiti (un indirizzo o un dominio, lo distingue la chiocciola). Il resolver chiede una
-- cosa sola: «questo recapito e' di qualcuno?».

-- name: GetAltroPerRecapito :one
SELECT s.* FROM recapito_altro r JOIN soggetto_altro s ON s.altro_id = r.altro_id
WHERE r.recapito = lower($1) AND r.attivo AND s.attivo;

-- name: ListSoggettiAltro :many
SELECT s.*, (SELECT count(*) FROM recapito_altro r WHERE r.altro_id = s.altro_id AND r.attivo)::int AS n_recapiti
FROM soggetto_altro s ORDER BY lower(s.etichetta);

-- name: GetSoggettoAltro :one
SELECT * FROM soggetto_altro WHERE altro_id = $1;

-- name: GetSoggettoAltroPerEtichetta :one
SELECT * FROM soggetto_altro WHERE lower(etichetta) = lower($1);

-- name: InsertSoggettoAltro :one
INSERT INTO soggetto_altro (etichetta, note) VALUES ($1, $2) RETURNING *;

-- name: ListRecapitiAltro :many
SELECT * FROM recapito_altro WHERE altro_id = $1 ORDER BY recapito;

-- name: InsertRecapitoAltro :exec
INSERT INTO recapito_altro (recapito, altro_id) VALUES (lower($1), $2);

-- name: SetRecapitoAltroAttivo :execrows
UPDATE recapito_altro SET attivo = $2 WHERE recapito = lower($1);
