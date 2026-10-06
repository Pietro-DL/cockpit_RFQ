-- LA FOTOGRAFIA DEL MOTORE A (giro 5, A1c; piano A, 6.5): le sei query nuove del caricatore in sola lettura
-- (core/fotorfq/caricatore). Non e' una migrazione. Tutte sono solo SELECT, senza lucchetti (niente FOR UPDATE,
-- FOR SHARE, FOR KEY SHARE, nessuna funzione che scrive o blocca: struttura_motivo_parziale e' LANGUAGE sql
-- IMMUTABLE), quindi valgono dentro una transazione READ ONLY; non leggono la tabella dei suggerimenti
-- dell'agente, le credenziali dei worker ne' le sessioni, e nessuna colonna password_hash o regole.

-- name: ListFattiDelThread :many
-- I fatti di tutti i contenuti della RFQ alla terna data: gli allegati dei messaggi del thread e i documenti del
-- thread (T-04: anche un documento confermato ha i suoi fatti, per la validita' del 2D). La terna la passa chi
-- chiama, letta da analizzatore_corrente NELLA STESSA transazione: mai «la riga piu' recente». motivo_parziale
-- viene dalla definizione unica della 0020 (struttura_motivo_parziale), solo per i fatti con la struttura STEP
-- (con_struttura): vuoto = completa. Per i fatti senza struttura (con_struttura falso) il motivo non c'e': sqlc
-- non sa che un CASE senza ELSE puo' dare NULL e lo leggerebbe come testo, quindi la mancanza la dice
-- con_struttura, e chi legge mette NULL. Nessun lucchetto: vale dentro una transazione READ ONLY.
SELECT af.sha256::text              AS sha256,
       af.versione_analizzatore,
       af.hash_configurazione::text AS hash_configurazione,
       af.fatti,
       af.calcolato_il,
       (af.fatti ? 'struttura')::boolean AS con_struttura,
       CASE WHEN af.fatti ? 'struttura'
            THEN COALESCE(struttura_motivo_parziale(af.fatti -> 'struttura'), '') ELSE '' END::text AS motivo_parziale
  FROM analisi_fatti af
 WHERE af.versione_analizzatore = sqlc.arg(versione_analizzatore)
   AND af.hash_configurazione   = sqlc.arg(hash_configurazione)
   AND af.sha256 IN (SELECT a.sha256 FROM allegato a
                       JOIN messaggio m ON m.messaggio_id = a.messaggio_id
                      WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND a.sha256 IS NOT NULL
                     UNION
                     SELECT d.sha256 FROM documento d
                      WHERE d.thread_id = sqlc.arg(thread_id)::uuid)
 ORDER BY af.sha256;

-- name: ListFattiPerSha :many
-- Come ListFattiDelThread, per un elenco di contenuti: gli allegati dei messaggi senza RFQ dei casi del banco.
SELECT af.sha256::text AS sha256, af.versione_analizzatore, af.hash_configurazione::text AS hash_configurazione,
       af.fatti, af.calcolato_il, (af.fatti ? 'struttura')::boolean AS con_struttura,
       CASE WHEN af.fatti ? 'struttura'
            THEN COALESCE(struttura_motivo_parziale(af.fatti -> 'struttura'), '') ELSE '' END::text AS motivo_parziale
  FROM analisi_fatti af
 WHERE af.sha256 = ANY(sqlc.arg(sha256)::bpchar[])
   AND af.versione_analizzatore = sqlc.arg(versione_analizzatore)
   AND af.hash_configurazione   = sqlc.arg(hash_configurazione)
 ORDER BY af.sha256;

-- name: ListClientiDellaFotografia :many
-- Identificativo e ragione sociale dei clienti della fotografia: la ragione sociale e' solo un controllo contro il
-- file delle regole (mai una chiave), e cliente.regole non si legge (le regole del motore A vengono dai file).
SELECT cliente_id, ragione_sociale FROM cliente
 WHERE cliente_id = ANY(sqlc.arg(clienti)::uuid[])
 ORDER BY cliente_id;

-- name: ListSigleUtenti :many
-- Chi ha deciso, nel rapporto del banco: identificativo e sigla. Mai password_hash.
SELECT utente_id, sigla FROM utente ORDER BY sigla, utente_id;

-- name: ListCandidatiCodiceThread :many
-- I codici della mail del vecchio motore, senza quelli dell'agente LLM (origine 'agente', 0008:74): servono al
-- confronto, mai al motore A.
SELECT c.messaggio_id, c.codice, c.ruolo, c.rev, c.origine, c.famiglia, c.punteggio
  FROM candidato_codice c JOIN messaggio m ON m.messaggio_id = c.messaggio_id
 WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND c.origine <> 'agente'
 ORDER BY c.messaggio_id, c.codice;

-- name: ListTriageThread :many
-- Il triage DETERMINISTICO dei messaggi della RFQ (fonte_triage, 0001:46): il motore A non usa mai quello
-- dell'agente.
SELECT pt.triage_id, pt.messaggio_id, pt.esito, pt.atto, pt.legame, pt.stato, pt.identificativi,
       pt.creato_il, pt.deciso_il
  FROM proposta_triage pt JOIN messaggio m ON m.messaggio_id = pt.messaggio_id
 WHERE m.thread_id = sqlc.arg(thread_id)::uuid AND pt.fonte = 'deterministico'
 ORDER BY pt.messaggio_id;
