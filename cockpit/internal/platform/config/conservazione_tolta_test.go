package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L1 — V1, ondata C, corsia file (U2 §3.13 punto 5; [U] Q06 e C-02 B): la sezione [conservazione] del deposito locale
// non esiste piu'. Scritta nel file, anche vuota o con la sola radice commentata, ferma il caricamento con il messaggio
// che rimanda alla cartella ALTRO: nessun deposito si attiva da un .toml. Senza la sezione il server parte.

func scriviConfigurazione(t *testing.T, righe string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "cockpit.toml")
	corpo := "[db]\ndsn = \"postgres://x@localhost/y\"\n[nas]\nstaging = '" + filepath.Join(dir, "staging") + "'\n" + righe
	if err := os.WriteFile(p, []byte(corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSezioneConservazioneFermaLAvvio(t *testing.T) {
	for _, righe := range []string{
		"[conservazione]\n",
		"[conservazione]\nradice = ''\n",
		"[conservazione]\n# radice = 'C:\\x'\n",
		"[conservazione]\nradice = 'C:\\deposito'\n",
	} {
		_, err := Carica(scriviConfigurazione(t, righe))
		if !errors.Is(err, ErrSezioneConservazione) {
			t.Errorf("%q: il caricamento non si ferma con il messaggio della sezione tolta: %v", righe, err)
		}
	}
	if _, err := Carica(scriviConfigurazione(t, "")); err != nil {
		t.Fatalf("senza la sezione il server non parte: %v", err)
	}
	if want := "cartella ALTRO della RFQ sul NAS"; !strings.Contains(ErrSezioneConservazione.Error(), want) {
		t.Errorf("il messaggio non rimanda alla cartella ALTRO: %q", ErrSezioneConservazione.Error())
	}
}
