//go:build integrazione

// La scena delle prove del giro G39 della corsia motore (dati sintetici): una RFQ del cliente di prova ACME in
// FATTIBILITA con il prodotto finito 9110001A, che la grammatica ACME di prova legge (base 9110001, marcatore A), senza
// working; le regole ACME pronte per il cliente e la sessione dell'amministratore.
package web

import (
	"testing"
	"time"

	"github.com/google/uuid"

	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

type scenaG39 struct {
	*rfqFascicolo
	prodotto uuid.UUID
	regole   motorea_web.Regole
	ad       *browser
}

func (b *bancoWeb) scenaG39(t *testing.T, chiave string) *scenaG39 {
	t.Helper()
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	r.fase("FATTIBILITA")
	s := &scenaG39{rfqFascicolo: r, prodotto: r.componenteTipo("9110001A", "finito")}
	cliente := uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, r.thread)
	s.regole = mwRegoleACME(t, cliente, "Acme S.p.A.", 30*time.Second)
	mwAccendi(t, b, s.regole)
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	return s
}

// stepG39: uno STEP arrivato nella RFQ, letto con i fatti dati, con la proposta di documento cad_3d che il worker scrive
// (codice: il codice letto dal nome dal vecchio lettore, "" per nessuno).
func (s *scenaG39) stepG39(nome, codice, fatti string) (uuid.UUID, string) {
	s.b.t.Helper()
	a, sha := s.allegatoExt(nome, "stp")
	s.analisiCorrente(sha, fatti)
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', NULLIF($3, ''), 90, 'nome_file')`,
		a, s.thread, codice)
	return a, sha
}
