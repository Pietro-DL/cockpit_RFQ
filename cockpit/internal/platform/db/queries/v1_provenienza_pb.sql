-- V1, direttiva 39 §4 (D-PB01…04, D-AR11, D-AR14; corsia persistenza, file proprio per INC-G21): le letture che servono
-- alle PB iniziali del server (le proposte univoche del registro d'importazione che diventano PB alla nascita della bozza
-- e nei comandi espliciti), ai suggerimenti dopo l'apertura (preassociazioni_proposte[] di R1 e delle forme del server) e
-- alla conversione delle pre-assegnazioni legacy aperte (P1) alla presa in carico del prodotto. Le chiama solo
-- core/rfq/distintabozza (pbiniziali3.go, conversionep1.go). Solo SELECT, senza lucchetti: chi scrive ha gia' bloccato la
-- RFQ (BloccaThread); R1 le legge nella sua transazione in sola lettura.

-- name: ListAssociazioniDegliAllegati :many
-- Tutte le AR (attive e chiuse) degli allegati dati nella RFQ: una AR attiva dice dove sta il file; una chiusa dice che
-- una persona ha gia' deciso su quel file (svincolo, sostituzione, scollegamento), e il server non lo ripropone da solo.
SELECT r.allegato_id, r.componente_id, (r.stato = 'attiva')::boolean AS attiva
  FROM file_rfq_associazione r
 WHERE r.thread_id = sqlc.arg(thread_id) AND r.allegato_id = ANY (sqlc.arg(allegati)::uuid[])
 ORDER BY r.allegato_id, r.creata_il, r.associazione_id;

-- name: ListPreassociazioniAltreBozze :many
-- Gli allegati preassociati nella versione corrente delle bozze aperte formato 3 degli altri prodotti della RFQ: un file
-- che un altro prodotto preassocia e' conteso, e il server non ne fa una PB (FILE_CONTESO_FRA_PRODOTTI si risolve a mano).
SELECT DISTINCT (d ->> 'allegato_id')::uuid AS allegato_id, b.prodotto_id
  FROM distinta_bozza b
  JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
 CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(v.snapshot -> 'documenti_preassociati') = 'array'
                                              THEN v.snapshot -> 'documenti_preassociati' ELSE '[]'::jsonb END) d
 WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta' AND b.formato = 3
   AND b.prodotto_id <> sqlc.arg(prodotto_id)
 ORDER BY 1, 2;

-- name: ListCopieSenzaARDellaRfq :many
-- Le pre-assegnazioni legacy aperte della RFQ (P1, D-AR14): una proposta di documento aperta con la copia del
-- componente e nessuna AR attiva dell'allegato. Con il tipo del componente, l'estensione dell'allegato, se l'occorrenza
-- e' attiva e si preassocia (le regole dei punti 1 e 4 di trg_fra_nasce) e se e' lo STEP legato come sorgente del finito.
SELECT p.proposta_id, p.allegato_id, p.componente_id::uuid AS componente_id, c.tipo::text AS tipo_componente,
       lower(COALESCE(a.estensione, ''))::text AS estensione,
       (file_rfq_attivo(sqlc.arg(thread_id)::uuid, p.allegato_id)
        AND NOT file_rfq_scartato_legacy(sqlc.arg(thread_id)::uuid, p.allegato_id))::boolean AS attivo,
       (a.natura IS NOT NULL AND a.natura IN ('file', 'elemento_outlook')
        AND lower(COALESCE(a.estensione, '')) NOT IN ('zip', '7z', 'rar')
        AND NOT EXISTS (SELECT 1 FROM allegato v WHERE v.contenitore_id = p.allegato_id))::boolean AS preassociabile,
       allegato_sorgente_del_finito(sqlc.arg(thread_id)::uuid, p.allegato_id)::boolean AS legato_come_sorgente
  FROM documento_proposta p
  JOIN componente c ON c.componente_id = p.componente_id AND c.thread_id = sqlc.arg(thread_id)
  JOIN allegato a ON a.allegato_id = p.allegato_id
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id AND m.thread_id = sqlc.arg(thread_id)
 WHERE p.stato = 'aperta' AND p.componente_id IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM file_rfq_associazione r
                    WHERE r.thread_id = sqlc.arg(thread_id) AND r.allegato_id = p.allegato_id AND r.stato = 'attiva')
 ORDER BY p.allegato_id;

-- name: ListDocumentiConfermatiDegliAllegati :many
-- Il componente del documento confermato della RFQ con lo stesso contenuto degli allegati dati (documento e' UNIQUE
-- (thread_id, sha256)): un file gia' documento confermato non diventa una PB del server (D-AR06, D-AR07; il punto 5 di
-- trg_fra_nasce lo fermerebbe comunque su un altro componente).
SELECT a.allegato_id, d.componente_id::uuid AS componente_id
  FROM allegato a
  JOIN documento d ON d.thread_id = sqlc.arg(thread_id) AND d.sha256 = a.sha256
 WHERE a.allegato_id = ANY (sqlc.arg(allegati)::uuid[]) AND a.sha256 IS NOT NULL AND d.componente_id IS NOT NULL
 ORDER BY a.allegato_id;
