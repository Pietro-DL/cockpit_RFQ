-- name: UpsertConversazione :one
INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
VALUES ($1, $2, $3)
ON CONFLICT (canale, chiave_esterna) DO UPDATE
    SET primo_messaggio_il = LEAST(conversazione.primo_messaggio_il, EXCLUDED.primo_messaggio_il)
RETURNING *;

-- name: GetConversazione :one
SELECT * FROM conversazione WHERE conversazione_id = $1;

-- name: CollegaConversazione :exec
UPDATE conversazione SET thread_id = $2, collegata_da = $3 WHERE conversazione_id = $1;

-- name: UpsertMessaggio :one
-- La direzione si CONSOLIDA: la stessa mail può arrivare da due caselle, e una copia che ci risulta
-- in entrata non deve cancellare il fatto che l'abbiamo mandata noi. «Uscita» vince (voce 2.1, D10).
-- `interno` è separato dalla direzione: una mail fra colleghi è in uscita ma non è traffico con il
-- cliente, e confondere le due cose farebbe nascere RFQ da conversazioni interne.
INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, parent_messaggio_id, direzione, data_evento,
                       mittente_nome, mittente_indirizzo, buyer_id, destinatari, oggetto, corpo_testo, corpo_html,
                       lingua, importanza, nota_operatore, registrato_da, interno)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
ON CONFLICT (canale, chiave_esterna) DO UPDATE SET
    conversazione_id = EXCLUDED.conversazione_id,
    direzione        = CASE WHEN 'uscita' IN (messaggio.direzione, EXCLUDED.direzione)
                            THEN 'uscita'::direzione ELSE EXCLUDED.direzione END,
    interno          = EXCLUDED.interno,
    mittente_nome    = COALESCE(EXCLUDED.mittente_nome, messaggio.mittente_nome),
    destinatari      = EXCLUDED.destinatari,
    corpo_testo      = COALESCE(EXCLUDED.corpo_testo, messaggio.corpo_testo),
    corpo_html       = COALESCE(EXCLUDED.corpo_html, messaggio.corpo_html),
    importanza       = COALESCE(EXCLUDED.importanza, messaggio.importanza),
    buyer_id         = COALESCE(messaggio.buyer_id, EXCLUDED.buyer_id)
RETURNING *, (xmax = 0) AS inserito;

-- name: UpsertMessaggioOutlook :exec
-- Solo ciò che è del MESSAGGIO. Entry_id, cartella, categorie, non_letto e flag_stato sono della
-- COPIA e stanno in messaggio_casella dalla 0004: tenerli qui significava che la seconda casella
-- sovrascriveva l'EntryID della prima, e «Apri in Outlook» apriva l'elemento sbagliato.
INSERT INTO messaggio_outlook (messaggio_id, conversation_id, conversation_index, in_reply_to, riferimenti)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (messaggio_id) DO UPDATE SET
    in_reply_to = COALESCE(EXCLUDED.in_reply_to, messaggio_outlook.in_reply_to),
    riferimenti = COALESCE(EXCLUDED.riferimenti, messaggio_outlook.riferimenti),
    aggiornato_il = now();

-- name: UpsertPresenza :exec
-- La presenza di un messaggio in una casella (voce 2.1). Due caselle = due righe, ognuna con il suo
-- EntryID, la sua cartella e il suo stato di lettura: sono fatti di quella copia, non del messaggio.
-- Nessuno store_id (0005): lo StoreID è del profilo Outlook della postazione, non della copia, e sta
-- in casella_store dove ogni worker lo scrive per sé (voce 2.6, N44).
INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, cartella, ricevuto_il, non_letto, flag_stato, categorie)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (messaggio_id, casella_id) DO UPDATE SET
    entry_id = EXCLUDED.entry_id, cartella = EXCLUDED.cartella, ricevuto_il = EXCLUDED.ricevuto_il,
    non_letto = EXCLUDED.non_letto, flag_stato = EXCLUDED.flag_stato, categorie = EXCLUDED.categorie,
    aggiornato_il = now();

-- name: ListPresenze :many
SELECT mc.*, c.nome AS casella_nome, c.indirizzo AS casella_indirizzo, c.condivisa
FROM messaggio_casella mc JOIN casella c ON c.casella_id = mc.casella_id
WHERE mc.messaggio_id = $1 ORDER BY c.nome;

-- name: GetPresenza :one
SELECT * FROM messaggio_casella WHERE messaggio_id = $1 AND casella_id = $2;

-- name: PresenzaDaAprire :one
-- La copia «di riferimento» di un messaggio quando NON c'è una postazione da cui decidere: la più
-- vecchia, a parità la casella con l'uuid minore. Arbitraria ma RIPETIBILE. Dalla voce 2.7 le azioni
-- interattive non la usano più: usano CopiaPerPostazione, e senza postazione non partono. Resta per
-- il download (che non apre finestre e può farlo qualunque worker autorizzato sulla casella) e per la
-- lettura del pannello.
SELECT mc.*, c.nome AS casella_nome, c.indirizzo AS casella_indirizzo
FROM messaggio_casella mc
JOIN casella c ON c.casella_id = mc.casella_id
WHERE mc.messaggio_id = $1 AND c.attiva
ORDER BY mc.ricevuto_il, mc.casella_id
LIMIT 1;

-- name: CopiaPerPostazione :one
-- ROUTING DEI JOB INTERATTIVI (voci 2.2 e 2.7, M3). La copia del messaggio che il worker Outlook di
-- QUESTA postazione serve: presente in una casella attiva su cui la credenziale del worker della
-- postazione è autorizzata. Se il messaggio è in più caselle servite, si preferisce la casella
-- personale del richiedente, poi la copia più vecchia. Zero righe = nessun worker idoneo su quella
-- postazione: il job NON si crea e l'operatore legge il motivo (M4, M10). Non si ripiega su un'altra
-- postazione: una finestra aperta su un altro PC non è un'azione riuscita, è un'azione sbagliata.
SELECT mc.*, c.nome AS casella_nome, c.indirizzo AS casella_indirizzo, w.worker_nome
FROM messaggio_casella mc
JOIN casella c ON c.casella_id = mc.casella_id
JOIN worker_credenziale w ON w.attivo AND w.worker_tipo = 'outlook'
     AND w.postazione_id = sqlc.arg(postazione_id) AND mc.casella_id = ANY (w.caselle)
WHERE mc.messaggio_id = sqlc.arg(messaggio_id) AND c.attiva
ORDER BY (c.utente_id IS NOT NULL AND c.utente_id = sqlc.narg(richiedente)::uuid) DESC, mc.ricevuto_il, mc.casella_id
LIMIT 1;

-- name: CaselleServiteDaPostazione :many
-- Le caselle su cui i worker Outlook di una postazione sono autorizzati: serve a dire PERCHÉ un'azione
-- non parte («il worker di PC-FRANCESCO non serve Commerciale»), non solo che non parte.
SELECT DISTINCT c.*
FROM worker_credenziale w JOIN casella c ON c.casella_id = ANY (w.caselle)
WHERE w.attivo AND w.worker_tipo = 'outlook' AND w.postazione_id = $1
ORDER BY c.nome;

-- name: SetEntryIDPresenza :exec
UPDATE messaggio_casella SET entry_id = sqlc.arg(entry_id), cartella = COALESCE(sqlc.narg(cartella), cartella),
    aggiornato_il = now()
WHERE messaggio_id = sqlc.arg(messaggio_id) AND casella_id = sqlc.arg(casella_id);

-- name: SetNonLettoPresenza :exec
UPDATE messaggio_casella SET non_letto = $3, aggiornato_il = now()
WHERE messaggio_id = $1 AND casella_id = $2;


-- name: GetMessaggio :one
SELECT * FROM messaggio WHERE messaggio_id = $1;

-- name: BloccaMessaggio :one
-- Come GetMessaggio, ma la riga resta bloccata fino alla fine della transazione (voce 1.9, T13).
-- Serve a ogni percorso che DECIDE qualcosa sul messaggio: nuova RFQ, aggancio, sgancio, ignora.
-- Senza, due operatori che premono "Nuova RFQ" sullo stesso messaggio nello stesso momento leggono
-- entrambi thread_id NULL, creano due RFQ e solo una resta agganciata: l'altra rimane in giro vuota,
-- con la sua cartella sul NAS, e nessuno dei due operatori vede un errore. Con il blocco il secondo
-- aspetta, rilegge il messaggio già agganciato e riceve un esito esplicito.
SELECT * FROM messaggio WHERE messaggio_id = $1 FOR UPDATE;

-- name: GetMessaggioPerChiave :one
SELECT * FROM messaggio WHERE canale = $1 AND chiave_esterna = $2;

-- name: GetMessaggioOutlook :one
SELECT * FROM messaggio_outlook WHERE messaggio_id = $1;

-- name: GetInboxRiga :one
SELECT * FROM v_inbox WHERE messaggio_id = $1;

-- name: ListInbox :many
-- `casella` vuoto = tutte le caselle. NON è un controllo di autorizzazione: è il filtro della
-- schermata. Chi può vedere che cosa è la voce 2.2 (puoVedere) e resta restrittivo fino ad allora.
SELECT * FROM v_inbox
WHERE (sqlc.arg(filtro)::text = 'tutti'
    OR (sqlc.arg(filtro)::text = 'orfani'     AND thread_id IS NULL AND NOT ignorato)
    OR (sqlc.arg(filtro)::text = 'agganciati' AND thread_id IS NOT NULL)
    OR (sqlc.arg(filtro)::text = 'ignorati'   AND thread_id IS NULL AND ignorato))
  AND (sqlc.narg(casella)::uuid IS NULL OR sqlc.narg(casella)::uuid = ANY (caselle_id))
ORDER BY data_evento DESC
LIMIT sqlc.arg(limite) OFFSET sqlc.arg(salta);

-- name: ContaInbox :one
SELECT count(*) FILTER (WHERE thread_id IS NULL AND NOT ignorato) AS orfani,
       count(*) FILTER (WHERE thread_id IS NOT NULL)             AS agganciati,
       count(*) FILTER (WHERE thread_id IS NULL AND ignorato)     AS ignorati,
       count(*)                                                  AS tutti
FROM v_inbox
WHERE sqlc.narg(casella)::uuid IS NULL OR sqlc.narg(casella)::uuid = ANY (caselle_id);

-- name: ListMessaggiThread :many
SELECT * FROM messaggio WHERE thread_id = $1 ORDER BY data_evento;

-- name: ListMessaggiConversazione :many
SELECT * FROM messaggio WHERE conversazione_id = $1 ORDER BY data_evento;

-- name: ThreadDellaConversazione :one
SELECT thread_id FROM messaggio
WHERE conversazione_id = $1 AND thread_id IS NOT NULL
ORDER BY data_evento DESC LIMIT 1;

-- name: AgganciaMessaggio :exec
UPDATE messaggio SET thread_id = $2, aggancio = $3, agganciato_da = $4, agganciato_il = now()
WHERE messaggio_id = $1;

-- name: SgangiaMessaggio :exec
UPDATE messaggio SET thread_id = NULL, aggancio = 'nessuno', agganciato_da = $2, agganciato_il = now()
WHERE messaggio_id = $1;

-- name: SetBuyerMessaggiPerIndirizzo :execrows
UPDATE messaggio SET buyer_id = $2 WHERE lower(mittente_indirizzo) = lower($1) AND buyer_id IS NULL;

-- Il cursore ha per chiave (casella, cartella) dalla 0004. Con la sola cartella due caselle che hanno
-- entrambe una «Posta in arrivo» si facevano avanzare il cursore a vicenda, e ogni avanzamento di
-- troppo è una finestra di tempo che l'altra casella non legge mai: messaggi persi in silenzio.

-- name: GetSyncCursore :one
SELECT * FROM sync_cursore WHERE casella_id = $1 AND cartella = $2;

-- name: ListSyncCursori :many
SELECT sc.*, c.indirizzo AS casella_indirizzo, c.nome AS casella_nome
FROM sync_cursore sc JOIN casella c ON c.casella_id = sc.casella_id
ORDER BY c.indirizzo, sc.cartella;

-- name: ListSyncCursoriCasella :many
SELECT * FROM sync_cursore WHERE casella_id = $1 ORDER BY cartella;

-- name: UpsertSyncCursore :exec
-- `ultimo_received` e' la mail piu' RECENTE che abbiamo di quella (casella, cartella), non fin dove
-- si e' guardato: avanza per lotto, in transazione con gli elementi, e GREATEST impedisce che due
-- tentativi la facciano arretrare (Q22). Dalla 0010 non decide piu' nessuna finestra — la frontiera
-- e' `coperto_fino_a`, che si scrive altrove e a condizioni molto piu' severe.
INSERT INTO sync_cursore (casella_id, cartella, ultimo_received, ultimo_sync, n_messaggi, errore)
VALUES ($1, $2, $3, now(), $4, $5)
ON CONFLICT (casella_id, cartella) DO UPDATE SET
    ultimo_received = GREATEST(COALESCE(sync_cursore.ultimo_received, EXCLUDED.ultimo_received), EXCLUDED.ultimo_received),
    ultimo_sync = now(), n_messaggi = sync_cursore.n_messaggi + EXCLUDED.n_messaggi, errore = EXCLUDED.errore;

-- name: SetCopertoFinoA :exec
-- La frontiera RECENTE: fin dove Outlook e' stato scandito per intero (0010).
--
-- La scrive un punto solo, `applicaRisultato`, e solo per le cartelle che il worker ha dichiarato
-- COMPLETE: non a ogni lotto, e non perche' il job HTTP e' finito senza eccezioni. In lettura dal
-- piu' recente al piu' vecchio il primo lotto contiene gia' la mail piu' nuova della finestra: farla
-- avanzare li' significherebbe dichiarare coperto tutto l'intervallo prima ancora di averlo letto,
-- e un worker che muore subito dopo lascerebbe invisibile per sempre tutto cio' che sta sotto.
--
-- E' un INSERT come SetStoricoFinoA e per lo stesso motivo: una (casella, cartella) sincronizzata
-- per la prima volta in una finestra dove non c'era nessuna mail non ha ancora una riga, e proprio
-- quello e' il caso che questa colonna esiste per ricordare.
--
-- GREATEST: due tentativi non possono farla arretrare. Arretrare sarebbe innocuo (si rilegge), ma
-- renderebbe illeggibile il significato della colonna.
INSERT INTO sync_cursore (casella_id, cartella, coperto_fino_a)
VALUES ($1, $2, $3)
ON CONFLICT (casella_id, cartella) DO UPDATE SET
    coperto_fino_a = GREATEST(COALESCE(sync_cursore.coperto_fino_a, EXCLUDED.coperto_fino_a), EXCLUDED.coperto_fino_a);

-- name: SetStoricoFinoA :exec
-- Quanto indietro è già arrivato "Carica precedenti" su quella (casella, cartella).
--
-- È un INSERT, non un UPDATE: la riga del cursore nasce con il primo sync ORDINARIO, e una casella
-- da cui si carica l'archivio prima di averla mai sincronizzata non ce l'ha ancora. Con l'UPDATE non
-- veniva scritto niente e non lo diceva nessuno: il clic dopo ricalcolava la stessa identica
-- finestra, all'infinito. `ultimo_received` resta NULL - la riga dice fin dove si è scesi, non fin
-- dove si è saliti - e il sync ordinario la riempirà quando toccherà a lui.
INSERT INTO sync_cursore (casella_id, cartella, storico_fino_a)
VALUES ($1, $2, $3)
ON CONFLICT (casella_id, cartella) DO UPDATE SET
    storico_fino_a = LEAST(COALESCE(sync_cursore.storico_fino_a, EXCLUDED.storico_fino_a), EXCLUDED.storico_fino_a);

-- name: SetBuyerMessaggio :exec
UPDATE messaggio SET buyer_id = $2 WHERE messaggio_id = $1;

-- name: AgganciaOrfaniConversazione :many
-- quando l'operatore crea/aggancia una RFQ, gli altri messaggi orfani della stessa conversazione la seguono
UPDATE messaggio SET thread_id = $2, aggancio = 'auto_conversazione', agganciato_il = now()
WHERE conversazione_id = $1 AND thread_id IS NULL AND messaggio_id <> $3
RETURNING messaggio_id;
