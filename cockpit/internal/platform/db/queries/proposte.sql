-- Le proposte di struttura (Blocco 8, B8.5): dai fatti di uno STEP ai nodi, agli archi e alle
-- rimozioni proposti per una RFQ (addendum A1.1, A1.2, A4.4).
--
-- Tre strati, e qui se ne scrive uno solo: i FATTI stanno in analisi_fatti e non si toccano, le
-- DECISIONI stanno in componente e componente_relazione e le prende una persona. Queste query scrivono
-- l'interpretazione. Una proposta gia' decisa non si riscrive: gli upsert cambiano solo le righe aperte.

-- name: ListComponenteProposteFile :many
SELECT * FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 ORDER BY chiave;

-- name: ListRelazioneProposteFile :many
SELECT * FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2 ORDER BY padre_chiave, figlio_chiave;

-- name: ListComponenteProposteStessoFile :many
-- I nodi proposti da un contenuto in una RFQ, qualunque allegato li porti: servono a riconoscere nello
-- STEP strutturale i nodi gia' decisi.
SELECT * FROM componente_proposta WHERE thread_id = $1 AND sha256 = $2 ORDER BY chiave, creato_il;

-- name: PortatoreDelFile :one
-- L'allegato che in questa RFQ porta le proposte di un contenuto. Lo stesso file arrivato due volte
-- nella stessa RFQ propone una volta sola: due serie di nodi uguali sarebbero doppio lavoro e doppio
-- conteggio nel gate.
SELECT allegato_id FROM componente_proposta WHERE thread_id = $1 AND sha256 = $2
ORDER BY creato_il, allegato_id LIMIT 1;

-- name: UpsertComponenteProposta :exec
-- Un nodo proposto. Si riscrive solo finche' e' aperto; un codice scritto a mano dall'operatore
-- (origine_codice = 'operatore') resta, anche se le regole del cliente cambiano: la riclassificazione
-- vale per cio' che ha classificato il server.
INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, descrizione, codice, rev,
                                 origine_codice, famiglia, tipo_proposto, fonte, confidenza, evidenza, stato, componente_id, nota, deciso_il)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(sha256), sqlc.arg(chiave), sqlc.arg(nome_grezzo), sqlc.arg(id_grezzo),
        sqlc.narg(descrizione), sqlc.narg(codice), sqlc.narg(rev), sqlc.narg(origine_codice), sqlc.arg(famiglia),
        sqlc.narg(tipo_proposto), 'step', sqlc.arg(confidenza), sqlc.arg(evidenza), sqlc.arg(stato), sqlc.narg(componente_id),
        sqlc.narg(nota), CASE WHEN sqlc.arg(stato)::stato_proposta = 'aperta' THEN NULL ELSE now() END)
ON CONFLICT (thread_id, allegato_id, chiave) DO UPDATE SET
    sha256         = EXCLUDED.sha256,
    nome_grezzo    = EXCLUDED.nome_grezzo,
    id_grezzo      = EXCLUDED.id_grezzo,
    descrizione    = EXCLUDED.descrizione,
    codice         = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.codice ELSE EXCLUDED.codice END,
    rev            = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.rev ELSE EXCLUDED.rev END,
    origine_codice = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.origine_codice ELSE EXCLUDED.origine_codice END,
    famiglia       = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.famiglia ELSE EXCLUDED.famiglia END,
    confidenza     = CASE WHEN componente_proposta.origine_codice = 'operatore' THEN componente_proposta.confidenza ELSE EXCLUDED.confidenza END,
    tipo_proposto  = EXCLUDED.tipo_proposto,
    evidenza       = EXCLUDED.evidenza,
    stato          = EXCLUDED.stato,
    componente_id  = EXCLUDED.componente_id,
    nota           = EXCLUDED.nota,
    deciso_il      = EXCLUDED.deciso_il
WHERE componente_proposta.stato = 'aperta';

-- name: UpsertRelazioneProposta :exec
-- Un arco proposto, una riga per coppia (padre, figlio) del file. Solo finche' e' aperto.
INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta, evidenza, stato, nota, deciso_il)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(padre_chiave), sqlc.arg(figlio_chiave), sqlc.arg(qta),
        sqlc.arg(evidenza), sqlc.arg(stato), sqlc.narg(nota), CASE WHEN sqlc.arg(stato)::stato_proposta = 'aperta' THEN NULL ELSE now() END)
ON CONFLICT (thread_id, allegato_id, padre_chiave, figlio_chiave) DO UPDATE SET
    qta       = EXCLUDED.qta,
    evidenza  = EXCLUDED.evidenza,
    stato     = EXCLUDED.stato,
    nota      = EXCLUDED.nota,
    deciso_il = EXCLUDED.deciso_il
WHERE relazione_proposta.stato = 'aperta';

-- name: BloccaComponenteProposta :one
SELECT * FROM componente_proposta WHERE proposta_id = $1 FOR UPDATE;

-- name: DecidiComponenteProposta :execrows
UPDATE componente_proposta SET stato = sqlc.arg(stato), componente_id = sqlc.narg(componente_id),
       deciso_da = sqlc.narg(deciso_da), deciso_il = now()
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta';

-- name: SetCodiceComponenteProposta :execrows
-- Il codice lo scrive l'operatore, su un nodo che il server non ha saputo classificare (A1.2): origine
-- 'operatore', e da qui una riclassificazione non lo tocca piu'.
UPDATE componente_proposta SET codice = sqlc.arg(codice), rev = sqlc.narg(rev), origine_codice = 'operatore',
       famiglia = '', confidenza = 100
WHERE proposta_id = sqlc.arg(proposta_id) AND stato = 'aperta';

-- name: RiconciliaProposteNodo :execrows
-- Un componente appena creato o agganciato riconcilia le altre proposte aperte dello stesso codice
-- nella RFQ: non creano un'identita' nuova, la ritrovano (A2.3). Senza chi le ha decise: e' la stessa
-- lettura che il server avrebbe dato alla prossima rianalisi.
UPDATE componente_proposta SET stato = 'duplicato', componente_id = sqlc.arg(componente_id), deciso_il = now()
WHERE thread_id = sqlc.arg(thread_id) AND upper(codice) = upper(sqlc.arg(codice)::text) AND stato = 'aperta'
  AND proposta_id <> sqlc.arg(esclusa);

-- name: BloccaRelazioneProposta :one
SELECT * FROM relazione_proposta
WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = $3 AND figlio_chiave = $4 FOR UPDATE;

-- name: DecidiRelazioneProposta :execrows
UPDATE relazione_proposta SET stato = sqlc.arg(stato), nota = COALESCE(sqlc.narg(nota), nota),
       deciso_da = sqlc.narg(deciso_da), deciso_il = now()
WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id)
  AND padre_chiave = sqlc.arg(padre_chiave) AND figlio_chiave = sqlc.arg(figlio_chiave) AND stato = 'aperta';

-- name: RiconciliaProposteRelazione :execrows
-- Un arco appena scritto nella working riconcilia le altre proposte aperte che lo dicono uguale, da
-- qualunque file della RFQ: stessi due componenti, stessa quantita'.
UPDATE relazione_proposta r SET stato = 'duplicato', deciso_il = now()
FROM componente_proposta p, componente_proposta f
WHERE r.thread_id = sqlc.arg(thread_id) AND r.stato = 'aperta' AND r.qta = sqlc.arg(qta)
  AND p.thread_id = r.thread_id AND p.allegato_id = r.allegato_id AND p.chiave = r.padre_chiave
  AND f.thread_id = r.thread_id AND f.allegato_id = r.allegato_id AND f.chiave = r.figlio_chiave
  AND p.componente_id = sqlc.arg(padre_id) AND f.componente_id = sqlc.arg(figlio_id);

-- name: ListAllegatiStessoFileConRfq :many
-- Ogni allegato con questo contenuto che sta in una RFQ: i fatti di uno STEP diventano proposte in
-- ognuna (A1.2), ciascuna con le regole del suo cliente.
SELECT sqlc.embed(a), m.thread_id AS rfq_id
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE a.sha256 = $1 AND m.thread_id IS NOT NULL
 ORDER BY m.thread_id, a.ricevuto_il, a.allegato_id;

-- name: ListStepDellaRfq :many
-- Gli STEP arrivati in una RFQ e scaricati (lo SHA si sa): quelli da cui possono nascere proposte.
SELECT a.* FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = $1 AND lower(a.estensione) IN ('stp', 'step') AND a.sha256 IS NOT NULL
 ORDER BY a.ricevuto_il, a.allegato_id;

-- name: GetAnalisiCorrente :one
-- I fatti di un contenuto con la chiave dell'analizzatore corrente (A4.5). Nessuna riga se
-- analizzatore_corrente e' vuota: nessuna analisi e' corrente, ed e' il lato che non concede.
SELECT af.* FROM analisi_fatti af
  JOIN analizzatore_corrente ac ON ac.versione_analizzatore = af.versione_analizzatore
                               AND ac.hash_configurazione = af.hash_configurazione
 WHERE af.sha256 = $1;

-- name: MotivoParziale :one
-- Perche' una struttura non e' completa, con la definizione della 0020 (struttura_motivo_parziale):
-- '' = completa. Il Go la chiede al database invece di ricalcolarla (A4.4: la definizione e' una).
SELECT COALESCE(struttura_motivo_parziale(sqlc.narg(struttura)::jsonb), '')::text AS motivo;

-- name: ProdottoDelloStepStrutturale :one
-- Il prodotto finito attivo che ha come STEP strutturale un documento corrente con questo contenuto.
SELECT c.* FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id
 WHERE c.thread_id = $1 AND d.sha256 = $2 AND d.sostituito_da IS NULL AND c.archiviato_il IS NULL
 ORDER BY c.codice LIMIT 1;

-- name: ListProdottiConStepStrutturale :many
SELECT c.* FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id
 WHERE c.thread_id = $1 AND d.sostituito_da IS NULL AND c.archiviato_il IS NULL
 ORDER BY c.codice;

-- name: ListRimozioniDiUnoStep :many
SELECT * FROM rimozione_proposta WHERE thread_id = $1 AND step_documento_id = $2 ORDER BY padre_id, figlio_id;

-- name: ChiudiRimozione :execrows
-- Una rimozione aperta che non vale piu' (l'arco non e' piu' nella working, o lo STEP strutturale lo
-- contiene di nuovo): scartata senza chi l'ha decisa, con la nota. E' interpretazione, non decisione.
UPDATE rimozione_proposta SET stato = 'scartata', deciso_da = NULL, deciso_il = now(), nota = sqlc.arg(nota)
WHERE thread_id = sqlc.arg(thread_id) AND step_documento_id = sqlc.arg(step_documento_id)
  AND padre_id = sqlc.arg(padre_id) AND figlio_id = sqlc.arg(figlio_id) AND stato = 'aperta';

-- name: BloccaRimozioneProposta :one
SELECT * FROM rimozione_proposta
WHERE thread_id = $1 AND step_documento_id = $2 AND padre_id = $3 AND figlio_id = $4 FOR UPDATE;

-- name: PropostaDocumentoDaRadice :execrows
-- D16: la radice dello STEP riconosciuta da una famiglia del cliente corregge la proposta del
-- documento, finche' e' aperta. regola_id resta com'e': le famiglie stanno in cliente.regole, non in
-- `regola`, e la famiglia che l'ha riconosciuta va nei dettagli.
UPDATE documento_proposta SET codice = sqlc.arg(codice), rev = sqlc.narg(rev), fonte = 'regola_cliente',
       confidenza = sqlc.arg(confidenza), dettagli = dettagli || sqlc.arg(dettagli)::jsonb
WHERE allegato_id = sqlc.arg(allegato_id) AND stato = 'aperta';

-- name: GetPropostaDocumentoDiAllegato :one
SELECT * FROM documento_proposta WHERE allegato_id = $1;

-- name: GetComponentePropostaPerChiave :one
SELECT * FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = $3;
