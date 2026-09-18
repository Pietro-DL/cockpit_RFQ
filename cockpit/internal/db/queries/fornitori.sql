-- Blocco 7A: fornitori, contatti, lavorazioni, qualifiche, convenzioni di codice e la controparte
-- del messaggio (0014). Nessuna di queste query scrive `messaggio.thread_id` ne' una proposta:
-- la controparte e' un FATTO sul messaggio, il triage resta un suggerimento.

-- ---------------------------------------------------------------- fornitore

-- name: ListFornitori :many
SELECT f.*,
       (SELECT count(*) FROM dominio_fornitore d WHERE d.fornitore_id = f.fornitore_id)  AS n_domini,
       (SELECT count(*) FROM contatto_fornitore c WHERE c.fornitore_id = f.fornitore_id) AS n_contatti,
       (SELECT count(*) FROM fornitore_lavorazione l WHERE l.fornitore_id = f.fornitore_id) AS n_lavorazioni,
       (SELECT count(*) FROM messaggio m WHERE m.controparte_fornitore_id = f.fornitore_id) AS n_messaggi
FROM fornitore f ORDER BY f.attivo DESC, f.ragione_sociale;

-- name: ListFornitoriAttivi :many
SELECT * FROM fornitore WHERE attivo ORDER BY ragione_sociale;

-- name: GetFornitore :one
SELECT * FROM fornitore WHERE fornitore_id = $1;

-- name: GetFornitorePerRagioneSociale :one
SELECT * FROM fornitore WHERE lower(ragione_sociale) = lower($1);

-- name: InsertFornitore :one
INSERT INTO fornitore (ragione_sociale, tipo, lingua, note)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdateFornitore :one
UPDATE fornitore SET ragione_sociale = $2, tipo = $3, lingua = $4, note = $5, attivo = $6
WHERE fornitore_id = $1 RETURNING *;

-- ---------------------------------------------------------------- domini e contatti

-- name: ListDominiFornitore :many
SELECT * FROM dominio_fornitore WHERE fornitore_id = $1 ORDER BY dominio;

-- name: GetFornitorePerDominio :one
SELECT f.* FROM dominio_fornitore d JOIN fornitore f ON f.fornitore_id = d.fornitore_id WHERE d.dominio = lower($1);

-- name: InsertDominioFornitore :exec
INSERT INTO dominio_fornitore (dominio, fornitore_id) VALUES (lower($1), $2);

-- name: EliminaDominioFornitore :execrows
DELETE FROM dominio_fornitore WHERE dominio = lower($1) AND fornitore_id = $2;

-- name: ListContattiFornitore :many
SELECT * FROM contatto_fornitore WHERE fornitore_id = $1 ORDER BY email;

-- ListFornitoriPerContatto: TUTTI i fornitori che hanno questa email fra i contatti. La chiave
-- unica e' (fornitore_id, email), quindi possono essere due: il resolver risponde `ambiguo`.
-- name: ListFornitoriPerContatto :many
SELECT f.* FROM contatto_fornitore c JOIN fornitore f ON f.fornitore_id = c.fornitore_id
WHERE c.email = lower($1) ORDER BY f.ragione_sociale;

-- name: InsertContattoFornitore :one
INSERT INTO contatto_fornitore (fornitore_id, nome, email, ruolo, lingua, note)
VALUES ($1, $2, lower($3), $4, $5, $6) RETURNING *;

-- name: EliminaContattoFornitore :execrows
DELETE FROM contatto_fornitore WHERE contatto_id = $1 AND fornitore_id = $2;

-- ---------------------------------------------------------------- lavorazioni

-- name: ListLavorazioni :many
SELECT * FROM lavorazione ORDER BY descrizione;

-- name: ListLavorazioniFornitore :many
SELECT l.* FROM fornitore_lavorazione fl JOIN lavorazione l ON l.codice = fl.lavorazione
WHERE fl.fornitore_id = $1 ORDER BY l.descrizione;

-- name: InsertLavorazioneFornitore :execrows
INSERT INTO fornitore_lavorazione (fornitore_id, lavorazione) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: EliminaLavorazioneFornitore :execrows
-- Fallisce con la chiave esterna se una qualifica dipende da questa capacita': e' voluto. Prima si
-- toglie la qualifica, poi la capacita'.
DELETE FROM fornitore_lavorazione WHERE fornitore_id = $1 AND lavorazione = $2;

-- ---------------------------------------------------------------- qualifiche: cliente x fornitore x lavorazione

-- name: ListQualificheFornitore :many
SELECT q.*, c.cartella_nas, c.ragione_sociale AS cliente, l.descrizione AS lavorazione_descrizione
FROM cliente_fornitore_lavorazione q
JOIN cliente c ON c.cliente_id = q.cliente_id
JOIN lavorazione l ON l.codice = q.lavorazione
WHERE q.fornitore_id = $1 ORDER BY c.cartella_nas, l.descrizione;

-- name: ListQualificheCliente :many
SELECT q.*, f.ragione_sociale AS fornitore, f.tipo, l.descrizione AS lavorazione_descrizione
FROM cliente_fornitore_lavorazione q
JOIN fornitore f ON f.fornitore_id = q.fornitore_id
JOIN lavorazione l ON l.codice = q.lavorazione
WHERE q.cliente_id = $1 ORDER BY l.descrizione, f.ragione_sociale;

-- name: InsertQualifica :execrows
INSERT INTO cliente_fornitore_lavorazione (cliente_id, fornitore_id, lavorazione)
VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: EliminaQualifica :execrows
DELETE FROM cliente_fornitore_lavorazione WHERE cliente_id = $1 AND fornitore_id = $2 AND lavorazione = $3;

-- ListFornitoriQualificati: chi e' qualificato per QUESTO cliente su QUESTA lavorazione. Chi fa la
-- lavorazione ma non e' qualificato per il cliente non compare: e' la differenza fra
-- fornitore_lavorazione (capacita') e cliente_fornitore_lavorazione (qualifica).
-- name: ListFornitoriQualificati :many
SELECT f.* FROM cliente_fornitore_lavorazione q JOIN fornitore f ON f.fornitore_id = q.fornitore_id
WHERE q.cliente_id = $1 AND q.lavorazione = $2 AND f.attivo ORDER BY f.ragione_sociale;

-- ---------------------------------------------------------------- convenzioni di codice (D39)

-- name: ListConvenzioniCliente :many
SELECT c.*,
       COALESCE((SELECT array_agg(l.lavorazione ORDER BY l.lavorazione)
                   FROM convenzione_codice_lavorazione l WHERE l.convenzione_id = c.convenzione_id), '{}')::text[] AS lavorazioni
FROM convenzione_codice c WHERE c.cliente_id = $1 ORDER BY c.creato_il, c.espressione;

-- name: GetConvenzione :one
SELECT * FROM convenzione_codice WHERE convenzione_id = $1;

-- name: InsertConvenzione :one
INSERT INTO convenzione_codice (cliente_id, modo, espressione, esempio, controesempio, descrizione)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: InsertConvenzioneLavorazione :exec
INSERT INTO convenzione_codice_lavorazione (convenzione_id, lavorazione) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: SetConvenzioneAttiva :execrows
UPDATE convenzione_codice SET attiva = $3 WHERE convenzione_id = $1 AND cliente_id = $2;

-- name: EliminaConvenzione :execrows
DELETE FROM convenzione_codice WHERE convenzione_id = $1 AND cliente_id = $2;

-- ---------------------------------------------------------------- la controparte sul messaggio (D33)

-- name: SetControparteMessaggio :execrows
-- Una controparte decisa da una persona (`manuale`) non si sovrascrive con una risoluzione
-- automatica: il ricalcolo passa oltre.
UPDATE messaggio
   SET controparte_tipo = $2, controparte_cliente_id = $3, controparte_fornitore_id = $4,
       controparte_via = sqlc.narg(via)::via_controparte, controparte_il = now()
 WHERE messaggio_id = $1
   AND (controparte_via IS DISTINCT FROM 'manuale' OR sqlc.narg(via)::via_controparte = 'manuale');

-- name: ContaControparti :many
SELECT controparte_tipo::text AS tipo, count(*)::int AS n FROM messaggio GROUP BY controparte_tipo ORDER BY 1;

-- name: ListMessaggiSenzaControparte :many
-- I messaggi entrati prima della 0014: la migrazione li lascia `sconosciuto` senza data, e il
-- ricalcolo al primo avvio li risolve a lotti (CP8).
SELECT * FROM messaggio WHERE controparte_il IS NULL ORDER BY registrato_il LIMIT $1;

-- ListMessaggiDaRitriage: i messaggi NON DECISI che parlano con questo indirizzo o questo dominio,
-- in entrata (mittente) o in uscita (uno dei destinatari). Non decisi = nessuna RFQ e nessuna
-- proposta accettata o rifiutata: le decisioni prese non si toccano (7A.3).
-- name: ListMessaggiDaRitriage :many
SELECT m.* FROM messaggio m
WHERE m.thread_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM proposta_triage p WHERE p.messaggio_id = m.messaggio_id AND p.stato <> 'proposta')
  AND (   lower(m.mittente_indirizzo) = lower(sqlc.arg(indirizzo)::text)
       OR (sqlc.arg(dominio)::text <> '' AND lower(split_part(m.mittente_indirizzo, '@', 2)) = lower(sqlc.arg(dominio)::text))
       OR EXISTS (SELECT 1 FROM jsonb_array_elements(m.destinatari) d
                   WHERE lower(d->>'indirizzo') = lower(sqlc.arg(indirizzo)::text)
                      OR (sqlc.arg(dominio)::text <> '' AND lower(split_part(d->>'indirizzo', '@', 2)) = lower(sqlc.arg(dominio)::text))))
ORDER BY m.data_evento;

-- name: ListMessaggiPerControparteFornitore :many
SELECT * FROM messaggio WHERE controparte_fornitore_id = $1 ORDER BY data_evento DESC LIMIT 50;
