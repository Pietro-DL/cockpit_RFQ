-- Coda dei job.
--
-- Il PREDICATO DI VALIDITÀ DEL TENTATIVO (piano §2.3) compare identico in HeartbeatJob, CompletaJob,
-- FallisciJob e BloccaTentativo:
--
--     stato = 'in_corso' AND lease_token = $token AND worker_id = $worker
--     AND lease_fino_a > now() AND now() <= avviato_il + durata_max_s
--
-- Zero righe significa «questo tentativo non vale più» e il chiamante risponde 409 senza applicare
-- nulla. Non va rilassato in nessuno dei quattro punti: basta un punto scoperto perché il risultato
-- di un tentativo scaduto si applichi al lavoro di quello nuovo.

-- name: InsertJob :one
INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, priorita, non_prima_di,
                 lease_s, durata_max_s, casella_id, postazione_id, richiesto_da, scade_il)
VALUES ($1, $2, $3, $4, $5, COALESCE(sqlc.narg(non_prima_di)::timestamptz, now()),
        sqlc.arg(lease_s), sqlc.arg(durata_max_s), sqlc.narg(casella_id), sqlc.narg(postazione_id),
        sqlc.narg(richiesto_da), sqlc.narg(scade_il))
ON CONFLICT (chiave_idempotenza) WHERE stato IN ('pronto','in_corso') DO NOTHING
RETURNING *;

-- name: ClaimJob :one
-- Un solo UPDATE: il tentativo nasce qui con un token nuovo e con avviato_il, che fissa l'inizio da
-- cui si misura durata_max_s. Un job interattivo già scaduto non viene assegnato a nessuno.
UPDATE job SET stato = 'in_corso',
    lease_token  = gen_random_uuid(),
    avviato_il   = now(),
    lease_fino_a = now() + make_interval(secs => lease_s),
    worker_id    = sqlc.arg(worker_id),
    tentativi    = tentativi + 1
WHERE job_id = (
    SELECT j.job_id FROM job j
    WHERE j.stato = 'pronto' AND j.worker_tipo = sqlc.arg(worker_tipo) AND j.non_prima_di <= now()
      AND (j.scade_il IS NULL OR j.scade_il > now())
    ORDER BY j.priorita, j.job_id
    FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING *;

-- name: HeartbeatJob :execrows
UPDATE job SET lease_fino_a = now() + make_interval(secs => lease_s)
WHERE job_id = sqlc.arg(job_id)
  AND stato = 'in_corso' AND lease_token = sqlc.arg(lease_token) AND worker_id = sqlc.arg(worker_id)
  AND lease_fino_a > now() AND now() <= avviato_il + make_interval(secs => durata_max_s);

-- name: BloccaTentativo :one
-- Usata dall'ingest: blocca la riga del job per tutta la transazione del lotto, così un tentativo
-- concorrente non può diventare valido a metà scrittura.
SELECT * FROM job
WHERE job_id = sqlc.arg(job_id)
  AND stato = 'in_corso' AND lease_token = sqlc.arg(lease_token) AND worker_id = sqlc.arg(worker_id)
  AND lease_fino_a > now() AND now() <= avviato_il + make_interval(secs => durata_max_s)
FOR UPDATE;

-- name: GetJob :one
SELECT * FROM job WHERE job_id = $1;

-- name: CompletaJob :one
UPDATE job SET stato = 'fatto', risultato = sqlc.arg(risultato), errore = NULL,
    lease_fino_a = NULL, lease_token = NULL, chiuso_il = now()
WHERE job_id = sqlc.arg(job_id)
  AND stato = 'in_corso' AND lease_token = sqlc.arg(lease_token) AND worker_id = sqlc.arg(worker_id)
  AND lease_fino_a > now() AND now() <= avviato_il + make_interval(secs => durata_max_s)
RETURNING *;

-- name: FallisciJob :one
UPDATE job SET
    stato        = CASE WHEN tentativi >= max_tentativi OR sqlc.arg(definitivo)::boolean THEN 'fallito'::stato_job ELSE 'pronto'::stato_job END,
    non_prima_di = now() + make_interval(secs => LEAST(600, 15 * power(2, tentativi))::int),
    lease_fino_a = NULL, lease_token = NULL, worker_id = NULL, errore = sqlc.arg(errore),
    chiuso_il    = CASE WHEN tentativi >= max_tentativi OR sqlc.arg(definitivo)::boolean THEN now() END
WHERE job_id = sqlc.arg(job_id)
  AND stato = 'in_corso' AND lease_token = sqlc.arg(lease_token) AND worker_id = sqlc.arg(worker_id)
  AND lease_fino_a > now() AND now() <= avviato_il + make_interval(secs => durata_max_s)
RETURNING *;

-- name: ScadutoPerDurataMassima :execrows
-- Chiamata quando il predicato fallisce: se il motivo è la durata massima il job torna disponibile
-- con il motivo scritto, invece di restare appeso fino alla scadenza del lease.
UPDATE job SET stato = 'pronto', lease_fino_a = NULL, lease_token = NULL, worker_id = NULL,
    non_prima_di = now(), errore = 'durata massima superata'
WHERE job_id = sqlc.arg(job_id) AND stato = 'in_corso' AND avviato_il IS NOT NULL
  AND now() > avviato_il + make_interval(secs => durata_max_s);

-- name: RilasciaLeaseScaduti :execrows
UPDATE job SET
    stato = CASE WHEN tentativi >= max_tentativi THEN 'fallito'::stato_job ELSE 'pronto'::stato_job END,
    lease_fino_a = NULL, lease_token = NULL, worker_id = NULL,
    errore = concat_ws(' ', errore, CASE WHEN avviato_il IS NOT NULL AND now() > avviato_il + make_interval(secs => durata_max_s)
                                         THEN '[durata massima superata]' ELSE '[lease scaduto]' END),
    chiuso_il = CASE WHEN tentativi >= max_tentativi THEN now() END
WHERE stato = 'in_corso'
  AND (lease_fino_a < now() OR (avviato_il IS NOT NULL AND now() > avviato_il + make_interval(secs => durata_max_s)));

-- name: AnnullaJobScaduti :execrows
-- Job interattivi che non servono più: nessuno li eseguirà, e restare 'pronto' li farebbe apparire
-- come lavoro arretrato.
UPDATE job SET stato = 'annullato'::stato_job, lease_fino_a = NULL, lease_token = NULL, worker_id = NULL,
    chiuso_il = now(), errore = concat_ws(' ', errore, '[scaduto prima di essere eseguito]')
WHERE scade_il IS NOT NULL AND scade_il <= now() AND stato IN ('pronto','in_corso');

-- name: RiaccodaJob :one
-- Zero righe = non riaccodabile: o non è chiuso, o esiste già un job pendente con la stessa chiave di
-- idempotenza. Il secondo caso violerebbe l'indice unico parziale: qui diventa un avviso, non un 500.
UPDATE job SET stato = 'pronto', tentativi = 0, errore = NULL, non_prima_di = now(), chiuso_il = NULL,
    lease_fino_a = NULL, lease_token = NULL, worker_id = NULL
WHERE job.job_id = sqlc.arg(job_id) AND job.stato IN ('fallito','annullato')
  AND NOT EXISTS (
      SELECT 1 FROM job p
      WHERE p.chiave_idempotenza IS NOT NULL AND p.chiave_idempotenza = job.chiave_idempotenza
        AND p.stato IN ('pronto','in_corso'))
RETURNING *;

-- name: EliminaJobVecchi :execrows
DELETE FROM job
WHERE stato IN ('fatto','fallito','annullato') AND chiuso_il IS NOT NULL
  AND chiuso_il < now() - make_interval(days => sqlc.arg(giorni)::int);

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
