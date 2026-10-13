package confermabom

// L1 — A1d P4, B2.3 parte pura: la firma del perimetro (contratto rev. 3 §5.1, emendato da D-FIRMA-01 = b [U],
// direttiva 12 §5: firma-perimetro-2). Entrano id, codice letterale, tipo, archiviato, step_strutturale_id dei
// componenti del perimetro, e padre, figlio, qta dei suoi archi; non entrano rev, descrizione, posizione e il codice
// canonico (le regole del cliente); l'ordine degli ingressi non conta; quello che sta fuori dal perimetro non conta.
// Codici inventati della famiglia ACME 712; UUID fissi, per la forma canonica scritta per intero.

import (
	"errors"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func id(n byte) uuid.UUID {
	var u uuid.UUID
	u[0], u[15] = 0xa1, n
	u[6], u[8] = 0x40, 0x80 // versione 4, variante RFC 4122: un UUID ben formato
	return u
}

// scena: il prodotto 7120001 (finito, con lo STEP strutturale) → 7120010 (assieme) ×1 → 7120011 ×2, e 7120012 ×4
// sotto il prodotto; fuori dal perimetro 7120099, figlio di un altro prodotto 7120002.
type scena struct {
	comp  []Componente
	archi []Arco
}

const (
	prodotto = 1
	assieme  = 2
	pezzo11  = 3
	pezzo12  = 4
	altro    = 5
	fuori    = 6
)

func nuovaScena() *scena {
	step := uuid.NullUUID{UUID: id(0xaa), Valid: true}
	return &scena{
		comp: []Componente{
			{ID: id(prodotto), CodiceLetterale: "7120001", Tipo: "finito", StepStrutturale: step, Rev: "A", Descrizione: "PRODOTTO ACME"},
			{ID: id(assieme), CodiceLetterale: "7120010_prt", Tipo: "sottoassieme", Rev: "B", Descrizione: "TELAIO"},
			{ID: id(pezzo11), CodiceLetterale: "7120011", Tipo: "sciolto"},
			{ID: id(pezzo12), CodiceLetterale: "7120012", Tipo: "commerciale"},
			{ID: id(altro), CodiceLetterale: "7120002", Tipo: "finito"},
			{ID: id(fuori), CodiceLetterale: "7120099", Tipo: "sciolto"},
		},
		archi: []Arco{
			{Padre: id(prodotto), Figlio: id(assieme), Qta: 1, Posizione: "10"},
			{Padre: id(assieme), Figlio: id(pezzo11), Qta: 2},
			{Padre: id(prodotto), Figlio: id(pezzo12), Qta: 4, Posizione: "20"},
			{Padre: id(altro), Figlio: id(fuori), Qta: 1},
		},
	}
}

func (s *scena) firma(t *testing.T) string {
	t.Helper()
	f, err := FirmaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (s *scena) c(n byte) *Componente {
	for i := range s.comp {
		if s.comp[i].ID == id(n) {
			return &s.comp[i]
		}
	}
	panic("componente assente")
}

func (s *scena) a(p, f byte) *Arco {
	for i := range s.archi {
		if s.archi[i].Padre == id(p) && s.archi[i].Figlio == id(f) {
			return &s.archi[i]
		}
	}
	panic("arco assente")
}

var reSha = regexp.MustCompile(`^[0-9a-f]{64}$`)

// La forma canonica scritta per intero: e' la documentazione della firma. Se cambia, cambiano tutte le firme
// registrate (VersioneFirma): la prova lo dice prima del commit.
func TestFormaCanonicaDelPerimetro(t *testing.T) {
	s := nuovaScena()
	p, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	b, err := FormaCanonica(p)
	if err != nil {
		t.Fatal(err)
	}
	const attesa = `{"archi":[` +
		`{"figlio":"a1000000-0000-4000-8000-000000000002","padre":"a1000000-0000-4000-8000-000000000001","qta":1},` +
		`{"figlio":"a1000000-0000-4000-8000-000000000004","padre":"a1000000-0000-4000-8000-000000000001","qta":4},` +
		`{"figlio":"a1000000-0000-4000-8000-000000000003","padre":"a1000000-0000-4000-8000-000000000002","qta":2}],` +
		`"canonicalizzazione":"canonico-1","componenti":[` +
		`{"archiviato":false,"codice":"7120001","id":"a1000000-0000-4000-8000-000000000001","step_strutturale_id":"a1000000-0000-4000-8000-0000000000aa","tipo":"finito"},` +
		`{"archiviato":false,"codice":"7120010_prt","id":"a1000000-0000-4000-8000-000000000002","step_strutturale_id":null,"tipo":"sottoassieme"},` +
		`{"archiviato":false,"codice":"7120011","id":"a1000000-0000-4000-8000-000000000003","step_strutturale_id":null,"tipo":"sciolto"},` +
		`{"archiviato":false,"codice":"7120012","id":"a1000000-0000-4000-8000-000000000004","step_strutturale_id":null,"tipo":"commerciale"}],` +
		`"prodotto":"a1000000-0000-4000-8000-000000000001","versione":"firma-perimetro-2"}`
	if string(b) != attesa {
		t.Fatalf("forma canonica:\n  %s\natteso:\n  %s", b, attesa)
	}
	f := s.firma(t)
	if !reSha.MatchString(f) {
		t.Fatalf("la firma e' di 64 cifre esadecimali minuscole (CHECK della 0022): %q", f)
	}
	if g, _ := p.Firma(); g != f {
		t.Errorf("Perimetro.Firma e FirmaPerimetro: %s, %s", g, f)
	}
}

// L'ordine in cui arrivano componenti e archi non conta: ne' negli ingressi di FirmaPerimetro, ne' in un Perimetro
// scritto a mano e passato a FormaCanonica (revisione della corsia D, B1: CalcolaPerimetro ordina gia', e senza la
// seconda parte un FormaCanonica che non ordina passerebbe).
func TestFirmaNonDipendeDallOrdine(t *testing.T) {
	s := nuovaScena()
	attesa := s.firma(t)
	r := rand.New(rand.NewSource(712))
	for i := 0; i < 50; i++ {
		r.Shuffle(len(s.comp), func(a, b int) { s.comp[a], s.comp[b] = s.comp[b], s.comp[a] })
		r.Shuffle(len(s.archi), func(a, b int) { s.archi[a], s.archi[b] = s.archi[b], s.archi[a] })
		if got := s.firma(t); got != attesa {
			t.Fatalf("giro %d: firma %s, attesa %s", i, got, attesa)
		}
	}
	p, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	forma, err := FormaCanonica(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		r.Shuffle(len(p.Componenti), func(a, b int) { p.Componenti[a], p.Componenti[b] = p.Componenti[b], p.Componenti[a] })
		r.Shuffle(len(p.Archi), func(a, b int) { p.Archi[a], p.Archi[b] = p.Archi[b], p.Archi[a] })
		got, err := FormaCanonica(p)
		if err != nil || string(got) != string(forma) {
			t.Fatalf("giro %d, perimetro rimescolato: forma %s (%v), attesa %s", i, got, err, forma)
		}
	}
}

// Rev, descrizione e posizione non entrano: una revisione solo CAD lascia la struttura com'e' (dir. 01 §4).
func TestFirmaIgnoraRevDescrizionePosizione(t *testing.T) {
	s := nuovaScena()
	attesa := s.firma(t)
	s.c(assieme).Rev, s.c(assieme).Descrizione = "C", "TELAIO SALDATO"
	s.c(prodotto).Rev = ""
	s.a(prodotto, assieme).Posizione = "99"
	s.a(assieme, pezzo11).Posizione = "1"
	if got := s.firma(t); got != attesa {
		t.Errorf("rev, descrizione e posizione hanno cambiato la firma: %s → %s", attesa, got)
	}
}

// Quello che entra cambia la firma: qta, tipo, archiviazione, step_strutturale_id, codice letterale, un arco in piu'
// o in meno. Ogni caso parte dalla scena intatta. Il codice canonico non c'e' piu' (firma-perimetro-2): le regole del
// cliente le prova TestScenarioFLeRegoleDelClienteNonCambianoLaFirma.
func TestFirmaCambiaConQuelloCheEntra(t *testing.T) {
	base := nuovaScena().firma(t)
	casi := []struct {
		nome   string
		cambia func(s *scena)
	}{
		{"qta di un arco", func(s *scena) { s.a(assieme, pezzo11).Qta = 3 }},
		{"qta di un arco del prodotto", func(s *scena) { s.a(prodotto, pezzo12).Qta = 1 }},
		{"tipo", func(s *scena) { s.c(pezzo11).Tipo = "commerciale" }},
		{"archiviato un figlio (esce dal perimetro con il suo arco)", func(s *scena) { s.c(pezzo11).Archiviato = true }},
		{"archiviato il prodotto (resta, con il segno)", func(s *scena) { s.c(prodotto).Archiviato = true }},
		{"step_strutturale_id tolto", func(s *scena) { s.c(prodotto).StepStrutturale = uuid.NullUUID{} }},
		{"step_strutturale_id sostituito", func(s *scena) { s.c(prodotto).StepStrutturale = uuid.NullUUID{UUID: id(0xbb), Valid: true} }},
		{"codice letterale (solo maiuscole, canonico uguale)", func(s *scena) { s.c(assieme).CodiceLetterale = "7120010_PRT" }},
		{"codice letterale (suffisso tolto, canonico uguale)", func(s *scena) { s.c(assieme).CodiceLetterale = "7120010" }},
		{"arco nuovo", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(pezzo12), Qta: 1}) }},
		{"arco tolto", func(s *scena) { s.archi = s.archi[1:] }},
		{"un componente fuori entra nel perimetro", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(pezzo11), Figlio: id(fuori), Qta: 1}) }},
	}
	viste := map[string]string{}
	for _, c := range casi {
		s := nuovaScena()
		c.cambia(s)
		got := s.firma(t)
		if got == base {
			t.Errorf("%s: la firma non e' cambiata", c.nome)
		}
		if prima, gia := viste[got]; gia {
			t.Errorf("%s e %s danno la stessa firma", c.nome, prima)
		}
		viste[got] = c.nome
	}
}

// Scenario F (direttiva 12 §10), parte pura: la firma porta il codice letterale com'e', e nessun codice canonico. Due
// letterali che le regole di un cliente renderebbero uguali («7120010_prt», «7120010_PRT», «7120010») danno tre
// firme diverse; uno spazio in coda e' un altro codice. Nella forma non c'e' la chiave «codice_canonico».
func TestScenarioFLaFirmaPortaIlCodiceLetterale(t *testing.T) {
	s := nuovaScena()
	attesa := s.firma(t)
	p, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := FormaCanonica(p)
	if !strings.Contains(string(b), `"codice":"7120010_prt","id"`) {
		t.Errorf("il letterale entra com'e': %s", b)
	}
	if strings.Contains(string(b), "codice_canonico") {
		t.Errorf("il codice canonico non entra nella firma (D-FIRMA-01 = b): %s", b)
	}
	if got := s.firma(t); got != attesa {
		t.Errorf("due calcoli sugli stessi ingressi: %s, %s", attesa, got)
	}
	viste := map[string]string{attesa: "7120010_prt"}
	for _, letterale := range []string{"7120010_PRT", "7120010", "7120010_prt "} {
		s := nuovaScena()
		s.c(assieme).CodiceLetterale = letterale
		got := s.firma(t)
		if prima, gia := viste[got]; gia {
			t.Errorf("%q e %q danno la stessa firma: il letterale deve entrare com'e'", letterale, prima)
		}
		viste[got] = letterale
	}
}

// Scenario F, la regola strutturale: le regole del cliente non sono un ingresso della firma. Il package non importa
// ne' il Motore della classificazione ne' il registro delle regole (senza import nessuna funzione puo' riceverli):
// chi rimettesse il canonico nella firma dovrebbe cambiare questa prova (e VersioneFirma).
func TestScenarioFLeRegoleDelClienteNonCambianoLaFirma(t *testing.T) {
	vietati := map[string]bool{
		"promatec/cockpit/internal/core/inbox/classificazione": true,
		"promatec/cockpit/internal/core/registro/regole":       true,
	}
	file, err := filepath.Glob("*.go")
	if err != nil || len(file) == 0 {
		t.Fatalf("i sorgenti del package: %v %v", file, err)
	}
	fs := token.NewFileSet()
	for _, f := range file {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fs, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			if percorso, _ := strconv.Unquote(imp.Path.Value); vietati[percorso] {
				t.Errorf("%s importa %s: la firma non dipende dalle regole del cliente (D-FIRMA-01 = b)", f, percorso)
			}
		}
	}
	if VersioneFirma != "firma-perimetro-2" {
		t.Errorf("VersioneFirma = %q: la forma senza canonico e' firma-perimetro-2", VersioneFirma)
	}
}

// Quello che sta fuori dal perimetro non conta: un altro prodotto, i suoi archi, la sua qta, un componente archiviato
// che nessuno raggiunge, un figlio archiviato con il suo arco.
func TestFuoriDalPerimetroNonConta(t *testing.T) {
	s := nuovaScena()
	attesa := s.firma(t)
	s.a(altro, fuori).Qta = 7
	s.c(fuori).Tipo, s.c(fuori).CodiceLetterale = "commerciale", "7120098"
	s.comp = append(s.comp, Componente{ID: id(0x70), CodiceLetterale: "7120070", Tipo: "sciolto", Archiviato: true})
	if got := s.firma(t); got != attesa {
		t.Errorf("un cambiamento fuori dal perimetro ha cambiato la firma: %s → %s", attesa, got)
	}
	// un figlio archiviato non entra, e nemmeno il suo arco: e' come se non ci fosse
	s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(0x70), Qta: 5})
	if got := s.firma(t); got != attesa {
		t.Errorf("un figlio archiviato ha cambiato la firma: %s → %s", attesa, got)
	}
	p, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Componenti) != 4 || len(p.Archi) != 3 {
		t.Errorf("perimetro: %d componenti, %d archi; attesi 4 e 3", len(p.Componenti), len(p.Archi))
	}
}

// Δ44: un arco aggiunto e poi tolto riporta la firma di prima (il contatore invece e' cresciuto due volte: decide la
// firma). Lo stesso per una qta cambiata e rimessa.
func TestFirmaTornaUgualeDopoArcoAggiuntoETolto(t *testing.T) {
	s := nuovaScena()
	attesa := s.firma(t)
	s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(pezzo12), Qta: 1})
	if s.firma(t) == attesa {
		t.Fatal("con l'arco in piu' la firma deve cambiare")
	}
	s.archi = s.archi[:len(s.archi)-1]
	s.a(assieme, pezzo11).Qta = 9
	s.a(assieme, pezzo11).Qta = 2
	if got := s.firma(t); got != attesa {
		t.Errorf("tolto l'arco e rimessa la qta: %s, attesa %s", got, attesa)
	}
}

// Un pezzo sotto due padri del perimetro c'e' una volta, con i due archi; un ciclo nella working non fa girare la
// visita all'infinito.
func TestPerimetroConPiuPadriECicli(t *testing.T) {
	s := nuovaScena()
	s.archi = append(s.archi, Arco{Padre: id(pezzo12), Figlio: id(pezzo11), Qta: 1})
	p, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Componenti) != 4 || len(p.Archi) != 4 {
		t.Errorf("un pezzo con due padri: %d componenti, %d archi", len(p.Componenti), len(p.Archi))
	}
	s.archi = append(s.archi, Arco{Padre: id(pezzo11), Figlio: id(assieme), Qta: 1})
	if _, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi); err != nil {
		t.Errorf("un ciclo (che la 0020 vieta) non e' un errore della firma: %v", err)
	}
}

// Gli ingressi impossibili sono un errore di chi chiama (ErrIngresso), non una firma.
func TestIngressiNonValidi(t *testing.T) {
	casi := []struct {
		nome   string
		cambia func(s *scena)
		prod   uuid.UUID
	}{
		{"prodotto assente", func(s *scena) {}, id(0x99)},
		{"componente ripetuto", func(s *scena) { s.comp = append(s.comp, s.comp[2]) }, id(prodotto)},
		{"componente senza codice", func(s *scena) { s.c(pezzo11).CodiceLetterale = "" }, id(prodotto)},
		{"tipo sconosciuto", func(s *scena) { s.c(pezzo11).Tipo = "prodotto" }, id(prodotto)},
		{"arco verso un assente", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(0x99), Qta: 1}) }, id(prodotto)},
		{"arco da un assente", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(0x99), Figlio: id(assieme), Qta: 1}) }, id(prodotto)},
		{"arco su se stesso", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(assieme), Qta: 1}) }, id(prodotto)},
		{"arco ripetuto", func(s *scena) { s.archi = append(s.archi, Arco{Padre: id(assieme), Figlio: id(pezzo11), Qta: 5}) }, id(prodotto)},
		{"qta zero", func(s *scena) { s.a(assieme, pezzo11).Qta = 0 }, id(prodotto)},
	}
	for _, c := range casi {
		s := nuovaScena()
		c.cambia(s)
		if _, err := FirmaPerimetro(c.prod, s.comp, s.archi); !errors.Is(err, ErrIngresso) {
			t.Errorf("%s: atteso ErrIngresso, ottenuto %v", c.nome, err)
		}
	}
	// un Perimetro scritto a mano con un componente senza codice: anche FormaCanonica lo rifiuta
	s := nuovaScena()
	p, err := CalcolaPerimetro(id(prodotto), s.comp, s.archi)
	if err != nil {
		t.Fatal(err)
	}
	p.Componenti[1].CodiceLetterale = ""
	if _, err := FormaCanonica(p); !errors.Is(err, ErrIngresso) {
		t.Errorf("perimetro a mano senza codice: atteso ErrIngresso, ottenuto %v", err)
	}
}

// Il prodotto senza figli (un pezzo unico) ha una firma sua, con il solo prodotto.
func TestFirmaDelProdottoSenzaFigli(t *testing.T) {
	c := []Componente{{ID: id(prodotto), CodiceLetterale: "7120001", Tipo: "finito"}}
	p, err := CalcolaPerimetro(id(prodotto), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Componenti) != 1 || len(p.Archi) != 0 {
		t.Fatalf("perimetro del pezzo unico: %+v", p)
	}
	b, _ := FormaCanonica(p)
	if want := `"archi":[]`; !regexp.MustCompile(regexp.QuoteMeta(want)).Match(b) {
		t.Errorf("senza archi la forma ha un elenco vuoto, non null: %s", b)
	}
}
