//go:build integrazione

// L4 — V1, ondata B, corsia persistenza (PM23 R-24; EM §2.3, §9.10): «Autorizza» e «Revoca» scrivono la loro riga della
// storia della fonte STEP del prodotto (fonte_step_evento, ruolo prodotto), nella transazione dell'atto: un atto
// rifiutato non scrive niente. «Conferma la fonte» sta in transport/web (motore_a_bozza_v1_db_test.go). Dati inventati
// (la RFQ ACME del banco del Fascicolo).

package fascicolo_test

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// storiaFonte: gli eventi della storia della fonte del prodotto, «azione:ruolo:documento:motivo», in ordine.
func (b *banco) storiaFonte(prodotto uuid.UUID) string {
	b.t.Helper()
	return uno[string](b, `SELECT coalesce(string_agg(azione || ':' || ruolo || ':' || coalesce(documento_id::text, '-') || ':' || coalesce(motivo, '-'),
		' ' ORDER BY evento_id), '') FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2`, b.thread, prodotto)
}

func TestV1AutorizzaERevocaScrivonoLaStoriaDellaFonte(t *testing.T) {
	b := nuovoBanco(t)
	prodotto, doc, a := b.prodottoProfondo()
	// un atto rifiutato (la firma di un'anteprima vecchia) non scrive niente
	if _, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.DichiaraStrutturale(b.ctx, q, b.thread, b.utente, fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc,
			Firma: "firma-vecchia", Autorizza: true, PresaDAtto: true})
	}); err == nil {
		t.Fatal("autorizzazione con una firma vecchia accettata")
	}
	if s := b.storiaFonte(prodotto); s != "" {
		t.Fatalf("un atto rifiutato ha scritto la storia della fonte: %s", s)
	}
	b.dichiaraOk(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	atteso := "autorizzazione:prodotto:" + doc.String() + ":-"
	if s := b.storiaFonte(prodotto); s != atteso {
		t.Fatalf("dopo «Autorizza»: %q, atteso %q", s, atteso)
	}
	if n := uno[int](b, `SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND allegato_id = $3 AND sha256 = $4 AND autore_id = $5`,
		b.thread, prodotto, a.AllegatoID, a.Sha256.String, b.utente); n != 1 {
		t.Fatalf("allegato, sha e autore della riga: %d", n)
	}
	// la stessa autorizzazione di nuovo (se l'atto la accetta) non scrive una seconda riga
	_, _ = b.dichiara(fascicolo.RichiestaAutorizzazione{Componente: prodotto, Documento: doc})
	if s := b.storiaFonte(prodotto); s != atteso {
		t.Fatalf("dopo una seconda «Autorizza» dello stesso file: %q", s)
	}
	// «Revoca»: la riga con il motivo
	if _, err := b.revoca(prodotto, ""); err != nil {
		t.Fatalf("revoca: %v", err)
	}
	atteso += " revoca:prodotto:" + doc.String() + ":prova"
	if s := b.storiaFonte(prodotto); s != atteso {
		t.Fatalf("dopo «Revoca»: %q, atteso %q", s, atteso)
	}
	// una revoca rifiutata (niente da revocare) non scrive
	if _, err := b.revoca(prodotto, ""); err == nil {
		t.Fatal("seconda revoca accettata")
	}
	if s := b.storiaFonte(prodotto); s != atteso {
		t.Fatalf("dopo una revoca rifiutata: %q", s)
	}
}
