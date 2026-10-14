package runtime

// L1 — giro 5, A1d-Q1: l'avvio della Distinta nuova del motore A (piano 7.4.5; direttiva A1d §2.2; Q-A1d-03 = A [U]):
// spenta senza file aperti, le regole ACME (testdata/motore_a), indice assente o rifiutato, nessuna grammatica attiva,
// casi non dichiarati, cambiati, non validi o fuori dalla cartella. Il cliente è ACME, inventato. Derivato dalle prove
// della PR #18 di Stefano Valo (337364a), riscritto sui tipi di transport/web/motore_a.

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/jsoncanonico"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
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
func primoCodice(a motorea_web.Regole, cliente uuid.UUID) string {
	if a.Insieme == nil || len(a.Insieme.Scartati[cliente]) == 0 {
		return ""
	}
	return a.Insieme.Scartati[cliente][0].Codice
}

// erroreSenzaDettagli: il testo per la pagina c'è e non porta percorsi né nomi di file (il dettaglio è del log).
func erroreSenzaDettagli(t *testing.T, a motorea_web.Regole, indice string) {
	t.Helper()
	e := a.Errore()
	if e == "" {
		t.Errorf("stato %s: atteso un testo d'errore per la pagina, vuoto", a.Stato)
	}
	for _, vietato := range []string{indice, filepath.Dir(indice), ".json", `\`, "/"} {
		if strings.Contains(e, vietato) {
			t.Errorf("il testo per la pagina %q contiene %q", e, vietato)
		}
	}
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

// Spenta, preparaMotoreA non apre niente: con un indice che non c'è restano StatoSpenta e i valori zero, una riga
// «spenta» e nessun avviso né errore. Cade se l'indice si legge prima del controllo del flag (M-A1d-22).
func TestMotoreASpentoNonApreNiente(t *testing.T) {
	log, buf := registro(t)
	indice := filepath.Join(t.TempDir(), "non_c_e", "indice.json")
	a := preparaMotoreA(config.MotoreA{Anteprima: false, Regole: indice}, log)
	if a.Accesa || a.Stato != motorea_web.StatoSpenta || a.Insieme != nil || a.Limite != 0 || a.Indice != "" ||
		a.Ingressi.Versione != 0 || a.Ingressi.Casi != nil || a.Pronta() || a.Errore() != "" {
		t.Errorf("spenta: %+v, attesi StatoSpenta e i valori zero", a)
	}
	if !strings.Contains(buf.String(), "anteprima motore A spenta") {
		t.Errorf("manca la riga «anteprima motore A spenta»:\n%s", buf)
	}
	if w, e := righe(buf, "WARN"), righe(buf, "ERROR"); len(w)+len(e) > 0 {
		t.Errorf("spenta, il log ha avvisi o errori:\n%s", buf)
	}
}

// Le regole ACME: pronte, un motore per il cliente, nessuno scartato, le due impronte dell'indice (canonica e grezza),
// le versioni, il limite di 20 s, i due casi letti e la riga dell'avvio con tutti i campi della direttiva §2.2. Cade
// con il limite in secondi (M-A1d-28).
func TestMotoreAConLeRegoleACME(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if !a.Pronta() || a.Stato != motorea_web.StatoPronta || a.Errore() != "" {
		t.Fatalf("le regole ACME non sono pronte: stato %s, errore %q; log:\n%s", a.Stato, a.Errore(), buf)
	}
	if len(a.Insieme.Motori) != 1 || a.Insieme.Motori[clienteACME] == nil || !slices.Equal(a.Attivi, []uuid.UUID{clienteACME}) {
		t.Errorf("motori %d, attivi %v: atteso solo il cliente ACME", len(a.Insieme.Motori), a.Attivi)
	}
	if len(a.Insieme.Scartati) != 0 || len(a.Scartati) != 0 {
		t.Errorf("clienti scartati: %v", a.Scartati)
	}
	if a.Insieme.ImprontaIndice != improntaIndiceACME {
		t.Errorf("impronta dell'indice %q, attesa %s", a.Insieme.ImprontaIndice, improntaIndiceACME)
	}
	grezzo := jsoncanonico.Impronta(leggiACME(t, indiceACME))
	if a.Sha256Indice != grezzo || a.Sha256Indice == a.Insieme.ImprontaIndice {
		t.Errorf("sha256 grezzo dell'indice %q, atteso %s e diverso dall'impronta canonica", a.Sha256Indice, grezzo)
	}
	if a.VersioneIndice != grammatica.VersioneIndice || a.VersioneLimiti != "limiti-acme-1" || a.Indice != indice {
		t.Errorf("versioni %d %q, indice %q", a.VersioneIndice, a.VersioneLimiti, a.Indice)
	}
	if a.Limite != 20*time.Second {
		t.Errorf("Limite = %v, atteso 20s (limiti.anteprima.tempo_massimo_ms = 20000)", a.Limite)
	}
	var casi []string
	for _, c := range a.Ingressi.Casi {
		casi = append(casi, c.ID)
	}
	improntaCasi := jsoncanonico.Impronta(leggiACME(t, casiACME))
	if !slices.Equal(casi, []string{"ACME-SCENARIO", "ACME-ELENCO"}) || a.Casi.Stato != motorea_web.CasiLetti ||
		a.Casi.Numero != 2 || a.Casi.Impronta != improntaCasi || a.Casi.File != casiACME {
		t.Errorf("casi letti %v, diagnosi %+v: attesi ACME-SCENARIO e ACME-ELENCO, letti", casi, a.Casi)
	}
	accesa := righe(buf, "INFO")
	for _, parte := range []string{"stato=pronta", "versione_limiti=limiti-acme-1", "versione_indice=1",
		"casi=letti", "impronta_casi=" + improntaCasi, "numero_casi=2", "clienti_attivi=1", clienteACME.String(),
		"clienti_scartati=0", "impronta_indice=" + improntaIndiceACME, "sha256_indice=" + grezzo, "limite=20s",
		"versione_valutazione=valutazione-5", "solo al ruolo admin"} {
		if !contiene(accesa, "anteprima motore A accesa", parte) {
			t.Errorf("la riga dell'avvio non dice %q:\n%s", parte, buf)
		}
	}
	if w, e := righe(buf, "WARN"), righe(buf, "ERROR"); len(w)+len(e) > 0 {
		t.Errorf("con le regole ACME il log ha avvisi o errori:\n%s", buf)
	}
}

// Un indice assente o rifiutato non ferma il Cockpit (Q-A1d-03 = A [U]): accesa, nessun insieme, lo stato distingue
// assente e non valido, la pagina ha un testo senza percorsi, il log il dettaglio con una riga ERROR. Cade se l'indice
// rotto spegne la Distinta (M-A1d-29) o se assente e rifiutato non si distinguono.
func TestMotoreAIndiceAssenteORifiutatoNonFermaIlCockpit(t *testing.T) {
	for _, x := range []struct {
		nome  string
		stato motorea_web.Stato
		rompi func(t *testing.T, indice string)
	}{
		{"indice assente", motorea_web.StatoIndiceAssente, func(t *testing.T, indice string) {
			if err := os.Remove(indice); err != nil {
				t.Fatal(err)
			}
		}},
		{"JSON rotto", motorea_web.StatoIndiceNonValido, func(t *testing.T, indice string) {
			scriviFile(t, indice, []byte(`{"versione_indice": 1, "limiti": {`))
		}},
		{"limiti.anteprima tolta", motorea_web.StatoIndiceNonValido, func(t *testing.T, indice string) {
			cambiaIndice(t, indice, func(m map[string]any) { delete(oggetto(t, m["limiti"]), "anteprima") })
		}},
		{"tempo_massimo_ms = 60001", motorea_web.StatoIndiceNonValido, func(t *testing.T, indice string) {
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
			if !a.Accesa || a.Insieme != nil || a.Stato != x.stato || a.Pronta() {
				t.Errorf("Accesa %v, Insieme %v, Stato %s: attesi accesa, nessun insieme e %s", a.Accesa, a.Insieme, a.Stato, x.stato)
			}
			erroreSenzaDettagli(t, a, indice)
			if !contiene(righe(buf, "ERROR"), "indice=", "stato="+string(x.stato)) {
				t.Errorf("manca la riga ERROR con l'indice e lo stato:\n%s", buf)
			}
		})
	}
}

// Una grammatica cambiata dopo l'indice scarta il suo cliente con regole.sha256_discorde. Con l'unico cliente scartato
// l'indice è valido ma nessuna grammatica è attiva: non è un motore funzionante (Q-A1d-03 = A [U]), lo stato è
// StatoNessunaGrammatica, la pagina ha un testo d'errore, il log una riga ERROR con l'UUID e il codice. Cade se un
// indice con zero grammatiche risulta pronto (F-11 della revisione della PR #18).
func TestMotoreAGrammaticaCambiataNessunaGrammaticaAttiva(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	spazioInCoda(t, filepath.Join(filepath.Dir(indice), grammaticaACME))
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if !a.Accesa || a.Insieme == nil || a.Stato != motorea_web.StatoNessunaGrammatica || a.Pronta() {
		t.Fatalf("Accesa %v, Insieme %v, Stato %s: attesi l'insieme per le diagnostiche e StatoNessunaGrammatica; log:\n%s",
			a.Accesa, a.Insieme, a.Stato, buf)
	}
	erroreSenzaDettagli(t, a, indice)
	if len(a.Insieme.Motori) != 0 || len(a.Attivi) != 0 {
		t.Errorf("motori %d: il cliente con la grammatica cambiata doveva essere scartato", len(a.Insieme.Motori))
	}
	if c := primoCodice(a, clienteACME); c != motorea.CodiceRegoleSha256Discorde {
		t.Errorf("scarto del cliente ACME: %q, atteso %s", c, motorea.CodiceRegoleSha256Discorde)
	}
	if len(a.Scartati) != 1 || a.Scartati[0].Cliente != clienteACME || !slices.Contains(a.Scartati[0].Codici, motorea.CodiceRegoleSha256Discorde) {
		t.Errorf("scarti %+v: atteso il cliente ACME con %s", a.Scartati, motorea.CodiceRegoleSha256Discorde)
	}
	if !contiene(righe(buf, "ERROR"), "nessuna grammatica attiva", "stato=nessuna_grammatica", "clienti_scartati=1",
		clienteACME.String()+" "+motorea.CodiceRegoleSha256Discorde) {
		t.Errorf("manca la riga ERROR con il cliente scartato e il codice:\n%s", buf)
	}
	if contiene(righe(buf, "INFO"), "anteprima motore A accesa") {
		t.Errorf("con zero grammatiche attive il log dice «accesa» a livello INFO:\n%s", buf)
	}
}

// Un file dei casi cambiato dopo l'indice non si legge: le regole restano pronte, gli ingressi sono vuoti, la diagnosi
// dice sha256_discorde, un avviso. Cade senza il controllo dello sha256 dei casi (M-A1d-13).
func TestMotoreACasiCambiatiDannoIngressiVuoti(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	spazioInCoda(t, filepath.Join(filepath.Dir(indice), casiACME))
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if !a.Pronta() || len(a.Insieme.Motori) != 1 {
		t.Fatalf("i casi cambiati hanno tolto le regole; log:\n%s", buf)
	}
	if len(a.Ingressi.Casi) != 0 || a.Casi.Stato != motorea_web.CasiSha256Discorde || a.Casi.Impronta != "" {
		t.Errorf("letti %d casi, diagnosi %+v: attesi nessun caso e sha256_discorde", len(a.Ingressi.Casi), a.Casi)
	}
	if !contiene(righe(buf, "WARN"), "file dei casi", "casi=sha256_discorde") {
		t.Errorf("manca l'avviso sul file dei casi:\n%s", buf)
	}
	if !contiene(righe(buf, "INFO"), "anteprima motore A accesa", "casi=sha256_discorde") {
		t.Errorf("la riga dell'avvio non dice casi=sha256_discorde:\n%s", buf)
	}
}

// Un file dei casi con lo sha256 giusto ma che valutazione.LeggiIngressi rifiuta: diagnosi non_validi, ingressi vuoti,
// le regole restano pronte. La diagnosi dei casi è separata da quella delle grammatiche (direttiva §2.2).
func TestMotoreACasiNonValidiDiagnosiSeparata(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	rotti := []byte(`{"versione_casi": 99, "casi": []}`)
	scriviFile(t, filepath.Join(filepath.Dir(indice), casiACME), rotti)
	cambiaIndice(t, indice, func(m map[string]any) { oggetto(t, m["casi"])["sha256"] = jsoncanonico.Impronta(rotti) })
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if !a.Pronta() || len(a.Insieme.Motori) != 1 {
		t.Fatalf("i casi non validi hanno tolto le regole: stato %s; log:\n%s", a.Stato, buf)
	}
	if len(a.Ingressi.Casi) != 0 || a.Casi.Stato != motorea_web.CasiNonValidi || a.Casi.Motivo == "" {
		t.Errorf("diagnosi %+v: attesi nessun caso, non_validi e il motivo", a.Casi)
	}
	if !contiene(righe(buf, "WARN"), "file dei casi", "casi=non_validi") {
		t.Errorf("manca l'avviso sui casi non validi:\n%s", buf)
	}
}

// Un indice senza il file dei casi è ammesso: ingressi vuoti, nessun avviso, «casi=non_dichiarati».
func TestMotoreACasiNonDichiaratiNessunIngresso(t *testing.T) {
	log, buf := registro(t)
	indice := regoleACME(t)
	cambiaIndice(t, indice, func(m map[string]any) { delete(m, "casi") })
	a := preparaMotoreA(config.MotoreA{Anteprima: true, Regole: indice}, log)
	if !a.Pronta() || len(a.Insieme.Motori) != 1 {
		t.Fatalf("senza casi le regole non si caricano; log:\n%s", buf)
	}
	if len(a.Ingressi.Casi) != 0 || a.Casi.Stato != motorea_web.CasiNonDichiarati {
		t.Errorf("letti %d casi, diagnosi %+v, da un indice che non ne dichiara", len(a.Ingressi.Casi), a.Casi)
	}
	if w, e := righe(buf, "WARN"), righe(buf, "ERROR"); len(w)+len(e) > 0 {
		t.Errorf("senza casi il log ha avvisi o errori:\n%s", buf)
	}
	if !contiene(righe(buf, "INFO"), "anteprima motore A accesa", "casi=non_dichiarati") {
		t.Errorf("la riga dell'avvio non dice casi=non_dichiarati:\n%s", buf)
	}
}

// I file dell'indice si leggono solo dentro la sua cartella (R55): un nome che ne esce, anche con le barre rovesciate,
// non si legge e il log lo dice; un collegamento simbolico dentro la cartella verso un file fuori non si apre (os.Root).
// Con l'unica grammatica scartata lo stato è StatoNessunaGrammatica. Cade con la lettura diretta,
// os.ReadFile(filepath.Join(cartella, v.File)), al posto di nomeLocale e os.Root (M-A1d-14).
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
		if !a.Pronta() || len(a.Insieme.Motori) != 1 {
			t.Fatalf("i casi fuori dalla cartella hanno tolto le regole; log:\n%s", buf)
		}
		if len(a.Ingressi.Casi) != 0 || a.Casi.Stato != motorea_web.CasiFuoriCartella {
			t.Errorf("letti %d casi, diagnosi %+v, da un file fuori dalla cartella dell'indice", len(a.Ingressi.Casi), a.Casi)
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
		if a.Insieme == nil || a.Stato != motorea_web.StatoNessunaGrammatica {
			t.Fatalf("stato %s; log:\n%s", a.Stato, buf)
		}
		if len(a.Insieme.Motori) != 0 {
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
	if a.Insieme == nil || a.Stato != motorea_web.StatoNessunaGrammatica {
		t.Fatalf("stato %s; log:\n%s", a.Stato, buf)
	}
	if len(a.Insieme.Motori) != 0 {
		t.Errorf("la grammatica %q, fuori dalla cartella dell'indice, è stata letta", nome)
	}
	if c := primoCodice(a, clienteACME); c != motorea.CodiceRegoleFileAssente {
		t.Errorf("scarto del cliente ACME: %q, atteso %s", c, motorea.CodiceRegoleFileAssente)
	}
	if !contiene(righe(buf, "WARN"), "esce dalla sua cartella", "file=") {
		t.Errorf("manca l'avviso sul file che esce dalla cartella:\n%s", buf)
	}
}

// Il testo per la pagina di ogni stato acceso non pronto c'è; spenta e pronta non hanno errore.
func TestMotoreAErroreDiOgniStato(t *testing.T) {
	for _, st := range []motorea_web.Stato{motorea_web.StatoIndiceAssente, motorea_web.StatoIndiceNonValido, motorea_web.StatoNessunaGrammatica} {
		if (motorea_web.Regole{Accesa: true, Stato: st}).Errore() == "" {
			t.Errorf("stato %s senza testo d'errore", st)
		}
	}
	if e := (motorea_web.Regole{}).Errore(); e != "" {
		t.Errorf("spenta con errore %q", e)
	}
	if e := (motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoPronta}).Errore(); e != "" {
		t.Errorf("pronta con errore %q", e)
	}
}
