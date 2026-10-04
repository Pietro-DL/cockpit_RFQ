package grammatica

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — l'indice delle regole e la ragione sociale (A1a-SNP per LeggiIndice e ControllaRagioneSociale;
// A1a-LIM per i limiti dell'indice; R29 d, R43 B; par.3.6 del piano A): il legame è per UUID, un cliente
// ripetuto o limiti assenti, nulli o oltre i tetti non attivano niente; la ragione sociale è un controllo in
// cui contano tutto salvo gli spazi ai bordi e le maiuscole ASCII.
//
// I clienti di questi test sono inventati: UUID di prova, file e sha256 di comodo, la ragione sociale di
// ACME. Nessun nome di file privato.

const indiceACME = `{
  "versione_indice": 1,
  "limiti": {
    "versione_limiti": "limiti-acme-1",
    "grammatica": {"max_famiglie": 20, "max_forme_per_famiglia": 16, "max_parti_per_forma": 12, "max_esempi": 64,
                   "max_lunghezza_pattern": 64, "max_ripetizione": 40, "max_lunghezza_letterale": 40},
    "riconoscimento": {"max_byte_unita": 1048576, "max_unita_documento": 10000, "max_letture_per_unita": 1000, "max_letture_documento": 20000},
    "anteprima": {"tempo_massimo_ms": 20000}
  },
  "grammatiche": [
    {"cliente_id": "00000000-0000-4000-8000-00000000ac01", "file": "acme.v1.json", "sha256": "0000000000000000000000000000000000000000000000000000000000000001"},
    {"cliente_id": "00000000-0000-4000-8000-00000000ac02", "file": "acme-due.v1.json", "sha256": "0000000000000000000000000000000000000000000000000000000000000002"}
  ]
}`

func indiceVariante(t *testing.T, sostituzioni ...string) []byte {
	t.Helper()
	s := indiceACME
	for i := 0; i+1 < len(sostituzioni); i += 2 {
		if n := strings.Count(s, sostituzioni[i]); n != 1 {
			t.Fatalf("indiceVariante: %q compare %d volte, non una", sostituzioni[i], n)
		}
		s = strings.Replace(s, sostituzioni[i], sostituzioni[i+1], 1)
	}
	return []byte(s)
}

func erroreIndice(t *testing.T, raw []byte) []evidenze.Diagnostica {
	t.Helper()
	ix, err := LeggiIndice(raw)
	if err == nil {
		t.Fatal("LeggiIndice non ha dato errore")
	}
	var ec *evidenze.ErroreContratto
	if !errors.As(err, &ec) {
		t.Fatalf("errore %T, atteso *evidenze.ErroreContratto", err)
	}
	if ix.Grammatiche != nil || ix.Limiti != (Limiti{}) {
		t.Fatal("con un errore l'indice restituito deve essere vuoto: nessuna grammatica si attiva")
	}
	return ec.Diagnostiche
}

func TestA1aSNPLeggiIndice(t *testing.T) {
	ix, err := LeggiIndice([]byte(indiceACME))
	if err != nil {
		t.Fatal(err)
	}
	if ix.VersioneIndice != 1 || len(ix.Grammatiche) != 2 || ix.Casi != nil {
		t.Fatalf("indice letto male: %+v", ix)
	}
	if ix.Limiti != limitiACME() {
		t.Fatalf("limiti = %+v, attesi quelli ACME", ix.Limiti)
	}
	if ix.Grammatiche[1].ClienteID != uuid.MustParse("00000000-0000-4000-8000-00000000ac02") || ix.Grammatiche[1].File != "acme-due.v1.json" {
		t.Fatalf("voce letta male: %+v", ix.Grammatiche[1])
	}
	// Il file dei casi può esserci, con il suo sha256 (A1c); il formato dell'indice non cambia.
	ix, err = LeggiIndice(indiceVariante(t, `"grammatiche": [`, `"casi": {"file": "../casi/casi-acme.v1.json", "sha256": "0000000000000000000000000000000000000000000000000000000000000003"},
  "grammatiche": [`))
	if err != nil {
		t.Fatal(err)
	}
	if ix.Casi == nil || ix.Casi.File != "../casi/casi-acme.v1.json" {
		t.Fatalf("casi = %+v", ix.Casi)
	}
}

func TestA1aSNPIndiceNonValido(t *testing.T) {
	casi := []struct {
		nome     string
		raw      []byte
		codice   string
		percorso string
	}{
		{"cliente ripetuto", indiceVariante(t, `"cliente_id": "00000000-0000-4000-8000-00000000ac02"`, `"cliente_id": "00000000-0000-4000-8000-00000000ac01"`), CodiceRegoleClienteRipetuto, "grammatiche[1].cliente_id"},
		{"cliente senza UUID", indiceVariante(t, `"cliente_id": "00000000-0000-4000-8000-00000000ac02"`, `"cliente_id": "00000000-0000-0000-0000-000000000000"`), CodiceCampoObbligatorio, "grammatiche[1].cliente_id"},
		{"file mancante", indiceVariante(t, `"file": "acme-due.v1.json", `, ``), CodiceCampoObbligatorio, "grammatiche[1].file"},
		{"sha256 mancante", indiceVariante(t, `, "sha256": "0000000000000000000000000000000000000000000000000000000000000002"`, ``), CodiceCampoObbligatorio, "grammatiche[1].sha256"},
		{"casi senza sha256", indiceVariante(t, `"grammatiche": [`, `"casi": {"file": "casi.v1.json"}, "grammatiche": [`), CodiceCampoObbligatorio, "casi.sha256"},
		{"versione_indice 2", indiceVariante(t, `"versione_indice": 1`, `"versione_indice": 2`), CodiceVersioneSchemaIgnota, "versione_indice"},
		{"versione_indice assente", indiceVariante(t, `"versione_indice": 1,`, ``), CodiceVersioneSchemaIgnota, "versione_indice"},
		{"chiave sconosciuta", indiceVariante(t, `"versione_indice": 1,`, `"versione_indice": 1, "cartella": "x",`), CodiceChiaveSconosciuta, "cartella"},
		{"chiave con le maiuscole diverse", indiceVariante(t, `"max_famiglie"`, `"Max_Famiglie"`), CodiceChiaveMaiuscole, "limiti.grammatica.Max_Famiglie"},
		{"null", indiceVariante(t, `"versione_limiti": "limiti-acme-1"`, `"versione_limiti": null`), CodiceNull, "limiti.versione_limiti"},
		{"BOM", append([]byte("\xef\xbb\xbf"), indiceACME...), CodiceBOM, ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			richiedi(t, erroreIndice(t, c.raw), c.codice, c.percorso)
		})
	}
}

// TestA1aLIMLimitiDellIndice: i limiti vengono dall'indice e il codice non ne ha di suoi (R43 B). Senza
// limiti, con un valore nullo o oltre il tetto, l'indice non vale e nessuna grammatica si attiva.
func TestA1aLIMLimitiDellIndice(t *testing.T) {
	inizio := strings.Index(indiceACME, `  "limiti": {`)
	fine := strings.Index(indiceACME, `  "grammatiche"`)
	senzaLimiti := indiceACME[:inizio] + indiceACME[fine:]
	casi := []struct {
		nome     string
		raw      []byte
		codice   string
		percorso string
	}{
		{"senza limiti", []byte(senzaLimiti), CodiceCampoObbligatorio, "limiti"},
		{"limiti vuoti", indiceVariante(t, indiceACME[inizio:fine], `  "limiti": {},
`), CodiceCampoObbligatorio, "limiti"},
		{"senza versione dei limiti", indiceVariante(t, `"versione_limiti": "limiti-acme-1",`, ``), CodiceCampoObbligatorio, "limiti.versione_limiti"},
		{"un valore nullo", indiceVariante(t, `"max_letture_per_unita": 1000`, `"max_letture_per_unita": 0`), CodiceCampoObbligatorio, "limiti.riconoscimento.max_letture_per_unita"},
		{"un valore assente", indiceVariante(t, `"anteprima": {"tempo_massimo_ms": 20000}`, `"anteprima": {}`), CodiceCampoObbligatorio, "limiti.anteprima.tempo_massimo_ms"},
		{"un valore oltre il tetto", indiceVariante(t, `"max_ripetizione": 40`, `"max_ripetizione": 101`), CodiceLimiteOltreTetto, "limiti.grammatica.max_ripetizione"},
		{"il tempo dell'anteprima oltre il tetto", indiceVariante(t, `"tempo_massimo_ms": 20000`, `"tempo_massimo_ms": 60001`), CodiceLimiteOltreTetto, "limiti.anteprima.tempo_massimo_ms"},
		{"un limite con i decimali", indiceVariante(t, `"max_esempi": 64`, `"max_esempi": 64.5`), CodiceNumeroNonIntero, "limiti.grammatica.max_esempi"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d := richiedi(t, erroreIndice(t, c.raw), c.codice, c.percorso)
			if d.Gravita != evidenze.GravitaErrore {
				t.Fatalf("%s con gravità %s", d.Codice, d.Gravita)
			}
		})
	}
}

func TestA1aSNPControllaRagioneSociale(t *testing.T) {
	g := Grammatica{Cliente: ClienteGrammatica{ID: uuid.MustParse("00000000-0000-4000-8000-00000000ac01"), RagioneSociale: "ACME S.p.A."}}
	for _, uguale := range []string{"ACME S.p.A.", "  acme s.P.a.\t", "Acme S.P.A.\n"} {
		if d := ControllaRagioneSociale(g, uguale); d != nil {
			t.Errorf("%q: diagnosi inattesa %s", uguale, d.Messaggio)
		}
	}
	for _, diversa := range []string{"ACME SpA", "ACME  S.p.A.", "ÀCME S.p.A.", "ACME S.p.A. Due", "ACME S.p.A.", ""} {
		d := ControllaRagioneSociale(g, diversa)
		if d == nil {
			t.Errorf("%q: attesa la diagnosi", diversa)
			continue
		}
		if d.Codice != CodiceRegoleRagioneSocialeDiscorde || d.Gravita != evidenze.GravitaAvviso || d.Natura != evidenze.NaturaDati {
			t.Errorf("%q: diagnosi %s %s %s", diversa, d.Codice, d.Gravita, d.Natura)
		}
		if strings.Contains(d.Messaggio, "ACME") {
			t.Errorf("il messaggio riporta la ragione sociale, che può finire nel log: %s", d.Messaggio)
		}
	}
	// Le maiuscole fuori dall'ASCII contano: «É» e «é» sono diverse.
	g.Cliente.RagioneSociale = "Società Él"
	if ControllaRagioneSociale(g, "società él") == nil {
		t.Error("le maiuscole fuori dall'ASCII non devono essere ignorate")
	}
}
