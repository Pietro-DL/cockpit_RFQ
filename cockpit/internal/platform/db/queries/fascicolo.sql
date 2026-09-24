-- name: ListFascicolo :many
SELECT * FROM v_fascicolo WHERE thread_id = $1 ORDER BY codice, tipo_documento;

-- name: GetBloccantiThread :one
SELECT * FROM v_thread_bloccanti WHERE thread_id = $1;

-- name: UpsertProposta :one
INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, componente_id, confidenza, fonte, regola_id, dettagli)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (allegato_id) DO UPDATE SET
    thread_id = COALESCE(EXCLUDED.thread_id, documento_proposta.thread_id),
    tipo_proposto = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.tipo_proposto ELSE documento_proposta.tipo_proposto END,
    codice = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.codice ELSE documento_proposta.codice END,
    rev = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.rev ELSE documento_proposta.rev END,
    confidenza = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.confidenza ELSE documento_proposta.confidenza END,
    fonte = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.fonte ELSE documento_proposta.fonte END,
    dettagli = CASE WHEN documento_proposta.stato = 'aperta' THEN EXCLUDED.dettagli ELSE documento_proposta.dettagli END
RETURNING *;

-- name: InsertPropostaSeAssente :exec
-- prima proposta a ingest (solo nome file): non tocca mai una proposta esistente, che può essere già raffinata
INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (allegato_id) DO NOTHING;

-- name: GetProposta :one
SELECT * FROM documento_proposta WHERE proposta_id = $1;

-- name: BloccaProposta :one
-- La proposta bloccata per la durata della transazione (voce 1.9, T14). Due conferme concorrenti sullo
-- stesso allegato leggerebbero entrambe stato='aperta' e creerebbero due documenti nel fascicolo, con
-- lo stesso file copiato due volte sul NAS. Con il blocco la seconda trova la proposta già decisa e
-- lo dice.
SELECT * FROM documento_proposta WHERE proposta_id = $1 FOR UPDATE;

-- name: ListProposteAperteThread :many
SELECT p.*, a.nome_file, a.estensione, a.bytes, a.sha256, a.path_staging, a.messaggio_id, a.contenitore_id
FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
WHERE p.thread_id = $1 AND p.stato = 'aperta'
ORDER BY a.ricevuto_il, a.messaggio_id, a.contenitore_id NULLS FIRST, a.indice;

-- name: ListProposteBulk :many
SELECT p.* FROM documento_proposta p
WHERE p.thread_id = $1 AND p.stato = 'aperta' AND p.confidenza >= $2 AND p.tipo_proposto <> 'rumore';

-- name: DecidiProposta :execrows
UPDATE documento_proposta SET stato = $2, deciso_da = $3, deciso_il = now() WHERE proposta_id = $1 AND stato = 'aperta';

-- name: AssegnaThreadProposte :execrows
UPDATE documento_proposta p SET thread_id = $2
FROM allegato a WHERE a.allegato_id = p.allegato_id AND a.messaggio_id = $1 AND p.thread_id IS NULL;

-- name: InsertDocumento :one
INSERT INTO documento (thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, bytes, path_relativo, confermato_da, nota)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetDocumento :one
SELECT * FROM documento WHERE documento_id = $1;

-- name: GetDocumentoPerHash :one
SELECT * FROM documento WHERE thread_id = $1 AND sha256 = $2;

-- name: ListDocumentiThread :many
SELECT * FROM documento WHERE thread_id = $1 ORDER BY componente_id NULLS LAST, tipo, nome_file;

-- name: InsertProvenienza :exec
INSERT INTO documento_provenienza (documento_id, allegato_id, rif_id, messaggio_id, ricevuto_il, caricato_da)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (documento_id, ricevuto_il) DO NOTHING;

-- name: SetDocumentoScritto :execrows
-- «Scritto» vale per il percorso su cui il file e' stato scritto, o verificato: chi chiama passa
-- QUEL percorso. Se nel frattempo il percorso del documento e' cambiato (una correzione del codice,
-- addendum A1.4), zero righe: il database non dichiara scritto un file in una cartella dove non c'e'.
UPDATE documento SET stato_nas = 'scritto', scritto_il = now(), errore_nas = NULL
WHERE documento_id = $1 AND path_relativo = $2;

-- name: BloccaDocumento :one
-- Il documento bloccato per la durata della transazione. Chi cambia componente, codice o percorso di
-- un documento parte da qui (A1.4), e due correzioni concorrenti si mettono in fila.
SELECT * FROM documento WHERE documento_id = $1 FOR UPDATE;

-- name: BloccaCopiaPendente :one
-- La copia sul NAS ancora da finire per un documento, bloccata (A1.4, regola 2). Nessuna riga: non
-- c'e' una copia in attesa. `pronto` bloccato: finche' la transazione e' aperta ClaimJob la salta
-- (SKIP LOCKED), e la copia parte DOPO, leggendo il percorso nuovo. `in_corso`: la copia sta gia'
-- usando il percorso vecchio, e chi voleva cambiarlo rinuncia.
SELECT * FROM job
WHERE tipo = 'copia_nas' AND chiave_idempotenza = $1 AND stato IN ('pronto','in_corso')
FOR UPDATE;

-- name: SetComponenteDocumento :execrows
-- Aggancia un documento a un componente, o lo sgancia (componente NULL). Il codice si scrive nella
-- stessa istruzione: la FK (thread, componente, codice) lo controlla alla fine, ed e' lei che decide
-- se il componente e' di questa RFQ e se il codice e' il suo, non il Go.
UPDATE documento SET componente_id = sqlc.narg(componente_id), codice = sqlc.narg(codice)
WHERE documento_id = sqlc.arg(documento_id);

-- name: SetPathDocumentoInCoda :execrows
-- Il percorso cambia solo per un documento che sul NAS non c'e' ancora: in coda o in errore (A1.4,
-- regole 2 e 4). Uno `scritto` si sposta con sposta_nas (B8.8), non con questa riga: zero righe
-- vuol dire che lo stato e' cambiato nel frattempo, e il chiamante rinuncia.
UPDATE documento SET path_relativo = $2
WHERE documento_id = $1 AND stato_nas IN ('in_coda','errore');

-- name: ListDocumentiComponente :many
-- I documenti agganciati a un componente, bloccati: la correzione del codice li porta tutti o nessuno.
SELECT * FROM documento WHERE componente_id = $1 ORDER BY documento_id FOR UPDATE;

-- name: SetComponenteProposta :execrows
-- Aggancia (o sgancia) una proposta ancora aperta. Il codice arriva dal componente (A2.2), e la FK a
-- tre campi lo controlla come per il documento.
UPDATE documento_proposta SET componente_id = sqlc.narg(componente_id), codice = sqlc.narg(codice)
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta';

-- name: SetCodiceProposteComponente :execrows
-- Le proposte agganciate a un componente, di qualunque stato, prendono il suo codice nuovo: la FK le
-- vuole uguali, e una proposta gia' confermata non si sgancia solo perche' il codice e' stato corretto.
UPDATE documento_proposta SET codice = $2 WHERE componente_id = $1;

-- name: SetDocumentoErrore :exec
UPDATE documento SET stato_nas = 'errore', errore_nas = $2 WHERE documento_id = $1;

-- name: GetCartellaDocumento :one
SELECT * FROM cartella_documento WHERE tipo = $1;

-- name: ListCartellaDocumento :many
SELECT * FROM cartella_documento ORDER BY tipo;

-- name: ListProposteThreadTutte :many
-- tutte le proposte (aperte e decise) degli allegati dei messaggi del thread, per la schermata B
SELECT p.* FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id JOIN messaggio m ON m.messaggio_id = a.messaggio_id
WHERE m.thread_id = $1;

-- name: ListProposteMessaggio :many
SELECT p.* FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id WHERE a.messaggio_id = $1;

-- name: GetComponentePerCodice :one
-- al piu' una riga: dalla 0018 (thread, upper(codice)) e' l'identita' del componente
SELECT * FROM componente WHERE thread_id = $1 AND upper(codice) = upper($2);

-- name: ListDocumentiMessaggio :many
-- documenti confermati a partire dagli allegati di questo messaggio (per mostrare "sul NAS" accanto all'allegato)
SELECT sqlc.embed(d), dp.allegato_id FROM documento d JOIN documento_provenienza dp ON dp.documento_id = d.documento_id
WHERE dp.messaggio_id = sqlc.arg(messaggio_id)::uuid;

-- ------------------------------------------------------------------ A4 (B8.A4a): revisioni, STEP del prodotto, nomi sul NAS
-- name: SetSostituitoDa :execrows
-- La sostituzione: confronta e scambia. Il trigger della catena vuole il successore ancora corrente e
-- lo legge FOR UPDATE; zero righe = il documento era gia' stato sostituito nel frattempo.
UPDATE documento SET sostituito_da = sqlc.arg(nuovo) WHERE documento_id = sqlc.arg(vecchio) AND sostituito_da IS NULL;

-- name: AnnullaSostituzione :execrows
-- Solo l'ultima sostituzione: il trigger vuole il successore ancora corrente.
UPDATE documento SET sostituito_da = NULL WHERE documento_id = sqlc.arg(vecchio) AND sostituito_da = sqlc.arg(successore);

-- name: ListStoriaDocumento :many
SELECT * FROM v_documento_storia
WHERE catena_id = (SELECT s.catena_id FROM v_documento_storia s WHERE s.documento_id = $1)
ORDER BY passo;

-- name: ListStepProdotto :many
SELECT * FROM v_step_prodotto WHERE thread_id = $1 ORDER BY codice;

-- name: BloccaCartella :exec
-- Chi sceglie un nome nuovo in una cartella e chi lo riserva si mettono in fila (A4.3): un lucchetto
-- di transazione sull'impronta del thread e della cartella in minuscolo. Due cartelle con la stessa
-- impronta si mettono in fila senza bisogno, e non succede altro.
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(thread_id)::uuid::text || ':' || lower(sqlc.arg(cartella)::text), 0));

-- name: PercorsoOccupato :one
-- Un percorso e' occupato se lo dichiara un documento del thread, se e' il `da` o l'`a` di uno
-- spostamento pendente, o se c'e' una riga aperta di nas_orfano (A4.3, R2.1). Sempre in minuscolo:
-- la condivisione e' Windows.
SELECT EXISTS (SELECT 1 FROM documento d
                WHERE d.thread_id = sqlc.arg(thread_id) AND lower(d.path_relativo) = lower(sqlc.arg(percorso)::text))
    OR EXISTS (SELECT 1 FROM job j
                WHERE j.tipo = 'sposta_nas' AND j.stato IN ('pronto', 'in_corso')
                  AND j.payload ->> 'thread_id' = sqlc.arg(thread_id)::uuid::text
                  AND (lower(j.payload ->> 'da') = lower(sqlc.arg(percorso)::text) OR lower(j.payload ->> 'a') = lower(sqlc.arg(percorso)::text)))
    OR EXISTS (SELECT 1 FROM nas_orfano o
                WHERE o.thread_id = sqlc.arg(thread_id) AND o.risolto_il IS NULL AND lower(o.percorso) = lower(sqlc.arg(percorso)::text)) AS occupato;

-- name: CartellaReferenziata :one
-- Una cartella si toglie solo se niente la nomina: documenti, spostamenti pendenti, orfani aperti.
-- Mai le istantanee: path_al_congelamento e' storico (R1.8).
SELECT EXISTS (SELECT 1 FROM documento d
                WHERE d.thread_id = sqlc.arg(thread_id) AND starts_with(lower(d.path_relativo), lower(sqlc.arg(cartella)::text) || '\'))
    OR EXISTS (SELECT 1 FROM job j
                WHERE j.tipo = 'sposta_nas' AND j.stato IN ('pronto', 'in_corso')
                  AND j.payload ->> 'thread_id' = sqlc.arg(thread_id)::uuid::text
                  AND (starts_with(lower(j.payload ->> 'da'), lower(sqlc.arg(cartella)::text) || '\')
                       OR starts_with(lower(j.payload ->> 'a'), lower(sqlc.arg(cartella)::text) || '\')))
    OR EXISTS (SELECT 1 FROM nas_orfano o
                WHERE o.thread_id = sqlc.arg(thread_id) AND o.risolto_il IS NULL
                  AND starts_with(lower(o.percorso), lower(sqlc.arg(cartella)::text) || '\')) AS referenziata;

-- name: BloccaSpostamentoPendente :one
-- Lo spostamento ancora da finire di un documento, bloccato (A4.3, passo 0): come BloccaCopiaPendente.
SELECT * FROM job
WHERE tipo = 'sposta_nas' AND chiave_idempotenza = $1 AND stato IN ('pronto', 'in_corso')
FOR UPDATE;

-- name: SetPathDocumentoSeUguale :execrows
-- Il passo 4 di sposta_nas: il percorso cambia solo se e' ancora quello da cui si sposta.
UPDATE documento SET path_relativo = sqlc.arg(a) WHERE documento_id = sqlc.arg(documento_id) AND path_relativo = sqlc.arg(da);
