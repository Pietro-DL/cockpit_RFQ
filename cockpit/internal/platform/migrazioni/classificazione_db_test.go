//go:build integrazione

// L4 — la 0016 (blocco 7C.0) rinomina lo stato `risposta` della richiesta a un fornitore in
// `offerta_ricevuta`. Una richiesta gia' in `risposta` era stata chiusa con la regola vecchia (una
// risposta qualunque), e nessuno sa se quella risposta fosse un'offerta: la migrazione NON la
// reinterpreta. Si ferma, dice quante sono, e non registra la propria versione.

package migrazioni_test

import (
	"context"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func TestLa0016SiFermaSeCiSonoRichiesteInRispostaENonLeReinterpreta(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaFinoA(t, p, 15)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()

	// una richiesta chiusa con la regola di prima della 0016
	if _, err := p.Exec(ctx, `
		INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale, regole)
		     VALUES ('10000000-0000-0000-0000-000000000016', 'PROVA0016', 'Prova 0016', '{}');
		INSERT INTO fornitore (fornitore_id, ragione_sociale, tipo)
		     VALUES ('20000000-0000-0000-0000-000000000016', 'Fornitore 0016', 'processi');
		INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio)
		     VALUES ('30000000-0000-0000-0000-000000000016', '10000000-0000-0000-0000-000000000016', 'outlook', now());
		INSERT INTO richiesta_fornitore (thread_id, fornitore_id, stato, risposta_il)
		     VALUES ('30000000-0000-0000-0000-000000000016', '20000000-0000-0000-0000-000000000016', 'risposta', now());`); err != nil {
		t.Fatal(err)
	}

	_, err := migrazioni.ApplicaFinoA(ctx, p, risorse.FS, 16, testutil.LogSilenzioso())
	if err == nil {
		t.Fatal("la 0016 è passata sopra una richiesta in stato risposta: doveva fermarsi")
	}
	if !strings.Contains(err.Error(), "non le reinterpreta") || !strings.Contains(err.Error(), "1 richieste in stato risposta e 1 con risposta_il") {
		t.Fatalf("il messaggio deve dire quante sono e che cosa fare: %v", err)
	}
	applicate, err := migrazioni.Applicate(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if applicate[16] {
		t.Fatal("la 0016 fallita risulta registrata")
	}
	var stato string
	if err := p.QueryRow(ctx, `SELECT stato::text FROM richiesta_fornitore`).Scan(&stato); err != nil || stato != "risposta" {
		t.Fatalf("la riga deve essere rimasta com'era: %q, %v", stato, err)
	}

	// riconciliata a mano (qui: era solo collegata, resta inviata), la migrazione passa e rinomina
	if _, err := p.Exec(ctx, `UPDATE richiesta_fornitore SET stato = 'inviata', risposta_il = NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := migrazioni.ApplicaFinoA(ctx, p, risorse.FS, 17, testutil.LogSilenzioso()); err != nil {
		t.Fatalf("dopo la riconciliazione la 0016 e la 0017 devono passare: %v", err)
	}
	var valori string
	if err := p.QueryRow(ctx, `SELECT enum_range(NULL::stato_richiesta_fornitore)::text`).Scan(&valori); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(valori, ",risposta,") || !strings.Contains(valori, "offerta_ricevuta") || !strings.Contains(valori, "declinata") {
		t.Fatalf("enum dopo la 0016: %s", valori)
	}
	var quadrante string
	if err := p.QueryRow(ctx, `SELECT 'interni'::text WHERE EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'v_inbox' AND column_name = 'quadrante')`).Scan(&quadrante); err != nil {
		t.Fatalf("v_inbox senza la colonna quadrante dopo la 0017: %v", err)
	}
}
