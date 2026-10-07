// L1 — la copia intatta del dump, senza database (A1c-L1-02; piano A, 6.4.7; R10, R44): CopiaDelDump rifiuta i nomi
// che non possono essere la copia (con «test», «dev», «prod», o uguali al database di prova) senza ripetere il DSN;
// ControllaCopia, con un lettore finto, dà un errore per ogni deviazione dal manifest (ruolo, nome, sola lettura,
// codifica, schema diverso o con buchi, tabelle escluse leggibili, sentinelle, impronte: R117 b) e per un manifest
// che non descrive una copia. Le impronte una per una stanno in impronte_test.go.
//
// Copie, ruoli, tabelle e numeri sono inventati: i valori veri stanno solo nel manifest privato, e il repository
// è pubblico.

package testutil

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/dataset"
	"promatec/cockpit/internal/platform/migrazioni"
)

func TestCopiaDelDumpRifiutaINomiSbagliati(t *testing.T) {
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	for _, c := range []struct {
		nome, dsn, frase string
	}{
		{"la copia", "postgres://lettore_acme@127.0.0.1:5432/acme_copia_20260101", ""},
		{"chiave=valore", "host=127.0.0.1 user=lettore_acme dbname=acme_copia_20260101", ""},
		{"con «test»", "postgres://lettore_acme@127.0.0.1:5432/acme_copia_test", "«test»"},
		{"con «dev»", "postgres://lettore_acme@127.0.0.1:5432/acme_dev", "«dev»"},
		{"con «prod» in maiuscolo", "postgres://lettore_acme@127.0.0.1:5432/ACME_PROD", "«prod»"},
		{"il nome risolto vince sul percorso", "postgres://lettore_acme@127.0.0.1:5432/acme_copia?dbname=acme_dev", "«dev»"},
	} {
		err := CopiaDelDump(c.dsn)
		switch {
		case c.frase == "" && err != nil:
			t.Errorf("%s: rifiutato (%v)", c.nome, err)
		case c.frase != "" && (err == nil || !strings.Contains(err.Error(), c.frase)):
			t.Errorf("%s: %v, atteso che dica %s", c.nome, err, c.frase)
		}
	}
	// lo stesso database di COCKPIT_TEST_DSN, anche senza «test» nel nome
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_condiviso")
	if err := CopiaDelDump("postgres://lettore_acme@127.0.0.1:5432/acme_condiviso"); err == nil || !strings.Contains(err.Error(), "R10") {
		t.Errorf("la copia sul database di prova: %v", err)
	}
	const illeggibile = "postgres://lettore_acme:segreta@127.0.0.1:porta/acme_copia"
	err := CopiaDelDump(illeggibile)
	if err == nil || strings.Contains(err.Error(), "segreta") || strings.Contains(err.Error(), "acme_copia") {
		t.Errorf("un DSN che non si legge: %v (senza ripetere il DSN)", err)
	}
}

// lettoreDiProva: un LeggiValore che risponde per testo SQL; una domanda senza risposta è un errore.
func lettoreDiProva(risposte map[string]string, errori map[string]error) LeggiValore {
	return func(_ context.Context, sql string) (string, error) {
		if err, ok := errori[sql]; ok {
			return "", err
		}
		v, ok := risposte[sql]
		if !ok {
			return "", fmt.Errorf("domanda inattesa: %s", sql)
		}
		return v, nil
	}
}

func ultimaDelBinario(t *testing.T) int {
	t.Helper()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	return migs[len(migs)-1].Versione
}

// improntaACME: un'impronta inventata, 64 cifre esadecimali.
var improntaACME = strings.Repeat("ac", 32)

// improntaComponenteACME: l'impronta dichiarata della tabella componente nella copia inventata: tre colonne, una con
// il fuso orario, ordinate per id.
func improntaComponenteACME() dataset.ImprontaTabella {
	return dataset.ImprontaTabella{Colonne: []string{"id", "codice", "creato_il"}, Ordine: []string{"id"}, Sha256: improntaACME}
}

// classiComponenteACME: le classi che il catalogo inventato dà alle colonne di componente.
var classiComponenteACME = map[string]string{"id": classeTesto, "codice": classeTesto, "creato_il": classeFuso}

// copiaACME: la copia intatta come il manifest (inventato) la dichiara, con le sentinelle e l'impronta di una delle
// due tabelle (R117 b), e le risposte di un database che è proprio quella copia.
func copiaACME(t *testing.T) (dataset.CopiaAttesa, map[string]string) {
	t.Helper()
	ultima := ultimaDelBinario(t)
	imp := improntaComponenteACME()
	attesa := dataset.CopiaAttesa{
		Database:   "acme_copia",
		Ruolo:      "lettore_acme",
		Schema:     ultima,
		Escluse:    []string{"tabella_esclusa"},
		Sentinelle: map[string]int64{"componente": 3, "allegato": 5},
		Impronte: &dataset.ImpronteCopia{Versione: VersioneImpronta,
			Tabelle: map[string]dataset.ImprontaTabella{"componente": imp}},
	}
	n := strconv.Itoa(ultima)
	risposte := map[string]string{
		sqlUtenteCorrente:               "lettore_acme",
		sqlDatabaseCorrente:             "acme_copia",
		sqlTransazioneRO:                "on",
		sqlDefaultRO:                    "on",
		sqlCodifica:                     "UTF8",
		sqlVersioniSchema:               n + " 1 " + n,
		sqlLeggibile("tabella_esclusa"): "false",
		sqlRighe("componente"):          "3",
		sqlRighe("allegato"):            "5",
	}
	risposte[sqlClassiColonne("componente", imp.Colonne)] = "codice:testo creato_il:fuso id:testo"
	risposte[sqlImpronta("componente", imp.Colonne, imp.Ordine, classiComponenteACME)] = "3 " + improntaACME
	return attesa, risposte
}

func TestControllaCopiaUnaDeviazionePerVolta(t *testing.T) {
	ctx := context.Background()
	t.Setenv(variabileTest, "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	attesa, buone := copiaACME(t)
	if err := ControllaCopia(ctx, lettoreDiProva(buone, nil), attesa); err != nil {
		t.Fatalf("la copia giusta è rifiutata: %v", err)
	}
	ultima := ultimaDelBinario(t)
	n := strconv.Itoa(ultima)
	prima := strconv.Itoa(ultima - 1)

	for _, c := range []struct {
		nome     string
		risposta map[string]string
		errore   map[string]error
		manifest func(*dataset.CopiaAttesa)
		frase    string
	}{
		{nome: "un altro ruolo", risposta: map[string]string{sqlUtenteCorrente: "proprietario_acme"}, frase: "utente"},
		{nome: "un altro database", risposta: map[string]string{sqlDatabaseCorrente: "acme_altra"}, frase: "database"},
		{nome: "il database di prova", risposta: map[string]string{sqlDatabaseCorrente: "acme_prova_test"},
			manifest: func(a *dataset.CopiaAttesa) { a.Database = "acme_prova_test" }, frase: "«test»"},
		{nome: "transazione scrivibile", risposta: map[string]string{sqlTransazioneRO: "off"}, frase: "transaction_read_only"},
		{nome: "default scrivibile", risposta: map[string]string{sqlDefaultRO: "off"}, frase: "default_transaction_read_only"},
		{nome: "codifica", risposta: map[string]string{sqlCodifica: "WIN1252"}, frase: "client_encoding"},
		{nome: "schema alla versione prima", risposta: map[string]string{sqlVersioniSchema: prima + " 1 " + prima}, frase: "schema"},
		{nome: "schema con un buco", risposta: map[string]string{sqlVersioniSchema: prima + " 1 " + n}, frase: "buchi"},
		{nome: "il manifest dice un altro schema", manifest: func(a *dataset.CopiaAttesa) { a.Schema = ultima - 1 }, frase: "il binario"},
		{nome: "tabella esclusa leggibile", risposta: map[string]string{sqlLeggibile("tabella_esclusa"): "true"}, frase: "escluse"},
		{nome: "esclusa con un nome non valido", manifest: func(a *dataset.CopiaAttesa) { a.Escluse = []string{"x; drop"} }, frase: "non è un nome"},
		{nome: "una sentinella diversa", risposta: map[string]string{sqlRighe("componente"): "4"}, frase: "sentinelle: componente"},
		{nome: "un'impronta diversa, a righe uguali", risposta: map[string]string{
			sqlImpronta("componente", improntaComponenteACME().Colonne, improntaComponenteACME().Ordine, classiComponenteACME): "3 " + strings.Repeat("0", 64)},
			frase: "impronte: valori cambiati: componente"},
		{nome: "le righe dell'impronta diverse dalla sentinella", risposta: map[string]string{
			sqlImpronta("componente", improntaComponenteACME().Colonne, improntaComponenteACME().Ordine, classiComponenteACME): "4 " + improntaACME},
			frase: "le righe sono cambiate fra un controllo e l'altro"},
		{nome: "manifest senza impronte", manifest: func(a *dataset.CopiaAttesa) { a.Impronte = nil }, frase: "parte delle impronte non è eseguita"},
		{nome: "impronte di un'altra versione", manifest: func(a *dataset.CopiaAttesa) { a.Impronte.Versione = VersioneImpronta + 1 }, frase: "versione_impronta"},
		{nome: "manifest senza ruolo", manifest: func(a *dataset.CopiaAttesa) { a.Ruolo = "" }, frase: "ruolo"},
		{nome: "manifest senza database", manifest: func(a *dataset.CopiaAttesa) { a.Database = "" }, frase: "database"},
		{nome: "manifest senza sentinelle", manifest: func(a *dataset.CopiaAttesa) { a.Sentinelle = nil }, frase: "sentinelle"},
		{nome: "una domanda che fallisce", errore: map[string]error{sqlCodifica: errors.New("errore finto")}, frase: "client_encoding"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			a, risposte := copiaACME(t)
			for k, v := range c.risposta {
				risposte[k] = v
			}
			if c.manifest != nil {
				c.manifest(&a)
			}
			err := ControllaCopia(ctx, lettoreDiProva(risposte, c.errore), a)
			if err == nil || !strings.Contains(err.Error(), c.frase) {
				t.Errorf("errore %v, atteso che nomini %q", err, c.frase)
			}
		})
	}
}
