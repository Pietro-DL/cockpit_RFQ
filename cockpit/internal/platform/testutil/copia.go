package testutil

// copia.go: le copie TEMPLATE del dump fatte per essere scritte, nelle prove L4 private (piano A, A1c, 3.3.9,
// 6.4.7 e 7.4.7; R33 d, R10, R44): la _run, che usano MOTORE-SENZA-LLM sui dati veri e poi A1d, e le copie di A2.
// Le prove scrivono lì righe di prova e le tolgono; la copia intatta non si tocca mai. Nessun valore della copia
// sta nel codice: nome, ruolo, schema e sentinelle vengono dal manifest (dataset.Manifest.CopiaRun).

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/dataset"
)

// PoolCopiaDelDump apre COCKPIT_DUMP_COPIA_DSN e, prima di restituire il pool, controlla (controllaCopiaScrivibile):
// nome uguale a quello del manifest, senza «test» e diverso dalla copia intatta; il marcatore delle copie usa e
// getta nel commento del database; l'utente atteso; schema_versione di partenza; transazione scrivibile; le
// sentinelle del manifest, ricontrollate anche in t.Cleanup (le righe di prova si tolgono prima).
//
// Senza la variabile, o con una copia diversa da quella del manifest: NON ESEGUITA, mai un salto (R44). Non chiama
// mai SchemaVuoto: una copia si ricrea, non si azzera.
func PoolCopiaDelDump(t testing.TB, attesa dataset.CopiaAttesa) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(variabileDumpCopia)
	if dsn == "" {
		NonEseguita(t, variabileDumpCopia+" non impostata: serve una copia TEMPLATE del dump fatta per essere scritta (lo script del database di prova, modo -Copia), senza password (pgpass)")
	}
	if err := copiaScrivibileDSN(dsn); err != nil {
		NonEseguita(t, err.Error())
	}
	ctx := context.Background()
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		NonEseguita(t, variabileDumpCopia+": il pool non si apre (il DSN non si ripete)")
	}
	t.Cleanup(p.Close)
	leggi := leggiDalPool(p)
	if err := controllaCopiaScrivibile(ctx, leggi, attesa); err != nil {
		NonEseguita(t, "la copia usa e getta non è quella del manifest: "+err.Error())
	}
	t.Cleanup(func() {
		if err := controllaSentinelle(context.Background(), leggi, attesa.Sentinelle); err != nil {
			t.Errorf("la copia usa e getta non è tornata com'era (righe di prova rimaste?): %v", err)
		}
	})
	return p
}

// copiaScrivibileDSN controlla, prima di collegarsi, il database del DSN della copia scrivibile: nessun «test»
// nel nome (il database di prova si azzera), diverso dalla copia intatta (COCKPIT_DUMP_DSN) e dal database di
// prova (COCKPIT_TEST_DSN). L'errore non ripete il DSN.
func copiaScrivibileDSN(dsn string) error {
	nome := databaseDi(dsn)
	switch {
	case nome == "":
		return fmt.Errorf("%s: il DSN non si legge o non porta a nessun database (non si ripete)", variabileDumpCopia)
	case strings.Contains(strings.ToLower(nome), "test"):
		return fmt.Errorf("%s porta a %q, che ha «test» nel nome: una copia del dump non è il database di prova", variabileDumpCopia, nome)
	case nome == databaseDi(os.Getenv(variabileDump)):
		return fmt.Errorf("%s porta alla copia intatta (%q): le prove scrivono solo su una copia usa e getta (R10)", variabileDumpCopia, nome)
	case nome == databaseDi(os.Getenv(variabileTest)):
		return fmt.Errorf("%s porta al database di %s (%q)", variabileDumpCopia, variabileTest, nome)
	}
	return nil
}

// controllaCopiaScrivibile: i controlli puri di PoolCopiaDelDump, con un lettore che le prove L1 sostituiscono. Si
// ferma al primo che non passa, con un errore che lo nomina:
//   - current_database() è il Database del manifest, senza «test» e diverso dal database di COCKPIT_TEST_DSN;
//   - il commento del database comincia con il marcatore delle copie usa e getta, e la copia che il marcatore
//     nomina non è questa (la copia intatta non ha il marcatore: con lei al suo posto ci si ferma);
//   - l'utente è il Ruolo del manifest;
//   - schema_versione da 1 a Schema, e Schema uguale all'ultima migrazione del binario;
//   - la transazione è scrivibile (transaction_read_only e default_transaction_read_only = off);
//   - le Sentinelle uguali al manifest.
func controllaCopiaScrivibile(ctx context.Context, leggi LeggiValore, attesa dataset.CopiaAttesa) error {
	if err := controllaAttesa(attesa); err != nil {
		return err
	}
	if err := controllaNomeCopia(ctx, leggi, attesa.Database); err != nil {
		return err
	}
	commento, err := leggi(ctx, sqlCommentoDelDatabase)
	if err != nil {
		return fmt.Errorf("marcatore: %w", err)
	}
	if !strings.HasPrefix(commento, marcatoreCopiaUsaEGetta) {
		return fmt.Errorf("marcatore: il commento del database non è quello di una copia usa e getta (%q)", commento)
	}
	origine, _, _ := strings.Cut(strings.TrimPrefix(commento, marcatoreCopiaUsaEGetta), " ")
	if origine == "" || origine == attesa.Database {
		return fmt.Errorf("marcatore: il commento non nomina la copia da cui viene, o nomina questa stessa (%q)", commento)
	}
	utente, err := leggi(ctx, sqlUtenteCorrente)
	if err != nil {
		return fmt.Errorf("utente: %w", err)
	}
	if utente != attesa.Ruolo {
		return fmt.Errorf("utente: collegato come %q, il manifest dice %q", utente, attesa.Ruolo)
	}
	if err := controllaSchemaCopia(ctx, leggi, attesa.Schema); err != nil {
		return err
	}
	for _, c := range []struct{ sql, nome string }{{sqlTransazioneRO, "transaction_read_only"}, {sqlDefaultRO, "default_transaction_read_only"}} {
		v, err := leggi(ctx, c.sql)
		if err != nil {
			return fmt.Errorf("scrivibile: %s: %w", c.nome, err)
		}
		if v != "off" {
			return fmt.Errorf("scrivibile: %s è %q: la copia usa e getta deve essere scrivibile", c.nome, v)
		}
	}
	return controllaSentinelle(ctx, leggi, attesa.Sentinelle)
}
