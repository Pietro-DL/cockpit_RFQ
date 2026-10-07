package testutil

// dump.go: la copia intatta del dump nelle prove L4 private (piano A, A1c, 3.3.9 e 6.4.7; R10, R44). La copia si
// legge soltanto, dal ruolo del banco: prima di dare il pool si controlla che sia proprio quella del manifest, e
// dopo la prova che le sue righe e i loro valori non siano cambiati (le sentinelle contano le righe, le impronte
// di contenuto vedono i valori: R117 b). Nessun valore della copia sta nel codice: nome, ruolo, schema, tabelle
// escluse, sentinelle e impronte vengono dal manifest privato (dataset.CopiaAttesa).

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

	"github.com/jackc/pgx/v5"
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
//   - in t.Cleanup ricontrolla le sentinelle e, se il manifest le dichiara, le impronte: se sono cambiate, la
//     prova fallisce (qualcuno ha scritto).
//
// Uno schema diverso da quello del binario, una copia diversa da quella del manifest o un ruolo che può
// scrivere: NON ESEGUITA, mai un salto e mai un fallimento del codice. Una copia giusta, ma senza le impronte
// dichiarate, o con una tabella che ha una colonna di un tipo che la versione 1 non rende: la parte delle impronte è
// NON ESEGUITA, con il motivo, e la prova va avanti senza essere mai verde (esitoDiControllaCopia; R117 b, R-141).
// Le prove che lo usano hanno sempre il tag privato (R44). Non migra, non semina, non svuota.
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
	esitoDiControllaCopia(t, ControllaCopia(ctx, leggi, attesa))
	if _, err := migrazioni.ControllaSolaLettura(ctx, p, attesa.Escluse); err != nil {
		NonEseguita(t, "il collegamento alla copia del dump non è in sola lettura: "+err.Error())
	}
	t.Cleanup(func() {
		if err := ricontrollaCopia(context.Background(), leggi, attesa); err != nil {
			t.Errorf("la copia del dump è cambiata durante la prova: %v", err)
		}
	})
	return p
}

// esitoDiControllaCopia: che cosa ne è della prova dopo ControllaCopia. nil: niente. ErrImpronteNonDichiarate o
// ErrTipoNonReso (tutti gli altri controlli sono passati, e nessun valore è cambiato): la parte delle impronte è NON
// ESEGUITA, con il motivo, senza fermare la prova (parteNonEseguita), che così non è mai verde (R117 b, R-141). Ogni
// altro errore: NON ESEGUITA, e la prova si ferma.
func esitoDiControllaCopia(t testing.TB, err error) {
	t.Helper()
	switch {
	case err == nil:
	case errors.Is(err, ErrImpronteNonDichiarate), errors.Is(err, ErrTipoNonReso):
		parteNonEseguita(t, err.Error())
	default:
		NonEseguita(t, "la copia del dump non è quella del manifest: "+err.Error())
	}
}

// ricontrollaCopia: il ricontrollo di t.Cleanup. Le sentinelle sempre; le impronte se il manifest le dichiara (se
// non le dichiara, la parte è già NON ESEGUITA all'inizio, e non si ripete). Le tabelle con un tipo non reso dalla
// versione 1 sono già NON ESEGUITE all'inizio anche loro: quell'errore non si ripete, i valori cambiati sì.
func ricontrollaCopia(ctx context.Context, leggi LeggiValore, attesa dataset.CopiaAttesa) error {
	if err := controllaSentinelle(ctx, leggi, attesa.Sentinelle); err != nil {
		return err
	}
	if attesa.Impronte == nil {
		return nil
	}
	if err := controllaImpronte(ctx, leggi, attesa); err != nil && !errors.Is(err, ErrTipoNonReso) {
		return err
	}
	return nil
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

// nomeTabella: un nome di tabella, o di colonna, che il manifest può dare (minuscole, cifre, trattino basso), così
// si scrive nel testo SQL senza rischi.
var nomeTabella = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func sqlLeggibile(tabella string) string {
	return `SELECT coalesce(has_table_privilege(to_regclass('public.` + tabella + `'), 'SELECT'), false)::text`
}

func sqlRighe(tabella string) string {
	return `SELECT count(*)::text FROM public.` + tabella
}

// ---- le impronte di contenuto (R117 b; E2 §2.10) ----
//
// L'SQL sta qui, accanto a quello dei conteggi, e gira solo nelle prove: il binario del banco non calcola mai
// un'impronta (T-B0-36, nessun SQL nuovo nel prodotto). La formula è la versione 1 (VersioneImpronta), per una
// tabella con le colonne c1…cn e l'ordine o1…ok dichiarati nel manifest:
//   - ogni riga diventa un array JSON compatto, «[v1,v2,…,vn]», con vi = to_json(ci)::text e «null» per NULL. Per
//     date e timestamp to_json scrive sempre ISO 8601, qualunque sia DateStyle; i timestamptz e i timetz si rendono
//     prima in UTC (AT TIME ZONE 'UTC'), perché il loro testo dipende dal TimeZone della sessione;
//   - la chiave di una riga è lo stesso array con le sole colonne dell'ordine;
//   - le righe si ordinano per chiave e poi per riga intera, tutte e due con la collazione "C": l'ordine non dipende
//     dalla collazione del database né dall'ordine fisico delle righe, e due righe con la stessa chiave hanno sempre
//     lo stesso posto;
//   - ogni riga dà il suo sha256, dei byte UTF-8 della riga, in esadecimale minuscolo; il testo è l'unione di questi
//     sha256, nell'ordine delle righe, con un a capo, e l'impronta è lo sha256 dei byte del testo, in esadecimale
//     minuscolo. Così lo stato dell'aggregato resta di 65 byte per riga, qualunque sia la grandezza dei valori (un
//     testo di PostgreSQL non supera 1 GB); una tabella vuota ha lo sha256 del testo vuoto.
// I tipi resi sono un elenco chiuso (tipiStabili, gli enum, tipiConFuso), e si sbaglia per difetto (R-141):
//   - testo, con to_json: bool, int2, int4, int8, numeric, text, varchar, bpchar, uuid, json, jsonb, date,
//     timestamp, inet, gli enum, e gli array di questi tipi;
//   - fuso, in UTC: timestamptz e timetz;
//   - un dominio vale per il suo tipo di base, a un livello solo.
// Ogni altro tipo la versione 1 non lo rende: quelli il cui testo dipende dalla sessione (float4 e float8 da
// extra_float_digits, interval da IntervalStyle, bytea da bytea_output, money da lc_monetary, range e multirange
// dal DateStyle e dal TimeZone, compositi e array con un tempo con il fuso dentro) e quelli che la formula non
// conosce (un dominio su un dominio, time, xml, i geometrici, …). Una colonna così dà l'errore «tipo non reso dalla
// versione 1» (ErrTipoNonReso), che la nomina, e l'impronta di quella tabella è una parte non eseguita: mai
// un'impronta che cambia con la sessione. Le impronte delle altre tabelle si controllano lo stesso.

// VersioneImpronta: la versione della formula delle impronte, che il manifest dichiara in
// copia.impronte.versione_impronta. Una formula diversa vuole una versione nuova e le impronte ricalcolate, con la
// riga di storico: un'impronta di un'altra versione non si confronta.
const VersioneImpronta = 1

// ErrImpronteNonDichiarate: la copia passa tutti gli altri controlli, ma il manifest non dichiara le sue impronte.
// È una parte non eseguita, con il motivo, mai un verde: i soli conteggi non vedono un valore cambiato in una riga
// esistente (R117). PoolDump la segna NON ESEGUITA senza fermare la prova.
var ErrImpronteNonDichiarate = errors.New("impronte: il manifest non le dichiara per la copia intatta, e la parte delle impronte " +
	"non è eseguita: i soli conteggi delle sentinelle non vedono un valore cambiato in una riga esistente (R117 b)")

// ErrTipoNonReso: una colonna dichiarata ha un tipo che la versione 1 non rende in modo stabile (R-141). L'impronta
// di quella tabella è una parte non eseguita, con il motivo, mai un verde; PoolDump la segna NON ESEGUITA senza
// fermare la prova, come ErrImpronteNonDichiarate.
var ErrTipoNonReso = errors.New("tipo non reso dalla versione 1")

// Le classi delle colonne nella formula (sqlClassiColonne).
const (
	classeTesto    = "testo"    // resa con to_json
	classeFuso     = "fuso"     // timestamptz o timetz, resa in UTC
	classeSessione = "sessione" // fuori dall'elenco dei tipi resi (il testo dipende dalla sessione, o la formula non lo conosce)
)

// I tipi con il fuso, resi in UTC, e quelli il cui testo, con to_json, non dipende dalla sessione. Gli enum si
// riconoscono dal typtype. Ogni altro tipo è fuori (classeSessione).
const (
	tipiConFuso = `'timestamptz'::regtype, 'timetz'::regtype`
	tipiStabili = `'bool'::regtype, 'int2'::regtype, 'int4'::regtype, 'int8'::regtype, 'numeric'::regtype, 'text'::regtype, ` +
		`'varchar'::regtype, 'bpchar'::regtype, 'uuid'::regtype, 'json'::regtype, 'jsonb'::regtype, 'date'::regtype, ` +
		`'timestamp'::regtype, 'inet'::regtype`
)

// sqlClassiColonne: per le colonne dichiarate che la tabella ha, «nome:classe» separati da uno spazio, in ordine di
// nome. b è il tipo della colonna, o la base del suo dominio (un livello); eb, per un array, il tipo dei suoi
// elementi, o la base del loro dominio (un livello). Tutto ciò che non è nell'elenco è classeSessione: un dominio su
// un dominio (b, o eb, è ancora un dominio), un array con elementi fuori dall'elenco, anche con il fuso, e ogni altro
// tipo. Una colonna che manca non c'è nella risposta; una tabella che manca dà una risposta vuota. Nomi già
// controllati (nomeTabella).
func sqlClassiColonne(tabella string, colonne []string) string {
	return `SELECT coalesce(string_agg(a.attname::text || ':' || CASE` +
		` WHEN b.oid IN (` + tipiConFuso + `) THEN '` + classeFuso + `'` +
		` WHEN b.oid IN (` + tipiStabili + `) OR b.typtype = 'e' THEN '` + classeTesto + `'` +
		` WHEN b.typcategory = 'A' AND (eb.oid IN (` + tipiStabili + `) OR eb.typtype = 'e') THEN '` + classeTesto + `'` +
		` ELSE '` + classeSessione + `' END, ' ' ORDER BY a.attname::text COLLATE "C"), '')` +
		` FROM pg_attribute a JOIN pg_type t ON t.oid = a.atttypid` +
		` JOIN pg_type b ON b.oid = CASE WHEN t.typtype = 'd' THEN t.typbasetype ELSE t.oid END` +
		` LEFT JOIN pg_type e ON e.oid = b.typelem AND b.typcategory = 'A'` +
		` LEFT JOIN pg_type eb ON eb.oid = CASE WHEN e.typtype = 'd' THEN e.typbasetype ELSE e.oid END` +
		` WHERE a.attrelid = to_regclass('public.` + tabella + `') AND a.attnum > 0 AND NOT a.attisdropped` +
		` AND a.attname::text = ANY (ARRAY['` + strings.Join(colonne, `', '`) + `']::text[])`
}

// sqlImpronta: «<righe> <sha256>» di una tabella, con la formula della versione 1. classi viene da
// sqlClassiColonne; nomi già controllati.
func sqlImpronta(tabella string, colonne, ordine []string, classi map[string]string) string {
	return `SELECT count(*)::text || ' ' || encode(sha256(convert_to(coalesce(string_agg(encode(sha256(convert_to(r.riga, 'UTF8')), 'hex'), chr(10)` +
		` ORDER BY r.chiave COLLATE "C", r.riga COLLATE "C"), ''), 'UTF8')), 'hex')` +
		` FROM (SELECT ` + arrayJSON(colonne, classi) + ` AS riga, ` + arrayJSON(ordine, classi) + ` AS chiave` +
		` FROM public.` + tabella + ` x) r`
}

// arrayJSON: l'espressione SQL dell'array JSON compatto dei valori delle colonne, nell'ordine dato.
func arrayJSON(colonne []string, classi map[string]string) string {
	parti := make([]string, len(colonne))
	for i, c := range colonne {
		v := "x." + pgx.Identifier{c}.Sanitize()
		if classi[c] == classeFuso {
			v = "(" + v + " AT TIME ZONE 'UTC')"
		}
		parti[i] = "coalesce(to_json(" + v + ")::text, 'null')"
	}
	return "'[' || " + strings.Join(parti, " || ',' || ") + " || ']'"
}

// ControllaCopia controlla che il collegamento sia proprio la copia intatta del manifest (piano A, 6.4.7), e si
// ferma al primo controllo che non passa, con un errore che lo nomina:
//   - l'utente è il Ruolo del manifest;
//   - current_database() è il Database del manifest, senza «test» e diverso dal database di COCKPIT_TEST_DSN;
//   - SHOW transaction_read_only e default_transaction_read_only = on;
//   - client_encoding = UTF8;
//   - schema_versione con count, min e max uguali a 1…Schema, e Schema uguale all'ultima migrazione del binario;
//   - le Escluse illeggibili;
//   - le Sentinelle uguali (righe per tabella);
//   - le Impronte uguali (il contenuto delle tabelle dichiarate, R117 b): a righe uguali, un'impronta diversa è
//     l'errore «valori cambiati».
//
// Un manifest senza ruolo, database, schema o sentinelle non descrive una copia: è un errore anche quello, come
// una sezione impronte che non si sa calcolare. Un manifest senza impronte, con tutto il resto che passa, dà
// ErrImpronteNonDichiarate: una parte non eseguita, mai nil. Una tabella con una colonna di un tipo che la versione 1
// non rende dà ErrTipoNonReso, dopo le altre tabelle: anche questa una parte non eseguita, mai nil (R-141).
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
	if err := controllaSentinelle(ctx, leggi, attesa.Sentinelle); err != nil {
		return err
	}
	return controllaImpronte(ctx, leggi, attesa)
}

// controllaAttesa: il manifest descrive davvero una copia, e le sue impronte, se ci sono, si sanno calcolare.
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
	return controllaDichiarazioneImpronte(attesa.Impronte, attesa.Sentinelle)
}

// improntaEsadecimale: uno sha256 in esadecimale; le maiuscole non cambiano il valore, come nelle voci del manifest.
var improntaEsadecimale = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// controllaDichiarazioneImpronte: la sezione impronte, se c'è, descrive impronte che questo codice sa calcolare
// (dataset ne legge solo la forma): la versione della formula; almeno una tabella; per ogni tabella, in ordine di
// nome, un nome valido con la sua sentinella (l'impronta sta accanto al conteggio, non al suo posto: è così che
// «a righe uguali» si dice), colonne valide e senza ripetizioni, un ordine non vuoto fatto di colonne dichiarate e
// senza ripetizioni, uno sha256 di 64 cifre esadecimali. Senza la sezione: nil, e ci pensa controllaImpronte.
func controllaDichiarazioneImpronte(imp *dataset.ImpronteCopia, sentinelle map[string]int64) error {
	if imp == nil {
		return nil
	}
	if imp.Versione != VersioneImpronta {
		return fmt.Errorf("manifest: impronte: versione_impronta %d, questo codice calcola la %d", imp.Versione, VersioneImpronta)
	}
	if len(imp.Tabelle) == 0 {
		return errors.New("manifest: impronte: nessuna tabella")
	}
	for _, tab := range tabelleInOrdine(imp.Tabelle) {
		d := imp.Tabelle[tab]
		_, conSentinella := sentinelle[tab]
		switch {
		case !nomeTabella.MatchString(tab):
			return fmt.Errorf("manifest: impronte: %q non è un nome di tabella", tab)
		case !conSentinella:
			return fmt.Errorf("manifest: impronte: %s non ha la sentinella delle righe: l'impronta sta accanto al conteggio", tab)
		case len(d.Colonne) == 0:
			return fmt.Errorf("manifest: impronte: %s non dichiara le colonne", tab)
		case len(d.Ordine) == 0:
			return fmt.Errorf("manifest: impronte: %s non dichiara l'ordine", tab)
		case !improntaEsadecimale.MatchString(d.Sha256):
			return fmt.Errorf("manifest: impronte: %s: lo sha256 vuole 64 cifre esadecimali", tab)
		}
		colonne := map[string]bool{}
		for _, c := range d.Colonne {
			switch {
			case !nomeTabella.MatchString(c):
				return fmt.Errorf("manifest: impronte: %s: %q non è un nome di colonna", tab, c)
			case colonne[c]:
				return fmt.Errorf("manifest: impronte: %s: la colonna %s compare due volte", tab, c)
			}
			colonne[c] = true
		}
		ordine := map[string]bool{}
		for _, o := range d.Ordine {
			switch {
			case !colonne[o]:
				return fmt.Errorf("manifest: impronte: %s: l'ordine usa %q, che non è fra le colonne", tab, o)
			case ordine[o]:
				return fmt.Errorf("manifest: impronte: %s: la colonna %s compare due volte nell'ordine", tab, o)
			}
			ordine[o] = true
		}
	}
	return nil
}

// tabelleInOrdine: i nomi delle tabelle delle impronte, in ordine di nome (nessun ordine dipende da una mappa).
func tabelleInOrdine(tabelle map[string]dataset.ImprontaTabella) []string {
	nomi := make([]string, 0, len(tabelle))
	for tab := range tabelle {
		nomi = append(nomi, tab)
	}
	sort.Strings(nomi)
	return nomi
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

// controllaImpronte: le impronte di contenuto delle tabelle dichiarate, in ordine di nome, uguali al manifest (R117
// b). Viene dopo controllaSentinelle, e ogni tabella con l'impronta ha la sua sentinella: a righe uguali,
// un'impronta diversa vuol dire valori cambiati in righe esistenti, l'errore «valori cambiati». Le tabelle cambiate
// si dicono tutte, con l'impronta trovata e quella del manifest, per la diagnosi: il manifest non si aggiorna mai
// con il valore appena trovato, e un'impronta cambiata vuole una riga di storico con il motivo. Senza la sezione
// impronte: ErrImpronteNonDichiarate.
//
// Una tabella con una colonna di un tipo che la versione 1 non rende (ErrTipoNonReso) non ferma le altre: la sua
// impronta è una parte non eseguita (R-141). Alla fine i valori cambiati vincono (sono una differenza vera, e
// l'errore nomina anche le tabelle non eseguite); senza valori cambiati, l'errore avvolge ErrTipoNonReso e nomina le
// tabelle e le colonne.
func controllaImpronte(ctx context.Context, leggi LeggiValore, attesa dataset.CopiaAttesa) error {
	if attesa.Impronte == nil {
		return ErrImpronteNonDichiarate
	}
	var cambiate, nonEseguite []string
	for _, tab := range tabelleInOrdine(attesa.Impronte.Tabelle) {
		d := attesa.Impronte.Tabelle[tab]
		righe, impronta, err := improntaDellaTabella(ctx, leggi, tab, d)
		if errors.Is(err, ErrTipoNonReso) {
			nonEseguite = append(nonEseguite, fmt.Sprintf("%s (%v)", tab, err))
			continue
		}
		if err != nil {
			return fmt.Errorf("impronte: %s: %w", tab, err)
		}
		if attese := strconv.FormatInt(attesa.Sentinelle[tab], 10); righe != attese {
			return fmt.Errorf("impronte: %s ha %s righe, il manifest ne dice %s: le righe sono cambiate fra un controllo e l'altro", tab, righe, attese)
		}
		if dichiarata := strings.ToLower(d.Sha256); impronta != dichiarata {
			cambiate = append(cambiate, fmt.Sprintf("%s (%s righe, come il manifest; impronta %s, il manifest dice %s)", tab, righe, impronta, dichiarata))
		}
	}
	switch {
	case len(cambiate) > 0 && len(nonEseguite) > 0:
		return fmt.Errorf("impronte: valori cambiati: %s; e senza impronta, parte non eseguita: %s",
			strings.Join(cambiate, "; "), strings.Join(nonEseguite, "; "))
	case len(cambiate) > 0:
		return fmt.Errorf("impronte: valori cambiati: %s", strings.Join(cambiate, "; "))
	case len(nonEseguite) > 0:
		return fmt.Errorf("impronte: l'impronta di %d tabelle è una parte non eseguita: %w: %s", len(nonEseguite), ErrTipoNonReso,
			strings.Join(nonEseguite, "; "))
	}
	return nil
}

// improntaDellaTabella: righe e impronta di una tabella, con le colonne e l'ordine dichiarati. Prima legge dal
// catalogo la classe di ogni colonna; una colonna che la tabella non ha è un errore che la nomina, una colonna che la
// versione 1 non rende è ErrTipoNonReso, con le colonne, e l'impronta non si calcola. Le risposte del database si
// controllano: una forma inattesa è un errore, mai un'impronta.
func improntaDellaTabella(ctx context.Context, leggi LeggiValore, tabella string, d dataset.ImprontaTabella) (righe, impronta string, err error) {
	v, err := leggi(ctx, sqlClassiColonne(tabella, d.Colonne))
	if err != nil {
		return "", "", fmt.Errorf("le classi delle colonne: %w", err)
	}
	classi := map[string]string{}
	for _, voce := range strings.Fields(v) {
		nome, classe, ok := strings.Cut(voce, ":")
		if !ok {
			return "", "", fmt.Errorf("le classi delle colonne: risposta %q", v)
		}
		classi[nome] = classe
	}
	var mancanti, nonRese []string
	for _, c := range d.Colonne {
		switch classi[c] {
		case classeTesto, classeFuso:
		case classeSessione:
			nonRese = append(nonRese, c)
		case "":
			mancanti = append(mancanti, c)
		default:
			return "", "", fmt.Errorf("le classi delle colonne: %s ha la classe %q", c, classi[c])
		}
	}
	switch {
	case len(mancanti) > 0:
		return "", "", fmt.Errorf("la tabella non c'è, o non ha le colonne %s", strings.Join(mancanti, ", "))
	case len(nonRese) > 0:
		return "", "", fmt.Errorf("le colonne %s: %w (il loro testo dipende dalla sessione, o la formula non conosce il tipo): "+
			"l'impronta della tabella non si calcola; vanno tolte dalle colonne dichiarate, o serve una versione nuova",
			strings.Join(nonRese, ", "), ErrTipoNonReso)
	}
	r, err := leggi(ctx, sqlImpronta(tabella, d.Colonne, d.Ordine, classi))
	if err != nil {
		return "", "", fmt.Errorf("l'impronta: %w", err)
	}
	righe, impronta, ok := strings.Cut(r, " ")
	if _, errN := strconv.ParseInt(righe, 10, 64); !ok || errN != nil || !improntaMinuscola.MatchString(impronta) {
		return "", "", fmt.Errorf("l'impronta: risposta %q", r)
	}
	return righe, impronta, nil
}

// improntaMinuscola: lo sha256 come lo scrive encode(…, 'hex').
var improntaMinuscola = regexp.MustCompile(`^[0-9a-f]{64}$`)
