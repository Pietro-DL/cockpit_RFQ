package fascicolo

// L1 — la AR tiene il componente (direttiva 35, blocco C-1c; INC-G26): cheCosaLoTiene conosce file_rfq_associazione
// (il rifiuto della cancellazione di RimuoviComponente) e fraseStoria la dice nel riepilogo dell'albero.

import (
	"testing"

	"promatec/cockpit/internal/platform/db"
)

func TestLaStoriaConosceLeAssociazioniRegistrate(t *testing.T) {
	if got := cheCosaLoTiene("file_rfq_associazione"); got != FraseAssociazioniRegistrate {
		t.Errorf("cheCosaLoTiene: %q", got)
	}
	if got := fraseStoria(db.ListComponentiConStoriaRow{Associazioni: true}); len(got) != 1 || got[0] != FraseAssociazioniRegistrate {
		t.Errorf("fraseStoria: %v", got)
	}
	if got := fraseStoria(db.ListComponentiConStoriaRow{}); len(got) != 0 {
		t.Errorf("senza storia: %v", got)
	}
}
