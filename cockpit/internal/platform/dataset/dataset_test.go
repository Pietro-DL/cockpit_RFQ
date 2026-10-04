package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L1 — il manifest del dataset privato (A1a-DS; par.3.7.4 del piano A, R2, R44): decodifica stretta; file
// mancante, sha256 o byte cambiati → errore tipizzato (ErrFileMancante, ErrImprontaDiversa), mai un salto;
// percorso dentro il modulo rifiutato; profili; voci con il ruolo controllo; la versione del manifest fissa.
//
// I clienti di questi test sono inventati. Non è pigrizia: il manifest vero elenca file e clienti privati, e
// questo repository è pubblico. Qui il manifest, i file e l'UUID del cliente ACME nascono in t.TempDir(), che
// sta fuori dal modulo; i nomi dei file sono inventati e diversi da quelli del dataset privato.

const uuidACME = "00000000-0000-4000-8000-00000000ac01"

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// manifestACME: un manifest valido con due voci, una del ruolo attesi e una del ruolo controllo, i profili e
// lo storico. I segnaposti {SHA} e {BYTE} si sostituiscono con i valori del file.
const manifestACME = `{
  "versione_manifest": 1,
  "voci": [
    {"nome": "attesi", "percorso": "dati/attesi_acme.yaml", "ruolo": "attesi", "sha256": "{SHA}", "byte": {BYTE}},
    {"nome": "controllo.nomi", "percorso": "controllo/nomi_acme.txt", "ruolo": "controllo", "sha256": "{SHA2}", "byte": {BYTE2}}
  ],
  "profili": {"acme": "` + uuidACME + `"},
  "copia": {},
  "storico": [{"data": "2026-10-04", "voce": "attesi", "sha256_prima": "{SHA}", "motivo": "riga inventata"}]
}`

// scenaACME scrive due file in t.TempDir() e il manifest che li elenca; restituisce la cartella e i byte del
// manifest.
func scenaACME(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	attesi := []byte("versione_attesi: 1\n")
	nomi := []byte("nome-inventato-acme\n")
	for p, b := range map[string][]byte{"dati/attesi_acme.yaml": attesi, "controllo/nomi_acme.txt": nomi} {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := strings.NewReplacer("{SHA}", sha(attesi), "{BYTE}", itoa(len(attesi)), "{SHA2}", sha(nomi), "{BYTE2}", itoa(len(nomi))).Replace(manifestACME)
	return dir, []byte(m)
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

func TestVersioneManifestFissa(t *testing.T) {
	if VersioneManifest != 1 {
		t.Fatalf("VersioneManifest = %d: cambiarla vuol dire riscrivere questa prova («Riscritta per …»)", VersioneManifest)
	}
}

func TestLeggiManifestValido(t *testing.T) {
	dir, raw := scenaACME(t)
	m, err := Leggi(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Versione != 1 || m.Cartella != dir || len(m.Voci) != 2 || len(m.Storico) != 1 {
		t.Fatalf("manifest letto male: %+v", m)
	}
	if m.Profili["acme"] != uuidACME {
		t.Fatalf("profili (D-09): %v", m.Profili)
	}
	if m.Voci[1].Ruolo != RuoloControllo {
		t.Fatalf("la voce degli elenchi del controllo prima del push ha il ruolo %q", m.Voci[1].Ruolo)
	}
	b, err := m.LeggiFile("attesi")
	if err != nil || string(b) != "versione_attesi: 1\n" {
		t.Fatalf("LeggiFile: %q, %v", b, err)
	}
	if _, err := m.LeggiFile("controllo.nomi"); err != nil {
		t.Fatalf("voce controllo: %v", err)
	}
	if got := m.PercorsoDi(m.Voci[0]); got != filepath.Join(dir, "dati", "attesi_acme.yaml") {
		t.Fatalf("PercorsoDi: %s", got)
	}
}

// TestLeggiManifestStretto: ogni ingresso fuori contratto è un errore, mai un ripiego.
func TestLeggiManifestStretto(t *testing.T) {
	dir, buono := scenaACME(t)
	casi := map[string]string{
		"BOM":                   "\xef\xbb\xbf" + string(buono),
		"UTF-8 non valido":      strings.Replace(string(buono), "riga inventata", "riga \xff", 1),
		"surrogato solo":        strings.Replace(string(buono), "riga inventata", `riga \ud800`, 1),
		"testo dopo l'oggetto":  string(buono) + " {}",
		"chiave sconosciuta":    strings.Replace(string(buono), `"copia": {}`, `"copia": {}, "export": {}`, 1),
		"chiave in una voce":    strings.Replace(string(buono), `"ruolo": "attesi"`, `"ruolo": "attesi", "nota": "x"`, 1),
		"chiave ripetuta":       strings.Replace(string(buono), `"copia": {}`, `"copia": {}, "copia": {}`, 1),
		"maiuscole diverse":     strings.Replace(string(buono), `"voci"`, `"Voci"`, 1),
		"null":                  strings.Replace(string(buono), `"copia": {}`, `"copia": null`, 1),
		"numero con decimali":   strings.Replace(string(buono), `"versione_manifest": 1`, `"versione_manifest": 1.0`, 1),
		"versione 2":            strings.Replace(string(buono), `"versione_manifest": 1`, `"versione_manifest": 2`, 1),
		"versione assente":      strings.Replace(string(buono), `"versione_manifest": 1,`, ``, 1),
		"byte assente":          strings.Replace(string(buono), `, "byte": 19}`, `}`, 1),
		"ruolo fuori elenco":    strings.Replace(string(buono), `"ruolo": "attesi"`, `"ruolo": "segreto"`, 1),
		"sha256 corto":          strings.Replace(string(buono), `"sha256": "`, `"sha256": "ab`, 1),
		"percorso assoluto":     strings.Replace(string(buono), `"percorso": "dati/attesi_acme.yaml"`, `"percorso": "/dati/attesi_acme.yaml"`, 1),
		"percorso con l'unità":  strings.Replace(string(buono), `"percorso": "dati/attesi_acme.yaml"`, `"percorso": "C:\\dati\\attesi_acme.yaml"`, 1),
		"voce ripetuta":         strings.Replace(string(buono), `"nome": "controllo.nomi"`, `"nome": "attesi"`, 1),
		"byte negativi":         strings.Replace(string(buono), `"byte": 19}`, `"byte": -1}`, 1),
		"profilo senza cliente": strings.Replace(string(buono), `"acme": "`+uuidACME+`"`, `"acme": ""`, 1),
		"tipo sbagliato":        strings.Replace(string(buono), `"versione_manifest": 1`, `"versione_manifest": "1"`, 1),
		"non un oggetto":        `[]`,
	}
	for nome, raw := range casi {
		t.Run(nome, func(t *testing.T) {
			if raw == string(buono) {
				t.Fatal("la sostituzione non ha cambiato niente: il caso non prova nulla")
			}
			if _, err := Leggi([]byte(raw), dir); err == nil {
				t.Fatal("manifest accettato")
			}
		})
	}
}

// TestLeggiFileErroriTipizzati: un file mancante o cambiato è un errore tipizzato, e chi lo riceve risulta
// NON ESEGUITO (R44): mai un salto, mai un file dato lo stesso.
func TestLeggiFileErroriTipizzati(t *testing.T) {
	dir, raw := scenaACME(t)
	m, err := Leggi(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.LeggiFile("regole.indice"); !errors.Is(err, ErrFileMancante) {
		t.Fatalf("voce assente dal manifest: %v", err)
	}

	p := filepath.Join(dir, "dati", "attesi_acme.yaml")
	if err := os.WriteFile(p, []byte("versione_attesi: 2\n"), 0o644); err != nil { // stessi byte, altro sha256
		t.Fatal(err)
	}
	if b, err := m.LeggiFile("attesi"); !errors.Is(err, ErrImprontaDiversa) || b != nil {
		t.Fatalf("sha256 cambiato: %q, %v", b, err)
	}
	if err := os.WriteFile(p, []byte("versione_attesi: 1\n\n"), 0o644); err != nil { // un byte in più
		t.Fatal(err)
	}
	if _, err := m.LeggiFile("attesi"); !errors.Is(err, ErrImprontaDiversa) {
		t.Fatalf("byte cambiati: %v", err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := m.LeggiFile("attesi"); !errors.Is(err, ErrFileMancante) {
		t.Fatalf("file tolto: %v", err)
	}
}

// TestShaConLeMaiuscole: le cifre esadecimali in maiuscolo sono lo stesso sha256.
func TestShaConLeMaiuscole(t *testing.T) {
	dir, raw := scenaACME(t)
	attesi := []byte("versione_attesi: 1\n")
	raw = []byte(strings.ReplaceAll(string(raw), sha(attesi), strings.ToUpper(sha(attesi))))
	m, err := Leggi(raw, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.LeggiFile("attesi"); err != nil {
		t.Fatal(err)
	}
}

// TestFuoriDalModulo: dentro l'albero che contiene go.mod no, fuori sì; anche una cartella che non esiste
// ancora, come quella dei rapporti.
func TestFuoriDalModulo(t *testing.T) {
	if err := FuoriDalModulo("."); err == nil {
		t.Fatal("la cartella del pacchetto sta nel modulo: andava rifiutata")
	}
	if err := FuoriDalModulo(filepath.Join("testdata", "non-esiste", "rapporti")); err == nil {
		t.Fatal("una cartella che non esiste dentro il modulo andava rifiutata")
	}
	if err := FuoriDalModulo(filepath.Join(t.TempDir(), "banco", "nuova")); err != nil {
		t.Fatalf("una cartella fuori dal modulo: %v", err)
	}
	if err := FuoriDalModulo(""); err == nil {
		t.Fatal("percorso vuoto accettato")
	}
	// Un modulo qualunque, non solo questo: un go.mod in una cartella sopra basta.
	altro := t.TempDir()
	if err := os.WriteFile(filepath.Join(altro, "go.mod"), []byte("module acme.example/altro\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := FuoriDalModulo(filepath.Join(altro, "dentro")); err == nil {
		t.Fatal("una cartella sotto un altro go.mod andava rifiutata")
	}
}
