-- Le query delle preassociazioni del formato 3 per allegato_id (direttiva 35, blocco C-2; EM §3.6.2, §3.7): un file
-- proprio della corsia persistenza (INC-G21). Le chiama core/rfq/distintabozza (preassociazioni3.go, la verifica di R2).

-- name: ListAllegatiPreassociabili :many
-- Le occorrenze date, una per riga anche se non esistono: se sono della RFQ (allegato -> messaggio -> thread), se sono
-- attive per il ciclo dei file (file_rfq_attivo e non scartate col vecchio «Scarta il file»: il punto 4 di trg_fra_nasce)
-- e se si preassociano (natura file o elemento_outlook, ne' contenitori ne' archivi: la regola del punto 1 di
-- trg_fra_nasce, scritta uguale). D-2 (c) [U], direttiva 39 §5: uno STEP si preassocia come un PDF, tranne lo STEP legato
-- come sorgente strutturale del finito (legato_come_sorgente, la stessa funzione del database). Natura ed estensione
-- servono al messaggio del rifiuto.
SELECT x.allegato_id::uuid AS allegato_id,
       COALESCE(m.thread_id = sqlc.arg(thread_id)::uuid, false)::boolean AS della_rfq,
       (file_rfq_attivo(sqlc.arg(thread_id)::uuid, x.allegato_id)
        AND NOT file_rfq_scartato_legacy(sqlc.arg(thread_id)::uuid, x.allegato_id))::boolean AS attivo,
       (a.natura IS NOT NULL AND a.natura IN ('file', 'elemento_outlook')
        AND lower(COALESCE(a.estensione, '')) NOT IN ('zip', '7z', 'rar')
        AND NOT EXISTS (SELECT 1 FROM allegato v WHERE v.contenitore_id = x.allegato_id))::boolean AS preassociabile,
       allegato_sorgente_del_finito(sqlc.arg(thread_id)::uuid, x.allegato_id)::boolean AS legato_come_sorgente,
       COALESCE(a.natura::text, '')::text AS natura,
       COALESCE(a.estensione, '')::text AS estensione
  FROM unnest(sqlc.arg(allegati)::uuid[]) AS x(allegato_id)
  LEFT JOIN allegato a ON a.allegato_id = x.allegato_id
  LEFT JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 ORDER BY 1;

-- name: ListAssociazioniAttiveDegliAllegati :many
-- Le associazioni registrate attive (AR) degli allegati dati nella RFQ, una per allegato (ux_fra_attiva_allegato): lo
-- svincolo nuovo o cambiato di R2 cita una AR attiva (EM §3.6.2). Sola lettura: le scrive solo il servizio unico.
SELECT r.associazione_id, r.allegato_id, r.componente_id
  FROM file_rfq_associazione r
 WHERE r.thread_id = sqlc.arg(thread_id) AND r.stato = 'attiva'
   AND r.allegato_id = ANY (sqlc.arg(allegati)::uuid[])
 ORDER BY r.allegato_id;
