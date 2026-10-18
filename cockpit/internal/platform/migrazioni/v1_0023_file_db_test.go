//go:build integrazione

// L4 — il ciclo dei file della Sorgente nella 0023 (piano della 0023, PM23 §4.10-§4.11, §8): la nascita e le transizioni
// dello stato (M-22, M-23), gli invarianti al COMMIT (M-24), il registro in sola aggiunta (M-25), la rimozione (M-26), la
// copia di conservazione (M-27), il predicato file_rfq_attivo (M-28), le cinque viste senza i file non attivi (M-31) e
// l'allineamento della RFQ delle proposte sulle righe di prova (M-33, parte del database). Le righe di base sono quelle
// di v1_0023_db_test.go (schema23ConStoria).

package migrazioni_test

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// righeViste: la baseline congelata V1 di T (un'ora fa) e, dopo, le proposte: DP3 (2D di S su A3), DP4 (3D di ACM-2000
// su A4, lo STEP), DP5 (2D di ACM-3000 su A5, senza componente), le righe strutturali CP1 e CP2 di A4 con l'arco #1 → #2.
var righeViste = []string{
	`INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F1}', '{T}', 'FATTIBILITA', now() - interval '2 hours');
	 INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
	      VALUES ('{V1}', '{T}', 1, 'preventivo', 'FATTIBILITA', '{F1}', 'prima baseline', '{U}');
	 INSERT INTO bom_versione_componente (bom_versione_id, thread_id, componente_id, codice, rev, tipo, descrizione, qta,
	                                      esito_fattibilita, note_fattibilita, step_strutturale_id, step_sha256)
	 SELECT '{V1}', c.thread_id, c.componente_id, c.codice, c.rev, c.tipo, c.descrizione, c.qta, c.esito_fattibilita, c.note_fattibilita,
	        c.step_strutturale_id, d.sha256
	   FROM componente c LEFT JOIN documento d ON d.documento_id = c.step_strutturale_id
	  WHERE c.thread_id = '{T}' AND c.archiviato_il IS NULL;
	 INSERT INTO bom_versione_relazione (bom_versione_id, padre_id, figlio_id, qta, posizione)
	 SELECT '{V1}', padre_id, figlio_id, qta, posizione FROM componente_relazione WHERE thread_id = '{T}';
	 INSERT INTO bom_versione_documento (bom_versione_id, componente_id, documento_id, tipo, rev, sha256, path_al_congelamento)
	 SELECT '{V1}', componente_id, documento_id, tipo, rev, sha256, path_relativo FROM documento
	  WHERE thread_id = '{T}' AND componente_id IS NOT NULL AND sostituito_da IS NULL;
	 UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now() - interval '1 hour' WHERE bom_versione_id = '{V1}';`,
	`INSERT INTO documento_proposta (proposta_id, allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte, dettagli) VALUES
	   ('{DP3}', '{A3}', '{T}', 'disegno_2d', 'ACM-1100', '{S}', 70, 'nome_file', '{"codici_nel_nome": ["ACM-1100"]}'),
	   ('{DP4}', '{A4}', '{T}', 'cad_3d', 'ACM-2000', NULL, 70, 'step', '{"codici_nel_nome": ["ACM-2000"]}'),
	   ('{DP5}', '{A5}', '{T}', 'disegno_2d', 'ACM-3000', NULL, 60, 'nome_file', '{}');
	 INSERT INTO componente_proposta (proposta_id, thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, fonte, confidenza) VALUES
	   ('{CP1}', '{T}', '{A4}', repeat('4', 64), '#1', 'ACM-2000', 'ACM-2000', 'step', 80),
	   ('{CP2}', '{T}', '{A4}', repeat('4', 64), '#2', 'ACM-2100', 'ACM-2100', 'step', 80);
	 INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ('{T}', '{A4}', '#1', '#2', 1);`,
}

// righeDelleViste: le righe delle cinque viste per la RFQ T, come testo ordinato (U-02 le confronta prima e dopo la 23).
func righeDelleViste(t *testing.T, p *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, v := range []string{"v_fascicolo", "v_step_prodotto", "v_thread_da_riesaminare", "v_cruscotto", "v_codici_candidati_thread"} {
		out[v] = uno23[string](t, p, `SELECT coalesce(string_agg(x::text, E'\n' ORDER BY x::text), '') FROM `+v+` x WHERE x.thread_id IN ('{T}', '{T2}')`)
	}
	return out
}

// M-22: una riga di stato nasce al primo gesto, in altri_file o scartato, alla versione 1, con la fotografia
// dell'allegato, per un file (non un inline) della stessa RFQ.
func TestV1M22LaNascitaDelloStatoDiUnFile(t *testing.T) {
	p := schema23ConStoria(t)
	nascita := func(allegato, stato, versione, cambi string) string {
		return `INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
			estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
		 SELECT '{T}', a.allegato_id, '` + stato + `', ` + versione + `, a.messaggio_id, m.canale, a.contenitore_id, a.indice, ` + cambi + `, a.path_interno,
			a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, '{U}', '{U}'
		   FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = '` + allegato + `'`
	}
	rifiuto23(t, p, "allegato di un'altra RFQ", "P4X09", "non e' della RFQ", nascita("{AX}", "altri_file", "1", "a.nome_file"))
	rifiuto23(t, p, "allegato inline", "P4X09", "di natura inline", nascita("{AI}", "altri_file", "1", "a.nome_file"))
	rifiuto23(t, p, "fotografia diversa dall'allegato", "P4X09", "la fotografia dell'occorrenza", nascita("{A5}", "altri_file", "1", "'altro.pdf'"))
	rifiuto23(t, p, "nascita in attivo", "P4X09", "una riga nasce in altri_file o scartato", nascita("{A5}", "attivo", "1", "a.nome_file"))
	rifiuto23(t, p, "nascita alla versione 2", "P4X09", "una riga nasce in altri_file o scartato", nascita("{A5}", "scartato", "2", "a.nome_file"))
	// la controprova: con la fotografia giusta, l'evento e la copia la riga nasce; date e autore li scrive il database
	riuscita23(t, p, "A5 in Altri file", inAltriFile("{A5}")...)
	if !uno23[bool](t, p, `SELECT creato_il > now() - interval '1 hour' AND aggiornato_da = creato_da FROM file_rfq WHERE allegato_id = '{A5}'`) {
		t.Error("creazione e aggiornamento non sono quelli del database")
	}
}

// M-23: le transizioni ammesse (attivo → altri_file | scartato; altri_file → attivo | scartato; scartato terminale), la
// versione che cresce di uno, identita' e fotografia che non cambiano; lo stato non si cancella.
func TestV1M23LeTransizioniDelloStato(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "A3 in Altri file", inAltriFile("{A3}")...)
	riuscita23(t, p, "A4 scartato", scartato("{A4}")...)
	frase := "non e' ammesso (lo scarto e' terminale)"
	rifiuto23(t, p, "da scartato ad attivo", "P4X09", frase, `UPDATE file_rfq SET stato = 'attivo', versione = versione + 1 WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "da scartato ad altri_file", "P4X09", frase, `UPDATE file_rfq SET stato = 'altri_file', versione = versione + 1 WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "la versione che salta", "P4X09", "la versione cresce di uno",
		`UPDATE file_rfq SET stato = 'attivo', versione = versione + 2 WHERE allegato_id = '{A3}'`)
	rifiuto23(t, p, "il nome che cambia", "P4X09", "identita', fotografia e creazione non cambiano",
		`UPDATE file_rfq SET nome_file = 'altro.pdf', versione = versione + 1, stato = 'attivo' WHERE allegato_id = '{A3}'`)
	rifiuto23(t, p, "lo sha256 che cambia", "P4X09", "identita', fotografia e creazione non cambiano",
		`UPDATE file_rfq SET sha256 = repeat('9', 64), versione = versione + 1, stato = 'attivo' WHERE allegato_id = '{A3}'`)
	rifiuto23(t, p, "DELETE dello stato", "P4X09", "non si cancella", `DELETE FROM file_rfq WHERE allegato_id = '{A3}'`)
	rifiuto23(t, p, "TRUNCATE dello stato", "P4X09", "non si svuota", `TRUNCATE file_rfq CASCADE`)
}

// M-24: gli invarianti al COMMIT: altri_file con una copia verificata, scartato con la riga della rimozione, ogni
// transizione con il suo evento; una copia esce da «copiata» solo se il file lascia Altri file nella stessa transazione
// (o ha un'altra copia verificata).
func TestV1M24GliInvariantiAlCommit(t *testing.T) {
	p := schema23ConStoria(t)
	evento := func(allegato, azione, prima, dopo string) string {
		motivo := "NULL"
		if azione == "scarta" {
			motivo = "'non serve'"
		}
		return `INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, origine, autore_id)
			VALUES ('{T}', '` + allegato + `', '` + azione + `', '` + prima + `', '` + dopo + `', ` + motivo + `, '{}', '{}', 'ponte', '{U}')`
	}
	rifiuto23(t, p, "altri_file senza copia verificata", "P4X09", "in Altri file senza una copia di conservazione verificata",
		fotografiaFile("{A5}", "altri_file"), evento("{A5}", "altri_file", "attivo", "altri_file"))
	rifiuto23(t, p, "scartato senza la rimozione", "P4X09", "scartato senza la riga della rimozione",
		fotografiaFile("{A5}", "scartato"), evento("{A5}", "scarta", "attivo", "scartato"))
	altri := inAltriFile("{A5}")
	rifiuto23(t, p, "transizione senza evento", "P4X09", "non ha il suo evento", altri[:3]...)
	riuscita23(t, p, "A3 in Altri file", inAltriFile("{A3}")...)
	rifiuto23(t, p, "la copia rilasciata con il file ancora in Altri file", "P4X09", "non e' piu' copiata e il file non ha lasciato Altri file",
		`UPDATE file_rfq_conservazione SET stato = 'da_rilasciare', causa = 'rifiutata' WHERE allegato_id = '{A3}' AND stato = 'copiata'`)
	riuscita23(t, p, "la copia rilasciata con un'altra copia verificata",
		`UPDATE file_rfq_conservazione SET stato = 'da_rilasciare', causa = 'rifiutata' WHERE allegato_id = '{A3}' AND stato = 'copiata'`,
		`INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, percorso_relativo, deposito_id)
		 VALUES ('{T}', '{A3}', repeat('3', 64), 'segnaposto', gen_random_uuid())`,
		`UPDATE file_rfq_conservazione SET stato = 'copiata', sorgente_byte = 'conservazione' WHERE allegato_id = '{A3}' AND stato = 'in_copia'`)
	riuscita23(t, p, "Riattiva con il rilascio della copia",
		`UPDATE file_rfq SET stato = 'attivo', versione = versione + 1, aggiornato_da = '{U}' WHERE allegato_id = '{A3}'`,
		`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, origine, autore_id)
		 VALUES ('{T}', '{A3}', 'riattiva', 'altri_file', 'attivo', '{}', 'ponte', '{U}')`,
		`UPDATE file_rfq_conservazione SET stato = 'da_rilasciare', causa = 'riattivato' WHERE allegato_id = '{A3}' AND stato = 'copiata'`)
	riuscita23(t, p, "A5 in Altri file", inAltriFile("{A5}")...)
	riuscita23(t, p, "Scarta da Altri file con il rilascio della copia",
		`UPDATE file_rfq SET stato = 'scartato', versione = versione + 1, aggiornato_da = '{U}' WHERE allegato_id = '{A5}'`,
		evento("{A5}", "scarta", "altri_file", "scartato"),
		`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id, sha256)
		 SELECT e.thread_id, e.allegato_id, e.evento_id, repeat('5', 64) FROM file_rfq_evento e WHERE e.allegato_id = '{A5}' AND e.azione = 'scarta'`,
		`UPDATE file_rfq_conservazione SET stato = 'da_rilasciare', causa = 'scartato' WHERE allegato_id = '{A5}' AND stato = 'copiata'`)
	if s := uno23[string](t, p, `SELECT stato || ' ' || versione FROM file_rfq WHERE allegato_id = '{A3}'`); s != "attivo 2" {
		t.Errorf("A3 dopo Riattiva: %s", s)
	}
}

// M-25: il registro dei file non si modifica e non si cancella; una chiave d'idempotenza vale una volta per occorrenza.
func TestV1M25IlRegistroDeiFile(t *testing.T) {
	p := schema23ConStoria(t)
	scarta := `INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, origine, autore_id,
		chiave_idempotenza) VALUES ('{T}', '{A5}', 'scarta', 'attivo', 'scartato', 'non serve', '{}', '{}', 'ponte', '{U}', '{K4}')`
	rifiuto23(t, p, "due scarta con la stessa chiave sulla stessa occorrenza", "23505", "ux_fre_chiave", scarta, scarta)
	rifiuto23(t, p, "UPDATE di un evento", "P4X02", "non si modifica", `UPDATE file_rfq_evento SET motivo = 'altro'`)
	rifiuto23(t, p, "DELETE di un evento", "P4X02", "non si modifica", `DELETE FROM file_rfq_evento`)
	rifiuto23(t, p, "TRUNCATE del registro", "P4X02", "non si svuota", `TRUNCATE file_rfq_evento CASCADE`)
	// data e transazione le scrive il database
	if !uno23[bool](t, p, `SELECT bool_and(quando > now() - interval '1 hour') FROM file_rfq_evento`) {
		t.Error("le date del registro non sono quelle del database")
	}
}

// M-26: la rimozione dallo staging: nasce in_attesa dallo scarto della sua occorrenza, una per occorrenza; da eseguita
// non si esce; da un errore non ritentabile si esce solo con eseguita / bonificato, e bonificato solo da li'; i
// tentativi non calano, il percorso trovato non cambia; la storia non si cancella.
func TestV1M26LaRimozione(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "A3 in Altri file", inAltriFile("{A3}")...)
	riuscita23(t, p, "A4 scartato", scartato("{A4}")...)
	riuscita23(t, p, "A5 scartato", scartato("{A5}")...)
	rifiuto23(t, p, "evento che non e' uno scarto", "P4X09", "non e' lo scarto dell'occorrenza",
		`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id) SELECT thread_id, allegato_id, evento_id FROM file_rfq_evento
		  WHERE allegato_id = '{A3}' AND azione = 'altri_file'`)
	rifiuto23(t, p, "nascita in eseguita", "P4X09", "nasce in_attesa",
		`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id, stato, motivo, concluso_il) SELECT thread_id, allegato_id, evento_id,
		  'eseguita', 'rimosso', now() FROM file_rfq_evento WHERE allegato_id = '{A4}' AND azione = 'scarta'`)
	rifiuto23(t, p, "seconda rimozione per la stessa occorrenza", "23505", "ux_rimozione_occorrenza",
		`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id) SELECT thread_id, allegato_id, evento_id FROM file_rfq_evento
		  WHERE allegato_id = '{A4}' AND azione = 'scarta'`)
	rifiuto23(t, p, "bonificato da una riga in_attesa", "P4X09", "bonificato vale solo",
		`UPDATE file_rfq_rimozione SET stato = 'eseguita', motivo = 'bonificato' WHERE allegato_id = '{A5}'`)
	riuscita23(t, p, "tentativo in errore ritentabile", `UPDATE file_rfq_rimozione SET stato = 'errore', motivo = 'file_occupato',
		errore = 'file occupato', tentativi = 2, percorso_trovato = 'staging\ab\cd' WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "tentativi che calano", "P4X09", "i tentativi non calano", `UPDATE file_rfq_rimozione SET tentativi = 1 WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "percorso trovato che cambia", "P4X09", "il percorso trovato non cambia",
		`UPDATE file_rfq_rimozione SET percorso_trovato = 'staging\xx' WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "bonificato da un errore ritentabile", "P4X09", "bonificato vale solo",
		`UPDATE file_rfq_rimozione SET stato = 'eseguita', motivo = 'bonificato', errore = NULL WHERE allegato_id = '{A4}'`)
	riuscita23(t, p, "errore non ritentabile", `UPDATE file_rfq_rimozione SET motivo = 'fuori_cache', ritentabile = false, errore = 'fuori dalla cache'
		WHERE allegato_id = '{A4}'`)
	frase := "da un errore non ritentabile si esce solo con eseguita / bonificato"
	rifiuto23(t, p, "da errore non ritentabile a in_attesa", "P4X09", frase,
		`UPDATE file_rfq_rimozione SET stato = 'in_attesa', motivo = 'da_eseguire', ritentabile = true, errore = NULL WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "da errore non ritentabile a eseguita / gia_assente", "P4X09", frase,
		`UPDATE file_rfq_rimozione SET stato = 'eseguita', motivo = 'gia_assente', ritentabile = true, errore = NULL WHERE allegato_id = '{A4}'`)
	rifiuto23(t, p, "da errore non ritentabile a eseguita / rimosso", "P4X09", frase,
		`UPDATE file_rfq_rimozione SET stato = 'eseguita', motivo = 'rimosso', ritentabile = true, errore = NULL WHERE allegato_id = '{A4}'`)
	riuscita23(t, p, "da errore non ritentabile a eseguita / bonificato",
		`UPDATE file_rfq_rimozione SET stato = 'eseguita', motivo = 'bonificato', ritentabile = true, errore = NULL WHERE allegato_id = '{A4}'`)
	if !uno23[bool](t, p, `SELECT concluso_il IS NOT NULL FROM file_rfq_rimozione WHERE allegato_id = '{A4}'`) {
		t.Error("la rimozione conclusa non ha concluso_il")
	}
	rifiuto23(t, p, "uscita da eseguita", "P4X09", "e' conclusa", `UPDATE file_rfq_rimozione SET tentativi = tentativi + 1 WHERE allegato_id = '{A4}'`)
	riuscita23(t, p, "in_attesa a eseguita / rimosso", `UPDATE file_rfq_rimozione SET stato = 'eseguita', motivo = 'rimosso' WHERE allegato_id = '{A5}'`)
	rifiuto23(t, p, "DELETE di una rimozione", "P4X02", "non si cancella", `DELETE FROM file_rfq_rimozione`)
	rifiuto23(t, p, "TRUNCATE delle rimozioni", "P4X02", "non si svuotano", `TRUNCATE file_rfq_rimozione`)
}

// M-27: la copia di conservazione: il percorso lo calcola il database (un file per riga), l'allegato e' della RFQ con
// lo sha256 dichiarato, una sola copia viva per occorrenza, transizioni ammesse; la storia non si cancella.
func TestV1M27LaConservazione(t *testing.T) {
	p := schema23ConStoria(t)
	copia := func(allegato, shaExpr, percorso string) string {
		return `INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, percorso_relativo, deposito_id)
			VALUES ('{T}', '` + allegato + `', ` + shaExpr + `, '` + percorso + `', gen_random_uuid())`
	}
	riuscita23(t, p, "copia con un percorso del Go", copia("{A5}", "repeat('5', 64)", `C:\fuori\..\x.pdf`))
	id := uno23[int64](t, p, `SELECT conservazione_id FROM file_rfq_conservazione WHERE allegato_id = '{A5}'`)
	atteso := u23("{T}") + `\` + u23("{A5}") + "." + itoa(id) + ".pdf"
	if got := uno23[string](t, p, `SELECT percorso_relativo FROM file_rfq_conservazione WHERE allegato_id = '{A5}'`); got != atteso {
		t.Errorf("percorso %q, atteso %q: lo calcola il trigger", got, atteso)
	}
	rifiuto23(t, p, "allegato di un'altra RFQ", "P4X09", "non e' della RFQ", strings.Replace(copia("{AX}", "repeat('b', 64)", "x"), "'{T}'", "'{T}'", 1))
	rifiuto23(t, p, "sha256 diverso dall'allegato", "P4X09", "non e' della RFQ", copia("{A3}", "repeat('9', 64)", "x"))
	rifiuto23(t, p, "nascita non in_copia", "P4X09", "nasce in_copia", `INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, percorso_relativo,
		deposito_id, stato, verificato_il, sorgente_byte) VALUES ('{T}', '{A3}', repeat('3', 64), 'x', gen_random_uuid(), 'copiata', now(), 'staging')`)
	rifiuto23(t, p, "due copie vive per la stessa occorrenza", "23505", "ux_cons_viva", copia("{A5}", "repeat('5', 64)", "x"))
	riuscita23(t, p, "battito della copia", `UPDATE file_rfq_conservazione SET tentativi = 1 WHERE allegato_id = '{A5}'`)
	rifiuto23(t, p, "tentativi che calano", "P4X09", "i tentativi non calano", `UPDATE file_rfq_conservazione SET tentativi = 0 WHERE allegato_id = '{A5}'`)
	rifiuto23(t, p, "deposito che cambia", "P4X09", "identita', percorso, deposito e chiave non cambiano",
		`UPDATE file_rfq_conservazione SET deposito_id = gen_random_uuid() WHERE allegato_id = '{A5}'`)
	rifiuto23(t, p, "percorso che cambia", "P4X09", "identita', percorso, deposito e chiave non cambiano",
		`UPDATE file_rfq_conservazione SET percorso_relativo = percorso_relativo || 'x' WHERE allegato_id = '{A5}'`)
	// battito fermo: da in_copia a da_rilasciare (tempo scaduto); poi una copia nuova della stessa occorrenza passa
	riuscita23(t, p, "copia ferma da rilasciare", `UPDATE file_rfq_conservazione SET stato = 'da_rilasciare', causa = 'tempo_scaduto' WHERE allegato_id = '{A5}'`)
	riuscita23(t, p, "copia nuova accanto a una da rilasciare", copia("{A5}", "repeat('5', 64)", "x"))
	rifiuto23(t, p, "da da_rilasciare a in_copia", "P4X09", "non e' ammesso",
		`UPDATE file_rfq_conservazione SET stato = 'in_copia', causa = NULL WHERE allegato_id = '{A5}' AND stato = 'da_rilasciare'`)
	riuscita23(t, p, "rilasciata", `UPDATE file_rfq_conservazione SET stato = 'rilasciata' WHERE allegato_id = '{A5}' AND stato = 'da_rilasciare'`)
	rifiuto23(t, p, "da rilasciata a copiata", "P4X09", "non e' ammesso",
		`UPDATE file_rfq_conservazione SET stato = 'copiata', causa = NULL, sorgente_byte = 'staging' WHERE allegato_id = '{A5}' AND stato = 'rilasciata'`)
	riuscita23(t, p, "copiata", `UPDATE file_rfq_conservazione SET stato = 'copiata', sorgente_byte = 'staging' WHERE allegato_id = '{A5}' AND stato = 'in_copia'`)
	if !uno23[bool](t, p, `SELECT verificato_il IS NOT NULL AND concluso_il IS NULL FROM file_rfq_conservazione WHERE allegato_id = '{A5}' AND stato = 'copiata'`) {
		t.Error("la copia verificata non ha verificato_il, o ha concluso_il")
	}
	if !uno23[bool](t, p, `SELECT concluso_il IS NOT NULL FROM file_rfq_conservazione WHERE allegato_id = '{A5}' AND stato = 'rilasciata'`) {
		t.Error("la copia rilasciata non ha concluso_il")
	}
	rifiuto23(t, p, "DELETE di una copia", "P4X02", "non si cancella", `DELETE FROM file_rfq_conservazione`)
	rifiuto23(t, p, "TRUNCATE delle copie", "P4X02", "non si svuotano", `TRUNCATE file_rfq_conservazione`)
}

// M-28: il predicato file_rfq_attivo: nessuna riga → vero; altri_file → falso; scartato → falso; attivo dopo Riattiva →
// vero; RFQ nulla → vero.
func TestV1M28IlPredicatoFileAttivo(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "A3 in Altri file", inAltriFile("{A3}")...)
	riuscita23(t, p, "A4 scartato", scartato("{A4}")...)
	got := uno23[string](t, p, `SELECT concat_ws(' ', file_rfq_attivo('{T}', '{A5}'), file_rfq_attivo('{T}', '{A3}'), file_rfq_attivo('{T}', '{A4}'),
		file_rfq_attivo(NULL, '{A3}'), file_rfq_attivo('{T2}', '{A3}'))`)
	if got != "t f f t t" {
		t.Errorf("file_rfq_attivo (nessuna riga, altri_file, scartato, RFQ nulla, altra RFQ): %s", got)
	}
	riuscita23(t, p, "Riattiva A3",
		`UPDATE file_rfq SET stato = 'attivo', versione = versione + 1, aggiornato_da = '{U}' WHERE allegato_id = '{A3}'`,
		`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, origine, autore_id)
		 VALUES ('{T}', '{A3}', 'riattiva', 'altri_file', 'attivo', '{}', 'ponte', '{U}')`,
		`UPDATE file_rfq_conservazione SET stato = 'da_rilasciare', causa = 'riattivato' WHERE allegato_id = '{A3}' AND stato = 'copiata'`)
	if !uno23[bool](t, p, `SELECT file_rfq_attivo('{T}', '{A3}')`) {
		t.Error("dopo Riattiva l'occorrenza non e' attiva")
	}
}

// M-31: con una proposta aperta (e righe strutturali) di un'occorrenza in altri_file o scartata, la riga sparisce da
// v_fascicolo e da v_step_prodotto, non conta in v_thread_da_riesaminare (file e proposte) ne' in
// v_cruscotto.n_da_smistare, e il suo codice non compare in v_codici_candidati_thread. M-33 (parte del database): sulle
// righe di prova la RFQ di ogni proposta e' quella del messaggio del suo allegato.
func TestV1M31LeVisteSenzaIFileNonAttivi(t *testing.T) {
	misure := func(t *testing.T, p *pgxpool.Pool) string {
		return uno23[string](t, p, `SELECT concat_ws(' | ',
			(SELECT coalesce(proposta_aperta::text, '-') || ' ' || n_proposte_aperte FROM v_fascicolo WHERE componente_id = '{S}' AND tipo_documento = 'disegno_2d'),
			(SELECT coalesce(proposta_aperta::text, '-') FROM v_step_prodotto WHERE componente_id = '{P2}'),
			(SELECT n_file || ' ' || n_proposte FROM v_thread_da_riesaminare WHERE thread_id = '{T}' AND tipo_motivo = 'evidenze_nuove'),
			(SELECT n_da_smistare::text FROM v_cruscotto WHERE thread_id = '{T}'),
			(SELECT count(*)::text FROM v_codici_candidati_thread WHERE thread_id = '{T}' AND allegato_id IN ('{A3}', '{A4}')))`)
	}
	prepara := func(t *testing.T) *pgxpool.Pool {
		p := schema23ConStoria(t)
		for _, s := range righeViste {
			esegui23(t, p, s)
		}
		return p
	}
	t.Run("A3 in Altri file, A4 scartato", func(t *testing.T) {
		p := prepara(t)
		// n_file: i due documenti tecnici di base (D, D2) e le quattro proposte aperte (DP di base, DP3, DP4, DP5), tutti
		// dopo la baseline; n_proposte: CP1, CP2 e l'arco su A4
		attesa := u23("{DP3}") + " 1 | " + u23("{DP4}") + " | 6 3 | 4 | 6"
		if m := misure(t, p); m != attesa {
			t.Fatalf("prima: %s, attese %s", m, attesa)
		}
		riuscita23(t, p, "A3 in Altri file", inAltriFile("{A3}")...)
		riuscita23(t, p, "A4 scartato", scartato("{A4}")...)
		if m, attesa := misure(t, p), "- 0 | - | 4 0 | 2 | 0"; m != attesa {
			t.Errorf("dopo: %s, attese %s", m, attesa)
		}
	})
	t.Run("A3 scartato, A4 in Altri file", func(t *testing.T) {
		p := prepara(t)
		riuscita23(t, p, "A3 scartato", scartato("{A3}")...)
		riuscita23(t, p, "A4 in Altri file", inAltriFile("{A4}")...)
		if m, attesa := misure(t, p), "- 0 | - | 4 0 | 2 | 0"; m != attesa {
			t.Errorf("dopo: %s, attese %s", m, attesa)
		}
		// M-33, parte del database: nessuna proposta con una RFQ diversa da quella del messaggio del suo allegato
		if n := uno23[int](t, p, `SELECT count(*) FROM documento_proposta d JOIN allegato a ON a.allegato_id = d.allegato_id
			JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE d.thread_id IS DISTINCT FROM m.thread_id`); n != 0 {
			t.Errorf("%d proposte con una RFQ diversa da quella del loro allegato", n)
		}
	})
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
