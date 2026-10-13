package web

import (
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// L1 — i due limiti dell'Inbox letti dal Server (13/10/2026): zero = valore predefinito, altrimenti
// quello del cockpit.toml; e la lista «piena» solo quando è arrivata al limite.
func TestLimitiInboxPredefinitiEDichiarati(t *testing.T) {
	var s Server
	if s.giorniStorico() != GiorniStoricoDefault || s.righeInbox() != RigheInboxDefault {
		t.Errorf("senza configurazione: %d giorni, %d righe; attesi %d e %d", s.giorniStorico(), s.righeInbox(), GiorniStoricoDefault, RigheInboxDefault)
	}
	s.GiorniStorico, s.InboxMaxMessaggi = 30, 1000
	if s.giorniStorico() != 30 || s.righeInbox() != 1000 {
		t.Errorf("con la configurazione: %d giorni, %d righe; attesi 30 e 1000", s.giorniStorico(), s.righeInbox())
	}
}

func TestLaListaDellInboxEPienaSoloAlLimite(t *testing.T) {
	d := inboxDati{Limite: 3, Righe: make([]db.VInbox, 2)}
	if d.Piena() {
		t.Error("2 righe su un limite di 3: la pagina direbbe che ce ne sono altre")
	}
	d.Righe = make([]db.VInbox, 3)
	if !d.Piena() {
		t.Error("3 righe su un limite di 3: la pagina non avvisa che la lista si è fermata")
	}
}
