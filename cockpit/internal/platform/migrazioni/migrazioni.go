// Package migrazioni applica in ordine i file migrations/NNNN_nome.sql incorporati nel binario
// (piano di correzione, voce 0.2: S5, N42).
//
// Regole:
//   - un file = una transazione; se fallisce a metà non resta nulla (S3);
//   - ogni file termina con INSERT INTO schema_versione (versione) VALUES (N) e il migratore verifica
//     che la riga esista prima del COMMIT: un file che dimentica di registrarsi non viene applicato;
//   - un file che aggiunge un valore a un enum (ALTER TYPE ... ADD VALUE) non lo usa nello stesso file,
//     perché PostgreSQL non permette di usare il valore prima del commit (S4);
//   - ogni REFERENCES punta a una tabella creata nello stesso file o in uno precedente (S1, N43);
//   - un lock consultivo di sessione evita che due istanze del server migrino insieme.
package migrazioni

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrazione è un file migrations/NNNN_nome.sql letto dal filesystem incorporato.
type Migrazione struct {
	Versione int
	Nome     string // es. "0002_fondazioni.sql"
	SQL      string
}

// lockMigrazioni è la chiave del lock consultivo ("COCK" in ASCII).
const lockMigrazioni int64 = 0x434f434b

var reNome = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// Elenca legge migrations/*.sql, li ordina per versione e verifica che le versioni siano 1..n senza buchi.
func Elenca(fsys fs.FS) ([]Migrazione, error) {
	voci, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migrazioni: %w", err)
	}
	var out []Migrazione
	for _, v := range voci {
		if v.IsDir() {
			continue
		}
		m := reNome.FindStringSubmatch(v.Name())
		if m == nil {
			return nil, fmt.Errorf("migrazioni: nome non valido %q (atteso NNNN_nome.sql)", v.Name())
		}
		n, _ := strconv.Atoi(m[1])
		b, err := fs.ReadFile(fsys, path.Join("migrations", v.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, Migrazione{Versione: n, Nome: v.Name(), SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Versione < out[j].Versione })
	for i, m := range out {
		if m.Versione != i+1 {
			return nil, fmt.Errorf("migrazioni: attesa la versione %04d, trovato %s", i+1, m.Nome)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("migrazioni: nessun file in migrations/")
	}
	return out, nil
}

// lettore è ciò che serve per interrogare schema_versione (pool, conn o tx).
type lettore interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Applicate restituisce le versioni registrate in schema_versione (vuoto se la tabella non esiste).
func Applicate(ctx context.Context, c lettore) (map[int]bool, error) {
	var esiste bool
	if err := c.QueryRow(ctx, "SELECT to_regclass('public.schema_versione') IS NOT NULL").Scan(&esiste); err != nil {
		return nil, fmt.Errorf("verifica schema: %w", err)
	}
	out := map[int]bool{}
	if !esiste {
		return out, nil
	}
	rows, err := c.Query(ctx, "SELECT versione FROM schema_versione ORDER BY versione")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// Applica porta il DB all'ultima versione disponibile. Restituisce il numero di file applicati.
func Applica(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, log *slog.Logger) (int, error) {
	migs, err := Elenca(fsys)
	if err != nil {
		return 0, err
	}
	return ApplicaElenco(ctx, pool, migs, len(migs), log)
}

// ApplicaFinoA applica le migrazioni fino alla versione indicata inclusa (per i test S2: dati inseriti
// alla versione precedente, poi migrazione, poi conteggi).
func ApplicaFinoA(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, finoA int, log *slog.Logger) (int, error) {
	migs, err := Elenca(fsys)
	if err != nil {
		return 0, err
	}
	return ApplicaElenco(ctx, pool, migs, finoA, log)
}

// ApplicaElenco applica, in ordine e una transazione per file, le migrazioni con versione <= finoA
// che non risultano già registrate. Rifiuta uno schema più recente del binario e versioni con buchi.
func ApplicaElenco(ctx context.Context, pool *pgxpool.Pool, migs []Migrazione, finoA int, log *slog.Logger) (int, error) {
	if err := VerificaStatica(migs); err != nil {
		return 0, err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrazioni: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockMigrazioni); err != nil {
		return 0, fmt.Errorf("migrazioni: lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockMigrazioni) }()

	fatte, err := Applicate(ctx, conn)
	if err != nil {
		return 0, err
	}
	massimaDB := 0
	for v := range fatte {
		if v > massimaDB {
			massimaDB = v
		}
	}
	if massimaDB > migs[len(migs)-1].Versione {
		return 0, fmt.Errorf("migrazioni: il DB è alla versione %d ma il binario conosce solo la %d: aggiornare cockpit.exe", massimaDB, migs[len(migs)-1].Versione)
	}
	for v := 1; v <= massimaDB; v++ {
		if !fatte[v] {
			return 0, fmt.Errorf("migrazioni: schema_versione ha la %d ma non la %d: DB incoerente, serve intervento manuale", massimaDB, v)
		}
	}
	n := 0
	for _, m := range migs {
		if m.Versione > finoA {
			break
		}
		if fatte[m.Versione] {
			continue
		}
		if err := applicaUna(ctx, conn.Conn(), m); err != nil {
			return n, err
		}
		n++
		if log != nil {
			log.Info("migrazione applicata", "file", m.Nome)
		}
	}
	if log != nil && n == 0 {
		log.Info("schema presente", "versione", massimaDB)
	}
	return n, nil
}

func applicaUna(ctx context.Context, conn *pgx.Conn, m Migrazione) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrazione %s: %w", m.Nome, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("migrazione %s: %w", m.Nome, err)
	}
	var registrata bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_versione WHERE versione = $1)", m.Versione).Scan(&registrata); err != nil {
		return fmt.Errorf("migrazione %s: schema_versione: %w", m.Nome, err)
	}
	if !registrata {
		return fmt.Errorf("migrazione %s: il file non registra la propria versione (manca INSERT INTO schema_versione)", m.Nome)
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------- verifica statica (S1, S4)

var (
	reCommento = regexp.MustCompile(`--[^\n]*`)
	reCreaTab  = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`)
	reRefs     = regexp.MustCompile(`(?i)\bREFERENCES\s+([a-z_][a-z0-9_]*)`)
	reAddValue = regexp.MustCompile(`(?i)\bALTER\s+TYPE\s+([a-z_][a-z0-9_]*)\s+ADD\s+VALUE\s+(?:IF\s+NOT\s+EXISTS\s+)?'([^']+)'`)
	reVersione = regexp.MustCompile(`(?i)INSERT\s+INTO\s+schema_versione\s*\(\s*versione\s*\)\s*VALUES\s*\(\s*(\d+)\s*\)`)
)

// VerificaStatica controlla, senza DB, che: ogni file registri la propria versione; ogni REFERENCES punti a
// una tabella creata nello stesso file o in uno precedente; un valore enum aggiunto non sia usato nello
// stesso file.
func VerificaStatica(migs []Migrazione) error {
	tabelle := map[string]bool{}
	for _, m := range migs {
		sql := reCommento.ReplaceAllString(m.SQL, "")
		v := reVersione.FindStringSubmatch(sql)
		if v == nil {
			return fmt.Errorf("verifica %s: manca INSERT INTO schema_versione (versione) VALUES (%d)", m.Nome, m.Versione)
		}
		if n, _ := strconv.Atoi(v[1]); n != m.Versione {
			return fmt.Errorf("verifica %s: registra la versione %d invece di %d", m.Nome, n, m.Versione)
		}
		for _, t := range reCreaTab.FindAllStringSubmatch(sql, -1) {
			tabelle[strings.ToLower(t[1])] = true
		}
		for _, r := range reRefs.FindAllStringSubmatch(sql, -1) {
			if !tabelle[strings.ToLower(r[1])] {
				return fmt.Errorf("verifica %s: REFERENCES %s ma la tabella non esiste nei file precedenti o in questo", m.Nome, r[1])
			}
		}
		for _, a := range reAddValue.FindAllStringSubmatch(sql, -1) {
			resto := strings.Replace(sql, a[0], "", 1)
			if strings.Contains(resto, "'"+a[2]+"'") {
				return fmt.Errorf("verifica %s: il valore enum %q di %s è aggiunto e usato nello stesso file (va usato dal file successivo)", m.Nome, a[2], a[1])
			}
		}
	}
	return nil
}
