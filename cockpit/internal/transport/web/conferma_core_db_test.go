//go:build integrazione

// L4 — direttiva 35, blocco C-3 / direttiva 39 (S8 passo 2): la conferma del documento resa funzione del core
// (confermadoc.Conferma) con la guardia di INC-G9, provata dalla rotta legacy «Conferma il documento»: con una
// associazione registrata attiva il documento entra solo sotto il componente della AR; un altro componente, o nessun
// componente, e' un rifiuto e non scrive niente. Senza AR la conferma e' quella di sempre (le prove legacy del package
// restano le stesse).

package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/testutil"
)

// arAttivaWeb: una AR attiva dell'allegato sul componente, con la sua conferma formato 1 minima, scritta a mano con i
// trigger delle tre tabelle spenti solo nella sua transazione (come arAttivaDiProva di distintabozza: la nascita vera
// della AR e' di R5).
func (r *rfqFascicolo) arAttivaWeb(prodotto, allegato, componente uuid.UUID) {
	r.b.t.Helper()
	ctx := context.Background()
	tx, err := r.b.pool.Begin(ctx)
	if err != nil {
		r.b.t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	conferma := uuid.New()
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			VALUES ($1, $2, $3, (SELECT COALESCE(max(numero), 0) + 1 FROM conferma_strutturale WHERE prodotto_id = $3),
				(SELECT conferma_id FROM conferma_strutturale WHERE prodotto_id = $3 ORDER BY numero DESC LIMIT 1), 1, true,
				repeat('3', 64), 0, 0, 0, '{"rotta":"prova"}', '{"formato":1}', repeat('9', 64), '{}', '{}', 'nessuno', $4)`,
			[]any{conferma, r.thread, prodotto, r.utente}},
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, codice, tipo)
			SELECT $1, c.thread_id, c.componente_id, c.codice, c.tipo FROM componente c WHERE c.componente_id = $2`, []any{conferma, componente}},
		{`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
				provenienza_pb, chiave_idempotenza, creata_da)
			VALUES (gen_random_uuid(), $1, $2, $3, 'conferma_bom', $4, 'k-prova', 'manuale', gen_random_uuid(), $5)`, []any{r.thread, allegato, componente, conferma, r.utente}},
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			r.b.t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		r.b.t.Fatal(err)
	}
}

func TestV1S8LaConfermaDelCoreConLaGuardiaDellaAR(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "G9")
	prodotto := r.componenteTipo("7120001", "finito")
	p := r.componenteTipo("7720001", "sciolto")
	altro := r.componenteTipo("7720002", "sciolto")
	r.arco(prodotto, p, 1)
	r.arco(prodotto, altro, 1)
	pid, all := r.propostaDa("7720001.pdf", "7720001")
	// la copia derivata segue la AR (invariante (e) della 0023)
	testutil.CopiaLegacy(r.b.t, r.b.pool, `UPDATE documento_proposta SET componente_id = $2 WHERE proposta_id = $1`, pid, p)
	r.arAttivaWeb(prodotto, all, p)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	conferma := func(form url.Values) string {
		t.Helper()
		_, html := w.daFascicolo(http.MethodPost, "/proposta/"+pid.String()+"/conferma", form, r.thread, "")
		return avvisoF(html) + leggibile(html)
	}
	documenti := func() int { return r.conta(`SELECT count(*) FROM documento WHERE thread_id = $1`, r.thread) }

	// un altro componente nel form: rifiuto, niente scritto
	if h := conferma(url.Values{"componente_id": {altro.String()}}); !strings.Contains(h, "associazione registrata con 7720001") {
		t.Fatalf("componente diverso dalla AR: atteso il rifiuto, avuto:\n%s", h)
	}
	if n := documenti(); n != 0 {
		t.Fatalf("documenti dopo il rifiuto = %d, attesi 0", n)
	}
	if n := r.conta(`SELECT count(*) FROM documento_proposta WHERE proposta_id = $1 AND stato = 'aperta'`, pid); n != 1 {
		t.Fatal("la proposta non e' piu' aperta dopo il rifiuto")
	}
	// lo stesso componente della AR (dal form, o dalla copia): il documento entra sotto di lui
	if h := conferma(url.Values{"componente_id": {p.String()}}); !strings.Contains(h, "Assegnato al componente 7720001") {
		t.Fatalf("componente della AR: attesa la conferma, avuto:\n%s", h)
	}
	if n := r.conta(`SELECT count(*) FROM documento WHERE thread_id = $1 AND componente_id = $2`, r.thread, p); n != 1 {
		t.Fatalf("documenti sotto il componente della AR = %d, atteso 1", n)
	}

	// senza componente con una AR attiva: rifiuto (la copia tolta a mano, il caso che l'invariante non lascia nascere)
	pid2, all2 := r.propostaDa("7720002.pdf", "7720002")
	testutil.CopiaLegacy(r.b.t, r.b.pool, `UPDATE documento_proposta SET componente_id = $2 WHERE proposta_id = $1`, pid2, altro)
	r.arAttivaWeb(prodotto, all2, altro)
	r.esegui(`ALTER TABLE documento_proposta DISABLE TRIGGER USER`)
	r.esegui(`UPDATE documento_proposta SET componente_id = NULL, codice = '7720002' WHERE proposta_id = $1`, pid2)
	r.esegui(`ALTER TABLE documento_proposta ENABLE TRIGGER USER`)
	_, html := w.daFascicolo(http.MethodPost, "/proposta/"+pid2.String()+"/conferma", url.Values{}, r.thread, "")
	if h := avvisoF(html) + leggibile(html); !strings.Contains(h, "non senza componente") {
		t.Fatalf("senza componente con la AR: atteso il rifiuto, avuto:\n%s", h)
	}
	if n := r.conta(`SELECT count(*) FROM documento WHERE thread_id = $1 AND componente_id IS DISTINCT FROM $2`, r.thread, p); n != 0 {
		t.Fatalf("documenti scritti dal rifiuto = %d", n)
	}

	// senza AR: la conferma di sempre (anche senza componente)
	pid3, _ := r.propostaDa("capitolato.pdf", "")
	r.esegui(`UPDATE documento_proposta SET tipo_proposto = 'capitolato' WHERE proposta_id = $1`, pid3)
	_, html = w.daFascicolo(http.MethodPost, "/proposta/"+pid3.String()+"/conferma", url.Values{}, r.thread, "")
	if h := avvisoF(html) + leggibile(html); !strings.Contains(h, "Senza componente") {
		t.Fatalf("senza AR: attesa la conferma di sempre, avuto:\n%s", h)
	}
}
