package testutil

// foto.go: l'impronta del contenuto del database, per le prove che dimostrano che una lettura non scrive
// (RO-DOMINIO; piano A, A1c, 6.4.7 e 3.3.9). La formula è quella di fotoDelDatabase delle prove di
// transport/web (nessunaget_db_test.go), che resta com'è: qui diventa un aiuto comune, perché la usano il
// caricatore del motore A, la valutazione e le prove sul dump.

import (
	"context"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValoreNonLeggibile è il valore della foto per una tabella che il ruolo non può leggere: la tabella si salta,
// ma resta nell'elenco, così una foto dice anche che cosa non ha potuto guardare (sul dump, letto dal ruolo del
// banco, sono le tabelle escluse).
const ValoreNonLeggibile = "non leggibile dal ruolo"

// FotoDelDatabase: per ogni tabella di base dello schema public, «righe md5(string_agg(riga::text ORDER BY
// riga::text))»; per le tabelle in soloConteggio, solo le righe (per esempio una tabella che chi guarda tocca da
// sé, come le sessioni). Le tabelle che il ruolo non può leggere (has_table_privilege) valgono
// ValoreNonLeggibile. Due foto uguali vogliono dire che nessuna tabella ha guadagnato, perso o cambiato righe.
func FotoDelDatabase(t testing.TB, p *pgxpool.Pool, soloConteggio ...string) map[string]string {
	t.Helper()
	ctx := context.Background()
	solo := map[string]bool{}
	for _, s := range soloConteggio {
		solo[s] = true
	}
	rows, err := p.Query(ctx, `SELECT c.relname::text, has_table_privilege(c.oid, 'SELECT')
		  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p')
		 ORDER BY c.relname::text COLLATE "C"`)
	if err != nil {
		t.Fatalf("foto del database: elenco delle tabelle: %v", err)
	}
	type tabella struct {
		nome      string
		leggibile bool
	}
	var tabelle []tabella
	for rows.Next() {
		var x tabella
		if err := rows.Scan(&x.nome, &x.leggibile); err != nil {
			rows.Close()
			t.Fatalf("foto del database: %v", err)
		}
		tabelle = append(tabelle, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("foto del database: %v", err)
	}
	foto := make(map[string]string, len(tabelle))
	for _, x := range tabelle {
		if !x.leggibile {
			foto[x.nome] = ValoreNonLeggibile
			continue
		}
		id := pgx.Identifier{x.nome}.Sanitize()
		sql := `SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM ` + id + ` x`
		if solo[x.nome] {
			sql = `SELECT count(*)::text FROM ` + id
		}
		var v string
		if err := p.QueryRow(ctx, sql).Scan(&v); err != nil {
			t.Fatalf("foto del database: %s: %v", x.nome, err)
		}
		foto[x.nome] = v
	}
	return foto
}

// Differenze dice quali tabelle sono diverse fra due foto, in ordine: con un valore diverso, o presenti in una
// sola delle due. Vuota se le foto coincidono.
func Differenze(prima, dopo map[string]string) []string {
	var diverse []string
	for tab, v := range dopo {
		if w, ok := prima[tab]; !ok || w != v {
			diverse = append(diverse, tab)
		}
	}
	for tab := range prima {
		if _, ok := dopo[tab]; !ok {
			diverse = append(diverse, tab)
		}
	}
	sort.Strings(diverse)
	return diverse
}
