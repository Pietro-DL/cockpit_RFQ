-- 0018_indietro.sql — ritorno MANUALE dallo schema 18 allo schema 17. Approvato il 23/09/2026.
--
-- Non e' una migrazione: il migratore non ha il verso «giu'» e questo file non va MAI in
-- cockpit/migrations/. Si lancia a mano, in una transazione, sul database che si vuole riportare
-- indietro, con il server FERMO; poi si riavvia il binario vecchio (schema 17).
--
-- Il ritorno vero e' il backup preso subito prima della 0018 (scripts/backup-db.ps1). Questo file
-- serve quando il backup non c'e' piu' o quando nel frattempo sono entrati dati che non si vogliono
-- perdere, e riporta la FORMA dello schema, non la storia. Non si annullano:
--   - le fusioni dei duplicati del passo 4 (le righe tolte non esistono piu');
--   - l'allineamento delle maiuscole dei codici dei documenti del passo 8;
--   - il contenuto di componente_proposta e relazione_proposta, che si butta: e' interpretazione,
--     e il server la rigenera dai fatti.
-- Si ferma se il modello nuovo contiene qualcosa che il vecchio non sa dire: un figlio con due padri.
--
-- UNA differenza di forma resta, ed e' ammessa (decisione del 23/09/2026): componente.padre_id,
-- ricreato con ALTER TABLE ... ADD COLUMN, torna fisicamente in FONDO alla tabella, non al terzo
-- posto dove l'aveva messo la 0001. Il binario della 17 legge componente per nome (sqlc espande
-- anche i SELECT *), quindi l'ordine non cambia niente per il codice; e ricostruire la tabella solo
-- per rimettere a posto l'ordine vorrebbe dire ricreare a mano FK, indici e commenti in uno script
-- che si usa di rado. Tutto il resto dello schema torna identico a quello della 17: colonne con le
-- loro posizioni, vincoli, indici, viste, commenti, fabbisogni di default
-- (TestIlRitornoManualeDalla18Alla17, sul ramo delle prove). Un pg_dump preso dopo il ritorno
-- mostra padre_id come ultima colonna di componente, e un ripristino da quel dump la lascia li'.

BEGIN;

DO $$
DECLARE
    v integer;
    problemi text := '';
    riga text;
BEGIN
    SELECT max(versione) INTO v FROM schema_versione;
    IF v IS DISTINCT FROM 18 THEN
        RAISE EXCEPTION '0018 indietro: lo schema e'' alla versione %, non alla 18', v;
    END IF;
    FOR riga IN
        SELECT format('componente %s ha %s padri', figlio_id, count(*))
          FROM componente_relazione GROUP BY figlio_id HAVING count(*) > 1 LIMIT 20
    LOOP problemi := problemi || E'\n' || riga; END LOOP;
    IF problemi <> '' THEN
        RAISE EXCEPTION USING MESSAGE =
            '0018 indietro: il modello vecchio ha un padre solo per componente, e questi ne hanno di piu'':' || problemi,
            HINT = 'Qui il ritorno e'' solo dal backup.';
    END IF;
    RAISE NOTICE '0018 indietro: si buttano % proposte di componente e % proposte di relazione',
        (SELECT count(*) FROM componente_proposta), (SELECT count(*) FROM relazione_proposta);
END $$;

DROP VIEW v_codici_candidati_thread;
DROP VIEW v_componente_albero;
DROP VIEW v_cruscotto;
DROP VIEW v_thread_bloccanti;
DROP VIEW v_fascicolo;

DROP TABLE relazione_proposta;
DROP TABLE componente_proposta;

ALTER TABLE documento
    DROP CONSTRAINT fk_documento_componente,
    DROP CONSTRAINT ck_documento_agganciato_ha_codice,
    DROP CONSTRAINT ck_documento_tecnico_ha_codice,
    ADD CONSTRAINT documento_componente_id_fkey FOREIGN KEY (componente_id) REFERENCES componente;
ALTER TABLE documento_proposta
    DROP CONSTRAINT fk_proposta_componente,
    DROP CONSTRAINT ck_proposta_componente_thread,
    DROP CONSTRAINT ck_proposta_agganciata_ha_codice,
    ADD CONSTRAINT documento_proposta_componente_id_fkey FOREIGN KEY (componente_id) REFERENCES componente;
ALTER TABLE deroga_fabbisogno
    DROP CONSTRAINT fk_deroga_componente,
    ADD CONSTRAINT deroga_fabbisogno_componente_id_fkey FOREIGN KEY (componente_id) REFERENCES componente;

-- padre_id torna dalle relazioni; la qta del figlio torna sulla sua riga, come nel modello vecchio
ALTER TABLE componente ADD COLUMN padre_id uuid;
UPDATE componente c SET padre_id = r.padre_id, qta = r.qta FROM componente_relazione r WHERE r.figlio_id = c.componente_id;
ALTER TABLE componente ADD CONSTRAINT componente_padre_id_fkey FOREIGN KEY (padre_id) REFERENCES componente;
DROP TABLE componente_relazione;

ALTER TABLE componente
    DROP CONSTRAINT ux_componente_thread_id_codice,
    DROP CONSTRAINT ux_componente_thread_id,
    DROP CONSTRAINT ck_componente_codice,
    ALTER COLUMN confermato_da DROP NOT NULL,
    ADD CONSTRAINT componente_thread_id_padre_id_codice_rev_key UNIQUE NULLS NOT DISTINCT (thread_id, padre_id, codice, rev);
DROP INDEX ux_componente_thread_codice;
COMMENT ON TABLE componente IS NULL;
COMMENT ON COLUMN componente.qta IS NULL;

-- i default di prima della 0018 (0001)
UPDATE fabbisogno_documento SET bloccante = true
 WHERE cliente_id IS NULL AND tipo_componente = 'sottoassieme' AND tipo = 'cad_3d';
DELETE FROM fabbisogno_documento
 WHERE cliente_id IS NULL AND (tipo_componente, tipo) IN (('sottoassieme','sviluppo_dxf'), ('sciolto','cad_3d'));

-- le tre viste della 0001, identiche
CREATE VIEW v_fascicolo AS
SELECT co.thread_id, co.componente_id, co.codice, co.rev, co.tipo AS tipo_componente, co.padre_id,
       f.tipo AS tipo_documento, f.bloccante,
       d.documento_id, d.stato_nas, d.path_relativo,
       p.proposta_id AS proposta_aperta,
       rp.rif_id     AS atteso_da_portale,
       dg.deroga_id,
       CASE WHEN d.documento_id IS NOT NULL THEN 'ok'
            WHEN dg.deroga_id  IS NOT NULL THEN 'derogato'
            WHEN p.proposta_id IS NOT NULL THEN 'da_confermare'
            WHEN rp.rif_id     IS NOT NULL THEN 'sul_portale'
            ELSE 'manca' END AS esito
FROM componente co
JOIN thread_offerta t ON t.thread_id = co.thread_id
JOIN LATERAL (
    SELECT x.tipo, x.bloccante FROM fabbisogno_documento x
    WHERE x.tipo_componente = co.tipo
      AND x.cliente_id IS NOT DISTINCT FROM (
            SELECT y.cliente_id FROM fabbisogno_documento y
            WHERE y.tipo_componente = co.tipo AND (y.cliente_id = t.cliente_id OR y.cliente_id IS NULL)
            ORDER BY (y.cliente_id IS NOT NULL) DESC LIMIT 1)
) f ON true
LEFT JOIN LATERAL (
    SELECT d.documento_id, d.stato_nas, d.path_relativo FROM documento d
    WHERE d.componente_id = co.componente_id AND d.tipo = f.tipo AND d.sostituito_da IS NULL
    ORDER BY d.confermato_il DESC LIMIT 1) d ON true
LEFT JOIN LATERAL (
    SELECT p.proposta_id FROM documento_proposta p
    WHERE p.thread_id = co.thread_id AND p.stato = 'aperta' AND p.tipo_proposto = f.tipo
      AND (p.componente_id = co.componente_id OR (p.componente_id IS NULL AND p.codice = co.codice))
    LIMIT 1) p ON true
LEFT JOIN LATERAL (
    SELECT r.rif_id FROM riferimento_portale r
    WHERE r.thread_id = co.thread_id AND r.codice = co.codice AND r.stato = 'da_scaricare'
      AND (r.tipo_atteso IS NULL OR r.tipo_atteso = f.tipo)
    LIMIT 1) rp ON true
LEFT JOIN deroga_fabbisogno dg ON dg.componente_id = co.componente_id AND dg.tipo = f.tipo;

CREATE VIEW v_thread_bloccanti AS
SELECT t.thread_id,
       count(v.componente_id) FILTER (WHERE v.bloccante AND v.esito NOT IN ('ok','derogato')) AS n_bloccanti,
       count(v.componente_id) FILTER (WHERE v.esito = 'da_confermare')                      AS n_da_confermare,
       count(v.componente_id) FILTER (WHERE v.esito = 'sul_portale')                        AS n_sul_portale,
       count(v.componente_id) FILTER (WHERE v.esito = 'manca')                              AS n_mancanti
FROM thread_offerta t LEFT JOIN v_fascicolo v ON v.thread_id = t.thread_id
GROUP BY t.thread_id;

CREATE VIEW v_cruscotto AS
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

DELETE FROM schema_versione WHERE versione = 18;

COMMIT;
