-- La struttura della BOM working che A4 aggiunge (Blocco 8, B8.A4a): archiviazione (A4.9), STEP
-- strutturale (A4.4), deroga strutturale (A4.5, D36), proposte di rimozione (A4.4, D27).
--
-- Dopo il congelamento queste scritture le rifiuta il database (trigger bom_working_modificabile,
-- D26): chi le chiama lo dice prima all'operatore, ma il muro e' li'.

-- name: ArchiviaComponente :execrows
UPDATE componente SET archiviato_il = now(), archiviato_da = sqlc.arg(archiviato_da), motivo_archiviazione = sqlc.arg(motivo)
WHERE componente_id = sqlc.arg(componente_id) AND archiviato_il IS NULL;

-- name: RipristinaComponente :execrows
UPDATE componente SET archiviato_il = NULL, archiviato_da = NULL, motivo_archiviazione = NULL
WHERE componente_id = $1 AND archiviato_il IS NOT NULL;

-- name: DeleteRelazioniComponente :execrows
-- Gli archi working del componente, in tutte e due le direzioni (A4.9). Le istantanee hanno i loro.
DELETE FROM componente_relazione WHERE padre_id = $1 OR figlio_id = $1;

-- name: DeleteComponente :execrows
-- Il rifiuto, se qualcosa lo tiene (documenti, proposte, deroghe, baseline), lo danno le FK.
DELETE FROM componente WHERE componente_id = $1;

-- name: SetStepStrutturale :execrows
UPDATE componente SET step_strutturale_id = sqlc.narg(step_strutturale_id) WHERE componente_id = sqlc.arg(componente_id);

-- name: GetStepProdotto :one
SELECT * FROM v_step_prodotto WHERE componente_id = $1;

-- name: InsertDerogaStruttura :one
INSERT INTO deroga_struttura (thread_id, componente_id, step_documento_id, step_sha256, versione_analizzatore,
                              hash_configurazione, analisi_calcolata_il, motivo_parziale, motivo, concessa_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(componente_id), sqlc.arg(step_documento_id), sqlc.arg(step_sha256), sqlc.narg(versione_analizzatore),
        sqlc.narg(hash_configurazione), sqlc.narg(analisi_calcolata_il), sqlc.arg(motivo_parziale), sqlc.arg(motivo), sqlc.arg(concessa_da))
RETURNING *;

-- name: DeleteDerogaStruttura :execrows
DELETE FROM deroga_struttura WHERE deroga_struttura_id = $1;

-- name: UpsertRimozioneProposta :exec
-- Idempotente come le altre proposte; una proposta gia' decisa non si riapre.
INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (thread_id, step_documento_id, padre_id, figlio_id) DO UPDATE SET qta_working = EXCLUDED.qta_working
WHERE rimozione_proposta.stato = 'aperta';

-- name: ListRimozioniAperte :many
SELECT * FROM rimozione_proposta WHERE thread_id = $1 AND stato = 'aperta' ORDER BY creato_il, padre_id, figlio_id;

-- name: DecidiRimozione :execrows
UPDATE rimozione_proposta SET stato = sqlc.arg(stato), deciso_da = sqlc.narg(deciso_da), deciso_il = now()
WHERE thread_id = sqlc.arg(thread_id) AND step_documento_id = sqlc.arg(step_documento_id)
  AND padre_id = sqlc.arg(padre_id) AND figlio_id = sqlc.arg(figlio_id) AND stato = 'aperta';

-- name: ChiudiProposteComponenteDiUnFile :execrows
-- Uno STEP sostituito, o un riferimento cambiato, chiude le proposte ancora aperte che venivano da li':
-- scartate, senza chi le ha decise (sono interpretazione, non decisioni), con la nota. Quelle decise restano.
UPDATE componente_proposta SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
WHERE thread_id = sqlc.arg(thread_id) AND sha256 = sqlc.arg(sha256) AND stato = 'aperta';

-- name: ChiudiProposteRelazioneDiUnFile :execrows
UPDATE relazione_proposta r SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
FROM allegato a
WHERE a.allegato_id = r.allegato_id AND a.sha256 = sqlc.arg(sha256) AND r.thread_id = sqlc.arg(thread_id) AND r.stato = 'aperta';

-- name: ChiudiRimozioniDiUnoStep :execrows
UPDATE rimozione_proposta SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
WHERE thread_id = sqlc.arg(thread_id) AND step_documento_id = sqlc.arg(step_documento_id) AND stato = 'aperta';

-- Gli archi della working scritti da una decisione (B8.5): accettare una relazione proposta, una
-- quantita' diversa, una rimozione. I cicli li rifiuta il Go prima di arrivare qui (A1.1), con la
-- riga del thread bloccata.

-- name: InsertRelazione :execrows
INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(padre_id), sqlc.arg(figlio_id), sqlc.arg(qta), sqlc.arg(origine), sqlc.arg(confermato_da))
ON CONFLICT (padre_id, figlio_id) DO NOTHING;

-- name: GetRelazione :one
SELECT * FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2;

-- name: SetQtaRelazione :execrows
UPDATE componente_relazione SET qta = sqlc.arg(qta), confermato_da = sqlc.arg(confermato_da)
WHERE padre_id = sqlc.arg(padre_id) AND figlio_id = sqlc.arg(figlio_id);

-- name: DeleteRelazione :execrows
DELETE FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2;
