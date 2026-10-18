-- V1, ondata B (giro 5, A1d; direttiva 23): le query delle rotte della bozza che la corsia persistenza scrive al passo 6
-- del piano della 0023 (R1, R2 nella forma salvataggio, R3, R6, R7, R11, la conversione delle bozze formato 2, l'allegato
-- della storia della fonte degli atti esistenti). Le query della 0023 restano in v1_formato3.sql; qui solo letture dello
-- stato della RFQ e gli aggiornamenti di f8_stato di R11.
-- Lo schema non cambia: nessuna migrazione.
--
-- Chi le chiama: core/rfq/confermabom (lo stato della RFQ: firma del perimetro, validita' dell'ultima conferma, versione
-- corrente con la fonte autorizzata), core/rfq/distintabozza (R1, R2, R6, R7, conversione) e transport/web (R11).
--
-- Regole (le stesse di p4_conferme.sql e v1_formato3.sql): il perimetro sta nel WHERE (la RFQ del percorso); le date le
-- scrive il database; le colonne tx non si leggono; nessun ON CONFLICT (T-03).

-- ------------------------------------------------------------------ lo stato della RFQ (firma, validita', versione corrente)

-- name: ListComponentiStatoRfq :many
-- Tutti i componenti della RFQ, archiviati compresi, con le colonne della firma del perimetro (id, codice letterale, tipo,
-- archiviazione, STEP autorizzato) e quelle che la firma non usa ma i confronti dell'identita' registrata si' (rev,
-- descrizione). Dalla tappa A (p4_bozza_v2.sql), rigenerata con sqlc.
SELECT componente_id, codice, rev, descrizione, tipo, archiviato_il, step_strutturale_id
  FROM componente
 WHERE thread_id = sqlc.arg(thread_id)
 ORDER BY componente_id;

-- name: ListArchiStatoRfq :many
-- Tutti gli archi della working della RFQ, con qta e posizione (la posizione non entra nella firma: asse «dati»).
SELECT padre_id, figlio_id, qta, posizione
  FROM componente_relazione
 WHERE thread_id = sqlc.arg(thread_id)
 ORDER BY padre_id, figlio_id;

-- name: ListConfermeStatoRfq :many
-- Le testate delle conferme strutturali di tutti i prodotti della RFQ, con le colonne che decidono la validita' (numero,
-- contatore, firma) e il criterio: senza i JSON grandi.
SELECT conferma_id, prodotto_id, numero, formato, contatore, firma_struttura, mono_pezzo, criterio_verifica
  FROM conferma_strutturale
 WHERE thread_id = sqlc.arg(thread_id)
 ORDER BY prodotto_id, numero;

-- name: ListFontiAutorizzateRfq :many
-- La fonte autorizzata di ogni finito della RFQ che ne ha una (componente.step_strutturale_id): il documento, il suo sha256,
-- il primo allegato da cui e' arrivato e se il documento e' stato sostituito (versione_corrente.fonte_autorizzata di R1).
SELECT c.componente_id, d.documento_id, d.sha256, prov.allegato_id, (d.sostituito_da IS NOT NULL)::boolean AS superata
  FROM componente c
  JOIN documento d ON d.documento_id = c.step_strutturale_id
  LEFT JOIN LATERAL (
        SELECT p.allegato_id FROM documento_provenienza p
         WHERE p.documento_id = d.documento_id AND p.allegato_id IS NOT NULL
         ORDER BY p.ricevuto_il, p.allegato_id
         LIMIT 1) prov ON true
 WHERE c.thread_id = sqlc.arg(thread_id) AND c.step_strutturale_id IS NOT NULL
 ORDER BY c.componente_id;

-- ------------------------------------------------------------------ appartenenza e occorrenze (R1, R2)

-- name: ListAllegatiCitati :many
-- Gli allegati dati, uno per riga anche se non esistono: lo sha256, se sono della RFQ (il legame passa da messaggio.thread_id)
-- e se l'occorrenza e' attiva (file_rfq_attivo, EM §10.8). Servono ai rif scaduti (EM §6.4) e ai riferimenti nuovi o cambiati
-- di un salvataggio (FUORI_RFQ, FILE_NON_ATTIVO).
SELECT x.allegato_id::uuid AS allegato_id, a.sha256, COALESCE(m.thread_id = sqlc.arg(thread_id)::uuid, false)::boolean AS della_rfq,
       file_rfq_attivo(sqlc.arg(thread_id)::uuid, x.allegato_id)::boolean AS attivo
  FROM unnest(sqlc.arg(allegati)::uuid[]) AS x(allegato_id)
  LEFT JOIN allegato a ON a.allegato_id = x.allegato_id
  LEFT JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 ORDER BY 1;

-- name: ListAllegatiAttiviPerSha :many
-- Le occorrenze attive della RFQ con uno sha256 (la conversione delle bozze formato 2, EM §6.7: le caselle si completano con
-- l'unico allegato attivo con quello sha256; piu' d'uno e' conversione_ambigua).
SELECT a.allegato_id
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND a.sha256 = sqlc.arg(sha256)
   AND file_rfq_attivo(sqlc.arg(thread_id)::uuid, a.allegato_id)
 ORDER BY a.allegato_id;

-- name: ListProposteDellaRfq :many
-- Le proposte di documento date che sono della RFQ (preassociazioni e svincoli di un salvataggio: FUORI_RFQ per le altre),
-- con l'allegato, lo stato e il componente.
SELECT p.proposta_id, p.allegato_id, p.componente_id, p.stato
  FROM documento_proposta p
 WHERE p.thread_id = sqlc.arg(thread_id) AND p.proposta_id = ANY (sqlc.arg(proposte)::uuid[])
 ORDER BY p.proposta_id;

-- name: GetClienteDellaRfq :one
-- Il cliente della RFQ: l'emittente del cliente nel formato 3 e' cliente:<cliente_id> (EM §3.4.2, §5.6).
SELECT cliente_id FROM thread_offerta WHERE thread_id = sqlc.arg(thread_id);

-- name: GetVersionePerChiaveGlobale :one
-- La versione salvata con una chiave di salvataggio, in qualunque RFQ (la ricerca della chiave e' globale, EM §9.2): una
-- chiave di un'altra RFQ e' una chiave gia' usata (409 CHIAVE_RIUSATA).
SELECT b.thread_id, v.bozza_id, v.versione, v.impronta, v.impronta_richiesta
  FROM distinta_bozza_versione v
  JOIN distinta_bozza b ON b.bozza_id = v.bozza_id
 WHERE v.chiave_salvataggio = sqlc.arg(chiave_salvataggio);

-- ------------------------------------------------------------------ R6: l'esito di un intento

-- name: BloccaThreadCondiviso :one
-- La riga della RFQ FOR KEY SHARE (R6, EM §9.6): aspetta una R5 o un gesto dei file ancora in corso, che tiene la riga
-- FOR UPDATE fino al COMMIT, senza scrivere niente. Chi chiama imposta prima lock_timeout nella transazione.
SELECT thread_id FROM thread_offerta WHERE thread_id = sqlc.arg(thread_id) FOR KEY SHARE;

-- name: ListRfqDellaChiave :many
-- Le RFQ in cui una chiave d'intento e' stata usata, in qualunque tabella che la registra (esiti, conferme, eventi dei
-- file, eventi delle associazioni): una chiave di un'altra RFQ e' un 422 FUORI_RFQ in R6, mai l'esito di quella RFQ.
SELECT DISTINCT x.thread_id FROM (
    SELECT e.thread_id FROM esito_intento e WHERE e.chiave = sqlc.arg(chiave)
    UNION ALL
    SELECT c.thread_id FROM conferma_strutturale c WHERE c.chiave_idempotenza = sqlc.arg(chiave)
    UNION ALL
    SELECT f.thread_id FROM file_rfq_evento f WHERE f.chiave_idempotenza = sqlc.arg(chiave)
    UNION ALL
    SELECT a.thread_id FROM associazione_evento a WHERE a.chiave_idempotenza = sqlc.arg(chiave)
) x
 ORDER BY x.thread_id;

-- name: GetF8StatoPerChiave :one
-- Lo stato di F8 di un intento della RFQ (R6: f8_stato).
SELECT f8_id, stato, errore, tentativi, risolto_da, aggiornato_il
  FROM f8_stato
 WHERE thread_id = sqlc.arg(thread_id) AND chiave_intento = sqlc.arg(chiave_intento);

-- name: GetF8StatoPerConferma :one
-- Lo stato di F8 di una conferma della RFQ (R6 per un intento oltre la conservazione, R7).
SELECT f8_id, stato, errore, tentativi, risolto_da, aggiornato_il
  FROM f8_stato
 WHERE thread_id = sqlc.arg(thread_id) AND conferma_id = sqlc.arg(conferma_id);

-- name: GetStatiCorrentiFile :one
-- Gli stati di un'occorrenza letti adesso (R6, stati_correnti dei gesti dei file, EM §9.6): lo stato e la versione nella RFQ
-- (attivo e 0 senza riga), l'ultima rimozione e l'ultima copia di conservazione ('' se non ce ne sono: sqlc non vede la
-- nullabilita' dei LEFT JOIN LATERAL, e il COALESCE la toglie).
SELECT COALESCE(f.stato, 'attivo')::text AS stato, COALESCE(f.versione, 0)::int AS versione,
       COALESCE(r.stato, '')::text AS rimozione_stato, COALESCE(r.motivo, '')::text AS rimozione_motivo,
       COALESCE(r.ritentabile, false)::boolean AS rimozione_ritentabile, COALESCE(k.stato, '')::text AS conservazione_stato
  FROM (SELECT sqlc.arg(thread_id)::uuid AS thread_id, sqlc.arg(allegato_id)::uuid AS allegato_id) q
  LEFT JOIN file_rfq f ON f.thread_id = q.thread_id AND f.allegato_id = q.allegato_id
  LEFT JOIN LATERAL (SELECT x.stato, x.motivo, x.ritentabile FROM file_rfq_rimozione x
                      WHERE x.thread_id = q.thread_id AND x.allegato_id = q.allegato_id
                      ORDER BY x.rimozione_id DESC LIMIT 1) r ON true
  LEFT JOIN LATERAL (SELECT y.stato FROM file_rfq_conservazione y
                      WHERE y.thread_id = q.thread_id AND y.allegato_id = q.allegato_id
                      ORDER BY y.conservazione_id DESC LIMIT 1) k ON true;

-- ------------------------------------------------------------------ R7: lo storico

-- name: ListConfermeStoricoProdotto :many
-- Lo storico delle conferme di un prodotto della RFQ, dalla prima (R7, EM §9.7): le testate senza i JSON grandi, con il
-- nome di chi ha confermato (PE-07: confermato_da_nome) e lo stato di F8 della conferma.
SELECT c.conferma_id, c.numero, c.precedente_id, c.formato, c.senza_riepilogo_v2, c.firma_struttura, c.firma_anteprima,
       c.contatore, c.contatore_dati, c.bozza_id, c.bozza_versione, c.fonte_allegato_id, c.fonte_sha256, c.fonte_gruppo,
       c.origine, c.impronta_snapshot, c.mono_pezzo, c.criterio_verifica, c.verifica, c.chiave_idempotenza, c.confermato_da,
       c.confermato_il, c.importazione_id, c.importazioni_citate, u.nome AS confermato_da_nome, u.sigla AS confermato_da_sigla,
       f.stato AS f8_stato
  FROM conferma_strutturale c
  JOIN utente u ON u.utente_id = c.confermato_da
  LEFT JOIN f8_stato f ON f.thread_id = c.thread_id AND f.conferma_id = c.conferma_id
 WHERE c.thread_id = sqlc.arg(thread_id) AND c.prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY c.numero;

-- name: ListImportazioniPerID :many
-- I registri della RFQ dati per id (R7 &importazione=1: tutti i registri citati da una conferma), interi.
SELECT importazione_id, thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole, formato_registro,
       versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da, creata_il
  FROM distinta_importazione
 WHERE thread_id = sqlc.arg(thread_id) AND importazione_id = ANY (sqlc.arg(importazioni)::uuid[])
 ORDER BY creata_il, importazione_id;

-- ------------------------------------------------------------------ R11: il riallineamento di F8

-- name: ListF8DaRiallineare :many
-- Gli stati di F8 della RFQ non ancora eseguiti (in attesa o in errore), creati prima dell'inizio del riallineamento: R11
-- (C3 §1.2, EM §9.8) li porta a eseguito con risolto_da = riallinea, oppure a errore.
SELECT f8_id, chiave_intento, conferma_id, stato, tentativi, creato_il
  FROM f8_stato
 WHERE thread_id = sqlc.arg(thread_id) AND stato <> 'eseguito' AND creato_il <= sqlc.arg(prima_di)
 ORDER BY f8_id;

-- name: SegnaF8Eseguito :execrows
-- Uno stato di F8 portato a eseguito (finale) da R11, con un tentativo in piu'.
UPDATE f8_stato SET stato = 'eseguito', errore = NULL, risolto_da = 'riallinea', tentativi = tentativi + 1
 WHERE f8_id = sqlc.arg(f8_id) AND thread_id = sqlc.arg(thread_id) AND stato <> 'eseguito';

-- name: SegnaF8Errore :execrows
-- Uno stato di F8 in errore dopo un riallineamento non riuscito, con il testo e un tentativo in piu'.
UPDATE f8_stato SET stato = 'errore', errore = sqlc.arg(errore), tentativi = tentativi + 1
 WHERE f8_id = sqlc.arg(f8_id) AND thread_id = sqlc.arg(thread_id) AND stato <> 'eseguito';

-- ------------------------------------------------------------------ la conversione delle bozze formato 2 (EM §6.7)

-- name: InsertDistintaBozzaConID :one
-- La testata della bozza formato 3 nata da una conversione, con l'id scelto da chi converte: la bozza formato 2 si chiude
-- nella stessa transazione, prima, con motivo_chiusura «convertita nel formato 3 nella bozza <id nuovo>» (EM §6.7), e la
-- testata nuova nasce dopo (una sola bozza aperta per prodotto). Per il resto e' InsertDistintaBozza.
INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore, firma_base,
                            fonte_allegato_id, fonte_sha256, fonte_gruppo, creata_da, aggiornata_da)
VALUES (sqlc.arg(bozza_id), sqlc.arg(thread_id), sqlc.arg(prodotto_id), sqlc.arg(formato), sqlc.narg(base_conferma_id), sqlc.arg(base_numero),
        sqlc.arg(base_contatore), sqlc.arg(firma_base), sqlc.narg(fonte_allegato_id), sqlc.narg(fonte_sha256),
        sqlc.narg(fonte_gruppo), sqlc.arg(utente_id), sqlc.arg(utente_id))
RETURNING bozza_id, creata_il;

-- ------------------------------------------------------------------ la storia della fonte degli atti esistenti (PM23 R-24)

-- name: GetAllegatoDelDocumento :one
-- Il primo allegato da cui e' arrivato un documento della RFQ: l'allegato della riga di fonte_step_evento che «Autorizza»,
-- «Revoca» e «Conferma la fonte» scrivono in V1 (EM §2.3, §9.10), quando la dichiarazione non porta il suo allegato.
SELECT p.allegato_id::uuid AS allegato_id
  FROM documento_provenienza p
  JOIN documento d ON d.documento_id = p.documento_id
 WHERE d.thread_id = sqlc.arg(thread_id) AND p.documento_id = sqlc.arg(documento_id) AND p.allegato_id IS NOT NULL
 ORDER BY p.ricevuto_il, p.allegato_id
 LIMIT 1;
