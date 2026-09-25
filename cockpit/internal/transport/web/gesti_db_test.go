//go:build integrazione

// L4 — i gesti sul Fascicolo dopo la revisione del 25/09: il codice scritto a mano passa dalla stessa
// guardia di ogni altro codice (al massimo 40 caratteri, niente spazi), e la RFQ si blocca PRIMA di
// componenti, documenti e proposte, come nel congelamento. Con l'ordine rovesciato due transazioni si
// aspettavano a vicenda e PostgreSQL ne uccideva una (40P01), che arrivava all'operatore come SQLSTATE.
package web

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/platform/db"
)

// La correzione del codice di un componente e la conferma di un file rifiutano un codice che nessun
// altro gesto accetterebbe: con uno spazio, o oltre i 40 caratteri. Niente cambia.
func TestIlCodiceScrittoAManoPassaDallaStessaGuardia(t *testing.T) {
	b := preparaBancoWeb(t)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "GUARDIA")
	comp := r.componente("PZ-001")
	w := operatore(b)

	for _, cattivo := range []string{"PZ 002", strings.Repeat("P", 41)} {
		a := r.correggiCodice(w, comp, cattivo)
		if !strings.Contains(a, "caratteri non ammessi") {
			t.Errorf("correzione in %q: %q", cattivo, a)
		}
		if got := b.codiceComponente(comp); got != "PZ-001" {
			t.Fatalf("la correzione rifiutata ha cambiato il codice in %q", got)
		}
	}
	if a := r.correggiCodice(w, comp, ""); !strings.Contains(a, "il codice non può essere vuoto") {
		t.Errorf("codice vuoto: %q", a)
	}
	if a := r.correggiCodice(w, comp, strings.Repeat("P", 40)); !strings.Contains(a, "Codice corretto") {
		t.Errorf("un codice di 40 caratteri si corregge: %q", a)
	}

	for _, c := range []struct {
		form  url.Values
		frase string
	}{
		{url.Values{"tipo": {"disegno_2d"}, "codice": {"PZ 003"}}, "il codice ha più di 40 caratteri o caratteri non ammessi"},
		{url.Values{"tipo": {"disegno_2d"}, "codice": {strings.Repeat("Q", 41)}}, "il codice ha più di 40 caratteri o caratteri non ammessi"},
		{url.Values{"tipo": {"disegno_2d"}, "codice": {"PZ-003"}, "rev": {"A B"}}, "la revisione ha più di 10 caratteri o caratteri non ammessi"},
	} {
		p := r.proposta("disegno.pdf", "", uuid.Nil)
		if a := confermaProposta(w, p, c.form); !strings.Contains(a, c.frase) {
			t.Errorf("conferma con %v: manca %q in\n%s", c.form, c.frase, estrai(a, "avviso"))
		}
		if st := b.fotoProposta(p); !strings.Contains(st, "stato=aperta") {
			t.Errorf("la conferma rifiutata ha deciso la proposta: %s", st)
		}
	}
	if n := r.conta(`SELECT count(*) FROM documento WHERE thread_id = $1`, r.thread); n != 0 {
		t.Errorf("le conferme rifiutate hanno creato %d documenti", n)
	}
}

// Mentre un'altra transazione tiene la RFQ (un congelamento, un gesto sulla struttura), l'assegnazione,
// la correzione del codice e la conferma si mettono in fila SULLA RFQ, senza aver ancora bloccato niente
// di loro: il documento, il componente e la proposta restano liberi. Con l'ordine di prima li tenevano
// gia' bloccati mentre aspettavano la RFQ, e bastava che l'altra transazione li chiedesse per un 40P01.
func TestIGestiSulFascicoloBloccanoPrimaLaRFQ(t *testing.T) {
	b := preparaBancoWeb(t)
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, "LUCCHETTI")
	vecchio := r.componente("PZ-001")
	nuovo := r.componente("PZ-002")
	doc := r.documento("d1.pdf", "PZ-001", vecchio, db.StatoNasErrore)
	prop := r.proposta("d2.pdf", "PZ-002", uuid.Nil)
	w := operatore(b)

	casi := []struct {
		nome, riga string
		id         uuid.UUID
		gesto      func() string
		esito      string
	}{
		{"assegna", "documento WHERE documento_id", doc, func() string {
			return r.assegna(w, url.Values{"componente": {nuovo.String()}, "documento": {doc.String()}, "correggi_codice": {"1"}})
		}, "assegnat"},
		{"codice del componente", "componente WHERE componente_id", nuovo, func() string {
			return r.correggiCodice(w, nuovo, "PZ-003")
		}, "Codice corretto"},
		{"conferma", "documento_proposta WHERE proposta_id", prop, func() string {
			return confermaProposta(w, prop, url.Values{"tipo": {"disegno_2d"}, "codice": {"PZ-002"}})
		}, "Confermato"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			tx, err := b.pool.Begin(b.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(b.ctx)
			// l'altra transazione: tiene la RFQ, come CongelaBom
			if _, err := db.New(tx).BloccaThread(b.ctx, r.thread); err != nil {
				t.Fatal(err)
			}
			risposta := make(chan string, 1)
			go func() { risposta <- c.gesto() }()
			limite := time.Now().Add(10 * time.Second)
			for r.conta(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`) == 0 {
				if time.Now().After(limite) {
					t.Fatal("il gesto non si e' messo in fila sulla RFQ")
				}
				time.Sleep(20 * time.Millisecond)
			}
			// la riga del gesto e' libera: il gesto aspetta la RFQ senza tenere niente di suo
			ctx, annulla := context.WithTimeout(b.ctx, 5*time.Second)
			defer annulla()
			_, err = tx.Exec(ctx, `SELECT 1 FROM `+c.riga+` = $1 FOR UPDATE NOWAIT`, c.id)
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "55P03" {
				t.Errorf("%s: la riga era gia' bloccata dal gesto in attesa della RFQ: l'ordine e' rovesciato", c.nome)
			} else if err != nil {
				t.Fatal(err)
			}
			if err := tx.Rollback(b.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Fatal(err)
			}
			if a := <-risposta; !strings.Contains(a, c.esito) {
				t.Errorf("%s dopo l'attesa: %q", c.nome, estrai(a, "avviso"))
			}
		})
	}
}
