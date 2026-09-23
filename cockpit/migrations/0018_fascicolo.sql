-- 0018_fascicolo.sql — Blocco 8, B8.2: la struttura del Fascicolo.
-- Approvata il 23/09/2026 con quattro correzioni (descrizione nella guardia dei duplicati, «revisione
-- nota» = nullif(btrim(rev),'') IS NOT NULL, e due lato Go). Il ritorno indietro manuale e' in
-- scripts/0018_indietro.sql; il ritorno vero e' il backup preso subito prima.
--
-- Che cosa fa, in una riga: `componente` smette di essere un albero con un padre solo e diventa
-- l'identita' di un pezzo nella RFQ (thread + upper(codice)); le relazioni padre → figlio vanno in
-- `componente_relazione`, che regge il sottoassieme condiviso; le proposte dai file vanno in
-- `componente_proposta` e `relazione_proposta`; ogni FK verso `componente` porta il thread con se'.
--
-- Riferimenti: piano §3, §9, §11; addendum A1.1, A1.3, A1.4, A1.6.1, A1.6.2, A2.1–A2.6.
-- Nessun enum nuovo, nessun ADD VALUE: tutti i valori usati esistono dalla 0001/0008.
--
-- Regole del migratore: un file = una transazione (se una guardia o un'istruzione fallisce non
-- cambia niente); ogni REFERENCES punta a una tabella gia' creata; il file termina con
-- INSERT INTO schema_versione.
--
-- ORDINE: 0. mappa della fusione (tabelle temporanee, nessuna scrittura sui dati)
--         1. guardie: TUTTE prima di qualunque modifica, e tutte in un solo elenco
--         2–4. componente_relazione, travaso di padre_id, fusione dei duplicati
--         5–9. viste giu', componente, FK composite, codici normalizzati, CHECK
--         10–11. le due tabelle di proposta
--         12. fabbisogni di default
--         13–15. viste
--         16. versione

-- ============================================================ 0. LA MAPPA DELLA FUSIONE
-- L'identita' diventa (thread_id, upper(codice)). Le righe che oggi condividono quell'identita' si
-- fondono nella piu' vecchia (creato_il, poi componente_id per stabilita'). La mappa copre TUTTI i
-- componenti: chi non si fonde punta a se stesso.
-- Le due tabelle temporanee nascono dentro un DO perche' sqlc, che legge le migrazioni come schema,
-- non le scambi per tabelle vere e non generi un modello Go per ciascuna.
DO $$
BEGIN
    CREATE TEMP TABLE m18_fusione ON COMMIT DROP AS
    SELECT c.componente_id AS vecchio,
           first_value(c.componente_id) OVER (PARTITION BY c.thread_id, upper(c.codice)
                                              ORDER BY c.creato_il, c.componente_id) AS nuovo
      FROM componente c;

    -- Le relazioni di domani, calcolate da padre_id di oggi e gia' passate per la mappa.
    -- qta: nel modello vecchio la quantita' di un figlio stava sulla sua riga; da qui sta sull'arco.
    CREATE TEMP TABLE m18_relazione ON COMMIT DROP AS
    SELECT f.thread_id, mp.nuovo AS padre_id, mf.nuovo AS figlio_id, f.qta, f.origine, f.confermato_da, f.creato_il
      FROM componente f
      JOIN m18_fusione mf ON mf.vecchio = f.componente_id
      JOIN m18_fusione mp ON mp.vecchio = f.padre_id
     WHERE f.padre_id IS NOT NULL;
END $$;

-- ============================================================ 1. GUARDIE
-- Nessuna guardia corregge: ciascuna conta, elenca (al massimo 20 righe) e ferma. Chi lancia la
-- migrazione riconcilia a mano e rilancia. Le guardie sono tutte valutate prima di fermarsi, cosi'
-- un solo giro dice TUTTO quello che non va.
--
-- Il 23/09/2026, in sola lettura su cockpit_dev: tutte zero (1 componente, 1 documento agganciato,
-- nessun padre_id, nessun fabbisogno per cliente).
DO $$
DECLARE
    problemi text := '';
    riga     text;
BEGIN
    -- (a) stessa identita' (thread, upper(codice)) con revisioni diverse, entrambe note: quale sia la
    --     revisione corrente e' una decisione, non una fusione. «Nota» vuol dire non NULL e non fatta
    --     di soli spazi; due revisioni note si confrontano senza spazi ai bordi e senza maiuscole, come
    --     fa rev_diversa in v_fascicolo.
    FOR riga IN
        SELECT format('thread %s codice %s: rev %s', thread_id, upper(codice), array_agg(DISTINCT rev))
          FROM componente WHERE nullif(btrim(rev), '') IS NOT NULL
         GROUP BY thread_id, upper(codice) HAVING count(DISTINCT upper(btrim(rev))) > 1 LIMIT 20
    LOOP problemi := problemi || E'\n(a) ' || riga; END LOOP;

    -- (a2) stessa identita' con tipo, descrizione o attributi di fattibilita' diversi: fonderli
    --      sceglierebbe al posto dell'ingegnere (il passo 4 riempie i vuoti con max(), che su due
    --      valori diversi ne butterebbe uno). La qta si confronta solo fra radici: sui figli passa
    --      sull'arco.
    FOR riga IN
        SELECT format('thread %s codice %s: tipi %s, esiti %s, descrizioni %s, righe %s', thread_id, upper(codice),
                      array_agg(DISTINCT tipo), array_agg(DISTINCT esito_fattibilita),
                      count(DISTINCT descrizione), count(*))
          FROM componente
         GROUP BY thread_id, upper(codice)
        HAVING count(DISTINCT tipo) > 1
            OR count(DISTINCT descrizione) > 1
            OR count(DISTINCT esito_fattibilita) > 1
            OR count(DISTINCT note_fattibilita) > 1
            OR count(DISTINCT materiale_testo) > 1
            OR count(DISTINCT spessore_mm) > 1
            OR count(DISTINCT peso_kg) > 1
            OR count(DISTINCT qta) FILTER (WHERE padre_id IS NULL) > 1
         LIMIT 20
    LOOP problemi := problemi || E'\n(a2) ' || riga; END LOOP;

    -- (b) un riferimento a un componente di un'ALTRA RFQ. Oggi il Go non lo fa (allegati.go usa lo
    --     stesso thread); la guardia c'e' per non presumerlo. Vale anche per il padre di oggi.
    FOR riga IN
        SELECT format('documento %s → componente di un altro thread', d.documento_id)
          FROM documento d JOIN componente c ON c.componente_id = d.componente_id WHERE c.thread_id <> d.thread_id
        UNION ALL
        SELECT format('documento_proposta %s → componente di un altro thread', p.proposta_id)
          FROM documento_proposta p JOIN componente c ON c.componente_id = p.componente_id WHERE c.thread_id <> p.thread_id
        UNION ALL
        SELECT format('deroga %s → componente di un altro thread', g.deroga_id)
          FROM deroga_fabbisogno g JOIN componente c ON c.componente_id = g.componente_id WHERE c.thread_id <> g.thread_id
        UNION ALL
        SELECT format('componente %s → padre di un altro thread', f.componente_id)
          FROM componente f JOIN componente p ON p.componente_id = f.padre_id WHERE p.thread_id <> f.thread_id
        LIMIT 20
    LOOP problemi := problemi || E'\n(b) ' || riga; END LOOP;

    -- (c) documento o proposta agganciati con un codice diverso da quello del componente, anche in
    --     maiuscolo. Differenze di sole maiuscole NON fermano: le normalizza il passo 8.
    FOR riga IN
        SELECT format('documento %s: codice %s, componente %s', d.documento_id, d.codice, c.codice)
          FROM documento d JOIN componente c ON c.componente_id = d.componente_id
         WHERE upper(d.codice) <> upper(c.codice)
        UNION ALL
        SELECT format('documento_proposta %s: codice %s, componente %s', p.proposta_id, p.codice, c.codice)
          FROM documento_proposta p JOIN componente c ON c.componente_id = p.componente_id
         WHERE upper(p.codice) <> upper(c.codice)
        LIMIT 20
    LOOP problemi := problemi || E'\n(c) ' || riga; END LOOP;

    -- (d) proposta con componente ma senza thread: con thread_id NULL la FK composita non verrebbe
    --     controllata (MATCH SIMPLE), e il CHECK del passo 9 fallirebbe.
    FOR riga IN
        SELECT format('documento_proposta %s', proposta_id)
          FROM documento_proposta WHERE componente_id IS NOT NULL AND thread_id IS NULL LIMIT 20
    LOOP problemi := problemi || E'\n(d) ' || riga; END LOOP;

    -- (e) agganciati a un componente con codice NULL o vuoto (A2.2).
    FOR riga IN
        SELECT format('documento %s', documento_id) FROM documento
         WHERE componente_id IS NOT NULL AND nullif(btrim(codice), '') IS NULL
        UNION ALL
        SELECT format('documento_proposta %s', proposta_id) FROM documento_proposta
         WHERE componente_id IS NOT NULL AND nullif(btrim(codice), '') IS NULL
        LIMIT 20
    LOOP problemi := problemi || E'\n(e) ' || riga; END LOOP;

    -- (f) componenti senza chi li ha confermati (A2.4): nessun utente di sistema li adotta.
    FOR riga IN
        SELECT format('componente %s (thread %s, codice %s)', componente_id, thread_id, codice)
          FROM componente WHERE confermato_da IS NULL LIMIT 20
    LOOP problemi := problemi || E'\n(f) ' || riga; END LOOP;

    -- (g) componenti con codice vuoto o di soli spazi (A2.6).
    FOR riga IN
        SELECT format('componente %s (thread %s)', componente_id, thread_id)
          FROM componente WHERE btrim(codice) = '' LIMIT 20
    LOOP problemi := problemi || E'\n(g) ' || riga; END LOOP;

    -- (h) documenti tecnici gia' confermati senza codice: il CHECK di A2.2 li rifiuterebbe con un
    --     errore anonimo; meglio l'elenco.
    FOR riga IN
        SELECT format('documento %s (%s, %s)', documento_id, tipo, nome_file)
          FROM documento WHERE tipo IN ('cad_3d', 'disegno_2d', 'sviluppo_dxf') AND nullif(btrim(codice), '') IS NULL
         LIMIT 20
    LOOP problemi := problemi || E'\n(h) ' || riga; END LOOP;

    -- (i) la fusione produrrebbe un pezzo dentro se stesso (X sotto x, stesso codice in maiuscolo).
    FOR riga IN
        SELECT format('thread %s: componente %s dentro se stesso', thread_id, padre_id)
          FROM m18_relazione WHERE padre_id = figlio_id LIMIT 20
    LOOP problemi := problemi || E'\n(i) ' || riga; END LOOP;

    -- (j) la fusione unirebbe due archi uguali con quantita' diverse: quale valga e' una decisione.
    FOR riga IN
        SELECT format('thread %s: %s → %s con qta %s', thread_id, padre_id, figlio_id, array_agg(DISTINCT qta))
          FROM m18_relazione GROUP BY thread_id, padre_id, figlio_id HAVING count(DISTINCT qta) > 1 LIMIT 20
    LOOP problemi := problemi || E'\n(j) ' || riga; END LOOP;

    -- (k) un ciclo, gia' nei padre_id di oggi o creato dalla fusione. Il database non li vieta da
    --     solo (A1.1: i cicli restano un controllo Go); qui non se ne porta dentro nessuno.
    FOR riga IN
        WITH RECURSIVE cammino (inizio, nodo, percorso) AS (
            SELECT padre_id, figlio_id, ARRAY[padre_id, figlio_id] FROM m18_relazione WHERE padre_id <> figlio_id
            UNION ALL
            SELECT c.inizio, r.figlio_id, c.percorso || r.figlio_id
              FROM cammino c JOIN m18_relazione r ON r.padre_id = c.nodo
             WHERE NOT r.figlio_id = ANY (c.percorso)
        )
        SELECT DISTINCT format('ciclo che passa da %s', c.inizio)
          FROM cammino c JOIN m18_relazione r ON r.padre_id = c.nodo AND r.figlio_id = c.inizio
         LIMIT 20
    LOOP problemi := problemi || E'\n(k) ' || riga; END LOOP;

    -- (l) due deroghe per lo stesso (componente, tipo) dopo la fusione: la UNIQUE lo vieta, e
    --     buttarne una sarebbe cancellare una decisione con il suo motivo. Il piano diceva «si tiene
    --     la prima»: qui si preferisce fermarsi, perche' il motivo dell'altra andrebbe perso.
    FOR riga IN
        SELECT format('componente %s, tipo %s: %s deroghe', m.nuovo, g.tipo, count(*))
          FROM deroga_fabbisogno g JOIN m18_fusione m ON m.vecchio = g.componente_id
         GROUP BY m.nuovo, g.tipo HAVING count(*) > 1 LIMIT 20
    LOOP problemi := problemi || E'\n(l) ' || riga; END LOOP;

    IF problemi <> '' THEN
        RAISE EXCEPTION USING
            MESSAGE = '0018: dati da riconciliare prima della migrazione (nessuna modifica fatta):' || problemi,
            HINT = 'Ogni lettera e'' una guardia descritta nel file. Riconciliare a mano e rilanciare.';
    END IF;
END $$;

-- ============================================================ 2. componente_relazione
-- «Il padre P contiene il figlio F, qta volte». Un componente senza archi entranti e' una radice.
-- Le FK composite verso componente arrivano al passo 7, quando esiste il loro bersaglio.
CREATE TABLE componente_relazione (
    thread_id     uuid NOT NULL REFERENCES thread_offerta,
    padre_id      uuid NOT NULL,
    figlio_id     uuid NOT NULL,
    qta           int NOT NULL DEFAULT 1 CHECK (qta > 0),
    posizione     varchar(20),                          -- riferimento di distinta, se c'e'
    origine       origine_componente NOT NULL,
    confermato_da uuid NOT NULL REFERENCES utente,      -- una relazione e' una decisione tecnica umana
    creato_il     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (padre_id, figlio_id),
    CHECK (padre_id <> figlio_id)
);
CREATE INDEX ix_componente_relazione_thread ON componente_relazione (thread_id);
CREATE INDEX ix_componente_relazione_figlio ON componente_relazione (figlio_id);

-- ============================================================ 3. TRAVASO DI padre_id
-- Sui dati noti sono zero righe: InsertComponente e' sempre chiamato senza padre. La migrazione non
-- lo presume. La guardia (f) ha gia' escluso confermato_da NULL; la (j) le quantita' in conflitto.
INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da, creato_il)
SELECT DISTINCT ON (padre_id, figlio_id) thread_id, padre_id, figlio_id, qta, origine, confermato_da, creato_il
  FROM m18_relazione
 ORDER BY padre_id, figlio_id, creato_il;

-- ============================================================ 4. FUSIONE DEI DUPLICATI
DO $$
DECLARE
    n_fusi integer;
BEGIN
    SELECT count(*) INTO n_fusi FROM m18_fusione WHERE vecchio <> nuovo;
    RAISE NOTICE '0018: % componenti fusi in un altro con la stessa identita'' (thread, upper(codice))', n_fusi;
END $$;

-- Il vincolo vecchio e padre_id non servono piu': le relazioni sono gia' in componente_relazione.
-- Il vincolo va tolto PRIMA di azzerare padre_id, altrimenti due figli con lo stesso codice sotto
-- due padri diversi diventerebbero due righe uguali per quella UNIQUE.
ALTER TABLE componente DROP CONSTRAINT componente_thread_id_padre_id_codice_rev_key;
UPDATE componente SET padre_id = NULL WHERE padre_id IS NOT NULL;

-- La riga che resta prende cio' che sapevano le altre dove lei non sapeva niente. Le guardie (a) e
-- (a2) hanno gia' escluso i disaccordi, quindi ogni max() vede un valore solo: qui si riempiono
-- solo i vuoti. Una revisione di soli spazi e' un vuoto come NULL.
UPDATE componente k
   SET rev               = CASE WHEN nullif(btrim(k.rev), '') IS NULL THEN COALESCE(x.rev, k.rev) ELSE k.rev END,
       descrizione       = COALESCE(k.descrizione, x.descrizione),
       materiale_testo   = COALESCE(k.materiale_testo, x.materiale_testo),
       spessore_mm       = COALESCE(k.spessore_mm, x.spessore_mm),
       peso_kg           = COALESCE(k.peso_kg, x.peso_kg),
       esito_fattibilita = COALESCE(k.esito_fattibilita, x.esito_fattibilita),
       note_fattibilita  = COALESCE(k.note_fattibilita, x.note_fattibilita)
  FROM (SELECT m.nuovo,
               max(c.rev) FILTER (WHERE nullif(btrim(c.rev), '') IS NOT NULL) AS rev, max(c.descrizione) AS descrizione, max(c.materiale_testo) AS materiale_testo,
               max(c.spessore_mm) AS spessore_mm, max(c.peso_kg) AS peso_kg,
               max(c.esito_fattibilita::text)::esito_fattibilita AS esito_fattibilita,
               max(c.note_fattibilita) AS note_fattibilita
          FROM m18_fusione m JOIN componente c ON c.componente_id = m.vecchio
         WHERE m.vecchio <> m.nuovo
         GROUP BY m.nuovo) x
 WHERE k.componente_id = x.nuovo;

UPDATE documento d          SET componente_id = m.nuovo FROM m18_fusione m WHERE d.componente_id = m.vecchio AND m.vecchio <> m.nuovo;
UPDATE documento_proposta p SET componente_id = m.nuovo FROM m18_fusione m WHERE p.componente_id = m.vecchio AND m.vecchio <> m.nuovo;
UPDATE deroga_fabbisogno g  SET componente_id = m.nuovo FROM m18_fusione m WHERE g.componente_id = m.vecchio AND m.vecchio <> m.nuovo;
DELETE FROM componente c USING m18_fusione m WHERE c.componente_id = m.vecchio AND m.vecchio <> m.nuovo;

-- ============================================================ 5. LE VISTE CHE LEGGONO padre_id
-- v_cruscotto dipende da v_thread_bloccanti, che dipende da v_fascicolo (che legge padre_id).
-- Si ricreano ai passi 13–15.
DROP VIEW v_cruscotto;
DROP VIEW v_thread_bloccanti;
DROP VIEW v_fascicolo;

-- ============================================================ 6. componente: L'IDENTITA'
ALTER TABLE componente DROP COLUMN padre_id;                 -- porta via anche componente_padre_id_fkey
CREATE UNIQUE INDEX ux_componente_thread_codice ON componente (thread_id, upper(codice));
-- Le due UNIQUE seguenti sono ridondanti con la PK: esistono come BERSAGLI delle FK composite (A2.1)
-- e come garanzia esplicita che un componente appartiene a una RFQ sola.
ALTER TABLE componente ADD CONSTRAINT ux_componente_thread_id        UNIQUE (thread_id, componente_id);
ALTER TABLE componente ADD CONSTRAINT ux_componente_thread_id_codice UNIQUE (thread_id, componente_id, codice);
ALTER TABLE componente ADD CONSTRAINT ck_componente_codice CHECK (btrim(codice) <> '');
-- componente = decisione tecnica umana (A2.4). La guardia (f) garantisce che non ci siano NULL.
ALTER TABLE componente ALTER COLUMN confermato_da SET NOT NULL;

COMMENT ON TABLE componente IS
'Identita'' di un pezzo dentro la RFQ: (thread_id, upper(codice)). La revisione e'' un attributo (quella
corrente), non l''identita''. Le relazioni padre → figlio stanno in componente_relazione. Una riga nasce
solo da un gesto dell''ingegnere: mai da un worker, da un job o dalla conferma di un file.';
COMMENT ON COLUMN componente.qta IS
'Quantita'' richiesta dal cliente, significativa sulle RADICI. La quantita'' di un figlio dentro un padre
sta sull''arco (componente_relazione.qta).';

-- ============================================================ 7. componente_relazione: STESSA RFQ
ALTER TABLE componente_relazione
    ADD CONSTRAINT fk_relazione_padre  FOREIGN KEY (thread_id, padre_id)  REFERENCES componente (thread_id, componente_id) ON DELETE CASCADE,
    ADD CONSTRAINT fk_relazione_figlio FOREIGN KEY (thread_id, figlio_id) REFERENCES componente (thread_id, componente_id) ON DELETE CASCADE;

COMMENT ON TABLE componente_relazione IS
'Occorrenza di un figlio in un padre, qta volte. Un sottoassieme condiviso fra due prodotti e'' UN
componente con due archi entranti. Le FK composite con thread_id rendono impossibile legare due RFQ.
I cicli NON li vieta il database: li rifiuta il Go prima di ogni INSERT (risalita degli antenati).';

-- ============================================================ 8. CODICI DEI DOCUMENTI AGGANCIATI
-- Dopo la guardia (c) differiscono solo per le maiuscole: si prende la stringa del componente,
-- perche' la FK del passo 9 la vuole identica (A1.4, punto 8). path_relativo NON si tocca: sul NAS
-- (NTFS) la cartella e' la stessa, e ricalcolarlo vorrebbe dire spostare file.
DO $$
DECLARE
    n_doc  integer;
    n_prop integer;
BEGIN
    UPDATE documento d SET codice = c.codice
      FROM componente c WHERE d.componente_id = c.componente_id AND d.codice IS DISTINCT FROM c.codice;
    GET DIAGNOSTICS n_doc = ROW_COUNT;
    UPDATE documento_proposta p SET codice = c.codice
      FROM componente c WHERE p.componente_id = c.componente_id AND p.codice IS DISTINCT FROM c.codice;
    GET DIAGNOSTICS n_prop = ROW_COUNT;
    RAISE NOTICE '0018: codice allineato al componente (solo maiuscole) su % documenti e % proposte', n_doc, n_prop;
END $$;

-- ============================================================ 9. FK COMPOSITE E CODICI OBBLIGATORI
-- documento: (thread, componente, codice) → componente. Il terzo campo e' l'invariante di A1.4:
-- un documento agganciato ha ESATTAMENTE il codice del suo componente.
-- DEFERRABLE INITIALLY IMMEDIATE: di norma si controlla subito. Chi corregge il codice di un
-- componente che ha documenti apre la transazione con SET CONSTRAINTS ALL DEFERRED, aggiorna il
-- componente e poi i documenti. Una transazione che se ne dimentica fallisce alla prima UPDATE: e'
-- voluto (prova TestCorreggereIlCodiceSenzaDeferredFallisceSubito).
ALTER TABLE documento DROP CONSTRAINT documento_componente_id_fkey;
ALTER TABLE documento
    ADD CONSTRAINT fk_documento_componente FOREIGN KEY (thread_id, componente_id, codice)
        REFERENCES componente (thread_id, componente_id, codice) DEFERRABLE INITIALLY IMMEDIATE,
    -- con MATCH SIMPLE un codice NULL spegnerebbe la FK: il CHECK chiude il buco (A2.2)
    ADD CONSTRAINT ck_documento_agganciato_ha_codice
        CHECK (componente_id IS NULL OR nullif(btrim(codice), '') IS NOT NULL),
    -- un file tecnico non diventa documento senza codice; capitolati, offerte, corrispondenza si'
    ADD CONSTRAINT ck_documento_tecnico_ha_codice
        CHECK (tipo NOT IN ('cad_3d', 'disegno_2d', 'sviluppo_dxf') OR nullif(btrim(codice), '') IS NOT NULL);

ALTER TABLE documento_proposta DROP CONSTRAINT documento_proposta_componente_id_fkey;
ALTER TABLE documento_proposta
    ADD CONSTRAINT fk_proposta_componente FOREIGN KEY (thread_id, componente_id, codice)
        REFERENCES componente (thread_id, componente_id, codice) DEFERRABLE INITIALLY IMMEDIATE,
    -- thread_id e' nullable (proposta di un messaggio orfano): senza thread la FK non verrebbe
    -- controllata, quindi senza thread niente componente
    ADD CONSTRAINT ck_proposta_componente_thread
        CHECK (componente_id IS NULL OR thread_id IS NOT NULL),
    ADD CONSTRAINT ck_proposta_agganciata_ha_codice
        CHECK (componente_id IS NULL OR nullif(btrim(codice), '') IS NOT NULL);

ALTER TABLE deroga_fabbisogno DROP CONSTRAINT deroga_fabbisogno_componente_id_fkey;
ALTER TABLE deroga_fabbisogno
    ADD CONSTRAINT fk_deroga_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id);

-- ============================================================ 10. componente_proposta
-- «Nel file F c'e' un nodo». INTERPRETAZIONE: la scrive il server dai fatti (fan-out, B8.5) e la
-- riclassifica finche' e' aperta; l'ingegnere decide (stato, componente_id).
CREATE TABLE componente_proposta (
    proposta_id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id      uuid NOT NULL REFERENCES thread_offerta,
    allegato_id    uuid NOT NULL REFERENCES allegato,    -- la provenienza: lo STEP o il PDF
    sha256         char(64) NOT NULL,
    chiave         varchar(120) NOT NULL,                -- indirizzo del nodo nel file (#12; riga di distinta)
    nome_grezzo    varchar(200) NOT NULL,                -- PRODUCT.name cosi' com'e': senza codice e' l'unica cosa da mostrare
    id_grezzo      varchar(200) NOT NULL DEFAULT '',     -- PRODUCT.id
    descrizione    varchar(200),
    codice         varchar(60),                          -- dal Motore del cliente (A1.2); vuoto = da digitare
    rev            varchar(10),
    origine_codice origine_codice,                       -- famiglia | generico | operatore; NULL senza codice
    famiglia       varchar(120) NOT NULL DEFAULT '',
    tipo_proposto  tipo_componente,                      -- suggerimento: sottoassieme se ha figli, sciolto se foglia
    fonte          fonte_proposta NOT NULL,              -- step | cartiglio
    confidenza     smallint NOT NULL CHECK (confidenza BETWEEN 0 AND 100),
    evidenza       jsonb NOT NULL DEFAULT '{}',
    stato          stato_proposta NOT NULL DEFAULT 'aperta',
    componente_id  uuid,
    deciso_da      uuid REFERENCES utente,
    deciso_il      timestamptz,
    creato_il      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ux_componente_proposta_chiave UNIQUE (thread_id, allegato_id, chiave),
    CONSTRAINT fk_componente_proposta_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    -- confermata = ha creato il componente; duplicato = si e' riconciliata con uno esistente (A2.3)
    CONSTRAINT ck_componente_proposta_decisa
        CHECK (stato NOT IN ('confermata', 'duplicato') OR componente_id IS NOT NULL)
);
CREATE INDEX ix_componente_proposta_aperte ON componente_proposta (thread_id) WHERE stato = 'aperta';
CREATE INDEX ix_componente_proposta_componente ON componente_proposta (componente_id) WHERE componente_id IS NOT NULL;

COMMENT ON TABLE componente_proposta IS
'Nodo proposto da un file per una RFQ (STEP: un PRODUCT; PDF: una riga di distinta). Il worker manda i
grezzi, il codice lo classifica il server con le regole del cliente. Upsert per chiave solo finche''
aperta. L''appartenenza dell''allegato al thread la controlla il Go al fan-out: allegato → messaggio
non ha un thread stabile, quindi non puo'' essere una FK composita.';

-- ============================================================ 11. relazione_proposta
-- «Nel file F il nodo P contiene il nodo C, qta volte». Gli estremi sono nodi dello STESSO file e
-- della STESSA RFQ per costruzione (FK composite). Nessun relazione_id: la relazione confermata si
-- ritrova per (padre.componente_id, figlio.componente_id).
CREATE TABLE relazione_proposta (
    thread_id     uuid NOT NULL,
    allegato_id   uuid NOT NULL,
    padre_chiave  varchar(120) NOT NULL,
    figlio_chiave varchar(120) NOT NULL,
    qta           int NOT NULL DEFAULT 1 CHECK (qta > 0),   -- numero di NAUO della coppia
    evidenza      jsonb NOT NULL DEFAULT '{}',
    stato         stato_proposta NOT NULL DEFAULT 'aperta',
    nota          varchar(200),                              -- «qta diversa: 2 contro 1»
    deciso_da     uuid REFERENCES utente,
    deciso_il     timestamptz,
    creato_il     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, allegato_id, padre_chiave, figlio_chiave),
    CHECK (padre_chiave <> figlio_chiave),
    CONSTRAINT fk_relazione_proposta_padre FOREIGN KEY (thread_id, allegato_id, padre_chiave)
        REFERENCES componente_proposta (thread_id, allegato_id, chiave) ON DELETE CASCADE,
    CONSTRAINT fk_relazione_proposta_figlio FOREIGN KEY (thread_id, allegato_id, figlio_chiave)
        REFERENCES componente_proposta (thread_id, allegato_id, chiave) ON DELETE CASCADE
);
CREATE INDEX ix_relazione_proposta_figlio ON relazione_proposta (thread_id, allegato_id, figlio_chiave);

COMMENT ON TABLE relazione_proposta IS
'Arco proposto da un file: una riga per coppia distinta (padre, figlio), qta = occorrenze. Si accetta
solo quando i due nodi sono gia'' accettati o agganciati; una coppia gia'' confermata con qta diversa
diventa duplicato con la nota, e la scelta resta all''ingegnere.';

-- ============================================================ 12. FABBISOGNI DI DEFAULT (§9.1)
-- La regola aziendale nuova: il 3D di un sottoassieme non blocca piu'; DXF del sottoassieme e 3D
-- dello sciolto entrano come non bloccanti. Vale per i clienti SENZA righe proprie per quel tipo di
-- componente (risoluzione in blocco, 0007): quelli con righe proprie non la vedono, e il NOTICE li
-- elenca perche' qualcuno li guardi. Non ci si ferma: sono regole del cliente, volute.
UPDATE fabbisogno_documento SET bloccante = false
 WHERE cliente_id IS NULL AND tipo_componente = 'sottoassieme' AND tipo = 'cad_3d';
INSERT INTO fabbisogno_documento (cliente_id, tipo_componente, tipo, bloccante) VALUES
    (NULL, 'sottoassieme', 'sviluppo_dxf', false),
    (NULL, 'sciolto',      'cad_3d',       false)
ON CONFLICT (cliente_id, tipo_componente, tipo) DO UPDATE SET bloccante = false;

DO $$
DECLARE
    riga text;
BEGIN
    FOR riga IN
        SELECT format('cliente %s (%s): righe proprie per %s', f.cliente_id, c.cartella_nas,
                      array_agg(DISTINCT f.tipo_componente))
          FROM fabbisogno_documento f JOIN cliente c ON c.cliente_id = f.cliente_id
         WHERE f.tipo_componente IN ('sottoassieme', 'sciolto')
         GROUP BY f.cliente_id, c.cartella_nas
    LOOP
        RAISE NOTICE '0018: i default nuovi NON valgono per %', riga;
    END LOOP;
END $$;

-- ============================================================ 13. v_fascicolo v2, bloccanti, cruscotto
-- Una riga per (componente, tipo di documento richiesto). Le colonne di prima restano nello stesso
-- ordine, meno padre_id (l'ordine dell'albero lo da' v_componente_albero); le nuove sono in coda.
-- Esiti:
--   ok             documento presente, sul NAS, nessuna anomalia aperta
--   ok_in_coda     documento presente, copia non ancora fatta
--   ok_errore_nas  documento presente, copia in errore oppure anomalia di integrita' aperta
--   derogato       deroga presente
--   da_confermare  proposta aperta di quel tipo, per quel componente o con lo stesso codice
--   sul_portale    riferimento al portale ancora da scaricare
--   manca          altrimenti (bloccante o no lo dice la colonna)
CREATE VIEW v_fascicolo AS
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
LEFT JOIN deroga_fabbisogno dg ON dg.componente_id = co.componente_id AND dg.tipo = f.tipo;

-- D9: un documento confermato ma ancora in coda NON blocca (la decisione c'e', la copia arriva);
-- un documento in errore o con un'anomalia aperta SI' (e' un fatto che qualcuno deve vedere prima
-- della fattibilita'). Colonne identiche a prima: v_cruscotto le legge.
CREATE VIEW v_thread_bloccanti AS
SELECT t.thread_id,
       count(v.componente_id) FILTER (WHERE v.bloccante AND v.esito NOT IN ('ok','ok_in_coda','derogato')) AS n_bloccanti,
       count(v.componente_id) FILTER (WHERE v.esito = 'da_confermare')                                   AS n_da_confermare,
       count(v.componente_id) FILTER (WHERE v.esito = 'sul_portale')                                     AS n_sul_portale,
       count(v.componente_id) FILTER (WHERE v.esito = 'manca')                                           AS n_mancanti
FROM thread_offerta t LEFT JOIN v_fascicolo v ON v.thread_id = t.thread_id
GROUP BY t.thread_id;

-- Testo identico alla 0001 (confrontato il 23/09/2026 con pg_get_viewdef su cockpit_dev).
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

-- ============================================================ 14. v_componente_albero
-- Dalle radici (nessun arco entrante) in giu'. Con piu' padri un componente compare UNA VOLTA PER
-- PERCORSO. qta_cumulata = prodotto delle qta degli archi lungo il percorso (1 sulla radice); la
-- quantita' totale di un pezzo nella RFQ = somma delle qta_cumulata dei suoi percorsi × qta della
-- radice. La guardia sul percorso protegge la vista anche da un ciclo entrato per un bug futuro.
CREATE VIEW v_componente_albero AS
WITH RECURSIVE albero (thread_id, componente_id, radice_id, padre_id, profondita, percorso, qta_cumulata) AS (
    SELECT c.thread_id, c.componente_id, c.componente_id, NULL::uuid, 0, ARRAY[c.componente_id], 1::bigint
      FROM componente c
     WHERE NOT EXISTS (SELECT 1 FROM componente_relazione r WHERE r.figlio_id = c.componente_id)
    UNION ALL
    SELECT a.thread_id, r.figlio_id, a.radice_id, r.padre_id, a.profondita + 1, a.percorso || r.figlio_id,
           a.qta_cumulata * r.qta
      FROM albero a
      JOIN componente_relazione r ON r.padre_id = a.componente_id
     WHERE NOT r.figlio_id = ANY (a.percorso)
)
SELECT thread_id, componente_id, radice_id, padre_id, profondita, percorso, qta_cumulata FROM albero;

-- ============================================================ 15. v_codici_candidati_thread
-- Tutti i codici che la RFQ ha visto, con da dove vengono. Una riga per evidenza: il Go deduplica
-- per upper(codice), tiene TUTTE le evidenze e decide le due liste (§6.1). Niente classificazione
-- in SQL: i nodi dei file arrivano gia' classificati da componente_proposta (A1.2).
-- Esclusi: il riferimento della richiesta (non e' un codice, sta in testata) e le proposte scartate.
CREATE VIEW v_codici_candidati_thread AS
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
 WHERE p.thread_id IS NOT NULL AND p.stato <> 'scartata' AND nullif(btrim(p.codice), '') IS NOT NULL
UNION ALL
SELECT p.thread_id, n.codice, '',
       'nome_file', 'nome_file', '', 50, a.nome_file::text, NULL,
       a.messaggio_id, p.allegato_id
  FROM documento_proposta p
  JOIN allegato a ON a.allegato_id = p.allegato_id
  CROSS JOIN LATERAL jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(p.dettagli -> 'codici_nel_nome') = 'array' THEN p.dettagli -> 'codici_nel_nome' ELSE '[]'::jsonb END
  ) AS n (codice)
 WHERE p.thread_id IS NOT NULL AND p.stato <> 'scartata' AND btrim(n.codice) <> ''
UNION ALL
SELECT cp.thread_id, cp.codice::text, COALESCE(cp.rev, '')::text,
       cp.fonte::text, cp.origine_codice::text, cp.famiglia::text, cp.confidenza::int,
       (cp.nome_grezzo || ' — ' || a.nome_file)::text, NULL,
       a.messaggio_id, cp.allegato_id
  FROM componente_proposta cp
  JOIN allegato a ON a.allegato_id = cp.allegato_id
 WHERE cp.stato <> 'scartata' AND nullif(btrim(cp.codice), '') IS NOT NULL;

-- ============================================================ 16. VERSIONE
INSERT INTO schema_versione (versione) VALUES (18);
