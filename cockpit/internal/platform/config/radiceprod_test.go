package config

import (
	"strings"
	"testing"
)

// L1 — blocco 4: scrivere sul NAS VERO non deve costare un interruttore solo.
//
// In shadow la radice di produzione è VIETATA (SH3): una prova che scrive sul NAS vero non è una
// prova. In produzione non può essere vietata — è il posto dove il Cockpit lavorerà davvero — ma
// cominciare a scriverci deve costare due dichiarazioni, non una.
//
// Il motivo è il giorno in cui `[nas].radice` passerà da `_nas_test` al percorso aziendale: copiando
// un file da un PC all'altro, tornando da una prova, cambiando una riga per sbaglio. In quel momento
// `nas_scrittura` è già `true` da settimane e nessuno la sta guardando: la radice e la capacità sono
// due voci lontane, e ognuna delle due, presa da sola, sembra innocua. Questa terza le lega.
//
// La capacità spenta non fa scattare niente: un server che non scrive non può sbagliare cartella.

const produzioneVera = `\\server-nas\PREVENTIVI`

func nasCon(radice string) string {
	return "radice = '" + radice + "'\nradici_produzione = ['" + produzioneVera + "']\n"
}

const soloNas = "\n[sicurezza]\noutlook_scrittura = false\nbozze = false\nnas_scrittura = true\n"

func TestScrivereSulNasVeroChiedeUnaSecondaDichiarazione(t *testing.T) {
	casi := []struct {
		nome, radice, sicurezza string
		rifiuta                 bool
	}{
		{"la radice di produzione, con la scrittura accesa", produzioneVera + `\PREVENTIVI DA FARE`, soloNas, true},
		{"la radice stessa", produzioneVera, soloNas, true},
		{"scritta con le maiuscole diverse e la barra finale", `\\SERVER-NAS\preventivi\`, soloNas, true},
		{"la scrittura sul NAS è spenta: non c'è niente da proteggere", produzioneVera + `\PREVENTIVI DA FARE`,
			"\n[sicurezza]\noutlook_scrittura = true\nbozze = true\nnas_scrittura = false\n", false},
		{"il NAS di prova", `C:\prove\_nas_test\PREVENTIVI DA FARE`, soloNas, false},
		{"con il consenso esplicito", produzioneVera + `\PREVENTIVI DA FARE`,
			soloNas + "consenti_nas_produzione = true\n", false},
	}
	for _, c := range casi {
		_, err := Carica(scriviCon(t, "modalita = \"produzione\"\n", nasCon(c.radice), c.sicurezza))
		if c.rifiuta {
			if err == nil {
				t.Errorf("%s: il server è partito e scrive sul NAS vero senza che nessuno l'abbia dichiarato", c.nome)
				continue
			}
			if !strings.Contains(err.Error(), "consenti_nas_produzione") {
				t.Errorf("%s: l'errore non dice come si dichiara il consenso: %v", c.nome, err)
			}
			if !strings.Contains(err.Error(), c.radice) {
				t.Errorf("%s: l'errore non dice quale radice ha riconosciuto: %v", c.nome, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: configurazione rifiutata: %v", c.nome, err)
		}
	}
}

// Quando il consenso c'è, il server parte — e lo dice a voce alta nel log d'avvio. Una capacità
// accesa su una cartella di prova e la stessa capacità accesa sul fascicolo vero di un cliente non
// sono la stessa notizia.
func TestConIlConsensoSiParteMaIlLogLoDice(t *testing.T) {
	cfg, err := Carica(scriviCon(t, "modalita = \"produzione\"\n", nasCon(produzioneVera+`\PREVENTIVI DA FARE`),
		soloNas+"consenti_nas_produzione = true\n"))
	if err != nil {
		t.Fatalf("con il consenso esplicito il server deve partire: %v", err)
	}
	cap := cfg.Capacita()
	if !cap.NasScrittura {
		t.Fatal("la scrittura sul NAS è rimasta spenta nonostante il file la chieda")
	}
	insieme := strings.Join(cap.Avvisi, " | ")
	if !strings.Contains(insieme, "NAS VERO") {
		t.Errorf("l'avvio non avvisa che si sta scrivendo sul NAS di produzione: %q", insieme)
	}
	if !strings.Contains(insieme, produzioneVera) {
		t.Errorf("l'avviso non dice quale radice: %q", insieme)
	}
}

// Sul NAS di prova non si avvisa niente: un avviso che compare sempre non si legge più.
func TestSulNasDiProvaNessunAvvisoDiProduzione(t *testing.T) {
	cfg, err := Carica(scriviCon(t, "modalita = \"produzione\"\n", nasCon(`C:\prove\_nas_test\PREVENTIVI DA FARE`), soloNas))
	if err != nil {
		t.Fatal(err)
	}
	cap := cfg.Capacita()
	if !cap.NasScrittura {
		t.Fatal("la scrittura sul NAS di prova è stata spenta")
	}
	if insieme := strings.Join(cap.Avvisi, " | "); strings.Contains(insieme, "NAS VERO") {
		t.Errorf("il NAS di prova viene annunciato come NAS di produzione: %q", insieme)
	}
}

// La shadow non cambia: lì la radice di produzione resta un rifiuto secco, e `consenti_nas_produzione`
// non la sblocca. Chi scrive «shadow» sta dicendo «questo server non tocca niente», e un consenso
// scritto tre righe sotto non deve poter contraddire quella frase.
func TestInShadowIlConsensoNonSbloccaNiente(t *testing.T) {
	_, err := Carica(scriviCon(t, "modalita = \"shadow\"\n", nasCon(produzioneVera+`\PREVENTIVI DA FARE`),
		soloNas+"consenti_nas_produzione = true\n"))
	if err == nil {
		t.Fatal("in shadow il server è partito puntato sul NAS di produzione")
	}
	if !strings.Contains(err.Error(), "radici_produzione") {
		t.Errorf("l'errore non è quello della shadow: %v", err)
	}
}
