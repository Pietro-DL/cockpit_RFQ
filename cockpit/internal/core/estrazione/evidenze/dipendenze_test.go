package evidenze

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
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
//   - MOTORE-SENZA-LLM: nessun riferimento ad analisi_messaggio o all'agente nei sorgenti del motore.
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
	progetto []string // import del progetto consentiti, relativi alla radice del modulo
	esterni  []string // librerie esterne consentite (la libreria standard è consentita salvo VD)
}

// pacchettiMotoreA: i pacchetti del motore A che esistono. Un pacchetto nuovo entra qui nel commit «le
// prove» della sessione che lo crea; un pacchetto elencato che non c'è fa fallire la prova.
var pacchettiMotoreA = []regolePacchetto{
	{percorso: "internal/platform/jsoncanonico", puro: true},
	{percorso: "internal/core/estrazione/evidenze", puro: true, esterni: []string{"github.com/google/uuid"}},
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
		if !r.puro {
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

func contiene(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}
