-- P4 (giro 5, A1d, blocco B1): le query della migrazione 0022. La bozza della distinta salvata sul server
-- (testata, versioni con lo snapshot formato 2 completo, giornale degli eventi), la lettura delle conferme
-- strutturali con i loro componenti e archi, l'esito degli intenti, i contatori della working.
--
-- Chi le chiama: core/rfq/distintabozza (la bozza); le letture delle conferme, l'esito degli intenti e i contatori
-- li useranno B2 (rotte R1-R7, conferma formato 2) e B3 (verifica): in B1 le chiamano solo le prove L4.
--
-- Regole che valgono per tutte (migrazione 0022, commenti in testa):
--   - le date della storia (creata_il, salvata_il, quando, creato_il, chiusa_il) le scrive il database: qui non
--     si passano;
--   - le colonne tx (xid8) non si leggono: servono ai trigger, non al Go;
--   - il perimetro sta nel WHERE: una bozza (testata, versioni, eventi), una conferma o un intento si leggono
--     sempre con la RFQ del percorso, e fuori da quella non si trovano (contratto v2 §1.3: FUORI_RFQ, mai
--     l'esistenza in un'altra RFQ). Versioni ed eventi portano la RFQ con una JOIN sulla testata (RC-19).

-- ------------------------------------------------------------------ la bozza della distinta

-- name: InsertDistintaBozza :one
-- La testata di una bozza nuova: nasce aperta alla versione 0 (trigger distinta_bozza_ciclo) e, nella stessa
-- transazione, riceve la versione 1 con InsertDistintaBozzaVersione (FK differita al COMMIT). Base e fonte qui sono
-- quelle della versione 1: il trigger della versione le riscrive comunque con le sue. V1 (0023): il formato si passa
-- (2 o 3, ck_distinta_bozza_formato) e non cambia piu' (trigger del ciclo); lo snapshot di ogni versione ha il formato
-- della testata (trigger della versione).
INSERT INTO distinta_bozza (thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore, firma_base,
                            fonte_allegato_id, fonte_sha256, fonte_gruppo, creata_da, aggiornata_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(prodotto_id), sqlc.arg(formato), sqlc.narg(base_conferma_id), sqlc.arg(base_numero),
        sqlc.arg(base_contatore), sqlc.arg(firma_base), sqlc.narg(fonte_allegato_id), sqlc.narg(fonte_sha256),
        sqlc.narg(fonte_gruppo), sqlc.arg(utente_id), sqlc.arg(utente_id))
RETURNING bozza_id, creata_il;

-- name: InsertDistintaBozzaVersione :one
-- Una versione salvata, con lo snapshot intero. Porta la testata alla sua versione (trigger
-- distinta_bozza_versione_avanza): solo se la bozza e' aperta alla versione precedente, altrimenti P4X04. V1 (0023):
-- nel formato 3 il registro della sorgente (importazione_id, che il trigger confronta con lo snapshot) e la chiave del
-- salvataggio con l'impronta della richiesta (obbligatorie: ck_dbv_chiave_salvataggio); nel formato 2 NULL.
INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero,
                                     base_contatore, firma_base, fonte_allegato_id, fonte_sha256, fonte_gruppo,
                                     salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
VALUES (sqlc.arg(bozza_id), sqlc.arg(versione), sqlc.arg(snapshot), sqlc.arg(impronta), sqlc.narg(base_conferma_id),
        sqlc.arg(base_numero), sqlc.arg(base_contatore), sqlc.arg(firma_base), sqlc.narg(fonte_allegato_id),
        sqlc.narg(fonte_sha256), sqlc.narg(fonte_gruppo), sqlc.arg(salvata_da), sqlc.narg(importazione_id),
        sqlc.narg(chiave_salvataggio), sqlc.narg(impronta_richiesta))
RETURNING salvata_il;

-- name: InsertDistintaBozzaEvento :exec
-- Un evento del giornale: solo alla versione corrente, nella transazione che l'ha salvata (trigger
-- distinta_bozza_evento_nasce). n continua la numerazione della bozza (UltimoEventoDistintaBozza + 1).
INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, chiave_bozza, prima, dopo, origine,
                                   motivo, utente_id)
VALUES (sqlc.arg(bozza_id), sqlc.arg(n), sqlc.arg(versione), sqlc.arg(vocabolario), sqlc.arg(azione),
        sqlc.narg(chiave_bozza), sqlc.narg(prima), sqlc.narg(dopo), sqlc.arg(origine), sqlc.narg(motivo),
        sqlc.arg(utente_id));

-- name: GetDistintaBozzaAperta :one
-- La bozza aperta di un prodotto della RFQ (al piu' una: ux_distinta_bozza_aperta).
SELECT * FROM distinta_bozza
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id) AND stato = 'aperta';

-- name: GetDistintaBozza :one
-- Una bozza della RFQ, in qualunque stato.
SELECT * FROM distinta_bozza WHERE bozza_id = sqlc.arg(bozza_id) AND thread_id = sqlc.arg(thread_id);

-- name: BloccaDistintaBozza :one
-- La testata bloccata per la durata della transazione, dopo BloccaThread (ordine dei lucchetti, piano P4-B0 F22):
-- il salvataggio e l'abbandono leggono qui la versione corrente.
SELECT * FROM distinta_bozza WHERE bozza_id = sqlc.arg(bozza_id) AND thread_id = sqlc.arg(thread_id) FOR UPDATE;

-- name: ListDistinteBozzeProdotto :many
-- Tutte le bozze di un prodotto della RFQ, dalla piu' vecchia (aperte, confermate e abbandonate non ancora
-- cancellate dalla conservazione).
SELECT * FROM distinta_bozza
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY creata_il, bozza_id;

-- name: GetDistintaBozzaVersione :one
-- Una versione salvata di una bozza della RFQ, con lo snapshot (e, dalla 0023, registro e chiave del salvataggio).
SELECT v.bozza_id, v.versione, v.snapshot, v.impronta, v.base_conferma_id, v.base_numero, v.base_contatore, v.firma_base,
       v.fonte_allegato_id, v.fonte_sha256, v.fonte_gruppo, v.salvata_da, v.salvata_il, v.importazione_id, v.chiave_salvataggio,
       v.impronta_richiesta
  FROM distinta_bozza_versione v
  JOIN distinta_bozza b ON b.bozza_id = v.bozza_id
 WHERE v.bozza_id = sqlc.arg(bozza_id) AND b.thread_id = sqlc.arg(thread_id) AND v.versione = sqlc.arg(versione);

-- name: ListDistintaBozzaVersioni :many
-- Tutte le versioni di una bozza della RFQ, dalla prima: la storia degli snapshot, per rifare il giornale. Solo per
-- la diagnosi e le prove (LeggiStoria): una rotta che la esponga vuole un tetto o le pagine (RC-17).
SELECT v.bozza_id, v.versione, v.snapshot, v.impronta, v.base_conferma_id, v.base_numero, v.base_contatore, v.firma_base,
       v.fonte_allegato_id, v.fonte_sha256, v.fonte_gruppo, v.salvata_da, v.salvata_il, v.importazione_id, v.chiave_salvataggio,
       v.impronta_richiesta
  FROM distinta_bozza_versione v
  JOIN distinta_bozza b ON b.bozza_id = v.bozza_id
 WHERE v.bozza_id = sqlc.arg(bozza_id) AND b.thread_id = sqlc.arg(thread_id)
 ORDER BY v.versione;

-- name: ListDistintaBozzaEventi :many
-- Il giornale di una bozza della RFQ, in ordine e a pagine: gli eventi fino alla versione indicata (una lettura
-- coerente con la versione letta prima: versioni ed eventi non si riscrivono), dopo il numero indicato, al piu'
-- limite righe.
SELECT e.* FROM distinta_bozza_evento e
  JOIN distinta_bozza b ON b.bozza_id = e.bozza_id
 WHERE e.bozza_id = sqlc.arg(bozza_id) AND b.thread_id = sqlc.arg(thread_id)
   AND e.versione <= sqlc.arg(versione_max) AND e.n > sqlc.arg(dopo_n)
 ORDER BY e.n
 LIMIT sqlc.arg(limite);

-- name: ContaDistintaBozzaEventi :one
-- Quanti eventi ha il giornale di una bozza della RFQ fino alla versione indicata (R1: «eventi» e' un numero).
SELECT count(*)::int AS n FROM distinta_bozza_evento e
  JOIN distinta_bozza b ON b.bozza_id = e.bozza_id
 WHERE e.bozza_id = sqlc.arg(bozza_id) AND b.thread_id = sqlc.arg(thread_id) AND e.versione <= sqlc.arg(versione_max);

-- name: GetDistintaBozzaEvento :one
-- Un evento del giornale di una bozza della RFQ, per numero: annulla e rifai nominano l'evento che annullano o
-- rifanno (contratto rev. 3 §6.6, dopo.n), anche di una versione precedente.
SELECT e.* FROM distinta_bozza_evento e
  JOIN distinta_bozza b ON b.bozza_id = e.bozza_id
 WHERE e.bozza_id = sqlc.arg(bozza_id) AND b.thread_id = sqlc.arg(thread_id) AND e.n = sqlc.arg(n);

-- name: UltimoEventoDistintaBozza :one
-- Il numero dell'ultimo evento di una bozza della RFQ (0 se non ce ne sono): il prossimo e' questo + 1.
SELECT COALESCE(max(e.n), 0)::int AS n FROM distinta_bozza_evento e
  JOIN distinta_bozza b ON b.bozza_id = e.bozza_id
 WHERE e.bozza_id = sqlc.arg(bozza_id) AND b.thread_id = sqlc.arg(thread_id);

-- name: AbbandonaDistintaBozza :execrows
-- La chiusura «abbandonata» di una bozza aperta alla versione attesa (confronta e scambia: 0 righe se nel
-- frattempo e' cambiata, chiusa o non e' della RFQ). chiusa_il la riscrive comunque il trigger con now().
UPDATE distinta_bozza
   SET stato = 'abbandonata', chiusa_da = sqlc.arg(chiusa_da), chiusa_il = now(), motivo_chiusura = sqlc.narg(motivo)
 WHERE bozza_id = sqlc.arg(bozza_id) AND thread_id = sqlc.arg(thread_id) AND stato = 'aperta'
   AND versione = sqlc.arg(versione);

-- name: ConfermaDistintaBozza :execrows
-- V1 (0023): la chiusura «confermata» della bozza alla versione che la conferma chiude, nella transazione di R5 e dopo
-- la testata della conferma (il ciclo vuole la conferma che la cita, scritta prima; il controllo al COMMIT vuole la
-- bozza chiusa). Confronta e scambia: 0 righe se nel frattempo e' cambiata o chiusa.
UPDATE distinta_bozza
   SET stato = 'confermata', chiusa_da = sqlc.arg(chiusa_da), chiusa_il = now()
 WHERE bozza_id = sqlc.arg(bozza_id) AND thread_id = sqlc.arg(thread_id) AND stato = 'aperta'
   AND versione = sqlc.arg(versione);

-- name: GetProdottoDellaRfq :one
-- Il componente che una bozza vuole come prodotto: della RFQ, con tipo e archiviazione (chi chiama vuole un finito
-- non archiviato; contratto v2 §1.3), e con codice, rev e descrizione (la radice della partenza «vuota»).
SELECT componente_id, codice, rev, descrizione, tipo, archiviato_il FROM componente
 WHERE thread_id = sqlc.arg(thread_id) AND componente_id = sqlc.arg(componente_id);

-- name: ListPerimetroRegistrato :many
-- Il perimetro registrato di un prodotto della RFQ: il prodotto e tutti i componenti che la working
-- (componente_relazione) gli mette sotto, a ogni livello. E' lo stato di partenza «registrato» della prima versione
-- di una bozza (contratto rev. 3 §6.6): il server lo ricostruisce da solo e lo confronta con quello che dicono le
-- azioni. UNION (non UNION ALL): un ciclo nella working non fa girare la query all'infinito. I figli archiviati non
-- ci sono (come v_componente_albero e il perimetro chiuso della conferma: la conferma non li certifica), neanche se
-- un arco rimasto li tocca (revisioni della notte, R-11 / RD-12).
WITH RECURSIVE perimetro(componente_id) AS (
    SELECT c.componente_id FROM componente c
     WHERE c.thread_id = sqlc.arg(thread_id) AND c.componente_id = sqlc.arg(prodotto_id)
  UNION
    SELECT r.figlio_id FROM componente_relazione r
      JOIN perimetro p ON p.componente_id = r.padre_id
      JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
     WHERE r.thread_id = sqlc.arg(thread_id)
)
SELECT c.componente_id, c.codice, c.rev, c.descrizione, c.tipo, c.archiviato_il
  FROM componente c
  JOIN perimetro p ON p.componente_id = c.componente_id
 WHERE c.thread_id = sqlc.arg(thread_id)
 ORDER BY c.componente_id;

-- name: ListArchiPerimetroRegistrato :many
-- Gli archi della working dentro il perimetro registrato di un prodotto della RFQ (padre nel perimetro), con qta e
-- posizione: gli archi dello stato di partenza «registrato». Senza gli archi verso un figlio archiviato.
WITH RECURSIVE perimetro(componente_id) AS (
    SELECT c.componente_id FROM componente c
     WHERE c.thread_id = sqlc.arg(thread_id) AND c.componente_id = sqlc.arg(prodotto_id)
  UNION
    SELECT r.figlio_id FROM componente_relazione r
      JOIN perimetro p ON p.componente_id = r.padre_id
      JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
     WHERE r.thread_id = sqlc.arg(thread_id)
)
SELECT r.padre_id, r.figlio_id, r.qta, r.posizione
  FROM componente_relazione r
  JOIN perimetro p ON p.componente_id = r.padre_id
  JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
 WHERE r.thread_id = sqlc.arg(thread_id)
 ORDER BY r.padre_id, r.figlio_id;

-- name: GetAllegatoDellaRfq :one
-- Un allegato di un messaggio della RFQ, con il suo sha256 (NULL finche' non e' nello staging): la fonte di una
-- bozza deve essere un file della RFQ con lo sha256 dichiarato.
SELECT a.allegato_id, a.nome_file, a.sha256 FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE a.allegato_id = sqlc.arg(allegato_id) AND m.thread_id = sqlc.arg(thread_id)::uuid;

-- ------------------------------------------------------------------ le conferme strutturali (sola lettura in B1)

-- name: ListConfermeStrutturali :many
-- Lo storico delle conferme di un prodotto della RFQ, dalla prima (R7): le testate senza i JSON grandi (snapshot,
-- delta, mappa, decisioni), che si leggono una per una con GetConfermaStrutturale.
SELECT conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
       firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id,
       bozza_versione, fonte_allegato_id, fonte_sha256, fonte_gruppo, origine, impronta_snapshot, mono_pezzo,
       mono_pezzo_motivo, criterio_verifica, verifica, chiave_idempotenza, confermato_da, confermato_il, importazione_id,
       importazioni_citate
  FROM conferma_strutturale
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY numero;

-- name: GetUltimaConfermaStrutturale :one
-- L'ultima conferma di un prodotto della RFQ: quella col numero piu' alto (contratto v2 §0 n. 8). Se e' ancora
-- valida lo decide chi chiama, con il contatore o la firma (§4.2).
SELECT conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
       firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id,
       bozza_versione, fonte_allegato_id, fonte_sha256, fonte_gruppo, origine, impronta_snapshot, mono_pezzo,
       mono_pezzo_motivo, criterio_verifica, verifica, chiave_idempotenza, confermato_da, confermato_il, importazione_id,
       importazioni_citate
  FROM conferma_strutturale
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY numero DESC
 LIMIT 1;

-- name: GetConfermaStrutturaleBase :one
-- Una conferma della RFQ, solo prodotto e numero: il controllo della base di una bozza (RC-17: senza leggere
-- snapshot, delta, mappa e decisioni).
SELECT conferma_id, prodotto_id, numero FROM conferma_strutturale
 WHERE thread_id = sqlc.arg(thread_id) AND conferma_id = sqlc.arg(conferma_id);

-- name: GetConfermaStrutturale :one
-- Una conferma della RFQ, intera (snapshot, delta, mappa, decisioni compresi).
SELECT conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
       firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id,
       bozza_versione, fonte_allegato_id, fonte_sha256, fonte_gruppo, origine, snapshot, impronta_snapshot, delta,
       mappa, decisioni, mono_pezzo, mono_pezzo_motivo, criterio_verifica, verifica, chiave_idempotenza,
       confermato_da, confermato_il, importazione_id, importazioni_citate
  FROM conferma_strutturale
 WHERE thread_id = sqlc.arg(thread_id) AND conferma_id = sqlc.arg(conferma_id);

-- name: ListConfermaStrutturaleComponenti :many
-- I componenti certificati da una conferma della RFQ: prima la radice, poi per codice.
SELECT k.* FROM conferma_strutturale_componente k
 WHERE k.conferma_id = sqlc.arg(conferma_id) AND k.thread_id = sqlc.arg(thread_id)
 ORDER BY k.radice DESC, k.codice, k.componente_id;

-- name: ListConfermaStrutturaleArchi :many
-- Gli archi (con la qta) certificati da una conferma della RFQ.
SELECT a.* FROM conferma_strutturale_arco a
  JOIN conferma_strutturale c ON c.conferma_id = a.conferma_id
 WHERE a.conferma_id = sqlc.arg(conferma_id) AND c.thread_id = sqlc.arg(thread_id)
 ORDER BY a.padre_id, a.figlio_id;

-- ------------------------------------------------------------------ l'esito degli intenti

-- name: InsertEsitoIntento :one
-- L'esito di un gesto idempotente, nella sua transazione prima del COMMIT (contratto v2 §0 n. 7). Una seconda riga
-- con la stessa chiave la ferma la PK: chi chiama ha gia' cercato l'intento subito dopo il lucchetto della RFQ.
INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
VALUES (sqlc.arg(chiave), sqlc.arg(thread_id), sqlc.arg(gesto), sqlc.arg(utente_id), sqlc.arg(impronta_corpo),
        sqlc.arg(esito), sqlc.narg(conferma_id))
RETURNING creato_il;

-- name: GetEsitoIntento :one
-- L'esito registrato per una chiave d'intento della RFQ (R6). Nessuna riga = non registrato (o oltre la
-- conservazione di 90 giorni: allora decide la conferma con la stessa chiave, contratto v2 rev. 3).
SELECT * FROM esito_intento WHERE chiave = sqlc.arg(chiave) AND thread_id = sqlc.arg(thread_id);

-- ------------------------------------------------------------------ i contatori della working

-- name: GetWorkingContatore :one
-- I due contatori della working della RFQ; una riga mancante vale (0, 0) (migrazione 0022, passo 2). Senza
-- lucchetto: chi registra una conferma li rilegge dopo le scritture, e il trigger della conferma li confronta con i
-- correnti sotto il suo FOR UPDATE.
SELECT COALESCE((SELECT w.struttura FROM working_contatore w WHERE w.thread_id = sqlc.arg(thread_id)), 0)::bigint AS struttura,
       COALESCE((SELECT w.dati FROM working_contatore w WHERE w.thread_id = sqlc.arg(thread_id)), 0)::bigint AS dati;
