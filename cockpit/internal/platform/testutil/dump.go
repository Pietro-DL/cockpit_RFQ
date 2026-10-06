package testutil

// dump.go: la copia intatta del dump nelle prove L4 private (piano A, A1c, 3.3.9 e 6.4.7; R10, R44). La copia si
// legge soltanto, dal ruolo del banco: prima di dare il pool si controlla che sia proprio quella del manifest, e
// dopo la prova che le sue righe non siano cambiate. Nessun valore della copia sta nel codice: nome, ruolo,
// schema, tabelle escluse e sentinelle vengono dal manifest privato (dataset.CopiaAttesa).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/dataset"
	"promatec/cockpit/internal/platform/migrazioni"
)

// Le variabili delle copie del dump (piano A, 3.7.3).
const (
	variabileDump      = "COCKPIT_DUMP_DSN"
	variabileDumpCopia = "COCKPIT_DUMP_COPIA_DSN"
	variabileTest      = "COCKPIT_TEST_DSN"
)

// LeggiValore fa una domanda al database e torna il primo valore come testo: è come gli aiuti della copia
// parlano con il database, così le prove L1 la sostituiscono con un lettore finto.
type LeggiValore func(ctx context.Context, sql string) (string, error)

// leggiDalPool: LeggiValore su un pool vero.
func leggiDalPool(p *pgxpool.Pool) LeggiValore {
	return func(ctx context.Context, sql string) (string, error) {
		var s string
		err := p.QueryRow(ctx, sql).Scan(&s)
		return s, err
	}
}

// DumpDSN restituisce COCKPIT_DUMP_DSN, dopo il controllo del nome (CopiaDelDump). Senza la variabile, o con un
// DSN che porta a un database che non può essere la copia intatta: NON ESEGUITA, mai un salto (R44).
func DumpDSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv(variabileDump)
	if dsn == "" {
		NonEseguita(t, variabileDump+" non impostata: serve la copia intatta del dump, letta dal ruolo del banco, senza password (pgpass)")
	}
	if err := CopiaDelDump(dsn); err != nil {
		NonEseguita(t, err.Error())
	}
	return dsn
}

// CopiaDelDump controlla, prima di collegarsi, il database a cui porta il DSN della copia intatta, con il nome
// risolto come lo leggerà pgx: rifiuta un nome con «test» (è il database di prova, che le prove azzerano), con
// «dev» (lo sviluppo) o con «prod» (la produzione), e un nome uguale al database di COCKPIT_TEST_DSN (R10).
// L'errore non ripete il DSN.
func CopiaDelDump(dsn string) error {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return errors.New(variabileDump + ": il DSN non si legge (non si ripete: può contenere la password)")
	}
	nome := cfg.Database
	if strings.TrimSpace(nome) == "" {
		return errors.New(variabileDump + ": il DSN non porta a nessun database")
	}
	basso := strings.ToLower(nome)
	for _, vietata := range []string{"test", "dev", "prod"} {
		if strings.Contains(basso, vietata) {
			return fmt.Errorf("%s porta a %q, che ha «%s» nel nome: non può essere la copia intatta del dump", variabileDump, nome, vietata)
		}
	}
	if delTest := databaseDi(os.Getenv(variabileTest)); delTest != "" && delTest == nome {
		return fmt.Errorf("%s porta allo stesso database di %s (%q): la copia del dump non è mai il database di prova (R10)", variabileDump, variabileTest, nome)
	}
	return nil
}

// databaseDi: il nome del database di un DSN, come lo leggerà pgx; "" se il DSN è vuoto o non si legge.
func databaseDi(dsn string) string {
	if dsn == "" {
		return ""
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return ""
	}
	return cfg.Database
}

// PoolDump apre COCKPIT_DUMP_DSN con migrazioni.ApriInLettura (pool in sola lettura, schema del binario), poi:
//   - ControllaCopia contro la CopiaAttesa del manifest;
//   - migrazioni.ControllaSolaLettura, con le tabelle escluse del manifest;
//   - in t.Cleanup ricontrolla le sentinelle: se sono cambiate, la prova fallisce (qualcuno ha scritto).
//
// Uno schema diverso da quello del binario, una copia diversa da quella del manifest o un ruolo che può
// scrivere: NON ESEGUITA, mai un salto e mai un fallimento del codice. Le prove che lo usano hanno sempre il tag
// privato (R44). Non migra, non semina, non svuota.
func PoolDump(t testing.TB, attesa dataset.CopiaAttesa) *pgxpool.Pool {
	t.Helper()
	dsn := DumpDSN(t)
	ctx := context.Background()
	p, err := migrazioni.ApriInLettura(ctx, dsn, risorse.FS)
	if err != nil {
		var sd *migrazioni.SchemaDiverso
		if errors.As(err, &sd) {
			NonEseguita(t, fmt.Sprintf("la copia del dump è alla versione %d dello schema, il binario alla %d: la copia intatta non si "+
				"migra mai; serve una copia TEMPLATE migrata dal proprietario, o un binario di questo schema", sd.DelDatabase, sd.DelBinario))
		}
		NonEseguita(t, "la copia del dump non si apre in sola lettura: "+err.Error())
	}
	t.Cleanup(p.Close)
	leggi := leggiDalPool(p)
	if err := ControllaCopia(ctx, leggi, attesa); err != nil {
		NonEseguita(t, "la copia del dump non è quella del manifest: "+err.Error())
	}
	if _, err := migrazioni.ControllaSolaLettura(ctx, p, attesa.Escluse); err != nil {
		NonEseguita(t, "il collegamento alla copia del dump non è in sola lettura: "+err.Error())
	}
	t.Cleanup(func() {
		if err := controllaSentinelle(context.Background(), leggi, attesa.Sentinelle); err != nil {
			t.Errorf("la copia del dump è cambiata durante la prova: %v", err)
		}
	})
	return p
}

// Le domande di ControllaCopia e di controllaCopiaScrivibile: una riga, un valore di testo.
const (
	sqlUtenteCorrente      = `SELECT current_user::text`
	sqlDatabaseCorrente    = `SELECT current_database()::text`
	sqlTransazioneRO       = `SHOW transaction_read_only`
	sqlDefaultRO           = `SHOW default_transaction_read_only`
	sqlCodifica            = `SHOW client_encoding`
	sqlVersioniSchema      = `SELECT count(*)::text || ' ' || coalesce(min(versione), 0)::text || ' ' || coalesce(max(versione), 0)::text FROM schema_versione`
	sqlCommentoDelDatabase = `SELECT coalesce(shobj_description(d.oid, 'pg_database'), '')::text FROM pg_database d WHERE d.datname = current_database()`
)

// nomeTabella: un nome di tabella che il manifest può dare (minuscole, cifre, trattino basso), così si scrive
// nel testo SQL senza rischi.
var nomeTabella = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func sqlLeggibile(tabella string) string {
	return `SELECT coalesce(has_table_privilege(to_regclass('public.` + tabella + `'), 'SELECT'), false)::text`
}

func sqlRighe(tabella string) string {
	return `SELECT count(*)::text FROM public.` + tabella
}

// ControllaCopia controlla che il collegamento sia proprio la copia intatta del manifest (piano A, 6.4.7), e si
// ferma al primo controllo che non passa, con un errore che lo nomina:
//   - l'utente è il Ruolo del manifest;
//   - current_database() è il Database del manifest, senza «test» e diverso dal database di COCKPIT_TEST_DSN;
//   - SHOW transaction_read_only e default_transaction_read_only = on;
//   - client_encoding = UTF8;
//   - schema_versione con count, min e max uguali a 1…Schema, e Schema uguale all'ultima migrazione del binario;
//   - le Escluse illeggibili;
//   - le Sentinelle uguali (righe per tabella).
//
// Un manifest senza ruolo, database, schema o sentinelle non descrive una copia: è un errore anche quello.
func ControllaCopia(ctx context.Context, leggi LeggiValore, attesa dataset.CopiaAttesa) error {
	if err := controllaAttesa(attesa); err != nil {
		return err
	}
	utente, err := leggi(ctx, sqlUtenteCorrente)
	if err != nil {
		return fmt.Errorf("utente: %w", err)
	}
	if utente != attesa.Ruolo {
		return fmt.Errorf("utente: collegato come %q, il manifest dice %q", utente, attesa.Ruolo)
	}
	if err := controllaNomeCopia(ctx, leggi, attesa.Database); err != nil {
		return err
	}
	for _, c := range []struct{ sql, nome string }{{sqlTransazioneRO, "transaction_read_only"}, {sqlDefaultRO, "default_transaction_read_only"}} {
		v, err := leggi(ctx, c.sql)
		if err != nil {
			return fmt.Errorf("sola lettura: %s: %w", c.nome, err)
		}
		if v != "on" {
			return fmt.Errorf("sola lettura: %s è %q, serve «on»", c.nome, v)
		}
	}
	codifica, err := leggi(ctx, sqlCodifica)
	if err != nil {
		return fmt.Errorf("client_encoding: %w", err)
	}
	if !strings.EqualFold(codifica, "UTF8") {
		return fmt.Errorf("client_encoding: %q, serve UTF8 (PGCLIENTENCODING)", codifica)
	}
	if err := controllaSchemaCopia(ctx, leggi, attesa.Schema); err != nil {
		return err
	}
	for _, e := range attesa.Escluse {
		if !nomeTabella.MatchString(e) {
			return fmt.Errorf("escluse: %q non è un nome di tabella", e)
		}
		v, err := leggi(ctx, sqlLeggibile(e))
		if err != nil {
			return fmt.Errorf("escluse: %s: %w", e, err)
		}
		if v != "false" {
			return fmt.Errorf("escluse: il ruolo legge %s, che il manifest dichiara esclusa", e)
		}
	}
	return controllaSentinelle(ctx, leggi, attesa.Sentinelle)
}

// controllaAttesa: il manifest descrive davvero una copia.
func controllaAttesa(attesa dataset.CopiaAttesa) error {
	switch {
	case strings.TrimSpace(attesa.Ruolo) == "":
		return errors.New("manifest: la copia non dichiara il ruolo")
	case strings.TrimSpace(attesa.Database) == "":
		return errors.New("manifest: la copia non dichiara il database")
	case attesa.Schema <= 0:
		return errors.New("manifest: la copia non dichiara lo schema")
	case len(attesa.Sentinelle) == 0:
		return errors.New("manifest: la copia non dichiara le sentinelle")
	}
	return nil
}

// controllaNomeCopia: current_database() è quello del manifest, senza «test» e diverso dal database di prova.
func controllaNomeCopia(ctx context.Context, leggi LeggiValore, atteso string) error {
	nome, err := leggi(ctx, sqlDatabaseCorrente)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	switch {
	case nome != atteso:
		return fmt.Errorf("database: collegato a %q, il manifest dice %q", nome, atteso)
	case strings.Contains(strings.ToLower(nome), "test"):
		return fmt.Errorf("database: %q ha «test» nel nome: è il database di prova, non una copia del dump", nome)
	case nome == databaseDi(os.Getenv(variabileTest)):
		return fmt.Errorf("database: %q è il database di %s (R10)", nome, variabileTest)
	}
	return nil
}

// controllaSchemaCopia: schema_versione da 1 a schema senza buchi, e schema uguale all'ultima migrazione del
// binario.
func controllaSchemaCopia(ctx context.Context, leggi LeggiValore, schema int) error {
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	if delBinario := migs[len(migs)-1].Versione; schema != delBinario {
		return fmt.Errorf("schema: il manifest dice %d, il binario conosce la %d", schema, delBinario)
	}
	v, err := leggi(ctx, sqlVersioniSchema)
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	parti := strings.Fields(v)
	if len(parti) != 3 {
		return fmt.Errorf("schema: risposta %q", v)
	}
	var n [3]int
	for i, p := range parti {
		if n[i], err = strconv.Atoi(p); err != nil {
			return fmt.Errorf("schema: risposta %q", v)
		}
	}
	if n[0] != schema || n[1] != 1 || n[2] != schema {
		return fmt.Errorf("schema: schema_versione ha %d righe, da %d a %d; attese %d, da 1 a %d (versione diversa o con buchi)",
			n[0], n[1], n[2], schema, schema)
	}
	return nil
}

// controllaSentinelle: le righe di ogni tabella sentinella, in ordine di nome, uguali al manifest.
func controllaSentinelle(ctx context.Context, leggi LeggiValore, sentinelle map[string]int64) error {
	if len(sentinelle) == 0 {
		return errors.New("sentinelle: il manifest non ne dichiara")
	}
	tabelle := make([]string, 0, len(sentinelle))
	for tab := range sentinelle {
		tabelle = append(tabelle, tab)
	}
	sort.Strings(tabelle)
	for _, tab := range tabelle {
		if !nomeTabella.MatchString(tab) {
			return fmt.Errorf("sentinelle: %q non è un nome di tabella", tab)
		}
		v, err := leggi(ctx, sqlRighe(tab))
		if err != nil {
			return fmt.Errorf("sentinelle: %s: %w", tab, err)
		}
		if v != strconv.FormatInt(sentinelle[tab], 10) {
			return fmt.Errorf("sentinelle: %s ha %s righe, il manifest ne dice %d", tab, v, sentinelle[tab])
		}
	}
	return nil
}
