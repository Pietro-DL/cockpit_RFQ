package runtime

// L1 — giro 5, A1d: l'avvio del motore A (piano 7.4.5; BE-16): spento senza file aperti, le regole ACME
// (testdata/motore_a), indice rotto, grammatica o casi cambiati, file fuori dalla cartella. Il cliente è ACME, inventato.

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/transport/web"
)

// I tre file della cartella ACME (T-A1d-33), come li nomina l'indice, e il cliente inventato della grammatica.
const (
	indiceACME     = "indice_acme.v1.json"
	grammaticaACME = "acme.v1.json"
	casiACME       = "casi_acme.v1.json"
)

var clienteACME = uuid.MustParse("00000000-0000-4000-8000-00000000ac01")

// improntaIndiceACME: l'impronta canonica dell'indice ACME (piano A1d §4.1). È del canonico, quindi non dipende dagli
// spazi del file né dai fine riga.
const improntaIndiceACME = "847f28788ff8e3723974d91ebb687e3222edb008f95306dc0fc71f6cd0e9d886"

// leggiACME: un file di testdata/motore_a con i CRLF portati a LF. Un checkout con core.autocrlf li scriverebbe CRLF, e
// gli sha256 dell'indice sono quelli dei file LF (T-A1d-33).
func leggiACME(t *testing.T, nome string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "motore_a", nome))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// regoleACME copia i tre file ACME in una cartella nuova, <t.TempDir()>/motore_a, con i fine riga LF, e restituisce il
// percorso dell'indice. La cartella madre resta della prova: ci vanno i file «fuori».
func regoleACME(t *testing.T) string {
	t.Helper()
	cartella := filepath.Join(t.TempDir(), "motore_a")
	if err := os.Mkdir(cartella, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, nome := range []string{indiceACME, grammaticaACME, casiACME} {
		scriviFile(t, filepath.Join(cartella, nome), leggiACME(t, nome))
	}
	return filepath.Join(cartella, indiceACME)
}

// registro: un log testuale in memoria, a livello Debug, per leggere le righe dell'avvio.
func registro(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

func scriviFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// spazioInCoda: uno spazio in fondo al file. Il JSON non cambia, lo sha256 sì.
func spazioInCoda(t *testing.T, p string) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	scriviFile(t, p, append(b, ' '))
}

// cambiaIndice riscrive l'indice dopo il cambio fatto da f sul suo JSON (i numeri restano interi: UseNumber).
func cambiaIndice(t *testing.T, indice string, f func(m map[string]any)) {
	t.Helper()
	b, err := os.ReadFile(indice)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	f(m)
	nuovo, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	scriviFile(t, indice, nuovo)
}

// oggetto: un oggetto JSON dentro l'indice decodificato, o la prova si ferma.
func oggetto(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("nell'indice ACME atteso un oggetto, trovato %T", v)
	}
	return m
}

// primaGrammatica: la prima voce di grammatiche dell'indice decodificato.
func primaGrammatica(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	g, ok := m["grammatiche"].([]any)
	if !ok || len(g) == 0 {
		t.Fatalf("l'indice ACME senza grammatiche: %v", m["grammatiche"])
	}
	return oggetto(t, g[0])
}

// righe: le righe del log con quel livello (DEBUG, INFO, WARN, ERROR).
func righe(buf *bytes.Buffer, livello string) []string {
	var out []string
	for _, r := range strings.Split(buf.String(), "\n") {
		if strings.Contains(r, "level="+livello+" ") {
			out = append(out, r)
		}
	}
	return out
}

// contiene: almeno una delle righe contiene tutte le parti.
func contiene(rr []string, parti ...string) bool {
	for _, r := range rr {
		tutte := true
		for _, p := range parti {
			tutte = tutte && strings.Contains(r, p)
		}
		if tutte {
			return true
		}
	}
	return false
}

// primoCodice: il codice della prima diagnostica di un cliente scartato, "" se non è scartato.
func primoCodice(a web.AnteprimaMotoreA, cliente uuid.UUID) string {
	if a.Regole == nil || len(a.Regole.Scartati[cliente]) == 0 {
		return ""
	}
	return a.Regole.Scartati[cliente][0].Codice
}

// La guardia delle fixture: gli sha256 dell'indice ACME sono quelli della grammatica e del file dei casi, con i fine
// riga LF (T-A1d-33). Se cade, è cambiato un file della cartella e non l'indice.
func TestMotoreALeRegoleACMEDelleProveSonoCoerenti(t *testing.T) {
	ind, err := grammatica.LeggiIndice(leggiACME(t, indiceACME))
	if err != nil {
		t.Fatalf("l'indice ACME non si legge: %v", err)
	}
	if len(ind.Grammatiche) != 1 || ind.Casi == nil {
		t.Fatalf("l'indice ACME ha %d grammatiche e casi %v: attese una grammatica e il file dei casi", len(ind.Grammatiche), ind.Casi)
	}
	g := ind.Grammatiche[0]
	if g.ClienteID != clienteACME || g.File != grammaticaACME || ind.Casi.File != casiACME {
		t.Errorf("l'indice ACME nomina %s %q e %q: attesi %s %q e %q", g.ClienteID, g.File, ind.Casi.File, clienteACME, grammaticaACME, casiACME)
	}
	for _, x := range []struct{ voce, file, sha string }{
		{"grammatiche[0]", grammaticaACME, g.Sha256},
		{"casi", casiACME, ind.Casi.Sha256},
	} {
		if vero := jsoncanonico.Impronta(leggiACME(t, x.file)); vero != x.sha {
			t.Errorf("%s: lo sha256 dell'indice è %s, quello di %s (LF) è %s", x.voce, x.sha, x.file, vero)
		}
	}
}

// Spenta, preparaMotoreA non apre niente: con un indice che non c'è resta il valore zero, una riga «spenta» e nessun
// avviso né errore. Cade se l'indice si legge prima del controllo del flag (M-A1d-22).
func TestMotoreASpentoNonApreNiente(t *testing.T) {
	log, buf := registro(t)
	indice := filepath.Join(t.TempDir(), "non_c_e", "indice.json")
	a := preparaMotoreA(config.MotoreA{Anteprima: false, Regole: indice}, log)
	if a.Accesa || a.Regole != nil || a.Errore != "" || a.Limite != 0 || a.Calcola != nil ||
		a.Ingressi.Versione != 0 || a.Ingressi.Casi != nil {
		t.Errorf("spenta: %+v, atteso il valore zero", a)
	}
	if !strings.Contains(buf.String(), "anteprima motore A spenta") {
		t.Errorf("manca la riga «anteprima motore A spenta»:\n%s", buf)
	}
	if w, e := righe(buf, "WARN"), righe(buf, "ERROR"); len(w)+len(e) > 0 {
		t.Errorf("spenta, il log ha avvisi o errori:\n%s", buf)
	}
}

// Le regole ACME: un motore per il cliente, nessuno scartato, l'impronta canonica dell'indice, il limite di 20 s
// (tempo_massimo_ms 20000), i due casi e la riga dell'avvio. Cade con il limite in secondi (M-A1d-28).
func TestMotoreAConLeRegoleACME(t *testing.T) {
	log, buf := registro(t)
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: regoleACME(t)}, log)
	if !a.Accesa || a.Errore != "" || a.Regole == nil {
		t.Fatalf("le regole ACME non si caricano: Accesa %v, Errore %q, Regole %v; log:\n%s", a.Accesa, a.Errore, a.Regole, buf)
	}
	if len(a.Regole.Motori) != 1 || a.Regole.Motori[clienteACME] == nil {
		t.Errorf("motori %d, attivo il cliente ACME %v: atteso solo lui", len(a.Regole.Motori), a.Regole.Motori[clienteACME] != nil)
	}
	if len(a.Regole.Scartati) != 0 {
		t.Errorf("clienti scartati: %v", a.Regole.Scartati)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a.Regole.ImprontaIndice) || a.Regole.ImprontaIndice != improntaIndiceACME {
		t.Errorf("impronta dell'indice %q, attesa %s", a.Regole.ImprontaIndice, improntaIndiceACME)
	}
	if a.Limite != 20*time.Second {
		t.Errorf("Limite = %v, atteso 20s (limiti.anteprima.tempo_massimo_ms = 20000)", a.Limite)
	}
	var casi []string
	for _, c := range a.Ingressi.Casi {
		casi = append(casi, c.ID)
	}
	if !slices.Equal(casi, []string{"ACME-SCENARIO", "ACME-ELENCO"}) {
		t.Errorf("casi letti %v, attesi ACME-SCENARIO e ACME-ELENCO", casi)
	}
	accesa := righe(buf, "INFO")
	for _, parte := range []string{"anteprima motore A accesa", "versione_limiti=limiti-acme-1",
		"casi=" + jsoncanonico.Impronta(leggiACME(t, casiACME)), "clienti_attivi=1", "clienti_scartati=0",
		"impronta_indice=" + improntaIndiceACME, "limite=20s", "solo al ruolo admin"} {
		if !contiene(accesa, "anteprima motore A accesa", parte) {
			t.Errorf("la riga dell'avvio non dice %q:\n%s", parte, buf)
		}
	}
	if w, e := righe(buf, "WARN"), righe(buf, "ERROR"); len(w)+len(e) > 0 {
		t.Errorf("con le regole ACME il log ha avvisi o errori:\n%s", buf)
	}
}

// Un indice che non si legge, non è JSON o ha i limiti sbagliati non ferma il Cockpit: l'anteprima resta accesa senza
// regole, la pagina ha il testo generico, il log il dettaglio. Cade se l'indice rotto spegne l'anteprima (M-A1d-29).
func TestMotoreAIndiceRottoNonFermaIlCockpit(t *testing.T) {
	for _, x := range []struct {
		nome  string
		rompi func(t *testing.T, indice string)
	}{
		{"indice assente", func(t *testing.T, indice string) {
			if err := os.Remove(indice); err != nil {
				t.Fatal(err)
			}
		}},
		{"JSON rotto", func(t *testing.T, indice string) {
			scriviFile(t, indice, []byte(`{"versione_indice": 1, "limiti": {`))
		}},
		{"limiti.anteprima tolta", func(t *testing.T, indice string) {
			cambiaIndice(t, indice, func(m map[string]any) { delete(oggetto(t, m["limiti"]), "anteprima") })
		}},
		{"tempo_massimo_ms = 60001", func(t *testing.T, indice string) {
			cambiaIndice(t, indice, func(m map[string]any) {
				oggetto(t, oggetto(t, m["limiti"])["anteprima"])["tempo_massimo_ms"] = json.Number("60001")
			})
		}},
	} {
		t.Run(x.nome, func(t *testing.T) {
			log, buf := registro(t)
			indice := regoleACME(t)
			x.rompi(t, indice)
			a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
			if !a.Accesa || a.Regole != nil || a.Errore != web.ErroreRegoleMotoreA {
				t.Errorf("Accesa %v, Regole %v, Errore %q: attese l'anteprima accesa, nessuna regola e il testo generico", a.Accesa, a.Regole, a.Errore)
			}
			if !contiene(righe(buf, "ERROR"), "indice=") {
				t.Errorf("manca la riga ERROR con l'indice:\n%s", buf)
			}
		})
	}
}

// Una grammatica cambiata dopo l'indice scarta il suo cliente con regole.sha256_discorde: lo sha256 delle grammatiche
// resta quello di CompilaInsieme, e il log lo dice con l'UUID.
func TestMotoreAGrammaticaCambiataScartaIlCliente(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	spazioInCoda(t, filepath.Join(filepath.Dir(indice), grammaticaACME))
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if !a.Accesa || a.Errore != "" || a.Regole == nil {
		t.Fatalf("Accesa %v, Errore %q, Regole %v: un cliente scartato non toglie le regole; log:\n%s", a.Accesa, a.Errore, a.Regole, buf)
	}
	if len(a.Regole.Motori) != 0 {
		t.Errorf("motori %d: il cliente con la grammatica cambiata doveva essere scartato", len(a.Regole.Motori))
	}
	if c := primoCodice(a, clienteACME); c != motorea.CodiceRegoleSha256Discorde {
		t.Errorf("scarto del cliente ACME: %q, atteso %s", c, motorea.CodiceRegoleSha256Discorde)
	}
	if !contiene(righe(buf, "INFO"), "anteprima motore A accesa", "clienti_scartati=1", clienteACME.String()) {
		t.Errorf("la riga dell'avvio non dice il cliente scartato:\n%s", buf)
	}
}

// Un file dei casi cambiato dopo l'indice non si legge: le regole restano, gli ingressi sono vuoti, un avviso. Cade
// senza il controllo dello sha256 dei casi (M-A1d-13).
func TestMotoreACasiCambiatiDannoIngressiVuoti(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	spazioInCoda(t, filepath.Join(filepath.Dir(indice), casiACME))
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if a.Regole == nil || len(a.Regole.Motori) != 1 {
		t.Fatalf("i casi cambiati hanno tolto le regole; log:\n%s", buf)
	}
	if len(a.Ingressi.Casi) != 0 {
		t.Errorf("letti %d casi da un file con lo sha256 diverso dall'indice", len(a.Ingressi.Casi))
	}
	if !contiene(righe(buf, "WARN"), "file dei casi") {
		t.Errorf("manca l'avviso sul file dei casi:\n%s", buf)
	}
	if !contiene(righe(buf, "INFO"), "anteprima motore A accesa", `casi="non letto"`) {
		t.Errorf("la riga dell'avvio non dice casi non letto:\n%s", buf)
	}
}

// Un indice senza il file dei casi è ammesso: ingressi vuoti, nessun avviso, «casi=nessuno».
func TestMotoreACasiNonDichiaratiNessunIngresso(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	cambiaIndice(t, indice, func(m map[string]any) { delete(m, "casi") })
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if a.Regole == nil || len(a.Regole.Motori) != 1 || a.Errore != "" {
		t.Fatalf("senza casi le regole non si caricano; log:\n%s", buf)
	}
	if len(a.Ingressi.Casi) != 0 {
		t.Errorf("letti %d casi da un indice che non ne dichiara", len(a.Ingressi.Casi))
	}
	if w, e := righe(buf, "WARN"), righe(buf, "ERROR"); len(w)+len(e) > 0 {
		t.Errorf("senza casi il log ha avvisi o errori:\n%s", buf)
	}
	if !contiene(righe(buf, "INFO"), "anteprima motore A accesa", "casi=nessuno") {
		t.Errorf("la riga dell'avvio non dice casi=nessuno:\n%s", buf)
	}
}

// I file dell'indice si leggono solo dentro la sua cartella (R55): un nome che ne esce, anche con le barre rovesciate,
// non si legge e il log lo dice; un collegamento simbolico dentro la cartella verso un file fuori non si apre (os.Root).
// Cade con la lettura diretta, os.ReadFile(filepath.Join(cartella, v.File)), al posto di nomeLocale e os.Root (M-A1d-14).
func TestMotoreAFileFuoriDallaCartellaNonSiLegge(t *testing.T) {
	t.Run("grammatica ../acme.v1.json", func(t *testing.T) {
		grammaticaFuori(t, "../"+grammaticaACME)
	})
	t.Run(`grammatica ..\acme.v1.json`, func(t *testing.T) {
		grammaticaFuori(t, `..\`+grammaticaACME)
	})
	t.Run("casi ../casi_acme.v1.json", func(t *testing.T) {
		log, buf := registro(t)
		indice := regoleACME(t)
		madre := filepath.Dir(filepath.Dir(indice))
		scriviFile(t, filepath.Join(madre, casiACME), leggiACME(t, casiACME))
		cambiaIndice(t, indice, func(m map[string]any) { oggetto(t, m["casi"])["file"] = "../" + casiACME })
		a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
		if a.Regole == nil || len(a.Regole.Motori) != 1 {
			t.Fatalf("i casi fuori dalla cartella hanno tolto le regole; log:\n%s", buf)
		}
		if len(a.Ingressi.Casi) != 0 {
			t.Errorf("letti %d casi da un file fuori dalla cartella dell'indice", len(a.Ingressi.Casi))
		}
		if !contiene(righe(buf, "WARN"), "file dei casi", "esce dalla cartella") {
			t.Errorf("manca l'avviso sul file dei casi fuori dalla cartella:\n%s", buf)
		}
	})
	t.Run("collegamento simbolico verso fuori", func(t *testing.T) {
		log, buf := registro(t)
		indice := regoleACME(t)
		cartella := filepath.Dir(indice)
		fuori := filepath.Join(filepath.Dir(cartella), "fuori")
		if err := os.Mkdir(fuori, 0o755); err != nil {
			t.Fatal(err)
		}
		bersaglio := filepath.Join(fuori, grammaticaACME)
		scriviFile(t, bersaglio, leggiACME(t, grammaticaACME))
		collegamento := filepath.Join(cartella, grammaticaACME)
		if err := os.Remove(collegamento); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(bersaglio, collegamento); err != nil {
			t.Skip("SALTATO-AMBIENTE: questo sistema non crea collegamenti simbolici:", err)
		}
		a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
		if a.Regole == nil {
			t.Fatalf("regole non caricate; log:\n%s", buf)
		}
		if len(a.Regole.Motori) != 0 {
			t.Errorf("la grammatica è stata letta attraverso un collegamento che esce dalla cartella dell'indice")
		}
		if c := primoCodice(a, clienteACME); c != motorea.CodiceRegoleFileAssente {
			t.Errorf("scarto del cliente ACME: %q, atteso %s", c, motorea.CodiceRegoleFileAssente)
		}
	})
}

// grammaticaFuori: l'indice nomina la grammatica con un nome che esce dalla cartella; il file c'è, nella cartella
// madre, con lo sha256 giusto. Il cliente è scartato con regole.file_assente e il log nomina il file.
func grammaticaFuori(t *testing.T, nome string) {
	t.Helper()
	log, buf := registro(t)
	indice := regoleACME(t)
	madre := filepath.Dir(filepath.Dir(indice))
	scriviFile(t, filepath.Join(madre, grammaticaACME), leggiACME(t, grammaticaACME))
	cambiaIndice(t, indice, func(m map[string]any) { primaGrammatica(t, m)["file"] = nome })
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if a.Regole == nil || a.Errore != "" {
		t.Fatalf("un file fuori dalla cartella toglie le regole: Errore %q; log:\n%s", a.Errore, buf)
	}
	if len(a.Regole.Motori) != 0 {
		t.Errorf("la grammatica %q, fuori dalla cartella dell'indice, è stata letta", nome)
	}
	if c := primoCodice(a, clienteACME); c != motorea.CodiceRegoleFileAssente {
		t.Errorf("scarto del cliente ACME: %q, atteso %s", c, motorea.CodiceRegoleFileAssente)
	}
	if !contiene(righe(buf, "WARN"), "esce dalla sua cartella", "file=") {
		t.Errorf("manca l'avviso sul file che esce dalla cartella:\n%s", buf)
	}
}
