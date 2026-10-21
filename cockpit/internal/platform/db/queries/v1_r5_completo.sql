-- V1, direttiva 39, blocco «R5 completo» della corsia persistenza (EM §9.5 passo 5 «ricodifiche», H.5 «Mantieni»; direttiva
-- 35 D-ASS-01 [U]), file proprio per INC-G21: le query che R4 e R5 formato 3 aggiungono per la ricodifica di un componente
-- registrato. Le chiama solo core/rfq/distintabozza (riepilogo3_lettura.go, conferma3_scritture.go).

-- name: ListDocumentiDeiComponentiR4 :many
-- I documenti confermati dei componenti dati, in ogni stato del NAS (anche quelli sostituiti): una ricodifica dovrebbe
-- cambiarne il codice e la cartella, e per i documenti gia' confermati resta il blocco RICODIFICA_FILE_NAS (D-ASS-01 [U]).
-- Solo SELECT, senza lucchetti: R4 la legge nella sua transazione di sola lettura e R5 nella connessione S.
SELECT d.documento_id, d.componente_id::uuid AS componente_id, d.stato_nas::text AS stato_nas
  FROM documento d
 WHERE d.thread_id = sqlc.arg(thread_id) AND d.componente_id = ANY (sqlc.arg(componenti)::uuid[])
 ORDER BY d.componente_id, d.documento_id;

-- name: ListAttiveDelComponenteR5 :many
-- Le AR attive di un componente con l'allegato (nome e sha256) e la proposta di documento dell'allegato con la lettura
-- conservata (dettagli.codice_letto, rev; senza proposta l'UUID nullo, che sqlc non vede nel LEFT JOIN LATERAL): le righe
-- della traccia della ricodifica in correzione_documento.
SELECT r.associazione_id, r.allegato_id, a.nome_file::text AS nome_file, COALESCE(a.sha256, '')::text AS sha256,
       COALESCE(p.proposta_id, '00000000-0000-0000-0000-000000000000'::uuid) AS proposta_id,
       COALESCE(p.dettagli ->> 'codice_letto', '')::text AS codice_letto, p.rev AS rev_letta
  FROM file_rfq_associazione r
  JOIN allegato a ON a.allegato_id = r.allegato_id
  LEFT JOIN LATERAL (SELECT x.proposta_id, x.dettagli, x.rev FROM documento_proposta x
                      WHERE x.allegato_id = r.allegato_id AND x.thread_id = r.thread_id
                      ORDER BY x.proposta_id LIMIT 1) p ON true
 WHERE r.thread_id = sqlc.arg(thread_id) AND r.componente_id = sqlc.arg(componente_id) AND r.stato = 'attiva'
 ORDER BY r.associazione_id;

-- name: InsertCorrezioneRicodificaR5 :exec
-- La traccia della ricodifica di un componente registrato su una sua AR attiva (EM §9.5 passo 5; H.5 «Mantieni»): la AR non
-- cambia riga, la copia si allinea (AllineaCodiceCopia), e correzione_documento registra codice prima e dopo con il motivo
-- della decisione ricodifica (ck_correzione_motivo: al piu' 500 caratteri), la chiave della conferma e chi conferma.
INSERT INTO correzione_documento (thread_id, proposta_id, componente_id, allegato_id, nome_file, sha256, codice_letto, rev_letta,
                                  codice_prima, codice_dopo, motivo, origine, chiave_idempotenza, autore_id, associazione_id)
VALUES (sqlc.arg(thread_id), sqlc.narg(proposta_id), sqlc.arg(componente_id), sqlc.arg(allegato_id), sqlc.arg(nome_file),
        sqlc.arg(sha256), sqlc.narg(codice_letto), sqlc.narg(rev_letta), sqlc.arg(codice_prima), sqlc.arg(codice_dopo),
        sqlc.arg(motivo), 'ricodifica_bom', sqlc.arg(chiave_idempotenza), sqlc.arg(autore_id), sqlc.arg(associazione_id));

-- name: RicodificaComponenteR5 :execrows
-- La ricodifica di un componente registrato nella working (EM §9.5 passo 5), solo se il codice e' ancora quello della
-- decisione (codice_prima): altrimenti 0 righe e R5 riprova. Chi chiama ha differito fk_proposta_componente (la copia si
-- allinea dopo, AllineaCodiceCopia) e ha verificato che il componente non ha documenti confermati (RICODIFICA_FILE_NAS).
UPDATE componente SET codice = sqlc.arg(codice_dopo), confermato_da = sqlc.arg(confermato_da)
 WHERE componente_id = sqlc.arg(componente_id) AND thread_id = sqlc.arg(thread_id) AND codice = sqlc.arg(codice_prima);

-- name: RipristinaComponenteR5 :execrows
-- Il riuso di un componente archiviato (EM §5.2 W3, «ripristino di un archiviato»): R5 lo rimette nella working, stesso
-- componente_id e stessa storia, solo se e' ancora archiviato; gli archi li scrive R5 dalla bozza.
UPDATE componente SET archiviato_il = NULL, archiviato_da = NULL, motivo_archiviazione = NULL, confermato_da = sqlc.arg(confermato_da)
 WHERE componente_id = sqlc.arg(componente_id) AND thread_id = sqlc.arg(thread_id) AND archiviato_il IS NOT NULL;
