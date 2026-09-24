-- La schermata del Fascicolo (Blocco 8, B8.7; piano §7): le letture che la disegnano e i pochi gesti che
-- non avevano ancora una query (la deroga del fabbisogno tolta, i dati di un componente, il contenitore
-- dei caricamenti interni). Nessuno schema nuovo: tutto legge e scrive tabelle della 0001–0020.
--
-- Le letture sono separate e semplici, e le mette insieme il Go: una riga per file con proposta e
-- documento in un solo SELECT vorrebbe LEFT JOIN LATERAL, e sqlc non ne capisce la nullabilita'.

-- name: ListAllegatiFascicolo :many
-- I file della RFQ, radici e voci di zip, con il messaggio da cui vengono: le righe del pannello
-- Documenti. Gli inline e i collegamenti non sono file da smistare.
SELECT a.allegato_id, a.messaggio_id, a.contenitore_id, a.indice, a.nome_file, a.estensione, a.origine,
       a.bytes, a.sha256, a.stato, a.path_staging, a.errore, a.ricevuto_il, a.caricato_da,
       m.canale, m.direzione, m.data_evento, m.mittente_nome, m.mittente_indirizzo, m.oggetto
  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = $1 AND a.natura = 'file'
 ORDER BY m.data_evento, a.messaggio_id, a.contenitore_id NULLS FIRST, a.indice;

-- name: ListProposteDocumentoThread :many
-- Le proposte dei file della RFQ, in tutti gli stati: una gia' decisa dice che cosa e' diventato il file.
SELECT p.* FROM documento_proposta p
  JOIN allegato a ON a.allegato_id = p.allegato_id
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE m.thread_id = $1;

-- name: ListProvenienzeThread :many
-- Da quali file e' nato ogni documento: anche i file che erano lo stesso contenuto di uno gia' confermato.
SELECT dp.allegato_id, dp.documento_id FROM documento_provenienza dp
  JOIN documento d ON d.documento_id = dp.documento_id
 WHERE d.thread_id = $1 AND dp.allegato_id IS NOT NULL;

-- name: ListComponenteProposteThread :many
-- I nodi proposti dagli STEP della RFQ, in tutti gli stati, con il file: la schermata li mette sotto il
-- componente del padre proposto, o nel blocco del loro file.
SELECT sqlc.embed(cp), a.nome_file FROM componente_proposta cp JOIN allegato a ON a.allegato_id = cp.allegato_id
 WHERE cp.thread_id = $1
 ORDER BY a.ricevuto_il, cp.allegato_id, cp.chiave;

-- name: ListRelazioneProposteThread :many
SELECT sqlc.embed(r), a.nome_file FROM relazione_proposta r JOIN allegato a ON a.allegato_id = r.allegato_id
 WHERE r.thread_id = $1
 ORDER BY a.ricevuto_il, r.allegato_id, r.padre_chiave, r.figlio_chiave;

-- name: ListDerogheThread :many
SELECT * FROM deroga_fabbisogno WHERE thread_id = $1 ORDER BY componente_id, tipo;

-- name: ListDerogheStrutturaThread :many
SELECT * FROM deroga_struttura WHERE thread_id = $1 ORDER BY concessa_il DESC;

-- name: DeleteDeroga :execrows
-- Dopo il congelamento la rifiuta il database (D26, R2.8): chi la chiama lo dice prima.
DELETE FROM deroga_fabbisogno WHERE deroga_id = sqlc.arg(deroga_id) AND thread_id = sqlc.arg(thread_id);

-- name: SetDatiComponente :execrows
-- Tipo, revisione e descrizione di un componente: il codice ha il suo gesto (A1.4), perche' tocca i
-- documenti e il NAS; questi no.
UPDATE componente SET tipo = sqlc.arg(tipo), rev = sqlc.narg(rev), descrizione = sqlc.narg(descrizione),
       confermato_da = sqlc.arg(confermato_da)
 WHERE componente_id = sqlc.arg(componente_id);

-- name: ContaAnalisiInCorso :one
-- Le analisi ancora da fare per i file della RFQ. Il job e' per contenuto (A15): lo stesso file in due
-- RFQ e' un job solo, e si conta in tutte e due.
SELECT count(DISTINCT j.job_id) FROM job j
  JOIN allegato a ON a.sha256 = j.payload ->> 'sha256'
  JOIN messaggio m ON m.messaggio_id = a.messaggio_id
 WHERE j.tipo = 'analizza_allegato' AND j.stato IN ('pronto', 'in_corso') AND m.thread_id = $1;

-- name: CollegaConversazioneNota :exec
UPDATE conversazione SET thread_id = sqlc.arg(thread_id), collegata_da = 'operatore'
 WHERE conversazione_id = sqlc.arg(conversazione_id) AND thread_id IS NULL;

-- name: UpsertNotaInterna :one
-- Il contenitore dei caricamenti interni di una RFQ: un messaggio del canale `nota`, uno per RFQ, gia'
-- agganciato. I file caricati a mano ne sono gli allegati, e da li' fanno la strada di tutti gli altri
-- (staging, analisi, proposta, decisione). La seconda chiamata ritrova la stessa riga.
INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, mittente_nome, oggetto, corpo_testo,
                       thread_id, aggancio, agganciato_da, agganciato_il, registrato_da, interno, controparte_tipo)
VALUES ('nota', sqlc.arg(chiave), sqlc.arg(conversazione_id), 'entrata', now(), sqlc.arg(mittente_nome), sqlc.arg(oggetto),
        sqlc.arg(corpo), sqlc.arg(thread_id), 'operatore', sqlc.arg(utente), now(), sqlc.arg(utente), true, 'interno')
ON CONFLICT (canale, chiave_esterna) DO UPDATE SET canale = EXCLUDED.canale
RETURNING *;

-- name: ProssimoIndiceAllegato :one
-- Il posto del file nuovo nel contenitore. Chi lo chiama ha bloccato il messaggio (BloccaMessaggio):
-- due caricamenti nella stessa nota si mettono in fila, e il secondo vede l'indice del primo.
SELECT COALESCE(max(indice), 0)::int + 1 FROM allegato WHERE messaggio_id = $1 AND contenitore_id IS NULL;

-- name: DocumentoInterno :one
-- Il documento viene da un caricamento interno (un allegato di origine manuale): sostituisce un altro
-- documento solo con un motivo (B8.7).
SELECT EXISTS (SELECT 1 FROM documento_provenienza dp JOIN allegato a ON a.allegato_id = dp.allegato_id
                WHERE dp.documento_id = $1 AND a.origine = 'manuale') AS interno;

-- name: SetNotaDocumento :exec
-- La nota si mostra e basta: resta libera anche con la BOM congelata (D26, R2.8).
UPDATE documento SET nota = sqlc.narg(nota) WHERE documento_id = sqlc.arg(documento_id);
