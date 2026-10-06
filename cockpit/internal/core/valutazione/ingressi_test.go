// L1 — LeggiIngressi, la decodifica stretta del file dei casi (A1c-L1-21; 6.6.3; par.3.4.3, regola 5; R29 a A, b C;
// R75 A): il file sintetico si legge per intero; chiavi ignote, ripetute, con maiuscole diverse, null, versione
// sbagliata o assente, BOM, UTF-8 non valido, surrogato solo, testo dopo l'oggetto, numeri non interi, origine o
// autorità diverse da «scenario», ID vuoti o ripetuti, thread ripetuti: errore di contratto, con il codice e il
// percorso, mai ingressi parziali.
package valutazione_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

func casiACME(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "casi_acme.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestL121IlFileDeiCasiSiLeggePerIntero(t *testing.T) {
	in, err := valutazione.LeggiIngressi(casiACME(t))
	if err != nil {
		t.Fatal(err)
	}
	tid := threadACME
	want := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{
		{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: ancoraggio.AutoritaScenario,
			Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:storia:1", Uso: "pertinente", Origine: "scenario",
				Motivo: "richiesta inoltrata senza confine: la storia non è separabile (R48 A)"}}},
		{ID: "ACME-ELENCO", ClienteID: clienteACME, Messaggi: []uuid.UUID{uid(0x91), uid(0x92)}},
	}}
	if !reflect.DeepEqual(in, want) {
		t.Fatalf("ingressi %+v\natteso %+v", in, want)
	}
	if c := in.CasoDelThread(threadACME); c == nil || c.ID != "ACME-SCENARIO" {
		t.Errorf("caso del thread %+v", c)
	}
	if c := in.CasoDelThread(uid(0x999)); c != nil {
		t.Errorf("un caso per un thread che non c'è: %+v", c)
	}
	if valutazione.VersioneCasi != 1 {
		t.Errorf("VersioneCasi %d: 6.6.3 dice 1", valutazione.VersioneCasi)
	}
}

func TestL121IlFileDeiCasiStretto(t *testing.T) {
	base := string(casiACME(t))
	sost := func(vecchio, nuovo string) []byte {
		if strings.Count(base, vecchio) != 1 {
			t.Fatalf("la variante non trova %q una volta sola", vecchio)
		}
		return []byte(strings.Replace(base, vecchio, nuovo, 1))
	}
	for _, c := range []struct {
		nome     string
		raw      []byte
		codice   string
		percorso string
	}{
		{"chiave ignota", sost(`"autorita": "scenario"`, `"autorita": "scenario", "attesi": 3`), grammatica.CodiceChiaveSconosciuta, "casi[0].attesi"},
		{"chiave ripetuta", sost(`"id": "ACME-ELENCO",`, `"id": "ACME-ELENCO", "id": "ACME-ALTRO",`), grammatica.CodiceChiaveRipetuta, "casi[1].id"},
		{"chiave con maiuscole diverse", sost(`"segmento_id"`, `"Segmento_ID"`), grammatica.CodiceChiaveMaiuscole, "casi[0].segmenti[0].Segmento_ID"},
		{"null", sost(`"autorita": "scenario"`, `"autorita": null`), grammatica.CodiceNull, "casi[0].autorita"},
		{"versione sbagliata", sost(`"versione_casi": 1`, `"versione_casi": 2`), grammatica.CodiceVersioneSchemaIgnota, "versione_casi"},
		{"versione non intera", sost(`"versione_casi": 1`, `"versione_casi": 1.0`), grammatica.CodiceVersioneSchemaIgnota, "versione_casi"},
		{"versione assente", sost(`"versione_casi": 1,`, ``), grammatica.CodiceVersioneSchemaIgnota, "versione_casi"},
		{"BOM", append([]byte("\xef\xbb\xbf"), base...), grammatica.CodiceBOM, ""},
		{"UTF-8 non valido", sost(`ACME-ELENCO`, "ACME-\xff"), grammatica.CodiceUTF8NonValido, ""},
		{"surrogato solo", sost(`ACME-ELENCO`, `ACME-\ud800`), grammatica.CodiceUTF8NonValido, ""},
		{"testo dopo l'oggetto", append([]byte(base), []byte(" {}")...), grammatica.CodiceValoreDopoOggetto, ""},
		{"JSON malformato", sost(`"casi": [`, `"casi": [[`), grammatica.CodiceJSONNonValido, ""},
		{"origine dell'operatore", sost(`"origine": "scenario"`, `"origine": "operatore"`), grammatica.CodiceEnumIgnoto, "casi[0].segmenti[0].origine"},
		{"uso fuori elenco", sost(`"uso": "pertinente"`, `"uso": "forse"`), grammatica.CodiceEnumIgnoto, "casi[0].segmenti[0].uso"},
		{"autorità confermata", sost(`"autorita": "scenario"`, `"autorita": "confermata"`), grammatica.CodiceEnumIgnoto, "casi[0].autorita"},
		{"ID vuoto", sost(`"id": "ACME-ELENCO"`, `"id": " "`), grammatica.CodiceIDVuoto, "casi[1].id"},
		{"ID ripetuto", sost(`"id": "ACME-ELENCO"`, `"id": "ACME-SCENARIO"`), grammatica.CodiceIDRipetuto, "casi[1].id"},
		{"thread ripetuto", sost(`"id": "ACME-ELENCO",`, `"id": "ACME-ELENCO", "thread_id": "00000000-0000-4000-8000-000000000071",`),
			grammatica.CodiceIDRipetuto, "casi[1].thread_id"},
		{"cliente assente", sost(`"cliente_id": "00000000-0000-4000-8000-00000000ac01",
      "messaggi"`, `"messaggi"`), grammatica.CodiceCampoObbligatorio, "casi[1].cliente_id"},
		{"segmento senza ID", sost(`"segmento_id": "s:storia:1"`, `"segmento_id": ""`), grammatica.CodiceIDVuoto, "casi[0].segmenti[0].segmento_id"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			in, err := valutazione.LeggiIngressi(c.raw)
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) {
				t.Fatalf("errore %v, atteso un errore di contratto", err)
			}
			trovato := false
			for _, d := range ec.Diagnostiche {
				trovato = trovato || (d.Codice == c.codice && d.Percorso == c.percorso && d.Gravita == evidenze.GravitaErrore && d.Natura == evidenze.NaturaContratto)
			}
			if !trovato {
				t.Errorf("diagnostiche %+v: attesa %s su %q", ec.Diagnostiche, c.codice, c.percorso)
			}
			if !reflect.DeepEqual(in, valutazione.Ingressi{}) {
				t.Errorf("con l'errore degli ingressi non vuoti: %+v", in)
			}
		})
	}

	t.Run("un surrogato intero e una barra scritta non sono errori", func(t *testing.T) {
		raw := bytes.Replace([]byte(base), []byte(`ACME-ELENCO`), []byte(`ACME-😀-\\ud800`), 1)
		if _, err := valutazione.LeggiIngressi(raw); err != nil {
			t.Errorf("errore %v", err)
		}
	})
}
