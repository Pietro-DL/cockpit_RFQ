-- IL COMANDO U5 (Smistamento F7, addendum A5.15): `cockpit -anteprima-riapri-agganci` e `-riapri-agganci
-- <nome-database>`. Gli agganci automatici sono ammessi nel database solo come proposte; quelli di prima dello
-- Smistamento, fatti per sola uguaglianza di codice (un nodo `duplicato` di un componente senza una persona che
-- l'abbia deciso, forma F-A, e gli archi chiusi in automatico perche' toccavano un nodo cosi', forma F-B), si
-- riaprono nelle RFQ in corso. Le BOM congelate non si toccano (U5, Domanda 6 = A): nessuna query qui scrive
-- componente, componente_relazione, documento, bom_versione o le istantanee.
--
-- Le letture valgono per l'anteprima (in una transazione in sola lettura: niente FOR UPDATE, niente
-- funzioni che bloccano righe) e per l'applicazione, che le rifa' dentro la transazione della RFQ, dopo
-- BloccaThread. La classificazione delle forme la fa il Go (ClassificaFormeLegacy, pura).

-- name: ListRfqPerLaRiapertura :many
-- Le RFQ, con quello che serve a dire il perimetro senza bloccare niente (A5.15.1): lo stato, se e' unita a
-- un'altra, l'ultima versione della BOM (numero e stato; senza versioni 0 e il testo vuoto) e quante versioni congelate
-- ha. In sola lettura la regola di bom_working_bloccata() si replica cosi', perche' la funzione prende la
-- riga del thread FOR KEY SHARE, che una transazione READ ONLY rifiuta. rfq NULL = tutte.
SELECT t.thread_id, t.stato, t.unito_in, coalesce(t.cartella_relativa, '')::text AS cartella,
       coalesce(t.oggetto, '')::text AS oggetto,
       coalesce((SELECT b.numero FROM bom_versione b WHERE b.thread_id = t.thread_id ORDER BY b.numero DESC LIMIT 1), 0)::int AS ultimo_numero,
       coalesce((SELECT b.stato::text FROM bom_versione b WHERE b.thread_id = t.thread_id ORDER BY b.numero DESC LIMIT 1), '')::text AS ultimo_stato,
       (SELECT count(*) FROM bom_versione b WHERE b.thread_id = t.thread_id AND b.stato = 'congelata')::int AS n_congelate
  FROM thread_offerta t
 WHERE sqlc.narg(rfq)::uuid IS NULL OR t.thread_id = sqlc.narg(rfq)::uuid
 ORDER BY t.data_inizio, t.thread_id;

-- name: WorkingBloccataNelDatabase :one
-- Il perimetro ricontrollato dentro la transazione dell'applicazione, dopo BloccaThread: la versione che
-- blocca la working, 0 se la working e' libera (bom_working_bloccata della 0020, la stessa regola del muro).
-- Una RFQ congelata fra l'anteprima e l'applicazione si salta da qui.
SELECT coalesce(bom_working_bloccata(sqlc.arg(thread_id)::uuid), 0)::int AS bloccata_in;

-- name: ListRimozioniDellaRfq :many
-- Tutte le rimozioni proposte della RFQ, in ogni stato: le chiusure automatiche (F-E) si contano e basta.
SELECT * FROM rimozione_proposta WHERE thread_id = $1 ORDER BY creato_il, padre_id, figlio_id;

-- name: ListRelazioniDellaRfq :many
-- Tutti gli archi della working della RFQ, anche fra componenti archiviati: la provenienza (F-K) si dice di
-- ogni arco che c'e'.
SELECT * FROM componente_relazione WHERE thread_id = $1 ORDER BY padre_id, figlio_id;

-- name: ListProposteDocumentoDellaRfq :many
-- Le proposte di documento della RFQ, con il minimo che serve al rapporto: la pre-assegnazione del vecchio
-- «Assegna» (F-G: aperta con un componente) e il codice riscritto dalla D16 (F-H: fonte regola_cliente con
-- la radice dello STEP nei dettagli). Non si toccano: si contano.
SELECT p.proposta_id, p.stato, p.componente_id, p.fonte, p.deciso_da, (p.dettagli ? 'radice_step')::bool AS radice_step,
       a.nome_file
  FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
 WHERE p.thread_id = $1
 ORDER BY a.ricevuto_il, p.proposta_id;

-- name: RiapriAgganciAutomaticiNodo :execrows
-- U5 (A5.15.3): un nodo agganciato per uguaglianza di codice, senza una persona, torna proposta. Niente si
-- cancella: lo stato di prima va in evidenza.storia, che gli upsert conservano (UpsertComponenteProposta).
-- Nell'UPDATE le espressioni a destra leggono i valori VECCHI della riga (semantica di PostgreSQL): la storia
-- registra com'era. Una riga marcata (evidenza.strutturale: radice, raggruppamento, delega, sospensione) o
-- decisa da una persona non si tocca mai, qualunque cosa dica chi chiama.
UPDATE componente_proposta SET stato = 'aperta', componente_id = NULL, deciso_il = NULL,
       nota = left('agganciata per codice a ' || sqlc.arg(codice_componente)::text || ' prima dello Smistamento, senza una persona: da confermare', 200),
       evidenza = jsonb_set(evidenza, '{storia}', coalesce(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
           'evento', 'riaperta_u5', 'stato', stato, 'componente_id', componente_id, 'codice_componente', sqlc.arg(codice_componente)::text,
           'deciso_il', deciso_il, 'nota', nota, 'il', now(), 'da', 'cockpit -riapri-agganci')))
WHERE proposta_id = sqlc.arg(proposta_id) AND thread_id = sqlc.arg(thread_id)
  AND stato = 'duplicato' AND deciso_da IS NULL AND componente_id IS NOT NULL AND NOT (evidenza ? 'strutturale');

-- name: RiapriAgganciAutomaticiArco :execrows
-- U5 (A5.15.3): un arco chiuso in automatico perche' toccava un nodo agganciato per codice (forma F-B: la
-- tenuta dei conti, `duplicato`, o lo «stesso componente», `scartata`, senza chi l'ha deciso) torna aperto,
-- con lo stato di prima nella storia. Un arco deciso da una persona non si tocca.
UPDATE relazione_proposta SET stato = 'aperta', deciso_il = NULL, nota = NULL,
       evidenza = jsonb_set(evidenza, '{storia}', coalesce(evidenza -> 'storia', '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
           'evento', 'riaperta_u5', 'stato', stato, 'nota', nota, 'deciso_il', deciso_il, 'il', now(), 'da', 'cockpit -riapri-agganci')))
WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id)
  AND padre_chiave = sqlc.arg(padre_chiave) AND figlio_chiave = sqlc.arg(figlio_chiave) AND deciso_da IS NULL
  AND (stato = 'duplicato' OR (stato = 'scartata' AND nota LIKE 'padre e figlio sono lo stesso componente%'));
