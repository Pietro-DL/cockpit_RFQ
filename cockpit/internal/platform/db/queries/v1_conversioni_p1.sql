-- V1, direttiva 39 §4 (D-AR14 A [T-39]; rilievo importante della revisione del 20/10), corsia persistenza: le
-- conversioni delle pre-assegnazioni legacy (P1) che una bozza abbandonata non ha consumato (conversionep1.go).

-- name: ListConversioniP1DaRiprendere :many
-- Gli allegati della RFQ il cui ultimo evento del giornale delle AR e' la conversione di una pre-assegnazione legacy
-- (sgancia, origine conversione_legacy, senza AR) e che oggi non hanno ne' una AR attiva ne' la copia del componente,
-- quando dopo quella conversione il prodotto dato non ha conferme formato 3: la bozza che l'aveva presa in carico e'
-- stata abbandonata (o cancellata dalla conservazione) senza R5, e l'assegnazione dell'ingegnere vive solo nel giornale
-- (componente_prima). Una conferma formato 3 del prodotto dopo la conversione l'ha decisa (AR, o PB tolta): non si
-- riprende. L'ultimo evento si cerca per allegato, anche sugli eventi scritti prima della 0023 (allegato dalla
-- proposta). Sola lettura.
WITH ev AS (
    SELECT e.evento_id, e.origine, e.azione, e.associazione_id, e.componente_prima, e.quando,
           COALESCE(e.allegato_id, dp.allegato_id) AS allegato_id
      FROM associazione_evento e
      LEFT JOIN documento_proposta dp ON dp.proposta_id = e.proposta_id
     WHERE e.thread_id = sqlc.arg(thread_id)::uuid
), ultimo AS (
    SELECT DISTINCT ON (ev.allegato_id) ev.*
      FROM ev
     WHERE ev.allegato_id IS NOT NULL
     ORDER BY ev.allegato_id, ev.evento_id DESC
)
SELECT u.allegato_id::uuid AS allegato_id, u.componente_prima::uuid AS componente_id, u.evento_id::bigint AS evento_id
  FROM ultimo u
 WHERE u.origine = 'conversione_legacy' AND u.azione = 'sgancia' AND u.associazione_id IS NULL
   AND u.componente_prima IS NOT NULL
   AND NOT EXISTS (SELECT 1 FROM file_rfq_associazione r
                    WHERE r.thread_id = sqlc.arg(thread_id)::uuid AND r.allegato_id = u.allegato_id AND r.stato = 'attiva')
   AND NOT EXISTS (SELECT 1 FROM documento_proposta p WHERE p.allegato_id = u.allegato_id AND p.componente_id IS NOT NULL)
   AND NOT EXISTS (SELECT 1 FROM conferma_strutturale cs
                    WHERE cs.thread_id = sqlc.arg(thread_id)::uuid AND cs.prodotto_id = sqlc.arg(prodotto_id)::uuid
                      AND cs.formato = 3 AND cs.confermato_il > u.quando)
 ORDER BY 1;
