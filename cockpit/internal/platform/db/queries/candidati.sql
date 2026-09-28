-- I codici candidati di una RFQ (Blocco 8, B8.6; piano §6): le evidenze che la RFQ ha gia' prodotto.
--
-- Qui non si classifica niente. v_codici_candidati_thread (0018) prende le righe da candidato_codice,
-- documento_proposta e componente_proposta, gia' classificate da chi le ha scritte; il Go le unisce per
-- upper(codice) (fascicolo.Unisci). Componenti e identificativi non sono evidenze: dicono se un codice e'
-- gia' deciso, e li leggono le loro query.

-- name: ListCodiciCandidatiThread :many
-- Una riga per evidenza, con il tipo che la proposta del documento da' al file per le righe che vengono
-- dal suo nome: il nome di un file tecnico e' un'evidenza forte, quello di un'offerta no (piano §6.1).
-- I testi passano da COALESCE: nella vista ruolo e' NULL fuori da candidato_codice.
SELECT COALESCE(v.codice, '')::text AS codice, COALESCE(v.rev, '')::text AS rev,
       COALESCE(v.sorgente, '')::text AS sorgente, COALESCE(v.origine, '')::text AS origine,
       COALESCE(v.famiglia, '')::text AS famiglia, COALESCE(v.punteggio, 0)::int AS punteggio,
       COALESCE(v.evidenza, '')::text AS evidenza, COALESCE(v.ruolo, '')::text AS ruolo,
       v.messaggio_id, v.allegato_id, COALESCE(p.tipo_proposto::text, '')::text AS tipo_file
  FROM v_codici_candidati_thread v
  LEFT JOIN documento_proposta p ON p.allegato_id = v.allegato_id AND v.sorgente IN ('proposta_documento', 'nome_file')
 WHERE v.thread_id = $1
 ORDER BY upper(v.codice), v.sorgente, v.evidenza, v.messaggio_id;

-- name: ListProposteNodoAperte :many
-- I nodi proposti dagli STEP ancora aperti e con un codice, con il file da cui vengono: un codice che ne
-- ha uno si porta alla sua proposta, non diventa un componente parallelo (B8.6).
SELECT cp.proposta_id, cp.allegato_id, cp.chiave, cp.codice::text AS codice, COALESCE(cp.rev, '')::text AS rev,
       cp.nome_grezzo, cp.tipo_proposto, COALESCE(cp.nota, '')::text AS nota, a.nome_file
  FROM componente_proposta cp JOIN allegato a ON a.allegato_id = cp.allegato_id
 WHERE cp.thread_id = $1 AND cp.stato = 'aperta' AND nullif(btrim(cp.codice), '') IS NOT NULL
 ORDER BY upper(cp.codice), a.ricevuto_il, cp.allegato_id, cp.chiave;
