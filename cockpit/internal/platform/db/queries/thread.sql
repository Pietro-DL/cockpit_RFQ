-- name: InsertThread :one
INSERT INTO thread_offerta (cliente_id, buyer_id, canale, data_inizio, data_scadenza, scadenza_origine, oggetto,
                            cartella_relativa, priorita, campionatura, note, creato_da)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetThread :one
SELECT * FROM thread_offerta WHERE thread_id = $1;

-- name: GetCruscottoRiga :one
SELECT * FROM v_cruscotto WHERE thread_id = $1;

-- name: CercaThreadAperti :many
SELECT v.* FROM v_cruscotto v
WHERE v.stato_thread = 'APERTA'
  AND (sqlc.arg(testo)::text = ''
       OR v.oggetto ILIKE '%' || sqlc.arg(testo)::text || '%'
       OR v.cliente ILIKE '%' || sqlc.arg(testo)::text || '%'
       OR EXISTS (SELECT 1 FROM identificativo_thread i WHERE i.thread_id = v.thread_id AND i.codice ILIKE '%' || sqlc.arg(testo)::text || '%'))
ORDER BY COALESCE(v.ultimo_aggiornamento, v.data_inizio) DESC
LIMIT 30;

-- name: SetCartellaThread :exec
UPDATE thread_offerta SET cartella_relativa = $2 WHERE thread_id = $1;

-- name: SetCartellaCreata :exec
UPDATE thread_offerta SET cartella_creata = true WHERE thread_id = $1;

-- name: SetScadenzaThread :exec
UPDATE thread_offerta SET data_scadenza = $2, scadenza_origine = $3 WHERE thread_id = $1;

-- name: UpsertIdentificativo :one
INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (thread_id, codice) DO UPDATE SET confermato_da = COALESCE(EXCLUDED.confermato_da, identificativo_thread.confermato_da)
RETURNING *;

-- name: ListIdentificativi :many
SELECT * FROM identificativo_thread WHERE thread_id = $1 ORDER BY creato_il;

-- name: ThreadPerCodiceCliente :one
SELECT t.thread_id FROM identificativo_thread i JOIN thread_offerta t ON t.thread_id = i.thread_id
WHERE t.cliente_id = $1 AND t.stato = 'APERTA' AND t.unito_in IS NULL AND upper(i.codice) = upper($2)
ORDER BY t.data_inizio DESC LIMIT 1;

-- name: InsertComponente :one
-- Dalla 0018 l'identita' di un componente e' (thread, upper(codice)): la revisione e' un attributo e
-- le relazioni padre → figlio stanno in componente_relazione. Un secondo inserimento dello stesso
-- codice ritrova la riga che c'e'.
INSERT INTO componente (thread_id, codice, rev, descrizione, qta, tipo, origine, confermato_da)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (thread_id, upper(codice)) DO UPDATE SET descrizione = COALESCE(EXCLUDED.descrizione, componente.descrizione)
RETURNING *;

-- name: ListComponentiThread :many
SELECT * FROM componente WHERE thread_id = $1 ORDER BY codice;

-- name: GetComponente :one
SELECT * FROM componente WHERE componente_id = $1;

-- name: BloccaComponente :one
-- Il componente bloccato per la durata della transazione: chi ne corregge il codice parte da qui, e
-- due correzioni concorrenti si mettono in fila invece di scriversi sopra.
SELECT * FROM componente WHERE componente_id = $1 FOR UPDATE;

-- name: SetCodiceComponente :exec
-- Solo dentro una transazione che ha differito fk_documento_componente e fk_proposta_componente
-- (addendum A1.4, regola 5): i documenti e le proposte agganciati portano ancora il codice vecchio,
-- e si allineano nelle UPDATE che seguono, prima del COMMIT.
UPDATE componente SET codice = $2 WHERE componente_id = $1;

-- name: SetNoteComponente :exec
UPDATE componente SET note_fattibilita = $2, esito_fattibilita = $3 WHERE componente_id = $1;

-- name: SetTipoComponente :exec
UPDATE componente SET tipo = $2, confermato_da = $3 WHERE componente_id = $1;

-- name: GetFaseCorrente :one
SELECT * FROM v_thread_fase WHERE thread_id = $1;

-- name: ListFaseLog :many
SELECT * FROM fase_log WHERE thread_id = $1 ORDER BY inizio;

-- name: ChiudiFaseAperta :execrows
UPDATE fase_log SET fine = $2, esito = $3, note = COALESCE(sqlc.narg(note), note) WHERE thread_id = $1 AND fine IS NULL;

-- name: ApriFase :one
INSERT INTO fase_log (thread_id, nome_fase, responsabile_id, inizio, note) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: SetBuyerThread :exec
UPDATE thread_offerta SET buyer_id = $2 WHERE thread_id = $1 AND buyer_id IS NULL;

-- ------------------------------------------------------------------ A4 (B8.A4a): versioni della BOM e fasi
-- name: BloccaThread :one
-- La riga della RFQ bloccata per la durata della transazione: congelamento, apertura e abbandono di
-- una revisione si mettono in fila qui (A4.6, passo 1). Il trigger della working bloccata prende la
-- stessa riga FOR KEY SHARE, e quindi aspetta.
SELECT * FROM thread_offerta WHERE thread_id = $1 FOR UPDATE;

-- name: GetFaseApertaPerThread :one
SELECT * FROM fase_log WHERE thread_id = $1 AND fine IS NULL;

-- name: GetFaseLog :one
SELECT * FROM fase_log WHERE fase_log_id = $1;

-- name: ApriFaseConBom :one
INSERT INTO fase_log (thread_id, nome_fase, responsabile_id, inizio, note, bom_versione_id)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListFasiDopo :many
-- Le righe di fase_log nate dopo quella data: per l'abbandono di una revisione (D37), che torna alla
-- fase di apertura solo se nel frattempo ci sono state solo FATTIBILITA e ATTESA_DISEGNI.
SELECT f.* FROM fase_log f
WHERE f.thread_id = sqlc.arg(thread_id)
  AND f.inizio >= (SELECT x.inizio FROM fase_log x WHERE x.fase_log_id = sqlc.arg(fase_log_id))
  AND f.fase_log_id <> sqlc.arg(fase_log_id)
ORDER BY f.inizio, f.fase_log_id;

-- name: ListThreadDaRiesaminare :many
SELECT * FROM v_thread_da_riesaminare WHERE thread_id = $1 ORDER BY tipo_motivo;
