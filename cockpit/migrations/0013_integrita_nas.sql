-- 0013 — l'integrita' del NAS diventa una cosa che si guarda (blocco 5B del checkpoint 3R).
--
-- Fino a qui il database diceva `stato_nas = 'scritto'` e nessuno andava mai a controllare se quel
-- file ci fosse davvero. E' una promessa fatta una volta sola, nel momento in cui la copia riusciva,
-- e poi mai piu' verificata: dopo di allora la cartella puo' essere stata spostata, il file
-- cancellato, oppure sostituito a mano da qualcun altro con un contenuto diverso. Il fascicolo
-- continuerebbe a dire di si'.
--
-- E il contrario: un documento `in_coda` da settimane — perche' la scrittura era spenta, perche' la
-- copia e' fallita e nessuno se n'e' accorto — non e' distinguibile, guardando la riga, da uno
-- confermato cinque minuti fa.
--
-- Da qui: un ricognitore periodico guarda i file veri, e cio' che non torna finisce QUI, in una riga
-- che l'amministratore vede e su cui puo' agire. Registrare l'anomalia, e non solo scriverla nel log,
-- e' quello che la rende un lavoro che qualcuno puo' prendere in carico.
--
-- Il conflitto — file presente con un contenuto DIVERSO da quello del documento — non ha nessuna
-- azione automatica, e non e' una dimenticanza: sovrascrivere vorrebbe dire buttare via il file di
-- qualcun altro senza sapere di chi fosse. Si segnala e si lascia decidere a una persona.

-- I sei modi in cui il database e il disco possono non essere d'accordo.
--   in_attesa     il documento aspetta la copia da troppo tempo e nessuna copia e' in coda
--   errore        la copia ha provato e non ce l'ha fatta
--   mancante      il documento risulta scritto, il file sul NAS non c'e'
--   conflitto     il file c'e' con un contenuto DIVERSO: non si sovrascrive, si segnala
--   gia_presente  il file c'e' gia' con l'hash giusto mentre il documento risulta ancora da copiare
--   illeggibile   il file c'e' ma non si e' riusciti a leggerlo: non si puo' dire se sia quello giusto
CREATE TYPE problema_nas AS ENUM ('in_attesa','errore','mancante','conflitto','gia_presente','illeggibile');

-- verificato_il: quando il ricognitore ha guardato QUESTO documento l'ultima volta.
--
-- Serve a due cose. La prima e' spazzare a giro: ogni passata prende i documenti guardati meno di
-- recente, cosi' un fascicolo con diecimila file si controlla per intero senza rileggerlo tutto ogni
-- quarto d'ora su una condivisione di rete. La seconda e' poter rispondere alla domanda «quando e'
-- stato controllato?»: senza, una schermata che non segnala niente non si distingue da una che non
-- ha mai guardato.
ALTER TABLE documento ADD COLUMN verificato_il timestamptz;

CREATE TABLE nas_anomalia (
    anomalia_id  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    documento_id uuid NOT NULL REFERENCES documento,
    thread_id    uuid NOT NULL REFERENCES thread_offerta,
    problema     problema_nas NOT NULL,
    stato_db     stato_nas NOT NULL,                   -- com'era il documento quando e' stata rilevata
    percorso     varchar(1000) NOT NULL,               -- relativo alla radice del NAS: dove si e' guardato
    sha_atteso   char(64) NOT NULL,
    sha_trovato  char(64),                             -- solo per 'conflitto' e 'gia_presente'
    dettaglio    text NOT NULL DEFAULT '',
    rilevata_il  timestamptz NOT NULL DEFAULT now(),
    vista_il     timestamptz NOT NULL DEFAULT now(),   -- l'ultima passata che l'ha ancora trovata cosi'
    risolta_il   timestamptz                           -- NULL = aperta
);

-- Un'anomalia aperta per documento, non una per passata: il ricognitore gira ogni quarto d'ora e
-- senza questo indice la stessa cartella mancante diventerebbe novanta righe al giorno, cioe' una
-- schermata che nessuno guarda piu'.
CREATE UNIQUE INDEX ux_nas_anomalia_aperta ON nas_anomalia (documento_id) WHERE risolta_il IS NULL;
CREATE INDEX ix_nas_anomalia_thread ON nas_anomalia (thread_id) WHERE risolta_il IS NULL;

INSERT INTO schema_versione (versione) VALUES (13);
