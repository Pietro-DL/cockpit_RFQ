-- Versioni della BOM (Blocco 8, A4, B8.A4a): una baseline congelata e le sue istantanee (addendum
-- A4.6, A4.7). Le versioni le scrive solo core/rfq/fascicolo (congelamento, apertura e abbandono di
-- una revisione). Le regole che contano le tiene il database: una versione nasce bozza, congelata non
-- cambia piu', la catena Vn → V(n-1) non ha salti, la working e' bloccata dopo il congelamento.

-- name: InsertBomVersione :one
INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura,
                          versione_precedente_id, numero_precedente, stato_precedente, motivo, creata_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(numero), sqlc.arg(contesto), sqlc.arg(fase_all_apertura), sqlc.arg(fase_log_id_all_apertura),
        sqlc.narg(versione_precedente_id), sqlc.narg(numero_precedente), sqlc.narg(stato_precedente), sqlc.arg(motivo), sqlc.arg(creata_da))
RETURNING *;

-- name: GetBomVersione :one
SELECT * FROM bom_versione WHERE bom_versione_id = $1;

-- name: GetUltimaVersione :one
-- L'ultima versione della RFQ, bozza o congelata. Per la catena la bozza, se c'e', e' sempre l'ultima.
SELECT * FROM bom_versione WHERE thread_id = $1 ORDER BY numero DESC LIMIT 1;

-- name: GetBozzaAperta :one
SELECT * FROM bom_versione WHERE thread_id = $1 AND stato = 'bozza';

-- name: GetUltimaCongelata :one
SELECT * FROM bom_versione WHERE thread_id = $1 AND stato = 'congelata' ORDER BY numero DESC LIMIT 1;

-- name: ListBomVersioni :many
SELECT * FROM v_bom_versioni WHERE thread_id = $1 ORDER BY numero;

-- name: CongelaBomVersione :execrows
UPDATE bom_versione SET stato = 'congelata', congelata_da = sqlc.arg(congelata_da), congelata_il = now()
WHERE bom_versione_id = sqlc.arg(bom_versione_id) AND stato = 'bozza';

-- name: EliminaBozza :execrows
DELETE FROM bom_versione WHERE bom_versione_id = $1 AND stato = 'bozza';

-- ------------------------------------------------------------------ le istantanee (A4.6, passo 5)
-- Si scrivono mentre la versione e' bozza, nella transazione che poi la congela. Contano solo i
-- componenti attivi, e di loro: gli archi fra due attivi, i documenti correnti, le deroghe del
-- fabbisogno in vigore. La deroga strutturale entra solo se e' servita, cioe' se lo STEP era letto in
-- parte o non ancora analizzato, ed e' quella valida adesso (v_step_prodotto).

-- name: SnapshotComponenti :execrows
INSERT INTO bom_versione_componente (bom_versione_id, thread_id, componente_id, codice, rev, tipo, descrizione, qta,
                                     esito_fattibilita, note_fattibilita, step_strutturale_id, step_sha256, deroga_struttura_id)
SELECT sqlc.arg(bom_versione_id)::uuid, c.thread_id, c.componente_id, c.codice, c.rev, c.tipo, c.descrizione, c.qta,
       c.esito_fattibilita, c.note_fattibilita, c.step_strutturale_id, d.sha256,
       CASE WHEN sp.esito IN ('presente_parziale', 'presente_non_analizzato') THEN sp.deroga_struttura_id END
  FROM componente c
  LEFT JOIN documento d ON d.documento_id = c.step_strutturale_id
  LEFT JOIN v_step_prodotto sp ON sp.componente_id = c.componente_id
 WHERE c.thread_id = sqlc.arg(thread_id) AND c.archiviato_il IS NULL;

-- name: SnapshotRelazioni :execrows
INSERT INTO bom_versione_relazione (bom_versione_id, padre_id, figlio_id, qta, posizione)
SELECT sqlc.arg(bom_versione_id)::uuid, r.padre_id, r.figlio_id, r.qta, r.posizione
  FROM componente_relazione r
  JOIN componente p ON p.componente_id = r.padre_id AND p.archiviato_il IS NULL
  JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
 WHERE r.thread_id = sqlc.arg(thread_id);

-- name: SnapshotDocumenti :execrows
-- path_al_congelamento e' storico (R1.8): dice dove stava il file, non serve ad aprirlo.
INSERT INTO bom_versione_documento (bom_versione_id, componente_id, documento_id, tipo, rev, sha256, path_al_congelamento)
SELECT sqlc.arg(bom_versione_id)::uuid, d.componente_id, d.documento_id, d.tipo, d.rev, d.sha256, d.path_relativo
  FROM documento d
  JOIN componente c ON c.componente_id = d.componente_id AND c.archiviato_il IS NULL
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.sostituito_da IS NULL;

-- name: SnapshotDeroghe :execrows
INSERT INTO bom_versione_deroga (bom_versione_id, componente_id, deroga_id, tipo, motivo, utente_id, creata_il)
SELECT sqlc.arg(bom_versione_id)::uuid, g.componente_id, g.deroga_id, g.tipo, g.motivo, g.utente_id, g.creata_il
  FROM deroga_fabbisogno g
  JOIN componente c ON c.componente_id = g.componente_id AND c.archiviato_il IS NULL
 WHERE g.thread_id = sqlc.arg(thread_id);

-- ------------------------------------------------------------------ una versione com'era (A4.7)
-- Leggono solo le istantanee, mai le tabelle vive: Vn resta interrogabile esattamente com'era.

-- name: ListBomComponenti :many
SELECT * FROM bom_versione_componente WHERE bom_versione_id = $1 ORDER BY codice;

-- name: ListBomRelazioni :many
SELECT * FROM bom_versione_relazione WHERE bom_versione_id = $1 ORDER BY padre_id, figlio_id;

-- name: ListBomDocumenti :many
SELECT * FROM bom_versione_documento WHERE bom_versione_id = $1 ORDER BY componente_id, tipo, documento_id;

-- name: ListBomDeroghe :many
SELECT * FROM bom_versione_deroga WHERE bom_versione_id = $1 ORDER BY componente_id, tipo;

-- ------------------------------------------------------------------ differenza V(n) ↔ working (A4.7)

-- name: DiffBomWorking :many
-- Le coppie (istantanea, working) di ogni oggetto, con FULL JOIN: prima NULL = aggiunto, dopo NULL =
-- tolto, tutte e due = da confrontare. Il confronto lo fa una funzione pura (fascicolo.Differenze),
-- che si prova da sola. Conta solo quello che e' scritto: una deroga strutturale decaduta per una
-- rianalisi non e' una differenza, una deroga strutturale nuova si'.
SELECT 'componente'::text AS oggetto, coalesce(s.componente_id, w.componente_id)::text AS chiave,
       CASE WHEN s.componente_id IS NOT NULL THEN jsonb_build_object('codice', s.codice, 'rev', s.rev, 'tipo', s.tipo,
            'descrizione', s.descrizione, 'qta', s.qta, 'esito_fattibilita', s.esito_fattibilita,
            'note_fattibilita', s.note_fattibilita, 'step_strutturale_id', s.step_strutturale_id) END::jsonb AS prima,
       CASE WHEN w.componente_id IS NOT NULL THEN jsonb_build_object('codice', w.codice, 'rev', w.rev, 'tipo', w.tipo,
            'descrizione', w.descrizione, 'qta', w.qta, 'esito_fattibilita', w.esito_fattibilita,
            'note_fattibilita', w.note_fattibilita, 'step_strutturale_id', w.step_strutturale_id) END::jsonb AS dopo
  FROM (SELECT * FROM bom_versione_componente x WHERE x.bom_versione_id = sqlc.arg(bom_versione_id)) s
  FULL JOIN (SELECT * FROM componente y WHERE y.thread_id = sqlc.arg(thread_id) AND y.archiviato_il IS NULL) w
         ON w.componente_id = s.componente_id
UNION ALL
SELECT 'arco', coalesce(s.padre_id, w.padre_id)::text || '>' || coalesce(s.figlio_id, w.figlio_id)::text,
       CASE WHEN s.padre_id IS NOT NULL THEN jsonb_build_object('qta', s.qta, 'posizione', s.posizione) END,
       CASE WHEN w.padre_id IS NOT NULL THEN jsonb_build_object('qta', w.qta, 'posizione', w.posizione) END
  FROM (SELECT * FROM bom_versione_relazione x WHERE x.bom_versione_id = sqlc.arg(bom_versione_id)) s
  FULL JOIN (SELECT r.* FROM componente_relazione r
               JOIN componente p ON p.componente_id = r.padre_id AND p.archiviato_il IS NULL
               JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
              WHERE r.thread_id = sqlc.arg(thread_id)) w
         ON w.padre_id = s.padre_id AND w.figlio_id = s.figlio_id
UNION ALL
SELECT 'documento', coalesce(s.documento_id, w.documento_id)::text,
       CASE WHEN s.documento_id IS NOT NULL THEN jsonb_build_object('componente_id', s.componente_id, 'tipo', s.tipo, 'rev', s.rev, 'sha256', s.sha256) END,
       CASE WHEN w.documento_id IS NOT NULL THEN jsonb_build_object('componente_id', w.componente_id, 'tipo', w.tipo, 'rev', w.rev, 'sha256', w.sha256) END
  FROM (SELECT * FROM bom_versione_documento x WHERE x.bom_versione_id = sqlc.arg(bom_versione_id)) s
  FULL JOIN (SELECT d.* FROM documento d
               JOIN componente c ON c.componente_id = d.componente_id AND c.archiviato_il IS NULL
              WHERE d.thread_id = sqlc.arg(thread_id) AND d.sostituito_da IS NULL) w
         ON w.documento_id = s.documento_id
UNION ALL
SELECT 'deroga', coalesce(s.componente_id, w.componente_id)::text || ':' || coalesce(s.tipo, w.tipo)::text,
       CASE WHEN s.componente_id IS NOT NULL THEN jsonb_build_object('motivo', s.motivo) END,
       CASE WHEN w.componente_id IS NOT NULL THEN jsonb_build_object('motivo', w.motivo) END
  FROM (SELECT * FROM bom_versione_deroga x WHERE x.bom_versione_id = sqlc.arg(bom_versione_id)) s
  FULL JOIN (SELECT g.* FROM deroga_fabbisogno g
               JOIN componente c ON c.componente_id = g.componente_id AND c.archiviato_il IS NULL
              WHERE g.thread_id = sqlc.arg(thread_id)) w
         ON w.componente_id = s.componente_id AND w.tipo = s.tipo
UNION ALL
SELECT 'deroga_struttura', ds.deroga_struttura_id::text, NULL::jsonb,
       jsonb_build_object('componente_id', ds.componente_id, 'step_documento_id', ds.step_documento_id, 'motivo', ds.motivo)
  FROM deroga_struttura ds
  JOIN bom_versione v ON v.bom_versione_id = sqlc.arg(bom_versione_id)
 WHERE ds.thread_id = sqlc.arg(thread_id) AND ds.concessa_il > v.congelata_il
ORDER BY 1, 2;

-- ------------------------------------------------------------------ il gate del congelamento (A4.6, passo 3)

-- name: GateCongelamento :one
-- Una riga di conteggi per la RFQ. Lo STEP dei prodotti finiti e la struttura senza cicli si guardano
-- a parte (ListGateStep, ListRelazioniAttive), perche' la regola sta in una funzione pura.
SELECT coalesce((SELECT b.n_bloccanti FROM v_thread_bloccanti b WHERE b.thread_id = sqlc.arg(thread_id)), 0)::int AS n_bloccanti,
       (SELECT count(*) FROM componente_proposta p WHERE p.thread_id = sqlc.arg(thread_id) AND p.stato = 'aperta')::int AS n_proposte_componente,
       (SELECT count(*) FROM relazione_proposta p WHERE p.thread_id = sqlc.arg(thread_id) AND p.stato = 'aperta')::int AS n_proposte_relazione,
       (SELECT count(*) FROM rimozione_proposta p WHERE p.thread_id = sqlc.arg(thread_id) AND p.stato = 'aperta')::int AS n_proposte_rimozione,
       (SELECT count(*) FROM documento d
          JOIN componente c ON c.componente_id = d.componente_id AND c.archiviato_il IS NULL
         WHERE d.thread_id = sqlc.arg(thread_id) AND d.sostituito_da IS NULL AND d.stato_nas = 'errore')::int AS n_documenti_errore,
       (SELECT count(*) FROM nas_anomalia a
          JOIN documento d ON d.documento_id = a.documento_id
          JOIN componente c ON c.componente_id = d.componente_id AND c.archiviato_il IS NULL
         WHERE d.thread_id = sqlc.arg(thread_id) AND d.sostituito_da IS NULL AND a.risolta_il IS NULL)::int AS n_anomalie_nas;

-- name: ListGateStep :many
-- Lo STEP di ogni prodotto finito, con la deroga del fabbisogno cad_3d se c'e' (A4.5).
SELECT sqlc.embed(sp),
       EXISTS (SELECT 1 FROM deroga_fabbisogno g WHERE g.componente_id = sp.componente_id AND g.tipo = 'cad_3d') AS deroga_cad
  FROM v_step_prodotto sp
 WHERE sp.thread_id = $1
 ORDER BY sp.codice;

-- name: ListRelazioniAttive :many
-- Gli archi della working fra componenti attivi.
SELECT r.* FROM componente_relazione r
  JOIN componente p ON p.componente_id = r.padre_id AND p.archiviato_il IS NULL
  JOIN componente f ON f.componente_id = r.figlio_id AND f.archiviato_il IS NULL
 WHERE r.thread_id = $1
 ORDER BY r.padre_id, r.figlio_id;

-- name: VersioniConLoStep :many
-- Le versioni congelate che registrano il documento come STEP strutturale: finche' ce n'e' una, il
-- documento non cambia componente (la FK della baseline, R2.5). Serve al rifiuto che la nomina.
SELECT DISTINCT v.numero FROM bom_versione_componente c
  JOIN bom_versione v ON v.bom_versione_id = c.bom_versione_id
 WHERE c.step_strutturale_id = $1
 ORDER BY v.numero;
