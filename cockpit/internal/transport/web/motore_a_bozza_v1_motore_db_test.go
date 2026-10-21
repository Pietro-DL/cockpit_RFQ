//go:build integrazione

// Gli aiuti delle prove L4 con il Motore A vero in R4 e R5 (motore_a_bozza_v1_motore.go; direttiva 39, blocco «R4 completo»
// della corsia persistenza): le regole ACME pronte per il cliente della RFQ della prova.

package web

import (
	"testing"
	"time"

	"github.com/google/uuid"

	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// accendiMotoreV1: le regole ACME pronte (mwRegoleACME) per il cliente della RFQ, sul server della prova, fino alla fine
// della prova; niente se sono gia' pronte.
func accendiMotoreV1(t *testing.T, b *bancoWeb, thread uuid.UUID) {
	t.Helper()
	if b.ws.MotoreA.Stato == motorea_web.StatoPronta {
		return
	}
	var cliente uuid.UUID
	var ragione string
	if err := b.pool.QueryRow(b.ctx, `SELECT c.cliente_id, c.ragione_sociale FROM thread_offerta t JOIN cliente c ON c.cliente_id = t.cliente_id
		WHERE t.thread_id = $1`, thread).Scan(&cliente, &ragione); err != nil {
		t.Fatal(err)
	}
	mwAccendi(t, b, mwRegoleACME(t, cliente, ragione, 60*time.Second))
}
