-- Fatti dell'analisi riusabili (migrazione 0003, voce 1.12).
--
-- I fatti dipendono dal contenuto del file, dalla versione dell'analizzatore e dalla configurazione:
-- a parità di tutte e tre il risultato è lo stesso, quindi si calcola una volta e si riusa. La chiave
-- primaria è esattamente quella terna: non c'è modo di scriverci sopra per sbaglio un risultato
-- ottenuto con un dizionario diverso.

-- name: UpsertAnalisiFatti :one
INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti)
VALUES (sqlc.arg(sha256), sqlc.arg(versione_analizzatore), sqlc.arg(hash_configurazione), sqlc.arg(fatti))
ON CONFLICT (sha256, versione_analizzatore, hash_configurazione) DO UPDATE SET
    fatti = EXCLUDED.fatti, calcolato_il = now()
RETURNING *;

-- name: GetAnalisiFatti :one
SELECT * FROM analisi_fatti
WHERE sha256 = sqlc.arg(sha256) AND versione_analizzatore = sqlc.arg(versione_analizzatore)
  AND hash_configurazione = sqlc.arg(hash_configurazione);

-- name: ListProposteAperteStessoFile :many
-- Tutte le proposte ancora APERTE degli allegati con quello stesso contenuto, in qualunque messaggio e
-- di qualunque cliente. È l'elenco a cui va distribuito il risultato di una sola analisi: lo stesso PDF
-- allegato a tre RFQ non va analizzato tre volte, ma va proposto tre volte, una per RFQ.
-- Le proposte già decise non compaiono: una decisione dell'operatore non si tocca.
SELECT p.*, a.nome_file, a.estensione, a.bytes, a.messaggio_id, a.contenitore_id
FROM documento_proposta p
JOIN allegato a ON a.allegato_id = p.allegato_id
WHERE a.sha256 = sqlc.arg(sha256) AND p.stato = 'aperta';

-- ------------------------------------------------------------------ A4 (B8.A4a): l'analizzatore corrente
-- name: ImpostaAnalizzatoreCorrente :exec
-- Il server la scrive a ogni avvio da cfg.Analisi. La data cambia solo se cambia la chiave.
INSERT INTO analizzatore_corrente (unico, versione_analizzatore, hash_configurazione)
VALUES (true, sqlc.arg(versione_analizzatore), sqlc.arg(hash_configurazione))
ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = EXCLUDED.versione_analizzatore,
    hash_configurazione = EXCLUDED.hash_configurazione, impostato_il = now()
WHERE (analizzatore_corrente.versione_analizzatore, analizzatore_corrente.hash_configurazione)
      IS DISTINCT FROM (EXCLUDED.versione_analizzatore, EXCLUDED.hash_configurazione);

-- name: GetAnalizzatoreCorrente :one
SELECT * FROM analizzatore_corrente;
