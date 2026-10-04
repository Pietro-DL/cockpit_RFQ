// L1 — Il controllo dei dati privati prima del push (scripts/controlla-privati.ps1; R2, R45, R46).
//
// Ogni caso costruisce al momento un repository Git sintetico in una cartella temporanea, con un remoto nudo
// locale (niente rete) e una cartella «privata» con il suo manifest. Token, nomi e contenuti privati sono
// inventati (ZZPRIVATO1, privato.example): il repository è pubblico, e qui non c'è nessun dato reale. Per ogni
// caso si controllano il codice d'uscita (0 pulito, 1 trovato, 2 uso, 3 NON ESEGUITO) e che a video non compaia
// mai un token in chiaro.
//
// La prova non usa testutil, che importa questo pacchetto (ciclo).
package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// I segreti inventati del banco: nessuno deve comparire nell'uscita dello script.
var segretiDiProva = []string{"ZZPRIVATO1", "privato.example", "Zebrina", "disegno_zz4242.pdf", "contenuto privato di prova"}

const tokenDiProva = `# elenco di prova: tutto inventato
[codici fisso]
ZZPRIVATO1
[domini fisso]
privato.example
[clienti parola]
Zebrina
[sigle sigla]
ZZS
`

type bancoPrivati struct {
	t        *testing.T
	script   string
	lavoro   string
	privato  string
	manifest string
	env      []string
}

func richiediAmbientePrivati(t *testing.T) (powershell string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("SALTATO-AMBIENTE: il controllo dei dati privati è uno script di Windows PowerShell")
	}
	p, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("SALTATO-AMBIENTE: powershell non nel PATH:", err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("SALTATO-AMBIENTE: git non nel PATH:", err)
	}
	return p
}

// nuovoBancoPrivati: remoto nudo, repository di lavoro con un commit già spinto, cartella privata con manifest ed
// elenchi. La configurazione di Git è isolata: niente impostazioni globali o di sistema della macchina.
func nuovoBancoPrivati(t *testing.T) *bancoPrivati {
	t.Helper()
	radice := t.TempDir()
	script, err := filepath.Abs(filepath.Join("scripts", "controlla-privati.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	vuota := filepath.Join(radice, "gitconfig")
	scriviFile(t, vuota, "")
	b := &bancoPrivati{
		t:        t,
		script:   script,
		lavoro:   filepath.Join(radice, "lavoro"),
		privato:  filepath.Join(radice, "privato"),
		manifest: filepath.Join(radice, "privato", "manifest_acme.json"),
		env: append(os.Environ(), "GIT_CONFIG_GLOBAL="+vuota, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
			"COCKPIT_DATASET_A="),
	}
	remoto := filepath.Join(radice, "remoto.git")
	b.gitIn(radice, "init", "--bare", "-b", "principale", remoto)
	b.gitIn(radice, "init", "-b", "principale", b.lavoro)
	b.git("config", "user.name", "Prova ACME")
	b.git("config", "user.email", "prova@acme.example")
	b.git("config", "core.autocrlf", "false")
	b.git("config", "commit.gpgsign", "false")
	b.scrivi(".gitignore", "docs/\n")
	b.scrivi("LEGGIMI.txt", "progetto di prova\n")
	b.git("add", ".")
	b.git("commit", "-m", "primo commit")
	b.git("remote", "add", "origin", remoto)
	b.git("push", "-u", "origin", "principale")
	b.git("fetch", "origin")

	scriviFile(t, filepath.Join(b.privato, "segreto.txt"), "contenuto privato di prova\nseconda riga\n")
	scriviFile(t, filepath.Join(b.privato, "radice", "bozza.txt"), "bozza privata del banco\n")
	scriviFile(t, filepath.Join(b.privato, "elenchi", "token.txt"), tokenDiProva)
	scriviFile(t, filepath.Join(b.privato, "elenchi", "nomi.txt"), "# nomi privati di prova\ndisegno_zz4242.pdf\n")
	scriviFile(t, filepath.Join(b.privato, "elenchi", "eccezioni.txt"), "# nessuna eccezione\n")
	scriviFile(t, filepath.Join(b.privato, "elenchi", "radici.txt"), "radice\n")
	b.scriviManifest()
	return b
}

func scriviFile(t *testing.T, p, testo string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(testo), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scriviManifest ricalcola sha256 e byte delle voci: va richiamata dopo ogni modifica a un elenco.
func (b *bancoPrivati) scriviManifest() {
	b.t.Helper()
	type voce struct {
		Nome     string `json:"nome"`
		Percorso string `json:"percorso"`
		Ruolo    string `json:"ruolo"`
		Sha256   string `json:"sha256"`
		Byte     int    `json:"byte"`
	}
	var voci []voce
	for _, v := range [][3]string{
		{"fixture.segreto", "segreto.txt", "fixture"},
		{"controllo.token", "elenchi/token.txt", "controllo"},
		{"controllo.nomi", "elenchi/nomi.txt", "controllo"},
		{"controllo.eccezioni", "elenchi/eccezioni.txt", "controllo"},
		{"controllo.radici", "elenchi/radici.txt", "controllo"},
	} {
		dati, err := os.ReadFile(filepath.Join(b.privato, filepath.FromSlash(v[1])))
		if errors.Is(err, os.ErrNotExist) {
			// un elenco tolto apposta resta nel manifest: lo script deve dire che non lo raggiunge
			voci = append(voci, voce{v[0], v[1], v[2], strings.Repeat("0", 64), 0})
			continue
		}
		if err != nil {
			b.t.Fatal(err)
		}
		s := sha256.Sum256(dati)
		voci = append(voci, voce{v[0], v[1], v[2], hex.EncodeToString(s[:]), len(dati)})
	}
	m, err := json.MarshalIndent(map[string]any{"versione_manifest": 1, "voci": voci, "copia": map[string]any{}}, "", "  ")
	if err != nil {
		b.t.Fatal(err)
	}
	scriviFile(b.t, b.manifest, string(m))
}

func (b *bancoPrivati) gitIn(dir string, arg ...string) string {
	b.t.Helper()
	cmd := exec.Command("git", arg...)
	cmd.Dir = dir
	cmd.Env = b.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		b.t.Fatalf("git %s: %v\n%s", strings.Join(arg, " "), err, out)
	}
	return string(out)
}

func (b *bancoPrivati) git(arg ...string) string { b.t.Helper(); return b.gitIn(b.lavoro, arg...) }

func (b *bancoPrivati) scrivi(rel, testo string) {
	b.t.Helper()
	scriviFile(b.t, filepath.Join(b.lavoro, filepath.FromSlash(rel)), testo)
}

// committa scrive il file, lo mette nell'indice anche se ignorato (git add -f) e fa il commit.
func (b *bancoPrivati) committa(rel, testo, messaggio string) {
	b.t.Helper()
	b.scrivi(rel, testo)
	b.git("add", "-f", "--", rel)
	b.git("commit", "-m", messaggio)
}

// controlla lancia lo script nel repository di lavoro; conManifest false lascia COCKPIT_DATASET_A vuota.
func (b *bancoPrivati) controlla(powershell string, conManifest bool, arg ...string) (int, string) {
	b.t.Helper()
	cmd := exec.Command(powershell, append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", b.script}, arg...)...)
	cmd.Dir = b.lavoro
	cmd.Env = b.env
	if conManifest {
		cmd.Env = append(cmd.Env, "COCKPIT_DATASET_A="+b.manifest)
	}
	out, err := cmd.CombinedOutput()
	codice := 0
	var uscita *exec.ExitError
	if errors.As(err, &uscita) {
		codice = uscita.ExitCode()
	} else if err != nil {
		b.t.Fatalf("lo script non parte: %v", err)
	}
	testo := strings.ReplaceAll(string(out), "\r\n", "\n")
	for _, s := range segretiDiProva {
		if strings.Contains(strings.ToLower(testo), strings.ToLower(s)) {
			b.t.Errorf("un segreto di prova compare in chiaro nell'uscita:\n%s", testo)
		}
	}
	return codice, testo
}

func ultimaRiga(testo string) string {
	righe := strings.Split(strings.TrimRight(testo, "\n"), "\n")
	return righe[len(righe)-1]
}

func impronta(testo string) string {
	s := sha256.Sum256([]byte(testo))
	return hex.EncodeToString(s[:])
}

func TestControllaPrivati(t *testing.T) {
	powershell := richiediAmbientePrivati(t)

	atteso := func(t *testing.T, codice int, testo string, voluto int, contiene ...string) {
		t.Helper()
		if codice != voluto {
			t.Fatalf("codice d'uscita %d, atteso %d:\n%s", codice, voluto, testo)
		}
		for _, c := range contiene {
			if !strings.Contains(testo, c) {
				t.Errorf("manca %q nell'uscita:\n%s", c, testo)
			}
		}
	}
	ramo := []string{"-Ramo", "principale"}

	t.Run("commit pulito: 0", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("src/a.go", "package a\n", "un file pulito")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 0, "Commit da pubblicare: 1")
		if !strings.HasPrefix(ultimaRiga(testo), "PULITO: 5 controlli eseguiti") {
			t.Errorf("ultima riga inattesa: %q", ultimaRiga(testo))
		}
	})

	t.Run("percorsi del codice simili a quelli privati: nessun falso allarme", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("internal/platform/dataset/dataset.go", "package dataset\n", "il pacchetto del dataset")
		b.committa("internal/app/bancoa/attesi.go", "package bancoa\n", "il lettore degli attesi")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 0)
	})

	t.Run("controllo 1: un file sotto docs/ aggiunto con add -f", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("docs/x.txt", "nota\n", "una nota")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "[1] percorso vietato")
	})

	t.Run("controllo 2: un file uguale al file privato, grezzo e con CRLF", func(t *testing.T) {
		t.Parallel()
		for _, testo := range []string{"contenuto privato di prova\nseconda riga\n", "contenuto privato di prova\r\nseconda riga\r\n"} {
			b := nuovoBancoPrivati(t)
			b.committa("copie/a.txt", testo, "una copia")
			codice, uscita := b.controlla(powershell, true, ramo...)
			atteso(t, codice, uscita, 1, "[2] un oggetto da pubblicare")
		}
	})

	t.Run("controllo 2: un file uguale a uno sotto le radici private", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("copie/b.txt", "bozza privata del banco\n", "una bozza")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "[2] un oggetto da pubblicare")
	})

	t.Run("controllo 3: un file con un nome privato", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("allegati/DISEGNO_ZZ4242.PDF", "x\n", "un allegato")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "[3] nome di file privato")
	})

	t.Run("controllo 4: un token in una riga aggiunta", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("src/b.go", "// il codice ZZPRIVATO1 qui\n", "un commento")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "[4] codici ZZ…1(10)", "src/b.go:1")
	})

	t.Run("controllo 4: un token nel messaggio del commit", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("src/c.go", "package c\n", "aggiunto per mail@privato.example")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "[4] domini", "messaggio del commit")
	})

	t.Run("controllo 4: confini di parola e maiuscole", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		// «Zebrinas» e «ZZSX» contengono un token ma non a parola intera; «zzs» non ha le maiuscole della sigla
		b.committa("src/d.go", "// Zebrinas, ZZSX, zzs, _x\n", "parole vicine")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 0)
		// a parola intera, anche con «_» come separatore e senza maiuscole per il modo parola
		b.committa("src/e.go", "// cliente_zebrina e ZZS\n", "parole intere")
		codice, testo = b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "[4] clienti Ze…a(7)", "[4] sigle Z…(3)")
	})

	pushConToken := func(t *testing.T) *bancoPrivati {
		b := nuovoBancoPrivati(t)
		b.committa("vecchio.txt", "riga con ZZPRIVATO1\n", "un file già pubblicato")
		b.git("push", "origin", "principale")
		return b
	}

	t.Run("controllo 5: token già spinto e in eccezione: 0", func(t *testing.T) {
		t.Parallel()
		b := pushConToken(t)
		scriviFile(t, filepath.Join(b.privato, "elenchi", "eccezioni.txt"),
			"vecchio.txt\t"+impronta("riga con ZZPRIVATO1")+"\tcodici\toccorrenza nota del banco\n")
		b.scriviManifest()
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 0, "Commit da pubblicare: 0", "occorrenze note: 1 (eccezioni)")
	})

	t.Run("controllo 4: token già spinto senza eccezione: 1", func(t *testing.T) {
		t.Parallel()
		b := pushConToken(t)
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "albero in punta, vecchio.txt:1")
	})

	t.Run("controllo 5: eccezione per una riga cambiata: 1 e avviso", func(t *testing.T) {
		t.Parallel()
		b := pushConToken(t)
		scriviFile(t, filepath.Join(b.privato, "elenchi", "eccezioni.txt"),
			"vecchio.txt\t"+impronta("riga con ZZPRIVATO1")+"\tcodici\toccorrenza nota del banco\n")
		b.scriviManifest()
		b.committa("vecchio.txt", "riga cambiata con ZZPRIVATO1\n", "la riga cambia")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 1, "eccezione scaduta")
	})

	t.Run("senza manifest: 3, con il controllo 1 eseguito", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("src/a.go", "package a\n", "un file pulito")
		codice, testo := b.controlla(powershell, false, ramo...)
		atteso(t, codice, testo, 3)
		if u := ultimaRiga(testo); !strings.HasPrefix(u, "NON ESEGUITO:") || !strings.HasSuffix(u, "controllo 1: pulito") {
			t.Errorf("ultima riga inattesa: %q", u)
		}
	})

	t.Run("senza un elenco: 3", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		if err := os.Remove(filepath.Join(b.privato, "elenchi", "nomi.txt")); err != nil {
			t.Fatal(err)
		}
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 3, "NON ESEGUITO: elenco non raggiungibile: controllo.nomi")
	})

	t.Run("un elenco cambiato dopo il manifest: 3", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		scriviFile(t, filepath.Join(b.privato, "elenchi", "token.txt"), tokenDiProva+"[altri fisso]\nZZALTRO9\n")
		codice, testo := b.controlla(powershell, true, ramo...)
		atteso(t, codice, testo, 3, "sha256 o byte diversi dal manifest: controllo.token")
	})

	t.Run("senza manifest ma con un percorso vietato: 1 prevale su 3", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.committa("docs/x.txt", "nota\n", "una nota")
		codice, testo := b.controlla(powershell, false, ramo...)
		atteso(t, codice, testo, 1, "[1] percorso vietato", "NON ESEGUITO in parte")
	})

	t.Run("ramo inesistente: 2", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		codice, testo := b.controlla(powershell, true, "-Ramo", "nessuno")
		atteso(t, codice, testo, 2, "ERRORE D'USO: il ramo non esiste")
	})

	t.Run("ramo nuovo senza upstream con la base sul remoto: solo i commit nuovi", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.git("switch", "-c", "nuovo")
		b.committa("src/n.go", "package n\n", "il primo commit del ramo nuovo")
		codice, testo := b.controlla(powershell, true, "-Ramo", "nuovo")
		atteso(t, codice, testo, 0, "Commit da pubblicare: 1")
	})

	t.Run("-PrimaDelCommit: diff in staging e messaggio", func(t *testing.T) {
		t.Parallel()
		b := nuovoBancoPrivati(t)
		b.scrivi("src/p.go", "package p\n")
		b.git("add", "src/p.go")
		messaggio := filepath.Join(b.privato, "messaggio.txt")
		scriviFile(t, messaggio, "un file pulito\n")
		codice, testo := b.controlla(powershell, true, "-PrimaDelCommit", "-Messaggio", messaggio)
		atteso(t, codice, testo, 0, "PULITO: controlli 1, 3 e 4")

		scriviFile(t, messaggio, "per Zebrina\n")
		codice, testo = b.controlla(powershell, true, "-PrimaDelCommit", "-Messaggio", messaggio)
		atteso(t, codice, testo, 1, "[4] clienti", "messaggio, riga 1")

		scriviFile(t, messaggio, "un file pulito\n")
		b.scrivi("src/q.go", "// ZZPRIVATO1\n")
		b.git("add", "src/q.go")
		codice, testo = b.controlla(powershell, true, "-PrimaDelCommit", "-Messaggio", messaggio)
		atteso(t, codice, testo, 1, "[4] codici", "staging, src/q.go:1")

		b.git("reset", "-q")
		b.scrivi("docs/y.txt", "nota\n")
		b.git("add", "-f", "docs/y.txt")
		codice, testo = b.controlla(powershell, false, "-PrimaDelCommit")
		atteso(t, codice, testo, 1, "[1] percorso vietato")
	})
}
