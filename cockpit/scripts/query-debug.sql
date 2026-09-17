-- Le query della sessione di debug, una per passo, nell'ordine in cui servono.
--
--   psql "postgres://cockpit:PASSWORD@127.0.0.1:5433/cockpit_dev" -f scripts\query-debug.sql
--
-- oppure, per lanciarne una sola, aprire psql e usare \i. Sono di sola lettura: nessuna di queste
-- righe scrive niente.
--
-- Il file sta nel repository perché la diagnosi di un job si fa sempre con le stesse cinque
-- domande, e ricordarsele a memoria a schermo condiviso è tempo perso.

\echo '== 1. la coda, come la vede il server =========================================='
-- ha_token e lease_fino_a dicono se il job è davvero in mano a qualcuno; tentativi > 1 significa
-- che qualcosa lo sta facendo ripetere (lease scaduto, oppure un result che non viene accettato).
SELECT job_id, tipo, stato, tentativi, worker_id, lease_token IS NOT NULL AS ha_token,
       lease_fino_a, avviato_il, non_prima_di, left(coalesce(errore, ''), 80) AS errore
FROM job
ORDER BY job_id DESC
LIMIT 20;

\echo '== 2. i cursori: si muovono solo con il sync ordinario ========================='
-- Le due frontiere e la misura (0010). Si leggono cosi':
--
--   coperto_fino_a   fin dove Outlook e' stato SCANDITO per intero. E' lei a decidere da dove parte
--                    il prossimo aggiornamento. Avanza solo a finestra conclusa: se resta ferma
--                    mentre arrivano messaggi, le finestre si stanno interrompendo a meta' (cerca
--                    "finestra non conclusa" nel log del server). NULL = mai sincronizzata: il
--                    prossimo sync e' un bootstrap di giorni_sync_iniziale;
--   storico_fino_a   fin dove indietro e' arrivato "Carica precedenti". Scende di GiorniStorico per
--                    clic, e solo a finestra conclusa;
--   ultimo_received  la mail piu' recente che abbiamo. Avanza per lotto e NON decide nessuna
--                    finestra: puo' restare indietro rispetto alla copertura per giorni, e vuol dire
--                    solo che in quei giorni non e' arrivato niente.
--
-- Un sync storico non tocca coperto_fino_a, e un aggiornamento non tocca storico_fino_a: e' voluto.
SELECT c.nome, s.cartella, s.coperto_fino_a, s.storico_fino_a, s.ultimo_received, s.ultimo_sync, s.errore
FROM sync_cursore s
JOIN casella c USING (casella_id)
ORDER BY c.nome, s.cartella;

\echo '== 3. chi è collegato e che cosa vede =========================================='
-- caselle_aperte è ciò che il worker ha risolto nel proprio profilo Outlook E che la sua
-- credenziale autorizza. avviso elenca quelle dichiarate ma non autorizzate.
-- ultimo_contatto = l'ultima richiesta autenticata ricevuta da quel worker: è su questa che la testata
-- dice online/offline. ultimo_claim = l'ultimo claim CONCLUSO, NULL se non se n'è concluso ancora
-- nessuno; un worker dentro un job lungo ha un ultimo_claim vecchio ed è vivissimo.
SELECT worker_nome, p.nome_host AS postazione, w.indirizzo_ip, w.outlook_ok,
       array_length(w.caselle_aperte, 1) AS n_caselle, w.ultimo_contatto, w.ultimo_claim, w.ultimo_job_il,
       left(coalesce(w.avviso, ''), 60) AS avviso, left(coalesce(w.ultimo_arresto, ''), 60) AS ultimo_arresto
FROM worker_presenza w
LEFT JOIN postazione p USING (postazione_id)
ORDER BY w.worker_nome;

\echo '== 4. una mail, due presenze: il controllo I3 =================================='
-- La stessa mail arrivata a due caselle censite deve stare in UNA riga di messaggio con DUE righe
-- di messaggio_casella. Se compare due volte in messaggio, la deduplicazione non ha funzionato.
SELECT m.messaggio_id, left(m.oggetto, 50) AS oggetto, m.data_evento,
       array_agg(c.nome ORDER BY c.nome) AS caselle
FROM messaggio m
JOIN messaggio_casella mc USING (messaggio_id)
JOIN casella c USING (casella_id)
GROUP BY m.messaggio_id, m.oggetto, m.data_evento
HAVING count(*) > 1
ORDER BY m.data_evento DESC
LIMIT 10;

\echo '== 5. che cosa è stato scartato in ingest ======================================'
SELECT scarto_id, origine, casella_id, left(coalesce(oggetto, ''), 40) AS oggetto,
       left(errore, 80) AS errore, tentativi, ultimo_il
FROM ingest_scarto
ORDER BY ultimo_il DESC
LIMIT 10;

\echo '== 6. gli allegati scaricati e il loro stato ==================================='
-- Il passo «Scarica» è finito quando lo stato non è più grezzo e path_staging punta a un file che
-- esiste davvero sul disco del SERVER (il worker lo ha caricato con PUT, non copiato).
SELECT a.allegato_id, a.nome_file, a.stato, a.bytes, left(coalesce(a.sha256, ''), 12) AS sha,
       a.path_staging IS NOT NULL AS in_staging, left(coalesce(a.errore, ''), 60) AS errore
FROM allegato a
ORDER BY a.ricevuto_il DESC
LIMIT 15;

\echo '== 7. TZ2: l ora che Outlook mostra accanto a quella in database =============='
-- Si apre Outlook, si guarda la colonna «Ricevuto» della stessa riga (stesso oggetto, stesso
-- mittente) e la si confronta con `ricevuto_locale`. Devono coincidere al minuto: è l'ultima riga
-- che manca per dire che la correzione del fuso del 16/09 vale anche su Outlook vero, e non solo su
-- una data costruita a mano in un test.
--
-- In database `ricevuto_il` è un timestamptz, cioè un ISTANTE, e psql lo mostra nel fuso della
-- sessione: `AT TIME ZONE` lo riporta all'orologio di chi sta guardando Outlook, che è il confronto
-- che conta. `nel_futuro` è il sintomo di quel difetto (due ore avanti): se compare un SI, l'ora non
-- è plausibile e l'ingest ha fermato l'elemento invece di farlo entrare.
SELECT c.nome AS casella, left(coalesce(m.oggetto, ''), 45) AS oggetto,
       coalesce(m.mittente_indirizzo, '') AS mittente,
       (mc.ricevuto_il AT TIME ZONE 'Europe/Rome')::timestamp(0) AS ricevuto_locale,
       (now() AT TIME ZONE 'Europe/Rome')::timestamp(0) AS adesso_locale,
       CASE WHEN mc.ricevuto_il > now() + interval '5 minutes' THEN 'SI' ELSE '' END AS nel_futuro
FROM messaggio_casella mc
JOIN messaggio m USING (messaggio_id)
JOIN casella c USING (casella_id)
ORDER BY mc.ricevuto_il DESC
LIMIT 15;
