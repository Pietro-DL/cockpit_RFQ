-- ============================================================================
--  0001_schema.sql — Cockpit RFQ, schema unico (PostgreSQL 17+)
--  Fonti: RFQ_plan.md §0.2, SPEC_Architettura_Cockpit_RFQ.md §0 e §4.2.
--  Regole:
--    - identificatori snake_case minuscoli (sqlc/pgx); nessuna colonna di stato in varchar: tutte ENUM
--    - i timestamp di FATTO (ReceivedTime, SentOn) non hanno DEFAULT now(); now() solo su registrato_il/creato_il
--      e sulle azioni umane
--    - idempotenza per chiave naturale: (canale, chiave_esterna) sui messaggi, chiave_idempotenza sui job,
--      (thread_id, sha256) sui documenti
--    - nessuna cancellazione: unito_in, stato terminale, attivo=false
--    - tre strati per i file: FATTO (allegato) → INTERPRETAZIONE (documento_proposta, riferimento_portale,
--      proposta_triage) → DECISIONE (documento, deroga_fabbisogno). La completezza è la vista v_fascicolo.
-- ============================================================================
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE schema_versione (
    versione     int PRIMARY KEY,
    applicata_il timestamptz NOT NULL DEFAULT now()
);

-- ============================================================================ 1. TIPI
CREATE TYPE ruolo_utente           AS ENUM ('admin','operatore','tecnico','consultazione');
CREATE TYPE tipo_buyer             AS ENUM ('buyer','tecnico','ordini','funzionale','altro');
CREATE TYPE origine_anagrafica     AS ENUM ('excel','censimento','auto','manuale');
CREATE TYPE canale                 AS ENUM ('outlook','whatsapp','telefono','portale','nota');
CREATE TYPE direzione              AS ENUM ('entrata','uscita');
CREATE TYPE aggancio               AS ENUM ('nessuno','auto_conversazione','auto_identificativo','operatore');
CREATE TYPE stato_thread           AS ENUM ('APERTA','CHIUSA');
CREATE TYPE scadenza_origine       AS ENUM ('mail','buyer','portale','stimata');       -- SPEC §0: da confermare
CREATE TYPE origine_identificativo AS ENUM ('proposta_oggetto','proposta_corpo','proposta_nome_file','proposta_step','manuale');
CREATE TYPE tipo_componente        AS ENUM ('finito','sottoassieme','sciolto','commerciale');
CREATE TYPE origine_componente     AS ENUM ('step','codice_rilevato','manuale');
CREATE TYPE esito_fattibilita      AS ENUM ('OK','KO','DUBBIO');
CREATE TYPE fase                   AS ENUM ('RICEVUTA','ATTESA_DISEGNI','FATTIBILITA','SCHEDA_COSTO','OFFERTE_FORN','OFFERTA_INVIATA',
                                            'ACCETTATA','DISTINTA_ERP','ORDINE','PRODUZIONE','PERSA','RESPINTA','SCADUTA');
CREATE TYPE esito_fase             AS ENUM ('OK','KO','RINVIATA');
CREATE TYPE natura_allegato        AS ENUM ('file','inline','elemento_outlook','collegamento');
CREATE TYPE origine_allegato       AS ENUM ('outlook','manuale','portale');
CREATE TYPE stato_allegato         AS ENUM ('grezzo','in_staging','analizzato','errore','ignorato');
CREATE TYPE tipo_documento         AS ENUM ('cad_3d','disegno_2d','sviluppo_dxf','capitolato','distinta_cliente','commerciale',
                                            'offerta_fornitore','offerta_promatec','ordine_cliente','corrispondenza','rumore','altro');
CREATE TYPE fonte_proposta         AS ENUM ('estensione','nome_file','step','cartiglio','regola_cliente','direzione','rumore','operatore');
CREATE TYPE stato_proposta         AS ENUM ('aperta','confermata','scartata','duplicato');
CREATE TYPE stato_rif_portale      AS ENUM ('da_scaricare','scaricato','non_trovato','ignorato');
CREATE TYPE esito_triage           AS ENUM ('nuova_rfq','aggancia','ignora');
CREATE TYPE fonte_triage           AS ENUM ('deterministico','agente');
CREATE TYPE stato_triage           AS ENUM ('proposta','accettata','rifiutata');
CREATE TYPE stato_nas              AS ENUM ('in_coda','scritto','errore');
CREATE TYPE tipo_job               AS ENUM ('sync_outlook','stage_allegato','analizza_allegato','copia_nas','crea_cartella_thread',
                                            'crea_bozza_outlook','apri_elemento_outlook','sposta_in_cartella','segna_letto','backup_db');
CREATE TYPE worker_tipo            AS ENUM ('server','outlook','analisi');
CREATE TYPE stato_job              AS ENUM ('pronto','in_corso','fatto','fallito');
CREATE TYPE tipo_bozza             AS ENUM ('risposta','rispondi_tutti','inoltro','nuovo','sollecito');
CREATE TYPE stato_bozza            AS ENUM ('in_coda','aperta','inviata','errore');
CREATE TYPE modo_regola            AS ENUM ('applica','proponi','spenta');

-- ============================================================================ 2. ANAGRAFICA E CATALOGHI
CREATE TABLE utente (
    utente_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sigla         varchar(10) NOT NULL UNIQUE,
    nome          varchar(80) NOT NULL,
    ufficio       varchar(40) NOT NULL,
    ruolo         ruolo_utente NOT NULL DEFAULT 'operatore',
    password_hash text,                                  -- bcrypt; NULL = non può fare login
    attivo        boolean NOT NULL DEFAULT true,
    creato_il     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessione (                                  -- cookie HttpOnly → utente; revocabile
    token          char(64) PRIMARY KEY,
    utente_id      uuid NOT NULL REFERENCES utente,
    creata_il      timestamptz NOT NULL DEFAULT now(),
    scade_il       timestamptz NOT NULL,
    ultimo_accesso timestamptz
);
CREATE INDEX ix_sessione_utente ON sessione (utente_id);

CREATE TABLE cliente (
    cliente_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cartella_nas    varchar(80)  NOT NULL UNIQUE,        -- nome cartella sotto PREVENTIVI DA FARE (es. 'LANDINI ARGO')
    ragione_sociale varchar(120) NOT NULL,
    profilo         varchar(30)  UNIQUE,                 -- chiave del set di regole (es. 'argo', 'toyota'); NULL = generico
    lingua          char(2),
    portale_url     varchar(300),                        -- SPEC §0: il portale è del cliente, non della RFQ
    portale_note    text,                                -- istruzioni, MAI credenziali
    regole          jsonb NOT NULL DEFAULT '{}',         -- regex codici, frasi portale, cartella_per_codice, finestra_aggancio_gg, sla_default_gg
    attivo          boolean NOT NULL DEFAULT true,
    note            text,
    creato_il       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dominio_cliente (                           -- dominio mittente → cliente (estensioni .it/.fr/.de)
    dominio    varchar(80) PRIMARY KEY CHECK (dominio = lower(dominio)),
    cliente_id uuid NOT NULL REFERENCES cliente
);

CREATE TABLE buyer (
    buyer_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cliente_id uuid NOT NULL REFERENCES cliente,
    cognome    varchar(60) NOT NULL,
    nome       varchar(60),
    email      varchar(120) UNIQUE CHECK (email IS NULL OR email = lower(email)),
    telefono   varchar(40),                              -- per note WhatsApp/telefono
    ruolo      varchar(60),
    tipo       tipo_buyer NOT NULL DEFAULT 'buyer',
    manda_file boolean NOT NULL DEFAULT false,
    lingua     char(2),
    confermato boolean NOT NULL DEFAULT true,
    origine    origine_anagrafica NOT NULL DEFAULT 'manuale',
    note       text,
    creato_il  timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (cliente_id, cognome, nome)
);

CREATE TABLE fase_catalogo (
    nome_fase   fase PRIMARY KEY,
    ordine      int NOT NULL UNIQUE,
    descrizione varchar(120) NOT NULL,
    ufficio     varchar(40) NOT NULL,
    sla_gg      int,
    terminale   boolean NOT NULL DEFAULT false
);
INSERT INTO fase_catalogo VALUES
 ('RICEVUTA',        10, 'Thread creato dal triage',                             'Sistema',       0, false),
 ('ATTESA_DISEGNI',  15, 'Fascicolo incompleto: portale o mail successiva',      'Commerciale',   3, false),
 ('FATTIBILITA',     20, 'Analisi fattibilità, processi e materiali (Luigi)',    'Tecnico',       2, false),
 ('SCHEDA_COSTO',    30, 'Compilazione SC (Francesco)',                          'Commerciale',   3, false),
 ('OFFERTE_FORN',    31, 'Quotazioni terzisti (Filippo), parallela',             'Acquisti',      5, false),
 ('OFFERTA_INVIATA', 40, 'SO inviata al cliente',                                'Commerciale',   0, false),
 ('ACCETTATA',       50, 'Il buyer accetta',                                     'Commerciale', NULL, false),
 ('DISTINTA_ERP',    60, 'Distinta e cicli nel gestionale (Meryem)',             'Uff. tecnico',  3, false),
 ('ORDINE',          70, 'Ordine con riferimento SO',                            'Sistema',       0, false),
 ('PRODUZIONE',      80, 'In produzione',                                        'Produzione', NULL, false),
 ('PERSA',           90, 'Offerta non accettata',                                '-',          NULL, true),
 ('RESPINTA',        91, 'Non fattibile',                                        '-',          NULL, true),
 ('SCADUTA',         92, 'Nessun esito dopo N giorni',                           '-',          NULL, true);

CREATE TABLE transizione (
    da              fase NOT NULL REFERENCES fase_catalogo,
    a               fase NOT NULL REFERENCES fase_catalogo,
    automatica      boolean NOT NULL,
    fatto_richiesto varchar(120),
    PRIMARY KEY (da, a)
);
INSERT INTO transizione VALUES
 ('RICEVUTA','FATTIBILITA',           true,  'v_fascicolo: nessuna riga bloccante con esito <> ok'),
 ('RICEVUTA','ATTESA_DISEGNI',        true,  'v_fascicolo: almeno una riga bloccante mancante/sul_portale'),
 ('ATTESA_DISEGNI','FATTIBILITA',     true,  'v_fascicolo: nessuna riga bloccante con esito <> ok (o deroga)'),
 ('FATTIBILITA','ATTESA_DISEGNI',     false, 'sollecito: particolare mancante'),
 ('FATTIBILITA','SCHEDA_COSTO',       false, 'Luigi: fattibilità OK'),
 ('FATTIBILITA','RESPINTA',           false, 'Luigi: non fattibile'),
 ('SCHEDA_COSTO','OFFERTE_FORN',      false, 'processi esterni nel ciclo'),
 ('SCHEDA_COSTO','OFFERTA_INVIATA',   false, 'mail in uscita con SO nnnn.pdf'),
 ('OFFERTE_FORN','SCHEDA_COSTO',      false, 'quotazioni rientrate'),
 ('OFFERTA_INVIATA','SCHEDA_COSTO',   false, 'rinegoziazione'),
 ('OFFERTA_INVIATA','ACCETTATA',      false, 'accettazione del buyer'),
 ('OFFERTA_INVIATA','ORDINE',         false, 'ordine con riferimento SO'),
 ('OFFERTA_INVIATA','PERSA',          false, 'rifiuto esplicito'),
 ('OFFERTA_INVIATA','SCADUTA',        false, 'timer'),
 ('ACCETTATA','DISTINTA_ERP',         false, 'accettazione confermata'),
 ('ACCETTATA','ORDINE',               false, 'ordine con riferimento SO'),
 ('DISTINTA_ERP','ORDINE',            false, 'ordine con riferimento SO'),
 ('ORDINE','PRODUZIONE',              false, 'distinta presente nel gestionale');

CREATE TABLE regola (                                    -- ogni automazione deterministica si spegne da sola
    regola_id         varchar(60) PRIMARY KEY,           -- 'argo.nome_file', 'triage.parole_chiave'
    descrizione       varchar(200) NOT NULL,
    modo              modo_regola NOT NULL DEFAULT 'proponi',
    soglia_correzioni numeric(4,3) NOT NULL DEFAULT 0.10,
    applicazioni      int NOT NULL DEFAULT 0,
    correzioni_umane  int NOT NULL DEFAULT 0,
    attiva_dal        date NOT NULL DEFAULT current_date
);

-- ============================================================================ 3. THREAD RFQ
CREATE TABLE thread_offerta (
    thread_id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cliente_id           uuid NOT NULL REFERENCES cliente,
    buyer_id             uuid REFERENCES buyer,
    canale               canale NOT NULL,                -- canale del primo messaggio
    data_inizio          timestamptz NOT NULL,           -- data_evento del PRIMO messaggio / creazione manuale
    ultimo_aggiornamento timestamptz,                    -- cache: ultimo messaggio agganciato (trigger)
    data_scadenza        date,
    scadenza_origine     scadenza_origine,
    oggetto              varchar(500),
    cartella_relativa    varchar(500),                   -- es. 'ACME\WIP\2026 09 08 Rossi Supporto cofano' (relativa alla radice NAS)
    cartella_creata      boolean NOT NULL DEFAULT false, -- job crea_cartella_thread eseguito
    priorita             smallint NOT NULL DEFAULT 1,
    campionatura         boolean NOT NULL DEFAULT false,
    stato                stato_thread NOT NULL DEFAULT 'APERTA',   -- CACHE dal trigger su fase_log, mai scritta dagli handler
    unito_in             uuid REFERENCES thread_offerta,
    note                 text,
    creato_da            uuid REFERENCES utente,
    creato_il            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_thread_cliente ON thread_offerta (cliente_id, stato);

CREATE TABLE identificativo_thread (                     -- codici PRODOTTO FINITO della RFQ (sottoassiemi/sciolti in componente)
    thread_id     uuid NOT NULL REFERENCES thread_offerta,
    codice        varchar(100) NOT NULL,
    origine       origine_identificativo NOT NULL,
    confidenza    smallint CHECK (confidenza BETWEEN 0 AND 100),
    confermato_da uuid REFERENCES utente,
    creato_il     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, codice)
);
CREATE INDEX ix_ident_codice ON identificativo_thread (codice);

CREATE TABLE componente (                                -- albero prodotto della RFQ; tipo='finito' e padre NULL = radice
    componente_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id         uuid NOT NULL REFERENCES thread_offerta,
    padre_id          uuid REFERENCES componente,
    codice            varchar(60) NOT NULL,
    rev               varchar(10),
    descrizione       varchar(200),
    qta               int NOT NULL DEFAULT 1,
    tipo              tipo_componente NOT NULL DEFAULT 'sciolto',
    origine           origine_componente NOT NULL DEFAULT 'manuale',
    materiale_testo   varchar(80),                       -- proposta dal cartiglio
    spessore_mm       numeric(6,2),
    peso_kg           numeric(10,3),
    esito_fattibilita esito_fattibilita,
    note_fattibilita  text,                              -- schermata E: le note di Luigi, sul componente non sulla carta
    confermato_da     uuid REFERENCES utente,
    creato_il         timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (thread_id, padre_id, codice, rev)
);
CREATE INDEX ix_componente_thread ON componente (thread_id);

CREATE TABLE fase_log (                                  -- la fase corrente del thread è la riga con fine IS NULL
    fase_log_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id       uuid NOT NULL REFERENCES thread_offerta,
    nome_fase       fase NOT NULL REFERENCES fase_catalogo,
    responsabile_id uuid REFERENCES utente,
    inizio          timestamptz NOT NULL,                -- timestamp del fatto; now() solo per azioni umane
    fine            timestamptz,
    esito           esito_fase,
    note            text,
    CHECK (fine IS NULL OR fine >= inizio)
);
CREATE UNIQUE INDEX ux_fase_aperta ON fase_log (thread_id) WHERE fine IS NULL;
CREATE INDEX ix_fase_thread ON fase_log (thread_id, inizio);

-- ============================================================================ 4. FATTO — MESSAGGI E ALLEGATI
CREATE TABLE conversazione (                             -- catena: Outlook ConversationID; WhatsApp: numero; telefono: generata
    conversazione_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    canale             canale NOT NULL,
    chiave_esterna     varchar(255) NOT NULL,
    thread_id          uuid REFERENCES thread_offerta,   -- NULL = conversazione orfana
    collegata_da       aggancio NOT NULL DEFAULT 'nessuno',
    primo_messaggio_il timestamptz NOT NULL,
    UNIQUE (canale, chiave_esterna)
);

CREATE TABLE messaggio (                                 -- ciò che è arrivato, qualunque canale; nessun giudizio
    messaggio_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    canale              canale NOT NULL,
    chiave_esterna      varchar(255) NOT NULL,           -- Internet Message-ID; 'wa:<id>'; 'tel:<uuid>'
    conversazione_id    uuid NOT NULL REFERENCES conversazione,
    parent_messaggio_id uuid REFERENCES messaggio,       -- .msg annidato (olEmbeddeditem) → messaggio figlio
    thread_id           uuid REFERENCES thread_offerta,  -- NULL = orfano. È la DECISIONE registrata, mai dedotta a runtime
    aggancio            aggancio NOT NULL DEFAULT 'nessuno',
    agganciato_da       uuid REFERENCES utente,          -- NULL = automatico
    agganciato_il       timestamptz,
    direzione           direzione NOT NULL,
    data_evento         timestamptz NOT NULL,            -- ReceivedTime / SentOn / ora della chiamata: mai now()
    mittente_nome       varchar(150),
    mittente_indirizzo  varchar(200),                    -- SMTP, numero WhatsApp o telefono
    buyer_id            uuid REFERENCES buyer,
    destinatari         jsonb NOT NULL DEFAULT '[]',     -- [{"nome":..,"indirizzo":..,"tipo":"a|cc|ccn"}]
    oggetto             varchar(500),
    corpo_testo         text,                            -- testo piano
    corpo_html          text,                            -- HTML originale (sanificato lato server prima del rendering)
    lingua              char(2),
    importanza          smallint,
    nota_operatore      text,                            -- telefono/whatsapp: cosa è stato detto
    n_allegati          smallint NOT NULL DEFAULT 0,     -- cache (trigger): allegati diretti non inline
    registrato_il       timestamptz NOT NULL DEFAULT now(),
    registrato_da       uuid REFERENCES utente,          -- NULL = worker
    UNIQUE (canale, chiave_esterna)
);
CREATE INDEX ix_msg_thread ON messaggio (thread_id, data_evento);
CREATE INDEX ix_msg_orfani ON messaggio (data_evento DESC) WHERE thread_id IS NULL;
CREATE INDEX ix_msg_conv   ON messaggio (conversazione_id);
CREATE INDEX ix_msg_data   ON messaggio (data_evento DESC);

CREATE TABLE messaggio_outlook (                         -- satellite: solo ciò che serve a ritrovare l'item via COM
    messaggio_id       uuid PRIMARY KEY REFERENCES messaggio,
    entry_id           text NOT NULL,                    -- cambia se l'item viene spostato: aggiornato a ogni sync
    store_id           text NOT NULL,                    -- Exchange: ~600 caratteri
    conversation_id    varchar(255),
    conversation_index text,                             -- +10 caratteri a ogni risposta
    in_reply_to        varchar(255),
    riferimenti        text[],                           -- header References
    cartella           varchar(200),
    categorie          text[],
    non_letto          boolean NOT NULL DEFAULT false,
    flag_stato         smallint,                         -- OlFlagStatus
    aggiornato_il      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_msgout_conv ON messaggio_outlook (conversation_id);

CREATE TABLE sync_cursore (                              -- cursore per cartella Outlook letta dal worker
    cartella        varchar(200) PRIMARY KEY,            -- es. 'Inbox', 'Sent Items'
    ultimo_received timestamptz,
    ultimo_sync     timestamptz,
    n_messaggi      int NOT NULL DEFAULT 0,
    errore          text
);

CREATE TABLE allegato (                                  -- scritto SOLO dal worker; mai modificato dall'operatore
    allegato_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    messaggio_id   uuid NOT NULL REFERENCES messaggio,
    contenitore_id uuid REFERENCES allegato,             -- NULL = allegato diretto; altrimenti voce estratta da quello zip
    indice         smallint NOT NULL,                    -- posizione nel messaggio o nello zip
    nome_file      varchar(300) NOT NULL,
    path_interno   varchar(500),                         -- percorso dentro lo zip
    estensione     varchar(10),
    content_type   varchar(120),
    natura         natura_allegato NOT NULL DEFAULT 'file',
    origine        origine_allegato NOT NULL DEFAULT 'outlook',
    bytes          bigint,
    sha256         char(64),
    stato          stato_allegato NOT NULL DEFAULT 'grezzo',
    path_staging   varchar(500),                         -- copia locale, prima del NAS
    errore         text,
    ricevuto_il    timestamptz NOT NULL,
    caricato_da    uuid REFERENCES utente,               -- upload manuale/portale
    UNIQUE NULLS NOT DISTINCT (messaggio_id, contenitore_id, indice)
);
CREATE INDEX ix_allegato_sha ON allegato (sha256);
CREATE INDEX ix_allegato_msg ON allegato (messaggio_id);

CREATE TABLE hash_rumore (                               -- SPEC §6.4: loghi/firme già scartati per dominio mittente
    sha256      char(64) NOT NULL,
    dominio     varchar(80) NOT NULL,
    n_visto     int NOT NULL DEFAULT 1,
    scartato_da uuid REFERENCES utente,
    ultimo_il   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (sha256, dominio)
);

-- ============================================================================ 5. INTERPRETAZIONE
CREATE TABLE documento_proposta (                        -- "questo allegato è un 2D del codice X rev 4, confidenza 92"
    proposta_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    allegato_id   uuid NOT NULL UNIQUE REFERENCES allegato,
    thread_id     uuid REFERENCES thread_offerta,
    tipo_proposto tipo_documento NOT NULL,
    codice        varchar(60),
    rev           varchar(10),
    componente_id uuid REFERENCES componente,
    confidenza    smallint NOT NULL CHECK (confidenza BETWEEN 0 AND 100),
    fonte         fonte_proposta NOT NULL,
    regola_id     varchar(60) REFERENCES regola,
    dettagli      jsonb NOT NULL DEFAULT '{}',           -- PRODUCT(...) dello STEP, testo cartiglio, regex applicata
    stato         stato_proposta NOT NULL DEFAULT 'aperta',
    deciso_da     uuid REFERENCES utente,
    deciso_il     timestamptz,
    creato_il     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_proposta_thread_aperte ON documento_proposta (thread_id) WHERE stato = 'aperta';

CREATE TABLE riferimento_portale (                       -- "vi abbiamo caricato i CAD sul portale": un file atteso che ancora non abbiamo
    rif_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    messaggio_id uuid NOT NULL REFERENCES messaggio,
    thread_id    uuid REFERENCES thread_offerta,
    codice       varchar(60),
    tipo_atteso  tipo_documento,
    url          varchar(500),
    testo_citato text NOT NULL,
    stato        stato_rif_portale NOT NULL DEFAULT 'da_scaricare',
    scaricato_da uuid REFERENCES utente,
    scaricato_il timestamptz,
    creato_il    timestamptz NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (messaggio_id, codice, tipo_atteso)
);
CREATE INDEX ix_rif_thread ON riferimento_portale (thread_id) WHERE stato = 'da_scaricare';

CREATE TABLE proposta_triage (                           -- il motore di flagging: sempre e solo un suggerimento
    triage_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    messaggio_id      uuid NOT NULL REFERENCES messaggio,
    esito             esito_triage NOT NULL,
    thread_proposto   uuid REFERENCES thread_offerta,
    cliente_proposto  uuid REFERENCES cliente,
    buyer_proposto    uuid REFERENCES buyer,
    identificativi    text[] NOT NULL DEFAULT '{}',
    scadenza_proposta date,
    confidenza        smallint NOT NULL CHECK (confidenza BETWEEN 0 AND 100),
    motivi            jsonb NOT NULL DEFAULT '[]',       -- ["oggetto corrisponde alla regola Argo", "mittente è buyer ACME"]
    fonte             fonte_triage NOT NULL,
    stato             stato_triage NOT NULL DEFAULT 'proposta',
    deciso_da         uuid REFERENCES utente,
    deciso_il         timestamptz,
    creato_il         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (messaggio_id, fonte)
);

-- ============================================================================ 6. DECISIONE
CREATE TABLE documento (                                 -- il file tecnico confermato: cosa è, di quale pezzo, dove va sul NAS
    documento_id  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id     uuid NOT NULL REFERENCES thread_offerta,
    componente_id uuid REFERENCES componente,            -- NULL per documenti "commerciali" del thread (offerta, capitolato)
    tipo          tipo_documento NOT NULL,
    codice        varchar(60),
    rev           varchar(10),
    nome_file     varchar(300) NOT NULL,                 -- nome sul NAS (di norma l'originale; rinominabile)
    estensione    varchar(10) NOT NULL,
    sha256        char(64) NOT NULL,
    bytes         bigint,
    path_relativo varchar(500) NOT NULL,                 -- relativo a thread_offerta.cartella_relativa
    stato_nas     stato_nas NOT NULL DEFAULT 'in_coda',
    errore_nas    text,
    scritto_il    timestamptz,
    confermato_da uuid NOT NULL REFERENCES utente,
    confermato_il timestamptz NOT NULL DEFAULT now(),
    sostituito_da uuid REFERENCES documento,             -- nuova revisione dello stesso disegno
    nota          text,
    UNIQUE (thread_id, sha256)                           -- stesso file ricevuto due volte = un documento, più provenienze
);
CREATE INDEX ix_doc_thread     ON documento (thread_id, tipo);
CREATE INDEX ix_doc_componente ON documento (componente_id);

CREATE TABLE documento_provenienza (                     -- da dove è arrivata ogni copia del documento
    documento_id uuid NOT NULL REFERENCES documento,
    allegato_id  uuid REFERENCES allegato,
    rif_id       uuid REFERENCES riferimento_portale,
    messaggio_id uuid REFERENCES messaggio,
    ricevuto_il  timestamptz NOT NULL,
    caricato_da  uuid REFERENCES utente,
    PRIMARY KEY (documento_id, ricevuto_il),
    CHECK (num_nonnulls(allegato_id, rif_id, messaggio_id) >= 1)
);

CREATE TABLE cartella_documento (                        -- layout NAS: sottocartella per tipo, sotto la cartella del thread
    tipo          tipo_documento PRIMARY KEY,
    sottocartella varchar(80) NOT NULL,                  -- '' = radice della cartella RFQ
    per_codice    boolean NOT NULL DEFAULT false         -- ulteriore sottocartella per codice
);
INSERT INTO cartella_documento VALUES
 ('cad_3d','ELENCO DISEGNI',true), ('disegno_2d','ELENCO DISEGNI',true), ('sviluppo_dxf','ELENCO DISEGNI',true),
 ('distinta_cliente','ELENCO DISEGNI',false), ('capitolato','CAPITOLATI',false), ('commerciale','',false),
 ('offerta_fornitore','OFFERTE FORNITORI',false), ('offerta_promatec','',false),
 ('ordine_cliente','ORDINE',false), ('corrispondenza','MAIL',false), ('rumore','',false), ('altro','ALTRO',false);

CREATE TABLE fabbisogno_documento (                      -- cosa deve esserci nel fascicolo perché la fattibilità possa iniziare
    fabbisogno_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cliente_id      uuid REFERENCES cliente,             -- NULL = regola di default; una riga per cliente la sovrascrive
    tipo_componente tipo_componente NOT NULL,
    tipo            tipo_documento NOT NULL,
    bloccante       boolean NOT NULL DEFAULT true,
    UNIQUE NULLS NOT DISTINCT (cliente_id, tipo_componente, tipo)
);
INSERT INTO fabbisogno_documento (cliente_id, tipo_componente, tipo, bloccante) VALUES
 (NULL,'finito','cad_3d',true),        (NULL,'finito','disegno_2d',true),
 (NULL,'sottoassieme','cad_3d',true),  (NULL,'sottoassieme','disegno_2d',true),
 (NULL,'sciolto','disegno_2d',true),   (NULL,'sciolto','sviluppo_dxf',false);

CREATE TABLE deroga_fabbisogno (                         -- "il DXF lo facciamo noi": procede senza il documento
    deroga_id     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id     uuid NOT NULL REFERENCES thread_offerta,
    componente_id uuid NOT NULL REFERENCES componente,
    tipo          tipo_documento NOT NULL,
    motivo        text NOT NULL,
    utente_id     uuid NOT NULL REFERENCES utente,
    creata_il     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (componente_id, tipo)
);

-- ============================================================================ 7. USCITA — BOZZE OUTLOOK
CREATE TABLE bozza (                                     -- mail preparata dal Cockpit e aperta in Outlook; l'invio resta manuale
    bozza_id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id            uuid REFERENCES thread_offerta,
    in_risposta_a        uuid REFERENCES messaggio,
    tipo                 tipo_bozza NOT NULL,
    destinatari          jsonb NOT NULL DEFAULT '[]',
    oggetto              varchar(500),
    corpo                text,
    documenti            uuid[] NOT NULL DEFAULT '{}',   -- documenti del fascicolo da allegare
    entry_id             text,                           -- valorizzato dal worker quando la bozza esiste in Outlook
    stato                stato_bozza NOT NULL DEFAULT 'in_coda',
    errore               text,
    inviata_messaggio_id uuid REFERENCES messaggio,      -- quando il sync vede la mail in Posta inviata
    creata_da            uuid NOT NULL REFERENCES utente,
    creata_il            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_bozza_thread ON bozza (thread_id);

-- ============================================================================ 8. INFRA — CODA JOB
CREATE TABLE job (
    job_id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tipo               tipo_job NOT NULL,
    worker_tipo        worker_tipo NOT NULL,
    payload            jsonb NOT NULL DEFAULT '{}',
    chiave_idempotenza varchar(300) UNIQUE,
    stato              stato_job NOT NULL DEFAULT 'pronto',
    priorita           smallint NOT NULL DEFAULT 5,      -- 1 = urgente (azione utente), 9 = fondo
    tentativi          int NOT NULL DEFAULT 0,
    max_tentativi      int NOT NULL DEFAULT 5,
    non_prima_di       timestamptz NOT NULL DEFAULT now(),
    lease_fino_a       timestamptz,
    worker_id          varchar(80),
    risultato          jsonb,
    errore             text,
    creato_il          timestamptz NOT NULL DEFAULT now(),
    aggiornato_il      timestamptz NOT NULL DEFAULT now(),
    chiuso_il          timestamptz
);
CREATE INDEX ix_job_pronti  ON job (worker_tipo, priorita, job_id) WHERE stato = 'pronto';
CREATE INDEX ix_job_incorso ON job (lease_fino_a) WHERE stato = 'in_corso';

-- ============================================================================ 9. FUNZIONI E TRIGGER
-- fase_log → thread.stato (CHIUSA se la fase aperta è terminale)
CREATE OR REPLACE FUNCTION aggiorna_stato_thread() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE t uuid := COALESCE(NEW.thread_id, OLD.thread_id);
BEGIN
    UPDATE thread_offerta th SET stato = CASE
        WHEN EXISTS (SELECT 1 FROM fase_log f JOIN fase_catalogo c ON c.nome_fase = f.nome_fase
                     WHERE f.thread_id = t AND f.fine IS NULL AND c.terminale) THEN 'CHIUSA'::stato_thread
        ELSE 'APERTA'::stato_thread END
    WHERE th.thread_id = t;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_stato_thread AFTER INSERT OR UPDATE OR DELETE ON fase_log
    FOR EACH ROW EXECUTE FUNCTION aggiorna_stato_thread();

-- messaggio agganciato → thread.ultimo_aggiornamento
CREATE OR REPLACE FUNCTION aggiorna_ultimo_messaggio() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.thread_id IS NOT NULL THEN
        UPDATE thread_offerta SET ultimo_aggiornamento = GREATEST(COALESCE(ultimo_aggiornamento, NEW.data_evento), NEW.data_evento)
         WHERE thread_id = NEW.thread_id;
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_ultimo_messaggio AFTER INSERT OR UPDATE OF thread_id ON messaggio
    FOR EACH ROW EXECUTE FUNCTION aggiorna_ultimo_messaggio();

-- allegato diretto non inline → messaggio.n_allegati
CREATE OR REPLACE FUNCTION aggiorna_n_allegati() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m uuid := COALESCE(NEW.messaggio_id, OLD.messaggio_id);
BEGIN
    UPDATE messaggio SET n_allegati = (SELECT count(*) FROM allegato a
                                        WHERE a.messaggio_id = m AND a.contenitore_id IS NULL AND a.natura IN ('file','elemento_outlook'))
     WHERE messaggio_id = m;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_n_allegati AFTER INSERT OR UPDATE OF natura OR DELETE ON allegato
    FOR EACH ROW EXECUTE FUNCTION aggiorna_n_allegati();

-- job: aggiornato_il
CREATE OR REPLACE FUNCTION tocca_job() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.aggiornato_il := now(); RETURN NEW; END $$;
CREATE TRIGGER trg_tocca_job BEFORE UPDATE ON job FOR EACH ROW EXECUTE FUNCTION tocca_job();

-- ============================================================================ 10. VISTE
-- fase corrente per thread, con SLA
CREATE VIEW v_thread_fase AS
SELECT t.thread_id, f.fase_log_id, f.nome_fase, f.inizio, f.responsabile_id, c.ordine, c.sla_gg, c.terminale,
       EXTRACT(day FROM now() - f.inizio)::int AS gg_in_fase,
       CASE WHEN c.terminale THEN 'bianco'
            WHEN c.sla_gg IS NULL THEN 'bianco'
            WHEN now() - f.inizio > make_interval(days => c.sla_gg) THEN 'rosso'
            WHEN now() - f.inizio > make_interval(days => GREATEST(c.sla_gg - 1, 0)) THEN 'giallo'
            ELSE 'verde' END AS semaforo
FROM thread_offerta t
JOIN fase_log f ON f.thread_id = t.thread_id AND f.fine IS NULL
JOIN fase_catalogo c ON c.nome_fase = f.nome_fase;

-- la matrice del fascicolo: una riga per (componente, tipo richiesto), con l'esito calcolato.
-- Regola per cliente sovrascrive il default; nessuna colonna di stato scritta a mano.
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

-- bloccanti aperti per thread: 0 = "Avvia fattibilità" abilitato
CREATE VIEW v_thread_bloccanti AS
SELECT t.thread_id,
       count(v.componente_id) FILTER (WHERE v.bloccante AND v.esito NOT IN ('ok','derogato')) AS n_bloccanti,
       count(v.componente_id) FILTER (WHERE v.esito = 'da_confermare')                      AS n_da_confermare,
       count(v.componente_id) FILTER (WHERE v.esito = 'sul_portale')                        AS n_sul_portale,
       count(v.componente_id) FILTER (WHERE v.esito = 'manca')                              AS n_mancanti
FROM thread_offerta t LEFT JOIN v_fascicolo v ON v.thread_id = t.thread_id
GROUP BY t.thread_id;

-- Inbox: un messaggio per riga con il cliente riconosciuto dal dominio e i contatori della schermata A
CREATE VIEW v_inbox AS
SELECT m.messaggio_id, m.canale, m.direzione, m.data_evento, m.thread_id, m.aggancio, m.conversazione_id,
       m.mittente_nome, m.mittente_indirizzo, m.oggetto, m.n_allegati, m.buyer_id,
       lower(split_part(m.mittente_indirizzo, '@', 2)) AS dominio,
       COALESCE(t.cliente_id, dc.cliente_id, b.cliente_id) AS cliente_id,
       COALESCE(ct.cartella_nas, cd.cartella_nas, cb.cartella_nas) AS cliente,
       b.cognome AS buyer_cognome,
       (SELECT count(*) FROM riferimento_portale r WHERE r.messaggio_id = m.messaggio_id) AS n_rif_portale,
       (SELECT count(*) FROM allegato a WHERE a.messaggio_id = m.messaggio_id AND a.contenitore_id IS NULL
          AND a.natura = 'file' AND lower(a.estensione) IN ('stp','step','sldprt','sldasm','igs','iges','dxf','dwg','pdf','tif','tiff','zip','7z','rar')) AS n_cad,
       pt.esito AS triage_esito, pt.confidenza AS triage_confidenza, pt.motivi AS triage_motivi, pt.thread_proposto,
       mo.non_letto, mo.cartella AS cartella_outlook, mo.entry_id
FROM messaggio m
LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
LEFT JOIN cliente ct ON ct.cliente_id = t.cliente_id
LEFT JOIN dominio_cliente dc ON dc.dominio = lower(split_part(m.mittente_indirizzo, '@', 2))
LEFT JOIN cliente cd ON cd.cliente_id = dc.cliente_id
LEFT JOIN buyer b ON b.buyer_id = m.buyer_id
LEFT JOIN cliente cb ON cb.cliente_id = b.cliente_id
LEFT JOIN messaggio_outlook mo ON mo.messaggio_id = m.messaggio_id
LEFT JOIN LATERAL (SELECT * FROM proposta_triage p WHERE p.messaggio_id = m.messaggio_id AND p.stato = 'proposta'
                   ORDER BY (p.fonte = 'agente') DESC, p.creato_il DESC LIMIT 1) pt ON true
WHERE m.parent_messaggio_id IS NULL;

-- Cruscotto: un thread per riga
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

INSERT INTO schema_versione (versione) VALUES (1);
