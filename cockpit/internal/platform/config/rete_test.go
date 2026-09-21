package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L1 — voce 2.4: il server non si espone in chiaro fuori da questo PC, e i percorsi del NAS si
// confrontano allo stesso modo su Windows e su Linux (il server va su una VM Linux, D23).

func TestSuLoopbackRiconosceDoveSiAscolta(t *testing.T) {
	casi := map[string]bool{
		"127.0.0.1:8080": true,
		"localhost:8080": true,
		"[::1]:8080":     true,
		"LOCALHOST:443":  true,
		"0.0.0.0:8080":   false,
		":8080":          false, // senza host = tutte le interfacce, cioè la LAN
		"[::]:8080":      false,
		"10.0.0.7:8443":  false,
		"cockpit:8443":   false, // un nome che non si può dire loopback: nel dubbio, non lo è
	}
	for indirizzo, atteso := range casi {
		if SuLoopback(indirizzo) != atteso {
			t.Errorf("SuLoopback(%q) = %v, atteso %v", indirizzo, !atteso, atteso)
		}
	}
}

func TestInChiaroFuoriDaQuestoPCIlServerNonParte(t *testing.T) {
	_, err := Carica(scriviCon(t, "indirizzo = \"0.0.0.0:8080\"\n", "", ""))
	if err == nil {
		t.Fatal("un server in chiaro sulla LAN è partito: posta, token dei worker e cookie di sessione leggibili da chiunque")
	}
	// Il messaggio deve dire tutte e due le uscite, o chi lo legge ne cerca una a caso.
	for _, atteso := range []string{"tls_cert", "consenti_lan_in_chiaro"} {
		if !strings.Contains(err.Error(), atteso) {
			t.Errorf("il rifiuto non nomina %q: %v", atteso, err)
		}
	}
}

func TestInChiaroSuLoopbackVaBene(t *testing.T) {
	// È lo sviluppo di tutti i giorni: niente TLS, niente certificati, nessun avviso.
	c, err := Carica(scriviCon(t, "indirizzo = \"127.0.0.1:8080\"\n", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.ETLS() || c.Schema() != "http" {
		t.Errorf("TLS=%v schema=%s", c.ETLS(), c.Schema())
	}
}

func TestLaViaDUscitaInChiaroVaDichiarata(t *testing.T) {
	c, err := Carica(scriviCon(t, "indirizzo = \"0.0.0.0:8080\"\nconsenti_lan_in_chiaro = true\n", "", ""))
	if err != nil {
		t.Fatalf("la dichiarazione esplicita non è stata accettata: %v", err)
	}
	if c.ETLS() {
		t.Error("ETLS con nessun certificato")
	}
}

func TestConIlCertificatoIlServerPuoStareInLan(t *testing.T) {
	d := t.TempDir()
	cert, chiave := filepath.Join(d, "cert.pem"), filepath.Join(d, "key.pem")
	c, err := Carica(scriviCon(t, "indirizzo = \"0.0.0.0:8443\"\ntls_cert = '"+cert+"'\ntls_key = '"+chiave+"'\n", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !c.ETLS() || c.Schema() != "https" {
		t.Fatalf("TLS=%v schema=%s", c.ETLS(), c.Schema())
	}
	// I file non esistono ancora: li genera il server all'avvio. La configurazione non deve
	// pretenderli, o il primo avvio su una macchina nuova non avverrebbe mai.
	if _, err := os.Stat(cert); err == nil {
		t.Error("la configurazione ha creato il certificato: è compito dell'avvio")
	}
}

func TestMezzoTLSNonEsiste(t *testing.T) {
	for _, riga := range []string{"tls_cert = 'c.pem'\n", "tls_key = 'k.pem'\n"} {
		if _, err := Carica(scriviCon(t, riga, "", "")); err == nil {
			t.Errorf("accettato %q da solo", strings.TrimSpace(riga))
		}
	}
}

func TestIPercorsiRelativiSiLeggonoDaDoveStaIlFile(t *testing.T) {
	// Un percorso in un file si legge rispetto al file, non rispetto alla cartella da cui è stato
	// lanciato l'eseguibile: un servizio di sistema parte da C:\Windows\System32, o da /.
	p := scriviCon(t, "indirizzo = \"127.0.0.1:8443\"\ntls_cert = \"tls/cert.pem\"\ntls_key = \"tls/key.pem\"\n", "", "")
	c, err := Carica(p)
	if err != nil {
		t.Fatal(err)
	}
	atteso := filepath.Join(filepath.Dir(p), "tls", "cert.pem")
	if c.Server.TLSCert != atteso {
		t.Errorf("tls_cert = %q, atteso %q", c.Server.TLSCert, atteso)
	}
}

// SH3 (b) sul server Linux: le radici di produzione si confrontano anche scritte alla maniera POSIX.
// Il server va su una VM Linux (D23) e lì il NAS è una share SMB montata, quindi `/mnt/nas/...`.
func TestRadiciDiProduzioneAncheConIPercorsiPosix(t *testing.T) {
	nas := "radice = '/mnt/nas/TECNICO - PREVENTIVI/PREVENTIVI DA FARE/2026'\n" +
		"radici_produzione = ['/mnt/nas/TECNICO - PREVENTIVI/PREVENTIVI DA FARE/']\n"
	_, err := Carica(scriviCon(t, "modalita = \"shadow\"\n", nas, ""))
	if err == nil {
		t.Fatal("una shadow puntata sul NAS di produzione è partita (percorsi POSIX)")
	}
	if !strings.Contains(err.Error(), "radici_produzione") {
		t.Errorf("il rifiuto non dice dove si cambia: %v", err)
	}
}
