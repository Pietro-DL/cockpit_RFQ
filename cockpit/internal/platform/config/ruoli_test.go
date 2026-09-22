package config

import (
	"strings"
	"testing"
)

// CF1 — un ruolo che non esiste ferma l'avvio.
//
// Prima diventava `operatore` in silenzio, ed è il modo peggiore di sbagliare: chi scrive
// «amministratore» ottiene un utente che crede di amministrare il Cockpit, non può aprire nessuna
// schermata tecnica, e non trova da nessuna parte la riga che gliene dà il motivo. Il messaggio deve
// dire tre cose: chi, che cosa ha scritto, e che cosa si poteva scrivere.
func TestCF1UnRuoloCheNonEsisteFermaLAvvio(t *testing.T) {
	casi := []struct {
		nome, riga string
	}{
		{"parola inventata", "ruolo = \"amministratore\"\n"},
		{"ruolo mancante", ""},
		{"ruolo vuoto", "ruolo = \"\"\n"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Carica(scrivi(t, "[[utenti]]\nsigla = \"FP\"\nnome = \"Nome Cognome\"\nufficio = \"Commerciale\"\n"+c.riga))
			if err == nil {
				t.Fatal("il server è partito con un ruolo che non sa leggere")
			}
			for _, atteso := range []string{"FP", "ruolo", "admin", "operatore", "tecnico", "consultazione"} {
				if !strings.Contains(err.Error(), atteso) {
					t.Errorf("l'errore non dice %q: %v", atteso, err)
				}
			}
		})
	}
}

// I quattro ruoli dell'enum si possono scrivere nel file, e si normalizzano: «Admin » con la
// maiuscola e uno spazio è lo stesso ruolo, non un errore da respingere in faccia a chi configura.
// `ufficio` invece resta quello che è scritto: è l'organigramma, non un permesso.
func TestIQuattroRuoliSiPossonoConfigurare(t *testing.T) {
	c, err := Carica(scrivi(t, `
[[utenti]]
sigla = "fp"
nome = "Nome Cognome"
ufficio = "Commerciale"
ruolo = " Operatore "
[[utenti]]
sigla = "PS"
nome = "Amministratore"
ufficio = "IT"
ruolo = "admin"
[[utenti]]
sigla = "TE"
nome = "Tecnico"
ufficio = "Ufficio tecnico"
ruolo = "tecnico"
[[utenti]]
sigla = "DI"
nome = "Direzione"
ufficio = "Direzione"
ruolo = "consultazione"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Utenti) != 4 {
		t.Fatalf("utenti letti: %d", len(c.Utenti))
	}
	if c.Utenti[0].Sigla != "FP" || c.Utenti[0].Ruolo != "operatore" {
		t.Errorf("sigla e ruolo non normalizzati: %q %q", c.Utenti[0].Sigla, c.Utenti[0].Ruolo)
	}
	if c.Utenti[0].Ufficio != "Commerciale" {
		t.Errorf("ufficio cambiato: %q", c.Utenti[0].Ufficio)
	}
}

// Due volte la stessa sigla è un file scritto male, e in database sarebbe un upsert che vince
// sull'altro senza dirlo: l'ultimo scritto deciderebbe il ruolo del primo.
func TestUnaSiglaDueVolteNonPassa(t *testing.T) {
	_, err := Carica(scrivi(t, `
[[utenti]]
sigla = "FP"
nome = "Uno"
ruolo = "operatore"
[[utenti]]
sigla = "fp"
nome = "Due"
ruolo = "admin"
`))
	if err == nil || !strings.Contains(err.Error(), "due volte") {
		t.Fatalf("due utenti con la stessa sigla: %v", err)
	}
}

// Almeno un `admin` fra gli [[utenti]]. La regola è dichiarata nel README dalla voce 6.9, e una
// regola dichiarata e non imposta è peggio di una che non c'è: un file con soli operatori è un
// Cockpit in cui *Postazioni* non la apre più nessuno, cioè in cui non si può più aggiungere un PC —
// e lo si scopre dal primo 403, mentre si sta facendo altro.
func TestSenzaNessunAdminIlServerNonParte(t *testing.T) {
	_, err := Carica(scrivi(t, `
[[utenti]]
sigla = "FP"
nome = "Nome Cognome"
ruolo = "operatore"
[[utenti]]
sigla = "LU"
nome = "Nome Cognome"
ruolo = "tecnico"
`))
	if err == nil {
		t.Fatal("il server è partito senza nessun amministratore")
	}
	for _, atteso := range []string{"admin", "postazioni"} {
		if !strings.Contains(err.Error(), atteso) {
			t.Errorf("l'errore non dice %q: %v", atteso, err)
		}
	}
}

// Un solo admin basta, e gli altri restano quello che sono.
func TestUnAdminBasta(t *testing.T) {
	c, err := Carica(scrivi(t, `
[[utenti]]
sigla = "FP"
nome = "Nome Cognome"
ruolo = "operatore"
[[utenti]]
sigla = "PS"
nome = "Nome Cognome"
ruolo = "admin"
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Utenti[0].Ruolo != "operatore" || c.Utenti[1].Ruolo != "admin" {
		t.Errorf("i ruoli non sono quelli scritti nel file: %v", c.Utenti)
	}
}

// Nessun utente configurato NON è un errore: è un file a cui non sono ancora stati aggiunti. Lo dice
// l'impossibilità di entrare, non un avvio che si rifiuta — e i test che non parlano di utenti non
// devono doverne dichiarare uno per far partire la configurazione.
func TestSenzaUtentiLaConfigurazioneSiCaricaLoStesso(t *testing.T) {
	if _, err := Carica(scrivi(t, "")); err != nil {
		t.Fatalf("una configurazione senza [[utenti]] non si carica più: %v", err)
	}
}
