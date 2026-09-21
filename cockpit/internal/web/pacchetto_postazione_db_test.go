//go:build integrazione

// L4 — 7C.1, P1: il pacchetto della postazione porta cio' che serve a installarla senza aprire
// PowerShell a mano: lo script di installazione, il certificato PUBBLICO del server per il browser,
// l'indirizzo dichiarato in [server].url_pubblico. E non porta mai la chiave privata.
package web

import (
	"path/filepath"
	"strings"
	"testing"

	"promatec/cockpit/internal/rete"
)

func TestIlPacchettoPortaIlCertificatoPubblicoELoScriptDellaPostazione(t *testing.T) {
	b := preparaBancoWeb(t)
	dir := t.TempDir()
	materiale, err := rete.Prepara(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), []string{"localhost", "10.0.0.7"})
	if err != nil {
		t.Fatal(err)
	}
	b.ws.TLS = materiale
	b.ws.URLPubblico = "https://10.0.0.7:8443"
	admin := b.browser("10.0.0.9:5000")
	admin.login("AD", "prova-ad")

	dentro := scarica(t, admin, "PC-FRANCESCO")
	for _, atteso := range []string{"worker.toml", "ISTRUZIONI.txt", "installa-postazione.ps1", "cert.pem", "requirements.txt"} {
		if _, ok := dentro[atteso]; !ok {
			t.Errorf("il pacchetto non contiene %s (dentro: %v)", atteso, chiavi(dentro))
		}
	}
	if dentro["cert.pem"] != string(materiale.PEM) {
		t.Error("cert.pem non e' il certificato del server")
	}
	for nome, contenuto := range dentro {
		if strings.Contains(contenuto, "PRIVATE KEY") {
			t.Errorf("%s contiene una chiave privata: il pacchetto viaggia sui PC degli operatori", nome)
		}
	}
	toml := dentro["worker.toml"]
	if !strings.Contains(toml, `server_url = "https://10.0.0.7:8443"`) {
		t.Errorf("worker.toml non usa [server].url_pubblico:\n%s", toml)
	}
	if !strings.Contains(toml, `impronta = "`+materiale.Impronta+`"`) {
		t.Errorf("worker.toml senza l'impronta del certificato:\n%s", toml)
	}
	istr := dentro["ISTRUZIONI.txt"]
	for _, frase := range []string{"https://10.0.0.7:8443", "installa-postazione.ps1", "-Ferma", "INVALIDA", "cert.pem"} {
		if !strings.Contains(istr, frase) {
			t.Errorf("le istruzioni non dicono %q:\n%s", frase, istr)
		}
	}
	script := dentro["installa-postazione.ps1"]
	for _, frase := range []string{"Register-ScheduledTask", "Cert:\\CurrentUser\\Root", "-Ferma", "pythonw", "IgnoreNew", "LogonType Interactive"} {
		if !strings.Contains(script, frase) {
			t.Errorf("lo script della postazione non contiene %q", frase)
		}
	}

	// in chiaro: nessun cert.pem, e le istruzioni non ne parlano
	b.ws.TLS = nil
	b.ws.URLPubblico = ""
	chiaro := scarica(t, admin, "PC-FRANCESCO")
	if _, c := chiaro["cert.pem"]; c {
		t.Error("il pacchetto di un server in chiaro porta un cert.pem")
	}
	if !strings.Contains(chiaro["worker.toml"], "server_url = \"http://") {
		t.Errorf("senza url_pubblico l'indirizzo si deriva dal bind:\n%s", chiaro["worker.toml"])
	}
}
