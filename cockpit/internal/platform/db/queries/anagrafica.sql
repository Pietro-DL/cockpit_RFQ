-- name: ListClienti :many
SELECT * FROM cliente WHERE attivo ORDER BY cartella_nas;

-- name: GetCliente :one
SELECT * FROM cliente WHERE cliente_id = $1;

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

-- ============================================================================
--  Blocco 3 — anagrafica non distruttiva (voce 6.6) e schermata Admin → Anagrafica (8.8)
--
--  Le query qui sotto sostituiscono `UpsertCliente` e `UpsertDominioCliente`, che erano
--  distruttive: la prima, su una `cartella_nas` gia' presa, RINOMINAVA il cliente esistente
--  lasciandogli domini, buyer e RFQ; la seconda spostava un dominio da un cliente all'altro in
--  silenzio. Un INSERT che fallisce e' un errore che l'operatore legge; una UPDATE che riesce e'
--  un errore che nessuno vede (T7, T8).
-- ============================================================================

-- name: InsertCliente :one
INSERT INTO cliente (cartella_nas, ragione_sociale, profilo, lingua, portale_url, portale_note, peso, regole)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateCliente :one
UPDATE cliente SET ragione_sociale = $2, profilo = $3, lingua = $4, portale_url = $5,
       portale_note = $6, peso = $7, attivo = $8, note = $9
WHERE cliente_id = $1
RETURNING *;

-- name: SetRegoleCliente :one
UPDATE cliente SET regole = $2 WHERE cliente_id = $1 RETURNING *;

-- name: InsertDominioCliente :exec
INSERT INTO dominio_cliente (dominio, cliente_id) VALUES (lower($1), $2);

-- name: EliminaDominioCliente :exec
DELETE FROM dominio_cliente WHERE dominio = lower($1) AND cliente_id = $2;

-- name: ListClientiTutti :many
SELECT c.*,
       (SELECT count(*) FROM dominio_cliente d WHERE d.cliente_id = c.cliente_id) AS n_domini,
       (SELECT count(*) FROM buyer b WHERE b.cliente_id = c.cliente_id)           AS n_buyer,
       (SELECT count(*) FROM thread_offerta t WHERE t.cliente_id = c.cliente_id)  AS n_richieste
FROM cliente c ORDER BY c.attivo DESC, c.ragione_sociale;

-- name: GetClientePerCartella :one
SELECT * FROM cliente WHERE cartella_nas = $1;

-- Il fabbisogno che vale per un cliente, risolto ESATTAMENTE come lo risolve `v_fascicolo`: per
-- ogni tipo_componente valgono le righe del cliente se ne ha almeno una, altrimenti i default. La
-- schermata Anagrafica e il fascicolo devono rispondere la stessa cosa; due risoluzioni diverse
-- della stessa regola sono una promessa che la UI fa e il fascicolo non mantiene.
-- `proprio` dice se la riga viene dal cliente o e' ereditata dal default: in UI si leggono diverse.
-- name: ListFabbisognoEffettivo :many
SELECT f.tipo_componente, f.tipo, f.bloccante, f.fonte_attesa,
       (f.cliente_id IS NOT NULL)::boolean AS proprio
FROM fabbisogno_documento f
WHERE f.cliente_id IS NOT DISTINCT FROM (
        SELECT y.cliente_id FROM fabbisogno_documento y
        WHERE y.tipo_componente = f.tipo_componente
          AND (y.cliente_id = sqlc.narg(cliente_id)::uuid OR y.cliente_id IS NULL)
        ORDER BY (y.cliente_id IS NOT NULL) DESC LIMIT 1)
ORDER BY f.tipo_componente, f.tipo;

-- La lista delle Richieste: aperte, ordinate per peso del cliente e poi per scadenza. Il peso e'
-- un dato di anagrafica (D27), non il punteggio di priorita' dell'addendum 2: quella formula non
-- esiste ancora e qui non se ne inventa una.
-- name: ListRichieste :many
SELECT v.*, c.peso AS peso_cliente, t.riferimento_cliente
FROM v_cruscotto v
JOIN thread_offerta t ON t.thread_id = v.thread_id
JOIN cliente c        ON c.cliente_id = t.cliente_id
WHERE v.stato_thread = 'APERTA'
ORDER BY c.peso DESC, v.data_scadenza ASC NULLS LAST, COALESCE(v.ultimo_aggiornamento, v.data_inizio) DESC
LIMIT $1;

-- ---------------------------------------------------------------- amministrazione (checkpoint 3R §7)
-- Buyer e fabbisogno diventano amministrabili dalla schermata. Prima la tabella del fabbisogno si
-- vedeva ma la pagina diceva «si modificano dal database»: una tabella che sembra configurabile e non
-- lo e' e' peggio di una tabella assente.

-- name: UpdateBuyer :one
UPDATE buyer
SET cognome    = sqlc.arg(cognome),
    nome       = sqlc.narg(nome),
    email      = sqlc.narg(email),
    telefono   = sqlc.narg(telefono),
    ruolo      = sqlc.narg(ruolo),
    tipo       = sqlc.arg(tipo)::tipo_buyer,
    manda_file = sqlc.arg(manda_file),
    lingua     = sqlc.narg(lingua),
    note       = sqlc.narg(note)
WHERE buyer_id = sqlc.arg(buyer_id)
RETURNING *;

-- name: EliminaBuyer :execrows
-- Un buyer citato da un messaggio o da una richiesta NON si cancella: si perderebbe chi ha scritto.
DELETE FROM buyer b WHERE b.buyer_id = $1
  AND NOT EXISTS (SELECT 1 FROM messaggio m WHERE m.buyer_id = b.buyer_id)
  AND NOT EXISTS (SELECT 1 FROM thread_offerta t WHERE t.buyer_id = b.buyer_id);

-- name: InsertFabbisogno :one
INSERT INTO fabbisogno_documento (cliente_id, tipo_componente, tipo, bloccante)
VALUES (sqlc.arg(cliente_id), sqlc.arg(tipo_componente)::tipo_componente, sqlc.arg(tipo)::tipo_documento, sqlc.arg(bloccante))
ON CONFLICT (cliente_id, tipo_componente, tipo) DO UPDATE SET bloccante = EXCLUDED.bloccante
RETURNING *;

-- name: EliminaFabbisogno :execrows
-- Solo le righe DI QUESTO cliente: i default (cliente_id NULL) valgono per tutti e non si tolgono da qui.
DELETE FROM fabbisogno_documento WHERE fabbisogno_id = $1 AND cliente_id = $2;

-- name: ListFabbisognoCliente :many
SELECT * FROM fabbisogno_documento WHERE cliente_id = $1 ORDER BY tipo_componente, tipo;

-- name: SetFonteFabbisogno :exec
UPDATE fabbisogno_documento SET fonte_attesa = sqlc.narg(fonte) WHERE fabbisogno_id = $1 AND cliente_id = $2;
