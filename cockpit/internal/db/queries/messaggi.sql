-- name: UpsertConversazione :one
INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
VALUES ($1, $2, $3)
ON CONFLICT (canale, chiave_esterna) DO UPDATE
    SET primo_messaggio_il = LEAST(conversazione.primo_messaggio_il, EXCLUDED.primo_messaggio_il)
RETURNING *;

-- name: GetConversazione :one
SELECT * FROM conversazione WHERE conversazione_id = $1;

-- name: CollegaConversazione :exec
UPDATE conversazione SET thread_id = $2, collegata_da = $3 WHERE conversazione_id = $1;

-- name: UpsertMessaggio :one
INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, parent_messaggio_id, direzione, data_evento,
                       mittente_nome, mittente_indirizzo, buyer_id, destinatari, oggetto, corpo_testo, corpo_html,
                       lingua, importanza, nota_operatore, registrato_da)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (canale, chiave_esterna) DO UPDATE SET
    conversazione_id = EXCLUDED.conversazione_id,
    mittente_nome    = COALESCE(EXCLUDED.mittente_nome, messaggio.mittente_nome),
    destinatari      = EXCLUDED.destinatari,
    corpo_testo      = COALESCE(EXCLUDED.corpo_testo, messaggio.corpo_testo),
    corpo_html       = COALESCE(EXCLUDED.corpo_html, messaggio.corpo_html),
    importanza       = COALESCE(EXCLUDED.importanza, messaggio.importanza),
    buyer_id         = COALESCE(messaggio.buyer_id, EXCLUDED.buyer_id)
RETURNING *, (xmax = 0) AS inserito;

-- name: UpsertMessaggioOutlook :exec
INSERT INTO messaggio_outlook (messaggio_id, entry_id, store_id, conversation_id, conversation_index, in_reply_to,
                               riferimenti, cartella, categorie, non_letto, flag_stato)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (messaggio_id) DO UPDATE SET
    entry_id = EXCLUDED.entry_id, store_id = EXCLUDED.store_id, cartella = EXCLUDED.cartella,
    categorie = EXCLUDED.categorie, non_letto = EXCLUDED.non_letto, flag_stato = EXCLUDED.flag_stato,
    in_reply_to = COALESCE(EXCLUDED.in_reply_to, messaggio_outlook.in_reply_to),
    riferimenti = COALESCE(EXCLUDED.riferimenti, messaggio_outlook.riferimenti),
    aggiornato_il = now();

-- name: GetMessaggio :one
SELECT * FROM messaggio WHERE messaggio_id = $1;

-- name: BloccaMessaggio :one
-- Come GetMessaggio, ma la riga resta bloccata fino alla fine della transazione (voce 1.9, T13).
-- Serve a ogni percorso che DECIDE qualcosa sul messaggio: nuova RFQ, aggancio, sgancio, ignora.
-- Senza, due operatori che premono "Nuova RFQ" sullo stesso messaggio nello stesso momento leggono
-- entrambi thread_id NULL, creano due RFQ e solo una resta agganciata: l'altra rimane in giro vuota,
-- con la sua cartella sul NAS, e nessuno dei due operatori vede un errore. Con il blocco il secondo
-- aspetta, rilegge il messaggio già agganciato e riceve un esito esplicito.
SELECT * FROM messaggio WHERE messaggio_id = $1 FOR UPDATE;

-- name: GetMessaggioPerChiave :one
SELECT * FROM messaggio WHERE canale = $1 AND chiave_esterna = $2;

-- name: GetMessaggioOutlook :one
SELECT * FROM messaggio_outlook WHERE messaggio_id = $1;

-- name: GetInboxRiga :one
SELECT * FROM v_inbox WHERE messaggio_id = $1;

-- name: ListInbox :many
SELECT * FROM v_inbox
WHERE (sqlc.arg(filtro)::text = 'tutti'
    OR (sqlc.arg(filtro)::text = 'orfani'     AND thread_id IS NULL AND NOT ignorato)
    OR (sqlc.arg(filtro)::text = 'agganciati' AND thread_id IS NOT NULL)
    OR (sqlc.arg(filtro)::text = 'ignorati'   AND thread_id IS NULL AND ignorato))
ORDER BY data_evento DESC
LIMIT sqlc.arg(limite) OFFSET sqlc.arg(salta);

-- name: ContaInbox :one
SELECT count(*) FILTER (WHERE thread_id IS NULL AND NOT ignorato) AS orfani,
       count(*) FILTER (WHERE thread_id IS NOT NULL)             AS agganciati,
       count(*) FILTER (WHERE thread_id IS NULL AND ignorato)     AS ignorati,
       count(*)                                                  AS tutti
FROM v_inbox;

-- name: ListMessaggiThread :many
SELECT * FROM messaggio WHERE thread_id = $1 ORDER BY data_evento;

-- name: ListMessaggiConversazione :many
SELECT * FROM messaggio WHERE conversazione_id = $1 ORDER BY data_evento;

-- name: ThreadDellaConversazione :one
SELECT thread_id FROM messaggio
WHERE conversazione_id = $1 AND thread_id IS NOT NULL
ORDER BY data_evento DESC LIMIT 1;

-- name: AgganciaMessaggio :exec
UPDATE messaggio SET thread_id = $2, aggancio = $3, agganciato_da = $4, agganciato_il = now()
WHERE messaggio_id = $1;

-- name: SgangiaMessaggio :exec
UPDATE messaggio SET thread_id = NULL, aggancio = 'nessuno', agganciato_da = $2, agganciato_il = now()
WHERE messaggio_id = $1;

-- name: SetBuyerMessaggiPerIndirizzo :execrows
UPDATE messaggio SET buyer_id = $2 WHERE lower(mittente_indirizzo) = lower($1) AND buyer_id IS NULL;

-- name: GetSyncCursore :one
SELECT * FROM sync_cursore WHERE cartella = $1;

-- name: ListSyncCursori :many
SELECT * FROM sync_cursore ORDER BY cartella;

-- name: UpsertSyncCursore :exec
INSERT INTO sync_cursore (cartella, ultimo_received, ultimo_sync, n_messaggi, errore)
VALUES ($1, $2, now(), $3, $4)
ON CONFLICT (cartella) DO UPDATE SET
    ultimo_received = GREATEST(COALESCE(sync_cursore.ultimo_received, EXCLUDED.ultimo_received), EXCLUDED.ultimo_received),
    ultimo_sync = now(), n_messaggi = sync_cursore.n_messaggi + EXCLUDED.n_messaggi, errore = EXCLUDED.errore;

-- name: SetStoricoFinoA :exec
UPDATE sync_cursore SET storico_fino_a = LEAST(COALESCE(storico_fino_a, $2), $2) WHERE cartella = $1;

-- name: SetEntryIDMessaggio :exec
UPDATE messaggio_outlook SET entry_id = $2, store_id = $3, cartella = COALESCE(sqlc.narg(cartella), cartella), aggiornato_il = now()
WHERE messaggio_id = $1;

-- name: SetBuyerMessaggio :exec
UPDATE messaggio SET buyer_id = $2 WHERE messaggio_id = $1;

-- name: AgganciaOrfaniConversazione :many
-- quando l'operatore crea/aggancia una RFQ, gli altri messaggi orfani della stessa conversazione la seguono
UPDATE messaggio SET thread_id = $2, aggancio = 'auto_conversazione', agganciato_il = now()
WHERE conversazione_id = $1 AND thread_id IS NULL AND messaggio_id <> $3
RETURNING messaggio_id;
