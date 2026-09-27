-- Candidati di aggancio e di codice (checkpoint 3R, fase 4.1).
-- Nessuna di queste query scrive `messaggio.thread_id`: quello lo fa solo una decisione dell'operatore.

-- name: InsertCandidatoAggancio :exec
INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (messaggio_id, thread_id, regola) DO UPDATE
    SET punteggio = EXCLUDED.punteggio, evidenza = EXCLUDED.evidenza, thread_stato = EXCLUDED.thread_stato;

-- UpsertCandidatoAggancioPiuForte: lo stesso inserimento, ma una riga gia' scritta si sostituisce solo con
-- una variante PIU' FORTE della stessa regola (Smistamento M1, K25). La chiave e' (messaggio, RFQ, regola):
-- due varianti di R1 verso la stessa RFQ (l'indice che discende e il solo ConversationID) finiscono nella
-- stessa riga, e il giro degli orfani non deve abbassare quella che l'ingest ha gia' scritto. Una riga con un
-- punteggio che nessuna variante attuale della regola ha (`attuali`) e' delle regole di prima e si
-- sostituisce sempre. Il ricalcolo di un messaggio non passa di qui: cancella e riscrive (Salva).
-- name: UpsertCandidatoAggancioPiuForte :exec
INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (messaggio_id, thread_id, regola) DO UPDATE
    SET punteggio = EXCLUDED.punteggio, evidenza = EXCLUDED.evidenza, thread_stato = EXCLUDED.thread_stato
    WHERE EXCLUDED.punteggio > candidato_aggancio.punteggio
       OR candidato_aggancio.punteggio <> ALL(sqlc.arg(attuali)::smallint[]);

-- ListCandidatiAggancio: tutti i candidati di un messaggio, i piu' forti in cima. TUTTI, non il primo:
-- scegliere per l'operatore e' esattamente cio' che questo checkpoint toglie (T16). Lo spareggio e'
-- stabile (Smistamento M1): a parita' di punteggio e di regola la RFQ piu' recente, poi l'id, cosi' due
-- letture della stessa pagina mettono le card nello stesso ordine.
-- name: ListCandidatiAggancio :many
SELECT k.messaggio_id, k.thread_id, k.regola, k.punteggio, k.evidenza, k.thread_stato,
       t.oggetto, t.data_inizio, t.riferimento_cliente, c.ragione_sociale AS cliente, t.cartella_relativa
FROM candidato_aggancio k
JOIN thread_offerta t ON t.thread_id = k.thread_id
JOIN cliente c        ON c.cliente_id = t.cliente_id
WHERE k.messaggio_id = $1
ORDER BY k.punteggio DESC, k.regola, t.data_inizio DESC, k.thread_id;

-- name: EliminaCandidatiAggancio :execrows
DELETE FROM candidato_aggancio WHERE messaggio_id = $1;

-- name: ContaCandidatiAggancio :one
SELECT count(*) FROM candidato_aggancio WHERE messaggio_id = $1;

-- name: InsertCandidatoCodice :exec
INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (messaggio_id, codice) DO UPDATE
    SET ruolo = EXCLUDED.ruolo, rev = EXCLUDED.rev, origine = EXCLUDED.origine,
        famiglia = EXCLUDED.famiglia, punteggio = EXCLUDED.punteggio, evidenza = EXCLUDED.evidenza;

-- name: ListCandidatiCodice :many
SELECT * FROM candidato_codice WHERE messaggio_id = $1
ORDER BY array_position(ARRAY['riferimento_rfq','prodotto','parte','non_classificato']::ruolo_codice[], ruolo),
         punteggio DESC, codice;

-- name: EliminaCandidatiCodice :execrows
DELETE FROM candidato_codice WHERE messaggio_id = $1;

-- ---------------------------------------------------------------- le sorgenti delle regole R0–R5

-- R0: i Message-ID citati da In-Reply-To e References, con cio' che serve a VERIFICARLI (Smistamento
-- M1, A5.16.3): mittente e destinatari del messaggio citato (il mittente nuovo era fra loro?) e le RFQ a
-- cui il citato appartiene. Sono tre strade: il citato e' agganciato (m.thread_id); oppure e' la nostra
-- mail preparata dal Cockpit, e allora vale la RFQ per cui e' stata preparata la bozza (b.thread_id) o
-- quella DI ADESSO della mail a cui la bozza rispondeva (mr.thread_id). La nostra mail inviata resta
-- orfana finche' qualcuno non la aggancia: senza la bozza, la risposta del cliente non trovava niente.
-- name: CitatiPerChiavi :many
SELECT m.messaggio_id, m.chiave_esterna, m.data_evento, m.mittente_indirizzo, m.destinatari,
       m.thread_id, t.stato AS stato_thread,
       b.thread_id AS thread_bozza, tb.stato AS stato_bozza,
       mr.thread_id AS thread_risposta, tr.stato AS stato_risposta
FROM messaggio m
LEFT JOIN thread_offerta t  ON t.thread_id = m.thread_id
LEFT JOIN bozza b           ON b.inviata_messaggio_id = m.messaggio_id
LEFT JOIN thread_offerta tb ON tb.thread_id = b.thread_id
LEFT JOIN messaggio mr      ON mr.messaggio_id = b.in_risposta_a
LEFT JOIN thread_offerta tr ON tr.thread_id = mr.thread_id
-- Il canale sta nella condizione perche' l'indice unico e' (canale, chiave_esterna): senza, ogni
-- messaggio con delle References scorreva tutta la tabella dei messaggi.
WHERE m.canale = 'outlook' AND m.chiave_esterna = ANY(sqlc.arg(chiavi)::text[])
  AND (m.thread_id IS NOT NULL OR b.thread_id IS NOT NULL OR mr.thread_id IS NOT NULL)
ORDER BY m.data_evento, m.messaggio_id;

-- R1: la conversazione, se un operatore l'ha gia' collegata a una RFQ, con il cliente della RFQ.
-- name: ThreadDellaConversazioneConStato :one
SELECT c.thread_id, t.stato, t.cliente_id
FROM conversazione c JOIN thread_offerta t ON t.thread_id = c.thread_id
WHERE c.conversazione_id = $1 AND c.thread_id IS NOT NULL;

-- R1 con il ConversationIndex (Smistamento M1): i messaggi della stessa conversazione che stanno in una
-- RFQ, con il loro indice. Stanno in una RFQ se sono agganciati, oppure se sono la nostra mail preparata
-- dal Cockpit per quella RFQ (ancora orfana: la risposta che segue discende da lei). `escluso` e' il
-- messaggio di cui si calcolano i candidati: il suo indice non conta come antenato di se stesso.
-- La nostra mail del Cockpit porta a DUE RFQ quando le origini non coincidono: quella della bozza
-- (b.thread_id) e quella DI ADESSO della mail a cui rispondeva (mr.thread_id), le stesse due strade di
-- CitatiPerChiavi. R1 le considera tutte e due, come R0 e il box del pannello: seguirne una sola
-- vorrebbe dire scegliere per l'operatore quale delle due origini conta.
-- name: ConversazioneConIndici :many
SELECT t.thread_id, t.stato, t.cliente_id, COALESCE(mo.conversation_index, '')::text AS indice
FROM messaggio m
JOIN thread_offerta t          ON t.thread_id = m.thread_id
LEFT JOIN messaggio_outlook mo ON mo.messaggio_id = m.messaggio_id
WHERE m.conversazione_id = sqlc.arg(conversazione_id) AND m.messaggio_id <> sqlc.arg(escluso)
UNION ALL
SELECT t.thread_id, t.stato, t.cliente_id, COALESCE(mo.conversation_index, '')::text AS indice
FROM messaggio m
JOIN bozza b                   ON b.inviata_messaggio_id = m.messaggio_id
LEFT JOIN messaggio mr         ON mr.messaggio_id = b.in_risposta_a
JOIN thread_offerta t          ON t.thread_id IN (b.thread_id, mr.thread_id)
LEFT JOIN messaggio_outlook mo ON mo.messaggio_id = m.messaggio_id
WHERE m.conversazione_id = sqlc.arg(conversazione_id) AND m.messaggio_id <> sqlc.arg(escluso) AND m.thread_id IS NULL;

-- R2: stesso oggetto (gia' normalizzato: `thread_offerta.oggetto` e' l'oggetto pulito), stesso cliente,
-- dentro la finestra. Senza finestra, «Richiesta offerta» aggancerebbe richieste di quattro anni fa (T3).
-- name: ThreadPerOggettoCliente :many
SELECT t.thread_id, t.stato, t.oggetto, t.data_inizio
FROM thread_offerta t
WHERE t.cliente_id = $1 AND t.unito_in IS NULL
  AND upper(t.oggetto) = upper(sqlc.arg(oggetto)::text)
  AND t.data_inizio >= sqlc.arg(dal)::timestamptz
ORDER BY t.data_inizio DESC
LIMIT 5;

-- R3: un codice gia' identificativo di una RFQ dello stesso cliente. Solo dello stesso cliente: due
-- clienti diversi usano gli stessi numeri e non vuol dire niente (T4). Il buyer della RFQ distingue le
-- due varianti di R3 (Smistamento M1): stesso buyer sopra, buyer diverso o non noto sotto.
-- name: ThreadPerCodiciCliente :many
SELECT DISTINCT t.thread_id, t.stato, t.data_inizio, i.codice, t.buyer_id
FROM identificativo_thread i
JOIN thread_offerta t ON t.thread_id = i.thread_id
WHERE t.cliente_id = $1 AND t.unito_in IS NULL
  AND upper(i.codice) = ANY(sqlc.arg(codici)::text[])
  AND t.data_inizio >= sqlc.arg(dal)::timestamptz
ORDER BY t.data_inizio DESC
LIMIT 10;

-- R4: il riferimento con cui il cliente chiama la richiesta.
-- name: ThreadPerRiferimentoCliente :many
SELECT t.thread_id, t.stato, t.data_inizio, t.riferimento_cliente
FROM thread_offerta t
WHERE t.cliente_id = $1 AND t.unito_in IS NULL
  AND upper(t.riferimento_cliente) = upper(sqlc.arg(riferimento)::text)
ORDER BY t.data_inizio DESC
LIMIT 5;

-- R5: lo stesso buyer, di recente. Da sola non decide niente e non basta mai: mette in fila i candidati
-- quando le regole forti non hanno parlato.
-- name: ThreadRecentiBuyer :many
SELECT t.thread_id, t.stato, t.oggetto, t.data_inizio
FROM thread_offerta t
WHERE t.buyer_id = $1 AND t.unito_in IS NULL AND t.stato = 'APERTA'
  AND t.data_inizio >= sqlc.arg(dal)::timestamptz
ORDER BY t.data_inizio DESC
LIMIT 5;

-- name: SetRiferimentoCliente :exec
UPDATE thread_offerta SET riferimento_cliente = sqlc.narg(riferimento) WHERE thread_id = $1;

-- ListOrfaniConversazione: gli altri messaggi orfani della stessa conversazione. Prima venivano
-- AGGANCIATI in blocco; adesso si leggono per proporli, uno per uno, con il loro candidato (T21).
-- name: ListOrfaniConversazione :many
SELECT messaggio_id FROM messaggio
WHERE conversazione_id = $1 AND thread_id IS NULL AND messaggio_id <> $2
ORDER BY data_evento;

-- AggiornaTriageCandidato: la proposta di un messaggio cambia quando compare un'evidenza nuova (per
-- esempio: un operatore aggancia un messaggio, e gli altri orfani della stessa conversazione acquistano
-- un candidato R1). Cambia la PROPOSTA, non la decisione: tocca solo le righe ancora `proposta`.
-- name: AggiornaTriageCandidato :execrows
UPDATE proposta_triage
SET esito = 'aggancia', thread_proposto = $2, confidenza = $3,
    motivi = motivi || sqlc.arg(motivo)::jsonb
WHERE messaggio_id = $1 AND fonte = 'deterministico' AND stato = 'proposta';

-- ---------------------------------------------------------------- l'origine Cockpit (Smistamento M1)

-- OrigineCockpit: la nostra mail preparata dal Cockpit (D84). La bozza da cui nasce, la RFQ per cui e'
-- stata preparata e quella DI ADESSO della mail a cui rispondeva. Si legge ogni volta, e non si scrive
-- niente: una bozza non si aggiorna quando la mail a cui rispondeva viene agganciata dopo, e la nostra
-- mail non si aggancia da sola. Una query per pagina: `messaggi` sono gli id della lista o del pannello.
-- name: OrigineCockpit :many
SELECT b.inviata_messaggio_id::uuid AS messaggio_id, b.bozza_id, b.thread_id, b.in_risposta_a,
       mr.thread_id AS thread_risposta, mr.oggetto AS oggetto_risposta, mr.data_evento AS data_risposta,
       b.creata_il, u.sigla,
       tb.oggetto AS oggetto_thread, tr.oggetto AS oggetto_thread_risposta
FROM bozza b
JOIN utente u               ON u.utente_id = b.creata_da
LEFT JOIN messaggio mr      ON mr.messaggio_id = b.in_risposta_a
LEFT JOIN thread_offerta tb ON tb.thread_id = b.thread_id
LEFT JOIN thread_offerta tr ON tr.thread_id = mr.thread_id
WHERE b.inviata_messaggio_id = ANY(sqlc.arg(messaggi)::uuid[])
ORDER BY b.creata_il, b.bozza_id;

-- OrigineRichiesta: la nostra mail di una richiesta a un fornitore, legata dal marcatore
-- CockpitRichiestaFornitore (D85). Il marcatore lega la richiesta e basta: la mail resta orfana, e la
-- RFQ della richiesta diventa un candidato molto forte, che una persona conferma.
-- name: OrigineRichiesta :many
SELECT m.messaggio_id, r.richiesta_id, r.thread_id, r.creata_il, u.sigla, f.ragione_sociale AS fornitore
FROM messaggio m
JOIN richiesta_fornitore r ON r.richiesta_id = m.richiesta_fornitore_id AND r.messaggio_id = m.messaggio_id
JOIN fornitore f           ON f.fornitore_id = r.fornitore_id
LEFT JOIN utente u         ON u.utente_id = r.creata_da
WHERE m.messaggio_id = ANY(sqlc.arg(messaggi)::uuid[]) AND m.direzione = 'uscita'
ORDER BY r.creata_il, r.richiesta_id;
