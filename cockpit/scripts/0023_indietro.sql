-- 0023_indietro.sql — ritorno MANUALE dallo schema 23 allo schema 22 (V1 della Distinta unica: formato 3, registro
-- d'importazione, nodi non inclusi, ciclo dei file).
--
-- Non e' una migrazione: il migratore non ha il verso «giu'» e questo file non va MAI in cockpit/migrations/. Si
-- lancia a mano, in una transazione, sul database che si vuole riportare indietro, con il server e i worker FERMI;
-- poi si riavvia il binario della 22: un cockpit.exe che conosce solo la 22 rifiuta di partire su un database alla 23
-- («aggiornare cockpit.exe»).
--
-- A che cosa serve. La 0023 si applica solo ai database di prova finche' l'utente non da' il suo OK. Se un database
-- sbagliato prende la 23, questo file lo riporta alla 22 senza perdere niente della 22. Il ritorno vero resta il
-- backup preso subito prima della migrazione (scripts/backup-db.ps1).
--
-- Si rifiuta, senza cambiare niente, se:
--   - lo schema non e' alla 23;
--   - c'e' una riga formato 3 (bozza, versione o conferma);
--   - c'e' una riga in una delle nove tabelle nuove (distinta_importazione, conferma_strutturale_non_incluso,
--     file_rfq, file_rfq_evento, file_rfq_rimozione, file_rfq_conservazione, proposta_aggiornata_rifiuto,
--     file_rfq_associazione, rfq_formato3_segno);
--   - c'e' un valore in una delle otto colonne nuove delle tabelle della 22 di formato 3 e identita';
--   - c'e' un esito d'intento con un gesto che la 22 non conosce;
--   - un evento di associazione o una correzione porta un valore nelle cinque colonne della 23 della direttiva 35
--     (allegato, riga della AR, riga sostituita, transazione dell'evento; riga della AR della correzione), o un evento
--     di associazione e' senza autore (conversione_legacy, direttiva 39);
--   - la storia della fonte ha una revoca della sorgente (azione revoca_sorgente, direttiva 39).
-- Tornare indietro in quei casi perderebbe dati della 23: si ferma.
--
-- Fine riga. Le cinque funzioni e i cinque commenti si rimettono con il testo della 22 e con i fine riga di questo file
-- com'e' sul disco. Il binario che ha applicato la 0022 puo' averla incorporata con altri fine riga (un checkout con
-- core.autocrlf scrive CRLF nei file presi dal repository, LF in quelli nati sul posto): allora il catalogo torna quello
-- della 22 a meno dei CR nei corpi delle cinque funzioni e nei cinque commenti, con lo stesso comportamento. La prova U-09
-- esegue il file com'e' sul disco e confronta il catalogo con quello della 22 a meno dei CR.
--
-- Poi toglie colonne, tabelle, funzioni e trigger della 0023 (il digest dei fatti si ricalcola: non e' un dato), rimette
-- i vincoli, le cinque funzioni, le cinque viste e i cinque commenti com'erano alla 22 (testi copiati dalla 0018, dalla 0020 e dalla 0022), e toglie le righe 23 di
-- schema_versione e schema_variante. La prova U-09 (platform/migrazioni) lo esegue su 22 → 23 → 22 e confronta il
-- catalogo con quello della 22.

BEGIN;

DO $$
DECLARE
    v integer;
    n bigint;
BEGIN
    SELECT max(versione) INTO v FROM schema_versione;
    IF v IS DISTINCT FROM 23 THEN
        RAISE EXCEPTION '0023 indietro: lo schema e'' alla versione %, non alla 23', v;
    END IF;
    SELECT (SELECT count(*) FROM distinta_bozza WHERE formato = 3)
         + (SELECT count(*) FROM distinta_bozza_versione WHERE snapshot ->> 'formato' = '3')
         + (SELECT count(*) FROM conferma_strutturale WHERE formato = 3) INTO n;
    IF n > 0 THEN
        RAISE EXCEPTION '0023 indietro: ci sono % righe formato 3 (bozze, versioni o conferme), e tornando alla 22 si perdono', n
            USING HINT = 'Il ritorno vale solo per un database che ha preso la 23 per sbaglio, senza lavoro formato 3.';
    END IF;
    SELECT (SELECT count(*) FROM distinta_importazione) + (SELECT count(*) FROM conferma_strutturale_non_incluso)
         + (SELECT count(*) FROM file_rfq) + (SELECT count(*) FROM file_rfq_evento)
         + (SELECT count(*) FROM file_rfq_rimozione) + (SELECT count(*) FROM file_rfq_conservazione)
         + (SELECT count(*) FROM proposta_aggiornata_rifiuto) + (SELECT count(*) FROM file_rfq_associazione)
         + (SELECT count(*) FROM rfq_formato3_segno) INTO n;
    IF n > 0 THEN
        RAISE EXCEPTION '0023 indietro: le tabelle della 23 hanno % righe (registri d''importazione, nodi non inclusi, ciclo dei file o rifiuti delle proposte aggiornate), e tornando alla 22 si perdono', n;
    END IF;
    SELECT (SELECT count(*) FROM distinta_bozza_versione
             WHERE importazione_id IS NOT NULL OR chiave_salvataggio IS NOT NULL OR impronta_richiesta IS NOT NULL)
         + (SELECT count(*) FROM conferma_strutturale WHERE importazione_id IS NOT NULL OR importazioni_citate IS NOT NULL)
         + (SELECT count(*) FROM conferma_strutturale_componente
             WHERE emittente IS NOT NULL OR emittente_presunto IS NOT NULL OR namespace IS NOT NULL) INTO n;
    IF n > 0 THEN
        RAISE EXCEPTION '0023 indietro: % righe hanno un valore nelle colonne della 23, e tornando alla 22 si perdono', n;
    END IF;
    SELECT count(*) INTO n FROM esito_intento
     WHERE gesto NOT IN ('conferma_bom_v2', 'lotto_fascicolo', 'correggi_documento', 'fonte_step', 'riprova_copia');
    IF n > 0 THEN
        RAISE EXCEPTION '0023 indietro: % esiti d''intento hanno un gesto della 23, e la 22 non li ammette', n;
    END IF;
    SELECT (SELECT count(*) FROM associazione_evento
             WHERE allegato_id IS NOT NULL OR associazione_id IS NOT NULL OR associazione_precedente_id IS NOT NULL OR tx IS NOT NULL
                OR autore_id IS NULL OR origine = 'conversione_legacy')
         + (SELECT count(*) FROM correzione_documento WHERE associazione_id IS NOT NULL) INTO n;
    IF n > 0 THEN
        RAISE EXCEPTION '0023 indietro: % eventi di associazione o correzioni hanno un valore nelle colonne della 23 (direttiva 35), e tornando alla 22 si perdono', n;
    END IF;
    SELECT count(*) INTO n FROM fonte_step_evento WHERE azione = 'revoca_sorgente';
    IF n > 0 THEN
        RAISE EXCEPTION '0023 indietro: % eventi della fonte sono revoche della sorgente (azione della 23), e la 22 non li ammette', n;
    END IF;
END $$;

-- ------------------------------------------------------------ le cinque viste com'erano alla 22 (prima delle funzioni:
-- usano file_rfq_attivo)
CREATE OR REPLACE VIEW v_fascicolo AS
SELECT co.thread_id, co.componente_id, co.codice, co.rev, co.tipo AS tipo_componente,
       f.tipo AS tipo_documento, f.bloccante,
       d.documento_id, d.stato_nas, d.path_relativo,
       p.proposta_id AS proposta_aperta,
       rp.rif_id     AS atteso_da_portale,
       dg.deroga_id,
       CASE WHEN d.documento_id IS NOT NULL AND (d.stato_nas = 'errore' OR an.anomalia_id IS NOT NULL) THEN 'ok_errore_nas'
            WHEN d.documento_id IS NOT NULL AND d.stato_nas = 'in_coda' THEN 'ok_in_coda'
            WHEN d.documento_id IS NOT NULL THEN 'ok'
            WHEN dg.deroga_id  IS NOT NULL THEN 'derogato'
            WHEN p.proposta_id IS NOT NULL THEN 'da_confermare'
            WHEN rp.rif_id     IS NOT NULL THEN 'sul_portale'
            ELSE 'manca' END AS esito,
       f.fonte_attesa,
       p.n AS n_proposte_aperte,
       d.rev AS documento_rev,
       (nullif(btrim(d.rev), '') IS NOT NULL AND nullif(btrim(co.rev), '') IS NOT NULL
        AND upper(btrim(d.rev)) <> upper(btrim(co.rev)) AND d.copertura_id IS NULL) AS rev_diversa,
       an.anomalia_id,
       d.copertura_id
FROM componente co
JOIN thread_offerta t ON t.thread_id = co.thread_id
JOIN LATERAL (
    SELECT x.tipo, x.bloccante, x.fonte_attesa FROM fabbisogno_documento x
    WHERE x.tipo_componente = co.tipo
      AND x.cliente_id IS NOT DISTINCT FROM (
            SELECT y.cliente_id FROM fabbisogno_documento y
            WHERE y.tipo_componente = co.tipo AND (y.cliente_id = t.cliente_id OR y.cliente_id IS NULL)
            ORDER BY (y.cliente_id IS NOT NULL) DESC LIMIT 1)
) f ON true
LEFT JOIN LATERAL (
    SELECT u.documento_id, u.stato_nas, u.path_relativo, u.rev, u.copertura_id FROM (
        SELECT x.documento_id, x.stato_nas, x.path_relativo, x.rev, NULL::uuid AS copertura_id, 0 AS ordine, x.confermato_il AS da
          FROM documento x
         WHERE x.componente_id = co.componente_id AND x.tipo = f.tipo AND x.sostituito_da IS NULL
        UNION ALL
        SELECT x.documento_id, x.stato_nas, x.path_relativo, x.rev, c.copertura_id, 1, c.quando
          FROM copertura_documento c
          JOIN documento x ON x.documento_id = c.documento_id
          JOIN componente o ON o.componente_id = x.componente_id AND o.archiviato_il IS NULL
         WHERE c.componente_id = co.componente_id AND c.tipo = f.tipo AND c.azione IN ('copre', 'rettifica')
           AND NOT EXISTS (SELECT 1 FROM copertura_documento s WHERE s.precedente_id = c.copertura_id)
           AND x.sostituito_da IS NULL AND x.sha256 = c.sha256
           AND x.componente_id = c.documento_componente AND x.tipo = c.tipo
           AND x.rev IS NOT DISTINCT FROM c.rev_documento AND co.rev IS NOT DISTINCT FROM c.rev_componente
           AND EXISTS (
               WITH RECURSIVE giu (id) AS (
                   SELECT c.documento_componente
                   UNION
                   SELECT r.figlio_id
                     FROM giu g
                     JOIN componente_relazione r ON r.padre_id = g.id
                     JOIN componente k ON k.componente_id = r.figlio_id AND k.archiviato_il IS NULL)
               SELECT 1 FROM giu WHERE giu.id = co.componente_id)
    ) u ORDER BY u.ordine, u.da DESC LIMIT 1) d ON true
LEFT JOIN LATERAL (
    SELECT a.anomalia_id FROM nas_anomalia a
    WHERE a.documento_id = d.documento_id AND a.risolta_il IS NULL) an ON true
LEFT JOIN LATERAL (
    SELECT (array_agg(p.proposta_id ORDER BY p.creato_il))[1] AS proposta_id, count(*) AS n
    FROM documento_proposta p
    WHERE p.thread_id = co.thread_id AND p.stato = 'aperta' AND p.tipo_proposto = f.tipo
      AND (p.componente_id = co.componente_id OR (p.componente_id IS NULL AND upper(p.codice) = upper(co.codice)))
) p ON true
LEFT JOIN LATERAL (
    SELECT r.rif_id FROM riferimento_portale r
    WHERE r.thread_id = co.thread_id AND upper(r.codice) = upper(co.codice) AND r.stato = 'da_scaricare'
      AND (r.tipo_atteso IS NULL OR r.tipo_atteso = f.tipo)
    LIMIT 1) rp ON true
LEFT JOIN deroga_fabbisogno dg ON dg.componente_id = co.componente_id AND dg.tipo = f.tipo
WHERE co.archiviato_il IS NULL;

CREATE OR REPLACE VIEW v_step_prodotto AS
SELECT co.thread_id, co.componente_id, co.codice, co.step_strutturale_id,
       st.n_step_correnti,
       (af.sha256 IS NOT NULL AND m.motivo IS NULL) AS analisi_completa,
       CASE WHEN rf.documento_id IS NULL OR rf.sostituito_da IS NOT NULL THEN NULL
            WHEN af.sha256 IS NULL THEN 'nessuna analisi corrente dello STEP'
            ELSE m.motivo END::text AS motivo_parziale,
       dg.deroga_struttura_id,
       a3.documento_id AS altro_3d_documento_id,
       pr.proposta_id  AS proposta_aperta,
       rp.rif_id       AS atteso_da_portale,
       CASE WHEN co.tipo <> 'finito'                                        THEN 'radice_senza_qualifica'
            WHEN rf.documento_id IS NOT NULL AND rf.sostituito_da IS NOT NULL THEN 'riferimento_superato'
            WHEN rf.documento_id IS NOT NULL AND af.sha256 IS NULL          THEN 'presente_non_analizzato'
            WHEN rf.documento_id IS NOT NULL AND m.motivo IS NULL           THEN 'presente_analizzato'
            WHEN rf.documento_id IS NOT NULL                                THEN 'presente_parziale'
            WHEN st.n_step_correnti > 0                                     THEN 'da_scegliere'
            WHEN a3.documento_id IS NOT NULL                                THEN 'solo_altro_3d'
            WHEN pr.proposta_id IS NOT NULL                                 THEN 'da_confermare'
            WHEN rp.rif_id IS NOT NULL                                      THEN 'sul_portale'
            ELSE 'mancante' END AS esito
  FROM componente co
  LEFT JOIN analizzatore_corrente ac ON true
  LEFT JOIN documento rf ON rf.documento_id = co.step_strutturale_id
  LEFT JOIN analisi_fatti af ON rf.sostituito_da IS NULL AND af.sha256 = rf.sha256
        AND af.versione_analizzatore = ac.versione_analizzatore AND af.hash_configurazione = ac.hash_configurazione
  CROSS JOIN LATERAL (SELECT struttura_motivo_parziale(af.fatti -> 'struttura') AS motivo) m
  CROSS JOIN LATERAL (
        SELECT count(*)::int AS n_step_correnti FROM documento s
         WHERE s.componente_id = co.componente_id AND s.sostituito_da IS NULL
           AND s.tipo = 'cad_3d' AND lower(s.estensione) IN ('stp', 'step')) st
  LEFT JOIN LATERAL (
        SELECT o.documento_id FROM documento o
         WHERE o.componente_id = co.componente_id AND o.sostituito_da IS NULL
           AND o.tipo = 'cad_3d' AND lower(o.estensione) NOT IN ('stp', 'step')
         ORDER BY o.confermato_il DESC LIMIT 1) a3 ON true
  LEFT JOIN LATERAL (
        SELECT dp.proposta_id FROM documento_proposta dp
         WHERE dp.thread_id = co.thread_id AND dp.stato = 'aperta' AND dp.tipo_proposto = 'cad_3d'
           AND (dp.componente_id = co.componente_id OR (dp.componente_id IS NULL AND upper(dp.codice) = upper(co.codice)))
         ORDER BY dp.creato_il LIMIT 1) pr ON true
  LEFT JOIN LATERAL (
        SELECT r.rif_id FROM riferimento_portale r
         WHERE r.thread_id = co.thread_id AND upper(r.codice) = upper(co.codice) AND r.stato = 'da_scaricare'
           AND (r.tipo_atteso IS NULL OR r.tipo_atteso = 'cad_3d')
         LIMIT 1) rp ON true
  LEFT JOIN LATERAL (
        SELECT ds.deroga_struttura_id FROM deroga_struttura ds
         WHERE ds.componente_id = co.componente_id
           AND ds.step_documento_id = rf.documento_id AND rf.sostituito_da IS NULL
           AND CASE WHEN af.sha256 IS NULL THEN ds.versione_analizzatore IS NULL
                    ELSE ds.versione_analizzatore = af.versione_analizzatore
                     AND ds.hash_configurazione = af.hash_configurazione
                     AND ds.analisi_calcolata_il = af.calcolato_il END
         ORDER BY ds.concessa_il DESC LIMIT 1) dg ON true
 WHERE co.archiviato_il IS NULL
   AND (co.tipo = 'finito'
        OR NOT EXISTS (SELECT 1 FROM componente_relazione cr JOIN componente cp ON cp.componente_id = cr.padre_id
                        WHERE cr.figlio_id = co.componente_id AND cp.archiviato_il IS NULL));

CREATE OR REPLACE VIEW v_thread_da_riesaminare AS
WITH ultima AS (
    SELECT DISTINCT ON (b.thread_id) b.thread_id, b.bom_versione_id, b.numero, b.contesto, b.fase_all_apertura, b.congelata_il
      FROM bom_versione b
     WHERE b.stato = 'congelata'
     ORDER BY b.thread_id, b.numero DESC
), scheda AS (
    SELECT DISTINCT ON (f.thread_id) f.thread_id, f.bom_versione_id, v.numero
      FROM fase_log f
      JOIN bom_versione v ON v.bom_versione_id = f.bom_versione_id
     WHERE f.nome_fase = 'SCHEDA_COSTO' AND f.bom_versione_id IS NOT NULL
     ORDER BY f.thread_id, f.inizio DESC, f.fase_log_id
)
SELECT u.thread_id,
       CASE WHEN u.contesto = 'tecnica' AND u.fase_all_apertura IN ('ORDINE', 'PRODUZIONE') THEN 'revisione_tecnica_dopo_ordine'
            WHEN u.contesto = 'tecnica' THEN 'revisione_tecnica_prima_ordine'
            ELSE 'baseline_nuova' END AS tipo_motivo,
       CASE WHEN u.contesto = 'tecnica' AND u.fase_all_apertura IN ('ORDINE', 'PRODUZIONE')
                 THEN format('revisione tecnica dopo l''ordine: la Scheda Costo poggia su V%s, l''ultima baseline e'' V%s', s.numero, u.numero)
            WHEN u.contesto = 'tecnica'
                 THEN format('revisione tecnica prima dell''ordine: l''offerta accettata poggia su V%s, l''ultima baseline e'' V%s', s.numero, u.numero)
            ELSE format('la Scheda Costo poggia su V%s, l''ultima baseline e'' V%s', s.numero, u.numero) END AS motivo,
       u.bom_versione_id AS ultima_congelata_id, u.numero AS ultimo_numero, s.numero AS numero_riferito,
       0::bigint AS n_file, 0::bigint AS n_proposte
  FROM ultima u
  JOIN scheda s ON s.thread_id = u.thread_id
 WHERE s.bom_versione_id <> u.bom_versione_id
UNION ALL
SELECT u.thread_id, 'evidenze_nuove',
       format('evidenze tecniche nuove dopo V%s: %s file, %s proposte', u.numero, e.n_file, e.n_proposte),
       u.bom_versione_id, u.numero, NULL::int, e.n_file, e.n_proposte
  FROM ultima u
  CROSS JOIN LATERAL (
        SELECT (SELECT count(*) FROM documento d
                 WHERE d.thread_id = u.thread_id AND d.tipo IN ('cad_3d', 'disegno_2d', 'sviluppo_dxf')
                   AND d.confermato_il > u.congelata_il)
             + (SELECT count(*) FROM documento_proposta p
                 WHERE p.thread_id = u.thread_id AND p.stato = 'aperta' AND p.tipo_proposto IN ('cad_3d', 'disegno_2d', 'sviluppo_dxf')
                   AND p.creato_il > u.congelata_il) AS n_file,
               (SELECT count(*) FROM componente_proposta c
                 WHERE c.thread_id = u.thread_id AND c.stato = 'aperta' AND c.creato_il > u.congelata_il)
             + (SELECT count(*) FROM relazione_proposta r
                 WHERE r.thread_id = u.thread_id AND r.stato = 'aperta' AND r.creato_il > u.congelata_il)
             + (SELECT count(*) FROM rimozione_proposta x
                 WHERE x.thread_id = u.thread_id AND x.stato = 'aperta' AND x.creato_il > u.congelata_il) AS n_proposte) e
 WHERE e.n_file + e.n_proposte > 0
   AND NOT EXISTS (SELECT 1 FROM bom_versione z WHERE z.thread_id = u.thread_id AND z.stato = 'bozza');

CREATE OR REPLACE VIEW v_cruscotto AS
SELECT t.thread_id, c.cartella_nas AS cliente, b.cognome AS buyer, t.oggetto, t.data_inizio, t.data_scadenza, t.scadenza_origine,
       t.ultimo_aggiornamento, t.stato AS stato_thread, t.cartella_relativa, t.priorita,
       (SELECT array_agg(i.codice ORDER BY i.creato_il) FROM identificativo_thread i WHERE i.thread_id = t.thread_id) AS identificativi,
       vf.nome_fase, vf.inizio AS in_fase_dal, vf.gg_in_fase, vf.sla_gg, vf.semaforo, u.sigla AS in_carico_a,
       bl.n_bloccanti, bl.n_da_confermare, bl.n_sul_portale, bl.n_mancanti,
       (SELECT count(*) FROM messaggio m WHERE m.thread_id = t.thread_id) AS n_messaggi,
       (SELECT count(*) FROM documento_proposta p WHERE p.thread_id = t.thread_id AND p.stato = 'aperta') AS n_da_smistare
FROM thread_offerta t
JOIN cliente c ON c.cliente_id = t.cliente_id
LEFT JOIN buyer b ON b.buyer_id = t.buyer_id
LEFT JOIN v_thread_fase vf ON vf.thread_id = t.thread_id
LEFT JOIN utente u ON u.utente_id = vf.responsabile_id
LEFT JOIN v_thread_bloccanti bl ON bl.thread_id = t.thread_id
WHERE t.unito_in IS NULL;

CREATE OR REPLACE VIEW v_codici_candidati_thread AS
SELECT m.thread_id, cc.codice::text AS codice, cc.rev::text AS rev,
       'messaggio'::text AS sorgente, cc.origine::text AS origine, cc.famiglia::text AS famiglia,
       cc.punteggio::int AS punteggio, cc.evidenza::text AS evidenza, cc.ruolo::text AS ruolo,
       cc.messaggio_id, NULL::uuid AS allegato_id
  FROM candidato_codice cc
  JOIN messaggio m ON m.messaggio_id = cc.messaggio_id
 WHERE m.thread_id IS NOT NULL AND cc.ruolo <> 'riferimento_rfq'
UNION ALL
SELECT p.thread_id, p.codice::text, COALESCE(p.rev, '')::text,
       'proposta_documento', p.fonte::text, '', p.confidenza::int, a.nome_file::text, NULL,
       a.messaggio_id, p.allegato_id
  FROM documento_proposta p
  JOIN allegato a ON a.allegato_id = p.allegato_id
 WHERE p.thread_id IS NOT NULL AND p.stato <> 'scartata' AND nullif(btrim(p.codice), '') IS NOT NULL
UNION ALL
SELECT p.thread_id, n.codice, '',
       'nome_file', 'nome_file', '', 50, a.nome_file::text, NULL,
       a.messaggio_id, p.allegato_id
  FROM documento_proposta p
  JOIN allegato a ON a.allegato_id = p.allegato_id
  CROSS JOIN LATERAL jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(p.dettagli -> 'codici_nel_nome') = 'array' THEN p.dettagli -> 'codici_nel_nome' ELSE '[]'::jsonb END
  ) AS n (codice)
 WHERE p.thread_id IS NOT NULL AND p.stato <> 'scartata' AND btrim(n.codice) <> ''
UNION ALL
SELECT cp.thread_id, cp.codice::text, COALESCE(cp.rev, '')::text,
       cp.fonte::text, cp.origine_codice::text, cp.famiglia::text, cp.confidenza::int,
       (cp.nome_grezzo || ' — ' || a.nome_file)::text, NULL,
       a.messaggio_id, cp.allegato_id
  FROM componente_proposta cp
  JOIN allegato a ON a.allegato_id = cp.allegato_id
 WHERE cp.stato <> 'scartata' AND nullif(btrim(cp.codice), '') IS NOT NULL;

-- ------------------------------------------------------------ le cinque funzioni della 0022, byte per byte
CREATE OR REPLACE FUNCTION distinta_bozza_versione_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    numero_base int;
BEGIN
    IF NEW.base_conferma_id IS NOT NULL THEN
        SELECT x.numero INTO numero_base FROM conferma_strutturale x WHERE x.conferma_id = NEW.base_conferma_id;
        IF FOUND AND numero_base <> NEW.base_numero THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X04',
                MESSAGE = format('distinta_bozza %s, versione %s: base_numero %s, ma la conferma di base %s e'' la n. %s',
                                 NEW.bozza_id, NEW.versione, NEW.base_numero, NEW.base_conferma_id, numero_base);
        END IF;
    END IF;
    NEW.tx := pg_current_xact_id();
    NEW.salvata_il := now();
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION conferma_strutturale_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    tipo_prodotto tipo_componente;
    ultimo_id     uuid;
    ultimo_numero int;
    b             record;
    c_struttura   bigint;
    c_dati        bigint;
BEGIN
    SELECT c.tipo INTO tipo_prodotto FROM componente c WHERE c.componente_id = NEW.prodotto_id AND c.thread_id = NEW.thread_id;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- lo rifiuta la FK, con il suo errore
    END IF;
    IF tipo_prodotto <> 'finito' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma strutturale: %s e'' un %s, non un prodotto finito', NEW.prodotto_id, tipo_prodotto);
    END IF;
    SELECT x.conferma_id, x.numero INTO ultimo_id, ultimo_numero
      FROM conferma_strutturale x WHERE x.prodotto_id = NEW.prodotto_id ORDER BY x.numero DESC LIMIT 1;
    IF NEW.numero <> COALESCE(ultimo_numero, 0) + 1 OR NEW.precedente_id IS DISTINCT FROM ultimo_id THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma strutturale di %s: attesa la n. %s dopo %s, arrivata la n. %s dopo %s',
                             NEW.prodotto_id, COALESCE(ultimo_numero, 0) + 1, COALESCE(ultimo_id::text, 'nessuna'),
                             NEW.numero, COALESCE(NEW.precedente_id::text, 'nessuna'));
    END IF;
    IF NEW.bozza_id IS NOT NULL THEN
        SELECT x.stato, x.versione, v.impronta INTO b
          FROM distinta_bozza x
          LEFT JOIN distinta_bozza_versione v ON v.bozza_id = x.bozza_id AND v.versione = x.versione
         WHERE x.bozza_id = NEW.bozza_id;
        IF FOUND AND (b.stato <> 'aperta' OR b.versione <> NEW.bozza_versione) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma strutturale: la bozza %s e'' %s alla versione %s, la conferma porta la versione %s',
                                 NEW.bozza_id, b.stato, b.versione, NEW.bozza_versione);
        END IF;
        -- la conferma certifica lo snapshot della versione che chiude, non un altro
        IF FOUND AND b.impronta IS DISTINCT FROM NEW.impronta_snapshot THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma strutturale: lo snapshot (impronta %s) non e'' quello della versione %s della bozza %s (impronta %s)',
                                 NEW.impronta_snapshot, NEW.bozza_versione, NEW.bozza_id, b.impronta);
        END IF;
    END IF;
    IF NEW.mono_pezzo AND EXISTS (SELECT 1 FROM componente_relazione r WHERE r.padre_id = NEW.prodotto_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma strutturale di %s: mono_pezzo dichiarato ma il prodotto ha figli', NEW.prodotto_id);
    END IF;
    -- i contatori registrati sono quelli correnti della RFQ (riga mancante = 0, 0)
    SELECT w.struttura, w.dati INTO c_struttura, c_dati FROM working_contatore w WHERE w.thread_id = NEW.thread_id FOR UPDATE;
    IF NEW.contatore <> COALESCE(c_struttura, 0) OR NEW.contatore_dati <> COALESCE(c_dati, 0) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma strutturale di %s: contatori registrati %s/%s, correnti %s/%s (struttura/dati)',
                             NEW.prodotto_id, NEW.contatore, NEW.contatore_dati, COALESCE(c_struttura, 0), COALESCE(c_dati, 0));
    END IF;
    -- da qui alla fine della transazione la working della RFQ non si scrive piu' (working_conta)
    INSERT INTO working_contatore AS w (thread_id, tx_conferma) VALUES (NEW.thread_id, pg_current_xact_id())
    ON CONFLICT (thread_id) DO UPDATE SET tx_conferma = EXCLUDED.tx_conferma;
    NEW.tx := pg_current_xact_id();                    -- i componenti e gli archi si scrivono solo in questa transazione
    NEW.confermato_il := now();
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION conferma_strutturale_contenuto() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    h record;
    c record;
BEGIN
    SELECT x.tx, x.prodotto_id, x.mono_pezzo, x.formato INTO h FROM conferma_strutturale x WHERE x.conferma_id = NEW.conferma_id;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- lo rifiuta la FK
    END IF;
    IF h.tx <> pg_current_xact_id() THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('%s: la conferma %s e'' gia'' registrata; il suo contenuto si scrive solo nella sua transazione',
                             TG_TABLE_NAME, NEW.conferma_id);
    END IF;
    IF position(NEW.conferma_id::text || ',' IN COALESCE(current_setting('cockpit.p4_conferme_controllate', true), '')) > 0 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('%s: la conferma %s ha gia'' passato il controllo finale (anche anticipato con SET CONSTRAINTS ... IMMEDIATE); il suo contenuto non cresce piu''',
                             TG_TABLE_NAME, NEW.conferma_id);
    END IF;
    IF TG_TABLE_NAME = 'conferma_strutturale_componente' THEN
        IF NEW.radice <> (NEW.componente_id = h.prodotto_id) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: radice e'' solo il prodotto %s (riga %s)', NEW.conferma_id, h.prodotto_id, NEW.componente_id);
        END IF;
        SELECT k.codice, k.rev, k.descrizione, k.tipo, k.archiviato_il, k.step_strutturale_id INTO c
          FROM componente k WHERE k.componente_id = NEW.componente_id;
        IF FOUND AND ((c.codice, c.rev, c.descrizione, c.tipo, c.step_strutturale_id)
                      IS DISTINCT FROM (NEW.codice, NEW.rev, NEW.descrizione, NEW.tipo, NEW.step_strutturale_id)
                      OR c.archiviato_il IS NOT NULL) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: il componente %s nella working non e'' quello registrato (codice, rev, descrizione, tipo, STEP o archiviazione)',
                                 NEW.conferma_id, NEW.componente_id);
        END IF;
        -- step_sha256 e' lo sha del documento STEP autorizzato in quel momento
        IF NEW.step_strutturale_id IS NOT NULL
           AND NOT EXISTS (SELECT 1 FROM documento d WHERE d.documento_id = NEW.step_strutturale_id AND d.sha256 = NEW.step_sha256) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: lo sha256 dello STEP del componente %s non e'' quello del documento %s',
                                 NEW.conferma_id, NEW.componente_id, NEW.step_strutturale_id);
        END IF;
        -- f.2: ogni componente certificato e' un nodo della bozza, con la sua chiave (la mappa la usa)
        IF h.formato = 2 AND NEW.chiave_bozza IS NULL THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s (formato 2): il componente %s non ha la chiave della bozza', NEW.conferma_id, NEW.componente_id);
        END IF;
    ELSE
        IF h.mono_pezzo THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: mono_pezzo, nessun arco', NEW.conferma_id);
        END IF;
        -- f.2: ogni arco dice da dove viene; origine_gerarchia (step | manuale | mista, E2.1) si calcola da qui.
        -- Che sia vera (un arco step e' davvero nella fonte) lo ricalcola il server in R4/R5, non la pagina.
        -- Revisione di dominio della notte, RD-03.
        IF h.formato = 2 AND NEW.origine IS NULL THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s (formato 2): l''arco %s → %s non dice da dove viene (origine)', NEW.conferma_id, NEW.padre_id, NEW.figlio_id);
        END IF;
        IF NOT EXISTS (SELECT 1 FROM componente_relazione r
                        WHERE r.padre_id = NEW.padre_id AND r.figlio_id = NEW.figlio_id AND r.qta = NEW.qta
                          AND r.posizione IS NOT DISTINCT FROM NEW.posizione) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: l''arco %s → %s qta %s (posizione %s) non e'' nella working',
                                 NEW.conferma_id, NEW.padre_id, NEW.figlio_id, NEW.qta, COALESCE(NEW.posizione, '-'));
        END IF;
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION conferma_strutturale_completa() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    n_radici int;
    step_radice uuid;
    tipo_radice text;
    manca record;
BEGIN
    SELECT count(*), max(k.step_strutturale_id::text)::uuid, max(k.tipo::text) INTO n_radici, step_radice, tipo_radice
      FROM conferma_strutturale_componente k WHERE k.conferma_id = NEW.conferma_id AND k.radice;
    IF n_radici <> 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s: al COMMIT manca la riga del prodotto %s fra i componenti certificati', NEW.conferma_id, NEW.prodotto_id);
    END IF;
    IF NEW.criterio_verifica = 'step' AND step_radice IS NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s: criterio step, ma il prodotto certificato non ha uno STEP autorizzato', NEW.conferma_id);
    END IF;
    IF NEW.mono_pezzo THEN
        IF tipo_radice IS DISTINCT FROM 'finito' THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: mono_pezzo, ma la radice certificata e'' un %s, non un finito', NEW.conferma_id, tipo_radice);
        END IF;
        IF step_radice IS NULL THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: mono_pezzo senza lo STEP autorizzato del prodotto sulla radice certificata (D-P4A-02 A)', NEW.conferma_id),
                HINT = 'prima la conferma del documento STEP e la sua autorizzazione, poi la conferma con mono_pezzo';
        END IF;
        IF EXISTS (SELECT 1 FROM conferma_strutturale_arco a WHERE a.conferma_id = NEW.conferma_id)
           OR EXISTS (SELECT 1 FROM conferma_strutturale_componente k WHERE k.conferma_id = NEW.conferma_id AND NOT k.radice)
           OR EXISTS (SELECT 1 FROM componente_relazione r WHERE r.padre_id = NEW.prodotto_id) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: mono_pezzo, ma al COMMIT il prodotto %s ha figli o archi (nella conferma o nella working)',
                                 NEW.conferma_id, NEW.prodotto_id);
        END IF;
    END IF;
    SELECT r.padre_id, r.figlio_id, r.qta INTO manca
      FROM conferma_strutturale_componente k
      JOIN componente_relazione r ON r.padre_id = k.componente_id
      JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
     WHERE k.conferma_id = NEW.conferma_id
       AND NOT EXISTS (SELECT 1 FROM conferma_strutturale_arco a
                        WHERE a.conferma_id = NEW.conferma_id AND a.padre_id = r.padre_id
                          AND a.figlio_id = r.figlio_id AND a.qta = r.qta)
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s: l''arco %s → %s qta %s della working non e'' fra gli archi certificati',
                             NEW.conferma_id, manca.padre_id, manca.figlio_id, manca.qta);
    END IF;
    WITH RECURSIVE raggiunti (id) AS (
        SELECT NEW.prodotto_id
        UNION
        SELECT a.figlio_id
          FROM conferma_strutturale_arco a
          JOIN raggiunti g ON g.id = a.padre_id
         WHERE a.conferma_id = NEW.conferma_id)
    SELECT k.componente_id INTO manca
      FROM conferma_strutturale_componente k
     WHERE k.conferma_id = NEW.conferma_id
       AND NOT EXISTS (SELECT 1 FROM raggiunti g WHERE g.id = k.componente_id)
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s: il componente %s e'' certificato ma nessun arco certificato ci arriva dalla radice', NEW.conferma_id, manca.componente_id);
    END IF;
    IF NEW.bozza_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM distinta_bozza x WHERE x.bozza_id = NEW.bozza_id AND x.stato = 'confermata') THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s: al COMMIT la bozza %s che la conferma chiude non e'' chiusa come confermata', NEW.conferma_id, NEW.bozza_id),
            HINT = 'nella transazione della conferma: UPDATE distinta_bozza SET stato = ''confermata'', chiusa_da = ...';
    END IF;
    PERFORM set_config('cockpit.p4_conferme_controllate',
                       COALESCE(current_setting('cockpit.p4_conferme_controllate', true), '') || NEW.conferma_id::text || ',', true);
    RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION fonte_step_risolvi() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    h       record;
    riusata bigint;
BEGIN
    IF NEW.chiave_bozza IS NULL THEN
        RETURN NULL;
    END IF;
    SELECT x.bozza_id, x.numero, x.confermato_da INTO h FROM conferma_strutturale x WHERE x.conferma_id = NEW.conferma_id;
    IF NOT FOUND OR h.bozza_id IS NULL THEN
        RETURN NULL;
    END IF;
    SELECT e.evento_id INTO riusata
      FROM fonte_step_evento e
     WHERE e.bozza_id = h.bozza_id AND e.chiave_bozza = NEW.chiave_bozza AND e.ruolo = 'sottoassieme'
       AND e.componente_id IS NULL AND e.azione <> 'risoluzione'
       AND NOT EXISTS (SELECT 1 FROM fonte_step_evento r WHERE r.risolve_evento_id = e.evento_id)
       AND EXISTS (SELECT 1 FROM distinta_bozza_evento g
                    WHERE g.bozza_id = h.bozza_id AND g.chiave_bozza = NEW.chiave_bozza AND g.azione = 'nodo_aggiunto'
                      AND g.versione > (e.destinazione ->> 'versione')::int
                      AND g.dopo IS DISTINCT FROM e.destinazione -> 'nodo')
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X07',
            MESSAGE = format('conferma n. %s: la destinazione temporanea %s dell''evento %s della fonte e'' stata riusata per un nodo diverso dopo l''evento; non si risolve sul nodo certificato',
                             h.numero, NEW.chiave_bozza, riusata),
            HINT = 'una chiave della bozza tolta non torna per un nodo diverso (regola di R2)';
    END IF;
    INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, componente_id, chiave_bozza, destinazione, risolve_evento_id, azione,
                                   allegato_id, sha256, gruppo, proposta_id, documento_id, bozza_id, motivo, autore_id)
    SELECT e.thread_id, e.prodotto_id, 'sottoassieme', NEW.componente_id, e.chiave_bozza, e.destinazione, e.evento_id, 'risoluzione',
           e.allegato_id, e.sha256, e.gruppo, e.proposta_id, e.documento_id, e.bozza_id,
           format('destinazione %s della bozza %s risolta dalla conferma n. %s', e.chiave_bozza, e.bozza_id, h.numero),
           h.confermato_da
      FROM fonte_step_evento e
     WHERE e.bozza_id = h.bozza_id AND e.chiave_bozza = NEW.chiave_bozza AND e.ruolo = 'sottoassieme'
       AND e.componente_id IS NULL AND e.azione <> 'risoluzione'
       AND NOT EXISTS (SELECT 1 FROM fonte_step_evento r WHERE r.risolve_evento_id = e.evento_id)
     ORDER BY e.evento_id;
    RETURN NULL;
END $$;

-- ------------------------------------------------------------ le associazioni registrate (direttiva 35 e 39, sezione 9d)
DROP VIEW v_ar_backfill_diagnostica;
ALTER TABLE fonte_step_evento DROP CONSTRAINT ck_fonte_revoca_sorgente, DROP CONSTRAINT ck_fonte_azione;
ALTER TABLE fonte_step_evento
    ADD CONSTRAINT ck_fonte_azione CHECK (azione IN ('scelta', 'rifiuto', 'sostituzione', 'caricamento',
                                                 'conferma_documento', 'autorizzazione', 'revoca', 'risoluzione'));
DROP TRIGGER trg_conferma_associazioni ON conferma_strutturale;
DROP FUNCTION conferma_strutturale_associazioni();
DROP TRIGGER trg_proposta_copia_derivata ON documento_proposta;
DROP FUNCTION documento_proposta_copia_derivata();
DROP TRIGGER trg_file_rfq_senza_ar ON file_rfq;
DROP FUNCTION file_rfq_senza_ar_attive();
DROP TRIGGER trg_associazione_della_riga ON associazione_evento;
DROP FUNCTION associazione_evento_della_riga();
DROP INDEX ux_associazione_evento_chiave;
DROP INDEX ix_associazione_evento_allegato;
DROP INDEX ix_associazione_evento_ar;
ALTER TABLE associazione_evento
    DROP CONSTRAINT fk_associazione_ar,
    DROP CONSTRAINT fk_associazione_ar_precedente,
    DROP CONSTRAINT ck_associazione_oggetto,
    DROP CONSTRAINT ck_associazione_azione,
    DROP CONSTRAINT ck_associazione_origine,
    DROP CONSTRAINT ck_associazione_autore,
    DROP CONSTRAINT ck_associazione_coerente;
ALTER TABLE associazione_evento DROP COLUMN allegato_id, DROP COLUMN associazione_id, DROP COLUMN associazione_precedente_id,
    DROP COLUMN tx;
ALTER TABLE associazione_evento ALTER COLUMN autore_id SET NOT NULL;
ALTER TABLE associazione_evento
    ADD CONSTRAINT ck_associazione_oggetto CHECK (proposta_id IS NOT NULL OR documento_id IS NOT NULL),
    ADD CONSTRAINT ck_associazione_azione CHECK (azione IN ('assegna', 'sgancia', 'scarta', 'riapri', 'preassociazione', 'caricamento', 'sposta_documento')),
    ADD CONSTRAINT ck_associazione_origine CHECK (origine IN ('ponte', 'legacy', 'conferma_bom', 'inbox')),
    ADD CONSTRAINT ck_associazione_coerente CHECK (
        (azione NOT IN ('assegna', 'preassociazione', 'sposta_documento') OR componente_dopo IS NOT NULL)
        AND (azione <> 'sgancia' OR (componente_prima IS NOT NULL AND componente_dopo IS NULL))
        AND (azione <> 'preassociazione' OR (conferma_id IS NOT NULL AND origine = 'conferma_bom')));
DROP INDEX ix_correzione_associazione;
ALTER TABLE correzione_documento DROP CONSTRAINT fk_correzione_associazione, DROP CONSTRAINT ck_correzione_oggetto;
ALTER TABLE correzione_documento DROP COLUMN associazione_id;
ALTER TABLE correzione_documento ADD CONSTRAINT ck_correzione_oggetto CHECK (proposta_id IS NOT NULL OR documento_id IS NOT NULL);
DROP TABLE file_rfq_associazione;
DROP FUNCTION file_rfq_associazione_invarianti();
DROP FUNCTION file_rfq_associazione_cambia();
DROP FUNCTION file_rfq_associazione_bom();
DROP FUNCTION file_rfq_associazione_nasce();
DROP FUNCTION allegato_sorgente_del_finito(uuid, uuid);
DROP FUNCTION rfq_e_formato3(uuid);
DROP TRIGGER trg_distinta_bozza_formato3 ON distinta_bozza;
DROP TABLE rfq_formato3_segno;
DROP FUNCTION rfq_formato3_segna();

-- ------------------------------------------------------------ esito degli intenti
DROP TRIGGER trg_esito_intento_formato ON esito_intento;
DROP FUNCTION esito_intento_formato();
ALTER TABLE esito_intento DROP CONSTRAINT ck_esito_intento_gesto, DROP CONSTRAINT ck_esito_intento_conferma;
ALTER TABLE esito_intento
    ADD CONSTRAINT ck_esito_intento_gesto CHECK (gesto IN ('conferma_bom_v2', 'lotto_fascicolo', 'correggi_documento', 'fonte_step', 'riprova_copia')),
    ADD CONSTRAINT ck_esito_intento_conferma CHECK ((gesto = 'conferma_bom_v2') = (conferma_id IS NOT NULL));

-- ------------------------------------------------------------ il rifiuto della proposta aggiornata e il digest dei fatti
DROP TABLE proposta_aggiornata_rifiuto;
DROP TRIGGER trg_analisi_fatti_digest ON analisi_fatti;
DROP FUNCTION analisi_fatti_digest();
ALTER TABLE analisi_fatti DROP CONSTRAINT ck_analisi_fatti_digest;
ALTER TABLE analisi_fatti DROP COLUMN digest;

-- ------------------------------------------------------------ i nodi non inclusi e il ciclo dei file
DROP TABLE conferma_strutturale_non_incluso;
DROP FUNCTION conferma_strutturale_non_incluso_nasce();
DROP TABLE file_rfq_conservazione;
DROP TABLE file_rfq_rimozione;
DROP TABLE file_rfq_evento;
DROP TABLE file_rfq;
DROP FUNCTION file_rfq_conservazione_invarianti();
DROP FUNCTION file_rfq_conservazione_cambia();
DROP FUNCTION file_rfq_conservazione_nasce();
DROP FUNCTION file_rfq_rimozione_cambia();
DROP FUNCTION file_rfq_rimozione_nasce();
DROP FUNCTION file_rfq_invarianti();
DROP FUNCTION file_rfq_cambia();
DROP FUNCTION file_rfq_nasce();

-- ------------------------------------------------------------ componenti certificati
ALTER TABLE conferma_strutturale_componente DROP CONSTRAINT ck_csc_identita;
DROP INDEX ix_csc_emittente;
ALTER TABLE conferma_strutturale_componente DROP COLUMN emittente, DROP COLUMN emittente_presunto, DROP COLUMN namespace;

-- ------------------------------------------------------------ conferma strutturale
DROP INDEX ix_conferma_importazioni_citate;
DROP INDEX ix_conferma_importazione;
ALTER TABLE conferma_strutturale
    DROP CONSTRAINT ck_conferma_importazioni_citate,
    DROP CONSTRAINT ck_conferma_importazione,
    DROP CONSTRAINT fk_conferma_importazione;
ALTER TABLE conferma_strutturale DROP COLUMN importazione_id, DROP COLUMN importazioni_citate;
ALTER TABLE conferma_strutturale DROP CONSTRAINT ck_conferma_verifica, DROP CONSTRAINT ck_conferma_preassociazioni;
ALTER TABLE conferma_strutturale
    DROP CONSTRAINT ck_conferma_formato,
    DROP CONSTRAINT ck_conferma_bozza,
    DROP CONSTRAINT ck_conferma_snapshot,
    DROP CONSTRAINT ck_conferma_chiave;
ALTER TABLE conferma_strutturale
    ADD CONSTRAINT ck_conferma_formato CHECK (formato IN (1, 2) AND senza_riepilogo_v2 = (formato = 1)),
    ADD CONSTRAINT ck_conferma_bozza CHECK ((bozza_id IS NULL) = (bozza_versione IS NULL) AND (formato = 2 OR bozza_id IS NULL)),
    ADD CONSTRAINT ck_conferma_snapshot CHECK (impronta_snapshot ~ '^[0-9a-f]{64}$'
        AND (formato = 1 OR (COALESCE(snapshot ->> 'formato' = '2', false)
                             AND COALESCE(snapshot ->> 'prodotto_id' = prodotto_id::text, false)))),
    ADD CONSTRAINT ck_conferma_chiave CHECK ((formato = 2) = (chiave_idempotenza IS NOT NULL));

-- ------------------------------------------------------------ versioni della bozza
DROP INDEX ux_dbv_chiave_salvataggio;
DROP INDEX ix_dbv_importazione;
ALTER TABLE distinta_bozza_versione
    DROP CONSTRAINT ck_dbv_chiave_salvataggio,
    DROP CONSTRAINT ck_dbv_preassociazioni,
    DROP CONSTRAINT ck_dbv_snapshot_formato,
    DROP CONSTRAINT fk_dbv_importazione;
ALTER TABLE distinta_bozza_versione DROP COLUMN importazione_id, DROP COLUMN chiave_salvataggio, DROP COLUMN impronta_richiesta;

-- ------------------------------------------------------------ registro d'importazione
DROP TABLE distinta_importazione;
DROP FUNCTION distinta_importazione_nasce();
DROP FUNCTION file_rfq_scartato_legacy(uuid, uuid);
DROP FUNCTION file_rfq_attivo(uuid, uuid);

-- ------------------------------------------------------------ testata della bozza
ALTER TABLE distinta_bozza DROP CONSTRAINT ck_distinta_bozza_formato;
ALTER TABLE distinta_bozza ADD CONSTRAINT ck_distinta_bozza_formato CHECK (formato = 2);

-- ------------------------------------------------------------ i commenti della 0022
COMMENT ON TABLE distinta_bozza_versione IS
'Lo snapshot formato 2 completo di ogni versione salvata della bozza, con la base e la fonte di quel momento.
Non si riscrive.';
COMMENT ON TABLE conferma_strutturale IS
'La conferma ufficiale della struttura di un prodotto: progressiva (numero) e immutabile. Formato 1 (giro 4,
senza_riepilogo_v2) o 2 (R5, con chiave di idempotenza). Diversa da bom_versione e da BOM.Verificata.
mono_pezzo con motivo, senza figli e con lo STEP autorizzato sulla radice. criterio_verifica: step, mono_pezzo
o nessuno. La struttura ufficiale nasce da questa conferma umana, non dall''accettazione dello STEP
(D-SHOCK-S1 B).';
COMMENT ON COLUMN conferma_strutturale_arco.origine IS
'Da dove viene l''arco: step (estratto dalla fonte STEP), utente (deciso da una persona), conferma o
registrato (gia'' nella conferma precedente o nella working). Obbligatoria in formato 2 (trigger). La ricalcola il
server in R4/R5 confrontando con la fonte dichiarata, mai copiata da quello che dice la pagina: un arco
disegnato a mano non diventa «estratto dallo STEP» (D-SHOCK-S1 B, E2.1; origine_gerarchia si calcola da qui).';
COMMENT ON COLUMN conferma_strutturale.verifica IS
'La verifica del Motore A calcolata al COMMIT (al_momento_della_conferma). Non e'' BOM.Verificata, che si
ricalcola a ogni lettura.';
COMMENT ON TABLE associazione_evento IS
'Chi e quando di ogni associazione di proposte e documenti (assegna, sgancia, scarta, riapri,
preassociazione, caricamento, spostamento), da tutte le strade quando le scriveranno: gli scrittori sono di B4, e
la storia comincia dal primo evento scritto da B4. Solo aggiunte.';
COMMENT ON COLUMN documento_proposta.componente_id IS NULL;

DELETE FROM schema_variante WHERE versione = 23;
DELETE FROM schema_versione WHERE versione = 23;

COMMIT;
