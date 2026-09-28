-- 0016_classificazione.sql — Blocco 7C.0, parte 1: il vocabolario della classificazione e
-- l'anagrafica «Altro». Bozza del 18/09/2026, approvata con sei modifiche lo stesso giorno.
--
-- Il contratto di classificazione separa tre domande che il triage mescolava:
--   CHI scrive            → controparte (fatto sul messaggio, deterministico, sei valori)
--   CHE COSA sta facendo  → atto business (proposta, neutro rispetto alla direzione)
--   A CHE COSA appartiene → legame operativo (proposta; il fatto resta thread_id / richiesta_fornitore_id)
-- L'esito del triage (nuova_rfq / aggancia / ignora) resta la proposta di AZIONE per la schermata,
-- e non e' una seconda rappresentazione del significato.
--
-- Perche' due file (0016 e 0017): il migratore vieta di usare un valore enum nello stesso file che
-- lo aggiunge (S4), e qui si aggiunge `altro` a tipo_controparte. Il CHECK sul messaggio e la vista
-- che lo usano stanno nella 0017. Questo file lascia il database coerente da solo: la colonna
-- `intento` resta finche' la 0017 non la toglie insieme alla vista che la legge.
--
-- Regole del migratore: un file = una transazione; ogni REFERENCES punta a una tabella gia' creata;
-- il file termina con INSERT INTO schema_versione.

-- ============================================================ 1. LA SESTA CONTROPARTE: «altro»

-- Un soggetto riconosciuto ESPLICITAMENTE come diverso da cliente, fornitore e interno: corriere,
-- banca, consulente, notifiche di un servizio, newsletter. Non e' il cestino degli sconosciuti: uno
-- sconosciuto resta `sconosciuto` (Da validare) finche' una persona non lo censisce, e l'agente non
-- puo' mai proporre di metterlo qui.
ALTER TYPE tipo_controparte ADD VALUE 'altro' BEFORE 'sconosciuto';

-- L'anagrafica e' piccola di proposito: un'etichetta, una nota, e i recapiti. Niente lavorazioni,
-- niente buyer, niente regole: chi ha bisogno di quelle e' un cliente o un fornitore.
CREATE TABLE soggetto_altro (
    altro_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    etichetta  varchar(120) NOT NULL CHECK (btrim(etichetta) <> ''),   -- «Corriere», «Notifiche Microsoft», «Banca»
    note       text,
    attivo     boolean NOT NULL DEFAULT true,
    creato_il  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_soggetto_altro_etichetta ON soggetto_altro (lower(etichetta));

-- Un recapito e' un INDIRIZZO («newsletter@cliente.example») oppure un DOMINIO («corriere.example»):
-- lo distingue la chiocciola. Una tabella sola e non due perche' il resolver li tratta con la stessa
-- precedenza degli altri (indirizzo esatto prima, dominio poi), e un dominio pubblico si censisce
-- per indirizzo, come per i fornitori. Lo stesso recapito puo' stare anche in dominio_cliente o in
-- contatto_fornitore: a parita' di specificita' il resolver risponde `ambiguo`, non sceglie in
-- silenzio.
CREATE TABLE recapito_altro (
    recapito   varchar(200) PRIMARY KEY CHECK (recapito = lower(recapito) AND btrim(recapito) <> ''),
    altro_id   uuid NOT NULL REFERENCES soggetto_altro,
    attivo     boolean NOT NULL DEFAULT true,   -- spento = non riconosce piu', ma resta la memoria di chi l'ha censito
    creato_il  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_recapito_altro_soggetto ON recapito_altro (altro_id);

-- La chiave esterna vera, come per cliente e fornitore: un id che non punta a niente non puo'
-- esistere. Il CHECK che la lega al tipo e' nella 0017 (usa il valore appena aggiunto).
ALTER TABLE messaggio ADD COLUMN controparte_altro_id uuid REFERENCES soggetto_altro;
-- Finche' il CHECK definitivo non arriva (0017), nessuna riga puo' avere un altro_id: cosi' anche
-- questo file, da solo, lascia un database in cui ck_messaggio_controparte dice ancora tutta la verita'.
ALTER TABLE messaggio ADD CONSTRAINT ck_messaggio_controparte_altro_provvisorio CHECK (controparte_altro_id IS NULL);

-- ============================================================ 2. LA RICHIESTA AL FORNITORE: una risposta non e' un'offerta

-- Fino a qui «Aggancia come risposta» portava la richiesta a `risposta` qualunque cosa il
-- fornitore avesse scritto: «ricevuto, vi rispondiamo domani» chiudeva la richiesta. Da qui lo stato
-- cambia SOLO con la conferma di un atto: `offerta` → offerta_ricevuta; «non quotiamo» →
-- declinata. Una mail collegata alla richiesta, da sola, non cambia lo stato.
-- Prima di rinominare si guarda che cosa c'e'. Una richiesta gia' in `risposta` era stata chiusa
-- con la regola vecchia (una risposta qualunque), e nessuno sa se quella risposta fosse un'offerta:
-- la migrazione NON la reinterpreta. Si ferma, dice quante sono, e chi la lancia le riconcilia a
-- mano (offerta ricevuta? solo collegata?) prima di riprovare. Il 18/09/2026 erano zero, su test e
-- su sviluppo.
DO $$
DECLARE
    n_stato integer;
    n_data  integer;
BEGIN
    SELECT count(*) FILTER (WHERE stato = 'risposta'), count(*) FILTER (WHERE risposta_il IS NOT NULL)
      INTO n_stato, n_data
      FROM richiesta_fornitore;
    IF n_stato > 0 OR n_data > 0 THEN
        RAISE EXCEPTION USING MESSAGE = format(
            '0016: %s richieste in stato risposta e %s con risposta_il valorizzato. La migrazione non le reinterpreta: '
            'riconciliarle a mano (offerta ricevuta, oppure solo collegata) e rilanciare.', n_stato, n_data);
    END IF;
END $$;
ALTER TYPE stato_richiesta_fornitore RENAME VALUE 'risposta' TO 'offerta_ricevuta';
ALTER TYPE stato_richiesta_fornitore ADD VALUE 'declinata' AFTER 'offerta_ricevuta';
ALTER TABLE richiesta_fornitore RENAME COLUMN risposta_il TO offerta_ricevuta_il;
ALTER TABLE richiesta_fornitore ADD COLUMN declinata_il timestamptz;

-- ============================================================ 3. L'ATTO BUSINESS (al posto di intento_messaggio)

-- Che cosa sta facendo il mittente in QUESTO messaggio. E' neutro rispetto alla direzione: e' la
-- coppia (controparte, direzione, atto) a dare il significato. «Cliente in entrata +
-- richiesta_offerta» e' una RFQ; «fornitore in uscita + richiesta_offerta» e' la nostra richiesta a
-- lui. Lo stesso vocabolario per il deterministico e per l'agente: la 0015 ne aveva uno, il
-- pacchetto agente un altro, e questo li sostituisce tutti e due.
--
-- SCELTA DA APPROVARE: tabella catalogo (qui sotto) oppure ENUM (in fondo al file, commentato). Il
-- vocabolario e' ancora in evoluzione: con la tabella un atto nuovo e' un INSERT in un file di
-- migrazione, e un atto ritirato resta leggibile sulle proposte vecchie (attivo = false); con
-- l'ENUM ogni aggiunta vuole ADD VALUE e quindi due file (S4), e un valore non si toglie mai.
-- Il lato in meno della tabella: sqlc non genera le costanti Go, che vanno tenute a mano in
-- `domain` (come gia' per le regole R0–R5, che sono stringhe in Go ed enum in database).
CREATE TABLE atto_business (
    codice      varchar(40)  PRIMARY KEY CHECK (codice ~ '^[a-z][a-z0-9_]*$'),
    descrizione varchar(120) NOT NULL,
    ordine      smallint     NOT NULL,                  -- come li elenca la schermata
    attivo      boolean      NOT NULL DEFAULT true       -- false = ritirato: non si propone piu', si legge ancora
);
INSERT INTO atto_business (codice, descrizione, ordine) VALUES
    ('richiesta_offerta',      'Richiesta d''offerta',                        10),
    ('offerta',                'Offerta',                                     20),
    ('revisione_documenti',    'Revisione di documenti gia'' inviati',         30),
    ('documenti_aggiuntivi',   'Documenti aggiuntivi',                        40),
    ('domanda_chiarimento',    'Domanda di chiarimento',                      50),
    ('risposta_chiarimento',   'Risposta a un chiarimento',                   60),
    ('sollecito',              'Sollecito',                                   70),
    ('ordine',                 'Ordine',                                      80),
    ('accettazione',           'Accettazione',                                90),
    ('rifiuto',                'Rifiuto',                                    100),
    ('conferma_ricezione',     'Conferma di ricezione',                      110),
    ('inoltro',                'Inoltro',                                    120),
    ('notifica',               'Notifica automatica',                        130),
    ('comunicazione_generica', 'Comunicazione generica',                     140),
    ('non_business',           'Non di lavoro (newsletter, cortesie)',       150),
    ('incerto',                'Incerto: decide una persona',                160);

-- La proposta porta l'atto, per tutte e due le fonti (deterministico, agente): il confronto fra
-- le due e' una query sulle due righe dello stesso messaggio, non una tabella in piu'.
ALTER TABLE proposta_triage ADD COLUMN atto varchar(40) REFERENCES atto_business;

-- Le proposte gia' scritte con il vocabolario della 0015 si traducono; la direzione che quel
-- vocabolario portava dentro (rfq_cliente / rfq_fornitore) sta gia' sul messaggio. L'unico
-- valore senza un corrispondente pulito e' `risposta_fornitore`, che era proprio il difetto:
-- «tutto il resto» diventa comunicazione_generica, e il riclassificatore dira' di meglio. E
-- `rfq_cliente` su una proposta «aggancia» (una risposta dentro una RFQ) non e' una richiesta
-- d'offerta: diventa `incerto`, perche' il deterministico l'atto di una risposta non lo sa.
UPDATE proposta_triage SET atto = CASE intento
    WHEN 'rfq_cliente'        THEN CASE WHEN esito = 'nuova_rfq' THEN 'richiesta_offerta' ELSE 'incerto' END
    WHEN 'rfq_fornitore'      THEN 'richiesta_offerta'
    WHEN 'offerta_promatec'   THEN 'offerta'
    WHEN 'offerta_fornitore'  THEN 'offerta'
    WHEN 'risposta_fornitore' THEN 'comunicazione_generica'
    WHEN 'domanda_fornitore'  THEN 'domanda_chiarimento'
    WHEN 'inoltro_interno'    THEN 'inoltro'
    WHEN 'non_rfq'            THEN 'non_business'
    WHEN 'incerto'            THEN 'incerto'
END
WHERE intento IS NOT NULL;
-- `intento` e `intento_messaggio` si tolgono nella 0017, insieme alla vista che li legge.

-- ============================================================ 4. IL LEGAME OPERATIVO (proposta)

-- «Questa mail e' nuova, oppure continua qualcosa che il Cockpit conosce gia'?». E' una PROPOSTA:
-- il fatto resta messaggio.thread_id / richiesta_fornitore_id dopo la decisione. Il bersaglio sta
-- nelle colonne che gia' esistono (thread_proposto, richiesta_proposta); qui c'e' il TIPO del
-- legame. Enum e non tabella: sei valori strutturali, che non cambiano con il mestiere.
CREATE TYPE legame_operativo AS ENUM ('nuovo', 'risposta', 'aggiornamento', 'inoltro', 'nessuno', 'incerto');
ALTER TABLE proposta_triage ADD COLUMN legame legame_operativo;

-- ============================================================ 5. ALTERNATIVA (NON applicata): l'atto come ENUM
--
-- Se si preferisce l'enum, al posto della sezione 3 andrebbe:
--
--   CREATE TYPE atto_business AS ENUM ('richiesta_offerta', 'offerta', 'revisione_documenti',
--       'documenti_aggiuntivi', 'domanda_chiarimento', 'risposta_chiarimento', 'sollecito', 'ordine',
--       'accettazione', 'rifiuto', 'conferma_ricezione', 'inoltro', 'notifica', 'comunicazione_generica',
--       'non_business', 'incerto');
--   ALTER TABLE proposta_triage ADD COLUMN atto atto_business;
--   UPDATE proposta_triage SET atto = (CASE intento ... END)::atto_business WHERE intento IS NOT NULL;
--
-- Pro: sqlc genera le costanti Go e Valid(); un valore scritto male non entra nemmeno dal codice.
-- Contro: ogni atto nuovo e' un ADD VALUE, quindi due file; un atto non si ritira mai; niente
-- descrizione ne' ordine in database (li tiene il Go, o una tabella accanto: e allora tanto vale).

INSERT INTO schema_versione (versione) VALUES (16);
