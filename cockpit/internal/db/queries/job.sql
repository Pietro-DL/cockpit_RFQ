-- name: InsertJob :one
INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, priorita, non_prima_di)
VALUES ($1, $2, $3, $4, $5, COALESCE(sqlc.narg(non_prima_di)::timestamptz, now()))
ON CONFLICT (chiave_idempotenza) WHERE stato IN ('pronto','in_corso') DO NOTHING
RETURNING *;

-- name: ClaimJob :one
UPDATE job SET stato = 'in_corso',
    lease_fino_a = now() + make_interval(secs => sqlc.arg(lease_secondi)::int),
    worker_id = sqlc.arg(worker_id), tentativi = tentativi + 1
WHERE job_id = (
    SELECT j.job_id FROM job j
    WHERE j.stato = 'pronto' AND j.worker_tipo = sqlc.arg(worker_tipo) AND j.non_prima_di <= now()
    ORDER BY j.priorita, j.job_id
    FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: HeartbeatJob :execrows
UPDATE job SET lease_fino_a = now() + make_interval(secs => sqlc.arg(lease_secondi)::int)
WHERE job_id = sqlc.arg(job_id) AND stato = 'in_corso' AND worker_id = sqlc.arg(worker_id);

-- name: GetJob :one
SELECT * FROM job WHERE job_id = $1;

-- name: CompletaJob :one
UPDATE job SET stato = 'fatto', risultato = $2, errore = NULL, lease_fino_a = NULL, chiuso_il = now()
WHERE job_id = $1 AND stato = 'in_corso'
RETURNING *;

-- name: FallisciJob :one
UPDATE job SET
    stato        = CASE WHEN tentativi >= max_tentativi OR sqlc.arg(definitivo)::boolean THEN 'fallito'::stato_job ELSE 'pronto'::stato_job END,
    non_prima_di = now() + make_interval(secs => LEAST(600, 15 * power(2, tentativi))::int),
    lease_fino_a = NULL, worker_id = NULL, errore = sqlc.arg(errore),
    chiuso_il    = CASE WHEN tentativi >= max_tentativi OR sqlc.arg(definitivo)::boolean THEN now() END
WHERE job_id = sqlc.arg(job_id) AND stato = 'in_corso'
RETURNING *;

-- name: RilasciaLeaseScaduti :execrows
UPDATE job SET
    stato = CASE WHEN tentativi >= max_tentativi THEN 'fallito'::stato_job ELSE 'pronto'::stato_job END,
    lease_fino_a = NULL, worker_id = NULL, errore = concat_ws(' ', errore, '[lease scaduto]'),
    chiuso_il = CASE WHEN tentativi >= max_tentativi THEN now() END
WHERE stato = 'in_corso' AND lease_fino_a < now();

-- name: RiaccodaJob :exec
UPDATE job SET stato = 'pronto', tentativi = 0, errore = NULL, non_prima_di = now(), chiuso_il = NULL
WHERE job_id = $1 AND stato = 'fallito';

-- name: ListJob :many
SELECT * FROM job
WHERE (sqlc.narg(stato)::stato_job IS NULL OR stato = sqlc.narg(stato)::stato_job)
ORDER BY job_id DESC LIMIT $1;

-- name: ContaJobPerStato :many
SELECT worker_tipo, stato, count(*) AS n FROM job GROUP BY worker_tipo, stato ORDER BY worker_tipo, stato;

-- name: EsisteJobPronto :one
SELECT EXISTS (SELECT 1 FROM job WHERE tipo = $1 AND stato IN ('pronto','in_corso'));

-- name: JobPendentePerChiave :one
SELECT * FROM job WHERE chiave_idempotenza = $1 AND stato IN ('pronto','in_corso') LIMIT 1;

-- name: UltimoJobPerChiavePrefisso :one
SELECT * FROM job WHERE chiave_idempotenza LIKE sqlc.arg(prefisso)::text || '%' ORDER BY job_id DESC LIMIT 1;

-- name: UpsertWorkerPresenza :exec
INSERT INTO worker_presenza (worker_tipo, worker_id, ultimo_claim, ultimo_job_il)
VALUES ($1, $2, now(), CASE WHEN sqlc.arg(con_job)::boolean THEN now() END)
ON CONFLICT (worker_tipo) DO UPDATE SET worker_id = EXCLUDED.worker_id, ultimo_claim = now(),
    ultimo_job_il = COALESCE(EXCLUDED.ultimo_job_il, worker_presenza.ultimo_job_il);

-- name: ListWorkerPresenza :many
SELECT * FROM worker_presenza ORDER BY worker_tipo;
