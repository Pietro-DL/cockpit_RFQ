//go:build integrazione

// L4 — la variante 20261019c della 0023 (direttiva 39; PM23 §4.16, prove M-42 riscritta, M-49, M-50, M-16; P39-05 a
// livello di vincoli): la provenienza della PB sulla AR (copia verificata dallo snapshot confermato), le origini
// gesto_legacy e conversione_legacy con la guardia della RFQ formato 3, lo STEP legato come sorgente del finito al posto
// dell'esclusione per estensione, la revoca della sorgente, e l'ordine delle scritture di R5 (working → sostituzione A
// dopo B → testata → righe e archi → AR → chiusura della bozza → controlli al COMMIT) con il rollback iniettato.
// Righe di v1_0023_db_test.go (schema23ConStoria) e conferma C3 (nuovaC3). Solo dati sintetici (ACME, example.com).
//
// Provenienza: [U] direttiva 39 §2, §4, §5, §6, §7; [T-39] il predicato dello STEP sorgente, il segno della RFQ formato
// 3, la forma dell'evento conversione_legacy (scelte della corsia persistenza, resoconto della direttiva 39).

package migrazioni_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var sost39 = strings.NewReplacer(
	"{AB}", "00000000-0000-4000-8000-0000002339a1", // lo STEP B di P (sha 8…)
	"{AS}", "00000000-0000-4000-8000-0000002339a2", // un'altra occorrenza del contenuto del documento D (sha c…)
	"{DB}", "00000000-0000-4000-8000-0000002339d1", // il documento cad_3d di B su P
	"{RA}", "00000000-0000-4000-8000-0000002339b1", // una AR
	"{RL}", "00000000-0000-4000-8000-0000002339b2", // una AR di un gesto legacy
	"{KS}", "00000000-0000-4000-8000-0000002339c1", // la chiave di S8 (k2)
	"{KR}", "00000000-0000-4000-8000-0000002339c2", // la chiave della revoca della sorgente
	"{KL}", "00000000-0000-4000-8000-0000002339c3", // la chiave di un gesto legacy
)

// e39 sostituisce i segnaposto di questo file (gli altri li sostituiscono rifiuto23, riuscita23, esegui23 e uno23).
func e39(istruzioni ...string) []string {
	out := make([]string, len(istruzioni))
	for i, s := range istruzioni {
		out[i] = sostAR.Replace(sost39.Replace(s))
	}
	return out
}

// stepB: lo STEP B di P (allegato AB, sha 8…) e il suo documento cad_3d DB; con s8, la conferma della sorgente di S8
// (conferma_documento con la chiave k2).
func stepB(s8 bool) []string {
	out := []string{
		`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il)
		 VALUES ('{AB}', '{M}', 11, 'ACM-1000_B.stp', 'stp', repeat('8', 64), now())`,
		`INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da)
		 VALUES ('{DB}', '{T}', '{P}', 'cad_3d', 'ACM-1000', 'B', 'ACM-1000_B.stp', 'stp', repeat('8', 64), 'ACME22/ACM-1000_B.stp', '{U}')`,
	}
	if s8 {
		out = append(out, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id,
			chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{P}', 'prodotto', 'conferma_documento', '{AB}', repeat('8', 64), 'bom', '{DB}', '{KS}', '{U}')`)
	}
	return e39(out...)
}

// revocaB: la revoca della sorgente B (azione revoca_sorgente, con il documento, la chiave e il motivo).
var revocaB = `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id, motivo,
	chiave_idempotenza, autore_id)
 VALUES ('{T}', '{P}', 'prodotto', 'revoca_sorgente', '{AB}', repeat('8', 64), 'bom', '{DB}', 'STEP scelto per errore', '{KR}', '{U}')`

// legatoComeSorgente: il predicato del database per un allegato della RFQ (thread), nella transazione di tx.
func legatoComeSorgente(t *testing.T, p *pgxpool.Pool, prima []string, thread, allegato string) bool {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range prima {
		if _, err := tx.Exec(ctx, e23(s)); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	var v bool
	if err := tx.QueryRow(ctx, e23(sost39.Replace(`SELECT allegato_sorgente_del_finito('`+thread+`', '`+allegato+`')`))).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// pb39: una PB formato 3 con la provenienza e, se data, l'importazione d'origine (impronta).
func pb39(allegato, chiave, provenienza, importazione string) map[string]any {
	m := map[string]any{"allegato_id": sostAR.Replace(sost39.Replace(u23(allegato))), "chiave_bozza": chiave, "provenienza": provenienza}
	if importazione != "" {
		m["importazione"] = importazione
	}
	return m
}

// nascita39: la AR di R5 con la provenienza della PB e il registro d'origine (espressione SQL o NULL), e il suo evento.
func nascita39(id, allegato, componente, chiave, provenienza, importazione string) []string {
	return e39(
		`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
			provenienza_pb, importazione_id, chiave_idempotenza, creata_da)
		 VALUES ('`+id+`', '{T}', '`+allegato+`', '`+componente+`', 'conferma_bom', '{C3}', '`+chiave+`', `+provenienza+`, `+importazione+`, '{K3}', '{U}')`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_dopo, conferma_id, origine,
			chiave_idempotenza, autore_id)
		 VALUES ('{T}', '`+allegato+`', '`+id+`', 'preassociazione', '`+componente+`', '{C3}', 'conferma_bom', '{K3}', '{U}')`)
}

// M-42 riscritta (D-2 (c) [U], direttiva 39 §5): il predicato dello STEP legato come sorgente del finito, ramo per ramo,
// e la nascita della AR: uno STEP documentale si associa, lo STEP sorgente no (anche come AR sul suo stesso finito).
func TestV1D39M42LoStepSorgenteELoStepDocumentale(t *testing.T) {
	p := schema23ConStoria(t)
	altraOccorrenzaDiD := e39(`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il)
		VALUES ('{AS}', '{M}', 12, 'ACM-1000_c.stp', 'stp', repeat('c', 64), now())`)
	// (d) A e' la sorgente della bozza aperta B3; A4 e' lo STEP di una parte; AX e' di un'altra RFQ
	if !legatoComeSorgente(t, p, nil, "{T}", "{A}") {
		t.Error("(d) la sorgente della bozza aperta non risulta legata come sorgente")
	}
	if legatoComeSorgente(t, p, nil, "{T}", "{A4}") {
		t.Error("uno STEP di una parte risulta legato come sorgente")
	}
	if legatoComeSorgente(t, p, nil, "{T}", "{AX}") || legatoComeSorgente(t, p, nil, "{T2}", "{AX}") {
		t.Error("AX (RFQ T2, stesso contenuto di A) risulta legato come sorgente")
	}
	// (b) il legame storico: il contenuto del documento cad_3d D del finito P, senza conferme di S8
	if !legatoComeSorgente(t, p, altraOccorrenzaDiD, "{T}", "{AS}") {
		t.Error("(b) il documento STEP storico del finito non risulta legato come sorgente")
	}
	// (a) la conferma S8 di B la lega; la revoca della sorgente la scioglie
	if !legatoComeSorgente(t, p, stepB(true), "{T}", "{AB}") {
		t.Error("(a) lo STEP confermato da S8 non risulta legato come sorgente")
	}
	if legatoComeSorgente(t, p, append(stepB(true), e39(revocaB)...), "{T}", "{AB}") {
		t.Error("(a) lo STEP revocato risulta ancora legato come sorgente")
	}
	// (b) solo per un prodotto senza conferme di S8: con la conferma S8 di B il documento storico D non e' piu' il legame
	// storico; (c) ma se e' lo STEP strutturale della working lo e'
	conS8 := append(append([]string{}, altraOccorrenzaDiD...), stepB(true)...)
	if legatoComeSorgente(t, p, conS8, "{T}", "{AS}") {
		t.Error("(b) il documento storico di un prodotto con una conferma S8 risulta legato come sorgente")
	}
	if !legatoComeSorgente(t, p, append(conS8, `UPDATE componente SET step_strutturale_id = '{D}' WHERE componente_id = '{P}'`), "{T}", "{AS}") {
		t.Error("(c) lo STEP strutturale della working non risulta legato come sorgente")
	}
	// (a) una conferma legacy (senza chiave) lega anche lei; un documento sostituito non lega piu'
	legacy := e39(`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, autore_id)
		VALUES ('{T}', '{P}', 'prodotto', 'conferma_documento', '{AS}', repeat('c', 64), '{D}', '{U}')`)
	if !legatoComeSorgente(t, p, append(append(append([]string{}, altraOccorrenzaDiD...), stepB(true)...), legacy...), "{T}", "{AS}") {
		t.Error("(a) la conferma legacy della fonte non lega lo STEP")
	}
	if legatoComeSorgente(t, p, append(append(append(append([]string{}, altraOccorrenzaDiD...), stepB(true)...), legacy...),
		e39(`UPDATE documento SET sostituito_da = '{DB}' WHERE documento_id = '{D}'`)...), "{T}", "{AS}") {
		t.Error("(a) il documento sostituito lega ancora lo STEP")
	}

	// la nascita: lo STEP documentale A4 sul telaio S passa; lo STEP B confermato da S8, anche sul suo finito P, no (P4X10)
	passaAiControlli(t, p, "STEP documentale come AR",
		c3ConAR([]any{pb("{A4}", "k-s")}, nascita("{AR1}", "{A4}", "{S}", "k-s", "{C3}", "{K3}")...).istruzioni(t)...)
	sorgente := c3ConAR([]any{pb39("{AB}", "k-p", "manuale", "")}, nascita39("{RA}", "{AB}", "{P}", "k-p", "'manuale'", "NULL")...)
	sorgente.prima = stepB(true)
	rifiuto23(t, p, "lo STEP sorgente confermato da S8 non diventa AR", "P4X10", "legato come sorgente strutturale", sorgente.istruzioni(t)...)
	// revocata, lo STEP B si associa come documento del suo finito (il documento DB e' gia' di P: il punto 5 passa)
	revocata := c3ConAR([]any{pb39("{AB}", "k-p", "manuale", "")}, nascita39("{RA}", "{AB}", "{P}", "k-p", "'manuale'", "NULL")...)
	revocata.prima = append(stepB(true), e39(revocaB)...)
	passaAiControlli(t, p, "STEP revocato come AR del suo finito", revocata.istruzioni(t)...)
}

// M-49 (D-PB03 A [U], direttiva 39 §4): la provenienza della PB e il registro d'origine sulla AR, copia verificata dallo
// snapshot confermato; il round-trip; una PB rimasta da un registro A dopo il passaggio a B; l'immutabilita'.
func TestV1D39M49LaProvenienzaSullaAR(t *testing.T) {
	p := schema23ConStoria(t)
	r1, r2, r3 := sha("6"), sha("e"), sha("f") // R1 e R2 di P, R3 di P2
	conPB := func(pbj map[string]any, prov, imp string) []string {
		return c3ConAR([]any{pbj}, nascita39("{RA}", "{A3}", "{S}", "k-s", prov, imp)...).istruzioni(t)
	}
	rifiuto23(t, p, "provenienza diversa dalla PB", "P4X10", "provenienza_pb manuale, ma la PB confermata ha la provenienza motore",
		conPB(pb39("{A3}", "k-s", "motore", r1), "'manuale'", "NULL")...)
	rifiuto23(t, p, "registro mancante sulla AR", "P4X10", "importazione_id -, ma la PB confermata cita il registro",
		conPB(pb39("{A3}", "k-s", "motore", r1), "'motore'", "NULL")...)
	rifiuto23(t, p, "registro diverso sulla AR", "P4X10", "ma la PB confermata cita il registro",
		conPB(pb39("{A3}", "k-s", "motore", r1), "'motore'", "'{R2}'")...)
	rifiuto23(t, p, "registro sulla AR senza registro nella PB", "P4X10", "ma la PB confermata cita il registro -",
		conPB(pb39("{A3}", "k-s", "corretta", ""), "'corretta'", "'{R1}'")...)
	rifiuto23(t, p, "la PB cita un registro di un altro prodotto", "P4X10", "che non e' del prodotto della conferma",
		conPB(pb39("{A3}", "k-s", "motore", r3), "'motore'", "'{R3}'")...)
	// la conferma vuole la provenienza di ogni PB (ck_conferma_preassociazioni)
	rifiuto23(t, p, "PB confermata senza provenienza", "23514", "ck_conferma_preassociazioni",
		c3ConAR([]any{map[string]any{"allegato_id": u23("{A3}"), "chiave_bozza": "k-s"}},
			nascita39("{RA}", "{A3}", "{S}", "k-s", "'manuale'", "NULL")...).istruzioni(t)...)
	// una PB rimasta dal registro R2 (la fonte di prima) in una conferma che chiude la versione sul registro R1: la AR
	// porta R2, non la fonte della conferma (un cambio della fonte non riscrive l'importazione d'origine)
	passaAiControlli(t, p, "PB rimasta da un registro precedente", conPB(pb39("{A3}", "k-s", "motore", r2), "'motore'", "'{R2}'")...)
	// il round-trip: snapshot → AR → snapshot, con una PB corretta del registro R1
	riuscita23(t, p, "conferma con la PB corretta", conPB(pb39("{A3}", "k-s", "corretta", r1), "'corretta'", "'{R1}'")...)
	var prov, imp, provSnap, impSnap string
	if err := p.QueryRow(context.Background(), e23(sost39.Replace(`SELECT r.provenienza_pb, r.importazione_id::text, d ->> 'provenienza', x.importazione_id::text
		FROM file_rfq_associazione r JOIN conferma_strutturale c ON c.conferma_id = r.conferma_id
		CROSS JOIN LATERAL jsonb_array_elements(c.snapshot -> 'documenti_preassociati') d
		JOIN distinta_importazione x ON x.thread_id = r.thread_id AND x.prodotto_id = c.prodotto_id AND x.impronta::text = d ->> 'importazione'
		WHERE r.associazione_id = '{RA}' AND d ->> 'allegato_id' = r.allegato_id::text AND d ->> 'chiave_bozza' = r.chiave_bozza`))).
		Scan(&prov, &imp, &provSnap, &impSnap); err != nil {
		t.Fatal(err)
	}
	if prov != "corretta" || prov != provSnap || imp != u23("{R1}") || imp != impSnap {
		t.Errorf("round-trip: AR (%s, %s), snapshot (%s, %s)", prov, imp, provSnap, impSnap)
	}
	// immutabili
	rifiuto23(t, p, "provenienza_pb non cambia", "P4X10", "identita', atto e nascita non cambiano",
		e39(`UPDATE file_rfq_associazione SET provenienza_pb = 'manuale' WHERE associazione_id = '{RA}'`)...)
	rifiuto23(t, p, "importazione_id non cambia", "P4X10", "identita', atto e nascita non cambiano",
		e39(`UPDATE file_rfq_associazione SET importazione_id = '{R2}' WHERE associazione_id = '{RA}'`)...)
}

// M-50 (D-AR03 D, D-AR04 [U]; D-AR14 A [T-39]): gesto_legacy solo in una RFQ interamente legacy, anche dopo l'abbandono
// della bozza formato 3; lo sgancio legacy; la conversione di una pre-assegnazione legacy con l'autore nullo, l'unica.
func TestV1D39M50LeOriginiLegacy(t *testing.T) {
	p := schema23ConStoria(t)
	// il segno: T e' formato 3 dalla bozza B3, T2 no; il segno non si cambia, non si cancella, non si svuota
	if !uno23[bool](t, p, `SELECT rfq_e_formato3('{T}')`) || uno23[bool](t, p, `SELECT rfq_e_formato3('{T2}')`) {
		t.Fatal("rfq_e_formato3: attesa T formato 3 e T2 legacy")
	}
	if uno23[string](t, p, `SELECT bozza_id::text FROM rfq_formato3_segno WHERE thread_id = '{T}'`) != u23("{B3}") {
		t.Error("il segno di T non cita la bozza B3")
	}
	rifiuto23(t, p, "il segno non si cancella", "P4X10", "non si cambia e non si cancella", `DELETE FROM rfq_formato3_segno`)
	rifiuto23(t, p, "il segno non si cambia", "P4X10", "non si cambia e non si cancella", `UPDATE rfq_formato3_segno SET dal = now()`)
	rifiuto23(t, p, "i segni non si svuotano", "P4X10", "non si svuotano", `TRUNCATE rfq_formato3_segno`)

	legacyT2 := e39(
		`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, chiave_idempotenza, creata_da)
		 VALUES ('{RL}', '{T2}', '{AX}', '{Q}', 'gesto_legacy', '{KL}', '{U}')`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_dopo, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T2}', '{AX}', '{RL}', 'assegna', '{Q}', 'legacy', '{KL}', '{U}')`)
	// gesto_legacy: una nascita «assegna», non una preassociazione; mai in una RFQ formato 3
	rifiuto23(t, p, "nascita legacy come preassociazione", "P4X10", "non e' quella del suo atto",
		legacyT2[0], strings.Replace(strings.Replace(legacyT2[1], "'assegna'", "'preassociazione'", 1), "'legacy'", "'conferma_bom'", 1))
	legacyT := e39(
		`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, chiave_idempotenza, creata_da)
		 VALUES ('{RL}', '{T}', '{A3}', '{S}', 'gesto_legacy', '{KL}', '{U}')`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_dopo, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{A3}', '{RL}', 'assegna', '{S}', 'legacy', '{KL}', '{U}')`)
	rifiuto23(t, p, "gesto legacy in una RFQ formato 3", "P4X10", "e' formato 3", legacyT...)
	rifiuto23(t, p, "gesto legacy dopo l'abbandono della bozza formato 3", "P4X10", "e' formato 3",
		append([]string{`UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}' WHERE bozza_id = '{B3}'`}, legacyT...)...)
	// una AR di R5 non nasce con un «assegna»
	rifiuto23(t, p, "nascita di R5 come assegna", "P4X10", "non e' quella del suo atto",
		c3ConAR([]any{pb39("{A3}", "k-s", "manuale", "")}, nascita39("{RA}", "{A3}", "{S}", "k-s", "'manuale'", "NULL")[0],
			e39(`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_dopo, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{A3}', '{RA}', 'assegna', '{S}', 'legacy', '{K3}', '{U}')`)[0]).istruzioni(t)...)
	// in T2 (interamente legacy) il gesto legacy nasce, e «Togli» lo chiude con sgancio_legacy
	riuscita23(t, p, "gesto legacy in una RFQ interamente legacy", legacyT2...)
	if n := uno23[int](t, p, sost39.Replace(`SELECT count(*) FROM file_rfq_associazione WHERE associazione_id = '{RL}' AND origine = 'gesto_legacy'
		AND conferma_id IS NULL AND provenienza_pb IS NULL AND creata_da = '{U}'`)); n != 1 {
		t.Fatalf("AR legacy: %d", n)
	}
	riuscita23(t, p, "sgancio legacy", e39(
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'sgancio_legacy', chiusa_chiave = '{KR}'
		 WHERE associazione_id = '{RL}'`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T2}', '{AX}', '{RL}', 'sgancia', '{Q}', 'legacy', '{KR}', '{U}')`)...)
	// «Togli» di una AR in una RFQ formato 3: rifiutato (la AR di R5 nella stessa transazione della conferma)
	sgancioT := c3ConAR([]any{pb39("{A3}", "k-s", "manuale", "")}, append(nascita39("{RA}", "{A3}", "{S}", "k-s", "'manuale'", "NULL"),
		e39(`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'sgancio_legacy' WHERE associazione_id = '{RA}'`)...)...)
	rifiuto23(t, p, "sgancio legacy in una RFQ formato 3", "P4X10", "la RFQ e' formato 3", sgancioT.istruzioni(t)...)
	// conversione_legacy: l'evento della conversione di una pre-assegnazione (la copia di DP su S, senza AR), senza autore
	riuscita23(t, p, "conversione di una pre-assegnazione", copiaDP("{S}", "ACM-1100"),
		`INSERT INTO associazione_evento (thread_id, proposta_id, allegato_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{DP}', '{A2}', 'sgancia', '{S}', 'conversione_legacy', NULL)`,
		copiaDP("", ""))
	rifiuto23(t, p, "un'altra origine senza autore", "23514", "ck_associazione_autore",
		`INSERT INTO associazione_evento (thread_id, proposta_id, allegato_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{DP}', '{A2}', 'sgancia', '{S}', 'legacy', NULL)`)
	rifiuto23(t, p, "conversione_legacy con una AR", "23514", "ck_associazione_coerente",
		e39(`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T2}', '{AX}', '{RL}', 'sgancia', '{Q}', 'conversione_legacy', NULL)`)...)
}

// M-16 (direttiva 39 §2; R39-10, R39-11 [T-39]): il gesto revoca_sorgente in esito_intento e l'azione revoca_sorgente
// della storia della fonte, con il documento, la chiave e il motivo.
func TestV1D39M16LaRevocaDellaSorgente(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "conferma S8 e revoca della sorgente", append(stepB(true), e39(revocaB,
		`INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito)
		 VALUES ('{KR}', '{T}', 'revoca_sorgente', '{U}', repeat('a', 64), '{"esito": "revocata"}')`)...)...)
	if n := uno23[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'revoca_sorgente'`); n != 1 {
		t.Errorf("revoche della sorgente: %d", n)
	}
	rifiuto23(t, p, "revoca della sorgente senza motivo", "23514", "ck_fonte_revoca_sorgente",
		e39(strings.Replace(revocaB, "'STEP scelto per errore'", "' '", 1))...)
	rifiuto23(t, p, "revoca della sorgente senza chiave", "23514", "ck_fonte_revoca_sorgente",
		e39(strings.Replace(revocaB, "'{KR}'", "NULL", 1))...)
}

// L'ordine delle scritture di R5 sul database (direttiva 39 §2, §7 [U + T-39]; P39-05 a livello di vincoli): la working
// (promozione di B in step_strutturale_id, con l'evento autorizzazione), la sostituzione documentale A → B dopo, la
// testata, le righe e gli archi certificati (criterio step: lo STEP certificato e' B), la AR di uno STEP documentale con
// la sua provenienza, la chiusura della bozza, i controlli al COMMIT. Gli ordini sbagliati si fermano; un errore
// iniettato fra promozione, sostituzione, AR e conferma non lascia niente.
func TestV1D39LOrdineDiR5SulDatabase(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "lo STEP B confermato da S8", stepB(true)...)
	promozione := e39(`UPDATE componente SET step_strutturale_id = '{DB}' WHERE componente_id = '{P}'`,
		`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id, motivo,
			chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{P}', 'prodotto', 'autorizzazione', '{AB}', repeat('8', 64), 'bom', '{DB}', 'promossa con la conferma n. 2', '{K3}', '{U}')`)
	sostituzione := `UPDATE documento SET sostituito_da = '` + sost39.Replace("{DB}") + `' WHERE documento_id = '{D}'`
	certificati := []string{
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo,
			emittente, emittente_presunto, namespace, step_strutturale_id, step_sha256)
		 VALUES ('{C3}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito', 'cliente:{C}', true, NULL, '` + sost39.Replace("{DB}") + `', repeat('8', 64)),
		        ('{C3}', '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme', 'cliente:{C}', true, NULL, NULL, NULL)`,
		`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ` + arcoC3,
		`INSERT INTO conferma_strutturale_non_incluso (conferma_id, thread_id, sha256, chiave, allegato_id, gruppo, decisione_id,
			azione_id, motivo_codice, motivo, segnale) VALUES ` + nonInclusoC3,
	}
	ar := nascita39("{RA}", "{A4}", "{S}", "k-s", "'motore'", "'{R1}'")
	r5 := func(prima, dopo []string) confermaC3 {
		c := nuovaC3()
		c.snap["documenti_preassociati"] = []any{pb39("{A4}", "k-s", "motore", sha("6"))}
		c.testata["criterio_verifica"] = "'step'"
		c.componenti, c.archi, c.nonInclusi = nil, nil, nil
		c.prima = prima
		c.dopo = dopo
		return c
	}
	valida := func() []string { return append(append([]string{}, certificati...), ar...) }
	stato := func() string {
		return uno23[string](t, p, sost39.Replace(`SELECT concat_ws('|',
			(SELECT COALESCE(step_strutturale_id::text, '-') FROM componente WHERE componente_id = '{P}'),
			(SELECT COALESCE(sostituito_da::text, '-') FROM documento WHERE documento_id = '{D}'),
			(SELECT count(*) FROM conferma_strutturale WHERE conferma_id = '{C3}'),
			(SELECT count(*) FROM file_rfq_associazione),
			(SELECT count(*) FROM fonte_step_evento WHERE azione = 'autorizzazione'),
			(SELECT stato FROM distinta_bozza WHERE bozza_id = '{B3}'))`))
	}
	prima := stato()
	if prima != "-|-|0|0|0|aperta" {
		t.Fatalf("stato di partenza: %s", prima)
	}
	// gli ordini sbagliati: la promozione dopo la testata (P4X01); il criterio step senza la promozione (P4X01 al
	// COMMIT); la AR prima della testata (la FK della conferma)
	rifiuto23(t, p, "promozione dopo la testata", "P4X01", "dopo una conferma strutturale",
		r5(nil, append(promozione, valida()...)).istruzioni(t)...)
	rifiuto23(t, p, "criterio step senza la promozione", "P4X01", "",
		r5(nil, append(e39(strings.Replace(certificati[0], "'"+sost39.Replace("{DB}")+"', repeat('8', 64)", "NULL, NULL", 1)), append(certificati[1:], ar...)...)).istruzioni(t)...)
	rifiuto23(t, p, "AR prima della testata", "23503", "fk_fra_conferma",
		r5(append(append(append([]string{}, promozione...), sostituzione), ar...), certificati).istruzioni(t)...)
	// l'errore iniettato fra promozione, sostituzione, AR e conferma annulla tutto
	errore := `SELECT 1 / 0`
	for nome, istr := range map[string][]string{
		"dopo la promozione":   r5(append(append([]string{}, promozione...), errore), valida()).istruzioni(t),
		"dopo la sostituzione": r5(append(append([]string{}, promozione...), sostituzione, errore), valida()).istruzioni(t),
		"dopo la testata":      r5(append(append([]string{}, promozione...), sostituzione), append([]string{errore}, valida()...)).istruzioni(t),
		"dopo la AR":           r5(append(append([]string{}, promozione...), sostituzione), append(valida(), errore)).istruzioni(t),
	} {
		rifiuto23(t, p, "errore iniettato "+nome, "22012", "", istr...)
		if s := stato(); s != prima {
			t.Errorf("errore iniettato %s: lo stato e' cambiato (%s, prima %s)", nome, s, prima)
		}
	}
	// l'ordine giusto passa tutto insieme
	riuscita23(t, p, "R5 nell'ordine della direttiva", r5(append(append([]string{}, promozione...), sostituzione), valida()).istruzioni(t)...)
	atteso := sost39.Replace("{DB}|{DB}|1|1|1|confermata")
	if s := stato(); s != atteso {
		t.Errorf("dopo R5: %s, atteso %s", s, atteso)
	}
	if v := uno23[string](t, p, sost39.Replace(`SELECT concat_ws('|', r.provenienza_pb, r.importazione_id::text, c.criterio_verifica, k.step_strutturale_id::text)
		FROM file_rfq_associazione r JOIN conferma_strutturale c ON c.conferma_id = r.conferma_id
		JOIN conferma_strutturale_componente k ON k.conferma_id = c.conferma_id AND k.radice
		WHERE r.associazione_id = '{RA}'`)); v != sost39.Replace("motore|"+u23("{R1}")+"|step|{DB}") {
		t.Errorf("AR e conferma: %s", v)
	}
}
