package config

// L1 — cockpit.toml.example parte cosi' com'e', cambiando solo le password (il DSN qui non si usa): nessuna
// voce sconosciuta, nessuna sigla di [[casella]] o [[postazione]] che [[utenti]] non dichiara, niente
// indirizzi che non siano d'esempio.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLEsempioParteCambiandoSoloDsnEPassword(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "cockpit.toml.example"))
	if err != nil {
		t.Fatal(err)
	}
	testo := strings.ReplaceAll(string(b), "INSERISCI_PASSWORD_INIZIALE", "una-password-vera")
	// lo staging dell'esempio sta in C:\Cockpit: la prova non crea cartelle fuori dalla sua
	staging := regexp.MustCompile(`(?m)^staging = '[^']*'`)
	if !staging.MatchString(testo) {
		t.Fatal("l'esempio non ha piu' la riga [nas].staging")
	}
	d := t.TempDir()
	testo = staging.ReplaceAllLiteralString(testo, "staging = '"+filepath.Join(d, "staging")+"'")
	p := filepath.Join(d, "cockpit.toml")
	if err := os.WriteFile(p, []byte(testo), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Carica(p)
	if err != nil {
		t.Fatalf("l'esempio non parte: %v", err)
	}
	if len(c.Avvisi) > 0 {
		t.Errorf("l'esempio ha voci che il server segnala:\n%s", strings.Join(c.Avvisi, "\n"))
	}
	sigle := map[string]bool{}
	for _, u := range c.Utenti {
		sigle[u.Sigla] = true
	}
	for _, k := range c.Caselle {
		if k.Utente != "" && !sigle[k.Utente] {
			t.Errorf("la casella %s ha utente %q, che [[utenti]] non dichiara", k.Indirizzo, k.Utente)
		}
		if !strings.HasSuffix(k.Indirizzo, ".example") {
			t.Errorf("indirizzo non d'esempio: %s", k.Indirizzo)
		}
	}
	for _, x := range c.Postazioni {
		if x.Utente != "" && !sigle[x.Utente] {
			t.Errorf("la postazione %s ha utente %q, che [[utenti]] non dichiara", x.NomeHost, x.Utente)
		}
	}
	if c.RadiceDiProduzione() != "" {
		t.Errorf("la radice dell'esempio e' sotto una radice di produzione: %s", c.NAS.Radice)
	}
}
