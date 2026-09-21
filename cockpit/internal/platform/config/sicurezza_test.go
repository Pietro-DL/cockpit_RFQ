package config

import (
	"strings"
	"testing"
)

// L1 — blocco 4: [sicurezza] risolta in ciò che il server può davvero fare.
//
// Tre regole, e ognuna esiste per un motivo che si è già visto sbagliare altrove:
//
//  1. shadow è un PRESET e vince su tutto. Chi scrive «shadow» sta dicendo «questo server non tocca
//     niente», e quella frase non deve poter essere contraddetta tre righe più sotto;
//  2. il silenzio vale «non scrivere». Un file che dice `modalita = "produzione"` e non conosce
//     [sicurezza] l'ha scritto qualcuno prima che queste voci esistessero: accendergli tutto sarebbe
//     esattamente il «comportamento pericoloso attivato in silenzio» che il mandato vieta;
//  3. `[nas].dry_run` continua a significare quello che ha sempre significato — non scrivere sul NAS —
//     ma è deprecata, e il server lo dice.

func capacitaDa(t *testing.T, server, nas, corpo string) Capacita {
	t.Helper()
	c, err := Carica(scriviCon(t, server, nas, corpo))
	if err != nil {
		t.Fatalf("configurazione rifiutata: %v", err)
	}
	return c.Capacita()
}

const tutteAccese = "\n[sicurezza]\noutlook_scrittura = true\nbozze = true\nnas_scrittura = true\n"

// Senza [sicurezza] non si accende niente, nemmeno in produzione, e qualcuno deve dirlo.
func TestSenzaSicurezzaNonSiScriveNiente(t *testing.T) {
	for _, modalita := range []string{"shadow", "produzione"} {
		cap := capacitaDa(t, "modalita = \""+modalita+"\"\n", "", "")
		if !(!cap.OutlookScrittura && !cap.Bozze && !cap.NasScrittura) {
			t.Errorf("modalita = %q senza [sicurezza]: qualcosa si è acceso da solo (%+v)", modalita, cap)
		}
		if len(cap.Avvisi) == 0 {
			t.Errorf("modalita = %q: nessun avviso spiega perché il server non scrive niente", modalita)
		}
	}
}

// La shadow è un preset: spegne tutto anche se il file chiede il contrario, e lo dice.
func TestShadowSpegneTuttoAncheSeIlFileChiedeAltro(t *testing.T) {
	cap := capacitaDa(t, "modalita = \"shadow\"\n", "", tutteAccese)
	if cap.OutlookScrittura || cap.Bozze || cap.NasScrittura {
		t.Fatalf("in shadow una capacità è rimasta accesa: %+v", cap)
	}
	insieme := strings.Join(cap.Avvisi, " | ")
	if !strings.Contains(insieme, "shadow") {
		t.Errorf("gli avvisi non dicono che è la shadow a spegnere: %q", insieme)
	}
	if !strings.Contains(insieme, "[sicurezza]") {
		t.Errorf("il file chiedeva capacità che la shadow spegne e nessuno lo segnala: %q", insieme)
	}
}

// In produzione si accende SOLO ciò che è dichiarato, una voce per volta. È il punto del blocco: il
// NAS di prova si prova senza toccare Outlook.
func TestInProduzioneSiAccendeSoloCioCheEDichiarato(t *testing.T) {
	casi := []struct {
		nome, sezione                string
		outlook, bozze, nasScrittura bool
	}{
		{"solo il NAS", "\n[sicurezza]\nnas_scrittura = true\n", false, false, true},
		{"solo Outlook", "\n[sicurezza]\noutlook_scrittura = true\n", true, false, false},
		{"solo le bozze", "\n[sicurezza]\nbozze = true\n", false, true, false},
		{"tutte e tre", tutteAccese, true, true, true},
		{"dichiarate e spente", "\n[sicurezza]\noutlook_scrittura = false\nbozze = false\nnas_scrittura = false\n", false, false, false},
	}
	for _, c := range casi {
		cap := capacitaDa(t, "modalita = \"produzione\"\n", "", c.sezione)
		if cap.OutlookScrittura != c.outlook || cap.Bozze != c.bozze || cap.NasScrittura != c.nasScrittura {
			t.Errorf("%s: %+v, attese outlook=%v bozze=%v nas=%v", c.nome, cap, c.outlook, c.bozze, c.nasScrittura)
		}
	}
}

// `[nas].dry_run` resta leggibile ma è deprecata: spegne il NAS e lo dice, in tutti e due i casi.
func TestDryRunSpegneIlNasEDiceCheEDeprecata(t *testing.T) {
	cap := capacitaDa(t, "modalita = \"produzione\"\n", "dry_run = true\n", tutteAccese)
	if cap.NasScrittura {
		t.Error("dry_run = true non ha spento la scrittura sul NAS: quella voce ha sempre significato questo")
	}
	if !cap.OutlookScrittura || !cap.Bozze {
		t.Error("dry_run ha spento anche le capacità di Outlook: riguarda il NAS e basta")
	}
	insieme := strings.Join(cap.Avvisi, " | ")
	if !strings.Contains(insieme, "DEPRECATA") {
		t.Errorf("nessun avviso dice che dry_run è deprecata: %q", insieme)
	}

	// e senza dry_run non compare nessun avviso su di lei: un file pulito non deve leggere avvisi
	pulito := capacitaDa(t, "modalita = \"produzione\"\n", "", tutteAccese)
	if strings.Contains(strings.Join(pulito.Avvisi, " | "), "dry_run") {
		t.Errorf("un file senza dry_run riceve un avviso su dry_run: %q", pulito.Avvisi)
	}
	if !pulito.NasScrittura || !pulito.OutlookScrittura || !pulito.Bozze {
		t.Errorf("con tutte e tre dichiarate a true non si sono accese: %+v", pulito)
	}
}
