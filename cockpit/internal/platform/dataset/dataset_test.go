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
		"chiave sconosciuta":    strings.Replace(string(buono), `"copia": {}`, `"copia": {}, "sconosciuta": {}`, 1),
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

// L1 — le sezioni di A1c (A1c-L1-29; piano A, 6.4.8; R33 d): copia_run, con la forma di copia, ed export, con la
// terna e lo schema degli export. Si leggono con i loro campi; la lettura resta stretta. Valori inventati: i veri
// stanno solo nel manifest privato.
func TestLeggiCopiaRunEExport(t *testing.T) {
	dir, buono := scenaACME(t)
	hash := strings.Repeat("ab", 32)
	sezioni := `"copia": {"database": "acme_copia", "ruolo": "lettore_acme", "schema": 21, "escluse": ["tabella_esclusa"],
    "sentinelle": {"componente": 3}},
  "copia_run": {"database": "acme_copia_run", "ruolo": "prove_acme", "schema": 21, "sentinelle": {"componente": 3, "allegato": 5}},
  "export": {"versione": 4, "hash_configurazione": "` + hash + `", "schema": 21}`
	raw := strings.Replace(string(buono), `"copia": {}`, sezioni, 1)
	m, err := Leggi([]byte(raw), dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Copia.Database != "acme_copia" || m.Copia.Ruolo != "lettore_acme" || len(m.Copia.Escluse) != 1 {
		t.Errorf("copia: %+v", m.Copia)
	}
	if m.CopiaRun.Database != "acme_copia_run" || m.CopiaRun.Ruolo != "prove_acme" || m.CopiaRun.Schema != 21 ||
		m.CopiaRun.Sentinelle["allegato"] != 5 || len(m.CopiaRun.Sentinelle) != 2 {
		t.Errorf("copia_run: %+v", m.CopiaRun)
	}
	if m.Export == nil || *m.Export != (ExportDichiarato{Versione: 4, HashConfigurazione: hash, Schema: 21}) {
		t.Errorf("export: %+v", m.Export)
	}

	// senza le sezioni: copia_run vuota, export assente (nil, mai una terna inventata)
	m, err = Leggi(buono, dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Export != nil || m.CopiaRun.Database != "" || len(m.CopiaRun.Sentinelle) != 0 {
		t.Errorf("senza le sezioni di A1c: export %+v, copia_run %+v", m.Export, m.CopiaRun)
	}

	casi := map[string]string{
		"export: chiave sconosciuta":     strings.Replace(raw, `"schema": 21}`, `"schema": 21, "data": "x"}`, 1),
		"export: senza la versione":      strings.Replace(raw, `"versione": 4, `, ``, 1),
		"export: senza l'hash":           strings.Replace(raw, `"hash_configurazione": "`+hash+`", `, ``, 1),
		"export: senza lo schema":        strings.Replace(raw, `, "schema": 21}`, `}`, 1),
		"export: versione zero":          strings.Replace(raw, `"versione": 4`, `"versione": 0`, 1),
		"export: versione con decimali":  strings.Replace(raw, `"versione": 4`, `"versione": 4.0`, 1),
		"export: hash corto":             strings.Replace(raw, `"hash_configurazione": "`+hash, `"hash_configurazione": "`+hash[:10], 1),
		"export: schema zero":            strings.Replace(raw, `"hash_configurazione": "`+hash+`", "schema": 21`, `"hash_configurazione": "`+hash+`", "schema": 0`, 1),
		"export: null":                   strings.Replace(raw, `"export": {"versione": 4, "hash_configurazione": "`+hash+`", "schema": 21}`, `"export": null`, 1),
		"export: nidificata nella terna": strings.Replace(raw, `"export": {"versione": 4, `, `"export": {"analizzatore": {"versione": 4}, `, 1),
		"copia_run: chiave sconosciuta":  strings.Replace(raw, `"ruolo": "prove_acme"`, `"ruolo": "prove_acme", "porta": 5432`, 1),
		"copia_run: maiuscole diverse":   strings.Replace(raw, `"copia_run"`, `"Copia_Run"`, 1),
		"copia_run: ripetuta":            strings.Replace(raw, `"export": {`, `"copia_run": {}, "export": {`, 1),
	}
	for nome, r := range casi {
		t.Run(nome, func(t *testing.T) {
			if r == raw {
				t.Fatal("la sostituzione non ha cambiato niente: il caso non prova nulla")
			}
			if _, err := Leggi([]byte(r), dir); err == nil {
				t.Fatal("manifest accettato")
			}
		})
	}
}

// L1 — le impronte della copia intatta (R117 b, ratificata e ampliata; E2 §2.10): la sezione impronte di copia si
// legge con la versione della formula e, per tabella, colonne, ordine e sha256; è facoltativa (senza, nil). La
// lettura resta stretta: chiavi sconosciute, assenti, ripetute o null rifiutate. Nella copia_run la chiave impronte
// è sconosciuta: ogni ambiente ha le sue condizioni attese, e quelle della copia _run non sono ancora previste. Il
// contenuto (versione, nomi, sha256) lo controlla platform/testutil, che calcola le impronte. Valori inventati.
func TestLeggiImpronteDellaCopia(t *testing.T) {
	dir, buono := scenaACME(t)
	hash := strings.Repeat("cd", 32)
	impronte := `"impronte": {"versione_impronta": 1, "tabelle": {"componente": {"colonne": ["id", "codice"], "ordine": ["id"], "sha256": "` + hash + `"}}}`
	raw := strings.Replace(string(buono), `"copia": {}`, `"copia": {"database": "acme_copia", "ruolo": "lettore_acme", "schema": 21,
    "sentinelle": {"componente": 3}, `+impronte+`},
  "copia_run": {"database": "acme_copia_run", "ruolo": "prove_acme", "schema": 21, "sentinelle": {"componente": 3}}`, 1)
	m, err := Leggi([]byte(raw), dir)
	if err != nil {
		t.Fatal(err)
	}
	imp := m.Copia.Impronte
	if imp == nil || imp.Versione != 1 || len(imp.Tabelle) != 1 {
		t.Fatalf("impronte: %+v", imp)
	}
	c := imp.Tabelle["componente"]
	if strings.Join(c.Colonne, ",") != "id,codice" || strings.Join(c.Ordine, ",") != "id" || c.Sha256 != hash {
		t.Errorf("l'impronta di componente: %+v", c)
	}
	if m.CopiaRun.Impronte != nil {
		t.Errorf("la copia_run ha delle impronte: %+v", m.CopiaRun.Impronte)
	}

	// senza la sezione: nil, mai delle impronte inventate
	senza := strings.Replace(raw, `, `+impronte, ``, 1)
	if senza == raw {
		t.Fatal("la sostituzione non ha tolto le impronte")
	}
	if m, err = Leggi([]byte(senza), dir); err != nil || m.Copia.Impronte != nil {
		t.Fatalf("senza impronte: %+v, %v", m.Copia.Impronte, err)
	}

	casi := map[string]string{
		"impronte: chiave sconosciuta":          strings.Replace(raw, `"versione_impronta": 1,`, `"versione_impronta": 1, "data": "x",`, 1),
		"impronte: senza la versione":           strings.Replace(raw, `"versione_impronta": 1, `, ``, 1),
		"impronte: senza le tabelle":            strings.Replace(raw, `, "tabelle": {"componente": {"colonne": ["id", "codice"], "ordine": ["id"], "sha256": "`+hash+`"}}`, ``, 1),
		"impronte: null":                        strings.Replace(raw, impronte, `"impronte": null`, 1),
		"impronte: versione con decimali":       strings.Replace(raw, `"versione_impronta": 1`, `"versione_impronta": 1.0`, 1),
		"impronte: versione come testo":         strings.Replace(raw, `"versione_impronta": 1`, `"versione_impronta": "1"`, 1),
		"impronte: ripetute":                    strings.Replace(raw, impronte, impronte+`, `+impronte, 1),
		"impronte: maiuscole diverse":           strings.Replace(raw, `"impronte"`, `"Impronte"`, 1),
		"tabella: chiave sconosciuta":           strings.Replace(raw, `"ordine": ["id"],`, `"ordine": ["id"], "righe": 3,`, 1),
		"tabella: senza le colonne":             strings.Replace(raw, `"colonne": ["id", "codice"], `, ``, 1),
		"tabella: senza l'ordine":               strings.Replace(raw, `"ordine": ["id"], `, ``, 1),
		"tabella: senza lo sha256":              strings.Replace(raw, `, "sha256": "`+hash+`"`, ``, 1),
		"tabella: colonne non un array":         strings.Replace(raw, `"colonne": ["id", "codice"]`, `"colonne": "id, codice"`, 1),
		"tabella: una colonna null":             strings.Replace(raw, `"colonne": ["id", "codice"]`, `"colonne": ["id", null]`, 1),
		"tabella: un valore semplice":           strings.Replace(raw, `"componente": {"colonne": ["id", "codice"], "ordine": ["id"], "sha256": "`+hash+`"}`, `"componente": "`+hash+`"`, 1),
		"copia_run: le impronte non si leggono": strings.Replace(raw, `"copia_run": {"database": "acme_copia_run",`, `"copia_run": {`+impronte+`, "database": "acme_copia_run",`, 1),
	}
	for nome, r := range casi {
		t.Run(nome, func(t *testing.T) {
			if r == raw {
				t.Fatal("la sostituzione non ha cambiato niente: il caso non prova nulla")
			}
			if _, err := Leggi([]byte(r), dir); err == nil {
				t.Fatal("manifest accettato")
			}
		})
	}
}
