-- 0020_bom_versioni.sql — Blocco 8, A4 (B8.A4a): revisioni dei documenti, versioni congelate della BOM,
-- STEP strutturale, deroga strutturale, archiviazione.
--
-- Che cosa fa, in una riga: `documento` diventa una catena di revisioni che non si riscrive; la BOM
-- working (componente + componente_relazione + deroghe + documenti assegnati) si fotografa in versioni
-- congelate che non cambiano piu', e dopo il congelamento si modifica solo aprendo una revisione.
--
-- Riferimenti: addendum A4.1 (catena, percorso unico, nome_file), A4.4 (STEP strutturale,
-- rimozione_proposta, struttura_completa), A4.5 (deroga_struttura, esiti dello STEP del prodotto),
-- A4.6 (versioni, istantanee, immutabilita'), A4.7 (working bloccata, transizioni), A4.8
-- (v_thread_da_riesaminare), A4.9 (archiviazione), A4.10 (guardie). Decisioni D21–D38.
--
-- Regole del migratore: un file = una transazione (se una guardia o un'istruzione fallisce non cambia
-- niente); ogni REFERENCES punta a una tabella gia' creata; nessun valore di enum aggiunto qui; il
-- file termina con INSERT INTO schema_versione. Richiede la 0019 (versioni consecutive).
--
-- Errori dei trigger. Ognuno ha un codice suo, cosi' il Go lo riconosce senza leggere il testo:
--   BOM01  la BOM working e' bloccata: l'ultima versione della RFQ e' congelata (D26)
--   BOM02  una versione congelata, o una sua istantanea, non si tocca; una bozza si puo' solo congelare
--   BOM03  catena delle revisioni: mossa non ammessa (A4.1)
--   BOM04  STEP strutturale: il documento scelto non e' uno STEP corrente (A4.4)
--   BOM05  una deroga strutturale non si modifica (A4.5)
--
-- I trigger che leggono lo stato della BOM prendono lucchetti di riga e poi rileggono: valgono in READ
-- COMMITTED, che e' il livello di tutte le transazioni del server.
--
-- ORDINE: 1. guardie: TUTTE prima di qualunque modifica, e tutte in un solo elenco
--         2. tipi
--         3–4. documento: catena delle revisioni, percorso unico, nome_file
--         5. componente: archiviazione e STEP strutturale
--         6. deroga_struttura
--         7. rimozione_proposta
--         8–9. versioni della BOM e istantanee; immutabilita'
--         10. working bloccata dopo il congelamento (D26)
--         11. struttura_completa e l'analizzatore corrente
--         12. viste: archiviati fuori da fascicolo e albero; storia, STEP del prodotto, versioni, riesame
--         13. transizioni
--         14. versione

-- ============================================================ 1. GUARDIE
-- Nessuna guardia corregge: ciascuna conta, elenca (al massimo 20 righe) e ferma. Chi lancia la
-- migrazione riconcilia a mano e rilancia. Su cockpit_dev oggi c'e' 1 documento, senza sostituito_da:
-- attese tutte zero.
DO $$
DECLARE
    problemi text := '';
    riga     text;
BEGIN
    -- (a) due documenti che dichiarano lo stesso file sul NAS. lower() come l'indice del passo 4: la
    --     condivisione e' Windows e non distingue le maiuscole.
    FOR riga IN
        SELECT format('thread %s: %s documenti dichiarano %s (%s)', thread_id, count(*), min(path_relativo),
                      array_agg(documento_id ORDER BY confermato_il))
          FROM documento GROUP BY thread_id, lower(path_relativo) HAVING count(*) > 1 LIMIT 20
    LOOP problemi := problemi || E'\n(a) ' || riga; END LOOP;

    -- (b) una sostituzione fra RFQ, componenti o tipi diversi: le FK del passo 3 la rifiuterebbero
    --     con un errore anonimo.
    FOR riga IN
        SELECT format('documento %s sostituito da %s: %s', x.documento_id, y.documento_id,
                      concat_ws(', ', CASE WHEN x.thread_id <> y.thread_id THEN 'altra RFQ' END,
                                      CASE WHEN x.componente_id IS DISTINCT FROM y.componente_id THEN 'altro componente' END,
                                      CASE WHEN x.tipo <> y.tipo THEN format('tipo %s contro %s', x.tipo, y.tipo) END))
          FROM documento x JOIN documento y ON y.documento_id = x.sostituito_da
         WHERE x.thread_id <> y.thread_id OR x.componente_id IS DISTINCT FROM y.componente_id OR x.tipo <> y.tipo
         LIMIT 20
    LOOP problemi := problemi || E'\n(b) ' || riga; END LOOP;

    -- (c) un documento senza componente che risulta sostituito: si rivede un documento di un pezzo,
    --     non un capitolato sciolto.
    FOR riga IN
        SELECT format('documento %s (%s, %s) sostituito da %s', documento_id, tipo, nome_file, sostituito_da)
          FROM documento WHERE sostituito_da IS NOT NULL AND componente_id IS NULL LIMIT 20
    LOOP problemi := problemi || E'\n(c) ' || riga; END LOOP;

    -- (d) una catena che torna su se stessa, anche lunga (A→B→C→A) o di un passo (A→A).
    FOR riga IN
        WITH RECURSIVE catena (inizio, nodo, percorso) AS (
            SELECT documento_id, sostituito_da, ARRAY[documento_id] FROM documento WHERE sostituito_da IS NOT NULL
            UNION ALL
            SELECT c.inizio, d.sostituito_da, c.percorso || c.nodo
              FROM catena c JOIN documento d ON d.documento_id = c.nodo
             WHERE d.sostituito_da IS NOT NULL AND NOT c.nodo = ANY (c.percorso)
        )
        SELECT DISTINCT format('ciclo che passa da %s', inizio) FROM catena WHERE nodo = inizio LIMIT 20
    LOOP problemi := problemi || E'\n(d) ' || riga; END LOOP;

    -- (e) due documenti sostituiti dallo stesso: l'indice unico del passo 3 lo vieta, e quale dei due
    --     sia la revisione precedente e' una decisione.
    FOR riga IN
        SELECT format('%s documenti sostituiti da %s', count(*), sostituito_da)
          FROM documento WHERE sostituito_da IS NOT NULL GROUP BY sostituito_da HAVING count(*) > 1 LIMIT 20
    LOOP problemi := problemi || E'\n(e) ' || riga; END LOOP;

    IF problemi <> '' THEN
        RAISE EXCEPTION USING
            MESSAGE = '0020: dati da riconciliare prima della migrazione (nessuna modifica fatta):' || problemi,
            HINT = 'Ogni lettera e'' una guardia descritta nel file. Riconciliare a mano e rilanciare.';
    END IF;
END $$;

-- ============================================================ 2. TIPI
CREATE TYPE stato_bom    AS ENUM ('bozza', 'congelata');        -- «superata» non si scrive: si deriva (D28)
CREATE TYPE contesto_bom AS ENUM ('preventivo', 'tecnica');     -- riapre il prezzo / lascia la fase dov'e' (D25a, D25b, D25c)

-- ============================================================ 3. documento: LA CATENA DELLE REVISIONI
-- Una revisione nuova e' una riga nuova; la vecchia riceve sostituito_da e non si tocca piu'.
-- La FK semplice della 0001 (documento_sostituito_da_fkey) resta: e' ridondante con le due composite,
-- e toglierla non serve.
ALTER TABLE documento
    ADD CONSTRAINT ck_documento_non_sostituisce_se_stesso CHECK (sostituito_da IS NULL OR sostituito_da <> documento_id),
    -- si rivede un documento di un pezzo; e con componente_id NULL la FK composita qui sotto non
    -- verrebbe controllata (MATCH SIMPLE)
    ADD CONSTRAINT ck_documento_sostituito_ha_componente CHECK (sostituito_da IS NULL OR componente_id IS NOT NULL),
    -- Bersagli delle FK composite: ridondanti con la PK, esistono per le FK.
    ADD CONSTRAINT ux_documento_thread_id          UNIQUE (thread_id, documento_id),            -- stessa RFQ (catena, rimozione_proposta)
    ADD CONSTRAINT ux_documento_componente_id      UNIQUE (componente_id, documento_id),        -- STEP strutturale (A4.4)
    ADD CONSTRAINT ux_documento_componente_tipo_id UNIQUE (componente_id, tipo, documento_id),  -- stesso componente e stesso tipo (D32)
    ADD CONSTRAINT ux_documento_componente_id_sha  UNIQUE (componente_id, documento_id, sha256); -- baseline e deroga strutturale (A4.5, A4.6)

-- Stessa RFQ; stesso componente e stesso tipo (R1.5, D32): un 2D si sostituisce con un 2D, uno STEP con
-- un 3D. Finche' un documento sta in una catena il suo tipo e il suo componente non cambiano, perche'
-- la FK rifiuta la modifica da entrambi i lati.
ALTER TABLE documento
    ADD CONSTRAINT fk_documento_sostituito_stessa_rfq FOREIGN KEY (thread_id, sostituito_da)
        REFERENCES documento (thread_id, documento_id),
    ADD CONSTRAINT fk_documento_sostituito_stesso_tipo FOREIGN KEY (componente_id, tipo, sostituito_da)
        REFERENCES documento (componente_id, tipo, documento_id);

-- Due revisioni non sostituiscono la stessa.
CREATE UNIQUE INDEX ux_documento_sostituito_da ON documento (sostituito_da) WHERE sostituito_da IS NOT NULL;

-- Due documenti non dichiarano lo stesso file sul NAS: e' l'autorita' che rende impossibile una
-- collisione di nomi fra documenti (A4.1). lower() perche' la condivisione e' Windows. Con gli
-- spostamenti pendenti e gli orfani la mette in fila PercorsoOccupato, sotto il lucchetto della cartella.
CREATE UNIQUE INDEX ux_documento_percorso ON documento (thread_id, lower(path_relativo));

-- La catena cresce solo in fondo, quindi non fa cicli (R1.5). Tre mosse ammesse:
--   INSERT             sostituito_da NULL: un documento non nasce gia' sostituito
--   UPDATE NULL → Y    sostituzione: Y dev'essere corrente, letto FOR UPDATE
--   UPDATE Y → NULL    annullamento dell'ultima sostituzione: Y dev'essere ancora corrente
-- Y → W diretto si rifiuta: prima si annulla, poi si sostituisce. Il FOR UPDATE su Y mette in fila due
-- sostituzioni incrociate (X→Y e Y→X): una aspetta l'altra e trova il bersaglio non piu' corrente,
-- oppure il database ne interrompe una per stallo. Mai tutte e due.
CREATE FUNCTION documento_catena_revisioni() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    y                uuid;
    successore_di_y  uuid;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.sostituito_da IS NOT NULL THEN
            RAISE EXCEPTION USING ERRCODE = 'BOM03',
                MESSAGE = format('documento %s: un documento non nasce gia'' sostituito', NEW.documento_id);
        END IF;
        RETURN NEW;
    END IF;

    IF NEW.sostituito_da IS NOT DISTINCT FROM OLD.sostituito_da THEN
        RETURN NEW;
    END IF;
    IF OLD.sostituito_da IS NOT NULL AND NEW.sostituito_da IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'BOM03',
            MESSAGE = format('documento %s: e'' sostituito da %s e non si ripunta a %s; prima si annulla la sostituzione',
                             NEW.documento_id, OLD.sostituito_da, NEW.sostituito_da);
    END IF;

    y := COALESCE(NEW.sostituito_da, OLD.sostituito_da);
    SELECT d.sostituito_da INTO successore_di_y FROM documento d WHERE d.documento_id = y FOR UPDATE;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- Y non esiste: lo rifiuta la FK, con il suo errore
    END IF;
    IF successore_di_y IS NOT NULL THEN
        IF NEW.sostituito_da IS NOT NULL THEN
            RAISE EXCEPTION USING ERRCODE = 'BOM03',
                MESSAGE = format('documento %s: non puo'' essere sostituito da %s, che a sua volta e'' stato sostituito da %s',
                                 NEW.documento_id, y, successore_di_y);
        END IF;
        RAISE EXCEPTION USING ERRCODE = 'BOM03',
            MESSAGE = format('documento %s: la sostituzione con %s non si annulla, perche'' %s e'' stato a sua volta sostituito da %s',
                             NEW.documento_id, y, y, successore_di_y);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_documento_catena_revisioni BEFORE INSERT OR UPDATE OF sostituito_da ON documento
    FOR EACH ROW EXECUTE FUNCTION documento_catena_revisioni();

-- ============================================================ 4. documento: nome_file (R1.6)
-- Solo il commento: i dati non si riscrivono, perche' per i documenti gia' confermati i due significati
-- coincidono (la conferma scriveva NomeFileSicuro dell'originale, che e' l'ultima parte del percorso).
COMMENT ON COLUMN documento.nome_file IS
'Il nome ORIGINALE del file, com''e'' arrivato: si mostra e ordina, e non entra mai in un percorso. Il nome sul
NAS vive solo in path_relativo (per i tipi tecnici <CODICE>_REV_<REV>[_n].<ext>, D21). Fino a B8.A4a la
conferma ci scriveva il nome sanificato, che coincideva con l''ultima parte di path_relativo.';
COMMENT ON COLUMN documento.sostituito_da IS
'La revisione che ha preso il posto di questa. Una catena cresce solo in fondo (trigger
documento_catena_revisioni): stessa RFQ, stesso componente, stesso tipo, niente cicli.';

-- ============================================================ 5. componente: ARCHIVIAZIONE E STEP STRUTTURALE
-- Archiviare (A4.9): il componente esce dalla BOM working ma resta, con documenti, proposte, deroghe e
-- storia. Il codice resta occupato (ux_componente_thread_codice vale anche per gli archiviati): se lo
-- stesso codice torna, si ripristina.
--
-- STEP strutturale (A4.4, D31): il file che dice «questa e' la distinta di X». Lo sceglie Luigi, solo
-- per i prodotti finiti, e non segue da solo le sostituzioni. La FK composita lo lega al componente, e
-- quindi alla RFQ: finche' e' il riferimento, il documento non cambia componente.
ALTER TABLE componente
    ADD COLUMN archiviato_il        timestamptz,
    ADD COLUMN archiviato_da        uuid REFERENCES utente,
    ADD COLUMN motivo_archiviazione text,
    ADD COLUMN step_strutturale_id  uuid,
    ADD CONSTRAINT ck_componente_archiviazione CHECK (
        (archiviato_il IS NULL AND archiviato_da IS NULL AND motivo_archiviazione IS NULL)
     OR (archiviato_il IS NOT NULL AND archiviato_da IS NOT NULL AND btrim(motivo_archiviazione) <> '')),
    ADD CONSTRAINT ck_componente_step_solo_finito CHECK (step_strutturale_id IS NULL OR tipo = 'finito'),
    ADD CONSTRAINT fk_componente_step_strutturale FOREIGN KEY (componente_id, step_strutturale_id)
        REFERENCES documento (componente_id, documento_id);

-- Al momento della scelta il documento dev'essere uno STEP (cad_3d, .stp o .step) e corrente. Dopo, se
-- viene sostituito, il riferimento resta dov'e' e v_step_prodotto dice riferimento_superato. FOR SHARE
-- mette in fila la scelta con una sostituzione concorrente dello stesso documento.
CREATE FUNCTION componente_step_strutturale() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    d record;
BEGIN
    IF NEW.step_strutturale_id IS NULL
       OR (TG_OP = 'UPDATE' AND NEW.step_strutturale_id IS NOT DISTINCT FROM OLD.step_strutturale_id) THEN
        RETURN NEW;
    END IF;
    SELECT x.tipo, x.estensione, x.sostituito_da INTO d FROM documento x WHERE x.documento_id = NEW.step_strutturale_id FOR SHARE;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- lo rifiuta la FK, con il suo errore
    END IF;
    IF d.tipo <> 'cad_3d' OR lower(d.estensione) NOT IN ('stp', 'step') THEN
        RAISE EXCEPTION USING ERRCODE = 'BOM04',
            MESSAGE = format('componente %s: lo STEP strutturale dev''essere un file STEP (cad_3d, .stp o .step), non %s .%s',
                             NEW.componente_id, d.tipo, d.estensione);
    END IF;
    IF d.sostituito_da IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'BOM04',
            MESSAGE = format('componente %s: il documento %s e'' stato sostituito da %s; si sceglie un documento corrente',
                             NEW.componente_id, NEW.step_strutturale_id, d.sostituito_da);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_componente_step_strutturale BEFORE INSERT OR UPDATE OF step_strutturale_id ON componente
    FOR EACH ROW EXECUTE FUNCTION componente_step_strutturale();

COMMENT ON COLUMN componente.step_strutturale_id IS
'Lo STEP che e'' la distinta di questo prodotto finito, scelto da una persona (A4.4, D31). Solo da questo
file, letto per intero, nascono le proposte di rimozione. Non segue da solo le sostituzioni.';

-- ============================================================ 6. deroga_struttura (D33, D36)
-- «Congelo con QUESTO STEP letto in parte». Non e' una deroga del fabbisogno: quella vale per
-- (componente, tipo) e sopravvive a qualunque file arrivi dopo; questa vale per quel file e per quella
-- lettura. La validita' non si scrive: si deriva (v_step_prodotto, passo 12). La deroga non sopravvive a
-- una sostituzione dello STEP, a una versione nuova dell'analizzatore o della configurazione, a una
-- rianalisi della stessa chiave (UpsertAnalisiFatti riscrive calcolato_il) e alla prima analisi di uno
-- STEP derogato quando non era analizzato.
CREATE TABLE deroga_struttura (
    deroga_struttura_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id             uuid NOT NULL,
    componente_id         uuid NOT NULL,
    step_documento_id     uuid NOT NULL,
    step_sha256           char(64) NOT NULL,
    versione_analizzatore smallint,              -- l'analisi derogata; tutte e tre NULL = STEP non ancora analizzato
    hash_configurazione   char(64),
    analisi_calcolata_il  timestamptz,           -- UpsertAnalisiFatti riscrive i fatti con la stessa chiave: questa colonna distingue le due letture
    motivo_parziale       text NOT NULL,         -- che cosa mancava, com'era scritto quando si e' derogato
    motivo                text NOT NULL CHECK (btrim(motivo) <> ''),
    concessa_da           uuid NOT NULL REFERENCES utente,
    concessa_il           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ux_deroga_struttura_bersaglio UNIQUE (deroga_struttura_id, componente_id, step_documento_id),  -- bersaglio della FK della baseline
    CONSTRAINT fk_deroga_struttura_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT fk_deroga_struttura_step FOREIGN KEY (componente_id, step_documento_id, step_sha256)
        REFERENCES documento (componente_id, documento_id, sha256),
    CONSTRAINT fk_deroga_struttura_analisi FOREIGN KEY (step_sha256, versione_analizzatore, hash_configurazione)
        REFERENCES analisi_fatti,
    CONSTRAINT ck_deroga_struttura_analisi CHECK (
        (versione_analizzatore IS NULL) = (hash_configurazione IS NULL)
        AND (versione_analizzatore IS NULL) = (analisi_calcolata_il IS NULL))
);
CREATE INDEX ix_deroga_struttura_componente ON deroga_struttura (componente_id, step_documento_id);

-- Una deroga non si modifica: se ne concede un'altra. Si cancella solo se nessuna baseline la usa, e
-- lo impedisce la FK di bom_versione_componente.
CREATE FUNCTION deroga_struttura_immutabile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = 'BOM05',
        MESSAGE = format('deroga strutturale %s: non si modifica; se ne concede una nuova', OLD.deroga_struttura_id);
END $$;
CREATE TRIGGER trg_deroga_struttura_immutabile BEFORE UPDATE ON deroga_struttura
    FOR EACH ROW EXECUTE FUNCTION deroga_struttura_immutabile();

COMMENT ON TABLE deroga_struttura IS
'Deroga per congelare con uno STEP strutturale letto in parte o non analizzato (A4.5, D33, D36). Vale solo
finche'' quel documento e'' il riferimento corrente e la sua analisi corrente e'' quella derogata (stessa
versione, configurazione e calcolato_il; oppure ancora nessuna). Non si modifica.';

-- ============================================================ 7. rimozione_proposta (D27)
-- «L'arco padre → figlio della working non c'e' piu' nello STEP strutturale letto per intero». Non e'
-- un nodo del file, quindi non sta in relazione_proposta, le cui FK puntano ai nodi del file stesso.
-- La tabella ricorda anche uno «scarta»: senza, la proposta tornerebbe a ogni apertura. La scrive solo
-- un'analisi completa dello STEP strutturale; nessuna proposta tocca la BOM da sola.
CREATE TABLE rimozione_proposta (
    thread_id         uuid NOT NULL,
    step_documento_id uuid NOT NULL,                 -- lo STEP strutturale che non contiene piu' l'arco: l'autorita' della rimozione
    padre_id          uuid NOT NULL,
    figlio_id         uuid NOT NULL,
    qta_working       int NOT NULL CHECK (qta_working > 0),
    stato             stato_proposta NOT NULL DEFAULT 'aperta',
    nota              varchar(200),                  -- «superata da <file>», «cambiato lo STEP strutturale» (A4.4)
    deciso_da         uuid REFERENCES utente,
    deciso_il         timestamptz,
    creato_il         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, step_documento_id, padre_id, figlio_id),
    CHECK (padre_id <> figlio_id),
    CONSTRAINT fk_rimozione_proposta_step FOREIGN KEY (thread_id, step_documento_id)
        REFERENCES documento (thread_id, documento_id),
    CONSTRAINT fk_rimozione_proposta_padre FOREIGN KEY (thread_id, padre_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT fk_rimozione_proposta_figlio FOREIGN KEY (thread_id, figlio_id)
        REFERENCES componente (thread_id, componente_id)
);
CREATE INDEX ix_rimozione_proposta_aperte ON rimozione_proposta (thread_id) WHERE stato = 'aperta';

-- Uno STEP sostituito, o un riferimento cambiato, chiude le proposte ancora aperte che venivano da li'
-- con la nota «superata da <file>» (A4.4). relazione_proposta ha gia' la colonna (0018),
-- componente_proposta no.
ALTER TABLE componente_proposta ADD COLUMN nota varchar(200);

-- ============================================================ 8. VERSIONI DELLA BOM E ISTANTANEE
-- La riga di fase_log aperta quando una versione nasce (D37). Il vincolo contiene la PK, quindi sui
-- dati esistenti non puo' fallire: esiste come bersaglio della FK composita di bom_versione, che lega il
-- nome della fase alla riga.
ALTER TABLE fase_log ADD CONSTRAINT ux_fase_log_thread_riga_nome UNIQUE (thread_id, fase_log_id, nome_fase);

CREATE TABLE bom_versione (
    bom_versione_id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id                uuid NOT NULL REFERENCES thread_offerta,
    numero                   int  NOT NULL CHECK (numero > 0),
    stato                    stato_bom NOT NULL DEFAULT 'bozza',
    contesto                 contesto_bom NOT NULL,
    fase_all_apertura        fase NOT NULL,          -- la fase aperta quando la versione e' nata
    fase_log_id_all_apertura uuid NOT NULL,          -- e QUELLA riga di fase_log (D37)
    versione_precedente_id   uuid,
    numero_precedente        int,                    -- numero - 1 (R2.10)
    stato_precedente         stato_bom,              -- sempre 'congelata' (R2.10)
    motivo                   text NOT NULL CHECK (btrim(motivo) <> ''),
    creata_da                uuid NOT NULL REFERENCES utente,
    creata_il                timestamptz NOT NULL DEFAULT now(),
    congelata_da             uuid REFERENCES utente,
    congelata_il             timestamptz,
    CONSTRAINT ux_bom_versione_numero        UNIQUE (thread_id, numero),
    CONSTRAINT ux_bom_versione_thread_id     UNIQUE (thread_id, bom_versione_id),
    CONSTRAINT ux_bom_versione_catena        UNIQUE (thread_id, bom_versione_id, numero, stato),   -- bersaglio della catena
    CONSTRAINT fk_bom_versione_fase FOREIGN KEY (thread_id, fase_log_id_all_apertura, fase_all_apertura)
        REFERENCES fase_log (thread_id, fase_log_id, nome_fase),       -- il nome e la riga non possono divergere
    CONSTRAINT ck_bom_versione_congelata CHECK ((stato = 'congelata') = (congelata_da IS NOT NULL AND congelata_il IS NOT NULL)),
    CONSTRAINT ck_bom_versione_precedente CHECK (CASE WHEN numero = 1
            THEN versione_precedente_id IS NULL AND numero_precedente IS NULL AND stato_precedente IS NULL
            ELSE versione_precedente_id IS NOT NULL AND numero_precedente = numero - 1 AND stato_precedente = 'congelata' END),
    -- Il confine di D25a, D25b, D25c. Una V1 nasce solo in FATTIBILITA (preventivo) o in ORDINE e
    -- PRODUZIONE (tecnica, una RFQ arrivata li' senza aver mai congelato). In ACCETTATA e DISTINTA_ERP
    -- il contesto lo sceglie una persona.
    CONSTRAINT ck_bom_versione_confine CHECK (CASE
        WHEN numero = 1 THEN (contesto = 'preventivo' AND fase_all_apertura = 'FATTIBILITA')
                          OR (contesto = 'tecnica'    AND fase_all_apertura IN ('ORDINE', 'PRODUZIONE'))
        WHEN fase_all_apertura IN ('SCHEDA_COSTO', 'OFFERTE_FORN', 'OFFERTA_INVIATA') THEN contesto = 'preventivo'
        WHEN fase_all_apertura IN ('ACCETTATA', 'DISTINTA_ERP')                     THEN true
        WHEN fase_all_apertura IN ('ORDINE', 'PRODUZIONE')                          THEN contesto = 'tecnica'
        ELSE false END)
);
-- La catena (R2.10): Vn punta a V(n-1) della stessa RFQ, congelata al momento dell'INSERT. Con la
-- UNIQUE su (thread_id, numero) niente salti e niente biforcazioni. Il bersaglio puo' contenere lo
-- stato perche' una versione congelata non cambia piu' (trigger del passo 9), e mentre V(n-1) e' in
-- bozza nessuna versione la puo' referenziare. Aggiunta dopo la CREATE perche' punta alla tabella stessa.
ALTER TABLE bom_versione ADD CONSTRAINT fk_bom_versione_precedente
    FOREIGN KEY (thread_id, versione_precedente_id, numero_precedente, stato_precedente)
    REFERENCES bom_versione (thread_id, bom_versione_id, numero, stato);
-- Una bozza sola per RFQ; per la catena, e' sempre l'ultima versione.
CREATE UNIQUE INDEX ux_bom_una_bozza ON bom_versione (thread_id) WHERE stato = 'bozza';

COMMENT ON TABLE bom_versione IS
'Una versione della BOM di una RFQ (A4.6). Nasce bozza e diventa congelata una volta; congelata non si
modifica e non si cancella. «Superata» si deriva (v_bom_versioni). contesto: preventivo riapre il prezzo,
tecnica lascia la fase dov''e'' (D25a, D25b, D25c).';

-- Le istantanee: la BOM com'era al congelamento. Si scrivono mentre la versione e' bozza, e la versione
-- diventa congelata nella stessa transazione. Nessuna FK ha una cascata: cio' che sta in una baseline
-- non si cancella da sotto.
CREATE TABLE bom_versione_componente (      -- il componente com'era al congelamento
    bom_versione_id     uuid NOT NULL,
    thread_id           uuid NOT NULL,
    componente_id       uuid NOT NULL,
    codice              varchar(60) NOT NULL,
    rev                 varchar(10),
    tipo                tipo_componente NOT NULL,
    descrizione         varchar(200),
    qta                 int NOT NULL,
    esito_fattibilita   esito_fattibilita,
    note_fattibilita    text,
    step_strutturale_id uuid,                -- da quale file e' stata letta la distinta (A4.4)
    step_sha256         char(64),
    deroga_struttura_id uuid,                -- con quale deroga, se la lettura non era completa (D36)
    PRIMARY KEY (bom_versione_id, componente_id),
    CONSTRAINT fk_bvc_versione FOREIGN KEY (thread_id, bom_versione_id)
        REFERENCES bom_versione (thread_id, bom_versione_id),
    CONSTRAINT fk_bvc_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    -- lo STEP e' un documento di quel componente, con quello SHA (R2.5)
    CONSTRAINT fk_bvc_step FOREIGN KEY (componente_id, step_strutturale_id, step_sha256)
        REFERENCES documento (componente_id, documento_id, sha256),
    CONSTRAINT fk_bvc_deroga FOREIGN KEY (deroga_struttura_id, componente_id, step_strutturale_id)
        REFERENCES deroga_struttura (deroga_struttura_id, componente_id, step_documento_id),
    -- una FK composita non controlla niente appena una colonna e' NULL: i due CHECK chiudono il buco
    CONSTRAINT ck_bvc_step_con_sha CHECK ((step_strutturale_id IS NULL) = (step_sha256 IS NULL)),
    CONSTRAINT ck_bvc_deroga_con_step CHECK (deroga_struttura_id IS NULL OR step_strutturale_id IS NOT NULL)
);
CREATE INDEX ix_bvc_componente ON bom_versione_componente (thread_id, componente_id);
CREATE INDEX ix_bvc_step ON bom_versione_componente (componente_id, step_strutturale_id) WHERE step_strutturale_id IS NOT NULL;
CREATE INDEX ix_bvc_deroga ON bom_versione_componente (deroga_struttura_id) WHERE deroga_struttura_id IS NOT NULL;

CREATE TABLE bom_versione_relazione (
    bom_versione_id uuid NOT NULL,
    padre_id        uuid NOT NULL,
    figlio_id       uuid NOT NULL,
    qta             int NOT NULL CHECK (qta > 0),
    posizione       varchar(20),
    PRIMARY KEY (bom_versione_id, padre_id, figlio_id),
    CHECK (padre_id <> figlio_id),
    CONSTRAINT fk_bvr_padre  FOREIGN KEY (bom_versione_id, padre_id)
        REFERENCES bom_versione_componente (bom_versione_id, componente_id),
    CONSTRAINT fk_bvr_figlio FOREIGN KEY (bom_versione_id, figlio_id)
        REFERENCES bom_versione_componente (bom_versione_id, componente_id)
);

-- I documenti correnti approvati. La FK su documento_id e' semplice: con quella composita nessun
-- documento di una baseline potrebbe piu' cambiare componente, nemmeno per correggere un'assegnazione
-- sbagliata durante una revisione (A4.6).
CREATE TABLE bom_versione_documento (
    bom_versione_id      uuid NOT NULL,
    componente_id        uuid NOT NULL,
    documento_id         uuid NOT NULL REFERENCES documento,
    tipo                 tipo_documento NOT NULL,
    rev                  varchar(10),
    sha256               char(64) NOT NULL,
    path_al_congelamento varchar(500) NOT NULL,   -- STORICO (R1.8): non apre il file, non tiene occupata una cartella
    PRIMARY KEY (bom_versione_id, documento_id),
    CONSTRAINT fk_bvd_componente FOREIGN KEY (bom_versione_id, componente_id)
        REFERENCES bom_versione_componente (bom_versione_id, componente_id)
);
CREATE INDEX ix_bvd_documento ON bom_versione_documento (documento_id);

-- Le deroga_fabbisogno in vigore, com'erano (R2.9). deroga_id e' storico come path_al_congelamento:
-- senza FK, cosi' una revisione puo' togliere la deroga viva quando il documento arriva, e la baseline
-- vecchia resta com'era.
CREATE TABLE bom_versione_deroga (
    bom_versione_id uuid NOT NULL,
    componente_id   uuid NOT NULL,
    deroga_id       uuid NOT NULL,
    tipo            tipo_documento NOT NULL,
    motivo          text NOT NULL,
    utente_id       uuid NOT NULL,
    creata_il       timestamptz NOT NULL,
    PRIMARY KEY (bom_versione_id, componente_id, tipo),
    CONSTRAINT fk_bvg_componente FOREIGN KEY (bom_versione_id, componente_id)
        REFERENCES bom_versione_componente (bom_versione_id, componente_id)
);

-- La fase SCHEDA_COSTO sa su quale baseline e' nata (A4.8): l'ancora di tutto cio' che sta a valle.
ALTER TABLE fase_log
    ADD COLUMN bom_versione_id uuid,
    ADD CONSTRAINT fk_fase_log_bom_versione FOREIGN KEY (thread_id, bom_versione_id)
        REFERENCES bom_versione (thread_id, bom_versione_id),
    ADD CONSTRAINT ck_fase_log_bom_solo_scheda CHECK (bom_versione_id IS NULL OR nome_fase = 'SCHEDA_COSTO');

-- ============================================================ 9. IMMUTABILITA' NEL DATABASE
-- bom_versione: una versione nasce bozza; una riga congelata non si modifica e non si cancella. Una
-- bozza si cancella (abbandono) oppure si congela una volta, e il congelamento cambia solo stato,
-- congelata_da e congelata_il. Il confronto e' sulla riga intera, cosi' vale anche per una colonna
-- aggiunta domani. Nascere gia' congelata vorrebbe dire una baseline senza istantanee: le istantanee
-- si scrivono solo mentre la versione e' bozza.
CREATE FUNCTION bom_versione_immutabile() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    attesa bom_versione;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.stato <> 'bozza' THEN
            RAISE EXCEPTION USING ERRCODE = 'BOM02',
                MESSAGE = format('la V%s della RFQ %s: una versione nasce bozza, e si congela dopo aver scritto le istantanee',
                                 NEW.numero, NEW.thread_id);
        END IF;
        RETURN NEW;
    END IF;
    IF OLD.stato = 'congelata' THEN
        RAISE EXCEPTION USING ERRCODE = 'BOM02',
            MESSAGE = format('la V%s della RFQ %s e'' congelata: non si modifica e non si cancella', OLD.numero, OLD.thread_id);
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    attesa := OLD;
    attesa.stato        := NEW.stato;
    attesa.congelata_da := NEW.congelata_da;
    attesa.congelata_il := NEW.congelata_il;
    IF NEW.stato <> 'congelata' OR NEW IS DISTINCT FROM attesa THEN
        RAISE EXCEPTION USING ERRCODE = 'BOM02',
            MESSAGE = format('la V%s della RFQ %s e'' in bozza: si puo'' solo congelare (stato, congelata_da, congelata_il)',
                             OLD.numero, OLD.thread_id);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_bom_versione_immutabile BEFORE INSERT OR UPDATE OR DELETE ON bom_versione
    FOR EACH ROW EXECUTE FUNCTION bom_versione_immutabile();

-- Le quattro istantanee: niente INSERT, UPDATE o DELETE se la versione di appartenenza e' congelata.
-- FOR SHARE sulla versione mette in fila la scrittura con il congelamento: chi arriva dopo il commit
-- del congelamento rilegge lo stato e trova congelata.
CREATE FUNCTION bom_istantanea_immutabile() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    versioni uuid[] := '{}';
    v        record;
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN versioni := versioni || OLD.bom_versione_id; END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN versioni := versioni || NEW.bom_versione_id; END IF;
    FOR v IN
        SELECT b.numero, b.thread_id, b.stato FROM bom_versione b
         WHERE b.bom_versione_id = ANY (versioni) ORDER BY b.bom_versione_id FOR SHARE
    LOOP
        IF v.stato = 'congelata' THEN
            RAISE EXCEPTION USING ERRCODE = 'BOM02',
                MESSAGE = format('%s: la V%s della RFQ %s e'' congelata e la sua istantanea non cambia', TG_TABLE_NAME, v.numero, v.thread_id);
        END IF;
    END LOOP;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END $$;
CREATE FUNCTION bom_istantanea_non_si_svuota() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = 'BOM02', MESSAGE = format('%s: le istantanee delle BOM non si svuotano', TG_TABLE_NAME);
END $$;

CREATE TRIGGER trg_bvc_immutabile BEFORE INSERT OR UPDATE OR DELETE ON bom_versione_componente
    FOR EACH ROW EXECUTE FUNCTION bom_istantanea_immutabile();
CREATE TRIGGER trg_bvr_immutabile BEFORE INSERT OR UPDATE OR DELETE ON bom_versione_relazione
    FOR EACH ROW EXECUTE FUNCTION bom_istantanea_immutabile();
CREATE TRIGGER trg_bvd_immutabile BEFORE INSERT OR UPDATE OR DELETE ON bom_versione_documento
    FOR EACH ROW EXECUTE FUNCTION bom_istantanea_immutabile();
CREATE TRIGGER trg_bvg_immutabile BEFORE INSERT OR UPDATE OR DELETE ON bom_versione_deroga
    FOR EACH ROW EXECUTE FUNCTION bom_istantanea_immutabile();
CREATE TRIGGER trg_bvc_non_si_svuota BEFORE TRUNCATE ON bom_versione_componente
    FOR EACH STATEMENT EXECUTE FUNCTION bom_istantanea_non_si_svuota();
CREATE TRIGGER trg_bvr_non_si_svuota BEFORE TRUNCATE ON bom_versione_relazione
    FOR EACH STATEMENT EXECUTE FUNCTION bom_istantanea_non_si_svuota();
CREATE TRIGGER trg_bvd_non_si_svuota BEFORE TRUNCATE ON bom_versione_documento
    FOR EACH STATEMENT EXECUTE FUNCTION bom_istantanea_non_si_svuota();
CREATE TRIGGER trg_bvg_non_si_svuota BEFORE TRUNCATE ON bom_versione_deroga
    FOR EACH STATEMENT EXECUTE FUNCTION bom_istantanea_non_si_svuota();

-- ============================================================ 10. LA WORKING BLOCCATA DOPO IL CONGELAMENTO (D26)
-- «Dopo il freeze si lavora solo aprendo una revisione», tenuto dal database. Se l'ultima versione
-- della RFQ e' congelata (e quindi non c'e' una bozza), si rifiutano:
--   - INSERT, UPDATE e DELETE su componente (compreso step_strutturale_id), componente_relazione,
--     deroga_fabbisogno e deroga_struttura;
--   - su documento, quando la riga vecchia o la nuova ha un componente: l'INSERT, il DELETE e l'UPDATE
--     dei campi tecnici (componente_id, tipo, codice, rev, estensione, sha256, sostituito_da, thread_id).
-- Restano liberi lo stato del NAS (path_relativo, stato_nas, errore_nas, scritto_il, verificato_il) e
-- nome_file e nota: una copia o uno spostamento accodati prima del congelamento devono poter finire.
-- La conferma e l'assegnazione lo dicono prima di arrivare qui; questo e' il muro, non il messaggio.
--
-- bom_working_bloccata prende la riga del thread FOR KEY SHARE prima di leggere le versioni.
-- congelaBom e l'apertura di una revisione la prendono FOR UPDATE, che con FOR KEY SHARE confligge:
-- senza, una modifica che ha visto la bozza potrebbe arrivare dopo l'istantanea. FOR KEY SHARE non
-- blocca gli aggiornamenti ordinari del thread (ultimo_aggiornamento, stato).
CREATE FUNCTION bom_working_bloccata(p_thread uuid) RETURNS int LANGUAGE plpgsql AS $$
DECLARE
    v record;
BEGIN
    PERFORM 1 FROM thread_offerta t WHERE t.thread_id = p_thread FOR KEY SHARE;
    SELECT b.numero, b.stato INTO v FROM bom_versione b WHERE b.thread_id = p_thread ORDER BY b.numero DESC LIMIT 1;
    IF FOUND AND v.stato = 'congelata' THEN
        RETURN v.numero;                               -- la versione che blocca la working
    END IF;
    RETURN NULL;
END $$;

CREATE FUNCTION bom_working_modificabile() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    t uuid;
    n int;
BEGIN
    IF TG_TABLE_NAME = 'documento' THEN
        IF TG_OP = 'INSERT' AND NEW.componente_id IS NULL THEN RETURN NEW; END IF;
        IF TG_OP = 'DELETE' AND OLD.componente_id IS NULL THEN RETURN OLD; END IF;
        IF TG_OP = 'UPDATE' THEN
            IF OLD.componente_id IS NULL AND NEW.componente_id IS NULL THEN RETURN NEW; END IF;
            IF (NEW.componente_id, NEW.tipo, NEW.codice, NEW.rev, NEW.estensione, NEW.sha256, NEW.sostituito_da, NEW.thread_id)
               IS NOT DISTINCT FROM
               (OLD.componente_id, OLD.tipo, OLD.codice, OLD.rev, OLD.estensione, OLD.sha256, OLD.sostituito_da, OLD.thread_id) THEN
                RETURN NEW;
            END IF;
        END IF;
    ELSIF TG_OP = 'UPDATE' AND NEW IS NOT DISTINCT FROM OLD THEN
        RETURN NEW;                                    -- un UPDATE che non cambia niente non cambia la BOM
    END IF;

    IF TG_OP = 'DELETE' THEN t := OLD.thread_id; ELSE t := NEW.thread_id; END IF;
    n := bom_working_bloccata(t);
    IF n IS NULL AND TG_OP = 'UPDATE' AND OLD.thread_id IS DISTINCT FROM NEW.thread_id THEN
        t := OLD.thread_id;
        n := bom_working_bloccata(t);
    END IF;
    IF n IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'BOM01',
            MESSAGE = format('la BOM della RFQ %s e'' congelata nella V%s: %s su %s solo aprendo una revisione',
                             t, n, TG_OP, TG_TABLE_NAME),
            HINT = 'addendum A4.7, D26';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER trg_bom_working_componente BEFORE INSERT OR UPDATE OR DELETE ON componente
    FOR EACH ROW EXECUTE FUNCTION bom_working_modificabile();
CREATE TRIGGER trg_bom_working_relazione BEFORE INSERT OR UPDATE OR DELETE ON componente_relazione
    FOR EACH ROW EXECUTE FUNCTION bom_working_modificabile();
CREATE TRIGGER trg_bom_working_deroga BEFORE INSERT OR UPDATE OR DELETE ON deroga_fabbisogno
    FOR EACH ROW EXECUTE FUNCTION bom_working_modificabile();
CREATE TRIGGER trg_bom_working_deroga_struttura BEFORE INSERT OR UPDATE OR DELETE ON deroga_struttura
    FOR EACH ROW EXECUTE FUNCTION bom_working_modificabile();
-- UPDATE OF: il trigger non scatta nemmeno per gli aggiornamenti dello stato del NAS, che sono i piu'
-- frequenti; dentro, confronta i valori, perche' una colonna nella SET non vuol dire che sia cambiata.
CREATE TRIGGER trg_bom_working_documento
    BEFORE INSERT OR DELETE OR UPDATE OF componente_id, tipo, codice, rev, estensione, sha256, sostituito_da, thread_id
    ON documento FOR EACH ROW EXECUTE FUNCTION bom_working_modificabile();

-- ============================================================ 11. struttura_completa (A4.4, D35) E ANALIZZATORE CORRENTE
-- Quando una lettura dello STEP e' completa, cioe' quando la mancanza di un arco e' un'informazione. La
-- definizione e' una sola: la usano la vista, il gate e le proposte, e il Go la interroga invece di
-- ricalcolarla. L'argomento e' fatti -> 'struttura' di analisi_fatti.
--
-- struttura_motivo_parziale dice il PRIMO motivo per cui non e' completa (NULL = completa), nell'ordine:
-- nessuna struttura; lettura fallita; forma diversa dalla v3 (gli scarti non sono numerati: le analisi
-- di oggi sono v2 e vanno rianalizzate); lettura troncata; scarti che possono nascondere un arco o un
-- codice (le occorrenze di un pezzo dentro se stesso no: sono archi impossibili); piu' o meno di una
-- radice; nessuna occorrenza di assieme.
-- Solo concatenazioni di testo (niente format(), che e' STABLE): la funzione e' davvero IMMUTABLE.
-- I CASE WHEN si valutano in ordine, e jsonb_array_length arriva solo dopo aver visto un array.
CREATE FUNCTION struttura_motivo_parziale(s jsonb) RETURNS text LANGUAGE sql IMMUTABLE AS $$
SELECT CASE
    WHEN s IS NULL OR jsonb_typeof(s) <> 'object' THEN
        'nessuna struttura nei fatti'
    WHEN EXISTS (SELECT 1
                   FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(s -> 'avvisi') = 'array' THEN s -> 'avvisi' ELSE '[]'::jsonb END) AS a (testo)
                  WHERE a.testo LIKE 'struttura non letta%' OR a.testo = 'non e'' un file STEP Part 21') THEN
        'struttura non letta'
    WHEN (s -> 'versione') IS DISTINCT FROM '3'::jsonb THEN
        'struttura v' || COALESCE(s ->> 'versione', '?') || ': gli scarti non sono numerati, serve la rianalisi v3'
    WHEN (s -> 'limiti' -> 'troncato') IS DISTINCT FROM 'false'::jsonb THEN
        'lettura troncata: ' || COALESCE(nullif(s -> 'limiti' ->> 'motivo', ''), 'motivo non indicato')
    WHEN jsonb_typeof(s -> 'scarti') IS DISTINCT FROM 'object' THEN
        'scarti non indicati'
    WHEN (s -> 'scarti' -> 'prodotti_senza_definizione') IS DISTINCT FROM '0'::jsonb THEN
        COALESCE(s -> 'scarti' ->> 'prodotti_senza_definizione', '?') || ' PRODUCT senza PRODUCT_DEFINITION'
    WHEN (s -> 'scarti' -> 'occorrenze_non_risolte') IS DISTINCT FROM '0'::jsonb THEN
        COALESCE(s -> 'scarti' ->> 'occorrenze_non_risolte', '?') || ' occorrenze con estremi non risolti'
    WHEN (s -> 'scarti' -> 'testi_troncati') IS DISTINCT FROM '0'::jsonb THEN
        COALESCE(s -> 'scarti' ->> 'testi_troncati', '?') || ' testi troncati'
    WHEN jsonb_typeof(s -> 'radici') IS DISTINCT FROM 'array' THEN
        'radici non indicate'
    WHEN jsonb_array_length(s -> 'radici') <> 1 THEN
        jsonb_array_length(s -> 'radici')::text || ' radici nel file: una distinta ne ha una'
    WHEN jsonb_typeof(s -> 'relazioni') IS DISTINCT FROM 'array' THEN
        'relazioni non indicate'
    WHEN jsonb_array_length(s -> 'relazioni') = 0 THEN
        'nessuna occorrenza di assieme: solo parti'
    END
$$;

CREATE FUNCTION struttura_completa(s jsonb) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
SELECT struttura_motivo_parziale(s) IS NULL
$$;

-- L'analizzatore corrente (A4.5). «L'analisi corrente» di uno STEP e' quella del suo SHA con la
-- versione dell'analizzatore e la configurazione che il server usa adesso. Le conosce il server
-- (cfg.Analisi: la versione, e l'hash dei parametri mandati al worker) e le scrive qui a ogni avvio;
-- v_step_prodotto e il gate le leggono da qui. Una riga sola. Vuota = nessuna analisi e' corrente:
-- e' il lato che non concede.
CREATE TABLE analizzatore_corrente (
    unico                 boolean PRIMARY KEY DEFAULT true CHECK (unico),
    versione_analizzatore smallint NOT NULL,
    hash_configurazione   char(64) NOT NULL,
    impostato_il          timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE analizzatore_corrente IS
'La chiave dell''analisi corrente (versione dell''analizzatore, hash della configurazione), scritta dal
server a ogni avvio da cfg.Analisi. Una riga sola; vuota = nessuna analisi corrente (A4.5).';

-- ============================================================ 12. VISTE
-- v_fascicolo e v_componente_albero: un componente archiviato esce dalla BOM working (A4.9). Stesse
-- colonne nello stesso ordine, quindi CREATE OR REPLACE senza toccare v_thread_bloccanti e v_cruscotto.
-- Il testo e' quello della 0018 con il solo filtro in piu'.
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
        AND upper(btrim(d.rev)) <> upper(btrim(co.rev))) AS rev_diversa,
       an.anomalia_id
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
    SELECT d.documento_id, d.stato_nas, d.path_relativo, d.rev FROM documento d
    WHERE d.componente_id = co.componente_id AND d.tipo = f.tipo AND d.sostituito_da IS NULL
    ORDER BY d.confermato_il DESC LIMIT 1) d ON true
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

-- Gli archi di un componente archiviato si tolgono con l'archiviazione (A4.9). La vista non dipende
-- dall'averlo fatto: un archiviato non entra da nessuna parte, e una radice e' un componente senza
-- padri ATTIVI, cosi' un arco rimasto verso un padre archiviato non fa sparire il figlio.
CREATE OR REPLACE VIEW v_componente_albero AS
WITH RECURSIVE albero (thread_id, componente_id, radice_id, padre_id, profondita, percorso, qta_cumulata) AS (
    SELECT c.thread_id, c.componente_id, c.componente_id, NULL::uuid, 0, ARRAY[c.componente_id], 1::bigint
      FROM componente c
     WHERE c.archiviato_il IS NULL
       AND NOT EXISTS (SELECT 1 FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id
                        WHERE r.figlio_id = c.componente_id AND p.archiviato_il IS NULL)
    UNION ALL
    SELECT a.thread_id, r.figlio_id, a.radice_id, r.padre_id, a.profondita + 1, a.percorso || r.figlio_id,
           a.qta_cumulata * r.qta
      FROM albero a
      JOIN componente_relazione r ON r.padre_id = a.componente_id
      JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
     WHERE NOT r.figlio_id = ANY (a.percorso)
)
SELECT thread_id, componente_id, radice_id, padre_id, profondita, percorso, qta_cumulata FROM albero;

-- v_documento_storia: ogni documento con la sua catena. catena_id e' il primo documento della catena,
-- passo la sua posizione (1 = il piu' vecchio). ListStoriaDocumento legge le righe con il catena_id del
-- documento chiesto. La guardia sul percorso protegge la vista da un ciclo entrato per un bug futuro.
CREATE VIEW v_documento_storia AS
WITH RECURSIVE catena (catena_id, documento_id, passo, percorso) AS (
    SELECT d.documento_id, d.documento_id, 1, ARRAY[d.documento_id]
      FROM documento d
     WHERE NOT EXISTS (SELECT 1 FROM documento p WHERE p.sostituito_da = d.documento_id)
    UNION ALL
    SELECT c.catena_id, d.sostituito_da, c.passo + 1, c.percorso || d.sostituito_da
      FROM catena c
      JOIN documento d ON d.documento_id = c.documento_id
     WHERE d.sostituito_da IS NOT NULL AND NOT d.sostituito_da = ANY (c.percorso)
)
SELECT d.thread_id, d.componente_id, c.catena_id, c.passo, d.documento_id, d.tipo, d.codice, d.rev,
       d.nome_file, d.estensione, d.sha256, d.path_relativo, d.stato_nas, d.confermato_da, d.confermato_il,
       d.sostituito_da, (d.sostituito_da IS NULL) AS corrente
  FROM catena c
  JOIN documento d ON d.documento_id = c.documento_id;

-- v_step_prodotto: lo STEP di ogni prodotto finito attivo (A4.5), piu' una riga di avviso per ogni
-- radice attiva che non e' un finito (radice come in v_componente_albero: nessun padre attivo).
-- «L'analisi corrente» e' quella con la chiave di analizzatore_corrente (passo 11); se la tabella e'
-- vuota nessuna analisi e' corrente, e uno STEP fissato risulta presente_non_analizzato: il gate vuole
-- allora la deroga strutturale.
--
-- esito, nell'ordine in cui si decide:
--   radice_senza_qualifica   una radice che non e' un finito: avviso, non blocca
--   riferimento_superato     lo STEP strutturale e' stato sostituito e nessuno ha scelto il nuovo
--   presente_non_analizzato  riferimento corrente, nessuna analisi corrente
--   presente_analizzato      riferimento corrente, analisi corrente completa
--   presente_parziale        riferimento corrente, analisi corrente incompleta (motivo_parziale)
--   da_scegliere             ci sono STEP correnti, nessuno e' il riferimento
--   solo_altro_3d            un 3D c'e', ma non STEP
--   da_confermare            una proposta aperta di 3D per il componente
--   sul_portale              un 3D atteso dal portale
--   mancante                 altrimenti
-- deroga_struttura_id e' la deroga strutturale VALIDA (D36): stesso riferimento corrente, stessa
-- analisi corrente con lo stesso calcolato_il, oppure ancora nessuna analisi se era stata data cosi'.
CREATE VIEW v_step_prodotto AS
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

-- v_bom_versioni: «superata» e «corrente» si derivano (D28). Corrente e' l'ultima congelata; una bozza
-- non e' ne' l'una ne' l'altra.
CREATE VIEW v_bom_versioni AS
SELECT b.thread_id, b.bom_versione_id, b.numero, b.stato, b.contesto, b.fase_all_apertura, b.fase_log_id_all_apertura,
       b.versione_precedente_id, b.motivo, b.creata_da, b.creata_il, b.congelata_da, b.congelata_il,
       (b.stato = 'congelata' AND x.dopo)     AS superata,
       (b.stato = 'congelata' AND NOT x.dopo) AS corrente
  FROM bom_versione b
  CROSS JOIN LATERAL (
        SELECT EXISTS (SELECT 1 FROM bom_versione s
                        WHERE s.thread_id = b.thread_id AND s.numero > b.numero AND s.stato = 'congelata') AS dopo) x;

-- v_thread_da_riesaminare (A4.8): che cosa va riguardato, derivato e mai scritto. Una riga per motivo.
--   la Scheda Costo poggia su una baseline che non e' piu' l'ultima congelata: dopo una revisione
--     tecnica, con il motivo che dipende dalla fase in cui e' nata (dopo l'ordine / prima dell'ordine);
--   evidenze tecniche nuove dopo l'ultima baseline, se non c'e' una bozza aperta (R2.11): documenti
--     tecnici confermati e proposte di documento tecnico aperte (n_file), proposte strutturali aperte
--     (n_proposte). Sparisce con le proposte decise, con una revisione aperta o con il congelamento dopo.
-- Le tabelle future a valle (ERP, ciclo, produzione) avranno la loro vista con lo stesso predicato.
CREATE VIEW v_thread_da_riesaminare AS
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

-- ============================================================ 13. TRANSIZIONI (A4.7, D25a, D25c, D37)
-- Verso FATTIBILITA: l'apertura di una revisione preventivo. Di ritorno: l'abbandono di una bozza
-- preventivo, che riporta la RFQ alla fase di apertura (anche passando per ATTESA_DISEGNI).
-- FATTIBILITA → SCHEDA_COSTO c'e' gia' (0001) e vale anche per il congelamento e per l'abbandono.
-- Oggi nessun codice legge transizione per decidere (ListTransizioniDa non ha chiamanti).
INSERT INTO transizione (da, a, automatica, fatto_richiesto) VALUES
 ('SCHEDA_COSTO',    'FATTIBILITA',     false, 'revisione preventivo della BOM (A4.7)'),
 ('OFFERTE_FORN',    'FATTIBILITA',     false, 'revisione preventivo della BOM (A4.7)'),
 ('OFFERTA_INVIATA', 'FATTIBILITA',     false, 'revisione preventivo della BOM (A4.7)'),
 ('ACCETTATA',       'FATTIBILITA',     false, 'revisione preventivo della BOM, scelta da chi la apre (D25c)'),
 ('DISTINTA_ERP',    'FATTIBILITA',     false, 'revisione preventivo della BOM, scelta da chi la apre (D25c)'),
 ('FATTIBILITA',     'OFFERTE_FORN',    false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('FATTIBILITA',     'OFFERTA_INVIATA', false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('FATTIBILITA',     'ACCETTATA',       false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('FATTIBILITA',     'DISTINTA_ERP',    false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('ATTESA_DISEGNI',  'SCHEDA_COSTO',    false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('ATTESA_DISEGNI',  'OFFERTE_FORN',    false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('ATTESA_DISEGNI',  'OFFERTA_INVIATA', false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('ATTESA_DISEGNI',  'ACCETTATA',       false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)'),
 ('ATTESA_DISEGNI',  'DISTINTA_ERP',    false, 'revisione della BOM abbandonata: ritorno alla fase di apertura (D37)')
ON CONFLICT (da, a) DO NOTHING;

-- ============================================================ 14. VERSIONE
INSERT INTO schema_versione (versione) VALUES (20);
