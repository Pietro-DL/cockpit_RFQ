-- V1, ondata C, corsia persistenza: impronta-ingressi-1 (U2 §6.3.3), dalla direttiva 35 (blocco C-1c) impronta-ingressi-2
-- (la parte proposta con la regola unica di cio' che R5 scrive), le parti degli ingressi del riepilogo che R1
-- ?ingressi=1 legge per la notifica alle sessioni con il wizard aperto, e che R5 rilegge per il controllo finale
-- (strategia A, §6.3.5: le parti proposta e analisi). Un'istruzione sola per parte: vede una sola istantanea anche in
-- READ COMMITTED, e non legge i fatti interi (il digest e' una colonna di analisi_fatti, 0023 riga 10). Non entrano
-- orari, id d'interpretazione, l'impronta della fotografia: solo il contenuto. Solo SELECT, senza lucchetti.

-- name: IngressiBom :one
-- La parte bom (per RFQ): working_contatore, l'ultima conferma di ogni prodotto, step_strutturale_id dei finiti, l'ultimo
-- evento della storia della fonte; con i valori del prodotto chiesto, e il digest di tutto.
SELECT COALESCE((SELECT w.struttura FROM working_contatore w WHERE w.thread_id = sqlc.arg(thread_id)::uuid), 0)::bigint AS contatore_struttura,
       COALESCE((SELECT w.dati FROM working_contatore w WHERE w.thread_id = sqlc.arg(thread_id)::uuid), 0)::bigint AS contatore_dati,
       COALESCE((SELECT c.conferma_id::text FROM conferma_strutturale c
                  WHERE c.thread_id = sqlc.arg(thread_id)::uuid AND c.prodotto_id = sqlc.arg(prodotto_id)::uuid
                  ORDER BY c.numero DESC LIMIT 1), '')::text AS ultima_conferma_id,
       COALESCE((SELECT k.step_strutturale_id::text FROM componente k
                  WHERE k.thread_id = sqlc.arg(thread_id)::uuid AND k.componente_id = sqlc.arg(prodotto_id)::uuid), '')::text AS step_strutturale_id,
       COALESCE((SELECT max(e.evento_id) FROM fonte_step_evento e WHERE e.thread_id = sqlc.arg(thread_id)::uuid), 0)::bigint AS fonte_evento_max,
       encode(sha256(convert_to(concat_ws('|',
           'bom-1',
           (SELECT w.struttura::text || ':' || w.dati::text FROM working_contatore w WHERE w.thread_id = sqlc.arg(thread_id)::uuid),
           (SELECT string_agg(u.prodotto_id::text || ':' || u.conferma_id::text, ',' ORDER BY u.prodotto_id)
              FROM (SELECT DISTINCT ON (c.prodotto_id) c.prodotto_id, c.conferma_id FROM conferma_strutturale c
                     WHERE c.thread_id = sqlc.arg(thread_id)::uuid ORDER BY c.prodotto_id, c.numero DESC) u),
           (SELECT string_agg(k.componente_id::text || ':' || COALESCE(k.step_strutturale_id::text, ''), ',' ORDER BY k.componente_id)
              FROM componente k WHERE k.thread_id = sqlc.arg(thread_id)::uuid AND k.tipo = 'finito'),
           (SELECT max(e.evento_id)::text FROM fonte_step_evento e WHERE e.thread_id = sqlc.arg(thread_id)::uuid)), 'UTF8')), 'hex')::text AS impronta;

-- name: IngressiBozza :many
-- La parte bozza: la testata della bozza aperta del prodotto (zero o una riga).
SELECT b.bozza_id, b.versione, b.aggiornata_da, b.aggiornata_il
  FROM distinta_bozza b
 WHERE b.thread_id = sqlc.arg(thread_id)::uuid AND b.prodotto_id = sqlc.arg(prodotto_id)::uuid AND b.stato = 'aperta';

-- name: IngressiProposta :one
-- La parte proposta (per RFQ): il contenuto delle righe di componente_proposta, relazione_proposta, rimozione_proposta e
-- documento_proposta; i fatti degli allegati attivi con la chiave completa e il digest; lo stato e la versione di file_rfq;
-- l'insieme degli allegati attivi; le deroghe di struttura e di fabbisogno; i documenti. Le righe riscritte sul posto
-- (UpsertComponenteProposta) cambiano il contenuto, non una colonna «aggiornato»: per questo entra il contenuto.
--
-- impronta-ingressi-2 (direttiva 35, blocco C-1c; PD35 §6.2, INC-G12): la regola unica [T] di cio' che R5 scrive.
--   (1) Fuori le colonne di copia di documento_proposta: niente componente_id, e per una proposta con la copia al posto del
--       codice (che e' quello del componente) entra la lettura conservata in dettagli.codice_letto (il codice, se la lettura
--       non c'e': una copia legacy). Cosi' la copia scritta o tolta da R5 non cambia la parte.
--   (2) Fuori le righe di componente_proposta, relazione_proposta che R5 segna: quelle con il segno di R5 (evidenza.albero con
--       formato 3 e gesto conferma_bom) e quelle dell'elenco che R5 passa prima di scriverle (escludi_proposte: proposta_id;
--       escludi_archi: «allegato|padre|figlio»), cosi' H_S e H_T le escludono tutte e due (CN-05b). R1 passa elenchi vuoti.
--   (2-bis) Direttiva 39, R5 formato 3: fuori anche i documenti che R5 marca sostituiti (escludi_documenti: lo STEP A della
--       working sostituito da B dopo la promozione) e le rimozioni degli STEP di cui R5 chiude le rimozioni aperte
--       (escludi_rimozioni: step_documento_id di A e delle marcature dei componenti che R5 archivia). Sono scritture di R5,
--       non cambiamenti esterni: H_S e H_T le escludono tutte e due. R1 passa elenchi vuoti.
--   (2-ter) Blocco «R5 completo»: per le copie legacy senza lettura conservata dei componenti che R5 ricodifica
--       (escludi_codice_componenti) il codice e' la copia che AllineaCodiceCopia riscrive: non entra (un segnaposto fisso).
--       R1 passa un elenco vuoto.
--   (3) Dentro le AR degli allegati che la bozza del prodotto cita (PB e svincoli scollega_documento della versione corrente
--       della bozza aperta formato 3, oppure l'elenco allegati_citati di R5 con citati_dalla_bozza falso), viste come prima
--       della transazione corrente: una AR nata in questa transazione non c'e', una chiusa in questa transazione e' ancora
--       attiva. La R5 di un altro prodotto cambia la parte solo se tocca gli stessi file (FILE_CONTESO_FRA_PRODOTTI).
WITH att AS (
    SELECT a.allegato_id, a.sha256
      FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
     WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND file_rfq_attivo(m.thread_id, a.allegato_id)
       AND NOT file_rfq_scartato_legacy(m.thread_id, a.allegato_id)),
citati AS (
    SELECT x::text AS allegato
      FROM unnest(COALESCE(sqlc.arg(allegati_citati)::uuid[], '{}'::uuid[])) AS x
     WHERE NOT sqlc.arg(citati_dalla_bozza)::boolean
    UNION
    SELECT e ->> 'allegato_id'
      FROM distinta_bozza b
      JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
     CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(v.snapshot -> 'documenti_preassociati') = 'array'
                                                  THEN v.snapshot -> 'documenti_preassociati' ELSE '[]'::jsonb END) e
     WHERE sqlc.arg(citati_dalla_bozza)::boolean AND b.thread_id = sqlc.arg(thread_id)::uuid
       AND b.prodotto_id = sqlc.arg(prodotto_id)::uuid AND b.stato = 'aperta' AND b.formato = 3
    UNION
    SELECT e ->> 'allegato_id'
      FROM distinta_bozza b
      JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
     CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(v.snapshot -> 'decisioni') = 'array'
                                                  THEN v.snapshot -> 'decisioni' ELSE '[]'::jsonb END) e
     WHERE sqlc.arg(citati_dalla_bozza)::boolean AND b.thread_id = sqlc.arg(thread_id)::uuid
       AND b.prodotto_id = sqlc.arg(prodotto_id)::uuid AND b.stato = 'aperta' AND b.formato = 3
       AND e ->> 'tipo' = 'scollega_documento')
SELECT encode(sha256(convert_to(concat_ws('|',
    'proposta-2',
    (SELECT string_agg(concat_ws('~', p.allegato_id, p.chiave, p.codice, p.rev, p.descrizione, p.tipo_proposto, p.evidenza::text, p.stato,
                                 p.componente_id, p.deciso_da), ',' ORDER BY p.allegato_id, p.chiave)
       FROM componente_proposta p
      WHERE p.thread_id = sqlc.arg(thread_id)::uuid
        AND NOT (p.proposta_id = ANY (COALESCE(sqlc.arg(escludi_proposte)::uuid[], '{}'::uuid[])))
        AND NOT COALESCE(p.evidenza -> 'albero' ->> 'formato' = '3' AND p.evidenza -> 'albero' ->> 'gesto' = 'conferma_bom', false)),
    (SELECT string_agg(concat_ws('~', r.allegato_id, r.padre_chiave, r.figlio_chiave, r.qta, r.evidenza::text, r.stato, r.deciso_da), ','
                       ORDER BY r.allegato_id, r.padre_chiave, r.figlio_chiave)
       FROM relazione_proposta r
      WHERE r.thread_id = sqlc.arg(thread_id)::uuid
        AND NOT (concat_ws('|', r.allegato_id, r.padre_chiave, r.figlio_chiave) = ANY (COALESCE(sqlc.arg(escludi_archi)::text[], '{}'::text[])))
        AND NOT COALESCE(r.evidenza -> 'albero' ->> 'formato' = '3' AND r.evidenza -> 'albero' ->> 'gesto' = 'conferma_bom', false)),
    (SELECT string_agg(concat_ws('~', x.step_documento_id, x.padre_id, x.figlio_id, x.qta_working, x.stato, x.deciso_da), ','
                       ORDER BY x.step_documento_id, x.padre_id, x.figlio_id)
       FROM rimozione_proposta x WHERE x.thread_id = sqlc.arg(thread_id)::uuid
        AND NOT (x.step_documento_id = ANY (COALESCE(sqlc.arg(escludi_rimozioni)::uuid[], '{}'::uuid[])))),
    (SELECT string_agg(concat_ws('~', d.proposta_id, d.allegato_id, d.stato,
                                 CASE WHEN d.componente_id IS NULL THEN d.codice
                                      WHEN d.dettagli ? 'codice_letto' THEN d.dettagli ->> 'codice_letto'
                                      WHEN d.componente_id = ANY (COALESCE(sqlc.arg(escludi_codice_componenti)::uuid[], '{}'::uuid[])) THEN '~copia'
                                      ELSE d.codice END,
                                 d.rev, d.tipo_proposto, d.deciso_da), ','
                       ORDER BY d.proposta_id)
       FROM documento_proposta d WHERE d.thread_id = sqlc.arg(thread_id)::uuid),
    (SELECT string_agg(concat_ws('~', f.sha256, f.versione_analizzatore, f.hash_configurazione, f.digest), ','
                       ORDER BY f.sha256, f.versione_analizzatore, f.hash_configurazione)
       FROM analisi_fatti f WHERE f.sha256 IN (SELECT att.sha256 FROM att WHERE att.sha256 IS NOT NULL)),
    (SELECT string_agg(concat_ws('~', q.allegato_id, q.stato, q.versione), ',' ORDER BY q.allegato_id)
       FROM file_rfq q WHERE q.thread_id = sqlc.arg(thread_id)::uuid),
    (SELECT string_agg(att.allegato_id::text, ',' ORDER BY att.allegato_id) FROM att),
    (SELECT string_agg(concat_ws('~', s.deroga_struttura_id, s.componente_id, s.step_documento_id, s.analisi_calcolata_il, s.motivo), ','
                       ORDER BY s.deroga_struttura_id)
       FROM deroga_struttura s WHERE s.thread_id = sqlc.arg(thread_id)::uuid),
    (SELECT string_agg(concat_ws('~', g.deroga_id, g.componente_id, g.tipo, g.motivo), ',' ORDER BY g.deroga_id)
       FROM deroga_fabbisogno g WHERE g.thread_id = sqlc.arg(thread_id)::uuid),
    (SELECT string_agg(concat_ws('~', o.documento_id, o.componente_id, o.tipo, o.codice, o.rev, o.sha256, o.sostituito_da), ','
                       ORDER BY o.documento_id)
       FROM documento o WHERE o.thread_id = sqlc.arg(thread_id)::uuid
        AND NOT (o.documento_id = ANY (COALESCE(sqlc.arg(escludi_documenti)::uuid[], '{}'::uuid[])))),
    (SELECT 'ar:' || COALESCE(string_agg(concat_ws('~', r.associazione_id, r.allegato_id, r.componente_id,
                                                   CASE WHEN r.chiusa_tx = pg_current_xact_id_if_assigned() THEN 'attiva' ELSE r.stato END),
                                         ',' ORDER BY r.associazione_id), '')
       FROM file_rfq_associazione r
      WHERE r.thread_id = sqlc.arg(thread_id)::uuid AND r.allegato_id::text IN (SELECT c.allegato FROM citati c)
        AND r.tx IS DISTINCT FROM pg_current_xact_id_if_assigned())), 'UTF8')), 'hex')::text AS impronta;

-- name: IngressiAnalisi :many
-- La parte analisi: gli allegati della RFQ con un lavoro pendente (download, estrazione, analisi per contenuto), con il
-- tipo del lavoro; il Go ne ricava lo stato e il digest dell'insieme.
SELECT a.allegato_id, a.nome_file::text AS nome_file, j.tipo::text AS tipo
  FROM job j
  JOIN allegato a ON (j.tipo IN ('stage_allegato', 'estrai_archivio') AND j.payload ->> 'allegato_id' = a.allegato_id::text)
                  OR (j.tipo = 'analizza_allegato' AND a.sha256 IS NOT NULL AND j.payload ->> 'sha256' = a.sha256::text)
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE j.stato IN ('pronto', 'in_corso') AND m.thread_id = sqlc.arg(thread_id)::uuid
 ORDER BY a.allegato_id, j.tipo;
