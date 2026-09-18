-- 0014_fornitori.sql — Blocco 7A: fornitori, controparte del messaggio, convenzioni di codice
-- (D32, D33, D39). Schema approvato il 18/09/2026 come impostazione.
--
-- Fino a qui il Cockpit conosceva una sola specie di interlocutore: il cliente. Chiunque scrivesse
-- da un dominio non censito era «mittente non censito», e una richiesta d'offerta di un FORNITORE
-- («RICHIESTA D'OFFERTA ... TG FIORE», con il PDF della nostra richiesta allegato) aveva le stesse
-- parole e gli stessi allegati di una RFQ del cliente: il triage la proponeva come RFQ nuova.
--
-- Da qui la controparte e' un fatto scritto sul messaggio, risolto dall'anagrafica con una
-- precedenza fissa (contatto esatto > dominio > sconosciuto, ambiguo se doppia), e un mittente
-- riconosciuto come fornitore in entrata non puo' per costruzione produrre una RFQ cliente.
--
-- Regole del migratore: un file = una transazione; gli enum NUOVI si usano nello stesso file (il
-- divieto vale solo per ALTER TYPE ... ADD VALUE); ogni REFERENCES punta a una tabella gia' creata;
-- il file termina con INSERT INTO schema_versione.

-- ============================================================ 1. I FORNITORI (D32)

-- La macro-classificazione e' un ENUM, non una stringa libera e non un JSONB (richiesta del 18/09):
--   materie_prime  quello che l'azienda COMPRA (lastre, tubi, ...)
--   processi       lavorazioni assegnate fuori (tornitura, zincatura, trattamenti, ...)
--   verniciatore   i verniciatori: legati ai processi di verniciatura E ai clienti (sotto)
CREATE TYPE tipo_fornitore AS ENUM ('materie_prime', 'processi', 'verniciatore');

CREATE TABLE fornitore (
    fornitore_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ragione_sociale varchar(150) NOT NULL,
    tipo            tipo_fornitore NOT NULL,
    lingua          char(2),
    note            text,                       -- «chiede i 3D prima di quotare», «non fa lotti sotto 100»
    attivo          boolean NOT NULL DEFAULT true,
    creato_il       timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_fornitore_ragione_sociale ON fornitore (lower(ragione_sociale));

-- Stesse regole di dominio_cliente: minuscolo, senza «@». Un dominio puo' stare QUI e in
-- dominio_cliente (un gruppo che compra e vende): non si vieta, il resolver risponde `ambiguo`.
CREATE TABLE dominio_fornitore (
    dominio      varchar(120) PRIMARY KEY CHECK (dominio = lower(dominio) AND position('@' IN dominio) = 0),
    fornitore_id uuid NOT NULL REFERENCES fornitore
);
CREATE INDEX ix_dominio_fornitore_fornitore ON dominio_fornitore (fornitore_id);

-- Stesse regole di buyer.email: minuscola. Un'email censita come contatto di un fornitore E come
-- buyer di un cliente e' `ambiguo` per il resolver, non un errore di inserimento.
CREATE TABLE contatto_fornitore (
    contatto_id  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fornitore_id uuid NOT NULL REFERENCES fornitore,
    nome         varchar(120),
    email        varchar(200) NOT NULL CHECK (email = lower(email)),
    ruolo        varchar(80),
    lingua       char(2),
    note         text,
    UNIQUE (fornitore_id, email)
);
CREATE INDEX ix_contatto_fornitore_email ON contatto_fornitore (email);

-- ============================================================ 2. LE LAVORAZIONI (capacita' tecniche)

-- Un processo che l'azienda affida fuori. NON c'e' `materiale` (richiesta del 18/09): una materia
-- prima non e' un processo, e chi la vende ha tipo = materie_prime.
CREATE TABLE lavorazione (
    codice      varchar(30) PRIMARY KEY CHECK (codice = lower(codice)),
    descrizione varchar(120) NOT NULL
);
INSERT INTO lavorazione (codice, descrizione) VALUES
    ('tornitura',            'Tornitura'),
    ('fresatura',            'Fresatura'),
    ('taglio_laser',         'Taglio laser'),
    ('piega',                'Piegatura'),
    ('curvatura_tubi',       'Curvatura tubi'),
    ('saldatura',            'Saldatura'),
    ('zincatura',            'Zincatura'),
    ('cataforesi',           'Cataforesi'),
    ('verniciatura_polvere', 'Verniciatura a polvere'),
    ('lavaggio_zinco',       'Lavaggio zinco'),
    ('sabbiatura',           'Sabbiatura'),
    ('trattamento_termico',  'Trattamento termico'),
    ('attrezzature',         'Attrezzature');

-- Che cosa sa fare un fornitore. Resta SEPARATA dal tipo: il tipo dice che cos'e', questa che cosa fa.
CREATE TABLE fornitore_lavorazione (
    fornitore_id uuid        NOT NULL REFERENCES fornitore,
    lavorazione  varchar(30) NOT NULL REFERENCES lavorazione,
    PRIMARY KEY (fornitore_id, lavorazione)
);

-- «Il verniciatore X fa la cataforesi per il cliente Y» (richiesta del 18/09): lo stesso verniciatore
-- puo' stare su clienti diversi e su processi diversi. La chiave esterna COMPOSTA verso
-- fornitore_lavorazione garantisce che un cliente sia legato a un fornitore solo per una lavorazione
-- che quel fornitore fa davvero. Nessuna colonna di classificazione in piu'; le celle verdi del foglio
-- NON sono modellate finche' non e' chiaro che cosa vogliono dire.
CREATE TABLE cliente_fornitore_lavorazione (
    cliente_id   uuid        NOT NULL REFERENCES cliente,
    fornitore_id uuid        NOT NULL,
    lavorazione  varchar(30) NOT NULL,
    PRIMARY KEY (cliente_id, fornitore_id, lavorazione),
    FOREIGN KEY (fornitore_id, lavorazione) REFERENCES fornitore_lavorazione (fornitore_id, lavorazione)
);
CREATE INDEX ix_cliente_fornitore_lavorazione_fornitore ON cliente_fornitore_lavorazione (fornitore_id);

-- ============================================================ 3. LA CONTROPARTE COME FATTO SUL MESSAGGIO (D33)

CREATE TYPE tipo_controparte AS ENUM ('cliente', 'fornitore', 'interno', 'sconosciuto', 'ambiguo');
-- Da dove viene la risposta del resolver. Enum, non varchar: e' una classificazione.
--   contatto   email esatta in contatto_fornitore o in buyer
--   dominio    dominio in dominio_fornitore o in dominio_cliente
--   casella    il mittente e' una casella censita o un dominio nostro (→ interno)
--   manuale    deciso da un operatore (Censisci, Aggancia)
CREATE TYPE via_controparte AS ENUM ('contatto', 'dominio', 'casella', 'manuale');

-- Due colonne con chiave esterna vera invece di un `controparte_id` polimorfo (il piano lo aveva
-- come uuid libero): un id che non punta a niente non puo' esistere. Il CHECK lega le colonne al
-- tipo; `ambiguo`, `interno` e `sconosciuto` non hanno id.
ALTER TABLE messaggio ADD COLUMN controparte_tipo         tipo_controparte NOT NULL DEFAULT 'sconosciuto';
ALTER TABLE messaggio ADD COLUMN controparte_cliente_id   uuid REFERENCES cliente;
ALTER TABLE messaggio ADD COLUMN controparte_fornitore_id uuid REFERENCES fornitore;
ALTER TABLE messaggio ADD COLUMN controparte_via          via_controparte;
ALTER TABLE messaggio ADD COLUMN controparte_il           timestamptz;   -- quando e' stata risolta (audit)
ALTER TABLE messaggio ADD CONSTRAINT ck_messaggio_controparte CHECK (
       (controparte_tipo = 'cliente'   AND controparte_cliente_id IS NOT NULL AND controparte_fornitore_id IS NULL)
    OR (controparte_tipo = 'fornitore' AND controparte_fornitore_id IS NOT NULL AND controparte_cliente_id IS NULL)
    OR (controparte_tipo IN ('interno','sconosciuto','ambiguo') AND controparte_cliente_id IS NULL AND controparte_fornitore_id IS NULL)
);
CREATE INDEX ix_messaggio_controparte_fornitore ON messaggio (controparte_fornitore_id) WHERE controparte_fornitore_id IS NOT NULL;
CREATE INDEX ix_messaggio_controparte_tipo      ON messaggio (controparte_tipo);

-- I messaggi gia' in database restano `sconosciuto`: il ricalcolo lo fa il resolver in Go al primo
-- avvio (CP8), con il conteggio per tipo prima/dopo nel registro. La migrazione non indovina.

-- ============================================================ 4. CONVENZIONI DI CODICE → LAVORAZIONE (D39)
--
-- Nei cartigli di alcuni clienti la lavorazione superficiale e' scritta nel codice del pezzo, di
-- solito come suffisso. Da quel suffisso si ricava la lavorazione richiesta e, con
-- cliente_fornitore_lavorazione, i fornitori qualificati per QUEL cliente su QUELLA lavorazione.
--
-- Stessa filosofia di cliente.regole (D17), ma in tabelle e non in jsonb: qui ogni convenzione punta
-- a una riga di `lavorazione` con chiave esterna, e un jsonb non sa tenere una chiave esterna.
--   * ogni convenzione porta un ESEMPIO che deve corrispondere e un CONTROESEMPIO facoltativo che
--     NON deve corrispondere: e' l'unico modo per accorgersi di una convenzione troppo larga
--     (il suffisso «V» prende anche «...ZV») il giorno in cui la si scrive, non mesi dopo;
--   * la regex si compila e si prova in Go (`domain.Convenzioni`): il dialetto di PostgreSQL non e'
--     RE2, e un CHECK qui direbbe si' a espressioni che il Go rifiuta, o viceversa;
--   * `Valida` rifiuta in scrittura; `Leggi` segna ✗ e non usa (D17: due porte, non una);
--   * nessun suffisso reale e' seminato: le convenzioni si scrivono dall'anagrafica del cliente.
--
-- Il risultato e' un INSIEME di lavorazioni, non una sola: un pezzo puo' volere zincatura E
-- verniciatura, e una convenzione puo' dire piu' lavorazioni (tabella figlia). Corrispondono tutte
-- le convenzioni attive che corrispondono; nessuna precedenza nascosta. Se due convenzioni si
-- sovrappongono e' il controesempio a dirlo in scrittura.
--
-- In 7A la risoluzione e' una funzione pura in Go piu' una query; la lavorazione richiesta si
-- PERSISTE sul componente solo con Articoli/Distinta, quando un codice diventa un articolo.

-- Come e' scritta la convenzione:
--   suffisso   testo letterale confrontato con la fine del codice, senza distinguere le maiuscole
--              (il Go la compila come (?i)<testo>$ con QuoteMeta: niente da sapere sulle regex)
--   regex      espressione RE2 completa, per tutto il resto (posizione in mezzo, separatore obbligatorio, ...)
CREATE TYPE modo_convenzione AS ENUM ('suffisso', 'regex');

CREATE TABLE convenzione_codice (
    convenzione_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cliente_id     uuid NOT NULL REFERENCES cliente,
    modo           modo_convenzione NOT NULL DEFAULT 'suffisso',
    espressione    varchar(200) NOT NULL CHECK (btrim(espressione) <> ''),
    esempio        varchar(60)  NOT NULL CHECK (btrim(esempio) <> ''),    -- un codice pezzo che DEVE corrispondere
    controesempio  varchar(60)  CHECK (controesempio IS NULL OR (btrim(controesempio) <> '' AND controesempio <> esempio)),
    descrizione    varchar(120) NOT NULL,                                  -- «suffisso Z = zincato», quello che legge l'operatore
    attiva         boolean NOT NULL DEFAULT true,
    creato_il      timestamptz NOT NULL DEFAULT now(),
    -- un suffisso e' corto e senza spazi: se serve altro e' una regex, e va dichiarata tale
    CONSTRAINT ck_convenzione_suffisso CHECK (modo <> 'suffisso' OR (length(espressione) <= 12 AND espressione !~ '\s')),
    UNIQUE (cliente_id, modo, espressione)
);
CREATE INDEX ix_convenzione_codice_cliente ON convenzione_codice (cliente_id) WHERE attiva;

-- Che cosa vuol dire la convenzione: una o piu' lavorazioni. Una convenzione senza righe qui non
-- dice niente: `Valida` la rifiuta e `Leggi` la segna ✗ (il database non sa contare le figlie).
CREATE TABLE convenzione_codice_lavorazione (
    convenzione_id uuid        NOT NULL REFERENCES convenzione_codice ON DELETE CASCADE,
    lavorazione    varchar(30) NOT NULL REFERENCES lavorazione,
    PRIMARY KEY (convenzione_id, lavorazione)
);

COMMENT ON TABLE convenzione_codice IS
'Come un cliente scrive la lavorazione nel codice del pezzo (D39). Ogni riga porta un esempio che deve
corrispondere e un controesempio che non deve; la regex si verifica in Go, in scrittura (rifiuto) e in
lettura (✗, non usata). Il risultato e'' l''insieme delle lavorazioni di tutte le convenzioni attive che
corrispondono; poi cliente_fornitore_lavorazione dice chi e'' qualificato. Nessun suffisso reale nel seme.';

-- ============================================================ 5. v_inbox: la controparte si vede
--
-- CREATE OR REPLACE VIEW puo' solo AGGIUNGERE colonne in coda: la definizione e' quella della 0004,
-- tale e quale, piu' due colonne. `controparte` e' il nome che l'Inbox mostra accanto al mittente:
-- la cartella NAS del cliente (come `cliente`, che resta per compatibilita') o la ragione sociale
-- del fornitore.
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
       END AS controparte
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

INSERT INTO schema_versione (versione) VALUES (14);
