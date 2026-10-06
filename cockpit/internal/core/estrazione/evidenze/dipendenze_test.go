package evidenze

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// L1 — il grafo degli import del motore A, letto dai sorgenti (piano A, par.3.1, 3.2, 3.5; R7, R41):
//   - G1: ogni pacchetto del motore A importa solo ciò che la tabella consente, e nessun pacchetto puro
//     raggiunge, nemmeno per transitività sul progetto, un pacchetto vietato (VP);
//   - G2: nei pacchetti puri niente file, rete, caso, goroutine né orologio (VD);
//   - G9: i pacchetti legacy che il motore A attraversa non cambiano import;
//   - MOTORE-SENZA-LLM: nessun riferimento ad analisi_messaggio o all'agente nei sorgenti del motore;
//   - G1, il divieto del legacy (R7, R40 a-b): motorea e grammatica non raggiungono il motore legacy di
//     classificazione né le regole legacy, e motorea non chiama il riconoscitore della minuteria;
//   - la guardia su RifCaso (R47 b): nei file non di prova di motorea nessun accesso al riferimento opaco al
//     caso degli attesi che un esempio può portare;
//   - G3 (R9, R40 e, P-18): la libreria YAML la importa solo il runner del banco, internal/app/bancoa, anche
//     contando gli import delle prove e i file con i tag integrazione, browser e privato;
//   - G4 (P-08, R48 A): gli adattatori di core/estrazione usano di classificazione e lettura solo un elenco
//     chiuso, il taglio con posizioni, EInoltro e le tabelle con origine;
//   - F19 (R52 A): il banco, internal/app/bancoa, usa di core/estrazione solo DaTesto;
//   - A1c-L1-28 (la parte di B1): G1 sul caricatore della fotografia e su platform/migrazioni, G2 anche sul
//     caricatore (il DB sì, l'orologio no), nessuna stringa analisi_messaggio nei sorgenti nuovi né nelle query
//     della fotografia (queries/fotografia.sql).
//
// I file di prova restano fuori: per le prove vale un controllo a parte. Il file si estende: ogni sessione
// aggiunge a pacchettiMotoreA i pacchetti che crea, e qui i suoi controlli (la grammatica che usa della
// foglia solo il vocabolario, il divieto del motore legacy, la libreria YAML solo nel runner).
//
// I clienti di questi test sono inventati: qui non ce n'è nessuno, si leggono solo i sorgenti del modulo.

// modulo: il percorso del modulo, come lo scrive go.mod.
const modulo = "promatec/cockpit"

// regolePacchetto: che cosa un pacchetto del motore A può importare (tabella del par.3.1).
type regolePacchetto struct {
	percorso string   // relativo alla radice del modulo
	puro     bool     // vale VP anche per transitività, e VD come import diretto
	g2       bool     // G2 anche se non è puro: niente orologio, file, rete, caso né goroutine (il caricatore)
	progetto []string // import del progetto consentiti, relativi alla radice del modulo
	esterni  []string // librerie esterne consentite (la libreria standard è consentita salvo VD)
}

// pacchettiMotoreA: i pacchetti del motore A che esistono. Un pacchetto nuovo entra qui nel commit «le
// prove» della sessione che lo crea; un pacchetto elencato che non c'è fa fallire la prova.
var pacchettiMotoreA = []regolePacchetto{
	{percorso: "internal/platform/jsoncanonico", puro: true},
	{percorso: "internal/core/estrazione/evidenze", puro: true, esterni: []string{"github.com/google/uuid"}},
	// La grammatica: della foglia solo il vocabolario e le diagnostiche (G8, qui sotto); mai il legacy
	// core/registro/regole, che sta nella cartella sopra ma non si importa (R40 a).
	{percorso: "internal/core/registro/regole/grammatica", puro: true,
		progetto: []string{"internal/core/estrazione/evidenze", "internal/platform/jsoncanonico"},
		esterni:  []string{"github.com/google/uuid"}},
	// Il motore A: le grammatiche, il vocabolario e le diagnostiche della foglia, il JSON canonico. Mai il
	// motore legacy di classificazione, che sta nella cartella sopra ma non si importa (R40 b; R7, qui sotto).
	{percorso: "internal/core/inbox/classificazione/motorea", puro: true,
		progetto: []string{"internal/core/registro/regole/grammatica", "internal/core/estrazione/evidenze",
			"internal/platform/jsoncanonico"},
		esterni: []string{"github.com/google/uuid"}},
	// I record della fotografia (A1b, -> R40 d) e, da A1c, la fotografia intera: tipi puri, senza DB; del
	// progetto solo le foglie (evidenze per le diagnostiche della fotografia, jsoncanonico per le impronte).
	{percorso: "internal/core/fotorfq", puro: true,
		progetto: []string{"internal/core/estrazione/evidenze", "internal/platform/jsoncanonico"},
		esterni:  []string{"github.com/google/uuid"}},
	// Il caricatore (A1c; F7-F10 del par.3.2.1): l'unico pacchetto del motore A con il DB, in sola lettura. Non è
	// puro, perché legge il database; ma niente orologio, file, rete, caso né goroutine (G2: l'ora è SELECT now()
	// del DB). Del progetto solo la fotografia, le diagnostiche, le query sqlc e le migrazioni (lo schema sulla
	// transazione); mai un altro core/*, mai ai, app, transport, la libreria YAML.
	{percorso: "internal/core/fotorfq/caricatore", g2: true,
		progetto: []string{"internal/core/fotorfq", "internal/core/estrazione/evidenze", "internal/platform/db",
			"internal/platform/migrazioni"},
		esterni: []string{"github.com/google/uuid", "github.com/jackc/pgx/v5", "github.com/jackc/pgx/v5/pgtype"}},
	// L'apertura in sola lettura (A1c, P-02): platform/migrazioni legge il DB, quindi non è pura; non importa
	// niente del progetto (anche G9) e delle librerie esterne solo pgx.
	{percorso: "internal/platform/migrazioni",
		esterni: []string{"github.com/jackc/pgx/v5", "github.com/jackc/pgx/v5/pgxpool"}},
	// Gli adattatori (A1b): il documento della foglia, i record della fotografia, i fatti del worker, il JSON
	// canonico per il BundleID; del legacy solo il taglio con posizioni ed EInoltro (classificazione) e le
	// tabelle con origine (lettura), con l'elenco chiuso di G4 (qui sotto). Mai grammatica né motorea: nessuna
	// regola cliente nei testi (par.3.3.5).
	{percorso: "internal/core/estrazione", puro: true,
		progetto: []string{"internal/core/estrazione/evidenze", "internal/core/fotorfq", "internal/platform/jsoncanonico",
			"internal/platform/contratti/worker", "internal/core/inbox/classificazione", "internal/core/inbox/lettura"},
		esterni: []string{"github.com/google/uuid"}},
	// Le proposte del motore A (A1c, P5): i prodotti chiesti dalla mail e, dai commit di B2 e B4, gli ancoraggi.
	// Le interpretazioni e le letture (motorea), i ruoli (grammatica), il documento e le diagnostiche della foglia,
	// il JSON canonico delle impronte. Mai la fotografia né il caricatore, che legge valutazione (T-B0-04), mai gli
	// adattatori, valutazione e confronto (grafo del par.3.2.1): estrazione e fotorfq solo nelle prove.
	{percorso: "internal/core/ancoraggio", puro: true,
		progetto: []string{"internal/core/inbox/classificazione/motorea", "internal/core/registro/regole/grammatica",
			"internal/core/estrazione/evidenze", "internal/platform/jsoncanonico"},
		esterni: []string{"github.com/google/uuid"}},
	// Il manifest del dataset privato: legge i file, quindi non è puro; non importa niente del progetto né
	// librerie esterne (platform non importa core: nessuna evidenze.Diagnostica).
	{percorso: "internal/platform/dataset"},
	// Il runner del banco: legge i file, non è puro. Usa lo stesso motore del prodotto e il manifest; è l'unico
	// importatore della libreria YAML (G3, qui sotto). Mai app/runtime, transport, ai, platform/config. Da A1b.11
	// importa core/estrazione, solo per DaTesto (F19, R52 A: la guardia è più sotto).
	{percorso: "internal/app/bancoa",
		progetto: []string{"internal/platform/dataset", "internal/core/registro/regole/grammatica",
			"internal/core/inbox/classificazione/motorea", "internal/core/estrazione/evidenze",
			"internal/core/estrazione", "internal/platform/jsoncanonico"},
		esterni: []string{"github.com/google/uuid", "gopkg.in/yaml.v3"}},
	// Il comando del banco, sottile come cmd/cockpit: solo il runner (in A1a nemmeno le migrazioni incorporate).
	{percorso: "cmd/bancoa", progetto: []string{"internal/app/bancoa"}},
}

// vietatiAiPuri (VP): vietati ai pacchetti puri, anche per transitività sugli import del progetto.
var vietatiAiPuri = regexp.MustCompile(`^promatec/cockpit/internal/platform/db($|/)` +
	`|^github\.com/jackc/pgx/v5($|/)` +
	`|^promatec/cockpit/internal/(ai|app|transport)/` +
	`|^net/http($|/)` +
	`|yaml` +
	`|^promatec/cockpit/internal/platform/(config|coda|testutil)($|/)` +
	`|^promatec/cockpit/internal/platform/storage/` +
	`|^promatec/cockpit/internal/core/rfq/` +
	`|^promatec/cockpit/internal/core/inbox/(ingest|aggancio)($|/)`)

// vietatiDiretti (VD): vietati come import diretto nei pacchetti puri. uuid li porta comunque, quindi si
// controllano sul sorgente e non con la chiusura.
var vietatiDiretti = map[string]bool{
	"os": true, "io/fs": true, "io/ioutil": true, "path/filepath": true,
	"net": true, "os/exec": true, "syscall": true, "database/sql": true,
	"math/rand": true, "math/rand/v2": true, "crypto/rand": true, "plugin": true, "unsafe": true,
}

// chiamateVietate: time è ammesso solo per i tipi; uuid solo per gli ID che nascono dal contenuto
// (par.3.4.5: niente uuid.New nei pacchetti puri).
var chiamateVietate = map[string]map[string]bool{
	"time": {"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "AfterFunc": true,
		"Tick": true, "NewTimer": true, "NewTicker": true},
	"github.com/google/uuid": {"New": true, "NewRandom": true, "NewString": true, "NewUUID": true,
		"NewV6": true, "NewV7": true, "NewRandomFromReader": true},
}

// radiceDelModulo risale dalla cartella del test fino a go.mod, e controlla che sia il modulo giusto.
func radiceDelModulo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			riga, _, _ := strings.Cut(string(b), "\n")
			if strings.TrimSpace(riga) != "module "+modulo {
				t.Fatalf("go.mod in %s non è del modulo %s: %q", dir, modulo, riga)
			}
			return dir
		}
		su := filepath.Dir(dir)
		if su == dir {
			t.Fatal("go.mod non trovato risalendo dalla cartella del test")
		}
		dir = su
	}
}

// sorgenti: il pacchetto (file non di prova, con i vincoli di build della piattaforma) di una cartella.
func sorgenti(t *testing.T, radice, rel string) *build.Package {
	t.Helper()
	p, err := build.Default.ImportDir(filepath.Join(radice, filepath.FromSlash(rel)), 0)
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	return p
}

// fileAnalizzati: l'albero sintattico dei file non di prova di un pacchetto, con i commenti.
func fileAnalizzati(t *testing.T, radice, rel string) (*token.FileSet, []*ast.File) {
	t.Helper()
	p := sorgenti(t, radice, rel)
	fset := token.NewFileSet()
	var out []*ast.File
	for _, nome := range p.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(p.Dir, nome), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s/%s: %v", rel, nome, err)
		}
		out = append(out, f)
	}
	return fset, out
}

func delProgetto(imp string) bool { return imp == modulo || strings.HasPrefix(imp, modulo+"/") }

func relativo(imp string) string {
	if imp == modulo {
		return "."
	}
	return strings.TrimPrefix(imp, modulo+"/")
}

// esterno: una libreria fuori dalla libreria standard ha un punto nel primo elemento del percorso.
func esterno(imp string) bool {
	primo, _, _ := strings.Cut(imp, "/")
	return !delProgetto(imp) && strings.Contains(primo, ".")
}

// TestMotoreAImportaSoloIlConsentito (G1): import diretti contro la tabella, poi la chiusura sugli import
// del progetto per i pacchetti puri.
func TestMotoreAImportaSoloIlConsentito(t *testing.T) {
	radice := radiceDelModulo(t)
	for _, r := range pacchettiMotoreA {
		p := sorgenti(t, radice, r.percorso)
		if len(p.GoFiles) == 0 {
			t.Errorf("%s: nessun file Go", r.percorso)
			continue
		}
		for _, imp := range p.Imports {
			switch {
			case delProgetto(imp):
				if !contiene(r.progetto, relativo(imp)) {
					t.Errorf("%s importa %s, che la tabella del par.3.1 non consente", r.percorso, imp)
				}
			case esterno(imp):
				if !contiene(r.esterni, imp) {
					t.Errorf("%s importa la libreria esterna %s, che la tabella del par.3.1 non consente", r.percorso, imp)
				}
			case r.puro && vietatiDiretti[imp]:
				t.Errorf("%s è puro e importa %s (VD)", r.percorso, imp)
			}
			if r.puro && vietatiAiPuri.MatchString(imp) {
				t.Errorf("%s è puro e importa %s (VP)", r.percorso, imp)
			}
		}
		if !r.puro {
			continue
		}
		// La chiusura: ogni pacchetto del progetto raggiunto, e ogni suo import diretto, contro VP.
		visti := map[string]bool{}
		coda := []string{r.percorso}
		for len(coda) > 0 {
			rel := coda[0]
			coda = coda[1:]
			if visti[rel] {
				continue
			}
			visti[rel] = true
			for _, imp := range sorgenti(t, radice, rel).Imports {
				if vietatiAiPuri.MatchString(imp) {
					t.Errorf("%s è puro e raggiunge %s attraverso %s (VP)", r.percorso, imp, rel)
				}
				if delProgetto(imp) {
					coda = append(coda, relativo(imp))
				}
			}
		}
	}
}

// TestMotoreASenzaOrologioFileNeRete (G2): nei pacchetti puri nessun import di VD, nessuna chiamata
// all'orologio o a un generatore di UUID casuali, nessuna istruzione go.
func TestMotoreASenzaOrologioFileNeRete(t *testing.T) {
	radice := radiceDelModulo(t)
	for _, r := range pacchettiMotoreA {
		if !r.puro && !r.g2 {
			continue
		}
		fset, files := fileAnalizzati(t, radice, r.percorso)
		for _, f := range files {
			nomi := map[string]string{} // nome locale del pacchetto importato → percorso
			for _, is := range f.Imports {
				imp, _ := strconv.Unquote(is.Path.Value)
				if vietatiDiretti[imp] {
					t.Errorf("%s: import %s (VD)", fset.Position(is.Pos()), imp)
				}
				nome := imp[strings.LastIndex(imp, "/")+1:]
				if is.Name != nil {
					nome = is.Name.Name
				}
				nomi[nome] = imp
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.GoStmt:
					t.Errorf("%s: istruzione go in un pacchetto puro", fset.Position(x.Pos()))
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && id.Obj == nil {
						if vietate := chiamateVietate[nomi[id.Name]]; vietate[x.Sel.Name] {
							t.Errorf("%s: %s.%s in un pacchetto puro", fset.Position(x.Pos()), id.Name, x.Sel.Name)
						}
					}
				}
				return true
			})
		}
	}
}

// TestMotoreASenzaLLMNeiSorgenti (MOTORE-SENZA-LLM, par.3.5): nei file non di prova dei pacchetti del
// motore A nessuna stringa analisi_messaggio, nessun nome «agente», nessun import dell'area ai.
func TestMotoreASenzaLLMNeiSorgenti(t *testing.T) {
	radice := radiceDelModulo(t)
	vietata := []byte("analisi" + "_messaggio")
	for _, r := range pacchettiMotoreA {
		p := sorgenti(t, radice, r.percorso)
		for _, nome := range p.GoFiles {
			b, err := os.ReadFile(filepath.Join(p.Dir, nome))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(bytes.ToLower(b), vietata) {
				t.Errorf("%s/%s nomina la tabella dell'agente", r.percorso, nome)
			}
		}
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, modulo+"/internal/ai/") {
				t.Errorf("%s importa %s", r.percorso, imp)
			}
		}
		fset, files := fileAnalizzati(t, radice, r.percorso)
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && strings.EqualFold(id.Name, "agente") {
					t.Errorf("%s: riferimento all'agente", fset.Position(id.Pos()))
				}
				return true
			})
		}
	}
}

// TestIlLegacyNonCambiaImport (G9): gli import diretti del progetto dei pacchetti che il motore A
// attraversa restano quelli di partenza del giro 5. Il legacy non importa niente di nuovo.
func TestIlLegacyNonCambiaImport(t *testing.T) {
	radice := radiceDelModulo(t)
	attesi := map[string][]string{
		"internal/core/registro/regole":       nil,
		"internal/core/inbox/classificazione": {"internal/core/registro/regole", "internal/platform/contratti/worker"},
		"internal/core/inbox/lettura":         {"internal/core/inbox/classificazione"},
		"internal/platform/migrazioni":        nil,
		"internal/platform/config":            {"internal/platform/db"},
	}
	percorsi := make([]string, 0, len(attesi))
	for p := range attesi {
		percorsi = append(percorsi, p)
	}
	sort.Strings(percorsi)
	for _, rel := range percorsi {
		var trovati []string
		for _, imp := range sorgenti(t, radice, rel).Imports {
			if delProgetto(imp) {
				trovati = append(trovati, relativo(imp))
			}
		}
		if strings.Join(trovati, " ") != strings.Join(attesi[rel], " ") {
			t.Errorf("%s importa %v dal progetto, attesi %v", rel, trovati, attesi[rel])
		}
	}
}

// ammessiAllaGrammatica: l'elenco chiuso della freccia grammatica → evidenze (R41 a; F1 del par.3.2.1 del
// piano A): il vocabolario dei selettori e il tipo Diagnostica. Le costanti Contesto*, Gravita* e Natura* si
// aggiungono lette dai sorgenti della foglia, per tipo dichiarato. Non ci sono: i tipi del documento, le
// costanti di VarianteCampo, i codici della foglia (la grammatica passa avanti le sue diagnostiche, non le
// ricostruisce).
var ammessiAllaGrammatica = []string{
	"Selettore", "LeggiSelettore", "Contesto", "VarianteCampo", "CampoFonte", "CampiAmmessi",
	"Diagnostica", "Gravita", "Natura", "ErroreContratto",
}

// costantiDiTipo: i nomi delle costanti della foglia dichiarate con uno di questi tipi.
func costantiDiTipo(t *testing.T, radice string, tipi ...string) []string {
	t.Helper()
	var out []string
	_, files := fileAnalizzati(t, radice, "internal/core/estrazione/evidenze")
	for _, f := range files {
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				vs := spec.(*ast.ValueSpec)
				if id, ok := vs.Type.(*ast.Ident); ok && contiene(tipi, id.Name) {
					for _, n := range vs.Names {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	return out
}

// TestLaGrammaticaUsaDiEvidenzeSoloIlVocabolario (G8; R41 a): nei file non di prova della grammatica ogni
// evidenze.X sta nell'elenco chiuso. DocumentoEvidenze, Fonte, Segmento, EntitaLocale, UnitaEvidenza,
// Localizzatore, Pos*, LegameFonte, QualitaFonte, UsoSegmenti, ValidaDocumento e Intervallo fanno fallire la
// prova: i DTO delle grammatiche tengono i selettori come stringhe, e la freccia serve alla validazione, non
// ai dati.
func TestLaGrammaticaUsaDiEvidenzeSoloIlVocabolario(t *testing.T) {
	radice := radiceDelModulo(t)
	ammessi := append(append([]string(nil), ammessiAllaGrammatica...), costantiDiTipo(t, radice, "Contesto", "Gravita", "Natura")...)
	if len(ammessi) < len(ammessiAllaGrammatica)+11+3+4 {
		t.Fatalf("costanti della foglia non trovate: %v", ammessi)
	}
	for _, vietato := range []string{"DocumentoEvidenze", "Fonte", "Segmento", "EntitaLocale", "UnitaEvidenza",
		"Localizzatore", "PosTesto", "PosPDF", "PosSTEP", "PosTabella", "PosNomeFile", "LegameFonte", "QualitaFonte",
		"UsoSegmenti", "UsoSconosciuto", "ValidaDocumento", "ValidaUso", "Intervallo", "VarianteNessuno",
		"CodiceSelettoreNonAmmesso"} {
		if contiene(ammessi, vietato) {
			t.Fatalf("l'elenco chiuso ammette %s", vietato)
		}
	}
	const foglia = modulo + "/internal/core/estrazione/evidenze"
	fset, files := fileAnalizzati(t, radice, "internal/core/registro/regole/grammatica")
	usi := 0
	for _, f := range files {
		nome := ""
		for _, is := range f.Imports {
			if imp, _ := strconv.Unquote(is.Path.Value); imp == foglia {
				nome = "evidenze"
				if is.Name != nil {
					nome = is.Name.Name
				}
			}
		}
		if nome == "." || nome == "_" {
			t.Errorf("%s: import della foglia con il nome %q: i nomi usati non si vedrebbero", fset.Position(f.Pos()), nome)
			continue
		}
		if nome == "" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == nome && id.Obj == nil {
				usi++
				if !contiene(ammessi, sel.Sel.Name) {
					t.Errorf("%s: la grammatica usa evidenze.%s, fuori dall'elenco chiuso di R41 a", fset.Position(sel.Pos()), sel.Sel.Name)
				}
			}
			return true
		})
	}
	if usi == 0 {
		t.Fatal("nessun uso della foglia trovato: la prova non legge i file giusti")
	}
}

func contiene(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}

// legacyVietati: il motore legacy di classificazione e le regole legacy, per percorso esatto: motorea e
// grammatica stanno nelle loro cartelle ma non li importano, nemmeno per transitività (R7, R40 a-b).
var legacyVietati = []string{
	modulo + "/internal/core/inbox/classificazione",
	modulo + "/internal/core/registro/regole",
}

// TestIlMotoreANonUsaIlLegacy (G1 con il divieto del legacy; R7): motorea e grammatica non raggiungono i
// pacchetti legacy per nessuna catena di import del progetto, e nei file non di prova di motorea non c'è
// nessun riferimento al riconoscitore legacy della minuteria: la categoria viene dalla famiglia dichiarata.
func TestIlMotoreANonUsaIlLegacy(t *testing.T) {
	radice := radiceDelModulo(t)
	for _, partenza := range []string{"internal/core/inbox/classificazione/motorea", "internal/core/registro/regole/grammatica"} {
		visti := map[string]bool{}
		coda := []string{partenza}
		for len(coda) > 0 {
			rel := coda[0]
			coda = coda[1:]
			if visti[rel] {
				continue
			}
			visti[rel] = true
			for _, imp := range sorgenti(t, radice, rel).Imports {
				for _, v := range legacyVietati {
					if imp == v {
						t.Errorf("%s raggiunge il legacy %s attraverso %s", partenza, imp, rel)
					}
				}
				if delProgetto(imp) {
					coda = append(coda, relativo(imp))
				}
			}
		}
	}

	fset, files := fileAnalizzati(t, radice, "internal/core/inbox/classificazione/motorea")
	if len(files) == 0 {
		t.Fatal("nessun file di motorea: la prova non legge i file giusti")
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if strings.EqualFold(x.Name, "Minuteria") {
					t.Errorf("%s: riferimento a %s nel motore A (R7)", fset.Position(x.Pos()), x.Name)
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING && strings.Contains(strings.ToLower(x.Value), "minuteria(") {
					t.Errorf("%s: chiamata alla minuteria scritta in una stringa", fset.Position(x.Pos()))
				}
			}
			return true
		})
	}
}

// TestLaLibreriaYAMLSoloNelBanco (G3; R9, R40 e, P-18): in tutto il modulo la libreria YAML la importa solo
// internal/app/bancoa, contando anche gli import delle prove (TestImports, XTestImports) e i file con i tag
// integrazione, browser e privato. Gli attesi entrano solo dal runner: il motore non li legge mai.
func TestLaLibreriaYAMLSoloNelBanco(t *testing.T) {
	radice := radiceDelModulo(t)
	ctx := build.Default
	ctx.BuildTags = []string{"integrazione", "browser", "privato"}
	const banco = "internal/app/bancoa"
	nelBanco, pacchetti := false, 0
	err := filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p != radice {
			nome := d.Name()
			if nome == "testdata" || strings.HasPrefix(nome, ".") || strings.HasPrefix(nome, "_") {
				return filepath.SkipDir // il go tool li ignora
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir // un altro modulo
			}
		}
		pkg, err := ctx.ImportDir(p, 0)
		if err != nil {
			var nessuno *build.NoGoError
			if errors.As(err, &nessuno) {
				return nil
			}
			return fmt.Errorf("%s: %w", p, err)
		}
		pacchetti++
		rel, err := filepath.Rel(radice, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, gruppo := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
			for _, imp := range gruppo {
				if !strings.Contains(strings.ToLower(imp), "yaml") {
					continue
				}
				if rel == banco {
					nelBanco = true
					continue
				}
				t.Errorf("%s importa %s: la libreria YAML sta solo in %s (G3)", rel, imp, banco)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !nelBanco || pacchetti < 20 {
		t.Fatalf("la prova non legge i file giusti: YAML nel banco %v, %d pacchetti letti", nelBanco, pacchetti)
	}
}

// TestIlMotoreANonLeggeRifCaso (R47 b): il riferimento al caso degli attesi è un metadato opaco, che legge
// solo il banco. Nei file non di prova di motorea nessun selettore .RifCaso, nessuna chiave RifCaso in un
// letterale composto e nessuna stringa che lo nomini (per esempio per la riflessione o per la chiave JSON).
func TestIlMotoreANonLeggeRifCaso(t *testing.T) {
	radice := radiceDelModulo(t)
	fset, files := fileAnalizzati(t, radice, "internal/core/inbox/classificazione/motorea")
	if len(files) == 0 {
		t.Fatal("nessun file di motorea: la prova non legge i file giusti")
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if x.Sel.Name == "RifCaso" {
					t.Errorf("%s: accesso a RifCaso nel motore (R47 b)", fset.Position(x.Pos()))
				}
			case *ast.KeyValueExpr:
				if id, ok := x.Key.(*ast.Ident); ok && id.Name == "RifCaso" {
					t.Errorf("%s: RifCaso in un letterale nel motore (R47 b)", fset.Position(x.Pos()))
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					v := strings.ToLower(x.Value)
					if strings.Contains(v, "rifcaso") || strings.Contains(v, "rif_caso") {
						t.Errorf("%s: il riferimento al caso nominato in una stringa (R47 b)", fset.Position(x.Pos()))
					}
				}
			}
			return true
		})
	}
}

// ammessiDalLegacy: l'elenco chiuso di G4 (par.3.2, con P-08 e R48 A): di classificazione e lettura gli
// adattatori usano solo il taglio con posizioni, EInoltro e le tabelle con origine. Le costanti dei tipi
// StatoTaglio e RegolaTaglio si aggiungono lette dai sorgenti di classificazione, per tipo dichiarato, come le
// costanti della foglia in G8.
var ammessiDalLegacy = map[string][]string{
	"internal/core/inbox/classificazione": {"TagliaCatenaConPosizioni", "Taglio", "StatoTaglio", "RegolaTaglio",
		"RigaTesto", "LivelliDellaStoria", "LivelloStoria", "EInoltro"},
	"internal/core/inbox/lettura": {"TabelleConOrigine", "TabellaOrigine", "CellaOrigine", "TestoDaHTML"},
}

// vietatiDalLegacy: i nomi che G4 nomina come vietati. Stanno già fuori dall'elenco chiuso; la prova controlla
// che l'elenco non li ammetta per sbaglio.
var vietatiDalLegacy = []string{"Minuteria", "Compila", "CodiciDa", "Riconosci", "Valuta", "Canonico", "TagliaCatena"}

// costantiDelTipo: i nomi delle costanti di un pacchetto dichiarate con uno di questi tipi.
func costantiDelTipo(t *testing.T, radice, rel string, tipi ...string) []string {
	t.Helper()
	var out []string
	_, files := fileAnalizzati(t, radice, rel)
	for _, f := range files {
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				vs := spec.(*ast.ValueSpec)
				if id, ok := vs.Type.(*ast.Ident); ok && contiene(tipi, id.Name) {
					for _, n := range vs.Names {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	return out
}

// TestEstrazioneUsaDelLegacySoloIlTaglio (G4; P-08, R48 A): nei file non di prova di core/estrazione ogni
// classificazione.X e lettura.X sta nell'elenco chiuso. Il motore legacy (Minuteria, Compila, CodiciDa,
// Riconosci, Valuta, Canonico, TagliaCatena senza posizioni) non entra negli adattatori. Un import del legacy
// con il nome «.» o «_» fa fallire la prova: i nomi usati non si vedrebbero.
func TestEstrazioneUsaDelLegacySoloIlTaglio(t *testing.T) {
	radice := radiceDelModulo(t)
	ammessi := map[string][]string{}
	for _, rel := range []string{"internal/core/inbox/classificazione", "internal/core/inbox/lettura"} {
		elenco := append([]string(nil), ammessiDalLegacy[rel]...)
		if rel == "internal/core/inbox/classificazione" {
			costanti := costantiDelTipo(t, radice, rel, "StatoTaglio", "RegolaTaglio")
			if len(costanti) < 3+5 {
				t.Fatalf("costanti di StatoTaglio e RegolaTaglio non trovate: %v", costanti)
			}
			elenco = append(elenco, costanti...)
		}
		for _, v := range vietatiDalLegacy {
			if contiene(elenco, v) {
				t.Fatalf("l'elenco chiuso di G4 ammette %s", v)
			}
		}
		ammessi[modulo+"/"+rel] = elenco
	}

	fset, files := fileAnalizzati(t, radice, "internal/core/estrazione")
	if len(files) == 0 {
		t.Fatal("nessun file di core/estrazione: la prova non legge i file giusti")
	}
	for _, f := range files {
		nomi := map[string]string{} // nome locale → percorso del pacchetto legacy
		for _, is := range f.Imports {
			imp, _ := strconv.Unquote(is.Path.Value)
			if _, ok := ammessi[imp]; !ok {
				continue
			}
			nome := imp[strings.LastIndex(imp, "/")+1:]
			if is.Name != nil {
				nome = is.Name.Name
			}
			if nome == "." || nome == "_" {
				t.Errorf("%s: import di %s con il nome %q: i nomi usati non si vedrebbero", fset.Position(is.Pos()), imp, nome)
				continue
			}
			nomi[nome] = imp
		}
		if len(nomi) == 0 {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Obj != nil {
				return true
			}
			if imp, ok := nomi[id.Name]; ok && !contiene(ammessi[imp], sel.Sel.Name) {
				t.Errorf("%s: core/estrazione usa %s.%s, fuori dall'elenco chiuso di G4", fset.Position(sel.Pos()), id.Name, sel.Sel.Name)
			}
			return true
		})
	}
}

// TestIlBancoUsaDiEstrazioneSoloDaTesto (F19, R52 A; A1b.11): nei file non di prova di internal/app/bancoa ogni
// estrazione.X è DaTesto. Il banco fa il documento del caso con DaTesto e lo passa a Interpreta: gli adattatori dei
// fatti (DaAllegato, DaMessaggio) e il resto del pacchetto non entrano nel modo casi. Un import di core/estrazione
// con il nome «.» o «_» fa fallire la prova: i nomi usati non si vedrebbero. Se nessun file usa DaTesto la prova
// fallisce: legge i file sbagliati, o la freccia è sparita senza togliere la riga dalla tabella.
func TestIlBancoUsaDiEstrazioneSoloDaTesto(t *testing.T) {
	radice := radiceDelModulo(t)
	const estrazione = modulo + "/internal/core/estrazione"
	fset, files := fileAnalizzati(t, radice, "internal/app/bancoa")
	if len(files) == 0 {
		t.Fatal("nessun file di app/bancoa: la prova non legge i file giusti")
	}
	usi := 0
	for _, f := range files {
		nome := ""
		for _, is := range f.Imports {
			imp, _ := strconv.Unquote(is.Path.Value)
			if imp != estrazione {
				continue
			}
			nome = "estrazione"
			if is.Name != nil {
				nome = is.Name.Name
			}
			if nome == "." || nome == "_" {
				t.Errorf("%s: import di %s con il nome %q: i nomi usati non si vedrebbero", fset.Position(is.Pos()), imp, nome)
				nome = ""
			}
		}
		if nome == "" {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Obj != nil || id.Name != nome {
				return true
			}
			if sel.Sel.Name != "DaTesto" {
				t.Errorf("%s: app/bancoa usa %s.%s: di core/estrazione il banco usa solo DaTesto (F19, R52 A)", fset.Position(sel.Pos()), id.Name, sel.Sel.Name)
			} else {
				usi++
			}
			return true
		})
	}
	if usi == 0 {
		t.Error("app/bancoa non usa estrazione.DaTesto: la freccia F19 della tabella non ha più motivo")
	}
}

// TestLeQueryDellaFotografiaSenzaLAgente (A1c-L1-28, MOTORE-SENZA-LLM, par.3.5): le query nuove della fotografia,
// la sorgente e il codice generato, non nominano mai la tabella dei suggerimenti dell'agente. Il file deve
// esserci: senza, la prova non legge il file giusto.
func TestLeQueryDellaFotografiaSenzaLAgente(t *testing.T) {
	radice := radiceDelModulo(t)
	vietata := []byte("analisi" + "_messaggio")
	for _, rel := range []string{"internal/platform/db/queries/fotografia.sql", "internal/platform/db/fotografia.sql.go"} {
		b, err := os.ReadFile(filepath.Join(radice, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if !bytes.Contains(b, []byte("ListFattiDelThread")) {
			t.Fatalf("%s non ha le query della fotografia: la prova non legge il file giusto", rel)
		}
		if bytes.Contains(bytes.ToLower(b), vietata) {
			t.Errorf("%s nomina la tabella dei suggerimenti dell'agente", rel)
		}
	}
}
