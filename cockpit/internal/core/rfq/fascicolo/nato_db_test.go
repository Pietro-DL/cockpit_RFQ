//go:build integrazione

// L4 — l'editor della struttura, dopo l'accettazione di un nodo nuovo, rilegge il componente con il codice
// del piano. Se nel frattempo la proposta ha cambiato codice, quel componente non c'e': prima era un «no
// rows in result set» grezzo, adesso e' un rifiuto che dice di riaprire l'editor. Il caso vero e' una corsa
// fra due transazioni, che una prova non sa fermare a meta'; qui si prova la lettura che la chiude.

package fascicolo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func TestUnComponenteNonNatoEUnRifiutoNonUnErroreGrezzo(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	var utente, cliente, thread uuid.UUID
	for _, r := range []struct {
		sql string
		arg []any
		dst *uuid.UUID
	}{
		{`INSERT INTO utente (sigla, nome, ufficio) VALUES ('NT', 'Prova', 'Tecnico') RETURNING utente_id`, nil, &utente},
		{`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'ACME') RETURNING cliente_id`, nil, &cliente},
	} {
		if err := p.QueryRow(ctx, r.sql, r.arg...).Scan(r.dst); err != nil {
			t.Fatal(err)
		}
	}
	if err := p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa)
		VALUES ($1, 'outlook', now(), 'ACME\WIP\rfq') RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	q := db.New(p)

	_, err := componenteNato(ctx, q, thread, "PZ-001")
	var r Rifiuto
	if !errors.As(err, &r) || !strings.Contains(string(r), "riapri l'editor") {
		t.Fatalf("un componente che non e' nato: %v, atteso un rifiuto che dica di riaprire l'editor", err)
	}

	if _, err := p.Exec(ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, 'PZ-001', 'sciolto', $2)`,
		thread, utente); err != nil {
		t.Fatal(err)
	}
	c, err := componenteNato(ctx, q, thread, "pz-001")
	if err != nil || c.Codice != "PZ-001" {
		t.Fatalf("il componente nato non si ritrova: %+v, %v", c, err)
	}
}
