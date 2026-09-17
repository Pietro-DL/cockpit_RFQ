-- 0008_interpretazione.sql — Checkpoint 3R: l'interpretazione smette di decidere.
--
-- Il banco reale (924 messaggi veri) ha mostrato quattro cose che i test simulati non potevano vedere,
-- tutte figlie della stessa scorciatoia: l'ingest DECIDEVA invece di PROPORRE.
--
--   1. l'aggancio automatico per ConversationID o per codice generico scriveva `messaggio.thread_id`
--      senza che nessuno avesse deciso niente. Un ConversationID dice «questi messaggi si citano»,
--      non «questi messaggi sono la stessa richiesta d'offerta»;
--   2. una risposta certa a una RFQ aperta diventava `nuova_rfq` perche' il punteggio del contenuto
--      superava 50. Il punteggio misura quanto un messaggio SEMBRA una richiesta: non puo' retrocedere
--      una risposta di cui esiste la prova nell'header;
--   3. ogni numero con tre cifre diventava un «codice», e al submit del form Nuova RFQ tutti i codici
--      precompilati diventavano identificativi confermati, con origine `manuale`. Nessuno li aveva
--      digitati;
--   4. un riferimento di richiesta del cliente (RDO 490020618, Anfrage_..., ODA) finiva nello stesso
--      elenco dei codici prodotto, e di li' in `identificativo_thread`.
--
-- Questa migrazione non toglie niente: aggiunge i posti dove l'evidenza puo' stare senza diventare
-- una decisione. I valori `auto_conversazione` e `auto_identificativo` di `aggancio` RESTANO
-- nell'enum: descrivono righe gia' scritte in sviluppo, e cancellare un valore d'enum vorrebbe dire
-- riscrivere la storia di quelle righe. Il codice non li produce piu'.

-- ============================================================================ 1. CANDIDATI DI AGGANCIO
-- Fase 4.1 del piano, anticipata (era il blocco 6 dell'addendum v2): «il sistema propone, l'operatore
-- decide» (D9) smette di essere una frase e diventa una tabella.
--
-- Le sei regole, in ordine di forza. L'ordine e' quello della precedenza del triage, non un peso:
--   R0  In-Reply-To / References che puntano alla chiave esterna di un messaggio gia' agganciato.
--       E' l'unico legame che scrive il client di posta e non l'utente: non si sbaglia per caso.
--   R1  ConversationID di una conversazione gia' agganciata da un operatore. Forte ma non certo:
--       «rispondi» su una mail vecchia per parlare d'altro produce lo stesso ConversationID.
--   R4  riferimento della richiesta secondo il cliente (RDO/Anfrage/ODA) uguale a quello di una RFQ.
--   R3  codice di una FAMIGLIA del cliente gia' identificativo di una RFQ aperta dello stesso cliente.
--   R2  oggetto normalizzato uguale, stesso cliente, dentro `finestra_aggancio_gg`.
--   R5  stesso buyer entro pochi giorni: da solo non basta mai, e serve a mettere in fila i candidati.
--
-- Un candidato verso una RFQ CHIUSA non sparisce: si vede con l'avviso (T2). Nasconderlo significa
-- lasciare l'operatore a creare un doppione di una richiesta che esiste gia'.
CREATE TYPE regola_aggancio AS ENUM ('R0_reply','R1_conversazione','R2_oggetto','R3_codice','R4_riferimento','R5_buyer');

CREATE TABLE candidato_aggancio (
    messaggio_id uuid NOT NULL REFERENCES messaggio,
    thread_id    uuid NOT NULL REFERENCES thread_offerta,
    regola       regola_aggancio NOT NULL,
    punteggio    smallint NOT NULL CHECK (punteggio BETWEEN 0 AND 100),
    evidenza     text NOT NULL,                       -- la frase che l'operatore legge: «In-Reply-To: <...>»
    thread_stato stato_thread NOT NULL,               -- fotografia al calcolo: serve all'avviso «RFQ chiusa»
    creato_il    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (messaggio_id, thread_id, regola)     -- due regole possono indicare lo stesso thread: sono due evidenze
);
CREATE INDEX ix_candidato_aggancio_msg ON candidato_aggancio (messaggio_id, punteggio DESC);

COMMENT ON TABLE candidato_aggancio IS
'Proposte di aggancio con punteggio ed evidenza (fase 4.1, D9). NON e'' una decisione: messaggio.thread_id
si scrive solo quando un operatore preme un bottone. Un messaggio puo'' avere piu'' candidati e li vede
tutti (T16); nessuno viene scelto dal sistema.';

-- ============================================================================ 2. CANDIDATI DI CODICE
-- Un numero trovato in una mail non e' un identificativo della RFQ: e' un numero trovato in una mail.
-- Qui ci sta con il suo RUOLO e la sua PROVENIENZA, e ci resta finche' un operatore non lo spunta.
--
-- `ruolo` separa cio' che il vecchio `EstraiCodici` mescolava:
--   riferimento_rfq    il numero con cui il CLIENTE chiama la richiesta. Non e' un codice prodotto e
--                      non lo diventa mai: ha un campo suo su thread_offerta (sotto).
--   prodotto           codice riconosciuto da una famiglia del cliente.
--   parte              sottoassieme / particolare candidato. Il motore deterministico NON lo propone:
--                      distinguere un finito da un sottoassieme richiede la distinta, che e' il blocco 8.
--                      Il valore esiste perche' l'operatore e l'agente possano usarlo senza migrazione.
--   non_classificato   l'estrattore generico ha visto qualcosa con la forma di un codice. Nient'altro.
--
-- `origine` dice CHI l'ha detto, e serve a non trattare allo stesso modo «questo e' un codice di questo
-- cliente» e «questo ha la forma di un codice».
CREATE TYPE ruolo_codice   AS ENUM ('riferimento_rfq','prodotto','parte','non_classificato');
CREATE TYPE origine_codice AS ENUM ('famiglia','riferimento','generico','nome_file','operatore','agente');

CREATE TABLE candidato_codice (
    messaggio_id uuid NOT NULL REFERENCES messaggio,
    codice       varchar(60) NOT NULL,
    ruolo        ruolo_codice NOT NULL,
    rev          varchar(10) NOT NULL DEFAULT '',
    origine      origine_codice NOT NULL,
    famiglia     varchar(120) NOT NULL DEFAULT '',    -- descrizione della famiglia del cliente che l'ha riconosciuto
    punteggio    smallint NOT NULL CHECK (punteggio BETWEEN 0 AND 100),
    evidenza     text NOT NULL DEFAULT '',            -- dove: «oggetto», «corpo», «allegato 6743449A_1.zip»
    creato_il    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (messaggio_id, codice)                -- UNA riga per codice: il ruolo e' uno solo, lo decide la precedenza
);
CREATE INDEX ix_candidato_codice_ruolo ON candidato_codice (messaggio_id, ruolo, punteggio DESC);

COMMENT ON TABLE candidato_codice IS
'Numeri trovati in un messaggio, con ruolo e provenienza. La chiave primaria e'' (messaggio, codice) senza
il ruolo apposta: lo stesso testo non puo'' essere insieme il riferimento della richiesta e un codice
prodotto. Le regole del cliente vengono prima dell''estrattore generico, quindi il ruolo lo decide la
precedenza, non l''ordine di inserimento.';

-- Il log degli agganci impara una parola nuova. `propaga` (una decisione su un messaggio che ne
-- trascinava altri) resta nel vincolo perche' descrive righe gia' scritte; il codice non la produce piu'.
-- `candidato` e' cio' che succede adesso agli altri orfani di una conversazione: una proposta, tracciata.
ALTER TABLE messaggio_aggancio_log DROP CONSTRAINT ck_aggancio_log_azione;
ALTER TABLE messaggio_aggancio_log ADD CONSTRAINT ck_aggancio_log_azione
    CHECK (azione IN ('aggancia','sgancia','ignora','propaga','candidato'));

-- ============================================================================ 3. IL RIFERIMENTO DEL CLIENTE
-- «RDO 490020618» e' il nome che il cliente da' alla richiesta. Finora finiva fra gli identificativi,
-- cioe' fra i codici dei pezzi, e di li' nel nome delle cartelle e nelle ricerche per codice.
ALTER TABLE thread_offerta ADD COLUMN riferimento_cliente varchar(60);
CREATE INDEX ix_thread_riferimento ON thread_offerta (cliente_id, upper(riferimento_cliente))
    WHERE riferimento_cliente IS NOT NULL;
COMMENT ON COLUMN thread_offerta.riferimento_cliente IS
'Il numero con cui il cliente chiama questa richiesta (RDO, Anfrage, ODA), riconosciuto da
cliente.regole.riferimento_rfq. Non e'' un codice prodotto: alimenta la regola di aggancio R4, non
identificativo_thread.';

-- ============================================================================ 4. PDF NON SIGNIFICA DISEGNO
-- `disegno_2d` per estensione era una affermazione tecnica su un file che nessuno aveva aperto. Il tipo
-- di un PDF si sa dopo l'analisi; prima si dice che non si sa.
ALTER TYPE tipo_documento ADD VALUE 'da_determinare';
-- Nessuna riga in `cartella_documento` per `da_determinare`, ed e' voluto: un documento di tipo ignoto
-- non ha una destinazione sul NAS, e la conferma lo rifiuta con un messaggio (non con un errore di query).

-- ============================================================================ 5. ORIGINE DEGLI IDENTIFICATIVI
-- Un codice che entra nella RFQ perche' l'operatore ha spuntato una casella NON e' `manuale`: manuale e'
-- quello che ha digitato lui. La differenza serve il giorno in cui si misura quanto il motore ci prende.
ALTER TYPE origine_identificativo ADD VALUE 'proposta_famiglia';
ALTER TYPE origine_identificativo ADD VALUE 'proposta_generico';

-- ============================================================================ 6. ANALISI SEMANTICA (agente)
-- Terzo strato dopo i fatti e il deterministico: propone e basta. Non scrive `thread_id`, non conferma
-- articoli, non tocca il NAS, non invia posta. Qui ci sta il suo output, verificato dal server contro il
-- testo del messaggio: cio' che l'agente cita ma nel messaggio non c'e' viene scartato e registrato.
CREATE TYPE stato_analisi AS ENUM ('in_corso','completata','rifiutata','errore');

CREATE TABLE analisi_messaggio (
    analisi_id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    messaggio_id uuid NOT NULL REFERENCES messaggio,
    input_hash   char(64) NOT NULL,                   -- sha256 di cio' che e' stato mandato: la rianalisi e' riproducibile
    versione     varchar(40) NOT NULL,                -- versione dell'analizzatore (codice Go)
    prompt       varchar(40) NOT NULL,                -- versione del prompt
    modello      varchar(60) NOT NULL,
    stato        stato_analisi NOT NULL DEFAULT 'in_corso',
    risultato    jsonb NOT NULL DEFAULT '{}',         -- output DOPO schema e grounding: e' cio' che la UI mostra
    grezzo       jsonb,                               -- output come e' arrivato: serve a spiegare cosa e' stato scartato
    scartato     jsonb NOT NULL DEFAULT '[]',         -- [{"campo":...,"valore":...,"motivo":"non presente nel messaggio"}]
    errore       text,
    token_in     integer,
    token_out    integer,
    durata_ms    integer,
    creato_il    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (messaggio_id, input_hash, prompt, modello) -- stesso input, stesso prompt, stesso modello: una volta sola
);
CREATE INDEX ix_analisi_messaggio ON analisi_messaggio (messaggio_id, creato_il DESC);

COMMENT ON TABLE analisi_messaggio IS
'Proposte dell''agente semantico. `risultato` e'' gia'' passato per lo schema e per il grounding in Go: ogni
codice e ogni riferimento citati sono stati ritrovati nel testo del messaggio, nei nomi degli allegati o nel
database. Cio'' che non ha riscontro sta in `scartato` con il motivo, e non raggiunge la UI. Lo stato
`rifiutata` significa che l''output non rispettava lo schema: nessun effetto, e l''analisi resta come prova.';

ALTER TYPE tipo_job ADD VALUE 'analizza_messaggio_ai';

INSERT INTO schema_versione (versione) VALUES (8);
