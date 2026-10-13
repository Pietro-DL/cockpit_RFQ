-- 0022_p4_conferme_working.sql — Giro 5, A1d/P4: la bozza della distinta salvata sul server, le conferme
-- strutturali della working, l'esito degli intenti, la storia di fonte STEP, associazioni, correzioni,
-- attestazioni del fascicolo e deroghe.
--
-- Che cosa fa, in una riga: aggiunge i quattro livelli della distinta che mancavano allo schema (la
-- proposta STEP c'e' gia'; qui la bozza di lavoro e la conferma strutturale; bom_versione resta com'e') e
-- i registri che li rendono ricostruibili. E' l'UNICO file di schema di P4 (blocchi B1..B5): non ci sara'
-- un'altra migrazione P4.
--
--   working_contatore            per RFQ, due contatori della working (struttura, dati) mossi da trigger su
--                                componente e componente_relazione: li vedono TUTTI gli scrittori, legacy
--                                compresi, e un UPDATE che non cambia niente non conta
--   distinta_bozza               la bozza di lavoro di un prodotto (aperta / confermata / abbandonata), con
--                                versione ottimistica, base e ancoraggio alla fonte (allegato, sha256, gruppo)
--   distinta_bozza_versione      lo snapshot formato 2 COMPLETO di ogni versione salvata: non si riscrive
--   distinta_bozza_evento        il giornale delle azioni (azione, prima, dopo, origine, autore, data): solo aggiunte
--   conferma_strutturale         la conferma ufficiale, progressiva e immutabile per prodotto (formato 1 o 2)
--   conferma_strutturale_componente, conferma_strutturale_arco   che cosa la conferma certifica: componenti e
--                                archi (con qta) della working al momento del COMMIT
--   esito_intento                l'esito della transazione per chiave di idempotenza (risposta persa → si recupera)
--   f8_stato                     lo stato del rismistamento F8, che gira DOPO il COMMIT, fuori dalla transazione
--   correzione_documento         audit delle correzioni di codice/rev di proposte e documenti
--   associazione_evento          chi e quando di assegna / sgancia / scarta / preassociazione / caricamento
--   distinta_bozza_registro      il ciclo di ogni bozza (aperta, confermata, abbandonata, cancellata), scritto
--                                solo dal trigger di distinta_bozza: resta dopo la conservazione delle bozze e
--                                tiene il prodotto (D-P4A-01, P-04a)
--   fonte_step_evento            storia della fonte STEP di un prodotto: scelta, rifiuto, sostituzione, ...;
--                                con ruolo prodotto | sottoassieme e la destinazione della fonte secondaria
--                                (componente_id, o bozza + chiave finche' il nodo non ha un UUID) (D-SHOCK-S2)
--   attestazione_fascicolo(_documento)   attestazione esplicita del fascicolo, sulla conferma ultima
--   deroga_fabbisogno_storia     copia append-only di ogni scrittura su deroga_fabbisogno (trigger)
--   deroga_struttura_storia      copia append-only di ogni concessione e revoca di deroga_struttura (trigger): con
--                                D-SHOCK-S1 B la deroga strutturale regge la verifica positiva, e la sua revoca
--                                (un DELETE) non deve cancellarne la storia
--   copertura_documento          le pagine di un documento ufficiale che soddisfano il disegno_2d di un altro
--                                componente della stessa RFQ, in sola aggiunta (D-SHOCK-DOC-01)
--   v_fascicolo                  RIDEFINITA (CREATE OR REPLACE VIEW): stesse 19 colonne della 0020, nello
--                                stesso ordine e con lo stesso significato, piu' copertura_id in coda; un
--                                disegno_2d coperto da una copertura valida conta come presente (D-SHOCK-DOC-02)
--   schema_variante              quale testo della 0022 ha creato lo schema: il migratore rifiuta un database
--                                che ha la 22 registrata con un'altra variante (o senza variante)
--
-- Riferimenti: ratifica integrativa 2026-10-12-05 (§1 una 0022 additiva, §2 i quattro livelli e il modello
-- snapshot + giornale, §3 D-P4B0-01/04/05/06, §4 fonte STEP sostituibile), direttiva operativa 2026-10-12-06
-- (B1: elenco minimo delle tabelle), matrice 2026-10-12-07, contratto backend A1d v2 rev. 2 (§2, §4, §5, §7,
-- §8, §9), piano P4-B0 rev. 2 §3.2 (DDL di massima) e §3.5 (conservazione); direttiva 2026-10-12-11 (le dodici
-- risposte dello shock test, §2 modifiche obbligatorie alla 0022) e contratto rev. 3, Emendamento 2 (E2.1-E2.12).
-- Le differenze dal §3.2 del piano e le scelte tecniche dell'emendamento sono motivate nella nota
-- 01-analisi/2026-10-12-P4-B1-ddl-0022-nota.md (privata, §11 e §12).
--
-- Regole del migratore: un file = una transazione; ogni REFERENCES punta a una tabella gia' creata (in questo
-- file o prima); nessun valore di enum nuovo (text + CHECK; si riusano solo i tipi gia' esistenti
-- tipo_componente e tipo_documento); il file termina con INSERT INTO schema_versione.
--
-- Additiva per le tabelle: nessuna colonna esistente cambia, nessuna riga esistente si riscrive, nessun indice
-- o vincolo esistente si tocca (la PK (padre_id, figlio_id) di componente_relazione resta: ARCO_CONDIVISO_QTA
-- e' un blocco del contratto, non dello schema), bom_versione resta com'e'. Su componente,
-- componente_relazione, deroga_fabbisogno e deroga_struttura nascono solo trigger AFTER nuovi.
--
-- DEROGA DICHIARATA (D-SHOCK-DOC-02 A [U], direttiva 11): la vista v_fascicolo e' ridefinita qui con CREATE OR
-- REPLACE VIEW, come la 0020 aveva fatto con quella della 0018. I file 0001..0021 (la 0020 compresa) restano
-- byte-identici; cambia solo l'oggetto vista nel database. Le 19 colonne di prima restano nello stesso ordine,
-- con lo stesso tipo e lo stesso significato per ogni riga senza copertura; copertura_id e' in coda.
-- v_thread_bloccanti e v_cruscotto non si toccano: leggono la vista per nome, quindi contano con la stessa
-- definizione (una sola definizione della completezza per Fascicolo, gate del congelamento e cruscotto).
--
-- Nessuna guardia sui dati: la migrazione non sposta e non riscrive righe (la vista ridefinita legge in piu' le
-- coperture, che nascono vuote: senza coperture ogni riga della vista e' quella di prima). Nessun riempimento iniziale:
-- working_contatore nasce vuota e una riga mancante vale (struttura 0, dati 0); le altre tabelle nascono
-- vuote (nessuna conferma inventata per le RFQ esistenti: la prima nasce da una conferma f.1 o f.2).
--
-- Errori dei trigger. Ognuno ha un codice suo, cosi' il Go lo riconosce senza leggere il testo:
--   P4X01  conferma strutturale: immutabile; numero progressivo per prodotto; contatori registrati uguali a
--          quelli correnti; componenti e archi scritti solo nella transazione della conferma e uguali alla
--          working; dopo la conferma, nella stessa transazione, la working della RFQ non si scrive piu' (la
--          conferma si scrive per ultima); al COMMIT una sola radice, criterio coerente, perimetro chiuso
--          (ogni componente certificato raggiunto dalla radice lungo gli archi certificati); mono_pezzo senza
--          figli, e al COMMIT con radice finito, senza archi ne' figli e con lo STEP autorizzato sulla radice
--          certificata (D-P4A-02 A); con la bozza, la versione corrente e la sua impronta, e al COMMIT la bozza
--          chiusa come confermata; in formato 2 ogni arco dice la sua origine; dopo il controllo finale
--          (anche anticipato con SET CONSTRAINTS ... IMMEDIATE) il contenuto della conferma non cresce piu'
--   P4X02  registro solo in aggiunta: versioni ed eventi della bozza, registro delle bozze, correzioni,
--          associazioni, fonte STEP, attestazioni, storia delle deroghe (del fabbisogno e strutturali), coperture
--          non si modificano e non si cancellano; le storie delle deroghe e il registro delle bozze li scrivono
--          solo i loro trigger
--   P4X03  esito_intento non si modifica; si cancella solo oltre la conservazione (90 giorni). f8_stato:
--          transizioni ammesse, non si cancella
--   P4X04  ciclo di vita della bozza: una bozza chiusa non cambia; la versione cresce di uno alla volta con
--          una riga di distinta_bozza_versione; un evento appartiene alla versione salvata nella sua stessa
--          transazione; un cambio di fonte vuole un evento motivato; si cancella solo una bozza abbandonata
--          da oltre 90 giorni (con versioni ed eventi, solo in cascata); mai una confermata
--   P4X05  working_contatore non torna indietro, non si cancella e non si svuota; la scrivono solo i trigger
--          della working e della conferma, con le marche di transazione della transazione corrente
--   P4X06  copertura documentale: documento corrente del proprietario dichiarato, dello stesso tipo, con lo
--          sha256 e la rev dichiarati; componente coperto attivo con la rev dichiarata; pagine in ordine e
--          senza doppi; catena di rettifiche e revoche in ordine, sempre con lo stesso proprietario registrato
--   P4X07  fonte STEP secondaria: la destinazione temporanea (bozza + chiave) e' un nodo senza UUID della
--          versione corrente di una bozza aperta dello stesso prodotto; la risoluzione la scrive solo la conferma,
--          e non la scrive su una chiave riusata per un nodo diverso dopo l'evento
--
-- Le date della storia (creazione, chiusura, salvataggio, conferma, attestazione, eventi) le scrive il
-- database con now(), non il chiamante: la conservazione di 90 giorni e l'ordine della storia non dipendono
-- da una data passata da fuori. I 90 giorni sono scritti nei trigger: cambiarli vuole una migrazione dopo P4.
--
-- I trigger che leggono altre tabelle valgono in READ COMMITTED, il livello di tutte le transazioni del
-- server. Ordine dei lucchetti atteso (piano P4-B0 F22): prima la RFQ (BloccaThread), poi il resto; il
-- trigger del contatore prende la riga di working_contatore della RFQ dopo la scrittura su componente.
-- Questi trigger difendono dagli errori delle query, non dal proprietario delle tabelle (che puo' disattivarli),
-- ne' da chi scrive a mano le variabili di transazione cockpit.* che i trigger usano (un gesto deliberato, come
-- disattivare un trigger).
--
-- ORDINE: 1. funzione comune per i registri in sola aggiunta
--         2. working_contatore e il trigger del contatore
--         3. distinta_bozza, distinta_bozza_versione, distinta_bozza_evento
--         4. conferma_strutturale, componenti, archi; i legami con la bozza; il ciclo della bozza e il suo
--            registro permanente (distinta_bozza_registro)
--         5. esito_intento, f8_stato
--         6. correzione_documento, associazione_evento, fonte_step_evento (con la risoluzione al COMMIT)
--         7. attestazione_fascicolo, attestazione_fascicolo_documento
--         8. deroga_fabbisogno_storia e deroga_struttura_storia, con i loro trigger
--         9. copertura_documento
--        10. v_fascicolo ridefinita
--        11. commenti
--        12. variante e versione

-- I lucchetti presi sulle tabelle esistenti (componente, componente_relazione, deroga_fabbisogno, deroga_struttura
-- e i bersagli delle FK) restano fino al COMMIT del file. Se un processo ancora acceso tiene una transazione su una
-- di quelle tabelle, la migrazione non aspetta senza limite: dopo 30 secondi si annulla tutta e il server non
-- parte (la si lancia a server e worker fermi, migrations/README.md).
SET LOCAL lock_timeout = '30s';

-- ============================================================ 1. REGISTRI IN SOLA AGGIUNTA
-- Una funzione sola per i trigger che rifiutano: TG_ARGV[0] e' il codice, TG_ARGV[1] la frase. Serve sia ai
-- trigger di riga (BEFORE UPDATE / DELETE) sia a quelli di istruzione (BEFORE TRUNCATE).
CREATE FUNCTION p4_rifiuta() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION USING ERRCODE = TG_ARGV[0],
        MESSAGE = format('%s (%s): %s', TG_TABLE_NAME, TG_OP, TG_ARGV[1]),
        HINT = 'P4, migrazione 0022: la storia si aggiunge, non si riscrive';
END $$;

-- Una funzione sola per le date che scrive il database: TG_ARGV sono i nomi delle colonne timestamptz da
-- mettere a now() (l'inizio della transazione) all'INSERT, qualunque valore porti il chiamante.
CREATE FUNCTION p4_ora_del_database() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    colonna text;
    ore     jsonb := '{}';
BEGIN
    FOREACH colonna IN ARRAY TG_ARGV LOOP
        ore := ore || jsonb_build_object(colonna, now());
    END LOOP;
    NEW := jsonb_populate_record(NEW, ore);
    RETURN NEW;
END $$;

-- ============================================================ 2. working_contatore
-- Una riga per RFQ. struttura conta le transazioni che hanno cambiato la struttura della working: codice
-- (anche solo le maiuscole), tipo, archiviazione, step_strutturale_id, RFQ di un componente; padre, figlio,
-- qta, RFQ di un arco; nascita o cancellazione di un componente o di un arco. dati conta le transazioni che
-- hanno cambiato rev o descrizione di un componente o la posizione di un arco (contratto v2 §4.2, R-03: una
-- revisione solo CAD non tocca la struttura). Le altre colonne di componente (qta della radice, materiale,
-- spessore, peso, fattibilita', origine, confermato_da) non contano.
-- Una transazione conta al piu' una volta per asse (tx_struttura / tx_dati = pg_current_xact_id()): una
-- conferma B7 con mille righe e' un incremento. Una riga mancante vale (0, 0): nasce alla prima scrittura.
-- Il valore non ha significato da solo: si confronta con quello registrato in una conferma.
-- tx_conferma: la transazione dell'ultima conferma strutturale della RFQ. Da quella conferma alla fine della
-- sua transazione la working della RFQ non si scrive piu' (P4X01 in working_conta): siccome il contatore
-- conta una volta per transazione, una scrittura dopo la conferma non lo muoverebbe e la conferma
-- risulterebbe valida su una working diversa. La conferma si scrive per ultima (contratto v2 §4.3 n. 11).
-- thread_id senza FK, di proposito: la riga non si cancella mai (sotto), e una FK con cascata la
-- cancellerebbe con la RFQ; senza cascata, invece, le prove che tolgono le proprie RFQ (prima i componenti,
-- poi thread_offerta) si fermerebbero sulla riga nata dal trigger. Una RFQ vera non si cancella; nelle prove
-- la riga resta orfana, e una prova che riusa lo stesso UUID su un DB non ripulito la ritrova con i valori
-- di prima: non disturba, perche' conta solo il confronto con il valore registrato in una conferma.
-- Un TRUNCATE di componente_relazione non muove il contatore (nessun trigger TRUNCATE su quelle tabelle):
-- nessun codice lo fa; TRUNCATE di componente lo fermano le FK e, in cascata, i trigger delle conferme.
CREATE TABLE working_contatore (
    thread_id     uuid PRIMARY KEY,
    struttura     bigint NOT NULL DEFAULT 0,
    dati          bigint NOT NULL DEFAULT 0,
    tx_struttura  xid8,
    tx_dati       xid8,
    tx_conferma   xid8,
    aggiornato_il timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_working_contatore_positivi CHECK (struttura >= 0 AND dati >= 0)
);

-- Il contatore non torna indietro: un valore gia' registrato in una conferma tornerebbe «uguale» per
-- coincidenza e farebbe sembrare valida una conferma superata. Per la stessa ragione la riga non si
-- cancella e la tabella non si svuota.
-- In piu' la riga la scrivono solo i trigger della 0022 (working_conta e conferma_strutturale_nasce: profondita'
-- 2, come i registri), e le marche tx_* sono o quelle di prima o la transazione corrente: un INSERT o un UPDATE
-- scritti a mano non possono fermare il contatore (una marca della transazione messa prima delle scritture) ne'
-- togliere il muro dopo la conferma (tx_conferma rimessa a NULL). Revisione SQL della notte, R-01.
CREATE FUNCTION working_contatore_monotono() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    x xid8 := pg_current_xact_id();
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X05',
            MESSAGE = format('working_contatore della RFQ %s: non si cancella (ripartirebbe da zero)', OLD.thread_id);
    END IF;
    IF TG_OP = 'UPDATE' AND (NEW.thread_id <> OLD.thread_id OR NEW.struttura < OLD.struttura OR NEW.dati < OLD.dati) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X05',
            MESSAGE = format('working_contatore della RFQ %s: i contatori crescono soltanto (struttura %s → %s, dati %s → %s)',
                             OLD.thread_id, OLD.struttura, NEW.struttura, OLD.dati, NEW.dati);
    END IF;
    IF pg_trigger_depth() < 2 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X05',
            MESSAGE = format('working_contatore della RFQ %s: la scrivono solo i trigger della working e della conferma (%s scritto a mano)',
                             NEW.thread_id, TG_OP);
    END IF;
    IF TG_OP = 'INSERT' THEN
        IF NEW.tx_struttura IS DISTINCT FROM x AND NEW.tx_struttura IS NOT NULL
           OR NEW.tx_dati IS DISTINCT FROM x AND NEW.tx_dati IS NOT NULL
           OR NEW.tx_conferma IS DISTINCT FROM x AND NEW.tx_conferma IS NOT NULL THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X05',
                MESSAGE = format('working_contatore della RFQ %s: una marca di transazione e'' solo quella corrente', NEW.thread_id);
        END IF;
    ELSIF NEW.tx_struttura IS DISTINCT FROM x AND NEW.tx_struttura IS DISTINCT FROM OLD.tx_struttura
          OR NEW.tx_dati IS DISTINCT FROM x AND NEW.tx_dati IS DISTINCT FROM OLD.tx_dati
          OR NEW.tx_conferma IS DISTINCT FROM x AND NEW.tx_conferma IS DISTINCT FROM OLD.tx_conferma THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X05',
            MESSAGE = format('working_contatore della RFQ %s: una marca di transazione resta com''era o diventa quella corrente', NEW.thread_id);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_working_contatore_monotono BEFORE INSERT OR UPDATE OR DELETE ON working_contatore
    FOR EACH ROW EXECUTE FUNCTION working_contatore_monotono();
CREATE TRIGGER trg_working_contatore_non_si_svuota BEFORE TRUNCATE ON working_contatore
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X05', 'i contatori della working non si azzerano');

-- Il trigger del contatore. AFTER: vede solo le scritture riuscite (BOM01 della 0020, BEFORE, ha gia'
-- rifiutato quelle su una working congelata). L'UPSERT con la WHERE nel ramo DO UPDATE: alla seconda riga
-- della stessa transazione la condizione e' falsa e la riga non si riscrive (nessuna versione nuova della
-- tupla, solo il lucchetto che la transazione ha gia').
CREATE FUNCTION working_conta() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    s boolean := false;
    d boolean := false;
    x xid8;
    t uuid;
BEGIN
    IF TG_OP IN ('INSERT', 'DELETE') THEN
        s := true;
    ELSIF TG_TABLE_NAME = 'componente' THEN
        s := (NEW.codice, NEW.tipo, NEW.archiviato_il, NEW.step_strutturale_id, NEW.thread_id)
             IS DISTINCT FROM (OLD.codice, OLD.tipo, OLD.archiviato_il, OLD.step_strutturale_id, OLD.thread_id);
        d := (NEW.rev, NEW.descrizione) IS DISTINCT FROM (OLD.rev, OLD.descrizione);
    ELSE -- componente_relazione
        s := (NEW.padre_id, NEW.figlio_id, NEW.qta, NEW.thread_id)
             IS DISTINCT FROM (OLD.padre_id, OLD.figlio_id, OLD.qta, OLD.thread_id);
        d := NEW.posizione IS DISTINCT FROM OLD.posizione;
    END IF;
    IF NOT (s OR d) THEN
        RETURN NULL;                                   -- un UPDATE che non cambia niente non conta
    END IF;
    x := pg_current_xact_id();
    FOREACH t IN ARRAY (CASE
                          WHEN TG_OP = 'INSERT' THEN ARRAY[NEW.thread_id]
                          WHEN TG_OP = 'DELETE' THEN ARRAY[OLD.thread_id]
                          WHEN NEW.thread_id = OLD.thread_id THEN ARRAY[NEW.thread_id]
                          ELSE ARRAY[OLD.thread_id, NEW.thread_id]          -- cambio di RFQ: contano tutte e due
                        END) LOOP
        -- dopo una conferma strutturale della RFQ, nella stessa transazione, la working non si scrive piu':
        -- il contatore non vedrebbe la scrittura (conta una volta per transazione) e la conferma
        -- risulterebbe valida su una working diversa da quella certificata
        IF EXISTS (SELECT 1 FROM working_contatore w WHERE w.thread_id = t AND w.tx_conferma = x) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X01',
                MESSAGE = format('RFQ %s: %s su %s dopo una conferma strutturale nella stessa transazione', t, TG_OP, TG_TABLE_NAME),
                HINT = 'la conferma strutturale si scrive per ultima, dopo componenti e archi (contratto v2 §4.3 n. 11)';
        END IF;
        INSERT INTO working_contatore AS w (thread_id, struttura, dati, tx_struttura, tx_dati)
        VALUES (t, s::int, d::int, CASE WHEN s THEN x END, CASE WHEN d THEN x END)
        ON CONFLICT (thread_id) DO UPDATE SET
            struttura     = w.struttura + CASE WHEN s AND w.tx_struttura IS DISTINCT FROM x THEN 1 ELSE 0 END,
            tx_struttura  = CASE WHEN s THEN x ELSE w.tx_struttura END,
            dati          = w.dati + CASE WHEN d AND w.tx_dati IS DISTINCT FROM x THEN 1 ELSE 0 END,
            tx_dati       = CASE WHEN d THEN x ELSE w.tx_dati END,
            aggiornato_il = now()
        WHERE (s AND w.tx_struttura IS DISTINCT FROM x) OR (d AND w.tx_dati IS DISTINCT FROM x);
    END LOOP;
    RETURN NULL;
END $$;

-- UPDATE OF: il trigger non scatta nemmeno per gli UPDATE delle colonne che non contano (note di
-- fattibilita', materiale, confermato_da da solo); dentro confronta i valori, perche' una colonna nella SET
-- non vuol dire che sia cambiata.
CREATE TRIGGER trg_working_conta_componente
    AFTER INSERT OR DELETE OR UPDATE OF codice, tipo, archiviato_il, step_strutturale_id, thread_id, rev, descrizione
    ON componente FOR EACH ROW EXECUTE FUNCTION working_conta();
CREATE TRIGGER trg_working_conta_relazione
    AFTER INSERT OR DELETE OR UPDATE OF padre_id, figlio_id, qta, thread_id, posizione
    ON componente_relazione FOR EACH ROW EXECUTE FUNCTION working_conta();

-- ============================================================ 3. LA BOZZA DELLA DISTINTA
-- Il nome «bozza» da solo e' delle mail preparate (0001, tabella bozza): qui sempre distinta_bozza*.
--
-- Il modello (ratifica 05 §2 [T]): snapshot formato 2 COMPLETO a ogni versione salvata + giornale degli
-- eventi in sola aggiunta. Lo snapshot dice CHE COSA l'operatore stava editando anche se STEP, grammatiche o
-- algoritmi cambiano; gli eventi dicono COME ci si e' arrivati. Il server li riconcilia (contratto v2 §5).
--
-- distinta_bozza e' la testata: stato, versione corrente, e la COPIA della base e della fonte della
-- versione corrente (per cercare le bozze ancorate a una fonte senza aprire gli snapshot). La copia la
-- scrive il database: ogni riga nuova di distinta_bozza_versione porta la testata alla sua versione
-- (trigger distinta_bozza_versione_avanza), e la testata non cambia in nessun altro modo finche' e' aperta.
-- Una testata nasce con versione 0 e nella stessa transazione riceve la versione 1: la FK differita
-- (bozza_id, versione) → distinta_bozza_versione lo controlla al COMMIT.
CREATE TABLE distinta_bozza (
    bozza_id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id         uuid NOT NULL REFERENCES thread_offerta,
    prodotto_id       uuid NOT NULL,
    stato             text NOT NULL DEFAULT 'aperta',
    formato           smallint NOT NULL DEFAULT 2,
    versione          int NOT NULL DEFAULT 0,               -- versione ottimistica = ultima riga di distinta_bozza_versione
    base_conferma_id  uuid,                                 -- FK al passo 4, quando esiste conferma_strutturale
    base_numero       int NOT NULL,                         -- 0 = il prodotto non aveva conferme
    base_contatore    bigint NOT NULL,                      -- working_contatore.struttura letto con la base
    firma_base        char(64) NOT NULL,                    -- firma del perimetro del prodotto: la legge il server, la rimanda la pagina; B2 la ricalcola
    fonte_allegato_id uuid REFERENCES allegato,             -- ancoraggio alla fonte STEP (contratto v2 §2.2 sorgente)
    fonte_sha256      char(64),
    fonte_gruppo      text,                                 -- opaca: bom | cand:<allegato>:<radice> | senza
    creata_da         uuid NOT NULL REFERENCES utente,
    creata_il         timestamptz NOT NULL DEFAULT now(),
    aggiornata_da     uuid NOT NULL REFERENCES utente,
    aggiornata_il     timestamptz NOT NULL DEFAULT now(),
    chiusa_da         uuid REFERENCES utente,
    chiusa_il         timestamptz,                          -- la scrive il trigger del ciclo, con now()
    motivo_chiusura   text,
    -- il prodotto con una bozza non si rimuove: si archivia (D-P4B1-04 A [U]). Questa FK dura quanto la
    -- bozza; la protezione permanente, anche dopo la conservazione, e' la FK di distinta_bozza_registro
    -- (D-P4A-01, P-04a [U])
    CONSTRAINT fk_distinta_bozza_prodotto FOREIGN KEY (thread_id, prodotto_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ux_distinta_bozza_id_prodotto UNIQUE (bozza_id, prodotto_id),
    CONSTRAINT ck_distinta_bozza_stato CHECK (stato IN ('aperta', 'confermata', 'abbandonata')),
    CONSTRAINT ck_distinta_bozza_formato CHECK (formato = 2),
    CONSTRAINT ck_distinta_bozza_versione CHECK (versione >= 0),
    CONSTRAINT ck_distinta_bozza_base CHECK (base_numero >= 0 AND base_contatore >= 0
        AND (base_numero = 0) = (base_conferma_id IS NULL)
        AND firma_base ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_distinta_bozza_fonte CHECK ((fonte_allegato_id IS NULL) = (fonte_sha256 IS NULL)
        AND (fonte_sha256 IS NULL OR fonte_sha256 ~ '^[0-9a-f]{64}$')
        AND (fonte_gruppo IS NULL OR (btrim(fonte_gruppo) <> '' AND char_length(fonte_gruppo) <= 300))),
    CONSTRAINT ck_distinta_bozza_chiusura CHECK ((stato = 'aperta') = (chiusa_il IS NULL)
        AND (chiusa_il IS NULL) = (chiusa_da IS NULL)
        AND (stato <> 'aperta' OR motivo_chiusura IS NULL)
        AND (motivo_chiusura IS NULL OR char_length(motivo_chiusura) <= 1000))
);
-- Una bozza aperta per prodotto (contratto v2 §5.1: la seconda e' 409 BOZZA_GIA_APERTA).
CREATE UNIQUE INDEX ux_distinta_bozza_aperta ON distinta_bozza (thread_id, prodotto_id) WHERE stato = 'aperta';
CREATE INDEX ix_distinta_bozza_prodotto ON distinta_bozza (prodotto_id, creata_il);
CREATE INDEX ix_distinta_bozza_fonte ON distinta_bozza (fonte_allegato_id) WHERE fonte_allegato_id IS NOT NULL;

-- Ogni versione salvata, con lo snapshot intero: non si riscrive e non si cancella (salvo la conservazione
-- delle bozze abbandonate). impronta = sha256 dello snapshot canonico, calcolata dal Go: un salvataggio con
-- lo stesso snapshot e senza azioni non crea una versione nuova (contratto v2 §5.1). La conferma che chiude
-- la bozza porta la stessa impronta (conferma_strutturale.impronta_snapshot). tx = la transazione del
-- salvataggio: gli eventi della versione si scrivono in quella stessa transazione.
CREATE TABLE distinta_bozza_versione (
    bozza_id          uuid NOT NULL REFERENCES distinta_bozza ON DELETE CASCADE,
    versione          int NOT NULL,
    snapshot          jsonb NOT NULL,
    impronta          char(64) NOT NULL,
    base_conferma_id  uuid,
    base_numero       int NOT NULL,
    base_contatore    bigint NOT NULL,
    firma_base        char(64) NOT NULL,
    fonte_allegato_id uuid REFERENCES allegato,
    fonte_sha256      char(64),
    fonte_gruppo      text,
    salvata_da        uuid NOT NULL REFERENCES utente,
    salvata_il        timestamptz NOT NULL DEFAULT now(),
    tx                xid8 NOT NULL DEFAULT pg_current_xact_id(),
    PRIMARY KEY (bozza_id, versione),
    CONSTRAINT ck_dbv_versione CHECK (versione >= 1),
    -- 4 MiB: il corpo JSON del salvataggio e' al piu' 2 MiB + 4 KiB (contratto v2 §1.2) e jsonb puo' pesare
    -- piu' del testo; pg_column_size qui misura il valore non ancora compresso
    CONSTRAINT ck_dbv_snapshot CHECK (jsonb_typeof(snapshot) = 'object' AND pg_column_size(snapshot) <= 4194304),
    CONSTRAINT ck_dbv_impronta CHECK (impronta ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_dbv_base CHECK (base_numero >= 0 AND base_contatore >= 0
        AND (base_numero = 0) = (base_conferma_id IS NULL)
        AND firma_base ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_dbv_fonte CHECK ((fonte_allegato_id IS NULL) = (fonte_sha256 IS NULL)
        AND (fonte_sha256 IS NULL OR fonte_sha256 ~ '^[0-9a-f]{64}$')
        AND (fonte_gruppo IS NULL OR (btrim(fonte_gruppo) <> '' AND char_length(fonte_gruppo) <= 300)))
);

-- Il giornale: ogni azione nel tempo, anche quelle poi annullate (matrice 07 §1). Il vocabolario delle
-- azioni e' versionato nel Go (colonna vocabolario); qui solo la forma. Ogni evento appartiene alla versione
-- prodotta dal salvataggio che lo ha registrato: la versione si scrive prima dei suoi eventi, nella stessa
-- transazione (trigger distinta_bozza_evento_nasce). Un evento del server fuori da un salvataggio vuole
-- quindi una versione nuova.
CREATE TABLE distinta_bozza_evento (
    bozza_id     uuid NOT NULL REFERENCES distinta_bozza ON DELETE CASCADE,
    n            int NOT NULL,
    versione     int NOT NULL,
    vocabolario  smallint NOT NULL DEFAULT 1,
    azione       text NOT NULL,
    chiave_bozza text,
    prima        jsonb,
    dopo         jsonb,
    origine      text NOT NULL,
    motivo       text,
    utente_id    uuid NOT NULL REFERENCES utente,
    quando       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bozza_id, n),
    CONSTRAINT fk_dbe_versione FOREIGN KEY (bozza_id, versione)
        REFERENCES distinta_bozza_versione (bozza_id, versione) ON DELETE CASCADE,
    CONSTRAINT ck_dbe_n CHECK (n >= 1 AND versione >= 1 AND vocabolario >= 1),
    CONSTRAINT ck_dbe_azione CHECK (azione ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT ck_dbe_chiave CHECK (chiave_bozza IS NULL OR chiave_bozza ~ '^[A-Za-z0-9._:-]{1,64}$'),
    CONSTRAINT ck_dbe_origine CHECK (origine IN ('step', 'utente', 'conferma', 'server')),
    CONSTRAINT ck_dbe_prima CHECK (prima IS NULL OR pg_column_size(prima) <= 262144),
    CONSTRAINT ck_dbe_dopo CHECK (dopo IS NULL OR pg_column_size(dopo) <= 262144),
    CONSTRAINT ck_dbe_motivo CHECK (motivo IS NULL OR (btrim(motivo) <> '' AND char_length(motivo) <= 1000))
);
CREATE INDEX ix_dbe_versione ON distinta_bozza_evento (bozza_id, versione);
CREATE TRIGGER trg_dbe_ora BEFORE INSERT ON distinta_bozza_evento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('quando');

-- ============================================================ 4. LA CONFERMA STRUTTURALE
-- Ufficiale, progressiva e immutabile per prodotto (ratifica 05 §2; contratto v2 §4). Diversa da
-- bom_versione (congelamento) e da BOM.Verificata (calcolata dal Motore A). Formato 1 (giro 4,
-- distinta/albero/conferma) o 2 (R5): il formato 1 registra senza riepilogo v2 e senza idempotenza
-- (D-P4B0-06 A). mono_pezzo e' una decisione esplicita con motivo, solo senza figli (D-P4B0-01 B'): perde
-- efficacia da sola quando compaiono figli, perche' la conferma non e' piu' valida. Si dichiara in tutti e due
-- i formati (D-P4B1-01 B [U]). E' la classificazione del nodo finito, mai una proprieta' del documento STEP
-- (D-P4A-02 A, precisazione [U]): componente.tipo resta finito, e «finito mono_pezzo» si legge dall'ultima
-- conferma valida [T]. Lo STEP autorizzato sulla radice resta un prerequisito (D-P4B1-02 A [U]): il database
-- lo controlla al COMMIT (conferma_strutturale_completa); che la fonte sia corrente (K-02) lo controlla il Go.
CREATE TABLE conferma_strutturale (
    conferma_id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id          uuid NOT NULL REFERENCES thread_offerta,
    prodotto_id        uuid NOT NULL,
    numero             int NOT NULL,
    precedente_id      uuid,                                -- la conferma numero - 1 dello stesso prodotto
    formato            smallint NOT NULL,
    senza_riepilogo_v2 boolean NOT NULL,
    firma_struttura    char(64) NOT NULL,                   -- firma del perimetro DOPO le scritture
    firma_anteprima    text,                                -- f.2: la firma del riepilogo; f.1: la firma della busta
    base_conferma_id   uuid,
    base_contatore     bigint NOT NULL,
    firma_base         char(64),
    contatore          bigint NOT NULL,                     -- working_contatore.struttura letto DOPO le scritture
    contatore_dati     bigint NOT NULL,                     -- working_contatore.dati letto DOPO le scritture
    bozza_id           uuid,
    bozza_versione     int,
    fonte_allegato_id  uuid REFERENCES allegato,            -- la fonte STEP su cui si e' confermato, se c'era
    fonte_sha256       char(64),
    fonte_gruppo       text,
    origine            jsonb NOT NULL,                      -- rotta (obbligatoria), bozza_f2_aperta (f.1), ...
    snapshot           jsonb NOT NULL,                      -- lo stato desiderato confermato
    impronta_snapshot  char(64) NOT NULL,                   -- come distinta_bozza_versione.impronta
    delta              jsonb NOT NULL,                      -- slide 1 e slide 2 calcolate dal server
    mappa              jsonb NOT NULL,                      -- chiave_bozza → componente_id (f.1: {} o per gruppo)
    decisioni          jsonb NOT NULL DEFAULT '[]',         -- il registro completo delle decisioni (f.1: quelle del formato 1, E1.1)
    mono_pezzo         boolean NOT NULL DEFAULT false,
    mono_pezzo_motivo  text,
    criterio_verifica  text NOT NULL,                       -- su che cosa puo' poggiare la verifica di questa conferma
    verifica           jsonb NOT NULL DEFAULT '{}',         -- verifica_motore_a calcolata nella transazione (contratto v2 §4.3 n. 12)
    chiave_idempotenza uuid,
    confermato_da      uuid NOT NULL REFERENCES utente,
    confermato_il      timestamptz NOT NULL DEFAULT now(),
    tx                 xid8 NOT NULL DEFAULT pg_current_xact_id(),
    CONSTRAINT fk_conferma_prodotto FOREIGN KEY (thread_id, prodotto_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ux_conferma_prodotto_numero UNIQUE (prodotto_id, numero),
    CONSTRAINT ux_conferma_thread_id UNIQUE (thread_id, conferma_id),          -- bersagli delle FK composite
    CONSTRAINT ux_conferma_id_prodotto UNIQUE (conferma_id, prodotto_id),
    CONSTRAINT ux_conferma_id_chiave UNIQUE (conferma_id, chiave_idempotenza),
    CONSTRAINT fk_conferma_precedente FOREIGN KEY (precedente_id, prodotto_id)
        REFERENCES conferma_strutturale (conferma_id, prodotto_id),
    CONSTRAINT fk_conferma_base FOREIGN KEY (base_conferma_id, prodotto_id)
        REFERENCES conferma_strutturale (conferma_id, prodotto_id),
    CONSTRAINT fk_conferma_bozza FOREIGN KEY (bozza_id, prodotto_id)
        REFERENCES distinta_bozza (bozza_id, prodotto_id),
    CONSTRAINT fk_conferma_bozza_versione FOREIGN KEY (bozza_id, bozza_versione)
        REFERENCES distinta_bozza_versione (bozza_id, versione),
    CONSTRAINT ck_conferma_numero CHECK (numero >= 1 AND (numero = 1) = (precedente_id IS NULL)),
    CONSTRAINT ck_conferma_formato CHECK (formato IN (1, 2) AND senza_riepilogo_v2 = (formato = 1)),
    CONSTRAINT ck_conferma_firme CHECK (firma_struttura ~ '^[0-9a-f]{64}$'
        AND (firma_base IS NULL OR firma_base ~ '^[0-9a-f]{64}$')
        AND (firma_anteprima IS NULL OR char_length(firma_anteprima) <= 200)
        AND (formato = 1 OR (firma_anteprima IS NOT NULL AND firma_anteprima ~ '^[0-9a-f]{64}$' AND firma_base IS NOT NULL))),
    -- f.2: la base e' l'ultima conferma letta sotto lucchetto (409 BASE_SUPERATA altrimenti), quindi la precedente
    CONSTRAINT ck_conferma_base CHECK (base_contatore >= 0 AND contatore >= 0 AND contatore_dati >= 0
        AND (formato = 1 OR base_conferma_id IS NOT DISTINCT FROM precedente_id)),
    CONSTRAINT ck_conferma_bozza CHECK ((bozza_id IS NULL) = (bozza_versione IS NULL) AND (formato = 2 OR bozza_id IS NULL)),
    CONSTRAINT ck_conferma_fonte CHECK ((fonte_allegato_id IS NULL) = (fonte_sha256 IS NULL)
        AND (fonte_sha256 IS NULL OR fonte_sha256 ~ '^[0-9a-f]{64}$')
        AND (fonte_gruppo IS NULL OR (btrim(fonte_gruppo) <> '' AND char_length(fonte_gruppo) <= 300))),
    -- delta fino a 16 MiB: lo calcola il server e con 5000 nodi (MaxVociBozza) puo' superare i 4 MiB del
    -- corpo; snapshot e decisioni vengono dal corpo (al piu' 2 MiB + 4 KiB, contratto v2 §1.2)
    CONSTRAINT ck_conferma_json CHECK (jsonb_typeof(origine) = 'object' AND pg_column_size(origine) <= 65536
        AND COALESCE(jsonb_typeof(origine -> 'rotta') = 'string', false)
        AND jsonb_typeof(snapshot) = 'object' AND pg_column_size(snapshot) <= 4194304
        AND jsonb_typeof(delta) = 'object' AND pg_column_size(delta) <= 16777216
        AND jsonb_typeof(mappa) = 'object' AND pg_column_size(mappa) <= 2097152
        AND jsonb_typeof(decisioni) = 'array' AND pg_column_size(decisioni) <= 4194304
        AND jsonb_typeof(verifica) = 'object' AND pg_column_size(verifica) <= 65536),
    -- f.2: lo snapshot e' il corpo formato 2 dello stesso prodotto (contratto v2 §2.1)
    CONSTRAINT ck_conferma_snapshot CHECK (impronta_snapshot ~ '^[0-9a-f]{64}$'
        AND (formato = 1 OR (COALESCE(snapshot ->> 'formato' = '2', false)
                             AND COALESCE(snapshot ->> 'prodotto_id' = prodotto_id::text, false)))),
    CONSTRAINT ck_conferma_mono_pezzo CHECK (mono_pezzo = (mono_pezzo_motivo IS NOT NULL)
        AND (mono_pezzo_motivo IS NULL OR (btrim(mono_pezzo_motivo) <> '' AND char_length(mono_pezzo_motivo) <= 1000))),
    -- f.2: la conferma porta mono_pezzo se e solo se lo snapshot che conferma contiene la dichiarazione mono_pezzo
    -- (D-P4A-03 A' [U], E2.12): R5 non registra una conferma mono_pezzo = false che finga di recepire la
    -- dichiarazione, ne' una mono_pezzo senza dichiarazione. Il formato 1 scrive le decisioni nella colonna
    -- decisioni (E1.1), non nello snapshot: qui non e' controllato. Revisione di dominio della notte, RD-02.
    CONSTRAINT ck_conferma_mono_pezzo_snapshot CHECK (formato = 1
        OR mono_pezzo = COALESCE(snapshot -> 'decisioni' @> '[{"tipo": "mono_pezzo"}]'::jsonb, false)),
    -- step: la verifica puo' poggiare sullo STEP autorizzato del prodotto (al COMMIT la radice certificata
    -- deve averlo); mono_pezzo: sulla dichiarazione motivata di questa conferma INSIEME allo STEP autorizzato
    -- (al COMMIT anche lui, D-P4A-02 A); nessuno: la verifica non poggia su uno STEP, perche' alla conferma il
    -- prodotto non aveva uno STEP autorizzato e corrente (caso normale prima dell'autorizzazione, o K-02 fallito:
    -- STEP sostituito o revocato). Il database ammette nessuno anche con uno STEP sulla radice: e' la direzione
    -- prudente. Testo completo nel COMMENT della colonna.
    CONSTRAINT ck_conferma_criterio CHECK (criterio_verifica IN ('step', 'mono_pezzo', 'nessuno')
        AND mono_pezzo = (criterio_verifica = 'mono_pezzo')),
    CONSTRAINT ck_conferma_chiave CHECK ((formato = 2) = (chiave_idempotenza IS NOT NULL))
);
CREATE UNIQUE INDEX ux_conferma_chiave ON conferma_strutturale (chiave_idempotenza) WHERE chiave_idempotenza IS NOT NULL;
CREATE UNIQUE INDEX ux_conferma_bozza ON conferma_strutturale (bozza_id) WHERE bozza_id IS NOT NULL;
CREATE INDEX ix_conferma_thread ON conferma_strutturale (thread_id, confermato_il);

-- I legami della testata della bozza con le conferme, ora che la tabella esiste.
ALTER TABLE distinta_bozza
    ADD CONSTRAINT fk_distinta_bozza_base FOREIGN KEY (base_conferma_id, prodotto_id)
        REFERENCES conferma_strutturale (conferma_id, prodotto_id),
    -- la testata punta sempre a una versione scritta: controllata al COMMIT, perche' la versione 1 nasce
    -- dopo la testata nella stessa transazione
    ADD CONSTRAINT fk_distinta_bozza_versione_corrente FOREIGN KEY (bozza_id, versione)
        REFERENCES distinta_bozza_versione (bozza_id, versione) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE distinta_bozza_versione
    ADD CONSTRAINT fk_dbv_base FOREIGN KEY (base_conferma_id) REFERENCES conferma_strutturale;

-- Che cosa la conferma certifica: i componenti del perimetro del prodotto (attivi, raggiungibili) com'erano
-- al COMMIT. La FK verso componente rende il componente «storia»: non si cancella piu', si archivia (piano
-- P4-B0 §3.3, K7). step_strutturale_id e step_sha256 sono copie storiche, senza FK: una revoca o una
-- sostituzione successive non devono essere bloccate da una conferma vecchia.
CREATE TABLE conferma_strutturale_componente (
    conferma_id         uuid NOT NULL,
    thread_id           uuid NOT NULL,
    componente_id       uuid NOT NULL,
    radice              boolean NOT NULL DEFAULT false,      -- il prodotto della conferma
    chiave_bozza        text,                                -- f.2: la chiave della bozza; f.1: NULL
    codice              varchar(60) NOT NULL,
    rev                 varchar(10),
    descrizione         varchar(200),
    tipo                tipo_componente NOT NULL,
    step_strutturale_id uuid,
    step_sha256         char(64),
    PRIMARY KEY (conferma_id, componente_id),
    CONSTRAINT fk_csc_conferma FOREIGN KEY (thread_id, conferma_id)
        REFERENCES conferma_strutturale (thread_id, conferma_id),
    CONSTRAINT fk_csc_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ck_csc_chiave CHECK (chiave_bozza IS NULL OR chiave_bozza ~ '^[A-Za-z0-9._:-]{1,64}$'),
    CONSTRAINT ck_csc_step CHECK ((step_strutturale_id IS NULL) = (step_sha256 IS NULL)
        AND (step_sha256 IS NULL OR step_sha256 ~ '^[0-9a-f]{64}$')
        AND (step_strutturale_id IS NULL OR tipo = 'finito'))
);
CREATE UNIQUE INDEX ux_csc_una_radice ON conferma_strutturale_componente (conferma_id) WHERE radice;
CREATE INDEX ix_csc_componente ON conferma_strutturale_componente (componente_id);

-- Gli archi coperti: TUTTI gli archi del perimetro con la loro qta al COMMIT (contratto v2 §4.3 n. 9). Sono
-- il nuovo produttore della verifica della gerarchia (piano §3.3, R-02): la gerarchia e' verificata solo se
-- gli archi correnti coincidono con quelli dell'ultima conferma. Gli estremi sono componenti della stessa
-- conferma (FK verso conferma_strutturale_componente, come bom_versione_relazione).
CREATE TABLE conferma_strutturale_arco (
    conferma_id uuid NOT NULL,
    padre_id    uuid NOT NULL,
    figlio_id   uuid NOT NULL,
    qta         int NOT NULL,
    posizione   varchar(20),
    origine     text,                                       -- step | utente | conferma | registrato: obbligatoria in f.2 (trigger)
    PRIMARY KEY (conferma_id, padre_id, figlio_id),
    CONSTRAINT fk_csa_padre FOREIGN KEY (conferma_id, padre_id)
        REFERENCES conferma_strutturale_componente (conferma_id, componente_id),
    CONSTRAINT fk_csa_figlio FOREIGN KEY (conferma_id, figlio_id)
        REFERENCES conferma_strutturale_componente (conferma_id, componente_id),
    CONSTRAINT ck_csa_arco CHECK (padre_id <> figlio_id AND qta > 0),
    CONSTRAINT ck_csa_origine CHECK (origine IS NULL OR origine IN ('step', 'utente', 'conferma', 'registrato'))
);
CREATE INDEX ix_csa_coppia ON conferma_strutturale_arco (padre_id, figlio_id);
CREATE INDEX ix_csa_figlio ON conferma_strutturale_arco (figlio_id);

-- Numero progressivo, prodotto finito, bozza aperta alla sua versione corrente con la stessa impronta,
-- mono_pezzo senza figli, contatori registrati uguali a quelli correnti. Poi segna la RFQ: da qui alla fine
-- della transazione la working non si scrive piu' (working_conta). Le righe si mettono in fila sul
-- lucchetto della RFQ che il Go prende prima (BloccaThread); due conferme concorrenti con lo stesso numero
-- le ferma comunque ux_conferma_prodotto_numero.
CREATE FUNCTION conferma_strutturale_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER trg_conferma_strutturale_nasce BEFORE INSERT ON conferma_strutturale
    FOR EACH ROW EXECUTE FUNCTION conferma_strutturale_nasce();

-- Componenti e archi: solo nella transazione della testata, e uguali alla working di quel momento (la
-- conferma si scrive dopo componenti e archi: contratto v2 §4.3 n. 11). Cosi' una conferma non puo'
-- certificare una cosa che nella working non c'era; e siccome dopo la testata la working della RFQ non si
-- scrive piu' (working_conta), la working di quel momento e' quella del COMMIT.
-- Il controllo finale (conferma_strutturale_completa) scatta una volta per conferma: al COMMIT, oppure prima se
-- qualcuno chiede SET CONSTRAINTS ... IMMEDIATE, e allora non si ripete. Per questo, quando passa, annota la
-- conferma nella variabile di transazione cockpit.p4_conferme_controllate, e da li' in poi il suo contenuto non
-- cresce piu' (revisione SQL della notte, R-02).
CREATE FUNCTION conferma_strutturale_contenuto() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER trg_csc_contenuto BEFORE INSERT ON conferma_strutturale_componente
    FOR EACH ROW EXECUTE FUNCTION conferma_strutturale_contenuto();
CREATE TRIGGER trg_csa_contenuto BEFORE INSERT ON conferma_strutturale_arco
    FOR EACH ROW EXECUTE FUNCTION conferma_strutturale_contenuto();

-- Al COMMIT, una volta per conferma: (a) una e una sola riga radice; (b) criterio step solo se la radice
-- certificata ha lo STEP autorizzato; (b') mono_pezzo (D-P4A-02 A [U]) solo con la radice certificata di tipo
-- finito, con lo STEP autorizzato, senza altri componenti ne' archi certificati e senza figli nella working;
-- (c) perimetro chiuso: ogni arco della working che parte da un componente certificato verso un figlio non
-- archiviato e' fra gli archi certificati (con la stessa qta), e ogni componente certificato e' raggiunto dalla
-- radice lungo gli archi certificati (visita con UNION sui soli archi della conferma: termina anche con un ciclo
-- e costa al piu' quanto le righe della conferma). La sola condizione «ogni non radice ha un arco che arriva» non
-- bastava: il database non vieta i cicli (0018), e un'isola X ↔ Y fuori dal perimetro la soddisfaceva
-- (revisioni della notte, R-03 / RSC-01). (d) con la bozza: al COMMIT la bozza e' chiusa come confermata (una
-- conferma che la cita la lascerebbe aperta e inservibile, RSC-09).
-- Il controllo (b') non dice se lo STEP e' ancora la fonte corrente (sostituito, revocato, superato): quello e'
-- K-02, del Go, che il database non sostituisce.
-- Quando passa, annota la conferma in cockpit.p4_conferme_controllate (conferma_strutturale_contenuto, R-02).
CREATE FUNCTION conferma_strutturale_completa() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE CONSTRAINT TRIGGER trg_conferma_strutturale_completa AFTER INSERT ON conferma_strutturale
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION conferma_strutturale_completa();

-- Immutabili: niente UPDATE, DELETE, TRUNCATE.
CREATE TRIGGER trg_conferma_strutturale_immutabile BEFORE UPDATE OR DELETE ON conferma_strutturale
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X01', 'una conferma strutturale non si modifica e non si cancella; se ne registra una nuova');
CREATE TRIGGER trg_csc_immutabile BEFORE UPDATE OR DELETE ON conferma_strutturale_componente
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X01', 'il contenuto di una conferma non cambia');
CREATE TRIGGER trg_csa_immutabile BEFORE UPDATE OR DELETE ON conferma_strutturale_arco
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X01', 'il contenuto di una conferma non cambia');
CREATE TRIGGER trg_conferma_strutturale_non_si_svuota BEFORE TRUNCATE ON conferma_strutturale
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X01', 'le conferme non si svuotano');
CREATE TRIGGER trg_csc_non_si_svuota BEFORE TRUNCATE ON conferma_strutturale_componente
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X01', 'le conferme non si svuotano');
CREATE TRIGGER trg_csa_non_si_svuota BEFORE TRUNCATE ON conferma_strutturale_arco
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X01', 'le conferme non si svuotano');

-- Il ciclo di vita della bozza (ora che conferma_strutturale esiste, la chiusura «confermata» la puo' guardare).
--   INSERT: nasce aperta, alla versione 0, di un prodotto finito;
--   UPDATE da aperta: o un salvataggio (versione + 1, solo base, fonte e aggiornata_*: lo fa il trigger di
--           distinta_bozza_versione), o la chiusura (confermata con una conferma che la cita, oppure
--           abbandonata), senza cambiare nient'altro; una bozza chiusa non cambia piu';
--   DELETE: solo abbandonata e chiusa da oltre 90 giorni (piano P4-B0 §3.5, R-20); versioni ed eventi vanno
--           con lei (cascata). Una confermata e' storia: mai.
CREATE FUNCTION distinta_bozza_ciclo() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    tipo_prodotto tipo_componente;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.stato <> 'aperta' OR NEW.versione <> 0 THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X04',
                MESSAGE = format('distinta_bozza %s: nasce aperta alla versione 0 (la versione 1 e'' la sua prima riga di distinta_bozza_versione)', NEW.bozza_id);
        END IF;
        SELECT c.tipo INTO tipo_prodotto FROM componente c WHERE c.componente_id = NEW.prodotto_id AND c.thread_id = NEW.thread_id;
        IF FOUND AND tipo_prodotto <> 'finito' THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X04',
                MESSAGE = format('distinta_bozza: %s e'' un %s, non un prodotto finito', NEW.prodotto_id, tipo_prodotto);
        END IF;
        NEW.creata_il := now();
        NEW.aggiornata_il := now();
        RETURN NEW;
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF OLD.stato <> 'abbandonata' OR OLD.chiusa_il >= now() - interval '90 days' THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X04',
                MESSAGE = format('distinta_bozza %s (%s): si cancella solo una bozza abbandonata da oltre 90 giorni', OLD.bozza_id, OLD.stato);
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.stato <> 'aperta' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s e'' %s: una bozza chiusa non cambia', OLD.bozza_id, OLD.stato);
    END IF;
    IF (NEW.bozza_id, NEW.thread_id, NEW.prodotto_id, NEW.formato, NEW.creata_da, NEW.creata_il)
       IS DISTINCT FROM (OLD.bozza_id, OLD.thread_id, OLD.prodotto_id, OLD.formato, OLD.creata_da, OLD.creata_il) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s: RFQ, prodotto, formato e creazione non cambiano', OLD.bozza_id);
    END IF;
    IF NEW.stato = 'aperta' THEN
        IF NEW.versione <> OLD.versione + 1 THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X04',
                MESSAGE = format('distinta_bozza %s: da aperta la versione cresce di uno con una riga di distinta_bozza_versione (%s → %s)',
                                 OLD.bozza_id, OLD.versione, NEW.versione);
        END IF;
        NEW.aggiornata_il := now();
        RETURN NEW;
    END IF;
    NEW.chiusa_il := now();                            -- la data della chiusura (e della conservazione) e' del database
    -- chiusura: niente altro cambia
    IF (NEW.versione, NEW.base_conferma_id, NEW.base_numero, NEW.base_contatore, NEW.firma_base,
        NEW.fonte_allegato_id, NEW.fonte_sha256, NEW.fonte_gruppo, NEW.aggiornata_da, NEW.aggiornata_il)
       IS DISTINCT FROM
       (OLD.versione, OLD.base_conferma_id, OLD.base_numero, OLD.base_contatore, OLD.firma_base,
        OLD.fonte_allegato_id, OLD.fonte_sha256, OLD.fonte_gruppo, OLD.aggiornata_da, OLD.aggiornata_il) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s: la chiusura cambia solo stato, chiusa_da, chiusa_il e motivo', OLD.bozza_id);
    END IF;
    IF (NEW.stato = 'confermata') <> EXISTS (SELECT 1 FROM conferma_strutturale x WHERE x.bozza_id = NEW.bozza_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s: confermata solo con la conferma strutturale che la cita, scritta prima; abbandonata solo senza',
                             OLD.bozza_id);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_distinta_bozza_ciclo BEFORE INSERT OR UPDATE OR DELETE ON distinta_bozza
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_ciclo();
CREATE TRIGGER trg_distinta_bozza_non_si_svuota BEFORE TRUNCATE ON distinta_bozza
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X04', 'le bozze della distinta non si svuotano');

-- Il registro permanente delle bozze (D-P4A-01 = P-04a [U], direttiva 11; D-P4B1-04 A). Una riga per ogni
-- passo del ciclo di una bozza: aperta, confermata, abbandonata, cancellata (dalla conservazione). La scrive
-- solo il trigger di distinta_bozza; non si modifica e non si cancella. bozza_id e' storico, senza FK: la bozza
-- abbandonata si cancella oltre 90 giorni con versioni ed eventi, il registro resta (e con lui la storia dei
-- rifiuti delle fonti secondarie che puntano a quella bozza, fonte_step_evento.bozza_id). prodotto_id ha la FK:
-- da quando si apre una bozza il prodotto non si rimuove piu', si archivia, anche dopo la conservazione.
CREATE TABLE distinta_bozza_registro (
    registro_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bozza_id    uuid NOT NULL,
    thread_id   uuid NOT NULL,
    prodotto_id uuid NOT NULL,
    passo       text NOT NULL,
    utente_id   uuid REFERENCES utente,                 -- NULL solo per 'cancellata' (la conservazione)
    motivo      text,
    quando      timestamptz NOT NULL DEFAULT now(),
    tx          xid8 NOT NULL DEFAULT pg_current_xact_id(),
    CONSTRAINT fk_dbr_prodotto FOREIGN KEY (thread_id, prodotto_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ck_dbr_passo CHECK (passo IN ('aperta', 'confermata', 'abbandonata', 'cancellata')),
    CONSTRAINT ck_dbr_utente CHECK ((passo = 'cancellata') = (utente_id IS NULL)),
    CONSTRAINT ck_dbr_motivo CHECK (motivo IS NULL OR (btrim(motivo) <> '' AND char_length(motivo) <= 1000))
);
CREATE UNIQUE INDEX ux_dbr_bozza_passo ON distinta_bozza_registro (bozza_id, passo);
CREATE INDEX ix_dbr_prodotto ON distinta_bozza_registro (prodotto_id, quando);
CREATE INDEX ix_dbr_thread ON distinta_bozza_registro (thread_id, quando);

-- AFTER: registra solo i passi riusciti (il ciclo, BEFORE, ha gia' rifiutato quelli non ammessi). La
-- conservazione non sa chi l'ha chiesta: utente NULL, e il motivo dice da quando la bozza era abbandonata.
CREATE FUNCTION distinta_bozza_registra() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO distinta_bozza_registro (bozza_id, thread_id, prodotto_id, passo, utente_id)
        VALUES (NEW.bozza_id, NEW.thread_id, NEW.prodotto_id, 'aperta', NEW.creata_da);
    ELSIF TG_OP = 'UPDATE' THEN
        IF NEW.stato IS DISTINCT FROM OLD.stato THEN
            INSERT INTO distinta_bozza_registro (bozza_id, thread_id, prodotto_id, passo, utente_id, motivo)
            VALUES (NEW.bozza_id, NEW.thread_id, NEW.prodotto_id, NEW.stato, NEW.chiusa_da, NEW.motivo_chiusura);
        END IF;
    ELSE
        INSERT INTO distinta_bozza_registro (bozza_id, thread_id, prodotto_id, passo, utente_id, motivo)
        VALUES (OLD.bozza_id, OLD.thread_id, OLD.prodotto_id, 'cancellata', NULL,
                format('conservazione: abbandonata il %s', OLD.chiusa_il));
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_distinta_bozza_registra AFTER INSERT OR UPDATE OF stato OR DELETE ON distinta_bozza
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_registra();

-- Una riga nasce solo dal trigger di distinta_bozza (profondita' 2), mai da un INSERT diretto; data e
-- transazione le scrive il database.
CREATE FUNCTION distinta_bozza_registro_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF pg_trigger_depth() < 2 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = 'distinta_bozza_registro: lo scrive solo il trigger di distinta_bozza';
    END IF;
    NEW.quando := now();
    NEW.tx := pg_current_xact_id();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_dbr_nasce BEFORE INSERT ON distinta_bozza_registro
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_registro_nasce();
CREATE TRIGGER trg_dbr_solo_aggiunte BEFORE UPDATE OR DELETE ON distinta_bozza_registro
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'il registro delle bozze non si modifica e non si cancella');
CREATE TRIGGER trg_dbr_non_si_svuota BEFORE TRUNCATE ON distinta_bozza_registro
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'il registro delle bozze non si svuota');

-- Una versione nuova porta la testata con se': versione, base, fonte, chi e quando. Solo se la bozza e'
-- aperta e la versione e' esattamente la successiva (salvataggi concorrenti: uno dei due trova la PK
-- occupata o la testata gia' avanti → 409 BOZZA_VERSIONE nel Go).
CREATE FUNCTION distinta_bozza_versione_avanza() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE distinta_bozza b
       SET versione = NEW.versione, base_conferma_id = NEW.base_conferma_id, base_numero = NEW.base_numero,
           base_contatore = NEW.base_contatore, firma_base = NEW.firma_base, fonte_allegato_id = NEW.fonte_allegato_id,
           fonte_sha256 = NEW.fonte_sha256, fonte_gruppo = NEW.fonte_gruppo,
           aggiornata_da = NEW.salvata_da, aggiornata_il = NEW.salvata_il
     WHERE b.bozza_id = NEW.bozza_id AND b.stato = 'aperta' AND b.versione = NEW.versione - 1;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s: la versione %s si scrive solo su una bozza aperta alla versione %s',
                             NEW.bozza_id, NEW.versione, NEW.versione - 1);
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER trg_dbv_avanza AFTER INSERT ON distinta_bozza_versione
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_versione_avanza();

-- Prima della riga: data e transazione del salvataggio le scrive il database; base_numero e' il numero della
-- conferma di base (stesso prodotto: lo controlla la FK composita della testata).
CREATE FUNCTION distinta_bozza_versione_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER trg_dbv_nasce BEFORE INSERT ON distinta_bozza_versione
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_versione_nasce();

-- Al COMMIT: se la fonte (allegato, sha256, gruppo) cambia rispetto alla versione precedente, che una fonte
-- l'aveva, nella nuova versione c'e' almeno un evento con motivo (dir. 06 B1 «se modificata, evento motivato»;
-- ratifica 05 §4: rebase esplicito). La prima scelta di una fonte, da nessuna fonte, non lo chiede.
CREATE FUNCTION distinta_bozza_versione_fonte_motivata() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    p record;
BEGIN
    IF NEW.versione = 1 THEN
        RETURN NULL;
    END IF;
    SELECT v.fonte_allegato_id, v.fonte_sha256, v.fonte_gruppo INTO p
      FROM distinta_bozza_versione v WHERE v.bozza_id = NEW.bozza_id AND v.versione = NEW.versione - 1;
    IF FOUND AND p.fonte_allegato_id IS NOT NULL
       AND (p.fonte_allegato_id, p.fonte_sha256, p.fonte_gruppo)
           IS DISTINCT FROM (NEW.fonte_allegato_id, NEW.fonte_sha256, NEW.fonte_gruppo)
       AND NOT EXISTS (SELECT 1 FROM distinta_bozza_evento e
                        WHERE e.bozza_id = NEW.bozza_id AND e.versione = NEW.versione AND e.motivo IS NOT NULL) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s: la fonte cambia dalla versione %s alla %s senza un evento motivato',
                             NEW.bozza_id, NEW.versione - 1, NEW.versione);
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER trg_dbv_fonte_motivata AFTER INSERT ON distinta_bozza_versione
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION distinta_bozza_versione_fonte_motivata();

-- Gli eventi si aggiungono solo a una bozza aperta, alla sua versione corrente, nella transazione in cui
-- quella versione e' stata salvata: cosi' il giornale e gli snapshot non divergono (ratifica 05 §2).
CREATE FUNCTION distinta_bozza_evento_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    b record;
BEGIN
    SELECT x.stato, x.versione, v.tx INTO b
      FROM distinta_bozza x
      LEFT JOIN distinta_bozza_versione v ON v.bozza_id = x.bozza_id AND v.versione = x.versione
     WHERE x.bozza_id = NEW.bozza_id;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- lo rifiuta la FK
    END IF;
    IF b.stato <> 'aperta' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s e'' %s: il giornale e'' chiuso', NEW.bozza_id, b.stato);
    END IF;
    IF NEW.versione <> b.versione OR b.tx IS DISTINCT FROM pg_current_xact_id() THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X04',
            MESSAGE = format('distinta_bozza %s: l''evento %s porta la versione %s; si aggiunge solo alla versione corrente (%s) nella transazione che l''ha salvata',
                             NEW.bozza_id, NEW.n, NEW.versione, b.versione);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_dbe_nasce BEFORE INSERT ON distinta_bozza_evento
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_evento_nasce();

-- Versioni ed eventi: mai UPDATE; DELETE solo dentro la cascata della cancellazione di una bozza abbandonata
-- da oltre 90 giorni: la testata, a quel punto, non si vede piu' (il suo trigger ha gia' controllato stato e
-- conservazione). Con la testata ancora visibile il DELETE diretto si rifiuta sempre: niente storia bucata.
CREATE FUNCTION distinta_bozza_figlia_cancellabile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM distinta_bozza x WHERE x.bozza_id = OLD.bozza_id) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = format('%s della bozza %s: si cancella solo con la bozza, abbandonata da oltre 90 giorni', TG_TABLE_NAME, OLD.bozza_id);
    END IF;
    RETURN OLD;
END $$;
CREATE TRIGGER trg_dbv_solo_aggiunte BEFORE UPDATE ON distinta_bozza_versione
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'una versione salvata della bozza non si riscrive');
CREATE TRIGGER trg_dbv_cancellabile BEFORE DELETE ON distinta_bozza_versione
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_figlia_cancellabile();
CREATE TRIGGER trg_dbv_non_si_svuota BEFORE TRUNCATE ON distinta_bozza_versione
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le versioni delle bozze non si svuotano');
CREATE TRIGGER trg_dbe_solo_aggiunte BEFORE UPDATE ON distinta_bozza_evento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'un evento della bozza non si riscrive');
CREATE TRIGGER trg_dbe_cancellabile BEFORE DELETE ON distinta_bozza_evento
    FOR EACH ROW EXECUTE FUNCTION distinta_bozza_figlia_cancellabile();
CREATE TRIGGER trg_dbe_non_si_svuota BEFORE TRUNCATE ON distinta_bozza_evento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'il giornale delle bozze non si svuota');

-- ============================================================ 5. ESITO DEGLI INTENTI E STATO DI F8
-- L'esito della transazione per chiave d'intento (contratto v2 §0 n. 7, §4.3 n. 2 e 14): scritto prima del
-- COMMIT, nella stessa transazione; una risposta persa si recupera con R6, mai una seconda conferma alla
-- cieca. Contiene solo successi: un rifiuto non arriva al COMMIT. Non si modifica; si cancella oltre la
-- conservazione (90 giorni, piano §3.5; creato_il la scrive il database): la conferma resta, con la sua
-- chiave_idempotenza, e basta a dire «registrato» anche dopo (contratto v2 rev. 3).
CREATE TABLE esito_intento (
    chiave         uuid PRIMARY KEY,
    thread_id      uuid NOT NULL REFERENCES thread_offerta,
    gesto          text NOT NULL,
    utente_id      uuid NOT NULL REFERENCES utente,
    impronta_corpo char(64) NOT NULL,
    esito          jsonb NOT NULL,                          -- la risposta 200 intera
    conferma_id    uuid,
    creato_il      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fk_esito_intento_conferma FOREIGN KEY (thread_id, conferma_id)
        REFERENCES conferma_strutturale (thread_id, conferma_id),
    -- la conferma citata ha la stessa chiave d'intento dell'esito
    CONSTRAINT fk_esito_intento_chiave FOREIGN KEY (conferma_id, chiave)
        REFERENCES conferma_strutturale (conferma_id, chiave_idempotenza),
    -- conferma_bom_v2 R5, lotto_fascicolo R9, correggi_documento R10, fonte_step (B4: scelta, rifiuto,
    -- sostituzione della fonte), riprova_copia (B4: ritentativo NAS mirato per documento)
    CONSTRAINT ck_esito_intento_gesto CHECK (gesto IN ('conferma_bom_v2', 'lotto_fascicolo', 'correggi_documento', 'fonte_step', 'riprova_copia')),
    CONSTRAINT ck_esito_intento_conferma CHECK ((gesto = 'conferma_bom_v2') = (conferma_id IS NOT NULL)),
    CONSTRAINT ck_esito_intento_impronta CHECK (impronta_corpo ~ '^[0-9a-f]{64}$'),
    -- 16 MiB come delta: la risposta di R5 lo puo' contenere
    CONSTRAINT ck_esito_intento_esito CHECK (jsonb_typeof(esito) = 'object' AND pg_column_size(esito) <= 16777216)
);
CREATE INDEX ix_esito_intento_thread ON esito_intento (thread_id, creato_il);
CREATE UNIQUE INDEX ux_esito_intento_conferma ON esito_intento (conferma_id) WHERE conferma_id IS NOT NULL;
CREATE TRIGGER trg_esito_intento_ora BEFORE INSERT ON esito_intento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('creato_il');

CREATE FUNCTION esito_intento_conservazione() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X03',
            MESSAGE = format('esito_intento %s: l''esito registrato non cambia (lo stato di F8 sta in f8_stato)', OLD.chiave);
    END IF;
    IF OLD.creato_il >= now() - interval '90 days' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X03',
            MESSAGE = format('esito_intento %s: si cancella solo oltre la conservazione di 90 giorni', OLD.chiave);
    END IF;
    RETURN OLD;
END $$;
CREATE TRIGGER trg_esito_intento_conservazione BEFORE UPDATE OR DELETE ON esito_intento
    FOR EACH ROW EXECUTE FUNCTION esito_intento_conservazione();
CREATE TRIGGER trg_esito_intento_non_si_svuota BEFORE TRUNCATE ON esito_intento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X03', 'gli esiti degli intenti non si svuotano');

-- F8 gira DOPO il COMMIT, nella stessa richiesta, fuori dalla transazione (contratto v2 §4.3 n. 16, R-15):
-- la riga nasce in_attesa nella transazione del gesto e cambia con una scrittura propria. Separata da
-- esito_intento, cosi' l'esito resta immutabile, e vale anche per la conferma formato 1, che non ha intento.
-- Nessuna riga = F8 non previsto. chiave_intento e' storica, senza FK: la conservazione degli esiti non deve
-- portare via uno stato F8 (in attesa, in errore, o legato a una conferma); f8_stato non si cancella. Il Go
-- scrive l'esito e la riga di F8 nella stessa transazione.
CREATE TABLE f8_stato (
    f8_id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id      uuid NOT NULL REFERENCES thread_offerta,
    chiave_intento uuid UNIQUE,
    conferma_id    uuid UNIQUE,
    stato          text NOT NULL DEFAULT 'in_attesa',
    errore         text,
    tentativi      int NOT NULL DEFAULT 0,
    risolto_da     text,                                    -- chi l'ha portato a eseguito
    creato_il      timestamptz NOT NULL DEFAULT now(),
    aggiornato_il  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fk_f8_conferma FOREIGN KEY (thread_id, conferma_id)
        REFERENCES conferma_strutturale (thread_id, conferma_id),
    CONSTRAINT ck_f8_oggetto CHECK (num_nonnulls(chiave_intento, conferma_id) >= 1),
    CONSTRAINT ck_f8_stato CHECK (stato IN ('in_attesa', 'eseguito', 'errore')),
    CONSTRAINT ck_f8_errore CHECK ((stato = 'errore') = (errore IS NOT NULL)
        AND (errore IS NULL OR (btrim(errore) <> '' AND char_length(errore) <= 2000))),
    CONSTRAINT ck_f8_tentativi CHECK (tentativi >= 0),
    -- stessa_richiesta: F8 dopo il COMMIT; f8_successivo: un F8 riuscito dell'intera RFQ, iniziato dopo;
    -- riallinea: R11 (admin)
    CONSTRAINT ck_f8_risolto CHECK ((stato = 'eseguito') = (risolto_da IS NOT NULL)
        AND (risolto_da IS NULL OR risolto_da IN ('stessa_richiesta', 'f8_successivo', 'riallinea')))
);
CREATE INDEX ix_f8_stato_pendenti ON f8_stato (thread_id, creato_il) WHERE stato <> 'eseguito';
CREATE TRIGGER trg_f8_stato_ora BEFORE INSERT ON f8_stato
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('creato_il', 'aggiornato_il');

-- eseguito e' finale; RFQ, intento, conferma e creazione non cambiano; i tentativi non calano; aggiornato_il
-- la scrive il database. Non si cancella.
CREATE FUNCTION f8_stato_transizione() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X03',
            MESSAGE = format('f8_stato %s: lo stato di F8 non si cancella', OLD.f8_id);
    END IF;
    IF OLD.stato = 'eseguito'
       OR (NEW.f8_id, NEW.thread_id, NEW.chiave_intento, NEW.conferma_id, NEW.creato_il)
          IS DISTINCT FROM (OLD.f8_id, OLD.thread_id, OLD.chiave_intento, OLD.conferma_id, OLD.creato_il)
       OR NEW.tentativi < OLD.tentativi THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X03',
            MESSAGE = format('f8_stato %s: transizione non ammessa (%s → %s)', OLD.f8_id, OLD.stato, NEW.stato);
    END IF;
    NEW.aggiornato_il := now();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_f8_stato_transizione BEFORE UPDATE OR DELETE ON f8_stato
    FOR EACH ROW EXECUTE FUNCTION f8_stato_transizione();
CREATE TRIGGER trg_f8_stato_non_si_svuota BEFORE TRUNCATE ON f8_stato
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X03', 'lo stato di F8 non si svuota');

-- ============================================================ 6. CORREZIONI, ASSOCIAZIONI, FONTE STEP
-- Audit delle correzioni di codice/rev (Q-P3-03, contratto v2 §9.2): il valore letto all'origine, prima,
-- dopo, motivo, autore, ora. motivo obbligatorio per R10 (origine motore_a); la ricodifica di un componente
-- (rinomina_componente) e quella della conferma BOM (ricodifica_bom) scrivono una riga per proposta toccata.
CREATE TABLE correzione_documento (
    correzione_id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id          uuid NOT NULL REFERENCES thread_offerta,
    proposta_id        uuid REFERENCES documento_proposta,
    documento_id       uuid,
    -- il componente della proposta in quel momento, se c'era: storico, senza FK (come in associazione_evento),
    -- perche' una riga di audit non deve impedire di togliere un componente mai confermato
    componente_id      uuid,
    allegato_id        uuid NOT NULL REFERENCES allegato,
    nome_file          varchar(300) NOT NULL,
    sha256             char(64) NOT NULL,
    codice_letto       varchar(60),
    rev_letta          varchar(10),
    codice_prima       varchar(60),
    rev_prima          varchar(10),
    codice_dopo        varchar(60),
    rev_dopo           varchar(10),
    motivo             text,
    origine            text NOT NULL,
    chiave_idempotenza uuid,
    autore_id          uuid NOT NULL REFERENCES utente,
    creato_il          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fk_correzione_documento FOREIGN KEY (thread_id, documento_id)
        REFERENCES documento (thread_id, documento_id),
    CONSTRAINT ck_correzione_oggetto CHECK (proposta_id IS NOT NULL OR documento_id IS NOT NULL),
    CONSTRAINT ck_correzione_origine CHECK (origine IN ('motore_a', 'assegna_legacy', 'decidi_legacy', 'rinomina_componente', 'ricodifica_bom')),
    CONSTRAINT ck_correzione_motivo CHECK ((origine <> 'motore_a' OR nullif(btrim(motivo), '') IS NOT NULL)
        AND (motivo IS NULL OR char_length(motivo) <= 500)),
    CONSTRAINT ck_correzione_sha CHECK (sha256 ~ '^[0-9a-f]{64}$')
);
CREATE INDEX ix_correzione_thread ON correzione_documento (thread_id, creato_il);
CREATE INDEX ix_correzione_proposta ON correzione_documento (proposta_id) WHERE proposta_id IS NOT NULL;
CREATE INDEX ix_correzione_documento ON correzione_documento (documento_id) WHERE documento_id IS NOT NULL;
CREATE TRIGGER trg_correzione_ora BEFORE INSERT ON correzione_documento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('creato_il');

-- Il «registro del momento 1» (R-05, contratto v2 §7.1 registro_momento1): ogni associazione, da tutte le
-- strade, legacy comprese, con chi e quando. Gli scrittori sono di B4 (piano B4.2, B4.3): fino ad allora nessuna
-- strada lo scrive, e la storia comincia dal primo evento scritto da B4 (revisione «scrittori», RSC-06).
-- componente_prima / componente_dopo sono storici, senza FK: la storia di un'associazione non deve impedire di
-- togliere un componente che non e' mai stato confermato.
CREATE TABLE associazione_evento (
    evento_id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id          uuid NOT NULL REFERENCES thread_offerta,
    proposta_id        uuid REFERENCES documento_proposta,
    documento_id       uuid,
    azione             text NOT NULL,
    componente_prima   uuid,
    componente_dopo    uuid,
    conferma_id        uuid,                                -- la conferma BOM che ha applicato una preassociazione
    motivo             text,
    origine            text NOT NULL,
    chiave_idempotenza uuid,
    autore_id          uuid NOT NULL REFERENCES utente,
    quando             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fk_associazione_documento FOREIGN KEY (thread_id, documento_id)
        REFERENCES documento (thread_id, documento_id),
    CONSTRAINT fk_associazione_conferma FOREIGN KEY (thread_id, conferma_id)
        REFERENCES conferma_strutturale (thread_id, conferma_id),
    CONSTRAINT ck_associazione_oggetto CHECK (proposta_id IS NOT NULL OR documento_id IS NOT NULL),
    CONSTRAINT ck_associazione_azione CHECK (azione IN ('assegna', 'sgancia', 'scarta', 'riapri', 'preassociazione', 'caricamento', 'sposta_documento')),
    CONSTRAINT ck_associazione_origine CHECK (origine IN ('ponte', 'legacy', 'conferma_bom', 'inbox')),
    CONSTRAINT ck_associazione_coerente CHECK (
        (azione NOT IN ('assegna', 'preassociazione', 'sposta_documento') OR componente_dopo IS NOT NULL)
        AND (azione <> 'sgancia' OR (componente_prima IS NOT NULL AND componente_dopo IS NULL))
        AND (azione <> 'preassociazione' OR (conferma_id IS NOT NULL AND origine = 'conferma_bom'))),
    CONSTRAINT ck_associazione_motivo CHECK (motivo IS NULL OR (btrim(motivo) <> '' AND char_length(motivo) <= 1000))
);
CREATE INDEX ix_associazione_evento_thread ON associazione_evento (thread_id, quando);
CREATE INDEX ix_associazione_evento_proposta ON associazione_evento (proposta_id) WHERE proposta_id IS NOT NULL;
CREATE INDEX ix_associazione_evento_documento ON associazione_evento (documento_id) WHERE documento_id IS NOT NULL;
CREATE TRIGGER trg_associazione_ora BEFORE INSERT ON associazione_evento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('quando');

-- La storia della fonte STEP di un prodotto (ratifica 05 §4 [U]+[T]): vedere una proposta senza confermarla
-- non si registra qui (e' un evento della bozza); si registrano la scelta di una candidata, il suo rifiuto
-- con motivo, la sostituzione (con la fonte di prima), il caricamento di uno STEP nuovo per il prodotto, e
-- i due atti gia' esistenti che la riguardano (conferma del documento sorgente, autorizzazione/revoca della
-- struttura). La fonte precedente non si cancella mai dalla storia. bozza_id e' storico, senza FK: la
-- conservazione delle bozze abbandonate non deve toccare questa storia. Per autorizzazione e revoca fa fede
-- la storia gia' esistente delle dichiarazioni (componente_proposta.evidenza); qui ne resta l'indice per
-- prodotto, scritto nella stessa transazione. prodotto_id ha la FK: un finito con una storia della fonte
-- non si rimuove, si archivia. Gli scrittori sono quelli di B4 (le strade della fonte, legacy comprese): fino ad
-- allora nessuna strada la scrive, e la storia comincia dal primo evento scritto da B4.
--
-- Fonti secondarie (D-SHOCK-S2 B [U], direttiva 11; protocollo fisico [T], nota della 0022 §12). ruolo dice di
-- chi e' la fonte: prodotto = lo STEP del finito prodotto_id, come prima; sottoassieme = lo STEP secondario di un
-- nodo del perimetro di prodotto_id, che ha una destinazione:
--   - componente_id, quando il nodo ha gia' un UUID (FK composita: stessa RFQ; il componente diventa storia);
--   - altrimenti la destinazione temporanea (bozza_id, chiave_bozza): un nodo senza UUID della versione corrente
--     di una bozza aperta dello stesso prodotto (trigger fonte_step_evento_nasce). bozza_id resta senza FK: la
--     conservazione cancella la bozza abbandonata, la riga e distinta_bozza_registro restano; e la riga porta in
--     destinazione la copia del nodo (com'era nella versione corrente, con il numero della versione), scritta dal
--     database: anche dopo la conservazione si sa quale nodo era (codice proposto, descrizione, tipo, caselle e
--     rif del nodo STEP), non solo la sua chiave (revisione di dominio della notte, RD-01);
--   - una sola destinazione per scelta e rifiuto: o l'UUID o la chiave (RD-06);
--   - al COMMIT della conferma che certifica quella chiave, il database aggiunge per ogni evento ancora
--     temporaneo un evento 'risoluzione' (risolve_evento_id = l'evento, componente_id = l'UUID certificato, la
--     stessa destinazione): nessun UPDATE della storia (trigger differito fonte_step_risolvi su
--     conferma_strutturale_componente; differito, cosi' risolve anche un evento scritto dopo i componenti nella
--     stessa transazione, R-08). Se dopo l'evento la bozza ha aggiunto con la stessa chiave un nodo diverso da
--     quello dell'evento, la conferma si rifiuta (P4X07): la destinazione non si risolve sul nodo sbagliato
--     (R-09, RSC-02; la regola di R2 che vieta il riuso e' di B2).
-- Per una fonte secondaria si registrano solo la scelta e il rifiuto motivato (piu' la risoluzione): la
-- sostituzione e' un rifiuto piu' una scelta, l'autorizzazione F5b del sottoassieme resta un atto distinto con la
-- sua storia (componente_proposta.evidenza). fonte_rifiutata (BASE_SUPERATA), lo storico R7 della fonte e il
-- rebase del prodotto leggono solo ruolo = 'prodotto'; quelli del nodo, ruolo = 'sottoassieme' con la sua
-- destinazione.
CREATE TABLE fonte_step_evento (
    evento_id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id              uuid NOT NULL REFERENCES thread_offerta,
    prodotto_id            uuid NOT NULL,
    ruolo                  text NOT NULL DEFAULT 'prodotto',
    componente_id          uuid,                            -- sottoassieme: la destinazione con UUID
    chiave_bozza           text,                            -- sottoassieme: la destinazione temporanea, con bozza_id
    destinazione           jsonb,                           -- con chiave_bozza: {versione, nodo}, la copia del nodo scritta dal database
    risolve_evento_id      bigint,                          -- risoluzione: l'evento temporaneo che risolve
    azione                 text NOT NULL,
    allegato_id            uuid NOT NULL REFERENCES allegato,
    sha256                 char(64) NOT NULL,
    gruppo                 text,
    proposta_id            uuid REFERENCES documento_proposta,
    documento_id           uuid,
    precedente_allegato_id uuid REFERENCES allegato,
    precedente_sha256      char(64),
    precedente_gruppo      text,
    bozza_id               uuid,
    motivo                 text,
    chiave_idempotenza     uuid,
    autore_id              uuid NOT NULL REFERENCES utente,
    quando                 timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT fk_fonte_prodotto FOREIGN KEY (thread_id, prodotto_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT fk_fonte_documento FOREIGN KEY (thread_id, documento_id)
        REFERENCES documento (thread_id, documento_id),
    CONSTRAINT ux_fonte_thread_evento UNIQUE (thread_id, evento_id),           -- bersaglio di fk_fonte_risolve
    CONSTRAINT fk_fonte_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT fk_fonte_risolve FOREIGN KEY (thread_id, risolve_evento_id)
        REFERENCES fonte_step_evento (thread_id, evento_id),
    CONSTRAINT ck_fonte_azione CHECK (azione IN ('scelta', 'rifiuto', 'sostituzione', 'caricamento',
                                                 'conferma_documento', 'autorizzazione', 'revoca', 'risoluzione')),
    CONSTRAINT ck_fonte_chiave CHECK (chiave_bozza IS NULL
        OR (bozza_id IS NOT NULL AND chiave_bozza ~ '^[A-Za-z0-9._:-]{1,64}$')),
    -- prodotto: come prima della direttiva 11, nessuna destinazione propria; sottoassieme: scelta, rifiuto o
    -- risoluzione, con una destinazione (UUID o chiave della bozza, una sola per scelta e rifiuto) che non e' il
    -- prodotto stesso
    CONSTRAINT ck_fonte_ruolo CHECK (ruolo IN ('prodotto', 'sottoassieme')
        AND (ruolo = 'sottoassieme' OR (componente_id IS NULL AND chiave_bozza IS NULL AND azione <> 'risoluzione'))
        AND (ruolo = 'prodotto' OR (azione IN ('scelta', 'rifiuto', 'risoluzione')
                                    AND (componente_id IS NOT NULL OR chiave_bozza IS NOT NULL)
                                    AND (azione = 'risoluzione' OR componente_id IS NULL OR chiave_bozza IS NULL)
                                    AND componente_id IS DISTINCT FROM prodotto_id))),
    -- la destinazione temporanea porta la copia del nodo: {versione: n, nodo: {...}} (la scrive il trigger)
    CONSTRAINT ck_fonte_destinazione CHECK ((chiave_bozza IS NULL) = (destinazione IS NULL)
        AND (destinazione IS NULL OR (jsonb_typeof(destinazione) = 'object'
             AND COALESCE(jsonb_typeof(destinazione -> 'versione') = 'number', false)
             AND COALESCE(jsonb_typeof(destinazione -> 'nodo') = 'object', false)
             AND pg_column_size(destinazione) <= 4194304))),
    -- la risoluzione lega una chiave della bozza a un UUID: ha l'evento che risolve, la chiave e l'UUID
    CONSTRAINT ck_fonte_risoluzione CHECK ((azione = 'risoluzione') = (risolve_evento_id IS NOT NULL)
        AND (azione <> 'risoluzione' OR (componente_id IS NOT NULL AND chiave_bozza IS NOT NULL))),
    CONSTRAINT ck_fonte_sha CHECK (sha256 ~ '^[0-9a-f]{64}$'
        AND (precedente_allegato_id IS NULL) = (precedente_sha256 IS NULL)
        AND (precedente_sha256 IS NULL OR precedente_sha256 ~ '^[0-9a-f]{64}$')),
    CONSTRAINT ck_fonte_gruppo CHECK ((gruppo IS NULL OR (btrim(gruppo) <> '' AND char_length(gruppo) <= 300))
        AND (precedente_gruppo IS NULL OR (btrim(precedente_gruppo) <> '' AND char_length(precedente_gruppo) <= 300))),
    CONSTRAINT ck_fonte_sostituzione CHECK (azione <> 'sostituzione' OR precedente_allegato_id IS NOT NULL),
    CONSTRAINT ck_fonte_documento CHECK (azione NOT IN ('conferma_documento', 'autorizzazione', 'revoca') OR documento_id IS NOT NULL),
    -- un allegato puo' avere piu' candidate (cand:<allegato>:<radice>): scelta, rifiuto e sostituzione dicono
    -- quale, con il gruppo (bom, senza o cand:..., opaco per il database)
    CONSTRAINT ck_fonte_gruppo_obbligatorio CHECK (azione NOT IN ('scelta', 'rifiuto', 'sostituzione') OR gruppo IS NOT NULL),
    CONSTRAINT ck_fonte_motivo CHECK ((azione NOT IN ('rifiuto', 'sostituzione', 'revoca') OR nullif(btrim(motivo), '') IS NOT NULL)
        AND (motivo IS NULL OR char_length(motivo) <= 1000))
);
CREATE INDEX ix_fonte_step_prodotto ON fonte_step_evento (prodotto_id, quando);
CREATE INDEX ix_fonte_step_allegato ON fonte_step_evento (allegato_id);
CREATE INDEX ix_fonte_step_thread ON fonte_step_evento (thread_id, quando);
CREATE INDEX ix_fonte_step_componente ON fonte_step_evento (componente_id, quando) WHERE componente_id IS NOT NULL;
CREATE INDEX ix_fonte_step_destinazione ON fonte_step_evento (bozza_id, chiave_bozza) WHERE chiave_bozza IS NOT NULL;
-- un evento temporaneo si risolve una volta sola
CREATE UNIQUE INDEX ux_fonte_step_risolve ON fonte_step_evento (risolve_evento_id) WHERE risolve_evento_id IS NOT NULL;
CREATE TRIGGER trg_fonte_step_ora BEFORE INSERT ON fonte_step_evento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('quando');

-- La destinazione temporanea e' esplicita e verificabile: la bozza esiste, e' aperta, e' dello stesso prodotto
-- e della stessa RFQ, e la chiave e' un nodo della sua versione corrente che non ha ancora un UUID (con un UUID
-- la destinazione e' componente_id). La copia del nodo (destinazione) la scrive il database, qualunque valore porti
-- il chiamante. La bozza si legge con FOR SHARE: una conferma concorrente che la sta chiudendo fa aspettare, e poi
-- l'evento trova la bozza confermata (P4X07) invece di entrare dopo la risoluzione (R-07). La risoluzione non si
-- scrive a mano: la scrive fonte_step_risolvi, con la destinazione dell'evento che risolve.
CREATE FUNCTION fonte_step_evento_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    b    record;
    nodo jsonb;
BEGIN
    IF NEW.azione = 'risoluzione' THEN
        IF pg_trigger_depth() < 2 THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X07',
                MESSAGE = 'fonte_step_evento: la risoluzione di una destinazione temporanea la scrive solo la conferma strutturale';
        END IF;
        NEW.destinazione := (SELECT e.destinazione FROM fonte_step_evento e WHERE e.evento_id = NEW.risolve_evento_id);
        RETURN NEW;
    END IF;
    IF NEW.chiave_bozza IS NULL THEN
        NEW.destinazione := NULL;
        RETURN NEW;
    END IF;
    SELECT x.stato, x.thread_id, x.prodotto_id, x.versione, v.snapshot INTO b
      FROM distinta_bozza x
      LEFT JOIN distinta_bozza_versione v ON v.bozza_id = x.bozza_id AND v.versione = x.versione
     WHERE x.bozza_id = NEW.bozza_id
       FOR SHARE OF x;
    IF NOT FOUND OR b.stato <> 'aperta' OR b.thread_id <> NEW.thread_id OR b.prodotto_id <> NEW.prodotto_id THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X07',
            MESSAGE = format('fonte_step_evento: la destinazione temporanea %s vuole la bozza %s aperta, del prodotto %s e della RFQ %s',
                             NEW.chiave_bozza, NEW.bozza_id, NEW.prodotto_id, NEW.thread_id);
    END IF;
    SELECT n.nodo INTO nodo
      FROM jsonb_array_elements(CASE WHEN jsonb_typeof(b.snapshot -> 'nodi') = 'array'
                                     THEN b.snapshot -> 'nodi' ELSE '[]'::jsonb END) AS n (nodo)
     WHERE n.nodo ->> 'chiave_bozza' = NEW.chiave_bozza
       AND jsonb_typeof(n.nodo -> 'componente_id') IS DISTINCT FROM 'string'
     LIMIT 1;
    IF NOT FOUND OR jsonb_typeof(nodo) IS DISTINCT FROM 'object' THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X07',
            MESSAGE = format('fonte_step_evento: %s non e'' un nodo senza UUID della versione corrente della bozza %s',
                             NEW.chiave_bozza, NEW.bozza_id),
            HINT = 'con un UUID la destinazione e'' componente_id; un nodo nuovo va prima salvato nella bozza';
    END IF;
    NEW.destinazione := jsonb_build_object('versione', b.versione, 'nodo', nodo);
    RETURN NEW;
END $$;
CREATE TRIGGER trg_fonte_step_nasce BEFORE INSERT ON fonte_step_evento
    FOR EACH ROW EXECUTE FUNCTION fonte_step_evento_nasce();

-- La risoluzione al COMMIT (D-SHOCK-S2: «mappa di risoluzione al COMMIT», «senza UPDATE dello storico»): quando
-- una conferma che chiude una bozza certifica un componente con la sua chiave, ogni evento secondario ancora
-- temporaneo su (quella bozza, quella chiave) riceve un evento 'risoluzione' con l'UUID certificato, l'autore
-- della conferma e la data del database. Lo scrive il database, nella transazione della conferma: il Go non lo
-- puo' dimenticare. Una chiave che la conferma non certifica (nodo tolto prima della conferma, bozza abbandonata)
-- resta temporanea: la storia si legge con distinta_bozza_registro e con la copia del nodo (destinazione).
-- Il trigger e' differito: gira al COMMIT (o al SET CONSTRAINTS ... IMMEDIATE), quando tutti gli eventi della
-- transazione ci sono. Prima di risolvere controlla che la chiave sia rimasta lo stesso nodo: se il giornale della
-- bozza ha, dopo la versione dell'evento, un nodo_aggiunto con quella chiave e un nodo diverso dalla copia
-- dell'evento (vocabolario 1 del giornale, distintabozza), la conferma si rifiuta.
CREATE FUNCTION fonte_step_risolvi() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE CONSTRAINT TRIGGER trg_csc_risolve_fonte AFTER INSERT ON conferma_strutturale_componente
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fonte_step_risolvi();

CREATE TRIGGER trg_correzione_solo_aggiunte BEFORE UPDATE OR DELETE ON correzione_documento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'una correzione registrata non si modifica e non si cancella');
CREATE TRIGGER trg_correzione_non_si_svuota BEFORE TRUNCATE ON correzione_documento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le correzioni non si svuotano');
CREATE TRIGGER trg_associazione_solo_aggiunte BEFORE UPDATE OR DELETE ON associazione_evento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'un evento di associazione non si modifica e non si cancella');
CREATE TRIGGER trg_associazione_non_si_svuota BEFORE TRUNCATE ON associazione_evento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le associazioni non si svuotano');
CREATE TRIGGER trg_fonte_step_solo_aggiunte BEFORE UPDATE OR DELETE ON fonte_step_evento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia della fonte STEP non si modifica e non si cancella');
CREATE TRIGGER trg_fonte_step_non_si_svuota BEFORE TRUNCATE ON fonte_step_evento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia della fonte STEP non si svuota');

-- ============================================================ 7. ATTESTAZIONE DEL FASCICOLO
-- Nasce SOLO dal campo esplicito attesta di R9 (R-06), mai a ogni lotto; sull'ultima conferma strutturale
-- del prodotto; progressiva e immutabile. «Da riverificare» non si scrive: si deriva (piano §3.4). requisiti
-- e' la tabella per nodo della slide 2 (esiti e deroghe) che l'operatore ha visto con firma_piano.
CREATE TABLE attestazione_fascicolo (
    attestazione_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id               uuid NOT NULL REFERENCES thread_offerta,
    prodotto_id             uuid NOT NULL,
    numero                  int NOT NULL,
    precedente_id           uuid,
    conferma_strutturale_id uuid NOT NULL,
    firma_piano             char(64) NOT NULL,
    requisiti               jsonb NOT NULL DEFAULT '[]',
    chiave_idempotenza      uuid,
    attestato_da            uuid NOT NULL REFERENCES utente,
    attestato_il            timestamptz NOT NULL DEFAULT now(),
    tx                      xid8 NOT NULL DEFAULT pg_current_xact_id(),
    CONSTRAINT fk_attestazione_prodotto FOREIGN KEY (thread_id, prodotto_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT fk_attestazione_conferma FOREIGN KEY (conferma_strutturale_id, prodotto_id)
        REFERENCES conferma_strutturale (conferma_id, prodotto_id),
    CONSTRAINT ux_attestazione_prodotto_numero UNIQUE (prodotto_id, numero),
    CONSTRAINT ux_attestazione_thread_id UNIQUE (thread_id, attestazione_id),
    CONSTRAINT ux_attestazione_id_prodotto UNIQUE (attestazione_id, prodotto_id),
    CONSTRAINT fk_attestazione_precedente FOREIGN KEY (precedente_id, prodotto_id)
        REFERENCES attestazione_fascicolo (attestazione_id, prodotto_id),
    CONSTRAINT ck_attestazione_numero CHECK (numero >= 1 AND (numero = 1) = (precedente_id IS NULL)),
    CONSTRAINT ck_attestazione_firma CHECK (firma_piano ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_attestazione_requisiti CHECK (jsonb_typeof(requisiti) = 'array' AND pg_column_size(requisiti) <= 4194304)
);
CREATE INDEX ix_attestazione_conferma ON attestazione_fascicolo (conferma_strutturale_id);

-- I documenti attestati, com'erano. La FK verso documento e' per RFQ e id, non per componente: come
-- bom_versione_documento, un'attestazione vecchia non deve impedire di correggere un'assegnazione.
CREATE TABLE attestazione_fascicolo_documento (
    attestazione_id uuid NOT NULL,
    thread_id       uuid NOT NULL,
    documento_id    uuid NOT NULL,
    componente_id   uuid NOT NULL,
    tipo            tipo_documento NOT NULL,
    rev             varchar(10),
    sha256          char(64) NOT NULL,
    PRIMARY KEY (attestazione_id, documento_id),
    CONSTRAINT fk_afd_attestazione FOREIGN KEY (thread_id, attestazione_id)
        REFERENCES attestazione_fascicolo (thread_id, attestazione_id),
    CONSTRAINT fk_afd_documento FOREIGN KEY (thread_id, documento_id)
        REFERENCES documento (thread_id, documento_id),
    CONSTRAINT ck_afd_sha CHECK (sha256 ~ '^[0-9a-f]{64}$')
);
CREATE INDEX ix_afd_documento ON attestazione_fascicolo_documento (documento_id);

-- Numero progressivo per prodotto, sulla conferma ULTIMA del prodotto (la validita' per contatore o firma la
-- controlla il Go sotto lucchetto); i documenti solo nella stessa transazione, correnti e uguali al documento.
CREATE FUNCTION attestazione_fascicolo_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    ultimo_id     uuid;
    ultimo_numero int;
    ultima_conf   uuid;
BEGIN
    SELECT x.attestazione_id, x.numero INTO ultimo_id, ultimo_numero
      FROM attestazione_fascicolo x WHERE x.prodotto_id = NEW.prodotto_id ORDER BY x.numero DESC LIMIT 1;
    IF NEW.numero <> COALESCE(ultimo_numero, 0) + 1 OR NEW.precedente_id IS DISTINCT FROM ultimo_id THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = format('attestazione di %s: attesa la n. %s, arrivata la n. %s', NEW.prodotto_id, COALESCE(ultimo_numero, 0) + 1, NEW.numero);
    END IF;
    SELECT x.conferma_id INTO ultima_conf FROM conferma_strutturale x WHERE x.prodotto_id = NEW.prodotto_id ORDER BY x.numero DESC LIMIT 1;
    IF ultima_conf IS DISTINCT FROM NEW.conferma_strutturale_id THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = format('attestazione di %s: si attesta sull''ultima conferma strutturale (%s), non su %s',
                             NEW.prodotto_id, COALESCE(ultima_conf::text, 'nessuna'), NEW.conferma_strutturale_id);
    END IF;
    NEW.tx := pg_current_xact_id();
    NEW.attestato_il := now();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_attestazione_nasce BEFORE INSERT ON attestazione_fascicolo
    FOR EACH ROW EXECUTE FUNCTION attestazione_fascicolo_nasce();

CREATE FUNCTION attestazione_documento_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    h  xid8;
    d  record;
BEGIN
    SELECT x.tx INTO h FROM attestazione_fascicolo x WHERE x.attestazione_id = NEW.attestazione_id;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- lo rifiuta la FK
    END IF;
    IF h <> pg_current_xact_id() THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = format('attestazione %s: i documenti si scrivono solo nella sua transazione', NEW.attestazione_id);
    END IF;
    SELECT k.componente_id, k.tipo, k.rev, k.sha256, k.sostituito_da INTO d FROM documento k WHERE k.documento_id = NEW.documento_id;
    IF FOUND AND ((d.componente_id, d.tipo, d.rev, d.sha256) IS DISTINCT FROM (NEW.componente_id, NEW.tipo, NEW.rev, NEW.sha256)
                  OR d.sostituito_da IS NOT NULL) THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = format('attestazione %s: il documento %s non e'' corrente o non e'' quello registrato', NEW.attestazione_id, NEW.documento_id);
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_afd_nasce BEFORE INSERT ON attestazione_fascicolo_documento
    FOR EACH ROW EXECUTE FUNCTION attestazione_documento_nasce();

CREATE TRIGGER trg_attestazione_solo_aggiunte BEFORE UPDATE OR DELETE ON attestazione_fascicolo
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'un''attestazione non si modifica e non si cancella; se ne registra una nuova');
CREATE TRIGGER trg_attestazione_non_si_svuota BEFORE TRUNCATE ON attestazione_fascicolo
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le attestazioni non si svuotano');
CREATE TRIGGER trg_afd_solo_aggiunte BEFORE UPDATE OR DELETE ON attestazione_fascicolo_documento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'i documenti di un''attestazione non cambiano');
CREATE TRIGGER trg_afd_non_si_svuota BEFORE TRUNCATE ON attestazione_fascicolo_documento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le attestazioni non si svuotano');

-- ============================================================ 8. STORIA DELLE DEROGHE (PD-20)
-- Copia append-only di ogni INSERT, UPDATE (che cambia qualcosa) e DELETE su deroga_fabbisogno, scritta dal
-- trigger: la vedono tutte le strade, legacy comprese. La regola della deroga non cambia. Gli id sono
-- storici, senza FK. autore_id: quello che il Go dichiara per la transazione, se c'e' ed e' un UUID, SOLO
-- con set_config('cockpit.autore_id', $1, true) (o SET LOCAL) dentro la transazione: un SET di sessione
-- resterebbe sulla connessione del pool e attribuirebbe le scritture successive alla persona sbagliata.
-- Senza dichiarazione: chi ha concesso la deroga per un INSERT; NULL per un UPDATE o una revoca, e lo si dice.
-- La tabella la scrive solo il trigger (P4X02 per un INSERT diretto); data e transazione le scrive il database.
-- deroga_struttura (0020) ha la sua storia piu' sotto (deroga_struttura_storia): con D-SHOCK-S1 B regge la verifica
-- positiva quando lo STEP non determina la gerarchia.
CREATE TABLE deroga_fabbisogno_storia (
    storia_id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operazione    text NOT NULL,
    deroga_id     uuid NOT NULL,
    thread_id     uuid NOT NULL,
    componente_id uuid NOT NULL,
    tipo          tipo_documento NOT NULL,
    prima         jsonb,
    dopo          jsonb,
    autore_id     uuid,
    tx            xid8 NOT NULL DEFAULT pg_current_xact_id(),
    quando        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_dfs_operazione CHECK (operazione IN ('INSERT', 'UPDATE', 'DELETE')
        AND (prima IS NULL) = (operazione = 'INSERT')
        AND (dopo IS NULL) = (operazione = 'DELETE')),
    CONSTRAINT ck_dfs_righe CHECK ((prima IS NULL OR (jsonb_typeof(prima) = 'object' AND pg_column_size(prima) <= 65536))
        AND (dopo IS NULL OR (jsonb_typeof(dopo) = 'object' AND pg_column_size(dopo) <= 65536)))
);
CREATE INDEX ix_dfs_componente ON deroga_fabbisogno_storia (componente_id, quando);
CREATE INDEX ix_dfs_thread ON deroga_fabbisogno_storia (thread_id, quando);

CREATE FUNCTION deroga_fabbisogno_registra() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    dichiarato text := current_setting('cockpit.autore_id', true);
    autore     uuid;
    r          deroga_fabbisogno;
BEGIN
    IF TG_OP = 'UPDATE' AND NEW IS NOT DISTINCT FROM OLD THEN
        RETURN NULL;
    END IF;
    IF dichiarato ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$' THEN
        autore := dichiarato::uuid;
    ELSIF TG_OP = 'INSERT' THEN
        autore := NEW.utente_id;                       -- chi concede; un UPDATE senza dichiarazione resta NULL
    END IF;
    IF TG_OP = 'DELETE' THEN r := OLD; ELSE r := NEW; END IF;
    INSERT INTO deroga_fabbisogno_storia (operazione, deroga_id, thread_id, componente_id, tipo, prima, dopo, autore_id)
    VALUES (TG_OP, r.deroga_id, r.thread_id, r.componente_id, r.tipo,
            CASE WHEN TG_OP <> 'INSERT' THEN to_jsonb(OLD) END,
            CASE WHEN TG_OP <> 'DELETE' THEN to_jsonb(NEW) END,
            autore);
    RETURN NULL;
END $$;
CREATE TRIGGER trg_deroga_fabbisogno_storia AFTER INSERT OR UPDATE OR DELETE ON deroga_fabbisogno
    FOR EACH ROW EXECUTE FUNCTION deroga_fabbisogno_registra();

-- Una riga di storia nasce solo dal trigger di deroga_fabbisogno (profondita' 2: il trigger di qui gira dentro
-- quello), mai da un INSERT diretto; data e transazione sono quelle vere.
CREATE FUNCTION deroga_fabbisogno_storia_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF pg_trigger_depth() < 2 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = 'deroga_fabbisogno_storia: la scrive solo il trigger di deroga_fabbisogno';
    END IF;
    NEW.quando := now();
    NEW.tx := pg_current_xact_id();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_dfs_nasce BEFORE INSERT ON deroga_fabbisogno_storia
    FOR EACH ROW EXECUTE FUNCTION deroga_fabbisogno_storia_nasce();

CREATE TRIGGER trg_dfs_solo_aggiunte BEFORE UPDATE OR DELETE ON deroga_fabbisogno_storia
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia delle deroghe non si modifica e non si cancella');
CREATE TRIGGER trg_dfs_non_si_svuota BEFORE TRUNCATE ON deroga_fabbisogno_storia
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia delle deroghe non si svuota');

-- La storia delle deroghe strutturali (revisione «scrittori» della notte, RSC-05). Con D-SHOCK-S1 B [U] la
-- verifica positiva di una conferma il cui STEP non determina la gerarchia poggia su una deroga_struttura motivata
-- e valida per quello STEP; la revoca e' un DELETE (DeleteDerogaStruttura) e la 0020 non ne tiene traccia. Qui una
-- copia append-only di ogni concessione (INSERT) e revoca (DELETE), con la riga intera com'era: chi l'ha concessa,
-- perche', per quale STEP, sha e analisi; chi l'ha revocata e quando. La deroga non si modifica (0020, BOM05),
-- quindi non ci sono UPDATE. autore_id come per deroga_fabbisogno_storia: dichiarato dal Go con
-- set_config('cockpit.autore_id', ..., true); senza dichiarazione, chi l'ha concessa per un INSERT, NULL per una
-- revoca. La revoca non ha un motivo nella 0020: lo portera' la rotta che la scrive. Gli id sono storici, senza FK.
CREATE TABLE deroga_struttura_storia (
    storia_id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operazione          text NOT NULL,
    deroga_struttura_id uuid NOT NULL,
    thread_id           uuid NOT NULL,
    componente_id       uuid NOT NULL,
    step_documento_id   uuid NOT NULL,
    step_sha256         char(64) NOT NULL,
    riga                jsonb NOT NULL,
    autore_id           uuid,
    tx                  xid8 NOT NULL DEFAULT pg_current_xact_id(),
    quando              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_dss_operazione CHECK (operazione IN ('INSERT', 'DELETE')),
    CONSTRAINT ck_dss_riga CHECK (jsonb_typeof(riga) = 'object' AND pg_column_size(riga) <= 65536)
);
CREATE INDEX ix_dss_componente ON deroga_struttura_storia (componente_id, quando);
CREATE INDEX ix_dss_thread ON deroga_struttura_storia (thread_id, quando);

CREATE FUNCTION deroga_struttura_registra() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    dichiarato text := current_setting('cockpit.autore_id', true);
    autore     uuid;
    r          deroga_struttura;
BEGIN
    IF dichiarato ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$' THEN
        autore := dichiarato::uuid;
    ELSIF TG_OP = 'INSERT' THEN
        autore := NEW.concessa_da;                     -- chi concede; una revoca senza dichiarazione resta NULL
    END IF;
    IF TG_OP = 'DELETE' THEN r := OLD; ELSE r := NEW; END IF;
    INSERT INTO deroga_struttura_storia (operazione, deroga_struttura_id, thread_id, componente_id, step_documento_id, step_sha256, riga, autore_id)
    VALUES (TG_OP, r.deroga_struttura_id, r.thread_id, r.componente_id, r.step_documento_id, r.step_sha256, to_jsonb(r), autore);
    RETURN NULL;
END $$;
CREATE TRIGGER trg_deroga_struttura_storia AFTER INSERT OR DELETE ON deroga_struttura
    FOR EACH ROW EXECUTE FUNCTION deroga_struttura_registra();

-- Una riga nasce solo dal trigger di deroga_struttura (profondita' 2), mai da un INSERT diretto.
CREATE FUNCTION deroga_struttura_storia_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF pg_trigger_depth() < 2 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X02',
            MESSAGE = 'deroga_struttura_storia: la scrive solo il trigger di deroga_struttura';
    END IF;
    NEW.quando := now();
    NEW.tx := pg_current_xact_id();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_dss_nasce BEFORE INSERT ON deroga_struttura_storia
    FOR EACH ROW EXECUTE FUNCTION deroga_struttura_storia_nasce();
CREATE TRIGGER trg_dss_solo_aggiunte BEFORE UPDATE OR DELETE ON deroga_struttura_storia
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia delle deroghe strutturali non si modifica e non si cancella');
CREATE TRIGGER trg_dss_non_si_svuota BEFORE TRUNCATE ON deroga_struttura_storia
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'la storia delle deroghe strutturali non si svuota');

-- ============================================================ 9. COPERTURA DOCUMENTALE CONDIVISA
-- D-SHOCK-DOC-01 A [U] (direttiva 11). Un documento ufficiale (una riga di documento, un solo file sul NAS) e'
-- l'evidenza del requisito disegno_2d di ALTRI componenti della stessa RFQ, per alcune pagine: il PDF
-- multipagina di un assieme che contiene i disegni dei figli. Nessun documento nuovo, nessuna copia NAS, nessun
-- file inventato: la riga dice soltanto «per il disegno_2d di <componente> valgono le pagine <pagine> di
-- <documento, sha256, rev>», con chi, quando e perche'. Le coperture nascono solo dal lotto R9 del wizard
-- (gesto lotto_fascicolo, chiave_idempotenza, D-SHOCK-DOC-04 A): con il flag motore_a spento si leggono soltanto.
--
-- Registro in sola aggiunta, a catena: per (documento, componente, tipo) una sola catena lineare (un inizio, un
-- successore per riga). copre apre la catena (o la riapre dopo una revoca); rettifica cambia le pagine; revoca la
-- chiude; ogni riga dice perche' (motivo obbligatorio anche su copre: la sola traccia di una scelta umana, per
-- esempio con figli specchiati, D-SHOCK-DOC-05; revisione di dominio della notte, RD-08 [T]). Rettifica e revoca
-- restano sul proprietario registrato della riga precedente: un cambio di proprietario passa da revoca + copre
-- (RD-07). Corrente = l'ultima riga della catena.
-- Validita' (derivata, mai scritta, come BOM.Verificata): corrente e non revocata; documento ancora esistente,
-- non sostituito, con lo stesso sha256 e la stessa rev; ancora del proprietario registrato e dello stesso tipo
-- (correzione D1); proprietario attivo; componente coperto attivo, con la rev registrata e raggiungibile dal
-- proprietario nella working. La calcola v_fascicolo (sotto); il Go la ripete nel Motore A e controlla in
-- piu', in R8/R9, che il componente sia nel perimetro del prodotto esaminato.
CREATE TABLE copertura_documento (
    copertura_id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id            uuid NOT NULL REFERENCES thread_offerta,
    documento_id         uuid NOT NULL,
    sha256               char(64) NOT NULL,                 -- i byte ufficiali su cui si e' deciso (= documento.sha256)
    -- il componente del documento in quel momento (il proprietario: il finito del PDF): storico, senza FK
    documento_componente uuid NOT NULL,
    -- il componente coperto: UUID ufficiale; con la FK un componente coperto non si rimuove, si archivia
    componente_id        uuid NOT NULL,
    tipo                 tipo_documento NOT NULL,
    azione               text NOT NULL,
    precedente_id        uuid,
    pagine               smallint[],
    pagine_totali        smallint,                          -- le pagine del documento, dall'analisi (testo_pdf)
    rev_documento        varchar(10),
    rev_componente       varchar(10),
    motivo               text,
    chiave_idempotenza   uuid NOT NULL,                     -- la chiave del lotto R9 che l'ha registrata
    autore_id            uuid NOT NULL REFERENCES utente,
    quando               timestamptz NOT NULL DEFAULT now(),
    tx                   xid8 NOT NULL DEFAULT pg_current_xact_id(),
    -- documento e componente coperto nella stessa RFQ della riga (FK composite)
    CONSTRAINT fk_copertura_documento FOREIGN KEY (thread_id, documento_id)
        REFERENCES documento (thread_id, documento_id),
    CONSTRAINT fk_copertura_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ux_copertura_thread_id UNIQUE (thread_id, copertura_id),
    CONSTRAINT fk_copertura_precedente FOREIGN KEY (thread_id, precedente_id)
        REFERENCES copertura_documento (thread_id, copertura_id),
    CONSTRAINT ck_copertura_azione CHECK (azione IN ('copre', 'rettifica', 'revoca')),
    -- oggi solo il 2D (SHOCK-P4-01); un altro tipo vuole una decisione e una migrazione dopo P4
    CONSTRAINT ck_copertura_tipo CHECK (tipo = 'disegno_2d'),
    CONSTRAINT ck_copertura_altro_componente CHECK (componente_id <> documento_componente),
    CONSTRAINT ck_copertura_pagine CHECK (CASE WHEN azione IN ('copre', 'rettifica') THEN
            pagine IS NOT NULL AND pagine_totali IS NOT NULL AND pagine_totali BETWEEN 1 AND 10000
            AND cardinality(pagine) BETWEEN 1 AND 1000 AND array_position(pagine, NULL) IS NULL
            AND 1 <= ALL (pagine) AND pagine_totali >= ALL (pagine)
        ELSE pagine IS NULL AND pagine_totali IS NULL END),
    -- rettifica e revoca seguono una riga
    CONSTRAINT ck_copertura_catena CHECK (azione = 'copre' OR precedente_id IS NOT NULL),
    -- ogni riga dice perche', anche copre (RD-08 [T])
    CONSTRAINT ck_copertura_motivo CHECK (nullif(btrim(motivo), '') IS NOT NULL AND char_length(motivo) <= 1000),
    CONSTRAINT ck_copertura_sha CHECK (sha256 ~ '^[0-9a-f]{64}$')
);
-- una sola catena per chiave, lineare: un solo inizio, un solo successore per riga
CREATE UNIQUE INDEX ux_copertura_inizio ON copertura_documento (documento_id, componente_id, tipo) WHERE precedente_id IS NULL;
CREATE UNIQUE INDEX ux_copertura_successore ON copertura_documento (precedente_id) WHERE precedente_id IS NOT NULL;
CREATE INDEX ix_copertura_componente ON copertura_documento (componente_id, tipo);
CREATE INDEX ix_copertura_documento ON copertura_documento (documento_id);
CREATE INDEX ix_copertura_thread ON copertura_documento (thread_id, quando);

-- Nascita. Sempre: lo sha256 dichiarato e' quello del documento. copre e rettifica (una decisione nuova): il
-- documento e' corrente, e' ancora del proprietario dichiarato (attivo), e' del tipo coperto e porta la rev
-- dichiarata; il componente coperto e' attivo e ha la rev dichiarata; le pagine sono in ordine crescente e
-- senza doppi. Catena: la riga precedente e' della stessa chiave; copre comincia la catena o segue una revoca;
-- rettifica e revoca seguono una copertura viva (copre o rettifica), con lo stesso proprietario registrato; una
-- revoca chiude la decisione precedente e puo' riguardare un documento gia' sostituito o spostato.
CREATE FUNCTION copertura_documento_nasce() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    d record;
    p record;
BEGIN
    SELECT k.componente_id, k.tipo, k.rev, k.sha256, k.sostituito_da INTO d
      FROM documento k WHERE k.documento_id = NEW.documento_id AND k.thread_id = NEW.thread_id FOR KEY SHARE;
    IF NOT FOUND THEN
        RETURN NEW;                                    -- lo rifiuta la FK
    END IF;
    IF d.sha256 IS DISTINCT FROM NEW.sha256 THEN
        RAISE EXCEPTION USING ERRCODE = 'P4X06',
            MESSAGE = format('copertura: lo sha256 dichiarato non e'' quello del documento %s', NEW.documento_id);
    END IF;
    IF NEW.azione IN ('copre', 'rettifica') THEN
        IF d.sostituito_da IS NOT NULL OR d.componente_id IS DISTINCT FROM NEW.documento_componente
           OR d.tipo IS DISTINCT FROM NEW.tipo OR d.rev IS DISTINCT FROM NEW.rev_documento THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X06',
                MESSAGE = format('copertura: il documento %s non e'' corrente o non e'' quello dichiarato (proprietario, tipo, rev)', NEW.documento_id);
        END IF;
        PERFORM 1 FROM componente o WHERE o.componente_id = NEW.documento_componente AND o.archiviato_il IS NULL;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X06',
                MESSAGE = format('copertura: il proprietario %s del documento e'' archiviato', NEW.documento_componente);
        END IF;
        IF NEW.pagine <> ARRAY(SELECT DISTINCT x FROM unnest(NEW.pagine) AS x ORDER BY x) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X06', MESSAGE = 'copertura: pagine in ordine crescente, senza doppi';
        END IF;
        PERFORM 1 FROM componente c
         WHERE c.componente_id = NEW.componente_id AND c.archiviato_il IS NULL
           AND c.rev IS NOT DISTINCT FROM NEW.rev_componente;
        IF NOT FOUND THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X06',
                MESSAGE = format('copertura: il componente %s e'' archiviato o non ha la rev dichiarata', NEW.componente_id);
        END IF;
    END IF;
    IF NEW.precedente_id IS NOT NULL THEN
        SELECT x.documento_id, x.componente_id, x.tipo, x.azione, x.documento_componente INTO p
          FROM copertura_documento x WHERE x.copertura_id = NEW.precedente_id;
        IF FOUND AND ((p.documento_id, p.componente_id, p.tipo) IS DISTINCT FROM (NEW.documento_id, NEW.componente_id, NEW.tipo)
                      OR (NEW.azione = 'copre' AND p.azione <> 'revoca')
                      OR (NEW.azione IN ('rettifica', 'revoca') AND p.azione = 'revoca')
                      OR (NEW.azione IN ('rettifica', 'revoca') AND p.documento_componente <> NEW.documento_componente)) THEN
            RAISE EXCEPTION USING ERRCODE = 'P4X06',
                MESSAGE = format('copertura: %s (%s) non puo'' seguire %s (%s): altra chiave, o passo fuori ordine',
                                 NEW.copertura_id, NEW.azione, NEW.precedente_id, p.azione);
        END IF;
    END IF;
    NEW.tx := pg_current_xact_id();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_copertura_nasce BEFORE INSERT ON copertura_documento
    FOR EACH ROW EXECUTE FUNCTION copertura_documento_nasce();
CREATE TRIGGER trg_copertura_ora BEFORE INSERT ON copertura_documento
    FOR EACH ROW EXECUTE FUNCTION p4_ora_del_database('quando');
-- con la BOM congelata una copertura nuova si rifiuta come un documento agganciato (0020, D26): funzione
-- esistente, riusata senza riscriverla
CREATE TRIGGER trg_bom_working_copertura BEFORE INSERT ON copertura_documento
    FOR EACH ROW EXECUTE FUNCTION bom_working_modificabile();
CREATE TRIGGER trg_copertura_solo_aggiunte BEFORE UPDATE OR DELETE ON copertura_documento
    FOR EACH ROW EXECUTE FUNCTION p4_rifiuta('P4X02', 'una copertura non si modifica e non si cancella; si rettifica o si revoca con una riga nuova');
CREATE TRIGGER trg_copertura_non_si_svuota BEFORE TRUNCATE ON copertura_documento
    FOR EACH STATEMENT EXECUTE FUNCTION p4_rifiuta('P4X02', 'le coperture non si svuotano');

-- ============================================================ 10. v_fascicolo RIDEFINITA (deroga dichiarata)
-- D-SHOCK-DOC-02 A [U]. Testo della 0020 (stesse 19 colonne, stesso ordine, stessi tipi) con due cambi:
--   1. il documento di un requisito e' il documento corrente proprio del componente (come prima: vince sempre)
--      oppure, se non c'e', quello di una copertura VALIDA (la piu' recente); copertura_id dice quale (in coda;
--      NULL = documento proprio, o nessun documento);
--   2. rev_diversa e' falsa per un requisito soddisfatto da copertura: la rev del PDF condiviso e' del documento
--      del proprietario, non del figlio, e la copertura registra la rev del figlio su cui si e' decisa.
-- Per un componente senza coperture valide ogni colonna e' quella di prima (provato dalle L4 della 0022).
-- La validita' qui e' tutta quella che si calcola per componente: copertura corrente e non revocata; documento
-- esistente, non sostituito, stesso sha256, stessa rev; ancora del proprietario registrato e dello stesso tipo
-- (D1); proprietario attivo; componente coperto attivo (la vista vede solo i componenti attivi) e con la rev
-- registrata; componente raggiungibile dal proprietario lungo gli archi della working (visita ricorsiva sui nodi,
-- con UNION: ogni nodo una volta, termina anche con un ciclo, e non conta i percorsi, che con sottoassiemi condivisi
-- crescono in modo esponenziale; revisione SQL della notte, R-04; solo per le coperture candidate). Resta al Go
-- (R8/R9, guardia, Motore A) [T] il perimetro del PRODOTTO esaminato, unico custode di quel controllo (RD-05), la
-- verifica della BOM e la ripetizione della stessa regola nel Motore A (asse documenti). La rev si confronta
-- esatta (la stringa registrata), non normalizzata come rev_diversa: una rev riscritta anche solo nelle maiuscole
-- chiede una decisione nuova [T] (RD-09).
-- v_thread_bloccanti e v_cruscotto leggono questa vista per nome: non cambiano.
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

-- ============================================================ 11. COMMENTI
COMMENT ON TABLE working_contatore IS
'Due contatori della BOM working per RFQ, mossi dai trigger su componente e componente_relazione (tutti gli
scrittori, legacy compresi): struttura (codice, tipo, archiviazione, STEP autorizzato, archi e qta) e dati
(rev, descrizione, posizione). Una transazione conta al piu'' una volta per asse; un UPDATE identico non conta.
Riga mancante = (0, 0). Crescono soltanto. Una conferma strutturale e'' valida se il contatore struttura e''
quello registrato oppure la firma del perimetro ricalcolata coincide (contratto v2 §4.2). Il contatore e'' per
RFQ: una scrittura in un altro prodotto lo muove, e allora decide la firma.';
COMMENT ON COLUMN working_contatore.tx_conferma IS
'La transazione dell''ultima conferma strutturale della RFQ: in quella transazione, dopo la conferma, la
working della RFQ non si scrive piu'' (P4X01). Fuori da quella transazione non ha effetto. Come tx_struttura e
tx_dati la scrivono solo i trigger, con la transazione corrente: non torna a NULL (P4X05).';
COMMENT ON TABLE distinta_bozza IS
'La bozza di lavoro della distinta di un prodotto, salvata sul server (P4): aperta, confermata o abbandonata.
versione e'' l''ultima riga di distinta_bozza_versione; base e fonte sono la copia di quella riga, scritta dal
trigger. Una aperta per prodotto. Salvare non tocca componenti, relazioni, proposte, NAS. Una confermata non si
cancella; una abbandonata si cancella oltre 90 giorni, con versioni ed eventi. Ogni passo del ciclo resta in
distinta_bozza_registro.';
COMMENT ON TABLE distinta_bozza_registro IS
'Il ciclo di ogni bozza della distinta (aperta, confermata, abbandonata, cancellata per conservazione), scritto
solo dal trigger di distinta_bozza: chi, quando, motivo. Solo aggiunte. bozza_id e'' storico, senza FK; la FK
verso il prodotto resta anche dopo la conservazione della bozza: un prodotto su cui si e'' aperta una bozza si
archivia, non si rimuove (D-P4B1-04 A, D-P4A-01 P-04a).';
COMMENT ON TABLE distinta_bozza_versione IS
'Lo snapshot formato 2 completo di ogni versione salvata della bozza, con la base e la fonte di quel momento.
Non si riscrive.';
COMMENT ON TABLE distinta_bozza_evento IS
'Il giornale della bozza: ogni azione (anche annullata) con prima, dopo, origine, autore e data. Solo
aggiunte. Il server lo riconcilia con gli snapshot: un evento del browser non e'' una prova senza confronto.';
COMMENT ON TABLE conferma_strutturale IS
'La conferma ufficiale della struttura di un prodotto: progressiva (numero) e immutabile. Formato 1 (giro 4,
senza_riepilogo_v2) o 2 (R5, con chiave di idempotenza). Diversa da bom_versione e da BOM.Verificata.
mono_pezzo con motivo, senza figli e con lo STEP autorizzato sulla radice. criterio_verifica: step, mono_pezzo
o nessuno. La struttura ufficiale nasce da questa conferma umana, non dall''accettazione dello STEP
(D-SHOCK-S1 B).';
COMMENT ON COLUMN conferma_strutturale.mono_pezzo IS
'La classificazione del nodo finito prodotto_id come pezzo unico, decisa e motivata da una persona
(mono_pezzo_motivo): mai una proprieta'' del documento STEP, mai dedotta dall''assenza di figli (D-P4A-02 A,
D-P4A-03 A''). componente.tipo resta finito: «finito mono_pezzo» si legge dall''ultima conferma valida
(tipo_visualizzato nelle letture, scelta tecnica consigliata dalla direttiva 11, non una decisione
dell''utente), senza un quinto valore di tipo_componente. Al COMMIT: radice finito, nessun figlio ne'' arco,
STEP autorizzato sulla radice certificata; che la fonte sia corrente lo controlla il Go (K-02). Una
dichiarazione ancora senza STEP resta nella bozza: R5 la blocca, non la registra come mono_pezzo = false (in
formato 2 lo vieta ck_conferma_mono_pezzo_snapshot: mono_pezzo se e solo se lo snapshot ha la dichiarazione). Al
COMMIT conta anche un arco rimasto verso un figlio archiviato (piu'' prudente del perimetro chiuso).';
COMMENT ON COLUMN conferma_strutturale.contatore IS
'working_contatore.struttura della RFQ letto DOPO le scritture della conferma, nella sua transazione; il
trigger di nascita vuole che sia quello corrente.';
COMMENT ON COLUMN conferma_strutturale.criterio_verifica IS
'Su che cosa puo'' poggiare la verifica di questa conferma. step: lo STEP autorizzato del prodotto e'' la fonte
confermata (la radice certificata lo ha, con il suo sha256); gli archi possono venire dallo STEP o essere
decisi da una persona, come dice conferma_strutturale_arco.origine: un arco con origine utente (manuale) e''
una decisione dell''operatore, mai un arco estratto dallo STEP, e non va presentato come tale. mono_pezzo: la
dichiarazione motivata di questa conferma insieme allo STEP autorizzato del prodotto (D-P4B1-02 A): la
dichiarazione non esenta dallo STEP. nessuno: la verifica non poggia su uno STEP, perche'' alla conferma il
prodotto non aveva uno STEP autorizzato e corrente (K-02); e'' il caso normale prima dell''autorizzazione, che poi
chiede la conferma n+1, e quello di uno STEP sostituito o revocato. Il criterio non e'' la verifica:
BOM.Verificata la calcola il Motore A e non si scrive. Il gesto di entrambi gli assi (nomenclatura e
gerarchia) si ricostruisce dall''ultima conferma valida (componenti e archi certificati), non dai soli segni
sulle proposte; quando lo STEP non determina la gerarchia (nessuna occorrenza di assieme), la verifica positiva
vuole anche una deroga_struttura motivata e valida per quello STEP, il suo sha256 e la sua analisi
(D-SHOCK-S1 B): nessuna deroga universale, nessun Verificata scritto.';
COMMENT ON COLUMN conferma_strutturale.verifica IS
'La verifica del Motore A calcolata al COMMIT (al_momento_della_conferma). Non e'' BOM.Verificata, che si
ricalcola a ogni lettura.';
COMMENT ON TABLE conferma_strutturale_componente IS
'I componenti del perimetro certificati dalla conferma, uguali alla working al COMMIT. Il componente citato
diventa storia: si archivia, non si cancella.';
COMMENT ON TABLE conferma_strutturale_arco IS
'Gli archi del perimetro con la qta al COMMIT: la gerarchia e'' verificata solo se gli archi correnti
coincidono con quelli dell''ultima conferma (R-02, DOM-10).';
COMMENT ON COLUMN conferma_strutturale_arco.origine IS
'Da dove viene l''arco: step (estratto dalla fonte STEP), utente (deciso da una persona), conferma o
registrato (gia'' nella conferma precedente o nella working). Obbligatoria in formato 2 (trigger). La ricalcola il
server in R4/R5 confrontando con la fonte dichiarata, mai copiata da quello che dice la pagina: un arco
disegnato a mano non diventa «estratto dallo STEP» (D-SHOCK-S1 B, E2.1; origine_gerarchia si calcola da qui).';
COMMENT ON TABLE esito_intento IS
'L''esito (la risposta 200) di un gesto idempotente, scritto nella sua transazione prima del COMMIT. Una
risposta persa si recupera con la stessa chiave. Non si modifica; si cancella oltre 90 giorni.';
COMMENT ON TABLE f8_stato IS
'Lo stato del rismistamento F8, che gira dopo il COMMIT fuori dalla transazione: nasce in_attesa con il gesto,
diventa eseguito (finale) o errore con una scrittura propria. Nessuna riga = F8 non previsto.';
COMMENT ON TABLE correzione_documento IS
'Audit delle correzioni di codice e rev di proposte e documenti: valore letto, prima, dopo, motivo, autore, ora.';
COMMENT ON TABLE associazione_evento IS
'Chi e quando di ogni associazione di proposte e documenti (assegna, sgancia, scarta, riapri,
preassociazione, caricamento, spostamento), da tutte le strade quando le scriveranno: gli scrittori sono di B4, e
la storia comincia dal primo evento scritto da B4. Solo aggiunte.';
COMMENT ON TABLE fonte_step_evento IS
'La storia della fonte STEP di un prodotto finito: scelta, rifiuto con motivo, sostituzione con la fonte di
prima, caricamento, conferma del documento sorgente, autorizzazione e revoca. Solo aggiunte. ruolo prodotto:
la fonte del finito; ruolo sottoassieme: una fonte secondaria (scelta, rifiuto, risoluzione) con la sua
destinazione, componente_id oppure bozza_id + chiave_bozza finche'' il nodo non ha un UUID; la risoluzione al
COMMIT la scrive il database. fonte_rifiutata, storico R7 e rebase del prodotto leggono solo ruolo prodotto.';
COMMENT ON COLUMN fonte_step_evento.chiave_bozza IS
'Destinazione temporanea di una fonte secondaria: un nodo senza UUID della versione corrente della bozza
bozza_id. Si risolve con un evento risoluzione (risolve_evento_id) quando la conferma certifica quella chiave;
altrimenti resta com''e'', leggibile con destinazione e con distinta_bozza_registro anche dopo la conservazione
della bozza.';
COMMENT ON COLUMN fonte_step_evento.destinazione IS
'Con chiave_bozza: {versione, nodo}, la copia del nodo della bozza che la chiave nominava (la versione corrente
in quel momento), scritta dal database qualunque valore porti il chiamante; la risoluzione porta la stessa. Resta
dopo la conservazione della bozza: la storia dei rifiuti dice quale nodo era, non solo la sua chiave (D-SHOCK-S2).';
COMMENT ON TABLE copertura_documento IS
'Copertura documentale condivisa (D-SHOCK-DOC-01): le pagine di un documento ufficiale che soddisfano il
disegno_2d di un altro componente della stessa RFQ. Non e'' un documento e non copia niente sul NAS. In sola
aggiunta, a catena (copre, rettifica, revoca); corrente = ultima riga della catena. Valida = corrente e non
revocata, documento non sostituito con lo stesso sha256 e la stessa rev, ancora del proprietario registrato e
dello stesso tipo, proprietario e componente attivi, rev del componente uguale, componente raggiungibile dal
proprietario (derivata in v_fascicolo, mai scritta). Ogni riga ha un motivo; rettifica e revoca restano sul
proprietario registrato. Nasce solo dal lotto R9 (lotto_fascicolo).';
COMMENT ON VIEW v_fascicolo IS
'Completezza del fascicolo per componente attivo e tipo richiesto. Ridefinita dalla 0022 (deroga dichiarata,
D-SHOCK-DOC-02): le colonne della 0020 restano uguali; copertura_id in coda dice quale copertura valida
soddisfa il requisito, quando non c''e'' un documento proprio. La leggono v_thread_bloccanti, v_cruscotto, il
Fascicolo, il gate del congelamento (ContaBloccantiLogici) e la fotografia del Motore A.';
COMMENT ON TABLE attestazione_fascicolo IS
'Attestazione esplicita del fascicolo di un prodotto (campo attesta di R9), sull''ultima conferma strutturale.
Progressiva e immutabile; «da riverificare» si deriva.';
COMMENT ON TABLE attestazione_fascicolo_documento IS
'I documenti correnti attestati, con componente, tipo, rev e sha256 di quel momento.';
COMMENT ON TABLE deroga_fabbisogno_storia IS
'Copia append-only di ogni scrittura su deroga_fabbisogno, scritta solo dal trigger. autore_id dal Go
(set_config(''cockpit.autore_id'', ..., true) nella transazione) o, per un INSERT, da chi ha concesso; NULL per
un UPDATE o una revoca senza dichiarazione.';
COMMENT ON TABLE deroga_struttura_storia IS
'Copia append-only di ogni concessione (INSERT) e revoca (DELETE) di deroga_struttura, con la riga intera, scritta
solo dal trigger. autore_id dal Go (set_config(''cockpit.autore_id'', ..., true) nella transazione) o, per una
concessione, da chi l''ha concessa; NULL per una revoca senza dichiarazione. Con D-SHOCK-S1 B la deroga regge la
verifica positiva: questa storia la rende ricostruibile anche dopo la revoca.';

-- ============================================================ 12. VARIANTE E VERSIONE
-- schema_versione registra solo il numero: un database con la 22 di un testo precedente di questo file e un
-- binario con il testo nuovo direbbero tutti e due «schema presente». La variante chiude il buco: il file la
-- dichiara qui (una costante), il migratore la legge dal file incorporato e, se la 22 risulta gia' registrata,
-- la vuole uguale nel database; altrimenti si ferma prima di toccare qualunque cosa (platform/migrazioni,
-- ControllaVarianti). Ogni modifica di questo file prima della spinta cambia anche la variante (la prova L1
-- TestP4VarianteDella0022 lega variante e impronta del file). Una migrazione senza variante non e' controllata.
CREATE TABLE schema_variante (
    versione int PRIMARY KEY,
    variante text NOT NULL,
    CONSTRAINT ck_schema_variante CHECK (versione >= 1 AND variante ~ '^[a-z0-9][a-z0-9._-]{0,63}$')
);
COMMENT ON TABLE schema_variante IS
'Quale testo di una migrazione ha creato lo schema, per le migrazioni che lo dichiarano (dalla 0022). Il
migratore confronta la variante del file incorporato con questa riga: un database con la stessa versione ma
un''altra variante (o nessuna) non si avvia. schema_versione registra solo il numero.';
INSERT INTO schema_variante (versione, variante) VALUES (22, '0022-p4-dir11-20261013b');
INSERT INTO schema_versione (versione) VALUES (22);
