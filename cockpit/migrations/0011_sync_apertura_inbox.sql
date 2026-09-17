-- 0011_sync_apertura_inbox.sql — Checkpoint 3R, blocco 3: l'aggiornamento alla prima apertura
-- dell'Inbox, una volta per sessione.
--
-- IL PROBLEMA. Con `intervallo_sync_s = 0` non si accoda nessun sync periodico, ed e' voluto: sul
-- banco reale non si vuole un worker che macina Outlook da solo. Ma allora chi apre il Cockpit la
-- mattina vede la posta di ieri finche' non preme «Aggiorna ora», e un'Inbox che mostra la posta di
-- ieri e' un'Inbox che nessuno guardera' piu': si riapre Outlook, e da li' in poi il Cockpit e' un
-- doppione.
--
-- PERCHE' NON BASTA L'IDEMPOTENZA CHE C'E' GIA'. La chiave `sync_outlook:<casella>` impedisce due
-- job pendenti insieme, e questo basta contro i dieci clic in dieci secondi. Non basta contro il
-- tasto F5: appena il job precedente si chiude, il refresh successivo ne accoda un altro, e una
-- pagina ricaricata dieci volte in un'ora sono dieci sync che nessuno ha chiesto.
--
-- La domanda giusta non e' «c'e' gia' un job?», e' «questa sessione ha gia' avuto il suo
-- aggiornamento di apertura?». Ed e' una proprieta' della sessione, quindi sta nella sessione.
--
-- PERCHE' NON AL LOGIN. Un admin puo' entrare soltanto per usare l'Anagrafica, e in quel caso non
-- deve svegliare nessun worker. Il momento e' la prima GET operativa dell'Inbox, che e' anche il
-- momento in cui qualcuno sta per guardare quella posta.
--
-- La colonna e' anche il modo in cui l'accodamento resta idempotente sotto carico: si scrive con un
-- UPDATE condizionato (`WHERE sync_inbox_il IS NULL`), e chi ottiene la riga e' uno solo. Due schede
-- aperte nello stesso istante non accodano due volte.

ALTER TABLE sessione ADD COLUMN sync_inbox_il timestamptz;

COMMENT ON COLUMN sessione.sync_inbox_il IS
    'Quando questa sessione ha accodato l''aggiornamento automatico alla prima apertura dell''Inbox. NULL = non ancora, ed e'' la condizione su cui si accoda: il refresh del browser e il poll HTMX non ne accodano un secondo. Si azzera con una sessione nuova, cioe'' con un login nuovo.';

INSERT INTO schema_versione (versione) VALUES (11);
