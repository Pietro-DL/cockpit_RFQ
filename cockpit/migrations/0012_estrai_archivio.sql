-- 0012 — l'estrazione di un archivio diventa un job (blocco 4A del checkpoint 3R).
--
-- Fino a qui uno zip veniva scompattato DENTRO la richiesta HTTP con cui il worker consegnava il
-- risultato del download. Non dentro la transazione — quello era gia' stato corretto alla voce 1.4 —
-- ma comunque dentro il gestore: il server teneva aperta una connessione HTTP mentre scriveva su
-- disco fino a mezzo gigabyte di file, e il worker restava fermo ad aspettare la risposta di un
-- lavoro che non era il suo.
--
-- Si vedeva nella prova reale: premuto «Carica precedenti», il server si trovava a scompattare zip
-- da 6 MB uno dietro l'altro mentre continuava a servire l'Inbox agli operatori.
--
-- Ora il result fa una cosa sola: promuove il contenuto verificato e ACCODA l'estrazione. Il job e'
-- di `worker_tipo = 'server'`, quindi lo prende l'esecutore interno — la stessa goroutine che scrive
-- sul NAS — che sta accanto al disco e non occupa il web server. Se il server muore a meta', il job
-- resta in coda e riparte: l'estrazione e' ripetibile, perche' le voci finiscono fra i contenuti con
-- il proprio sha256 per nome e le righe si riscrivono per upsert.
--
-- Questo file aggiunge soltanto il valore all'enum e non lo usa: PostgreSQL non permette di usare un
-- valore di enum prima del commit che lo crea.
ALTER TYPE tipo_job ADD VALUE IF NOT EXISTS 'estrai_archivio';

INSERT INTO schema_versione (versione) VALUES (12);
