-- ============================================================================
--  0004_caselle_presenza.sql — Cockpit RFQ, fase 2 del piano di correzione (voce 2.1)
--
--  È la prima migrazione che SPOSTA RIGHE, non solo colonne: per questo va provata su un dump della
--  versione 3 con dati dentro (test S2) e non solo su un database vuoto.
--
--  Che cosa cambia, e perché ognuna di queste cose non poteva restare com'era.
--
--  1. LA PRESENZA (`messaggio_casella`). Finora un messaggio aveva UNA riga `messaggio_outlook` con
--     entry_id, cartella, non_letto, categorie: cioè il modello diceva che un messaggio sta in un
--     posto solo. La stessa mail mandata a Commerciale e a Francesco è invece un solo messaggio in
--     due posti, ognuno con la sua cartella, il suo stato di lettura e il suo EntryID. Con una riga
--     sola la seconda casella sovrascriveva la prima: l'EntryID diventava quello dell'altra casella,
--     e «Apri in Outlook» apriva l'elemento sbagliato o non lo trovava affatto.
--     `messaggio_casella` è la riga per COPIA; `messaggio` resta l'identità (il Message-ID).
--
--  2. IL CURSORE PER CASELLA. Alla versione 3 `sync_cursore` aveva per chiave la sola cartella: due
--     caselle con una cartella «Posta in arrivo» scrivevano sulla stessa riga e si facevano avanzare
--     il cursore a vicenda. Ogni avanzamento di troppo è una finestra di tempo che l'altra casella
--     non leggerà mai — messaggi mai acquisiti, senza nessun errore da nessuna parte. È il motivo
--     per cui il server, alla versione 3, si rifiuta di partire con più di una casella attiva
--     (fondazioni.UnaSolaCasellaAttiva): applicata questa migrazione, quel controllo smette da solo
--     di intervenire.
--
--  3. `ricevuto_il` PER COPIA (chiude W2). Il cursore avanzava su `data_evento`, che per la Posta
--     inviata è SentOn, mentre il filtro della scansione usa ReceivedTime. Per la Posta in arrivo i
--     due coincidono e il difetto non si vede; per la Posta inviata no, e una mail scritta lunedì e
--     inviata giovedì poteva spingere il cursore oltre elementi non ancora letti. `ricevuto_il` è il
--     ReceivedTime DI QUELLA CASELLA: è lo stesso valore su cui filtra la scansione, quindi cursore e
--     filtro parlano finalmente della stessa grandezza.
--
--  4. `messaggio.interno` (D10). «Da noi» e «fra noi» sono due cose diverse: un'offerta al cliente e
--     una mail fra colleghi sono entrambe in uscita, ma la seconda non è traffico con il cliente e non
--     deve generare una RFQ né comparire come tale. Separare il flag dalla direzione, invece di
--     aggiungere un terzo valore all'enum, tiene distinte due domande che restano distinte.
--
--  5. `v_inbox` CON LE CASELLE AGGREGATE e la regola dell'Inbox [v2.1]: una riga per messaggio, con
--     l'elenco delle caselle in cui si trova, e visibile se ha almeno una presenza diretta OPPURE non
--     è figlio di nessuno. Prima la condizione era `parent_messaggio_id IS NULL` e basta: una RDO
--     ricevuta direttamente spariva dall'Inbox il giorno in cui arrivava anche come allegato di un
--     inoltro (I21). Un messaggio resta nascosto solo se esiste UNICAMENTE come figlio.
--
--  Che cosa NON è qui, di proposito:
--    - `parent_messaggio_id` non viene eliminato. Il piano lo toglie insieme alle occorrenze (§2.4,
--      fase 3) e il test S2 lo dice esplicitamente per la 0005: «parent_messaggio_id travasato in
--      occorrenze prima del DROP». Toglierlo adesso cancellerebbe l'unico legame fra un messaggio
--      annidato e il suo contenitore senza avere ancora ciò che lo sostituisce.
--    - la risoluzione dello store per postazione (`casella_store`, voce 2.6). Nel frattempo lo
--      StoreID letto dal worker viaggia in `messaggio_casella.store_id_locale`, dichiarato come ponte:
--      il piano non lo vuole lì proprio perché è locale al profilo, ma tenerlo su `messaggio` sarebbe
--      già sbagliato oggi, con due caselle dello stesso profilo che hanno due store diversi.
--
--  Nessun valore di enum viene aggiunto qui (regola S4). Ogni REFERENCES punta a tabelle create nei
--  file precedenti (regola S1).
-- ============================================================================

-- ---------------------------------------------------------------- 1. la presenza
CREATE TABLE messaggio_casella (
    messaggio_id  uuid NOT NULL REFERENCES messaggio,
    casella_id    uuid NOT NULL REFERENCES casella,
    entry_id      text NOT NULL,                  -- EntryID dell'elemento IN QUESTA casella
    -- PONTE fino alla voce 2.6, non modello. Lo StoreID è locale al PROFILO Outlook che lo ha letto
    -- (N44): con una sola postazione è il valore giusto, con due l'ultima che sincronizza vince e le
    -- azioni dell'altra non trovano l'elemento. La 2.6 lo sostituisce risolvendo la casella nello
    -- store locale di ogni postazione (casella_store). Sta qui e non su `messaggio` perché due
    -- caselle dello stesso profilo hanno DUE store diversi: una riga per messaggio sarebbe sbagliata
    -- per una delle due già oggi, su un PC solo.
    store_id_locale text NOT NULL DEFAULT '',
    cartella      varchar(200),                   -- cartella in questa casella: può differire fra copie
    ricevuto_il   timestamptz NOT NULL,           -- ReceivedTime in questa casella: è il cursore (W2)
    non_letto     boolean NOT NULL DEFAULT false,
    flag_stato    smallint,
    categorie     text[],
    aggiornato_il timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (messaggio_id, casella_id)
);
CREATE INDEX ix_msgcas_casella ON messaggio_casella (casella_id, ricevuto_il DESC);
CREATE INDEX ix_msgcas_entry   ON messaggio_casella (casella_id, entry_id);

COMMENT ON TABLE messaggio_casella IS
    'Una riga per COPIA: la stessa mail in due caselle è un solo messaggio e due presenze. entry_id e cartella sono di questa casella; lo store_id NON sta qui perché appartiene al profilo Outlook della postazione (casella_store, N44).';
COMMENT ON COLUMN messaggio_casella.ricevuto_il IS
    'ReceivedTime in questa casella: la stessa grandezza su cui filtra la scansione. Il cursore avanza su questo e non su messaggio.data_evento, che per la Posta inviata è SentOn (W2).';

-- ---------------------------------------------------------------- 2. il cursore per casella
ALTER TABLE sync_cursore ADD COLUMN casella_id uuid REFERENCES casella;

-- ---------------------------------------------------------------- 3. il travaso (S2)
--
-- Le righe esistenti appartengono per forza all'unica casella attiva: alla versione 3 il server non
-- parte con più di una. Se qui non si riesce a dire con certezza di CHI sono, la migrazione si ferma
-- invece di assegnarle a caso: una presenza attribuita alla casella sbagliata è un EntryID che non
-- apre niente, e nessuno se ne accorgerebbe fino al primo clic su «Apri in Outlook».
DO $$
DECLARE unica uuid; n_att int; n_tot int; n_msg int; n_cur int;
BEGIN
    SELECT count(*) INTO n_msg FROM messaggio_outlook;
    SELECT count(*) INTO n_cur FROM sync_cursore;
    IF n_msg = 0 AND n_cur = 0 THEN
        RETURN;                                   -- database nuovo: non c'è niente da travasare
    END IF;

    SELECT count(*) INTO n_att FROM casella WHERE attiva;
    IF n_att = 1 THEN
        SELECT casella_id INTO unica FROM casella WHERE attiva;
    ELSE
        SELECT count(*) INTO n_tot FROM casella;
        IF n_tot = 1 THEN
            SELECT casella_id INTO unica FROM casella;
        END IF;
    END IF;

    IF unica IS NULL THEN
        RAISE EXCEPTION
            '0004: ci sono % messaggi Outlook e % cursori da assegnare a una casella, ma le caselle attive sono % (censite in tutto: %). Non si puo indovinare a quale appartengano.',
            n_msg, n_cur, n_att, (SELECT count(*) FROM casella)
            USING HINT = 'Lasciare attiva la sola casella che ha prodotto questi messaggi, applicare la 0004, poi riattivare le altre.';
    END IF;

    -- messaggio_outlook -> messaggio_casella. ricevuto_il parte da data_evento: per la posta in
    -- arrivo E il ReceivedTime; per la posta inviata e SentOn, cioe un'approssimazione che il primo
    -- sync successivo corregge da se, perche da ora il worker manda ricevuto_il per ogni elemento.
    INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, store_id_locale, cartella, ricevuto_il,
                                   non_letto, flag_stato, categorie)
    SELECT mo.messaggio_id, unica, mo.entry_id, mo.store_id, mo.cartella, m.data_evento,
           mo.non_letto, mo.flag_stato, mo.categorie
      FROM messaggio_outlook mo
      JOIN messaggio m ON m.messaggio_id = mo.messaggio_id;

    UPDATE sync_cursore SET casella_id = unica WHERE casella_id IS NULL;
END $$;

ALTER TABLE sync_cursore ALTER COLUMN casella_id SET NOT NULL;
ALTER TABLE sync_cursore DROP CONSTRAINT sync_cursore_pkey;
ALTER TABLE sync_cursore ADD PRIMARY KEY (casella_id, cartella);
COMMENT ON TABLE sync_cursore IS
    'Un cursore per (casella, cartella). Con la sola cartella due caselle si sovrascrivevano il cursore a vicenda e perdevano messaggi in silenzio.';

-- ---------------------------------------------------------------- 4. interno
ALTER TABLE messaggio ADD COLUMN interno boolean NOT NULL DEFAULT false;
COMMENT ON COLUMN messaggio.interno IS
    'Traffico fra caselle nostre: mittente e destinatari sono tutti di un nostro dominio. Separato dalla direzione (D10): una mail fra colleghi e un''offerta al cliente sono entrambe in uscita, ma solo la seconda è traffico con il cliente.';

-- ---------------------------------------------------------------- 5. messaggio_outlook dimagrisce
-- Le colonne per copia se ne vanno: tenerle in due posti significherebbe due verità sull'EntryID, e
-- quella sbagliata si scopre solo quando un'azione su Outlook non trova l'elemento.
-- v_inbox le legge, quindi va ricreata dopo.
DROP VIEW v_inbox;

ALTER TABLE messaggio_outlook
    DROP COLUMN entry_id,
    DROP COLUMN store_id,
    DROP COLUMN cartella,
    DROP COLUMN categorie,
    DROP COLUMN non_letto,
    DROP COLUMN flag_stato;
COMMENT ON TABLE messaggio_outlook IS
    'Ciò che è del MESSAGGIO e non della copia: conversation_id, conversation_index, in_reply_to, riferimenti. Tutto il resto — EntryID, store locale, cartella, stato di lettura — è della copia e sta in messaggio_casella.';

-- ---------------------------------------------------------------- 6. v_inbox con le caselle
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
       -- le caselle in cui il messaggio si trova: l'elenco, non la prima che capita
       COALESCE(pr.caselle, '{}')      AS caselle,
       COALESCE(pr.caselle_id, '{}')   AS caselle_id,
       COALESCE(pr.n_caselle, 0)       AS n_caselle,
       COALESCE(pr.non_letto, false)   AS non_letto,     -- non letto in ALMENO una casella
       pr.ricevuto_il,
       pr.casella_id, pr.cartella AS cartella_outlook, pr.entry_id
FROM messaggio m
LEFT JOIN thread_offerta t ON t.thread_id = m.thread_id
LEFT JOIN cliente ct ON ct.cliente_id = t.cliente_id
LEFT JOIN dominio_cliente dc ON dc.dominio = lower(split_part(m.mittente_indirizzo, '@', 2))
LEFT JOIN cliente cd ON cd.cliente_id = dc.cliente_id
LEFT JOIN buyer b ON b.buyer_id = m.buyer_id
LEFT JOIN cliente cb ON cb.cliente_id = b.cliente_id
LEFT JOIN LATERAL (
    -- una riga sola per messaggio: gli aggregati delle presenze più la copia «di riferimento»,
    -- scelta in modo ripetibile (la più vecchia, a parità la casella con l'uuid minore). Quale copia
    -- aprire davvero diventa una decisione della postazione del richiedente con la voce 2.7.
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
-- Regola dell'Inbox [v2.1]: c'è se è stato ricevuto da qualche parte, oppure se non è figlio di
-- nessuno. Sparisce solo chi esiste unicamente come allegato di un altro messaggio (I21).
WHERE COALESCE(pr.n_caselle, 0) > 0 OR m.parent_messaggio_id IS NULL;

INSERT INTO schema_versione (versione) VALUES (4);
