-- 0009_presenza_contatto.sql — Checkpoint 3R, blocco 2: «online» smette di voler dire «ha appena
-- concluso un claim».
--
-- IL DIFETTO, visto sul sistema vero. `worker_presenza` aveva una sola colonna di tempo, ultimo_claim,
-- e la scriveva un solo punto del codice: l'handler del claim, DOPO `jobs.Claim(...)`. Ma `jobs.Claim`
-- e' un long-poll: quando non c'e' lavoro resta appeso fino a venti secondi prima di rispondere, e
-- quando il lavoro c'e' la riga si riaggiorna solo al claim SUCCESSIVO, cioe' alla fine del job.
-- Quindi:
--
--   * un worker che sta sincronizzando una casella per tre minuti non toccava quella colonna per tre
--     minuti. La testata, che chiamava offline chi non si faceva vivo da sessanta secondi, lo
--     dichiarava OFFLINE proprio mentre stava lavorando;
--   * fra l'accensione del worker e la sua prima risposta passano fino a venti secondi, e in quella
--     finestra la riga non esisteva ancora: chi apriva il browser in quel momento leggeva «il worker
--     non e' mai stato avviato», e restava scritto cosi' fino al primo aggiornamento della pagina.
--
-- Alzare la soglia a centoventi secondi avrebbe fatto sparire il sintomo lasciando intatta la causa:
-- la colonna misurava un'altra cosa. Un claim che si conclude e' un EVENTO DI LAVORO; l'essere vivo
-- e' una proprieta' del worker, e si misura sull'ultima volta che si e' fatto riconoscere.
--
-- DA QUI DUE COLONNE, perche' sono due fatti diversi:
--
--   ultimo_contatto   l'ultima richiesta AUTENTICATA ricevuta da quel worker, qualunque essa sia:
--                     l'INGRESSO di un claim (prima del long-poll, non dopo), un heartbeat, la domanda
--                     su quali caselle servire, un ingest, l'upload di un allegato, i cursori. La
--                     scrive il middleware di autenticazione, che e' l'unico punto attraversato da
--                     tutte le rotte dei worker: un posto solo, e nessuna rotta futura da ricordarsi
--                     di istruire.
--   ultimo_claim      resta, e resta a significare precisamente cio' che diceva: l'ultimo claim
--                     CONCLUSO. Diventa NULL-abile perche' adesso esiste uno stato che prima non si
--                     poteva scrivere — «worker vivo, nessun claim ancora concluso» — e che prima
--                     veniva raccontato con now(), cioe' con una comoda bugia.
--
-- CHI LEGGE CHE COSA. La liveness (stato per casella in testata, chip dell'analisi, abbinamento
-- sessione→postazione per IP) legge ultimo_contatto e la soglia unica `api.PresenzaOnlineEntro`.
-- ultimo_claim resta un dato diagnostico: dice da quanto quel worker non riceve lavoro, che e' una
-- domanda legittima ma diversa da «e' acceso?».

ALTER TABLE worker_presenza ADD COLUMN ultimo_contatto timestamptz NOT NULL DEFAULT now();

-- Le righe gia' scritte sanno una cosa sola, l'ultimo claim: e' anche l'ultimo contatto che di quel
-- worker si sia mai registrato, quindi e' il valore vero da portare avanti. Metterci now() avrebbe
-- dichiarato vivi, per il tempo di una soglia, worker spenti da giorni.
UPDATE worker_presenza SET ultimo_contatto = ultimo_claim;

ALTER TABLE worker_presenza ALTER COLUMN ultimo_claim DROP NOT NULL;
ALTER TABLE worker_presenza ALTER COLUMN ultimo_claim DROP DEFAULT;

COMMENT ON COLUMN worker_presenza.ultimo_contatto IS
    'Ultima richiesta autenticata ricevuta da questo worker, scritta dal middleware di autenticazione PRIMA di servire la rotta: ingresso di un claim, heartbeat, caselle, ingest, upload, cursori. E'' la sola colonna su cui si decide online/offline, con la soglia api.PresenzaOnlineEntro (2 x l''attesa del claim).';
COMMENT ON COLUMN worker_presenza.ultimo_claim IS
    'Ultimo claim CONCLUSO (con o senza job assegnato). NULL = il worker si e'' fatto vivo ma nessun claim si e'' ancora chiuso: e'' lo stato dei primi venti secondi dopo l''accensione. NON usarla per online/offline — un worker dentro un job lungo non conclude claim e resta vivissimo: per quello c''e'' ultimo_contatto.';

INSERT INTO schema_versione (versione) VALUES (9);
