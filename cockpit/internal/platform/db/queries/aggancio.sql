-- Candidati di aggancio e di codice (checkpoint 3R, fase 4.1).
-- Nessuna di queste query scrive `messaggio.thread_id`: quello lo fa solo una decisione dell'operatore.

-- name: InsertCandidatoAggancio :exec
INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (messaggio_id, thread_id, regola) DO UPDATE
    SET punteggio = EXCLUDED.punteggio, evidenza = EXCLUDED.evidenza, thread_stato = EXCLUDED.thread_stato;

-- ListCandidatiAggancio: tutti i candidati di un messaggio, i piu' forti in cima. TUTTI, non il primo:
-- scegliere per l'operatore e' esattamente cio' che questo checkpoint toglie (T16).
-- name: ListCandidatiAggancio :many
SELECT k.messaggio_id, k.thread_id, k.regola, k.punteggio, k.evidenza, k.thread_stato,
       t.oggetto, t.data_inizio, t.riferimento_cliente, c.ragione_sociale AS cliente
FROM candidato_aggancio k
JOIN thread_offerta t ON t.thread_id = k.thread_id
JOIN cliente c        ON c.cliente_id = t.cliente_id
WHERE k.messaggio_id = $1
ORDER BY k.punteggio DESC, k.regola;

-- name: EliminaCandidatiAggancio :execrows
DELETE FROM candidato_aggancio WHERE messaggio_id = $1;

-- name: ContaCandidatiAggancio :one
SELECT count(*) FROM candidato_aggancio WHERE messaggio_id = $1;

-- name: InsertCandidatoCodice :exec
INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (messaggio_id, codice) DO UPDATE
    SET ruolo = EXCLUDED.ruolo, rev = EXCLUDED.rev, origine = EXCLUDED.origine,
        famiglia = EXCLUDED.famiglia, punteggio = EXCLUDED.punteggio, evidenza = EXCLUDED.evidenza;

-- name: ListCandidatiCodice :many
SELECT * FROM candidato_codice WHERE messaggio_id = $1
ORDER BY array_position(ARRAY['riferimento_rfq','prodotto','parte','non_classificato']::ruolo_codice[], ruolo),
         punteggio DESC, codice;

-- name: EliminaCandidatiCodice :execrows
DELETE FROM candidato_codice WHERE messaggio_id = $1;

-- ---------------------------------------------------------------- le sorgenti delle regole R0–R5

-- R0: i Message-ID citati da In-Reply-To e References, risolti su messaggi GIA' agganciati.
-- name: ThreadPerChiaviCitate :many
SELECT DISTINCT m.thread_id, m.chiave_esterna, t.stato
FROM messaggio m
JOIN thread_offerta t ON t.thread_id = m.thread_id
-- Il canale sta nella condizione perche' l'indice unico e' (canale, chiave_esterna): senza, ogni
-- messaggio con delle References scorreva tutta la tabella dei messaggi.
WHERE m.canale = 'outlook' AND m.chiave_esterna = ANY(sqlc.arg(chiavi)::text[]) AND m.thread_id IS NOT NULL;

-- R1: la conversazione, se un operatore l'ha gia' collegata a una RFQ.
-- name: ThreadDellaConversazioneConStato :one
SELECT c.thread_id, t.stato
FROM conversazione c JOIN thread_offerta t ON t.thread_id = c.thread_id
WHERE c.conversazione_id = $1 AND c.thread_id IS NOT NULL;

-- R2: stesso oggetto (gia' normalizzato: `thread_offerta.oggetto` e' l'oggetto pulito), stesso cliente,
-- dentro la finestra. Senza finestra, «Richiesta offerta» aggancerebbe richieste di quattro anni fa (T3).
-- name: ThreadPerOggettoCliente :many
SELECT t.thread_id, t.stato, t.oggetto, t.data_inizio
FROM thread_offerta t
WHERE t.cliente_id = $1 AND t.unito_in IS NULL
  AND upper(t.oggetto) = upper(sqlc.arg(oggetto)::text)
  AND t.data_inizio >= sqlc.arg(dal)::timestamptz
ORDER BY t.data_inizio DESC
LIMIT 5;

-- R3: un codice gia' identificativo di una RFQ dello stesso cliente. Solo dello stesso cliente: due
-- clienti diversi usano gli stessi numeri e non vuol dire niente (T4).
-- name: ThreadPerCodiciCliente :many
SELECT DISTINCT t.thread_id, t.stato, t.data_inizio, i.codice
FROM identificativo_thread i
JOIN thread_offerta t ON t.thread_id = i.thread_id
WHERE t.cliente_id = $1 AND t.unito_in IS NULL
  AND upper(i.codice) = ANY(sqlc.arg(codici)::text[])
  AND t.data_inizio >= sqlc.arg(dal)::timestamptz
ORDER BY t.data_inizio DESC
LIMIT 10;

-- R4: il riferimento con cui il cliente chiama la richiesta.
-- name: ThreadPerRiferimentoCliente :many
SELECT t.thread_id, t.stato, t.data_inizio, t.riferimento_cliente
FROM thread_offerta t
WHERE t.cliente_id = $1 AND t.unito_in IS NULL
  AND upper(t.riferimento_cliente) = upper(sqlc.arg(riferimento)::text)
ORDER BY t.data_inizio DESC
LIMIT 5;

-- R5: lo stesso buyer, di recente. Da sola non decide niente e non basta mai: mette in fila i candidati
-- quando le regole forti non hanno parlato.
-- name: ThreadRecentiBuyer :many
SELECT t.thread_id, t.stato, t.oggetto, t.data_inizio
FROM thread_offerta t
WHERE t.buyer_id = $1 AND t.unito_in IS NULL AND t.stato = 'APERTA'
  AND t.data_inizio >= sqlc.arg(dal)::timestamptz
ORDER BY t.data_inizio DESC
LIMIT 5;

-- name: SetRiferimentoCliente :exec
UPDATE thread_offerta SET riferimento_cliente = sqlc.narg(riferimento) WHERE thread_id = $1;

-- ListOrfaniConversazione: gli altri messaggi orfani della stessa conversazione. Prima venivano
-- AGGANCIATI in blocco; adesso si leggono per proporli, uno per uno, con il loro candidato (T21).
-- name: ListOrfaniConversazione :many
SELECT messaggio_id FROM messaggio
WHERE conversazione_id = $1 AND thread_id IS NULL AND messaggio_id <> $2
ORDER BY data_evento;

-- AggiornaTriageCandidato: la proposta di un messaggio cambia quando compare un'evidenza nuova (per
-- esempio: un operatore aggancia un messaggio, e gli altri orfani della stessa conversazione acquistano
-- un candidato R1). Cambia la PROPOSTA, non la decisione: tocca solo le righe ancora `proposta`.
-- name: AggiornaTriageCandidato :execrows
UPDATE proposta_triage
SET esito = 'aggancia', thread_proposto = $2, confidenza = $3,
    motivi = motivi || sqlc.arg(motivo)::jsonb
WHERE messaggio_id = $1 AND fonte = 'deterministico' AND stato = 'proposta';
