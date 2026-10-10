package config

// L1 — giro 5, A1d: la sezione [motore_a] (piano 7.4.1, 7.6.1; BE-16): spenta per difetto, ferma l'avvio solo accesa
// senza indice, percorsi dalla cartella del file. Nessun file aperto: i percorsi sono inventati.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Senza la sezione il Cockpit si comporta come prima: anteprima spenta, nessun indice, nessun avviso che la nomini.
// Cade con l'anteprima accesa fra i predefiniti di CaricaConRete (M-A1d-27).
func TestMotoreASenzaSezioneESpenta(t *testing.T) {
	c, err := Carica(scrivi(t, ""))
	if err != nil {
		t.Fatalf("un file senza [motore_a] non parte: %v", err)
	}
	if c.MotoreA != (MotoreA{}) {
		t.Errorf("senza [motore_a] la sezione è %+v: attesa spenta e senza indice", c.MotoreA)
	}
	for _, a := range c.Avvisi {
		if strings.Contains(a, "motore_a") {
			t.Errorf("avviso su una sezione che il file non ha: %s", a)
		}
	}
}

// Accesa senza indice l'avvio si ferma (normalizzaMotoreA, l'unico controllo della sezione), anche con una riga di
// soli spazi, che assoluto svuota. Cade se normalizzaMotoreA non è chiamata (M-A1d-15).
func TestMotoreAAccesaSenzaRegoleFermaLAvvio(t *testing.T) {
	for _, x := range []struct{ nome, sezione string }{
		{"senza regole", "[motore_a]\nanteprima = true\n"},
		{"regole = '   '", "[motore_a]\nanteprima = true\nregole = '   '\n"},
	} {
		t.Run(x.nome, func(t *testing.T) {
			_, err := Carica(scrivi(t, x.sezione))
			if err == nil || !strings.Contains(err.Error(), "[motore_a]") {
				t.Errorf("anteprima accesa, %s: errore %v, atteso un errore d'avvio che nomini [motore_a]", x.nome, err)
			}
		})
	}
}

// regole relativa si legge dalla cartella del file, come [nas].radice e [nas].staging; assoluta, con la lettera del
// disco e UNC resta com'è (assoluto). Nel file relativo i separatori sono «/»: la prova vale su Linux e su Windows.
// Cade se assoluto non si applica a Regole (M-A1d-25).
func TestMotoreALeRegoleSiLeggonoDallaCartellaDelFile(t *testing.T) {
	p := scrivi(t, "[motore_a]\nanteprima = true\nregole = 'motore_a/indice_motore_a.json'\n")
	c, err := Carica(p)
	if err != nil {
		t.Fatal(err)
	}
	if !c.MotoreA.Anteprima {
		t.Errorf("anteprima = true letta come spenta")
	}
	if atteso := filepath.Join(filepath.Dir(p), "motore_a", "indice_motore_a.json"); c.MotoreA.Regole != atteso {
		t.Errorf("regole relativa = %q, atteso %q: si legge dalla cartella del file", c.MotoreA.Regole, atteso)
	}
	for _, invariato := range []string{
		filepath.ToSlash(filepath.Join(t.TempDir(), "regole", "indice_motore_a.json")),
		`C:\Cockpit\motore_a\indice_motore_a.json`,
		`\\nas.acme.example\regole\indice_motore_a.json`,
	} {
		c, err := Carica(scrivi(t, "[motore_a]\nanteprima = true\nregole = '"+invariato+"'\n"))
		if err != nil {
			t.Fatal(err)
		}
		if c.MotoreA.Regole != invariato {
			t.Errorf("regole = %q è diventata %q: doveva restare com'era", invariato, c.MotoreA.Regole)
		}
	}
}

// Spenta, la sezione non guarda il file: un indice che non c'è non dà errori né avvisi, e il percorso si risolve lo
// stesso dalla cartella del file (il contratto «spenta = nessun file», piano 7.4.1).
func TestMotoreASpentaNonGuardaIlFile(t *testing.T) {
	p := scrivi(t, "[motore_a]\nanteprima = false\nregole = 'non_c_e/indice_motore_a.json'\n")
	c, err := Carica(p)
	if err != nil {
		t.Fatalf("spenta con un indice che non c'è: %v, atteso nessun errore", err)
	}
	if c.MotoreA.Anteprima {
		t.Errorf("anteprima = false letta come accesa")
	}
	cartella := filepath.Join(filepath.Dir(p), "non_c_e")
	if atteso := filepath.Join(cartella, "indice_motore_a.json"); c.MotoreA.Regole != atteso {
		t.Errorf("regole = %q, atteso %q", c.MotoreA.Regole, atteso)
	}
	if _, err := os.Stat(cartella); !os.IsNotExist(err) {
		t.Errorf("la cartella dell'indice c'è (%v): il caricamento non crea niente per [motore_a]", err)
	}
	for _, a := range c.Avvisi {
		if strings.Contains(a, "motore_a") {
			t.Errorf("avviso sulla sezione spenta: %s", a)
		}
	}
}

// Le due voci sono conosciute: scritte giuste non danno avvisi. Un refuso nella sezione è un avviso con il suo nome
// completo, non ferma l'avvio e non accende niente. Cade con un tag toml sbagliato (M-A1d-26).
func TestMotoreALeVociDellaSezioneSonoConosciute(t *testing.T) {
	c, err := Carica(scrivi(t, "[motore_a]\nanteprima = true\nregole = 'motore_a/indice_motore_a.json'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Avvisi) > 0 {
		t.Errorf("[motore_a] scritta giusta dà avvisi:\n%s", strings.Join(c.Avvisi, "\n"))
	}
	c, err = Carica(scrivi(t, "[motore_a]\nantiprima = true\n"))
	if err != nil {
		t.Fatalf("un refuso in [motore_a] ferma l'avvio: %v", err)
	}
	if tutti := strings.Join(c.Avvisi, "\n"); !strings.Contains(tutti, "motore_a.antiprima") {
		t.Errorf("il refuso non è fra gli avvisi:\n%s", tutti)
	}
	if c.MotoreA.Anteprima {
		t.Errorf("il refuso ha acceso l'anteprima")
	}
}
