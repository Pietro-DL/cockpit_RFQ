package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// L1 — [server].reti_consentite e la rete dalla riga di comando (scripts/avvio-rete).

func TestRetiConsentiteSoloDellaLAN(t *testing.T) {
	ammesse := map[string]string{
		"10.0.0.0/24":              "10.0.0.0/24",
		"10.0.0.7/24":              "10.0.0.0/24", // l'indirizzo della scheda con il suo prefisso: si maschera
		"10.0.0.15":                "10.0.0.15/32",
		" 172.16.4.0/22 ":          "172.16.4.0/22",
		"192.168.1.0/24":           "192.168.1.0/24",
		"169.254.0.0/16":           "169.254.0.0/16",
		"127.0.0.1":                "127.0.0.1/32",
		"::ffff:192.168.1.0/120":   "192.168.1.0/24", // 4in6 riportato a IPv4
		"fd12:3456:789a:1::/64":    "fd12:3456:789a:1::/64",
		"fd12:3456:789a:1::7/64":   "fd12:3456:789a:1::/64",
		"FD12:3456:789A:1::15":     "fd12:3456:789a:1::15/128",
		"fe80::/64":                "fe80::/64",
		"::1":                      "::1/128",
		"2001:db8:1:2::/64":        "2001:db8:1:2::/64", // globale, un segmento
		"2001:db8:1:2:3:4:5:6/128": "2001:db8:1:2:3:4:5:6/128",
	}
	for voce, attesa := range ammesse {
		p, err := leggiRete(voce)
		if err != nil {
			t.Errorf("%q rifiutata: %v", voce, err)
			continue
		}
		if p.String() != attesa {
			t.Errorf("%q letta come %s, attesa %s", voce, p, attesa)
		}
	}
	rifiutate := map[string]string{
		"0.0.0.0/0":         "non e' una rete della LAN",
		"::/0":              "non e' una rete della LAN",
		"8.8.8.0/24":        "non e' una rete della LAN", // IPv4 pubblico
		"10.0.0.0/7":        "non e' una rete della LAN", // piu' largo del blocco privato
		"172.16.0.0/11":     "non e' una rete della LAN",
		"192.168.0.0/15":    "non e' una rete della LAN",
		"100.64.0.0/10":     "non e' una rete della LAN", // CGNAT: non e' la rete dell'azienda
		"2001:db8:1::/48":   "non e' una rete della LAN", // globale piu' largo di un segmento
		"2000::/3":          "non e' una rete della LAN",
		"fc00::/6":          "non e' una rete della LAN",
		"::ffff:0.0.0.0/95": "va oltre gli indirizzi IPv4",
		"10.0.0.0/33":       "non e' una rete",
		"rete-ufficio":      "non e' un indirizzo",
		"fe80::1%23":        "non e' un indirizzo", // la zona non si confronta
		"fe80::1%23/64":     "non e' una rete",
		"":                  "non e' un indirizzo",
	}
	for voce, atteso := range rifiutate {
		_, err := leggiRete(voce)
		if err == nil {
			t.Errorf("%q ammessa: doveva essere rifiutata (%s)", voce, atteso)
			continue
		}
		if !strings.Contains(err.Error(), atteso) {
			t.Errorf("%q: atteso un errore che contenga %q, ottenuto: %v", voce, atteso, err)
		}
	}
}

func TestRetiConsentiteDalFile(t *testing.T) {
	c, err := Carica(scriviCon(t, "reti_consentite = [\"10.0.0.7/24\", \"fd12:3456:789a:1::/64\"]\n", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Server.Reti) != 2 || c.Server.Reti[0].String() != "10.0.0.0/24" || c.Server.Reti[1].String() != "fd12:3456:789a:1::/64" {
		t.Errorf("reti lette: %v", c.Server.Reti)
	}
	if c.Server.DaRigaDiComando {
		t.Error("una rete dal file segnata come riga di comando")
	}
	// una voce sola fuori dalla LAN ferma l'avvio: un filtro con un buco non e' un filtro
	if _, err := Carica(scriviCon(t, "reti_consentite = [\"10.0.0.0/24\", \"0.0.0.0/0\"]\n", "", "")); err == nil {
		t.Error("0.0.0.0/0 fra le reti consentite: il server e' partito")
	}
	// assente: nessun filtro, com'era prima
	c, err = Carica(scrivi(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Server.Reti) != 0 {
		t.Errorf("senza reti_consentite: %v", c.Server.Reti)
	}
}

// Con -ascolto la rete della riga di comando vale per intero: niente del file sopravvive, nemmeno
// l'«in chiaro» dichiarato per un'altra rete.
func TestLaReteDallaRigaDiComandoValePerIntero(t *testing.T) {
	file := scriviCon(t, "indirizzo = \"0.0.0.0:8080\"\nconsenti_lan_in_chiaro = true\ntls_nomi = [\"vecchio\"]\n"+
		"url_pubblico = \"http://vecchio:8080\"\nreti_consentite = [\"192.168.9.0/24\"]\n", "", "")
	c, err := CaricaConRete(file, Rete{Indirizzo: "[fd12:3456:789a:1::7]:8443", TLSCert: "tls/https/cert.pem", TLSKey: "tls/https/key.pem",
		URLPubblico: "https://[fd12:3456:789a:1::7]:8443/", Reti: []string{"fd12:3456:789a:1::7/64"}})
	if err != nil {
		t.Fatal(err)
	}
	s := c.Server
	if s.Indirizzo != "[fd12:3456:789a:1::7]:8443" || !c.ETLS() || s.URLPubblico != "https://[fd12:3456:789a:1::7]:8443" {
		t.Errorf("rete: %q tls=%v url=%q", s.Indirizzo, c.ETLS(), s.URLPubblico)
	}
	if s.TLSCert != filepath.Join(filepath.Dir(file), "tls", "https", "cert.pem") {
		t.Errorf("il certificato non e' relativo al file di configurazione: %s", s.TLSCert)
	}
	if len(s.TLSNomi) != 0 || s.ConsentiLanInChiaro || len(s.Reti) != 1 || s.Reti[0].String() != "fd12:3456:789a:1::/64" {
		t.Errorf("dal file e' rimasto qualcosa: nomi=%v chiaro=%v reti=%v", s.TLSNomi, s.ConsentiLanInChiaro, s.Reti)
	}
	if !s.DaRigaDiComando || c.HostPubblico() != "fd12:3456:789a:1::7" {
		t.Errorf("DaRigaDiComando=%v host=%q", s.DaRigaDiComando, c.HostPubblico())
	}

	// Il file diceva «in chiaro sulla LAN va bene»; la riga di comando non lo eredita.
	_, err = CaricaConRete(file, Rete{Indirizzo: "10.0.0.7:8080"})
	if err == nil || !strings.Contains(err.Error(), "tls_cert") {
		t.Errorf("in chiaro sulla LAN dalla riga di comando: %v", err)
	}
	// Le verifiche del file valgono anche qui: schema di url_pubblico, reti della LAN.
	_, err = CaricaConRete(file, Rete{Indirizzo: "10.0.0.7:8443", TLSCert: "c.pem", TLSKey: "k.pem", URLPubblico: "http://10.0.0.7:8443"})
	if err == nil || !strings.Contains(err.Error(), "usa http://") {
		t.Errorf("url_pubblico con lo schema sbagliato: %v", err)
	}
	_, err = CaricaConRete(file, Rete{Indirizzo: "10.0.0.7:8443", TLSCert: "c.pem", TLSKey: "k.pem", Reti: []string{"0.0.0.0/0"}})
	if err == nil || !strings.Contains(err.Error(), "non e' una rete della LAN") {
		t.Errorf("0.0.0.0/0 dalla riga di comando: %v", err)
	}
	// Una rete a meta' (certificato o filtro senza dire dove ascoltare) non parte.
	_, err = CaricaConRete(file, Rete{Reti: []string{"10.0.0.0/24"}})
	if err == nil || !strings.Contains(err.Error(), "-ascolto") {
		t.Errorf("reti senza -ascolto: %v", err)
	}
	// Vuota: vale il file, come Carica.
	c, err = CaricaConRete(file, Rete{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.Indirizzo != "0.0.0.0:8080" || !c.Server.ConsentiLanInChiaro || c.Server.DaRigaDiComando {
		t.Errorf("senza rete dalla riga di comando: %q chiaro=%v riga=%v", c.Server.Indirizzo, c.Server.ConsentiLanInChiaro, c.Server.DaRigaDiComando)
	}
}
