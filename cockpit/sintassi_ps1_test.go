package cockpit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Il 21/09/2026 installa-postazione.ps1 e' arrivato sul banco reale senza nemmeno essere analizzato
// da Windows PowerShell 5.1: due interpolazioni seguite subito da ':' ("$tomlPath:", "$python:")
// sono un riferimento a variabile non valido, e l'errore e' di analisi, non di esecuzione: lo script
// non parte affatto. Nessun test lo intercettava perche' i .ps1 sono solo testo per Go.
//
// Qui li diamo in pasto all'analizzatore di PowerShell, che e' gia' sulla macchina: nessuna
// dipendenza nuova. Non esegue niente, legge soltanto (ParseFile), quindi e' innocuo anche per gli
// script che toccano il database o le attivita' pianificate.
const analizza = `
$errori = $null
$token = $null
[void][System.Management.Automation.Language.Parser]::ParseFile($env:COCKPIT_PS1, [ref]$token, [ref]$errori)
if ($errori.Count) {
    $errori | ForEach-Object { 'riga {0}, colonna {1}: {2}' -f $_.Extent.StartLineNumber, $_.Extent.StartColumnNumber, $_.Message }
    exit 1
}
exit 0
`

func TestGliScriptPowerShellSonoAnalizzabili(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("l'analizzatore di PowerShell c'e' solo su Windows")
	}
	powershell, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("powershell non nel PATH:", err)
	}

	// Lo script della postazione si prova COME VIAGGIA, cioe' dall'embed: e' quello che finisce nel
	// pacchetto, non il file sul disco dello sviluppatore.
	dati, err := FS.ReadFile("workers/installa-postazione.ps1")
	if err != nil {
		t.Fatalf("installa-postazione.ps1 non e' nell'embed: %v", err)
	}
	incorporato := filepath.Join(t.TempDir(), "installa-postazione.ps1")
	if err := os.WriteFile(incorporato, dati, 0o644); err != nil {
		t.Fatal(err)
	}

	script := []string{incorporato}
	altri, err := filepath.Glob(filepath.Join("scripts", "*.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(altri) == 0 {
		t.Fatal("nessuno script in scripts/: il percorso e' cambiato?")
	}
	script = append(script, altri...)

	for _, s := range script {
		nome := filepath.Base(s)
		t.Run(nome, func(t *testing.T) {
			assoluto, err := filepath.Abs(s)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", analizza)
			cmd.Env = append(os.Environ(), "COCKPIT_PS1="+assoluto)
			uscita, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("%s non passa l'analisi di PowerShell (%v):\n%s", nome, err, strings.TrimSpace(string(uscita)))
			}
		})
	}
}
