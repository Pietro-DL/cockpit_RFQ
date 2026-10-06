package testutil

// registro.go: il registratore del testo SQL, per le prove che dimostrano che cosa un pacchetto chiede al
// database e su quale connessione (piano A, A1c, 6.4.7 e 3.3.9): il caricatore del motore A legge in una
// transazione sola, sulla stessa connessione, e chiude con ROLLBACK; MOTORE-SENZA-LLM non tocca mai la tabella
// dei suggerimenti dell'agente.

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VoceSQL: un testo SQL mandato al database, con la connessione che l'ha eseguito (il PID del processo del
// server, che distingue le connessioni del pool).
type VoceSQL struct {
	SQL         string
	Connessione uint32
}

// RegistroSQL: i testi SQL eseguiti da un pool, in ordine, BEGIN e ROLLBACK compresi. È un pgx.QueryTracer
// (pgx v5, tracer.go): pgx gli passa ogni Exec, Query e QueryRow, e Begin e Rollback sono Exec.
type RegistroSQL struct {
	mu   sync.Mutex
	voci []VoceSQL
}

// TraceQueryStart registra il testo e la connessione.
func (r *RegistroSQL) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	var pid uint32
	if conn != nil && conn.PgConn() != nil {
		pid = conn.PgConn().PID()
	}
	r.mu.Lock()
	r.voci = append(r.voci, VoceSQL{SQL: data.SQL, Connessione: pid})
	r.mu.Unlock()
	return ctx
}

// TraceQueryEnd non serve: conta che cosa si è chiesto, non come è andata.
func (r *RegistroSQL) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Voci: una copia dei testi registrati, in ordine.
func (r *RegistroSQL) Voci() []VoceSQL {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]VoceSQL(nil), r.voci...)
}

// Azzera dimentica i testi registrati: una prova lo chiama dopo aver preparato la scena con lo stesso pool.
func (r *RegistroSQL) Azzera() {
	r.mu.Lock()
	r.voci = nil
	r.mu.Unlock()
}

// sqlVietato: le tabelle che una lettura del motore A non tocca mai (la tabella dei suggerimenti dell'agente, le
// credenziali dei worker, le sessioni) e le parole che scrivono o prendono lucchetti. Le parole si cercano
// intere e senza badare alle maiuscole; LOCK anche dopo un trattino basso (le funzioni dei lucchetti
// consultivi). FOR UPDATE, FOR NO KEY UPDATE, FOR SHARE e FOR KEY SHARE stanno nelle stesse parole.
var sqlVietato = regexp.MustCompile(`(?i)analisi_messaggio|worker_credenziale|sessione` +
	`|\b(insert|update|delete|truncate|copy)\b` +
	`|(\b|_)lock\b` +
	`|\bfor\s+(no\s+key\s+update|key\s+share|share|update)\b`)

// Vietati restituisce i testi che contengono analisi_messaggio, worker_credenziale, sessione, INSERT, UPDATE,
// DELETE, TRUNCATE, COPY, LOCK o FOR UPDATE/SHARE/KEY SHARE, in ordine e senza ripetizioni.
func (r *RegistroSQL) Vietati() []string {
	var out []string
	visti := map[string]bool{}
	for _, v := range r.Voci() {
		if sqlVietato.MatchString(v.SQL) && !visti[v.SQL] {
			visti[v.SQL] = true
			out = append(out, v.SQL)
		}
	}
	return out
}

// PoolConRegistro apre un pool sul DSN dato con il registratore SQL, e lo chiude alla fine della prova. Non
// controlla il nome del database e non apre in sola lettura: sul database di prova lo si usa dopo testutil.DSN,
// sulla copia del dump solo dopo PoolDump, che ha fatto i controlli.
func PoolConRegistro(t testing.TB, dsn string) (*pgxpool.Pool, *RegistroSQL) {
	t.Helper()
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// l'errore di pgx può citare il DSN: non si ripete
		t.Fatal("pool con registro: il DSN non si legge")
	}
	r := &RegistroSQL{}
	pc.ConnConfig.Tracer = r
	p, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		t.Fatalf("pool con registro: %v", err)
	}
	t.Cleanup(p.Close)
	return p, r
}
