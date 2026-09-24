package cockpit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Gli avviatori di scripts/avvio-rete sono bash, e girano in due posti diversi: Git Bash su Windows e
// bash sulla VM Linux. Qui si passano a `bash -n` (analisi senza esecuzione), e poi si lanciano con
// --mostra, che sceglie l'indirizzo e stampa il comando del server senza avviare niente.

// bashDiProva trova bash: nel PATH (Git Bash, Linux), altrimenti quello di Git for Windows. Senza,
// la prova si salta e lo dice.
func bashDiProva(t *testing.T) string {
	t.Helper()
	if b, err := exec.LookPath("bash"); err == nil && !strings.EqualFold(filepath.Dir(b), `C:\Windows\System32`) {
		return b
	}
	if runtime.GOOS == "windows" {
		if b := filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"); fileEsiste(b) {
			return b
		}
	}
	t.Skip("bash non trovato (Git Bash o bash di Linux): gli avviatori di rete non si possono provare")
	return ""
}

func fileEsiste(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestGliAvviatoriDiReteSonoAnalizzabili(t *testing.T) {
	bash := bashDiProva(t)
	script, err := filepath.Glob(filepath.Join("scripts", "avvio-rete", "*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(script) < 3 {
		t.Fatalf("attesi comune.sh, avvia-lan.sh, avvia-https.sh: trovati %v", script)
	}
	for _, s := range script {
		dati, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		// Un \r e' un difetto anche se bash -n passa: la riga «set -euo pipefail\r» si ferma all'avvio.
		if strings.Contains(string(dati), "\r") {
			t.Errorf("%s ha righe CRLF: bash su Windows e su Linux si ferma alla prima (.gitattributes: *.sh eol=lf)", s)
		}
		uscita, err := exec.Command(bash, "-n", s).CombinedOutput()
		if err != nil {
			t.Errorf("%s non passa bash -n (%v):\n%s", s, err, uscita)
		}
	}
}

// --mostra non avvia niente: dice su che indirizzo ascolterebbe, con quale certificato e quali reti.
// 127.0.0.1 e ::1 ci sono su ogni PC, cosi' la prova non dipende dalla rete di chi la lancia.
func TestGliAvviatoriDiReteMostranoIlComando(t *testing.T) {
	bash := bashDiProva(t)
	// cockpit.toml non e' nel repository (e' gitignorato): la prova porta il suo, perche' un checkout
	// pulito deve poterla eseguire. --mostra non lo legge oltre a controllare che esista. Barre in
	// avanti: `dirname` di bash non capisce le rovesciate di Windows.
	config := filepath.ToSlash(filepath.Join(t.TempDir(), "cockpit.toml"))
	if err := os.WriteFile(config, []byte("[db]\ndsn = \"postgres://x@localhost/y\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lancia := func(script string, arg ...string) (string, error) {
		if len(arg) == 0 || arg[0] != "--config" {
			arg = append([]string{"--config", config}, arg...)
		}
		cmd := exec.Command(bash, append([]string{filepath.Join("scripts", "avvio-rete", script)}, arg...)...)
		uscita, err := cmd.CombinedOutput()
		return string(uscita), err
	}
	casi := []struct {
		script string
		arg    []string
		attesi []string
	}{
		{"avvia-lan.sh", []string{"-a", "127.0.0.1", "--mostra"},
			[]string{"-ascolto 127.0.0.1:8443", "-tls-cert tls/lan/cert.pem", "-url-pubblico https://127.0.0.1:8443", "-reti 127.0.0.1/8", "questo PC soltanto"}},
		{"avvia-lan.sh", []string{"-a", "127.0.0.1", "-p", "9443", "-c", "10.0.0.15,10.0.0.16", "--mostra"},
			[]string{"-ascolto 127.0.0.1:9443", "-reti 10.0.0.15,10.0.0.16"}},
		{"avvia-https.sh", []string{"-a", "::1", "--mostra"},
			[]string{"-ascolto '[::1]:8443'", "-tls-cert tls/https/cert.pem", "-url-pubblico 'https://[::1]:8443'", "-reti ::1/128"}},
		{"avvia-https.sh", []string{"-a", "fd12:3456:789a:1::7/64", "--mostra"},
			[]string{"-ascolto '[fd12:3456:789a:1::7]:8443'", "-reti fd12:3456:789a:1::7/64"}},
	}
	for _, c := range casi {
		uscita, err := lancia(c.script, c.arg...)
		if err != nil {
			t.Errorf("%s %v: %v\n%s", c.script, c.arg, err, uscita)
			continue
		}
		for _, a := range c.attesi {
			if !strings.Contains(uscita, a) {
				t.Errorf("%s %v: manca %q in\n%s", c.script, c.arg, a, uscita)
			}
		}
	}
	rifiuti := []struct {
		script string
		arg    []string
		atteso string
	}{
		{"avvia-https.sh", []string{"-a", "fe80::1", "--mostra"}, "link-local"},
		{"avvia-https.sh", []string{"-a", "10.0.0.7", "--mostra"}, "non e' un IPv6"},
		{"avvia-https.sh", []string{"-a", "::ffff:10.0.0.7", "--mostra"}, "IPv4 in forma IPv6"},
		{"avvia-lan.sh", []string{"-a", "fd00::1", "--mostra"}, "non e' un IPv4"},
		{"avvia-lan.sh", []string{"-a", "192.0.2.123", "--mostra"}, "non e' un indirizzo IPv4 di questo PC"},
		{"avvia-lan.sh", []string{"-p", "99999", "--mostra"}, "porta non valida"},
		{"avvia-lan.sh", []string{"--config", "non-esiste.toml", "--mostra"}, "file di configurazione non trovato"},
		{"avvia-lan.sh", []string{"--boh"}, "opzione sconosciuta"},
	}
	for _, r := range rifiuti {
		uscita, err := lancia(r.script, r.arg...)
		if err == nil {
			t.Errorf("%s %v: accettato, doveva fermarsi (%s)\n%s", r.script, r.arg, r.atteso, uscita)
			continue
		}
		if !strings.Contains(uscita, r.atteso) {
			t.Errorf("%s %v: atteso %q, ottenuto:\n%s", r.script, r.arg, r.atteso, uscita)
		}
	}
}
