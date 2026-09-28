-- Le note tecniche sui disegni (0021, Fascicolo v3): un punto di una pagina di un file, con il testo.
-- Le coordinate stanno in numeric(7,6) nella tabella; qui si leggono e si scrivono come float8, perche'
-- al Go serve un numero e non un pgtype.Numeric.

-- name: ListAnnotazioniThread :many
-- Tutte le note della RFQ, con lo sha256 del file su cui sono state scritte (la schermata le mostra per
-- contenuto), chi le ha scritte e il codice del componente che si stava guardando ("" = nessuno).
SELECT n.annotazione_id, n.componente_id, n.allegato_id, a.sha256, n.pagina,
       n.x_norm::float8 AS x, n.y_norm::float8 AS y, n.testo,
       n.creata_da, u.sigla AS autore_sigla, u.nome AS autore_nome, n.creata_il, n.modificata_il,
       coalesce(c.codice, '')::text AS codice_componente
  FROM annotazione_pdf n
  JOIN allegato a ON a.allegato_id = n.allegato_id
  JOIN utente u ON u.utente_id = n.creata_da
  LEFT JOIN componente c ON c.componente_id = n.componente_id
 WHERE n.thread_id = $1
 ORDER BY n.creata_il, n.annotazione_id;

-- name: AllegatoDelThread :one
-- Il file e' della RFQ: un allegato di un messaggio agganciato a lei.
SELECT EXISTS (SELECT 1 FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
                WHERE a.allegato_id = sqlc.arg(allegato_id) AND m.thread_id = sqlc.arg(thread_id)::uuid)::boolean AS del_thread;

-- name: InsertAnnotazione :one
INSERT INTO annotazione_pdf (thread_id, componente_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da)
VALUES (sqlc.arg(thread_id), sqlc.arg(componente_id), sqlc.arg(allegato_id), sqlc.arg(pagina),
        round(sqlc.arg(x)::float8::numeric, 6), round(sqlc.arg(y)::float8::numeric, 6), sqlc.arg(testo), sqlc.arg(creata_da))
RETURNING annotazione_id;

-- name: ModificaAnnotazione :execrows
-- Il testo lo cambia solo chi l'ha scritta.
UPDATE annotazione_pdf SET testo = sqlc.arg(testo), modificata_da = sqlc.arg(utente_id)::uuid, modificata_il = now()
 WHERE annotazione_id = sqlc.arg(annotazione_id) AND thread_id = sqlc.arg(thread_id) AND creata_da = sqlc.arg(utente_id);

-- name: DeleteAnnotazione :execrows
-- La toglie solo chi l'ha scritta.
DELETE FROM annotazione_pdf
 WHERE annotazione_id = sqlc.arg(annotazione_id) AND thread_id = sqlc.arg(thread_id) AND creata_da = sqlc.arg(utente_id);
