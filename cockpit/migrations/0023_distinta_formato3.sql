-- 0023_distinta_formato3.sql — V1 della Distinta unica (giro 5, A1d; direttiva 23).
--   Formato 3 della bozza e della conferma; registro d'importazione del Motore A; nodi non inclusi (LS-02);
--   identita' certificata dei componenti; gesto conferma_bom_v3; ciclo dei file della Sorgente (stato
--   dell'occorrenza nella RFQ, registro, rimozione dallo staging, deposito «Altri file»).
--
-- Perche' una migrazione nuova. La ratifica 05 §1 ammette un'altra migrazione P4 solo per un'impossibilita' tecnica
--   provata. La 0022 ha sette punti che vogliono un DDL: sei vincoli che vietano il formato 3 (ck_distinta_bozza_formato,
--   ck_conferma_formato, ck_conferma_bozza, ck_conferma_snapshot, ck_conferma_chiave, trigger del contenuto) e il
--   CHECK chiuso dei gesti (ck_esito_intento_gesto). Il trigger del ciclo della bozza resta: impone la conversione
--   esplicita (una bozza formato 2 aperta non diventa formato 3 sul posto). La 0022 e' distribuita: non si riscrive.
--   La sua intestazione («non ci sara' un'altra migrazione P4») resta com'e'.
--
-- Regole. Nessuna riga esistente cambia; nessun riempimento. I vincoli cambiati si tolgono e si rimettono con lo
--   stesso nome; il ramo formato 2 di ogni vincolo dice esattamente quello di prima. Le quattro funzioni della 0022
--   che cambiano (distinta_bozza_versione_nasce, conferma_strutturale_nasce, conferma_strutturale_contenuto,
--   conferma_strutturale_completa) hanno il corpo della 0022 byte per byte, salvo le righe marcate «(0023)», una
--   per una: la prova L1 T-11 confronta i corpi. Ritorno manuale: scripts/0023_indietro.sql (mai in migrations/).
--
-- Deroga dichiarata (come la 0022 per v_fascicolo): v_fascicolo, v_step_prodotto, v_thread_da_riesaminare, v_cruscotto
--   e v_codici_candidati_thread si ridefiniscono con le stesse colonne, nello stesso ordine e con lo stesso significato
--   per ogni file attivo; cambia solo che una proposta (o una riga strutturale) di un'occorrenza in «Altri file» o
--   scartata non conta piu'. Il testo di ogni vista e' quello di oggi, con il solo predicato file_rfq_attivo aggiunto
--   sulle righe marcate (la prova T-11 lo controlla). Senza righe in file_rfq le viste danno le righe di prima.
--
-- Codici: P4X08 registro d'importazione; P4X09 ciclo dei file; P4X01-P4X04 come nella 0022.
-- Si applica a server e worker fermi. Solo sui database di prova finche' l'utente non da' il suo OK: un database
-- alla 23 ferma ogni binario che conosce solo la 22.
--
-- ORDINE: 1. testata della bozza
--         2. registro d'importazione (distinta_importazione)
--         3. versioni della bozza
--         4. conferma strutturale
--         5. componenti certificati: l'identita' interrogabile
--         6. nodi non inclusi (LS-02)
--         7. il controllo al COMMIT della conferma
--         8. esito degli intenti
--         9. ciclo dei file della Sorgente (file_rfq, registro, rimozione, conservazione, file_rfq_attivo)
--        10. le cinque viste
--        11. commenti
--        12. variante e versione
SET LOCAL lock_timeout = '30s';

-- ============================================================ 1. TESTATA DELLA BOZZA
ALTER TABLE distinta_bozza DROP CONSTRAINT ck_distinta_bozza_formato;
ALTER TABLE distinta_bozza ADD CONSTRAINT ck_distinta_bozza_formato CHECK (formato IN (2, 3));
-- DEFAULT 2 invariato (righe e INSERT di prima). Il Go del formato 3 passa il formato (InsertDistintaBozza).
-- Il trigger del ciclo (distinta_bozza_ciclo) resta: il formato di una bozza non cambia; si converte con una bozza nuova.

-- ============================================================ 2. REGISTRO D'IMPORTAZIONE
-- La proposta del Motore A per un prodotto e un gruppo, com'era quando l'ingegnere l'ha importata: immutabile,
-- citata per impronta dalle versioni della bozza e dalle conferme (FK). Non si cancella in V1.
CREATE TABLE distinta_importazione (
    importazione_id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id             uuid NOT NULL REFERENCES thread_offerta,
    prodotto_id           uuid NOT NULL,
    impronta              char(64) NOT NULL,          -- sha256 del contenuto canonico intero (la chiave)
    impronta_contenuto    char(64) NOT NULL,          -- struttura e identita' (elenco chiuso): si confronta
    impronta_evidenze     char(64) NOT NULL,          -- il resto della proposta: solo informativa
    impronta_regole       char(64) NOT NULL,          -- sha256 di contenuto.regole
    formato_registro      smallint NOT NULL,
    versione_importazione text NOT NULL,              -- importazione-1
    gruppo                text NOT NULL,
    fonte_allegato_id     uuid REFERENCES allegato,
    fonte_sha256          char(64),
    fonte_radice          text,                       -- il rif della radice del gruppo (contenuto.gruppo.radice): con fonte_allegato_id e
                                                      -- fonte_sha256 ritrova il gruppo corrente della sorgente senza leggere il contenuto
    allegati_citati       uuid[] NOT NULL DEFAULT '{}',
    contenuto             jsonb NOT NULL,
    creata_da             uuid NOT NULL REFERENCES utente,
    creata_il             timestamptz NOT NULL DEFAULT now(),
    tx                    xid8 NOT NULL DEFAULT pg_current_xact_id(),
    CONSTRAINT fk_dimp_prodotto FOREIGN KEY (thread_id, prodotto_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ux_dimp_thread_id UNIQUE (thread_id, importazione_id),        -- bersaglio della FK della conferma
    CONSTRAINT ux_dimp_impronta UNIQUE (thread_id, prodotto_id, impronta),   -- stessa proposta = stesso registro
    CONSTRAINT ck_dimp_impronte CHECK (impronta ~ '^[0-9a-f]{64}$' AND impronta_contenuto ~ '^[0-9a-f]{64}$'
        AND impronta_evidenze ~ '^[0-9a-f]{64}$' AND impronta_regole ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_dimp_formato CHECK (formato_registro = 1 AND versione_importazione ~ '^importazione-[1-9][0-9]*$'),
    CONSTRAINT ck_dimp_gruppo CHECK (btrim(gruppo) <> '' AND char_length(gruppo) <= 300 AND gruppo <> 'senza'),
    CONSTRAINT ck_dimp_fonte CHECK ((fonte_allegato_id IS NULL) = (fonte_sha256 IS NULL)
        AND (fonte_sha256 IS NULL OR fonte_sha256 ~ '^[0-9a-f]{64}$')
        AND (fonte_allegato_id IS NULL) = (fonte_radice IS NULL)
        AND (fonte_radice IS NULL OR (fonte_radice ~ '^nodo:[0-9a-f]{64}:.+$' AND char_length(fonte_radice) <= 400))),
    CONSTRAINT ck_dimp_citati CHECK (cardinality(allegati_citati) <= 10000
        AND (fonte_allegato_id IS NULL OR fonte_allegato_id = ANY (allegati_citati))),
    CONSTRAINT ck_dimp_contenuto CHECK (jsonb_typeof(contenuto) = 'object'
        AND COALESCE(contenuto ->> 'formato_registro' = formato_registro::text, false)
        AND COALESCE(contenuto ->> 'versione_importazione' = versione_importazione, false)
        AND COALESCE(contenuto ->> 'thread_id' = thread_id::text, false)
        AND COALESCE(contenuto ->> 'prodotto_id' = prodotto_id::text, false)
        AND COALESCE(contenuto -> 'gruppo' ->> 'chiave' = gruppo, false)
        AND (contenuto -> 'gruppo' ->> 'radice') IS NOT DISTINCT FROM fonte_radice
        AND COALESCE(contenuto ->> 'impronta_contenuto' = impronta_contenuto::text, false)
        AND COALESCE(contenuto ->> 'impronta_evidenze' = impronta_evidenze::text, false)
        AND COALESCE(contenuto ->> 'impronta_regole' = impronta_regole::text, false)
        AND pg_column_size(contenuto) <= 16777216)
);
CREATE INDEX ix_dimp_prodotto ON distinta_importazione (prodotto_id, creata_il);
CREATE INDEX ix_dimp_fonte ON distinta_importazione (fonte_allegato_id) WHERE fonte_allegato_id IS NOT NULL;
CREATE INDEX ix_dimp_citati ON distinta_importazione USING gin (allegati_citati);
CREATE TRIGGER trg_dimp_ora BEFORE INSERT ON distinta_importazione
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('creata_il');

-- La fonte e ogni allegato citato sono occorrenze della stessa RFQ e attive; la fonte ha lo sha256 dichiarato.
-- (L'attivita' la ricontrolla anche R2 sotto BloccaThread, con un 409 leggibile. Qui e' la difesa.) file_rfq_attivo
-- e' definita nella sezione 9: il corpo plpgsql la risolve all'esecuzione, non alla creazione.
CREATE FUNCTION distinta_importazione_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    fuori uuid;
BEGIN
    IF NEW.fonte_allegato_id IS NOT NULL AND NOT EXISTS (
           SELECT 1 FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
            WHERE a.allegato_id = NEW.fonte_allegato_id AND a.sha256 = NEW.fonte_sha256
              AND m.thread_id = NEW.thread_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X08',
            MESSAGE = format('distinta_importazione: la fonte %s non e'' un allegato della RFQ %s con sha256 %s',
                             NEW.fonte_allegato_id, NEW.thread_id, NEW.fonte_sha256);
    END IF;
    SELECT c.id INTO fuori
      FROM unnest(NEW.allegati_citati) AS c(id)
     WHERE NOT EXISTS (SELECT 1 FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
                        WHERE a.allegato_id = c.id AND m.thread_id = NEW.thread_id)
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X08',
            MESSAGE = format('distinta_importazione: l''allegato %s citato non e'' della RFQ %s', fuori, NEW.thread_id);
    END IF;
    SELECT c.id INTO fuori FROM unnest(NEW.allegati_citati) AS c(id)
     WHERE NOT file_rfq_attivo(NEW.thread_id, c.id) LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X08',
            MESSAGE = format('distinta_importazione: l''allegato %s citato non e'' attivo nella RFQ %s (in Altri file o scartato)', fuori, NEW.thread_id);
    END IF;
    NEW.tx := pg_current_xact_id();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_dimp_nasce BEFORE INSERT ON distinta_importazione
    FOR EACH ROW EXECUTE FUNCTION distinta_importazione_nasce();
CREATE TRIGGER trg_dimp_solo_aggiunte BEFORE UPDATE OR DELETE ON distinta_importazione
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'un''importazione registrata non si modifica e non si cancella');
CREATE TRIGGER trg_dimp_non_si_svuota BEFORE TRUNCATE ON distinta_importazione
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le importazioni non si svuotano');

-- ============================================================ 3. VERSIONI DELLA BOZZA
-- importazione_id: il registro di sorgente.importazione (solo formato 3). chiave_salvataggio e impronta_richiesta: la
-- chiave del salvataggio R2 e l'impronta della richiesta, per riconoscere un reinvio senza scrivere un doppione
-- (obbligatorie nel formato 3).
ALTER TABLE distinta_bozza_versione
    ADD COLUMN importazione_id    uuid,
    ADD COLUMN chiave_salvataggio uuid,
    ADD COLUMN impronta_richiesta char(64);
ALTER TABLE distinta_bozza_versione
    ADD CONSTRAINT fk_dbv_importazione FOREIGN KEY (importazione_id) REFERENCES distinta_importazione,
    -- nome scelto perche' venga dopo ck_dbv_snapshot nell'ordine (alfabetico) dei controlli: il caso della 0022
    -- con snapshot = '[]' da' ancora ck_dbv_snapshot
    ADD CONSTRAINT ck_dbv_snapshot_formato CHECK (COALESCE(snapshot ->> 'formato' IN ('2', '3'), false)
        AND (importazione_id IS NULL OR snapshot ->> 'formato' = '3')
        -- formato 3: nodi, archi e decisioni sono array (anche assenti: rifiutati), e ogni nodo e' un oggetto con la sua
        -- identita' oggetto (niente nodi di forma formato 2 sotto un «3»). In modo lax il filtro ? (...) scarta un livello
        -- di array: il tipo del nodo e quello dell'identita' si guardano con .type(), che non lo scarta
        AND (snapshot ->> 'formato' <> '3'
             OR (COALESCE(jsonb_typeof(snapshot -> 'nodi') = 'array', false)
                 AND COALESCE(jsonb_typeof(snapshot -> 'archi') = 'array', false)
                 AND COALESCE(jsonb_typeof(snapshot -> 'decisioni') = 'array', false)
                 AND NOT jsonb_path_exists(snapshot, '$.nodi[*] ? (!exists(@.identita))')
                 AND NOT jsonb_path_exists(snapshot, '$.nodi[*].type() ? (@ != "object")')
                 AND NOT jsonb_path_exists(snapshot, '$.nodi[*].identita.type() ? (@ != "object")')))),
    ADD CONSTRAINT ck_dbv_chiave_salvataggio CHECK ((chiave_salvataggio IS NULL) = (impronta_richiesta IS NULL)
        AND (impronta_richiesta IS NULL OR impronta_richiesta ~ '^[0-9a-f]{64}$')
        AND (snapshot ->> 'formato' <> '3' OR chiave_salvataggio IS NOT NULL));
CREATE INDEX ix_dbv_importazione ON distinta_bozza_versione (importazione_id) WHERE importazione_id IS NOT NULL;
CREATE UNIQUE INDEX ux_dbv_chiave_salvataggio ON distinta_bozza_versione (chiave_salvataggio)
    WHERE chiave_salvataggio IS NOT NULL;

-- Corpo della 0022 con tre aggiunte: lo snapshot ha il formato della testata; il registro della sorgente e' della
-- stessa RFQ e dello stesso prodotto, con l'impronta, il gruppo e la fonte dello snapshot; i registri citati dalle
-- decisioni esistono e sono dello stesso prodotto.
CREATE OR REPLACE FUNCTION distinta_bozza_versione_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    numero_base int;
    t record;                                                                                         -- (0023)
    i record;                                                                                         -- (0023)
    d text;                                                                                           -- (0023)
BEGIN
    IF NEW.base_conferma_id IS NOT NULL THEN
        SELECT x.numero INTO numero_base FROM conferma_strutturale x WHERE x.conferma_id = NEW.base_conferma_id;
        IF FOUND AND numero_base <> NEW.base_numero THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X04',
                MESSAGE = format('distinta_bozza %s, versione %s: base_numero %s, ma la conferma di base %s e'' la n. %s',
                                 NEW.bozza_id, NEW.versione, NEW.base_numero, NEW.base_conferma_id, numero_base);
        END IF;
    END IF;
    SELECT b.formato, b.thread_id, b.prodotto_id INTO t FROM distinta_bozza b WHERE b.bozza_id = NEW.bozza_id;  -- (0023)
    IF NOT FOUND THEN                                                                                 -- (0023)
        RETURN NEW;                                                       -- lo rifiuta la FK         -- (0023)
    END IF;                                                                                           -- (0023)
    -- lo snapshot ha il formato della testata (mai uno snapshot formato 3 sotto una testata formato 2, ne' il contrario)  -- (0023)
    IF (NEW.snapshot ->> 'formato') IS DISTINCT FROM t.formato::text THEN                             -- (0023)
        RAISE EXCEPTION USING ERRCODE = 'P4X04',                                                      -- (0023)
            MESSAGE = format('distinta_bozza %s (formato %s): la versione %s porta uno snapshot formato %s',  -- (0023)
                             NEW.bozza_id, t.formato, NEW.versione, COALESCE(NEW.snapshot ->> 'formato', '-'));  -- (0023)
    END IF;                                                                                           -- (0023)
    -- il registro della sorgente e' della stessa RFQ e dello stesso prodotto, con l'impronta, il gruppo e la fonte dello snapshot  -- (0023)
    IF NEW.importazione_id IS NOT NULL THEN                                                           -- (0023)
        SELECT x.thread_id, x.prodotto_id, x.impronta, x.gruppo, x.fonte_allegato_id INTO i           -- (0023)
          FROM distinta_importazione x WHERE x.importazione_id = NEW.importazione_id;                 -- (0023)
        IF FOUND AND (i.thread_id <> t.thread_id OR i.prodotto_id <> t.prodotto_id                    -- (0023)
                      OR (NEW.snapshot -> 'sorgente' ->> 'importazione') IS DISTINCT FROM i.impronta::text  -- (0023)
                      OR (NEW.snapshot -> 'sorgente' ->> 'gruppo') IS DISTINCT FROM i.gruppo          -- (0023)
                      OR (NEW.snapshot -> 'sorgente' ->> 'allegato_id') IS DISTINCT FROM i.fonte_allegato_id::text) THEN  -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X04',                                                  -- (0023)
                MESSAGE = format('distinta_bozza %s, versione %s: il registro %s non e'' quello dello snapshot (impronta, gruppo, fonte) o non e'' del prodotto',  -- (0023)
                                 NEW.bozza_id, NEW.versione, NEW.importazione_id);                    -- (0023)
        END IF;                                                                                       -- (0023)
    ELSIF NEW.snapshot -> 'sorgente' ->> 'importazione' IS NOT NULL THEN                              -- (0023)
        RAISE EXCEPTION USING ERRCODE = 'P4X04',                                                      -- (0023)
            MESSAGE = format('distinta_bozza %s, versione %s: lo snapshot cita un registro che la versione non registra',  -- (0023)
                             NEW.bozza_id, NEW.versione);                                             -- (0023)
    END IF;                                                                                           -- (0023)
    -- i registri citati dalle decisioni (identita, non_includere) esistono e sono dello stesso prodotto  -- (0023)
    SELECT x.imp INTO d                                                                               -- (0023)
      FROM (SELECT DISTINCT e ->> 'importazione' AS imp                                               -- (0023)
              FROM jsonb_array_elements(CASE WHEN jsonb_typeof(NEW.snapshot -> 'decisioni') = 'array' -- (0023)
                                             THEN NEW.snapshot -> 'decisioni' ELSE '[]'::jsonb END) e -- (0023)
             WHERE e ? 'importazione') x                                                              -- (0023)
     WHERE NOT EXISTS (SELECT 1 FROM distinta_importazione r                                          -- (0023)
                        WHERE r.thread_id = t.thread_id AND r.prodotto_id = t.prodotto_id AND r.impronta = x.imp)  -- (0023)
     LIMIT 1;                                                                                         -- (0023)
    IF FOUND THEN                                                                                     -- (0023)
        RAISE EXCEPTION USING ERRCODE = 'P4X04',                                                      -- (0023)
            MESSAGE = format('distinta_bozza %s, versione %s: una decisione cita il registro %s, che non e'' del prodotto',  -- (0023)
                             NEW.bozza_id, NEW.versione, COALESCE(d, '-'));                           -- (0023)
    END IF;                                                                                           -- (0023)
    NEW.tx := pg_current_xact_id();
    NEW.salvata_il := now();
    RETURN NEW;
END $$;

-- ============================================================ 4. CONFERMA STRUTTURALE
ALTER TABLE conferma_strutturale
    DROP CONSTRAINT ck_conferma_formato,
    DROP CONSTRAINT ck_conferma_bozza,
    DROP CONSTRAINT ck_conferma_snapshot,
    DROP CONSTRAINT ck_conferma_chiave;
ALTER TABLE conferma_strutturale
    ADD CONSTRAINT ck_conferma_formato CHECK (formato IN (1, 2, 3) AND senza_riepilogo_v2 = (formato = 1)),
    -- formato 3: la conferma chiude sempre una versione salvata con R2
    ADD CONSTRAINT ck_conferma_bozza CHECK ((bozza_id IS NULL) = (bozza_versione IS NULL)
        AND (formato IN (2, 3) OR bozza_id IS NULL)
        AND (formato <> 3 OR bozza_id IS NOT NULL)),
    -- formato 2: snapshot «2» dello stesso prodotto; il CHECK non guarda dentro (che una conferma formato 2 non porti dati
    -- formato 3 lo garantisce il Go, prova T-12b). Formato 3: snapshot «3» con nodi, archi e decisioni in array e ogni nodo
    -- un oggetto con la sua identita' oggetto, come ck_dbv_snapshot_formato (niente nodi di forma formato 2 sotto un «3»)
    ADD CONSTRAINT ck_conferma_snapshot CHECK (impronta_snapshot ~ '^[0-9a-f]{64}$'
        AND (formato = 1 OR (COALESCE(snapshot ->> 'formato' = formato::text, false)
                             AND COALESCE(snapshot ->> 'prodotto_id' = prodotto_id::text, false)))
        AND (formato <> 3 OR (COALESCE(jsonb_typeof(snapshot -> 'nodi') = 'array', false)
                              AND COALESCE(jsonb_typeof(snapshot -> 'archi') = 'array', false)
                              AND COALESCE(jsonb_typeof(snapshot -> 'decisioni') = 'array', false)
                              AND NOT jsonb_path_exists(snapshot, '$.nodi[*] ? (!exists(@.identita))')
                              AND NOT jsonb_path_exists(snapshot, '$.nodi[*].type() ? (@ != "object")')
                              AND NOT jsonb_path_exists(snapshot, '$.nodi[*].identita.type() ? (@ != "object")')))),
    ADD CONSTRAINT ck_conferma_chiave CHECK ((formato IN (2, 3)) = (chiave_idempotenza IS NOT NULL));
-- ck_conferma_firme, ck_conferma_base, ck_conferma_mono_pezzo_snapshot: invariati, valgono anche per il 3.
-- jsonb_path_exists senza _tz e' IMMUTABLE, quindi ammessa in un CHECK.
ALTER TABLE conferma_strutturale ADD COLUMN importazione_id uuid, ADD COLUMN importazioni_citate uuid[];
ALTER TABLE conferma_strutturale
    ADD CONSTRAINT fk_conferma_importazione FOREIGN KEY (thread_id, importazione_id)
        REFERENCES distinta_importazione (thread_id, importazione_id),
    ADD CONSTRAINT ck_conferma_importazione CHECK (importazione_id IS NULL OR formato = 3),
    -- tutti i registri citati dallo snapshot (sorgente e decisioni): solo nel formato 3, e sempre nel formato 3
    ADD CONSTRAINT ck_conferma_importazioni_citate CHECK ((formato = 3) = (importazioni_citate IS NOT NULL)
        AND (importazioni_citate IS NULL OR cardinality(importazioni_citate) <= 1000)
        AND (importazione_id IS NULL OR importazione_id = ANY (importazioni_citate)));
CREATE INDEX ix_conferma_importazione ON conferma_strutturale (importazione_id) WHERE importazione_id IS NOT NULL;
CREATE INDEX ix_conferma_importazioni_citate ON conferma_strutturale USING gin (importazioni_citate)
    WHERE importazioni_citate IS NOT NULL;

-- Un array non puo' avere una FK: l'esistenza e l'appartenenza dei registri in importazioni_citate le controlla il
-- trigger di nascita qui sotto, e i registri non si cancellano (P4X02). Corpo della 0022 con tre aggiunte: la bozza
-- dello stesso formato e con lo stesso registro; il registro dello stesso prodotto con l'impronta, il gruppo e la
-- fonte dello snapshot; importazioni_citate uguale all'insieme dei registri citati dallo snapshot.
CREATE OR REPLACE FUNCTION conferma_strutturale_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    tipo_prodotto tipo_componente;
    ultimo_id     uuid;
    ultimo_numero int;
    b             record;
    c_struttura   bigint;
    c_dati        bigint;
    g             record;                                                                             -- (0023)
    citate        uuid[];                                                                             -- (0023)
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
        SELECT x.stato, x.versione, x.formato, v.impronta, v.importazione_id INTO b                   -- (0023: formato, importazione_id)
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
        -- la bozza ha il formato della conferma, e la versione che chiude cita lo stesso registro    -- (0023)
        IF FOUND AND (b.formato <> NEW.formato OR b.importazione_id IS DISTINCT FROM NEW.importazione_id) THEN  -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma strutturale formato %s: la bozza %s e'' formato %s, o la sua versione cita un altro registro',  -- (0023)
                                 NEW.formato, NEW.bozza_id, b.formato);                               -- (0023)
        END IF;                                                                                       -- (0023)
    END IF;
    -- il registro citato e' dello stesso prodotto, e lo snapshot ne porta impronta, gruppo e fonte   -- (0023)
    IF NEW.importazione_id IS NOT NULL THEN                                                           -- (0023)
        SELECT x.prodotto_id, x.impronta, x.gruppo, x.fonte_allegato_id INTO g                        -- (0023)
          FROM distinta_importazione x WHERE x.importazione_id = NEW.importazione_id;                 -- (0023)
        IF FOUND AND (g.prodotto_id <> NEW.prodotto_id                                                -- (0023)
                      OR (NEW.snapshot -> 'sorgente' ->> 'importazione') IS DISTINCT FROM g.impronta::text  -- (0023)
                      OR g.gruppo IS DISTINCT FROM NEW.fonte_gruppo                                   -- (0023)
                      OR g.fonte_allegato_id IS DISTINCT FROM NEW.fonte_allegato_id) THEN             -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma strutturale di %s: il registro %s non e'' quello dello snapshot (impronta, gruppo, fonte) o non e'' del prodotto',  -- (0023)
                                 NEW.prodotto_id, NEW.importazione_id);                               -- (0023)
        END IF;                                                                                       -- (0023)
    ELSIF NEW.formato = 3 AND (NEW.snapshot -> 'sorgente' ->> 'importazione') IS NOT NULL THEN        -- (0023)
        RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                      -- (0023)
            MESSAGE = format('conferma strutturale di %s: lo snapshot cita un registro che la conferma non registra', NEW.prodotto_id);  -- (0023)
    END IF;                                                                                           -- (0023)
    -- importazioni_citate = i registri dello snapshot (sorgente e decisioni), tutti del prodotto, senza ripetizioni  -- (0023)
    IF NEW.formato = 3 THEN                                                                           -- (0023)
        SELECT array_agg(DISTINCT r.importazione_id ORDER BY r.importazione_id) INTO citate           -- (0023)
          FROM distinta_importazione r                                                                -- (0023)
         WHERE r.thread_id = NEW.thread_id AND r.prodotto_id = NEW.prodotto_id                        -- (0023)
           AND r.impronta IN (SELECT NEW.snapshot -> 'sorgente' ->> 'importazione'                    -- (0023)
                              UNION                                                                   -- (0023)
                              SELECT e ->> 'importazione'                                             -- (0023)
                                FROM jsonb_array_elements(CASE WHEN jsonb_typeof(NEW.snapshot -> 'decisioni') = 'array'  -- (0023)
                                                               THEN NEW.snapshot -> 'decisioni' ELSE '[]'::jsonb END) e  -- (0023)
                               WHERE e ? 'importazione');                                             -- (0023)
        IF (SELECT count(DISTINCT x) FROM (SELECT NEW.snapshot -> 'sorgente' ->> 'importazione' AS x  -- (0023)
                                           UNION                                                      -- (0023)
                                           SELECT e ->> 'importazione'                                -- (0023)
                                             FROM jsonb_array_elements(CASE WHEN jsonb_typeof(NEW.snapshot -> 'decisioni') = 'array'  -- (0023)
                                                                            THEN NEW.snapshot -> 'decisioni' ELSE '[]'::jsonb END) e  -- (0023)
                                            WHERE e ? 'importazione') s WHERE x IS NOT NULL)          -- (0023)
               <> COALESCE(cardinality(citate), 0)                                                    -- (0023)
           OR ARRAY(SELECT unnest(NEW.importazioni_citate) ORDER BY 1) IS DISTINCT FROM COALESCE(citate, '{}'::uuid[]) THEN  -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma strutturale di %s: importazioni_citate non e'' l''insieme dei registri citati dallo snapshot, o un registro non e'' del prodotto',  -- (0023)
                                 NEW.prodotto_id);                                                    -- (0023)
        END IF;                                                                                       -- (0023)
    END IF;                                                                                           -- (0023)
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

-- ============================================================ 5. COMPONENTI CERTIFICATI
ALTER TABLE conferma_strutturale_componente
    ADD COLUMN emittente text, ADD COLUMN emittente_presunto boolean, ADD COLUMN namespace text;
-- La forma dell'emittente e' quella della regola della grammatica: cliente:<uuid>, oppure partner: seguito da un id
-- minuscolo senza spazi, «:» o «/» (internal/core/registro/regole/grammatica/partner.go). Il tetto di 200 caratteri
-- dell'id e quello di 500 del namespace sono solo di sicurezza: la grammatica non ne ha, e la stessa regola la
-- applica il Go in R2, R4 e R5 (EmittenteAmmissibile), cosi' un valore oltre il tetto e' un 422 leggibile e mai un
-- 23514 al COMMIT. La prova T-14 tiene le due regole uguali. «Non vuoto» e' quello di strings.TrimSpace della grammatica
-- e del Go: senza i bianchi Unicode (White_Space: tab, a capo, NBSP, gli spazi tipografici), non solo lo spazio ASCII.
-- Resta una differenza, dichiarata: lower() del database conosce le maiuscole non ASCII solo con una LC_CTYPE che le
-- conosce (resoconto della corsia persistenza di V1, P-17).
ALTER TABLE conferma_strutturale_componente ADD CONSTRAINT ck_csc_identita CHECK (
    (emittente IS NULL
     OR emittente ~ '^cliente:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
     OR (left(emittente, 8) = 'partner:'
         AND char_length(emittente) BETWEEN 9 AND 208
         AND btrim(substr(emittente, 9), E' \t\n\r\f\u000b\u0085               　') <> ''
         AND strpos(substr(emittente, 9), ' ') = 0 AND strpos(substr(emittente, 9), ':') = 0
         AND strpos(substr(emittente, 9), '/') = 0
         AND substr(emittente, 9) = lower(substr(emittente, 9))))
    AND (namespace IS NULL
         OR (btrim(namespace, E' \t\n\r\f\u000b\u0085               　') <> ''
             AND char_length(namespace) <= 500))
    AND (namespace IS NULL OR emittente IS NOT NULL)
    AND ((emittente IS NULL) = (emittente_presunto IS NULL)));
CREATE INDEX ix_csc_emittente ON conferma_strutturale_componente (componente_id, conferma_id) WHERE emittente IS NOT NULL;

-- Corpo della 0022 con quattro cambi: chiave_bozza obbligatoria anche nel formato 3, l'origine degli archi anche nel
-- formato 3 (con i due messaggi che dicono il formato), emittente, presunzione e namespace solo nel formato 3.
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
        -- f.2 e f.3: ogni componente certificato e' un nodo della bozza, con la sua chiave (la mappa la usa)  -- (0023: «f.3»)
        IF h.formato IN (2, 3) AND NEW.chiave_bozza IS NULL THEN                                      -- (0023: era «= 2»)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s (formato %s): il componente %s non ha la chiave della bozza', NEW.conferma_id, h.formato, NEW.componente_id);  -- (0023: testo «(formato %s)»)
        END IF;
        -- emittente, presunzione e namespace esistono solo nel formato 3, e li' emittente e presunzione sono  -- (0023)
        -- obbligatori; il namespace c'e' se l'identita' decisa ne ha uno                             -- (0023)
        IF (h.formato = 3) <> (NEW.emittente IS NOT NULL) OR (h.formato = 3) <> (NEW.emittente_presunto IS NOT NULL)  -- (0023)
           OR (h.formato <> 3 AND NEW.namespace IS NOT NULL) THEN                                     -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma %s (formato %s): emittente, presunzione e namespace del componente %s esistono solo nel formato 3, e li'' emittente e presunzione sono obbligatori',  -- (0023)
                                 NEW.conferma_id, h.formato, NEW.componente_id);                      -- (0023)
        END IF;                                                                                       -- (0023)
    ELSE
        IF h.mono_pezzo THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s: mono_pezzo, nessun arco', NEW.conferma_id);
        END IF;
        -- f.2 e f.3: ogni arco dice da dove viene; origine_gerarchia (step | manuale | mista, E2.1) si calcola da qui.  -- (0023: «f.3»)
        -- Che sia vera (un arco step e' davvero nella fonte) lo ricalcola il server in R4/R5, non la pagina.
        -- Revisione di dominio della notte, RD-03.
        IF h.formato IN (2, 3) AND NEW.origine IS NULL THEN                                           -- (0023: era «= 2»)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('conferma %s (formato %s): l''arco %s → %s non dice da dove viene (origine)', NEW.conferma_id, h.formato, NEW.padre_id, NEW.figlio_id);  -- (0023: testo «(formato %s)»)
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

-- ============================================================ 6. NODI NON INCLUSI (LS-02)
CREATE TABLE conferma_strutturale_non_incluso (
    conferma_id   uuid NOT NULL,
    thread_id     uuid NOT NULL,
    sha256        char(64) NOT NULL,
    chiave        text NOT NULL,                      -- la chiave del PRODUCT nel file («#12»)
    allegato_id   uuid NOT NULL REFERENCES allegato,  -- l'occorrenza da cui e' stata presa la decisione
    gruppo        text NOT NULL,
    decisione_id  text NOT NULL,                      -- id della decisione non_includere nello snapshot
    azione_id     uuid NOT NULL,                      -- la selezione multipla
    motivo_codice text NOT NULL,
    motivo        text NOT NULL,
    segnale       jsonb,                              -- {codice, regola, versione_servizio}; NULL = nodo non segnalato
    righe_chiuse  uuid[] NOT NULL DEFAULT '{}',       -- componente_proposta.proposta_id chiuse da R5 (copie storiche, senza FK)
    archi_chiusi  jsonb NOT NULL DEFAULT '[]',        -- [{allegato_id, padre_chiave, figlio_chiave}] di relazione_proposta chiuse da R5
    PRIMARY KEY (conferma_id, sha256, chiave),
    CONSTRAINT fk_csni_conferma FOREIGN KEY (thread_id, conferma_id)
        REFERENCES conferma_strutturale (thread_id, conferma_id),
    CONSTRAINT ux_csni_decisione UNIQUE (conferma_id, decisione_id),
    CONSTRAINT ck_csni_sha CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_csni_chiave CHECK (btrim(chiave) <> '' AND char_length(chiave) <= 330),
    CONSTRAINT ck_csni_gruppo CHECK (btrim(gruppo) <> '' AND char_length(gruppo) <= 300),
    CONSTRAINT ck_csni_decisione CHECK (decisione_id ~ '^[A-Za-z0-9._:-]{1,64}$'),
    CONSTRAINT ck_csni_motivo CHECK (motivo_codice IN ('elemento_cad', 'duplicato_cad', 'non_articolo', 'altro')
        AND btrim(motivo) <> '' AND char_length(motivo) <= 1000),
    CONSTRAINT ck_csni_json CHECK ((segnale IS NULL OR (jsonb_typeof(segnale) = 'object' AND pg_column_size(segnale) <= 8192))
        AND jsonb_typeof(archi_chiusi) = 'array' AND pg_column_size(archi_chiusi) <= 1048576)
);
CREATE INDEX ix_csni_rif ON conferma_strutturale_non_incluso (thread_id, sha256, chiave);

-- Solo nella transazione della conferma, prima del controllo finale, per una conferma formato 3; l'occorrenza e' della
-- RFQ e ha lo sha256 dichiarato (stessa disciplina di conferma_strutturale_contenuto).
CREATE FUNCTION conferma_strutturale_non_incluso_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    h record;
BEGIN
    SELECT x.tx, x.formato INTO h FROM conferma_strutturale x WHERE x.conferma_id = NEW.conferma_id;
    IF NOT FOUND THEN
        RETURN NEW;                                       -- lo rifiuta la FK
    END IF;
    IF h.tx <> pg_current_xact_id()
       OR position(NEW.conferma_id::text || ',' IN COALESCE(current_setting('cockpit.p4_conferme_controllate', true), '')) > 0 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma_strutturale_non_incluso: la conferma %s e'' gia'' registrata o controllata', NEW.conferma_id);
    END IF;
    IF h.formato <> 3 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s (formato %s): i nodi non inclusi esistono solo nel formato 3', NEW.conferma_id, h.formato);
    END IF;
    -- l'allegato e' della RFQ della conferma, come per P4X08
    IF NOT EXISTS (SELECT 1 FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
                    WHERE a.allegato_id = NEW.allegato_id AND a.sha256 = NEW.sha256 AND m.thread_id = NEW.thread_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X01',
            MESSAGE = format('conferma %s: l''allegato %s non e'' della RFQ %s o non ha lo sha256 %s del nodo non incluso',
                             NEW.conferma_id, NEW.allegato_id, NEW.thread_id, NEW.sha256);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_csni_nasce BEFORE INSERT ON conferma_strutturale_non_incluso
    FOR EACH ROW EXECUTE FUNCTION conferma_strutturale_non_incluso_nasce();
CREATE TRIGGER trg_csni_immutabile BEFORE UPDATE OR DELETE ON conferma_strutturale_non_incluso
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X01', 'il contenuto di una conferma non cambia');
CREATE TRIGGER trg_csni_non_si_svuota BEFORE TRUNCATE ON conferma_strutturale_non_incluso
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X01', 'le conferme non si svuotano');

-- ============================================================ 7. IL CONTROLLO AL COMMIT DELLA CONFERMA
-- Corpo della 0022 con un blocco in piu' per il formato 3, prima del controllo della bozza: (i) ogni componente
-- certificato ha il suo nodo per chiave_bozza, la mappa lo conferma, e ne porta l'identita' decisa; (ii) ogni arco
-- certificato e' un arco dello snapshot con la stessa origine e la stessa qta; (iii) i nodi non inclusi sono tutte e
-- sole le decisioni non_includere dello snapshot. Il trigger di vincolo trg_conferma_strutturale_completa resta com'e'.
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
    IF NEW.formato = 3 THEN                                                                           -- (0023)
        -- (i) ogni componente certificato ha il suo nodo per chiave_bozza (obbligatoria nei formati 2 e 3), la mappa  -- (0023)
        --     lo conferma, e porta l'identita' decisa del nodo: codice, rev, emittente, presunzione, namespace.  -- (0023)
        --     Sono le stesse condizioni che R4 calcola: un rifiuto qui e' un difetto del server.     -- (0023)
        WITH n AS (SELECT x ->> 'chiave_bozza' AS chiave, x -> 'identita' AS id                       -- (0023)
                     FROM jsonb_array_elements(NEW.snapshot -> 'nodi') x)                             -- (0023)
        SELECT k.componente_id INTO manca                                                             -- (0023)
          FROM conferma_strutturale_componente k                                                      -- (0023)
          LEFT JOIN n ON n.chiave = k.chiave_bozza                                                    -- (0023)
         WHERE k.conferma_id = NEW.conferma_id                                                        -- (0023)
           AND (n.chiave IS NULL                                                                      -- (0023)
                OR (NEW.mappa ->> k.chiave_bozza) IS DISTINCT FROM k.componente_id::text              -- (0023)
                OR n.id ->> 'codice' IS DISTINCT FROM k.codice                                        -- (0023)
                OR NULLIF(n.id ->> 'rev', '') IS DISTINCT FROM NULLIF(k.rev, '')                      -- (0023: '' = senza rev)
                OR n.id ->> 'emittente' IS DISTINCT FROM k.emittente                                  -- (0023)
                OR COALESCE((n.id ->> 'emittente_presunto')::boolean, false) IS DISTINCT FROM k.emittente_presunto  -- (0023)
                OR n.id ->> 'namespace' IS DISTINCT FROM k.namespace)                                 -- (0023)
         LIMIT 1;                                                                                     -- (0023)
        IF FOUND THEN                                                                                 -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma %s: il componente %s certificato non ha il suo nodo, o non ne porta l''identita'' decisa (codice, rev, emittente, presunzione, namespace)',  -- (0023)
                                 NEW.conferma_id, manca.componente_id);                               -- (0023)
        END IF;                                                                                       -- (0023)
        -- (ii) ogni arco certificato e' un arco dello snapshot con la stessa origine e la stessa qta; «step» solo con  -- (0023)
        --      archi[].step                                                                          -- (0023)
        WITH m AS (SELECT key AS chiave, value::uuid AS componente_id FROM jsonb_each_text(NEW.mappa)),  -- (0023)
             a AS (SELECT x ->> 'padre' AS padre, x ->> 'figlio' AS figlio, x ->> 'origine' AS origine, x -> 'step' AS step,  -- (0023)
                          x ->> 'qta' AS qta                                                          -- (0023)
                     FROM jsonb_array_elements(NEW.snapshot -> 'archi') x)                            -- (0023)
        SELECT c.padre_id, c.figlio_id INTO manca                                                     -- (0023)
          FROM conferma_strutturale_arco c                                                            -- (0023)
         WHERE c.conferma_id = NEW.conferma_id                                                        -- (0023)
           AND NOT EXISTS (SELECT 1 FROM a                                                            -- (0023)
                             JOIN m mp ON mp.chiave = a.padre                                         -- (0023)
                             JOIN m mf ON mf.chiave = a.figlio                                        -- (0023)
                            WHERE mp.componente_id = c.padre_id AND mf.componente_id = c.figlio_id    -- (0023)
                              AND a.origine = c.origine                                               -- (0023)
                              AND a.qta = c.qta::text                                                 -- (0023)
                              AND (c.origine = 'step') = COALESCE(jsonb_typeof(a.step) = 'object', false))  -- (0023)
         LIMIT 1;                                                                                     -- (0023)
        IF FOUND THEN                                                                                 -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma %s: l''arco certificato %s → %s non ha il suo arco nello snapshot (origine, qta) o e'' step senza step',  -- (0023)
                                 NEW.conferma_id, manca.padre_id, manca.figlio_id);                   -- (0023)
        END IF;                                                                                       -- (0023)
        -- (iii) i nodi non inclusi sono tutte e sole le decisioni non_includere dello snapshot, con casella (sha,  -- (0023)
        --       chiave, allegato, gruppo), azione, motivo e segnale uguali                           -- (0023)
        WITH d AS (SELECT x ->> 'id' AS id,                                                           -- (0023)
                          substr(x -> 'casella' ->> 'rif', 6, 64) AS sha,                             -- (0023)
                          substr(x -> 'casella' ->> 'rif', 71) AS chiave,                             -- (0023)
                          x -> 'casella' ->> 'allegato_id' AS allegato,                               -- (0023)
                          x -> 'casella' ->> 'gruppo' AS gruppo,                                      -- (0023)
                          x ->> 'azione_id' AS azione,                                                -- (0023)
                          x ->> 'motivo_codice' AS motivo_codice,                                     -- (0023)
                          x ->> 'motivo' AS motivo,                                                   -- (0023)
                          x -> 'segnale_visto' AS segnale                                             -- (0023)
                     FROM jsonb_array_elements(NEW.snapshot -> 'decisioni') x  -- un array: ck_conferma_snapshot  -- (0023)
                    WHERE x ->> 'tipo' = 'non_includere'),                                            -- (0023)
             r AS (SELECT * FROM conferma_strutturale_non_incluso WHERE conferma_id = NEW.conferma_id)  -- (0023)
        SELECT COALESCE(r.decisione_id, d.id) AS id INTO manca                                        -- (0023)
          FROM r FULL JOIN d ON d.id = r.decisione_id                                                 -- (0023)
         WHERE r.decisione_id IS NULL OR d.id IS NULL                                                 -- (0023)
            OR (d.sha, d.chiave, d.allegato, d.gruppo, d.azione, d.motivo_codice, d.motivo)           -- (0023)
               IS DISTINCT FROM (r.sha256::text, r.chiave, r.allegato_id::text, r.gruppo, r.azione_id::text, r.motivo_codice, r.motivo)  -- (0023)
            OR d.segnale IS DISTINCT FROM r.segnale                                                   -- (0023)
         LIMIT 1;                                                                                     -- (0023)
        IF FOUND THEN                                                                                 -- (0023)
            RAISE EXCEPTION USING ERRCODE = 'P4X01',                                                  -- (0023)
                MESSAGE = format('conferma %s: i nodi non inclusi non sono le decisioni non_includere dello snapshot (%s)', NEW.conferma_id, manca.id);  -- (0023)
        END IF;                                                                                       -- (0023)
    END IF;                                                                                           -- (0023)
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

-- ============================================================ 8. ESITO DEGLI INTENTI
ALTER TABLE esito_intento DROP CONSTRAINT ck_esito_intento_gesto, DROP CONSTRAINT ck_esito_intento_conferma;
ALTER TABLE esito_intento
    ADD CONSTRAINT ck_esito_intento_gesto CHECK (gesto IN (
        'conferma_bom_v2', 'conferma_bom_v3', 'lotto_fascicolo', 'correggi_documento', 'fonte_step', 'riprova_copia',
        'scarta_file', 'altri_file', 'riattiva_file', 'associa_step')),
    ADD CONSTRAINT ck_esito_intento_conferma CHECK ((gesto IN ('conferma_bom_v2', 'conferma_bom_v3')) = (conferma_id IS NOT NULL));

-- Il gesto della conferma dice il suo formato: conferma_bom_v2 per una formato 2, conferma_bom_v3 per una formato 3.
CREATE FUNCTION esito_intento_formato() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    f smallint;
BEGIN
    IF NEW.conferma_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT c.formato INTO f FROM conferma_strutturale c WHERE c.conferma_id = NEW.conferma_id;
    IF FOUND AND ((NEW.gesto = 'conferma_bom_v2') <> (f = 2) OR (NEW.gesto = 'conferma_bom_v3') <> (f = 3)) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X03',
            MESSAGE = format('esito_intento %s: il gesto %s non e'' quello di una conferma formato %s', NEW.chiave, NEW.gesto, f);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_esito_intento_formato BEFORE INSERT ON esito_intento
    FOR EACH ROW EXECUTE FUNCTION esito_intento_formato();

-- ============================================================ 9. CICLO DEI FILE DELLA SORGENTE
-- Lo stato dell'occorrenza (RFQ, allegato) nella RFQ. «Nessuna riga» vale «attivo»: la riga nasce al primo gesto,
-- in altri_file o scartato. Lo scrive solo il servizio del ciclo dei file (rotte della Sorgente e custode).
CREATE TABLE file_rfq (
    thread_id      uuid NOT NULL REFERENCES thread_offerta,
    allegato_id    uuid NOT NULL REFERENCES allegato,
    stato          text NOT NULL,
    versione       int  NOT NULL,
    -- fotografia dell'occorrenza al primo gesto (occorrenza, nome, SHA, origine)
    messaggio_id   uuid NOT NULL REFERENCES messaggio,
    canale         canale NOT NULL,
    contenitore_id uuid REFERENCES allegato,
    indice         smallint NOT NULL,
    nome_file      varchar(300) NOT NULL,
    path_interno   varchar(500),
    estensione     varchar(10),
    sha256         char(64),
    bytes          bigint,
    origine        origine_allegato NOT NULL,
    ricevuto_il    timestamptz NOT NULL,
    caricato_da    uuid REFERENCES utente,
    creato_da      uuid NOT NULL REFERENCES utente,
    creato_il      timestamptz NOT NULL DEFAULT now(),
    aggiornato_da  uuid NOT NULL REFERENCES utente,
    aggiornato_il  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, allegato_id),
    CONSTRAINT ck_file_rfq_stato CHECK (stato IN ('attivo', 'altri_file', 'scartato')),
    CONSTRAINT ck_file_rfq_versione CHECK (versione >= 1),
    CONSTRAINT ck_file_rfq_sha CHECK (sha256 IS NULL OR sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_file_rfq_bytes CHECK (bytes IS NULL OR bytes >= 0)
);
CREATE INDEX ix_file_rfq_allegato ON file_rfq (allegato_id);
CREATE INDEX ix_file_rfq_non_attivi ON file_rfq (thread_id) WHERE stato <> 'attivo';

CREATE FUNCTION file_rfq_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    a record;
BEGIN
    SELECT x.messaggio_id, x.contenitore_id, x.indice, x.nome_file, x.path_interno, x.estensione, x.sha256, x.bytes,
           x.origine, x.ricevuto_il, x.caricato_da, x.natura, m.thread_id, m.canale
      INTO a
      FROM allegato x JOIN messaggio m ON m.messaggio_id = x.messaggio_id
     WHERE x.allegato_id = NEW.allegato_id;
    IF NOT FOUND THEN
        RETURN NEW;                                       -- lo rifiuta la FK
    END IF;
    IF a.thread_id IS DISTINCT FROM NEW.thread_id THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq: l''allegato %s non e'' della RFQ %s', NEW.allegato_id, NEW.thread_id);
    END IF;
    IF a.natura NOT IN ('file', 'elemento_outlook') THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq: l''allegato %s e'' di natura %s, che il ciclo dei file non gestisce', NEW.allegato_id, a.natura);
    END IF;
    IF (a.messaggio_id, a.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno, a.estensione, a.sha256,
        a.bytes, a.origine, a.ricevuto_il, a.caricato_da)
       IS DISTINCT FROM (NEW.messaggio_id, NEW.canale, NEW.contenitore_id, NEW.indice, NEW.nome_file, NEW.path_interno,
                         NEW.estensione, NEW.sha256, NEW.bytes, NEW.origine, NEW.ricevuto_il, NEW.caricato_da) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq: la fotografia dell''occorrenza %s non e'' quella dell''allegato', NEW.allegato_id);
    END IF;
    IF NEW.stato NOT IN ('altri_file', 'scartato') OR NEW.versione <> 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: una riga nasce in altri_file o scartato, alla versione 1 (arrivata: %s, versione %s)',
                             NEW.allegato_id, NEW.stato, NEW.versione);
    END IF;
    NEW.creato_il := now();
    NEW.aggiornato_il := now();
    NEW.aggiornato_da := NEW.creato_da;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_file_rfq_nasce BEFORE INSERT ON file_rfq FOR EACH ROW EXECUTE FUNCTION file_rfq_nasce();

-- Transizioni: attivo → altri_file | scartato; altri_file → attivo | scartato; scartato e' terminale in V1.
CREATE FUNCTION file_rfq_cambia() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.thread_id, NEW.allegato_id, NEW.messaggio_id, NEW.canale, NEW.contenitore_id, NEW.indice, NEW.nome_file,
        NEW.path_interno, NEW.estensione, NEW.sha256, NEW.bytes, NEW.origine, NEW.ricevuto_il, NEW.caricato_da,
        NEW.creato_da, NEW.creato_il)
       IS DISTINCT FROM (OLD.thread_id, OLD.allegato_id, OLD.messaggio_id, OLD.canale, OLD.contenitore_id, OLD.indice,
                         OLD.nome_file, OLD.path_interno, OLD.estensione, OLD.sha256, OLD.bytes, OLD.origine,
                         OLD.ricevuto_il, OLD.caricato_da, OLD.creato_da, OLD.creato_il) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: identita'', fotografia e creazione non cambiano', OLD.allegato_id);
    END IF;
    IF NEW.versione <> OLD.versione + 1 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: la versione cresce di uno (%s → %s)', OLD.allegato_id, OLD.versione, NEW.versione);
    END IF;
    IF NOT ((OLD.stato = 'attivo' AND NEW.stato IN ('altri_file', 'scartato'))
            OR (OLD.stato = 'altri_file' AND NEW.stato IN ('attivo', 'scartato'))) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: da %s a %s non e'' ammesso (lo scarto e'' terminale)', OLD.allegato_id, OLD.stato, NEW.stato);
    END IF;
    NEW.aggiornato_il := now();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_file_rfq_cambia BEFORE UPDATE ON file_rfq FOR EACH ROW EXECUTE FUNCTION file_rfq_cambia();
CREATE TRIGGER trg_file_rfq_non_si_cancella BEFORE DELETE ON file_rfq
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X09', 'lo stato di un file nella RFQ non si cancella');
CREATE TRIGGER trg_file_rfq_non_si_svuota BEFORE TRUNCATE ON file_rfq
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X09', 'lo stato dei file non si svuota');

-- Il registro in sola aggiunta. FK verso RFQ e allegato (non verso file_rfq): un evento di conservazione su un file
-- che resta attivo non ha una riga di stato. origine legacy: il ramo HTML di «carica».
CREATE TABLE file_rfq_evento (
    evento_id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id          uuid NOT NULL REFERENCES thread_offerta,
    allegato_id        uuid NOT NULL REFERENCES allegato,
    azione             text NOT NULL,
    stato_prima        text,
    stato_dopo         text,
    motivo             text,
    fotografia         jsonb,
    dipendenze         jsonb,
    esito              jsonb,
    altro_allegato_id  uuid REFERENCES allegato,
    origine            text NOT NULL,
    autore_id          uuid REFERENCES utente,
    chiave_idempotenza uuid,
    quando             timestamptz NOT NULL DEFAULT now(),
    tx                 xid8 NOT NULL DEFAULT pg_current_xact_id(),
    CONSTRAINT ck_fre_azione CHECK (azione IN ('scarta', 'altri_file', 'riattiva',
        'rimozione_eseguita', 'rimozione_non_necessaria', 'rimozione_errore',
        'conservazione_copiata', 'conservazione_errore', 'conservazione_rilasciata',
        'ricaricato_dopo_scarto', 'anomalia_riestrazione')),
    CONSTRAINT ck_fre_stati CHECK ((stato_prima IS NULL OR stato_prima IN ('attivo', 'altri_file', 'scartato'))
        AND (stato_dopo IS NULL OR stato_dopo IN ('attivo', 'altri_file', 'scartato'))),
    CONSTRAINT ck_fre_origine CHECK (origine IN ('ponte', 'legacy', 'server')),
    -- i tre rami delle decisioni con COALESCE(..., false): uno stato_prima o uno stato_dopo NULL non fa passare il CHECK
    -- (NULL non e' falso)
    CONSTRAINT ck_fre_coerente CHECK (
        (azione <> 'scarta' OR COALESCE(origine = 'ponte' AND stato_prima IN ('attivo', 'altri_file') AND stato_dopo = 'scartato'
                                         AND motivo IS NOT NULL AND fotografia IS NOT NULL AND dipendenze IS NOT NULL, false))
        AND (azione <> 'altri_file' OR COALESCE(origine = 'ponte' AND stato_prima = 'attivo' AND stato_dopo = 'altri_file'
                                                 AND fotografia IS NOT NULL AND dipendenze IS NOT NULL, false))
        AND (azione <> 'riattiva' OR COALESCE(origine = 'ponte' AND stato_prima = 'altri_file' AND stato_dopo = 'attivo'
                                               AND fotografia IS NOT NULL, false))
        AND (azione NOT IN ('rimozione_eseguita', 'rimozione_non_necessaria', 'rimozione_errore', 'conservazione_copiata',
                            'conservazione_errore', 'conservazione_rilasciata', 'anomalia_riestrazione')
             OR (origine = 'server' AND esito IS NOT NULL AND stato_prima IS NULL AND stato_dopo IS NULL))
        AND (azione <> 'ricaricato_dopo_scarto' OR (origine IN ('ponte', 'legacy') AND altro_allegato_id IS NOT NULL
                                                    AND stato_prima IS NULL AND stato_dopo IS NULL))
        AND (origine = 'server' OR autore_id IS NOT NULL)),
    CONSTRAINT ck_fre_motivo CHECK (motivo IS NULL OR (btrim(motivo) <> '' AND char_length(motivo) <= 1000)),
    CONSTRAINT ck_fre_json CHECK ((fotografia IS NULL OR (jsonb_typeof(fotografia) = 'object' AND pg_column_size(fotografia) <= 65536))
        AND (dipendenze IS NULL OR (jsonb_typeof(dipendenze) = 'object' AND pg_column_size(dipendenze) <= 262144))
        AND (esito IS NULL OR (jsonb_typeof(esito) = 'object' AND pg_column_size(esito) <= 65536)))
);
CREATE INDEX ix_fre_thread ON file_rfq_evento (thread_id, quando);
CREATE INDEX ix_fre_allegato ON file_rfq_evento (allegato_id, quando);
CREATE UNIQUE INDEX ux_fre_chiave ON file_rfq_evento (chiave_idempotenza, allegato_id)
    WHERE chiave_idempotenza IS NOT NULL AND azione IN ('scarta', 'altri_file', 'riattiva');
CREATE TRIGGER trg_fre_ora BEFORE INSERT ON file_rfq_evento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('quando');
CREATE TRIGGER trg_fre_solo_aggiunte BEFORE UPDATE OR DELETE ON file_rfq_evento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'un evento della vita di un file non si modifica e non si cancella');
CREATE TRIGGER trg_fre_non_si_svuota BEFORE TRUNCATE ON file_rfq_evento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'il registro dei file non si svuota');

-- Lo stato della rimozione dallo staging: una per occorrenza (lo scarto e' terminale), un passo tracciato e idempotente.
CREATE TABLE file_rfq_rimozione (
    rimozione_id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id             uuid NOT NULL,
    allegato_id           uuid NOT NULL,
    evento_id             bigint NOT NULL REFERENCES file_rfq_evento,
    sha256                char(64),
    percorso_trovato      varchar(500),                -- il path_staging trovato al tempo 2; lo scrive il server, mai il client
    stato                 text NOT NULL DEFAULT 'in_attesa',
    motivo                text NOT NULL DEFAULT 'da_eseguire',
    ritentabile           boolean NOT NULL DEFAULT true,
    errore                text,
    tentativi             int NOT NULL DEFAULT 0,
    prossimo_tentativo_il timestamptz,
    creato_il             timestamptz NOT NULL DEFAULT now(),
    aggiornato_il         timestamptz NOT NULL DEFAULT now(),
    concluso_il           timestamptz,
    CONSTRAINT fk_rimozione_file FOREIGN KEY (thread_id, allegato_id)
        REFERENCES file_rfq (thread_id, allegato_id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT ux_rimozione_occorrenza UNIQUE (thread_id, allegato_id),
    CONSTRAINT ck_rim_stato CHECK (stato IN ('in_attesa', 'eseguita', 'non_necessaria_condiviso', 'errore')),
    -- pin_transitorio al posto della chiusura su «pin»; rfq_cambiata; bonificato, la sola uscita da un errore non
    -- ritentabile
    CONSTRAINT ck_rim_motivo CHECK (
           (stato = 'in_attesa' AND motivo IN ('da_eseguire', 'lavoro_in_corso', 'copia_nas_in_attesa', 'anomalia_nas', 'file_giovane',
                                               'pin_transitorio'))
        OR (stato = 'errore' AND motivo IN ('file_occupato', 'errore_io', 'fuori_cache', 'percorso_non_valido'))
        OR (stato = 'eseguita' AND motivo IN ('rimosso', 'gia_assente', 'bonificato'))
        OR (stato = 'non_necessaria_condiviso' AND motivo IN ('condiviso', 'rfq_cambiata'))),
    CONSTRAINT ck_rim_ritentabile CHECK (ritentabile = (motivo NOT IN ('fuori_cache', 'percorso_non_valido'))),
    CONSTRAINT ck_rim_errore CHECK ((stato = 'errore') = (errore IS NOT NULL) AND (errore IS NULL OR char_length(errore) <= 2000)),
    CONSTRAINT ck_rim_tentativi CHECK (tentativi >= 0),
    CONSTRAINT ck_rim_sha CHECK (sha256 IS NULL OR sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_rim_concluso CHECK ((stato IN ('eseguita', 'non_necessaria_condiviso')) = (concluso_il IS NOT NULL))
);
CREATE INDEX ix_rim_custode ON file_rfq_rimozione (prossimo_tentativo_il)
    WHERE stato IN ('in_attesa', 'errore') AND ritentabile;
CREATE TRIGGER trg_rim_ora BEFORE INSERT ON file_rfq_rimozione
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('creato_il', 'aggiornato_il');

CREATE FUNCTION file_rfq_rimozione_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM file_rfq_evento e WHERE e.evento_id = NEW.evento_id AND e.azione = 'scarta'
                      AND e.thread_id = NEW.thread_id AND e.allegato_id = NEW.allegato_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione: l''evento %s non e'' lo scarto dell''occorrenza %s', NEW.evento_id, NEW.allegato_id);
    END IF;
    IF NEW.stato <> 'in_attesa' OR NEW.tentativi <> 0 OR NEW.concluso_il IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: nasce in_attesa, senza tentativi', NEW.allegato_id);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_rim_nasce BEFORE INSERT ON file_rfq_rimozione FOR EACH ROW EXECUTE FUNCTION file_rfq_rimozione_nasce();

CREATE FUNCTION file_rfq_rimozione_cambia() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.thread_id, NEW.allegato_id, NEW.evento_id, NEW.sha256, NEW.creato_il)
       IS DISTINCT FROM (OLD.thread_id, OLD.allegato_id, OLD.evento_id, OLD.sha256, OLD.creato_il) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: identita'' e creazione non cambiano', OLD.rimozione_id);
    END IF;
    IF OLD.stato IN ('eseguita', 'non_necessaria_condiviso') THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: e'' conclusa (%s, %s)', OLD.rimozione_id, OLD.stato, OLD.motivo);
    END IF;
    -- un errore non ritentabile non si riprende da solo: l'unica uscita e' eseguita / bonificato, quando il controllo
    -- giornaliero del custode trova assente il file di percorso_trovato, sotto la radice dello staging; mai per il solo
    -- path_staging nullo. E bonificato nasce solo da li'. L'UPDATE di chiusura porta anche errore a NULL e ritentabile
    -- a vero, come vogliono ck_rim_errore e ck_rim_ritentabile.
    IF OLD.stato = 'errore' AND NOT OLD.ritentabile AND NOT (NEW.stato = 'eseguita' AND NEW.motivo = 'bonificato') THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: da un errore non ritentabile si esce solo con eseguita / bonificato', OLD.rimozione_id);
    END IF;
    IF NEW.motivo = 'bonificato' AND NOT (OLD.stato = 'errore' AND NOT OLD.ritentabile) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: bonificato vale solo per chiudere un errore non ritentabile', OLD.rimozione_id);
    END IF;
    IF NEW.tentativi < OLD.tentativi THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: i tentativi non calano', OLD.rimozione_id);
    END IF;
    IF OLD.percorso_trovato IS NOT NULL AND NEW.percorso_trovato IS DISTINCT FROM OLD.percorso_trovato THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_rimozione %s: il percorso trovato non cambia', OLD.rimozione_id);
    END IF;
    NEW.aggiornato_il := now();
    IF NEW.stato IN ('eseguita', 'non_necessaria_condiviso') THEN
        NEW.concluso_il := now();
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_rim_cambia BEFORE UPDATE ON file_rfq_rimozione FOR EACH ROW EXECUTE FUNCTION file_rfq_rimozione_cambia();
CREATE TRIGGER trg_rim_non_si_cancella BEFORE DELETE ON file_rfq_rimozione
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia di una rimozione non si cancella');
CREATE TRIGGER trg_rim_non_si_svuota BEFORE TRUNCATE ON file_rfq_rimozione
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le rimozioni non si svuotano');

-- La copia nel deposito di conservazione degli «Altri file» ([conservazione].radice in cockpit.toml). La riga nasce
-- prima della transizione (fase A): FK verso RFQ e allegato, non verso file_rfq. Il percorso e' relativo alla radice,
-- lo calcola il trigger di nascita e ha la sola forma <thread_id>\<allegato_id>.<conservazione_id>.<ext>: un file per
-- riga, mai condiviso fra due righe, nemmeno fra due cicli della stessa occorrenza. deposito_id e' l'UUID del
-- segnaposto della radice in cui la copia e' stata scritta.
CREATE TABLE file_rfq_conservazione (
    conservazione_id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id             uuid NOT NULL REFERENCES thread_offerta,
    allegato_id           uuid NOT NULL REFERENCES allegato,
    sha256                char(64) NOT NULL,
    bytes                 bigint,
    percorso_relativo     varchar(500) NOT NULL,
    deposito_id           uuid NOT NULL,
    sorgente_byte         text,
    stato                 text NOT NULL DEFAULT 'in_copia',
    causa                 text,
    errore                text,
    tentativi             int NOT NULL DEFAULT 0,
    prossimo_tentativo_il timestamptz,
    chiave_idempotenza    uuid,
    iniziata_il           timestamptz NOT NULL DEFAULT now(),
    verificato_il         timestamptz,
    aggiornato_il         timestamptz NOT NULL DEFAULT now(),     -- il battito della copia (in_copia → in_copia)
    concluso_il           timestamptz,
    CONSTRAINT ck_cons_stato CHECK (stato IN ('in_copia', 'copiata', 'errore_copia', 'da_rilasciare', 'rilasciata')),
    CONSTRAINT ck_cons_sha CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_cons_percorso CHECK (percorso_relativo ~ ('^' || thread_id::text || '[\\/]' || allegato_id::text
                                                            || '\.' || conservazione_id::text || '\.[a-z0-9]{1,10}$')),
    CONSTRAINT ck_cons_sorgente CHECK (sorgente_byte IS NULL OR sorgente_byte IN ('staging', 'conservazione', 'nas')),
    -- causa: obbligatoria con errore_copia; facoltativa con da_rilasciare e rilasciata (perche' la copia si rilascia)
    CONSTRAINT ck_cons_causa CHECK ((stato <> 'errore_copia' OR causa IS NOT NULL)
        AND (stato NOT IN ('in_copia', 'copiata') OR causa IS NULL)
        AND (causa IS NULL OR causa IN ('spazio', 'permessi', 'sorgente_illeggibile', 'verifica_sha', 'conflitto', 'tempo_scaduto',
                                        'rifiutata', 'riattivato', 'scartato'))),
    CONSTRAINT ck_cons_copiata CHECK (stato <> 'copiata' OR (verificato_il IS NOT NULL AND sorgente_byte IS NOT NULL)),
    CONSTRAINT ck_cons_errore CHECK (errore IS NULL OR char_length(errore) <= 2000),
    CONSTRAINT ck_cons_tentativi CHECK (tentativi >= 0),
    CONSTRAINT ck_cons_concluso CHECK ((stato IN ('errore_copia', 'rilasciata')) = (concluso_il IS NOT NULL))
);
-- una sola copia in corso o copiata per occorrenza; le righe da_rilasciare non contano: ognuna ha il suo file
CREATE UNIQUE INDEX ux_cons_viva ON file_rfq_conservazione (thread_id, allegato_id)
    WHERE stato IN ('in_copia', 'copiata');
CREATE INDEX ix_cons_custode ON file_rfq_conservazione (prossimo_tentativo_il)
    WHERE stato IN ('in_copia', 'da_rilasciare');
CREATE INDEX ix_cons_allegato ON file_rfq_conservazione (allegato_id) WHERE stato IN ('copiata', 'da_rilasciare');
CREATE TRIGGER trg_cons_ora BEFORE INSERT ON file_rfq_conservazione
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('iniziata_il', 'aggiornato_il');

CREATE FUNCTION file_rfq_conservazione_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    ext text;
BEGIN
    SELECT CASE WHEN lower(a.estensione) ~ '^[a-z0-9]{1,10}$' THEN lower(a.estensione) ELSE 'bin' END INTO ext
      FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
     WHERE a.allegato_id = NEW.allegato_id AND m.thread_id = NEW.thread_id AND a.sha256 = NEW.sha256;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_conservazione: l''allegato %s non e'' della RFQ %s con lo sha256 %s', NEW.allegato_id, NEW.thread_id, NEW.sha256);
    END IF;
    IF NEW.stato <> 'in_copia' OR NEW.verificato_il IS NOT NULL OR NEW.concluso_il IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_conservazione %s: nasce in_copia', NEW.allegato_id);
    END IF;
    -- il percorso lo calcola il database, mai il client ne' il Go
    NEW.percorso_relativo := NEW.thread_id::text || '\' || NEW.allegato_id::text || '.' || NEW.conservazione_id::text || '.' || ext;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_cons_nasce BEFORE INSERT ON file_rfq_conservazione FOR EACH ROW EXECUTE FUNCTION file_rfq_conservazione_nasce();
-- conservazione_id (identita') ha gia' il suo valore nel trigger BEFORE: la riga si inserisce con un percorso
-- segnaposto e il trigger lo sostituisce; il Go legge il percorso con RETURNING.

-- Transizioni: in_copia → in_copia (battito) | copiata | errore_copia | da_rilasciare; copiata → da_rilasciare;
-- da_rilasciare → da_rilasciare (nuovo tentativo) | rilasciata. errore_copia e rilasciata sono finali.
CREATE FUNCTION file_rfq_conservazione_cambia() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.thread_id, NEW.allegato_id, NEW.sha256, NEW.percorso_relativo, NEW.deposito_id, NEW.chiave_idempotenza, NEW.iniziata_il)
       IS DISTINCT FROM (OLD.thread_id, OLD.allegato_id, OLD.sha256, OLD.percorso_relativo, OLD.deposito_id, OLD.chiave_idempotenza, OLD.iniziata_il) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_conservazione %s: identita'', percorso, deposito e chiave non cambiano', OLD.conservazione_id);
    END IF;
    IF NOT ((OLD.stato = 'in_copia' AND NEW.stato IN ('in_copia', 'copiata', 'errore_copia', 'da_rilasciare'))
            OR (OLD.stato = 'copiata' AND NEW.stato = 'da_rilasciare')
            OR (OLD.stato = 'da_rilasciare' AND NEW.stato IN ('da_rilasciare', 'rilasciata'))) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_conservazione %s: da %s a %s non e'' ammesso', OLD.conservazione_id, OLD.stato, NEW.stato);
    END IF;
    IF NEW.tentativi < OLD.tentativi THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_conservazione %s: i tentativi non calano', OLD.conservazione_id);
    END IF;
    NEW.aggiornato_il := now();
    IF NEW.stato = 'copiata' AND OLD.stato <> 'copiata' THEN
        NEW.verificato_il := now();
    END IF;
    IF NEW.stato IN ('errore_copia', 'rilasciata') THEN
        NEW.concluso_il := now();
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_cons_cambia BEFORE UPDATE ON file_rfq_conservazione FOR EACH ROW EXECUTE FUNCTION file_rfq_conservazione_cambia();
CREATE TRIGGER trg_cons_non_si_cancella BEFORE DELETE ON file_rfq_conservazione
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia di una copia di conservazione non si cancella');
CREATE TRIGGER trg_cons_non_si_svuota BEFORE TRUNCATE ON file_rfq_conservazione
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le copie di conservazione non si svuotano');

-- Al COMMIT: altri_file vuole una copia verificata; scartato vuole la riga della rimozione; ogni transizione ha il suo
-- evento nella stessa transazione.
CREATE FUNCTION file_rfq_invarianti() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    attuale text;
BEGIN
    SELECT f.stato INTO attuale FROM file_rfq f WHERE f.thread_id = NEW.thread_id AND f.allegato_id = NEW.allegato_id;
    IF attuale = 'altri_file' AND NOT EXISTS (SELECT 1 FROM file_rfq_conservazione c
            WHERE c.thread_id = NEW.thread_id AND c.allegato_id = NEW.allegato_id AND c.stato = 'copiata') THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: in Altri file senza una copia di conservazione verificata', NEW.allegato_id);
    END IF;
    IF attuale = 'scartato' AND NOT EXISTS (SELECT 1 FROM file_rfq_rimozione r
            WHERE r.thread_id = NEW.thread_id AND r.allegato_id = NEW.allegato_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: scartato senza la riga della rimozione', NEW.allegato_id);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM file_rfq_evento e
            WHERE e.thread_id = NEW.thread_id AND e.allegato_id = NEW.allegato_id
              AND e.stato_dopo = NEW.stato AND e.tx = pg_current_xact_id()) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq %s: la transizione verso %s non ha il suo evento', NEW.allegato_id, NEW.stato);
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER trg_file_rfq_invarianti AFTER INSERT OR UPDATE ON file_rfq
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION file_rfq_invarianti();

-- Lo stesso invariante visto dalla conservazione: una copia esce da «copiata» solo se, al COMMIT, l'occorrenza non e'
-- piu' in altri_file (Riattiva o Scarta nella stessa transazione), oppure ha un'altra copia copiata. Senza questo
-- controllo una scrittura sulla sola conservazione lascerebbe un file in Altri file senza copia.
CREATE FUNCTION file_rfq_conservazione_invarianti() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.stato = 'copiata' AND NEW.stato <> 'copiata'
       AND EXISTS (SELECT 1 FROM file_rfq f WHERE f.thread_id = NEW.thread_id AND f.allegato_id = NEW.allegato_id
                                              AND f.stato = 'altri_file')
       AND NOT EXISTS (SELECT 1 FROM file_rfq_conservazione c
                        WHERE c.thread_id = NEW.thread_id AND c.allegato_id = NEW.allegato_id AND c.stato = 'copiata') THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X09',
            MESSAGE = format('file_rfq_conservazione %s: la copia dell''occorrenza %s in Altri file non e'' piu'' copiata e il file non ha lasciato Altri file',
                             NEW.conservazione_id, NEW.allegato_id);
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER trg_cons_invarianti AFTER UPDATE ON file_rfq_conservazione
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION file_rfq_conservazione_invarianti();

-- Il predicato «occorrenza attiva nella sua RFQ», usato da query e viste. RFQ nulla: vale attiva. Uno scarto fatto col
-- vecchio «Scarta il file» (nessuna riga qui) vale attivo per il predicato: le viste e i lettori legacy escludono gia'
-- le proposte scartate, e il ciclo dei file lo tratta come scartato nel Go.
CREATE FUNCTION file_rfq_attivo(p_thread uuid, p_allegato uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT p_thread IS NULL OR NOT EXISTS (
        SELECT 1 FROM file_rfq f WHERE f.thread_id = p_thread AND f.allegato_id = p_allegato AND f.stato <> 'attivo')
$$;

-- ============================================================ 10. LE CINQUE VISTE (deroga dichiarata)
-- Testo di oggi (0022 per v_fascicolo, 0020 per v_step_prodotto e v_thread_da_riesaminare, 0018 per v_cruscotto e
-- v_codici_candidati_thread) con il solo predicato file_rfq_attivo aggiunto sulle righe marcate «(0023)»: stesse
-- colonne, stesso ordine. Le viste che leggono queste per nome (v_thread_bloccanti, ...) non cambiano.
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
    WHERE p.thread_id = co.thread_id AND p.stato = 'aperta' AND p.tipo_proposto = f.tipo AND file_rfq_attivo(p.thread_id, p.allegato_id)  -- (0023)
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
         WHERE dp.thread_id = co.thread_id AND dp.stato = 'aperta' AND dp.tipo_proposto = 'cad_3d' AND file_rfq_attivo(dp.thread_id, dp.allegato_id)  -- (0023)
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
                 WHERE p.thread_id = u.thread_id AND p.stato = 'aperta' AND p.tipo_proposto IN ('cad_3d', 'disegno_2d', 'sviluppo_dxf') AND file_rfq_attivo(p.thread_id, p.allegato_id)  -- (0023)
                   AND p.creato_il > u.congelata_il) AS n_file,
               (SELECT count(*) FROM componente_proposta c
                 WHERE c.thread_id = u.thread_id AND c.stato = 'aperta' AND c.creato_il > u.congelata_il AND file_rfq_attivo(c.thread_id, c.allegato_id))  -- (0023)
             + (SELECT count(*) FROM relazione_proposta r
                 WHERE r.thread_id = u.thread_id AND r.stato = 'aperta' AND r.creato_il > u.congelata_il AND file_rfq_attivo(r.thread_id, r.allegato_id))  -- (0023)
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
       (SELECT count(*) FROM documento_proposta p WHERE p.thread_id = t.thread_id AND p.stato = 'aperta' AND file_rfq_attivo(p.thread_id, p.allegato_id)) AS n_da_smistare  -- (0023)
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
 WHERE p.thread_id IS NOT NULL AND p.stato <> 'scartata' AND nullif(btrim(p.codice), '') IS NOT NULL AND file_rfq_attivo(p.thread_id, p.allegato_id)  -- (0023)
UNION ALL
SELECT p.thread_id, n.codice, '',
       'nome_file', 'nome_file', '', 50, a.nome_file::text, NULL,
       a.messaggio_id, p.allegato_id
  FROM documento_proposta p
  JOIN allegato a ON a.allegato_id = p.allegato_id
  CROSS JOIN LATERAL jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(p.dettagli -> 'codici_nel_nome') = 'array' THEN p.dettagli -> 'codici_nel_nome' ELSE '[]'::jsonb END
  ) AS n (codice)
 WHERE p.thread_id IS NOT NULL AND p.stato <> 'scartata' AND btrim(n.codice) <> '' AND file_rfq_attivo(p.thread_id, p.allegato_id)  -- (0023)
UNION ALL
SELECT cp.thread_id, cp.codice::text, COALESCE(cp.rev, '')::text,
       cp.fonte::text, cp.origine_codice::text, cp.famiglia::text, cp.confidenza::int,
       (cp.nome_grezzo || ' — ' || a.nome_file)::text, NULL,
       a.messaggio_id, cp.allegato_id
  FROM componente_proposta cp
  JOIN allegato a ON a.allegato_id = cp.allegato_id
 WHERE cp.stato <> 'scartata' AND nullif(btrim(cp.codice), '') IS NOT NULL AND file_rfq_attivo(cp.thread_id, cp.allegato_id);  -- (0023)

-- ============================================================ 11. COMMENTI
COMMENT ON TABLE distinta_importazione IS
'Registro d''importazione del Motore A (V1, formato 3): la proposta per un prodotto e un gruppo, com''era al momento
dell''importazione, con versioni, grammatiche, partner e contesto. Immutabile, citato dalle versioni della bozza e dalle
conferme. impronta e'' la chiave; impronta_contenuto si confronta alla riapertura; impronta_evidenze e'' solo
informativa; impronta_regole dice se sono cambiate le regole. Non si cancella in V1.';
COMMENT ON COLUMN distinta_importazione.fonte_radice IS
'Il rif della radice del gruppo importato (contenuto.gruppo.radice): con fonte_allegato_id e fonte_sha256 ritrova il
gruppo corrente della sorgente senza leggere il contenuto.';
COMMENT ON COLUMN distinta_bozza_versione.importazione_id IS
'Formato 3: il registro d''importazione di sorgente.importazione (stessa RFQ, stesso prodotto, stesso gruppo e stessa
fonte: trigger). NULL nel formato 2 e per una bozza formato 3 senza registro.';
COMMENT ON COLUMN distinta_bozza_versione.chiave_salvataggio IS
'Formato 3: la chiave del salvataggio R2, con impronta_richiesta: un reinvio con la stessa chiave non scrive un
doppione. Obbligatoria nel formato 3, NULL nel formato 2.';
COMMENT ON TABLE conferma_strutturale_non_incluso IS
'Nodi non inclusi nella BOM da una conferma formato 3 (LS-02): una riga per decisione non_includere, con la casella
d''origine, il motivo, il segnale visto e le righe di proposta chiuse. Immutabile.';
COMMENT ON COLUMN conferma_strutturale_componente.emittente IS
'Emittente dell''identita'' certificata (formato 3): cliente:<uuid> o partner:<id>. NULL nei formati 1 e 2.';
COMMENT ON COLUMN conferma_strutturale_componente.emittente_presunto IS
'Vero se l''emittente certificato e'' una presunzione (componente legacy, nodo scritto a mano, nodo formato_2): la
conferma non la trasforma in un''identita'' certa. NULL nei formati 1 e 2.';
COMMENT ON COLUMN conferma_strutturale_componente.namespace IS
'Namespace dell''identita'' certificata (formato 3), se l''identita'' decisa ne ha uno. NULL nei formati 1 e 2.';
COMMENT ON COLUMN conferma_strutturale.importazione_id IS
'Formato 3: il registro d''importazione della versione che la conferma chiude (sorgente.importazione). NULL nei formati
1 e 2.';
COMMENT ON COLUMN conferma_strutturale.importazioni_citate IS
'Formato 3: tutti i registri d''importazione citati dallo snapshot (la sorgente e le decisioni). NULL nei formati 1 e 2.';
-- Il COMMENT della 0022 su conferma_strutturale_arco.origine («ricalcolata dal server, mai copiata da quello che dice
-- la pagina») vale per il formato 2; per il formato 3 si aggiorna:
COMMENT ON COLUMN conferma_strutturale_arco.origine IS
'Da dove viene l''arco certificato: step (estratto dalla fonte STEP), utente (deciso da una persona), conferma o
registrato (gia'' nella conferma precedente o nella working). Obbligatoria nei formati 2 e 3 (trigger). Formato 2: la
ricalcola il server in R4/R5, mai copiata dalla pagina. Formato 3: e'' quella dichiarata dallo snapshot
(archi[].origine), verificata da R4 e R5 e dal controllo al COMMIT; se la verifica fallisce la conferma si rifiuta, mai
una riscrittura. Un arco disegnato a mano non diventa «estratto dallo STEP» (D-SHOCK-S1 B, E2.1).';
COMMENT ON TABLE distinta_bozza_versione IS
'Lo snapshot completo (formato 2 o 3, uguale al formato della testata: trigger) di ogni versione salvata della bozza,
con la base e la fonte di quel momento; nel formato 3 anche il registro d''importazione e la chiave del salvataggio.
Non si riscrive.';
COMMENT ON TABLE conferma_strutturale IS
'La conferma ufficiale della struttura di un prodotto: progressiva (numero) e immutabile. Formato 1 (giro 4,
senza_riepilogo_v2), 2 (R5, con chiave di idempotenza) o 3 (V1: R5 con chiave di idempotenza, chiude sempre una bozza
formato 3, cita i registri d''importazione). Diversa da bom_versione e da BOM.Verificata. mono_pezzo con motivo, senza
figli e con lo STEP autorizzato sulla radice. criterio_verifica: step, mono_pezzo o nessuno. La struttura ufficiale
nasce da questa conferma umana, non dall''accettazione dello STEP (D-SHOCK-S1 B).';
COMMENT ON TABLE file_rfq IS
'Stato dell''occorrenza di un file nella RFQ (attivo, altri_file, scartato), con la fotografia al primo gesto.
Nessuna riga = attivo. Lo scrive solo il servizio del ciclo dei file.';
COMMENT ON TABLE file_rfq_evento IS 'Vita dei file della Sorgente: decisioni ed esiti, in sola aggiunta.';
COMMENT ON TABLE file_rfq_rimozione IS
'Rimozione dallo staging dopo uno scarto: passo tracciato, idempotente e recuperabile. «File eliminato» solo con
eseguita/rimosso. Un errore non ritentabile si chiude solo con eseguita/bonificato, quando il file di percorso_trovato
non esiste piu''.';
COMMENT ON TABLE file_rfq_conservazione IS
'Copia verificata nel deposito di conservazione degli «Altri file» (fuori dallo staging e dal NAS): un file per riga,
<thread_id>\<allegato_id>.<conservazione_id>.<ext>, calcolato dal database.';
COMMENT ON FUNCTION file_rfq_attivo(uuid, uuid) IS
'Vero se l''occorrenza (RFQ, allegato) e'' attiva: nessuna riga in file_rfq, o la riga in stato attivo. RFQ nulla:
vera. Lo usano le cinque viste ridefinite dalla 0023 e le query delle proposte.';

-- ============================================================ 12. VARIANTE E VERSIONE
-- La variante lega il testo di questo file al database (platform/migrazioni, ControllaVarianti): ogni cambio del testo
-- prima della spinta cambia anche la variante (la prova L1 lega variante e impronta del file). 20261018a: il testo del
-- primo passaggio della corsia; 20261018b: dopo la revisione (CHECK del formato 3, identita' certificata, controllo (i)).
INSERT INTO schema_variante (versione, variante) VALUES (23, '0023-v1-formato3-file-20261018b');
INSERT INTO schema_versione (versione) VALUES (23);
