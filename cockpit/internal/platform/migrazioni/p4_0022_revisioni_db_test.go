//go:build integrazione

// L4 — le correzioni della 0022 chieste dalle tre revisioni indipendenti della notte 12→13/10 (giro 5, A1d, P4):
// lente SQL e PostgreSQL (R-*), lente di dominio (RD-*), lente «guardie e scrittori» (RSC-*). Ogni prova riproduce
// l'aggiramento o il buco descritto dal revisore e vuole il rifiuto del database, con la controprova che il percorso
// normale passa. Solo dati sintetici (ACME). Le righe di base sono quelle di p4_0022_db_test.go (schema22ConStoria),
// gli aiuti quelli di p4_0022_dir11_db_test.go.

package migrazioni_test

import (
	"fmt"
	"strings"
	"testing"
)

// R-01 + R-02 (prova T5 della revisione SQL): prima della correzione, una conferma mono_pezzo arrivava al COMMIT con un
// figlio nella working, togliendo a mano il muro tx_conferma e anticipando il controllo finale. Ora il primo passo si
// rifiuta (P4X05), e il controllo anticipato chiude il contenuto della conferma (P4X01).
func TestP4RevisioniControlloFinaleAnticipato(t *testing.T) {
	p := schema22ConStoria(t)
	ok := conferma2("0", "0", "2", "nessuno")
	// T2: contenuto giusto, controllo finale anticipato, poi un componente in piu' senza arco
	rifiutoAtteso(t, p, "R-02 componente aggiunto dopo il controllo anticipato", "P4X01", "ha gia' passato il controllo finale",
		ok, radiceC2, figlioC2, arcoC2,
		`SET CONSTRAINTS trg_conferma_strutturale_completa IMMEDIATE`,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo)
		 VALUES ('{C2}', '{T}', '{P2}', false, 'k-p2', 'ACM-2000', 'Piastra', 'finito')`)
	// lo stesso con SET CONSTRAINTS ALL
	rifiutoAtteso(t, p, "R-02 con SET CONSTRAINTS ALL IMMEDIATE", "P4X01", "ha gia' passato il controllo finale",
		ok, radiceC2, figlioC2, arcoC2, `SET CONSTRAINTS ALL IMMEDIATE`,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo)
		 VALUES ('{C2}', '{T}', '{P2}', false, 'k-p2', 'ACM-2000', 'Piastra', 'finito')`)
	// T5, primo passo: il muro della conferma non si toglie a mano
	rifiutoAtteso(t, p, "R-01 tx_conferma rimessa a NULL dopo la conferma", "P4X05", "la scrivono solo i trigger",
		ok, radiceC2, figlioC2, arcoC2,
		`UPDATE working_contatore SET tx_conferma = NULL WHERE thread_id = '{T}'`)
	// T1a, T1b: il contatore non si ferma con una marca scritta prima delle scritture
	rifiutoAtteso(t, p, "R-01 riga del contatore scritta a mano", "P4X05", "la scrivono solo i trigger",
		`INSERT INTO working_contatore (thread_id, tx_struttura, tx_dati) VALUES (gen_random_uuid(), pg_current_xact_id(), pg_current_xact_id())`)
	rifiutoAtteso(t, p, "R-01 marca della struttura scritta prima di un cambio di codice", "P4X05", "la scrivono solo i trigger",
		`UPDATE working_contatore SET tx_struttura = pg_current_xact_id() WHERE thread_id = '{T}'`,
		`UPDATE componente SET codice = 'ACM-3001' WHERE componente_id = '{P3}'`)
	// la controprova: il contatore si muove ancora da solo, e la conferma con il controllo anticipato passa
	prima := uno11[int64](t, p, `SELECT struttura FROM working_contatore WHERE thread_id = '{T}'`)
	riuscita(t, p, "cambio di codice", `UPDATE componente SET codice = 'ACM-3001' WHERE componente_id = '{P3}'`,
		`UPDATE componente SET codice = 'ACM-3000' WHERE componente_id = '{P3}'`)
	if dopo := uno11[int64](t, p, `SELECT struttura FROM working_contatore WHERE thread_id = '{T}'`); dopo != prima+1 {
		t.Errorf("il contatore struttura dopo una transazione con due cambi di codice: %d → %d (atteso +1)", prima, dopo)
	}
	riuscita(t, p, "conferma con il controllo finale anticipato e niente dopo", ok, radiceC2, figlioC2, arcoC2,
		`SET CONSTRAINTS trg_conferma_strutturale_completa IMMEDIATE`)
}

// R-03 / RSC-01: un'isola X ↔ Y nella working, fuori dal perimetro di P, non si certifica nella conferma di P (prima
// passava: ogni componente aveva un arco certificato che arrivava).
func TestP4RevisioniIsolaCiclica(t *testing.T) {
	p := schema22ConStoria(t)
	const x, y = "00000000-0000-4000-8000-000000221301", "00000000-0000-4000-8000-000000221302"
	isola := `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES
		  ('` + x + `', '{T}', 'ACM-1401', 'sottoassieme', 'manuale', '{U}'), ('` + y + `', '{T}', 'ACM-1402', 'sottoassieme', 'manuale', '{U}');
		INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES
		  ('{T}', '` + x + `', '` + y + `', 1, 'manuale', '{U}'), ('{T}', '` + y + `', '` + x + `', 1, 'manuale', '{U}')`
	certificaIsola := []string{
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, tipo) VALUES
		  ('{C2}', '{T}', '` + x + `', false, 'k-x', 'ACM-1401', 'sottoassieme'), ('{C2}', '{T}', '` + y + `', false, 'k-y', 'ACM-1402', 'sottoassieme')`,
		`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES
		  ('{C2}', '` + x + `', '` + y + `', 1, 'utente'), ('{C2}', '` + y + `', '` + x + `', 1, 'utente')`,
	}
	rifiutoAtteso(t, p, "isola ciclica certificata", "P4X01", "nessun arco certificato ci arriva dalla radice",
		append([]string{isola, conferma2("0", "0", "2", "nessuno"), radiceC2, figlioC2, arcoC2}, certificaIsola...)...)
	// la controprova: con l'isola nella working ma non certificata, la conferma di P passa
	riuscita(t, p, "isola nella working, conferma del solo perimetro di P", isola, conferma2("0", "0", "2", "nessuno"), radiceC2, figlioC2, arcoC2)
}

// RSC-09: una conferma che cita una bozza la chiude come confermata nella stessa transazione; senza la chiusura il
// COMMIT si rifiuta (prima restava una bozza aperta con una conferma: non abbandonabile, non riconfermabile).
func TestP4RevisioniLaBozzaSiChiudeConLaConferma(t *testing.T) {
	p := schema22ConStoria(t)
	conferma := `INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
			base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
			criterio_verifica, chiave_idempotenza, confermato_da)
		 SELECT '{T}', '{P2}', 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
			'{B1}', 1, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text), repeat('a', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}'`
	radice := `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo)
		 SELECT conferma_id, '{T}', '{P2}', true, 'k-p2', 'ACM-2000', 'Piastra', 'finito' FROM conferma_strutturale WHERE bozza_id = '{B1}'`
	rifiutoAtteso(t, p, "conferma con la bozza lasciata aperta", "P4X01", "non e' chiusa come confermata", conferma, radice)
	riuscita(t, p, "conferma che chiude la bozza", conferma, radice,
		`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{B1}'`)
}

// RD-02: in formato 2 la conferma porta mono_pezzo se e solo se lo snapshot confermato contiene la dichiarazione
// (D-P4A-03 A' [U]): niente conferma «finta» mono_pezzo = false con la dichiarazione, niente mono_pezzo senza.
func TestP4RevisioniMonoPezzoDichiaratoNelloSnapshot(t *testing.T) {
	p := schema22ConStoria(t)
	const step = "00000000-0000-4000-8000-0000002213d3"
	conStep := `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ('` + step + `', '{T}', '{P2}', 'cad_3d', 'ACM-2000', 'ACM-2000.stp', 'stp', repeat('7', 64), 'ACME22/ACM-2000.stp', '{U}');
		UPDATE componente SET step_strutturale_id = '` + step + `' WHERE componente_id = '{P2}'`
	testata := func(decisioni, mono, motivo, criterio string) string {
		return `INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
				base_contatore, firma_base, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, mono_pezzo, mono_pezzo_motivo,
				criterio_verifica, chiave_idempotenza, confermato_da)
			 SELECT '00000000-0000-4000-8000-0000002213c2', '{T}', '{P2}', 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
				COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
				'{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text, 'decisioni', '` + decisioni + `'::jsonb), repeat('e', 64),
				'{}', '{}', ` + mono + `, ` + motivo + `, '` + criterio + `', gen_random_uuid(), '{U}'`
	}
	radice := `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo, step_strutturale_id, step_sha256)
		VALUES ('00000000-0000-4000-8000-0000002213c2', '{T}', '{P2}', true, 'k-p2', 'ACM-2000', 'Piastra', 'finito', '` + step + `', repeat('7', 64))`
	dichiarazione := `[{"id": "d-1", "tipo": "mono_pezzo", "motivo": "piastra tagliata al laser"}]`
	rifiutoAtteso(t, p, "dichiarazione nello snapshot e conferma mono_pezzo = false", "23514", "ck_conferma_mono_pezzo_snapshot",
		conStep, testata(dichiarazione, "false", "NULL", "step"))
	rifiutoAtteso(t, p, "conferma mono_pezzo senza la dichiarazione nello snapshot", "23514", "ck_conferma_mono_pezzo_snapshot",
		conStep, testata(`[]`, "true", "'piastra tagliata al laser'", "mono_pezzo"))
	// la controprova: formato 2 con la dichiarazione, lo STEP autorizzato sulla radice, mono_pezzo
	riuscita(t, p, "mono_pezzo in formato 2 con la dichiarazione", conStep,
		testata(dichiarazione, "true", "'piastra tagliata al laser'", "mono_pezzo"), radice)
}

// RD-03: in formato 2 ogni arco certificato dice da dove viene (origine_gerarchia si calcola da qui).
func TestP4RevisioniArcoSenzaOrigine(t *testing.T) {
	p := schema22ConStoria(t)
	rifiutoAtteso(t, p, "arco formato 2 senza origine", "P4X01", "non dice da dove viene",
		conferma2("0", "0", "2", "nessuno"), radiceC2, figlioC2,
		`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta) VALUES ('{C2}', '{P}', '{S}', 2)`)
}

// La conferma n. 2 di P che chiude la bozza BF alla versione indicata, con il nodo nuovo N (codice dato) certificato
// con la chiave k-n.
func confermaBF(versione, impronta, codiceN string) []string {
	return []string{
		`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ('{N}', '{T}', '` + codiceN + `', 'sottoassieme', 'manuale', '{U}')`,
		`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P}', '{N}', 1, 'manuale', '{U}')`,
		`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
			firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot,
			impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da)
		 SELECT '{C2}', '{T}', '{P}', 2, '{C1}', 2, false, repeat('3', 64), repeat('2', 64), '{C1}', 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
			'{BF}', ` + versione + `, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text), repeat('` + impronta + `', 64), '{}', '{}',
			'nessuno', gen_random_uuid(), '{U}'`,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo) VALUES
			('{C2}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito'),
			('{C2}', '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme'),
			('{C2}', '{T}', '{N}', false, 'k-n', '` + codiceN + `', NULL, NULL, 'sottoassieme')`,
		`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ('{C2}', '{P}', '{S}', 2, 'conferma'), ('{C2}', '{P}', '{N}', 1, 'utente')`,
	}
}

const chiudiBF = `UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{BF}'`

// R-08: la risoluzione gira al COMMIT (trigger differito): un rifiuto temporaneo scritto nella transazione della
// conferma dopo i componenti certificati, a bozza ancora aperta, si risolve anche lui.
func TestP4RevisioniRisoluzioneAlCommit(t *testing.T) {
	p := schema22ConStoria(t)
	esegui11(t, p, bozzaBF)
	passi := append(confermaBF("1", "9", "ACM-1300"),
		eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-n'"}), chiudiBF)
	riuscita(t, p, "conferma con un rifiuto scritto dopo i componenti", passi...)
	if s := uno11[string](t, p, `SELECT coalesce(string_agg(r.azione || ' ' || r.componente_id::text, ' | '), 'nessuna')
		FROM fonte_step_evento e JOIN fonte_step_evento r ON r.risolve_evento_id = e.evento_id WHERE e.chiave_bozza = 'k-n' AND e.azione = 'rifiuto'`); s != "risoluzione "+e11("{N}") {
		t.Errorf("risoluzione del rifiuto scritto dopo i componenti: %s", s)
	}
}

// R-09 / RSC-02: una chiave tolta e riaggiunta per un nodo diverso dopo l'evento della fonte non porta la
// destinazione temporanea sul nodo nuovo: la conferma si rifiuta (P4X07). Riaggiunta identica: si risolve.
func TestP4RevisioniChiaveRiusataPerUnAltroNodo(t *testing.T) {
	p := schema22ConStoria(t)
	esegui11(t, p, bozzaBF)
	riuscita(t, p, "rifiuto sul sottoassieme nuovo k-n (versione 1)", eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-n'"}))
	versione2 := func(nodoNuovo string) []string {
		return []string{
			`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base, salvata_da)
			 VALUES ('{BF}', 2, jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text, 'nodi', jsonb_build_array(
				jsonb_build_object('chiave_bozza', 'k-p', 'componente_id', '{P}'::text, 'op', 'riusa'),
				jsonb_build_object('chiave_bozza', 'k-s', 'componente_id', '{S}'::text, 'op', 'riusa'),
				` + nodoNuovo + `)), repeat('7', 64), '{C1}', 1, 0, repeat('1', 64), '{U}')`,
			`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, chiave_bozza, prima, origine, utente_id)
			 VALUES ('{BF}', 1, 2, 'nodo_tolto', 'k-n', jsonb_build_object('chiave_bozza', 'k-n', 'op', 'crea', 'codice', 'ACM-1300'), 'utente', '{U}')`,
			`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, chiave_bozza, dopo, origine, utente_id)
			 VALUES ('{BF}', 2, 2, 'nodo_aggiunto', 'k-n', ` + nodoNuovo + `, 'utente', '{U}')`,
		}
	}
	diverso := `jsonb_build_object('chiave_bozza', 'k-n', 'op', 'crea', 'codice', 'ACM-1399', 'tipo', 'sciolto')`
	rifiutoAtteso(t, p, "chiave riusata per un nodo diverso", "P4X07", "riusata per un nodo diverso",
		append(append(versione2(diverso), confermaBF("2", "7", "ACM-1399")...), chiudiBF)...)
	if n := uno11[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'risoluzione'`); n != 0 {
		t.Errorf("risoluzioni dopo il rifiuto: %d", n)
	}
	identico := `jsonb_build_object('chiave_bozza', 'k-n', 'op', 'crea', 'codice', 'ACM-1300')`
	riuscita(t, p, "chiave tolta e rimessa con lo stesso nodo", append(append(versione2(identico), confermaBF("2", "7", "ACM-1300")...), chiudiBF)...)
	if n := uno11[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'risoluzione' AND componente_id = '{N}'`); n != 1 {
		t.Errorf("risoluzione con la chiave rimessa identica: %d, attesa 1", n)
	}
}

// RSC-05: ogni concessione e revoca di una deroga strutturale resta in deroga_struttura_storia, con la riga intera e
// l'autore; la storia la scrive solo il trigger.
func TestP4RevisioniStoriaDelleDerogheStrutturali(t *testing.T) {
	p := schema22ConStoria(t)
	U := e11("{U}")
	storia := func() string {
		return uno11[string](t, p, `SELECT coalesce(string_agg(operazione || ':' || coalesce(autore_id::text, '-') || ':' || (riga ->> 'motivo'), ' | ' ORDER BY storia_id), '')
			FROM deroga_struttura_storia WHERE componente_id = '{P}'`)
	}
	if s := storia(); s != "INSERT:"+U+":la gerarchia e' quella del PDF" {
		t.Errorf("storia della deroga concessa nelle righe di base: %s", s)
	}
	riuscita(t, p, "revoca con l'autore dichiarato", `SELECT set_config('cockpit.autore_id', '{U}', true)`,
		`DELETE FROM deroga_struttura WHERE componente_id = '{P}'`)
	riuscita(t, p, "concessione e revoca senza dichiarazione",
		`INSERT INTO deroga_struttura (thread_id, componente_id, step_documento_id, step_sha256, motivo_parziale, motivo, concessa_da)
		 VALUES ('{T}', '{P}', '{D}', repeat('c', 64), 'nessuna occorrenza di assieme', 'seconda deroga', '{U}')`)
	riuscita(t, p, "revoca senza dichiarazione", `DELETE FROM deroga_struttura WHERE componente_id = '{P}'`)
	want := fmt.Sprintf("INSERT:%s:la gerarchia e' quella del PDF | DELETE:%s:la gerarchia e' quella del PDF | INSERT:%s:seconda deroga | DELETE:-:seconda deroga", U, U, U)
	if s := storia(); s != want {
		t.Errorf("storia delle deroghe strutturali:\n %s\natteso\n %s", s, want)
	}
	if n := uno11[int](t, p, `SELECT count(*) FROM deroga_struttura WHERE componente_id = '{P}'`); n != 0 {
		t.Errorf("deroghe rimaste: %d", n)
	}
	rifiutoAtteso(t, p, "INSERT diretto nella storia", "P4X02", "la scrive solo il trigger",
		`INSERT INTO deroga_struttura_storia (operazione, deroga_struttura_id, thread_id, componente_id, step_documento_id, step_sha256, riga)
		 VALUES ('INSERT', gen_random_uuid(), '{T}', '{P}', '{D}', repeat('c', 64), '{}')`)
	if !uno11[bool](t, p, `SELECT bool_and(quando > now() - interval '1 hour') FROM deroga_struttura_storia`) {
		t.Error("la data della storia non e' quella del database")
	}
}

// R-04: la visita della vista e' sui nodi (UNION): con un ciclo nella working e un figlio condiviso la copertura vale
// ancora, e la lettura termina.
func TestP4RevisioniVistaConUnCiclo(t *testing.T) {
	p := schema22ConStoria(t)
	esegui11(t, p, scenaCoperture)
	c1 := "00000000-0000-4000-8000-000000221101"
	riuscita(t, p, "copertura di G1", copertura(map[string]string{"copertura_id": "'" + c1 + "'"}))
	esegui11(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES
		('{T}', '{G1}', '{G2}', 1, 'manuale', '{U}'), ('{T}', '{G2}', '{G1}', 1, 'manuale', '{U}')`)
	if s := statoDel2D(t, p, "{G1}"); s != coperto(c1) {
		t.Errorf("G1 coperto con un ciclo G1 ↔ G2 nella working: %s", s)
	}
	if s := statoDel2D(t, p, "{G2}"); !strings.HasPrefix(s, "manca") {
		t.Errorf("G2 senza copertura: %s", s)
	}
	bloccantiConcordi(t, p)
}
