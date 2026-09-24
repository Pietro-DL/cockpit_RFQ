-- 0019_sposta_nas.sql — Blocco 8, A4 (B8.A4a): lo spostamento di un file sul NAS e quello che lascia.
--
-- Che cosa fa: aggiunge il tipo di job dello spostamento e le due tabelle del suo protocollo
-- (addendum A4.3). Nessun codice le usa ancora: il protocollo arriva con B8.8. La migrazione si
-- consegna prima della 0020 perche' il migratore vuole versioni consecutive, e resta innocua finche'
-- nessuno la usa.
--
--   nas_orfano     un file sul NAS che nessun documento dichiara, e che lo spostamento non e' riuscito
--                  o non ha voluto togliere (D34). Una riga aperta tiene occupato il suo nome.
--   nas_creazione  la prova che un file l'ha creato proprio quel job (D38): si scrive solo dopo una
--                  promozione esclusiva riuscita, e senza questa riga il file non si toglie.
--
-- Riferimenti: addendum A4.3 (protocollo, riserva dei nomi, orfani, prova della creazione), A4.10.
--
-- Regole del migratore: un file = una transazione; ogni REFERENCES punta a una tabella gia' creata; il
-- valore di enum aggiunto qui NON si usa qui (PostgreSQL non lo accetta prima del commit che lo crea:
-- entra in uso dal codice di B8.8); il file termina con INSERT INTO schema_versione.
--
-- Nessuna guardia: nessun dato esistente viene toccato, e le due tabelle nascono vuote.

-- ============================================================ 1. IL TIPO DI JOB
-- Chiave idempotente del job: sposta:<documento_id>. L'indice parziale ux_job_chiave_pendente (0001)
-- ammette un solo spostamento pendente per documento.
ALTER TYPE tipo_job ADD VALUE 'sposta_nas';

-- ============================================================ 2. nas_orfano (D34)
-- Perche' una riga e' nata:
--   rimozione_fallita  la sorgente non si e' potuta togliere (permessi, file aperto da un client
--                      Windows) e i tentativi del job sono finiti: nasce nella stessa transazione
--                      che porta il job a `fallito`
--   non_nostro         al passo 5 la sorgente ha uno SHA diverso da quello del job: non la si tocca
--   percorso_cambiato  al passo 4b il documento non dichiara piu' la sorgente, e nessun altro la
--                      dichiara: non l'ha creata il job, e non si sa perche' sia rimasta
--   origine_incerta    al passo 4b la destinazione c'e' con lo SHA giusto, ma manca la prova che l'abbia
--                      creata questo job (nas_creazione): il dubbio sta dal lato che non cancella
CREATE TYPE motivo_orfano AS ENUM ('rimozione_fallita', 'non_nostro', 'percorso_cambiato', 'origine_incerta');

CREATE TABLE nas_orfano (
    nas_orfano_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    thread_id     uuid NOT NULL REFERENCES thread_offerta,
    percorso      varchar(500) NOT NULL CHECK (btrim(percorso) <> ''),  -- come documento.path_relativo: relativo alla cartella del thread
    sha256        char(64),                                -- lo SHA letto sul file quando la riga e' nata; NULL se non si e' potuto leggere
    motivo        motivo_orfano NOT NULL,
    -- Il job che l'ha lasciato. SET NULL perche' la riga deve sopravvivere al job: EliminaJobVecchi
    -- cancella i job chiusi da tempo, e una FK senza azione gli impedirebbe di cancellare proprio i
    -- job falliti che hanno lasciato un orfano.
    job_id        bigint REFERENCES job ON DELETE SET NULL,
    rilevato_il   timestamptz NOT NULL DEFAULT now(),
    -- Una riga si chiude solo quando a quel percorso il file non c'e' piu': tolto dall'amministratore
    -- con i controlli del passo 5 rifatti al momento, oppure gia' sparito. Chiusa dal sistema (un job
    -- riaccodato che toglie la sorgente) non ha risolto_da.
    risolto_il    timestamptz,
    risolto_da    uuid REFERENCES utente,
    nota          text,
    CHECK (risolto_da IS NULL OR risolto_il IS NOT NULL)
);

-- Una sola riga aperta per file (R2.1). lower() perche' la condivisione e' Windows e non distingue le
-- maiuscole: e' la stessa regola dell'indice su documento.path_relativo della 0020. InsertNasOrfano fa
-- ON CONFLICT DO NOTHING su questo indice e dice se la riga c'era gia': per come e' costruita la
-- riserva non dovrebbe succedere, e se succede e' un difetto da guardare, non una seconda riga.
CREATE UNIQUE INDEX ux_nas_orfano_aperto ON nas_orfano (thread_id, lower(percorso)) WHERE risolto_il IS NULL;
-- RisolviNasOrfaniDelJob: il passo 6 di un job riaccodato che ha tolto la sorgente chiude le sue righe.
CREATE INDEX ix_nas_orfano_job ON nas_orfano (job_id) WHERE risolto_il IS NULL;

COMMENT ON TABLE nas_orfano IS
'File sul NAS che nessun documento dichiara, lasciato da uno spostamento (addendum A4.3, D34). Una riga
aperta tiene occupato il suo nome (PercorsoOccupato) e si chiude solo quando a quel percorso il file non
c''e'' piu''. Se si decide di tenere il file, la riga resta aperta. La vede l''amministratore nella
schermata dell''integrita''.';

-- ============================================================ 3. nas_creazione (D38)
-- Il passo 3b: dopo una promozione ESCLUSIVA riuscita (link, oppure O_CREATE|O_EXCL) il tentativo
-- scrive qui, in una transazione breve e con il predicato di validita' del tentativo, che quel file
-- l'ha creato lui. Il passo 4b toglie la destinazione solo con questa riga, lo stesso SHA sul file e
-- nessun documento che la dichiari. Se il tentativo cade fra la promozione e la riga, la prova manca
-- e il file vale come non nostro.
--
-- Serve solo finche' il job e' pendente: se ne va con lui quando EliminaJobVecchi lo cancella.
CREATE TABLE nas_creazione (
    job_id    bigint NOT NULL REFERENCES job ON DELETE CASCADE,
    percorso  varchar(500) NOT NULL,                        -- la destinazione, com'e' scritta nel payload del job
    sha256    char(64) NOT NULL,
    creato_il timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (job_id, percorso)
);

COMMENT ON TABLE nas_creazione IS
'Prova che un file sul NAS l''ha creato quel job (addendum A4.3, passo 3b, D38). Si scrive solo dopo una
promozione esclusiva riuscita e si legge solo al passo 4b. Non e'' il risultato del job: quello lo scrive
solo CompletaJob, e lo sovrascrive.';

-- ============================================================ 4. VERSIONE
INSERT INTO schema_versione (versione) VALUES (19);
