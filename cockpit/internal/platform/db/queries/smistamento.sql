-- Il flusso ancorato al prodotto (Smistamento F8, addendum A5.13). Il flusso legge lo stato della RFQ e scrive
-- una cosa sola: la destinazione proposta di ogni file, dentro documento_proposta.dettagli, per le righe
-- esistenti e aperte (FP3). Nessuna INSERT, nessuna tabella nuova, nessuna migrazione: le letture qui sotto
-- non esistevano ancora nella forma che il flusso vuole, e alzare la priorita' di un'analisi in coda e' un
-- UPDATE del job.

-- name: ScriviDestinazione :execrows
-- La destinazione calcolata dal flusso (A5.13.6), solo su una proposta aperta e solo se e' cambiata. La
-- guardia della decisione sta nel WHERE (P34): in READ COMMITTED una decisione concorrente che ha bloccato
-- la riga fa rivalutare il WHERE sulla versione nuova, e l'UPDATE non tocca la riga decisa. La firma
-- impedisce le riscritture uguali: ripetere l'evento non riscrive niente (prova 186).
UPDATE documento_proposta
   SET dettagli = jsonb_set(COALESCE(dettagli, '{}'::jsonb), '{destinazione}', sqlc.arg(destinazione)::jsonb)
 WHERE proposta_id = sqlc.arg(proposta_id)
   AND stato = 'aperta'
   AND (dettagli -> 'destinazione' ->> 'firma') IS DISTINCT FROM sqlc.arg(firma)::text;

-- name: ListFileDelFlusso :many
-- I file della RFQ come il flusso li vede: il nome e la cartella dentro l'archivio (le evidenze del nome),
-- il nome dell'archivio che li contiene, se sono a loro volta un archivio con delle voci, e se vengono da
-- una fonte del cliente (la stessa condizione di ListStepDellaRfq, P27: non da un fornitore, non da una
-- nostra mail in uscita fuori dal canale `nota`). Gli inline e i collegamenti non sono file da smistare.
SELECT a.allegato_id, a.contenitore_id, a.nome_file, a.path_interno, a.estensione, a.sha256, a.ricevuto_il,
       c.nome_file AS nome_contenitore,
       (m.controparte_tipo <> 'fornitore' AND (m.direzione = 'entrata' OR m.canale = 'nota'))::bool AS del_cliente,
       (EXISTS (SELECT 1 FROM allegato v WHERE v.contenitore_id = a.allegato_id))::bool AS contenitore
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN allegato c ON c.allegato_id = a.contenitore_id
 WHERE m.thread_id = sqlc.arg(thread_id) AND a.natura = 'file'
 ORDER BY a.ricevuto_il, a.allegato_id;

-- name: ListShaConFattiCorrenti :many
-- I contenuti della RFQ che hanno i fatti dell'analizzatore corrente. Solo lo sha: il flusso non legge mai
-- il JSON del worker (consuma le evidenze che il core ne ha tratto: la valutazione, le righe della struttura),
-- e qui vuole sapere soltanto se l'analisi c'e'.
SELECT DISTINCT a.sha256::text AS sha256
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  JOIN analisi_fatti af ON af.sha256 = a.sha256
  JOIN analizzatore_corrente ac ON ac.versione_analizzatore = af.versione_analizzatore
                               AND ac.hash_configurazione = af.hash_configurazione
 WHERE m.thread_id = sqlc.arg(thread_id) AND a.sha256 IS NOT NULL
 ORDER BY 1;

-- name: ListUltimiJobPerChiavi :many
-- L'ultimo job di ogni chiave, in qualunque stato: per le analisi dei file della RFQ distingue «in attesa»
-- (pronto, in corso) da «non si e' potuto leggere» (fallito) (A5.13.3, passo 2b).
SELECT DISTINCT ON (chiave_idempotenza) chiave_idempotenza::text AS chiave, stato, priorita
  FROM job
 WHERE chiave_idempotenza = ANY(sqlc.arg(chiavi)::text[])
 ORDER BY chiave_idempotenza, job_id DESC;

-- name: AlzaPrioritaJob :execrows
-- Prima gli STEP (e i PDF) compatibili per nome con un prodotto della RFQ (A5.13.6): la coda serve per
-- (priorita, job_id), e un numero piu' basso passa prima. LEAST: il flusso abbassa il numero e non lo alza
-- mai; un job gia' preso (in corso) non si tocca. Idempotente.
UPDATE job SET priorita = LEAST(priorita, sqlc.arg(priorita)::smallint)
 WHERE chiave_idempotenza = ANY(sqlc.arg(chiavi)::text[]) AND stato = 'pronto'
   AND priorita > sqlc.arg(priorita)::smallint;
