//go:build integrazione

// L4 — l'emendamento della 0022 voluto dalla direttiva 11 (giro 5, A1d, P4; contratto rev. 3, Emendamento 2):
// la copertura documentale condivisa e v_fascicolo ridefinita (E2.5, E2.6, correzione D1), il registro permanente
// delle bozze (E2.10, P-04a), le fonti STEP secondarie con la destinazione temporanea e la risoluzione al COMMIT
// (E2.2), la protezione dalle due varianti della 0022 sullo stesso database; in piu' le prove che uccidono i
// mutanti S* sopravvissuti alla revisione delle prove B1/Q1 (scratchpad mut_b1). Solo dati sintetici (ACME,
// example.com). Le righe di base sono quelle di p4_0022_db_test.go (schema22ConStoria).

package migrazioni_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

var sost11 = strings.NewReplacer(
	"{G1}", "00000000-0000-4000-8000-0000002211a1",
	"{G2}", "00000000-0000-4000-8000-0000002211a2",
	"{G3}", "00000000-0000-4000-8000-0000002211a3",
	"{N}", "00000000-0000-4000-8000-0000002211e1",
	"{DG}", "00000000-0000-4000-8000-0000002211d1",
	"{DG3}", "00000000-0000-4000-8000-0000002211d3",
	"{DQ}", "00000000-0000-4000-8000-0000002211d9",
	"{C2}", "00000000-0000-4000-8000-0000002211c2",
	"{BF}", "00000000-0000-4000-8000-0000002211b1",
	"{BX}", "00000000-0000-4000-8000-0000002211b2",
	"{BR}", "00000000-0000-4000-8000-0000002211b3",
)

func e11(sql string) string { return sost22.Replace(sost11.Replace(sql)) }

func u11(nome string) uuid.UUID { return uuid.MustParse(e11(nome)) }

func esegui11(t *testing.T, p *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := p.Exec(context.Background(), e11(sql)); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func uno11[T any](t *testing.T, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sql string) T {
	t.Helper()
	var v T
	if err := q.QueryRow(context.Background(), e11(sql)).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

// rifiutoAtteso esegue le istruzioni in una transazione e vuole il rifiuto con il codice (SQLSTATE o P4Xnn) e la
// frase attesi, sull'istruzione o al COMMIT (i trigger differiti). La transazione si annulla comunque.
func rifiutoAtteso(t *testing.T, p *pgxpool.Pool, nome, codiceAtteso, frase string, istruzioni ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range istruzioni {
		if _, err = tx.Exec(ctx, e11(s)); err != nil {
			break
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if codice(err) != codiceAtteso || !strings.Contains(err.Error(), frase) {
		t.Errorf("%s: atteso %s («%s»), ottenuto %v", nome, codiceAtteso, frase, err)
	}
}

// riuscita esegue le istruzioni in una transazione e la conferma.
func riuscita(t *testing.T, p *pgxpool.Pool, nome string, istruzioni ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range istruzioni {
		if _, err := tx.Exec(ctx, e11(s)); err != nil {
			t.Fatalf("%s: %v\n%s", nome, err, s)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("%s: al COMMIT %v", nome, err)
	}
}

// ------------------------------------------------------------------ coperture e v_fascicolo

// scenaCoperture: sulle righe di base, tre figli sciolti di P (G1, G2, G3) e il PDF multipagina di P (DG, il
// disegno_2d del finito, rev B2, 16 pagine), piu' un 2D di Q nell'altra RFQ (DQ).
const scenaCoperture = `
INSERT INTO componente (componente_id, thread_id, codice, rev, descrizione, tipo, origine, confermato_da) VALUES
  ('{G1}', '{T}', 'ACM-1201', NULL, 'Staffa', 'sciolto', 'manuale', '{U}'),
  ('{G2}', '{T}', 'ACM-1202', NULL, 'Piatto', 'sciolto', 'manuale', '{U}'),
  ('{G3}', '{T}', 'ACM-1203', NULL, 'Perno', 'sciolto', 'manuale', '{U}');
INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES
  ('{T}', '{P}', '{G1}', 1, 'manuale', '{U}'), ('{T}', '{P}', '{G2}', 2, 'manuale', '{U}'), ('{T}', '{P}', '{G3}', 4, 'manuale', '{U}');
INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da) VALUES
  ('{DG}', '{T}', '{P}', 'disegno_2d', 'ACM-1000', 'B2', 'acm-1000_b2.pdf', 'pdf', repeat('5', 64), 'ACME22/ACM-1000_REV_B2.pdf', '{U}'),
  ('{DQ}', '{T2}', '{Q}', 'disegno_2d', 'ACM-9000', NULL, 'acm-9000.pdf', 'pdf', repeat('4', 64), 'ACME22/ACM-9000.pdf', '{U}');`

// copertura: una riga valida di copertura_documento sul PDF di P; cambi sostituisce colonne (espressioni SQL).
func copertura(cambi map[string]string) string {
	base := []struct{ col, expr string }{
		{"copertura_id", "gen_random_uuid()"}, {"thread_id", "'{T}'"}, {"documento_id", "'{DG}'"}, {"sha256", "repeat('5', 64)"},
		{"documento_componente", "'{P}'"}, {"componente_id", "'{G1}'"}, {"tipo", "'disegno_2d'"}, {"azione", "'copre'"},
		{"precedente_id", "NULL"}, {"pagine", "'{10,11}'"}, {"pagine_totali", "16"}, {"rev_documento", "'B2'"},
		{"rev_componente", "NULL"}, {"motivo", "'il disegno del figlio e'' nel PDF del finito'"}, {"chiave_idempotenza", "gen_random_uuid()"},
		{"autore_id", "'{U}'"},
	}
	var cols, vals []string
	for _, c := range base {
		v := c.expr
		if x, ok := cambi[c.col]; ok {
			v = x
		}
		cols = append(cols, c.col)
		vals = append(vals, v)
	}
	if q, ok := cambi["quando"]; ok {
		cols = append(cols, "quando")
		vals = append(vals, q)
	}
	return fmt.Sprintf("INSERT INTO copertura_documento (%s) VALUES (%s)", strings.Join(cols, ", "), strings.Join(vals, ", "))
}

// statoDel2D: esito, documento e copertura della riga disegno_2d del componente in v_fascicolo; «assente» se la
// vista non ha la riga (componente archiviato).
func statoDel2D(t *testing.T, p *pgxpool.Pool, comp string) string {
	t.Helper()
	return uno11[string](t, p, `SELECT coalesce((SELECT esito || ' ' || coalesce(documento_id::text, '-') || ' ' || coalesce(copertura_id::text, '-')
		FROM v_fascicolo WHERE componente_id = '`+comp+`' AND tipo_documento = 'disegno_2d'), 'assente')`)
}

func coperto(id string) string { return "ok_in_coda " + e11("{DG}") + " " + e11(id) }
func scoperto() string         { return "manca - -" }

// bloccantiConcordi: v_thread_bloccanti, v_cruscotto, ContaBloccantiLogici (il gate del congelamento) e il conto
// in Go delle righe di ListFascicolo (il Fascicolo legacy) dicono la stessa cosa per la RFQ T; torna il conteggio
// logico.
func bloccantiConcordi(t *testing.T, p *pgxpool.Pool) int32 {
	t.Helper()
	ctx := context.Background()
	q := db.New(p)
	T := u22("{T}")
	gate, err := q.ContaBloccantiLogici(ctx, T)
	if err != nil {
		t.Fatal(err)
	}
	righe, err := q.ListFascicolo(ctx, T)
	if err != nil {
		t.Fatal(err)
	}
	var logici, fisici int32
	for _, r := range righe {
		if !r.Bloccante {
			continue
		}
		switch r.Esito {
		case "ok", "ok_in_coda", "derogato":
		case "ok_errore_nas":
			fisici++
		default:
			logici++
			fisici++
		}
	}
	if gate != logici {
		t.Errorf("ContaBloccantiLogici %d, ListFascicolo %d: il gate e il Fascicolo leggono definizioni diverse", gate, logici)
	}
	vista := uno11[int64](t, p, `SELECT n_bloccanti FROM v_thread_bloccanti WHERE thread_id = '{T}'`)
	cruscotto := uno11[int64](t, p, `SELECT n_bloccanti FROM v_cruscotto WHERE thread_id = '{T}'`)
	if vista != int64(fisici) || cruscotto != vista {
		t.Errorf("v_thread_bloccanti %d, v_cruscotto %d, ListFascicolo (con ok_errore_nas) %d: non concordano", vista, cruscotto, fisici)
	}
	return gate
}

// D-SHOCK-DOC-01 A, DOC-02 A [U] e la correzione D1: una pagina condivisa da piu' figli, un figlio su piu' pagine,
// un figlio senza 2D; rettifica, revoca, ripristino; rev del componente, archiviazione, perimetro, proprietario e
// tipo del documento, sostituzione con una revisione nuova; identita' condivisa fra due prodotti. Ogni passo:
// v_fascicolo, v_thread_bloccanti, v_cruscotto, ListFascicolo e ContaBloccantiLogici concordi.
func TestP4CopertureEVFascicolo(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	esegui11(t, p, scenaCoperture)
	q := db.New(p)
	documentiPrima := testutil.Conta(t, p, "documento")
	b0 := bloccantiConcordi(t, p)
	for _, g := range []string{"{G1}", "{G2}", "{G3}"} {
		if s := statoDel2D(t, p, g); s != scoperto() {
			t.Fatalf("prima delle coperture %s: %s", g, s)
		}
	}

	// G1 sulle pagine 10 e 11 (un figlio su piu' pagine), G2 sulla 10 (pagina condivisa); G3 resta senza 2D
	c1, c2 := "00000000-0000-4000-8000-000000221101", "00000000-0000-4000-8000-000000221102"
	riuscita(t, p, "due coperture", copertura(map[string]string{"copertura_id": "'" + c1 + "'"}),
		copertura(map[string]string{"copertura_id": "'" + c2 + "'", "componente_id": "'{G2}'", "pagine": "'{10}'"}))
	if s := statoDel2D(t, p, "{G1}"); s != coperto(c1) {
		t.Errorf("G1 coperto: %s", s)
	}
	if s := statoDel2D(t, p, "{G2}"); s != coperto(c2) {
		t.Errorf("G2 coperto: %s", s)
	}
	if s := statoDel2D(t, p, "{G3}"); s != scoperto() {
		t.Errorf("G3 senza copertura deve restare manca: %s", s)
	}
	// il proprietario vede il suo documento, senza copertura; rev_diversa resta falsa per i coperti
	if s := statoDel2D(t, p, "{P}"); s != "ok_in_coda "+e11("{DG}")+" -" {
		t.Errorf("il 2D del prodotto: %s", s)
	}
	if uno11[bool](t, p, `SELECT bool_or(rev_diversa) FROM v_fascicolo WHERE componente_id IN ('{G1}', '{G2}')`) {
		t.Error("rev_diversa vera per un requisito coperto")
	}
	if n := testutil.Conta(t, p, "documento"); n != documentiPrima {
		t.Errorf("le coperture hanno creato documenti: %d → %d", documentiPrima, n)
	}
	if b := bloccantiConcordi(t, p); b != b0-2 {
		t.Errorf("bloccanti logici %d → %d: attesi due in meno", b0, b)
	}
	// ListFascicolo porta copertura_id (sqlc rigenerato)
	righe, err := q.ListFascicolo(ctx, u22("{T}"))
	if err != nil {
		t.Fatal(err)
	}
	visti := 0
	for _, r := range righe {
		if r.CoperturaID.Valid {
			visti++
			if r.DocumentoID.UUID != u11("{DG}") || r.TipoDocumento != db.TipoDocumentoDisegno2d {
				t.Errorf("riga coperta di ListFascicolo: %+v", r)
			}
		}
	}
	if visti != 2 {
		t.Errorf("righe di ListFascicolo con copertura_id: %d, attese 2", visti)
	}

	// rettifica di G1 (pagine 10-12, con motivo): vale la riga nuova
	c1r := "00000000-0000-4000-8000-000000221103"
	riuscita(t, p, "rettifica", copertura(map[string]string{"copertura_id": "'" + c1r + "'", "azione": "'rettifica'",
		"precedente_id": "'" + c1 + "'", "pagine": "'{10,11,12}'", "motivo": "'la quota della staffa e'' a pagina 12'"}))
	if s := statoDel2D(t, p, "{G1}"); s != coperto(c1r) {
		t.Errorf("G1 dopo la rettifica: %s", s)
	}
	// revoca di G2: torna manca; poi una copertura nuova sulla stessa catena
	c2v, c2n := "00000000-0000-4000-8000-000000221104", "00000000-0000-4000-8000-000000221105"
	riuscita(t, p, "revoca", copertura(map[string]string{"copertura_id": "'" + c2v + "'", "componente_id": "'{G2}'", "azione": "'revoca'",
		"precedente_id": "'" + c2 + "'", "pagine": "NULL", "pagine_totali": "NULL", "motivo": "'pagina sbagliata'"})) // la revoca porta sha e rev del documento (mutazione M12)
	if s := statoDel2D(t, p, "{G2}"); s != scoperto() {
		t.Errorf("G2 dopo la revoca: %s", s)
	}
	if b := bloccantiConcordi(t, p); b != b0-1 {
		t.Errorf("bloccanti logici dopo la revoca: %d, attesi %d", b, b0-1)
	}
	riuscita(t, p, "ripristino dopo la revoca", copertura(map[string]string{"copertura_id": "'" + c2n + "'", "componente_id": "'{G2}'",
		"precedente_id": "'" + c2v + "'", "pagine": "'{10}'"}))
	if s := statoDel2D(t, p, "{G2}"); s != coperto(c2n) {
		t.Errorf("G2 ricoperto: %s", s)
	}

	// le condizioni di validita' ricontrollate a ogni lettura: ognuna rende G1 manca, e tornando indietro G1 torna coperto
	casi := []struct{ nome, rompi, ripara, atteso string }{
		{"rev del componente cambiata", `UPDATE componente SET rev = 'A' WHERE componente_id = '{G1}'`,
			`UPDATE componente SET rev = NULL WHERE componente_id = '{G1}'`, scoperto()},
		{"componente archiviato", `UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'prova' WHERE componente_id = '{G1}'`,
			`UPDATE componente SET archiviato_il = NULL, archiviato_da = NULL, motivo_archiviazione = NULL WHERE componente_id = '{G1}'`, "assente"},
		{"fuori dal perimetro del proprietario", `DELETE FROM componente_relazione WHERE padre_id = '{P}' AND figlio_id = '{G1}'`,
			`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P}', '{G1}', 1, 'manuale', '{U}')`, scoperto()},
		{"proprietario del documento cambiato (D1)", `UPDATE documento SET componente_id = '{P2}', codice = 'ACM-2000' WHERE documento_id = '{DG}'`,
			`UPDATE documento SET componente_id = '{P}', codice = 'ACM-1000' WHERE documento_id = '{DG}'`, scoperto()},
		{"sha256 del documento diverso da quello registrato", `UPDATE documento SET sha256 = repeat('7', 64) WHERE documento_id = '{DG}'`,
			`UPDATE documento SET sha256 = repeat('5', 64) WHERE documento_id = '{DG}'`, scoperto()},
		{"rev del documento diversa da quella registrata", `UPDATE documento SET rev = 'B9' WHERE documento_id = '{DG}'`,
			`UPDATE documento SET rev = 'B2' WHERE documento_id = '{DG}'`, scoperto()},
		{"tipo del documento cambiato (D1)", `UPDATE documento SET tipo = 'sviluppo_dxf' WHERE documento_id = '{DG}'`,
			`UPDATE documento SET tipo = 'disegno_2d' WHERE documento_id = '{DG}'`, scoperto()},
		{"proprietario archiviato", `UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'prova' WHERE componente_id = '{P}'`,
			`UPDATE componente SET archiviato_il = NULL, archiviato_da = NULL, motivo_archiviazione = NULL WHERE componente_id = '{P}'`, scoperto()},
	}
	for _, c := range casi {
		esegui11(t, p, c.rompi)
		if s := statoDel2D(t, p, "{G1}"); s != c.atteso {
			t.Errorf("%s: G1 %s, atteso %s", c.nome, s, c.atteso)
		}
		bloccantiConcordi(t, p)
		esegui11(t, p, c.ripara)
		if s := statoDel2D(t, p, "{G1}"); s != coperto(c1r) {
			t.Errorf("%s, riparato: G1 %s", c.nome, s)
		}
	}

	// una revisione nuova del PDF (B3) sostituisce la B2: le coperture sulla B2 non valgono piu' (nessuno spostamento
	// silenzioso); una copertura nuova sulla B2 si rifiuta; annullando la sostituzione tornano valide
	esegui11(t, p, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{DG3}', '{T}', '{P}', 'disegno_2d', 'ACM-1000', 'B3', 'acm-1000_b3.pdf', 'pdf', repeat('6', 64), 'ACME22/ACM-1000_REV_B3.pdf', '{U}')`)
	if n, err := q.SetSostituitoDa(ctx, db.SetSostituitoDaParams{Nuovo: uuid.NullUUID{UUID: u11("{DG3}"), Valid: true}, Vecchio: u11("{DG}")}); err != nil || n != 1 {
		t.Fatalf("sostituzione B2 → B3: %d %v", n, err)
	}
	for _, g := range []string{"{G1}", "{G2}"} {
		if s := statoDel2D(t, p, g); s != scoperto() {
			t.Errorf("%s dopo la revisione B3: %s (la copertura era sulla B2)", g, s)
		}
	}
	if s := statoDel2D(t, p, "{P}"); s != "ok_in_coda "+e11("{DG3}")+" -" {
		t.Errorf("il 2D del prodotto dopo la revisione: %s", s)
	}
	rifiutoAtteso(t, p, "copertura nuova su un documento sostituito", "P4X06", "non e' corrente",
		copertura(map[string]string{"componente_id": "'{G3}'", "pagine": "'{5}'"}))
	if n, err := q.AnnullaSostituzione(ctx, db.AnnullaSostituzioneParams{Vecchio: u11("{DG}"), Successore: uuid.NullUUID{UUID: u11("{DG3}"), Valid: true}}); err != nil || n != 1 {
		t.Fatalf("annullamento della sostituzione: %d %v", n, err)
	}
	if s := statoDel2D(t, p, "{G1}"); s != coperto(c1r) {
		t.Errorf("G1 dopo l'annullamento della sostituzione: %s", s)
	}

	// identita' condivisa: G1 anche figlio di P2; la copertura vale per il componente (una riga, coperta)
	esegui11(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P2}', '{G1}', 3, 'manuale', '{U}')`)
	if n := uno11[int](t, p, `SELECT count(*) FROM v_fascicolo WHERE componente_id = '{G1}' AND tipo_documento = 'disegno_2d'`); n != 1 {
		t.Errorf("righe del 2D di G1 con due padri: %d", n)
	}
	if s := statoDel2D(t, p, "{G1}"); s != coperto(c1r) {
		t.Errorf("G1 con due padri: %s", s)
	}
	bloccantiConcordi(t, p)

	// mutazione M08: un figlio coperto con una sua rev (C) diversa da quella del PDF (B2) non ha rev_diversa;
	// mutazione M09: con un documento proprio, vince il proprio sulla copertura
	c3 := "00000000-0000-4000-8000-000000221106"
	esegui11(t, p, `UPDATE componente SET rev = 'C' WHERE componente_id = '{G3}'`)
	riuscita(t, p, "copertura di G3 con la sua rev", copertura(map[string]string{"copertura_id": "'" + c3 + "'", "componente_id": "'{G3}'",
		"pagine": "'{4}'", "rev_componente": "'C'"}))
	if s := statoDel2D(t, p, "{G3}"); s != coperto(c3) {
		t.Errorf("G3 coperto con la rev C: %s", s)
	}
	if uno11[bool](t, p, `SELECT rev_diversa FROM v_fascicolo WHERE componente_id = '{G3}' AND tipo_documento = 'disegno_2d'`) {
		t.Error("rev_diversa vera per G3 coperto: la rev del PDF condiviso non e' quella del figlio")
	}
	esegui11(t, p, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('00000000-0000-4000-8000-0000002211d4', '{T}', '{G3}', 'disegno_2d', 'ACM-1203', 'C', 'acm-1203.pdf', 'pdf', repeat('2', 64), 'ACME22/ACM-1203.pdf', '{U}')`)
	if s := statoDel2D(t, p, "{G3}"); s != "ok_in_coda 00000000-0000-4000-8000-0000002211d4 -" {
		t.Errorf("G3 con un documento proprio e una copertura: %s (vince il documento proprio)", s)
	}
	bloccantiConcordi(t, p)

	// un componente coperto non si rimuove (FK), si archivia
	if _, err := q.DeleteComponente(ctx, u11("{G1}")); vincoloViolato(err) != "fk_copertura_componente" {
		t.Errorf("rimozione di un componente coperto: %v", err)
	}
}

// Le regole di nascita di una copertura (P4X06), le FK per RFQ, la catena, l'immutabilita' (P4X02), il muro del
// congelamento (BOM01), la data del database.
func TestP4CopertureRifiutate(t *testing.T) {
	p := schema22ConStoria(t)
	esegui11(t, p, scenaCoperture)
	c1 := "00000000-0000-4000-8000-000000221101"
	riuscita(t, p, "copertura di base", copertura(map[string]string{"copertura_id": "'" + c1 + "'", "quando": "'2000-01-01'"}))
	if v := uno11[bool](t, p, `SELECT quando > now() - interval '1 hour' FROM copertura_documento WHERE copertura_id = '`+c1+`'`); !v {
		t.Error("la data della copertura non e' quella del database")
	}
	casi := []struct {
		nome, codice, frase string
		sql                 []string
	}{
		{"sha256 diverso da quello del documento", "P4X06", "sha256 dichiarato", []string{copertura(map[string]string{"componente_id": "'{G2}'", "sha256": "repeat('6', 64)"})}},
		{"proprietario dichiarato diverso", "P4X06", "non e' quello dichiarato", []string{copertura(map[string]string{"componente_id": "'{G2}'", "documento_componente": "'{S}'"})}},
		{"rev del documento diversa", "P4X06", "non e' quello dichiarato", []string{copertura(map[string]string{"componente_id": "'{G2}'", "rev_documento": "'B1'"})}},
		{"documento di un altro tipo (D1 alla nascita)", "P4X06", "non e' quello dichiarato", []string{
			`UPDATE documento SET tipo = 'sviluppo_dxf' WHERE documento_id = '{DG}'`, copertura(map[string]string{"componente_id": "'{G2}'"})}},
		{"rev del componente diversa", "P4X06", "non ha la rev dichiarata", []string{copertura(map[string]string{"componente_id": "'{G2}'", "rev_componente": "'X'"})}},
		{"componente archiviato", "P4X06", "archiviato", []string{
			`UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'prova' WHERE componente_id = '{G2}'`,
			copertura(map[string]string{"componente_id": "'{G2}'"})}},
		{"proprietario archiviato", "P4X06", "proprietario", []string{
			`UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'prova' WHERE componente_id = '{P}'`,
			copertura(map[string]string{"componente_id": "'{G2}'"})}},
		{"pagine non in ordine", "P4X06", "pagine in ordine", []string{copertura(map[string]string{"componente_id": "'{G2}'", "pagine": "'{11,10}'"})}},
		{"pagine doppie", "P4X06", "pagine in ordine", []string{copertura(map[string]string{"componente_id": "'{G2}'", "pagine": "'{10,10}'"})}},
		{"documento di un'altra RFQ", "23503", "fk_copertura_documento", []string{copertura(map[string]string{"componente_id": "'{G2}'",
			"documento_id": "'{DQ}'", "sha256": "repeat('4', 64)", "documento_componente": "'{Q}'", "rev_documento": "NULL"})}},
		{"componente di un'altra RFQ", "23503", "fk_copertura_componente", []string{copertura(map[string]string{"componente_id": "'{Q}'"})}},
		{"seconda catena per la stessa chiave", "23505", "ux_copertura_inizio", []string{copertura(map[string]string{})}},
		{"copre dopo copre", "P4X06", "non puo' seguire", []string{copertura(map[string]string{"precedente_id": "'" + c1 + "'"})}},
		{"due successori della stessa riga", "23505", "ux_copertura_successore", []string{
			copertura(map[string]string{"azione": "'rettifica'", "precedente_id": "'" + c1 + "'", "motivo": "'uno'"}),
			copertura(map[string]string{"azione": "'rettifica'", "precedente_id": "'" + c1 + "'", "motivo": "'due'"})}},
		// RD-08: ogni riga dice perche', anche copre
		{"rettifica senza motivo", "23514", "ck_copertura_motivo", []string{copertura(map[string]string{"azione": "'rettifica'", "precedente_id": "'" + c1 + "'",
			"motivo": "NULL"})}},
		{"copre senza motivo", "23514", "ck_copertura_motivo", []string{copertura(map[string]string{"componente_id": "'{G2}'", "motivo": "NULL"})}},
		{"rettifica senza la riga precedente", "23514", "ck_copertura_catena", []string{copertura(map[string]string{"componente_id": "'{G2}'", "azione": "'rettifica'"})}},
		// RD-07: una rettifica resta sul proprietario registrato; un cambio di proprietario passa da revoca + copre
		{"rettifica di un altro proprietario", "P4X06", "non puo' seguire", []string{
			`UPDATE documento SET componente_id = '{P2}', codice = 'ACM-2000' WHERE documento_id = '{DG}'`,
			copertura(map[string]string{"azione": "'rettifica'", "precedente_id": "'" + c1 + "'", "documento_componente": "'{P2}'",
				"motivo": "'il PDF ora e'' di P2'"})}},
		{"rettifica dopo una revoca", "P4X06", "non puo' seguire", []string{
			copertura(map[string]string{"copertura_id": "'00000000-0000-4000-8000-000000221109'", "azione": "'revoca'", "precedente_id": "'" + c1 + "'",
				"pagine": "NULL", "pagine_totali": "NULL", "motivo": "'via'"}),
			copertura(map[string]string{"azione": "'rettifica'", "precedente_id": "'00000000-0000-4000-8000-000000221109'", "motivo": "'di nuovo'"})}},
		{"revoca dopo una revoca", "P4X06", "non puo' seguire", []string{
			copertura(map[string]string{"copertura_id": "'00000000-0000-4000-8000-000000221109'", "azione": "'revoca'", "precedente_id": "'" + c1 + "'",
				"pagine": "NULL", "pagine_totali": "NULL", "motivo": "'via'"}),
			copertura(map[string]string{"azione": "'revoca'", "precedente_id": "'00000000-0000-4000-8000-000000221109'", "pagine": "NULL",
				"pagine_totali": "NULL", "motivo": "'ancora via'"})}},
		{"revoca di un altro proprietario", "P4X06", "non puo' seguire", []string{copertura(map[string]string{"azione": "'revoca'",
			"precedente_id": "'" + c1 + "'", "documento_componente": "'{P2}'", "pagine": "NULL", "pagine_totali": "NULL", "motivo": "'via'"})}},
		{"precedente di un'altra chiave", "P4X06", "non puo' seguire", []string{copertura(map[string]string{"componente_id": "'{G2}'",
			"azione": "'rettifica'", "precedente_id": "'" + c1 + "'", "motivo": "'altra chiave'"})}},
		{"UPDATE di una copertura", "P4X02", "non si modifica", []string{`UPDATE copertura_documento SET pagine = '{1}'`}},
		{"DELETE di una copertura", "P4X02", "non si modifica", []string{`DELETE FROM copertura_documento`}},
		{"TRUNCATE delle coperture", "P4X02", "non si svuotano", []string{`TRUNCATE copertura_documento`}},
		// il muro del congelamento: una V1 congelata scritta a mano (i trigger di bom_versione spenti nella transazione)
		{"BOM congelata", "BOM01", "congelata", []string{
			`INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('00000000-0000-4000-8000-0000002211f1', '{T}', 'FATTIBILITA', now())`,
			`ALTER TABLE bom_versione DISABLE TRIGGER USER`,
			`INSERT INTO bom_versione (thread_id, numero, stato, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da, congelata_da, congelata_il)
			 VALUES ('{T}', 1, 'congelata', 'preventivo', 'FATTIBILITA', '00000000-0000-4000-8000-0000002211f1', 'prova', '{U}', '{U}', now())`,
			`ALTER TABLE bom_versione ENABLE TRIGGER USER`,
			copertura(map[string]string{"componente_id": "'{G2}'"})}},
	}
	for _, c := range casi {
		rifiutoAtteso(t, p, c.nome, c.codice, c.frase, c.sql...)
	}
}

// Senza coperture, ogni riga di v_fascicolo e' quella della definizione della 0020 (letta dal file incorporato e
// creata come vista temporanea): le 19 colonne storiche non cambiano ne' valore ne' significato. Con le coperture,
// cambiano solo le righe con copertura_id.
func TestP4VFascicoloSenzaCopertureComeLa0020(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	esegui11(t, p, scenaCoperture)
	// stati diversi: una proposta aperta (da_confermare), un riferimento del portale (sul_portale), un 2D con rev
	// diversa, un documento scritto sul NAS, una deroga gia' nelle righe di base (derogato)
	esegui11(t, p, `
		INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		  VALUES ('{A}', '{T}', 'disegno_2d', 'ACM-1203', 50, 'nome_file');
		INSERT INTO riferimento_portale (messaggio_id, thread_id, codice, tipo_atteso, testo_citato)
		  VALUES ('{M}', '{T}', 'ACM-1202', 'disegno_2d', 'vedi portale');
		UPDATE componente SET rev = 'B' WHERE componente_id = '{P3}';
		INSERT INTO documento (thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, stato_nas, confermato_da)
		  VALUES ('{T}', '{P3}', 'disegno_2d', 'ACM-3000', 'A', 'acm-3000.pdf', 'pdf', repeat('3', 64), 'ACME22/ACM-3000.pdf', 'scritto', '{U}')`)
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	testo := migs[19].SQL
	if migs[19].Versione != 20 {
		t.Fatalf("attesa la 0020, trovata %s", migs[19].Nome)
	}
	testo = strings.ReplaceAll(testo, "\r\n", "\n")
	inizio := strings.Index(testo, "CREATE OR REPLACE VIEW v_fascicolo AS")
	fine := strings.Index(testo[inizio:], "WHERE co.archiviato_il IS NULL;")
	if inizio < 0 || fine < 0 {
		t.Fatal("definizione di v_fascicolo non trovata nella 0020")
	}
	vecchia := strings.Replace(testo[inizio:inizio+fine+len("WHERE co.archiviato_il IS NULL;")],
		"CREATE OR REPLACE VIEW v_fascicolo AS", "CREATE TEMP VIEW v_fascicolo_0020 AS", 1)
	conn, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, vecchia); err != nil {
		t.Fatalf("vista della 0020: %v", err)
	}
	defer conn.Exec(context.Background(), `DROP VIEW IF EXISTS v_fascicolo_0020`) //nolint:errcheck
	const colonne = `thread_id, componente_id, codice, rev, tipo_componente, tipo_documento, bloccante, documento_id, stato_nas,
		path_relativo, proposta_aperta, atteso_da_portale, deroga_id, esito, fonte_attesa, n_proposte_aperte, documento_rev, rev_diversa, anomalia_id`
	diverse := func(filtro string) int {
		var n int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM ((SELECT `+colonne+` FROM v_fascicolo `+filtro+` EXCEPT ALL SELECT `+colonne+` FROM v_fascicolo_0020)
			UNION ALL (SELECT `+colonne+` FROM v_fascicolo_0020 EXCEPT ALL SELECT `+colonne+` FROM v_fascicolo `+filtro+`)) x`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var righe, esiti int
	if err := conn.QueryRow(ctx, `SELECT count(*), count(DISTINCT esito) FROM v_fascicolo`).Scan(&righe, &esiti); err != nil {
		t.Fatal(err)
	}
	if esiti < 5 {
		t.Fatalf("la scena ha solo %d esiti diversi: serve varieta' per il confronto", esiti)
	}
	if n := diverse(""); n != 0 {
		t.Errorf("senza coperture v_fascicolo e la definizione della 0020 differiscono in %d righe (su %d)", n, righe)
	}
	if uno11[int](t, p, `SELECT count(*) FROM v_fascicolo WHERE copertura_id IS NOT NULL`) != 0 {
		t.Error("copertura_id non nullo senza coperture")
	}
	// con una copertura cambia solo la riga coperta: le altre restano quelle della 0020
	riuscita(t, p, "copertura", copertura(map[string]string{"componente_id": "'{G3}'", "pagine": "'{4}'"}))
	if n := diverse("WHERE copertura_id IS NULL"); n != 1 {
		t.Errorf("con una copertura: %d righe diverse dalla 0020 fra quelle senza copertura (attesa 1: la riga coperta manca da questa parte)", n)
	}
}

// ------------------------------------------------------------------ registro permanente delle bozze

// D-P4A-01 = P-04a [U]: ogni passo del ciclo della bozza resta in distinta_bozza_registro, scritto solo dal
// trigger; dopo la conservazione (bozza abbandonata da oltre 90 giorni cancellata) il registro resta e il prodotto
// non si rimuove; il registro non si modifica; una confermata ha il suo passo.
func TestP4RegistroDelleBozze(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	apri := `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		VALUES ('{BR}', '{T}', '{P3}', 0, 0, repeat('1', 64), '{U}', '{U}');
		INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		VALUES ('{BR}', 1, '{"formato":2}', repeat('a', 64), 0, 0, repeat('1', 64), '{U}')`
	riuscita(t, p, "apertura", apri)
	passi := func() string {
		return uno11[string](t, p, `SELECT coalesce(string_agg(passo || ':' || coalesce(utente_id::text, '-') || ':' || coalesce(motivo, '-'), ' | ' ORDER BY registro_id), '')
			FROM distinta_bozza_registro WHERE bozza_id = '{BR}'`)
	}
	U := e11("{U}")
	if s := passi(); s != "aperta:"+U+":-" {
		t.Errorf("registro dopo l'apertura: %s", s)
	}
	riuscita(t, p, "salvataggio", `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		VALUES ('{BR}', 2, '{"formato":2}', repeat('b', 64), 0, 0, repeat('1', 64), '{U}')`)
	if s := passi(); s != "aperta:"+U+":-" {
		t.Errorf("un salvataggio non e' un passo del ciclo: %s", s)
	}
	riuscita(t, p, "abbandono", `UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}', motivo_chiusura = 'non serve piu''' WHERE bozza_id = '{BR}'`)
	if s := passi(); s != "aperta:"+U+":- | abbandonata:"+U+":non serve piu'" {
		t.Errorf("registro dopo l'abbandono: %s", s)
	}
	rifiutoAtteso(t, p, "cancellazione prima dei 90 giorni", "P4X04", "oltre 90 giorni", `DELETE FROM distinta_bozza WHERE bozza_id = '{BR}'`)
	riuscita(t, p, "conservazione",
		`ALTER TABLE distinta_bozza DISABLE TRIGGER trg_distinta_bozza_ciclo`,
		`UPDATE distinta_bozza SET chiusa_il = now() - interval '91 days' WHERE bozza_id = '{BR}'`,
		`ALTER TABLE distinta_bozza ENABLE TRIGGER trg_distinta_bozza_ciclo`,
		`DELETE FROM distinta_bozza WHERE bozza_id = '{BR}'`)
	if n := uno11[int](t, p, `SELECT (SELECT count(*) FROM distinta_bozza WHERE bozza_id = '{BR}') + (SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = '{BR}')`); n != 0 {
		t.Errorf("la bozza conservata ha ancora %d righe", n)
	}
	s := passi()
	if !strings.HasPrefix(s, "aperta:"+U+":- | abbandonata:"+U+":non serve piu' | cancellata:-:conservazione: abbandonata il ") {
		t.Errorf("registro dopo la conservazione: %s", s)
	}
	// il prodotto con una storia di bozze non si rimuove (FK permanente)
	if _, err := db.New(p).DeleteComponente(ctx, u22("{P3}")); vincoloViolato(err) != "fk_dbr_prodotto" {
		t.Errorf("rimozione del prodotto dopo la conservazione della bozza: %v", err)
	}
	// solo dal trigger, solo aggiunte
	rifiutoAtteso(t, p, "INSERT diretto nel registro", "P4X02", "lo scrive solo il trigger",
		`INSERT INTO distinta_bozza_registro (bozza_id, thread_id, prodotto_id, passo, utente_id) VALUES (gen_random_uuid(), '{T}', '{P3}', 'aperta', '{U}')`)
	rifiutoAtteso(t, p, "UPDATE del registro", "P4X02", "non si modifica", `UPDATE distinta_bozza_registro SET motivo = 'altro'`)
	rifiutoAtteso(t, p, "DELETE del registro", "P4X02", "non si modifica", `DELETE FROM distinta_bozza_registro`)
	rifiutoAtteso(t, p, "TRUNCATE del registro", "P4X02", "non si svuota", `TRUNCATE distinta_bozza_registro`)

	// una bozza confermata: la conferma n. 1 di P2 chiude B1 (righe di base)
	riuscita(t, p, "conferma che chiude B1",
		`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
			base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
			criterio_verifica, chiave_idempotenza, confermato_da)
		 SELECT '{T}', '{P2}', 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
			'{B1}', 1, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text), repeat('a', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}'`,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo)
		 SELECT conferma_id, '{T}', '{P2}', true, 'k-p2', 'ACM-2000', 'Piastra', 'finito' FROM conferma_strutturale WHERE bozza_id = '{B1}'`,
		`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{B1}'`)
	if s := uno11[string](t, p, `SELECT string_agg(passo, ' ' ORDER BY registro_id) FROM distinta_bozza_registro WHERE bozza_id = '{B1}'`); s != "aperta confermata" {
		t.Errorf("registro della bozza confermata: %s", s)
	}
	rifiutoAtteso(t, p, "una confermata non si cancella nemmeno dopo 400 giorni", "P4X04", "oltre 90 giorni",
		`ALTER TABLE distinta_bozza DISABLE TRIGGER trg_distinta_bozza_ciclo`,
		`UPDATE distinta_bozza SET chiusa_il = now() - interval '400 days' WHERE bozza_id = '{B1}'`,
		`ALTER TABLE distinta_bozza ENABLE TRIGGER trg_distinta_bozza_ciclo`,
		`DELETE FROM distinta_bozza WHERE bozza_id = '{B1}'`)
}

// ------------------------------------------------------------------ fonti STEP secondarie

// La bozza BF di P: nodi P (k-p) e S (k-s) con UUID, e un nodo nuovo senza UUID (k-n).
const bozzaBF = `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_conferma_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
	VALUES ('{BF}', '{T}', '{P}', '{C1}', 1, 0, repeat('1', 64), '{U}', '{U}');
INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base, salvata_da)
	VALUES ('{BF}', 1, jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text, 'nodi', jsonb_build_array(
		jsonb_build_object('chiave_bozza', 'k-p', 'componente_id', '{P}'::text, 'op', 'riusa'),
		jsonb_build_object('chiave_bozza', 'k-s', 'componente_id', '{S}'::text, 'op', 'riusa'),
		jsonb_build_object('chiave_bozza', 'k-n', 'op', 'crea', 'codice', 'ACM-1300'))),
		repeat('9', 64), '{C1}', 1, 0, repeat('1', 64), '{U}')`

func eventoFonte(cambi map[string]string) string {
	base := []struct{ col, expr string }{
		{"thread_id", "'{T}'"}, {"prodotto_id", "'{P}'"}, {"ruolo", "'sottoassieme'"}, {"componente_id", "NULL"}, {"chiave_bozza", "NULL"},
		{"azione", "'rifiuto'"}, {"allegato_id", "'{A}'"}, {"sha256", "repeat('b', 64)"}, {"gruppo", "'cand:a:n'"}, {"bozza_id", "NULL"},
		{"motivo", "'i figli dello STEP secondario non sono quelli del PDF'"}, {"autore_id", "'{U}'"},
	}
	var cols, vals []string
	for _, c := range base {
		v := c.expr
		if x, ok := cambi[c.col]; ok {
			v = x
		}
		cols = append(cols, c.col)
		vals = append(vals, v)
	}
	return "INSERT INTO fonte_step_evento (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(vals, ", ") + ")"
}

// D-SHOCK-S2 B [U] con il protocollo [T] della nota §12: ruolo prodotto come prima; ruolo sottoassieme con la
// destinazione su un UUID esistente o temporanea (bozza + chiave di un nodo senza UUID); la risoluzione al COMMIT
// scritta dal database con un evento nuovo, senza UPDATE; la storia dei rifiuti leggibile dopo la conservazione.
func TestP4FontiSecondarie(t *testing.T) {
	p := schema22ConStoria(t)
	esegui11(t, p, bozzaBF)
	prodottoPrima := uno11[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE ruolo = 'prodotto'`)
	// destinazione su un UUID esistente
	riuscita(t, p, "scelta della fonte secondaria di S", eventoFonte(map[string]string{"azione": "'scelta'", "componente_id": "'{S}'",
		"gruppo": "'cand:a:s'", "motivo": "NULL"}))
	// destinazione temporanea: il nodo k-n della bozza, senza UUID
	riuscita(t, p, "rifiuto con destinazione temporanea", eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-n'"}))
	temporaneo := uno11[int64](t, p, `SELECT evento_id FROM fonte_step_evento WHERE chiave_bozza = 'k-n' AND azione = 'rifiuto'`)
	primaDellaRisoluzione := uno11[string](t, p, fmt.Sprintf(`SELECT to_jsonb(e)::text FROM fonte_step_evento e WHERE evento_id = %d`, temporaneo))
	// RD-01: la riga porta la copia del nodo k-n della versione 1, scritta dal database; la destinazione con UUID
	// non ne ha; una copia mandata dal chiamante si riscrive
	if d := uno11[string](t, p, fmt.Sprintf(`SELECT (destinazione ->> 'versione') || ' ' || (destinazione -> 'nodo' ->> 'codice') || ' ' || (destinazione -> 'nodo' ->> 'op')
		FROM fonte_step_evento WHERE evento_id = %d`, temporaneo)); d != "1 ACM-1300 crea" {
		t.Errorf("copia del nodo della destinazione temporanea: %q", d)
	}
	if n := uno11[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE componente_id = '{S}' AND destinazione IS NOT NULL`); n != 0 {
		t.Errorf("la destinazione con UUID ha una copia del nodo: %d", n)
	}
	riuscita(t, p, "destinazione falsa del chiamante",
		`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, chiave_bozza, destinazione, azione, allegato_id, sha256, gruppo, bozza_id, autore_id)
		 VALUES ('{T}', '{P}', 'sottoassieme', 'k-n', '{"versione": 9, "nodo": {"codice": "FALSO"}}', 'scelta', '{A}', repeat('b', 64), 'cand:a:n', '{BF}', '{U}')`)
	if d := uno11[string](t, p, `SELECT (destinazione ->> 'versione') || ' ' || (destinazione -> 'nodo' ->> 'codice') FROM fonte_step_evento
		WHERE azione = 'scelta' AND chiave_bozza = 'k-n'`); d != "1 ACM-1300" {
		t.Errorf("la destinazione mandata dal chiamante non e' stata riscritta dal database: %q", d)
	}

	casi := []struct{ nome, frase, sql string }{
		{"chiave che non e' nella bozza", "non e' un nodo senza UUID", eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-x'"})},
		{"chiave di un nodo che ha un UUID", "non e' un nodo senza UUID", eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-s'"})},
		{"bozza di un altro prodotto", "vuole la bozza", eventoFonte(map[string]string{"bozza_id": "'{B1}'", "chiave_bozza": "'k-n'"})},
		{"bozza che non esiste", "vuole la bozza", eventoFonte(map[string]string{"bozza_id": "gen_random_uuid()", "chiave_bozza": "'k-n'"})},
		{"risoluzione scritta a mano", "la scrive solo la conferma", eventoFonte(map[string]string{"azione": "'risoluzione'", "bozza_id": "'{BF}'",
			"chiave_bozza": "'k-n'", "componente_id": "'{S}'", "risolve_evento_id": fmt.Sprint(temporaneo)})},
	}
	for _, c := range casi {
		rifiutoAtteso(t, p, c.nome, "P4X07", c.frase, c.sql)
	}
	// RD-06: scelta e rifiuto hanno una sola destinazione, l'UUID o la chiave
	rifiutoAtteso(t, p, "due destinazioni", "23514", "ck_fonte_ruolo", eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-n'",
		"componente_id": "'{S}'"}))

	// la conferma n. 2 di P chiude BF e certifica il nodo nuovo N con la chiave k-n: il database aggiunge la risoluzione
	riuscita(t, p, "conferma che risolve k-n",
		`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ('{N}', '{T}', 'ACM-1300', 'sottoassieme', 'manuale', '{U}')`,
		`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P}', '{N}', 1, 'manuale', '{U}')`,
		`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
			firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot,
			impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da)
		 SELECT '{C2}', '{T}', '{P}', 2, '{C1}', 2, false, repeat('3', 64), repeat('2', 64), '{C1}', 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
			'{BF}', 1, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text), repeat('9', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}'`,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo) VALUES
			('{C2}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito'),
			('{C2}', '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme'),
			('{C2}', '{T}', '{N}', false, 'k-n', 'ACM-1300', NULL, NULL, 'sottoassieme')`,
		`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ('{C2}', '{P}', '{S}', 2, 'conferma'), ('{C2}', '{P}', '{N}', 1, 'utente')`,
		`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{BF}'`)
	ris := uno11[string](t, p, fmt.Sprintf(`SELECT string_agg(ruolo || ' ' || azione || ' ' || componente_id::text || ' ' || chiave_bozza || ' ' || bozza_id::text || ' ' || autore_id::text
		|| ' ' || allegato_id::text || ' ' || gruppo, ' | ') FROM fonte_step_evento WHERE risolve_evento_id = %d`, temporaneo))
	if want := "sottoassieme risoluzione " + e11("{N}") + " k-n " + e11("{BF}") + " " + e11("{U}") + " " + e11("{A}") + " cand:a:n"; ris != want {
		t.Errorf("risoluzione al COMMIT:\n %s\natteso\n %s", ris, want)
	}
	if !uno11[bool](t, p, fmt.Sprintf(`SELECT r.destinazione = e.destinazione FROM fonte_step_evento r JOIN fonte_step_evento e ON e.evento_id = r.risolve_evento_id
		WHERE r.risolve_evento_id = %d`, temporaneo)) {
		t.Error("la risoluzione non porta la stessa copia del nodo dell'evento che risolve")
	}
	if dopo := uno11[string](t, p, fmt.Sprintf(`SELECT to_jsonb(e)::text FROM fonte_step_evento e WHERE evento_id = %d`, temporaneo)); dopo != primaDellaRisoluzione {
		t.Errorf("l'evento temporaneo e' cambiato:\n%s\n%s", primaDellaRisoluzione, dopo)
	}
	// la destinazione si ricostruisce: evento temporaneo → risoluzione → UUID
	if d := uno11[string](t, p, fmt.Sprintf(`SELECT coalesce(e.componente_id, r.componente_id)::text FROM fonte_step_evento e
		LEFT JOIN fonte_step_evento r ON r.risolve_evento_id = e.evento_id WHERE e.evento_id = %d`, temporaneo)); d != e11("{N}") {
		t.Errorf("destinazione ricostruita: %s", d)
	}
	if n := uno11[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE ruolo = 'prodotto'`); n != prodottoPrima {
		t.Errorf("gli eventi secondari sono entrati fra quelli del prodotto: %d → %d", prodottoPrima, n)
	}
	// un nodo certificato con l'UUID gia' noto (k-s) non riceve risoluzioni; una sola risoluzione per evento (due
	// eventi temporanei su k-n: il rifiuto e la scelta con la copia falsa riscritta)
	if n := uno11[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'risoluzione'`); n != 2 {
		t.Errorf("risoluzioni: %d, attese 2", n)
	}
	// bozza chiusa: niente destinazioni temporanee nuove
	rifiutoAtteso(t, p, "destinazione temporanea su una bozza chiusa", "P4X07", "vuole la bozza",
		eventoFonte(map[string]string{"bozza_id": "'{BF}'", "chiave_bozza": "'k-n'"}))

	// la storia dopo la conservazione: un rifiuto temporaneo su una bozza di P3 poi abbandonata e cancellata resta,
	// e il registro dice di chi era la bozza e come e' finita
	riuscita(t, p, "bozza BX di P3", `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		VALUES ('{BX}', '{T}', '{P3}', 0, 0, repeat('1', 64), '{U}', '{U}');
		INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		VALUES ('{BX}', 1, jsonb_build_object('formato', 2, 'prodotto_id', '{P3}'::text, 'nodi', jsonb_build_array(
			jsonb_build_object('chiave_bozza', 'k-m', 'op', 'crea', 'codice', 'ACM-3100'))), repeat('8', 64), 0, 0, repeat('1', 64), '{U}')`)
	riuscita(t, p, "rifiuto temporaneo su BX", eventoFonte(map[string]string{"prodotto_id": "'{P3}'", "bozza_id": "'{BX}'", "chiave_bozza": "'k-m'"}))
	riuscita(t, p, "abbandono e conservazione di BX",
		`UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}' WHERE bozza_id = '{BX}'`)
	riuscita(t, p, "conservazione di BX",
		`ALTER TABLE distinta_bozza DISABLE TRIGGER trg_distinta_bozza_ciclo`,
		`UPDATE distinta_bozza SET chiusa_il = now() - interval '91 days' WHERE bozza_id = '{BX}'`,
		`ALTER TABLE distinta_bozza ENABLE TRIGGER trg_distinta_bozza_ciclo`,
		`DELETE FROM distinta_bozza WHERE bozza_id = '{BX}'`)
	// RD-01: dopo la conservazione la riga dice anche quale nodo era (codice proposto, op), non solo la chiave
	storia := uno11[string](t, p, `SELECT e.azione || ' ' || e.chiave_bozza || ' ' || (e.destinazione -> 'nodo' ->> 'codice') || ' '
		|| (e.destinazione -> 'nodo' ->> 'op') || ' ' || string_agg(r.passo, ',' ORDER BY r.registro_id) || ' ' || min(r.prodotto_id::text)
		FROM fonte_step_evento e JOIN distinta_bozza_registro r ON r.bozza_id = e.bozza_id
		WHERE e.bozza_id = '{BX}' GROUP BY e.evento_id, e.azione, e.chiave_bozza, e.destinazione`)
	if want := "rifiuto k-m ACM-3100 crea aperta,abbandonata,cancellata " + e11("{P3}"); storia != want {
		t.Errorf("storia del rifiuto dopo la conservazione: %q, attesa %q", storia, want)
	}
}

// ------------------------------------------------------------------ due varianti della 0022

// Lo schema registra solo il numero della versione. Un database con la 22 di un altro testo (senza variante, con
// la tabella delle varianti tolta o con un'altra variante) e il migratore nuovo: il migratore si ferma prima di
// applicare qualunque cosa e non cambia niente; anche l'apertura in sola lettura si ferma. Con la variante giusta,
// avvio normale. Il testo «senza variante» qui e' la 0022 attuale senza la sezione della variante, cioe' com'era
// la 0022 5416295c per il migratore; la prova con il file 5416295c vero e' nella nota della 0022 §12.
func TestP4DueVariantiDella0022(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	m22 := migs[len(migs)-1]
	if m22.Variante == "" {
		t.Fatal("la 0022 non dichiara la sua variante")
	}
	dsn := testutil.DSN(t)
	rifiutata := func(nome string) {
		t.Helper()
		prima := impronta18(t, p)
		n, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
		var vd *migrazioni.VarianteDiversa
		if !errors.As(err, &vd) || n != 0 || vd.Versione != 22 || vd.DelBinario != m22.Variante {
			t.Errorf("%s: il migratore doveva fermarsi con VarianteDiversa, ottenuto %d migrazioni e %v", nome, n, err)
		} else if !strings.Contains(err.Error(), "non e' stato toccato") {
			t.Errorf("%s: messaggio poco chiaro: %v", nome, err)
		}
		if d := differenze(prima, impronta18(t, p)); d != "" {
			t.Errorf("%s: il migratore fermato ha cambiato lo schema:\n%s", nome, d)
		}
		if pool, err := migrazioni.ApriInLettura(ctx, dsn, risorse.FS); !errors.As(err, &vd) {
			if pool != nil {
				pool.Close()
			}
			t.Errorf("%s: l'apertura in sola lettura doveva fermarsi, ottenuto %v", nome, err)
		}
	}

	// variante giusta: avvio normale (niente da applicare), anche in sola lettura
	testutil.SchemaPulito(t, p)
	if n, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso()); err != nil || n != 0 {
		t.Fatalf("avvio con la variante giusta: %d %v", n, err)
	}
	if pool, err := migrazioni.ApriInLettura(ctx, dsn, risorse.FS); err != nil {
		t.Errorf("apertura in sola lettura con la variante giusta: %v", err)
	} else {
		pool.Close()
	}
	if v := uno22[string](t, p, `SELECT variante FROM schema_variante WHERE versione = 22`); v != m22.Variante {
		t.Errorf("variante registrata %q, dichiarata %q", v, m22.Variante)
	}

	esegui22(t, p, `UPDATE schema_variante SET variante = '0022-un-altro-testo' WHERE versione = 22`)
	rifiutata("un'altra variante")
	esegui22(t, p, `DELETE FROM schema_variante`)
	rifiutata("riga della variante assente")
	esegui22(t, p, `DROP TABLE schema_variante`)
	rifiutata("tabella delle varianti assente")

	// la 0022 senza la sezione della variante sopra la 21 (com'era il testo 5416295c per il migratore)
	testutil.SchemaFinoA(t, p, 21)
	sql := strings.ReplaceAll(m22.SQL, "\r\n", "\n")
	i, j := strings.Index(sql, "CREATE TABLE schema_variante ("), strings.Index(sql, "INSERT INTO schema_versione (versione) VALUES (22);")
	if i < 0 || j < i {
		t.Fatal("sezione della variante non trovata")
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sql[:i]+sql[j:]); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("0022 senza variante: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rifiutata("0022 senza variante")

	// un file che dichiara una variante e non la registra non si applica (come la riga di schema_versione)
	testutil.SchemaFinoA(t, p, 21)
	falsi := append([]migrazioni.Migrazione(nil), migs...)
	falsi[len(falsi)-1].Variante = "un-altra-dichiarazione"
	if _, err := migrazioni.ApplicaElenco(ctx, p, falsi, 22, testutil.LogSilenzioso()); err == nil || !strings.Contains(err.Error(), "dichiara la variante") {
		t.Errorf("file che non registra la variante dichiarata: %v", err)
	}
	if applicata(t, p, 22) {
		t.Error("la 22 risulta applicata dopo il rifiuto")
	}
}

// ------------------------------------------------------------------ mutanti sopravvissuti alla revisione B1/Q1

// conferma2 e' la conferma n. 2 di P (formato 2, base e precedente C1) con i contatori correnti piu' gli scarti
// indicati; componenti e archi validi a parte.
func conferma2(scartoStruttura, scartoDati, numero, criterio string) string {
	return `INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2,
		firma_struttura, firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot,
		impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da)
	 SELECT '{C2}', '{T}', '{P}', ` + numero + `, '{C1}', 2, false, repeat('3', 64), repeat('2', 64), '{C1}', 0, repeat('1', 64),
		COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0) + ` + scartoStruttura + `,
		COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0) + ` + scartoDati + `, '{"rotta":"prova"}',
		jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text), repeat('a', 64), '{}', '{}', '` + criterio + `', gen_random_uuid(), '{U}'`
}

const (
	radiceC2 = `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
		VALUES ('{C2}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito')`
	figlioC2 = `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
		VALUES ('{C2}', '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme')`
	arcoC2 = `INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ('{C2}', '{P}', '{S}', 2, 'conferma')`
)

// Mutanti S10-S24: ogni controllo della conferma ha una prova che lo vede rispondere da solo (codice e frase).
func TestP4LaConfermaRifiutaCioCheNonECertificabile(t *testing.T) {
	p := schema22ConStoria(t)
	ok := conferma2("0", "0", "2", "nessuno")
	riuscitaSenzaCommit := func(nome string, istruzioni ...string) {
		t.Helper()
		ctx := context.Background()
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		for _, s := range istruzioni {
			if _, err := tx.Exec(ctx, e11(s)); err != nil {
				t.Fatalf("%s: %v", nome, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
	}
	casi := []struct {
		nome, frase string
		sql         []string
	}{
		{"S10 contatore dati diverso da quello corrente", "contatori registrati", []string{conferma2("0", "1", "2", "nessuno")}},
		{"S11 numero fuori sequenza", "attesa la n.", []string{conferma2("0", "0", "3", "nessuno")}},
		{"S16 radice su un componente che non e' il prodotto", "radice e' solo il prodotto", []string{ok, radiceC2,
			strings.Replace(figlioC2, "false, 'k-s'", "true, 'k-s'", 1)}},
		{"S17 componente diverso dalla working", "nella working non e' quello registrato", []string{ok, radiceC2,
			strings.Replace(figlioC2, "'ACM-1100'", "'ACM-1101'", 1)}},
		{"S18 sha256 dello STEP diverso dal documento", "lo sha256 dello STEP", []string{
			`UPDATE componente SET step_strutturale_id = '{D}' WHERE componente_id = '{P}'`, ok,
			`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo, step_strutturale_id, step_sha256)
			 VALUES ('{C2}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito', '{D}', repeat('d', 64))`}},
		{"S19 formato 2 senza la chiave della bozza", "non ha la chiave della bozza", []string{ok,
			strings.Replace(radiceC2, "true, 'k-p'", "true, NULL", 1)}},
		{"S20 arco con una qta che la working non ha", "non e' nella working", []string{ok, radiceC2, figlioC2,
			strings.Replace(arcoC2, "'{S}', 2, ", "'{S}', 3, ", 1)}},
		{"S21 al COMMIT senza la riga del prodotto", "manca la riga del prodotto", []string{ok, figlioC2}},
		{"S22 criterio step senza lo STEP sulla radice", "criterio step", []string{conferma2("0", "0", "2", "step"), radiceC2, figlioC2, arcoC2}},
		// S23: il controllo al COMMIT sulla qta si raggiunge solo con il controllo di nascita dell'arco spento
		{"S23 al COMMIT un arco certificato con un'altra qta", "fra gli archi certificati", []string{ok, radiceC2, figlioC2,
			`ALTER TABLE conferma_strutturale_arco DISABLE TRIGGER trg_csa_contenuto`,
			strings.Replace(arcoC2, "'{S}', 2, ", "'{S}', 3, ", 1),
			`ALTER TABLE conferma_strutturale_arco ENABLE TRIGGER trg_csa_contenuto`}},
		{"S24 al COMMIT un componente certificato senza arco", "nessun arco certificato ci arriva", []string{ok, radiceC2, figlioC2, arcoC2,
			`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo)
			 VALUES ('{C2}', '{T}', '{P2}', false, 'k-p2', 'ACM-2000', 'Piastra', 'finito')`}},
		// S13, S14: la bozza della conferma (B1 di P2, versione 1, impronta a…)
		{"S13 bozza gia' chiusa", "e' abbandonata", []string{
			`UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}' WHERE bozza_id = '{B1}'`,
			`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
				base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
				criterio_verifica, chiave_idempotenza, confermato_da)
			 SELECT '{T}', '{P2}', 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
				COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
				'{B1}', 1, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text), repeat('a', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}'`}},
		{"S14 snapshot diverso da quello della versione", "lo snapshot (impronta", []string{
			`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
				base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
				criterio_verifica, chiave_idempotenza, confermato_da)
			 SELECT '{T}', '{P2}', 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
				COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
				'{B1}', 1, '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text), repeat('b', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}'`}},
	}
	for _, c := range casi {
		rifiutoAtteso(t, p, c.nome, "P4X01", c.frase, c.sql...)
	}
	// la controprova: la stessa conferma con righe valide passa
	riuscitaSenzaCommit("conferma n. 2 valida", ok, radiceC2, figlioC2, arcoC2)
}

// Mutanti S37, S39, S41, S43: la fonte che cambia vuole un evento motivato; il giornale chiuso; il numero della
// base; la data degli eventi.
func TestP4LaBozzaFonteBaseEGiornale(t *testing.T) {
	p := schema22ConStoria(t)
	riuscita(t, p, "bozza BR di P3 con la fonte", `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		VALUES ('{BR}', '{T}', '{P3}', 0, 0, repeat('1', 64), '{U}', '{U}');
		INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da)
		VALUES ('{BR}', 1, '{"formato":2}', repeat('a', 64), 0, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}')`)
	v2 := `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da)
		VALUES ('{BR}', 2, '{"formato":2}', repeat('c', 64), 0, 0, repeat('1', 64), '{A}', repeat('b', 64), 'cand:a:r', '{U}')`
	rifiutoAtteso(t, p, "S37 fonte cambiata senza evento motivato", "P4X04", "senza un evento motivato", v2)
	riuscita(t, p, "fonte cambiata con un evento motivato", v2,
		`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, origine, motivo, utente_id, quando)
		 VALUES ('{BR}', 1, 2, 'cambia_fonte', 'utente', 'lo STEP giusto e'' l''altra radice', '{U}', '2000-01-01')`)
	// S43: la data dell'evento la scrive il database
	if !uno11[bool](t, p, `SELECT quando > now() - interval '1 hour' FROM distinta_bozza_evento WHERE bozza_id = '{BR}' AND n = 1`) {
		t.Error("la data dell'evento non e' quella del database")
	}
	// S39: un evento della versione salvata nella stessa transazione, ma con la bozza gia' chiusa
	rifiutoAtteso(t, p, "S39 evento su una bozza chiusa nella stessa transazione", "P4X04", "il giornale e' chiuso",
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da)
		 VALUES ('{BR}', 3, '{"formato":2}', repeat('d', 64), 0, 0, repeat('1', 64), '{A}', repeat('b', 64), 'cand:a:r', '{U}')`,
		`UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}' WHERE bozza_id = '{BR}'`,
		`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, origine, utente_id) VALUES ('{BR}', 2, 3, 'togli_nodo', 'utente', '{U}')`)
	// S41: base_numero della versione diverso dal numero della conferma di base (bozza di P, base C1 = n. 1)
	rifiutoAtteso(t, p, "S41 base_numero diverso dalla conferma di base", "P4X04", "base_numero",
		`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_conferma_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		 VALUES ('{BF}', '{T}', '{P}', '{C1}', 1, 0, repeat('1', 64), '{U}', '{U}')`,
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base, salvata_da)
		 VALUES ('{BF}', 1, '{"formato":2}', repeat('a', 64), '{C1}', 2, 0, repeat('1', 64), '{U}')`)
}

// Mutanti S53-S56, S58, S61: attestazioni e storia delle deroghe.
func TestP4AttestazioniEDerogheMutanti(t *testing.T) {
	p := schema22ConStoria(t)
	att := func(numero, precedente, conferma string) string {
		return `INSERT INTO attestazione_fascicolo (attestazione_id, thread_id, prodotto_id, numero, precedente_id, conferma_strutturale_id, firma_piano, attestato_da)
			VALUES ('00000000-0000-4000-8000-0000002211a7', '{T}', '{P}', ` + numero + `, ` + precedente + `, '` + conferma + `', repeat('f', 64), '{U}')`
	}
	rifiutoAtteso(t, p, "S54 attestazione fuori sequenza", "P4X02", "attesa la n.", att("3", "'{AT1}'", "{C1}"))
	rifiutoAtteso(t, p, "S55 documento attestato fuori dalla transazione dell'attestazione", "P4X02", "solo nella sua transazione",
		`INSERT INTO attestazione_fascicolo_documento (attestazione_id, thread_id, documento_id, componente_id, tipo, rev, sha256)
		 VALUES ('{AT1}', '{T}', '{D2}', '{S}', 'disegno_2d', 'A', repeat('e', 64))`)
	rifiutoAtteso(t, p, "S56 documento attestato diverso dal documento", "P4X02", "non e' corrente o non e' quello registrato",
		att("2", "'{AT1}'", "{C1}"),
		`INSERT INTO attestazione_fascicolo_documento (attestazione_id, thread_id, documento_id, componente_id, tipo, rev, sha256)
		 VALUES ('00000000-0000-4000-8000-0000002211a7', '{T}', '{D2}', '{S}', 'disegno_2d', 'A', repeat('f', 64))`)
	rifiutoAtteso(t, p, "S58 TRUNCATE delle attestazioni", "P4X02", "attestazione_fascicolo (TRUNCATE)", `TRUNCATE attestazione_fascicolo CASCADE`)
	// S53: con una conferma n. 2 di P, l'attestazione sulla n. 1 si rifiuta
	riuscita(t, p, "conferma n. 2", conferma2("0", "0", "2", "nessuno"), radiceC2, figlioC2, arcoC2)
	rifiutoAtteso(t, p, "S53 attestazione su una conferma superata", "P4X02", "si attesta sull'ultima", att("2", "'{AT1}'", "{C1}"))
	riuscita(t, p, "attestazione sull'ultima conferma", att("2", "'{AT1}'", "{C2}"))
	// S61: un UPDATE identico di una deroga non scrive storia
	prima := testutil.Conta(t, p, "deroga_fabbisogno_storia")
	esegui11(t, p, `UPDATE deroga_fabbisogno SET motivo = motivo WHERE deroga_id = '{DF}'`)
	if n := testutil.Conta(t, p, "deroga_fabbisogno_storia"); n != prima {
		t.Errorf("un UPDATE identico della deroga ha scritto %d righe di storia", n-prima)
	}
	esegui11(t, p, `UPDATE deroga_fabbisogno SET motivo = 'motivo nuovo' WHERE deroga_id = '{DF}'`)
	if n := testutil.Conta(t, p, "deroga_fabbisogno_storia"); n != prima+1 {
		t.Errorf("un UPDATE vero della deroga: %d righe di storia nuove, attesa 1", n-prima)
	}
}
