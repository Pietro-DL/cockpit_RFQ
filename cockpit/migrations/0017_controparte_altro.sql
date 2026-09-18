-- 0017_controparte_altro.sql — Blocco 7C.0, parte 2: i vincoli e la vista che usano `altro`.
-- Separata dalla 0016 per la regola S4 del migratore (un valore enum aggiunto non si usa nello
-- stesso file).

-- ============================================================ 1. IL CHECK sul messaggio, con la sesta controparte
ALTER TABLE messaggio DROP CONSTRAINT ck_messaggio_controparte_altro_provvisorio;
ALTER TABLE messaggio DROP CONSTRAINT ck_messaggio_controparte;
ALTER TABLE messaggio ADD CONSTRAINT ck_messaggio_controparte CHECK (
       (controparte_tipo = 'cliente'   AND controparte_cliente_id   IS NOT NULL AND controparte_fornitore_id IS NULL AND controparte_altro_id IS NULL)
    OR (controparte_tipo = 'fornitore' AND controparte_fornitore_id IS NOT NULL AND controparte_cliente_id   IS NULL AND controparte_altro_id IS NULL)
    OR (controparte_tipo = 'altro'     AND controparte_altro_id     IS NOT NULL AND controparte_cliente_id   IS NULL AND controparte_fornitore_id IS NULL)
    OR (controparte_tipo IN ('interno','sconosciuto','ambiguo')
        AND controparte_cliente_id IS NULL AND controparte_fornitore_id IS NULL AND controparte_altro_id IS NULL)
);
CREATE INDEX ix_messaggio_controparte_altro ON messaggio (controparte_altro_id) WHERE controparte_altro_id IS NOT NULL;

-- ============================================================ 2. VIA il vecchio vocabolario
-- La vista legge proposta_triage.intento: si toglie prima lei (CREATE OR REPLACE non sa togliere
-- ne' rinominare colonne), poi la colonna e il tipo. Tutto in questa transazione.
DROP VIEW v_inbox;
ALTER TABLE proposta_triage DROP COLUMN intento;
DROP TYPE intento_messaggio;

-- ============================================================ 3. v_inbox: l'atto, l'etichetta di «altro», il QUADRANTE
--
-- Novita' rispetto alla 0015:
--   * triage_intento → triage_atto (il codice dell'atto proposto, mai NULL);
--   * `controparte` mostra anche l'etichetta di un soggetto «altro»;
--   * `quadrante`: il dominio Inbox del messaggio, calcolato QUI e in nessun altro posto. Finora lo
--     calcolavano in due — le tre query di messaggi.sql e quadranteDi() in Go — con lo stesso CASE
--     scritto due volte. Da qui le query filtrano su quadrante = $1 e il Go legge la colonna.
--     Valori: clienti | fornitori | interni | altro | validare. Dipende SOLO dalla controparte:
--     Da validare = sconosciuto + ambiguo. La regola 7B.3 che mandava li' anche la posta «non di
--     lavoro» di un cliente non c'e' piu' (decisione del 18/09/2026): un indirizzo automatico noto
--     si censisce come Altro, e la newsletter dal dominio del cliente sta fra i Clienti finche'
--     qualcuno non lo fa;
--   * la proposta letta e' quella DETERMINISTICA. Prima la vista sceglieva quella dell'agente
--     quando c'era (ORDER BY fonte = 'agente'): una scelta implicita di chi ha ragione, che non
--     spetta a una vista. Il confronto fra le due fonti e' una query dedicata.
CREATE VIEW v_inbox AS
SELECT m.messaggio_id, m.canale, m.direzione, m.interno, m.data_evento, m.thread_id, m.aggancio, m.conversazione_id,
       m.mittente_nome, m.mittente_indirizzo, m.oggetto, m.n_allegati, m.buyer_id,
       lower(split_part(m.mittente_indirizzo, '@', 2)) AS dominio,
       COALESCE(t.cliente_id, dc.cliente_id, b.cliente_id) AS cliente_id,
       COALESCE(ct.cartella_nas, cd.cartella_nas, cb.cartella_nas) AS cliente,
       b.cognome AS buyer_cognome,
       (SELECT count(*) FROM riferimento_portale r WHERE r.messaggio_id = m.messaggio_id) AS n_rif_portale,
       (SELECT count(*) FROM allegato a WHERE a.messaggio_id = m.messaggio_id AND a.contenitore_id IS NULL
          AND a.natura = 'file' AND lower(a.estensione) IN ('stp','step','sldprt','sldasm','igs','iges','dxf','dwg','pdf','tif','tiff','zip','7z','rar')) AS n_cad,
       pt.esito AS triage_esito, pt.confidenza AS triage_confidenza, pt.motivi AS triage_motivi, pt.thread_proposto,
       EXISTS (SELECT 1 FROM proposta_triage x WHERE x.messaggio_id = m.messaggio_id AND x.stato = 'rifiutata') AS ignorato,
       COALESCE(pr.caselle, '{}')      AS caselle,
       COALESCE(pr.caselle_id, '{}')   AS caselle_id,
       COALESCE(pr.n_caselle, 0)       AS n_caselle,
       COALESCE(pr.non_letto, false)   AS non_letto,
       pr.ricevuto_il,
       pr.casella_id, pr.cartella AS cartella_outlook, pr.entry_id,
       m.controparte_tipo::text AS controparte_tipo,
       CASE m.controparte_tipo
            WHEN 'cliente'   THEN cc.cartella_nas
            WHEN 'fornitore' THEN f.ragione_sociale
            WHEN 'altro'     THEN al.etichetta
       END AS controparte,
       COALESCE(pt.atto, '')           AS triage_atto,
       m.richiesta_fornitore_id,
       COALESCE(pt.legame::text, '')::varchar AS triage_legame,
       CASE m.controparte_tipo
            WHEN 'cliente'   THEN 'clienti'
            WHEN 'fornitore' THEN 'fornitori'
            WHEN 'interno'   THEN 'interni'
            WHEN 'altro'     THEN 'altro'
            ELSE 'validare'
       END AS quadrante
FROM messaggio m
LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
LEFT JOIN cliente ct ON ct.cliente_id = t.cliente_id
LEFT JOIN dominio_cliente dc ON dc.dominio = lower(split_part(m.mittente_indirizzo, '@', 2))
LEFT JOIN cliente cd ON cd.cliente_id = dc.cliente_id
LEFT JOIN buyer b ON b.buyer_id = m.buyer_id
LEFT JOIN cliente cb ON cb.cliente_id = b.cliente_id
LEFT JOIN cliente cc ON cc.cliente_id = m.controparte_cliente_id
LEFT JOIN fornitore f ON f.fornitore_id = m.controparte_fornitore_id
LEFT JOIN soggetto_altro al ON al.altro_id = m.controparte_altro_id
LEFT JOIN LATERAL (
    SELECT array_agg(c.nome        ORDER BY c.nome)  AS caselle,
           array_agg(mc.casella_id ORDER BY c.nome)  AS caselle_id,
           count(*)::int                             AS n_caselle,
           bool_or(mc.non_letto)                     AS non_letto,
           max(mc.ricevuto_il)                       AS ricevuto_il,
           (array_agg(mc.casella_id ORDER BY mc.ricevuto_il, mc.casella_id))[1] AS casella_id,
           (array_agg(mc.cartella   ORDER BY mc.ricevuto_il, mc.casella_id))[1] AS cartella,
           (array_agg(mc.entry_id   ORDER BY mc.ricevuto_il, mc.casella_id))[1] AS entry_id
      FROM messaggio_casella mc
      JOIN casella c ON c.casella_id = mc.casella_id
     WHERE mc.messaggio_id = m.messaggio_id
) pr ON true
LEFT JOIN proposta_triage pt ON pt.messaggio_id = m.messaggio_id AND pt.fonte = 'deterministico' AND pt.stato = 'proposta'
WHERE COALESCE(pr.n_caselle, 0) > 0 OR m.parent_messaggio_id IS NULL;

INSERT INTO schema_versione (versione) VALUES (17);
