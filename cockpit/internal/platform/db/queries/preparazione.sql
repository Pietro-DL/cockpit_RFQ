-- B8.7b: la preparazione automatica del Fascicolo. I file utili della RFQ scendono nello staging da
-- soli, gli archivi fermi si estraggono, i file fermi si analizzano; la schermata sa quanto lavoro e'
-- ancora in corso e si aggiorna finche' ce n'e'. Nessuna tabella nuova: si leggono allegati, proposte e
-- job. Il NAS non si tocca da qui: i documenti nascono solo con la conferma.

-- name: ListAllegatiDaPreparare :many
-- Gli allegati diretti della RFQ che valgono la pena di scaricare (pre_spunta nella proposta a ingest:
-- tecnici, offerte, fogli di calcolo, archivi, PDF da determinare) e non sono ancora scesi: nessun
-- percorso in staging, stato grezzo senza il marcatore «in coda» e senza un errore. Oltre max_bytes non
-- si scaricano da soli: quel limite e' quello degli upload dei worker, e un file piu' grande fallirebbe.
SELECT a.allegato_id, a.messaggio_id, a.nome_file, a.bytes
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  JOIN documento_proposta p ON p.allegato_id = a.allegato_id
 WHERE m.thread_id = sqlc.arg(thread_id) AND m.canale = 'outlook'
   AND a.contenitore_id IS NULL AND a.natura = 'file'
   AND a.stato = 'grezzo' AND a.errore IS NULL AND a.path_staging IS NULL
   AND p.stato = 'aperta' AND coalesce((p.dettagli ->> 'pre_spunta')::boolean, false)
   AND (a.bytes IS NULL OR a.bytes <= sqlc.arg(max_bytes)::bigint)
 ORDER BY m.data_evento, a.indice;

-- name: ListAllegatiFermiInStaging :many
-- I file della RFQ scesi nello staging e mai arrivati in fondo: uno zip mai estratto, un file mai
-- analizzato. Succede quando il loro lavoro non e' mai stato accodato (l'analisi spenta quando il file e'
-- sceso) o e' fallito; chi chiama decide se riaccodare guardando l'ultimo job con la stessa chiave.
SELECT a.*
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = sqlc.arg(thread_id) AND a.natura = 'file' AND a.stato = 'in_staging'
   AND a.path_staging IS NOT NULL AND a.sha256 IS NOT NULL
 ORDER BY m.data_evento, a.contenitore_id NULLS FIRST, a.indice;

-- name: ListLavoroPendenteRfq :many
-- I lavori ancora da fare sui file della RFQ: i download (stage_allegato), le estrazioni degli archivi
-- (estrai_archivio) e le analisi, che sono per contenuto (A15) e si contano in ogni RFQ che ha quel file.
-- Solo i job pendenti: un job fallito o annullato non e' lavoro in corso, e una pagina che aspettasse
-- quello aspetterebbe per sempre. Il CASE tiene il cast fuori dalle righe degli altri tipi di job.
WITH pendenti AS (
    SELECT j.job_id, j.tipo, j.stato, j.payload FROM job j
     WHERE j.stato IN ('pronto', 'in_corso') AND j.tipo IN ('stage_allegato', 'estrai_archivio', 'analizza_allegato')
)
SELECT p.job_id, p.tipo, p.stato, a.allegato_id, a.nome_file
  FROM pendenti p
  JOIN allegato a ON a.allegato_id = CASE WHEN p.tipo <> 'analizza_allegato' THEN (p.payload ->> 'allegato_id')::uuid END
                  OR a.sha256 = CASE WHEN p.tipo = 'analizza_allegato' THEN p.payload ->> 'sha256' END
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = sqlc.arg(thread_id)
 ORDER BY p.job_id;

-- name: DecidiPropostaDocumento :execrows
-- Una decisione su un file non ancora confermato (B8.7b, «Da verificare»): il tipo, il codice, la
-- revisione. Scrive fonte = 'operatore', e da li' una lettura che arriva dopo non la riscrive
-- (UpsertProposta). Solo una proposta aperta e non ancora agganciata: il codice di una proposta agganciata
-- e' quello del suo componente (A2.2).
UPDATE documento_proposta
   SET tipo_proposto = sqlc.arg(tipo_proposto), codice = sqlc.narg(codice), rev = sqlc.narg(rev),
       fonte = 'operatore', confidenza = 100,
       dettagli = dettagli || jsonb_build_object('deciso_da_operatore', true)
 WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta' AND componente_id IS NULL;
