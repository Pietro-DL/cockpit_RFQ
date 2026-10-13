package config

import (
	"strings"
	"testing"
)

// L1 — [outlook].giorni_carica_precedenti e [outlook].inbox_max_messaggi (13/10/2026, richiesta
// dell'utente dal collaudo: la lista dell'Inbox si fermava a 200 righe e «Carica precedenti» saliva di
// due giorni per clic, senza modo di cambiarlo dalla configurazione).
//
// Assente = 0 = «non dichiarato»: il valore predefinito lo decide il server web (GiorniStoricoDefault,
// RigheInboxDefault), non un secondo numero scritto qui.
func TestSenzaLeRigheInboxEStoricoRestanoAiPredefiniti(t *testing.T) {
	c, err := Carica(scrivi(t, "[outlook]\ncartelle = [\"Inbox\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Outlook.GiorniCaricaPrecedenti != 0 || c.Outlook.InboxMaxMessaggi != 0 {
		t.Errorf("senza le righe: giorni_carica_precedenti = %d, inbox_max_messaggi = %d; attesi 0 e 0",
			c.Outlook.GiorniCaricaPrecedenti, c.Outlook.InboxMaxMessaggi)
	}
}

func TestInboxEStoricoSiScrivonoNelFile(t *testing.T) {
	c, err := Carica(scrivi(t, "[outlook]\ngiorni_carica_precedenti = 30\ninbox_max_messaggi = 1000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Outlook.GiorniCaricaPrecedenti != 30 || c.Outlook.InboxMaxMessaggi != 1000 {
		t.Errorf("giorni_carica_precedenti = %d, inbox_max_messaggi = %d; attesi 30 e 1000",
			c.Outlook.GiorniCaricaPrecedenti, c.Outlook.InboxMaxMessaggi)
	}
}

// Fuori dai limiti il server non parte, e l'errore nomina la chiave e il valore scritto.
func TestInboxEStoricoFuoriDaiLimitiFermanoLAvvio(t *testing.T) {
	for _, riga := range []string{
		"giorni_carica_precedenti = -1", "giorni_carica_precedenti = 366",
		"inbox_max_messaggi = -5", "inbox_max_messaggi = 5001",
	} {
		t.Run(riga, func(t *testing.T) {
			_, err := Carica(scrivi(t, "[outlook]\n"+riga+"\n"))
			if err == nil {
				t.Fatalf("il server è partito con %q", riga)
			}
			chiave, valore, _ := strings.Cut(riga, " = ")
			for _, atteso := range []string{chiave, valore} {
				if !strings.Contains(err.Error(), atteso) {
					t.Errorf("l'errore non dice %q: %v", atteso, err)
				}
			}
		})
	}
}
