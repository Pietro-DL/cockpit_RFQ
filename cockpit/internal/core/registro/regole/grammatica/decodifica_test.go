package grammatica

import (
	"errors"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — la porta stretta di lettura (A1a-DEC; parte 1 §7.5; par.3.4.3 del piano A; T20, T21): un codice per
// caso, con il percorso del campo. encoding/json da solo accetterebbe in silenzio una chiave con le
// maiuscole sbagliate, lascerebbe vincere un doppione e cambierebbe un surrogato solo in U+FFFD: qui sono
// tutti errori di contratto.
//
// I clienti di questi test sono inventati: le varianti partono dalla grammatica ACME di valida_test.go, e
// ogni caso ne cambia un pezzo solo.

// erroreDecodifica: Decodifica deve fallire con un *evidenze.ErroreContratto; ne restituisce le
// diagnostiche.
func erroreDecodifica(t *testing.T, raw []byte) []evidenze.Diagnostica {
	t.Helper()
	g, err := Decodifica(raw)
	if err == nil {
		t.Fatal("Decodifica non ha dato errore")
	}
	var ec *evidenze.ErroreContratto
	if !errors.As(err, &ec) {
		t.Fatalf("errore %T, atteso *evidenze.ErroreContratto", err)
	}
	if g.VersioneSchema != 0 || g.Famiglie != nil {
		t.Fatal("con un errore la grammatica restituita deve essere vuota")
	}
	for _, d := range ec.Diagnostiche {
		if d.Gravita != evidenze.GravitaErrore || d.Natura != evidenze.NaturaContratto {
			t.Fatalf("diagnostica della lettura non è un errore di contratto:%s", elenca(ec.Diagnostiche))
		}
	}
	return ec.Diagnostiche
}

func TestA1aDECLetturaStretta(t *testing.T) {
	casi := []struct {
		nome     string
		raw      []byte
		codice   string
		percorso string
	}{
		{"chiave sconosciuta in testa", variante(t, `"versione_schema": 1,`, `"versione_schema": 1, "colore": "blu",`), CodiceChiaveSconosciuta, "colore"},
		{"campo legacy non coinvolto (R20 c)", variante(t, `"versione_schema": 1,`, `"versione_schema": 1, "frasi_portale": ["nel portale"],`), CodiceChiaveSconosciuta, "frasi_portale"},
		{"chiave sconosciuta annidata", variante(t, `{"id": "cartiglio", "selettori"`, `{"id": "cartiglio", "colore": "blu", "selettori"`), CodiceChiaveSconosciuta, "famiglie_codice[0].forme[2].colore"},
		{"chiave ripetuta", variante(t, `"id": "acme-punti",`, `"id": "acme-punti", "id": "acme-punti",`), CodiceChiaveRipetuta, "famiglie_codice[1].id"},
		{"chiave con le maiuscole diverse", variante(t, `"id": "acme-etichetta",`, `"ID": "acme-etichetta",`), CodiceChiaveMaiuscole, "famiglie_codice[2].ID"},
		{"null", variante(t, `"descrizione": "codici inventati con il prefisso di fase",`, `"descrizione": null,`), CodiceNull, "famiglie_codice[0].descrizione"},
		{"null in un elenco", variante(t, `"ruoli": ["componente"]`, `"ruoli": [null]`), CodiceNull, "famiglie_codice[1].ruoli[0]"},
		{"UTF-8 non valido", append([]byte(grammaticaACME[:20]), append([]byte{0xff}, grammaticaACME[20:]...)...), CodiceUTF8NonValido, ""},
		{"surrogato alto solo", variante(t, `"motivo": "domanda aperta inventata"`, `"motivo": "domanda \ud800 aperta"`), CodiceUTF8NonValido, ""},
		{"surrogato basso solo", variante(t, `"motivo": "domanda aperta inventata"`, `"motivo": "\udc00"`), CodiceUTF8NonValido, ""},
		{"surrogato alto seguito da un altro carattere", variante(t, `"motivo": "domanda aperta inventata"`, `"motivo": "\ud800A"`), CodiceUTF8NonValido, ""},
		{"BOM", append([]byte("\xef\xbb\xbf"), grammaticaACME...), CodiceBOM, ""},
		{"testo dopo l'oggetto", []byte(grammaticaACME + "\n{}"), CodiceValoreDopoOggetto, ""},
		{"numero con i decimali", variante(t, `"spazi_max": 1`, `"spazi_max": 1.0`), CodiceNumeroNonIntero, "famiglie_codice[2].etichette[0].spazi_max"},
		{"numero con l'esponente", variante(t, `"spazi_max": 1`, `"spazi_max": 1e0`), CodiceNumeroNonIntero, "famiglie_codice[2].etichette[0].spazi_max"},
		{"JSON malformato", []byte(grammaticaACME[:len(grammaticaACME)-1]), CodiceJSONNonValido, ""},
		{"virgola in più", variante(t, `"ruoli": ["componente"]`, `"ruoli": ["componente",]`), CodiceJSONNonValido, ""},
		{"tipo sbagliato", variante(t, `"spazi_max": 1`, `"spazi_max": "1"`), CodiceJSONNonValido, "famiglie_codice[2].etichette[0].spazi_max"},
		{"UUID illeggibile", variante(t, `"id": "00000000-0000-4000-8000-00000000ac01"`, `"id": "acme"`), CodiceJSONNonValido, "cliente.id"},
		{"equivalenza di tre elementi", variante(t, `"separatori": ["/"],`, `"separatori": ["/"], "equivalenze": [["00", "0", "000"]],`), CodiceJSONNonValido, "famiglie_codice[1].revisioni[0].equivalenze[0]"},
		{"un array al posto dell'oggetto", []byte(`[` + grammaticaACME + `]`), CodiceJSONNonValido, ""},
		{"annidamento troppo profondo", []byte(`{"versione_schema": 1, "x": ` + strings.Repeat("[", 100) + strings.Repeat("]", 100) + `}`), CodiceJSONNonValido, ""},
		{"file vuoto", []byte(""), CodiceJSONNonValido, ""},
		{"versione_schema come stringa", variante(t, `"versione_schema": 1`, `"versione_schema": "1"`), CodiceVersioneSchemaIgnota, "versione_schema"},
		{"versione_schema 1.0", variante(t, `"versione_schema": 1`, `"versione_schema": 1.0`), CodiceVersioneSchemaIgnota, "versione_schema"},
		// Un booleano o un intero obbligatorio non ha un valore sottinteso: assente non vuol dire false o 0.
		{"min assente", variante(t, `{"tipo": "revisione", "rif": "rev", "min": 0, "max": 1}`, `{"tipo": "revisione", "rif": "rev", "max": 1}`), CodiceCampoObbligatorio, "famiglie_codice[1].forme[0].parti[1].min"},
		{"identitario assente", variante(t, `"separatore": ".", "identitario": true}],`, `"separatore": "."}],`), CodiceCampoObbligatorio, "famiglie_codice[1].base.segmenti[2].identitario"},
		{"obbligatoria assente", variante(t, `"selettori": ["corpo", "oggetto"], "obbligatoria": true`, `"selettori": ["corpo", "oggetto"]`), CodiceCampoObbligatorio, "famiglie_codice[2].etichette[0].obbligatoria"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ds := erroreDecodifica(t, c.raw)
			richiedi(t, ds, c.codice, c.percorso)
		})
	}
}

// TestA1aDECTutteLeDiagnosticheInOrdine: la lettura non si ferma alla prima chiave sbagliata; le
// diagnostiche seguono l'ordine del file.
func TestA1aDECTutteLeDiagnosticheInOrdine(t *testing.T) {
	raw := variante(t,
		`"versione_schema": 1,`, `"versione_schema": 1, "colore": "blu",`,
		`"descrizione": "codici inventati con il prefisso di fase",`, `"descrizione": null,`,
		`"id": "acme-etichetta",`, `"Id": "acme-etichetta",`)
	ds := erroreDecodifica(t, raw)
	var codici []string
	for _, d := range ds {
		codici = append(codici, d.Codice)
	}
	atteso := []string{CodiceChiaveSconosciuta, CodiceNull, CodiceChiaveMaiuscole}
	if strings.Join(codici, " ") != strings.Join(atteso, " ") {
		t.Fatalf("codici %v, attesi %v nell'ordine del file", codici, atteso)
	}
}

// TestA1aDECLetturaRiuscita: la grammatica ACME si legge per intero; gli escape validi (anche una coppia di
// surrogati e il backspace, che poi rifiuta la validazione) arrivano come caratteri; spazi e a capo intorno
// all'oggetto non contano.
func TestA1aDECLetturaRiuscita(t *testing.T) {
	g, err := Decodifica([]byte("\n  " + grammaticaACME + "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if g.VersioneSchema != 1 || len(g.Famiglie) != 4 || g.Famiglie[3].ID != "acme-documento" {
		t.Fatalf("lettura incompleta: versione %d, %d famiglie", g.VersioneSchema, len(g.Famiglie))
	}
	if g.Cliente.ID.String() != "00000000-0000-4000-8000-00000000ac01" {
		t.Fatalf("cliente.id = %s", g.Cliente.ID)
	}
	fo := g.Famiglie[1].Forme[1]
	if fo.Completa || fo.ConfineDopo == nil || *fo.ConfineDopo != ConfineParolaASCIIOPuntoCifra || fo.ConfinePrima != nil {
		t.Fatalf("forma parziale letta male: %+v", fo)
	}
	if g.Famiglie[0].Esempi[1].RifCaso != "ACME-CASO-1" {
		t.Fatal("rif_caso non letto: il banco lo usa per la coerenza con i casi (R47 b)")
	}
	g2, err := Decodifica(variante(t, `"motivo": "domanda aperta inventata"`, `"motivo": "😀 è \b"`))
	if err != nil {
		t.Fatal(err)
	}
	if m := g2.Profilo.Riserve[0].Motivo; m != "\U0001F600 è \b" {
		t.Fatalf("escape letti male: %q", m)
	}
}
