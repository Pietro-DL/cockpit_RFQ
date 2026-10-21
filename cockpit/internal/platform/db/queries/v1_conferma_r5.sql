-- V1, direttiva 39 (§1, §2, §7; EM §9.5, §7.5, App. I.6), corsia persistenza, file proprio per INC-G21: le query di R5
-- formato 3 (la conferma atomica, core/rfq/distintabozza/conferma3*.go) che non c'erano gia'. I lucchetti seguono l'ordine
-- documentato nel README di distintabozza (RFQ → bozza → documenti → componente del prodotto → documento_proposta →
-- componente_proposta → relazione_proposta); le scritture della working sono quelle del passo 1 di App. I.6, prima della
-- testata della conferma (working_conta, P4X01).

-- name: InsertRelazioneR5 :exec
-- Un arco nuovo della working scritto da R5: una INSERT senza ON CONFLICT (C3 §5.3 passo 8): una coppia gia' presente e'
-- un errore (23505), mai un arco ignorato in silenzio. L'origine e' step per un arco accettato dallo STEP, manuale per gli
-- altri; la posizione e' quella della bozza.
INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, posizione, origine, confermato_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(padre_id), sqlc.arg(figlio_id), sqlc.arg(qta), sqlc.narg(posizione), sqlc.arg(origine),
        sqlc.arg(confermato_da));

-- name: SetPosizioneRelazioneR5 :execrows
-- La posizione di un arco della working che la bozza cambia (asse «dati» di working_contatore).
UPDATE componente_relazione SET posizione = sqlc.narg(posizione), confermato_da = sqlc.arg(confermato_da)
 WHERE thread_id = sqlc.arg(thread_id) AND padre_id = sqlc.arg(padre_id) AND figlio_id = sqlc.arg(figlio_id);

-- name: BloccaDocumentiPropostaR5 :many
-- Le proposte di documento degli allegati che R5 associa o svincola (la copia derivata la scrive il servizio unico), in
-- ordine di proposta: il primo gruppo delle righe del worker nell'ordine della strategia A (EM §9.5 passo 7).
SELECT d.proposta_id
  FROM documento_proposta d
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.allegato_id = ANY (sqlc.arg(allegati)::uuid[])
 ORDER BY d.proposta_id
   FOR UPDATE;

-- name: BloccaComponentiPropostaR5 :many
-- Le righe di componente_proposta che R5 segna o chiude, in ordine di proposta.
SELECT p.proposta_id
  FROM componente_proposta p
 WHERE p.thread_id = sqlc.arg(thread_id) AND p.proposta_id = ANY (sqlc.arg(proposte)::uuid[])
 ORDER BY p.proposta_id
   FOR UPDATE;

-- name: BloccaRelazioniPropostaR5 :many
-- Le righe di relazione_proposta che R5 segna o chiude, per chiave «allegato|padre|figlio» (la forma di IngressiProposta),
-- in ordine di chiave.
SELECT r.allegato_id, r.padre_chiave, r.figlio_chiave
  FROM relazione_proposta r
 WHERE r.thread_id = sqlc.arg(thread_id)
   AND concat_ws('|', r.allegato_id, r.padre_chiave, r.figlio_chiave) = ANY (sqlc.arg(chiavi)::text[])
 ORDER BY r.allegato_id, r.padre_chiave, r.figlio_chiave
   FOR UPDATE;

-- name: ListProposteDelloShaR5 :many
-- Le righe di componente_proposta di un contenuto (lo STEP A sostituito da B): R5 chiude quelle aperte ai segni legacy (la
-- nota «superata da ‹B› (conferma n)», R39-13), e l'elenco le esclude dalla parte proposta di H_S e H_T.
SELECT p.proposta_id, p.allegato_id, p.chiave, p.stato
  FROM componente_proposta p
 WHERE p.thread_id = sqlc.arg(thread_id) AND p.sha256 = sqlc.arg(sha256)
 ORDER BY p.proposta_id;

-- name: ListRelazioniDelloShaR5 :many
-- Le righe di relazione_proposta delle occorrenze di un contenuto (lo STEP A sostituito da B).
SELECT r.allegato_id, r.padre_chiave, r.figlio_chiave, r.stato
  FROM relazione_proposta r
  JOIN allegato a ON a.allegato_id = r.allegato_id
 WHERE r.thread_id = sqlc.arg(thread_id) AND a.sha256 = sqlc.arg(sha256)
 ORDER BY r.allegato_id, r.padre_chiave, r.figlio_chiave;

-- name: ListRelazioniPropostaR5 :many
-- Le righe di relazione_proposta per chiave «allegato|padre|figlio» (gli archi dello STEP della bozza): R5 ci mette il segno.
SELECT r.allegato_id, r.padre_chiave, r.figlio_chiave, r.stato, r.deciso_da
  FROM relazione_proposta r
 WHERE r.thread_id = sqlc.arg(thread_id)
   AND concat_ws('|', r.allegato_id, r.padre_chiave, r.figlio_chiave) = ANY (sqlc.arg(chiavi)::text[])
 ORDER BY r.allegato_id, r.padre_chiave, r.figlio_chiave;

-- name: ChiudiProposteDelloShaR5 :execrows
-- Lo STEP A sostituito da B con R5 (segni legacy, R39-13): le sue righe di componente_proposta ancora aperte si chiudono
-- come scartate da un automatismo (come ChiudiProposteComponenteDiUnFile della sostituzione legacy), tranne quelle che la
-- bozza confermata usa come caselle (escluse).
UPDATE componente_proposta SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
 WHERE thread_id = sqlc.arg(thread_id) AND sha256 = sqlc.arg(sha256) AND stato = 'aperta'
   AND NOT (proposta_id = ANY (COALESCE(sqlc.arg(escluse)::uuid[], '{}'::uuid[])));

-- name: ChiudiRelazioniDelloShaR5 :execrows
-- Come ChiudiProposteDelloShaR5, per le righe di relazione_proposta aperte delle occorrenze di A, tranne gli archi che la
-- bozza confermata usa (escluse, per chiave «allegato|padre|figlio»).
UPDATE relazione_proposta r SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
  FROM allegato a
 WHERE a.allegato_id = r.allegato_id AND a.sha256 = sqlc.arg(sha256) AND r.thread_id = sqlc.arg(thread_id) AND r.stato = 'aperta'
   AND NOT (concat_ws('|', r.allegato_id, r.padre_chiave, r.figlio_chiave) = ANY (COALESCE(sqlc.arg(escluse)::text[], '{}'::text[])));

-- name: SegnaF8DellaConfermaEseguito :execrows
-- F8 dopo il COMMIT di R5, nella stessa richiesta (C3 §5.3 passo 19): lo stato di F8 della conferma a eseguito.
UPDATE f8_stato SET stato = 'eseguito', errore = NULL, risolto_da = 'stessa_richiesta', tentativi = tentativi + 1
 WHERE thread_id = sqlc.arg(thread_id) AND conferma_id = sqlc.arg(conferma_id) AND stato <> 'eseguito';

-- name: SegnaF8DellaConfermaErrore :execrows
-- F8 dopo il COMMIT di R5 non riuscito: lo stato di F8 della conferma a errore (R11 lo riprende).
UPDATE f8_stato SET stato = 'errore', errore = sqlc.arg(errore), tentativi = tentativi + 1
 WHERE thread_id = sqlc.arg(thread_id) AND conferma_id = sqlc.arg(conferma_id) AND stato <> 'eseguito';

-- name: GetF8StatoDellaConferma :one
-- Lo stato di F8 della conferma (la risposta di R5 e il reinvio riproposto).
SELECT stato FROM f8_stato WHERE thread_id = sqlc.arg(thread_id) AND conferma_id = sqlc.arg(conferma_id);
