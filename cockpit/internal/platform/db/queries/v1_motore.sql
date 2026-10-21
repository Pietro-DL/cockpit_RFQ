-- V1 (giro 5, A1d; direttiva 23): le letture del modello della pagina della Distinta (/motore-a/dati, emendamento di V1
-- §9.9; piano della 0023 §14 passo 3, corsia motore). Solo SELECT, nello snapshot REPEATABLE READ READ ONLY del modello,
-- insieme alla fotografia: le bozze aperte formato 3 con le colonne del loro registro (il gruppo corrente della
-- sorgente, mai il contenuto) e lo stato del rismistamento F8. L'identità certificata dei componenti la legge
-- ListIdentitaRegistrate (v1_formato3.sql); le occorrenze non attive la lettura unica del ciclo dei file,
-- ListOccorrenzeNonAttiveRfq (v1_file.sql), la stessa di F8 (decisioni dell'ondata B §5.1).
--
-- Regole (le stesse di p4_conferme.sql e v1_formato3.sql): il perimetro sta nel WHERE (la RFQ del percorso); le colonne
-- tx non si leggono; nessuna scrittura.

-- name: ListSorgentiBozzeAperte :many
-- Le bozze aperte formato 3 della RFQ, una per prodotto, con la versione corrente e, dal registro della versione, il
-- gruppo, la fonte, la radice e l'impronta del contenuto; mai il contenuto (EM §6.4.1, §9.9: bozza_del_prodotto). Senza
-- registro le colonne del registro sono NULL e vale la fonte della testata.
SELECT b.bozza_id, b.prodotto_id, b.versione, b.fonte_allegato_id, b.fonte_sha256, b.fonte_gruppo, v.importazione_id,
       i.gruppo AS registro_gruppo, i.fonte_allegato_id AS registro_fonte_allegato_id, i.fonte_sha256 AS registro_fonte_sha256,
       i.fonte_radice AS registro_fonte_radice, i.impronta_contenuto AS registro_impronta_contenuto
  FROM distinta_bozza b
  JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
  LEFT JOIN distinta_importazione i ON i.importazione_id = v.importazione_id
 WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta' AND b.formato = 3
 ORDER BY b.prodotto_id, b.bozza_id;

-- name: GetStatoF8DelThread :one
-- Lo stato del rismistamento F8 della RFQ (f8_stato, 0022): quante righe in errore, quante in attesa e quante in tutto.
-- Nessuna riga = F8 mai previsto.
SELECT count(*) FILTER (WHERE stato = 'errore')::int    AS in_errore,
       count(*) FILTER (WHERE stato = 'in_attesa')::int AS in_attesa,
       count(*)::int                                     AS totale
  FROM f8_stato
 WHERE thread_id = sqlc.arg(thread_id);

-- name: ListEventiFonteDelThread :many
-- Gli eventi della fonte STEP dei prodotti della RFQ che dicono conferma, sostituzione, autorizzazione e revoca, con il
-- nome dell'autore, in ordine (emendamento U2 §2.4: /dati.fonte e la sezione del prodotto di S9 con data e autore della
-- conferma e dell'autorizzazione). con_chiave: l'evento ha la chiave d'idempotenza di un intento (direttiva 39, R39-09:
-- la conferma_documento del passo 2 di S8 e la revoca della sorgente di S8 la hanno; gli eventi delle pagine legacy no),
-- per non leggere una revoca della sorgente della bozza come la revoca dell'autorizzazione della working (R39-10).
SELECT e.evento_id, e.prodotto_id, e.azione, e.allegato_id, e.sha256, e.documento_id, e.autore_id, u.nome AS autore_nome, e.quando,
       (e.chiave_idempotenza IS NOT NULL)::boolean AS con_chiave
  FROM fonte_step_evento e
  JOIN utente u ON u.utente_id = e.autore_id
 WHERE e.thread_id = sqlc.arg(thread_id) AND e.ruolo = 'prodotto'
   AND e.azione IN ('conferma_documento', 'sostituzione', 'autorizzazione', 'revoca')
 ORDER BY e.evento_id;

-- name: ListNomiUtenti :many
-- I nomi degli utenti dati (chi ha confermato un documento prima di V1, senza eventi della fonte: U2 §2.4).
SELECT utente_id, nome
  FROM utente
 WHERE utente_id = ANY (sqlc.arg(utenti)::uuid[])
 ORDER BY utente_id;

-- name: ListUltimeConfermeDelThread :many
-- L'ultima conferma strutturale di ogni prodotto della RFQ (numero piu' alto), con il formato, il registro della
-- conferma, chi e quando, la fonte, il criterio, il registro delle decisioni e la mappa (U2 §5.3 conferma_corrente;
-- §6.4.2 l'adattatore ConfermaCategoria dall'ultima conferma valida). La validita' la decide confermabom sullo stato
-- della RFQ letto nello stesso snapshot.
SELECT DISTINCT ON (c.prodotto_id) c.conferma_id, c.prodotto_id, c.numero, c.formato, c.importazione_id, c.confermato_il,
       c.confermato_da, u.nome AS confermato_da_nome, c.fonte_allegato_id, c.fonte_sha256, c.fonte_gruppo, c.criterio_verifica,
       c.decisioni, c.mappa
  FROM conferma_strutturale c
  JOIN utente u ON u.utente_id = c.confermato_da
 WHERE c.thread_id = sqlc.arg(thread_id)
 ORDER BY c.prodotto_id, c.numero DESC;
