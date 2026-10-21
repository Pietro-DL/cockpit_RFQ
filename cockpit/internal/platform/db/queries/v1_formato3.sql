-- V1 (giro 5, A1d; direttiva 23): le query della migrazione 0023 (piano della 0023, §6 e §7). Il formato 3 della bozza e
-- della conferma, il registro d'importazione del Motore A, i nodi non inclusi (LS-02), l'identita' certificata dei
-- componenti, gli atti della fonte, l'esito degli intenti e il ciclo dei file della Sorgente.
--
-- Chi le chiama: core/rfq/distintabozza e le rotte R1-R7 (corsia persistenza), il costruttore del registro e il modello
-- della pagina (corsia motore, con le query della persistenza), il servizio del ciclo dei file (corsia file). In V1, al
-- primo commit, le chiamano solo le prove L4 della 0023.
--
-- Regole (le stesse di p4_conferme.sql):
--   - le date della storia e le transazioni le scrive il database: qui non si passano;
--   - le colonne tx (xid8) non si leggono;
--   - il perimetro sta nel WHERE: ogni riga si legge con la RFQ del percorso, e fuori da quella non si trova;
--   - nessuna query usata da R2-R5 ha ON CONFLICT (BL V1-Q04 A; prova T-03): un doppione e' un errore, non una fusione.

-- ------------------------------------------------------------------ la bozza formato 3

-- name: GetVersionePerChiaveSalvataggio :one
-- La versione salvata con una chiave di salvataggio R2 (unica: ux_dbv_chiave_salvataggio), nella RFQ: un reinvio con la
-- stessa chiave e la stessa impronta della richiesta risponde con quella versione, senza scriverne un'altra.
SELECT v.bozza_id, v.versione, v.impronta, v.impronta_richiesta, v.importazione_id, v.salvata_da, v.salvata_il
  FROM distinta_bozza_versione v
  JOIN distinta_bozza b ON b.bozza_id = v.bozza_id
 WHERE v.chiave_salvataggio = sqlc.arg(chiave_salvataggio) AND b.thread_id = sqlc.arg(thread_id);

-- name: GetSorgenteBozzaAperta :one
-- Per il modello della pagina (bozza_del_prodotto di /dati, EM §6.4.1): la bozza aperta formato 3 del prodotto, la sua
-- versione corrente e, dal registro della versione, gruppo, fonte, radice e impronte; mai il contenuto. Senza registro
-- le colonne del registro sono NULL e vale la fonte della testata.
SELECT b.bozza_id, b.versione, b.formato, b.fonte_allegato_id, b.fonte_sha256, b.fonte_gruppo, v.importazione_id,
       i.gruppo AS registro_gruppo, i.fonte_allegato_id AS registro_fonte_allegato_id, i.fonte_sha256 AS registro_fonte_sha256,
       i.fonte_radice AS registro_fonte_radice, i.impronta AS registro_impronta, i.impronta_contenuto AS registro_impronta_contenuto,
       i.impronta_evidenze AS registro_impronta_evidenze, i.impronta_regole AS registro_impronta_regole
  FROM distinta_bozza b
  JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
  LEFT JOIN distinta_importazione i ON i.importazione_id = v.importazione_id
 WHERE b.thread_id = sqlc.arg(thread_id) AND b.prodotto_id = sqlc.arg(prodotto_id) AND b.stato = 'aperta' AND b.formato = 3;

-- ------------------------------------------------------------------ il registro d'importazione

-- name: InsertDistintaImportazione :one
-- Un registro nuovo: solo dentro una forma di R2 costruita dal server, dopo GetDistintaImportazionePerImpronta sotto il
-- lucchetto della RFQ (prima si cerca, poi si scrive: niente ON CONFLICT). Il trigger vuole la fonte e gli allegati
-- citati della RFQ e attivi (P4X08); data e transazione le scrive il database.
INSERT INTO distinta_importazione (thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole,
                                   formato_registro, versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice,
                                   allegati_citati, contenuto, creata_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(prodotto_id), sqlc.arg(impronta), sqlc.arg(impronta_contenuto), sqlc.arg(impronta_evidenze),
        sqlc.arg(impronta_regole), sqlc.arg(formato_registro), sqlc.arg(versione_importazione), sqlc.arg(gruppo),
        sqlc.narg(fonte_allegato_id), sqlc.narg(fonte_sha256), sqlc.narg(fonte_radice), sqlc.arg(allegati_citati)::uuid[],
        sqlc.arg(contenuto), sqlc.arg(creata_da))
RETURNING importazione_id, creata_il;

-- name: GetDistintaImportazione :one
-- Un registro della RFQ, intero (contenuto compreso).
SELECT importazione_id, thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole, formato_registro,
       versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da, creata_il
  FROM distinta_importazione
 WHERE importazione_id = sqlc.arg(importazione_id) AND thread_id = sqlc.arg(thread_id);

-- name: GetDistintaImportazionePerImpronta :one
-- Il registro di un prodotto della RFQ con un'impronta (ux_dimp_impronta: stessa proposta, stesso registro).
SELECT importazione_id, thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole, formato_registro,
       versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da, creata_il
  FROM distinta_importazione
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id) AND impronta = sqlc.arg(impronta);

-- name: ListImportazioniPerImpronte :many
-- I registri di un prodotto della RFQ con le impronte date (le decisioni citano il registro su cui sono state prese,
-- EM §3.6.1): interi, per verificare le copie delle letture e i segnali visti.
SELECT importazione_id, thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole, formato_registro,
       versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da, creata_il
  FROM distinta_importazione
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id) AND impronta = ANY (sqlc.arg(impronte)::text[])
 ORDER BY creata_il, importazione_id;

-- name: ListImportazioniProdotto :many
-- I registri d'importazione di un prodotto della RFQ, dal primo, senza il contenuto (PM23 §6).
SELECT importazione_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole, gruppo, fonte_allegato_id, fonte_sha256,
       fonte_radice, creata_da, creata_il
  FROM distinta_importazione
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY creata_il, importazione_id;

-- ------------------------------------------------------------------ la conferma formato 3 (R5)

-- name: InsertComponenteSenzaFusione :one
-- Un componente nuovo creato da R5: una INSERT senza ON CONFLICT (BL V1-Q04 A; EM §5.4). Un 23505 vuol dire che uno
-- scrittore legacy ha scritto senza il lucchetto della RFQ: 409 BASE_SUPERATA, motivo codice_occupato.
INSERT INTO componente (thread_id, codice, rev, descrizione, qta, tipo, origine, confermato_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(codice), sqlc.narg(rev), sqlc.narg(descrizione), sqlc.arg(qta), sqlc.arg(tipo), sqlc.arg(origine),
        sqlc.arg(confermato_da))
RETURNING componente_id;

-- name: InsertConfermaStrutturale :one
-- La testata della conferma, scritta dopo le scritture della working (il trigger di nascita la rende l'ultima scrittura
-- della working nella transazione). Formato 3: bozza, chiave, registro della versione e tutti i registri citati dallo
-- snapshot; i formati 1 e 2 passano NULL in importazione_id e importazioni_citate.
INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
                                  firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id,
                                  bozza_versione, fonte_allegato_id, fonte_sha256, fonte_gruppo, origine, snapshot, impronta_snapshot,
                                  delta, mappa, decisioni, mono_pezzo, mono_pezzo_motivo, criterio_verifica, verifica, chiave_idempotenza,
                                  confermato_da, importazione_id, importazioni_citate)
VALUES (sqlc.arg(thread_id), sqlc.arg(prodotto_id), sqlc.arg(numero), sqlc.narg(precedente_id), sqlc.arg(formato),
        sqlc.arg(senza_riepilogo_v2), sqlc.arg(firma_struttura), sqlc.narg(firma_anteprima), sqlc.narg(base_conferma_id),
        sqlc.arg(base_contatore), sqlc.narg(firma_base), sqlc.arg(contatore), sqlc.arg(contatore_dati), sqlc.narg(bozza_id),
        sqlc.narg(bozza_versione), sqlc.narg(fonte_allegato_id), sqlc.narg(fonte_sha256), sqlc.narg(fonte_gruppo), sqlc.arg(origine),
        sqlc.arg(snapshot), sqlc.arg(impronta_snapshot), sqlc.arg(delta), sqlc.arg(mappa), sqlc.arg(decisioni), sqlc.arg(mono_pezzo),
        sqlc.narg(mono_pezzo_motivo), sqlc.arg(criterio_verifica), sqlc.arg(verifica), sqlc.narg(chiave_idempotenza),
        sqlc.arg(confermato_da), sqlc.narg(importazione_id), sqlc.narg(importazioni_citate)::uuid[])
RETURNING conferma_id, confermato_il;

-- name: InsertConfermaStrutturaleComponente :exec
-- Un componente certificato, uguale alla working (trigger). Formato 3: emittente e presunzione obbligatori, namespace se
-- l'identita' decisa ne ha uno; formati 1 e 2: NULL.
INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo,
                                             step_strutturale_id, step_sha256, emittente, emittente_presunto, namespace)
VALUES (sqlc.arg(conferma_id), sqlc.arg(thread_id), sqlc.arg(componente_id), sqlc.arg(radice), sqlc.narg(chiave_bozza), sqlc.arg(codice),
        sqlc.narg(rev), sqlc.narg(descrizione), sqlc.arg(tipo), sqlc.narg(step_strutturale_id), sqlc.narg(step_sha256),
        sqlc.narg(emittente), sqlc.narg(emittente_presunto), sqlc.narg(namespace));

-- name: InsertConfermaStrutturaleArco :exec
-- Un arco certificato con la qta e l'origine (obbligatoria nei formati 2 e 3).
INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, posizione, origine)
VALUES (sqlc.arg(conferma_id), sqlc.arg(padre_id), sqlc.arg(figlio_id), sqlc.arg(qta), sqlc.narg(posizione), sqlc.narg(origine));

-- name: InsertConfermaNonIncluso :exec
-- La traccia per nodo di un'esclusione applicata (LS-02, EM §7.5; U2 §6.1): una decisione non_includere (origine
-- non_includere, con decisione, azione e motivo) o un'esclusione senza decisione propria (assenza, legacy, ereditata,
-- escludi: decisione, azione e motivo facoltativi). Solo formato 3, nella transazione della conferma e prima del
-- controllo finale (trigger).
INSERT INTO conferma_strutturale_non_incluso (conferma_id, thread_id, sha256, chiave, allegato_id, gruppo, origine, decisione_id, azione_id,
                                              motivo_codice, motivo, segnale, righe_chiuse, archi_chiusi)
VALUES (sqlc.arg(conferma_id), sqlc.arg(thread_id), sqlc.arg(sha256), sqlc.arg(chiave), sqlc.arg(allegato_id), sqlc.arg(gruppo),
        sqlc.arg(origine), sqlc.narg(decisione_id), sqlc.narg(azione_id), sqlc.narg(motivo_codice), sqlc.narg(motivo), sqlc.narg(segnale),
        sqlc.arg(righe_chiuse)::uuid[], sqlc.arg(archi_chiusi));

-- name: InsertAssociazioneEvento :one
-- Un evento di associazione: preassociazione applicata da R5 (origine conferma_bom, con conferma_id), svincolo
-- (sgancia), e gli altri gesti delle proposte.
INSERT INTO associazione_evento (thread_id, proposta_id, documento_id, azione, componente_prima, componente_dopo, conferma_id, motivo,
                                 origine, chiave_idempotenza, autore_id)
VALUES (sqlc.arg(thread_id), sqlc.narg(proposta_id), sqlc.narg(documento_id), sqlc.arg(azione), sqlc.narg(componente_prima),
        sqlc.narg(componente_dopo), sqlc.narg(conferma_id), sqlc.narg(motivo), sqlc.arg(origine), sqlc.narg(chiave_idempotenza),
        sqlc.arg(autore_id)::uuid)
RETURNING evento_id, quando;

-- name: InsertF8StatoConferma :one
-- Lo stato di F8 di una conferma, in attesa, nella transazione della conferma (F8 gira dopo il COMMIT).
INSERT INTO f8_stato (thread_id, chiave_intento, conferma_id)
VALUES (sqlc.arg(thread_id), sqlc.narg(chiave_intento), sqlc.narg(conferma_id))
RETURNING f8_id;

-- ------------------------------------------------------------------ gli atti della fonte

-- name: InsertFonteStepEvento :one
-- Un evento della storia della fonte STEP, nella transazione dell'atto: conferma_documento («Conferma la fonte»),
-- autorizzazione, revoca, scelta e sostituzione (R2), caricamento (S7). Per una fonte secondaria con la destinazione
-- temporanea la copia del nodo la scrive il database (destinazione); la risoluzione la scrive solo la conferma.
INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, componente_id, chiave_bozza, azione, allegato_id, sha256, gruppo, proposta_id,
                               documento_id, precedente_allegato_id, precedente_sha256, precedente_gruppo, bozza_id, motivo,
                               chiave_idempotenza, autore_id)
VALUES (sqlc.arg(thread_id), sqlc.arg(prodotto_id), sqlc.arg(ruolo), sqlc.narg(componente_id), sqlc.narg(chiave_bozza), sqlc.arg(azione),
        sqlc.arg(allegato_id), sqlc.arg(sha256), sqlc.narg(gruppo), sqlc.narg(proposta_id), sqlc.narg(documento_id),
        sqlc.narg(precedente_allegato_id), sqlc.narg(precedente_sha256), sqlc.narg(precedente_gruppo), sqlc.narg(bozza_id),
        sqlc.narg(motivo), sqlc.narg(chiave_idempotenza), sqlc.arg(autore_id))
RETURNING evento_id, quando;

-- name: ListFonteStepProdotto :many
-- La storia della fonte STEP di un prodotto della RFQ (R7 fonte[]), in ordine: ruolo prodotto e sottoassieme.
SELECT evento_id, ruolo, componente_id, chiave_bozza, destinazione, risolve_evento_id, azione, allegato_id, sha256, gruppo, proposta_id,
       documento_id, precedente_allegato_id, precedente_sha256, precedente_gruppo, bozza_id, motivo, chiave_idempotenza, autore_id, quando
  FROM fonte_step_evento
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY evento_id;

-- ------------------------------------------------------------------ R4: identita', collisioni, righe legacy

-- name: ListIdentitaRegistrate :many
-- L'identita' certificata di ogni componente dato, dall'ultima conferma formato 3 che lo contiene (EM §5.1), con la
-- validita': il codice certificato e' byte per byte il codice di oggi (non upper()) e il componente non e' archiviato.
-- Nessuna riga = componente legacy (cliente presunto, senza namespace). valida falso = certificazione superata da una
-- ricodifica legacy (avviso EMITTENTE_CERTIFICATO_SUPERATO).
SELECT u.componente_id, u.codice, u.emittente, u.emittente_presunto, u.namespace, u.conferma_id, u.numero, u.confermato_il,
       w.codice AS codice_working, (u.codice = w.codice AND w.archiviato_il IS NULL)::bool AS valida
  FROM (SELECT DISTINCT ON (k.componente_id) k.componente_id, k.codice, k.emittente, k.emittente_presunto, k.namespace,
               c.conferma_id, c.numero, c.confermato_il
          FROM conferma_strutturale_componente k
          JOIN conferma_strutturale c ON c.conferma_id = k.conferma_id
         WHERE k.thread_id = sqlc.arg(thread_id) AND c.formato = 3 AND k.componente_id = ANY (sqlc.arg(componenti)::uuid[])
         ORDER BY k.componente_id, c.confermato_il DESC, c.numero DESC) u
  JOIN componente w ON w.componente_id = u.componente_id
 ORDER BY u.componente_id;

-- name: ChiaviWorking :many
-- La chiave di collisione con la working di ogni codice dato, calcolata dal database con upper() (la stessa
-- dell'indice ux_componente_thread_codice, M18:280; mai strings.ToUpper del Go: EM §3.10), con il componente della RFQ
-- che la occupa (archiviati compresi), se c'e'.
SELECT x.codice::text AS codice, upper(x.codice)::text AS chiave, c.componente_id AS occupante_id, c.codice AS occupante_codice,
       c.archiviato_il AS occupante_archiviato_il
  FROM unnest(sqlc.arg(codici)::text[]) AS x(codice)
  LEFT JOIN componente c ON c.thread_id = sqlc.arg(thread_id) AND upper(c.codice) = upper(x.codice)
 ORDER BY 1;

-- name: ListRighePerShaChiave :many
-- Le righe legacy (componente_proposta) dei nodi dati per (sha256, chiave), sul portatore e su ogni copia della RFQ:
-- R4 e R5 ci mettono i segni (EM §2.1, §7.4), mai il codice.
SELECT cp.proposta_id, cp.allegato_id, cp.sha256, cp.chiave, cp.stato, cp.componente_id, cp.codice, cp.deciso_da, cp.deciso_il, cp.nota,
       cp.evidenza
  FROM componente_proposta cp
 WHERE cp.thread_id = sqlc.arg(thread_id)
   AND (cp.sha256, cp.chiave) IN (SELECT unnest(sqlc.arg(shas)::text[]), unnest(sqlc.arg(chiavi)::text[]))
 ORDER BY cp.sha256, cp.chiave, cp.allegato_id;

-- name: ListArchiPerShaChiave :many
-- Gli archi legacy (relazione_proposta) che entrano nei nodi dati per (sha256, chiave del figlio), da ogni padre nello
-- stesso file: gli archi da chiudere di una non inclusione (EM §7.4, §7.7).
SELECT r.allegato_id, r.padre_chiave, r.figlio_chiave, r.qta, r.stato, f.sha256
  FROM relazione_proposta r
  JOIN componente_proposta f ON f.thread_id = r.thread_id AND f.allegato_id = r.allegato_id AND f.chiave = r.figlio_chiave
 WHERE r.thread_id = sqlc.arg(thread_id)
   AND (f.sha256, f.chiave) IN (SELECT unnest(sqlc.arg(shas)::text[]), unnest(sqlc.arg(chiavi)::text[]))
 ORDER BY f.sha256, r.figlio_chiave, r.allegato_id, r.padre_chiave;

-- name: ListProdottiConLoStessoStep :many
-- I prodotti finiti della RFQ che hanno lo sha256 dato come STEP autorizzato, come fonte di una bozza aperta o come fonte
-- di una conferma (prodotti_con_lo_stesso_step[] dei non inclusi, firmato: EM §7.3, §7.4).
SELECT DISTINCT x.prodotto_id, x.come FROM (
    SELECT c.componente_id AS prodotto_id, 'step_autorizzato'::text AS come
      FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id
     WHERE c.thread_id = sqlc.arg(thread_id) AND c.tipo = 'finito' AND c.archiviato_il IS NULL AND d.sha256 = sqlc.arg(sha256)
    UNION ALL
    SELECT b.prodotto_id, 'fonte_bozza'::text FROM distinta_bozza b
     WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta' AND b.fonte_sha256 = sqlc.arg(sha256)
    UNION ALL
    SELECT k.prodotto_id, 'fonte_conferma'::text FROM conferma_strutturale k
     WHERE k.thread_id = sqlc.arg(thread_id) AND k.fonte_sha256 = sqlc.arg(sha256)
) x
 ORDER BY x.prodotto_id, x.come;

-- name: ListNonInclusiPerRif :many
-- Il ripristino di un nodo non incluso (R4 «ripristino di un nodo non incluso nella conferma n», R5 riapertura; EM
-- §7.6): per ogni (sha256, chiave) del rif delle caselle reincluse, la riga della conferma piu' recente del prodotto.
-- Mai per gruppo o allegato: dopo un riallineamento la casella ha un altro gruppo, e dopo un cambio di copia un altro
-- allegato. Un nodo che sta fra i nodi della conferma di base (base_conferma_id = versione_bom_base.conferma_id, NULL
-- senza conferme) non e' un ripristino: l'ha gia' reincluso una conferma successiva, che ne ha riaperto e deciso le
-- righe (PM23 §6, terzo giro su O-01, OT-06; EM §7.6 punto 3).
SELECT DISTINCT ON (n.sha256, n.chiave) n.sha256, n.chiave, n.conferma_id, n.decisione_id, n.allegato_id, n.gruppo, n.righe_chiuse,
       n.archi_chiusi, c.firma_anteprima, c.numero
  FROM conferma_strutturale_non_incluso n JOIN conferma_strutturale c ON c.conferma_id = n.conferma_id
 WHERE n.thread_id = sqlc.arg(thread_id) AND c.prodotto_id = sqlc.arg(prodotto_id)
   AND (n.sha256, n.chiave) IN (SELECT unnest(sqlc.arg(shas)::text[]), unnest(sqlc.arg(chiavi)::text[]))
   AND NOT EXISTS (SELECT 1
         FROM conferma_strutturale b
         CROSS JOIN LATERAL jsonb_path_query(b.snapshot, '$.nodi[*].caselle[*].rif') AS r
        WHERE b.conferma_id = sqlc.narg(base_conferma_id)::uuid AND r #>> '{}' = 'nodo:' || n.sha256 || ':' || n.chiave)
 ORDER BY n.sha256, n.chiave, c.numero DESC;

-- ------------------------------------------------------------------ R6, R7

-- name: GetConfermaPerChiave :one
-- La conferma registrata con una chiave d'intento della RFQ (R6, anche oltre la conservazione degli esiti).
SELECT conferma_id, prodotto_id, numero, formato, bozza_id, bozza_versione, impronta_snapshot, firma_anteprima, importazione_id,
       confermato_da, confermato_il
  FROM conferma_strutturale
 WHERE chiave_idempotenza = sqlc.arg(chiave_idempotenza) AND thread_id = sqlc.arg(thread_id);

-- name: GetAssociazioneEventoPerChiave :many
-- Gli eventi del gesto associa_step (S7: «assegna» della pagina nuova, origine ponte, senza riga di file_rfq_associazione)
-- scritti con una chiave d'intento della RFQ (R6). Direttiva 35, blocco C-1c (PD35 §3.5): ristretta al gesto, perche' con la
-- stessa chiave scrivono eventi anche R5 (preassociazione, sostituisci, sgancia con la riga della AR) e gli altri gesti
-- delle proposte; quelli non sono associa_step.
SELECT evento_id, proposta_id, documento_id, azione, componente_prima, componente_dopo, conferma_id, motivo, origine, autore_id, quando
  FROM associazione_evento
 WHERE chiave_idempotenza = sqlc.arg(chiave_idempotenza) AND thread_id = sqlc.arg(thread_id)
   AND azione = 'assegna' AND origine = 'ponte' AND associazione_id IS NULL
 ORDER BY evento_id;

-- name: ListNonInclusiConferma :many
-- I nodi non inclusi da una conferma della RFQ (R7 e l'elenco della pagina; PM23 §6).
SELECT n.decisione_id, n.sha256, n.chiave, n.allegato_id, n.gruppo, n.origine, n.azione_id, n.motivo_codice, n.motivo, n.segnale,
       n.righe_chiuse, n.archi_chiusi, c.firma_anteprima, c.numero
  FROM conferma_strutturale_non_incluso n JOIN conferma_strutturale c ON c.conferma_id = n.conferma_id
 WHERE n.conferma_id = sqlc.arg(conferma_id) AND n.thread_id = sqlc.arg(thread_id)
 ORDER BY n.origine, n.decisione_id, n.sha256, n.chiave;

-- name: ListAttestazioniProdotto :many
-- Le attestazioni del fascicolo di un prodotto della RFQ, dalla prima (R7).
SELECT attestazione_id, numero, precedente_id, conferma_strutturale_id, firma_piano, requisiti, attestato_da, attestato_il
  FROM attestazione_fascicolo
 WHERE thread_id = sqlc.arg(thread_id) AND prodotto_id = sqlc.arg(prodotto_id)
 ORDER BY numero;

-- ------------------------------------------------------------------ il ciclo dei file (corsia file, DDL della persistenza)

-- name: GetFileRfq :one
-- Lo stato di un'occorrenza nella RFQ; nessuna riga = attivo, versione 0.
SELECT thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno, estensione, sha256,
       bytes, origine, ricevuto_il, caricato_da, creato_da, creato_il, aggiornato_da, aggiornato_il
  FROM file_rfq
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id);

-- name: ListFileRfqPerThread :many
-- Lo stato di ogni file (natura file o elemento Outlook) della RFQ (S1; PM23 §6): attivo con versione 0 senza riga.
SELECT a.allegato_id, a.contenitore_id, a.nome_file, a.sha256, COALESCE(f.stato, 'attivo')::text AS stato,
       COALESCE(f.versione, 0)::int AS versione
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN file_rfq f ON f.thread_id = m.thread_id AND f.allegato_id = a.allegato_id
 WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND a.natura IN ('file', 'elemento_outlook')
 ORDER BY m.data_evento, a.contenitore_id NULLS FIRST, a.indice, a.allegato_id;

-- name: InsertFileRfq :one
-- La riga di stato al primo gesto, in altri_file o scartato, alla versione 1, con la fotografia letta dall'allegato
-- della RFQ (il trigger la ricontrolla): il chiamante non la passa. Nessuna riga se l'allegato non e' della RFQ.
INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
                      estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
SELECT m.thread_id, a.allegato_id, sqlc.arg(stato)::text, 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file,
       a.path_interno, a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, sqlc.arg(utente_id), sqlc.arg(utente_id)
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE a.allegato_id = sqlc.arg(allegato_id) AND m.thread_id = sqlc.arg(thread_id)::uuid
RETURNING thread_id, allegato_id, stato, versione, creato_il;

-- name: UpdateFileRfqStato :execrows
-- Una transizione dello stato alla versione attesa (confronta e scambia: 0 righe = la versione e' cambiata, 409
-- FILE_VERSIONE). Le transizioni ammesse le controlla il trigger (P4X09).
UPDATE file_rfq SET stato = sqlc.arg(stato), versione = versione + 1, aggiornato_da = sqlc.arg(utente_id)
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id) AND versione = sqlc.arg(versione_attesa);

-- name: InsertFileRfqEvento :one
-- Un evento della vita di un file, in sola aggiunta; data e transazione le scrive il database.
INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, esito,
                             altro_allegato_id, origine, autore_id, chiave_idempotenza)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(azione), sqlc.narg(stato_prima), sqlc.narg(stato_dopo), sqlc.narg(motivo),
        sqlc.narg(fotografia), sqlc.narg(dipendenze), sqlc.narg(esito), sqlc.narg(altro_allegato_id), sqlc.arg(origine),
        sqlc.narg(autore_id), sqlc.narg(chiave_idempotenza))
RETURNING evento_id, quando;

-- name: ListFileRfqEventi :many
-- La storia di un'occorrenza della RFQ, in ordine.
SELECT evento_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, esito, altro_allegato_id, origine, autore_id,
       chiave_idempotenza, quando
  FROM file_rfq_evento
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id)
 ORDER BY evento_id;

-- name: GetFileEventoPerChiave :many
-- Gli eventi scritti con una chiave d'intento della RFQ (R6 dei gesti dei file: un archivio puo' toccare piu'
-- occorrenze con la stessa chiave).
SELECT evento_id, allegato_id, azione, stato_prima, stato_dopo, esito, origine, autore_id, quando
  FROM file_rfq_evento
 WHERE chiave_idempotenza = sqlc.arg(chiave_idempotenza) AND thread_id = sqlc.arg(thread_id)
 ORDER BY evento_id;

-- name: InsertRimozione :one
-- La rimozione dallo staging di un'occorrenza scartata, in attesa: nasce dall'evento di scarto (trigger), una per
-- occorrenza.
INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id, sha256)
VALUES (sqlc.arg(thread_id), sqlc.arg(allegato_id), sqlc.arg(evento_id), sqlc.narg(sha256))
RETURNING rimozione_id, creato_il;

-- name: UpdateRimozione :execrows
-- Un passo della rimozione dallo stato atteso (confronta e scambia). percorso_trovato si scrive una volta (il trigger
-- lo tiene fermo); da un errore non ritentabile si esce solo con eseguita / bonificato (trigger).
UPDATE file_rfq_rimozione
   SET stato = sqlc.arg(stato), motivo = sqlc.arg(motivo), ritentabile = sqlc.arg(ritentabile), errore = sqlc.narg(errore),
       tentativi = sqlc.arg(tentativi), prossimo_tentativo_il = sqlc.narg(prossimo_tentativo_il),
       percorso_trovato = COALESCE(percorso_trovato, sqlc.narg(percorso_trovato))
 WHERE rimozione_id = sqlc.arg(rimozione_id) AND stato = sqlc.arg(stato_atteso);

-- name: ListRimozioniDaRiprendere :many
-- Le rimozioni da riprendere (il custode), bloccate e saltando quelle gia' in mano a un altro.
SELECT rimozione_id, thread_id, allegato_id, evento_id, sha256, percorso_trovato, stato, motivo, ritentabile, tentativi
  FROM file_rfq_rimozione
 WHERE stato IN ('in_attesa', 'errore') AND ritentabile
   AND (prossimo_tentativo_il IS NULL OR prossimo_tentativo_il <= now())
 ORDER BY prossimo_tentativo_il NULLS FIRST, rimozione_id
 LIMIT sqlc.arg(limite)
   FOR UPDATE SKIP LOCKED;

-- name: ListRimozioniNonRitentabili :many
-- Gli errori non ritentabili, per il controllo giornaliero del custode: si chiudono eseguita / bonificato solo se il file
-- di percorso_trovato non esiste piu' sotto la radice dello staging (EM §10.4).
SELECT rimozione_id, thread_id, allegato_id, percorso_trovato
  FROM file_rfq_rimozione
 WHERE stato = 'errore' AND NOT ritentabile
 ORDER BY rimozione_id
 LIMIT sqlc.arg(limite)
   FOR UPDATE SKIP LOCKED;

-- name: InsertConservazione :one
-- Una copia nuova di un «Altro file» nella cartella ALTRO, in copia (U2 §3.3, fase A). Il nome lo sceglie e lo riserva il
-- chiamante (percorso_relativo = ALTRO\<nome>, sotto BloccaCartella); il database lo controlla e non lo calcola.
-- percorso_relativo NULL vale il nome provvisorio ALTRO\<allegato_id>.<conservazione_id>.<ext>, unico per riga: e'
-- l'aggancio minimo per il deposito di prova dell'ondata B finche' la corsia file non sceglie il nome in fase A
-- (Prenota). token NULL = un token nuovo. eredita_da: la riga trattenuta di cui si riusa il file e la prova (il trigger
-- la cede nella stessa istruzione, U2 §3.8).
WITH n AS (SELECT nextval(pg_get_serial_sequence('file_rfq_conservazione', 'conservazione_id')) AS id)
INSERT INTO file_rfq_conservazione (conservazione_id, thread_id, allegato_id, sha256, bytes, percorso_relativo, radice_id, cartella_thread,
                                    token, eredita_da, creato_dal_cockpit, chiave_idempotenza)
OVERRIDING SYSTEM VALUE
SELECT n.id, sqlc.arg(thread_id)::uuid, sqlc.arg(allegato_id)::uuid, sqlc.arg(sha256)::char(64), sqlc.narg(bytes)::bigint,
       COALESCE(sqlc.narg(percorso_relativo)::text,
                'ALTRO\' || sqlc.arg(allegato_id)::uuid::text || '.' || n.id::text || '.'
                || COALESCE((SELECT CASE WHEN lower(a.estensione) ~ '^[a-z0-9]{1,10}$' THEN lower(a.estensione) END
                               FROM allegato a WHERE a.allegato_id = sqlc.arg(allegato_id)::uuid), 'bin')),
       sqlc.arg(radice_id)::uuid, sqlc.narg(cartella_thread)::text, COALESCE(sqlc.narg(token)::uuid, gen_random_uuid()),
       sqlc.narg(eredita_da)::bigint, sqlc.arg(creato_dal_cockpit)::bool, sqlc.narg(chiave_idempotenza)::uuid
  FROM n
RETURNING conservazione_id, percorso_relativo, iniziata_il, token;

-- name: UpdateConservazione :execrows
-- Un passo della copia dallo stato atteso (confronta e scambia; in_copia → in_copia e' il battito). Le transizioni le
-- controlla il trigger (P4X09); verificato_il e concluso_il li scrive lui.
UPDATE file_rfq_conservazione
   SET stato = sqlc.arg(stato), causa = sqlc.narg(causa), errore = sqlc.narg(errore), tentativi = sqlc.arg(tentativi),
       prossimo_tentativo_il = sqlc.narg(prossimo_tentativo_il), sorgente_byte = COALESCE(sqlc.narg(sorgente_byte), sorgente_byte),
       bytes = COALESCE(sqlc.narg(bytes), bytes)
 WHERE conservazione_id = sqlc.arg(conservazione_id) AND stato = sqlc.arg(stato_atteso);

-- name: GetConservazioneCopiata :one
-- La copia verificata di un'occorrenza in Altri file (per aprirla e per il pin dello staging).
SELECT conservazione_id, sha256, bytes, percorso_relativo, radice_id, sorgente_byte, verificato_il
  FROM file_rfq_conservazione
 WHERE thread_id = sqlc.arg(thread_id) AND allegato_id = sqlc.arg(allegato_id) AND stato = 'copiata';

-- name: ListConservazioniDaRiprendere :many
-- Le copie da riprendere (il custode): da rilasciare, o in copia con il battito fermo da piu' di 15 minuti (da quel
-- momento vanno a da_rilasciare con la causa tempo_scaduto, mai a errore_copia: il loro file puo' esistere).
SELECT conservazione_id, thread_id, allegato_id, sha256, percorso_relativo, radice_id, stato, causa, tentativi
  FROM file_rfq_conservazione
 WHERE (stato = 'da_rilasciare' AND (prossimo_tentativo_il IS NULL OR prossimo_tentativo_il <= now()))
    OR (stato = 'in_copia' AND aggiornato_il < now() - interval '15 minutes')
 ORDER BY conservazione_id
 LIMIT sqlc.arg(limite)
   FOR UPDATE SKIP LOCKED;

-- name: ListDipendenzeBozzaPerFile :many
-- La dipendenza D-6 (PM23 §6; EM §10.3): le occorrenze citate dalle bozze aperte della RFQ, in una passata. Nel gesto
-- (S2, S3) gira sotto BloccaThread per ogni occorrenza del gesto; nel dettaglio di S1 senza lucchetto. tipo «registro»
-- e' D-6b (si mostra), gli altri tipi bloccano. Preassociazioni e svincoli su due rami (direttiva 35, blocco C-2): nel
-- formato 3 citano l'occorrenza ({allegato_id, chiave_bozza}, scollega_documento {allegato_id, componente_id}); nel
-- formato 2 la PB cita la proposta di documento, risolta con il join di prima (il formato 2 si rilegge come era, e non ha
-- svincoli).
WITH aperte AS (
    SELECT b.bozza_id, b.prodotto_id, b.formato, b.fonte_allegato_id, v.snapshot, v.importazione_id
      FROM distinta_bozza b
      JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
     WHERE b.thread_id = sqlc.arg(thread_id) AND b.stato = 'aperta'),
citati AS (
    SELECT a.bozza_id, a.prodotto_id, a.fonte_allegato_id::text AS allegato, 'fonte'::text AS tipo
      FROM aperte a WHERE a.fonte_allegato_id IS NOT NULL
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, c #>> '{}', 'casella'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.nodi[*].caselle[*].allegato_id') AS c
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, c #>> '{}', 'non_includere'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.decisioni[*] ? (@.tipo == "non_includere").casella.allegato_id') AS c
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, q #>> '{}', 'preassociazione'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.documenti_preassociati[*].allegato_id') AS q
     WHERE a.formato = 3
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, p.allegato_id::text, 'preassociazione'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.documenti_preassociati[*].proposta_id') AS q
      JOIN documento_proposta p ON p.proposta_id = (q #>> '{}')::uuid
     WHERE a.formato = 2
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, q #>> '{}', 'svincolo'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.decisioni[*] ? (@.tipo == "scollega_documento").allegato_id') AS q
     WHERE a.formato = 3
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, split_part(c #>> '{}', '/', 1), 'prova_identita'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.decisioni[*] ? (@.tipo == "identita").chiave_lettura') AS c
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, al.allegato_id::text, 'casella_formato_2'
      FROM aperte a
      CROSS JOIN LATERAL jsonb_path_query(a.snapshot, '$.nodi[*].caselle[*].rif') AS r
      JOIN allegato al ON al.sha256 = substr(r #>> '{}', 6, 64)
      JOIN messaggio m ON m.messaggio_id = al.messaggio_id AND m.thread_id = sqlc.arg(thread_id)
     WHERE a.snapshot ->> 'formato' = '2'
    UNION ALL
    SELECT a.bozza_id, a.prodotto_id, u::text, 'registro'
      FROM aperte a
      JOIN distinta_importazione i ON i.importazione_id = a.importazione_id
      CROSS JOIN LATERAL unnest(i.allegati_citati) AS u)
SELECT DISTINCT bozza_id, prodotto_id, allegato::uuid AS allegato_id, tipo FROM citati
 ORDER BY bozza_id, allegato_id, tipo;

-- name: BloccaContenuto :exec
-- Il lucchetto per contenuto (EM §10.6): chi cancella un contenuto dallo staging e chi lo adotta lo prendono nella
-- transazione che scrive path_staging, dopo BloccaThread.
SELECT pg_advisory_xact_lock(hashtextextended('contenuto:' || lower(sqlc.arg(sha256)::text), 0));
