// L1 — i controlli puri di PoolCopiaDelDump (A1c-L1-02; piano A, 6.4.7 e 7.4.7; R33 d, R10, R44): il DSN di una copia
// usa e getta non porta né al database di prova né alla copia intatta; sul collegamento, nome e marcatore, utente,
// schema, transazione scrivibile e sentinelle, una deviazione per volta. Con la copia intatta al posto della copia
// usa e getta ci si ferma. A1d estende queste prove.
//
// Copie, ruoli e numeri sono inventati: i valori veri stanno solo nel manifest privato, e il repository è pubblico.

package testutil

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/dataset"
)

func TestIlDSNDellaCopiaUsaEGetta(t *testing.T) {
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	t.Setenv(variabileDump, "postgres://lettore_acme@127.0.0.1:5432/acme_copia")
	for _, c := range []struct {
		nome, dsn, frase string
	}{
		{"la copia usa e getta", "postgres://prove_acme@127.0.0.1:5432/acme_copia_run", ""},
		{"il database di prova", "postgres://prove_acme@127.0.0.1:5432/acme_prova_test", "«test»"},
		{"la copia intatta", "postgres://prove_acme@127.0.0.1:5432/acme_copia", "copia intatta"},
		{"un DSN che non si legge", "postgres://prove_acme:segreta@127.0.0.1:porta/acme_copia_run", "non si legge"},
	} {
		err := copiaScrivibileDSN(c.dsn)
		switch {
		case c.frase == "" && err != nil:
			t.Errorf("%s: rifiutato (%v)", c.nome, err)
		case c.frase != "" && (err == nil || !strings.Contains(err.Error(), c.frase)):
			t.Errorf("%s: %v, atteso che dica %q", c.nome, err, c.frase)
		}
		if err != nil && strings.Contains(err.Error(), "segreta") {
			t.Errorf("%s: l'errore ripete la password: %v", c.nome, err)
		}
	}
}

// copiaRunACME: la copia usa e getta come il manifest (inventato) la dichiara, e le risposte di un database che è
// proprio quella copia.
func copiaRunACME(t *testing.T) (dataset.CopiaAttesa, map[string]string) {
	t.Helper()
	ultima := ultimaDelBinario(t)
	attesa := dataset.CopiaAttesa{
		Database:   "acme_copia_run",
		Ruolo:      "prove_acme",
		Schema:     ultima,
		Sentinelle: map[string]int64{"componente": 3, "allegato": 5},
	}
	n := strconv.Itoa(ultima)
	return attesa, map[string]string{
		sqlDatabaseCorrente:    "acme_copia_run",
		sqlCommentoDelDatabase: marcatoreCopiaUsaEGetta + "acme_copia (2026-10-06 08:00)",
		sqlUtenteCorrente:      "prove_acme",
		sqlVersioniSchema:      n + " 1 " + n,
		sqlTransazioneRO:       "off",
		sqlDefaultRO:           "off",
		sqlRighe("componente"): "3",
		sqlRighe("allegato"):   "5",
	}
}

func TestIControlliDellaCopiaUsaEGetta(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	attesa, buone := copiaRunACME(t)
	if err := controllaCopiaScrivibile(ctx, lettoreDiProva(buone, nil), attesa); err != nil {
		t.Fatalf("la copia usa e getta giusta è rifiutata: %v", err)
	}
	prima := strconv.Itoa(ultimaDelBinario(t) - 1)

	// la copia intatta al suo posto: un altro nome, nessun marcatore, in sola lettura, un altro utente
	_, intatta := copiaACME(t)
	intatta[sqlCommentoDelDatabase] = ""
	if err := controllaCopiaScrivibile(ctx, lettoreDiProva(intatta, nil), attesa); err == nil {
		t.Error("con la copia intatta al posto della copia usa e getta il controllo passa")
	}
	// e anche con il nome giusto per sbaglio nel manifest: niente marcatore, sola lettura
	intatta[sqlDatabaseCorrente] = "acme_copia_run"
	if err := controllaCopiaScrivibile(ctx, lettoreDiProva(intatta, nil), attesa); err == nil || !strings.Contains(err.Error(), "marcatore") {
		t.Errorf("una copia senza marcatore: %v", err)
	}

	for _, c := range []struct {
		nome     string
		risposta map[string]string
		manifest func(*dataset.CopiaAttesa)
		frase    string
	}{
		{nome: "un altro database", risposta: map[string]string{sqlDatabaseCorrente: "acme_altra"}, frase: "database"},
		{nome: "senza marcatore", risposta: map[string]string{sqlCommentoDelDatabase: ""}, frase: "marcatore"},
		{nome: "il commento del database di prova", risposta: map[string]string{sqlCommentoDelDatabase: "database di prova inventato"}, frase: "marcatore"},
		{nome: "il marcatore nomina questa stessa copia", risposta: map[string]string{sqlCommentoDelDatabase: marcatoreCopiaUsaEGetta + "acme_copia_run (prova)"}, frase: "marcatore"},
		{nome: "il marcatore senza la copia d'origine", risposta: map[string]string{sqlCommentoDelDatabase: marcatoreCopiaUsaEGetta}, frase: "marcatore"},
		{nome: "un altro utente", risposta: map[string]string{sqlUtenteCorrente: "lettore_acme"}, frase: "utente"},
		{nome: "schema alla versione prima", risposta: map[string]string{sqlVersioniSchema: prima + " 1 " + prima}, frase: "schema"},
		{nome: "transazione in sola lettura", risposta: map[string]string{sqlTransazioneRO: "on"}, frase: "scrivibile"},
		{nome: "default in sola lettura", risposta: map[string]string{sqlDefaultRO: "on"}, frase: "scrivibile"},
		{nome: "una sentinella diversa", risposta: map[string]string{sqlRighe("allegato"): "6"}, frase: "sentinelle: allegato"},
		{nome: "manifest senza la copia _run", manifest: func(a *dataset.CopiaAttesa) { *a = dataset.CopiaAttesa{} }, frase: "manifest"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			a, risposte := copiaRunACME(t)
			for k, v := range c.risposta {
				risposte[k] = v
			}
			if c.manifest != nil {
				c.manifest(&a)
			}
			err := controllaCopiaScrivibile(ctx, lettoreDiProva(risposte, nil), a)
			if err == nil || !strings.Contains(err.Error(), c.frase) {
				t.Errorf("errore %v, atteso che nomini %q", err, c.frase)
			}
		})
	}
}
