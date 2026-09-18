-- name: UpsertRiferimentoPortale :one
INSERT INTO riferimento_portale (messaggio_id, thread_id, codice, tipo_atteso, url, testo_citato)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (messaggio_id, codice, tipo_atteso) DO UPDATE SET thread_id = COALESCE(EXCLUDED.thread_id, riferimento_portale.thread_id)
RETURNING *;

-- name: ListRiferimentiPortaleThread :many
SELECT * FROM riferimento_portale WHERE thread_id = $1 ORDER BY creato_il;

-- name: SetRiferimentoPortaleStato :exec
UPDATE riferimento_portale SET stato = $2, scaricato_da = $3, scaricato_il = now() WHERE rif_id = $1;

-- name: AssegnaThreadRiferimenti :execrows
UPDATE riferimento_portale SET thread_id = $2 WHERE messaggio_id = $1 AND thread_id IS NULL;

-- name: InsertDeroga :one
INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (componente_id, tipo) DO UPDATE SET motivo = EXCLUDED.motivo, utente_id = EXCLUDED.utente_id, creata_il = now()
RETURNING *;

-- name: ListFabbisogno :many
SELECT * FROM fabbisogno_documento WHERE cliente_id IS NULL OR cliente_id = $1 ORDER BY cliente_id NULLS FIRST, tipo_componente, tipo;

-- name: UpsertTriage :one
-- Dalla 0016 porta l'ATTO (che cosa sta facendo il mittente) e il LEGAME (nuovo, risposta,
-- aggiornamento...), tutti e due proposte; e per la posta dei fornitori il bersaglio: una richiesta
-- esistente o «richiesta a X per la RFQ Y» (fornitore_proposto + thread_proposto). L'esito resta
-- la proposta di azione per la schermata.
INSERT INTO proposta_triage (messaggio_id, esito, thread_proposto, cliente_proposto, buyer_proposto, identificativi,
                             scadenza_proposta, confidenza, motivi, fonte, atto, legame, richiesta_proposta, fornitore_proposto)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (messaggio_id, fonte) DO UPDATE SET
    esito = EXCLUDED.esito, thread_proposto = EXCLUDED.thread_proposto, cliente_proposto = EXCLUDED.cliente_proposto,
    buyer_proposto = EXCLUDED.buyer_proposto, identificativi = EXCLUDED.identificativi, scadenza_proposta = EXCLUDED.scadenza_proposta,
    confidenza = EXCLUDED.confidenza, motivi = EXCLUDED.motivi,
    atto = EXCLUDED.atto, legame = EXCLUDED.legame, richiesta_proposta = EXCLUDED.richiesta_proposta, fornitore_proposto = EXCLUDED.fornitore_proposto
WHERE proposta_triage.stato = 'proposta'
RETURNING *;

-- name: GetTriageMessaggio :one
-- La proposta DETERMINISTICA. Prima sceglieva quella dell'agente quando c'era: una preferenza
-- implicita che non spetta a una query di lettura (7C.0). Il confronto fra le due fonti e' una
-- lettura a parte, quando servira'.
SELECT * FROM proposta_triage WHERE messaggio_id = $1 AND fonte = 'deterministico';

-- name: IgnoraMessaggio :exec
-- "Ignora" dall'Inbox: chiude la proposta se c'è, altrimenti registra la decisione come proposta rifiutata
INSERT INTO proposta_triage (messaggio_id, esito, confidenza, motivi, fonte, stato, deciso_da, deciso_il)
VALUES ($1, 'ignora', 100, '["ignorato dall''operatore"]', 'deterministico', 'rifiutata', $2, now())
ON CONFLICT (messaggio_id, fonte) DO UPDATE SET stato = 'rifiutata', deciso_da = EXCLUDED.deciso_da, deciso_il = now();

-- name: DecidiTriage :execrows
UPDATE proposta_triage SET stato = $2, deciso_da = $3, deciso_il = now() WHERE messaggio_id = $1 AND stato = 'proposta';

-- name: InsertBozza :one
INSERT INTO bozza (thread_id, in_risposta_a, tipo, destinatari, oggetto, corpo, documenti, creata_da)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetBozza :one
SELECT * FROM bozza WHERE bozza_id = $1;

-- name: SetBozzaAperta :exec
UPDATE bozza SET stato = 'aperta', entry_id = $2, errore = NULL WHERE bozza_id = $1;

-- name: SetBozzaErrore :exec
UPDATE bozza SET stato = 'errore', errore = $2 WHERE bozza_id = $1;

-- name: ListBozzeThread :many
SELECT * FROM bozza WHERE thread_id = $1 ORDER BY creata_il DESC;
