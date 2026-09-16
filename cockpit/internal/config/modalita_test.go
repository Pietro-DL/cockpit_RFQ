package config

import (
	"strings"
	"testing"
)

// L1 — la modalità del server (voce 9.5, §2.7) e il rifiuto di SH3 (b).
//
// Qui si prova la parte della shadow che vive nella configurazione: che cosa vale se non è scritto
// niente, che cosa succede se è scritto male, e il divieto di fare una prova in shadow sul NAS vero.

// Il silenzio del file vale shadow. Un cockpit.toml scritto prima che questa opzione esistesse non
// contiene la riga, e fra le due letture possibili di quel silenzio ce n'è una sola che si può
// correggere dopo: «non toccare niente».
func TestSenzaModalitaIlServerGiraInShadow(t *testing.T) {
	c, err := Carica(scrivi(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !c.EShadow() || c.Server.Modalita != ModalitaShadow {
		t.Fatalf("modalita = %q: senza la riga nel file deve valere shadow", c.Server.Modalita)
	}
}

// Una parola scritta male non deve poter significare «produzione» per distrazione: o è una delle due
// che esistono, o il server non parte.
func TestUnaModalitaScrittaMaleNonPassaPerProduzione(t *testing.T) {
	if _, err := Carica(scriviCon(t, "modalita = \"Produzzione\"\n", "", "")); err == nil {
		t.Fatal("una modalità scritta male è stata accettata: doveva fermare l'avvio")
	}
	c, err := Carica(scriviCon(t, "modalita = \"  PRODUZIONE \"\n", "", ""))
	if err != nil {
		t.Fatalf("maiuscole e spazi devono essere tollerati: %v", err)
	}
	if c.EShadow() {
		t.Fatalf("modalita = %q: attesa produzione", c.Server.Modalita)
	}
}

// SH3 (b) — in shadow il server non parte se la radice del NAS è (o sta sotto) una radice di
// produzione dichiarata, e `dry_run` viene forzato a true qualunque cosa dica il file.
//
// I casi con le maiuscole, la barra finale e le barre al contrario non sono pignoleria: sono i tre
// modi in cui lo stesso percorso viene scritto passando da un file all'altro, e un confronto fra
// stringhe che non li riconosce lascia passare proprio la copia-incolla da cui viene il pericolo.
func TestSH3ShadowRifiutaLaRadiceDiProduzione(t *testing.T) {
	produzione := `\\nas01\TECNICO - PREVENTIVI`
	casi := []struct {
		nome, radice string
		rifiuta      bool
	}{
		{"la radice stessa", `\\nas01\TECNICO - PREVENTIVI`, true},
		{"una sottocartella", `\\nas01\TECNICO - PREVENTIVI\PREVENTIVI DA FARE`, true},
		{"maiuscole e barra finale", `\\NAS01\tecnico - preventivi\`, true},
		{"barre al contrario", `//nas01/TECNICO - PREVENTIVI/PREVENTIVI DA FARE`, true},
		{"un'altra cartella che comincia uguale", `\\nas01\TECNICO - PREVENTIVI PROVA`, false},
		{"una cartella locale", `C:\prove\nas`, false},
	}
	for _, c := range casi {
		nas := "radice = '" + c.radice + "'\ndry_run = false\nradici_produzione = ['" + produzione + "']\n"
		cfg, err := Carica(scriviCon(t, "modalita = \"shadow\"\n", nas, ""))
		if c.rifiuta {
			if err == nil {
				t.Errorf("%s (%s): il server è partito in shadow sul NAS di produzione", c.nome, c.radice)
			} else if !strings.Contains(err.Error(), "radici_produzione") {
				t.Errorf("%s: l'errore non spiega da dove viene il divieto: %v", c.nome, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s (%s): rifiutata una radice che non è di produzione: %v", c.nome, c.radice, err)
			continue
		}
		if !cfg.NAS.DryRun {
			t.Errorf("%s: in shadow dry_run va forzato a true anche se il file dice false", c.nome)
		}
	}
}

// In produzione la dichiarazione non impedisce niente: è lì per la shadow.
func TestInProduzioneLaRadiceDichiarataNonBloccaLAvvio(t *testing.T) {
	radice := `\\nas01\PREVENTIVI`
	nas := "radice = '" + radice + "'\ndry_run = false\nradici_produzione = ['" + radice + "']\n"
	c, err := Carica(scriviCon(t, "modalita = \"produzione\"\n", nas, ""))
	if err != nil {
		t.Fatalf("in produzione la radice di produzione è quella giusta: %v", err)
	}
	if c.NAS.DryRun {
		t.Error("in produzione dry_run non va forzato: lo decide il file")
	}
	if c.RadiceDiProduzione() == "" {
		t.Error("la radice dichiarata deve restare riconoscibile anche in produzione")
	}
}
