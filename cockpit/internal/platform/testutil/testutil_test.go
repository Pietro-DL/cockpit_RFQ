package testutil

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// L1 — la guardia sul database di test si prova senza database: dice di no PRIMA di connettersi, ed
// e' proprio quello che deve fare. Un DSN che porta a un database di sviluppo non deve mai arrivare a
// SchemaVuoto, che distrugge lo schema.
func TestLaGuardiaLeggeIlNomeDelDatabaseComeLoLeggePgx(t *testing.T) {
	// PGDATABASE fissato qui, perche' il caso «senza database» sotto dipenda solo da questa prova e
	// non dall'ambiente di chi la lancia
	t.Setenv("PGDATABASE", "cockpit_dev")
	casi := []struct {
		nome   string
		dsn    string
		valido bool
	}{
		{"URL verso un database di test", "postgres://cockpit_test:cockpit_test@127.0.0.1:5433/cockpit_test_fx", true},
		{"chiave=valore verso un database di test", "host=127.0.0.1 port=5433 user=cockpit dbname=cockpit_test", true},
		{"URL verso lo sviluppo", "postgres://cockpit:x@127.0.0.1:5432/cockpit_dev", false},
		// il caso che passava: nessun percorso, e «tester» nel nome dell'utente sembrava un test
		{"chiave=valore verso lo sviluppo, utente «tester»", "host=x user=tester dbname=cockpit_dev", false},
		// il percorso dice test, il parametro dice altro: vince il parametro, come in pgx
		{"URL con ?dbname= che porta altrove", "postgres://u:p@127.0.0.1:5433/cockpit_test?dbname=cockpit_dev", false},
		// nessun nome nel DSN: decide l'ambiente (qui PGDATABASE=cockpit_dev), non il testo
		{"URL senza database", "postgres://tester:p@127.0.0.1:5433/", false},
		{"DSN illeggibile", "postgres://%zz", false},
	}
	for _, c := range casi {
		err := DatabaseDiTest(c.dsn)
		if c.valido && err != nil {
			t.Errorf("%s: rifiutato (%v), doveva passare", c.nome, err)
		}
		if !c.valido && err == nil {
			t.Errorf("%s: accettato %q — i test distruggerebbero lo schema di un database che non e' di test", c.nome, c.dsn)
		}
	}
}

// L1 — la guardia di SchemaVuoto sul collegamento (A1c-L1-02, la parte della guardia; piano A, 6.4.7): prima del
// DROP il database collegato deve avere «test» nel nome e l'utente non deve essere il ruolo del banco. I nomi di
// database e ruoli sono inventati.
func TestLaGuardiaDiSchemaVuotoGuardaIlCollegamento(t *testing.T) {
	for _, c := range []struct {
		nome                         string
		database, utente, ruoloBanco string
		ammesso                      bool
	}{
		{"database di prova, nessun ruolo del banco dichiarato", "acme_prova_test", "prove_acme", "", true},
		{"database di prova, utente diverso dal ruolo del banco", "acme_prova_test", "prove_acme", "lettore_acme", true},
		{"maiuscole nel nome", "ACME_Prova_TEST", "prove_acme", "", true},
		{"database senza «test»", "acme_copia_intatta", "prove_acme", "", false},
		{"database di sviluppo", "acme_dev", "prove_acme", "lettore_acme", false},
		{"il ruolo del banco su un database di prova", "acme_prova_test", "lettore_acme", "lettore_acme", false},
	} {
		err := schemaVuotoAmmesso(c.database, c.utente, c.ruoloBanco)
		if c.ammesso && err != nil {
			t.Errorf("%s: rifiutato (%v)", c.nome, err)
		}
		if !c.ammesso && err == nil {
			t.Errorf("%s: ammesso, e lo schema di %q sarebbe distrutto", c.nome, c.database)
		}
	}
}

// L1 — il ruolo del banco viene dalla variabile della copia del dump, letto come lo legge pgx; senza la variabile,
// o con un DSN che non si legge, non c'è (resta la guardia sul nome).
func TestIlRuoloDelBancoVieneDallaVariabileDelDump(t *testing.T) {
	t.Setenv("COCKPIT_DUMP_DSN", "postgres://lettore_acme@127.0.0.1:5432/acme_copia")
	if r := ruoloDelBanco(); r != "lettore_acme" {
		t.Errorf("ruolo del banco: %q", r)
	}
	t.Setenv("COCKPIT_DUMP_DSN", "host=127.0.0.1 user=lettore_kv dbname=acme_copia")
	if r := ruoloDelBanco(); r != "lettore_kv" {
		t.Errorf("ruolo del banco da un DSN chiave=valore: %q", r)
	}
	t.Setenv("COCKPIT_DUMP_DSN", "")
	if r := ruoloDelBanco(); r != "" {
		t.Errorf("senza la variabile il ruolo del banco è %q", r)
	}
	t.Setenv("COCKPIT_DUMP_DSN", "postgres://%zz")
	if r := ruoloDelBanco(); r != "" {
		t.Errorf("con un DSN che non si legge il ruolo del banco è %q", r)
	}
}

// L1 — dove le prove possono scrivere i suggerimenti dell'agente (A1c-L4S-11, la parte pura; R10): il database
// di prova o una copia usa e getta con il marcatore; mai un database senza l'uno e senza l'altro.
func TestDoveSiScrivonoLeRigheDiProva(t *testing.T) {
	for _, c := range []struct {
		nome, database, commento string
		ammesso                  bool
	}{
		{"database di prova", "acme_prova_test", "", true},
		{"copia usa e getta", "acme_copia_run", marcatoreCopiaUsaEGetta + "acme_copia (2026-10-06 08:00)", true},
		{"copia intatta: niente «test», niente marcatore", "acme_copia", "", false},
		{"commento qualunque", "acme_copia", "copia di lavoro", false},
		{"marcatore non all'inizio", "acme_copia", "nota: " + marcatoreCopiaUsaEGetta + "acme_copia", false},
	} {
		err := scritturaDiProvaAmmessa(c.database, c.commento)
		if c.ammesso != (err == nil) {
			t.Errorf("%s: ammesso=%v, errore %v", c.nome, c.ammesso, err)
		}
	}
}

// L1 — i testi SQL vietati a una lettura del motore A (A1c-L4S-02, -08: il registro SQL): le tabelle escluse, le
// scritture, i lucchetti; le letture passano, anche quando una colonna contiene una parola simile.
func TestIlRegistroSQLRiconosceITestiVietati(t *testing.T) {
	r := &RegistroSQL{}
	vietati := []string{
		"INSERT INTO utente (sigla) VALUES ($1)",
		"update documento_proposta set stato = stato where false",
		"DELETE FROM job",
		"TRUNCATE componente",
		"COPY allegato TO STDOUT",
		"LOCK TABLE documento",
		"SELECT pg_advisory_xact_lock(1)",
		"SELECT * FROM documento WHERE thread_id = $1 FOR UPDATE",
		"SELECT * FROM documento FOR NO KEY UPDATE",
		"SELECT * FROM documento FOR SHARE",
		"SELECT * FROM documento FOR KEY SHARE",
		"SELECT * FROM " + "analisi" + "_messaggio",
		"SELECT * FROM worker_credenziale",
		"SELECT token FROM sessione",
	}
	ammessi := []string{
		"begin isolation level repeatable read read only",
		"SHOW transaction_read_only",
		"SELECT now()",
		"SELECT aggiornato_il, ultimo_aggiornamento FROM thread_offerta",
		"SELECT * FROM v_fascicolo WHERE thread_id = $1 ORDER BY codice, tipo_documento",
		"rollback",
	}
	for _, s := range append(append([]string(nil), vietati...), ammessi...) {
		r.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: s})
	}
	r.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: vietati[0]}) // ripetuto: una volta sola
	got := r.Vietati()
	if strings.Join(got, "\n") != strings.Join(vietati, "\n") {
		t.Errorf("vietati:\n%s\natteso:\n%s", strings.Join(got, "\n"), strings.Join(vietati, "\n"))
	}
	if n := len(r.Voci()); n != len(vietati)+len(ammessi)+1 {
		t.Errorf("voci registrate: %d", n)
	}
	r.Azzera()
	if len(r.Voci()) != 0 || len(r.Vietati()) != 0 {
		t.Error("Azzera non ha dimenticato le voci")
	}
}

// L1 — le differenze fra due foto del database: valori diversi, tabelle in una sola foto, in ordine.
func TestDifferenzeFraDueFoto(t *testing.T) {
	prima := map[string]string{"allegato": "2 aa", "componente": "1 bb", "job": "0 -"}
	dopo := map[string]string{"allegato": "2 aa", "componente": "2 cc", "sessione": "1"}
	if got := strings.Join(Differenze(prima, dopo), ","); got != "componente,job,sessione" {
		t.Errorf("differenze: %q", got)
	}
	if d := Differenze(prima, prima); len(d) != 0 {
		t.Errorf("la stessa foto ha differenze: %v", d)
	}
}
