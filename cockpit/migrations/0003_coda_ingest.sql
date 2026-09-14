-- ============================================================================
--  0003_coda_ingest.sql — Cockpit RFQ, fase 1 del piano di correzione
--
--  Tre cose, tutte a servizio della fase 1 «coda e ingest che non si bloccano e non si duplicano»:
--
--  1. Il TENTATIVO diventa un'entità identificata (§2.3). Finora un job «in corso» era identificato
--     solo da worker_id: un tentativo scaduto e uno nuovo erano indistinguibili, quindi il risultato
--     tardivo del primo poteva sovrascrivere il lavoro del secondo. Con lease_token, avviato_il e
--     durata_max_s esiste un predicato unico di validità che vale per OGNI scrittura proveniente da
--     un worker: heartbeat, result ok, result errore, ingest e aggiornamento dei cursori.
--  2. Un elemento che il database rifiuta non fa più fallire il lotto intero: finisce in
--     ingest_scarto, con il payload che permette di riprovarlo senza ripassare da Outlook (§2.4).
--  3. Gli identificativi non vengono più troncati: chiave_esterna diventa text. Un Message-ID
--     troncato a 255 caratteri è un'identità sbagliata, cioè un duplicato o una fusione di due
--     messaggi diversi (N34).
--
--  Nessun valore di enum aggiunto qui viene usato qui (regola S4): 'annullato' e 'rileggi_elemento'
--  entrano in uso dal codice, dopo che questa migrazione è stata applicata.
-- ============================================================================

-- ---------------------------------------------------------------- il tentativo come entità
ALTER TABLE job ADD COLUMN casella_id    uuid REFERENCES casella;      -- NULL = nessun vincolo di casella
ALTER TABLE job ADD COLUMN postazione_id uuid REFERENCES postazione;   -- job interattivo: la postazione del richiedente
ALTER TABLE job ADD COLUMN richiesto_da  uuid REFERENCES utente;
ALTER TABLE job ADD COLUMN lease_s       int NOT NULL DEFAULT 120;     -- durata di un singolo lease
ALTER TABLE job ADD COLUMN durata_max_s  int NOT NULL DEFAULT 1800;    -- durata massima dell'intero tentativo
ALTER TABLE job ADD COLUMN lease_token   uuid;                         -- identifica IL TENTATIVO, non il worker
ALTER TABLE job ADD COLUMN avviato_il    timestamptz;                  -- inizio del tentativo corrente
ALTER TABLE job ADD COLUMN scade_il      timestamptz;                  -- job interattivo: oltre questa ora non serve più

COMMENT ON COLUMN job.lease_token IS
    'Identifica il tentativo. Ogni scrittura che arriva da un worker deve esibirlo: un result tardivo di un tentativo scaduto trova zero righe e riceve 409, invece di applicarsi al tentativo nuovo.';
COMMENT ON COLUMN job.durata_max_s IS
    'Un worker che rinnova il lease ogni 30 s potrebbe tenere un job per sempre. Oltre avviato_il + durata_max_s il tentativo non vale più, anche con il lease fresco.';

CREATE INDEX ix_job_scadenza ON job (scade_il) WHERE scade_il IS NOT NULL;
CREATE INDEX ix_job_token    ON job (lease_token) WHERE lease_token IS NOT NULL;

-- ---------------------------------------------------------------- identificativi integri (N34)
ALTER TABLE messaggio     ALTER COLUMN chiave_esterna TYPE text;
ALTER TABLE conversazione ALTER COLUMN chiave_esterna TYPE text;

-- ---------------------------------------------------------------- scarti dell'ingest (§2.4)
CREATE TABLE ingest_scarto (
    scarto_id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    casella_id  uuid NOT NULL REFERENCES casella,
    entry_id    text NOT NULL,
    cartella    varchar(200),
    message_id  text,
    ricevuto_il timestamptz,
    oggetto     varchar(500),
    origine     varchar(10) NOT NULL,       -- 'ingest' = payload completo | 'lettura' = elemento non convertito dal worker
    payload     jsonb NOT NULL,             -- 'ingest': il MessaggioIn intero; 'lettura': solo i riferimenti
    errore      text NOT NULL,
    tentativi   int NOT NULL DEFAULT 1,
    primo_il    timestamptz NOT NULL DEFAULT now(),
    ultimo_il   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_scarto_origine CHECK (origine IN ('ingest','lettura')),
    CONSTRAINT ux_scarto_casella_entry UNIQUE (casella_id, entry_id)
);
CREATE INDEX ix_scarto_origine ON ingest_scarto (origine, ultimo_il DESC);
COMMENT ON TABLE ingest_scarto IS
    'Un elemento che il database rifiuta non fa fallire il lotto: viene annullato fino al savepoint e registrato qui. Con origine=ingest il payload basta a riprovarlo senza Outlook; con origine=lettura serve rileggere l''elemento dalla casella.';

-- ---------------------------------------------------------------- log delle decisioni di aggancio (1.9, fase 3)
CREATE TABLE messaggio_aggancio_log (
    log_id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    messaggio_id uuid NOT NULL REFERENCES messaggio,
    thread_id    uuid REFERENCES thread_offerta,
    azione       varchar(12) NOT NULL,      -- 'aggancia' | 'sgancia' | 'ignora' | 'propaga'
    utente_id    uuid REFERENCES utente,    -- NULL = automatico
    motivo       text,
    eseguito_il  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_aggancio_log_azione CHECK (azione IN ('aggancia','sgancia','ignora','propaga'))
);
CREATE INDEX ix_aggancio_log_messaggio ON messaggio_aggancio_log (messaggio_id, eseguito_il DESC);

-- ---------------------------------------------------------------- fatti dell'analisi riusabili (1.12)
CREATE TABLE analisi_fatti (
    sha256                char(64) NOT NULL,
    versione_analizzatore smallint NOT NULL,
    hash_configurazione   char(64) NOT NULL,   -- sha256 del dizionario dei termini e dei parametri mandati al worker
    fatti                 jsonb NOT NULL,
    calcolato_il          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (sha256, versione_analizzatore, hash_configurazione)
);
COMMENT ON TABLE analisi_fatti IS
    'Lo stesso file analizzato due volte dà gli stessi fatti: si calcolano una volta per (contenuto, versione, configurazione) e si distribuiscono a tutte le proposte aperte. La tabella nasce qui perché la migrazione precede il codice che la usa.';

-- ---------------------------------------------------------------- valori di enum (usati dal codice, non da qui)
ALTER TYPE stato_job ADD VALUE 'annullato';
ALTER TYPE tipo_job  ADD VALUE 'rileggi_elemento';

INSERT INTO schema_versione (versione) VALUES (3);
