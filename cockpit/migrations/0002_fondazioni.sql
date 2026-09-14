-- ============================================================================
--  0002_fondazioni.sql — Cockpit RFQ, fase 0 del piano di correzione (voce 0.6)
--
--  Introduce le quattro tabelle su cui poggiano le fasi 1 e 2, SENZA toccare `messaggio`:
--    casella            una casella di posta (personale o condivisa); un solo UUID anche se più
--                       postazioni la vedono. Le presenze per casella arrivano nella 0004.
--    postazione         un PC con Outlook e un worker; il routing dei job interattivi la usa (fase 2).
--    worker_credenziale credenziale individuale per worker + elenco delle caselle autorizzate.
--    casella_store      come OGNI postazione vede una casella nel PROPRIO profilo Outlook (N44):
--                       lo StoreID è locale al profilo e non è un riferimento valido su un altro PC.
--
--  Nessun valore di enum viene aggiunto qui (regola S4): `canale` e `worker_tipo` esistono dalla 0001.
--  Nessuna REFERENCES punta a tabelle create dopo questo file (regola S1).
-- ============================================================================

-- ---------------------------------------------------------------- caselle
CREATE TABLE casella (
    casella_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    canale        canale NOT NULL DEFAULT 'outlook',
    indirizzo     varchar(200) NOT NULL,            -- sempre minuscolo: 'commerciale@azienda.it'
    nome          varchar(80) NOT NULL,             -- etichetta in UI: 'Commerciale'
    condivisa     boolean NOT NULL DEFAULT false,   -- true = cassetta condivisa Exchange
    utente_id     uuid REFERENCES utente,           -- proprietario; NULL per la condivisa
    attiva        boolean NOT NULL DEFAULT true,
    creato_il     timestamptz NOT NULL DEFAULT now(),
    aggiornato_il timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_casella_indirizzo_minuscolo CHECK (indirizzo = lower(indirizzo)),
    CONSTRAINT ck_casella_condivisa_senza_utente CHECK (NOT condivisa OR utente_id IS NULL),
    CONSTRAINT ux_casella_canale_indirizzo UNIQUE (canale, indirizzo)
);
COMMENT ON TABLE  casella IS 'Casella di posta: un solo UUID anche quando più postazioni la aprono (piano §2.1).';
COMMENT ON COLUMN casella.utente_id IS 'Proprietario di una casella personale; NULL per una condivisa (vincolo ck_casella_condivisa_senza_utente).';

-- ---------------------------------------------------------------- postazioni
CREATE TABLE postazione (
    postazione_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    nome_host     varchar(80) NOT NULL UNIQUE,      -- sempre maiuscolo: 'PC-FRANCESCO'
    descrizione   varchar(120) NOT NULL DEFAULT '',
    utente_id     uuid REFERENCES utente,           -- chi ci lavora abitualmente
    attiva        boolean NOT NULL DEFAULT true,
    creato_il     timestamptz NOT NULL DEFAULT now(),
    aggiornato_il timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_postazione_host_maiuscolo CHECK (nome_host = upper(nome_host))
);
COMMENT ON TABLE postazione IS 'PC con Outlook classico e un worker: i job interattivi vanno alla postazione del richiedente (piano §2.2).';

-- ---------------------------------------------------------------- credenziali dei worker
CREATE TABLE worker_credenziale (
    worker_nome   varchar(80) PRIMARY KEY,          -- 'outlook@PC-FRANCESCO'
    worker_tipo   worker_tipo NOT NULL,
    token_hash    char(64) NOT NULL,                -- sha256 esadecimale del token: il token non è mai in DB
    postazione_id uuid REFERENCES postazione,
    caselle       uuid[] NOT NULL DEFAULT '{}',     -- caselle autorizzate; il claim interseca con questo elenco
    attivo        boolean NOT NULL DEFAULT true,
    creato_il     timestamptz NOT NULL DEFAULT now(),
    aggiornato_il timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_worker_token_hash_hex CHECK (token_hash ~ '^[0-9a-f]{64}$')
);
CREATE INDEX ix_worker_credenziale_postazione ON worker_credenziale (postazione_id);
COMMENT ON COLUMN worker_credenziale.caselle IS 'Autorizzazione: il server interseca con le caselle dichiarate dal claim e non si fida del JSON (piano §2.2).';

-- ---------------------------------------------------------------- store locali per postazione
CREATE TABLE casella_store (
    postazione_id uuid NOT NULL REFERENCES postazione,
    casella_id    uuid NOT NULL REFERENCES casella,
    store_id      text NOT NULL,                    -- StoreID LOCALE al profilo di quella postazione
    rilevato_il   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (postazione_id, casella_id)
);
COMMENT ON TABLE casella_store IS 'N44: lo StoreID appartiene al profilo, non alla casella. Ogni worker registra qui come vede le proprie caselle; i payload dei job portano casella_id + entry_id, mai uno store_id estraneo.';

INSERT INTO schema_versione (versione) VALUES (2);
