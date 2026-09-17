-- 0010_copertura_sync.sql — Checkpoint 3R, blocco 3: «fin dove siamo arrivati» smette di voler dire
-- «qual e' la mail piu' recente che abbiamo».
--
-- IL DIFETTO. `sync_cursore.ultimo_received` e' il ReceivedTime dell'ultimo messaggio CONSEGNATO, e
-- veniva usato come frontiera: la finestra del sync successivo partiva da li'. Sono due cose diverse,
-- e si separano appena la finestra ha una coda senza posta:
--
--   * se fra le 17:00 e le 09:00 non e' arrivato niente, `ultimo_received` resta alle 17:00 anche
--     dopo che il worker ha scandito per intero la notte. Quella notte viene rispedita alla lettura
--     ogni volta, per sempre: il sistema non ha modo di ricordare di averla gia' guardata;
--   * e nel verso opposto — quello grave — `ultimo_received` avanzava A OGNI LOTTO. Finche' la
--     lettura e' in ordine CRESCENTE questo e' sicuro: cio' che si e' consegnato e' il pezzo vecchio
--     della finestra, e un'interruzione lascia indietro solo il pezzo nuovo, che il cursore non ha
--     superato. Ma la voce chiesta dal blocco 3 e' la lettura dal PIU' RECENTE al PIU' VECCHIO, e li'
--     lo stesso avanzamento per lotto diventa una perdita: il primo lotto contiene la mail piu'
--     nuova, il cursore salta in cima alla finestra, e se il worker muore subito dopo, tutto cio' che
--     sta sotto non verra' mai piu' letto da nessuno.
--
-- LA SEPARAZIONE. Due colonne perche' sono due fatti, e nessuno dei due si deduce dall'altro:
--
--   ultimo_received   la mail piu' recente che abbiamo di quella (casella, cartella). Semantica
--                     INVARIATA — continua ad avanzare per lotto, in transazione con gli elementi —
--                     ma smette di decidere le finestre. Resta un dato diagnostico e da mostrare;
--   coperto_fino_a    fin dove Outlook e' stato scandito PER INTERO. Avanza solo a finestra
--                     conclusa, mai per lotto, e solo se il worker dichiara di averla percorsa
--                     tutta. Vale anche quando in quella coda di finestra non c'era nessuna mail:
--                     e' il ricordo di aver guardato, non di aver trovato.
--
-- Insieme a `storico_fino_a`, che esiste dalla 0001 e non cambia, il modello e' a due frontiere:
--
--     PASSATO                                                          PRESENTE
--          storico_fino_a  [ quello che il Cockpit ha ]  coperto_fino_a
--                 <- «Carica precedenti»            «Aggiorna» ->
--
-- «Carica precedenti» muove solo la frontiera sinistra, «Aggiorna» solo la destra. Il primo sync di
-- una (casella, cartella) — il bootstrap — e' l'unico momento in cui ne nascono due insieme.

ALTER TABLE sync_cursore ADD COLUMN coperto_fino_a timestamptz;

-- Il travaso. La copertura di una riga gia' esistente e' `ultimo_received`: non e' tutta la verita'
-- — la coda muta delle finestre gia' lette e' coperta senza che nessuno l'abbia scritto da nessuna
-- parte — ma e' esattamente la frontiera che il sistema stava gia' usando per decidere da dove
-- ripartire. Portarla avanti tale e quale significa che la migrazione non cambia da sola nemmeno una
-- finestra: ne' rilegge mesi, ne' dichiara coperto qualcosa che non lo era.
--
-- L'alternativa scartata era now(): avrebbe dichiarato scandito fino a questo istante tutto cio' che
-- nessuno ha mai letto, cioe' un buco silenzioso grande quanto il tempo passato dall'ultimo sync.
UPDATE sync_cursore SET coperto_fino_a = ultimo_received WHERE ultimo_received IS NOT NULL;

COMMENT ON COLUMN sync_cursore.coperto_fino_a IS
    'Frontiera RECENTE: fin dove questa (casella, cartella) e'' stata scandita per intero, anche se nell''ultimo tratto non c''era nessuna mail. Avanza SOLO a finestra conclusa e solo su dichiarazione esplicita del worker (RisultatoSync.cartelle[].completa), mai per lotto: in lettura dal piu'' recente al piu'' vecchio un avanzamento per lotto salterebbe in cima alla finestra e renderebbe invisibile tutto cio'' che sta sotto. NULL = mai sincronizzata: il prossimo sync e'' un bootstrap.';
COMMENT ON COLUMN sync_cursore.ultimo_received IS
    'La mail piu'' recente che abbiamo di questa (casella, cartella). Avanza per lotto, in transazione con gli elementi. NON e'' una frontiera di copertura e dalla 0010 non decide piu'' le finestre: per quello c''e'' coperto_fino_a.';
COMMENT ON COLUMN sync_cursore.storico_fino_a IS
    'Frontiera PASSATA: fin dove indietro e'' arrivato «Carica precedenti». Ogni clic estende di GiorniStorico e la scrive solo a finestra conclusa. Indipendente da coperto_fino_a: le due frontiere si muovono in direzioni opposte e non si toccano mai, salvo il bootstrap che le stabilisce entrambe.';

INSERT INTO schema_versione (versione) VALUES (10);
