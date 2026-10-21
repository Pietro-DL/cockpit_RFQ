-- V1, direttiva 39 (§1, §4, §5, §6; EM §9.4, H.7, App. I), corsia persistenza, file proprio per INC-G21: le letture del
-- riepilogo formato 3 (R4) e del Piano del servizio unico delle AR (associazioni.Piano) che non c'erano gia'. Le chiamano
-- core/rfq/associazioni (piano.go, LeggiFattiPiano) e core/rfq/distintabozza (riepilogo3*.go). Solo SELECT, senza
-- lucchetti: R4 le legge nella sua transazione REPEATABLE READ READ ONLY; R5 (strategia A) le rileggera' nella connessione
-- S in sola lettura, dopo il lucchetto della RFQ.

-- name: ListOccorrenzePiano :many
-- Le occorrenze date, una per riga anche se non esistono, con quello che il Piano guarda (EM H.7; MAT §4.2): se sono
-- della RFQ (allegato -> messaggio -> thread), attive (il punto 4 di trg_fra_nasce), preassociabili (il punto 1, scritto
-- come ListAllegatiPreassociabili), legate come sorgente strutturale del finito (la stessa funzione del database), lo sha256
-- e la lettura del codice della proposta di documento: la copia del componente (codice) non e' la lettura quando la
-- proposta e' gia' associata, e la lettura conservata sta in dettagli.codice_letto (DIR35-DP, INC-G4; la stessa regola di
-- v1_ingressi.sql). Nessuna proposta = nessun codice letto (D-1 A [T-39]).
SELECT x.allegato_id::uuid AS allegato_id,
       COALESCE(m.thread_id = sqlc.arg(thread_id)::uuid, false)::boolean AS della_rfq,
       (file_rfq_attivo(sqlc.arg(thread_id)::uuid, x.allegato_id)
        AND NOT file_rfq_scartato_legacy(sqlc.arg(thread_id)::uuid, x.allegato_id))::boolean AS attivo,
       (a.natura IS NOT NULL AND a.natura IN ('file', 'elemento_outlook')
        AND lower(COALESCE(a.estensione, '')) NOT IN ('zip', '7z', 'rar')
        AND NOT EXISTS (SELECT 1 FROM allegato v WHERE v.contenitore_id = x.allegato_id))::boolean AS preassociabile,
       allegato_sorgente_del_finito(sqlc.arg(thread_id)::uuid, x.allegato_id)::boolean AS legato_come_sorgente,
       COALESCE(a.sha256, '')::text AS sha256,
       COALESCE(a.nome_file, '')::text AS nome_file,
       (p.proposta_id IS NOT NULL)::boolean AS con_proposta,
       COALESCE(p.tipo_proposto::text, '')::text AS tipo_proposto,
       COALESCE(CASE WHEN p.componente_id IS NULL THEN p.codice
                     WHEN p.dettagli ? 'codice_letto' THEN p.dettagli ->> 'codice_letto'
                     ELSE p.codice END, '')::text AS codice_letto,
       COALESCE(p.rev, '')::text AS rev_letta
  FROM unnest(sqlc.arg(allegati)::uuid[]) AS x(allegato_id)
  LEFT JOIN allegato a ON a.allegato_id = x.allegato_id
  LEFT JOIN messaggio m ON m.messaggio_id = a.messaggio_id
  LEFT JOIN documento_proposta p ON p.allegato_id = x.allegato_id
 ORDER BY 1;

-- name: ListDocumentiDegliAllegati :many
-- I documenti confermati della RFQ con lo stesso contenuto degli allegati dati (documento e' UNIQUE (thread_id, sha256)),
-- anche senza componente: D-AR06 A [T-39] (documentazione generale gia' confermata: blocco, mai una conversione
-- silenziosa in AR) e D-AR07 A [T-39] (lo stesso contenuto su un altro componente: blocco).
SELECT a.allegato_id, d.documento_id, d.componente_id, d.tipo::text AS tipo, d.stato_nas::text AS stato_nas,
       (d.sostituito_da IS NOT NULL)::boolean AS sostituito
  FROM allegato a
  JOIN documento d ON d.thread_id = sqlc.arg(thread_id) AND d.sha256 = a.sha256
 WHERE a.allegato_id = ANY (sqlc.arg(allegati)::uuid[]) AND a.sha256 IS NOT NULL
 ORDER BY a.allegato_id, d.documento_id;

-- name: ListDocumentiScrittiDeiComponenti :many
-- I documenti dei componenti dati gia' scritti sul NAS (stato_nas = scritto): una ricodifica cambierebbe la loro cartella
-- (RICODIFICA_FILE_NAS, C3 §4.4 n. 16; D-ASS-01 [U]: per i documenti gia' confermati resta il blocco).
SELECT d.componente_id::uuid AS componente_id, d.documento_id, d.nome_file, d.path_relativo
  FROM documento d
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.componente_id = ANY (sqlc.arg(componenti)::uuid[])
   AND d.stato_nas = 'scritto'
 ORDER BY d.componente_id, d.documento_id;

-- name: GetStrutturaDellaSorgente :one
-- La gerarchia dello STEP sorgente della bozza, per la condizione (d) di prima-istanza-1 (U2 §5.4; D-SHOCK-S1 B [U]): se
-- c'e' l'analisi corrente del contenuto (analizzatore_corrente) e il motivo per cui la sua struttura non e' completa (la
-- stessa funzione della vista v_step_prodotto e del gate; vuoto = completa), e la deroga di struttura del prodotto per quel
-- documento che vale ancora (stessa analisi derogata, oppure nessuna analisi ancora: il commento di deroga_struttura).
SELECT (af.sha256 IS NOT NULL)::boolean AS analizzata,
       COALESCE(CASE WHEN af.sha256 IS NULL THEN 'nessuna analisi corrente dello STEP'
                     ELSE struttura_motivo_parziale(af.fatti -> 'struttura') END, '')::text AS motivo_parziale,
       (dg.deroga_struttura_id IS NOT NULL)::boolean AS con_deroga,
       COALESCE(dg.motivo, '')::text AS motivo_deroga
  FROM (SELECT 1 AS uno) x
  LEFT JOIN analizzatore_corrente ac ON true
  LEFT JOIN analisi_fatti af ON af.sha256 = sqlc.arg(sha256)::text
        AND af.versione_analizzatore = ac.versione_analizzatore AND af.hash_configurazione = ac.hash_configurazione
  LEFT JOIN LATERAL (
        SELECT d.deroga_struttura_id, d.motivo FROM deroga_struttura d
         WHERE d.thread_id = sqlc.arg(thread_id)::uuid AND d.componente_id = sqlc.arg(componente_id)::uuid
           AND d.step_documento_id = sqlc.arg(documento_id)::uuid
           AND ((d.versione_analizzatore IS NULL AND af.sha256 IS NULL)
                OR (d.versione_analizzatore = af.versione_analizzatore AND d.hash_configurazione = af.hash_configurazione
                    AND d.analisi_calcolata_il = af.calcolato_il))
         ORDER BY d.concessa_il DESC, d.deroga_struttura_id LIMIT 1) dg ON true;

-- name: ListStepDelProdotto :many
-- I documenti cad_3d del prodotto finito, con estensione e nome del file: R4 ne ricava il legame storico univoco di
-- D-AR04 (iii) con confermabom.LegameStoricoUnivoco (lo STEP .stp/.step corrente, uno solo, per un prodotto senza conferme
-- di S8: la regola di /dati.fonte.legame_storico), che non soddisfa il gate della sorgente (direttiva 39 §2 [U + T-39]).
-- Il filtro sullo STEP (estensione o nome del file) lo fa la funzione, non la query, perche' la regola sia una sola.
SELECT d.documento_id, d.sha256::text AS sha256, d.nome_file, d.estensione, (d.sostituito_da IS NOT NULL)::boolean AS sostituito
  FROM documento d
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.componente_id = sqlc.arg(prodotto_id)::uuid AND d.tipo = 'cad_3d'
 ORDER BY d.confermato_il, d.documento_id;
