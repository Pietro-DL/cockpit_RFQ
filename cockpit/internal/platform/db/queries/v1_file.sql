-- Il ciclo dei file della Sorgente (V1, corsia file, ondata B; EM §10, PM23 §6-§7). Le tabelle sono della 0023
-- (file_rfq, file_rfq_evento, file_rfq_rimozione, file_rfq_conservazione, file_rfq_attivo); le query di base della
-- persistenza stanno in v1_formato3.sql (InsertFileRfq, UpdateFileRfqStato, InsertFileRfqEvento, InsertRimozione,
-- UpdateRimozione, InsertConservazione, UpdateConservazione, ListDipendenzeBozzaPerFile, BloccaContenuto). Qui le
-- letture e le scritture del servizio core/rfq/filerfq: un'occorrenza e' (RFQ, allegato) e la RFQ e' sempre
-- messaggio.thread_id, letta insieme all'allegato.

-- ------------------------------------------------------------------ le occorrenze

-- name: ListOccorrenzeSorgente :many
-- Le occorrenze del ciclo dei file per id, con lo stato (nessuna riga = attivo, versione 0), la proposta di documento
-- e lo scarto fatto col vecchio «Scarta il file»: proposta scartata da una persona, occorrenza attiva per il ciclo dei
-- file (nessuna riga, o la riga in attivo dopo una «Riattiva») e nessun evento «scarta» del ciclo per l'allegato, in
-- nessuna RFQ. Uno scarto della Sorgente in un'altra RFQ, prima di un cambio di RFQ del messaggio, ha il suo evento e
-- non e' un gesto precedente (PM23 CO-14); il vecchio «Scarta il file» usato dopo una «Riattiva» si' (rilievo della
-- revisione, PF-06). Le stesse condizioni stanno in ogni query di questo file che riconosce lo scarto col gesto
-- precedente. L'ordine e' quello degli id, per chi blocca e scrive in un ordine fisso.
SELECT a.allegato_id, a.messaggio_id, a.contenitore_id, a.indice, a.nome_file, a.path_interno, a.estensione, a.natura,
       a.origine, a.bytes, a.sha256, a.stato AS stato_allegato, a.path_staging, a.errore, a.ricevuto_il, a.caricato_da,
       m.thread_id, m.canale,
       COALESCE(f.stato, 'attivo')::text AS stato, COALESCE(f.versione, 0)::int AS versione,
       p.proposta_id, p.stato AS stato_proposta, p.componente_id AS proposta_componente_id, p.deciso_da AS proposta_decisa_da,
       ((f.stato IS NULL OR f.stato = 'attivo') AND p.stato = 'scartata' AND p.deciso_da IS NOT NULL
        AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ez WHERE ez.allegato_id = a.allegato_id AND ez.azione = 'scarta'))::bool AS scartato_legacy
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN file_rfq f ON f.thread_id = m.thread_id AND f.allegato_id = a.allegato_id
  LEFT JOIN documento_proposta p ON p.allegato_id = a.allegato_id
 WHERE a.allegato_id = ANY(sqlc.arg(allegati)::uuid[])
 ORDER BY a.allegato_id;

-- name: ListDiscendentiArchivio :many
-- Le voci di un archivio, ricorsivamente (EM §10.4, «Archivi e voci»): gli allegati con contenitore_id uguale
-- all'archivio, e le voci delle voci. In ordine di allegato_id: e' l'ordine di impronta_voci.
WITH RECURSIVE v AS (
    SELECT a.allegato_id FROM allegato a WHERE a.contenitore_id = sqlc.arg(archivio)::uuid
    UNION
    SELECT a.allegato_id FROM allegato a JOIN v ON a.contenitore_id = v.allegato_id)
SELECT v.allegato_id FROM v ORDER BY v.allegato_id;

-- name: ListFileSorgente :many
-- Il modello dei file della Sorgente (S1): ogni file (natura file o elemento Outlook) della RFQ con lo stato, la
-- proposta, il documento confermato di cui e' provenienza, la rimozione, l'ultima copia di conservazione e gli ultimi
-- eventi di scarto e di «Altri file». Nessun lucchetto: S1 legge in una transazione REPEATABLE READ READ ONLY.
SELECT a.allegato_id, a.messaggio_id, a.contenitore_id, a.indice, a.nome_file, a.path_interno, a.estensione, a.natura,
       a.origine, a.bytes, a.sha256, a.stato AS stato_allegato, a.path_staging, a.errore, a.ricevuto_il,
       m.canale, m.data_evento,
       COALESCE(f.stato, 'attivo')::text AS stato, COALESCE(f.versione, 0)::int AS versione,
       p.proposta_id, p.stato AS stato_proposta, p.tipo_proposto, p.codice AS proposta_codice, p.rev AS proposta_rev,
       p.componente_id AS proposta_componente_id, p.deciso_da AS proposta_decisa_da,
       ((f.stato IS NULL OR f.stato = 'attivo') AND p.stato = 'scartata' AND p.deciso_da IS NOT NULL
        AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ez WHERE ez.allegato_id = a.allegato_id AND ez.azione = 'scarta'))::bool AS scartato_legacy,
       doc.documento_id, doc.stato_nas AS documento_stato_nas, doc.codice AS documento_codice, doc.rev AS documento_rev,
       doc.tipo AS documento_tipo,
       r.rimozione_id, r.stato AS rimozione_stato, r.motivo AS rimozione_motivo, r.tentativi AS rimozione_tentativi,
       c.conservazione_id, c.stato AS conservazione_stato, c.causa AS conservazione_causa, c.errore AS conservazione_errore,
       c.verificato_il AS conservazione_verificato_il, c.bytes AS conservazione_bytes, c.percorso_relativo AS conservazione_percorso,
       c.radice_id AS conservazione_radice, c.esito_rimozione AS conservazione_esito,
       es.quando AS scarto_quando, es.motivo AS scarto_motivo, us.sigla AS scarto_autore,
       ea.quando AS altri_file_quando, ea.motivo AS altri_file_motivo, ua.sigla AS altri_file_autore,
       (SELECT count(*) FROM allegato v WHERE v.contenitore_id = a.allegato_id)::int AS n_voci_dirette,
       (EXISTS (SELECT 1 FROM documento d WHERE d.thread_id = m.thread_id AND d.sha256 = a.sha256 AND d.stato_nas = 'scritto'
                   AND NOT EXISTS (SELECT 1 FROM nas_anomalia n WHERE n.documento_id = d.documento_id AND n.risolta_il IS NULL)))::bool AS nas_scritto
  FROM allegato a
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN file_rfq f ON f.thread_id = m.thread_id AND f.allegato_id = a.allegato_id
  LEFT JOIN documento_proposta p ON p.allegato_id = a.allegato_id
  -- le righe «ultime» si prendono con un LEFT JOIN sulla tabella e una sottoquery nella condizione (non un LATERAL):
  -- cosi' sqlc vede le colonne nullabili
  LEFT JOIN documento doc ON doc.documento_id = (
      SELECT d.documento_id FROM documento_provenienza dp JOIN documento d ON d.documento_id = dp.documento_id
       WHERE dp.allegato_id = a.allegato_id AND d.thread_id = m.thread_id
       ORDER BY d.confermato_il DESC, d.documento_id LIMIT 1)
  LEFT JOIN file_rfq_rimozione r ON r.thread_id = m.thread_id AND r.allegato_id = a.allegato_id
  LEFT JOIN file_rfq_conservazione c ON c.conservazione_id = (
      SELECT max(x.conservazione_id) FROM file_rfq_conservazione x WHERE x.thread_id = m.thread_id AND x.allegato_id = a.allegato_id)
  LEFT JOIN file_rfq_evento es ON es.evento_id = (
      SELECT max(e.evento_id) FROM file_rfq_evento e
       WHERE e.thread_id = m.thread_id AND e.allegato_id = a.allegato_id AND e.azione = 'scarta')
  LEFT JOIN utente us ON us.utente_id = es.autore_id
  LEFT JOIN file_rfq_evento ea ON ea.evento_id = (
      SELECT max(e.evento_id) FROM file_rfq_evento e
       WHERE e.thread_id = m.thread_id AND e.allegato_id = a.allegato_id AND e.azione = 'altri_file')
  LEFT JOIN utente ua ON ua.utente_id = ea.autore_id
 WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND a.natura IN ('file', 'elemento_outlook')
 ORDER BY m.data_evento, a.messaggio_id, a.contenitore_id NULLS FIRST, a.indice, a.allegato_id;

-- name: ListEventiFileConAutore :many
-- La storia di un'occorrenza per il dettaglio di S1, con la sigla di chi ha fatto il gesto (gli eventi del server non
-- hanno un autore).
SELECT e.evento_id, e.azione, e.stato_prima, e.stato_dopo, e.motivo, e.esito, e.altro_allegato_id, e.origine, e.quando,
       u.sigla AS autore
  FROM file_rfq_evento e
  LEFT JOIN utente u ON u.utente_id = e.autore_id
 WHERE e.thread_id = sqlc.arg(thread_id) AND e.allegato_id = sqlc.arg(allegato_id)
 ORDER BY e.evento_id;

-- name: ListLavoriPendentiSorgente :many
-- I lavori pendenti sui file della RFQ, per lo stato dell'analisi di S1: il download (stage:<allegato>), l'estrazione
-- (estrai:<allegato>) e l'analisi, che e' per contenuto e vale per ogni allegato della RFQ con quello sha.
SELECT a.allegato_id, j.tipo::text AS tipo
  FROM job j
  JOIN allegato a ON (j.tipo IN ('stage_allegato', 'estrai_archivio') AND j.payload ->> 'allegato_id' = a.allegato_id::text)
                  OR (j.tipo = 'analizza_allegato' AND a.sha256 IS NOT NULL AND j.payload ->> 'sha256' = a.sha256::text)
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE j.stato IN ('pronto', 'in_corso') AND m.thread_id = sqlc.arg(thread_id)::uuid
 ORDER BY a.allegato_id, j.tipo;

-- ------------------------------------------------------------------ le dipendenze (EM §10.3)

-- name: ListDipendenzeFile :many
-- Le dipendenze D-1…D-5, D-7, D-7b, D-8, D-9, D-11, D-12, D-15 di ogni occorrenza dell'elenco (l'occorrenza del gesto
-- e, per un archivio, le sue voci), in una passata. D-6/D-6b vengono da ListDipendenzeBozzaPerFile, D-10 dal perimetro,
-- D-13 dai contesti, D-14 dal Go (i byte che restano). Ogni riga: l'occorrenza, il codice, un riferimento e
-- un'etichetta per il testo, un conteggio. Solo le occorrenze della RFQ: un allegato di un'altra RFQ non ha righe.
WITH occ AS (
    SELECT a.allegato_id, a.sha256, a.path_staging, m.thread_id
      FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
     WHERE a.allegato_id = ANY(sqlc.arg(allegati)::uuid[]) AND m.thread_id = sqlc.arg(thread_id)::uuid),
dip AS (
    -- D-1: documento confermato di cui l'occorrenza e' provenienza
    SELECT occ.allegato_id, 'D-1'::text AS codice, d.documento_id::text AS rif, COALESCE(d.codice, d.nome_file)::text AS etichetta, 0 AS n
      FROM occ JOIN documento_provenienza dp ON dp.allegato_id = occ.allegato_id
      JOIN documento d ON d.documento_id = dp.documento_id AND d.thread_id = occ.thread_id
    UNION ALL
    SELECT occ.allegato_id, 'D-1', p.proposta_id::text, COALESCE(p.codice, '')::text, 0
      FROM occ JOIN documento_proposta p ON p.allegato_id = occ.allegato_id
     WHERE p.stato IN ('confermata', 'duplicato')
    UNION ALL
    -- D-2: copia sul NAS in attesa di un documento della RFQ con lo stesso sha, di cui l'occorrenza non e' provenienza
    SELECT occ.allegato_id, 'D-2', d.documento_id::text, COALESCE(d.codice, d.nome_file)::text, 0
      FROM occ JOIN documento d ON d.thread_id = occ.thread_id AND d.sha256 = occ.sha256 AND d.stato_nas IN ('in_coda', 'errore')
     WHERE NOT EXISTS (SELECT 1 FROM documento_provenienza dp WHERE dp.documento_id = d.documento_id AND dp.allegato_id = occ.allegato_id)
    UNION ALL
    -- D-3: fonte autorizzata, dalle righe marcate strutturali lette da questo file
    SELECT occ.allegato_id, 'D-3', COALESCE(cp.componente_id::text, ''), COALESCE(c.codice, '')::text, 0
      FROM occ JOIN componente_proposta cp ON cp.allegato_id = occ.allegato_id AND cp.thread_id = occ.thread_id
      LEFT JOIN componente c ON c.componente_id = cp.componente_id
     WHERE cp.evidenza ? 'strutturale'
    UNION ALL
    -- D-3: fonte autorizzata, provenienza del documento in componente.step_strutturale_id
    SELECT occ.allegato_id, 'D-3', c.componente_id::text, c.codice::text, 0
      FROM occ JOIN documento_provenienza dp ON dp.allegato_id = occ.allegato_id
      JOIN componente c ON c.step_strutturale_id = dp.documento_id AND c.thread_id = occ.thread_id
    UNION ALL
    -- D-4: portatore delle righe di struttura della RFQ
    SELECT occ.allegato_id, 'D-4', '', '', (SELECT count(*) FROM componente_proposta cp
                                              WHERE cp.allegato_id = occ.allegato_id AND cp.thread_id = occ.thread_id)::int
      FROM occ
     WHERE EXISTS (SELECT 1 FROM componente_proposta cp WHERE cp.allegato_id = occ.allegato_id AND cp.thread_id = occ.thread_id)
    UNION ALL
    -- D-5: nodi gia' decisi da questo file
    SELECT occ.allegato_id, 'D-5', '', '', (SELECT count(*) FROM componente_proposta cp
                                              WHERE cp.allegato_id = occ.allegato_id AND cp.thread_id = occ.thread_id
                                                AND cp.componente_id IS NOT NULL AND cp.stato IN ('confermata', 'duplicato'))::int
      FROM occ
     WHERE EXISTS (SELECT 1 FROM componente_proposta cp WHERE cp.allegato_id = occ.allegato_id AND cp.thread_id = occ.thread_id
                      AND cp.componente_id IS NOT NULL AND cp.stato IN ('confermata', 'duplicato'))
    UNION ALL
    -- D-7 dalle AR (direttiva 35, C-6b; piano §7.2 C-6: «D-7 e D-7b dalle AR»): l'associazione registrata attiva
    -- dell'occorrenza (n = 1, anche senza una proposta aperta), oppure la copia senza AR di una proposta aperta (n = 0, la
    -- pre-assegnazione legacy o il legame della sorgente)
    SELECT occ.allegato_id, 'D-7', fra.componente_id::text, COALESCE(c.codice, '')::text, 1
      FROM occ JOIN file_rfq_associazione fra ON fra.allegato_id = occ.allegato_id AND fra.thread_id = occ.thread_id AND fra.stato = 'attiva'
      LEFT JOIN componente c ON c.componente_id = fra.componente_id
    UNION ALL
    SELECT occ.allegato_id, 'D-7', p.componente_id::text, COALESCE(c.codice, '')::text, 0
      FROM occ JOIN documento_proposta p ON p.allegato_id = occ.allegato_id
      LEFT JOIN componente c ON c.componente_id = p.componente_id
     WHERE p.stato = 'aperta' AND p.componente_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM file_rfq_associazione fra
                        WHERE fra.allegato_id = occ.allegato_id AND fra.thread_id = occ.thread_id AND fra.stato = 'attiva')
    UNION ALL
    -- D-7b: proposta aperta non assegnata e senza AR attiva
    SELECT occ.allegato_id, 'D-7b', p.proposta_id::text, '', 0
      FROM occ JOIN documento_proposta p ON p.allegato_id = occ.allegato_id
     WHERE p.stato = 'aperta' AND p.componente_id IS NULL
       AND NOT EXISTS (SELECT 1 FROM file_rfq_associazione fra
                        WHERE fra.allegato_id = occ.allegato_id AND fra.thread_id = occ.thread_id AND fra.stato = 'attiva')
    UNION ALL
    -- D-8: lavori in corso: download, estrazione, analisi con l'occorrenza nel payload
    SELECT occ.allegato_id, 'D-8', j.tipo::text, '', 0
      FROM occ JOIN job j ON j.stato IN ('pronto', 'in_corso')
       AND (j.chiave_idempotenza IN ('stage:' || occ.allegato_id::text, 'estrai:' || occ.allegato_id::text)
            OR (j.tipo = 'analizza_allegato' AND j.payload ->> 'allegato_id' = occ.allegato_id::text))
    UNION ALL
    -- D-9: lo stesso file indicato da altre occorrenze non scartate, anche di altre RFQ
    SELECT occ.allegato_id, 'D-9', '', '', count(*)::int
      FROM occ JOIN allegato x ON x.sha256 = occ.sha256 AND x.allegato_id <> occ.allegato_id
       AND x.path_staging IS NOT NULL AND occ.path_staging IS NOT NULL
       AND lower(replace(x.path_staging, '/', '\')) = lower(replace(occ.path_staging, '/', '\'))
      JOIN messaggio mx ON mx.messaggio_id = x.messaggio_id
     WHERE x.allegato_id <> ALL(sqlc.arg(allegati)::uuid[])
       AND NOT EXISTS (SELECT 1 FROM file_rfq fx WHERE fx.thread_id = mx.thread_id AND fx.allegato_id = x.allegato_id AND fx.stato = 'scartato')
       AND NOT (EXISTS (SELECT 1 FROM documento_proposta px WHERE px.allegato_id = x.allegato_id AND px.stato = 'scartata' AND px.deciso_da IS NOT NULL)
                AND NOT EXISTS (SELECT 1 FROM file_rfq fy WHERE fy.thread_id = mx.thread_id AND fy.allegato_id = x.allegato_id AND fy.stato <> 'attivo')
                AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ey WHERE ey.allegato_id = x.allegato_id AND ey.azione = 'scarta'))
     GROUP BY occ.allegato_id
    UNION ALL
    -- D-11: note sui PDF
    SELECT occ.allegato_id, 'D-11', '', '', count(*)::int
      FROM occ JOIN annotazione_pdf n ON n.allegato_id = occ.allegato_id AND n.thread_id = occ.thread_id
     GROUP BY occ.allegato_id
    UNION ALL
    -- D-12: anomalia NAS aperta su un documento della RFQ con lo stesso sha
    SELECT occ.allegato_id, 'D-12', d.documento_id::text, COALESCE(d.codice, d.nome_file)::text, 0
      FROM occ JOIN documento d ON d.thread_id = occ.thread_id AND d.sha256 = occ.sha256
      JOIN nas_anomalia n ON n.documento_id = d.documento_id AND n.risolta_il IS NULL
    UNION ALL
    -- D-15: storia della fonte e conferme che citano l'occorrenza
    SELECT occ.allegato_id, 'D-15', '', '', count(*)::int
      FROM occ JOIN fonte_step_evento fe ON fe.thread_id = occ.thread_id
       AND (fe.allegato_id = occ.allegato_id OR fe.precedente_allegato_id = occ.allegato_id)
     GROUP BY occ.allegato_id
    UNION ALL
    SELECT occ.allegato_id, 'D-15', '', '', count(*)::int
      FROM occ JOIN conferma_strutturale cs ON cs.thread_id = occ.thread_id
       AND cs.snapshot -> 'sorgente' ->> 'allegato_id' = occ.allegato_id::text
     GROUP BY occ.allegato_id)
SELECT dip.allegato_id, dip.codice, dip.rif::text AS rif, dip.etichetta::text AS etichetta, dip.n::int AS n
  FROM dip
 ORDER BY dip.allegato_id, dip.codice, dip.rif;

-- name: ListCodiciComponenti :many
-- Il codice di questi componenti della RFQ, per i testi delle dipendenze (il prodotto della bozza di D-6).
SELECT c.componente_id, c.codice FROM componente c
 WHERE c.thread_id = sqlc.arg(thread_id) AND c.componente_id = ANY(sqlc.arg(componenti)::uuid[]);

-- name: ListCopieStagingStessoSha :many
-- D-14 e il rilascio: gli altri allegati, di qualunque RFQ, con lo stesso contenuto e un percorso nello staging, con lo
-- stato della loro occorrenza (uno scarto col gesto precedente conta come scartato). Il Go verifica il file.
SELECT x.allegato_id, x.path_staging::text AS path_staging, mx.thread_id,
       (EXISTS (SELECT 1 FROM file_rfq fx WHERE fx.thread_id = mx.thread_id AND fx.allegato_id = x.allegato_id AND fx.stato = 'scartato')
        OR (EXISTS (SELECT 1 FROM documento_proposta px WHERE px.allegato_id = x.allegato_id AND px.stato = 'scartata' AND px.deciso_da IS NOT NULL)
            AND NOT EXISTS (SELECT 1 FROM file_rfq fy WHERE fy.thread_id = mx.thread_id AND fy.allegato_id = x.allegato_id AND fy.stato <> 'attivo')
            AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ey WHERE ey.allegato_id = x.allegato_id AND ey.azione = 'scarta')))::bool AS scartata,
       file_rfq_attivo(mx.thread_id, x.allegato_id)::bool AS attiva
  FROM allegato x JOIN messaggio mx ON mx.messaggio_id = x.messaggio_id
 WHERE x.sha256 = sqlc.arg(sha256)::char(64) AND x.path_staging IS NOT NULL
   AND x.allegato_id <> ALL(sqlc.arg(esclusi)::uuid[])
 ORDER BY x.allegato_id;

-- name: ListCopieStagingPerSha :many
-- D-14 in blocco (un archivio di 1.800 voci, PM23 SC-18): come ListCopieStagingStessoSha, per tutti i contenuti delle
-- occorrenze del gesto in una volta. Il Go verifica il file.
SELECT x.allegato_id, x.sha256::text AS sha256, x.path_staging::text AS path_staging,
       (EXISTS (SELECT 1 FROM file_rfq fx WHERE fx.thread_id = mx.thread_id AND fx.allegato_id = x.allegato_id AND fx.stato = 'scartato')
        OR (EXISTS (SELECT 1 FROM documento_proposta px WHERE px.allegato_id = x.allegato_id AND px.stato = 'scartata' AND px.deciso_da IS NOT NULL)
            AND NOT EXISTS (SELECT 1 FROM file_rfq fy WHERE fy.thread_id = mx.thread_id AND fy.allegato_id = x.allegato_id AND fy.stato <> 'attivo')
            AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ey WHERE ey.allegato_id = x.allegato_id AND ey.azione = 'scarta')))::bool AS scartata
  FROM allegato x JOIN messaggio mx ON mx.messaggio_id = x.messaggio_id
 WHERE x.sha256 = ANY(sqlc.arg(sha)::char(64)[]) AND x.path_staging IS NOT NULL
   AND x.allegato_id <> ALL(sqlc.arg(esclusi)::uuid[])
 ORDER BY x.allegato_id;

-- name: ListShaConservazioniCopiateAltre :many
-- D-14 in blocco: i contenuti che hanno una copia di conservazione verificata di un'ALTRA occorrenza.
SELECT DISTINCT c.sha256::text AS sha256 FROM file_rfq_conservazione c
 WHERE c.sha256 = ANY(sqlc.arg(sha)::char(64)[]) AND c.stato = 'copiata' AND c.allegato_id <> ALL(sqlc.arg(esclusi)::uuid[]);

-- name: ListShaDocumentiScritti :many
-- D-14 in blocco: i contenuti con un documento della RFQ scritto sul NAS, senza un'anomalia aperta.
SELECT DISTINCT d.sha256::text AS sha256 FROM documento d
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.sha256 = ANY(sqlc.arg(sha)::char(64)[]) AND d.stato_nas = 'scritto'
   AND NOT EXISTS (SELECT 1 FROM nas_anomalia n WHERE n.documento_id = d.documento_id AND n.risolta_il IS NULL);

-- name: EsisteDocumentoScrittoSha :one
-- Un documento della RFQ con questo contenuto scritto sul NAS, senza un'anomalia aperta: i byte restano raggiungibili.
SELECT EXISTS (SELECT 1 FROM documento d WHERE d.thread_id = sqlc.arg(thread_id) AND d.sha256 = sqlc.arg(sha256)::char(64)
                  AND d.stato_nas = 'scritto'
                  AND NOT EXISTS (SELECT 1 FROM nas_anomalia n WHERE n.documento_id = d.documento_id AND n.risolta_il IS NULL))::bool;

-- name: GetDocumentoScrittoPerSha :one
-- Il documento della RFQ con questo contenuto scritto sul NAS, senza anomalie aperte, con la cartella della RFQ: una
-- sorgente dei byte in sola lettura (copia di conservazione, S5). scritto_il e verificato_il dicono a S5 se il file e'
-- «in dubbio» (toccato dopo l'ultima volta che il server l'ha guardato): allora lo rilegge per lo sha, senza scrivere.
SELECT d.documento_id, d.path_relativo, d.bytes, d.sha256, d.scritto_il, d.verificato_il, t.cartella_relativa
  FROM documento d JOIN thread_offerta t ON t.thread_id = d.thread_id
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.sha256 = sqlc.arg(sha256)::char(64) AND d.stato_nas = 'scritto'
   AND NOT EXISTS (SELECT 1 FROM nas_anomalia n WHERE n.documento_id = d.documento_id AND n.risolta_il IS NULL)
 ORDER BY d.confermato_il DESC, d.documento_id
 LIMIT 1;

-- ------------------------------------------------------------------ l'intento dei gesti della Sorgente

-- name: GetEsitoIntentoPerChiave :one
-- L'esito di una chiave d'intento, di qualunque RFQ: una chiave di un'altra RFQ e' 422 FUORI_RFQ (EM §10.9).
SELECT * FROM esito_intento WHERE chiave = sqlc.arg(chiave);

-- ------------------------------------------------------------------ il tempo 1 dello scarto, Altri file, Riattiva

-- name: AnnullaJobPendentiPerChiave :many
-- I download e le estrazioni pendenti delle occorrenze del gesto (stage:<allegato>, estrai:<allegato>), annullati nel
-- tempo 1: un risultato tardivo riceve 409. Un job la cui riga e' gia' in mano a un risultato che sta scrivendo si
-- salta (SKIP LOCKED): aspettarlo, con la RFQ bloccata, rischierebbe un deadlock con le FK verso thread_offerta; quel
-- risultato lo ferma la guardia G-1 sotto BloccaContenuto.
UPDATE job SET stato = 'annullato'::stato_job, lease_fino_a = NULL, lease_token = NULL, worker_id = NULL,
       chiuso_il = now(), errore = concat_ws(' ', errore, '[annullato: file scartato nella Sorgente]')
 WHERE job_id IN (SELECT j.job_id FROM job j
                   WHERE j.chiave_idempotenza = ANY(sqlc.arg(chiavi)::text[]) AND j.stato IN ('pronto', 'in_corso')
                   ORDER BY j.job_id
                     FOR UPDATE SKIP LOCKED)
RETURNING job_id, chiave_idempotenza;

-- name: BloccaAllegatiPerFotografia :many
-- Le righe degli allegati del gesto FOR SHARE, in ordine, prima di leggerne la fotografia: il risultato tardivo di un
-- download (che scrive sha e byte) non la cambia fra la lettura e il trigger di file_rfq (EM §10.1; giro 1, TR-23).
SELECT a.allegato_id FROM allegato a
 WHERE a.allegato_id = ANY(sqlc.arg(allegati)::uuid[])
 ORDER BY a.allegato_id
   FOR SHARE;

-- name: BloccaPropostaDiAllegato :many
-- La proposta di documento degli allegati, FOR UPDATE, in ordine: il gesto e il risultato di un download prendono la
-- proposta prima dell'allegato, sempre nello stesso ordine.
SELECT p.proposta_id FROM documento_proposta p
 WHERE p.allegato_id = ANY(sqlc.arg(allegati)::uuid[])
 ORDER BY p.proposta_id
   FOR UPDATE;

-- name: ListConservazioniVive :many
-- Le copie di conservazione vive di un'occorrenza (in copia o copiata): lo scarto le porta a da_rilasciare.
SELECT * FROM file_rfq_conservazione
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id) AND stato IN ('in_copia', 'copiata')
 ORDER BY conservazione_id
   FOR UPDATE;

-- name: GetConservazionePerAggiornare :one
-- Una copia di conservazione, bloccata (fase C, battito, rinomina, rilascio).
SELECT * FROM file_rfq_conservazione WHERE conservazione_id = sqlc.arg(conservazione_id) FOR UPDATE;

-- name: GetConservazioneSeLibera :one
-- Una copia di conservazione, bloccata solo se nessun altro la sta toccando (il custode, SKIP LOCKED).
SELECT * FROM file_rfq_conservazione WHERE conservazione_id = sqlc.arg(conservazione_id) FOR UPDATE SKIP LOCKED;

-- name: GetConservazione :one
-- Una copia di conservazione, senza lucchetto.
SELECT * FROM file_rfq_conservazione WHERE conservazione_id = sqlc.arg(conservazione_id);

-- name: ListCopieConservazioneAllegato :many
-- Le copie di conservazione dello stesso allegato, anche sotto un'altra RFQ, ancora non rilasciate (copiata, o
-- da_rilasciare con il file presente: lo controlla il Go): una sorgente dei byte (S5, S3, Riattiva; EM §10.1).
SELECT * FROM file_rfq_conservazione
 WHERE allegato_id = sqlc.arg(allegato_id) AND stato IN ('copiata', 'da_rilasciare')
 ORDER BY (stato = 'copiata') DESC, conservazione_id DESC;

-- name: ListConservazioniDelGesto :many
-- Le copie di conservazione di un gesto (la chiave d'intento), per la risposta di S3 e del reinvio.
SELECT * FROM file_rfq_conservazione
 WHERE thread_id = sqlc.arg(thread_id) AND chiave_idempotenza = sqlc.arg(chiave)::uuid
 ORDER BY allegato_id, conservazione_id;

-- name: ListUltimaConservazione :many
-- L'ultima copia di conservazione di ogni occorrenza dell'elenco (stato corrente per le risposte).
SELECT DISTINCT ON (c.allegato_id) c.*
  FROM file_rfq_conservazione c
 WHERE c.thread_id = sqlc.arg(thread_id) AND c.allegato_id = ANY(sqlc.arg(allegati)::uuid[])
 ORDER BY c.allegato_id, c.conservazione_id DESC;

-- name: ListRimozioniOccorrenze :many
-- Le rimozioni delle occorrenze dell'elenco (stato corrente per le risposte).
SELECT * FROM file_rfq_rimozione
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = ANY(sqlc.arg(allegati)::uuid[])
 ORDER BY allegato_id;

-- name: GetRimozionePerAggiornare :one
-- Una rimozione, bloccata (il tempo 2 dopo il gesto e il reinvio).
SELECT * FROM file_rfq_rimozione WHERE rimozione_id = sqlc.arg(rimozione_id) FOR UPDATE;

-- name: GetRimozioneSeLibera :one
-- Una rimozione, bloccata solo se nessun altro la sta toccando (il custode, SKIP LOCKED).
SELECT * FROM file_rfq_rimozione WHERE rimozione_id = sqlc.arg(rimozione_id) FOR UPDATE SKIP LOCKED;

-- name: ListIdRimozioniDaRiprendere :many
-- Le rimozioni da riprendere (il custode), senza lucchetto: ognuna si prende poi nella sua transazione, SKIP LOCKED. Una
-- pagina alla volta, per chiave (rimozione_id > dopo): il custode le scorre tutte nella stessa passata, e una riga che
-- resta da riprendere (in mano a un altro, o con un errore) non torna nella stessa passata.
SELECT rimozione_id FROM file_rfq_rimozione
 WHERE stato IN ('in_attesa', 'errore') AND ritentabile
   AND (prossimo_tentativo_il IS NULL OR prossimo_tentativo_il <= sqlc.arg(adesso)::timestamptz)
   AND rimozione_id > sqlc.arg(dopo)::bigint
 ORDER BY rimozione_id
 LIMIT sqlc.arg(limite);

-- name: ListIdRimozioniNonRitentabili :many
-- Gli errori non ritentabili, per il controllo giornaliero della bonifica (EM §10.4: «ogni riga in errore non
-- ritentabile»). Una pagina alla volta, per chiave (rimozione_id > dopo): la bonifica le scorre tutte, anche quando le
-- prime restano in errore perche' i loro file ci sono ancora (rilievo bloccante della revisione).
SELECT rimozione_id FROM file_rfq_rimozione
 WHERE stato = 'errore' AND NOT ritentabile AND rimozione_id > sqlc.arg(dopo)::bigint
 ORDER BY rimozione_id
 LIMIT sqlc.arg(limite);

-- name: ListIdConservazioniDaRiprendere :many
-- Le copie da riprendere (il custode), senza lucchetto: da rilasciare con il tentativo scaduto, o in copia con il
-- battito fermo da piu' di 15 minuti (ultimo_battito, la colonna del battito: U2 §3.3 passo 2; senza battito l'inizio). Una pagina alla volta, per chiave
-- (conservazione_id > dopo), come le rimozioni. Non tornano le copie ferme su un esito che non si ritenta da solo (la
-- scrittura sul NAS spenta: «Riprova la rimozione dalla cartella ALTRO»; gli errori da bonificare: U2 §3.5, §3.7,
-- U2-P42, U2-P43); tornano le rimozioni dal NAS interrotte, ferme in in_rimozione da oltre 15 minuti.
SELECT conservazione_id FROM file_rfq_conservazione
 WHERE ((stato = 'da_rilasciare' AND (prossimo_tentativo_il IS NULL OR prossimo_tentativo_il <= sqlc.arg(adesso)::timestamptz)
         AND (esito_rimozione IS NULL
              OR esito_rimozione NOT IN ('scrittura_nas_spenta', 'radice_cambiata', 'fuori_cartella', 'nome_ripreso')))
        OR (stato = 'in_copia' AND COALESCE(ultimo_battito, iniziata_il) < sqlc.arg(adesso)::timestamptz - interval '15 minutes')
        OR (stato = 'in_rimozione' AND aggiornato_il < sqlc.arg(adesso)::timestamptz - interval '15 minutes'))
   AND conservazione_id > sqlc.arg(dopo)::bigint
 ORDER BY conservazione_id
 LIMIT sqlc.arg(limite);

-- name: ListConservazioniRecenti :many
-- Le copie toccate dopo un istante, senza lucchetto (il custode: le cartelle in cui cercare i parziali scaduti, U2
-- §3.9). Il segnaposto del deposito locale e i suoi conteggi all'avvio non ci sono piu' (U2 §3.13): la radice di una
-- copia e' radice_id (0023, variante 20261018d).
SELECT conservazione_id, radice_id, cartella_thread, percorso_relativo, sha256
  FROM file_rfq_conservazione
 WHERE aggiornato_il >= sqlc.arg(dopo)::timestamptz
 ORDER BY conservazione_id;

-- ------------------------------------------------------------------ il tempo 2 dello scarto

-- name: GetAllegatoConRfq :one
-- L'allegato con la RFQ del suo messaggio, letto di nuovo nel tempo 2 (la RFQ puo' essere cambiata: rfq_cambiata).
SELECT a.allegato_id, a.sha256, a.path_staging, a.nome_file, a.origine, a.contenitore_id, m.thread_id
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE a.allegato_id = sqlc.arg(allegato_id);

-- name: ContaLavoriPendentiOccorrenza :one
-- I lavori ancora pendenti di un'occorrenza: download ed estrazione per allegato, analisi con l'occorrenza nel payload.
SELECT count(*)::int FROM job j
 WHERE j.stato IN ('pronto', 'in_corso')
   AND (j.chiave_idempotenza IN ('stage:' || sqlc.arg(allegato_id)::text, 'estrai:' || sqlc.arg(allegato_id)::text)
        OR (j.tipo = 'analizza_allegato' AND j.payload ->> 'allegato_id' = sqlc.arg(allegato_id)::text));

-- name: CopiaNasOAnomaliaPerSha :one
-- Il tempo 2 aspetta una copia sul NAS in attesa o un'anomalia NAS aperta per lo stesso contenuto nella RFQ.
SELECT EXISTS (SELECT 1 FROM documento d WHERE d.thread_id = sqlc.arg(thread_id) AND d.sha256 = sqlc.arg(sha256)::char(64)
                  AND d.stato_nas IN ('in_coda', 'errore'))::bool AS copia_in_attesa,
       EXISTS (SELECT 1 FROM documento d JOIN nas_anomalia n ON n.documento_id = d.documento_id AND n.risolta_il IS NULL
                WHERE d.thread_id = sqlc.arg(thread_id) AND d.sha256 = sqlc.arg(sha256)::char(64))::bool AS anomalia_aperta;

-- name: PinDelFile :one
-- I pin di UN file dello staging per il tempo 2 (EM §10.4 passo 8; §10.6): il documento in attesa di copia, l'anomalia
-- NAS e il job pendente valgono per lo sha (fanno solo aspettare); il pin «proposta aperta» vale per il file, cioe'
-- solo per le proposte aperte di allegati attivi il cui path_staging e' quel file; la «conservazione in corso» per lo
-- sha. Nessun pin nato dall'occorrenza esclusa (la sua proposta, la sua conservazione). '' = nessun pin.
SELECT COALESCE((
    SELECT x.motivo FROM (
        SELECT 'documento in attesa di copia'::text AS motivo, 1 AS ordine
         WHERE EXISTS (SELECT 1 FROM documento d WHERE d.sha256 = sqlc.arg(sha256)::char(64) AND d.stato_nas IN ('in_coda', 'errore'))
        UNION ALL
        SELECT 'anomalia NAS aperta', 2
         WHERE EXISTS (SELECT 1 FROM nas_anomalia n JOIN documento d ON d.documento_id = n.documento_id
                        WHERE d.sha256 = sqlc.arg(sha256)::char(64) AND n.risolta_il IS NULL)
        UNION ALL
        SELECT 'proposta ancora aperta', 3
         WHERE EXISTS (SELECT 1 FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
                         JOIN messaggio m ON m.messaggio_id = a.messaggio_id
                        WHERE a.sha256 = sqlc.arg(sha256)::char(64) AND p.stato = 'aperta' AND a.allegato_id <> sqlc.arg(escluso)::uuid
                          AND a.path_staging IS NOT NULL
                          AND lower(replace(a.path_staging, '/', '\')) = lower(replace(sqlc.arg(percorso)::text, '/', '\'))
                          AND file_rfq_attivo(m.thread_id, a.allegato_id))
        UNION ALL
        SELECT 'job pendente', 4
         WHERE EXISTS (SELECT 1 FROM job j WHERE j.stato IN ('pronto', 'in_corso') AND (
                   j.payload ->> 'sha256' = sqlc.arg(sha256)::text
                OR EXISTS (SELECT 1 FROM allegato a WHERE a.allegato_id::text = j.payload ->> 'allegato_id' AND a.sha256 = sqlc.arg(sha256)::char(64)
                              AND a.allegato_id <> sqlc.arg(escluso)::uuid)
                OR EXISTS (SELECT 1 FROM documento d WHERE d.documento_id::text = j.payload ->> 'documento_id' AND d.sha256 = sqlc.arg(sha256)::char(64))))
        UNION ALL
        SELECT 'conservazione in corso', 5
         WHERE EXISTS (SELECT 1 FROM file_rfq_conservazione c WHERE c.sha256 = sqlc.arg(sha256)::char(64) AND c.stato = 'in_copia'
                          AND c.allegato_id <> sqlc.arg(escluso)::uuid)
    ) x ORDER BY x.ordine LIMIT 1), '')::text AS motivo;

-- name: ListAltriAllegatiStessoSha :many
-- Gli altri allegati con lo stesso contenuto e un percorso nello staging, con lo stato della loro occorrenza: il tempo
-- 2 non cancella un file indicato da un'occorrenza non scartata (passo 7), e il passo 4 cerca chi indica lo stesso
-- percorso. Il confronto dei percorsi (pulito, senza maiuscole) lo fa il Go.
SELECT x.allegato_id, x.path_staging::text AS path_staging, mx.thread_id,
       (EXISTS (SELECT 1 FROM file_rfq fx WHERE fx.thread_id = mx.thread_id AND fx.allegato_id = x.allegato_id AND fx.stato = 'scartato')
        OR (EXISTS (SELECT 1 FROM documento_proposta px WHERE px.allegato_id = x.allegato_id AND px.stato = 'scartata' AND px.deciso_da IS NOT NULL)
            AND NOT EXISTS (SELECT 1 FROM file_rfq fy WHERE fy.thread_id = mx.thread_id AND fy.allegato_id = x.allegato_id AND fy.stato <> 'attivo')
            AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ey WHERE ey.allegato_id = x.allegato_id AND ey.azione = 'scarta')))::bool AS scartata
  FROM allegato x JOIN messaggio mx ON mx.messaggio_id = x.messaggio_id
 WHERE x.path_staging IS NOT NULL AND x.allegato_id <> sqlc.arg(escluso)::uuid
   AND (x.sha256 = sqlc.narg(sha256)::char(64)
        OR lower(replace(x.path_staging, '/', '\')) = lower(replace(sqlc.arg(percorso)::text, '/', '\')))
 ORDER BY x.allegato_id;

-- name: AzzeraPathStagingAllegato :execrows
-- Il riferimento di UNA occorrenza al file dello staging, tolto (tempo 2, passi 4 e 7).
UPDATE allegato SET path_staging = NULL WHERE allegato_id = sqlc.arg(allegato_id) AND path_staging IS NOT NULL;

-- ------------------------------------------------------------------ l'ingresso (E1′ e il ramo HTML di carica)

-- name: ListAllegatiDirettiStessoSha :many
-- I doppioni di un file caricato: gli allegati diretti dello stesso contenitore con lo stesso contenuto, con lo stato
-- della loro occorrenza (uno scarto col gesto precedente conta come scartato). Si legge dopo BloccaMessaggio(nota),
-- nella transazione della registrazione (EM §10.9, E1′; giro 1, TR-10).
SELECT x.allegato_id, x.nome_file, x.path_staging, mx.thread_id,
       COALESCE(fx.stato, 'attivo')::text AS stato,
       ((fx.stato IS NULL OR fx.stato = 'attivo')
        AND EXISTS (SELECT 1 FROM documento_proposta px WHERE px.allegato_id = x.allegato_id AND px.stato = 'scartata'
                       AND px.deciso_da IS NOT NULL)
        AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ez WHERE ez.allegato_id = x.allegato_id AND ez.azione = 'scarta'))::bool AS scartato_legacy
  FROM allegato x JOIN messaggio mx ON mx.messaggio_id = x.messaggio_id
  LEFT JOIN file_rfq fx ON fx.thread_id = mx.thread_id AND fx.allegato_id = x.allegato_id
 WHERE x.messaggio_id = sqlc.arg(messaggio_id) AND x.contenitore_id IS NULL AND x.sha256 = sqlc.arg(sha256)::char(64)
 ORDER BY x.allegato_id;

-- ------------------------------------------------------------------ le guardie (EM §10.7)

-- name: StatoOccorrenzaAllegato :one
-- Lo stato dell'occorrenza di un allegato nella RFQ del suo messaggio, per le guardie degli scrittori e dei gesti
-- legacy: attivo (nessuna riga), altri_file o scartato; scartato_legacy per uno scarto col vecchio «Scarta il file».
SELECT a.allegato_id, m.thread_id, COALESCE(f.stato, 'attivo')::text AS stato,
       ((f.stato IS NULL OR f.stato = 'attivo')
        AND EXISTS (SELECT 1 FROM documento_proposta p WHERE p.allegato_id = a.allegato_id AND p.stato = 'scartata'
                       AND p.deciso_da IS NOT NULL)
        AND NOT EXISTS (SELECT 1 FROM file_rfq_evento ez WHERE ez.allegato_id = a.allegato_id AND ez.azione = 'scarta'))::bool AS scartato_legacy
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN file_rfq f ON f.thread_id = m.thread_id AND f.allegato_id = a.allegato_id
 WHERE a.allegato_id = sqlc.arg(allegato_id);

-- name: ListOccorrenzeNonAttiveRfq :many
-- Gli allegati della RFQ in Altri file o scartati con il ciclo dei file (l'elenco NonAttivi: EM §10.8).
SELECT f.allegato_id, f.stato FROM file_rfq f
 WHERE f.thread_id = sqlc.arg(thread_id) AND f.stato <> 'attivo'
 ORDER BY f.allegato_id;

-- name: ListConservazioniCopiatePerSha :many
-- Le copie verificate di occorrenze in Altri file per questi contenuti: la cache non conta il pin «proposta aperta» di
-- un'occorrenza in Altri file solo dopo aver visto il file della sua copia nel deposito con la dimensione attesa
-- (EM §10.5; PM23 §7).
SELECT c.sha256::char(64) AS sha256, c.cartella_thread, c.percorso_relativo, c.bytes, c.radice_id
  FROM file_rfq_conservazione c
  JOIN file_rfq f ON f.thread_id = c.thread_id AND f.allegato_id = c.allegato_id AND f.stato = 'altri_file'
 WHERE c.stato = 'copiata' AND c.sha256 = ANY(sqlc.arg(sha)::char(64)[])
   AND EXISTS (SELECT 1 FROM documento_proposta p WHERE p.allegato_id = c.allegato_id AND p.stato = 'aperta')
 ORDER BY c.sha256, c.conservazione_id;

-- ------------------------------------------------------------------ la copia nella cartella ALTRO (direttiva 35, C-6a)

-- name: AggiornaConservazione :execrows
-- Un passo di una copia degli Altri file dallo stato atteso (confronta e scambia), con le colonne della 0023 che la
-- corsia file scrive (U2 §3.3, §3.4, §3.7): la prova di creazione (creato_dal_cockpit: si accende, non si spegne mai),
-- il battito (ultimo_battito = now()), il token della rimozione (scritto entrando in in_rimozione, poi conservato) e
-- l'esito della rimozione dal NAS (esito_rimozione: lo stato lo vuole, ck_cons_esito). Le transizioni le controlla il
-- trigger (P4X09); verificato_il e concluso_il li scrive lui.
UPDATE file_rfq_conservazione
   SET stato = sqlc.arg(stato), causa = sqlc.narg(causa), errore = sqlc.narg(errore), tentativi = sqlc.arg(tentativi),
       prossimo_tentativo_il = sqlc.narg(prossimo_tentativo_il),
       sorgente_byte = COALESCE(sqlc.narg(sorgente_byte), sorgente_byte),
       bytes = COALESCE(sqlc.narg(bytes), bytes),
       creato_dal_cockpit = creato_dal_cockpit OR sqlc.arg(prova)::bool,
       token_rimozione = COALESCE(sqlc.narg(token_rimozione)::uuid, token_rimozione),
       esito_rimozione = sqlc.narg(esito_rimozione),
       ultimo_battito = CASE WHEN sqlc.arg(battito)::bool THEN now() ELSE ultimo_battito END
 WHERE conservazione_id = sqlc.arg(conservazione_id) AND stato = sqlc.arg(stato_atteso);

-- name: ListConservazioniTrattenuteRfq :many
-- Le copie trattenute del cockpit nella RFQ con questo contenuto, sotto questa radice e nella cartella della RFQ di
-- adesso (base B di U2-Q01, U2 §3.8): un nuovo «Metti in Altri file» ne riusa il file ed eredita la prova (eredita_da;
-- il trigger di nascita ricontrolla tutto e cede la riga vecchia). Si leggono in fase A, sotto BloccaThread e
-- BloccaCartella(thread, «ALTRO»): la piu' recente prima.
SELECT * FROM file_rfq_conservazione
 WHERE thread_id = sqlc.arg(thread_id) AND sha256 = sqlc.arg(sha256)::char(64) AND stato = 'trattenuta' AND creato_dal_cockpit
   AND radice_id = sqlc.arg(radice_id) AND cartella_thread = sqlc.arg(cartella_thread)::text
 ORDER BY conservazione_id DESC;

-- name: GetConservazioneCopiataRiga :one
-- La copia verificata di un'occorrenza in Altri file, con tutte le colonne (la cartella della RFQ fotografata nella
-- riga serve per trovare il file: U2 §3.4). Per «Riattiva».
SELECT * FROM file_rfq_conservazione
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id) AND stato = 'copiata';

-- name: PercorsoDiUnDocumento :one
-- Il percorso di una copia degli Altri file e' anche quello di un documento della RFQ, di uno spostamento pendente (da
-- o a) o di un orfano aperto: la rimozione dal NAS non lo tocca (U2 §3.6 regola 2, §3.7). Le stesse tre condizioni di
-- PercorsoOccupato, senza le copie di conservazione (la riga che si rilascia nomina quel percorso). In minuscolo: la
-- condivisione e' Windows. Si legge sotto BloccaCartella(thread, «ALTRO»).
SELECT (EXISTS (SELECT 1 FROM documento d
                 WHERE d.thread_id = sqlc.arg(thread_id) AND lower(d.path_relativo) = lower(sqlc.arg(percorso)::text))
     OR EXISTS (SELECT 1 FROM job j
                 WHERE j.tipo = 'sposta_nas' AND j.stato IN ('pronto', 'in_corso')
                   AND j.payload ->> 'thread_id' = sqlc.arg(thread_id)::uuid::text
                   AND (lower(j.payload ->> 'da') = lower(sqlc.arg(percorso)::text) OR lower(j.payload ->> 'a') = lower(sqlc.arg(percorso)::text)))
     OR EXISTS (SELECT 1 FROM nas_orfano o
                 WHERE o.thread_id = sqlc.arg(thread_id) AND o.risolto_il IS NULL AND lower(o.percorso) = lower(sqlc.arg(percorso)::text)))::bool AS documento;

-- name: ListTokenConservazioniInCopia :many
-- I token delle copie degli Altri file in corso (in_copia): i loro parziali ALTRO\<nome>.parte.<token> sono vivi per
-- chiunque pulisca i parziali della cartella ALTRO, i documenti come il custode (U2 §3.9: l'unione dei token).
SELECT token FROM file_rfq_conservazione WHERE stato = 'in_copia' ORDER BY token;

-- name: ListCitazioniRegistroConferme :many
-- D-15b (U2-P04, U2 §4.6): le occorrenze dell'elenco citate dal registro d'importazione di una conferma della RFQ
-- (conferma_strutturale.importazioni_citate → distinta_importazione.allegati_citati), con il numero della conferma e il
-- codice del prodotto. Si mostra, non blocca.
SELECT DISTINCT a.allegato_id, cs.conferma_id, cs.numero, co.codice AS prodotto
  FROM conferma_strutturale cs
  JOIN distinta_importazione di ON di.thread_id = cs.thread_id AND di.importazione_id = ANY (cs.importazioni_citate)
  JOIN allegato a ON a.allegato_id = ANY (di.allegati_citati)
  JOIN componente co ON co.componente_id = cs.prodotto_id
 WHERE cs.thread_id = sqlc.arg(thread_id) AND a.allegato_id = ANY (sqlc.arg(allegati)::uuid[])
 ORDER BY a.allegato_id, cs.numero, cs.conferma_id;

-- name: DocumentoStepDelFinito :one
-- Il documento e' uno STEP del prodotto finito (direttiva 39 §2, §5 [U]: «Cambiarlo = S8, non un normale gesto
-- documentale»): lo STEP strutturale della working di un finito della RFQ, la sorgente di una bozza aperta di un prodotto
-- della RFQ, o il contenuto di un allegato della RFQ legato come sorgente strutturale del finito (il predicato della 0023,
-- allegato_sorgente_del_finito: la conferma della sorgente non revocata, il legame storico senza S8, la working, la
-- bozza). I gesti legacy «Sostituisci», «Annulla sostituzione» e «Assegna» dei documenti nelle RFQ formato 3 lo leggono
-- per rimandare a S8 e R5 (fascicolo/formato3_legacy.go). Nessuna riga = il documento non e' della RFQ.
SELECT (EXISTS (SELECT 1 FROM componente c
                 WHERE c.thread_id = d.thread_id AND c.tipo = 'finito' AND c.step_strutturale_id = d.documento_id)
     OR EXISTS (SELECT 1 FROM distinta_bozza b
                 WHERE b.thread_id = d.thread_id AND b.stato = 'aperta' AND b.fonte_sha256::text = d.sha256::text)
     OR EXISTS (SELECT 1 FROM allegato x JOIN messaggio m ON m.messaggio_id = x.messaggio_id
                 WHERE m.thread_id = d.thread_id AND x.sha256::text = d.sha256::text
                   AND allegato_sorgente_del_finito(d.thread_id, x.allegato_id)))::bool AS step_del_finito
  FROM documento d
 WHERE d.documento_id = sqlc.arg(documento_id) AND d.thread_id = sqlc.arg(thread_id);

-- name: ListAssociazioniAttiveDelDocumento :many
-- Le AR attive (file_rfq_associazione) degli allegati della RFQ con il contenuto del documento, con il codice del
-- componente: «Assegna» di un documento confermato nelle RFQ formato 3 non lo porta via dal componente della AR (la
-- guardia di INC-G9, D-AR07 [T-39]). In ordine di codice e di allegato.
SELECT f.allegato_id, f.componente_id, c.codice
  FROM documento d
  JOIN allegato x ON x.sha256::text = d.sha256::text
  JOIN messaggio m ON m.messaggio_id = x.messaggio_id AND m.thread_id = d.thread_id
  JOIN file_rfq_associazione f ON f.thread_id = d.thread_id AND f.allegato_id = x.allegato_id AND f.stato = 'attiva'
  JOIN componente c ON c.componente_id = f.componente_id
 WHERE d.documento_id = sqlc.arg(documento_id) AND d.thread_id = sqlc.arg(thread_id)
 ORDER BY c.codice, f.allegato_id;
