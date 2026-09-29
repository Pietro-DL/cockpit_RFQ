//go:build integrazione

// L4 — Smistamento 4.13b, correzione 1: il numero d'ordine del cliente (la voce `numero_ordine`, «ODA_0001234»)
// non entra nei codici che il nome di un file cita quando il file scende in staging (scriviProposta): e' lo
// stesso `codici_nel_nome` che scrive l'ingest, e il Fascicolo lo rilegge come un codice «nel nome di …». Il
// codice vero citato accanto all'ordine resta. La controprova e' lo stesso file per un cliente senza la voce.
// Cliente e numeri inventati.

package workerapi

import (
	"testing"

	"github.com/google/uuid"
)

const regoleConOrdine = `{"numero_ordine": {"regex": "\\bODA_\\d{7}\\b", "descrizione": "ordini ODA", "esempio": "ODA_0001234"}}`

func TestIlNumeroDOrdineNonECitatoNelNomeDelFileCheScende(t *testing.T) {
	b := preparaBancoSuffissi(t)
	acme := b.clienteConRegole("ACME", regoleConOrdine)
	beta := b.clienteConRegole("BETA", `{}`)
	// per BETA lo stesso nome con l'iniziale minuscola: il banco fa dal nome la chiave del messaggio
	casi := []struct {
		nome    string
		cliente uuid.UUID
		atteso  string
	}{
		{"Ordine ODA_0001234.pdf", acme, "-/-"},
		{"Ordine ODA_0001234 per 7120010.pdf", acme, `-/- ["7120010"]`},
		{"ordine ODA_0001234.pdf", beta, `-/- ["ODA_0001234"]`},
		{"ordine ODA_0001234 per 7120010.pdf", beta, `-/- ["ODA_0001234", "7120010"]`},
	}
	for i, c := range casi {
		a, m := b.messaggioConAllegato(c.nome, estensione(c.nome))
		m = b.mettiNellaRfq(b.daControparte(m, c.cliente), b.rfqDelCliente(c.cliente))
		b.scendeInStaging(a, m, int64(300+i))
		if got := letturaDellaProposta(t, b, a.AllegatoID); got != c.atteso {
			t.Errorf("%s: proposta %q, attesa %q", c.nome, got, c.atteso)
		}
	}
}
