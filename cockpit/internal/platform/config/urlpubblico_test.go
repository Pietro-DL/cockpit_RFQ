package config

import (
	"strings"
	"testing"
)

// [server].url_pubblico (7C.1, P1): l'indirizzo con cui i worker chiamano il server. Lo schema deve
// essere quello che il listener parla davvero, e non ci va un percorso.
func TestURLPubblicoValidato(t *testing.T) {
	// le righe vanno dentro [server], che scriviCon apre gia' (un secondo [server] e' un errore TOML)
	rifiutati := []struct{ nome, server, atteso string }{
		{"https senza TLS", "url_pubblico = \"https://10.0.0.7:8443\"\n", "usa https:// ma il server parla http://"},
		{"http con TLS", "tls_cert = \"c.pem\"\ntls_key = \"k.pem\"\nurl_pubblico = \"http://10.0.0.7:8443\"\n", "usa http:// ma il server parla https://"},
		{"con un percorso", "url_pubblico = \"http://10.0.0.7:8080/cockpit\"\n", "senza percorso"},
		{"senza schema", "url_pubblico = \"10.0.0.7:8080\"\n", "non valido"},
		{"schema inventato", "url_pubblico = \"ftp://10.0.0.7\"\n", "non valido"},
	}
	for _, c := range rifiutati {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Carica(scriviCon(t, c.server, "", ""))
			if err == nil {
				t.Fatalf("configurazione accettata: doveva essere rifiutata (%s)", c.atteso)
			}
			if !strings.Contains(err.Error(), c.atteso) {
				t.Fatalf("atteso un errore che contenga %q, ottenuto: %v", c.atteso, err)
			}
		})
	}

	c, err := Carica(scriviCon(t, "tls_cert = \"c.pem\"\ntls_key = \"k.pem\"\nurl_pubblico = \"https://cockpit.azienda.local:8443/\"\n", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.URLPubblico != "https://cockpit.azienda.local:8443" {
		t.Errorf("url_pubblico non normalizzato: %q", c.Server.URLPubblico)
	}
	if c.HostPubblico() != "cockpit.azienda.local" {
		t.Errorf("HostPubblico = %q", c.HostPubblico())
	}
	c, err = Carica(scriviCon(t, "url_pubblico = \"http://[::1]:8080\"\n", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.HostPubblico() != "::1" {
		t.Errorf("HostPubblico con IPv6 = %q", c.HostPubblico())
	}
	// non dichiarato: vuoto, e l'host pubblico e' vuoto (si deriva dal bind altrove)
	c, err = Carica(scrivi(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.URLPubblico != "" || c.HostPubblico() != "" {
		t.Errorf("senza url_pubblico: %q %q", c.Server.URLPubblico, c.HostPubblico())
	}
}
