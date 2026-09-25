package main

// L1 — quale cockpit.toml si legge: con -config quello; senza, quello della cartella corrente se c'e',
// altrimenti quello accanto all'eseguibile (un'attivita' pianificata parte da un'altra cartella).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSenzaConfigSiLeggeIlFileAccantoAllEseguibile(t *testing.T) {
	exeDir := t.TempDir()
	exe := filepath.Join(exeDir, "cockpit.exe")
	accanto := filepath.Join(exeDir, "cockpit.toml")
	eseguibile := func() (string, error) { return exe, nil }
	nessuno := func(string) bool { return false }
	soloAccanto := func(p string) bool { return p == accanto }
	tutti := func(string) bool { return true }

	// esplicito: quello, anche se non esiste (l'errore lo dira' la lettura)
	if p, _ := percorsoConfig(true, `D:\altrove\cockpit.toml`, nessuno, eseguibile); p != `D:\altrove\cockpit.toml` {
		t.Errorf("con -config: %q", p)
	}
	// la cartella corrente ha il suo: vince (lo sviluppo, go run dalla cartella cockpit)
	if p, _ := percorsoConfig(false, "cockpit.toml", tutti, eseguibile); p != "cockpit.toml" {
		t.Errorf("con il file nella cartella corrente: %q", p)
	}
	// la cartella corrente non ce l'ha: quello accanto all'eseguibile
	if p, _ := percorsoConfig(false, "cockpit.toml", soloAccanto, eseguibile); p != accanto {
		t.Errorf("senza il file nella cartella corrente: %q, atteso %q", p, accanto)
	}
	// nessuno dei due: si dice dove si e' cercato
	p, cercati := percorsoConfig(false, "cockpit.toml", nessuno, eseguibile)
	if p != accanto || len(cercati) != 2 || !filepath.IsAbs(cercati[0]) || cercati[1] != accanto {
		t.Errorf("senza nessun file: %q, cercati %v", p, cercati)
	}
	// l'eseguibile non si trova: resta il file della cartella corrente
	if p, _ := percorsoConfig(false, "cockpit.toml", nessuno, func() (string, error) { return "", errors.New("no") }); p != "cockpit.toml" {
		t.Errorf("senza eseguibile: %q", p)
	}
}

func TestEsisteVuoleUnFile(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "cockpit.toml")
	if esiste(f) {
		t.Error("un file che non c'e' esiste")
	}
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !esiste(f) || esiste(d) {
		t.Errorf("esiste(file) = %v, esiste(cartella) = %v", esiste(f), esiste(d))
	}
}
