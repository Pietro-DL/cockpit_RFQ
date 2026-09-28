-- La panoramica delle Richieste: una card per RFQ, e dentro una scheda per ogni codice della richiesta
-- (identificativo_thread) con il disegno da guardare. Tre letture per pagina, qualunque sia il numero di
-- RFQ: le RFQ, i loro prodotti, i disegni candidati all'anteprima. Le mette insieme il Go, come per la
-- schermata del Fascicolo: una sola SELECT vorrebbe LEFT JOIN LATERAL, e sqlc non ne capisce la
-- nullabilita' (schermata.sql).
--
-- Niente migration: la testata c'e' gia' in v_cruscotto, i prodotti sono gli identificativi, e
-- l'anteprima e' un dato derivato dai documenti e dalle proposte che esistono gia'.

-- name: ListRichiestePanoramica :many
-- Le RFQ secondo i filtri della barra. Un filtro NULL (o false) non filtra. `modello` e' gia' il
-- modello ILIKE ('%testo%', con % _ \ protetti dal Go) e cerca in cliente, oggetto, riferimento del
-- cliente, buyer e codici della richiesta. L'ordine «priorita» e' quello di sempre (peso del cliente,
-- poi scadenza), con le aperte prima delle chiuse: il punteggio dell'addendum 2 non esiste ancora.
-- `totale` e' il numero di RFQ che rispondono ai filtri, prima di LIMIT: dice se ce ne sono altre.
SELECT v.thread_id, t.cliente_id, v.cliente, c.ragione_sociale, c.peso AS peso_cliente,
       v.buyer, v.oggetto, t.riferimento_cliente, v.stato_thread, v.nome_fase, v.gg_in_fase, v.sla_gg,
       v.semaforo, v.data_inizio, v.data_scadenza, v.ultimo_aggiornamento,
       v.n_bloccanti, v.n_da_smistare, v.identificativi,
       count(*) OVER () AS totale
FROM v_cruscotto v
JOIN thread_offerta t ON t.thread_id = v.thread_id
JOIN cliente c        ON c.cliente_id = t.cliente_id
WHERE (sqlc.narg(stato)::stato_thread IS NULL OR v.stato_thread = sqlc.narg(stato)::stato_thread)
  AND (sqlc.narg(cliente_id)::uuid IS NULL OR t.cliente_id = sqlc.narg(cliente_id)::uuid)
  AND (sqlc.narg(fase)::fase IS NULL OR v.nome_fase = sqlc.narg(fase)::fase)
  AND (NOT sqlc.arg(solo_bloccanti)::boolean OR COALESCE(v.n_bloccanti, 0) > 0)
  AND (NOT sqlc.arg(solo_da_smistare)::boolean OR v.n_da_smistare > 0)
  AND (NOT sqlc.arg(solo_sla_critico)::boolean OR v.semaforo = 'rosso')
  AND (sqlc.narg(modello)::text IS NULL
       OR v.cliente ILIKE sqlc.narg(modello)::text
       OR c.ragione_sociale ILIKE sqlc.narg(modello)::text
       OR v.oggetto ILIKE sqlc.narg(modello)::text
       OR t.riferimento_cliente ILIKE sqlc.narg(modello)::text
       OR v.buyer ILIKE sqlc.narg(modello)::text
       OR EXISTS (SELECT 1 FROM identificativo_thread i
                   WHERE i.thread_id = v.thread_id AND i.codice ILIKE sqlc.narg(modello)::text))
ORDER BY
  CASE WHEN sqlc.arg(ordine)::text = 'priorita' THEN v.stato_thread = 'CHIUSA' END,
  CASE WHEN sqlc.arg(ordine)::text = 'priorita' THEN c.peso END DESC,
  CASE WHEN sqlc.arg(ordine)::text IN ('priorita', 'scadenza') THEN v.data_scadenza END ASC NULLS LAST,
  COALESCE(v.ultimo_aggiornamento, v.data_inizio) DESC,
  v.thread_id
LIMIT sqlc.arg(limite)::int OFFSET sqlc.arg(salta)::int;

-- name: ListProdottiPanoramica :many
-- I prodotti delle RFQ date, uno per identificativo, anche quando il componente non c'e' ancora: il
-- codice appartiene alla richiesta dal triage. Dal componente, se c'e' e non e' archiviato, vengono
-- revisione e descrizione (il codice di un componente e' unico nella RFQ: al piu' una riga).
SELECT i.thread_id, i.codice, c.componente_id, c.rev, c.descrizione
FROM identificativo_thread i
LEFT JOIN componente c ON c.thread_id = i.thread_id AND upper(c.codice) = upper(i.codice) AND c.archiviato_il IS NULL
WHERE i.thread_id = ANY(sqlc.arg(thread_ids)::uuid[])
ORDER BY i.thread_id, i.creato_il, i.codice;

-- name: ListAnteprimePanoramica :many
-- I disegni che possono fare da anteprima nelle RFQ date: i 2D correnti (non sostituiti, di un
-- componente non archiviato) con un allegato fra le provenienze, e le proposte 2D ancora aperte. Il Go
-- sceglie per ogni prodotto: prima il documento, poi la proposta; fra piu' candidati il piu' recente.
SELECT d.thread_id, d.componente_id, d.codice, a.allegato_id, a.nome_file, 'documento'::text AS fonte, dp.ricevuto_il AS quando
  FROM documento d
  JOIN documento_provenienza dp ON dp.documento_id = d.documento_id
  JOIN allegato a ON a.allegato_id = dp.allegato_id
  LEFT JOIN componente ca ON ca.componente_id = d.componente_id
 WHERE d.thread_id = ANY(sqlc.arg(thread_ids)::uuid[])
   AND d.tipo = 'disegno_2d' AND d.sostituito_da IS NULL AND ca.archiviato_il IS NULL
UNION ALL
SELECT p.thread_id, p.componente_id, p.codice, a.allegato_id, a.nome_file, 'proposta'::text AS fonte, p.creato_il AS quando
  FROM documento_proposta p
  JOIN allegato a ON a.allegato_id = p.allegato_id
 WHERE p.thread_id = ANY(sqlc.arg(thread_ids)::uuid[])
   AND p.stato = 'aperta' AND p.tipo_proposto = 'disegno_2d';
