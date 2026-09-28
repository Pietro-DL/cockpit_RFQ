-- Inbox viva (voce 2.16): che cosa è arrivato da quando l'operatore ha guardato l'ultima volta, e a
-- che punto è il sync di ogni casella.
--
-- Nessuna di queste interrogazioni è un'autorizzazione: dicono quante mail sono entrate e quando, non
-- chi può leggerle. La visibilità resta la regola restrittiva di D12 (voce 2.15).

-- name: ToccaVistaInbox :exec
-- Chiamata quando l'Inbox viene APERTA davvero (pagina intera), mai ai poll HTMX: un contatore
-- azzerato ogni 15 secondi direbbe sempre zero.
UPDATE utente SET ultima_vista_inbox = now() WHERE utente_id = $1;

-- name: ContaNuoveDallaVisita :one
-- Il totale: un messaggio presente in due caselle è UNO, non due (la stessa regola di v_inbox).
-- `da` NULL = non ha mai aperto l'Inbox: è tutto nuovo.
SELECT count(*)::int FROM messaggio
WHERE registrato_il > COALESCE(sqlc.narg(da)::timestamptz, '-infinity'::timestamptz);

-- name: ContaNuovePerCasella :many
-- Lo stesso conteggio spezzato per casella, per la chip in testata: «Commerciale · 3 nuove». Qui la
-- stessa mail arrivata a due caselle conta una volta per ciascuna, perché è nuova in entrambe.
SELECT mc.casella_id, count(DISTINCT mc.messaggio_id)::int AS n
FROM messaggio_casella mc
JOIN messaggio m ON m.messaggio_id = mc.messaggio_id
WHERE m.registrato_il > COALESCE(sqlc.narg(da)::timestamptz, '-infinity'::timestamptz)
GROUP BY mc.casella_id;

-- name: ListMessaggiNuoviDa :many
-- Gli id da segnare con il pallino nella lista. Con un limite: se qualcuno rientra dopo le ferie e
-- trova duemila messaggi nuovi, il pallino su tutti non aggiunge informazione e la query sì.
SELECT messaggio_id FROM messaggio
WHERE registrato_il > COALESCE(sqlc.narg(da)::timestamptz, '-infinity'::timestamptz)
ORDER BY registrato_il DESC LIMIT sqlc.arg(limite);

-- name: UltimoSyncPerCasella :many
-- Quando è finito l'ultimo sync RIUSCITO di ogni casella.
--
-- La fonte è il JOB, non `sync_cursore.ultimo_sync`: il cursore si muove solo quando arriva un lotto,
-- quindi un sync che non trova niente di nuovo — il caso normale, una volta a regime — lascerebbe il
-- cursore fermo e la testata direbbe «ultimo sync 12:11» per ore mentre il worker lavora regolarmente.
--
-- Una casella che non ha mai sincronizzato non compare: non c'è una riga con «mai», c'è l'assenza
-- della riga, ed è per questo che il `max()` qui non è mai NULL.
SELECT casella_id, max(chiuso_il)::timestamptz AS ultimo_sync
FROM job
WHERE tipo = 'sync_outlook' AND stato = 'fatto' AND casella_id IS NOT NULL AND chiuso_il IS NOT NULL
GROUP BY casella_id;

-- name: CaselleConSyncInCorso :many
-- Le caselle che stanno sincronizzando adesso (o che hanno un sync in coda): è ciò che fa comparire
-- «in corso» in testata e accelera il poll della testata finché non finisce.
SELECT DISTINCT casella_id FROM job
WHERE tipo = 'sync_outlook' AND stato IN ('pronto','in_corso') AND casella_id IS NOT NULL;
