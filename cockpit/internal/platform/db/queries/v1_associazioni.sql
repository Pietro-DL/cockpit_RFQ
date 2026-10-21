-- Le query del servizio unico delle associazioni registrate (AR, direttiva 35 §3; piano delle dipendenze §3.6, blocco
-- C-1c): un file proprio della corsia persistenza (INC-G21). Le chiama solo core/rfq/associazioni, l'unico scrittore di
-- file_rfq_associazione, del suo giornale in associazione_evento e della copia derivata documento_proposta.componente_id.
-- Nessuna prende lucchetti (precondizione del servizio: chi chiama ha gia' bloccato la RFQ prima di ogni proposta,
-- INC-G25) e nessuna usa ON CONFLICT (V1-Q04 A): l'idempotenza la decide il servizio leggendo per chiave.

-- name: ListAssociazioniAttiveDiAllegato :many
-- Le AR attive di un allegato della RFQ (al piu' una con ux_fra_attiva_allegato: provvisorio, base [T] D-ASS-04 A).
SELECT * FROM file_rfq_associazione
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id) AND stato = 'attiva'
 ORDER BY creata_il, associazione_id;

-- name: ListAssociazioniAttiveDellaRfq :many
-- Le AR attive della RFQ: la lettura della working usa solo queste.
SELECT * FROM file_rfq_associazione
 WHERE thread_id = sqlc.arg(thread_id) AND stato = 'attiva'
 ORDER BY allegato_id, componente_id;

-- name: ListStoriaAssociazioniDiAllegato :many
-- Tutte le AR di un allegato della RFQ, attive e chiuse, dalla prima: le righe chiuse restano (chi, quando, motivo).
-- A parita' di nascita la riga chiusa viene prima di quella che l'ha rimpiazzata nella stessa transazione.
SELECT * FROM file_rfq_associazione
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id)
 ORDER BY creata_il, (stato = 'attiva'), associazione_id;

-- name: ListAssociazioniPerChiave :many
-- Le AR nate con la chiave d'atto data per un allegato (ux_fra_chiave comprende il componente): il reinvio dello stesso
-- atto le ritrova invece di farle rinascere.
SELECT * FROM file_rfq_associazione
 WHERE thread_id = sqlc.arg(thread_id) AND chiave_idempotenza = sqlc.arg(chiave_idempotenza) AND allegato_id = sqlc.arg(allegato_id)
 ORDER BY creata_il, associazione_id;

-- name: ListAssociazioniChiusePerChiave :many
-- Le AR chiuse con la chiave d'atto data per un allegato (ux_fra_chiusa_chiave): il reinvio della stessa chiusura.
SELECT * FROM file_rfq_associazione
 WHERE thread_id = sqlc.arg(thread_id) AND chiusa_chiave = sqlc.arg(chiusa_chiave) AND allegato_id = sqlc.arg(allegato_id)
 ORDER BY chiusa_il, associazione_id;

-- name: ListEventiAssociazionePerChiave :many
-- Gli eventi del giornale scritti con la chiave d'atto data per un allegato (anche quelli senza riga: lo sgancio di una
-- copia senza AR).
SELECT * FROM associazione_evento
 WHERE thread_id = sqlc.arg(thread_id) AND chiave_idempotenza = sqlc.arg(chiave_idempotenza) AND allegato_id = sqlc.arg(allegato_id)
 ORDER BY evento_id;

-- name: InsertAssociazione :one
-- La nascita di una AR (attiva). Stato, ora e transazione li scrive il database; le sei verifiche della direttiva §2.1
-- le fanno i trigger della 0023 (trg_fra_nasce, trg_fra_bom, FK) e, al COMMIT, trg_fra_invarianti (con la copia
-- verificata della provenienza della PB e del registro d'origine, direttiva 39 §4).
INSERT INTO file_rfq_associazione (thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza, provenienza_pb,
                                   importazione_id, sostituisce_id, motivo, chiave_idempotenza, creata_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(componente_id), sqlc.arg(origine), sqlc.narg(conferma_id),
        sqlc.narg(chiave_bozza), sqlc.narg(provenienza_pb), sqlc.narg(importazione_id), sqlc.narg(sostituisce_id), sqlc.narg(motivo),
        sqlc.arg(chiave_idempotenza), sqlc.arg(creata_da))
RETURNING *;

-- name: ChiudiAssociazione :one
-- La chiusura di una AR attiva, una volta sola (trg_fra_cambia scrive chiusa_il e chiusa_tx). Nessuna riga se la AR non
-- e' piu' attiva.
UPDATE file_rfq_associazione
   SET stato = 'chiusa', chiusa_da = sqlc.arg(chiusa_da), motivo_chiusura = sqlc.arg(motivo_chiusura),
       chiusa_conferma_id = sqlc.narg(chiusa_conferma_id), chiusa_chiave = sqlc.narg(chiusa_chiave), chiusa_nota = sqlc.narg(chiusa_nota)
 WHERE associazione_id = sqlc.arg(associazione_id) AND thread_id = sqlc.arg(thread_id) AND stato = 'attiva'
RETURNING *;

-- name: InsertEventoAssociazione :one
-- Un evento del giornale delle AR (direttiva 35 §2.4): nascita (preassociazione), sostituzione (sostituisci, con la riga
-- chiusa), chiusura (sgancia), sgancio di una copia senza AR (sgancia senza riga), legame della sorgente (assegna). La
-- transazione la scrive il database (trg_associazione_della_riga), che controlla anche che l'evento ripeta la riga.
INSERT INTO associazione_evento (thread_id, proposta_id, allegato_id, associazione_id, associazione_precedente_id, azione,
                                 componente_prima, componente_dopo, conferma_id, motivo, origine, chiave_idempotenza, autore_id)
VALUES (sqlc.arg(thread_id), sqlc.narg(proposta_id), sqlc.arg(allegato_id), sqlc.narg(associazione_id),
        sqlc.narg(associazione_precedente_id), sqlc.arg(azione), sqlc.narg(componente_prima), sqlc.narg(componente_dopo),
        sqlc.narg(conferma_id), sqlc.narg(motivo), sqlc.arg(origine), sqlc.arg(chiave_idempotenza), sqlc.arg(autore_id)::uuid)
RETURNING evento_id;

-- name: ListProposteDiAllegatoPerCopia :many
-- La proposta di documento dell'allegato (zero o una: documento_proposta.allegato_id e' UNIQUE), per la copia.
SELECT proposta_id, thread_id, stato, componente_id, codice
  FROM documento_proposta
 WHERE allegato_id = sqlc.arg(allegato_id);

-- name: ScriviCopiaAssociazione :many
-- La copia derivata di compatibilita' (direttiva 35 §3): la proposta aperta dell'allegato prende il componente della AR e
-- il suo codice (la FK a tre campi lo vuole uguale). Prima della copia la lettura del codice si conserva in
-- dettagli.codice_letto (DIR35-DP, INC-G4): solo quando la proposta non aveva ancora una copia, e anche quando la lettura
-- e' vuota (JSON null), cosi' lo scollegamento la rimette e impronta-ingressi-2 legge sempre la stessa lettura. La
-- proposta senza RFQ prende la RFQ della AR (ck_proposta_componente_thread).
UPDATE documento_proposta p
   SET componente_id = c.componente_id,
       codice = c.codice,
       thread_id = c.thread_id,
       dettagli = CASE WHEN p.componente_id IS NULL
                       THEN COALESCE(p.dettagli, '{}'::jsonb) || jsonb_build_object('codice_letto', p.codice)
                       ELSE p.dettagli END
  FROM componente c
 WHERE p.allegato_id = sqlc.arg(allegato_id) AND p.stato = 'aperta'
   AND (p.thread_id IS NULL OR p.thread_id = c.thread_id)
   AND c.componente_id = sqlc.arg(componente_id) AND c.thread_id = sqlc.arg(thread_id)
RETURNING p.proposta_id;

-- name: TogliCopiaAssociazione :many
-- La copia tolta dalla proposta aperta dell'allegato, se dice ancora quel componente: il codice torna la lettura
-- conservata in dettagli.codice_letto (se c'e'), e la chiave esce dai dettagli.
UPDATE documento_proposta p
   SET componente_id = NULL,
       codice = CASE WHEN p.dettagli ? 'codice_letto' THEN p.dettagli ->> 'codice_letto' ELSE p.codice END,
       dettagli = p.dettagli - 'codice_letto'
 WHERE p.allegato_id = sqlc.arg(allegato_id) AND p.stato = 'aperta' AND p.componente_id = sqlc.arg(componente_id)
RETURNING p.proposta_id;

-- name: AllineaCodiceCopieDelComponente :execrows
-- Dopo la ricodifica di un componente (D-ASS-01; oggi anche la correzione del codice delle pagine legacy) le copie di
-- quel componente, in qualunque stato, prendono il codice corrente del componente: la AR non cambia. La FK a tre campi
-- e' DEFERRABLE: chi ricodifica la differisce nella sua transazione (SET CONSTRAINTS fk_proposta_componente DEFERRED).
UPDATE documento_proposta p
   SET codice = c.codice
  FROM componente c
 WHERE c.componente_id = sqlc.arg(componente_id) AND c.thread_id = sqlc.arg(thread_id)
   AND p.componente_id = c.componente_id AND p.codice IS DISTINCT FROM c.codice;

-- name: GetComponenteDellaRfqPerAR :one
-- Il componente della RFQ (codice, tipo, archiviazione) per i controlli del servizio prima della scrittura.
SELECT componente_id, codice, tipo, archiviato_il
  FROM componente
 WHERE componente_id = sqlc.arg(componente_id) AND thread_id = sqlc.arg(thread_id);

-- name: EsisteDocumentoConfermatoAltrove :one
-- Provvisorio, base [T] D-AR07 A: il contenuto dell'allegato e' gia' un documento confermato della RFQ su un altro
-- componente (lo stesso controllo del punto 5 di trg_fra_nasce, per un rifiuto leggibile prima della scrittura).
SELECT EXISTS (SELECT 1 FROM allegato a JOIN documento d ON d.thread_id = sqlc.arg(thread_id) AND d.sha256 = a.sha256
                WHERE a.allegato_id = sqlc.arg(allegato_id) AND a.sha256 IS NOT NULL
                  AND d.componente_id IS NOT NULL AND d.componente_id <> sqlc.arg(componente_id))::boolean AS esiste;

-- name: GetAllegatoPerSorgente :one
-- L'allegato della RFQ con il suo contenuto, per il legame della sorgente (fonte_step_evento vuole lo sha256).
SELECT a.allegato_id, a.sha256, m.thread_id
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE a.allegato_id = sqlc.arg(allegato_id) AND m.thread_id = sqlc.arg(thread_id);

-- name: InsertFonteStepCaricamento :one
-- Provvisorio, base [T] D-AR02 A, NON cablato (INC-S1): il caricamento dello STEP sorgente di un prodotto nella storia della
-- fonte, la fonte autorevole del legame STEP → prodotto proposta dal piano (§4).
INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, proposta_id, motivo, chiave_idempotenza,
                               autore_id)
VALUES (sqlc.arg(thread_id), sqlc.arg(prodotto_id), 'prodotto', 'caricamento', sqlc.arg(allegato_id), sqlc.arg(sha256),
        sqlc.narg(proposta_id), sqlc.narg(motivo), sqlc.arg(chiave_idempotenza), sqlc.arg(autore_id))
RETURNING evento_id;
