-- L'editor della struttura (Fascicolo v3): quello che le query di sempre non dicevano.

-- name: TieniArcoDellaRimozione :execrows
-- Lo STEP strutturale proponeva di togliere un arco che l'operatore ha appena confermato nell'editor: la
-- rimozione si chiude, e una rimozione scartata non si ripropone (UpsertRimozioneProposta riscrive solo le
-- righe aperte).
UPDATE rimozione_proposta SET stato = 'scartata', nota = 'tenuto nella struttura confermata', deciso_da = sqlc.narg(deciso_da), deciso_il = now()
WHERE thread_id = sqlc.arg(thread_id) AND step_documento_id = sqlc.arg(step_documento_id)
  AND padre_id = sqlc.arg(padre_id) AND figlio_id = sqlc.arg(figlio_id) AND stato = 'aperta';
