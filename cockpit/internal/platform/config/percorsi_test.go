package config

// L1 — i percorsi relativi del file si leggono dalla cartella del file (come gia' tls_cert e tls_key), non
// dalla cartella da cui e' partito l'eseguibile; e le voci che non esistono, o che non hanno effetto, si
// dicono invece di passare in silenzio.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Un'attivita' pianificata o un servizio partono da C:\Windows\System32: lo staging relativo finiva li', e
// la radice del NAS non si trovava. Adesso contano la cartella del file, qualunque sia quella corrente.
func TestIPercorsiDelNasEDelLogSiLeggonoDallaCartellaDelFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "cockpit.toml")
	corpo := "[db]\ndsn = \"postgres://x@localhost/y\"\n[server]\n" + `log_file = 'log\cockpit.log'` + "\n" +
		"[nas]\n" + `radice = 'nas_prova\PREVENTIVI DA FARE'` + "\n" + `staging = 'lavoro\staging'` + "\n"
	if err := os.WriteFile(p, []byte(corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	altrove := t.TempDir()
	t.Chdir(altrove) // come un servizio che parte da un'altra cartella

	c, err := Carica(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct{ voce, got, atteso string }{
		{"[nas].radice", c.NAS.Radice, filepath.Join(d, "nas_prova", "PREVENTIVI DA FARE")},
		{"[nas].staging", c.NAS.Staging, filepath.Join(d, "lavoro", "staging")},
		{"[server].log_file", c.PercorsoLog(), filepath.Join(d, "log", "cockpit.log")},
	} {
		if x.got != x.atteso {
			t.Errorf("%s = %q, atteso %q", x.voce, x.got, x.atteso)
		}
	}
	if _, err := os.Stat(filepath.Join(d, "lavoro", "staging")); err != nil {
		t.Errorf("lo staging non e' stato creato accanto al file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(altrove, "lavoro")); !os.IsNotExist(err) {
		t.Errorf("lo staging e' stato creato nella cartella corrente")
	}
	// e con -config relativo la base e' la cartella del file, resa assoluta
	t.Chdir(d)
	c, err = Carica("cockpit.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(c.NAS.Staging) || c.NAS.Staging != filepath.Join(d, "lavoro", "staging") {
		t.Errorf("con -config relativo lo staging e' %q", c.NAS.Staging)
	}
}

// Gli assoluti, gli UNC (anche con le barre al contrario), i percorsi «dalla radice» e quelli con la
// lettera del disco restano come sono; "-" per il log resta «solo a schermo».
func TestIPercorsiAssolutiEUncNonCambiano(t *testing.T) {
	dir := t.TempDir()
	assolutoQui := filepath.Join(dir, "altrove", "staging")
	for _, p := range []string{assolutoQui, `\\server-nas\preventivi\PREVENTIVI DA FARE`, `//server-nas/preventivi`, `/mnt/nas/preventivi`,
		`C:\Cockpit\staging`, `\Cockpit\staging`} {
		if got := assoluto(dir, p); got != p {
			t.Errorf("assoluto(%q) = %q: doveva restare com'era", p, got)
		}
	}
	if got := assoluto(dir, ""); got != "" {
		t.Errorf("un percorso vuoto e' diventato %q", got)
	}
	unc := `\\server-nas\preventivi\PREVENTIVI DA FARE`
	c, err := Carica(scriviCon(t, "log_file = '-'\n", "radice = '"+unc+"'\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.NAS.Radice != unc {
		t.Errorf("la radice UNC e' cambiata: %q", c.NAS.Radice)
	}
	if c.PercorsoLog() != "" {
		t.Errorf("log_file = \"-\" vuol dire solo a schermo, non %q", c.PercorsoLog())
	}
}

// Una voce che non esiste (un refuso) si dice nel log e non ferma l'avvio: «reti_consentit» valeva come
// nessun filtro, in silenzio. Lo stesso per le voci lette ma senza effetto.
func TestLeVociSconosciuteESenzaEffettoSiDiconoSenzaFermareLAvvio(t *testing.T) {
	c, err := Carica(scriviCon(t, "reti_consentit = [\"10.0.0.0/24\"]\nsegreto_sessione = \"abc\"\n", "",
		"[outlook]\nconsenti_invio = false\n\n[[utenti]]\nsigla = \"AM\"\nruolo = \"admin\"\npasswrd = \"x\"\n"))
	if err != nil {
		t.Fatalf("un file con voci sconosciute non parte: %v", err)
	}
	tutti := strings.Join(c.Avvisi, "\n")
	for _, atteso := range []string{`"server.reti_consentit"`, `"utenti.passwrd"`, "segreto_sessione", "consenti_invio"} {
		if !strings.Contains(tutti, atteso) {
			t.Errorf("gli avvisi non dicono %s:\n%s", atteso, tutti)
		}
	}
	// un file pulito non ha avvisi di questo tipo
	pulito, err := Carica(scriviCon(t, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range pulito.Avvisi {
		if !strings.Contains(a, "token_worker") {
			t.Errorf("avviso su un file pulito: %s", a)
		}
	}
}

// max_upload_mb = 0 non e' «il default»: e' un errore d'avvio, e il commento ora lo dice.
func TestMaxUploadAZeroEUnErrore(t *testing.T) {
	if _, err := Carica(scriviCon(t, "max_upload_mb = 0\n", "", "")); err == nil || !strings.Contains(err.Error(), "max_upload_mb") {
		t.Errorf("max_upload_mb = 0: %v, atteso un errore", err)
	}
}
