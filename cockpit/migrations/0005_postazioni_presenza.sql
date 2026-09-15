-- ============================================================================
--  0005_postazioni_presenza.sql — Cockpit RFQ, fase 2 del piano di correzione (voci 2.2, 2.6, 2.7)
--
--  Tre cose, tutte al servizio della stessa domanda: SU QUALE PC deve succedere una cosa, e quale
--  worker ha davvero in mano la casella che serve.
--
--  1. LA PRESENZA È PER WORKER, NON PER TIPO (`worker_presenza` riscritta, §2.2). Fino a qui la
--     tabella aveva per chiave il tipo di worker: con due worker Outlook su due PC, la riga era una
--     sola e diceva «outlook attivo» anche quando quello che serviva la casella giusta era spento da
--     un'ora. Ora c'è una riga per worker (`worker_nome`, la stessa chiave di `worker_credenziale`),
--     con la postazione, l'indirizzo IP da cui ha fatto l'ultimo claim, se Outlook rispondeva e quali
--     caselle ha risolto nel proprio profilo (`caselle_aperte`). È ciò che permette alla testata di
--     dire «Francesco: attivo su PC-FRANCESCO» invece di «outlook attivo», e all'abbinamento per IP
--     della voce 2.7 di esistere.
--
--  2. LA SESSIONE UI HA UNA POSTAZIONE (`sessione.postazione_id`, `postazione_origine`, §2.2 e N50).
--     «Apri in Outlook» apre una finestra: DEVE aprirla sul PC di chi ha premuto il pulsante, e il
--     server non lo sapeva. Ora la sessione lo sa: abbinata per IP a un worker attivo ('ip') oppure
--     scelta dall'operatore in testata ('scelta'). Senza postazione le azioni interattive sono
--     disabilitate con il motivo, non eseguite «da qualche parte».
--
--  3. VIA `store_id_locale` DA `messaggio_casella` (voce 2.6, N44). Era il ponte dichiarato dalla
--     0004: lo StoreID è locale al PROFILO Outlook che lo ha letto, e con due postazioni l'ultima che
--     sincronizzava vinceva e le azioni dell'altra non trovavano l'elemento. Da qui lo store lo risolve
--     ogni worker nel proprio profilo e lo registra in `casella_store` (postazione, casella): i payload
--     dei job portano casella_id + entry_id + Message-ID e MAI uno StoreID, perché uno StoreID di un
--     altro profilo non è un riferimento, è un numero a caso.
--
--  Che cosa NON è qui, di proposito: nessun valore di enum (S4); nessuna REFERENCES a tabelle create
--  dopo (S1). `job.casella_id`, `job.postazione_id`, `job.richiesto_da` e `job.scade_il` esistono
--  dalla 0003 e non cambiano: cambia CHI li riempie (il routing) e CHI li filtra (il claim).
-- ============================================================================

-- ---------------------------------------------------------------- 1. presenza per worker
-- Le righe vecchie (una per tipo, con worker_id) si travasano per nome: sono solo l'ultimo contatto,
-- ma buttarle vorrebbe dire che al riavvio ogni worker risulta «mai avviato» per un minuto senza
-- motivo.
CREATE TABLE worker_presenza_nuova (
    worker_nome     varchar(80) PRIMARY KEY,           -- 'outlook@PC-FRANCESCO': la chiave di worker_credenziale
    worker_tipo     worker_tipo NOT NULL,
    postazione_id   uuid REFERENCES postazione,        -- dalla credenziale, non dal JSON del claim
    indirizzo_ip    inet,                              -- remote address dell'ultimo claim (voce 2.7)
    ultimo_claim    timestamptz NOT NULL DEFAULT now(),
    outlook_ok      boolean NOT NULL DEFAULT true,     -- false = il worker gira ma COM non risponde
    caselle_aperte  uuid[] NOT NULL DEFAULT '{}',      -- caselle risolte nello store locale E autorizzate
    ultimo_job_il   timestamptz,
    ultimo_arresto  text,                              -- motivo dell'ultima uscita forzata (C16), riportato al riavvio
    avviso          text                               -- es. caselle dichiarate ma non autorizzate (Q18)
);
INSERT INTO worker_presenza_nuova (worker_nome, worker_tipo, ultimo_claim, ultimo_job_il)
SELECT worker_id, worker_tipo, ultimo_claim, ultimo_job_il FROM worker_presenza;
DROP TABLE worker_presenza;
ALTER TABLE worker_presenza_nuova RENAME TO worker_presenza;
CREATE INDEX ix_worker_presenza_ip ON worker_presenza (indirizzo_ip) WHERE indirizzo_ip IS NOT NULL;
COMMENT ON TABLE worker_presenza IS
    'Una riga per worker (non per tipo): postazione, IP dell''ultimo claim, Outlook raggiungibile, caselle risolte nel profilo locale. La testata mostra lo stato PER CASELLA da qui; l''abbinamento sessione→postazione per IP legge indirizzo_ip.';
COMMENT ON COLUMN worker_presenza.caselle_aperte IS
    'Intersezione fra le caselle che il worker dichiara di aver risolto nel proprio profilo Outlook e quelle autorizzate in worker_credenziale.caselle. Il claim assegna a un worker solo job di queste caselle (M12).';

-- ---------------------------------------------------------------- 2. postazione della sessione
ALTER TABLE sessione ADD COLUMN postazione_id uuid REFERENCES postazione;
ALTER TABLE sessione ADD COLUMN postazione_origine varchar(10);
ALTER TABLE sessione ADD CONSTRAINT ck_sessione_postazione_origine
    CHECK ((postazione_id IS NULL AND postazione_origine IS NULL)
        OR (postazione_id IS NOT NULL AND postazione_origine IN ('ip', 'scelta')));
COMMENT ON COLUMN sessione.postazione_id IS
    'Il PC da cui l''operatore sta usando il Cockpit: i job interattivi (apri, bozza, segna letto) vanno al worker di QUESTA postazione, senza ripiego su altre. NULL = azioni interattive disabilitate.';
COMMENT ON COLUMN sessione.postazione_origine IS
    'ip = abbinata al login per coincidenza con worker_presenza.indirizzo_ip di una postazione autorizzata; scelta = scelta dall''operatore in testata. Un IP sconosciuto non abilita nulla (P1).';

-- ---------------------------------------------------------------- 3. via lo store locale dalla presenza
ALTER TABLE messaggio_casella DROP COLUMN store_id_locale;
COMMENT ON TABLE messaggio_casella IS
    'Una riga per COPIA: la stessa mail in due caselle è un solo messaggio e due presenze. entry_id e cartella sono di questa casella. Lo StoreID NON sta qui: appartiene al profilo Outlook della postazione (casella_store, N44) e ogni worker lo risolve da sé.';
COMMENT ON TABLE casella_store IS
    'N44: lo StoreID appartiene al profilo, non alla casella. Ogni worker lo registra a ogni claim (postazione, casella → store); i payload dei job portano casella_id + entry_id + Message-ID, mai uno store_id. Uno store del profilo non censito in `casella` non compare qui: viene ignorato, non censito d''ufficio.';

INSERT INTO schema_versione (versione) VALUES (5);
