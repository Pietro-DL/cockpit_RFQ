//go:build integrazione

package web

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// La pagina di un cliente cancella solo le persone di quel cliente: l'id del buyer arriva da un form,
// e prima bastava cambiarlo per cancellare il buyer di un altro cliente. E un buyer che una proposta
// di triage cita non si cancella: la chiave esterna lo rifiuterebbe con un errore grezzo in pagina.
func TestUnBuyerSiCancellaSoloDallaPaginaDelSuoCliente(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	q := db.New(p)

	cliente := func(cartella string) uuid.UUID {
		var id uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1, $1) RETURNING cliente_id`,
			cartella).Scan(&id); err != nil {
			t.Fatalf("cliente %s: %v", cartella, err)
		}
		return id
	}
	acme, beta := cliente("ACME"), cliente("BETA")
	buyer := func(c uuid.UUID, cognome string) uuid.UUID {
		b, err := q.InsertBuyer(ctx, db.InsertBuyerParams{ClienteID: c, Cognome: cognome, Tipo: db.TipoBuyerBuyer, Origine: db.OrigineAnagraficaManuale})
		if err != nil {
			t.Fatalf("buyer %s: %v", cognome, err)
		}
		return b.BuyerID
	}
	diBeta := buyer(beta, "Rossi")

	n, err := q.EliminaBuyer(ctx, db.EliminaBuyerParams{BuyerID: diBeta, ClienteID: acme})
	if err != nil || n != 0 {
		t.Fatalf("dalla pagina di ACME si è cancellato il buyer di BETA: n=%d err=%v", n, err)
	}
	n, err = q.EliminaBuyer(ctx, db.EliminaBuyerParams{BuyerID: diBeta, ClienteID: beta})
	if err != nil || n != 1 {
		t.Fatalf("il buyer di BETA non si cancella dalla sua pagina: n=%d err=%v", n, err)
	}

	// un buyer citato solo da una proposta di triage resta, senza errore
	citato := buyer(acme, "Bianchi")
	var conv, mid uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', 'CONV-BUYER', now()) RETURNING conversazione_id`).Scan(&conv); err != nil {
		t.Fatalf("conversazione: %v", err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento)
		VALUES ('outlook', '<buyer-citato@acme.example>', $1, 'entrata', now()) RETURNING messaggio_id`, conv).Scan(&mid); err != nil {
		t.Fatalf("messaggio: %v", err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO proposta_triage (messaggio_id, esito, buyer_proposto, confidenza, fonte)
		VALUES ($1, 'nuova_rfq', $2, 60, 'deterministico')`, mid, citato); err != nil {
		t.Fatalf("proposta: %v", err)
	}
	n, err = q.EliminaBuyer(ctx, db.EliminaBuyerParams{BuyerID: citato, ClienteID: acme})
	if err != nil || n != 0 {
		t.Fatalf("un buyer citato da una proposta: n=%d err=%v (atteso: resta, senza errore)", n, err)
	}
}
