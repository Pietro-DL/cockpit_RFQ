-- 0021_indietro.sql — ritorno MANUALE dallo schema 21 allo schema 20 (Fascicolo v3, note sui disegni).
--
-- Non e' una migrazione: il migratore non ha il verso «giu'» e questo file non va MAI in
-- cockpit/migrations/. Si lancia a mano, in una transazione, sul database che si vuole riportare
-- indietro, con il server FERMO; poi si riavvia il binario vecchio (schema 20): un cockpit.exe che conosce
-- solo la 20 rifiuta di partire su un database alla 21 («aggiornare cockpit.exe»).
--
-- Il ritorno vero e' il backup preso subito prima della 0021 (scripts/backup-db.ps1). Questo file serve
-- quando il backup non c'e' piu'. La 0021 aggiunge solo la tabella delle note sui disegni: tornare indietro
-- la toglie. Se ci sono delle note si ferma, perche' andrebbero perse: per toglierle lo stesso si mette
-- a true la variabile qui sotto (le note sono lavoro di validazione, e si perdono davvero).

BEGIN;

SET LOCAL cockpit.perdi_le_note = 'false';

DO $$
DECLARE
    v integer;
    n bigint;
BEGIN
    SELECT max(versione) INTO v FROM schema_versione;
    IF v IS DISTINCT FROM 21 THEN
        RAISE EXCEPTION '0021 indietro: lo schema e'' alla versione %, non alla 21', v;
    END IF;
    SELECT count(*) INTO n FROM annotazione_pdf;
    IF n > 0 AND current_setting('cockpit.perdi_le_note') <> 'true' THEN
        RAISE EXCEPTION '0021 indietro: ci sono % note sui disegni, e tornando alla 20 si perdono', n
            USING HINT = 'Prima un backup (scripts/backup-db.ps1); poi, se si vuole davvero, SET LOCAL cockpit.perdi_le_note = ''true'' in questo file.';
    END IF;
END $$;

DROP TABLE annotazione_pdf;
DELETE FROM schema_versione WHERE versione = 21;

COMMIT;
