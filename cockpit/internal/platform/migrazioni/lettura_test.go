// L1 — l'apertura in sola lettura di platform/migrazioni (A1c-L1-01; piano A, 6.4.3 e 3.3.6; P-02): SchemaDiverso
// con un testo neutro, UltimaApplicata, Destinazione senza password e con un errore che non ripete il DSN,
// ControllaSolaLettura con un lettore finto (un controllo fallito per volta, e ogni errore nomina il suo
// controllo), e lettura.go che non ha ApriSenzaMigrare e non chiama mai le funzioni che applicano le migrazioni.
//
// I database e gli utenti di questi test sono inventati: nessun nome reale di copia o di ruolo, il repository è
// pubblico.

package migrazioni

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSchemaDiversoHaUnTestoNeutro(t *testing.T) {
	err := error(&SchemaDiverso{DelDatabase: 20, DelBinario: 21})
	testo := err.Error()
	if !strings.Contains(testo, "20") || !strings.Contains(testo, "21") {
		t.Errorf("il testo non dice le due versioni: %q", testo)
	}
	// il rimedio lo dice chi chiama: il Cockpit (backup e -migra), il banco (una copia migrata)
	for _, rimedio := range []string{"-migra", "backup", "cockpit.exe", "copia"} {
		if strings.Contains(testo, rimedio) {
			t.Errorf("il testo di SchemaDiverso suggerisce un rimedio (%q): %q", rimedio, testo)
		}
	}
	var sd *SchemaDiverso
	if !errors.As(fmt.Errorf("apertura: %w", err), &sd) || sd.DelDatabase != 20 || sd.DelBinario != 21 {
		t.Errorf("SchemaDiverso non si ritrova con errors.As: %v", sd)
	}
}

func TestUltimaApplicata(t *testing.T) {
	for _, c := range []struct {
		nome      string
		applicate map[int]bool
		attesa    int
	}{
		{"nessuna (schema vuoto)", nil, 0},
		{"mappa vuota", map[int]bool{}, 0},
		{"una sola", map[int]bool{1: true}, 1},
		{"in ordine", map[int]bool{1: true, 2: true, 3: true}, 3},
		{"il massimo anche con un buco (il buco lo ferma ApplicaElenco, non qui)", map[int]bool{1: true, 2: true, 7: true}, 7},
	} {
		if got := UltimaApplicata(c.applicate); got != c.attesa {
			t.Errorf("%s: %d, attesa %d", c.nome, got, c.attesa)
		}
	}
}

func TestDestinazioneSenzaPassword(t *testing.T) {
	for _, c := range []struct {
		nome, dsn, testo, database string
	}{
		{"URL", "postgres://utente_prova:segreta@127.0.0.1:5433/acme_prova_test", "127.0.0.1:5433/acme_prova_test come utente_prova", "acme_prova_test"},
		// il nome è quello risolto: `?dbname=` vince sul percorso
		{"URL con dbname", "postgres://utente_prova:segreta@127.0.0.1:5433/acme_altro?dbname=acme_prova_test", "127.0.0.1:5433/acme_prova_test come utente_prova", "acme_prova_test"},
		{"chiave=valore", "host=127.0.0.1 port=5433 user=utente_prova password=segreta dbname=acme_prova_test", "127.0.0.1:5433/acme_prova_test come utente_prova", "acme_prova_test"},
		{"IPv6", "postgres://utente_prova:segreta@[::1]:5433/acme_prova_test", "[::1]:5433/acme_prova_test come utente_prova", "acme_prova_test"},
	} {
		testo, nome, err := Destinazione(c.dsn)
		if err != nil || testo != c.testo || nome != c.database {
			t.Errorf("%s: %q %q %v; atteso %q %q", c.nome, testo, nome, err, c.testo, c.database)
		}
		if strings.Contains(testo, "segreta") {
			t.Errorf("%s: la destinazione dice la password: %q", c.nome, testo)
		}
	}
	const illeggibile = "postgres://utente_prova:segreta@127.0.0.1:porta/acme_prova_test"
	_, _, err := Destinazione(illeggibile)
	if err == nil {
		t.Fatal("un DSN che non si legge: nessun errore")
	}
	for _, pezzo := range []string{"segreta", "utente_prova", "acme_prova_test", "porta"} {
		if strings.Contains(err.Error(), pezzo) {
			t.Errorf("l'errore ripete il DSN (%q): %v", pezzo, err)
		}
	}
}

// rigaFinta: la risposta di una domanda del lettore finto. err, se c'è, la dà Scan.
type rigaFinta struct {
	valori []any
	err    error
}

func (r rigaFinta) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.valori) {
		return fmt.Errorf("Scan con %d destinazioni, la riga ne ha %d", len(dest), len(r.valori))
	}
	for i, d := range dest {
		v := reflect.ValueOf(d)
		if v.Kind() != reflect.Pointer {
			return fmt.Errorf("destinazione %d non è un puntatore", i)
		}
		v.Elem().Set(reflect.ValueOf(r.valori[i]))
	}
	return nil
}

// lettoreFinto risponde alle domande di ControllaSolaLettura per testo SQL, e ricorda gli argomenti.
type lettoreFinto struct {
	risposte map[string]rigaFinta
	escluse  []string
	domande  []string
}

func (l *lettoreFinto) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	l.domande = append(l.domande, sql)
	if sql == sqlEscluseLeggibili && len(args) == 1 {
		l.escluse, _ = args[0].([]string)
	}
	r, ok := l.risposte[sql]
	if !ok {
		return rigaFinta{err: fmt.Errorf("domanda inattesa: %s", sql)}
	}
	return r
}

func (l *lettoreFinto) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("ControllaSolaLettura non usa Query")
}

// collegamentoBuono: il ruolo del banco come deve essere, con nomi inventati.
func collegamentoBuono() map[string]rigaFinta {
	return map[string]rigaFinta{
		sqlDefaultSolaLettura:  {valori: []any{"on"}},
		sqlUtenti:              {valori: []any{"lettore_acme", "lettore_acme", "acme_copia"}},
		sqlAttributiRuolo:      {valori: []any{false, false, false, false}},
		sqlAppartenenze:        {valori: []any{[]string{}}},
		sqlRelazioniScrivibili: {valori: []any{[]string{}}},
		sqlCreateSulloSchema:   {valori: []any{false}},
		sqlEscluseLeggibili:    {valori: []any{[]string{}}},
	}
}

func TestControllaSolaLetturaUnControlloPerVolta(t *testing.T) {
	ctx := context.Background()
	escluse := []string{"tabella_esclusa_a", "tabella_esclusa_b"}

	l := &lettoreFinto{risposte: collegamentoBuono()}
	col, err := ControllaSolaLettura(ctx, l, escluse)
	if err != nil {
		t.Fatalf("il collegamento buono è rifiutato: %v", err)
	}
	if col.Utente != "lettore_acme" || col.Database != "acme_copia" || !col.SolaLettura || col.Superutente || col.Scrive ||
		len(col.Ruoli) != 0 || len(col.EscluseLeggibili) != 0 {
		t.Errorf("collegamento letto male: %+v", col)
	}
	if !reflect.DeepEqual(l.escluse, escluse) {
		t.Errorf("le tabelle escluse passate alla domanda: %v, attese %v", l.escluse, escluse)
	}
	if len(l.domande) != 7 {
		t.Errorf("domande fatte: %d, attese 7 (una per controllo)", len(l.domande))
	}

	casi := []struct {
		nome   string
		sql    string
		riga   rigaFinta
		frase  string
		verifi func(Collegamento) bool
	}{
		{"default_transaction_read_only spento", sqlDefaultSolaLettura, rigaFinta{valori: []any{"off"}}, "default_transaction_read_only",
			func(c Collegamento) bool { return !c.SolaLettura }},
		{"SET ROLE", sqlUtenti, rigaFinta{valori: []any{"altro_ruolo", "lettore_acme", "acme_copia"}}, "SET ROLE", nil},
		{"superutente", sqlAttributiRuolo, rigaFinta{valori: []any{true, false, false, false}}, "rolsuper",
			func(c Collegamento) bool { return c.Superutente }},
		{"crea database", sqlAttributiRuolo, rigaFinta{valori: []any{false, true, false, false}}, "rolcreatedb", nil},
		{"crea ruoli", sqlAttributiRuolo, rigaFinta{valori: []any{false, false, true, false}}, "rolcreaterole", nil},
		{"scavalca la sicurezza di riga", sqlAttributiRuolo, rigaFinta{valori: []any{false, false, false, true}}, "rolbypassrls", nil},
		{"membro di un ruolo", sqlAppartenenze, rigaFinta{valori: []any{[]string{"pg_read_all_data"}}}, "pg_auth_members",
			func(c Collegamento) bool { return len(c.Ruoli) == 1 }},
		{"scrive su una tabella", sqlRelazioniScrivibili, rigaFinta{valori: []any{[]string{"tabella_acme"}}}, "privilegi di scrittura",
			func(c Collegamento) bool { return c.Scrive }},
		{"CREATE sullo schema", sqlCreateSulloSchema, rigaFinta{valori: []any{true}}, "CREATE sullo schema public",
			func(c Collegamento) bool { return c.Scrive }},
		{"legge una tabella esclusa", sqlEscluseLeggibili, rigaFinta{valori: []any{[]string{"tabella_esclusa_b"}}}, "tabelle escluse tabella_esclusa_b",
			func(c Collegamento) bool { return len(c.EscluseLeggibili) == 1 }},
		{"la domanda fallisce", sqlAppartenenze, rigaFinta{err: errors.New("errore finto")}, "appartenenze", nil},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			risposte := collegamentoBuono()
			risposte[c.sql] = c.riga
			col, err := ControllaSolaLettura(ctx, &lettoreFinto{risposte: risposte}, escluse)
			if err == nil {
				t.Fatalf("accettato: %+v", col)
			}
			if !strings.Contains(err.Error(), c.frase) {
				t.Errorf("l'errore non nomina il controllo (%q): %v", c.frase, err)
			}
			if c.verifi != nil && !c.verifi(col) {
				t.Errorf("il collegamento non dice che cosa ha trovato: %+v", col)
			}
		})
	}
}

// TestLetturaNonMigra: lettura.go serve ad aprire, non a migrare (P-02, G10). Non dichiara ApriSenzaMigrare,
// che resta in app/runtime, e non chiama nessuna delle funzioni che applicano le migrazioni.
func TestLetturaNonMigra(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "lettura.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	vietate := map[string]bool{"Applica": true, "ApplicaFinoA": true, "ApplicaElenco": true, "applicaUna": true}
	funzioni := 0
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			funzioni++
			if fn.Name.Name == "ApriSenzaMigrare" || fn.Name.Name == "ApriDatabaseSenzaMigrare" {
				t.Errorf("lettura.go dichiara %s: resta in app/runtime (P-02)", fn.Name.Name)
			}
		}
	}
	if funzioni == 0 {
		t.Fatal("nessuna funzione in lettura.go: la prova non legge il file giusto")
	}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		nome := ""
		switch x := c.Fun.(type) {
		case *ast.Ident:
			nome = x.Name
		case *ast.SelectorExpr:
			nome = x.Sel.Name
		}
		if vietate[nome] {
			t.Errorf("lettura.go chiama %s: l'apertura in lettura non migra (P-02, G10)", nome)
		}
		return true
	})
}
