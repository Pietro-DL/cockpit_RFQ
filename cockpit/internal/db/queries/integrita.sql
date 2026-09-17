-- Integrita' NAS (blocco 5B): il ricognitore confronta il database con i file veri.

-- name: ListDocumentiDaVerificare :many
-- I documenti guardati meno di recente per primi: la passata e' limitata, e a giro si arriva a tutti.
-- Non si filtra per stato: un documento «scritto» va controllato proprio perche' dice di esserlo.
SELECT sqlc.embed(d), t.cartella_relativa
FROM documento d JOIN thread_offerta t ON t.thread_id = d.thread_id
ORDER BY d.verificato_il NULLS FIRST, d.documento_id
LIMIT $1;

-- name: SetDocumentoVerificato :exec
UPDATE documento SET verificato_il = now() WHERE documento_id = $1;

-- name: ListCopieNasPendenti :many
-- Le chiavi delle copie gia' in coda. Un documento la cui copia deve ancora partire non e'
-- un'anomalia: e' lavoro in corso, e metterlo in elenco vorrebbe dire chiedere a una persona di
-- occuparsi di qualcosa che il sistema sta gia' facendo.
SELECT chiave_idempotenza FROM job
WHERE tipo = 'copia_nas' AND stato IN ('pronto','in_corso') AND chiave_idempotenza IS NOT NULL;

-- name: ApriAnomaliaNas :one
INSERT INTO nas_anomalia (documento_id, thread_id, problema, stato_db, percorso, sha_atteso, sha_trovato, dettaglio)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (documento_id) WHERE risolta_il IS NULL DO UPDATE SET
    problema = EXCLUDED.problema, stato_db = EXCLUDED.stato_db, percorso = EXCLUDED.percorso,
    sha_atteso = EXCLUDED.sha_atteso, sha_trovato = EXCLUDED.sha_trovato,
    dettaglio = EXCLUDED.dettaglio, vista_il = now()
RETURNING *;

-- name: ChiudiAnomaliaNas :execrows
UPDATE nas_anomalia SET risolta_il = now() WHERE documento_id = $1 AND risolta_il IS NULL;

-- name: GetAnomaliaNas :one
SELECT * FROM nas_anomalia WHERE documento_id = $1 AND risolta_il IS NULL;

-- name: ListAnomalieNas :many
SELECT sqlc.embed(a), d.nome_file, d.codice, d.rev, d.tipo, d.verificato_il,
       t.oggetto, c.ragione_sociale
FROM nas_anomalia a
JOIN documento d ON d.documento_id = a.documento_id
JOIN thread_offerta t ON t.thread_id = a.thread_id
JOIN cliente c ON c.cliente_id = t.cliente_id
WHERE a.risolta_il IS NULL
ORDER BY a.problema, a.rilevata_il;

-- name: ContaAnomalieNas :many
SELECT problema, count(*)::int AS n FROM nas_anomalia WHERE risolta_il IS NULL GROUP BY problema ORDER BY problema;

-- name: ContaAnomalieThread :one
SELECT count(*)::int FROM nas_anomalia WHERE thread_id = $1 AND risolta_il IS NULL;

-- name: StatoIntegritaNas :one
SELECT count(*)::int AS documenti,
       count(*) FILTER (WHERE verificato_il IS NOT NULL)::int AS verificati
FROM documento;

-- name: UltimoControlloNas :one
-- Nessuna riga = il ricognitore non ha ancora guardato niente. E' una risposta, non un errore: una
-- schermata che non segnala nulla perche' non ha mai controllato non e' una schermata tranquilla.
SELECT verificato_il::timestamptz FROM documento
WHERE verificato_il IS NOT NULL ORDER BY verificato_il DESC LIMIT 1;
