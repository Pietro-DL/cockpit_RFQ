//go:build integrazione

package migrazioni_test

// L4 — T-14, la parte del database (PM23 §11; EM §3.4.2; PM23 §4.6): gli stessi casi della parte L1
// (distintabozza.CasiT14, che li confronta con il validatore delle grammatiche) passano dal CHECK ck_csc_identita di
// conferma_strutturale_componente, e il CHECK da' lo stesso esito di EmittenteAmmissibile e NamespaceAmmissibile, tetti
// di sicurezza compresi. Il confronto delle minuscole del CHECK usa lower() del database: il caso con le lettere non
// ASCII maiuscole dice se lower() le conosce (dipende dalla LC_CTYPE del database).

import (
	"context"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

// letteraleSQL: una stringa come letterale SQL.
func letteraleSQL(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func TestV1T14LaRegolaDellEmittenteDelDatabase(t *testing.T) {
	p := schema23ConStoria(t)
	ctx := context.Background()
	var ctype string
	if err := p.QueryRow(ctx, `SELECT datctype FROM pg_database WHERE datname = current_database()`).Scan(&ctype); err != nil {
		t.Fatal(err)
	}
	var piega bool
	if err := p.QueryRow(ctx, `SELECT lower('È') <> 'È'`).Scan(&piega); err != nil {
		t.Fatal(err)
	}
	t.Logf("LC_CTYPE del database di prova: %s; lower() conosce le maiuscole non ASCII: %v", ctype, piega)
	for _, c := range distintabozza.CasiT14() {
		t.Run(c.Nome, func(t *testing.T) {
			if c.MaiuscolaNonASCII && !piega {
				t.Skipf("SALTATA-AMBIENTE: con LC_CTYPE %s lower() del database non conosce le maiuscole non ASCII, e il CHECK "+
					"non puo' dare l'esito del Go (che rifiuta): differenza da decidere (resoconto della corsia persistenza)", ctype)
			}
			emittente, namespace := letteraleSQL("partner:beta"), "NULL"
			if c.Partner != "" {
				emittente = letteraleSQL("partner:" + c.Partner)
			}
			if c.Namespace != "" {
				namespace = letteraleSQL(c.Namespace)
			}
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			if _, err := tx.Exec(ctx, "ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER"); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, insertCon("conferma_strutturale_componente",
				map[string]string{"emittente": emittente, "emittente_presunto": "false", "namespace": namespace}))
			accettato := err == nil
			if !accettato && vincoloViolato(err) != "ck_csc_identita" {
				t.Fatalf("la riga si ferma su un altro vincolo: %v", err)
			}
			if accettato != c.Atteso {
				t.Errorf("CHECK ck_csc_identita: accettato = %v, atteso %v", accettato, c.Atteso)
			}
			if c.Partner != "" && distintabozza.EmittenteAmmissibile("partner:"+c.Partner) != accettato {
				t.Errorf("EmittenteAmmissibile e il CHECK non dicono la stessa cosa")
			}
			if c.Namespace != "" && distintabozza.NamespaceAmmissibile(c.Namespace) != accettato {
				t.Errorf("NamespaceAmmissibile e il CHECK non dicono la stessa cosa")
			}
		})
	}
}
