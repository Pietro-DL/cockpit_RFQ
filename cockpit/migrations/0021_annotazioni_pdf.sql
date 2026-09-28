-- 0021_annotazioni_pdf.sql — Fascicolo v3: le note tecniche sui disegni.
--
-- Che cosa fa: aggiunge la tabella delle note puntate su un PDF (i «pin» della vista Documenti del
-- Fascicolo). Una nota sta su un punto preciso di una pagina di un file, e appartiene a quel file:
--
--   annotazione_pdf  RFQ, componente che si stava guardando (se c'e': un file ancora da associare, un
--                    capitolato o un documento della RFQ non hanno componente), allegato (il file visto),
--                    pagina, punto (coordinate normalizzate 0..1 sulla pagina com'e' mostrata, con la sua
--                    rotazione), testo, chi e quando.
--
-- Perche' l'allegato e non il documento: una nota si scrive anche su un PDF ancora da validare, prima
-- che diventi un documento del fascicolo. Perche' il componente per id e non per codice: se il codice
-- si corregge (un suffisso tolto, una lettera sbagliata), la nota resta dello stesso pezzo. Quando una revisione del disegno
-- viene sostituita la nota NON migra sul file nuovo: resta evidenza della revisione su cui e' stata
-- scritta, e la schermata la conta come «note sulla revisione precedente».
--
-- Coordinate normalizzate e non pixel: zoom e risoluzione dello schermo non spostano il punto.
--
-- Non e' struttura della BOM: il muro della BOM congelata (BOM01) non la riguarda, una nota si scrive
-- anche sulla V congelata. Un componente con delle note non si cancella (RimuoviComponente): si archivia,
-- e le note restano. Se il messaggio del file viene staccato dalla RFQ, le note restano della RFQ su cui
-- sono state scritte (thread_id) e la schermata non le mostra piu' finche' quel contenuto non torna.
--
-- Riferimenti: specifica «Fascicolo v3» (vista Documenti, note tecniche sul PDF).
--
-- Regole del migratore: un file = una transazione; ogni REFERENCES punta a una tabella gia' creata; nessun
-- enum nuovo; il file termina con INSERT INTO schema_versione.
--
-- Nessuna guardia: nessun dato esistente viene toccato, e la tabella nasce vuota.

-- ============================================================ 1. annotazione_pdf
CREATE TABLE annotazione_pdf (
    annotazione_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id      uuid NOT NULL REFERENCES thread_offerta,
    componente_id  uuid,
    allegato_id    uuid NOT NULL REFERENCES allegato,
    pagina         int NOT NULL,
    x_norm         numeric(7,6) NOT NULL,
    y_norm         numeric(7,6) NOT NULL,
    testo          text NOT NULL,
    creata_da      uuid NOT NULL REFERENCES utente,
    creata_il      timestamptz NOT NULL DEFAULT now(),
    modificata_da  uuid REFERENCES utente,
    modificata_il  timestamptz,
    CONSTRAINT fk_annotazione_componente FOREIGN KEY (thread_id, componente_id)
        REFERENCES componente (thread_id, componente_id),
    CONSTRAINT ck_annotazione_pagina CHECK (pagina BETWEEN 1 AND 10000),
    CONSTRAINT ck_annotazione_punto CHECK (x_norm BETWEEN 0 AND 1 AND y_norm BETWEEN 0 AND 1),
    CONSTRAINT ck_annotazione_testo CHECK (btrim(testo) <> '' AND char_length(testo) <= 2000),
    CONSTRAINT ck_annotazione_modifica CHECK ((modificata_da IS NULL) = (modificata_il IS NULL))
);

CREATE INDEX ix_annotazione_thread ON annotazione_pdf (thread_id);
CREATE INDEX ix_annotazione_allegato ON annotazione_pdf (allegato_id);
CREATE INDEX ix_annotazione_componente ON annotazione_pdf (componente_id);

COMMENT ON TABLE annotazione_pdf IS
'Nota tecnica puntata su un PDF (Fascicolo v3, vista Documenti): un punto di una pagina di un file, con il
testo. Appartiene al file su cui e'' stata scritta (allegato_id) e non migra quando il disegno viene
sostituito da una revisione nuova. La schermata mostra le note di un file per contenuto (lo sha256 dell''allegato
nella RFQ): due allegati identici mostrano le stesse note. Solo chi l''ha scritta la cambia o la toglie.';
COMMENT ON COLUMN annotazione_pdf.componente_id IS
'Il componente che si stava guardando quando la nota e'' nata: per id, cosi'' una correzione del codice non la
stacca dal pezzo. NULL per un file senza componente (da associare, capitolato, documento della RFQ).';
COMMENT ON COLUMN annotazione_pdf.pagina IS 'La pagina del PDF, da 1.';
COMMENT ON COLUMN annotazione_pdf.x_norm IS
'Il punto sulla pagina, da 0 (sinistra) a 1 (destra), sulla pagina com''e'' mostrata (rotazione del PDF applicata).';
COMMENT ON COLUMN annotazione_pdf.y_norm IS 'Il punto sulla pagina, da 0 (in alto) a 1 (in basso).';

-- ============================================================ 2. VERSIONE
INSERT INTO schema_versione (versione) VALUES (21);
