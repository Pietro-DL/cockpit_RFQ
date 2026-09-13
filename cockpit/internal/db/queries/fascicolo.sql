-- name: ListFascicolo :many
SELECT * FROM v_fascicolo WHERE thread_id = $1 ORDER BY padre_id NULLS FIRST, codice, tipo_documento;

-- name: GetBloccantiThread :one
SELECT * FROM v_thread_bloccanti WHERE thread_id = $1;

-- name: UpsertProposta :one
INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, componente_id, confidenza, fonte, regola_id, dettagli)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (allegato_id) DO UPDATE SET
    thread_id = COALESCE(EXCLUDED.thread_id, documento_proposta.thread_id),
    tipo_proposto = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.tipo_proposto ELSE documento_proposta.tipo_proposto END,
    codice = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.codice ELSE documento_proposta.codice END,
    rev = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.rev ELSE documento_proposta.rev END,
    confidenza = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.confidenza ELSE documento_proposta.confidenza END,
    fonte = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.fonte ELSE documento_proposta.fonte END,
    dettagli = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.dettagli ELSE documento_proposta.dettagli END
RETURNING *;

-- name: GetProposta :one
SELECT * FROM documento_proposta WHERE proposta_id = $1;

-- name: ListProposteAperteThread :many
SELECT p.*, a.nome_file, a.estensione, a.bytes, a.sha256, a.path_staging, a.messaggio_id, a.contenitore_id
FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
WHERE p.thread_id = $1 AND p.stato = 'aperta'
ORDER BY a.ricevuto_il, a.messaggio_id, a.contenitore_id NULLS FIRST, a.indice;

-- name: ListProposteBulk :many
SELECT p.* FROM documento_proposta p
WHERE p.thread_id = $1 AND p.stato = 'aperta' AND p.confidenza >= $2 AND p.tipo_proposto <> 'rumore';

-- name: DecidiProposta :execrows
UPDATE documento_proposta SET stato = $2, deciso_da = $3, deciso_il = now() WHERE proposta_id = $1 AND stato = 'aperta';

-- name: AssegnaThreadProposte :execrows
UPDATE documento_proposta p SET thread_id = $2
FROM allegato a WHERE a.allegato_id = p.allegato_id AND a.messaggio_id = $1 AND p.thread_id IS NULL;

-- name: InsertDocumento :one
INSERT INTO documento (thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, bytes, path_relativo, confermato_da, nota)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetDocumento :one
SELECT * FROM documento WHERE documento_id = $1;

-- name: GetDocumentoPerHash :one
SELECT * FROM documento WHERE thread_id = $1 AND sha256 = $2;

-- name: ListDocumentiThread :many
SELECT * FROM documento WHERE thread_id = $1 ORDER BY componente_id NULLS LAST, tipo, nome_file;

-- name: InsertProvenienza :exec
INSERT INTO documento_provenienza (documento_id, allegato_id, rif_id, messaggio_id, ricevuto_il, caricato_da)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (documento_id, ricevuto_il) DO NOTHING;

-- name: SetDocumentoScritto :exec
UPDATE documento SET stato_nas = 'scritto', scritto_il = now(), errore_nas = NULL WHERE documento_id = $1;

-- name: SetDocumentoErrore :exec
UPDATE documento SET stato_nas = 'errore', errore_nas = $2 WHERE documento_id = $1;

-- name: GetCartellaDocumento :one
SELECT * FROM cartella_documento WHERE tipo = $1;

-- name: ListCartellaDocumento :many
SELECT * FROM cartella_documento ORDER BY tipo;
