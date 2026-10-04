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
//   - MOTORE-SENZA-LLM: nessun riferimento ad analisi_messaggio o all'agente nei sorgenti del motore;
//   - G1, il divieto del legacy (R7, R40 a-b): motorea e grammatica non raggiungono il motore legacy di
//     classificazione né le regole legacy, e motorea non chiama il riconoscitore della minuteria;
//   - la guardia su RifCaso (R47 b): nei file non di prova di motorea nessun accesso al riferimento opaco al
//     caso degli attesi che un esempio può portare.
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
