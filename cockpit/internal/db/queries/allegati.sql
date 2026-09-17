-- name: UpsertAllegato :one
INSERT INTO allegato (messaggio_id, contenitore_id, indice, nome_file, path_interno, estensione, content_type,
                      natura, origine, bytes, sha256, ricevuto_il, caricato_da)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (messaggio_id, contenitore_id, indice) DO UPDATE SET
    nome_file = EXCLUDED.nome_file, bytes = COALESCE(EXCLUDED.bytes, allegato.bytes),
    sha256 = COALESCE(EXCLUDED.sha256, allegato.sha256), content_type = COALESCE(EXCLUDED.content_type, allegato.content_type)
RETURNING *;

-- name: GetAllegato :one
SELECT * FROM allegato WHERE allegato_id = $1;

-- name: ListAllegatiMessaggio :many
SELECT * FROM allegato WHERE messaggio_id = $1 ORDER BY contenitore_id NULLS FIRST, indice;

-- name: ListAllegatiThread :many
SELECT a.* FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
WHERE m.thread_id = $1 ORDER BY m.data_evento, a.indice;

-- name: SetAllegatoStaging :exec
UPDATE allegato SET path_staging = $2, sha256 = $3, bytes = $4, stato = 'in_staging', errore = NULL
WHERE allegato_id = $1;

-- name: ShaAncoraUsati :many
-- Quali di questi contenuti servono ancora a qualcuno (blocco 4A, pulizia dello staging).
--
-- La domanda si fa sull'HASH e non sul percorso. Il nome di un contenuto in staging E' il suo
-- sha256, quindi l'hash basta; il percorso invece e' una stringa che su Windows puo' differire per
-- maiuscole o per separatori senza indicare un file diverso, e una pulizia che sbaglia il confronto
-- cancella il disegno che stava per essere copiato sul NAS.
--
-- Basta che UN allegato porti quell'hash: non si guarda il suo path_staging. Un allegato che ha
-- l'hash ma non il percorso e' uno che quel contenuto lo riotterrebbe senza riscaricarlo, ed e'
-- esattamente il caso che la deduplica serve a rendere gratuito.
SELECT DISTINCT sha256 FROM allegato WHERE sha256 = ANY(@sha::char(64)[]);

-- name: AllegatoInStagingPerHash :one
-- Un ALTRO allegato con lo stesso contenuto già sceso in staging (voce 1.11). Lo stesso disegno
-- allegato a tre richieste diverse è lo stesso file: scaricarlo tre volte significa tre giri in COM su
-- Outlook e tre copie identiche sul disco. Si esclude l'allegato di partenza, altrimenti troverebbe
-- se stesso e non risponderebbe mai alla domanda che gli viene fatta.
SELECT * FROM allegato
WHERE sha256 = sqlc.arg(sha256) AND path_staging IS NOT NULL AND path_staging <> ''
  AND allegato_id <> sqlc.arg(escluso)
ORDER BY (stato = 'in_staging') DESC, ricevuto_il
LIMIT 1;

-- name: SetAllegatoStato :exec
UPDATE allegato SET stato = $2, errore = $3 WHERE allegato_id = $1;

-- name: ListAllegatiSenzaStaging :many
SELECT a.* FROM allegato a
WHERE a.contenitore_id IS NULL AND a.natura IN ('file','elemento_outlook') AND a.stato = 'grezzo'
  AND a.origine = 'outlook'
ORDER BY a.ricevuto_il LIMIT $1;

-- name: ContaHashVisto :one
SELECT count(DISTINCT a.messaggio_id) FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
WHERE a.sha256 = $1 AND lower(split_part(m.mittente_indirizzo, '@', 2)) = lower($2);

-- name: UpsertHashRumore :exec
INSERT INTO hash_rumore (sha256, dominio, scartato_da) VALUES ($1, lower($2), $3)
ON CONFLICT (sha256, dominio) DO UPDATE SET n_visto = hash_rumore.n_visto + 1, ultimo_il = now(),
    scartato_da = COALESCE(EXCLUDED.scartato_da, hash_rumore.scartato_da);

-- name: IsHashRumore :one
SELECT EXISTS (SELECT 1 FROM hash_rumore WHERE sha256 = $1 AND dominio = lower($2));
