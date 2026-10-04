package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L1 — il comando del banco (A1a-BA, la parte sui flag e sui codici d'uscita; R44): conforme → 0; un caso
// fallito → 1; flag sbagliati o mancanti, -uscita dentro il modulo → 2, senza rapporto e con la causa su
// stderr; manifest assente o voce con un'impronta diversa → 3, con il rapporto che comincia con «ESITO: NON
// ESEGUITO —»; una differenza e una voce assente insieme → 1. L'aiuto usa segnaposti, mai un percorso reale.
//
// I clienti di questi test sono inventati. Non è pigrizia: il dataset vero è privato, e questo repository è
// pubblico. Il dataset è quello ACME di internal/app/bancoa/testdata, copiato in t.TempDir() (fuori dal
// modulo) con gli sha256 calcolati sui byte copiati.

const testdataBanco = "../../internal/app/bancoa/testdata"

func shaDi(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func leggi(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdataBanco, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n") // il checkout può avere CRLF: gli sha256 si calcolano su ciò che si scrive
}

// dataset: copia attesi, indice e grammatica ACME, poi scrive il manifest con sha256 e byte veri. muta cambia
// gli attesi prima del calcolo. Restituisce la cartella e il percorso del manifest.
func dataset(t *testing.T, muta func(string) string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	scrivi := func(rel, s string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	zero := strings.Repeat("0", 64)
	g := leggi(t, "regole/acme.v1.json")
	ix := strings.Replace(leggi(t, "regole/indice_acme.v1.json"), zero, shaDi([]byte(g)), 1)
	at := leggi(t, "attesi_acme.yaml")
	if muta != nil {
		at = muta(at)
	}
	scrivi("regole/acme.v1.json", g)
	scrivi("regole/indice_acme.v1.json", ix)
	scrivi("attesi_acme.yaml", at)
	m := leggi(t, "manifest_acme.json")
	for _, x := range []struct{ percorso, contenuto string }{{"attesi_acme.yaml", at}, {"regole/indice_acme.v1.json", ix}} {
		vecchio := `"percorso": "` + x.percorso + `",
      "ruolo": `
		i := strings.Index(m, vecchio)
		if i < 0 {
			t.Fatalf("voce %s non trovata nel manifest di testdata", x.percorso)
		}
		resto := m[i:]
		resto = strings.Replace(resto, `"sha256": "`+zero+`"`, `"sha256": "`+shaDi([]byte(x.contenuto))+`"`, 1)
		resto = strings.Replace(resto, `"byte": 0`, `"byte": `+itoa(len(x.contenuto)), 1)
		m = m[:i] + resto
	}
	scrivi("manifest_acme.json", m)
	return dir, filepath.Join(dir, "manifest_acme.json")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func lancia(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	codice := esegui(context.Background(), args, &out, &errb)
	return codice, out.String(), errb.String()
}

func TestUscitaConforme(t *testing.T) {
	dir, manifest := dataset(t, nil)
	for _, modalita := range []string{"regole", "casi"} {
		uscita := filepath.Join(dir, "banco", modalita)
		codice, out, errb := lancia("-modalita", modalita, "-dataset", manifest, "-uscita", uscita)
		if codice != 0 || !strings.HasPrefix(out, "ESITO: ESEGUITO — conforme\n") || errb != "" {
			t.Fatalf("%s: uscita %d\n%s\n%s", modalita, codice, out, errb)
		}
		txt, err := os.ReadFile(filepath.Join(uscita, modalita+".txt"))
		if err != nil || string(txt) != out {
			t.Fatalf("%s: il rapporto scritto è diverso dal riepilogo: %v", modalita, err)
		}
	}
}

func TestUscitaCasoFallito(t *testing.T) {
	dir, manifest := dataset(t, func(s string) string {
		return strings.Replace(s, "      numero_codici: 2\n", "      numero_codici: 3\n", 1)
	})
	codice, out, _ := lancia("-modalita", "casi", "-dataset", manifest, "-uscita", filepath.Join(dir, "banco"))
	if codice != 1 || !strings.HasPrefix(out, "ESITO: ESEGUITO — con differenze (1)\n") {
		t.Fatalf("uscita %d\n%s", codice, out)
	}
}

// TestUscitaUso: ogni errore d'uso esce con 2, scrive la causa su stderr e nessun rapporto.
func TestUscitaUso(t *testing.T) {
	dir, manifest := dataset(t, nil)
	uscita := filepath.Join(dir, "banco")
	casi := map[string][]string{
		"flag sconosciuto":         {"-modalita", "casi", "-dataset", manifest, "-uscita", uscita, "-dsn", "x"},
		"senza modalità":           {"-dataset", manifest, "-uscita", uscita},
		"modalità ignota":          {"-modalita", "exports", "-dataset", manifest, "-uscita", uscita},
		"senza dataset":            {"-modalita", "casi", "-uscita", uscita},
		"senza uscita":             {"-modalita", "casi", "-dataset", manifest},
		"argomenti in più":         {"-modalita", "casi", "-dataset", manifest, "-uscita", uscita, "altro"},
		"uscita dentro il modulo":  {"-modalita", "casi", "-dataset", manifest, "-uscita", "rapporti-non-ammessi"},
		"dataset dentro il modulo": {"-modalita", "casi", "-dataset", filepath.Join(testdataBanco, "manifest_acme.json"), "-uscita", uscita},
		"aiuto":                    {"-h"},
	}
	for nome, args := range casi {
		t.Run(nome, func(t *testing.T) {
			codice, out, errb := lancia(args...)
			if codice != 2 || out != "" || errb == "" {
				t.Fatalf("uscita %d, stdout %q, stderr %q", codice, out, errb)
			}
		})
	}
	if _, err := os.Stat(uscita); !os.IsNotExist(err) {
		t.Fatalf("un errore d'uso ha scritto un rapporto: %v", err)
	}
	if _, err := os.Stat("rapporti-non-ammessi"); !os.IsNotExist(err) {
		t.Fatal("cartella creata dentro il modulo")
	}
	// L'aiuto usa segnaposti.
	_, _, errb := lancia("-h")
	if !strings.Contains(errb, "<manifest del dataset privato>") || !strings.Contains(errb, "<cartella dei rapporti>") {
		t.Fatalf("aiuto: %s", errb)
	}
}

// TestUscitaNonEseguito: il manifest che manca, o una voce con un'impronta diversa, danno 3 e il rapporto
// con il motivo in testa (R44).
func TestUscitaNonEseguito(t *testing.T) {
	dir, _ := dataset(t, nil)
	uscita := filepath.Join(dir, "banco-assente")
	codice, out, _ := lancia("-modalita", "regole", "-dataset", filepath.Join(dir, "manifest-che-non-c-e.json"), "-uscita", uscita)
	if codice != 3 || !strings.HasPrefix(out, "ESITO: NON ESEGUITO — ") {
		t.Fatalf("manifest assente: uscita %d\n%s", codice, out)
	}
	txt, err := os.ReadFile(filepath.Join(uscita, "regole.txt"))
	if err != nil || !strings.HasPrefix(string(txt), "ESITO: NON ESEGUITO —") {
		t.Fatalf("rapporto del non eseguito: %q, %v", txt, err)
	}

	dir, manifest := dataset(t, nil)
	if err := os.WriteFile(filepath.Join(dir, "regole", "indice_acme.v1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	codice, out, _ = lancia("-modalita", "casi", "-dataset", manifest, "-uscita", filepath.Join(dir, "banco"))
	if codice != 3 || !strings.HasPrefix(out, "ESITO: NON ESEGUITO — regole.indice: ") {
		t.Fatalf("voce con impronta diversa: uscita %d\n%s", codice, out)
	}
}

// TestUscitaDifferenzaPiuVoceAssente: un cliente scartato e gli attesi assenti insieme: 1 prevale su 3.
func TestUscitaDifferenzaPiuVoceAssente(t *testing.T) {
	dir, manifest := dataset(t, nil)
	if err := os.WriteFile(filepath.Join(dir, "regole", "acme.v1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "attesi_acme.yaml")); err != nil {
		t.Fatal(err)
	}
	codice, out, _ := lancia("-modalita", "regole", "-dataset", manifest, "-uscita", filepath.Join(dir, "banco"))
	if codice != 1 || !strings.HasPrefix(out, "ESITO: ESEGUITO — con differenze") || !strings.Contains(out, "attesi: non_eseguito") {
		t.Fatalf("uscita %d\n%s", codice, out)
	}
}
