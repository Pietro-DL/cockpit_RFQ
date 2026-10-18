package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L1 — V1, PM23 AF-06 (EM §10.5, «Il deposito di conservazione»; giro 1, SIC-16): [conservazione].radice.
//   - senza la sezione (o con la radice vuota) il server parte, e la conservazione non e' configurata (S3 rispondera'
//     503 NON_CONFIGURATO: lo prova la L4 delle rotte);
//   - la radice non coincide con lo staging, con la radice del NAS o con una delle radici di produzione, e non sta ne'
//     sotto ne' sopra una di loro: errore di configurazione, il server non parte, e l'errore dice quale voce;
//   - i percorsi si confrontano risolti (assoluti, senza maiuscole, barre uniformi, senza la barra finale);
//   - un percorso UNC o una lettera di rete si rifiutano: il deposito sta su un disco locale.

// scriviConservazione: un cockpit.toml con lo staging e la radice del NAS sotto la cartella temporanea, piu' le righe date.
func scriviConservazione(t *testing.T, dir, righe string) string {
	t.Helper()
	p := filepath.Join(dir, "cockpit.toml")
	corpo := "[db]\ndsn = \"postgres://x@localhost/y\"\n[nas]\nstaging = '" + filepath.Join(dir, "staging") + "'\n" +
		"radice = '" + filepath.Join(dir, "nas", "PREVENTIVI DA FARE") + "'\n" +
		"radici_produzione = ['" + filepath.Join(dir, "produzione") + "']\n" + righe
	if err := os.WriteFile(p, []byte(corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConservazioneAssenteIlServerParte(t *testing.T) {
	for _, righe := range []string{"", "[conservazione]\n", "[conservazione]\nradice = ''\n"} {
		c, err := Carica(scriviConservazione(t, t.TempDir(), righe))
		if err != nil {
			t.Fatalf("%q: il server non parte senza il deposito: %v", righe, err)
		}
		if c.Conservazione.Configurata() {
			t.Errorf("%q: la conservazione risulta configurata senza una radice", righe)
		}
	}
}

func TestConservazioneSovrapposizioniVietate(t *testing.T) {
	dir := t.TempDir()
	casi := []struct {
		nome, radice, voce string
	}{
		{"uguale allo staging", filepath.Join(dir, "staging"), "[nas].staging"},
		{"dentro lo staging (la cache la svuoterebbe)", filepath.Join(dir, "staging", "_contenuti", "deposito"), "[nas].staging"},
		{"sopra lo staging", dir, "[nas].staging"},
		{"uguale alla radice del NAS", filepath.Join(dir, "nas", "PREVENTIVI DA FARE"), "[nas].radice"},
		{"sotto la radice del NAS", filepath.Join(dir, "nas", "PREVENTIVI DA FARE", "ALTRO"), "[nas].radice"},
		{"sotto una radice di produzione", filepath.Join(dir, "produzione", "deposito"), "[nas].radici_produzione"},
		{"lo staging con le maiuscole diverse e la barra finale", strings.ToUpper(filepath.Join(dir, "staging")) + `\`, "[nas].staging"},
	}
	for _, c := range casi {
		_, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = '"+c.radice+"'\n"))
		if err == nil {
			t.Errorf("%s: il server parte con il deposito sovrapposto", c.nome)
			continue
		}
		if !strings.Contains(err.Error(), "[conservazione].radice") || !strings.Contains(err.Error(), c.voce) {
			t.Errorf("%s: l'errore non dice la voce che si sovrappone (%s): %v", c.nome, c.voce, err)
		}
	}
	// accanto allo staging, fuori dal NAS e dalle radici di produzione: va
	c, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = '"+filepath.Join(dir, "conservazione")+"'\n"))
	if err != nil {
		t.Fatalf("un deposito accanto allo staging e' rifiutato: %v", err)
	}
	if !c.Conservazione.Configurata() || c.Conservazione.Radice != filepath.Join(dir, "conservazione") {
		t.Errorf("radice letta: %q", c.Conservazione.Radice)
	}
}

func TestConservazioneRelativaAllaCartellaDelFile(t *testing.T) {
	dir := t.TempDir()
	c, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = 'conservazione'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Conservazione.Radice != filepath.Join(dir, "conservazione") {
		t.Errorf("la radice relativa non si legge dalla cartella del file: %q", c.Conservazione.Radice)
	}
}

func TestConservazioneSoloSuUnDiscoLocale(t *testing.T) {
	dir := t.TempDir()
	for _, r := range []string{`\\server-nas\deposito`, `//server-nas/deposito`} {
		_, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = '"+r+"'\n"))
		if err == nil || !strings.Contains(err.Error(), "disco locale") {
			t.Errorf("%s: un percorso UNC deve fermare l'avvio: %v", r, err)
		}
	}
	// una lettera mappata sulla rete: la si simula (una lettera di rete vera non c'e' su ogni macchina)
	prima := unitaDiReteFn
	t.Cleanup(func() { unitaDiReteFn = prima })
	unitaDiReteFn = func(p string) bool { return strings.HasPrefix(strings.ToUpper(p), "Z:") }
	_, err := Carica(scriviConservazione(t, dir, "[conservazione]\nradice = 'Z:\\deposito'\n"))
	if err == nil || !strings.Contains(err.Error(), "disco locale") {
		t.Errorf("una lettera di rete deve fermare l'avvio: %v", err)
	}
}

// TestSovrappostiPuro: la funzione del confronto, sui casi di confine.
func TestSovrappostiPuro(t *testing.T) {
	casi := []struct {
		a, b string
		si   bool
	}{
		{`c:\a\b`, `c:\a\b`, true},
		{`c:\a\b\c`, `c:\a\b`, true},
		{`c:\a`, `c:\a\b`, true},
		{`c:\a\bc`, `c:\a\b`, false},
		{`c:\a\b`, `c:\a\bc`, false},
		{`c:\a`, `d:\a`, false},
		{``, `c:\a`, false},
	}
	for _, c := range casi {
		if got := sovrapposti(confrontabile(c.a), confrontabile(c.b)); got != c.si {
			t.Errorf("sovrapposti(%q, %q) = %v, atteso %v", c.a, c.b, got, c.si)
		}
	}
}
