-- La cache dei contenuti (Pre-7, D31).
--
-- `_contenuti` non e' una coda che si svuota quando la copia sul NAS e' andata a buon fine: e' una
-- CACHE indirizzata dal contenuto, e serve a tre cose dopo quella copia — non riscaricare da Outlook
-- lo stesso file ricevuto un'altra volta, rianalizzarlo quando cambia il dizionario, ricopiarlo sul
-- NAS se un giorno il file sparisce. Quindi un contenuto si toglie solo quando NESSUNO ne ha ancora
-- bisogno adesso (i pin qui sotto) E da tanto non lo tocca nessuno (retention), oppure quando la
-- cache ha superato la capienza che le e' stata data.

-- name: ListContenutiPinnati :many
-- Quali di questi contenuti NON si possono togliere, e perche'. Un motivo per contenuto: il primo
-- nell'ordine qui sotto, che e' l'ordine in cui e' grave perderlo.
--
--   1. un documento confermato che aspetta ancora la copia (`in_coda`, `errore`): togliere il file
--      adesso vuol dire far fallire la copia che sta per partire;
--   2. un'anomalia NAS aperta su un documento con quell'hash: il riconciliatore potrebbe doverlo
--      ricopiare, ed e' proprio il caso per cui la cache esiste;
--   3. una proposta ancora aperta: l'operatore non ha ancora deciso, e quando decide il file deve
--      esserci;
--   4. un job pendente che lo cita — per hash, per allegato o per documento — qualunque sia il tipo.
--
-- I payload dei job si leggono come testo e non si castano: un payload che non porta quella chiave
-- da' NULL, e NULL non e' uguale a niente. Un cast a uuid su una chiave assente farebbe fallire la
-- query intera, cioe' la pulizia di tutto.
WITH s AS (SELECT DISTINCT unnest(@sha::char(64)[]) AS sha256),
pin AS (
    SELECT s.sha256, 'documento in attesa di copia'::text AS motivo, 1 AS ordine FROM s
     WHERE EXISTS (SELECT 1 FROM documento d WHERE d.sha256 = s.sha256 AND d.stato_nas IN ('in_coda','errore'))
    UNION ALL
    SELECT s.sha256, 'anomalia NAS aperta', 2 FROM s
     WHERE EXISTS (SELECT 1 FROM nas_anomalia n JOIN documento d ON d.documento_id = n.documento_id
                    WHERE d.sha256 = s.sha256 AND n.risolta_il IS NULL)
    UNION ALL
    SELECT s.sha256, 'proposta ancora aperta', 3 FROM s
     WHERE EXISTS (SELECT 1 FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
                    WHERE a.sha256 = s.sha256 AND p.stato = 'aperta')
    UNION ALL
    SELECT s.sha256, 'job pendente', 4 FROM s
     WHERE EXISTS (SELECT 1 FROM job j WHERE j.stato IN ('pronto','in_corso') AND (
               j.payload->>'sha256' = s.sha256::text
            OR EXISTS (SELECT 1 FROM allegato a WHERE a.allegato_id::text = j.payload->>'allegato_id' AND a.sha256 = s.sha256)
            OR EXISTS (SELECT 1 FROM documento d WHERE d.documento_id::text = j.payload->>'documento_id' AND d.sha256 = s.sha256)))
)
SELECT DISTINCT ON (pin.sha256) pin.sha256::char(64) AS sha256, pin.motivo FROM pin ORDER BY pin.sha256, pin.ordine;

-- name: ListVociDiArchivio :many
-- Per ogni archivio fra questi contenuti, gli hash delle voci estratte da lui. Un archivio non si
-- toglie finche' una delle sue voci e' pinnata: se la voce sparisse, si riestrarrebbe da lui senza
-- tornare in Outlook — ed e' la riparazione a buon mercato che questa dipendenza conserva.
SELECT DISTINCT z.sha256::char(64) AS archivio, f.sha256::char(64) AS voce
FROM allegato z JOIN allegato f ON f.contenitore_id = z.allegato_id
WHERE z.sha256 = ANY(@sha::char(64)[]) AND f.sha256 IS NOT NULL;

-- name: UltimoUsoContenuti :many
-- L'ultima volta che il DATABASE ha avuto a che fare con questi contenuti: l'allegato piu' recente
-- che li porta, la conferma o la scrittura piu' recente di un documento con quell'hash. Il disco ha
-- la sua data (l'orario del file, che la copia sul NAS rinfresca); vale la piu' recente delle due.
SELECT u.sha256::char(64) AS sha256, max(u.quando)::timestamptz AS ultimo_uso FROM (
    SELECT a.sha256, a.ricevuto_il AS quando FROM allegato a WHERE a.sha256 = ANY(@sha::char(64)[])
    UNION ALL
    SELECT d.sha256, d.confermato_il FROM documento d WHERE d.sha256 = ANY(@sha::char(64)[])
    UNION ALL
    SELECT d.sha256, d.scritto_il FROM documento d WHERE d.sha256 = ANY(@sha::char(64)[]) AND d.scritto_il IS NOT NULL
) u GROUP BY u.sha256;

-- name: AzzeraPathStagingPerSha :execrows
-- Il contenuto non c'e' piu': gli allegati che lo nominavano non hanno piu' un percorso. Lo stato
-- resta com'e' — il file e' ricostruibile, da Outlook o dall'archivio — e chi ne ha bisogno lo
-- riscarica o lo riestrae.
UPDATE allegato SET path_staging = NULL WHERE sha256 = $1 AND path_staging IS NOT NULL;

-- name: UltimoJobPerChiave :one
-- L'ultimo job, in qualunque stato, accodato con questa chiave. Serve a chi sta per riaccodare un
-- lavoro per sapere se lo stesso lavoro e' gia' stato provato e com'e' finito.
SELECT * FROM job WHERE chiave_idempotenza = $1 ORDER BY job_id DESC LIMIT 1;
