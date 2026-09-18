-- 0015_richiesta_fornitore.sql — Blocco 7B: le richieste d'offerta AI fornitori, l'intento del
-- messaggio, i candidati verso una richiesta.
--
-- Una richiesta a un fornitore e' FIGLIA della RFQ cliente, non un thread parallelo: la RFQ e' una,
-- e sotto di lei stanno le domande che facciamo a chi deve fare la zincatura, la cataforesi, la
-- tornitura. Il suo ciclo: bozza (creata dal Cockpit, non ancora mandata) -> inviata (la nostra
-- mail e' nella Posta inviata, legata dal marcatore o confermata a mano) -> risposta (l'offerta
-- del fornitore e' agganciata) | scaduta | annullata.
--
-- L'INTENTO e' che cosa il messaggio E' (una richiesta di un cliente, un'offerta di un fornitore,
-- una newsletter); l'ESITO del triage resta che cosa si propone di FARE (nuova_rfq, aggancia,
-- ignora). Sono due cose diverse e stanno in due colonne: un'offerta di fornitore ha esito
-- `aggancia` verso una richiesta, non verso una RFQ nuova. Enum, non testo libero.
--
-- Regole della verifica statica: ogni REFERENCES punta a una tabella gia' creata; i tipi nuovi si
-- creano qui e si usano qui (non e' un ADD VALUE).

CREATE TYPE stato_richiesta_fornitore AS ENUM ('bozza', 'inviata', 'risposta', 'scaduta', 'annullata');
CREATE TYPE intento_messaggio AS ENUM (
    'rfq_cliente',        -- un cliente chiede un'offerta: e' la RFQ
    'offerta_promatec',   -- la nostra offerta al cliente (uscita)
    'rfq_fornitore',      -- la nostra richiesta a un fornitore (uscita)
    'offerta_fornitore',  -- il fornitore risponde con l'offerta
    'risposta_fornitore', -- il fornitore risponde senza offerta (conferma, tempi, precisazioni)
    'domanda_fornitore',  -- il fornitore chiede qualcosa
    'inoltro_interno',    -- un collega gira una mail
    'non_rfq',            -- newsletter, notifiche, spam, cortesie
    'incerto'             -- non si sa: decide una persona
);
CREATE TYPE regola_richiesta AS ENUM ('R0_reply', 'R1_conversazione', 'R3f_codice', 'RF_oggetto');

CREATE TABLE richiesta_fornitore (
    richiesta_id  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id     uuid NOT NULL REFERENCES thread_offerta,
    fornitore_id  uuid NOT NULL REFERENCES fornitore,
    lavorazione   varchar(30) REFERENCES lavorazione,
    codici        text[] NOT NULL DEFAULT '{}',          -- i codici cliente chiesti al fornitore
    messaggio_id  uuid REFERENCES messaggio,             -- la nostra mail, quando e' nota
    stato         stato_richiesta_fornitore NOT NULL DEFAULT 'bozza',
    inviata_il    timestamptz,
    risposta_il   timestamptz,
    note          text,
    creata_da     uuid REFERENCES utente,
    creata_il     timestamptz NOT NULL DEFAULT now()
);
-- una richiesta per (RFQ, fornitore, lavorazione); la lavorazione puo' mancare, e due NULL non
-- sarebbero uguali per un UNIQUE: l'indice sul COALESCE li rende uguali
CREATE UNIQUE INDEX ux_richiesta_fornitore ON richiesta_fornitore (thread_id, fornitore_id, COALESCE(lavorazione, ''));
CREATE INDEX ix_richiesta_fornitore_fornitore ON richiesta_fornitore (fornitore_id, stato);
CREATE INDEX ix_richiesta_fornitore_thread    ON richiesta_fornitore (thread_id);
CREATE INDEX ix_richiesta_fornitore_messaggio ON richiesta_fornitore (messaggio_id) WHERE messaggio_id IS NOT NULL;

COMMENT ON TABLE richiesta_fornitore IS
'Blocco 7B: la richiesta d''offerta a UN fornitore per UNA RFQ cliente (figlia della RFQ, non un thread
parallelo). messaggio_id e'' la nostra mail: la lega il marcatore CockpitRichiestaFornitore letto dalla Posta
inviata, o la conferma dell''operatore su una mail mandata a mano. La risposta del fornitore si aggancia alla
richiesta (messaggio.richiesta_fornitore_id) e alla RFQ (messaggio.thread_id) con una decisione.';

ALTER TABLE messaggio ADD COLUMN richiesta_fornitore_id uuid REFERENCES richiesta_fornitore;
CREATE INDEX ix_messaggio_richiesta ON messaggio (richiesta_fornitore_id) WHERE richiesta_fornitore_id IS NOT NULL;

ALTER TABLE bozza ADD COLUMN richiesta_fornitore_id uuid REFERENCES richiesta_fornitore;

-- la proposta: l'intento, e per la posta dei fornitori il bersaglio (una richiesta esistente, o
-- «richiesta a X per la RFQ Y» quando la nostra mail e' stata mandata a mano)
ALTER TABLE proposta_triage ADD COLUMN intento            intento_messaggio;
ALTER TABLE proposta_triage ADD COLUMN richiesta_proposta uuid REFERENCES richiesta_fornitore;
ALTER TABLE proposta_triage ADD COLUMN fornitore_proposto uuid REFERENCES fornitore;

-- i candidati verso una richiesta: come candidato_aggancio, ma il bersaglio e' una richiesta a un
-- fornitore. Piu' righe per messaggio, tutte visibili, nessuna scelta dal sistema (IB5)
CREATE TABLE candidato_richiesta (
    messaggio_id uuid NOT NULL REFERENCES messaggio,
    richiesta_id uuid NOT NULL REFERENCES richiesta_fornitore,
    regola       regola_richiesta NOT NULL,
    punteggio    smallint NOT NULL CHECK (punteggio BETWEEN 0 AND 100),
    evidenza     text NOT NULL,
    creato_il    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (messaggio_id, richiesta_id, regola)
);
CREATE INDEX ix_candidato_richiesta_msg ON candidato_richiesta (messaggio_id, punteggio DESC);

-- v_inbox: le colonne nuove vanno IN CODA (CREATE OR REPLACE VIEW non rinomina ne' riordina)
CREATE OR REPLACE VIEW v_inbox AS
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
       END AS controparte,
       COALESCE(pt.intento::text, '') AS triage_intento,   -- mai NULL: la riga si legge anche senza proposta
       m.richiesta_fornitore_id
FROM messaggio m
LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
LEFT JOIN cliente ct ON ct.cliente_id = t.cliente_id
LEFT JOIN dominio_cliente dc ON dc.dominio = lower(split_part(m.mittente_indirizzo, '@', 2))
LEFT JOIN cliente cd ON cd.cliente_id = dc.cliente_id
LEFT JOIN buyer b ON b.buyer_id = m.buyer_id
LEFT JOIN cliente cb ON cb.cliente_id = b.cliente_id
LEFT JOIN cliente cc ON cc.cliente_id = m.controparte_cliente_id
LEFT JOIN fornitore f ON f.fornitore_id = m.controparte_fornitore_id
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
LEFT JOIN LATERAL (SELECT * FROM proposta_triage p WHERE p.messaggio_id = m.messaggio_id AND p.stato = 'proposta'
                   ORDER BY (p.fonte = 'agente') DESC, p.creato_il DESC LIMIT 1) pt ON true
WHERE COALESCE(pr.n_caselle, 0) > 0 OR m.parent_messaggio_id IS NULL;

INSERT INTO schema_versione (versione) VALUES (15);
