-- Blocco 7B: le richieste d'offerta ai fornitori (0015). Nessuna di queste query decide: scrivere
-- `messaggio.thread_id` o `messaggio.richiesta_fornitore_id` e' una decisione, e passa da
-- `AgganciaRispostaFornitore` che chiama solo il gestore della conferma.

-- ---------------------------------------------------------------- la richiesta

-- name: InsertRichiestaFornitore :one
INSERT INTO richiesta_fornitore (thread_id, fornitore_id, lavorazione, codici, note, creata_da, stato, messaggio_id, inviata_il)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetRichiesta :one
SELECT * FROM richiesta_fornitore WHERE richiesta_id = $1;

-- name: ListRichiesteThread :many
SELECT r.*, f.ragione_sociale AS fornitore, l.descrizione AS lavorazione_descrizione,
       m.oggetto AS oggetto_mail, m.data_evento AS data_mail,
       (SELECT count(*) FROM messaggio x WHERE x.richiesta_fornitore_id = r.richiesta_id AND x.direzione = 'entrata') AS n_risposte
FROM richiesta_fornitore r
JOIN fornitore f ON f.fornitore_id = r.fornitore_id
LEFT JOIN lavorazione l ON l.codice = r.lavorazione
LEFT JOIN messaggio m ON m.messaggio_id = r.messaggio_id
WHERE r.thread_id = $1 ORDER BY r.creata_il;

-- name: ListRichiesteFornitore :many
SELECT r.*, t.oggetto AS oggetto_rfq, t.cartella_relativa, c.cartella_nas AS cliente
FROM richiesta_fornitore r
JOIN thread_offerta t ON t.thread_id = r.thread_id
JOIN cliente c ON c.cliente_id = t.cliente_id
WHERE r.fornitore_id = $1 ORDER BY r.creata_il DESC LIMIT 50;

-- name: SetRichiestaInviata :execrows
-- La nostra mail e' nota: dal marcatore (sync della Posta inviata) o dalla conferma a mano. Non
-- si sovrascrive una richiesta che ha gia' la sua mail.
UPDATE richiesta_fornitore SET messaggio_id = $2, stato = 'inviata', inviata_il = $3
WHERE richiesta_id = $1 AND messaggio_id IS NULL AND stato IN ('bozza', 'inviata');

-- name: SetRichiestaOffertaRicevuta :execrows
-- Solo con la conferma di un atto `offerta` (7C.0): una mail collegata alla richiesta, da sola,
-- non cambia lo stato.
UPDATE richiesta_fornitore SET stato = 'offerta_ricevuta', offerta_ricevuta_il = COALESCE(offerta_ricevuta_il, $2)
WHERE richiesta_id = $1 AND stato IN ('inviata', 'offerta_ricevuta');

-- name: SetRichiestaDeclinata :execrows
-- «Non quotiamo»: la richiesta e' chiusa senza offerta.
UPDATE richiesta_fornitore SET stato = 'declinata', declinata_il = COALESCE(declinata_il, $2)
WHERE richiesta_id = $1 AND stato = 'inviata';

-- name: SetRichiestaStato :execrows
UPDATE richiesta_fornitore SET stato = $2 WHERE richiesta_id = $1 AND thread_id = $3;

-- name: RichiestaPerMarcatore :one
SELECT * FROM richiesta_fornitore WHERE richiesta_id = $1;

-- ---------------------------------------------------------------- i candidati verso una richiesta (7B.2)

-- R0: In-Reply-To / References verso LA NOSTRA mail di richiesta.
-- name: RichiestePerChiaviCitate :many
SELECT r.*, m.chiave_esterna
FROM richiesta_fornitore r
JOIN messaggio m ON m.messaggio_id = r.messaggio_id
WHERE m.chiave_esterna = ANY(sqlc.arg(chiavi)::text[]) AND r.fornitore_id = sqlc.arg(fornitore_id);

-- R1: la conversazione di Outlook della nostra mail di richiesta.
-- name: RichiestePerConversazione :many
SELECT r.*
FROM richiesta_fornitore r
JOIN messaggio m ON m.messaggio_id = r.messaggio_id
WHERE m.conversazione_id = $1 AND r.fornitore_id = $2;

-- R3f: un codice cliente citato che appartiene a una RFQ con una richiesta a QUESTO fornitore,
-- nei codici chiesti o fra gli identificativi della RFQ.
-- name: RichiestePerCodiciFornitore :many
SELECT DISTINCT r.*, i.codice
FROM richiesta_fornitore r
JOIN thread_offerta t ON t.thread_id = r.thread_id
JOIN identificativo_thread i ON i.thread_id = t.thread_id
WHERE r.fornitore_id = $1 AND r.stato IN ('inviata', 'offerta_ricevuta')
  AND (upper(i.codice) = ANY(sqlc.arg(codici)::text[]) OR EXISTS (SELECT 1 FROM unnest(r.codici) c WHERE upper(c) = ANY(sqlc.arg(codici)::text[])))
LIMIT 20;

-- I clienti che hanno richieste aperte a questo fornitore: le SOLE famiglie con cui si cercano codici
-- nella posta di un fornitore (IB8: materiali e norme non sono codici di nessuno).
-- name: ClientiConRichiesteAlFornitore :many
SELECT DISTINCT c.*
FROM richiesta_fornitore r
JOIN thread_offerta t ON t.thread_id = r.thread_id
JOIN cliente c ON c.cliente_id = t.cliente_id
WHERE r.fornitore_id = $1 AND r.stato IN ('inviata', 'offerta_ricevuta');

-- RF_oggetto: una nostra mail a un fornitore che cita un codice di una RFQ aperta (di qualunque
-- cliente): la richiesta mandata a mano.
-- name: ThreadApertePerCodici :many
SELECT DISTINCT t.thread_id, t.oggetto, t.cartella_relativa, c.cartella_nas AS cliente, i.codice
FROM identificativo_thread i
JOIN thread_offerta t ON t.thread_id = i.thread_id
JOIN cliente c ON c.cliente_id = t.cliente_id
WHERE t.stato = 'APERTA' AND t.unito_in IS NULL AND upper(i.codice) = ANY(sqlc.arg(codici)::text[])
LIMIT 20;

-- name: CancellaCandidatiRichiesta :exec
DELETE FROM candidato_richiesta WHERE messaggio_id = $1;

-- name: InsertCandidatoRichiesta :exec
INSERT INTO candidato_richiesta (messaggio_id, richiesta_id, regola, punteggio, evidenza)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (messaggio_id, richiesta_id, regola) DO UPDATE SET punteggio = EXCLUDED.punteggio, evidenza = EXCLUDED.evidenza, creato_il = now();

-- name: ListCandidatiRichiesta :many
SELECT k.*, r.thread_id, r.fornitore_id, r.stato AS richiesta_stato, r.codici, r.lavorazione,
       f.ragione_sociale AS fornitore, t.oggetto AS oggetto_rfq, t.cartella_relativa, c.cartella_nas AS cliente
FROM candidato_richiesta k
JOIN richiesta_fornitore r ON r.richiesta_id = k.richiesta_id
JOIN fornitore f ON f.fornitore_id = r.fornitore_id
JOIN thread_offerta t ON t.thread_id = r.thread_id
JOIN cliente c ON c.cliente_id = t.cliente_id
WHERE k.messaggio_id = $1
ORDER BY k.punteggio DESC, k.creato_il;

-- ---------------------------------------------------------------- le decisioni

-- name: AgganciaRispostaFornitore :exec
UPDATE messaggio SET richiesta_fornitore_id = $2 WHERE messaggio_id = $1;

-- name: SetAttoTriage :execrows
UPDATE proposta_triage SET atto = $2, legame = $3, richiesta_proposta = $4, fornitore_proposto = $5
WHERE messaggio_id = $1 AND fonte = 'deterministico' AND stato = 'proposta';

-- name: InsertBozzaRichiesta :one
INSERT INTO bozza (thread_id, in_risposta_a, tipo, destinatari, oggetto, corpo, documenti, creata_da, richiesta_fornitore_id)
VALUES ($1, NULL, 'nuovo', $2, $3, $4, '{}', $5, $6) RETURNING *;

-- name: SetBozzaInviata :execrows
-- Il marcatore CockpitBozza sulla mail in Posta inviata chiude il cerchio: la bozza e' partita.
UPDATE bozza SET stato = 'inviata', inviata_messaggio_id = $2 WHERE bozza_id = $1 AND inviata_messaggio_id IS NULL;

-- name: ContaRichiesteAperte :one
SELECT count(*) FROM richiesta_fornitore WHERE thread_id = $1 AND stato IN ('bozza', 'inviata');

-- name: RiproponiAllegatiComeOffertaFornitore :execrows
-- Alla conferma «e' la risposta del fornitore» le proposte ancora aperte sui suoi allegati (file, non
-- inline) diventano `offerta_fornitore`: e' il tipo che le porta in OFFERTE FORNITORI della RFQ cliente.
-- Solo le proposte APERTE: una gia' confermata e' una decisione.
UPDATE documento_proposta p SET tipo_proposto = 'offerta_fornitore'
FROM allegato a
WHERE p.allegato_id = a.allegato_id AND a.messaggio_id = $1 AND p.stato = 'aperta' AND a.natura = 'file'
  AND lower(a.estensione) IN ('pdf', 'xls', 'xlsx', 'doc', 'docx');
