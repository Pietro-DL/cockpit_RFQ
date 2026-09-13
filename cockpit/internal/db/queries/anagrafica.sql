-- name: ListClienti :many
SELECT * FROM cliente WHERE attivo ORDER BY cartella_nas;

-- name: GetCliente :one
SELECT * FROM cliente WHERE cliente_id = $1;

-- name: UpsertCliente :one
INSERT INTO cliente (cartella_nas, ragione_sociale, profilo, lingua, portale_url, portale_note, regole)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (cartella_nas) DO UPDATE SET ragione_sociale = EXCLUDED.ragione_sociale, profilo = EXCLUDED.profilo,
    lingua = EXCLUDED.lingua, portale_url = EXCLUDED.portale_url, portale_note = EXCLUDED.portale_note, regole = EXCLUDED.regole
RETURNING *;

-- name: UpsertDominioCliente :exec
INSERT INTO dominio_cliente (dominio, cliente_id) VALUES (lower($1), $2)
ON CONFLICT (dominio) DO UPDATE SET cliente_id = EXCLUDED.cliente_id;

-- name: GetClientePerDominio :one
SELECT c.* FROM dominio_cliente d JOIN cliente c ON c.cliente_id = d.cliente_id WHERE d.dominio = lower($1);

-- name: ListDominiCliente :many
SELECT * FROM dominio_cliente WHERE cliente_id = $1 ORDER BY dominio;

-- name: GetBuyerPerEmail :one
SELECT * FROM buyer WHERE email = lower($1);

-- name: GetBuyer :one
SELECT * FROM buyer WHERE buyer_id = $1;

-- name: ListBuyerCliente :many
SELECT * FROM buyer WHERE cliente_id = $1 ORDER BY cognome, nome;

-- name: InsertBuyer :one
INSERT INTO buyer (cliente_id, cognome, nome, email, telefono, ruolo, tipo, lingua, origine, note)
VALUES ($1, $2, $3, lower(sqlc.narg(email)), $4, $5, $6, $7, $8, $9)
ON CONFLICT (cliente_id, cognome, nome) DO UPDATE SET email = COALESCE(EXCLUDED.email, buyer.email),
    telefono = COALESCE(EXCLUDED.telefono, buyer.telefono)
RETURNING *;

-- name: ListFaseCatalogo :many
SELECT * FROM fase_catalogo ORDER BY ordine;

-- name: ListTransizioniDa :many
SELECT * FROM transizione WHERE da = $1;

-- name: ListRegole :many
SELECT * FROM regola ORDER BY regola_id;

-- name: IncrementaRegola :exec
UPDATE regola SET applicazioni = applicazioni + 1, correzioni_umane = correzioni_umane + sqlc.arg(correzione)::int
WHERE regola_id = sqlc.arg(regola_id);
