//go:build integrazione

// L4 — impronta-ingressi-2 (direttiva 35, blocco C-1c; PD35 §6.2, INC-G12): la regola unica di cio' che R5 scrive nella
// parte proposta. La copia derivata scritta e tolta come la scrive il servizio delle associazioni non cambia la parte;
// le righe che R5 segna escono con l'elenco prima e con il segno dopo (H_S = H_T, CN-05b); le AR entrano solo per gli
// allegati che la bozza del prodotto cita, viste come prima della transazione corrente. Solo dati sintetici.

package distintabozza

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func TestV1C1cImprontaIngressi2(t *testing.T) {
	sc := preparaBozza3(t)
	ctx := context.Background()
	parte := func(r5 *RigheDiR5) string {
		t.Helper()
		tx, err := sc.p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		x, err := PartePropostaDi(ctx, db.New(tx), sc.thread, sc.prodotto, "regole-di-prova", r5)
		if err != nil {
			t.Fatalf("parte proposta: %v", err)
		}
		return x.Impronta
	}
	if AlgoritmoIngressi != "impronta-ingressi-2" {
		t.Fatalf("algoritmo: %s", AlgoritmoIngressi)
	}
	a := parte(nil)

	// (1) la copia della proposta del PDF scritta come la scrive il servizio (componente, codice del componente, lettura in
	// dettagli.codice_letto) e poi tolta (la lettura rimessa): la parte non cambia
	testutil.CopiaLegacy(t, sc.p, `UPDATE documento_proposta SET componente_id = c.componente_id, codice = c.codice,
		dettagli = documento_proposta.dettagli || jsonb_build_object('codice_letto', documento_proposta.codice)
		FROM componente c WHERE documento_proposta.proposta_id = $1 AND c.componente_id = $2`, sc.proposta, sc.sotto)
	if got := parte(nil); got != a {
		t.Error("la copia scritta ha cambiato la parte proposta")
	}
	testutil.CopiaLegacy(t, sc.p, `UPDATE documento_proposta SET componente_id = NULL, codice = dettagli ->> 'codice_letto', dettagli = dettagli - 'codice_letto'
		WHERE proposta_id = $1`, sc.proposta)
	if got := parte(nil); got != a {
		t.Error("la copia tolta ha cambiato la parte proposta")
	}

	// (2) una riga di componente_proposta che R5 segnera': con l'elenco di R5 la parte prima e dopo il segno e' la stessa
	var cp uuid.UUID
	if err := sc.p.QueryRow(ctx, `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza)
		VALUES ($1, $2, $3, '#77', 'Cordone', 'step', 50) RETURNING proposta_id`, sc.thread, sc.id.allegato, sc.id.sha).Scan(&cp); err != nil {
		t.Fatal(err)
	}
	b := parte(nil)
	if b == a {
		t.Fatal("una riga di proposta nuova non cambia la parte")
	}
	elenco := &RigheDiR5{AllegatiCitati: []uuid.UUID{sc.id.pdf}, Proposte: []uuid.UUID{cp}}
	hs := parte(elenco)
	esegui3(t, sc.p, `UPDATE componente_proposta SET stato = 'scartata', deciso_da = $2, deciso_il = now(),
		evidenza = evidenza || '{"albero": {"formato": 3, "gesto": "conferma_bom", "decisione": "non_includere"}}'::jsonb WHERE proposta_id = $1`,
		cp, sc.id.utente)
	if ht := parte(elenco); ht != hs {
		t.Error("H_S e H_T diverse per una riga che R5 segna (CN-05b)")
	}
	if got := parte(nil); got != hs {
		t.Error("la riga col segno di R5 entra ancora nella parte di R1")
	}

	// (3) le AR: solo quelle degli allegati che la bozza cita (il PDF della PB); lo STEP non e' citato
	c := parte(nil)
	arStep := arAttivaDiProva(t, sc.p, sc.thread, sc.prodotto, sc.id.allegato, sc.sotto, sc.id.utente)
	if got := parte(nil); got != c {
		t.Error("la AR di un allegato che la bozza non cita ha cambiato la parte")
	}
	ar := arConLaStessaConferma(t, sc, arStep, sc.id.pdf)
	d := parte(nil)
	if d == c {
		t.Fatal("la AR dell'allegato della PB non cambia la parte")
	}
	if got := parte(&RigheDiR5{AllegatiCitati: []uuid.UUID{sc.id.pdf}}); got != d {
		t.Error("l'elenco degli allegati citati uguale alla bozza da' un'altra parte")
	}
	if got := parte(&RigheDiR5{}); got == d {
		t.Error("senza allegati citati la AR del PDF entra ancora")
	}
	// vista come prima della transazione: chiusa qui e' ancora attiva, nata qui non c'e'
	tx, err := sc.p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// i trigger della tabella spenti solo in questa transazione (annullata alla fine): la chiusura scrive da se' quello che
	// scriverebbe trg_fra_cambia (chiusa_il, chiusa_tx di questa transazione)
	if _, err := tx.Exec(ctx, `ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	var conferma uuid.UUID
	if err := tx.QueryRow(ctx, `UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = $2, motivo_chiusura = 'scollega_file',
		chiusa_il = now(), chiusa_tx = pg_current_xact_id() WHERE associazione_id = $1 RETURNING conferma_id`, ar, sc.id.utente).Scan(&conferma); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`INSERT INTO file_rfq_associazione (thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza, provenienza_pb, chiave_idempotenza, creata_da)
		 VALUES ('` + sc.thread.String() + `', '` + sc.id.pdf.String() + `', '` + sc.prodotto.String() + `', 'conferma_bom', '` + conferma.String() +
			`', 'k-prova', 'manuale', gen_random_uuid(), '` + sc.id.utente.String() + `')`} {
		if _, err := tx.Exec(ctx, s); err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	nella, err := PartePropostaDi(ctx, db.New(tx), sc.thread, sc.prodotto, "regole-di-prova", nil)
	if err != nil {
		t.Fatal(err)
	}
	if nella.Impronta != d {
		t.Error("nella transazione che chiude e fa nascere, la parte non e' quella di prima")
	}
}

// arConLaStessaConferma: una seconda AR attiva (allegato → lo stesso componente) con la conferma e il componente
// certificato della AR data (arAttivaDiProva ne scrive una per conferma formato 1), con i trigger della tabella spenti
// solo nella transazione.
func arConLaStessaConferma(t *testing.T, sc scena3DB, di, allegato uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	tx, err := sc.p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	id := uuid.New()
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`, nil},
		{`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
				provenienza_pb, chiave_idempotenza, creata_da)
			SELECT $1, r.thread_id, $3, r.componente_id, 'conferma_bom', r.conferma_id, 'k-prova', 'manuale', gen_random_uuid(), r.creata_da
			  FROM file_rfq_associazione r WHERE r.associazione_id = $2`, []any{id, di, allegato}},
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}
