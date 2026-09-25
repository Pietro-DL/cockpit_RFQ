package testutil

import "testing"

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
